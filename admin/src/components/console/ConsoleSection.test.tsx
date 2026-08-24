import React from "react";
import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { createRoot, type Root } from "react-dom/client";
import ConsoleSection, { TableCard } from "./ConsoleSection";
import AdminError from "../../app/(admin)/error";
import FullWidthError from "../../app/(full-width-pages)/error";
import {
  ApplicationForm,
  OrganizationForm,
} from "../../app/(admin)/applications/ApplicationsForms";

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

describe("ConsoleSection", () => {
  it("keeps its content in a horizontally scrollable shell", () => {
    render(
      <ConsoleSection title="Applications">
        <table><tbody><tr><td>Wide content</td></tr></tbody></table>
      </ConsoleSection>
    );

    const content = container?.querySelector<HTMLElement>(
      '[data-testid="console-section-content"]'
    );

    expect(content).not.toBeNull();
    expect(content?.classList.contains("overflow-x-auto")).toBe(true);
  });

  it("wraps direct tables in a horizontally scrollable card", () => {
    render(
      <TableCard>
        <table><tbody><tr><td>Wide table</td></tr></tbody></table>
      </TableCard>
    );

    const content = container?.querySelector<HTMLElement>(
      '[data-testid="table-card-content"]'
    );

    expect(content).not.toBeNull();
    expect(content?.classList.contains("overflow-x-auto")).toBe(true);
  });

  it.each([
    ["the admin shell", AdminError],
    ["the full-width shell", FullWidthError],
  ])("lets a visitor retry %s without exposing server details", (_, ErrorBoundary) => {
    let retries = 0;
    render(
      <ErrorBoundary
        error={new Error("internal database address 10.0.0.12")}
        reset={() => { retries += 1; }}
      />
    );

    const retry = Array.from(container?.querySelectorAll("button") ?? []).find(
      (button) => button.textContent === "Retry"
    );

    expect(retry).toBeDefined();
    expect(retry?.hasAttribute("disabled")).toBe(false);
    expect(container?.textContent).not.toContain("internal database address");
    act(() => retry?.click());
    expect(retries).toBe(1);
  });

  it("gives application creation controls accessible labels", () => {
    render(
      <ApplicationForm
        organizations={[{ id: "org-1", name: "KeyStar", slug: "keystar", status: "active", created_at: "", updated_at: "" }]}
        organizationID="org-1"
        name=""
        slug=""
        busy={false}
        onOrganizationChange={() => undefined}
        onNameChange={() => undefined}
        onSlugChange={() => undefined}
        onSubmit={() => undefined}
        onCancel={() => undefined}
      />
    );

    expect(container?.querySelector('label[for="application-organization"]')).not.toBeNull();
    expect(container?.querySelector('label[for="application-name"]')).not.toBeNull();
    expect(container?.querySelector('label[for="application-slug"]')).not.toBeNull();
    expect(container?.querySelector<HTMLInputElement>("#application-name")?.name).toBe("name");
  });

  it("gives organization creation controls an accessible label", () => {
    let submissions = 0;
    let cancellations = 0;
    render(
      <OrganizationForm
        name=""
        busy={false}
        onNameChange={() => undefined}
        onSubmit={() => { submissions += 1; }}
        onCancel={() => { cancellations += 1; }}
      />
    );

    expect(container?.querySelector('label[for="organization-name"]')).not.toBeNull();
    expect(container?.querySelector<HTMLInputElement>("#organization-name")?.name).toBe("name");
    const cancel = Array.from(container?.querySelectorAll("button") ?? []).find((button) => button.textContent === "Cancel");
    expect(cancel).toBeDefined();
    act(() => cancel?.click());
    expect(cancellations).toBe(1);
    expect(submissions).toBe(0);
  });
});
