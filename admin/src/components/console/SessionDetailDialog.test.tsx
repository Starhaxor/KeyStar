import React, { useState } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import type { ConsoleSession } from "@/lib/types";
import SessionDetailDialog from "./SessionDetailDialog";

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
});

const session = {
  id: "session-123",
  user_id: "user-123",
  user_email: "operator@example.com",
  license_id: "license-123",
  status: "active",
  expires_at: "2026-08-25T12:00:00Z",
  created_at: "2026-08-24T12:00:00Z",
  token_hash: "must-not-be-rendered",
  raw_fingerprint: "must-not-be-rendered",
} as ConsoleSession & { token_hash: string; raw_fingerprint: string };

function Harness() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>View session</button>
      <SessionDetailDialog session={session} isOpen={open} onClose={() => setOpen(false)} />
    </>
  );
}

describe("SessionDetailDialog", () => {
  it("shows safe session metadata and restores focus after Escape", () => {
    render(<Harness />);
    const trigger = document.querySelector<HTMLButtonElement>("button");
    act(() => trigger?.focus());
    act(() => trigger?.click());

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog?.textContent).toContain("Session details");
    expect(dialog?.textContent).toContain("session-123");
    expect(dialog?.textContent).toContain("operator@example.com");
    expect(dialog?.textContent).toContain("license-123");
    expect(dialog?.textContent).not.toContain("must-not-be-rendered");
    expect(dialog?.contains(document.activeElement)).toBe(true);

    act(() => document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })));

    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });
});
