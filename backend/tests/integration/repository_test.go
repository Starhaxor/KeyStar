package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
	"github.com/starloader/backend/internal/service"
	"github.com/starloader/backend/internal/store"
)

func TestApplicationProvisioningCommitsApplicationAndActiveKeyTogether(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	organization, err := repository.CreateOrganization(ctx, "Provisioning Commit")
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	activatedAt := time.Date(2026, time.August, 31, 11, 45, 0, 0, time.UTC)
	provisioner := service.NewApplicationProvisioner(
		repository,
		newIntegrationApplicationKeyCipher(t),
		func() time.Time { return activatedAt },
	)

	application, err := provisioner.Create(ctx, domain.NewApplication{
		OrganizationID: organization.ID,
		Name:           "Provisioned Application",
		Slug:           "provisioned-application",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	activeKey, err := repository.FindActiveApplicationSigningKey(ctx, application.ID)
	if err != nil {
		t.Fatalf("FindActiveApplicationSigningKey() error = %v", err)
	}
	if activeKey.ApplicationID != application.ID || activeKey.Status != domain.ApplicationSigningKeyActive {
		t.Fatalf("active key application/status = %q/%q, want %q/%q", activeKey.ApplicationID, activeKey.Status, application.ID, domain.ApplicationSigningKeyActive)
	}
	if activeKey.ActivatedAt == nil || !activeKey.ActivatedAt.Equal(activatedAt) {
		t.Fatalf("active key activation = %v, want %v", activeKey.ActivatedAt, activatedAt)
	}
	keys, err := repository.ListApplicationSigningKeys(ctx, application.ID)
	if err != nil {
		t.Fatalf("ListApplicationSigningKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0].KID != activeKey.KID {
		t.Fatalf("application signing key count = %d, want one committed active key", len(keys))
	}
}

func TestApplicationProvisioningRollsBackApplicationWhenKeyFactoryFails(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	organization, err := repository.CreateOrganization(ctx, "Provisioning Factory Failure")
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	factoryErr := errors.New("key factory failed")

	application, err := repository.CreateApplicationWithSigningKey(ctx, domain.NewApplication{
		OrganizationID: organization.ID,
		Name:           "Rolled Back Factory",
		Slug:           "rolled-back-factory",
	}, func(string) (domain.NewApplicationSigningKey, error) {
		return domain.NewApplicationSigningKey{}, factoryErr
	})
	if !errors.Is(err, factoryErr) || application != nil {
		t.Fatalf("CreateApplicationWithSigningKey() = (%#v, %v), want nil, factory error", application, err)
	}
	if _, err := repository.FindApplicationBySlug(ctx, "rolled-back-factory"); !errors.Is(err, domain.ErrApplicationNotFound) {
		t.Fatalf("FindApplicationBySlug() error = %v, want %v after rollback", err, domain.ErrApplicationNotFound)
	}
}

func TestApplicationProvisioningRollsBackApplicationWhenKeyInsertFails(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	organization, err := repository.CreateOrganization(ctx, "Provisioning Insert Failure")
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	application, err := repository.CreateApplicationWithSigningKey(ctx, domain.NewApplication{
		OrganizationID: organization.ID,
		Name:           "Rolled Back Insert",
		Slug:           "rolled-back-insert",
	}, func(applicationID string) (domain.NewApplicationSigningKey, error) {
		return domain.NewApplicationSigningKey{
			KID:                  "invalid-kid",
			ApplicationID:        applicationID,
			Algorithm:            "Ed25519",
			PublicKey:            bytes.Repeat([]byte{0x11}, 32),
			EncryptedPrivateKey:  bytes.Repeat([]byte{0x22}, 48),
			EncryptionNonce:      bytes.Repeat([]byte{0x33}, 12),
			EncryptionKeyVersion: 1,
			Status:               domain.ApplicationSigningKeyActive,
			ActivatedAt:          pointerToTime(time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)),
		}, nil
	})
	if err == nil || application != nil {
		t.Fatalf("CreateApplicationWithSigningKey() = (%#v, %v), want key insert failure", application, err)
	}
	if _, err := repository.FindApplicationBySlug(ctx, "rolled-back-insert"); !errors.Is(err, domain.ErrApplicationNotFound) {
		t.Fatalf("FindApplicationBySlug() error = %v, want %v after rollback", err, domain.ErrApplicationNotFound)
	}
}

func TestApplicationProvisioningGeneratesDifferentPublicKeys(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	organization, err := repository.CreateOrganization(ctx, "Provisioning Distinct Keys")
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	provisioner := service.NewApplicationProvisioner(repository, newIntegrationApplicationKeyCipher(t), nil)

	first, err := provisioner.Create(ctx, domain.NewApplication{
		OrganizationID: organization.ID, Name: "First Key", Slug: "first-key",
	})
	if err != nil {
		t.Fatalf("Create() first error = %v", err)
	}
	second, err := provisioner.Create(ctx, domain.NewApplication{
		OrganizationID: organization.ID, Name: "Second Key", Slug: "second-key",
	})
	if err != nil {
		t.Fatalf("Create() second error = %v", err)
	}
	firstKey, err := repository.FindActiveApplicationSigningKey(ctx, first.ID)
	if err != nil {
		t.Fatalf("FindActiveApplicationSigningKey(first) error = %v", err)
	}
	secondKey, err := repository.FindActiveApplicationSigningKey(ctx, second.ID)
	if err != nil {
		t.Fatalf("FindActiveApplicationSigningKey(second) error = %v", err)
	}
	if bytes.Equal(firstKey.PublicKey, secondKey.PublicKey) {
		t.Fatalf("two applications received the same public key: %x", firstKey.PublicKey)
	}
}

func TestCreateInitialSigningKeyIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	organization, err := repository.CreateOrganization(ctx, "Initial Key Backfill")
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	application, err := repository.CreateApplication(ctx, domain.NewApplication{
		OrganizationID: organization.ID, Name: "Existing Application", Slug: "existing-application",
	})
	if err != nil {
		t.Fatalf("CreateApplication() error = %v", err)
	}
	withoutKey, err := repository.ListApplicationsWithoutSigningKey(ctx)
	if err != nil {
		t.Fatalf("ListApplicationsWithoutSigningKey() error = %v", err)
	}
	if !containsString(withoutKey, application.ID) {
		t.Fatalf("applications without signing key = %#v, want %q", withoutKey, application.ID)
	}

	cipher := newIntegrationApplicationKeyCipher(t)
	first := generateActiveIntegrationKey(t, cipher, application.ID, time.Date(2026, time.August, 31, 13, 0, 0, 0, time.UTC))
	inserted, err := repository.CreateInitialSigningKey(ctx, application.ID, first)
	if err != nil || !inserted {
		t.Fatalf("CreateInitialSigningKey() first = (%t, %v), want true, nil", inserted, err)
	}
	second := generateActiveIntegrationKey(t, cipher, application.ID, time.Date(2026, time.August, 31, 14, 0, 0, 0, time.UTC))
	inserted, err = repository.CreateInitialSigningKey(ctx, application.ID, second)
	if err != nil || inserted {
		t.Fatalf("CreateInitialSigningKey() second = (%t, %v), want false, nil", inserted, err)
	}
	active, err := repository.FindActiveApplicationSigningKey(ctx, application.ID)
	if err != nil {
		t.Fatalf("FindActiveApplicationSigningKey() error = %v", err)
	}
	if active.KID != first.KID || bytes.Equal(active.PublicKey, second.PublicKey) {
		t.Fatalf("active key was replaced: got kid %q, want %q", active.KID, first.KID)
	}
}

func newIntegrationApplicationKeyCipher(t *testing.T) *security.ApplicationKeyCipher {
	t.Helper()
	cipher, err := security.NewApplicationKeyCipher(map[int][]byte{
		1: bytes.Repeat([]byte{0x5a}, 32),
	}, 1, rand.Reader)
	if err != nil {
		t.Fatalf("NewApplicationKeyCipher() error = %v", err)
	}
	return cipher
}

func generateActiveIntegrationKey(
	t *testing.T,
	cipher *security.ApplicationKeyCipher,
	applicationID string,
	activatedAt time.Time,
) domain.NewApplicationSigningKey {
	t.Helper()
	key, err := cipher.Generate(applicationID)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	key.Status = domain.ApplicationSigningKeyActive
	key.ActivatedAt = &activatedAt
	return key
}

func pointerToTime(value time.Time) *time.Time {
	return &value
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestAdminBootstrapCreatesExactlyOneOwner(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)

	required, err := repository.AdminBootstrapRequired(ctx)
	if err != nil || !required {
		t.Fatalf("AdminBootstrapRequired() = %t, %v; want true, nil", required, err)
	}

	const attempts = 6
	start := make(chan struct{})
	results := make(chan error, attempts)
	var ready sync.WaitGroup
	ready.Add(attempts)
	for index := 0; index < attempts; index++ {
		go func(index int) {
			ready.Done()
			<-start
			_, err := repository.BootstrapAdminAccount(ctx, domain.NewAdminAccount{
				Email:        fmt.Sprintf("root-%d@example.com", index),
				PasswordHash: "$argon2id$v=19$bootstrap-hash",
				RoleName:     domain.RoleViewer,
			})
			results <- err
		}(index)
	}
	ready.Wait()
	close(start)

	succeeded := 0
	closed := 0
	for index := 0; index < attempts; index++ {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrAdminBootstrapClosed):
			closed++
		default:
			t.Fatalf("BootstrapAdminAccount() error = %v", err)
		}
	}
	if succeeded != 1 || closed != attempts-1 {
		t.Fatalf("bootstrap results = %d success, %d closed", succeeded, closed)
	}

	required, err = repository.AdminBootstrapRequired(ctx)
	if err != nil || required {
		t.Fatalf("AdminBootstrapRequired() = %t, %v; want false, nil", required, err)
	}
	accounts, err := repository.ListAdminAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].RoleName != domain.RoleOwner {
		t.Fatalf("ListAdminAccounts() = %#v, %v; want one owner", accounts, err)
	}
}

