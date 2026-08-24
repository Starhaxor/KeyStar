import path from "node:path";
import { defineConfig, devices } from "@playwright/test";

const adminDirectory = __dirname;
const backendDirectory = path.resolve(adminDirectory, "../backend");
const testDatabaseURL =
  process.env.TEST_DATABASE_URL ??
  "postgres://keystar_test:keystar_test@127.0.0.1:5432/keystar_test?sslmode=disable";
const isCI = Boolean(process.env.CI);

const backendEnvironment = {
  TEST_DATABASE_URL: testDatabaseURL,
  DATABASE_URL: testDatabaseURL,
  LICENSE_HMAC_KEY: "e2e-license-hmac-key",
  HARDWARE_HMAC_KEY: "e2e-hardware-hmac-key",
  ED25519_PRIVATE_KEY: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
  LICENSE_ISSUER: "keystar-e2e",
  LICENSE_AUDIENCE: "keystar-e2e-clients",
  PRODUCT: "e2e-product",
  ADMIN_SESSION_SECRET: "e2e-admin-session-secret",
  ADMIN_ALLOWED_ORIGIN: "http://127.0.0.1:3000",
  ADMIN_COOKIE_SECURE: "false",
  SERVER_ADDR: "127.0.0.1:8080",
};

const backendCommand = isCI
  ? "go run ./cmd/e2e-fixture reset && go run ./cmd/server serve"
  : "docker compose -f ../docker-compose.yml up -d --wait db && go run ./cmd/e2e-fixture reset && go run ./cmd/server serve";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: isCI,
  retries: isCI ? 1 : 0,
  workers: 1,
  reporter: isCI ? [["github"], ["list"]] : [["list"]],
  timeout: 45_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: "http://127.0.0.1:3000",
    ...devices["Desktop Chrome"],
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  webServer: [
    {
      command: backendCommand,
      cwd: backendDirectory,
      env: backendEnvironment,
      url: "http://127.0.0.1:8080/readyz",
      reuseExistingServer: false,
      timeout: 180_000,
      stdout: "pipe",
      stderr: "pipe",
    },
    {
      command: "npm run dev -- --hostname 127.0.0.1 --port 3000",
      cwd: adminDirectory,
      env: { NEXT_PUBLIC_API_URL: "http://127.0.0.1:8080" },
      url: "http://127.0.0.1:3000/signin",
      reuseExistingServer: false,
      timeout: 180_000,
      stdout: "pipe",
      stderr: "pipe",
    },
  ],
});
