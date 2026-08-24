import React, { useState } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import AccessibleDialog from "./AccessibleDialog";

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

describe("AccessibleDialog", () => {
  it("exposes dialog semantics, moves focus inside, and restores it after Escape", () => {
    render(<DialogHarness />);

    const trigger = document.querySelector("button");
    act(() => trigger?.focus());
    act(() => trigger?.click());

    const dialog = document.querySelector('[role="dialog"]');
    expect(dialog?.getAttribute("aria-modal")).toBe("true");
    expect(dialog?.contains(document.activeElement)).toBe(true);

    act(() => {
      dialog?.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    });

    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });
});
