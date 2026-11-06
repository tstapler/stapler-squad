# Research: Architecture + Root Cause — notification-actions-and-agent-messaging

Confidence labels: VERIFIED = file opened or live read-only query run (output saved under `/tmp/claude-1000/q/`); INFERRED = reasoned, not run.

## 1. Root cause of the empty Background tab

### Finding
The records do not exist because the **delivery gate drops them at the EventBus publish filter, before the history subscriber ever sees them**. It is intended behavior (ADR-001), not a bug in a producer or in prune. The tab is empty because the frontend join (`selectBackgroundRows`) is fed only by history, and history is by design a "needs a human" lane.

### Evidence chain
1. **Healthy hidden-session completion is classed routine.** `server/deliverygate/policy.go` `typeClass`: `TASK_COMPLETE`, `STATUS_CHANGE`, `INFO`, `AUTO_APPROVED`, `PROCESS_*` are `ClassRoutine`. Only `ERROR`/`FAILURE` (failure) and `APPROVAL_NEEDED`/`INPUT_REQUIRED`/`CONFIRMATION_NEEDED` (needs-human) pass for hidden sessions. VERIFIED (policy.go:117-138).
2. **The filter sits upstream of persistence.** `Gate.PublishFilter()` is installed via `EventBus.SetPublishFilter` (`gate.go:201`, `pkg/events/bus.go:75`). `notifications.StartSubscriber` subscribes to the same bus (`server/notifications/subscriber.go:41`) and `eventToRecord`s whatever it receives. A filtered event is never published, so no `NotificationRecord` is written. VERIFIED.
3. **Live counters confirm the drops.** `GetDeliveryGateStats` (`/tmp/claude-1000/q/gate.json`) shows `suppressed{channel=bus,reason=routine_for_hidden,type=TASK_COMPLETE}` with kind=`review` count 3 (other: 1) and `suppressed{channel=push_status,...STATUS_CHANGE}` kind=`review` count 2 since process start (uptime 5504s); hourly buckets show many `suppressed` kind=review/other, class=routine. VERIFIED.
4. **Zero history rows reference any of the 11 hidden sessions.** I looped all 11 ids from `ListSessions{hiddenOnly:true}` (10 `review:*`, 1 `diagnose:*`, all `SESSION_STATUS_STOPPED`) against `GetNotificationHistory` output: 0 matches each. History holds 40 records across 19 session ids, none hidden; types are INFO 13, TASK_COMPLETE 17, STATUS_CHANGE 9, WARNING 8, INPUT_REQUIRED 3 (the counts overlap categories; recount before quoting). VERIFIED.
5. **Prune is not the cause.** `PruneHiddenSessionNotifications` (`server/services/notification_prune.go:45`, `server/notifications/prune.go`) is operator-invoked (`Apply` flag, dry run default) and `grep -c prune ~/.stapler-squad/logs/staplersquad.log` = 0. INFERRED that it never ran on this instance (log path could differ if workspace mode is on; not checked).
6. **Why not "no producer".** Review sessions do produce events: the review queue maps `ReasonTaskComplete -> TASK_COMPLETE` and `ReasonUncommittedChanges -> STATUS_CHANGE` (`server/review_queue_manager.go:967-985`), and counters show those were suppressed. A failing review session would emit ERROR/FAILURE and pass; none did in this window. The `hidden_delivered needs_human kind=other` counter = 1 confirms the pass-through works.
7. **Secondary leak (not the cause here).** `unresolved` counters (e.g. 98 routine since start) are fail-open deliveries where the session id did not resolve in the visibility index; these *do* reach history, which is why history is full of visible/unresolved rows but no hidden ones.

### Verdict (which layer drops)
| Layer | Drops hidden routine records? |
|---|---|
| Producers (review queue, session exit) | No, they emit TASK_COMPLETE/STATUS_CHANGE |
| `deliverygate` `PublishFilter` | **Yes, by policy** (`routine_for_hidden`) |
| History subscriber / store | Never sees them |
| `PruneHiddenSessionNotifications` | Not run (INFERRED) |
| Frontend `selectBackgroundRows` | Correctly lists only failure/needs-human; also derives `completedOkToday`/`running` from the hidden-session list, so the *counts* have a source that is independent of history |

