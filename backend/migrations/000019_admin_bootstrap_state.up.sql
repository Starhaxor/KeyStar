create table admin_bootstrap_state (
    singleton boolean primary key default true check (singleton),
    completed_at timestamptz,
    completed_by uuid references admin_accounts(id) on delete set null,
    created_at timestamptz not null default clock_timestamp()
);

insert into admin_bootstrap_state (singleton, completed_at, completed_by)
select true,
       case when first_admin.id is null then null else clock_timestamp() end,
       first_admin.id
from (select id from admin_accounts order by created_at asc, id asc limit 1) first_admin
right join (select true) seed on true;
