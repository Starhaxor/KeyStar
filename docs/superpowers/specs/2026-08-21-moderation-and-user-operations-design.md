# Moderation and User Operations Design

## Goal

Provide application-scoped account-ban and device/HWID-ban operations, complete audit history, and a richer user directory without exposing raw hardware identifiers.

## Scope and isolation

Every record is bound to `application_id`. Switching the console application changes every moderation list, summary, user count, and action target. A caller can never query or modify a ban or user belonging to another application.

## Account bans

The existing `bans` table remains the account-ban source of truth. It gains application scoping, issuer and lift actor references, and an immutable event stream. An account ban can be active, lifted, or expired; it can be permanent or have an expiry. The page exposes these views separately: Active, Temporary, Permanent, Expired, Lifted, and History.

## Device / HWID bans

A device ban targets the existing device record and its stored HMAC fingerprints. The platform never stores or returns raw HWID values. An active device ban blocks device verification for that exact device fingerprint. It does not automatically ban every linked account; linked accounts are displayed to an authorized administrator and account bans remain an explicit separate action. Device bans have the same permanent/temporary/lifted/expired lifecycle as account bans and record every create, lift, expiry, and blocked-verification event.

## History and audit

`moderation_events` is append-only and application-scoped. It records the ban type, ban ID, event kind, actor admin ID, affected user ID and device ID when applicable, reason, metadata, and timestamp. The public console response contains only safe values: user email, device record ID, and device status/fingerprint-presence flags; HMAC values and raw hardware IDs are never returned.

## User directory

The Users area contains real filtered views, not duplicated links: All users, Active users, Disabled users, and User activity. User rows show status, licenses, devices, active sessions, last login, registration time, current ban state, and a compact moderation summary. The user detail page gains a chronological activity timeline with registration, session, license, device, and moderation events.

## Navigation

The sidebar removes Bans from Users. Users contains All users, Active users, Disabled users, and Activity. A standalone Moderation category contains Account bans and Device / HWID bans; each expands to its operational views. Links use query filters that the destination pages actually consume.

## APIs

Admin API endpoints are application-scoped through `X-KeyStar-App` after normal admin authentication and RBAC. Existing `users.read`/`users.write` permissions cover account bans. New `devices.read`/`devices.write` permissions cover device bans. Read endpoints return paginated records and totals; write endpoints require CSRF and create audit/moderation events.

## Verification

Backend tests cover tenant isolation, ban lifecycle, device verification rejection, and safe JSON serialization. Frontend tests cover navigation groups, query-driven views, and API request context. Lint, frontend production build, migration-up verification, and relevant Go packages must pass before delivery.