func TestAdminBootstrapRemainsClosedWhenAllAdminsAreRemoved(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)

	created, err := repository.CreateAdminAccount(ctx, domain.NewAdminAccount{
		Email: "first@example.com", PasswordHash: "$argon2id$v=19$bootstrap-hash", RoleName: domain.RoleViewer,
	})
	if err != nil {
		t.Fatalf("CreateAdminAccount() error = %v", err)
	}
	if created.RoleName != domain.RoleOwner {
		t.Fatalf("first admin role = %q, want %q", created.RoleName, domain.RoleOwner)
	}
	if _, err := pool.Exec(ctx, `delete from admin_accounts`); err != nil {
		t.Fatalf("delete admin accounts: %v", err)
	}
	required, err := repository.AdminBootstrapRequired(ctx)
	if err != nil || required {
		t.Fatalf("AdminBootstrapRequired() = %t, %v; want false, nil", required, err)
	}
}

func TestGeneratedDatabaseIDsUseUUIDv7(t *testing.T) {
	fixture := newPostgresVerificationFixture(t, 1)
	input := fixture.newInput(t, generateP256Key(t), acceptanceHardware("uuid-v7"), fixture.now.Add(time.Minute))
	verified, err := fixture.deviceService.Verify(fixture.ctx, input)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	var challengeID string
	if err := fixture.pool.QueryRow(fixture.ctx, `select id::text from device_challenges where session_id = $1`, input.SessionID).Scan(&challengeID); err != nil {
		t.Fatalf("read challenge ID: %v", err)
	}
	for name, value := range map[string]string{
		"user": fixture.user.ID, "license": fixture.license.ID, "session": input.SessionID,
		"challenge": challengeID, "device": verified.DeviceID,
	} {
		t.Run(name, func(t *testing.T) { assertUUIDv7(t, value) })
	}
}

