package adminapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

func (router *Router) routeAdminDeviceBans(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	if len(segments) == 1 && request.Method == http.MethodGet {
		if !router.RequirePermission(writer, request, account, domain.PermDevicesRead) {
			return
		}
		router.handleDeviceBanList(writer, request)
		return
	}
	if len(segments) == 1 && request.Method == http.MethodPost {
		if !router.RequirePermission(writer, request, account, domain.PermDevicesWrite) {
			return
		}
		router.handleDeviceBanCreate(writer, request, account)
		return
	}
	if len(segments) == 3 && segments[2] == "lift" && request.Method == http.MethodPost {
		if !router.RequirePermission(writer, request, account, domain.PermDevicesWrite) {
			return
		}
		router.handleDeviceBanLift(writer, request, account, segments[1])
		return
	}
	httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
}

func deviceBanJSON(item domain.ConsoleDeviceBan) map[string]any {
	return map[string]any{"id": item.ID, "device_id": item.DeviceID, "user_id": item.UserID, "user_email": item.UserEmail, "reason": item.Reason, "expires_at": formatTimePtr(item.ExpiresAt), "status": string(item.Status), "banned_at": httpapi.FormatTime(item.BannedAt), "lifted_at": formatTimePtr(item.LiftedAt), "lift_reason": item.LiftReason}
}
func (router *Router) handleDeviceBanList(writer http.ResponseWriter, request *http.Request) {
	page, size, offset := parseAdminPagination(request)
	items, total, err := router.Admin.Console.ListConsoleDeviceBans(request.Context(), router.AdminApplicationID(request), offset, size, strings.TrimSpace(request.URL.Query().Get("status")))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	output := make([]map[string]any, 0, len(items))
	for _, item := range items {
		output = append(output, deviceBanJSON(item))
	}
	httpapi.WriteJSON(writer, http.StatusOK, adminPageResponse{OK: true, Items: output, Total: total, Page: page, PageSize: size})
}
func (router *Router) handleDeviceBanCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		DeviceID  string `json:"device_id"`
		Reason    string `json:"reason"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil || !httpapi.ValidUUID(body.DeviceID) {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "device_id is required")
		return
	}
	var expires *time.Time
	if strings.TrimSpace(body.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, body.ExpiresAt)
		if err != nil || !parsed.After(time.Now()) {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "expires_at must be a future RFC3339 time")
			return
		}
		expires = &parsed
	}
	item, err := router.Admin.Console.CreateDeviceBan(request.Context(), router.AdminApplicationID(request), body.DeviceID, strings.TrimSpace(body.Reason), expires)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "DEVICE_BANNED", "device_ban", item.ID, nil)
	httpapi.WriteJSON(writer, http.StatusCreated, map[string]any{"ok": true, "ban": deviceBanJSON(*item)})
}
func (router *Router) handleDeviceBanLift(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, banID string) {
	var body struct {
		Reason string `json:"reason"`
	}
	if !httpapi.ValidUUID(banID) || httpapi.DecodeJSONBody(writer, request, &body) != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if err := router.Admin.Console.LiftDeviceBan(request.Context(), router.AdminApplicationID(request), banID, strings.TrimSpace(body.Reason)); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "DEVICE_BAN_LIFTED", "device_ban", banID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, map[string]bool{"ok": true})
}
