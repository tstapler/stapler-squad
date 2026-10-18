# Research: Features + current-state inventory

Date 2026-10-17. Method: sg/grep and targeted reads of the worktree at branch `stapler-squad-durable-actions` (HEAD cc55228c8). Citations are `path:line` in this repo. Label VERIFIED = opened in code this session; INFERRED = deduced; UNVERIFIED = not checked. Plan text was NOT treated as evidence of shipped status.

## 1. Inventory: what already exists

### 1.1 Notifications (server)
| Piece | Where | State |
|---|---|---|
| Notification types (14 + CUSTOM) | `proto/session/v1/types.proto:1012-1038` | VERIFIED |
| History store + coalescing subscriber (bus -> `NotificationRecord`, metadata map persisted) | `server/notifications/subscriber.go:34-60,179-205` | VERIFIED |
| RPCs: SendNotification, GetNotificationHistory, MarkNotificationRead, ClearNotificationHistory, PruneHiddenSessionNotifications, GetDeliveryGateStats | `proto/session/v1/session.proto:109,152,156,160,166`; impl `server/services/notification_service.go:114-380`, `notification_prune.go:45` | VERIFIED |
| MCP: only `get_notification_history` (read) | `server/mcp/tools_notifications.go:154` | VERIFIED. No MCP tool to create a notification or ask the human |
| `ssq-notify` shell script: type names map to legacy enum numbers (`question` -> INPUT_REQUIRED=2) | `scripts/ssq-notify:1-50`; enum parity test `server/services/ssq_notify_enum_parity_test.go` | VERIFIED |
| Item/system notifier (`Notify(itemID,...)`, `NotifySession`, `NotifyAutoRemediating`) publishes `NewNotificationEvent` | `server/services/backlog_notifier.go:19-59`, interface `session/backlog_lifecycle.go:41-48` | VERIFIED |
| Delivery gate (single publish filter, copy-on-write visibility index, per-kind live flags, fail-open, counters) | `server/deliverygate/gate.go:201-372`, policy `server/deliverygate/policy.go` | VERIFIED. Default on (commit 0b3ad5387) |
| Slack notifier, web push, stale-session / memory-pressure notifiers | `server/services/slack_notifier.go`, `server/push/`, `stale_session_notifier.go`, `memory_pressure_notifier.go` | VERIFIED present; contents out of scope |

**Gate policy (VERIFIED, `policy.go` typeClass table + `ShouldDeliver`)**: hidden session -> routine types (TASK_COMPLETE, PROCESS_*, INFO, STATUS_CHANGE, AUTO_APPROVED, WARNING, CUSTOM) are SUPPRESSED before they reach the bus or history; only ERROR/FAILURE (failure) and APPROVAL_NEEDED/INPUT_REQUIRED/CONFIRMATION_NEEDED (needs-human) are delivered. Hints can promote WARNING to failure (`HintFailure`) or demote per-tool ERROR (`HintRoutine`), never demote needs-human.

### 1.2 Notification UI (web-app)
- `NotificationPanel` (tray) with a "Notifications | Background" segment: `web-app/src/components/ui/NotificationPanel.tsx:759-814`; `NotificationsPage` at `web-app/src/app/notifications/NotificationsPage.tsx`; toast `NotificationToast.tsx`; nav badge `NotificationsNavBadge.tsx`.
- Inline actions that exist TODAY in `NotificationItem.tsx`: Approve / Deny / "Approve anyway" (CI-blocked) via `resolveApproval` (`:257-326`), mark read/dismiss/remove, "Open session". Backlog-linked rows only get a link to `/backlog?item=<id>` (`:339-344`). No answer/option UI on question rows.
- Background tab: `useBackgroundSessions` polls `ListSessions{hidden_only}` every 30s only while the segment shows (`lib/hooks/useBackgroundSessions.ts:15-40`); `selectBackgroundRows` (`lib/utils/backgroundActivity.ts:86-125`) emits a row only for a hidden session with an UNREAD failure/needs-human history record; routine completions are "counted, never listed" (doc comment `:78-85`). Empty states exist: `background-empty-healthy`, `background-empty-none` (`components/ui/BackgroundActivity.tsx:184,195`).

