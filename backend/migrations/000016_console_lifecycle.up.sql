-- Console lifecycle persistence. Catalog rows are retained for historical
-- license references; only their operational state changes.

alter table applications drop constraint if exists applications_status_check;
update applications set status = 'disabled' where status = 'suspended';
alter table applications add constraint applications_status_check
    check (status in ('active', 'maintenance', 'disabled'));

alter table products drop constraint if exists products_status_check;
update products set status = 'inactive' where status = 'disabled';
alter table products add constraint products_status_check
    check (status in ('active', 'inactive', 'archived'));

alter table plans drop constraint if exists plans_status_check;
update plans set status = 'inactive' where status = 'disabled';
alter table plans add constraint plans_status_check
    check (status in ('active', 'inactive', 'archived'));

create index products_application_active_idx on products (application_id)
    where status = 'active';
create index plans_product_active_idx on plans (product_id)
    where status = 'active';
create index licenses_application_active_idx on licenses (application_id)
    where status = 'active';
create index licenses_product_active_idx on licenses (product_id)
    where status = 'active';
create index licenses_plan_active_idx on licenses (plan_id)
    where status = 'active';
