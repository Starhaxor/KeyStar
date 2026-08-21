package domain

import (
	"errors"
	"time"
)

type ApplicationStatus string

const (
	ApplicationStatusActive      ApplicationStatus = "active"
	ApplicationStatusMaintenance ApplicationStatus = "maintenance"
	ApplicationStatusSuspended   ApplicationStatus = "suspended"
	ApplicationStatusDisabled    ApplicationStatus = "disabled"
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

var (
	ErrApplicationNotFound  = &NotFoundError{Entity: "application"}
	ErrOrganizationNotFound = &NotFoundError{Entity: "organization"}
	ErrApplicationExists    = errors.New("an application with this slug already exists")
	ErrOrganizationExists   = errors.New("an organization with this slug already exists")
)
