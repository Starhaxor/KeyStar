import type { Application } from "@/lib/types";

export type OnboardingStep = "application" | "credential" | "catalog" | "license" | "complete";

export type PersistedOnboardingState = {
  application: Application | null;
  credential_count: number;
  product_count: number;
  plan_count: number;
  license_count: number;
};

export function deriveOnboardingStep(progress: PersistedOnboardingState): OnboardingStep {
  if (!progress.application) return "application";
  if (progress.credential_count < 1) return "credential";
  if (progress.product_count < 1 || progress.plan_count < 1) return "catalog";
  if (progress.license_count < 1) return "license";
  return "complete";
}
