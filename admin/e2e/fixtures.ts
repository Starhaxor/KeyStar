import { createHmac } from "node:crypto";
import { execFile } from "node:child_process";
import path from "node:path";
import { promisify } from "node:util";
import { expect, test as base, type Page } from "@playwright/test";

const execFileAsync = promisify(execFile);
const backendDirectory = path.resolve(__dirname, "../../backend");
const defaultDatabaseURL =
  "postgres://keystar_test:keystar_test@127.0.0.1:5432/keystar_test?sslmode=disable";

type AdminFixture = {
  email: string;
  password: string;
  totpSecret?: string;
};

type ApplicationFixture = {
  id: string;
  name: string;
  userEmail: string;
  licenseId: string;
  deviceId: string;
  authSessionId: string;
};

export type E2EFixture = {
  admin: AdminFixture & { totpSecret: string };
  unenrolledAdmin: AdminFixture;
  applications: {
    alpha: ApplicationFixture;
    beta: ApplicationFixture;
  };
};

type TestFixtures = {
  authenticatedPage: Page;
};

type WorkerFixtures = {
  e2eFixture: E2EFixture;
};

function databaseURL() {
  return process.env.TEST_DATABASE_URL ?? defaultDatabaseURL;
}

async function runDatabaseFixture(action: "seed" | "reset") {
  const { stdout } = await execFileAsync(
    "go",
    ["run", "./cmd/e2e-fixture", action],
    {
      cwd: backendDirectory,
      env: { ...process.env, TEST_DATABASE_URL: databaseURL() },
      windowsHide: true,
    },
  );
  return stdout.trim();
}

function decodeBase32(value: string) {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const character of value.toUpperCase().replace(/=+$/, "")) {
    const index = alphabet.indexOf(character);
    if (index < 0) throw new Error("E2E TOTP secret is not valid base32");
    bits += index.toString(2).padStart(5, "0");
  }
  const bytes: number[] = [];
  for (let offset = 0; offset + 8 <= bits.length; offset += 8) {
    bytes.push(Number.parseInt(bits.slice(offset, offset + 8), 2));
  }
  return Buffer.from(bytes);
}

function currentTOTP(secret: string) {
  const counter = Math.floor(Date.now() / 30_000);
  const buffer = Buffer.alloc(8);
  buffer.writeBigUInt64BE(BigInt(counter));
  const digest = createHmac("sha1", decodeBase32(secret)).update(buffer).digest();
  const offset = digest[digest.length - 1] & 0x0f;
  const value =
    (((digest[offset] & 0x7f) << 24) |
      (digest[offset + 1] << 16) |
      (digest[offset + 2] << 8) |
      digest[offset + 3]) %
    1_000_000;
  return value.toString().padStart(6, "0");
}

export async function signIn(page: Page, admin: AdminFixture) {
  await page.goto("/signin");
  await page.getByLabel("Email").fill(admin.email);
  await page.getByLabel("Password").fill(admin.password);
  await page.getByRole("button", { name: "Sign in" }).click();

  if (admin.totpSecret) {
    await expect(page.getByLabel("Authentication code")).toBeVisible();
    await page
      .getByLabel("Authentication code")
      .fill(currentTOTP(admin.totpSecret));
    await page.getByRole("button", { name: "Verify" }).click();
  }
}

export const test = base.extend<TestFixtures, WorkerFixtures>({
  e2eFixture: [
    async ({}, provide) => {
      const serializedFixture = await runDatabaseFixture("seed");
      const fixture = JSON.parse(serializedFixture) as E2EFixture;
      try {
        await provide(fixture);
      } finally {
        await runDatabaseFixture("reset");
      }
    },
    { scope: "worker" },
  ],

  page: async ({ page, baseURL, e2eFixture }, provide) => {
    if (!baseURL) throw new Error("Playwright baseURL is required for E2E tests");
    await page.context().addCookies([
      {
        name: "keystar_application_id",
        value: e2eFixture.applications.alpha.id,
        url: baseURL,
      },
    ]);
    await provide(page);
  },

  authenticatedPage: async ({ page, e2eFixture }, provide) => {
    await signIn(page, e2eFixture.admin);
    await expect(page.getByLabel("Selected application")).toHaveValue(
      e2eFixture.applications.alpha.id,
    );
    await provide(page);
  },
});

export { expect };
