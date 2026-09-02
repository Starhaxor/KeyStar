package security

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
)

func TestApplicationSignerIssuesDeterministicProofBoundTokenWithActiveApplicationKey(t *testing.T) {
	cipher := newApplicationKeyCipherForTest(t)
	record := activeApplicationSigningKeyRecord(t, cipher, "app-a")
	signer := NewApplicationSigner(fixedApplicationSigningKeyRepository(&record), cipher)
	now := time.Unix(1_788_343_200, 0).UTC()
	signer.now = func() time.Time { return now }
	signer.random = bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})
	claims := requiredProofBoundClaims(now)
	claims.ApplicationID = "app-a"
	claims.IssuedAt = time.Time{}
	claims.ExpiresAt = time.Time{}
	claims.ProofBound.NotBefore = time.Time{}
	claims.ProofBound.TokenID = ""

	token, expiresAt, err := signer.IssueProofBound(context.Background(), "app-a", claims)
	if err != nil {
		t.Fatalf("IssueProofBound() error = %v", err)
	}
	if !expiresAt.Equal(now.Add(600 * time.Second)) {
		t.Fatalf("expiresAt = %v, want %v", expiresAt, now.Add(600*time.Second))
	}
	verifier := NewProofBoundTokenVerifier(fixedApplicationSigningKeyRepository(&record), "keystar", "starloader-client", "StarLoader")
	verifier.now = func() time.Time { return now }
	got, err := verifier.Verify(context.Background(), "app-a", token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got.ProofBound == nil || got.ProofBound.TokenID != "AAECAwQFBgcICQoLDA0ODw" {
		t.Fatalf("verified token ID = %#v", got.ProofBound)
	}
	if got.ApplicationID != "app-a" || got.ProofBound.SessionID != claims.ProofBound.SessionID || got.ProofBound.DeviceKeyThumbprint != claims.ProofBound.DeviceKeyThumbprint {
		t.Fatalf("verified bindings = %#v", got)
	}
}

func TestApplicationSignerProofBoundIssuanceSanitizesKeyAndRandomFailures(t *testing.T) {
	cipher := newApplicationKeyCipherForTest(t)
	now := time.Unix(1_788_343_200, 0).UTC()
	claims := requiredProofBoundClaims(now)
	claims.ApplicationID = "app-a"
	for _, test := range []struct {
		name       string
		repository ActiveApplicationKeyRepository
		random     io.Reader
	}{
		{name: "missing key", repository: fixedApplicationSigningKeyRepository(nil), random: bytes.NewReader(make([]byte, 16))},
		{name: "ambiguous key query", repository: applicationSignerRepositoryStub{find: func(context.Context, string) (*domain.ApplicationSigningKey, error) {
			return nil, errors.New("multiple active rows")
		}}, random: bytes.NewReader(make([]byte, 16))},
		{name: "random failure", repository: fixedApplicationSigningKeyRepository(pointerToActiveApplicationSigningKey(t, cipher, "app-a")), random: failingReader{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			signer := NewApplicationSigner(test.repository, cipher)
			signer.now = func() time.Time { return now }
			signer.random = test.random
			token, expiresAt, err := signer.IssueProofBound(context.Background(), "app-a", claims)
			if err != ErrApplicationSigningUnavailable {
				t.Fatalf("IssueProofBound() error = %v, want only %v", err, ErrApplicationSigningUnavailable)
			}
			if token != "" || !expiresAt.IsZero() {
				t.Fatalf("IssueProofBound() = %q, %v; want zero values", token, expiresAt)
			}
		})
	}
}

