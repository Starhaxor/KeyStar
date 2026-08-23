package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/starloader/backend/internal/domain"
)

const credentialColumns = `id::text, application_id::text, environment, credential_type, name, key_prefix, key_hash, scopes, status, last_used_at, expires_at, created_at, revoked_at`

func (s *Store) CreateCredential(ctx context.Context, input domain.NewApplicationCredential) (*domain.ApplicationCredential, error) {
	row := s.db.QueryRow(ctx, `
		insert into application_credentials (
			application_id, environment, credential_type, name, key_prefix, key_hash, scopes, expires_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8)
		returning `+credentialColumns,
		input.ApplicationID, input.Environment, input.CredentialType, input.Name,
		input.KeyPrefix, input.KeyHash, input.Scopes, input.ExpiresAt)
	credential, err := scanCredential(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "application_credentials_key_prefix_unique" {
			return nil, domain.ErrCredentialExists
		}
		return nil, fmt.Errorf("create credential: %w", err)
	}
	return credential, nil
}

// FindCredentialByPrefix resolves a credential by its locator prefix within
// one application. The application boundary makes cross-tenant prefix
// lookups impossible.
func (s *Store) FindCredentialByPrefix(ctx context.Context, applicationID, prefix string) (*domain.ApplicationCredential, error) {
	credential, err := scanCredential(s.db.QueryRow(ctx,
		`select `+credentialColumns+` from application_credentials where application_id = $1::uuid and key_prefix = $2`,
		applicationID, prefix))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrCredentialNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find credential by prefix: %w", err)
	}
	return credential, nil
}

func (s *Store) ListCredentials(ctx context.Context, applicationID string) ([]domain.ApplicationCredential, error) {
	rows, err := s.db.Query(ctx, `
		select `+credentialColumns+`
		from application_credentials
		where application_id = $1::uuid
		order by created_at desc, id desc`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer rows.Close()
	credentials := make([]domain.ApplicationCredential, 0)
	for rows.Next() {
		credential, err := scanCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		credentials = append(credentials, *credential)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	return credentials, nil
}

// RevokeCredential permanently disables a credential. Zero-downtime rotation
// is supported by keeping other active credentials of the same application
// untouched.
func (s *Store) RevokeCredential(ctx context.Context, applicationID, credentialID string) error {
	err := s.db.QueryRow(ctx, `
		update application_credentials
		set status = 'revoked', revoked_at = now()
		where id = $1::uuid and application_id = $2::uuid and status = 'active'
		returning id`, credentialID, applicationID).Scan(new(string))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrCredentialNotFound
	}
	if err != nil {
		return fmt.Errorf("revoke credential: %w", err)
	}
	return nil
}

// FindCredentialByID resolves one application credential by ID.
func (s *Store) FindCredentialByID(ctx context.Context, applicationID, credentialID string) (*domain.ApplicationCredential, error) {
	credential, err := scanCredential(s.db.QueryRow(ctx,
		`select `+credentialColumns+` from application_credentials where application_id = $1::uuid and id = $2::uuid`,
		applicationID, credentialID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrCredentialNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find credential by id: %w", err)
	}
	return credential, nil
}

// ExpireCredentialAt schedules a grace-period expiry for an active
// credential, keeping it valid until that instant (rotation window).
func (s *Store) ExpireCredentialAt(ctx context.Context, applicationID, credentialID string, expiresAt time.Time) error {
	err := s.db.QueryRow(ctx, `
		update application_credentials
		set expires_at = $3
		where id = $1::uuid and application_id = $2::uuid and status = 'active'
		returning id`, credentialID, applicationID, expiresAt).Scan(new(string))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrCredentialNotFound
	}
	if err != nil {
		return fmt.Errorf("expire credential: %w", err)
	}
	return nil
}

// TouchCredentialLastUsed records credential usage. Failures are swallowed by
// the verifier; a credential must never be rejected because telemetry failed.
func (s *Store) TouchCredentialLastUsed(ctx context.Context, applicationID, credentialID string) error {
	var id string
	err := s.db.QueryRow(ctx, `
		update application_credentials
		set last_used_at = now()
		where id = $1::uuid and application_id = $2::uuid
		returning id`, credentialID, applicationID).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("touch credential last used: %w", err)
	}
	return nil
}

func scanCredential(row pgx.Row) (*domain.ApplicationCredential, error) {
	var credential domain.ApplicationCredential
	var lastUsedAt, expiresAt, revokedAt pgtype.Timestamptz
	err := row.Scan(
		&credential.ID, &credential.ApplicationID, &credential.Environment, &credential.CredentialType,
		&credential.Name, &credential.KeyPrefix, &credential.KeyHash, &credential.Scopes, &credential.Status,
		&lastUsedAt, &expiresAt, &credential.CreatedAt, &revokedAt,
	)
	if lastUsedAt.Valid {
		credential.LastUsedAt = &lastUsedAt.Time
	}
	if expiresAt.Valid {
		credential.ExpiresAt = &expiresAt.Time
	}
	if revokedAt.Valid {
		credential.RevokedAt = &revokedAt.Time
	}
	return &credential, err
}
