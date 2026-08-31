# Per-Application Signing Keys Design

Date: 2026-08-29

## Summary

KeyStar will replace its installation-wide Ed25519 session-token signing key with an independently managed Ed25519 key ring for every application. API credentials and signing keys remain separate security domains: `application_credentials` continues to authenticate publishable and secret API keys by hash, while a new `application_signing_keys` table stores the material and lifecycle state needed to sign and verify application-bound tokens.

This design limits a signing-key compromise to one application, supports safe key rotation, keeps secret API keys out of desktop clients, and permits a device-bound offline license lease for at most 24 hours. It does not claim that a desktop executable is impossible to patch; its goal is to prevent forged licenses, contain key compromise, and make bypass materially harder.

## Goals

- Give every KeyStar application an independent Ed25519 signing-key ring.
- Bind every issued token to an application, product, license, user, and device.
- Keep private signing keys encrypted at rest and out of API responses and logs.
- Support signing-key rotation without interrupting valid sessions.
- Preserve publishable and secret API credential behavior.
- Allow at most 24 hours of device-bound offline use after successful online verification.
- Leave a clean signing-provider boundary for a future KMS or HSM implementation.

## Non-goals

- Making a desktop binary impossible to reverse engineer or patch.
- Treating publishable keys as secrets.
- Embedding secret API credentials or private signing keys in a desktop client.
- Implementing a specific cloud KMS in the first version.
- Replacing the existing refresh-token mechanism with offline leases.

## Security boundaries

### API credentials

`application_credentials` remains responsible for request authentication. Publishable and secret credentials continue to be stored as a locator prefix plus a one-way SHA-256 hash. They are not used to sign JWTs.

- Publishable keys identify a distributed client and its application context. They are not confidential.
- Secret keys authorize trusted server-to-server operations and must never be included in StarLoader or another distributed client.

### Application signing keys

`application_signing_keys` is a separate table and service boundary. Signing keys are asymmetric and have a different lifecycle from API credentials. The private part must be recoverable for signing, so it is encrypted with AES-256-GCM rather than hashed. The key-encryption key is supplied to the backend outside PostgreSQL through `APPLICATION_KEY_ENCRYPTION_KEY`.

Keeping the tables separate prevents credential-listing and credential-rotation code from reading signing material and permits more restrictive database or KMS access later.

## Data model

A new migration creates `application_signing_keys` with:

- `id uuid primary key`
- `kid text not null unique`, generated from cryptographically secure random bytes
- `application_id uuid not null references applications(id) on delete cascade`
- `algorithm text not null`, restricted to `Ed25519`
- `public_key bytea not null`, exactly 32 bytes
- `encrypted_private_key bytea not null`
- `encryption_nonce bytea not null`, exactly 12 bytes for AES-GCM
- `encryption_key_version integer not null`
- `status text not null`, restricted to `pending`, `active`, `retiring`, or `revoked`
- `created_at timestamptz not null`
- `activated_at timestamptz`, null while a key is `pending` and required after activation
- `retire_at timestamptz`
- `revoked_at timestamptz`

A partial unique index permits exactly one `active` key per application. Database constraints require `pending` keys to have no activation, retirement, or revocation timestamp; `active` keys to have `activated_at`; `retiring` keys to have `activated_at` and `retire_at`; and `revoked` keys to have `revoked_at`. A revoked key can never return to `active`.

The encrypted value contains the 32-byte Ed25519 seed, not the expanded 64-byte private key. AES-GCM additional authenticated data includes a versioned context string, `application_id`, `kid`, algorithm, and encryption-key version. Moving ciphertext to another application or changing its metadata therefore makes decryption fail.

Key-encryption keys are versioned 32-byte random values supplied as strict standard Base64 in `APPLICATION_KEY_ENCRYPTION_KEYS`, using the exact comma-separated form `1=<base64>,2=<base64>`. `APPLICATION_KEY_ACTIVE_VERSION` selects one positive integer version present in that map. Configuration loading fails closed on duplicate versions, malformed Base64, a decoded size other than 32 bytes, or a missing active version. Old versions remain available solely to decrypt existing rows during key-encryption-key rotation. Secrets and decrypted key bytes must never be logged and should be retained in memory only for the signing operation.

## Key lifecycle

### Application creation

