alter table refresh_sessions add column license_id uuid;

update refresh_sessions rs
set license_id = d.license_id
from devices d
where d.id = rs.device_id
  and d.application_id = rs.application_id
  and d.user_id = rs.user_id;

alter table refresh_sessions alter column license_id set not null;
alter table refresh_sessions add constraint refresh_sessions_license_id_fkey
    foreign key (license_id) references licenses(id) on delete cascade;
create index idx_refresh_sessions_license on refresh_sessions(application_id, license_id, status);
