import { describe, expect, it } from "vitest";

import { sidebarSections } from "./sidebarNavigation";

describe("sidebar navigation", () => {
  it("groups application resources under one dedicated section", () => {
    const application = sidebarSections.find((section) => section.name === "Application");

    expect(application?.items.map((item) => item.name)).toEqual([
      "Users",
      "Licenses",
      "Devices",
      "Products & Plans",
      "Sessions",
      "Variables",
      "API Credentials",
      "Webhooks",
    ]);
  });

  it("keeps security controls together under administration", () => {
    const administration = sidebarSections.find((section) => section.name === "Administration");
    const security = administration?.items.find((item) => item.name === "Security");

    expect(security?.children?.map((item) => item.label)).toEqual([
      "MFA & settings",
      "Security events",
    ]);
  });
});
