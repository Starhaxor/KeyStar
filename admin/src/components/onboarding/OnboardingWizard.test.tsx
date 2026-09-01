import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AdminIdentityProvider } from "@/context/AdminIdentityContext";
import { ApplicationProvider, useApplication } from "@/context/ApplicationContext";
import { api, type OnboardingProgress } from "@/lib/api";
import type { Application } from "@/lib/types";
import OnboardingWizard from "./OnboardingWizard";
import { deriveOnboardingStep } from "./onboardingState";

const navigation = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => navigation }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

const application: Application = {
  id: "app-1",
  organization_id: "org-1",
  name: "Desktop client",
  slug: "desktop-client",
  status: "active",
  environment_mode: "separate",
};

const secondApplication: Application = {
  ...application,
  id: "app-2",
  name: "Second app",
  slug: "second-app",
};

const credentialProgress: OnboardingProgress = {
  ok: true,
  application,
  credential_count: 0,
  product_count: 0,
  plan_count: 0,
  license_count: 0,
};

const catalogProgress: OnboardingProgress = {
  ...credentialProgress,
  credential_count: 1,
  credential_environment: "test",
};

function RefreshApplicationContext() {
  const { refresh } = useApplication();
  return <button type="button" onClick={() => void refresh()}>Refresh application context</button>;
}

function renderWizard(extra?: React.ReactNode) {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => {
    root?.render(
      <AdminIdentityProvider>
        <ApplicationProvider>
          <OnboardingWizard />
          {extra}
        </ApplicationProvider>
      </AdminIdentityProvider>
    );
  });
}

async function flushEffects() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

function button(name: string) {
  return Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (element) => element.textContent?.trim() === name
  );
}

