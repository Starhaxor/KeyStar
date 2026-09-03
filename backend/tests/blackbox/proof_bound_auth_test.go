package blackbox_test

// Black-box coverage for the application-scoped proof_bound profile.
//
// The test exercises a live proof-enabled StarLoader fixture end to end:
// password login, TPM P-256 challenge verification with a device JWK,
// 600-second key-bound token issuance, DPoP-protected /v1/me, replay
// rejection, different-key (stolen token) rejection, stale-proof expiry,
// refresh rejection, bearer-fallback rejection, a parallel legacy
// application, and concurrent identical-proof submission where exactly one
// request wins.
//
// Fixture contract (all client keys are generated at runtime; nothing here
// is a secret):
//
//	STARLOADER_PROOFBOUND_BASE_URL (fallback STARLOADER_SMOKE_BASE_URL)
//	STARLOADER_PROOFBOUND_EMAIL    (fallback STARLOADER_SMOKE_EMAIL)
//	STARLOADER_PROOFBOUND_PASSWORD (fallback STARLOADER_SMOKE_PASSWORD)
//	STARLOADER_PROOFBOUND_APP_ID + STARLOADER_PROOFBOUND_PUBLISHABLE_KEY (optional;
//	  when empty the fixture default application must already be proof_bound)
//	STARLOADER_PROOFBOUND_PUBLIC_URL (optional canonical public entry point
//	  used for the DPoP htu; defaults to the base URL and must match the
//	  server PUBLIC_SCHEME/PUBLIC_HOST configuration)
//	STARLOADER_LEGACY_APP_ID + STARLOADER_LEGACY_PUBLISHABLE_KEY (optional;
//	  enables the parallel legacy-application section)
//	STARLOADER_PROOFBOUND_ADMIN_COOKIE (optional; when set the test attempts
//	  to provision the proof-bound profile via PATCH
//	  /v1/admin/applications/{id} before exercising the contract)
//
// Without a reachable native proof-enabled fixture the test skips instead
// of passing vacuously.

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/starloader/backend/internal/httpapi"
	"github.com/starloader/backend/internal/security"
)

// Compile-time proof that the client-side construction below targets the
// real server verifier contract.
var _ httpapi.ProofBoundTokenVerifier = (*security.ProofBoundTokenVerifier)(nil)

const proofBoundFixtureUnavailable = "native proof-enabled StarLoader fixture unavailable"

type proofBoundDeviceKey struct {
	private *ecdsa.PrivateKey
	jwk     string
	thumb   string
}

type proofBoundFixture struct {
	baseURL      string
	publicURL    string
	email        string
	password     string
	appHeaders   map[string]string
	legacyHeader map[string]string
	haveLegacy   bool
	adminCookie  string
}

func proofBoundBaseURL(t *testing.T) string {
	t.Helper()
	if value := strings.TrimRight(strings.TrimSpace(os.Getenv("STARLOADER_PROOFBOUND_BASE_URL")), "/"); value != "" {
		return value
	}
	if value := strings.TrimRight(strings.TrimSpace(os.Getenv("STARLOADER_SMOKE_BASE_URL")), "/"); value != "" {
		return value
	}
	t.Skip(proofBoundFixtureUnavailable + ": set STARLOADER_PROOFBOUND_BASE_URL (or STARLOADER_SMOKE_BASE_URL) with a proof-enabled fixture")
	return ""
}

func proofBoundEnv(t *testing.T, primary, fallback string) string {
	t.Helper()
	if value := strings.TrimSpace(os.Getenv(primary)); value != "" {
		return value
	}
	if fallback != "" {
		if value := strings.TrimSpace(os.Getenv(fallback)); value != "" {
			return value
		}
	}
	t.Skip(proofBoundFixtureUnavailable + ": set " + primary)
	return ""
}

func proofBoundAppHeaders(appID, key string) map[string]string {
	headers := map[string]string{}
	if strings.TrimSpace(appID) != "" {
		headers["X-KeyStar-App"] = strings.TrimSpace(appID)
	}
	if strings.TrimSpace(key) != "" {
		headers["Authorization"] = "Bearer " + strings.TrimSpace(key)
	}
	return headers
}

