import React, { useState } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import { Modal } from "@/components/ui/modal";
import AccessibleDialog from "./AccessibleDialog";

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

function render(element: React.ReactNode) {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root?.render(element));
}

function pressEscape(target: Document | HTMLElement = document) {
  act(() => {
    target.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  });
}

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  document.body.style.overflow = "";
  container = undefined;
  root = undefined;
});

function DialogHarness() {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <>
      <button type="button" onClick={() => setIsOpen(true)}>
        Open dialog
      </button>
      <AccessibleDialog
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        title="Confirm action"
      >
        <button type="button">Continue</button>
      </AccessibleDialog>
    </>
  );
}

function StackedDialogHarness() {
  const [firstOpen, setFirstOpen] = useState(false);
  const [secondOpen, setSecondOpen] = useState(false);

  return (
    <>
      <button
        type="button"
        onClick={() => {
          setFirstOpen(true);
          setSecondOpen(true);
        }}
      >
        Open stacked dialogs
      </button>
      <AccessibleDialog isOpen={firstOpen} onClose={() => setFirstOpen(false)} title="First dialog">
        <button type="button">First action</button>
      </AccessibleDialog>
      <AccessibleDialog isOpen={secondOpen} onClose={() => setSecondOpen(false)} title="Second dialog">
        <button type="button">Second action</button>
      </AccessibleDialog>
    </>
  );
}

function LegacyModalHarness() {
  const [isOpen, setIsOpen] = useState(true);

  return (
    <Modal isOpen={isOpen} onClose={() => setIsOpen(false)}>
      <h2>Create application</h2>
      <button type="button">Save application</button>
    </Modal>
  );
}

describe("AccessibleDialog", () => {
  it("labels the dialog, locks scrolling, and restores focus after document Escape", () => {
    document.body.style.overflow = "auto";
    render(<DialogHarness />);

    const trigger = document.querySelector("button");
    act(() => trigger?.focus());
    act(() => trigger?.click());

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    const heading = dialog && document.getElementById(dialog.getAttribute("aria-labelledby") ?? "");
    expect(dialog?.getAttribute("aria-modal")).toBe("true");
    expect(heading?.textContent).toBe("Confirm action");
    expect(dialog?.contains(document.activeElement)).toBe(true);
    expect(document.body.style.overflow).toBe("hidden");

    pressEscape();

    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
    expect(document.body.style.overflow).toBe("auto");
  });

  it("skips display-hidden controls when choosing the initial focus target", () => {
    render(
      <AccessibleDialog isOpen onClose={() => undefined} title="Hidden control" showCloseButton={false}>
        <button type="button" style={{ display: "none" }}>
          Hidden control
        </button>
        <button type="button">Visible control</button>
      </AccessibleDialog>
    );

    expect(document.activeElement?.textContent).toBe("Visible control");
  });

  it("wraps Tab and Shift+Tab between the first and last visible controls", () => {
    render(
      <AccessibleDialog isOpen onClose={() => undefined} title="Focus loop" showCloseButton={false}>
        <button type="button">First action</button>
        <button type="button">Last action</button>
      </AccessibleDialog>
    );

    const first = Array.from(document.querySelectorAll("button")).find(
      (button) => button.textContent === "First action"
    );
    const last = Array.from(document.querySelectorAll("button")).find(
      (button) => button.textContent === "Last action"
    );
    act(() => last?.focus());
    act(() => last?.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true })));
    expect(document.activeElement).toBe(first);

    act(() => first?.focus());
    act(() =>
      first?.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", shiftKey: true, bubbles: true }))
    );
    expect(document.activeElement).toBe(last);
  });

  it("closes through the backdrop and named close button", () => {
    render(<DialogHarness />);

    const trigger = document.querySelector("button");
    act(() => trigger?.click());
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    act(() => dialog?.parentElement?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true })));
    expect(document.querySelector('[role="dialog"]')).toBeNull();

    act(() => trigger?.click());
    const closeButton = document.querySelector<HTMLButtonElement>('button[aria-label="Close dialog"]');
    expect(closeButton).not.toBeNull();
    act(() => closeButton?.click());
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  it("keeps the lower dialog focused and scroll-locked until a stacked top dialog closes", () => {
    render(<StackedDialogHarness />);

    const trigger = document.querySelector("button");
    act(() => trigger?.focus());
    act(() => trigger?.click());
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(2);
    expect(document.body.style.overflow).toBe("hidden");

    pressEscape();

    const remainingDialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
    expect(remainingDialog?.contains(document.activeElement)).toBe(true);
    expect(document.body.style.overflow).toBe("hidden");

    pressEscape();

    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(0);
    expect(document.activeElement).toBe(trigger);
    expect(document.body.style.overflow).toBe("");
  });

  it("derives a meaningful legacy Modal name from its heading", () => {
    render(<LegacyModalHarness />);

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    const heading = dialog && document.getElementById(dialog.getAttribute("aria-labelledby") ?? "");
    expect(heading?.textContent).toBe("Create application");
  });
});
