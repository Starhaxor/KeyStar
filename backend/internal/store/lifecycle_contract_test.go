package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/starloader/backend/internal/domain"
)

// These contract tests use the Store DB seam because the PostgreSQL-backed
// suite is optional in this repository. They exercise public store behavior
// and the typed errors callers receive from lifecycle guards.
func TestUpdateProductToInactiveRejectsActiveDependencies(t *testing.T) {
	inactive := domain.CatalogStatusInactive
	tx := &lifecycleContractTx{rows: []pgx.Row{
		productContractRow(domain.CatalogStatusActive),
		contractRow{values: []any{true}},
	}}
	db := &lifecycleContractDB{
		tx: tx,
		// The pre-fix direct update reports success; after the guard is added,
		// the transaction rows above drive the observable conflict.
		rows: []pgx.Row{productContractRow(domain.CatalogStatusInactive)},
	}

	_, err := New(db).UpdateProduct(context.Background(), "application-1", "product-1", domain.UpdateProduct{Status: &inactive})
	if !errors.Is(err, domain.ErrCatalogRecordInUse) {
		t.Fatalf("UpdateProduct(inactive) error = %v, want %v", err, domain.ErrCatalogRecordInUse)
	}
}

func TestCreateOrganizationNormalizesStoredName(t *testing.T) {
	now := time.Now().UTC()
	db := &lifecycleContractDB{rows: []pgx.Row{contractRow{values: []any{
		"organization-1", "mixed organization", "mixed organization", domain.OrganizationStatusActive, now, now,
	}}}}

	organization, err := New(db).CreateOrganization(context.Background(), "  Mixed Organization  ")
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	if organization.Name != "mixed organization" {
		t.Fatalf("CreateOrganization() name = %q, want normalized lowercase name", organization.Name)
	}
	if len(db.queries) != 1 {
		t.Fatalf("CreateOrganization() query count = %d, want 1", len(db.queries))
	}
	if got := db.queries[0].args[0]; got != "mixed organization" {
		t.Fatalf("CreateOrganization() stored name = %q, want normalized lowercase name", got)
	}
	if got := db.queries[0].args[1]; got != "mixed organization" {
		t.Fatalf("CreateOrganization() slug = %q, want trimmed lowercase slug", got)
	}
}

func TestUpdatePlanRejectsReactivationUnderArchivedProduct(t *testing.T) {
	active := domain.CatalogStatusActive
	tx := &lifecycleContractTx{rows: []pgx.Row{
		contractRow{values: []any{domain.ApplicationStatusActive, domain.CatalogStatusArchived, domain.CatalogStatusInactive}},
	}}
	db := &lifecycleContractDB{
		tx: tx,
		// The pre-fix query can update this plan despite its archived parent.
		rows: []pgx.Row{planContractRow(domain.CatalogStatusActive)},
	}

	_, err := New(db).UpdatePlan(context.Background(), "application-1", "product-1", "plan-1", domain.UpdatePlan{Status: &active})
	if !errors.Is(err, domain.ErrCatalogRecordInactive) {
		t.Fatalf("UpdatePlan(active under archived product) error = %v, want %v", err, domain.ErrCatalogRecordInactive)
	}
}

func TestUpdatePlanToInactiveRejectsActiveLicenses(t *testing.T) {
	inactive := domain.CatalogStatusInactive
	tx := &lifecycleContractTx{rows: []pgx.Row{
		contractRow{values: []any{domain.ApplicationStatusActive, domain.CatalogStatusActive, domain.CatalogStatusActive}},
		contractRow{values: []any{true}},
	}}
	db := &lifecycleContractDB{tx: tx}

	_, err := New(db).UpdatePlan(context.Background(), "application-1", "product-1", "plan-1", domain.UpdatePlan{Status: &inactive})
	if !errors.Is(err, domain.ErrCatalogRecordInUse) {
		t.Fatalf("UpdatePlan(inactive) error = %v, want %v", err, domain.ErrCatalogRecordInUse)
	}
}

