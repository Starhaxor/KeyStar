-- Tenant-safe moderation records. Device bans reference a device record;
-- raw hardware identifiers and their HMAC values remain only in devices.
alter table bans add column application_id uuid references applications(id) on delete cascade;
update bans b set application_id = u.application_id from users u where u.id = b.user_id and b.application_id is null;
alter table bans alter column application_id set not null;
create index bans_application_status_banned_at_idx on bans (application_id, status, banned_at desc, id desc);
drop index if exists bans_one_active_per_user_idx;
create unique index bans_one_active_per_application_user_idx on bans (application_id, user_id) where status = 'active';

create table device_bans (
    id uuid primary key default starloader_uuid_v7()
        constraint device_bans_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    application_id uuid not null references applications(id) on delete cascade,
    device_id uuid not null references devices(id) on delete restrict,
    reason text not null default '',
    expires_at timestamptz,
    status text not null default 'active' constraint device_bans_status_check check (status in ('active', 'lifted', 'expired')),
    banned_at timestamptz not null default now(),
    lifted_at timestamptz,
    lift_reason text not null default ''
);
create index device_bans_application_status_banned_at_idx on device_bans (application_id, status, banned_at desc, id desc);
create unique index device_bans_one_active_per_device_idx on device_bans (application_id, device_id) where status = 'active';

create table moderation_events (
    id uuid primary key default starloader_uuid_v7()
        constraint moderation_events_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    application_id uuid not null references applications(id) on delete cascade,
    subject_type text not null constraint moderation_events_subject_type_check check (subject_type in ('account_ban', 'device_ban')),
    subject_id uuid not null,
    event_type text not null,
    actor_admin_account_id uuid references admin_accounts(id) on delete set null,
    user_id uuid references users(id) on delete set null,
    device_id uuid references devices(id) on delete set null,
    reason text not null default '',
    created_at timestamptz not null default now()
);
create index moderation_events_application_subject_created_idx on moderation_events (application_id, subject_type, subject_id, created_at desc, id desc);
