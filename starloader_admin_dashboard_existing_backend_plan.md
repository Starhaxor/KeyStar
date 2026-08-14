# StarLoader Admin Dashboard --- Mevcut Backend Üzerine Genişletme Planı

## 1. Bu dokümanın kapsamı

Bu doküman **yeni bir Go backend, Qt Login Client veya HWID Obtainer
geliştirme planı değildir**. Bunlar mevcut ve ayrı bileşenlerdir.

Bu planın konusu yalnızca:

-   mevcut StarLoader backend ve PostgreSQL şemasını bozmadan
    genişletmek,
-   Next.js Admin Dashboard geliştirmek,
-   Organization / Team / RBAC altyapısını şimdiden eklemek,
-   mevcut users / licenses / devices / auth_sessions /
    device_challenges verilerini korumak,
-   tüm yeni primary key'lerde UUIDv7 standardını sürdürmek,
-   admin tarafını yüksek güvenlik standardıyla tasarlamaktır.

------------------------------------------------------------------------

# 2. GitHub'daki mevcut durum

StarLoader repository incelendiğinde backend zaten bulunmaktadır ve
migration sistemi kullanılmaktadır.

Mevcut ana tablolar:

``` text
users
licenses
devices
auth_sessions
device_challenges
```

Mevcut migration ayrıca `starloader_uuid_v7()` fonksiyonunu tanımlamakta
ve bu tabloların primary key'lerini UUIDv7 olarak üretmektedir.

Bu nedenle **bunları yeniden oluşturmayacağız**.

Yeni yapı migration `000003_...` ve devamı olarak mevcut sistemin
üzerine eklenecektir.

------------------------------------------------------------------------

# 3. UUID Standardı

Tüm yeni entity primary key'leri:

``` text
UUIDv7
```

olacaktır.

Mevcut backend'deki:

``` sql
default starloader_uuid_v7()
```

mekanizması kullanılacaktır.

Yeni tablolarda ayrıca mevcut sistemdeki gibi UUID version constraint
uygulanacaktır:

``` sql
check ((get_byte(uuid_send(id), 6) >> 4) = 7)
```

UUIDv4 veya serial/bigserial primary key kullanılmayacaktır.

Not: UUIDv7 authorization mekanizması değildir. Her endpoint ayrıca
tenant ve permission doğrulaması yapacaktır.

------------------------------------------------------------------------

# 4. Mevcut Tablolar Korunacak

Aşağıdakiler yeniden yazılmayacak:

``` text
users
licenses
devices
auth_sessions
device_challenges
```

Mevcut ilişkiler ve güvenlik özellikleri korunacaktır:

``` text
User
 └── License
      └── Device
           └── TPM public key

User + License
 └── Auth Session
      └── Device Challenge
```

Yeni dashboard/SaaS altyapısı bunların üzerine eklenecektir.

------------------------------------------------------------------------

# 5. Yeni Tenant Modeli

Gelecekte tek kişi veya takım kullanabilmek için temel sahiplik modeli:

``` text
Platform Account
       │
       ▼
Organization
       │
       ├── Members
       │     └── Role → Permissions
       │
       └── Applications
              │
              └── mevcut StarLoader verileri
```

İlk kullanımda tek kişi bile:

``` text
Personal Organization
└── OWNER
```

olarak oluşturulur.

Böylece daha sonra Team özelliği geldiğinde temel DB yeniden
tasarlanmaz.

------------------------------------------------------------------------

# 6. Yeni Tablolar

İlk genişletmede eklenecek tablolar:

``` text
admin_accounts
organizations
organization_members
roles
permissions
role_permissions
applications
admin_sessions
admin_mfa_credentials
admin_recovery_codes
admin_login_attempts
audit_logs
security_events
```

İleride fakat şimdilik zorunlu olmayanlar:

``` text
organization_invitations
api_keys
webhooks
webhook_deliveries
billing_accounts
subscriptions
custom_roles
```

------------------------------------------------------------------------

# 7. Admin Accounts

End-user `users` tablosu ile dashboard yöneticileri ayrılacaktır.