### Fix recommendation (finishes the requirements.md start)
- **Make the feed session-derived, not history-derived.** `selectBackgroundRows` already receives `hiddenSessions` (`HiddenSessionSummary`: state, updatedAtMs). Add an `activity` row kind built from the hidden-session list (title, kind from tag, state, updatedAt, linked backlog item / latest verdict via existing fields) so the 11 stopped sessions render as rows. Pinned failure/needs-human rows stay first. No server change needed for the first increment (lowest risk, matches "Background fix first").
- **Do not relax the gate.** Letting routine hidden events into history would reintroduce the noise ADR-001 removed and break `PruneHiddenSessionNotifications` assumptions. If durable activity history is wanted later, add a separate low-volume `background_activity` lane written by a gate sink on suppress (counted, never pushed), not a history row.
- Explanatory empty state keyed on `hasHiddenSessions`/`completedOkToday`/`running`, already plumbed through `BackgroundActivity` props (NotificationPanel.tsx:~800).

## 2. Architecture for the three capabilities

### 2.1 Existing building blocks (VERIFIED unless marked)
| Block | Location | Reuse for |
|---|---|---|
| `GuidanceRequest` ent entity (scope backlog-item/session/standalone; question_text, question_type yes-no/multiple-choice/short-answer, options JSON, answer, created/notified/answered/cancelled; status derived; dedup via `scope_key`) | `session/ent/schema/guidance_request.go`, `session/ent_repository_guidance.go`, `server/services/guidance_request_service.go` | The single question store (requirements constraint: no parallel store) |
| Hidden AskUserQuestion reply path (flag `hidden_session_reply`, single-select, digit keystroke, audited, rate limited, idempotent) | `server/services/hidden_question_reply.go`, `pending_question_store.go`, `session/pane_reply.go` | Live-session answer transport for Claude Code (tmux keystroke) |
| Approval handler/store/question | `server/services/approval_*.go` | Approval inline action |
| EventBus (in-process, ring buffer `EventsSince`, **not durable**, publish filter) | `pkg/events/bus.go`, `pkg/events/types.go` (events: session.*, notification, backlog_item_changed, workflow_changed, remote.health_changed) | Wake-trigger evaluation input; best-effort nudge only |
| Backlog event payload with Kind (status_transition, verdict_recorded, session_attached, ...) and in-process subscribers (`StartVerdictSteering`, `wait_for_backlog_event`) | `pkg/events/types.go:55-100`, `server/services/backlog_service_verdict_steer.go` | Event-based wake source and the model for a bus consumer |
| `SessionSteerer` injection path | used by verdict steer and PR-fix steering | Delivering a wake/answer into a live session |
| Cron scheduler (robfig/cron, `FireNow`, `FireTrigger`, admission gate, rate limiter, `TriggerFireEvent` ledger) | `server/workflows/scheduler.go`, `session/ent/schema/trigger_fire_event.go` | Time/cron wakes: **reuse the one cron engine**, do not add a second |
| Delivery gate + resolver (hidden classification) | `server/deliverygate` | Every new notification type must be classed in `typeClass` (exhaustiveness test enforces) |
| Prior design for answer delivery (live write, else respawn via headless.Pool, persisted row as durable fallback) | `project_plans/durable-guidance-request/research/architecture.md` §4, §7; `backlog-agent-communication` `RespondToHelpRequest` | Delivery semantics for answers and peer help |

Key prior conclusion carried forward: the event bus is a **best-effort low-latency nudge**; durable truth lives in the ent row read on demand (`wait_for_backlog_event` is a live ≤60 s call with no restart-reconnect).

