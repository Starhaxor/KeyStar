# Application Proof-Bound Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a reusable application-scoped `proof_bound` profile to KeyStar and activate the backend contract required by StarLoader without changing legacy applications.

**Architecture:** Application policy selects either the existing bearer/refresh flow or a strict 600-second EdDSA token bound to a TPM P-256 JWK thumbprint. A reusable DPoP verifier performs stateless proof checks, then atomically consumes an application-scoped replay identifier before handlers receive verified session claims.

**Tech Stack:** Go 1.24, PostgreSQL/pgx, net/http, Ed25519, ECDSA P-256, Next.js/TypeScript admin console, Docker-backed integration tests.

**Spec:** `docs/superpowers/specs/2026-09-02-application-proof-bound-auth-design.md`

## Global Constraints

- Existing applications default to `legacy`; only explicitly selected applications use `proof_bound`.
- Proof-bound access tokens have an exact 600-second lifetime and no refresh token or bearer fallback.
- Never log or persist access tokens, proof bodies, JWK coordinates, or raw `jti` values.
- Client headers cannot override the application ID signed into a verified token.
- Replay-store errors and ambiguous policy/key state fail closed.
- Every task starts with a focused failing test and ends with a focused green run and commit.

---

### Task 1: Persist the Application Authentication Profile

**Files:**
- Create: `backend/migrations/000021_application_auth_profile.up.sql`
- Create: `backend/migrations/000021_application_auth_profile.down.sql`
- Modify: `backend/internal/domain/application.go`
- Modify: `backend/internal/store/applications.go`
- Modify: `backend/internal/store/applications_test.go`
- Modify: `backend/tests/integration/schema_migrations_test.go`
- Modify: `backend/internal/httpapi/adminapi/admin_applications.go`
- Modify: `backend/internal/httpapi/adminapi/admin_applications_test.go`

**Interfaces:**
- Produces: `domain.ApplicationAuthProfile` with `legacy` and `proof_bound`.
- Produces: `Application.AuthProfile` and `UpdateApplication.AuthProfile`.
- Produces: admin application JSON field `auth_profile`.

- [x] Add migration/store tests proving existing and new applications default to `legacy`, only the two exact values are accepted, and update/list/get preserve the profile.
- [x] Run `go test ./internal/store ./tests/integration -run 'Application|Migration'` and confirm RED because the column/type do not exist.
- [x] Add `applications.auth_profile text not null default 'legacy' check (auth_profile in ('legacy','proof_bound'))`; the down migration removes only this column.
- [x] Add the typed domain constants and reject an empty/unknown profile in update validation while letting create omit it for the database default.

```go
type ApplicationAuthProfile string

const (
    ApplicationAuthLegacy     ApplicationAuthProfile = "legacy"
    ApplicationAuthProofBound ApplicationAuthProfile = "proof_bound"
)
```

- [x] Update every application query/scan and the admin JSON mapper/request decoder; add admin tests for valid transition and `INVALID_REQUEST` on unknown values.
- [x] Run focused store, migration, and admin HTTP tests; commit `feat(applications): add proof-bound auth profile`.

### Task 2: Issue Strict Application-Scoped Proof-Bound Tokens

**Files:**
- Modify: `backend/internal/security/token.go`
- Modify: `backend/internal/security/token_test.go`
- Modify: `backend/internal/security/application_signer.go`
- Modify: `backend/internal/security/application_signer_test.go`
- Modify: `backend/internal/httpapi/types.go`

**Interfaces:**
- Produces: `security.ProofBoundClaims` fields `SessionID`, `TokenID`, `DeviceKeyThumbprint`, `NotBefore` in the existing `SessionClaims` boundary.
- Produces: `ApplicationSigner.IssueProofBound(ctx, applicationID string, claims security.SessionClaims) (token string, expiresAt time.Time, err error)`.
- Token header includes the active application signing key `kid`.

- [x] Add deterministic token tests for exact header `{alg:EdDSA,typ:JWT,kid}`, exact `exp-iat=600`, required `nbf/sid/jti/cnf.jkt`, application/product/license/device bindings, duplicate member rejection, canonical base64url, unknown/revoked `kid`, and ±60-second skew boundaries.
- [x] Confirm focused RED with `go test ./internal/security -run 'ProofBound|ApplicationSigner'`.
- [x] Split strict parsing into small helpers that reject duplicate JSON members before unmarshalling; cap compact tokens at 16 KiB and require three nonempty canonical segments.
- [x] Extend `ApplicationSigner` to load the active application key, decrypt it, issue with a random 128-bit token ID, and return a generic error on missing/ambiguous key state.

