import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import UserDropdown from "./UserDropdown";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

// SVG imports need the Next.js loader, which is not present in jsdom.
vi.mock("@/icons", () => ({
  ChevronDownIcon: () => <svg aria-hidden="true" />,
  LockIcon: () => <svg aria-hidden="true" />,
  UserCircleIcon: () => <svg aria-hidden="true" />,
}));

let root: Root | undefined;
let container: HTMLDivElement | undefined;

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  vi.unstubAllGlobals();
});

async function clickButton(label: string) {
  const button = Array.from(document.querySelectorAll("button")).find(
    (item) => item.getAttribute("aria-label") === label || item.textContent?.trim() === label,
  );
  if (!button) throw new Error(`Missing button: ${label}`);
  await act(async () => button.click());
}

describe("admin sign out", () => {
  it.each(["server", "network"])("reports a %s logout failure while leaving the dialog available for retry", async (failure) => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      if (String(input) === "/v1/admin/me") {
        return new Response(JSON.stringify({ ok: true, email: "admin@example.test", role: "root", mfa_enrolled: true, permissions: [] }));
      }
      if (failure === "network") throw new TypeError("Failed to fetch");
      return new Response(JSON.stringify({ code: "SERVER_ERROR", message: "service unavailable" }), { status: 503 });
    });
    vi.stubGlobal("fetch", fetchMock);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    await act(async () => root?.render(<UserDropdown />));
    await clickButton("Account menu");
    await clickButton("Sign out");
    await clickButton("Sign out");

    expect(document.querySelector('[role="alert"]')?.textContent ?? "").toMatch(/still be signed in/i);
    const retry = Array.from(document.querySelectorAll("button")).find((button) => button.textContent?.trim() === "Sign out");
    expect(retry?.disabled).toBe(false);

    await clickButton("Sign out");
    expect(fetchMock.mock.calls.filter(([path]) => path === "/v1/admin/auth/logout")).toHaveLength(2);
  });
});
