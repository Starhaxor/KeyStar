"use client";

import { EmptyNote, ErrorNote, LoadingNote, PageTitle, TableCard } from "@/components/console/ConsoleSection";
import Button from "@/components/ui/button/Button";
import { Modal } from "@/components/ui/modal";
import { useAdminIdentity } from "@/context/AdminIdentityContext";
import { api } from "@/lib/api";
import { reportClientError } from "@/lib/clientError";
import { formatDuration } from "@/lib/time";
import type { Plan, Product } from "@/lib/types";
import React, { useCallback, useEffect, useState } from "react";

const inputClass = "h-10 w-full rounded-lg border border-gray-300 bg-transparent px-3 text-sm text-gray-800 dark:border-gray-700 dark:text-white/90";

export default function ProductsPage() {
  const { hasPermission } = useAdminIdentity();
  const [products, setProducts] = useState<Product[]>([]);
  const [plans, setPlans] = useState<Record<string, Plan[]>>({});
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [productOpen, setProductOpen] = useState(false);
  const [planProduct, setPlanProduct] = useState<Product | null>(null);
  const [productName, setProductName] = useState("");
  const [slug, setSlug] = useState("");
  const [planName, setPlanName] = useState("");
  const [code, setCode] = useState("");
  const [level, setLevel] = useState(1);
  const [maxDevices, setMaxDevices] = useState(1);
  const canWrite = hasPermission("catalog.write");

  const load = useCallback(async () => {
    setLoading(true); setError(null);
    try {
      const response = await api.products();
      setProducts(response.items);
      const entries = await Promise.all(response.items.map(async (product) => [product.id, (await api.plans(product.id)).items] as const));
      setPlans(Object.fromEntries(entries));
    } catch (err) { setError(reportClientError(err, "Unable to load the catalog. Try again.")); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  async function createProduct() {
    setBusy(true); setError(null);
    try { await api.createProduct(productName.trim(), slug.trim()); setProductName(""); setSlug(""); setProductOpen(false); await load(); }
    catch (err) { setError(reportClientError(err, "Unable to create the product. Try again.")); }
    finally { setBusy(false); }
  }
  async function createPlan() {
    if (!planProduct) return;
    setBusy(true); setError(null);
    try { await api.createPlan(planProduct.id, { name: planName.trim(), code: code.trim(), level, max_devices: maxDevices }); setPlanName(""); setCode(""); setLevel(1); setMaxDevices(1); setPlanProduct(null); await load(); }
    catch (err) { setError(reportClientError(err, "Unable to create the plan. Try again.")); }
    finally { setBusy(false); }
  }

  return <>
    <PageTitle title="Products & Plans" description="Catalog products and the license plans available for each one." actions={canWrite ? <Button size="sm" onClick={() => setProductOpen(true)}>Add product</Button> : undefined} />
    {error && <div className="mb-4"><ErrorNote message={error} onRetry={load} /></div>}
    <TableCard>
      {loading ? <LoadingNote /> : products.length === 0 ? <EmptyNote message="No catalog products yet. Create a product, then add at least one plan." /> : products.map((product) => <section key={product.id} className="border-b border-gray-200 p-5 last:border-0 dark:border-gray-800"><div className="flex flex-wrap items-start justify-between gap-3"><div><h2 className="font-semibold">{product.name}</h2><p className="text-xs text-gray-500">{product.slug}</p></div>{canWrite && <Button size="sm" variant="outline" onClick={() => setPlanProduct(product)}>Add plan</Button>}</div><div className="mt-4 grid gap-3 md:grid-cols-3">{(plans[product.id] ?? []).map((plan) => <div key={plan.id} className="rounded-xl border border-gray-200 p-3 text-sm dark:border-gray-700"><strong>{plan.name}</strong><p className="mt-1 text-gray-500">{plan.code} · Level {plan.level} · {plan.max_devices} devices</p><p className="mt-1 text-gray-500">Default duration: {formatDuration(plan.default_duration_seconds) ?? "not set"}</p><p className="mt-2 text-xs text-gray-500">{plan.status}</p></div>)}{(plans[product.id] ?? []).length === 0 && <p className="text-sm text-gray-500">No plans yet.</p>}</div></section>)}
    </TableCard>
    <Modal isOpen={productOpen} onClose={() => !busy && setProductOpen(false)} className="max-w-md p-6"><h2 className="text-lg font-semibold">Create product</h2><div className="mt-4 space-y-4"><label className="block text-sm">Name<input className={inputClass} value={productName} onChange={(event) => setProductName(event.target.value)} /></label><label className="block text-sm">Slug <span className="text-gray-400">(optional)</span><input className={inputClass} value={slug} onChange={(event) => setSlug(event.target.value)} placeholder="desktop-client" /></label><div className="flex justify-end gap-2"><Button size="sm" variant="outline" disabled={busy} onClick={() => setProductOpen(false)}>Cancel</Button><Button size="sm" disabled={busy || !productName.trim()} onClick={createProduct}>Create</Button></div></div></Modal>
    <Modal isOpen={planProduct !== null} onClose={() => !busy && setPlanProduct(null)} className="max-w-md p-6"><h2 className="text-lg font-semibold">Create plan</h2><p className="mt-1 text-sm text-gray-500">{planProduct?.name}</p><div className="mt-4 space-y-4"><label className="block text-sm">Name<input className={inputClass} value={planName} onChange={(event) => setPlanName(event.target.value)} /></label><label className="block text-sm">Code<input className={inputClass} value={code} onChange={(event) => setCode(event.target.value)} placeholder="pro-monthly" /></label><div className="grid grid-cols-2 gap-3"><label className="block text-sm">Level<input className={inputClass} type="number" min="1" value={level} onChange={(event) => setLevel(Number(event.target.value))} /></label><label className="block text-sm">Max devices<input className={inputClass} type="number" min="1" value={maxDevices} onChange={(event) => setMaxDevices(Number(event.target.value))} /></label></div><div className="flex justify-end gap-2"><Button size="sm" variant="outline" disabled={busy} onClick={() => setPlanProduct(null)}>Cancel</Button><Button size="sm" disabled={busy || !planName.trim() || !code.trim() || maxDevices < 1} onClick={createPlan}>Create</Button></div></div></Modal>
  </>;
}
