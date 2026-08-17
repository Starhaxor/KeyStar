drop table if exists variables;

alter table licenses
    drop column if exists notes,
    drop column if exists level;

alter table users
    drop column if exists banned_at,
    drop column if exists ban_reason,
    drop column if exists notes;

alter table users drop constraint if exists users_status_check;
alter table users add constraint users_status_check check (status in ('active', 'disabled'));
