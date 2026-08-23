package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
)

// fakeWebhookRepo records outbox transitions made by the dispatcher.
type fakeWebhookRepo struct {
	webhook    *domain.Webhook
	deliveries []domain.WebhookDelivery

	findErr      error
	markedOK     []string
	failedIDs    []string
	failedErrors []string
}

func (fake *fakeWebhookRepo) FindWebhookByID(context.Context, string, string) (*domain.Webhook, error) {
	return nil, errors.New("not used")
}

func (fake *fakeWebhookRepo) FindWebhookForDelivery(_ context.Context, webhookID string) (*domain.Webhook, error) {
	if fake.findErr != nil {
		return nil, fake.findErr
	}
	if fake.webhook == nil || fake.webhook.ID != webhookID {
		return nil, domain.ErrVariableNotFound
	}
	return fake.webhook, nil
}

func (fake *fakeWebhookRepo) DequeuePendingDeliveries(context.Context, int) ([]domain.WebhookDelivery, error) {
	return fake.deliveries, nil
}

func (fake *fakeWebhookRepo) MarkDeliveryDelivered(_ context.Context, deliveryID string) error {
	fake.markedOK = append(fake.markedOK, deliveryID)
	return nil
}

func (fake *fakeWebhookRepo) MarkDeliveryFailed(_ context.Context, deliveryID string, errMsg string) error {
	fake.failedIDs = append(fake.failedIDs, deliveryID)
	fake.failedErrors = append(fake.failedErrors, errMsg)
	return nil
}

func testDelivery() domain.WebhookDelivery {
	created := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
	return domain.WebhookDelivery{
		ID:        "delivery-1",
		WebhookID: "webhook-1",
		EventType: "license.created",
		Payload:   json.RawMessage(`{"license_id":"lic-1"}`),
		Status:    domain.WebhookDeliveryStatusPending,
		CreatedAt: created,
	}
}

func TestProcessPendingDeliveriesPostsSignedRequest(t *testing.T) {
	secret := []byte("receiver-knows-this-secret")
	hash := domain.HashWebhookSecret(secret)

	var receivedBody []byte
	var receivedHeaders http.Header
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		receivedBody, _ = io.ReadAll(request.Body)
		receivedHeaders = request.Header.Clone()
		receivedPath = request.URL.Path
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	webhook := &domain.Webhook{
		ID:            "webhook-1",
		ApplicationID: "app-1",
		URL:           server.URL + "/hook",
		SecretHash:    hash,
		Status:        domain.WebhookStatusActive,
		Events:        []string{"license.*"},
	}
	repo := &fakeWebhookRepo{webhook: webhook, deliveries: []domain.WebhookDelivery{testDelivery()}}
	dispatcher := NewWebhookDispatcher(WebhookDispatcherConfig{
		WebhookRepo: repo,
		Now: func() time.Time {
			return time.Unix(1_800_000_000, 0).UTC()
		},
	})

	delivered, err := dispatcher.ProcessPendingDeliveries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 1 || len(repo.markedOK) != 1 || repo.markedOK[0] != "delivery-1" {
		t.Fatalf("delivered=%d marked=%v failed=%v", delivered, repo.markedOK, repo.failedIDs)
	}
	if receivedPath != "/hook" || len(receivedBody) == 0 {
		t.Fatalf("request not received: path=%q body=%d", receivedPath, len(receivedBody))
	}

	// The receiver verifies by hashing its plaintext secret.
	timestamp := receivedHeaders.Get("X-KeyStar-Timestamp")
	signature := receivedHeaders.Get("X-KeyStar-Signature")
	key := sha256.Sum256(secret)
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(timestamp))
	mac.Write(receivedBody)
	expected := "t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		t.Fatalf("signature mismatch: got %q want %q", signature, expected)
	}
	if receivedHeaders.Get("X-KeyStar-Event-Type") != "license.created" {
		t.Fatalf("event type header = %q", receivedHeaders.Get("X-KeyStar-Event-Type"))
	}
	var event domain.WebhookEvent
	if err := json.Unmarshal(receivedBody, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "license.created" || event.ApplicationID != "app-1" || string(event.Data) != `{"license_id":"lic-1"}` {
		t.Fatalf("event = %#v", event)
	}
}

func TestProcessPendingDeliveriesMarksFailuresAndSkipsDisabled(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	disabledWebhook := &domain.Webhook{ID: "webhook-disabled", Status: domain.WebhookStatusDisabled, SecretHash: []byte("x")}
	first := testDelivery()
	second := testDelivery()
	second.ID = "delivery-2"
	second.WebhookID = "webhook-disabled"

	repo := &fakeWebhookRepo{
		webhook:    &domain.Webhook{ID: "webhook-1", ApplicationID: "app-1", URL: server.URL, Status: domain.WebhookStatusActive, SecretHash: domain.HashWebhookSecret([]byte("s"))},
		deliveries: []domain.WebhookDelivery{first, second},
	}
	// Second delivery resolves to the disabled webhook via its own lookup.
	repo.deliveries[1].WebhookID = "webhook-disabled"
	dispatcher := NewWebhookDispatcher(WebhookDispatcherConfig{WebhookRepo: repo})
	dispatcher.webhookRepo = &multiWebhookRepo{
		primary: repo,
		byID: map[string]*domain.Webhook{
			"webhook-1":        repo.webhook,
			"webhook-disabled": disabledWebhook,
		},
	}

	delivered, err := dispatcher.ProcessPendingDeliveries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 0 {
		t.Fatalf("delivered = %d, want 0", delivered)
	}
	if calls != 1 {
		t.Fatalf("disabled webhook was contacted %d times", calls-1+1)
	}
	if len(repo.failedIDs) != 1 || repo.failedIDs[0] != "delivery-1" {
		t.Fatalf("failed IDs = %v", repo.failedIDs)
	}
	if len(repo.failedErrors) != 1 || repo.failedErrors[0] == "" {
		t.Fatalf("failure reason not recorded: %v", repo.failedErrors)
	}
}

// multiWebhookRepo resolves lookups from a fixed table so a single fake can
// serve webhooks in different states.
type multiWebhookRepo struct {
	primary WebhookDeliveryRepository
	byID    map[string]*domain.Webhook
}

func (multi *multiWebhookRepo) FindWebhookByID(ctx context.Context, applicationID, webhookID string) (*domain.Webhook, error) {
	return multi.primary.FindWebhookByID(ctx, applicationID, webhookID)
}

func (multi *multiWebhookRepo) FindWebhookForDelivery(_ context.Context, webhookID string) (*domain.Webhook, error) {
	if webhook, ok := multi.byID[webhookID]; ok {
		return webhook, nil
	}
	return nil, domain.ErrVariableNotFound
}

func (multi *multiWebhookRepo) DequeuePendingDeliveries(ctx context.Context, limit int) ([]domain.WebhookDelivery, error) {
	return multi.primary.DequeuePendingDeliveries(ctx, limit)
}

func (multi *multiWebhookRepo) MarkDeliveryDelivered(ctx context.Context, deliveryID string) error {
	return multi.primary.MarkDeliveryDelivered(ctx, deliveryID)
}

func (multi *multiWebhookRepo) MarkDeliveryFailed(ctx context.Context, deliveryID string, errMsg string) error {
	return multi.primary.MarkDeliveryFailed(ctx, deliveryID, errMsg)
}