function setControlValue(control: HTMLInputElement | HTMLSelectElement, value: string) {
  const prototype = control instanceof HTMLSelectElement
    ? HTMLSelectElement.prototype
    : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(prototype, "value")?.set;
  act(() => {
    setter?.call(control, value);
    control.dispatchEvent(new Event("change", { bubbles: true }));
    control.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

beforeEach(() => {
  navigation.replace.mockReset();
  vi.spyOn(api, "me").mockResolvedValue({
    ok: true,
    id: "admin-1",
    email: "owner@example.com",
    status: "active",
    role: "owner",
    mfa_enrolled: true,
    permissions: [
      "applications.read",
      "applications.write",
      "credentials.read",
      "credentials.write",
      "catalog.read",
      "catalog.write",
      "licenses.read",
      "licenses.write",
    ],
  });
  vi.spyOn(api, "applications").mockResolvedValue({ ok: true, items: [application] });
  vi.spyOn(api, "organizations").mockResolvedValue({ ok: true, items: [] });
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
});

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  document.body.style.overflow = "";
  document.cookie = "keystar_application_id=; Max-Age=0; Path=/";
  container = undefined;
  root = undefined;
  vi.restoreAllMocks();
});

describe("deriveOnboardingStep", () => {
  it("derives the next incomplete step only from the persisted progress snapshot", () => {
    expect(deriveOnboardingStep({ ...credentialProgress, application: null })).toBe("application");
    expect(deriveOnboardingStep(credentialProgress)).toBe("credential");
    expect(deriveOnboardingStep(catalogProgress)).toBe("catalog");
    expect(deriveOnboardingStep({ ...catalogProgress, product_count: 1, plan_count: 1 })).toBe("license");
    expect(deriveOnboardingStep({ ...catalogProgress, product_count: 1, plan_count: 1, license_count: 1 })).toBe("complete");
  });
});

describe("OnboardingWizard", () => {
  it("stops rendering and returns to overview when onboarding is complete", async () => {
    vi.spyOn(api, "onboardingProgress").mockResolvedValue({
      ...catalogProgress,
      product_count: 1,
      plan_count: 1,
      license_count: 1,
    });

    renderWizard();
    await flushEffects();

    expect(navigation.replace).toHaveBeenCalledWith("/");
    expect(document.body.textContent).not.toContain("Application setup is complete");
  });

  it("creates the first organization inside the application dialog and continues with it selected", async () => {
    vi.spyOn(api, "onboardingProgress").mockResolvedValue(credentialProgress);
    const createdOrganization = {
      id: "org-new",
      name: "StarLoader",
      slug: "starloader",
      status: "active",
      created_at: "2026-08-29T00:00:00Z",
      updated_at: "2026-08-29T00:00:00Z",
    };
    const createOrganization = vi.spyOn(api, "createOrganization").mockResolvedValue({
      ok: true,
      organization: createdOrganization,
    });
    renderWizard();
    await flushEffects();
    act(() => button("Create application")?.click());

    let dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    const organizationName = dialog?.querySelector<HTMLInputElement>("#onboarding-inline-organization-name");
    expect(organizationName).not.toBeNull();
    expect(dialog?.querySelector("#onboarding-application-organization")).toBeNull();
    expect(dialog?.textContent).toContain("Create your first organization here");

    if (organizationName) setControlValue(organizationName, createdOrganization.name);
    const createOrganizationButton = Array.from(dialog?.querySelectorAll<HTMLButtonElement>("button") ?? [])
      .find((element) => element.textContent?.trim() === "Create organization");
    await act(async () => createOrganizationButton?.click());
    await flushEffects();

    dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    const organization = dialog?.querySelector<HTMLSelectElement>("#onboarding-application-organization");
    expect(createOrganization).toHaveBeenCalledWith("StarLoader");
    expect(organization?.value).toBe("org-new");

    const applicationName = dialog?.querySelector<HTMLInputElement>("#onboarding-application-name");
    const applicationSlug = dialog?.querySelector<HTMLInputElement>("#onboarding-application-slug");
    if (applicationName) setControlValue(applicationName, "StarLoader Desktop");
    if (applicationSlug) setControlValue(applicationSlug, "starloader-desktop");
    const createApplicationButton = Array.from(dialog?.querySelectorAll<HTMLButtonElement>("button") ?? [])
      .find((element) => element.textContent?.trim() === "Create application");
    expect(createApplicationButton?.disabled).toBe(false);
  });

  it("waits for application initialization before loading progress", async () => {
    document.cookie = "keystar_application_id=app-1; Path=/";
    let resolveApplications!: (value: { ok: boolean; items: Application[] }) => void;
    vi.mocked(api.applications).mockReturnValue(new Promise((resolve) => {
      resolveApplications = resolve;
    }));
    const loadProgress = vi.spyOn(api, "onboardingProgress").mockResolvedValue({
      ...credentialProgress,
      application: secondApplication,
    });

    renderWizard();
    await flushEffects();
    expect(loadProgress).not.toHaveBeenCalled();

    resolveApplications({ ok: true, items: [secondApplication] });
    await flushEffects();
    expect(loadProgress).toHaveBeenCalledWith("app-2");
  });

  it("refetches progress when application context selects a different application", async () => {
    document.cookie = "keystar_application_id=app-1; Path=/";
    vi.mocked(api.applications)
      .mockResolvedValueOnce({ ok: true, items: [application] })
      .mockResolvedValueOnce({ ok: true, items: [secondApplication] });
    const loadProgress = vi.spyOn(api, "onboardingProgress").mockImplementation(async (applicationID: string) => ({
      ...catalogProgress,
      application: applicationID === application.id ? application : secondApplication,
    }));

    renderWizard(<RefreshApplicationContext />);
    await flushEffects();
    expect(loadProgress).toHaveBeenCalledWith("app-1");

    await act(async () => button("Refresh application context")?.click());
    await flushEffects();

    expect(loadProgress).toHaveBeenCalledWith("app-2");
    expect(document.querySelector<HTMLSelectElement>("#onboarding-application")?.value).toBe("app-2");
  });

  it("does not expose mutation controls for progress from a different application", async () => {
    vi.spyOn(api, "onboardingProgress").mockResolvedValue({
      ...credentialProgress,
      application: secondApplication,
    });

    renderWizard();
    await flushEffects();

    expect(document.querySelector<HTMLSelectElement>("#onboarding-application")?.value).toBe("app-1");
    expect(button("Create credential")).toBeUndefined();
    expect(document.body.textContent).toContain("Unable to load onboarding progress. Try again.");
  });

  it("reloads server progress on re-entry instead of restoring browser wizard state", async () => {
    const loadProgress = vi.spyOn(api, "onboardingProgress")
      .mockResolvedValueOnce(credentialProgress)
      .mockResolvedValueOnce(catalogProgress);
    const localStorageWrite = vi.spyOn(Storage.prototype, "setItem");

    renderWizard();
    await flushEffects();
    expect(document.body.textContent).toContain("Create a publishable credential");

    act(() => root?.unmount());
    container?.remove();
    container = undefined;
    root = undefined;

    renderWizard();
    await flushEffects();
    expect(document.body.textContent).toContain("Create a product and plan");
    expect(loadProgress).toHaveBeenCalledTimes(2);
    expect(localStorageWrite).not.toHaveBeenCalled();
  });

  it("creates a publishable key for the chosen environment and keeps its secret in a dismissible one-time dialog", async () => {
    vi.spyOn(api, "onboardingProgress")
      .mockResolvedValueOnce(credentialProgress)
      .mockResolvedValueOnce(catalogProgress);
    const createCredential = vi.spyOn(api, "createCredential").mockResolvedValue({
      ok: true,
      credential: {
        id: "credential-1",
        name: "Desktop SDK",
        environment: "test",
        type: "publishable",
        scopes: ["auth.login"],
        key_prefix: "ks_pk_test_1234",
        status: "active",
        last_used_at: null,
        expires_at: null,
        created_at: "2026-08-24T00:00:00Z",
      },
      key: "ks_pk_test_shown-once",
    });

    renderWizard();
    await flushEffects();

    const environment = document.querySelector<HTMLSelectElement>("#onboarding-environment");
    const name = document.querySelector<HTMLInputElement>("#onboarding-credential-name");
    expect(environment?.getAttribute("aria-describedby")).toContain("onboarding-environment-description");
    if (environment) setControlValue(environment, "test");
    if (name) setControlValue(name, "Desktop SDK");
    await act(async () => button("Create credential")?.click());

    expect(createCredential).toHaveBeenCalledWith({
      name: "Desktop SDK",
      environment: "test",
      type: "publishable",
      scopes: ["auth.login", "device.verify", "auth.refresh", "auth.logout"],
    });
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog?.textContent).toContain("ks_pk_test_shown-once");

    await act(async () => button("Copy credential")?.click());
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("ks_pk_test_shown-once");
    expect(button("Copied")).toBeDefined();

    await act(async () => button("Done")?.click());
    expect(document.body.textContent).not.toContain("ks_pk_test_shown-once");
  });

  it("keeps a newly issued one-time license visible until it is acknowledged before navigating", async () => {
    const licenseProgress: OnboardingProgress = {
      ...catalogProgress,
      product_count: 1,
      plan_count: 1,
      product: { id: "product-1", name: "Desktop" },
      plan: { id: "plan-1", name: "Test" },
    };
    vi.spyOn(api, "onboardingProgress")
      .mockResolvedValueOnce(licenseProgress)
      .mockResolvedValueOnce({ ...licenseProgress, license_count: 1 });
    vi.spyOn(api, "createLicense").mockResolvedValue({
      ok: true,
      license: {
        id: "license-1",
        user_id: "user-1",
        user_email: "tester@example.com",
        product: "Desktop",
        status: "active",
        level: 1,
        max_devices: 1,
        notes: "",
        expires_at: "2026-09-08T00:00:00Z",
        created_at: "2026-09-01T00:00:00Z",
      },
      key: "license_shown_once",
    });

    renderWizard();
    await flushEffects();

    const email = document.querySelector<HTMLInputElement>("#onboarding-license-email");
    if (email) setControlValue(email, "tester@example.com");
    await act(async () => button("Issue test license")?.click());

    expect(navigation.replace).not.toHaveBeenCalled();
    expect(document.querySelector<HTMLElement>('[role="dialog"]')?.textContent).toContain("license_shown_once");

    await act(async () => button("Done")?.click());
    expect(navigation.replace).toHaveBeenCalledWith("/");
    expect(document.body.textContent).not.toContain("license_shown_once");
  });

  it("reloads persisted catalog state after plan creation fails and retries without recreating the product", async () => {
    const productOnlyProgress: OnboardingProgress = {
      ...catalogProgress,
      product_count: 1,
      product: { id: "product-1", name: "Desktop" },
    };
    const loadProgress = vi.spyOn(api, "onboardingProgress")
      .mockResolvedValueOnce(catalogProgress)
      .mockResolvedValue(productOnlyProgress);
    const createProduct = vi.spyOn(api, "createProduct").mockResolvedValue({
      ok: true,
      product: {
        id: "product-1",
        application_id: application.id,
        name: "Desktop",
        slug: "desktop",
        status: "active",
        created_at: "2026-08-24T00:00:00Z",
        updated_at: "2026-08-24T00:00:00Z",
      },
    });
    const createPlan = vi.spyOn(api, "createPlan")
      .mockRejectedValueOnce(new Error("plan insert failed"))
      .mockResolvedValue({
        ok: true,
        plan: {
          id: "plan-1",
          product_id: "product-1",
          name: "Test plan",
          code: "test",
          level: 1,
          max_devices: 1,
          default_duration_seconds: null,
          status: "active",
          created_at: "2026-08-24T00:00:00Z",
          updated_at: "2026-08-24T00:00:00Z",
        },
      });

    renderWizard();
    await flushEffects();
    const productName = document.querySelector<HTMLInputElement>("#onboarding-product-name");
    if (productName) setControlValue(productName, "Desktop");

    await act(async () => button("Create product and plan")?.click());
    await flushEffects();

    expect(loadProgress).toHaveBeenCalledTimes(2);
    expect(document.querySelector("#onboarding-product-name")).toBeNull();
    expect(document.body.textContent).toContain("Add the first active plan to Desktop.");

    await act(async () => button("Create product and plan")?.click());
    expect(createProduct).toHaveBeenCalledOnce();
    expect(createPlan).toHaveBeenCalledTimes(2);
    expect(createPlan).toHaveBeenLastCalledWith("product-1", {
      name: "Test plan",
      code: "test",
      level: 1,
      max_devices: 1,
    });
  });
});
