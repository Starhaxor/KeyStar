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

// ResolveProductPlan finds or creates an active product identified by its
// display name within one application. Existing inactive or archived catalog
// records are historical and must not be silently reactivated for issuance.
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
		on conflict (application_id, slug) do nothing
		returning id::text`, applicationID, name, slug).Scan(&productID)
	if errors.Is(err, pgx.ErrNoRows) {
		var status domain.CatalogStatus
		err = s.db.QueryRow(ctx, `select id::text, status from products where application_id = $1::uuid and slug = $2`, applicationID, slug).Scan(&productID, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", domain.ErrProductNotFound
		}
		if err != nil {
			return "", "", fmt.Errorf("find resolved product: %w", err)
		}
		if status != domain.CatalogStatusActive {
			return "", "", domain.ErrCatalogRecordInactive
		}
	} else if err != nil {
		return "", "", fmt.Errorf("resolve product: %w", err)
	}
	planID, err := s.findOrCreateDefaultPlan(ctx, productID)
	if err != nil {
		return "", "", err
	}
	return productID, planID, nil
}

// findOrCreateDefaultPlan returns the active 'default' plan of a product,
// creating it only for a newly active catalog product.
func (s *Store) findOrCreateDefaultPlan(ctx context.Context, productID string) (string, error) {
	var planID string
	err := s.db.QueryRow(ctx, `
		insert into plans (product_id, name, code, level, max_devices)
		values ($1, 'Default', 'default', 1, 1)
		on conflict (product_id, code) do nothing
		returning id::text`, productID).Scan(&planID)
	if errors.Is(err, pgx.ErrNoRows) {
		// The default plan already exists; archived and inactive plans remain
		// historical records rather than being recreated.
		var status domain.CatalogStatus
		err = s.db.QueryRow(ctx, `select id::text, status from plans where product_id = $1 and code = 'default'`, productID).Scan(&planID, &status)
		if err == nil && status != domain.CatalogStatusActive {
			return "", domain.ErrCatalogRecordInactive
		}
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
		on conflict (application_id, slug) do nothing
		returning `+productColumns, applicationID, name, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, findErr := s.FindProductBySlug(ctx, applicationID, slug)
		if findErr != nil {
			return nil, findErr
		}
		if existing.Status != domain.CatalogStatusActive {
			return nil, domain.ErrCatalogRecordInactive
		}
		return existing, nil
	}
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
		select id, $2, $3, $4, $5
		from products
		where id = $1::uuid and status = 'active'
		for key share
		returning `+planColumns,
		input.ProductID, input.Name, input.Code, input.Level, input.MaxDevices))
	if errors.Is(err, pgx.ErrNoRows) {
		var status domain.CatalogStatus
		findErr := s.db.QueryRow(ctx, `select status from products where id = $1::uuid`, input.ProductID).Scan(&status)
		if errors.Is(findErr, pgx.ErrNoRows) {
			return nil, domain.ErrProductNotFound
		}
		if findErr != nil {
			return nil, fmt.Errorf("find product for plan: %w", findErr)
		}
		return nil, domain.ErrCatalogRecordInactive
	}
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

// UpdateProduct updates editable fields of a product scoped to its owning
// application. Archived products remain read-only historical records.
func (s *Store) UpdateProduct(ctx context.Context, applicationID, productID string, input domain.UpdateProduct) (*domain.Product, error) {
	setClauses, args, err := productUpdateFields(input)
	if err != nil {
		return nil, err
	}
	if len(setClauses) == 0 {
		return s.FindProductByID(ctx, applicationID, productID)
	}
	if input.Status != nil && *input.Status == domain.CatalogStatusInactive {
		return s.updateProductToInactive(ctx, applicationID, productID, setClauses, args)
	}
	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, productID, applicationID)
	product, err := scanProduct(s.db.QueryRow(ctx, `update products set `+strings.Join(setClauses, ", ")+` where id = $`+fmt.Sprint(len(args)-1)+`::uuid and application_id = $`+fmt.Sprint(len(args))+`::uuid and status <> 'archived' returning `+productColumns, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, findErr := s.FindProductByID(ctx, applicationID, productID)
		if findErr != nil {
			return nil, findErr
		}
		if existing.Status == domain.CatalogStatusArchived {
			return nil, domain.ErrCatalogRecordInactive
		}
	}
	if err != nil {
		return nil, fmt.Errorf("update product: %w", err)
	}
	return product, nil
}

func productUpdateFields(input domain.UpdateProduct) ([]string, []any, error) {
	setClauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, nil, domain.ErrProductInvalidName
		}
		args = append(args, name)
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if input.Slug != nil {
		slug := domain.ProductSlug(*input.Slug)
		if slug == "" {
			return nil, nil, domain.ErrProductInvalidName
		}
		args = append(args, slug)
		setClauses = append(setClauses, fmt.Sprintf("slug = $%d", len(args)))
	}
	if input.Status != nil {
		if err := domain.ValidateCatalogUpdateStatus(*input.Status); err != nil {
			return nil, nil, err
		}
		args = append(args, *input.Status)
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", len(args)))
	}
	return setClauses, args, nil
}

func (s *Store) updateProductToInactive(ctx context.Context, applicationID, productID string, setClauses []string, args []any) (*domain.Product, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin product inactivation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	product, err := scanProduct(tx.QueryRow(ctx, `select `+productColumns+` from products where id = $1::uuid and application_id = $2::uuid for update`, productID, applicationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProductNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock product for inactivation: %w", err)
	}
	if product.Status == domain.CatalogStatusArchived {
		return nil, domain.ErrCatalogRecordInactive
	}
	if product.Status == domain.CatalogStatusActive {
		var inUse bool
		if err := tx.QueryRow(ctx, `select exists (
			select 1 from licenses where product_id = $1::uuid and status = 'active'
			union all select 1 from plans where product_id = $1::uuid and status = 'active'
		)`, productID).Scan(&inUse); err != nil {
			return nil, fmt.Errorf("check product inactivation dependencies: %w", err)
		}
		if inUse {
			return nil, domain.ErrCatalogRecordInUse
		}
	}
	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, productID, applicationID)
	product, err = scanProduct(tx.QueryRow(ctx, `update products set `+strings.Join(setClauses, ", ")+` where id = $`+fmt.Sprint(len(args)-1)+`::uuid and application_id = $`+fmt.Sprint(len(args))+`::uuid returning `+productColumns, args...))
	if err != nil {
		return nil, fmt.Errorf("inactivate product: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit product inactivation: %w", err)
	}
	return product, nil
}

// ArchiveProduct retains a historical product row and rejects the archive
// while active licenses or plans still depend on it.
func (s *Store) ArchiveProduct(ctx context.Context, applicationID, productID string) (*domain.Product, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin product archive: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	product, err := scanProduct(tx.QueryRow(ctx, `select `+productColumns+` from products where id = $1::uuid and application_id = $2::uuid for update`, productID, applicationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProductNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock product for archive: %w", err)
	}
	if product.Status == domain.CatalogStatusArchived {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit unchanged product archive: %w", err)
		}
		return product, nil
	}
	var inUse bool
	if err := tx.QueryRow(ctx, `select exists (
		select 1 from licenses where product_id = $1::uuid and status = 'active'
		union all select 1 from plans where product_id = $1::uuid and status = 'active'
	)`, productID).Scan(&inUse); err != nil {
		return nil, fmt.Errorf("check product archive dependencies: %w", err)
	}
	if inUse {
		return nil, domain.ErrCatalogRecordInUse
	}
	product, err = scanProduct(tx.QueryRow(ctx, `update products set status = 'archived', updated_at = now() where id = $1::uuid and application_id = $2::uuid returning `+productColumns, productID, applicationID))
	if err != nil {
		return nil, fmt.Errorf("archive product: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit product archive: %w", err)
	}
	return product, nil
}

// UpdatePlan updates a plan only through the product's application boundary.
func (s *Store) UpdatePlan(ctx context.Context, applicationID, productID, planID string, input domain.UpdatePlan) (*domain.Plan, error) {
	setClauses, args, err := planUpdateFields(input)
	if err != nil {
		return nil, err
	}
	if len(setClauses) == 0 {
		return s.findPlanByIDForProduct(ctx, applicationID, productID, planID)
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin plan update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var productStatus, planStatus domain.CatalogStatus
	err = tx.QueryRow(ctx, `select p.status, pl.status
		from plans pl join products p on p.id = pl.product_id
		where pl.id = $1::uuid and pl.product_id = $2::uuid and p.application_id = $3::uuid
		for update of p, pl`, planID, productID, applicationID).Scan(&productStatus, &planStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.findPlanByIDForProduct(ctx, applicationID, productID, planID)
	}
	if err != nil {
		return nil, fmt.Errorf("lock plan for update: %w", err)
	}
	if productStatus != domain.CatalogStatusActive || planStatus == domain.CatalogStatusArchived {
		return nil, domain.ErrCatalogRecordInactive
	}
	if input.Status != nil && *input.Status == domain.CatalogStatusInactive && planStatus == domain.CatalogStatusActive {
		var inUse bool
		if err := tx.QueryRow(ctx, `select exists(select 1 from licenses where plan_id = $1::uuid and status = 'active')`, planID).Scan(&inUse); err != nil {
			return nil, fmt.Errorf("check plan inactivation dependencies: %w", err)
		}
		if inUse {
			return nil, domain.ErrCatalogRecordInUse
		}
	}
	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, planID, productID)
	plan, err := scanPlan(tx.QueryRow(ctx, `update plans set `+strings.Join(setClauses, ", ")+`
		where id = $`+fmt.Sprint(len(args)-1)+`::uuid and product_id = $`+fmt.Sprint(len(args))+`::uuid
		returning `+planColumns, args...))
	if err != nil {
		return nil, fmt.Errorf("update plan: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit plan update: %w", err)
	}
	return plan, nil
}

func planUpdateFields(input domain.UpdatePlan) ([]string, []any, error) {
	setClauses := make([]string, 0, 6)
	args := make([]any, 0, 6)
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, nil, domain.ErrProductInvalidName
		}
		args = append(args, name)
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if input.Code != nil {
		code := strings.ToLower(strings.TrimSpace(*input.Code))
		if code == "" {
			return nil, nil, domain.ErrProductInvalidName
		}
		args = append(args, code)
		setClauses = append(setClauses, fmt.Sprintf("code = $%d", len(args)))
	}
	if input.Level != nil {
		args = append(args, *input.Level)
		setClauses = append(setClauses, fmt.Sprintf("level = $%d", len(args)))
	}
	if input.MaxDevices != nil {
		if *input.MaxDevices < 1 {
			return nil, nil, domain.ErrProductInvalidName
		}
		args = append(args, *input.MaxDevices)
		setClauses = append(setClauses, fmt.Sprintf("max_devices = $%d", len(args)))
	}
	if input.Status != nil {
		if err := domain.ValidateCatalogUpdateStatus(*input.Status); err != nil {
			return nil, nil, err
		}
		args = append(args, *input.Status)
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", len(args)))
	}
	return setClauses, args, nil
}

// ArchivePlan retains the plan for existing licenses and rejects the archive
// while an active license still references it.
func (s *Store) ArchivePlan(ctx context.Context, applicationID, productID, planID string) (*domain.Plan, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin plan archive: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var productStatus domain.CatalogStatus
	err = tx.QueryRow(ctx, `select p.status
		from plans pl join products p on p.id = pl.product_id
		where pl.id = $1::uuid and pl.product_id = $2::uuid and p.application_id = $3::uuid for update of p, pl`, planID, productID, applicationID).Scan(&productStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock plan for archive: %w", err)
	}
	if productStatus != domain.CatalogStatusActive {
		return nil, domain.ErrCatalogRecordInactive
	}
	plan, err := scanPlan(tx.QueryRow(ctx, `select `+planColumns+` from plans where id = $1::uuid and product_id = $2::uuid`, planID, productID))
	if err != nil {
		return nil, fmt.Errorf("read locked plan for archive: %w", err)
	}
	if plan.Status == domain.CatalogStatusArchived {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit unchanged plan archive: %w", err)
		}
		return plan, nil
	}
	var inUse bool
	if err := tx.QueryRow(ctx, `select exists(select 1 from licenses where plan_id = $1::uuid and status = 'active')`, planID).Scan(&inUse); err != nil {
		return nil, fmt.Errorf("check plan archive dependencies: %w", err)
	}
	if inUse {
		return nil, domain.ErrCatalogRecordInUse
	}
	plan, err = scanPlan(tx.QueryRow(ctx, `update plans set status = 'archived', updated_at = now() where id = $1::uuid and product_id = $2::uuid returning `+planColumns, planID, productID))
	if err != nil {
		return nil, fmt.Errorf("archive plan: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit plan archive: %w", err)
	}
	return plan, nil
}

func (s *Store) findPlanByIDForProduct(ctx context.Context, applicationID, productID, planID string) (*domain.Plan, error) {
	plan, err := scanPlan(s.db.QueryRow(ctx, `select pl.id::text, pl.product_id::text, pl.name, pl.code, pl.level, pl.max_devices, pl.default_duration_seconds, pl.status, pl.created_at, pl.updated_at
		from plans pl join products p on p.id = pl.product_id
		where pl.id = $1::uuid and pl.product_id = $2::uuid and p.application_id = $3::uuid`, planID, productID, applicationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find plan by id: %w", err)
	}
	return plan, nil
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
