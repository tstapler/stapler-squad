# Research: Existing Features This Should Be Consistent With

## 1. The two most directly related prior features

### ADR-002 — RejectPlan stays two clicks, deliberately

`project_plans/plan-approval-ux/decisions/ADR-002-reject-plan-manual-retrigger.md`

- `RejectPlan(item_id, reason)` only **persists** state (`PlanRejectionReason`,
  `PlanRejectedAt`) — it never calls `TriggerTriage` or any LLM path itself, and
  returns immediately, mirroring `ApprovePlan`.
- The frontend closes the gap with a **second, visually distinct button** —
  `PlanVerdictBox`'s "Regenerate Plan with This Feedback" — which calls the
  already-shipped `triggerTriage(item.id, item.planRejectionReason)`.
- Rejected alternative: auto-invoking `TriggerTriage` synchronously inside
  `RejectPlan`. The stated reason is that `TriggerTriage`'s real work sits behind
  a precondition/guard sequence (`triageInFlight` TOCTOU guard, `triageSem`
  concurrency semaphore, a budget-scoped context, and
  `tombstoneOrphanTriageSessions`'s orphan-session sweep) that all run *before*
  the LLM call — folding a second handler into that path means either
  duplicating the guard sequence (drift risk) or extracting it into a shared
  helper first, which the ADR calls "a real refactor... out of proportion" for
  that pass.
- Also rejected: a frontend "fire and forget" two-RPC call with no user click in
  between — reintroduces the "nothing visibly happened but a 7-15 minute LLM
  call is now running" confusion, and removes the ability to reject several
  plans in a review pass before spending any LLM budget.
- Accepted tradeoff, stated explicitly: two clicks to see a new plan, revisit
  only if user friction is confirmed in practice.

### backlog-operator-feedback-loop — the Q&A / steer / plan-review feedback loop

`project_plans/backlog-operator-feedback-loop/requirements.md`

Three gaps shipped as one ticket, all under the same goal ("talk back to the
agent from the item detail view"):

1. **Triage clarifying questions** — `TriageReviewPanel`'s "Not quite — give
   feedback" / `GuidanceRequestPanel` answer flow. An answer is delivered as
   feedback for the item's **next triage run**, via the existing feedback-driven
   re-triage path — explicitly "no new triage mechanism."
2. **Steer a live session in place** — wiring `steer_session` into
   `SessionsSection.tsx`'s backlog item detail view (previously only reachable
   from the general session list).
3. **`RejectPlan` / Request Changes** — new backend surface (this is what became
   ADR-002 + `PlanVerdictBox`), applying only while the item sits at `ready`
   with `PlanApproved=false`.
- Single-operator assumption stated explicitly (no multi-tab/multi-operator
  concurrency token) — same assumption `plan-feedback`'s requirements.md carries
  forward.
- Kano framing: plan review (Gap 3) is Must-be, triage Q&A (Gap 1) is
  Performance, steer-in-place (Gap 2) is Attractive — useful precedent for how
  this project's own feature might be framed, though not requested here.

### How `plan-feedback` differs from and builds on both

- **Builds on**: reuses the exact same `TriggerTriage(item_id, feedback)` path
  both priors funnel into — no new triage/LLM invocation code, per this
  project's own Constraints section. Reuses `RejectPlan`'s cap precedent
  (`maxRejectReasonLength = 10000`, `backlog_service_lifecycle.go:892`) and
  `PlanVerdictBox`'s reject-form UI conventions (see §2 below).
- **Differs in trigger scope**: ADR-002's flow only applies at `ready` with
  `PlanApproved=false` (pre-implementation). Gap 1's Q&A flow only applies
  during active triage (`item.status === "idea"`, per `TriageReviewPanel`'s doc
  comment). Neither covers `in_progress`/`review`/`pr_pending`/`done` — items
  whose session has already produced a reviewable result. `plan-feedback` is
  the first feedback-capture surface reachable from *those* four statuses.
- **Differs in click count — the one deliberate, called-out deviation**:
  ADR-002 is intentionally two clicks (Reject, then separately Regenerate) so a
  reviewer can reject without spending LLM budget immediately, and can batch
  several rejections before regenerating any of them. `plan-feedback`'s
  requirements.md explicitly asks for **one action** (type feedback, submit,
  done) — the user's own explicit direction, called out in Scope as "a
  deliberate, justified deviation... not an oversight." The two flows differ
  because their calling context differs: ADR-002's user is a reviewer batching
  judgments on plans that haven't started yet (nothing is running, no urgency to
  restart), while `plan-feedback`'s user is redirecting an item that's already
  past planning and possibly mid-flight — there's no equivalent "review several,
  then decide" batching workflow motivating a pause. Per requirements.md's Rabbit
  Holes, this can still be implemented as two RPC calls made back-to-back from
  one UI submit (structurally two calls, functionally one click) — that's
  compatible with ADR-002's underlying reasoning (don't duplicate
  `TriggerTriage`'s guard sequence into a second handler) while diverging only
  on the UI's click count.
