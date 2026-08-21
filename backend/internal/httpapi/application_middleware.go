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

func credentialScopes(scopes []string) map[string]struct{} {
	set := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		set[scope] = struct{}{}
	}
	return set
}

// RequireCredential protects an endpoint with application resolution and
// credential validation. Endpoint scopes must be granted on the credential;
// publishable endpoints reject secret keys and vice versa.
func (router *Router) RequireCredential(requiredType domain.CredentialType, requiredScopes ...string) func(http.Handler) http.Handler {
	return router.requireCredentialMode(requiredType, false, requiredScopes...)
}

// RequireServerCredential is like RequireCredential but never falls back to
// the legacy default application: the server API is machine-to-machine only
// and always demands a valid secret key.
func (router *Router) RequireServerCredential(requiredType domain.CredentialType, requiredScopes ...string) func(http.Handler) http.Handler {
	return router.requireCredentialMode(requiredType, true, requiredScopes...)
}

func (router *Router) requireCredentialMode(requiredType domain.CredentialType, strict bool, requiredScopes ...string) func(http.Handler) http.Handler {
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
				// Removed when the legacy mode is disabled. Strict mode (the
				// server API) never falls back.
				if strict || router.disableLegacyApplication {
					WriteError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "application credential required")
					return
				}
				next.ServeHTTP(writer, request.WithContext(withAppPrincipal(request.Context(), principal)))
				return
			}
			key, ok := bearerKey(authorization)
			if !ok {
				WriteError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
				return
			}
			if requiredType == domain.CredentialPublishable && !strings.HasPrefix(key, credential.PrefixPublishableLive) && !strings.HasPrefix(key, credential.PrefixPublishableTest) {
				WriteError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
				return
			}
			if requiredType == domain.CredentialSecret && !strings.HasPrefix(key, credential.PrefixSecretLive) && !strings.HasPrefix(key, credential.PrefixSecretTest) {
				WriteError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
				return
			}
			if router.credentials == nil {
				WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
				return
			}
			applicationCredential, err := router.credentials.Verify(request.Context(), principal.ApplicationID, key)
			if err != nil {
				router.writeCredentialVerificationError(writer, request, err)
				return
			}
			if applicationCredential == nil {
				WriteError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
				return
			}
			if applicationCredential.CredentialType != requiredType {
				WriteError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
				return
			}
			granted := credentialScopes(applicationCredential.Scopes)
			for _, required := range requiredScopes {
				if _, ok := granted[required]; !ok {
					WriteError(writer, request, http.StatusForbidden, "INSUFFICIENT_SCOPE", "insufficient scope")
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
		WriteError(writer, request, http.StatusBadRequest, "INVALID_APPLICATION", "invalid application")
		return AppPrincipal{}, false
	}
	application, err := router.applications.FindApplicationByID(request.Context(), applicationID)
	if errors.Is(err, domain.ErrApplicationNotFound) {
		WriteError(writer, request, http.StatusNotFound, "INVALID_APPLICATION", "invalid application")
		return AppPrincipal{}, false
	}
	if err != nil {
		WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return AppPrincipal{}, false
	}
	if application == nil {
		WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return AppPrincipal{}, false
	}
	switch application.Status {
	case domain.ApplicationStatusSuspended, domain.ApplicationStatusDisabled:
		WriteError(writer, request, http.StatusForbidden, "APPLICATION_DISABLED", "application disabled")
		return AppPrincipal{}, false
	case domain.ApplicationStatusMaintenance:
		WriteError(writer, request, http.StatusServiceUnavailable, "APPLICATION_MAINTENANCE", "application maintenance")
		return AppPrincipal{}, false
	case domain.ApplicationStatusActive:
	default:
		WriteError(writer, request, http.StatusForbidden, "APPLICATION_DISABLED", "application disabled")
		return AppPrincipal{}, false
	}
	return AppPrincipal{
		ApplicationID:  application.ID,
		OrganizationID: application.OrganizationID,
	}, true
}

// ResolveApplication validates the application selected by X-KeyStar-App and
// returns its principal without requiring an application credential. Admin
// endpoints use this after administrator authentication so their data access
// has the same tenant boundary as the public and server APIs.
func (router *Router) ResolveApplication(writer http.ResponseWriter, request *http.Request) (AppPrincipal, bool) {
	return router.resolveApplicationPrincipal(writer, request)
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
		WriteError(writer, request, http.StatusUnauthorized, "CREDENTIAL_REVOKED", "credential revoked")
	case errors.Is(err, domain.ErrCredentialExpired):
		WriteError(writer, request, http.StatusUnauthorized, "CREDENTIAL_EXPIRED", "credential expired")
	case errors.Is(err, domain.ErrInvalidCredential):
		WriteError(writer, request, http.StatusUnauthorized, "INVALID_CREDENTIAL", "invalid credential")
	default:
		WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
	}
}
