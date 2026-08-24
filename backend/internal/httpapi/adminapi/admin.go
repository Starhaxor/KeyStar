// Package adminapi implements the /v1/admin dashboard namespace: session
// cookies, CSRF, RBAC permissions, MFA and the console handlers. It mounts
// onto the core httpapi.Router via New.
package adminapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type adminApplicationContextKey struct{}

func withAdminApplicationID(ctx context.Context, applicationID string) context.Context {
	return context.WithValue(ctx, adminApplicationContextKey{}, applicationID)
}

// AdminApplicationID returns the validated application selected for this
// dashboard request, falling back to the configured legacy application.
func (router *Router) AdminApplicationID(request *http.Request) string {
	if applicationID, ok := request.Context().Value(adminApplicationContextKey{}).(string); ok && applicationID != "" {
		return applicationID
	}
	return router.DefaultApplicationID()
}

const defaultMFAIssuer = "KeyStar Admin"

// Router wraps the core router so namespace handlers keep method receivers
// while reading only the exported surface of httpapi.Router.
type Router struct {
	*httpapi.Router
}

// New builds the /v1/admin namespace handler and returns it ready to mount
// with httpapi.Router.MountAdmin.
func New(core *httpapi.Router) http.Handler {
	api := &Router{Router: core}
	return http.HandlerFunc(api.serveAdmin)
}

func (router *Router) AdminEnabled() bool {
	return router.Admin.Auth != nil && router.Admin.Console != nil
}

func (router *Router) AdminMFAIssuer() string {
	if strings.TrimSpace(router.Admin.MFAIssuer) != "" {
		return router.Admin.MFAIssuer
	}
	return defaultMFAIssuer
}

