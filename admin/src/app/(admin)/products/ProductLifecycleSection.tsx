"use client";

import ConfirmModal from "@/components/console/ConfirmModal";
import Field from "@/components/form/Field";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { api } from "@/lib/api";
import { reportClientError } from "@/lib/clientError";
import { formatDuration } from "@/lib/time";
import type { Plan, Product } from "@/lib/types";
import { useState } from "react";

const inputClass = "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90";

function StatusBadge({ status }: { status: string }) {
  const colors = status === "archived" ? "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300" : status === "inactive" ? "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-200" : "bg-success-50 text-success-700 dark:bg-success-500/15 dark:text-success-300";
  return <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${colors}`}>{status}</span>;
}

export default function ProductLifecycleSection({ product, plans, canWrite, onRefresh, onAddPlan }: { product: Product; plans: Plan[]; canWrite: boolean; onRefresh: () => Promise<void>; onAddPlan: (product: Product) => void }) {
  const [productEditOpen, setProductEditOpen] = useState(false);
  const [planEdit, setPlanEdit] = useState<Plan | null>(null);
  const [archiveTarget, setArchiveTarget] = useState<{ kind: "product"; product: Product } | { kind: "plan"; plan: Plan } | null>(null);
  const [productName, setProductName] = useState(product.name);
  const [productSlug, setProductSlug] = useState(product.slug);
  const [planName, setPlanName] = useState("");
  const [planCode, setPlanCode] = useState("");
  const [planLevel, setPlanLevel] = useState(1);
  const [planMaxDevices, setPlanMaxDevices] = useState(1);
  const [busy, setBusy] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);
  const [archiveError, setArchiveError] = useState<string | null>(null);
  const archived = product.status === "archived";

  function openProductEdit() {
    setProductName(product.name);
    setProductSlug(product.slug);
    setEditError(null);
    setProductEditOpen(true);
  }

  function openPlanEdit(plan: Plan) {
    setPlanEdit(plan);
    setPlanName(plan.name);
    setPlanCode(plan.code);
    setPlanLevel(plan.level);
    setPlanMaxDevices(plan.max_devices);
    setEditError(null);
  }

  async function saveProduct() {
    setBusy(true); setEditError(null);
    try {
      await api.updateProduct(product.id, { name: productName.trim(), slug: productSlug.trim() });
      setProductEditOpen(false);
      await onRefresh();
    } catch (error) { setEditError(reportClientError(error, "Unable to update the product. Try again.")); }
    finally { setBusy(false); }
  }

  async function savePlan() {
    if (!planEdit) return;
    setBusy(true); setEditError(null);
    try {
      await api.updatePlan(product.id, planEdit.id, { name: planName.trim(), code: planCode.trim(), level: planLevel, max_devices: planMaxDevices });
      setPlanEdit(null);
      await onRefresh();
    } catch (error) { setEditError(reportClientError(error, "Unable to update the plan. Try again.")); }
    finally { setBusy(false); }
  }

  async function archive() {
    if (!archiveTarget) return;
    setBusy(true); setArchiveError(null);
    try {
      if (archiveTarget.kind === "product") await api.archiveProduct(archiveTarget.product.id);
      else await api.archivePlan(product.id, archiveTarget.plan.id);
      setArchiveTarget(null);
      await onRefresh();
    } catch (error) {
      setArchiveError(reportClientError(error, archiveTarget.kind === "product" ? "Unable to archive the product because it is still in use." : "Unable to archive the plan because it is still in use."));
    } finally { setBusy(false); }
  }

  return <section className="border-b border-gray-200 p-5 last:border-0 dark:border-gray-800">
    <div className="flex flex-wrap items-start justify-between gap-3"><div><div className="flex items-center gap-2"><h2 className="font-semibold">{product.name}</h2><StatusBadge status={product.status} /></div><p className="text-xs text-gray-500">{product.slug}</p></div>{canWrite && !archived && <div className="flex flex-wrap gap-2"><Button type="button" size="sm" variant="outline" onClick={() => onAddPlan(product)}>Add plan</Button><Button type="button" size="sm" variant="outline" onClick={openProductEdit}>Edit product</Button><Button type="button" size="sm" variant="outline" onClick={() => setArchiveTarget({ kind: "product", product })}>Archive product</Button></div>}</div>
    <div className="mt-4 grid gap-3 md:grid-cols-3">{plans.map((plan) => { const planArchived = plan.status === "archived"; return <div key={plan.id} className="rounded-xl border border-gray-200 p-3 text-sm dark:border-gray-700"><div className="flex items-start justify-between gap-2"><strong>{plan.name}</strong><StatusBadge status={plan.status} /></div><p className="mt-1 text-gray-500">{plan.code} · Level {plan.level} · {plan.max_devices} devices</p><p className="mt-1 text-gray-500">Default duration: {formatDuration(plan.default_duration_seconds) ?? "not set"}</p>{canWrite && !archived && !planArchived && <div className="mt-3 flex gap-2"><Button type="button" size="sm" variant="outline" onClick={() => openPlanEdit(plan)}>Edit plan</Button><Button type="button" size="sm" variant="outline" onClick={() => setArchiveTarget({ kind: "plan", plan })}>Archive plan</Button></div>}</div>; })}{plans.length === 0 && <p className="text-sm text-gray-500">No plans yet.</p>}</div>
    <Modal isOpen={productEditOpen} onClose={() => !busy && setProductEditOpen(false)} title="Edit product" className="max-w-md p-6"><h2 className="text-lg font-semibold">Edit product</h2><form className="mt-4 space-y-4" onSubmit={(event) => { event.preventDefault(); void saveProduct(); }}><Field id={`product-name-${product.id}`} label="Product name" name="name"><input className={inputClass} value={productName} onChange={(event) => setProductName(event.target.value)} /></Field><Field id={`product-slug-${product.id}`} label="Slug" name="slug"><input className={inputClass} value={productSlug} onChange={(event) => setProductSlug(event.target.value)} /></Field>{editError && <p role="alert" className="text-sm text-error-500">{editError}</p>}<div className="flex justify-end gap-2"><Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => setProductEditOpen(false)}>Cancel</Button><Button type="submit" size="sm" disabled={busy || !productName.trim() || !productSlug.trim()}>Save changes</Button></div></form></Modal>
    <Modal isOpen={planEdit !== null} onClose={() => !busy && setPlanEdit(null)} title="Edit plan" className="max-w-md p-6"><h2 className="text-lg font-semibold">Edit plan</h2><form className="mt-4 space-y-4" onSubmit={(event) => { event.preventDefault(); void savePlan(); }}><Field id={`plan-name-${planEdit?.id ?? "new"}`} label="Plan name" name="name"><input className={inputClass} value={planName} onChange={(event) => setPlanName(event.target.value)} /></Field><Field id={`plan-code-${planEdit?.id ?? "new"}`} label="Plan code" name="code"><input className={inputClass} value={planCode} onChange={(event) => setPlanCode(event.target.value)} /></Field><div className="grid grid-cols-2 gap-3"><Field id={`plan-level-${planEdit?.id ?? "new"}`} label="Level" name="level"><input className={inputClass} type="number" min="1" value={planLevel} onChange={(event) => setPlanLevel(Number(event.target.value))} /></Field><Field id={`plan-max-devices-${planEdit?.id ?? "new"}`} label="Max devices" name="max_devices"><input className={inputClass} type="number" min="1" value={planMaxDevices} onChange={(event) => setPlanMaxDevices(Number(event.target.value))} /></Field></div>{editError && <p role="alert" className="text-sm text-error-500">{editError}</p>}<div className="flex justify-end gap-2"><Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => setPlanEdit(null)}>Cancel</Button><Button type="submit" size="sm" disabled={busy || !planName.trim() || !planCode.trim() || planLevel < 1 || planMaxDevices < 1}>Save changes</Button></div></form></Modal>
    <ConfirmModal isOpen={archiveTarget !== null} title={archiveTarget?.kind === "product" ? "Archive product" : "Archive plan"} message={archiveTarget?.kind === "product" ? `Archive ${archiveTarget.product.name}? Historical licenses will be retained.` : archiveTarget ? `Archive ${archiveTarget.plan.name}? Historical licenses will be retained.` : ""} confirmLabel="Confirm archive" busy={busy} error={archiveError} onConfirm={() => void archive()} onClose={() => !busy && setArchiveTarget(null)} />
  </section>;
}