func assertUUIDv7(t *testing.T, value string) {
	t.Helper()
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '7' || !strings.ContainsRune("89ab", rune(value[19])) {
		t.Fatalf("ID %q is not canonical UUIDv7", value)
	}
}

func TestUserAndLicenseRoundTrip(t *testing.T) {
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Microsecond)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	applicationID := defaultApplicationIDForTest(t, repository)

	createdUser, err := repository.CreateUser(ctx, applicationID, domain.NewUser{
		Email:        "person@example.com",
		PasswordHash: "$argon2id$v=19$test-hash",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if createdUser.ID == "" {
		t.Fatal("CreateUser() returned an empty ID")
	}
	if createdUser.ApplicationID != applicationID {
		t.Fatalf("CreateUser() application ID = %q, want %q", createdUser.ApplicationID, applicationID)
	}

	foundUser, err := repository.FindUserByEmail(ctx, applicationID, "person@example.com")
	if err != nil {
		t.Fatalf("FindUserByEmail() error = %v", err)
	}
	if foundUser.ID != createdUser.ID || foundUser.Email != "person@example.com" || foundUser.PasswordHash != "$argon2id$v=19$test-hash" || foundUser.Status != domain.UserStatusActive {
		t.Fatalf("FindUserByEmail() = %#v", foundUser)
	}

	expiresAt := base.Add(30 * 24 * time.Hour)
	licenseHMAC := "5c89e0aeacdc0f1e84682f1d9f4b7bc81c279466603fefb87941b21df91f5fd2"
	productID, planID := resolveTestProductPlan(t, ctx, repository, applicationID, "StarLoader")
	createdLicense, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: licenseHMAC,
		UserID:      createdUser.ID,
		ProductID:   productID,
		PlanID:      planID,
		MaxDevices:  2,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateLicense() error = %v", err)
	}

	// Runtime PRODUCT is a stable product identifier and may use the canonical
	// lowercase slug while the catalog keeps a human-friendly display name.
	foundLicense, err := repository.FindLicenseByUserAndProduct(ctx, applicationID, createdUser.ID, "starloader")
	if err != nil {
		t.Fatalf("FindLicenseByUserAndProduct() error = %v", err)
	}
	if foundLicense.ID != createdLicense.ID || foundLicense.UserID != createdUser.ID || foundLicense.LicenseHMAC != licenseHMAC || foundLicense.Product != "StarLoader" || foundLicense.Status != domain.LicenseStatusActive || foundLicense.MaxDevices != 2 || !foundLicense.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("FindLicenseByUserAndProduct() = %#v", foundLicense)
	}
}

