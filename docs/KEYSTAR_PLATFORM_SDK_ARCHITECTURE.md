# KeyStar Platform & SDK Architecture

**Document type:** Technical Architecture / Implementation Specification  
**Project:** KeyStar  
**Primary first-party client:** StarLoader  
**Primary SDK target:** C++  
**Backend:** Go + PostgreSQL  
**Admin:** Next.js  
**Identifier standard:** UUIDv7  
**Scope:** Authentication, Licensing, HWID / Device Binding, Sessions, Application SDK, Multi-Tenant Platform

---

## 1. Amaç

KeyStar'ın amacı yalnızca StarLoader için çalışan özel bir backend olmak değil, KeyAuth benzeri ancak daha modern, daha esnek ve güvenlik sınırları daha net tanımlanmış bir **authentication + licensing + device identity platformu** olmaktır.

Bu mimaride:

- StarLoader, KeyStar platformuna bağlı ilk gerçek uygulamadır.
- KeyStar backend hiçbir uygulamaya hard-coded bağlı değildir.
- Her geliştirici veya ekip kendi `Application` kaydını oluşturabilir.
- Her uygulama kendi kullanıcılarını, lisanslarını, cihazlarını, oturumlarını, değişkenlerini, webhooklarını ve güvenlik politikalarını izole biçimde yönetir.
- Bir geliştirici C++, C#, Rust, Go, Python veya JavaScript SDK aracılığıyla uygulamasını KeyStar'a bağlayabilir.
- Desktop/game client içinde **server secret bulunmaz**.
- Client uygulamalar yalnızca `Application ID + Publishable Key` kullanır.
- Yönetim / otomasyon / server-to-server istekler `Secret Key` ile yapılır.
- HWID tek bir değişkene bağlı değildir; cihaz kimliği çoklu sinyaller ve mümkün olan ortamlarda TPM tabanlı cihaz anahtarıyla doğrulanır.
- StarLoader mevcut login/HWID akışını KeyStar'ın public client API'si üzerinden kullanır.

Hedef ürün modeli:

```text
KeyStar Platform
│
├── Organization A
│   ├── Application: StarLoader
│   ├── Application: Game A
│   └── Application: Tool B
│
├── Organization B
│   ├── Application: Launcher X
│   └── Application: Desktop App Y
│
└── Organization C
    └── Application: Private Client
```

---

# 2. Temel Tasarım İlkeleri

## 2.1 KeyStar uygulamadan bağımsız olmalıdır

Backend içinde aşağıdakine benzer sabit uygulama mantıkları bulunmamalıdır:

```go
const Product = "starloader"
```

veya:

```go
if app == "StarLoader" {
    // special login
}
```

Bunun yerine her request bir `ApplicationContext` ile çalışmalıdır.

```go
type ApplicationContext struct {
    ApplicationID  uuid.UUID
    OrganizationID uuid.UUID
    CredentialID   uuid.UUID
    Environment    string
    Scopes         []string
}
```

Bu context request middleware tarafından çözülür.

---

## 2.2 Tenant izolasyonu varsayılan olmalıdır

Aşağıdaki tüm domain objeleri doğrudan veya dolaylı şekilde bir application'a bağlı olmalıdır:

- Users
- Licenses
- Products / Plans
- Devices
- Sessions
- Bans
- Variables
- Webhooks
- Security Events
- Audit Logs
- API Credentials
- Rate-limit policies
- Application settings

Bir uygulamadaki kayıt başka application sorgusunda asla görünmemelidir.

Örnek:

```text
Application A
mustafa@example.com -> banned

Application B
mustafa@example.com -> active
```

Bu iki kullanıcı birbirinden bağımsızdır.

---

# 3. Mevcut KeyStar Yapısının Korunacak Bölümleri

Mevcut projedeki aşağıdaki yapılar korunmalı ve genişletilmelidir:

```text
backend/internal/
├── admin
├── config
├── domain
├── httpapi
├── security
├── service
└── store
```

Mevcut güvenlik ve domain altyapısından korunacak parçalar:

- UUIDv7 üretimi ve database constraintleri
- Argon2id password hashing
- Session/token altyapısı
- Device challenge sistemi
- TPM public key saklama
- TPM public key SHA-256
- SMBIOS UUID HMAC
- Motherboard Serial HMAC
- BIOS Serial HMAC
- System Disk Serial HMAC
- Machine GUID HMAC
- Aggregate Fingerprint HMAC
- Admin RBAC
- Admin MFA / TOTP
- Recovery codes
- Security events
- Audit logs
- Existing Organizations
- Existing Applications

Bunların yeniden yazılması yerine platform seviyesine çıkarılması hedeflenmelidir.

---

# 4. Platform Veri Modeli

## 4.1 Organizations

Organization, KeyStar üzerindeki ana tenant sahibidir.

```sql
organizations
-------------
id uuid primary key
name text not null
slug text not null
status text not null
created_at timestamptz
updated_at timestamptz
```

Önerilen status değerleri:

```text
active
suspended
disabled
```

---

## 4.2 Organization Members

```sql
organization_members
--------------------
id uuid primary key
organization_id uuid not null
admin_account_id uuid not null
role_id uuid not null
status text not null
created_at timestamptz
updated_at timestamptz
```

Organization üyeleri platformun yönetici kullanıcılarıdır. Bunlar son kullanıcı `users` tablosuyla karıştırılmamalıdır.

---

# 5. Applications

Application, platformun en önemli izolasyon sınırıdır.

```sql
applications
------------
id uuid primary key
organization_id uuid not null
name text not null
slug text not null
status text not null
environment_mode text not null default 'separate'
created_at timestamptz not null
updated_at timestamptz not null
```

Önerilen status:

```text
active
maintenance
suspended
disabled
```

Örnek:

```text
Organization: MevaTech
Application: StarLoader
Application ID: 019c....
Slug: starloader
Status: active
```

StarLoader artık özel backend davranışı değil, normal bir KeyStar application olur.

---

# 6. Application Credentials

Bu bölüm KeyStar'ın SDK olarak güvenli kullanılmasının temelidir.

## 6.1 Credential türleri

İki ana credential tipi kullanılmalıdır:

### Publishable Key

Client-side SDK için.

```text
ks_pk_test_...
ks_pk_live_...
```

Masaüstü uygulamasında bulunabilir.

Yetkileri sınırlıdır:

```text
auth.login
auth.register
auth.refresh
license.activate
device.verify
variables.read_public
```

### Secret Key

Sadece geliştiricinin backend'i / server-to-server işlemleri için.

```text
ks_sk_test_...
ks_sk_live_...
```

Desktop executable içine kesinlikle gömülmemelidir.

Örnek server scope'ları:

```text
users.read
users.write
licenses.read
licenses.write
devices.read
devices.write
sessions.read
sessions.revoke
variables.read
variables.write
webhooks.read
webhooks.write
analytics.read
```

---

## 6.2 Credential tablosu

```sql
application_credentials
-----------------------
id uuid primary key
application_id uuid not null
environment text not null
credential_type text not null
name text not null
key_prefix text not null
key_hash bytea not null
scopes text[] not null
status text not null
last_used_at timestamptz
expires_at timestamptz
created_at timestamptz not null
revoked_at timestamptz
```

Önerilen `credential_type`:

```text
publishable
secret
```

Önerilen `environment`:

```text
test
live
```

---

## 6.3 Key formatı

Öneri:

```text
ks_pk_live_<credential-id-short>_<random-secret>
ks_sk_live_<credential-id-short>_<random-secret>
```

Örneğin:

```text
ks_pk_live_8DX3bY_MdJqJ43....
ks_sk_live_3KR7wQ_FxN9xK2....
```

Backend key içindeki credential locator bölümünden kaydı bulur ve secret kısmını hash compare ile doğrular.

Tam key database'de saklanmaz.

---

## 6.4 Secret storage

Secret oluşturulurken:

1. CSPRNG ile en az 256-bit random secret üretilir.
2. Kullanıcıya yalnızca ilk oluşturma anında plaintext gösterilir.
3. DB'ye plaintext kaydedilmez.
4. DB'ye doğrulama için hash kaydedilir.
5. Dashboard sonradan yalnızca prefix gösterir.
6. Secret kaybolursa reveal edilmez, yenisi oluşturulur.

Dashboard örneği:

```text
Server Secret
ks_sk_live_3KR7wQ_•••••••••••••••••••

Created: 18 Aug 2026
Last used: 2 minutes ago

[Rotate] [Revoke]
```

---

# 7. Application Request Resolution

Her client request aşağıdaki bilgileri taşımalıdır:

