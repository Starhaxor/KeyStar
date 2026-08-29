import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SignInForm from "./SignInForm";
import { api } from "@/lib/api";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }),
}));

vi.mock("@/icons", () => ({
  EyeIcon: () => <svg aria-hidden="true" />,
  EyeCloseIcon: () => <svg aria-hidden="true" />,
}));

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    bootstrapStatus: vi.fn(),
    bootstrapRoot: vi.fn(),
    login: vi.fn(),
    completeMfa: vi.fn(),
  },
}));

let container: HTMLDivElement | undefined;
let root: Root | undefined;

function render(element: React.ReactNode) {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root?.render(element));
}

function setInput(name: string, value: string) {
  const input = document.querySelector<HTMLInputElement>(`input[name="${name}"]`);
  if (!input) throw new Error(`input ${name} not found`);
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  act(() => {
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

async function settle() {
  await act(async () => {
    await Promise.resolve();
  });
}

beforeEach(() => {
  vi.mocked(api.bootstrapStatus).mockResolvedValue({ ok: true, setup_required: false });
});

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  container = undefined;
  root = undefined;
});

describe("SignInForm", () => {
  it("exposes password visibility as a named button", async () => {
    render(<SignInForm />);
    await settle();

    const visibilityButton = document.querySelector('button[aria-label="Show password"]');
    expect(visibilityButton).not.toBeNull();

    act(() => visibilityButton?.dispatchEvent(new MouseEvent("click", { bubbles: true })));

    expect(document.querySelector('button[aria-label="Hide password"]')).not.toBeNull();
    expect(document.querySelector('input[name="password"]')?.getAttribute("type")).toBe("text");
  });

  it("shows root registration instead of sign in on a fresh installation", async () => {
    vi.mocked(api.bootstrapStatus).mockResolvedValue({ ok: true, setup_required: true });

    render(<SignInForm />);
    await settle();

    expect(document.body.textContent).toContain("Create root administrator");
    expect(document.body.textContent).toContain("MFA setup is required");
    expect(document.body.textContent).toContain("Bootstrap token");
    expect(document.body.textContent).not.toContain("Enter your admin email and password");
  });

  it("recovers to sign in when root exists but enrollment session creation fails", async () => {
    vi.mocked(api.bootstrapStatus).mockResolvedValue({ ok: true, setup_required: true });
    vi.mocked(api.bootstrapRoot).mockResolvedValue({ ok: true, session_created: false });
    render(<SignInForm />);
    await settle();
    setInput("email", "root@example.com");
    setInput("password", "long enough root password");
    setInput("password-confirmation", "long enough root password");
    setInput("bootstrap-token", "test-bootstrap-token-0123456789abcdef");

    const form = document.querySelector("form");
    await act(async () => {
      form?.dispatchEvent(new SubmitEvent("submit", { bubbles: true, cancelable: true }));
      await Promise.resolve();
    });

    expect(document.body.textContent).toContain("Enter your admin email and password");
    expect(document.body.textContent).toContain("Root account was created. Sign in to continue MFA setup.");
  });

  it("rechecks setup after an ambiguous bootstrap network failure", async () => {
    vi.mocked(api.bootstrapStatus)
      .mockResolvedValueOnce({ ok: true, setup_required: true })
      .mockResolvedValueOnce({ ok: true, setup_required: false });
    vi.mocked(api.bootstrapRoot).mockRejectedValue(new Error("connection lost"));
    render(<SignInForm />);
    await settle();
    setInput("email", "root@example.com");
    setInput("password", "long enough root password");
    setInput("password-confirmation", "long enough root password");
    setInput("bootstrap-token", "test-bootstrap-token-0123456789abcdef");

    const form = document.querySelector("form");
    await act(async () => {
      form?.dispatchEvent(new SubmitEvent("submit", { bubbles: true, cancelable: true }));
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(document.body.textContent).toContain("Enter your admin email and password");
    expect(document.body.textContent).toContain("Root setup completed. Sign in to continue MFA setup.");
  });
});
