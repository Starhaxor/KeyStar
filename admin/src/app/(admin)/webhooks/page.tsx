"use client";

import ConfirmModal from "@/components/console/ConfirmModal";
import { EmptyNote, ErrorNote, LoadingNote, PageTitle } from "@/components/console/ConsoleSection";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { api } from "@/lib/api";
import type { Webhook } from "@/lib/types";
import React, { useCallback, useEffect, useState } from "react";

const inputClass = "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90";

export default function WebhooksPage() {
  const [items, setItems] = useState<Webhook[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [url, setURL] = useState("");
  const [events, setEvents] = useState("license.created");
  const [busy, setBusy] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [removing, setRemoving] = useState<Webhook | null>(null);

  const load = useCallback(async () => {
    setLoading(true); setError(null);
    try { setItems((await api.webhooks()).items); }
    catch (err) { setError(err instanceof Error ? err.message : "Webhooks could not be loaded"); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  const selectedEvents = () => events.split(",").map((event) => event.trim()).filter(Boolean);
  async function create() {
    setBusy(true); setError(null);
    try { const result = await api.createWebhook({ url: url.trim(), events: selectedEvents() }); setSecret(result.secret); setURL(""); setCreateOpen(false); await load(); }
    catch (err) { setError(err instanceof Error ? err.message : "Webhook could not be created"); }
    finally { setBusy(false); }
  }
  async function toggle(item: Webhook) {
    setBusy(true); setError(null);
    try { await api.updateWebhook(item.id, { status: item.status === "active" ? "disabled" : "active" }); await load(); }
    catch (err) { setError(err instanceof Error ? err.message : "Webhook could not be updated"); }
    finally { setBusy(false); }
  }
  async function remove() {
    if (!removing) return;
    setBusy(true);
    try { await api.deleteWebhook(removing.id); setRemoving(null); await load(); }
    catch (err) { setError(err instanceof Error ? err.message : "Webhook could not be deleted"); }
    finally { setBusy(false); }
  }

  return <>
    <PageTitle title="Webhooks" description="Send signed event notifications to your application services." actions={<Button size="sm" onClick={() => setCreateOpen(true)}>Add endpoint</Button>} />
    {error && <div className="mb-4"><ErrorNote message={error} /></div>}
    <div className="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03]">
      {loading ? <LoadingNote /> : items.length === 0 ? <EmptyNote message="No webhook endpoints configured for this application." /> : <table className="min-w-full text-left text-sm"><thead className="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-gray-800"><tr><th className="px-5 py-3">Endpoint</th><th>Events</th><th>Status</th><th className="px-5 py-3 text-right">Actions</th></tr></thead><tbody>{items.map((item) => <tr key={item.id} className="border-b border-gray-100 dark:border-gray-800"><td className="max-w-md truncate px-5 py-4 font-mono text-xs">{item.url}</td><td className="max-w-sm truncate">{item.events.join(", ")}</td><td><span className={item.status === "active" ? "text-success-600" : "text-gray-500"}>{item.status}</span></td><td className="space-x-2 px-5 text-right"><Button size="sm" variant="outline" disabled={busy} onClick={() => toggle(item)}>{item.status === "active" ? "Disable" : "Enable"}</Button><Button size="sm" variant="outline" disabled={busy} onClick={() => setRemoving(item)}>Delete</Button></td></tr>)}</tbody></table>}
    </div>
    <Modal isOpen={createOpen} onClose={() => !busy && setCreateOpen(false)} className="max-w-lg p-6"><h2 className="text-lg font-semibold">Add webhook endpoint</h2><div className="mt-4 space-y-4"><label className="block text-sm">Endpoint URL<input className={inputClass} placeholder="https://example.com/hooks/keystar" value={url} onChange={(event) => setURL(event.target.value)} /></label><label className="block text-sm">Events (comma separated)<input className={inputClass} value={events} onChange={(event) => setEvents(event.target.value)} /></label><p className="text-xs text-gray-500">Examples: license.created, license.revoked, user.created, device.bound</p><div className="flex justify-end gap-2"><Button size="sm" variant="outline" disabled={busy} onClick={() => setCreateOpen(false)}>Cancel</Button><Button size="sm" disabled={busy || !url.trim() || selectedEvents().length === 0} onClick={create}>Create</Button></div></div></Modal>
    <Modal isOpen={secret !== null} onClose={() => setSecret(null)} className="max-w-lg p-6"><h2 className="text-lg font-semibold">Copy the signing secret now</h2><p className="mt-2 text-sm text-gray-500">It is shown only once. Keep it in your server-side secret manager.</p><code className="mt-4 block break-all rounded-lg bg-gray-100 p-3 text-sm dark:bg-gray-800">{secret}</code><div className="mt-5 flex justify-end"><Button size="sm" onClick={() => setSecret(null)}>Done</Button></div></Modal>
    <ConfirmModal isOpen={removing !== null} title="Delete webhook" message="Pending deliveries will no longer be sent to this endpoint." confirmLabel="Delete" busy={busy} error={null} onConfirm={remove} onClose={() => !busy && setRemoving(null)} />
  </>;
}
