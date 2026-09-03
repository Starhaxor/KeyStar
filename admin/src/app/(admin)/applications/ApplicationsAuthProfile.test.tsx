import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Application, Organization } from "@/lib/types";
import { api } from "@/lib/api";
import ApplicationsView from "./ApplicationsView";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

const organization: Organization = {
  id: "org-1",
  name: "StarLoader",
  slug: "starloader",
  status: "active",
  created_at: "2026-08-29T00:00:00Z",
  updated_at: "2026-08-29T00:00:00Z",
};

function render(applications: Application[]) {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root?.render(
    <ApplicationsView applications={applications} organizations={[organization]} loading={false} canWrite={true} onRefresh={async () => undefined} />
  ));
}

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  container = undefined;
  root = undefined;
  vi.restoreAllMocks();
});

function appWith(profile: Application["auth_profile"]): Application {
  return {
    id: "01a04caa-baa0-72ec-9b69-b4ba548bb3e5",
    organization_id: "org-1",
    name: "StarLoader",
    slug: "starloader",
    status: "active",
    environment_mode: "separate",
    auth_profile: profile,
  };
}

describe("proof-bound application profile", () => {
  it("displays the auth profile and fails closed on unknown values", () => {
    render([appWith("proof_bound")]);
    expect(document.body.textContent).toContain("proof_bound");

    act(() => root?.unmount());
    container?.remove();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    const unknown = { ...appWith("legacy"), auth_profile: "magic" } as unknown as Application;
    act(() => root?.render(
      <ApplicationsView applications={[unknown]} organizations={[organization]} loading={false} canWrite={true} onRefresh={async () => undefined} />
    ));
    expect(document.body.textContent).toContain("legacy");
    expect(document.body.textContent).not.toContain("magic");
  });

  it("sends auth_profile in the update payload with activation warning", async () => {
    vi.spyOn(api, "applicationSigningKeys").mockResolvedValue({ ok: true, items: [{ kid: "ksk_1", algorithm: "Ed25519", status: "active", public_key: "x", created_at: "2026-09-01T00:00:00Z", activated_at: "2026-09-01T00:00:00Z", retire_at: null, revoked_at: null }] });
    const updateSpy = vi.spyOn(api, "updateApplication").mockResolvedValue({ ok: true, application: appWith("proof_bound") });
    render([appWith("legacy")]);

    const edit = Array.from(document.querySelectorAll("button")).find((b) => b.textContent === "Edit application");
    expect(edit).toBeDefined();
    await act(async () => edit?.click());
    const select = document.querySelector('select[name="auth_profile"]') as HTMLSelectElement | null;
    expect(select).not.toBeNull();
    await act(async () => {
      select!.value = "proof_bound";
      select!.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(document.body.textContent).toContain("Refresh tokens and Bearer clients stop working");
    const save = Array.from(document.querySelectorAll("button")).find((b) => b.textContent === "Save changes");
    await act(async () => save?.click());
    expect(updateSpy).toHaveBeenCalledWith(appWith("legacy").id, expect.objectContaining({ auth_profile: "proof_bound" }));
  });

  it("disables proof_bound when no active signing key exists", async () => {
    vi.spyOn(api, "applicationSigningKeys").mockResolvedValue({ ok: true, items: [] });
    vi.spyOn(api, "updateApplication").mockResolvedValue({ ok: true, application: appWith("legacy") });
    render([appWith("legacy")]);
    const edit = Array.from(document.querySelectorAll("button")).find((b) => b.textContent === "Edit application");
    await act(async () => edit?.click());
    await act(async () => undefined);
    const option = document.querySelector('select[name="auth_profile"] option[value="proof_bound"]') as HTMLOptionElement | null;
    expect(option?.disabled).toBe(true);
    expect(document.body.textContent).toContain("No active signing key");
  });
});
