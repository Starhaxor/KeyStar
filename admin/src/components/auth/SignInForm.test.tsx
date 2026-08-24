import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import SignInForm from "./SignInForm";

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
  api: { login: vi.fn(), completeMfa: vi.fn() },
}));

let container: HTMLDivElement | undefined;
let root: Root | undefined;

function render(element: React.ReactNode) {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root?.render(element));
}

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  container = undefined;
  root = undefined;
});

describe("SignInForm", () => {
  it("exposes password visibility as a named button", () => {
    render(<SignInForm />);

    const visibilityButton = document.querySelector('button[aria-label="Show password"]');
    expect(visibilityButton).not.toBeNull();

    act(() => visibilityButton?.dispatchEvent(new MouseEvent("click", { bubbles: true })));

    expect(document.querySelector('button[aria-label="Hide password"]')).not.toBeNull();
    expect(document.querySelector('input[name="password"]')?.getAttribute("type")).toBe("text");
  });
});
