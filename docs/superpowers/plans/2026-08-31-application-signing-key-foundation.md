# KeyStar Application Signing-Key Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provision one independently encrypted Ed25519 signing key for every KeyStar application and expose a tested backend provider that later token-profile work can use without changing current token issuance yet.

**Architecture:** A dedicated security component generates and envelope-encrypts Ed25519 seeds with versioned AES-256-GCM keys. PostgreSQL owns key lifecycle state and atomically creates an application's first active key in the same transaction as the application; an explicit idempotent CLI backfills existing applications. A read-only admin endpoint exposes public metadata, while private material remains reachable only through an application-scoped signing provider.

**Tech Stack:** Go 1.26.6, PostgreSQL 15+, pgx v5, Ed25519, AES-256-GCM, existing KeyStar admin API and test harness.

**Spec:** `docs/superpowers/specs/2026-08-29-per-application-signing-keys-design.md` and `docs/superpowers/specs/2026-08-31-starloader-full-reauth-device-proof-design.md`

## Global Constraints

- Preserve all unrelated modified and untracked files in the KeyStar and StarLoader worktrees.
- Do not switch login, device verification, refresh, or bearer verification to the new keys in this foundation plan; token-profile migration is a separate independently testable delivery.
- `APPLICATION_KEY_ENCRYPTION_KEYS` uses exact entries such as `1=<standard-base64>,2=<standard-base64>`; decoded keys are exactly 32 bytes.
- `APPLICATION_KEY_ACTIVE_VERSION` is a positive integer present in the configured key map.
- Persist only the encrypted 32-byte Ed25519 seed, a fresh 12-byte GCM nonce, the public key, and lifecycle metadata.
- AES-GCM additional authenticated data binds context version, application ID, `kid`, algorithm, and encryption-key version.
- Never log or return encryption keys, plaintext seeds, ciphertext, nonces, access tokens, API credentials, or decrypted private-key material.
- Application creation and initial active-key insertion are one PostgreSQL transaction.
- Existing applications receive independent keys only through the explicit idempotent `server signing-keys backfill` command.
- Admin responses expose public-key metadata only and remain protected by existing cookie authentication, MFA-enrollment, application RBAC, and CORS rules.

---

### Task 1: Parse the Versioned Key-Encryption Configuration

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `backend/.env.example`

**Interfaces:**
- Produces: `Config.ApplicationKeyEncryptionKeys map[int][]byte`
- Produces: `Config.ApplicationKeyActiveVersion int`
- Produces: `parseVersionedEncryptionKeys(string) (map[int][]byte, error)`

- [ ] **Step 1: Write failing configuration tests**

Add table-driven tests that require the two settings and reject duplicate versions, zero or negative versions, malformed standard Base64, decoded values other than 32 bytes, and an active version absent from the map:

```go
func TestLoadParsesApplicationKeyEncryptionKeys(t *testing.T) {
    setRequiredEnvironment(t)
    first := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
    second := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32))
    t.Setenv("APPLICATION_KEY_ENCRYPTION_KEYS", "1="+first+",2="+second)
    t.Setenv("APPLICATION_KEY_ACTIVE_VERSION", "2")

    configuration, err := Load()
    if err != nil {
        t.Fatal(err)
    }
    if configuration.ApplicationKeyActiveVersion != 2 || !bytes.Equal(configuration.ApplicationKeyEncryptionKeys[2], bytes.Repeat([]byte{0x22}, 32)) {
        t.Fatalf("application signing-key configuration = %#v", configuration)
    }
}
```

- [ ] **Step 2: Run the focused tests and verify failure**

Run: `cd backend; go test ./internal/config -run ApplicationKey -count=1`

Expected: FAIL because the new fields and parser do not exist.

- [ ] **Step 3: Implement strict parsing and secret separation**

Add the fields, require both settings, parse with `strconv.Atoi` and `base64.StdEncoding.Strict()`, deep-copy decoded values, and include every decoded key in the existing no-secret-reuse check:

