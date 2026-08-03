# Requirements: review-queue-e2e-onboarding

## Complexity

Originally scoped as 1 (quick task) on the assumption the onboarding modal was
the sole cause. **Corrected during implementation (2026-08-03): the onboarding
modal was never the actual cause of any of the 10 failures.** Running the
suite live (not just reading it) surfaced two unrelated, genuine bugs — see
"Root cause (corrected)" below. Fixing them required one small production
change (`ReviewQueuePanel.tsx`) and rewriting two tests whose selectors
targeted a component that no longer renders. Both were explicitly authorized
mid-session after presenting the evidence, superseding the original AC3
("no production code, no test-intent changes").

## Source

Backlog item `5ab8a12e-17a3-441c-bc4c-88be826eb5bf`: "bug: tests/e2e/review-queue.spec.ts — 10/18 tests fail on first-run onboarding modal"

## Problem

`tests/e2e/review-queue.spec.ts` has 10 of 18 tests failing/timing out waiting for
`[data-testid="review-queue-loaded"]` / `[data-testid="review-queue"]`. Confirmed
pre-existing (not caused by any in-flight feature branch) and deterministic
(reproduces with `--workers=1`, single project — not resource-contention flake).

## Root cause: original hypothesis (disproven)

The original theory was that a fresh browser context's first-run
`OnboardingModal` overlay blocks/races `review-queue.spec.ts`'s assertions,
and that adding the same dismissal step `escalation-reasoning.spec.ts` uses
would fix it. **This was tested and disproven**: adding
`dismissOnboardingIfPresent(page)` after every `goto()` did not change the
pass/fail count at all — the same 10 test/project combinations kept failing,
byte-for-byte identical to before the change. In hindsight this makes sense:
Playwright's `toBeVisible()`/`waitForSelector()`/`toBeAttached()` checks test
DOM presence and CSS visibility, not click-actionability — a `z-index`
overlay sitting on top of an element does not make that element "not
visible" to these assertions. The modal could only ever have blocked a
`.click()`, and none of the 10 originally-failing tests fail at a click step.

The dismissal helper (`tests/e2e/pages/OnboardingPage.ts`,
`dismissOnboardingIfPresent`) was still implemented and wired in per AC1/AC4/AC5,
since it's cheap, harmless, and does match `escalation-reasoning.spec.ts`'s
existing pattern (which the DRY-up in AC4 targets) — it's just not what fixed
the 10 failures.

## Root cause (corrected): two independent, pre-existing bugs

**Bug A — `review-queue-loaded` sentinel only rendered when the queue was
non-empty.** `ReviewQueuePanel.tsx`'s render logic was:
```
{loading && items.length === 0 ? (...loading spinner, no sentinel...)
: items.length === 0 ? (...empty-state text, NO sentinel...)
: (<>{!loading && <div data-testid="review-queue-loaded" .../>}...items...</>)}
```
The sentinel — whose own comment says it "confirms ... the loading state
resolved" — only appeared in the third (non-empty) branch. The e2e test
server's queue is empty by default (demo/live seed sessions don't produce
review-queue items), so on an empty queue the sentinel never rendered and
every test waiting on it (`review-queue-loaded sentinel is present after
page renders`, `when queue has items, each carries acknowledge data-testid`,
`acknowledge button removes item from DOM`) timed out — 3 tests × 2 projects
= 6 of the 10 failures. `escalation-reasoning.spec.ts` never hit this because
it always drives the queue to have exactly one item before checking the
sentinel.
**Fix**: moved the sentinel to render unconditionally on `!loading`
(regardless of `items.length`), removing the now-redundant duplicate inside
the non-empty branch. One file, 5 lines changed.

**Bug B — two tests targeted `SessionWizard`, a component that is
`@deprecated` and "no longer rendered".** `session creation wizard has all
steps` and `session creation form has required test IDs` asserted on
`[data-testid="wizard-step-label"]`, `session-title`, `session-path`,
`auto-yes-checkbox`, `create-session-button` — all defined only in
`web-app/src/components/sessions/SessionWizard.tsx`, which carries the
docblock `@deprecated Use the Omnibar (OmnibarContext) for session creation.
This component is no longer rendered.` (added in commit `16f1a17d0`,
"replace SessionWizard modal with Omnibar"). `grep` confirms zero remaining
imports of `SessionWizard` anywhere in `web-app/src/`. `/sessions/new`
actually redirects client-side to `/?new=true`, which calls
`openOmnibar()` with no arguments — opening the Omnibar in **discovery
mode** (an empty "Session source input"), not creation mode directly. Typing
a local path (e.g. `/tmp`) triggers `LocalPathDetector`, which switches the
Omnibar into its creation panel (`OmnibarCreationPanel.tsx`) — a `Session
Type` radiogroup, `Session Name` field, `Working Directory` field, `Create
Session` button — an entirely different UI from the old wizard's 4-step
flow. 2 of the 10 failures.
**Fix**: rewrote both tests to exercise the real current flow: assert the
Omnibar's session-source input appears after the `/sessions/new` redirect,
then (2nd test) type a path and assert the resulting creation panel's actual
fields/roles. No change to what user-facing behavior is being verified
("does session creation work from this entry point"), only to which UI
elements prove it, since the originally-asserted elements no longer exist.

## `escalation-reasoning.spec.ts`: out of scope by design

This branch predates PR #315 (`feat(review-queue): escalation reasoning...`,
on `origin/main` but not in this branch's ancestry — confirmed via
`git merge-base --is-ancestor`), so `tests/e2e/escalation-reasoning.spec.ts`
does not exist here as a tracked file. It was copied in from `origin/main`
solely so AC4's "reused by both spec files" could be verified locally; only
its dismissal-block call site was touched (migrated to
`dismissOnboardingIfPresent(page)`), matching AC4 exactly. Its
`escalation-reason-*` assertions fail in this branch because the
escalation-reasoning feature's production code (`pkg/classifier/escalation.go`,
`ReviewQueuePanel.tsx`'s reason-text rendering) isn't present here at all —
that is unrelated to this bug, owned by a separate active session
(`stapler-squad-escalation-reasoning-r3`), and intentionally left untouched.

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

### AC3 superseded (2026-08-03)

AC3 as originally written ("no production/source code is modified" and "no
selector/test-intent change") assumed the onboarding modal was the whole
story. It was not — see "Root cause (corrected)" above. Given the actual
evidence (both bugs demonstrated live, not guessed), the constraint was
explicitly relaxed mid-session to allow: (a) the minimal `ReviewQueuePanel.tsx`
sentinel fix, and (b) rewriting the two `SessionWizard`-targeting tests'
selectors/assertions to match the real current Omnibar UI. Both are narrowly
scoped to what the corrected root cause required — no other production file
and no other test's assertions were touched. AC2 ("0 failed") is the
criterion that actually matters and is met; AC3 as originally literally
worded is not, and is superseded by this note rather than tracked as failed.

## Non-goals

- Not fixing any backend/session-poller logic — confirmed unrelated (reproduces
  with `session/review_queue_poller.go` reverted).
- Not changing `OnboardingModal` component behavior or when it shows.
- Not adding onboarding-dismissal to unrelated spec files beyond
  `review-queue.spec.ts` (and only extracting a shared helper if that reduces
  duplication cleanly — not a required refactor).
