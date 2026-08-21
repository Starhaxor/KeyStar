package integration_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/starloader/backend/internal/store"
)

func TestSchemaRejectsInvalidStatusesAndUnprotectedValues(t *testing.T) {
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Microsecond)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	user, license := createUserAndLicense(t, ctx, repository, "constraints@example.com", base)

	invalidStatements := []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "unnormalized email",
			sql:  `insert into users (application_id, email, password_hash) values ((select id from applications where slug = 'starloader'), 'Mixed@Example.COM', 'hash')`,
		},
		{
			name: "UUIDv4 identifier",
			sql:  `insert into users (id, application_id, email, password_hash) values ('550e8400-e29b-41d4-a716-446655440000', (select id from applications where slug = 'starloader'), 'uuidv4@example.com', 'hash')`,
		},
		{
			name: "invalid user status",
			sql:  `update users set status = 'unknown' where id = $1`,
			args: []any{user.ID},
		},
		{
			name: "invalid license status",
			sql:  `update licenses set status = 'unknown' where id = $1`,
			args: []any{license.ID},
		},
		{
			name: "non-positive device limit",
			sql:  `update licenses set max_devices = 0 where id = $1`,
			args: []any{license.ID},
		},
		{
			name: "non-HMAC license value",
			sql:  `update licenses set license_hmac = 'plaintext-license' where id = $1`,
			args: []any{license.ID},
		},
		{
			name: "invalid device status",
			sql: `insert into devices (
				application_id, user_id, license_id, tpm_public_key, tpm_public_key_sha256, fingerprint_hmac, status
			) values ((select id from applications where slug = 'starloader'), $1, $2, $3, $4, $5, 'unknown')`,
			args: []any{user.ID, license.ID, []byte{0x01}, bytes.Repeat([]byte{0x02}, 32), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		},
		{
			name: "invalid session status",
			sql:  `insert into auth_sessions (application_id, user_id, license_id, status, expires_at) values ((select id from applications where slug = 'starloader'), $1, $2, 'unknown', $3)`,
			args: []any{user.ID, license.ID, base.Add(2 * time.Minute)},
		},
	}

	for _, tt := range invalidStatements {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, tt.sql, tt.args...); err == nil {
				t.Fatal("invalid database write unexpectedly succeeded")
			}
		})
	}
}

func TestMigrationDownAndUp(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)

	if err := store.MigrateDown(ctx, pool); err != nil {
		t.Fatalf("MigrateDown() error = %v", err)
	}
	assertTablesExist(t, ctx, pool, false)

	if err := store.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("second MigrateUp() error = %v", err)
	}
	assertTablesExist(t, ctx, pool, true)
}

func TestMigrationUpIsIdempotentAndTracked(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	if _, err := pool.Exec(ctx, "drop schema public cascade; create schema public"); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := store.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("first MigrateUp() error = %v", err)
	}
	if err := store.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("second MigrateUp() error = %v", err)
	}
	var applied int
	if err := pool.QueryRow(ctx, `select count(*) from schema_migrations where version in (1, 2)`).Scan(&applied); err != nil {
		t.Fatalf("read schema migration version: %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied migration rows = %d, want 2", applied)
	}
}

func TestOwnerRoleReceivesAllConsoleManagementPermissions(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)

	var missing []string
	if err := pool.QueryRow(ctx, `
		select array_agg(permission order by permission)
		from unnest(array[
			'applications.read', 'applications.write',
			'catalog.read', 'catalog.write',
			'webhooks.read', 'webhooks.write'
		]) as permission
		where not exists (
			select 1 from roles
			where name = 'owner' and permission = any(permissions)
		)
	`).Scan(&missing); err != nil {
		t.Fatalf("read owner role permissions: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("owner role is missing console management permissions: %v", missing)
	}
}

func TestMigrationUpgradesVersionOneSchema(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetToVersionOne(t, ctx, pool)

	if err := store.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("MigrateUp() error = %v", err)
	}

	var applied int
	if err := pool.QueryRow(ctx, `select count(*) from schema_migrations where version in (1, 2)`).Scan(&applied); err != nil {
		t.Fatalf("read schema migration versions: %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied migration rows = %d, want 2", applied)
	}
}

func TestMigrationVersionTwoRejectsExistingDuplicateLicenses(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetToVersionOne(t, ctx, pool)
	// Insert directly against the version-1 schema (no application_id column
	// exists yet) to simulate pre-tenant data with a duplicate user/product.
	var userID string
	if err := pool.QueryRow(ctx, `
		insert into users (email, password_hash)
		values ('duplicate-migration@example.com', '$argon2id$v=19$integration-hash')
		returning id::text`).Scan(&userID); err != nil {
		t.Fatalf("create version-one user fixture: %v", err)
	}
	for _, hmac := range []string{
		"8f46bf9ec2d930aaae995b45ad6f7867ad5c8c8ef9b4b1e9c4ab325ce36af7ac",
		"9a2d43a5aaafcab6f63b9ec10fbe45d47b628d39cf1fd4f00bf21a4e6123a941",
	} {
		if _, err := pool.Exec(ctx, `
			insert into licenses (license_hmac, user_id, product, status, max_devices, expires_at)
			values ($1, $2, 'StarLoader', 'active', 1, now() + interval '1 day')`, hmac, userID); err != nil {
			t.Fatalf("create duplicate license fixture: %v", err)
		}
	}

	if err := store.MigrateUp(ctx, pool); err == nil {
		t.Fatal("MigrateUp() unexpectedly accepted duplicate user/product licenses")
	}
	var licenseCount int
	if err := pool.QueryRow(ctx, `select count(*) from licenses where user_id = $1 and product = 'StarLoader'`, userID).Scan(&licenseCount); err != nil {
		t.Fatalf("count duplicate licenses: %v", err)
	}
	if licenseCount != 2 {
		t.Fatalf("duplicate licenses after rejected migration = %d, want 2", licenseCount)
	}
	var versionTwoApplied bool
	if err := pool.QueryRow(ctx, `select exists(select 1 from schema_migrations where version = 2)`).Scan(&versionTwoApplied); err != nil {
		t.Fatalf("read migration version 2: %v", err)
	}
	if versionTwoApplied {
		t.Fatal("failed version 2 migration was recorded as applied")
	}
}

func TestConcurrentMigrationUpIsSerialized(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := openTestPool(t, ctx)
	if _, err := pool.Exec(ctx, "drop schema public cascade; create schema public"); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			results <- store.MigrateUp(ctx, pool)
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent MigrateUp() error = %v", err)
		}
	}
	var applied int
	if err := pool.QueryRow(ctx, `select count(*) from schema_migrations where version in (1, 2)`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("applied migration rows = %d, want 2", applied)
	}
}