func TestUpdatePlanScopesMissingPlanToApplication(t *testing.T) {
	name := "renamed"
	db := &lifecycleContractDB{tx: &lifecycleContractTx{rows: []pgx.Row{
		contractRow{err: pgx.ErrNoRows},
	}}, rows: []pgx.Row{
		contractRow{err: pgx.ErrNoRows},
		contractRow{err: pgx.ErrNoRows},
	}}

	_, err := New(db).UpdatePlan(context.Background(), "other-application", "product-1", "plan-1", domain.UpdatePlan{Name: &name})
	if !errors.Is(err, domain.ErrPlanNotFound) {
		t.Fatalf("UpdatePlan(other application) error = %v, want %v", err, domain.ErrPlanNotFound)
	}
	if len(db.tx.(*lifecycleContractTx).queries) != 1 || !strings.Contains(db.tx.(*lifecycleContractTx).queries[0].sql, "p.application_id") {
		t.Fatal("UpdatePlan must scope the locked plan lookup by application")
	}
	if got := db.tx.(*lifecycleContractTx).queries[0].args[2]; got != "other-application" {
		t.Fatalf("UpdatePlan application scope = %v, want other-application", got)
	}
}

func TestCreateLicenseRejectsIneligibleCatalogRows(t *testing.T) {
	for _, name := range []string{"inactive catalog", "archived catalog"} {
		t.Run(name, func(t *testing.T) {
			db := &lifecycleContractDB{rows: []pgx.Row{contractRow{err: pgx.ErrNoRows}}}
			_, err := New(db).CreateLicense(context.Background(), "application-1", domain.NewLicense{
				LicenseHMAC: "license-hmac", UserID: "user-1", ProductID: "product-1", PlanID: "plan-1",
				MaxDevices: 1, ExpiresAt: time.Now().UTC(),
			})
			if !errors.Is(err, domain.ErrCatalogRecordInactive) {
				t.Fatalf("CreateLicense() error = %v, want %v", err, domain.ErrCatalogRecordInactive)
			}
			if len(db.queries) != 1 || !strings.Contains(db.queries[0].sql, "p.status = 'active'") || !strings.Contains(db.queries[0].sql, "pl.status = 'active'") {
				t.Fatal("CreateLicense must require active product and plan rows")
			}
		})
	}
}

func TestResolveProductPlanRejectsProductArchivedDuringDefaultPlanCreation(t *testing.T) {
	tx := &lifecycleContractTx{rows: []pgx.Row{
		contractRow{values: []any{domain.ApplicationStatusActive}},
		contractRow{err: pgx.ErrNoRows},
		contractRow{values: []any{"product-1", domain.CatalogStatusActive}},
		contractRow{err: pgx.ErrNoRows},
		contractRow{values: []any{domain.CatalogStatusArchived}},
	}}
	db := &lifecycleContractDB{tx: tx}

	_, _, err := New(db).ResolveProductPlan(context.Background(), "application-1", "Product")
	if !errors.Is(err, domain.ErrCatalogRecordInactive) {
		t.Fatalf("ResolveProductPlan() error = %v, want %v", err, domain.ErrCatalogRecordInactive)
	}
	if len(tx.queries) < 4 || !strings.Contains(tx.queries[3].sql, "status = 'active'") || !strings.Contains(tx.queries[3].sql, "for key share") {
		t.Fatal("default plan creation must lock and require an active product")
	}
}

func TestUpdateProductActivationRequiresActiveApplication(t *testing.T) {
	active := domain.CatalogStatusActive
	tx := &lifecycleContractTx{rows: []pgx.Row{contractRow{values: []any{domain.ApplicationStatusDisabled}}}}
	db := &lifecycleContractDB{tx: tx, rows: []pgx.Row{productContractRow(domain.CatalogStatusActive)}}

	_, err := New(db).UpdateProduct(context.Background(), "application-1", "product-1", domain.UpdateProduct{Status: &active})
	if !errors.Is(err, domain.ErrApplicationInactive) {
		t.Fatalf("UpdateProduct(active) error = %v, want %v", err, domain.ErrApplicationInactive)
	}
	if len(tx.queries) != 1 || !strings.Contains(tx.queries[0].sql, "applications") || !strings.Contains(tx.queries[0].sql, "for update") {
		t.Fatal("product activation must lock its parent application")
	}
}

