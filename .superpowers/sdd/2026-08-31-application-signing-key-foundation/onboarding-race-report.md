# Onboarding One-Time License Acknowledgment Implementation Plan and Report

> **For agentic workers:** Execute this plan inline with `superpowers:systematic-debugging`, `superpowers:test-driven-development`, and `superpowers:verification-before-completion`. Do not dispatch subagents or reviewers.

**Goal:** Keep newly issued one-time test-license material visible until the administrator explicitly acknowledges it, then allow onboarding completion to navigate to Overview.

**Architecture:** Continue holding plaintext license material only in `OnboardingWizard` component state and preserve the existing persisted-resource progress reload. Gate the existing completion redirect/render suppression on the absence of a pending one-time secret, so the dialog lifecycle remains the only plaintext lifetime.

**Tech Stack:** Next.js 16, React 19, TypeScript, Vitest/jsdom, Playwright.

**Spec:** Parent task instructions for the remaining `admin/e2e/lifecycle-onboarding.spec.ts` failure.

## Global Constraints

- Work only in `C:\Users\pc\Desktop\Projelerim\KeyStar\.worktrees\codex-application-signing-key-foundation`.
- Do not add waits, retries, longer timeouts, or weaken the E2E assertion.
- Do not expose or persist the one-time secret beyond the existing dialog lifecycle.
- Use strict RED/GREEN TDD and complete all requested verification gates.

## Phase 1: Root-cause evidence

### Baseline

- HEAD: `3b4574dde934313e07967e1b8a75e7a72dd3c2db` (`fix(security): close signing-key foundation review gaps`).
- Initial tracked worktree: clean.
- Focused reproduction command: `cd admin && npx playwright test e2e/lifecycle-onboarding.spec.ts --trace on --reporter=list`.
- Result: FAIL after 45.7 seconds. In this captured run, line 46 observed the heading, then line 47 timed out waiting for the dialog's `Done` button. The final page snapshot and screenshot show Overview. This is the same race as the reported line-46 failure, with a slightly longer transient observation window.
- Artifacts: `admin/test-results/lifecycle-onboarding-resum-feea6-d-and-issues-a-test-license/{trace.zip,error-context.md,test-failed-1.png,video.webm}`.

### Complete action/data-flow trace

1. `OnboardingWizard.createTestLicense` calls `api.createLicense(...)` for the selected product and plan.
2. `api.createLicense` sends `POST /v1/admin/licenses` and returns the `CreatedLicense` response.
3. The trace records the POST at monotonic time 8354.60 with HTTP 200. A redacted response inspection confirms response keys `ok`, `license`, and `key`, with non-empty `key` and license ID. The plaintext value is not copied into this report.
4. `createTestLicense` calls `reveal("license", response.key)`, which stores the plaintext only in local `revealedSecret` state.
5. The trace proves this state rendered: Playwright resolved the `Save the test license` heading at time 8373.52, and the after-assertion snapshot at 8375.30 contains both that heading and `Done`.
6. The same action then awaits `load(applicationID)`. The resulting `GET /v1/admin/onboarding/progress` returned HTTP 200 at time 8367.72 with `credential_count=1`, `product_count=1`, `plan_count=1`, and `license_count=1`.
7. `deriveOnboardingStep` maps that persisted snapshot to `complete`.
8. The completion effect unconditionally calls `router.replace("/")` whenever loading is false and the step is complete; the render guard simultaneously returns `null`.
9. The next progress request at time 8464.59 has Overview (`/`) as its referrer, and the final snapshot is Overview. The component has unmounted, so its one-time `revealedSecret` state and dialog are destroyed before the pending `Done` click can resolve.

### Working-pattern comparison

- The wizard's credential flow sets its one-time key state before reloading progress. Because credential completion advances only to the catalog step rather than triggering a route replacement, the dialog remains mounted until `Done`; the existing component test verifies copy and dismissal.
- The API Credentials and Webhooks pages likewise set one-time secret state, refresh durable lists, and render a modal controlled exclusively by that state. Neither performs a progress-driven redirect while a secret is pending.
- Material difference: only onboarding's final license action turns its refresh result into an unconditional redirect/render suppression.

### Specific hypothesis and distinguishing evidence

**Hypothesis:** The root cause is a premature product redirect: onboarding completion ignores the pending one-time secret. The progress reload correctly marks durable onboarding complete, and that completion unmounts the local dialog before acknowledgment.

- Not an API/secret-loss failure: POST is 200 with a key, and the heading/dialog render transiently.
- Not an obsolete test: the existing credential flow and all other one-time-secret dialogs require explicit dismissal, matching the security requirement that plaintext be saved before it disappears.
- Component state is lost, but as an effect of the premature redirect/unmount rather than the originating defect: state is demonstrably present before navigation.

## Planned TDD task

### Task 1: Gate completion navigation on one-time-secret acknowledgment

**Files:**

- Modify/test: `admin/src/components/onboarding/OnboardingWizard.test.tsx`
- Modify: `admin/src/components/onboarding/OnboardingWizard.tsx`
- Update evidence: `.superpowers/sdd/2026-08-31-application-signing-key-foundation/onboarding-race-report.md`

**Interfaces:**