```go
type Config struct {
    // existing fields
    ApplicationKeyEncryptionKeys map[int][]byte
    ApplicationKeyActiveVersion  int
}

func parseVersionedEncryptionKeys(value string) (map[int][]byte, error) {
    keys := make(map[int][]byte)
    for _, entry := range strings.Split(value, ",") {
        pair := strings.SplitN(strings.TrimSpace(entry), "=", 2)
        if len(pair) != 2 {
            return nil, errors.New("APPLICATION_KEY_ENCRYPTION_KEYS has invalid syntax")
        }
        version, err := strconv.Atoi(pair[0])
        if err != nil || version <= 0 {
            return nil, errors.New("APPLICATION_KEY_ENCRYPTION_KEYS contains an invalid version")
        }
        decoded, err := base64.StdEncoding.Strict().DecodeString(pair[1])
        if err != nil || len(decoded) != 32 {
            return nil, errors.New("APPLICATION_KEY_ENCRYPTION_KEYS values must decode to 32 bytes")
        }
        if _, duplicate := keys[version]; duplicate {
            return nil, errors.New("APPLICATION_KEY_ENCRYPTION_KEYS contains a duplicate version")
        }
        keys[version] = append([]byte(nil), decoded...)
    }
    if len(keys) == 0 {
        return nil, errors.New("APPLICATION_KEY_ENCRYPTION_KEYS is empty")
    }
    return keys, nil
}
```

Document non-functional examples in `.env.example` with blank values; do not add a sample usable key.

- [ ] **Step 4: Run config tests**

Run: `cd backend; go test ./internal/config -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the configuration boundary**

```bash
git add backend/internal/config/config.go backend/internal/config/config_test.go backend/.env.example
git commit -m "feat(config): add application signing-key encryption ring"
```

### Task 2: Add the Application-Key Cipher and Generator

**Files:**
- Create: `backend/internal/security/application_key_cipher.go`
- Create: `backend/internal/security/application_key_cipher_test.go`
- Create: `backend/internal/domain/application_signing_key.go`

**Interfaces:**
- Consumes: `map[int][]byte` and active version from Task 1.
- Produces: `security.NewApplicationKeyCipher(keys map[int][]byte, activeVersion int, random io.Reader) (*ApplicationKeyCipher, error)`
- Produces: `(*ApplicationKeyCipher).Generate(applicationID string) (domain.NewApplicationSigningKey, error)`
- Produces: `(*ApplicationKeyCipher).Decrypt(record domain.ApplicationSigningKey) (ed25519.PrivateKey, error)`

- [ ] **Step 1: Define the domain record and write failing cipher tests**

Use these exact domain shapes:

```go
type ApplicationSigningKeyStatus string

const (
    ApplicationSigningKeyPending  ApplicationSigningKeyStatus = "pending"
    ApplicationSigningKeyActive   ApplicationSigningKeyStatus = "active"
    ApplicationSigningKeyRetiring ApplicationSigningKeyStatus = "retiring"
    ApplicationSigningKeyRevoked  ApplicationSigningKeyStatus = "revoked"
)

type ApplicationSigningKey struct {
    ID, KID, ApplicationID, Algorithm string
    PublicKey, EncryptedPrivateKey, EncryptionNonce []byte
    EncryptionKeyVersion int
    Status ApplicationSigningKeyStatus
    CreatedAt time.Time
    ActivatedAt, RetireAt, RevokedAt *time.Time
}

type NewApplicationSigningKey struct {
    KID, ApplicationID, Algorithm string
    PublicKey, EncryptedPrivateKey, EncryptionNonce []byte
    EncryptionKeyVersion int
    Status ApplicationSigningKeyStatus
    ActivatedAt *time.Time
}
```

Tests must prove two generated records differ, `kid` begins with `ksk_`, public keys are 32 bytes, nonces are 12 bytes, decrypted keys sign correctly, and decryption fails after modifying application ID, `kid`, algorithm, version, nonce, ciphertext, or public key.

- [ ] **Step 2: Run the cipher tests and verify failure**

Run: `cd backend; go test ./internal/security -run ApplicationKeyCipher -count=1`

Expected: FAIL because the cipher does not exist.

- [ ] **Step 3: Implement AES-256-GCM with bound AAD**

Generate a 32-byte seed and 16 random `kid` bytes from the injected reader. Seal the seed without prepending the nonce because the schema stores the nonce separately:

```go
func applicationKeyAAD(applicationID, kid, algorithm string, version int) []byte {
    return []byte(fmt.Sprintf("keystar:application-signing-key:v1\x00%s\x00%s\x00%s\x00%d", applicationID, kid, algorithm, version))
}

