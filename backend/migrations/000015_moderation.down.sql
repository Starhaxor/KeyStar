drop table if exists moderation_events;
drop table if exists device_bans;
drop index if exists bans_one_active_per_application_user_idx;
create unique index bans_one_active_per_user_idx on bans (user_id) where status = 'active';
drop index if exists bans_application_status_banned_at_idx;
alter table bans drop column if exists application_id;
