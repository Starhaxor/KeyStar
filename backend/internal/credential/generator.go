package credential

import (
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// Generated holds one freshly created credential. The full key is shown to
// the operator exactly once; the store persists only Prefix and KeyHash.
type Generated struct {
	Key    string
	Prefix string
	Secret string
	Hash   []byte
}

// Generate creates a new credential key. The secret carries at least 256 bits
// of CSPRNG entropy; the locator is random so prefixes cannot be enumerated.
func Generate(credentialType, environment string, random io.Reader) (Generated, error) {
	if random == nil {
		random = cryptorand.Reader
	}
	prefixBase, err := Prefix(credentialType, environment)
	if err != nil {
		return Generated{}, err
	}
	locatorBytes := make([]byte, 6)
	if _, err := io.ReadFull(random, locatorBytes); err != nil {
		return Generated{}, fmt.Errorf("generate credential locator: %w", err)
	}
	secretBytes := make([]byte, secretLength)
	if _, err := io.ReadFull(random, secretBytes); err != nil {
		return Generated{}, fmt.Errorf("generate credential secret: %w", err)
	}
	locator := encodeCrockford(locatorBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	prefix := prefixBase + locator
	hash := sha256.Sum256([]byte(secret))
	return Generated{
		Key:    prefix + "_" + secret,
		Prefix: prefix,
		Secret: secret,
		Hash:   hash[:],
	}, nil
}
