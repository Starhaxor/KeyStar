package httpapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
)

type fakeProofBoundVerifier struct {
	claims       security.SessionClaims
	err          error
	calls        int
	gotAppID     string
	gotToken     string
}

func (fake *fakeProofBoundVerifier) Verify(_ context.Context, applicationID, token string) (security.SessionClaims, error) {
	fake.calls++
	fake.gotAppID = applicationID
	fake.gotToken = token
	return fake.claims, fake.err
}

type fakeReplayStore struct {
	consumed  bool
	err       error
	calls     int
	gotApp    string
	gotDigest [32]byte
	gotToken  string
}

func (fake *fakeReplayStore) ConsumeDPoP(_ context.Context, applicationID string, jtiDigest [32]byte, tokenID string, _ time.Time) (bool, error) {
	fake.calls++
	fake.gotApp = applicationID
	fake.gotDigest = jtiDigest
	fake.gotToken = tokenID
	return fake.consumed, fake.err
}

type dpopMiddlewareKey struct {
	private *ecdsa.PrivateKey
	jwk     string
	thumb   string
}

func newDPoPMiddlewareKey(t *testing.T) *dpopMiddlewareKey {
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
	_, thumb, err := security.ParseP256JWK([]byte(jwk))
	if err != nil {
		t.Fatal(err)
	}
	return &dpopMiddlewareKey{private: private, jwk: jwk, thumb: thumb}
}