func (cipher *ApplicationKeyCipher) Generate(applicationID string) (domain.NewApplicationSigningKey, error) {
    seed := make([]byte, ed25519.SeedSize)
    kidBytes := make([]byte, 16)
    nonce := make([]byte, 12)
    if _, err := io.ReadFull(cipher.random, seed); err != nil { return domain.NewApplicationSigningKey{}, err }
    if _, err := io.ReadFull(cipher.random, kidBytes); err != nil { return domain.NewApplicationSigningKey{}, err }
    if _, err := io.ReadFull(cipher.random, nonce); err != nil { return domain.NewApplicationSigningKey{}, err }
    kid := "ksk_" + base64.RawURLEncoding.EncodeToString(kidBytes)
    privateKey := ed25519.NewKeyFromSeed(seed)
    publicKey := append([]byte(nil), privateKey[ed25519.SeedSize:]...)
    sealed := cipher.activeAEAD.Seal(nil, nonce, seed, applicationKeyAAD(applicationID, kid, "Ed25519", cipher.activeVersion))
    clear(seed)
    return domain.NewApplicationSigningKey{KID: kid, ApplicationID: applicationID, Algorithm: "Ed25519", PublicKey: publicKey, EncryptedPrivateKey: sealed, EncryptionNonce: nonce, EncryptionKeyVersion: cipher.activeVersion, Status: domain.ApplicationSigningKeyPending}, nil
}
```

`Decrypt` selects the recorded version, authenticates the same AAD, reconstructs the private key, and constant-time compares its derived public key with the persisted public key before returning it.

- [ ] **Step 4: Run security tests**

Run: `cd backend; go test ./internal/security -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the cryptographic component**

```bash
git add backend/internal/domain/application_signing_key.go backend/internal/security/application_key_cipher.go backend/internal/security/application_key_cipher_test.go
git commit -m "feat(security): encrypt per-application signing seeds"
```

### Task 3: Add Migration 20 and Persistence Constraints

**Files:**
- Create: `backend/migrations/000020_application_signing_keys.up.sql`
- Create: `backend/migrations/000020_application_signing_keys.down.sql`
- Modify: `backend/internal/store/migrations.go`
- Modify: `backend/internal/store/migrations_test.go`
- Modify: `backend/tests/integration/schema_migrations_test.go`

**Interfaces:**
- Consumes: domain lifecycle values from Task 2.
- Produces: constrained `application_signing_keys` table and `application_signing_keys_one_active_idx`.

- [ ] **Step 1: Write failing migration-ledger and constraint tests**

Update the latest-migration assertion to version 20. Add integration cases that reject wrong key sizes, invalid status, pending timestamps, active-without-activation, retiring-without-retirement, revoked-without-revocation, and a second active key for one application.

- [ ] **Step 2: Run focused migration tests and verify failure**

Run: `cd backend; go test ./internal/store -run Migration -count=1`

Expected: FAIL because migration 20 is absent.

- [ ] **Step 3: Create the constrained schema migration**

The up migration must use these core constraints:

```sql
create table application_signing_keys (
    id uuid primary key default starloader_uuid_v7()
        check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    kid text not null unique check (kid ~ '^ksk_[A-Za-z0-9_-]{22}$'),
    application_id uuid not null references applications(id) on delete cascade,
    algorithm text not null check (algorithm = 'Ed25519'),
    public_key bytea not null check (octet_length(public_key) = 32),
    encrypted_private_key bytea not null check (octet_length(encrypted_private_key) = 48),
    encryption_nonce bytea not null check (octet_length(encryption_nonce) = 12),
    encryption_key_version integer not null check (encryption_key_version > 0),
    status text not null check (status in ('pending','active','retiring','revoked')),
    created_at timestamptz not null default clock_timestamp(),
    activated_at timestamptz,
    retire_at timestamptz,
    revoked_at timestamptz,
    check (
        (status = 'pending' and activated_at is null and retire_at is null and revoked_at is null) or
        (status = 'active' and activated_at is not null and retire_at is null and revoked_at is null) or
        (status = 'retiring' and activated_at is not null and retire_at is not null and revoked_at is null) or
        (status = 'revoked' and revoked_at is not null)
    )
);

create unique index application_signing_keys_one_active_idx
    on application_signing_keys(application_id) where status = 'active';
```

The down migration drops only this table. Register migration 20 after migration 19.

- [ ] **Step 4: Run unit and dedicated PostgreSQL migration tests**

Run: `cd backend; go test ./internal/store -count=1`

Run after `docker compose up -d db`: `./scripts/test-integration.ps1`

Expected: both PASS; the integration runner must target only `keystar_test`.

- [ ] **Step 5: Commit the schema**

```bash
git add backend/migrations/000020_application_signing_keys.* backend/internal/store/migrations.go backend/internal/store/migrations_test.go backend/tests/integration/schema_migrations_test.go
git commit -m "feat(db): add application signing-key lifecycle"
```

### Task 4: Implement Key Storage and Atomic Application Provisioning

**Files:**
- Create: `backend/internal/store/application_signing_keys.go`
- Modify: `backend/internal/store/applications.go`
- Create: `backend/internal/service/application_provisioning.go`
- Create: `backend/internal/service/application_provisioning_test.go`
- Modify: `backend/tests/integration/repository_test.go`

**Interfaces:**
- Consumes: `(*ApplicationKeyCipher).Generate(applicationID)` from Task 2.
- Produces: `service.NewApplicationProvisioner(repository ApplicationProvisioningRepository, keys *security.ApplicationKeyCipher, now func() time.Time) *ApplicationProvisioner`.
- Produces: `CreateApplicationWithSigningKey(context.Context, domain.NewApplication, func(string) (domain.NewApplicationSigningKey, error)) (*domain.Application, error)`.
- Produces: `ListApplicationsWithoutSigningKey(context.Context) ([]string, error)`.
- Produces: `CreateInitialSigningKey(context.Context, string, domain.NewApplicationSigningKey) (bool, error)` where `bool=false` means an active key already exists.
- Produces: `ListApplicationSigningKeys(context.Context, string) ([]domain.ApplicationSigningKey, error)` and `FindActiveApplicationSigningKey(context.Context, string) (*domain.ApplicationSigningKey, error)`.

- [ ] **Step 1: Write failing service and integration tests**

The service fake must assert that the repository passes the database-assigned application ID to the key factory. PostgreSQL tests must prove application creation and active-key insertion commit together, a factory error rolls back the application, two applications receive different public keys, and idempotent initial-key creation never replaces an existing active key.

- [ ] **Step 2: Run tests and verify failure**

Run: `cd backend; go test ./internal/service -run ApplicationProvision -count=1`

Expected: FAIL because the provisioner and repository methods do not exist.

- [ ] **Step 3: Implement the narrow provisioning service**

