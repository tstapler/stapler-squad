# Research: Similar Features, Edge Cases, Unstated Needs (Agent 2 — Features)

**Question**: What similar features exist elsewhere in this codebase for the three gaps
(triage question answers, backlog-detail steering, plan reject/request-changes), what
edge cases/failure modes should the design handle, and what does the operator need beyond
the explicit ACs?

## 0. Gap 3 has a complete, orphaned prior design — reuse it, don't re-derive

`project_plans/plan-approval-ux/` is a finished Phase 1-4 SDD run for almost exactly this
item's Gap 3. Its `implementation/plan.md` (chosen approach "C — Incremental") already answers
this item's Open Question 1 and 2:

- **Open Question 1 answer** ("reuse `feedback`/status, or a dedicated RPC?"): the prior plan
  chose a **dedicated `RejectPlan(item_id, reason, expected_modified_at_unix_ms)` RPC**,
  explicitly rejecting reuse of `TriggerTriageRequest.feedback` directly (P3 in the plan's
  decision table: `RejectPlan` persists rejection state only; it does **not** itself trigger
  regeneration — that stays a separate, explicit "Regenerate with This Feedback" button that
  calls the existing `triggerTriage(id, reason)`). Rationale given: auto-invoking `TriggerTriage`
  inside `RejectPlan` would require refactoring `TriggerTriage`'s in-flight-guard/orphan-tombstone
  sequence (`backlog_service_triage.go:1864-1893`), judged out of proportion.
- **Open Question 2 answer** ("does `changes_requested` need to be a persisted status?"):
  explicitly **no** — the plan's decision table (P2) rejects making it a `BacklogStatus` enum
  value or a `BacklogStatusEvent` transition, because that breaks the `from_status != to_status`
  invariant every status-event reader assumes and requires bypassing the transition FSM's
  `validTransitions` whitelist. Instead: two new **durable but non-enum** fields
  (`plan_rejection_reason string`, `plan_rejected_at timestamp`) on `BacklogItem`, plus a
  **frontend-only derived** `PlanReviewStatus` union (`no_plan | pending_review | approved |
  changes_requested | skipped`) computed client-side from
  `planArtifactsPath`/`planApproved`/`planRejectionReason`/`skipPlanning` — never persisted as
  its own status. This satisfies this item's AC5 ("a state distinguishable from both 'plan
  approved' and 'plan never reviewed'... visible in the item detail view") **without** touching
  the backlog status machine at all.
- **Reference component**: `web-app/src/components/backlog/GateVerdictBox.tsx` is a complete,
  already-shipped, accessible approve/reject-with-reason UI (5-state verdict card, `readOnly`
  historical variant, reopen-with-feedback textarea, required-reason validation, keyboard
  shortcuts, `aria-live` announcements) for the *sibling* review gate. plan-approval-ux's own
  research called this "the pattern to copy, not a rough analog." This item's Request Changes UI
  should copy the same shape rather than invent a new one.

**Status of the prior work**: only the *backend* half was ever committed, and only to an
orphaned branch — commit `bc0955d41` (`feat(backlog): plan rejection state, RejectPlan/
GetPlanArtifactContent RPCs, widen stuck detection`) on `recover/plan-approval-ux`, currently
111 commits stale, never merged to `main`. It touches proto (`RejectPlanRequest/Response`,
`GetPlanArtifactContent*`, `expected_modified_at_unix_ms` optimistic-concurrency field),
`server/services/backlog_service_lifecycle.go` (+167), ent schema (3 new fields:
`plan_rejection_reason`, `plan_rejected_at`, `plan_artifacts_set_at`), and widens
`reconcilePlanNotApprovedItems`/`selfHealStuck` staleness detection to `ready`-status items, not
just `queued`. **No frontend code exists anywhere for this** — `PlanVerdictBox`/status-chip
component, `useBacklogService.rejectPlan()`, and the `ActionsSection.tsx`/`PlanArtifactsSection.tsx`
wiring in `implementation/plan.md`'s Epics 5-9 were planned but never written. Recommendation
for Phase 3: rebase/cherry-pick `bc0955d41`'s backend commit (after re-verifying it still applies
cleanly 111 commits later — schema/proto conflicts are likely) rather than re-implementing the
RPC and ent fields from scratch, then build only the frontend half fresh.

