# BUG-094: a11y test "Retry button" tab-walk uses a fixed 60-Tab budget that doesn't scale with list length [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-09-02
**Impact**: `UX Analysis (Axe + Lighthouse + Claude)` CI job fails intermittently on unrelated PRs, forcing re-runs and eroding trust in that check.

## Problem Description

`tests/e2e/accessibility.spec.ts:562` ("Cancel and Retry controls are keyboard-reachable buttons with distinct aria-labels") intermittently fails with `Error: Tab order never reached the Retry button` (assertion at line 688).

Observed failing on PR #678 (github.com/tstapler/stapler-squad, branch `stapler-squad-task-detection`) across two separate CI runs on two different commits:
- Run 33674246815, commit 13b600964
- Run 33681354888, commit 974ead4f0

PR #678's diff only touches `session/detection/detector.go`, `session/review_queue_determiner.go`, and one line in `web-app/src/components/sessions/SessionCard.tsx` (an unrelated `SubStatusChip` prop for the `WAITING_FOR_AGENT` sub-status) — nothing in the Failed-state Cancel/Retry code path this test exercises. The failure is not caused by, and isn't fixable within, that PR.

## Reproduction Steps

1. Run the `UX Analysis (Axe + Lighthouse + Claude)` CI job (or `npx playwright test accessibility.spec.ts` locally) on a PR unrelated to session-creation UI.
2. The Cancel-button check (lines 621-650) retries up to 5 times, creating a fresh "Creating" session each time a race causes the prior one to resolve mid-tab-walk before the check completes.
3. Expected: the subsequent Retry-button tab-walk (lines 676-688) reaches the Failed session's Retry button within the fixed 60 `Tab` presses.
4. Actual: it sometimes doesn't — see error above.

## Root Cause

Not fully confirmed — needs investigation. Hypothesis: each Cancel-check retry attempt's "Creating" session card may not be reliably removed from the list before the next attempt (or before the Failed-session check runs), so the number of focusable elements between the search input (where the tab walk starts, line 679) and the target Retry button grows with the number of retries this run happened to need. The tab budget is a hardcoded literal (`for (let i = 0; i &lt; 60; i++)`, line 681) rather than scaled to the actual number of focusable elements in the list at that point, so a run needing several Cancel-check retries can push the Retry button beyond reach.

## Files Likely Affected

- `tests/e2e/accessibility.spec.ts` — the test itself (lines 562-689), specifically the fixed tab-count budget and the Cancel-check retry loop above it.
- Possibly `web-app/src/components/sessions/SessionList.tsx` / `SessionCard.tsx` — if leftover "Creating" session cards from earlier retry attempts aren't cleaned up as expected.

## Fix Approach

Unknown. Candidate directions:
- Scale the tab budget to the actual focusable-element count (query all focusable elements up to the target and tab exactly that many times, or tab until reaching the target with a generous upper bound instead of a fixed 60).
- Ensure each Cancel-check retry attempt's session is deleted/cleaned up before the next attempt or before the Failed-session check runs, so the list length stays bounded.
- Alternatively, target the Retry button directly with `page.keyboard.press('Tab')` in a loop with an early-exit condition rather than a fixed count.

## Verification

Run the `UX Analysis (Axe + Lighthouse + Claude)` job (or `accessibility.spec.ts`) repeatedly (e.g. 10+ times, or with an artificially inflated session list) and confirm the Retry-button check no longer fails due to tab-count exhaustion.

## Related Tasks

Prior related fix: commit `522cd0673` ("fix(async-session-creation): stop the whole session list from hiding on a mutation error, fix flaky Cancel-button a11y test") already touched this same test's Cancel-button flakiness — this bug is a recurrence in the same test, now on the Retry-button side.
