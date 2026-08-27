package serverapi

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/security"
)

type serverWebhookJSON struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Status    string   `json:"status"`
	Events    []string `json:"events"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

func mapWebhook(wh domain.Webhook) serverWebhookJSON {
	return serverWebhookJSON{
		ID: wh.ID, URL: wh.URL, Status: string(wh.Status),
		Events:    wh.Events,
		CreatedAt: httpapi.FormatTime(wh.CreatedAt), UpdatedAt: httpapi.FormatTime(wh.UpdatedAt),
	}
}

// handleServerWebhookList returns all webhooks for the calling application.
func (router *Router) handleServerWebhookList(writer http.ResponseWriter, request *http.Request) {
	applicationID := principalApplicationID(request)
	webhooks, err := router.ServerStore.ListWebhooks(request.Context(), applicationID)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}
	items := make([]serverWebhookJSON, 0, len(webhooks))
	for _, wh := range webhooks {
		items = append(items, mapWebhook(wh))
	}
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK    bool                `json:"ok"`
		Items []serverWebhookJSON `json:"items"`
	}{OK: true, Items: items})
}

// handleServerWebhookCreate creates a new webhook. Returns the signing
// secret exactly once — the caller must store it securely.
func (router *Router) handleServerWebhookCreate(writer http.ResponseWriter, request *http.Request) {
	applicationID := principalApplicationID(request)
	var body struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	body.URL = strings.TrimSpace(body.URL)
	if err := security.ValidatePublicHTTPSURL(body.URL); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "url must be a public HTTPS endpoint")
		return
	}

	// Validate event types.
	for _, event := range body.Events {
		if !domain.ValidWebhookEvents[event] {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid event type: "+event)
			return
		}
	}

	// Generate signing secret (32 bytes random).
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		httpapi.WriteError(writer, request, http.StatusInternalServerError, "SERVER_ERROR", "could not generate secret")
		return
	}
	secretValue := base64.RawURLEncoding.EncodeToString(secret)
	secretHash := domain.HashWebhookSecret([]byte(secretValue))

	wh, err := router.ServerStore.CreateWebhook(request.Context(), applicationID, domain.NewWebhook{
		URL:    body.URL,
		Events: body.Events,
	}, secretHash)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}

	httpapi.WriteJSON(writer, http.StatusCreated, struct {
		OK      bool              `json:"ok"`
		Webhook serverWebhookJSON `json:"webhook"`
		Secret  string            `json:"secret"`
	}{OK: true, Webhook: mapWebhook(*wh), Secret: secretValue})
}

// handleServerWebhookUpdate updates a webhook's URL, status, or events.
func (router *Router) handleServerWebhookUpdate(writer http.ResponseWriter, request *http.Request) {
	applicationID := principalApplicationID(request)
	webhookID := serverPathID(request)
	if webhookID == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "webhook id required")
		return
	}
	if !httpapi.ValidUUID(webhookID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "WEBHOOK_NOT_FOUND", "webhook not found")
		return
	}

	var body struct {
		URL    *string   `json:"url"`
		Status *string   `json:"status"`
		Events *[]string `json:"events"`
	}
	if err := httpapi.DecodeJSONBody(writer, request, &body); err != nil {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid request")
		return
	}
	if body.URL != nil {
		trimmed := strings.TrimSpace(*body.URL)
		if err := security.ValidatePublicHTTPSURL(trimmed); err != nil {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "url must be a public HTTPS endpoint")
			return
		}
		body.URL = &trimmed
	}

	var status *domain.WebhookStatus
	if body.Status != nil {
		s := domain.WebhookStatus(*body.Status)
		if s != domain.WebhookStatusActive && s != domain.WebhookStatusDisabled {
			httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "invalid status")
			return
		}
		status = &s
	}

	if err := router.ServerStore.UpdateWebhook(request.Context(), applicationID, webhookID, body.URL, status, body.Events); err != nil {
		router.writeServerError(writer, request, err)
		return
	}

	wh, err := router.ServerStore.FindWebhookByID(request.Context(), applicationID, webhookID)
	if err != nil {
		router.writeServerError(writer, request, err)
		return
	}

	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK      bool              `json:"ok"`
		Webhook serverWebhookJSON `json:"webhook"`
	}{OK: true, Webhook: mapWebhook(*wh)})
}

// handleServerWebhookDelete deletes a webhook.
func (router *Router) handleServerWebhookDelete(writer http.ResponseWriter, request *http.Request) {
	applicationID := principalApplicationID(request)
	webhookID := serverPathID(request)
	if webhookID == "" {
		httpapi.WriteError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "webhook id required")
		return
	}
	if !httpapi.ValidUUID(webhookID) {
		httpapi.WriteError(writer, request, http.StatusNotFound, "WEBHOOK_NOT_FOUND", "webhook not found")
		return
	}

	if err := router.ServerStore.DeleteWebhook(request.Context(), applicationID, webhookID); err != nil {
		router.writeServerError(writer, request, err)
		return
	}

	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}
