package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/starloader/backend/internal/credential"
	"github.com/starloader/backend/internal/domain"
)

const (
	applicationHeader = "X-KeyStar-App"
)

// AppPrincipal is the resolved application context of a request, produced by
// the application + credential middleware and consumed by handlers.
type AppPrincipal struct {
	ApplicationID  string
	OrganizationID string
	CredentialID   string
	CredentialType string
	Environment    string
	Scopes         map[string]struct{}
}

type appPrincipalContextKey struct{}

// AppPrincipalFromContext returns the principal installed by the application
// middleware. A request that never passed the middleware has no principal.
func AppPrincipalFromContext(ctx context.Context) (AppPrincipal, bool) {
	principal, ok := ctx.Value(appPrincipalContextKey{}).(AppPrincipal)
	return principal, ok
}

func withAppPrincipal(ctx context.Context, principal AppPrincipal) context.Context {
	return context.WithValue(ctx, appPrincipalContextKey{}, principal)
}

// ApplicationResolver resolves an application by ID for request resolution.
type ApplicationResolver interface {
	FindApplicationByID(context.Context, string) (*domain.Application, error)
}

// CredentialVerifier validates a credential key against one application.
type CredentialVerifier interface {
	Verify(context.Context, string, string) (*domain.ApplicationCredential, error)
}

func credentialScopes(scopes []string) map[string]struct{} {
	set := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		set[scope] = struct{}{}
	}
	return set
}

// requireCredential protects an endpoint with application resolution and
// credential validation. Endpoint scopes must be granted on the credential;
// publishable endpoints reject secret keys and vice versa.
func (router *Router) requireCredential(requiredType domain.CredentialType, requiredScopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			principal, ok := router.resolveApplicationPrincipal(writer, request)
			if !ok {
				return
			}
			authorization := strings.TrimSpace(request.Header.Get("Authorization"))
			if authorization == "" {
				// Legacy compatibility (phase A): requests without a
				// credential fall back to the default application principal.
				// Removed when the legacy mode is disabled.
				if router.disableLegacyApplication {
					writeError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "application credential required")
					return
				}
				next.ServeHTTP(writer, request.WithContext(withAppPrincipal(request.Context(), principal)))
				return
			}
			key, ok := bearerKey(authorization)
			if !ok {
				writeError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
				return
			}
			if requiredType == domain.CredentialPublishable && !strings.HasPrefix(key, credential.PrefixPublishableLive) && !strings.HasPrefix(key, credential.PrefixPublishableTest) {
				writeError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
				return
			}
			if requiredType == domain.CredentialSecret && !strings.HasPrefix(key, credential.PrefixSecretLive) && !strings.HasPrefix(key, credential.PrefixSecretTest) {
				writeError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
				return
			}
			if router.credentials == nil {
				writeError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
				return
			}
			applicationCredential, err := router.credentials.Verify(request.Context(), principal.ApplicationID, key)
			if err != nil {
				router.writeCredentialVerificationError(writer, request, err)
				return
			}
			if applicationCredential == nil {
				writeError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
				return
			}
			if applicationCredential.CredentialType != requiredType {
				writeError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
				return
			}
			granted := credentialScopes(applicationCredential.Scopes)
			for _, required := range requiredScopes {
				if _, ok := granted[required]; !ok {
					writeError(writer, request, http.StatusForbidden, "INSUFFICIENT_SCOPE", "insufficient scope")
					return
				}
			}
			principal.CredentialID = applicationCredential.ID
			principal.CredentialType = string(applicationCredential.CredentialType)
			principal.Environment = string(applicationCredential.Environment)
			principal.Scopes = granted
			next.ServeHTTP(writer, request.WithContext(withAppPrincipal(request.Context(), principal)))
		})
	}
}

// resolveApplicationPrincipal resolves the application boundary of a request
// from the X-KeyStar-App header or the configured default application, and
// validates that the application is operational.
func (router *Router) resolveApplicationPrincipal(writer http.ResponseWriter, request *http.Request) (AppPrincipal, bool) {
	applicationID := strings.TrimSpace(request.Header.Get(applicationHeader))
	if applicationID == "" {
		applicationID = router.defaultApplicationID
	}
	if router.applications == nil {
		// Application resolution is not configured (bare unit-test routers).
		// Production wiring always provides the resolver and the default ID.
		if applicationID == "" || !validCanonicalUUID(applicationID) {
			return AppPrincipal{}, true
		}
		return AppPrincipal{ApplicationID: applicationID}, true
	}
	if applicationID == "" || !validCanonicalUUID(applicationID) {
		writeError(writer, request, http.StatusBadRequest, "INVALID_APPLICATION", "invalid application")
		return AppPrincipal{}, false
	}
	application, err := router.applications.FindApplicationByID(request.Context(), applicationID)
	if errors.Is(err, domain.ErrApplicationNotFound) {
		writeError(writer, request, http.StatusNotFound, "INVALID_APPLICATION", "invalid application")
		return AppPrincipal{}, false
	}
	if err != nil {
		writeError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return AppPrincipal{}, false
	}
	if application == nil {
		writeError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return AppPrincipal{}, false
	}
	switch application.Status {
	case domain.ApplicationStatusSuspended, domain.ApplicationStatusDisabled:
		writeError(writer, request, http.StatusForbidden, "APPLICATION_DISABLED", "application disabled")
		return AppPrincipal{}, false
	case domain.ApplicationStatusMaintenance:
		writeError(writer, request, http.StatusServiceUnavailable, "APPLICATION_MAINTENANCE", "application maintenance")
		return AppPrincipal{}, false
	case domain.ApplicationStatusActive:
	default:
		writeError(writer, request, http.StatusForbidden, "APPLICATION_DISABLED", "application disabled")
		return AppPrincipal{}, false
	}
	return AppPrincipal{
		ApplicationID:  application.ID,
		OrganizationID: application.OrganizationID,
	}, true
}

func bearerKey(authorization string) (string, bool) {
	scheme, value, found := strings.Cut(authorization, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(value) == "" {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func (router *Router) writeCredentialVerificationError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrCredentialRevoked):
		writeError(writer, request, http.StatusUnauthorized, "CREDENTIAL_REVOKED", "credential revoked")
	case errors.Is(err, domain.ErrCredentialExpired):
		writeError(writer, request, http.StatusUnauthorized, "CREDENTIAL_EXPIRED", "credential expired")
	case errors.Is(err, domain.ErrInvalidCredential):
		writeError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
	default:
		writeError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
	}
}
