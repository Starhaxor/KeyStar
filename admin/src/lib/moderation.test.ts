import { describe, expect, it } from "vitest";
import { deviceBanExpiry, moderationStatus } from "./moderation";

describe("moderationStatus", () => {
  it("uses the URL status without a second default state", () => {
    expect(moderationStatus(new URLSearchParams("status=lifted"), "active")).toBe("lifted");
  });

  it("falls back when the URL has no status", () => {
    expect(moderationStatus(new URLSearchParams(), "active")).toBe("active");
  });
});

describe("deviceBanExpiry", () => {
  it("creates an ISO expiry from a selected duration", () => {
    expect(deviceBanExpiry("2026-08-21T10:00:00.000Z", 72)).toBe("2026-08-24T10:00:00.000Z");
  });

  it("keeps a permanent ban without an expiry", () => {
    expect(deviceBanExpiry("2026-08-21T10:00:00.000Z", null)).toBeUndefined();
  });
});