func (router *Router) serveAdmin(writer http.ResponseWriter, request *http.Request) {
	origin := request.Header.Get("Origin")
	originAllowed := origin != "" && slices.Contains(router.Admin.AllowedOrigins, origin)
	if request.Method == http.MethodOptions {
		if originAllowed && request.Header.Get("Access-Control-Request-Method") != "" {
			header := writer.Header()
			header.Set("Access-Control-Allow-Origin", origin)
			header.Set("Access-Control-Allow-Credentials", "true")
			header.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			header.Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")
			header.Set("Access-Control-Max-Age", "600")
			header.Add("Vary", "Origin")
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		httpapi.WriteError(writer, request, http.StatusMethodNotAllowed, "INVALID_REQUEST", "method not allowed")
		return
	}
	if originAllowed {
		header := writer.Header()
		header.Set("Access-Control-Allow-Origin", origin)
		header.Set("Access-Control-Allow-Credentials", "true")
		header.Add("Vary", "Origin")
	}
	// Checked after the CORS headers so a disabled console still produces a
	// readable error in the browser instead of an opaque network failure.
	if !router.AdminEnabled() {
		httpapi.WriteError(writer, request, http.StatusServiceUnavailable, "SERVER_ERROR", "admin console unavailable")
		return
	}

	path := strings.TrimPrefix(request.URL.Path, httpapi.AdminPathPrefix)
	if path == "/auth/login" || path == "/auth/mfa" {
		if request.Method != http.MethodPost {
			httpapi.WriteError(writer, request, http.StatusMethodNotAllowed, "INVALID_REQUEST", "method not allowed")
			return
		}
		if path == "/auth/login" {
			router.handleAdminLogin(writer, request)
		} else {
			router.handleAdminMFA(writer, request)
		}
		return
	}

	session, account, token, ok := router.AuthenticateAdmin(writer, request)
	if !ok {
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		if !router.VerifyAdminCSRF(request, token) {
			router.RecordSecurityEvent(request, account, "ADMIN_CSRF_REJECTED", "warning", map[string]string{"path": request.URL.Path})
			httpapi.WriteError(writer, request, http.StatusForbidden, "CSRF_REJECTED", "csrf token rejected")
			return
		}
	}
	if !account.MFAEnrolled && !adminEnrollmentExempt(path, request.Method) {
		httpapi.WriteError(writer, request, http.StatusForbidden, "MFA_ENROLLMENT_REQUIRED", "multi-factor authentication enrollment is required")
		return
	}
	// Application lifecycle recovery cannot depend on the operational state of
	// the default application: otherwise disabling that application would lock
	// operators out of listing or restoring it. The exception remains limited
	// to the platform-scoped identity, list and named transition routes; every
	// application-scoped admin operation still resolves an active tenant.
	if isApplicationLifecycleRecoveryRoute(path, request.Method) {
		router.routeAdmin(writer, request, session, account, path)
		return
	}
	principal, applicationOK := router.ResolveApplication(writer, request)
	if !applicationOK {
		return
	}
	request = request.WithContext(withAdminApplicationID(request.Context(), principal.ApplicationID))
	router.routeAdmin(writer, request, session, account, path)
}

func isApplicationLifecycleRecoveryRoute(path, method string) bool {
	segments := splitAdminPath(path)
	return (len(segments) == 1 && segments[0] == "me" && method == http.MethodGet) ||
		(len(segments) == 1 && segments[0] == "applications" && method == http.MethodGet) ||
		(len(segments) == 2 && segments[0] == "applications" && segments[1] == "organizations" && method == http.MethodGet) ||
		(len(segments) == 3 && segments[0] == "applications" && segments[2] == "transition" && method == http.MethodPost)
}

// adminEnrollmentExempt lists the routes an unenrolled administrator may
// still reach: identity endpoints plus the enrollment flow itself.
func adminEnrollmentExempt(path string, method string) bool {
	switch {
	case path == "/auth/logout" && method == http.MethodPost:
	case path == "/me" && method == http.MethodGet:
	case path == "/mfa/enroll/start" && method == http.MethodPost:
	case path == "/mfa/enroll/confirm" && method == http.MethodPost:
	default:
		return false
	}
	return true
}

func (router *Router) routeAdmin(writer http.ResponseWriter, request *http.Request, session *domain.AdminSession, account *domain.AdminAccount, path string) {
	segments := splitAdminPath(path)
	switch {
	case len(segments) == 2 && segments[0] == "auth" && segments[1] == "logout":
		if request.Method != http.MethodPost {
			httpapi.WriteError(writer, request, http.StatusMethodNotAllowed, "INVALID_REQUEST", "method not allowed")
			return
		}
		router.handleAdminLogout(writer, request, session, token(request))
	case len(segments) == 1 && segments[0] == "me" && request.Method == http.MethodGet:
		router.handleAdminMe(writer, request, account)
	case len(segments) == 2 && segments[0] == "me" && segments[1] == "activity" && request.Method == http.MethodGet:
		router.handleAdminActivity(writer, request, account)
	case len(segments) == 3 && segments[0] == "mfa" && segments[1] == "enroll" && segments[2] == "start" && request.Method == http.MethodPost:
		router.handleAdminMFAEnrollStart(writer, request, account)
	case len(segments) == 3 && segments[0] == "mfa" && segments[1] == "enroll" && segments[2] == "confirm" && request.Method == http.MethodPost:
		router.handleAdminMFAEnrollConfirm(writer, request, account)
	case len(segments) == 2 && segments[0] == "mfa" && segments[1] == "disable" && request.Method == http.MethodPost:
		router.handleAdminMFADisable(writer, request, account)
	case len(segments) == 2 && segments[0] == "overview" && segments[1] == "stats" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermOverviewRead) {
			return
		}
		router.handleAdminOverviewStats(writer, request)
	case len(segments) == 2 && segments[0] == "overview" && segments[1] == "today" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermOverviewRead) {
			return
		}
		router.handleAdminOverviewToday(writer, request)
	case len(segments) == 1 && segments[0] == "overview" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermOverviewRead) {
			return
		}
		router.handleAdminOverview(writer, request, account)
	case len(segments) == 2 && segments[0] == "onboarding" && segments[1] == "progress" && request.Method == http.MethodGet:
		for _, permission := range []string{domain.PermApplicationsRead, domain.PermCredentialsRead, domain.PermCatalogRead, domain.PermLicensesRead} {
			if !router.RequirePermission(writer, request, account, permission) {
				return
			}
		}
		router.handleAdminOnboardingProgress(writer, request)
	case len(segments) >= 1 && segments[0] == "users":
		router.routeAdminUsers(writer, request, account, segments)
	case len(segments) == 1 && segments[0] == "bans" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermUsersRead) {
			return
		}
		router.handleAdminBanList(writer, request)
	case len(segments) >= 1 && segments[0] == "licenses":
		router.routeAdminLicenses(writer, request, account, segments)
	case len(segments) >= 1 && segments[0] == "products":
		router.routeAdminProducts(writer, request, account, segments)
	case len(segments) >= 1 && segments[0] == "webhooks":
		router.routeAdminWebhooks(writer, request, account, segments)
	case len(segments) >= 1 && segments[0] == "applications":
		router.routeAdminApplications(writer, request, account, segments)
	case len(segments) >= 1 && segments[0] == "devices":
		router.routeAdminDevices(writer, request, account, segments)
	case len(segments) >= 1 && segments[0] == "device-bans":
		router.routeAdminDeviceBans(writer, request, account, segments)
	case len(segments) >= 1 && segments[0] == "sessions":
		router.routeAdminSessions(writer, request, account, segments)
	case len(segments) == 1 && segments[0] == "audit-logs" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermAuditRead) {
			return
		}
		router.handleAdminAuditLogs(writer, request)
	case len(segments) == 1 && segments[0] == "security-events" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermSecurityRead) {
			return
		}
		router.handleAdminSecurityEvents(writer, request)
	case len(segments) >= 1 && segments[0] == "variables":
		router.routeAdminVariables(writer, request, account, segments)
	case len(segments) == 1 && segments[0] == "roles" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsRead) {
			return
		}
		router.handleAdminRoles(writer, request)
	case len(segments) == 1 && segments[0] == "roles" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsWrite) {
			return
		}
		router.handleAdminRoleCreate(writer, request, account)
	case len(segments) == 2 && segments[0] == "roles" && request.Method == http.MethodPatch:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsWrite) {
			return
		}
		router.handleAdminRoleUpdate(writer, request, account, segments[1])
	case len(segments) == 3 && segments[0] == "roles" && segments[2] == "members" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsRead) {
			return
		}
		router.handleAdminRoleMembers(writer, request, segments[1])
	case len(segments) == 2 && segments[0] == "roles" && request.Method == http.MethodDelete:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsWrite) {
			return
		}
		router.handleAdminRoleDelete(writer, request, account, segments[1])
	case len(segments) >= 1 && segments[0] == "admins":
		router.routeAdminAccounts(writer, request, account, segments)
	case len(segments) == 1 && segments[0] == "credentials" && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermCredentialsRead) {
			return
		}
		router.handleAdminCredentialList(writer, request)
	case len(segments) == 1 && segments[0] == "credentials" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermCredentialsWrite) {
			return
		}
		router.handleAdminCredentialCreate(writer, request, account)
	case len(segments) == 3 && segments[0] == "credentials" && segments[2] == "revoke" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermCredentialsWrite) {
			return
		}
		router.handleAdminCredentialRevoke(writer, request, account, segments[1])
	case len(segments) == 3 && segments[0] == "credentials" && segments[2] == "rotate" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermCredentialsWrite) {
			return
		}
		router.handleAdminCredentialRotate(writer, request, account, segments[1])
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

