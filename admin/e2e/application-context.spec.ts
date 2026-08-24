import { expect, test } from "./fixtures";

test("application switching keeps users, licenses, and devices scoped", async ({
  authenticatedPage: page,
  e2eFixture,
}) => {
  const alpha = e2eFixture.applications.alpha;
  const beta = e2eFixture.applications.beta;

  for (const route of ["/users", "/licenses", "/devices"]) {
    await page.goto(route);
    await expect(page.getByText(alpha.userEmail).first()).toBeVisible();
    await expect(page.getByText(beta.userEmail)).toHaveCount(0);
  }

  await page.getByLabel("Selected application").selectOption(beta.id);
  await expect(page.getByLabel("Selected application")).toHaveValue(beta.id);

  for (const route of ["/users", "/licenses", "/devices"]) {
    await page.goto(route);
    await expect(page.getByText(beta.userEmail).first()).toBeVisible();
    await expect(page.getByText(alpha.userEmail)).toHaveCount(0);
  }
});
