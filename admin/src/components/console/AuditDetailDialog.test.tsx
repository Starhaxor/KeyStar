import React, { useState } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import type { AuditEntry } from "@/lib/types";
import AuditDetailDialog from "./AuditDetailDialog";

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

const entry: AuditEntry = {
  id: "audit-123",
  admin_account_id: "admin-123",
  actor_email: "operator@example.com",
  action: "USER_STATUS_CHANGED",
  resource_type: "user",
  resource_id: "user-123",
  user_agent: "test agent",
  metadata: {
    status: "disabled",
    api_token: "must-not-be-rendered",
    nested: { fingerprint: "must-not-be-rendered" },
  },
  created_at: "2026-08-24T12:00:00Z",
};

function Harness() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>View audit event</button>
      <AuditDetailDialog entry={entry} isOpen={open} onClose={() => setOpen(false)} />
    </>
  );
}

describe("AuditDetailDialog", () => {
  it("renders readable safe metadata and suppresses secret values", () => {
    render(<Harness />);
    act(() => document.querySelector<HTMLButtonElement>("button")?.click());

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog?.textContent).toContain("Audit event details");
    expect(dialog?.textContent).toContain("operator@example.com");
    expect(dialog?.textContent).toContain("USER_STATUS_CHANGED");
    expect(dialog?.textContent).toContain("user-123");
    expect(dialog?.textContent).toContain("status");
    expect(dialog?.textContent).toContain("disabled");
    expect(dialog?.textContent).toContain("[redacted]");
    expect(dialog?.textContent).not.toContain("must-not-be-rendered");
    expect(dialog?.querySelector('a[href="/users/user-123"]')).not.toBeNull();
    expect(dialog?.querySelector('button[aria-label="Copy resource ID"]')).not.toBeNull();
  });
});