## 1. Gap 1 — triage question answers: the read-only rendering and the answer path already exist, just disconnected

- `web-app/src/components/backlog/TriageDiffSection.tsx:87-97` renders the "Triage Questions"
  section: `questionSuggestions.map((q, i) => <div key={i}>{q.text}</div>)` — plain text, no
  form, no per-question state, no stable identifier beyond array index `i`.
- `web-app/src/components/backlog/TriageReviewPanel.tsx:301-366` already has the exact
  interaction shape needed: a "Not quite — give feedback" toggle button → `<textarea>` → Submit
  calls `onRefine(feedback: string)` → `props.onRefine` ultimately calls
  `useBacklogService.triggerTriage(id, feedback)` (per requirements.md's cited hook line
  589/822). This is the *only* feedback-to-retriage path in the codebase today, confirming
  requirements.md's AC2 framing ("no new triage mechanism").
- **The missing piece is entirely in `TriageDiffSection.tsx`**: it needs (a) a per-question
  answer affordance (e.g. an inline "Answer" button/textarea next to each question item,
  following `TriageReviewPanel`'s toggle-button-then-textarea pattern for visual consistency),
  and (b) a way to compose that answer into feedback text that still identifies which question
  it answers. Since `submit_triage_result`'s `suggestions` array (verified in
  `server/mcp/tools_backlog.go:1933-1943`) has **no question ID field** — just `{text,
  rationale}` — the only stable identifier is the question's own text. The natural
  implementation is client-side string composition: prefix/quote the question text into the
  feedback string sent to `triggerTriage` (e.g. `Q: "<question text>"\nA: "<answer text>"`),
  since there's no backend concept of an "answered question" to persist. This matches
  requirements.md's own bias toward the "default to stateless" resolution of Open Question 2.
