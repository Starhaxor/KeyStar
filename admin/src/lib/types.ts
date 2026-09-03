// Shared API types for the StarLoader admin console.
// Field names mirror the backend JSON contracts (internal/httpapi/admin_*.go).

export interface AdminIdentity {
  id: string;
  email: string;
  status: string;
  role: string;
  permissions: string[];
  mfa_enrolled: boolean;
}

export interface AdminAccount {
  id: string;
  email: string;
  status: string;
  role: string;
  permissions: string[];
  mfa_enrolled: boolean;
  created_at: string;
}

export interface AdminRole {
  id: string;
  name: string;
  description: string;
  permissions: string[];
  built_in: boolean;
  member_count: number;
}

export interface RoleMember {
  id: string;
  email: string;
  status: string;
  mfa_enrolled: boolean;
  created_at: string;
}

export interface SecurityEvent {
  id: string;
  kind: string;
  severity: string;
  admin_account_id: string;
  actor_email: string;
  user_agent: string;
  created_at: string;
}

export interface MfaEnrollment {
  secret: string;
  provisioning_uri: string;
}

export interface ConsoleUser {
  id: string;
  email: string;
  status: string;
  license_count: number;
  device_count: number;
  active_session_count: number;
  last_login_at: string | null;
  created_at: string;
}

export interface ConsoleLicense {
  id: string;
  user_id: string;
  user_email: string;
  product: string;
  status: string;
  level: number;
  max_devices: number;
  notes: string;
  expires_at: string;
  created_at: string;
}

export interface Variable {
  id: string;
  key: string;
  value: string;
  description: string;
  created_at: string;
  updated_at: string;
}

export interface ConsoleDevice {
  id: string;
  user_id: string;
  user_email: string;
  license_id: string;
  tpm_registered: boolean;
  // Presence flags only: the backend stores HMACs of hardware ids and never
  // returns the raw values.
  has_smbios_uuid: boolean;
  has_motherboard_serial: boolean;
  has_bios_serial: boolean;
  has_system_disk_serial: boolean;
  has_machine_guid: boolean;
  status: string;
  created_at: string;
  last_seen_at: string;
}

export interface ConsoleDeviceDetail {
  device: ConsoleDevice;
  product: string;
  tpm_fingerprint: string;
}

export interface ConsoleSession {
  id: string;
  user_id: string;
  user_email: string;
  license_id: string;
  status: string;
  expires_at: string;
  created_at: string;
}

export interface AuditEntry {
  id: string;
  admin_account_id: string;
  actor_email: string;
  action: string;
  resource_type: string;
  resource_id: string;
  user_agent: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface Overview {
  total_users: number;
  active_licenses: number;
  active_devices: number;
  active_sessions: number;
  recent_audit: AuditEntry[];
}

export interface TodayStats {
  logins_today: number;
  activations_today: number;
  new_devices_today: number;
  admin_logins_today: number;
  failed_logins_today: number;
  permission_denied_today: number;
  banned_users: number;
  expired_licenses: number;
}

export interface DailyStat {
  day: string;
  licenses_created: number;
  devices_registered: number;
  sessions_created: number;
  audit_events: number;
  admin_logins: number;
}

export interface UserDetail {
  user: ConsoleUser;
  notes: string;
  ban_reason: string;
  banned_at: string;
  ban_expires_at: string | null;
  licenses: ConsoleLicense[];
  devices: ConsoleDevice[];
  sessions: ConsoleSession[];
}

export interface PageResult<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export interface BanRecord {
  id: string;
  user_id: string;
  user_email: string;
  reason: string;
  expires_at: string;
  status: string;
  banned_at: string;
  lifted_at: string;
  lift_reason: string;
}

export interface LoginResponse {
  email: string;
  expires_at: string;
  mfa_required: boolean;
  mfa_token: string;
}

export interface CreatedLicense {
  license: ConsoleLicense;
  key: string;
}

export interface ApplicationCredential {
  id: string;
  name: string;
  environment: "test" | "live";
  type: "publishable" | "secret";
  scopes: string[];
  key_prefix: string;
  status: string;
  last_used_at: string | null;
  expires_at: string | null;
  created_at: string;
}

export interface DevicePolicy {
  id: string;
  application_id: string;
  tpm_policy: "optional" | "required" | "disabled";
  min_match_score: number;
  step_up_score: number;
  allow_auto_rebind: boolean;
  rebind_cooldown_seconds: number;
  max_device_changes_per_30d: number;
  created_at: string;
  updated_at: string;
}

export interface Product {
  id: string; application_id: string; name: string; slug: string; status: CatalogStatus;
  created_at: string; updated_at: string;
}

export interface Plan {
  id: string; product_id: string; name: string; code: string; level: number;
  max_devices: number; default_duration_seconds: number | null; status: CatalogStatus;
  created_at: string; updated_at: string;
}

export interface Webhook {
  id: string;
  url: string;
  status: "active" | "disabled";
  events: string[];
  created_at: string;
  updated_at: string;
}

export type WebhookDeliveryStatus =
  | "pending"
  | "delivering"
  | "delivered"
  | "failed";

export interface WebhookDelivery {
  id: string;
  webhook_id: string;
  event_type: string;
  status: WebhookDeliveryStatus;
  attempts: number;
  max_attempts: number;
  next_attempt_at: string;
  last_error: string;
  delivered_at: string | null;
  created_at: string;
}

export interface Application {
  id: string;
  organization_id: string;
  name: string;
  slug: string;
  status: "active" | "maintenance" | "suspended" | "disabled";
  environment_mode: string;
  auth_profile?: "legacy" | "proof_bound";
}

export interface ApplicationSigningKeyMetadata {
  kid: string;
  algorithm: "Ed25519";
  status: "pending" | "active" | "retiring" | "revoked";
  public_key: string;
  created_at: string;
  activated_at: string | null;
  retire_at: string | null;
  revoked_at: string | null;
}

export type CatalogStatus = "active" | "inactive" | "archived";

export interface DeviceBanRecord {
  id: string; device_id: string; user_id: string; user_email: string; reason: string;
  expires_at: string; status: "active" | "lifted" | "expired"; banned_at: string;
  lifted_at: string; lift_reason: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  status: string;
  created_at: string;
  updated_at: string;
}
