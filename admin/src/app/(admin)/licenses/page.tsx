"use client";
import ConsoleSection, {
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import EmptyState from "@/components/console/EmptyState";
import { TableSkeleton } from "@/components/common/Skeleton";
import ExportCsvButton from "@/components/common/ExportCsvButton";
import ConfirmModal from "@/components/console/ConfirmModal";
import LicenseCreateModal from "@/components/console/LicenseCreateModal";
import RowActions, { type RowAction } from "@/components/console/RowActions";
import StatusBadge from "@/components/console/StatusBadge";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import Pagination from "@/components/tables/Pagination";
import { api, formatDateTime } from "@/lib/api";
import type { ConsoleLicense, PageResult } from "@/lib/types";
import { DocsIcon } from "@/icons";
import React, { useCallback, useEffect, useMemo, useState } from "react";

const fieldClasses =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

export default function LicensesPage() {
  const [result, setResult] = useState<PageResult<ConsoleLicense> | null>(null);
  const [page, setPage] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const [filter, setFilter] = useState("");

  const [createOpen, setCreateOpen] = useState(false);

  const [extendTarget, setExtendTarget] = useState<ConsoleLicense | null>(null);
  const [extendDays, setExtendDays] = useState(30);
  const [extendMaxDevices, setExtendMaxDevices] = useState(0);
  const [extendLevel, setExtendLevel] = useState(1);
  const [extendNotes, setExtendNotes] = useState("");
  const [extendBusy, setExtendBusy] = useState(false);
  const [extendError, setExtendError] = useState<string | null>(null);

  const [revokeTarget, setRevokeTarget] = useState<ConsoleLicense | null>(null);
  const [revokeBusy, setRevokeBusy] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const response = await api.licenses(page);
      setResult(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load licenses");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get("create") === "1") {
      window.history.replaceState({}, "", "/licenses");
      setCreateOpen(true);
    }
  }, []);

  const allItems = result?.items ?? [];
  const items = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return allItems;
    return allItems.filter(
      (license) =>
        license.user_email.toLowerCase().includes(q) ||
        license.product.toLowerCase().includes(q) ||
        license.status.toLowerCase().includes(q) ||
        license.id.toLowerCase().includes(q)
    );
  }, [allItems, filter]);

  async function handleExtend(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!extendTarget) return;
    setExtendBusy(true);
    setExtendError(null);
    try {
      await api.updateLicense(
        extendTarget.id,
        extendDays,
        extendMaxDevices,
        extendLevel,
        extendNotes
      );
      setExtendTarget(null);
      await load();
    } catch (err) {
      setExtendError(err instanceof Error ? err.message : "Update failed");
    } finally {
      setExtendBusy(false);
    }
  }

  async function handleRevoke() {
    if (!revokeTarget) return;
    setRevokeBusy(true);
    setRevokeError(null);
    try {
      await api.revokeLicense(revokeTarget.id);
      setRevokeTarget(null);
      await load();
    } catch (err) {
      setRevokeError(err instanceof Error ? err.message : "Revoke failed");
    } finally {
      setRevokeBusy(false);
    }
  }

  const totalPages = result
    ? Math.max(1, Math.ceil(result.total / result.page_size))
    : 1;

  const activeCount = allItems.filter((l) => l.status === "active").length;
  const revokedCount = allItems.filter((l) => l.status === "revoked").length;
  const expiredCount = allItems.filter((l) => l.status === "expired").length;

  return (
    <div>
      <PageTitle
        title="Licenses"
        description="Issue, extend and revoke product licenses."
        actions={
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            Create License
          </Button>
        }
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
        title="License Directory"
        description={
          result ? `${result.total} license(s) total` : "Loading licenses"
        }
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
              filename="licenses.csv"
              headers={["user_email", "product", "status", "max_devices", "expires_at", "created_at"]}
              rows={items.map((l) => [
                l.user_email,
                l.product,
                l.status,
                l.max_devices,
                formatDateTime(l.expires_at),
                formatDateTime(l.created_at),
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
            icon={<DocsIcon />}
            title="No licenses found"
            message="Issue a license to an end user to get started."
          />
        ) : items.length === 0 ? (
          <EmptyState
            icon={<DocsIcon />}
            title="No matching licenses"
            message={`Nothing matches “${filter}” on this page.`}
          />
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 dark:border-gray-800">
                <tr className="text-xs uppercase text-gray-400">
                  <th className="px-5 py-3 font-medium">User</th>
                  <th className="px-5 py-3 font-medium">Product</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                  <th className="px-5 py-3 font-medium">Level</th>
                  <th className="px-5 py-3 font-medium">Max Devices</th>
                  <th className="px-5 py-3 font-medium">Expires</th>
                  <th className="px-5 py-3 font-medium">Created</th>
                  <th className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {items.map((license) => {
                  const actions: RowAction[] = [
                    {
                      label: "Extend",
                      tone: "warning",
                      disabled: license.status === "revoked",
                      onClick: () => {
                        setExtendError(null);
                        setExtendDays(30);
                        setExtendMaxDevices(license.max_devices);
                        setExtendLevel(license.level ?? 1);
                        setExtendNotes(license.notes ?? "");
                        setExtendTarget(license);
                      },
                    },
                    {
                      label: "Revoke",
                      tone: "danger",
                      disabled: license.status === "revoked",
                      onClick: () => {
                        setRevokeError(null);
                        setRevokeTarget(license);
                      },
                    },
                  ];
                  return (
                    <tr
                      key={license.id}
                      className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"
                    >
                      <td className="px-5 py-3.5 text-gray-700 dark:text-gray-300">
                        {license.user_email}
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {license.product}
                      </td>
                      <td className="px-5 py-3.5">
                        <StatusBadge status={license.status} />
                      </td>
                      <td className="px-5 py-3.5">
                        <span
                          title={"Level " + (license.level ?? 1)}
                          className="inline-flex items-center justify-center rounded-md bg-brand-50 px-2 py-0.5 text-xs font-semibold text-brand-600 dark:bg-brand-500/10 dark:text-brand-400"
                        >
                          Lv {license.level ?? 1}
                        </span>
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {license.max_devices}
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {formatDateTime(license.expires_at)}
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                        {formatDateTime(license.created_at)}
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

      <LicenseCreateModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={load}
      />

      {/* Extend modal */}
      <Modal
        isOpen={extendTarget !== null}
        onClose={() => !extendBusy && setExtendTarget(null)}
        className="max-w-md p-6"
      >
        {extendTarget && (
          <form onSubmit={handleExtend}>
            <h3 className="mb-4 text-lg font-semibold text-gray-800 dark:text-white/90">
              Extend License
            </h3>
            <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
              {extendTarget.user_email} · currently expires{" "}
              {formatDateTime(extendTarget.expires_at)}
            </p>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-400">
                  Extend by (days)
                </label>
                <input
                  className={fieldClasses}
                  type="number"
                  min={0}
                  max={3650}
                  value={extendDays}
                  onChange={(e) => setExtendDays(Number(e.target.value))}
                />
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-400">
                  Max Devices
                </label>
                <input
                  className={fieldClasses}
                  type="number"
                  min={1}
                  max={10000}
                  value={extendMaxDevices}
                  onChange={(e) => setExtendMaxDevices(Number(e.target.value))}
                />
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-400">
                  Level (1-100)
                </label>
                <input
                  className={fieldClasses}
                  type="number"
                  min={1}
                  max={100}
                  value={extendLevel}
                  onChange={(e) => setExtendLevel(Number(e.target.value))}
                />
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-400">
                  Notes
                </label>
                <input
                  className={fieldClasses}
                  type="text"
                  maxLength={2000}
                  value={extendNotes}
                  onChange={(e) => setExtendNotes(e.target.value)}
                  placeholder="Internal note"
                />
              </div>
            </div>
            {extendError && (
              <p className="mt-3 text-sm text-error-500" role="alert">
                {extendError}
              </p>
            )}
            <div className="mt-5 flex justify-end gap-3">
              <button
                type="button"
                disabled={extendBusy}
                onClick={() => setExtendTarget(null)}
                className="inline-flex items-center justify-center font-medium gap-2 rounded-lg transition px-4 py-3 text-sm bg-white text-gray-700 ring-1 ring-inset ring-gray-300 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-400 dark:ring-gray-700 disabled:opacity-50"
              >
                Cancel
              </button>
              <Button size="sm" disabled={extendBusy}>
                {extendBusy ? "Saving..." : "Save"}
              </Button>
            </div>
          </form>
        )}
      </Modal>

      <ConfirmModal
        isOpen={revokeTarget !== null}
        title="Revoke license"
        message={
          revokeTarget
            ? `Revoke the license of ${revokeTarget.user_email}? Active sessions for it will be expired. This cannot be undone.`
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
