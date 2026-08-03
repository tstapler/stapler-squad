# Implementation Plan: review-queue-e2e-onboarding

**Feature**: Dismiss the first-run onboarding modal in `review-queue.spec.ts` so its
8 active tests stop racing/timing out against the modal overlay.
**Date**: 2026-08-03
**Status**: Ready for implementation
**ADRs**: None — a 5-line test helper does not warrant an ADR; no architecture,
schema, or production-code decision is being made.

---

## Domain Glossary
| Term | Definition | Notes |
|------|-----------|-------|
| Onboarding modal | First-run overlay rendered by `OnboardingModal.tsx` when `localStorage['stapler-squad:onboarded']` is unset | Mounts ~800ms after page load via `setTimeout` in `useOnboarding.ts:7-16` |
| `dismissIfPresent` | New helper method that waits briefly for the "Skip onboarding" button and clicks it if present, swallowing "never appeared" as a no-op | Lives in new `tests/e2e/pages/OnboardingPage.ts` |
| `review-queue-loaded` sentinel | `[data-testid="review-queue-loaded"]` element used by existing tests to detect the review-queue page has rendered | Unaffected by this fix; still asserted after dismissal |
| Active test | One of the 8 non-skipped test bodies across 3 `test.describe` blocks in `review-queue.spec.ts` that call `page.goto()` | The 3 tests in `test.describe.skip('Advanced Review Queue Tests (Require Backend)')` are out of scope — already skipped, no bodies to touch |

---

## Pattern Decisions
| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Helper shape | `class OnboardingPage { constructor(page: Page) {...}; async dismissIfPresent(): Promise<void> {...} }` | `tests/e2e/pages/SessionsPage.ts` (`class FooPage`, `constructor(page: Page)`, `readonly page: Page`, `async` action methods) | Plain exported function | Every one of the 11 existing files in `tests/e2e/pages/` is a `<Domain>Page` class; a bare function would be the only exception in the directory for no functional gain |
| Dismissal logic inside `dismissIfPresent` | `isVisible({ timeout: 5000 }).catch(() => false)` guard, then unguarded `.click()` only if visible | pitfalls.md #1 and #5 | Bare `.getByRole(...).click({ timeout: 5000 }).catch(() => {})` (verbatim escalation-reasoning.spec.ts snippet) | The bare click-and-catch swallows every action error (strict-mode violations, detached-element races, not just "never appeared"), so a future `aria-label` rename would silently reproduce this exact bug with the real cause hidden. The `isVisible` guard narrows the swallow to "button never appeared" and lets a genuine click failure surface as a real, attributable test failure. Behaviorally equivalent (no-op on already-onboarded contexts, real dismissal on fresh ones) — same selector (`getByRole('button', { name: 'Skip onboarding' })`) and same 5000ms timeout, so AC1's "same pattern" is satisfied in effect, not byte-for-byte |
| Call-site wiring | Explicit `await onboarding.dismissIfPresent();` as the first line after every `page.goto()` in each of the 8 active test bodies | pitfalls.md #2, #3 | File-wide `test.beforeEach` | The file navigates to two different routes (`/review-queue`, `/sessions/new`); a `beforeEach` runs before `goto()`, not after, so it can't own "goto then dismiss" for both without per-test conditional logic — per-call-site is simpler and matches how `escalation-reasoning.spec.ts` already places the block (immediately after its own `goto`) |
| `escalation-reasoning.spec.ts` | Not modified in this task | requirements.md non-goals | Migrating it to `OnboardingPage` now | Out of scope per requirements.md; the helper is created for reuse but wired into `review-queue.spec.ts` only — flagged as a natural follow-up, not part of this fix |

---

## Migration Plan
N/A — no schema or data changes.

## Observability Plan
- **Logs**: N/A for a test-file fix.
- **Metrics**: N/A.
- **Alerts**: N/A — no new alerts required.

## Risk Control
- **Feature flag**: not gated (test-only change, no production code path affected).
- **Rollback procedure**: standard revert via PR close + revert commit.
- **Staged rollout**: full rollout on merge (CI gate is the only consumer).

## Unresolved Questions
None.