```go
type confirmationWire struct { JKT string `json:"jkt"` }
// exp = iat + 600; nbf = iat; kid is mandatory for proof_bound.
```

- [x] Preserve the existing one-hour legacy issuer/verifier unchanged behind its legacy call path.
- [x] Run all security tests; commit `feat(tokens): issue application proof-bound sessions`.

### Task 3: Bind Device Verification to the TPM JWK and Disable Refresh

**Files:**
- Create: `backend/internal/security/p256_jwk.go`
- Create: `backend/internal/security/p256_jwk_test.go`
- Modify: `backend/internal/httpapi/device_verify.go`
- Modify: `backend/internal/httpapi/device_verify_test.go`
- Modify: `backend/internal/service/device_verify.go`
- Modify: `backend/internal/service/device_verify_test.go`
- Modify: `backend/internal/httpapi/refresh.go`
- Modify: `backend/internal/httpapi/refresh_test.go`

**Interfaces:**
- Produces: `security.ParseP256JWK(raw json.RawMessage) (publicKey *ecdsa.PublicKey, thumbprint string, err error)`.
- Extends proof-bound device verify input with `device_jwk`.
- Consumes Task 2 `ApplicationSigner.IssueProofBound`.

- [x] Add JWK tests for exact member set, canonical 32-byte `x/y`, P-256 curve membership, RFC 7638 fixture, extra/missing members, invalid points, padding, and malformed JSON.
- [x] Add service/HTTP tests proving proof-bound verification requires the JWK, verifies the existing TPM challenge with that key, returns a 600-second token with matching `cnf.jkt`, and returns no refresh token; legacy output stays unchanged.
- [x] Add refresh tests proving a proof-bound application/session is rejected with a generic unauthorized response and no token issuance.
- [x] Confirm focused RED, then implement strict JWK parsing and pass the computed thumbprint—not client text—into token issuance.
- [x] Branch refresh issuance only on authoritative `Application.AuthProfile`; never infer policy from request fields.
- [x] Run `go test ./internal/security ./internal/service ./internal/httpapi -run 'JWK|Device|Refresh|ProofBound'`; commit `feat(auth): bind proof sessions to device keys`.

### Task 4: Verify Compact DPoP Proofs

**Files:**
- Create: `backend/internal/security/dpop.go`
- Create: `backend/internal/security/dpop_test.go`

**Interfaces:**
- Produces: `DPoPInput { Proof, AccessToken, Method, URI string; Token security.SessionClaims; Now time.Time }`.
- Produces: `VerifyDPoP(input DPoPInput) (ProofClaims, error)` where `ProofClaims` contains only `JTIDigest [32]byte`, `IssuedAt`, and `KeyThumbprint`.

- [x] Write table tests for exact `typ=dpop+jwt`, `alg=ES256`, embedded P-256 JWK, 64-byte raw `r||s`, canonical compact encoding, method, normalized URI, `ath`, signed `cnf.jkt`, 128-bit `jti`, and accepted clock skew.
- [x] Add rejection tests for DER signatures, duplicate members, query/fragment in `htu`, host/scheme mismatch, wrong method/token/key, expired token/proof, malformed randomness, and reflected secret-free errors.
- [x] Confirm RED because `VerifyDPoP` is absent.
- [x] Implement parsing with size caps, duplicate-member rejection, `ecdsa.Verify`, constant-time digest comparisons, and RFC 7638 reuse from Task 3.

```go
expectedATH := sha256.Sum256([]byte(input.AccessToken))
// htu is compared to a server-built canonical absolute URI, never Host/X-Forwarded-* directly.
```

- [x] Run `go test ./internal/security -run DPoP`; commit `feat(security): verify TPM-bound DPoP proofs`.

### Task 5: Add Atomic Replay Consumption and Proof-Required Middleware