- **Differs in status-machine surface touched**: `TransitionBacklogItemStatus`'s
  idea/refining reset block (`backlog_service_lifecycle.go:811-827`) already
  exists and wipes `PlanApproved`/`PlanArtifactsPath`/`PlanRejectionReason` on
  any transition to `idea` or `refining` — neither prior feature transitions an
  item to `refining` (ADR-002/RejectPlan never transitions status at all; Gap 1
  stays in `idea`), so this reset block has never actually been exercised by a
  real caller. `plan-feedback` is positioned to be the first caller that would
  invoke it (if `refining` is chosen — see Rabbit Holes) or bypass it entirely
  (if `ready` is chosen, which preserves `PlanArtifactsPath` as retriage
  context per `TriggerTriage`'s `priorResult`-aware prompt builder).

## 2. UI conventions to match (`TriageReviewPanel.tsx`, `PlanVerdictBox.tsx`)

Both components share one shape almost exactly — a new feedback box should
follow it:

| Convention | TriageReviewPanel ("Not quite — give feedback") | PlanVerdictBox ("Request Changes") |
|---|---|---|
| Toggle | Secondary button reveals `showRefineForm`/`showReject` textarea | Same — `aria-expanded` on the toggle button, stays mounted (not conditionally unmounted) so its ref survives for focus-return on cancel |
| Label | `<label>` "What should change?" | `<label>` "What should change? (required)" |
| Textarea | `rows={3}`, placeholder example text, `data-testid` on the textarea itself | Same shape; placeholder "Explain what should be different about this plan…" |
| Focus | — (no explicit focus-on-open effect in TriageReviewPanel) | `useEffect` focuses the textarea on open (`textareaRef.current?.focus()`) |
| Cancel | Resets text, hides form | Resets text, hides form, returns focus to the toggle button |
| Escape key | Not handled | `onKeyDown` — Escape triggers Cancel |
| Validation | Submit disabled unless `feedback.trim()` non-empty | Submit disabled (`canSubmit`) unless `reason.trim().length > 0` |
| Character limit | No client-side cap rendered (server caps separately) | No client-side cap rendered either — `maxRejectReasonLength` (10000) is server-only |
| Loading state | Local `"idle"\|"submitting"\|"error"` state; button label flips to "Refining…"; `aria-busy` | Same shape: `localPending`/`actionPending` union; button label flips to "Requesting…"; `aria-busy` |
| Error handling | `TriageErrorBanner` (message + Reload/Skip actions) rendered above the form | `InlineError` (dismiss-only, no wireable retry — "the user re-triggers the action via the still-open form/button") |
| Success | Closes form, clears text, resets local state | Closes form, clears reason |
| Test IDs | `triage-refine-toggle-button`, `triage-refine-textarea`, `triage-refine-submit-button`, `triage-refine-cancel-button` | `backlog-action-reject-plan`, `plan-reject-reason`, `backlog-action-reject-plan-submit` |

Both intentionally omit a visible remaining-character counter even though a
server-side cap exists — a new feedback box should match that (no new UI
pattern needed) rather than inventing a counter neither precedent has.

## 3. Edge cases / failure modes from live/orphan-session handling precedent

Checked `session/backlog_lifecycle_triage.go` (periodic orphan-triage
reconciliation — a different code path, listener-driven, not part of this
feature's synchronous request) and
`server/services/backlog_service_trigger_triage.go` /
`backlog_service_triage.go`'s synchronous guards, which are directly relevant:

- **`tombstoneOrphanTriageSessions` only ever touches triage-role sessions**
  (`is.Role != string(session.SessionRoleTriage)` is skipped,
  `backlog_service_triage.go:3153-3197`). It never looks at work- or
  review-role sessions. So calling `TriggerTriage` on an item that also has a
  **live work/review tmux pane** (true for `in_progress`, `review`, and — since
  `pr_pending` is not in `session.IsTerminalStatus`, only `done`/`archived` are,
  `session/terminal_status.go:28-33` — also possibly true for `pr_pending`) will
  **not** stop, archive, or otherwise touch that pane. It is left running,
  untouched, exactly as before.
- **The existing precedent for "there's a leftover session" is
  `forceResetItem`** (`backlog_service_triage.go:1122-1144`), invoked from
  `SpawnSessionFromItem` when `req.Msg.Force=true` and status is `in_progress`
  or `review` (wired to the already-shipped "Restart Session" action,
  `BacklogItemDetail.tsx:739-741`). It actively calls
  `sessionStopper.StopSessionByUUID` (kills the tmux pane **and** the worktree,
  unlike `killEndedWorkSessionPanes`' pane-only kill for already-*ended*
  sessions) on every live work/review session, marks the `ItemSession` row
  ended, and — if status was `review` — transitions the item back to
  `in_progress` first. **This is the strongest available precedent** for what
  "send back" should do to a live session, since it's the one existing
  path that intentionally tears down live backlog work to restart from a
  clean state.
- **If a live work session is left running (feature does nothing to it) and
  the operator later tries to start a new session from the revised plan,
  `SpawnSessionFromItem` blocks the respawn** —
  `TestSpawnSessionFromItem_LiveWorkSession_StillBlocksSpawn`
  (`backlog_service_test.go:2374`) confirms a live work session return a
  `FailedPrecondition`-shaped block unless `force=true` is passed. So *not*
  handling the live-session case in this feature doesn't fail silently — it
  surfaces later, as a confusing "can't restart" error the operator has to
  work around by separately using "Restart Session" (force) first. That's a
  real UX gap worth closing explicitly rather than deferring.
- **`CleanupTerminalItem`** (worktree cleanup + `archiveItemWorkSessions`,
  `server/services/backlog_service.go:1159-1167`) fires **only** on transitions
  *to* a terminal status (`done`/`archived`) — never on a transition *away*
  from one. So an item already at `done` (which had its work/review session
  archived + pane-killed when it first reached `done`) is safe — nothing live
  to worry about. But `pr_pending`, `review`, and `in_progress` items have never
  had this cleanup run, so their sessions can genuinely still be live when
  feedback is submitted.
- **Recommendation for Phase 3**: for `in_progress`/`review` (and possibly
  `pr_pending`, if it can carry a live session — verify), reuse
  `forceResetItem`'s live-session teardown (or a call to the same
  `sessionStopper.StopSessionByUUID`/`UpdateItemSessionEnded` pair) as part of
  the send-back action, rather than leaving a stale session running against a
  now-outdated plan. This is **not** the out-of-scope "steer a live session in
  place" (injecting feedback into a running session) — it's stopping a session
  that no longer matches the plan it was working from, the same distinction
  `forceResetItem` already draws for "Restart Session."

## 4. Unstated needs — visibility in item history

- **No existing feedback-driven retriage path writes a durable, visible
  progress-note record of the feedback text.** Checked: `RejectPlan`
  (`backlog_service_lifecycle.go:898-944`) does not call `AppendProgressNote` —
  it only persists `PlanRejectionReason`/`PlanRejectedAt` as item fields (later
  rendered inline by `PlanVerdictBox` while `changes_requested` lasts, but with
  no separate history trail once superseded). `TriggerTriage`
  (`backlog_service_trigger_triage.go`, `backlog_service_triage.go`) never calls
  `AppendProgressNote` with the feedback text either — feedback flows only into
  the LLM prompt via `BuildHeadlessRetriagePrompt`'s `priorResult`-aware
  builder.
- **The one place that *does* write a visible note for an operator-initiated
  redirect is `TransitionBacklogItemStatus`'s `override_reason` handling**
  (`backlog_service_lifecycle.go:791-800`): `note := fmt.Sprintf("Manually
  overridden by operator: %s -> %s (%s)", from, to, req.Msg.OverrideReason)` via
  `AppendProgressNote(ctx, itemID, -1, note, string(to))`, plus
  `s.notifyManualOverride(...)`. This is the only precedent in the codebase for
  "operator explains why they're redirecting the item, and that explanation is
  durably visible later" — and it matches this session's own standing memory
  (`feedback_document_ai_decisions_in_edge_cases.md`: "self-heal/auto-close
  actions should post a visible comment + notify(), not act silently").
- **Gap this creates for `plan-feedback`**: requirements.md's Success Metrics
  say the feedback should be "verifiably used by the resulting
  triage_result/plan diff... without leaving the item detail view" — that's
  satisfied by feeding `feedback` into `TriggerTriage` alone (the LLM prompt
  carries it, and the new `triageResult.summary`/suggestions reflect it). But
  neither prior feature answers "can a future viewer, reading this item's
  history later, see *why* re-planning was requested and by whom, once the new
  triage result has superseded the old one and the original feedback text is no
  longer surfacing anywhere in the live UI." `RejectPlan`'s reason only remains
  visible in `PlanVerdictBox` while `status === "changes_requested"` — once a
  new triage result lands, there's no rendered trace of the original rejection
  reason at all. `plan-feedback` should decide explicitly whether to follow the
  `override_reason` precedent (an `AppendProgressNote` call recording the
  feedback text and the status change, so it survives in history independent
  of whatever field currently holds it) rather than silently inheriting
  `RejectPlan`'s here-today-gone-tomorrow behavior — this is an unstated need
  the requirements doc doesn't call out directly but the user's own standing
  instinct on this project (see memory) argues for.
- **Notification parity**: `notifyManualOverride` fires for the
  `override_reason` path; `RejectPlan` fires no notification at all. If
  `plan-feedback`'s send-back is meant to be as visible/auditable as a manual
  status override (arguably more so — it also kicks off a new triage run), it
  should probably pair its progress note with a `notify()` call too, per the
  same standing memory.
