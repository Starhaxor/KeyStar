package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/starloader/backend/internal/domain"
)

const productColumns = `id::text, application_id::text, name, slug, status, created_at, updated_at`

const planColumns = `id::text, product_id::text, name, code, level, max_devices, default_duration_seconds, status, created_at, updated_at`

// ResolveProductPlan finds or creates the product identified by its display
// name within one application and returns its (product_id, plan_id) pair. The
// default plan (code 'default') is created alongside a new product so every
// license created through the legacy product-name API is bound to a plan.
func (s *Store) ResolveProductPlan(ctx context.Context, applicationID, name string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", domain.ErrProductInvalidName
	}
	slug := domain.ProductSlug(name)
	if slug == "" {
		return "", "", domain.ErrProductInvalidName
	}
	var productID string
	err := s.db.QueryRow(ctx, `
		insert into products (application_id, name, slug)
		values ($1, $2, $3)
		on conflict (application_id, slug) do update set slug = excluded.slug
		returning id::text`, applicationID, name, slug).Scan(&productID)
	if err != nil {
		return "", "", fmt.Errorf("resolve product: %w", err)
	}
	planID, err := s.findOrCreateDefaultPlan(ctx, productID)
	if err != nil {
		return "", "", err
	}
	return productID, planID, nil
}

// findOrCreateDefaultPlan returns the 'default' plan of a product, creating it
// when the product has no plans yet.
func (s *Store) findOrCreateDefaultPlan(ctx context.Context, productID string) (string, error) {
	var planID string
	err := s.db.QueryRow(ctx, `
		insert into plans (product_id, name, code, level, max_devices)
		values ($1, 'Default', 'default', 1, 1)
		on conflict (product_id, code) do nothing
		returning id::text`, productID).Scan(&planID)
	if errors.Is(err, pgx.ErrNoRows) {
		// The default plan already exists; read it back.
		err = s.db.QueryRow(ctx, `select id::text from plans where product_id = $1 and code = 'default'`, productID).Scan(&planID)
	}
	if err != nil {
		return "", fmt.Errorf("resolve default plan: %w", err)
	}
	return planID, nil
}

func (s *Store) CreateProduct(ctx context.Context, applicationID string, input domain.NewProduct) (*domain.Product, error) {
	name := strings.TrimSpace(input.Name)
	slug := input.Slug
	if slug == "" {
		slug = domain.ProductSlug(name)
	}
	if name == "" || slug == "" {
		return nil, domain.ErrProductInvalidName
	}
	product, err := scanProduct(s.db.QueryRow(ctx, `
		insert into products (application_id, name, slug)
		values ($1, $2, $3)
		on conflict (application_id, slug) do update set name = excluded.name
		returning `+productColumns, applicationID, name, slug))
	if err != nil {
		return nil, fmt.Errorf("create product: %w", err)
	}
	return product, nil
}

func (s *Store) FindProductByID(ctx context.Context, applicationID, productID string) (*domain.Product, error) {
	product, err := scanProduct(s.db.QueryRow(ctx,
		`select `+productColumns+` from products where id = $1::uuid and application_id = $2::uuid`, productID, applicationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProductNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find product by id: %w", err)
	}
	return product, nil
}

func (s *Store) FindProductBySlug(ctx context.Context, applicationID, slug string) (*domain.Product, error) {
	product, err := scanProduct(s.db.QueryRow(ctx,
		`select `+productColumns+` from products where slug = $2 and application_id = $1::uuid`, applicationID, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProductNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find product by slug: %w", err)
	}
	return product, nil
}

func (s *Store) ListProducts(ctx context.Context, applicationID string) ([]domain.Product, error) {
	rows, err := s.db.Query(ctx,
		`select `+productColumns+` from products where application_id = $1::uuid order by created_at, id`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()
	products := make([]domain.Product, 0)
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, *product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	return products, nil
}

func (s *Store) CreatePlan(ctx context.Context, input domain.NewPlan) (*domain.Plan, error) {
	plan, err := scanPlan(s.db.QueryRow(ctx, `
		insert into plans (product_id, name, code, level, max_devices)
		values ($1, $2, $3, $4, $5)
		returning `+planColumns,
		input.ProductID, input.Name, input.Code, input.Level, input.MaxDevices))
	if err != nil {
		return nil, fmt.Errorf("create plan: %w", err)
	}
	return plan, nil
}

func (s *Store) FindPlanByID(ctx context.Context, planID string) (*domain.Plan, error) {
	plan, err := scanPlan(s.db.QueryRow(ctx, `select `+planColumns+` from plans where id = $1::uuid`, planID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find plan by id: %w", err)
	}
	return plan, nil
}

func (s *Store) ListPlans(ctx context.Context, productID string) ([]domain.Plan, error) {
	rows, err := s.db.Query(ctx,
		`select `+planColumns+` from plans where product_id = $1::uuid order by level, created_at, id`, productID)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()
	plans := make([]domain.Plan, 0)
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		plans = append(plans, *plan)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	return plans, nil
}

func scanProduct(row pgx.Row) (*domain.Product, error) {
	var product domain.Product
	err := row.Scan(
		&product.ID, &product.ApplicationID, &product.Name, &product.Slug, &product.Status,
		&product.CreatedAt, &product.UpdatedAt,
	)
	return &product, err
}

func scanPlan(row pgx.Row) (*domain.Plan, error) {
	var plan domain.Plan
	var defaultDuration *int64
	err := row.Scan(
		&plan.ID, &plan.ProductID, &plan.Name, &plan.Code, &plan.Level, &plan.MaxDevices,
		&defaultDuration, &plan.Status, &plan.CreatedAt, &plan.UpdatedAt,
	)
	plan.DefaultDurationSeconds = defaultDuration
	return &plan, err
}
