package main

import (
	"context"
	"log"
	"time"

	"github.com/starloader/backend/internal/service"
)

// runWebhookWorker drains the webhook delivery outbox until the application
// context is cancelled. Each tick dequeues due deliveries and POSTs them;
// failures stay in the outbox with the backoff schedule applied by the
// store, so no event is ever lost on shutdown or error.
func runWebhookWorker(ctx context.Context, dispatcher *service.WebhookDispatcher, logger *log.Logger) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			workerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			delivered, err := dispatcher.ProcessPendingDeliveries(workerCtx)
			cancel()
			if err != nil {
				logger.Printf("webhook worker: %v", err)
				continue
			}
			if delivered > 0 {
				logger.Printf("webhook worker: delivered %d event(s)", delivered)
			}
		}
	}
}
