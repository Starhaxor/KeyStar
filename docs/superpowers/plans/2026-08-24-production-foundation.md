# Production Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the KeyStar SDK, verification environment, and administration-console foundation production-verifiable before adding lifecycle and onboarding features.

**Architecture:** The C++ SDK gains a deterministic Windows build boundary (WinHTTP + DPAPI), while CMake selects platform sources rather than compiling mutually exclusive implementations. Local Docker Compose and GitHub Actions provide the same PostgreSQL integration environment. The console consolidates accessible dialogs and fields before its existing feature pages are extended, then adds Playwright coverage for critical operator flows.

**Tech Stack:** Go 1.24, PostgreSQL 15, C++20/MSVC/WinHTTP/DPAPI, CMake/CTest, Next.js 16, React 19, Vitest, Playwright, Docker Compose, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-08-24-platform-completion-design.md`

## Global Constraints

- Preserve existing `/v1/auth`, `/v1/device`, `/v1/server`, and `/v1/admin` response contracts.
- Never expose raw hardware identifiers or persist secrets in browser storage.
- C++ platform fallbacks must fail explicitly; no silent plaintext or in-memory production persistence.
- Every behavioral change starts with a failing focused test and runs red before implementation.
- All user-facing console copy uses KeyStar terminology unless identifying the legacy StarLoader reference application.
- Run the full Release A quality gate before beginning Release B.

---

## File structure

- `backend/sdk/cpp/CMakeLists.txt`: platform source selection, Windows system dependencies, test target configuration.
- `backend/sdk/cpp/include/keystar/*.hpp`: complete public declarations and required standard-library includes.
- `backend/sdk/cpp/src/platform/transport_winhttp.cpp`: WinHTTP `Transport` implementation.
- `backend/sdk/cpp/src/platform/token_store_dpapi.cpp`: DPAPI-backed `TokenStore` implementation.
- `backend/sdk/cpp/tests/*`: SDK unit and integration-style transport/store tests.
- `docker-compose.yml`: local PostgreSQL service and health check.
- `.github/workflows/verify.yml`: Linux Go/admin verification and Windows SDK verification.
- `scripts/test-integration.ps1` and `scripts/test-integration.sh`: wait for the compose database and run Go integration tests with `TEST_DATABASE_URL`.
- `admin/src/components/ui/dialog/AccessibleDialog.tsx`: shared focus-managed modal primitive.
- `admin/src/components/form/Field.tsx`: programmatically labelled console field primitive.
- `admin/src/app/**/loading.tsx` and `admin/src/app/**/error.tsx`: route-level feedback boundaries.
- `admin/src/components/**/*.test.tsx`: accessibility/unit tests in jsdom.
- `admin/playwright.config.ts`, `admin/e2e/*.spec.ts`, and `admin/package.json`: browser test configuration and scripts.

## Task 1: Repair and verify the Windows C++ SDK build boundary

**Files:**
- Modify: `backend/sdk/cpp/CMakeLists.txt`
- Modify: `backend/sdk/cpp/include/keystar/client.hpp`
- Modify: `backend/sdk/cpp/include/keystar/json_parser.hpp`
- Modify: `backend/sdk/cpp/src/client.cpp`
- Modify: `backend/sdk/cpp/src/platform/transport_winhttp.cpp`
- Modify: `backend/sdk/cpp/src/platform/token_store_dpapi.cpp`
- Test: `backend/sdk/cpp/tests/test_transport.cpp`
- Test: `backend/sdk/cpp/tests/test_token_store.cpp`

**Consumes:** Existing `Transport`, `TokenStore`, `HttpRequest`, `HttpResponse`, `StoredSession`, and CTest target.

**Produces:** A Windows CMake build that compiles exactly one default transport and one secure storage provider, and CTest cases that exercise supported/error paths.

- [ ] **Step 1: Write failing SDK compilation/behavior tests**

Add a Windows-guarded transport test that constructs the default transport and asserts it is non-null. Add a token-store round-trip test that saves a `StoredSession`, loads it back, and clears it. Use a unique test application ID so the test never overwrites another application’s record.

```cpp
#ifdef _WIN32
TEST_CASE(default_windows_transport_is_available) {
  REQUIRE(keystar::createDefaultTransport() != nullptr);
}

TEST_CASE(dpapi_store_round_trips_a_session) {
  auto store = keystar::createPlatformTokenStore("keystar-sdk-test-unique-id");
  REQUIRE(store->save({.refresh_token = "test-refresh", .user_id = "user"}));
  REQUIRE(store->load()->refresh_token == "test-refresh");
  REQUIRE(store->clear());
}
#endif
```

- [ ] **Step 2: Verify the test/build is red**

Run: `cmake -S backend/sdk/cpp -B build/sdk -DKEYSTAR_BUILD_TESTS=ON; cmake --build build/sdk --config Release`

Expected: FAIL because the current SDK has missing `<vector>`/`<memory>` declarations, a `noexcept` mismatch, and no functional Windows transport/store.

- [ ] **Step 3: Implement the smallest Windows build-safe SDK surface**

1. Include `<vector>` in `json_parser.hpp`; include `<memory>` where platform factories use `std::shared_ptr`.
2. Make `Client::isAuthenticated` declaration and definition use the same exception specification; protect its state read with the existing mutex.
3. Change `CMakeLists.txt` so Windows excludes `transport_curl.cpp`, compiles `transport_winhttp.cpp`, links `winhttp` and `crypt32`, and non-Windows alone requires CURL.
4. Implement `WinHttpTransport::send` using `WinHttpCrackUrl`, `WinHttpOpen`, `WinHttpConnect`, `WinHttpOpenRequest`, `WinHttpSendRequest`, `WinHttpReceiveResponse`, `WinHttpQueryHeaders`, and repeated `WinHttpReadData`. Convert UTF-8 URLs/headers to wide strings at the boundary and return a populated `HttpResponse`; convert WinHTTP failures into an `HttpResponse` with a stable SDK error status/message.
5. Implement a DPAPI store that serializes only `StoredSession` fields, protects bytes with `CryptProtectData`, writes beneath `%LOCALAPPDATA%\\KeyStar\\<SHA-256(application_id)>.bin`, uses atomic replace, and reverses the process with `CryptUnprotectData`. `load` returns `nullopt` for a missing record and `save` returns false for crypto/I/O failures.
6. Do not make POSIX storage appear supported; retain its explicit unsupported behavior until its dedicated implementation task.

- [ ] **Step 4: Verify green on Windows**

Run: `cmake -S backend/sdk/cpp -B build/sdk -DKEYSTAR_BUILD_TESTS=ON; cmake --build build/sdk --config Release; ctest --test-dir build/sdk -C Release --output-on-failure`

Expected: configure, compilation, and all CTest cases pass without curl headers being required on Windows.

- [ ] **Step 5: Commit**

```powershell
git add backend/sdk/cpp
git commit -m "fix(sdk): build and secure Windows defaults"
```

## Task 2: Add repeatable PostgreSQL integration infrastructure

**Files:**
- Create: `docker-compose.yml`
- Create: `scripts/test-integration.ps1`
- Create: `scripts/test-integration.sh`
- Create: `.github/workflows/verify.yml`
- Modify: `README.md`
- Test: `backend/tests/integration/schema_migrations_test.go`

**Consumes:** `backend/tests/integration/helpers_test.go` expects `TEST_DATABASE_URL`; backend migrations are embedded in `backend/migrations`.

**Produces:** One local command and one CI workflow that execute the integration suite against disposable PostgreSQL 15.

- [ ] **Step 1: Write a failing environment-contract test**

Add a test helper assertion that verifies the integration URL includes a database name different from the normal development database before `resetAndMigrate` drops the schema.

```go
func TestIntegrationDatabaseMustUseDedicatedName(t *testing.T) {
  url := os.Getenv("TEST_DATABASE_URL")
  if url == "" { t.Skip("requires TEST_DATABASE_URL") }
  if strings.Contains(url, "/keystar?") { t.Fatal("integration database must not be the development database") }
}
```

- [ ] **Step 2: Verify red without the environment**

Run: `cd backend; go test ./tests/integration -run TestIntegrationDatabaseMustUseDedicatedName -count=1`

Expected: the test skips until the dedicated compose URL is supplied; the existing full integration suite still cannot run without `TEST_DATABASE_URL`.

- [ ] **Step 3: Add the local and CI database contract**

1. Add a PostgreSQL 15 `db` service with database `keystar_test`, a non-default user/password for local development, a health check using `pg_isready`, and a named volume.
2. Add both scripts to start `db`, wait for `service_healthy`, set `TEST_DATABASE_URL=postgres://keystar_test:keystar_test@localhost:5432/keystar_test?sslmode=disable`, run `go test ./tests/integration/... -count=1`, and preserve the database on failures for inspection.
3. Add a GitHub Actions workflow with separate `backend`, `admin`, and `sdk-windows` jobs. The backend job starts PostgreSQL as a service and runs `go test ./...` with the same dedicated URL. The admin job runs `npm ci`, `npm test`, `npm run lint`, and `npm run build`. The SDK job configures/builds/ctests under MSVC on `windows-latest`.
4. Document the exact local setup and clean database reset in README. Do not write credentials into source beyond the intentionally local compose defaults.

- [ ] **Step 4: Verify green with Compose**

Run: `docker compose up -d db; ./scripts/test-integration.ps1`

Expected: PostgreSQL becomes healthy and every test in `backend/tests/integration` passes with a dedicated test database.

- [ ] **Step 5: Commit**

```powershell
git add docker-compose.yml scripts .github README.md backend/tests/integration
git commit -m "ci: verify integration tests and SDK builds"
```

## Task 3: Introduce accessible field and dialog primitives

**Files:**
- Create: `admin/src/components/form/Field.tsx`
- Create: `admin/src/components/ui/dialog/AccessibleDialog.tsx`
- Create: `admin/src/components/form/Field.test.tsx`
- Create: `admin/src/components/ui/dialog/AccessibleDialog.test.tsx`
- Modify: `admin/src/components/ui/modal/index.tsx`
- Modify: `admin/src/components/auth/SignInForm.tsx`
- Modify: `admin/src/components/common/ThemeTogglerTwo.tsx`
- Modify: `admin/src/context/ToastContext.tsx`
- Modify: `admin/src/app/globals.css`

**Consumes:** Existing Tailwind classes, `Button`, `Label`, and application toast context.

**Produces:** A shared dialog and field contract used by the sign-in flow and legacy modal wrapper without breaking existing console page call sites.

- [ ] **Step 1: Write failing accessibility tests**

Write jsdom tests asserting that a `Field` links label/input/error through `htmlFor`, `id`, and `aria-describedby`; that an `AccessibleDialog` has `role="dialog"`, `aria-modal="true"`, moves focus inside on open, restores focus after close, and closes with Escape; and that the sign-in password visibility control is a named button.

```tsx
expect(screen.getByLabelText("Email")).toHaveAttribute("name", "email");
expect(screen.getByRole("dialog")).toHaveAttribute("aria-modal", "true");
await user.keyboard("{Escape}");
expect(trigger).toHaveFocus();
```

- [ ] **Step 2: Verify red**

Run: `cd admin; npm test -- Field AccessibleDialog SignInForm`

Expected: FAIL because the current modal has no dialog semantics/focus management and sign-in labels are not associated with fields.

- [ ] **Step 3: Implement the shared primitives and migrate the foundation**

1. Implement `Field` with required `id`, `label`, `name`, `children`, optional `description`, and optional `error`. It renders a real label, attaches `aria-invalid` and `aria-describedby`, and lets existing input classes pass through unchanged.
2. Implement `AccessibleDialog` using a portal, focusable-element loop, focus restore, document scroll lock, Escape handling, backdrop close callback, labelled heading ID, and `prefers-reduced-motion` compatible classes. Use `overscroll-contain` on its scrollable body.
3. Reimplement the existing `Modal` component as a compatibility wrapper around `AccessibleDialog`, preserving its `isOpen`, `onClose`, `className`, `showCloseButton`, and `isFullscreen` public props. Add an accessible name to the close button.
4. Migrate `SignInForm` email/password/MFA inputs to `Field`; add semantic `name`, autocomplete, and `spellCheck={false}` values. Replace the clickable password `<span>` with a button whose `aria-label` changes between “Show password” and “Hide password”.
5. Give `ThemeTogglerTwo` an `aria-label`, mark decorative SVGs `aria-hidden`, add an `aria-live="polite"` region to toast notifications, and add a global reduced-motion media rule.

- [ ] **Step 4: Verify green and run the frontend suite**

Run: `cd admin; npm test; npm run lint; npm run build`

Expected: all unit tests, lint, and production build pass; interactive controls have visible focus styles and named accessible roles.

- [ ] **Step 5: Commit**

```powershell
git add admin/src
git commit -m "feat(admin): add accessible dialog and form foundation"
```

## Task 4: Add route resilience and responsive console shells

**Files:**
- Create: `admin/src/app/(admin)/loading.tsx`
- Create: `admin/src/app/(admin)/error.tsx`
- Create: `admin/src/app/(full-width-pages)/loading.tsx`
- Create: `admin/src/app/(full-width-pages)/error.tsx`
- Modify: `admin/src/components/console/ConsoleSection.tsx`
- Modify: `admin/src/app/(admin)/applications/page.tsx`
- Modify: `admin/src/app/(admin)/credentials/page.tsx`
- Modify: `admin/src/app/(admin)/products/page.tsx`
- Modify: `admin/src/app/(admin)/webhooks/page.tsx`
- Test: `admin/src/components/console/ConsoleSection.test.tsx`

**Consumes:** Existing `LoadingNote`, `ErrorNote`, page components, and Next App Router conventions.

**Produces:** Recoverable route-level errors and a single responsive table wrapper that prevents clipped content on narrow screens.

- [ ] **Step 1: Write failing UI tests**

Test that `ConsoleSection` exposes its content through a horizontal overflow container, that an error boundary presents a retry button, and that the application page’s form controls have accessible labels rather than placeholder-only inputs.

```tsx
expect(screen.getByTestId("console-section-content")).toHaveClass("overflow-x-auto");
expect(screen.getByRole("button", { name: "Retry" })).toBeEnabled();
```

- [ ] **Step 2: Verify red**

Run: `cd admin; npm test -- ConsoleSection`

Expected: FAIL because raw application/catalog tables currently own inconsistent `overflow-hidden` containers and no route error boundary exists.

- [ ] **Step 3: Implement route and table resilience**

1. Add loading skeletons and reset-capable error boundaries to both route groups. Error copy must state the next action and never expose raw server payloads.
2. Add a `data-testid="console-section-content"` content shell to `ConsoleSection`, retaining `overflow-x-auto`, and introduce a reusable table card wrapper for direct-table pages.
3. Migrate Applications, Credentials, Products, and Webhooks to the wrapper; table controls use the Task 3 field contract or explicit `aria-label`s.
4. Split the currently one-line Applications page into view, modal form, and data-loading hooks so its create actions retain their current API behavior while becoming testable.
5. Replace duplicated StarLoader-specific console copy with KeyStar/application-neutral copy.

- [ ] **Step 4: Verify green**

Run: `cd admin; npm test; npm run lint; npm run build`

Expected: route boundaries compile, responsive wrappers have unit coverage, and no existing UI test regresses.

- [ ] **Step 5: Commit**

```powershell
git add admin/src
git commit -m "feat(admin): add resilient routes and responsive tables"
```

## Task 5: Add Playwright coverage for critical operator journeys

**Files:**
- Modify: `admin/package.json`
- Create: `admin/playwright.config.ts`
- Create: `admin/e2e/fixtures.ts`
- Create: `admin/e2e/auth.spec.ts`
- Create: `admin/e2e/application-context.spec.ts`
- Create: `admin/e2e/operator-actions.spec.ts`
- Modify: `.github/workflows/verify.yml`

**Consumes:** Local compose PostgreSQL, backend admin bootstrap command, admin Next server, and existing API routes.

**Produces:** Browser-level tests for the routes that can damage user access or cross application boundaries.

- [ ] **Step 1: Write the first failing browser test**

Create an authenticated fixture that creates a disposable admin/application through backend commands/API, then assert an unauthenticated `/users` visit redirects to `/signin` and a valid administrator can complete sign-in and see the selected application label.

```ts
test("protects console routes and opens the selected application", async ({ page }) => {
  await page.goto("/users");
  await expect(page).toHaveURL(/\/signin$/);
  await page.getByLabel("Email").fill(fixture.adminEmail);
  await page.getByLabel("Password").fill(fixture.adminPassword);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByLabel("Selected application")).toHaveValue(fixture.applicationId);
});
```

- [ ] **Step 2: Verify red**

Run: `cd admin; npx playwright test e2e/auth.spec.ts`

Expected: FAIL because Playwright configuration, deterministic backend fixture setup, and browser scripts are absent.

- [ ] **Step 3: Implement deterministic E2E infrastructure and journeys**

1. Add `@playwright/test`, an `e2e` script, a config that starts backend/admin services with the compose database, and browser installation instructions.
2. Add fixtures that create isolated organizations/applications/admin accounts and always clean their test data through a dedicated test database reset, never the developer database.
3. Cover: authentication/redirect, MFA enrollment required path, application switching keeps user/license/device data scoped, destructive user/session/device actions require confirmation before requests execute, and keyboard Escape closes the shared dialog and restores focus.
4. Add the Playwright command to CI after backend/admin jobs make their services available.

- [ ] **Step 4: Verify green**

Run: `cd admin; npm run e2e`

Expected: all browser journeys pass headlessly against the compose-backed services.

- [ ] **Step 5: Commit**

```powershell
git add admin .github/workflows/verify.yml
git commit -m "test(admin): cover critical operator journeys"
```

## Task 6: Run the Release A quality gate and document the handoff

**Files:**
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-08-24-platform-completion-design.md`
- Test: all release commands below

**Consumes:** Tasks 1–5.

**Produces:** A documented, reproducible Release A gate and a clear handoff for Release B lifecycle/onboarding implementation.

- [ ] **Step 1: Add failing documentation verification checks**

Add shell/PowerShell checks in CI that fail if README does not mention the compose integration command, C++ SDK Windows build command, admin E2E command, and required local environment variables.

```powershell
$required = @('docker compose up -d db', 'test-integration.ps1', 'npm run e2e', 'cmake -S backend/sdk/cpp')
foreach ($text in $required) { if (-not (Select-String -LiteralPath README.md -Pattern $text -SimpleMatch)) { throw "README missing $text" } }
```

- [ ] **Step 2: Verify red**

Run the documentation check before README changes.

Expected: FAIL because the current README does not define the complete Release A gate.

- [ ] **Step 3: Document the supported verification path**

Document prerequisites, compose startup, Go integration test command, C++ CMake/CTest command, frontend checks, Playwright browser installation/test command, and teardown guidance. Update the design spec Release A acceptance note from planned to delivered only after all checks pass.

- [ ] **Step 4: Run the full gate**

Run in order:

```powershell
docker compose up -d db
./scripts/test-integration.ps1
cd backend; go test ./...
cmake -S sdk/cpp -B ../build/sdk -DKEYSTAR_BUILD_TESTS=ON
cmake --build ../build/sdk --config Release
ctest --test-dir ../build/sdk -C Release --output-on-failure
cd ../admin; npm test; npm run lint; npm run build; npm run e2e
```

Expected: every command exits 0. Record any external prerequisite that prevents a command from running; do not claim Release A complete without its output.

- [ ] **Step 5: Commit**

```powershell
git add README.md docs .github scripts
git commit -m "docs: publish production foundation verification gate"
```

## Plan self-review

- Spec coverage: Tasks 1–2 cover SDK and CI; Tasks 3–5 cover accessible console, route feedback, and E2E; Task 6 enforces the release gate. Release B and C remain intentionally out of scope and require separate plans after this gate passes.
- Placeholder scan: executable task steps contain no unresolved work markers or deferred implementation language.
- Type consistency: Tasks use existing SDK interfaces and preserve current admin API contracts; new frontend primitives are introduced before their migrations.
