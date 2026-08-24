"use client";
import React, { useState } from "react";
import Link from "next/link";
import Image from "next/image";
import { usePathname } from "next/navigation";
import { useSidebar } from "../context/SidebarContext";
import { useAdminIdentity } from "../context/AdminIdentityContext";
import { useApplication } from "../context/ApplicationContext";
import {
  sidebarSections,
  isSidebarItemVisible,
  type SidebarIcon,
  type SidebarItem,
  type SidebarSubItem,
} from "./sidebarNavigation";
import { applicationSelectorOptions } from "../lib/applicationSelection";
import {
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

const iconFor: Record<SidebarIcon, React.ReactNode> = {
  grid: <GridIcon />,
  app: <BoxCubeIcon />,
  users: <UserCircleIcon />,
  licenses: <DocsIcon />,
  devices: <BoxCubeIcon />,
  products: <BoxCubeIcon />,
  sessions: <TimeIcon />,
  audit: <ListIcon />,
  admins: <GroupIcon />,
  variables: <ListIcon />,
  credentials: <LockIcon />,
  webhooks: <ListIcon />,
  security: <LockIcon />,
};

function initialsFor(email: string): string {
  const local = email.split("@")[0] ?? "";
  return local.slice(0, 2).toUpperCase() || "A";
}

const AppSidebar: React.FC = () => {
  const { isExpanded, isMobileOpen, isHovered, setIsHovered, toggleSidebar } =
    useSidebar();
  const pathname = usePathname();
  const { hasPermission, identity } = useAdminIdentity();
  const { applications, selectedApplicationID, selectApplication, loading: applicationsLoading } = useApplication();

  // Sections explicitly expanded by the user. The active section is always
  // open without a navigation-side effect.
  const [openSections, setOpenSections] = useState<Record<string, boolean>>({});

  const isActive = (path: string) =>
    path === "/" ? pathname === "/" : pathname === path || pathname.startsWith(`${path}/`);

  // A sub-item is the current location only when it is the canonical page
  // itself; query-string shortcuts are actions, never highlighted.
  const isChildActive = (child: SidebarSubItem) => {
    if (child.href.includes("?")) return false;
    return pathname === child.href || pathname.startsWith(`${child.href}/`);
  };

  const isSectionActive = (nav: SidebarItem) =>
    isActive(nav.path) || (nav.children ?? []).some(isChildActive);

  const toggleSection = (name: string) =>
    setOpenSections((prev) => ({ ...prev, [name]: !prev[name] }));

  const visibleSections = sidebarSections
    .map((section) => ({
      ...section,
      items: section.items.filter((item) => isSidebarItemVisible(item, hasPermission)),
    }))
    .filter((section) => section.items.length > 0);

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
        <nav className="mb-6 space-y-6" aria-label="Console navigation">
          {visibleSections.map((section) => (
            <section key={section.name}>
              <h2 className={`mb-2 flex items-center text-[10px] font-semibold uppercase tracking-[0.16em] text-gray-400 ${!expanded ? "lg:justify-center" : "justify-start"}`}>
                {expanded ? section.name : <HorizontaLDots />}
              </h2>
              {expanded && section.name === "Application" && (
                <div className="mb-2 rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-gray-800 dark:bg-white/[0.03]">
                  <label className="mb-1.5 block px-1 text-[10px] font-semibold uppercase tracking-[0.12em] text-gray-400" htmlFor="sidebar-application-selector">Current application</label>
                  <select id="sidebar-application-selector" aria-label="Current application" value={selectedApplicationID ?? ""} disabled={applicationsLoading || applications.length === 0} onChange={(event) => selectApplication(event.target.value)} className="h-9 w-full cursor-pointer rounded-md border border-gray-200 bg-white px-2.5 text-xs font-medium text-gray-700 outline-none transition-colors focus:border-brand-500 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200 dark:[color-scheme:dark] disabled:cursor-not-allowed disabled:opacity-60">
                    <option value="" disabled>{applicationsLoading ? "Loading applications…" : "Select application"}</option>
                    {applicationSelectorOptions(applications).map((application) => <option key={application.value} value={application.value}>{application.label}</option>)}
                  </select>
                  <Link href="/applications" className="mt-2 block px-1 text-[11px] text-gray-500 transition-colors hover:text-brand-500 dark:hover:text-brand-400">Manage applications</Link>
                </div>
              )}
              <ul className="flex flex-col gap-1.5">
                {section.items.map((nav) => {
                  const active = isSectionActive(nav);
                  const sectionOpen = active || Boolean(openSections[nav.name]);
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
                              {iconFor[nav.icon]}
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
                              {iconFor[nav.icon]}
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
            </section>
          ))}
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