## Dependency Visualization
```
Task 1.1.1a (create OnboardingPage.ts)
        │
        ▼
Task 1.1.1b (wire into "Review Queue Smoke Tests", 3 tests)
Task 1.1.1c (wire into "Session Creation Flow (UI Only)", 2 tests)     ── all three can run
Task 1.1.1d (wire into "Review Queue Acknowledge Flow — UI Contract", ── in parallel once
             3 tests)                                                    1.1.1a lands
        │         │         │
        └─────────┴─────────┘
                  ▼
Task 1.1.2a (run full spec, verify 18/18 on chromium + chromium-dom)
                  │
                  ▼
Task 1.1.2b (re-run with --retries=0 for a clean, non-masked signal)
```

---

## Phase 1: Fix onboarding-modal race in review-queue.spec.ts

### Epic 1.1: Dismiss onboarding modal before every review-queue.spec.ts assertion
**Goal**: All 18 tests in `tests/e2e/review-queue.spec.ts` (15 active + 3
skipped-by-design) pass on both `chromium` and `chromium-dom` projects, with the
onboarding-dismissal logic centralized in a new, reusable page helper and no
production code touched.

#### Story 1.1.1: Extract and wire a reusable onboarding-dismissal helper
**As a** developer maintaining `review-queue.spec.ts`, **I want** a single
`OnboardingPage.dismissIfPresent()` helper called right after every `page.goto()`,
**so that** the first-run onboarding overlay can never again block or race a
subsequent assertion in this file, without duplicating the click-and-catch
snippet 8 times.

**Acceptance Criteria**:
- New file `tests/e2e/pages/OnboardingPage.ts` exports `class OnboardingPage`
  with an `async dismissIfPresent(): Promise<void>` method.
  - *Given* a fresh browser context that has never seen onboarding, *When*
    `dismissIfPresent()` is called after `page.goto()`, *Then* it waits up to
    5000ms for the "Skip onboarding" button, clicks it, and returns without
    throwing.
  - *Given* a browser context that has already dismissed onboarding (button
    never appears), *When* `dismissIfPresent()` is called, *Then* it resolves
    as a no-op within ~5000ms without throwing.