func (key *dpopMiddlewareKey) mintProof(t *testing.T, accessToken, method, htu, jti string, iat int64) string {
	t.Helper()
	digest := sha256.Sum256([]byte(accessToken))
	header := `{"alg":"ES256","jwk":` + key.jwk + `,"typ":"dpop+jwt"}`
	payload := `{"ath":"` + base64.RawURLEncoding.EncodeToString(digest[:]) + `","htm":"` + method + `","htu":"` + htu + `","iat":` + itoa(iat) + `,"jti":"` + jti + `"}`
	signingInput := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(payload))
	inputDigest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key.private, inputDigest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func itoa(value int64) string {
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

func proofBoundMiddlewareClaims(now time.Time, thumb string) security.SessionClaims {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return security.SessionClaims{
		Subject: "user-1", ApplicationID: middlewareTestApplicationID, LicenseID: "license-1", DeviceID: "device-1",
		Product: "StarLoader", Issuer: "keystar", Audience: "starloader-client",
		IssuedAt: now, ExpiresAt: now.Add(600 * time.Second),
		ProofBound: &security.ProofBoundClaims{
			SessionID: "session-1", TokenID: base64.RawURLEncoding.EncodeToString(raw),
			DeviceKeyThumbprint: thumb, NotBefore: now,
		},
	}
}

func randomCanonicalJTI(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// unsignedAccessToken builds a JWT-shaped access token carrying the signed
// application hint. Signatures are the fake verifier's duty in these tests;
// the middleware only reads the unverified routing hint before authoritative
// verification.
func unsignedAccessToken(applicationID string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"app":"` + applicationID + `"}`))
	return "dpop." + payload + ".unsigned"
}

func sessionAuthConfig(legacy BearerVerifier) SessionAuthConfig {
	return SessionAuthConfig{
		LegacyVerifier: legacy,
		Now:            time.Now,
		PublicScheme:   "https",
		PublicHost:     "api.example.com",
	}
}

func TestRequireSessionProofBoundDPoPSuccess(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	key := newDPoPMiddlewareKey(t)
	claims := proofBoundMiddlewareClaims(now, key.thumb)
	proofVerifier := &fakeProofBoundVerifier{claims: claims}
	replays := &fakeReplayStore{consumed: true}
	config := sessionAuthConfig(&fakeBearerVerifier{err: errors.New("must not verify bearer")})
	config.ProofBoundVerifier = proofVerifier
	config.Applications = &middlewareTestApplicationResolver{application: proofBoundApplication()}
	config.Replays = replays
	config.Now = func() time.Time { return now }

	jti := randomCanonicalJTI(t)
	accessToken := unsignedAccessToken(middlewareTestApplicationID)
	proof := key.mintProof(t, accessToken, "GET", "https://api.example.com/v1/me", jti, now.Unix())
	var gotClaims security.SessionClaims
	var found bool
	handler := RequireSession(config, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		gotClaims, found = SessionClaimsFromContext(request.Context())
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://api.example.com/v1/me?user_id=attacker", nil)
	request.Header.Set("Authorization", "DPoP "+accessToken)
	request.Header.Set("DPoP", proof)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !found || gotClaims.ApplicationID != middlewareTestApplicationID || gotClaims.ProofBound == nil {
		t.Fatalf("SessionClaimsFromContext() = %#v, found = %t", gotClaims, found)
	}
	if proofVerifier.calls != 1 || proofVerifier.gotAppID != middlewareTestApplicationID || proofVerifier.gotToken != accessToken {
		t.Fatalf("proof-bound verifier calls = %d, app = %q, token = %q", proofVerifier.calls, proofVerifier.gotAppID, proofVerifier.gotToken)
	}
	if replays.calls != 1 || replays.gotApp != middlewareTestApplicationID || replays.gotToken != claims.ProofBound.TokenID {
		t.Fatalf("replay consumption = calls %d app %q token %q", replays.calls, replays.gotApp, replays.gotToken)
	}
	wantDigest := sha256.Sum256([]byte(jti))
	if replays.gotDigest != wantDigest {
		t.Fatal("replay store did not receive the proof jti digest")
	}
}

func TestRequireSessionDPoPRequiresExactlyOneOfEachHeader(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	build := func() (SessionAuthConfig, *fakeReplayStore, *fakeProofBoundVerifier) {
		key := newDPoPMiddlewareKey(t)
		proofVerifier := &fakeProofBoundVerifier{claims: proofBoundMiddlewareClaims(now, key.thumb)}
		replays := &fakeReplayStore{consumed: true}
		config := sessionAuthConfig(&fakeBearerVerifier{})
		config.ProofBoundVerifier = proofVerifier
		config.Applications = &middlewareTestApplicationResolver{application: proofBoundApplication()}
		config.Replays = replays
		config.Now = func() time.Time { return now }
		return config, replays, proofVerifier
	}
	for _, test := range []struct {
		name    string
		headers func(request *http.Request)
	}{
		{name: "missing DPoP header", headers: func(request *http.Request) {
			request.Header.Set("Authorization", "DPoP proof-access-token")
		}},
		{name: "duplicate DPoP header", headers: func(request *http.Request) {
			request.Header.Set("Authorization", "DPoP proof-access-token")
			request.Header.Add("DPoP", "proof-one")
			request.Header.Add("DPoP", "proof-two")
		}},
		{name: "duplicate Authorization header", headers: func(request *http.Request) {
			request.Header.Add("Authorization", "DPoP proof-access-token")
			request.Header.Add("Authorization", "DPoP proof-access-token")
		}},
		{name: "unknown scheme", headers: func(request *http.Request) {
			request.Header.Set("Authorization", "MAC proof-access-token")
			request.Header.Set("DPoP", "proof")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, replays, proofVerifier := build()
			nextCalled := false
			handler := RequireSession(config, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				nextCalled = true
			}))
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "https://api.example.com/v1/me", nil)
			test.headers(request)

			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body = %s", recorder.Code, recorder.Body.String())
			}
			if nextCalled {
				t.Fatal("downstream handler was called")
			}
			if replays.calls != 0 || proofVerifier.calls != 0 {
				t.Fatal("header failure reached verification or replay work")
			}
			assertErrorCode(t, recorder, "INVALID_SESSION_TOKEN")
		})
	}
}

func TestRequireSessionReplayFailuresNeverInvokeHandler(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, test := range []struct {
		name     string
		consumed bool
		err      error
		status   int
	}{
		{name: "replay conflict", consumed: false, status: http.StatusUnauthorized},
		{name: "database error fails closed", consumed: false, err: errors.New("connection reset"), status: http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			key := newDPoPMiddlewareKey(t)
			proofVerifier := &fakeProofBoundVerifier{claims: proofBoundMiddlewareClaims(now, key.thumb)}
			replays := &fakeReplayStore{consumed: test.consumed, err: test.err}
			config := sessionAuthConfig(&fakeBearerVerifier{})
			config.ProofBoundVerifier = proofVerifier
			config.Applications = &middlewareTestApplicationResolver{application: proofBoundApplication()}
			config.Replays = replays
			config.Now = func() time.Time { return now }
			nextCalled := false
			handler := RequireSession(config, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				nextCalled = true
			}))
			jti := randomCanonicalJTI(t)
			accessToken := unsignedAccessToken(middlewareTestApplicationID)
			proof := key.mintProof(t, accessToken, "GET", "https://api.example.com/v1/me", jti, now.Unix())
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "https://api.example.com/v1/me", nil)
			request.Header.Set("Authorization", "DPoP "+accessToken)
			request.Header.Set("DPoP", proof)

			handler.ServeHTTP(recorder, request)

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, test.status, recorder.Body.String())
			}
			if nextCalled {
				t.Fatal("downstream handler was called after replay failure")
			}
		})
	}
}

func TestRequireSessionRejectsBearerForProofBoundWithoutRetry(t *testing.T) {
	legacy := &fakeBearerVerifier{claims: security.SessionClaims{
		Subject: "user-1", ApplicationID: middlewareTestApplicationID, LicenseID: "license-1", DeviceID: "device-1",
		Product: "StarLoader", Issuer: "keystar", Audience: "starloader-client",
		IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour),
	}}
	config := sessionAuthConfig(legacy)
	config.Applications = &middlewareTestApplicationResolver{application: proofBoundApplication()}
	nextCalled := false
	handler := RequireSession(config, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://api.example.com/v1/me", nil)
	request.Header.Set("Authorization", "Bearer legacy-token")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", recorder.Code, recorder.Body.String())
	}
	if nextCalled {
		t.Fatal("proof-bound application admitted a bearer token")
	}
}

func TestRequireSessionLegacyRejectsDPoPAndKeepsBearer(t *testing.T) {
	legacyClaims := security.SessionClaims{
		Subject: "user-1", ApplicationID: middlewareTestApplicationID, LicenseID: "license-1", DeviceID: "device-1",
		Product: "StarLoader", Issuer: "keystar", Audience: "starloader-client",
		IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour),
	}
	legacy := &fakeBearerVerifier{claims: legacyClaims}
	proofVerifier := &fakeProofBoundVerifier{claims: proofBoundMiddlewareClaims(time.Now().UTC(), "unused")}
	legacyApp := &domain.Application{
		ID: middlewareTestApplicationID, OrganizationID: "org-1", Status: domain.ApplicationStatusActive,
		AuthProfile: domain.ApplicationAuthLegacy,
	}

	t.Run("bearer still accepted", func(t *testing.T) {
		config := sessionAuthConfig(legacy)
		config.Applications = &middlewareTestApplicationResolver{application: legacyApp}
		var gotClaims security.SessionClaims
		var found bool
		handler := RequireSession(config, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			gotClaims, found = SessionClaimsFromContext(request.Context())
		}))
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
		request.Header.Set("Authorization", "Bearer legacy-token")

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK || !found || gotClaims.Subject != "user-1" {
			t.Fatalf("status = %d, found = %t, claims = %#v", recorder.Code, found, gotClaims)
		}
	})

	t.Run("dpop rejected without proof verification", func(t *testing.T) {
		config := sessionAuthConfig(legacy)
		config.ProofBoundVerifier = proofVerifier
		config.Applications = &middlewareTestApplicationResolver{application: legacyApp}
		config.Replays = &fakeReplayStore{consumed: true}
		nextCalled := false
		handler := RequireSession(config, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		}))
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "https://api.example.com/v1/me", nil)
		request.Header.Set("Authorization", "DPoP some-token")
		request.Header.Set("DPoP", "some-proof")

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401; body = %s", recorder.Code, recorder.Body.String())
		}
		if nextCalled || proofVerifier.calls != 0 {
			t.Fatalf("legacy application ran proof verification (calls=%d) or invoked handler", proofVerifier.calls)
		}
	})
}

func assertErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, code string) {
	t.Helper()
	var response errorResponse
	decodeResponse(t, recorder, &response)
	if response.Code != code {
		t.Fatalf("response = %#v, want code %q", response, code)
	}
	if response.Message != "invalid session token" {
		t.Fatalf("response message = %q, want generic", response.Message)
	}
}
