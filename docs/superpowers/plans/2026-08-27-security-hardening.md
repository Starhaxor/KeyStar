# Comprehensive Security Hardening Implementation Plan

> Execute inline with strict red-green-refactor cycles. Preserve unrelated user changes and do not alter live data.

**Goal:** Close every confirmed, source-fixable security finding from the 2026-08-27 audit and leave repeatable verification in CI.

**Architecture:** Security policy is centralized at trust boundaries: config validation at startup, application scoping in repository interfaces, outbound URL/network validation in the webhook transport, encrypted MFA storage in the auth service, and secure defaults in clients and deployment. Tests exercise both the validation helper and the boundary that consumes it.

**Stack:** Go, PostgreSQL, Next.js/TypeScript, C++17, CMake, GitHub Actions.

---

1. Add failing Go tests for unpredictable webhook secrets, public-HTTPS URL enforcement, redirect/DNS protections, tenant-scoped refresh revocation, valid refreshed claims, encrypted TOTP persistence, strong config secrets, and metrics authentication.
2. Implement the minimal backend changes required by those tests, including interface and SQL updates plus migration-safe encrypted TOTP handling.
3. Upgrade the Go toolchain declaration and vulnerable modules, then run unit/integration tests, vet, vulnerability scanning, and static analysis.
4. Add console header tests, update the Next.js configuration, and run test/lint/build/audit.
5. Add failing C++ tests for URL policy, JSON escaping, timeouts, redirect policy, and response limits; implement the cross-platform transport changes and run CTest.
6. Harden CI workflow permissions and security gates, update environment/deployment documentation, and validate configuration examples.
7. Review the complete diff for regressions and rerun the full release gate before reporting completion.
