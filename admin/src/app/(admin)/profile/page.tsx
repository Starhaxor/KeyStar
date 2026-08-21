"use client";
import ConsoleSection, {
  EmptyNote,
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import StatusBadge from "@/components/console/StatusBadge";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { api, formatDateTime } from "@/lib/api";
import type { AuditEntry } from "@/lib/types";
import { LockIcon } from "@/icons";
import Link from "next/link";
import React, { useEffect, useState } from "react";

function initialsFor(email: string): string {
  const local = email.split("@")[0] ?? "";
  return local.slice(0, 2).toUpperCase() || "A";
}

export default function ProfilePage() {
  const { identity, loading: identityLoading } = useAdminIdentity();
  const [activity, setActivity] = useState<AuditEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    void api.myActivity().then(
      (response) => {
        if (active) setActivity(response.items);
      },
      (err: unknown) => {
        if (active) {
          setError(err instanceof Error ? err.message : "Failed to load activity");
        }
      }
    );
    return () => {
      active = false;
    };
  }, []);

  if (identityLoading || !identity) {
    return (
      <div>
        <PageTitle title="Profile" description="Your admin account details." />
        <div className="rounded-2xl border border-gray-200 bg-white shadow-theme-xs dark:border-gray-800 dark:bg-white/[0.03]">
          <LoadingNote />
        </div>
      </div>
    );
  }

  const myActivity = (activity ?? []).filter(
    (entry) => entry.actor_email === identity.email
  );

  return (
    <div className="space-y-6">
      <PageTitle
        title="Profile"
        description="Your admin account details and security status."
      />

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        {/* Account card */}
        <ConsoleSection title="Account" description="Who you are signed in as.">
          <div className="flex items-start gap-4 p-5">
            <span className="flex h-14 w-14 shrink-0 items-center justify-center rounded-full bg-brand-500 text-lg font-semibold text-white">
              {initialsFor(identity.email)}
            </span>
            <div className="min-w-0">
              <p className="truncate text-base font-semibold text-gray-800 dark:text-white/90">
                {identity.email}
              </p>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <StatusBadge status={identity.status} />
                <BadgePill>{identity.role}</BadgePill>
                <BadgePill
                  tone={identity.mfa_enrolled ? "success" : "warning"}
                >
                  {identity.mfa_enrolled ? "MFA enabled" : "MFA not set up"}
                </BadgePill>
              </div>
              <dl className="mt-4 space-y-1.5 text-sm">
                <div className="flex gap-2">
                  <dt className="w-28 shrink-0 text-gray-400">Account ID</dt>
                  <dd className="truncate font-mono text-xs text-gray-700 dark:text-gray-300">
                    {identity.id}
                  </dd>
                </div>
                <div className="flex gap-2">
                  <dt className="w-28 shrink-0 text-gray-400">Permissions</dt>
                  <dd className="text-gray-700 dark:text-gray-300">
                    {identity.permissions.length} granted
                  </dd>
                </div>
              </dl>
            </div>
          </div>
        </ConsoleSection>

        {/* Security card */}
        <ConsoleSection
          title="Security"
          description="Two-factor authentication and access controls."
          actions={
            <Link
              href="/security"
              className="inline-flex items-center gap-2 rounded-lg bg-brand-500 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-600"
            >
              <LockIcon />
              Manage
            </Link>
          }
        >
          <div className="space-y-3 p-5">
            <div
              className={`flex items-center justify-between rounded-xl border px-4 py-3 ${
                identity.mfa_enrolled
                  ? "border-success-500/30 bg-success-50 dark:bg-success-500/[0.06]"
                  : "border-warning-500/30 bg-warning-50 dark:bg-warning-500/[0.06]"
              }`}
            >
              <div>
                <p className="text-sm font-medium text-gray-800 dark:text-white/90">
                  Two-factor authentication
                </p>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  {identity.mfa_enrolled
                    ? "Your account is protected with an authenticator app."
                    : "Set up MFA to protect your account."}
                </p>
              </div>
              <StatusBadge
                status={identity.mfa_enrolled ? "active" : "pending"}
              />
            </div>
            <p className="text-xs text-gray-400 dark:text-gray-500">
              Sign-in attempts and security events are recorded in the audit
              log. Review them from{" "}
              <Link
                href="/security-events"
                className="font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400"
              >
                Security Events
              </Link>
              .
            </p>
          </div>
        </ConsoleSection>
      </div>

      {/* Recent activity */}
      <ConsoleSection
        title="Recent Activity"
        description="Your latest actions across the console."
      >
        {error ? (
          <ErrorNote message={error} />
        ) : !activity ? (
          <LoadingNote />
        ) : myActivity.length === 0 ? (
          <EmptyNote message="No activity recorded for this account yet." />
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 dark:border-gray-800">
              <tr className="text-xs uppercase text-gray-400">
                <th className="px-5 py-3 font-medium">Action</th>
                <th className="px-5 py-3 font-medium">Resource</th>
                <th className="px-5 py-3 font-medium">When</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {myActivity.slice(0, 8).map((entry) => (
                <tr key={entry.id}>
                  <td className="px-5 py-3 font-medium text-gray-700 dark:text-gray-300">
                    {entry.action}
                  </td>
                  <td className="px-5 py-3 text-gray-500 dark:text-gray-400">
                    {entry.resource_type
                      ? `${entry.resource_type}${
                          entry.resource_id ? ` · ${entry.resource_id.slice(0, 8)}…` : ""
                        }`
                      : "—"}
                  </td>
                  <td className="px-5 py-3 text-gray-500 dark:text-gray-400">
                    {formatDateTime(entry.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </ConsoleSection>
    </div>
  );
}

function BadgePill({
  children,
  tone = "info",
}: {
  children: React.ReactNode;
  tone?: "info" | "success" | "warning";
}) {
  const tones = {
    info: "bg-brand-50 text-brand-600 dark:bg-brand-500/10 dark:text-brand-400",
    success:
      "bg-success-50 text-success-600 dark:bg-success-500/10 dark:text-success-400",
    warning:
      "bg-warning-50 text-warning-600 dark:bg-warning-500/10 dark:text-warning-400",
  };
  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium capitalize ${tones[tone]}`}
    >
      {children}
    </span>
  );
}
