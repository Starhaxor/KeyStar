-- Phase 4: Products & plans normalization.
--
-- The licenses.product free-text column is replaced by a product catalog
-- (products) and an optional plan catalog (plans). Existing licenses are
-- backfilled: one product per (application, product name), one default plan
-- per product, and every license is linked to product_id + plan_id.

create table products (
    id uuid primary key default starloader_uuid_v7()
        constraint products_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    application_id uuid not null references applications(id) on delete cascade,
    name text not null,
    slug text not null,
    status text not null default 'active'
        constraint products_status_check check (status in ('active', 'disabled')),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint products_name_not_empty_check check (btrim(name) <> ''),
    constraint products_slug_normalized_check check (slug = lower(btrim(slug))),
    constraint products_application_slug_unique unique (application_id, slug)
);

create table plans (
    id uuid primary key default starloader_uuid_v7()
        constraint plans_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    product_id uuid not null references products(id) on delete cascade,
    name text not null,
    code text not null,
    level integer not null default 0,
    max_devices integer not null default 1,
    default_duration_seconds bigint,
    metadata jsonb not null default '{}'::jsonb,
    status text not null default 'active'
        constraint plans_status_check check (status in ('active', 'disabled')),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint plans_code_normalized_check check (code = lower(btrim(code))),
    constraint plans_product_code_unique unique (product_id, code)
);
create index plans_product_id_idx on plans (product_id);

-- Backfill: one product per (application, product name) used by licenses.
insert into products (application_id, name, slug)
select distinct licenses.application_id,
                licenses.product,
                lower(regexp_replace(btrim(licenses.product), '[^a-zA-Z0-9]+', '-', 'g'))
from licenses
on conflict (application_id, slug) do nothing;

-- Seed the default StarLoader product so fresh installs have a catalog.
insert into products (application_id, name, slug)
select id, name, slug
from applications
where slug = 'starloader'
on conflict do nothing;

alter table licenses add column product_id uuid references products(id) on delete restrict;
alter table licenses add column plan_id uuid references plans(id) on delete set null;

update licenses
set product_id = products.id
from products
where products.application_id = licenses.application_id
  and products.slug = lower(regexp_replace(btrim(licenses.product), '[^a-zA-Z0-9]+', '-', 'g'));

-- One default plan per product; its device allowance follows the largest
-- license of the product so backfilled licenses keep their entitlements.
insert into plans (product_id, name, code, level, max_devices)
select products.id,
       'Default',
       'default',
       1,
       coalesce((select max(licenses.max_devices)
                 from licenses
                 where licenses.product_id = products.id and licenses.max_devices > 0), 1)
from products;

update licenses
set plan_id = plans.id
from plans
where plans.product_id = licenses.product_id and plans.code = 'default';

-- The one-license-per-product guarantee now keys on the product identity
-- instead of its name. The constraint must be dropped BEFORE the product
-- column it references, because PostgreSQL drops dependent constraints
-- automatically when the column goes away.
alter table licenses drop constraint licenses_user_product_unique;
alter table licenses drop column product;
alter table licenses add constraint licenses_user_product_unique unique (user_id, product_id);
alter table licenses alter column product_id set not null;

create index licenses_product_id_idx on licenses (product_id);
create index licenses_plan_id_idx on licenses (plan_id);
