package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
)

func (router *Router) handleServerUserList(writer http.ResponseWriter, request *http.Request) {
	limit, after := parseServerPagination(request)
	users, nextCursor, hasMore, err := router.serverStore.ListServerUsers(request.Context(), principalApplicationID(request), after, limit)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		OK    bool             `json:"ok"`
		Data  []serverUserJSON `json:"data"`
		Page  serverPage       `json:"page"`
	}{OK: true, Data: mapServerUsers(users), Page: serverPage{NextCursor: nextCursor, HasMore: hasMore}})
}

type createServerUserRequestBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Notes    string `json:"notes"`
}

func (router *Router) handleServerUserCreate(writer http.ResponseWriter, request *http.Request) {
	var body createServerUserRequestBody
	if err := decodeAdminJSONBody(writer, request, &body); err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	email := strings.TrimSpace(body.Email)
	if email == "" || body.Password == "" {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if len(body.Password) < minEndUserPasswordLength {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	hash, err := security.HashPassword(body.Password)
	if err != nil {
		writeError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	applicationID := principalApplicationID(request)
	user, err := router.serverStore.CreateUser(request.Context(), applicationID, domain.NewUser{
		Email: email, PasswordHash: hash,
	})
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	if notes := strings.TrimSpace(body.Notes); notes != "" {
		if err := router.serverStore.SetUserNotes(request.Context(), applicationID, user.ID, notes); err != nil {
			router.writeServerError(writer, request, err)
			return
		}
	}
	// Re-read the user so the response reflects the notes set above.
	created, err := router.serverStore.FindServerUserByID(request.Context(), applicationID, user.ID)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, struct {
		OK   bool           `json:"ok"`
		Data serverUserJSON `json:"data"`
	}{OK: true, Data: mapServerUser(*created)})
}

func (router *Router) handleServerUserDetail(writer http.ResponseWriter, request *http.Request) {
	user, err := router.serverStore.FindServerUserByID(request.Context(), principalApplicationID(request), serverPathID(request))
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		OK   bool           `json:"ok"`
		Data serverUserJSON `json:"data"`
	}{OK: true, Data: mapServerUser(*user)})
}

type updateServerUserRequestBody struct {
	Status string `json:"status"`
	Notes  string `json:"notes"`
}

func (router *Router) handleServerUserUpdate(writer http.ResponseWriter, request *http.Request) {
	var body updateServerUserRequestBody
	if err := decodeAdminJSONBody(writer, request, &body); err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	applicationID := principalApplicationID(request)
	userID := serverPathID(request)
	status := strings.ToLower(strings.TrimSpace(body.Status))
	if status != "" {
		if status != string(domain.UserStatusActive) && status != string(domain.UserStatusDisabled) {
			writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
			return
		}
		if err := router.serverStore.SetUserStatus(request.Context(), applicationID, userID, domain.UserStatus(status)); err != nil {
			router.writeServerError(writer, request, err)
			return
		}
	}
	if strings.TrimSpace(body.Notes) != "" {
		if err := router.serverStore.SetUserNotes(request.Context(), applicationID, userID, strings.TrimSpace(body.Notes)); err != nil {
			router.writeServerError(writer, request, err)
			return
		}
	}
	user, err := router.serverStore.FindServerUserByID(request.Context(), applicationID, userID)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		OK   bool           `json:"ok"`
		Data serverUserJSON `json:"data"`
	}{OK: true, Data: mapServerUser(*user)})
}

// handleServerUserDelete soft-deletes an end user by disabling the account
// (hard deletion is avoided for security-critical identities; the user and
// their audit trail remain intact).
func (router *Router) handleServerUserDelete(writer http.ResponseWriter, request *http.Request) {
	applicationID := principalApplicationID(request)
	userID := serverPathID(request)
	if _, err := router.serverStore.FindServerUserByID(request.Context(), applicationID, userID); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	if err := router.serverStore.SetUserStatus(request.Context(), applicationID, userID, domain.UserStatusDisabled); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

type banServerUserRequestBody struct {
	Reason    string `json:"reason"`
	ExpiresIn string `json:"expires_in"` // Go duration, e.g. "720h"
}

func (router *Router) handleServerUserBan(writer http.ResponseWriter, request *http.Request) {
	var body banServerUserRequestBody
	if err := decodeAdminJSONBody(writer, request, &body); err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	var expiresAt *time.Time
	if duration := strings.TrimSpace(body.ExpiresIn); duration != "" {
		parsed, err := time.ParseDuration(duration)
		if err != nil || parsed <= 0 {
			writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
			return
		}
		value := time.Now().UTC().Add(parsed)
		expiresAt = &value
	}
	if err := router.serverStore.BanUser(request.Context(), principalApplicationID(request), serverPathID(request), strings.TrimSpace(body.Reason), expiresAt); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (router *Router) handleServerUserUnban(writer http.ResponseWriter, request *http.Request) {
	if err := router.serverStore.UnbanUser(request.Context(), principalApplicationID(request), serverPathID(request)); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (router *Router) handleServerUserResetDevices(writer http.ResponseWriter, request *http.Request) {
	revoked, err := router.serverStore.ResetUserDevices(request.Context(), principalApplicationID(request), serverPathID(request))
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		OK      bool  `json:"ok"`
		Revoked int64 `json:"revoked"`
	}{OK: true, Revoked: revoked})
}

func principalApplicationID(request *http.Request) string {
	if principal, ok := AppPrincipalFromContext(request.Context()); ok && principal.ApplicationID != "" {
		return principal.ApplicationID
	}
	return ""
}

func serverPathID(request *http.Request) string {
	segments := splitServerPath(strings.TrimPrefix(request.URL.Path, serverPathPrefix))
	if len(segments) < 2 {
		return ""
	}
	return segments[1]
}
