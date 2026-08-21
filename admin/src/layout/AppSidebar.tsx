"use client";
import React, { useEffect, useState } from "react";
import Link from "next/link";
import Image from "next/image";
import { usePathname } from "next/navigation";
import { useSidebar } from "../context/SidebarContext";
import { useAdminIdentity } from "../context/AdminIdentityContext";
import {
  AlertIcon,
  BoxCubeIcon,
  ChevronLeftIcon,
  ChevronDownIcon,
  DocsIcon,
  GridIcon,
  GroupIcon,
  HorizontaLDots,
  ListIcon,
  LockIcon,
  TimeIcon,
  UserCircleIcon,
} from "../icons/index";

type SubItem = { label: string; href: string; permission?: string };
type NavItem = {
  name: string;
  icon: React.ReactNode;
  path: string;
  // permission gates the item in the UI; undefined means always visible.
  permission?: string;
  children?: SubItem[];
};

const navItems: NavItem[] = [
  { icon: <GridIcon />, name: "Overview", path: "/", permission: "overview.read" },
  {
    icon: <UserCircleIcon />,
    name: "Users",
    path: "/users",
    permission: "users.read",
    children: [
      { label: "Directory", href: "/users" },
      { label: "Bans", href: "/bans" },
    ],
  },
  {
    icon: <DocsIcon />,
    name: "Licenses",
    path: "/licenses",
    permission: "licenses.read",
    children: [{ label: "Directory", href: "/licenses" }],
  },
  { icon: <BoxCubeIcon />, name: "Devices", path: "/devices", permission: "devices.read" },
  { icon: <TimeIcon />, name: "Sessions", path: "/sessions", permission: "sessions.read" },
  {
    icon: <ListIcon />,
    name: "Audit Log",
    path: "/audit-logs",
    permission: "audit.read",
  },
  {
    icon: <GroupIcon />,
    name: "Admins",
    path: "/admins",
    permission: "admins.read",
    children: [
      { label: "Accounts", href: "/admins" },
      { label: "Roles & permissions", href: "/roles", permission: "admins.read" },
    ],
  },
  {
    icon: <ListIcon />,
    name: "Variables",
    path: "/variables",
    permission: "admins.read",
  },
  {
    icon: <LockIcon />,
    name: "API Credentials",
    path: "/credentials",
    permission: "credentials.read",
  },
  {
    icon: <LockIcon />,
    name: "Security",
    path: "/security",
    children: [
      { label: "MFA & settings", href: "/security" },
      { label: "Security events", href: "/security-events" },
    ],
  },
];

function initialsFor(email: string): string {
  const local = email.split("@")[0] ?? "";
  return local.slice(0, 2).toUpperCase() || "A";
}