### 1.3 Reply / answer paths
1. **Hidden-session AskUserQuestion Reply** (the only live-answer path). Chain, all VERIFIED:
   - Claude `PermissionRequest`/PreToolUse hook -> `ssq-hooks` -> `ApprovalHandler`; for tool `AskUserQuestion` it publishes INPUT_REQUIRED "Claude has a question" and answers the hook with a DEFER (native dialog stays) `server/services/approval_handler.go:436-447,783-815`.
   - `ParseAskUserQuestion` classifies single / multi / unknown; only single-select with 1..7 non-empty labels, question and header present is replyable; `multiSelect` absent = unknown `server/services/pending_question_store.go:393-440`.
   - `PendingQuestionStore` is IN-MEMORY, 256 total / 8 per session, TTL 30 min (`pending_question_store.go:16-27`). Lost on restart. Registration needs a verified sender proof (`approval_question.go:79-145`, `HookProofs`).
   - `ReplyToPendingQuestion` RPC (`session.proto:173`, impl `hidden_question_reply.go:285`): localhost-only (`EvaluateLocalWrite`), flag `hidden_session_reply` default ON (`:90-122`), rejects visible sessions with `NOT_HIDDEN` (`:303`), per-session 1 reply/5 s and 10/min global (`:36-39`), idempotent `reply_id`, claim/lease/audit, one digit keystroke, no Enter, no retry, `DialogMatch` screen verification before writing (`session/pane_reply.go:1-60`).
   - UI: `ReplyCard` (one numbered button per option, no text field) rendered ONLY in session detail (`components/sessions/SessionDetail.tsx:154`), not in tray/notification rows. Notification metadata carries `question_id`, `question_shape`, `question_options`, `reply_unavailable` (broadcast comment `approval_handler.go:783-788`).
2. **Approvals**: `ResolveApproval` / `ListPendingApprovals` (`session.proto:139,143`), classifier rules, auto-approve rows; inline in tray (above). Secret scan gate `pkg/threatscan` (commit 3469967d6).
3. **Raw terminal injection MCP tools**: `write_to_session`, `send_control`, `steer_session` (`server/mcp/tools_terminal.go:91,110,147,707-760`). `steer_session` uses the write lease + `SubmitContentWithEnter` or `claude --resume` for stopped one-shots. NO sender identity, NO hidden-session refusal, NO rate limit in the handler body I read (only `withDiagnoseGate` blocks diagnose callers `:158`). This is today's de-facto peer messaging, unattributed.
4. **Server-driven steer to a work session**: `StartVerdictSteering` subscribes to backlog events and steers the item's live work session with the review verdict, gated by `SessionSteerer.IsHiddenSession/IsReadyForSteer` (`server/services/backlog_service_verdict_steer.go:1-40`; interface `backlog_service.go:100-112`). This is the only existing "event wakes a named session" mechanism and it is verdict-only.
5. **Guidance requests** (durable Q&A): ent `GuidanceRequest` (`session/ent/schema/guidance_request.go`, status derived), RPCs Create/Answer/Get/List/ListAllPending (`proto/session/v1/guidance_request.proto:18-35`), MCP `create_guidance_request` / `get_guidance_request` (`server/mcp/tools_guidance.go`, ownership checks `:70-110`), types yes-no / multiple-choice / short-answer, per-scope pending cap (`PENDING_CAP_EXCEEDED`). UI: `GuidanceRequestPanel` embedded in `SessionDetailView.tsx:1696`, `BacklogItemDetail`, `TriageReviewPanel` ONLY; it is not referenced from `NotificationPanel`/`NotificationItem` (grep empty). **Answer delivery gap (VERIFIED `guidance_request_service.go:68-95,317-350`)**: after AnswerGuidanceRequest, `notifyAnswered` publishes an INPUT_REQUIRED notification addressed to the originating item/session UUID (i.e. to the human tray), and only scope=backlog-item respawns TRIAGE (`resumeTriageIfHalted`). For scope=session the agent is NOT messaged; it must call `get_guidance_request`. Answer is human-only (no ownership check, requirements out-of-scope).
6. **Omnibar**: session-creation and action dispatcher; no reply-to-question action found in this pass (UNVERIFIED beyond grep of NotificationPanel; not deeply searched).

