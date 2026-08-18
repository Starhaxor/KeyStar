package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/starloader/backend/internal/domain"
)

const licenseColumns = `id::text, application_id::text, license_hmac, user_id::text, product, status, level, max_devices, notes, expires_at, created_at, updated_at`

func (s *Store) CreateLicense(ctx context.Context, applicationID string, input domain.NewLicense) (*domain.License, error) {
	row := s.db.QueryRow(ctx, `
		insert into licenses (application_id, license_hmac, user_id, product, max_devices, expires_at)
		values ($1, $2, $3, $4, $5, $6)
		returning `+licenseColumns,
		applicationID, input.LicenseHMAC, input.UserID, input.Product, input.MaxDevices, input.ExpiresAt)
	license, err := scanLicense(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "licenses_user_product_unique" {
			return nil, domain.ErrLicenseAlreadyExists
		}
		return nil, fmt.Errorf("create license: %w", err)
	}
	return license, nil
}

func (s *Store) FindLicenseByUserAndProduct(ctx context.Context, applicationID, userID, product string) (*domain.License, error) {
	license, err := scanLicense(s.db.QueryRow(ctx,
		`select `+licenseColumns+` from licenses where application_id = $1::uuid and user_id = $2 and product = $3`,
		applicationID, userID, product))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrLicenseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find license by user and product: %w", err)
	}
	return license, nil
}

func (s *Store) FindLicenseByHMAC(ctx context.Context, applicationID, licenseHMAC string) (*domain.License, error) {
	license, err := scanLicense(s.db.QueryRow(ctx,
		`select `+licenseColumns+` from licenses where application_id = $1::uuid and license_hmac = $2`, applicationID, licenseHMAC))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrLicenseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find license by HMAC: %w", err)
	}
	return license, nil
}

func scanLicense(row pgx.Row) (*domain.License, error) {
	var license domain.License
	err := row.Scan(
		&license.ID,
		&license.ApplicationID,
		&license.LicenseHMAC,
		&license.UserID,
		&license.Product,
		&license.Status,
		&license.Level,
		&license.MaxDevices,
		&license.Notes,
		&license.ExpiresAt,
		&license.CreatedAt,
		&license.UpdatedAt,
	)
	return &license, err
}
