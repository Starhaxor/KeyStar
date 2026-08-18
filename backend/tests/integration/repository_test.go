package integration_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/store"
)

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

	foundLicense, err := repository.FindLicenseByUserAndProduct(ctx, applicationID, createdUser.ID, "StarLoader")
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
