package store

import (
	"context"
	"fmt"
	"time"
)

// AllowRateLimit atomically increments a shared PostgreSQL rate-limit bucket.
func (s *Store) AllowRateLimit(ctx context.Context, key []byte, limit int, window time.Duration, now time.Time) (bool, int, error) {
	if limit <= 0 || window <= 0 {
		return false, 0, fmt.Errorf("invalid rate limit")
	}
	var count int
	var expires time.Time
	err := s.db.QueryRow(ctx, `
		with cleanup as (
			delete from rate_limit_buckets where expires_at < $2::timestamptz - interval '5 minutes'
		)
		insert into rate_limit_buckets (bucket_key, request_count, window_started_at, expires_at)
		values ($1, 1, $2::timestamptz, $2::timestamptz + $3::double precision * interval '1 second')
		on conflict (bucket_key) do update set
			request_count = case when rate_limit_buckets.expires_at <= $2::timestamptz then 1 else rate_limit_buckets.request_count + 1 end,
			window_started_at = case when rate_limit_buckets.expires_at <= $2::timestamptz then $2::timestamptz else rate_limit_buckets.window_started_at end,
			expires_at = case when rate_limit_buckets.expires_at <= $2::timestamptz then $2::timestamptz + $3::double precision * interval '1 second' else rate_limit_buckets.expires_at end
		returning request_count, expires_at`, key, now.UTC(), int64(window/time.Second)).Scan(&count, &expires)
	if err != nil {
		return false, 0, fmt.Errorf("apply rate limit: %w", err)
	}
	retry := int(expires.Sub(now).Seconds()) + 1
	if retry < 1 {
		retry = 1
	}
	return count <= limit, retry, nil
}