func TestLoadProfileBindsUserLicenseAndDevice(t *testing.T) {
	// These cases fail if the profile query omits a claimed identifier or an ownership join.
	fixture := newPostgresVerificationFixture(t, 1)
	activated, err := fixture.deviceService.Verify(
		fixture.ctx,
		fixture.newInput(t, generateP256Key(t), acceptanceHardware("profile"), fixture.now.Add(time.Minute)),
	)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	profile, err := fixture.repository.LoadProfile(fixture.ctx, fixture.applicationID, fixture.user.ID, fixture.license.ID, activated.DeviceID)
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	if profile.Email != "device-verification@example.com" || profile.AccountStatus != domain.UserStatusActive ||
		profile.Product != "StarLoader" || profile.LicenseStatus != domain.LicenseStatusActive ||
		!profile.LicenseExpiresAt.Equal(fixture.license.ExpiresAt) || profile.MaxDevices != 1 ||
		profile.DeviceID != activated.DeviceID || profile.DeviceStatus != domain.DeviceStatusActive {
		t.Fatalf("LoadProfile() = %#v", profile)
	}

	otherUser, err := fixture.repository.CreateUser(fixture.ctx, fixture.applicationID, domain.NewUser{
		Email: "other-profile@example.com", PasswordHash: "$argon2id$v=19$integration-hash",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	productID, planID := resolveTestProductPlan(t, fixture.ctx, fixture.repository, fixture.applicationID, "StarLoader")
	otherLicense, err := fixture.repository.CreateLicense(fixture.ctx, fixture.applicationID, domain.NewLicense{
		LicenseHMAC: strings.Repeat("b", 64), UserID: otherUser.ID, ProductID: productID, PlanID: planID, MaxDevices: 1, ExpiresAt: fixture.now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateLicense() error = %v", err)
	}
	var otherDeviceID string
	err = fixture.pool.QueryRow(fixture.ctx, `
		insert into devices (
			application_id, user_id, license_id, tpm_public_key, tpm_public_key_sha256, fingerprint_hmac, last_seen_at
		) values ($1, $2, $3, $4, $5, $6, $7)
		returning id::text`,
		fixture.applicationID, otherUser.ID, otherLicense.ID, []byte("other-tpm-public-key"), bytes.Repeat([]byte{0x2a}, 32), strings.Repeat("c", 64), fixture.now,
	).Scan(&otherDeviceID)
	if err != nil {
		t.Fatalf("create other device: %v", err)
	}
	plusProductID, plusPlanID := resolveTestProductPlan(t, fixture.ctx, fixture.repository, fixture.applicationID, "StarLoader Plus")
	sameUserLicense, err := fixture.repository.CreateLicense(fixture.ctx, fixture.applicationID, domain.NewLicense{
		LicenseHMAC: strings.Repeat("d", 64), UserID: fixture.user.ID, ProductID: plusProductID, PlanID: plusPlanID, MaxDevices: 2, ExpiresAt: fixture.now.Add(48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateLicense() for same user error = %v", err)
	}
	var sameUserDeviceID string
	err = fixture.pool.QueryRow(fixture.ctx, `
		insert into devices (
			application_id, user_id, license_id, tpm_public_key, tpm_public_key_sha256, fingerprint_hmac, last_seen_at
		) values ($1, $2, $3, $4, $5, $6, $7)
		returning id::text`,
		fixture.applicationID, fixture.user.ID, sameUserLicense.ID, []byte("same-user-tpm-public-key"), bytes.Repeat([]byte{0x3a}, 32), strings.Repeat("e", 64), fixture.now,
	).Scan(&sameUserDeviceID)
	if err != nil {
		t.Fatalf("create same-user device: %v", err)
	}
	if _, err := fixture.repository.LoadProfile(fixture.ctx, fixture.applicationID, fixture.user.ID, sameUserLicense.ID, sameUserDeviceID); err != nil {
		t.Fatalf("LoadProfile() for same-user alternate license error = %v", err)
	}

	for _, tt := range []struct {
		name      string
		userID    string
		licenseID string
		deviceID  string
	}{
		{name: "other user", userID: otherUser.ID, licenseID: fixture.license.ID, deviceID: activated.DeviceID},
		{name: "other license", userID: fixture.user.ID, licenseID: otherLicense.ID, deviceID: otherDeviceID},
		{name: "other device", userID: fixture.user.ID, licenseID: fixture.license.ID, deviceID: otherDeviceID},
		{name: "same user alternate device with original license", userID: fixture.user.ID, licenseID: fixture.license.ID, deviceID: sameUserDeviceID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fixture.repository.LoadProfile(fixture.ctx, fixture.applicationID, tt.userID, tt.licenseID, tt.deviceID)
			if !errors.Is(err, domain.ErrProfileNotFound) {
				t.Fatalf("LoadProfile() error = %v, want %v", err, domain.ErrProfileNotFound)
			}
		})
	}
}

func TestSingleLicensePerUserProduct(t *testing.T) {
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Microsecond)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	user, _ := createUserAndLicense(t, ctx, repository, "single-license@example.com", base)

	applicationID := defaultApplicationIDForTest(t, repository)
	productID, planID := resolveTestProductPlan(t, ctx, repository, applicationID, "StarLoader")
	_, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: "cf038a1bf56e961e35dc7252eb82f800553db70ab311e7f88d85afc739128e7e",
		UserID:      user.ID,
		ProductID:   productID,
		PlanID:      planID,
		MaxDevices:  2,
		ExpiresAt:   base.Add(60 * 24 * time.Hour),
	})
	if !errors.Is(err, domain.ErrLicenseAlreadyExists) {
		t.Fatalf("CreateLicense() error = %v, want %v", err, domain.ErrLicenseAlreadyExists)
	}
}

func TestUserRepositoryNormalizesEmail(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	applicationID := defaultApplicationIDForTest(t, repository)

	created, err := repository.CreateUser(ctx, applicationID, domain.NewUser{
		Email:        "  Person@Example.COM ",
		PasswordHash: "$argon2id$v=19$normalized-email",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if created.Email != "person@example.com" {
		t.Fatalf("CreateUser() email = %q", created.Email)
	}

	found, err := repository.FindUserByEmail(ctx, applicationID, " PERSON@EXAMPLE.COM ")
	if err != nil {
		t.Fatalf("FindUserByEmail() error = %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("FindUserByEmail() ID = %q, want %q", found.ID, created.ID)
	}
}

func TestRepositoryNotFoundErrorsAreTyped(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	applicationID := defaultApplicationIDForTest(t, repository)

	tests := []struct {
		name   string
		entity string
		find   func() error
	}{
		{
			name:   "user",
			entity: "user",
			find: func() error {
				_, err := repository.FindUserByEmail(ctx, applicationID, "missing@example.com")
				return err
			},
		},
		{
			name:   "license",
			entity: "license",
			find: func() error {
				_, err := repository.FindLicenseByHMAC(ctx, applicationID, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
				return err
			},
		},
		{
			name:   "challenge",
			entity: "challenge",
			find: func() error {
				return repository.WithLockedChallenge(ctx, applicationID, "00000000-0000-0000-0000-000000000000", func(*store.LockedChallenge) error {
					return nil
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.find()
			var notFound *domain.NotFoundError
			if !errors.As(err, &notFound) || notFound.Entity != tt.entity {
				t.Fatalf("error = %v, want typed %s not-found error", err, tt.entity)
			}
		})
	}
}

// TestProductCatalogAndPlanBinding verifies the Phase 4 normalization: product
// names resolve into an application-scoped catalog, every license is bound to
// a product and its default plan, and the resolved display name round-trips.
func TestProductCatalogAndPlanBinding(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	applicationID := defaultApplicationIDForTest(t, repository)

	// The default StarLoader application is seeded with a product during
	// migration 000010; resolving the same name is idempotent.
	firstProductID, firstPlanID := resolveTestProductPlan(t, ctx, repository, applicationID, "StarLoader")
	secondProductID, secondPlanID := resolveTestProductPlan(t, ctx, repository, applicationID, "StarLoader")
	if firstProductID != secondProductID || firstPlanID != secondPlanID {
		t.Fatalf("ResolveProductPlan is not idempotent: (%s, %s) then (%s, %s)",
			firstProductID, secondProductID, firstPlanID, secondPlanID)
	}

	product, err := repository.FindProductByID(ctx, applicationID, firstProductID)
	if err != nil {
		t.Fatalf("FindProductByID() error = %v", err)
	}
	if product.Name != "StarLoader" || product.Slug != "starloader" || product.Status != "active" {
		t.Fatalf("seeded product = %#v", product)
	}
	plans, err := repository.ListPlans(ctx, firstProductID)
	if err != nil {
		t.Fatalf("ListPlans() error = %v", err)
	}
	if len(plans) != 1 || plans[0].Code != "default" || plans[0].MaxDevices != 1 {
		t.Fatalf("default plan = %#v", plans)
	}

	// A second product is created on demand with its own default plan.
	plusProductID, plusPlanID := resolveTestProductPlan(t, ctx, repository, applicationID, "StarLoader Plus")
	if plusProductID == firstProductID || plusPlanID == firstPlanID {
		t.Fatalf("second product/plan collided with the first: (%s, %s)", plusProductID, plusPlanID)
	}

	// Licenses bound to distinct products do not collide under the
	// (user_id, product_id) uniqueness guarantee.
	user, err := repository.CreateUser(ctx, applicationID, domain.NewUser{Email: "catalog@example.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	base := time.Now().UTC().Truncate(time.Second)
	first, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: strings.Repeat("1", 64), UserID: user.ID, ProductID: firstProductID, PlanID: firstPlanID,
		MaxDevices: 2, ExpiresAt: base.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateLicense() error = %v", err)
	}
	second, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: strings.Repeat("2", 64), UserID: user.ID, ProductID: plusProductID, PlanID: plusPlanID,
		MaxDevices: 3, ExpiresAt: base.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateLicense() second product error = %v", err)
	}
	if first.Product != "StarLoader" || second.Product != "StarLoader Plus" {
		t.Fatalf("resolved product names = %q, %q", first.Product, second.Product)
	}
	if first.PlanID != firstPlanID || second.PlanID != plusPlanID {
		t.Fatalf("plan binding lost: %q, %q", first.PlanID, second.PlanID)
	}

	// The same product again violates the per-product uniqueness.
	if _, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: strings.Repeat("3", 64), UserID: user.ID, ProductID: firstProductID, PlanID: firstPlanID,
		MaxDevices: 1, ExpiresAt: base.Add(24 * time.Hour),
	}); !errors.Is(err, domain.ErrLicenseAlreadyExists) {
		t.Fatalf("CreateLicense() duplicate product error = %v, want %v", err, domain.ErrLicenseAlreadyExists)
	}

	// Products are application-scoped: a second application sees an empty
	// catalog and cannot resolve the first application's product.
	organization, err := repository.CreateOrganization(ctx, "second-tenant")
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	otherApp, err := repository.CreateApplication(ctx, domain.NewApplication{
		OrganizationID: organization.ID, Name: "Second", Slug: "second",
	})
	if err != nil {
		t.Fatalf("CreateApplication() error = %v", err)
	}
	otherProducts, err := repository.ListProducts(ctx, otherApp.ID)
	if err != nil {
		t.Fatalf("ListProducts(other app) error = %v", err)
	}
	if len(otherProducts) != 0 {
		t.Fatalf("other application catalog = %#v, want empty", otherProducts)
	}
	if _, err := repository.FindProductByID(ctx, otherApp.ID, firstProductID); !errors.Is(err, domain.ErrProductNotFound) {
		t.Fatalf("FindProductByID(other app) error = %v, want %v", err, domain.ErrProductNotFound)
	}
}
