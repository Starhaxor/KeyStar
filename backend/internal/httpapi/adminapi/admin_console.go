package adminapi

import (
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/security"
)

const (
	defaultAdminPageSize   = 20
	maxAdminPageSize       = 100
	minAdminPasswordLength = 12
)

func parseAdminPagination(request *http.Request) (page, pageSize, offset int) {
	page = atoiOrDefault(request.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}
	pageSize = atoiOrDefault(request.URL.Query().Get("page_size"), defaultAdminPageSize)
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > maxAdminPageSize {
		pageSize = maxAdminPageSize
	}
	return page, pageSize, (page - 1) * pageSize
}

func atoiOrDefault(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

type adminPageResponse struct {
	OK       bool  `json:"ok"`
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func formatTimePtr(value *time.Time) string {
	if value == nil {
		return ""
	}
	return httpapi.FormatTime(*value)
}

func (router *Router) handleAdminOverview(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	overview, err := router.Admin.Console.ConsoleOverview(request.Context())
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK             bool             `json:"ok"`
		TotalUsers     int64            `json:"total_users"`
		ActiveLicenses int64            `json:"active_licenses"`
		ActiveDevices  int64            `json:"active_devices"`
		ActiveSessions int64            `json:"active_sessions"`
		RecentAudit    []auditEntryJSON `json:"recent_audit"`
	}{
		OK:             true,
		TotalUsers:     overview.TotalUsers,
		ActiveLicenses: overview.ActiveLicenses,
		ActiveDevices:  overview.ActiveDevices,
		ActiveSessions: overview.ActiveSessions,
		RecentAudit:    mapAuditEntries(overview.RecentAudit),
	})
}

type dailyStatJSON struct {
	Day               string `json:"day"`
	LicensesCreated   int64  `json:"licenses_created"`
	DevicesRegistered int64  `json:"devices_registered"`
	SessionsCreated   int64  `json:"sessions_created"`
	AuditEvents       int64  `json:"audit_events"`
	AdminLogins       int64  `json:"admin_logins"`
}

func mapDailyStats(stats []domain.DailyStat) []dailyStatJSON {
	items := make([]dailyStatJSON, 0, len(stats))
	for _, stat := range stats {
		items = append(items, dailyStatJSON{
			Day:               stat.Day,
			LicensesCreated:   stat.LicensesCreated,
			DevicesRegistered: stat.DevicesRegistered,
			SessionsCreated:   stat.SessionsCreated,
			AuditEvents:       stat.AuditEvents,
			AdminLogins:       stat.AdminLogins,
		})
	}
	return items
}

// handleAdminOverviewStats returns the trailing 14-day activity series for
// the dashboard charts.
func (router *Router) handleAdminOverviewStats(writer http.ResponseWriter, request *http.Request) {
	days := atoiOrDefault(request.URL.Query().Get("days"), 14)
	stats, err := router.Admin.Console.ConsoleDailyStats(request.Context(), days)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK   bool            `json:"ok"`
		Days []dailyStatJSON `json:"days"`
	}{
		OK:   true,
		Days: mapDailyStats(stats),
	})
}

// handleAdminOverviewToday returns the operations-center snapshot: counters
// since UTC midnight plus the totals an operator should watch.
func (router *Router) handleAdminOverviewToday(writer http.ResponseWriter, request *http.Request) {
	stats, err := router.Admin.Console.ConsoleTodayStats(request.Context())
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK                    bool  `json:"ok"`
		LoginsToday           int64 `json:"logins_today"`
		ActivationsToday      int64 `json:"activations_today"`
		NewDevicesToday       int64 `json:"new_devices_today"`
		AdminLoginsToday      int64 `json:"admin_logins_today"`
		FailedLoginsToday     int64 `json:"failed_logins_today"`
		PermissionDeniedToday int64 `json:"permission_denied_today"`
		BannedUsers           int64 `json:"banned_users"`
		ExpiredLicenses       int64 `json:"expired_licenses"`
	}{
		OK:                    true,
		LoginsToday:           stats.LoginsToday,
		ActivationsToday:      stats.ActivationsToday,
		NewDevicesToday:       stats.NewDevicesToday,
		AdminLoginsToday:      stats.AdminLoginsToday,
		FailedLoginsToday:     stats.FailedLoginsToday,
		PermissionDeniedToday: stats.PermissionDeniedToday,
		BannedUsers:           stats.BannedUsers,
		ExpiredLicenses:       stats.ExpiredLicenses,
	})
}

// Users

type consoleUserJSON struct {
	ID                 string  `json:"id"`
	Email              string  `json:"email"`
	Status             string  `json:"status"`
	LicenseCount       int     `json:"license_count"`
	DeviceCount        int     `json:"device_count"`
	ActiveSessionCount int     `json:"active_session_count"`
	LastLoginAt        *string `json:"last_login_at"`
	CreatedAt          string  `json:"created_at"`
}

func mapConsoleUser(user domain.ConsoleUser) consoleUserJSON {
	return consoleUserJSON{
		ID:                 user.ID,
		Email:              user.Email,
		Status:             string(user.Status),
		LicenseCount:       user.LicenseCount,
		DeviceCount:        user.DeviceCount,
		ActiveSessionCount: user.ActiveSessionCount,
		LastLoginAt:        httpapi.FormatOptionalTime(user.LastLoginAt),
		CreatedAt:          httpapi.FormatTime(user.CreatedAt),
	}
}

