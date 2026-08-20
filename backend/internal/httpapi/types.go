// Package httpapi is the HTTP layer of the platform: router, middleware,
// response helpers and the public client API (login, device verification,
// /v1/me). The admin dashboard namespace lives in the adminapi subpackage and
// the machine-to-machine namespace in the serverapi subpackage; both mount
// their handlers onto the Router built here.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/service"
	"github.com/starloader/backend/internal/service/adminauth"
)

const (
	// AdminPathPrefix is the URL prefix of the admin dashboard namespace.
	AdminPathPrefix = "/v1/admin"
	// ServerPathPrefix is the URL prefix of the machine-to-machine namespace.
	ServerPathPrefix = "/v1/server"

	// MaxRequestBodyBytes caps every decoded JSON request body.
	MaxRequestBodyBytes = 64 * 1024
	// MinEndUserPasswordLength is the minimum password length for end users.
	MinEndUserPasswordLength = 10

	// AdminSessionCookieName is the dashboard session cookie.
	AdminSessionCookieName = "starloader_admin_session"
	// AdminCSRFCookieName is the double-submit CSRF cookie.
	AdminCSRFCookieName = "starloader_admin_csrf"
	// AdminCSRFHeader is the header carrying the CSRF token on mutations.
	AdminCSRFHeader = "X-CSRF-Token"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidUUID reports whether value has the canonical UUID shape.
func ValidUUID(value string) bool {
	return uuidPattern.MatchString(value)
}

// LoginService authenticates an end user and starts a device challenge.
type LoginService interface {
	Login(context.Context, service.LoginInput) (service.PendingChallenge, error)
}

// DeviceVerificationService verifies a signed device challenge.
type DeviceVerificationService interface {
	Verify(context.Context, service.VerifyInput) (service.VerifiedSession, error)
}

// ProfileRepository loads the profile of an authenticated session.
type ProfileRepository interface {
	LoadProfile(context.Context, string, string, string, string) (*domain.UserProfile, error)
}

// ApplicationResolver resolves an application by ID for request resolution.
type ApplicationResolver interface {
	FindApplicationByID(context.Context, string) (*domain.Application, error)
}

// CredentialVerifier validates a credential key against one application.
type CredentialVerifier interface {
	Verify(context.Context, string, string) (*domain.ApplicationCredential, error)
}

// AdminAuthService authenticates dashboard administrators and manages their
// TOTP enrollment.
type AdminAuthService interface {
	Login(ctx context.Context, email, password, ipAddress, userAgent string) (adminauth.LoginResult, error)
	CompleteMFA(ctx context.Context, challengeToken, code, recoveryCode, ipAddress, userAgent string) (string, *domain.AdminAccount, error)
	StartMFAEnrollment(ctx context.Context, account *domain.AdminAccount, issuer string) (string, string, error)
	ConfirmMFAEnrollment(ctx context.Context, account *domain.AdminAccount, code, ipAddress, userAgent string) ([]string, error)
	DisableMFA(ctx context.Context, account *domain.AdminAccount, password, ipAddress, userAgent string) error
	Authenticate(ctx context.Context, token string) (*domain.AdminSession, *domain.AdminAccount, error)
	Logout(ctx context.Context, token string) error
}

// AdminConsoleStore is the persistence boundary for dashboard management.
type AdminConsoleStore interface {
	ConsoleOverview(ctx context.Context) (*domain.ConsoleOverview, error)
	ConsoleTodayStats(ctx context.Context) (*domain.ConsoleTodayStats, error)
	ConsoleDailyStats(ctx context.Context, days int) ([]domain.DailyStat, error)
	ListConsoleUsers(ctx context.Context, offset, limit int, search string, status string) ([]domain.ConsoleUser, int64, error)
	ConsoleUserDetail(ctx context.Context, userID string) (*domain.ConsoleUserDetail, error)
	// End-user mutations are tenant-scoped; applicationID is the application
	// boundary of the operation. Dashboard list/read views remain global until
	// per-application console routing lands (platform phase 9).
	SetUserStatus(ctx context.Context, applicationID, userID string, status domain.UserStatus) error
	SetUserNotes(ctx context.Context, applicationID, userID, notes string) error
	BanUser(ctx context.Context, applicationID, userID, reason string, expiresAt *time.Time) error
	UnbanUser(ctx context.Context, applicationID, userID string) error
	AutoUnbanExpired(ctx context.Context, applicationID, userID string) error
	ListConsoleBans(ctx context.Context, offset, limit int, search, statusFilter string) ([]domain.BanRecord, int64, error)
	ResetUserDevices(ctx context.Context, applicationID, userID string) (int64, error)
	BulkSetUserStatus(ctx context.Context, applicationID string, userIDs []string, status domain.UserStatus) (int64, error)
	BulkRevokeUserSessions(ctx context.Context, applicationID string, userIDs []string) (int64, error)
	FindUserByEmail(ctx context.Context, applicationID, email string) (*domain.User, error)
	FindUserByID(ctx context.Context, applicationID, userID string) (*domain.User, error)
	SetUserPassword(ctx context.Context, applicationID, userID, passwordHash string) error
	CreateUser(ctx context.Context, applicationID string, input domain.NewUser) (*domain.User, error)
	RevokeUserSessions(ctx context.Context, applicationID, userID string) (int64, error)
	ListConsoleLicenses(ctx context.Context, offset, limit int) ([]domain.ConsoleLicense, int64, error)
	ResolveProductPlan(ctx context.Context, applicationID, name string) (string, string, error)
	CreateLicense(ctx context.Context, applicationID string, input domain.NewLicense) (*domain.License, error)
	FindLicenseByID(ctx context.Context, applicationID, licenseID string) (*domain.License, error)
	AdminUpdateLicense(ctx context.Context, applicationID, licenseID string, expiresAt time.Time, maxDevices, level int, notes string) error
	AdminRevokeLicense(ctx context.Context, applicationID, licenseID string) error
	ListVariables(ctx context.Context, applicationID string) ([]domain.Variable, error)
	CreateVariable(ctx context.Context, applicationID, key, value, description string) (*domain.Variable, error)
	UpdateVariable(ctx context.Context, applicationID, variableID, value, description string) error
	DeleteVariable(ctx context.Context, applicationID, variableID string) error
	ListConsoleDevices(ctx context.Context, offset, limit int) ([]domain.ConsoleDevice, int64, error)
	FindConsoleDeviceByID(ctx context.Context, deviceID string) (*domain.ConsoleDeviceDetail, error)
	AdminRevokeDevice(ctx context.Context, applicationID, deviceID string) error
	AdminResetDevice(ctx context.Context, applicationID, deviceID string) error
	ListConsoleSessions(ctx context.Context, offset, limit int) ([]domain.ConsoleSession, int64, error)
	AdminRevokeAuthSession(ctx context.Context, applicationID, sessionID string) error
	CreateCredential(ctx context.Context, input domain.NewApplicationCredential) (*domain.ApplicationCredential, error)
	ListCredentials(ctx context.Context, applicationID string) ([]domain.ApplicationCredential, error)
	RevokeCredential(ctx context.Context, applicationID, credentialID string) error
	ListAuditLogs(ctx context.Context, offset, limit int) ([]domain.AuditLog, int64, error)
	AppendAuditLog(ctx context.Context, input domain.NewAuditLog) error
	ListAdminAccounts(ctx context.Context) ([]domain.AdminAccount, error)
	GetDevicePolicy(ctx context.Context, applicationID string) (*domain.DevicePolicy, error)
	UpsertDevicePolicy(ctx context.Context, applicationID string, input domain.NewDevicePolicy) (*domain.DevicePolicy, error)
	DeleteDevicePolicy(ctx context.Context, applicationID string) error
	FindAdminAccountByID(ctx context.Context, adminID string) (*domain.AdminAccount, error)
	CreateAdminAccount(ctx context.Context, input domain.NewAdminAccount) (*domain.AdminAccount, error)
	UpdateAdminAccountStatusAndRole(ctx context.Context, adminID string, status domain.AdminAccountStatus, roleName string) error
	SetAdminPassword(ctx context.Context, adminID, passwordHash string) error
	RevokeAllAdminSessions(ctx context.Context, adminID string) error
	ListRoles(ctx context.Context) ([]domain.Role, error)
	ListRoleMembers(ctx context.Context, roleID string) ([]domain.RoleMember, error)
	CreateRole(ctx context.Context, input domain.NewRole) (*domain.Role, error)
	UpdateRole(ctx context.Context, roleID, description string, permissions []string) error
	DeleteRole(ctx context.Context, roleID string) error
	ListSecurityEvents(ctx context.Context, offset, limit int) ([]domain.SecurityEvent, int64, error)
	AppendSecurityEvent(ctx context.Context, input domain.NewSecurityEvent) error
}

// AdminConfig bundles the dependencies of the /v1/admin namespace. The
// namespace stays disabled unless both Auth and Console are provided.
type AdminConfig struct {
	Auth           AdminAuthService
	Console        AdminConsoleStore
	LicenseHMACKey []byte
	Product        string
	MFAIssuer      string
	AllowedOrigins []string
	CSRFSecret     []byte
	CookieSecure   bool
	SessionTTL     time.Duration
}

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
	ResolveProductPlan(ctx context.Context, applicationID, name string) (string, string, error)
	CreateLicense(ctx context.Context, applicationID string, input domain.NewLicense) (*domain.License, error)
	AdminUpdateLicense(ctx context.Context, applicationID, licenseID string, expiresAt time.Time, maxDevices, level int, notes string) error
	AdminRevokeLicense(ctx context.Context, applicationID, licenseID string) error
	ListVariables(ctx context.Context, applicationID string) ([]domain.Variable, error)
	CreateVariable(ctx context.Context, applicationID, key, value, description string) (*domain.Variable, error)
	UpdateVariable(ctx context.Context, applicationID, variableID, value, description string) error
	DeleteVariable(ctx context.Context, applicationID, variableID string) error
	GetDevicePolicy(ctx context.Context, applicationID string) (*domain.DevicePolicy, error)
	UpsertDevicePolicy(ctx context.Context, applicationID string, input domain.NewDevicePolicy) (*domain.DevicePolicy, error)
	ListRefreshSessions(ctx context.Context, applicationID, userID, after string, limit int) ([]domain.RefreshSession, string, bool, error)
	RevokeRefreshSession(ctx context.Context, sessionID string) error
	RevokeAllUserRefreshSessions(ctx context.Context, userID string) (int64, error)
	CreateWebhook(ctx context.Context, applicationID string, input domain.NewWebhook, secretHash []byte) (*domain.Webhook, error)
	ListWebhooks(ctx context.Context, applicationID string) ([]domain.Webhook, error)
	FindWebhookByID(ctx context.Context, applicationID, webhookID string) (*domain.Webhook, error)
	UpdateWebhook(ctx context.Context, applicationID, webhookID string, url *string, status *domain.WebhookStatus, events *[]string) error
	DeleteWebhook(ctx context.Context, applicationID, webhookID string) error
}