```http
X-KeyStar-App: <application-id>
Authorization: Bearer ks_pk_live_...
```

Server request:

```http
X-KeyStar-App: <application-id>
Authorization: Bearer ks_sk_live_...
```

Middleware sırası:

```text
Request
  ↓
Request ID
  ↓
IP normalization / trusted proxy
  ↓
Rate limiter
  ↓
Application ID resolver
  ↓
Credential parser
  ↓
Credential hash validation
  ↓
Credential status validation
  ↓
Environment validation
  ↓
Application status validation
  ↓
Organization status validation
  ↓
Scope validation
  ↓
ApplicationContext
  ↓
Handler / Service
```

---

# 8. Go Middleware Tasarımı

```go
type AppPrincipal struct {
    ApplicationID  uuid.UUID
    OrganizationID uuid.UUID
    CredentialID   uuid.UUID
    CredentialType string
    Environment    string
    Scopes         map[string]struct{}
}
```

Context helper:

```go
func AppPrincipalFromContext(ctx context.Context) (AppPrincipal, bool)
```

Middleware:

```go
func RequireApplicationCredential(
    verifier ApplicationCredentialVerifier,
    requiredType CredentialType,
    requiredScopes ...string,
) func(http.Handler) http.Handler
```

Böylece endpoint bazında authorization tanımlanabilir.

Örnek:

```go
router.Handle(
    "POST /v1/auth/login",
    RequireApplicationCredential(
        verifier,
        CredentialPublishable,
        "auth.login",
    )(loginHandler),
)
```

---

# 9. User Modeli

Mevcut `users` tablosundaki application scoping zorunlu hale getirilmelidir.

```sql
users
-----
id uuid primary key
application_id uuid not null
email text not null
username text
password_hash text not null
status text not null
notes text not null default ''
ban_reason text not null default ''
banned_at timestamptz
ban_expires_at timestamptz
created_at timestamptz
updated_at timestamptz
```

Unique constraint:

```sql
unique(application_id, email)
```

Global:

```sql
unique(email)
```

olmamalıdır.

Aynı e-mail farklı uygulamalarda bulunabilir.

---

# 10. Products ve Plans

Mevcut `licenses.product` string alanını uzun vadede normalize etmek önerilir.

## Products

```sql
products
--------
id uuid primary key
application_id uuid not null
name text not null
slug text not null
status text not null
created_at timestamptz
updated_at timestamptz
```

## Plans

```sql
plans
-----
id uuid primary key
product_id uuid not null
name text not null
code text not null
level integer not null
max_devices integer not null
default_duration_seconds bigint
metadata jsonb not null default '{}'
status text not null
created_at timestamptz
updated_at timestamptz
```

Örnek:

```text
Application: StarLoader

Product:
StarLoader Client

Plans:
- Free
- Basic
- Premium
- Lifetime
```

---

# 11. License Modeli

```sql
licenses
--------
id uuid primary key
application_id uuid not null
user_id uuid
product_id uuid not null
plan_id uuid
license_hmac text not null
status text not null
max_devices integer not null
expires_at timestamptz
activated_at timestamptz
created_at timestamptz
updated_at timestamptz
notes text not null default ''
metadata jsonb not null default '{}'
```

Status:

```text
pending
active
revoked
expired
suspended
```

License plaintext saklanmamalıdır.

DB'de yalnız HMAC/hash tutulmalıdır.

---

# 12. License Key Formatı

Örnek kullanıcı-facing format:

```text
KS-ABCD-EFGH-JKLM-NPQR
```

veya:

```text
STAR-8K3D-7PX2-4JQ9-MN8W
```

Key üretiminde yeterli entropy bulunmalıdır.

Backend:

```text
plaintext license
    ↓
HMAC-SHA-256(platform secret)
    ↓
license_hmac DB lookup
```

Plain license anahtarı database'e kaydedilmez.

---

# 13. HWID / Device Identity Tasarımı

KeyStar HWID sistemi tek bir seri numarasına bağlı olmamalıdır.

Cihaz kimliği şu sinyalleri destekler:

```text
TPM public key
TPM key hash
SMBIOS UUID
Motherboard serial
BIOS serial
System disk serial
Machine GUID
CPU-related stable metadata where appropriate
Aggregate fingerprint
```

Ancak ham değerler mümkün olduğu kadar server-side düz metin tutulmamalıdır.

---

# 14. Device Tablosu

```sql
devices
-------
id uuid primary key
application_id uuid not null
user_id uuid not null
license_id uuid not null

name text
platform text
architecture text

hardware_key_type text
hardware_public_key bytea
hardware_public_key_sha256 bytea

smbios_uuid_hmac text
motherboard_serial_hmac text
bios_serial_hmac text
system_disk_serial_hmac text
machine_guid_hmac text
fingerprint_hmac text not null

risk_score integer not null default 0
status text not null

first_seen_at timestamptz
last_seen_at timestamptz
created_at timestamptz
updated_at timestamptz
```

Status:

```text
pending
active
trusted
revoked
blocked
```

---

# 15. Device Enrollment Modeli

İlk login sırasında server doğrudan cihazı güvenilir kabul etmemelidir.

Önerilen flow:

```text
Client
  │
  ├─ login(email,password)
  │
Server
  │
  ├─ user doğrula
  ├─ license doğrula
  ├─ pending auth session oluştur
  ├─ random challenge üret
  │
Client
  │
  ├─ challenge'ı TPM/device private key ile imzala
  ├─ hardware fingerprint sinyallerini hazırla
  │
Server
  │
  ├─ signature verify
  ├─ device fingerprint compare
  ├─ risk score hesapla
  ├─ device policy uygula
  ├─ device bind / update
  └─ verified session oluştur
```

---

# 16. TPM Kullanımı

TPM mevcutsa KeyStar cihaz kimliğinin ana güven kökü olarak kullanılabilir.

İlk enrollment:

1. Client TPM içinde non-exportable key pair oluşturur.
2. Private key TPM dışına çıkarılmaz.
3. Public key KeyStar backend'e gönderilir.
4. Backend public key hashini cihaz kaydına bağlar.
5. Her kritik authentication challenge client tarafından TPM key ile imzalanır.

Avantaj:

- Kopyalanmış fingerprint tek başına yeterli olmaz.
- Private key başka makineye kolay taşınamaz.
- Replay attack'a karşı challenge nonce kullanılabilir.

Fallback:

TPM olmayan cihazlar tamamen reddedilmek zorunda değildir. Application policy şunlardan biri olabilir:

```text
TPM required
TPM preferred
TPM optional
```

---

# 17. HWID Scoring

Bir cihaz tek alan değiştiğinde otomatik ban olmamalıdır.

Örnek weighted scoring:

```text
TPM key match               45
Motherboard match           20
SMBIOS UUID match           10
BIOS serial match            8
System disk match            7
Machine GUID match           5
Other stable signals         5
-------------------------------
Total                       100
```

Örnek policy:

```text
80-100 = same device
60-79  = likely same device / step-up verification
40-59  = suspicious / rebind approval
0-39   = different device
```

Bu değerler application bazında configurable olabilir.

---

# 18. Application Device Policy

```sql
application_device_policies
---------------------------
id uuid primary key
application_id uuid unique not null
require_tpm boolean not null
min_match_score integer not null
step_up_score integer not null
allow_auto_rebind boolean not null
rebind_cooldown_seconds bigint not null
max_device_changes_per_30d integer not null
created_at timestamptz
updated_at timestamptz
```

---

# 19. Login Flow

Public API:

```http
POST /v1/auth/login
```

Headers:

```http
X-KeyStar-App: <application_id>
Authorization: Bearer ks_pk_live_...
Content-Type: application/json
```

Body:

```json
{
  "identifier": "mustafa@example.com",
  "password": "user-password",
  "device": {
    "fingerprint": "...",
    "platform": "windows",
    "architecture": "x86_64"
  }
}
```

Response:

```json
{
  "status": "device_verification_required",
  "session_id": "019c...",
  "challenge": "base64...",
  "expires_at": "2026-08-18T09:20:00Z"
}
```

---

# 20. Device Verification Flow

```http
POST /v1/device/verify
```

Body:

```json
{
  "session_id": "019c...",
  "challenge_signature": "base64...",
  "device_public_key": "base64...",
  "hardware": {
    "smbios_uuid": "...",
    "motherboard_serial": "...",
    "bios_serial": "...",
    "system_disk_serial": "...",
    "machine_guid": "..."
  }
}
```

Backend ham hardware değerlerini normalize eder ve keyed HMAC üretir.

Başarılı response:

