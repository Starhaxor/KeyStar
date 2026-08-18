package credential

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/starloader/backend/internal/domain"
)

func TestGenerateProducesWellFormedKeys(t *testing.T) {
	for _, test := range []struct {
		credentialType string
		environment    string
		prefix         string
	}{
		{credentialType: "publishable", environment: "test", prefix: PrefixPublishableTest},
		{credentialType: "publishable", environment: "live", prefix: PrefixPublishableLive},
		{credentialType: "secret", environment: "test", prefix: PrefixSecretTest},
		{credentialType: "secret", environment: "live", prefix: PrefixSecretLive},
	} {
		t.Run(test.prefix, func(t *testing.T) {
			generated, err := Generate(test.credentialType, test.environment, bytes.NewReader(bytes.Repeat([]byte{0x42}, 64)))
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			if !strings.HasPrefix(generated.Key, test.prefix) {
				t.Fatalf("key %q does not start with %q", generated.Key, test.prefix)
			}
			if generated.Prefix != test.prefix+generated.Key[len(test.prefix):len(test.prefix)+locatorLength] {
				t.Fatalf("prefix %q does not match key %q", generated.Prefix, generated.Key)
			}
			if !strings.HasPrefix(generated.Key, generated.Prefix+"_") {
				t.Fatalf("key %q does not contain prefix %q", generated.Key, generated.Prefix)
			}
			if len(generated.Secret) != 43 {
				t.Fatalf("secret length = %d, want 43", len(generated.Secret))
			}
			if len(generated.Hash) != 32 {
				t.Fatalf("hash length = %d, want 32", len(generated.Hash))
			}
			if !strings.HasSuffix(generated.Key, "_"+generated.Secret) {
				t.Fatalf("key %q does not end with its secret", generated.Key)
			}
		})
	}
}