``` sql
create table admin_accounts (
    id uuid primary key default starloader_uuid_v7(),
    email text not null,
    password_hash text not null,
    status text not null default 'active',
    mfa_required boolean not null default true,
    password_changed_at timestamptz not null default now(),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint admin_accounts_id_uuid_v7_check
        check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    constraint admin_accounts_email_normalized_check
        check (email = lower(btrim(email))),
    constraint admin_accounts_email_unique unique (email),
    constraint admin_accounts_status_check
        check (status in ('active','disabled','locked'))
);
```

Admin password hashing:

``` text
Argon2id
+ unique random salt
+ server-side pepper
```

Pepper DB'de tutulmaz.

------------------------------------------------------------------------

# 8. Organizations

``` sql
create table organizations (
    id uuid primary key default starloader_uuid_v7(),
    name text not null,
    slug text not null,
    status text not null default 'active',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint organizations_id_uuid_v7_check
        check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    constraint organizations_slug_unique unique (slug)
);
```

Organization sistemdeki tenant/security boundary'dir.

------------------------------------------------------------------------

# 9. Applications

``` sql
create table applications (
    id uuid primary key default starloader_uuid_v7(),
    organization_id uuid not null references organizations(id) on delete restrict,
    name text not null,
    slug text not null,
    status text not null default 'active',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint applications_id_uuid_v7_check
        check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    constraint applications_org_slug_unique unique (organization_id, slug)
);
```

StarLoader ilk Application olarak bağlanabilir.

İleride aynı organization:

``` text
StarLoader
Launcher B
Application C
```

yönetebilir.

------------------------------------------------------------------------

# 10. Mevcut Verileri Tenant-Aware Hale Getirme

Mevcut tabloları drop/recreate etmeyeceğiz.

Aşamalı migration yapılacaktır.

Örnek sıra:

``` text
1. organizations oluştur
2. applications oluştur
3. mevcut StarLoader için default organization oluştur
4. mevcut StarLoader application oluştur
5. users/licenses/devices/auth_sessions tablolarına gerekli scope kolonlarını nullable ekle
6. mevcut kayıtları backfill et
7. foreign key/index ekle
8. doğrula
9. en son NOT NULL yap
```

Production migration tek büyük kırıcı işlem olmamalıdır.

------------------------------------------------------------------------

# 11. Scope Stratejisi

Minimum olarak application'a ait veriler `application_id` ile scope
edilmelidir.

Örneğin mevcut `users`:

``` text
users
+ application_id
```

License:

``` text
licenses
+ application_id
```

Device:

``` text
devices
+ application_id
```

Organization application üzerinden türetilebildiği için her tabloya
gereksiz `organization_id` kopyalamak zorunlu değildir. Ancak performans
veya güvenlik modeli gerektirirse kontrollü denormalizasyon yapılabilir.

Ama tenant sorgusu daima doğrulanacaktır.

------------------------------------------------------------------------

# 12. Organization Members

``` sql
create table organization_members (
    id uuid primary key default starloader_uuid_v7(),
    organization_id uuid not null references organizations(id) on delete restrict,
    admin_account_id uuid not null references admin_accounts(id) on delete restrict,
    role_id uuid not null,
    status text not null default 'active',
    created_at timestamptz not null default now(),
    constraint organization_members_id_uuid_v7_check
        check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    constraint organization_members_unique
        unique (organization_id, admin_account_id)
);
```

Bir admin account birden fazla organization üyesi olabilir.

------------------------------------------------------------------------

# 13. RBAC

Başlangıç roller:

``` text
OWNER
ADMIN
DEVELOPER
SUPPORT
VIEWER
```

Fakat authorization doğrudan role adına bağlanmayacaktır.

Yanlış:

``` go
if user.Role == "ADMIN" { ... }
```

Doğru yaklaşım:

``` text
Role
 ↓
Permissions
 ↓
RequirePermission(...)
```

------------------------------------------------------------------------

# 14. Permission Modeli

Başlangıç permission'ları:

``` text
organization.read
organization.update
organization.delete

members.read
members.invite
members.update
members.remove

applications.read
applications.create
applications.update
applications.delete

users.read
users.update
users.disable

licenses.read
licenses.create
licenses.update
licenses.extend
licenses.suspend
licenses.revoke

devices.read
devices.reset
devices.revoke

sessions.read
sessions.revoke

security.read
audit.read

settings.read
settings.update
```

