import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const source = readFileSync(resolve(process.cwd(), "src/context/ThemeContext.tsx"), "utf8");

describe("ThemeProvider hydration", () => {
  it("uses the same theme state during server and first client render", () => {
    expect(source).toContain("useSyncExternalStore");
    expect(source).toContain('function getServerTheme(): Theme { return "system"; }');
  });
});