```go
type ApplicationProvisioningRepository interface {
    CreateApplicationWithSigningKey(context.Context, domain.NewApplication, func(string) (domain.NewApplicationSigningKey, error)) (*domain.Application, error)
    ListApplicationsWithoutSigningKey(context.Context) ([]string, error)
    CreateInitialSigningKey(context.Context, string, domain.NewApplicationSigningKey) (bool, error)
}

type ApplicationProvisioner struct {
    repository ApplicationProvisioningRepository
    keys *security.ApplicationKeyCipher
    now func() time.Time
}

func NewApplicationProvisioner(repository ApplicationProvisioningRepository, keys *security.ApplicationKeyCipher, now func() time.Time) *ApplicationProvisioner {
    if now == nil { now = time.Now }
    return &ApplicationProvisioner{repository: repository, keys: keys, now: now}
}

func (service *ApplicationProvisioner) Create(ctx context.Context, input domain.NewApplication) (*domain.Application, error) {
    return service.repository.CreateApplicationWithSigningKey(ctx, input, service.newActiveKey)
}

func (service *ApplicationProvisioner) newActiveKey(applicationID string) (domain.NewApplicationSigningKey, error) {
    key, err := service.keys.Generate(applicationID)
    if err != nil { return domain.NewApplicationSigningKey{}, err }
    activatedAt := service.now().UTC()
    key.Status = domain.ApplicationSigningKeyActive
    key.ActivatedAt = &activatedAt
    return key, nil
}

func (service *ApplicationProvisioner) Backfill(ctx context.Context) (int, error) {
    ids, err := service.repository.ListApplicationsWithoutSigningKey(ctx)
    if err != nil { return 0, err }
    created := 0
    for _, id := range ids {
        key, err := service.newActiveKey(id)
        if err != nil { return created, err }
        inserted, err := service.repository.CreateInitialSigningKey(ctx, id, key)
        if err != nil { return created, err }
        if inserted { created++ }
    }
    return created, nil
}
```

`CreateApplicationWithSigningKey` begins a transaction, inserts and scans the application, calls the factory with its real ID, inserts the active key, then commits. Factory or insert failure rolls the whole transaction back.

- [ ] **Step 4: Run service and PostgreSQL tests**

Run: `cd backend; go test ./internal/service -count=1`

Run after the dedicated database is healthy: `./scripts/test-integration.ps1`

Expected: PASS.

- [ ] **Step 5: Commit atomic provisioning**

```bash
git add backend/internal/store/application_signing_keys.go backend/internal/store/applications.go backend/internal/service/application_provisioning.go backend/internal/service/application_provisioning_test.go backend/tests/integration/repository_test.go
git commit -m "feat(applications): provision encrypted signing keys atomically"
```

### Task 5: Add the Application-Scoped Signing Provider

**Files:**
- Create: `backend/internal/security/application_signer.go`
- Create: `backend/internal/security/application_signer_test.go`

**Interfaces:**
- Consumes: `FindActiveApplicationSigningKey(context.Context, string)` from Task 4.
- Consumes: `(*ApplicationKeyCipher).Decrypt(domain.ApplicationSigningKey)` from Task 2.
- Produces: `(*ApplicationSigner).Sign(context.Context, string, []byte) (SignedMessage, error)`.

- [ ] **Step 1: Write failing provider tests**

Use a fake repository and real generated encrypted keys. Assert the returned `kid`, signature, and public key verify with Ed25519; application A cannot select B's key; missing, revoked, malformed, public/private-inconsistent, and authentication-tag-failing records return one sanitized `ErrApplicationSigningUnavailable`.

- [ ] **Step 2: Run focused tests and verify failure**

Run: `cd backend; go test ./internal/security -run ApplicationSigner -count=1`

Expected: FAIL because the provider does not exist.

- [ ] **Step 3: Implement the provider without caching plaintext keys**

```go
var ErrApplicationSigningUnavailable = errors.New("application signing unavailable")

type ActiveApplicationKeyRepository interface {
    FindActiveApplicationSigningKey(context.Context, string) (*domain.ApplicationSigningKey, error)
}

type SignedMessage struct {
    KID string
    PublicKey []byte
    Signature []byte
}

func (signer *ApplicationSigner) Sign(ctx context.Context, applicationID string, message []byte) (SignedMessage, error) {
    record, err := signer.repository.FindActiveApplicationSigningKey(ctx, applicationID)
    if err != nil || record.Status != domain.ApplicationSigningKeyActive { return SignedMessage{}, ErrApplicationSigningUnavailable }
    privateKey, err := signer.cipher.Decrypt(*record)
    if err != nil { return SignedMessage{}, ErrApplicationSigningUnavailable }
    signature := ed25519.Sign(privateKey, message)
    clear(privateKey)
    return SignedMessage{KID: record.KID, PublicKey: append([]byte(nil), record.PublicKey...), Signature: signature}, nil
}
```

