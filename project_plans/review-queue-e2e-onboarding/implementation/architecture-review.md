# Architecture Review: review-queue-e2e-onboarding
**Date**: 2026-08-03
**Verdict**: CLEAN (re-reviewed 2026-08-03 — sole Concern resolved, no new issues)

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository
(verified: `ls docs/adr/` — no `000`-prefixed file; lowest numbers present are
`003-*.md`). No constitution to treat as a hard constraint. No violations to
report.

## Blockers

None.

## Concerns

- [x] **RESOLVED (re-reviewed 2026-08-03).** Task 1.1.1a / Pattern Decisions table —
  The plan now specifies `export async function dismissOnboardingIfPresent(page:
  Page): Promise<void>` (no class) in `tests/e2e/pages/OnboardingPage.ts`, and the
  "Helper shape" row in the Pattern Decisions table cites `BacklogMutations.ts`'s
  7 plain `export async function`s (zero classes) as precedent — re-verified
  directly: `rg -n "export (async )?function|export class"
  tests/e2e/pages/BacklogMutations.ts` shows 7 functions, 0 classes, confirming the
  citation is now accurate (the original review's factual objection was to the old
  "every file is a class" claim, which this replaces). The row's stated rationale
  ("Corrected per architecture review: the original rationale... was factually
  wrong... A helper with one locator and one call site is the latter [stateless
  single-purpose case]") directly and correctly addresses the original finding.
  No further action needed.

  <details><summary>Original finding text (for record)</summary>

  Task 1.1.1a / Pattern Decisions table — The plan justifies choosing a
  `class OnboardingPage` over a plain function *specifically* because "Every one
  of the 11 existing files in `tests/e2e/pages/` is a `<Domain>Page` class; a
  bare function would be the only exception in the directory for no functional
  gain." This is factually wrong, and the error undermines the stated rationale.
  Verified directly: `tests/e2e/pages/BacklogMutations.ts` exports **zero**
  classes and **seven** plain `export async function`s (`createBacklogItemDirect`,
  etc.). `tests/e2e/pages/StuckItemsPage.ts` and `BacklogItemDetailPage.ts` each
  pair a `<Domain>Page` class with standalone exported helper functions
  (`seedStuckItem`, `enableBacklogFeatureFlag`, `disableBacklogFeatureFlag`,
  `seedHeadlessTriageItem`) for actions that don't need to hold locator state.
  **Remediation (now applied)**: change Task 1.1.1a to export
  `async function dismissOnboardingIfPresent(page: Page): Promise<void>` from
  `tests/e2e/pages/OnboardingPage.ts`.

  </details>

## Re-review (2026-08-03): new tasks added by the adversarial review

The plan also picked up Task 1.1.1e (migrate `escalation-reasoning.spec.ts` to
the shared helper) and Task 1.1.2c (verify it), and switched Task 1.1.1a's
dismissal logic from an `isVisible()` guard to a bare click+catch. Spot-checked
these — no new architecture problem:

- The bare-click rationale ("`Locator.isVisible({timeout})`'s `timeout` option
  is deprecated/ignored — it doesn't poll, so it could evaluate before the
  modal mounts ~800ms after `goto()` and silently skip the click") is
  consistent with Playwright's own documented behavior (`isVisible`'s
  `timeout` option has been a no-op since Playwright 1.20).
- Task 1.1.1e's snippet (`await page.getByRole('button', { name: 'Skip
  onboarding' }).click({ timeout: 5000 }).catch(() => {});`) was verified
  verbatim against the real source: `git show
  origin/main:tests/e2e/escalation-reasoning.spec.ts` lines 184–187 match
  exactly.
- Task 1.1.1e depends on `tests/e2e/escalation-reasoning.spec.ts`, which does
  **not** exist in this worktree's local `main` (26 commits behind
  `origin/main`, confirmed via `git ls-tree HEAD -- tests/e2e/escalation-
  reasoning.spec.ts` returning empty). This is not a new gap the plan
  introduced — `research/stack.md`/`pitfalls.md` already document the
  divergence and the plan's own task text routes around it by reading the
  file via `git show origin/main:...` rather than assuming a local copy.
  It does mean the branch this plan is implemented on must be rebased onto
  (or otherwise contain) `origin/main`'s escalation-reasoning commit before
  Task 1.1.1e can literally edit the file — worth a one-line prerequisite
  note in Task 1.1.1e, but not an architecture defect and not blocking.
- The two nitpicks below that referenced the old `isVisible` guard are now
  moot (guard removed, explicit `{ timeout: 5000 }` now always present) —
  left in place, struck, for record.

## Nitpicks

- ~~**AC1 wording vs. actual behavior**: AC1 says the fix uses "the same...
  pattern as `escalation-reasoning.spec.ts`," but the Pattern Decisions table
  documented a deliberate deviation (`isVisible().catch(() => false)` guard).~~
  **Moot as of the re-review**: the guard was removed; Task 1.1.1a's dismissal
  logic is now the verbatim `escalation-reasoning.spec.ts` snippet, so AC1 is
  literally true, not just "in effect."
- **`test.step()` wrapping omitted**: `research/pitfalls.md` #5 suggested
  wrapping the dismiss-if-present logic in `test.step('Dismiss onboarding modal
  if present', ...)` so a swallowed "button never appeared" outcome is visible
  in the HTML/trace report rather than silent. The plan doesn't include this.
  Cheap, optional addition — not required for correctness.
- ~~**No explicit timeout on the guarded `.click()`**~~ **Moot as of the
  re-review**: the `isVisible` guard was removed; the click now always carries
  an explicit `{ timeout: 5000 }`.
- Per-call-site `await dismissOnboardingIfPresent(page);` (vs. a shared
  Playwright fixture) matches this repo's existing convention exactly —
  verified no spec file in `tests/e2e/` uses `test.extend`/fixtures. Not a
  concern, noted only to confirm this axis was checked. (Originally phrased
  against the now-superseded `new OnboardingPage(page).dismissIfPresent()`
  class call; updated to match the plain-function call the plan now uses —
  the underlying observation is unchanged.)
