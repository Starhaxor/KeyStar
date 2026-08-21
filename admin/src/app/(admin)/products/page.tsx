"use client";
import { EmptyNote, ErrorNote, LoadingNote, PageTitle } from "@/components/console/ConsoleSection";
import { api } from "@/lib/api";
import type { Plan, Product } from "@/lib/types";
import React, { useCallback, useEffect, useState } from "react";

export default function ProductsPage() {
  const [products, setProducts] = useState<Product[]>([]);
  const [plans, setPlans] = useState<Record<string, Plan[]>>({});
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const load = useCallback(async () => {
    setLoading(true); setError(null);
    try {
      const response = await api.products();
      setProducts(response.items);
      const result: Record<string, Plan[]> = {};
      await Promise.all(response.items.map(async (product) => { result[product.id] = (await api.plans(product.id)).items; }));
      setPlans(result);
    } catch (err) { setError(err instanceof Error ? err.message : "Catalog could not be loaded"); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);
  return <div><PageTitle title="Products & Plans" description="Catalog products and their license plans." />
    <div className="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-white/[0.03]">
      {loading ? <LoadingNote /> : error ? <ErrorNote message={error} /> : products.length === 0 ? <EmptyNote message="No catalog products yet." /> : products.map((product) => <section key={product.id} className="border-b border-gray-200 p-5 dark:border-gray-800"><h2 className="font-semibold">{product.name}</h2><p className="text-xs text-gray-500">{product.slug}</p><div className="mt-3 grid gap-3 md:grid-cols-3">{(plans[product.id] || []).map((plan) => <div key={plan.id} className="rounded-xl border border-gray-200 p-3 text-sm dark:border-gray-700"><strong>{plan.name}</strong><p className="mt-1 text-gray-500">{plan.code} · Level {plan.level} · {plan.max_devices} devices</p></div>)}</div></section>)}
    </div></div>;
}
