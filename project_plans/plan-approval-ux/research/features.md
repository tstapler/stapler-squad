# Research: Similar Features, Edge Cases, Unstated Needs

**Phase**: 2 — Research (Agent 2: Features)
**Question**: What similar approve/reject flows exist in this codebase or industry, what
edge cases/failure modes should plan-approval handle, and what does the user need beyond
the explicit requirements?

## 1. The codebase already has a mature "verdict + reason" pattern — `GateVerdictBox`

The single most important finding: **the review-verdict UI (`web-app/src/components/backlog/GateVerdictBox.tsx`)
is a fully-built, battle-tested reference implementation of exactly the interaction plan-approval
needs.** It is not a rough analog — it is the pattern to copy, because it already solves the
"solo reviewer approve/reject-with-reason" problem for a sibling gate (the review gate, not the
plan gate).

What it does, concretely:
- **Verdict card** with 5 states (`PASS`/`PARTIAL`/`FAIL`/`PENDING`/`UNVERIFIABLE`), each with a
  distinct icon/color/label (`VERDICT_CONFIG` map) — a persistent, glanceable status indicator
  (directly answers requirements.md Success Criterion 1).
- **`readOnly` variant** (`GateVerdictBoxReadOnlyProps`) that renders the same card and
  per-criterion outcome list but omits every action button/form — used for historical
  Headless-Diagnostic-Session records. This is the precedent for "approved/rejected state must
  remain visible and inspectable after the fact," not just as a transient toast.
- **"Reopen for Revision" form**: a `<textarea>` for free-text feedback ("What should the agent
  fix or improve?"), Cancel/Submit buttons, focus management (`useEffect` autofocus on open),
  `Escape` to close. This is the direct precedent for a plan-reject-with-reason form — same
  shape, same UX, same accessibility handling (`role="form"`, `aria-label`).
- **"Override" form**: same shape but with a **required** reason (`MIN_OVERRIDE_REASON_LENGTH =
  5`, submit button `aria-disabled`/`disabled` until met) — precedent for validating that a
  rejection reason isn't empty, if the plan design wants to require one.
- **Keyboard shortcuts**: `Ctrl+Enter` triggers the primary action (approve on PASS, opens reopen
  form on PARTIAL/FAIL) via a `handleKeyDown` on the section root.
- **`role="status" aria-live="polite" aria-atomic="true"`** on the whole section — verdict
  changes are announced to screen readers automatically.
- Danger-styled "Skip Gate" escape hatch with a confirmation dialog (`role="alertdialog"
  aria-modal="true"`) and full focus-trap keyboard handling (Tab cycling, Escape-to-cancel) —
  precedent for how to handle a destructive/gate-bypassing action safely.

**Recommendation for the plan phase**: design the plan-approval UI as a `PlanVerdictBox` (or
extend `GateVerdictBox` to a shared component) with states `NO_PLAN` / `PENDING_REVIEW` /
`APPROVED` / `CHANGES_REQUESTED`, reusing the reopen-form-with-feedback-textarea pattern for
"Request Changes" and the readOnly-card pattern for after-the-fact visibility. This directly
satisfies Success Criteria 1 and 3 in requirements.md with a proven, accessible, already-tested
component shape rather than inventing a new one.

## 2. The backend already has two overlapping "feedback" channels — pick the right one

- `TriggerTriageRequest.feedback` (proto `backlog.proto:432-435`) — free-text field that
  "requests a refinement of the item's most recent [triage result]." Used today for
  re-triaging. This is the mechanism requirements.md points to as the pattern plan-rejection
  should mirror.
- `OverrideVerdictRequest.override_reason` (proto `backlog.proto:458-464`) — free text, but this
  is for *overriding* a review verdict (force-done), the opposite polarity from
  reject/request-changes. Not the right model for plan rejection — it's a "force past the gate"
  action, not a "send feedback for another pass" action.
- The **reopen** flow in `GateVerdictBox` (`onReopen(feedback: string)`) is the correct polarity
  match: feedback text that triggers *another* generation pass, not an override.

**Architecture implication (flagging for Agent 3/Architecture research, not deciding here)**:
`ApprovePlanRequest` currently only has `item_id` (proto `backlog.proto:442-444`) — no
`approved: bool` and no feedback field. Two viable shapes:
  (a) reuse `TriggerTriageRequest.feedback` — i.e., "Request Changes" just calls `TriggerTriage`
      with feedback text, no new RPC, and plan-rejection has no state of its own beyond
      `planApproved` staying `false`;
  (b) add a `RejectPlanRequest { item_id, feedback }` RPC that stores the rejection reason
      distinctly (so it shows in history/timeline) and *then* triggers re-triage.
  Option (b) is more consistent with `GateVerdictBox`'s reopen pattern (which is a single action
  that both records feedback and re-triggers work) and gives a home for a persisted "changes
  requested, reason: ..." record the UI can show — recommend evaluating (b) but note (a) is the
  cheaper MVP if the plan phase wants to size down.

