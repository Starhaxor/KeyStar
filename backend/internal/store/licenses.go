package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/starloader/backend/internal/domain"
)

// licenseColumns selects the license row plus its resolved product name. Every
// query using these columns must join products on p.id = l.product_id.
const licenseColumns = `l.id::text, l.application_id::text, l.license_hmac, l.user_id::text, l.product_id::text, coalesce(l.plan_id::text, ''), p.name, l.status, l.level, l.max_devices, l.notes, l.expires_at, l.created_at, l.updated_at`

func (s *Store) CreateLicense(ctx context.Context, applicationID string, input domain.NewLicense) (*domain.License, error) {
	license, err := scanLicense(s.db.QueryRow(ctx, `
		with inserted as (
			insert into licenses (application_id, license_hmac, user_id, product_id, plan_id, max_devices, expires_at)
			values ($1, $2, $3, $4, $5, $6, $7)
			returning id, application_id, license_hmac, user_id, product_id, plan_id, status, level, max_devices, notes, expires_at, created_at, updated_at
		)
		select `+licenseColumns+`
		from inserted l
		join products p on p.id = l.product_id`,
		applicationID, input.LicenseHMAC, input.UserID, input.ProductID, input.PlanID, input.MaxDevices, input.ExpiresAt))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "licenses_user_product_unique" {
			return nil, domain.ErrLicenseAlreadyExists
		}
		return nil, fmt.Errorf("create license: %w", err)
	}
	return license, nil
}

// FindLicenseByUserAndProduct resolves a license by the user and the product
// display name. The name is unique per application because product slugs (and
// therefore names) are unique within an application.
func (s *Store) FindLicenseByUserAndProduct(ctx context.Context, applicationID, userID, product string) (*domain.License, error) {
	license, err := scanLicense(s.db.QueryRow(ctx,
		`select `+licenseColumns+` from licenses l
		 join products p on p.id = l.product_id and p.name = $3
		 where l.application_id = $1::uuid and l.user_id = $2`,
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
		`select `+licenseColumns+` from licenses l
		 join products p on p.id = l.product_id
		 where l.application_id = $1::uuid and l.license_hmac = $2`, applicationID, licenseHMAC))
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
		&license.ProductID,
		&license.PlanID,
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
