package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/starloader/backend/internal/domain"
)

const userColumns = `id::text, application_id::text, email, password_hash, status, ban_expires_at, created_at, updated_at`

func (s *Store) CreateUser(ctx context.Context, applicationID string, input domain.NewUser) (*domain.User, error) {
	row := s.db.QueryRow(ctx, `
		insert into users (application_id, email, password_hash)
		values ($1, $2, $3)
		returning `+userColumns, applicationID, normalizeEmail(input.Email), input.PasswordHash)
	user, err := scanUser(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "users_application_email_unique" {
			return nil, domain.ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (s *Store) FindUserByEmail(ctx context.Context, applicationID, email string) (*domain.User, error) {
	user, err := scanUser(s.db.QueryRow(ctx,
		`select `+userColumns+` from users where application_id = $1::uuid and email = $2`, applicationID, normalizeEmail(email)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by email: %w", err)
	}
	return user, nil
}

// SetUserPassword replaces the Argon2id hash of an end-user account. Used by
// admins to reset a forgotten password; the caller is responsible for handing
// the new password to the user over a trusted channel.
func (s *Store) SetUserPassword(ctx context.Context, applicationID, userID, passwordHash string) error {
	err := s.db.QueryRow(ctx, `
		update users
		set password_hash = $3, updated_at = now()
		where id = $2::uuid and application_id = $1::uuid
		returning id`, applicationID, userID, passwordHash).Scan(new(string))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("set user password: %w", err)
	}
	return nil
}

func (s *Store) FindUserByID(ctx context.Context, applicationID, userID string) (*domain.User, error) {
	user, err := scanUser(s.db.QueryRow(ctx,
		`select `+userColumns+` from users where application_id = $1::uuid and id = $2::uuid`, applicationID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return user, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func scanUser(row pgx.Row) (*domain.User, error) {
	var user domain.User
	var banExpiresAt pgtype.Timestamptz
	err := row.Scan(&user.ID, &user.ApplicationID, &user.Email, &user.PasswordHash, &user.Status, &banExpiresAt, &user.CreatedAt, &user.UpdatedAt)
	if banExpiresAt.Valid {
		user.BanExpiresAt = &banExpiresAt.Time
	}
	return &user, err
}
