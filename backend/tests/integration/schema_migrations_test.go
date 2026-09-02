package integration_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/store"
)

func TestIntegrationDatabaseMustUseDedicatedName(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	if err := validateIntegrationDatabaseURL(databaseURL); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationDatabaseURLRejectsNonDedicatedDatabases(t *testing.T) {
	for _, tt := range []struct {
		name        string
		databaseURL string
		wantError   bool
	}{
		{name: "development database", databaseURL: "postgres://user:password@localhost:5432/keystar?sslmode=disable", wantError: true},
		{name: "another database", databaseURL: "postgres://user:password@localhost:5432/customer_data?sslmode=disable", wantError: true},
		{name: "dbname query override", databaseURL: "postgres://user:password@localhost:5432/keystar_test?dbname=keystar&sslmode=disable", wantError: true},
		{name: "database query override", databaseURL: "postgres://user:password@localhost:5432/keystar_test?database=keystar&sslmode=disable", wantError: true},
		{name: "dedicated database", databaseURL: "postgres://keystar_test:keystar_test@localhost:5432/keystar_test?sslmode=disable", wantError: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIntegrationDatabaseURL(tt.databaseURL)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateIntegrationDatabaseURL() error = %v, want error = %t", err, tt.wantError)
			}
		})
	}
}

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

func TestApplicationAuthProfileDefaultsConstraintsAndPersistence(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)

	existing, err := repository.FindApplicationBySlug(ctx, "starloader")
	if err != nil {
		t.Fatalf("find existing application: %v", err)
	}
	if existing.AuthProfile != domain.ApplicationAuthLegacy {
		t.Fatalf("existing auth profile = %q, want legacy", existing.AuthProfile)
	}

	organization, err := repository.CreateOrganization(ctx, "Auth profile test organization")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	created, err := repository.CreateApplication(ctx, domain.NewApplication{
		OrganizationID: organization.ID, Name: "Auth profile test application", Slug: "auth-profile-test",
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	if created.AuthProfile != domain.ApplicationAuthLegacy {
		t.Fatalf("new auth profile = %q, want legacy", created.AuthProfile)
	}

	proofBound := domain.ApplicationAuthProofBound
	updated, err := repository.UpdateApplication(ctx, created.ID, domain.UpdateApplication{AuthProfile: &proofBound})
	if err != nil {
		t.Fatalf("update auth profile: %v", err)
	}
	if updated.AuthProfile != domain.ApplicationAuthProofBound {
		t.Fatalf("updated auth profile = %q, want proof_bound", updated.AuthProfile)
	}
	byID, err := repository.FindApplicationByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("find application by ID: %v", err)
	}
	bySlug, err := repository.FindApplicationBySlug(ctx, created.Slug)
	if err != nil {
		t.Fatalf("find application by slug: %v", err)
	}
	applications, err := repository.ListApplications(ctx)
	if err != nil {
		t.Fatalf("list applications: %v", err)
	}
	if byID.AuthProfile != domain.ApplicationAuthProofBound || bySlug.AuthProfile != domain.ApplicationAuthProofBound {
		t.Fatalf("loaded profiles = ID %q, slug %q, want proof_bound", byID.AuthProfile, bySlug.AuthProfile)
	}
	found := false
	for _, application := range applications {
		if application.ID == created.ID {
			found = application.AuthProfile == domain.ApplicationAuthProofBound
		}
	}
	if !found {
		t.Fatal("listed application did not preserve proof_bound auth profile")
	}

	for _, profile := range []domain.ApplicationAuthProfile{"", "unknown"} {
		profile := profile
		if _, err := repository.UpdateApplication(ctx, created.ID, domain.UpdateApplication{AuthProfile: &profile}); !errors.Is(err, domain.ErrInvalidApplicationAuthProfile) {
			t.Fatalf("update profile %q error = %v, want ErrInvalidApplicationAuthProfile", profile, err)
		}
	}
	if _, err := pool.Exec(ctx, `update applications set auth_profile = 'unknown' where id = $1::uuid`, created.ID); err == nil {
		t.Fatal("database accepted an unknown auth profile")
	}
}

