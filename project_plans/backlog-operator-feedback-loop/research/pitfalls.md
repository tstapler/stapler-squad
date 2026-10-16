# Pitfalls Research: backlog-operator-feedback-loop

Agent 4 (Pitfalls), SDD Phase 2. Covers known risks for the three gaps, carrying
forward relevant prior art from the abandoned `plan-approval-ux` project
(orphaned commit `bc0955d41` on branch `recover/plan-approval-ux`, never merged
— **111 commits stale, and confirmed the backend-only half of that feature was
never even merged**: `session/ent/schema/backlog_item.go` today has
`plan_approved`/`plan_approved_at` but no `plan_rejection_reason` /
`plan_rejected_at` / `plan_artifacts_set_at` field. Nothing from that branch
landed; treat its plan/reviews as a design reference, not as already-shipped
groundwork).

## Why the plan-approval-ux branch likely stalled

No explicit "abandoned because X" note exists anywhere in the project's
artifacts. What the pre-mortem/adversarial-review/architecture-review trail
does show: by the end of Phase 4 (validation), reviewers had found and the
plan had patched **5 blockers** (2 in adversarial-review.md, 3 in
architecture-review.md — the `PlanRejectionReason` clearing gap, the
`checkPlanArtifactFreshness` fail-open bug, the `MarkStuck` hardcoded-status
dead-code bug, the `selfHealStuck` immediate-resolve bug, and the
`RejectPlan`/`ApprovePlan` asymmetry) — all marked `[x] RESOLVED` in
pre-mortem.md/adversarial-review.md before implementation started. The single
commit that exists (`bc0955d41`) is annotated "Backend half of
plan-approval-ux" — i.e. implementation got through the Go/proto/ent side and
stopped before the three React surfaces (Epic 5/6/7 — `PlanVerdictBox`,
`ActionsSection` wiring, `PlanArtifactsSection`) were built, then the branch
went stale (111 commits behind) and was never rebased/finished. **Inferred**,
not confirmed: this looks like ordinary stall-and-bitrot (backend landed,
frontend half never got picked back up before `main` moved on), not a
rejected design — the design itself (ADR-001/ADR-002, the reviewed plan) holds
up and is reusable. Do not assume the design was abandoned *because* it was
wrong; the reviews found real bugs and the plan fixed them before this stalled.

## Carried-forward risks still relevant to Gap 3 (Request Changes)

These are from `plan-approval-ux`'s pre-mortem/adversarial-review/
architecture-review and apply directly to this project's Gap 3, which is
functionally the same feature (`RejectPlan`/"Request Changes" + a
`changes_requested`-shaped status). Re-verified against current HEAD, not
taken on faith:

1. **Symmetry gap: Reject must clear Approve, and vice versa on regeneration
   (architecture-review.md Blocker, P1).** If `RequestChanges`/`RejectPlan`
   only sets a rejection reason and never resets `plan_approved`, a plan that
   was approved-then-rejected leaves `plan_approved=true` AND a non-empty
   rejection reason simultaneously. Any backend gate that checks
   `item.PlanApproved` directly (this project's equivalent of
   `backlog_service_triage.go:438,656`) will let a session spawn against a
   plan the UI displays as rejected. **Design against this explicitly**: the
   write that records "changes requested" must, in the same
   `session.BacklogItemUpdate` call, also clear `PlanApproved`. Do not treat
   this as "shouldn't happen" defensive code — the previous review proved a
   normal user flow (approve, notice a problem, reject) produces it.