func TestProofBoundTokenVerifierRejectsUnknownOrRevokedActiveKey(t *testing.T) {
	cipher := newApplicationKeyCipherForTest(t)
	record := activeApplicationSigningKeyRecord(t, cipher, "app-a")
	now := time.Unix(1_788_343_200, 0).UTC()
	signer := NewApplicationSigner(fixedApplicationSigningKeyRepository(&record), cipher)
	signer.now = func() time.Time { return now }
	signer.random = bytes.NewReader(make([]byte, 16))
	claims := requiredProofBoundClaims(now)
	claims.ApplicationID = "app-a"
	token, _, err := signer.IssueProofBound(context.Background(), "app-a", claims)
	if err != nil {
		t.Fatal(err)
	}

	revoked := record
	revoked.Status = domain.ApplicationSigningKeyRevoked
	unknown := activeApplicationSigningKeyRecord(t, cipher, "app-a")
	for _, test := range []struct {
		name   string
		record *domain.ApplicationSigningKey
	}{
		{name: "revoked kid", record: &revoked},
		{name: "unknown kid", record: &unknown},
		{name: "missing kid", record: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			verifier := NewProofBoundTokenVerifier(fixedApplicationSigningKeyRepository(test.record), "keystar", "starloader-client", "StarLoader")
			verifier.now = func() time.Time { return now }
			if _, err := verifier.Verify(context.Background(), "app-a", token); err != ErrInvalidSessionToken {
				t.Fatalf("Verify() error = %v, want only %v", err, ErrInvalidSessionToken)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropy source details") }

func pointerToActiveApplicationSigningKey(t *testing.T, cipher *ApplicationKeyCipher, applicationID string) *domain.ApplicationSigningKey {
	t.Helper()
	record := activeApplicationSigningKeyRecord(t, cipher, applicationID)
	return &record
}

func TestApplicationSignerSignsWithRequestedApplicationsActiveKey(t *testing.T) {
	cipher := newApplicationKeyCipherForTest(t)
	record := activeApplicationSigningKeyRecord(t, cipher, "app-a")
	signer := NewApplicationSigner(applicationSignerRepositoryStub{
		find: func(_ context.Context, applicationID string) (*domain.ApplicationSigningKey, error) {
			if applicationID != "app-a" {
				t.Fatalf("FindActiveApplicationSigningKey() application ID = %q, want app-a", applicationID)
			}
			return &record, nil
		},
	}, cipher)
	message := []byte("application-scoped payload")

	signed, err := signer.Sign(context.Background(), "app-a", message)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if signed.KID != record.KID {
		t.Fatalf("Sign() KID = %q, want %q", signed.KID, record.KID)
	}
	if !ed25519.Verify(ed25519.PublicKey(signed.PublicKey), message, signed.Signature) {
		t.Fatal("Sign() signature does not verify with returned public key")
	}
	if !ed25519.Verify(ed25519.PublicKey(record.PublicKey), message, signed.Signature) {
		t.Fatal("Sign() signature does not verify with persisted public key")
	}
}

func TestApplicationSignerRejectsKeyForAnotherApplication(t *testing.T) {
	cipher := newApplicationKeyCipherForTest(t)
	otherApplicationsRecord := activeApplicationSigningKeyRecord(t, cipher, "app-b")
	signer := NewApplicationSigner(applicationSignerRepositoryStub{
		find: func(context.Context, string) (*domain.ApplicationSigningKey, error) {
			return &otherApplicationsRecord, nil
		},
	}, cipher)

	assertApplicationSigningUnavailable(t, signer, "app-a")
}

func TestApplicationSignerSanitizesUnavailableKeyFailures(t *testing.T) {
	tests := []struct {
		name       string
		repository func(*testing.T, *ApplicationKeyCipher) ActiveApplicationKeyRepository
	}{
		{
			name: "missing record",
			repository: func(*testing.T, *ApplicationKeyCipher) ActiveApplicationKeyRepository {
				return applicationSignerRepositoryStub{find: func(context.Context, string) (*domain.ApplicationSigningKey, error) {
					return nil, nil
				}}
			},
		},
		{
			name: "repository failure",
			repository: func(*testing.T, *ApplicationKeyCipher) ActiveApplicationKeyRepository {
				return applicationSignerRepositoryStub{find: func(context.Context, string) (*domain.ApplicationSigningKey, error) {
					return nil, errors.New("database connection included sensitive details")
				}}
			},
		},
		{
			name: "revoked record",
			repository: func(t *testing.T, cipher *ApplicationKeyCipher) ActiveApplicationKeyRepository {
				record := activeApplicationSigningKeyRecord(t, cipher, "app-a")
				record.Status = domain.ApplicationSigningKeyRevoked
				return fixedApplicationSigningKeyRepository(&record)
			},
		},
		{
			name: "active record without activation time",
			repository: func(t *testing.T, cipher *ApplicationKeyCipher) ActiveApplicationKeyRepository {
				record := activeApplicationSigningKeyRecord(t, cipher, "app-a")
				record.ActivatedAt = nil
				return fixedApplicationSigningKeyRepository(&record)
			},
		},
		{
			name: "active record with retirement time",
			repository: func(t *testing.T, cipher *ApplicationKeyCipher) ActiveApplicationKeyRepository {
				record := activeApplicationSigningKeyRecord(t, cipher, "app-a")
				retireAt := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
				record.RetireAt = &retireAt
				return fixedApplicationSigningKeyRepository(&record)
			},
		},
		{
			name: "active record with revocation time",
			repository: func(t *testing.T, cipher *ApplicationKeyCipher) ActiveApplicationKeyRepository {
				record := activeApplicationSigningKeyRecord(t, cipher, "app-a")
				revokedAt := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
				record.RevokedAt = &revokedAt
				return fixedApplicationSigningKeyRepository(&record)
			},
		},
		{
			name: "malformed record",
			repository: func(t *testing.T, cipher *ApplicationKeyCipher) ActiveApplicationKeyRepository {
				record := activeApplicationSigningKeyRecord(t, cipher, "app-a")
				record.EncryptionNonce = nil
				return fixedApplicationSigningKeyRepository(&record)
			},
		},
		{
			name: "public private key mismatch",
			repository: func(t *testing.T, cipher *ApplicationKeyCipher) ActiveApplicationKeyRepository {
				record := activeApplicationSigningKeyRecord(t, cipher, "app-a")
				record.PublicKey[0] ^= 0x80
				return fixedApplicationSigningKeyRepository(&record)
			},
		},
		{
			name: "authentication tag failure",
			repository: func(t *testing.T, cipher *ApplicationKeyCipher) ActiveApplicationKeyRepository {
				record := activeApplicationSigningKeyRecord(t, cipher, "app-a")
				record.EncryptedPrivateKey[len(record.EncryptedPrivateKey)-1] ^= 0x80
				return fixedApplicationSigningKeyRepository(&record)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cipher := newApplicationKeyCipherForTest(t)
			signer := NewApplicationSigner(test.repository(t, cipher), cipher)
			assertApplicationSigningUnavailable(t, signer, "app-a")
		})
	}
}

type applicationSignerRepositoryStub struct {
	find func(context.Context, string) (*domain.ApplicationSigningKey, error)
}

func (stub applicationSignerRepositoryStub) FindActiveApplicationSigningKey(
	ctx context.Context,
	applicationID string,
) (*domain.ApplicationSigningKey, error) {
	return stub.find(ctx, applicationID)
}

func fixedApplicationSigningKeyRepository(record *domain.ApplicationSigningKey) ActiveApplicationKeyRepository {
	return applicationSignerRepositoryStub{find: func(context.Context, string) (*domain.ApplicationSigningKey, error) {
		return record, nil
	}}
}

func activeApplicationSigningKeyRecord(
	t *testing.T,
	cipher *ApplicationKeyCipher,
	applicationID string,
) domain.ApplicationSigningKey {
	t.Helper()
	generated, err := cipher.Generate(applicationID)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	record := applicationSigningKeyRecord(generated)
	record.Status = domain.ApplicationSigningKeyActive
	activatedAt := time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
	record.ActivatedAt = &activatedAt
	return record
}

func assertApplicationSigningUnavailable(t *testing.T, signer *ApplicationSigner, applicationID string) {
	t.Helper()
	signed, err := signer.Sign(context.Background(), applicationID, []byte("must not sign"))
	if err != ErrApplicationSigningUnavailable {
		t.Fatalf("Sign() error = %v, want only sanitized %v", err, ErrApplicationSigningUnavailable)
	}
	if signed.KID != "" || len(signed.PublicKey) != 0 || len(signed.Signature) != 0 {
		t.Fatalf("Sign() result = %+v, want zero value", signed)
	}
}
