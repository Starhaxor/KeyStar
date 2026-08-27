# KeyStar Comprehensive Security Hardening Design

**Date:** 2026-08-27

## Scope and decisions

This change closes the confirmed findings from the repository-wide security audit across the Go API, Next.js console, C++ SDK, dependencies, CI, and deployment defaults. Webhooks are intentionally restricted to public HTTPS destinations in every environment; loopback and private-network delivery is rejected even after DNS resolution and redirects.

## Backend design

- Generate webhook signing secrets with `crypto/rand`, return a URL-safe encoded value once, and hash exactly the returned secret bytes.
- Centralize webhook URL validation. Require HTTPS, a hostname, an allowed port, public DNS results, and revalidate every outbound connection. Disable automatic redirects so redirects cannot bypass validation.
- Scope refresh-session revocation by application ID at the HTTP, interface, and SQL layers.
- Route refresh requests through application resolution and credential verification. Issue refreshed access tokens with the configured issuer, audience, and product and reload the user's current license/features before signing.
- Encrypt TOTP seeds at rest with AES-256-GCM using a dedicated environment key. Decrypt only inside the authentication boundary and support an explicit encrypted value format so plaintext records cannot silently continue.
- Enforce minimum 32-byte independent application secrets, secure production cookie settings, and strict origin parsing.
- Require a dedicated bearer token for `/metrics` whenever metrics are enabled, checked in constant time.
- Replace process-local security rate-limit state with PostgreSQL-backed counters where authentication state is shared. Keep a bounded in-memory fallback only for endpoints that run before database availability, and document the deployment boundary.

## Console and SDK design

- Disable the Next.js powered-by header and add CSP, HSTS, Permissions-Policy, and the existing clickjacking/MIME/referrer protections centrally.
- Make the C++ client reject plaintext HTTP by default. A narrowly named development option may permit HTTP only for loopback hosts.
- Encode request JSON through one escaping helper, add connect/overall timeouts, cap response bodies, and reject redirects or protocol downgrades in both curl and WinHTTP transports.

## Supply chain and operations

- Upgrade Go and vulnerable modules to patched supported versions.
- Add `govulncheck`, `gosec`, `go vet`, npm production audit, tests, and builds to CI; use least-privilege workflow permissions and pinned action revisions.
- Update examples and deployment documentation for required secrets, secure cookies, metrics authentication, TLS, and mandatory image rebuild/recreation.

## Verification

Each behavior starts with a failing unit or integration test. Completion requires Go tests with serial database packages, `go vet`, `govulncheck`, `gosec`, npm tests/lint/build/audit, C++ build/CTest, and a final review of the complete diff. No live database or container is destructively changed as part of implementation.
