# Task 5 report — Playwright coverage for critical operator journeys

## Changed files

- `admin/package.json` and `admin/package-lock.json`
  - Added `@playwright/test`, `npm run e2e`, and the local Chromium install command `npm run e2e:install`.
- `admin/playwright.config.ts`
  - Starts the dedicated PostgreSQL service locally through Compose, resets/migrates `keystar_test`, starts the Go backend, and starts the Next admin server.
  - In CI, uses the workflow PostgreSQL service instead of Compose.
- `admin/e2e/fixtures.ts`
  - Seeds one worker-isolated organization, two applications, an MFA-enrolled owner, an unenrolled owner, and application-scoped user/license/device/session records.
  - Selects the alpha application through the real cookie contract, completes real UI sign-in with a generated TOTP, and resets the dedicated database after the worker.
- `admin/e2e/auth.spec.ts`
  - Covers unauthenticated redirect, password + MFA sign-in, selected application label/value, and the MFA-enrollment-required path.
- `admin/e2e/application-context.spec.ts`
  - Proves users, licenses, and devices remain scoped while switching between the two seeded applications.
- `admin/e2e/operator-actions.spec.ts`
  - Proves destructive user/session/device requests do not execute before confirmation, then exercises the real confirmed requests.
  - Proves Escape closes the shared dialog and restores focus to its trigger.
- `backend/cmd/e2e-fixture/main.go` and `main_test.go`
  - Added the strictly test-only reset/seed command required to provision records consistently on local Compose and GitHub service-container PostgreSQL.
  - Added focused coverage for the dedicated database URL guard.
- `.github/workflows/verify.yml`
  - Added an `admin-e2e` job after the existing backend and admin jobs, with PostgreSQL 15, Go/Node setup, Chromium installation, and `npm run e2e`.
- `admin/.gitignore`
  - Ignores Playwright reports and test results.

## Setup and teardown safety

- Destructive setup/teardown reads only `TEST_DATABASE_URL`; it never falls back to `DATABASE_URL`.
- The URL is parsed by pgx and the parsed database name must equal exactly `keystar_test`. Similar names such as `keystar_test_backup`, the developer database `keystar`, missing database names, and malformed URLs are rejected before opening a connection.
- Playwright resets/migrates the dedicated database before starting the backend, seeds test records once for the single worker, and resets/migrates the same dedicated database in fixture teardown.
- `workers: 1` prevents shared PostgreSQL fixture mutation from crossing worker boundaries.
- Local Playwright startup never reuses an already-running backend/admin server, avoiding accidental connection to services configured for a developer database.
- A negative end-to-end startup check using a `.../keystar` URL stopped with: `refusing E2E database "keystar": TEST_DATABASE_URL must name exactly "keystar_test"`.

## Red/green evidence

1. Browser-spec red:
   - Command: `cd admin; npx playwright test e2e/auth.spec.ts --reporter=list`
   - Result: failed because `./fixtures` did not exist. This was the expected missing Playwright fixture/config infrastructure.
2. Database-guard red:
   - Command: `cd backend; go test ./cmd/e2e-fixture -run TestValidateDedicatedDatabaseURL -count=1`
   - Result: compile failure, `undefined: validateDedicatedDatabaseURL`.
3. Database-guard green:
   - Command: `cd backend; go test ./cmd/e2e-fixture -count=1`
   - Result: pass.
4. Static/browser-code green:
   - Command: `cd admin; npx tsc --noEmit`
   - Result: pass.

## Verification results

- `cd admin; npm run e2e:install`
  - Pass. Chromium and Chromium Headless Shell 151 were installed.
- `cd admin; npm test`
  - Pass: 16 files, 56 tests.
- `cd admin; npm run lint`
  - Pass: zero errors.
- `cd admin; npx tsc --noEmit`
  - Pass.
- `cd admin; npm run build`
  - Pass: production build compiled, typechecked, and generated all 23 static pages.
- `cd backend; go test ./cmd/e2e-fixture -count=1`
  - Pass.
- `cd backend; go test ./...`
  - Every non-integration package passed. `backend/tests/integration` could not run because `TEST_DATABASE_URL` is unavailable without PostgreSQL; its failures all report `TEST_DATABASE_URL must be set for PostgreSQL integration tests`.
- `cd admin; npm run e2e`
  - Not executable to browser assertions on this machine because Docker Desktop is not running. Playwright configuration started and then failed while bringing up PostgreSQL with: `failed to connect to the docker API at npipe:////./pipe/dockerDesktopLinuxEngine`.
  - This is a local external prerequisite limitation, not a skipped browser installation. CI supplies a healthy PostgreSQL 15 service and runs the same Playwright command after the existing backend/admin jobs.

## Review follow-up

- The reset path already ran `store.MigrateUp`, and migration 000004 creates the required `starloader` application before the backend starts. The reset command now also calls `FindDefaultApplication` as an explicit postcondition and fails before `serve` if the seeded application is missing or has the wrong slug.
- Worker teardown now encloses the seed command, JSON parsing, and test execution. A partial seed failure or malformed fixture JSON therefore still invokes the guarded `reset` command instead of leaving partial records behind.
- Added focused red/green evidence:
  - `go test ./cmd/e2e-fixture -run TestVerifyRequiredDefaultApplication -count=1` was red with `undefined: verifyRequiredDefaultApplication`, then passed after the reset postcondition was implemented.
  - `npx vitest run e2e/fixture-lifecycle.test.ts` was red because `fixture-lifecycle` did not exist, then passed 2/2 tests proving cleanup after seed and JSON-parse failures.
- Fresh follow-up verification:
  - `go test ./cmd/e2e-fixture -count=1`: pass.
  - `npm test`: pass, 17 files and 58 tests.
  - `npm run lint`: pass.
  - `npx tsc --noEmit`: pass.
  - `npm run build`: pass, including production type checking and 23 generated static pages.
  - `npm run e2e`: still blocked before browser assertions by the unavailable Docker Desktop engine pipe, with the same exact error recorded above.
