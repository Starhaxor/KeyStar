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
import { Modal } from "@/components/ui/modal";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { api, formatDateTime } from "@/lib/api";
import type { ConsoleDevice, ConsoleDeviceDetail, PageResult } from "@/lib/types";
import React, { useCallback, useEffect, useState } from "react";

// HWID parts tracked by the backend. Only presence is shown — raw hardware
// identifiers are stored as HMACs server-side and never leave the backend.
const HWID_PARTS = [
  { key: "has_smbios_uuid", label: "SMBIOS UUID" },
  { key: "has_motherboard_serial", label: "Motherboard serial" },
  { key: "has_bios_serial", label: "BIOS serial" },
  { key: "has_system_disk_serial", label: "System disk serial" },
  { key: "has_machine_guid", label: "Machine GUID" },
] as const;

function hwidCount(device: ConsoleDevice): number {
  return HWID_PARTS.filter((part) => device[part.key]).length;
}

export default function DevicesPage() {
  const { hasPermission } = useAdminIdentity();
  const [result, setResult] = useState<PageResult<ConsoleDevice> | null>(null);
  const [page, setPage] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const [revokeTarget, setRevokeTarget] = useState<ConsoleDevice | null>(null);
  const [revokeBusy, setRevokeBusy] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  const [resetTarget, setResetTarget] = useState<ConsoleDevice | null>(null);
  const [resetBusy, setResetBusy] = useState(false);
  const [resetError, setResetError] = useState<string | null>(null);

  const [detailDevice, setDetailDevice] = useState<ConsoleDevice | null>(null);
  const [detail, setDetail] = useState<ConsoleDeviceDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);

  const canWrite = hasPermission("devices.write");

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

  async function handleReset() {
    if (!resetTarget) return;
    setResetBusy(true);
    setResetError(null);
    try {
      await api.resetDevice(resetTarget.id);
      setResetTarget(null);
      await load();
    } catch (err) {
      setResetError(err instanceof Error ? err.message : "Reset failed");
    } finally {
      setResetBusy(false);
    }
  }

  async function openDetail(device: ConsoleDevice) {
    setDetailDevice(device);
    setDetail(null);
    setDetailError(null);
    setDetailLoading(true);
    try {
      setDetail(await api.deviceDetail(device.id));
    } catch (err) {
      setDetailError(err instanceof Error ? err.message : "Failed to load device detail");
    } finally {
      setDetailLoading(false);
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
                  <th className="px-5 py-3 font-medium">HWID</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                  <th className="px-5 py-3 font-medium">Last Seen</th>
                  <th className="px-5 py-3 font-medium">Created</th>
                  <th className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {result.items.map((device) => {
                  const parts = hwidCount(device);
                  return (
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
                        <span
                          title="Hardware identifier parts present (of 5)"
                          className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${
                            parts === 5
                              ? "bg-success-50 text-success-600 dark:bg-success-500/[0.1]"
                              : parts === 0
                                ? "bg-warning-50 text-warning-600 dark:bg-warning-500/[0.1]"
                                : "bg-gray-100 text-gray-700 dark:bg-white/[0.05] dark:text-gray-300"
                          }`}
                        >
                          {parts}/5 parts
                        </span>
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
                        <div className="flex justify-end gap-2">
                          <button
                            onClick={() => openDetail(device)}
                            className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-white/[0.05]"
                          >
                            Details
                          </button>
                          {canWrite && (
                            <button
                              onClick={() => {
                                setResetError(null);
                                setResetTarget(device);
                              }}
                              className="rounded-lg border border-warning-500/40 px-3 py-1.5 text-xs font-medium text-warning-600 hover:bg-warning-50 dark:hover:bg-warning-500/10"
                            >
                              Reset
                            </button>
                          )}
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

      <ConfirmModal
        isOpen={resetTarget !== null}
        title="Reset device"
        message={
          resetTarget
            ? `Delete the hardware registration for ${resetTarget.user_email}? The device will be able to register again on its next launch, freeing a device slot.`
            : ""
        }
        confirmLabel="Reset"
        busy={resetBusy}
        error={resetError}
        onConfirm={handleReset}
        onClose={() => setResetTarget(null)}
      />

      <Modal
        isOpen={detailDevice !== null}
        onClose={() => setDetailDevice(null)}
        className="max-w-lg p-6"
      >
        {detailDevice && (
          <div className="space-y-5">
            <div>
              <h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">
                Device detail
              </h3>
              <p className="mt-1 break-all font-mono text-xs text-gray-500 dark:text-gray-400">
                {detailDevice.id}
              </p>
            </div>
            {detailLoading ? (
              <LoadingNote />
            ) : detailError ? (
              <ErrorNote message={detailError} />
            ) : detail ? (
              <div className="space-y-4 text-sm">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <span className="block text-xs uppercase text-gray-400">User</span>
                    <span className="mt-1 block text-gray-700 dark:text-gray-300">
                      {detail.device.user_email}
                    </span>
                  </div>
                  <div>
                    <span className="block text-xs uppercase text-gray-400">Product</span>
                    <span className="mt-1 block text-gray-700 dark:text-gray-300">
                      {detail.product || "—"}
                    </span>
                  </div>
                  <div>
                    <span className="block text-xs uppercase text-gray-400">Status</span>
                    <span className="mt-1 block">
                      <StatusBadge status={detail.device.status} />
                    </span>
                  </div>
                  <div>
                    <span className="block text-xs uppercase text-gray-400">Last seen</span>
                    <span className="mt-1 block text-gray-700 dark:text-gray-300">
                      {formatDateTime(detail.device.last_seen_at)}
                    </span>
                  </div>
                </div>
                <div>
                  <span className="block text-xs uppercase text-gray-400">
                    Hardware identifiers (presence only)
                  </span>
                  <ul className="mt-2 space-y-1.5">
                    {HWID_PARTS.map((part) => (
                      <li
                        key={part.key}
                        className="flex items-center justify-between rounded-lg border border-gray-200 px-3 py-2 dark:border-gray-800"
                      >
                        <span className="text-gray-700 dark:text-gray-300">
                          {part.label}
                        </span>
                        <span
                          className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${
                            detail.device[part.key]
                              ? "bg-success-50 text-success-600 dark:bg-success-500/[0.1]"
                              : "bg-gray-100 text-gray-500 dark:bg-white/[0.05] dark:text-gray-400"
                          }`}
                        >
                          {detail.device[part.key] ? "present" : "missing"}
                        </span>
                      </li>
                    ))}
                  </ul>
                  <p className="mt-2 text-xs text-gray-400">
                    Raw hardware values are stored as HMACs on the server and
                    are never exposed.
                  </p>
                </div>
                <div>
                  <span className="block text-xs uppercase text-gray-400">
                    TPM fingerprint (SHA-256 of TPM public key)
                  </span>
                  <p className="mt-2 break-all rounded-lg bg-gray-50 px-3 py-2 font-mono text-xs text-gray-700 dark:bg-white/[0.03] dark:text-gray-300">
                    {detail.tpm_fingerprint || "Not registered"}
                  </p>
                </div>
              </div>
            ) : null}
            <div className="flex justify-end">
              <button
                type="button"
                className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 dark:border-gray-700 dark:text-gray-300"
                onClick={() => setDetailDevice(null)}
              >
                Close
              </button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}