Endpoint permission kontrolü merkezi middleware/policy katmanında
yapılacaktır.

------------------------------------------------------------------------

# 15. Admin MFA

Admin Dashboard için MFA opsiyonel bırakılmayacaktır.

Tercih edilen başlangıç:

``` text
Password
   +
TOTP MFA
```

Daha sonra:

``` text
WebAuthn / Passkeys
```

eklenebilir.

TOTP secret plaintext olarak DB'de tutulmamalıdır; uygulama seviyesinde
authenticated encryption ile korunmalıdır.

Recovery code'lar yalnızca hash olarak tutulacaktır.

------------------------------------------------------------------------

# 16. Admin Session Güvenliği

Browser'da access/refresh credential:

``` text
localStorage ❌
HttpOnly cookie ✅
```

Cookie minimum:

``` text
HttpOnly
Secure
SameSite=Strict veya uygun Lax politikası
Path=/
```

Session identifier yüksek entropili CSPRNG ile üretilir.

DB'de raw session/refresh token saklanmaz; hash/HMAC tutulur.

Session rotation uygulanır.

Logout/revoke sonrası token tekrar kullanılamaz.

Şüpheli refresh-token reuse security event üretir.

------------------------------------------------------------------------

# 17. CSRF

Cookie tabanlı admin auth nedeniyle state-changing endpoint'lerde CSRF
savunması uygulanacaktır.

Örnek:

``` text
Origin / Referer validation
+ CSRF token
+ SameSite cookie
```

Tek başına CORS bir CSRF savunması olarak kabul edilmez.

------------------------------------------------------------------------

# 18. CORS

Production allowlist:

``` text
https://admin.example.com
```

Development:

``` text
http://localhost:3000
```

Admin credential endpointlerinde:

``` text
Access-Control-Allow-Origin: *
```

kullanılmayacaktır.

------------------------------------------------------------------------

# 19. Tenant Isolation

En kritik güvenlik kurallarından biridir.

Yanlış:

``` sql
select * from licenses where id = $1;
```

Doğru mantık:

``` sql
select l.*
from licenses l
join applications a on a.id = l.application_id
where l.id = $1
  and l.application_id = $2
  and a.organization_id = $3;
```

`organization_id` ve permission context authenticated admin
session/membership üzerinden resolve edilir.

Client'ın gönderdiği tenant ID tek başına güven kaynağı değildir.

------------------------------------------------------------------------

# 20. IDOR / BOLA Koruması

Her resource endpoint'i şu üç soruyu doğrular:

``` text
1. Bu admin authenticated mı?
2. Bu organization'ın aktif üyesi mi?
3. Bu resource için gereken permission'a sahip mi?
4. İstenen resource gerçekten bu tenant/application'a mı ait?
```

UUIDv7 kullanılması IDOR/BOLA kontrolünün yerine geçmez.

------------------------------------------------------------------------

# 21. Audit Logs

Admin'in kritik her değişikliği immutable-style audit event
oluşturmalıdır.

Örnek:

``` text
ADMIN_LOGIN
ADMIN_LOGIN_FAILED
MFA_FAILED
MEMBER_INVITED
MEMBER_ROLE_CHANGED
USER_DISABLED
LICENSE_CREATED
LICENSE_REVOKED
DEVICE_RESET
DEVICE_REVOKED
SESSION_REVOKED
SETTINGS_CHANGED
```

Audit record:

``` text
id UUIDv7
organization_id
actor_admin_id
action
resource_type
resource_id
request_id
ip_hash
user_agent summary
metadata
created_at
```

Password, token, TOTP secret, raw license key veya hassas HWID audit
log'a yazılmaz.

------------------------------------------------------------------------

# 22. Security Events

Audit log ile security event ayrılacaktır.

Audit:

``` text
Admin ne yaptı?
```

Security event:

``` text
Sistemde hangi güvenlik olayı meydana geldi?
```

Örnek:

``` text
BRUTE_FORCE_DETECTED
ADMIN_ACCOUNT_LOCKED
MFA_FAILURE_SPIKE
SESSION_REUSE_DETECTED
HWID_MISMATCH
TPM_SIGNATURE_INVALID
CHALLENGE_REPLAY
DEVICE_REVOKED
```

------------------------------------------------------------------------

# 23. Rate Limiting

Özellikle:

``` text
/admin/auth/login
/admin/auth/mfa
/admin/auth/recovery
password reset
member invite
license generation
```

endpoint'leri rate limited olacaktır.

Limit yalnız IP bazlı olmamalıdır.

Kombinasyon:

``` text
IP
account/email
organization
endpoint
```

kullanılabilir.

Distributed deployment geldiğinde rate-limit state merkezi store'a
taşınabilir.

------------------------------------------------------------------------

# 24. Brute Force Savunması

Admin login:

``` text
progressive delay
rate limit
account lock policy
MFA
security event
```

Generic hata:

``` text
Invalid credentials
```

Kullanıcının var olup olmadığı login response'undan anlaşılmamalıdır.

------------------------------------------------------------------------

# 25. Password Güvenliği

Admin password:

``` text
Argon2id
```

Parametreler deployment makinesinde benchmark edilerek seçilir.

Ayrıca:

``` text
minimum password length
breached-password kontrolü (uygunsa)
password change -> other sessions revoke
constant-time sensitive comparisons
server-side pepper
```

uygulanacaktır.

------------------------------------------------------------------------

# 26. Database Security

Admin Dashboard PostgreSQL'e doğrudan bağlanmayacaktır.

``` text
Next.js
  ↓ HTTPS
Mevcut API
  ↓
PostgreSQL
```

DB tarafı:

``` text
least privilege DB user
TLS in production
parameterized queries
statement/query timeouts
connection limits
migration role != runtime role (tercihen)
regular backup
restore test
```

SQL string concatenation kullanılmayacaktır.

------------------------------------------------------------------------

# 27. PostgreSQL RLS

Application-level tenant kontrolü zorunludur.

Buna ek olarak, mimari uygun hale geldiğinde kritik tenant tablolarında
PostgreSQL Row Level Security ikinci savunma katmanı olarak
değerlendirilecektir.

RLS eklenirse application context transaction başında güvenli
server-side session variable ile set edilmeli; client-controlled değer
doğrudan kullanılmamalıdır.

RLS, API authorization'ın yerine geçmez; defense-in-depth katmanıdır.

------------------------------------------------------------------------

# 28. HTTP Güvenliği

Admin API için:

``` text
TLS only production
HSTS
strict Content-Type
request body size limit
header size limit
read timeout
write timeout
idle timeout
request ID
structured security logging
```

Next.js için:

``` text
Content-Security-Policy
frame-ancestors 'none'
X-Content-Type-Options: nosniff
Referrer-Policy
Permissions-Policy
```

uygulanacaktır.

------------------------------------------------------------------------

# 29. Admin API Namespace

Mevcut auth/client endpoint'lerine dokunmadan admin API ayrı namespace
altında tutulacaktır:

``` text
/v1/admin/*
```

Örnek:

``` text
POST /v1/admin/auth/login
POST /v1/admin/auth/mfa/verify
POST /v1/admin/auth/logout
POST /v1/admin/auth/refresh

GET  /v1/admin/me
GET  /v1/admin/organizations
GET  /v1/admin/applications

GET  /v1/admin/users
GET  /v1/admin/users/{id}
PATCH /v1/admin/users/{id}

GET  /v1/admin/licenses
POST /v1/admin/licenses
GET  /v1/admin/licenses/{id}
PATCH /v1/admin/licenses/{id}
POST /v1/admin/licenses/{id}/revoke

GET  /v1/admin/devices
GET  /v1/admin/devices/{id}
POST /v1/admin/devices/{id}/reset
POST /v1/admin/devices/{id}/revoke

GET  /v1/admin/sessions
POST /v1/admin/sessions/{id}/revoke

GET  /v1/admin/security/events
GET  /v1/admin/audit-logs

GET  /v1/admin/members
POST /v1/admin/members/invite
PATCH /v1/admin/members/{id}
DELETE /v1/admin/members/{id}
```

