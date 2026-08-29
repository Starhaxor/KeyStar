package domain

import "errors"

// NotFoundError identifies a missing domain entity without exposing database
// implementation details.
type NotFoundError struct {
	Entity string
}

func (e *NotFoundError) Error() string {
	return e.Entity + " not found"
}

var (
	ErrUserNotFound          = &NotFoundError{Entity: "user"}
	ErrUserAlreadyExists     = errors.New("user already exists")
	ErrLicenseNotFound       = &NotFoundError{Entity: "license"}
	ErrLicenseAlreadyExists  = errors.New("license already exists for user and product")
	ErrChallengeNotFound     = &NotFoundError{Entity: "challenge"}
	ErrDeviceNotFound        = &NotFoundError{Entity: "device"}
	ErrAuthSessionNotFound   = &NotFoundError{Entity: "auth session"}
	ErrAdminAlreadyExists    = errors.New("admin account already exists")
	ErrAdminBootstrapClosed  = errors.New("admin bootstrap is already complete")
	ErrVariableNotFound      = &NotFoundError{Entity: "variable"}
	ErrVariableAlreadyExists = errors.New("a variable with this key already exists")
	ErrProductNotFound       = &NotFoundError{Entity: "product"}
	ErrPlanNotFound          = &NotFoundError{Entity: "plan"}
	ErrDevicePolicyNotFound  = &NotFoundError{Entity: "device policy"}

	ErrDevicePolicyInvalidTPMPolicy   = errors.New("invalid TPM policy: must be required, preferred or optional")
	ErrDevicePolicyInvalidScore       = errors.New("invalid score: must be between 0 and 100")
	ErrDevicePolicyStepUpTooHigh      = errors.New("step_up_score must be less than min_match_score")
	ErrDevicePolicyInvalidCooldown    = errors.New("rebind_cooldown_seconds must be non-negative")
	ErrDevicePolicyInvalidChangeLimit = errors.New("max_device_changes_per_30d must be non-negative")

	ErrRefreshSessionNotFound = &NotFoundError{Entity: "refresh session"}
	ErrRefreshTokenRevoked    = errors.New("refresh token has been revoked")
	ErrRefreshTokenRotated    = errors.New("refresh token has been rotated")
	ErrRefreshTokenExpired    = errors.New("refresh token has expired")
	ErrRefreshTokenReuse      = errors.New("refresh token reuse detected: family revoked")
)

// ChallengeConsumedError marks the single-use challenge conflict while
// remaining independent of PostgreSQL error details.
type ChallengeConsumedError struct{}

func (*ChallengeConsumedError) Error() string {
	return "challenge already consumed"
}

var ErrChallengeConsumed = &ChallengeConsumedError{}
