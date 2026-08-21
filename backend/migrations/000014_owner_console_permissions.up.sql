-- Ensure existing owner accounts can access every first-party console area.
-- These permissions were introduced after the original owner role seed, so
-- installations already migrated to earlier versions need this backfill.
update roles
set permissions = permissions || '{applications.read,applications.write,catalog.read,catalog.write,webhooks.read,webhooks.write}'
where name = 'owner';
