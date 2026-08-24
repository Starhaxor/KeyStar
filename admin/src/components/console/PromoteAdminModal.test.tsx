import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import PromoteAdminModal from "./PromoteAdminModal";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

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
  vi.restoreAllMocks();
});

describe("PromoteAdminModal", () => {
  it("associates form labels and waits for explicit confirmation before requesting promotion", async () => {
    const promote = vi.fn().mockResolvedValue({ tempPassword: "one-time-password" });
    render(<PromoteAdminModal open userEmail="user@example.com" onClose={() => undefined} onPromote={promote} />);

    const email = document.querySelector<HTMLInputElement>("input[type=email]");
    const role = document.querySelector<HTMLSelectElement>("select");
    expect(document.querySelector('label[for="promote-admin-email"]')?.textContent).toBe("Email");
    expect(document.querySelector('label[for="promote-admin-role"]')?.textContent).toBe("Role");
    expect(email?.id).toBe("promote-admin-email");
    expect(role?.id).toBe("promote-admin-role");
    expect(promote).not.toHaveBeenCalled();

    const confirm = Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
      (button) => button.textContent?.trim() === "Create admin",
    );
    await act(async () => confirm?.click());

    expect(promote).toHaveBeenCalledOnce();
    expect(promote).toHaveBeenCalledWith("viewer");
  });
});
