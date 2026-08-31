package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/starloader/backend/internal/domain"
)

const applicationKeyAlgorithm = "Ed25519"

var errInvalidApplicationSigningKey = errors.New("invalid application signing key")

type ApplicationKeyCipher struct {
	aeads         map[int]cipher.AEAD
	activeVersion int
	activeAEAD    cipher.AEAD
	random        io.Reader
}

func NewApplicationKeyCipher(keys map[int][]byte, activeVersion int, random io.Reader) (*ApplicationKeyCipher, error) {
	if len(keys) == 0 || random == nil {
		return nil, errors.New("application key cipher configuration is invalid")
	}

	aeads := make(map[int]cipher.AEAD, len(keys))
	for version, key := range keys {
		if version <= 0 || len(key) != 32 {
			return nil, errors.New("application key cipher configuration is invalid")
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, errors.New("application key cipher configuration is invalid")
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, errors.New("application key cipher configuration is invalid")
		}
		aeads[version] = aead
	}
	activeAEAD, ok := aeads[activeVersion]
	if !ok {
		return nil, errors.New("application key cipher configuration is invalid")
	}
	return &ApplicationKeyCipher{
		aeads: aeads, activeVersion: activeVersion, activeAEAD: activeAEAD, random: random,
	}, nil
}

func applicationKeyAAD(applicationID, kid, algorithm string, version int) []byte {
	return []byte(fmt.Sprintf("keystar:application-signing-key:v1\x00%s\x00%s\x00%s\x00%d", applicationID, kid, algorithm, version))
}

func (cipher *ApplicationKeyCipher) Generate(applicationID string) (domain.NewApplicationSigningKey, error) {
	if cipher == nil || cipher.activeAEAD == nil || cipher.random == nil {
		return domain.NewApplicationSigningKey{}, errors.New("application key cipher is not configured")
	}

	seed := make([]byte, ed25519.SeedSize)
	defer clear(seed)
	kidBytes := make([]byte, 16)
	defer clear(kidBytes)
	nonce := make([]byte, cipher.activeAEAD.NonceSize())
	if _, err := io.ReadFull(cipher.random, seed); err != nil {
		return domain.NewApplicationSigningKey{}, errors.New("generate application signing key")
	}
	if _, err := io.ReadFull(cipher.random, kidBytes); err != nil {
		return domain.NewApplicationSigningKey{}, errors.New("generate application signing key")
	}
	if _, err := io.ReadFull(cipher.random, nonce); err != nil {
		return domain.NewApplicationSigningKey{}, errors.New("generate application signing key")
	}

	kid := "ksk_" + base64.RawURLEncoding.EncodeToString(kidBytes)
	privateKey := ed25519.NewKeyFromSeed(seed)
	defer clear(privateKey)
	publicKey := append([]byte(nil), privateKey[ed25519.SeedSize:]...)
	sealed := cipher.activeAEAD.Seal(nil, nonce, seed, applicationKeyAAD(applicationID, kid, applicationKeyAlgorithm, cipher.activeVersion))
	return domain.NewApplicationSigningKey{
		KID: kid, ApplicationID: applicationID, Algorithm: applicationKeyAlgorithm,
		PublicKey: publicKey, EncryptedPrivateKey: sealed, EncryptionNonce: nonce,
		EncryptionKeyVersion: cipher.activeVersion, Status: domain.ApplicationSigningKeyPending,
	}, nil
}

func (cipher *ApplicationKeyCipher) Decrypt(record domain.ApplicationSigningKey) (ed25519.PrivateKey, error) {
	if cipher == nil || len(record.EncryptionNonce) == 0 || len(record.EncryptedPrivateKey) == 0 {
		return nil, errInvalidApplicationSigningKey
	}
	aead, ok := cipher.aeads[record.EncryptionKeyVersion]
	if !ok || len(record.EncryptionNonce) != aead.NonceSize() {
		return nil, errInvalidApplicationSigningKey
	}
	seed, err := aead.Open(nil, record.EncryptionNonce, record.EncryptedPrivateKey, applicationKeyAAD(
		record.ApplicationID,
		record.KID,
		record.Algorithm,
		record.EncryptionKeyVersion,
	))
	if err != nil || len(seed) != ed25519.SeedSize {
		clear(seed)
		return nil, errInvalidApplicationSigningKey
	}
	defer clear(seed)

	privateKey := ed25519.NewKeyFromSeed(seed)
	if len(record.PublicKey) != ed25519.PublicKeySize || subtle.ConstantTimeCompare(privateKey[ed25519.SeedSize:], record.PublicKey) != 1 {
		clear(privateKey)
		return nil, errInvalidApplicationSigningKey
	}
	return privateKey, nil
}
