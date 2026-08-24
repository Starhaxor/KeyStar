# Task 5 report — lifecycle/onboarding integration and E2E coverage

## Delivered

- Added server API integration coverage that proves a credential can issue a
  license only through its active application catalog, rejects a plan archive
  while that active license depends on it, and cannot issue against a user in
  another application.
- Added `admin/e2e/lifecycle-onboarding.spec.ts`, covering browser creation of
  an isolated application and user, persisted credential/catalog progress
  across reloads, test-license issuance, and the completed state after reload.
- Added browser-only onboarding fixture data. It does not seed, reset, or
  select a database; `e2e-fixture` remains the sole guarded database fixture.
- Documented the focused Playwright command, the exact `keystar_test` guard,
  Docker requirement, and the required free ports (`5432`, `8080`, `3000`).

The existing `admin-e2e` workflow already invokes `npm run e2e`, while the
Playwright configuration matches every `e2e/**/*.spec.ts` file. The new spec
is therefore included without narrowing or otherwise weakening that job; no
workflow edit was necessary.

## Test-first note

Tasks 1–4 were already present in this worktree when Task 5 began. The new
regression tests were written before any application-code changes, but a
feature-missing red state could not be reproduced without removing completed
behavior. No production behavior was changed for this test-only task.

## Verification

- PASS: `cd admin; npm test` — 27 files, 86 tests.
- PASS: `cd admin; npm run lint`.
- PASS: `cd admin; npm run build`.
- PASS: `cd admin; npx playwright test e2e/lifecycle-onboarding.spec.ts --list`
  — one targeted test discovered.
- BLOCKED: `cd backend; go test ./tests/integration -run
  TestServerAPILifecycleKeepsLicenseIssuanceScopedToActiveCatalog -count=1`
  reached the database setup but PostgreSQL rejected the configured
  `keystar_test` credentials with `SQLSTATE 28P01`.
- BLOCKED: `cd admin; npx playwright test e2e/lifecycle-onboarding.spec.ts
  --reporter=list` stopped before the test started because
  `http://127.0.0.1:8080/readyz` was already in use. The browser runner is
  configured not to reuse an existing backend, so it could not safely prove
  that process was attached to the guarded `keystar_test` database.

No Docker service, port owner, or database state was changed while recording
these blocks.

## P1 follow-up — catalog issuance rejection

- Added an HTTP-level integration assertion that first creates a valid user,
  archives a separate unused product plan, then attempts issuance for that
  same valid user. This guarantees the handler reaches catalog resolution
  rather than returning from user lookup.
- The server API now maps typed catalog conflicts to the safe
  `409 CATALOG_RECORD_INACTIVE` response, instead of exposing the condition
  as a generic 500 response.
- PASS: `cd backend; go test ./internal/httpapi/serverapi -count=1`.
- BLOCKED again: the focused integration command cannot authenticate to the
  locally reachable `keystar_test` PostgreSQL instance (`SQLSTATE 28P01`), so
  the database-backed assertion awaits a Docker-capable or correctly
  provisioned test database.
