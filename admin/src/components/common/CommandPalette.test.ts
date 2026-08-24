import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const source = readFileSync(
  resolve(process.cwd(), "src/components/common/CommandPalette.tsx"),
  "utf8"
);

describe("CommandPalette", () => {
  it("binds the Ctrl/⌘+K shortcut", () => {
    expect(source).toContain('event.key.toLowerCase() === "k"');
    expect(source).toContain("metaKey || event.ctrlKey");
  });

  it("can be opened through the shared DOM event used by the header", () => {
    expect(source).toContain(
      'window.addEventListener(COMMAND_PALETTE_EVENT, onOpenEvent)'
    );
  });

  it("filters navigation entries by admin permissions", () => {
    expect(source).toContain("isSidebarItemVisible(item, hasPermission)");
  });

  it("supports keyboard navigation and dismissal", () => {
    expect(source).toContain('"ArrowDown"');
    expect(source).toContain('"ArrowUp"');
    expect(source).toContain('"Escape"');
    expect(source).toContain('"Enter"');
  });
});
