# KeyStar Platform Console Design

## Goal

Complete the KeyStar admin console so it can operate every platform capability
available to an administrator: applications, credentials, products and plans,
device policy, webhooks, operational lists and their supporting security and
quality controls.

## Scope and boundaries

The existing console remains a Next.js 16 application backed by Go HTTP
handlers under `/v1/admin`. It will remain compatible with the current default
application, while allowing privileged users to choose an application explicitly.
The server-to-server API remains an integration API; its webhook resources gain
an equivalent admin-console management surface rather than being moved.

The work does not introduce billing, public self-service pages, email delivery,
or a webhook-delivery worker. It exposes and manages the persisted domain
entities already present in the repository.

## Information architecture

The sidebar will contain these permission-gated sections:

- Overview
- Applications
- Products & plans
- Users and bans
- Licenses
- Devices, including Device policy
- Sessions
- Webhooks
- API credentials
- Variables
- Audit log
- Administrators and roles
- Security, including MFA and security events

An application selector in the header is visible to authorized administrators.
It persists the selection in a cookie and every selected-application request
uses the existing `X-KeyStar-App` header. Without that header, the backend
uses the established default application for backwards compatibility.

## Backend contract

Admin endpoints will be added for organizations/applications, product catalog
and plans, webhook CRUD and webhook delivery history. Existing credential and
device policy routes will be added to the TypeScript API client.

Every application-scoped admin route resolves the selection header, validates
that the application is active, and confines storage calls to that application.
The response shape follows the existing `{ ok, items, total, page, page_size }`
convention. List routes accept `page`, `page_size`, `search`, `status`,
`from`, `to`, and a route-specific sort field where it is meaningful.

New permissions are grouped by resource: applications, catalog, webhooks and
credentials. Device-policy writes use the existing devices permissions. Role
creation validates the complete backend permission set, and the UI renders the
same set so no backend permission becomes unassignable.

Credentials and webhook signing secrets are returned only by their create
responses. List responses expose prefixes and metadata, never secret values.

## Console flows

### Applications

Administrators can list applications, create one inside an existing
organization, select an application, and update its operational status. The
selector clears list state by causing page queries to refetch.

### Products and plans

Products can be created, listed and updated. Each product owns plans that can
be created, listed and updated. The license form loads active products and
plans for the selected application, applies plan defaults for duration and
device count, and still permits an authorized administrator to override them.

### Credentials, device policy and webhooks

Credential management supports list, create and revoke. Device policy supports
read, update and reset-to-default. Webhooks support list, create, edit,
disable/delete and a read-only delivery history. Each destructive action uses
the existing confirmation modal and displays API errors inline.

### Operational data

Users, bans, licenses, devices, sessions, audit logs and security events use
server-side filtering and pagination. CSV export requests all records matching
the active filter, rather than exporting only the current client page.

The dashboard keeps its default 14-day view and adds a date range and a manual
refresh action. Dashboard cards link to the filtered source list.

## Security and error handling

The backend remains the authorization authority; frontend permission checks
only hide unavailable navigation and actions. Security-events navigation is
explicitly gated. The profile activity view must not depend on `audit.read`;
it receives a dedicated endpoint scoped to the current admin.

The route proxy remains fail-closed for invalid sessions. API transport errors
are shown as recoverable page errors with retry controls. All state-changing
requests retain the existing CSRF header behavior.

## Quality bar

New behavior is developed test-first. Go handler tests cover permission,
application-boundary, validation and secret-redaction behavior. Frontend tests
cover API-client headers, important forms and authorization visibility. A
Playwright smoke suite covers sign-in, application selection, credential
creation, product-plan licensing and webhook creation.

`npm run lint`, `npm run build`, `go test ./internal/... ./cmd/server/...`,
and the new frontend tests must pass with no lint errors. Existing React effect
lint violations are refactored as part of this work.

## Delivery order

1. Establish application context and RBAC contract.
2. Add platform-management admin APIs and Go tests.
3. Add TypeScript API client/types and shared selected-application behavior.
4. Implement the five platform screens and wire license creation to the catalog.
5. Upgrade filtering/export/dashboard and fix permission/profile gaps.
6. Add frontend and browser tests, then make lint and production build clean.
