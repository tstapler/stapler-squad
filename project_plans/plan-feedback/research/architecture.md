# Architecture Research: plan-feedback

Resolves the three "Rabbit Holes" in `project_plans/plan-feedback/requirements.md`
with file:line evidence. Hotspot context (not re-derived here): `docs/reference/hotspot-ranking.md`
row 4 — `server/services/backlog_service_triage.go` is **partially remediated**
(pure file-level extraction, commit `601b4550e`) into
`backlog_service_trigger_triage.go`, which now holds `TriggerTriage`/`CancelTriage`.
`backlog_service_lifecycle.go` (home of `TransitionBacklogItemStatus`/`RejectPlan`)
is not a flagged hotspot.

## 1. Target status: `refining` vs `ready` — recommend `ready`

**Key evidence that changes the framing in requirements.md's Rabbit Holes section:**
the retriage prompt does *not* actually depend on `item.PlanArtifactsPath` for its
context, so the "refining wipes prior plan as retriage context" concern is weaker
than it looks — but `ready` is still the right target, for a different, stronger
reason: **field semantics already line up with the exact status this action lands
the item on.**

- `TriggerTriage`'s status guard (`server/services/backlog_service_trigger_triage.go:184-188`)
  only accepts `idea` or `ready`. `refining` is rejected outright — confirmed:
  ```go
  if item.Status != string(session.BacklogStatusIdea) && item.Status != string(session.BacklogStatusReady) {
      return nil, connect.NewError(connect.CodeFailedPrecondition, ...)
  }
  ```
- The retriage prompt's "prior result" (`priorResult`, used by `BuildHeadlessRetriagePrompt`/
  `BuildHeadlessChatRetriagePrompt`, `session/backlog_triage.go:117,176`) comes from
  `findPriorTriageResult(existingSessions)` (`server/services/backlog_service_trigger_triage.go:731-744`),
  which scans `ItemSession.TriageResult` (a JSON blob on the *session* row) — **not**
  from `item.PlanArtifactsPath` or any item-entity field. The prompt's `artifactAbsPath`
  is also independently recomputed as `filepath.Join(triageBase, item.ID)`
  (`backlog_service_trigger_triage.go:280`, deterministic from item ID alone) and then
  reads `plan.md`/`validation.md`/`research/*.md` straight off disk
  (`session/backlog_triage.go:142-156`). None of this reads the item entity's
  `PlanArtifactsPath` field. So wiping that field (the `idea`/`refining` reset block,
  `backlog_service_lifecycle.go:811-827`) does **not** actually starve the retriage
  prompt of context — the prior-plan files on disk and the prior JSON result on the
  session row both survive untouched.
- What *does* get reset by that idea/refining block, and would need to be manually
  re-derived if the target were `refining`: `PlanApproved`, `PlanArtifactsPath`
  (transiently, until the next triage run's completion overwrites it — see
  `backlog_service_trigger_triage.go:600-611`), and `PlanRejectionReason`
  (`backlog_service_lifecycle.go:818-820`). None of these feed `TriggerTriage`'s
  own precondition or prompt — but they do feed **UI state** while the operator is
  waiting: `derivePlanReviewStatus` (`web-app/src/lib/backlog/planReviewStatus.ts:24-32`)
  and `PlanVerdictBox`/`ActionsSection` read those exact three fields to decide what
  to render, and are only ever invoked at `ready`/`queued` status
  (`web-app/src/lib/backlog/itemActions.ts` `ready`/`queued` cases, ~L143-167). At
  `refining`, `ItemActionabilityInput`'s `refining` case has "no status-specific
  primary action" (`itemActions.ts` ~L145-148) — there is no rendering surface for
  the plan/feedback state at all while sitting there.
- Sending to `ready` instead: `TransitionGuard`'s reset block only fires for
  `to == idea || to == refining` (`backlog_service_lifecycle.go:813`) — `ready` is
  excluded, so none of `PlanApproved`/`PlanArtifactsPath`/`PlanRejectionReason` gets
  wiped by the transition itself. And `ready` is already a valid `TriggerTriage`
  starting status with zero guard changes (item 4 in requirements.md's gap list is
  resolved for free).
