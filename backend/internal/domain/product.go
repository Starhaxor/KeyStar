package domain

import (
	"errors"
	"strings"
	"time"
)

type CatalogStatus string

const (
	CatalogStatusActive   CatalogStatus = "active"
	CatalogStatusInactive CatalogStatus = "inactive"
	CatalogStatusArchived CatalogStatus = "archived"
)

// Product is one entry of an application's product catalog. Licenses no
// longer carry a free-text product name; they reference products.id and the
// display name is resolved through this table (migration 000010).
type Product struct {
	ID            string        `json:"id"`
	ApplicationID string        `json:"application_id"`
	Name          string        `json:"name"`
	Slug          string        `json:"slug"`
	Status        CatalogStatus `json:"status"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// UpdateProduct carries editable catalog fields. Lifecycle changes use the
// explicit archive operation so history cannot be removed accidentally.
type UpdateProduct struct {
	Name   *string
	Slug   *string
	Status *CatalogStatus
}

// NewProduct carries the fields needed to create a catalog product. Slug is
// derived by ProductSlug when empty.
type NewProduct struct {
	ApplicationID string
	Name          string
	Slug          string
}

// Plan is an entitlement tier of one product (Free, Basic, Premium, ...).
// Every license is bound to a plan so device limits and levels can later be
// inherited from a single catalog row instead of per-license values.
type Plan struct {
	ID                     string        `json:"id"`
	ProductID              string        `json:"product_id"`
	Name                   string        `json:"name"`
	Code                   string        `json:"code"`
	Level                  int           `json:"level"`
	MaxDevices             int           `json:"max_devices"`
	DefaultDurationSeconds *int64        `json:"default_duration_seconds"`
	Status                 CatalogStatus `json:"status"`
	CreatedAt              time.Time     `json:"created_at"`
	UpdatedAt              time.Time     `json:"updated_at"`
}

// UpdatePlan carries editable catalog fields.
type UpdatePlan struct {
	Name       *string
	Code       *string
	Level      *int
	MaxDevices *int
	Status     *CatalogStatus
}

// NewPlan carries the fields needed to create a plan.
type NewPlan struct {
	ProductID  string
	Name       string
	Code       string
	Level      int
	MaxDevices int
}

// ErrProductInvalidName marks a product name that cannot be normalized into a
// unique slug (empty or non-alphanumeric only).
var (
	ErrProductInvalidName    = errors.New("invalid product name")
	ErrInvalidCatalogStatus  = errors.New("catalog status must be active, inactive or archived")
	ErrCatalogRecordInactive = &ConflictError{ConflictCode: "CATALOG_RECORD_INACTIVE", Message: "catalog record is inactive or archived"}
	ErrCatalogRecordInUse    = &ConflictError{ConflictCode: "CATALOG_RECORD_IN_USE", Message: "catalog record has active dependent licenses or plans"}
)

func ValidateCatalogStatus(status CatalogStatus) error {
	switch status {
	case CatalogStatusActive, CatalogStatusInactive, CatalogStatusArchived:
		return nil
	default:
		return ErrInvalidCatalogStatus
	}
}

// ValidateCatalogUpdateStatus permits ordinary activation/inactivation while
// reserving archive for the explicit, dependency-checked archive operations.
func ValidateCatalogUpdateStatus(status CatalogStatus) error {
	switch status {
	case CatalogStatusActive, CatalogStatusInactive:
		return nil
	default:
		return ErrInvalidCatalogStatus
	}
}

// ValidateCatalogIssuance prevents new licenses from referencing historical
// catalog records while retaining those references on existing licenses.
func ValidateCatalogIssuance(productStatus, planStatus CatalogStatus) error {
	if productStatus != CatalogStatusActive || planStatus != CatalogStatusActive {
		return ErrCatalogRecordInactive
	}
	return nil
}

// ProductSlug normalizes a display name into the unique key shared with the
// migration 000010 backfill: lowercase with every run of non-alphanumeric
// characters collapsed into a single dash (regexp_replace(...,'[^a-zA-Z0-9]+','-','g')).
func ProductSlug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var builder strings.Builder
	dashPending := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dashPending {
				builder.WriteByte('-')
				dashPending = false
			}
			builder.WriteRune(r)
		} else {
			dashPending = true
		}
	}
	return strings.Trim(builder.String(), "-")
}
