"use client";
import ActionBadge from "@/components/console/ActionBadge";
import ConsoleSection, {
  ErrorNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import EmptyState from "@/components/console/EmptyState";
import { TableSkeleton } from "@/components/common/Skeleton";
import ExportCsvButton from "@/components/common/ExportCsvButton";
import TimeAgo from "@/components/common/TimeAgo";
import Pagination from "@/components/tables/Pagination";
import SortableHeader from "@/components/tables/SortableHeader";
import { useTableSort } from "@/hooks/useTableSort";
import { api, formatDateTime } from "@/lib/api";
import type { AuditEntry, PageResult } from "@/lib/types";
import { ListIcon } from "@/icons";
import React, { useCallback, useEffect, useState } from "react";

export default function AuditLogsPage() {
  const [result, setResult] = useState<PageResult<AuditEntry> | null>(null);
  const [page, setPage] = useState(1);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const response = await api.auditLogs(page, search);
      setResult(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load audit logs");
    } finally {
      setLoading(false);
    }
  }, [page, search]);

  useEffect(() => {
    load();
  }, [load]);

  // Live search: debounce keystrokes, then refetch from page 1. The backend
  // matches actor email, action and resource fields.
  useEffect(() => {
    const timer = setTimeout(() => {
      setPage(1);
      setSearch(searchInput.trim());
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  const totalPages = result
    ? Math.max(1, Math.ceil(result.total / result.page_size))
    : 1;

  type AuditSortKey = "time" | "action" | "admin";

  const { sorted: items, sort, toggleSort } = useTableSort<
    AuditEntry,
    AuditSortKey
  >(result?.items ?? [], {
    time: (entry) => entry.created_at,
    action: (entry) => entry.action,
    admin: (entry) => entry.actor_email,
  });

  return (
    <div>
      <PageTitle
        title="Audit Log"
        description="Append-only record of administrator activity."
      />
      <ConsoleSection
        title="Admin Activity"
        description={result ? `${result.total} event(s) total` : "Loading audit log"}
        actions={
          <div className="flex items-center gap-2">
            <input
              type="search"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="Search by action, admin, resource..."
              className="h-10 w-60 rounded-lg border border-gray-300 bg-transparent px-3.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
            />
            <ExportCsvButton
              filename="audit-logs.csv"
              headers={["action", "actor_email", "resource_type", "resource_id", "created_at"]}
              rows={items.map((entry) => [
                entry.action,
                entry.actor_email,
                entry.resource_type,
                entry.resource_id,
                formatDateTime(entry.created_at),
              ])}
            />
          </div>
        }
      >
        {loading && !error ? (
          <TableSkeleton rows={6} cols={5} />
        ) : error ? (
          <ErrorNote message={error} />
        ) : !result || items.length === 0 ? (
          <EmptyState
            icon={<ListIcon />}
            title={
              search.trim()
                ? "No matching audit events"
                : "No audit events yet"
            }
            message={
              search.trim()
                ? "Nothing matches the current filters."
                : "Administrator actions will appear here."
            }
          />
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 dark:border-gray-800">
                <tr className="text-xs uppercase text-gray-400">
                  <SortableHeader label="Time" sortKey="time" sort={sort} onToggle={toggleSort} />
                  <SortableHeader label="Action" sortKey="action" sort={sort} onToggle={toggleSort} />
                  <SortableHeader label="Admin" sortKey="admin" sort={sort} onToggle={toggleSort} />
                  <th className="px-5 py-3 font-medium">Resource</th>
                  <th className="px-5 py-3 font-medium">Details</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {items.map((entry) => (
                  <tr
                    key={entry.id}
                    className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                  >
                    <td className="whitespace-nowrap px-5 py-3.5 text-gray-500 dark:text-gray-400">
                      <TimeAgo value={entry.created_at} />
                    </td>
                    <td className="px-5 py-3.5">
                      <ActionBadge action={entry.action} />
                    </td>
                    <td className="px-5 py-3.5 text-gray-700 dark:text-gray-300">
                      {entry.actor_email}
                    </td>
                    <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                      {entry.resource_type}
                      {entry.resource_id ? (
                        <span className="ml-1.5 font-mono text-xs text-gray-400">
                          {entry.resource_id.slice(0, 13)}…
                        </span>
                      ) : null}
                    </td>
                    <td className="max-w-xs truncate px-5 py-3.5 font-mono text-xs text-gray-400 dark:text-gray-500">
                      {Object.keys(entry.metadata ?? {}).length > 0
                        ? JSON.stringify(entry.metadata)
                        : "—"}
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
