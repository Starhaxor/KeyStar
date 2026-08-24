# Console lifecycle and onboarding implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let operators safely manage application/catalog lifecycles and take a new application from organization selection through a verified test license.

**Architecture:** Keep mutations in `/v1/admin`, with the existing admin cookie, CSRF, RBAC, audit trail, and `X-KeyStar-App` context. Persist lifecycle state in PostgreSQL; use named archive actions for destructive transitions. The console reads real server state to derive resumable onboarding progress instead of storing client-only wizard state.

**Tech Stack:** Go 1.24, PostgreSQL 15 migrations/pgx, Next.js 16, React 19, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-08-24-platform-completion-design.md`

## Global constraints

- Go 1.24+, PostgreSQL 15+, Node.js 20+, Next.js 16, React 19, C++17 baseline.
- New behavior is test-first; every defect receives focused regression coverage.
- Use KeyStar terminology in visible copy; no raw server error payloads or secrets in UI/logs.
- Use the existing `AccessibleDialog`, `Field`, safe client-error messages, and responsive table primitives.
- Every mutation keeps existing app isolation, CSRF, RBAC, and audit semantics.

---

### Task 1: Add lifecycle persistence and safe transition services

**Files:**
- Create: `backend/migrations/000016_console_lifecycle.up.sql`
- Create: `backend/migrations/000016_console_lifecycle.down.sql`
- Modify: `backend/internal/store/migrations.go`
- Modify: `backend/internal/domain/application.go`
- Modify: `backend/internal/domain/product.go`
- Modify: `backend/internal/store/applications.go`
- Modify: `backend/internal/store/products.go`
- Test: `backend/internal/store/*_test.go`

**Consumes:** existing `applications.status`, `organizations.status`, `products`, `plans`, and license foreign keys.

**Produces:** `UpdateApplication`, `TransitionApplication`, `UpdateProduct`, `ArchiveProduct`, `UpdatePlan`, and `ArchivePlan` store operations that return typed conflict errors for unsafe transitions.

- [ ] **Step 1: Write failing store tests**

```go
func TestArchiveProductRejectsActiveLicenses(t *testing.T) {
  err := repository.ArchiveProduct(ctx, applicationID, productID)
  requireErrorCode(t, err, "CATALOG_RECORD_IN_USE")
}

func TestArchivedPlanCannotBeUsedForNewLicense(t *testing.T) {
  _, err := repository.ResolveProductPlan(ctx, applicationID, "Pro")
  requireErrorCode(t, err, "CATALOG_RECORD_ARCHIVED")
}
```

- [ ] **Step 2: Run tests and verify failure**

Run: `cd backend; go test ./internal/store -run 'Archive|Archived' -count=1`

Expected: FAIL because lifecycle mutation and issuance guards do not exist.

- [ ] **Step 3: Add migration and minimal domain/store behavior**

Add catalog/application state validation and indexes. Preserve records on archive; never physically delete catalog data. Transition application only to `active`, `maintenance`, or `disabled`; reject archive/disable when active dependent resources would become inaccessible. Make resolution and creation queries require active catalog records.

- [ ] **Step 4: Run focused and backend tests**

Run: `cd backend; go test ./internal/store ./internal/service -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add backend/migrations backend/internal/domain backend/internal/store
git commit -m "feat(core): add safe application and catalog lifecycle"
```

### Task 2: Expose audited lifecycle admin APIs

**Files:**
- Modify: `backend/internal/httpapi/adminapi/admin_applications.go`
- Modify: `backend/internal/httpapi/adminapi/admin_products.go`
- Modify: `backend/internal/httpapi/adminapi/admin.go`
- Modify: `backend/internal/httpapi/adminapi/*_test.go`

**Consumes:** Task 1 store methods and `applications.write` / `catalog.write` permissions.

**Produces:** PATCH edit endpoints and explicit archive/transition action endpoints with consistent conflict responses and audit events.

- [ ] **Step 1: Write failing handler tests**

```go
func TestAdminApplicationTransitionAuditsAndScopes(t *testing.T) {
  response := requestAdmin(t, http.MethodPost, "/v1/admin/applications/"+id+"/transition", `{"status":"maintenance"}`)
  requireStatus(t, response, http.StatusOK)
  requireAuditAction(t, "APPLICATION_STATUS_CHANGED")
}

func TestAdminProductArchiveReturnsConflictWhenInUse(t *testing.T) {
  response := requestAdmin(t, http.MethodPost, "/v1/admin/products/"+id+"/archive", `{}`)
  requireStatus(t, response, http.StatusConflict)
}
```

- [ ] **Step 2: Run tests and verify failure**

Run: `cd backend; go test ./internal/httpapi/adminapi -run 'Transition|Archive' -count=1`

Expected: FAIL because routes are absent.

- [ ] **Step 3: Implement endpoints**

Use `PATCH /v1/admin/applications/{id}`, `POST /v1/admin/applications/{id}/transition`, `PATCH /v1/admin/products/{id}`, `POST /v1/admin/products/{id}/archive`, `PATCH /v1/admin/products/{id}/plans/{planID}`, and `POST /v1/admin/products/{id}/plans/{planID}/archive`. Return typed safe errors; audit each successful action with entity IDs and before/after state.

- [ ] **Step 4: Run API suite**

Run: `cd backend; go test ./internal/httpapi/adminapi ./internal/service -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add backend/internal/httpapi/adminapi
git commit -m "feat(api): expose audited lifecycle actions"
```

### Task 3: Add lifecycle controls to Applications and Products

**Files:**
- Modify: `admin/src/lib/api.ts`
- Modify: `admin/src/lib/types.ts`
- Modify: `admin/src/app/(admin)/applications/*`
- Modify: `admin/src/app/(admin)/products/page.tsx`
- Create: `admin/src/app/(admin)/applications/ApplicationsLifecycle.test.tsx`
- Create: `admin/src/app/(admin)/products/ProductsLifecycle.test.tsx`

**Consumes:** Task 2 API routes, `ConfirmModal`, `AccessibleDialog`, `Field`.

**Produces:** accessible edit/status/archive UI with safe copy and refreshed active application context.

- [ ] **Step 1: Write failing UI tests**

```tsx
expect(screen.getByRole("button", { name: "Archive product" })).toBeEnabled();
await user.click(screen.getByRole("button", { name: "Archive product" }));
expect(screen.getByRole("dialog", { name: /archive product/i })).toBeVisible();
```

- [ ] **Step 2: Run focused tests and verify failure**

Run: `cd admin; npm test -- ApplicationsLifecycle ProductsLifecycle`

Expected: FAIL because controls and API methods do not exist.

- [ ] **Step 3: Implement console controls**

Add edit controls and status badges on applications. Require confirmation for maintenance/disabled/archive actions; show server conflict as safe action-specific copy. Add product/plan editing and archive actions, with archived records filtered separately but retained as historical items.

- [ ] **Step 4: Run frontend quality checks**

Run: `cd admin; npm test; npm run lint; npm run build`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add admin/src
git commit -m "feat(admin): manage application and catalog lifecycle"
```

### Task 4: Build resumable application onboarding

**Files:**
- Create: `admin/src/app/(admin)/onboarding/page.tsx`
- Create: `admin/src/components/onboarding/OnboardingWizard.tsx`
- Create: `admin/src/components/onboarding/onboardingState.ts`
- Create: `admin/src/components/onboarding/OnboardingWizard.test.tsx`
- Modify: `admin/src/lib/api.ts`
- Modify: `admin/src/layout/sidebarNavigation.ts`
- Modify: `backend/internal/httpapi/adminapi/admin_console.go`
- Test: `backend/internal/httpapi/adminapi/admin_console_test.go`

**Consumes:** organizations/applications, credentials, products/plans and license APIs; active application selection.

**Produces:** a server-derived progress response and wizard which resumes after refresh.

- [ ] **Step 1: Write failing progress tests**

```tsx
expect(deriveOnboardingStep({ application: null, credentialCount: 0, productCount: 0, licenseCount: 0 })).toBe("application");
expect(deriveOnboardingStep({ application: app, credentialCount: 1, productCount: 1, licenseCount: 1 })).toBe("complete");
```

- [ ] **Step 2: Run tests and verify failure**

Run: `cd admin; npm test -- OnboardingWizard`

Expected: FAIL because the wizard/progress state does not exist.

- [ ] **Step 3: Add minimal server progress and wizard**

Provide one admin progress endpoint based only on persisted resources. Wizard steps: organization/application, environment choice, publishable credential, product/plan, test license. Render an ephemeral credential secret once only; never store it in browser storage. Refresh/re-entry derives the next incomplete step from API data.

- [ ] **Step 4: Run focused + full UI checks**

Run: `cd admin; npm test; npm run lint; npm run build`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add admin/src backend/internal/httpapi/adminapi
git commit -m "feat(admin): add resumable application onboarding"
```

### Task 5: Add integration and browser regression coverage

**Files:**
- Modify: `backend/tests/integration/server_api_test.go`
- Create: `admin/e2e/lifecycle-onboarding.spec.ts`
- Modify: `admin/e2e/fixtures.ts`
- Modify: `.github/workflows/verify.yml`
- Modify: `README.md`

**Consumes:** Tasks 1–4 and existing disposable test database contract.

**Produces:** CI coverage proving cross-tenant safety, safe archive conflict, resumable onboarding, and test-license issuance.

- [ ] **Step 1: Write the failing integration/browser tests**

```ts
test("resumes onboarding and issues a test license", async ({ page, fixture }) => {
  await page.goto("/onboarding");
  await expect(page.getByText("Create a credential")).toBeVisible();
  // Complete each persisted step, reload, then assert the next incomplete step.
});
```

- [ ] **Step 2: Run targeted tests and verify failure**

Run: `cd admin; npx playwright test e2e/lifecycle-onboarding.spec.ts`

Expected: FAIL before Tasks 1–4 are implemented.

- [ ] **Step 3: Wire fixtures and CI**

Use only `TEST_DATABASE_URL` with the exact `keystar_test` guard. Add the new spec to the existing `admin-e2e` CI job; document the lifecycle/onboarding verification command.

- [ ] **Step 4: Run Release B checks**

Run: `cd backend; go test ./...; cd ../admin; npm test; npm run lint; npm run build; npm run e2e`

Expected: PASS in a Docker-capable environment; record any unavailable external prerequisite without claiming success.

- [ ] **Step 5: Commit**

```powershell
git add backend/tests admin/e2e .github README.md
git commit -m "test: cover lifecycle and onboarding journeys"
```

## Plan self-review

- Spec coverage: Tasks 1–3 implement safe lifecycle for applications and catalog; Task 4 implements a server-derived resumable onboarding path; Task 5 verifies the end-to-end safety and journey.
- Placeholder scan: no deferred implementation markers or unspecified handlers remain.
- Type consistency: Task 1 store operations underpin Task 2 routes; Task 2 routes are consumed by Task 3 and progress data from Task 4; Task 5 exercises those public contracts.
