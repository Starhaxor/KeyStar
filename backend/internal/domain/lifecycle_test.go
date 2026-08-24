package domain

import (
	"errors"
	"testing"
)

func TestApplicationTransitionAcceptsOnlyOperationalStates(t *testing.T) {
	for _, status := range []ApplicationStatus{
		ApplicationStatusActive,
		ApplicationStatusMaintenance,
		ApplicationStatusDisabled,
	} {
		if err := ValidateApplicationTransition(status); err != nil {
			t.Fatalf("ValidateApplicationTransition(%q) error = %v", status, err)
		}
	}

	if err := ValidateApplicationTransition(ApplicationStatusSuspended); !errors.Is(err, ErrInvalidApplicationTransition) {
		t.Fatalf("ValidateApplicationTransition(suspended) error = %v, want %v", err, ErrInvalidApplicationTransition)
	}
}

func TestCatalogIssuanceRejectsInactiveAndArchivedRecords(t *testing.T) {
	if err := ValidateCatalogIssuance(CatalogStatusActive, CatalogStatusActive); err != nil {
		t.Fatalf("ValidateCatalogIssuance(active, active) error = %v", err)
	}

	for _, status := range []CatalogStatus{CatalogStatusInactive, CatalogStatusArchived} {
		if err := ValidateCatalogIssuance(status, CatalogStatusActive); !errors.Is(err, ErrCatalogRecordInactive) {
			t.Fatalf("ValidateCatalogIssuance(%q, active) error = %v, want %v", status, err, ErrCatalogRecordInactive)
		}
		if err := ValidateCatalogIssuance(CatalogStatusActive, status); !errors.Is(err, ErrCatalogRecordInactive) {
			t.Fatalf("ValidateCatalogIssuance(active, %q) error = %v, want %v", status, err, ErrCatalogRecordInactive)
		}
	}
}

func TestCatalogRecordInUseExposesSafeConflictCode(t *testing.T) {
	var conflict interface{ Code() string }
	if !errors.As(ErrCatalogRecordInUse, &conflict) {
		t.Fatal("catalog in-use error must expose a conflict code")
	}
	if got := conflict.Code(); got != "CATALOG_RECORD_IN_USE" {
		t.Fatalf("catalog conflict code = %q, want CATALOG_RECORD_IN_USE", got)
	}
}
