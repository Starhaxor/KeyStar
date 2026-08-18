package credential

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/starloader/backend/internal/domain"
)

// CredentialRepository is the persistence boundary the verifier needs.
type CredentialRepository interface {
	FindCredentialByPrefix(context.Context, string, string) (*domain.ApplicationCredential, error)
	TouchCredentialLastUsed(context.Context, string, string) error
}

// Verifier resolves a credential key to its application-scoped record and
// validates the secret digest in constant time.
type Verifier struct {
	repository CredentialRepository
	now        func() time.Time
}

func NewVerifier(repository CredentialRepository) *Verifier {
	return &Verifier{repository: repository, now: time.Now}
}

func (verifier *Verifier) WithClock(now func() time.Time) *Verifier {
	verifier.now = now
	return verifier
}

// Verify resolves and validates a credential key for the given application.
// A revoked or expired credential is rejected even when the digest matches.
func (verifier *Verifier) Verify(ctx context.Context, applicationID, key string) (*domain.ApplicationCredential, error) {
	if verifier == nil || verifier.repository == nil {
		return nil, errors.New("credential verifier is not configured")
	}
	prefix, secret, err := ParseKey(key)
	if err != nil {
		return nil, domain.ErrInvalidCredential
	}
	credential, err := verifier.repository.FindCredentialByPrefix(ctx, applicationID, prefix)
	if errors.Is(err, domain.ErrCredentialNotFound) {
		// Keep the timing close to the success path: perform the same digest
		// comparison against a fixed dummy so prefix probing cannot be timed.
		dummy := sha256.Sum256([]byte("keystar-unknown-credential"))
		_ = hmac.Equal(dummy[:], dummy[:])
		return nil, domain.ErrInvalidCredential
	}
	if err != nil {
		return nil, err
	}
	presented := sha256.Sum256([]byte(secret))
	if !hmac.Equal(presented[:], credential.KeyHash) {
		return nil, domain.ErrInvalidCredential
	}
	now := verifier.now().UTC()
	if credential.Status != domain.CredentialStatusActive {
		return nil, domain.ErrCredentialRevoked
	}
	if credential.ExpiresAt != nil && !credential.ExpiresAt.After(now) {
		return nil, domain.ErrCredentialExpired
	}
	_ = verifier.repository.TouchCredentialLastUsed(ctx, applicationID, credential.ID)
	return credential, nil
}
