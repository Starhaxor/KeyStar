// Admin API client. Talks to the StarLoader backend with cookie credentials;
// mutating requests carry the double-submit CSRF token from the csrf cookie.

import type {
  AdminAccount,
  Application,
  Organization,
  ApplicationCredential,
  AdminIdentity,
  AdminRole,
  AuditEntry,
  BanRecord,
  ConsoleDevice,
  ConsoleDeviceDetail,
  ConsoleLicense,
  DailyStat,
  DevicePolicy,
  ConsoleSession,
  ConsoleUser,
  CreatedLicense,
  LoginResponse,
  MfaEnrollment,
  Overview,
  PageResult,
  Plan,
  Product,
  RoleMember,
  SecurityEvent,
  TodayStats,
  UserDetail,
  Variable,
  Webhook,
} from "./types";

// Same-origin: requests go to this Next.js server and are proxied to the
// backend via the /v1 rewrite in next.config.ts. This keeps the session
// cookie on the admin origin (localhost) instead of the API's host.
export const API_URL = "";

const SESSION_COOKIE = "starloader_admin_session";
const CSRF_COOKIE = "starloader_admin_csrf";
const CSRF_HEADER = "X-CSRF-Token";
const APPLICATION_COOKIE = "keystar_application_id";
const APPLICATION_HEADER = "X-KeyStar-App";
export const applicationCookieName = APPLICATION_COOKIE;

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
  init?: { method?: string; body?: unknown; isLogin?: boolean; skipApplicationHeader?: boolean }
): Promise<T> {
  const method = init?.method ?? "GET";
  const headers: Record<string, string> = {};
	const applicationID = readCookie(APPLICATION_COOKIE);
	if (applicationID && !init?.skipApplicationHeader) headers[APPLICATION_HEADER] = applicationID;
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
      cache: "no-store",
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

export async function fetchAllPages<T>(load: (page: number, pageSize: number) => Promise<PageResult<T>>, pageSize = 100): Promise<T[]> {
  const first = await load(1, pageSize);
  const items = [...first.items];
  for (let page = 2; page <= Math.ceil(first.total / pageSize); page += 1) {
    items.push(...(await load(page, pageSize)).items);
  }
  return items;
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
  // Turns an existing end-user into a dashboard admin. The backend generates
  // a strong temporary password (returned once) — the user's client password
  // is never reused for console access.
  promoteToAdmin(userId: string, role: string) {
    return request<{
      ok: boolean;
      admin: AdminAccount;
      temp_password?: string;
    }>(`/v1/admin/users/${userId}/promote`, {
      method: "POST",
      body: { role },
    });
  },
  // Resets another admin's password. When password is empty the backend
  // generates one and returns it as temp_password exactly once.
  resetAdminPassword(adminId: string, password?: string) {
    return request<{ ok: boolean; temp_password?: string }>(
      `/v1/admin/admins/${adminId}/reset-password`,
      { method: "POST", body: { password: password ?? "" } }
    );
  },
  roles() {
    return request<{ ok: boolean; items: AdminRole[]; total: number }>(
      "/v1/admin/roles"
    );
  },
  applications() {
    return request<{ ok: boolean; items: Application[] }>("/v1/admin/applications", { skipApplicationHeader: true });
  },
  organizations() {
    return request<{ ok: boolean; items: Organization[] }>("/v1/admin/applications/organizations", { skipApplicationHeader: true });
  },
  createOrganization(name: string) {
    return request<{ ok: boolean; organization: Organization }>("/v1/admin/applications/organizations", { method: "POST", body: { name } });
  },
  createApplication(input: { organization_id: string; name: string; slug: string }) {
    return request<{ ok: boolean; application: Application }>("/v1/admin/applications", { method: "POST", body: input });
  },
  products() {
    return request<{ ok: boolean; items: Product[] }>("/v1/admin/products");
  },
  createProduct(name: string, slug = "") {
    return request<{ ok: boolean; product: Product }>("/v1/admin/products", { method: "POST", body: { name, slug } });
  },
  plans(productId: string) {
    return request<{ ok: boolean; items: Plan[] }>(`/v1/admin/products/${productId}/plans`);
  },
  createPlan(productId: string, input: { name: string; code: string; level: number; max_devices: number }) {
    return request<{ ok: boolean; plan: Plan }>(`/v1/admin/products/${productId}/plans`, { method: "POST", body: input });
  },
  webhooks() {
    return request<{ ok: boolean; items: Webhook[] }>("/v1/admin/webhooks");
  },
  createWebhook(input: { url: string; events: string[] }) {
    return request<{ ok: boolean; webhook: Webhook; secret: string }>("/v1/admin/webhooks", { method: "POST", body: input });
  },
  updateWebhook(webhookId: string, input: { url?: string; status?: "active" | "disabled"; events?: string[] }) {
    return request<{ ok: boolean; webhook: Webhook }>(`/v1/admin/webhooks/${webhookId}`, { method: "PATCH", body: input });
  },
  deleteWebhook(webhookId: string) {
    return request<{ ok: boolean }>(`/v1/admin/webhooks/${webhookId}`, { method: "DELETE", body: {} });
  },
  webhookDeliveries(webhookId: string, page = 1) {
    return request<{ ok: boolean } & PageResult<import("./types").WebhookDelivery>>(
      `/v1/admin/webhooks/${webhookId}/deliveries?${pageQuery(page)}`
    );
  },
  retryWebhookDelivery(webhookId: string, deliveryId: string) {
    return request<{ ok: boolean }>(
      `/v1/admin/webhooks/${webhookId}/deliveries/${deliveryId}/retry`,
      { method: "POST", body: {} }
    );
  },
  credentials() {
    return request<{ ok: boolean; credentials: ApplicationCredential[] }>(
      "/v1/admin/credentials"
    );
  },
  createCredential(input: {
    name: string;
    environment: "test" | "live";
    type: "publishable" | "secret";
    scopes: string[];
    expires_in?: string;
  }) {
    return request<{ ok: boolean; credential: ApplicationCredential; key: string }>(
      "/v1/admin/credentials",
      { method: "POST", body: input }
    );
  },
  revokeCredential(credentialId: string) {
    return request<{ ok: boolean }>(
      `/v1/admin/credentials/${credentialId}/revoke`,
      { method: "POST", body: {} }
    );
  },
  devicePolicy() {
    return request<{ ok: boolean; policy: DevicePolicy }>(
      "/v1/admin/devices/policy"
    );
  },
  updateDevicePolicy(input: Omit<DevicePolicy, "id" | "application_id" | "created_at" | "updated_at">) {
    return request<{ ok: boolean; policy: DevicePolicy }>(
      "/v1/admin/devices/policy",
      { method: "PATCH", body: input }
    );
  },
  resetDevicePolicy() {
    return request<{ ok: boolean }>("/v1/admin/devices/policy", {
      method: "DELETE",
      body: {},
    });
  },
  createRole(
    name: string,
    description: string,
    permissions: string[]
  ) {
    return request<{ ok: boolean; role: AdminRole }>("/v1/admin/roles", {
      method: "POST",
      body: { name, description, permissions },
    });
  },
  roleMembers(roleId: string) {
    return request<{ ok: boolean; items: RoleMember[]; total: number }>(
      `/v1/admin/roles/${roleId}/members`
    );
  },
  updateRole(roleId: string, description: string, permissions: string[]) {
    return request<{ ok: boolean }>(`/v1/admin/roles/${roleId}`, {
      method: "PATCH",
      body: { description, permissions },
    });
  },
  deleteRole(roleId: string) {
    return request<{ ok: boolean }>(`/v1/admin/roles/${roleId}`, {
      method: "DELETE",
      body: {},
    });
  },
  securityEvents(page: number, search = "", severity = "") {
    return request<{ ok: boolean } & PageResult<SecurityEvent>>(
      `/v1/admin/security-events?${pageQuery(page, { search, severity })}`
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
  myActivity() {
    return request<{ ok: boolean; items: AuditEntry[] }>("/v1/admin/me/activity");
  },
  overview() {
    return request<{ ok: boolean } & Overview>("/v1/admin/overview");
  },
  overviewStats(days = 14) {
    return request<{ ok: boolean; days: DailyStat[] }>(
      `/v1/admin/overview/stats?${new URLSearchParams({ days: String(days) })}`
    );
  },
  overviewToday() {
    return request<{ ok: boolean } & TodayStats>("/v1/admin/overview/today");
  },
  users(page: number, search: string, status = "") {
    return request<{ ok: boolean } & PageResult<ConsoleUser>>(
      `/v1/admin/users?${pageQuery(page, { search, status })}`
    );
  },
  bans(page: number, search: string, status = "") {
    return request<{ ok: boolean } & PageResult<BanRecord>>(
      `/v1/admin/bans?${pageQuery(page, { search, status })}`
    );
  },
  deviceBans(page: number, status = "") {
    return request<{ ok: boolean } & PageResult<import("./types").DeviceBanRecord>>(`/v1/admin/device-bans?${pageQuery(page, { status })}`);
  },
  createDeviceBan(input: { device_id: string; reason: string; expires_at?: string }) {
    return request<{ ok: boolean; ban: import("./types").DeviceBanRecord }>("/v1/admin/device-bans", { method: "POST", body: input });
  },
  liftDeviceBan(banId: string, reason = "") { return request<{ ok: boolean }>(`/v1/admin/device-bans/${banId}/lift`, { method: "POST", body: { reason } }); },
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
  bulkSetUserStatus(ids: string[], status: "active" | "disabled") {
    return request<{ ok: boolean; updated: number }>(
      "/v1/admin/users/bulk-status",
      { method: "POST", body: { ids, status } }
    );
  },
  bulkRevokeUserSessions(ids: string[]) {
    return request<{ ok: boolean; revoked: number }>(
      "/v1/admin/users/bulk/revoke-sessions",
      { method: "POST", body: { ids } }
    );
  },
  revokeUserSessions(userId: string) {
    return request<{ ok: boolean; revoked: number }>(
      `/v1/admin/users/${userId}/sessions/revoke`,
      { method: "POST", body: {} }
    );
  },
  // permanent=true bans forever; otherwise duration_value + duration_unit
  // (hours/days/weeks/months/years) produce a temporary ban that reopens
  // automatically when the deadline passes.
  banUser(
    userId: string,
    reason: string,
    options: { permanent: boolean; durationValue?: number; durationUnit?: string }
  ) {
    return request<{ ok: boolean; ban_until?: string; permanent?: boolean }>(
      `/v1/admin/users/${userId}/ban`,
      {
        method: "POST",
        body: {
          reason,
          permanent: options.permanent,
          duration_value: options.durationValue ?? 0,
          duration_unit: options.durationUnit ?? "",
        },
      }
    );
  },
  unbanUser(userId: string) {
    return request<{ ok: boolean }>(`/v1/admin/users/${userId}/unban`, {
      method: "POST",
      body: {},
    });
  },
  setUserNotes(userId: string, notes: string) {
    return request<{ ok: boolean; notes: string }>(
      `/v1/admin/users/${userId}/notes`,
      { method: "PATCH", body: { notes } }
    );
  },
  resetUserDevices(userId: string) {
    return request<{ ok: boolean; devices: number }>(
      `/v1/admin/users/${userId}/reset-devices`,
      { method: "POST", body: {} }
    );
  },
  // Sets a new password for an end-user. When password is empty the backend
  // generates one and returns it as temp_password exactly once.
  resetUserPassword(userId: string, password?: string) {
    return request<{
      ok: boolean;
      password_set: boolean;
      temp_password?: string;
    }>(`/v1/admin/users/${userId}/password`, {
      method: "POST",
      body: { password: password ?? "" },
    });
  },
  licenses(page: number, pageSize = 20, search = "", status = "") {
    return request<{ ok: boolean } & PageResult<ConsoleLicense>>(
      `/v1/admin/licenses?${pageQuery(page, { search, status, page_size: String(pageSize) })}`
    );
  },
  createLicense(
    userEmail: string,
    duration: { value: number; unit: string },
    maxDevices: number,
    catalog?: { productId: string; planId: string }
  ) {
    return request<{ ok: boolean } & CreatedLicense>("/v1/admin/licenses", {
      method: "POST",
      body: {
        user_email: userEmail,
        value: duration.value,
        unit: duration.unit,
        max_devices: maxDevices,
        ...(catalog ? { product_id: catalog.productId, plan_id: catalog.planId } : {}),
      },
    });
  },
  updateLicense(
    licenseId: string,
    input: {
      extendValue?: number;
      extendUnit?: string;
      maxDevices?: number;
      level?: number;
      notes?: string;
    }
  ) {
    const body: Record<string, unknown> = {};
    if (input.extendValue) {
      body.extend_value = input.extendValue;
      body.extend_unit = input.extendUnit ?? "days";
    }
    if (input.maxDevices !== undefined) body.max_devices = input.maxDevices;
    if (input.level !== undefined) body.level = input.level;
    if (input.notes !== undefined) body.notes = input.notes;
    return request<{ ok: boolean }>(`/v1/admin/licenses/${licenseId}`, {
      method: "PATCH",
      body,
    });
  },
  variables() {
    return request<{ ok: boolean; items: Variable[]; total: number }>(
      "/v1/admin/variables"
    );
  },
  createVariable(key: string, value: string, description: string) {
    return request<{ ok: boolean; variable: Variable }>(
      "/v1/admin/variables",
      { method: "POST", body: { key, value, description } }
    );
  },
  updateVariable(variableId: string, value: string, description: string) {
    return request<{ ok: boolean }>(`/v1/admin/variables/${variableId}`, {
      method: "PATCH",
      body: { value, description },
    });
  },
  deleteVariable(variableId: string) {
    return request<{ ok: boolean }>(`/v1/admin/variables/${variableId}`, {
      method: "DELETE",
      body: {},
    });
  },
  revokeLicense(licenseId: string) {
    return request<{ ok: boolean }>(
      `/v1/admin/licenses/${licenseId}/revoke`,
      { method: "POST", body: {} }
    );
  },
  devices(page: number, pageSize = 20, search = "", status = "") {
    return request<{ ok: boolean } & PageResult<ConsoleDevice>>(
      `/v1/admin/devices?${pageQuery(page, { search, status, page_size: String(pageSize) })}`
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
  sessions(page: number, pageSize = 20, search = "", status = "") {
    return request<{ ok: boolean } & PageResult<ConsoleSession>>(
      `/v1/admin/sessions?${pageQuery(page, { search, status, page_size: String(pageSize) })}`
    );
  },
  revokeSession(sessionId: string) {
    return request<{ ok: boolean }>(
      `/v1/admin/sessions/${sessionId}/revoke`,
      { method: "POST", body: {} }
    );
  },
  auditLogs(page: number, search = "") {
    return request<{ ok: boolean } & PageResult<AuditEntry>>(
      `/v1/admin/audit-logs?${pageQuery(page, { search })}`
    );
  },
};

export { formatDateTime } from "./time";