## 3. `submit_review_verdict` / `request_review` MCP tools — the agent-facing mirror

`server/mcp/tools_backlog.go` implements the agent side of the review gate: `request_review`
(work role signals done) and `submit_review_verdict` (review role submits PASS/FAIL/PARTIAL per
criterion). Two points relevant to plan-approval design:

- The **role-aware prompt injection** in `getBacklogItem` (`tools_backlog.go:190-228`) tailors
  the MCP tool guidance shown to an agent based on its session role (`triage`/`work`/`review`).
  If plan-approval feedback needs to reach a *triage* session specifically, the equivalent
  "## Your Role: Triage" block should be extended to mention the plan-rejection feedback the
  same way `get_backlog_item` already surfaces "## Latest Review Verdict" to a `work` session
  (`tools_backlog.go:166-188`) — i.e., there's a ready-made injection point for "## Plan Rejection
  Feedback" text if the plan phase chooses RPC option (b) above.
- `ReviewTrigger` interface (`tools_backlog.go:81-87`) lets `request_review` spawn the review
  gate immediately instead of waiting up to 60s for the next reconcile tick. If plan-rejection
  needs to kick off a fresh triage session immediately (rather than waiting for a poll cycle),
  the same "trigger immediately, don't wait for reconcile" pattern applies.

## 4. The plan-approval gate is *not* as narrow as requirements.md states — verify before scoping

