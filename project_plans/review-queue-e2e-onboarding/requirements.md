# Requirements: review-queue-e2e-onboarding

## Complexity

1 (quick task) — mechanical test-file fix, no production code change, single
established pattern already proven in `escalation-reasoning.spec.ts`.

## Source

Backlog item `5ab8a12e-17a3-441c-bc4c-88be826eb5bf`: "bug: tests/e2e/review-queue.spec.ts — 10/18 tests fail on first-run onboarding modal"

## Problem

`tests/e2e/review-queue.spec.ts` has 10 of 18 tests failing/timing out waiting for
`[data-testid="review-queue-loaded"]` / `[data-testid="review-queue"]`. Confirmed
pre-existing (not caused by any in-flight feature branch) and deterministic
(reproduces with `--workers=1`, single project — not resource-contention flake).

## Root cause (confirmed during triage, see research.md)

Playwright is configured `fullyParallel: false` / `workers: 1`, but each `test()`
still gets its own fresh browser context by default, meaning a clean `localStorage`
per test. `OnboardingModal` renders on top of the page on any context that hasn't
seen onboarding before. `review-queue.spec.ts` calls `page.goto(...)` directly in
every test and asserts on page content without ever dismissing that modal, so the
modal overlay blocks/races the assertions and `waitForSelector` calls time out.

`tests/e2e/escalation-reasoning.spec.ts` (merged in PR #315, present on
`origin/main` — note: the local `main` checkout used for this triage was 26 commits
behind `origin/main` and does not contain this file; verify against `origin/main`
before implementing) already handles this correctly:

```ts
await page
  .getByRole('button', { name: 'Skip onboarding' })
  .click({ timeout: 5000 })
  .catch(() => {});
```

placed after `page.goto(...)` and before the first assertion/`waitForSelector`. The
short timeout plus `.catch(() => {})` makes this a no-op on contexts that have
already dismissed onboarding (modal never appears) and a real dismissal on fresh
contexts.

`review-queue.spec.ts` is the only spec with 3 `test.describe` blocks touching
`/review-queue` or `/sessions/new` without this step; grep confirms no other
current spec file contains `'Skip onboarding'` besides `escalation-reasoning.spec.ts`.

## Acceptance Criteria

1. Every test in `tests/e2e/review-queue.spec.ts` that calls `page.goto(...)` and
   then asserts on page content dismisses the onboarding modal first, using the
   same `getByRole('button', { name: 'Skip onboarding' }).click({ timeout: 5000 }).catch(() => {})`
   pattern as `escalation-reasoning.spec.ts`.
2. `npx playwright test review-queue.spec.ts --reporter=list` reports 0 failed
   across both the `chromium` and `chromium-dom` projects, including the 10
   previously-failing test/project combinations listed in the bug report.
   (Verified during Phase 4 validation: the file has 11 `test()` bodies — 8
   active + 3 permanently skipped via `test.describe.skip` — × 2 projects = 22
   total test instances per run, not "18 total/15 passed/3 skipped" as
   originally assumed from the bug report's own count; confirm the exact
   pass/skip split with `--list` before treating any number as a hard target.)
3. The fix does not change any assertion, selector, or test intent — only inserts
   the onboarding-dismissal step. No production/source code is modified.
4. If the dismissal logic is extracted into a shared helper (e.g.
   `tests/e2e/pages/`), it is reused (not duplicated) by both `review-queue.spec.ts`
   and `escalation-reasoning.spec.ts`, per this repo's e2e helper convention
   (`.claude/rules/e2e-test-conventions.md`, "New page helpers go in `tests/e2e/pages/`").
5. `session creation wizard has all steps` and `session creation form has required
   test IDs` tests (which goto `/sessions/new`, not `/review-queue`) also get the
   dismissal step — the onboarding modal is global to first-run contexts, not
   scoped to the review-queue route.

## Non-goals

- Not fixing any backend/session-poller logic — confirmed unrelated (reproduces
  with `session/review_queue_poller.go` reverted).
- Not changing `OnboardingModal` component behavior or when it shows.
- Not adding onboarding-dismissal to unrelated spec files beyond
  `review-queue.spec.ts` (and only extracting a shared helper if that reduces
  duplication cleanly — not a required refactor).