// requirePermission enforces RBAC data-driven: the check only consults the
// account's permission set, never role names.
func (router *Router) RequirePermission(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, permission string) bool {
	if account.HasPermission(permission) {
		return true
	}
	router.RecordSecurityEvent(request, account, "ADMIN_PERMISSION_DENIED", "warning", map[string]string{
		"permission": permission,
		"path":       request.URL.Path,
	})
	httpapi.WriteError(writer, request, http.StatusForbidden, "PERMISSION_DENIED", "permission denied")
	return false
}

func token(request *http.Request) string {
	cookie, err := request.Cookie(httpapi.AdminSessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func splitAdminPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// authenticateAdmin resolves the session cookie to an active admin session.
func (router *Router) AuthenticateAdmin(writer http.ResponseWriter, request *http.Request) (*domain.AdminSession, *domain.AdminAccount, string, bool) {
	cookie, err := request.Cookie(httpapi.AdminSessionCookieName)
	if err != nil || cookie.Value == "" {
		httpapi.WriteError(writer, request, http.StatusUnauthorized, "ADMIN_UNAUTHENTICATED", "authentication required")
		return nil, nil, "", false
	}
	session, account, err := router.Admin.Auth.Authenticate(request.Context(), cookie.Value)
	if err != nil {
		router.ClearAdminCookies(writer)
		httpapi.WriteError(writer, request, http.StatusUnauthorized, "ADMIN_UNAUTHENTICATED", "authentication required")
		return nil, nil, "", false
	}
	return session, account, cookie.Value, true
}

// adminCSRFToken derives a session-bound CSRF token; the double-submit cookie
// value and the expected header value share this derivation.
func (router *Router) AdminCSRFToken(sessionToken string) string {
	mac := hmac.New(sha256.New, router.Admin.CSRFSecret)
	mac.Write([]byte("admin-csrf|" + sessionToken))
	return hex.EncodeToString(mac.Sum(nil))
}

func (router *Router) VerifyAdminCSRF(request *http.Request, sessionToken string) bool {
	expected := router.AdminCSRFToken(sessionToken)
	provided := request.Header.Get(httpapi.AdminCSRFHeader)
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// auditAdmin appends one audit record for a console action. Audit failures
// never break the primary operation because the action already happened.
func (router *Router) AuditAdmin(request *http.Request, account *domain.AdminAccount, action, resourceType, resourceID string, metadata any) {
	entry := domain.NewAuditLog{
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IPSHA256:     hashClientIP(request, router.TrustedProxies()),
		UserAgent:    truncateAdminUserAgent(request.UserAgent()),
	}
	if account != nil {
		entry.AdminAccountID = account.ID
		entry.ActorEmail = account.Email
	}
	if metadata != nil {
		if raw, err := jsonMarshal(metadata); err == nil {
			entry.Metadata = raw
		}
	}
	_ = router.Admin.Console.AppendAuditLog(request.Context(), entry)
}

// recordSecurityEvent appends one security event; failures are swallowed so
// anomaly tracking can never break the primary operation.
func (router *Router) RecordSecurityEvent(request *http.Request, account *domain.AdminAccount, kind, severity string, metadata any) {
	entry := domain.NewSecurityEvent{
		Kind:      kind,
		Severity:  severity,
		IPSHA256:  hashClientIP(request, router.TrustedProxies()),
		UserAgent: truncateAdminUserAgent(request.UserAgent()),
	}
	if account != nil {
		entry.AdminAccountID = account.ID
		entry.ActorEmail = account.Email
	}
	if metadata != nil {
		if raw, err := jsonMarshal(metadata); err == nil {
			entry.Metadata = raw
		}
	}
	_ = router.Admin.Console.AppendSecurityEvent(request.Context(), entry)
}

func (router *Router) WriteConsoleError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
	case errors.Is(err, domain.ErrUserAlreadyExists):
		httpapi.WriteError(writer, request, http.StatusConflict, "USER_ALREADY_EXISTS", "a user with this email already exists")
	case errors.Is(err, domain.ErrLicenseNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "LICENSE_NOT_FOUND", "license not found")
	case errors.Is(err, domain.ErrLicenseAlreadyExists):
		httpapi.WriteError(writer, request, http.StatusConflict, "LICENSE_ALREADY_EXISTS", "license already exists for user and product")
	case errors.Is(err, domain.ErrProductInvalidName):
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_PRODUCT", "product name cannot be normalized")
	case errors.Is(err, domain.ErrDeviceNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "DEVICE_NOT_FOUND", "device not found")
	case errors.Is(err, domain.ErrAuthSessionNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "SESSION_NOT_FOUND", "session not found")
	case errors.Is(err, domain.ErrAdminNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "ADMIN_NOT_FOUND", "admin not found")
	case errors.Is(err, domain.ErrAdminAlreadyExists):
		httpapi.WriteError(writer, request, http.StatusConflict, "ADMIN_ALREADY_EXISTS", "an admin account with this email already exists")
	case errors.Is(err, domain.ErrRoleNotFound):
		httpapi.WriteError(writer, request, http.StatusBadRequest, "ROLE_NOT_FOUND", "role not found")
	case errors.Is(err, domain.ErrRoleAlreadyExists):
		httpapi.WriteError(writer, request, http.StatusConflict, "ROLE_ALREADY_EXISTS", "a role with this name already exists")
	case errors.Is(err, domain.ErrBuiltInRole):
		httpapi.WriteError(writer, request, http.StatusForbidden, "BUILT_IN_ROLE", "built-in roles cannot be modified")
	case errors.Is(err, domain.ErrRoleInUse):
		httpapi.WriteError(writer, request, http.StatusConflict, "ROLE_IN_USE", "role is assigned to an admin account")
	case errors.Is(err, domain.ErrVariableNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "VARIABLE_NOT_FOUND", "variable not found")
	case errors.Is(err, domain.ErrVariableAlreadyExists):
		httpapi.WriteError(writer, request, http.StatusConflict, "VARIABLE_ALREADY_EXISTS", "a variable with this key already exists")
	default:
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
	}
}