func TestParseKeyRoundTripAndMalformedRejection(t *testing.T) {
	generated, err := Generate("secret", "live", nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix, secret, err := ParseKey(generated.Key)
	if err != nil {
		t.Fatalf("ParseKey() error = %v", err)
	}
	if prefix != generated.Prefix || secret != generated.Secret {
		t.Fatalf("ParseKey() = (%q, %q), want (%q, %q)", prefix, secret, generated.Prefix, generated.Secret)
	}

	const validSecret = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // 43 chars
	for _, key := range []string{
		"",
		"ks_pk_live_",
		"ks_pk_live_LLLLLLLLLL_" + validSecret,
		"ks_pk_live_0123456789_",
		"ks_sk_test_0123456789_short",
		"ks_pk_live_0123456789_" + strings.Repeat("A", 42), // secret too short
		"plain-random-string",
		"ks_pk_0123456789_" + validSecret,
	} {
		if _, _, err := ParseKey(key); !errors.Is(err, ErrMalformedKey) {
			t.Fatalf("ParseKey(%q) error = %v, want malformed", key, err)
		}
	}
}

func TestPrefixRejectsUnknownCombinations(t *testing.T) {
	for _, test := range []struct{ credentialType, environment string }{
		{"publishable", "prod"},
		{"admin", "live"},
		{"", ""},
	} {
		if _, err := Prefix(test.credentialType, test.environment); !errors.Is(err, ErrMalformedKey) {
			t.Fatalf("Prefix(%q, %q) error = %v, want malformed", test.credentialType, test.environment, err)
		}
	}
}

const testValidSecret = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // 43 chars

func TestVerifierAcceptsValidKeyAndTouchesUsage(t *testing.T) {
	now := time.Now().UTC()
	repository := &fakeCredentialRepository{
		credential: &domain.ApplicationCredential{
			ID: "cred-1", ApplicationID: "app-1", Status: domain.CredentialStatusActive,
			KeyHash: sha256Of([]byte(testValidSecret)), ExpiresAt: nil,
		},
	}
	verifier := NewVerifier(repository).WithClock(func() time.Time { return now })

	key := "ks_pk_live_0123456789_" + testValidSecret
	credential, err := verifier.Verify(context.Background(), "app-1", key)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if credential.ID != "cred-1" {
		t.Fatalf("Verify() credential = %#v", credential)
	}
	if repository.prefix != "ks_pk_live_0123456789" || repository.applicationID != "app-1" {
		t.Fatalf("lookup args = (%q, %q)", repository.applicationID, repository.prefix)
	}
	if repository.touched != 1 {
		t.Fatalf("TouchCredentialLastUsed() calls = %d, want 1", repository.touched)
	}
}

func TestVerifierRejectsWrongSecretUnknownPrefixRevokedAndExpired(t *testing.T) {
	now := time.Now().UTC()
	valid := &domain.ApplicationCredential{
		ID: "cred-1", ApplicationID: "app-1", Status: domain.CredentialStatusActive,
		KeyHash: sha256Of([]byte(testValidSecret)),
	}
	expiredAt := now.Add(-time.Minute)
	key := "ks_pk_live_0123456789_" + testValidSecret
	tests := []struct {
		name       string
		key        string
		credential *domain.ApplicationCredential
		want       error
	}{
		{name: "wrong secret", key: "ks_pk_live_0123456789_" + strings.Repeat("B", 43), credential: valid, want: domain.ErrInvalidCredential},
		{name: "unknown prefix", key: "ks_pk_live_AAAAAAAAAA_" + testValidSecret, credential: nil, want: domain.ErrInvalidCredential},
		{name: "revoked", key: key, credential: func() *domain.ApplicationCredential {
			copy := *valid
			copy.Status = domain.CredentialStatusRevoked
			return &copy
		}(), want: domain.ErrCredentialRevoked},
		{name: "expired", key: key, credential: func() *domain.ApplicationCredential {
			copy := *valid
			copy.ExpiresAt = &expiredAt
			return &copy
		}(), want: domain.ErrCredentialExpired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeCredentialRepository{credential: test.credential}
			verifier := NewVerifier(repository).WithClock(func() time.Time { return now })
			_, err := verifier.Verify(context.Background(), "app-1", test.key)
			if !errors.Is(err, test.want) {
				t.Fatalf("Verify() error = %v, want %v", err, test.want)
			}
			if repository.touched != 0 {
				t.Fatalf("rejected verification touched usage: calls = %d", repository.touched)
			}
		})
	}
}

func TestScopesEnforceTypeBoundaries(t *testing.T) {
	if !ValidScopes("publishable", []string{"auth.login", "device.verify"}) {
		t.Fatal("publishable auth.login/device.verify scopes rejected")
	}
	if ValidScopes("publishable", []string{"users.write"}) {
		t.Fatal("publishable key granted a server scope")
	}
	if !ValidScopes("secret", []string{"users.read", "licenses.write"}) {
		t.Fatal("secret server scopes rejected")
	}
	if ValidScopes("secret", []string{"auth.login"}) {
		t.Fatal("secret key granted a client-only scope")
	}
	if ValidScopes("publishable", []string{"unknown.scope"}) {
		t.Fatal("unknown scope accepted")
	}
}

type fakeCredentialRepository struct {
	credential    *domain.ApplicationCredential
	applicationID string
	prefix        string
	touched       int
}

func (repository *fakeCredentialRepository) FindCredentialByPrefix(_ context.Context, applicationID, prefix string) (*domain.ApplicationCredential, error) {
	repository.applicationID = applicationID
	repository.prefix = prefix
	if repository.credential == nil {
		return nil, domain.ErrCredentialNotFound
	}
	return repository.credential, nil
}

func (repository *fakeCredentialRepository) TouchCredentialLastUsed(context.Context, string, string) error {
	repository.touched++
	return nil
}

func sha256Of(value []byte) []byte {
	sum := sha256.Sum256(value)
	return sum[:]
}
