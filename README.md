# KeyStar

**A self-hosted, multi-application platform for identity, licensing, device
trust, and operational security.**

KeyStar is built for desktop applications that need more than a serial-key
check. It resolves every request to an application, authenticates the caller,
verifies the user and entitlement, proves the device where required, and issues
short-lived session tokens for subsequent operations.

StarLoader is the first application connected to KeyStar. It is not a special
backend code path: it uses the same application, credential, device, and
session model available to every future KeyStar customer application.

> **Project status:** actively developed. This repository contains the Go +
> PostgreSQL backend and the Next.js administration console.

---

## Table of contents

- [Why KeyStar](#why-keystar)
- [What is included](#what-is-included)
- [Architecture](#architecture)
- [Security model](#security-model)
- [How KeyStar compares](#how-keystar-compares)
- [Quick start](#quick-start)
- [Application integration](#application-integration)
- [Administration workflow](#administration-workflow)
- [Repository layout](#repository-layout)
- [Development and verification](#development-and-verification)
- [Roadmap](#roadmap)

---

## Why KeyStar

Most licensing systems answer one question: **is this license key valid?**

For a modern desktop product, that is not enough. A secure and operable system
also needs to answer:

- Is the request scoped to the intended application and organization?
- Does the desktop client hold only a publishable credential, never a server
  secret?
- Is the user active, entitled, and not banned?
- Is the current device known, trusted, or suspicious?
- Can an operator revoke sessions, reset a device, or ban one compromised
  machine without affecting another application?
- Is every administrative action attributable afterwards?

KeyStar treats those questions as one coherent flow rather than a collection of
unrelated endpoints.

```text
Desktop client
  └─ Application ID + publishable key
       └─ user authentication
            └─ device challenge and verification
                 └─ short-lived user access token
                      └─ authenticated application operations

Developer backend / automation
  └─ Application ID + scoped secret key
       └─ user, license, device, webhook, and variable operations

Administration console
  └─ admin session + CSRF + MFA + RBAC
       └─ application-scoped operational control
```

---

## What is included

| Area | Capability |
| --- | --- |
| Multi-tenant platform | Organizations and applications form the isolation boundary for users, licenses, devices, sessions, products, plans, variables, webhooks, bans, and audit records. |
| Credential model | `ks_pk_*` publishable keys are for distributed clients. `ks_sk_*` secret keys are for developer-controlled servers and are limited by scopes. |
| User authentication | Password authentication, tokenized sessions, refresh-token lifecycle controls, account states, and user operations. |
| Licensing | Product and plan management, entitlement issuance, expiration, revocation, device limits, and administrator-facing license operations. |
| Device trust | Challenge-based device verification, TPM public-key support, multi-signal hardware matching, configurable device policy, reset and revoke workflows. |
| Hardware privacy | Hardware attributes are normalized and HMAC-protected server-side; the admin console shows only safe presence and status information. |
| Moderation | Account bans and device/HWID bans, durations, lift workflow, history, and application-scoped enforcement. |
| Operations | MFA, RBAC, CSRF protection, audit logs, security events, session revocation, webhooks, and configurable variables. |
| Data integrity | PostgreSQL migrations, UUIDv7 identifiers, tenant-aware queries, and bounded domain/service layers. |

---

## Architecture

```text
                              ┌──────────────────────────────────────┐
                              │               KeyStar                │
                              │        Go API + PostgreSQL            │
                              └──────────────────┬───────────────────┘
                                                 │
                          application resolver + credential verification
                                                 │
           ┌─────────────────────┬───────────────┼───────────────┬─────────────────────┐
           │                     │               │               │                     │
      StarLoader            Customer App A  Customer App B    Admin Console      Developer Backend
           │                     │               │               │                     │
      publishable key      publishable key  publishable key   MFA + RBAC      scoped secret key
           │                     │               │               │                     │
      user + device flow   user + device flow               operations        server automation
```

### API surfaces

KeyStar intentionally separates public client, developer server, and console
authority. One credential type must not silently act as another.

| Surface | Intended caller | Authentication |
| --- | --- | --- |
| `/v1/auth/*`, `/v1/device/*`, `/v1/me` | Desktop or client SDK | `X-KeyStar-App`, publishable key, then an end-user token where applicable |
| `/v1/server/*` | Developer-controlled backend or automation | `X-KeyStar-App`, scoped secret key |
| `/v1/admin/*` | KeyStar administration console | Admin session cookie, CSRF validation, MFA policy, and RBAC |

### Application resolution

An application is the primary security boundary. The server resolves it from
the request header and credential—not from a client-provided JSON field.

```http
X-KeyStar-App: <application-uuid>
Authorization: Bearer <credential-or-session-token>
```

This design allows the same email address to represent separate users in two
different applications while keeping their devices, licenses, sessions, and
moderation records isolated.

---

## Security model

### Credential separation

| Credential | Prefix | Where it belongs | Typical permissions |
| --- | --- | --- | --- |
| Publishable key | `ks_pk_test_*`, `ks_pk_live_*` | Desktop, mobile, or browser client | Login, registration, device verification, public configuration |
| Secret key | `ks_sk_test_*`, `ks_sk_live_*` | Developer backend, CI/CD, private job runner | User management, licensing, devices, variables, webhooks |

**A secret key must never be embedded in a desktop executable, mobile app,
browser bundle, installer, or public repository.** A publishable key is not a
secret; security comes from the user authentication, device proof, entitlement
checks, short-lived tokens, scopes, rate limits, and application isolation that
surround it.

### Device identity and hardware privacy

KeyStar does not reduce device identity to one exposed serial number. The
device workflow can combine:

- TPM public-key proof where hardware supports it;
- SMBIOS UUID;
- motherboard serial;
- BIOS serial;
- system-disk serial;
- Machine GUID; and
- an aggregate fingerprint.

Raw values are processed server-side. HMAC-derived representations are used for
matching so the dashboard can operate without becoming a raw hardware-ID data
export tool. Device policy can require TPM, set match thresholds, control
automatic rebinding, and limit changes over time.

### Session lifecycle

The intended session model is:

```text
login request
  → pending authentication / challenge
  → device verification
  → access token + refresh token
  → token refresh rotation
  → explicit revoke, expiration, or security response
```

Administrative tools can revoke user sessions, revoke a device registration,
reset a user’s devices, disable a user, ban a user, or ban a particular device.
These actions are application-scoped.

### Administrative security

The console is not an alternative server API. It is protected with an admin
session, CSRF defenses, MFA support, permission-based RBAC, audit logs, and
security-event recording. Server-to-server integrations should use scoped secret
keys instead of console cookies.

---

## How KeyStar compares

KeyStar operates in the same problem space as **KeyAuth**, **Cryptolens**, and
**LicenseSpring**: application licensing, customer access, and device-bound
activation. The comparison below is intentionally specific and balanced.

KeyStar’s differentiator is not “every feature of every licensing platform.”
Its focus is a source-controlled, self-hosted control plane where application
isolation, user sessions, device trust, and operations are designed together.

| Comparison area | KeyStar | KeyAuth | Cryptolens | LicenseSpring |
| --- | --- | --- | --- | --- |
| Primary model | Application-scoped identity, licensing, devices, and sessions | Application users, subscriptions, and HWID controls | License keys, products, activations, and machine codes | Products, licenses, entitlements, hardware IDs, and management APIs |
| Isolation boundary | Explicit application context on every protected request | Application-oriented dashboard model | Product and license-key model | Product code and license/user model |
| Desktop credential boundary | Publishable client credential; secret key restricted to server operations | Application API workflow | Client SDK or Web API access-token workflow | License API signing/verifying key workflow |
| Device trust | TPM-capable challenge proof plus multiple HMAC-protected hardware signals | HWID checks and reset workflow | Machine-code activation and configurable activations | Hardware ID and node-locking workflows |
| Operational controls | MFA, RBAC, audit logs, security events, user/device moderation | Dashboard user/HWID operations | Management API, activation management, SDK ecosystem | Management API, licensing operations, events/webhooks |
| Deployment posture | Repository-controlled Go + PostgreSQL deployment | Managed product | Managed product, with documented enterprise deployment options | Managed product |

### Where KeyStar is stronger

- **One application context across the stack.** Authentication, device
  verification, licensing, admin operations, webhooks, and variables are
  organized around the same application boundary.
- **A stricter client/server secret boundary.** Publishable and secret
  credentials are separate types with scopes, helping prevent accidental use of
  a management credential in a distributed client.
- **Device privacy by design.** The operational model is built around safe
  hardware matching and TPM-capable proof rather than presenting raw HWID data
  to administrators.
- **Self-hosted operational control.** Teams can inspect the implementation,
  control their PostgreSQL data plane, customize deployment, and keep their
  internal operations close to the product.
- **Modern dashboard operations.** The console groups applications, users,
  licenses, devices, device policy, account bans, device bans, sessions,
  credentials, variables, webhooks, and security functions in one workspace.

### Where competitors may be a better fit

- **Broad, production-ready SDK coverage:** Cryptolens documents SDKs across
  several languages and platforms.
- **Offline, floating, consumption, or enterprise licensing:** Cryptolens and
  LicenseSpring document mature options for those licensing models.
- **Fast managed-service adoption:** KeyAuth, Cryptolens, and LicenseSpring can
  be appropriate when operating a backend is not desired.

Choose KeyStar when you need a platform you can deploy and evolve with your own
application architecture. Choose a managed vendor when their specific licensing
model, SDK portfolio, support agreement, or hosted operations better matches the
project.

Official competitor references: [KeyAuth user and HWID management](https://docs.keyauth.cc/dashboard/app/users), [KeyAuth login API](https://docs.keyauth.cc/api/getting-started/login), [Cryptolens Web API](https://help.cryptolens.io/basics/webapi), [Cryptolens client SDKs](https://help.cryptolens.io/libraries/index), [LicenseSpring concepts](https://docs.licensespring.com/getting-started/basic-concepts), and [LicenseSpring API overview](https://licensespring.com/api).

---

## Quick start

### Prerequisites

- Go 1.24 or later
- PostgreSQL 15 or later
- Node.js 20 or later
- npm

### 1. Configure the backend

```powershell
cd backend
Copy-Item .env.example .env
```

Set the required values in `backend/.env`:

```text
DATABASE_URL
LICENSE_HMAC_KEY
HARDWARE_HMAC_KEY
ED25519_PRIVATE_KEY
LICENSE_ISSUER
LICENSE_AUDIENCE
PRODUCT
ADMIN_SESSION_SECRET
```

The backend reads environment variables rather than loading `.env` itself. For
local PowerShell development, load the file into the current process first:

```powershell
Get-Content .env | ForEach-Object {
  if ($_ -match '^([^#=]+)=(.*)$') {
    [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
  }
}
```

Apply the database schema and start the service:

```powershell
go run ./cmd/server migrate up
go run ./cmd/server serve
```

The API listens on `http://localhost:8080` by default.

### 2. Create the first administrator

Use the same terminal session, with the environment loaded:

```powershell
go run ./cmd/server admin create-admin --email admin@example.com --role owner
```

The password is requested securely through the terminal. In production, set
`ADMIN_COOKIE_SECURE=true` and configure `ADMIN_ALLOWED_ORIGIN` for the actual
console origin.

### 3. Run the administration console

```powershell
cd ..\admin
npm install
npm run dev
```

Open `http://localhost:3000`, sign in, create or select an organization, then
create an application. All application resources in the console follow that
active application selection.

---

## Application integration

### Desktop application or SDK

A distributed application should contain only:

```text
Application ID
Publishable key (ks_pk_live_...)
```

The client supplies those values during login and device verification. Once the
device flow succeeds, the client receives the user session tokens required for
subsequent protected operations.

```http
X-KeyStar-App: <application-uuid>
Authorization: Bearer ks_pk_live_...
Content-Type: application/json
```

The client must not treat a publishable key as proof of user authorization. It
is an application credential. User authorization begins only after KeyStar has
authenticated the user and completed the required device verification.

### Developer backend and automation

For license provisioning, user administration, webhook administration, device
operations, and private variables, use a scoped secret credential from your own
backend:

```http
X-KeyStar-App: <application-uuid>
Authorization: Bearer ks_sk_live_...
```

Grant only the scopes required by that integration. A service that only lists
users should receive `users.read`, not a broad write credential. Rotate and
revoke credentials through the console when ownership or deployment changes.

### StarLoader

StarLoader follows the desktop integration model:

1. Configure a KeyStar application in the console.
2. Generate a publishable credential for the client.
3. Configure the StarLoader client with the application ID and publishable key.
4. Authenticate the user, complete device verification, and store the issued
   tokens in the platform’s secure client storage.
5. Use user tokens for all protected application operations.

No legacy StarLoader-only backend contract or desktop secret is required for
this flow.

---

## Administration workflow

1. **Create or select an organization.** It owns one or more applications.
2. **Create an application.** The application becomes the resource and security
   boundary.
3. **Create products and plans.** Define the commercial entitlement structure.
4. **Create credentials.** Generate a publishable key for clients and scoped
   secret keys for trusted server integrations.
5. **Operate users and licenses.** Create users, issue or revoke licenses,
   reset passwords, revoke sessions, and inspect entitlement state.
6. **Manage devices.** Inspect trusted registrations, change device policy,
   revoke a device, or reset a user’s registrations.
7. **Moderate safely.** Use account bans for account-level enforcement and
   device/HWID bans for a specific registered device, with a reason and optional
   expiration.
8. **Integrate.** Configure variables and webhooks, then inspect audit and
   security records during operations.

---

## Repository layout

```text
admin/                         Next.js administration console
backend/
  cmd/server/                  serve | migrate | admin | keygen commands
  internal/httpapi/            public client API and common HTTP layer
  internal/httpapi/adminapi/   /v1/admin console handlers
  internal/httpapi/serverapi/  /v1/server machine-to-machine handlers
  internal/service/            authentication, device, session business logic
  internal/store/              PostgreSQL repositories and migration runner
  internal/security/           tokens, passwords, HMACs, signatures
  migrations/                  ordered PostgreSQL migrations
docs/                          architecture and SDK reference documents
```

---

## Development and verification

### Backend

```powershell
cd backend
go test ./internal/...
go build ./cmd/server
```

To run integration tests, provide an isolated PostgreSQL database through
`TEST_DATABASE_URL`:

```powershell
$env:TEST_DATABASE_URL = "postgres://postgres:postgres@localhost:5432/keystar_test?sslmode=disable"
go test ./tests/...
```

### Administration console

```powershell
cd admin
npm test
npm run lint
npm run build
```

---

## Roadmap

- Independently packaged C++ SDK and reference clients
- Improved webhook-delivery observability and replay tooling
- Offline lease and signed-license workflows
- Additional SDKs for C#, Rust, and Python
- Public OpenAPI description and external developer guide
- External beta hardening: rate limiting, monitoring, and deployment guidance

## Further reading

- [KeyStar Platform and SDK Architecture](docs/KEYSTAR_PLATFORM_SDK_ARCHITECTURE.md)
- [Database migrations](backend/migrations)

---

**KeyStar is not a StarLoader backend. StarLoader is the first application that
uses KeyStar.**
