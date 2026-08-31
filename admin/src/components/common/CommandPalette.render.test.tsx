import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import CommandPalette, { COMMAND_PALETTE_EVENT } from "./CommandPalette";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/context/AdminIdentityContext", () => ({ useAdminIdentity: () => ({ hasPermission: () => true }) }));
vi.mock("@/context/ThemeContext", () => ({ useTheme: () => ({ resolvedTheme: "dark", toggleTheme: vi.fn() }) }));
vi.mock("@/layout/sidebarNavigation", () => ({ isSidebarItemVisible: () => true, sidebarSections: [] }));
vi.mock("@/icons", () => ({ SearchIcon: () => <svg aria-hidden="true" /> }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  document.body.style.overflow = "";
  container = undefined;
  root = undefined;
  vi.restoreAllMocks();
});

describe("CommandPalette overlay", () => {
  it("dims the page without blurring content behind the palette", () => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root?.render(<CommandPalette />));
    act(() => window.dispatchEvent(new Event(COMMAND_PALETTE_EVENT)));

    const backdrop = document.querySelector<HTMLButtonElement>('button[aria-label="Close command palette"]');
    expect(backdrop?.className).toContain("bg-gray-950/40");
    expect(backdrop?.className).not.toContain("backdrop-blur");
  });
});
