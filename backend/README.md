# KeyStar Backend

Go + PostgreSQL API serving the public client (`/v1/auth/*`,
`/v1/device/*`, `/v1/me`), developer server (`/v1/server/*`), and
administration console (`/v1/admin/*`) surfaces.

## Develop

```powershell
go test ./internal/...
go vet ./...
go run ./cmd/server migrate up
go run ./cmd/server serve
```

Integration tests use the disposable Compose database only (see
`..\scripts\test-integration.ps1`); never point `TEST_DATABASE_URL`
at a development database. Black-box tests in `tests/blackbox/`
target a live fixture and skip explicitly when its environment is
absent.

## Proof-bound activation

Applications default to the `legacy` bearer/refresh profile. A
`proof_bound` application issues strict 600-second EdDSA tokens
bound to a TPM P-256 JWK thumbprint (`cnf.jkt`), requires a DPoP
proof per request, and has no refresh flow. Activation order:

1. Deploy migrations and code with the application still `legacy`.
2. Provision and confirm exactly one active application signing key
   (`server signing-keys backfill` for pre-existing applications;
   `GET /v1/admin/applications/{id}/signing-keys` to confirm).
3. Set `PUBLIC_SCHEME` (default `https`) and `PUBLIC_HOST` to the
   exact public entry point; forwarding headers are never trusted.
4. Deploy the proof-ready client (DPoP per request, no refresh
   dependency; TLS pins ship with the client release).
5. Switch the application to `proof_bound`, then run
   `go test ./tests/blackbox/ -run ProofBound -v` against a
   proof-enabled fixture.

Full contract, replay retention, safe-metrics rules, and rollback:
`..\docs\PROOF_BOUND_APPLICATIONS.md`.
