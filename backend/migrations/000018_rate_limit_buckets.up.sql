create table rate_limit_buckets (
    bucket_key bytea primary key,
    request_count integer not null,
    window_started_at timestamptz not null,
    expires_at timestamptz not null
);
create index idx_rate_limit_buckets_expires on rate_limit_buckets(expires_at);
