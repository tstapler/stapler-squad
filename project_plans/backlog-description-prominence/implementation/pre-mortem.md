# Pre-mortem: backlog-description-prominence
**Date**: 2026-08-02

## Method

Read `plan.md`, `requirements.md`, and `implementation/adversarial-review.md` in full.
Independently verified the facts behind each failure mode rather than asserting them:
read `DescriptionSection.tsx`, `useSectionExpandState.ts`, `BacklogItemDetail.markdown.test.tsx`
(confirmed it renders the *real* `BacklogItemDetail` — not the isolated `DescriptionSection` —
so it is an effective, if incidental, integration-level guard against a partial seed/prop
implementation), `tests/e2e/backlog-item-detail-redesign.spec.ts`, and grepped every
`.github/workflows/*.yml` for `playwright test` invocations to determine, as fact rather
than assumption, whether the rewritten e2e spec is executed by any CI workflow (it is not —
see Failure #1).

## Failure Modes

| # | Failure | First Symptom | Prevention | Severity |
|---|---------|--------------|------------|----------|
| 1 | The rewritten `tests/e2e/backlog-item-detail-redesign.spec.ts` (Task 1.1.2c) is never executed by anything — not Task 1.1.3a's verification gate (only runs Jest + `make quick-check`, neither touches Playwright), and not any CI workflow: `e2e-video.yml`'s `FEATURE_SPECS` list explicitly excludes this file, `demo-publish.yml` and `ux-analysis.yml` don't include it either, and even the one workflow that *would* run an unlisted spec (`demo-publish.yml`) pipes failures through `\|\| true`. AC3 and AC5 — the two ACs this spec exists to prove — ship with zero executed verification, on a spec already flagged pre-change as "never run against a live instance." | None in CI — this is a silent gap, not an observable failure. Would only surface via a manual `npx playwright test` run, or much later if a future refactor breaks the spec and nothing red ever shows up. | Add a task to Story 1.1.3 that starts an isolated `e2e-local` instance per `CLAUDE.md`'s E2E Tests section and runs `cd tests/e2e && npx playwright test backlog-item-detail-redesign.spec.ts` before shipping, per the adversarial review's own recommendation (currently unaddressed in plan.md). Consider also adding this spec to a CI-gating list so it stays a standing check, not a one-time manual run. | P2 |
| 2 | Story 1.1.1's three tasks (seed flip, prop threading, call-site update) are documented as an atomic unit, but the only thing that would actually catch a *future* desync between the seed value and the threaded prop is `BacklogItemDetail.markdown.test.tsx`'s integration-style render — an incidental side effect of Task 1.1.2b's rewrite, not a test written to guard this invariant on purpose. `DescriptionSection.test.tsx`'s standalone tests hardcode `defaultExpanded` explicitly and would stay green regardless of what the real seed value is. A future editor touching only `BacklogItemDetail.tsx`'s seed (or only `DescriptionSection`'s prop plumbing) could silently reintroduce the exact "compiles but is a no-op" bug this plan is fixing, without realizing the one guard rail is an unlabeled side effect elsewhere. | `BacklogItemDetail.markdown.test.tsx` failing at a `findByTestId("backlog-description-rendered")` timeout — but only if that specific test/assertion still exists; a later refactor could remove it without anyone noticing it was load-bearing for this invariant. | Add a one-line comment in `BacklogItemDetail.markdown.test.tsx` near `beforeEach`/the render calls noting that this suite is the only place that verifies the real seed value and the threaded prop stay in sync end-to-end, so it isn't accidentally weakened by someone who believes `DescriptionSection.test.tsx` already covers it. | P3 |
| 3 | Task 1.1.1b's proposed docstring text ("`defaultExpanded` here mirrors the parent's controlled expand state") is itself imprecise relative to the plan's own Domain Glossary, which establishes that `defaultExpanded` only seeds *initial* state and is architecturally dead once mounted inside a `CollapsibleGroup` — it does not "mirror" anything continuously. The docstring rewrite intended to fix a stale/inaccurate docstring risks introducing a smaller, subtler version of the same problem. | A future contributor reading the new docstring assumes `defaultExpanded` is live/reactive and either misuses it in a future refactor of `Collapsible.tsx` or is confused during debugging. | Tighten the docstring snippet in Task 1.1.1b to say `defaultExpanded` seeds the initial `useSectionExpandState`-backed value only (matching the Domain Glossary's own precise "architecturally dead post-mount inside a group" language), not "mirrors." | P3 |
| 4 | No AC or test covers the layout/UX effect of Description now always rendering expanded for every item, including ones with long, multi-paragraph descriptions — this pushes Acceptance Criteria, Notes, and other sections further below the fold on first view, which matters more on mobile viewports (per this project's own mobile+desktop UX convention). The change solves "buries the field I authored" for the common case but was never checked against the long-description case, which the plan explicitly treats as out of scope to second-guess. | A user files a complaint or backlog item within the first month noting the detail view "feels longer" or needs more scrolling, especially on mobile, for items with long descriptions. | No plan.md task currently checks this. Add a lightweight manual pass to Story 1.1.3: view 2-3 real long-description items on the manual-test instance (per `CLAUDE.md`'s non-live-instance testing pattern) at both desktop and mobile widths before shipping, to catch an obviously-bad case early rather than via a later complaint; if it's bad, file a follow-up item rather than blocking this one. | P2 |
| 5 | Task 1.1.3a still frames `BacklogItemDetail.test.tsx`'s roving-tabindex test (~lines 995-1023) as an open risk ("if it fails, investigate") even though both the plan's own research and this pre-mortem's independent re-check of the adversarial review confirm the test only asserts `.focus()`/`.toHaveFocus()`, with zero coupling to expand/collapse state. If it does unexpectedly fail for some other reason, the implementer has no more guidance than "investigate," risking a rushed, under-scrutinized fix bolted on during the verification story instead of being handled with the rigor of planning. | `BacklogItemDetail.test.tsx`'s roving-tabindex test fails during Task 1.1.3a's targeted Jest run. | Update Task 1.1.3a's note to state the "unaffected" claim has already been independently verified twice (by research and by adversarial review) rather than leaving it phrased as an open question — removes ambiguity for whoever executes the task, and signals a genuine failure here would be surprising and worth escalating rather than shrugging off. | P3 |

## P1 Items (address before implementation)

None. Nothing here is both likely and catastrophic for a change this small and trivially
revertible (`git revert`, no schema/proto/backend surface). The two P2 items are real gaps
worth closing before shipping, not blockers to starting implementation.

## Summary

0 P1, 2 P2, 3 P3. Top failure mode: the rewritten e2e spec (`backlog-item-detail-redesign.spec.ts`)
that's supposed to prove AC3 and AC5 is not run by Task 1.1.3a's verification gate nor by any
CI workflow — confirmed by grepping every `.github/workflows/*.yml` for `playwright test`
invocations, none of which include this file — so those two acceptance criteria ship with zero
executed verification unless a manual Playwright run is added to the plan.
