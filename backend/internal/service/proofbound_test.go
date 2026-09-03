package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
	"github.com/starloader/backend/internal/security"
)

type fakeApplicationResolver struct {
	profile domain.ApplicationAuthProfile
	err     error
	calls   int
}

func (fake *fakeApplicationResolver) FindApplicationByID(_ context.Context, applicationID string) (*domain.Application, error) {
	fake.calls++
	if fake.err != nil {
		return nil, fake.err
	}
	return &domain.Application{ID: applicationID, AuthProfile: fake.profile}, nil
}

type fakeProofBoundIssuer struct {
	token      string
	expiresAt  time.Time
	err        error
	calls      int
	gotAppID   string
	gotClaims  security.SessionClaims
}

func (fake *fakeProofBoundIssuer) IssueProofBound(_ context.Context, applicationID string, claims security.SessionClaims) (string, time.Time, error) {
	fake.calls++
	fake.gotAppID = applicationID
	fake.gotClaims = claims
	return fake.token, fake.expiresAt, fake.err
}

func proofBoundJWK(t *testing.T, key *ecdsa.PrivateKey) json.RawMessage {
	t.Helper()
	encode := func(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }
	x := make([]byte, 32)
	y := make([]byte, 32)
	key.X.FillBytes(x)
	key.Y.FillBytes(y)
	return json.RawMessage(`{"crv":"P-256","kty":"EC","x":"` + encode(x) + `","y":"` + encode(y) + `"}`)
}

func signChallengeRaw(t *testing.T, key *ecdsa.PrivateKey, challenge []byte) string {
	t.Helper()
	digest := sha256.Sum256(challenge)
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return base64.StdEncoding.EncodeToString(signature)
}

