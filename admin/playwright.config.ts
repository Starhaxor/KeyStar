import path from "node:path";
import { defineConfig, devices } from "@playwright/test";

const adminDirectory = __dirname;
const backendDirectory = path.resolve(adminDirectory, "../backend");
const testDatabaseURL =
  process.env.TEST_DATABASE_URL ??
  "postgres://keystar_test:keystar_test@127.0.0.1:55432/keystar_test?sslmode=disable";
const isCI = Boolean(process.env.CI);
const backendPort = process.env.E2E_BACKEND_PORT ?? "8080";
const adminPort = process.env.E2E_ADMIN_PORT ?? "3000";
const backendURL = `http://127.0.0.1:${backendPort}`;
const adminURL = `http://127.0.0.1:${adminPort}`;
const adminCommand = process.env.E2E_USE_PRODUCTION_ADMIN
  ? `npm run build && npm run start -- --hostname 127.0.0.1 --port ${adminPort}`
  : `npm run dev -- --hostname 127.0.0.1 --port ${adminPort}`;

const backendEnvironment = {
  TEST_DATABASE_URL: testDatabaseURL,
  DATABASE_URL: testDatabaseURL,
  LICENSE_HMAC_KEY: "e2e-license-key-0123456789abcdef0123456789",
  HARDWARE_HMAC_KEY: "e2e-hardware-key-0123456789abcdef01234567",
  ED25519_PRIVATE_KEY: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
  LICENSE_ISSUER: "keystar-e2e",
  LICENSE_AUDIENCE: "keystar-e2e-clients",
  PRODUCT: "e2e-product",
  ADMIN_SESSION_SECRET: "e2e-session-key-0123456789abcdef0123456789",
  ADMIN_BOOTSTRAP_TOKEN: "e2e-bootstrap-token-0123456789abcdef012345",
  ADMIN_MFA_ENCRYPTION_KEY: "e2e-mfa-0123456789abcdef01234567",
  APPLICATION_KEY_ENCRYPTION_KEYS:
    "1=bm5ubm5ubm5ubm5ubm5ubm5ubm5ubm5ubm5ubm5ubm4=",
  APPLICATION_KEY_ACTIVE_VERSION: "1",
  ADMIN_ALLOWED_ORIGIN: adminURL,
  ADMIN_COOKIE_SECURE: "false",
  SERVER_ADDR: `127.0.0.1:${backendPort}`,
};

const backendCommand = isCI || Boolean(process.env.TEST_DATABASE_URL)
  ? "go run ./cmd/e2e-fixture reset && go run ./cmd/server serve"
  : "docker compose -f ../docker-compose.yml up -d --wait db && go run ./cmd/e2e-fixture reset && go run ./cmd/server serve";

export default defineConfig({
  testDir: "./e2e",
  testMatch: "**/*.spec.ts",
  fullyParallel: false,
  forbidOnly: isCI,
  retries: isCI ? 1 : 0,
  workers: 1,
  reporter: isCI ? [["github"], ["list"]] : [["list"]],
  timeout: 45_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: adminURL,
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
      url: `${backendURL}/readyz`,
      reuseExistingServer: false,
      timeout: 180_000,
      stdout: "pipe",
      stderr: "pipe",
    },
    {
      command: adminCommand,
      cwd: adminDirectory,
      env: { NEXT_PUBLIC_API_URL: backendURL },
      url: `${adminURL}/signin`,
      reuseExistingServer: false,
      timeout: 180_000,
      stdout: "pipe",
      stderr: "pipe",
    },
  ],
});