func newProofBoundDeviceKey(t *testing.T) *proofBoundDeviceKey {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	x := make([]byte, 32)
	y := make([]byte, 32)
	private.X.FillBytes(x)
	private.Y.FillBytes(y)
	encode := base64.RawURLEncoding.EncodeToString
	jwk := `{"crv":"P-256","kty":"EC","x":"` + encode(x) + `","y":"` + encode(y) + `"}`
	public, thumbprint, err := security.ParseP256JWK(json.RawMessage(jwk))
	if err != nil {
		t.Fatalf("ParseP256JWK(generated key): %v", err)
	}
	if public == nil || thumbprint == "" {
		t.Fatal("ParseP256JWK returned an empty key or thumbprint")
	}
	return &proofBoundDeviceKey{private: private, jwk: jwk, thumb: thumbprint}
}

// cngBlob renders the same P-256 key as a legacy CNG public blob so the
// server cross-check between tpm_public_key and device_jwk is exercised.
func (key *proofBoundDeviceKey) cngBlob() string {
	blob := make([]byte, 72)
	binary.LittleEndian.PutUint32(blob[:4], 0x31534345)
	binary.LittleEndian.PutUint32(blob[4:8], 32)
	key.private.X.FillBytes(blob[8:40])
	key.private.Y.FillBytes(blob[40:72])
	return base64.StdEncoding.EncodeToString(blob)
}

func (key *proofBoundDeviceKey) signChallenge(t *testing.T, challenge []byte) string {
	t.Helper()
	digest := sha256.Sum256(challenge)
	r, s, err := ecdsa.Sign(rand.Reader, key.private, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return base64.StdEncoding.EncodeToString(signature)
}

func proofBoundPostJSON(t *testing.T, url string, headers map[string]string, value any) response {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	for name, header := range headers {
		if strings.EqualFold(name, "Authorization") {
			continue
		}
		request.Header.Set(name, header)
	}
	if authorization, ok := headers["Authorization"]; ok {
		request.Header.Set("Authorization", authorization)
	}
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	var responseBody bytes.Buffer
	if _, err := responseBody.ReadFrom(result.Body); err != nil {
		t.Fatal(err)
	}
	return response{status: result.StatusCode, body: responseBody.Bytes(), requestID: result.Header.Get("X-Request-ID")}
}

func proofBoundGetWithDPoP(t *testing.T, url, token, proof string, headers map[string]string) response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "DPoP "+token)
	request.Header.Set("DPoP", proof)
	for name, header := range headers {
		if strings.EqualFold(name, "Authorization") {
			continue
		}
		request.Header.Set(name, header)
	}
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	var responseBody bytes.Buffer
	if _, err := responseBody.ReadFrom(result.Body); err != nil {
		t.Fatal(err)
	}
	return response{status: result.StatusCode, body: responseBody.Bytes(), requestID: result.Header.Get("X-Request-ID")}
}

// mintProofBoundDPoP builds a client DPoP proof with the real JWK binding
// rules the server enforces (ES256, embedded P-256 JWK, r||s signature,
// htm/htu/ath/iat/jti). Callers must use a fresh 128-bit jti per request.
func mintProofBoundDPoP(t *testing.T, key *proofBoundDeviceKey, accessToken, method, htu string, issuedAt time.Time) (proof, jti string) {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	jti = base64.RawURLEncoding.EncodeToString(raw)
	accessDigest := sha256.Sum256([]byte(accessToken))
	header := `{"alg":"ES256","jwk":` + key.jwk + `,"typ":"dpop+jwt"}`
	payload := `{"ath":"` + base64.RawURLEncoding.EncodeToString(accessDigest[:]) +
		`","htm":"` + method + `","htu":"` + htu + `","iat":` + proofBoundInt64(issuedAt.Unix()) +
		`,"jti":"` + jti + `"}`
	signingInput := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(payload))
	inputDigest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key.private, inputDigest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	proof = signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
	return proof, jti
}

