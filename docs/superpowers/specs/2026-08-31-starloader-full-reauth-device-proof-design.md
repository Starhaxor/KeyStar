# StarLoader Full Reauthentication and Device Proof Design

**Date:** 2026-08-31

## Summary

StarLoader will use KeyStar as the only online authorization authority. A local
client decision such as `success=true` will never grant a server-controlled
entitlement. Every application start and every expired access session requires
a fresh email/password authentication followed by a fresh, single-use TPM
challenge. StarLoader stores no refresh token and receives no offline lease.

Every protected client request also carries a short-lived, single-use proof
signed by the registered TPM P-256 device key. The proof binds the access token,
HTTP method, canonical target URI, request creation time, and a unique proof ID.
KeyStar validates the proof, the access session, the current account, license,
product, plan, device, ban, and entitlement state before serving the operation.

Selected StarLoader decision code is virtualized in protected release builds to
raise reverse-engineering cost. Virtualization is defense in depth and is never
treated as an authorization boundary.

## Repositories and ownership

- `KeyStar` owns application policy, authentication, device verification,
  access-session state, request-proof verification, replay state, entitlement
  enforcement, audit events, application signing keys, and the administration
  controls for these policies.
- `StarLoader` owns credential entry, TPM key use, server-token verification,
  request-proof generation, in-memory session state, pin verification, release
  protection markers, and the signed release pipeline.
- The KeyStar C++ SDK remains a general platform SDK. StarLoader may use its
  transport primitives, but StarLoader's no-refresh policy is enforced by the
  KeyStar application configuration rather than only by client behavior.

## Security objectives

- A patched local `login()` result cannot create a valid KeyStar session.
- A copied access token is unusable without the registered TPM private key.
- A captured request proof cannot be replayed.
- Revoking an access session, account, license, device, plan, or entitlement is
  effective at the next protected server request.
- StarLoader never persists an access token, refresh token, password, TPM
  private key, or request proof.
- Compromise of one KeyStar application's signing key cannot forge artifacts
  for another application.
- Protected releases are Authenticode-signed after virtualization so Windows
  can validate publisher identity and final-file integrity.

## Explicit non-goals

- The desktop executable is not claimed to be unpatchable.
- TPM possession is not proof that the operating system is malware-free.
- Passwords are not sent with every API request. Password verification occurs
  only during full authentication; protected requests use TPM proof of
  possession and current server-side authorization.
- StarLoader has no offline grace period, automatic sign-in, refresh-token
  flow, or persisted session.
- Anti-debugging, hook detection, code virtualization, and pinning cannot
  replace KeyStar authorization.

## Application session policy

KeyStar adds an application-scoped session policy. The trusted application
context resolved from `X-KeyStar-App` and the authenticated publishable
credential selects the policy; request JSON cannot override it.

The policy has the following fields:

| Field | StarLoader value | Meaning |
| --- | --- | --- |
| `access_ttl_seconds` | `600` | Access token lifetime is exactly ten minutes. |
| `refresh_mode` | `disabled` | Device verification returns no refresh token and refresh requests fail closed. |
| `offline_mode` | `disabled` | No offline lease can be issued. |
| `proof_mode` | `required` | Every protected end-user request requires a valid TPM proof. |
| `full_reauth_on_expiry` | `true` | Expiry requires password plus a new TPM challenge. |
| `proof_max_age_seconds` | `60` | Proof creation time must be within the accepted server window. |

Existing KeyStar applications retain an explicit compatibility policy during
migration. New applications default to secure online behavior, but changing an
existing application to proof-required is a staged administrator action so
deployed clients are not broken without warning.

Policy changes require an owner permission, recent MFA, CSRF validation, and an
audit record containing the application, actor, old values, new values, result,
and time. Secrets, tokens, raw proofs, passwords, and TPM public-key bytes are
not included in the audit payload.

## Authentication and session flow

1. StarLoader starts with empty in-memory authentication state.
2. The user enters email and password.
3. StarLoader ensures its named Microsoft Platform Crypto Provider P-256 key
   exists with private export disabled and signing-only usage.
4. `POST /v1/auth/login` resolves the KeyStar application and publishable
   credential, verifies the account and application-scoped license, and creates
   a pending UUIDv7 authentication session plus a random 32-byte challenge.
