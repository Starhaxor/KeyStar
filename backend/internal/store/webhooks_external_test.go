package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starloader/backend/internal/domain"
)

// TestExternalWebhookDeliveryPipeline exercises the outbox end to end:
// create endpoint, enqueue events, page the delivery history and requeue a
// terminal failure through the console retry path.
func TestExternalWebhookDeliveryPipeline(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	repository := New(pool)
	application, err := repository.FindApplicationBySlug(ctx, "starloader")
	if err != nil {
		t.Fatal(err)
	}

	secret := domain.HashWebhookSecret([]byte("external-pipeline-secret"))
	webhook, err := repository.CreateWebhook(ctx, application.ID, domain.NewWebhook{
		URL: "https://example.invalid/hook", Events: []string{"license.*"},
	}, secret)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = repository.DeleteWebhook(context.Background(), application.ID, webhook.ID)
	})

	for _, eventType := range []string{"license.created", "license.revoked"} {
		if err := repository.EnqueueWebhookEvent(ctx, webhook.ID, eventType, []byte(`{"id":"x"}`)); err != nil {
			t.Fatal(err)
		}
	}

	due, err := repository.DequeuePendingDeliveries(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, delivery := range due {
		if delivery.WebhookID == webhook.ID {
			found++
			// Drive one delivery into the terminal failed state.
			for i := 0; i < 6; i++ {
				if err := repository.MarkDeliveryFailed(ctx, delivery.ID, "simulated outage"); err != nil {
					t.Fatal(err)
				}
				row, err := repository.FindWebhookForDelivery(ctx, webhook.ID)
				if err != nil || row.URL != "https://example.invalid/hook" {
					t.Fatalf("FindWebhookForDelivery = (%#v, %v)", row, err)
				}
			}
		}
	}
	if found != 2 {
		t.Fatalf("expected both fixture deliveries due, got %d", found)
	}

	deliveries, total, err := repository.ListWebhookDeliveries(ctx, application.ID, webhook.ID, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(deliveries) != 2 {
		t.Fatalf("delivery history = (%d rows, total %d)", len(deliveries), total)
	}
	var failedID string
	for _, delivery := range deliveries {
		if delivery.Status != domain.WebhookDeliveryStatusFailed {
			t.Fatalf("unexpected status %q after exhausting retries", delivery.Status)
		}
		if failedID == "" || delivery.ID < failedID {
			failedID = delivery.ID
		}
	}

	if err := repository.RetryWebhookDelivery(ctx, application.ID, webhook.ID, failedID); err != nil {
		t.Fatal(err)
	}
	requeued, _, err := repository.ListWebhookDeliveries(ctx, application.ID, webhook.ID, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var pending int
	for _, delivery := range requeued {
		if delivery.Status == domain.WebhookDeliveryStatusPending && delivery.Attempts == 0 {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("retry did not requeue exactly one delivery, got %d", pending)
	}

	// A retry against another webhook's ID must not cross the boundary.
	if err := repository.RetryWebhookDelivery(ctx, application.ID, "00000000-0000-0000-0000-000000000000", failedID); err == nil {
		t.Fatal("cross-webhook retry should fail")
	}
}
