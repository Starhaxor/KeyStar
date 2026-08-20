-- Phase 9: Webhooks
-- Enables developers to receive event notifications on their own backends.

create table webhooks (
    id uuid primary key default starloader_uuid_v7()
        constraint webhooks_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    application_id uuid not null references applications(id) on delete cascade,
    url text not null,
    -- SHA-256 of the webhook signing secret (plaintext never stored).
    secret_hash bytea not null,
    status text not null default 'active'
        constraint webhooks_status_check check (status in ('active', 'disabled')),
    -- Array of event type patterns this webhook subscribes to.
    -- Example: '{"user.created", "license.*"}'
    events text[] not null default '{}',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index idx_webhooks_application on webhooks(application_id);

-- Outbox table for webhook event delivery. Events are written here first,
-- then delivered by a background worker. Retried with exponential backoff.
create table webhook_deliveries (
    id uuid primary key default starloader_uuid_v7()
        constraint webhook_deliveries_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    webhook_id uuid not null references webhooks(id) on delete cascade,
    event_type text not null,
    payload jsonb not null,
    status text not null default 'pending'
        constraint webhook_deliveries_status_check check (status in ('pending', 'delivering', 'delivered', 'failed')),
    attempts integer not null default 0,
    max_attempts integer not null default 6,
    next_attempt_at timestamptz not null default now(),
    last_error text,
    delivered_at timestamptz,
    created_at timestamptz not null default now()
);

create index idx_webhook_deliveries_pending on webhook_deliveries(status, next_attempt_at)
    where status in ('pending', 'failed');
create index idx_webhook_deliveries_webhook on webhook_deliveries(webhook_id);
