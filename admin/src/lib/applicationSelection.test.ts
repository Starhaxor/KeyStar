import { describe, expect, it } from "vitest";

import { applicationSelectorOptions, initialOrganizationSelection, nextSelectedApplicationID } from "./applicationSelection";

describe("initial organization selection", () => {
  it("selects the sole available organization", () => {
    expect(initialOrganizationSelection([{ id: "org-1", name: "StarLoader" }])).toBe("org-1");
  });

  it("requires an explicit choice when multiple organizations exist", () => {
    expect(initialOrganizationSelection([{ id: "org-1", name: "One" }, { id: "org-2", name: "Two" }])).toBe("");
  });

  it("uses each application name and environment in the sidebar selector", () => {
    expect(applicationSelectorOptions([
      { id: "app-1", name: "StarLoader", environment_mode: "live" },
      { id: "app-2", name: "Sandbox", environment_mode: "test" },
    ])).toEqual([
      { value: "app-1", label: "StarLoader · Live" },
      { value: "app-2", label: "Sandbox · Test" },
    ]);
  });

  it("keeps the current selection when application choices refresh", () => {
    expect(nextSelectedApplicationID("app-2", [{ id: "app-1" }, { id: "app-2" }])).toBe("app-2");
  });

  it("selects the first available application when there is no valid selection", () => {
    expect(nextSelectedApplicationID(null, [{ id: "app-1" }, { id: "app-2" }])).toBe("app-1");
  });
});
