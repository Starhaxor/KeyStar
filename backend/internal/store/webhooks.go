package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/starloader/backend/internal/domain"
)

const webhookColumns = `
	id::text, application_id::text, url, secret_hash, status, events,
	created_at, updated_at`

const webhookDeliveryColumns = `
	d.id::text, d.webhook_id::text, d.event_type, d.payload, d.status, d.attempts,
	d.max_attempts, d.next_attempt_at, d.last_error, d.delivered_at, d.created_at`

// CreateWebhook stores a new webhook endpoint.
func (s *Store) CreateWebhook(ctx context.Context, applicationID string, input domain.NewWebhook, secretHash []byte) (*domain.Webhook, error) {
	wh, err := scanWebhook(s.db.QueryRow(ctx, `
		insert into webhooks (application_id, url, secret_hash, events)
		values ($1::uuid, $2, $3, $4)
		returning `+webhookColumns,
		applicationID, input.URL, secretHash, input.Events))
	if err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}
	return wh, nil
}

// ListWebhooks returns all webhooks for an application.
func (s *Store) ListWebhooks(ctx context.Context, applicationID string) ([]domain.Webhook, error) {
	rows, err := s.db.Query(ctx,
		`select `+webhookColumns+` from webhooks where application_id = $1::uuid order by created_at`,
		applicationID)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	defer rows.Close()
	var whs []domain.Webhook
	for rows.Next() {
		wh, err := scanWebhook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan webhook: %w", err)
		}
		whs = append(whs, *wh)
	}
	return whs, rows.Err()
}

// FindWebhookByID looks up a webhook by ID.
func (s *Store) FindWebhookByID(ctx context.Context, applicationID, webhookID string) (*domain.Webhook, error) {
	wh, err := scanWebhook(s.db.QueryRow(ctx,
		`select `+webhookColumns+` from webhooks where application_id = $1::uuid and id = $2::uuid`,
		applicationID, webhookID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrVariableNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find webhook: %w", err)
	}
	return wh, nil
}

// UpdateWebhook updates URL, status, and/or events of a webhook.
func (s *Store) UpdateWebhook(ctx context.Context, applicationID, webhookID string, url *string, status *domain.WebhookStatus, events *[]string) error {
	setClauses := []string{}
	args := []any{applicationID, webhookID}
	argIdx := 3
	if url != nil {
		setClauses = append(setClauses, fmt.Sprintf("url = $%d", argIdx))
		args = append(args, *url)
		argIdx++
	}
	if status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *status)
		argIdx++
	}
	if events != nil {
		setClauses = append(setClauses, fmt.Sprintf("events = $%d", argIdx))
		args = append(args, *events)
		argIdx++
	}
	if len(setClauses) == 0 {
		return nil
	}
	setClauses = append(setClauses, "updated_at = now()")

	query := fmt.Sprintf("update webhooks set %s where application_id = $1::uuid and id = $2::uuid returning id", joinStrings(setClauses, ", "))
	var id string
	err := s.db.QueryRow(ctx, query, args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrVariableNotFound
	}
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}
	return nil
}

// DeleteWebhook removes a webhook.
func (s *Store) DeleteWebhook(ctx context.Context, applicationID, webhookID string) error {
	var id string
	err := s.db.QueryRow(ctx,
		`delete from webhooks where application_id = $1::uuid and id = $2::uuid returning id`,
		applicationID, webhookID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // idempotent
	}
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	return nil
}

// EnqueueWebhookEvent writes an event to the outbox for delivery.
// QueryRow + returning keeps the write on one round trip; a bare Query would
// leak a pooled connection per insert because its rows are discarded.
func (s *Store) EnqueueWebhookEvent(ctx context.Context, webhookID, eventType string, payload json.RawMessage) error {
	var id string
	err := s.db.QueryRow(ctx, `
		insert into webhook_deliveries (webhook_id, event_type, payload)
		values ($1::uuid, $2, $3)
		returning id`, webhookID, eventType, payload).Scan(&id)
	if err != nil {
		return fmt.Errorf("enqueue webhook event: %w", err)
	}
	return nil
}

// DequeuePendingDeliveries fetches the next batch of pending/failed deliveries.
func (s *Store) DequeuePendingDeliveries(ctx context.Context, limit int) ([]domain.WebhookDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		select `+webhookDeliveryColumns+`
		from webhook_deliveries d
		where d.status in ('pending', 'failed') and d.next_attempt_at <= now()
		order by d.next_attempt_at
		limit $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("dequeue pending deliveries: %w", err)
	}
	defer rows.Close()
	var deliveries []domain.WebhookDelivery
	for rows.Next() {
		d, err := scanWebhookDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("scan webhook delivery: %w", err)
		}
		deliveries = append(deliveries, *d)
	}
	return deliveries, rows.Err()
}

// MarkDeliveryDelivered marks a delivery as successfully delivered.
func (s *Store) MarkDeliveryDelivered(ctx context.Context, deliveryID string) error {
	var id string
	err := s.db.QueryRow(ctx, `
		update webhook_deliveries
		set status = 'delivered', delivered_at = now()
		where id = $1::uuid returning id`, deliveryID).Scan(&id)
	if err != nil {
		return fmt.Errorf("mark delivery delivered: %w", err)
	}
	return nil
}

