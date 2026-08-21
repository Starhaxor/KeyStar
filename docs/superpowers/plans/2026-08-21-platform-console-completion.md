# Platform Console Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a multi-application KeyStar administrator console with all persisted platform capabilities, reliable operations screens and a clean quality gate.

**Architecture:** Extend the Go admin API through resource-specific routes and retain it as the authorization authority. Add a selected-application context to the Next.js client, then build management pages on the existing table, modal and confirmation components.

**Tech Stack:** Go, net/http, PostgreSQL/pgx, Next.js 16, React 19, TypeScript, Tailwind CSS, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-08-21-platform-console-design.md`

## Global Constraints

- Preserve default-application behavior when no application header is sent.
- Validate authorization and application boundaries in Go for every request.
- Return credential and webhook secrets only at creation time.
- Write and observe a failing test before every production behavior change.
- Finish with passing Go tests, frontend tests, Playwright tests, lint and build.

---

### Task 1: Application context and RBAC

**Files:**
- Modify: `backend/internal/domain/admin.go`
- Modify: `backend/internal/httpapi/adminapi/admin.go`
- Modify: `backend/internal/httpapi/adminapi/admin_test.go`
- Create: `admin/src/context/ApplicationContext.tsx`
- Modify: `admin/src/lib/api.ts`
- Modify: `admin/src/lib/permissions.ts`
- Modify: `admin/src/lib/types.ts`
- Modify: `admin/src/app/(admin)/layout.tsx`

**Interfaces:**
- Produces `AdminApplicationID(*http.Request) string`.
- Produces `useApplicationContext(): { applicationId: string | null; selectApplication(id: string): void }`.
- Adds `applications.*`, `catalog.*`, `webhooks.*` and missing `credentials.*` permissions.

- [ ] **Step 1: Write a failing Go test**

```go
func TestAdminRequestUsesSelectedApplication(t *testing.T) {
  request := adminRequest(t, http.MethodGet, "/v1/admin/users", "")
  request.Header.Set("X-KeyStar-Application-ID", alternateApplicationID)
  recorder := httptest.NewRecorder()
  router.ServeHTTP(recorder, request)
  if recorder.Code != http.StatusOK || fake.lastApplicationID != alternateApplicationID {
    t.Fatal("selected application was not used")
  }
}
```

- [ ] **Step 2: Verify the red test**

Run: `go test ./internal/httpapi/adminapi -run TestAdminRequestUsesSelectedApplication -count=1`

Expected: FAIL because handlers use the default application.

- [ ] **Step 3: Implement context resolution**

Resolve and validate `X-KeyStar-Application-ID` once in the admin router, place it in request context, update application-scoped routes to call `AdminApplicationID`, and preserve the existing default only for requests without a header. Add the matching domain and UI permissions.

- [ ] **Step 4: Implement the frontend provider**

Persist the selection in a cookie, attach it in the shared API request helper, mount the provider in the admin layout and expose typed application summaries.

- [ ] **Step 5: Verify green**

Run: `go test ./internal/httpapi/adminapi -run TestAdminRequestUsesSelectedApplication -count=1; npm run build`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/domain/admin.go backend/internal/httpapi/adminapi admin/src/context admin/src/lib admin/src/app
git commit -m "feat: add admin application context"
```

### Task 2: Platform administration API

**Files:**
- Create: `backend/internal/httpapi/adminapi/admin_platform.go`
- Create: `backend/internal/httpapi/adminapi/admin_platform_test.go`
- Modify: `backend/internal/httpapi/adminapi/admin.go`
- Modify: `backend/internal/store/applications.go`
- Modify: `backend/internal/store/products.go`
- Modify: `backend/internal/store/webhooks.go`

**Interfaces:**
- Produces admin CRUD for applications, products, plans and webhooks.
- Produces a paginated webhook-delivery list.
- List calls accept bounded page, search, status and date filters.

- [ ] **Step 1: Write failing route tests**

```go
func TestProductCreateUsesSelectedApplication(t *testing.T) {
  recorder := doAdminRequest(t, http.MethodPost, "/v1/admin/products", alternateApplicationID, `{"name":"Pro"}`)
  if recorder.Code != http.StatusOK || fake.createdProduct.ApplicationID != alternateApplicationID {
    t.Fatal("product escaped application boundary")
  }
}

func TestWebhookListDoesNotExposeSecret(t *testing.T) {
  recorder := doAdminRequest(t, http.MethodGet, "/v1/admin/webhooks", defaultApplicationID, "")
  if strings.Contains(recorder.Body.String(), "secret") {
    t.Fatal("webhook secret leaked")
  }
}
```

- [ ] **Step 2: Verify red**

