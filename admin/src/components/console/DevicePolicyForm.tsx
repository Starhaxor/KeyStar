"use client";

import Button from "@/components/ui/button/Button";
import { api } from "@/lib/api";
import type { DevicePolicy } from "@/lib/types";
import React, { useEffect, useState } from "react";

const field = "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm dark:border-gray-700";

export default function DevicePolicyForm({ onClose }: { onClose: () => void }) {
  const [policy, setPolicy] = useState<DevicePolicy | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void (async () => {
      try { setPolicy((await api.devicePolicy()).policy); }
      catch (err) { setError(err instanceof Error ? err.message : "Device policy could not be loaded"); }
    })();
  }, []);

  async function save() {
    if (!policy) return;
    setBusy(true); setError(null);
    try {
      const { id, application_id, created_at, updated_at, ...input } = policy;
      void id; void application_id; void created_at; void updated_at;
      setPolicy((await api.updateDevicePolicy(input)).policy);
      onClose();
    } catch (err) { setError(err instanceof Error ? err.message : "Device policy could not be saved"); }
    finally { setBusy(false); }
  }

  async function reset() {
    setBusy(true); setError(null);
    try { await api.resetDevicePolicy(); setPolicy((await api.devicePolicy()).policy); }
    catch (err) { setError(err instanceof Error ? err.message : "Device policy could not be reset"); }
    finally { setBusy(false); }
  }

  if (!policy && !error) return <p className="py-8 text-center text-sm text-gray-500">Loading policy…</p>;
  if (error && !policy) return <p className="py-4 text-sm text-error-500">{error}</p>;
  if (!policy) return null;

  return <div className="space-y-4">
    <p className="text-sm text-gray-500">Controls hardware proof, device matching, and automatic rebinds for the selected application.</p>
    {error && <p className="text-sm text-error-500">{error}</p>}
    <label className="block text-sm">TPM policy<select className={field} value={policy.tpm_policy} onChange={(e) => setPolicy({ ...policy, tpm_policy: e.target.value as DevicePolicy["tpm_policy"] })}><option value="optional">Optional</option><option value="required">Required</option><option value="disabled">Disabled</option></select></label>
    <div className="grid grid-cols-2 gap-4">
      <label className="text-sm">Minimum match score<input className={field} type="number" min="0" value={policy.min_match_score} onChange={(e) => setPolicy({ ...policy, min_match_score: Number(e.target.value) })} /></label>
      <label className="text-sm">Step-up score<input className={field} type="number" min="0" value={policy.step_up_score} onChange={(e) => setPolicy({ ...policy, step_up_score: Number(e.target.value) })} /></label>
      <label className="text-sm">Cooldown seconds<input className={field} type="number" min="0" value={policy.rebind_cooldown_seconds} onChange={(e) => setPolicy({ ...policy, rebind_cooldown_seconds: Number(e.target.value) })} /></label>
      <label className="text-sm">Changes per 30 days<input className={field} type="number" min="0" value={policy.max_device_changes_per_30d} onChange={(e) => setPolicy({ ...policy, max_device_changes_per_30d: Number(e.target.value) })} /></label>
    </div>
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={policy.allow_auto_rebind} onChange={(e) => setPolicy({ ...policy, allow_auto_rebind: e.target.checked })} /> Allow automatic rebind</label>
    <div className="flex justify-end gap-2"><Button size="sm" variant="outline" onClick={reset} disabled={busy}>Reset defaults</Button><Button size="sm" onClick={save} disabled={busy}>Save policy</Button></div>
  </div>;
}

