import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Application, Organization } from "@/lib/types";
import ApplicationsView from "./ApplicationsView";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

const application: Application = {
  id: "01a04caa-baa0-72ec-9b69-b4ba548bb3e5",
  organization_id: "org-1",
  name: "StarLoader",
  slug: "starloader",
  status: "active",
  environment_mode: "separate",
  auth_profile: "legacy",
};

const organization: Organization = {
  id: "org-1",
  name: "StarLoader",
  slug: "starloader",
  status: "active",
  created_at: "2026-08-29T00:00:00Z",
  updated_at: "2026-08-29T00:00:00Z",
};

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  container = undefined;
  root = undefined;
  vi.restoreAllMocks();
});

describe("ApplicationsView", () => {
  it("shows the full application ID and copies it for SDK configuration", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root?.render(
      <ApplicationsView applications={[application]} organizations={[organization]} loading={false} canWrite={false} onRefresh={async () => undefined} />
    ));

    expect(document.body.textContent).toContain(application.id);
    const copy = document.querySelector<HTMLButtonElement>('button[aria-label="Copy StarLoader application ID"]');
    expect(copy).not.toBeNull();
    await act(async () => copy?.click());
    expect(writeText).toHaveBeenCalledWith(application.id);
    expect(copy?.textContent).toBe("Copied");
  });
});
