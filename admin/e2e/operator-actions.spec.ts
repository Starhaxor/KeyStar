import type { Page } from "@playwright/test";
import { expect, test } from "./fixtures";

async function openRowAction(page: Page, rowText: string, actionName: string) {
  const row = page.getByRole("row").filter({ hasText: rowText });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "Row actions" }).click();
  await page.getByRole("button", { name: actionName, exact: true }).click();
}

test("destructive user, session, and device actions wait for confirmation", async ({
  authenticatedPage: page,
  e2eFixture,
}) => {
  const alpha = e2eFixture.applications.alpha;
  const destructiveRequests: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET" && request.url().includes("/v1/admin/")) {
      destructiveRequests.push(`${request.method()} ${new URL(request.url()).pathname}`);
    }
  });

  await page.goto("/users");
  await openRowAction(page, alpha.userEmail, "Disable user");
  await expect(page.getByRole("heading", { name: "Disable user" })).toBeVisible();
  expect(destructiveRequests).toEqual([]);
  const userResponse = page.waitForResponse(
    (response) =>
      response.request().method() === "PATCH" &&
      response.url().includes("/v1/admin/users/"),
  );
  await page.getByRole("dialog").getByRole("button", { name: "Disable" }).click();
  expect((await userResponse).ok()).toBe(true);

  await page.goto("/sessions");
  await openRowAction(page, alpha.userEmail, "Revoke");
  await expect(page.getByRole("heading", { name: "Revoke session" })).toBeVisible();
  expect(destructiveRequests).toHaveLength(1);
  const sessionResponse = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      /\/v1\/admin\/sessions\/[^/]+\/revoke$/.test(new URL(response.url()).pathname),
  );
  await page.getByRole("dialog").getByRole("button", { name: "Revoke" }).click();
  expect((await sessionResponse).ok()).toBe(true);

  await page.goto("/devices");
  await openRowAction(page, alpha.userEmail, "Revoke");
  await expect(page.getByRole("heading", { name: "Revoke device" })).toBeVisible();
  expect(destructiveRequests).toHaveLength(2);
  const deviceResponse = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      /\/v1\/admin\/devices\/[^/]+\/revoke$/.test(new URL(response.url()).pathname),
  );
  await page.getByRole("dialog").getByRole("button", { name: "Revoke" }).click();
  expect((await deviceResponse).ok()).toBe(true);
  expect(destructiveRequests).toHaveLength(3);
});

test("Escape closes the shared dialog and restores focus", async ({
  authenticatedPage: page,
}) => {
  await page.goto("/applications");
  const trigger = page.getByRole("button", { name: "Add application" });
  await trigger.click();
  await expect(page.getByRole("dialog")).toBeVisible();

  await page.keyboard.press("Escape");

  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(trigger).toBeFocused();
});
