import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const stylesheet = readFileSync(resolve(process.cwd(), "src/app/globals.css"), "utf8");

describe("dark console theme", () => {
  it("uses navy overrides for dark surfaces without changing shared text tokens", () => {
    expect(stylesheet).toMatch(/\.dark\s*\{[\s\S]*--console-canvas:\s*#0b1220;/);
    expect(stylesheet).toContain(".dark .dark\\:bg-gray-900");
    expect(stylesheet).toContain(".dark .dark\\:bg-gray-dark");
    expect(stylesheet).toContain(".dark .dark\\:bg-gray-800");
    expect(stylesheet).not.toMatch(/\.dark\s*\{[\s\S]*--color-gray-800:/);
  });
});
