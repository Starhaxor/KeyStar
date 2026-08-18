// Package credential implements application credential keys (ks_pk_/ks_sk_)
// per the KeyStar platform architecture. Keys are structured as
//
//	ks_<type>_<environment>_<locator>_<secret>
//
// where the locator is a Crockford base32 value used for DB lookup and the
// secret is a 256-bit random value verified by SHA-256 digest comparison.
package credential

import (
	"errors"
	"strings"
)

// Key prefixes per credential type and environment.
const (
	PrefixPublishableTest = "ks_pk_test_"
	PrefixPublishableLive = "ks_pk_live_"
	PrefixSecretTest      = "ks_sk_test_"
	PrefixSecretLive      = "ks_sk_live_"
)

const (
	locatorLength = 10
	secretLength  = 32 // 256-bit random secret
)

var ErrMalformedKey = errors.New("malformed application credential key")

// Prefix returns the static key prefix for a credential type and environment.
func Prefix(credentialType, environment string) (string, error) {
	switch credentialType + ":" + environment {
	case "publishable:test":
		return PrefixPublishableTest, nil
	case "publishable:live":
		return PrefixPublishableLive, nil
	case "secret:test":
		return PrefixSecretTest, nil
	case "secret:live":
		return PrefixSecretLive, nil
	default:
		return "", ErrMalformedKey
	}
}

// ParseKey splits a credential key into its locator prefix and secret. The
// returned prefix is the exact value stored in application_credentials.key_prefix.
// The separator position is derived from the fixed static prefix and locator
// lengths because the base64url secret itself may contain underscores.
func ParseKey(key string) (prefix, secret string, err error) {
	var static string
	switch {
	case strings.HasPrefix(key, PrefixPublishableTest):
		static = PrefixPublishableTest
	case strings.HasPrefix(key, PrefixPublishableLive):
		static = PrefixPublishableLive
	case strings.HasPrefix(key, PrefixSecretTest):
		static = PrefixSecretTest
	case strings.HasPrefix(key, PrefixSecretLive):
		static = PrefixSecretLive
	default:
		return "", "", ErrMalformedKey
	}
	separator := len(static) + locatorLength
	if len(key) < separator+1+secretLength || key[separator] != '_' || !isCrockfordLocator(key[len(static):separator]) {
		return "", "", ErrMalformedKey
	}
	prefix = key[:separator]
	secret = key[separator+1:]
	if !validSecret(secret) {
		return "", "", ErrMalformedKey
	}
	return prefix, secret, nil
}

func validSecret(secret string) bool {
	if len(secret) != 43 {
		return false
	}
	for _, r := range secret {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// crockfordAlphabet omits I, L, O and U for unambiguous display.
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func isCrockfordLocator(value string) bool {
	if len(value) != locatorLength {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune(crockfordAlphabet, r) {
			return false
		}
	}
	return true
}

func encodeCrockford(value []byte) string {
	var bits uint64
	var bitCount uint
	var encoded strings.Builder
	for _, b := range value {
		bits = bits<<8 | uint64(b)
		bitCount += 8
		for bitCount >= 5 {
			bitCount -= 5
			encoded.WriteByte(crockfordAlphabet[(bits>>bitCount)&0x1f])
		}
	}
	if bitCount > 0 {
		encoded.WriteByte(crockfordAlphabet[(bits<<(5-bitCount))&0x1f])
	}
	result := encoded.String()
	if len(result) < locatorLength {
		result = strings.Repeat("0", locatorLength-len(result)) + result
	}
	return result
}
