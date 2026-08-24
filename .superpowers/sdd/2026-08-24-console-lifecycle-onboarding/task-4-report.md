# Task 4 report — Resumable application onboarding

## Status

Implemented Task 4. The admin console now derives onboarding progress from the selected application's persisted resources and resumes at the next incomplete step after re-entry.

## Implementation

- Added `GET /v1/admin/onboarding/progress`, guarded by the existing application, credential, catalog, and license read permissions.
- Scoped credential, product, plan, and license reads to the validated `X-KeyStar-App` context.
- Returned only the selected application, active resource counts, credential environment, and first usable product/plan display context. Credential hashes, prefixes, scopes, and plaintext values are excluded.
- Added a pure `deriveOnboardingStep` function for application, credential, catalog, license, and complete states.
- Added the `/onboarding` page and permission-filtered sidebar entry.
- Added application/organization selection and creation controls using the existing APIs and application context.
- Added test/live publishable credential creation, active product/plan creation, and seven-day test-license issuance using existing APIs.
- Kept credential and license plaintext values only in ephemeral React state, displayed through an accessible one-time dialog with copy and dismissal controls. No wizard progress or plaintext value is written to localStorage, sessionStorage, cookies, or logs.
- Used accessible `Field` and `AccessibleDialog` patterns and safe client-facing error copy.

`backend/internal/httpapi/adminapi/admin.go` was also changed to register the new endpoint; route registration is required for the handler requested in `admin_console.go`.

## TDD evidence

- Backend RED: `go test ./internal/httpapi/adminapi -run OnboardingProgress -count=1` failed with the expected 404 before route/handler implementation.
- Backend GREEN: the focused endpoint tests passed after the minimal route/handler implementation.
- Admin RED: `npm test -- OnboardingWizard` failed because the onboarding state and wizard modules did not exist.
- Admin GREEN: 3 focused tests passed after implementing state derivation, reload behavior, environment/credential controls, one-time copy, and dismissal.

## Verification

- `cd backend; go test ./internal/httpapi/adminapi -count=1` — PASS
- `cd admin; npm test` — PASS (23 files, 76 tests)
- `cd admin; npm run lint` — PASS (0 errors, 0 warnings)
- `cd admin; npm run build` — PASS; `/onboarding` generated successfully
- `git diff --check` — PASS (line-ending notices only)

## Concerns

- Test-license issuance intentionally reuses the existing license API, which requires an existing user email in the selected application; the wizard states that prerequisite instead of creating a hidden user or backend workflow.
- The existing application middleware requires a valid active default/selected application before any admin route is served. The wizard supports selecting and creating additional organizations/applications, but bootstrapping a deployment with zero applications remains governed by the existing platform bootstrap path and is outside Task 4.

## Fix round

Addressed all Task 4 review findings:

- The wizard now waits for `ApplicationContext` initialization, requests progress with the explicit selected application header, refetches when selection changes, rejects mismatched snapshots, and disables application-scoped mutations unless displayed progress matches the selected application.
- Progress counts only usable active publishable credentials whose optional expiry is still in the future.
- Progress counts only active, time-valid licenses whose product and plan are both active and whose plan belongs to the license product. Active license rows are paged so the derived count is not truncated.
- The onboarding sidebar and command-palette entry now require the same four read permissions as the endpoint: `applications.read`, `credentials.read`, `catalog.read`, and `licenses.read`.
- Catalog creation refreshes server progress in a `finally` path. If product creation succeeds and plan creation fails, the wizard resumes with the persisted product and retries only plan creation.

Fix-round TDD evidence:

- Backend RED exposed `credential_count: 2` and `license_count: 7` for expired/revoked/catalog-invalid fixtures; focused GREEN reports the expected usable counts of 1 and checks every missing read permission.
- Admin RED reproduced six failures covering stale headers, initialization timing, selection refetch/mismatch, composite sidebar permissions, and partial catalog recovery.
- Focused admin GREEN: 4 files and 18 tests passed.

Fix-round verification:

- `cd backend; go test ./internal/httpapi/adminapi -count=1` — PASS
- `cd admin; npm test` — PASS (23 files, 82 tests)
- `cd admin; npm run lint` — PASS (0 errors, 0 warnings)
- `cd admin; npm run build` — PASS; `/onboarding` generated successfully
