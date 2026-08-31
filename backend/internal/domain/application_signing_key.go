package domain

import "time"

type ApplicationSigningKeyStatus string

const (
	ApplicationSigningKeyPending  ApplicationSigningKeyStatus = "pending"
	ApplicationSigningKeyActive   ApplicationSigningKeyStatus = "active"
	ApplicationSigningKeyRetiring ApplicationSigningKeyStatus = "retiring"
	ApplicationSigningKeyRevoked  ApplicationSigningKeyStatus = "revoked"
)

type ApplicationSigningKey struct {
	ID, KID, ApplicationID, Algorithm               string
	PublicKey, EncryptedPrivateKey, EncryptionNonce []byte
	EncryptionKeyVersion                            int
	Status                                          ApplicationSigningKeyStatus
	CreatedAt                                       time.Time
	ActivatedAt, RetireAt, RevokedAt                *time.Time
}

type NewApplicationSigningKey struct {
	KID, ApplicationID, Algorithm                   string
	PublicKey, EncryptedPrivateKey, EncryptionNonce []byte
	EncryptionKeyVersion                            int
	Status                                          ApplicationSigningKeyStatus
	ActivatedAt                                     *time.Time
}