```json
{
  "status": "authenticated",
  "access_token": "...",
  "refresh_token": "...",
  "expires_in": 900,
  "device": {
    "id": "019c...",
    "trusted": true,
    "score": 96
  }
}
```

---

# 21. Session ve Token Yaşam Döngüsü

Access token kısa ömürlü olmalıdır.

Öneri:

```text
Access token: 10-15 dakika
Refresh token: 7-30 gün
Device challenge: 2 dakika
Login pending session: 2-5 dakika
```

Refresh token DB'de plaintext tutulmamalıdır.

```sql
refresh_sessions
----------------
id uuid primary key
application_id uuid not null
user_id uuid not null
device_id uuid not null
token_hash bytea not null
status text not null
expires_at timestamptz not null
last_used_at timestamptz
created_at timestamptz
revoked_at timestamptz
```

Refresh token rotation kullanılmalıdır.

Her refresh işleminde yeni token oluşturulur, eski token invalid edilir.

Reuse detection yapılmalıdır.

---

# 22. Token Claims

JWT kullanılacaksa minimum claim yaklaşımı tercih edilmelidir.

Örnek:

```json
{
  "iss": "keystar",
  "sub": "user_uuid",
  "aud": "application_uuid",
  "sid": "session_uuid",
  "did": "device_uuid",
  "iat": 1787030000,
  "exp": 1787030900
}
```

Token içine email, HWID, lisans key gibi gereksiz hassas bilgiler koyulmamalıdır.

---

# 23. `/v1/me`

```http
GET /v1/me
Authorization: Bearer <access-token>
```

Response:

```json
{
  "user": {
    "id": "019c...",
    "email": "mustafa@example.com",
    "status": "active"
  },
  "license": {
    "id": "019c...",
    "status": "active",
    "plan": "premium",
    "expires_at": "2027-08-18T00:00:00Z"
  },
  "device": {
    "id": "019c...",
    "status": "trusted"
  }
}
```

---

# 24. Public Client API

Önerilen endpointler:

```text
POST /v1/auth/login
POST /v1/auth/register
POST /v1/auth/refresh
POST /v1/auth/logout

POST /v1/licenses/activate
GET  /v1/licenses/me

POST /v1/device/verify
GET  /v1/device/me

GET  /v1/me
GET  /v1/variables/public
```

Bunlar publishable credential + user session ile çalışır.

---

# 25. Server-to-Server API

Secret key gerektirir.

```text
GET    /v1/server/users
POST   /v1/server/users
GET    /v1/server/users/{id}
PATCH  /v1/server/users/{id}
DELETE /v1/server/users/{id}

GET    /v1/server/licenses
POST   /v1/server/licenses
GET    /v1/server/licenses/{id}
POST   /v1/server/licenses/{id}/revoke
POST   /v1/server/licenses/{id}/extend

GET    /v1/server/devices
POST   /v1/server/devices/{id}/revoke
POST   /v1/server/users/{id}/reset-devices

POST   /v1/server/users/{id}/ban
POST   /v1/server/users/{id}/unban

GET    /v1/server/sessions
POST   /v1/server/sessions/{id}/revoke

GET    /v1/server/variables
POST   /v1/server/variables
PATCH  /v1/server/variables/{id}
DELETE /v1/server/variables/{id}
```

---

# 26. Admin API ve Server API Ayrımı

Üç farklı authentication surface açıkça ayrılmalıdır:

```text
1. Admin Dashboard
   Admin JWT / Session + MFA + RBAC

2. Developer Server API
   ks_sk_live_...

3. End User Client API
   Application ID + ks_pk_live_... + user access token
```

Bunlar aynı authorization sistemiymiş gibi davranmamalıdır.

---

# 27. Variables

Mevcut global `variables.key unique` yaklaşımı multi-app sistem için değiştirilmelidir.

```sql
variables
---------
id uuid primary key
application_id uuid not null
key text not null
value text not null
visibility text not null
description text not null
created_at timestamptz
updated_at timestamptz

unique(application_id, key)
```

Visibility:

```text
public
authenticated
server_only
```

Örnek:

```text
Application: StarLoader
minimum_version = 1.4.0
maintenance = false
news_message = ...
```

---

# 28. Application Settings

```sql
application_settings
--------------------
application_id uuid primary key
allow_registration boolean
require_license boolean
require_device_verification boolean
require_tpm boolean
max_sessions_per_user integer
access_token_ttl_seconds integer
refresh_token_ttl_seconds integer
maintenance_message text
metadata jsonb
updated_at timestamptz
```

---

# 29. Bans

Ban mutlaka application-scoped olmalıdır.

```sql
bans
----
id uuid primary key
application_id uuid not null
user_id uuid
device_id uuid
ip_hash text
reason text not null
expires_at timestamptz
created_by_type text not null
created_by_id uuid
created_at timestamptz
revoked_at timestamptz
```

Ban tipleri:

```text
user
device
network
combined
```

---

# 30. Webhooks

Geliştiricinin kendi backend'ine event göndermek için.

```sql
webhooks
--------
id uuid primary key
application_id uuid not null
url text not null
secret_hash bytea not null
status text not null
events text[] not null
created_at timestamptz
updated_at timestamptz
```

Event örnekleri:

```text
user.created
user.login.succeeded
user.login.failed
user.banned
user.unbanned
license.created
license.activated
license.expired
license.revoked
device.bound
device.changed
device.revoked
security.suspicious_device
```

Webhook body:

```json
{
  "id": "evt_019c...",
  "type": "license.expired",
  "application_id": "019c...",
  "created_at": "2026-08-18T09:30:00Z",
  "data": {}
}
```

Webhook HMAC signature header:

```text
X-KeyStar-Signature
X-KeyStar-Timestamp
```

Replay protection için timestamp doğrulanmalıdır.

---

# 31. Audit Logs

Audit log immutable mantıkta tasarlanmalıdır.

```sql
audit_logs
----------
id uuid primary key
organization_id uuid
application_id uuid
actor_type text
actor_id uuid
action text
resource_type text
resource_id uuid
ip_hash text
user_agent text
metadata jsonb
created_at timestamptz
```

Örnek action:

```text
application.created
credential.created
credential.revoked
license.created
license.revoked
user.banned
device.reset
admin.role.changed
```

---

# 32. Security Events

Audit log ile security event aynı şey değildir.

Security event örnekleri:

```text
credential.invalid
credential.revoked_used
login.bruteforce
login.impossible_device_change
device.signature_invalid
device.score_low
refresh_token.reuse
rate_limit.exceeded
admin.mfa_failed
```

Severity:

```text
info
warning
critical
```

---

# 33. Rate Limiting

Rate limit yalnız IP bazlı olmamalıdır.

Kombine limiter:

```text
IP
Application ID
Credential ID
User ID
Endpoint group
```

Örnek:

```text
Login:
5 requests/minute/IP
20 requests/minute/application+IP

Refresh:
30 requests/minute/session

Server API:
300 requests/minute/secret-key
```

İleride plan bazlı limit uygulanabilir.

---

# 34. SDK Mimarisi

İlk resmi SDK C++ olmalıdır çünkü StarLoader bu SDK'yı gerçek ortamda test edecektir.

Repo önerisi:

```text
keystar-sdk-cpp/
├── include/
│   └── keystar/
│       ├── client.hpp
│       ├── auth.hpp
│       ├── device.hpp
│       ├── license.hpp
│       ├── session.hpp
│       ├── error.hpp
│       └── types.hpp
│
├── src/
│   ├── client.cpp
│   ├── auth.cpp
│   ├── device.cpp
│   ├── hwid_windows.cpp
│   ├── tpm_windows.cpp
│   ├── transport.cpp
│   ├── json.cpp
│   └── crypto.cpp
│
├── tests/
├── examples/
└── CMakeLists.txt
```

---

# 35. C++ SDK Public API

Hedef kullanım olabildiğince basit olmalıdır.

```cpp
#include <keystar/keystar.hpp>

keystar::Client client({
    .application_id = "019c...",
    .publishable_key = "ks_pk_live_...",
    .base_url = "https://api.keystar.dev"
});
```

Login:

```cpp
auto result = client.auth().login({
    .email = email,
    .password = password
});

if (!result) {
    showError(result.error().message());
    return;
}

const auto& session = result.value();
```

SDK kendi içinde:

1. HWID toplar.
2. TPM key oluşturur/yükler.
3. `/v1/auth/login` çağırır.
4. Challenge alır.
5. Challenge imzalar.
6. `/v1/device/verify` çağırır.
7. Access/refresh token yönetir.

Uygulama geliştiricisi bu düşük seviye akışı elle yazmak zorunda değildir.

---

# 36. C++ SDK Basitleştirilmiş API

