import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import Field from "./Field";

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

describe("Field", () => {
  it("links a label, description, and error to its named input", () => {
    render(
      <Field
        id="admin-email"
        label="Email"
        name="email"
        description="Use your administrator email address."
        error="Enter a valid email address."
      >
        <input type="email" />
      </Field>
    );

    const input = document.querySelector("input");
    const label = document.querySelector("label");
    const description = document.getElementById("admin-email-description");
    const error = document.getElementById("admin-email-error");

    expect(label?.htmlFor).toBe("admin-email");
    expect(input?.id).toBe("admin-email");
    expect(input?.getAttribute("name")).toBe("email");
    expect(input?.getAttribute("aria-describedby")).toBe(
      "admin-email-description admin-email-error"
    );
    expect(input?.getAttribute("aria-invalid")).toBe("true");
    expect(description?.textContent).toBe("Use your administrator email address.");
    expect(error?.textContent).toBe("Enter a valid email address.");
  });
});
