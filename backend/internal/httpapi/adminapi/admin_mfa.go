package adminapi

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/security"
	"github.com/starloader/backend/internal/service/adminauth"
)

// --- TOTP enrollment ---

func (router *Router) handleAdminMFAEnrollStart(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	secret, provisioningURI, err := router.Admin.Auth.StartMFAEnrollment(request.Context(), account, router.AdminMFAIssuer())
	if errors.Is(err, adminauth.ErrMFAAlreadyEnrolled) {
		httpapi.WriteError(writer, request, http.StatusConflict, "MFA_ALREADY_ENROLLED", "mfa already enrolled")
		return
	}
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK              bool   `json:"ok"`
		Secret          string `json:"secret"`
		ProvisioningURI string `json:"provisioning_uri"`
	}{OK: true, Secret: secret, ProvisioningURI: provisioningURI})
}

func (router *Router) handleAdminMFAEnrollConfirm(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		Code string `json:"code"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if strings.TrimSpace(body.Code) == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "code is required")
		return
	}
	ipAddress := httpapi.ClientIP(request, router.TrustedProxies())
	recoveryCodes, err := router.Admin.Auth.ConfirmMFAEnrollment(request.Context(), account, body.Code, ipAddress, request.UserAgent())
	switch {
	case errors.Is(err, adminauth.ErrInvalidMFACode):
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_MFA_CODE", "invalid mfa code")
		return
	case errors.Is(err, adminauth.ErrMFANotEnrolled):
		httpapi.WriteError(writer, request, http.StatusBadRequest, "MFA_ENROLLMENT_NOT_STARTED", "start mfa enrollment first")
		return
	case errors.Is(err, adminauth.ErrMFAAlreadyEnrolled):
		httpapi.WriteError(writer, request, http.StatusConflict, "MFA_ALREADY_ENROLLED", "mfa already enrolled")
		return
	case err != nil:
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	router.RecordSecurityEvent(request, account, "ADMIN_MFA_ENROLLED", "info", nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK            bool     `json:"ok"`
		RecoveryCodes []string `json:"recovery_codes"`
	}{OK: true, RecoveryCodes: recoveryCodes})
}

// handleAdminMFADisable turns MFA off for the caller after password
// re-verification. Every session of the account is revoked, so the response
// also clears the cookies.
func (router *Router) handleAdminMFADisable(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		Password string `json:"password"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if body.Password == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "password is required")
		return
	}
	ipAddress := httpapi.ClientIP(request, router.TrustedProxies())
	if err := router.Admin.Auth.DisableMFA(request.Context(), account, body.Password, ipAddress, request.UserAgent()); err != nil {
		if errors.Is(err, adminauth.ErrInvalidCredentials) {
			httpapi.WriteError(writer, request, http.StatusUnauthorized, "INVALID_PASSWORD", "password verification failed")
			return
		}
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	router.RecordSecurityEvent(request, account, "ADMIN_MFA_DISABLED", "warning", nil)
	router.ClearAdminCookies(writer)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// --- Admin account management ---

type adminAccountJSON struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	Status      string   `json:"status"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	MFAEnrolled bool     `json:"mfa_enrolled"`
	CreatedAt   string   `json:"created_at"`
}

func mapAdminAccount(account domain.AdminAccount) adminAccountJSON {
	return adminAccountJSON{
		ID:          account.ID,
		Email:       account.Email,
		Status:      string(account.Status),
		Role:        account.RoleName,
		Permissions: account.Permissions,
		MFAEnrolled: account.MFAEnrolled,
		CreatedAt:   httpapi.FormatTime(account.CreatedAt),
	}
}

func (router *Router) routeAdminAccounts(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsRead) {
			return
		}
		router.handleAdminAccountList(writer, request)
	case len(segments) == 1 && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsWrite) {
			return
		}
		router.handleAdminAccountCreate(writer, request, account)
	case len(segments) == 2 && request.Method == http.MethodPatch:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsWrite) {
			return
		}
		router.handleAdminAccountUpdate(writer, request, account, segments[1])
	case len(segments) == 3 && segments[2] == "reset-password" && request.Method == http.MethodPost:
		if !router.RequirePermission(writer, request, account, domain.PermAdminsWrite) {
			return
		}
		router.handleAdminPasswordReset(writer, request, account, segments[1])
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

// handleAdminPasswordReset sets a new password for another administrator. A
// strong temporary password is generated when none is supplied and returned
// exactly once. All of the target's sessions are revoked so the change takes
// effect immediately. Self-service password changes are handled separately;
// self-reset here is rejected.
func (router *Router) handleAdminPasswordReset(writer http.ResponseWriter, request *http.Request, actor *domain.AdminAccount, adminID string) {
	if !httpapi.ValidUUID(adminID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "ADMIN_NOT_FOUND", "admin not found")
		return
	}
	if adminID == actor.ID {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "ADMIN_SELF_MODIFICATION", "you cannot reset your own password here")
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
	} else if len(password) < minAdminPasswordLength {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "password must be at least 12 characters")
		return
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	if err := router.Admin.Console.SetAdminPassword(request.Context(), adminID, hash); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	_ = router.Admin.Console.RevokeAllAdminSessions(request.Context(), adminID)
	router.AuditAdmin(request, actor, "ADMIN_PASSWORD_RESET", "admin_account", adminID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK           bool   `json:"ok"`
		TempPassword string `json:"temp_password"`
	}{OK: true, TempPassword: password})
}

func (router *Router) handleAdminAccountList(writer http.ResponseWriter, request *http.Request) {
	accounts, err := router.Admin.Console.ListAdminAccounts(request.Context())
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	items := make([]adminAccountJSON, 0, len(accounts))
	for _, admin := range accounts {
		items = append(items, mapAdminAccount(admin))
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool               `json:"ok"`
		Items []adminAccountJSON `json:"items"`
		Total int                `json:"total"`
	}{OK: true, Items: items, Total: len(items)})
}

// handleAdminAccountCreate provisions a dashboard administrator. The new
// account starts MFA-unenrolled, so the enrollment gate forces TOTP setup on
// first sign-in before any console endpoint becomes reachable.
func (router *Router) handleAdminAccountCreate(writer http.ResponseWriter, request *http.Request, actor *domain.AdminAccount) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
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
	if len(body.Password) < minAdminPasswordLength {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "password must be at least 12 characters")
		return
	}
	role := strings.ToLower(strings.TrimSpace(body.Role))
	if role == "" {
		role = domain.RoleViewer
	}
	roles, err := router.Admin.Console.ListRoles(request.Context())
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	roleValid := false
	for _, candidate := range roles {
		if candidate.Name == role {
			roleValid = true
			break
		}
	}
	if !roleValid {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "ROLE_NOT_FOUND", "role not found")
		return
	}
	hash, err := security.HashPassword(body.Password)
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	created, err := router.Admin.Console.CreateAdminAccount(request.Context(), domain.NewAdminAccount{
		Email:        email,
		PasswordHash: hash,
		RoleName:     role,
	})
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, actor, "ADMIN_CREATED", "admin_account", created.ID, map[string]string{"email": created.Email, "role": role})
	httpapi.WriteJSON(writer, http.StatusCreated, struct {
		OK    bool             `json:"ok"`
		Admin adminAccountJSON `json:"admin"`
	}{OK: true, Admin: mapAdminAccount(*created)})
}

// handleAdminAccountUpdate changes the status and/or role of another admin.
// Self-modification is rejected: the acting admin manages their own MFA via
// the /mfa endpoints and cannot lock themselves out or escalate themselves.
func (router *Router) handleAdminAccountUpdate(writer http.ResponseWriter, request *http.Request, actor *domain.AdminAccount, adminID string) {
	if !httpapi.ValidUUID(adminID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "ADMIN_NOT_FOUND", "admin not found")
		return
	}
	if adminID == actor.ID {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "ADMIN_SELF_MODIFICATION", "you cannot modify your own account")
		return
	}
	var body struct {
		Status string `json:"status"`
		Role   string `json:"role"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	status := strings.TrimSpace(body.Status)
	roleName := strings.ToLower(strings.TrimSpace(body.Role))
	if status == "" && roleName == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "status or role is required")
		return
	}
	if status != "" && status != string(domain.AdminStatusActive) && status != string(domain.AdminStatusDisabled) {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "status must be active or disabled")
		return
	}
	if status == "" {
		status = string(domain.AdminStatusActive)
	}
	if roleName != "" {
		roles, err := router.Admin.Console.ListRoles(request.Context())
		if err != nil {
			router.WriteConsoleError(writer, request, err)
			return
		}
		found := false
		for _, role := range roles {
			if role.Name == roleName {
				found = true
				break
			}
		}
		if !found {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "ROLE_NOT_FOUND", "unknown role")
			return
		}
	}
	target, err := router.Admin.Console.FindAdminAccountByID(request.Context(), adminID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	if status == "" {
		status = string(target.Status)
	}
	if status == string(domain.AdminStatusDisabled) {
		router.RecordSecurityEvent(request, actor, "ADMIN_ACCOUNT_DISABLED", "warning", map[string]string{"target_admin_id": adminID})
	}
	if err := router.Admin.Console.UpdateAdminAccountStatusAndRole(request.Context(), adminID, domain.AdminAccountStatus(status), roleName); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, actor, "ADMIN_ACCOUNT_UPDATED", "admin_account", adminID, map[string]string{
		"status": status, "role": roleName,
	})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// --- Roles ---

