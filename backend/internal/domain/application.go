package domain

import (
	"errors"
	"time"
)

type ApplicationStatus string

const (
	ApplicationStatusActive      ApplicationStatus = "active"
	ApplicationStatusMaintenance ApplicationStatus = "maintenance"
	ApplicationStatusDisabled    ApplicationStatus = "disabled"
	// ApplicationStatusSuspended remains for compatibility with records from
	// before migration 000016. New lifecycle transitions reject it.
	ApplicationStatusSuspended ApplicationStatus = "suspended"
)

type OrganizationStatus string

const (
	OrganizationStatusActive    OrganizationStatus = "active"
	OrganizationStatusSuspended OrganizationStatus = "suspended"
	OrganizationStatusDisabled  OrganizationStatus = "disabled"
)

// Application is the primary isolation boundary of the platform. Every
// end-user domain object (users, licenses, devices, sessions, variables) is
// bound to exactly one application.
type Application struct {
	ID              string
	OrganizationID  string
	Name            string
	Slug            string
	Status          ApplicationStatus
	EnvironmentMode string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Organization owns the applications of one tenant.
type Organization struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Slug      string             `json:"slug"`
	Status    OrganizationStatus `json:"status"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type NewApplication struct {
	OrganizationID string
	Name           string
	Slug           string
}

// UpdateApplication carries optional editable application fields.
type UpdateApplication struct {
	Name *string
	Slug *string
}

// ConflictError is safe to map directly to an API conflict response without
// leaking persistence details.
type ConflictError struct {
	ConflictCode string
	Message      string
}

func (e *ConflictError) Error() string { return e.Message }

func (e *ConflictError) Code() string { return e.ConflictCode }

var (
	ErrApplicationNotFound          = &NotFoundError{Entity: "application"}
	ErrOrganizationNotFound         = &NotFoundError{Entity: "organization"}
	ErrApplicationExists            = errors.New("an application with this slug already exists")
	ErrOrganizationExists           = errors.New("an organization with this slug already exists")
	ErrInvalidApplicationUpdate     = errors.New("application name and slug must not be empty")
	ErrInvalidApplicationTransition = errors.New("application status must be active, maintenance or disabled")
	ErrApplicationInUse             = &ConflictError{ConflictCode: "APPLICATION_IN_USE", Message: "application has active dependent records"}
	ErrApplicationInactive          = &ConflictError{ConflictCode: "APPLICATION_INACTIVE", Message: "application is not active"}
)

// ValidateApplicationTransition limits lifecycle transitions to operational
// states supported by the application boundary.
func ValidateApplicationTransition(status ApplicationStatus) error {
	switch status {
	case ApplicationStatusActive, ApplicationStatusMaintenance, ApplicationStatusDisabled:
		return nil
	default:
		return ErrInvalidApplicationTransition
	}
}