Application creation and initial signing-key insertion occur in one database transaction:

1. Generate a new Ed25519 seed using the operating system CSPRNG.
2. Derive its public key.
3. Generate a unique `kid`.
4. Encrypt the seed using the current key-encryption-key version and fresh nonce.
5. Insert the application's first key as `active`.

If any step fails, application creation is rolled back. An application cannot become usable without an active signing key.

### Rotation

Signing-key rotation is an authenticated admin operation requiring recent MFA or re-authentication. In one transaction it:

1. Creates a new key as `active`.
2. Moves the previous active key to `retiring`.
3. Sets its `retire_at` to no earlier than the maximum lifetime of every token or offline lease it signed.

New tokens are immediately signed by the new key. A retiring key remains available only for verification until `retire_at`. After that time it becomes `revoked` and is rejected. Emergency revocation may skip the grace period and invalidates all artifacts signed by the compromised key.

Public keys are not deleted during normal rotation so historical audit records remain meaningful. Encrypted private material for a revoked key may be cryptographically erased after all required audit and incident-response retention periods.

## Token profiles

KeyStar issues two distinct signed artifacts.

### Online access token

The existing session JWT retains its exact one-hour lifetime. Its protected header contains:

- `alg: EdDSA`
- `typ: JWT`
- `kid`

Its required claims include:

- `iss`
- `aud`
- `sub`
- `app` (`application_id`)
- `product_id` and canonical product slug
- `license_id`
- `device_id`
- `iat`
- `nbf`
- `exp`
- a unique `jti`
- authorized features

The issuer resolves the signing key from the already authenticated application context. It never trusts an arbitrary application identifier supplied in the request after credential authentication.

### Offline license lease

Offline use is authorized by a separate signed lease, not by extending the online access token. A lease contains the same application, license, product, and device bindings plus `issued_at`, `not_before`, `offline_until`, `kid`, and a unique lease identifier. `offline_until` may be no more than 24 hours after the last successful online verification.

The client stores the lease using operating-system protected storage and binds it to the local device identity. StarLoader records the last trusted server time in protected storage. If the wall clock moves materially behind that trusted time, the lease is rejected and online verification is required. This is tamper resistance, not a claim that a hostile administrator can never patch the executable.

Refresh tokens remain online-only and do not extend an offline lease without contacting KeyStar.

## Request and verification flow

1. The client sends its application ID and publishable key over HTTPS.
2. Credential middleware resolves and authenticates the publishable key, producing a trusted application context.
3. Login or device verification resolves the license and product inside that same application.
4. The signing service loads the application's active signing-key record, authenticates and decrypts its seed, and signs the artifact with its `kid`.
5. The response includes the signed artifact. It never includes a private key or secret API credential.
6. The desktop client selects only a public key explicitly trusted for that application and `kid`.
7. The client verifies the Ed25519 signature before parsing claims for authorization, then verifies issuer, audience, application ID, product, license, device, time bounds, and token profile.

A token signed correctly for application A is rejected by application B both because B does not trust A's key and because the application claim differs.

## Public-key distribution

The admin application detail view exposes only `kid`, algorithm, status, creation time, retirement time, and a copyable base64 public key. Private material is never returned.

For the initial StarLoader integration, trusted application public keys are compiled into the desktop build. Rotation requires distributing the new public key before making it active, or temporarily compiling both current and next public keys. The backend rotation operation therefore supports a staged next key:

- `pending`: public key may be distributed but cannot sign.
- `active`: signs new artifacts.
- `retiring`: verifies existing artifacts only.
- `revoked`: never accepted.

The partial unique index permits only one `active` key. Activation of a staged key is a separate MFA-protected operation.

A later client version may consume a signed JWKS-style key set. Such a document must be anchored to a separately pinned root or update key; downloading keys over TLS alone must not silently replace the client's trust root.

## API and admin behavior

- Application creation automatically creates an active signing key so the application is immediately usable. Subsequent rotation keys begin as `pending` to allow client distribution before activation.
- Application detail returns public signing-key metadata through an admin-authenticated endpoint.
- Key generation, staging, activation, rotation, and emergency revocation are owner-only operations and require recent MFA or re-authentication.
- Publishable and secret credential endpoints remain unchanged.
- Login, device verification, and refresh use the trusted application context to select a signer.
- Unknown, revoked, malformed, mismatched, or expired keys and artifacts fail with stable non-sensitive error codes. Responses never reveal whether private-key decryption or a particular internal lookup failed.
- Audit events record application ID, `kid`, action, actor, result, and timestamp but never key bytes, ciphertext, nonce, credentials, tokens, or decrypted claims containing unnecessary personal data.