```cpp
keystar::Client client({
    .application_id = APP_ID,
    .publishable_key = APP_KEY,
});

auto login = client.login(username, password);

if (!login.ok()) {
    std::cerr << login.error().message << '\n';
    return 1;
}

std::cout << "Welcome " << login.user().email << '\n';
```

---

# 37. License-only Kullanım

Bazı geliştiriciler username/password istemeyebilir.

SDK şu modu da desteklemelidir:

```cpp
auto result = client.activateLicense("KS-ABCD-EFGH-JKLM");
```

Bu durumda KeyStar gerekirse internal user identity oluşturabilir veya lisansı device-bound entitlement şeklinde çalıştırabilir.

Application setting:

```text
Auth mode:
- account_only
- license_only
- account_and_license
```

---

# 38. SDK Error Modeli

SDK string karşılaştırma gerektirmemelidir.

```cpp
enum class ErrorCode {
    InvalidCredentials,
    LicenseRequired,
    LicenseExpired,
    LicenseRevoked,
    DeviceLimitReached,
    DeviceRejected,
    DeviceVerificationFailed,
    ApplicationSuspended,
    Maintenance,
    RateLimited,
    NetworkError,
    ServerError,
    InvalidResponse
};
```

Örnek:

```cpp
if (result.error().code == keystar::ErrorCode::LicenseExpired) {
    openRenewPage();
}
```

---

# 39. Token Storage SDK Tarafı

SDK refresh token'ı düz text dosyada saklamamalıdır.

Windows:

```text
DPAPI / Credential Manager
```

macOS:

```text
Keychain
```

Linux:

```text
Secret Service / libsecret
```

Uygun secure storage yoksa developer açıkça opt-in etmedikçe persistent refresh token saklanmamalıdır.

---

# 40. StarLoader Entegrasyonu

StarLoader, KeyStar'ın özel bir backend modu değil, KeyStar SDK'nın ilk consumer'ı olmalıdır.

KeyStar DB:

```text
Organization: MevaTech
Application: StarLoader
Application ID: <UUIDv7>
Publishable Key: ks_pk_live_...
```

StarLoader Qt/C++ tarafı:

```cpp
keystar::Client keyStar({
    .application_id = STARLOADER_APP_ID,
    .publishable_key = STARLOADER_PUBLISHABLE_KEY,
});
```

Login button:

```cpp
void LoginWindow::onLoginClicked()
{
    auto result = keyStar.login(
        ui->emailEdit->text().toStdString(),
        ui->passwordEdit->text().toStdString()
    );

    if (!result.ok()) {
        showLoginError(result.error());
        return;
    }

    openMainWindow();
}
```

StarLoader hiçbir `ks_sk_...` key taşımaz.

---

# 41. StarLoader'da Kullanılacak Akış

```text
StarLoader.exe
   ↓
KeyStar C++ SDK
   ↓
collect device identity
   ↓
POST /v1/auth/login
   ↓
challenge
   ↓
TPM/device key signature
   ↓
POST /v1/device/verify
   ↓
access token
   ↓
GET /v1/me
   ↓
StarLoader main UI
```

---

# 42. StarLoader'ın Backend'e Özel Bağımlılığı Kaldırılmalı

StarLoader source içinde doğrudan custom HTTP endpoint kodları yerine SDK kullanılmalıdır.

Yanlış:

```cpp
http.post("/v1/auth/login", customJson);
```

Doğru:

```cpp
keyStar.login(email, password);
```

Bu sayede KeyStar API değiştiğinde StarLoader yalnız SDK update eder.

---

# 43. SDK Versioning

Semantic versioning:

```text
1.0.0
1.1.0
2.0.0
```

API header:

```text
X-KeyStar-SDK: cpp/1.0.0
```

Backend telemetry SDK versionlarını görebilir.

Application bazında minimum SDK version tanımlanabilir.

---

# 44. Diğer SDK'lar

C++ stabil olduktan sonra:

```text
keystar-cpp
keystar-csharp
keystar-rust
keystar-go
keystar-python
keystar-js
```

Tüm SDK'lar aynı API semantics kullanmalıdır.

---

# 45. C# Örneği

```csharp
var client = new KeyStarClient(new KeyStarOptions
{
    ApplicationId = "019c...",
    PublishableKey = "ks_pk_live_..."
});

var session = await client.Auth.LoginAsync(email, password);
```

---

# 46. Rust Örneği

```rust
let client = KeyStar::new(
    "019c...",
    "ks_pk_live_..."
);

let session = client.login(email, password).await?;
```

---

# 47. Go Server SDK

Server-to-server için ayrı package önerilir:

```go
client := keystar.NewServerClient(
    os.Getenv("KEYSTAR_SECRET_KEY"),
)

license, err := client.Licenses.Create(ctx, ...)
```

---

# 48. Application Dashboard

Yeni ana navigasyon:

```text
Organization

Applications
Team
Audit
Billing
Organization Settings
```

Application seçildiğinde:

```text
Overview
Users
Licenses
Products
Devices
Sessions
Bans
Variables
API Keys
Webhooks
Security
Logs
Settings
```

---

# 49. Application Overview

Kartlar:

```text
Total Users
Active Users
Active Licenses
Expiring Licenses
Bound Devices
Active Sessions
Failed Logins 24h
Security Alerts
API Requests
```

---

# 50. API Keys Dashboard

```text
Publishable Keys

Name          Environment   Last Used     Status
Desktop SDK   Live          2 min ago     Active
Test SDK      Test          yesterday     Active

Server Keys

Name          Environment   Scopes        Last Used
Backend       Live          8 scopes      1 min ago
CI            Live          2 scopes      3 days ago
```

Create modal:

```text
Name
Environment
Type
Scopes
Expiration
```

---

# 51. Credential Rotation

Rotation flow:

```text
Old Key = Active
New Key = Active

Deployment updated

Old Key = Revoked
```

Zero-downtime rotation desteklenmelidir.

Aynı application için birden fazla aktif credential bulunabilir.

---

# 52. Repository / Store Scoping

Mevcut repository metotları application ID almadan sorgu yapmamalıdır.

Yanlış:

```go
FindUserByEmail(ctx, email)
```

Doğru:

```go
FindUserByEmail(ctx, applicationID, email)
```

Yanlış:

```go
FindLicenseByUserAndProduct(ctx, userID, product)
```

Doğru:

```go
FindActiveLicense(ctx, applicationID, userID, productID)
```

Her query'de application boundary olmalıdır.

---

# 53. Defensive SQL Pattern

Örnek:

```sql
select id, email, password_hash, status
from users
where application_id = $1
  and email = $2
limit 1;
```

Update:

```sql
update devices
set status = 'revoked'
where id = $1
  and application_id = $2;
```

Sadece `id = $1` kullanmak tenant leakage riskidir.

---

# 54. Foreign Key Tasarımı

Mümkün olan yerlerde composite integrity kullanılabilir.

Örneğin:

```sql
unique(id, application_id)
```

ve child table:

```sql
foreign key (user_id, application_id)
references users(id, application_id)
```

Bu, yanlış application'a ait user/device ilişkisini database seviyesinde de engeller.

---

# 55. Migration Planı

Yeni migration sıralaması önerisi:

```text
000008_application_credentials
000009_application_scoping_hardening
000010_products_plans
000011_variables_application_scope
000012_device_policy
000013_refresh_sessions
000014_webhooks
000015_api_audit
000016_server_api
000017_application_settings
```

Mevcut migrationlar rewrite edilmemelidir.

Yeni migrationlar append edilmelidir.

---

# 56. Application Scoping Hardening Migration

Mevcut nullable application_id alanları:

```text
users.application_id
licenses.application_id
devices.application_id
auth_sessions.application_id
```

Backfill tamamlandıktan sonra:

```sql
alter table users alter column application_id set not null;
alter table licenses alter column application_id set not null;
alter table devices alter column application_id set not null;
alter table auth_sessions alter column application_id set not null;
```

Ardından unique indexler tenant-aware hale getirilmelidir.

---

# 57. Backward Compatibility

İlk rollout sırasında mevcut StarLoader client bir anda kırılmamalıdır.

Aşamalı geçiş:

```text
Phase A
Mevcut endpointler çalışır.
Default StarLoader application context otomatik atanır.

Phase B
Yeni SDK endpoint authentication aktif olur.
StarLoader yeni SDK build'e geçer.

Phase C
Legacy no-app requests warning log üretir.

Phase D
Legacy mode kapatılır.
Application ID + publishable key zorunlu olur.
```

---

# 58. Legacy Compatibility Middleware

Geçici:

