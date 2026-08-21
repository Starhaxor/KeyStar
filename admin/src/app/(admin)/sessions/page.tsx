"use client";
import ConsoleSection, {
  ErrorNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import EmptyState from "@/components/console/EmptyState";
import ConfirmModal from "@/components/console/ConfirmModal";
import RowActions, { type RowAction } from "@/components/console/RowActions";
import StatusBadge from "@/components/console/StatusBadge";
import { TableSkeleton } from "@/components/common/Skeleton";
import ExportCsvButton from "@/components/common/ExportCsvButton";
import Pagination from "@/components/tables/Pagination";
import { useToast } from "@/context/ToastContext";
import { api, fetchAllPages, formatDateTime } from "@/lib/api";
import type { ConsoleSession, PageResult } from "@/lib/types";
import { TimeIcon } from "@/icons";
import React, { useCallback, useEffect, useMemo, useState } from "react";

export default function SessionsPage() {
  const toast = useToast();
  const [result, setResult] = useState<PageResult<ConsoleSession> | null>(null);
  const [page, setPage] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState("");

  const [revokeTarget, setRevokeTarget] = useState<ConsoleSession | null>(null);
  const [revokeBusy, setRevokeBusy] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const response = await api.sessions(page);
      setResult(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load sessions");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    load();
  }, [load]);

  async function handleRevoke() {
    if (!revokeTarget) return;
    setRevokeBusy(true);
    setRevokeError(null);
    try {
      await api.revokeSession(revokeTarget.id);
      setRevokeTarget(null);
      await load();
      toast.success("Session revoked", revokeTarget.user_email);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Revoke failed";
      setRevokeError(message);
      toast.error("Revoke failed", message);
    } finally {
      setRevokeBusy(false);
    }
  }

  const allItems = useMemo(() => result?.items ?? [], [result]);
  const items = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return allItems;
    return allItems.filter(
      (session) =>
        session.user_email.toLowerCase().includes(q) ||
        session.id.toLowerCase().includes(q) ||
        session.license_id.toLowerCase().includes(q) ||
        session.status.toLowerCase().includes(q)
    );
  }, [allItems, filter]);

  const totalPages = result
    ? Math.max(1, Math.ceil(result.total / result.page_size))
    : 1;

  const activeCount = allItems.filter((s) => s.status === "active").length;
  const expiredCount = allItems.filter((s) => s.status === "expired").length;
  const revokedCount = allItems.filter((s) => s.status === "revoked").length;

  return (
    <div>
      <PageTitle
        title="Sessions"
        description="Active and recent user authentication sessions."
      />
      {result && (
        <div className="mb-4 flex flex-wrap gap-2">
          <span className="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700 dark:bg-white/[0.05] dark:text-gray-300">
            {result.total} total
          </span>
          <span className="rounded-full bg-success-50 px-3 py-1 text-xs font-medium text-success-600 dark:bg-success-500/10 dark:text-success-400">
            {activeCount} active (this page)
          </span>
          <span className="rounded-full bg-warning-50 px-3 py-1 text-xs font-medium text-warning-600 dark:bg-warning-500/10 dark:text-warning-400">
            {expiredCount} expired (this page)
          </span>
          <span className="rounded-full bg-error-50 px-3 py-1 text-xs font-medium text-error-600 dark:bg-error-500/10 dark:text-error-400">
            {revokedCount} revoked (this page)
          </span>
        </div>
      )}
      <ConsoleSection
        title="Auth Sessions"
        description={result ? `${result.total} session(s) total` : "Loading sessions"}
        actions={
          <div className="flex items-center gap-2">
            <input
              type="search"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Type to filter..."
              className="h-10 w-56 rounded-lg border border-gray-300 bg-transparent px-3.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
            />
            <ExportCsvButton
              filename="sessions.csv"
              headers={["id", "user_email", "status", "expires_at", "created_at"]}
              rows={items.map((s) => [
                s.id,
                s.user_email,
                s.status,
                formatDateTime(s.expires_at),
                formatDateTime(s.created_at),
              ])}
              loadAllRows={async () => (await fetchAllPages(api.sessions)).map((s) => [s.id, s.user_email, s.status, formatDateTime(s.expires_at), formatDateTime(s.created_at)])}
            />
          </div>
        }
      >
        {loading && !error ? (
          <TableSkeleton rows={6} cols={7} />
        ) : error ? (
          <ErrorNote message={error} />
        ) : !result || result.items.length === 0 ? (
          <EmptyState
            icon={<TimeIcon />}
            title="No sessions found"
            message="Verified auth sessions will appear here."
          />
        ) : items.length === 0 ? (
          <EmptyState
            icon={<TimeIcon />}
            title="No matching sessions"
            message={`Nothing matches “${filter}” on this page.`}
          />
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 dark:border-gray-800">
                <tr className="text-xs uppercase text-gray-400">
                  <th className="px-5 py-3 font-medium">Session</th>
                  <th className="px-5 py-3 font-medium">User</th>
                  <th className="px-5 py-3 font-medium">License</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                  <th className="px-5 py-3 font-medium">Expires</th>
                  <th className="px-5 py-3 font-medium">Created</th>
                  <th className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {items.map((session) => {
                  const actions: RowAction[] = [
                    {
                      label: "Revoke",
                      danger: true,
                      disabled: session.status === "revoked",
                      onClick: () => {
                        setRevokeError(null);
                        setRevokeTarget(session);
                      },
                    },
                  ];
                  return (
                  <tr
                    key={session.id}
                    className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                  >
                    <td className="px-5 py-3.5 font-mono text-xs text-gray-700 dark:text-gray-300">
                      {session.id.slice(0, 13)}…
                    </td>
                    <td className="px-5 py-3.5 text-gray-700 dark:text-gray-300">
                      {session.user_email}
                    </td>
                    <td className="px-5 py-3.5 font-mono text-xs text-gray-500 dark:text-gray-400">
                      {session.license_id.slice(0, 13)}…
                    </td>
                    <td className="px-5 py-3.5">
                      <StatusBadge status={session.status} />
                    </td>
                    <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                      {formatDateTime(session.expires_at)}
                    </td>
                    <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                      {formatDateTime(session.created_at)}
                    </td>
                    <td className="px-5 py-3.5">
                      <div className="flex justify-end">
                        <RowActions actions={actions} />
                      </div>
                    </td>
                  </tr>
                  );
                })}
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

      <ConfirmModal
        isOpen={revokeTarget !== null}
        title="Revoke session"
        message={
          revokeTarget
            ? `Revoke this session for ${revokeTarget.user_email}? They will be signed out immediately.`
            : ""
        }
        confirmLabel="Revoke"
        busy={revokeBusy}
        error={revokeError}
        onConfirm={handleRevoke}
        onClose={() => setRevokeTarget(null)}
      />
    </div>
  );
}