requirements.md claims (Problem #2) that `StuckReasonPlanNotApproved` "only blocks
`DequeueNextQueuedItems`... An item not going through the queue... proceeds regardless of
`planApproved`." Direct inspection of `server/services/backlog_service_triage.go` shows this is
**only half true**:

- `SpawnSessionFromItem` (line 438): `if !isReopen && !item.SkipPlanning && !item.PlanApproved &&
  !req.Msg.Autonomous { ...blocked... }` — this guard fires on the **direct spawn path**, not
  just the queue.
- `spawnSessionAfterGates` (line 656): `if !item.SkipPlanning && !item.PlanApproved && !autonomous
  { ...blocked... }` — same guard, defense-in-depth at the actual spawn point.
- So the gate **is** applied uniformly to every path that spawns a work session, queued or not —
  the *only* documented bypass is `autonomous=true` (comment at line 432: "Autonomous mode
  bypasses the gate — the driver handles its own planning loop").

**This changes Success Criterion 2's framing.** The real gap is narrower than "the gate barely
gates anything" — it's:
  (a) `StuckReasonPlanNotApproved`/`planApprovalStaleness` (5 min) is a **queue-specific staleness
      detector**, separate from the spawn-time guard, and only fires for items sitting in
      `queued` status waiting on `DequeueNextQueuedItems` — an item stuck in `ready` status
      with an unapproved plan (never queued) gets no staleness/stuck surfacing at all, only a
      disabled button.
  (b) Autonomous-mode bypass is real and intentional, not a bug — but the UI gives it no visual
      distinction (see #5 below).
Recommend the plan phase re-verify and re-scope Success Criterion 2 against this — "apply
consistently" may already be ~90% true at the RPC layer; the actual gap is UI legibility of
*why* an item is stuck (ready-but-unapproved with no stuck-item surfacing) plus autonomous-mode
visual distinction, not a backend gating hole.

## 5. Autonomous mode bypass — UI has zero visual distinction today

`ActionsSection.tsx:63`: `const canRunAutonomously = item.status === "ready";` — no dependency on
`planApproved`/`skipPlanning` at all, confirming autonomous intentionally skips the plan gate.
But nothing in `ActionsSection.tsx` or `PlanArtifactsSection.tsx` marks an item as "this session
is running/ran autonomously, the plan gate did not apply." A user glancing at item history after
the fact cannot tell "plan was approved" from "plan gate was never in play." This is a genuine
gap requirements.md's Success Criterion 1 should explicitly cover: the four states listed
("no plan yet", "pending review", "approved", "changes requested") should arguably be five,
adding "gate bypassed (autonomous)" — otherwise an approved-looking absence of the button is
ambiguous with an autonomous-run item.

## 6. Industry comparables, scaled to solo-reviewer single-file review

- **GitHub PR review** (Approve / Request Changes / Comment, with inline line-comments on a
  diff) is the closest mental model and matches `GateVerdictBox`'s own tri-state shape almost
  exactly (PASS≈Approve, FAIL/PARTIAL≈Request Changes). The multi-reviewer/required-approvals
  machinery (CODEOWNERS, review dismissal, re-request) is irrelevant here per the explicit
  "solo-developer" out-of-scope note in requirements.md — don't import that complexity.
- **Line-level/inline comments on a diff** (GitHub PR, Notion block comments, Linear) are the
  right reference for Success Criterion 5 (line-level feedback), but note the plan artifact is
  **markdown prose, not a diff** — there's no natural "line added/removed" framing. The more
  apt analogy is Google Docs / Notion "select text → comment" (anchored to a text range within
  rendered content) rather than a diff gutter. A cheaper MVP matching GitHub's *simplest* form
  (a single PR-level review comment box, no inline anchoring) is exactly what `GateVerdictBox`'s
  reopen-feedback textarea already does — recommend that as the v1 floor, with section-anchored
  comments (anchor feedback to a markdown heading/section rather than a literal line number) as
  a fallback middle ground if full text-range selection proves too complex, consistent with
  requirements.md's own "may land as a follow-up" hedge.
- **Regenerate-after-reject diffing**: no code review tool exactly matches this because most
  tools review a fixed diff, not a regenerated document. The nearest analogy is Google Docs
  "compare versions" / suggestion-mode diffing. Given the out-of-scope note ruling out a full
  CRDT editor, recommend scoping this down to "show a text diff between the previous and
  regenerated plan.md" (a markdown-to-markdown diff, reusing whatever diff-rendering already
  exists for PR review — see `web-app/src/lib/utils/parseDiff.ts`, which already exists for
  something diff-shaped) rather than building comment-anchored regeneration tracking.

## 7. Edge cases and failure modes to design for

1. **Stale rendered plan after regeneration.** If a user has `plan.md` open (rendered) in a
   browser tab and triage re-runs (feedback-driven refinement) producing a new
   `planArtifactsPath` or updated content at the same path, the tab shows stale content with no
   indicator. `PlanArtifactsSection.tsx` currently has no watch/poll on content changes — it
   only reacts to `item.planArtifactsPath` changing as a *prop*, and even that swap wouldn't
   re-fetch rendered markdown content (there's no content-fetching call today at all, since only
   the path string is shown). Any content-rendering addition needs either a live watch (event-bus
   push, consistent with the rest of the app's event-driven update pattern — see
   `useWatchBacklogItems.test.ts`) or a manual refresh affordance with a "content may be stale"
   banner if the underlying file's mtime changed since last fetch.
2. **Approval state after regeneration — does it reset?** `ApprovePlan` (`backlog_service_lifecycle.go:617-657`)
   sets `PlanApproved=true`/`PlanApprovedAt=now` but nothing in `TriggerTriage`
   (`backlog_service_triage.go:1834+`) was found resetting `PlanApproved` back to `false` when a
   *new* plan is generated via feedback-driven refinement — confirm in the architecture research
   phase, but if true this is a correctness bug independent of UX: an already-approved item could
   silently get a re-triaged/changed plan while `planApproved` stays stuck `true`, defeating the
   gate. This should be an explicit design decision, not an oversight: regenerating a plan should
   almost certainly reset `planApproved=false` (mirroring how a new commit dismisses a stale
   GitHub PR approval on some repo settings) or at minimum bump `PlanApprovedAt` staleness
   visibly.
3. **Concurrent plan review across multiple backlog items.** No item-specific locking issue found
   here beyond the existing `triageInFlight` per-item in-flight guard
   (`backlog_service_triage.go:1884-1893`), which already prevents double-triggering triage for
   the *same* item. Reviewing/rejecting plans on multiple *different* items concurrently is
   already safe (each item has independent state) — no new concurrency hazard, but worth an e2e
   test since `plan-gate.spec.ts` today only covers a single item.
4. **`skipPlanning` vs `planApproved` visual distinction.** `canSpawnSession` in
   `ActionsSection.tsx:59-61` treats them as equivalent ("either is fine to spawn"), but they mean
   different things to a user reviewing history: `skipPlanning=true` means "no plan was ever
   required," `planApproved=true` means "a plan existed and a human signed off on it." Collapsing
   these into one green checkmark loses information the user likely wants (matches
   requirements.md's own Success Criterion 1 wording distinguishing 4+ states).
5. **Rejecting a plan for an item with no plan artifacts yet.** `PlanArtifactsSection.tsx:14`
   (`if (!item.planArtifactsPath) return null;`) and `ActionsSection.tsx:160`
   (`{item.planArtifactsPath && (...Approve Plan button...)}`) both guard on
   `planArtifactsPath` being set — the reject action needs the identical guard, and the UI needs
   a distinct "no plan yet" state (already called out in Success Criterion 1) rather than simply
   hiding all plan-related UI, so the user doesn't wonder whether the feature is broken versus
   not-yet-applicable.
6. **`skipReviewGate` interaction** — mentioned in requirements.md's pitfalls list but not found
   as a literal identifier in this codebase; likely refers to `skipPlanning` and/or the "Skip
   Gate" button in `GateVerdictBox`. Recommend the Pitfalls research agent (Agent 4) confirm the
   actual flag name(s) rather than assuming `skipReviewGate` exists verbatim — a quick grep
   during this research turned up only `SkipPlanning`/`skip_planning` (plan gate) and
   `GateVerdictBox`'s "Skip gate" button (review gate, no backing boolean field name matching
   `skipReviewGate` found in the schema).

## 8. Unstated needs beyond the explicit requirements

1. **Notification when a plan is ready to review.** No `notify()` call was found firing when
   triage completes and produces a plan (searched `server/services/backlog_service_triage.go`
   for `notify`/`Notif` — only found `notifyReworkCapHit`/`notifyRepeatedFailure`, both
   failure-path notifications, not a "plan ready" success notification). Compare to
   `submit_review_verdict`'s flow, which does have an operator-facing event
   (`BacklogItemVerdictRecordedEvent`, `backlog.proto:656-660`) surfaced via the event bus. A
   "plan ready for your review" notification (reusing the existing `NotificationEvent`/event-bus
   plumbing already used elsewhere in `backlog_lifecycle.go`, ~20 call sites of `l.notify(...)`)
   would close the loop the user is implicitly asking for: right now there is no push signal that
   triage finished, so review only happens if the user manually revisits the item. This is a
   strong candidate for a "Should Have" or "Must Have" addition even though requirements.md
   doesn't currently list it verbatim — it's implied by "surface staleness signal" work already
   done in a related recent commit (`cf452dea7 fix(backlog): surface staleness signal in
   blocked-spawn error (#292)`), which is solving the adjacent problem of "the user doesn't know
   something needs their attention."
2. **A diff view for regenerated plans.** Directly implied by edge case #1/#2 above — if
   `planApproved` resets on regeneration (as it likely should per #2), the user rejecting a plan
   and getting a new one back has no way to see *what changed* without manually diffing two
   markdown files themselves. Given `parseDiff.ts` already exists in the codebase for
   diff-rendering (likely for PR/session diffs), reusing that utility for plan.md-to-plan.md
   diffing is a low-cost way to meet this unstated need without new dependencies.
3. **History/audit trail of approval and rejection events**, not just current state. Success
   Criterion "Should Have" already names this, but concretely: `GateVerdictBox`'s `readOnly`
   variant is the direct precedent (verdict cards render as permanent historical records once a
   `headless-re-review-*` row exists) — the same pattern (a persisted `PlanVerdictBox`-shaped
   record per approve/reject event, not just two mutable fields `PlanApproved`/`PlanApprovedAt`
   overwritten in place) would give plan-approval the same historical fidelity review-verdicts
   already have. Today `ApprovePlan` only ever writes the *current* approval state (two fields,
   last-write-wins) — there is no per-event history table for plan approvals analogous to
   `ItemSession.ReviewVerdict`, so a "Should Have" timeline entry needs either a new events
   table/event type or piggybacking on the existing generic status-event stream if one exists
   (worth confirming in Architecture research, Agent 3).

## Sources (files read/grepped during this research)

- `proto/session/v1/backlog.proto` (messages: `ApprovePlanRequest/Response`,
  `TriggerTriageRequest/Response`, `OverrideVerdictRequest/Response`, `ReviewVerdict`,
  `CriterionVerdict`, `BacklogItemVerdictRecordedEvent`)
- `session/backlog_lifecycle.go` (`StuckReasonPlanNotApproved`, `planApprovalStaleness`,
  `notify()` call sites)
- `server/services/backlog_service_triage.go` (`TriggerTriage`, `SpawnSessionFromItem`,
  `spawnSessionAfterGates`, autonomous bypass, `notifyReworkCapHit`/`notifyRepeatedFailure`)
- `server/services/backlog_service_lifecycle.go` (`ApprovePlan` handler)
- `server/services/backlog_service_test.go` (`TestApprovePlan_*` behavioral confirmation)
- `server/mcp/tools_backlog.go` (`submit_review_verdict`/`request_review`/`get_backlog_item` MCP
  tool handlers, role-aware prompt injection, `ReviewTrigger`/`ReviewCompletionSignaler`)
- `session/review_gate.go` (`ReviewGateRunner`, review-gate prompt construction)
- `web-app/src/components/backlog/GateVerdictBox.tsx` (full read — the reference
  approve/reject-with-reason component)
- `web-app/src/components/backlog/detail/ActionsSection.tsx` (current Approve Plan button,
  `canSpawnSession`/`canRunAutonomously` derivations)
- `web-app/src/components/backlog/detail/PlanArtifactsSection.tsx` (full read — current
  path-only rendering, no content fetch)
- `web-app/src/components/backlog/detail/DescriptionSection.tsx` (existing `react-markdown` +
  `remark-gfm` usage precedent, confirms markdown rendering is already a dependency)
- `web-app/package.json` (`react-markdown@^10.1.0` already present)
- `web-app/src/lib/hooks/useAuditLog.ts` (generic user-interaction logger — not the same as a
  status timeline, ruled out as the history mechanism)
- `tests/e2e/plan-gate.spec.ts` (existing single-item gate coverage, must not regress)