```go
if appHeader == "" && legacyMode {
    principal = defaultStarLoaderPrincipal
}
```

Bu davranış yalnız migration dönemi için kullanılmalıdır.

Kalıcı olmamalıdır.

---

# 59. Threat Model

KeyStar şu saldırı sınıflarını düşünmelidir:

```text
Credential theft
Executable reverse engineering
Publishable key extraction
Secret key leakage
Brute-force login
Credential stuffing
Replay attacks
HWID spoofing
TPM absence / emulation
Session token theft
Refresh token reuse
API replay
Tenant ID tampering
Cross-tenant data access
SQL injection
Mass assignment
Webhook forgery
Admin account takeover
MFA bypass attempts
Rate-limit bypass
Clock skew abuse
```

---

# 60. Publishable Key Çalınırsa

Publishable key gizli kabul edilmemelidir.

Bir saldırgan executable içinden çıkarabilir.

Bu nedenle publishable key:

```text
Admin işlemi yapamaz
User oluşturamaz (policy izin vermiyorsa)
License üretemez
Ban kaldıramaz
Başka application verisi okuyamaz
Secret endpoint çağıramaz
```

Publishable key'in amacı **application identification + limited client permission** sağlamaktır; tam bir gizli anahtar değildir.

---

# 61. Secret Key Güvenliği

Secret key:

- Source control'a girmemeli.
- Client executable içine girmemeli.
- Loglara yazılmamalı.
- Error response içinde dönmemeli.
- Dashboard'da tekrar plaintext gösterilmemeli.
- Environment variable veya secret manager ile kullanılmalı.

---

# 62. Request Signing - Opsiyonel İleri Seviye

Server API için opsiyonel HMAC request signing eklenebilir.

Headers:

```text
X-KeyStar-Timestamp
X-KeyStar-Nonce
X-KeyStar-Signature
```

Canonical message:

```text
METHOD
PATH
TIMESTAMP
NONCE
SHA256(BODY)
```

Bu model özellikle yüksek güvenlik isteyen müşteriler için seçilebilir.

---

# 63. Client Attestation

Gerçekçi yaklaşım:

Desktop executable hiçbir zaman tamamen güvenilir değildir.

Bu nedenle SDK:

- Publishable key'i secret kabul etmez.
- Authentication kararını server verir.
- Device proof kullanır.
- Session kısa ömürlüdür.
- Server-side entitlement kontrolü yapılır.

İleri aşamada optional:

```text
Windows code signing metadata
Build ID
SDK integrity metadata
application version policy
```

kullanılabilir.

---

# 64. API Response Format

Standart response envelope önerilir.

Başarılı:

```json
{
  "data": {},
  "request_id": "req_..."
}
```

Hata:

```json
{
  "error": {
    "code": "LICENSE_EXPIRED",
    "message": "license expired"
  },
  "request_id": "req_..."
}
```

SDK `code` alanını enum'a dönüştürür.

---

# 65. Error Code Standardı

```text
INVALID_REQUEST
INVALID_APPLICATION
APPLICATION_DISABLED
APPLICATION_MAINTENANCE
INVALID_CREDENTIAL
CREDENTIAL_REVOKED
CREDENTIAL_EXPIRED
INSUFFICIENT_SCOPE
INVALID_CREDENTIALS
USER_DISABLED
USER_BANNED
LICENSE_REQUIRED
LICENSE_EXPIRED
LICENSE_REVOKED
DEVICE_LIMIT_REACHED
DEVICE_VERIFICATION_REQUIRED
DEVICE_VERIFICATION_FAILED
DEVICE_REVOKED
SESSION_EXPIRED
TOKEN_REUSE_DETECTED
RATE_LIMITED
SERVER_ERROR
```

---

# 66. API Versioning

Public API:

```text
/v1/...
```

Breaking changes:

```text
/v2/...
```

SDK version ile API version birbirinden ayrıdır.

```text
SDK 1.4.0 -> API v1
SDK 2.0.0 -> API v1 veya v2
```

---

# 67. Idempotency

Server API'de create işlemleri idempotency desteklemelidir.

```http
Idempotency-Key: <uuid>
```

Özellikle:

```text
Create License
Create User
Create Credential
Create Webhook
```

için faydalıdır.

---

# 68. SDK Retry Policy

SDK yalnız güvenli durumlarda retry yapmalıdır.

Retry:

```text
network timeout
502
503
504
```

Automatic retry yapılmaması gerekenler:

```text
401
403
license activation without idempotency
credential failure
```

Exponential backoff + jitter kullanılmalıdır.

---

# 69. Telemetry

KeyStar minimum telemetry:

```text
request count
request latency
error rate
login success rate
login failure rate
device verification success
license activation count
active sessions
credential usage
SDK versions
```

Hassas kullanıcı verileri metric label olarak kullanılmamalıdır.

---

# 70. Observability

Her request:

```text
request_id
application_id
credential_id
route
status_code
latency
```

ile loglanabilir.

Password, raw token, raw HWID, secret key kesinlikle loglanmamalıdır.

---

# 71. Admin RBAC

Organization seviyesinde roller:

```text
owner
admin
developer
support
viewer
```

Örnek permission:

```text
applications.read
applications.write
credentials.read
credentials.write
users.read
users.write
licenses.read
licenses.write
devices.read
devices.write
security.read
audit.read
webhooks.write
```

---

# 72. Application-Specific RBAC

İleride organization member yalnız belli application'lara erişebilir.

```sql
application_member_roles
------------------------
id uuid
application_id uuid
organization_member_id uuid
role_id uuid
```

Örneğin support çalışanı yalnız StarLoader kullanıcılarını görebilir.

---

# 73. Application Environments

Test/live ayrımı credential prefix ile başlamalıdır.

İleri model:

```text
Application
├── Test Environment
└── Live Environment
```

Test ortamındaki kullanıcı/lisans/device verileri live ile karışmamalıdır.

Uzun vadede `environment_id` ayrı tablo olarak modellenebilir.

---

# 74. Suggested Environment Model

```sql
application_environments
------------------------
id uuid primary key
application_id uuid not null
name text not null
mode text not null
created_at timestamptz

unique(application_id, name)
```

Örnek:

```text
test
live
staging
```

Büyük ölçekte domain tabloları `environment_id` ile scope edilebilir.

---

# 75. Security Headers

API:

```text
Strict-Transport-Security
X-Content-Type-Options: nosniff
Cache-Control: no-store
```

Admin web:

```text
Content-Security-Policy
frame-ancestors
Referrer-Policy
Permissions-Policy
```

---

# 76. TLS

Production API yalnız HTTPS.

Plain HTTP:

- production'da reddedilmeli.
- local development'ta allow edilebilir.

SDK default base URL HTTPS olmalıdır.

---

# 77. Password Policy

Passwordlar Argon2id ile hashlenmeye devam etmelidir.

Login enumeration engellemek için unknown-user path de benzer maliyetli dummy hash doğrulaması yapmalıdır.

Bu mevcut yaklaşım korunmalıdır.

---

# 78. MFA End User - Opsiyonel

İlk sürüm için zorunlu değildir.

İleri sürüm:

```text
TOTP
Email OTP
Passkeys/WebAuthn
```

Application bazında aktif edilebilir.

---

# 79. Passkey Gelecek Uyumluluğu

User identity modeli yalnız password'a bağımlı tasarlanmamalıdır.

İleride:

```sql
user_authenticators
-------------------
id uuid
application_id uuid
user_id uuid
type text
credential_data jsonb
created_at timestamptz
```

ile passkey eklenebilir.

---

# 80. SDK Thread Safety

C++ SDK:

- `Client` instance thread-safe veya documented non-thread-safe olmalıdır.
- Token refresh concurrent çağrılarda tekflight yapılmalıdır.
- Aynı anda 10 request token expired görürse 10 ayrı refresh yapılmamalıdır.

---

# 81. SDK Async API

Qt/GUI uygulamalarında network request UI thread'i bloklamamalıdır.

Öneri:

```cpp
client.auth().loginAsync(...)
```

veya coroutine:

```cpp
co_await client.auth().login(...)
```

İlk sürüm sync + async ikisini destekleyebilir.

---

# 82. Qt Adapter

İleri rahatlık için optional Qt adapter:

```text
keystar-qt
```

Örnek:

```cpp
connect(
    keyStar,
    &KeyStarQtClient::loginSucceeded,
    this,
    &LoginWindow::onLoginSucceeded
);
```

Ama core SDK Qt bağımlı olmamalıdır.

---

# 83. C++ SDK Dependency Politikası

SDK minimum dependency ile tasarlanmalıdır.

