package integration_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starloader/backend/internal/credential"
	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/security"
	"github.com/starloader/backend/internal/service"
	"github.com/starloader/backend/internal/store"
	"github.com/starloader/backend/migrations"
)

func TestDeviceVerificationAcceptanceMatrix(t *testing.T) {
	t.Run("first activation repeat login and no raw hardware", func(t *testing.T) {
		fixture := newPostgresVerificationFixture(t, 1)
		key := generateP256Key(t)
		hardware := acceptanceHardware("raw-one")
		firstInput := fixture.newInput(t, key, hardware, fixture.now.Add(time.Minute))

		first, err := fixture.deviceService.Verify(fixture.ctx, firstInput)
		if err != nil {
			t.Fatalf("first Verify() error = %v", err)
		}
		claims, err := fixture.tokenVerifier.Verify(first.Token)
		if err != nil {
			t.Fatalf("Verify(token) error = %v", err)
		}
		if claims.Subject != fixture.user.ID || claims.ApplicationID != fixture.applicationID || claims.LicenseID != fixture.license.ID || claims.DeviceID != first.DeviceID || claims.Product != "StarLoader" || claims.Issuer != "starloader" || claims.Audience != "starloader-client" || !claims.ExpiresAt.Equal(fixture.now.Add(time.Hour)) {
			t.Fatalf("token claims = %#v", claims)
		}

		repeatInput := fixture.newInput(t, key, hardware, fixture.now.Add(time.Minute))
		var capturedLogs strings.Builder
		router := httpapi.NewRouter(httpapi.RouterConfig{
			DeviceVerification:   fixture.deviceService,
			Logger:               log.New(&capturedLogs, "", 0),
			DefaultApplicationID: fixture.applicationID,
		})
		request := httptest.NewRequest(http.MethodPost, "/v1/device/verify", strings.NewReader(deviceVerificationJSON(t, repeatInput)))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("repeat HTTP status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		if capturedLogs.Len() != 0 {
			t.Fatalf("verification logged request data: %q", capturedLogs.String())
		}

		var deviceCount int
		if err := fixture.pool.QueryRow(fixture.ctx, `select count(*) from devices`).Scan(&deviceCount); err != nil {
			t.Fatal(err)
		}
		if deviceCount != 1 {
			t.Fatalf("device count after repeat login = %d, want 1", deviceCount)
		}
		assertNoRawHardwareInDatabase(t, fixture, hardware)
	})

	t.Run("score 65 is below threshold and reaches full-device limit", func(t *testing.T) {
		fixture := newPostgresVerificationFixture(t, 1)
		key := generateP256Key(t)
		original := acceptanceHardware("original")
		first := fixture.newInput(t, key, original, fixture.now.Add(time.Minute))
		if _, err := fixture.deviceService.Verify(fixture.ctx, first); err != nil {
			t.Fatal(err)
		}
		belowThreshold := acceptanceHardware("changed")
		belowThreshold.MotherboardSerial = original.MotherboardSerial
		input := fixture.newInput(t, key, belowThreshold, fixture.now.Add(time.Minute))

		_, err := fixture.deviceService.Verify(fixture.ctx, input)
		if !errors.Is(err, service.ErrDeviceLimitReached) {
			t.Fatalf("Verify() error = %v, want device limit", err)
		}
		assertChallengeUnconsumed(t, fixture, input.SessionID)
	})

	t.Run("score 70 accepts the existing device", func(t *testing.T) {
		fixture := newPostgresVerificationFixture(t, 1)
		key := generateP256Key(t)
		original := acceptanceHardware("original")
		firstInput := fixture.newInput(t, key, original, fixture.now.Add(time.Minute))
		first, err := fixture.deviceService.Verify(fixture.ctx, firstInput)
		if err != nil {
			t.Fatal(err)
		}
		threshold := acceptanceHardware("changed")
		threshold.SMBIOSUUID = original.SMBIOSUUID
		input := fixture.newInput(t, key, threshold, fixture.now.Add(time.Minute))

		verified, err := fixture.deviceService.Verify(fixture.ctx, input)
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
		if verified.DeviceID != first.DeviceID {
			t.Fatalf("device ID = %q, want existing %q", verified.DeviceID, first.DeviceID)
		}
	})

	t.Run("invalid signature does not consume", func(t *testing.T) {
		fixture := newPostgresVerificationFixture(t, 1)
		input := fixture.newInput(t, generateP256Key(t), acceptanceHardware("invalid"), fixture.now.Add(time.Minute))
		input.ChallengeSignature = base64.StdEncoding.EncodeToString(make([]byte, 64))

		_, err := fixture.deviceService.Verify(fixture.ctx, input)
		if !errors.Is(err, service.ErrInvalidDeviceSignature) {
			t.Fatalf("Verify() error = %v, want invalid signature", err)
		}
		assertChallengeUnconsumed(t, fixture, input.SessionID)
		assertDeviceCount(t, fixture, 0)
	})

	t.Run("expired challenge does not consume", func(t *testing.T) {
		fixture := newPostgresVerificationFixture(t, 1)
		input := fixture.newInput(t, generateP256Key(t), acceptanceHardware("expired"), fixture.now)

		_, err := fixture.deviceService.Verify(fixture.ctx, input)
		if !errors.Is(err, service.ErrChallengeExpired) {
			t.Fatalf("Verify() error = %v, want expired", err)
		}
		assertChallengeUnconsumed(t, fixture, input.SessionID)
	})

	t.Run("replay is rejected", func(t *testing.T) {
		fixture := newPostgresVerificationFixture(t, 1)
		input := fixture.newInput(t, generateP256Key(t), acceptanceHardware("replay"), fixture.now.Add(time.Minute))
		if _, err := fixture.deviceService.Verify(fixture.ctx, input); err != nil {
			t.Fatal(err)
		}

		if _, err := fixture.deviceService.Verify(fixture.ctx, input); !errors.Is(err, domain.ErrChallengeConsumed) {
			t.Fatalf("replay error = %v, want consumed", err)
		}
	})

	t.Run("matching revoked device does not consume", func(t *testing.T) {
		fixture := newPostgresVerificationFixture(t, 1)
		key := generateP256Key(t)
		hardware := acceptanceHardware("revoked")
		firstInput := fixture.newInput(t, key, hardware, fixture.now.Add(time.Minute))
		first, err := fixture.deviceService.Verify(fixture.ctx, firstInput)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, `update devices set status = 'revoked' where id = $1`, first.DeviceID); err != nil {
			t.Fatal(err)
		}
		input := fixture.newInput(t, key, acceptanceHardware("revoked-changed"), fixture.now.Add(time.Minute))

		_, err = fixture.deviceService.Verify(fixture.ctx, input)
		if !errors.Is(err, service.ErrDeviceRevoked) {
			t.Fatalf("Verify() error = %v, want revoked", err)
		}
		assertChallengeUnconsumed(t, fixture, input.SessionID)
	})

	t.Run("concurrent activations enforce one license slot", func(t *testing.T) {
		fixture := newPostgresVerificationFixture(t, 1)
		firstInput := fixture.newInput(t, generateP256Key(t), acceptanceHardware("concurrent-one"), fixture.now.Add(time.Minute))
		secondInput := fixture.newInput(t, generateP256Key(t), acceptanceHardware("concurrent-two"), fixture.now.Add(time.Minute))
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, input := range []service.VerifyInput{firstInput, secondInput} {
			input := input
			go func() {
				<-start
				_, err := fixture.deviceService.Verify(fixture.ctx, input)
				results <- err
			}()
		}
		close(start)
		succeeded, limited := 0, 0
		for range 2 {
			select {
			case err := <-results:
				switch {
				case err == nil:
					succeeded++
				case errors.Is(err, service.ErrDeviceLimitReached):
					limited++
				default:
					t.Fatalf("concurrent Verify() error = %v", err)
				}
			case <-fixture.ctx.Done():
				t.Fatalf("timed out waiting for concurrent verification: %v", fixture.ctx.Err())
			}
		}
		if succeeded != 1 || limited != 1 {
			t.Fatalf("concurrent results: succeeded=%d limited=%d", succeeded, limited)
		}
		assertDeviceCount(t, fixture, 1)
		var consumedCount int
		if err := fixture.pool.QueryRow(fixture.ctx, `select count(*) from device_challenges where consumed_at is not null`).Scan(&consumedCount); err != nil {
			t.Fatal(err)
		}
		if consumedCount != 1 {
			t.Fatalf("consumed challenges = %d, want 1", consumedCount)
		}
	})
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

