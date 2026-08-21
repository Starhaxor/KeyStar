"use client";

import ConsoleSection, { EmptyNote, ErrorNote, LoadingNote, PageTitle } from "@/components/console/ConsoleSection";
import ConfirmModal from "@/components/console/ConfirmModal";
import StatusBadge from "@/components/console/StatusBadge";
import Pagination from "@/components/tables/Pagination";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { useToast } from "@/context/ToastContext";
import { api, formatDateTime } from "@/lib/api";
import { deviceBanExpiry, moderationStatus } from "@/lib/moderation";
import type { ConsoleDevice, DeviceBanRecord, PageResult } from "@/lib/types";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import React, { Suspense, useCallback, useEffect, useState } from "react";

const durations: Array<{ label: string; hours: number | null }> = [
  { label: "Permanent", hours: null }, { label: "24 hours", hours: 24 },
  { label: "7 days", hours: 168 }, { label: "30 days", hours: 720 },
];

function DeviceBansContent() {
  const params = useSearchParams();
  const router = useRouter();
  const toast = useToast();
  const { hasPermission } = useAdminIdentity();
  const status = moderationStatus(params, "active");
  const [result, setResult] = useState<PageResult<DeviceBanRecord> | null>(null);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [target, setTarget] = useState<DeviceBanRecord | null>(null);
  const [busy, setBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [devices, setDevices] = useState<ConsoleDevice[]>([]);
  const [devicesLoading, setDevicesLoading] = useState(false);
  const [selectedDeviceId, setSelectedDeviceId] = useState("");
  const [reason, setReason] = useState("");
  const [durationHours, setDurationHours] = useState<number | null>(null);
  const [createError, setCreateError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const canWrite = hasPermission("devices.write");

  const load = useCallback(async () => {
    setLoading(true);
    try { setError(null); setResult(await api.deviceBans(page, status)); }
    catch (err) { setError(err instanceof Error ? err.message : "Device bans could not be loaded"); }
    finally { setLoading(false); }
  }, [page, status]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => { setPage(1); }, [status]);

  const setStatus = (next: string) => router.push(next ? `/device-bans?status=${next}` : "/device-bans");

  async function openCreate() {
    setCreateOpen(true); setCreateError(null); setReason(""); setSelectedDeviceId(""); setDurationHours(null); setDevicesLoading(true);
    try { const response = await api.devices(1, 100); setDevices(response.items.filter((device) => device.status === "active")); }
    catch (err) { setCreateError(err instanceof Error ? err.message : "Devices could not be loaded"); }
    finally { setDevicesLoading(false); }
  }

  async function createBan(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedDeviceId) { setCreateError("Select a registered device first."); return; }
    if (!reason.trim()) { setCreateError("A ban reason is required."); return; }
    setCreating(true); setCreateError(null);
    try {
      await api.createDeviceBan({ device_id: selectedDeviceId, reason: reason.trim(), expires_at: deviceBanExpiry(new Date().toISOString(), durationHours) });
      setCreateOpen(false); toast.success("Device ban created", "The selected device is blocked immediately.");
      if (status !== "active") router.push("/device-bans?status=active"); else await load();
    } catch (err) { setCreateError(err instanceof Error ? err.message : "Device ban could not be created"); }
    finally { setCreating(false); }
  }

  async function lift() {
    if (!target) return;
    setBusy(true);
    try { await api.liftDeviceBan(target.id); setTarget(null); toast.success("Device ban lifted", `${target.user_email} can use this device again.`); await load(); }
    catch (err) { setError(err instanceof Error ? err.message : "Device ban could not be lifted"); }
    finally { setBusy(false); }
  }

  const items = result?.items ?? [];
  const totalPages = result ? Math.max(1, Math.ceil(result.total / result.page_size)) : 1;

  return <div>
    <PageTitle title="Device / HWID Bans" description="Block a registered device in this application. Raw hardware identifiers never leave the server." actions={canWrite ? <Button size="sm" onClick={openCreate}>Ban a device</Button> : undefined} />
    <div className="mb-5 flex flex-wrap items-center gap-2" aria-label="Device ban status filter">
      {[{ value: "active", label: "Active" }, { value: "lifted", label: "Lifted" }, { value: "expired", label: "Expired" }, { value: "", label: "All history" }].map((option) => <button key={option.value || "all"} type="button" onClick={() => setStatus(option.value)} className={`rounded-full px-3.5 py-2 text-sm font-medium transition ${status === option.value ? "bg-brand-500 text-white shadow-theme-xs" : "bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-white/[0.05] dark:text-gray-300 dark:hover:bg-white/[0.08]"}`}>{option.label}</button>)}
      {result && <span className="ml-1 text-sm text-gray-500 dark:text-gray-400">{result.total} record{result.total === 1 ? "" : "s"}</span>}
    </div>
    <ConsoleSection title="Device enforcement" description="Device-specific enforcement with a full audit-friendly history.">
      {loading && !error ? <LoadingNote /> : error ? <ErrorNote message={error} /> : items.length === 0 ? <EmptyNote message="No device bans match this view." /> : <>
        <table className="w-full text-left text-sm"><thead className="border-b border-gray-200 dark:border-gray-800"><tr className="text-xs uppercase text-gray-400"><th className="px-5 py-3 font-medium">Account</th><th className="px-5 py-3 font-medium">Device record</th><th className="px-5 py-3 font-medium">Reason</th><th className="px-5 py-3 font-medium">Status</th><th className="px-5 py-3 font-medium">Ends</th><th className="px-5 py-3 font-medium text-right">Action</th></tr></thead><tbody className="divide-y divide-gray-100 dark:divide-gray-800">{items.map((item) => <tr key={item.id} className="hover:bg-gray-50 dark:hover:bg-white/[0.02]"><td className="px-5 py-4"><Link className="font-medium text-brand-500 hover:text-brand-600 dark:text-brand-400" href={`/users/${item.user_id}`}>{item.user_email}</Link><p className="mt-1 text-xs text-gray-400">Banned {formatDateTime(item.banned_at)}</p></td><td className="px-5 py-4 font-mono text-xs text-gray-500 dark:text-gray-400">{item.device_id.slice(0, 12)}…</td><td className="max-w-xs px-5 py-4 text-gray-700 dark:text-gray-300">{item.reason || "—"}</td><td className="px-5 py-4"><StatusBadge status={item.status} /></td><td className="px-5 py-4 text-gray-500 dark:text-gray-400">{item.expires_at ? formatDateTime(item.expires_at) : "Permanent"}</td><td className="px-5 py-4 text-right">{canWrite && item.status === "active" ? <Button size="sm" variant="outline" onClick={() => setTarget(item)}>Lift ban</Button> : "—"}</td></tr>)}</tbody></table>
        <div className="flex justify-end border-t border-gray-200 px-5 py-4 dark:border-gray-800"><Pagination currentPage={result?.page ?? 1} totalPages={totalPages} onPageChange={(next) => setPage(Math.max(1, Math.min(next, totalPages)))} /></div>
      </>}
    </ConsoleSection>
    <Modal isOpen={createOpen} onClose={() => !creating && setCreateOpen(false)} className="max-w-xl p-6"><form onSubmit={createBan} className="space-y-5"><div><h3 className="text-lg font-semibold text-gray-800 dark:text-white/90">Ban a registered device</h3><p className="mt-1 text-sm text-gray-500 dark:text-gray-400">Choose the device here; you do not need to leave this page.</p></div><div><label className="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">Device</label><select required value={selectedDeviceId} disabled={devicesLoading} onChange={(event) => setSelectedDeviceId(event.target.value)} className="h-11 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90"><option value="">{devicesLoading ? "Loading devices…" : "Select a device"}</option>{devices.map((device) => <option key={device.id} value={device.id}>{device.user_email} · {device.id.slice(0, 10)}… · seen {formatDateTime(device.last_seen_at)}</option>)}</select>{!devicesLoading && devices.length === 0 && <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">No active registered device is available in the selected application.</p>}</div><div><label className="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">Reason</label><textarea required value={reason} onChange={(event) => setReason(event.target.value)} rows={3} placeholder="Why is this device being blocked?" className="w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 text-sm text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90" /></div><div><label className="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">Duration</label><div className="grid grid-cols-2 gap-2 sm:grid-cols-4">{durations.map((option) => <button key={option.label} type="button" onClick={() => setDurationHours(option.hours)} className={`rounded-lg border px-3 py-2 text-sm font-medium ${durationHours === option.hours ? "border-brand-500 bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300" : "border-gray-200 text-gray-600 dark:border-gray-700 dark:text-gray-300"}`}>{option.label}</button>)}</div></div>{createError && <p role="alert" className="text-sm text-error-500">{createError}</p>}<div className="flex justify-end gap-2"><button type="button" onClick={() => setCreateOpen(false)} className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 dark:border-gray-700 dark:text-gray-300">Cancel</button><Button size="sm" disabled={creating || devicesLoading}>{creating ? "Creating…" : "Create device ban"}</Button></div></form></Modal>
    <ConfirmModal isOpen={target !== null} title="Lift device ban" message={target ? `Allow ${target.user_email}'s selected device to authenticate again?` : ""} confirmLabel="Lift ban" busy={busy} error={null} onConfirm={lift} onClose={() => !busy && setTarget(null)} />
  </div>;
}

export default function DeviceBansPage() { return <Suspense fallback={<LoadingNote />}><DeviceBansContent /></Suspense>; }
