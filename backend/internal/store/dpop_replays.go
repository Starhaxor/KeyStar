package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ConsumeDPoP atomically records a DPoP proof identifier for one
// application. The first consumption returns true; a primary-key conflict
// means the proof is a replay and returns false without an error. Only the
// SHA-256 of the canonical proof identifier, the token binding, and the
// expiry are stored — never access tokens, proofs, JWKs, or raw jti values.
// Database errors are returned so callers fail closed.
func (s *Store) ConsumeDPoP(ctx context.Context, applicationID string, jtiDigest [32]byte, tokenID string, expiresAt time.Time) (bool, error) {
	if strings.TrimSpace(applicationID) == "" || strings.TrimSpace(tokenID) == "" || expiresAt.IsZero() {
		return false, errors.New("invalid dpop replay consumption")
	}
	digest := append([]byte(nil), jtiDigest[:]...)
	var stored []byte
	err := s.db.QueryRow(ctx, `
		insert into dpop_replays (application_id, jti_digest, token_id, expires_at)
		values ($1::uuid, $2, $3, $4)
		on conflict do nothing
		returning jti_digest`,
		applicationID, digest, tokenID, expiresAt.UTC()).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consume dpop replay: %w", err)
	}
	return true, nil
}

// DeleteExpiredDPoPReplays removes replay records past their expiry.
// Correctness never depends on cleanup running; it only bounds table size.
func (s *Store) DeleteExpiredDPoPReplays(ctx context.Context) (int64, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin delete expired dpop replays: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `delete from dpop_replays where expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired dpop replays: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit delete expired dpop replays: %w", err)
	}
	return tag.RowsAffected(), nil
}
