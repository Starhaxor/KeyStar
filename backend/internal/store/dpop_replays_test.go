package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starloader/backend/internal/domain"
)

// dpopContractDB is a scripted DB seam for replay consumption: rows drive
// QueryRow results in order; execTags drive transaction Exec outcomes.
type dpopContractDB struct {
	DB
	rows     []pgx.Row
	execTags []pgconn.CommandTag
	execErr  error
	queries  []string
}

func (db *dpopContractDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	db.queries = append(db.queries, sql)
	if len(db.rows) == 0 {
		return contractRow{err: pgx.ErrNoRows}
	}
	row := db.rows[0]
	db.rows = db.rows[1:]
	return row
}

func (db *dpopContractDB) BeginTx(_ context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	return &dpopContractTx{db: db}, nil
}

type dpopContractTx struct {
	pgx.Tx
	db *dpopContractDB
}

func (tx *dpopContractTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	tx.db.queries = append(tx.db.queries, sql)
	if tx.db.execErr != nil {
		return pgconn.CommandTag{}, tx.db.execErr
	}
	if len(tx.db.execTags) == 0 {
		return pgconn.NewCommandTag("DELETE 0"), nil
	}
	tag := tx.db.execTags[0]
	tx.db.execTags = tx.db.execTags[1:]
	return tag, nil
}

func (tx *dpopContractTx) Commit(context.Context) error   { return nil }
func (tx *dpopContractTx) Rollback(context.Context) error { return nil }

func testJTIDigest(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}

func TestConsumeDPoPInsertsOnceAndReportsReplay(t *testing.T) {
	digest := testJTIDigest("proof-jti-canonical")
	db := &dpopContractDB{rows: []pgx.Row{
		contractRow{values: []any{digest[:]}},
		contractRow{err: pgx.ErrNoRows},
	}}
	repository := New(db)
	expiresAt := time.Now().UTC().Add(10 * time.Minute)

	consumed, err := repository.ConsumeDPoP(context.Background(), "application-1", digest, "token-1", expiresAt)
	if err != nil || !consumed {
		t.Fatalf("ConsumeDPoP() first = (%v, %v), want (true, nil)", consumed, err)
	}
	consumed, err = repository.ConsumeDPoP(context.Background(), "application-1", digest, "token-1", expiresAt)
	if err != nil || consumed {
		t.Fatalf("ConsumeDPoP() replay = (%v, %v), want (false, nil)", consumed, err)
	}
	if len(db.queries) != 2 {
		t.Fatalf("ConsumeDPoP() issued %d statements, want 2 single-statement consumptions", len(db.queries))
	}
	for _, query := range db.queries {
		if !strings.Contains(query, "dpop_replays") || !strings.Contains(query, "on conflict do nothing") {
			t.Fatalf("ConsumeDPoP() must use single-statement insert ... on conflict do nothing, got %q", query)
		}
		if strings.Contains(query, "jti\"") || strings.Contains(strings.ToLower(query), "proof") && strings.Contains(query, "returning *") {
			t.Fatalf("ConsumeDPoP() must not return raw proof material, got %q", query)
		}
	}
}

func TestConsumeDPoPRejectsInvalidInputWithoutTouchingDB(t *testing.T) {
	db := &dpopContractDB{}
	repository := New(db)
	digest := testJTIDigest("proof-jti-canonical")
	expiresAt := time.Now().UTC().Add(10 * time.Minute)

	for _, test := range []struct {
		name          string
		applicationID string
		tokenID       string
		expiresAt     time.Time
	}{
		{name: "empty application", applicationID: "", tokenID: "token-1", expiresAt: expiresAt},
		{name: "blank application", applicationID: "  ", tokenID: "token-1", expiresAt: expiresAt},
		{name: "empty token", applicationID: "application-1", tokenID: "", expiresAt: expiresAt},
		{name: "zero expiry", applicationID: "application-1", tokenID: "token-1", expiresAt: time.Time{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if consumed, err := repository.ConsumeDPoP(context.Background(), test.applicationID, digest, test.tokenID, test.expiresAt); err == nil || consumed {
				t.Fatalf("ConsumeDPoP() = (%v, %v), want fail-closed error", consumed, err)
			}
		})
	}
	if len(db.queries) != 0 {
		t.Fatalf("invalid consumption reached the database with %d statements", len(db.queries))
	}
}

func TestConsumeDPoPPropagatesDBErrorsFailClosed(t *testing.T) {
	db := &dpopContractDB{rows: []pgx.Row{contractRow{err: errors.New("connection reset")}}}
	repository := New(db)

	consumed, err := repository.ConsumeDPoP(context.Background(), "application-1", testJTIDigest("jti"), "token-1", time.Now().UTC().Add(time.Minute))
	if err == nil || consumed {
		t.Fatalf("ConsumeDPoP() = (%v, %v), want fail-closed error", consumed, err)
	}
}

func TestDeleteExpiredDPoPReplaysRemovesOnlyExpired(t *testing.T) {
	db := &dpopContractDB{execTags: []pgconn.CommandTag{pgconn.NewCommandTag("DELETE 3")}}
	repository := New(db)

	deleted, err := repository.DeleteExpiredDPoPReplays(context.Background())
	if err != nil || deleted != 3 {
		t.Fatalf("DeleteExpiredDPoPReplays() = (%d, %v), want (3, nil)", deleted, err)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0], "dpop_replays") || !strings.Contains(db.queries[0], "expires_at") {
		t.Fatalf("DeleteExpiredDPoPReplays() queries = %#v", db.queries)
	}
}