Do not add this provider to current JWT issuance in this task.

- [ ] **Step 4: Run security tests**

Run: `cd backend; go test ./internal/security -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the provider**

```bash
git add backend/internal/security/application_signer.go backend/internal/security/application_signer_test.go
git commit -m "feat(security): add application-scoped signing provider"
```

### Task 6: Wire Provisioning, Backfill, and Public-Metadata Admin API

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/cmd/server/main_test.go`
- Modify: `backend/internal/httpapi/types.go`
- Modify: `backend/internal/httpapi/adminapi/admin_applications.go`
- Modify: `backend/internal/httpapi/adminapi/admin_lifecycle_test.go`
- Modify: `docs/openapi.yaml`

**Interfaces:**
- Consumes: `ApplicationProvisioner` and signing-key read store from Tasks 4-5.
- Produces: CLI `server signing-keys backfill`.
- Produces: `GET /v1/admin/applications/{application_id}/signing-keys` returning public metadata only.

- [ ] **Step 1: Write failing CLI and admin API tests**

Add parse-command coverage for exactly `signing-keys backfill` and rejection of other arguments. Add handler tests proving `applications.read` is required, the requested application is honored rather than the selected default, disabled applications remain readable for recovery, and JSON contains only:

```json
{
  "ok": true,
  "items": [{
    "kid": "ksk_AAAAAAAAAAAAAAAAAAAAAA",
    "algorithm": "Ed25519",
    "status": "active",
    "public_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
    "created_at": "2026-08-31T12:00:00Z",
    "activated_at": "2026-08-31T12:00:00Z",
    "retire_at": null,
    "revoked_at": null
  }]
}
```

Tests must assert ciphertext, nonce, encryption version, and private material are absent.

- [ ] **Step 2: Run focused tests and verify failure**

Run: `cd backend; go test ./cmd/server ./internal/httpapi/adminapi -run 'SigningKey|SigningKeys' -count=1`

Expected: FAIL because the command and route are absent.

- [ ] **Step 3: Wire dependencies and handlers**

Extend `httpapi.AdminConfig` without growing `AdminConsoleStore`:

```go
type AdminApplicationProvisioner interface {
    Create(context.Context, domain.NewApplication) (*domain.Application, error)
}

type AdminApplicationSigningKeyReader interface {
    ListApplicationSigningKeys(context.Context, string) ([]domain.ApplicationSigningKey, error)
}

type AdminConfig struct {
    // existing fields
    ApplicationProvisioner AdminApplicationProvisioner
    ApplicationSigningKeys AdminApplicationSigningKeyReader
}
```

Change application creation to call `ApplicationProvisioner.Create`. The read route checks `PermApplicationsRead`, verifies the named application exists, maps only public metadata, and never calls the decrypting signer.

The backfill command loads configuration and the database, constructs `ApplicationKeyCipher` and `ApplicationProvisioner`, runs `Backfill`, and prints only `application signing-key backfill created N key(s)`.

- [ ] **Step 4: Run backend unit tests**