### 1.4 Backlog decision surface
- Web RPCs (`proto/session/v1/backlog.proto`): TransitionBacklogItemStatus `:1562`, TriggerTriage `:1571`, CancelTriage `:1574`, TriggerReReview `:1591`, SubmitManualReview `:1779`, ListStuckBacklogItems `:1784`, SnoozeStuckItem `:1788`, ResetStuckRemediation `:1796`, BulkReset `:1803`, WatchBacklogItems stream `:1832`; `MergePR` in `session.proto:102`. UI callers only in `components/backlog/BacklogItemDetail.tsx`, `lib/hooks/useBacklogService.ts`, `useStuckBacklogItems.ts` (grep). Notification rows for backlog events link out (`NotificationItem.tsx:339`), no inline decision.
- MCP (agent-facing): `transition_backlog_item_status`, `submit_review_verdict`, `request_review`, `report_blocked`, `report_progress`, `report_pr_created`, `report_duplicate`, `post_backlog_update`, `submit_triage_result`, `submit_diagnosis_result`, `create_guidance_request`/`get_guidance_request`, `wait_for_backlog_event`, `resume_work`, `link_session_to_item` etc. (tool list from `mcpgo.NewTool(` across `server/mcp/*.go`).
- `wait_for_backlog_event` is capped at 3 consecutive timeouts per session/item and then REFUSED, with prompts telling agents not to poll (`server/mcp/tools_backlog.go:817-885`). Prompts in `session/backlog_context.go:120`, `backlog_commands.go:234`, `pipeline_mode_seed.go:170,412` forbid `ScheduleWakeup` and `/loop`.

### 1.5 Question / prompt detection
- Hook-based (structured, Claude only): see 1.3.1 (payload `{questions:[{question,header,options[{label,description}],multiSelect}]}`, `pending_question_store.go:383-395`).
- Screen-based (all harnesses): `session/detection/binaries/claude.go:420-450` `InputRequired` patterns: `[❯●]\s*\d+\.\s+\w` numbered selector, OpenCode `┃ N.` options and "Allow once/Allow always" buttons. Detection yields only a status (INPUT_REQUIRED), no option extraction. `StatusReady` is dead code per memory (do not use).
- `internal/claudehooks/claudehooks.go:68-92` installs PreToolUse (rules), Notification and Stop hooks. `ssq-hooks` also parses OpenCode and Gemini payloads (`cmd/ssq-hooks/main.go:354-431`); no structured question path for them found.

### 1.6 Scheduling / wake / event substrate
- `EventBus` (`pkg/events/bus.go:25-190`): in-process pub/sub, per-subscriber bounded channel (drops when full), replay ring `EventsSince(seq)`, publish filter used by the gate. NOT durable across restart. Event types: session.*, notification, approval_response, backlog_item_changed, remote.health_changed, workflow_changed (`pkg/events/types.go:16-47`). No CI/PR-event type there (PR events live in the separate `pr-event-webhooks` / `webhook-triggers` plans; UNVERIFIED which shipped).
- Cron exists ONLY for Workflows: `server/workflows/scheduler.go` (robfig/cron v3, 5-field), spawns workflow sessions; restart handling is warn-only missed-fire detection, "never replay-fires" (`:570-590`). Ent `Workflow` has cron/webhook/github_push triggers (`session/ent/schema/workflow.go:28-55`), `TriggerFireEvent` audit table. None of it re-prompts an existing session.
- `ScheduleWakeup` is a Claude Code CLIENT tool; the server has no code for it (only prompt text forbidding it) and no detection of a session "sleeping on a wakeup". So the server cannot see, persist, or survive-restart a client-side wake.
- Server steer into a session: write lease + `IsReadyForSteer` idle check (verdict steer, above) is the reusable delivery primitive.

