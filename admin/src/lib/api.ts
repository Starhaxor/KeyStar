// Admin API client. Talks to the StarLoader backend with cookie credentials;
// mutating requests carry the double-submit CSRF token from the csrf cookie.

import type {
  AdminAccount,
  AdminIdentity,
  AdminRole,
  AuditEntry,
  ConsoleDevice,
  ConsoleDeviceDetail,
  ConsoleLicense,
  DailyStat,
  ConsoleSession,
  ConsoleUser,
  CreatedLicense,
  LoginResponse,
  MfaEnrollment,
  Overview,
  PageResult,
  SecurityEvent,
  UserDetail,
} from "./types";

// Same-origin: requests go to this Next.js server and are proxied to the
// backend via the /v1 rewrite in next.config.ts. This keeps the session
// cookie on the admin origin (localhost) instead of the API's host.
export const API_URL = "";

const SESSION_COOKIE = "starloader_admin_session";
const CSRF_COOKIE = "starloader_admin_csrf";
const CSRF_HEADER = "X-CSRF-Token";

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export function readCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const match = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]*)`));
  return match ? decodeURIComponent(match[1]) : null;
}

export function hasSessionCookie(): boolean {
  return Boolean(readCookie(SESSION_COOKIE));
}

async function request<T>(
  path: string,
  init?: { method?: string; body?: unknown; isLogin?: boolean }
): Promise<T> {
  const method = init?.method ?? "GET";
  const headers: Record<string, string> = {};
  if (init?.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (method !== "GET" && method !== "HEAD") {
    const csrfToken = readCookie(CSRF_COOKIE);
    if (csrfToken) {
      headers[CSRF_HEADER] = csrfToken;
    }
  }

  let response: Response;
  try {
    response = await fetch(`${API_URL}${path}`, {
      method,
      headers,
      credentials: "include",
      body: init?.body !== undefined ? JSON.stringify(init.body) : undefined,
    });
  } catch {
    throw new ApiError(0, "NETWORK_ERROR", "Backend is unreachable");
  }

  let payload: Record<string, unknown> = {};
  try {
    payload = (await response.json()) as Record<string, unknown>;
  } catch {
    // Non-JSON error bodies fall through to the generic message below.
  }

  if (!response.ok) {
    const code =
      typeof payload.code === "string" ? payload.code : "SERVER_ERROR";
    const message =
      typeof payload.message === "string" ? payload.message : "Request failed";
    if (
      response.status === 403 &&
      code === "MFA_ENROLLMENT_REQUIRED" &&
      typeof window !== "undefined" &&
      !window.location.pathname.startsWith("/security")
    ) {
      window.location.assign("/security");
    } else if (
      response.status === 401 &&
      typeof window !== "undefined" &&
      !init?.isLogin &&
      !window.location.pathname.startsWith("/signin")
    ) {
      window.location.assign("/signin");
    }
    throw new ApiError(response.status, code, message);
  }

  return payload as T;
}

function pageQuery(page: number, extra?: Record<string, string>) {
  const params = new URLSearchParams({ page: String(page), page_size: "20" });
  for (const [key, value] of Object.entries(extra ?? {})) {
    if (value !== "") params.set(key, value);
  }
  return params.toString();
}

export const api = {
  login(email: string, password: string) {
    return request<{ ok: boolean } & LoginResponse>("/v1/admin/auth/login", {
      method: "POST",
      body: { email, password },
      isLogin: true,
    });
  },
  completeMfa(
    mfaToken: string,
    input: { code?: string; recovery_code?: string }
  ) {
    return request<{ ok: boolean }>("/v1/admin/auth/mfa", {
      method: "POST",
      body: { mfa_token: mfaToken, ...input },
      isLogin: true,
    });
  },
  mfaEnrollStart() {
    return request<{ ok: boolean } & MfaEnrollment>("/v1/admin/mfa/enroll/start", {
      method: "POST",
      body: {},
    });
  },
  mfaEnrollConfirm(code: string) {
    return request<{ ok: boolean; recovery_codes: string[] }>(
      "/v1/admin/mfa/enroll/confirm",
      { method: "POST", body: { code } }
    );
  },
  mfaDisable(password: string) {
    return request<{ ok: boolean }>("/v1/admin/mfa/disable", {
      method: "POST",
      body: { password },
    });
  },
  admins() {
    return request<{ ok: boolean; items: AdminAccount[]; total: number }>(
      "/v1/admin/admins"
    );
  },
  updateAdmin(adminId: string, update: { status?: string; role?: string }) {
    return request<{ ok: boolean }>(`/v1/admin/admins/${adminId}`, {
      method: "PATCH",
      body: update,
    });
  },
  createAdmin(email: string, password: string, role: string) {
    return request<{ ok: boolean; admin: AdminAccount }>("/v1/admin/admins", {
      method: "POST",
      body: { email, password, role },
    });
  },
  roles() {
    return request<{ ok: boolean; items: AdminRole[]; total: number }>(
      "/v1/admin/roles"
    );
  },
  securityEvents(page: number) {
    return request<{ ok: boolean } & PageResult<SecurityEvent>>(
      `/v1/admin/security-events?${pageQuery(page)}`
    );
  },
  logout() {
    return request<{ ok: boolean }>("/v1/admin/auth/logout", {
      method: "POST",
      body: {},
    });
  },
  me() {
    return request<{ ok: boolean } & AdminIdentity>("/v1/admin/me");
  },
  overview() {
    return request<{ ok: boolean } & Overview>("/v1/admin/overview");
  },
  overviewStats() {
    return request<{ ok: boolean; days: DailyStat[] }>(
      "/v1/admin/overview/stats"
    );
  },
  users(page: number, search: string) {
    return request<{ ok: boolean } & PageResult<ConsoleUser>>(
      `/v1/admin/users?${pageQuery(page, { search })}`
    );
  },
  createUser(email: string, password: string) {
    return request<{ ok: boolean; user: ConsoleUser }>("/v1/admin/users", {
      method: "POST",
      body: { email, password },
    });
  },
  userDetail(userId: string) {
    return request<{ ok: boolean } & UserDetail>(`/v1/admin/users/${userId}`);
  },
  setUserStatus(userId: string, status: "active" | "disabled") {
    return request<{ ok: boolean }>(`/v1/admin/users/${userId}`, {
      method: "PATCH",
      body: { status },
    });
  },
  revokeUserSessions(userId: string) {
    return request<{ ok: boolean; revoked: number }>(
      `/v1/admin/users/${userId}/sessions/revoke`,
      { method: "POST", body: {} }
    );
  },
  licenses(page: number) {
    return request<{ ok: boolean } & PageResult<ConsoleLicense>>(
      `/v1/admin/licenses?${pageQuery(page)}`
    );
  },
  createLicense(userEmail: string, days: number, maxDevices: number) {
    return request<{ ok: boolean } & CreatedLicense>("/v1/admin/licenses", {
      method: "POST",
      body: { user_email: userEmail, days, max_devices: maxDevices },
    });
  },
  updateLicense(licenseId: string, extendDays: number, maxDevices: number) {
    return request<{ ok: boolean }>(`/v1/admin/licenses/${licenseId}`, {
      method: "PATCH",
      body: { extend_days: extendDays, max_devices: maxDevices },
    });
  },
  revokeLicense(licenseId: string) {
    return request<{ ok: boolean }>(
      `/v1/admin/licenses/${licenseId}/revoke`,
      { method: "POST", body: {} }
    );
  },
  devices(page: number) {
    return request<{ ok: boolean } & PageResult<ConsoleDevice>>(
      `/v1/admin/devices?${pageQuery(page)}`
    );
  },
  deviceDetail(deviceId: string) {
    return request<{ ok: boolean } & ConsoleDeviceDetail>(
      `/v1/admin/devices/${deviceId}`
    );
  },
  resetDevice(deviceId: string) {
    return request<{ ok: boolean }>(`/v1/admin/devices/${deviceId}/reset`, {
      method: "POST",
      body: {},
    });
  },
  revokeDevice(deviceId: string) {
    return request<{ ok: boolean }>(`/v1/admin/devices/${deviceId}/revoke`, {
      method: "POST",
      body: {},
    });
  },
  sessions(page: number) {
    return request<{ ok: boolean } & PageResult<ConsoleSession>>(
      `/v1/admin/sessions?${pageQuery(page)}`
    );
  },
  revokeSession(sessionId: string) {
    return request<{ ok: boolean }>(
      `/v1/admin/sessions/${sessionId}/revoke`,
      { method: "POST", body: {} }
    );
  },
  auditLogs(page: number) {
    return request<{ ok: boolean } & PageResult<AuditEntry>>(
      `/v1/admin/audit-logs?${pageQuery(page)}`
    );
  },
};

export function formatDateTime(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString();
}