func TestVerificationLocksDeviceRowsAgainstConcurrentRevocation(t *testing.T) {
	fixture := newPostgresVerificationFixture(t, 1)
	key := generateP256Key(t)
	hardware := acceptanceHardware("row-lock")
	activated, err := fixture.deviceService.Verify(
		fixture.ctx,
		fixture.newInput(t, key, hardware, fixture.now.Add(time.Minute)),
	)
	if err != nil {
		t.Fatal(err)
	}
	pending := fixture.newInput(t, key, hardware, fixture.now.Add(time.Minute))

	verificationConn, err := fixture.pool.Acquire(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer verificationConn.Release()
	revocationConn, err := fixture.pool.Acquire(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer revocationConn.Release()
	var revocationPID int
	if err := revocationConn.QueryRow(fixture.ctx, `select pg_backend_pid()`).Scan(&revocationPID); err != nil {
		t.Fatal(err)
	}

	decisionRepository := store.New(verificationConn)
	rowsLocked := make(chan struct{})
	releaseDecision := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseDecision) }) }
	defer release()
	decisionErr := errors.New("decision complete without commit")
	decisionResult := make(chan error, 1)
	go func() {
		decisionResult <- decisionRepository.WithLockedChallenge(fixture.ctx, fixture.applicationID, pending.SessionID, func(locked *store.LockedChallenge) error {
			if _, err := locked.LockLicense(fixture.ctx); err != nil {
				return err
			}
			devices, err := locked.ListDevices(fixture.ctx)
			if err != nil {
				return err
			}
			if len(devices) != 1 || devices[0].ID != activated.DeviceID {
				return fmt.Errorf("locked devices = %#v", devices)
			}
			close(rowsLocked)
			select {
			case <-releaseDecision:
				return decisionErr
			case <-fixture.ctx.Done():
				return fixture.ctx.Err()
			}
		})
	}()
	waitForSignal(t, fixture.ctx, rowsLocked, "verification transaction to read device rows")

	const revocationMarker = "/* task9:concurrent-device-revocation */"
	revocationResult := make(chan error, 1)
	go func() {
		_, err := revocationConn.Exec(fixture.ctx, `
			update `+revocationMarker+` devices
			set status = 'revoked'
			where id = $1`, activated.DeviceID)
		revocationResult <- err
	}()
	waitForBackendQueryLockOrCompletion(t, fixture.ctx, fixture.pool, revocationPID, revocationMarker, revocationResult)
	release()

	if err := receiveResult(t, fixture.ctx, decisionResult); !errors.Is(err, decisionErr) {
		t.Fatalf("verification decision error = %v, want %v", err, decisionErr)
	}
	if err := receiveResult(t, fixture.ctx, revocationResult); err != nil {
		t.Fatalf("revocation update error = %v", err)
	}
	assertChallengeUnconsumed(t, fixture, pending.SessionID)
	var status domain.DeviceStatus
	if err := fixture.pool.QueryRow(fixture.ctx, `select status from devices where id = $1`, activated.DeviceID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != domain.DeviceStatusRevoked {
		t.Fatalf("device status = %s, want revoked", status)
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
	createdLicense, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: licenseHMAC,
		UserID:      createdUser.ID,
		Product:     "StarLoader",
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
	otherLicense, err := fixture.repository.CreateLicense(fixture.ctx, fixture.applicationID, domain.NewLicense{
		LicenseHMAC: strings.Repeat("b", 64), UserID: otherUser.ID, Product: "StarLoader", MaxDevices: 1, ExpiresAt: fixture.now.Add(24 * time.Hour),
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
	sameUserLicense, err := fixture.repository.CreateLicense(fixture.ctx, fixture.applicationID, domain.NewLicense{
		LicenseHMAC: strings.Repeat("d", 64), UserID: fixture.user.ID, Product: "StarLoader Plus", MaxDevices: 2, ExpiresAt: fixture.now.Add(48 * time.Hour),
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

	_, err := repository.CreateLicense(ctx, defaultApplicationIDForTest(t, repository), domain.NewLicense{
		LicenseHMAC: "cf038a1bf56e961e35dc7252eb82f800553db70ab311e7f88d85afc739128e7e",
		UserID:      user.ID,
		Product:     "StarLoader",
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

	licenseA, err := repository.CreateLicense(ctx, appA, domain.NewLicense{
		LicenseHMAC: strings.Repeat("a", 64), UserID: userA.ID, Product: "StarLoader", MaxDevices: 1, ExpiresAt: base.Add(24 * time.Hour),
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

func TestPublicClientFlowWithPublishableCredential(t *testing.T) {
	// Full platform flow: login + device verification through the HTTP router
	// authenticated by a publishable credential, mirroring the SDK usage.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	now := time.Now().UTC().Truncate(time.Second)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	applicationID := defaultApplicationIDForTest(t, repository)
	passwordHash, err := security.HashPassword("sdk-test-password")
	if err != nil {
		t.Fatal(err)
	}
	user, err := repository.CreateUser(ctx, applicationID, domain.NewUser{
		Email: "sdk-flow@example.com", PasswordHash: passwordHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	license, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: strings.Repeat("f", 64), UserID: user.ID, Product: "StarLoader", MaxDevices: 1, ExpiresAt: now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	generated, err := credential.Generate("publishable", "live", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCredential(ctx, domain.NewApplicationCredential{
		ApplicationID: applicationID, Environment: domain.CredentialEnvironmentLive,
		CredentialType: domain.CredentialPublishable, Name: "SDK",
		KeyPrefix: generated.Prefix, KeyHash: generated.Hash,
		Scopes: []string{"auth.login", "device.verify"},
	}); err != nil {
		t.Fatal(err)
	}
	router := httpapi.NewRouter(httpapi.RouterConfig{
		Login:                service.NewLoginService(repository, "StarLoader"),
		DeviceVerification:   newIntegrationDeviceService(t, repository, now),
		DefaultApplicationID: applicationID,
		Applications:         repository,
		Credentials:          credential.NewVerifier(repository),
	})
	authorization := "Bearer " + generated.Key

	// Login with the publishable key succeeds and returns a challenge.
	loginRequest := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{
		"email": "sdk-flow@example.com",
		"password": "sdk-test-password",
		"device_fingerprint": "sdk-device"
	}`))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set("Authorization", authorization)
	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRecorder.Code, loginRecorder.Body.String())
	}
	var loginResponse struct {
		OK        bool   `json:"ok"`
		SessionID string `json:"session_id"`
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(loginRecorder.Body.Bytes(), &loginResponse); err != nil {
		t.Fatal(err)
	}
	if !loginResponse.OK || loginResponse.SessionID == "" || loginResponse.Challenge == "" {
		t.Fatalf("login response = %#v", loginResponse)
	}

	// Device verification signs the challenge and completes the session.
	key := generateP256Key(t)
	challenge, err := base64.StdEncoding.DecodeString(loginResponse.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	publicBlob, signature := postgresCNGProof(t, key, challenge)
	hardware := acceptanceHardware("sdk")
	verifyBody := struct {
		SessionID          string                         `json:"session_id"`
		Challenge          string                         `json:"challenge"`
		ChallengeSignature string                         `json:"challenge_signature"`
		TPMPublicKey       string                         `json:"tpm_public_key"`
		Hardware           deviceVerificationJSONHardware `json:"hardware"`
	}{
		SessionID: loginResponse.SessionID, Challenge: loginResponse.Challenge,
		ChallengeSignature: base64.StdEncoding.EncodeToString(signature), TPMPublicKey: base64.StdEncoding.EncodeToString(publicBlob),
		Hardware: deviceVerificationJSONHardware{
			SMBIOSUUID: hardware.SMBIOSUUID, MotherboardSerial: hardware.MotherboardSerial,
			BIOSSerial: hardware.BIOSSerial, SystemDiskSerial: hardware.SystemDiskSerial,
			MachineGuid: hardware.MachineGuid, Fingerprint: hardware.Fingerprint,
		},
	}
	verifyRaw, err := json.Marshal(verifyBody)
	if err != nil {
		t.Fatal(err)
	}
	verifyRequest := httptest.NewRequest(http.MethodPost, "/v1/device/verify", strings.NewReader(string(verifyRaw)))
	verifyRequest.Header.Set("Content-Type", "application/json")
	verifyRequest.Header.Set("Authorization", authorization)
	verifyRecorder := httptest.NewRecorder()
	router.ServeHTTP(verifyRecorder, verifyRequest)
	if verifyRecorder.Code != http.StatusOK {
		t.Fatalf("verify status = %d, body = %s", verifyRecorder.Code, verifyRecorder.Body.String())
	}
	var verifyResponse struct {
		OK       bool   `json:"ok"`
		Token    string `json:"token"`
		DeviceID string `json:"device_id"`
	}
	if err := json.Unmarshal(verifyRecorder.Body.Bytes(), &verifyResponse); err != nil {
		t.Fatal(err)
	}
	if !verifyResponse.OK || verifyResponse.Token == "" || verifyResponse.DeviceID == "" {
		t.Fatalf("verify response = %#v", verifyResponse)
	}

	// The device was bound to the license of the logged-in user.
	var boundUser string
	if err := pool.QueryRow(ctx, `select user_id::text from devices where id = $1::uuid`, verifyResponse.DeviceID).Scan(&boundUser); err != nil {
		t.Fatal(err)
	}
	if boundUser != user.ID {
		t.Fatalf("device bound to user %q, want %q", boundUser, user.ID)
	}
	_ = license

	// A secret key is rejected on the client endpoint.
	secretKey, err := credential.Generate("secret", "live", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCredential(ctx, domain.NewApplicationCredential{
		ApplicationID: applicationID, Environment: domain.CredentialEnvironmentLive,
		CredentialType: domain.CredentialSecret, Name: "Server",
		KeyPrefix: secretKey.Prefix, KeyHash: secretKey.Hash, Scopes: []string{"users.read"},
	}); err != nil {
		t.Fatal(err)
	}
	badRequest := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":"x@y.z","password":"p","device_fingerprint":"f"}`))
	badRequest.Header.Set("Content-Type", "application/json")
	badRequest.Header.Set("Authorization", "Bearer "+secretKey.Key)
	badRecorder := httptest.NewRecorder()
	router.ServeHTTP(badRecorder, badRequest)
	assertIntegrationError(t, badRecorder, http.StatusUnauthorized, "INVALID_CREDENTIAL")
}

// newIntegrationDeviceService builds the device service used by the SDK flow
// test with the same key material as the postgres verification fixture.
func newIntegrationDeviceService(t *testing.T, repository *store.Store, now time.Time) *service.DeviceService {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := security.NewTokenIssuer(privateKey, "starloader", "starloader-client", "StarLoader")
	if err != nil {
		t.Fatal(err)
	}
	return service.NewDeviceService(service.NewStoreDeviceRepository(repository), service.DeviceServiceConfig{
		HardwareHMACKey: []byte("integration-hardware-secret"), TokenIssuer: issuer,
		Issuer: "starloader", Audience: "starloader-client", Product: "StarLoader", Now: func() time.Time { return now },
	})
}

func assertIntegrationError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, status, recorder.Body.String())
	}
	var response struct {
		OK   bool   `json:"ok"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Code != code {
		t.Fatalf("error response = %#v", response)
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

type postgresVerificationFixture struct {
	ctx           context.Context
	pool          *pgxpool.Pool
	repository    *store.Store
	deviceService *service.DeviceService
	tokenVerifier *security.TokenVerifier
	now           time.Time
	applicationID string
	user          *domain.User
	license       *domain.License
}

func newPostgresVerificationFixture(t *testing.T, maxDevices int) *postgresVerificationFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	now := time.Now().UTC().Truncate(time.Second)
	pool := openTestPool(t, ctx)
	resetAndMigrate(t, ctx, pool)
	repository := store.New(pool)
	applicationID := defaultApplicationIDForTest(t, repository)
	user, err := repository.CreateUser(ctx, applicationID, domain.NewUser{
		Email: "device-verification@example.com", PasswordHash: "$argon2id$v=19$integration-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	license, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: "a7a5cc218577a36a399be56de9ba9901391f73cc7446c6ee74846825fcc94343",
		UserID:      user.ID, Product: "StarLoader", MaxDevices: maxDevices, ExpiresAt: now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := security.NewTokenIssuer(privateKey, "starloader", "starloader-client", "StarLoader")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := security.NewTokenVerifier(publicKey, "starloader", "starloader-client", "StarLoader")
	if err != nil {
		t.Fatal(err)
	}
	deviceService := service.NewDeviceService(service.NewStoreDeviceRepository(repository), service.DeviceServiceConfig{
		HardwareHMACKey: []byte("integration-hardware-secret"), TokenIssuer: issuer,
		Issuer: "starloader", Audience: "starloader-client", Product: "StarLoader", Now: func() time.Time { return now },
	})
	return &postgresVerificationFixture{
		ctx: ctx, pool: pool, repository: repository, deviceService: deviceService,
		tokenVerifier: verifier, now: now, applicationID: applicationID, user: user, license: license,
	}
}

func (fixture *postgresVerificationFixture) newInput(t *testing.T, key *ecdsa.PrivateKey, hardware service.HardwareSignals, expiresAt time.Time) service.VerifyInput {
	t.Helper()
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(challenge)
	pending, err := fixture.repository.CreatePendingSession(fixture.ctx, fixture.applicationID, domain.NewPendingSession{
		ApplicationID: fixture.applicationID, UserID: fixture.user.ID, LicenseID: fixture.license.ID, ChallengeSHA256: digest[:], ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	publicBlob, signature := postgresCNGProof(t, key, challenge)
	return service.VerifyInput{
		ApplicationID: fixture.applicationID, SessionID: pending.Session.ID, Challenge: base64.StdEncoding.EncodeToString(challenge),
		ChallengeSignature: base64.StdEncoding.EncodeToString(signature), TPMPublicKey: base64.StdEncoding.EncodeToString(publicBlob),
		Hardware: hardware,
	}
}

func generateP256Key(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func postgresCNGProof(t *testing.T, key *ecdsa.PrivateKey, challenge []byte) ([]byte, []byte) {
	t.Helper()
	blob := make([]byte, 72)
	binary.LittleEndian.PutUint32(blob[:4], 0x31534345)
	binary.LittleEndian.PutUint32(blob[4:8], 32)
	key.X.FillBytes(blob[8:40])
	key.Y.FillBytes(blob[40:72])
	digest := sha256.Sum256(challenge)
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return blob, signature
}

func acceptanceHardware(suffix string) service.HardwareSignals {
	return service.HardwareSignals{
		SMBIOSUUID: "smbios-" + suffix, MotherboardSerial: "motherboard-" + suffix,
		BIOSSerial: "bios-" + suffix, SystemDiskSerial: "disk-" + suffix,
		MachineGuid: "guid-" + suffix, Fingerprint: "fingerprint-" + suffix,
	}
}

func deviceVerificationJSON(t *testing.T, input service.VerifyInput) string {
	t.Helper()
	body := struct {
		SessionID          string                         `json:"session_id"`
		Challenge          string                         `json:"challenge"`
		ChallengeSignature string                         `json:"challenge_signature"`
		TPMPublicKey       string                         `json:"tpm_public_key"`
		Hardware           deviceVerificationJSONHardware `json:"hardware"`
	}{
		SessionID: input.SessionID, Challenge: input.Challenge, ChallengeSignature: input.ChallengeSignature,
		TPMPublicKey: input.TPMPublicKey,
		Hardware: deviceVerificationJSONHardware{
			SMBIOSUUID: input.Hardware.SMBIOSUUID, MotherboardSerial: input.Hardware.MotherboardSerial,
			BIOSSerial: input.Hardware.BIOSSerial, SystemDiskSerial: input.Hardware.SystemDiskSerial,
			MachineGuid: input.Hardware.MachineGuid, Fingerprint: input.Hardware.Fingerprint,
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

type deviceVerificationJSONHardware struct {
	SMBIOSUUID        string `json:"smbios_uuid"`
	MotherboardSerial string `json:"motherboard_serial"`
	BIOSSerial        string `json:"bios_serial"`
	SystemDiskSerial  string `json:"system_disk_serial"`
	MachineGuid       string `json:"machine_guid"`
	Fingerprint       string `json:"fingerprint"`
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
