package adminapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"
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
	case len(segments) == 3 && segments[1] != "" && segments[2] == "deliveries" && request.Method == http.MethodGet:
		if router.RequirePermission(writer, request, account, domain.PermWebhooksRead) {
			router.handleAdminWebhookDeliveries(writer, request, segments[1])
		}
	case len(segments) == 5 && segments[1] != "" && segments[2] == "deliveries" && segments[4] == "retry" && request.Method == http.MethodPost:
		if router.RequirePermission(writer, request, account, domain.PermWebhooksWrite) {
			router.handleAdminWebhookDeliveryRetry(writer, request, account, segments[1], segments[3])
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

type webhookDeliveryJSON struct {
	ID            string `json:"id"`
	WebhookID     string `json:"webhook_id"`
	EventType     string `json:"event_type"`
	Status        string `json:"status"`
	Attempts      int    `json:"attempts"`
	MaxAttempts   int    `json:"max_attempts"`
	NextAttemptAt string `json:"next_attempt_at"`
	LastError     string `json:"last_error"`
	DeliveredAt   string `json:"delivered_at"`
	CreatedAt     string `json:"created_at"`
}

func mapWebhookDelivery(delivery domain.WebhookDelivery) webhookDeliveryJSON {
	deliveredAt := ""
	if delivery.DeliveredAt != nil {
		deliveredAt = httpapi.FormatTime(*delivery.DeliveredAt)
	}
	return webhookDeliveryJSON{
		ID:            delivery.ID,
		WebhookID:     delivery.WebhookID,
		EventType:     delivery.EventType,
		Status:        string(delivery.Status),
		Attempts:      delivery.Attempts,
		MaxAttempts:   delivery.MaxAttempts,
		NextAttemptAt: httpapi.FormatTime(delivery.NextAttemptAt),
		LastError:     delivery.LastError,
		DeliveredAt:   deliveredAt,
		CreatedAt:     httpapi.FormatTime(delivery.CreatedAt),
	}
}

func (router *Router) handleAdminWebhookDeliveries(writer http.ResponseWriter, request *http.Request, webhookID string) {
	page, pageSize, offset := parseAdminPagination(request)
	deliveries, total, err := router.Admin.Console.ListWebhookDeliveries(request.Context(), router.AdminApplicationID(request), webhookID, offset, pageSize)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	items := make([]webhookDeliveryJSON, 0, len(deliveries))
	for _, delivery := range deliveries {
		items = append(items, mapWebhookDelivery(delivery))
	}
	httpapi.WriteJSON(writer, http.StatusOK, adminPageResponse{OK: true, Items: items, Total: total, Page: page, PageSize: pageSize})
}

func (router *Router) handleAdminWebhookDeliveryRetry(writer http.ResponseWriter, request *http.Request, account *domain.AdminAccount, webhookID, deliveryID string) {
	err := router.Admin.Console.RetryWebhookDelivery(request.Context(), router.AdminApplicationID(request), webhookID, deliveryID)
	if err != nil {
		router.WriteConsoleError(writer, request, err)
		return
	}
	router.AuditAdmin(request, account, "WEBHOOK_DELIVERY_RETRIED", "webhook_delivery", deliveryID, nil)
	httpapi.WriteJSON(writer, http.StatusOK, struct {
		OK bool `json:"ok"`
	}{OK: true})
}

// EmitWebhookEvent fans one application event out to every active webhook
// subscribed to its type. Emission failures never fail the triggering
// mutation: the outbox is the durability boundary once a row lands.
func (router *Router) EmitWebhookEvent(request *http.Request, eventType string, data any) {
	applicationID := router.AdminApplicationID(request)
	webhooks, err := router.Admin.Console.ListWebhooks(request.Context(), applicationID)
	if err != nil {
		log.Printf("webhook emit (%s): list webhooks: %v", eventType, err)
		return
	}
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("webhook emit (%s): marshal payload: %v", eventType, err)
		return
	}
	for _, webhook := range webhooks {
		if webhook.Status != domain.WebhookStatusActive || !domain.MatchWebhookEvent(eventType, webhook.Events) {
			continue
		}
		if err := router.Admin.Console.EnqueueWebhookEvent(request.Context(), webhook.ID, eventType, payload); err != nil {
			log.Printf("webhook emit (%s): enqueue for %s: %v", eventType, webhook.ID, err)
		}
	}
}