### 2.2 Inline notification actions framework
- **Model:** a notification record gains a typed `actions[]` (kind, label, params, requires-confirm) computed server-side from the underlying entity, not stored on the history row. Kinds: `answer_question` (GuidanceRequest or live AskUserQuestion), `approve`/`deny`, `backlog_decision` (verdict accept/rework/merge), `open`. The client renders from the descriptor; unknown kinds degrade to `open`. This mirrors the `OmnibarAction` union + registry convention (types / dispatch / test kept in sync).
- **Single dispatch RPC:** `InvokeNotificationAction(record_id, action_kind, params, idempotency_key)` resolving to the existing handlers (AnswerGuidanceRequest, approval respond, hidden reply, backlog transition). One idempotency table (hidden reply already has one) and the existing approval-gate/caller-identity checks stay in the handlers, not the dispatcher.
- **Consistency:** the action must act on the *entity*, then the history row is marked resolved by an event (the `resolved_item_reaper.go` pattern), never the reverse. Stale actions (question already answered/cancelled) return a typed `already_resolved` outcome and the UI refetches.
- **Flag:** `notification_inline_actions`, live-settable, global + per-scope (reuse `FlagCache` plumbing).

### 2.3 Structured AskUserQuestion entity and answer routing
- **Entity:** extend `GuidanceRequest`, do not add a table. Additive columns: `source` (`agent_mcp` | `ask_user_question_hook` | `peer`), `asker_session_uuid`, `answerer_kind` (`human` | `session`) and `answerer_session_uuid` (answers open question 3: **reuse the entity with asker→answerer fields**; peer help is the same lifecycle with a different answerer), `multi_select` bool, `questions_json` for multi-question payloads (current schema is single question; `QuestionShapeMulti` exists in `pending_question_store.go` and is deliberately not replyable today), `delivery` (`live_keystroke` | `hook_reply` | `resume` | `read_only`), `delivery_attempts`, `answer_latency` derived from created_at/answered_at (already available).
- **Creation path:** a `PreToolUse`/hook for AskUserQuestion already feeds `PendingQuestionStore` (in-memory, 30 min TTL, 256 cap). Mirror each registered question into a `GuidanceRequest(scope=session, source=ask_user_question_hook)` so it becomes durable and renders in the pane for visible sessions too. Dedup via the existing `scope_key` upsert.
- **Answer routing (decision table):**
  | Session state | Route |
  |---|---|
  | Live Claude Code, dialog on screen, single-select | existing claim/digit keystroke path (generalize `hidden_session_reply` beyond hidden via a new flag; keep audit + limiter) |
  | Live, multi-select/multi-question | UNVERIFIED which harnesses allow it; keep `open` fallback until the harness capability matrix (other research file) says otherwise |
  | Live, MCP-originated question (`create_guidance_request`) | `write_to_session`/steer injection of the answer text (prior design) |
  | Stopped/archived | persist answer; deliver via explicit-user resume only (see 2.4 revival rule) |
  | Non-Claude harness | terminal-text detection + keystroke, or read-only (requirements Feasibility Risk) |
- **Open item for the harness research:** hook-mediated answer (`PreToolUse` deny-with-answer) vs keystroke injection vs SDK `canUseTool`. Keystroke is the only path VERIFIED in this repo; the others are UNVERIFIED here.

### 2.4 Agent messaging and durable wake triggers
Do **not** add a poll loop. Proposed substrate:

- **`WakeTrigger` ent entity** (new, additive): `id`, `owner_session_uuid`, `trigger_type` (`at`/`cron`/`backlog_event`/`pr_event`/`ci_event`/`session_state`/`guidance_answered`), `spec` (JSON: cron expr, item_id + kind filter, pr ref, peer uuid + target state, guidance_request_id), `action` (`steer_text` + payload), `status` (pending/fired/missed/cancelled), `next_fire_at` (nullable, indexed), `fired_at`, `fire_count`, `max_fires`, `created_at`. Indexes: `(status, next_fire_at)` and `(trigger_type, status)`.
- **Evaluator = one bus consumer + the existing cron engine, not a new scheduler goroutine:**
  1. Time triggers register with the existing `workflows.Scheduler` cron (or one `time.Timer` armed to the min `next_fire_at`, re-armed on change) rather than a ticker scanning rows.
  2. Event triggers: one `WakeDispatcher` subscribes to the EventBus (same shape as `StartVerdictSteering`), indexes pending triggers by `(trigger_type, key)` in memory, and on match does a conditional `UPDATE ... WHERE status='pending'` (fire-exactly-once, same answered_at-guard idiom as `AnswerGuidanceRequest`).
  3. **Restart recovery:** on startup, load pending rows; fire/mark `missed` any `next_fire_at` in the past; for event triggers, reconcile current state (item status, PR state, guidance row answered, peer session state) since the bus ring buffer is lost on restart. This is the durability the bus cannot give.
