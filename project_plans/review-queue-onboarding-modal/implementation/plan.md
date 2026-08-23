# Implementation Plan: review-queue-onboarding-modal

**Feature**: Close backlog item `5ab8a12e-17a3-441c-bc4c-88be826eb5bf` after a
FAIL verdict, by formally failing the one AC that conflicts with the verified
root causes and re-requesting review — no further code changes.
**Date**: 2026-08-15
**Status**: Ready for implementation (implementation = reporting/tooling
steps only; no source diff)
**ADRs**: None — the conflict and its resolution are fully documented in
`requirements.md`'s "Verdict" section and `research/architecture.md` /
`research/pitfalls.md`; a separate ADR would duplicate that record for a
one-time backlog-closing decision, not a reusable architectural choice.

---

## Domain Glossary

N/A — complexity 1, no new domain types introduced. The relevant vocabulary
(`ReviewItem`, sentinel, `OmnibarCreationPanel`) is pre-existing and defined
in `requirements.md`.

## Pattern Decisions

N/A — complexity 1, no new components or patterns. This plan sequences three
existing MCP tool calls (`report_progress` × 7, `request_review` × 1); no
architecture or design pattern applies.

## Migration Plan

N/A — complexity 1, no schema or data changes.

## Observability Plan

N/A — complexity 1, no new operations or service boundaries.

## Risk Control

N/A — complexity 1, no code shipped by this plan beyond what's already on
`HEAD` (`2811df54e`), which was already verified safe in
`research/pitfalls.md` §1–2 (behaviorally-safe sentinel widening,
equivalent-or-better test coverage).

## Unresolved Questions

- [ ] Whether AC4 should be formally amended (its text corrected to match the
  real root cause) or left as a permanently-fail-marked criterion with an
  accepted exception — blocks nothing in this pass, this is a human review
  decision per `research/pitfalls.md` §3's "no standing rule permits an
  implementing agent to decide this unilaterally" finding. Owner: whoever
  reviews this item's next `request_review` call.

## Dependency Visualization

```
report_progress(AC1, pass) ─┐
report_progress(AC2, pass) ─┤
report_progress(AC3, pass) ─┼──> request_review(message, verification_notes) ──> wait_for_backlog_event(verdict_recorded)
report_progress(AC4, FAIL) ─┤
report_progress(AC5, pass) ─┤
report_progress(AC6, pass) ─┤
report_progress(AC7, pass) ─┘
```
All seven `report_progress` calls are independent of each other (no shared
state, no ordering constraint) and can run in any order before
`request_review`. `request_review` depends on all seven having been called
first — MCP tool descriptions require every criterion to have a recorded
outcome before requesting review (per `research/pitfalls.md` §3).

---

## Phase 1: Close out the backlog item's second review cycle

### Epic 1.1: Record per-criterion outcomes with evidence

**Goal**: every acceptance criterion has an explicit, evidenced disposition
(`pass` or `fail`) recorded via `report_progress` — none silently skipped,
matching the precedent at `project_plans/launchd-shell-sourcing/implementation/validation.md:11-13`.

#### Story 1.1.1: Report the six satisfiable criteria as pass
**As a** backlog-automation reviewer, **I want** each satisfiable AC marked
`pass` with concrete evidence, **so that** the reviewer can verify each claim
against a citation instead of trusting a bare assertion.

**Acceptance Criteria**:
- AC1 (dismissal helper called on every `goto()`) marked pass, citing
  `tests/e2e/review-queue.spec.ts` lines showing `dismissOnboardingIfPresent(page)`
  after each `page.goto(...)`.
  - *Given* the current `HEAD` state of `review-queue.spec.ts`, *When*
    `report_progress(item_id, criteria_index=0, status="pass")` is called,
    *Then* the note cites the specific line ranges where the helper is
    invoked (e.g. lines 27, 39, 65, 74, 84, 118, 132, 155).
- AC2 (0 failed across 22 instances) marked pass, citing the exact
  `--reporter=list` run output.
  - *Given* a clean run of `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list`,
    *When* the run completes, *Then* it must report `0 failed` with
    `14 passed` / `8 skipped` (matching requirements.md's re-verified
    numbers) before `report_progress(criteria_index=1, status="pass")` is
    called with that exact count in the note.
- AC3 (deterministic under `--retries=0`) marked pass, citing a second run
  with that flag.
  - *Given* the same spec file, *When*
    `npx playwright test review-queue.spec.ts --reporter=list --retries=0`
    is run, *Then* it must also report `0 failed` before
    `report_progress(criteria_index=2, status="pass")` is called.
- AC5 (shared `OnboardingPage.ts` helper, plain function) marked pass, citing
  `tests/e2e/pages/OnboardingPage.ts`'s export and both spec files' imports
  of it.
- AC6 (Session Creation Flow tests get the dismissal step) marked pass,
  citing the two tests in that `describe` block calling
  `dismissOnboardingIfPresent`.
- AC7 (`escalation-reasoning.spec.ts` matches its pre-migration pass count)
  marked pass, citing a run of that spec file's `--reporter=list` output.

