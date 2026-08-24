import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/lib/api";
import UsersPage from "./page";

const state = vi.hoisted(() => ({ permissions: [] as string[] }));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("@/context/AdminIdentityContext", () => ({
  useAdminIdentity: () => ({
    hasPermission: (permission: string) => state.permissions.includes(permission),
  }),
}));

vi.mock("@/context/ToastContext", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

function render() {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root?.render(<UsersPage />));
}

async function flushEffects() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  container = undefined;
  root = undefined;
  state.permissions = [];
  vi.restoreAllMocks();
});

describe("UsersPage actions", () => {
  it("does not expose promotion or status mutations to a read-only operator", async () => {
    vi.spyOn(api, "users").mockResolvedValue({
      ok: true,
      items: [{
        id: "user-123",
        email: "reader@example.com",
        status: "active",
        license_count: 1,
        device_count: 1,
        active_session_count: 1,
        last_login_at: null,
        created_at: "2026-08-24T12:00:00Z",
      }],
      total: 1,
      page: 1,
      page_size: 20,
    });

    render();
    await flushEffects();

    const rowActions = document.querySelector<HTMLButtonElement>('button[aria-label="Row actions"]');
    await act(async () => rowActions?.click());

    expect(document.body.textContent).toContain("Detail");
    expect(document.body.textContent).not.toContain("Make administrator");
    expect(document.body.textContent).not.toContain("Disable user");
    expect(document.body.textContent).not.toContain("Enable user");
  });
});
