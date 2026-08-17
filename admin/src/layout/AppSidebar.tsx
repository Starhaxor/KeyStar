"use client";
import React from "react";
import Link from "next/link";
import Image from "next/image";
import { usePathname } from "next/navigation";
import { useSidebar } from "../context/SidebarContext";
import { useAdminIdentity } from "../context/AdminIdentityContext";
import {
  AlertIcon,
  BoxCubeIcon,
  ChevronLeftIcon,
  DocsIcon,
  GridIcon,
  GroupIcon,
  HorizontaLDots,
  ListIcon,
  LockIcon,
  TimeIcon,
  UserCircleIcon,
} from "../icons/index";

type NavItem = {
  name: string;
  icon: React.ReactNode;
  path: string;
  // permission gates the item in the UI; undefined means always visible.
  permission?: string;
};

const navItems: NavItem[] = [
  { icon: <GridIcon />, name: "Overview", path: "/", permission: "overview.read" },
  { icon: <UserCircleIcon />, name: "Users", path: "/users", permission: "users.read" },
  { icon: <DocsIcon />, name: "Licenses", path: "/licenses", permission: "licenses.read" },
  { icon: <BoxCubeIcon />, name: "Devices", path: "/devices", permission: "devices.read" },
  { icon: <TimeIcon />, name: "Sessions", path: "/sessions", permission: "sessions.read" },
  { icon: <ListIcon />, name: "Audit Log", path: "/audit-logs", permission: "audit.read" },
  { icon: <GroupIcon />, name: "Admins", path: "/admins", permission: "admins.read" },
  { icon: <AlertIcon />, name: "Security Events", path: "/security-events", permission: "security.read" },
  // Security (MFA) is always visible: unenrolled admins are restricted to it.
  { icon: <LockIcon />, name: "Security", path: "/security" },
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

  const isActive = (path: string) =>
    path === "/"
      ? pathname === "/"
      : pathname === path || pathname.startsWith(`${path}/`);

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
                {visibleItems.map((nav) => (
                  <li key={nav.name} className="relative">
                    {isActive(nav.path) && (
                      <span className="absolute left-[-16px] top-1/2 h-5 w-1 -translate-y-1/2 rounded-r-full bg-brand-500" />
                    )}
                    <Link
                      href={nav.path}
                      title={expanded ? undefined : nav.name}
                      className={`menu-item group ${
                        isActive(nav.path)
                          ? "menu-item-active"
                          : "menu-item-inactive"
                      }`}
                    >
                      <span
                        className={`${
                          isActive(nav.path)
                            ? "menu-item-icon-active"
                            : "menu-item-icon-inactive"
                        }`}
                      >
                        {nav.icon}
                      </span>
                      {expanded && (
                        <span className={`menu-item-text`}>{nav.name}</span>
                      )}
                    </Link>
                  </li>
                ))}
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