func TestUpdatePlanActivationRequiresActiveApplication(t *testing.T) {
	active := domain.CatalogStatusActive
	tx := &lifecycleContractTx{rows: []pgx.Row{contractRow{values: []any{domain.ApplicationStatusDisabled, domain.CatalogStatusActive, domain.CatalogStatusInactive}}}}
	db := &lifecycleContractDB{tx: tx}

	_, err := New(db).UpdatePlan(context.Background(), "application-1", "product-1", "plan-1", domain.UpdatePlan{Status: &active})
	if !errors.Is(err, domain.ErrApplicationInactive) {
		t.Fatalf("UpdatePlan(active) error = %v, want %v", err, domain.ErrApplicationInactive)
	}
}

func TestCreateProductRequiresActiveApplication(t *testing.T) {
	db := &lifecycleContractDB{tx: &lifecycleContractTx{rows: []pgx.Row{
		contractRow{values: []any{domain.ApplicationStatusDisabled}},
	}}}

	_, err := New(db).CreateProduct(context.Background(), "application-1", domain.NewProduct{Name: "Product"})
	if !errors.Is(err, domain.ErrApplicationInactive) {
		t.Fatalf("CreateProduct() error = %v, want %v", err, domain.ErrApplicationInactive)
	}
	queries := db.tx.(*lifecycleContractTx).queries
	if len(queries) != 1 || !strings.Contains(queries[0].sql, "applications") || !strings.Contains(queries[0].sql, "for update") {
		t.Fatal("CreateProduct must lock and require its parent application")
	}
}

func TestResolveProductPlanRequiresActiveApplication(t *testing.T) {
	db := &lifecycleContractDB{tx: &lifecycleContractTx{rows: []pgx.Row{
		contractRow{values: []any{domain.ApplicationStatusDisabled}},
	}}}

	_, _, err := New(db).ResolveProductPlan(context.Background(), "application-1", "Product")
	if !errors.Is(err, domain.ErrApplicationInactive) {
		t.Fatalf("ResolveProductPlan() error = %v, want %v", err, domain.ErrApplicationInactive)
	}
	queries := db.tx.(*lifecycleContractTx).queries
	if len(queries) != 1 || !strings.Contains(queries[0].sql, "applications") || !strings.Contains(queries[0].sql, "for update") {
		t.Fatal("ResolveProductPlan must lock and require its parent application")
	}
}

type lifecycleContractDB struct {
	DB
	rows    []pgx.Row
	tx      pgx.Tx
	queries []lifecycleQuery
}

func (db *lifecycleContractDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	db.queries = append(db.queries, lifecycleQuery{sql: sql, args: args})
	if len(db.rows) == 0 {
		return contractRow{err: pgx.ErrNoRows}
	}
	row := db.rows[0]
	db.rows = db.rows[1:]
	return row
}

func (db *lifecycleContractDB) BeginTx(_ context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	return db.tx, nil
}

type lifecycleContractTx struct {
	pgx.Tx
	rows    []pgx.Row
	queries []lifecycleQuery
}

func (tx *lifecycleContractTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	tx.queries = append(tx.queries, lifecycleQuery{sql: sql, args: args})
	if len(tx.rows) == 0 {
		return contractRow{err: pgx.ErrNoRows}
	}
	row := tx.rows[0]
	tx.rows = tx.rows[1:]
	return row
}

type lifecycleQuery struct {
	sql  string
	args []any
}

func (*lifecycleContractTx) Commit(context.Context) error   { return nil }
func (*lifecycleContractTx) Rollback(context.Context) error { return nil }

type contractRow struct {
	values []any
	err    error
}

func (row contractRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != len(row.values) {
		return errors.New("unexpected lifecycle contract scan shape")
	}
	for index, value := range row.values {
		target := reflect.ValueOf(dest[index])
		target.Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

func productContractRow(status domain.CatalogStatus) pgx.Row {
	now := time.Now().UTC()
	return contractRow{values: []any{"product-1", "application-1", "Product", "product", status, now, now}}
}

func planContractRow(status domain.CatalogStatus) pgx.Row {
	now := time.Now().UTC()
	var duration *int64
	return contractRow{values: []any{"plan-1", "product-1", "Plan", "plan", 1, 1, duration, status, now, now}}
}
