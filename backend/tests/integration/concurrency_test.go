package integration_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/service"
	"github.com/starloader/backend/internal/store"
)

func TestConcurrentChallengeConsumptionHasExactlyOneConsumer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	base := time.Now().UTC().Truncate(time.Microsecond)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	pending, applicationID := createPendingSession(t, ctx, repository, base)

	firstConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire first connection: %v", err)
	}
	defer firstConn.Release()
	secondConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire second connection: %v", err)
	}
	defer secondConn.Release()
	firstRepository := store.New(firstConn)
	secondRepository := store.New(secondConn)

	var secondBackendPID int
	if err := secondConn.QueryRow(ctx, `select pg_backend_pid()`).Scan(&secondBackendPID); err != nil {
		t.Fatalf("read second backend PID: %v", err)
	}

	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	defer release()
	secondStarted := make(chan struct{})
	secondCallbackRan := make(chan struct{}, 1)
	results := make(chan error, 2)
	go func() {
		results <- firstRepository.WithLockedChallenge(ctx, applicationID, pending.Session.ID, func(*store.LockedChallenge) error {
			close(firstLocked)
			select {
			case <-releaseFirst:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	waitForSignal(t, ctx, firstLocked, "first callback to acquire the challenge lock")

	go func() {
		close(secondStarted)
		results <- secondRepository.WithLockedChallenge(ctx, applicationID, pending.Session.ID, func(*store.LockedChallenge) error {
			secondCallbackRan <- struct{}{}
			return nil
		})
	}()
	waitForSignal(t, ctx, secondStarted, "second transaction to start")
	waitForBackendLock(t, ctx, pool, secondBackendPID)
	release()

	succeeded := 0
	consumed := 0
	for range 2 {
		err := receiveResult(t, ctx, results)
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrChallengeConsumed):
			consumed++
		default:
			t.Fatalf("WithLockedChallenge() error = %v", err)
		}
	}
	if succeeded != 1 || consumed != 1 {
		t.Fatalf("challenge results: succeeded=%d consumed=%d", succeeded, consumed)
	}
	select {
	case <-secondCallbackRan:
		t.Fatal("second callback ran after the first transaction consumed the challenge")
	default:
	}
	assertChallengeConsumedAfterCreation(t, ctx, pool, pending.Challenge)
}

func TestWithLockedChallengeRollsBackCallbackFailure(t *testing.T) {
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Microsecond)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	pending, applicationID := createPendingSession(t, ctx, repository, base)
	callbackErr := errors.New("verification failed")

	err := repository.WithLockedChallenge(ctx, applicationID, pending.Session.ID, func(*store.LockedChallenge) error {
		return callbackErr
	})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("WithLockedChallenge() error = %v, want %v", err, callbackErr)
	}

	var consumedAt *time.Time
	if err := pool.QueryRow(ctx, `select consumed_at from device_challenges where id = $1`, pending.Challenge.ID).Scan(&consumedAt); err != nil {
		t.Fatalf("read rolled-back consumed_at: %v", err)
	}
	if consumedAt != nil {
		t.Fatalf("failed callback persisted consumed_at %s", consumedAt)
	}

	err = repository.WithLockedChallenge(ctx, applicationID, pending.Session.ID, func(locked *store.LockedChallenge) error {
		if locked.Challenge.ConsumedAt != nil {
			return domain.ErrChallengeConsumed
		}
		return nil
	})
	if err != nil {
		t.Fatalf("second WithLockedChallenge() error = %v", err)
	}
	assertChallengeConsumedAfterCreation(t, ctx, pool, pending.Challenge)
}

func TestSuccessfulLockedChallengeCallbackAlwaysConsumes(t *testing.T) {
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Microsecond)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	pending, applicationID := createPendingSession(t, ctx, repository, base)

	if err := repository.WithLockedChallenge(ctx, applicationID, pending.Session.ID, func(*store.LockedChallenge) error {
		return nil
	}); err != nil {
		t.Fatalf("first WithLockedChallenge() error = %v", err)
	}

	callbackCalled := false
	err := repository.WithLockedChallenge(ctx, applicationID, pending.Session.ID, func(*store.LockedChallenge) error {
		callbackCalled = true
		return nil
	})
	if !errors.Is(err, domain.ErrChallengeConsumed) {
		t.Fatalf("second WithLockedChallenge() error = %v, want %v", err, domain.ErrChallengeConsumed)
	}
	if callbackCalled {
		t.Fatal("callback ran for an already-consumed challenge")
	}

	assertChallengeConsumedAfterCreation(t, ctx, pool, pending.Challenge)
}

