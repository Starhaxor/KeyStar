package adminapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/security"
)

// handleAdminBootstrap exposes the one-time first-run state and root account
// creation. After any admin exists, the store permanently closes creation.
func (router *Router) handleAdminBootstrap(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet {
		required, err := router.Admin.Console.AdminBootstrapRequired(request.Context())
		if err != nil {
			httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
			return
		}
		httpapi.WriteJSON(writer, http.StatusOK, struct {
			OK            bool `json:"ok"`
			SetupRequired bool `json:"setup_required"`
		}{OK: true, SetupRequired: required})
		return
	}
	required, err := router.Admin.Console.AdminBootstrapRequired(request.Context())
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	if !required {
		httpapi.WriteError(writer, request, http.StatusConflict, "BOOTSTRAP_ALREADY_COMPLETED", "initial setup is already complete")
		return
	}
	ipAddress := httpapi.ClientIP(request, router.TrustedProxies())
	if !router.AllowAdminRate(request.Context(), ipAddress+"|admin-bootstrap") {
		httpapi.WriteError(writer, request, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
		return
	}

	var body struct {
		Email          string `json:"email"`
		Password       string `json:"password"`
		BootstrapToken string `json:"bootstrap_token"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	providedToken := sha256.Sum256([]byte(body.BootstrapToken))
	expectedToken := sha256.Sum256([]byte(router.Admin.BootstrapToken))
	if strings.TrimSpace(body.BootstrapToken) == "" || subtle.ConstantTimeCompare(providedToken[:], expectedToken[:]) != 1 {
		router.RecordSecurityEvent(request, nil, "ADMIN_BOOTSTRAP_TOKEN_REJECTED", "critical", nil)
		httpapi.WriteError(writer, request, http.StatusUnauthorized, "INVALID_BOOTSTRAP_TOKEN", "invalid bootstrap token")
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
	hash, err := security.HashPassword(body.Password)
	if err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	account, err := router.Admin.Console.BootstrapAdminAccount(request.Context(), domain.NewAdminAccount{
		Email:        email,
		PasswordHash: hash,
		RoleName:     domain.RoleOwner,
	})
	if errors.Is(err, domain.ErrAdminBootstrapClosed) {
		httpapi.WriteError(writer, request, http.StatusConflict, "BOOTSTRAP_ALREADY_COMPLETED", "initial setup is already complete")
		return
	}
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.RecordSecurityEvent(request, account, "ADMIN_ROOT_BOOTSTRAPPED", "critical", nil)
	router.AuditAdmin(request, account, "ADMIN_ROOT_BOOTSTRAPPED", "admin_account", account.ID, nil)
	loginResult, loginErr := router.Admin.Auth.Login(request.Context(), email, body.Password, ipAddress, request.UserAgent())
	sessionCreated := loginErr == nil && !loginResult.MFARequired && loginResult.Token != ""
	if sessionCreated {
		router.SetAdminCookies(writer, loginResult.Token)
	} else {
		router.RecordSecurityEvent(request, account, "ADMIN_BOOTSTRAP_SESSION_FAILED", "warning", nil)
	}
	httpapi.WriteJSON(writer, http.StatusCreated, struct {
		OK             bool `json:"ok"`
		SessionCreated bool `json:"session_created"`
	}{OK: account != nil, SessionCreated: sessionCreated})
}
