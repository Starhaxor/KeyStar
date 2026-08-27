package integration_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/credential"
	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/security"
	"github.com/starloader/backend/internal/service"
	"github.com/starloader/backend/internal/store"
)

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
	productID, planID := resolveTestProductPlan(t, ctx, repository, applicationID, "StarLoader")
	license, err := repository.CreateLicense(ctx, applicationID, domain.NewLicense{
		LicenseHMAC: strings.Repeat("f", 64), UserID: user.ID, ProductID: productID, PlanID: planID, MaxDevices: 1, ExpiresAt: now.Add(24 * time.Hour),
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
	refreshService := service.NewRefreshService(service.RefreshServiceConfig{
		Repository: repository, Profile: repository, HMACKey: []byte("integration-license-hmac"), TokenIssuer: issuer,
		Issuer: "starloader", Audience: "starloader-client", Product: "StarLoader",
	})
	return service.NewDeviceService(service.NewStoreDeviceRepository(repository), service.DeviceServiceConfig{
		HardwareHMACKey: []byte("integration-hardware-secret"), TokenIssuer: issuer,
		Issuer: "starloader", Audience: "starloader-client", Product: "StarLoader",
		RefreshService: refreshService, Now: func() time.Time { return now },
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

// assertIntegrationOK asserts a plain success status without inspecting the
// body.
func assertIntegrationOK(t *testing.T, recorder *httptest.ResponseRecorder, status int) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, status, recorder.Body.String())
	}
}

// serverAPIRequest sends an HTTP request against the server API namespace with
// an optional Authorization header and application context.
