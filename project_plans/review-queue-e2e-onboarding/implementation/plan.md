# Implementation Plan: review-queue-e2e-onboarding

> **Corrected post-implementation (2026-08-03)**: the onboarding-modal
> dismissal below was implemented as planned but, verified live, did not fix
> any of the 10 failing tests — the modal was never the actual cause. The
> real causes were two unrelated bugs found during implementation: see
> `requirements.md`'s "Root cause (corrected)" section. This plan's Story
> 1.1.1/1.1.2 execution steps stayed accurate for the dismissal-helper work;
> they just weren't sufficient on their own. The additional fix (a
> `ReviewQueuePanel.tsx` sentinel change) and test rewrite (the two
> `SessionWizard`-targeting tests) aren't reflected in the task breakdown
> below — see the commit diff for their actual scope.

**Feature**: Dismiss the first-run onboarding modal in `review-queue.spec.ts` so its
8 active tests stop racing/timing out against the modal overlay, via a shared plain-function
helper also reused by `escalation-reasoning.spec.ts`.
**Date**: 2026-08-03
**Status**: Ready for implementation
**ADRs**: None — a 5-line test helper does not warrant an ADR; no architecture,
schema, or production-code decision is being made.

---

## Domain Glossary
| Term | Definition | Notes |
|------|-----------|-------|
| Onboarding modal | First-run overlay rendered by `OnboardingModal.tsx` when `localStorage['stapler-squad:onboarded']` is unset | Mounts ~800ms after page load via `setTimeout` in `useOnboarding.ts:7-16` |
| `dismissOnboardingIfPresent` | New plain exported function that attempts a bare click on the "Skip onboarding" button and swallows any failure (never appeared, already dismissed, etc.) as a no-op | Lives in new `tests/e2e/pages/OnboardingPage.ts`; verbatim click-and-catch pattern from `escalation-reasoning.spec.ts`, not a class method |
| `review-queue-loaded` sentinel | `[data-testid="review-queue-loaded"]` element used by existing tests to detect the review-queue page has rendered | Unaffected by this fix; still asserted after dismissal |
| Active test | One of the 8 non-skipped test bodies across 3 `test.describe` blocks in `review-queue.spec.ts` that call `page.goto()` | The 3 tests in `test.describe.skip('Advanced Review Queue Tests (Require Backend)')` are out of scope — already skipped, no bodies to touch |

---

## Pattern Decisions
| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Helper shape | `export async function dismissOnboardingIfPresent(page: Page): Promise<void> {...}` (plain function, no class) | `tests/e2e/pages/BacklogMutations.ts` (7 plain `export async function`s, zero classes); `StuckItemsPage.ts`/`BacklogItemDetailPage.ts` (standalone functions alongside a `<Domain>Page` class for actions with no reusable state); `research/pitfalls.md` #2/#3 (recommended a plain function from the start) | `class OnboardingPage` with a `dismissIfPresent()` method | Corrected per architecture review: the original rationale ("every one of the 11 files in `tests/e2e/pages/` is a `<Domain>Page` class") was factually wrong — `BacklogMutations.ts` exports plain functions, and the directory's own convention is classes for stateful multi-locator page objects, plain functions for single-purpose, stateless actions. A helper with one locator and one call site is the latter |
| Dismissal logic inside `dismissOnboardingIfPresent` | Bare `page.getByRole('button', { name: 'Skip onboarding' }).click({ timeout: 5000 }).catch(() => {})` — verbatim `escalation-reasoning.spec.ts` snippet | `tests/e2e/escalation-reasoning.spec.ts` (proven pattern, already correctly polling) | `isVisible({ timeout: 5000 }).catch(() => false)` guard before an unguarded `.click()` | Corrected per adversarial review: `Locator.isVisible({ timeout })` has a **deprecated, ignored** `timeout` option — it checks the DOM synchronously and returns immediately, it does not poll. Since the modal mounts ~800ms after `page.goto()`, the guard would evaluate before the modal exists, return `false`, and skip the click, leaving the bug unfixed. `.click({ timeout })` (unlike `.isVisible({ timeout })`) does actually poll/wait, so the bare click+catch is not just simpler but the only one of the two that works. This also makes AC1's "same pattern as escalation-reasoning.spec.ts" true literally, not just "in effect" |
| Call-site wiring | Explicit `await dismissOnboardingIfPresent(page);` as the first line after every `page.goto()` in each of the 8 active test bodies in `review-queue.spec.ts`, plus a migrated call in `escalation-reasoning.spec.ts` | pitfalls.md #2, #3 | File-wide `test.beforeEach` | The file navigates to two different routes (`/review-queue`, `/sessions/new`); a `beforeEach` runs before `goto()`, not after, so it can't own "goto then dismiss" for both without per-test conditional logic — per-call-site is simpler and matches how `escalation-reasoning.spec.ts` already places the block (immediately after its own `goto`) |
| `escalation-reasoning.spec.ts` | Migrated: its existing inline click-and-catch block is replaced with `await dismissOnboardingIfPresent(page);` (Task 1.1.1e) | AC4 (requires the helper be reused, not duplicated, by both spec files) | Leaving it unmodified | Corrected per adversarial review Blocker 2: AC4 explicitly requires reuse by both files once extraction is chosen. `escalation-reasoning.spec.ts` already has its own dismissal logic — replacing that in-place call with the shared helper is a DRY-up of an already-in-scope file, not the "adding onboarding-dismissal to unrelated spec files" that requirements.md's non-goal actually rules out. Zero production code is touched either way |

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
Task 1.1.0 (PREREQ: rebase/create worktree onto origin/main — local main
            is 26 commits behind and lacks escalation-reasoning.spec.ts
            entirely; `git show origin/main:tests/e2e/escalation-reasoning.spec.ts`
            must succeed before Task 1.1.1e can run)
        │
        ▼
