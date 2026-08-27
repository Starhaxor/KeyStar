package security

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSecretBoxEncryptsAndAuthenticates(t *testing.T) {
	box, err := NewSecretBox([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Encrypt("totp-seed")
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "totp-seed" || !strings.HasPrefix(sealed, "enc:v1:") {
		t.Fatalf("sealed=%q", sealed)
	}
	plain, err := box.Decrypt(sealed)
	if err != nil || plain != "totp-seed" {
		t.Fatalf("Decrypt()=(%q,%v)", plain, err)
	}
	encoded := strings.TrimPrefix(sealed, secretBoxPrefix)
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)-1] ^= 0x01
	tampered := secretBoxPrefix + base64.RawURLEncoding.EncodeToString(payload)
	if _, err := box.Decrypt(tampered); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}

func TestSecretBoxRejectsInvalidKeyLengthAndPlaintext(t *testing.T) {
	if _, err := NewSecretBox([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
	box, _ := NewSecretBox([]byte("0123456789abcdef0123456789abcdef"))
	if _, err := box.Decrypt("plaintext"); err == nil {
		t.Fatal("plaintext accepted as ciphertext")
	}
}