func proofBoundInt64(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

type proofBoundTokenView struct {
	headerKID     string
	subject       string
	applicationID string
	issuedAt      int64
	expiresAt     int64
	notBefore     int64
	sessionID     string
	tokenID       string
	keyThumbprint string
}

func decodeProofBoundToken(t *testing.T, token string) proofBoundTokenView {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("proof-bound token is not compact")
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode proof-bound header: %v", err)
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
		KID       string `json:"kid"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		t.Fatalf("decode proof-bound header JSON: %v", err)
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode proof-bound payload: %v", err)
	}
	var payload struct {
		Subject       string `json:"sub"`
		ApplicationID string `json:"app"`
		IssuedAt      int64  `json:"iat"`
		ExpiresAt     int64  `json:"exp"`
		SessionID     string `json:"sid"`
		TokenID       string `json:"jti"`
		NotBefore     int64  `json:"nbf"`
		Confirmation  struct {
			JKT string `json:"jkt"`
		} `json:"cnf"`
	}
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		t.Fatalf("decode proof-bound payload JSON: %v", err)
	}
	return proofBoundTokenView{
		headerKID:     header.KID,
		subject:       payload.Subject,
		applicationID: payload.ApplicationID,
		issuedAt:      payload.IssuedAt,
		expiresAt:     payload.ExpiresAt,
		notBefore:     payload.NotBefore,
		sessionID:     payload.SessionID,
		tokenID:       payload.TokenID,
		keyThumbprint: payload.Confirmation.JKT,
	}
}

func assertProofBoundRejected(t *testing.T, result response) {
	t.Helper()
	if result.status != http.StatusUnauthorized {
		t.Fatalf("proof-bound rejection status = %d body = %s", result.status, result.body)
	}
	assertUUIDv7(t, result.requestID)
	var errorResponse struct {
		OK        bool   `json:"ok"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(result.body, &errorResponse); err != nil {
		t.Fatalf("decode proof-bound rejection: %v", err)
	}
	assertExactJSONKeys(t, result.body, []string{"code", "message", "ok", "request_id"})
	if errorResponse.OK || errorResponse.Code != "INVALID_SESSION_TOKEN" || errorResponse.Message != "invalid session token" {
		t.Fatalf("proof-bound rejection is not the exact safe contract: %s", result.body)
	}
	if errorResponse.RequestID != result.requestID {
		t.Fatal("proof-bound rejection header/body request IDs do not match")
	}
	if strings.Contains(string(result.body), "jkt") || strings.Contains(string(result.body), "jti") {
		t.Fatal("proof-bound rejection reflects proof material")
	}
}

func TestProofBoundApplicationAuth(t *testing.T) {
	baseURL := proofBoundBaseURL(t)
	email := proofBoundEnv(t, "STARLOADER_PROOFBOUND_EMAIL", "STARLOADER_SMOKE_EMAIL")
	password := proofBoundEnv(t, "STARLOADER_PROOFBOUND_PASSWORD", "STARLOADER_SMOKE_PASSWORD")
	publicURL := strings.TrimRight(strings.TrimSpace(os.Getenv("STARLOADER_PROOFBOUND_PUBLIC_URL")), "/")
	if publicURL == "" {
		publicURL = baseURL
	}
	fixture := proofBoundFixture{
		baseURL:     baseURL,
		publicURL:   publicURL,
		email:       email,
		password:    password,
		appHeaders:  proofBoundAppHeaders(os.Getenv("STARLOADER_PROOFBOUND_APP_ID"), os.Getenv("STARLOADER_PROOFBOUND_PUBLISHABLE_KEY")),
		adminCookie: strings.TrimSpace(os.Getenv("STARLOADER_PROOFBOUND_ADMIN_COOKIE")),
	}
	if legacyApp := strings.TrimSpace(os.Getenv("STARLOADER_LEGACY_APP_ID")); legacyApp != "" {
		fixture.legacyHeader = proofBoundAppHeaders(legacyApp, os.Getenv("STARLOADER_LEGACY_PUBLISHABLE_KEY"))
		fixture.haveLegacy = true
	} else if len(fixture.appHeaders) != 0 {
		// Proof-bound traffic selects its application explicitly, so the
		// fixture default application remains the parallel legacy peer.
		fixture.legacyHeader = map[string]string{}
		fixture.haveLegacy = true
	}

	healthResponse, err := http.Get(baseURL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	healthResponse.Body.Close()
	if healthResponse.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", healthResponse.StatusCode)
	}

	if fixture.adminCookie != "" {
		if appID := strings.TrimSpace(os.Getenv("STARLOADER_PROOFBOUND_APP_ID")); appID != "" {
			provision := proofBoundPostJSON(t, baseURL+"/v1/admin/applications/"+appID, map[string]string{
				"X-KeyStar-App": appID,
				"Cookie":        fixture.adminCookie,
			}, map[string]any{"auth_profile": "proof_bound"})
			t.Logf("provision proof_bound profile: status = %d body = %s", provision.status, provision.body)
		} else {
			t.Log("admin cookie set but STARLOADER_PROOFBOUND_APP_ID is empty; fixture application must already be provisioned as proof_bound")
		}
	}

	deviceKey := newProofBoundDeviceKey(t)

	loginResponse := proofBoundPostJSON(t, baseURL+"/v1/auth/login", fixture.appHeaders, map[string]any{
		"email": email, "password": password,
		"device_fingerprint": "proofbound-device-fingerprint",
	})
	if loginResponse.status != http.StatusOK {
		t.Fatalf("login status = %d body = %s", loginResponse.status, loginResponse.body)
	}
	assertUUIDv7(t, loginResponse.requestID)
	var pending struct {
		SessionID string `json:"session_id"`
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(loginResponse.body, &pending); err != nil {
		t.Fatal(err)
	}
	assertUUIDv7(t, pending.SessionID)
	challenge, err := base64.StdEncoding.DecodeString(pending.Challenge)
	if err != nil || len(challenge) != 32 {
		t.Fatal("server challenge is invalid")
	}

	verifyBody := map[string]any{
		"session_id": pending.SessionID, "challenge": pending.Challenge,
		"challenge_signature": deviceKey.signChallenge(t, challenge),
		"tpm_public_key":      deviceKey.cngBlob(),
		"device_jwk":          json.RawMessage(deviceKey.jwk),
		"hardware": map[string]string{
			"smbios_uuid": "proofbound-smbios", "motherboard_serial": "proofbound-board",
			"bios_serial": "proofbound-bios", "system_disk_serial": "proofbound-disk",
			"machine_guid": "proofbound-guid", "fingerprint": "proofbound-device-fingerprint",
		},
	}
	verificationResponse := proofBoundPostJSON(t, baseURL+"/v1/device/verify", fixture.appHeaders, verifyBody)
	if verificationResponse.status != http.StatusOK {
		t.Fatalf("device verify status = %d body = %s", verificationResponse.status, verificationResponse.body)
	}
	assertUUIDv7(t, verificationResponse.requestID)
	var rawVerify map[string]json.RawMessage
	if err := json.Unmarshal(verificationResponse.body, &rawVerify); err != nil {
		t.Fatal(err)
	}
	if _, present := rawVerify["refresh_token"]; present {
		t.Fatal("proof-bound verify response must not contain refresh_token")
	}
	var verified struct {
		Token          string `json:"token"`
		TokenExpiresAt string `json:"token_expires_at"`
		LicenseID      string `json:"license_id"`
		DeviceID       string `json:"device_id"`
	}
	if err := json.Unmarshal(verificationResponse.body, &verified); err != nil {
		t.Fatal(err)
	}
	assertUUIDv7(t, verified.LicenseID)
	assertUUIDv7(t, verified.DeviceID)

	view := decodeProofBoundToken(t, verified.Token)
	if view.headerKID == "" {
		t.Fatal("proof-bound token header is missing the active application signing key kid")
	}
	if view.expiresAt-view.issuedAt != 600 {
		t.Fatalf("proof-bound exp-iat = %d, want exactly 600", view.expiresAt-view.issuedAt)
	}
	if view.notBefore != view.issuedAt {
		t.Fatal("proof-bound nbf must equal iat")
	}
	if view.keyThumbprint != deviceKey.thumb {
		t.Fatal("proof-bound cnf.jkt does not match the verified TPM JWK thumbprint")
	}
	if view.sessionID == "" || view.tokenID == "" {
		t.Fatal("proof-bound token is missing sid or jti")
	}
	expiresAt, err := time.Parse(time.RFC3339, verified.TokenExpiresAt)
	if err != nil || expiresAt.Unix() != view.expiresAt {
		t.Fatal("token_expires_at does not match the 600-second token expiry")
	}

	// Client-side sanity check with the real server proof helper before
	// any DPoP request leaves the test process.
	trialProof, _ := mintProofBoundDPoP(t, deviceKey, verified.Token, "GET", fixture.publicURL+"/v1/me", time.Now().UTC())
	if _, err := security.VerifyDPoP(security.DPoPInput{
		Proof: trialProof, AccessToken: verified.Token, Method: "GET",
		URI: fixture.publicURL + "/v1/me",
		Token: security.SessionClaims{
			ExpiresAt: time.Unix(view.expiresAt, 0).UTC(),
			ProofBound: &security.ProofBoundClaims{
				DeviceKeyThumbprint: deviceKey.thumb,
			},
		},
		Now: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("local VerifyDPoP sanity check: %v", err)
	}

	htu := fixture.publicURL + "/v1/me"
	proof, _ := mintProofBoundDPoP(t, deviceKey, verified.Token, "GET", htu, time.Now().UTC())
	profileResponse := proofBoundGetWithDPoP(t, baseURL+"/v1/me", verified.Token, proof, fixture.appHeaders)
	if profileResponse.status != http.StatusOK {
		t.Fatalf("DPoP profile status = %d body = %s", profileResponse.status, profileResponse.body)
	}
	assertUUIDv7(t, profileResponse.requestID)
	var profile struct {
		OK    bool   `json:"ok"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(profileResponse.body, &profile); err != nil {
		t.Fatal(err)
	}
	if !profile.OK || profile.Email != email {
		t.Fatalf("DPoP profile is not bound to the verified account: %s", profileResponse.body)
	}

	// Identical proof replay must be rejected with the safe contract.
	assertProofBoundRejected(t, proofBoundGetWithDPoP(t, baseURL+"/v1/me", verified.Token, proof, fixture.appHeaders))

	// A stolen token presented with a different device key must be rejected.
	otherKey := newProofBoundDeviceKey(t)
	stolenProof, _ := mintProofBoundDPoP(t, otherKey, verified.Token, "GET", htu, time.Now().UTC())
	assertProofBoundRejected(t, proofBoundGetWithDPoP(t, baseURL+"/v1/me", verified.Token, stolenProof, fixture.appHeaders))

	// A stale proof outside the clock-skew window must be rejected (token
	// expiry itself is pinned by the exp-iat==600 assertion above).
	staleProof, _ := mintProofBoundDPoP(t, deviceKey, verified.Token, "GET", htu, time.Now().UTC().Add(-5*time.Minute))
	assertProofBoundRejected(t, proofBoundGetWithDPoP(t, baseURL+"/v1/me", verified.Token, staleProof, fixture.appHeaders))

	// Proof-bound sessions never use bearer fallback.
	bearerRequest, err := http.NewRequest(http.MethodGet, baseURL+"/v1/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	bearerRequest.Header.Set("Authorization", "Bearer "+verified.Token)
	bearerResult, err := http.DefaultClient.Do(bearerRequest)
	if err != nil {
		t.Fatal(err)
	}
	var bearerBody bytes.Buffer
	if _, err := bearerBody.ReadFrom(bearerResult.Body); err != nil {
		t.Fatal(err)
	}
	bearerResult.Body.Close()
	assertProofBoundRejected(t, response{status: bearerResult.StatusCode, body: bearerBody.Bytes(), requestID: bearerResult.Header.Get("X-Request-ID")})

	// Proof-bound applications reject refresh with generic unauthorized and
	// issue no tokens.
	refreshResponse := proofBoundPostJSON(t, baseURL+"/v1/auth/refresh", fixture.appHeaders, map[string]any{
		"refresh_token": "proof-bound-has-no-refresh-token",
	})
	if refreshResponse.status != http.StatusUnauthorized {
		t.Fatalf("refresh status = %d body = %s", refreshResponse.status, refreshResponse.body)
	}
	var refreshError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(refreshResponse.body, &refreshError); err != nil {
		t.Fatal(err)
	}
	if refreshError.Code != "INVALID_REFRESH_TOKEN" || refreshError.Message != "invalid refresh token" {
		t.Fatalf("refresh rejection is not generic: %s", refreshResponse.body)
	}
	if strings.Contains(string(refreshResponse.body), "access_token") {
		t.Fatal("refresh rejection must not issue tokens")
	}

	// Only one of two identical concurrent proof submissions may succeed.
	raceProof, _ := mintProofBoundDPoP(t, deviceKey, verified.Token, "GET", htu, time.Now().UTC())
	var okCount, rejectedCount atomic.Int64
	var raceGroup sync.WaitGroup
	for i := 0; i < 2; i++ {
		raceGroup.Add(1)
		go func() {
			defer raceGroup.Done()
			result := proofBoundGetWithDPoP(t, baseURL+"/v1/me", verified.Token, raceProof, fixture.appHeaders)
			switch result.status {
			case http.StatusOK:
				okCount.Add(1)
			case http.StatusUnauthorized:
				rejectedCount.Add(1)
			default:
				t.Errorf("concurrent DPoP status = %d body = %s", result.status, result.body)
			}
		}()
	}
	raceGroup.Wait()
	if okCount.Load() != 1 || rejectedCount.Load() != 1 {
		t.Fatalf("concurrent identical proofs: ok = %d, rejected = %d, want exactly 1 and 1", okCount.Load(), rejectedCount.Load())
	}

	if !fixture.haveLegacy {
		t.Log("parallel legacy application not configured (set STARLOADER_LEGACY_APP_ID); legacy isolation asserted by unit suites")
		return
	}
	t.Run("parallel legacy application", func(t *testing.T) {
		login := proofBoundPostJSON(t, baseURL+"/v1/auth/login", fixture.legacyHeader, map[string]any{
			"email": email, "password": password,
			"device_fingerprint": "proofbound-legacy-fingerprint",
		})
		if login.status != http.StatusOK {
			t.Fatalf("legacy login status = %d body = %s", login.status, login.body)
		}
		var legacyPending struct {
			SessionID string `json:"session_id"`
			Challenge string `json:"challenge"`
		}
		if err := json.Unmarshal(login.body, &legacyPending); err != nil {
			t.Fatal(err)
		}
		legacyChallenge, err := base64.StdEncoding.DecodeString(legacyPending.Challenge)
		if err != nil || len(legacyChallenge) != 32 {
			t.Fatal("legacy server challenge is invalid")
		}
		legacyBlob, legacySignature := cngProof(t, legacyChallenge)
		legacyVerify := proofBoundPostJSON(t, baseURL+"/v1/device/verify", fixture.legacyHeader, map[string]any{
			"session_id": legacyPending.SessionID, "challenge": legacyPending.Challenge,
			"challenge_signature": base64.StdEncoding.EncodeToString(legacySignature),
			"tpm_public_key":      base64.StdEncoding.EncodeToString(legacyBlob),
			"hardware": map[string]string{
				"smbios_uuid": "legacy-smbios", "motherboard_serial": "legacy-board",
				"bios_serial": "legacy-bios", "system_disk_serial": "legacy-disk",
				"machine_guid": "legacy-guid", "fingerprint": "proofbound-legacy-fingerprint",
			},
		})
		if legacyVerify.status != http.StatusOK {
			t.Fatalf("legacy device verify status = %d body = %s", legacyVerify.status, legacyVerify.body)
		}
		var legacyVerified struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(legacyVerify.body, &legacyVerified); err != nil {
			t.Fatal(err)
		}
		legacyMe := getWithBearer(t, baseURL+"/v1/me", legacyVerified.Token)
		if legacyMe.status != http.StatusOK {
			t.Fatalf("legacy bearer profile status = %d body = %s", legacyMe.status, legacyMe.body)
		}
		// The legacy bearer token must not cross into the proof-bound flow.
		assertProofBoundRejected(t, proofBoundGetWithDPoP(t, baseURL+"/v1/me", legacyVerified.Token, raceProof, fixture.appHeaders))
	})
}
