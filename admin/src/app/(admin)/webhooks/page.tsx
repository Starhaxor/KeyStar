"use client";

import ConfirmModal from "@/components/console/ConfirmModal";
import { EmptyNote, ErrorNote, LoadingNote, PageTitle } from "@/components/console/ConsoleSection";
import TimeAgo from "@/components/common/TimeAgo";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { useToast } from "@/context/ToastContext";
import { api } from "@/lib/api";
import type { Webhook, WebhookDelivery, WebhookDeliveryStatus } from "@/lib/types";
import React, { useCallback, useEffect, useState } from "react";

const inputClass = "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90";

const deliveryStatusStyles: Record<WebhookDeliveryStatus, string> = {
  delivered: "bg-success-50 text-success-600 dark:bg-success-500/10 dark:text-success-400",
  pending: "bg-blue-light-50 text-blue-light-600 dark:bg-blue-light-500/10 dark:text-blue-light-400",
  delivering: "bg-warning-50 text-warning-600 dark:bg-warning-500/10 dark:text-warning-400",
  failed: "bg-error-50 text-error-600 dark:bg-error-500/10 dark:text-error-400",
};

function DeliveriesModal({
  webhook,
  onClose,
  onRetryDone,
}: {
  webhook: Webhook;
  onClose: () => void;
  onRetryDone: () => void;
}) {
  const toast = useToast();
  const [deliveries, setDeliveries] = useState<WebhookDelivery[] | null>(null);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [retrying, setRetrying] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const response = await api.webhookDeliveries(webhook.id, page);
      setDeliveries(response.items);
      setTotal(response.total);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Deliveries could not be loaded");
    }
  }, [webhook.id, page]);

  useEffect(() => {
    void load();
  }, [load]);

  async function retry(delivery: WebhookDelivery) {
    setRetrying(delivery.id);
    try {
      await api.retryWebhookDelivery(webhook.id, delivery.id);
      toast.success("Delivery requeued", `${delivery.event_type} will be sent again shortly.`);
      await load();
      onRetryDone();
    } catch (err) {
      toast.error("Retry failed", err instanceof Error ? err.message : "Unknown error");
    } finally {
      setRetrying(null);
    }
  }

  return (
    <Modal isOpen onClose={onClose} className="max-w-4xl p-6">
      <h2 className="text-lg font-semibold">Delivery history</h2>
      <p className="mt-1 break-all text-sm text-gray-500 dark:text-gray-400">{webhook.url}</p>
      <div className="custom-scrollbar mt-4 max-h-96 overflow-y-auto">
        {error ? (
          <ErrorNote message={error} />
        ) : deliveries === null ? (
          <LoadingNote />
        ) : deliveries.length === 0 ? (
          <EmptyNote message="No events have been enqueued for this endpoint yet." />
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-gray-800">
              <tr>
                <th className="px-3 py-2.5">Event</th>
                <th className="px-3 py-2.5">Status</th>
                <th className="px-3 py-2.5">Attempts</th>
                <th className="px-3 py-2.5">Created</th>
                <th className="px-3 py-2.5" colSpan={2}>Detail</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {deliveries.map((delivery) => (
                <tr key={delivery.id}>
                  <td className="px-3 py-3 font-medium">{delivery.event_type}</td>
                  <td className="px-3 py-3">
                    <span className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${deliveryStatusStyles[delivery.status]}`}>
                      {delivery.status}
                    </span>
                  </td>
                  <td className="px-3 py-3 text-gray-500 dark:text-gray-400">
                    {delivery.attempts}/{delivery.max_attempts}
                  </td>
                  <td className="whitespace-nowrap px-3 py-3 text-gray-500 dark:text-gray-400">
                    <TimeAgo value={delivery.created_at} />
                  </td>
                  <td className="max-w-xs truncate px-3 py-3 text-xs text-gray-400 dark:text-gray-500" title={delivery.last_error || undefined}>
                    {delivery.status === "delivered"
                      ? `Delivered ${delivery.delivered_at ? new Date(delivery.delivered_at).toLocaleString() : ""}`
                      : delivery.last_error || `Next attempt ${new Date(delivery.next_attempt_at).toLocaleString()}`}
                  </td>
                  <td className="px-3 py-3 text-right">
                    {(delivery.status === "failed" || delivery.status === "delivered") && (
                      <Button size="sm" variant="outline" disabled={retrying !== null} onClick={() => retry(delivery)}>
                        {retrying === delivery.id ? "..." : "Retry"}
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      <div className="mt-4 flex items-center justify-between">
        <span className="text-xs text-gray-500 dark:text-gray-400">{total} delivery record(s)</span>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>Previous</Button>
          <Button size="sm" variant="outline" disabled={deliveries !== null && page * 20 >= total} onClick={() => setPage((p) => p + 1)}>Next</Button>
          <Button size="sm" onClick={onClose}>Close</Button>
        </div>
      </div>
    </Modal>
  );
}

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
  const [deliveriesFor, setDeliveriesFor] = useState<Webhook | null>(null);

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

  function openDeliveries(item: Webhook) {
    setDeliveriesFor(item);
  }

  return <>
    <PageTitle title="Webhooks" description="Send signed event notifications to your application services." actions={<Button size="sm" onClick={() => setCreateOpen(true)}>Add endpoint</Button>} />
    {error && <div className="mb-4"><ErrorNote message={error} /></div>}
    <div className="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03]">
      {loading ? <LoadingNote /> : items.length === 0 ? <EmptyNote message="No webhook endpoints configured for this application." /> : <table className="min-w-full text-left text-sm"><thead className="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-gray-800"><tr><th className="px-5 py-3">Endpoint</th><th>Events</th><th>Status</th><th className="px-5 py-3 text-right">Actions</th></tr></thead><tbody>{items.map((item) => <tr key={item.id} className="border-b border-gray-100 dark:border-gray-800"><td className="max-w-md truncate px-5 py-4 font-mono text-xs">{item.url}</td><td className="max-w-sm truncate">{item.events.join(", ")}</td><td><span className={item.status === "active" ? "text-success-600" : "text-gray-500"}>{item.status}</span></td><td className="space-x-2 px-5 text-right"><Button size="sm" variant="outline" onClick={() => openDeliveries(item)}>Deliveries</Button>{item.status === "active" && <Button size="sm" variant="outline" disabled={busy} onClick={() => toggle(item)}>Disable</Button>}{item.status === "disabled" && <Button size="sm" variant="outline" disabled={busy} onClick={() => toggle(item)}>Enable</Button>}<Button size="sm" variant="outline" disabled={busy} onClick={() => setRemoving(item)}>Delete</Button></td></tr>)}</tbody></table>}
    </div>
    <Modal isOpen={createOpen} onClose={() => !busy && setCreateOpen(false)} className="max-w-lg p-6"><h2 className="text-lg font-semibold">Add webhook endpoint</h2><div className="mt-4 space-y-4"><label className="block text-sm">Endpoint URL<input className={inputClass} placeholder="https://example.com/hooks/keystar" value={url} onChange={(event) => setURL(event.target.value)} /></label><label className="block text-sm">Events (comma separated)<input className={inputClass} value={events} onChange={(event) => setEvents(event.target.value)} /></label><p className="text-xs text-gray-500">Examples: license.created, license.revoked, user.created, device.bound</p><div className="flex justify-end gap-2"><Button size="sm" variant="outline" disabled={busy} onClick={() => setCreateOpen(false)}>Cancel</Button><Button size="sm" disabled={busy || !url.trim() || selectedEvents().length === 0} onClick={create}>Create</Button></div></div></Modal>
    <Modal isOpen={secret !== null} onClose={() => setSecret(null)} className="max-w-lg p-6"><h2 className="text-lg font-semibold">Copy the signing secret now</h2><p className="mt-2 text-sm text-gray-500">It is shown only once. Keep it in your server-side secret manager.</p><code className="mt-4 block break-all rounded-lg bg-gray-100 p-3 text-sm dark:bg-gray-800">{secret}</code><div className="mt-5 flex justify-end"><Button size="sm" onClick={() => setSecret(null)}>Done</Button></div></Modal>
    <ConfirmModal isOpen={removing !== null} title="Delete webhook" message="Pending deliveries will no longer be sent to this endpoint." confirmLabel="Delete" busy={busy} error={null} onConfirm={remove} onClose={() => !busy && setRemoving(null)} />
    {deliveriesFor && (
      <DeliveriesModal
        webhook={deliveriesFor}
        onClose={() => setDeliveriesFor(null)}
        onRetryDone={() => {
          void load();
        }}
      />
    )}
  </>;
}
