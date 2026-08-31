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

func (s *Store) CreateApplicationWithSigningKey(
	ctx context.Context,
	input domain.NewApplication,
	keyFactory func(string) (domain.NewApplicationSigningKey, error),
) (*domain.Application, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin application provisioning: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	application, err := scanApplication(tx.QueryRow(ctx, `
		insert into applications (organization_id, name, slug)
		values ($1, $2, $3)
		returning `+applicationColumns,
		input.OrganizationID, strings.TrimSpace(input.Name), normalizeSlug(input.Slug)))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "applications_slug_unique" {
			return nil, domain.ErrApplicationExists
		}
		return nil, fmt.Errorf("create application for provisioning: %w", err)
	}

	key, err := keyFactory(application.ID)
	if err != nil {
		return nil, fmt.Errorf("create application signing key: %w", err)
	}
	if key.ApplicationID != application.ID {
		return nil, errors.New("application signing key does not match created application")
	}
	if err := insertApplicationSigningKey(ctx, tx, key); err != nil {
		return nil, fmt.Errorf("insert application signing key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit application provisioning: %w", err)
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

// UpdateApplication updates editable application metadata without changing its
// lifecycle state. Lifecycle transitions use TransitionApplication so unsafe
// disable operations cannot be hidden inside a general update.
func (s *Store) UpdateApplication(ctx context.Context, applicationID string, input domain.UpdateApplication) (*domain.Application, error) {
	setClauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, domain.ErrInvalidApplicationUpdate
		}
		args = append(args, name)
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if input.Slug != nil {
		slug := normalizeSlug(*input.Slug)
		if slug == "" {
			return nil, domain.ErrInvalidApplicationUpdate
		}
		args = append(args, slug)
		setClauses = append(setClauses, fmt.Sprintf("slug = $%d", len(args)))
	}
	if len(setClauses) == 0 {
		return s.FindApplicationByID(ctx, applicationID)
	}
	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, applicationID)
	application, err := scanApplication(s.db.QueryRow(ctx, `update applications set `+strings.Join(setClauses, ", ")+` where id = $`+fmt.Sprint(len(args))+`::uuid returning `+applicationColumns, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrApplicationNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "applications_slug_unique" {
		return nil, domain.ErrApplicationExists
	}
	if err != nil {
		return nil, fmt.Errorf("update application: %w", err)
	}
	return application, nil
}

// TransitionApplication changes the operational state of an application. A
// disable is rejected while active application-scoped resources exist, so the
// transition cannot silently strand active users, licenses, or credentials.
func (s *Store) TransitionApplication(ctx context.Context, applicationID string, status domain.ApplicationStatus) (*domain.Application, error) {
	if err := domain.ValidateApplicationTransition(status); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin application transition: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	application, err := scanApplication(tx.QueryRow(ctx, `select `+applicationColumns+` from applications where id = $1::uuid for update`, applicationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrApplicationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock application for transition: %w", err)
	}
	if status == domain.ApplicationStatusDisabled {
		var inUse bool
		if err := tx.QueryRow(ctx, `
			select exists (
				select 1 from users where application_id = $1::uuid and status = 'active'
				union all select 1 from licenses where application_id = $1::uuid and status = 'active'
				union all select 1 from devices where application_id = $1::uuid and status = 'active'
				union all select 1 from auth_sessions where application_id = $1::uuid and status in ('pending', 'verified')
				union all select 1 from refresh_sessions where application_id = $1::uuid and status = 'active'
				union all select 1 from application_credentials where application_id = $1::uuid and status = 'active'
				union all select 1 from webhooks where application_id = $1::uuid and status = 'active'
				union all select 1 from products where application_id = $1::uuid and status = 'active'
				union all select 1 from plans join products on products.id = plans.product_id
					where products.application_id = $1::uuid and plans.status = 'active'
			)`, applicationID).Scan(&inUse); err != nil {
			return nil, fmt.Errorf("check application dependencies: %w", err)
		}
		if inUse {
			return nil, domain.ErrApplicationInUse
		}
	}
	if application.Status == status {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit unchanged application transition: %w", err)
		}
		return application, nil
	}
	application, err = scanApplication(tx.QueryRow(ctx, `update applications set status = $2, updated_at = now() where id = $1::uuid returning `+applicationColumns, applicationID, status))
	if err != nil {
		return nil, fmt.Errorf("transition application: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit application transition: %w", err)
	}
	return application, nil
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
