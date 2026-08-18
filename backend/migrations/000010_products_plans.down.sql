-- Reintroduce the free-text product column and drop the catalogs.
alter table licenses drop constraint licenses_user_product_unique;
alter table licenses add column product text;
update licenses set product = products.name from products where products.id = licenses.product_id;
alter table licenses alter column product set not null;
alter table licenses drop column plan_id;
alter table licenses drop column product_id;
alter table licenses add constraint licenses_user_product_unique unique (user_id, product);
drop table plans;
drop table products;
