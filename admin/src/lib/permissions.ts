// Assignable console permissions, mirroring the backend domain constants.
// Grouped by resource so the role editor can render a readable picker.
export const PERMISSION_GROUPS: {
  resource: string;
  permissions: { id: string; label: string }[];
}[] = [
  {
    resource: "Overview",
    permissions: [{ id: "overview.read", label: "View dashboard" }],
  },
  {
    resource: "Users",
    permissions: [
      { id: "users.read", label: "View users" },
      { id: "users.write", label: "Manage users (create, disable, reset)" },
    ],
  },
  {
    resource: "Licenses",
    permissions: [
      { id: "licenses.read", label: "View licenses" },
      { id: "licenses.write", label: "Manage licenses (issue, extend, revoke)" },
    ],
  },
  {
    resource: "Devices",
    permissions: [
      { id: "devices.read", label: "View devices" },
      { id: "devices.write", label: "Manage devices (reset, revoke)" },
    ],
  },
  {
    resource: "Sessions",
    permissions: [
      { id: "sessions.read", label: "View sessions" },
      { id: "sessions.write", label: "Revoke sessions" },
    ],
  },
  {
    resource: "Audit",
    permissions: [{ id: "audit.read", label: "View audit log" }],
  },
  {
    resource: "Security",
    permissions: [{ id: "security.read", label: "View security events" }],
  },
  {
    resource: "API credentials",
    permissions: [
      { id: "credentials.read", label: "View API credentials" },
      { id: "credentials.write", label: "Create and revoke API credentials" },
    ],
  },
  {
    resource: "Applications",
    permissions: [
      { id: "applications.read", label: "View organizations and applications" },
      { id: "applications.write", label: "Manage organizations and applications" },
    ],
  },
  {
    resource: "Catalog",
    permissions: [
      { id: "catalog.read", label: "View products and plans" },
      { id: "catalog.write", label: "Manage products and plans" },
    ],
  },
  {
    resource: "Webhooks",
    permissions: [
      { id: "webhooks.read", label: "View webhook endpoints" },
      { id: "webhooks.write", label: "Manage webhook endpoints" },
    ],
  },
  {
    resource: "Admins",
    permissions: [
      { id: "admins.read", label: "View admin accounts" },
      { id: "admins.write", label: "Manage admins and roles" },
    ],
  },
];

export function permissionLabel(id: string): string {
  for (const group of PERMISSION_GROUPS) {
    for (const permission of group.permissions) {
      if (permission.id === id) return permission.label;
    }
  }
  return id;
}