type roleJSON struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	BuiltIn     bool     `json:"built_in"`
	MemberCount int      `json:"member_count"`
}

func mapRoleJSON(role domain.Role) roleJSON {
	return roleJSON{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		Permissions: role.Permissions,
		BuiltIn:     role.BuiltIn,
		MemberCount: role.MemberCount,
	}
}

func (router *Router) handleAdminRoles(writer http.ResponseWriter, request *http.Request) {
	roles, err := router.Admin.Console.ListRoles(request.Context())
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	items := make([]roleJSON, 0, len(roles))
	for _, role := range roles {
		items = append(items, mapRoleJSON(role))
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool       `json:"ok"`
		Items []roleJSON `json:"items"`
		Total int        `json:"total"`
	}{OK: true, Items: items, Total: len(items)})
}

// handleAdminRoleMembers returns the admin accounts assigned to a role. The
// email list powers the expandable member panel in the Roles page.
func (router *Router) handleAdminRoleMembers(writer http.ResponseWriter, request *http.Request, roleID string) {
	if !httpapi.ValidUUID(roleID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "ROLE_NOT_FOUND", "role not found")
		return
	}
	members, err := router.Admin.Console.ListRoleMembers(request.Context(), roleID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	items := make([]roleMemberJSON, 0, len(members))
	for _, member := range members {
		items = append(items, roleMemberJSON{
			ID:          member.ID,
			Email:       member.Email,
			Status:      string(member.Status),
			MFAEnrolled: member.MFAEnrolled,
			CreatedAt:   httpapi.FormatTime(member.CreatedAt),
		})
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool             `json:"ok"`
		Items []roleMemberJSON `json:"items"`
		Total int              `json:"total"`
	}{OK: true, Items: items, Total: len(items)})
}

