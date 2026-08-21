package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/starloader/backend/internal/domain"
)

const applicationColumns = `id::text, organization_id::text, name, slug, status, environment_mode, created_at, updated_at`

const organizationColumns = `id::text, name, slug, status, created_at, updated_at`

func (s *Store) CreateApplication(ctx context.Context, input domain.NewApplication) (*domain.Application, error) {
	row := s.db.QueryRow(ctx, `
		insert into applications (organization_id, name, slug)
		values ($1, $2, $3)
		returning `+applicationColumns,
		input.OrganizationID, strings.TrimSpace(input.Name), normalizeSlug(input.Slug))
	application, err := scanApplication(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "applications_slug_unique" {
			return nil, domain.ErrApplicationExists
		}
		return nil, fmt.Errorf("create application: %w", err)
	}
	return application, nil
}

func (s *Store) FindApplicationByID(ctx context.Context, applicationID string) (*domain.Application, error) {
	application, err := scanApplication(s.db.QueryRow(ctx,
		`select `+applicationColumns+` from applications where id = $1::uuid`, applicationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrApplicationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find application by id: %w", err)
	}
	return application, nil
}

func (s *Store) FindApplicationBySlug(ctx context.Context, slug string) (*domain.Application, error) {
	application, err := scanApplication(s.db.QueryRow(ctx,
		`select `+applicationColumns+` from applications where slug = $1`, normalizeSlug(slug)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrApplicationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find application by slug: %w", err)
	}
	return application, nil
}

func (s *Store) ListApplications(ctx context.Context) ([]domain.Application, error) {
	rows, err := s.db.Query(ctx, `select `+applicationColumns+` from applications order by created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()
	applications := make([]domain.Application, 0)
	for rows.Next() {
		application, err := scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}
		applications = append(applications, *application)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	return applications, nil
}

// FindDefaultApplication resolves the default tenant application. It is used
// by legacy flows and the admin console until per-application dashboards land.
func (s *Store) FindDefaultApplication(ctx context.Context) (*domain.Application, error) {
	return s.FindApplicationBySlug(ctx, "starloader")
}

func (s *Store) CreateOrganization(ctx context.Context, name string) (*domain.Organization, error) {
	row := s.db.QueryRow(ctx, `
		insert into organizations (name, slug)
		values ($1, $2)
		returning `+organizationColumns, strings.TrimSpace(name), normalizeSlug(name))
	organization, err := scanOrganization(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "organizations_slug_unique" {
			return nil, domain.ErrOrganizationExists
		}
		return nil, fmt.Errorf("create organization: %w", err)
	}
	return organization, nil
}

// ListOrganizations returns all platform organizations for administration.
func (s *Store) ListOrganizations(ctx context.Context) ([]domain.Organization, error) {
	rows, err := s.db.Query(ctx, `select `+organizationColumns+` from organizations order by created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()
	items := make([]domain.Organization, 0)
	for rows.Next() {
		organization, scanErr := scanOrganization(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan organization: %w", scanErr)
		}
		items = append(items, *organization)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	return items, nil
}

func scanApplication(row pgx.Row) (*domain.Application, error) {
	var application domain.Application
	err := row.Scan(
		&application.ID, &application.OrganizationID, &application.Name, &application.Slug,
		&application.Status, &application.EnvironmentMode, &application.CreatedAt, &application.UpdatedAt,
	)
	return &application, err
}

func scanOrganization(row pgx.Row) (*domain.Organization, error) {
	var organization domain.Organization
	err := row.Scan(
		&organization.ID, &organization.Name, &organization.Slug, &organization.Status,
		&organization.CreatedAt, &organization.UpdatedAt,
	)
	return &organization, err
}

func normalizeSlug(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}
