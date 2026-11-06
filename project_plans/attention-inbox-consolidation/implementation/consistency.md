# Cross-Artifact Consistency: attention-inbox-consolidation

**Inputs**: `requirements.md`, `implementation/plan.md` (886 lines), `design/ux.md`, `decisions/ADR-001..006`.
**Method**: full read of all inputs; story/task/epic counts and file existence re-checked with grep/ls (93 tasks, 36 stories, 17 epics match the plan's effort table; `hook_deduper.go`, `useNowTicker.ts`, `relativeTime.ts`, `feature_flags_notification_actions.go` exist). Behavior of the live app and the source code was not re-verified here beyond that.

**Summary**: 22 findings: 2 BLOCKER, 12 CONCERN, 8 NITPICK. Highest severity: ADR-005 says `LastMeaningfulChangeAt` is written "through the same recorder chokepoint as `SessionEvent`", which plan.md explicitly rejected (adversarial B1) in favor of an always-on `SessionActivity` that works with `session_event_log` off.

## BLOCKER

**B1. ADR-005 vs plan: who writes `LastMeaningfulChangeAt`** (ADR-005 decision 2; ADR-004 "one chokepoint"; plan Event Ownership, Story 1.2.3, Story 4.1.1)
ADR-005 says the value is "written through the same recorder chokepoint as `SessionEvent` (ADR-004)", and ADR-004 describes the recorder as the single write path. The plan rejects exactly this (Verification Notes row "Recorder was assumed a single chokepoint", REJECTED): the field is advanced by the always-on `SessionActivity.Touch/touchLocked` with the recorder off, persists on the full-save cadence (accepting a crash loss window), and has one owner per event kind. An implementer reading the ADRs would build the rejected design, and the timer would stop advancing when `session_event_log` is off.
Resolution: amend ADR-005 decision 2 and ADR-004's "one chokepoint" wording to match the Event Ownership table (always-on `SessionActivity`, flag-gated recorder, full-save persistence with stated loss window); bump both to "Amended in repair iteration 3".

**B2. Requirement "UI to browse events across sessions (recently finished, unchanged for a while)" has no UI story** (requirements In Scope bullet 4; ADR-001 row "New read model"; plan Story 2.1.3, Epic 5.1/5.2; ux.md S6)
The RPC supports cross-session queries (`session_id:""`, `kinds:[TURN_ENDED]`, `since`), and ADR-001 says it "supplies history, 'recently finished' and the drawer". But the only consumers in plan/UX are the per-session drawer and queue-backed chips: "Finished" is open `awaiting_direction` items (gone after Done), and "Quiet" is client-derived. No story renders a cross-session recently-finished list, and UX's "Show done" filter (S4) is not in the plan either. The metric "which just finished" is unmet once an item is marked Done or was never raised (Hidden/flag-off).
Resolution: add a story (e.g. 5.2.3 "Recently finished" filter/section backed by cross-session `ListSessionEvents`, including the UX "Show done" recovery view), or state in requirements that the Finished chip plus per-session timeline is the accepted reduction.

## CONCERN

**C1. Notification for `AwaitingDirection`** (requirements In Scope bullet 2 "a reliable notification for each" and Problem 2; ADR-002 consequences; plan Story 3.2.3; ux.md S2)
Requirements want a reliable notification for both new reasons. ADR-002, plan (`TASK_COMPLETE`, "tray-only and silent") and UX ("silent: list and badge only") make `AwaitingDirection` silent. Problem 2 complains that completion produces "weak or no notification". Either requirements are narrowed or the design under-delivers.
Resolution: owner ratifies (with ADR-002) that "reliable" for `AwaitingDirection` means durable list row plus tray record, and edit requirements to say so.

**C2. Push/Slack interrupt level for `ProseQuestion`** (ux.md S2 table; ADR-002; plan Story 3.2.3)
UX: prose question is "toast (tier 2)", approvals are "toast + push". Plan maps it to `NOTIFICATION_TYPE_INPUT_REQUIRED`, which "already reaches tray, toast, web push and Slack", so a heuristic-detected question would push to the phone and Slack like a real prompt, before the precision gate is proven.
Resolution: decide in Story 3.2.3 whether prose questions suppress push/Slack (metadata `attention_reason` check in the gate) or update UX; Spike 0.1.4b should record the answer.

**C3. Nav/tab badge definition** (ux.md S10, S8 vs S2 and plan Story 3.2.1c/5.2.2/6.1.1)
UX S10/S8: badge counts needs-me only, excluding informational rows. UX S2 says `awaiting_direction` is silent "(list and badge only)", implying it is in the badge. Plan Story 3.2.1c makes the two new reasons visible via `reviewQueueVisibility`, which feeds the existing badge, so `AwaitingDirection` would inflate the badge; no plan task changes the badge to needs-me-only (`ReviewQueueNavBadge.tsx` is listed under Files without an AC). Document title "(N)" (Story 5.2.2) has the same ambiguity.
Resolution: pick one definition, fix UX S2 wording, and add an AC to Story 5.2.1/5.2.2 asserting badge, tab badge and title count needs-me only.

**C4. ADR-004 says prune on delete and archive; plan keeps archived rows** (ADR-004 Decision; plan Story 2.1.4 title vs AC)
Story 2.1.4 is titled "Prune on delete and archive" but its AC says "archived sessions keep rows until retention". ADR-004: "explicit prune on delete/archive".
Resolution: retitle Story 2.1.4 "Prune on delete" and amend ADR-004 to "prune on delete; archived rows age out by retention".

**C5. Excerpt length 200 (plan/ADR-004) vs 160 (UX)** (ADR-004 fields; plan Story 3.1.1, 3.2.3, Migration `turn_end_excerpt` <= 200; ux.md S2, S6, UX-37)
UX-37: "no row, tray item or timeline entry shows more than 160 characters". Plan stores and sends up to 200 (toast message is "the scrubbed excerpt"). Toast/tray would breach UX-37.
Resolution: choose one cap for display (160) and apply it in the notification message builder as well, or relax UX-37 to 200.

**C6. Which text the excerpt contains** (ux.md S2 "show the matched sentence"; plan Story 3.1.1/3.1.2; ADR-003)
UX requires the matched question sentence so the operator can judge a false positive (UX-02). The plan's classifier returns only `(Kind, Confidence)` and the excerpt is "<= 200 chars" of the last assistant message with no rule for which part. A long message ending in a question would show its opening, not the question.
Resolution: add to Story 3.1.2 an `ExtractQuestionSentence`/tail-biased excerpt rule with a golden test.

**C7. Hidden sessions in the Attention list** (ux.md S1 "Show hidden" toggle; plan Event Ownership "Hidden-session decision", `shouldSkipSession` at `:828`)
UX offers "Show hidden" including hidden sessions with a tag. Plan states Hidden sessions never get `ReviewItem`s, so the toggle would show nothing. No story implements the toggle.
Resolution: drop the toggle from UX, or add a story that sources hidden rows from the session list/event log (and keep them from notifying).

**C8. Paused/Stopped group has no data source** (ux.md S1, UX-08; plan Story 5.2.1 AC "Paused/hidden"; requirements In Scope bullet 3 "only if research shows it matters")
`shouldSkipSession` skips Paused/Stopped/Crashed/Hibernated, so the queue has no such items. Plan's AC asserts a collapsed "Paused (1)" group without saying where it comes from (session list join?). Requirements make this secondary and conditional on research; UX makes it a required group.
Resolution: specify the source (client join with the sessions slice, no queue change) in Story 5.2.1 or mark it deferred.

**C9. Undo, bulk "mark all seen", and "Show done" have no plan tasks or API** (ux.md S3, S4, UX-03, UX-07; plan Story 3.2.4, Task 5.2.1e)
UX requires a 10 s Undo for "Not a question" (via "re-add RPC/refetch"), Undo for Done and bulk mark-seen, a bulk "Mark all finished as seen" action, and a "Show done" view. Plan has none (grep for undo/bulk/Show done in plan.md returns no matches). Undo needs a server-side un-ack (clear `AckedEpisodeID`, re-raise item) which interacts with ADR-006's episode rule and the persisted columns.
Resolution: add stories for un-ack (RPC `dismiss_kind` inverse or `UnacknowledgeSession`), bulk informational ack, and "Show done"; or drop Undo from UX.

**C10. `ListSessionEvents` has no pagination cursor** (ux.md S6 "Load older", 50 per page; plan Story 2.1.3)
Plan request fields: `session_id`, `kinds`, `since`, `limit` (max 500). "Load older" needs a `before`/page token. `since` filters the wrong direction.
Resolution: add `before_occurred_at`/`page_token` to the proto in Task 2.1.3a and an AC for page 2.

**C11. `LastMeaningfulChangeAt` advance rules differ** (ADR-005 decision 2 vs plan Event Ownership)
ADR-005: output, "transition into/out of an attention state", user input, TurnEnded. Plan: output, every `transitionTo` (any status change), TurnEnded, lifecycle hooks; no user-input source. A status flip between non-attention states now counts; user input does not. This also affects UX "Quiet" (30 min) and the card label.
Resolution: align ADR-005 with the plan (or add user input as a `Touch` caller) and state whether every status transition counts.

**C12. Redirect permanence** (requirements Risk Control "until the flag-gated redirect is removed in a later PR" vs ADR-001 "kept permanently ... never deleted")
Requirements anticipate removing the redirect; ADR-001 and plan Story 6.2.1 keep `/review-queue` forever.
Resolution: edit requirements Risk Control to match ADR-001 (it is the later, owner-pending decision).

**C13. Screen-text classification contradicts ADR-003 point 3** (ADR-003 points 3 and 5; plan Story 3.1.3)
ADR-003 point 3: classifier is "applied only to the last assistant message, never to screen tail". Point 5 and plan Story 3.1.3 run a screen-fallback stage on the bottom-most prose paragraph and classify "ends with a question" for non-Claude programs. Plan already ships those as `Finished`-only until a corpus exists.
Resolution: reword ADR-003 point 3 to "never to a raw screen tail; the screen stage extracts the bottom-most paragraph first and ships Finished-only until the per-program corpus gates it".

**C14. `tests_failing` classification** (ux.md S1/S2 needs-me; plan `NeedsHumanReasons` = {Approval, InputRequired, Error, ProseQuestion}; ADR-006 informational list)
UX puts `tests_failing` in NEEDS ME with no Done. It is in neither the plan's needs-human set nor ADR-006's informational set (AwaitingDirection, TaskComplete, UncommittedChanges), and UX's "informational stale" is not in ADR-006 either. "Done absent on needs-human rows" is then undefined for these reasons.
Resolution: list every `AttentionReason` once in a table (needs-human / informational / Done allowed) in plan.md and reference it from UX and ADR-006.

## NITPICK

**N1. `useNow` vs `useNowTicker`** — UX S7/S12 and ADR-005 point 5 say a new `useNow(30_000)`; plan uses the existing `useNowTicker` and adds no hook. Use `useNowTicker` in UX/ADR-005. The plan's Tech Debt row still says "Three local `useNow` + 4 `formatTimeAgo` files" but Verification Notes found one `useNow`; fix the row.

**N2. Term naming drift** — requirements say "finished-awaiting-context"/"paused"; plan, ADR, UX use `AwaitingDirection` / "Finished, waiting for direction". Add the mapping to the requirements or glossary.

**N3. Stale glossary/count refs** — plan Review Disposition says "34 terms" but the glossary lists 35. ADR-004 "scrubber added in Story 2.2" refers to a story that does not exist (scrubber is Story 2.1.2, `session_event_scrub.go`).

**N4. Flag set** — requirements Risk Control names 3 flags; plan registers 6 (adds `attention_ack_bypass_fix`, `attention_idle_clock_output`, `attention_prose_question_llm_tiebreak`). ADR-005 decision 4 states the idle-clock switch unconditionally, but the plan gates it. Update requirements and ADR-005 to list the plan's flags.

**N5. Usage metric has no story** — requirements Success Metrics "review/attention surface opens per day via `usePageView`" has no task (grep finds no `usePageView` in plan). Add an XS task or mark observational/out of plan.

**N6. UX surfaces without plan ACs** — SessionCard "Created" label (S7; Story 4.2.2 only covers Last activity), tray-to-list edge banners and "Linked" tag (S5), keyboard shortcuts j/k/t/d/? (UX-26), "Seen" marker persistence (where stored is undefined; glossary says implicit mark), "Attention" copy rename of nav label/page title (ADR-001 "UI copy only"; no task, and `route-meta.ts` title must match page metadata per nav-facade rule). Add ACs or mark deferred.

**N7. UX layout wording** — ux.md header says "768-1023 px uses the mobile layout with the desktop drawer"; S6/UX-11 say drawer >= 768, sheet < 768. Plan uses < 768 sheet. Fix the header line.

**N8. Observability metric** — requirements ask for notifications "emitted vs delivered vs suppressed"; plan counters are `notifications_emitted|suppressed_total`, no `delivered`. Add or define delivered = emitted minus suppressed.

## Checked and consistent

- Event kinds: ADR-004, plan glossary/Event Ownership, UX wording map list the same 10 kinds (Lifecycle split into 3).
- Retention: 200 rows / 30 days per session, ~50k global in ADR-004 and plan 2.1.1c; UX "older events trimmed" matches `trimmed=true`.
- Quiet threshold 30 min: UX S1 and plan Story 5.2.1.
- Undo window 10 s appears only in UX (no conflict, but see C9).
- Routes/queries: `/review-queue?session=<id>` (ADR-001, plan 5.2.2, UX S5), redirect to `/unfinished?tab=sessions&session=..&filter=..` (plan 6.2.1, UX S11), default-tab order (ADR-001, plan 6.1.1, UX S8). The `filter=needs` value is not defined in the plan (see N6 scope).
- Reason names/proto: `prose_question` / `awaiting_direction`, proto values 11/12, `Medium`/`Low` agree across ADR-002, plan, UX.
- ADR-006: episode ack, unconditional for new reasons, `attention_ack_bypass_fix` for `InputRequired`, 4 h snooze expiry agree with plan Stories 1.2.2 and 3.2.2.
- Dismissal semantics ("this episode only") agree across UX S3, ADR-006, plan 3.2.2/3.2.4.
