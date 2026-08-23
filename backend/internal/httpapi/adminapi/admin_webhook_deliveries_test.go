package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
)

type fakeWebhookDeliveryConsole struct {
	httpapi.AdminConsoleStore
	listApp       string
	listWebhook   string
	retryApp      string
	retryWebhook  string
	retryDelivery string
	deliveries    []domain.WebhookDelivery
	total         int64
	enqueued      []domain.WebhookDelivery
	webhooks      []domain.Webhook
}

func (fake *fakeWebhookDeliveryConsole) AppendAuditLog(context.Context, domain.NewAuditLog) error {
	return nil
}

func (fake *fakeWebhookDeliveryConsole) AppendSecurityEvent(context.Context, domain.NewSecurityEvent) error {
	return nil
}

func (fake *fakeWebhookDeliveryConsole) ListWebhookDeliveries(_ context.Context, applicationID, webhookID string, _, _ int) ([]domain.WebhookDelivery, int64, error) {
	fake.listApp = applicationID
	fake.listWebhook = webhookID
	return fake.deliveries, fake.total, nil
}

func (fake *fakeWebhookDeliveryConsole) RetryWebhookDelivery(_ context.Context, applicationID, webhookID, deliveryID string) error {
	fake.retryApp = applicationID
	fake.retryWebhook = webhookID
	fake.retryDelivery = deliveryID
	return nil
}

func (fake *fakeWebhookDeliveryConsole) ListWebhooks(context.Context, string) ([]domain.Webhook, error) {
	return fake.webhooks, nil
}

func (fake *fakeWebhookDeliveryConsole) EnqueueWebhookEvent(_ context.Context, webhookID, eventType string, payload json.RawMessage) error {
	fake.enqueued = append(fake.enqueued, domain.WebhookDelivery{WebhookID: webhookID, EventType: eventType, Payload: payload})
	return nil
}

func TestAdminWebhookDeliveriesListAndRetry(t *testing.T) {
	deliveredAt := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	console := &fakeWebhookDeliveryConsole{
		deliveries: []domain.WebhookDelivery{
			{
				ID: "delivery-1", WebhookID: "webhook-1", EventType: "user.banned",
				Status: domain.WebhookDeliveryStatusDelivered, MaxAttempts: 6,
				DeliveredAt: &deliveredAt,
				CreatedAt:   time.Date(2026, 8, 23, 9, 30, 0, 0, time.UTC),
			},
		},
		total: 1,
	}
	router := newListTestRouter(t, console)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, credentialAdminRequest(t, router, http.MethodGet, "/v1/admin/webhooks/webhook-1/deliveries?page=2&page_size=20", ""))

	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if console.listApp != "019c1111-1111-7111-8111-111111111111" || console.listWebhook != "webhook-1" {
		t.Fatalf("list args = (%q, %q)", console.listApp, console.listWebhook)
	}
	var listResponse struct {
		OK    bool `json:"ok"`
		Items []struct {
			ID          string `json:"id"`
			Status      string `json:"status"`
			Attempts    int    `json:"attempts"`
			MaxAttempts int    `json:"max_attempts"`
			LastError   string `json:"last_error"`
			DeliveredAt string `json:"delivered_at"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &listResponse); err != nil {
		t.Fatal(err)
	}
	if !listResponse.OK || len(listResponse.Items) != 1 || listResponse.Items[0].Status != "delivered" || listResponse.Total != 1 {
		t.Fatalf("list response = %#v", listResponse)
	}
	if listResponse.Items[0].DeliveredAt == "" {
		t.Fatal("delivered_at not serialized")
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, credentialAdminRequest(t, router, http.MethodPost, "/v1/admin/webhooks/webhook-1/deliveries/delivery-1/retry", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("retry status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if console.retryApp != "019c1111-1111-7111-8111-111111111111" || console.retryWebhook != "webhook-1" || console.retryDelivery != "delivery-1" {
		t.Fatalf("retry args = (%q, %q, %q)", console.retryApp, console.retryWebhook, console.retryDelivery)
	}
}

func TestEmitWebhookEventFansOutToMatchingActiveWebhooks(t *testing.T) {
	console := &fakeWebhookDeliveryConsole{
		webhooks: []domain.Webhook{
			{ID: "match", ApplicationID: "app", Status: domain.WebhookStatusActive, Events: []string{"license.*"}},
			{ID: "other-events", ApplicationID: "app", Status: domain.WebhookStatusActive, Events: []string{"user.created"}},
			{ID: "disabled", ApplicationID: "app", Status: domain.WebhookStatusDisabled, Events: []string{"license.*"}},
		},
	}
	router := newListTestRouter(t, console)
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/licenses", nil)

	router.EmitWebhookEvent(request, "license.created", map[string]any{"license_id": "lic-9"})

	if len(console.enqueued) != 1 {
		t.Fatalf("enqueued %d events, want 1: %#v", len(console.enqueued), console.enqueued)
	}
	if console.enqueued[0].WebhookID != "match" || console.enqueued[0].EventType != "license.created" {
		t.Fatalf("enqueued event = %#v", console.enqueued[0])
	}
	var payload map[string]any
	if err := json.Unmarshal(console.enqueued[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["license_id"] != "lic-9" {
		t.Fatalf("payload = %#v", payload)
	}
}
