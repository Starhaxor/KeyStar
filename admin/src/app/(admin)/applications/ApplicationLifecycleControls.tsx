"use client";

import ConfirmModal from "@/components/console/ConfirmModal";
import Field from "@/components/form/Field";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { api } from "@/lib/api";
import { reportClientError } from "@/lib/clientError";
import type { Application } from "@/lib/types";
import { useState } from "react";

const inputClass = "h-10 w-full rounded-lg border border-gray-300 bg-white px-3 text-sm text-gray-800 dark:border-gray-700 dark:bg-gray-900 dark:text-white/90 dark:[color-scheme:dark]";
type ApplicationStatus = "active" | "maintenance" | "disabled";

function statusLabel(status: ApplicationStatus) {
  return status === "active" ? "Set active" : status === "maintenance" ? "Set maintenance" : "Disable application";
}

export default function ApplicationLifecycleControls({ application, canWrite, onRefresh }: { application: Application; canWrite: boolean; onRefresh: () => Promise<void> }) {
  const [editOpen, setEditOpen] = useState(false);
  const [name, setName] = useState(application.name);
  const [slug, setSlug] = useState(application.slug);
  const [authProfile, setAuthProfile] = useState<"legacy" | "proof_bound">(application.auth_profile === "proof_bound" ? "proof_bound" : "legacy");
  const [hasActiveSigningKey, setHasActiveSigningKey] = useState<boolean | null>(null);
  const [status, setStatus] = useState<ApplicationStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);
  const [transitionError, setTransitionError] = useState<string | null>(null);

  if (!canWrite) return null;

  function openEdit() {
    setName(application.name);
    setSlug(application.slug);
    setAuthProfile(application.auth_profile === "proof_bound" ? "proof_bound" : "legacy");
    setEditError(null);
    setHasActiveSigningKey(null);
    setEditOpen(true);
    void api.applicationSigningKeys(application.id).then(
      (result) => setHasActiveSigningKey(result.items.some((key) => key.status === "active")),
      () => setHasActiveSigningKey(false),
    );
  }

  async function saveApplication() {
    setBusy(true);
    setEditError(null);
    try {
      await api.updateApplication(application.id, { name: name.trim(), slug: slug.trim(), auth_profile: authProfile });
      setEditOpen(false);
      await onRefresh();
    } catch (error) {
      setEditError(reportClientError(error, "Unable to update the application. Try again."));
    } finally {
      setBusy(false);
    }
  }

  async function transitionApplication() {
    if (!status) return;
    setBusy(true);
    setTransitionError(null);
    try {
      await api.transitionApplication(application.id, status);
      setStatus(null);
      await onRefresh();
    } catch (error) {
      setTransitionError(reportClientError(error, "Unable to change application status. Try again."));
    } finally {
      setBusy(false);
    }
  }

  return <div className="flex flex-wrap gap-2">
    <Button type="button" size="sm" variant="outline" onClick={openEdit}>Edit application</Button>
    {(["active", "maintenance", "disabled"] as ApplicationStatus[]).filter((next) => next !== application.status).map((next) => <Button key={next} type="button" size="sm" variant={next === "disabled" ? "danger" : "outline"} onClick={() => { setTransitionError(null); setStatus(next); }}>{statusLabel(next)}</Button>)}
    <Modal isOpen={editOpen} onClose={() => !busy && setEditOpen(false)} title="Edit application" className="max-w-md p-6">
      <h2 className="text-lg font-semibold">Edit application</h2>
      <form className="mt-4 space-y-4" onSubmit={(event) => { event.preventDefault(); void saveApplication(); }}>
        <Field id={`application-name-${application.id}`} label="Application name" name="name"><input className={inputClass} value={name} onChange={(event) => setName(event.target.value)} /></Field>
        <Field id={`application-slug-${application.id}`} label="Slug" name="slug"><input className={inputClass} value={slug} onChange={(event) => setSlug(event.target.value)} /></Field>
        <Field id={`application-auth-profile-${application.id}`} label="Authentication profile" name="auth_profile">
          <select className={inputClass} value={authProfile} onChange={(event) => setAuthProfile(event.target.value === "proof_bound" ? "proof_bound" : "legacy")}>
            <option value="legacy">legacy</option>
            <option value="proof_bound" disabled={hasActiveSigningKey === false}>proof_bound</option>
          </select>
        </Field>
        {authProfile === "proof_bound" && (
          <p role="note" className="text-sm text-amber-700 dark:text-amber-200">
            Switching to proof_bound requires an active application signing key. Refresh tokens and Bearer clients stop working for this application.
          </p>
        )}
        {hasActiveSigningKey === false && (
          <p role="note" className="text-sm text-gray-500">No active signing key found; proof_bound selection is disabled until a key is activated.</p>
        )}
        {editError && <p role="alert" className="text-sm text-error-500">{editError}</p>}
        <div className="flex justify-end gap-2"><Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => setEditOpen(false)}>Cancel</Button><Button type="submit" size="sm" disabled={busy || !name.trim() || !slug.trim()}>Save changes</Button></div>
      </form>
    </Modal>
    <ConfirmModal isOpen={status !== null} title="Change application status" message={status ? `Change ${application.name} to ${status}? This can affect application access.` : ""} confirmLabel="Confirm status change" busy={busy} error={transitionError} onConfirm={() => void transitionApplication()} onClose={() => !busy && setStatus(null)} />
  </div>;
}