5. KeyStar stores only the challenge digest. The challenge is single-use and
   expires after the existing short challenge window.
6. StarLoader signs the decoded challenge through the TPM and sends the exact
   challenge, raw fixed-width P-256 signature, public CNG blob, and bounded
   hardware signals to `POST /v1/device/verify`.
7. KeyStar locks and validates the challenge, pending session, application,
   account, license, product, plan, device policy, device state, and ban state
   before consuming the challenge.
8. KeyStar creates an active access-session record bound to the application,
   user, license, product, device, and TPM public-key thumbprint.
9. KeyStar returns only a ten-minute access token for the StarLoader policy. It
   does not return a refresh token or offline lease.
10. StarLoader verifies the server signature and every required claim before
    retaining the token in memory.
11. StarLoader calls `/v1/me` with the access token and a fresh TPM request
    proof. The dashboard opens only after the protected server response passes.
12. Closing StarLoader clears the session. Token expiry while the application
    is running clears protected state and returns the UI to full login.

Password failures return the same safe public error as unknown accounts.
Challenge, device, and session failures use stable non-sensitive error codes.

## Access-token profile

KeyStar implements the already-designed per-application Ed25519 signing-key
ring before enabling the new token profile. The protected header contains:

- `alg: EdDSA`
- `typ: JWT`
- the application signing-key `kid`

The payload requires:

- `iss`, `aud`, `sub`
- `app`
- `product_id` and canonical product slug
- `license_id`, `device_id`
- `sid` for the active access-session record
- a cryptographically random `jti`
- `iat`, `nbf`, `exp`, with `exp - iat = 600` for StarLoader
- current authorized features
- `cnf.jkt`, the RFC 7638 SHA-256 JWK thumbprint of the registered P-256
  device public key

The token verifier rejects unknown or revoked `kid` values, duplicate JSON
members, unsupported algorithms or types, non-canonical encoding, missing or
extra policy-critical claim shapes, wrong application/product/device/license,
invalid time bounds, and an invalid signature.

## Active access sessions and revocation

KeyStar persists access sessions separately from refresh sessions. An access
session includes the token `sid`, application, user, license, product, device,
device-key thumbprint, status, issue time, expiry time, revocation time, and a
bounded revocation reason code.

Statuses are `active`, `revoked`, and `expired`. A revoked session cannot become
active again. Protected middleware loads the session by `sid` and verifies all
token bindings. Expired rows may be transitioned or removed by bounded
maintenance after the audit-retention window.

Administrative session revocation, user disablement, license revocation,
product or plan archival, entitlement removal, account ban, device ban, and
device revocation all cause protected requests to fail. Authorization is
computed from current database state; token features are an upper bound and
never restore an entitlement removed after issuance.

## TPM request proof

Protected client calls use the `DPoP` authorization scheme and a compact JWS in
the `DPoP` header. StarLoader converts its existing CNG P-256 public blob to the
corresponding public EC JWK. The proof header contains `typ=dpop+jwt`,
`alg=ES256`, and the public JWK. The proof payload contains:

- `jti`: 128 bits from the operating-system CSPRNG, base64url encoded
- `htm`: uppercase HTTP method
- `htu`: normalized absolute HTTPS URI without query or fragment
- `iat`: proof creation time
- `ath`: base64url SHA-256 hash of the exact ASCII access-token value

The ES256 signature uses the fixed-width 64-byte `r || s` representation that
the existing Windows CNG key produces. Before handler execution, KeyStar:

1. validates the access token and application policy;
2. parses the proof with strict size and shape limits;
3. verifies `htm`, normalized `htu`, `iat`, and `ath`;
4. computes the proof JWK thumbprint and compares it with `cnf.jkt` and the
   active session's registered device-key thumbprint;
5. verifies the ES256 signature;
6. atomically records the application-scoped hash of `jti` for the accepted
   proof window; and
7. rejects a previously recorded proof ID.

Proof replay rows store only the application ID, a keyed or plain SHA-256 proof
ID digest, and expiry. They contain no token, public key, request body, or raw
proof. A database constraint and unique index make acceptance atomic across
multiple KeyStar instances. Cleanup deletes only expired proof rows.

The first release does not require a server-provided DPoP nonce for every
request because strict `iat`, single-use `jti`, TLS, method/URI binding, and
token binding provide the required online replay controls. The protocol leaves
a stable `use_dpop_nonce` error path for a later risk-triggered server nonce.

