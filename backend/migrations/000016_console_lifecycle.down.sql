drop index if exists licenses_plan_active_idx;
drop index if exists licenses_product_active_idx;
drop index if exists licenses_application_active_idx;
drop index if exists plans_product_active_idx;
drop index if exists products_application_active_idx;

update plans set status = 'disabled' where status in ('inactive', 'archived');
alter table plans drop constraint if exists plans_status_check;
alter table plans add constraint plans_status_check check (status in ('active', 'disabled'));

update products set status = 'disabled' where status in ('inactive', 'archived');
alter table products drop constraint if exists products_status_check;
alter table products add constraint products_status_check check (status in ('active', 'disabled'));

alter table applications drop constraint if exists applications_status_check;
alter table applications add constraint applications_status_check
    check (status in ('active', 'maintenance', 'suspended', 'disabled'));
