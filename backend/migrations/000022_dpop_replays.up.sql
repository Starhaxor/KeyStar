create table dpop_replays (
    application_id uuid not null references applications(id) on delete cascade,
    jti_digest bytea not null check (octet_length(jti_digest) = 32),
    token_id text not null check (char_length(token_id) > 0),
    expires_at timestamptz not null,
    created_at timestamptz not null default clock_timestamp(),
    primary key (application_id, jti_digest)
);

create index dpop_replays_expires_at_idx on dpop_replays (expires_at);