- **The reuse argument is stronger than "avoids a guard change":** the state a
  `ready`-targeted send-back produces (`PlanApproved=false`,
  `PlanRejectionReason=<feedback>`) is *exactly* `RejectPlan`'s post-condition
  (`backlog_service_lifecycle.go:928-934`). That means `PlanVerdictBox`'s existing
  "changes_requested" card and "Regenerate Plan with This Feedback" button
  (`web-app/src/components/backlog/BacklogItemDetail.tsx:1009-1013`) render correctly
  the moment the item lands at `ready` — for free, and as a resilience path: if the
  frontend's second call (`triggerTriage`) fails after the first
  (`transitionStatus`) succeeds, the operator sees the same recognizable
  "changes_requested" UI and can retry via the existing button rather than facing a
  silent dead end.
- One added consideration not previously called out in requirements.md: the
  `review`/`pr_pending` → `ready` backward edge carries its own guard —
  `ErrVerdictClearRequiredForReady` (`session/domain/backlog.go:644-657`), which
  blocks the transition if the item has a recorded PASS verdict *unless*
  `OverrideReason` is set. Since `send back` items are frequently coming from
  `review` with a PASS verdict (an operator watched review pass but wants a redo),
  the frontend's `transitionStatus` call must always pass a non-empty
  `overrideReason` (the feedback text itself is a natural fit) — harmless for the
  unguarded `in_progress`/`done` sources (it just becomes an audit progress note,
  `backlog_service_lifecycle.go:794-800`), required for `review`/`pr_pending` with a
  PASS verdict.

**Recommendation: target `ready`.** No `TriggerTriage` guard change, no new
intermediate status hop, and it reuses `RejectPlan`'s existing "changes_requested"
UI surface as both the display and the failure-recovery path. `refining` should be
treated as out of scope for this feature — see "dead code disposition" below.

## 2. Where to persist the feedback text — recommend reuse `PlanRejectionReason`

- `PlanRejectionReason` already exists, is already length-capped
  (`maxRejectReasonLength = 10000`, `backlog_service_lifecycle.go:892`), already
  flows into `triggerTriage(id, feedback)` via the existing
  `handleRegeneratePlanWithFeedback` pattern (`BacklogItemDetail.tsx:1009-1013`:
  `triggerTriage(item.id, item.planRejectionReason)`), and is already rendered by
  `PlanVerdictBox` (`rejectionReason={item.planRejectionReason}`,
  `BacklogItemDetail.tsx:1634`).
- The semantic objection in requirements.md ("why I rejected an unapproved plan," not
  "what to change" from a later stage) is real for the field's *name*, but not for
  its *lifecycle*: once the target status is `ready` (§1), the field's consumer
  (`derivePlanReviewStatus`) treats a non-empty value identically regardless of which
  earlier status the item came from — it only ever asks "is there outstanding
  feedback on the plan sitting at `ready` right now," which is precisely what this
  feature produces.
- The field self-clears on every successful triage completion, regardless of what
  triggered it: `TriggerTriage`'s post-run persistence
  (`backlog_service_trigger_triage.go:604-611`) unconditionally sets
  `PlanRejectionReason: &clearedReason` (empty string) alongside the fresh
  `PlanArtifactsPath`/`PlanApproved` reset. So reused feedback text does not linger
  past the retriage it fed — it resolves the same way `RejectPlan`'s reason always
  has.
- Genuine risk if reused as-is: `RejectPlan` itself (`backlog_service_lifecycle.go:898-935`)
  has **no status guard at all** — only `item.PlanArtifactsPath == ""` is checked
  (line 923-926). If this project's frontend ends up calling `RejectPlan` directly
  (as an alternative to writing `PlanRejectionReason` via a status-transition side
  effect), it would already work unmodified from any status where
  `PlanArtifactsPath` is still set (i.e., anything that hasn't passed through an
  idea/refining reset) — worth confirming in Phase 3 planning whether reusing
  `RejectPlan` itself (not just its field) is viable, since it removes one need
  for new backend code entirely.

**Recommendation: reuse `PlanRejectionReason`, do not add a new field.** The
"different semantic scope" objection dissolves once the target status is `ready`
(§1) — the field's actual contract is "outstanding feedback on the plan currently
sitting at ready," not "feedback specifically from the pre-implementation stage."

