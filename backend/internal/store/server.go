package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/starloader/backend/internal/domain"
)

const serverUserColumns = `id::text, email, status, notes, ban_reason, banned_at, ban_expires_at, created_at, updated_at`

const serverLicenseColumns = `l.id::text, l.user_id::text, u.email, l.product_id::text, l.plan_id::text, p.name, l.status, l.level, l.max_devices, l.notes, l.expires_at, l.created_at, l.updated_at`

// ListServerUsers pages the end users of one application newest-first using a
// UUIDv7 cursor (id < after). limit+1 rows are fetched to report has_more.
func (s *Store) ListServerUsers(ctx context.Context, applicationID, after string, limit int) ([]domain.ServerUser, string, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		select `+serverUserColumns+`
		from users
		where application_id = $1::uuid and ($2 = '' or id < $2::uuid)
		order by id desc
		limit $3`, applicationID, after, limit+1)
	if err != nil {
		return nil, "", false, fmt.Errorf("list server users: %w", err)
	}
	defer rows.Close()
	users := make([]domain.ServerUser, 0, limit+1)
	for rows.Next() {
		user, err := scanServerUser(rows)
		if err != nil {
			return nil, "", false, fmt.Errorf("scan server user: %w", err)
		}
		users = append(users, *user)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("list server users: %w", err)
	}
	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}
	nextCursor := ""
	if hasMore {
		nextCursor = users[len(users)-1].ID
	}
	return users, nextCursor, hasMore, nil
}

func (s *Store) FindServerUserByID(ctx context.Context, applicationID, userID string) (*domain.ServerUser, error) {
	user, err := scanServerUser(s.db.QueryRow(ctx,
		`select `+serverUserColumns+` from users where application_id = $1::uuid and id = $2::uuid`,
		applicationID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find server user: %w", err)
	}
	return user, nil
}

// ListServerLicenses pages the licenses of one application newest-first with
// a UUIDv7 cursor. The email of the owning user is joined for display.
func (s *Store) ListServerLicenses(ctx context.Context, applicationID, after string, limit int) ([]domain.ServerLicense, string, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		select `+serverLicenseColumns+`
		from licenses l
		join users u on u.id = l.user_id
		join products p on p.id = l.product_id
		where l.application_id = $1::uuid and ($2 = '' or l.id < $2::uuid)
		order by l.id desc
		limit $3`, applicationID, after, limit+1)
	if err != nil {
		return nil, "", false, fmt.Errorf("list server licenses: %w", err)
	}
	defer rows.Close()
	licenses := make([]domain.ServerLicense, 0, limit+1)
	for rows.Next() {
		license, err := scanServerLicense(rows)
		if err != nil {
			return nil, "", false, fmt.Errorf("scan server license: %w", err)
		}
		licenses = append(licenses, *license)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("list server licenses: %w", err)
	}
	hasMore := len(licenses) > limit
	if hasMore {
		licenses = licenses[:limit]
	}
	nextCursor := ""
	if hasMore {
		nextCursor = licenses[len(licenses)-1].ID
	}
	return licenses, nextCursor, hasMore, nil
}

func (s *Store) FindServerLicenseByID(ctx context.Context, applicationID, licenseID string) (*domain.ServerLicense, error) {
	license, err := scanServerLicense(s.db.QueryRow(ctx, `
		select `+serverLicenseColumns+`
		from licenses l
		join users u on u.id = l.user_id
		join products p on p.id = l.product_id
		where l.application_id = $1::uuid and l.id = $2::uuid`, applicationID, licenseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrLicenseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find server license: %w", err)
	}
	return license, nil
}

func scanServerUser(row pgx.Row) (*domain.ServerUser, error) {
	var user domain.ServerUser
	var bannedAt, banExpiresAt pgtype.Timestamptz
	err := row.Scan(
		&user.ID, &user.Email, &user.Status, &user.Notes, &user.BanReason,
		&bannedAt, &banExpiresAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if bannedAt.Valid {
		user.BannedAt = &bannedAt.Time
	}
	if banExpiresAt.Valid {
		user.BanExpiresAt = &banExpiresAt.Time
	}
	return &user, err
}

func scanServerLicense(row pgx.Row) (*domain.ServerLicense, error) {
	var license domain.ServerLicense
	err := row.Scan(
		&license.ID, &license.UserID, &license.UserEmail,
		&license.ProductID, &license.PlanID, &license.Product, &license.Status,
		&license.Level, &license.MaxDevices, &license.Notes, &license.ExpiresAt,
		&license.CreatedAt, &license.UpdatedAt,
	)
	return &license, err
}