func (router *Router) routeAdminUsers(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermUsersRead) {
			return
		}
		router.handleAdminUserList(writer, request)
	case len(segments) == 1 && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermUsersWrite) {
			return
		}
		router.handleAdminUserCreate(writer, request, account)
	case len(segments) == 2 && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermUsersRead) {
			return
		}
		router.handleAdminUserDetail(writer, request, segments[1])
	case len(segments) == 2 && request.Method == http.MethodPatch:
		if !router.RequirePermission(writer, request, account, domain.PermUsersWrite) {
			return
		}
		router.handleAdminUserStatus(writer, request, account, segments[1])
	case len(segments) == 4 && segments[2] == "sessions" && segments[3] == "revoke" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermSessionsWrite) {
			return
		}
		router.handleAdminUserSessionsRevoke(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "promote" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsWrite) {
			return
		}
		router.handleAdminUserPromote(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "ban" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermUsersWrite) {
			return
		}
		router.handleAdminUserBan(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "unban" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermUsersWrite) {
			return
		}
		router.handleAdminUserUnban(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "notes" && request.Method == http.MethodPatch:
		if !router.RequirePermission(writer, request, account, domain.PermUsersWrite) {
			return
		}
		router.handleAdminUserNotes(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "reset-devices" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermUsersWrite) {
			return
		}
		router.handleAdminUserResetDevices(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "password" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermUsersWrite) {
			return
		}
		router.handleAdminUserPasswordReset(writer, request, account, segments[1])
	case len(segments) == 2 && segments[1] == "bulk-status" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermUsersWrite) {
			return
		}
		router.handleAdminUserBulkStatus(writer, request, account)
	case len(segments) == 3 && segments[1] == "bulk" && segments[2] == "revoke-sessions" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermSessionsWrite) {
			return
		}
		router.handleAdminUserBulkRevoke(writer, request, account)
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

// handleAdminUserCreate provisions an end-user account. The password is
// hashed with Argon2id; only the hash is persisted.
func (router *Router) handleAdminUserCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" || !strings.Contains(email, "@") || len(email) > 254 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "a valid email is required")
		return
	}
	if len(body.Password) < httpapi.MinEndUserPasswordLength {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "password must be at least 10 characters")
		return
	}
	hash, err := security.HashPassword(body.Password)
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	user, err := router.Admin.Console.CreateUser(request.Context(), router.AdminApplicationID(request), domain.NewUser{Email: email, PasswordHash: hash})
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "USER_CREATED", "user", user.ID, map[string]string{"email": user.Email})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK   bool            `json:"ok"`
		User consoleUserJSON `json:"user"`
	}{
		OK: true,
		User: consoleUserJSON{
			ID: user.ID, Email: user.Email, Status: string(user.Status), CreatedAt: httpapi.FormatTime(user.CreatedAt),
		},
	})
}

func (router *Router) handleAdminUserList(writer http.ResponseWriter, request *http.Request) {
	page, pageSize, offset := parseAdminPagination(request)
	users, total, err := router.Admin.Console.ListConsoleUsers(request.Context(), router.AdminApplicationID(request), offset, pageSize, request.URL.Query().Get("search"), request.URL.Query().Get("status"))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	items := make([]consoleUserJSON, 0, len(users))
	for _, user := range users {
		items = append(items, mapConsoleUser(user))
	}
	httpapi.WriteJSON(writer, http.StatusOK, adminPageResponse{OK: true, Items: items, Total: total, Page: page, PageSize: pageSize})
}

func (router *Router) handleAdminBanList(writer http.ResponseWriter, request *http.Request) {
	page, pageSize, offset := parseAdminPagination(request)
	bans, total, err := router.Admin.Console.ListConsoleBans(request.Context(), offset, pageSize, request.URL.Query().Get("search"), request.URL.Query().Get("status"))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	type banJSON struct {
		ID         string `json:"id"`
		UserID     string `json:"user_id"`
		UserEmail  string `json:"user_email"`
		Reason     string `json:"reason"`
		ExpiresAt  string `json:"expires_at"`
		Status     string `json:"status"`
		BannedAt   string `json:"banned_at"`
		LiftedAt   string `json:"lifted_at"`
		LiftReason string `json:"lift_reason"`
	}
	items := make([]banJSON, 0, len(bans))
	for _, ban := range bans {
		items = append(items, banJSON{
			ID:         ban.ID,
			UserID:     ban.UserID,
			UserEmail:  ban.UserEmail,
			Reason:     ban.Reason,
			ExpiresAt:  formatTimePtr(ban.ExpiresAt),
			Status:     ban.Status,
			BannedAt:   httpapi.FormatTime(ban.BannedAt),
			LiftedAt:   formatTimePtr(ban.LiftedAt),
			LiftReason: ban.LiftReason,
		})
	}
	httpapi.WriteJSON(writer, http.StatusOK, adminPageResponse{OK: true, Items: items, Total: total, Page: page, PageSize: pageSize})
}

