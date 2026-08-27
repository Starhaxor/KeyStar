package adminapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/service/adminauth"
)

const maxAdminUserAgentLength = 200

type adminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type adminMFARequest struct {
	MFAToken     string `json:"mfa_token"`
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}

func (router *Router) handleAdminLogin(writer http.ResponseWriter, request *http.Request) {
	ipAddress := httpapi.ClientIP(request, router.TrustedProxies())
	if !router.AllowAdminRate(request.Context(), ipAddress+"|admin-login") {
		router.RecordSecurityEvent(request, nil, "ADMIN_LOGIN_RATE_LIMITED", "warning", nil)
		httpapi.WriteError(writer, request, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), router.LoginTimeout())
	defer cancel()
	request = request.WithContext(ctx)

	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		httpapi.WriteError(writer, request, http.StatusUnsupportedMediaType, "INVALID_REQUEST", "invalid request")
		return
	}
	body, err := decodeAdminLoginRequest(writer, request)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			httpapi.WriteError(writer, request, http.StatusRequestEntityTooLarge, "INVALID_REQUEST", "invalid request")
			return
		}
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if strings.TrimSpace(body.Email) == "" || body.Password == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}

	result, err := router.Admin.Auth.Login(ctx, body.Email, body.Password, ipAddress, request.UserAgent())
	if errors.Is(err, adminauth.ErrInvalidCredentials) {
		router.RecordSecurityEvent(request, nil, "ADMIN_LOGIN_FAILED", "warning", map[string]string{"email": strings.ToLower(strings.TrimSpace(body.Email))})
		httpapi.WriteError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid credentials")
		return
	}
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}

	if result.MFARequired {
		httpapi.WriteJSON(writer, http.StatusOK, struct {
			OK          bool   `json:"ok"`
			MFARequired bool   `json:"mfa_required"`
			MFAToken    string `json:"mfa_token"`
			Email       string `json:"email"`
		}{
			OK:          true,
			MFARequired: true,
			MFAToken:    result.ChallengeToken,
			Email:       result.Account.Email,
		})
		return
	}

	router.SetAdminCookies(writer, result.Token)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK        bool   `json:"ok"`
		Email     string `json:"email"`
		ExpiresAt string `json:"expires_at"`
	}{
		OK:        true,
		Email:     result.Account.Email,
		ExpiresAt: router.Now().Add(router.Admin.SessionTTL).UTC().Format(time.RFC3339),
	})
}

// handleAdminMFA completes a password-verified login with a TOTP or recovery
// code. The challenge token issued by handleAdminLogin is single-use.
func (router *Router) handleAdminMFA(writer http.ResponseWriter, request *http.Request) {
	ipAddress := httpapi.ClientIP(request, router.TrustedProxies())
	if !router.AllowAdminRate(request.Context(), ipAddress+"|admin-mfa") {
		router.RecordSecurityEvent(request, nil, "ADMIN_MFA_RATE_LIMITED", "warning", nil)
		httpapi.WriteError(writer, request, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), router.LoginTimeout())
	defer cancel()
	request = request.WithContext(ctx)

	var body adminMFARequest
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if strings.TrimSpace(body.MFAToken) == "" || (strings.TrimSpace(body.Code) == "" && strings.TrimSpace(body.RecoveryCode) == "") {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "mfa_token and a code or recovery_code are required")
		return
	}

	tokenValue, account, err := router.Admin.Auth.CompleteMFA(ctx, body.MFAToken, body.Code, body.RecoveryCode, ipAddress, request.UserAgent())
	switch {
	case errors.Is(err, adminauth.ErrMFAChallengeExpired):
		httpapi.WriteError(writer, request, http.StatusUnauthorized, "MFA_CHALLENGE_EXPIRED", "mfa challenge expired")
		return
	case errors.Is(err, adminauth.ErrInvalidMFACode):
		router.RecordSecurityEvent(request, account, "ADMIN_MFA_FAILED", "warning", nil)
		httpapi.WriteError(writer, request, http.StatusUnauthorized, "INVALID_MFA_CODE", "invalid mfa code")
		return
	case errors.Is(err, adminauth.ErrMFANotEnrolled):
		httpapi.WriteError(writer, request, http.StatusBadRequest, "MFA_NOT_ENROLLED", "mfa not enrolled")
		return
	case errors.Is(err, adminauth.ErrInvalidCredentials):
		httpapi.WriteError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid credentials")
		return
	case err != nil:
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}

	router.SetAdminCookies(writer, tokenValue)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK        bool   `json:"ok"`
		Email     string `json:"email"`
		ExpiresAt string `json:"expires_at"`
	}{
		OK:        true,
		Email:     account.Email,
		ExpiresAt: router.Now().Add(router.Admin.SessionTTL).UTC().Format(time.RFC3339),
	})
}

