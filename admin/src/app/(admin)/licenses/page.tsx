"use client";
import ConsoleSection, {
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import EmptyState from "@/components/console/EmptyState";
import { TableSkeleton } from "@/components/common/Skeleton";
import ConfirmModal from "@/components/console/ConfirmModal";
import StatusBadge from "@/components/console/StatusBadge";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import Pagination from "@/components/tables/Pagination";
import { api, formatDateTime } from "@/lib/api";
import type { ConsoleLicense, CreatedLicense, PageResult } from "@/lib/types";
import { DocsIcon } from "@/icons";
import React, { useCallback, useEffect, useState } from "react";

const fieldClasses =
  "h-11 w-full rounded-lg border border-gray-300 bg-transparent px-4 text-sm text-gray-800 shadow-theme-xs placeholder:text-gray-400 focus:border-brand-300 focus:outline-hidden focus:ring-3 focus:ring-brand-500/10 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90";

export default function LicensesPage() {
  const [result, setResult] = useState<PageResult<ConsoleLicense> | null>(null);
  const [page, setPage] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const [createOpen, setCreateOpen] = useState(false);
  const [userEmail, setUserEmail] = useState("");
  const [days, setDays] = useState(30);
  const [maxDevices, setMaxDevices] = useState(1);
  const [createBusy, setCreateBusy] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [created, setCreated] = useState<CreatedLicense | null>(null);
  const [keyCopied, setKeyCopied] = useState(false);

  const [extendTarget, setExtendTarget] = useState<ConsoleLicense | null>(null);
  const [extendDays, setExtendDays] = useState(30);
  const [extendMaxDevices, setExtendMaxDevices] = useState(0);
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

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreateBusy(true);
    setCreateError(null);
    try {
      const response = await api.createLicense(userEmail.trim(), days, maxDevices);
      setCreated(response);
      setCreateOpen(false);
      setKeyCopied(false);
      await load();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : "License creation failed");
    } finally {
      setCreateBusy(false);
    }
  }

  async function copyKey() {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.key);
      setKeyCopied(true);
    } catch {
      setKeyCopied(false);
    }
  }

  async function handleExtend(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!extendTarget) return;
    setExtendBusy(true);
    setExtendError(null);
    try {
      await api.updateLicense(extendTarget.id, extendDays, extendMaxDevices);
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

  return (
    <div>
      <PageTitle
        title="Licenses"
        description="Issue, extend and revoke product licenses."
        actions={
          <Button
            size="sm"
            onClick={() => {
              setCreateError(null);
              setCreateOpen(true);
            }}
          >
            Create License
          </Button>
        }
      />
      <ConsoleSection
        title="License Directory"
        description={result ? `${result.total} license(s) total` : "Loading licenses"}
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
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 dark:border-gray-800">
                <tr className="text-xs uppercase text-gray-400">
                  <th className="px-5 py-3 font-medium">User</th>
                  <th className="px-5 py-3 font-medium">Product</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                  <th className="px-5 py-3 font-medium">Max Devices</th>
                  <th className="px-5 py-3 font-medium">Expires</th>
                  <th className="px-5 py-3 font-medium">Created</th>
                  <th className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {result.items.map((license) => (
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
                      <div className="flex justify-end gap-2">
                        <button
                          onClick={() => {
                            setExtendError(null);
                            setExtendDays(30);
                            setExtendMaxDevices(license.max_devices);
                            setExtendTarget(license);
                          }}
                          disabled={license.status === "revoked"}
                          className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-40 dark:border-gray-700 dark:text-gray-400 dark:hover:bg-white/[0.03]"
                        >
                          Extend
                        </button>
                        <button
                          onClick={() => {
                            setRevokeError(null);
                            setRevokeTarget(license);
                          }}
                          disabled={license.status === "revoked"}
                          className="rounded-lg border border-error-500/40 px-3 py-1.5 text-xs font-medium text-error-500 hover:bg-error-50 disabled:opacity-40 dark:hover:bg-error-500/10"
                        >
                          Revoke
                        </button>
                      </div>
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

      {/* Create license modal */}
      <Modal
        isOpen={createOpen}
        onClose={() => !createBusy && setCreateOpen(false)}
        className="max-w-md p-6"
      >
        <h3 className="mb-4 text-lg font-semibold text-gray-800 dark:text-white/90">
          Create License
        </h3>
        <form onSubmit={handleCreate} className="space-y-4">
          <div>
            <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-400">
              User Email
            </label>
            <input
              className={fieldClasses}
              type="email"
              required
              value={userEmail}
              onChange={(e) => setUserEmail(e.target.value)}
              placeholder="user@example.com"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-400">
                Duration (days)
              </label>
              <input
                className={fieldClasses}
                type="number"
                min={1}
                max={3650}
                required
                value={days}
                onChange={(e) => setDays(Number(e.target.value))}
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
                required
                value={maxDevices}
                onChange={(e) => setMaxDevices(Number(e.target.value))}
              />
            </div>
          </div>
          {createError && (
            <p className="text-sm text-error-500" role="alert">
              {createError}
            </p>
          )}
          <div className="flex justify-end gap-3 pt-2">
            <button
              type="button"
              disabled={createBusy}
              onClick={() => setCreateOpen(false)}
              className="inline-flex items-center justify-center font-medium gap-2 rounded-lg transition px-4 py-3 text-sm bg-white text-gray-700 ring-1 ring-inset ring-gray-300 hover:bg-gray-50 dark:bg-gray-800 dark:text-gray-400 dark:ring-gray-700 disabled:opacity-50"
            >
              Cancel
            </button>
            <Button size="sm" disabled={createBusy}>
              {createBusy ? "Creating..." : "Create"}
            </Button>
          </div>
        </form>
      </Modal>

      {/* One-time key modal */}
      <Modal
        isOpen={created !== null}
        onClose={() => setCreated(null)}
        className="max-w-lg p-6"
      >
        {created && (
          <div>
            <h3 className="mb-2 text-lg font-semibold text-gray-800 dark:text-white/90">
              License Created
            </h3>
            <p className="mb-4 text-sm text-gray-500 dark:text-gray-400">
              This is the only time the plaintext key is shown. Copy it now —
              only its HMAC is stored.
            </p>
            <div className="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-gray-800 dark:bg-white/[0.03]">
              <code className="flex-1 break-all font-mono text-sm text-gray-800 dark:text-white/90">
                {created.key}
              </code>
              <button
                onClick={copyKey}
                className="shrink-0 rounded-lg bg-brand-500 px-3 py-1.5 text-xs font-medium text-white hover:bg-brand-600"
              >
                {keyCopied ? "Copied" : "Copy"}
              </button>
            </div>
            <div className="mt-4 flex justify-end">
              <Button size="sm" onClick={() => setCreated(null)}>
                Done
              </Button>
            </div>
          </div>
        )}
      </Modal>

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
