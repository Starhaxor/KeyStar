import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import ConfirmModal from "./ConfirmModal";

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

describe("ConfirmModal", () => {
  it("announces a confirmation error to assistive technology", () => {
    render(<ConfirmModal isOpen title="Archive product" message="Archive this product?" confirmLabel="Confirm archive" busy={false} error="Unable to archive product." onConfirm={() => undefined} onClose={() => undefined} />);

    expect(document.querySelector('[role="alert"]')?.textContent).toBe("Unable to archive product.");
  });
});
