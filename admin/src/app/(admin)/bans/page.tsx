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
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { useToast } from "@/context/ToastContext";
import { api, formatDateTime } from "@/lib/api";
import { moderationStatus } from "@/lib/moderation";
import type { BanRecord, PageResult } from "@/lib/types";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import React, { Suspense, useCallback, useEffect, useState } from "react";

function BansContent() {
  const { hasPermission } = useAdminIdentity();
  const toast = useToast();
  const params = useSearchParams();
  const statusFromUrl = moderationStatus(params, "active");
  const [result, setResult] = useState<PageResult<BanRecord> | null>(null);
  const [page, setPage] = useState(1);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState(statusFromUrl);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const [unbanTarget, setUnbanTarget] = useState<BanRecord | null>(null);
  const [unbanBusy, setUnbanBusy] = useState(false);
  const [unbanError, setUnbanError] = useState<string | null>(null);

  const canWrite = hasPermission("users.write");

  useEffect(() => {
    setPage(1);
    setStatusFilter(statusFromUrl);
  }, [statusFromUrl]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const response = await api.bans(page, search, statusFilter);
      setResult(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load bans");
    } finally {
      setLoading(false);
    }
  }, [page, search, statusFilter]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    const timer = setTimeout(() => {
      setPage(1);
      setSearch(searchInput.trim());
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  async function handleUnban() {
    if (!unbanTarget) return;
    setUnbanBusy(true);
    setUnbanError(null);
    try {
      await api.unbanUser(unbanTarget.user_id);
      setUnbanTarget(null);
      await load();
      toast.success("User unbanned", unbanTarget.user_email);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unban failed";
      setUnbanError(message);
      toast.error("Unban failed", message);
    } finally {
      setUnbanBusy(false);
    }
  }

  const totalPages = result
    ? Math.max(1, Math.ceil(result.total / result.page_size))
    : 1;

  const items = result?.items ?? [];

  function banTypeLabel(ban: BanRecord): string {
    return ban.expires_at ? `Temporary · until ${formatDateTime(ban.expires_at)}` : "Permanent";
  }

  return (
    <div>
      <PageTitle
        title="Bans"
        description="Every ban issued against end users, including lifted and expired ones."
      />
      {result && (
        <div className="mb-4 flex flex-wrap gap-2">
          <span className="rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700 dark:bg-white/[0.05] dark:text-gray-300">
            {result.total} ban(s){statusFilter ? ` · ${statusFilter}` : ""}
          </span>
        </div>
      )}
      <ConsoleSection
        title="Ban History"
        description={result ? `${result.total} ban(s) total` : "Loading bans"}
        actions={
          <div className="flex items-center gap-2">
            <input
              type="search"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="Search by email..."
              className="h-10 w-56 rounded-lg border border-gray-300 bg-transparent px-3.5 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
            />
            <select
              value={statusFilter}
              onChange={(e) => {
                setStatusFilter(e.target.value);
                setPage(1);
              }}
              aria-label="Filter bans by state"
              className="h-10 rounded-lg border border-gray-300 bg-transparent px-2.5 text-sm text-gray-800 shadow-theme-xs focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"
            >
              <option value="active">Active</option>
              <option value="lifted">Lifted</option>
              <option value="expired">Expired</option>
              <option value="">All</option>
            </select>
            <ExportCsvButton
              filename="bans.csv"
              headers={["user_email", "reason", "type", "status", "banned_at", "expires_at", "lifted_at"]}
              rows={items.map((ban) => [
                ban.user_email,
                ban.reason,
                ban.expires_at ? "temporary" : "permanent",
                ban.status,
                formatDateTime(ban.banned_at),
                formatDateTime(ban.expires_at),
                formatDateTime(ban.lifted_at),
              ])}
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
            title={statusFilter === "active" ? "No active bans" : "No bans found"}
            message={
              statusFilter === "active"
                ? "No currently banned users. New bans appear here immediately."
                : "Ban records will appear here."
            }
          />
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 dark:border-gray-800">
                <tr className="text-xs uppercase text-gray-400">
                  <th className="px-5 py-3 font-medium">User</th>
                  <th className="px-5 py-3 font-medium">Reason</th>
                  <th className="px-5 py-3 font-medium">Type</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                  <th className="px-5 py-3 font-medium">Banned At</th>
                  <th className="px-5 py-3 font-medium">Lifted / Lifted By</th>
                  <th className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {items.map((ban) => {
                  const actions: RowAction[] = [
                    {
                      label: "View profile",
                      href: `/users/${ban.user_id}`,
                      tone: "info",
                    },
                  ];
                  if (canWrite && ban.status === "active") {
                    actions.push({
                      label: "Unban user",
                      tone: "success",
                      onClick: () => {
                        setUnbanError(null);
                        setUnbanTarget(ban);
                      },
                    });
                  }
                  return (
                    <tr
                      key={ban.id}
                      className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                    >
                      <td className="px-5 py-3.5">
                        <Link
                          href={`/users/${ban.user_id}`}
                          className="font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400"
                        >
                          {ban.user_email}
                        </Link>
                      </td>
                      <td className="max-w-[280px] px-5 py-3.5 text-gray-700 dark:text-gray-300">
                        <span className="line-clamp-2">{ban.reason || "—"}</span>
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {banTypeLabel(ban)}
                      </td>
                      <td className="px-5 py-3.5">
                        <StatusBadge status={ban.status} />
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {formatDateTime(ban.banned_at)}
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {ban.lifted_at
                          ? `${formatDateTime(ban.lifted_at)} (${ban.lift_reason})`
                          : "—"}
                      </td>
                      <td className="px-5 py-3.5 text-right">
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
        isOpen={unbanTarget !== null}
        title="Unban user"
        message={
          unbanTarget
            ? `Unban ${unbanTarget.user_email}? The ban will be recorded as lifted and the account returns to active.`
            : ""
        }
        confirmLabel="Unban"
        busy={unbanBusy}
        error={unbanError}
        onConfirm={handleUnban}
        onClose={() => setUnbanTarget(null)}
      />
    </div>
  );
}

export default function BansPage() {
  return <Suspense fallback={<div className="py-8"><TableSkeleton rows={6} cols={7} /></div>}><BansContent /></Suspense>;
}
