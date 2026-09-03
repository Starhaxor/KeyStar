package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
)

// BearerVerifier verifies a session token presented in an Authorization header.
type BearerVerifier interface {
	Verify(string) (security.SessionClaims, error)
}

type sessionClaimsContextKey struct{}

// errDPoPPublicURIUnconfigured fails canonical URI construction closed when
// the trusted public scheme/host is missing or the path is not normalizable.
var errDPoPPublicURIUnconfigured = errors.New("dpop public uri unconfigured")

// SessionRateGate rate-limits session authentication before signature work.
// A nil gate disables limiting.
type SessionRateGate func(ctx context.Context, key string) (allowed bool, retryAfter int)

// SessionAuthConfig selects the session authentication flow from the
// authoritative application profile: legacy bearer or proof-bound DPoP.
type SessionAuthConfig struct {
	LegacyVerifier     BearerVerifier
	ProofBoundVerifier ProofBoundTokenVerifier
	Applications       ApplicationResolver
	Replays            DPoPReplayStore
	Now                func() time.Time
	// PublicScheme and PublicHost build the canonical external request URI
	// for DPoP verification. They come from trusted server configuration,
	// never from forwarding headers. Empty values fail closed.
	PublicScheme string
	PublicHost   string
	RateLimit    SessionRateGate
}

// RequireSession admits only requests bearing a verified session token.
// Legacy applications use the existing bearer flow; proof-bound
// applications require exactly one Authorization: DPoP header plus one DPoP
// proof that is verified statelessly and consumed atomically before the
// handler runs. Handlers receive the same SessionClaims either way.
func RequireSession(config SessionAuthConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		values := request.Header.Values("Authorization")
		if len(values) != 1 {
			writeInvalidSessionToken(writer, request)
			return
		}
		fields := strings.Fields(values[0])
		if len(fields) != 2 || fields[1] == "" {
			writeInvalidSessionToken(writer, request)
			return
		}
		if config.RateLimit != nil {
			digest := sha256.Sum256([]byte(values[0]))
			if allowed, _ := config.RateLimit(request.Context(), hex.EncodeToString(digest[:])); !allowed {
				WriteError(writer, request, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
				return
			}
		}
		switch {
		case strings.EqualFold(fields[0], "Bearer"):
			serveBearerSession(config, writer, request, next, fields[1])
		case strings.EqualFold(fields[0], "DPoP"):
			serveDPoPSession(config, writer, request, next, fields[1])
		default:
			writeInvalidSessionToken(writer, request)
		}
	})
}

// serveBearerSession keeps the existing bearer flow for legacy
// applications. Proof-bound applications and proof-bound tokens are
// rejected without any DPoP retry.
func serveBearerSession(config SessionAuthConfig, writer http.ResponseWriter, request *http.Request, next http.Handler, token string) {
	if config.LegacyVerifier == nil {
		writeInvalidSessionToken(writer, request)
		return
	}
	claims, err := config.LegacyVerifier.Verify(token)
	if err != nil {
		writeInvalidSessionToken(writer, request)
		return
	}
	if claims.ProofBound != nil {
		writeInvalidSessionToken(writer, request)
		return
	}
	if config.Applications != nil {
		application, err := config.Applications.FindApplicationByID(request.Context(), claims.ApplicationID)
		if err != nil || application == nil {
			WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
			return
		}
		if application.AuthProfile == domain.ApplicationAuthProofBound {
			writeInvalidSessionToken(writer, request)
			return
		}
	}
	next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), sessionClaimsContextKey{}, claims)))
}

