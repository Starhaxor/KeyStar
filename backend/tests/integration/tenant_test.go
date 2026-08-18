package integration_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/credential"
	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/store"
)

func TestCrossTenantIsolation(t *testing.T) {
	// The most critical tenant guarantee: an object created in application A
	// must be invisible to every repository query resolved for application B,
	// even when the email or identifiers are identical.
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Microsecond)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	appA := defaultApplicationIDForTest(t, repository)

	organization, err := repository.CreateOrganization(ctx, "tenant-b")
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	appB, err := repository.CreateApplication(ctx, domain.NewApplication{
		OrganizationID: organization.ID, Name: "Tenant B App", Slug: "tenant-b-app",
	})
	if err != nil {
		t.Fatalf("CreateApplication() error = %v", err)
	}

	// The same email may exist in both applications and resolves to the row
	// of the queried application only.
	userA, err := repository.CreateUser(ctx, appA, domain.NewUser{
		Email: "mustafa@example.com", PasswordHash: "$argon2id$v=19$app-a-hash",
	})
	if err != nil {
		t.Fatalf("CreateUser(app A) error = %v", err)
	}
	userB, err := repository.CreateUser(ctx, appB.ID, domain.NewUser{
		Email: "mustafa@example.com", PasswordHash: "$argon2id$v=19$app-b-hash",
	})
	if err != nil {
		t.Fatalf("CreateUser(app B) error = %v", err)
	}
	if userA.ID == userB.ID || userA.ApplicationID == userB.ApplicationID {
		t.Fatalf("same email in different applications collapsed into one row: A=%#v B=%#v", userA, userB)
	}
	foundB, err := repository.FindUserByEmail(ctx, appB.ID, "mustafa@example.com")
	if err != nil {
		t.Fatalf("FindUserByEmail(app B) error = %v", err)
	}
	if foundB.ID != userB.ID || foundB.PasswordHash != "$argon2id$v=19$app-b-hash" {
		t.Fatalf("FindUserByEmail(app B) = %#v, want user B", foundB)
	}

	// Cross-application identifier lookups must fail as if the row never
	// existed (404 semantics, not 403).
	if _, err := repository.FindUserByID(ctx, appB.ID, userA.ID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("FindUserByID(app B, user A) error = %v, want %v", err, domain.ErrUserNotFound)
	}

	productID, planID := resolveTestProductPlan(t, ctx, repository, appA, "StarLoader")
	licenseA, err := repository.CreateLicense(ctx, appA, domain.NewLicense{
		LicenseHMAC: strings.Repeat("a", 64), UserID: userA.ID, ProductID: productID, PlanID: planID, MaxDevices: 1, ExpiresAt: base.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateLicense(app A) error = %v", err)
	}
	if _, err := repository.FindLicenseByHMAC(ctx, appB.ID, licenseA.LicenseHMAC); !errors.Is(err, domain.ErrLicenseNotFound) {
		t.Fatalf("FindLicenseByHMAC(app B, license A) error = %v, want %v", err, domain.ErrLicenseNotFound)
	}
	if _, err := repository.FindLicenseByUserAndProduct(ctx, appB.ID, userA.ID, "StarLoader"); !errors.Is(err, domain.ErrLicenseNotFound) {
		t.Fatalf("FindLicenseByUserAndProduct(app B, user A) error = %v, want %v", err, domain.ErrLicenseNotFound)
	}

	// Pending sessions and their challenges are app-bound as well.
	pending, err := repository.CreatePendingSession(ctx, appA, domain.NewPendingSession{
		ApplicationID: appA, UserID: userA.ID, LicenseID: licenseA.ID,
		ChallengeSHA256: bytes.Repeat([]byte{0x7c}, 32), ExpiresAt: base.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreatePendingSession(app A) error = %v", err)
	}
	if err := repository.WithLockedChallenge(ctx, appB.ID, pending.Session.ID, func(*store.LockedChallenge) error {
		return nil
	}); !errors.Is(err, domain.ErrChallengeNotFound) {
		t.Fatalf("WithLockedChallenge(app B, session A) error = %v, want %v", err, domain.ErrChallengeNotFound)
	}

	// A verified device from app A must never resolve through an app B profile
	// query, and a profile built with the wrong application must fail.
	var deviceID string
	if err := pool.QueryRow(ctx, `
		insert into devices (
			application_id, user_id, license_id, tpm_public_key, tpm_public_key_sha256, fingerprint_hmac, last_seen_at
		) values ($1, $2, $3, $4, $5, $6, $7)
		returning id::text`,
		appA, userA.ID, licenseA.ID, []byte("tpm-key"), bytes.Repeat([]byte{0x4d}, 32), strings.Repeat("e", 64), base,
	).Scan(&deviceID); err != nil {
		t.Fatalf("create device fixture: %v", err)
	}
	if _, err := repository.LoadProfile(ctx, appB.ID, userA.ID, licenseA.ID, deviceID); !errors.Is(err, domain.ErrProfileNotFound) {
		t.Fatalf("LoadProfile(app B, user A) error = %v, want %v", err, domain.ErrProfileNotFound)
	}
}

func TestApplicationCredentialsLifecycleAndIsolation(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	appA := defaultApplicationIDForTest(t, repository)

	organization, err := repository.CreateOrganization(ctx, "credential-tenant")
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	appB, err := repository.CreateApplication(ctx, domain.NewApplication{
		OrganizationID: organization.ID, Name: "Credential Tenant App", Slug: "credential-tenant-app",
	})
	if err != nil {
		t.Fatalf("CreateApplication() error = %v", err)
	}

	generated, err := credential.Generate("publishable", "live", nil)
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.CreateCredential(ctx, domain.NewApplicationCredential{
		ApplicationID: appA, Environment: domain.CredentialEnvironmentLive,
		CredentialType: domain.CredentialPublishable, Name: "Desktop SDK",
		KeyPrefix: generated.Prefix, KeyHash: generated.Hash,
		Scopes: []string{"auth.login", "device.verify"},
	})
	if err != nil {
		t.Fatalf("CreateCredential() error = %v", err)
	}
	if created.KeyPrefix != generated.Prefix || len(created.KeyHash) != 32 || created.Status != domain.CredentialStatusActive {
		t.Fatalf("created credential = %#v", created)
	}

	listed, err := repository.ListCredentials(ctx, appA)
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID || listed[0].KeyHash == nil {
		t.Fatalf("ListCredentials() = %#v", listed)
	}
	if other, err := repository.ListCredentials(ctx, appB.ID); err != nil || len(other) != 0 {
		t.Fatalf("ListCredentials(app B) = %#v, err = %v", other, err)
	}

	verifier := credential.NewVerifier(repository)
	verified, err := verifier.Verify(ctx, appA, generated.Key)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.ID != created.ID {
		t.Fatalf("Verify() = %#v", verified)
	}
	var lastUsedNotNull bool
	if err := pool.QueryRow(ctx, `select last_used_at is not null from application_credentials where id = $1`, created.ID).Scan(&lastUsedNotNull); err != nil {
		t.Fatal(err)
	}
	if !lastUsedNotNull {
		t.Fatal("credential last_used_at was not touched after verification")
	}

	// Cross-tenant lookup and verification must fail as if the key never existed.
	if _, err := repository.FindCredentialByPrefix(ctx, appB.ID, generated.Prefix); !errors.Is(err, domain.ErrCredentialNotFound) {
		t.Fatalf("FindCredentialByPrefix(app B) error = %v, want %v", err, domain.ErrCredentialNotFound)
	}
	if _, err := verifier.Verify(ctx, appB.ID, generated.Key); !errors.Is(err, domain.ErrInvalidCredential) {
		t.Fatalf("Verify(app B) error = %v, want %v", err, domain.ErrInvalidCredential)
	}

	// A wrong secret on a known prefix is rejected.
	wrongSecret := generated.Prefix + "_" + strings.Repeat("Z", 43)
	if _, err := verifier.Verify(ctx, appA, wrongSecret); !errors.Is(err, domain.ErrInvalidCredential) {
		t.Fatalf("Verify(wrong secret) error = %v, want %v", err, domain.ErrInvalidCredential)
	}

	// Revoked credentials are rejected even with the correct secret.
	if err := repository.RevokeCredential(ctx, appA, created.ID); err != nil {
		t.Fatalf("RevokeCredential() error = %v", err)
	}
	if _, err := verifier.Verify(ctx, appA, generated.Key); !errors.Is(err, domain.ErrCredentialRevoked) {
		t.Fatalf("Verify(revoked) error = %v, want %v", err, domain.ErrCredentialRevoked)
	}
	if err := repository.RevokeCredential(ctx, appA, created.ID); !errors.Is(err, domain.ErrCredentialNotFound) {
		t.Fatalf("second RevokeCredential() error = %v, want %v", err, domain.ErrCredentialNotFound)
	}

	// Expired credentials are rejected.
	expiring, err := credential.Generate("secret", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := repository.CreateCredential(ctx, domain.NewApplicationCredential{
		ApplicationID: appA, Environment: domain.CredentialEnvironmentTest,
		CredentialType: domain.CredentialSecret, Name: "Expired CI",
		KeyPrefix: expiring.Prefix, KeyHash: expiring.Hash, Scopes: []string{"users.read"}, ExpiresAt: &past,
	}); err != nil {
		t.Fatalf("CreateCredential(expired) error = %v", err)
	}
	if _, err := verifier.Verify(ctx, appA, expiring.Key); !errors.Is(err, domain.ErrCredentialExpired) {
		t.Fatalf("Verify(expired) error = %v, want %v", err, domain.ErrCredentialExpired)
	}
}
