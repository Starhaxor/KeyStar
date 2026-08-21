# Moderation and User Operations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add application-scoped account and device/HWID moderation with safe histories, and make the user directory operationally detailed.

**Architecture:** Account bans remain separate from device bans. New device-ban and moderation-event records are application-scoped; device bans retain only device IDs and HMAC-backed device matches, never raw hardware values. Admin routes resolve the selected application before repository access; frontend pages use query filters and the existing API client.

**Tech Stack:** Go 1.24, PostgreSQL migrations, Next.js 16, React 19, TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-08-21-moderation-and-user-operations-design.md`

## Global Constraints

- All moderation reads and writes must use the selected application ID.
- Do not serialize raw HWID or HMAC values to the console or API responses.
- Device bans do not automatically ban linked accounts.
- All mutating admin requests retain permission checks and CSRF protection.
- Every behavior change begins with a failing test.

---

### Task 1: Tenant-safe moderation persistence

**Files:**
- Create: `backend/migrations/000015_moderation.up.sql`
- Create: `backend/migrations/000015_moderation.down.sql`
- Modify: `backend/internal/domain/*.go`
- Modify: `backend/internal/store/console.go`, `backend/internal/store/devices.go`
- Test: `backend/internal/store/*_test.go`

- [ ] Write failing tests proving a device ban can be listed only in its application and raw fingerprint fields are absent from the returned record.
- [ ] Run the affected Go test package and confirm the new tests fail.
- [ ] Add application-scoped `device_bans` and append-only `moderation_events` tables, indexes, lifecycle checks, and a reversible migration.
- [ ] Add domain records and store methods for create, list, lift, expiry evaluation, and linked-account lookup using only device IDs and safe presence flags.
- [ ] Run store tests and migration-up verification; commit the isolated persistence change.

### Task 2: Device verification enforcement and admin APIs

**Files:**
- Modify: `backend/internal/service/device_verify.go`, `backend/internal/store/devices.go`
- Modify: `backend/internal/httpapi/types.go`, `backend/internal/httpapi/adminapi/admin.go`, `backend/internal/httpapi/adminapi/admin_devices.go`
- Test: `backend/internal/service/device_verify_test.go`, `backend/internal/httpapi/adminapi/*_test.go`

- [ ] Write failing tests that an active device ban rejects verification and that a lifted or expired ban does not.
- [ ] Run the focused tests and confirm the failure is caused by the missing device-ban check.
- [ ] Resolve device bans inside the locked verification transaction before session verification completes; record a safe moderation event for rejected attempts.
- [ ] Add paginated admin routes to list, create, and lift device bans; require `devices.read` and `devices.write` respectively.
- [ ] Run service and admin API tests; commit the enforcement/API change.

### Task 3: Account-ban history and detailed user contracts

**Files:**
- Modify: `backend/internal/domain/*.go`, `backend/internal/store/console.go`
- Modify: `backend/internal/httpapi/adminapi/admin_console.go`, `backend/internal/httpapi/types.go`
- Test: `backend/internal/httpapi/adminapi/*_test.go`, `backend/internal/store/*_test.go`

- [ ] Write failing tests for application-scoped account-ban lists, history event ordering, and user rows that contain only safe operational fields.
- [ ] Run the tests and confirm the richer contract is absent.
- [ ] Extend account-ban operations to append moderation events with issuer/lift actor metadata and return application-scoped counts.
- [ ] Extend console-user/list/detail responses with ban summary and activity timeline entries for registration, sessions, licenses, devices, and moderation.
- [ ] Run relevant Go tests and commit the user/history contract change.

### Task 4: Moderation console pages and actionable navigation

**Files:**
- Modify: `admin/src/lib/api.ts`, `admin/src/lib/types.ts`, `admin/src/layout/sidebarNavigation.ts`
- Modify: `admin/src/app/(admin)/bans/page.tsx`, `admin/src/app/(admin)/users/page.tsx`, `admin/src/app/(admin)/users/[id]/page.tsx`
- Create: `admin/src/app/(admin)/device-bans/page.tsx`
- Test: `admin/src/lib/api.test.ts`, `admin/src/layout/sidebarNavigation.test.ts`

- [ ] Write failing tests for moderation navigation groups, query-driven user and ban views, and application-scoped device-ban requests.
- [ ] Run Vitest and confirm the navigation/API expectations fail.
- [ ] Move account bans into a Moderation category with Active, Temporary, Permanent, Expired, Lifted, and History views backed by query filters.
- [ ] Add the Device / HWID Bans page with safe device identity, linked-account list, lifecycle filters, create/lift actions, and history.
- [ ] Expand the Users section to All, Active, Disabled, and Activity views; enrich rows and detail timeline with the new API contract.
- [ ] Run frontend tests, lint, and production build; commit the console change.

### Task 5: End-to-end tenant and security verification

**Files:**
- Modify: `backend/tests/integration/tenant_test.go`
- Test: `backend/tests/integration/tenant_test.go`, `admin/src/lib/api.test.ts`

- [ ] Write an integration test that creates two applications, bans a device in one, and verifies the second application remains unaffected.
- [ ] Run the integration test and confirm it fails before the final isolation wiring is complete.
- [ ] Complete any missing tenant filters discovered by the test without exposing fingerprint HMACs.
- [ ] Run the focused integration test, all impacted Go packages, frontend tests, lint, frontend build, and migration-up verification.
- [ ] Review the diff for raw-hardware serialization and commit the verification change.
