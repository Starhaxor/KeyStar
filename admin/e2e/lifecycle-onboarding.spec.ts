import { expect, test } from "./fixtures";

test("resumes onboarding after reload and issues a test license", async ({
  authenticatedPage: page,
  onboarding,
}) => {
  await page.goto("/onboarding");
  await page.getByRole("button", { name: "Create application" }).click();
  const applicationDialog = page.getByRole("dialog", { name: "Create application" });
  await applicationDialog.getByLabel("Organization").selectOption({ label: onboarding.organizationName });
  await applicationDialog.getByLabel("Application name").fill(onboarding.applicationName);
  await applicationDialog.getByLabel("Slug (optional)").fill(onboarding.applicationSlug);
  await applicationDialog.getByRole("button", { name: "Create application" }).click();

  await expect(page.getByRole("heading", { name: "Create a publishable credential" })).toBeVisible();

  await page.getByRole("button", { name: "Users" }).click();
  await page.getByRole("link", { name: "All users" }).click();
  await expect(page).toHaveURL(/\/users$/);
  await page.getByRole("button", { name: "Add user" }).click();
  const userDialog = page.getByRole("dialog");
  await userDialog.getByPlaceholder("user@example.com").fill(onboarding.userEmail);
  await userDialog.getByPlaceholder("At least 10 characters").fill(onboarding.userPassword);
  await userDialog.getByRole("button", { name: "Create user" }).click();
  await expect(page.getByRole("link", { name: onboarding.userEmail })).toBeVisible();

  await page.goto("/onboarding");
  await page.getByLabel("Environment").selectOption("test");
  await page.getByLabel("Credential name").fill("E2E onboarding SDK");
  await page.getByRole("button", { name: "Create credential" }).click();
  await expect(page.getByRole("heading", { name: "Save the credential now" })).toBeVisible();
  await page.getByRole("dialog").getByRole("button", { name: "Done" }).click();

  await page.reload();
  await expect(page.getByRole("heading", { name: "Create a product and plan" })).toBeVisible();
  await page.getByLabel("Product name").fill("E2E Onboarding Product");
  await page.getByLabel("Product slug (optional)").fill("e2e-onboarding-product");
  await page.getByLabel("Plan name").fill("E2E Onboarding Plan");
  await page.getByLabel("Plan code").fill("e2e-onboarding-plan");
  await page.getByRole("button", { name: "Create product and plan" }).click();
  await expect(page.getByRole("heading", { name: "Issue a test license" })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name: "Issue a test license" })).toBeVisible();
  await page.getByLabel("Existing test user email").fill(onboarding.userEmail);
  await page.getByRole("button", { name: "Issue test license" }).click();
  await expect(page.getByRole("heading", { name: "Save the test license" })).toBeVisible();
  await page.getByRole("dialog").getByRole("button", { name: "Done" }).click();

  await expect(page).toHaveURL(/\/$/);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
});