type roleMemberJSON struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Status      string `json:"status"`
	MFAEnrolled bool   `json:"mfa_enrolled"`
	CreatedAt   string `json:"created_at"`
}

var roleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// validateRolePermissions rejects permission strings that are not part of the
// known assignable set, keeping custom roles data-driven and safe.
func validateRolePermissions(permissions []string) error {
	for _, permission := range permissions {
		valid := false
		for _, candidate := range domain.AllPermissions {
			if candidate == permission {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("unknown permission %q", permission)
		}
	}
	return nil
}

// handleAdminRoleCreate provisions a custom RBAC role with an explicit
// permission set.
func (router *Router) handleAdminRoleCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	name := strings.ToLower(strings.TrimSpace(body.Name))
	if !roleNamePattern.MatchString(name) {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "role name must be lowercase letters, digits, dashes or underscores (max 32)")
		return
	}
	if len(strings.TrimSpace(body.Description)) > 200 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "description must be at most 200 characters")
		return
	}
	if err := validateRolePermissions(body.Permissions); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	created, err := router.Admin.Console.CreateRole(request.Context(), domain.NewRole{
		Name:        name,
		Description: strings.TrimSpace(body.Description),
		Permissions: body.Permissions,
	})
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "ROLE_CREATED", "role", created.ID, map[string]string{
		"name": created.Name, "permissions": strings.Join(created.Permissions, ","),
	})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK   bool     `json:"ok"`
		Role roleJSON `json:"role"`
	}{
		OK: true,
		Role: roleJSON{
			ID: created.ID, Name: created.Name, Description: created.Description,
			Permissions: created.Permissions, BuiltIn: created.BuiltIn,
		},
	})
}

