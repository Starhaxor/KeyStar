package security

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"time"

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
	now        func() time.Time
	random     io.Reader
}

func NewApplicationSigner(repository ActiveApplicationKeyRepository, cipher *ApplicationKeyCipher) *ApplicationSigner {
	return &ApplicationSigner{repository: repository, cipher: cipher, now: time.Now, random: rand.Reader}
}

type ProofBoundTokenVerifier struct {
	repository ActiveApplicationKeyRepository
	issuer     string
	audience   string
	product    string
	now        func() time.Time
}

func NewProofBoundTokenVerifier(
	repository ActiveApplicationKeyRepository,
	issuer, audience, product string,
) *ProofBoundTokenVerifier {
	return &ProofBoundTokenVerifier{
		repository: repository, issuer: issuer, audience: audience, product: product, now: time.Now,
	}
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

func (signer *ApplicationSigner) IssueProofBound(
	ctx context.Context,
	applicationID string,
	claims SessionClaims,
) (string, time.Time, error) {
	if signer == nil || signer.repository == nil || signer.cipher == nil || signer.now == nil || signer.random == nil ||
		applicationID == "" || claims.ApplicationID != applicationID || claims.ProofBound == nil {
		return "", time.Time{}, ErrApplicationSigningUnavailable
	}
	record, err := signer.repository.FindActiveApplicationSigningKey(ctx, applicationID)
	if err != nil || !validActiveApplicationSigningKey(record, applicationID) {
		return "", time.Time{}, ErrApplicationSigningUnavailable
	}
	privateKey, err := signer.cipher.Decrypt(*record)
	if err != nil {
		return "", time.Time{}, ErrApplicationSigningUnavailable
	}
	defer clear(privateKey)

	tokenID := make([]byte, 16)
	defer clear(tokenID)
	if _, err := io.ReadFull(signer.random, tokenID); err != nil {
		return "", time.Time{}, ErrApplicationSigningUnavailable
	}
	now := signer.now().UTC().Truncate(time.Second)
	expiresAt := now.Add(proofBoundTokenLifetime)
	claims.IssuedAt = now
	claims.ExpiresAt = expiresAt
	proof := *claims.ProofBound
	proof.TokenID = base64.RawURLEncoding.EncodeToString(tokenID)
	proof.NotBefore = now
	claims.ProofBound = &proof
	token, err := issueProofBoundToken(privateKey, record.KID, claims)
	if err != nil {
		return "", time.Time{}, ErrApplicationSigningUnavailable
	}
	return token, expiresAt, nil
}

func (verifier *ProofBoundTokenVerifier) Verify(
	ctx context.Context,
	applicationID, token string,
) (SessionClaims, error) {
	if verifier == nil || verifier.repository == nil || verifier.now == nil || applicationID == "" {
		return SessionClaims{}, ErrInvalidSessionToken
	}
	record, err := verifier.repository.FindActiveApplicationSigningKey(ctx, applicationID)
	if err != nil || !validActiveApplicationSigningKey(record, applicationID) {
		return SessionClaims{}, ErrInvalidSessionToken
	}
	return verifyProofBoundToken(
		token,
		ed25519.PublicKey(record.PublicKey),
		record.KID,
		applicationID,
		verifier.issuer,
		verifier.audience,
		verifier.product,
		verifier.now().UTC(),
	)
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
