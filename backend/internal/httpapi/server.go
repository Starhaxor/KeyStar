package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/starloader/backend/internal/domain"
)

const serverPathPrefix = "/v1/server"

// ServerStore is the persistence boundary of the server-to-server API. It is
// satisfied by store.Store; every operation is application-scoped.
type ServerStore interface {
	ListServerUsers(ctx context.Context, applicationID, after string, limit int) ([]domain.ServerUser, string, bool, error)
	FindServerUserByID(ctx context.Context, applicationID, userID string) (*domain.ServerUser, error)
	FindUserByID(ctx context.Context, applicationID, userID string) (*domain.User, error)
	FindUserByEmail(ctx context.Context, applicationID, email string) (*domain.User, error)
	CreateUser(ctx context.Context, applicationID string, input domain.NewUser) (*domain.User, error)
	SetUserStatus(ctx context.Context, applicationID, userID string, status domain.UserStatus) error
	SetUserNotes(ctx context.Context, applicationID, userID, notes string) error
	BanUser(ctx context.Context, applicationID, userID, reason string, expiresAt *time.Time) error
	UnbanUser(ctx context.Context, applicationID, userID string) error
	ResetUserDevices(ctx context.Context, applicationID, userID string) (int64, error)
	ListServerLicenses(ctx context.Context, applicationID, after string, limit int) ([]domain.ServerLicense, string, bool, error)
	FindServerLicenseByID(ctx context.Context, applicationID, licenseID string) (*domain.ServerLicense, error)
	CreateLicense(ctx context.Context, applicationID string, input domain.NewLicense) (*domain.License, error)
	AdminUpdateLicense(ctx context.Context, applicationID, licenseID string, expiresAt time.Time, maxDevices, level int, notes string) error
	AdminRevokeLicense(ctx context.Context, applicationID, licenseID string) error
	ListVariables(ctx context.Context, applicationID string) ([]domain.Variable, error)
	CreateVariable(ctx context.Context, applicationID, key, value, description string) (*domain.Variable, error)
	UpdateVariable(ctx context.Context, applicationID, variableID, value, description string) error
	DeleteVariable(ctx context.Context, applicationID, variableID string) error
}

// ServerConfig carries the dependencies of the server API namespace.
type ServerConfig struct {
	LicenseHMACKey []byte
	Product        string
}