## Protected authorization pipeline

Every protected end-user endpoint executes these stages in order:

1. resolve the application and verify the allowed publishable credential;
2. verify the application-specific access-token signature and claims;
3. verify the active access session and device-key binding;
4. verify and atomically consume the TPM request proof;
5. reload current account, license, product, plan, device, and ban state;
6. evaluate endpoint-specific entitlement and scope;
7. execute the operation; and
8. record security-relevant success or denial events without secrets.

Endpoint handlers cannot opt out accidentally. Public, client-authenticated,
end-user-protected, server-credential, and admin-cookie routes use distinct
router groups and middleware types.

## Rate limiting and audit

Authentication limits are PostgreSQL-backed and independently keyed by safe
digests of client IP, normalized account identifier, application, license,
device fingerprint, pending session, and active session where those values are
available. Public responses remain generic and use `429` with bounded retry
metadata. Rate-limit keys never contain plaintext passwords, license keys, raw
hardware identifiers, access tokens, or proofs.

Security events cover authentication success/failure, challenge replay,
invalid device signatures, invalid or replayed request proofs, session
revocation, policy changes, signing-key lifecycle actions, pin failures
reported through safe telemetry, and abnormal concurrent-session behavior.
Audit and security-event readers remain application-scoped and RBAC-protected.

## StarLoader client behavior

StarLoader keeps email only as ordinary UI state. It clears password storage as
soon as the login request is serialized and never logs it. Access tokens and
proof material remain in process memory only and are overwritten or cleared on
logout, expiry, protected-request failure, or destruction of the auth manager.

The API client has one request-proof builder shared by all protected calls. It
receives the method, canonical target URL, and access token and returns the
compact proof without exposing the TPM private key. A protected request cannot
be sent through a code path that omits proof generation.

`401` invalid-token/session/proof responses and current-state authorization
denials clear protected client state. Token expiry does not call a refresh
endpoint; it returns to the credential form. Server-side operations remain
authoritative even if local UI state is patched.

## Certificate pinning

Production StarLoader builds use normal Windows trust validation plus an SPKI
SHA-256 allowlist. The build contains a current pin and one staged next pin so
certificate rotation does not require disabling validation. Pins apply only to
the exact configured KeyStar production host. Redirects, hostname changes,
certificate errors, and pin mismatch fail closed.

Loopback HTTP remains available only in an explicitly named local-development
build. Runtime environment variables cannot weaken production TLS or replace
pins.

## Selective bytecode virtualization

Protected releases use VMProtect markers behind a StarLoader-owned abstraction:

- `STARLOADER_VM_BEGIN(name)` / `STARLOADER_VM_END()` for virtualization;
- `STARLOADER_MUTATE_BEGIN(name)` / `STARLOADER_MUTATE_END()` for lighter
  mutation where virtualization cost is unjustified; and
- no-op definitions in development, unit-test, and ordinary local builds.

The first protected regions are small, deterministic, infrequently executed
coordination functions for:

- request-proof field construction and binding checks;
- strict access-token claim-policy decisions;
- SPKI pin selection and the final pin decision;
- consolidation of distributed integrity-check results; and
- transition from a verified server response to authenticated client state.

The following code is not virtualized: Qt UI/event loops, network transports,
OpenSSL or Windows CNG library internals, exception-handling blocks, large or
tight loops, parsing over attacker-controlled unbounded data, and entire
executables. Marker regions have one entry and one exit and do not permit jumps
from unprotected code into their interior.

No private key, password, refresh token, server signing secret, or symmetric
protocol secret is introduced by virtualization. A patched VM decision can
alter local behavior but cannot create a valid KeyStar authorization.

The release pipeline is:

1. build the unsigned release candidate;
2. run unit, integration, native, and UI tests on the unprotected candidate;
3. apply VMProtect using a version-controlled project configuration that
   contains no license credential;
4. smoke-test the protected executable, including real TPM signing and KeyStar
   proof verification;
5. scan the protected artifacts with the organization's supported malware
   scanners and retain results;
6. Authenticode-sign the final protected EXE and shipped DLLs with SHA-256;
7. apply an RFC 3161 SHA-256 timestamp;
8. verify signatures and timestamps with Windows tooling; and
9. generate the distribution package and checksums.