## 2. Existing plans: shipped vs planned (verified against code)
| Plan | Verdict | Evidence |
|---|---|---|
| `durable-guidance-request` | SHIPPED (PR #809, commit 19d61b8ab): entity, RPCs, MCP tools, 3-view panel. Gaps vs this ask: no tray integration, no agent wake on session-scope answer, no agent-submitted answers, no multi-question | sections 1.3.5 |
| `notification-tray-and-hidden-session-gate` | MOSTLY SHIPPED: gate, tray, read-only hidden view (#974), default-on (#987), tray default (#1031), Reply (ADR-010) with flag default on. Plan text still says re-review "PENDING" so its status header is stale; `hidden_session_gate` and `hidden_session_reply` flags exist in code; I found no `notification_tray_v2` string in server/session/config (UNVERIFIED in web-app) | commits 0b3ad5387, a9b19d863, 666a7a54c; grep |
| `notification-revamp` | Not re-verified in detail (dedup/collapse, reconciliation); coalescing buffer exists (`subscriber.go:61-105`) | partial |
| `backlog-event-subscribe` (#428) | SHIPPED as `wait_for_backlog_event` (#447) PLUS the stronger replacement: server-side verdict steer (#929). The ask's "zero new ScheduleWakeup loops" is already policy for backlog verdicts; remaining polling exists for non-verdict waits | tools_backlog.go:817; verdict_steer.go |
| `backlog-agent-communication` | PARTIAL: report_blocked, create_guidance_request exist; ADR-003 defers a persistent master agent; infra-issue report entity not verified | MCP tool list; ADRs |
| `webhook-triggers` / `pr-event-webhooks` | Schema (`TriggerFireEvent`, `WebhookRegistration`, `WebhookRequestLedger`) exists; whether PR/CI events can wake a session is UNVERIFIED | ent schema list |

## 3. Why the Background tab is empty (answers Open Question 1, VERIFIED by code, not by live run)
It is by design, not a bug in selection: the gate suppresses ALL routine types for hidden sessions before the bus (`policy.go` `ShouldDeliver`, `gate.go:334-372`), so a healthy review/diagnose/triage session never writes a history record, and `selectBackgroundRows` only reads history. 11 healthy stopped review sessions therefore produce zero rows. `PruneHiddenSessionNotifications` is operator-invoked only and cannot explain it. Producer-side: review sessions rarely emit INPUT_REQUIRED/ERROR. A real activity feed needs a NON-notification data source (the hidden-session list + backlog item/session status + last-activity) — the tab already fetches hidden sessions every 30 s but only uses them as join keys. Do not "fix" it by loosening the gate (would re-open the notification flood the gate was built to stop).

## 4. Gaps (explicit)
1. No inline answer UI in tray rows: `ReplyCard` only on session page; guidance panel only on 3 detail views; backlog decisions link out.
2. Reply is hidden-session-only. A visible live session's AskUserQuestion has NO out-of-band answer path (`NOT_HIDDEN`, `hidden_question_reply.go:303`), and for hooks the handler always DEFERs to the native dialog; answering through the hook (PermissionRequest/PreToolUse returning an answer) is not implemented. UNVERIFIED: whether Claude Code PreToolUse can return a tool result for AskUserQuestion (research stack file).
3. Multi-question, multiSelect, free-text "Type something" answers are explicitly not replyable (`pending_question_store.go:31-40`); only digits 1-7.
4. Pending-question registry is in-memory with 30-min TTL; after restart or TTL the card degrades to "Answer in the terminal" although the dialog is still open. No durable structured question object for live AskUserQuestion (it is NOT a GuidanceRequest).
5. Non-Claude harnesses: only screen-regex INPUT_REQUIRED; no option extraction, no reply.
6. Guidance answer does not reach a live asking session (scope=session) and only triage is auto-resumed for item scope; no generic "answer arrives" wake.
7. No peer help request: `GuidanceRequest` has no asker/answerer-session fields; `steer_session` is unattributed raw injection and can write to any session including hidden ones.
8. No durable wake table, evaluator, or trigger types (delay, cron-on-session, PR/CI event, other-session state, human-answer). EventBus is volatile; workflow cron never replays after downtime.
9. Metrics asked for in requirements (inline-action invoked/succeeded, question created->answered latency, wake counters) do not exist; guidance entity has created_at/answered_at so latency is derivable.
10. Notification rows for backlog decisions have no stable "decision schema" (options, verdict choices) in metadata; only free text + `item_id`.

## 5. Edge cases and failure modes (for design/ADR/tests)
- Answer races: two tabs/devices answer the same question (guidance: conditional UPDATE gives `applied=false`; Reply: claim + `reply_id` idempotency; approvals: reconciliation race already handled in `NotificationItem`). Inline actions must surface "already answered by X" not an error.
- Stale prompt: user answers after the dialog closed or changed (`DialogMatch` -> `stale_prompt`); after Claude moved on, a digit keystroke lands in the next prompt. Keep screen verification; no blind keystrokes.
- Session paused/stopped/archived while question pending; `noarchivedrevival` forbids reviving to deliver an answer. Durable answer must wait for explicit resume.
- Restart while question pending or wake scheduled (store lost, bus buffer lost, cron not replayed).
- Hook proof missing/spoofed (title-header attribution ignored when proof verifies, `approval_handler.go:445-448`); remote `:8444` access vs localhost-only reply (`EvaluateLocalWrite`): inline actions from a phone over the private-CA remote port are blocked for Reply today. This is a major unstated user need (mobile).
- Rate limits (1/5 s, 10/min) will throttle a user clearing a queue of questions; burst UX needs explicit copy.
- Multiple parallel AskUserQuestion calls (`pendingParallelWindow` 30 s), same question text from different sessions, option labels > 80 runes or > 7 options.
- Unknown/multi-shape questions need a graceful "open session" fallback with the question text still shown.
- Prompt-injection / secrets: option labels and question text are model-controlled; sanitize (`sanitizeNotificationText`), never auto-answer; answers typed by the human travel as keystrokes (secret-scan story for free text).
- Wake storms: N sessions waking on one event; every wake re-reads whole context cost (rationale of existing no-poll rule); need dedupe, coalescing, max fires, cooldown, and a cap analogous to `maxWaitTimeoutsPerSession`.
- Delivery to a busy session: `IsReadyForSteer` unknown must mean "do not write" (existing rule); queue and retry with expiry.
- Gate interaction: a new notification type or hint for background decisions must be classified in `typeClass` (exhaustiveness test) or hidden-session decisions are silently dropped.
- Background feed volume: 11 sessions now, review sessions churn; retention and reaping (`session_retention_sweeper`, `superseded_session_sweeper`) delete the sessions the feed refers to ("departed" handling exists client-side only, per tab).
- Flag semantics: explicit-false persisted flags survive default flips (documented in gate ADR-004); new flags need the same status_detail treatment and live-settable controller (pattern `replyFlagController`).
- jscpd threshold 0.14%: mock-heavy new inline-action tests may trip it.

## 6. Unstated user needs (INFERRED from the code and memory notes)
- Triage queue ordering: one unified "needs me" inbox sorted by blocking cost (a blocked agent vs an informational item), with count in the badge that matches what is actionable.
- Keyboard-first batch handling (leader-key command bar exists, #1012) and mobile one-tap with large targets.
- Answer provenance: show who/what answered and when to the agent (so it can trust it) and in the audit log (`AuditSink` exists for reply).
- Undo/"send back": after approving or answering wrongly, a way to correct (steer) without opening the session.
- "Snooze / remind me" on a human-needed row (time and event based) — symmetric to agent wake triggers and probably the same substrate.
- Notification dedup for the same blocked question appearing as toast + tray + Slack + push; answering in one channel must retire the others (resolved_item_reaper exists: `server/notifications/resolved_item_reaper.go`).
- Visibility for the operator of what agents are waiting for (a "scheduled wakes / waiting on" view), and a kill switch (global halt exists for triage per ADR-003).
- Cost awareness: waking a session is not free; show estimated context cost or default to steer-with-payload rather than "go re-check".
