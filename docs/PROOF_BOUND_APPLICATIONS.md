# Proof-Bound Applications

A KeyStar application selects one of two authentication profiles. `legacy`
is the default bearer/refresh flow. `proof_bound` is a strict
600-second EdDSA session bound to a TPM P-256 device key and presented
with a DPoP proof on every request. The profile is authoritative
server-side state (`applications.auth_profile`, migrations
`000021`/`000022`); clients cannot negotiate or override it.

## Application policy

- Every application defaults to `legacy`. Only an explicit update to
  `proof_bound` changes behavior, via `PATCH
  /v1/admin/applications/{id}` (`{"auth_profile":"proof_bound"}`) or
  the console application form.
- Unknown values are rejected (`INVALID_REQUEST`); the console renders
  unknown stored values fail-closed.
- Request handling branches only on the resolved application profile:
  - `legacy` accepts `Authorization: Bearer <session>` and the
    refresh/logout lifecycle.
  - `proof_bound` requires exactly one `Authorization: DPoP
    <access-token>` header plus one `DPoP <proof>` header. Bearer
    tokens, refresh tokens, and DPoP-on-legacy are rejected without
    retry or fallback.
- The signed `app` claim inside a verified token wins over every
  client hint (`X-KeyStar-App`, forwarding headers).

## Key prerequisite: one active application signing key

Proof-bound tokens are issued and verified with the application's own
Ed25519 key, not the legacy global issuer key.

- Before switching an application to `proof_bound`, confirm exactly
  one active key: `GET
  /v1/admin/applications/{id}/signing-keys`, or run `server
  signing-keys backfill` after migration 20 for pre-existing
  applications. Application creation provisions its initial key
  atomically.
- The console disables `proof_bound` selection while signing-key
  metadata reports no active key. Backend validation remains
  authoritative: missing or ambiguous key state fails closed with a
  generic error and issues nothing.
- Token headers carry the signing-key `kid`; verifiers only accept
  the currently active key. Keep PostgreSQL backups paired with the
  full `APPLICATION_KEY_ENCRYPTION_KEYS` ring: losing every
  configured version makes encrypted key seeds unrecoverable.

## Public URI configuration

DPoP proofs bind method and absolute URI. The server rebuilds the
canonical URI from trusted configuration only:

- `PUBLIC_SCHEME` (defaults to `https`) and `PUBLIC_HOST` (required;
  empty fails proof-bound requests closed).
- Canonical form is `<scheme>://<host><normalized-path>` with no
  query, fragment, userinfo, or forwarding-header input
  (`Host`/`X-Forwarded-*` are never trusted).
- The deployment's public entry point must match this configuration
  exactly, or legitimate proofs are rejected. Set both before
  switching the first application.

## Proof-bound token contract

- Lifetime is exactly 600 seconds (`exp - iat = 600`, `nbf = iat`).
- Required claims: `sid`, `jti` (128-bit), `cnf.jkt` (RFC 7638
  SHA-256 thumbprint of the verified TPM P-256 JWK, computed
  server-side, never taken from client text), plus the
  application/product/license/device bindings.
- Device verification for `proof_bound` requires `device_jwk`
  (exact member set `{kty,crv,x,y}`, `EC`/`P-256`, canonical 32-byte
  coordinates on the curve) and verifies the TPM challenge signature
  with that key. The response contains no `refresh_token`.
- `/v1/auth/refresh` on a `proof_bound` application returns generic
  `401 INVALID_REFRESH_TOKEN` without invoking the refresh service.

## Replay retention

- Each accepted DPoP proof is consumed atomically with `insert ...
  on conflict do nothing` into `dpop_replays`, keyed by
  `(application_id, jti_digest)`. The first submission wins; replays
  get `401 INVALID_SESSION_TOKEN`.
- Stored per proof: the SHA-256 of the proof `jti`, the token
  binding, and expiry. Access tokens, proofs, JWKs, coordinates, and
  raw `jti` values are never persisted.
- Rows live until token expiry. `DeleteExpiredDPoPReplays` prunes
  expired rows to bound table size; correctness never depends on
  cleanup running.

## Safe metrics, logs, and errors

- Never log or persist access tokens, proof bodies, JWK coordinates,
  or raw `jti` values. Only digests, expiries, and request IDs leave
  the verification boundary.
- Client-visible failures use the generic safe contract
  (`INVALID_SESSION_TOKEN` / `invalid session token`,
  `INVALID_REFRESH_TOKEN` / `invalid refresh token`) with exact JSON
  keys and matching `X-Request-ID` header/body IDs. Rejection bodies
  never reflect presented secrets or proof material.
- Prometheus request counters/histograms carry route/status labels
  only, no token, key, device, or proof identifiers.

## StarLoader activation order

1. Deploy migrations (`000021`, `000022`) plus code with the
   application still `legacy`. Verify `/status` and `/readyz`.
2. Provision and confirm exactly one active application signing key
   for the StarLoader application (backfill if pre-existing; admin
   signing-keys metadata must show one active key).
3. Deploy the proof-ready StarLoader client (P-256 device JWK,
   DPoP proof per request, no refresh dependency) together with its
   TLS pins. TLS pins remain a client release input, not server
   configuration.
4. Switch the application to `proof_bound` via the console or
   `PATCH /v1/admin/applications/{id}`. Legacy Bearer/refresh
   clients for that application stop working immediately by design.
5. Run the native smoke tests: `STARLOADER_PROOFBOUND_BASE_URL`,
   `STARLOADER_PROOFBOUND_EMAIL`, `STARLOADER_PROOFBOUND_PASSWORD`
   (plus `STARLOADER_PROOFBOUND_APP_ID` /
   `STARLOADER_PROOFBOUND_PUBLISHABLE_KEY` and
   `STARLOADER_PROOFBOUND_PUBLIC_URL` when the fixture is not the
   default application or public entry point), i.e.
   `go test ./tests/blackbox/ -run ProofBound -v`. Without a
   reachable proof-enabled fixture the suite skips explicitly and
   that gap must be recorded, never papered over with bearer.

## Rollback implications

- Rollback is a forward profile change: set the application back to
  `legacy`. Outstanding 600-second proof-bound tokens stop being
  accepted (the bearer path rejects proof-bound tokens and the DPoP
  path no longer applies); clients fall back to password login plus
  device verification on the legacy refresh lifecycle.
- The client must still support the legacy flow (refresh storage
  and rotation) until rollback is ruled out, or rolled-back clients
  cannot stay signed in.
- Replay rows for the proof-bound window are harmless under
  `legacy` (that path never consults them) and expire out;
  pruning continues on its normal schedule.
- Key state is untouched by rollback. Do not rotate or retire the
  application signing key as part of a rollback; that would break a
  subsequent re-activation instead of restoring service.