type serverUserJSON struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Status       string    `json:"status"`
	Notes        string    `json:"notes"`
	BanReason    string    `json:"ban_reason"`
	BannedAt     *string   `json:"banned_at"`
	BanExpiresAt *string   `json:"ban_expires_at"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
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
		BannedAt: formatOptionalTime(user.BannedAt), BanExpiresAt: formatOptionalTime(user.BanExpiresAt),
		CreatedAt: formatTime(user.CreatedAt), UpdatedAt: formatTime(user.UpdatedAt),
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
		ExpiresAt: formatTime(license.ExpiresAt), CreatedAt: formatTime(license.CreatedAt),
		UpdatedAt: formatTime(license.UpdatedAt),
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

func (router *Router) serveServer(writer http.ResponseWriter, request *http.Request) {
	if router.serverStore == nil || len(router.server.LicenseHMACKey) == 0 {
		writeError(writer, request, http.StatusServiceUnavailable, "SERVER_ERROR", "server api unavailable")
		return
	}
	segments := splitServerPath(strings.TrimPrefix(request.URL.Path, serverPathPrefix))
	switch {
	case len(segments) == 1 && segments[0] == "users" && request.Method == http.MethodGet:
		router.requireServerCredential(domain.CredentialSecret, "users.read")(http.HandlerFunc(router.handleServerUserList)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "users" && request.Method == http.MethodPost:
		router.requireServerCredential(domain.CredentialSecret, "users.write")(http.HandlerFunc(router.handleServerUserCreate)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "users" && request.Method == http.MethodGet:
		router.requireServerCredential(domain.CredentialSecret, "users.read")(http.HandlerFunc(router.handleServerUserDetail)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "users" && request.Method == http.MethodPatch:
		router.requireServerCredential(domain.CredentialSecret, "users.write")(http.HandlerFunc(router.handleServerUserUpdate)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "users" && request.Method == http.MethodDelete:
		router.requireServerCredential(domain.CredentialSecret, "users.write")(http.HandlerFunc(router.handleServerUserDelete)).ServeHTTP(writer, request)
	case len(segments) == 3 && segments[0] == "users" && segments[2] == "ban" && request.Method == http.MethodPost:
		router.requireServerCredential(domain.CredentialSecret, "users.write")(http.HandlerFunc(router.handleServerUserBan)).ServeHTTP(writer, request)
	case len(segments) == 3 && segments[0] == "users" && segments[2] == "unban" && request.Method == http.MethodPost:
		router.requireServerCredential(domain.CredentialSecret, "users.write")(http.HandlerFunc(router.handleServerUserUnban)).ServeHTTP(writer, request)
	case len(segments) == 3 && segments[0] == "users" && segments[2] == "reset-devices" && request.Method == http.MethodPost:
		router.requireServerCredential(domain.CredentialSecret, "devices.write")(http.HandlerFunc(router.handleServerUserResetDevices)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "licenses" && request.Method == http.MethodGet:
		router.requireServerCredential(domain.CredentialSecret, "licenses.read")(http.HandlerFunc(router.handleServerLicenseList)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "licenses" && request.Method == http.MethodPost:
		router.requireServerCredential(domain.CredentialSecret, "licenses.write")(http.HandlerFunc(router.handleServerLicenseCreate)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "licenses" && request.Method == http.MethodGet:
		router.requireServerCredential(domain.CredentialSecret, "licenses.read")(http.HandlerFunc(router.handleServerLicenseDetail)).ServeHTTP(writer, request)
	case len(segments) == 3 && segments[0] == "licenses" && segments[2] == "revoke" && request.Method == http.MethodPost:
		router.requireServerCredential(domain.CredentialSecret, "licenses.write")(http.HandlerFunc(router.handleServerLicenseRevoke)).ServeHTTP(writer, request)
	case len(segments) == 3 && segments[0] == "licenses" && segments[2] == "extend" && request.Method == http.MethodPost:
		router.requireServerCredential(domain.CredentialSecret, "licenses.write")(http.HandlerFunc(router.handleServerLicenseExtend)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "variables" && request.Method == http.MethodGet:
		router.requireServerCredential(domain.CredentialSecret, "variables.read")(http.HandlerFunc(router.handleServerVariableList)).ServeHTTP(writer, request)
	case len(segments) == 1 && segments[0] == "variables" && request.Method == http.MethodPost:
		router.requireServerCredential(domain.CredentialSecret, "variables.write")(http.HandlerFunc(router.handleServerVariableCreate)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "variables" && request.Method == http.MethodPatch:
		router.requireServerCredential(domain.CredentialSecret, "variables.write")(http.HandlerFunc(router.handleServerVariableUpdate)).ServeHTTP(writer, request)
	case len(segments) == 2 && segments[0] == "variables" && request.Method == http.MethodDelete:
		router.requireServerCredential(domain.CredentialSecret, "variables.write")(http.HandlerFunc(router.handleServerVariableDelete)).ServeHTTP(writer, request)
	default:
		writeError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func splitServerPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func (router *Router) writeServerError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		writeError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
	case errors.Is(err, domain.ErrUserAlreadyExists):
		writeError(writer, request, http.StatusConflict, "USER_ALREADY_EXISTS", "a user with this email already exists")
	case errors.Is(err, domain.ErrLicenseNotFound):
		writeError(writer, request, http.StatusNotFound, "LICENSE_NOT_FOUND", "license not found")
	case errors.Is(err, domain.ErrLicenseAlreadyExists):
		writeError(writer, request, http.StatusConflict, "LICENSE_ALREADY_EXISTS", "license already exists for user and product")
	case errors.Is(err, domain.ErrVariableNotFound):
		writeError(writer, request, http.StatusNotFound, "VARIABLE_NOT_FOUND", "variable not found")
	case errors.Is(err, domain.ErrVariableAlreadyExists):
		writeError(writer, request, http.StatusConflict, "VARIABLE_ALREADY_EXISTS", "a variable with this key already exists")
	default:
		writeError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
	}
}
