package domain

import (
	"errors"
	"time"
)

type CredentialType string

const (
	CredentialPublishable CredentialType = "publishable"
	CredentialSecret      CredentialType = "secret"
)

type CredentialEnvironment string

const (
	CredentialEnvironmentTest CredentialEnvironment = "test"
	CredentialEnvironmentLive CredentialEnvironment = "live"
)

type CredentialStatus string

const (
	CredentialStatusActive  CredentialStatus = "active"
	CredentialStatusRevoked CredentialStatus = "revoked"
)

// ApplicationCredential authenticates application-level requests. The full
// key is never stored: only the locator prefix and the SHA-256 digest of the
// random secret persist.
type ApplicationCredential struct {
	ID             string
	ApplicationID  string
	Environment    CredentialEnvironment
	CredentialType CredentialType
	Name           string
	KeyPrefix      string
	KeyHash        []byte
	Scopes         []string
	Status         CredentialStatus
	LastUsedAt     *time.Time
	ExpiresAt      *time.Time
	CreatedAt      time.Time
	RevokedAt      *time.Time
}

// NewApplicationCredential carries the persisted fields of a credential. The
// plaintext key is returned by the creator once and never written anywhere.
type NewApplicationCredential struct {
	ApplicationID  string
	Environment    CredentialEnvironment
	CredentialType CredentialType
	Name           string
	KeyPrefix      string
	KeyHash        []byte
	Scopes         []string
	ExpiresAt      *time.Time
}

var (
	ErrCredentialNotFound  = &NotFoundError{Entity: "credential"}
	ErrCredentialExists    = errors.New("a credential with this prefix already exists")
	ErrInvalidCredential   = errors.New("invalid application credential")
	ErrCredentialRevoked   = errors.New("application credential revoked")
	ErrCredentialExpired   = errors.New("application credential expired")
)
