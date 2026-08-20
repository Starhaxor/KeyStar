-- Phase 5: Device Policy
-- Per-application configuration for TPM requirements, scoring thresholds,
-- rebind rules and device-change rate limiting.

create table application_device_policies (
    id uuid primary key default starloader_uuid_v7()
        constraint application_device_policies_id_uuid_v7_check check ((get_byte(uuid_send(id), 6) >> 4) = 7),
    application_id uuid unique not null references applications(id) on delete cascade,
    -- TPM enforcement: "required" | "preferred" | "optional"
    tpm_policy text not null default 'optional',
    -- Minimum device-match score (0–100) for a device to be recognized.
    -- Existing hard-coded default is 70.
    min_match_score integer not null default 70,
    -- Score below min_match_score but above step_up_score triggers a
    -- step-up challenge (re-verify with additional hardware signals).
    step_up_score integer not null default 40,
    -- Whether a device whose TPM key changed may re-bind without admin
    -- intervention after the cooldown period.
    allow_auto_rebind boolean not null default true,
    rebind_cooldown_seconds bigint not null default 86400,
    -- Maximum number of device-binding changes a single license may
    -- perform within any rolling 30-day window.
    max_device_changes_per_30d integer not null default 5,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index idx_device_policies_application on application_device_policies(application_id);
