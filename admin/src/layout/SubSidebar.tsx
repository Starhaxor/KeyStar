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

export default function SubSidebar() {
  const pathname = usePathname();
  const section = sections.find((s) => matches(pathname, s));

  if (!section) return null;

  return (
    <aside className="hidden w-52 shrink-0 border-r border-gray-200 px-3 py-6 lg:block dark:border-gray-800">
      <div className="sticky top-20">
        <h2 className="mb-3 px-3 text-xs font-semibold uppercase tracking-wide text-gray-400">
          {section.title}
        </h2>
        <nav className="flex flex-col gap-1">
          {section.items.map((item) => {
            // An item is active only when it is the canonical page itself.
            // Query-parameter shortcuts (e.g. ?create=1) are actions, never
            // highlighted as the current location.
            const hasQuery = item.href.includes("?");
            const base = item.href.split("?")[0];
            const active = !hasQuery && pathname === base;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
                  active
                    ? "bg-brand-50 text-brand-600 dark:bg-brand-500/10 dark:text-brand-400"
                    : "text-gray-600 hover:bg-gray-100 hover:text-gray-800 dark:text-gray-400 dark:hover:bg-white/5 dark:hover:text-gray-200"
                }`}
              >
                {item.label}
              </Link>
            );
          })}
        </nav>
        <p className="mt-8 px-3 text-xs leading-relaxed text-gray-400 dark:text-gray-500">
          KeyStar licensing console — StarLoader backend.
        </p>
      </div>
    </aside>
  );
}