Run: `go test ./internal/httpapi/adminapi -run 'Test(ProductCreateUses|WebhookListDoes)' -count=1`

Expected: FAIL because the routes do not exist.

- [ ] **Step 3: Implement storage and handlers**

Add scoped list/create/update methods; validate application status, product and plan names/codes, webhook HTTPS URLs and event patterns. Dispatch `/applications`, `/products`, `/products/{id}/plans`, `/webhooks`, and `/webhooks/{id}/deliveries`; audit each mutation and map secret-free list values.

- [ ] **Step 4: Verify green**

Run: `go test ./internal/httpapi/adminapi -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/httpapi/adminapi backend/internal/store
git commit -m "feat: add platform administration API"
```

### Task 3: Typed client, credentials and device policy

**Files:**
- Modify: `admin/src/lib/api.ts`
- Modify: `admin/src/lib/types.ts`
- Create: `admin/src/lib/api.test.ts`
- Modify: `admin/src/layout/AppHeader.tsx`
- Modify: `admin/src/layout/AppSidebar.tsx`
- Modify: `admin/package.json`

**Interfaces:**
- Produces typed client methods for applications, catalog, credentials, device policy and webhooks.
- Produces header application selector and permission-gated navigation.

- [ ] **Step 1: Write a failing client test**

```ts
it("sends the selected application header for webhook creation", async () => {
  document.cookie = "keystar_application_id=app-2";
  fetchMock.mockResponseOnce(JSON.stringify({ ok: true, webhook: { id: "w-1" }, secret: "once" }));
  await api.createWebhook({ url: "https://example.test/hook", events: ["license.*"] });
  expect(fetchMock).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({
    headers: expect.objectContaining({ "X-KeyStar-Application-ID": "app-2" }),
  }));
});
```

- [ ] **Step 2: Verify red**

Run: `npm test -- api.test.ts`

Expected: FAIL because the test harness and API method are absent.

- [ ] **Step 3: Implement**

Configure Vitest and jsdom. Add secret-safe create-result types and all resource methods. Add selector loading to the header. Gate Security Events and every new menu item with its read permission; add credentials permissions to the role editor.

- [ ] **Step 4: Verify green**

Run: `npm test -- api.test.ts; npm run build`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add admin/package.json admin/package-lock.json admin/src/lib admin/src/layout
git commit -m "feat: add platform client and navigation"
```

### Task 4: Platform console pages and catalog licensing

**Files:**
- Create: `admin/src/app/(admin)/applications/page.tsx`
- Create: `admin/src/app/(admin)/products/page.tsx`
- Create: `admin/src/app/(admin)/credentials/page.tsx`
- Create: `admin/src/app/(admin)/webhooks/page.tsx`
- Create: `admin/src/components/console/SecretReveal.tsx`
- Create: `admin/src/components/console/DevicePolicyForm.tsx`
- Modify: `admin/src/app/(admin)/devices/page.tsx`
- Modify: `admin/src/components/console/LicenseCreateModal.tsx`
- Create: `admin/src/components/console/PlatformPages.test.tsx`

**Interfaces:**
- Consumes Task 3 methods and application context.
- Produces applications, products/plans, credentials, webhooks and device-policy management flows.

- [ ] **Step 1: Write failing component tests**

```tsx
it("reveals a created credential only until dismissed", async () => {
  render(<CredentialsPage />);
  await userEvent.click(screen.getByRole("button", { name: /create credential/i }));
  await userEvent.click(screen.getByRole("button", { name: /^create$/i }));
  expect(await screen.findByText("ks_sk_test_secret")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: /done/i }));
  expect(screen.queryByText("ks_sk_test_secret")).not.toBeInTheDocument();
});

it("applies selected plan defaults to the license form", async () => {
  render(<LicenseCreateModal open onClose={vi.fn()} onCreated={vi.fn()} />);
  await userEvent.selectOptions(screen.getByLabelText(/plan/i), "pro");
  expect(screen.getByLabelText(/max devices/i)).toHaveValue(3);
});
```

- [ ] **Step 2: Verify red**

Run: `npm test -- PlatformPages.test.tsx`

Expected: FAIL because the pages and selectors are absent.

- [ ] **Step 3: Implement all management pages**

Use existing shared UI components. Create/update/revoke credentials with one-time `SecretReveal`; create/update products and nested plans; create/update/delete webhooks and inspect deliveries; create/select/update applications; edit/reset device policy from Devices. Add product and plan selection to license issuance and retain authorized overrides.

- [ ] **Step 4: Verify green**

Run: `npm test -- PlatformPages.test.tsx; npm run build`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add admin/src/app admin/src/components
git commit -m "feat: add platform management pages"
```

