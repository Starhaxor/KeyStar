package domain

import "time"

// TPMPolicy defines the TPM enforcement mode for an application's device policy.
type TPMPolicy string

const (
	// TPMRequired means the device must present a valid TPM public key to
	// pass verification. Without TPM the device is rejected.
	TPMRequired TPMPolicy = "required"
	// TPMPreferred means the device gets a higher match score when TPM is
	// present. Missing TPM does not block verification but reduces the
	// effective score.
	TPMPreferred TPMPolicy = "preferred"
	// TPMOptional means TPM is not considered in scoring at all.
	TPMOptional TPMPolicy = "optional"
)

// ValidTPMPolicies enumerates the accepted TPM policy values.
var ValidTPMPolicies = map[TPMPolicy]bool{
	TPMRequired:  true,
	TPMPreferred: true,
	TPMOptional:  true,
}

// DevicePolicy is the per-application configuration that controls how device
// verification behaves: TPM enforcement, match-score thresholds, rebind
// rules and rate limits.
type DevicePolicy struct {
	ID                     string
	ApplicationID          string
	TPMPolicy              TPMPolicy
	MinMatchScore          int
	StepUpScore            int
	AllowAutoRebind        bool
	RebindCooldownSeconds  int64
	MaxDeviceChangesPer30d int
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// DefaultDevicePolicy returns the sensible defaults that match the current
// hard-coded behaviour before this table was introduced.
func DefaultDevicePolicy(applicationID string) *DevicePolicy {
	return &DevicePolicy{
		ApplicationID:          applicationID,
		TPMPolicy:              TPMOptional,
		MinMatchScore:          70,
		StepUpScore:            40,
		AllowAutoRebind:        true,
		RebindCooldownSeconds:  86400, // 24 hours
		MaxDeviceChangesPer30d: 5,
	}
}

// NewDevicePolicy is the input for creating or updating a device policy.
type NewDevicePolicy struct {
	TPMPolicy              TPMPolicy
	MinMatchScore          int
	StepUpScore            int
	AllowAutoRebind        bool
	RebindCooldownSeconds  int64
	MaxDeviceChangesPer30d int
}

// Validate ensures the policy fields are within acceptable ranges.
func (p *NewDevicePolicy) Validate() error {
	if !ValidTPMPolicies[p.TPMPolicy] {
		return ErrDevicePolicyInvalidTPMPolicy
	}
	if p.MinMatchScore < 0 || p.MinMatchScore > 100 {
		return ErrDevicePolicyInvalidScore
	}
	if p.StepUpScore < 0 || p.StepUpScore > 100 {
		return ErrDevicePolicyInvalidScore
	}
	if p.StepUpScore >= p.MinMatchScore {
		return ErrDevicePolicyStepUpTooHigh
	}
	if p.RebindCooldownSeconds < 0 {
		return ErrDevicePolicyInvalidCooldown
	}
	if p.MaxDeviceChangesPer30d < 0 {
		return ErrDevicePolicyInvalidChangeLimit
	}
	return nil
}