func (router *Router) handleAdminUserDetail(writer http.ResponseWriter, request *http.Request, userID string) {
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}
	detail, err := router.Admin.Console.ConsoleUserDetail(request.Context(), router.AdminApplicationID(request), userID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK           bool                 `json:"ok"`
		User         consoleUserJSON      `json:"user"`
		Notes        string               `json:"notes"`
		BanReason    string               `json:"ban_reason"`
		BannedAt     string               `json:"banned_at"`
		BanExpiresAt string               `json:"ban_expires_at"`
		Licenses     []consoleLicenseJSON `json:"licenses"`
		Devices      []consoleDeviceJSON  `json:"devices"`
		Sessions     []consoleSessionJSON `json:"sessions"`
	}{
		OK:           true,
		User:         mapConsoleUser(detail.User),
		Notes:        detail.Notes,
		BanReason:    detail.BanReason,
		BannedAt:     formatTimePtr(detail.BannedAt),
		BanExpiresAt: formatTimePtr(detail.BanExpiresAt),
		Licenses:     mapConsoleLicenses(detail.Licenses),
		Devices:      mapConsoleDevices(detail.Devices),
		Sessions:     mapConsoleSessions(detail.Sessions),
	})
}

func (router *Router) handleAdminUserStatus(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, userID string) {
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if body.Status != string(domain.UserStatusActive) && body.Status != string(domain.UserStatusDisabled) {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "status must be active or disabled")
		return
	}
	if err := router.Admin.Console.SetUserStatus(request.Context(), router.AdminApplicationID(request), userID, domain.UserStatus(body.Status)); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "USER_STATUS_CHANGED", "user", userID, map[string]string{"status": body.Status})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// handleAdminUserPromote turns an existing end-user into a dashboard admin.
// A strong temporary password is generated (or the request may supply one) and
// returned exactly once — the end-user's own password is NEVER reused for
// console access, so a compromised client credential cannot open the console.
// The role defaults to viewer when omitted.
func (router *Router) handleAdminUserPromote(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, userID string) {
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}
	var body struct {
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	user, err := router.Admin.Console.FindUserByID(request.Context(), router.AdminApplicationID(request), userID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	password := body.Password
	if password == "" {
		generated, err := generateTemporaryPassword()
		if err != nil {
			httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
			return
		}
		password = generated
	} else if len(password) < minAdminPasswordLength {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "password must be at least 12 characters")
		return
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	role := strings.ToLower(strings.TrimSpace(body.Role))
	if role == "" {
		role = domain.RoleViewer
	}
	created, err := router.Admin.Console.CreateAdminAccount(request.Context(), domain.NewAdminAccount{
		Email:        user.Email,
		PasswordHash: hash,
		RoleName:     role,
	})
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "ADMIN_PROMOTED", "admin_account", created.ID, map[string]string{"email": created.Email, "role": role})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK           bool             `json:"ok"`
		Admin        adminAccountJSON `json:"admin"`
		TempPassword string           `json:"temp_password"`
	}{
		OK:           true,
		Admin:        mapAdminAccount(*created),
		TempPassword: password,
	})
}