// handleAdminRoleUpdate changes the description and permission set of a custom
// role. Built-in roles are rejected by the store.
func (router *Router) handleAdminRoleUpdate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, roleID string) {
	if !httpapi.ValidUUID(roleID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "ROLE_NOT_FOUND", "role not found")
		return
	}
	var body struct {
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if len(strings.TrimSpace(body.Description)) > 200 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "description must be at most 200 characters")
		return
	}
	if err := validateRolePermissions(body.Permissions); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if err := router.Admin.Console.UpdateRole(request.Context(), roleID, strings.TrimSpace(body.Description), body.Permissions); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "ROLE_UPDATED", "role", roleID, map[string]string{
		"permissions": strings.Join(body.Permissions, ","),
	})
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// handleAdminRoleDelete removes a custom role. Built-in roles (owner, viewer)
// and roles still assigned to an admin account are rejected; the acting admin
// can never delete the role they are currently assigned to.
func (router *Router) handleAdminRoleDelete(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, roleID string) {
	if !httpapi.ValidUUID(roleID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "ROLE_NOT_FOUND", "role not found")
		return
	}
	if account.RoleID == roleID {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "ROLE_IN_USE", "you cannot delete the role you are currently assigned to")
		return
	}
	if err := router.Admin.Console.DeleteRole(request.Context(), roleID); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "ROLE_DELETED", "role", roleID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// --- Security events ---

type securityEventJSON struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Severity       string `json:"severity"`
	AdminAccountID string `json:"admin_account_id"`
	ActorEmail     string `json:"actor_email"`
	UserAgent      string `json:"user_agent"`
	CreatedAt      string `json:"created_at"`
}

func (router *Router) handleAdminSecurityEvents(writer http.ResponseWriter, request *http.Request) {
	page, pageSize, offset := parseAdminPagination(request)
	query := request.URL.Query()
	events, total, err := router.Admin.Console.ListSecurityEvents(request.Context(), offset, pageSize, query.Get("search"), query.Get("severity"))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	items := make([]securityEventJSON, 0, len(events))
	for _, event := range events {
		items = append(items, securityEventJSON{
			ID:             event.ID,
			Kind:           event.Kind,
			Severity:       event.Severity,
			AdminAccountID: event.AdminAccountID,
			ActorEmail:     event.ActorEmail,
			UserAgent:      event.UserAgent,
			CreatedAt:      httpapi.FormatTime(event.CreatedAt),
		})
	}
	httpapi.WriteJSON(writer, http.StatusOK, adminPageResponse{OK: true, Items: items, Total: total, Page: page, PageSize: pageSize})
}
