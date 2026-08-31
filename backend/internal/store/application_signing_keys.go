package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/starloader/backend/internal/domain"
)

const applicationSigningKeyColumns = `
	id::text, kid, application_id::text, algorithm, public_key, encrypted_private_key,
	encryption_nonce, encryption_key_version, status, created_at, activated_at, retire_at, revoked_at`

func (s *Store) ListApplicationsWithoutSigningKey(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		select applications.id::text
		from applications
		where not exists (
			select 1 from application_signing_keys
			where application_signing_keys.application_id = applications.id
			and application_signing_keys.status = 'active'
		)
		order by applications.created_at, applications.id`)
	if err != nil {
		return nil, fmt.Errorf("list applications without signing key: %w", err)
	}
	defer rows.Close()

	applicationIDs := make([]string, 0)
	for rows.Next() {
		var applicationID string
		if err := rows.Scan(&applicationID); err != nil {
			return nil, fmt.Errorf("scan application without signing key: %w", err)
		}
		applicationIDs = append(applicationIDs, applicationID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list applications without signing key: %w", err)
	}
	return applicationIDs, nil
}

func (s *Store) CreateInitialSigningKey(
	ctx context.Context,
	applicationID string,
	key domain.NewApplicationSigningKey,
) (bool, error) {
	if key.ApplicationID != applicationID {
		return false, errors.New("initial signing key application does not match target application")
	}

	var keyID string
	err := s.db.QueryRow(ctx, `
		insert into application_signing_keys (
			kid, application_id, algorithm, public_key, encrypted_private_key,
			encryption_nonce, encryption_key_version, status, activated_at
		)
		select $2, $1::uuid, $3, $4, $5, $6, $7, $8, $9
		where not exists (
			select 1 from application_signing_keys
			where application_id = $1::uuid and status = 'active'
		)
		on conflict (application_id) where status = 'active' do nothing
		returning id::text`,
		applicationID,
		key.KID,
		key.Algorithm,
		key.PublicKey,
		key.EncryptedPrivateKey,
		key.EncryptionNonce,
		key.EncryptionKeyVersion,
		key.Status,
		key.ActivatedAt,
	).Scan(&keyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create initial application signing key: %w", err)
	}
	return true, nil
}

func (s *Store) ListApplicationSigningKeys(ctx context.Context, applicationID string) ([]domain.ApplicationSigningKey, error) {
	rows, err := s.db.Query(ctx, `
		select `+applicationSigningKeyColumns+`
		from application_signing_keys
		where application_id = $1::uuid
		order by created_at, id`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list application signing keys: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.ApplicationSigningKey, 0)
	for rows.Next() {
		key, err := scanApplicationSigningKey(rows)
		if err != nil {
			return nil, fmt.Errorf("scan application signing key: %w", err)
		}
		keys = append(keys, *key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list application signing keys: %w", err)
	}
	return keys, nil
}

func (s *Store) FindActiveApplicationSigningKey(ctx context.Context, applicationID string) (*domain.ApplicationSigningKey, error) {
	key, err := scanApplicationSigningKey(s.db.QueryRow(ctx, `
		select `+applicationSigningKeyColumns+`
		from application_signing_keys
		where application_id = $1::uuid and status = 'active'`, applicationID))
	if err != nil {
		return nil, fmt.Errorf("find active application signing key: %w", err)
	}
	return key, nil
}

func insertApplicationSigningKey(
	ctx context.Context,
	tx pgx.Tx,
	key domain.NewApplicationSigningKey,
) error {
	var keyID string
	err := tx.QueryRow(ctx, `
		insert into application_signing_keys (
			kid, application_id, algorithm, public_key, encrypted_private_key,
			encryption_nonce, encryption_key_version, status, activated_at
		) values ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9)
		returning id::text`,
		key.KID,
		key.ApplicationID,
		key.Algorithm,
		key.PublicKey,
		key.EncryptedPrivateKey,
		key.EncryptionNonce,
		key.EncryptionKeyVersion,
		key.Status,
		key.ActivatedAt,
	).Scan(&keyID)
	return err
}

func scanApplicationSigningKey(row pgx.Row) (*domain.ApplicationSigningKey, error) {
	var key domain.ApplicationSigningKey
	err := row.Scan(
		&key.ID,
		&key.KID,
		&key.ApplicationID,
		&key.Algorithm,
		&key.PublicKey,
		&key.EncryptedPrivateKey,
		&key.EncryptionNonce,
		&key.EncryptionKeyVersion,
		&key.Status,
		&key.CreatedAt,
		&key.ActivatedAt,
		&key.RetireAt,
		&key.RevokedAt,
	)
	return &key, err
}
