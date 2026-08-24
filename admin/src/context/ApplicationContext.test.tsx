import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api, applicationCookieName } from "@/lib/api";
import { ApplicationProvider, useApplication } from "./ApplicationContext";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

function SelectedApplication() {
  const { selectedApplicationID } = useApplication();
  return <output>{selectedApplicationID ?? "none"}</output>;
}

function render() {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root?.render(<ApplicationProvider><SelectedApplication /></ApplicationProvider>));
}

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  document.cookie = `${applicationCookieName}=; Max-Age=0; Path=/`;
  container = undefined;
  root = undefined;
  vi.restoreAllMocks();
});

describe("ApplicationProvider", () => {
  it("replaces an inactive selected application with an active application during refresh", async () => {
    document.cookie = `${applicationCookieName}=app-2; Path=/`;
    vi.spyOn(api, "applications").mockResolvedValue({ ok: true, items: [
      { id: "app-1", organization_id: "org-1", name: "Active", slug: "active", status: "active", environment_mode: "live" },
      { id: "app-2", organization_id: "org-1", name: "Paused", slug: "paused", status: "maintenance", environment_mode: "live" },
    ] });

    await act(async () => render());

    expect(document.querySelector("output")?.textContent).toBe("app-1");
    expect(document.cookie).toContain(`${applicationCookieName}=app-1`);
  });

  it("clears the selected application cookie when refresh has no active applications", async () => {
    document.cookie = `${applicationCookieName}=app-2; Path=/`;
    vi.spyOn(api, "applications").mockResolvedValue({ ok: true, items: [
      { id: "app-2", organization_id: "org-1", name: "Paused", slug: "paused", status: "disabled", environment_mode: "live" },
    ] });

    await act(async () => render());

    expect(document.querySelector("output")?.textContent).toBe("none");
    expect(document.cookie).not.toContain(`${applicationCookieName}=`);
  });
});
