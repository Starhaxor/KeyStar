-- Phase 6: Refresh Sessions
-- Refresh token rotation with reuse detection. Tokens are stored as SHA-256
-- hashes; the plaintext is only ever returned to the client on creation.

create table refresh_sessions (
    id uuid primary key default starloader_uuid_v7()
        constraint refresh_sessions_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    application_id uuid not null references applications(id) on delete cascade,
    user_id uuid not null references users(id) on delete cascade,
    device_id uuid not null references devices(id) on delete cascade,
    -- SHA-256 of the opaque refresh token (base64url-encoded, 32 bytes).
    token_hash bytea not null,
    -- Lifecycle: "active" → "rotated" (normal) or "active" → "revoked"
    -- (explicit logout / reuse detection).
    status text not null default 'active'
        constraint refresh_sessions_status_check check (status in ('active', 'rotated', 'revoked')),
    expires_at timestamptz not null,
    last_used_at timestamptz,
    created_at timestamptz not null default now(),
    revoked_at timestamptz
);

-- Fast lookup by token hash (the hot path on every refresh request).
create unique index idx_refresh_sessions_token_hash on refresh_sessions(token_hash);

-- Family queries: list active sessions for a user+device pair.
create index idx_refresh_sessions_user_device on refresh_sessions(user_id, device_id, status);

-- Cleanup: find expired sessions.
create index idx_refresh_sessions_expires on refresh_sessions(expires_at) where status = 'active';