Seçenekler:

```text
Transport: libcurl / WinHTTP abstraction
JSON: lightweight parser
Crypto: OS crypto / vetted library
```

Windows-only ilk sürümde WinHTTP + Windows CNG/TPM kullanılabilir.

Ancak public API platform-independent tutulmalıdır.

---

# 84. HWID Provider Interface

```cpp
class DeviceIdentityProvider {
public:
    virtual DeviceIdentity collect() = 0;
    virtual DeviceProof signChallenge(span<const byte> challenge) = 0;
    virtual ~DeviceIdentityProvider() = default;
};
```

Windows implementation:

```text
WindowsDeviceIdentityProvider
```

Gelecekte:

```text
MacDeviceIdentityProvider
LinuxDeviceIdentityProvider
```

---

# 85. Transport Interface

```cpp
class Transport {
public:
    virtual HttpResponse send(const HttpRequest&) = 0;
};
```

Bu testlerde fake transport kullanılmasını kolaylaştırır.

---

# 86. SDK Testability

SDK constructor dependency injection desteklemelidir:

```cpp
Client(
    ClientOptions options,
    std::shared_ptr<Transport> transport,
    std::shared_ptr<DeviceIdentityProvider> deviceProvider,
    std::shared_ptr<TokenStore> tokenStore
);
```

Production default implementation otomatik oluşturulabilir.

---

# 87. Server Test Stratejisi

Test katmanları:

```text
Unit tests
Store integration tests
HTTP handler tests
Black-box API tests
Multi-tenant isolation tests
Security regression tests
SDK contract tests
End-to-end StarLoader tests
```

---

# 88. Multi-Tenant Isolation Testleri

En kritik test grubu.

Örnek:

```text
Application A user oluştur.
Application B secret key ile A user ID sorgula.
Beklenen: 404 / RESOURCE_NOT_FOUND
```

Aynı test:

```text
users
licenses
devices
sessions
variables
bans
webhooks
```

üzerinde uygulanmalıdır.

---

# 89. Credential Security Tests

```text
revoked key rejected
expired key rejected
publishable key server endpoint rejected
secret key wrong application rejected
wrong environment rejected
malformed key rejected
key prefix lookup timing sane
scope missing rejected
```

---

# 90. Device Security Tests

```text
challenge replay rejected
expired challenge rejected
wrong signature rejected
wrong session rejected
same challenge twice rejected
revoked device rejected
low score policy enforced
max devices enforced
TPM required policy enforced
```

---

# 91. Session Security Tests

```text
expired access token rejected
revoked session rejected
refresh token rotation works
old refresh token reuse detected
reuse revokes token family
wrong application audience rejected
wrong device session rejected
```

---

# 92. StarLoader E2E Test

```text
Create StarLoader application
Create publishable credential
Create product + plan
Create test user
Assign license
Launch SDK integration test
Login
Device challenge
Device verify
Session issued
/v1/me success
Logout
Session rejected
```

Bu flow CI'da otomatik çalışmalıdır.

---

# 93. SDK Contract Test

Backend OpenAPI spec üretmelidir.

SDK testleri aynı contract'a karşı çalışmalıdır.

```text
openapi/keystar-v1.yaml
```

Breaking response değişiklikleri CI'da yakalanmalıdır.

---

# 94. OpenAPI

Dokümantasyon için tüm public/server endpointler OpenAPI ile tanımlanmalıdır.

```text
GET /docs
GET /openapi.json
```

Production'da docs auth gerektirebilir.

---

# 95. Developer Documentation

KeyStar dashboard içinde Developer bölümünde:

```text
Quickstart
C++
C#
Rust
Go
Python
JavaScript
REST API
Webhooks
Errors
Security
Migration Guides
```

bulunmalıdır.

---

# 96. Quickstart UX

Yeni application oluşturunca dashboard doğrudan şunu göstermelidir:

```cpp
#include <keystar/keystar.hpp>

keystar::Client client({
    .application_id = "019c...",
    .publishable_key = "ks_pk_test_..."
});
```

Copy button olmalıdır.

---

# 97. SDK Distribution

C++:

```text
vcpkg
Conan
GitHub Releases
CMake FetchContent
```

C#:

```text
NuGet
```

Rust:

```text
crates.io
```

Go:

```text
Go module
```

---

# 98. C++ CMake Kullanımı

```cmake
find_package(KeyStar CONFIG REQUIRED)

target_link_libraries(MyApp PRIVATE KeyStar::KeyStar)
```

veya:

```cmake
FetchContent_Declare(
    keystar
    GIT_REPOSITORY https://github.com/.../keystar-sdk-cpp.git
    GIT_TAG v1.0.0
)
```

---

# 99. Secret Scanner

KeyStar kendi key prefixlerini kullandığı için GitHub secret scanning entegrasyonu ileride yapılabilir.

Örneğin:

```text
ks_sk_live_
ks_sk_test_
```

pattern leak detection için kullanılabilir.

---

# 100. Key Revocation Event

Secret key revoke edildiğinde:

```text
credential.revoked
```

audit + security event oluşturulmalıdır.

Revoked credential kullanılırsa:

```text
credential.revoked_used
```

warning/critical event üretilebilir.

---

# 101. Application Kill Switch

Dashboard'da application status:

```text
Active
Maintenance
Disabled
```

Maintenance mode response:

```json
{
  "error": {
    "code": "APPLICATION_MAINTENANCE",
    "message": "Scheduled maintenance"
  }
}
```

SDK bunu ayrı error olarak verir.

---

# 102. Minimum Client Version

Application settings:

```text
minimum_client_version
latest_client_version
force_update
update_url
```

Login sırasında SDK veya app version gönderilebilir:

```http
X-KeyStar-App-Version: 1.4.2
```

Backend outdated client'ı reddedebilir.

---

# 103. SDK Version Header

```http
X-KeyStar-SDK: cpp/1.0.0
```

Telemetry ve compatibility için kullanılmalıdır.

---

# 104. Metadata

Domain tablolarda kontrolsüz kolon eklemek yerine belirli extensibility alanlarında JSONB kullanılabilir.

Örnek:

```sql
metadata jsonb not null default '{}'
```

Ama core authorization kararları JSONB içinde tutulmamalıdır.

---

# 105. Pagination

Server API list endpointleri cursor pagination kullanmalıdır.

```http
GET /v1/server/users?limit=50&after=<cursor>
```

Response:

```json
{
  "data": [...],
  "page": {
    "next_cursor": "...",
    "has_more": true
  }
}
```

UUIDv7 zaman sıralı olduğu için cursor stratejisinde avantaj sağlar.

---

# 106. UUIDv7 Politikası

Tüm yeni primary keyler UUIDv7 olmalıdır.

```text
organizations
applications
credentials
users
products
plans
licenses
devices
sessions
webhooks
audit_logs
security_events
```

DB constraint ile version 7 doğrulaması devam etmelidir.

---

# 107. Soft Delete vs Hard Delete

Security-critical objeler hard delete edilmemelidir.

Örneğin credential:

```text
revoked_at
status = revoked
```

License:

```text
status = revoked
```

Audit log korunur.

Privacy gereksiniminde user deletion ayrı anonymization flow ile ele alınabilir.

---

# 108. Data Retention

Application bazında retention politikası ileride desteklenebilir.

```text
security logs: 90 days
audit logs: 365 days
session logs: 30 days
```

Admin audit gibi kritik loglar daha uzun tutulabilir.

---

# 109. IP Storage

Ham IP yerine güvenlik amaçlı keyed hash kullanılabilir.

Geolocation/abuse detection gerekiyorsa ayrı privacy policy değerlendirilmelidir.

KeyStar'ın mevcut hashed IP yaklaşımı korunabilir.

---

# 110. Application Credential Cache

Yüksek trafikte her request DB credential lookup yapmamalıdır.

Cache:

```text
credential locator -> application principal
TTL: 30-120 seconds
```

Revocation event cache invalidation tetiklemelidir.

İlk sürümde memory cache yeterli olabilir.

Distributed deployment'ta Redis opsiyonel kullanılabilir.

---

# 111. Dependency Politikası

KeyStar core mümkün olduğunca PostgreSQL dışında zorunlu dependency istememelidir.

İlk sürüm:

```text
Required:
- Go API
- PostgreSQL

Optional:
- Redis
- object storage
- message broker
```

Redis olmadan sistem çalışmalıdır.

---

# 112. Webhook Queue

Webhook gönderimi login request'i bloklamamalıdır.

İlk sürüm:

```text
PostgreSQL outbox table + worker
```

Yeterlidir.

İleride:

```text
Redis Streams
NATS
Kafka
```

