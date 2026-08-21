"use client";

import ConfirmModal from "@/components/console/ConfirmModal";
import { EmptyNote, ErrorNote, LoadingNote, PageTitle } from "@/components/console/ConsoleSection";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { api } from "@/lib/api";
import type { ApplicationCredential } from "@/lib/types";
import React, { useCallback, useEffect, useState } from "react";

const inputClass = "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90";

export default function CredentialsPage() {
  const [items, setItems] = useState<ApplicationCredential[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [revoke, setRevoke] = useState<ApplicationCredential | null>(null);
  const [busy, setBusy] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [environment, setEnvironment] = useState<"test" | "live">("live");
  const [type, setType] = useState<"publishable" | "secret">("secret");
  const [scopes, setScopes] = useState("users.read");

  const load = useCallback(async () => {
    setLoading(true); setError(null);
    try { setItems((await api.credentials()).credentials); }
    catch (err) { setError(err instanceof Error ? err.message : "Credentials could not be loaded"); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { void load(); }, [load]);

  async function create() {
    setBusy(true); setError(null);
    try {
      const result = await api.createCredential({
        name: name.trim(), environment, type,
        scopes: scopes.split(",").map((scope) => scope.trim()).filter(Boolean),
      });
      setSecret(result.key); setCreateOpen(false); setName(""); await load();
    } catch (err) { setError(err instanceof Error ? err.message : "Credential could not be created"); }
    finally { setBusy(false); }
  }

  async function revokeCredential() {
    if (!revoke) return;
    setBusy(true);
    try { await api.revokeCredential(revoke.id); setRevoke(null); await load(); }
    catch (err) { setError(err instanceof Error ? err.message : "Credential could not be revoked"); }
    finally { setBusy(false); }
  }

  return (
    <>
      <PageTitle title="API Credentials" description="Create and revoke application keys. Secret keys are shown only once." actions={<Button size="sm" onClick={() => setCreateOpen(true)}>Create credential</Button>} />
      {error && <div className="mb-4"><ErrorNote message={error} /></div>}
      <div className="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03]">
        {loading ? <LoadingNote /> : items.length === 0 ? <EmptyNote message="No credentials have been created for this application." /> : (
          <table className="min-w-full text-left text-sm">
            <thead className="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-gray-800"><tr><th className="px-5 py-3">Name</th><th>Type</th><th>Prefix</th><th>Scopes</th><th>Status</th><th className="px-5 py-3 text-right">Action</th></tr></thead>
            <tbody>{items.map((item) => <tr key={item.id} className="border-b border-gray-100 dark:border-gray-800"><td className="px-5 py-4 font-medium">{item.name}<div className="text-xs text-gray-500">{item.environment}</div></td><td>{item.type}</td><td><code>{item.key_prefix}</code></td><td className="max-w-xs truncate">{item.scopes.join(", ")}</td><td>{item.status}</td><td className="px-5 text-right">{item.status === "active" && <Button size="sm" variant="outline" onClick={() => setRevoke(item)}>Revoke</Button>}</td></tr>)}</tbody>
          </table>
        )}
      </div>

      <Modal isOpen={createOpen} onClose={() => !busy && setCreateOpen(false)} className="max-w-lg p-6">
        <h2 className="text-lg font-semibold">Create credential</h2>
        <div className="mt-4 space-y-4">
          <label className="block text-sm">Name<input className={inputClass} value={name} onChange={(event) => setName(event.target.value)} /></label>
          <div className="grid grid-cols-2 gap-4"><label className="text-sm">Environment<select className={inputClass} value={environment} onChange={(event) => setEnvironment(event.target.value as "test" | "live")}><option value="live">Live</option><option value="test">Test</option></select></label><label className="text-sm">Key type<select className={inputClass} value={type} onChange={(event) => setType(event.target.value as "publishable" | "secret")}><option value="secret">Secret</option><option value="publishable">Publishable</option></select></label></div>
          <label className="block text-sm">Scopes (comma separated)<input className={inputClass} value={scopes} onChange={(event) => setScopes(event.target.value)} /></label>
          <div className="flex justify-end gap-2"><Button variant="outline" size="sm" onClick={() => setCreateOpen(false)} disabled={busy}>Cancel</Button><Button size="sm" onClick={create} disabled={busy || !name.trim() || !scopes.trim()}>Create</Button></div>
        </div>
      </Modal>

      <Modal isOpen={secret !== null} onClose={() => setSecret(null)} className="max-w-lg p-6">
        <h2 className="text-lg font-semibold">Copy your key now</h2><p className="mt-2 text-sm text-gray-500">For security, this secret will not be displayed again.</p>
        <code className="mt-4 block break-all rounded-lg bg-gray-100 p-3 text-sm dark:bg-gray-800">{secret}</code>
        <div className="mt-5 flex justify-end"><Button size="sm" onClick={() => setSecret(null)}>Done</Button></div>
      </Modal>

      <ConfirmModal isOpen={revoke !== null} title="Revoke credential" message="This key will stop working immediately and cannot be restored." confirmLabel="Revoke" busy={busy} error={null} onConfirm={revokeCredential} onClose={() => !busy && setRevoke(null)} />
    </>
  );
}

