import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/lib/api";
import type { Application } from "@/lib/types";
import ApplicationLifecycleControls from "./ApplicationLifecycleControls";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

const application: Application = {
  id: "app-1",
  organization_id: "org-1",
  name: "Desktop client",
  slug: "desktop-client",
  status: "active",
  environment_mode: "live",
};

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

describe("ApplicationLifecycleControls", () => {
  it("confirms a named status change before it requests the transition and refreshes context", async () => {
    const transition = vi.spyOn(api, "transitionApplication").mockResolvedValue({ ok: true, application: { ...application, status: "maintenance" } });
    const refresh = vi.fn().mockResolvedValue(undefined);
    render(<ApplicationLifecycleControls application={application} canWrite onRefresh={refresh} />);

    await act(async () => button("Set maintenance")?.click());
    expect(transition).not.toHaveBeenCalled();
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    const heading = dialog && document.getElementById(dialog.getAttribute("aria-labelledby") ?? "");
    expect(heading?.textContent).toBe("Change application status");

    await act(async () => button("Confirm status change")?.click());
    expect(transition).toHaveBeenCalledWith("app-1", "maintenance");
    expect(refresh).toHaveBeenCalledOnce();
  });

  it("shows safe copy when a status change is rejected", async () => {
    vi.spyOn(api, "transitionApplication").mockRejectedValue(new Error("active licenses: database detail"));
    render(<ApplicationLifecycleControls application={application} canWrite onRefresh={async () => undefined} />);

    await act(async () => button("Disable application")?.click());
    await act(async () => button("Confirm status change")?.click());

    expect(document.body.textContent).toContain("Unable to change application status. Try again.");
    expect(document.body.textContent).not.toContain("database detail");
  });

  it("does not expose application lifecycle controls without applications.write", () => {
    render(<ApplicationLifecycleControls application={application} canWrite={false} onRefresh={async () => undefined} />);

    expect(button("Edit application")).toBeUndefined();
    expect(button("Set maintenance")).toBeUndefined();
  });
});