func newProofBoundFixture(t *testing.T, now time.Time) (*fakeDeviceRepository, VerifyInput, *ecdsa.PrivateKey) {
	t.Helper()
	repository, input := newVerificationFixture(t, now, 1)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := base64.StdEncoding.DecodeString(input.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	input.ChallengeSignature = signChallengeRaw(t, key, challenge)
	input.DeviceJWK = proofBoundJWK(t, key)
	// Proof-bound clients bind device_jwk, not the legacy CNG blob.
	input.TPMPublicKey = ""
	return repository, input, key
}

func newProofBoundDeviceService(t *testing.T, repository DeviceRepository, now time.Time, issuer *fakeProofBoundIssuer) (*DeviceService, *fakeApplicationResolver) {
	t.Helper()
	resolver := &fakeApplicationResolver{profile: domain.ApplicationAuthProofBound}
	service := NewDeviceService(repository, DeviceServiceConfig{
		HardwareHMACKey:     []byte("hardware-secret"),
		Issuer:              "starloader",
		Audience:            "starloader-client",
		Product:             "StarLoader",
		Now:                 func() time.Time { return now },
		ApplicationResolver: resolver,
		ProofBoundIssuer:    issuer,
	})
	return service, resolver
}

func TestDeviceVerifyProofBoundIssuesKeyBoundTokenWithoutRefresh(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repository, input, key := newProofBoundFixture(t, now)
	issuer := &fakeProofBoundIssuer{token: "proof-token", expiresAt: now.Add(600 * time.Second)}
	service, _ := newProofBoundDeviceService(t, repository, now, issuer)

	verified, err := service.Verify(context.Background(), input)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.Token != "proof-token" || !verified.ExpiresAt.Equal(now.Add(600*time.Second)) {
		t.Fatalf("verified session = %#v", verified)
	}
	if verified.RefreshToken != "" {
		t.Fatalf("proof-bound verification issued a refresh token: %q", verified.RefreshToken)
	}
	if issuer.calls != 1 || issuer.gotAppID != "app-1" {
		t.Fatalf("proof-bound issuer calls = %d, app = %q", issuer.calls, issuer.gotAppID)
	}
	_, wantThumbprint, err := security.ParseP256JWK(proofBoundJWK(t, key))
	if err != nil {
		t.Fatal(err)
	}
	got := issuer.gotClaims
	if got.ProofBound == nil || got.ProofBound.DeviceKeyThumbprint != wantThumbprint {
		t.Fatalf("issued thumbprint = %#v, want %q", got.ProofBound, wantThumbprint)
	}
	if got.ProofBound.SessionID != "session-1" || got.ApplicationID != "app-1" {
		t.Fatalf("issued bindings = %#v", got)
	}
}

func TestDeviceVerifyProofBoundRequiresJWK(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repository, input, _ := newProofBoundFixture(t, now)
	input.DeviceJWK = nil
	issuer := &fakeProofBoundIssuer{}
	service, _ := newProofBoundDeviceService(t, repository, now, issuer)

	if _, err := service.Verify(context.Background(), input); !errors.Is(err, ErrInvalidVerifyRequest) {
		t.Fatalf("Verify() error = %v, want %v", err, ErrInvalidVerifyRequest)
	}
	if repository.calls != 0 || issuer.calls != 0 {
		t.Fatalf("missing JWK reached transaction (%d) or issuer (%d)", repository.calls, issuer.calls)
	}
}

func TestDeviceVerifyProofBoundRejectsForeignKeySignature(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repository, input, _ := newProofBoundFixture(t, now)
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	input.DeviceJWK = proofBoundJWK(t, other)
	issuer := &fakeProofBoundIssuer{}
	service, _ := newProofBoundDeviceService(t, repository, now, issuer)

	if _, err := service.Verify(context.Background(), input); !errors.Is(err, ErrInvalidDeviceSignature) {
		t.Fatalf("Verify() error = %v, want %v", err, ErrInvalidDeviceSignature)
	}
	if issuer.calls != 0 {
		t.Fatal("forged signature reached the proof-bound issuer")
	}
}

func TestDeviceVerifyProofBoundRejectsMismatchedTPMBlob(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repository, input, _ := newProofBoundFixture(t, now)
	// A stale legacy blob that does not match the verified JWK fails closed.
	repository2, stale := newVerificationFixture(t, now, 1)
	_ = repository2
	input.TPMPublicKey = stale.TPMPublicKey
	issuer := &fakeProofBoundIssuer{}
	service, _ := newProofBoundDeviceService(t, repository, now, issuer)

	if _, err := service.Verify(context.Background(), input); !errors.Is(err, ErrInvalidDeviceSignature) {
		t.Fatalf("Verify() error = %v, want %v", err, ErrInvalidDeviceSignature)
	}
	if issuer.calls != 0 {
		t.Fatal("mismatched TPM blob reached the proof-bound issuer")
	}
}

func TestRefreshRejectsProofBoundApplication(t *testing.T) {
	repository := newFakeRefreshRepository()
	issuer := &fakeSessionTokenIssuer{token: "access"}
	service := NewRefreshService(RefreshServiceConfig{Repository: repository, Profile: repository,
		HMACKey: []byte("hmac-key"), TokenIssuer: issuer,
		Issuer:  "starloader", Audience: "starloader-client", Product: "StarLoader",
		ApplicationResolver: &fakeApplicationResolver{profile: domain.ApplicationAuthProofBound}})
	// Plant a refresh session issued before the application migrated to
	// proof_bound; rotation must still be rejected.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := repository.CreateRefreshSession(context.Background(), domain.NewRefreshSession{
		ApplicationID: "app-1", UserID: "user-1", LicenseID: "license-1", DeviceID: "device-1",
		TokenHash: hashRefreshToken(raw), ExpiresAt: now.Add(24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	before := len(repository.sessions)
	if _, err := service.Refresh(context.Background(), RefreshInput{ApplicationID: "app-1", RefreshToken: token}); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("Refresh() error = %v, want %v", err, ErrInvalidRefreshToken)
	}
	if len(repository.sessions) != before {
		t.Fatal("proof-bound refresh issued a replacement session")
	}
	for _, session := range repository.sessions {
		if session.Status == domain.RefreshSessionStatusRotated {
			t.Fatal("proof-bound refresh rotated the presented token")
		}
	}
}

func TestRefreshIssuanceRejectsProofBoundApplication(t *testing.T) {
	repository := newFakeRefreshRepository()
	service := NewRefreshService(RefreshServiceConfig{Repository: repository, Profile: repository,
		HMACKey: []byte("hmac-key"), TokenIssuer: &fakeSessionTokenIssuer{token: "access"},
		Issuer:  "starloader", Audience: "starloader-client", Product: "StarLoader",
		ApplicationResolver: &fakeApplicationResolver{profile: domain.ApplicationAuthProofBound}})

	if _, _, err := service.IssueRefreshToken(context.Background(), "app-1", "user-1", "license-1", "device-1"); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("IssueRefreshToken() error = %v, want %v", err, ErrInvalidRefreshToken)
	}
	if len(repository.sessions) != 0 {
		t.Fatal("proof-bound issuance persisted a refresh session")
	}
}