func TestDeleteExpiredDPoPReplaysPropagatesDBErrors(t *testing.T) {
	db := &dpopContractDB{execErr: errors.New("connection reset")}
	repository := New(db)

	if _, err := repository.DeleteExpiredDPoPReplays(context.Background()); err == nil {
		t.Fatal("DeleteExpiredDPoPReplays() = nil, want database error")
	}
}

// TestExternalDPoPReplayConsumption proves atomic application-scoped replay
// rejection against PostgreSQL when TEST_DATABASE_URL is configured.
func TestExternalDPoPReplayConsumption(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	repository := New(pool)

	application, err := repository.FindApplicationBySlug(ctx, "starloader")
	if err != nil {
		t.Fatal(err)
	}
	secondOrg, err := repository.CreateOrganization(ctx, fmt.Sprintf("DPoP Replay Test %d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.CreateApplication(ctx, domain.NewApplication{
		OrganizationID: secondOrg.ID,
		Name:           fmt.Sprintf("DPoP Replay Test %d", time.Now().UnixNano()),
		Slug:           fmt.Sprintf("dpop-replay-test-%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteTestDPoPApplication(t, repository, second.ID, secondOrg.ID) })

	t.Run("uniqueness", func(t *testing.T) {
		digest := testJTIDigest("external-unique-jti")
		expiresAt := time.Now().UTC().Add(10 * time.Minute)
		consumed, err := repository.ConsumeDPoP(ctx, application.ID, digest, "token-ext-1", expiresAt)
		if err != nil || !consumed {
			t.Fatalf("first consume = (%v, %v)", consumed, err)
		}
		consumed, err = repository.ConsumeDPoP(ctx, application.ID, digest, "token-ext-1", expiresAt)
		if err != nil || consumed {
			t.Fatalf("replay consume = (%v, %v), want (false, nil)", consumed, err)
		}
	})

	t.Run("tenant isolation", func(t *testing.T) {
		digest := testJTIDigest("external-tenant-jti")
		expiresAt := time.Now().UTC().Add(10 * time.Minute)
		if consumed, err := repository.ConsumeDPoP(ctx, application.ID, digest, "token-ext-1", expiresAt); err != nil || !consumed {
			t.Fatalf("first tenant consume = (%v, %v)", consumed, err)
		}
		if consumed, err := repository.ConsumeDPoP(ctx, second.ID, digest, "token-ext-2", expiresAt); err != nil || !consumed {
			t.Fatalf("second tenant consume = (%v, %v), want (true, nil)", consumed, err)
		}
	})

	t.Run("digest length constraint", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, `insert into dpop_replays (application_id, jti_digest, token_id, expires_at)
			values ($1::uuid, $2, 'token-ext-1', now() + interval '1 minute')`, application.ID, make([]byte, 31))
		if err == nil {
			t.Fatal("31-byte jti_digest insert succeeded, want check-constraint failure")
		}
	})

	t.Run("concurrent atomic", func(t *testing.T) {
		digest := testJTIDigest("external-race-jti")
		expiresAt := time.Now().UTC().Add(10 * time.Minute)
		results := make(chan bool, 8)
		for index := 0; index < 8; index++ {
			go func() {
				consumed, err := repository.ConsumeDPoP(ctx, application.ID, digest, "token-ext-race", expiresAt)
				results <- err == nil && consumed
			}()
		}
		winners := 0
		for index := 0; index < 8; index++ {
			if <-results {
				winners++
			}
		}
		if winners != 1 {
			t.Fatalf("concurrent consume winners = %d, want exactly 1", winners)
		}
	})

	t.Run("expiry cleanup", func(t *testing.T) {
		digest := testJTIDigest("external-expired-jti")
		if _, err := repository.ConsumeDPoP(ctx, application.ID, digest, "token-ext-old", time.Now().UTC().Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		deleted, err := repository.DeleteExpiredDPoPReplays(ctx)
		if err != nil || deleted < 1 {
			t.Fatalf("cleanup deleted = (%d, %v)", deleted, err)
		}
		consumed, err := repository.ConsumeDPoP(ctx, application.ID, digest, "token-ext-old", time.Now().UTC().Add(10*time.Minute))
		if err != nil || !consumed {
			t.Fatalf("consume after cleanup = (%v, %v), want (true, nil)", consumed, err)
		}
	})

	t.Run("database error fails closed", func(t *testing.T) {
		consumed, err := repository.ConsumeDPoP(ctx, "00000000-0000-0000-0000-000000000000", testJTIDigest("external-fk-jti"), "token-ext-x", time.Now().UTC().Add(10*time.Minute))
		if err == nil || consumed {
			t.Fatalf("unknown-application consume = (%v, %v), want fail-closed error", consumed, err)
		}
	})
}

// deleteTestDPoPApplication removes the tenant-isolation fixture in
// dependency order. It runs in-package through the store DB seam.
func deleteTestDPoPApplication(t *testing.T, repository *Store, applicationID, organizationID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := repository.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin dpop test cleanup: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, statement := range []struct {
		sql string
		arg string
	}{
		{sql: `delete from dpop_replays where application_id = $1::uuid`, arg: applicationID},
		{sql: `delete from applications where id = $1::uuid`, arg: applicationID},
		{sql: `delete from organizations where id = $1::uuid`, arg: organizationID},
	} {
		if _, err := tx.Exec(ctx, statement.sql, statement.arg); err != nil {
			t.Fatalf("dpop test cleanup %q: %v", statement.sql, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit dpop test cleanup: %v", err)
	}
}