**Files**: none changed — this story is entirely `report_progress` MCP tool
calls informed by re-running existing commands.

##### Task 1.1.1a: Re-run both spec files to capture fresh evidence (~3 min)
- Run `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list`
- Run `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list --retries=0`
- Run `cd tests/e2e && npx playwright test escalation-reasoning.spec.ts --reporter=list`
- For AC7, additionally diff `escalation-reasoning.spec.ts` at `HEAD` against
  its pre-migration source: `git diff 303d43fad:tests/e2e/escalation-reasoning.spec.ts HEAD:tests/e2e/escalation-reasoning.spec.ts`
  (`303d43fad` = the upstream `main` commit, PR #315, that first added this
  file — this branch predates it, so the file was added fresh in `2811df54e`
  rather than modified). The delta must be limited to the intended
  dismissal-helper swap; a single fresh run's pass count alone is not
  sufficient evidence for "matches its pre-migration pass count."
- **Contingency**: if any of the three commands above does not reproduce
  0 failed (review-queue.spec.ts, both invocations) or a fresh count with no
  new failures relative to the diff-based baseline (escalation-reasoning.spec.ts),
  **stop** — do not mark AC2/AC3/AC7 `pass`, do not proceed to Task 1.1.1b or
  1.1.2a using requirements.md's stale numbers as if they were this task's own
  evidence. Re-open root-cause investigation for whatever regressed (possible
  causes: environment drift, or an interaction with the `chore(sdd)` commits
  layered on top since 2026-08-03 — `cb82049a5`, `85ddf26e9`). This
  contingency exists because every "pass" disposition below and the whole
  "keep the fix, fail only AC4" strategy rests on these numbers holding.
- Files: none (read-only verification)

##### Task 1.1.1b: Call report_progress for AC1, AC2, AC3, AC5, AC6, AC7 (~3 min)
- `criteria_index` 0, 1, 2, 4, 5, 6 respectively, `status="pass"`, each `note`
  citing the file:line or command output backing it. Re-verify exact line
  numbers at call time rather than reusing this plan's draft citations
  verbatim (they drift as the file changes — e.g. this plan's Story 1.1.1
  draft cited lines 27/39/65/74/84/118/132/155 for AC1's helper calls; at
  plan-writing time `HEAD` actually had them at 26/36/48/72/79/116/127/152 —
  confirm fresh with `grep -n dismissOnboardingIfPresent tests/e2e/review-queue.spec.ts`).
- AC5's note must explicitly name the "plain function, not a class"
  structural requirement, not just cite the import — e.g. "`OnboardingPage.ts`
  exports `async function dismissOnboardingIfPresent(page: Page): Promise<void>`,
  no class, imported by both spec files."
- Files: none (MCP tool calls only)

---

#### Story 1.1.2: Report AC4 as fail, with the full evidentiary chain in the note
**As a** backlog-automation reviewer, **I want** AC4 marked `fail` with a
note pointing to the research that proves it conflicts with AC2, **so that**
I don't waste a review cycle re-deriving what's already been established
twice (this backlog item's first review cycle, and this pass's two research
agents).

**Acceptance Criteria**:
- AC4 marked `fail`, not silently `pass` and not omitted.
  - *Given* `research/architecture.md`'s verdict ("AC4 has no viable
    compliant alternative") and `research/pitfalls.md`'s recommendation
    ("Do not mark AC4 pass"), *When*
    `report_progress(item_id, criteria_index=3, status="fail")` is called,
    *Then* its `note` cites both research files by path and states in one
    sentence why: "onboarding-modal hypothesis disproven live; real root
    causes (empty-queue sentinel gating, dead SessionWizard selectors)
    require exactly the production/test-intent changes AC4 forbids —
    see research/architecture.md, research/pitfalls.md."

**Files**: none changed — `report_progress` call only.

##### Task 1.1.2a: Call report_progress for AC4 (~1 min)
- `criteria_index=3`, `status="fail"`, note per above.
- Files: none (MCP tool call only)

**Indexing note**: the backlog item's own "Latest Review Verdict" text
(from the prior review cycle) labels the failed criterion "AC3" / "Criterion
2 (FAIL)," but the reasoning quoted there ("modified `ReviewQueuePanel.tsx`...
rewrote assertions/selectors... explicitly prohibited by AC3's literal text")
is verbatim AC4's text (item's 4th listed criterion, `criteria_index=3`), not
AC3's ("`--retries=0` re-run also reports 0 failed"). This looks like an
off-by-one in the prior verdict's own labeling, not a different criterion
actually failing. This plan re-evidences all 7 indices fresh regardless (so
the practical risk is neutralized), but `request_review`'s `message` should
include one reconciling sentence so a reviewer whose mental model says "I
failed AC3" isn't confused by this pass reporting "AC4 fail."

---

### Epic 1.2: Request review with the conflict surfaced up front

**Goal**: the human/reviewer sees the one sentence that matters
(AC4-vs-AC2 conflict + recommendation) before any supporting detail, per
`research/pitfalls.md`'s explicit recommendation on `message` vs.
`verification_notes` structuring.

