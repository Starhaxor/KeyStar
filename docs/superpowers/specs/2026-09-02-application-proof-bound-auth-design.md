# Application-Scoped Proof-Bound Authentication Design

## Goal

Add a reusable `proof_bound` authentication profile to KeyStar. StarLoader will use it first: access tokens last exactly 600 seconds, carry a TPM P-256 key thumbprint, cannot be refreshed, and authorize protected requests only when accompanied by a valid DPoP proof. Existing applications remain on the current `legacy` profile until explicitly migrated.

## Security Boundary

The application is the policy boundary. `legacy` preserves the existing bearer and refresh behavior. `proof_bound` enforces all of the following without per-request fallback:

- application-scoped Ed25519 signing key with a required `kid`;
- exact 600-second access-token lifetime;
- application, product, license, device, session, token ID, and TPM JWK thumbprint claims;
- no refresh-token issuance or refresh endpoint use;
- `Authorization: DPoP <access-token>` plus exactly one `DPoP` header on protected routes;
- atomic, application-scoped replay rejection for every proof `jti`.

Changing an application from `legacy` to `proof_bound` invalidates compatibility with legacy clients by design. It does not alter other applications.

## Application Policy

Add an application authentication profile with two allowed values: `legacy` and `proof_bound`. Existing rows default to `legacy`. The admin application API and console expose the selection and warn that proof-bound clients are required before activation.

Runtime authorization resolves the application from the verified publishable credential or the token's signed application claim. Client-provided headers never override the signed application boundary. The profile is loaded from authoritative storage for token issuance and protected-request verification.

## Device Key Registration

The device-verification request for a proof-bound application includes the TPM P-256 public JWK (`kty=EC`, `crv=P-256`, 32-byte base64url `x` and `y`). KeyStar validates canonical base64url encoding, curve membership/validity, and exact member set. It computes the RFC 7638 thumbprint over:

```json
{"crv":"P-256","kty":"EC","x":"...","y":"..."}
```

The existing challenge signature continues to prove possession of the non-exportable device key. KeyStar binds the computed thumbprint to the verified device/session result and never accepts a client-supplied thumbprint directly.

## Access Token Profile

The application signer issues compact EdDSA JWTs with header `alg=EdDSA`, `typ=JWT`, and the active application signing-key `kid`. Proof-bound claims include:

- `iss`, `aud`, `sub`;
- `app`, `product`, `license_id`, `device_id`;
- `sid` and random 128-bit `jti`;
- integer `iat`, `nbf`, and `exp`, where `exp - iat = 600` exactly;
- `cnf: {"jkt":"<RFC7638 thumbprint>"}`.

Verification rejects duplicate JSON members, noncanonical base64url, unknown/revoked keys, missing claims, wrong application/product, invalid time boundaries, or malformed thumbprints. Error responses remain generic and never echo a token or proof.

Proof-bound device verification returns only the access token and expiry metadata. It does not create or return a refresh token. `/v1/auth/refresh` rejects proof-bound application sessions even if a legacy refresh credential is presented.

## DPoP Verification

A reusable HTTP middleware protects session routes. For `proof_bound`, it requires exactly one DPoP authorization header and one compact proof. It validates before calling the handler:

1. protected header is exactly `typ=dpop+jwt`, `alg=ES256`, with a valid embedded P-256 public JWK;
2. JWK thumbprint equals the access token's signed `cnf.jkt`;
3. ES256 signature is a canonical fixed-width 64-byte `r || s` signature over the compact signing input;
4. `htm` equals the actual uppercase HTTP method;
5. `htu` equals the normalized absolute request URI, without query or fragment and with the production host/scheme supplied by trusted server configuration;
6. `ath` equals base64url SHA-256 of the exact ASCII access token;
7. `iat` is within a narrow configured clock-skew window and not after token expiry;
8. `jti` is canonical 128-bit randomness and has not been consumed.

Legacy applications continue through the existing bearer verifier. Route handlers receive the same verified `SessionClaims` context regardless of profile, so authorization logic is not duplicated.

## Replay Store

Add a `dpop_replays` table keyed by `(application_id, jti_digest)`, where `jti_digest` is SHA-256 of the canonical proof `jti`. Store token ID/session binding and `expires_at`; do not store access tokens or proof bodies.

Consumption is a single atomic insert with a unique constraint. A conflict is replay and fails closed. The expiry is bounded by the earlier of token expiry and the proof acceptance window. Cleanup deletes expired rows opportunistically and through the existing maintenance pattern; correctness never depends on cleanup running.

## Failure and Compatibility Behavior

- Missing, malformed, expired, mismatched, or replayed proofs return one generic unauthorized response.
- Database errors during replay consumption fail closed.
- No proof-bound request is retried as bearer.
- Rate limiting remains outside expensive signature verification where possible, while replay consumption occurs only after all stateless checks pass.
- Logs contain request IDs and stable reason codes only, never credentials, JWTs, JWK coordinates, or DPoP bodies.
- Existing SDK behavior remains available only to `legacy` applications. SDK proof support is a separate consumer; StarLoader is the first proof-bound client.

## Admin and Activation

The admin console can view and change the application authentication profile. Enabling `proof_bound` requires an active application signing key. The UI warns that existing refresh sessions stop authorizing new proof-bound access and that rollback should be treated as an explicit operational decision.

StarLoader activation order:

1. deploy migrations and proof-capable KeyStar code with StarLoader still `legacy`;
2. provision/confirm the active application signing key;
3. deploy the proof-ready StarLoader client and production TLS pins;
4. switch only the StarLoader application to `proof_bound`;
5. run native login, protected profile, replay, stolen-token/different-key, expiry, and TLS-pin smoke tests.

## Testing

Unit tests cover strict token/JWK/DPoP parsing, signature and binding failures, exact 600-second timing, URL normalization, and sanitized errors. Store tests prove atomic replay rejection, tenant isolation, expiry cleanup, and fail-closed database errors. HTTP tests prove no handler invocation on proof failure and no bearer fallback. Integration tests cover login/device verification through `/v1/me`, refresh rejection, replay, different TPM key, application isolation, and legacy compatibility. Admin tests cover profile validation and activation prerequisites.

## Explicit Non-Goals

- Removing legacy bearer/refresh support globally.
- Persisting StarLoader refresh or offline leases.
- Trusting TPM attestation as a replacement for proof of possession.
- Solving client TLS pin rotation in KeyStar; StarLoader owns its two-pin release configuration.
- Claiming production readiness without the native proof-enabled StarLoader smoke test.
