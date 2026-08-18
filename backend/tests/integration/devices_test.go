package integration_test

import (
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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/security"
	"github.com/starloader/backend/internal/service"
	"github.com/starloader/backend/internal/store"
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