const AppSidebar: React.FC = () => {
  const { isExpanded, isMobileOpen, isHovered, setIsHovered, toggleSidebar } =
    useSidebar();
  const pathname = usePathname();
  const { hasPermission, identity } = useAdminIdentity();

  // Which sections have their submenu expanded. The section containing the
  // current route is opened automatically.
  const [openSections, setOpenSections] = useState<Record<string, boolean>>({});

  const isActive = (path: string) =>
    path === "/" ? pathname === "/" : pathname === path || pathname.startsWith(`${path}/`);

  // A sub-item is the current location only when it is the canonical page
  // itself; query-string shortcuts are actions, never highlighted.
  const isChildActive = (child: SubItem) => {
    if (child.href.includes("?")) return false;
    return pathname === child.href || pathname.startsWith(`${child.href}/`);
  };

  const isSectionActive = (nav: NavItem) =>
    isActive(nav.path) || (nav.children ?? []).some(isChildActive);

  // On navigation only the section containing the current route stays open;
  // every other section collapses. This keeps the menu deterministic and
  // prevents stale sections from lingering expanded.
  useEffect(() => {
    setOpenSections(() => {
      const next: Record<string, boolean> = {};
      for (const nav of navItems) {
        if (nav.children && isSectionActive(nav)) next[nav.name] = true;
      }
      return next;
    });
  }, [pathname]); // eslint-disable-line react-hooks/exhaustive-deps

  const toggleSection = (name: string) =>
    setOpenSections((prev) => ({ ...prev, [name]: !prev[name] }));

  const visibleItems = navItems.filter(
    (nav) => !nav.permission || hasPermission(nav.permission)
  );

  const expanded = isExpanded || isHovered || isMobileOpen;
  const email = identity?.email ?? "";

  return (
    <aside
      className={`fixed mt-16 flex flex-col lg:mt-0 top-0 px-5 left-0 bg-white dark:bg-gray-900 dark:border-gray-800 text-gray-900 h-screen transition-all duration-300 ease-in-out z-50 border-r border-gray-200 
        ${
          expanded
            ? "w-[290px]"
            : "w-[90px]"
        }
        ${isMobileOpen ? "translate-x-0" : "-translate-x-full"}
        lg:translate-x-0`}
      onMouseEnter={() => !isExpanded && setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      <div
        className={`py-8 flex  ${
          !expanded ? "lg:justify-center" : "justify-start"
        }`}
      >
        <Link href="/">
          {expanded ? (
            <span className="text-lg font-semibold text-gray-800 dark:text-white/90">
              KeyStar Admin
            </span>
          ) : (
            <Image
              src="/images/logo/logo-icon.svg"
              alt="Logo"
              width={32}
              height={32}
            />
          )}
        </Link>
      </div>
      <div className="flex flex-col overflow-y-auto duration-300 ease-linear no-scrollbar grow">
        <nav className="mb-6">
          <div className="flex flex-col gap-4">
            <div>
              <h2
                className={`mb-4 text-xs uppercase flex leading-[20px] text-gray-400 ${
                  !expanded ? "lg:justify-center" : "justify-start"
                }`}
              >
                {expanded ? "Menu" : <HorizontaLDots />}
              </h2>
              <ul className="flex flex-col gap-4">
                {visibleItems.map((nav) => {
                  const active = isSectionActive(nav);
                  const sectionOpen = Boolean(openSections[nav.name]);
                  return (
                    <li key={nav.name}>
                      <div className="relative flex items-center">
                        {active && (
                          <span className="absolute left-[-16px] top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full bg-brand-500" />
                        )}
                        {nav.children ? (
                          // Sections with a submenu: the whole row toggles it.
                          <button
                            type="button"
                            onClick={() => toggleSection(nav.name)}
                            title={expanded ? undefined : nav.name}
                            aria-expanded={sectionOpen}
                            className={`menu-item group flex-1 ${
                              active ? "menu-item-active" : "menu-item-inactive"
                            }`}
                          >
                            <span
                              className={
                                active
                                  ? "menu-item-icon-active"
                                  : "menu-item-icon-inactive"
                              }
                            >
                              {nav.icon}
                            </span>
                            {expanded && (
                              <span className="flex-1 text-left">
                                {nav.name}
                              </span>
                            )}
                            {expanded && (
                              <ChevronDownIcon
                                className={`h-4 w-4 shrink-0 transition-transform ${
                                  sectionOpen ? "rotate-180" : ""
                                } ${
                                  active
                                    ? "text-brand-500 dark:text-brand-400"
                                    : "text-gray-400"
                                }`}
                              />
                            )}
                          </button>
                        ) : (
                          <Link
                            href={nav.path}
                            title={expanded ? undefined : nav.name}
                            className={`menu-item group flex-1 ${
                              active ? "menu-item-active" : "menu-item-inactive"
                            }`}
                          >
                            <span
                              className={
                                active
                                  ? "menu-item-icon-active"
                                  : "menu-item-icon-inactive"
                              }
                            >
                              {nav.icon}
                            </span>
                            {expanded && (
                              <span className="flex-1 text-left">
                                {nav.name}
                              </span>
                            )}
                          </Link>
                        )}
                      </div>
                      {expanded && nav.children && sectionOpen && (
                        <ul className="mt-1 space-y-1">
                          {nav.children
                            .filter(
                              (child) =>
                                !child.permission ||
                                hasPermission(child.permission)
                            )
                            .map((child) => {
                              const childActive = isChildActive(child);
                              return (
                                <li key={child.href}>
                                  <Link
                                    href={child.href}
                                    className={`menu-dropdown-item ${
                                      childActive
                                        ? "menu-dropdown-item-active"
                                        : "menu-dropdown-item-inactive"
                                    }`}
                                  >
                                    <span
                                      className={`h-1.5 w-1.5 rounded-full bg-current ${
                                        childActive ? "opacity-100" : "opacity-40"
                                      }`}
                                    />
                                    {child.label}
                                  </Link>
                                </li>
                              );
                            })}
                        </ul>
                      )}
                    </li>
                  );
                })}
              </ul>
            </div>
          </div>
        </nav>
      </div>

      {/* Footer: user card + collapse toggle */}
      <div className="border-t border-gray-200 py-4 dark:border-gray-800">
        {expanded ? (
          <div className="flex items-center gap-3">
            <Link
              href="/profile"
              className="flex min-w-0 flex-1 items-center gap-3 rounded-lg p-1.5 transition-colors hover:bg-gray-100 dark:hover:bg-white/5"
            >
              <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-brand-500 text-sm font-semibold text-white">
                {initialsFor(email)}
              </span>
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium text-gray-800 dark:text-white/90">
                  {email || "Administrator"}
                </span>
                <span className="block text-xs text-gray-400">View profile</span>
              </span>
            </Link>
            <button
              onClick={toggleSidebar}
              aria-label="Collapse sidebar"
              className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-white/5 dark:hover:text-white"
            >
              <ChevronLeftIcon />
            </button>
          </div>
        ) : (
          <div className="flex flex-col items-center gap-3">
            <Link
              href="/profile"
              title={email || "Profile"}
              className="flex h-9 w-9 items-center justify-center rounded-full bg-brand-500 text-sm font-semibold text-white"
            >
              {initialsFor(email)}
            </Link>
            <button
              onClick={toggleSidebar}
              aria-label="Expand sidebar"
              className="flex h-8 w-8 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-white/5 dark:hover:text-white"
            >
              <ChevronLeftIcon className="rotate-180" />
            </button>
          </div>
        )}
      </div>
    </aside>
  );
};

export default AppSidebar;
