package domain

import "time"

// ServerUser is the server-to-server API view of an end user. Password hashes
// and license HMACs are never exposed through this view.
type ServerUser struct {
	ID           string
	Email        string
	Status       UserStatus
	Notes        string
	BanReason    string
	BannedAt     *time.Time
	BanExpiresAt *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ServerLicense is the server-to-server API view of a license. LicenseHMAC is
// deliberately omitted: the plaintext key is shown once at creation and the
// digest alone is never returned.
type ServerLicense struct {
	ID         string
	UserID     string
	UserEmail  string
	ProductID  string
	PlanID     string
	Product    string // resolved product display name (joined from products)
	Status     LicenseStatus
	Level      int
	MaxDevices int
	Notes      string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
