-- Phase 1: Tenant hardening.
--
-- application_id becomes mandatory on every end-user domain table and every
-- uniqueness guarantee becomes tenant-aware. Organizations and applications
-- gain lifecycle status fields. The global variables namespace is replaced by
-- a per-application one.

-- Backfill any rows created before the per-application rollout into the
-- default StarLoader application. This must run before NOT NULL is applied.
update users
set application_id = (select id from applications where slug = 'starloader')
where application_id is null;

update licenses
set application_id = (select id from applications where slug = 'starloader')
where application_id is null;

update devices
set application_id = (select id from applications where slug = 'starloader')
where application_id is null;

update auth_sessions
set application_id = (select id from applications where slug = 'starloader')
where application_id is null;

alter table users alter column application_id set not null;
alter table licenses alter column application_id set not null;
alter table devices alter column application_id set not null;
alter table auth_sessions alter column application_id set not null;

-- The same email may exist in different applications; uniqueness is now
-- guaranteed only within one application.
alter table users drop constraint users_email_unique;
alter table users add constraint users_application_email_unique unique (application_id, email);

-- License HMACs are keyed by the platform secret, but a tenant boundary is
-- still enforced so a license can only ever resolve inside its application.
alter table licenses drop constraint licenses_hmac_unique;
alter table licenses add constraint licenses_application_hmac_unique unique (application_id, license_hmac);

-- Organization lifecycle fields (owner of applications).
alter table organizations add column slug text;
update organizations set slug = name where slug is null or slug = '';
alter table organizations alter column slug set not null;
alter table organizations add constraint organizations_slug_normalized_check check (slug = lower(btrim(slug)));
alter table organizations add constraint organizations_slug_unique unique (slug);
alter table organizations add column status text not null default 'active';
alter table organizations add constraint organizations_status_check check (status in ('active', 'suspended', 'disabled'));
alter table organizations add column updated_at timestamptz not null default now();

-- Application lifecycle fields.
alter table applications add column status text not null default 'active';
alter table applications add constraint applications_status_check check (status in ('active', 'maintenance', 'suspended', 'disabled'));
alter table applications add column environment_mode text not null default 'separate';
alter table applications add constraint applications_environment_mode_check check (environment_mode in ('separate', 'shared'));
alter table applications add column updated_at timestamptz not null default now();

-- Variables become application-scoped: two applications may use the same key.
alter table variables add column application_id uuid references applications(id) on delete set null;
update variables
set application_id = (select id from applications where slug = 'starloader')
where application_id is null;
alter table variables alter column application_id set not null;
alter table variables drop constraint variables_key_key;
alter table variables add constraint variables_application_key_unique unique (application_id, key);
create index variables_application_id_idx on variables (application_id);

-- Tenant-aware lookup indexes on the domain tables.
create index users_application_email_idx on users (application_id, email);
create index licenses_application_hmac_idx on licenses (application_id, license_hmac);
create index auth_sessions_application_id_idx on auth_sessions (application_id);
