package adminapi

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type webhookJSON struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Status    string   `json:"status"`
	Events    []string `json:"events"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

func mapWebhook(entry domain.Webhook) webhookJSON {
	return webhookJSON{ID: entry.ID, URL: entry.URL, Status: string(entry.Status), Events: entry.Events, CreatedAt: httpapi.FormatTime(entry.CreatedAt), UpdatedAt: httpapi.FormatTime(entry.UpdatedAt)}
}

func (router *Router) routeAdminWebhooks(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, segments []string) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if router.RequirePermission(writer, request, account, domain.PermWebhooksRead) {
			router.handleAdminWebhookList(writer, request)
		}
	case len(segments) == 1 && request.Method == http.MethodPost:
		if router.RequirePermission(writer, request, account, domain.PermWebhooksWrite) {
			router.handleAdminWebhookCreate(writer, request, account)
		}
	case len(segments) == 2 && segments[1] != "" && request.Method == http.MethodPatch:
		if router.RequirePermission(writer, request, account, domain.PermWebhooksWrite) {
			router.handleAdminWebhookUpdate(writer, request, account, segments[1])
		}
	case len(segments) == 2 && segments[1] != "" && request.Method == http.MethodDelete:
		if router.RequirePermission(writer, request, account, domain.PermWebhooksWrite) {
			router.handleAdminWebhookDelete(writer, request, account, segments[1])
		}
	default:
		httpapi.WriteError(writer, request, http.StatusNotFound, "INVALID_REQUEST", "not found")
	}
}

func validateWebhookEvents(writer http.ResponseWriter, request *http.Request, events []string) bool {
	if len(events) == 0 {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "at least one event is required")
		return false
	}
	for _, event := range events {
		if event != "*" && !domain.ValidWebhookEvents[event] {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid event type: "+event)
			return false
		}
	}
	return true
}

func (router *Router) handleAdminWebhookList(writer http.ResponseWriter, request *http.Request) {
	entries, err := router.Admin.Console.ListWebhooks(request.Context(), router.AdminApplicationID(request))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	items := make([]webhookJSON, 0, len(entries))
	for _, entry := range entries {
		items = append(items, mapWebhook(entry))
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool          `json:"ok"`
		Items []webhookJSON `json:"items"`
	}{OK: true, Items: items})
}

func (router *Router) handleAdminWebhookCreate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount) {
	var body struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if strings.TrimSpace(body.URL) == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "url is required")
		return
	}
	if !validateWebhookEvents(writer, request, body.Events) {
		return
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "could not generate secret")
		return
	}
	entry, err := router.Admin.Console.CreateWebhook(request.Context(), router.AdminApplicationID(request), domain.NewWebhook{URL: strings.TrimSpace(body.URL), Events: body.Events}, domain.HashWebhookSecret(secretBytes))
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "WEBHOOK_CREATED", "webhook", entry.ID, nil)
	httpapi.WriteJSON(writer, http.StatusCreated, struct {
		OK      bool        `json:"ok"`
		Webhook webhookJSON `json:"webhook"`
		Secret  string      `json:"secret"`
	}{OK: true, Webhook: mapWebhook(*entry), Secret: base64.RawURLEncoding.EncodeToString(secretBytes)})
}

func (router *Router) handleAdminWebhookUpdate(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, webhookID string) {
	var body struct {
		URL    *string   `json:"url"`
		Status *string   `json:"status"`
		Events *[]string `json:"events"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if body.URL != nil && strings.TrimSpace(*body.URL) == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "url is required")
		return
	}
	if body.Events != nil && !validateWebhookEvents(writer, request, *body.Events) {
		return
	}
	var status *domain.WebhookStatus
	if body.Status != nil {
		value := domain.WebhookStatus(*body.Status)
		if value != domain.WebhookStatusActive && value != domain.WebhookStatusDisabled {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid status")
			return
		}
		status = &value
	}
	if err := router.Admin.Console.UpdateWebhook(request.Context(), router.AdminApplicationID(request), webhookID, body.URL, status, body.Events); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	entry, err := router.Admin.Console.FindWebhookByID(request.Context(), router.AdminApplicationID(request), webhookID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "WEBHOOK_UPDATED", "webhook", webhookID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool        `json:"ok"`
		Webhook webhookJSON `json:"webhook"`
	}{OK: true, Webhook: mapWebhook(*entry)})
}

func (router *Router) handleAdminWebhookDelete(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, webhookID string) {
	if err := router.Admin.Console.DeleteWebhook(request.Context(), router.AdminApplicationID(request), webhookID); err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "WEBHOOK_DELETED", "webhook", webhookID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}