- Every active test body in `tests/e2e/review-queue.spec.ts` calls
  `await onboarding.dismissIfPresent();` (with an `OnboardingPage` instance
  constructed from that test's `page`) as the first statement after its
  `page.goto(...)` call and strictly before any other `waitForSelector` /
  `expect(...)`.
  - *Given* the 8 active tests across `Review Queue Smoke Tests`,
    `Session Creation Flow (UI Only)`, and `Review Queue Acknowledge Flow —
    UI Contract`, *When* each is inspected, *Then* each has exactly one
    `dismissIfPresent()` call positioned between its `goto()` and its first
    subsequent wait/assertion — including `review queue badge is visible`,
    which has no `waitForSelector` and asserts directly with `toBeVisible()`.
- No assertion, selector, or test intent in `review-queue.spec.ts` is changed
  — diff is limited to the new import, one `OnboardingPage` construction per
  test (or one shared `const onboarding = new OnboardingPage(page);` per
  test if that reads more naturally — implementer's call, no behavior
  difference), and one `dismissIfPresent()` call per test.
- No file under `web-app/src/...`, `session/...`, or any other production
  source path is modified.
- `escalation-reasoning.spec.ts` is not modified.
**Files**: `tests/e2e/pages/OnboardingPage.ts` (new), `tests/e2e/review-queue.spec.ts`

##### Task 1.1.1a: Create `OnboardingPage.ts` helper (~5 min)
- Create `tests/e2e/pages/OnboardingPage.ts` following the `SessionsPage.ts`
  shape: `import { Page, Locator } from '@playwright/test';`, `export class
  OnboardingPage`, `readonly page: Page`, constructor `(page: Page)`.
- Add `readonly skipButton: Locator = page.getByRole('button', { name: 'Skip
  onboarding' });` (built in the constructor, matching `SessionsPage.ts`'s
  pattern of pre-built `Locator` fields).
- Implement:
  ```ts
  async dismissIfPresent(): Promise<void> {
    const visible = await this.skipButton
      .isVisible({ timeout: 5000 })
      .catch(() => false);
    if (visible) {
      await this.skipButton.click();
    }
  }
  ```
- Add a short comment (2-3 lines max) noting: modal mounts ~800ms after load
  via `useOnboarding.ts`'s `setTimeout`, hence the timeout; `isVisible` guard
  (not bare click+catch) so a future `aria-label` rename fails loudly instead
  of silently reproducing this bug.
- Files: `tests/e2e/pages/OnboardingPage.ts`

##### Task 1.1.1b: Wire helper into "Review Queue Smoke Tests" (~5 min)
- Add `import { OnboardingPage } from './pages/OnboardingPage';` to
  `tests/e2e/review-queue.spec.ts` (top of file, alongside existing imports).
- In each of the 3 tests in `test.describe('Review Queue Smoke Tests', ...)`
  (`review queue page loads successfully`, `review queue badge is visible`,
  `review queue panel renders without errors`), insert
  `await new OnboardingPage(page).dismissIfPresent();` immediately after
  `page.goto(...)` and before the first subsequent `waitForSelector`/`expect`.
- Files: `tests/e2e/review-queue.spec.ts`

##### Task 1.1.1c: Wire helper into "Session Creation Flow (UI Only)" (~4 min)
- In both tests in `test.describe('Session Creation Flow (UI Only)', ...)`
  (`session creation wizard has all steps`, `session creation form has
  required test IDs`), insert `await new OnboardingPage(page).dismissIfPresent();`
  immediately after `page.goto(`${BASE_URL}/sessions/new`)` and before the
  first subsequent assertion.
- Files: `tests/e2e/review-queue.spec.ts`

##### Task 1.1.1d: Wire helper into "Review Queue Acknowledge Flow — UI Contract" (~5 min)
- In all 3 tests in `test.describe('Review Queue Acknowledge Flow — UI
  Contract', ...)` (`review-queue-loaded sentinel is present after page
  renders`, `when queue has items, each carries acknowledge data-testid`,
  `acknowledge button removes item from DOM (optimistic UI)`), insert
  `await new OnboardingPage(page).dismissIfPresent();` immediately after
  `page.goto(...)` and before the first subsequent wait/assertion.
- Do not touch `test.describe.skip('Advanced Review Queue Tests (Require
  Backend)', ...)` — its 3 tests have empty bodies and no `goto()`.
- Files: `tests/e2e/review-queue.spec.ts`

#### Story 1.1.2: Verify the fix closes all 10 previously-failing tests
**As a** CI maintainer, **I want** confirmation that all 18 tests in
`review-queue.spec.ts` pass deterministically on both configured projects,
**so that** the backlog bug (`5ab8a12e-17a3-441c-bc4c-88be826eb5bf`) can be
closed with evidence, not assertion.
**Acceptance Criteria**:
- `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list`
  reports 18 tests total, 15 passed, 3 skipped, 0 failed, across both the
  `chromium` and `chromium-dom` projects.
  - *Given* the wiring from Story 1.1.1 is complete, *When* the command above
    is run, *Then* the previously-failing 10 test/project combinations listed
    in the bug report now pass.
- A second run with `--retries=0` also passes 15/15 active tests, confirming
  the fix is not being masked by the config's default `retries: 1`.
**Files**: none (verification only, no additional file changes)

##### Task 1.1.2a: Run full spec and confirm pass counts (~3 min)
- Run `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list`.
- Confirm output shows `18 passed` / `15 passed, 3 skipped` (exact wording per
  installed Playwright version) with 0 failures, for both `chromium` and
  `chromium-dom` projects.
- Files: none

##### Task 1.1.2b: Re-run with `--retries=0` for a clean signal (~3 min)
- Run `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list --retries=0`.
- Confirm the same pass/skip counts with zero retries consumed — proves the
  fix is deterministic, not luck-of-the-retry.
- If either run shows a failure, re-open Story 1.1.1 and re-check the exact
  test body where the `dismissIfPresent()` call is missing or misplaced
  (see pitfalls.md #2: it must precede literally the first wait/assertion,
  not just the primary one).
- Files: none