------------------------------------------------------------------------

# 30. Next.js Admin Dashboard

Dashboard ayrı repository olacaktır.

Ana menü:

``` text
Organization [▼]
Application  [▼]

Overview
Users
Licenses
Devices
Sessions
Security

Organization
  Members
  Settings
```

Custom Roles UI şimdilik yapılmayabilir; RBAC altyapısı backend'de hazır
olacaktır.

------------------------------------------------------------------------

# 31. Overview

``` text
Total Users
Active Licenses
Active Devices
Active Sessions
```

Ek olarak:

``` text
Login activity
Failed logins
License activations
HWID mismatches
TPM failures
Recent admin activity
```

------------------------------------------------------------------------

# 32. Users

Liste:

``` text
Email
Status
License
Devices
Last Login
Created
```

Detail:

``` text
Account
Licenses
Devices
Sessions
Security Events
```

Actions permission-controlled:

``` text
Disable
Enable
Revoke Sessions
```

------------------------------------------------------------------------

# 33. Licenses

Mevcut `licenses` tablosu kullanılacaktır.

Dashboard:

``` text
License ID
User
Product
Status
Device Limit
Expires
Created
```

Actions:

``` text
Create
Extend
Revoke
Change Device Limit
```

Mevcut sistem license HMAC sakladığı için admin UI'ın raw key'i sonradan
DB'den okuyabileceği varsayılmamalıdır.

Yeni license key üretildiğinde plaintext key yalnızca oluşturma anında
kullanıcıya gösterilebilir; kalıcı storage'da HMAC tutulur.

------------------------------------------------------------------------

# 34. Devices

Mevcut `devices` tablosu kullanılacaktır.

Gösterilecek alanlar:

``` text
Device UUIDv7
User
License
TPM status
HWID/fingerprint status
Last Seen
Status
```

Detail:

``` text
TPM public key fingerprint
SMBIOS match
Motherboard match
BIOS match
Disk match
MachineGuid match
Fingerprint
Security history
```

Admin UI'da mümkün olduğunca raw hardware identifier gösterilmemelidir.

Actions:

``` text
Reset
Revoke
Revoke Sessions
```

------------------------------------------------------------------------

# 35. Sessions

Mevcut `auth_sessions` yapısı dikkate alınacaktır.

Dashboard:

``` text
Session UUIDv7
User
License
Status
Created
Expires
```

İleride admin/browser session yapısı ayrı `admin_sessions` tablosunda
tutulacaktır.

------------------------------------------------------------------------

# 36. Security Panel

Tabs:

``` text
Login Attempts
Device Security
Admin Audit
Security Events
```

Filtre:

``` text
organization
application
user
severity
event type
date range
```

------------------------------------------------------------------------

# 37. Members

``` text
Name / Email
Role
Status
Joined
Last Active
```

Actions:

``` text
Invite
Change Role
Disable Membership
Remove
```

Kritik invariant:

``` text
Bir organization aktif son OWNER'ını kaybedemez.
```

Ownership transfer explicit flow ile yapılır.

------------------------------------------------------------------------

# 38. Destructive Admin Actions

Şunlar normal tek tık action olmamalıdır:

``` text
organization delete
member owner removal
license revoke
device revoke
mass session revoke
security policy downgrade
```

Risk seviyesine göre:

``` text
confirmation
recent authentication
MFA re-authentication
reason field
audit log
```

uygulanacaktır.

------------------------------------------------------------------------

# 39. Sensitive Data Redaction

API response'larında gerekmeyen veri gönderilmez.

Örnek:

``` text
password_hash          NEVER
session token          NEVER
refresh token          NEVER
TOTP secret            NEVER
recovery code          NEVER
license HMAC           normally NEVER
raw hardware IDs       normally NEVER
```

Dashboard sadece ihtiyaç duyduğu view model'i alır.

------------------------------------------------------------------------

# 40. Logging

Structured JSON logs tercih edilir.

Her request:

``` text
request_id
route
method
status
latency
admin_account_id (varsa)
organization_id (varsa)
application_id (varsa)
```

Hassas body/header loglanmaz.

