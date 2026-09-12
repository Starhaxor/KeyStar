# KeyStar

**Self-hosted authentication, software licensing, and device authorization for your applications.**

KeyStar gives you a Go API and web administration console to manage users, licenses, products, devices, and sessions across multiple applications. Run it on your own infrastructure and connect your desktop clients with a publishable application key.

Built for licensed software, private utilities, and authorized modding or testing tools. [StarLoader](https://github.com/Starhaxor/StarLoader) is the Windows reference client for its TPM-backed, proof-bound authentication flow.

[Quick start](#quick-start) · [Application integration](#application-integration) · [StarLoader + KeyStar](#starloader--keystar) · [MIT License](LICENSE)

## What you get

- **Application isolation:** organizations and applications scope users, licenses, devices, credentials, and administrative operations.
- **Licensing controls:** products, plans, expiration, device limits, and revocation.
- **Device trust:** challenge verification, TPM support, hardware matching policies, and device reset/revoke workflows.
- **Administration:** a web console with MFA, role-based permissions, audit logs, and account/device moderation.
- **Integration:** publishable client keys, scoped server credentials, variables, webhooks, and a Windows C++ SDK target.

Actively developed. Deployment validation remains necessary; offline leases are a [proposal](docs/OFFLINE_LEASES_RFC.md), not a supported production workflow.

## Stack

| Component | Technology | Location |
|---|---|---|
| API and authorization | Go 1.26.6 | `backend/` |
| Persistence | PostgreSQL | `backend/migrations/` |
| Administration console | Next.js 16, React 19, TypeScript, Tailwind CSS 4 | `admin/` |
| Windows SDK | C++, WinHTTP, DPAPI | `backend/sdk/cpp/` |
| API reference | OpenAPI | [docs/openapi.yaml](docs/openapi.yaml) |

## Architecture

```text
Desktop application ── publishable key + user/device flow ──┐
Developer backend ──── scoped secret key ──────────────────┼─→ KeyStar API → PostgreSQL
Admin console ──────── admin session + MFA / RBAC ──────────┘
```

| API surface | Caller | Authority |
|---|---|---|
| `/v1/auth/*`, `/v1/device/*`, `/v1/me` | Desktop client | Publishable credential for login/device verification; user session for protected operations |
| `/v1/server/*` | Your trusted backend | Scoped secret credential |
| `/v1/admin/*` | Administration console | Admin session, CSRF checks, MFA, and permissions |

Application context and verified credentials define the authorization boundary. A publishable key identifies the application; it does not grant a user's license or administrative permissions.

## Security model

- Client credentials (`ks_pk_*`) and server credentials (`ks_sk_*`) have separate roles. Never embed server secrets or private signing keys in a distributed application.
- Hardware attributes are normalized and HMAC-protected server-side for matching. Device policy controls TPM requirements, match thresholds, and rebinding.
- Administrative actions use permission checks and audit records. Users, devices, licenses, and sessions can be restricted or revoked within their application.
- Application signing keys are encrypted at rest. Keep database backups paired with backups of the external encryption-key ring.
- TPM key possession is a device proof, not evidence that the operating system is uncompromised.

Two application authentication profiles are available:

| Profile | Session model | Protected requests |
|---|---|---|
| `legacy` (default) | Bearer access tokens with refresh-token lifecycle | Bearer user session |
| `proof_bound` | Exactly 600-second Ed25519 tokens bound to a TPM P-256 key | DPoP proof per request; no refresh or bearer fallback |

The server selects the profile. See the [proof-bound contract](docs/PROOF_BOUND_APPLICATIONS.md) for key binding, replay rejection, activation, and rollback behavior.

## Quick start

### 1. Prepare the backend

Install Go 1.26.6, PostgreSQL 15 or later, and Node.js 22.12+ from the 22.x line with npm. Create a development database and set its connection string below. Docker Desktop is also needed for the repository's isolated integration tests.

```powershell
git clone https://github.com/Starhaxor/KeyStar.git
cd KeyStar/backend
Copy-Item .env.example .env
go run ./cmd/server keygen
```

Edit [backend/.env.example](backend/.env.example)'s copied `.env` file before continuing:

| Settings | Value |
|---|---|
| `DATABASE_URL` | Your PostgreSQL development database connection |
| `LICENSE_HMAC_KEY`, `HARDWARE_HMAC_KEY`, `ADMIN_SESSION_SECRET`, `ADMIN_BOOTSTRAP_TOKEN` | A separate cryptographically random value of at least 32 bytes for each |
| `ED25519_PRIVATE_KEY` | Private key from `keygen`, kept only on the backend |
| `ADMIN_MFA_ENCRYPTION_KEY` | Base64 encoding of an independently generated 32-byte key |
| `APPLICATION_KEY_ENCRYPTION_KEYS` | `1=<base64-of-another-random-32-byte-key>` |
| `APPLICATION_KEY_ACTIVE_VERSION` | `1` for the key-ring entry above |
| `LICENSE_ISSUER`, `LICENSE_AUDIENCE`, `PRODUCT` | Values for your deployment and client policy |

Keep `.env` private. The example's `ADMIN_COOKIE_SECURE=false` is for local HTTP only; production requires HTTPS, secure cookies, and the exact console origin in `ADMIN_ALLOWED_ORIGIN`.

### 2. Start the API

The backend reads process environment variables. From `KeyStar/backend`, load your local file, apply migrations, and start the service:

```powershell
Get-Content .env | ForEach-Object {
  if ($_ -match '^([^#=]+)=(.*)$') {
    [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
  }
}
go run ./cmd/server migrate up
go run ./cmd/server serve
```

The default API address is `http://localhost:8080`. Check `/status` and `/readyz` before connecting clients.

### 3. Open the console

In a second terminal, from the repository root:

```powershell
cd admin
npm ci
npm run dev
```

Open `http://localhost:3000`. For another API address, configure `NEXT_PUBLIC_API_URL` before starting or building the console.

On a fresh database, enter your `ADMIN_BOOTSTRAP_TOKEN` to create the first owner, then complete mandatory MFA enrollment. Public root setup closes after the first administrator exists.

Create an organization and application, add a product/plan, provision a user and license, and issue a publishable client credential.

## Application integration

For login and device verification, a client supplies its application ID and publishable key:

```http
X-KeyStar-App: <application-uuid>
Authorization: Bearer <publishable-key>
Content-Type: application/json
```

After successful user and device verification, use the resulting user session according to the application's profile. The bearer header above authenticates the publishable credential; it is not a bearer-session fallback for proof-bound applications.

For server-side provisioning or management, call `/v1/server/*` with the application ID and a scoped `ks_sk_*` credential held by your backend. Grant only the permissions that integration needs.

Use the [OpenAPI reference](docs/openapi.yaml) for request schemas and the [platform/SDK architecture](docs/KEYSTAR_PLATFORM_SDK_ARCHITECTURE.md) for integration details.

## StarLoader + KeyStar

[StarLoader](https://github.com/Starhaxor/StarLoader) supplies a Qt/C++ Windows client with TPM signing, token validation, and authenticated profile handling. Its authentication layer can be adapted to an ImGui frontend; it does not ship a ready-made ImGui SDK.

1. Deploy KeyStar migrations and code, then confirm exactly one active application signing key. Existing applications may need `server signing-keys backfill`.
2. Configure `PUBLIC_SCHEME` and `PUBLIC_HOST` for the exact public API origin.
3. Build StarLoader with matching application/product IDs, publishable key, signing public-key ring, API origin, and current/staged TLS pins.
4. Enable `proof_bound` for that application and run native login, expiry, revocation, and DPoP replay-rejection checks.

StarLoader keeps tokens in memory and requires DPoP for protected requests. It does not use KeyStar's legacy refresh/storage flow. Follow the [activation guide](docs/PROOF_BOUND_APPLICATIONS.md) and [StarLoader build guide](https://github.com/Starhaxor/StarLoader#quick-start); a skipped live test is not production verification.

## Development and verification

Backend checks, from `backend/`:

```powershell
go test ./internal/...
go vet ./...
go build ./cmd/server
```

Integration checks, from the repository root:

```powershell
docker compose up -d db
.\scripts\test-integration.ps1
```

These tests use the dedicated `keystar_test` database and reset its schema. Keep development and production data separate from it.

Console checks, from `admin/`:

```powershell
npm test
npm run lint
npm run build
npm run e2e:install
npm run e2e
```

The browser suite requires Docker and the isolated test database. Windows SDK builds require CMake and Visual Studio C++ tools; see [backend/sdk/cpp](backend/sdk/cpp).

## License

Released under the [MIT License](LICENSE). Anyone may use, modify, distribute, and sell this software, including in commercial projects, provided the copyright and permission notice is retained. Third-party components remain subject to their own licenses.

The administration console includes TailAdmin code under its existing [MIT notice](admin/LICENSE).
