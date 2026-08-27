import { expect, signIn, test } from "./fixtures";

test("protects console routes and opens the selected application", async ({
  page,
  e2eFixture,
}) => {
  await page.goto("/users");
  await expect(page).toHaveURL(/\/signin$/);

  await signIn(page, e2eFixture.admin);

  await expect(page).toHaveURL(/\/$/);
  const applicationSelector = page.getByLabel("Selected application");
  await expect(applicationSelector).toHaveValue(e2eFixture.applications.alpha.id);
  await expect(applicationSelector.locator("option:checked")).toHaveText(
    e2eFixture.applications.alpha.name,
  );
});

test("requires MFA enrollment before application administration", async ({
  page,
  e2eFixture,
}) => {
  await page.goto("/signin");
  await page.locator("#sign-in-email").fill(e2eFixture.unenrolledAdmin.email);
  await page.locator("#sign-in-password").fill(e2eFixture.unenrolledAdmin.password);
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(page).toHaveURL(/\/security$/);
  await expect(
    page.getByRole("heading", { name: "Set up two-factor authentication" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Start setup" })).toBeEnabled();
});