// ServerConfig carries the dependencies of the server API namespace.
type ServerConfig struct {
	LicenseHMACKey []byte
	Product        string
}

// DecodeJSONBody decodes one strict JSON object from the request body.
func DecodeJSONBody(writer http.ResponseWriter, request *http.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("invalid content type")
	}
	request.Body = http.MaxBytesReader(writer, request.Body, MaxRequestBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

// FormatTime renders a time in RFC3339 UTC.
func FormatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

// FormatOptionalTime renders a nullable time in RFC3339 UTC.
func FormatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

// VariableJSON is the safe response shape of a platform variable, shared by
// the admin and server namespaces.
type VariableJSON struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// MapVariable renders a domain variable for the API.
func MapVariable(variable domain.Variable) VariableJSON {
	return VariableJSON{
		ID: variable.ID, Key: variable.Key, Value: variable.Value,
		Description: variable.Description,
		CreatedAt:   FormatTime(variable.CreatedAt), UpdatedAt: FormatTime(variable.UpdatedAt),
	}
}

// RefreshService abstracts the refresh token management boundary used by the
// HTTP layer. It is satisfied by service.RefreshService.
type RefreshService interface {
	Refresh(ctx context.Context, input service.RefreshInput) (service.RefreshResult, error)
}

// refreshServiceAdapter wraps an optional RefreshService for the router.
type refreshServiceAdapter struct {
	service RefreshService
}

func wrapRefreshService(s RefreshService) *refreshServiceAdapter {
	if s == nil {
		return nil
	}
	return &refreshServiceAdapter{service: s}
}

func (a *refreshServiceAdapter) Refresh(ctx context.Context, input service.RefreshInput) (service.RefreshResult, error) {
	if a == nil || a.service == nil {
		return service.RefreshResult{}, errors.New("refresh service is not configured")
	}
	return a.service.Refresh(ctx, input)
}
