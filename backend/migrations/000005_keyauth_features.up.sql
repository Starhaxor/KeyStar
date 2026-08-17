alter table users
    add column notes text not null default '',
    add column ban_reason text not null default '',
    add column banned_at timestamptz;

alter table users drop constraint users_status_check;
alter table users add constraint users_status_check check (status in ('active', 'disabled', 'banned'));

alter table licenses
    add column level integer not null default 1,
    add column notes text not null default '';

create table variables (
    id uuid primary key default starloader_uuid_v7()
        constraint variables_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    key text not null unique,
    value text not null default '',
    description text not null default '',
    created_at timestamptz not null default clock_timestamp(),
    updated_at timestamptz not null default clock_timestamp()
);
