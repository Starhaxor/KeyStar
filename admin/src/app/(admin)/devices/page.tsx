"use client";
import ConsoleSection, {
  EmptyNote,
  ErrorNote,
  LoadingNote,
  PageTitle,
} from "@/components/console/ConsoleSection";
import ConfirmModal from "@/components/console/ConfirmModal";
import StatusBadge from "@/components/console/StatusBadge";
import Pagination from "@/components/tables/Pagination";
import { api, formatDateTime } from "@/lib/api";
import type { ConsoleDevice, PageResult } from "@/lib/types";
import React, { useCallback, useEffect, useState } from "react";

export default function DevicesPage() {
  const [result, setResult] = useState<PageResult<ConsoleDevice> | null>(null);
  const [page, setPage] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const [revokeTarget, setRevokeTarget] = useState<ConsoleDevice | null>(null);
  const [revokeBusy, setRevokeBusy] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setError(null);
      const response = await api.devices(page);
      setResult(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load devices");
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
      await api.revokeDevice(revokeTarget.id);
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
        title="Devices"
        description="Hardware registrations bound to user licenses."
      />
      <ConsoleSection
        title="Device Directory"
        description={result ? `${result.total} device(s) total` : "Loading devices"}
      >
        {loading && !error ? (
          <LoadingNote />
        ) : error ? (
          <ErrorNote message={error} />
        ) : !result || result.items.length === 0 ? (
          <EmptyNote message="No devices found." />
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 dark:border-gray-800">
                <tr className="text-xs uppercase text-gray-400">
                  <th className="px-5 py-3 font-medium">Device</th>
                  <th className="px-5 py-3 font-medium">User</th>
                  <th className="px-5 py-3 font-medium">TPM</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                  <th className="px-5 py-3 font-medium">Last Seen</th>
                  <th className="px-5 py-3 font-medium">Created</th>
                  <th className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {result.items.map((device) => (
                  <tr key={device.id}>
                    <td className="px-5 py-3.5 font-mono text-xs text-gray-700 dark:text-gray-300">
                      {device.id.slice(0, 13)}…
                    </td>
                    <td className="px-5 py-3.5 text-gray-700 dark:text-gray-300">
                      {device.user_email}
                    </td>
                    <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                      {device.tpm_registered ? "Yes" : "No"}
                    </td>
                    <td className="px-5 py-3.5">
                      <StatusBadge status={device.status} />
                    </td>
                    <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                      {formatDateTime(device.last_seen_at)}
                    </td>
                    <td className="px-5 py-3.5 text-gray-500 dark:text-gray-400">
                      {formatDateTime(device.created_at)}
                    </td>
                    <td className="px-5 py-3.5">
                      <div className="flex justify-end">
                        <button
                          onClick={() => {
                            setRevokeError(null);
                            setRevokeTarget(device);
                          }}
                          disabled={device.status === "revoked"}
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

      <ConfirmModal
        isOpen={revokeTarget !== null}
        title="Revoke device"
        message={
          revokeTarget
            ? `Revoke this device for ${revokeTarget.user_email}? The device will no longer be able to authenticate.`
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
