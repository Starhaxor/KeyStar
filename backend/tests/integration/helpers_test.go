package integration_test

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/store"
	"github.com/starloader/backend/migrations"
)

func openTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL must be set for PostgreSQL integration tests")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("database ping failed: %v", err)
	}
	return pool
}

func resetAndMigrate(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "drop schema public cascade; create schema public"); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := store.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("MigrateUp() error = %v", err)
	}
}

func resetToVersionOne(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "drop schema public cascade; create schema public"); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	initialSQL, err := migrations.Files.ReadFile("000001_initial.up.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(initialSQL)); err != nil {
		t.Fatalf("execute initial migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		create table schema_migrations (
			version bigint primary key,
			applied_at timestamptz not null default clock_timestamp()
		);
		insert into schema_migrations (version) values (1)`); err != nil {
		t.Fatalf("record initial migration: %v", err)
	}
}

func assertTablesExist(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want bool) {
	t.Helper()
	for _, table := range []string{"users", "licenses", "devices", "auth_sessions", "device_challenges"} {
		var exists bool
		if err := pool.QueryRow(ctx, `select to_regclass('public.' || $1) is not null`, table).Scan(&exists); err != nil {
			t.Fatalf("look up table %s: %v", table, err)
		}
		if exists != want {
			t.Errorf("table %s exists = %t, want %t", table, exists, want)
		}
	}
}

func waitForSignal(t *testing.T, ctx context.Context, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s: %v", description, ctx.Err())
	}
}

func receiveResult(t *testing.T, ctx context.Context, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-ctx.Done():
		t.Fatalf("timed out waiting for transaction result: %v", ctx.Err())
		return nil
	}
}

func createPendingSession(t *testing.T, ctx context.Context, repository *store.Store, base time.Time) (*domain.PendingSession, string) {
	t.Helper()
	applicationID := defaultApplicationIDForTest(t, repository)
	user, license := createUserAndLicense(t, ctx, repository, "challenge@example.com", base)
	challengeSHA256 := bytes.Repeat([]byte{0x5a}, 32)
	pending, err := repository.CreatePendingSession(ctx, applicationID, domain.NewPendingSession{
		ApplicationID:   applicationID,
		UserID:          user.ID,
		LicenseID:       license.ID,
		ChallengeSHA256: challengeSHA256,
		ExpiresAt:       base.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreatePendingSession() error = %v", err)
	}
	if !bytes.Equal(pending.Challenge.ChallengeSHA256, challengeSHA256) {
		t.Fatalf("stored challenge SHA-256 = %x", pending.Challenge.ChallengeSHA256)
	}
	return pending, applicationID
}

func createUserAndLicense(t *testing.T, ctx context.Context, repository *store.Store, email string, base time.Time) (*domain.User, *domain.License) {
	t.Helper()
	applicationID := defaultApplicationIDForTest(t, repository)
	user, err := repository.CreateUser(ctx, applicationID, domain.NewUser{
		Email:        email,
		PasswordHash: "$argon2id$v=19$integration-hash",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	license, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: "8f46bf9ec2d930aaae995b45ad6f7867ad5c8c8ef9b4b1e9c4ab325ce36af7ac",
		UserID:      user.ID,
		Product:     "StarLoader",
		MaxDevices:  1,
		ExpiresAt:   base.Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateLicense() error = %v", err)
	}
	return user, license
}

// defaultApplicationIDForTest resolves the default StarLoader application that
// migration 000004 seeds; every end-user fixture is scoped to it.
func defaultApplicationIDForTest(t *testing.T, repository *store.Store) string {
	t.Helper()
	application, err := repository.FindApplicationBySlug(context.Background(), "starloader")
	if err != nil {
		t.Fatalf("resolve default application: %v", err)
	}
	return application.ID
}
