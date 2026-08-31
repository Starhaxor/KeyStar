create table application_signing_keys (
    id uuid primary key default starloader_uuid_v7()
        check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    kid text not null unique check (kid ~ '^ksk_[A-Za-z0-9_-]{22}$'),
    application_id uuid not null references applications(id) on delete cascade,
    algorithm text not null check (algorithm = 'Ed25519'),
    public_key bytea not null check (octet_length(public_key) = 32),
    encrypted_private_key bytea not null check (octet_length(encrypted_private_key) = 48),
    encryption_nonce bytea not null check (octet_length(encryption_nonce) = 12),
    encryption_key_version integer not null check (encryption_key_version > 0),
    status text not null check (status in ('pending','active','retiring','revoked')),
    created_at timestamptz not null default clock_timestamp(),
    activated_at timestamptz,
    retire_at timestamptz,
    revoked_at timestamptz,
    check (
        (status = 'pending' and activated_at is null and retire_at is null and revoked_at is null) or
        (status = 'active' and activated_at is not null and retire_at is null and revoked_at is null) or
        (status = 'retiring' and activated_at is not null and retire_at is not null and revoked_at is null) or
        (status = 'revoked' and revoked_at is not null)
    )
);

create unique index application_signing_keys_one_active_idx
    on application_signing_keys(application_id) where status = 'active';