### Task 5: Operational data, exports and access defects

**Files:**
- Modify: `backend/internal/httpapi/adminapi/admin_console.go`
- Modify: `backend/internal/store/console.go`
- Modify: `backend/internal/httpapi/adminapi/admin_console_test.go`
- Modify: `admin/src/lib/api.ts`
- Modify: `admin/src/components/common/ExportCsvButton.tsx`
- Modify: `admin/src/app/(admin)/{licenses,devices,sessions,audit-logs,security-events,profile,page}.tsx`

**Interfaces:**
- Produces server-side filters and all-matching export retrieval.
- Produces `GET /v1/admin/me/activity`, without an audit permission requirement.

- [ ] **Step 1: Write failing handler tests**

```go
func TestAdminLicenseListForwardsFilters(t *testing.T) {
  recorder := doAdminRequest(t, http.MethodGet, "/v1/admin/licenses?search=alice&status=active", defaultApplicationID, "")
  if recorder.Code != http.StatusOK || fake.lastLicenseFilter.Search != "alice" {
    t.Fatal("license filter was not forwarded")
  }
}

func TestAdminSelfActivityRequiresNoAuditPermission(t *testing.T) {
  recorder := doLimitedAdminRequest(t, http.MethodGet, "/v1/admin/me/activity", "")
  if recorder.Code != http.StatusOK { t.Fatalf("got %d", recorder.Code) }
}
```

- [ ] **Step 2: Verify red**

Run: `go test ./internal/httpapi/adminapi -run 'TestAdmin(LicenseListForwards|SelfActivity)' -count=1`

Expected: FAIL because filter support and self activity route are absent.

- [ ] **Step 3: Implement**

Add bounded server filters for licenses, devices, sessions, audit logs and security events; use them from all list pages; reset pagination when filters change. Make CSV obtain every matching page before generating. Replace profile audit-log access with self activity. Add 7/14/30-day dashboard range and manual refresh.

- [ ] **Step 4: Verify green**

Run: `go test ./internal/httpapi/adminapi -count=1; npm test; npm run build`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/httpapi/adminapi backend/internal/store admin/src
git commit -m "feat: improve administration operations"
```

### Task 6: Lint cleanup and browser verification

**Files:**
- Modify: `admin/jsvectormap.d.ts`
- Modify: `admin/src/app/(admin)/profile/page.tsx`
- Modify: `admin/src/components/console/RowActions.tsx`
- Modify: `admin/src/context/SidebarContext.tsx`
- Modify: `admin/src/context/ThemeContext.tsx`
- Modify: `admin/src/layout/AppSidebar.tsx`
- Create: `admin/playwright.config.ts`
- Create: `admin/e2e/admin-console.spec.ts`

**Interfaces:**
- Produces a warning-free lint command and browser smoke suite.

- [ ] **Step 1: Write a failing browser smoke test**

```ts
test("administrator creates a credential in a selected application", async ({ page }) => {
  await page.goto("/signin");
  await page.getByLabel(/email/i).fill(process.env.E2E_ADMIN_EMAIL!);
  await page.getByLabel(/password/i).fill(process.env.E2E_ADMIN_PASSWORD!);
  await page.getByRole("button", { name: /sign in/i }).click();
  await page.getByLabel(/application/i).selectOption(process.env.E2E_APPLICATION_ID!);
  await page.getByRole("link", { name: /api credentials/i }).click();
  await page.getByRole("button", { name: /create credential/i }).click();
  await expect(page.getByText(/copy it now/i)).toBeVisible();
});
```

- [ ] **Step 2: Verify red**

Run: `npx playwright test e2e/admin-console.spec.ts`

Expected: FAIL because the Playwright setup and resource pages are absent.

- [ ] **Step 3: Implement test infrastructure and lint-safe refactors**

Configure a local web server and seeded backend environment. Replace effect-driven synchronous state initialization with lazy initialization or subscriptions, remove unused imports, narrow the vector-map declaration type and assign explicit form-button types.

- [ ] **Step 4: Verify the full quality gate**

Run: `npm run lint; npm test; npm run build; npx playwright test; go test ./internal/... ./cmd/server/...`

Expected: all commands pass with zero lint errors.

- [ ] **Step 5: Commit**

```bash
git add admin backend
git commit -m "test: verify completed platform console"
```

## Plan self-review

- Spec coverage: Tasks 1–4 implement multi-application context, new API resources and every new management screen. Task 5 covers operational filtering/export and current access defects. Task 6 closes testing and lint quality.
- Placeholder scan: no unresolved implementation markers are present.
- Type consistency: client interfaces are introduced before page use, and application context is established before resource scoping.