- **AC1 wording risk**: AC1 says "without retyping the question text" — the UI must not require
  the *operator* to retype it, but under the design above the *client* still composes the
  question text into the feedback payload sent to the backend (that's an implementation detail,
  not a UI retype, so it satisfies the AC's actual intent).

## 2. Gap 2 — steering from backlog item detail: the reusable dialog exists, but its visibility gate does not match this item's use case

- `web-app/src/components/sessions/SessionActionsOverflow.tsx:722-726`: the Steer menu item is
  gated `{onSteerAutonomousSession && session.autonomousMode && (...)}` — **it only renders for
  sessions with `autonomousMode: true`** ("Fix Autonomously" mode, a *different* feature: an
  `AutonomousDriver` loop on a regular interactive session, per `session/instance.go:552-554`).
  Headless backlog triage/review/work sessions are not spawned with `AutonomousMode: true` (grep
  of `server/services/backlog_service_triage.go` finds no `AutonomousMode` wiring at
  spawn-from-backlog-item time) — so naively adding `SessionActionsOverflow`'s existing overflow
  menu to `SessionsSection.tsx`'s session rows would **not** surface a Steer option for a typical
  backlog session, because the `autonomousMode` guard would block it. **This is the single most
  important gap-2 finding**: the plan phase must decide whether to (a) add a *new*, ungated Steer
  affordance directly in `SessionsSection.tsx` (simplest — a button per active session row,
  independent of `SessionActionsOverflow`'s autonomous-only gate), or (b) loosen
  `SessionActionsOverflow`'s gate when rendered from the backlog context. (a) is lower-risk and
  matches AC7's "reuse the `steer_session` *path*" (the backend call), not necessarily the exact
  overflow-menu component/gate.
- **The actual backend path to reuse** (AC7): the UI's Steer dialog does **not** call the
  `steer_session` MCP tool's ConnectRPC sibling directly — it calls
  `updateSession(sessionId, { steerMessage: message })` (see `web-app/src/app/page.tsx:289-292`,
  wired through `CockpitActionsContext.onSteerAutonomousSession`). This `UpdateSession` RPC with
  a `steerMessage` field is the concrete "existing `steer_session` path" AC7 refers to — the new
  backlog-detail Steer control should call the same `updateSession` mutation (or the same
  underlying hook), not a new RPC or a direct MCP-tool invocation.
- **Which sessions get a Steer control**: `SessionsSection.tsx` already computes an `active`
  session per item status (`statusToRole` map: `idea→triage`, `in_progress→work`,
  `review→review`, lines 61-83) and renders `<SessionMonitor>` only for that one active session.
  The Steer control most naturally attaches to that same `active` session, not every row — an
  ended/orphaned session (already labeled `ended` per `isOrphan`, line 113) has nothing live to
  steer.

## 3. Gap 2 edge case — does steering a headless session behave like an interactive one? (Open Question 3)

Traced `steer_session`'s server implementation (`server/mcp/tools_terminal.go:638-707`,
`findInstance` at :719-735) to answer this directly:

- `findInstance` looks up the session by ID first in `th.live` (in-memory live instances), then
  falls back to `LoadInstances()` from storage. A session that's fully torn down and not in
  either will return `ErrSessionNotFound` cleanly — no crash.
- For a session that **is** found: if it's `OneShot && ClaudeConversationUUID != "" &&
  GetEffectiveStatus() == session.Stopped`, `steerSession` uses `inst.RunWithResume(ctx,
  message)` — a **`claude --resume` subprocess**, not a live PTY write (method returned:
  `"resume_subprocess"`). This is the path a Stopped headless triage/review session with a saved
  conversation UUID would hit.
- Otherwise (session still running, or no UUID), it falls back to PTY `send-keys`
  (`inst.SendKeys(text)`, 5s timeout) — the same path used for a live interactive terminal
  session. **This means the two are not the same code path** — a headless OneShot session that
  has *already finished* (Stopped) steers via subprocess resume; one still *running* steers via
  PTY injection, identically to an interactive session. **Concretely answers Open Question 3**:
  behavior is conditional on session lifecycle state (running vs. Stopped-with-resumable-UUID),
  not uniformly "same as interactive" — the plan phase should design the backlog-detail Steer UI
  to handle both success shapes (`Method: "send_keys"` vs `"resume_subprocess"`) and to not
  assume a Stopped session simply can't be steered (it may still be resumable).
- `inst.SendKeys` failing (e.g. truly-dead PTY, no live instance, no UUID) surfaces as a clean
  `errResult(ErrInternalError, "send keys failed: ...")` from the MCP tool — but that's the
  MCP-tool code path; the UI path (`updateSession({steerMessage})`) needs its own equivalent
  error handling verified in Architecture research (Agent 3), since it's a different RPC than
  `steer_session` even though it should route to the same backend `SendKeys`/`RunWithResume`
  logic underneath.

## 4. Edge cases and failure modes to design for (all three gaps)

1. **Re-triage already in flight when a question answer is submitted (Gap 1).** Confirmed
   precedent: `backlog_service_triage.go:1884-1893` has a `triageInFlight` per-item guard that
   already prevents double-triggering triage for the same item — `triggerTriage(id, feedback)`
   (the same call `TriageReviewPanel.onRefine` uses) presumably already surfaces this as an error
   today; the question-answer UI just needs to handle that existing error path (e.g. disable
   submit while a triage is running, or surface the existing error message) rather than build new
   locking.
2. **Multiple unanswered questions, one submitted.** Since there's no persisted "answered"
   state (per Gap 1's stateless design above), submitting an answer to question A and then
   re-triaging will produce a **new** `TriageResult` with its own (possibly overlapping,
   possibly different) question list — the old questions B, C are implicitly superseded, not
   marked resolved. The UI must not carry stale question state across a `TriageResult` refresh;
   `TriageDiffSection` already re-renders fresh from `triageResult.suggestions` each time, so this
   should fall out naturally as long as no local component state persists across item reloads.
3. **Session already terminated when steering is attempted (Gap 2).** Covered in detail above
   (§3) — a Stopped OneShot session with a UUID is *still steerable* via resume-subprocess; only
   a session with neither a live instance nor a resumable UUID truly can't be steered. The UI
   should not hide the Steer control purely because `endedAt` is set — `isOrphan`/`ended` badge
   logic in `SessionsSection.tsx` already exists for *display*, but shouldn't gate the *action*
   without checking resumability, or should show a clear "this may fail" affordance if it does
   gate on `endedAt`.
4. **Plan already approved when Request Changes is submitted, or vice versa (Gap 3).** The
   orphaned `bc0955d41` backend already handles the "approve after reject" direction (`RejectPlan`
   clears `plan_approved` at the same write site, and `TriggerTriage`'s regeneration-completion
   write resets `plan_approved`/clears `plan_rejection_reason` — see that commit's message).
   Confirm during Architecture research whether the reverse direction (`ApprovePlan` called while
   a `plan_rejection_reason` is still set from a prior rejection) clears the stale reason —
   `plan-approval-ux/implementation/plan.md` line 308 notes the reason is "cleared on
   `ApprovePlan`" so this is already designed for, just needs re-verification post-rebase.
5. **Race between plan regeneration and a stale rendered/pending Request-Changes form.** Same
   class of bug as `plan-approval-ux`'s pitfalls doc: if an operator opens the Request Changes
   form, and in the background a new triage run regenerates the plan before they submit, the
   `expected_modified_at_unix_ms` optimistic-concurrency token (already designed in `RejectPlan`'s
   proto shape in the orphaned commit) is the existing mechanism to fail closed on that race —
   reuse it rather than re-deriving a new staleness check.
6. **Empty/whitespace-only Request Changes text.** AC4 requires this be impossible.
   `GateVerdictBox`'s Override form already has this exact validation precedent
   (`MIN_OVERRIDE_REASON_LENGTH = 5`, submit button `disabled`/`aria-disabled` until met) — reuse
   the same length-gated-submit pattern rather than a bare non-empty check, for UI consistency
   with the sibling gate.

## 5. Unstated operator needs beyond the explicit ACs

1. **No notification when a triage question is asked, or when a plan is rejected/approved.**
   Same finding `plan-approval-ux`'s research made for "plan ready to review" — grepping
   `server/services/backlog_service_triage.go` and `session/backlog_lifecycle.go` for `notify`/
   `Notif` call sites finds only failure-path notifications (`notifyReworkCapHit`,
   `notifyRepeatedFailure`), no success-path "a question is waiting for you" or "changes were
   requested" push. Both this item's Gap 1 and Gap 3 add new things an operator needs to notice
   proactively rather than discover by revisiting the item — a reasonable "Should Have" addition
   reusing the existing event-bus/`notify()` plumbing already used elsewhere in
   `backlog_lifecycle.go` (~20 call sites), even though requirements.md's "Out of scope" section
   explicitly defers "badges, notifications" as a follow-up. Worth flagging to the plan phase as
   a cheap addition if time allows, since the infrastructure already exists.
2. **Visibility into *which* question an answer was for, after the fact.** Because Gap 1's
   design is stateless (no persisted question-answer pairing), once a re-triage completes there's
   no durable record showing "operator answered question X with Y, which is why criteria changed
   this way." If the composed feedback string (`Q: ... A: ...`) is only sent to `triggerTriage`
   and not also displayed anywhere as a persisted item-history entry, this becomes invisible after
   the fact — the operator's own past input disappears once the next `TriageResult` supersedes it.
   Consider whether the composed Q&A feedback should also render in whatever "past feedback"
   or audit trail exists for the item (worth confirming in Architecture research whether one
   exists) rather than being pure fire-and-forget.
3. **Distinguishing "steering worked" from "steering silently landed nowhere."** Because
   `steer_session`'s PTY path can succeed (`chars_sent` > 0) even if the receiving session isn't
   actually paying attention (e.g. it's mid-tool-call and the injected keystrokes land in an
   unexpected place — a known category of PTY-injection UX risk), the backlog-detail Steer UI
   should set expectations similarly to the existing `SessionActionsOverflow` dialog (which
   already just fires-and-forgets via `onSteerAutonomousSession`) — not a new problem this item
   introduces, but worth carrying the same "sent" vs. "acted upon" distinction/toast language
   forward for consistency.
4. **A visible link from "Request Changes was submitted" to "here's the regenerated plan."**
   Per the orphaned plan's own scoping (P3: reject is persist-only, regeneration is a *separate*
   explicit action), the operator will submit Request Changes and then have to take a *second*
   action ("Regenerate with This Feedback") to actually trigger new triage. This two-click flow
   is an accepted, documented cost in `plan-approval-ux/implementation/plan.md` (not a gap this
   research is flagging as missing) — but this item's own AC2 for Gap 1 ("submitting an answer
   ... triggers (or explicitly queues) a re-triage") suggests Gap 1's UX may intentionally be
   more automatic (single-click, auto-triggers) than Gap 3's two-click design. The plan phase
   should explicitly decide whether Gap 1 and Gap 3 share the same one-click-vs-two-click
   philosophy or intentionally diverge (question-answering is lower-risk/cheaper to auto-retry
   than a full plan regeneration, so divergence is defensible) — call this out as a design
   decision, not an oversight, if the two gaps land with different click-counts.

## Sources (files read/grepped during this research)

- `project_plans/backlog-operator-feedback-loop/requirements.md` (full read)
- `project_plans/plan-approval-ux/requirements.md`, `research/features.md` (full read)
- `project_plans/plan-approval-ux/implementation/plan.md` (grepped for `changes_requested`,
  `RejectPlan`, status-enum decision — P2/P3 decision-table rows, Epic 3/6 detail)
- `git log`/`git show bc0955d41 --stat` (orphaned backend commit on `recover/plan-approval-ux`)
- `web-app/src/components/backlog/TriageDiffSection.tsx` (full read — read-only questions
  section)
- `web-app/src/components/backlog/TriageReviewPanel.tsx` (full read — existing refine-feedback
  form, the direct precedent for Gap 1's answer UI)
- `web-app/src/components/backlog/detail/SessionsSection.tsx` (full read — plain `<a>` session
  rows, `active` session derivation, `isOrphan`/`ended` badge logic)
- `web-app/src/components/sessions/SessionActionsOverflow.tsx` (grepped for Steer — dialog markup,
  `autonomousMode` visibility gate at line 723)
- `web-app/src/lib/contexts/CockpitActionsContext.ts`, `web-app/src/app/page.tsx:289-292`
  (`handleSteerAutonomousSession` → `updateSession({ steerMessage })` — the actual UI-side
  "steer_session path")
- `server/mcp/tools_terminal.go:627-735` (`steerSession` handler, `findInstance`,
  OneShot-resume vs. PTY-send-keys branching — Open Question 3)
- `server/mcp/tools_backlog.go:1919-1958` (`submit_triage_result` tool schema — confirms no
  question-ID field, only `{text, rationale}`)
- `session/instance.go:179-182,552-554,662` (`AutonomousMode` field — confirms it's an unrelated
  "Fix Autonomously" feature, not something headless backlog sessions set)
