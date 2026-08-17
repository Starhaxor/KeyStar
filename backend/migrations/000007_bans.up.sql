create table bans (
    id uuid primary key default starloader_uuid_v7()
        constraint bans_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    user_id uuid not null references users(id) on delete cascade,
    reason text not null default '',
    expires_at timestamptz,
    status text not null default 'active'
        constraint bans_status_check check (status in ('active', 'lifted', 'expired')),
    banned_at timestamptz not null default now(),
    lifted_at timestamptz,
    lift_reason text not null default ''
);

create index bans_user_id_idx on bans (user_id);
create index bans_banned_at_idx on bans (banned_at desc, id desc);
create unique index bans_one_active_per_user_idx on bans (user_id) where status = 'active';

insert into bans (user_id, reason, expires_at, status, banned_at)
select id, ban_reason, ban_expires_at, 'active', coalesce(banned_at, now())
from users
where status = 'banned';