// MarkDeliveryFailed increments attempts and schedules the next retry with
// exponential backoff: 1m, 5m, 15m, 1h, 6h, 24h.
func (s *Store) MarkDeliveryFailed(ctx context.Context, deliveryID string, errMsg string) error {
	var id string
	err := s.db.QueryRow(ctx, `
		update webhook_deliveries
		set status = case when attempts + 1 >= max_attempts then 'failed' else 'pending' end,
		    attempts = attempts + 1,
		    last_error = $2,
		    next_attempt_at = now() + interval '1 minute' * CASE
		        WHEN attempts + 1 >= max_attempts THEN 0
		        WHEN attempts = 0 THEN 1
		        WHEN attempts = 1 THEN 5
		        WHEN attempts = 2 THEN 15
		        WHEN attempts = 3 THEN 60
		        WHEN attempts = 4 THEN 360
		        ELSE 1440
		    END
		where id = $1::uuid returning id`, deliveryID, errMsg).Scan(&id)
	if err != nil {
		return fmt.Errorf("mark delivery failed: %w", err)
	}
	return nil
}

// CountOutboxBacklog returns pending + failed deliveries. The worker exposes
// it as the webhook backlog gauge.
func (s *Store) CountOutboxBacklog(ctx context.Context) (int64, error) {
	var backlog int64
	err := s.db.QueryRow(ctx, `
		select count(*) from webhook_deliveries where status in ('pending', 'failed')`).Scan(&backlog)
	if err != nil {
		return 0, fmt.Errorf("count outbox backlog: %w", err)
	}
	return backlog, nil
}

// FindWebhookForDelivery looks up a webhook by ID without application
// scoping. Only the trusted delivery worker uses it.
func (s *Store) FindWebhookForDelivery(ctx context.Context, webhookID string) (*domain.Webhook, error) {
	wh, err := scanWebhook(s.db.QueryRow(ctx,
		`select `+webhookColumns+` from webhooks where id = $1::uuid`, webhookID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrVariableNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find webhook for delivery: %w", err)
	}
	return wh, nil
}

// ListWebhookDeliveries pages the delivery history of one application's
// webhook, newest first.
func (s *Store) ListWebhookDeliveries(ctx context.Context, applicationID, webhookID string, offset, limit int) ([]domain.WebhookDelivery, int64, error) {
	var total int64
	if err := s.db.QueryRow(ctx, `
		select count(*)
		from webhook_deliveries d
		join webhooks w on w.id = d.webhook_id
		where w.application_id = $1::uuid and w.id = $2::uuid`,
		applicationID, webhookID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count webhook deliveries: %w", err)
	}
	rows, err := s.db.Query(ctx, `
		select `+webhookDeliveryColumns+`
		from webhook_deliveries d
		join webhooks w on w.id = d.webhook_id
		where w.application_id = $1::uuid and w.id = $2::uuid
		order by d.created_at desc, d.id desc
		limit $3 offset $4`, applicationID, webhookID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list webhook deliveries: %w", err)
	}
	defer rows.Close()
	deliveries := make([]domain.WebhookDelivery, 0, limit)
	for rows.Next() {
		delivery, err := scanWebhookDelivery(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan webhook delivery: %w", err)
		}
		deliveries = append(deliveries, *delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list webhook deliveries: %w", err)
	}
	return deliveries, total, nil
}

// RetryWebhookDelivery requeues a failed or already delivered event with a
// fresh attempt budget. The webhook scoping keeps the operation inside one
// application boundary.
func (s *Store) RetryWebhookDelivery(ctx context.Context, applicationID, webhookID, deliveryID string) error {
	var id string
	err := s.db.QueryRow(ctx, `
		update webhook_deliveries d
		set status = 'pending', attempts = 0, next_attempt_at = now(), last_error = null, delivered_at = null
		from webhooks w
		where w.id = d.webhook_id and w.application_id = $1::uuid and w.id = $2::uuid
		  and d.id = $3::uuid and d.status in ('failed', 'delivered')
		returning d.id`,
		applicationID, webhookID, deliveryID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrVariableNotFound
	}
	if err != nil {
		return fmt.Errorf("retry webhook delivery: %w", err)
	}
	return nil
}

func scanWebhook(row pgx.Row) (*domain.Webhook, error) {
	var wh domain.Webhook
	err := row.Scan(
		&wh.ID, &wh.ApplicationID, &wh.URL, &wh.SecretHash,
		&wh.Status, &wh.Events, &wh.CreatedAt, &wh.UpdatedAt,
	)
	return &wh, err
}

func scanWebhookDelivery(row pgx.Row) (*domain.WebhookDelivery, error) {
	var d domain.WebhookDelivery
	var lastError *string
	var deliveredAt *time.Time
	err := row.Scan(
		&d.ID, &d.WebhookID, &d.EventType, &d.Payload,
		&d.Status, &d.Attempts, &d.MaxAttempts, &d.NextAttemptAt,
		&lastError, &deliveredAt, &d.CreatedAt,
	)
	if lastError != nil {
		d.LastError = *lastError
	}
	if deliveredAt != nil {
		d.DeliveredAt = deliveredAt
	}
	return &d, err
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