// handleAdminUserBulkStatus enables or disables several end-user accounts in a
// single statement.
func (router *Router) handleAdminUserBulkStatus(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		IDs    []string `json:"ids"`
		Status string   `json:"status"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > 500 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "ids must contain between 1 and 500 users")
		return
	}
	if body.Status != string(domain.UserStatusActive) && body.Status != string(domain.UserStatusDisabled) {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "status must be active or disabled")
		return
	}
	updated, err := router.Admin.Console.BulkSetUserStatus(request.Context(), router.AdminApplicationID(request), body.IDs, domain.UserStatus(body.Status))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "USERS_BULK_STATUS", "user", "", map[string]string{
		"status": body.Status, "count": strconv.FormatInt(updated, 10),
	})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool  `json:"ok"`
		Updated int64 `json:"updated"`
	}{OK: true, Updated: updated})
}

// handleAdminUserBulkRevoke expires every pending or verified auth session of
// several users in one statement.
func (router *Router) handleAdminUserBulkRevoke(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > 500 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "ids must contain between 1 and 500 users")
		return
	}
	revoked, err := router.Admin.Console.BulkRevokeUserSessions(request.Context(), router.AdminApplicationID(request), body.IDs)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "USERS_BULK_SESSIONS_REVOKED", "user", "", map[string]string{
		"count": strconv.FormatInt(revoked, 10),
	})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool  `json:"ok"`
		Revoked int64 `json:"revoked"`
	}{OK: true, Revoked: revoked})
}

// handleAdminUserPasswordReset sets a new password for an end-user. When the
// request body omits a password, a strong random one is generated and returned
// exactly once so the admin can hand it to the user over a trusted channel.
func (router *Router) handleAdminUserPasswordReset(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, userID string) {
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	password := body.Password
	if password == "" {
		generated, err := generateTemporaryPassword()
		if err != nil {
			httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
			return
		}
		password = generated
	} else if len(password) < httpapi.MinEndUserPasswordLength {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "password must be at least 10 characters")
		return
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	if err := router.Admin.Console.SetUserPassword(request.Context(), router.AdminApplicationID(request), userID, hash); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "USER_PASSWORD_RESET", "user", userID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK           bool   `json:"ok"`
		PasswordSet  bool   `json:"password_set"`
		TempPassword string `json:"temp_password,omitempty"`
	}{
		OK:           true,
		PasswordSet:  body.Password != "",
		TempPassword: password,
	})
}

// generateTemporaryPassword returns a 16-character password from a
// cryptographically secure alphabet that avoids visually ambiguous characters.
func generateTemporaryPassword() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%^&*"
	const length = 16
	bytes := make([]byte, length)
	if _, err := cryptorand.Read(bytes); err != nil {
		return "", err
	}
	for index := range bytes {
		bytes[index] = alphabet[int(bytes[index])%len(alphabet)]
	}
	return string(bytes), nil
}

var banDurationUnits = map[string]bool{
	"hours": true, "days": true, "weeks": true, "months": true, "years": true,
}

// addLicenseDuration moves a timestamp forward by a value/unit pair.
func addLicenseDuration(base time.Time, value int, unit string) (time.Time, error) {
	if value < 1 || value > 1000 {
		return time.Time{}, errors.New("duration must be between 1 and 1000")
	}
	switch unit {
	case "hours":
		return base.Add(time.Duration(value) * time.Hour), nil
	case "days":
		return base.AddDate(0, 0, value), nil
	case "weeks":
		return base.AddDate(0, 0, 7*value), nil
	case "months":
		return base.AddDate(0, value, 0), nil
	case "years":
		return base.AddDate(value, 0, 0), nil
	default:
		return time.Time{}, errors.New("duration unit must be hours, days, weeks, months or years")
	}
}

// banExpiresAt computes a temporary ban deadline from a value/unit pair.
func banExpiresAt(now time.Time, value int, unit string) (time.Time, error) {
	if value < 1 || value > 1000 {
		return time.Time{}, errors.New("duration must be between 1 and 1000")
	}
	switch unit {
	case "hours":
		return now.Add(time.Duration(value) * time.Hour), nil
	case "days":
		return now.AddDate(0, 0, value), nil
	case "weeks":
		return now.AddDate(0, 0, 7*value), nil
	case "months":
		return now.AddDate(0, value, 0), nil
	case "years":
		return now.AddDate(value, 0, 0), nil
	default:
		return time.Time{}, errors.New("duration unit must be hours, days, weeks, months or years")
	}
}

// handleAdminUserBan bans a user with a required reason. The ban is permanent
// by default; a duration (value + unit) produces a temporary ban that expires
// automatically (see store.AutoUnbanExpired). Banned users are rejected by the
// client login path (status is no longer active).
func (router *Router) handleAdminUserBan(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, userID string) {
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}
	var body struct {
		Reason        string `json:"reason"`
		Permanent     bool   `json:"permanent"`
		DurationValue int    `json:"duration_value"`
		DurationUnit  string `json:"duration_unit"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" || len(reason) > 500 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "a ban reason (max 500 chars) is required")
		return
	}
	var expiresAt *time.Time
	if !body.Permanent {
		if body.DurationValue == 0 {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "a duration or permanent=true is required")
			return
		}
		deadline, err := banExpiresAt(router.Now(), body.DurationValue, body.DurationUnit)
		if err != nil {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		expiresAt = &deadline
	}
	if err := router.Admin.Console.BanUser(request.Context(), router.AdminApplicationID(request), userID, reason, expiresAt); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	metadata := map[string]string{"reason": reason}
	if expiresAt != nil {
		metadata["ban_until"] = expiresAt.UTC().Format(time.RFC3339)
		metadata["duration"] = fmt.Sprintf("%d %s", body.DurationValue, body.DurationUnit)
	} else {
		metadata["duration"] = "permanent"
	}
	router.AuditAdmin(request, account, "USER_BANNED", "user", userID, metadata)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK        bool   `json:"ok"`
		BanUntil  string `json:"ban_until"`
		Permanent bool   `json:"permanent"`
	}{
		OK:        true,
		BanUntil:  formatTimePtr(expiresAt),
		Permanent: expiresAt == nil,
	})
}

// handleAdminUserUnban clears a user's ban and restores active status.
func (router *Router) handleAdminUserUnban(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, userID string) {
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}
	if err := router.Admin.Console.UnbanUser(request.Context(), router.AdminApplicationID(request), userID); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "USER_UNBANNED", "user", userID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// handleAdminUserNotes updates the free-form admin notes on a user.
