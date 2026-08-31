package security

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
)

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
