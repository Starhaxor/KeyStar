# Task 2 — Audited lifecycle admin APIs

## Changed files

- `backend/internal/httpapi/adminapi/admin_applications.go`
- `backend/internal/httpapi/adminapi/admin_products.go`
- `backend/internal/httpapi/adminapi/admin.go`
- `backend/internal/httpapi/adminapi/admin_lifecycle_test.go`
- `.superpowers/sdd/2026-08-24-console-lifecycle-onboarding/task-2-report.md`

## Red/green results

- RED: the focused lifecycle handler suite failed with `404 INVALID_REQUEST` for every new route before implementation.
- GREEN: `go test ./internal/httpapi/adminapi -run 'TestAdmin(ApplicationTransitionRequiresApplicationsWrite|ApplicationTransitionMapsDependencyConflict|ApplicationPatchReturnsUpdatedPayloadAndAuditState|ProductArchiveUsesSelectedApplicationAndAudits|PlanPatchUsesSelectedApplicationAndAudits|PlanArchiveMapsConflict)$' -count=1` passed.
- Verification: `go test ./internal/httpapi/adminapi ./internal/service -count=1` passed.
- Verification: `go test ./internal/... -count=1` passed.

## Action and error mapping

| Endpoint | Audit action |
| --- | --- |
| `PATCH /v1/admin/applications/{id}` | `APPLICATION_UPDATED` |
| `POST /v1/admin/applications/{id}/transition` | `APPLICATION_TRANSITIONED` |
| `PATCH /v1/admin/products/{id}` | `PRODUCT_UPDATED` |
| `POST /v1/admin/products/{id}/archive` | `PRODUCT_ARCHIVED` |
| `PATCH /v1/admin/products/{id}/plans/{planID}` | `PLAN_UPDATED` |
| `POST /v1/admin/products/{id}/plans/{planID}/archive` | `PLAN_ARCHIVED` |

Every successful lifecycle mutation records the entity ID and non-secret `before`/`after` metadata. `domain.ConflictError` values map to HTTP 409 using their safe domain code/message, including `APPLICATION_IN_USE`, `CATALOG_RECORD_IN_USE`, and `CATALOG_RECORD_INACTIVE`; missing resources map to entity-specific 404s. Invalid lifecycle inputs map to safe 400 responses.

## Commit

`feat(api): expose audited lifecycle actions`