func TestApplicationSigningKeyConstraints(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)

	const insertKey = `
		insert into application_signing_keys (
			kid, application_id, algorithm, public_key, encrypted_private_key,
			encryption_nonce, encryption_key_version, status, activated_at, retire_at, revoked_at
		) values (
			$1, (select id from applications where slug = 'starloader'), 'Ed25519',
			$2, $3, $4, 1, $5, $6, $7, $8
		)`

	validPublicKey := bytes.Repeat([]byte{0x11}, 32)
	validEncryptedPrivateKey := bytes.Repeat([]byte{0x22}, 48)
	validNonce := bytes.Repeat([]byte{0x33}, 12)
	now := time.Now().UTC().Truncate(time.Microsecond)

	invalidWrites := []struct {
		name                string
		kid                 string
		publicKey           []byte
		encryptedPrivateKey []byte
		nonce               []byte
		status              string
		activatedAt         any
		retireAt            any
		revokedAt           any
	}{
		{name: "public key size", kid: "ksk_0000000000000000000001", publicKey: bytes.Repeat([]byte{0x11}, 31), encryptedPrivateKey: validEncryptedPrivateKey, nonce: validNonce, status: "pending"},
		{name: "encrypted private key size", kid: "ksk_0000000000000000000002", publicKey: validPublicKey, encryptedPrivateKey: bytes.Repeat([]byte{0x22}, 47), nonce: validNonce, status: "pending"},
		{name: "encryption nonce size", kid: "ksk_0000000000000000000003", publicKey: validPublicKey, encryptedPrivateKey: validEncryptedPrivateKey, nonce: bytes.Repeat([]byte{0x33}, 11), status: "pending"},
		{name: "invalid status", kid: "ksk_0000000000000000000004", publicKey: validPublicKey, encryptedPrivateKey: validEncryptedPrivateKey, nonce: validNonce, status: "unknown"},
		{name: "pending with activation timestamp", kid: "ksk_0000000000000000000005", publicKey: validPublicKey, encryptedPrivateKey: validEncryptedPrivateKey, nonce: validNonce, status: "pending", activatedAt: now},
		{name: "pending with retirement timestamp", kid: "ksk_0000000000000000000006", publicKey: validPublicKey, encryptedPrivateKey: validEncryptedPrivateKey, nonce: validNonce, status: "pending", retireAt: now},
		{name: "pending with revocation timestamp", kid: "ksk_0000000000000000000007", publicKey: validPublicKey, encryptedPrivateKey: validEncryptedPrivateKey, nonce: validNonce, status: "pending", revokedAt: now},
		{name: "active without activation", kid: "ksk_0000000000000000000008", publicKey: validPublicKey, encryptedPrivateKey: validEncryptedPrivateKey, nonce: validNonce, status: "active"},
		{name: "retiring without retirement", kid: "ksk_0000000000000000000009", publicKey: validPublicKey, encryptedPrivateKey: validEncryptedPrivateKey, nonce: validNonce, status: "retiring", activatedAt: now},
		{name: "revoked without revocation", kid: "ksk_0000000000000000000010", publicKey: validPublicKey, encryptedPrivateKey: validEncryptedPrivateKey, nonce: validNonce, status: "revoked"},
	}

	for _, tt := range invalidWrites {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, insertKey,
				tt.kid,
				tt.publicKey,
				tt.encryptedPrivateKey,
				tt.nonce,
				tt.status,
				tt.activatedAt,
				tt.retireAt,
				tt.revokedAt,
			)
			if err == nil {
				t.Fatal("invalid application signing key unexpectedly succeeded")
			}
		})
	}

	if _, err := pool.Exec(ctx, insertKey,
		"ksk_0000000000000000000011",
		validPublicKey,
		validEncryptedPrivateKey,
		validNonce,
		"active",
		now,
		nil,
		nil,
	); err != nil {
		t.Fatalf("create first active application signing key: %v", err)
	}
	if _, err := pool.Exec(ctx, insertKey,
		"ksk_0000000000000000000012",
		validPublicKey,
		validEncryptedPrivateKey,
		validNonce,
		"active",
		now,
		nil,
		nil,
	); err == nil {
		t.Fatal("second active application signing key unexpectedly succeeded")
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
