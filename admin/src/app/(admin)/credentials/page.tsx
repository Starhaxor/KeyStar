"use client";

import ConfirmModal from "@/components/console/ConfirmModal";
import { EmptyNote, ErrorNote, LoadingNote, PageTitle, TableCard } from "@/components/console/ConsoleSection";
import TimeAgo from "@/components/common/TimeAgo";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { api } from "@/lib/api";
import { reportClientError } from "@/lib/clientError";
import {
  defaultScopesForCredentialType,
  scopeOptionsForCredentialType,
  type CredentialType,
} from "@/lib/credentialScopes";
import type { ApplicationCredential } from "@/lib/types";
import React, { useCallback, useEffect, useState } from "react";

const inputClass = "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90";

export default function CredentialsPage() {
  const [items, setItems] = useState<ApplicationCredential[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [revoke, setRevoke] = useState<ApplicationCredential | null>(null);
  const [rotating, setRotating] = useState<ApplicationCredential | null>(null);
  const [graceHours, setGraceHours] = useState(24);
  const [rotateBusy, setRotateBusy] = useState(false);
  const [busy, setBusy] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [environment, setEnvironment] = useState<"test" | "live">("live");
  const [type, setType] = useState<CredentialType>("publishable");
  const [scopes, setScopes] = useState<string[]>(() => defaultScopesForCredentialType("publishable"));

  const load = useCallback(async () => {
    setLoading(true); setError(null);
    try { setItems((await api.credentials()).credentials); }
    catch (err) { setError(reportClientError(err, "Unable to load credentials. Try again.")); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { void load(); }, [load]);

  async function create() {
    setBusy(true); setError(null);
    try {
      const result = await api.createCredential({
        name: name.trim(), environment, type,
        scopes,
      });
      setSecret(result.key); setCreateOpen(false); setName(""); await load();
    } catch (err) { setError(reportClientError(err, "Unable to create the credential. Try again.")); }
    finally { setBusy(false); }
  }

  function changeCredentialType(nextType: CredentialType) {
    setType(nextType);
    setScopes(defaultScopesForCredentialType(nextType));
  }

  function toggleScope(scope: string) {
    setScopes((current) => current.includes(scope)
      ? current.filter((item) => item !== scope)
      : [...current, scope]);
  }

  async function revokeCredential() {
    if (!revoke) return;
    setBusy(true);
    try { await api.revokeCredential(revoke.id); setRevoke(null); await load(); }
    catch (err) { setError(reportClientError(err, "Unable to revoke the credential. Try again.")); }
    finally { setBusy(false); }
  }

  async function rotateCredential() {
    if (!rotating) return;
    setRotateBusy(true);
    try {
      const result = await api.rotateCredential(rotating.id, graceHours);
      setRotating(null);
      setSecret(result.key);
      await load();
    } catch (err) {
      setError(reportClientError(err, "Unable to rotate the credential. Try again."));
    } finally {
      setRotateBusy(false);
    }
  }

  return (
    <>
      <PageTitle title="API Credentials" description="Create and revoke application keys. Secret keys are shown only once." actions={<Button size="sm" onClick={() => setCreateOpen(true)}>Create credential</Button>} />
      {error && <div className="mb-4"><ErrorNote message={error} onRetry={load} /></div>}
      <TableCard>
        {loading ? <LoadingNote /> : items.length === 0 ? <EmptyNote message="No credentials have been created for this application." /> : (
          <table className="min-w-full text-left text-sm">
            <thead className="border-b border-gray-200 text-xs uppercase text-gray-500 dark:border-gray-800"><tr><th className="px-5 py-3">Name</th><th>Type</th><th>Prefix</th><th>Scopes</th><th>Status</th><th>Last used</th><th className="px-5 py-3 text-right">Action</th></tr></thead>
            <tbody>{items.map((item) => <tr key={item.id} className="border-b border-gray-100 dark:border-gray-800"><td className="px-5 py-4 font-medium">{item.name}<div className="text-xs text-gray-500">{item.environment}</div></td><td>{item.type}</td><td><code>{item.key_prefix}</code></td><td className="max-w-xs truncate">{item.scopes.join(", ")}</td><td>{item.status}</td><td className="whitespace-nowrap px-5 py-4 text-gray-500 dark:text-gray-400"><TimeAgo value={item.last_used_at} /></td><td className="px-5 text-right"><div className="flex justify-end gap-2">{item.status === "active" && (<><Button size="sm" variant="outline" onClick={() => { setGraceHours(24); setRotating(item); }}>Rotate</Button><Button size="sm" variant="outline" onClick={() => setRevoke(item)}>Revoke</Button></>)}</div></td></tr>)}</tbody>
          </table>
        )}
      </TableCard>

      <Modal isOpen={createOpen} onClose={() => !busy && setCreateOpen(false)} className="max-w-2xl p-6">
        <h2 className="text-lg font-semibold">Create credential</h2>
        <div className="mt-4 space-y-4">
          <label className="block text-sm">Name<input className={inputClass} value={name} onChange={(event) => setName(event.target.value)} /></label>
          <div className="grid grid-cols-2 gap-4"><label className="text-sm">Environment<select className={inputClass} value={environment} onChange={(event) => setEnvironment(event.target.value as "test" | "live")}><option value="live">Live</option><option value="test">Test</option></select></label><label className="text-sm">Key type<select className={inputClass} value={type} onChange={(event) => changeCredentialType(event.target.value as CredentialType)}><option value="publishable">Publishable</option><option value="secret">Secret</option></select></label></div>
          <section aria-labelledby="credential-scopes-heading">
            <div className="flex flex-wrap items-baseline justify-between gap-2"><div><h3 id="credential-scopes-heading" className="text-sm font-medium">Permissions</h3><p className="mt-1 text-xs text-gray-500">Select only the permissions this key needs.</p></div><span className="text-xs font-medium text-gray-500">{scopes.length} selected</span></div>
            {type === "publishable" && <div className="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-brand-200 bg-brand-50 px-3 py-2 text-xs text-brand-700 dark:border-brand-500/30 dark:bg-brand-500/10 dark:text-brand-300"><span><strong>Desktop clients:</strong> Sign-in, device verification, refresh, and logout are selected.</span><button type="button" onClick={() => setScopes(defaultScopesForCredentialType("publishable"))} className="font-semibold underline underline-offset-2">Reset to recommended</button></div>}
            <div className="mt-3 grid gap-2 sm:grid-cols-2">
              {scopeOptionsForCredentialType(type).map((scope) => {
                const selected = scopes.includes(scope.value);
                return <label key={scope.value} className={`flex cursor-pointer gap-3 rounded-xl border p-3 transition-colors ${selected ? "border-brand-500 bg-brand-50/70 dark:border-brand-500/60 dark:bg-brand-500/10" : "border-gray-200 bg-white hover:border-gray-300 dark:border-gray-800 dark:bg-white/[0.03] dark:hover:border-gray-700"}`}>
                  <input type="checkbox" checked={selected} onChange={() => toggleScope(scope.value)} className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-500 focus:ring-brand-500" />
                  <span><span className="block text-sm font-medium text-gray-800 dark:text-white/90">{scope.label}</span><span className="mt-0.5 block text-xs text-gray-500">{scope.description}</span></span>
                </label>;
              })}
            </div>
          </section>
          <div className="flex justify-end gap-2"><Button variant="outline" size="sm" onClick={() => setCreateOpen(false)} disabled={busy}>Cancel</Button><Button size="sm" onClick={create} disabled={busy || !name.trim() || scopes.length === 0}>Create</Button></div>
        </div>
      </Modal>

      <Modal isOpen={secret !== null} onClose={() => setSecret(null)} className="max-w-lg p-6">
        <h2 className="text-lg font-semibold">Copy your key now</h2><p className="mt-2 text-sm text-gray-500">For security, this secret will not be displayed again.</p>
        <code className="mt-4 block break-all rounded-lg bg-gray-100 p-3 text-sm dark:bg-gray-800">{secret}</code>
        <div className="mt-5 flex justify-end"><Button size="sm" onClick={() => setSecret(null)}>Done</Button></div>
      </Modal>

      <ConfirmModal isOpen={revoke !== null} title="Revoke credential" message="This key will stop working immediately and cannot be restored." confirmLabel="Revoke" busy={busy} error={null} onConfirm={revokeCredential} onClose={() => !busy && setRevoke(null)} />

      <Modal isOpen={rotating !== null} onClose={() => !rotateBusy && setRotating(null)} className="max-w-md p-6">
        <h2 className="text-lg font-semibold">Rotate credential</h2>
        {rotating && (
          <>
            <p className="mt-2 text-sm text-gray-500">
              Issues a fresh key with the same name, type and scopes as
              <strong> {rotating.name}</strong>. Choose how long the old key
              stays valid.
            </p>
            <label className="mt-4 block text-sm">
              Grace period for the old key
              <select className={inputClass + " mt-1.5"} value={graceHours} onChange={(event) => setGraceHours(Number(event.target.value))}>
                <option value={0}>Revoke immediately</option>
                <option value={24}>24 hours (recommended)</option>
                <option value={72}>3 days</option>
                <option value={168}>7 days</option>
              </select>
            </label>
            <div className="mt-5 flex justify-end gap-2">
              <Button size="sm" variant="outline" disabled={rotateBusy} onClick={() => setRotating(null)}>Cancel</Button>
              <Button size="sm" disabled={rotateBusy} onClick={rotateCredential}>{rotateBusy ? "Rotating..." : "Rotate key"}</Button>
            </div>
          </>
        )}
      </Modal>
    </>
  );
}