- Consumes: existing `revealedSecret: { kind: "credential" | "license"; value: string } | null`, persisted `OnboardingProgress`, and `router.replace("/")`.
- Produces: completion navigation only when `step === "complete"`, loading is false, and no one-time secret awaits acknowledgment.

- [x] Add a component test that begins on the license step, issues a mocked one-time license, reloads completed progress, asserts the plaintext and dialog remain present with no navigation, clicks `Done`, then asserts navigation occurs and plaintext is removed.
- [x] Run the focused component test and capture the expected RED: current production code calls `navigation.replace("/")` before `Done` and suppresses the dialog.
- [x] Make the minimal production change: include the pending-secret state in both completion redirect and render-suppression conditions.
- [x] Re-run the focused component test GREEN.
- [x] Run focused Playwright with `--repeat-each=5`.
- [x] Run full admin unit, lint, build, and E2E gates.
- [x] Inspect the diff for secret persistence/exposure, unrelated changes, and correctness; commit with a clear subject.

## RED/GREEN evidence

### RED

Command:

`cd admin && npm test -- src/components/onboarding/OnboardingWizard.test.tsx -t "keeps a newly issued one-time license visible until it is acknowledged before navigating"`

Result: exit 1. Vitest ran the onboarding file; the new regression was the sole failure. At `OnboardingWizard.test.tsx:366`, `expect(navigation.replace).not.toHaveBeenCalled()` failed because `replace("/")` had already been called once. This is the expected missing-behavior failure, not a setup/type error.

### GREEN

Minimal implementation: both the completion effect and the `return null` completion guard now require `!revealedSecret`. No new storage, timers, retries, or secret-copying path was introduced.

Focused regression command:

`cd admin && npx vitest run src/components/onboarding/OnboardingWizard.test.tsx -t "keeps a newly issued one-time license visible until it is acknowledged before navigating"`

Result: exit 0; 1 passed, 9 skipped.

Focused component command:

`cd admin && npx vitest run src/components/onboarding/OnboardingWizard.test.tsx`

Result: exit 0; 10 passed.

### Focused browser verification and final assertion alignment

The first post-fix `--repeat-each=5` run passed the security-critical heading and `Done` interaction in every repetition, then all five repetitions failed at the old final assertion for `Application setup is complete`. That assertion contradicted the existing unit-tested completion contract (`router.replace("/")`) and the task requirement that navigation occur after acknowledgment. The E2E test was updated without weakening the one-time-secret assertion: it still requires the dialog heading and explicit `Done`, then now verifies the root URL and Overview after reload.

Final command:

`cd admin && npx playwright test e2e/lifecycle-onboarding.spec.ts --repeat-each=5 --reporter=list`

Result: exit 0; 5 passed in 27.4 seconds.

## Full verification

- `cd admin && npm test` — exit 0; 32 files passed, 104 tests passed.
- `cd admin && npm run lint` — exit 0; ESLint reported no errors.
- `cd admin && npm run build` — exit 0; optimized Next.js production build compiled, type-checked, and generated 24/24 static pages.
- `cd admin && npm run e2e` — exit 0; 7 Playwright tests passed in 17.7 seconds.
- `git diff --check` — exit 0; no whitespace errors. Git emitted only the repository's Windows LF-to-CRLF working-copy notices.

## Files changed

- `admin/src/components/onboarding/OnboardingWizard.tsx` — gate completion redirect and render suppression on acknowledgment of any pending one-time secret.
- `admin/src/components/onboarding/OnboardingWizard.test.tsx` — regression coverage for response, completed progress reload, retained dialog, acknowledgment, navigation, and plaintext removal.
- `admin/e2e/lifecycle-onboarding.spec.ts` — retain the one-time-license dialog/Done assertions and align the final persisted-completion assertion with the established Overview redirect.
- `.superpowers/sdd/2026-08-31-application-signing-key-foundation/onboarding-race-report.md` — investigation, TDD, verification, and self-review evidence.

## Self-review

- Root-cause scope: the production diff changes only the two conditions that destroyed the pending dialog; no unrelated refactor is included.
- Secret lifecycle: plaintext remains only in the existing React `revealedSecret` state. It is neither persisted nor added to progress, URLs, logging, storage, or another component. Existing close/Done behavior clears the state, after which completion navigation proceeds.
- Durable state: `load(applicationID)` still runs after issuance, so onboarding completion remains derived from server resources and survives reload.
- Navigation: an already-complete onboarding visit with no local secret retains the existing immediate Overview redirect. A newly completed visit redirects as soon as the dialog is dismissed.
- Test quality/mutation check: removing either `!revealedSecret` gate causes the regression to fail through premature navigation or loss of the dialog. Removing post-dismiss redirect behavior fails the same test's `replace("/")` assertion.
- E2E strength: the test still observes `Save the test license`, interacts with the real dialog's `Done` button, and additionally verifies the resulting URL and Overview.
- Diff hygiene: no arbitrary waits, retries, timeout increases, dependency changes, backend changes, or generated artifacts are included.
- Planned commit subject: `fix(admin): preserve onboarding license reveal`.

## Concerns

No product concerns. Non-failing test-server noise observed: Node warns that `NO_COLOR` is ignored when `FORCE_COLOR` is set, and one auth E2E emitted Next.js RSC fetch fallback messages while still passing. Neither is related to this change.
