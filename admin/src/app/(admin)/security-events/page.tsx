"use client";
import ConsoleSection, {
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import EmptyState from "@/components/console/EmptyState";
import Pagination from "@/components/tables/Pagination";
import { api, formatDateTime } from "@/lib/api";
import type { PageResult, SecurityEvent } from "@/lib/types";
import { AlertIcon } from "@/icons";
import React, { useCallback, useEffect, useMemo, useState } from "react";

const severityStyles: Record<string, string> = {
  info: "bg-gray-100 text-gray-700 dark:bg-white/[0.05] dark:text-gray-300",
  warning:
    "bg-warning-50 text-warning-600 dark:bg-warning-500/[0.1] dark:text-warning-400",
  critical:
    "bg-error-50 text-error-600 dark:bg-error-500/[0.1] dark:text-error-400",
};

export default function SecurityEventsPage() {
  const [result, setResult] = useState<PageResult<SecurityEvent> | null>(null);
  const [page, setPage] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const response = await api.securityEvents(page);
      setResult(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load events");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    load();
  }, [load]);

  const allItems = useMemo(() => result?.items ?? [], [result]);
  const items = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return allItems;
    return allItems.filter(
      (event) =>
        event.kind.toLowerCase().includes(q) ||
        event.severity.toLowerCase().includes(q) ||
        (event.actor_email ?? "").toLowerCase().includes(q)
    );
  }, [allItems, filter]);

  const totalPages = result
    ? Math.max(1, Math.ceil(result.total / result.page_size))
    : 1;

  const criticalCount = allItems.filter((e) => e.severity === "critical").length;
  const warningCount = allItems.filter((e) => e.severity === "warning").length;
  const mfaCount = allItems.filter((e) => e.kind.toLowerCase().includes("mfa")).length;

  return (
    <div>
      <PageTitle
        title="Security Events"
        description="Authentication, MFA and authorization signals across the console."
      />
      {result && (
        <div className="mb-4 flex flex-wrap gap-2">
          <span className="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700 dark:bg-white/[0.05] dark:text-gray-300">
            {result.total} total
          </span>
          <span className="rounded-full bg-error-50 px-3 py-1 text-xs font-medium text-error-600 dark:bg-error-500/10 dark:text-error-400">
            {criticalCount} critical (this page)
          </span>
          <span className="rounded-full bg-warning-50 px-3 py-1 text-xs font-medium text-warning-600 dark:bg-warning-500/10 dark:text-warning-400">
            {warningCount} warnings (this page)
          </span>
          <span className="rounded-full bg-brand-50 px-3 py-1 text-xs font-medium text-brand-600 dark:bg-brand-500/10 dark:text-brand-400">
            {mfaCount} MFA events (this page)
          </span>
        </div>
      )}
      <ConsoleSection
        title="Recent Events"
        description={result ? `${result.total} event(s) total` : "Loading events"}
        actions={
          <input
            type="search"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Type to filter..."
            className="h-10 w-56 rounded-lg border border-gray-300 bg-transparent px-3.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
          />
        }
      >
        {loading && !error ? (
          <LoadingNote />
        ) : error ? (
          <ErrorNote message={error} />
        ) : !result || result.items.length === 0 ? (
          <EmptyState
            icon={<AlertIcon />}
            title="No security events yet"
            message="Authentication, MFA and authorization signals will appear here."
          />
        ) : items.length === 0 ? (
          <EmptyState
            icon={<AlertIcon />}
            title="No matching events"
            message={`Nothing matches “${filter}” on this page.`}
          />
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 dark:border-gray-800">
                <tr className="text-xs uppercase text-gray-400">
                  <th className="px-5 py-3 font-medium">Time</th>
                  <th className="px-5 py-3 font-medium">Event</th>
                  <th className="px-5 py-3 font-medium">Severity</th>
                  <th className="px-5 py-3 font-medium">Account</th>
                  <th className="px-5 py-3 font-medium">User Agent</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {items.map((event) => (
                  <tr
                    key={event.id}
                    className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                  >
                    <td className="whitespace-nowrap px-5 py-3.5 text-gray-500 dark:text-gray-400">
                      {formatDateTime(event.created_at)}
                    </td>
                    <td className="px-5 py-3.5 font-medium text-gray-700 dark:text-gray-300">
                      {event.kind}
                    </td>
                    <td className="px-5 py-3.5">
                      <span
                        className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${
                          severityStyles[event.severity] ?? severityStyles.info
                        }`}
                      >
                        {event.severity}
                      </span>
                    </td>
                    <td className="px-5 py-3.5 text-gray-700 dark:text-gray-300">
                      {event.actor_email || "—"}
                    </td>
                    <td className="max-w-xs truncate px-5 py-3.5 text-xs text-gray-400 dark:text-gray-500">
                      {event.user_agent || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div className="flex justify-end border-t border-gray-200 px-5 py-4 dark:border-gray-800">
              <Pagination
                currentPage={result.page}
                totalPages={totalPages}
                onPageChange={(next) =>
                  setPage(Math.min(Math.max(next, 1), totalPages))
                }
              />
            </div>
          </>
        )}
      </ConsoleSection>
    </div>
  );
}