func (router *Router) handleAdminUserNotes(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, userID string) {
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}
	var body struct {
		Notes string `json:"notes"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	notes := strings.TrimSpace(body.Notes)
	if len(notes) > 4000 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "notes must be at most 4000 characters")
		return
	}
	if err := router.Admin.Console.SetUserNotes(request.Context(), router.AdminApplicationID(request), userID, notes); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "USER_NOTES_UPDATED", "user", userID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool   `json:"ok"`
		Notes string `json:"notes"`
	}{OK: true, Notes: notes})
}

// handleAdminUserResetDevices deletes every device of a user (HWID reset).
func (router *Router) handleAdminUserResetDevices(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, userID string) {
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}
	reset, err := router.Admin.Console.ResetUserDevices(request.Context(), router.AdminApplicationID(request), userID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "USER_DEVICES_RESET", "user", userID, map[string]int64{"devices": reset})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool  `json:"ok"`
		Devices int64 `json:"devices"`
	}{OK: true, Devices: reset})
}

// handleAdminUserSessionsRevoke expires every pending or verified auth
// session of a single user.
func (router *Router) handleAdminUserSessionsRevoke(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, userID string) {
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}
	revoked, err := router.Admin.Console.RevokeUserSessions(request.Context(), router.AdminApplicationID(request), userID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "USER_SESSIONS_REVOKED", "user", userID, map[string]int64{"revoked": revoked})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool  `json:"ok"`
		Revoked int64 `json:"revoked"`
	}{OK: true, Revoked: revoked})
}

// Licenses

type consoleLicenseJSON struct {
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
}

func mapConsoleLicenses(licenses []domain.ConsoleLicense) []consoleLicenseJSON {
	items := make([]consoleLicenseJSON, 0, len(licenses))
	for _, license := range licenses {
		items = append(items, consoleLicenseJSON{
			ID:         license.ID,
			UserID:     license.UserID,
			UserEmail:  license.UserEmail,
			Product:    license.Product,
			Status:     string(license.Status),
			Level:      license.Level,
			MaxDevices: license.MaxDevices,
			Notes:      license.Notes,
			ExpiresAt:  httpapi.FormatTime(license.ExpiresAt),
			CreatedAt:  httpapi.FormatTime(license.CreatedAt),
		})
	}
	return items
}

func (router *Router) routeAdminLicenses(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermLicensesRead) {
			return
		}
		router.handleAdminLicenseList(writer, request)
	case len(segments) == 1 && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermLicensesWrite) {
			return
		}
		router.handleAdminLicenseCreate(writer, request, account)
	case len(segments) == 2 && request.Method == http.MethodPatch:
		if !router.RequirePermission(writer, request, account, domain.PermLicensesWrite) {
			return
		}
		router.handleAdminLicenseUpdate(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "revoke" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermLicensesWrite) {
			return
		}
		router.handleAdminLicenseRevoke(writer, request, account, segments[1])
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func (router *Router) handleAdminLicenseList(writer http.ResponseWriter, request *http.Request) {
	page, pageSize, offset := parseAdminPagination(request)
	licenses, total, err := router.Admin.Console.ListConsoleLicenses(request.Context(), router.AdminApplicationID(request), offset, pageSize)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, adminPageResponse{OK: true, Items: mapConsoleLicenses(licenses), Total: total, Page: page, PageSize: pageSize})
}

func (router *Router) handleAdminLicenseCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		UserEmail  string `json:"user_email"`
		Days       int    `json:"days"`
		Value      int    `json:"value"`
		Unit       string `json:"unit"`
		MaxDevices int    `json:"max_devices"`
		ProductID  string `json:"product_id"`
		PlanID     string `json:"plan_id"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	durationValue := body.Value
	unit := strings.ToLower(strings.TrimSpace(body.Unit))
	if unit == "" {
		unit = "days"
	}
	if durationValue == 0 {
		durationValue = body.Days // legacy callers send days only
	}
	if strings.TrimSpace(body.UserEmail) == "" || !banDurationUnits[unit] || body.MaxDevices < 1 || body.MaxDevices > 10000 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "user_email, a valid duration (unit: hours/days/weeks/months/years) and max_devices (1-10000) are required")
		return
	}
	expiresAt, err := addLicenseDuration(router.Now().UTC(), durationValue, unit)
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	user, err := router.Admin.Console.FindUserByEmail(request.Context(), router.AdminApplicationID(request), body.UserEmail)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	productID, planID := body.ProductID, body.PlanID
	if productID == "" || planID == "" {
		// Legacy callers use the configured product and its default plan.
		productID, planID, err = router.Admin.Console.ResolveProductPlan(request.Context(), router.AdminApplicationID(request), router.Admin.Product)
		if err != nil {
			router.WriteConsoleError(writer, request, err)
			return
		}
	} else {
		product, findErr := router.Admin.Console.FindProductByID(request.Context(), router.AdminApplicationID(request), productID)
		if findErr != nil || product == nil {
			router.WriteConsoleError(writer, request, findErr)
			return
		}
		plans, listErr := router.Admin.Console.ListPlans(request.Context(), productID)
		if listErr != nil {
			router.WriteConsoleError(writer, request, listErr)
			return
		}
		found := false
		for _, plan := range plans {
			if plan.ID == planID {
				found = true
				break
			}
		}
		if !found {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "plan does not belong to product")
			return
		}
	}
	plain, normalized, err := security.GenerateLicense(cryptorand.Reader)
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	license, err := router.Admin.Console.CreateLicense(request.Context(), router.AdminApplicationID(request), domain.NewLicense{
		LicenseHMAC: security.HMACHex(router.Admin.LicenseHMACKey, normalized),
		UserID:      user.ID,
		ProductID:   productID,
		PlanID:      planID,
		MaxDevices:  body.MaxDevices,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "LICENSE_CREATED", "license", license.ID, map[string]string{"user_email": user.Email})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool               `json:"ok"`
		License consoleLicenseJSON `json:"license"`
		Key     string             `json:"key"`
	}{
		OK: true,
		License: consoleLicenseJSON{
			ID: license.ID, UserID: license.UserID, UserEmail: user.Email, Product: license.Product,
			Status: string(license.Status), MaxDevices: license.MaxDevices,
			ExpiresAt: httpapi.FormatTime(license.ExpiresAt), CreatedAt: httpapi.FormatTime(license.CreatedAt),
		},
		Key: plain,
	})
}