kullanılabilir.

---

# 113. Outbox Pattern

```sql
outbox_events
-------------
id uuid primary key
application_id uuid
type text
payload jsonb
status text
attempts integer
available_at timestamptz
created_at timestamptz
processed_at timestamptz
```

Domain transaction ile event aynı transaction içinde kaydedilir.

---

# 114. Webhook Retry

```text
1m
5m
15m
1h
6h
24h
```

Exponential retry.

Webhook delivery dashboard'da görülebilir.

---

# 115. Admin Dashboard Güvenlik

Mevcut:

```text
RBAC
TOTP MFA
recovery code
security event
```

korunmalıdır.

Ek olarak:

```text
recent sessions
session revoke
credential create confirmation
critical action re-auth
```

önerilir.

---

# 116. Critical Action Re-auth

Aşağıdaki işlemlerde admin'den yeniden password/TOTP istenebilir:

```text
Create server secret
Revoke all credentials
Delete application
Disable application
Reset all devices
Transfer organization ownership
```

---

# 117. SDK Secret Misuse Detection

Bir `ks_sk_` key client SDK user-agent ile gelirse security event üretilebilir.

Örnek:

```text
X-KeyStar-SDK: cpp/1.2.0
Authorization: Bearer ks_sk_live_...
```

Bu genellikle developer'ın server secret'ı executable içine koyduğunu gösterir.

Server warning verebilir veya policy ile reddedebilir.

---

# 118. Application Origin Restrictions

Web SDK için gelecekte:

```text
allowed_origins
allowed_redirect_urls
```

kullanılabilir.

Desktop SDK için origin güvenlik sınırı değildir.

---

# 119. Machine Clone Detection

Aynı TPM/public key + çok farklı fingerprint kombinasyonu veya aynı fingerprint + farklı TPM key anomalileri security event üretebilir.

Örnek:

```text
device.clone_suspected
```

Bu otomatik ban yerine risk signal olarak kullanılmalıdır.

---

# 120. Device Change History

```sql
device_identity_history
-----------------------
id uuid primary key
application_id uuid
device_id uuid
fingerprint_hmac text
score integer
change_summary jsonb
created_at timestamptz
```

Support ekibi kullanıcı cihazının neden re-verification istediğini anlayabilir.

---

# 121. Support-Friendly HWID Reset

Admin panel:

```text
User -> Devices -> Reset Device Binding
```

Reset doğrudan device silmek yerine:

```text
old device -> revoked
new enrollment allowed
```

şeklinde çalışmalıdır.

Audit event oluşturulmalıdır.

---

# 122. Max Devices

Device limit plan veya license bazında belirlenebilir.

Resolution sırası:

```text
License override
↓
Plan max_devices
↓
Application default
```

---

# 123. Concurrent Sessions

Application setting:

```text
max_sessions_per_user
```

Policy:

```text
reject_new
revoke_oldest
allow
```

---

# 124. Offline Mode

Tam offline licensing ayrı security tradeoff gerektirir.

İlk KeyStar sürümünde online verification temel alınmalıdır.

İleri sürümde signed offline lease:

```text
user
application
license
expires_at
device_public_key_hash
nonce
signature
```

kullanılabilir.

Lease kısa süreli olmalıdır.

---

# 125. SDK Local Session Cache

SDK access token'ı memory'de tutabilir.

Refresh token secure storage'da.

Application kapandığında access token memory'den gider.

---

# 126. Logout

```http
POST /v1/auth/logout
```

Server refresh session'ı revoke eder.

SDK secure storage tokenlarını siler.

---

# 127. Logout All Devices

Server/admin endpoint:

```text
POST /v1/server/users/{id}/sessions/revoke-all
```

Opsiyon:

```text
keep_current=true
```

---

# 128. Registration

Application setting:

```text
allow_registration
```

Client endpoint:

```http
POST /v1/auth/register
```

Kapatılmışsa:

```text
REGISTRATION_DISABLED
```

---

# 129. Email Verification

İlk sürüm opsiyonel.

Application policy:

```text
none
optional
required
```

Email provider platform seviyesinde configurable olmalıdır.

---

# 130. Password Reset

```text
POST /v1/auth/password/forgot
POST /v1/auth/password/reset
```

Token single-use ve hashed storage.

User enumeration yapılmamalıdır.

---

# 131. Server API Scope Check

Her server endpoint explicit scope istemelidir.

Örnek:

```text
GET /v1/server/users
-> users.read

POST /v1/server/users
-> users.write

POST /v1/server/licenses
-> licenses.write
```

`admin` gibi tek broad scope'a yaslanılmamalıdır.

---

# 132. API Key IP Restrictions

Secret credential için optional:

```sql
credential_ip_rules
-------------------
id uuid
credential_id uuid
cidr cidr
```

Dashboard:

```text
Restrict this key to:
203.0.113.0/24
```

---

# 133. API Key Expiration

Credential create sırasında:

```text
Never
7 days
30 days
90 days
Custom
```

özellikle CI keyleri için faydalıdır.

---

# 134. Application Transfer

İleride application başka organization'a taşınabilir.

Bu kritik işlem audit + reauth gerektirir.

İlk sürümde desteklenmesi şart değildir.

---

# 135. Data Export

Developer kendi application datasını export edebilmelidir:

```text
Users CSV
Licenses CSV
Devices CSV
Audit JSON
```

Secret/HWID raw data export edilmemelidir.

---

# 136. Developer Onboarding

Flow:

```text
Sign up
↓
Create Organization
↓
Create Application
↓
Get Application ID
↓
Create Test Publishable Key
↓
Install SDK
↓
Run Quickstart
↓
Create Product/Plan
↓
Create Test License
↓
Test Login
↓
Switch to Live
```

---

# 137. StarLoader Dogfooding

KeyStar'ın en önemli kalite stratejisi:

**StarLoader production'da KeyStar SDK kullanmalıdır.**

Böylece:

- SDK kullanılabilirliği gerçek kullanımda test edilir.
- Login sorunları kullanıcıdan önce görülür.
- Device policy gerçekte sınanır.
- API backward compatibility ciddiye alınır.
- KeyStar'ın müşteriden beklediği integration modeli kendi ürününde doğrulanır.

---

# 138. StarLoader İçin Geçiş Fazları

## Faz 1

DB application scoping hardening.

## Faz 2

Application credentials.

## Faz 3

Public client middleware.

## Faz 4

C++ SDK alpha.

## Faz 5

StarLoader SDK entegrasyonu.

## Faz 6

Legacy StarLoader HTTP code kaldırılır.

## Faz 7

External developer beta.

---

# 139. Uygulama Fazları

## Phase 0 — Foundation Audit

- Existing schema doğrula.
- Existing application_id kullanımlarını bul.
- Application-scoped olmayan queryleri listele.
- Legacy product dependencylerini bul.
- Existing tests baseline oluştur.

## Phase 1 — Tenant Hardening

- `application_id NOT NULL`.
- Tenant-aware unique constraints.
- Repository method signature update.
- Cross-tenant integration tests.

## Phase 2 — Credentials

- `application_credentials`.
- `ks_pk_` / `ks_sk_` generation.
- Hash verification.
- Scope engine.
- Rotation/revoke.

## Phase 3 — Public API Context

- Application middleware.
- Public client authorization.
- Legacy StarLoader compatibility.

## Phase 4 — Product / Plan

- Products.
- Plans.
- License normalization.

## Phase 5 — Device Policy

- Device policy table.
- TPM required/preferred/optional.
- Scoring configuration.

## Phase 6 — Sessions

- Refresh token sessions.
- Rotation.
- Reuse detection.

## Phase 7 — C++ SDK

- Transport.
- Auth.
- Device provider.
- TPM.
- Secure token store.
- Error mapping.

## Phase 8 — StarLoader

- SDK dependency.
- Login migration.
- HWID migration.
- Session migration.
- E2E tests.

## Phase 9 — Developer Platform

- Applications dashboard.
- API Keys.
- Products.
- Webhooks.
- Docs.

## Phase 10 — External Beta

- C++ SDK release.
- Test environment.
- Quickstart.
- Rate limiting.
- Monitoring.

---

# 140. Önerilen Backend Package Genişlemesi

```text
backend/internal/
├── application
│   ├── service.go
│   ├── repository.go
│   └── credentials.go
│
├── credential
│   ├── generator.go
│   ├── verifier.go
│   └── scopes.go
│
├── domain
│   ├── application.go
│   ├── credential.go
│   ├── product.go
│   ├── plan.go
│   └── ...
│
├── httpapi
│   ├── middleware_application.go
│   ├── client_auth.go
│   ├── server_users.go
│   ├── server_licenses.go
│   └── ...
│
├── service
│   ├── login.go
│   ├── device_verify.go
│   ├── license_activate.go
│   └── ...
│
└── store
    ├── applications.go
    ├── credentials.go
    ├── products.go
    ├── plans.go
    └── ...
```

