package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
)

// WebhookDispatcher delivers webhook events to registered endpoints using
// the outbox pattern. It polls pending deliveries, signs the payload with
// HMAC-SHA256, and POSTs to the webhook URL with retry support.
type WebhookDispatcher struct {
	httpClient  *http.Client
	webhookRepo WebhookDeliveryRepository
	now         func() time.Time
}

// WebhookDeliveryRepository abstracts the persistence boundary for
// webhook delivery management.
type WebhookDeliveryRepository interface {
	FindWebhookByID(ctx context.Context, applicationID, webhookID string) (*domain.Webhook, error)
	FindWebhookForDelivery(ctx context.Context, webhookID string) (*domain.Webhook, error)
	DequeuePendingDeliveries(ctx context.Context, limit int) ([]domain.WebhookDelivery, error)
	MarkDeliveryDelivered(ctx context.Context, deliveryID string) error
	MarkDeliveryFailed(ctx context.Context, deliveryID string, errMsg string) error
}

// WebhookDispatcherConfig carries the dependencies for the webhook dispatcher.
type WebhookDispatcherConfig struct {
	WebhookRepo WebhookDeliveryRepository
	HTTPClient  *http.Client
	Now         func() time.Time
}

// NewWebhookDispatcher builds a WebhookDispatcher.
func NewWebhookDispatcher(config WebhookDispatcherConfig) *WebhookDispatcher {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = security.NewPublicHTTPSClient(10 * time.Second)
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &WebhookDispatcher{
		httpClient:  httpClient,
		webhookRepo: config.WebhookRepo,
		now:         now,
	}
}

// ProcessPendingDeliveries picks up pending/failed deliveries and attempts
// to deliver them. This is called periodically by the background worker in
// cmd/server. Deliveries belonging to disabled webhooks are left untouched
// so they do not burn their retry budget while the endpoint is paused.
func (dispatcher *WebhookDispatcher) ProcessPendingDeliveries(ctx context.Context) (int, error) {
	deliveries, err := dispatcher.webhookRepo.DequeuePendingDeliveries(ctx, 50)
	if err != nil {
		return 0, fmt.Errorf("dequeue deliveries: %w", err)
	}

	delivered := 0
	for _, delivery := range deliveries {
		webhook, err := dispatcher.webhookRepo.FindWebhookForDelivery(ctx, delivery.WebhookID)
		if err != nil {
			if markErr := dispatcher.webhookRepo.MarkDeliveryFailed(ctx, delivery.ID, "webhook unavailable: "+err.Error()); markErr != nil {
				return delivered, markErr
			}
			continue
		}
		if webhook.Status != domain.WebhookStatusActive {
			continue
		}
		if err := dispatcher.deliver(ctx, webhook, delivery); err != nil {
			if markErr := dispatcher.webhookRepo.MarkDeliveryFailed(ctx, delivery.ID, err.Error()); markErr != nil {
				return delivered, markErr
			}
			continue
		}
		if err := dispatcher.webhookRepo.MarkDeliveryDelivered(ctx, delivery.ID); err != nil {
			return delivered, err
		}
		delivered++
	}
	return delivered, nil
}

// deliver performs the signed HTTP POST for one outbox row.
//
// Signature scheme: the HMAC key is the stored SHA-256 digest of the
// webhook's signing secret (the plaintext is never kept server-side).
// Receivers verify by hashing their own secret once:
//
//	key      = SHA-256(signing_secret)            // 32 raw bytes
//	signed   = timestamp + "." + request_body     // timestamp = X-KeyStar-Timestamp
//	expected = hex(HMAC-SHA256(key, signed))
//
// and comparing it against the v1 value in X-KeyStar-Signature.
func (dispatcher *WebhookDispatcher) deliver(ctx context.Context, webhook *domain.Webhook, delivery domain.WebhookDelivery) error {
	if err := security.ValidatePublicHTTPSURL(webhook.URL); err != nil {
		return fmt.Errorf("unsafe webhook target: %w", err)
	}
	event := domain.WebhookEvent{
		ID:            delivery.ID,
		Type:          delivery.EventType,
		ApplicationID: webhook.ApplicationID,
		CreatedAt:     delivery.CreatedAt.UTC().Format(time.RFC3339),
		Data:          delivery.Payload,
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	timestamp := strconv.FormatInt(dispatcher.now().UTC().Unix(), 10)
	mac := hmac.New(sha256.New, webhook.SecretHash)
	mac.Write([]byte(timestamp))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-KeyStar-Event-Id", delivery.ID)
	request.Header.Set("X-KeyStar-Event-Type", delivery.EventType)
	request.Header.Set("X-KeyStar-Timestamp", timestamp)
	request.Header.Set("X-KeyStar-Signature", "t="+timestamp+",v1="+signature)

	response, err := dispatcher.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("post webhook: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	return nil
}