func (router *Router) handleAdminLicenseUpdate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, licenseID string) {
	if !httpapi.ValidUUID(licenseID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "LICENSE_NOT_FOUND", "license not found")
		return
	}
	var body struct {
		ExtendValue int    `json:"extend_value"`
		ExtendUnit  string `json:"extend_unit"`
		ExtendDays  int    `json:"extend_days"`
		MaxDevices  int    `json:"max_devices"`
		Level       *int   `json:"level"`
		Notes       string `json:"notes"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if body.ExtendValue == 0 && body.ExtendDays == 0 && body.MaxDevices == 0 && body.Level == nil && strings.TrimSpace(body.Notes) == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "extend_days, max_devices, level or notes is required")
		return
	}
	if body.ExtendDays < 0 || body.ExtendDays > 3650 || body.MaxDevices < 0 || body.MaxDevices > 10000 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid extend_days or max_devices")
		return
	}
	if len(body.Notes) > 2000 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "notes must be at most 2000 characters")
		return
	}
	license, err := router.Admin.Console.FindLicenseByID(request.Context(), router.AdminApplicationID(request), licenseID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	if license.Status == domain.LicenseStatusRevoked {
		httpapi.WriteError(writer, request, http.StatusConflict, "LICENSE_REVOKED", "revoked licenses cannot be modified")
		return
	}
	unit := strings.ToLower(strings.TrimSpace(body.ExtendUnit))
	if unit == "" {
		unit = "days"
	}
	if !banDurationUnits[unit] {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "extend_unit must be hours, days, weeks, months or years")
		return
	}
	extendValue := body.ExtendValue
	if extendValue == 0 {
		extendValue = body.ExtendDays // legacy callers send extend_days only
	}
	expiresAt := license.ExpiresAt
	if extendValue > 0 {
		deadline, err := addLicenseDuration(license.ExpiresAt, extendValue, unit)
		if err != nil {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		if !deadline.After(router.Now()) {
			deadline, err = addLicenseDuration(router.Now(), extendValue, unit)
			if err != nil {
				httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
				return
			}
		}
		expiresAt = deadline
	}
	maxDevices := body.MaxDevices
	if maxDevices == 0 {
		maxDevices = license.MaxDevices
	}
	level := license.Level
	if body.Level != nil {
		if *body.Level < 1 || *body.Level > 100 {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "level must be between 1 and 100")
			return
		}
		level = *body.Level
	}
	notes := license.Notes
	if strings.TrimSpace(body.Notes) != "" || body.Level != nil {
		notes = strings.TrimSpace(body.Notes)
	}
	if err := router.Admin.Console.AdminUpdateLicense(request.Context(), router.AdminApplicationID(request), licenseID, expiresAt, maxDevices, level, notes); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "LICENSE_UPDATED", "license", licenseID, map[string]any{
		"extend": fmt.Sprintf("%d %s", extendValue, unit), "max_devices": maxDevices, "level": level,
	})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (router *Router) handleAdminLicenseRevoke(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, licenseID string) {
	if !httpapi.ValidUUID(licenseID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "LICENSE_NOT_FOUND", "license not found")
		return
	}
	if err := router.Admin.Console.AdminRevokeLicense(request.Context(), router.AdminApplicationID(request), licenseID); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "LICENSE_REVOKED", "license", licenseID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// --- Variables (KeyAuth-style key-value store) ---
func (router *Router) routeAdminVariables(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsRead) {
			return
		}
		router.handleAdminVariablesList(writer, request)
	case len(segments) == 1 && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsWrite) {
			return
		}
		router.handleAdminVariableCreate(writer, request, account)
	case len(segments) == 2 && request.Method == http.MethodPatch:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsWrite) {
			return
		}
		router.handleAdminVariableUpdate(writer, request, account, segments[1])
	case len(segments) == 2 && request.Method == http.MethodDelete:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsWrite) {
			return
		}
		router.handleAdminVariableDelete(writer, request, account, segments[1])
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func (router *Router) handleAdminVariablesList(writer http.ResponseWriter, request *http.Request) {
	variables, err := router.Admin.Console.ListVariables(request.Context(), router.AdminApplicationID(request))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	items := make([]httpapi.VariableJSON, 0, len(variables))
	for _, variable := range variables {
		items = append(items, httpapi.MapVariable(variable))
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool                   `json:"ok"`
		Items []httpapi.VariableJSON `json:"items"`
		Total int                    `json:"total"`
	}{OK: true, Items: items, Total: len(items)})
}

var variableKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func (router *Router) handleAdminVariableCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		Key         string `json:"key"`
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	key := strings.ToLower(strings.TrimSpace(body.Key))
	if !variableKeyPattern.MatchString(key) {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "variable key must be lowercase letters, digits, dots, dashes or underscores (max 64)")
		return
	}
	if len(body.Value) > 10000 || len(body.Description) > 500 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "value or description too long")
		return
	}
	created, err := router.Admin.Console.CreateVariable(request.Context(), router.AdminApplicationID(request), key, body.Value, strings.TrimSpace(body.Description))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "VARIABLE_CREATED", "variable", created.ID, map[string]string{"key": key})
	httpapi.WriteJSON(writer, http.StatusCreated, struct {
		OK       bool                 `json:"ok"`
		Variable httpapi.VariableJSON `json:"variable"`
	}{OK: true, Variable: httpapi.MapVariable(*created)})
}

func (router *Router) handleAdminVariableUpdate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, variableID string) {
	if !httpapi.ValidUUID(variableID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "VARIABLE_NOT_FOUND", "variable not found")
		return
	}
	var body struct {
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if len(body.Value) > 10000 || len(body.Description) > 500 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "value or description too long")
		return
	}
	if err := router.Admin.Console.UpdateVariable(request.Context(), router.AdminApplicationID(request), variableID, body.Value, strings.TrimSpace(body.Description)); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "VARIABLE_UPDATED", "variable", variableID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (router *Router) handleAdminVariableDelete(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, variableID string) {
	if !httpapi.ValidUUID(variableID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "VARIABLE_NOT_FOUND", "variable not found")
		return
	}
	if err := router.Admin.Console.DeleteVariable(request.Context(), router.AdminApplicationID(request), variableID); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "VARIABLE_DELETED", "variable", variableID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// Devices

type consoleDeviceJSON struct {
	ID                   string `json:"id"`
	UserID               string `json:"user_id"`
	UserEmail            string `json:"user_email"`
	LicenseID            string `json:"license_id"`
	TPMRegistered        bool   `json:"tpm_registered"`
	HasSMBIOSUUID        bool   `json:"has_smbios_uuid"`
	HasMotherboardSerial bool   `json:"has_motherboard_serial"`
	HasBIOSSerial        bool   `json:"has_bios_serial"`
	HasSystemDiskSerial  bool   `json:"has_system_disk_serial"`
	HasMachineGUID       bool   `json:"has_machine_guid"`
	Status               string `json:"status"`
	CreatedAt            string `json:"created_at"`
	LastSeenAt           string `json:"last_seen_at"`
}

func mapConsoleDevice(device domain.ConsoleDevice) consoleDeviceJSON {
	return consoleDeviceJSON{
		ID:                   device.ID,
		UserID:               device.UserID,
		UserEmail:            device.UserEmail,
		LicenseID:            device.LicenseID,
		TPMRegistered:        device.TPMRegistered,
		HasSMBIOSUUID:        device.HasSMBIOSUUID,
		HasMotherboardSerial: device.HasMotherboardSerial,
		HasBIOSSerial:        device.HasBIOSSerial,
		HasSystemDiskSerial:  device.HasSystemDiskSerial,
		HasMachineGUID:       device.HasMachineGUID,
		Status:               string(device.Status),
		CreatedAt:            httpapi.FormatTime(device.CreatedAt),
		LastSeenAt:           httpapi.FormatTime(device.LastSeenAt),
	}
}

func mapConsoleDevices(devices []domain.ConsoleDevice) []consoleDeviceJSON {
	items := make([]consoleDeviceJSON, 0, len(devices))
	for _, device := range devices {
		items = append(items, mapConsoleDevice(device))
	}
	return items
}

func (router *Router) routeAdminDevices(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	switch {
	case len(segments) == 1 && segments[0] == "policy" && request.Method == http.MethodGet:
		router.handleAdminDevicePolicyGet(writer, request, account)
	case len(segments) == 1 && segments[0] == "policy" && (request.Method == http.MethodPut || request.Method == http.MethodPatch):
		router.handleAdminDevicePolicyUpdate(writer, request, account)
	case len(segments) == 1 && segments[0] == "policy" && request.Method == http.MethodDelete:
		router.handleAdminDevicePolicyDelete(writer, request, account)
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermDevicesRead) {
			return
		}
		router.handleAdminDeviceList(writer, request)
	case len(segments) == 2 && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermDevicesRead) {
			return
		}
		router.handleAdminDeviceDetail(writer, request, segments[1])
	case len(segments) == 3 && segments[2] == "revoke" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermDevicesWrite) {
			return
		}
		router.handleAdminDeviceRevoke(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "reset" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermDevicesWrite) {
			return
		}
		router.handleAdminDeviceReset(writer, request, account, segments[1])
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func (router *Router) handleAdminDeviceList(writer http.ResponseWriter, request *http.Request) {
	page, pageSize, offset := parseAdminPagination(request)
	devices, total, err := router.Admin.Console.ListConsoleDevices(request.Context(), router.AdminApplicationID(request), offset, pageSize)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, adminPageResponse{OK: true, Items: mapConsoleDevices(devices), Total: total, Page: page, PageSize: pageSize})
}

func (router *Router) handleAdminDeviceRevoke(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, deviceID string) {
	if !httpapi.ValidUUID(deviceID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "DEVICE_NOT_FOUND", "device not found")
		return
	}
	if err := router.Admin.Console.AdminRevokeDevice(request.Context(), router.AdminApplicationID(request), deviceID); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "DEVICE_REVOKED", "device", deviceID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// handleAdminDeviceDetail returns the redacted device view with HWID
// component presence and the TPM public key fingerprint. Raw hardware
// identifiers and HMACs never leave the database.
func (router *Router) handleAdminDeviceDetail(writer http.ResponseWriter, request *http.Request, deviceID string) {
	if !httpapi.ValidUUID(deviceID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "DEVICE_NOT_FOUND", "device not found")
		return
	}
	detail, err := router.Admin.Console.FindConsoleDeviceByID(request.Context(), router.AdminApplicationID(request), deviceID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK             bool              `json:"ok"`
		Device         consoleDeviceJSON `json:"device"`
		Product        string            `json:"product"`
		TPMFingerprint string            `json:"tpm_fingerprint"`
	}{
		OK:             true,
		Device:         mapConsoleDevice(detail.Device),
		Product:        detail.Product,
		TPMFingerprint: detail.TPMFingerprint,
	})
}

// handleAdminDeviceReset removes the hardware registration so the user can
// register a fresh device on the same license.
func (router *Router) handleAdminDeviceReset(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, deviceID string) {
	if !httpapi.ValidUUID(deviceID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "DEVICE_NOT_FOUND", "device not found")
		return
	}
	if err := router.Admin.Console.AdminResetDevice(request.Context(), router.AdminApplicationID(request), deviceID); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "DEVICE_RESET", "device", deviceID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// Sessions

type consoleSessionJSON struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	UserEmail string `json:"user_email"`
	LicenseID string `json:"license_id"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

func mapConsoleSessions(sessions []domain.ConsoleSession) []consoleSessionJSON {
	items := make([]consoleSessionJSON, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, consoleSessionJSON{
			ID:        session.ID,
			UserID:    session.UserID,
			UserEmail: session.UserEmail,
			LicenseID: session.LicenseID,
			Status:    string(session.Status),
			ExpiresAt: httpapi.FormatTime(session.ExpiresAt),
			CreatedAt: httpapi.FormatTime(session.CreatedAt),
		})
	}
	return items
}