2. **The regeneration-completion write must clear the rejection reason
   (adversarial-review.md Blocker, resolved via a task in the old plan).**
   Symmetric to #1: when the triage/plan-regeneration path (this project's
   feedback-driven re-triage, AC2) completes and produces a fresh, unreviewed
   plan, the completion write must clear the rejection state (reason +
   timestamp), or the freshly-regenerated plan is mislabeled "changes
   requested" with stale text from the *previous* plan the operator never saw.
   This is functionally identical to the Gap 1 "stale question" risk below —
   same shape of bug (an artifact from run N surviving into run N+1's display),
   different field.

3. **Empty-reason validation must be a real precondition, not a client-side
   nicety (this project's AC4, explicit).** The old plan's `RejectPlan`
   required a non-empty reason at the RPC boundary. Confirm the new
   `RequestChanges`/equivalent RPC does the same server-side — a UI-only
   "reason required" check is bypassable by any other RPC caller (MCP tool,
   future automation) and produces an unreviewable "changes requested" state
   with no actionable text, defeating AC4's own wording ("an empty rejection
   is not possible").

4. **Two-tab / concurrent Approve-vs-Reject race is real and was left
   explicitly undecided, not fixed (adversarial-review.md Concern, never
   resolved to a P1/P2 fix in the old plan — only documented).** Neither
   `ApprovePlan` (today's handler, `server/services/backlog_service_lifecycle.go:742-786`)
   nor the old `RejectPlan` design does compare-and-swap on the `BacklogItem`
   row itself — only on the plan *file's* mtime via
   `expected_modified_at_unix_ms`. Two tabs open on the same item, one clicks
   Approve and the other clicks Reject in close succession: both freshness
   checks pass (neither touches `plan.md`'s mtime), and the second RPC's write
   silently wins with no conflict surfaced to either tab. This project's AC3/
   AC5 don't call out concurrent-click handling. **Decide explicitly** (not by
   omission) whether to: (a) accept this as an out-of-scope solo-operator race
   (single human, sequential single-tab use is the assumed model) and say so
   in requirements/plan, or (b) add a lightweight guard (e.g. include the
   item's own `UpdatedAt` in the request, `FailedPrecondition` on mismatch).
   Given this project's target user is the same solo-operator persona as
   plan-approval-ux, defaulting to (a) with an explicit statement is
   reasonable — but it must be a stated decision, not a silent gap the next
   reviewer has to rediscover.

5. **Naming collision: `changes_requested` already means something different
   in this codebase — GitHub PR review state, not backlog plan review.**
   `web-app/src/lib/vcs/mergeability.ts:9,28` defines a `MergeabilityStatus`
   union that includes the literal string `"changes_requested"` (driven by
   `data.github.changesReqCount > 0`), rendered via `MergeabilityPill.tsx`, and
   `session/stuck_decisions.go:152` reads `info.ChangesRequestedCount`. A
   backlog item detail view can plausibly show *both* a PR mergeability pill
   (existing) and a plan-review status badge (this project) on the same page
   once a PR exists for the item. If Gap 3's new backlog-item-level status
   also renders the string "Changes requested" (or a `changes_requested`
   value in a differently-scoped enum/prop), an operator glancing at the page
   cannot tell at sight which of two unrelated things ("your PR has
   reviewer-requested changes on GitHub" vs. "you rejected the agent's plan
   and asked it to revise") is being reported. **Design against this**: pick
   visually and lexically distinct copy for the plan-review state (e.g. "Plan:
   Revise requested" / a distinct badge color/icon), and if a shared frontend
   union type is tempting for either field, keep them as two separate,
   non-overlapping types rather than merging into one generic
   `"changes_requested"` value used for two different domains.

6. **No canonical derivation function — same class of bug the old
   architecture-review flagged as root cause (architecture-review.md
   Concern).** The old plan's core defect (item #1 above) existed *because*
   there was no single `DerivePlanReviewStatus`-equivalent function that every
   consumer (UI badge, backend spawn gate, MCP prompt injection) called — each
   checked raw fields ad hoc and drifted. If this project reuses the "derived,
   not stored" pattern (a `changes_requested`-vs-`approved`-vs-`pending`
   status computed from `plan_approved` + a rejection-reason field, rather
   than a first-class status enum value), write one function (Go-side, and
   its TS mirror) from day one and route every consumer through it — including
   the stuck-item reconciler (see next section) — rather than letting each
   write site independently decide what "changes requested" means.

## Gap 3 — stuck-item reconciler risk (new, not in the old plan)

`session/backlog_remediation.go` (referenced by
`project_plans/backlog-agent-communication/requirements.md`) and
`session/backlog_lifecycle.go`'s `reconcilePlanNotApprovedItems` (current
HEAD, `session/backlog_lifecycle.go:1282-1341`) are today's stuck-item
reconciliation path. Verified against current code (not the old plan):

- `reconcilePlanNotApprovedItems` **still hardcodes `BacklogStatusQueued`** as
  the `expectedStatus` argument to `MarkStuck` (`session/backlog_lifecycle.go:1314`).
  `MarkStuck`'s precondition (`current.Status != string(expectedStatus)`)
  silently no-ops (`applied=false`, no error, no log) for any item whose
  status isn't exactly `queued`. **This is the exact trap the old
  architecture-review already caught once** (its Blocker #1, re: widening to
  `ready`-status items) — a live, currently-unfixed pattern in this codebase,
  not a hypothetical.
- If Gap 3 introduces a `changes_requested` status (or a derived-equivalent)
  and a backlog item can sit in that state indefinitely without an operator
  ever clicking "Regenerate" (pre-mortem.md Failure #4's exact scenario —
  "reject and walk away for a week" — carries over unchanged to this project),
  the natural next step is to teach the stuck reconciler to detect and
  surface stale `changes_requested` items. If that reconciliation work is
  added, it **will hit the same `MarkStuck`-hardcoded-status bug** unless the
  call site is updated to pass the item's actual current status rather than a
  hardcoded constant. Flag this explicitly in the Phase 3 plan if reconciler
  coverage for `changes_requested` is in scope; do not assume "we'll wire the
  reconciler" is a small follow-on — the adjacent bug is proven to be easy to
  reintroduce.
- No requirement in this project's AC list currently calls for stuck-item
  detection of a stale `changes_requested` item — AC5 only requires the state
  be *visible*, not that it be *escalated* if ignored. Recommend treating
  reconciler coverage as explicitly out-of-scope for this pass (mirroring
  pre-mortem Failure #4's unresolved P2) rather than silently assumed-covered.

## Gap 1 — triage question → answer → re-triage wiring

1. **Stale question text if a new triage run supersedes it before the
   operator answers.** `submit_triage_result`'s `suggestions` array (with
   `rationale: "question"`, `server/mcp/tools_backlog.go:~1921`) is
   fully replaced on each triage completion — verified structurally: nothing
   in the current schema tracks question identity across runs (no per-question
   ID, no answered/resolved flag; this matches requirements.md's own
   open question 2, "should answered questions carry persistent state" —
   currently no). Concretely: triage run N asks "should this support X?",
   operator is mid-typing an answer, but run N is superseded (operator
   clicked "Refine" again, or a re-triage was independently triggered) before
   they submit — run N+1 completes and overwrites the `suggestions` array
   with a *different* question set. If the UI held a reference to "the
   question the operator is answering" by index or by the old array's
   identity rather than by content, the answer could get silently attached to
   the wrong (new) question, or submitted against a question no longer being
   asked. **Design against this**: key the in-flight answer form to the
   question's literal text (or a hash of it) at render time, and if the
   `suggestions` array changes underneath the open form (poll/refetch
   detects a newer triage result), invalidate the form and show a "this
   question was superseded by a new triage run" notice rather than silently
   accepting a stale-context answer.

2. **Race with an in-flight triage run — `triageInFlight` sync.Map guard
   (verified live at `server/services/backlog_service_triage.go:2078,2270,2372`,
   matches ADR-002's description of the mechanism exactly).**
   `TriggerTriage` does `LoadOrStore(itemID, struct{}{})` and rejects a second
   concurrent call for the same item while one is in flight
   (`alreadyInFlight` branch at line 2270). If "submit answer → triggers
   re-triage" (AC2) calls the same `TriggerTriage(itemId, feedback)` RPC while
   a previous triage/re-triage for that item is still running (the LLM call
   can run up to `triageCallBudget = 30 * time.Minute`), the operator's
   answer-triggered re-triage will be **rejected outright**, not queued.
   **Design against this explicitly**: the answer-submission handler must
   surface this as a distinct, actionable error ("triage is still running for
   this item — your answer will be used on submit, try again once it
   completes" or similar), not a generic RPC failure the operator has to
   interpret. Do not silently drop the answer or retry-loop client-side
   without operator visibility — both hide that the click had no effect.

3. **`suggestions` is a flat array with no question ID — submitting "an
   answer to a specific question" (AC1) requires inventing an identity
   scheme, and whatever scheme is chosen must survive the array being
   entirely replaced on every triage run (see #1).** Because there is no
   stable question ID today, "answer to *this* question" almost certainly has
   to be implemented as "send the question's text back to the model
   verbatim, plus the answer" (i.e., feedback text like
   `Q: <question text>\nA: <answer text>`) rather than an ID-based
   reference — which is exactly the shape `TriggerTriageRequest.feedback`
   already accepts (out of scope to change, per requirements.md). Confirm the
   Phase 3 plan settles on echoing the question text into the feedback
   payload rather than trying to introduce IDs, since the out-of-scope note
   forbids changing `feedback` semantics.

## Gap 2 — steering a backlog-linked session from item detail

**This is the highest-severity, most concrete finding in this research pass**
— it directly resolves requirements.md's open question 3, and the answer is
unfavorable to AC6/AC7 as currently worded for two of the three session kinds
named ("triage, review, or work session").

Verified via code reading (`server/services/backlog_service_triage.go`,
`session/headless/caller.go:487`, `server/mcp/tools_terminal.go:627-706`):

- **Headless triage and review sessions are not `steer_session`-reachable at
  all, structurally, not as an edge case.** Headless triage/review runs go
  through `session.headless.Pool.CallBlocking` — a single synchronous
  subprocess call to the `claude` CLI with no PTY, no tmux session, and no
  `session.Instance` registered anywhere `steer_session` can find one. The
  synthetic UUID assigned to these runs
  (`headlessTriageUUIDPrefix + uuid.New().String()`,
  `server/services/backlog_service_triage.go:2344`, and the equivalent
  `headlessReReviewUUIDPrefix`) is stored only in an `ItemSession` record for
  UI display and orphan-detection bookkeeping — it is **not** a session ID
  that `steer_session`'s `findInstance` (`server/mcp/tools_terminal.go:637-651`)
  can resolve. `findInstance` checks `th.live.FindLiveInstance(sessionID)`
  then falls back to scanning `th.store.LoadInstances()` for a
  `session.Instance` whose ID matches — headless runs never create either.
  Calling `steer_session` with a headless triage/review session's ID will
  return `ErrSessionNotFound` ("session not found. Use list_sessions to find
  available sessions") — a real, immediate, always-reproducible failure, not
  a probabilistic race.
- **`work` sessions (`SessionTypeDirectory`/`SessionTypeNewWorktree`/etc.,
  actual tmux-backed `Instance`s) are genuinely steerable** — this is the
  path `steer_session` already works for from the general session list, and
  nothing about being backlog-linked changes that. Gap 2's affordance will
  work correctly for the "work session" case.
- **Design against this explicitly, don't discover it in implementation**:
  AC6's "triage, review, or work session" wording should be revisited in
  Phase 3. Options: (a) narrow AC6 to work sessions only (triage/review are
  structurally non-interactive — there is nothing mid-run to steer since the
  LLM call is a single blocking round-trip, and the operator's actual lever
  for triage is "submit an answer to be used on the *next* run," which is
  Gap 1, not Gap 2); (b) if the UI must still render a Steer control for a
  headless-attached item, it must detect the `headless*UUIDPrefix` shape (or
  equivalent server-exposed session-kind field) and disable/hide the Steer
  action with an explanatory tooltip rather than exposing a button that
  always errors. Shipping a Steer button that silently 404s for two of the
  three named session kinds is the exact "affordance that has no effect"
  failure requirements.md's Feasibility Risks section already flags as an
  open question — this research confirms it's not just "unverified," it's
  reproducible as designed. AC7 ("no parallel steering implementation") is
  satisfiable either way — the fix is scoping/UI-gating, not a new steering
  mechanism.
- **Secondary risk, work sessions**: a work session already terminated or
  archived by the time the operator clicks Steer. `findInstance` falls back
  to `LoadInstances()` (all sessions in storage, not just live ones) —
  confirm whether a genuinely `Stopped`/archived session's ID still resolves
  there and produces a *meaningful* error (e.g. "session has ended") versus a
  generic "not found," since AC6 implies the operator is looking at a session
  reference that may be stale relative to the backlog item's current state
  (the item detail view's session list may not live-refresh session status).

## Priority ranking for Phase 3 plan

- **P1 — must design against**: Gap 2's headless-session steer-is-a-no-op
  finding (AC6 as worded overclaims for triage/review). Gap 3's
  Approve/Reject symmetry (#1 in carried-forward list) and empty-reason
  validation (#3) — both proven failure modes in the prior design, cheap to
  prevent by construction.
- **P2 — decide and document, don't silently omit**: Gap 3's two-tab race
  (#4) and `changes_requested`/PR-mergeability naming collision (#5); Gap 1's
  in-flight-triage race UX (#2); Gap 3 reconciler scope (explicitly out of
  scope recommended, but say so).
- **P3 — lower stakes, note for completeness**: Gap 1's stale-question-form
  handling (#1) — real but low blast radius (worst case: operator resubmits);
  Gap 3's canonical-derivation-function discipline (#6) — good hygiene,
  not a correctness blocker if the two known write sites are done right the
  first time.
