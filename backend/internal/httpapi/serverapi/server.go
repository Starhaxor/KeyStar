// Package serverapi implements the machine-to-machine /v1/server namespace:
// secret-key authentication with per-endpoint scopes. Handlers mount onto the
// core httpapi.Router via New.
package serverapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

// Router wraps the core router so namespace handlers keep method receivers
// while reading only the exported surface of httpapi.Router.
type Router struct {
	*httpapi.Router
}

// New builds the /v1/server handler and returns it ready to mount with
// httpapi.Router.MountServer.
func New(core *httpapi.Router) http.Handler {
	api := &Router{Router: core}
	return http.HandlerFunc(api.serveServer)
}

type serverUserJSON struct {
	ID           string  `json:"id"`
	Email        string  `json:"email"`
	Status       string  `json:"status"`
	Notes        string  `json:"notes"`
	BanReason    string  `json:"ban_reason"`
	BannedAt     *string `json:"banned_at"`
	BanExpiresAt *string `json:"ban_expires_at"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type serverLicenseJSON struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	UserEmail  string `json:"user_email"`
	Product    string `json:"product"`
	Status     string `json:"status"`
	Level      int    `json:"level"`
	MaxDevices int    `json:"max_devices"`
	Notes      string `json:"notes"`
	ExpiresAt  string `json:"expires_at"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func mapServerUser(user domain.ServerUser) serverUserJSON {
	return serverUserJSON{
		ID: user.ID, Email: user.Email, Status: string(user.Status),
		Notes: user.Notes, BanReason: user.BanReason,
		BannedAt: httpapi.FormatOptionalTime(user.BannedAt), BanExpiresAt: httpapi.FormatOptionalTime(user.BanExpiresAt),
		CreatedAt: httpapi.FormatTime(user.CreatedAt), UpdatedAt: httpapi.FormatTime(user.UpdatedAt),
	}
}

func mapServerUsers(users []domain.ServerUser) []serverUserJSON {
	result := make([]serverUserJSON, 0, len(users))
	for _, user := range users {
		result = append(result, mapServerUser(user))
	}
	return result
}

func mapServerLicense(license domain.ServerLicense) serverLicenseJSON {
	return serverLicenseJSON{
		ID: license.ID, UserID: license.UserID, UserEmail: license.UserEmail,
		Product: license.Product, Status: string(license.Status), Level: license.Level,
		MaxDevices: license.MaxDevices, Notes: license.Notes,
		ExpiresAt: httpapi.FormatTime(license.ExpiresAt), CreatedAt: httpapi.FormatTime(license.CreatedAt),
		UpdatedAt: httpapi.FormatTime(license.UpdatedAt),
	}
}

func mapServerLicenses(licenses []domain.ServerLicense) []serverLicenseJSON {
	result := make([]serverLicenseJSON, 0, len(licenses))
	for _, license := range licenses {
		result = append(result, mapServerLicense(license))
	}
	return result
}

type serverPage struct {
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}

func parseServerPagination(request *http.Request) (limit int, after string) {
	limit = 50
	if configured, err := strconv.Atoi(strings.TrimSpace(request.URL.Query().Get("limit"))); err == nil && configured > 0 {
		limit = configured
	}
	if limit > 200 {
		limit = 200
	}
	after = strings.TrimSpace(request.URL.Query().Get("after"))
	return limit, after
}

// serveServer dispatches the /v1/server namespace. Every route demands a
// secret credential with the exact scope of the operation.
func (router *Router) serveServer(writer http.ResponseWriter, request *http.Request) {
	if router.ServerStore == nil || len(router.Server.LicenseHMACKey) == 0 {
		httpapi.WriteError(writer, request, http.StatusServiceUnavailable, "SERVER_ERROR", "server api unavailable")
		return
	}
	segments := splitServerPath(strings.TrimPrefix(request.URL.Path, httpapi.ServerPathPrefix))
	switch {
	case len(segments) == 1 && segments[0] == "users" && request.Method == http.MethodGet:
		router.RequireServerCredential(domain.CredentialSecret, "users.read")(http.HandlerFunc(router.handleServerUserList)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "users" && request.Method == http.MethodPost:
		router.RequireServerCredential(domain.CredentialSecret, "users.write")(http.HandlerFunc(router.handleServerUserCreate)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "users" && request.Method == http.MethodGet:
		router.RequireServerCredential(domain.CredentialSecret, "users.read")(http.HandlerFunc(router.handleServerUserDetail)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "users" && request.Method == http.MethodPatch:
		router.RequireServerCredential(domain.CredentialSecret, "users.write")(http.HandlerFunc(router.handleServerUserUpdate)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "users" && request.Method == http.MethodDelete:
		router.RequireServerCredential(domain.CredentialSecret, "users.write")(http.HandlerFunc(router.handleServerUserDelete)).ServeHTTP(writer, request)
	case len(segments) == 3 && segments[0] == "users" && segments[2] == "ban" && request.Method == http.MethodPost:
		router.RequireServerCredential(domain.CredentialSecret, "users.write")(http.HandlerFunc(router.handleServerUserBan)).ServeHTTP(writer, request)
	case len(segments) == 3 && segments[0] == "users" && segments[2] == "unban" && request.Method == http.MethodPost:
		router.RequireServerCredential(domain.CredentialSecret, "users.write")(http.HandlerFunc(router.handleServerUserUnban)).ServeHTTP(writer, request)
	case len(segments) == 3 && segments[0] == "users" && segments[2] == "reset-devices" && request.Method == http.MethodPost:
		router.RequireServerCredential(domain.CredentialSecret, "devices.write")(http.HandlerFunc(router.handleServerUserResetDevices)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "licenses" && request.Method == http.MethodGet:
		router.RequireServerCredential(domain.CredentialSecret, "licenses.read")(http.HandlerFunc(router.handleServerLicenseList)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "licenses" && request.Method == http.MethodPost:
		router.RequireServerCredential(domain.CredentialSecret, "licenses.write")(http.HandlerFunc(router.handleServerLicenseCreate)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "licenses" && request.Method == http.MethodGet:
		router.RequireServerCredential(domain.CredentialSecret, "licenses.read")(http.HandlerFunc(router.handleServerLicenseDetail)).ServeHTTP(writer, request)
	case len(segments) == 3 && segments[0] == "licenses" && segments[2] == "revoke" && request.Method == http.MethodPost:
		router.RequireServerCredential(domain.CredentialSecret, "licenses.write")(http.HandlerFunc(router.handleServerLicenseRevoke)).ServeHTTP(writer, request)
	case len(segments) == 3 && segments[0] == "licenses" && segments[2] == "extend" && request.Method == http.MethodPost:
		router.RequireServerCredential(domain.CredentialSecret, "licenses.write")(http.HandlerFunc(router.handleServerLicenseExtend)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "variables" && request.Method == http.MethodGet:
		router.RequireServerCredential(domain.CredentialSecret, "variables.read")(http.HandlerFunc(router.handleServerVariableList)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "variables" && request.Method == http.MethodPost:
		router.RequireServerCredential(domain.CredentialSecret, "variables.write")(http.HandlerFunc(router.handleServerVariableCreate)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "variables" && request.Method == http.MethodPatch:
		router.RequireServerCredential(domain.CredentialSecret, "variables.write")(http.HandlerFunc(router.handleServerVariableUpdate)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "variables" && request.Method == http.MethodDelete:
		router.RequireServerCredential(domain.CredentialSecret, "variables.write")(http.HandlerFunc(router.handleServerVariableDelete)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "device-policy" && request.Method == http.MethodGet:
		router.RequireServerCredential(domain.CredentialSecret, "devices.read")(http.HandlerFunc(router.handleServerDevicePolicyGet)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "device-policy" && (request.Method == http.MethodPut || request.Method == http.MethodPatch):
		router.RequireServerCredential(domain.CredentialSecret, "devices.write")(http.HandlerFunc(router.handleServerDevicePolicyUpdate)).ServeHTTP(writer, request)
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func splitServerPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// serverPathID returns the second path segment (the resource ID).
func serverPathID(request *http.Request) string {
	segments := splitServerPath(strings.TrimPrefix(request.URL.Path, httpapi.ServerPathPrefix))
	if len(segments) < 2 {
		return ""
	}
	return segments[1]
}

// principalApplicationID resolves the application boundary installed by the
// credential middleware.
func principalApplicationID(request *http.Request) string {
	if principal, ok := httpapi.AppPrincipalFromContext(request.Context()); ok && principal.ApplicationID != "" {
		return principal.ApplicationID
	}
	return ""
}

func (router *Router) writeServerError(writer http.ResponseWriter, request *http.Request, err error) {
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
	case errors.Is(err, domain.ErrVariableNotFound):
		httpapi.WriteError(writer, request, http.StatusNotFound, "VARIABLE_NOT_FOUND", "variable not found")
	case errors.Is(err, domain.ErrVariableAlreadyExists):
		httpapi.WriteError(writer, request, http.StatusConflict, "VARIABLE_ALREADY_EXISTS", "a variable with this key already exists")
	default:
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
	}
}
