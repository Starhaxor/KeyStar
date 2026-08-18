-- Phase 1 tenant hardening rollback. Restores the global unique constraints
-- and makes application_id nullable again. Used only by the migration CLI;
-- production rollback of tenant hardening is not expected.
drop index if exists auth_sessions_application_id_idx;
drop index if exists licenses_application_hmac_idx;
drop index if exists users_application_email_idx;

drop index if exists variables_application_id_idx;
alter table variables drop constraint if exists variables_application_key_unique;
alter table variables add constraint variables_key_key unique (key);
alter table variables alter column application_id drop not null;

alter table applications drop constraint if exists applications_environment_mode_check;
alter table applications drop column if exists environment_mode;
alter table applications drop constraint if exists applications_status_check;
alter table applications drop column if exists status;
alter table applications drop column if exists updated_at;

alter table organizations drop constraint if exists organizations_status_check;
alter table organizations drop column if exists status;
alter table organizations drop column if exists updated_at;
alter table organizations drop constraint if exists organizations_slug_unique;
alter table organizations drop constraint if exists organizations_slug_normalized_check;
alter table organizations alter column slug drop not null;

alter table licenses drop constraint if exists licenses_application_hmac_unique;
alter table licenses add constraint licenses_hmac_unique unique (license_hmac);

alter table users drop constraint if exists users_application_email_unique;
alter table users add constraint users_email_unique unique (email);

alter table auth_sessions alter column application_id drop not null;
alter table devices alter column application_id drop not null;
alter table licenses alter column application_id drop not null;
alter table users alter column application_id drop not null;
