package security

import (
	"bytes"
	"crypto/ed25519"
	"io"
	"testing"

	"github.com/starloader/backend/internal/domain"
)

func TestApplicationKeyCipherGeneratesIndependentPendingKeys(t *testing.T) {
	cipher := newApplicationKeyCipherForTest(t)

	first, err := cipher.Generate("app-1")
	if err != nil {
		t.Fatalf("Generate() first key error = %v", err)
	}
	second, err := cipher.Generate("app-1")
	if err != nil {
		t.Fatalf("Generate() second key error = %v", err)
	}

	if first.KID == second.KID {
		t.Fatal("two generated keys have the same kid")
	}
	if bytes.Equal(first.PublicKey, second.PublicKey) {
		t.Fatal("two generated keys have the same public key")
	}
	if bytes.Equal(first.EncryptionNonce, second.EncryptionNonce) {
		t.Fatal("two generated keys have the same nonce")
	}
	if bytes.Equal(first.EncryptedPrivateKey, second.EncryptedPrivateKey) {
		t.Fatal("two generated keys have the same encrypted seed")
	}
	for _, generated := range []domain.NewApplicationSigningKey{first, second} {
		if len(generated.KID) < len("ksk_") || generated.KID[:len("ksk_")] != "ksk_" {
			t.Fatalf("KID = %q, want ksk_ prefix", generated.KID)
		}
		if generated.ApplicationID != "app-1" || generated.Algorithm != "Ed25519" {
			t.Fatalf("generated key identity = (%q, %q), want (app-1, Ed25519)", generated.ApplicationID, generated.Algorithm)
		}
		if len(generated.PublicKey) != ed25519.PublicKeySize {
			t.Fatalf("public key length = %d, want %d", len(generated.PublicKey), ed25519.PublicKeySize)
		}
		if len(generated.EncryptionNonce) != 12 {
			t.Fatalf("nonce length = %d, want 12", len(generated.EncryptionNonce))
		}
		if len(generated.EncryptedPrivateKey) != ed25519.SeedSize+16 {
			t.Fatalf("ciphertext length = %d, want encrypted 32-byte seed plus 16-byte tag", len(generated.EncryptedPrivateKey))
		}
		if generated.EncryptionKeyVersion != 7 {
			t.Fatalf("encryption key version = %d, want 7", generated.EncryptionKeyVersion)
		}
		if generated.Status != domain.ApplicationSigningKeyPending || generated.ActivatedAt != nil {
			t.Fatalf("generated lifecycle = (%q, %v), want pending and not activated", generated.Status, generated.ActivatedAt)
		}
	}
}

func TestApplicationKeyCipherDecryptsAUsableEd25519Key(t *testing.T) {
	cipher := newApplicationKeyCipherForTest(t)
	generated, err := cipher.Generate("app-1")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	record := applicationSigningKeyRecord(generated)

	privateKey, err := cipher.Decrypt(record)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	message := []byte("application-scoped payload")
	signature := ed25519.Sign(privateKey, message)
	if !ed25519.Verify(ed25519.PublicKey(record.PublicKey), message, signature) {
		t.Fatal("decrypted private key did not sign for the persisted public key")
	}
}

func TestApplicationKeyCipherRejectsTamperedRecord(t *testing.T) {
	cipher := newApplicationKeyCipherForTest(t)
	generated, err := cipher.Generate("app-1")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	original := applicationSigningKeyRecord(generated)

	tests := []struct {
		name   string
		tamper func(*domain.ApplicationSigningKey)
	}{
		{name: "application ID", tamper: func(record *domain.ApplicationSigningKey) { record.ApplicationID = "app-2" }},
		{name: "kid", tamper: func(record *domain.ApplicationSigningKey) { record.KID += "x" }},
		{name: "algorithm", tamper: func(record *domain.ApplicationSigningKey) { record.Algorithm = "Ed448" }},
		{name: "encryption key version", tamper: func(record *domain.ApplicationSigningKey) { record.EncryptionKeyVersion = 8 }},
		{name: "nonce", tamper: func(record *domain.ApplicationSigningKey) { record.EncryptionNonce[0] ^= 0x80 }},
		{name: "ciphertext", tamper: func(record *domain.ApplicationSigningKey) { record.EncryptedPrivateKey[0] ^= 0x80 }},
		{name: "public key", tamper: func(record *domain.ApplicationSigningKey) { record.PublicKey[0] ^= 0x80 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := cloneApplicationSigningKeyRecord(original)
			test.tamper(&record)
			if privateKey, err := cipher.Decrypt(record); err == nil || privateKey != nil {
				t.Fatalf("Decrypt() = (%d-byte key, %v), want failure", len(privateKey), err)
			}
		})
	}
}

func TestNewApplicationKeyCipherRejectsInvalidConfiguration(t *testing.T) {
	validKey := bytes.Repeat([]byte{0x42}, 32)
	tests := []struct {
		name          string
		keys          map[int][]byte
		activeVersion int
		random        io.Reader
	}{
		{name: "empty key ring", keys: map[int][]byte{}, activeVersion: 7, random: bytes.NewReader(nil)},
		{name: "invalid key length", keys: map[int][]byte{7: []byte("short")}, activeVersion: 7, random: bytes.NewReader(nil)},
		{name: "missing active version", keys: map[int][]byte{7: validKey}, activeVersion: 8, random: bytes.NewReader(nil)},
		{name: "nil random reader", keys: map[int][]byte{7: validKey}, activeVersion: 7},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if cipher, err := NewApplicationKeyCipher(test.keys, test.activeVersion, test.random); err == nil || cipher != nil {
				t.Fatalf("NewApplicationKeyCipher() = (%v, %v), want failure", cipher, err)
			}
		})
	}
}

func newApplicationKeyCipherForTest(t *testing.T) *ApplicationKeyCipher {
	t.Helper()
	key := bytes.Repeat([]byte{0x42}, 32)
	randomness := make([]byte, 2*(ed25519.SeedSize+16+12))
	for i := range randomness {
		randomness[i] = byte(i)
	}
	cipher, err := NewApplicationKeyCipher(map[int][]byte{7: key, 8: key}, 7, bytes.NewReader(randomness))
	if err != nil {
		t.Fatalf("NewApplicationKeyCipher() error = %v", err)
	}
	return cipher
}

func applicationSigningKeyRecord(generated domain.NewApplicationSigningKey) domain.ApplicationSigningKey {
	return domain.ApplicationSigningKey{
		KID: generated.KID, ApplicationID: generated.ApplicationID, Algorithm: generated.Algorithm,
		PublicKey:            append([]byte(nil), generated.PublicKey...),
		EncryptedPrivateKey:  append([]byte(nil), generated.EncryptedPrivateKey...),
		EncryptionNonce:      append([]byte(nil), generated.EncryptionNonce...),
		EncryptionKeyVersion: generated.EncryptionKeyVersion,
		Status:               generated.Status, ActivatedAt: generated.ActivatedAt,
	}
}

func cloneApplicationSigningKeyRecord(record domain.ApplicationSigningKey) domain.ApplicationSigningKey {
	record.PublicKey = append([]byte(nil), record.PublicKey...)
	record.EncryptedPrivateKey = append([]byte(nil), record.EncryptedPrivateKey...)
	record.EncryptionNonce = append([]byte(nil), record.EncryptionNonce...)
	return record
}
