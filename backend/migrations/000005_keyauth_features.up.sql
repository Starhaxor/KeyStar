alter table users
    add column notes text not null default '',
    add column ban_reason text not null default '',
    add column banned_at timestamptz;

alter table licenses
    add column level integer not null default 1,
    add column notes text not null default '';

create table variables (
    id uuid primary key default gen_random_uuid(),
    key text not null unique,
    value text not null default '',
    description text not null default '',
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp()
);