## 3. RPC shape: two calls behind one submit, not a new merged RPC

- ADR-002 (`project_plans/plan-approval-ux/decisions/ADR-002-reject-plan-manual-retrigger.md`,
  referenced in requirements.md's Constraints) already rejected auto-invoking triage
  as a side effect of a status/field write, specifically to avoid duplicating
  `TriggerTriage`'s precondition/guard sequence: the `triageInFlight` sync.Map
  TOCTOU guard (`backlog_service_trigger_triage.go:238-247`), the `triageSem`
  8-slot concurrency semaphore (lines 351-370), and
  `tombstoneOrphanTriageSessions` (line 226) orphan-session handling. That
  machinery lives entirely inside `TriggerTriage` and is not separately callable —
  a new merged RPC would have to either import/call `TriggerTriage`'s logic
  wholesale (no functional difference from calling the RPC twice) or reimplement a
  parallel copy of it (exactly what ADR-002 rejected).
- Nothing about this feature's shape differs from `RejectPlan`+`triggerTriage`'s
  existing precedent in a way that would change that calculus. The one apparent
  difference — "the user wants ONE action, not two separate clicks" — is a
  **frontend UX difference**, not a backend one: `handleRegeneratePlanWithFeedback`
  (`BacklogItemDetail.tsx:1009-1013`) already demonstrates that a single `useCallback`
  handler can sequence two RPC calls (`rejectPlan` then `triggerTriage`) — it's
  just currently split across two separately-clicked buttons in the UI, not two
  backend calls. Making it "one click" only requires collapsing that into one
  handler triggered by one submit, exactly as ADR-002 anticipated when it scoped
  its two-RPC rejection to "duplicating the *sequence*," not to "how many clicks the
  UI exposes."
- Concretely, three RPC calls chain behind the one submit button (see §1's
  `overrideReason` note for why the first is needed):
  1. `transitionStatus(id, "ready", { overrideReason: feedback })` — moves the item
     back to `ready`; the guard at `review`/`pr_pending` with a PASS verdict needs
     `overrideReason` non-empty (`session/domain/backlog.go:651-657`), harmless
     elsewhere (becomes an audit note, `backlog_service_lifecycle.go:794-800`).
  2. Persist the feedback for display/resilience — either reuse `RejectPlan(id, feedback)`
     directly (precondition-compatible per §2), or fold the write into step 1's
     path if Phase 3 planning prefers a single combined status+field write inside
     `TransitionBacklogItemStatus` (out of scope for this research; a planning
     decision, not an architecture blocker).
  3. `triggerTriage(id, feedback)` — the actual retriage kickoff, now valid because
     the item is at `ready`.
  All three are pre-existing RPCs; no new backend RPC/message type is needed.

**Recommendation: do NOT introduce a new merged RPC.** Sequence existing RPCs from
one frontend submit handler, matching ADR-002's precedent and rationale exactly.

## 4. Frontend integration points

- `web-app/src/lib/hooks/useBacklogService.ts` already exposes every RPC method
  needed — `transitionStatus` (L929-957, already accepts `overrideReason`),
  `triggerTriage` (L981-994, already accepts `feedback`), and `rejectPlan`
  (L1020, referenced by `handleRejectPlan` at L990-1007). **No new hook method is
  required.**
- `web-app/src/components/backlog/BacklogItemDetail.tsx` needs a new combined
  handler (parallel to `handleRegeneratePlanWithFeedback`, L1009-1013, but firing
  both calls from one submit instead of two separately-clicked buttons) — e.g.
  `handleSendBackWithFeedback(feedback: string)` that runs the
  transitionStatus → (persist) → triggerTriage sequence in §3, then
  `await load()` and a toast, matching the existing action-handler pattern in the
  big switch at lines 760-813 and the toast/error handling pattern in
  `handleRejectPlan` (L990-1007).
- `web-app/src/components/backlog/detail/ActionsSection.tsx` needs a new UI
  affordance replacing today's bare "↩ Back to Ready" button (the `send_back_ready`
  case, `itemActions.ts` `CAN_SEND_BACK_READY`, L122-127) for the
  `in_progress`/`review`/`pr_pending`/`done` statuses — a feedback textarea +
  submit, structurally similar to `PlanVerdictBox.tsx`'s existing reject-reason
  form (`showReject`/`reason` state, `textareaRef` autofocus, `canSubmit` gate —
  `PlanVerdictBox.tsx:80-97,211-220`) but rendered from a status range
  `PlanVerdictBox` itself never covers (it only renders at `pending_review`/
  `changes_requested`, i.e. near `ready` — `PlanVerdictBox.tsx:193`). This is a new
  small component or an inline extension of `ActionsSection`, not a `PlanVerdictBox`
  reuse, since the statuses don't overlap.
