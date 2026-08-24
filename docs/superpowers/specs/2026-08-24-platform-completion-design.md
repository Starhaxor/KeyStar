# KeyStar Platform Completion Design

## Goal

Bring KeyStar from an internally operable licensing console to a deployable, testable, and developer-integratable multi-application platform without breaking the current public or server API contracts.

## Scope and delivery order

The work is deliberately split into independently shippable releases. A later release must not begin until the earlier release has a passing automated quality gate.

### Release A: Production foundation

1. Make the C++ SDK compile and run its tests on Windows. The public header declarations, implementation exception specifications, standard-library includes, and CMake dependency discovery must agree. Unsupported transport and secure-store implementations must return an explicit `Unsupported` error rather than silently behaving as a usable production implementation.
2. Provide a repeatable local and CI verification environment: Docker Compose for PostgreSQL, a GitHub Actions workflow, unit tests, frontend tests/build/lint, and PostgreSQL integration tests run with `TEST_DATABASE_URL`.
3. Establish an accessible console foundation. Inputs have programmatic labels, icon-only controls have names, overlays use a single accessible dialog primitive with focus capture/restore, toasts announce updates, motion honors user preferences, and each route has loading and error recovery.
4. Add browser E2E coverage for authentication, MFA enrollment, application selection, user/license/device operations, and destructive-action confirmation.

### Release B: Console completion

1. Extend organizations and applications from create/list to an explicit lifecycle: edit names/slugs, set active/maintenance/disabled state, and archive only when the server proves the operation is safe. Application actions remain fully audited.
2. Extend products and plans with edit/archive operations. Archived catalog records remain visible for historical licenses but cannot be used for new issuance.
3. Add an application onboarding wizard: select organization, create/select application, select test/live mode, create a credential, copy integration values once, create a product/plan, and issue a test license. Each step is resumable and uses the active application context.
4. Add application-facing operational detail views: credential usage/status, license detail, device detail, and webhook delivery retry/history.

### Release C: Platform capability completion

1. Add the chosen end-user account model. Default decision: keep the existing admin-provisioned model during Releases A and B, then add self-service registration, password reset, and email verification behind explicit application policy flags.
2. Implement the approved offline-lease RFC as a separate security release: signed lease issue/refresh/verify, application policy, key identifier rotation, device binding, monotonic local checkpoint, and revocation-at-next-online-contact semantics.
3. Ship a supported SDK matrix: C++ first, then C#, Rust, and Python. Every SDK needs installation, login/device verification, refresh, secure storage behavior, and CI coverage on supported platforms.
4. Publish operational documentation: Docker deployment, environment contract, reverse proxy/TLS, database backup/restore, metrics/alerting, key rotation, disaster recovery, and API integration quickstarts.

## Architecture

### Backend lifecycle APIs

New admin endpoints live under the existing `/v1/admin` namespace and continue to use the selected `X-KeyStar-App` application context, admin cookie, CSRF protection, RBAC, and audit logging. Existing create/list response contracts remain unchanged. Updates use `PATCH`; destructive state transitions use named actions where their meaning matters, such as `/archive`.

Organization mutations are platform-scoped and require `applications.write`. Application and catalog mutations are application-scoped. Any action that can make data inaccessible must return a conflict response if dependent active records make the transition unsafe.

### Catalog retention

Products and plans receive a status field rather than physical deletion. Existing licenses retain their product/plan history. The license-issuance API accepts only active catalog records. The console shows archived records in a dedicated filter and presents the historical relationship read-only.

### Console UI

The admin console retains Next.js App Router but moves common interaction behavior into reusable primitives:

- `AccessibleDialog`: focus trap, `role=dialog`, `aria-modal`, labelled heading, Escape/backdrop behavior, scroll locking, focus restoration, reduced-motion support.
- `Field`: label, id/name/autocomplete, description, inline validation and error association.
- `DataTableShell`: responsive horizontal scrolling, empty/loading/error states, pagination, URL-synchronized filters.
- `OnboardingWizard`: state is derived from server resources rather than hidden client-only progress; refresh resumes the appropriate next step.

Heavy chart modules remain dynamically imported. New route-level loading and error files prevent a client fetch failure from rendering a blank page.

### SDK build boundary

The C++ SDK builds a transport selected by CMake. On Windows the supported transport is WinHTTP; curl is optional and must be discovered, linked, and documented when selected. Secure token storage uses DPAPI on Windows. macOS Keychain and Linux libsecret support are separately gated targets until implemented. Tests may use an in-memory token store, but packaged clients must not silently fall back to plain-text storage.

### Verification and release gate

Each pull request runs:

1. Go unit, blackbox, and PostgreSQL integration tests.
2. C++ configure/build/ctest on Windows.
3. Admin unit tests, lint, production build, and Playwright E2E tests.
4. Migration up/down validation against a fresh PostgreSQL database.

The default CI service is GitHub Actions. Docker Compose provides the same database topology locally.

## Error handling and safety

- Destructive console actions always use a confirmation dialog with clear affected-resource copy.
- State transitions return typed conflict errors for unsafe dependencies; the UI shows the reason and a link to the blocking resource list where available.
- Account and application selection failures fail closed. No mutation is retried automatically.
- Secrets, temporary passwords, and credential values are rendered once, offer copy affordances, and are never persisted in local storage or logs.
- Offline leases remain disabled unless an application policy explicitly enables them.

## Non-goals

- Billing, payment processing, or subscription collection.
- A permanent transferable license-file product in the offline-lease release.
- Replacing Go/PostgreSQL/Next.js or rewriting existing API surfaces.
- Bulk data migration from external license vendors.

## Global constraints

- Go 1.24+, PostgreSQL 15+, Node.js 20+, Next.js 16, React 19, and C++17 remain the supported baseline.
- New behavior is test-first; every bug fix has a focused regression test.
- No raw hardware identifiers are exposed to the console, SDK logs, or API responses.
- All copy uses KeyStar terminology; StarLoader may appear only in an explicit migration/reference-client context.
- UI must meet the repository's Web Interface Guidelines audit baseline.

## Acceptance criteria

- A fresh checkout can start dependencies, run the complete quality gate, and build the C++ SDK on Windows without manual source edits.
- An operator can create, edit, state-transition, and archive application/catalog records safely, with audit records.
- A new application can complete onboarding and reach a verified test license without consulting source code.
- Core console flows are keyboard accessible and protected by E2E tests.
- Releases C capabilities land as separately reviewed security and SDK releases after Releases A and B are green.
