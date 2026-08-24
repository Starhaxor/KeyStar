# Task 6 report — Production foundation verification gate

## Delivered changes

- Added a `documentation` GitHub Actions job using PowerShell. It fails when
  `README.md` does not contain the Docker Compose command, PowerShell
  integration runner, Playwright E2E command, CMake SDK command, or the local
  backend/test environment-variable names.
- Expanded the README with prerequisites, backend environment contract,
  Compose-backed Go integration steps, Windows CMake/CTest commands, admin
  unit/lint/build commands, Playwright installation/E2E commands, and safe
  teardown/reset guidance.
- Added an explicit Release A handoff status to the platform-completion spec.
  It separates the delivered implementation components from the incomplete
  full-gate verification, and records Releases B and C as intentionally
  deferred.

## Documentation check red/green evidence

1. Red, before the README change:

   ```powershell
   $readme = Get-Content -Raw README.md
   $required = @('docker compose up -d db', 'test-integration.ps1', 'npm run e2e', 'cmake -S backend/sdk/cpp')
   $requiredEnv = @('DATABASE_URL', 'LICENSE_HMAC_KEY', 'HARDWARE_HMAC_KEY', 'ED25519_PRIVATE_KEY', 'LICENSE_ISSUER', 'LICENSE_AUDIENCE', 'PRODUCT', 'ADMIN_SESSION_SECRET', 'TEST_DATABASE_URL')
   $missing = @($required + $requiredEnv | Where-Object { -not $readme.Contains($_) })
   if ($missing.Count -gt 0) { Write-Error ('README missing required verification documentation: ' + ($missing -join ', ')); exit 1 }
   ```

   Exit 1, with the expected output:

   ```text
   README missing required verification documentation: npm run e2e, cmake -S backend/sdk/cpp
   ```

2. Green, after the README change: the same check exited 0 and printed
   `README release-gate documentation check passed.` The workflow uses the
   same requirement set under `.github/workflows/verify.yml`.

## Release-gate verification (2026-08-24)

| Command | Result | Evidence |
| --- | --- | --- |
| `docker compose up -d db` | Blocked (exit 1) | Docker Desktop engine pipe is unavailable: `failed to connect to the docker API at npipe:////./pipe/dockerDesktopLinuxEngine`. |
| `.\scripts\test-integration.ps1` | Blocked (exit 1) | The runner reached Compose, received the same engine-pipe error, then threw `Could not start the PostgreSQL test container.` |
| `cd backend; go test ./...` | Partially successful / overall exit 1 | All non-integration packages passed; the PostgreSQL integration package failed only because `TEST_DATABASE_URL must be set for PostgreSQL integration tests`. The dedicated URL cannot be supplied without the unavailable Compose database. |
| `cmake -S backend/sdk/cpp -B build/sdk -DKEYSTAR_BUILD_TESTS=ON -G "Visual Studio 17 2022" -A x64` | Locally blocked by existing build cache (exit 1) | The untracked `build/sdk` cache was generated with Visual Studio 18 2026; it was preserved rather than deleted. |
| `cmake -S backend/sdk/cpp -B build/sdk-task6 -DKEYSTAR_BUILD_TESTS=ON` | Pass (exit 0) | Clean isolated configuration selected Visual Studio 18 2026 and MSVC 19.51.36256.0. |
| `cmake --build build/sdk-task6 --config Release` | Pass (exit 0) | Built `keystar.lib` and `keystar_tests.exe`. |
| `ctest --test-dir build/sdk-task6 -C Release --output-on-failure` | Pass (exit 0) | `1/1 Test #1: keystar_tests ... Passed`; 100% tests passed. |
| `cd admin; npm test` | Pass (exit 0) | 17 test files and 58 tests passed. |
| `cd admin; npm run lint` | Pass (exit 0) | ESLint exited cleanly. |
| `cd admin; npm run build` | Pass (exit 0) | Next.js production build compiled, typechecked, and generated 23 static pages. |
| `cd admin; npm run e2e:install` | Pass (exit 0) | Playwright Chromium installation command completed. |
| `cd admin; npm run e2e` | Blocked (exit 1) | Playwright failed before browser assertions because its Compose web server hit the same unavailable Docker Desktop engine pipe. |

## Handoff status

Release A implementation and its documented/CI-enforced verification path are
present. This is **not** a Release A delivery or all-green result: the required
Docker-backed integration and browser E2E commands could not run in this
environment. Re-run the documented gate on a Docker-capable Windows environment
and require every command to exit 0 before marking Release A complete.

Release B and Release C are intentionally deferred.
