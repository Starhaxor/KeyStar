-- Phase 2: Application credentials.
--
-- Publishable (ks_pk_) and secret (ks_sk_) keys authenticate application
-- requests. Only the key prefix (locator) and the SHA-256 digest of the
-- random secret are stored; the full key is never persisted.

create table application_credentials (
    id uuid primary key default starloader_uuid_v7()
        constraint application_credentials_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    application_id uuid not null references applications(id) on delete cascade,
    environment text not null
        constraint application_credentials_environment_check check (environment in ('test', 'live')),
    credential_type text not null
        constraint application_credentials_type_check check (credential_type in ('publishable', 'secret')),
    name text not null
        constraint application_credentials_name_not_empty_check check (btrim(name) <> ''),
    key_prefix text not null
        constraint application_credentials_key_prefix_format_check check (key_prefix ~ '^ks_(pk|sk)_(test|live)_[0-9A-Z]{10}$'),
    key_hash bytea not null
        constraint application_credentials_key_hash_length_check check (octet_length(key_hash) = 32),
    scopes text[] not null default '{}',
    status text not null default 'active'
        constraint application_credentials_status_check check (status in ('active', 'revoked')),
    last_used_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz not null default now(),
    revoked_at timestamptz,
    constraint application_credentials_key_prefix_unique unique (key_prefix),
    constraint application_credentials_revoked_after_created_check check (revoked_at is null or revoked_at >= created_at)
);

create index application_credentials_application_id_idx on application_credentials (application_id);

-- The owner role gains credential management; viewers stay read-only.
update roles
set permissions = permissions || '{credentials.read,credentials.write}'
where name = 'owner';