Özellikle:

``` text
Authorization
Cookie
password
MFA code
license key
```

redact edilir.

------------------------------------------------------------------------

# 41. Secrets

Production secret'ları repository'ye girmez.

Örnek:

``` text
PASSWORD_PEPPER
SESSION_HMAC_KEY
MFA_ENCRYPTION_KEY
CSRF_SECRET
DATABASE_URL
```

Secret manager/environment üzerinden sağlanır.

Secret rotation planı bulunmalıdır.

------------------------------------------------------------------------

# 42. Migration Güvenliği

Mevcut production DB olduğundan migration prensibi:

``` text
expand
backfill
verify
switch
contract
```

Örneğin `application_id` eklemek:

``` text
ADD nullable
↓
backfill
↓
index concurrently / uygun yöntem
↓
FK validation
↓
application kodunu yeni kolona geçir
↓
NOT NULL
```

Mevcut `000001` ve `000002` migration dosyaları değiştirilmemelidir.
Yeni migration oluşturulmalıdır.

------------------------------------------------------------------------

# 43. Önerilen Yeni Migration Sırası

``` text
000003_admin_tenancy.up.sql
000003_admin_tenancy.down.sql

000004_rbac.up.sql
000004_rbac.down.sql

000005_application_scope.up.sql
000005_application_scope.down.sql

000006_admin_security.up.sql
000006_admin_security.down.sql

000007_audit_security_events.up.sql
000007_audit_security_events.down.sql
```

Bu ayrım migration review ve rollback'i kolaylaştırır.

------------------------------------------------------------------------

# 44. Testler

Sadece happy-path test yeterli değildir.

Özellikle authorization test matrix:

``` text
Org A admin → Org A resource        ALLOW
Org A admin → Org B resource        DENY
Viewer → read                       ALLOW
Viewer → write                      DENY
Support → permitted device reset    ALLOW
Support → org delete                DENY
Disabled member → any protected API DENY
Revoked session → protected API     DENY
```

Security tests:

``` text
CSRF
session fixation
refresh reuse
brute force
MFA replay
IDOR/BOLA
SQL injection
mass assignment
invalid UUID
oversized body
malformed JSON
expired session
cross-tenant pagination/filtering
```

------------------------------------------------------------------------

# 45. Next.js Güvenlik Kuralları

Frontend authorization yalnız UX içindir.

Örneğin Viewer için button gizlemek güzeldir ama güvenlik değildir.

``` text
UI permission check
      +
API permission check ← asıl enforcement
```

Next.js'te secret backend credentials client bundle'a konulmaz.

`NEXT_PUBLIC_*` değişkenlerinde secret bulunmaz.

------------------------------------------------------------------------

# 46. İlk Yapılacak Dashboard Sürümü

İlk sürüm:

``` text
Login + MFA
Organization selector
Application selector
Overview
Users
Licenses
Devices
Sessions
Security
Members
Settings
```

Şimdilik yapılmayacak:

``` text
Billing
SSO/SAML
Social login
Custom role editor
Webhooks UI
Public API key UI
Developer portal
SDK portal
Complex analytics
```

Ama Organization + Application + RBAC altyapısı şimdiden bulunacaktır.

------------------------------------------------------------------------

# 47. Son Hedef Mimari

``` text
                    NEXT.JS ADMIN DASHBOARD
                              │
                              │ HTTPS
                              ▼
                       EXISTING API
                              │
              ┌───────────────┼────────────────┐
              │               │                │
          Admin Auth      Authorization     Audit/Security
              │               │                │
              └───────────────┼────────────────┘
                              │
                              ▼
                         PostgreSQL
                              │
              ┌───────────────┼──────────────────┐
              │               │                  │
        Organizations      Applications        RBAC
              │               │
              │               └───────────────┐
              │                               │
              └───────────────────────────────┤
                                              ▼
                                    EXISTING STARLOADER DATA

                                    users
                                    licenses
                                    devices
                                    auth_sessions
                                    device_challenges
```

Temel prensip: **mevcut çalışan sistemi yeniden yazmak değil, güvenli ve
migration-friendly biçimde tenant + admin katmanını üzerine eklemek.**
