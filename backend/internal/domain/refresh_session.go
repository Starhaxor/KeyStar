package domain

import "time"

// RefreshSessionStatus tracks the lifecycle of a refresh token.
type RefreshSessionStatus string

const (
	RefreshSessionStatusActive  RefreshSessionStatus = "active"
	RefreshSessionStatusRotated RefreshSessionStatus = "rotated"
	RefreshSessionStatusRevoked RefreshSessionStatus = "revoked"
)

// RefreshSession is an opaque refresh token bound to a specific device and
// user. The plaintext token is never stored; only its SHA-256 hash is
// persisted. Refresh tokens rotate on every use: the old token is marked
// "rotated" and a new one is issued.
type RefreshSession struct {
	ID            string
	ApplicationID string
	UserID        string
	LicenseID     string
	DeviceID      string
	TokenHash     []byte
	Status        RefreshSessionStatus
	ExpiresAt     time.Time
	LastUsedAt    *time.Time
	CreatedAt     time.Time
	RevokedAt     *time.Time
}

// NewRefreshSession is the input for creating a refresh token.
type NewRefreshSession struct {
	ApplicationID string
	UserID        string
	LicenseID     string
	DeviceID      string
	TokenHash     []byte
	ExpiresAt     time.Time
}
