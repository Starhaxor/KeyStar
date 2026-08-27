drop index if exists idx_refresh_sessions_license;
alter table refresh_sessions drop constraint if exists refresh_sessions_license_id_fkey;
alter table refresh_sessions drop column if exists license_id;