Run: `cd backend; go test ./internal/... ./cmd/server -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the integration surface**

```bash
git add backend/cmd/server backend/internal/httpapi/types.go backend/internal/httpapi/adminapi/admin_applications.go backend/internal/httpapi/adminapi/admin_lifecycle_test.go docs/openapi.yaml
git commit -m "feat(api): expose application signing-key foundation"
```

### Task 7: Complete Documentation and the Foundation Verification Gate

**Files:**
- Modify: `README.md`
- Modify: `docs/KEYSTAR_PLATFORM_SDK_ARCHITECTURE.md`
- Modify: `docs/superpowers/plans/2026-08-31-application-signing-key-foundation.md`

**Interfaces:**
- Consumes: every foundation interface from Tasks 1-6.
- Produces: deployment order and recorded verification evidence for the next token-profile plan.

- [ ] **Step 1: Document the exact safe deployment order**

Document this sequence without including usable secret values:

```text
1. Back up PostgreSQL and encryption-key configuration.
2. Set APPLICATION_KEY_ENCRYPTION_KEYS and APPLICATION_KEY_ACTIVE_VERSION.
3. Deploy the binary containing migration 20 and the backfill command.
4. Run server migrate up.
5. Run server signing-keys backfill.
6. Verify every application has exactly one active public key through the admin API.
7. Start serving traffic; token issuance remains on the legacy global key until the next plan.
```

State that losing every configured version makes its encrypted seeds unrecoverable and that database backups must be paired with external key-ring backups.

- [ ] **Step 2: Run formatting and static verification**

Run:

```powershell
cd backend
gofmt -w internal/config/*.go internal/domain/application_signing_key.go internal/security/application_key_cipher.go internal/security/application_key_cipher_test.go internal/security/application_signer.go internal/security/application_signer_test.go internal/store/application_signing_keys.go internal/store/applications.go internal/service/application_provisioning.go internal/service/application_provisioning_test.go internal/httpapi/types.go internal/httpapi/adminapi/admin_applications.go internal/httpapi/adminapi/admin_lifecycle_test.go cmd/server/main.go cmd/server/main_test.go
go vet ./...
go test -p 1 ./...
```

Expected: all commands exit 0. Integration tests may skip only when `TEST_DATABASE_URL` is absent; the next step supplies it.

- [ ] **Step 3: Run the dedicated PostgreSQL integration gate**

Run from the repository root:

```powershell
docker compose up -d db
./scripts/test-integration.ps1
```

Expected: PASS against the dedicated `keystar_test` database.

- [ ] **Step 4: Inspect secret handling and worktree isolation**

Run:

```powershell
rg -n "EncryptedPrivateKey|EncryptionNonce|ApplicationKeyEncryptionKeys" backend/internal/httpapi admin docs/openapi.yaml
git diff --check
git status --short
```

Expected: no private-field response mapping; no whitespace errors; pre-existing unrelated modified/untracked paths remain present and uncommitted.

- [ ] **Step 5: Record the verified commands and commit documentation**

Append a `Verification Evidence` section to this plan with the exact command, date, and exit result from Steps 2-4, then commit only the documentation files:

```bash
git add README.md docs/KEYSTAR_PLATFORM_SDK_ARCHITECTURE.md docs/superpowers/plans/2026-08-31-application-signing-key-foundation.md
git commit -m "docs: document application signing-key foundation"
```

## Plan Self-Review

- Spec coverage in this delivery: versioned key-encryption configuration, AES-GCM AAD binding, independent key generation, constrained storage, atomic new-application provisioning, idempotent existing-application backfill, private-key consistency checks, application-scoped signing provider, public metadata API, and deployment documentation.
- Deliberately deferred to separate plans: JWT `kid` issuance and verification, legacy-key cutoff, signing-key staging/activation/retirement/revocation UI, recent-MFA key lifecycle operations, session policy, access-session revocation, DPoP request proof, StarLoader trust-key migration, VMProtect, SPKI pinning, and Authenticode.
- Type consistency: all tasks use `domain.ApplicationSigningKey`, `domain.NewApplicationSigningKey`, `security.ApplicationKeyCipher`, `service.ApplicationProvisioner`, and `security.ApplicationSigner` with the signatures declared above.

## Verification Evidence

Date: 2026-09-01

### Formatting and static verification

- Windows-safe formatting command — exit 0, formatting 18 explicitly resolved files:

  ```powershell
  $goFiles = @(
    Get-ChildItem -LiteralPath 'backend\internal\config' -Filter '*.go' -File
    Get-Item -LiteralPath 'backend\internal\domain\application_signing_key.go'
    Get-Item -LiteralPath 'backend\internal\security\application_key_cipher.go'
    Get-Item -LiteralPath 'backend\internal\security\application_key_cipher_test.go'
    Get-Item -LiteralPath 'backend\internal\security\application_signer.go'
    Get-Item -LiteralPath 'backend\internal\security\application_signer_test.go'
    Get-Item -LiteralPath 'backend\internal\store\application_signing_keys.go'
    Get-Item -LiteralPath 'backend\internal\store\applications.go'
    Get-Item -LiteralPath 'backend\internal\service\application_provisioning.go'
    Get-Item -LiteralPath 'backend\internal\service\application_provisioning_test.go'
    Get-Item -LiteralPath 'backend\internal\httpapi\types.go'
    Get-Item -LiteralPath 'backend\internal\httpapi\adminapi\admin_applications.go'
    Get-Item -LiteralPath 'backend\internal\httpapi\adminapi\admin_lifecycle_test.go'
    Get-Item -LiteralPath 'backend\cmd\server\main.go'
    Get-Item -LiteralPath 'backend\cmd\server\main_test.go'
    Get-Item -LiteralPath 'backend\cmd\e2e-fixture\main.go'
    Get-Item -LiteralPath 'backend\cmd\e2e-fixture\main_test.go'
  ) | Sort-Object -Property FullName -Unique
  foreach ($goFile in $goFiles) {
    gofmt -w $goFile.FullName
    if ($LASTEXITCODE -ne 0) { throw "gofmt failed for $($goFile.FullName)" }
  }
  ```

- `cd backend; $env:TEST_DATABASE_URL = 'postgres://keystar_test:keystar_test@127.0.0.1:55432/keystar_test?sslmode=disable'; go vet ./...; go test -p 1 ./... -count=1` — exit 0; every Go package passed, including black-box and PostgreSQL integration packages.
- `cd backend; govulncheck ./...` — exit 0; no called vulnerabilities found.
- `cd backend; gosec -quiet -severity high -confidence high ./...` — exit 0 with no findings.

### Dedicated PostgreSQL integration gate

- `docker compose up -d db` — exit 0; the dedicated PostgreSQL container was healthy on port 55432.
- `./scripts/test-integration.ps1` — exit 0; `github.com/starloader/backend/tests/integration` passed in 19.166 seconds against `keystar_test`.
- `cd backend; $env:TEST_DATABASE_URL = 'postgres://keystar_test:keystar_test@127.0.0.1:55432/keystar_test?sslmode=disable'; go run ./cmd/e2e-fixture reset; go run ./cmd/e2e-fixture seed` followed by a database count query — exit 0; the clean fixture contained 3 applications, 3 active application signing keys, and 0 applications without an active key.

### Admin and browser verification

- `cd admin; npm audit --omit=dev --audit-level=high; npm test; npm run lint; npm run build` — exit 0; 0 vulnerabilities, 32 test files and 103 tests passed, lint passed, and the production build completed.
- `cd admin; npm run e2e` — exit 1 after backend and admin readiness; 6 tests passed and `lifecycle-onboarding.spec.ts` failed because license creation completed and redirected to the overview before the test observed the one-time-secret dialog. A focused rerun reproduced the same single failure. The original missing `APPLICATION_KEY_ENCRYPTION_KEYS` startup failure is fixed.
- The local Docker Desktop engine stopped once during verification because of a stale model-runner socket. `docker desktop disable model-runner; docker desktop start` restored the engine; subsequent PostgreSQL, backend, and browser startup checks reached the application normally.

### Secret handling and worktree checks

- `rg -n "EncryptedPrivateKey|EncryptionNonce|ApplicationKeyEncryptionKeys|APPLICATION_KEY_ENCRYPTION_KEYS|APPLICATION_KEY_ACTIVE_VERSION" backend/internal/httpapi admin docs/openapi.yaml README.md .github/workflows/verify.yml` — exit 0; private fields appear only in the admin handler test fixture and are not response-mapped. Required variable references are present in the quick start, browser test environment, and documentation gate.
- The README release-gate PowerShell check from `.github/workflows/verify.yml` — exit 0 with both application-key variables included.
- `git diff --check` — exit 0; no whitespace errors.
- Initial `git rev-parse --short HEAD; git status --short` — `e038340` and a clean tracked worktree before this final fix wave.