**Files:**
- Create: `backend/migrations/000022_dpop_replays.up.sql`
- Create: `backend/migrations/000022_dpop_replays.down.sql`
- Create: `backend/internal/store/dpop_replays.go`
- Create: `backend/internal/store/dpop_replays_test.go`
- Modify: `backend/internal/httpapi/auth.go`
- Modify: `backend/internal/httpapi/auth_test.go`
- Modify: `backend/internal/httpapi/router.go`
- Modify: `backend/internal/httpapi/types.go`
- Modify: `backend/cmd/server/main.go`

**Interfaces:**
- Produces: `ReplayStore.ConsumeDPoP(ctx context.Context, applicationID string, jtiDigest [32]byte, tokenID string, expiresAt time.Time) (consumed bool, err error)`.
- Produces: profile-aware `RequireSession` that selects legacy bearer or proof-bound DPoP after authoritative token/application resolution.

- [x] Add migration/store tests for `(application_id,jti_digest)` uniqueness, tenant isolation, 32-byte digest constraint, atomic concurrent consumption, expiry cleanup, and database-error failure.
- [x] Add middleware tests proving exactly one `Authorization: DPoP` and one `DPoP` header are required, replay/stateless failures never invoke the handler, database errors fail closed, and no Bearer retry occurs.
- [x] Add legacy compatibility tests proving `legacy` still accepts its existing Bearer path and rejects DPoP unless migrated.
- [x] Confirm focused RED, create the replay table without raw proof/JTI storage, and implement single-statement `insert ... on conflict do nothing` consumption.
- [x] Build the canonical external URI only from configured public scheme/host plus the normalized request path; do not trust arbitrary forwarding headers.
- [x] Wire the repository, application resolver, application signing-key verifier, and clock in `main.go`; keep rate limiting ahead of signature work.
- [x] Run store/HTTP tests and `go test ./...`; commit `feat(api): require DPoP for proof-bound applications`.

### Task 6: Expose the Profile in the Admin Console

**Files:**
- Modify: `admin/src/lib/api.ts`
- Modify: `admin/src/lib/api.test.ts`
- Modify: `admin/src/app/(admin)/applications/ApplicationsForms.tsx`
- Modify: `admin/src/app/(admin)/applications/ApplicationsView.tsx`
- Modify: `admin/src/app/(admin)/applications/ApplicationsView.test.tsx`
- Modify: `admin/e2e/application-context.spec.ts`

**Interfaces:**
- Consumes Task 1 `auth_profile` admin field.
- Produces an application edit control with `legacy` and `proof_bound` and an activation warning.

- [x] Add component/API tests proving profile display, update payload, unknown-value fail-closed rendering, and warning text that states refresh/Bearer clients stop working.
- [x] Confirm RED, then add the typed API field and a select control to the existing application lifecycle form without creating a separate settings source of truth.
- [x] Disable proof-bound selection in the UI when signing-key metadata has no active key, while backend validation remains authoritative.
- [x] Run `npm test`, lint, and build from `admin`; run the focused Playwright application test; commit `feat(admin): configure proof-bound authentication`.

### Task 7: Prove StarLoader Compatibility and Document Activation

**Files:**
- Create: `backend/tests/blackbox/proof_bound_auth_test.go`
- Modify: `backend/tests/blackbox/production_server_test.go`
- Modify: `backend/README.md`
- Modify: `README.md`
- Create: `docs/PROOF_BOUND_APPLICATIONS.md`
- Modify: `docs/superpowers/plans/2026-09-02-application-proof-bound-auth.md`

**Interfaces:**
- Consumes all prior tasks and the StarLoader request/token contract.
- Produces documented activation and rollback order for any application.

- [x] Add a black-box test that provisions a proof-bound application and exercises password login, TPM challenge verification, 600-second keyed token, DPoP `/v1/me`, replay rejection, different-key stolen token rejection, expiry, refresh rejection, and a parallel legacy application.
- [x] Add concurrency coverage proving only one of two identical proof submissions succeeds.
- [x] Run the black-box test against PostgreSQL and confirm RED before final routing/configuration fixes.
- [x] Document application policy, key prerequisite, public URI configuration, replay retention, safe metrics, StarLoader activation order, rollback implications, and that TLS pins remain a client release input.
- [x] Run `go test ./...`, repository integration/black-box suites, admin tests/lint/build/Playwright, migration up/down/up, `git diff --check`, and a secret scan.
- [x] Record any unavailable native StarLoader fixture explicitly; never replace it with bearer or claim it passed.
- [x] Commit `docs: activate proof-bound KeyStar applications`.
