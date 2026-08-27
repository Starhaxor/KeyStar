package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/starloader/backend/internal/store"
)

func TestPostgresRateLimitBucketIsAtomicAndExpires(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	key := []byte("hashed-security-boundary")
	for attempt := 1; attempt <= 3; attempt++ {
		allowed, retry, err := repository.AllowRateLimit(ctx, key, 2, time.Minute, now)
		if err != nil {
			t.Fatal(err)
		}
		if allowed != (attempt <= 2) || retry < 1 {
			t.Fatalf("attempt=%d allowed=%v retry=%d", attempt, allowed, retry)
		}
	}
	allowed, _, err := repository.AllowRateLimit(ctx, key, 2, time.Minute, now.Add(time.Minute))
	if err != nil || !allowed {
		t.Fatalf("expired bucket did not reset: allowed=%v err=%v", allowed, err)
	}
}
