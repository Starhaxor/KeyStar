// Shared API types for the StarLoader admin console.
// Field names mirror the backend JSON contracts (internal/httpapi/admin_*.go).

export interface AdminIdentity {
  id: string;
  email: string;
  status: string;
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
  max_devices: number;
  expires_at: string;
  created_at: string;
}

export interface ConsoleDevice {
  id: string;
  user_id: string;
  user_email: string;
  license_id: string;
  tpm_registered: boolean;
  status: string;
  created_at: string;
  last_seen_at: string;
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

export interface UserDetail {
  user: ConsoleUser;
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

export interface LoginResponse {
  email: string;
  expires_at: string;
}

export interface CreatedLicense {
  license: ConsoleLicense;
  key: string;
}
