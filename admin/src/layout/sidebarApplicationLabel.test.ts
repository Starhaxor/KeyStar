import { describe, expect, it } from "vitest";

import { applicationNavigationLabel } from "./sidebarApplicationLabel";

describe("application navigation label", () => {
  it("shows the selected application's name and environment", () => {
    expect(applicationNavigationLabel({ name: "StarLoader", environment_mode: "live" })).toBe("StarLoader · Live");
  });

  it("makes the missing selection explicit", () => {
    expect(applicationNavigationLabel(null)).toBe("No application selected");
  });
});