Protection never runs after Authenticode signing. Release automation fails if
required markers were not processed, protection verification fails, signing is
missing, timestamping is missing, or signature verification fails. Developer
builds remain usable without VMProtect, but cannot be mislabeled as protected
production releases.

## Application signing keys

The approved per-application signing-key design in
`docs/superpowers/specs/2026-08-29-per-application-signing-keys-design.md`
remains the source of truth for encrypted Ed25519 key rings, `kid` lifecycle,
rotation, emergency revocation, and migration from the global key. This design
overrides only its StarLoader online token lifetime and offline behavior:
StarLoader tokens live for ten minutes, refresh is disabled, and no offline
lease is issued.

## Rollout and compatibility

1. Restore a green StarLoader baseline without absorbing unrelated working-tree
   changes.
2. Implement KeyStar per-application signing keys and migrate existing
   applications with an explicit legacy-token deadline.
3. Add access-session storage and application session policies in compatibility
   mode.
4. Extend the token profile with `kid`, `sid`, `jti`, `nbf`, and `cnf.jkt`.
5. Add TPM proof verification and replay persistence behind policy mode.
6. Release a StarLoader build that supports the new token and proof flow while
   the KeyStar StarLoader policy remains staged.
7. Enable proof-required, refresh-disabled, offline-disabled, ten-minute policy
   for the StarLoader application.
8. Let legacy one-hour tokens expire, then remove their verification path.
9. Add pinning, protected release packaging, Authenticode signing, and release
   evidence gates.

No rollout step silently falls back from required proof to bearer-only access.

## Failure handling

- Missing application policy, active signing key, session row, or device
  binding fails closed for proof-required applications.
- Database failure during proof replay consumption denies the request; it does
  not accept an untracked proof.
- TPM unavailability prevents StarLoader authentication and protected calls.
- Pin mismatch or TLS failure prevents network authentication.
- VMProtect unavailability blocks only the protected production-release job,
  not development and test builds.
- Authenticode certificate or timestamp-service failure blocks release output.
- Public errors are stable and non-sensitive; internal diagnostics use request
  IDs and secret-free structured events.

## Verification strategy

### KeyStar

- Token tests cover per-application `kid`, ten-minute StarLoader lifetime,
  `sid`, unique `jti`, `nbf`, `cnf.jkt`, wrong application/device/product, and
  legacy rejection after the compatibility deadline.
- Proof tests cover valid ES256, wrong key, method, URI, token hash, clock
  window, malformed JWK, duplicate fields, oversized proof, and replay.
- Concurrency tests prove two KeyStar instances cannot both consume one `jti`.
- Session tests prove every revocation and current-state change denies the next
  request.
- Policy tests prove StarLoader receives no refresh token or offline lease and
  cannot call refresh successfully.
- Admin tests cover RBAC, recent MFA, CSRF, policy staging, audit payloads, and
  application isolation.

### StarLoader

- Unit tests cover canonical proof fields, token hash, unique proof IDs, CNG
  signature format, token-policy validation, session clearing, and no refresh.
- API tests assert every protected request carries exactly one access token and
  one matching proof.
- Native live-flow tests perform password login, TPM challenge, protected
  `/v1/me`, proof replay rejection, expiry, and full reauthentication.
- TLS tests cover current and staged pins, wrong host, wrong key, redirects,
  certificate failure, and production refusal of HTTP.
- Protected-release smoke tests verify every named VM marker is processed and
  critical flows still work after virtualization.
- Signature checks prove the protected output and shipped DLLs have valid
  SHA-256 Authenticode signatures and RFC 3161 timestamps.

## Acceptance criteria

- StarLoader requires password plus a fresh TPM challenge on every launch and
  after every access-token expiry.
- StarLoader persists no session or refresh credential and has no offline use.
- A copied access token fails without the registered TPM key.
- Replaying an accepted proof fails across concurrent KeyStar instances.
- Revocation and entitlement changes take effect on the next protected call.
- Every application uses an independent, rotatable signing-key ring.
- StarLoader's production policy is enforced by KeyStar, not by client choice.
- Selected critical code is virtualized only in protected release builds.
- Final release artifacts are protected first and Authenticode-signed and
  timestamped afterward.
- Existing unrelated working-tree changes in both repositories are preserved.