## Backward-compatible migration

The rollout must not mix the installation-wide key with per-application keys indefinitely.

1. Add the signing-key table and encryption configuration while retaining the global key as a temporary migration input.
2. Backfill one independently generated key per existing application. Do not copy the global private key into every application.
3. Expose each application's new public key and update StarLoader builds to trust the new `kid` and key alongside the legacy key during a bounded transition.
4. Switch issuance to application-specific keys.
5. Allow legacy tokens to expire. The compatibility deadline is explicit and no longer than the longest previously issued artifact lifetime.
6. Remove legacy verification and the `ED25519_PRIVATE_KEY` runtime requirement after the deadline.

The migration is idempotent and creates a key only for applications without one. A deployment fails closed if per-application issuance is enabled but an application lacks an active key.

## Failure handling

- Missing or invalid key-encryption configuration prevents backend startup once per-application signing is enabled.
- Authentication-tag failure, inconsistent public/private material, or invalid key metadata prevents signing and emits a sanitized internal error plus a security audit event.
- Missing active key prevents issuance for only the affected application.
- An unknown `kid`, wrong application, wrong device, wrong product, invalid signature, invalid time bounds, or revoked key causes client verification failure.
- Database and signing failures do not fall back to a global key.
- Offline lease expiry or clock rollback requires online verification; it never grants an automatic extension.
- Emergency revocation prioritizes containment over uninterrupted offline access.

## Testing strategy

### Security unit tests

- AES-256-GCM round trip, wrong key, modified ciphertext, modified nonce, and modified additional authenticated data.
- Strict key sizes, base64 parsing, nonce uniqueness behavior, and Ed25519 seed/public-key consistency.
- JWT header `kid`, required claims, application binding, unique `jti`, and strict time validation.
- Offline lease maximum lifetime and clock-rollback decisions.

### Store and service tests

- One active key per application and valid lifecycle transitions.
- Two applications receive different key pairs.
- Application A cannot issue or validate an artifact in application B's context.
- Credential authentication determines the signer; request data cannot override it.
- Rotation signs with the new key while the retiring key verifies only until its deadline.
- Emergency revocation rejects the affected key immediately.
- Plaintext private seeds do not appear in persisted rows, API responses, logs, or audit payloads.

### Integration and client tests

- Application creation atomically provisions its signing key.
- Login, device verification, refresh, and offline lease flows work with the correct application key.
- StarLoader accepts the active and explicitly staged rotation keys and rejects unknown `kid` values.
- A valid token with the wrong application, product, device, issuer, or audience is rejected.
- Offline operation stops after 24 hours and after material clock rollback.
- Legacy-key compatibility works only during the declared migration window.

### Verification

- Run all backend unit and PostgreSQL integration tests.
- Run admin unit/component tests and production build.
- Build StarLoader through both the local preset and the exact Qt Creator configuration, then run all CTest tests.
- Perform a manual end-to-end check covering application creation, public-key copy, login, offline grace, key rotation, and emergency revocation.

## Operational requirements

- Production uses independently generated, high-entropy key-encryption keys; test and live environments never share them.
- Key-encryption-key versions and backups are managed outside the database. Losing every usable version makes encrypted signing keys unrecoverable.
- Logs, crash dumps, metrics, traces, and support exports are reviewed to ensure they cannot contain signing seeds, credentials, or complete tokens.
- HTTPS remains mandatory outside explicit loopback-only local development.
- A future KMS/HSM signer implements the same application-key selection interface, allowing private-key handling to move out of the process without changing token or service contracts.

## Acceptance criteria

- Every application has an independent active Ed25519 signing key.
- No distributed client contains a secret API key or private signing key.
- A database-only compromise does not reveal usable private signing keys.
- A compromised application signing key cannot forge artifacts for another application.
- Rotation and emergency revocation have deterministic, audited behavior.
- Offline use cannot be granted for more than 24 hours without renewed online verification.
- The global Ed25519 key is removed after a bounded compatibility period.