#### Story 1.2.1: Call request_review with a two-tier evidence structure
**As a** backlog-automation reviewer, **I want** `request_review`'s `message`
field to lead with the AC4 conflict and its recommendation, and
`verification_notes` to carry the full citation trail, **so that** a
time-constrained human reviewer gets the decision they need to make in the
first sentence, with full backup available if they want it.

**Acceptance Criteria**:
- Before calling `request_review`, run `sdd:6-verify` per
  `.claude/commands/backlog/review.md`'s mandatory pre-review gate ("Resolve
  every BLOCKER and REFACTOR finding it surfaces. Only continue once it
  reports PASS, or you have deliberately accepted and documented any
  remaining CONCERNS."). For the unchanged-since-2026-08-03 diff on `HEAD`,
  this gate was already run once and came back CLEAN
  (`project_plans/review-queue-e2e-onboarding/implementation/architecture-review.md`,
  `.../adversarial-review.md`) — re-run it this pass rather than relying
  solely on that citation, since the pipeline instructions for this session
  call for it explicitly; if it reproduces CLEAN/PASS, cite both the fresh
  run and the prior one in `verification_notes`.
- `request_review(item_id, message, verification_notes)` is called exactly
  once after all seven `report_progress` calls in Epic 1.1 complete and
  `sdd:6-verify` has reported PASS (or documented CONCERNS have been
  accepted).
  - *Given* all seven criteria have recorded outcomes (six `pass`, one
    `fail`), *When* `request_review` is called, *Then* `message` opens with:
    "AC4 cannot be satisfied together with AC2 given the real, verified root
    causes (production-code sentinel bug + dead-component selectors) —
    recommend accepting the fix as-is or explicitly amending AC4," followed
    by a 1-sentence summary of what was built/kept, *and* `verification_notes`
    contains: the AC2/AC3 test-run evidence, the AC4 evidentiary chain
    (both root causes with file:line citations from requirements.md), and
    the "no test-only alternative exists" argument for both bugs from
    `research/architecture.md`.
- After `request_review`, call
  `wait_for_backlog_event(item_id, event_type="verdict_recorded")` rather
  than ending the session or polling — per this item's own workflow
  instructions (already established, not new to this plan).

**Files**: none changed — MCP tool calls only.

##### Task 1.2.1a: Call request_review (~2 min)
- Compose `message` and `verification_notes` per the Given-When-Then above.
- **Fallback if `request_review` errors or behaves unexpectedly** (e.g.
  rejects the call outright, or silently coerces AC4's recorded state): its
  own description text ("Call after all acceptance criteria are marked
  pass") is unverified as advisory-only vs. server-enforced — this is the
  first time in this codebase's SDD history two ACs in one item have been
  mutually exclusive (`research/architecture.md` §3), so this exact call
  shape has no precedent to confirm it's accepted cleanly. If it errors,
  do not retry with AC4 silently flipped to `pass` to route around the
  error — that would repeat the original problem. Instead stop and surface
  the tool error to the human via whatever channel is available (session
  notes, a comment on the backlog item if the MCP surface allows one), since
  no `report_progress`/`request_review` alternative exists for this
  situation.
- Files: none (MCP tool call only)

##### Task 1.2.1b: Call wait_for_backlog_event and handle the verdict (~1 min, blocking)
- `PASS` → proceed to `/backlog/ship`.
- `FAIL`/`PARTIAL` on a *different* criterion than AC4 → fix that specific
  gap and repeat Epic 1.1/1.2 for the affected criterion only (do not
  re-open the AC4 question — already settled by two research passes this
  session).
- `FAIL`/`PARTIAL` citing AC4 again with no new information → per
  `.claude/commands/backlog/review.md`, up to 3 `/backlog/review` cycles are
  allowed *within this session* before shipping to a human regardless. This
  session has not yet called `request_review`, so Task 1.2.1a's call is this
  session's **1st** cycle, not its 2nd (a prior FAIL exists on this backlog
  item, but it came from an earlier, different session — `review.md`'s
  counter is explicitly per-session, not per-item). If this session's own
  count reaches 3 without PASS, run `/backlog/ship` anyway per that doc's
  explicit instruction, handing the AC4 disposition to a human via the PR.
- Files: none (MCP tool call + branching on its result)

---

## Out of scope (explicitly, per requirements.md's verdict)

- **No changes to `web-app/src/components/sessions/ReviewQueuePanel.tsx`.**
  Already fixed and verified safe (`research/pitfalls.md` §1). Reopening
  this would repeat exactly the work two research agents already
  concluded is unnecessary.
- **No changes to `tests/e2e/review-queue.spec.ts` or
  `tests/e2e/escalation-reasoning.spec.ts` assertions/selectors.** Already
  correct and verified equivalent-or-better coverage
  (`research/pitfalls.md` §2).
- **No new server-side seed endpoint for the review queue.** Considered and
  rejected in `research/architecture.md` §1 — would itself be production
  code, still forbidden by AC4, and wouldn't fix the underlying sentinel
  defect for other consumers.
