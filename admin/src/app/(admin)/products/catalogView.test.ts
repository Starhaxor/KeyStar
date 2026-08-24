import { describe, expect, it } from "vitest";
import type { Plan, Product } from "@/lib/types";
import { catalogView } from "./catalogView";

const activeProduct: Product = { id: "product-active", application_id: "app-1", name: "Active product", slug: "active", status: "active", created_at: "", updated_at: "" };
const archivedProduct: Product = { id: "product-archived", application_id: "app-1", name: "Archived product", slug: "archived", status: "archived", created_at: "", updated_at: "" };
const activePlan: Plan = { id: "plan-active", product_id: "product-archived", name: "Active plan", code: "active", level: 1, max_devices: 1, default_duration_seconds: null, status: "active", created_at: "", updated_at: "" };
const archivedPlan: Plan = { id: "plan-archived", product_id: "product-active", name: "Archived plan", code: "archived", level: 1, max_devices: 1, default_duration_seconds: null, status: "archived", created_at: "", updated_at: "" };

describe("catalogView", () => {
  it("keeps archived plans attached to active products in historical mode", () => {
    expect(catalogView([activeProduct], { [activeProduct.id]: [archivedPlan] }, true)).toEqual([
      { product: activeProduct, plans: [archivedPlan] },
    ]);
  });

  it("keeps active plans attached to archived products in historical mode", () => {
    expect(catalogView([archivedProduct], { [archivedProduct.id]: [activePlan] }, true)).toEqual([
      { product: archivedProduct, plans: [activePlan] },
    ]);
  });
});
