import { describe, expect, it } from "vitest";
import nextConfig from "../next.config";

describe("security headers", () => {
  it("sets a restrictive browser policy and hides framework identity", async () => {
    expect(nextConfig.poweredByHeader).toBe(false);
    const entries = await nextConfig.headers?.();
    const headers = new Map(entries?.[0]?.headers.map((item) => [item.key, item.value]));
    expect(headers.get("Content-Security-Policy")).toContain("default-src 'self'");
    expect(headers.get("Content-Security-Policy")).toContain("frame-ancestors 'none'");
    expect(headers.get("Strict-Transport-Security")).toContain("max-age=");
    expect(headers.get("Permissions-Policy")).toContain("camera=()");
  });
});