func (router *Router) handleAdminLogout(writer http.ResponseWriter, request *http.Request, session *domain.AdminSession, sessionToken string) {
	if err := router.Admin.Auth.Logout(request.Context(), sessionToken); err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	router.ClearAdminCookies(writer)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (router *Router) handleAdminMe(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK          bool     `json:"ok"`
		ID          string   `json:"id"`
		Email       string   `json:"email"`
		Status      string   `json:"status"`
		Role        string   `json:"role"`
		Permissions []string `json:"permissions"`
		MFAEnrolled bool     `json:"mfa_enrolled"`
	}{
		OK:          true,
		ID:          account.ID,
		Email:       account.Email,
		Status:      string(account.Status),
		Role:        account.RoleName,
		Permissions: account.Permissions,
		MFAEnrolled: account.MFAEnrolled,
	})
}

// adminCookieSameSite returns SameSite=None when cookies are Secure (the API
// is served over HTTPS from a different origin than the admin panel), because
// browsers otherwise withhold Lax cookies from cross-site fetch() requests and
// the session never sticks. SameSite=None is only honored on Secure cookies,
// so plain-HTTP/local deployments keep SameSite=Lax.
func adminCookieSameSite(secure bool) http.SameSite {
	if secure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

func (router *Router) SetAdminCookies(writer http.ResponseWriter, sessionToken string) {
	maxAge := int(router.Admin.SessionTTL.Seconds())
	http.SetCookie(writer, &http.Cookie{
		Name:     httpapi.AdminSessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   router.Admin.CookieSecure,
		SameSite: adminCookieSameSite(router.Admin.CookieSecure),
	})
	http.SetCookie(writer, &http.Cookie{
		Name:     httpapi.AdminCSRFCookieName,
		Value:    router.AdminCSRFToken(sessionToken),
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   router.Admin.CookieSecure,
		SameSite: adminCookieSameSite(router.Admin.CookieSecure),
	})
}

func (router *Router) ClearAdminCookies(writer http.ResponseWriter) {
	secure := router.Admin.CookieSecure
	http.SetCookie(writer, &http.Cookie{
		Name:     httpapi.AdminSessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: adminCookieSameSite(secure),
	})
	http.SetCookie(writer, &http.Cookie{
		Name:     httpapi.AdminCSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secure,
		SameSite: adminCookieSameSite(secure),
	})
}

func decodeAdminLoginRequest(writer http.ResponseWriter, request *http.Request) (adminLoginRequest, error) {
	request.Body = http.MaxBytesReader(writer, request.Body, httpapi.MaxRequestBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var body adminLoginRequest
	if err := decoder.Decode(&body); err != nil {
		return adminLoginRequest{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return adminLoginRequest{}, errors.New("multiple JSON values")
		}
		return adminLoginRequest{}, err
	}
	return body, nil
}

func hashClientIP(request *http.Request, trustedProxies []netip.Prefix) string {
	ipAddress := httpapi.ClientIP(request, trustedProxies)
	if ipAddress == "" || ipAddress == "unknown" {
		return ""
	}
	digest := sha256.Sum256([]byte(ipAddress))
	return hex.EncodeToString(digest[:])
}

func truncateAdminUserAgent(userAgent string) string {
	userAgent = strings.TrimSpace(userAgent)
	if len(userAgent) > maxAdminUserAgentLength {
		return userAgent[:maxAdminUserAgentLength]
	}
	return userAgent
}

func jsonMarshal(value any) (json.RawMessage, error) {
	return json.Marshal(value)
}
