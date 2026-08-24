import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/lib/api";
import type { Plan, Product } from "@/lib/types";
import ProductLifecycleSection from "./ProductLifecycleSection";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

const product: Product = { id: "product-1", application_id: "app-1", name: "Desktop", slug: "desktop", status: "active", created_at: "", updated_at: "" };
const plan: Plan = { id: "plan-1", product_id: "product-1", name: "Pro", code: "pro", level: 2, max_devices: 3, default_duration_seconds: null, status: "active", created_at: "", updated_at: "" };

function render(element: React.ReactNode) {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root?.render(element));
}

function button(name: string) {
  return Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (element) => element.textContent?.trim() === name
  );
}

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  container = undefined;
  root = undefined;
  vi.restoreAllMocks();
});

describe("ProductLifecycleSection", () => {
  it("confirms archive before requesting it and refreshes the catalog", async () => {
    const archive = vi.spyOn(api, "archiveProduct").mockResolvedValue({ ok: true, product: { ...product, status: "archived" } });
    const refresh = vi.fn().mockResolvedValue(undefined);
    render(<ProductLifecycleSection product={product} plans={[plan]} canWrite onRefresh={refresh} onAddPlan={() => undefined} />);

    expect(button("Archive product")?.disabled).toBe(false);
    await act(async () => button("Archive product")?.click());
    expect(archive).not.toHaveBeenCalled();
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    const heading = dialog && document.getElementById(dialog.getAttribute("aria-labelledby") ?? "");
    expect(heading?.textContent).toBe("Archive product");

    await act(async () => button("Confirm archive")?.click());
    expect(archive).toHaveBeenCalledWith("product-1");
    expect(refresh).toHaveBeenCalledOnce();
  });

  it("renders conflict errors safely without exposing server detail", async () => {
    vi.spyOn(api, "archiveProduct").mockRejectedValue(new Error("foreign key conflict: internal table"));
    render(<ProductLifecycleSection product={product} plans={[plan]} canWrite onRefresh={async () => undefined} onAddPlan={() => undefined} />);

    await act(async () => button("Archive product")?.click());
    await act(async () => button("Confirm archive")?.click());

    expect(document.body.textContent).toContain("Unable to archive the product because it is still in use.");
    expect(document.body.textContent).not.toContain("internal table");
  });
});
