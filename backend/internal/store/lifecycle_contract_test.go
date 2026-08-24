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

func TestUpdatePlanRejectsReactivationUnderArchivedProduct(t *testing.T) {
	active := domain.CatalogStatusActive
	tx := &lifecycleContractTx{rows: []pgx.Row{
		contractRow{values: []any{domain.CatalogStatusArchived, domain.CatalogStatusInactive}},
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
		contractRow{values: []any{domain.CatalogStatusActive, domain.CatalogStatusActive}},
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