// serveDPoPSession verifies a proof-bound DPoP request: authoritative
// application resolution from the token's signed application hint, token
// verification with the active application key, stateless proof checks
// against the server-built canonical URI, then atomic replay consumption.
// Any failure denies without invoking the handler and never retries bearer.
func serveDPoPSession(config SessionAuthConfig, writer http.ResponseWriter, request *http.Request, next http.Handler, token string) {
	proofs := request.Header.Values("DPoP")
	if len(proofs) != 1 || proofs[0] == "" {
		writeInvalidSessionToken(writer, request)
		return
	}
	if config.Applications == nil {
		writeInvalidSessionToken(writer, request)
		return
	}
	applicationID, ok := unverifiedTokenApplicationID(token)
	if !ok {
		writeInvalidSessionToken(writer, request)
		return
	}
	application, err := config.Applications.FindApplicationByID(request.Context(), applicationID)
	if err != nil || application == nil {
		WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	if application.AuthProfile != domain.ApplicationAuthProofBound {
		writeInvalidSessionToken(writer, request)
		return
	}
	if config.ProofBoundVerifier == nil || config.Replays == nil {
		WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	claims, err := config.ProofBoundVerifier.Verify(request.Context(), application.ID, token)
	if err != nil {
		writeInvalidSessionToken(writer, request)
		return
	}
	// The signed application boundary wins over every client hint.
	if claims.ApplicationID != application.ID || claims.ProofBound == nil {
		writeInvalidSessionToken(writer, request)
		return
	}
	uri, err := canonicalDPoPURI(config.PublicScheme, config.PublicHost, dpopRequestPath(request))
	if err != nil {
		WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	now := time.Now().UTC()
	if config.Now != nil {
		now = config.Now().UTC()
	}
	proof, err := security.VerifyDPoP(security.DPoPInput{
		Proof: proofHeaderValue(proofs[0]), AccessToken: token, Method: request.Method,
		URI: uri, Token: claims, Now: now,
	})
	if err != nil {
		writeInvalidSessionToken(writer, request)
		return
	}
	consumed, err := config.Replays.ConsumeDPoP(request.Context(), claims.ApplicationID, proof.JTIDigest, claims.ProofBound.TokenID, claims.ExpiresAt)
	if err != nil {
		WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	if !consumed {
		writeInvalidSessionToken(writer, request)
		return
	}
	next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), sessionClaimsContextKey{}, claims)))
}

// unverifiedTokenApplicationID extracts the application hint from a token
// payload without verification. It only routes to the authoritative
// verifier; the verified claims decide.
func unverifiedTokenApplicationID(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[1] == "" {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(payload) == 0 || len(payload) > 16*1024 {
		return "", false
	}
	var hint struct {
		ApplicationID string `json:"app"`
	}
	if err := json.Unmarshal(payload, &hint); err != nil || hint.ApplicationID == "" {
		return "", false
	}
	return hint.ApplicationID, true
}

// proofHeaderValue normalizes the DPoP header to the compact proof form.
func proofHeaderValue(header string) string {
	return strings.Join(strings.Fields(header), "")
}

// dpopRequestPath returns the normalized request path without query or
// fragment for canonical URI construction.
func dpopRequestPath(request *http.Request) string {
	if request == nil || request.URL == nil {
		return "/"
	}
	if escaped := request.URL.EscapedPath(); escaped != "" {
		return escaped
	}
	if request.URL.Path != "" {
		return request.URL.Path
	}
	return "/"
}

// canonicalDPoPURI builds the absolute request URI from trusted server
// configuration plus the normalized path. Forwarding headers are ignored.
func canonicalDPoPURI(scheme, host, path string) (string, error) {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	host = strings.ToLower(strings.TrimSpace(host))
	if scheme == "" || host == "" {
		return "", errDPoPPublicURIUnconfigured
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#") {
		return "", errDPoPPublicURIUnconfigured
	}
	return scheme + "://" + host + path, nil
}

// SessionClaimsFromContext returns the verified session claims, when present.
func SessionClaimsFromContext(ctx context.Context) (security.SessionClaims, bool) {
	claims, ok := ctx.Value(sessionClaimsContextKey{}).(security.SessionClaims)
	return claims, ok
}

func writeInvalidSessionToken(writer http.ResponseWriter, request *http.Request) {
	WriteError(writer, request, http.StatusUnauthorized, "INVALID_SESSION_TOKEN", "invalid session token")
}