Task 1.1.1a (create dismissOnboardingIfPresent() in OnboardingPage.ts)
        │
        ▼
Task 1.1.1b (wire into "Review Queue Smoke Tests", 3 tests)
Task 1.1.1c (wire into "Session Creation Flow (UI Only)", 2 tests)      ── all four can run
Task 1.1.1d (wire into "Review Queue Acknowledge Flow — UI Contract",   ── in parallel once
             3 tests)                                                      1.1.1a lands
Task 1.1.1e (migrate escalation-reasoning.spec.ts's inline block)
        │         │         │         │
        └─────────┴─────────┴─────────┘
                  ▼
Task 1.1.2a (run review-queue.spec.ts, verify 18/18 on chromium + chromium-dom)
                  │
                  ▼
Task 1.1.2b (re-run review-queue.spec.ts with --retries=0 for a clean, non-masked signal)
                  │
                  ▼
Task 1.1.2c (run escalation-reasoning.spec.ts, verify it still passes post-migration)
```

---

## Phase 1: Fix onboarding-modal race in review-queue.spec.ts

### Epic 1.1: Dismiss onboarding modal before every review-queue.spec.ts assertion
**Goal**: All 18 tests in `tests/e2e/review-queue.spec.ts` (15 active + 3
skipped-by-design) pass on both `chromium` and `chromium-dom` projects, with the
onboarding-dismissal logic centralized in a new, reusable page helper and no
production code touched.

#### Story 1.1.1: Extract and wire a reusable onboarding-dismissal helper
**As a** developer maintaining `review-queue.spec.ts` and `escalation-reasoning.spec.ts`,
**I want** a single `dismissOnboardingIfPresent(page)` helper called right after every
`page.goto()`, **so that** the first-run onboarding overlay can never again block or race
a subsequent assertion in either file, without duplicating the click-and-catch snippet
9 times (8 new call sites + 1 migrated).

**Acceptance Criteria**:
- New file `tests/e2e/pages/OnboardingPage.ts` exports a plain
  `async function dismissOnboardingIfPresent(page: Page): Promise<void>` (no class).
  - *Given* a fresh browser context that has never seen onboarding, *When*
    `dismissOnboardingIfPresent(page)` is called after `page.goto()`, *Then* it
    waits up to 5000ms for the "Skip onboarding" button, clicks it, and returns
    without throwing.
  - *Given* a browser context that has already dismissed onboarding (button
    never appears), *When* `dismissOnboardingIfPresent(page)` is called, *Then*
    it resolves as a no-op within ~5000ms without throwing.
- Every active test body in `tests/e2e/review-queue.spec.ts` calls
  `await dismissOnboardingIfPresent(page);` as the first statement after its
  `page.goto(...)` call and strictly before any other `waitForSelector` /
  `expect(...)`.
  - *Given* the 8 active tests across `Review Queue Smoke Tests`,
    `Session Creation Flow (UI Only)`, and `Review Queue Acknowledge Flow —
    UI Contract`, *When* each is inspected, *Then* each has exactly one
    `dismissOnboardingIfPresent(page)` call positioned between its `goto()` and
    its first subsequent wait/assertion — including `review queue badge is
    visible`, which has no `waitForSelector` and asserts directly with
    `toBeVisible()`.
- `tests/e2e/escalation-reasoning.spec.ts`'s existing inline dismissal block is
  replaced with a call to the same `dismissOnboardingIfPresent(page)` helper
  (Task 1.1.1e), satisfying AC4's "reused, not duplicated, by both spec files."
- No assertion, selector, or test intent in `review-queue.spec.ts` or
  `escalation-reasoning.spec.ts` is changed — diff is limited to the new
  import and one `dismissOnboardingIfPresent(page)` call per test (replacing
  the inline block in `escalation-reasoning.spec.ts`'s one test).
- No file under `web-app/src/...`, `session/...`, or any other production
  source path is modified.
**Files**: `tests/e2e/pages/OnboardingPage.ts` (new), `tests/e2e/review-queue.spec.ts`,
`tests/e2e/escalation-reasoning.spec.ts`

##### Task 1.1.0: Verify implementation base is `origin/main` (~2 min) — PREREQUISITE, do this first
- Pre-mortem P1 finding: the `main` checkout used during planning was 26 commits
  behind `origin/main` and does not contain `tests/e2e/escalation-reasoning.spec.ts`
  at all (added by PR #315). `tests/e2e/review-queue.spec.ts` itself was verified
  byte-identical between the two bases, so only Task 1.1.1e is at risk — but
  confirm before starting so it isn't silently skipped.
- Run `git show origin/main:tests/e2e/escalation-reasoning.spec.ts | head -1` —
  must succeed (prints the file's first line). If it fails on the implementation
  branch/worktree, rebase or re-branch from `origin/main` before proceeding.
- Also run `npx playwright test review-queue.spec.ts --list` once to confirm the
  actual test count matches this plan's assumption (11 `test()` bodies × 2
  projects = 22 instances — see Task 1.1.2a) before treating any pass/skip
  number in Story 1.1.2 as a hard target.
- Files: none (verification only)

##### Task 1.1.1a: Create `OnboardingPage.ts` helper (~5 min)
- Create `tests/e2e/pages/OnboardingPage.ts` as a plain function module (not a
  class) — matches `tests/e2e/pages/BacklogMutations.ts`'s convention of
  `export async function` helpers for single-purpose, stateless actions,
  rather than `SessionsPage.ts`'s `<Domain>Page` class shape, which is for
  stateful, multi-locator page objects. This helper holds no state and has
  exactly one call pattern, so it doesn't earn a class.
- Implement:
  ```ts
  import { Page } from '@playwright/test';

  /**
   * Dismisses the first-run onboarding modal if it's present. The modal mounts
   * ~800ms after page load (useOnboarding.ts's setTimeout), so this must be a
   * real click-and-catch, not an isVisible() guard — Locator.isVisible({timeout})
   * is a deprecated no-op that checks the DOM synchronously and never polls,
   * which would silently skip the click before the modal exists. .click({timeout})
   * does poll, so the bare click+catch below is what actually works.
   */
  export async function dismissOnboardingIfPresent(page: Page): Promise<void> {
    await page
      .getByRole('button', { name: 'Skip onboarding' })
      .click({ timeout: 5000 })
      .catch(() => {});
  }
  ```
- This is the exact pattern already proven in `escalation-reasoning.spec.ts` —
  no new logic, just extracted to a shared location.
- Files: `tests/e2e/pages/OnboardingPage.ts`

##### Task 1.1.1b: Wire helper into "Review Queue Smoke Tests" (~5 min)
- Add `import { dismissOnboardingIfPresent } from './pages/OnboardingPage';` to
  `tests/e2e/review-queue.spec.ts` (top of file, alongside existing imports).
- In each of the 3 tests in `test.describe('Review Queue Smoke Tests', ...)`
  (`review queue page loads successfully`, `review queue badge is visible`,
  `review queue panel renders without errors`), insert
  `await dismissOnboardingIfPresent(page);` immediately after `page.goto(...)`
  and before the first subsequent `waitForSelector`/`expect`.
- Files: `tests/e2e/review-queue.spec.ts`

##### Task 1.1.1c: Wire helper into "Session Creation Flow (UI Only)" (~4 min)
- In both tests in `test.describe('Session Creation Flow (UI Only)', ...)`
  (`session creation wizard has all steps`, `session creation form has
  required test IDs`), insert `await dismissOnboardingIfPresent(page);`
  immediately after `page.goto(`${BASE_URL}/sessions/new`)` and before the
  first subsequent assertion.
- Files: `tests/e2e/review-queue.spec.ts`

##### Task 1.1.1d: Wire helper into "Review Queue Acknowledge Flow — UI Contract" (~5 min)
- In all 3 tests in `test.describe('Review Queue Acknowledge Flow — UI
  Contract', ...)` (`review-queue-loaded sentinel is present after page
  renders`, `when queue has items, each carries acknowledge data-testid`,
  `acknowledge button removes item from DOM (optimistic UI)`), insert
  `await dismissOnboardingIfPresent(page);` immediately after `page.goto(...)`
  and before the first subsequent wait/assertion.
- Do not touch `test.describe.skip('Advanced Review Queue Tests (Require
  Backend)', ...)` — its 3 tests have empty bodies and no `goto()`.
- Files: `tests/e2e/review-queue.spec.ts`

##### Task 1.1.1e: Migrate `escalation-reasoning.spec.ts` to the shared helper (~4 min)
- Confirm the exact call site with `git show origin/main:tests/e2e/escalation-reasoning.spec.ts`
  (expected around line 16-28 per research/stack.md).
- Add `import { dismissOnboardingIfPresent } from './pages/OnboardingPage';` to
  `tests/e2e/escalation-reasoning.spec.ts` (top of file, alongside existing imports).
- Replace the existing inline block:
  ```ts
  await page
    .getByRole('button', { name: 'Skip onboarding' })
    .click({ timeout: 5000 })
    .catch(() => {});
  ```
  with:
  ```ts
  await dismissOnboardingIfPresent(page);
  ```
  at its one call site.
- No other line in this file changes — this is a pure extraction, not a
  behavior change (the helper's implementation is byte-for-byte the same
  logic that was inline here).
- Files: `tests/e2e/escalation-reasoning.spec.ts`

#### Story 1.1.2: Verify the fix closes all 10 previously-failing tests, and that `escalation-reasoning.spec.ts` still passes
**As a** CI maintainer, **I want** confirmation that all 18 tests in
`review-queue.spec.ts` pass deterministically on both configured projects, and
that `escalation-reasoning.spec.ts` still passes after being migrated to the
shared helper, **so that** the backlog bug (`5ab8a12e-17a3-441c-bc4c-88be826eb5bf`)
can be closed with evidence, not assertion, and the DRY-up in Task 1.1.1e is
verified rather than assumed safe.
**Acceptance Criteria**:
- `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list`
  reports **0 failed**, across both the `chromium` and `chromium-dom` projects.
  (Corrected during Phase 4 validation/pre-mortem: the file has 11 `test()`
  bodies — 3 Smoke + 2 Session-Creation + 3 Acknowledge-Flow = 8 active, + 3
  permanently skipped via `test.describe.skip` — × 2 unrestricted projects =
  **22 total test instances** per run, not "18 total/15 passed/3 skipped" as
  originally assumed from the bug report. The 3 skipped tests contribute 6
  fixed skips; 2 of the 8 active tests additionally contain a runtime
  `test.skip()` if the test-server's review queue happens to be empty, so the
  exact passed-vs-skipped split among the remaining 16 active instances can
  vary — the only truly fixed target is 0 failed.)
  - *Given* the wiring from Story 1.1.1 is complete, *When* the command above
    is run, *Then* the previously-failing 10 test/project combinations listed
    in the bug report now pass (or, for the 2 data-dependent tests, skip
    cleanly rather than time out).
- A second run with `--retries=0` also reports 0 failed, confirming the fix is
  not being masked by the config's default `retries: 1`.
- `cd tests/e2e && npx playwright test escalation-reasoning.spec.ts --reporter=list`
  still passes with the same pass count as before the Task 1.1.1e migration,
  confirming the shared-helper extraction didn't regress that file.
**Files**: none (verification only, no additional file changes)

##### Task 1.1.2a: Run full spec and confirm 0 failed (~3 min)
- Run `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list`.
- Confirm the report shows **0 failed** across both `chromium` and
  `chromium-dom` (22 total test instances: 11 test bodies × 2 projects — do
  not chase a specific passed/skipped split, since 2 of the 8 active tests
  self-skip at runtime depending on test-server queue contents; see Story
  1.1.2's corrected acceptance criteria).
- Files: none

##### Task 1.1.2b: Re-run with `--retries=0` for a clean signal (~3 min)
- Run `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list --retries=0`.
- Confirm 0 failed with zero retries consumed — proves the fix is
  deterministic, not luck-of-the-retry.
- If either run shows a failure, re-open Story 1.1.1 and re-check the exact
  test body where the `dismissOnboardingIfPresent(page)` call is missing or
  misplaced (see pitfalls.md #2: it must precede literally the first
  wait/assertion, not just the primary one).
- Files: none

##### Task 1.1.2c: Verify `escalation-reasoning.spec.ts` still passes after migration (~3 min)
- Run `cd tests/e2e && npx playwright test escalation-reasoning.spec.ts --reporter=list`.
- Confirm the pass count matches the pre-migration baseline (same tests pass,
  0 new failures) — this is the direct check that Task 1.1.1e's DRY-up
  (replacing the inline click-and-catch with `dismissOnboardingIfPresent(page)`)
  didn't regress the file, since Tasks 1.1.2a/1.1.2b only re-verify
  `review-queue.spec.ts`.
- If it fails, re-check Task 1.1.1e's call site placement matches exactly
  where the original inline block was (same point in the test body, same
  `page` instance).
- Files: none