- **Peer messaging:** `GuidanceRequest` with `answerer_kind=session` is the message. Delivery to a live peer through the steer path; a peer with no live process is handled by the revival rule below. A new MCP tool needs a server-side caller-identity gate (prior instinct: `--allowedTools` is client-side only).
- **"Human answer arrives" trigger:** is just `guidance_answered` keyed on request id, fired from the same `AnswerGuidanceRequest` success path that already publishes the answered event (needs the `mapBacklogChangeKind` seam noted in the durable-guidance doc §3, or a new event type for non-item scopes; that doc says planning must pick explicitly).
- **Revival rule:** any wake targeting a stopped/archived session must pass the `noarchivedrevival` guard. Default policy: wake never revives an archived session; for stopped (non-archived) sessions it goes through the normal resume path only if the trigger was created with `allow_revive=true`; otherwise it records `missed` and raises an operator-visible notification (document AI/automated decisions visibly, per memory).
- **Capacity:** firing must pass the same admission gate as `Scheduler` (WIP cap 2 for backlog work, rate limiter) so wakes cannot cause the 2026-07-12 OOM pattern.
- **Flag:** `agent_wake_triggers`.

### 2.5 Data flow (target)
```
agent MCP tool / hook ──► GuidanceRequest (durable) ──► EventBus ──► notification record (gate-classed needs_human)
                                  ▲                                         │
operator clicks inline action ────┘ InvokeNotificationAction ──► AnswerGuidanceRequest (guarded UPDATE)
                                                                 │
                                       ┌─────────────────────────┴───────────────┐
                                       ▼                                         ▼
                              live: keystroke / steer                  stopped: persisted answer + WakeTrigger(guidance_answered)
                                                                                 │
EventBus (backlog/PR/CI/session) ──► WakeDispatcher ◄── cron/timer ──────────────┘ (restart: reconcile from DB)
```

## 3. Event-Command-Policy table

| Domain Event | Policy trigger | Command | Actor / System |
|---|---|---|---|
| `HiddenSessionActivityObserved` | Background tab renders / hidden session list changes | Derive activity rows from `ListSessions{hiddenOnly}`; no gate change | Web client (pure selector) |
| `NotificationSuppressedForHidden` | Gate decides `routine_for_hidden` | Count only (existing); optional later: write to a separate activity lane, never history | Delivery gate |
| `QuestionRegistered` (AskUserQuestion hook fires) | `PendingQuestionStore.Register` accepts a question | `UpsertGuidanceRequest(scope=session, source=hook)` mirror | Hook handler |
| `GuidanceRequestCreated` | MCP `create_guidance_request` or hook mirror succeeds | Publish event; classify as needs_human; fire Notifier | Storage + publisher |
| `NotificationActionInvoked` | Operator clicks an inline action | `InvokeNotificationAction` -> verdict/flag/idempotency/limiter -> entity handler | Operator via RPC |
| `GuidanceRequestAnswered` | Action `answer_question` succeeds (guarded `pending -> answered`) | Deliver by route table; set `notified_at` on success | RPC handler + delivery adapter |
| `AnswerDeliveryFailed` | Live write fails / dialog closed / stale prompt | Mark `delivery_attempts`, fall back to persisted read + `guidance_answered` wake; emit operator notification (no silent fail) | Delivery adapter |
| `ApprovalDecided` | Operator approves/denies inline | Existing approval respond handler (keeps approval-gate checks) | Operator via RPC |
| `BacklogDecisionMade` | Operator picks verdict/rework/merge inline | Existing backlog transition/verdict command, then resolve the notification via the reaper pattern | Operator via RPC |
| `WakeScheduled` | Agent calls `schedule_wake` (new MCP tool, caller-identity gated) | Insert `WakeTrigger`; arm timer/cron or index by key | Agent session |
| `WakeConditionMet` | Bus event matches an indexed trigger, or timer/cron fires | Guarded `UPDATE pending->fired`; pass admission/rate gate; steer the owner | `WakeDispatcher` |
| `WakeTargetUnavailable` | Owner is archived/stopped without `allow_revive` | Mark `missed`, notify operator; never auto-revive (`noarchivedrevival`) | `WakeDispatcher` |
| `ServiceRestarted` | Process start | Reload pending triggers; mark overdue `missed`/fire; reconcile entity state for event triggers | `WakeDispatcher` startup |
| `PeerHelpRequested` | Agent calls `request_help(peer)` | `GuidanceRequest(answerer_kind=session)`; steer peer if live; else operator-visible | Agent via gated MCP tool |
| `WakeSelfHealSweep` | Same cadence as existing reconcile sweeps | Retry unnotified answered rows; surface wakes stuck pending past bound | System sweep (no new ticker: piggyback `ReconcileStuck`) |

