"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import React from "react";

type SubItem = { label: string; href: string };

type Section = {
  match: string[];
  title: string;
  items: SubItem[];
};

const sections: Section[] = [
  {
    match: ["/"],
    title: "Overview",
    items: [{ label: "Live overview", href: "/" }],
  },
  {
    match: ["/users"],
    title: "Users",
    items: [
      { label: "Directory", href: "/users" },
      { label: "Add user", href: "/users?create=1" },
    ],
  },
  {
    match: ["/licenses"],
    title: "Licenses",
    items: [
      { label: "Directory", href: "/licenses" },
      { label: "Create license", href: "/licenses?create=1" },
    ],
  },
  {
    match: ["/devices"],
    title: "Devices",
    items: [{ label: "Directory", href: "/devices" }],
  },
  {
    match: ["/sessions"],
    title: "Sessions",
    items: [{ label: "Directory", href: "/sessions" }],
  },
  {
    match: ["/audit-logs"],
    title: "Audit",
    items: [
      { label: "Audit log", href: "/audit-logs" },
      { label: "Security events", href: "/security-events" },
    ],
  },
  {
    match: ["/security-events"],
    title: "Security",
    items: [
      { label: "Security events", href: "/security-events" },
      { label: "Audit log", href: "/audit-logs" },
      { label: "MFA & settings", href: "/security" },
    ],
  },
  {
    match: ["/admins"],
    title: "Admins",
    items: [
      { label: "Accounts", href: "/admins" },
      { label: "Add admin", href: "/admins?create=1" },
    ],
  },
  {
    match: ["/security"],
    title: "Security",
    items: [
      { label: "MFA & settings", href: "/security" },
      { label: "Security events", href: "/security-events" },
      { label: "Audit log", href: "/audit-logs" },
    ],
  },
  {
    match: ["/profile"],
    title: "Account",
    items: [
      { label: "Profile", href: "/profile" },
      { label: "Security", href: "/security" },
    ],
  },
];

function matches(pathname: string, section: Section): boolean {
  return section.match.some(
    (prefix) =>
      pathname === prefix ||
      (prefix !== "/" && pathname.startsWith(`${prefix}/`))
  );
}

// Horizontal sub-navigation bar rendered directly under the header.
// Query-parameter shortcuts (?create=1) are actions and are never
// highlighted as the current location.
export default function SubSidebar() {
  const pathname = usePathname();
  const section = sections.find((s) => matches(pathname, s));

  if (!section) return null;

  return (
    <div className="flex items-center gap-1 overflow-x-auto border-b border-gray-200 bg-white px-4 py-2 no-scrollbar dark:border-gray-800 dark:bg-gray-900">
      <span className="mr-2 hidden shrink-0 text-xs font-semibold uppercase tracking-wide text-gray-400 sm:block">
        {section.title}
      </span>
      {section.items.map((item) => {
        const hasQuery = item.href.includes("?");
        const base = item.href.split("?")[0];
        const active = !hasQuery && pathname === base;
        return (
          <Link
            key={item.href}
            href={item.href}
            className={`shrink-0 whitespace-nowrap rounded-lg px-3 py-1.5 text-sm font-medium transition-colors ${
              active
                ? "bg-brand-50 text-brand-600 dark:bg-brand-500/10 dark:text-brand-400"
                : "text-gray-600 hover:bg-gray-100 hover:text-gray-800 dark:text-gray-400 dark:hover:bg-white/5 dark:hover:text-gray-200"
            }`}
          >
            {item.label}
          </Link>
        );
      })}
    </div>
  );
}
