export type SidebarIcon =
  | "grid"
  | "app"
  | "users"
  | "licenses"
  | "devices"
  | "products"
  | "sessions"
  | "audit"
  | "admins"
  | "variables"
  | "credentials"
  | "webhooks"
  | "security";

export type SidebarSubItem = { label: string; href: string; permission?: string };

export type SidebarItem = {
  name: string;
  icon: SidebarIcon;
  path: string;
  permission?: string;
  permissions?: string[];
  children?: SidebarSubItem[];
};

export type SidebarSection = {
  name: string;
  items: SidebarItem[];
};

export function isSidebarItemVisible(
  item: SidebarItem,
  hasPermission: (permission: string) => boolean,
  onboardingComplete = false
) {
  if (item.path === "/onboarding" && onboardingComplete) return false;
  if (item.permission && !hasPermission(item.permission)) return false;
  return (item.permissions ?? []).every(hasPermission);
}

export const sidebarSections: SidebarSection[] = [
  {
    name: "Workspace",
    items: [
      { icon: "grid", name: "Overview", path: "/", permission: "overview.read" },
      { icon: "app", name: "Onboarding", path: "/onboarding", permissions: ["applications.read", "credentials.read", "catalog.read", "licenses.read"] },
      { icon: "app", name: "Applications", path: "/applications", permission: "applications.read" },
    ],
  },
  {
    name: "Application",
    items: [
      { icon: "users", name: "Users", path: "/users", permission: "users.read", children: [{ label: "All users", href: "/users" }, { label: "Active users", href: "/users?status=active" }, { label: "Disabled users", href: "/users?status=disabled" }] },
      { icon: "licenses", name: "Licenses", path: "/licenses", permission: "licenses.read", children: [{ label: "Directory", href: "/licenses" }] },
      { icon: "devices", name: "Devices", path: "/devices", permission: "devices.read" },
      { icon: "products", name: "Products & Plans", path: "/products", permission: "catalog.read" },
      { icon: "sessions", name: "Sessions", path: "/sessions", permission: "sessions.read" },
      { icon: "variables", name: "Variables", path: "/variables", permission: "admins.read" },
      { icon: "credentials", name: "API Credentials", path: "/credentials", permission: "credentials.read" },
      { icon: "webhooks", name: "Webhooks", path: "/webhooks", permission: "webhooks.read" },
    ],
  },
  {
    name: "Moderation",
    items: [
      { icon: "users", name: "Account bans", path: "/bans", permission: "users.read", children: [{ label: "Active bans", href: "/bans?status=active" }, { label: "Ban history", href: "/bans?status=" }] },
      { icon: "devices", name: "Device / HWID bans", path: "/device-bans", permission: "devices.read", children: [{ label: "Active device bans", href: "/device-bans?status=active" }, { label: "Device ban history", href: "/device-bans?status=" }] },
    ],
  },
  {
    name: "Administration",
    items: [
      { icon: "audit", name: "Audit Log", path: "/audit-logs", permission: "audit.read" },
      { icon: "admins", name: "Admins", path: "/admins", permission: "admins.read", children: [{ label: "Accounts", href: "/admins" }, { label: "Roles & permissions", href: "/roles", permission: "admins.read" }] },
      { icon: "security", name: "Security", path: "/security", permission: "security.read", children: [{ label: "MFA & settings", href: "/security" }, { label: "Security events", href: "/security-events" }] },
    ],
  },
];
