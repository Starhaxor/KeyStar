# KeyStar

KeyStar is a license & device-identity platform: a Go backend serving the
public client API, the server-to-server API and the admin dashboard API, plus
a Next.js admin panel and an SDK architecture reference.

## Repository layout

```
docs/                         — architecture & design documents
  KEYSTAR_PLATFORM_SDK_ARCHITECTURE.md
scripts/                      — one-off helper scripts (Windows dialog watcher)
backend/                      — Go license/device service
  cmd/server/                 — the server binary (serve | migrate | admin | keygen)
  internal/
    httpapi/                  — HTTP layer: router, middleware, response helpers,
                                public client API (login, device verify, me)
    httpapi/adminapi/         — /v1/admin namespace handlers (dashboard API)
    httpapi/serverapi/        — /v1/server namespace handlers (machine-to-machine)
    service/                  — business logic (login, device verification, admin auth)
    store/                    — PostgreSQL repository (per-aggregate files)
    domain/                   — domain models, errors, permissions
    credential/               — application credential keys (ks_pk_/ks_sk_)
    security/                 — tokens, licenses, passwords, TOTP, HMAC, signatures
    admin/                    — CLI admin command implementations (create-user/…)
    config/                   — environment configuration
  migrations/                 — numbered SQL migrations (000001_initial … 000009_*)
  tests/
    integration/              — PostgreSQL integration tests (store + HTTP flows)
    blackbox/                 — end-to-end smoke tests
admin/                        — Next.js admin dashboard (TailAdmin template)
```

## API namespaces

| Namespace      | Package                     | Auth                                   |
| -------------- | --------------------------- | -------------------------------------- |
| `/v1/auth/*`, `/v1/device/*`, `/v1/me` | `internal/httpapi`    | publishable credential (`ks_pk_*`) + session token |
| `/v1/server/*` | `internal/httpapi/serverapi`| secret credential (`ks_sk_*`) + scope  |
| `/v1/admin/*`  | `internal/httpapi/adminapi` | admin session cookie + CSRF + RBAC     |

## Backend quickstart

```bash
cd backend
cp .env.example .env          # set DATABASE_URL, LICENSE_HMAC_KEY, keys
go run ./cmd/server migrate up
go run ./cmd/server serve
```

Every feature task ends with the full suite:

```bash
cd backend
go build ./... && go vet ./...
go test ./internal/...
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/keystar_test?sslmode=disable" \
  go test ./tests/...
```
