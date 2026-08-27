package serverapi

import (
	"net/http"
	"strings"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type serverRefreshSessionJSON struct {
	ID        string  `json:"id"`
	UserID    string  `json:"user_id"`
	DeviceID  string  `json:"device_id"`
	Status    string  `json:"status"`
	ExpiresAt string  `json:"expires_at"`
	LastUsed  *string `json:"last_used_at"`
	CreatedAt string  `json:"created_at"`
	RevokedAt *string `json:"revoked_at"`
}

func mapRefreshSession(s domain.RefreshSession) serverRefreshSessionJSON {
	return serverRefreshSessionJSON{
		ID:        s.ID,
		UserID:    s.UserID,
		DeviceID:  s.DeviceID,
		Status:    string(s.Status),
		ExpiresAt: httpapi.FormatTime(s.ExpiresAt),
		LastUsed:  httpapi.FormatOptionalTime(s.LastUsedAt),
		CreatedAt: httpapi.FormatTime(s.CreatedAt),
		RevokedAt: httpapi.FormatOptionalTime(s.RevokedAt),
	}
}

// handleServerSessionList returns paginated refresh sessions for a user.
// Query params: user_id (required), after (cursor), limit.
func (router *Router) handleServerSessionList(writer http.ResponseWriter, request *http.Request) {
	applicationID := principalApplicationID(request)
	userID := request.URL.Query().Get("user_id")
	if userID == "" {
		segments := splitServerPath(strings.TrimPrefix(request.URL.Path, httpapi.ServerPathPrefix))
		if len(segments) == 3 && segments[0] == "users" && segments[2] == "sessions" {
			userID = segments[1]
		}
	}
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "user_id is required")
		return
	}
	limit, after := parseServerPagination(request)
	sessions, _, hasMore, err := router.ServerStore.ListRefreshSessions(request.Context(), applicationID, userID, after, limit)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	items := make([]serverRefreshSessionJSON, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, mapRefreshSession(s))
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool                       `json:"ok"`
		Items   []serverRefreshSessionJSON `json:"items"`
		HasMore bool                       `json:"has_more"`
	}{OK: true, Items: items, HasMore: hasMore})
}

// handleServerSessionRevoke revokes a single refresh session by ID.
func (router *Router) handleServerSessionRevoke(writer http.ResponseWriter, request *http.Request) {
	applicationID := principalApplicationID(request)
	segments := splitServerPath(strings.TrimPrefix(request.URL.Path, httpapi.ServerPathPrefix))
	if len(segments) < 3 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "session id required")
		return
	}
	sessionID := segments[1]
	if !httpapi.ValidUUID(sessionID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "SESSION_NOT_FOUND", "session not found")
		return
	}
	if err := router.ServerStore.RevokeRefreshSession(request.Context(), applicationID, sessionID); err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// handleServerSessionRevokeAll revokes every active refresh session for a user.
func (router *Router) handleServerSessionRevokeAll(writer http.ResponseWriter, request *http.Request) {
	applicationID := principalApplicationID(request)
	segments := splitServerPath(strings.TrimPrefix(request.URL.Path, httpapi.ServerPathPrefix))
	if len(segments) < 3 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "user_id required")
		return
	}
	userID := segments[1]
	if !httpapi.ValidUUID(userID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
		return
	}
	count, err := router.ServerStore.RevokeAllUserRefreshSessions(request.Context(), applicationID, userID)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool  `json:"ok"`
		Revoked int64 `json:"revoked"`
	}{OK: true, Revoked: count})
}
