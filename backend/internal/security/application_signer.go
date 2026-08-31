package security

import (
	"context"
	"crypto/ed25519"
	"errors"

	"github.com/starloader/backend/internal/domain"
)

var ErrApplicationSigningUnavailable = errors.New("application signing unavailable")

type ActiveApplicationKeyRepository interface {
	FindActiveApplicationSigningKey(context.Context, string) (*domain.ApplicationSigningKey, error)
}

type SignedMessage struct {
	KID       string
	PublicKey []byte
	Signature []byte
}

type ApplicationSigner struct {
	repository ActiveApplicationKeyRepository
	cipher     *ApplicationKeyCipher
}

func NewApplicationSigner(repository ActiveApplicationKeyRepository, cipher *ApplicationKeyCipher) *ApplicationSigner {
	return &ApplicationSigner{repository: repository, cipher: cipher}
}

func (signer *ApplicationSigner) Sign(
	ctx context.Context,
	applicationID string,
	message []byte,
) (SignedMessage, error) {
	if signer == nil || signer.repository == nil || signer.cipher == nil || applicationID == "" {
		return SignedMessage{}, ErrApplicationSigningUnavailable
	}

	record, err := signer.repository.FindActiveApplicationSigningKey(ctx, applicationID)
	if err != nil || !validActiveApplicationSigningKey(record, applicationID) {
		return SignedMessage{}, ErrApplicationSigningUnavailable
	}

	privateKey, err := signer.cipher.Decrypt(*record)
	if err != nil {
		return SignedMessage{}, ErrApplicationSigningUnavailable
	}
	defer clear(privateKey)

	return SignedMessage{
		KID:       record.KID,
		PublicKey: append([]byte(nil), record.PublicKey...),
		Signature: ed25519.Sign(privateKey, message),
	}, nil
}

func validActiveApplicationSigningKey(record *domain.ApplicationSigningKey, applicationID string) bool {
	return record != nil &&
		record.ApplicationID == applicationID &&
		record.KID != "" &&
		record.Algorithm == applicationKeyAlgorithm &&
		len(record.PublicKey) == ed25519.PublicKeySize &&
		len(record.EncryptedPrivateKey) != 0 &&
		len(record.EncryptionNonce) != 0 &&
		record.EncryptionKeyVersion > 0 &&
		record.Status == domain.ApplicationSigningKeyActive &&
		record.ActivatedAt != nil &&
		record.RetireAt == nil &&
		record.RevokedAt == nil
}
