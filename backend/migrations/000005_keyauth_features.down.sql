drop table if exists variables;

alter table licenses
    drop column if exists notes,
    drop column if exists level;

alter table users
    drop column if exists banned_at,
    drop column if exists ban_reason,
    drop column if exists notes;
