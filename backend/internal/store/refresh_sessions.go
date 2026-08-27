package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/starloader/backend/internal/domain"
)

const refreshSessionColumns = `
	id::text, application_id::text, user_id::text, license_id::text, device_id::text,
	token_hash, status, expires_at, last_used_at, created_at, revoked_at`

// CreateRefreshSession stores a new refresh token hash.
func (s *Store) CreateRefreshSession(ctx context.Context, input domain.NewRefreshSession) (*domain.RefreshSession, error) {
	session, err := scanRefreshSession(s.db.QueryRow(ctx, `
		insert into refresh_sessions (
			application_id, user_id, license_id, device_id, token_hash, expires_at
		) values ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6)
		returning `+refreshSessionColumns,
		input.ApplicationID, input.UserID, input.LicenseID, input.DeviceID, input.TokenHash, input.ExpiresAt))
	if err != nil {
		return nil, fmt.Errorf("create refresh session: %w", err)
	}
	return session, nil
}

// FindRefreshSessionByTokenHash looks up a refresh session by the SHA-256 of
// the opaque token. This is the hot path on every refresh request.
func (s *Store) FindRefreshSessionByTokenHash(ctx context.Context, tokenHash []byte) (*domain.RefreshSession, error) {
	session, err := scanRefreshSession(s.db.QueryRow(ctx,
		`select `+refreshSessionColumns+` from refresh_sessions where token_hash = $1`, tokenHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrRefreshSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find refresh session: %w", err)
	}
	return session, nil
}

// RotateRefreshSession marks the old session as "rotated", records last_used_at,
// and returns the updated record. The caller is responsible for creating the
// new refresh session in the same transaction or request.
func (s *Store) RotateRefreshSession(ctx context.Context, sessionID string, now time.Time) (*domain.RefreshSession, error) {
	session, err := scanRefreshSession(s.db.QueryRow(ctx, `
		update refresh_sessions
		set status = 'rotated', last_used_at = $2, revoked_at = $2
		where id = $1::uuid and status = 'active'
		returning `+refreshSessionColumns, sessionID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrRefreshSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("rotate refresh session: %w", err)
	}
	return session, nil
}

// RevokeRefreshSession explicitly revokes a refresh token.
func (s *Store) RevokeRefreshSession(ctx context.Context, applicationID, sessionID string) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin revoke refresh session: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		update refresh_sessions
		set status = 'revoked', revoked_at = now()
		where application_id = $1::uuid and id = $2::uuid and status = 'active'`, applicationID, sessionID)
	if err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}
	_ = tag // idempotent: already revoked/rotated is fine
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit revoke refresh session: %w", err)
	}
	return nil
}

// RevokeRefreshSessionFamily revokes every active session for the same
// user+device pair (reuse detection). This is the nuclear option: when a
// rotated or revoked token is presented, the entire family is killed.
func (s *Store) RevokeRefreshSessionFamily(ctx context.Context, applicationID, userID, deviceID string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin revoke refresh session family: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		update refresh_sessions
		set status = 'revoked', revoked_at = now()
		where application_id = $1::uuid and user_id = $2::uuid and device_id = $3::uuid and status = 'active'`,
		applicationID, userID, deviceID)
	if err != nil {
		return 0, fmt.Errorf("revoke refresh session family: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit revoke refresh session family: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RevokeAllUserRefreshSessions revokes every active refresh token for a user
// across all devices. Used by admin "revoke all sessions" and logout-all.
func (s *Store) RevokeAllUserRefreshSessions(ctx context.Context, applicationID, userID string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin revoke all user refresh sessions: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		update refresh_sessions
		set status = 'revoked', revoked_at = now()
		where application_id = $1::uuid and user_id = $2::uuid and status = 'active'`, applicationID, userID)
	if err != nil {
		return 0, fmt.Errorf("revoke all user refresh sessions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit revoke all user refresh sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListRefreshSessions pages the refresh sessions for a user within an
// application. Supports cursor-based pagination.
func (s *Store) ListRefreshSessions(ctx context.Context, applicationID, userID, after string, limit int) ([]domain.RefreshSession, string, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		select `+refreshSessionColumns+`
		from refresh_sessions
		where application_id = $1::uuid and user_id = $2::uuid and ($3 = '' or id < $3::uuid)
		order by id desc
		limit $4`, applicationID, userID, after, limit+1)
	if err != nil {
		return nil, "", false, fmt.Errorf("list refresh sessions: %w", err)
	}
	defer rows.Close()
	sessions := make([]domain.RefreshSession, 0, limit+1)
	for rows.Next() {
		session, err := scanRefreshSession(rows)
		if err != nil {
			return nil, "", false, fmt.Errorf("scan refresh session: %w", err)
		}
		sessions = append(sessions, *session)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("list refresh sessions: %w", err)
	}
	hasMore := len(sessions) > limit
	if hasMore {
		sessions = sessions[:limit]
	}
	nextCursor := ""
	if hasMore {
		nextCursor = sessions[len(sessions)-1].ID
	}
	return sessions, nextCursor, hasMore, nil
}

// DeleteExpiredRefreshSessions removes refresh sessions past their expiry.
// Intended for periodic cleanup.
func (s *Store) DeleteExpiredRefreshSessions(ctx context.Context) (int64, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin delete expired refresh sessions: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		delete from refresh_sessions where expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired refresh sessions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit delete expired refresh sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func scanRefreshSession(row pgx.Row) (*domain.RefreshSession, error) {
	var session domain.RefreshSession
	var lastUsedAt, revokedAt *time.Time
	err := row.Scan(
		&session.ID, &session.ApplicationID, &session.UserID, &session.LicenseID, &session.DeviceID,
		&session.TokenHash, &session.Status, &session.ExpiresAt,
		&lastUsedAt, &session.CreatedAt, &revokedAt,
	)
	if lastUsedAt != nil {
		session.LastUsedAt = lastUsedAt
	}
	if revokedAt != nil {
		session.RevokedAt = revokedAt
	}
	return &session, err
}

// HashRefreshToken returns the SHA-256 of the opaque refresh token for
// storage. The token itself is a cryptographically random 32-byte value
// encoded as base64url (43 characters).
func HashRefreshToken(token []byte) []byte {
	digest := sha256.Sum256(token)
	return digest[:]
}
