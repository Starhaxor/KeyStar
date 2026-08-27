package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const secretBoxPrefix = "enc:v1:"

// SecretBox provides versioned AES-256-GCM envelope encryption for small
// application secrets such as TOTP seeds.
type SecretBox struct{ aead cipher.AEAD }

func NewSecretBox(key []byte) (*SecretBox, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must be exactly 32 bytes")
	}
	block, err := aes.NewCipher(append([]byte(nil), key...))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

func (box *SecretBox) Encrypt(plaintext string) (string, error) {
	if box == nil || box.aead == nil || plaintext == "" {
		return "", errors.New("secret box is not configured")
	}
	nonce := make([]byte, box.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := box.aead.Seal(nonce, nonce, []byte(plaintext), []byte(secretBoxPrefix))
	return secretBoxPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (box *SecretBox) Decrypt(ciphertext string) (string, error) {
	if box == nil || box.aead == nil || !strings.HasPrefix(ciphertext, secretBoxPrefix) {
		return "", errors.New("invalid encrypted secret")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, secretBoxPrefix))
	if err != nil || len(payload) < box.aead.NonceSize()+box.aead.Overhead() {
		return "", errors.New("invalid encrypted secret")
	}
	nonce, sealed := payload[:box.aead.NonceSize()], payload[box.aead.NonceSize():]
	plain, err := box.aead.Open(nil, nonce, sealed, []byte(secretBoxPrefix))
	if err != nil {
		return "", errors.New("invalid encrypted secret")
	}
	return string(plain), nil
}

func IsEncryptedSecret(value string) bool { return strings.HasPrefix(value, secretBoxPrefix) }