func (router *Router) routeAdminSessions(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermSessionsRead) {
			return
		}
		router.handleAdminSessionList(writer, request)
	case len(segments) == 3 && segments[2] == "revoke" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermSessionsWrite) {
			return
		}
		router.handleAdminSessionRevoke(writer, request, account, segments[1])
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func (router *Router) handleAdminSessionList(writer http.ResponseWriter, request *http.Request) {
	page, pageSize, offset := parseAdminPagination(request)
	sessions, total, err := router.Admin.Console.ListConsoleSessions(request.Context(), router.AdminApplicationID(request), offset, pageSize)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, adminPageResponse{OK: true, Items: mapConsoleSessions(sessions), Total: total, Page: page, PageSize: pageSize})
}

func (router *Router) handleAdminSessionRevoke(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, sessionID string) {
	if !httpapi.ValidUUID(sessionID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "SESSION_NOT_FOUND", "session not found")
		return
	}
	if err := router.Admin.Console.AdminRevokeAuthSession(request.Context(), router.AdminApplicationID(request), sessionID); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "SESSION_REVOKED", "session", sessionID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// Audit logs

type auditEntryJSON struct {
	ID             string          `json:"id"`
	AdminAccountID string          `json:"admin_account_id"`
	ActorEmail     string          `json:"actor_email"`
	Action         string          `json:"action"`
	ResourceType   string          `json:"resource_type"`
	ResourceID     string          `json:"resource_id"`
	UserAgent      string          `json:"user_agent"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      string          `json:"created_at"`
}

func mapAuditEntries(logs []domain.AuditLog) []auditEntryJSON {
	items := make([]auditEntryJSON, 0, len(logs))
	for _, entry := range logs {
		metadata := entry.Metadata
		if len(metadata) == 0 {
			metadata = json.RawMessage("{}")
		}
		items = append(items, auditEntryJSON{
			ID:             entry.ID,
			AdminAccountID: entry.AdminAccountID,
			ActorEmail:     entry.ActorEmail,
			Action:         entry.Action,
			ResourceType:   entry.ResourceType,
			ResourceID:     entry.ResourceID,
			UserAgent:      entry.UserAgent,
			Metadata:       metadata,
			CreatedAt:      httpapi.FormatTime(entry.CreatedAt),
		})
	}
	return items
}

func (router *Router) handleAdminAuditLogs(writer http.ResponseWriter, request *http.Request) {
	page, pageSize, offset := parseAdminPagination(request)
	logs, total, err := router.Admin.Console.ListAuditLogs(request.Context(), offset, pageSize)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, adminPageResponse{OK: true, Items: mapAuditEntries(logs), Total: total, Page: page, PageSize: pageSize})
}

func (router *Router) handleAdminActivity(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	logs, err := router.Admin.Console.ListAdminActivity(request.Context(), account.ID, 10)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool             `json:"ok"`
		Items []auditEntryJSON `json:"items"`
	}{OK: true, Items: mapAuditEntries(logs)})
}
