package domain

import "time"

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusBanned   UserStatus = "banned"
)

type User struct {
	ID            string
	ApplicationID string
	Email         string
	PasswordHash  string
	Status        UserStatus
	BanExpiresAt  *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type NewUser struct {
	ApplicationID string
	Email         string
	PasswordHash  string
}

// UserProfile contains the safe account and device information shown to a verified session.
type UserProfile struct {
	ApplicationID    string
	Email            string
	AccountStatus    UserStatus
	Product          string
	LicenseStatus    LicenseStatus
	LicenseExpiresAt time.Time
	MaxDevices       int
	DeviceID         string
	DeviceStatus     DeviceStatus
}

var ErrProfileNotFound = &NotFoundError{Entity: "profile"}

type LicenseStatus string

const (
	LicenseStatusActive  LicenseStatus = "active"
	LicenseStatusRevoked LicenseStatus = "revoked"
	LicenseStatusExpired LicenseStatus = "expired"
)

type License struct {
	ID            string
	ApplicationID string
	LicenseHMAC   string
	UserID        string
	ProductID     string
	PlanID        string
	Product       string // resolved product display name (joined from products)
	Status        LicenseStatus
	Level         int
	MaxDevices    int
	Notes         string
	ExpiresAt     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type NewLicense struct {
	ApplicationID string
	LicenseHMAC   string
	UserID        string
	ProductID     string
	PlanID        string
	MaxDevices    int
	ExpiresAt     time.Time
}

type DeviceStatus string

const (
	DeviceStatusActive  DeviceStatus = "active"
	DeviceStatusRevoked DeviceStatus = "revoked"
)

type Device struct {
	ID                    string
	ApplicationID         string
	UserID                string
	LicenseID             string
	TPMPublicKey          []byte
	TPMPublicKeySHA256    []byte
	SMBIOSUUIDHMAC        string
	MotherboardSerialHMAC string
	BIOSSerialHMAC        string
	SystemDiskSerialHMAC  string
	MachineGuidHMAC       string
	FingerprintHMAC       string
	Status                DeviceStatus
	CreatedAt             time.Time
	UpdatedAt             time.Time
	LastSeenAt            time.Time
}

type NewDevice struct {
	ApplicationID         string
	UserID                string
	LicenseID             string
	TPMPublicKey          []byte
	TPMPublicKeySHA256    []byte
	SMBIOSUUIDHMAC        string
	MotherboardSerialHMAC string
	BIOSSerialHMAC        string
	SystemDiskSerialHMAC  string
	MachineGuidHMAC       string
	FingerprintHMAC       string
	SeenAt                time.Time
}

type UpdateDevice struct {
	ID                    string
	ApplicationID         string
	SMBIOSUUIDHMAC        string
	MotherboardSerialHMAC string
	BIOSSerialHMAC        string
	SystemDiskSerialHMAC  string
	MachineGuidHMAC       string
	FingerprintHMAC       string
	SeenAt                time.Time
}

type SessionStatus string

const (
	SessionStatusPending  SessionStatus = "pending"
	SessionStatusVerified SessionStatus = "verified"
	SessionStatusExpired  SessionStatus = "expired"
)

type AuthSession struct {
	ID            string
	ApplicationID string
	UserID        string
	LicenseID     string
	Status        SessionStatus
	ExpiresAt     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type DeviceChallenge struct {
	ID              string
	SessionID       string
	ChallengeSHA256 []byte
	ExpiresAt       time.Time
	ConsumedAt      *time.Time
	CreatedAt       time.Time
}

// PendingSession is created atomically so a pending authentication session
// can never exist without its SHA-256 challenge record.
type PendingSession struct {
	Session   AuthSession
	Challenge DeviceChallenge
}

type NewPendingSession struct {
	ApplicationID   string
	UserID          string
	LicenseID       string
	ChallengeSHA256 []byte
	ExpiresAt       time.Time
}