- `web-app/src/lib/backlog/itemActions.ts`: the bare `send_back_ready` action
  (L122-127, L222-224) should be replaced or given a feedback-carrying variant.
  Also fixes requirements.md's success metric #2: `send_back_refining` is
  referenced in `BacklogItemDetail.tsx`'s switch (L793-795) and toast map (L92)
  but `itemActions.ts` never adds it to any action set — confirmed dead by
  grepping `itemActions.ts` for `CAN_SEND_BACK_REFINING` (no match; only
  `CAN_SEND_BACK_IDEA`/`CAN_SEND_BACK_READY` exist, L114-127).

## 5. Event-Command-Policy table

Skipped — this is a single-actor (operator), single-system UI action layered on
existing, already-modeled machinery (backward transition + retriage). It doesn't
introduce new actors, systems, or cross-boundary business rules beyond what
`TransitionGuard`/`TriggerTriage` already encode; an EventStorming table would just
restate the three-call sequence in §3 without adding clarity.

## 6. Tech debt disposition

- **`server/services/backlog_service_trigger_triage.go` (`TriggerTriage`) — Extend
  as-is.** No changes needed at all under the `ready`-target recommendation (§1) —
  its status guard already accepts `ready`, and its feedback param already flows
  into the prompt builders unchanged. The file is a partially-remediated hotspot
  (`hotspot-ranking.md` row 4), but this feature adds zero lines to it.
- **`server/services/backlog_service_lifecycle.go` (`TransitionBacklogItemStatus`,
  `RejectPlan`) — Extend as-is.** Not a flagged hotspot. `TransitionBacklogItemStatus`
  needs no code change under the `ready`-target recommendation (the idea/refining
  reset block is simply not triggered); `RejectPlan` needs no change either if
  reused directly (§2). If Phase 3 instead chooses to fold the feedback-persist
  write into `TransitionBacklogItemStatus` itself (§3's step 2 alternative), that
  handler is ~140 lines (`backlog_service_lifecycle.go:689-832`) and already has
  several status-conditional blocks (hasUnshippedCode, hasUnresolvedBlockers, the
  idea/refining reset) — adding one more conditional block for `to == ready` follows
  the file's existing pattern rather than fighting it; still "extend as-is," not a
  refactor trigger.
- **`web-app/src/components/backlog/BacklogItemDetail.tsx` — Extend as-is (isolate
  new state via a seam if it grows).** This file already centralizes every
  action's handler in one big switch/useCallback (L760-813) plus a growing set of
  per-feature `useState` pairs (manual review, reject-plan interplay via
  `PlanVerdictBox`, etc.) — not flagged as a hotspot in the row-4 analysis, but it
  is clearly accumulating per-feature state. Adding one more handler
  (`handleSendBackWithFeedback`) and one more piece of textarea state follows the
  existing pattern; if a future feature needs a fourth or fifth such feedback-box
  variant, extracting a shared `useFeedbackSubmit`-style hook (mirroring
  `handleRejectPlan`'s shape) would be the seam to isolate through, not a full
  refactor now.

## Summary of recommendations

| Question | Recommendation |
|---|---|
| Target status | `ready` (not `refining`) — zero guard changes, reuses `RejectPlan`'s existing UI state as both display and failure-recovery path |
| Feedback field | Reuse `PlanRejectionReason` — its contract ("outstanding feedback on the plan at ready") already matches once target is `ready` |
| RPC shape | No new merged RPC — sequence `transitionStatus` → (persist reason) → `triggerTriage` from one frontend submit handler, per ADR-002's precedent |
| `send_back_refining` dead code | Remove (superseded by the `ready`-targeted feedback flow) rather than wired up, since `refining` was rejected as the target |
