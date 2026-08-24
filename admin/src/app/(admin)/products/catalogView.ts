import type { Plan, Product } from "@/lib/types";

export function catalogView(products: Product[], plansByProduct: Record<string, Plan[]>, historical: boolean) {
  return products
    .filter((product) => {
      if (!historical) return product.status !== "archived";
      return product.status === "archived" || (plansByProduct[product.id] ?? []).some((plan) => plan.status === "archived");
    })
    .map((product) => ({
      product,
      plans: historical ? plansByProduct[product.id] ?? [] : (plansByProduct[product.id] ?? []).filter((plan) => plan.status !== "archived"),
    }));
}