---

# 141. LoginService Yeni Interface

Eski:

```go
type LoginRepository interface {
    FindUserByEmail(context.Context, string) (*domain.User, error)
    FindLicenseByUserAndProduct(context.Context, string, string) (*domain.License, error)
}
```

Yeni:

```go
type LoginRepository interface {
    FindUserByIdentifier(
        context.Context,
        uuid.UUID,
        string,
    ) (*domain.User, error)

    FindActiveEntitlement(
        context.Context,
        uuid.UUID,
        uuid.UUID,
    ) (*domain.License, error)

    CreatePendingSession(
        context.Context,
        domain.NewPendingSession,
    ) (*domain.PendingSession, error)
}
```

Application ID explicit parametre olmalıdır.

---

# 142. Service Input

```go
type LoginInput struct {
    ApplicationID     uuid.UUID
    Email             string
    Password          string
    DeviceFingerprint string
    ClientVersion     string
    SDKVersion        string
}
```

Application ID handler tarafından context'ten doldurulur. Client JSON'dan gelen application_id service güven sınırı olarak kullanılmamalıdır.

---

# 143. Handler Pattern

```go
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
    principal, ok := AppPrincipalFromContext(r.Context())
    if !ok {
        writeError(...)
        return
    }

    var req LoginRequest
    if err := decodeJSON(r, &req); err != nil {
        ...
    }

    out, err := h.login.Login(r.Context(), service.LoginInput{
        ApplicationID: principal.ApplicationID,
        Email: req.Email,
        Password: req.Password,
        DeviceFingerprint: req.DeviceFingerprint,
    })
}
```

---

# 144. Admin Dashboard Applications Sayfası

Route önerisi:

```text
/admin/applications
/admin/applications/[id]
/admin/applications/[id]/users
/admin/applications/[id]/licenses
/admin/applications/[id]/devices
/admin/applications/[id]/credentials
/admin/applications/[id]/webhooks
/admin/applications/[id]/settings
```

---

# 145. Organization Switcher

Dashboard üst bölüm:

```text
MevaTech Software
  ├─ StarLoader
  ├─ Test App
  └─ + New Application
```

Current application tüm page requestlerinde explicit state olmalıdır.

---

# 146. Admin API Tenant Check

Admin JWT organization access verse bile application'ın organization'a ait olduğu ayrıca doğrulanmalıdır.

```text
Admin -> Organization A
Request -> Application B (Organization B)
Result -> 404
```

403 yerine resource enumeration engellemek için bazı endpointlerde 404 tercih edilebilir.

---

# 147. First-Party Marker

StarLoader'a özel backend davranışı olmamalıdır; fakat operasyonel amaçla application metadata içinde:

```json
{
  "first_party": true
}
```

bulunabilir.

Authorization kararları buna bağlı olmamalıdır.

---

# 148. Public SDK Key'in Rolü

Önemli güvenlik notu:

```text
Publishable key = application secret değildir.
```

Bir attacker bunu çıkarabilir.

Güvenlik aşağıdakilerden gelir:

```text
user authentication
device proof
server-side license check
short-lived session
rate limiting
application isolation
server secrets only on server
```

---

# 149. SDK Kullanıcı Deneyimi Hedefi

Developer için maksimum basitlik:

```cpp
KeyStar ks(APP_ID, PUBLIC_KEY);

auto result = ks.login(email, password);
```

Ama SDK içinde güçlü workflow:

```text
App auth
User auth
License auth
Device identity
TPM challenge
Session creation
Token storage
Refresh
Error handling
```

çalışmalıdır.

---

# 150. MVP Sınırı

İlk public beta için mutlaka gerekenler:

```text
Organizations
Applications
Publishable keys
Secret keys
Application-scoped users
Application-scoped licenses
Application-scoped devices
Application-scoped sessions
Login
HWID/device verification
TPM preferred support
C++ SDK
StarLoader integration
Admin Applications UI
API Keys UI
Rate limiting
Audit logs
Security events
Documentation
```

İlk beta için şart olmayanlar:

```text
C#/Rust/Python SDK
Billing
Passkeys
Offline license lease
Advanced webhook analytics
Redis
Kafka
Multi-region
```

---

# 151. Definition of Done — StarLoader

StarLoader entegrasyonu tamamlanmış sayılması için:

- StarLoader DB'de normal bir KeyStar application olmalı.
- StarLoader publishable key kullanmalı.
- StarLoader server secret içermemeli.
- Login KeyStar C++ SDK üzerinden yapılmalı.
- HWID SDK device provider üzerinden toplanmalı.
- TPM challenge desteklenmeli.
- License backend tarafında application-scoped doğrulanmalı.
- Session token KeyStar tarafından verilmeli.
- `/v1/me` KeyStar üzerinden çalışmalı.
- Device reset admin dashboard'dan yapılabilmeli.
- StarLoader özel login endpoint kodu bulunmamalı.
- E2E test geçmeli.

---

# 152. Definition of Done — External SDK

Dış geliştirici aşağıdaki işlemleri backend koduna dokunmadan yapabilmelidir:

1. Dashboard'da application oluştur.
2. Publishable key oluştur.
3. Product/plan oluştur.
4. User/license oluştur.
5. SDK'yı kur.
6. `Client(appId, publishableKey)` oluştur.
7. `login()` çağır.
8. KeyStar otomatik HWID/device verification yapsın.
9. Başarılı session dönsün.
10. Dashboard'da user/device/session görünsün.

Bu akış çalışıyorsa KeyStar artık gerçekten reusable platformdur.

---

# 153. Nihai Mimari

```text
                         ┌──────────────────────────┐
                         │       KeyStar Cloud      │
                         │                          │
                         │ Go API + PostgreSQL      │
                         └────────────┬─────────────┘
                                      │
                          Application Resolver
                                      │
                   ┌──────────────────┼──────────────────┐
                   │                  │                  │
             StarLoader          Customer Game      Customer Tool
             app_01...            app_02...          app_03...
                   │                  │                  │
              C++ SDK             C++ SDK            C# SDK
                   │                  │                  │
          Login + License       Login + License     Login + License
          HWID + TPM            HWID + TPM          Device Identity
                   │                  │                  │
                   └──────────────────┼──────────────────┘
                                      │
                           KeyStar Auth Platform
```

Server integrations:

```text
Customer Backend
      │
      │ ks_sk_live_...
      ↓
KeyStar Server API
```

Admin:

```text
Developer / Organization Owner
      │
      │ Admin Session + MFA + RBAC
      ↓
KeyStar Dashboard
```

---

# 154. Sonuç

KeyStar bundan sonra şu şekilde düşünülmelidir:

> **KeyStar bir StarLoader backend'i değildir. StarLoader, KeyStar platformunu kullanan ilk uygulamadır.**

KeyStar'ın temel ürün sınırı:

```text
Application Identity
+
User Authentication
+
Licensing
+
Device Identity / HWID
+
Session Security
+
Developer SDK
+
Admin Platform
```

olmalıdır.

Dış geliştiricinin ihtiyacı yalnızca:

```text
Application ID
Publishable Key
KeyStar SDK
```

olmalıdır.

Server-side yönetim yapmak istiyorsa:

```text
Secret Key
```

kullanır.

StarLoader da aynı public SDK üzerinden bağlandığı için sistemin özel-case olmadan gerçek kullanımda doğrulanması sağlanır.

Bu mimari uygulandığında KeyStar, tek projeye özel auth backend olmaktan çıkar ve bağımsız, çoklu uygulama destekli, SDK-first bir authentication/licensing/HWID platformuna dönüşür.

---

# 155. Uygulamaya Başlama Sırası — Kısa Referans

```text
1. Tenant scoping hardening
2. Application credentials
3. Credential middleware
4. Public/server API separation
5. Products/plans normalization
6. Device policy
7. Refresh-session rotation
8. C++ SDK core
9. Windows HWID provider
10. TPM provider
11. StarLoader SDK migration
12. Applications dashboard
13. API keys dashboard
14. Webhooks
15. OpenAPI/docs
16. External beta
```

**İlk geliştirme hedefi:** `application_credentials + application middleware + repository application scoping`.  
Bunlar bitmeden SDK yazmaya başlanmamalıdır; aksi halde SDK yine StarLoader'a özel bir API'nin wrapper'ına dönüşür.