func TestLockedChallengeIDMutationCannotRedirectConsumption(t *testing.T) {
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Microsecond)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	original, applicationID := createPendingSession(t, ctx, repository, base)
	second, err := repository.CreatePendingSession(ctx, applicationID, domain.NewPendingSession{
		ApplicationID:   applicationID,
		UserID:          original.Session.UserID,
		LicenseID:       original.Session.LicenseID,
		ChallengeSHA256: bytes.Repeat([]byte{0x6b}, 32),
		ExpiresAt:       base.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreatePendingSession() second error = %v", err)
	}

	err = repository.WithLockedChallenge(ctx, applicationID, original.Session.ID, func(locked *store.LockedChallenge) error {
		locked.Challenge.ID = second.Challenge.ID
		return nil
	})
	if err != nil {
		t.Fatalf("WithLockedChallenge() error = %v", err)
	}

	originalConsumedAt := readChallengeConsumedAt(t, ctx, pool, original.Challenge.ID)
	secondConsumedAt := readChallengeConsumedAt(t, ctx, pool, second.Challenge.ID)
	if originalConsumedAt == nil {
		t.Error("original locked challenge remained unconsumed")
	}
	if secondConsumedAt != nil {
		t.Errorf("callback-selected challenge was consumed at %s", *secondConsumedAt)
	}

	replayCallbackRan := false
	err = repository.WithLockedChallenge(ctx, applicationID, original.Session.ID, func(*store.LockedChallenge) error {
		replayCallbackRan = true
		return nil
	})
	if !errors.Is(err, domain.ErrChallengeConsumed) {
		t.Errorf("replay error = %v, want %v", err, domain.ErrChallengeConsumed)
	}
	if replayCallbackRan {
		t.Error("replay callback ran for the original challenge")
	}
}

func waitForBackendLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, backendPID int) {
	t.Helper()
	const lockedChallengeQueryMarker = "/* starloader:with-locked-challenge */"
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()

	for {
		var waiting bool
		err := pool.QueryRow(ctx, `
			select exists (
				select 1
				from pg_stat_activity
				where pid = $1
				  and wait_event_type = 'Lock'
				  and query like '%' || $2 || '%'
			)`, backendPID, lockedChallengeQueryMarker).Scan(&waiting)
		if err != nil {
			t.Fatalf("inspect second backend lock wait: %v", err)
		}
		if waiting {
			return
		}
		select {
		case <-poll.C:
		case <-ctx.Done():
			t.Fatalf("second backend %d never reported a marked Lock wait: %v", backendPID, ctx.Err())
		}
	}
}

func waitForBackendQueryLockOrCompletion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, backendPID int, marker string, completed <-chan error) {
	t.Helper()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()

	for {
		select {
		case err := <-completed:
			t.Fatalf("query completed before the verification decision released its device rows: %v", err)
		default:
		}
		var waiting bool
		if err := pool.QueryRow(ctx, `
			select exists (
				select 1 from pg_stat_activity
				where pid = $1 and wait_event_type = 'Lock' and query like '%' || $2 || '%'
			)`, backendPID, marker).Scan(&waiting); err != nil {
			t.Fatalf("inspect device-row lock wait: %v", err)
		}
		if waiting {
			return
		}
		select {
		case <-poll.C:
		case err := <-completed:
			t.Fatalf("query completed before the verification decision released its device rows: %v", err)
		case <-ctx.Done():
			t.Fatalf("backend %d never reported device-row lock wait: %v", backendPID, ctx.Err())
		}
	}
}

func assertChallengeConsumedAfterCreation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, challenge domain.DeviceChallenge) {
	t.Helper()
	consumedAt := readChallengeConsumedAt(t, ctx, pool, challenge.ID)
	if consumedAt == nil {
		t.Fatal("challenge remained unconsumed")
	}
	if consumedAt.Before(challenge.CreatedAt) {
		t.Fatalf("consumed_at %s is before created_at %s", consumedAt, challenge.CreatedAt)
	}
}

func readChallengeConsumedAt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, challengeID string) *time.Time {
	t.Helper()
	var consumedAt *time.Time
	if err := pool.QueryRow(ctx, `select consumed_at from device_challenges where id = $1`, challengeID).Scan(&consumedAt); err != nil {
		t.Fatalf("read consumed_at: %v", err)
	}
	return consumedAt
}

func assertChallengeUnconsumed(t *testing.T, fixture *postgresVerificationFixture, sessionID string) {
	t.Helper()
	var consumedAt *time.Time
	var status domain.SessionStatus
	if err := fixture.pool.QueryRow(fixture.ctx, `
		select c.consumed_at, s.status
		from device_challenges c join auth_sessions s on s.id = c.session_id
		where s.id = $1`, sessionID).Scan(&consumedAt, &status); err != nil {
		t.Fatal(err)
	}
	if consumedAt != nil || status != domain.SessionStatusPending {
		t.Fatalf("failed verification persisted consumed_at=%v status=%s", consumedAt, status)
	}
}

func assertDeviceCount(t *testing.T, fixture *postgresVerificationFixture, want int) {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, `select count(*) from devices`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("device count = %d, want %d", count, want)
	}
}

func assertNoRawHardwareInDatabase(t *testing.T, fixture *postgresVerificationFixture, hardware service.HardwareSignals) {
	t.Helper()
	var stored string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		select concat_ws('|', smbios_uuid_hmac, motherboard_serial_hmac, bios_serial_hmac,
			system_disk_serial_hmac, machine_guid_hmac, fingerprint_hmac)
		from devices limit 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{hardware.SMBIOSUUID, hardware.MotherboardSerial, hardware.BIOSSerial, hardware.SystemDiskSerial, hardware.MachineGuid, hardware.Fingerprint} {
		if raw != "" && strings.Contains(strings.ToLower(stored), strings.ToLower(raw)) {
			t.Fatalf("database contains raw hardware value %q", raw)
		}
	}
}