## 4. Consistency requirements
- **Exactly-once effects:** every state flip is a guarded conditional UPDATE (`answered_at IS NULL`, `status='pending'`); delivery is at-least-once with idempotency keys, so handlers must tolerate duplicate steers.
- **SQLite single connection** (`SetMaxOpenConns(1)`) serializes transactions; reuse that for dedup rather than DB unique constraints (same reasoning as the `scope_key` comment in the guidance schema).
- **Bus is lossy** (ring buffer, publish filter): never the only path. Every trigger type needs a state-reconcile on startup.
- **Check-then-reserve** (WIP/admission) needs a process-wide lock, not a per-trigger one (recurring bug class in memory).
- **Gate classification:** new notification types must be added to `typeClass` (exhaustiveness test) and given a class so hidden sessions' questions are not suppressed.
- **Proto/ent:** schema additions are additive; `make proto-gen`/`ent-gen` run in every Make target, generated output stays uncommitted. Remember the `--feature sql/upsert` flag.

## 5. Tech-debt disposition for touched hotspots

| Hotspot | Evidence | Disposition |
|---|---|---|
| `web-app/src/components/ui/NotificationPanel.tsx` | 837 lines; 29 hook calls (useState/useEffect/useMemo/useCallback); 6 commits in 90 days; owns tray segmentation, bulk ops, background, filters (VERIFIED by grep/wc/git log) | **Isolate via seam.** Do not add action rendering inline. Extract an `InlineActions` component keyed by the server-provided action descriptor, and a `useBackgroundActivity` selector hook boundary (`backgroundActivity.ts` is already a pure module). Panel only composes. Refactor-first is not justified: churn is moderate and the existing `BackgroundActivity` and `NotificationItem` already give natural seams. Watch the jscpd margin (0.10% headroom noted in memory) when adding tests; share fixtures. |
| `session/ent/schema/guidance_request.go` + `ent_repository_guidance.go` (481 lines) + `guidance_request_service.go` (420) | single-question shape, in-flight feature, well commented | **Extend as-is** with additive columns; the multi-question JSON is the only design choice. |
| `server/services/hidden_question_reply.go` | Reply path is hidden-only and flag-fail-closed | **Isolate via seam:** extract the "claim + audit + lease + one write" core behind an interface so visible-session and approval inline actions reuse it; keep the hidden flag semantics untouched. |
| `pkg/events/types.go` + `mapBacklogChangeKind` | switch panics (recovered) on unmapped kind, silently dropping events | **Extend as-is** but add a test that every new kind is mapped (prior doc §3 flags this seam). |
| `server/workflows/scheduler.go` | owns the only cron engine plus admission/rate gates | **Extend as-is**: register wake timers through it; do not fork a second scheduler. |
| `server/deliverygate` | exhaustiveness-tested policy table | **Extend as-is**; only add type classes. |

## 6. Open questions answered / left
1. Why no hidden records: answered in section 1 (gate publish filter, by design).
2. Harness native answer channels: not researched here; keystroke is the only VERIFIED path; see the harness-matrix research.
3. Peer help entity: reuse `GuidanceRequest` with `answerer_kind`/`answerer_session_uuid` (section 2.3).
4. Wake persistence/evaluator: new `WakeTrigger` ent table; evaluated by one bus consumer + existing cron engine, with startup reconcile (section 2.4).
Left open: whether to add a durable `background_activity` lane (recommended only if the session-derived feed proves insufficient), and the new-event-type vs synthetic-payload choice for non-item guidance events.
