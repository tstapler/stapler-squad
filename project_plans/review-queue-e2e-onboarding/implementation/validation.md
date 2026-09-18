# Validation Plan: review-queue-e2e-onboarding

**Date**: 2026-08-03

> **Corrected post-implementation (2026-08-03)**: see `requirements.md`'s
> "Root cause (corrected)" section — the onboarding modal was not the actual
> cause of the failures this plan targets. Coverage below still holds (same
> test file, same 0-failed bar), it's just backed by a different fix than
> originally planned.

## Happy Path Scenario
Given a fresh browser context that has never dismissed the first-run onboarding modal, when a `review-queue.spec.ts` test calls `page.goto(...)` followed immediately by `await dismissOnboardingIfPresent(page)`, then the modal (if present) is clicked away within 5000ms and the test's own assertions/`waitForSelector` calls proceed unobstructed to a pass.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: every `page.goto()` test dismisses onboarding first, same pattern as `escalation-reasoning.spec.ts` | tests/e2e/review-queue.spec.ts | `review queue page loads successfully` | E2E | Fresh context; modal must be dismissed before `waitForSelector('[data-testid="review-queue"]')` |
| AC1 (cont.) | tests/e2e/review-queue.spec.ts | `review queue badge is visible` | E2E | No `waitForSelector` at all — asserts directly with `toBeVisible()`, so dismissal must precede the first `expect(...)`, not just the primary wait |
| AC1 (cont.) | tests/e2e/review-queue.spec.ts | `review queue panel renders without errors` | E2E | Fresh context; modal dismissal precedes `waitForSelector('[data-testid="review-queue"]')` |
| AC1 (cont.) | tests/e2e/review-queue.spec.ts | `session creation wizard has all steps` | E2E | Route is `/sessions/new`, not `/review-queue` — proves dismissal isn't scoped to one route (see AC5) |
| AC1 (cont.) | tests/e2e/review-queue.spec.ts | `session creation form has required test IDs` | E2E | Same as above, multi-step wizard form |
| AC1 (cont.) | tests/e2e/review-queue.spec.ts | `review-queue-loaded sentinel is present after page renders` | E2E | Modal dismissal precedes `waitForSelector('[data-testid="review-queue"]')` and the `review-queue-loaded` sentinel check |
| AC1 (cont.) | tests/e2e/review-queue.spec.ts | `when queue has items, each carries acknowledge data-testid` | E2E | Dismissal precedes `waitForSelector('[data-testid="review-queue-loaded"]', { state: 'attached' })` |
| AC1 (cont.) | tests/e2e/review-queue.spec.ts | `acknowledge button removes item from DOM (optimistic UI)` | E2E | Dismissal precedes the same sentinel wait; also exercises a stateful click flow after dismissal, confirming the modal doesn't reappear mid-test |
| AC2: `npx playwright test review-queue.spec.ts --reporter=list` reports 0 failed (both `chromium` and `chromium-dom`), including the 10 previously-failing combinations | tests/e2e/review-queue.spec.ts | (full file — 11 test bodies × 2 projects = 22 test instances) | E2E | Run `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list` (Task 1.1.2a); expect **0 failed** — not a fixed "18/15/3" split (corrected during Phase 4 pre-mortem: actual count is 22 instances, with the passed/skipped split among the 8 active tests varying by test-server queue state) per plan.md Story 1.1.2's corrected acceptance criteria |
| AC3: fix changes only the dismissal step — no assertion/selector/test-intent change, no production code touched | tests/e2e/review-queue.spec.ts, tests/e2e/escalation-reasoning.spec.ts | (all 8 active tests in review-queue.spec.ts + `shows escalation reason for a real no-match hook escalation`) | E2E | Indirect proof: every pre-existing assertion in both files still passes verbatim post-change (Tasks 1.1.2a–c) — an accidental assertion/selector edit would surface as a new failure, not a pass. Directly confirmed by diff review (`git diff` scoped to the 3 files touched, per plan.md's Files list) rather than a runtime check |
| AC4: dismissal helper is reused (not duplicated) by both `review-queue.spec.ts` and `escalation-reasoning.spec.ts` | tests/e2e/escalation-reasoning.spec.ts | `shows escalation reason for a real no-match hook escalation` | E2E | Run `cd tests/e2e && npx playwright test escalation-reasoning.spec.ts --reporter=list` (Task 1.1.2c) post-migration; same pass count as pre-migration baseline proves the `dismissOnboardingIfPresent(page)` call site (replacing the inline click-and-catch block) behaves identically |
| AC5: `/sessions/new` tests also get the dismissal step (modal is global, not route-scoped) | tests/e2e/review-queue.spec.ts | `session creation wizard has all steps`, `session creation form has required test IDs` | E2E | Both tests `goto(${BASE_URL}/sessions/new)`; covered by the same full-file run as AC2 (Task 1.1.2a) |

## UX Acceptance Tests
N/A — pure test-infrastructure fix, no user-facing surface.

## Test Stack
- **E2E**: `@playwright/test` v1.61.1, run via `cd tests/e2e && npx playwright test <file>.spec.ts --reporter=list`, against the isolated test server spun up automatically by `tests/e2e/global-setup.ts` (dynamically-assigned free port, PID-scoped `--test-dir`, torn down by `global-teardown.ts`). No unit or integration test layer applies to this repo's e2e suite, and this fix has no data-store/migration surface, so no other test type is in scope.

## Coverage Targets and How to Measure

This repo's e2e suite has no line-coverage tool — the pass/fail bar is the actual gate:

- `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list` reports **0 failed** across both `chromium` and `chromium-dom` projects (22 total test instances: 11 test bodies × 2 projects — do not chase a fixed passed/skipped split, per plan.md Story 1.1.2's corrected acceptance criteria) — including all 10 test/project combinations listed as failing in the original bug report.
- Re-run with `--retries=0` (`cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list --retries=0`) must show 0 failed with zero retries consumed — proves the fix is deterministic and not masked by the config's default `retries: 1` (Task 1.1.2b).
- `cd tests/e2e && npx playwright test escalation-reasoning.spec.ts --reporter=list` must match its pre-migration baseline pass count exactly — 0 new failures introduced by the `dismissOnboardingIfPresent(page)` migration (Task 1.1.2c).
- Any deviation from these counts blocks completion — re-open Story 1.1.1 per plan.md's Task 1.1.2b/c guidance (check for a missing or misplaced `dismissOnboardingIfPresent(page)` call, i.e. one not strictly preceding the test's first wait/assertion).
