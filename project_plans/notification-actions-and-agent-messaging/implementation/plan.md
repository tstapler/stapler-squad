# Implementation Plan: notification-actions-and-agent-messaging

**Feature**: Fix the empty Background tab, make notifications actionable in place, turn `AskUserQuestion` into a structured durable answerable object, and give agents a durable messaging and wake-trigger substrate.
**Date**: 2026-10-17
**Status**: Planning complete. Phase 5 implementation may start; **nine owner items proceed on their documented defaults**. The Phase 5 start gate is the first step of Phase 5 and happens **before wave W0 begins** (see "Unresolved Questions"): the owner accepts ADR-001 to ADR-008 in one step (Proposed to Accepted) and confirms or amends the defaults. Repairs applied: the 2026-10-17 review repair, the Phase 4 repair, triad rounds 1, 2 and 3 (2026-10-18; see the Review repair logs at the end). Spike-gated stories are marked, each spike has a pass criterion and a fallback
**ADRs**: [ADR-001](../decisions/ADR-001-background-feed-from-hidden-session-list.md) Background feed from hidden-session list, gate unchanged; [ADR-002](../decisions/ADR-002-inline-action-descriptors-and-single-dispatcher.md) descriptors + one dispatcher; [ADR-003](../decisions/ADR-003-extend-guidance-request-as-question-and-help-store.md) extend `GuidanceRequest`; [ADR-004](../decisions/ADR-004-inline-action-transport-risk-tiers-and-remote-access.md) risk tiers and remote access; [ADR-005](../decisions/ADR-005-askuserquestion-answer-routing-ladder.md) answer routing ladder; [ADR-006](../decisions/ADR-006-durable-wake-trigger-table-and-single-dispatcher.md) `WakeTrigger` table + dispatcher; [ADR-007](../decisions/ADR-007-agent-messaging-security-and-revival-policy.md) messaging security and revival policy; [ADR-008](../decisions/ADR-008-rollout-flag-semantics.md) flag semantics

Inputs: `requirements.md`, `research/{stack,features,architecture,pitfalls,ux,build-vs-buy}.md` (all read in full). Predecessors consulted for conventions: `project_plans/durable-guidance-request/implementation/plan.md`, `project_plans/notification-revamp`, ADR-010 of `notification-tray-and-hidden-session-gate`.

Labels used below: VERIFIED = a research file cites an opened source or command output; INFERRED = reasoned, not run; SPIKE = a task in Epic 0 closes the unknown.

---

## Vision

### Current state vs target

| Area | Today (VERIFIED in `research/features.md`, `architecture.md`) | Target |
|---|---|---|
| Background tab | Empty. `selectBackgroundRows` reads only history; the delivery gate drops routine events for hidden sessions before the bus (`policy.go`, `gate.go:201`). Live: 11 hidden sessions, 0 history rows, 0 rows rendered | One row per hidden session from the list the tab already polls; failure/needs-human pinned first; never-blank roll-up. Gate untouched |
| Notification actions | Approve/Deny/Approve-anyway, mark read, open. Backlog rows link out. `ReplyCard` only in session detail; `GuidanceRequestPanel` only in 3 detail views | Approve, Deny, answer a question (options, multi-select), decide a backlog item (verdict / status / rework / merge), answer guidance - all from the pane, one interaction, desktop and phone, through one RPC and one UI row component |
| AskUserQuestion | Hook defers to the native dialog; in-memory `PendingQuestionStore` (256 cap, 30 min TTL, lost on restart); Reply single-select digit, hidden sessions only | Durable `GuidanceRequest(source=ask_user_question_hook)` with full 1-4 question payload, multi-select, delivery state; answer via keystroke (visible and hidden) or hook reply if spike passes; dead sessions keep a durable answer |
| Agent messaging | `steer_session`/`write_to_session` unattributed raw injection; `GuidanceRequest` human-answerer only; no wake substrate; sessions poll with `ScheduleWakeup` (ssq#428) | Attributed, gated peer help on the same entity; durable `WakeTrigger` table with one dispatcher (bus + its own clock-injected timer); wakes survive restart; zero new poll loops |
| Observability | Delivery-gate counters only | Per-action, per-delivery, per-wake counters and latency, plus the alert on missed wakes after restart |

### Harness capability matrix (summary of `research/stack.md` section 2)

| Harness | Structured question + external answer | Resume/schedule | Hooks | Plan stance |
|---|---|---|---|---|
| Claude Code | `AskUserQuestion` `questions[]`; `PreToolUse` allow + `updatedInput.answers` is documented for non-interactive use; interactive behavior UNVERIFIED (SPIKE S1); ADR-010 keystroke is VERIFIED | `ScheduleWakeup`/`CronCreate` session-bound, 7-day expiry (VERIFIED docs); Channels push UNVERIFIED (SPIKE S3) | `Notification`, `Stop`, `PreToolUse`, HTTP hooks (VERIFIED docs) | Keystroke default, hook reply conditional, own durable wake layer |
| Gemini CLI / agy | No documented question API; agy never consults hooks for confirmation (INFERRED from memory) | None documented | `BeforeTool`, observe-only `Notification` | Terminal detection + keystroke only if fixture measured; else read-only (SPIKE S5) |
| OpenCode | Plugin events `permission.asked`; no first-class question event documented | Plugin `client` may send prompts (INFERRED) | Plugin event handler | SPIKE S5; keystroke fallback |
| pi | Extension intercepts tool calls; question channel UNVERIFIED | Resume across restart via adapter | Extension events + JSONL tail | SPIKE S5 |
| Aider | Text prompts only | None | None | Detection + keystroke if fixture measured; else read-only |
| Codex | Not a supported harness (survey only) | n/a | n/a | Out of scope |

Degrade ladder is ADR-005: hook reply (conditional) -> keystroke -> durable record + explicit resume -> read-only.

### Competitor gap summary (`research/ux.md` s1, `build-vs-buy.md` s1-4; absence of evidence is low confidence)

- Verified: Warp's mailbox navigates but does not answer in place; Claude Code Remote Control keeps `AskUserQuestion` open indefinitely and answers inside the session view; Devin exposes `waiting_for_user`/`waiting_for_approval` status; Copilot mission control exposes "awaiting feedback".
- Not found in any source: structured option answers from a notification surface, attributed peer-to-peer agent help, self-scheduled event/state/human-answer triggers that survive restart. These are the differentiators; borrowed vocabulary only (A2A states, Inngest event-match + timeout, LangGraph response schema, MCP elicitation flat form schema). No framework or SaaS adopted (`build-vs-buy.md` section 5).

---

## Step 0.5 - Creative pass (recorded)

- **(A) Four independent epics, each with its own substrate** (own question table, own scheduler, own action RPCs). Strength: epics ship and revert independently with no shared churn. Weakness: duplicates the answer/guard path, forks the question store (forbidden), and the jscpd 0.14% gate would trip on copy-pasted action UIs.
- **(B) One generic `AgentMessage` inbox entity subsuming guidance, peer help, wake and notification actions.** Strength: single conceptual model. Weakness: rewrites the shipped, reviewed `GuidanceRequest` path and ADR-010, and bundles the lowest-risk fix with the highest-risk substrate.
- **(C, chosen) Stage on shared seams: session-derived Background feed first; descriptor + single dispatcher for actions; additive extension of `GuidanceRequest`; a new `WakeTrigger` table only where no entity exists; each behind its own live flag.** Strength: reuses every guarded write path and registry convention, order matches the requirements' staged rollout, a flag off restores prior behavior. Weakness: several seams must land first (reply core, dispatcher timer/`Clock`, MCP credential, UI row component) and the epics share a few files, so waves must use separate worktrees.

Chosen: (C). The key sub-choices are ADR-001 to ADR-008.

---

## Step 1 - System type

A feature set layered onto an existing event-driven Go server plus React SPA: closed-set command dispatch (actions), an extended durable entity with a state machine (questions), and a small durable scheduler on top of an in-process bus (wakes). No green-field infrastructure; zero new Go or npm dependencies are planned (cron parsing already in robfig/cron v3 in `server/workflows/scheduler.go`).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `HiddenSessionSummary` | Existing client type for a hidden session from `ListSessions{hidden_only}` (state, `updatedAtMs`, tags) | Input to the activity selector |
| `ActivityRow` | One Background tab row for one hidden session | Fields: `state` (`ActivityState`), `role` (`SessionRole`), `outcome` (`BackgroundOutcome`), title, last activity |
| `ActivityState` | Sum type: `exception_failure`, `exception_needs_human`, `running`, `finished`, `stopped_unknown`, `departed` | Exceptions pinned first; exhaustive switch in TS. `finished` requires positive evidence: the session is stopped AND its server-computed `BackgroundOutcome` is `completed`. `exception_failure` also arises, without any history record, from outcome `failed`. A stopped session whose outcome is `unknown` is `stopped_unknown`, never labelled healthy. Notification history is never the evidence for `finished`: the delivery gate suppresses those records for hidden sessions |
| `SessionRole` | `review`, `diagnose`, `triage`, `other` (from the `backlog:*`/`review:*`/`diagnose:*` tags) | What the session is for; separate from its state. Fixtures say "10 review, 1 diagnose" in terms of this field |
| `BackgroundOutcome` | `completed`, `failed`, `unknown`: proto field `Session.background_outcome`, populated only for `ListSessions{hidden_only}` | ADR-001 decision 6. `completed` = the session's `ItemSession` records a `review_verdict` (review) or `triage_result` (triage); `failed` = status `CRASHED` or an `end_reason` on the **failure allowlist** (ADR-001 decision 6; every other value, including ones added later, is not a failure); else `unknown` (always so for diagnose) |
| `BackgroundRollup` | The never-blank summary: counts by state plus last-activity time and copy variant (`no_failures_reported`, `has_exceptions`, `none`) | Pure function of rows; copy never says "healthy" because absence of a failure record is not proof of health |
| `NotificationActionDescriptor` | Server-computed, read-time description of one action available on a notification (`kind`, label, params schema, `tier`, `available`, target key) | Never stored on history rows (ADR-002) |
| `ActionKind` | Enum: `OPEN`, `APPROVE`, `DENY`, `ANSWER_QUESTION`, `BACKLOG_DECISION`, `ANSWER_GUIDANCE`, `CANCEL_WAKE`, `CANCEL_PEER_HELP` | `*_UNSPECIFIED = 0`; unknown degrades to `OPEN`. Tier A: `OPEN`, `APPROVE`, `DENY`, `BACKLOG_DECISION`, `ANSWER_GUIDANCE`, both cancel kinds, and `ANSWER_QUESTION` on a durable (non-live) route; Tier B: `ANSWER_QUESTION` with route `live_keystroke` (ADR-004) |
| `ActionTier` | `A` (data-only) or `B` (writes bytes into a live pane) | ADR-004 |
| `TargetKey` | Entity key an action targets: `approval_id`, `question_id`, `guidance_request_id`, or `item_id` | Never the notification record ID. `question_id` is a `PendingQuestionStore` key (a live prompt with no durable row yet); `guidance_request_id` is the durable row id once the prompt is mirrored. The descriptor carries `guidance_request_id` when a mirror exists, else `question_id`; notification metadata carries both |
| `ActionOutcome` | Enum returned by `InvokeNotificationAction`: `APPLIED`, `ALREADY_RESOLVED`, `STALE`, `NOT_SENT`, `SEND_INDETERMINATE`, `RATE_LIMITED`, `FORBIDDEN`, `FLAG_OFF`, `INVALID` | Typed, not Connect errors. `STALE` carries a `reason` (`dialog_gone`, `dialog_replaced`, `session_ended`) that selects the copy: only `dialog_gone` and `session_ended` read "Already handled"; `dialog_replaced` reads "The session is now asking something else" (`design/ux.md` 3.1 rows 11 and 12) |
| `IdempotencyKey` | Client-generated key; same key + different payload hash is `INVALID` | Mirrors hidden reply `reply_id` |
| `PaneReplyCore` | Extracted claim + audit + write-lease + `DialogMatch` + one-write core of `server/services/hidden_question_reply.go` | Seam; hidden flag semantics unchanged |
| `QuestionSource` | Enum-by-convention: `agent_mcp`, `ask_user_question_hook`, `peer` | `GuidanceRequest.source`; also selects the pending-cap class and the coalescing key (ADR-003 rule 1). `ask_user_question_hook` means "mirrored from a harness prompt" (Claude hook or a terminal-detected replyable non-Claude dialog, profile in `questions_json.profile`); there is no `terminal_detect` source |
| `AnswererKind` | `human` or `session` | `GuidanceRequest.answerer_kind` |
| `QuestionPayload` | Canonical value object: 1-4 `Question{text, header, options[{label, description}], multiSelect}` | Serialized in `questions_json` |
| `QuestionID` | Newtype over the `PendingQuestionStore` key of a live prompt | Dedup and action key before a durable row exists; see `TargetKey` |
| `GuidanceRequestID` | Newtype over the `GuidanceRequest` id string | Action key once mirrored; equals no `QuestionID` (two id spaces) |
| `HookCallID` | Newtype over the Claude `tool_use_id` of the hook invocation | Correlation, optional |
| `DeliveryRoute` | `hook_reply`, `live_keystroke`, `resume`, `read_only` | ADR-005 ladder |
| `DeliveryState` | `pending`, `sent`, `acknowledged`, `delivery_failed`, `saved_for_later`, `stale` | Sum type; separate from answered; transitions via a guarded table; cross-field rules enforced by `NewGuidanceRequest` (ADR-003 rule 3b). `delivery_failed` carries a `reason` (`not_sent`: nothing written, copy row 5; `still_showing`: a digit was written but the dialog stayed, copy row 16; `design/ux.md` 3.1) |
| `QuestionDialogProfile` | Per-harness strategy: dialog matcher, option extractor, key encoder, replyable flag | Strategy (GoF); one fixture per harness |
| `AnswerRouter` | Chooses `DeliveryRoute` from session state, harness profile and flag stage | Closed switch |
| `WakeTrigger` | Durable ent row: owner session, trigger type/spec, action, status, claim/delivery split with idempotent `delivered_at` | ADR-006 |
| `WakeTriggerType` | `at`, `cron`, `backlog_event`, `pr_event`, `ci_event`, `session_state`, `guidance_answered` | `spec_json` is a sum type per type |
| `WakeStatus` | `pending`, `claimed`, `delivered`, `needs_revive`, `missed`, `cancelled`, `skipped` | Guarded transitions only, full table in ADR-006 rule 2: `pending -> claimed | missed | cancelled | skipped` (the last only for a one-shot row on its third rate-limit deferral, reason `rate_limited`); `claimed -> delivered | pending (cron re-arm only) | needs_revive | missed | skipped | cancelled`; `needs_revive -> claimed | missed | skipped | cancelled`; terminal: `delivered`, `missed`, `cancelled`, `skipped`. `claimed` is not `delivered`: only a confirmed pane write sets `delivered_at` |
| `WakeAction` | `steer_text{payload}` or `notify_operator{text}` | Operator "Remind me" uses the second. The revival guards apply to `steer_text` only (ADR-006 rule 2 decision table); `notify_operator` is emitted as a system-level notification with no session binding so the hidden-session gate does not suppress it |
| `WakeDispatcher` | The single bus consumer + scheduler registrant that fires triggers | `server/wakes` |
| `WakeDedupKey` | Non-null string unique key from owner + type + canonical spec | No nullable column in the key |
| `WakeGeneration` | Counter incremented when a woken turn schedules another wake | Bounds chains |
| `AllowRevive` | Creation-time bool permitting the normal resume path for a stopped (never archived) owner | Default false (ADR-007). With false, a stopped owner's `steer_text` wake is `skipped(owner_stopped)`; `needs_revive` exists only for true |
| `Admission` | The `server/wakes` port over `BacklogService.Admit` (`backlog_service.go:562`, WIP cap 2) | Revives of stopped owners must pass it; the rework cap is spawn-only and does not apply to a revive. There is no `SpawnGate` type |
| `CallerIdentity` | Session UUID of an MCP caller **verified** by `verifiedCallerSessionUUID(ctx)`: set only when `X-Stapler-Session-Proof` verifies for the claimed UUID | Never an argument. The unverified `callerSessionUUID(ctx)` is a client claim (`server/server.go:1112-1118`) and is not used by the new tools |
| `PeerHelpRequest` | A `GuidanceRequest` with `source=peer`, `answerer_kind=session` | No separate entity |
| `ScopedFlag` | A flag value resolved by the existing `Config.GetFeatureFlagScoped(name, scope, default)` (`config/config.go:1925`): an explicit scoped value wins in either direction (a scoped `true` under a global `false` is on; a scoped `false` under a global `true` is off), then the explicit global value, then the default | ADR-008 decision matrix; no new resolver. "Off" always means "the decision point resolves false" |
| `SteerDelivery` | Cache of `claimed` wake rows plus retry wrapper around the `Steerer` port: writes only when `IsReadyForSteer` is true, expires; the row, not the queue, is the source of truth | Unknown readiness means do not write |
| `McpSessionProof` | HMAC-SHA256 proof, domain `ssq-mcp-v1`, over the session UUID, minted at spawn with the `server/services/hook_proof.go` machinery, delivered only through 0600 per-session files (never argv, never tmux env) and sent as `X-Stapler-Session-Proof` | ADR-007 rule 0; defeats header forgery by UUID knowledge, not a same-user file reader or an agent that makes another session print the file (accepted risk) |
| `GuidanceSessionReaper` | New small component that cancels open `source=ask_user_question_hook` and `source=peer` guidance rows when their session is deleted/archived/stopped/missing (event subscription + startup pass), calling `CancelGuidanceRequest`; never cancels `agent_mcp` session-scope guidance (Story 3.4 delivery states own those) | Replaces the nonexistent "self-heal sweep"; keeps `backlog_lifecycle.go` untouched |
| `ListenerClass` | `local` or `remote`, derived from the request context and passed to the descriptor assembler | Lets the same history page differ per listener (Tier B `available=false` on `:8444`) |
| `Clock` | Port in `server/wakes` (`Now`, `NewTimer`) with real and fake implementations | The dispatcher injects it; robfig/cron has no clock seam |
| `AnswerListener` | Func type owned by `server/services` and set by wiring; called on a successful `AnswerGuidanceRequest` | Breaks the `server/wakes <-> server/services` import cycle |
| `AnswerLatency` | `answered_at - created_at` on the guidance row, logged as `answer_latency_ms` | Success metric |
| `WakeLimits` | The numeric defaults and hard ceilings table in this plan ("Wake limits", 12 rows), enforced by `schedule_wake` and the dispatcher | Single source for ADR-006 rule 5 and ADR-007 rule 6 |
| `EngagedCard` | A card held by an operator-originated interaction, defined once in the `design/ux.md` 7.1 table (operator-placed focus on an unresolved card, pointer over it, a touch in the last 5 s, an open sub-step, draft or unsent selection, a running commit delay; ends 750 ms after the last). The related **digit-armed** state (the operator opened or Tabbed to the card, plus a 750 ms grace that a digit's own focus move does not restart) gates only the digit shortcuts | While any card is engaged the Needs-you tier is frozen (UX-AC60); programmatic focus neither engages nor arms (UX-AC61); no plan text restates the definition |
| `CommitDelay` | The 3 s hold with Undo between a tap on Approve, Send back or Mark done and its dispatch | Client-side; closing the panel, a click outside the desktop popover, a route change, Back, `visibilitychange` to hidden, `pagehide` or going offline **cancels** the hold and sends nothing (the card is restored with a note and the badge still counts it); not applied to Deny, answers or Merge (`design/ux.md` 2.4) |
| `HumanActivity` | The signal behind the `human_active` guard: the latest of (a) the time of the last UI-stream pane input recorded in `paneWriter.SendInput` (`server/services/terminal_access.go:140`) for the session and (b) `#{client_activity}` of any attached tmux client whose `#{client_control_mode}` is 0 | Control-mode clients (ours) are always attached, so mere attachment is not a signal; defined in ADR-004 and Task 3.4f |
| `ReviewedMark` | A browser-local "Mark reviewed" / "Mark as handled" flag on a Background row or an Unconfirmed answer | `localStorage` behind try/catch; never a server write (`design/ux.md` 1.4) |
| `PeerHelpEscalator` | `server/wakes` component that raises one needs-human notification per open peer row after 10 minutes without an answer, or at once when the answerer is stopped/archived/deleted | Deadlines derived from durable rows and re-armed at startup; no poll loop (Story 4.11) |

Term count: 48 (glossary table rows recounted by script after the triad round 1 repair).

**The word "kind" (disambiguation).** Five unrelated things were called "kind": `ActivityState` (Background row state; formerly `ActivityKind`), `SessionRole` (review/diagnose/triage/other), `ActionKind` (enum of inline actions), the event `kind` inside a `WakeTrigger` spec (a `BacklogChangeKind` such as `verdict_recorded`), and `NotificationType`. Prose and metric labels use the specific name: `background_activity_rows{state}`, never `{kind}`. Bare "kind" survives only in the identifiers `ActionKind`, `action_kind`, a trigger spec's `kind` field, and the `kind` label of the two `notification_inline_action_*` metrics, where it always means `ActionKind`.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Background rows | Pure selector over a sum type (`ActivityState`) | type-driven-design | Extend history row shape | Gate suppresses the source data; ADR-001 |
| Inline actions | Command dispatch + closed `ActionKind` switch; Service Layer facade `InvokeNotificationAction` | PoEAA Service Layer, GoF Command | Per-type client RPC calls | Shared idempotency/audit; ADR-002 |
| Action descriptors | Read-time projection (Query Object / Assembler) | PoEAA | Stored actions | Go stale; bind to collapsed record ID |
| Reply core | Extract Function into `PaneReplyCore`; callers pass an `AnswerTarget` | Fowler refactoring | Copy path for visible sessions | Second keystroke path is top pitfall 1 |
| Answer routing | Closed `switch` over `DeliveryRoute` in `AnswerRouter` | Go house style (as durable-guidance ADR-002 plan) | Interface per route | One implementation per route; interface pollution |
| Harness dialogs | Strategy (`QuestionDialogProfile`) | GoF | `if harness ==` chains | Open set of harnesses, one fixture each |
| Question entity | Additive extension of existing aggregate | ADR-003 | New table | Forbidden parallel store |
| Question dedup | Transactional lookup-then-insert keyed on `(asker_session_uuid, hook_call_id)`, bypassing the `(scope, scope_key, question_text)` coalesce for non-`agent_mcp` sources; cap counted per source class | existing cap-check precedent | Unique index on new column; leaving the text coalesce in place | Existing coalesce merges identical-text calls and the cap of 4 starves on terminal-answered rows; ADR-003 |
| Newtypes | `QuestionID`, `HookCallID`, `WakeDedupKey`, `CallerIdentity`, `TargetKey` | Minsky, type-driven-design | raw `string` | Prevents ID mix-ups (record id vs question id) |
| State types | Sum types: `ActivityState`, `BackgroundOutcome`, `DeliveryState`, `DeliveryRoute`, `WakeStatus`, `ActionOutcome`, `WakeSpec` | type-driven-design | free strings | Exhaustive handling |
| Wake persistence | Durable row + guarded conditional UPDATE with claim/delivered split | existing `AnswerGuidanceRequest` idiom | In-memory timers; marking `fired` before delivery | Restart or crash between mark and write loses the wake; ADR-006 |
| Wake evaluation | Observer on EventBus (one consumer) + one dispatcher-owned `min(next_fire_at)` timer | GoF Observer, `StartVerdictSteering` shape | Second cron engine / row-scanning ticker | One admission path; sleeps until the next due row |
| Wake timing | Dispatcher-owned single timer on `min(next_fire_at)` with an injected `Clock`; `cron.ParseStandard` only computes next times | Ports and adapters | Extend `workflows.Scheduler` (no `ScheduleAt`, `Start` returns early without a repo, robfig/cron has no clock seam) | One admission path, testable time |
| Delivery | `SteerDelivery`: Queue with expiry + readiness check | Fowler | Immediate write | Busy-session typing collision |
| MCP identity | Per-session HMAC credential verified in `mcpWithUUID`, plus decorator `withCallerIdentityGate` around handlers | GoF Decorator; `server/services/hook_proof.go` precedent | Trusting the client-supplied header | Header is forgeable (ADR-007 rule 0); registration test verifies every tool is wrapped and a forged-header test verifies denial |
| Caps | Check-then-reserve under one short process-wide cap mutex (+ transactional COUNT where DB-backed); revives use a separate bounded revive lock (ADR-007 rule 5) | memory instinct | Per-item lock; one lock spanning a tmux and worktree spawn | Recurring race class; a spawn must not stall cap checks |
| Flags | `*FlagController` per flag registered in `server/services/feature_flag_service.go`; precedence via existing `Config.GetFeatureFlagScoped` | `replyFlagController` precedent | New `ResolveScopedFlag`; env vars | Constraint; ADR-008 |
| UI action row | One `ActionRow` component + `useInvokeNotificationAction` hook; shared `GuidanceQuestionForm` | React composition | Three forked cards | jscpd 0.14% |
| Panel decomposition | Isolate via seam (compose-only `NotificationPanel`) | Fowler | Inline more branches | 837-line hotspot |
| Metrics for client-only behavior | Existing client-event RPC (handler located in Task 0.7a; `server/features/client_event.go` is registry metadata only) | existing RPC | New endpoint | Reuse |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| `web-app/src/components/ui/NotificationPanel.tsx` (837 lines, 29 hook calls, 6 commits in 90 days) | Owns segmentation, bulk ops, background, filters | **Isolate via seam** (Stories 1.1, 2.5) | Extract `useBackgroundActivity` and `ActionRow`; the panel only composes. Refactor-first is not justified: moderate churn and natural seams exist |
| `session/ent/schema/guidance_request.go`, `ent_repository_guidance.go` (481 lines), `server/services/guidance_request_service.go` (420) | Single-question shape | **Extend as-is**, additive columns (Story 3.1) | In-flight, well commented; one design choice (dedup) recorded in ADR-003 |
| `server/services/hidden_question_reply.go` | Reply core tangled with hidden-only policy | **Isolate via seam**: extract `PaneReplyCore` (Story 2.4) | Visible-session, approval and backlog actions reuse claim/audit/lease/one write; hidden flag semantics untouched |
| `server/services/pending_question_store.go` + `ParseAskUserQuestion` | In-memory, 30 min TTL, multi/unknown shapes non-replyable | **Isolate via seam**: keep as verified-proof cache, durable row is truth (Stories 3.2, 3.3) | Do not make a keystroke-shaped store durable |
| `pkg/events/types.go` + `mapBacklogChangeKind` | Switch panics on unmapped kind, dropping events; no `GuidanceRequestEventPayload` exists (checked) | **Extend as-is** + mapping test (Story 4.4) | `guidance_answered` is delivered by a direct callback plus reconcile, so no new backlog change kind is introduced |
| `server/workflows/scheduler.go` | Only cron engine; workflow-shaped (`Start` returns early without a repo; no one-shot, callback or clock seam; robfig/cron v3.0.1 has no clock injection) | **Leave untouched** (Story 4.2) | Wakes use a dispatcher-owned timer with an injected `Clock`, so there is no coupling to the workflow scheduler and no risk to workflow behavior. This reverses the earlier "register through the one cron engine" plan (ADR-006 rule 3) |
| `session/backlog_lifecycle.go` (2441 lines, 81 commits in 90 days: highest-churn hotspot in the repo) | The earlier Task 3.3c would have extended a nonexistent guidance-cancel sweep here (`CancelGuidanceRequest` has no production caller) | **Do not extend: isolate via new component** `GuidanceSessionReaper` (Story 3.3, Task 3.3c) | Session-scope hook-question cleanup is not backlog-domain; wired at the existing session-deleted/archived event seam; the file is removed from Story 3.3's Files |
| `server/dependencies.go` (2039 lines, 64 commits/90d), `server/services/approval_handler.go` (1401, 19), `cmd/ssq-hooks/main.go` (1915, 12), `server/mcp/tools_backlog.go` (3235, 50; home of `callerSessionUUID`) | Large and high-churn; Stories 3.3a, 3.4c, 4.2, 4.5 add code near them | **Extend as-is with a one-line budget**: wire via one `notificationActionsModule` / `wakesModule` constructor (one added line per hotspot file); all new logic in new files (`guidance_request_hook_mirror.go`, `caller_identity_gate.go`, `mcp_proof.go`); the identity gate is not placed in `tools_backlog.go` | Keeps hotspot growth to wiring lines |
| `server/deliverygate` | Exhaustiveness-tested `typeClass` | **Extend as-is**; only assert mappings (Story 2.2c) | No new enum values planned |
| `steer_session` / `write_to_session` (`server/mcp/tools_terminal.go`) | Unattributed raw injection, no hidden-session refusal | **Isolate**: new attributed `SteerDelivery`, tools unchanged (ADR-007 rule 7) | Fixing them is out of scope; new code must not extend the debt |
| `NotificationService.SendNotification` (~110 lines, gocognit gate) | Already large | **Extend as-is: do not modify** | No new notification emission goes through it |
| `docs/tasks/askuserquestion-ui.md` | Stale payload `{prompt, options:[string]}` disagrees with official `questions[]` | **Refactor-first** (Task 3.2b) | Fix before the parser relies on it |
| `ScheduleWakeup` references (counts per file from `git grep -c ScheduleWakeup -- '*.go'`: `server/mcp/tools_backlog.go` 6, `tools_backlog_test.go` 2, `tools_backlog_wait_cap_test.go` 3, `server/services/backlog_service_verdict_steer.go` 2, `session/backlog_commands.go` 1, `session/backlog_context.go` 1, `session/backlog_post_review_prompt_test.go` 1, `session/pipeline_mode_seed.go` 2) | Prompts forbid it but give no replacement; `tools_backlog.go` (3235 lines) also holds the tool descriptions | **Extend as-is** + point prompts at `schedule_wake` when flag on (Story 4.7); the guard test counts per file against this explicit map | Not this project's job to remove client-side polling already forbidden |

---

## Migration Plan

- **Schema changes** (ent auto-migrate, `client.Schema.Create` guarded by `EntSchemaCreateMu`; no hand-written SQL migration in this repo):
  - `GuidanceRequest` additive columns, each with a default so legacy rows remain valid (SQLite `ADD COLUMN` needs defaults): `source='agent_mcp'`, `answerer_kind='human'`, `multi_select=false`, `delivery_attempts=0`; optional/nullable: `asker_session_uuid`, `answerer_session_uuid`, `questions_json`, `delivery_route`, `delivery_state`, `hook_call_id`. No new unique index on this table (ADR-003 rule 1: the real constraints are the existing Go-side `(scope, scope_key, question_text)` coalesce and the pending cap of 4, which are bypassed/re-keyed per source class in Go, not by a schema change).
  - New table `wake_triggers` (Story 4.1) with a unique index on non-null `dedup_key`, status values `pending|claimed|delivered|needs_revive|missed|cancelled|skipped`, columns `claimed_at`, `delivered_at`, `delivery_attempts`, `rate_limit_deferrals` (int, default 0; reset on a successful claim or a cron occurrence skip, persisted so a restart keeps it), and indexes `(status, next_fire_at)`, `(trigger_type, status)`.
- **Files**: `session/ent/schema/guidance_request.go` (edit), `session/ent/schema/wake_trigger.go` (new), `tools/lint/noarchivedrevival/analyzer.go` (one string added to `lifecycleFiles`, Task 4.3f). Generated ent and proto output is **not committed**; generate with `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` and `make proto-gen` (all Make targets already depend on `ent-gen`); fresh worktrees run `make build` first.
- **Reversibility**: additive only. Reverting the PR leaves inert columns/table; the extra columns are never read by old code. No backfill.
- **Zero-downtime strategy**: `CREATE TABLE IF NOT EXISTS` / `ADD COLUMN` on startup; no load until flags are on.
- **Rollback procedure**: flip the flag off (live); data is retained and inert; for code, revert the PR. Do not `DROP` columns.

## Observability Plan

- **Logs** (structured `slog`, one line per transition, each with `question_id`/`trigger_id`/`action_kind`): inline action invoked/outcome (`action_kind`, `tier`, `outcome`, `channel`); question created (`source`, `scope`), answered (`answer_latency_ms`, `route`), delivery state change (`from`, `to`, `route`, `attempt`); wake scheduled/claimed/delivered/needs_revive/skipped/missed (`type`, `owner`, `reason`, `lag_ms`); reconcile summary at startup (`pending`, `delivered`, `requeued_claimed`, `missed`, `reconciled_event`); cap refusals (`cap`, `current`); gate decisions stay in the gate's own rate-limited log.
- **Metrics** (names follow `notification_delivery_suppressed_total`):
  - `notification_inline_action_total{kind,tier,outcome,channel}`; `notification_inline_action_duration_seconds{kind}` (SLO: p95 < 1 s on localhost).
  - `guidance_question_answer_latency_seconds{source,route}` (created_at to answered_at); `guidance_answer_delivery_total{route,state}`.
  - `background_activity_rows{state}` via the client-event RPC (SPIKE S7 confirms the sink); `background_tab_empty_total{variant}`; `notification_pane_open_ms` (client-event histogram of open-click to first action row, SLO p95 <= 200 ms from cached state, REQ-18).
  - `wake_trigger_total{type,state}` with `state` in scheduled/claimed/delivered/needs_revive/missed/cancelled/skipped; `wake_fire_lag_seconds{type}`; `wake_reconcile_total{result}` (including `requeued_claimed`); `wake_limit_hit_total{limit,phase}` (`phase` is `create` or `fire`; one `limit` value per row of the Wake limits table); `wake_delivered_per_owner_hour` (gauge); `peer_help_total{state}` (`waiting|answered|cancelled|escalated|not_sent|rejected`); `mcp_caller_gate_denied_total{tool,role}`; `mcp_identity_unverified_total{tool}`; `guidance_mirror_cleared_total{reason}` (`answered_in_terminal|dialog_gone|dialog_replaced|session_ended`); `guidance_mirror_failed_total{reason}` (hook-path mirror error or timeout, Task 3.3i); `question_dialog_match_total{result,claude_version}` (`match|mismatch|indeterminate`; `claude_version` is `unknown` until Spike S7 item (9) finds a source; Task 3.3h); `reopen_after_action_total{kind}` and `inline_action_undo_total{kind}` (the wrong-answer proxy, Task 5.1d); `human_active_refusal_total`; `wake_reconcile_digest_total`; `wake_duplicate_delivery_total` (a `<wake id>` delivered more than once); `wake_needs_revive_age_seconds` (gauge, oldest `needs_revive` row); `background_reviewed_mark_total{storage}` (client-event; the cost of the accepted per-browser limitation, `design/ux.md` 1.4); `wake_fires_per_session` (histogram recorded when a session ends) and `agent_schedule_wakeup_calls_total{role}` (from the PostToolUse receipt's tool name; the Epic 4 runtime cost outcome, Task 5.1e).
- **Alerts** (single operator; surfaced as operator notifications and the `/actuator`-style stats, not pages):
  - `increase(wake_trigger_total{state="missed"}[reconcile]) > 0` after a restart -> operator notification of failure class (requirements: missed wakes after restart > 0). A `claimed` row older than the claim lease that reconcile could not re-queue is also counted as missed, so a lost delivery is never silent.
  - `notification_inline_action_total{outcome=~"NOT_SENT|SEND_INDETERMINATE"}` rate above 3 in 10 minutes -> operator notification "actions failing delivery to session".
  - `guidance_answer_delivery_total{state="delivery_failed"} > 0` surfaces on the card itself (no silent failure).
  - `background_activity_rows{state="finished"} == 0` while `{state="stopped_unknown"} > 0` for 24 h -> operator notification "Background outcome evidence is empty" (Story 1.5). It keeps the "finished is unsatisfiable" failure from recurring silently.
  - Any `wake_limit_hit_total` increment -> one failure-class digest notification per owner per hour, never one per hit (Story 4.3, Task 4.3h).
  - `question_dialog_match_total{result="mismatch"}` share above 50% over at least 5 attempts in 1 h across sessions -> operator notification "Claude Code's question layout may have changed; inline answers are paused" and Tier B `ANSWER_QUESTION` descriptors for hook-mirrored rows degrade to `read_only` (question, options and Open session, no answer control; `hook_reply` still answers if enabled) until the share recovers; agent-created rows use a durable route and are unaffected (layout-drift canary, Task 3.3h; pre-mortem #5).
  - `wake_needs_revive_age_seconds` above 3,600 -> failure-class notification "A scheduled prompt has waited an hour for a free work slot" (Task 4.3k; pre-mortem #14).

## Risk Control

| Flag (live-settable via the feature-flag RPC/panel, ADR-008) | Default | Rollback | Staged rollout |
|---|---|---|---|
| `background_activity_feed` | ON | Flip OFF: legacy history-only selector, no data impact | Ships first (Epic 1); no soak beyond e2e |
| `notification_inline_actions` | OFF | Flip OFF: `FLAG_OFF`, descriptors reduce to `OPEN`, legacy approve/deny path remains | 1) approvals + backlog decisions on desktop, 2) phone Tier A, 3) Tier B remote only after S6 via scope `channel:remote:tierB` |
| `askuserquestion_structured` | OFF | Flip OFF: no new writes; durable rows remain visible | `stage:render` -> `stage:answer` -> `stage:hook_reply` (the last only if S1/S2 pass) |
| `agent_wake_triggers` | OFF | Flip OFF: dispatcher stops, tools return `flag_off`, rows retained; re-enable handled by reconcile policy | `trigger:at`/`cron` first, then `guidance_answered`, then event/state types, then `tool:request_help` |

Generic rollback for any story: standard revert via PR close + revert commit; all data additions are additive. Manual verification uses only the isolated instance (`PORT=62871`, `--remote-port 62872`, `STAPLER_SQUAD_INSTANCE`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`, binary at `~/.stapler-squad/manual-builds/manual-1/stapler-squad`); never `make install-service`.

### Flag precedence and dependency matrix (pre-mortem #10; ADR-008 is the source, this is its test table)

The resolver is unchanged: `Config.GetFeatureFlagScoped(name, scope, default)` = scoped explicit value, else global explicit value, else default. "Off" in any flag description means *the decision point resolves false*, not "global false". Dependencies are applied by the caller, in this order, so every cell below has one answer.

| Flag | Decision point (scope) | Effective value | Notes |
|---|---|---|---|
| `askuserquestion_structured` | render (`stage:render`) | `Scoped(stage:render)` | A scoped `true` under global `false` renders only; this is how stage 1 ships |
| | answer (`stage:answer`) | `Scoped(stage:answer) AND effective(render)` | An answer enabled while render is off is *ignored* (no invisible cards with live buttons) |
| | hook reply (`stage:hook_reply`) | `Scoped(stage:hook_reply) AND effective(answer)` | Also requires S1 PASS (a compiled-in constant until S1 is recorded) |
| `notification_inline_actions` | an action (`action:<ActionKind>`) | `Scoped(action:<kind>)` | `OPEN` is always on; `ANSWER_QUESTION` also needs its route gate, next row |
| | `ANSWER_QUESTION` (`action:ANSWER_QUESTION` AND the route gate) | `Scoped(action:ANSWER_QUESTION) AND` one of: hidden-session `live_keystroke` -> `hidden_session_reply`; visible-session `live_keystroke` -> `effective(stage:answer)` (itself `Scoped(stage:answer) AND effective(stage:render)`); durable record (`resume`, stored answer for a stopped session or an `agent_mcp` row, Tier A) -> `effective(stage:answer)`; `hook_reply` -> `effective(stage:hook_reply)` | The only cell that composes three flags. Task 0.8d enumerates action x route class x hidden x render x answer permutations; an answer with render off is `FLAG_OFF` |
| | remote (`channel:remote`) | `Scoped(channel:remote)` | `false` blocks every remote action |
| | remote Tier B (`channel:remote:tierB`) | `Scoped(channel:remote:tierB) AND effective(channel:remote) AND S6 PASS` | Default false |
| `agent_wake_triggers` | dispatcher running | `Global OR any explicit trigger:<type> == true` | A scoped `true` under global `false` runs the dispatcher for that type only |
| | create a trigger of a type (`trigger:<type>`) | `Scoped(trigger:<type>) AND dispatcher running` | Existing rows of a type whose scope turns false stay `pending` and are neither fired nor marked `missed` while the scope is false, with one carve-out: `expires_at` always wins, so a row past its `expires_at` becomes `missed` at the next reconcile even while its scope is false |
| | an MCP tool (`tool:<name>`) | `Scoped(tool:<name>) AND dispatcher running` | `schedule_wake` also needs the type's `trigger:<type>`; `request_help` needs `tool:request_help` |
| `background_activity_feed` | (none) | `Global` | Default ON; explicit OFF is a kill switch |

Rules that close the contradictions: (1) the server evaluates the flag on every `InvokeNotificationAction`, `schedule_wake` and write, so a stale client cache can only mis-draw a button, never allow a write; the client reads flags only to choose the legacy or new row, and the outcome `FLAG_OFF` maps to "This action is turned off. Open session." with an Open control (UX-AC55, UX-AC57). (2) A row's availability comes from the server descriptor, never the client flag cache. (3) A flag flip is audited by the existing flag path; loosening flips obey the audit-sink rule of Story 0.8. (4) Table test `TestFlagMatrix_ShouldResolveEveryCell_WhenGlobalAndScopesPermuted` enumerates global x scope x dependency permutations for every row (Task 0.8d).

## Wake limits

Single source of truth for every wake and help cap (ADR-006 rule 5, ADR-007 rule 6 point here). Values are proposed defaults (INFERRED from the $106.69 idle-wait incident in `research/pitfalls.md` section 4 and Claude Code's own 7-day recurring-task expiry; the owner confirms the numbers at the Phase 5 start gate with the other defaults). Defaults are config values; ceilings are compiled in and cannot be raised by an agent or by config. Every limit is injectable in tests.

| Limit | Default | Hard ceiling | Enforced at | On hit | `limit` label |
|---|---|---|---|---|---|
| Minimum `cron` interval between fires | 10 min (= 60 / the per-owner default of 6) | effective floor = `max(5, ceil(60 / effective per-owner limit))` min, derived in code; config may ask for less but is clamped up to it (so a 5 min floor needs the per-owner limit at 12) | `schedule_wake` (create) | `cap_exceeded` naming the limit; no row | `min_interval` |
| Pending triggers per owner session | 5 | 10 | create, under the short cap lock (`capMu`) | `cap_exceeded`; no row | `pending_per_owner` |
| `max_fires` per trigger | 1 for `at` and event types; 12 for `cron` | 48 | create | `cap_exceeded` | `max_fires` |
| `expires_at` horizon from creation | 24 h | 7 days | create | `cap_exceeded` | `expires_horizon` |
| Chain `generation` (a woken turn scheduling another wake) | 3 | 5 | create (child `generation` = parent + 1) | `cap_exceeded` | `generation` |
| Deliveries per owner per rolling hour | 6 | 12 | claim time, dispatcher | row stays `pending` with `next_fire_at` moved to when the window frees a slot and the row's persisted `rate_limit_deferrals` is incremented (reset on a successful claim or a cron occurrence skip; survives a restart); after 3 consecutive deferrals a one-shot row becomes `skipped(reason=rate_limited)`, while a `cron` row skips only that occurrence (stays `pending`, re-armed, ADR-006 rule 2); one digest notification per owner per hour | `fires_per_owner_hour` |
| Deliveries per rolling hour, all owners | 60 | 120 | claim time, dispatcher | same deferral rule | `fires_global_hour` |
| Open peer requests per asker | 3 | 5 | `request_help` (create) | typed `cap_exceeded` result, visible to the asker | `peer_open_per_asker` |
| Wake or help payload size | 2,000 chars | 4,000 | create | `invalid` | `payload_size` |
| Debounce window per trigger key | 5 s | 60 s | fire | coalesced into one delivery (counted, not an error) | `debounce` |
| Busy-owner hold: how long a `claimed` `steer_text` row waits in `SteerDelivery` for an owner that never becomes ready | 30 min, never past `expires_at` | 2 h | claim time, dispatcher | `missed(reason=owner_busy_timeout)` with a failure-class digest entry; never silently dropped (pre-mortem #9) | `busy_hold` |
| Revive lock hold: how long the revive path may hold `reviveMu` from before `Admit` until the revived session counts as live | 30 s | 60 s | revive, dispatcher | lock released, row stays `needs_revive`, the next cycle re-reads the owner (a session that did come up is treated as live) | `revive_lock` |

One derivation, one number, enforced in code: the cron floor is computed from the effective per-owner hourly limit (`max(5, ceil(60 / limit))` minutes; default 10 min = 6 per hour; hard floor 5 min = 12 per hour = the per-owner ceiling), and a configured floor below that is clamped up. The two config values are independent inputs, so they could disagree; the clamp is what keeps a cron row at or above the floor from deferring on its own limit (Story 4.3 AC). Two locks: `capMu` guards the create-time and claim-time checks of the pending, `max_fires` and hourly rows and is held only for the in-memory check plus one transactional COUNT or insert (never across tmux or revive I/O; Task 4.3h's race test asserts a 50 ms budget); `reviveMu` is taken only by the revive path (ADR-007 rule 5) and is bounded by the `revive_lock` row, so a slow tmux and worktree spawn cannot stall cap checks.

The two hourly delivery limits count `steer_text` deliveries only (they exist to bound model cost); `notify_operator` wakes write nothing into a session and are bounded by the pending cap alone. A refusal is always a visible typed result to the agent and a count in `wake_limit_hit_total{limit,phase}`; none is a silent drop. Acceptance criteria and tests: Story 4.3 (Task 4.3h) and Story 4.8 (Task 4.8d).

## Unresolved Questions

Nine owner decisions have documented defaults and **block no wave**: implementation proceeds on each default. The **Phase 5 start gate** is the first step of Phase 5 and happens **before wave W0 begins**: in one review Tyler accepts ADR-001 to ADR-008 (Proposed to Accepted, all eight at once) and confirms or amends items 1 to 7. Items 8 and 9 are reviews of spike results (S1 and S3; S5) that cannot exist before W0, so they are confirmed when the spike records its result, not at the gate. An amendment after the gate follows ADR-007 rule 2's amendment process.

| # | Item (owner: Tyler) | Default that proceeds | Affects |
|---|---|---|---|
| 1 | Wake limits table: defaults and ceilings, including the cron floor (10 min default, 5 min hard floor) | as written in "Wake limits" | Task 4.3h constants only (config-backed) |
| 2 | ADR-004: Tier B (live keystroke) stays desktop-only on `:8444` in v1 | desktop-only | Task 2.8b, phone copy in 3.7 |
| 3 | ADR-007 rule 0: the per-session MCP credential (Story 4.9) and its accepted same-user risk (a same-user process, or an agent using `run_command` / `steer_session` + `read_session_output`, can obtain another session's proof; redaction is best-effort) | the credential as written; the fallback that deletes `FORBIDDEN` claims is recorded in ADR-007 | Story 4.9 |
| 4 | Hook-mediated answer: is a bounded-blocking hook (`hook_block_ms > 0`) acceptable, given it removes the terminal as an answer path while waiting | `hook_block_ms = 0` (non-blocking) | Task 3.4c, after S1 and S2 |
| 5 | Role allowlist table for the new MCP tools (ADR-007 rule 2) | work sessions all five tools; hidden roles `request_help` and `list_wakes` | Task 4.5a merge |
| 6 | Default `allow_revive` for `schedule_wake` is false; confirm no work-session flow needs true | false | Tasks 4.3d and 4.3l tests |
| 7 | Adoption targets (provisional) and the flag-default rule in `requirements.md` | provisional numbers and the 14-day rule as written | Story 5.3 flips |
| 8 | Review of the Spike S1 (hook answer) and S3 (Channels) results | decision reviewed when each spike records its result | Tasks 3.4c, 4.3c |
| 9 | Which non-Claude harnesses may carry the per-session credential (Spike S5, Task 0.5e) | none until S5 passes per harness (fail closed) | Task 4.9g |

Decided and closed (not on the list):
- [x] Remind me and Resume scheduled prompts are **in scope** (triad round 1). Default presets "In 1 hour", "Tomorrow 9:00", "Pick time..."; changing preset values is a config edit that blocks nothing. Scheduled in the last wave (W6), optional-to-order. UX research recommends no snooze on blocking questions; Remind me stays on informational rows only. This closes the contradiction between `requirements.md` scope item 6 and `design/ux.md` 8.3.
- [x] Resume scheduled prompts (Task 4.10c) flips `agent_wake_triggers` from the pane behind an inline confirm that states the scope ("for all sessions"), local listener only, audit-logged by the existing flag path. Tyler may veto at review; the default proceeds.
- Spike outcomes S1 to S7 are results, not owner decisions; each unblocks the stories named in its definition.

## Dependency Visualization

```
Epic 0  Spikes + flags (parallel)
 S1 AUQ hook answer  S2 hook timeout  S3 Channels  S4 PR/CI events  S5 other harnesses  S6 remote chain  S7 code state
 0.8 flag controllers (existing GetFeatureFlagScoped) ───┐
        │                                                │
        ▼                                                ▼
Epic 1 Background (flag background_activity_feed)   Epic 2 Inline actions (flag notification_inline_actions)
 1.1 seam ─► 1.2 selector ─► 1.3 UI ─► 1.4 flag/e2e      2.1 proto ─► 2.2 descriptors ─► 2.3 dispatcher ─► 2.9 e2e
 1.7 outcome field ─┘ └─► 1.5 metrics   1.6 errors+offline banner (feeds 2.6b, 4.10)                            2.4 PaneReplyCore ──────────────────┘   ▲
                                                          2.5 ActionRow UI ─► 2.6 a11y/mobile ─► 2.7 sync ─► 2.8 remote tiers
        (1.1 and 2.5 both touch NotificationPanel: serialize 1.1 before 2.5)
                                                │
                                                ▼
Epic 3 AskUserQuestion (flag askuserquestion_structured)   needs S1,S2,S5 for 3.4c,3.5,3.6
 3.1 entity+proto+constructor ─► 3.2 parser/fixtures ─► 3.3 hook mirror + GuidanceSessionReaper ─► 3.4 AnswerRouter ─► 3.5 multi-step ─► 3.7 UI ─► (e2e)
        │                    3.8 MCP schema            └────► 3.6 harness profiles (S5)
        └──────────────────────────────┐
                                       ▼
Epic 4 Messaging + wakes (flag agent_wake_triggers)        needs S4 for 4.4a (S3 only informs 4.3c)
 4.1 schema/repo ─► 4.2 timer+Clock ─► [checkpoint] ─► 4.3 dispatcher ─► 4.4 sources ─► 4.8 integration/race/restart-gap
 4.9 MCP credential ─► 4.5 MCP tools (needs 3.1 for peer help) ─► 4.6 operator surface ─► 4.11 peer-help rows + escalator ─► 4.10 error states + exits ─► 4.7 prompts
                                       ▼
Epic 5 Observability, docs, rollout readiness (Tasks 5.1a and 5.1d start after 2.3 and land in W4 so adoption is measurable in shipping order; the rest of 5.1 in W7; 5.3 last)
```

Parallel-safe waves (each worker in its own worktree, **at most 3 concurrent workers**, so a wave with more stories runs in batches; see Effort section): W0 spikes + 0.8; W1 1.1, 2.1, 2.4, 3.1, 4.1, 4.2, 4.9 with 1.7 starting when 2.1's proto PR merges (both edit `proto/session/v1/types.proto`; Task 2.1a adds the `Session.background_outcome` field in that one commit); W2 1.2, 2.2, 3.2, 4.3 core; then fan-in.

---

# Epic 0: Spikes and flag foundations

**Goal**: Close every UNVERIFIED research item with a pass/fail criterion and a fallback, and land flag plumbing every later story consumes. Each spike writes its result into `project_plans/notification-actions-and-agent-messaging/research/spike-<id>.md` (evidence: commands run and captured output) and updates the dependent ADR status line.

**Conditional-story rule.** A story or task gated on a spike result or an owner decision (3.4c on S1, 3.5 on S1c, 3.6 on S5, Channels on S3, per-harness proof 4.9g on S5) keeps its tests written and named, as `describe.skip` / `t.Skip("blocked on S1: <link>")` with the spike or decision named in the skip message; they are never deleted, so a PASS enables them without redesign. Each such story carries an AC that says so.

### Story 0.1: Spike S1 - capture a real AskUserQuestion payload and test the hook answer in the interactive TUI
**As a** planner, **I want** measured payloads and measured hook behavior in a real interactive Claude Code session, **so that** the parser and the `hook_reply` rung are built on facts.
**Acceptance Criteria**:
- A captured `tool_input` for single-select, multi-select and multi-question calls is checked in as a fixture and matches (or documents divergences from) the official `questions[]` schema.
  - *Given* a manual instance session running Claude Code in a private tmux socket (`ssq-manual-1`), *When* the agent is prompted to call `AskUserQuestion` with 2 questions (one `multiSelect: true`, options labelled "A", "B", "C"), *Then* `ssq-hooks check` logs the raw JSON and the fixture file `server/services/testdata/askuserquestion/multi_two_questions.json` contains `questions[0].options[0].label == "A"` and `questions[1].multiSelect == true`.
- **PASS criterion**: a `PreToolUse` hook returning `permissionDecision:"allow"` with `updatedInput{questions, answers}` makes the interactive dialog **not render** and the agent continues with the supplied answer. **FAIL**: dialog still renders, hook output ignored, or Claude errors. **Fallback on FAIL**: ADR-005 rung 1 is deleted; Task 3.4c closes "not applicable"; keystroke is the only live route (no other plan change).
  - *Given* the same session with a hook returning `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"questions":[...],"answers":{"Which cache?":"Redis"}}}}`, *When* the agent calls `AskUserQuestion` "Which cache?", *Then* `tmux capture-pane` shows no numbered selector and the next agent message contains "Redis".
- **S1c** (the third PASS bullet of this story, Task 0.1c): multi-select and multi-question dialog navigation (keys that toggle and submit) are recorded as capture-pane fixtures; **PASS**: a deterministic key sequence answers a 2-question dialog; **FAIL fallback**: Task 3.5 closes, multi shapes stay read-only with Open session.
  - *Given* a 2-question dialog with question 2 `multiSelect`, *When* the recorded sequence is replayed in a fresh session, *Then* the tool result contains answers for both questions.
**Files**: `project_plans/notification-actions-and-agent-messaging/research/spike-S1.md`, `server/services/testdata/askuserquestion/*.json`, `session/testdata/reply_dialogs/*.txt` (the existing home of the Claude `AskUserQuestion` captures `question_single_select_3opts.txt` and `question_wrapped_2opts.txt`; new Claude captures go beside them)

##### Task 0.1a: Capture real payloads (S)
- Run the manual instance per CLAUDE.md; add temporary stdout logging only inside a local test hook; save sanitized payloads as fixtures; diff against `docs/tasks/askuserquestion-ui.md:13-22`.
- Files: `server/services/testdata/askuserquestion/`
##### Task 0.1b: Hook-answer experiment (M)
- Write a throwaway hook script (scratchpad, not committed) returning allow + `updatedInput.answers`; test single-select, then multi-select; record result with capture-pane output.
- Files: `project_plans/.../research/spike-S1.md`
##### Task 0.1c: Multi-select / multi-question navigation capture (S)
- Record capture-pane frames and the key sequences that answer them; check against the ADR-010 `DialogMatch` bottom-anchor assumptions (Claude Code v2.1.296 layout).
- Files: `session/testdata/reply_dialogs/`
##### Task 0.1d: Record result and update ADR-005 status (XS)
- Files: `spike-S1.md`, `decisions/ADR-005-askuserquestion-answer-routing-ladder.md`

### Story 0.2: Spike S2 - hook maximum blocking duration and failure modes
**As a** planner, **I want** the real timeout ceiling and failure behavior of command and HTTP hooks, **so that** `hook_block_ms` has a defensible bound.
**Acceptance Criteria**:
- **PASS**: a hook (command or HTTP) can block >= the configured value (test with 300 s) in an interactive session without Claude killing it, and a hook timeout/crash results in the native dialog appearing (fail-open to dialog). **FAIL fallback**: `hook_block_ms` is capped at the measured ceiling; if timeout kills the tool call, blocking mode is dropped and only pre-answered (non-blocking) `hook_reply` ships.
  - *Given* a `PreToolUse` HTTP hook that sleeps 300 s before responding, *When* the agent calls `AskUserQuestion`, *Then* the session shows the hook running (not a dialog) for 300 s, then proceeds, or the spike records the exact timeout and resulting behavior.
- Behavior when the hook returns non-2xx or empty is recorded.
  - *Given* the hook returns HTTP 500, *When* the agent calls `AskUserQuestion`, *Then* the spike notes whether the native dialog appears.
**Files**: `project_plans/.../research/spike-S2.md`

##### Task 0.2a: Timeout and error-mode experiments (S)
##### Task 0.2b: Record result; set `hook_block_ms` ceiling constant in ADR-005 text (XS)

### Story 0.3: Spike S3 - Claude Channels as a push path into a live session
**As a** planner, **I want** to know if ssq can push a re-prompt into a live interactive session without keystrokes, **so that** `SteerDelivery` has a better transport option.
**Acceptance Criteria**:
- **PASS**: a minimal channel MCP server pushes a message that appears as a user turn in an interactive tmux session without a manual approval step beyond a one-time launch setting that ssq can set when it spawns the session. **FAIL fallback** (research-preview, enterprise gating, or needs interactive opt-in each run): `SubmitContentWithEnter` via `SteerDelivery` remains the only transport; Channels recorded as a future optimization.
  - *Given* a session started with the channel enabled on the manual instance, *When* the prototype pushes "wake 1234: PR #901 checks passed", *Then* the agent responds to that text within its next idle turn and no keystrokes were sent (verified by absence of `send-keys` in tmux logging).
- Conditional-story rule: the Channels transport test (`Delivery_should_useChannelWhenSpikePassed`) is written and skipped with `t.Skip("blocked on S3: <link>")` until PASS.
**Files**: `project_plans/.../research/spike-S3.md`

##### Task 0.3a: Read channels reference, build a scratchpad prototype, test (M)
- Files: `project_plans/notification-actions-and-agent-messaging/research/spike-S3.md` (the only committed file; the prototype stays in the scratchpad)
##### Task 0.3b: Record result; add transport note to ADR-006 (XS)

### Story 0.4: Spike S4 - which PR/CI/webhook events exist and can wake a session
**As a** planner, **I want** an inventory of event sources, **so that** Task 4.4a picks real sources.
**Acceptance Criteria**:
- Produces a table: event source -> reaches `EventBus`? (yes/no) -> payload fields. **PASS**: at least PR-merged and check-run-completed reach the bus as typed events. **FAIL fallback**: the dispatcher derives `pr_event`/`ci_event` from the server-side PR poller already used by the "Up Next" PR tab (state-reconcile style), not from session polling; if neither exists the two trigger types ship as `unavailable` (typed error from `schedule_wake`) and everything else proceeds.
  - *Given* a PR with a check run finishing on a test repo against the manual instance, *When* the webhook/poller fires, *Then* the spike records the exact bus event type and fields, or "none" with the poller entry point.
**Files**: `project_plans/.../research/spike-S4.md`

##### Task 0.4a: Inventory `webhook_*` ent schemas, `TriggerFireEvent`, PR poller, `pkg/events/types.go`; exercise one event (S)
##### Task 0.4b: Record result (XS)

### Story 0.5: Spike S5 - answer channels and measured fixtures for the other harnesses
**As a** planner, **I want** a per-harness verdict, **so that** only measured harnesses become replyable.
**Acceptance Criteria**:
- For each of OpenCode, pi, Gemini CLI, agy, Aider: a row "native question API? / external answer channel? / measured capture-pane fixture checked in? / verdict (replyable | read_only)". **PASS per harness**: a bottom-anchored matcher can distinguish the dialog from forged text in a fixture. **FAIL fallback per harness**: `read_only`.
  - *Given* a captured OpenCode permission screen (a `△ Permission required` header and an `Allow once  Allow always  Reject` button row; OpenCode 1.18.30 shows no numbered rows, so the earlier `┃ 1. Allow once` shape was wrong, `research/spike-S5.md`), *When* the candidate matcher runs on it and on a forged variant with a numbered row after the footer, *Then* the first matches and the second returns no-match.
- **MCP identity channel per harness (B2).** For each of OpenCode, pi, Gemini CLI, agy, Aider the row also records: does the harness load an MCP config that ssq can write **per session** (a flag, a project-level file in the session directory, or an env-referenced path)? can that config carry an `X-Stapler-Session-Proof` header whose value is read from a 0600 file, without argv or tmux env? **PASS per harness**: yes to both, demonstrated with a sentinel header seen by a local test endpoint. **FAIL fallback per harness**: no proof is minted and the five tools return `identity_unverified` (fail closed). Global `llm-sync` MCP entries cannot carry a per-session value and do not count.
  - *Given* an OpenCode session launched with a per-session config pointing at a local echo endpoint, *When* it makes an MCP request, *Then* the echo endpoint received the sentinel header read from the 0600 file and `ps` and `tmux show-environment` for the session do not contain it; or the spike records the harness as FAIL.
**Files**: `project_plans/.../research/spike-S5.md`, `session/testdata/panes/<harness>/*.txt`

##### Task 0.5a: OpenCode plugin client / server API and fixtures (S)
##### Task 0.5b: pi extension API and Gemini/agy hooks behavior (S)
##### Task 0.5c: Aider prompt fixtures (S)
##### Task 0.5d: Matrix result (XS)
##### Task 0.5e: MCP identity channel per harness: per-session config path, header from a 0600 file, sentinel test; result feeds Story 4.9's matrix and Task 4.9g (S)

### Story 0.6: Spike S6 - remote-listener write-path review for Tier B actions
**As a** security reviewer, **I want** the `:8444` chain reviewed on the exact handlers inline actions use, **so that** ADR-004's promotion decision is evidence-based.
**Acceptance Criteria**:
- **PASS**: `InvokeNotificationAction` and the delegate handlers are reachable on `:8444` only through the authenticated chain with the Host allowlist and no state-changing GET; **FAIL fallback**: Tier B stays localhost-only (ADR-004 default, nothing to build).
  - *Given* the manual instance with `--remote-port 62872` and a request lacking credentials, *When* it posts `InvokeNotificationAction`, *Then* the response is 401/403 and no handler runs.
  - The e2e server has no `--remote-port` (`tests/e2e/helpers/test-server.ts`, `global-setup.ts`), so the durable proof of the remote chain is a Go integration test through the real remote handler chain (`server/remote_listener_actions_integration_test.go`); Playwright only checks descriptor rendering with fulfilled `available=false` descriptors.
**Files**: `project_plans/.../research/spike-S6.md`, `server/` remote chain files located during the task

##### Task 0.6a: Walk the remote middleware chain and attempt unauthenticated/foreign-Host calls (S)
##### Task 0.6b: Record result; update ADR-004 (XS)

### Story 0.7: Spike S7 - verify code state the plan assumes
**As a** planner, **I want** the INFERRED code-state assumptions checked, **so that** later tasks do not build on stale plan text.
**Acceptance Criteria**: a checklist with command output for each remaining item: (1) ADR-010 Reply is implemented (header says "implemented 2026-10-10"); (2) the real client-event RPC handler is located (it is not `server/features/client_event.go`, which is `featureregistry.Register` metadata only) and accepts arbitrary named events, or the sink for Story 1.5 is chosen; (3) `EventBus` event types available for `session_state`; (4) the proto message name of the history record and the gate `typeClass` table; (5) `GetFeatureFlagScoped` has no production caller yet, so Story 0.8 adds the first; (7) **MCP header echo** (Task 0.7c): whether the Claude CLI prints MCP `headers` in debug logs or the `/mcp` UI (pass: the sentinel proof appears nowhere; fail: redaction scope in Task 4.9e grows to that sink); (8) **background outcome evidence** (ADR-001 decision 6): read-only, for each of the 11 live stopped hidden sessions run `GetItemSessionBySessionUUID` (or the equivalent ent query) and record role, `ended_at`, `end_reason`, and whether `review_verdict` / `triage_result` is set, plus the distinct `end_reason` values present in `item_sessions` (any value not on the ADR-001 decision 6 failure allowlist or its non-failure list is reported, not guessed); this decides how many rows show `finished` at launch, and if none do the fallback is recorded (rows ship as `stopped_unknown`; Story 1.5's alert reports it); (6) **the injected PostToolUse hook matcher**: `server/services/hook_injector.go` injects `post_tool_logging` (`/api/hooks/post-tool-use`) and `git_drift_check`; record whether the generated matcher fires for `AskUserQuestion` (run a real call against the manual instance and log the hook receipt). Story 3.4e's `acknowledged` state depends on (6): if it does not match, Task 3.4e adds `AskUserQuestion` to the injected matcher and until then `acknowledged` derives from dialog-gone only.
**Already answered from code at repair time (no spike spent; recorded so tasks can start from them)**: no `GuidanceRequestEventPayload` exists in `pkg/events/types.go`; `workflows.Scheduler` public methods are `SetAdmissionGate`, `SetTriggerFireEventRepo`, `SetRateLimiter`, `SetModelFamilies`, `Start`, `Stop`, `Reload`, `Remove`, `FireNow`, `FireTrigger`, `FireTriggerChained` (`sg --pattern 'func ($_ *Scheduler) $_($$$)' --lang go server/workflows`), i.e. no `ScheduleAt`/`ScheduleCron`/`Cancel`, no callback registration, and `Start` returns early when `repo == nil` (`scheduler.go:158-162`). Task 4.2a is therefore already adjusted (dispatcher-owned timer, ADR-006 rule 3) and Task 1.5a starts from the corrected sink in check (2).
**Round 1 additions (triad review)**: (9) **Claude Code version source** for the layout-drift canary (Task 3.3h): does the hook payload, `claude --version` at spawn, or the transcript carry the version? record the source or `none` (the canary then labels `claude_version="unknown"` and still works from the mismatch share); (10) **human-input path**: confirm that every browser-originated pane write goes through `paneWriter.SendInput` (`server/services/terminal_access.go:140`, reached from `server/services/connectrpc_websocket.go` `writerFor` and `server/services/session_service_stream_terminal.go`), list any other writer that types as the operator (for example ssq-mux, `server/services/terminal_service.go:141`), and run `tmux list-clients -F '#{client_activity} #{client_control_mode}'` on a manual-instance session with a plain `tmux attach` client to confirm a human's own terminal shows `client_control_mode=0` and a moving `client_activity` (the variables are documented in tmux 3.6a's manual; the live behavior is UNVERIFIED until run); (11) **jscpd and flaky-suite baseline**: run `cd web-app && pnpm install --frozen-lockfile && pnpm run lint:duplicates` on main and record the percentage and the margin to 0.14% (the figure in memory, 0.10% margin, is UNVERIFIED and this checkout has no `node_modules`), and record the pass/fail of the known-flaky suites in `Known flaky suites` (Story 5.4) so later failures can be attributed.
  - *Given* the worktree at HEAD, *When* checks (1)-(6) and (8)-(11) (plus the 0.7c probe, item (7)) are run, *Then* each has pasted command output and any "no" adds a note to the dependent story's first task.
**Files**: `project_plans/.../research/spike-S7.md`

##### Task 0.7a: Run the code-state reads (1) ADR-010 Reply implemented, (2) the client-event RPC handler, (3) `EventBus` types for `session_state`, (4) the history-record proto name and the gate `typeClass` table; paste the command output (S)
##### Task 0.7b: Record result (XS)
##### Task 0.7c: Probe whether the Claude CLI echoes MCP `headers` (the `X-Stapler-Session-Proof` value) in its debug output or the `/mcp` UI (S)
  - UNVERIFIED today. Method: on the manual instance, spawn a Claude session whose `mcp/<uuid>.json` carries a sentinel `X-Stapler-Session-Proof` value, run `claude --debug` (and open `/mcp`), then grep the debug log, `~/.claude` logs and the captured pane for the sentinel. **PASS**: the sentinel appears nowhere. **FAIL**: it appears in any of them; **fallback**: Task 4.9e redaction is extended to that sink (debug-log path documented, and the pane redaction treats it as in scope) and the residual-risk paragraph names it; the proof-in-file design is unchanged because the alternative (env or argv) is strictly worse (ADR-007 rule 0). Record the result in `research/spike-S7.md`.
  - *Given* a sentinel proof in the Claude MCP config, *When* the CLI is run with debug output and `/mcp` is opened, *Then* the grep result is pasted and Story 4.9's residual-risk text and Task 4.9e scope match it.
##### Task 0.7d: Item (11): measure the jscpd baseline on main and the known-flaky suites' current state; write the per-story duplication budget table (Story 5.4) from the measured margin (S)
##### Task 0.7e: Checks (5) `GetFeatureFlagScoped` has no production caller and (6) the live injected PostToolUse matcher probe for `AskUserQuestion` (and `ScheduleWakeup`, used by Task 5.1e) against the manual instance (S)
##### Task 0.7f: Check (8) read-only background-outcome evidence for the 11 live stopped hidden sessions, including the distinct live `end_reason` values (S)
##### Task 0.7g: Checks (9) the Claude Code version source for the layout canary and (10) the human-input path (`paneWriter.SendInput` callers, ssq-mux, `tmux list-clients` with a plain attached client) (S)

### Story 0.8: Flag controllers on the existing scoped-flag resolver
**As an** operator, **I want** four live-settable flags with a tested precedence rule, **so that** each epic can be rolled out and killed independently.
**Acceptance Criteria**:
- Precedence is the existing `Config.GetFeatureFlagScoped(name, scope, default)` (`config/config.go:1925`); no new resolver. A new table test over (global x scope) combinations for the four flag names and their scope vocabularies shows a scoped explicit `false` beats global `true`, and absent scope inherits global.
  - *Given* `notification_inline_actions` global `true` and scope `action:DENY` explicitly `false`, *When* `GetFeatureFlagScoped("notification_inline_actions","action:DENY",false)` is evaluated, *Then* it returns `false` and the same call for `action:APPROVE` returns `true`.
- The four flags are registered in `server/services/feature_flag_service.go` (known-flag table with `scopes`, `FlagAuditPolicy`, `flagScopeReadback`) with `*FlagController` following `replyFlagController` (`server/services/hidden_question_reply.go:85`). A live flip takes effect without restart and shows `status_detail`.
  - *Given* the manual instance with `agent_wake_triggers` off, *When* `UpdateFeatureFlag(agent_wake_triggers, true)` is called, *Then* `GetFeatureFlags` returns it `true` with `status_detail` and a dispatcher stub reports enabled within one call.
- `background_activity_feed` defaults ON; its audit policy is stated: loosening flips are audit-fsynced and refused while the audit sink is down, and `status_detail` says so.
  - *Given* the audit sink is down, *When* an operator flips `background_activity_feed` from explicit OFF to ON, *Then* the flip is refused with that reason and the flag keeps its prior value.
- The dependency/precedence matrix in this plan's "Flag precedence and dependency matrix" (ADR-008) holds for every cell: `stage:answer` is ignored while `stage:render` is off, `stage:hook_reply` while `stage:answer` is off, `channel:remote:tierB` while `channel:remote` is off, a `tool:*` or `trigger:*` scope while the dispatcher is not running; a scoped `true` under global `false` enables exactly that decision point; the server re-evaluates on every write so a stale client flag cache cannot allow one; `FLAG_OFF` maps to "This action is turned off. Open session." with an Open control.
  - *Given* global `askuserquestion_structured=false` and `stage:answer=true` with `stage:render` unset, *When* an `ANSWER_QUESTION` is invoked, *Then* the outcome is `FLAG_OFF` and no byte is written; *Given* `stage:render=true` as well, *Then* the answer proceeds.
  - *Given* `action:ANSWER_QUESTION` true and, in turn: a hidden session with `hidden_session_reply` off and `stage:answer` on (a `live_keystroke` write), a visible session with `stage:answer` on and `stage:render` off, a stopped session (durable record) with `stage:answer` off, and a `hook_reply` route with `stage:hook_reply` on but `stage:answer` off, *When* `ANSWER_QUESTION` is invoked, *Then* each is `FLAG_OFF` with zero writes; *Given* each of the four with its own gates on (hidden: `hidden_session_reply`; visible and durable: `stage:answer` and `stage:render`; `hook_reply`: all three stages, S1 PASS), *Then* it proceeds, and `action:ANSWER_QUESTION` scoped false blocks all four.
  - *Given* global `agent_wake_triggers=false` and `trigger:at=true`, *When* `schedule_wake(at)` is called with `tool:schedule_wake=true`, *Then* it succeeds and `schedule_wake(cron)` returns `flag_off`; *Given* `trigger:at` turned false afterwards, *Then* an already-pending `at` row stays `pending` (neither fired nor `missed`) until `expires_at`, when it becomes `missed` even though its scope is still false (ADR-008 carve-out).
**Files**: `server/services/feature_flag_service.go`, `server/services/feature_flags_notification_actions.go` (new controllers), `config/config.go` (no new resolver; callers only), `config/feature_flag_scopes_test.go` (extend), `web-app/src/lib/hooks/useFeatureFlagValue.ts` (new; flag reads on the web today go through `GetFeatureFlags` consumers such as `app/settings/features`)

##### Task 0.8a: Register four flags with defaults, `*FlagController`s and audit policy following `replyFlagController`; `status_detail` (S)
##### Task 0.8b: Table test over the existing `GetFeatureFlagScoped` for the new flag names and scopes; flags read at decision time (S)
##### Task 0.8c: Web hook reading a flag (+ scope) via `GetFeatureFlags` with cache and a Jest test (S)
##### Task 0.8d: Flag matrix: a single `effectiveFlag(name, decisionPoint)` caller helper applying the dependency order, the `TestFlagMatrix_ShouldResolveEveryCell_*` table test (every row of the plan's matrix, including the three-flag `ANSWER_QUESTION` composition over action x route class x hidden x render x answer), and the `FLAG_OFF` outcome copy mapping in `ActionRow` (S)
- Files: `server/services/feature_flags_notification_actions.go`, `server/services/feature_flags_matrix_test.go` (new), `config/feature_flag_scopes_test.go`, `web-app/src/components/ui/notification-actions/ActionRow.tsx`

---

# Epic 1: Background tab shows real activity (flag `background_activity_feed`)

**Goal**: Replace the history-only selector with a session-derived activity feed plus pinned exceptions and a never-blank roll-up, leaving the delivery gate unchanged (ADR-001).

### Story 1.1: Seam - `useBackgroundActivity` and a compose-only panel
**As a** maintainer, **I want** the Background logic out of `NotificationPanel.tsx`, **so that** the 837-line hotspot does not grow.
**Acceptance Criteria**:
- `NotificationPanel.tsx` no longer imports `selectBackgroundRows` directly; it calls `useBackgroundActivity()`; existing panel tests pass unchanged.
  - *Given* the existing `NotificationPanel.test.tsx` suite, *When* it runs after the extraction, *Then* all existing tests pass and the line count of `NotificationPanel.tsx` is lower than before (`wc -l`).
- Shared notification test fixtures live in one module so later epics add no duplicated mocks.
  - *Given* `pnpm run lint:duplicates` in `web-app/`, *When* run after the extraction, *Then* the reported duplication is <= its pre-change value.
**Files**: `web-app/src/lib/hooks/useBackgroundActivity.ts`, `web-app/src/components/ui/NotificationPanel.tsx`, `web-app/src/components/ui/__fixtures__/notificationFixtures.ts`

##### Task 1.1a: Extract `useBackgroundActivity` wrapping `useBackgroundSessions` + selector (S)
##### Task 1.1b: Create shared notification fixtures module and migrate existing tests to it (S)

### Story 1.2: Activity selector from the hidden-session list
**As an** operator, **I want** a row for each hidden session, **so that** background work is visible and an empty list is never mistaken for a broken one.
**Acceptance Criteria**:
- With the live-shaped input (10 stopped `review:*`, 1 stopped `diagnose:*`, zero history rows) the selector returns 11 rows and a rollup of `no_failures_reported`. The `finished` vs `stopped_unknown` split is defined by one input that exists in production: the server-computed `background_outcome` on each hidden session (Story 1.7, ADR-001 decision 6). A stopped session is `finished` only when its outcome is `completed`; `unknown` is `stopped_unknown`; `failed` is `exception_failure`. Fixtures contain no history record that production cannot produce: the gate suppresses non-error records for hidden sessions, so history contributes only failure and needs-human exceptions.
  - *Given* 11 stopped sessions (10 `review` with `backgroundOutcome:'completed'`, 1 `diagnose` with `'unknown'`) and an empty history, *When* `buildActivityRows` runs, *Then* it returns 11 `ActivityRow`s: 10 with state `finished` and role `review`, 1 with state `stopped_unknown` and role `diagnose`, and `BackgroundRollup{variant:'no_failures_reported', total:11}`.
  - *Given* the same 11 sessions all with `backgroundOutcome:'unknown'` (no `ItemSession` verdicts recorded) and an empty history, *When* the selector runs, *Then* all 11 are `stopped_unknown` with the "Stopped, outcome unknown" chip and the rollup is still `no_failures_reported`, never "healthy".
  - *Given* one stopped session with `backgroundOutcome:'failed'` and an empty history, *When* the selector runs, *Then* it is `exception_failure`, pinned first, with no history record involved.
- Exceptions pin first and win for the same session; ordering is stable between polls.
  - *Given* the same 11 sessions plus an unread ERROR record for `review:abc`, *When* the selector runs, *Then* row 0 is `review:abc` with kind `exception_failure` and there are still 11 rows (not 12).
- A session that disappears between polls renders as `departed` for one cycle, then is dropped (retention sweepers delete sessions).
  - *Given* the previous result contained `review:old` and the new list does not, *When* the selector runs with the previous rows, *Then* `review:old` appears once as `departed`.
**Files**: `web-app/src/lib/utils/backgroundActivity.ts`, `web-app/src/lib/utils/backgroundActivity.test.ts` (**new**: only `backgroundActivity.ts` exists today; any existing selector coverage lives in `BackgroundActivity.test.tsx` and must keep passing)

##### Task 1.2a: Add `ActivityState`, `SessionRole`, `ActivityRow`, `BackgroundRollup` types and pure `buildActivityRows` + `buildRollup` (S)
##### Task 1.2b: Create `backgroundActivity.test.ts` with the table tests, including the production-realistic fixtures above (S)
##### Task 1.2c: Departed handling and stable ordering (S)

### Story 1.3: Background UI - rows, roll-up, empty states, tab semantics
**As an** operator on desktop or phone, **I want** a readable feed with exceptions on top, **so that** "no news" is distinguishable from "broken".
**Acceptance Criteria**:
- Roll-up copy is softened because a stopped session with no failure record is not proof of health: no-failures -> "11 background sessions, no failures reported. Last activity 3m ago."; none -> "No background work. Review and diagnose sessions will appear here while they run." (supersedes the "all healthy" wording in `research/ux.md` s5; `design/ux.md` UX-AC1 updated to match). `stopped_unknown` rows render a distinct chip "Stopped, outcome unknown" next to `finished` "Finished".
  - *Given* the 11-session fixture with empty history, *When* `BackgroundActivity` renders, *Then* `data-testid="background-rollup"` text contains "11 background sessions, no failures reported" and does not contain "healthy".
- Rows render from cached poll state on the first commit with no loading state (pane-open budget, REQ-18, `notification_pane_open_ms`), and more than 100 rows render the first 100 with a "Show all (N)" button (UX-S1 edge table).
  - *Given* cached poll state with 150 hidden sessions, *When* the tab renders, *Then* 100 rows and a "Show all (150)" button exist on the first commit, and activating it renders 150.
- Segment control is a real `role="tablist"` with roving tabindex, Left/Right arrows, `aria-selected`; the Background tab's accessible name carries only the exception count ("Background, 1 needs attention", plain "Background" at 0), never the session total, so it does not change when informational rows arrive (UX-AC7).
  - *Given* a fixture with 11 hidden sessions of which one has an unread failure record and focus on the "Notifications" tab, *When* ArrowRight is pressed, *Then* focus and `aria-selected="true"` move to "Background" and its accessible name is "Background, 1 needs attention"; *Given* the same fixture with the failure acknowledged and 12 sessions after the next poll, *Then* the name is "Background" and unchanged by the added row.
- Rows are not reordered under the cursor; new rows show an "N new" pill (no hover-only controls, 44px targets, one scroll region).
  - *Given* the tab is open and a 12th hidden session appears in the next poll, *When* the poll completes, *Then* a button "1 new" appears and the existing rows keep their positions until it is activated.
**Files**: `web-app/src/components/ui/BackgroundActivity.tsx`, `BackgroundActivity.css.ts`, `BackgroundActivity.test.tsx`, `NotificationPanel.tsx`

##### Task 1.3a: Render activity rows, roll-up, pinned section, empty variants and the 100-row cap with "Show all (N)" (reuse the `background-empty-healthy`/`background-empty-none` test ids, an acknowledged legacy: "healthy" survives only in that id, never in copy; copy per the softened wording; distinct `stopped_unknown` chip "Stopped, outcome unknown"; group label "No failures reported (N)") (M)
- Files: `web-app/src/components/ui/BackgroundActivity.tsx`, `web-app/src/components/ui/BackgroundActivity.css.ts`, `web-app/src/components/ui/BackgroundActivity.test.tsx`, `web-app/src/lib/utils/backgroundActivity.ts`
##### Task 1.3b: Tablist semantics, count in accessible name, 44px targets, mobile single-scroll layout (S)
##### Task 1.3c: "N new" pill and no-reorder behavior (S)

### Story 1.4: Flag wiring and end-to-end spec
**As an** operator, **I want** a kill switch and proof against a real server, **so that** the fix is safe.
**Acceptance Criteria**:
- Flag OFF restores the legacy selector; ON uses the new one.
  - *Given* `background_activity_feed` is false, *When* the tab renders with the 11-session fixture, *Then* the legacy empty state shows and no activity rows exist.
- E2E: against the isolated test server with seeded hidden sessions, the tab shows rows and the roll-up (success metric: 0 rows -> 11 rows on the live-shaped data).
  - *Given* a test server seeded with 11 hidden stopped sessions and no history, *When* the Background tab is opened, *Then* `getByRole('row')` count within the tab equals 11 and the roll-up text matches.
  - *Given* the same seed with `ListSessions{hiddenOnly}` fulfilled so that 10 sessions carry `backgroundOutcome:'completed'` and the history is empty, *When* the tab is opened, *Then* at least one row has the chip "Finished" (the production-realistic case: no history rows, still `finished`); with all outcomes `unknown` every chip is "Stopped, outcome unknown".
**Files**: `web-app/src/lib/hooks/useBackgroundActivity.ts`, `tests/e2e/background-activity.spec.ts`, `tests/e2e/pages/NotificationPanel.ts`

##### Task 1.4a: Gate selector choice on the flag value (existing scoped resolver via the Task 0.8c hook) (XS)
##### Task 1.4b: **Extend** the existing `tests/e2e/background-activity.spec.ts` and `tests/e2e/pages/NotificationPanel.ts` (both exist; no parallel file). Test mode cannot create `Hidden: true` sessions, so hidden sessions are route-fulfilled on `ListSessions{hiddenOnly}` as that spec's header already does; real failures use `SendNotification`. `// @feature notification:background`, `data-testid`/ARIA locators, no `waitForTimeout` (M)
- Files: `tests/e2e/background-activity.spec.ts`, `tests/e2e/pages/NotificationPanel.ts`, `web-app/src/lib/hooks/useBackgroundActivity.ts`

### Story 1.5: Background observability
**As an** operator, **I want** row counts by state recorded, **so that** the metric in the requirements exists.
**Acceptance Criteria**:
- Each Background render posts `background_activity_rows{state}` once per poll via the client-event sink.
  - *Given* a poll yielding 10 `stopped_unknown`, 1 `exception_failure`, *When* the tab renders, *Then* the client-event call carries `{stopped_unknown:10, exception_failure:1}` and the server increments the counters.
- The pane-open time is recorded as `notification_pane_open_ms` (client-event histogram) so the 200 ms SLO (REQ-18) is observable in production, not only asserted in tests.
  - *Given* the pane opened from cached state, *When* the first action row commits, *Then* one `notification_pane_open_ms` sample is posted.
- Alert: `finished` rows are zero while `stopped_unknown` rows exist for 24 h raises an operator notification ("Background outcome evidence is empty").
  - *Given* a fake clock and polls of 11 `stopped_unknown`, 0 `finished` across 24 h, *When* the alert evaluates, *Then* exactly one notification is raised, and none when a single `finished` row appears.
**Files**: `web-app/src/lib/hooks/useBackgroundActivity.ts`, the client-event RPC handler located by Task 0.7a (not `server/features/client_event.go`, which is registry metadata)

##### Task 1.5a: Emit counts through the client-event RPC located in Task 0.7a and map to server metrics; add the `notification_pane_open_ms` sample (S)
##### Task 1.5b: `finished == 0 while stopped_unknown > 0 for 24 h` alert evaluation and operator notification (XS)

### Story 1.6: Background error states and the shared offline banner
**As an** operator, **I want** a visible, recoverable state when the session poll fails or I am offline, **so that** stale data is never mistaken for current data (UX-AC5, UX-AC6, UX-AC56).
**Acceptance Criteria**:
- Poll failure keeps last-known rows and adds a retry banner.
  - *Given* a successful poll followed by a failing `ListSessions{hiddenOnly}` at 10:04, *When* the tab renders, *Then* the 11 rows remain, the text "Couldn't refresh background sessions. Showing data from 10:03." is present, and clicking Retry issues a new poll and clears the banner on success.
- Open on a departed session shows "Session no longer exists" instead of navigating.
  - *Given* a `departed` row, *When* Open is activated, *Then* the row shows "Session no longer exists" and no navigation occurs.
- A shared `OfflineBanner` (used by every pane surface) appears within 2 s of connectivity loss as a persistent text element (not a live region of its own; the single global polite region announces "Offline" once) "Offline. Showing last known state.", action controls become `aria-disabled` with reason "Offline", drafts survive, no focus movement, and actions work again after reconnect without reload.
  - *Given* the panel open with a half-typed answer draft, *When* `navigator.onLine` flips false then true, *Then* the banner appears then disappears, the draft text is unchanged and focus did not move.
- Cold-load, reconnecting and request-timeout states (UX-AC73): with no cached data "Loading..." appears only after 150 ms and becomes "Still loading." with Retry after 8 s; a failed poll or dropped stream while `navigator.onLine` is true shows "Reconnecting to the server. Showing last known state." (backoff 1 s doubling to 15 s) and after 30 s "Can't reach the server." with Retry; an action pending 10 s shows "Still sending..." (not announced) and at 30 s the no-response copy with Retry on the same idempotency key.
  - *Given* fake timers and a never-resolving `ListSessions`, *When* 149 ms pass, *Then* no loading text exists; *When* 151 ms pass, *Then* "Loading..." is present; *When* 8 s pass, *Then* "Still loading." and a Retry button are present. *Given* `navigator.onLine` true and the poll failing, *When* 30 s pass, *Then* the banner reads "Can't reach the server." with Retry, rows are kept, and exactly one live region exists in the panel.
- Recovery and the two Notifications tiers (`design/ux.md` section 9): after Offline or Reconnecting ends, action controls stay `aria-disabled` with the reason "Updating..." until the first successful refresh completes, and Needs you shows a plain-text "As of {time}" line; Needs you and Recent activity each have their own empty, loading and fetch-failed copy; drafts live in `sessionStorage` with a memory fallback.
  - *Given* a reconnect after 40 s offline and a refresh that has not resolved, *When* the panel is inspected, *Then* every action control is `aria-disabled` with description "Updating...", and *When* the refresh resolves, *Then* the controls re-enable and "As of" shows the refresh time; *Given* no notifications, *Then* Needs you reads "Nothing needs you right now." and Recent activity "No recent activity."; *Given* a failed fetch with cached rows, *Then* "Couldn't load notifications. Showing data from {time}." with Retry appears and the rows are kept; *Given* `sessionStorage` throwing, *Then* a typed draft is still restored after closing and reopening the panel (memory fallback) and nothing throws.
**Files**: `web-app/src/components/ui/OfflineBanner.tsx`, `web-app/src/lib/hooks/useOnlineStatus.ts`, `web-app/src/lib/hooks/useBackgroundActivity.ts`, `BackgroundActivity.tsx`, `tests/e2e/background-activity.spec.ts`

##### Task 1.6a: Poll-failure state with last-known rows, timestamp and Retry (S)
##### Task 1.6b: `OfflineBanner` + `useOnlineStatus`, disabled-with-reason wiring contract for `ActionRow` (M)
- Files: `web-app/src/components/ui/OfflineBanner.tsx`, `web-app/src/components/ui/OfflineBanner.test.tsx`, `web-app/src/lib/hooks/useOnlineStatus.ts`, `web-app/src/components/ui/NotificationPanel.tsx`
##### Task 1.6c: Departed-session Open copy (XS)
##### Task 1.6d: Cold-load, reconnecting (browser online, server down) and in-flight timeout states with their copy, backoff and exits; extend `OfflineBanner` to three states; one live region only (S)
##### Task 1.6e: Controls stay disabled until the first successful refresh after recovery, the "As of {time}" line, Needs-you and Recent empty/loading/fetch-failed copy, and `sessionStorage` draft persistence with a memory fallback (S)

### Story 1.7: Background outcome from the session list
**As an** operator, **I want** a stopped background session labelled "Finished" only when the system recorded a completed outcome, **so that** the tab neither claims health it cannot know nor leaves every stopped session as "outcome unknown" forever (pre-mortem P1-a, ADR-001 decision 6).
**Why this story exists**: the earlier definition of `finished` ("stopped AND a non-error history record exists") needed a record the delivery gate deliberately suppresses for hidden sessions (`server/deliverygate/policy.go` `typeClass`: TASK_COMPLETE and PROCESS_FINISHED are `ClassRoutine`); the old Story 1.2 fixture added such a record, so its passing test described a state production never produces. What already exists in production (VERIFIED by reading, not run against the live database): `ItemSession` rows keyed by `session_uuid` with `session_role`, `ended_at`, `end_reason`, `review_verdict` and `triage_result` (`proto/session/v1/backlog.proto` `ItemSession`; `session.EntRepository.GetItemSessionBySessionUUID`, `session/storage_backlog.go:386`) and `SessionStatus.CRASHED` with `exit_reason` (`proto/session/v1/types.proto`); the web client today maps `CRASHED` to `other` (`useBackgroundSessions.ts` `stateOf`) and reads neither. UNVERIFIED until Spike S7 item (8) runs: how many of the 11 live sessions actually have a recorded verdict.
**Acceptance Criteria**:
- A pure function `ComputeBackgroundOutcome(status, itemSession)` returns `completed` when the session's `ItemSession` carries a recorded `review_verdict` (review role) or `triage_result` (triage role); `failed` when the status is `CRASHED` or `end_reason` is on the **failure allowlist** (ADR-001 decision 6: `pool_saturated`, `idle`, `timeout`, `claude_not_found`, `subprocess_start_error`, `fanout_ceiling`, `cost_ceiling`, `other`, `parse_error`, `dispatch_failed`, `dispatch_incomplete`, `jules_timed_out`, `jules_session_missing`, `jules_failed`; every other value, including unknown future ones, is a non-failure); otherwise `unknown` (always `unknown` for the diagnose role and for a session with no `ItemSession`).
  - *Given* an `ItemSession{role:review, review_verdict:{overall_outcome:"PASS"}, end_reason:""}` and status `STOPPED`, *When* computed, *Then* `completed`; *Given* status `CRASHED`, *Then* `failed`; *Given* `ItemSession{end_reason:"shutdown"}` with no verdict, *Then* `unknown` (our own restart is not a failure); *Given* `ItemSession{role:diagnose}` with `ended_at` set, *Then* `unknown`; *Given* no `ItemSession`, *Then* `unknown`; *Given* `end_reason:"timeout"` and no verdict, *Then* `failed`; *Given* `end_reason:"jules_completed"` or an unlisted `"some_future_reason"`, *Then* `unknown` (default non-failure); *Given* a recorded verdict and `end_reason:"timeout"`, *Then* `completed`; *Given* the list of every `end_reason` literal passed to `UpdateItemSessionEndedWithReason` / `WithDetail` (recorded by `git grep` in the test's comment), *Then* `TestEndReasonClassification_ShouldCoverEveryKnownProducer` finds each in exactly one of the failure and non-failure maps.
- `ListSessions{hidden_only}` populates `Session.background_outcome` (additive proto field, next free number in `Session`, confirmed at implementation) for each returned session using **one batched query per call** (a new repository method `ListItemSessionsBySessionUUIDs(uuids)`; no batch lookup by session UUID exists today, only the per-session `GetItemSessionBySessionUUID`, which is not called in a loop), bounded by the existing hidden-session count (tens), and leaves it empty for non-hidden requests.
  - *Given* a seeded database with 3 hidden sessions (one with a recorded verdict, one crashed, one without an `ItemSession`), *When* `ListSessions{hidden_only:true}` is served, *Then* the outcomes are `completed`, `failed`, `unknown`; *When* `ListSessions{}` is served, *Then* the field is empty; *Given* 40 hidden sessions and a counting fake repository, *When* `ListSessions{hidden_only:true}` is served, *Then* the batched method is called once and `GetItemSessionBySessionUUID` zero times.
- The web hook maps the field into `HiddenSessionSummary.backgroundOutcome` (`completed|failed|unknown`, absent from an older server maps to `unknown`).
  - *Given* a `ListSessions` response without the field, *When* mapped, *Then* every summary has `backgroundOutcome:'unknown'` and the tab shows "Stopped, outcome unknown" rather than throwing.
**Files**: `proto/session/v1/types.proto` (edited once, by Task 2.1a, so the file has one editor), `session/background_outcome.go` (new), `session/background_outcome_test.go`, `server/services/session_service_crud.go`, `web-app/src/lib/hooks/useBackgroundSessions.ts`, `web-app/src/lib/utils/backgroundActivity.ts`

##### Task 1.7a: Pure `ComputeBackgroundOutcome` with the failure and non-failure `end_reason` maps and the table tests above (the proto field is added by Task 2.1a) (M)
- Files: `session/background_outcome.go`, `session/background_outcome_test.go`
##### Task 1.7b: Populate the field in `ListSessions{hidden_only}` with the new batched `ListItemSessionsBySessionUUIDs` (one query per call), the call-count test and an integration test over a seeded in-memory ent database (S)
##### Task 1.7c: Map the field in `useBackgroundSessions` (including the old-server default) with a Jest test (S)

### Story 1.8: Background feed governance
**As an** operator, **I want** failures surfaced by name, a badge that means something, a way to retire permanent rows, and a bounded list, **so that** the tab stays trustworthy as it grows (pre-mortem #8; `design/ux.md` 1.4; UX-AC60, 66-72).
**Acceptance Criteria**:
- A new failure or needs-human session is named in a sticky failure strip, never only counted in an "N new" pill; an existing row that becomes a failure gains its chip in place; Show moves focus; the strip shows at most 3 names then "N more failures" (UX-AC66).
  - *Given* the tab open with a card engaged and a poll that marks `review:pr-1042` failed, *When* the poll completes, *Then* the strip text contains "review:pr-1042 failed", no existing row changed position, the polite region said "1 background session needs attention" once and `document.activeElement` is unchanged; *When* Show is activated, *Then* focus is on that row.
- The bell badge and the Background tab badge follow the composition rule (UX-AC67).
  - *Given* 3 unread actionable items, 1 Unconfirmed answer, 2 unacknowledged Background failures, 10 Finished rows and 1 un-escalated peer request, *When* the badge is computed, *Then* it is 6 and the Background tab badge is 2.
- Mark reviewed / Mark all reviewed retire permanent rows without a server write; storage failure degrades to page-session marks with the stated note (UX-AC68).
  - *Given* 11 `stopped_unknown` rows and a working `localStorage`, *When* "Mark all reviewed" is activated and the page reloaded, *Then* the roll-up reads "... no failures reported (11 reviewed)"; *Given* `localStorage` throwing, *Then* the marks apply for the session and the text "Marks are not saved in this browser" is present and nothing throws.
  - *Focus after the operator's own list actions* follows the `design/ux.md` section 7 keyboard model: activating "N new" moves focus to the first merged row's header, Mark reviewed to the next row in the group (else the group heading, `tabindex="-1"`), Mark all reviewed to the group heading. *Given* 3 unreviewed rows with focus on row 2's Mark reviewed, *When* it is activated, *Then* focus is on row 3's header; *Given* the last row, *Then* focus is on the group heading; *Given* "1 new" activated, *Then* focus is on the merged row's header.
- Rows older than 24 h that are Finished, `stopped_unknown` or reviewed collapse into "Older (N)"; at most 100 rows render with "Show all (N)"; the list is never windowed; 500 rows commit within the first-render budget (UX-AC69).
  - *Given* 500 hidden-session summaries (480 older than 24 h), *When* the tab renders, *Then* one render pass commits at most 100 + 20 row elements, "Older (480)" is a collapsed disclosure whose count is in the button text, and the `Profiler` commit time is under 100 ms in the Jest run (soft on CI).
- A first-run note explains the tab once per browser (UX-AC70); row click semantics are toggle-only (UX-AC71); no notification id renders in both tabs (UX-AC72).
  - *Given* a hidden session with a pending question and the same session in the Background list, *When* both tabs render, *Then* the question controls appear only in Needs you and the Background row has the link chip "Needs you (1)" and no buttons other than Open and Mark reviewed; *Given* a click on row text, *Then* details toggle and no navigation occurs.
**Files**: `web-app/src/components/ui/BackgroundActivity.tsx`, `BackgroundFailureStrip.tsx` (new), `web-app/src/lib/hooks/useBackgroundActivity.ts`, `web-app/src/lib/utils/badgeCount.ts` (new), `web-app/src/lib/hooks/useReviewedMarks.ts` (new), `BackgroundActivity.test.tsx`

##### Task 1.8a: `BackgroundFailureStrip`, in-place failure chip and the Show focus behavior (M)
- Files: `web-app/src/components/ui/BackgroundFailureStrip.tsx`, `web-app/src/components/ui/BackgroundActivity.tsx`, `web-app/src/lib/hooks/useBackgroundActivity.ts`, `web-app/src/components/ui/BackgroundActivity.test.tsx`
##### Task 1.8b: `badgeCount` pure function and the Background tab badge, with the exact composition table test (S)
##### Task 1.8c: `useReviewedMarks` (try/catch `localStorage`, session fallback, note), Mark reviewed / Mark all reviewed, Reviewed group and first-run note, plus the client-event counter `background_reviewed_mark_total{storage}` that measures the accepted per-browser limitation (S)
##### Task 1.8d: Older (24 h) collapse, the 500-row render test, row click semantics, link chip and the no-double-display assertion (S)

---

# Epic 2: Inline notification actions (flag `notification_inline_actions`)

**Goal**: One descriptor model, one dispatcher RPC and one UI row component so approvals, questions, guidance and backlog decisions resolve from the pane in one interaction (ADR-002, ADR-004).

### Story 2.1: Proto and registry
**As a** developer, **I want** the contract defined first, **so that** server and web build against the same types.
**Acceptance Criteria**:
- `InvokeNotificationAction` and its messages exist; every enum has `*_UNSPECIFIED = 0` and the type prefix (buf STANDARD lint); `make proto-gen` and `make lint` pass; generated output uncommitted.
  - *Given* the proto edits, *When* `make proto-gen && make lint` runs, *Then* both pass and `git status` shows no `gen/` files staged.
- `make registry-generate` produces feature files for the new RPC.
  - *Given* the new RPC with `// +api: notification:invoke-action`, *When* `make registry-generate` runs, *Then* `docs/registry/features/` contains the new mapping and `make registry-diff` is empty afterwards.
**Files**: `proto/session/v1/notification_action.proto` (new), `proto/session/v1/session.proto`, `proto/session/v1/types.proto`, `docs/registry/features/`

##### Task 2.1a: Define `NotificationActionDescriptor`, `ActionKind`, `ActionTier`, `ActionOutcome`, request/response; add `repeated NotificationActionDescriptor actions` to the history record message; add RPC; also add `Session.background_outcome` (next free field number in `Session`, confirmed at implementation) for Story 1.7 so `proto/session/v1/types.proto` has exactly one editor (S)
##### Task 2.1b: `make registry-generate`, commit feature files only (XS)

### Story 2.2: Descriptor builder
**As an** operator, **I want** each notification to know which actions are valid right now, **so that** buttons are never offered for a resolved item.
**Acceptance Criteria**:
- Pending approval -> `APPROVE` + `DENY` (Create rule is not an `ActionKind`; it stays the existing desktop-only control behind the `More` menu, Story 2.5); pending question with options -> `ANSWER_QUESTION` carrying option labels; open guidance -> `ANSWER_GUIDANCE`; backlog item awaiting a decision -> `BACKLOG_DECISION` with choices; resolved entities -> only `OPEN`.
  - *Given* a notification for `question_id=q1` whose `PendingQuestionStore` entry has labels ["Redis","Memcached"] and `reply_unavailable=false`, *When* `GetNotificationHistory` is served, *Then* its descriptor list is `[ANSWER_QUESTION{options:["Redis","Memcached"], tier:B}, OPEN]`.
  - *Given* the same question already answered, *When* history is served, *Then* descriptors are `[OPEN]` only.
- Backlog decision notifications carry a stable decision schema in metadata (`decision_schema`, choices) (gap 10).
  - *Given* item `b608ab1e-...` in `review` with a recorded verdict awaiting merge/rework, *When* the notifier emits, *Then* metadata `decision_schema` contains choices `["accept","rework","merge"]` mapped to the existing RPCs.
- Descriptor assembly is batched per history page (one pending-question lookup, one backlog-state lookup and one approval lookup per page, not per record), descriptors are assembled only for unresolved records, and 300 records over 40 sessions assemble within a stated budget so the 200 ms pane SLO (REQ-18) holds.
  - *Given* 300 history records over 40 sessions and counting fakes for the three stores, *When* one page is served, *Then* each store is called once per page and the benchmark `BenchmarkBuildDescriptors_300RecordsWith40Sessions` reports p95 under 100 ms.
- Every notification emission added by this project is asserted in a table test to map to a non-routine gate class where hidden sessions are involved.
  - *Given* the table of new emissions (`INPUT_REQUIRED` question, `CONFIRMATION_NEEDED` decision, peer-help escalation as `INPUT_REQUIRED`, wake missed/skipped/rate-limited as failure class with `HintFailure`, and the `notify_operator` reminder as a system-level notification with no session binding, `VisibilityNotASession`), *When* `deliverygate.ShouldDeliver` runs with a hidden session, *Then* each is delivered and the test fails if a new emission is not in the table.
**Files**: `server/services/notification_actions.go`, `server/services/notification_actions_test.go`, `server/services/backlog_notifier.go`, `server/deliverygate/policy_test.go`

##### Task 2.2a: Builder with approval, guidance and pending-question descriptors (M)
- Files: `server/services/notification_actions.go`, `server/services/notification_actions_test.go`, `server/services/session_service_delegates.go`
##### Task 2.2b: Backlog decision descriptors; map choices to existing RPCs (`TransitionBacklogItemStatus`, `SubmitManualReview`, `TriggerReReview`, `MergePR` - confirm against `proto/session/v1/backlog.proto` first; any choice lacking an RPC is emitted as `OPEN` and recorded) and add `decision_schema` metadata in `server/services/backlog_notifier.go` (M)
- Files: `server/services/backlog_notifier.go`, `server/services/notification_actions.go`, `server/services/notification_actions_test.go`
##### Task 2.2c: Gate-class table test for new emissions (S)
##### Task 2.2d: Pass a `ListenerClass` derived from the request context into the assembler (`GetNotificationHistory` is served via `SessionService` -> `NotificationService`, `server/services/session_service_delegates.go:269`, `server/services/notification_service.go:209`, with no listener identity today; `EvaluateLocalWrite` at `server/services/local_write_verdict.go:57` is the only precedent); test that the same history page yields `available=false, reason=desktop_only` for Tier B on `remote` and `available=true` on `local` (S)
##### Task 2.2e: Batch the lookups per page, skip resolved records, add the 300-record benchmark and the call-count test (S)

### Story 2.3: Dispatcher `InvokeNotificationAction`
**As an** operator, **I want** one reliable action endpoint, **so that** results are consistent and safe.
**Acceptance Criteria**:
- Same `IdempotencyKey` + payload -> cached result, no second side effect; same key + different payload -> `INVALID`.
  - *Given* `APPROVE` for `approval_id=a1` invoked twice with key `k1`, *When* both calls complete, *Then* both return `APPLIED` and `ResolveApproval` ran once.
- Flag off -> `FLAG_OFF`; unknown/resolved target -> `ALREADY_RESOLVED` or `STALE`, never a Connect error.
  - *Given* `askuserquestion_structured` answer stage off and a `ANSWER_QUESTION` on a visible session, *When* invoked, *Then* the outcome is `FLAG_OFF` with zero writes.
- Delegates keep their own checks: a classifier-blocked approval still needs the existing "Approve anyway" confirm; a secret-scan hit is refused.
  - *Given* an approval blocked by CI status, *When* `APPROVE` is invoked without `confirm`, *Then* the outcome is `INVALID` and the approval is unresolved.
- p95 < 1 s on localhost (REQ-19). The number is proven by `BenchmarkInvokeNotificationAction_Approve100Sequential` (opt-in via `make benchmark-tier1`, not in the default `go test`, per `notimesleeptest` and `deterministic-fast-tests`), a deterministic fake-clock unit test guards the hot path against blocking calls, and Story 2.9 measures the real round trip on the isolated server. The dispatcher depends on one small consumer-side interface per delegate (`ApprovalResolver`, `QuestionReplier`, `GuidanceAnswerer`, `BacklogDecider`) defined in `notification_action_service.go` and a closed `switch` over `ActionKind`; it never holds `*SessionService`.
  - *Given* 100 sequential `APPROVE` calls on a fake approval store, *When* the benchmark runs, *Then* p95 < 1 s; *Given* the same calls in the unit test with fake delegates and a fake clock, *Then* no call blocks on a timer.
- The in-memory idempotency cache is not the only protection after a restart: each `BACKLOG_DECISION` delegate (`TransitionBacklogItemStatus`, `SubmitManualReview`, `TriggerReReview`, `MergePR`) is verified idempotent on its own or guarded by entity state.
  - *Given* `MergePR` already applied for item X and the dispatcher restarted (cache empty), *When* the same `BACKLOG_DECISION` merge is retried, *Then* the outcome is `ALREADY_RESOLVED` and no second merge call is made.
- Every invocation writes an audit entry with `channel` (`web_local`, `web_remote`, `mcp`, `unknown`) and `caller_kind` (`operator_ui`, `session`, `unknown`) so an action with no UI channel (a same-user agent calling `:8543` directly, accepted risk ADR-004 and ADR-007 rule 0, pre-mortem #17) is queryable rather than invisible.
  - *Given* an `APPROVE` posted through the Connect handler without the SPA's client header and one posted by the SPA, *When* both complete, *Then* their audit entries carry `channel`/`caller_kind` of `unknown`/`unknown` and `web_local`/`operator_ui` respectively, and the first increments `notification_inline_action_total{channel="unknown"}`.
- Stub seam for `ANSWER_QUESTION` (Story 2.3 lands in W3, Story 3.4's `AnswerRouter` in W4): the dispatcher depends only on the consumer-side `QuestionReplier` interface. Until Task 3.4a lands, wiring supplies `hiddenReplyAdapter` over `PaneReplyCore` (Story 2.4), which serves hidden sessions exactly as today and returns `FLAG_OFF` for visible sessions; Task 3.4a swaps the router in behind the same interface with no change to the dispatcher or the UI, and Story 2.9's `ANSWER_QUESTION` e2e runs after Story 3.4.
  - *Given* a visible session and the adapter, *When* `ANSWER_QUESTION` is invoked, *Then* the outcome is `FLAG_OFF` with zero writes; *Given* a hidden session, *Then* the result is the existing `hidden_session_reply` result.
**Files**: `server/services/notification_action_service.go`, `notification_action_service_test.go`, `server/server.go`

##### Task 2.3a: Handler skeleton, flag, idempotency (payload-hash cache with bounded LRU), outcome mapping, audit, metrics (M)
- Files: `server/services/notification_action_service.go`, `server/services/notification_action_service_test.go`, `server/services/notification_action_metrics.go`, `server/server.go`
##### Task 2.3b: Approve/Deny delegates (S)
##### Task 2.3c: Backlog decision delegates, then resolve notification via reaper event (M)
- Files: `server/services/notification_action_service.go`, `server/services/notification_action_backlog.go` (new), `server/services/notification_action_service_test.go`, `server/notifications/resolved_item_reaper.go`
##### Task 2.3d: Guidance answer delegate (`AnswerGuidanceRequest`) with `ALREADY_RESOLVED` when `applied=false` (S)
##### Task 2.3e: Verify each backlog delegate's idempotency (or add an entity-state guard / persisted key) and add the post-restart retry test; define the consumer-side delegate interfaces (S)

### Story 2.4: Seam - extract `PaneReplyCore`
**As a** maintainer, **I want** the claim/audit/lease/one-write core separated from the hidden-only policy, **so that** visible-session answers reuse it instead of copying it.
**Acceptance Criteria**:
- No behavior change for hidden sessions: the existing `hidden_question_reply_test.go` suite passes unchanged, and fault-injection tests (`NOT_SENT` vs `SEND_INDETERMINATE`) still pass.
  - *Given* the existing test suite, *When* `go test ./server/services -run HiddenQuestionReply -race` runs after extraction, *Then* all pass.
- `NOT_HIDDEN` remains returned for visible sessions when the new stage flag is off.
  - *Given* a visible session and `stage:answer` off, *When* `ReplyToPendingQuestion` is called, *Then* the outcome is `NOT_HIDDEN`.
- The generic "is any dialog showing" check has one owner and lands here in W1, beside `DialogMatch`, so its three consumers (Story 3.3's sampler, Task 3.4e, Task 4.3c's wake-write safety) need no cross-epic dependency: `session.PaneShowsDialog(capture) DialogPresence` returns `Showing` (the `Enter to select` footer, a permission prompt or any recognised dialog), `Absent` (only when the capture positively shows an idle composer layout) or `Indeterminate` (anything unrecognised, an empty capture or a capture error); callers treat only `Absent` as "no dialog" and `Indeterminate` as "do not write".
  - *Given* the fixtures in `session/testdata/reply_dialogs/` (the `AskUserQuestion` captures), a permission-prompt capture, an idle-composer capture, an empty string and a capture with a forged numbered row after the footer, *When* `PaneShowsDialog` runs on each, *Then* the results are `Showing`, `Showing`, `Absent`, `Indeterminate` and `Showing`, and a table test fails if a fixture added later has no expected value.
**Files**: `server/services/hidden_question_reply.go`, `server/services/pane_reply_core.go`, `session/pane_reply.go`, `session/dialog_presence.go` (new), `session/dialog_presence_test.go` (new)

##### Task 2.4a: Extract `PaneReplyCore` with an `AnswerTarget` parameter; keep limiter constants (per-session 1/5 s, global 10/min) in one place; extract the existing fault-injecting pane-write test double from `hidden_question_reply_test.go` (fault cases `NOT_SENT` vs `SEND_INDETERMINATE`) into a shared `_test` helper, because no type named `paneSubmitter` fault double exists and Stories 3.4, 3.5 and the visible-session tests reuse it (M)
- Files: `server/services/hidden_question_reply.go`, `server/services/pane_reply_core.go`, `server/services/hidden_question_reply_test.go`, `server/services/pane_reply_testdouble_test.go` (new shared helper)
##### Task 2.4b: Run regression, race and sendkeysguard checks (XS)
##### Task 2.4c: `session.PaneShowsDialog` tri-state check and its fixture table test, consumed by Task 3.3f, Task 3.4e and Task 4.3c (S)

### Story 2.5: `ActionRow` component family
**As an** operator, **I want** the same action UI on every notification type, **so that** it behaves predictably.
**Acceptance Criteria**:
- `NotificationItem` renders actions only from descriptors via `ActionRow`; legacy Approve/Deny code path is removed behind the flag-on branch but retained for flag-off.
  - *Given* a descriptor list `[APPROVE, DENY, OPEN]`, *When* `NotificationItem` renders with the flag on, *Then* buttons "Approve" and "Deny" appear with `data-testid` of their kind and no separate legacy handlers are used.
- Outcome-to-UI mapping (delivery strings come from `design/ux.md` 3.1, the single copy source): `APPLIED` -> "Approved" / "Decided: {choice}" using the server value, and for a live-pane answer the delivery line row 2 "Sent - waiting for the agent" ("Answered: {value}" only once acknowledged); `ALREADY_RESOLVED` -> read-only "Already handled: {value}" with no error toast; `STALE` -> "Already handled in the session. No action needed." when the dialog is gone, or "The session is now asking something else. Open session to answer it." when a different dialog is showing; `NOT_SENT` -> "Not sent. Nothing was typed into the session. Retry" (the `human_active` reason: "Not sent: someone is typing in that session. Retry when they are done.") with **Retry** (safe: nothing was written) and **Open session**; `SEND_INDETERMINATE` -> "Sent, but we couldn't confirm it arrived. Check the session before retrying." with **Open session** as the primary and **Retry** only behind an inline "Retry anyway?" confirm; double-tap is disabled after the first press. The two failure strings are distinct and neither is shown as answered (UX-AC11).
  - *Given* `ALREADY_RESOLVED` returned with `value="Redis"`, *When* the row updates, *Then* it shows "Already handled: Redis" and no `role="alert"` toast is created; *Given* `NOT_SENT` and, separately, `SEND_INDETERMINATE` for the same row, *When* each renders, *Then* the two strings differ exactly as listed, `NOT_SENT` offers Retry directly with Open session, and `SEND_INDETERMINATE` offers Open session and a Retry that appears only after "Retry anyway?" is confirmed; *Given* `STALE` with a different dialog on screen, *Then* the text is "The session is now asking something else. Open session to answer it." and not "Already handled".
- Inline sub-steps, no modal (UX-AC15): Deny opens an optional one-line reason field with "Send denial" and "Back"; Merge (and any `requires_confirm` action) shows "Merge PR #901?" with "Confirm merge" and "Back" inside the row, focus on Confirm merge, Back restores the row and the draft. No button is labelled with a bare "Cancel" (vocabulary table, UX-AC76).
  - *Given* a `DENY` descriptor, *When* Deny is activated, *Then* a reason field appears with no `role="dialog"` and Back restores the original buttons with the typed draft kept; *Given* a `BACKLOG_DECISION` merge choice with `requires_confirm`, *When* it is activated, *Then* focus is on "Confirm merge".
- `RATE_LIMITED` shows "Too many actions. Try again in {n}s." as text with a countdown and re-enables at zero (UX-AC17); `INVALID` shows "That action isn't valid anymore. Refresh." with a Refresh button that refetches descriptors; `STALE` on an auto-approved approval shows "Handled automatically by rule {name}" with Open, otherwise the `STALE` strings above; actionable unread items have no dismiss control and "Mark all read" leaves them unread (UX-AC16).
  - *Given* `RATE_LIMITED` with `retry_after=3`, *When* fake timers advance 3 s, *Then* the text counts 3, 2, 1 and the control loses `aria-disabled` at 0; *Given* `INVALID`, *When* Refresh is activated, *Then* `GetNotificationHistory` is called once and the row re-renders from the new descriptors.
- The pane renders action rows from cached state without an awaited fetch before first paint, and the open-to-first-row time is posted as `notification_pane_open_ms` (REQ-18, 200 ms).
  - *Given* cached history with 40 actionable items, *When* the pane opens, *Then* the first row commits in the same render pass (no loading state) and Playwright's `performance.measure` between the open click and the first `ActionRow` is under 200 ms on the isolated server (annotated soft on CI).
- The approval row shows the complete command text (UX-AC64) and Approve/Deny spacing on mobile (UX-AC65): wrapped with no ellipsis, a labelled keyboard-scrollable region with a character count and Copy over 8 lines or 400 characters, "Can't show this command. Open session to review it." with Open instead of Approve for an unrenderable input; Deny then Approve separated by at least 24px below 900px.
  - *Given* an approval whose command is 1,204 characters over 14 lines, *When* the row renders, *Then* the full text is in the DOM, the region has `tabindex="0"` and `aria-label="Command, scrollable"`, "1,204 characters" and a Copy button exist, and no `text-overflow: ellipsis` style applies; *Given* a tool input that cannot be stringified, *Then* Approve is absent and Open is present; *Given* a 390px viewport, *Then* the Deny-to-Approve gap is >= 24px and Deny precedes Approve in DOM order.
- Create rule is not in the primary row (UX-AC65, `design/ux.md` 2.1 and 2.4): the approval row's primary controls are Approve, Deny and Open; **Create rule...** lives in a `More` menu rendered only at widths >= 900px on the local listener, and opens an inline confirm ("Create a rule that auto-approves {tool and pattern}?" with Create rule and Back); it is not rendered below 900px or on `:8444`.
  - *Given* an approval descriptor at 1280px on the local listener, *When* rendered, *Then* the DOM order is Approve, Deny, More, Open, no element named "Create rule" is in the primary row, activating More then Create rule shows the confirm and nothing is created until "Create rule" is confirmed; *Given* a 390px viewport, *Then* no More button exists; *Given* the remote listener, *Then* no More button exists.
- UI copy comes from one vocabulary module so the operator terms in `design/ux.md` (scheduled prompt, Reminder (you), Cancel prompt, Cancel request, Back, Clear, Keep, Answer for {peer}) are enforced by a test, not by review.
  - *Given* every string exported by `web-app/src/lib/copy/notificationCopy.ts` and the panel rendered in every state of `design/ux.md` sections 1.3, 2.3, 3.1, 3.3, 5.3 and 6.3 (the DOM text is scanned too), *When* `copyGuard.test.ts` runs, *Then* none contains the word "wake" or "wakes" (the one allowed occurrence is the flag name `agent_wake_triggers` inside the flag-settings link), the phrase "Waiting on" (so no section label and no "Waiting on you" chip; the chip reads "Needs you (N)"), or equals a bare "Cancel", and the string for `FLAG_OFF` is "This action is turned off. Open session.". The guard covers UI strings only; internal identifiers (`wake_trigger`, `schedule_wake`, `CANCEL_WAKE`, `WakeTrigger`, RPC and file names) keep their names (`design/ux.md` section 0).
- Test fixtures shared so `pnpm run lint:duplicates` stays under 0.14%.
  - *Given* the new test files, *When* `pnpm run lint:duplicates` runs, *Then* it exits 0.
**Files**: `web-app/src/components/ui/notification-actions/ActionRow.tsx`, `ActionRow.css.ts`, `ActionRow.test.tsx`, `web-app/src/lib/hooks/useInvokeNotificationAction.ts`, `web-app/src/components/ui/NotificationItem.tsx`

##### Task 2.5a: `ActionRow` + `useInvokeNotificationAction` (idempotency key per attempt, pending state, outcome mapping, optimistic UI with rollback) (M)
- Files: `web-app/src/components/ui/notification-actions/ActionRow.tsx`, `web-app/src/components/ui/notification-actions/ActionRow.css.ts`, `web-app/src/components/ui/notification-actions/ActionRow.test.tsx`, `web-app/src/lib/hooks/useInvokeNotificationAction.ts`
##### Task 2.5b: Integrate into `NotificationItem`; keep legacy path for flag-off (M)
- Files: `web-app/src/components/ui/NotificationItem.tsx`, `web-app/src/components/ui/NotificationPanel.tsx`, `web-app/src/components/ui/notification-actions/ActionRow.tsx`, `web-app/src/components/ui/NotificationItem.test.tsx`
##### Task 2.5c: Shared fixtures and parametrized tests (S)
##### Task 2.5d: Inline Deny-reason and Merge confirm, `RATE_LIMITED` countdown, `INVALID` Refresh, auto-handled STALE variant, no-dismiss rule, and the pane-open budget test (S)
##### Task 2.5e: `notificationCopy.ts` vocabulary module and `copyGuard.test.ts` (UX-AC76); row-click semantics shared with Story 1.8 (S)
##### Task 2.5f: Approval command display (full text, scroll region, count, Copy, fail-closed), mobile Deny/Approve spacing, and the desktop-only `More` menu with the Create rule inline confirm (S)

### Story 2.6: Keyboard, a11y and mobile behavior
**As a** keyboard or phone user, **I want** accessible, large-target actions, **so that** the one-interaction promise holds on every form factor.
**Acceptance Criteria**:
- The keyboard model is the table in `design/ux.md` section 7 and the lifecycle is the table in 7.1; this story asserts them and defines nothing of its own: Up/Down and J/K move item focus identically, Enter expands, digits only move focus in a digit-armed card, Esc closes the innermost layer first, drafts survive in `sessionStorage`; a resolved row stays in place until nothing is engaged for 750 ms and focus moves to the next item only then, and only if it was inside the leaving row; the polite status region announces the short result (region present from first paint).
  - *Given* focus on item 2 of 3 and an approval resolves with the pointer elsewhere, *When* 750 ms pass and the row leaves the list, *Then* focus is on item 3 (not engaged, not digit-armed) and the status region text was "Approved" once; *Given* the pointer still over the resolved row, *Then* the row stays showing "Approved" and focus does not move; *Given* Esc pressed inside an open Deny reason, *Then* only the sub-step closes, a second Esc collapses the card and a third closes the panel and focuses the bell; *Given* ArrowDown and `j` on the same item header, *Then* focus moves identically.
- Targets >= 44px, stacked options below 900px, full-width Submit, 16px textarea, no hover-only controls; toasts carrying actions pause on hover/focus and the item remains in the panel.
  - *Given* a 390px viewport (touch), *When* an `ANSWER_QUESTION` row is expanded, *Then* each option button has computed height >= 44px. *Given* an action toast, fake timers and a hover (and, separately, focus) on the toast, *When* the toast TTL elapses, *Then* the toast is still present; *When* the pointer leaves and the TTL elapses, *Then* it is dismissed and its notification item is still listed in the panel (UX-AC47).
- The focus indicator is never hidden behind a sticky action bar (UX-AC54) and every action is operable by tap alone (UX-AC45).
  - *Given* a phone viewport with a sticky Submit bar, *When* Tab reaches the last option, *Then* its bounding box does not intersect the sticky bar; *When* the test uses `tap()` only, *Then* every action completes.
- WCAG 2.2 and device sweep (UX-AC81 to UX-AC88): 320 px reflow, text-spacing overrides, 24 px inline-target rule, exactly one live region (countdowns and the delivery line are not live), heading levels with `tabindex="-1"` focus fallbacks, `prefers-reduced-motion` and `forced-colors`, the mobile keyboard matrix, and shortcuts inert in inputs, the terminal, with modifiers and during IME composition, and the single-character shortcuts (J/K, digits) bound only while focus is inside the panel list (WCAG 2.1.4).
  - *Given* the panel rendered at 320 px, *When* `document.documentElement.scrollWidth` is read, *Then* it equals the viewport width; *Given* the 1.4.12 override stylesheet injected, *Then* no element in the panel has `scrollHeight > clientHeight` with hidden overflow; *Given* the panel with a rate-limit countdown and a delivery line, *Then* `querySelectorAll('[aria-live],[role=status],[role=alert]')` has length 1; *Given* focus inside a `textarea` or the xterm container, *When* `1`, `j` or Escape are pressed, *Then* no panel handler runs; *Given* `isComposing` true or Ctrl held, *Then* none runs.
- Mobile presentation and Back (UX-AC74, UX-AC75): the mobile panel is the Alerts page with one history entry; Back closes it with drafts kept; no `role="dialog"`/`aria-modal`/bottom sheet anywhere.
  - *Given* a 390 px touch viewport, *When* the bell is tapped and `history.back()` is called, *Then* the page is closed, the previous route is shown, one entry was added and removed, and a typed draft is restored on reopening; *Given* any open surface, *Then* `[role=dialog],[aria-modal=true]` matches nothing.
**Files**: `ActionRow.tsx`, `ActionRow.css.ts`, `NotificationPanel.tsx`, `web-app/src/components/ui/AlertsPage.tsx`, `web-app/src/lib/hooks/usePanelHistory.ts`

##### Task 2.6a: Keyboard model, focus management, live region (S)
##### Task 2.6b: Mobile layout and touch targets; single-scroll-region behavior (no sheet); consume `OfflineBanner` (Task 1.6b) so action rows are `aria-disabled` with reason "Offline" (S)
##### Task 2.6c: WCAG 2.2 sweep: reflow, text spacing, target size, single live region, heading levels and focus fallbacks, reduced motion, forced colors, axe checks (M)
- Files: `web-app/src/components/ui/notification-actions/ActionRow.tsx`, `web-app/src/components/ui/notification-actions/ActionRow.css.ts`, `web-app/src/components/ui/NotificationPanel.tsx`, `web-app/src/components/ui/notification-actions/ActionRow.a11y.test.tsx` (new)
##### Task 2.6d: Mobile Alerts page, `usePanelHistory` (one entry, Back closes, deep link replace), presentation rule test (S)
##### Task 2.6e: Keyboard matrix: input attributes per field, `visualViewport` sticky bar, shortcut guards (inputs, terminal, modifiers, IME) (S)

### Story 2.7: Resolution sync and dedup
**As an** operator, **I want** an answered item retired everywhere, **so that** I do not see a stale card in another tab or channel.
**Acceptance Criteria**:
- Answering in one tab marks the record resolved and the other tab shows "Answered elsewhere: <value>" via id-set sync (never "clear everything").
  - *Given* two tabs open on the same notification, *When* tab A answers question `q1`, *Then* tab B shows "Answered elsewhere: Redis" within the next sync without focus stealing.
- Two distinct questions from one session are both actionable (dedup by `question_id`; collapse only identical text+labels).
  - *Given* session `s1` raises `q1` ("Cache?") and `q2` ("Region?") within 5 s, *When* history is served, *Then* two records with two `ANSWER_QUESTION` descriptors exist.
**Files**: `server/notifications/resolved_item_reaper.go`, `server/notifications/store.go`, `web-app/src/lib/hooks/useNotificationSync.ts` (locate existing cross-tab module first)

##### Task 2.7a: Event-driven resolve path in the notification store keyed on `question_id`/`approval_id`: the dispatcher publishes a resolve event on answer success and the store retires the history row. This is new design, not the existing `ReapResolvedItemNotifications` pass, which is polled, item-scoped and keyed on `metadata["item_id"]` with an `ItemStateLookup` (`resolved_item_reaper.go:28-62`); sized M+ (M)
- Files: `server/notifications/store.go`, `server/notifications/resolved_item_reaper.go`, `server/services/notification_action_service.go`, `server/notifications/store_test.go` 
##### Task 2.7c: BroadcastChannel id-set payload for cross-tab "Answered elsewhere" (S)
##### Task 2.7b: Dedup by `question_id` in the store collapse (S)

### Story 2.8: Risk tiers on the remote listener
**As a** phone user, **I want** Tier A actions available remotely and Tier B clearly marked desktop-only, **so that** nothing silently fails.
**Acceptance Criteria**:
- On `:8444`, Tier A succeeds; Tier B descriptors carry `available=false, reason="desktop_only"` and `InvokeNotificationAction` returns `FORBIDDEN` for them.
  - *Given* a request through the remote chain invoking `ANSWER_QUESTION` route `live_keystroke`, *When* processed, *Then* the outcome is `FORBIDDEN` and no pane write occurs.
  - *Given* the same chain invoking `BACKLOG_DECISION`, *When* processed, *Then* it is `APPLIED`.
  - *Given* the same chain invoking `ANSWER_GUIDANCE`, `CANCEL_WAKE` and `CANCEL_PEER_HELP` (all Tier A, ADR-004), *When* processed, *Then* each is `APPLIED` (or `ALREADY_RESOLVED`) and none writes to a pane.
- The remote chain is proven by a Go integration test through the real remote handler chain (`server/remote_listener_actions_integration_test.go`), not by Playwright: the e2e server has no `--remote-port`. Playwright only asserts descriptor rendering with fulfilled `available=false, reason="desktop_only"` descriptors.
- Every `ActionKind` (and the controls Remind me and Resume scheduled prompts) has exactly one tier and one remote-listener availability, in one table that the descriptor builder, the dispatcher and `design/ux.md` 2.4 share; a test fails when a new `ActionKind` is added without a row (UX-AC77).
  - *Given* the closed `ActionKind` set, *When* `TestEveryActionKindHasTierAndRemoteAvailability` runs, *Then* it passes, and it fails for a fixture enum value without a table row; *Given* the remote chain, *When* Resume scheduled prompts is requested, *Then* the control is `aria-disabled` with "Resume from desktop" and the flag is unchanged.
- UI shows "Answer from desktop" with Open session; never an unexplained disabled button.
  - *Given* a descriptor with `available=false, reason="desktop_only"`, *When* rendered, *Then* the button is `aria-disabled` and its description reads "Answer from desktop", the Open session button beside it is enabled, focusable and navigates, and a Tier A `APPROVE` descriptor on the same listener renders an enabled working button (UX-AC13).
**Files**: `server/services/notification_action_service.go`, `server/services/hidden_question_reply.go` (`EvaluateLocalWrite` callers), `ActionRow.tsx`

##### Task 2.8a: Tier policy in the dispatcher using the listener identity; scope `channel:remote:tierB` check (M)
- Files: `server/services/notification_action_service.go`, `server/services/notification_actions.go`, `server/services/local_write_verdict.go` (caller only), `server/remote_listener_actions_integration_test.go`
##### Task 2.8b: UI copy and disabled-with-reason rendering (S)
##### Task 2.8c: Single tier/availability table, completeness test and the Resume-from-desktop rule (S)

### Story 2.9: End-to-end specs for inline actions
**As a** maintainer, **I want** specs per action type, **so that** the success metric (1 interaction) is measured.
**Acceptance Criteria**:
- One spec each: approve, deny, backlog decision, guidance answer; each resolves with exactly one click after the pane opens.
  - *Given* a seeded pending approval, *When* the test opens the pane and clicks Approve once, *Then* the row shows "Approved" and the approval list is empty.
- Stale/answered-elsewhere path covered.
  - *Given* a question answered via the RPC before the click, *When* the user clicks an option, *Then* the row shows "Already handled" and no alert.
- Flipping a flag off at runtime degrades the surface without reload, and an action in flight straddling the flip completes or falls back to Open (UX-AC57).
  - *Given* a delayed `InvokeNotificationAction` response and `notification_inline_actions` flipped off while it is pending, *When* the response arrives, *Then* the row shows either the applied result or "Open session" with no error toast and no reload, and the next render shows legacy Approve/Deny.
- Action round trip on the isolated server: p95 < 1 s (REQ-19) over 20 invocations of each of approve and backlog decision, measured in the page between click and the row's resolved state (annotated soft on CI).
  - *Given* 20 seeded approvals, *When* each is approved once, *Then* the measured p95 is under 1000 ms.
**Files**: `tests/e2e/notification-inline-actions.spec.ts`, `tests/e2e/pages/NotificationPanel.ts`

##### Task 2.9a: Approve/deny/backlog-decision specs (M)
- Files: `tests/e2e/notification-inline-actions.spec.ts`, `tests/e2e/pages/NotificationPanel.ts`, `tests/e2e/helpers/test-server.ts`
##### Task 2.9b: Guidance answer and stale/answered-elsewhere specs; touch project with `hasTouch` for phone case (M)
- Files: `tests/e2e/notification-inline-actions.spec.ts`, `tests/e2e/pages/NotificationPanel.ts`, `playwright.config.ts`
##### Task 2.9c: Round-trip latency measurement spec (p95 < 1 s, soft on CI) (XS)

### Story 2.10: Interaction safety (engaged-card freeze, shortcut guards, commit delay)
**As an** operator, **I want** the list to hold still while I act and the three consequential taps to be undoable, **so that** one interaction cannot become a wrong irreversible one (triad blockers B1 and B2; UX-AC60 to UX-AC63).
**Acceptance Criteria**:
- Engaged-card freeze: while a card is engaged no Needs-you row is inserted, removed or reordered; arrivals queue behind a sticky "N new" button; a resolved row keeps its final state until nothing is engaged or the panel closes (UX-AC60).
  - *Given* card A focused and a poll that adds item B and resolves item C, *When* the poll completes, *Then* the row order is unchanged, C shows "Approved" in place, a button "1 new" exists, and after focus leaves A and 750 ms pass the list merges and the polite region says "1 new item" once.
- Resolved-row lifecycle and focus advance follow the one table in `design/ux.md` 7.1: programmatic focus, including the advance to the next item, never engages a card, so it does not re-freeze the list.
  - *Given* approval A resolved by keyboard and card B next in the list, *When* 750 ms pass and A leaves, *Then* focus is on B's header, B is not engaged (a poll that now adds item C inserts it at once with no "N new"), and `1` pressed within 750 ms does nothing.
  - *Given* keyboard focus on a sent answer's option button and a late acknowledgement that retires the row, *When* the row leaves, *Then* the single polite announcement reads "Answer confirmed, moved to Recent activity" and focus is on the next item header; *Given* focus elsewhere, *Then* the late acknowledgement is silent (D6, `design/ux.md` 7.1 and 11.1).
- Digit shortcuts act only on a digit-armed card (the `design/ux.md` 7.1 definition), are inert for 750 ms after any programmatic focus move or list change (a digit's own focus move is not one: it neither restarts the 750 ms nor disarms the card, and it counts as operator-placed focus so the card stays engaged), and only move focus (UX-AC61).
  - *Given* an approval resolves, the row leaves after 750 ms and focus advances to the next question card, *When* `1` is pressed 100 ms after the advance, *Then* nothing happens and no `InvokeNotificationAction` call is made; *Given* the operator pressed Enter to open the card and 750 ms passed, *When* `1`, `2` and `3` are pressed 50 ms apart, *Then* focus visits option 1, 2, 3 in order, the card is still engaged (a poll adding item B queues behind "N new") and nothing is sent; *When* Enter is pressed on option 2, *Then* exactly one invocation is made. The pre-expanded first item on panel open ignores digits until the operator Tabs onto or clicks it.
- Operator-expanded cards stay engaged until collapsed (`design/ux.md` 7.1 (f)), because iOS Safari does not focus a button on tap.
  - *Given* a touch device (no focus on tap) and a card expanded by tapping its header 10 s ago with no other interaction, *When* a poll adds item B, *Then* B queues behind "1 new" and the rows do not move; *When* the card is collapsed and 750 ms pass, *Then* B merges.
- Settled layout: a control that was just rendered or moved ignores activation for 300 ms (UX-AC62).
  - *Given* a control mounted at t0, *When* it is activated at t0+100 ms and again at t0+400 ms (fake timers), *Then* only the second activation invokes.
- Commit delay with Undo for Approve, Send back and Mark done only (UX-AC63): a tap makes the pressed button `aria-disabled` "Approving in 3s", moves focus to Undo and shows Undo; Undo sends nothing; **leaving cancels, never commits**: closing the panel, a click outside the desktop popover, a route change, Back, `visibilitychange` to hidden, `pagehide` or going offline during the hold sends nothing and restores the card with "Approval cancelled when you left. Tap Approve to try again.", and the restored card still counts in the bell badge; Deny, answers and Merge have no hold; the countdown is not a live region.
  - *Given* Approve tapped at t0, *Then* the button is `aria-disabled` and focus is on Undo; *When* Undo is activated at t0+2 s, *Then* no `InvokeNotificationAction` call was made and the row is back to its prior state; *Given* Approve tapped and the panel closed (separately: `visibilitychange` to hidden, `pagehide`) at t0+1 s, *Then* no call is ever made, on return the card shows the cancellation line, `badgeCount` still includes the item and the polite region said "Approval undone" once (the same holds for a click outside the popover and for `offline`); *Given* 3 s elapse with the panel open, *Then* one call is made and the p95 timer starts at dispatch; *Given* Deny tapped, *Then* the call is immediate.
- Layout stability and the fling guard (`design/ux.md` 7.1): the connectivity banner, the "N new" pill, the failure strip and the "As of" line use one reserved status slot, the scroll region sets `overflow-anchor: auto`, and a touch that begins within 120 ms of a scroll event activates nothing.
  - *Given* a failure strip, an "N new" pill and an Offline banner appearing in turn, *When* each renders, *Then* no list row's `getBoundingClientRect().top` changes; *Given* a `scroll` event at t0 and a touch on Approve at t0+100 ms, *Then* nothing is invoked and the click is swallowed; *Given* a touch at t0+200 ms, *Then* it activates.
**Files**: `web-app/src/lib/hooks/useEngagedCard.ts` (new), `web-app/src/lib/hooks/useFrozenList.ts` (new), `web-app/src/lib/hooks/useCommitDelay.ts` (new), `web-app/src/lib/hooks/useScrollSettle.ts` (new), `ActionRow.tsx`, `NotificationPanel.tsx`, `ActionRow.test.tsx`

##### Task 2.10a: `useEngagedCard` + `useFrozenList` and the "N new" merge/announce (M)
- Files: `web-app/src/lib/hooks/useEngagedCard.ts`, `web-app/src/lib/hooks/useFrozenList.ts`, `web-app/src/components/ui/NotificationPanel.tsx`, `web-app/src/components/ui/notification-actions/ActionRow.tsx`
##### Task 2.10b: Digit/focus guards (programmatic focus never engages, 750 ms grace not restarted by a digit's own focus move, operator-expanded cards engaged until collapsed, pre-expanded first item inert, focus-only digits) and the 300 ms settled-layout window (S)
##### Task 2.10c: `useCommitDelay` with Undo, focus to Undo, cancel (never flush) on panel close, outside click, route change, Back, `visibilitychange`, `pagehide` and `offline` with the restore note, the badge still counting the card and the "Approval undone" announcement, non-live countdown; wire Approve, Send back, Mark done (S)
##### Task 2.10d: Reserved status slot, `overflow-anchor`, the 120 ms scroll-fling guard (`useScrollSettle`) and the no-row-movement test (S)

---

# Epic 3: Structured AskUserQuestion (flag `askuserquestion_structured`)

**Goal**: A live `AskUserQuestion` becomes a durable `GuidanceRequest` with its full payload, rendered with options in the pane, answered through the ADR-005 ladder, with explicit delivery state.

### Story 3.1: Extend `GuidanceRequest`
**As a** developer, **I want** the additive entity fields and domain types, **so that** questions and peer help share one store.
**Acceptance Criteria**:
- Existing rows survive migration with defaults; new columns readable.
  - *Given* a database with an existing `GuidanceRequest` row (`scope=backlog-item`, `question_type=yes-no`), *When* the new schema is auto-migrated, *Then* `GetGuidanceRequest` returns it with `source="agent_mcp"`, `answerer_kind="human"`, `multi_select=false`.
- Hook-source create dedups on `(asker_session_uuid, hook_call_id)` and bypasses the existing `(scope, scope_key, question_text)` open-row coalesce (`session/ent_repository_guidance.go:231` `upsertGuidanceRequest`; index at `session/ent/schema/guidance_request.go:105`): N concurrent creates of the same call yield exactly 1 row (`-race`), and two calls with identical text but different `hook_call_id` yield 2 rows.
  - *Given* 8 goroutines calling create with the same `asker_session_uuid="7c1e..."`, `hook_call_id="toolu_01"`, *When* all return, *Then* the table holds 1 row and all callers receive its id.
  - *Given* two hook calls in one session both with `question_text="Which cache?"` and `hook_call_id` `toolu_01` / `toolu_02`, *When* both are created, *Then* two open rows exist (the legacy coalesce did not merge them).
- The pending cap (`DefaultGuidanceRequestPendingCap = 4`, `ent_repository_guidance.go:18-25`) is counted per `(scope, scope_key, source class)`; hook and peer rows cannot exhaust agent guidance slots or each other; dedup hits never raise `PENDING_CAP_EXCEEDED`.
  - *Given* a session with 4 open `agent_mcp` rows, *When* a hook-source question is mirrored, *Then* it is created (separate class); *Given* the hook class at its cap, *When* the identical hook question is re-registered, *Then* the existing row is returned and no `PENDING_CAP_EXCEEDED` is raised.
  - *Reverse direction (round 2)*: *Given* a session with 4 open hook-source rows (and, separately, 4 open peer rows), *When* an `agent_mcp` guidance request (`report_blocked` / `create_guidance_request`, scope=session) is created, *Then* it is created and no `PENDING_CAP_EXCEEDED` is raised; *Given* 4 open `agent_mcp` rows, *When* a 5th `agent_mcp` row is created, *Then* `PENDING_CAP_EXCEEDED` is still raised (legacy behavior preserved). The current `enforceGuidanceRequestPendingCap` (`ent_repository_guidance.go:272-292`) has no `source` predicate, so this requires the code change in Task 3.1e.
- Peer-help rows use `scope=session`, `scope_key=<asker session UUID>`, `source=peer`, `expires_at` 24 h (ADR-003 rule 1).
  - *Given* `request_help` from A to B, *When* the row is created, *Then* `scope_key == A`, `source == "peer"`, `answerer_session_uuid == B`.
- Illegal field combinations are unrepresentable at the repository boundary via `NewGuidanceRequest`, and `DeliveryState` moves only along the guarded transition table.
  - *Given* a create with `source=peer` and `answerer_kind=human`, or `delivery_route=read_only` with `delivery_state=sent`, *When* constructed, *Then* it returns a typed error and nothing is written.
**Files**: `session/ent/schema/guidance_request.go`, `session/domain/guidance_request_source.go` (new; `session/domain/guidance_request.go` does not exist, existing request types are in `session/domain/backlog.go:307,330`), `session/ent_repository_guidance.go`, `proto/session/v1/guidance_request.proto`

##### Task 3.1a: Ent schema additive fields with defaults; regenerate with `--feature sql/upsert`; `go build ./...` (S)
##### Task 3.1b: Domain sum types `QuestionSource`, `AnswererKind`, `DeliveryRoute`, `DeliveryState` with `IsValid()`; `QuestionPayload` value object with 1-4 question validation, option label length and count rules (S)
##### Task 3.1c: Repository: transactional create/dedup for hook and peer sources keyed on `hook_call_id`, `SetDelivery(route,state,attempts)` guarded transitions, queries by `asker_session_uuid`, `CancelGuidanceRequest(reason)` callers wired in 3.3 (M)
- Files: `session/ent_repository_guidance.go`, `session/ent_repository_guidance_test.go`, `session/domain/guidance_request_source.go`
##### Task 3.1d: Proto fields on `GuidanceRequest`, `make proto-gen`, round-trip test (S)
##### Task 3.1e: `NewGuidanceRequest` constructor with cross-field rules and the `DeliveryState` transition table; per-source-class cap counting (add a `source` class argument and predicate to `enforceGuidanceRequestPendingCap`; legacy rows default to `agent_mcp`) with tests in both directions, and the non-`agent_mcp` coalescing bypass in `upsertGuidanceRequest` (M)
- Files: `session/domain/guidance_request_source.go`, `session/domain/guidance_request_source_test.go`, `session/ent_repository_guidance.go`, `session/ent_repository_guidance_test.go`

### Story 3.2: Parser and fixtures from the real payload
**As a** developer, **I want** `ParseAskUserQuestion` to cover the official schema, **so that** multi-select and multi-question are represented.
**Acceptance Criteria**:
- Fixtures from S1 parse: single-select -> replyable shape; multi-select and multi-question -> `QuestionPayload` with correct counts, still marked non-keystroke-replyable until Story 3.5.
  - *Given* fixture `multi_two_questions.json`, *When* `ParseAskUserQuestion` runs, *Then* the payload has 2 questions, question 2 `MultiSelect=true`, labels preserved verbatim, and `Replyable=false` until stepwise answering is enabled.
- Hostile text is sanitized (`sanitizeNotificationText`); option labels > 80 runes or > 7 options -> `unknown` shape.
  - *Given* a label of 200 characters, *When* parsed, *Then* shape is `unknown` and the question text is still available for display.
**Files**: `server/services/pending_question_store.go`, `server/services/askuserquestion_parser.go` (extract), `server/services/askuserquestion_parser_test.go`, `docs/tasks/askuserquestion-ui.md`

##### Task 3.2a: Rewrite parser to produce `QuestionPayload`; fixture-driven tests (M)
- Files: `server/services/askuserquestion_parser.go`, `server/services/askuserquestion_parser_test.go`, `server/services/pending_question_store.go`, `server/services/testdata/askuserquestion/`
##### Task 3.2b: Correct the stale `docs/tasks/askuserquestion-ui.md` payload description (XS)

### Story 3.3: Hook mirror, durability and lifecycle
**As an** operator, **I want** a live question to survive a service restart and show up everywhere, **so that** it stops degrading to "Answer in the terminal".
**Acceptance Criteria**:
- Registering a verified-proof question mirrors to `GuidanceRequest(scope=session, source=ask_user_question_hook)` and the notification metadata carries `guidance_request_id` and `question_id`.
  - *Given* a hook payload with a verified HMAC proof for session UUID `7c1e...`, *When* `ApprovalHandler` registers it, *Then* one guidance row exists with `source="ask_user_question_hook"` and the INPUT_REQUIRED metadata includes its id.
- Unverified attribution (`cwd`/`unknown`) mirrors as view-only (`delivery_route=read_only`), never replyable.
  - *Given* attribution came from cwd prefix, *When* mirrored, *Then* the row's route is `read_only`.
- After restart, a pending row is re-hydrated and any write re-verifies `DialogMatch` first; a mismatch -> `STALE`.
  - *Given* a pending row and a restarted service, *When* `ANSWER_QUESTION` is invoked while the pane shows a different dialog, *Then* the outcome is `STALE` and zero bytes are written.
- Hook-source and peer rows for archived/stopped/deleted sessions are cancelled by a new `GuidanceSessionReaper`, not by extending the backlog lifecycle (verified: `CancelGuidanceRequest` has no production caller today, `session/backlog_lifecycle.go` has no guidance references, and `selfHealStuck` at `backlog_lifecycle.go:2145` is backlog-item scoped). The reaper is **limited to `source=ask_user_question_hook` and `source=peer` rows**: the dialog or peer context dies with the session. It never cancels `source=agent_mcp` session-scope guidance (created by `server/mcp/tools_guidance.go`); those rows follow Story 3.4 (stopped session -> `saved_for_later`; archived -> answer stored, "will not be delivered unless you resume"), so the ACs of 3.3 and 3.4 do not overlap on any row. The reaper subscribes to `events.EventSessionDeleted`/`EventSessionArchived` (precedent: `server/review_queue_manager.go:277`), resolves `event.SessionID` (UUID vs title; see `resolveQueueKey`, `review_queue_manager.go:283`) to a session UUID before matching `session_uuid`/`asker_session_uuid`, is invoked beside the `PendingQuestionStore.SessionDeleted` seam (`server/services/hidden_question_reply.go:241`), and runs one startup pass for open hook/peer rows whose session is archived, stopped or missing. It calls `CancelGuidanceRequest`.
  - *Given* a pending hook-source row whose session is then archived, *When* the archive event is published, *Then* `cancelled_at` is set and the card reads "Session ended before this was answered".
  - *Given* a pending row and a service restart while its session was deleted during downtime, *When* the startup pass runs, *Then* the row is cancelled with the same reason.
  - *Given* an open `agent_mcp` session-scope guidance row and its session is then stopped (and, separately, archived), *When* the reaper runs, *Then* the row is not cancelled and Story 3.4's `saved_for_later` / stored-undelivered states apply.
  - *Given* an event whose `SessionID` is the session title, *When* the reaper handles it, *Then* it resolves to the UUID and cancels that session's hook rows only.
- A hook-source row is cleared when the dialog is resolved by any route (ADR-003 rule 1 "Mirror clearing"), so terminal-answered questions never fill the cap.
  - *Given* 5 `AskUserQuestion` calls each answered directly in the terminal (PostToolUse or dialog-gone observed), *When* a 6th question is raised, *Then* the 6th is mirrored (no `PENDING_CAP_EXCEEDED`) and the first 5 rows are `cancelled` with reason `answered_in_terminal` or `dialog_gone`, `guidance_mirror_cleared_total` incremented.
- The open card reacts to a terminal answer (UX-AC26): when a mirror is cleared with reason `answered_in_terminal` or `dialog_gone` while its card is open, the card flips to read-only "Already handled in the session. No action needed." with one polite announcement and no focus movement.
  - *Given* an expanded card with focus on an option and a mirror clear with reason `answered_in_terminal`, *When* the live update arrives, *Then* the card shows the read-only copy, `document.activeElement` is unchanged, and the polite region text changed exactly once.
- The "dialog gone on two samples" fallback has an owner, the `DialogGoneSampler` (Task 3.3f), so the 6th-question AC passes whether or not S7 (6) finds a PostToolUse matcher for `AskUserQuestion`. It is the one deliberate exception to "zero new poll loops" (a bounded server-side goroutine, joined on shutdown): it samples only sessions with an open hook-source row (hidden and idle sessions included), every 3 s per row for the first minute then every 30 s, at most 20 rows per pass. After 10 minutes it **stops polling that row; the row stays open and answerable**, and nothing about its delivery state changes. Answerability never depends on the sampler: a row leaves the answerable state only through a failed `DialogMatch` at answer time (`STALE`), a confirmed dialog-gone, a terminal answer or the session ending. `stale` therefore never means "time passed" and its copy ("Already handled in the session") is only shown after a confirmed mismatch (UX-AC59).
  - *Given* an open hook row whose pane shows neither a `DialogMatch` for it nor any dialog (`session.PaneShowsDialog` returns `Absent`, Task 2.4c; `Showing` with a different dialog is the `dialog_replaced` case and `Indeterminate` counts as neither) on two consecutive fake-clock samples 3 s apart, *When* the sampler runs, *Then* the row is cancelled with reason `dialog_gone`; *Given* the same samples but the pane shows a different dialog, *Then* it is cancelled with reason `dialog_replaced` and the card reads "The session is now asking something else. Open session to answer it."; *Given* a single non-matching sample followed by a matching one, *Then* the row stays open.
  - *Given* a row open for 10 minutes with the dialog still matching, *When* the sampler's bound is reached, *Then* sampling of that row stops, the row's `delivery_state` is unchanged and its descriptors still include `ANSWER_QUESTION`.
  - *Given* a row open 45 minutes (past the 30-minute `PendingQuestionStore` TTL, rehydrated from the durable row) with the fixture dialog still on screen, *When* `ANSWER_QUESTION` option 1 is invoked, *Then* `DialogMatch` re-verifies, exactly one byte `1` is written, and the outcome is `APPLIED`; *Given* the dialog was meanwhile answered in the terminal, *Then* the outcome is `STALE` with zero writes.
- A hook-class create that would hit the pending cap first re-verifies that session's open hook rows once, on demand (one capture per row, not a poll), and cancels those whose dialog is gone (`dialog_gone`), so rows the sampler stopped watching cannot starve the cap.
  - *Given* 4 open hook rows older than 10 minutes of which 3 have no dialog on screen, *When* a 5th question is mirrored, *Then* the 3 are cancelled with `dialog_gone`, the 5th is created and no `PENDING_CAP_EXCEEDED` is raised; *Given* all 4 dialogs still match, *Then* the create is refused as today.
- A hook payload with no `hook_call_id` falls back to the canonical payload hash and may coalesce two distinct identical questions into one row; this is an accepted limitation (ADR-003 rule 1), pinned by a test that documents the behavior.
  - *Given* two hook payloads with identical content and no `hook_call_id`, *When* both are mirrored, *Then* one row exists.
- Per-source answer side effects: answering a hook-source row from the pane does not emit the second "Your guidance request was answered" notification (`notifyAnswered`, `server/services/guidance_request_service.go:68-82`); `resumeTriageIfHalted` does not fire for session scope.
  - *Given* a hook-source row answered through `InvokeNotificationAction`, *When* the answer completes, *Then* exactly zero `notifyAnswered` notifications are created and the pane card shows the answer.
- The hook-path mirror fails open (pre-mortem #7): the mirror write runs after the hook response has been decided (the native dialog is already deferred), with a 250 ms bound; a locked SQLite, a slow repository or an error is logged, counted in `guidance_mirror_failed_total{reason}`, and never changes or delays the hook's reply, so the dialog always appears.
  - *Given* a mirror repository fake that blocks for 5 s (and, separately, returns an error), *When* an `AskUserQuestion` hook arrives, *Then* the hook handler returns the defer decision in under 300 ms, the dialog is not suppressed, and `guidance_mirror_failed_total` is incremented once; the question is then answerable only after a later successful re-registration, or via Open session.
- Layout-drift canary (pre-mortem #5): every `DialogMatch` outcome is counted in `question_dialog_match_total{result,claude_version}`; when more than 50% of at least 5 attempts in an hour are mismatches across sessions the server raises one operator notification and the Tier B `ANSWER_QUESTION` descriptors of hook-mirrored rows degrade to **`read_only`** (question, options and Open session, no answer control) until the share recovers. There is no durable-record route for a hook-mirrored row: `saved_for_later` never applies to it and the reaper cancels it with its session, so the earlier "degrades to the durable-record route (Tier A)" had no target. Exception: if `hook_reply` is enabled (S1 PASS, `stage:hook_reply`), that route is not keystroke-based and keeps answering. Agent-created (`agent_mcp`) rows already use a durable route and are unaffected; the pinned fixture version (`v2.1.296`) is recorded next to the fixtures, and a fixture-vs-live comparison is part of the Task 5.3 checklist after any Claude Code update.
  - *Given* a fake clock and 6 `DialogMatch` attempts in 1 h of which 4 mismatch, *When* the canary evaluates, *Then* exactly one notification is raised, a hook-mirrored row's descriptor has route `read_only` with no `ANSWER_QUESTION` control and an agent-created row's descriptor is unchanged; *Given* `hook_reply` enabled, *Then* the hook row's route stays `hook_reply`; *Given* the share drops below 50% over the next 5 attempts, *Then* the descriptor returns to `live_keystroke`.
- `questions_json` and option text are sanitized with `sanitizeNotificationText` and scanned with the existing secret scanner at write time; a hit stores the row with the matching span replaced by `[redacted]`, and terminal rows (answered, cancelled) are removed from `questions_json` after 30 days (pre-mortem #18).
  - *Given* a payload whose option description contains a string matching the secret scanner, *When* mirrored, *Then* `questions_json` holds `[redacted]` in its place and the notification snippet does too.
**Files**: `server/services/approval_handler.go`, `server/services/approval_question.go`, `server/services/guidance_request_hook_mirror.go`, `server/services/guidance_session_reaper.go` (new), `server/services/guidance_dialog_gone_sampler.go` (new), `server/services/guidance_request_service.go` (per-source side effects)

##### Task 3.3a: Mirror on register (idempotent), metadata fields, attribution-based route (M)
- Files: `server/services/approval_handler.go` (one call), `server/services/approval_question.go`, `server/services/guidance_request_hook_mirror.go`, `server/services/guidance_request_hook_mirror_test.go`
##### Task 3.3b: Re-hydrate `PendingQuestionStore` from durable rows on start; DialogMatch re-verification before any write (M)
- Files: `server/services/pending_question_store.go`, `server/services/guidance_request_hook_mirror.go`, `server/services/pane_reply_core.go`, `server/services/pending_question_store_test.go`
##### Task 3.3c: New `GuidanceSessionReaper` (event subscription at the session-deleted/archived seam plus startup pass; `source IN (ask_user_question_hook, peer)` only; resolves event `SessionID` to UUID) calling `CancelGuidanceRequest`; do not touch `backlog_lifecycle.go` (M)
- Files: `server/services/guidance_session_reaper.go`, `server/services/guidance_session_reaper_test.go`, `server/services/hidden_question_reply.go` (the `SessionDeleted` seam call only), `server/dependencies.go` (one wiring line)
##### Task 3.3d: Mirror clearing on terminal-answered/dialog-gone/dialog-replaced, the open-card flip to read-only (UX-AC26) and the "5 terminal-answered then a 6th mirrors" test (M)
- Files: `server/services/guidance_request_hook_mirror.go`, `server/services/guidance_request_service.go`, `web-app/src/components/guidance/GuidanceQuestionForm.tsx`, `server/services/guidance_request_hook_mirror_test.go`
##### Task 3.3e: Per-source `notifyAnswered` behavior (suppress for hook source) with test (S)
##### Task 3.3f: `DialogGoneSampler` (who polls: a server-side goroutine joined on shutdown; cadence 3 s per row for the first minute then 30 s; bound 20 rows per pass; stops polling a row after 10 min without changing its state or answerability; two-consecutive-sample rule via `DialogMatch` and `session.PaneShowsDialog` from Task 2.4c; fake-clock tests; cancels with `dialog_gone`) (M)
- Files: `server/services/guidance_dialog_gone_sampler.go`, `server/services/guidance_dialog_gone_sampler_test.go`, `server/dependencies.go` (one wiring line)
##### Task 3.3g: On-demand re-verification of a session's open hook rows when a hook-class create would hit the cap, and the 45-minute answer test (S)
##### Task 3.3h: Layout-drift canary: `question_dialog_match_total`, the 50%-of-5 alert, route degradation to `read_only` for hook-mirrored rows (unless `hook_reply` is enabled; agent-created rows unaffected), fixture version note (S)
##### Task 3.3i: Fail-open hook-path mirror (post-decision, 250 ms bound, `guidance_mirror_failed_total`) with the blocking-repo and erroring-repo fault tests (S)
##### Task 3.3j: Sanitize and secret-scan `questions_json` at write; 30-day terminal-row purge (S)

### Story 3.4: `AnswerRouter` and delivery state machine
**As an** operator, **I want** my answer to reach the session by the best available route and to see its delivery state, **so that** I know the agent got it.
**Acceptance Criteria**:
- Live visible Claude session, single-select, dialog on screen -> `live_keystroke` via `PaneReplyCore`; state goes `pending -> sent -> acknowledged`.
  - *Given* visible session `s1` with fresh `DialogMatch` for question "Cache?" options [Redis, Memcached], *When* `ANSWER_QUESTION` with option 1 is invoked, *Then* exactly one byte `1` is written with no Enter, delivery state is `sent`, and after the PostToolUse hook for AskUserQuestion it becomes `acknowledged`.
- Stopped non-archived session -> `saved_for_later`; archived -> answer stored, state `saved_for_later` with the UX-AC23 copy "This session is archived. Your answer will be saved but not delivered unless you resume it." (no pane write); nothing is revived.
- Archived-session copy is visible before the operator acts (UX-AC27): a card for an archived session shows "This session is archived. Your answer will be saved but not delivered unless you resume it." as soon as it renders, and answering never starts, recovers or transitions the session.
  - *Given* an archived session with a pending row, *When* the card renders and before any click, *Then* the archived copy is in the DOM; *When* the operator answers, *Then* the session is still archived and the spy saw zero `Start`, `RecoverFromStopped`, `restartForRetry` and `transitionToLocked(..., Active)` calls.
- The `resume` route never revives (decided in triad round 2; ADR-005 rung 3): it only stores the answer as `saved_for_later` and never calls `Start`, `RecoverFromStopped`, `restartForRetry` or `transitionToLocked(..., Active)`, for stopped and archived sessions alike; the operator resumes through Open session. The file is still in the analyzer's `lifecycleFiles` as a ratchet (Task 4.3f).
  - *Given* a stopped, non-archived session with a pending `agent_mcp` row, *When* it is answered, *Then* the row is `saved_for_later` and the lifecycle spy recorded zero calls to any of the four functions.
  - Delivery of a saved answer is the agent's existing pull, not a push (decided; verified that `server/mcp/tools_guidance.go` already registers `get_guidance_request`, described as the way to check "after a respawn" whether the human has answered). The card copy is therefore "Your answer is saved. The agent can read it once you resume the session." (`design/ux.md` 3.1 row 8) and no task writes a stored answer into a resumed pane. *Given* a `saved_for_later` row and the operator then resuming the session through Open session, *When* the agent calls `get_guidance_request(id)`, *Then* it returns the answered state with the stored answer text and `answered_at`.
  - *Given* an archived session with a pending row, *When* answered, *Then* `answered_at` is set, the session remains archived and no `Start`/`RecoverFromStopped` is called (asserted by a spy).
- Acknowledgement signal and the Unconfirmed sub-state (triad blocker B3; ADR-005 amended; UX-AC24). `acknowledged` is set by the first of (a) a `PostToolUse` receipt for `AskUserQuestion` from that session after `sent_at` (primary; present only if S7 item (6) shows the matcher covers it, otherwise Task 3.4e adds it) and (b) two consecutive fresh captures 3 s apart showing neither a `DialogMatch` for this row's question nor `session.PaneShowsDialog` `Showing` (Task 2.4c; fallback; the only signal until (a) exists). At 60 s with no acknowledgement the server takes one more fresh capture under the write lease and follows the single decision table in `design/ux.md` 3.1: the same dialog still showing -> `delivery_failed(reason=still_showing)` (a digit **was** written, so the copy is row 16, not "Nothing was typed"; Retry sits behind "Retry anyway?", re-verifies `DialogMatch` and writes one digit); dialog gone -> `acknowledged` by (b); capture error or indeterminate layout -> the row stays `sent` with `unacknowledged=true` (the **Unconfirmed** chip). `SEND_INDETERMINATE` is known-uncertain: it sets `unacknowledged=true` and the row-6 copy and chip immediately at write time and counts in the badge at once (a 5 s test sees it counted). Only a plain `sent` row (row 2) under 60 s, without `SEND_INDETERMINATE`, is excluded from `badgeCount`. Rows 5, 14 and 16 also stay in Needs you and in the badge and never leave after 750 ms. An Unconfirmed row **stays in Needs you** and in the bell badge; it is never demoted to Recent activity by a timer, and leaves only on acknowledgement, the operator's browser-local "Mark as handled", or the session ending.
  - *Given* state `sent` at t0 and a capture that still shows the row's dialog, *When* 61 s of fake clock elapse, *Then* the state is `delivery_failed(still_showing)`, the line is row 16 ("Sent, but the question is still showing. Check the session before retrying.") and never row 5, Open session is primary, Retry appears only after "Retry anyway?" is confirmed and then re-verifies `DialogMatch` and writes exactly one byte; *Given* the same capture on a row that was `SEND_INDETERMINATE`, *Then* the result is the same row 16; *Given* a capture showing no dialog, *Then* the state is `acknowledged`; *Given* a capture error, *Then* the state stays `sent`, the descriptor flags `unacknowledged=true`, the item is still in the Needs-you set and counted in `badgeCount`; *Given* a PostToolUse receipt arrives later, *Then* the state becomes `acknowledged` and the item leaves Needs you; *Given* a `sent` row (and, separately, an Unconfirmed row and a row 16) whose session then ends, *Then* the line is row 17 "Sent, but the session ended before confirming it." (never "Session ended before this was answered."), the item moves to Recent activity and leaves the badge; *Given* a plain `sent` row 30 s old, *Then* `badgeCount` excludes it; *Given* a `SEND_INDETERMINATE` row 5 s old, *Then* it carries the Unconfirmed chip, `unacknowledged=true` and is counted in `badgeCount`.
  - *Given* an agent-created (`agent_mcp`) session-scope row on a stopped session that is answered, *Then* it is `saved_for_later`; *Given* a hook-mirrored row on the same stopped session, *Then* the `GuidanceSessionReaper` has cancelled it and the card reads "Session ended before this was answered." (`saved_for_later` never applies to `ask_user_question_hook` or `peer` rows).
- A visible-session `live_keystroke` answer is refused when the human is active in the pane (ADR-004 residual risk, pre-mortem #16). The signal is defined, not assumed: `HumanActivity` = the latest of (a) the time of the last browser-originated pane input recorded for that session in `paneWriter.SendInput` (`server/services/terminal_access.go:140`, the single sink of both web-terminal stream handlers) and (b) `#{client_activity}` of any attached tmux client with `#{client_control_mode}` = 0 (a human's own `tmux attach`; control-mode clients, which are always attached under ssq, are excluded because attachment alone is not activity). Within 5 s of that time -> `NOT_SENT` with reason `human_active`, zero bytes written. A failure to read (b) (tmux error) is treated as "no signal" for (b) only and logged; (a) always applies.
  - *Given* a visible session whose web terminal sent input 2 s ago (fake clock), *When* `ANSWER_QUESTION` is invoked, *Then* the outcome is `NOT_SENT(human_active)` and the fake pane received nothing; *Given* the last input 6 s ago, *Then* the answer proceeds.
  - *Given* a fake `list-clients` output with one control-mode client (`client_control_mode=1`, activity 1 s ago) and no other client, *When* invoked, *Then* the answer proceeds; *Given* an additional client with `client_control_mode=0` and `client_activity` 2 s ago, *Then* the outcome is `NOT_SENT(human_active)`; *Given* `list-clients` failing, *Then* the guard uses signal (a) only and the answer proceeds if (a) is quiet.
- `acknowledged` depends on Spike S7 check (6): if the injected PostToolUse matcher does not cover `AskUserQuestion`, Task 3.4e adds it; until then `acknowledged` is derived from dialog-gone only.
  - *Given* S7 (6) reported the matcher does not match `AskUserQuestion`, *When* Task 3.4e completes, *Then* the injected hook config contains an `AskUserQuestion` matcher and the receipt test passes.
- `answer_latency_ms` logged and `guidance_question_answer_latency_seconds` observed on answer.
  - *Given* created_at 10:00:00 and answered_at 10:00:07.5, *When* answered, *Then* the log field is 7500.
- Bounded-blocking hook fails open (pre-mortem #19; conditional on S1 PASS and the owner decision): `hook_block_ms` defaults to 0; any non-zero value is capped below the Spike S2 ceiling; a hung or unreachable server makes the hook exit empty at the cap so the native dialog appears, and the cap and fallback are recorded in ADR-005.
  - *Given* `hook_block_ms=2000` and a server that never answers, *When* the hook fires, *Then* it exits with empty output at <= 2 s plus a margin and the dialog appears; *Given* the default, *Then* the hook never blocks.
- Conditional-story rule for Task 3.4c: its tests (`HookReply_*`) are written and skipped with `t.Skip("blocked on S1: <link>")` until S1 passes, never deleted.
**Files**: `server/services/question_answer_router.go`, `server/services/question_answer_router_test.go`, `server/services/guidance_request_service.go`, `server/services/pane_reply_core.go`, `cmd/ssq-hooks/main.go`, `internal/claudehooks/claudehooks.go`, `server/services/hook_injector.go` (only if S7 (6) finds the matcher missing)

##### Task 3.4a: `AnswerRouter` route table + delivery state transitions (guarded UPDATEs) (M)
- Files: `server/services/question_answer_router.go`, `server/services/question_answer_router_test.go`, `server/services/guidance_request_service.go`, `session/ent_repository_guidance.go`
##### Task 3.4b: Visible-session `live_keystroke` via `PaneReplyCore` under `stage:answer`; amend ADR-010 status text (M)
- Files: `server/services/question_answer_router.go`, `server/services/pane_reply_core.go`, `server/services/hidden_question_reply.go`, the ADR-010 status line of `notification-tray-and-hidden-session-gate`
##### Task 3.4c: **Conditional on S1 PASS** - `hook_reply` for pre-answered rows (and bounded wait up to the S2 ceiling if the owner approves): `ssq-hooks` asks the server for an existing answer, returns allow + `updatedInput.answers`, else exits empty so the native dialog stays. On S1 FAIL close this task with the S1 result recorded (M)
- Files: `cmd/ssq-hooks/main.go`, `internal/claudehooks/claudehooks.go`, `server/services/question_answer_router.go`, `server/services/question_answer_router_test.go`
##### Task 3.4d: `resume`/`read_only` routes that only store the answer and never call `Start`/`RecoverFromStopped`/`restartForRetry`/`transitionToLocked`; spy tests for stopped and archived sessions, plus the `get_guidance_request` read-back test of the saved answer (`server/mcp/tools_guidance_test.go`) (M)
- Files: `server/services/question_answer_router.go`, `server/services/question_answer_router_test.go`, `server/mcp/tools_guidance_test.go`
##### Task 3.4e: Acknowledgement detection per the signal above ((a) PostToolUse receipt, adding the `AskUserQuestion` matcher if S7 (6) says it is missing; (b) two fresh-capture samples via `session.PaneShowsDialog`), the 60 s capture decision per the `design/ux.md` 3.1 table (`delivery_failed(still_showing)` / `acknowledged` / Unconfirmed), the `unacknowledged` flag and the row-17 session-ended case (M)
- Files: `server/services/guidance_ack_detector.go` (new), `server/services/guidance_ack_detector_test.go` (new), `server/services/hook_injector.go`, `internal/claudehooks/claudehooks.go`, `cmd/ssq-hooks/main.go`
##### Task 3.4f: `HumanActivity` recorder in `paneWriter.SendInput` (per-session last-input timestamp, injectable clock), the `list-clients` reader, and the `human_active` guard with the three tests above (S)
##### Task 3.4g: Unconfirmed state end to end: badge/Needs-you retention, browser-local Mark as handled, auto-clear on late acknowledgement, bounded `hook_block_ms` cap test (S)

### Story 3.5: Multi-select and multi-question stepwise answering
**As an** operator, **I want** to answer checkbox and multi-question prompts inline, **so that** I do not fall back to the terminal for those shapes.
**Acceptance Criteria** (**conditional on S1c PASS**; otherwise the story closes and shapes stay read-only):
- A 2-question dialog with a multi-select answers deterministically with a `DialogMatch` check before every step; any mismatch aborts with `STALE` and no further writes.
  - *Given* the recorded S1c fixture and answers {Q1: "Redis", Q2: ["A","C"]}, *When* invoked, *Then* the shared fault-injecting pane double extracted in Task 2.4a records the exact key sequence from the fixture and the final tool result contains both answers.
- Conditional-story rule: the `Sequence_*` tests and UX-AC29 specs are written and skipped with `t.Skip("blocked on S1c: <link>")` until S1c passes, never deleted.
  - *Given* the pane changes after step 1, *When* step 2's DialogMatch fails, *Then* the outcome is `STALE` with a recorded partial state `sent`-aborted and an operator-visible message.
- Exit path for an aborted multi-question answer (UX-AC29, `design/ux.md` 8.2.1): Submit is disabled, the card is pinned in Needs you with "Part of your answer was sent, then the prompt changed. Open session to finish.", and the only ways out are Open session or Dismiss with an inline confirm ("Dismiss without finishing?"); Retry is not offered because part of the answer was already typed.
  - *Given* an aborted partial answer, *When* the card renders, *Then* Submit is `aria-disabled`, Open session is enabled, the card is in Needs you, and Dismiss requires the confirm step.
**Files**: `session/pane_reply.go`, `server/services/pane_reply_core.go`, `server/services/pane_reply_sequence_test.go`

##### Task 3.5a: Sequence driver with per-step DialogMatch and lease; outcomes `NOT_SENT` vs `SEND_INDETERMINATE` per step (M)
- Files: `session/pane_reply.go`, `server/services/pane_reply_core.go`, `server/services/pane_reply_sequence.go` (new), `server/services/pane_reply_sequence_test.go`
##### Task 3.5b: Fault-injection tests per stage using the real capture fixtures (M)
- Files: `server/services/pane_reply_sequence_test.go`, `session/testdata/reply_dialogs/`, `server/services/pane_reply_testdouble_test.go`
##### Task 3.5c: Aborted-partial card state: pinned in Needs you, Submit disabled, Open session / confirmed Dismiss (S)

### Story 3.6: Non-Claude harness profiles
**As an** operator using another harness, **I want** options extracted and answerable where safe, **so that** the pane works beyond Claude.
**Acceptance Criteria**:
- A replyable harness has a `QuestionDialogProfile` with fixture, bottom-anchored matcher, forged-text negative test; others report `read_only` with the question text.
  - *Given* the OpenCode fixture and its forged variant, *When* the profile matches, *Then* the first yields options ["Allow once","Allow always","Reject"] and the second yields no match.
- Option extraction populates a `GuidanceRequest(source=ask_user_question_hook)` for harnesses whose profile is replyable, with the profile name in `questions_json.profile` (there is no `terminal_detect` source; ADR-003); unmeasured harnesses create no replyable row.
- Conditional-story rule: per-harness profile tests are written and skipped with `t.Skip("blocked on S5: <harness>")` until that harness is measured, never deleted.
  - *Given* Aider's `(Y)es/(N)o` prompt without a measured fixture, *When* detection fires, *Then* the notification has only `OPEN` and no `ANSWER_QUESTION`.
**Files**: `session/detection/question_profile.go`, `session/detection/binaries/{opencode,pi,gemini,agy,aider}.go`, `session/testdata/panes/<harness>/` (non-Claude captures; the Claude captures stay in `session/testdata/reply_dialogs/`)

##### Task 3.6a: `QuestionDialogProfile` interface (Strategy) + Claude profile wrapping existing `DialogMatch` (M)
- Files: `session/detection/question_profile.go`, `session/detection/question_profile_test.go`, `session/pane_reply.go`
##### Task 3.6b: Profiles + fixtures for each harness that S5 marks replyable; registry of replyable harnesses with a test that unmeasured harnesses are absent (M)
- Files: one `session/detection/binaries/<harness>.go` per replyable harness (at most five), `session/testdata/panes/<harness>/`, `session/detection/question_profile.go`
##### Task 3.6c: **Conditional on S5; last wave, optional-to-order (kept in scope, not cut)** harness-native answer channel (OpenCode plugin `client` / pi extension) as a route ahead of keystroke where S5 passed (M)
- Files: `server/services/question_answer_router.go`, `session/detection/question_profile.go`, one adapter file per passing harness under `session/detection/binaries/`

### Story 3.7: Question UI with delivery state
**As an** operator, **I want to** see the question, options, and what happened to my answer, **so that** I trust the result.
**Acceptance Criteria**:
- `GuidanceQuestionForm` is shared by `GuidanceRequestPanel` and `ActionRow`; it renders single-select as **option buttons** in a labelled `role="group"` (triad blocker B1: not a `radiogroup`, because arrow keys on radios change the selection and a keyboard or screen-reader user browsing options would send an irreversible answer), multi-select (checkboxes + Submit answers + Clear), multi-question (stacked groups + one Submit), >8 options as `<select>`, free-text "Other" with an explicit Send **only on routes that can carry text** (an `agent_mcp` durable-record row; a `hook_reply` row if S1 passes). It is never rendered for `live_keystroke` or hook-mirrored cards, whose write is a single digit; those cards show the options and Open session only (`design/ux.md` 3.1).
  - *Given* a payload with Q1 single (Redis, Memcached) and Q2 multiSelect (A, B, C), *When* rendered in the pane, *Then* Q1 is a `role="group"` labelled by its text containing two `<button>` options (no element with `role="radio"` or `role="radiogroup"`), Q2 has three `checkbox`es, and one "Submit answers" button.
  - *Given* the same single-select payload on a hook-mirrored `live_keystroke` row and, separately, on an `agent_mcp` row, *When* rendered, *Then* the first has no text input and no Send button, and the second has an "Other" field (16px) with an explicit Send.
- What sends and what only moves focus is fixed and tested (UX-AC18, UX-AC48, UX-AC61): click, tap, Enter or Space on a focused single-select option button sends once and locks the card; Tab, Shift+Tab, Up/Down arrows and digits 1-9 only move focus; a keyboard user can traverse every option with zero `InvokeNotificationAction` calls.
  - *Given* a single-select card with 3 options opened by the operator (Enter) 750 ms ago, *When* the test presses Tab, ArrowDown, ArrowUp and then `1`, `2`, `3` 50 ms apart, *Then* zero invocations are made, focus visited each button in order (a digit's own focus move does not make the next digit inert) and the card stayed engaged; *When* Enter is pressed on option 2, *Then* exactly one invocation with option 2 is made and all options become `aria-disabled`; *Given* a screen-reader tree snapshot, *Then* the options are exposed as buttons, not radios.
- The delivery-state line uses exactly the copy in `design/ux.md` section 3.1 (the single copy source; UX-AC23), one of 17 settled-state strings (18 literals counting the `human_active` variant; the 6 transient and reason strings of `design/ux.md` 3.1 are a separate set: "Still sending...", the 30 s no-response copy, "Already answered - reload to see the answer", "Offline", "Reconnecting", "Updating..."): "Sending...", "Sent - waiting for the agent", "Sent, but the agent hasn't confirmed yet. Open session." (Unconfirmed), "Answered: {value}", "Not sent. Nothing was typed into the session. Retry" (`NOT_SENT`; the `human_active` variant names the typing), "Sent, but we couldn't confirm it arrived. Check the session before retrying." (`SEND_INDETERMINATE`), "Couldn't send your answer. Retry" (client-side failure), "Your answer is saved. The agent can read it once you resume the session.", "This session is archived. Your answer will be saved but not delivered unless you resume it.", "Session ended before this was answered.", "Already handled in the session. No action needed.", "The session is now asking something else. Open session to answer it.", "Answered elsewhere: {value}", "Part of your answer was sent, then the prompt changed. Open session to finish.", "Answer from desktop", "Sent, but the question is still showing. Check the session before retrying." (`delivery_failed(still_showing)`), "Sent, but the session ended before confirming it." (a `sent`, Unconfirmed or still-showing row whose session ended; "Session ended before this was answered." is for rows where nothing was sent). It is plain text (not a live region); the single global region announces only terminal results. The wording in `research/ux.md` s5 and Story 3.4's earlier fragments are superseded by it.
  - *Given* `delivery_state=saved_for_later` on a stopped session, *When* rendered, *Then* the text "Your answer is saved. The agent can read it once you resume the session." is present and exactly one delivery line exists and it has no `aria-live` or live `role`.
  - *Given* outcomes `NOT_SENT` and `SEND_INDETERMINATE`, *When* the line renders for each, *Then* the two strings differ exactly as listed, `NOT_SENT` offers Retry and Open session, and `SEND_INDETERMINATE` offers Open session and a Retry that requires an inline "Retry anyway?" confirm.
  - *Given* `delivery_failed(still_showing)` and, separately, a `sent` row whose session ended, *When* the line renders, *Then* the strings are rows 16 and 17 exactly, neither contains "Nothing was typed" or "Session ended before this was answered", and row 16 offers Open session with a Retry behind "Retry anyway?".
- Validation and unrenderable shapes (UX-AC22, UX-AC28): submitting with a required group empty shows an inline text error linked by `aria-describedby`, sets `aria-invalid` and moves focus to the first invalid group; an unrenderable or unmeasured shape shows the question and options as text with "View full prompt in session" and Open session.
  - *Given* a two-question payload with Q2 unanswered, *When* Submit is activated, *Then* the Q2 group has `aria-invalid="true"`, its error text is referenced by `aria-describedby` and `document.activeElement` is inside Q2; *Given* an `unknown`-shape payload, *Then* the options render as a text list with a "View full prompt in session" link.
- One polite announcement per asynchronous delivery transition (`design/ux.md` 11.1): `delivery_failed(not_sent)` (row 5) says "Answer not delivered", `sent` to `delivery_failed(still_showing)` (row 16) says "Answer sent, question still showing" (a digit was written, so never "Answer not delivered"), a card turning Unconfirmed says "Answer not confirmed", an aborted multi-question answer says "Answer partly sent", a late acknowledgement is silent; the `sent` transition announces "Answer sent", never "Answered"; the delivery line stays non-live.
  - *Given* a card whose state moves to `delivery_failed(not_sent)` (and separately `sent` to `delivery_failed(still_showing)`, to Unconfirmed, and separately to aborted), *When* each transition fires twice with the same state, *Then* the polite region text changes exactly once per distinct transition with the strings above and the delivery line has no live role; *Given* an acknowledgement arriving after Unconfirmed, *Then* no announcement is made.
- Tier B on remote shows "Answer from desktop" (ADR-004).
- E2E: a structured question with options renders and an answer reaches the originating live session on the isolated server using a real private-tmux-socket session and a fake dialog fixture.
  - *Given* the isolated test server and a session whose pane displays the fixture dialog, *When* the test clicks "Redis" in the pane, *Then* the fake pane receives exactly `1`, the row shows "Sent - waiting for the agent" and, once the fixture dialog is removed for two samples, "Answered: Redis".
**Files**: `web-app/src/components/guidance/GuidanceQuestionForm.tsx`, `web-app/src/components/guidance/GuidanceRequestPanel.tsx`, `web-app/src/components/ui/notification-actions/ActionRow.tsx` (created by Story 2.5), `tests/e2e/askuserquestion-structured.spec.ts`

##### Task 3.7a: Extract `GuidanceQuestionForm` from `GuidanceRequestPanel` without behavior change (M)
- Files: `web-app/src/components/guidance/GuidanceQuestionForm.tsx`, `web-app/src/components/guidance/GuidanceRequestPanel.tsx`, `web-app/src/components/guidance/GuidanceQuestionForm.test.tsx`
##### Task 3.7b: Multi-select, multi-question, delivery-state line (17 strings), copy, a11y and the one-announcement-per-transition rule (M)
- Files: `web-app/src/components/guidance/GuidanceQuestionForm.tsx`, `web-app/src/components/guidance/GuidanceQuestionForm.css.ts`, `web-app/src/lib/copy/notificationCopy.ts`, `web-app/src/components/guidance/GuidanceQuestionForm.test.tsx`
##### Task 3.7c: E2E spec with fake-pane fixture, `// @feature session:guidance-structured`, phone case with `hasTouch` (M)
- Files: `tests/e2e/askuserquestion-structured.spec.ts`, `tests/e2e/pages/NotificationPanel.ts`, `tests/e2e/helpers/test-server.ts`
##### Task 3.7d: Inline validation error wiring (UX-AC22) and the unrenderable-shape "View full prompt in session" state (UX-AC28) (S)
##### Task 3.7e: "Saved for later (N)" list in Recent activity (read-only, agent-created rows only, Open session, not in the badge) and the stopped-vs-hook-row distinction (UX-AC79) (S)
##### Task 3.7f: Option-button single-select (replaces any radio markup), focus-only keyboard model and the browse-without-sending test (UX-AC18) (S)

### Story 3.8: MCP schema for agent-originated structured questions
**As an** agent, **I want** `create_guidance_request` to accept options, `multi_select` and multiple questions, **so that** non-hook questions render the same way.
**Acceptance Criteria**:
- New optional arguments validate against `QuestionPayload` limits; omitted arguments behave as today.
  - *Given* a call with `questions=[{text:"Region?",options:[{label:"us"},{label:"eu"}]}]`, *When* created, *Then* the row has `questions_json` set and `question_text="Region?"`.
**Files**: `server/mcp/tools_guidance.go`, `server/mcp/tools_guidance_test.go`

##### Task 3.8a: Extend tool args and validation; keep ownership checks (S)

---

# Epic 4: Agent messaging and durable wake triggers (flag `agent_wake_triggers`)

**Goal**: Attributed, gated help requests (human or peer) and wake triggers that survive restart, evaluated by one dispatcher (ADR-003, ADR-006, ADR-007).

### Story 4.1: `WakeTrigger` schema and repository
**As a** developer, **I want** a durable, race-safe trigger table with separate claim and delivery facts, **so that** wakes survive restart and are delivered exactly once to the receiver.
**Acceptance Criteria**:
- Unique non-null `dedup_key`: creating the same trigger twice returns the first.
  - *Given* owner `7c1e...` creating `at` trigger for `2026-10-18T09:00:00Z` with payload "check PR" twice, *When* both complete, *Then* one row exists and both callers get its id.
- Claim is idempotent: concurrent claims -> exactly one `pending -> claimed` transition (`-race`); `delivered_at` is set only by `MarkDelivered` after a confirmed write.
  - *Given* 8 goroutines calling `Claim(id)`, *When* complete, *Then* exactly one returns `applied=true`, `claimed_at` is set and `delivered_at` is null.
  - *Given* a `claimed` row, *When* `MarkDelivered(id)` runs twice, *Then* the first sets `delivered_at`, the second is a no-op, and `fire_count==1`.
- Expired/unreachable rows are markable `missed` without a claim; `needs_revive` is a valid state for a claimed wake awaiting admission.
  - *Given* a pending row with `expires_at` in the past, *When* `MarkMissed(id)` runs, *Then* status is `missed` and `claimed_at`/`delivered_at` stay null.
  - *Given* a `claimed` row whose stopped owner has `allow_revive=true`, *When* `MarkNeedsRevive(id)` runs, *Then* status is `needs_revive` and the transition table rejects `delivered -> claimed`.
- The repository enforces the complete transition table (ADR-006 rule 2) and rejects everything else, including `pending -> delivered` and any move out of a terminal state.
  - *Given* a `claimed` row and the archived-owner guard, *When* `MarkSkipped(id, reason)` runs, *Then* status is `skipped`; *Given* a `needs_revive` or `claimed` row, *When* `Cancel(id)` runs, *Then* status is `cancelled`; *Given* a `delivered` row, *When* `Cancel(id)` runs, *Then* it is a no-op returning `applied=false`.
- Recurring `cron` rows re-arm on the same row: a confirmed write with `fire_count + 1 < max_fires` takes `claimed -> pending` with `next_fire_at` advanced; the last fire (or `expires_at`) ends in `delivered`.
  - *Given* a `cron` row `*/5 * * * *` with `max_fires=3`, *When* it is claimed and `MarkDelivered` runs, *Then* status is `pending`, `fire_count==1` and `next_fire_at` is the next 5-minute boundary; after the third delivery status is `delivered` and a fourth claim is rejected.
- A one-shot row takes `pending -> skipped(reason=rate_limited)` on its third consecutive claim-time deferral; a `cron` row never does (it skips that occurrence and stays `pending`, ADR-006 rule 2).
  - *Given* a `pending` `at` row, *When* `MarkSkipped(id,"rate_limited")` runs, *Then* status is `skipped`; *Given* a `pending` `cron` row, *When* `MarkSkipped(id,"rate_limited")` is attempted, *Then* the repository rejects it, and `SkipOccurrence(id)` advances `next_fire_at` to the next cron time with status and `fire_count` unchanged.
- The consecutive-deferral count has storage: the `rate_limit_deferrals` column (int, default 0), incremented in the same UPDATE that moves `next_fire_at`, reset to 0 by a successful claim or by `SkipOccurrence`, and kept across a restart.
  - *Given* a `pending` `at` row deferred twice, *When* the repository is closed and reopened (restart), *Then* `rate_limit_deferrals==2` and the third deferral takes it to `skipped(rate_limited)`; *Given* a row deferred twice and then claimed successfully, *Then* the column is 0 and a later deferral starts again at 1; *Given* a `cron` row deferred, *When* `SkipOccurrence` runs, *Then* the column is 0.
- Terminal rows free their dedup key and are purged after 30 days (ADR-006 rule 12).
  - *Given* a `delivered` `at` row, *When* the same trigger is created again, *Then* a new row is created (the old key was tombstoned to `<key>#<id>`); *Given* a `pending` row, *Then* a duplicate create returns the first; *Given* terminal rows aged 29 and 31 days, *When* the retention sweep runs, *Then* only the 31-day row is deleted, and the sweep runs at startup and on the reconcile cadence with no new ticker.
**Files**: `session/ent/schema/wake_trigger.go`, `session/ent_repository_wake.go`, `session/domain/wake_trigger.go`, `session/ent_repository_wake_test.go`

##### Task 4.1a: Ent schema (`claimed_at`, `delivered_at`, `delivery_attempts`, `rate_limit_deferrals` default 0, status enum incl. `claimed`/`delivered`/`needs_revive`), indexes, regenerate with `--feature sql/upsert` (S)
##### Task 4.1b: Repository: create (dedup upsert), guarded claim/deliver/needs_revive/miss/cancel/skip, list pending and stale-claimed, count-by-owner in the same transaction as insert (M)
- Files: `session/ent_repository_wake.go`, `session/ent_repository_wake_test.go`, `session/domain/wake_trigger.go`
##### Task 4.1c: Domain sum types `WakeTriggerType`, `WakeStatus` with the full transition table (incl. `pending -> missed`, `pending -> skipped` for one-shot rows on the third rate-limit deferral, `claimed -> skipped`, `cancelled` from `pending|claimed|needs_revive`, cron `claimed -> pending` re-arm), `WakeSpec` per type, `WakeAction`, `WakeDedupKey` constructor from canonical spec; table-driven test over all (from, to) pairs (S)
##### Task 4.1d: Tombstone `dedup_key` to `<key>#<id>` in the terminal-transition UPDATE, the 30-day terminal-row retention sweep on the reconcile cadence, `Defer(id, nextFireAt)` (increments the persisted `rate_limit_deferrals`), and `SkipOccurrence` for cron rate-limit deferral (resets it) (S)

### Story 4.2: Dispatcher timer and injectable clock
**As a** maintainer, **I want** the dispatcher to own one timer over the durable table behind a `Clock` port, **so that** time triggers are testable without a clock seam in robfig/cron and without coupling to the workflow scheduler.
**Acceptance Criteria**:
- `server/wakes` owns one timer on `min(next_fire_at)` rebuilt on create/cancel/restart; `Clock` is injected (real in production, fake in tests); `workflows.Scheduler` is not modified and its tests are unchanged.
  - *Given* a pending `at` trigger for `now+2s` and a fake clock, *When* the clock advances 2 s, *Then* the trigger is claimed once.
  - *Given* a trigger created for `now+1h` while the timer is armed for `now+2h`, *When* it is created, *Then* the timer is re-armed to `now+1h`; *When* it is cancelled, *Then* the timer returns to `now+2h`.
- `cron` triggers use `cron.ParseStandard` only to compute the next fire time (minute granularity documented).
  - *Given* spec `*/5 * * * *` and fake time 10:02, *When* the next time is computed, *Then* it is 10:05; after firing it re-arms for 10:10.
- Cron timezone and DST are defined, not inherited (pre-mortem #20). Times are stored in UTC; a `cron` spec carries an IANA zone (`CRON_TZ=America/Chicago */30 * * * *`, which `cron.ParseStandard` supports, `robfig/cron/v3@v3.0.1/parser.go:95`) and defaults to `UTC` when absent, never to the server's local zone; an unknown zone is `invalid`. The Scheduled prompts row states the zone. A wall-clock time that does not exist (spring-forward gap; robfig documents that such jobs are not run, `doc.go:169`) is skipped and the next occurrence used, with `wake_trigger_total{state="skipped"}`-style log `reason=dst_gap`; an ambiguous wall-clock time (fall-back hour) fires once, at the first occurrence; a backward clock jump or a clock set earlier never re-fires an occurrence already recorded in `fire_count`/`delivered_at`.
  - *Given* the fake `Clock` and `CRON_TZ=America/Chicago 30 2 * * *` around 2026-03-08 (spring forward), *When* the day passes, *Then* no fire occurs on 2026-03-08 for the nonexistent 02:30 and the next fire is 2026-03-09 02:30 local; *Given* `30 1 * * *` around 2026-11-01 (fall back), *Then* exactly one fire occurs that day; *Given* the fake clock jumps back 1 h after a fire, *Then* the same occurrence does not fire again; *Given* a spec with no zone, *Then* `next_fire_at` is computed in UTC.
- Dispatcher goroutines are joined on shutdown and guarded by a single-leader lock for restart overlap.
  - *Given* a shutdown signal, *When* `Stop(ctx)` is called, *Then* all dispatcher goroutines exit before it returns (goleak).
- Wakes keep working when the workflow scheduler is not constructed (its `Start` returns early when `repo == nil`, `scheduler.go:158-162`).
  - *Given* a server wired with no workflow repo, *When* an `at` trigger comes due, *Then* it is claimed.
**Files**: `server/wakes/clock.go`, `server/wakes/timer.go`, `server/wakes/timer_test.go`, `server/dependencies.go` (one wiring line)

##### Task 4.2a: `Clock` port, fake clock, `min(next_fire_at)` timer with re-arm on create/cancel/restart (M)
- Files: `server/wakes/clock.go`, `server/wakes/timer.go`, `server/wakes/timer_test.go`, `server/dependencies.go` (one wiring line)
##### Task 4.2b: Cron next-time computation and re-arm (S)
##### Task 4.2c: Shutdown join + single-leader guard + goleak test; wiring through a `wakesModule` constructor (S)
##### Task 4.2d: Cron timezone (`CRON_TZ`, default UTC, row shows the zone) and DST/clock-jump handling with the four fake-`Clock` cases above (S)

**Epic 4 checkpoint (after 4.1 and 4.2, before 4.3)**: Epic 4 is the likeliest overrun. If cumulative spend on 4.1 + 4.2 exceeds 1.25 x their "CU with verification" value, pause and report the delta with its cause before starting 4.3.

### Story 4.3: `WakeDispatcher`
**As an** agent, **I want** my trigger delivered exactly once to me when its condition holds, even across a restart, **so that** I never need to poll.
**Acceptance Criteria**:
- Subscribe-before-read: an event published between subscription and the initial state read still fires.
  - *Given* a pending `backlog_event` trigger for item `b608ab1e-...` kind `verdict_recorded` and an event published after `Subscribe` but before the first state read, *When* the dispatcher starts, *Then* the trigger is claimed and delivered exactly once.
- Claim then deliver: fire claims the row; the row becomes `delivered` only after the pane write is confirmed; a crash between claim and write loses nothing.
  - *Given* a claimed row and a process kill before the pane write, *When* the service restarts and reconcile runs, *Then* the claim older than the lease is re-queued and delivered once, and a row already `delivered` is never re-sent.
  - *Given* the owner is mid-turn (`IsReadyForSteer=false`), *When* the trigger is claimed, *Then* no bytes are written, the row stays `claimed`, and after the session idles "<wake id=... from=operator-schedule>..." is delivered once and the row becomes `delivered`.
- Wake-write safety (ADR-006 rule 8, pre-mortem P1-c): before any `steer_text` write the dispatcher takes the `TerminalWriteLease`, runs a fresh `tmux capture-pane`, and writes only if the capture positively shows the idle composer and no dialog. The cached `IsReadyForSteer` status alone is not enough (blank 6-9 s after a keypress, `research/pitfalls.md` row 1), and `session.DialogMatch` cannot be the negative test because it needs a specific question (`session/pane_reply.go:74`), so Task 4.3c consumes the generic `session.PaneShowsDialog` check that Task 2.4c adds in W1 (the `Enter to select` footer and permission prompts are `Showing`; any unrecognized layout or a capture error is `Indeterminate`; only `Absent` permits a write). The payload always begins with `<wake` and is written with `SubmitContentWithEnter`, never as a selection key.
  - *Given* an owner whose cached status reads idle but whose pane shows the fixture `AskUserQuestion` dialog (`session/testdata/reply_dialogs/question_single_select_3opts.txt`), *When* a due `steer_text` wake is claimed, *Then* the fault-injecting pane double (Task 2.4a) receives zero bytes and the row stays `claimed`; *When* the dialog clears and a fresh capture shows the idle composer, *Then* the wake is delivered once.
  - *Given* a capture error, an unrecognized layout, or a permission dialog on screen, *When* a wake is claimed, *Then* zero bytes are written; *Given* any delivered wake, *Then* its first written character is `<`.
- Stale-claim re-queue is startup-only; the periodic `ReconcileStuck`-cadence pass never applies the lease and skips every `claimed` id held by the live `SteerDelivery` cache (ADR-006 rule 2).
  - *Given* a running dispatcher, a `claimed` row 3 minutes old (past the 2-minute lease) held by `SteerDelivery` because its owner is busy, *When* the periodic reconcile pass runs, *Then* the row is not re-queued and exactly one delivery happens when the owner idles; *Given* the same row after a process restart (cache empty), *When* startup reconcile runs, *Then* it is re-queued once.
- Restart reconcile: 1 overdue `at` trigger (within `expires_at`), 1 expired trigger, 1 `backlog_event` trigger whose condition became true while down, 1 `claimed` row older than the lease -> fire-once, `missed` with notification, fire-once, re-queued-and-delivered; `delivered` rows are never re-sent.
  - *Given* those four rows and a fresh dispatcher over the same database, *When* reconcile runs, *Then* counters show delivered=3, missed=1, `wake_reconcile_total{result="requeued_claimed"}=1`, and a second reconcile changes nothing.
- Guards and the owner-state decision table. **ADR-006 rule 2 is the single source of truth** for what happens to a claimed wake in each owner state; this story implements that table and does not restate it differently. The archived, one-shot, terminal-item and revive guards apply to `action_kind=steer_text` only: archived owner, one-shot owner (via the exported `session.IsOneShot(inst)`; `isOneShot` is unexported at `session/session_driver.go:1171`), owner whose backlog item is terminal, or deleted owner -> `skipped` with reason and operator notification. A stopped non-archived owner is revived only by `reviveStoppedOwner` (the single function that calls `RecoverFromStopped`, `session/instance_state.go:533`, then the normal start path) and only when `allow_revive` AND `Admission.Admit` (`BacklogService.Admit`, WIP cap 2) admits it; with `allow_revive=true` the row is `needs_revive` while `Admit` refuses, within `expires_at`, then `missed`; with `allow_revive=false` (the default) it is `skipped(reason=owner_stopped)` with a failure-class notification and is never `needs_revive`. A `notify_operator` wake is exempt from every guard above (it writes nothing into the owner): it is delivered as a system-level INFO notification regardless of owner state. The rework cap is spawn-only and is not consulted. `RecoverFromStopped` only resets status to `Creating` (it starts nothing), so `reviveStoppedOwner` calls `RecoverFromStopped()` then `Start(false)` (the pairing at `session/instance_claude.go:143-145`, `session/retry_state.go:335`); it re-checks `IsArchived()` itself immediately before (the analyzer is lexical); and the **revive lock** (`reviveMu`, separate from the short cap lock and bounded by the `revive_lock` row, default 30 s) is held from before `Admit` until the revived session counts as an active work session, because `Admit` reserves no slot.
  - *Given* an archived owner and a `steer_text` trigger, *When* it is claimed, *Then* status is `skipped(reason=archived)`, no `Start`/`RecoverFromStopped` call occurs (spy), and a failure-class notification is emitted.
  - *Given* a stopped, non-archived owner with `allow_revive=false`, *When* its `steer_text` trigger is claimed, *Then* status is `skipped(reason=owner_stopped)`, a failure-class notification is emitted, no `Start`/`RecoverFromStopped` call occurs (spy) and the row never enters `needs_revive`.
  - *Given* a `notify_operator` trigger (an operator "Remind me") whose owner is archived (and, separately, one-shot, item-terminal, stopped and deleted), *When* it is claimed, *Then* status is `delivered`, an INFO notification with no session binding and `metadata.target_session_uuid` is created, no guard ran and no revive call occurred (spy), and `deliverygate.ShouldDeliver` delivers it; for the deleted owner the text reads "Session no longer exists".
  - *Given* WIP cap 2 with 2 work sessions in progress and an `allow_revive=true` wake for a stopped third, *When* it is claimed, *Then* the row is `needs_revive`, nothing is revived, and after a slot frees the next cycle revives it once.
  - *Given* an owner archived between claim and revive, *When* `reviveStoppedOwner` runs, *Then* it returns without calling `RecoverFromStopped`/`Start` (spy) because its own `IsArchived()` check fires, and the analyzer reports zero findings on the file without relying on caller guards.
  - *Given* 3 `allow_revive=true` wakes for 3 stopped owners with exactly 1 free slot and two goroutines racing `Admit`, *When* both run, *Then* exactly 1 owner is revived (the revive lock spans `Admit` through the revived session counting as live) and the other 2 rows are `needs_revive`.
- The revive lock is bounded and separate from the cap lock (ADR-007 rule 5): a slow tmux and worktree spawn cannot stall create-time or claim-time cap checks, and a hung spawn cannot hold admission forever.
  - *Given* a fake `Start` that blocks past 31 s of fake clock, *When* a revive is attempted, *Then* the revive lock is released at 30 s, the row stays `needs_revive`, and a concurrent `schedule_wake` create-time cap check completed within 50 ms of fake time while the spawn was blocked; *Given* the next cycle finds the session running, *Then* it is treated as live and the wake is delivered once.
- Caps under one short process-wide cap lock (`capMu`): per-session pending cap, per-trigger `max_fires`, global budget, chain `generation` limit; exactly N of M concurrent creates succeed (`-race`, boundary `>` vs `>=` tested).
  - *Given* per-session cap 5 and 20 concurrent `schedule` attempts, *When* complete, *Then* exactly 5 succeed and 15 return a visible `cap_exceeded` result.
- Every row of the **Wake limits** table is enforced at its stated point (create or claim) with the default and the hard ceiling, and every hit increments `wake_limit_hit_total{limit,phase}` (Task 4.3h). Defaults and ceilings are injectable.
  - *Given* `schedule_wake(cron="* * * * *")` or `"*/5 * * * *"` with the default 10-minute floor, *When* handled, *Then* it returns `cap_exceeded` naming `min_interval` and creates no row; *Given* operator config lowering the floor to 5 min while the per-owner limit is still 6, *Then* the effective floor is `max(5, ceil(60/6))` = 10 min and `*/5 * * * *` is still refused; *Given* the floor config at 5 min and the per-owner limit raised to 12, *Then* `*/5 * * * *` is accepted and a floor config of 4 min is clamped to 5; *Given* a per-owner limit config above the ceiling of 12, *Then* it is clamped to 12 (a `TestEffectiveCronFloor_*` table test over limit 1..12 asserts `floor == max(5, ceil(60/limit))`); *Given* `max_fires=49` (ceiling 48) or `expires_at` 8 days out (ceiling 7), *Then* each is refused naming its limit.
  - *Given* one owner with 7 wakes due within an hour (limit 6), *When* claimed in order, *Then* 6 are delivered, the 7th stays `pending` with `next_fire_at` moved to when the window frees a slot, exactly one failure-class digest notification is raised for that owner, and `wake_limit_hit_total{limit="fires_per_owner_hour",phase="fire"}` is 1; *Given* the global limit of 60, *Then* the 61st due wake across owners is deferred the same way.
  - *Given* a one-shot `at` row deferred 3 consecutive times, *When* the third deferral occurs, *Then* it becomes `skipped(reason=rate_limited)` with a notification; *Given* a `cron` row deferred at one occurrence, *Then* that occurrence is skipped, the row stays `pending` with `next_fire_at` at the next cron time and `fire_count` unchanged, and it never becomes `skipped` for rate limiting; *Given* a chain at `generation` 5 scheduling another wake, *Then* it is refused `cap_exceeded(generation)`.
- Coalescing: 10 events for the same key within the debounce window produce one delivery.
  - *Given* 10 `verdict_recorded` events for item X in 2 s with debounce 5 s (the supported `(backlog_event, verdict_recorded)` row of Story 4.4's table), *When* processed, *Then* one steer is delivered.
- Reconcile and re-enable notifications are digested, not flooded (pre-mortem #6): one startup reconcile run, however many rows it marks `missed` or `skipped`, raises exactly **one** failure-class digest notification ("3 scheduled prompts were missed during the restart") listing up to 5 owners with a count for the rest, capped at 1 digest per run and per owner per hour; archived-owner `skipped` rows count toward that digest instead of emitting one each; re-enabling `agent_wake_triggers` after it was off fires each overdue in-window row at most once through the same admission, debounce and Wake-limits path as any other fire (no bypass).
  - *Given* 12 expired rows across 4 owners and 3 archived-owner rows at startup, *When* reconcile runs, *Then* exactly one digest notification exists and no per-row notification; *Given* the flag off for an hour with 30 overdue `at` rows for one owner, *When* it is turned on, *Then* at most 6 are delivered in the first hour (the per-owner limit), the rest are deferred under the Wake-limits rule, and the digest names the deferral.
- Duplicate delivery is a contract, not a hope (pre-mortem #9): every delivered text begins `<wake id=... n=... >` followed by the standing sentence "If you have already acted on wake id ... , do nothing."; a re-send of an already-`delivered` id is impossible by the repository rule, but the crash-after-write/before-`MarkDelivered` window can re-send once, which `wake_duplicate_delivery_total` counts when the same id is written twice. A `claimed` row for a busy owner is held at most the **busy-owner hold** (30 min default, 2 h ceiling, never past `expires_at`) and then becomes `missed(reason=owner_busy_timeout)`.
  - *Given* a prompt golden for a wake payload, *When* it is rendered, *Then* it contains the id and the standing sentence; *Given* a process kill after the pane write but before `MarkDelivered`, *When* restart reconcile re-sends, *Then* `wake_duplicate_delivery_total` is 1 and the second payload carries the same id; *Given* a busy owner for 31 minutes, *Then* the row is `missed(owner_busy_timeout)` and a digest entry exists.
- `needs_revive` cannot sit forever (pre-mortem #14): the oldest `needs_revive` age is a gauge with a 1-hour alert; the case where `Admit` counts the owner's own `in_progress` item and refuses its revive (`countLiveBacklogWorkSessions`, `server/services/backlog_service_triage.go:1314`, plan round-2 nuance) is a pinned test showing the row stays `needs_revive`, the alert fires at 1 h, and `expires_at` then makes it `missed`.
  - *Given* an `allow_revive=true` wake for a stopped owner whose item is `in_progress` and a fake `Admission` that refuses, *When* 61 minutes pass on the fake clock, *Then* the row is still `needs_revive`, one failure-class notification was raised and `wake_needs_revive_age_seconds` exceeded 3,600.
- Package direction: `server/wakes` defines the ports `Steerer`, `Admission`, `OwnerLookup`, `Clock`; `server/services` supplies adapters in wiring and depends only on the func type `AnswerListener` it owns; there is no import cycle.
  - *Given* the built module graph, *When* `go list -deps ./server/wakes` runs in a test, *Then* it does not contain `server/services` or `server/mcp`.
**Files**: `server/wakes/dispatcher.go`, `server/wakes/reconcile.go`, `server/wakes/delivery.go`, `server/wakes/guards.go`, `server/wakes/revive.go`, `server/wakes/ports.go`, `server/wakes/*_test.go`, `session/session_driver.go` (export `IsOneShot` only)

##### Task 4.3a: Dispatcher core: bus consumer, index by `(type,key)`, subscribe-before-read, guarded claim (M)
- Files: `server/wakes/dispatcher.go`, `server/wakes/dispatcher_test.go`, `server/wakes/ports.go`
##### Task 4.3b: Startup reconcile (time and event state, stale-claim re-queue, `needs_revive` re-admission), fire-once policy, readiness barrier (M)
- Files: `server/wakes/reconcile.go`, `server/wakes/reconcile_test.go`, `server/wakes/dispatcher.go`, `server/wakes/predicates.go`
##### Task 4.3c: `SteerDelivery`: queue as a cache of `claimed` rows + `IsReadyForSteer` + expiry + escaping + provenance + `MarkDelivered` after confirmed write; the wake-write safety step (lease, fresh capture, `session.PaneShowsDialog` from Task 2.4c, `<wake` marker, `SubmitContentWithEnter`) and its fault-injection test; optional Channels transport only if S3 passed (M)
- Files: `server/wakes/delivery.go`, `server/wakes/delivery_test.go`, `server/wakes/ports.go` (the `Steerer` port carries the `session.PaneShowsDialog` check)
##### Task 4.3d: Guards (`steer_text` only; `notify_operator` is exempt): archived/one-shot (export `session.IsOneShot`)/terminal-item/owner-missing, `allow_revive` (false -> `skipped(owner_stopped)`, never `needs_revive`), chain `generation`, per-key debounce (S)
- Files: `server/wakes/guards.go`, `server/wakes/guards_test.go`, `session/session_driver.go` (export `IsOneShot` only)
##### Task 4.3l: Revive path (split from 4.3d): `Admission` + WIP cap, `reviveStoppedOwner` (`RecoverFromStopped()` then `Start(false)`, own `IsArchived()` check, `make lint` SA1019 check with a `//nolint:staticcheck` reason if flagged), and the revive lock (30 s bound, separate from the cap lock) held across `Admit` until the revived session is live, with the racing-`Admit` and blocked-`Start` tests (M)
- Files: `server/wakes/revive.go`, `server/wakes/revive_test.go`, `server/wakes/ports.go`, `server/wakes/dispatcher.go`
##### Task 4.3e: Missed/skipped/failed operator notifications using existing types with `HintFailure`/ERROR so hidden-session owners still reach the operator; add rows to the gate-class table test (S)
##### Task 4.3f: Extend `tools/lint/noarchivedrevival/analyzer.go` `lifecycleFiles` (lines 38-45) with `server/wakes/revive.go` **and `server/services/question_answer_router.go`** (the `AnswerRouter` `resume` route of Task 3.4d, which never calls `Start`, `RecoverFromStopped`, `restartForRetry` or `transitionToLocked(..., Active)` by decision, so the entry is a ratchet and the analyzer fixture records that); add analyzer testdata fixtures (unguarded revive in that path is reported, guarded one is not); AC "analyzer reports zero findings on `server/wakes`" run in CI via `make lint-custom` (S)
##### Task 4.3g: Ports (`Steerer`, `Admission`, `OwnerLookup`, `Clock`), `AnswerListener` func type, wiring adapters, and the `go list -deps` import-direction test (S)
##### Task 4.3h: Wake limits: config-backed defaults and compiled ceilings for every row of the Wake limits table, create-time and claim-time enforcement under the short cap lock (race test asserts the 50 ms budget) plus the bounded revive lock, deferral and `rate_limited` skip, per-owner digest notification, `wake_limit_hit_total` and `wake_delivered_per_owner_hour`, plus the effective cron floor derivation and its `TestEffectiveCronFloor_*` table test (M)
- Files: `server/wakes/limits.go`, `server/wakes/limits_test.go`, `server/wakes/dispatcher.go`, `server/wakes/metrics.go`, `config/config.go`
##### Task 4.3i: Reconcile digest notification (one per run, per-owner/hour cap, archived-owner skips folded in) and the re-enable path through the normal admission, with the two tests above (S)
##### Task 4.3j: Wake marker standing sentence, `wake_duplicate_delivery_total`, busy-owner hold (30 min / 2 h) and `missed(owner_busy_timeout)` (S)
##### Task 4.3k: `wake_needs_revive_age_seconds` gauge and 1-hour alert, and the `in_progress` refused-revive pinned test (XS)

### Story 4.4: Trigger sources
**As an** agent, **I want** to wait on time, backlog/PR/CI events, a peer's state, or a human answer, **so that** I can stop polling.
**Acceptance Criteria**:
- `backlog_event`: kind filter on `BacklogItemEventPayload.Kind`; every kind used is covered by a mapping test (a missing kind in `mapBacklogChangeKind` fails the test).
  - *Given* a trigger on item X kind `verdict_recorded`, *When* the event arrives, *Then* it fires; *When* a kind not in the mapping table is introduced, *Then* the test `TestEveryBacklogKindMapped` fails.
- Every supported `(trigger_type, kind, predicate)` declares a persisted-state predicate used by reconcile (the `EventBus` drops events per slow subscriber and is in-process, `pkg/events/bus.go:86-109`); a kind with no such predicate is rejected by `schedule_wake` with the typed error `no_reconcile_predicate`. The supported table (final list confirmed in Task 4.4d against `session/backlog_item_change.go` and `mapBacklogChangeKind`):

| trigger_type | kind (event) | predicate evaluated by reconcile |
|---|---|---|
| `backlog_event` | `status_reached{status}` (event `status_transition`) | item.status equals `status` |
| `backlog_event` | `verdict_recorded` | item has a recorded verdict |
| `backlog_event` | `item_archived` | item is archived |
| `backlog_event` | `item_removed` | item no longer exists |
| `backlog_event` | `session_attached{role}` | item has an attached `ItemSession` of that role |
| `session_state` | `status_reached{status}` | peer session status equals `status` |
| `guidance_answered` | `answered` | the row's `answered_at` is set |
| `pr_event`, `ci_event` | per Spike S4 | PR or check state from the poller, or typed `unavailable` |
| `at`, `cron` | time | `next_fire_at` is due |

Rejected with `no_reconcile_predicate`: bare `status_transition` (no target status), `item_updated`, `triage_progress_updated`, `activity_note_added`.

  - *Given* a request for `backlog_event` kind `status_transition` as a bare edge with no target status, *When* `schedule_wake` is called, *Then* it returns `no_reconcile_predicate` and no row is created; *Given* `status_reached{status:"review"}`, *Then* it is accepted; *Given* kind `verdict_recorded` with predicate "item has a recorded verdict", *When* the condition became true while the service was down, *Then* reconcile fires it once.
  - *Given* the full `(type,kind)` table, *When* `TestEveryKindHasReconcilePredicateOrIsRejected` runs, *Then* it passes and fails if a new kind is added without one.
- `pr_event`/`ci_event` implemented per S4 (bus event, poller-derived, or typed `unavailable` error), subject to the same predicate rule.
  - *Given* S4 reported a poller-derived source, *When* the PR's checks complete, *Then* the dispatcher fires with payload `{pr:901, conclusion:"success"}`; or *Given* S4 reported none, *When* `schedule_wake(type=ci_event)` is called, *Then* it returns `unavailable`.
- `session_state`: fires when peer session UUID reaches a target status; re-evaluated at reconcile.
  - *Given* peer `p1` Running and a trigger "p1 reaches Idle", *When* p1 becomes Idle (or already is at subscribe time), *Then* it fires once.
- `guidance_answered`: fired from the `AnswerGuidanceRequest` success path via the `AnswerListener` callback and by reconcile when the row is already answered.
  - *Given* trigger on request `g1` and `AnswerGuidanceRequest(g1,"yes")` succeeding, *When* the callback runs, *Then* the trigger is claimed once, and a restart before delivery still delivers once via reconcile.
**Files**: `server/wakes/sources_backlog.go`, `sources_pr.go`, `sources_session.go`, `sources_guidance.go`, `server/wakes/predicates.go`, `server/services/guidance_request_service.go` (listener hook only), `pkg/events/types.go`

##### Task 4.4a: Backlog + PR/CI sources per S4 (M)
- Files: `server/wakes/sources_backlog.go`, `server/wakes/sources_pr.go`, `server/wakes/predicates.go`, `pkg/events/types.go`
##### Task 4.4b: Peer session-state source (S)
##### Task 4.4c: Guidance-answered callback + reconcile (S)
##### Task 4.4d: Reconcile predicate table per `(type,kind)`, `no_reconcile_predicate` rejection, and the table-completeness test (S)

### Story 4.5: MCP tools with verified caller identity
**As an** agent, **I want** to schedule wakes and ask peers or the human, **so that** I coordinate without polling or raw injection.
**Acceptance Criteria**:
- Every new tool is wrapped by `withCallerIdentityGate`; a registration test fails if one is not (`MCPServer.GetTool`).
  - *Given* the MCP server with the tools registered, *When* the test inspects each new tool's handler, *Then* all five are the gate-wrapped form; adding an unwrapped tool in a test fixture makes the test fail.
- Identity comes only from the verified context (Story 4.9); a forged `X-Stapler-Session-UUID` without a valid proof is rejected by each of the five tools with `identity_unverified` and zero writes.
  - *Given* a request to `schedule_wake` with header `X-Stapler-Session-UUID: <session B's uuid>` and no or invalid `X-Stapler-Session-Proof`, *When* handled, *Then* the result is `identity_unverified`, no row is created and `mcp_identity_unverified_total{tool="schedule_wake"}` is 1.
  - *Given* session A's valid proof presented with session B's UUID, *When* `answer_help` is called for a request addressed to B, *Then* the result is `identity_unverified` and the request is unchanged.
- A `session_id` argument is rejected/ignored.
  - *Given* a dispatched review session with a valid proof calling `schedule_wake` with `session_id` of another session, *When* handled, *Then* the owner recorded is the verified caller UUID, or the call is denied per the role allowlist.
- Peer help: self-target, archived, one-shot and cross-workspace peers are rejected at call time with a typed `rejected` result naming the reason and **no row is created** (refined in Story 4.11; counted in `peer_help_total{state="rejected"}`); only the named answerer (verified) can `answer_help`; cross-pair answer -> `FORBIDDEN`.
  - *Given* request R from A to B, *When* session C (valid C proof) calls `answer_help(R)`, *Then* the result is `FORBIDDEN` and R is unchanged.
  - *Given* B (valid proof) calls `answer_help(R,"use approach 2")`, *When* handled, *Then* the answered row has `answerer_session_uuid=B`, A's implicit `guidance_answered` wake fires with provenance "from session B", and no human-guidance `notifyAnswered` notification is created for the asker (peer answers route through the wake/`SteerDelivery`).
- Human help uses existing `create_guidance_request`; `request_help` with `peer=human` is an alias with no new store.
**Files**: `server/mcp/caller_identity_gate.go`, `server/mcp/tools_wake.go`, `server/mcp/tools_peer_help.go`, `server/mcp/*_test.go`, `server/services/guidance_request_service.go` (answerer authorization, per-source side effects), `server/mcp/server.go`

##### Task 4.5a: `withCallerIdentityGate` (in its own file, not `tools_backlog.go`) + role allowlist table + registration test (M)
- Files: `server/mcp/caller_identity_gate.go`, `server/mcp/caller_identity_gate_test.go`, `server/mcp/server.go`
##### Task 4.5b: `schedule_wake`, `list_wakes`, `cancel_wake` (typed `flag_off`, `cap_exceeded`, `unavailable`, `no_reconcile_predicate`, `identity_unverified` errors; "event stream unavailable" when `eventBus=nil`) (M)
- Files: `server/mcp/tools_wake.go`, `server/mcp/tools_wake_test.go`, `server/mcp/server.go`, `server/wakes/dispatcher.go`
##### Task 4.5c: `request_help`, `answer_help` + answerer authorization in `GuidanceRequestService.Answer`, implicit `guidance_answered` wake for the asker, per-source `notifyAnswered` suppression for peer rows (M)
- Files: `server/mcp/tools_peer_help.go`, `server/mcp/tools_peer_help_test.go`, `server/services/guidance_request_service.go`, `server/services/guidance_request_service_test.go`
##### Task 4.5d: Per-tool forged-header and cross-session-proof tests for all five tools (S)

### Story 4.6: Operator visibility, kill switch and reminders
**As an** operator, **I want to** see what agents are waiting on and cancel it, **so that** automation is never invisible.
**Acceptance Criteria**:
- "Scheduled prompts" section lists pending triggers and open peer requests (owner, condition, since, next fire) and a Cancel action (Tier A descriptors `CANCEL_WAKE` / `CANCEL_PEER_HELP`).
  - *Given* two pending triggers, *When* the Background tab's "Scheduled prompts" section renders, *Then* two rows with a Cancel button appear and cancelling one sets `status=cancelled`.
- Turning `agent_wake_triggers` off stops firing and shows "Scheduled prompts paused" in that section.
- Cancel asks for inline confirmation and handles the fire race (UX-AC36, UX-AC37): "Cancel this scheduled prompt? The agent will not be re-prompted." with "Yes, cancel" and "Keep"; after confirm the row leaves and a polite "Prompt cancelled" is announced; if the trigger fired first the row shows "Already fired at {time}." with no toast. More than 20 rows render the first 20 with "Show all (N)".
  - *Given* a pending trigger, *When* "Cancel prompt" is activated, *Then* the inline confirm appears with focus on "Yes, cancel", confirming sets `status=cancelled` and announces "Prompt cancelled"; *Given* the trigger became `delivered` before the confirm, *Then* the row reads "Already fired at 13:40." and no `role="alert"` exists; *Given* 25 pending triggers, *Then* 20 rows and "Show all (25)" render.
- Remind me is no longer conditional on an owner decision (decided in triad round 1): the `RemindMeMenu` tests are enabled; being in the last wave is a scheduling fact only.
- Operator "Remind me" on informational Background rows creates a `notify_operator` trigger and never appears on blocking questions. **Last wave (W6), optional-to-order, kept in scope; default presets "In 1 hour", "Tomorrow 9:00", "Pick time..." (config-changeable; nothing is blocked on the owner).**
  - *Given* a finished review row, *When* "Remind me in 1 hour" is chosen, *Then* a `WakeTrigger(action=notify_operator)` exists and an INFO-class operator notification is raised at fire time as a system-level notification with no session binding (so the hidden-session gate does not suppress it) and `metadata.target_session_uuid` linking to the row's session, even when that session is archived or stopped (guards do not apply to `notify_operator`, ADR-006 rule 2); the blocking-question row has no such button.
**Files**: `proto/session/v1/wake_trigger.proto`, `server/services/wake_trigger_service.go`, `web-app/src/components/ui/ScheduledPrompts.tsx` (the component for the "Scheduled prompts" section), `BackgroundActivity.tsx`, `tests/e2e/agent-wake-triggers.spec.ts`

##### Task 4.6a: `ListWakeTriggers`/`CancelWakeTrigger` RPCs and registry (S)
##### Task 4.6b: `ScheduledPrompts` UI + kill-switch indicator, mobile layout (M)
- Files: `web-app/src/components/ui/ScheduledPrompts.tsx`, `web-app/src/components/ui/ScheduledPrompts.css.ts`, `web-app/src/components/ui/ScheduledPrompts.test.tsx`, `web-app/src/components/ui/BackgroundActivity.tsx`
##### Task 4.6c: Operator "Remind me" on informational rows (S) - last wave, default presets
##### Task 4.6d: Scheduled prompts inline Cancel prompt confirm with the "Already fired at {time}." race copy, the 20-row cap with "Show all (N)", and their tests (S)

### Story 4.7: Prompt updates and polling retirement
**As an** operator, **I want** agents told to use the new tool instead of polling, **so that** no new `ScheduleWakeup` loops are added.
**Acceptance Criteria**:
- With the flag on, backlog/pipeline prompts reference `schedule_wake` and still forbid `ScheduleWakeup`/`/loop`; with the flag off they are unchanged.
  - *Given* `agent_wake_triggers` on, *When* `session/backlog_context.go` builds the prompt, *Then* it contains "schedule_wake" and "ScheduleWakeup" as forbidden; flag off -> snapshot equals the stored golden.
- A test asserts no new `ScheduleWakeup` references in Go code (zero new poll loops metric) by counting occurrences per file against an explicit map, not by matching prompt strings.
  - *Given* the repo tree, *When* the test runs `git grep -c ScheduleWakeup -- '*.go'`, *Then* the per-file counts equal the map `{server/mcp/tools_backlog.go:6, server/mcp/tools_backlog_test.go:2, server/mcp/tools_backlog_wait_cap_test.go:3, server/services/backlog_service_verdict_steer.go:2, session/backlog_commands.go:1, session/backlog_context.go:1, session/backlog_post_review_prompt_test.go:1, session/pipeline_mode_seed.go:2}` (counts verified 2026-10-17) and any new file or higher count fails.
**Files**: `session/backlog_context.go`, `session/backlog_commands.go`, `session/pipeline_mode_seed.go`, `session/*_prompt_test.go`, `server/mcp/tools_backlog.go` (description text only, if any), the guard test file

##### Task 4.7a: Flag-conditioned prompt text + golden tests (S)
##### Task 4.7b: Guard test for new `ScheduleWakeup` Go references with the explicit per-file map (XS)

### Story 4.8: Restart, race and storm integration tests
**As a** maintainer, **I want** the failure classes from the incident history reproduced as tests, **so that** they cannot regress silently.
**Acceptance Criteria**:
- Full-stack restart test: create triggers, stop server, restart, assert delivered/missed counters and single delivery (the success metric "wakes survive restart").
  - *Given* 3 pending triggers (one due during downtime), *When* the server process restarts against the same database, *Then* each trigger is delivered exactly once and `wake_reconcile_total` matches.
- Restart-gap tests for the claim/delivery split.
  - *Given* a trigger claimed and the process killed before the pane write, *When* the service restarts, *Then* it is delivered once; *Given* a trigger `delivered`, *When* the service restarts, *Then* it is not re-sent; *Given* a busy owner at restart, *When* it idles, *Then* the claimed wake is delivered once.
- `-race` cap tests, wake storm, chained-wake generation limit. The storm is stated in terms of revives of stopped owners, not spawns (wakes target existing sessions).
  - *Given* 50 `allow_revive=true` triggers for 50 stopped owners on item X and WIP cap 2, *When* X changes status, *Then* at most 2 owners are revived per admission cycle and the rest are `needs_revive`, none lost, and live (non-stopped) owners are woken without any revive.
  - *Given* wakes for 3 stopped owners with exactly 1 free slot, *When* they are claimed concurrently (`-race`), *Then* exactly 1 owner is revived and 2 rows are `needs_revive` (the revive lock spans `Admit` until the revived session is live).
- Cost-shaped storm test whose assertion is the numeric ceiling (the $106.69 idle-wait shape, `research/pitfalls.md` section 4): one owner repeatedly scheduling the most aggressive wakes the table allows yields at most the per-owner hourly limit of deliveries, and all owners together at most the global limit.
  - *Given* a fake clock and one owner that schedules a `*/10 * * * *` cron (the default floor) with `max_fires=48` and re-schedules from every woken turn, *When* 3 simulated hours pass, *Then* deliveries to that owner never exceed 6 in any rolling hour, `wake_delivered_per_owner_hour` never exceeds 6, the chain never exceeds `generation` 5, and `wake_limit_hit_total` is non-zero; *Given* 20 such owners, *Then* total deliveries never exceed 60 in any rolling hour; *Given* the config floor lowered to 5 min and the per-owner limit at its ceiling of 12, *Then* deliveries to one owner never exceed 12 in any rolling hour.
**Files**: `server/wakes/integration_test.go`, `server/wakes/storm_test.go`, `server/wakes/restart_gap_test.go`

##### Task 4.8a: Restart-survival integration test using an on-disk temp database (M)
- Files: `server/wakes/integration_test.go`, `server/wakes/dispatcher.go`, `server/wakes/reconcile.go`
##### Task 4.8b: Race, storm (revive-admission framing) and generation tests (M)
- Files: `server/wakes/storm_test.go`, `server/wakes/integration_test.go`, `server/wakes/revive.go`
##### Task 4.8c: Claim/delivery restart-gap tests (kill after claim, delivered never re-sent, busy-owner) (M)
- Files: `server/wakes/restart_gap_test.go`, `server/wakes/delivery.go`, `server/wakes/reconcile.go`
##### Task 4.8d: Numeric-ceiling storm test and the `wake_delivered_per_owner_hour` assertion (S)

### Story 4.9: Verified MCP session credential (prerequisite of 4.5)
**As a** security reviewer, **I want** the MCP caller identity bound to a per-session credential that is delivered without touching argv or the tmux environment, **so that** an agent that merely knows another session's UUID cannot act as it (ADR-007 rule 0; adversarial #1 and round-2 R1, architecture #1).
**Acceptance Criteria**:
- A proof `HMAC-SHA256(secret, "ssq-mcp-v1" || sessionUUID)` is minted at spawn from the `server/services/hook_proof.go` machinery (new domain string; not interchangeable with a hook proof) and written to 0600 per-session files in 0700 directories: `<configDir>/mcp-proofs/<uuid>` (read by the stdio proxy) and the `X-Stapler-Session-Proof` header inside `<configDir>/mcp/<uuid>.json` (the Claude CLI config). **No `STAPLER_SESSION_PROOF` environment variable exists.**
  - *Given* a newly spawned session, *When* the proof files and the proxy request are inspected in a test, *Then* both carry a proof that verifies for that session's UUID and not for another's.
- The primary Claude path is covered: `claudeMCPConfigArgsWithToken` (`session/instance_tmux.go:655-680`) always emits the proof-bearing config through the 0600 file route (path in `--mcp-config`), also when no local API token exists (today `token == ""` returns the JSON inline at lines 666-667); it never emits a proof inline. **Fail closed on write failure**: the existing write-failure fallback (`instance_tmux.go:677-679`) deletes only `Authorization` from the shared `headers` map and re-marshals the config inline; Task 4.9d must restructure it so the proof header is never in the map used for the inline fallback (build the proof-free inline config from a separate map), so a failed file write launches the session with no proof (its five tools return `identity_unverified`), never with the proof in argv.
  - *Given* a spawned Claude session with `require_local_auth` off, *When* its `--mcp-config` argument is built, *Then* the value is a file path under `<configDir>/mcp/`, the file mode is 0600 and the directory 0700, and the file contains the proof header.
  - *Given* a freshly spawned Claude work session (spawned-session verify), *When* its `/mcp` request reaches `mcpWithUUID`, *Then* the verified context carries that session's UUID and `schedule_wake` is not refused with `identity_unverified`.
  - *Given* the `<configDir>/mcp` directory is made unwritable (forced write failure, e.g. a read-only directory or a path occupied by a file), *When* the Claude command is built, *Then* the command line and `buildExtraEnv()` contain neither the proof nor (when a token exists) the Authorization value, and the launched session's five tools return `identity_unverified`.
- Non-leak tests: the proof string appears in neither the built command line (`buildClaudeCommand` output, including `--mcp-config` and `--settings` values) nor the output of `buildExtraEnv()` / `claudeSettingsEnvOverrideArgs()`.
  - *Given* a minted proof `P` for session S, *When* the full launch command line and `buildExtraEnv()` are captured, *Then* `strings.Contains(..., P)` is false for both.
- `session` cannot import `server/services`: it receives the proof through an injected provider `SetMCPProofProvider(func(sessionUUID string) (headerValue string, ok bool))` (precedent `SetMCPServerURLProvider`, `session/instance_actor_setters.go:75-81`) wired from `server`; a nil provider or `ok=false` emits no proof.
  - *Given* an `Instance` with no provider, *When* its Claude command is built, *Then* the config has no `X-Stapler-Session-Proof` header and the build does not fail; `go list -deps ./session` does not contain `server/services`.
- `mcpWithUUID` (`server/server.go:1112-1118`) sets a verified identity on the context only when the proof verifies; the unverified `WithSessionUUID` path is unchanged for pre-existing tools.
  - *Given* a request with `X-Stapler-Session-UUID: A` and no proof, *When* it reaches a pre-existing tool, *Then* behavior is unchanged; *When* it reaches any of the five new tools, *Then* the result is `identity_unverified`.
  - *Given* session A's proof and session B's UUID in the same request, *When* verified, *Then* verification fails.
- Explicit degraded behavior (documented in `docs/reference/wake-triggers-and-agent-messaging.md`): sessions spawned before the upgrade, sessions whose proof file write failed, and the in-process `RunStdio` path (`server/mcp/server.go:170`, no HTTP middleware) return `identity_unverified` from the five new tools; the error copy does not say "Set STAPLER_SESSION_UUID in your environment"; `HookProofs.Rotate` revokes MCP proofs and sessions need a respawn.
  - *Given* a session whose `mcp/<uuid>.json` predates the upgrade (no proof header), *When* it calls `list_wakes`, *Then* the result is `identity_unverified` with the "relaunch the session" copy and its pre-existing tools still work.
- The stdio-proxy path (`server/mcp/proxy.go` `ProxyHeaders`, `proxy.go:66-68`) reads the proof from `<configDir>/mcp-proofs/<uuid>` using `STAPLER_SESSION_UUID`; absent file -> no header -> `identity_unverified`.
  - *Given* a proxy process with `STAPLER_SESSION_UUID=S` and the proof file present, *When* `ProxyHeaders()` runs, *Then* it returns `X-Stapler-Session-Proof` that verifies for S; with the file removed it returns no proof header.
- **Spawn-path matrix** (also in `docs/reference/wake-triggers-and-agent-messaging.md`). `mcp-proofs/<uuid>` and the `mcp/<uuid>.json` proof header are minted for Claude-family tmux programs (the programs that go through `buildClaudeCommand`) and, through Task 4.9g, for every other harness whose per-session MCP config ssq controls, as determined by Spike S5 (Task 0.5e). A harness that cannot load an MCP config carrying our credential cannot call the five tools at all, so the fail-closed default for the rest is a consequence, not a choice; requirements and success metric 4 say "harnesses that support MCP with a verified credential". No other program gets a proof file. The matrix is VERIFIED against `session/instance_tmux.go` and `session/headless/runner.go:109-124` by the adversarial re-review 2, and Task 4.9b re-checks it.

| Spawn path | Proof delivery | Five-tool outcome |
|---|---|---|
| Claude tmux: work, hidden, triage/one-shot, and resumed (same `buildClaudeCommand`, `Build(i, base, resumeSessionID)`) | `mcp/<uuid>.json` header plus `mcp-proofs/<uuid>` for the proxy | verified |
| Headless Claude runner (`--strict-mcp-config`, no MCP servers) | none needed, no MCP access | not reachable |
| Non-Claude harness that Spike S5 (Task 0.5e) marks proof-capable (a per-session MCP config path that carries a header read from a 0600 file) | proof file and per-session config written through that harness's path (Task 4.9g) | verified |
| Any other non-Claude harness (S5 FAIL or not measured: pi, Aider, agy, Gemini; OpenCode passed S5 and is the row above, `research/spike-S5.md`) | none minted | `identity_unverified`, fail closed |
| External / IDE sessions (ssq-mux) | none minted | `identity_unverified`, fail closed |
| Nil proof provider (boot window before `SetMCPProofProvider` is wired) | none; the session keeps no proof until relaunch | `identity_unverified`, fail closed |
| In-process `RunStdio` (`server/mcp/server.go:170`) | no HTTP middleware | `identity_unverified`, fail closed |
| Session spawned before the upgrade | no proof header | `identity_unverified` ("relaunch the session" copy) |

  - *Given* a non-Claude session whose harness S5 marked FAIL and an ssq-mux session, *When* either calls `list_wakes` through the proxy, *Then* the result is `identity_unverified` and no `mcp-proofs/<uuid>` file exists for it.
  - *Given* a harness S5 marked proof-capable (conditional on S5; skipped with `t.Skip("blocked on S5: <harness>")` until then), *When* a session of that harness is spawned and calls `list_wakes`, *Then* the verified context carries its UUID, the proof appears in neither argv nor `buildExtraEnv()` for that harness, and a forged UUID header without the proof gets `identity_unverified`.
- **Proof lifetime and liveness**: the proof is a deterministic `HMAC(secret, domain || uuid)`, valid for the session's whole life and until a global `HookProofs.Rotate` (stated in the reference doc; per-session rotation is out of scope). Nothing removes `mcp/*.json` today (VERIFIED by grep: the only `RemoveFile` callers are in `server/services/hook_proof*.go`), so a stale file for a dead UUID would still verify. Therefore (a) on session deletion, `mcp/<uuid>.json` and `mcp-proofs/<uuid>` are deleted, reusing the `EventSessionDeleted` subscription seam of the `GuidanceSessionReaper` (Task 3.3c) or the existing session-delete cleanup, plus a startup sweep of files whose UUID is not a known session; and (b) each of the five tools requires the verified UUID to resolve to a live (existing, non-deleted) session, else `identity_unverified`.
  - *Given* a session with proof files, *When* it is deleted, *Then* both files are gone and a later request carrying its (still HMAC-valid) proof gets `identity_unverified` from the five tools.
  - *Given* a stale `mcp-proofs/<uuid>` for a UUID with no session at startup, *When* the sweep runs, *Then* the file is removed.
- Residual risk is documented honestly and the exfiltration path has a stated decision: accepted risk (ADR-007 rule 0) that a same-user process, or an agent using `run_command` / `steer_session` + `read_session_output` to make another session print a proof file, can obtain a proof; best-effort redaction of the proof header value format in `read_session_output` and `wait_for_output` is a mitigation, not a boundary. **Redaction scope decision (ADR-007 rule 0)**: redaction covers only those two tools. Persisted scrollback, Claude transcripts reachable through `search_claude_history`, `get_session` previews, notification snippets and `get_session_diff` (if a proof file sits in a worktree path) are an **accepted residual**, listed by name in ADR-007 and the reference doc; widening redaction to them is rejected because redaction is not a boundary and a proof that was printed once is already exposed to the same user. If Task 0.7c FAILS, the echoing sink is added to the covered set. UI/doc copy never claims `:8543` authenticates the operator.
  - *Given* the reference doc, *When* grepped, *Then* it contains the residual-risk paragraph naming the same-user file read, `tmux show-environment` (why env is not used) and the `steer_session`/`run_command` + `read_session_output` route.
  - *Given* a session pane that printed a proof header value, *When* `read_session_output` returns it, *Then* the value is replaced with `[redacted-proof]`.
**Files**: `server/services/mcp_proof.go` (new, reusing `HookProofs`), `server/server.go` (middleware, one block), `server/mcp/proxy.go`, `server/mcp/server.go`, `session/instance_tmux.go` (`claudeMCPConfigArgsWithToken`), `session/instance_actor_setters.go` (provider setter), `server/dependencies.go` (provider wiring), `server/mcp/identity_context.go` (new; `server/mcp/context.go` does not exist, `WithSessionUUID` is at `server/mcp/tools_backlog.go:32`), `server/mcp/tools_terminal.go` (redaction only), `server/services/guidance_session_reaper.go` or the existing session-delete cleanup (proof-file deletion), `pkg/events/types.go` (`EventSessionDeleted`, read only), tests

##### Task 4.9a: `McpProofs` (domain `ssq-mcp-v1`) over the existing secret and per-session files (`mcp-proofs/<uuid>`), plus unit tests (S)
##### Task 4.9b: `SetMCPProofProvider` in `session` (`session/instance_actor_setters.go`), its one-line wiring from `server` (`server/dependencies.go`), and minting and writing the 0600 proof files at spawn (`server/services/mcp_proof.go` plus its test) (M)
- Files: `session/instance_actor_setters.go`, `server/dependencies.go` (one wiring line), `server/services/mcp_proof.go`, `server/services/mcp_proof_test.go`
##### Task 4.9i: Stdio-proxy header read: `ProxyHeaders` reads the proof from `<configDir>/mcp-proofs/<uuid>` (`server/mcp/proxy.go`, `server/mcp/proxy_test.go`), and update the existing injection tests for the provider (`session/instance_tmux_test.go`) (S)
##### Task 4.9c: Verify in `mcpWithUUID`; `WithVerifiedSessionUUID` / `verifiedCallerSessionUUID(ctx)` in the new `identity_context.go`; middleware forged-header and cross-session tests (S)
##### Task 4.9d: `claudeMCPConfigArgsWithToken` always-file route for proof-bearing configs; the write-failure branch at `instance_tmux.go:677-679` fails closed (the proof is never in the map used for the inline fallback) with the forced-write-failure test (`session/instance_tmux.go`, `session/instance_tmux_test.go`) (M)
- Files: `session/instance_tmux.go`, `session/instance_tmux_test.go`
##### Task 4.9j: Spawn-path verification tests and copy: spawned-session verify, argv-non-leak and env-non-leak, pre-upgrade-session and stdio-path `identity_unverified`, and the error copy (`server/mcp/caller_identity_gate.go` copy string, `server/mcp/spawn_path_matrix_test.go`, `session/instance_tmux_nonleak_test.go`) (S)
##### Task 4.9e: Best-effort proof-value redaction in `read_session_output` / `wait_for_output` output, and the residual-risk section of the reference doc (S); the section lists scrollback, `search_claude_history`, `get_session` previews, notification snippets and `get_session_diff` as accepted residual (not redacted)
##### Task 4.9f: Proof lifetime: delete `mcp/<uuid>.json` and `mcp-proofs/<uuid>` on `EventSessionDeleted` (default: the existing session-delete cleanup seam, because 4.9 lands in W1 and the Task 3.3c reaper in W3; reuse the reaper subscription only if 3.3c has already merged) with a startup sweep, require a live session in the five tools, and the spawn-path matrix test (non-Claude harnesses that S5 did not pass, and ssq-mux, get no proof file) (S)
##### Task 4.9g: **Conditional on S5, per harness**: per-session MCP config and proof-file delivery for each non-Claude harness Spike S5 marks proof-capable, following the 0600-file, never-argv, never-tmux-env rule; extend the matrix test and the non-leak tests to that harness; harnesses S5 fails stay fail-closed (M)
- Files: `server/services/mcp_proof.go`, `server/mcp/spawn_path_matrix_test.go`, `session/instance_tmux_nonleak_test.go`, one per-harness spawn file under `session/` for each harness S5 passes
##### Task 4.9h: Startup self-check for the credential path (mint, write, verify round trip for a synthetic UUID, log `mcp credential self-check ok`) and a `proof_present` boolean on `get_session` (never the value) so "the tools aren't working" is diagnosable per session (pre-mortem #13) (S)

### Story 4.10: Operator error states and exit paths for Scheduled prompts, Remind me and peer help
**As an** operator, **I want** every failure and stuck state in the new surfaces to offer a way out, **so that** none ends in a message with no control (UX-AC32, 35-41, 55; `design/ux.md` section 8).
**Acceptance Criteria**:
- Scheduled prompts list load failure and missed-prompt rows.
  - *Given* `ListWakeTriggers` fails, *When* the section renders, *Then* it shows "Couldn't load scheduled prompts. Retry" and Retry re-issues the call; *Given* reconcile marked a trigger `missed`, *When* the section renders, *Then* an exception row "Missed: {reason}" with warning icon and text offers Cancel and Open owner.
- Paused scheduled prompts have an in-pane resume control.
  - *Given* `agent_wake_triggers` off, *When* the section renders, *Then* "Scheduled prompts paused. Agents will not be prompted automatically." shows with a "Resume scheduled prompts" button (inline confirm, calls `UpdateFeatureFlag`) and a link to flag settings; Cancel still works and rows are labelled "Paused".
- Remind me failure states (last wave; default presets).
  - *Given* `schedule` fails, *When* a preset is chosen, *Then* "Couldn't set the reminder. Retry"; *Given* the cap is exceeded, *Then* "Too many pending reminders. Cancel one in Scheduled prompts." with a link; *Given* a picked time in the past, *Then* inline "Choose a time in the future" and re-pick; *Given* a duplicate tap, *Then* a single trigger exists (idempotent dedup key).
- Stalled peer request: Cancel request and Answer for {peer}.
  - *Given* peer help with no reply in 10 min or a stopped peer, *When* the `PeerHelpEscalator` (Story 4.11) escalates it, *Then* it appears in Needs you with Answer for {peer}, Cancel request and Open; Cancel request (Tier A `CANCEL_PEER_HELP`) cancels the row and the asker's `guidance_answered` wake delivers "cancelled"; Answer for {peer} opens the UX-S3 card (the `GuidanceQuestionForm`) with the operator as answerer through the existing guidance answer path; each failure shows an inline error with Retry.
  - *Given* the peer answered first, *When* the operator answers, *Then* the row shows "Already answered by {session}: {value}".
- Remind me confirmation and Undo (UX-AC40): picking a preset shows "Reminder set for {time}" with an Undo button for 10 s, after which the reminder persists in Scheduled prompts; Undo cancels the trigger.
  - *Given* a preset picked at 13:10, *When* the confirmation renders, *Then* it reads "Reminder set for 14:10" with Undo; *When* Undo is activated within 10 s, *Then* the trigger is `cancelled`; *When* 10 s of fake time pass first, *Then* Undo is removed and the reminder is listed in Scheduled prompts.
- Session reaped between polls.
  - *Given* a Scheduled prompts or Background row whose owner session is deleted between polls, *When* Open is activated, *Then* the row shows "Session no longer exists" (reuses Task 1.6c) and Cancel remains.
**Files**: `web-app/src/components/ui/ScheduledPrompts.tsx`, `RemindMeMenu.tsx` (`PeerHelpRow.tsx` is created in Task 4.11a; 4.10d consumes it), `server/services/wake_trigger_service.go`, `server/services/notification_actions.go` (cancel descriptors), `tests/e2e/agent-wake-triggers.spec.ts`

##### Task 4.10a: Scheduled prompts load-failure Retry and missed-prompt exception rows (S)
##### Task 4.10b: Remind-me create/cap/past-time/duplicate error states and the 10 s Undo (S) - last wave
##### Task 4.10c: "Resume scheduled prompts" control with inline confirm and audit (S)
##### Task 4.10d: Peer-help stalled-row exits: `CANCEL_PEER_HELP` descriptor/handler, Answer for {peer} via the UX-S3 card, already-answered race copy (M)
- Files: `web-app/src/components/ui/PeerHelpRow.tsx`, `server/services/notification_actions.go`, `server/services/wake_trigger_service.go`, `tests/e2e/agent-wake-triggers.spec.ts`

### Story 4.11: Peer-help operator surface and escalation
**As an** operator, **I want** to see help requests between agents and be pulled in only when one stalls, **so that** peer help is visible without adding noise (UX-AC31, UX-AC33, UX-AC34; `design/ux.md` UX-S4). This story is the owner of the whole peer-help UX that Stories 4.5 and 4.10 only referenced, including the 10-minute escalation emitter the stalled-row AC in Story 4.10 depends on (consistency C7).
**Acceptance Criteria**:
- Accepted requests render as `PeerHelpRow` in the Background feed, Recent activity and Scheduled prompts, with asker, answerer, question, age and state (waiting / answered by {session} / not sent / cancelled), and do not increment the Needs-you badge before escalation.
  - *Given* request R from A to B accepted 3 minutes ago, *When* the Background tab renders, *Then* a row shows "A asked B", the question, "waiting 3m" and "Waiting for B", and the Needs-you badge count is unchanged.
- The `PeerHelpEscalator` (`server/wakes/escalator.go`) holds one deadline per open peer row at `created_at + 10 min`, derived from durable rows and re-armed at startup on the dispatcher's `Clock` (no poll loop), and escalates immediately when the answerer session is stopped, archived or deleted (event subscription plus a startup pass). Escalation raises one needs-human (`INPUT_REQUIRED`) notification whose descriptors are `ANSWER_GUIDANCE`, `CANCEL_PEER_HELP` and `OPEN`, dedup-keyed on the request id so a restart or a second pass never duplicates it.
  - *Given* an open peer row and a fake clock, *When* 10 minutes pass with no answer, *Then* exactly one `INPUT_REQUIRED` notification exists with the three descriptors, `deliverygate.ShouldDeliver` delivers it for a hidden asker, and a restart plus a second pass creates no second notification.
  - *Given* the answerer is stopped, archived or deleted, *When* the event arrives, *Then* escalation is immediate with the plain-language reason ("Peer is stopped. Not delivered unless resumed."); *Given* the peer answers at minute 4, *Then* no escalation occurs and the deadline is dropped.
- Requests rejected at call time (self-target, archived, one-shot, cross-workspace) return a typed `rejected` result with the reason to the asker and create no row; only an accepted request that later becomes undeliverable shows "Not sent: {reason}" (UX-AC33). Both count in `peer_help_total`.
  - *Given* a `request_help` aimed at an archived peer, *When* handled, *Then* the asker gets `rejected(reason=peer_archived)`, zero rows exist and `peer_help_total{state="rejected"}` is 1; *Given* an accepted request whose answerer is then deleted, *Then* the row reads "Not sent: peer no longer exists" and escalates.
- Escalation presentation (UX-AC80): the escalated row raises the bell badge, produces exactly one polite announcement "1 item needs a decision", never moves focus, respects the engaged-card freeze (it waits behind "N new" while a card is engaged), and its first line distinguishes "{asker} asked {peer}: no reply in 10m" from a question the operator's own sessions asked; "Answer for {peer}" delivers to the asker with provenance `from=operator on-behalf-of={peer}`.
  - *Given* a card engaged and an escalation arriving, *When* the notification is persisted, *Then* the badge count is +1, the polite region text is "1 item needs a decision" once, `document.activeElement` is unchanged, the list order is unchanged and "1 new" is present; *Given* the operator uses "Answer for diagnose:abc", *Then* the asker's wake payload contains `on-behalf-of=diagnose:abc`.
- An answered request shows "Answered by {session}" with the provenance text and moves to Recent activity without operator action (UX-AC34).
  - *Given* B answers R, *When* the row updates, *Then* it reads "Answered by B" and appears under Recent activity, not Needs you.
**Files**: `web-app/src/components/ui/PeerHelpRow.tsx`, `PeerHelpRow.test.tsx`, `server/wakes/escalator.go`, `server/wakes/escalator_test.go`, `server/mcp/tools_peer_help.go` (rejection semantics only), `server/deliverygate/policy_test.go` (gate-class table row)

##### Task 4.11a: `PeerHelpRow` and its Background / Recent / Scheduled prompts integration with the badge rule (M)
- Files: `web-app/src/components/ui/PeerHelpRow.tsx`, `web-app/src/components/ui/PeerHelpRow.test.tsx`, `web-app/src/components/ui/BackgroundActivity.tsx`, `web-app/src/components/ui/ScheduledPrompts.tsx`
##### Task 4.11b: `PeerHelpEscalator`: deadlines on the `Clock`, startup re-arm, immediate escalation on answerer end, deduplicated needs-human notification and gate-class table row (M)
- Files: `server/wakes/escalator.go`, `server/wakes/escalator_test.go`, `server/deliverygate/policy_test.go`
##### Task 4.11c: Call-time rejection semantics (typed `rejected`, no row), `peer_help_total` states and the accepted-then-undeliverable "Not sent" row (S)
##### Task 4.11d: Escalation presentation (badge, one announcement, no focus move, freeze-aware) and the operator-on-behalf-of provenance (S)

---

# Epic 5: Observability, documentation and rollout readiness

**Goal**: Make the metrics and alerts real, document operation, and verify each flag on the isolated instance before any default flip.

### Story 5.1: Metrics, logs and alerts
**As an** operator, **I want** the counters in the Observability Plan exported, **so that** I can see failures.
**Acceptance Criteria**:
- Each metric listed in the Observability Plan is registered and incremented by a test exercising the corresponding path.
  - *Given* a test that invokes `APPROVE` then `ANSWER_QUESTION` (stale), *When* metrics are scraped, *Then* `notification_inline_action_total{kind="APPROVE",outcome="APPLIED"}` is 1 and `{kind="ANSWER_QUESTION",outcome="STALE"}` is 1.
- The missed-wake alert fires an operator notification after a restart that misses a trigger.
  - *Given* Story 4.8's restart test with one expired trigger, *When* reconcile completes, *Then* a failure-class operator notification references the trigger id.
- The paired quality metrics exist and are reported with the adoption counters (`requirements.md`): delivery-failure rate, wrong-answer proxy (`reopen_after_action_total`, `inline_action_undo_total`), Unconfirmed rate, and the in-pane vs. in-session resolution share; each has a documented query in the reference doc and is exported before any default flip.
  - *Given* a test that performs 10 answers of which 1 returns `NOT_SENT`, 1 Undo and 1 session open within 60 s of an action, *When* metrics are scraped, *Then* the delivery-failure rate query yields 0.1, `inline_action_undo_total` is 1 and `reopen_after_action_total` is 1.
- Scheduling: Tasks 5.1a and 5.1d land in wave W4 (after Story 2.3, with Story 2.5) so the action, delivery and in-pane-share counters exist from the first inline-action release and adoption is measurable in shipping order; Task 5.1e lands in W5 after Story 4.3 and starts the 7-day baseline window of Task 5.1f (which therefore completes during W6 to W8 and long before the last flag's soak); the rest of the story is W7.
- The Epic 4 cost outcome exists at runtime, not only as a Go grep guard (`requirements.md` Adoption targets).
  - *Given* a session that ends after 3 wake deliveries and 2 `ScheduleWakeup` PostToolUse receipts, *When* metrics are scraped, *Then* `wake_fires_per_session` has a sample of 3 and `agent_schedule_wakeup_calls_total{role}` is 2.
- The baseline against which that outcome is judged exists before the flag does (Task 5.1f): at least 7 days of the `ScheduleWakeup` counter with `agent_wake_triggers` OFF, recorded in the reference doc, and the first `agent_wake_triggers` flip PR cites it.
  - *Given* the reference doc, *When* grepped before the first `agent_wake_triggers` flip PR, *Then* it contains a baseline row with a session count and a capture window of at least 7 days; the flip PR template (Task 5.3b) has a line that fails review if the row is missing.
**Files**: `server/services/notification_action_metrics.go`, `server/wakes/metrics.go`, `server/deliverygate/stats.go` (read-only reference), `docs/reference/notification-actions-observability.md`

##### Task 5.1a: Register and increment action/delivery/latency metrics (S)
##### Task 5.1b: Register wake/peer/gate metrics and the two alert conditions (S)
##### Task 5.1c: Reference doc with metric names and example queries (XS)
##### Task 5.1d: Quality and adoption metrics: `reopen_after_action_total`, `inline_action_undo_total`, in-pane vs in-session share, delivery-failure and Unconfirmed rates, with the scrape test (S)
##### Task 5.1e: Epic 4 cost outcome at runtime: `wake_fires_per_session` recorded when a session ends and `agent_schedule_wakeup_calls_total{role}` from the PostToolUse receipt's tool name (extend the injected matcher like Task 3.4e if Task 0.7e finds it missing), documented query and scrape test (S)
##### Task 5.1f: Capture the Epic 4 baseline before the flag is on: with `agent_wake_triggers` OFF and Task 5.1e deployed, record at least 7 days of `agent_schedule_wakeup_calls_total{role}` per backlog session in `docs/reference/notification-actions-observability.md` (median, p90, session count). If Task 0.7e found the matcher does not cover `ScheduleWakeup`, 5.1e's matcher extension is the fallback instrument; if that also fails the metric is reported as "no instrument" and flip criterion (iv) in `requirements.md` is dropped by the owner, not silently (XS)

### Story 5.2: Documentation and registry
**As a** future maintainer, **I want** how-to, reference and explanation docs, **so that** the system is operable.
**Acceptance Criteria**:
- The inline-actions how-to states the away-operator dependency (alerting depends on the out-of-scope Slack and web-push channels, delivery not verified here; UX-AC78), the explicit Tier A/B list and the vocabulary table.
  - *Given* `docs/how-to/use-inline-notification-actions.md`, *When* grepped, *Then* it contains the dependency paragraph and the tier table.
- Docs exist under `docs/how-to`, `docs/reference`, `docs/explanation` and are indexed in `CLAUDE.md`'s reference table; `make registry-generate` is clean.
  - *Given* the merged work, *When* `make registry-diff` runs, *Then* it reports no pending changes and CLAUDE.md contains rows for the three new docs.
**Files**: `docs/how-to/use-inline-notification-actions.md`, `docs/reference/wake-triggers-and-agent-messaging.md`, `docs/explanation/askuserquestion-answer-routing.md`, `CLAUDE.md`, `docs/registry/features/`

##### Task 5.2a: Write the three docs (S)
##### Task 5.2b: CLAUDE.md index rows and `make registry-generate` (XS)

### Story 5.3: Rollout readiness on the isolated instance
**As an** operator, **I want** each flag verified against a real instance, **so that** defaults are flipped on evidence.
**Acceptance Criteria**:
- A checklist run on the manual instance (`PORT=62871`, `--remote-port 62872`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`) covers: Background shows rows; approve, question, backlog decision inline; structured question answered on a live session; wake survives a restart; each flag flips live and rolls back; output pasted into the PR description.
  - *Given* the manual build, *When* the checklist is executed, *Then* each item records command output and `ss -ltnp | grep -E '62871|62872'` showed free ports beforehand.
- The checklist also records the UX sweeps that automation cannot prove: every error or terminal state in `design/ux.md` sections 1.3, 2.3, 3.3, 4.3, 5.3 and 6.3 offers an exit control (UX-AC55, with the parametrized `ux-error-exits.spec.ts` as the automated half); copy review plus `copyGuard.test.ts` confirm no copy claims local authentication or delivery to an archived session (UX-AC58); real-device soft-keyboard, one-scroll-region and focus-ring checks on iOS Safari and Android Chrome (UX-AC43, UX-AC46, UX-AC54) and icon/border contrast measurement (UX-AC52).
  - *Given* the manual build, *When* the UX sweep is run, *Then* each of those items has a recorded pass or a filed defect.
- The usability validation plan (`design/ux.md` section 12, tasks T1 to T10 on desktop and phone; T8 to T10 are the hostile trials: fling-tap, arrival under the thumb, app switch mid-hold) is executed per flag and recorded; a flip follows the "Decision rule for flag defaults" in `requirements.md` (14 days of soak; for the two answer flags the last 7 days of the three quality metrics and no task failing twice in a row; for `background_activity_feed` and `agent_wake_triggers` their own criteria there, including the Task 5.1f baseline). The Claude Code layout canary (Task 3.3h) is checked against the pinned fixture version after any Claude Code update.
  - *Given* the manual build and the T1-T10 script, *When* the operator runs it, *Then* each task has a recorded pass or a filed defect and the quality metrics for the last 7 days are pasted.
- Default flips (`notification_inline_actions`, later others) are separate one-line PRs after the operator's soak.
**Files**: `docs/how-to/verify-notification-actions-rollout.md`

##### Task 5.3a: Execute and record the checklist (M)
- Files: `docs/how-to/verify-notification-actions-rollout.md`
##### Task 5.3b: Flag default-flip PR template, with a per-flag criteria checklist from `requirements.md` and the Task 5.1f baseline-row check for `agent_wake_triggers` (XS)
##### Task 5.3c: Run the usability script T1-T10 and the 14-day soak quality-metric review before each flip (S)

### Story 5.4: CI hygiene gates
**As a** maintainer, **I want** the known gates to pass before each PR, **so that** CI does not bounce.
**Acceptance Criteria**:
- `make ready` passes; `pnpm run lint:duplicates` under 0.14%; e2e convention checks (`@feature` header, no `waitForTimeout`, no CSS-class locators) pass; generated ent/proto output is not tracked.
- Duplication budget per web story (pre-mortem #11): `jscpd` is an absolute ratchet with no per-diff scoping, and about 12 new components with heavily mocked tests land in parallel worktrees, so each PR can pass alone and the last merge fail. The measured margin (Task 0.7d; the 0.10% in memory is UNVERIFIED) is divided into the budget table below, shared mocks live in `web-app/src/components/ui/__fixtures__/` and root `__mocks__`, and each web PR rebases onto main and re-runs `pnpm run lint:duplicates` immediately before merge (not only at branch creation). The `dupl` check on the five similar MCP tool handlers is addressed by a shared handler-builder in `server/mcp/tools_wake.go`/`tools_peer_help.go`.
  - *Given* a web PR, *When* it is about to merge, *Then* `pnpm run lint:duplicates` was run on the rebased tree and its reported percentage is in the PR body; *Given* the budget table, *Then* the sum of the per-story budgets is <= the measured margin.

  | Web story | Budget (share of the measured margin) |
  |---|---|
  | 1.3, 1.6, 1.8 (Background UI, banner, governance) | 20% |
  | 2.5, 2.6, 2.10 (ActionRow, a11y, interaction safety) | 30% |
  | 3.7 (GuidanceQuestionForm family) | 20% |
  | 4.6, 4.10, 4.11 (Scheduled prompts, Remind me, PeerHelpRow) | 20% |
  | slack (rebase drift, fixes) | 10% |
- Known flaky suites do not block new-code signal and are never silently skipped (CI-isolation approach below).
  - *Given* a CI run where `TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown` flakes, *When* the new-code step runs, *Then* it passes independently and the flake is attributed per the rule below.

  **Known flaky suites near this work (cited from `docs/bugs/open/`; the bug numbers are reused across files, so each is cited by file).** (1) BUG-067 `docs/bugs/open/BUG-067-server-services-race-suite-exceeds-150s-ci-timeout.md`: the `server/services` race suite runs long. (2) BUG-099 `docs/bugs/open/BUG-099-server-services-package-slow-enough-to-blow-ci-job-budgets.md`: the same package's ~500 s runtime intermittently blows CI job budgets; also `docs/bugs/open/BUG-099-retrysession-permanentlyfailed-instance-status-race-under-full-suite.md` (`server/services/session_service_retry_test.go`) and `docs/bugs/open/BUG-099-retry-session-permanently-failed-test-helper-race.md`. (3) BUG-090 `docs/bugs/open/BUG-090-hubregistry-restartpump-reconnect-flake-under-full-suite-load.md` (`server/services/connectrpc_websocket_test.go`). (4) BUG-079 `docs/bugs/open/BUG-079-jest-full-suite-only-failures-stuckitem-websocket-terminal-backlogdetail.md` (Jest: the StuckItem, WebSocket terminal and BacklogDetail suites fail only in a full `npx jest` run) and `docs/bugs/open/BUG-079-autonomousdriver-idle-timer-race-under-full-suite.md` (`session`).

  **Isolation approach.** (a) New wake code lives in its own package `server/wakes`, so it is not subject to the `server/services` package runtime or its flakes. (b) New `server/services` tests use a stable name prefix and run as a required, separate CI step: `go test ./server/services -race -run 'NotificationAction|BuildDescriptors|QuestionAnswerRouter|PaneReplyCore|GuidanceMirror|GuidanceReaper|GuidanceSampler|WakeTrigger|FlagMatrix|HumanActivity' -timeout=20m`, so a flake elsewhere in the package neither blocks nor hides them. (c) New Jest tests run via `--testPathPatterns` for the new components (the Validation coverage command), never relying on the full-suite run that BUG-079 breaks. (d) A failure in a known-flaky test is first re-run in isolation (`go test ./server/services -run '^TestName$' -count=3 -race`); a pass in isolation is recorded and attributed to the bug, a fail is treated as real. (e) Before blaming or fixing a red full-suite run, compare it with main's own runs (`instinct_ci_failure_check_main_first`). (f) This project does not edit, skip or retry the four flaky tests; fixing them follows `fix-flaky-tests-dont-defer` as its own commit, not as part of a feature PR.
  - *Given* each epic's final branch, *When* `make ready`, `pnpm run lint:duplicates`, and `git ls-files session/ent | grep -v schema | grep -v generate.go` run, *Then* the first two pass and the third prints nothing.
**Files**: none (verification story applied per epic)

##### Task 5.4a: Per-epic gate run recorded in each PR (S)

---

## Pre-mortem P2/P3 disposition (rows #5 to #20 of `implementation/pre-mortem.md`)

Each row has an owning task or AC that exists in this plan; "accepted" rows state the accepted risk and what makes it detectable.

| # | Failure mode | Owner (task / AC) | Status |
|---|---|---|---|
| 5 | Claude Code update drifts the dialog layout; every `DialogMatch` fails closed | Story 3.3 AC "Layout-drift canary", Task 3.3h; Observability alert; Task 5.3 checklist re-check after updates; Spike S7 item (9) | Folded |
| 6 | Reconcile and re-enable notification flood | Story 4.3 AC "Reconcile and re-enable notifications are digested", Task 4.3i | Folded |
| 7 | Hook-path mirror write slows or errors the hook | Story 3.3 AC "The hook-path mirror fails open", Task 3.3i | Folded |
| 8 | Background feed grows without bound | Story 1.8 (Older collapse, 100-row cap, 500-row test), Task 1.8d | Folded |
| 9 | Lost or duplicated wakes in the delivery gap; busy-owner expiry | Story 4.3 AC "Duplicate delivery is a contract", Task 4.3j; Wake limits row `busy_hold` | Folded |
| 10 | Inconsistent flags across scopes; ADR-008 contradiction | "Flag precedence and dependency matrix", Story 0.8 AC + Task 0.8d, ADR-008 rewrite | Folded |
| 11 | jscpd/dupl/gocognit break across the parallel PR train | Story 5.4 AC (budget table, rebase-and-recheck), Task 0.7d (measure baseline) | Folded; baseline UNVERIFIED until Task 0.7d |
| 12 | Epic 4 overruns or stalls; review-repair loops overbuild | Epic 4 checkpoint (Story 4.2), overrun checkpoint (Effort); **stop rule**: a story needing a third repair round is simplified or cut back to its last mergeable flag-off slice, not patched again | Folded |
| 13 | Verified-credential path rejects legitimate callers | Story 4.9 spawn-path matrix, Task 4.9h (startup self-check, `proof_present` on `get_session`) | Folded |
| 14 | `needs_revive` stalls; router `resume` route outside `lifecycleFiles` | Story 4.3 AC (needs_revive age alert), Task 4.3k; Task 4.3f (adds `server/services/question_answer_router.go`); Story 3.4 AC and Task 3.4d (the `resume` route never revives, decided, spy-tested) | Folded and closed |
| 15 | Descriptor assembly misses the 200 ms budget | Story 2.2 AC and Task 2.2e (batching, benchmark) | Already covered |
| 16 | `human_active` has no defined signal | Story 3.4 AC (`HumanActivity` definition and three tests), Task 3.4f; ADR-004; Spike S7 item (10) | Folded |
| 17 | Same-user agent impersonates a session or the UI | Accepted risk (ADR-004, ADR-007 rule 0); Story 2.3 AC adds `channel`/`caller_kind` to the audit so it is queryable | Accepted + detectable |
| 18 | Secrets in `questions_json` outlive the session | Story 3.3 AC and Task 3.3j (sanitize, secret scan, 30-day purge) | Folded |
| 19 | Bounded blocking hook removes the terminal path | Story 3.4 AC "Bounded-blocking hook fails open", Task 3.4c; ADR-005 | Folded |
| 20 | Cron timezone and DST | Story 4.2 AC and Task 4.2d | Folded |

## UX acceptance criteria trace

Every `design/ux.md` criterion (88, UX-AC1 to UX-AC88) has one primary owning story whose acceptance criteria assert it; stories that only consume the behavior are not listed. The Phase 4 consistency check found nine criteria with no plan AC and they were assigned; the triad round 1 repair added UX-AC60 to UX-AC88 and changed UX-AC1, 7, 11, 15, 18, 23, 24, 36, 48, 49, 50, 51, 53, 56.

| Owning story (task) | UX-ACs |
|---|---|
| 1.3 | 1, 2, 3, 4, 7, 46 (100-row cap with "Show all (N)" is in the same story) |
| 1.6 (Tasks 1.6a-e) | 5, 6, 56, 73 |
| 1.8 | 66, 67, 68, 69, 70, 71, 72 |
| 2.5 (Tasks 2.5d-f) | 8, 9, 10, 11, 12, 14, 15, 16, 17, 64, 65, 76 (Invalid/Refresh and the auto-handled STALE variant are in Task 2.5d) |
| 2.8 (Task 2.8c) | 13, 53, 77 |
| 2.6 (Tasks 2.6a-e) | 30, 42, 43, 44, 45, 47, 48, 50, 51, 52, 54, 74, 75, 81, 82, 83, 84, 85, 86, 87, 88 |
| 2.7 | 25 |
| 2.9 | 57 |
| 2.10 (Tasks 2.10a-d) | 60, 61, 62, 63 |
| 3.3 (Tasks 3.3f, 3.3g) | 26, 59 |
| 3.4 (Tasks 3.4e-g) | 24, 27, 49 (focus-only digits; the guard itself is Story 2.10) |
| 3.5 | 29 |
| 3.7 (Tasks 3.7d-f) | 18, 19, 20, 21, 22, 23, 28, 79 |
| 4.6 (Task 4.6d) | 35, 36, 37 (20-row cap with "Show all (N)" is in the same task) |
| 4.10 | 32, 38, 39, 40 (Undo), 41 |
| 4.11 (Task 4.11d) | 31, 33, 34, 80 |
| 5.2 | 78 |
| 5.3 | 55, 58 |

Coverage check (run after the last edit): the union of the UX-AC numbers in this table is exactly 1 to 88 with no number twice (UX-AC49 is listed once, under 3.4; the 2.10 guard is asserted through UX-AC61).

## Effort estimate

Basis: bands and weights INFERRED from `ESTIMATION.md`; recalibrate after the first implementer run. Task classes: XS 20k, S 55k, M 130k raw tokens (class midpoints). Assumed mix per implementation run: 12% output (x5), 28% fresh input (x1), 60% cache read (x0.1) gives CU = 0.94 x raw. Verification multiplier x1.75 (range 1.5-2.0) per story for spec/quality review, verify layers and CI reruns.

| Story | Tasks | Class mix | Raw tokens (k) | Output share | CU (k) = 0.94 x raw | Wall-clock blocker |
|---|---|---|---|---|---|---|
| 0.1 S1 | 4 | M 2S XS | 260 | 12% | 244 | live Claude session (real-time waits); result reviewed by Tyler (owner item 8) |
| 0.2 S2 | 2 | S XS | 75 | 12% | 70 | live session; a 300 s hook wait |
| 0.3 S3 | 2 | M XS | 150 | 12% | 141 | live session; Channels is a research preview (owner item 8) |
| 0.4 S4 | 2 | S XS | 75 | 12% | 70 | test-repo PR/CI run |
| 0.5 S5 | 5 | 4S XS | 240 | 12% | 226 | live sessions and installs per harness; feeds owner item 9 |
| 0.6 S6 | 2 | S XS | 75 | 12% | 70 | - |
| 0.7 S7 | 7 | 6S XS | 350 | 12% | 329 | live manual-instance probes (hook matcher, tmux attach, `claude --debug`) |
| 0.8 flags | 4 | 4S | 220 | 12% | 207 | - |
| 1.1 | 2 | 2S | 110 | 12% | 103 | - |
| 1.2 | 3 | 3S | 165 | 12% | 155 | - |
| 1.3 | 3 | M 2S | 240 | 12% | 226 | - |
| 1.4 | 2 | M XS | 150 | 12% | 141 | - |
| 1.5 | 2 | S XS | 75 | 12% | 70 | - |
| 1.6 | 5 | M 3S XS | 315 | 12% | 296 | - |
| 1.7 outcome field | 3 | M 2S | 240 | 12% | 226 | - |
| 1.8 governance | 4 | M 3S | 295 | 12% | 277 | - |
| 2.1 | 2 | S XS | 75 | 12% | 70 | - |
| 2.2 | 5 | 2M 3S | 425 | 12% | 399 | - |
| 2.3 | 5 | 2M 3S | 425 | 12% | 399 | - |
| 2.4 | 3 | M S XS | 205 | 12% | 193 | - |
| 2.5 | 6 | 2M 4S | 480 | 12% | 451 | - |
| 2.6 | 5 | M 4S | 350 | 12% | 329 | - |
| 2.7 | 3 | M 2S | 240 | 12% | 226 | - |
| 2.8 | 3 | M 2S | 240 | 12% | 226 | S6 result before any Tier B promotion (default stays desktop-only) |
| 2.9 | 3 | 2M XS | 280 | 12% | 263 | - |
| 2.10 interaction safety | 4 | M 3S | 295 | 12% | 277 | - |
| 3.1 | 5 | 2M 3S | 425 | 12% | 399 | - |
| 3.2 | 2 | M XS | 150 | 12% | 141 | - |
| 3.3 | 10 | 5M 5S | 925 | 12% | 869 | - |
| 3.4 | 7 | 5M 2S | 760 | 12% | 714 | Task 3.4c waits on S1, S2 and owner item 4 |
| 3.5 | 3 | 2M S | 315 | 12% | 296 | S1c result |
| 3.6 | 3 | 3M | 390 | 12% | 367 | S5 per harness (3.6c last) |
| 3.7 | 6 | 3M 3S | 555 | 12% | 522 | - |
| 3.8 | 1 | S | 55 | 12% | 52 | - |
| 4.1 | 4 | M 3S | 295 | 12% | 277 | - |
| 4.2 timer+Clock | 4 | M 3S | 295 | 12% | 277 | - |
| 4.3 | 12 | 5M 6S XS | 1000 | 12% | 940 | Task 4.3c Channels transport waits on S3 |
| 4.4 | 4 | M 3S | 295 | 12% | 277 | S4 result for Task 4.4a |
| 4.5 | 4 | 3M S | 445 | 12% | 418 | Task 4.5a merge waits on owner item 5 (role allowlist) |
| 4.6 | 4 | M 3S | 295 | 12% | 277 | - |
| 4.7 | 2 | S XS | 75 | 12% | 70 | - |
| 4.8 | 4 | 3M S | 445 | 12% | 418 | - |
| 4.9 credential | 10 | 3M 7S | 775 | 12% | 728 | Task 4.9g waits on S5 (owner item 9) |
| 4.10 exits | 4 | M 3S | 295 | 12% | 277 | - |
| 4.11 peer-help UX | 4 | 2M 2S | 370 | 12% | 348 | - |
| 5.1 | 6 | 4S 2XS | 260 | 12% | 244 | Task 5.1f needs a 7-day window of the `ScheduleWakeup` counter with `agent_wake_triggers` OFF |
| 5.2 | 2 | S XS | 75 | 12% | 70 | - |
| 5.3 | 3 | M S XS | 205 | 12% | 193 | 14-day soak per flag, real-device checks, deploy window |
| 5.4 | 1 | S | 55 | 12% | 52 | - |

Every row uses the same 12% output share so one factor (0.94) applies throughout. The spike rows (0.1 to 0.7) read more than they write, so an 8% share would give 0.756 for them; using 0.94 overstates those rows by about 225k CU, roughly 1.6% of the total, and the overstatement is accepted (INFERRED) rather than carrying two factors.

Epic subtotals (raw tokens k / CU k at x0.94 / CU k with x1.75 verification):

| Epic | Raw | CU | CU with verification |
|---|---|---|---|
| 0 Spikes + flags | 1,445 | 1,358 | 2,377 |
| 1 Background | 1,590 | 1,495 | 2,616 |
| 2 Inline actions | 3,015 | 2,834 | 4,960 |
| 3 AskUserQuestion | 3,575 | 3,361 | 5,882 |
| 4 Messaging + wakes | 4,585 | 4,310 | 7,543 |
| 5 Hardening + docs | 595 | 559 | 978 |
| **Total** | **14,805** | **13,917** | **24,356** |

Each epic's raw cell is the sum of its stories' raw cells (recounted by script from the `(XS|S|M)` class on every task heading; the same script reproduced every unchanged story row exactly); CU is 0.94 x the epic's raw, and the verification column is that CU x 1.75, both rounded half-up per epic. The total row is the sum of the per-epic rounded cells; the unrounded product is 14,805k x 0.94 x 1.75 = 24,354k, so per-epic rounding accounts for the 2k difference. The story-level CU cells are informational and are not summed (a x.5 value rounds down in them, for example 75k -> 70k).

Size band (informational, per ESTIMATION.md "Scope default"): total about 24.4M CU at x1.75 (20.9M at x1.5, 27.8M at x2.0), i.e. XL; `requirements.md` quotes the same band. History of the raw total: round 1 added 3 stories and 24 tasks (8,790k -> 10,600k; Epic 4 2,205k -> 3,145k, +43%, because the dispatcher timer and `Clock`, the claim/delivery split, the credential, reconcile predicates, the lint extension and the operator exit paths are real work the first draft assumed away; Epic 3 2,430k -> 2,930k; Epic 1 720k -> 925k); round 2 added 3 tasks (-> 10,915k: Tasks 4.9d, 4.9e, 3.3f); round 3 added 2 tasks (-> 11,025k: Tasks 0.7c, 4.9f); the Phase 4 repair added 2 stories and 17 tasks (+1,240k raw -> 12,265k); the triad round 1 repair added 2 stories and 30 tasks (+1,915k raw -> 14,180k); the **triad round 2 repair added 0 stories and 9 tasks (+495k raw -> 14,675k)**: Epic 0 +165k (Tasks 0.7e, 0.7f, 0.7g, from splitting the nine-check Task 0.7a), Epic 1 +55k (Task 1.6e), Epic 2 +55k (Task 2.10d), Epic 4 +165k (Tasks 4.1d, 4.9i, 4.9j; 4.9b and 4.9d were split to keep each task within 3 to 5 files), Epic 5 +55k (Task 5.1e); the **triad round 3 repair added 0 stories and 3 tasks (+130k raw -> 14,805k)**: Epic 2 +55k (Task 2.4c, the early `PaneShowsDialog` owner), Epic 4 +55k (Task 4.3d, a guards-only S after the split, plus Task 4.3l, the revive path, M: net +55k) and Epic 5 +20k (Task 5.1f, the `ScheduleWakeup` baseline). The 3-15M note in the old requirements band is exceeded at every multiplier in the range 1.5-2.0, and the multiplier itself is INFERRED. Proceeding with full scope (the owner's default), no cut line. **Epic 4 is the likeliest overrun** and carries an explicit checkpoint after 4.1 + 4.2 (see Story 4.2) in addition to the cumulative one below; the stop rule for repair loops is in the pre-mortem disposition (#12).

Critical path in agent waves (task = one worker run; stories on disjoint files fan out; every concurrent worker uses its own worktree and branch because shared-worktree workers swept each other's hunks):
**Concurrency cap: at most 3 workers at once.** The repository already recorded one out-of-memory incident from too many concurrent sessions (the 2026-07-12 pattern cited in `research/architecture.md`; the backlog WIP limit is 2) and each worker holds a worktree, a `go build` cache and, for the spikes, a live Claude session. A wave with more than 3 stories therefore runs as consecutive batches of at most 3, in the order given; wave membership is the dependency level and the batches below are the execution order. Cost (CU) is unchanged by the cap; only the number of sequential batches grows, and the critical-chain story is always placed in the first batch of its wave so the chain is not lengthened.
- W0 (8 items, 3 batches): B1 S1 (internally serial 0.1a -> 0.1b/0.1c), S4, 0.8 (incl. 0.8d); B2 S2, S5, S6; B3 S3, S7 (Tasks 0.7a-g, incl. Task 0.7d, the jscpd and flaky-suite baseline).
- W1 (8 items, 3 batches): B1 3.1, 2.1, 2.4 (incl. 2.4c); B2 1.1, 4.1 (incl. 4.1d), 4.2; B3 4.9, 1.7 (1.7 starts when 2.1's proto PR merges: both edit `proto/session/v1/types.proto`; Task 2.1a adds the `Session.background_outcome` field in that one commit). Epic 4 checkpoint after 4.1 + 4.2 before 4.3 starts.
- W2 (4 items, 2 batches): B1 3.2, 1.2 (after 1.7), 2.2 (incl. 2.2e); B2 4.3a/b.
- W3 (6 items, 2 batches): B1 3.3 (incl. 3.3g-j), 1.3, 2.3 (ships `ANSWER_QUESTION` behind the `QuestionReplier` interface with `hiddenReplyAdapter`; see the stub-seam AC); B2 1.6 (incl. 1.6d, 1.6e), 4.3c/d/e/f/g/h/i/j/k/l, 4.4 (4.4c after 3.3).
- W4 (7 items, 3 batches): B1 3.4 (swaps the `AnswerRouter` in behind `QuestionReplier`; incl. 3.4g), 1.4, 2.5 (after 1.1 on `NotificationPanel`; incl. 2.5d-f); B2 1.5 (incl. 1.5b), 4.5 (after 4.9 and 3.4), the adoption counters 5.1a + 5.1d (after 2.3, with 2.5) so the in-pane share, delivery-failure and Undo counters exist when the first inline-action release ships; B3 1.8 (after 1.3, 1.4a, 1.5a).
- W5 (10 items, 4 batches): B1 3.5, 2.6 (after 1.6b; incl. 2.6c-e), 2.7; B2 3.6 (3.6c last), 2.10 (after 2.6; incl. 2.10d), 2.8 (incl. 2.8c); B3 4.6a/b/d, 4.9g (conditional on S5), 4.9h; B4 4.11 (after 4.5c and 4.6b; incl. 4.11d), 5.1e (after 4.3).
- W6 (5 items, 2 batches): B1 3.7 (incl. 3.7d-f), 2.9 (incl. 2.9c; its `ANSWER_QUESTION` spec runs after 3.4), 3.8; B2 4.7, 4.10 (4.10d after 4.11a), and the optional-to-order tasks 4.6c / 4.10b (Remind me) and 3.6c (harness-native channels), scheduled last so owner answers never delay other stories.
- W7 (3 items, 1 batch): 4.8 (incl. 4.8d), 5.1 (remaining 5.1b, 5.1c), 5.2.
- W8 (2 items, 1 batch): 5.3 (incl. 5.3c), 5.4.
Nine dependency levels (W0 to W8) on the critical chain S1 -> 3.1 -> 3.2 -> 3.3 -> 3.4 -> 3.5/3.7 -> 5.3, executed as 21 batches under the cap (3+3+2+2+3+4+2+1+1, recounted from the lists above); an uncapped run would be 9. **Serialization points, with the order inside a wave** (the order in which the PRs merge; a later item rebases onto the earlier): `ActionRow.tsx` and `NotificationPanel.tsx` (2.5, then 2.6, then 2.10), `proto/session/v1/types.proto` and generated output (2.1 first; 1.7 starts when it merges), `NotificationPanel.tsx` (1.1 before 2.5/2.6), `BackgroundActivity.tsx` and `useBackgroundActivity.ts` (1.3, then 1.6, then 1.4a, 1.5a, 1.8, later 4.6b, then 4.11a), `server/services/hidden_question_reply.go` (2.4 before 3.4b), `server/services/guidance_request_service.go` (3.1, then 3.3, then 4.4c in W3, then 3.4, then 4.5c in W4), `server/dependencies.go` (4.2, then 4.9 in W1, then 4.3g), `session/instance_actor_setters.go` (4.9b only), `server/wakes/` (4.2 before 4.3), `server/server.go` MCP middleware (4.9 before 4.5), `tools/lint/noarchivedrevival/analyzer.go` (4.3f only). The `ANSWER_QUESTION` seam between Story 2.3 (W3) and Story 3.4 (W4) is the consumer-side `QuestionReplier` interface, so neither story edits the other's files; `session.PaneShowsDialog` (Task 2.4c, W1) is likewise built before its three consumers.

**Overrun checkpoint**: if cumulative spend on completed stories exceeds 1.25 x the "CU with verification" value for those stories (for example above 7,352k CU when all of Epic 3 is done: 3,361k CU x 1.75 x 1.25 = 7,352.2k; above 9,428k CU when all of Epic 4 is done: 4,310k CU x 1.75 x 1.25 = 9,428.1k), pause and report the delta with its cause (wrong estimate or real surprise). Informational, not a cut invitation.

**Wall-clock blockers** (named per story in the table above; these are the calendar facts, not agent hours):
- Calendar chain, in order: (1) the live spikes S1, S2, S3 and S5 need interactive sessions and real-time waits and S4 needs a test-repo CI run, all inside W0; (2) the owner holds the Phase 5 start gate before W0 begins (accepts the eight ADRs, confirms or amends items 1 to 7) and reviews the spike-result items 8 and 9 as S1, S3 and S5 record them; no wave waits on a default because each proceeds, and an amendment to items 3, 4, 5 or 9 only re-gates Tasks 4.9, 3.4c, 4.5a and 4.9g; (3) agent waves W1 to W8 with a CI run per PR (including Playwright e2e per epic); (4) the per-flag soaks of 14 days each (5.3), run in rollout order (Background, inline actions, structured questions, wake triggers) with a later flag's soak starting only after the earlier flag's quality metrics pass; (5) real-device checks and the single production deploy window after merge. The critical path is therefore W0 -> W8 followed by the soaks, and the soaks, not the build, set the calendar.
- Owner review of the vision and ADR-004, ADR-005 decisions (bounded blocking hook), ADR-007 rule 0 credential and role allowlist before per-harness adapters and Epic 4 tools merge - Tyler.
- Live interactive sessions for S1, S2, S3, S5 (real-time waits, harness installs) on the manual instance - the executing agent.
- Test-repo CI/webhook run for S4 - executing agent.
- CI runs, including Playwright e2e per epic - CI.
- Per-flag dogfood soak before any default flip (5.3, 14 days of use per flag plus the usability script T1-T10 and the quality-metric review) - Tyler.
- Real-device soft-keyboard, Back-gesture and contrast checks on iOS Safari and Android Chrome (UX-AC74, 87) - Tyler's devices.
- A Claude Code update landing during the project: the layout canary and fixture comparison (Task 3.3h) must be re-run - the executing agent.
- `make install-service` is never used for testing (it restarts the live service and kills tmux without `--tmux-keep-server`); the single production deploy after merge needs a deploy window - Tyler.

---

## Self-review notes (adversarial pass, recorded)

- Every research-listed UNVERIFIED item maps to a spike with PASS/FAIL and fallback: hook answer in TUI (S1), hook blocking duration (S2), Channels (S3), webhook/PR/CI waking (S4), other-harness channels (S5), remote chain (S6), assumed code state (S7).
- Conflict resolved: UX research wants peer requests informational; pitfalls want hidden peer-help/wake failures to be needs-human or failure class. Decision: a pending peer request is shown only in "Scheduled prompts", Background and Recent rows (no notification, no badge); a stall (10 minutes or an ended answerer) emits one needs-human `INPUT_REQUIRED` notification from the `PeerHelpEscalator` (Task 4.11b); wake missed/skipped/rate-limited emit failure-class notifications (Task 4.3e). This keeps the gate unchanged.
- The `guidance_answered` trigger avoids `mapBacklogChangeKind` by using a direct callback plus reconcile (Story 4.4), so no new backlog change kind can be silently dropped.
- Dedup for hook rows avoids a new unique index (ADR-003) and, after review, is keyed on `(asker_session_uuid, hook_call_id)` with the existing text coalesce bypassed and the pending cap counted per source class; the earlier "legacy defaults" rationale alone was incomplete.
- Triad round 2 stop rule applied (repeated rounds kept finding contradictions that the previous repair introduced): each decision now lives in exactly one place and the other places cite it. Engagement, list freeze, focus advance, digit guard and the fling guard: `design/ux.md` 7.1. Keyboard keys and Esc precedence: `design/ux.md` section 7. Delivery-state strings: `design/ux.md` 3.1. Transition table including `pending -> skipped(rate_limited)` and the cron occurrence skip: ADR-006 rule 2. Cron floor, per-owner limit and the revive-lock bound: the "Wake limits" table only. Owner defaults and the single accept step: "Unresolved Questions". Flag-default rule, provisional targets and the A1 fallback: `requirements.md`. Deleted rather than patched: the flush-on-exit commit delay (replaced by cancel), Create rule in the primary row, the single lock that spanned a revive, and the "10 limits / 11 rows" count statements.
- Tasks that depend on the code-state spike (S7) say so in their first step: 1.5a, 2.2b. The scheduler question (old 4.2a) is closed by reading the code (see Story 0.7) rather than left to a spike.

---

## Review repair log (2026-10-17)

*Counts in the first three repair logs are as of those rounds; the current counts are in the Phase 4 log at the end.*

Counts after repair (recounted with `grep -c "^# Epic"`, `grep -c "^### Story"`, `grep -c "^##### Task"`): **6 epics, 45 stories (was 42), 132 tasks (was 108)**. New stories: 1.6, 4.9, 4.10. Story 4.2 was re-scoped (scheduler seam -> dispatcher timer and `Clock`), Story 0.8 re-scoped (no new resolver), Story 0.7 re-scoped (items answered from code removed, PostToolUse matcher probe added). Effort: raw 8,790k -> 10,600k; CU with verification 14.5M -> 17.4M (XL); see the Effort section.

### Blockers

| # | Blocker (source) | Resolution | Changed |
|---|---|---|---|
| 1 | Caller identity forgeable (adversarial #1, architecture #1) | Chose the per-session HMAC credential (reusing `server/services/hook_proof.go`, domain `ssq-mcp-v1`, `X-Stapler-Session-Proof` / `STAPLER_SESSION_PROOF`) verified in `mcpWithUUID`; new tools use `verifiedCallerSessionUUID`; unverified requests get `identity_unverified`. Residual same-user risk stated honestly | ADR-007 rules 0, 1, Consequences, Alternatives; Story 4.9 (new, Tasks 4.9a-c); Story 4.5 ACs and Tasks 4.5a-d (forged-header and cross-session-proof tests); glossary `CallerIdentity`, `McpSessionProof`; Unresolved Questions (owner confirm); Observability (`mcp_identity_unverified_total`) |
| 2 | Hook rows vs `GuidanceRequest` coalescing and cap (adversarial #2) | Corrected ADR-003 rationale (real constraints: Go-side `(scope, scope_key, question_text)` coalesce at `ent_repository_guidance.go:231` and cap 4 at `:18-25`); coalesce keyed on `(asker_session_uuid, hook_call_id)` for non-`agent_mcp` sources; cap per source class; `scope_key` = asker UUID for hook and peer rows; mirror clearing on terminal answer/dialog gone/session end; per-source `notifyAnswered`; `NewGuidanceRequest` constructor and transition table | ADR-003 rules 1, 3a, 3b, alternatives; Story 3.1 ACs and Tasks 3.1c/3.1e; Story 3.3 ACs and Tasks 3.3d/3.3e; Story 4.5 AC (peer answer notification); Migration Plan; Pattern Decisions (Question dedup) |
| 3 | Wake revival not covered by `noarchivedrevival` (adversarial #3) | Named the resume function (`reviveStoppedOwner` calling `(*Instance).RecoverFromStopped`, `session/instance_state.go:533`, gated by `BacklogService.Admit`); added `server/wakes/revive.go` to `lifecycleFiles` with analyzer testdata and a CI-run AC; exported `session.IsOneShot` | ADR-007 rule 5; Story 4.3 ACs and Tasks 4.3d/4.3f; Migration Plan Files; glossary `Admission` (replaces nonexistent `SpawnGate`) |
| 4 | Wake delivery durability (architecture #2) | Claim/delivery split: `pending -> claimed -> delivered | needs_revive | missed`; `delivered_at` set only after a confirmed write; reconcile re-queues stale claims; at-least-once with wake-id marker; storm test restated as revives of stopped owners | ADR-006 rules 1, 2, 4; Story 4.1 (ACs, Tasks 4.1a-c); Story 4.3 ACs and Task 4.3b/c; Story 4.8 ACs and Task 4.8c; glossary `WakeStatus`, `SteerDelivery`; Observability (states, `requeued_claimed`, stale-claim alert) |
| 5 | Task 3.3c extends a nonexistent sweep in a hotspot (architecture #3) | Replaced with a new `GuidanceSessionReaper` (event subscription at the existing session-deleted/archived seam, `server/services/hidden_question_reply.go:241` / `review_queue_manager.go:277` precedent, plus startup pass) calling `CancelGuidanceRequest`; `backlog_lifecycle.go` removed from Files; Tech Debt rows added for it and the other hotspots | Story 3.3 AC, Files and Task 3.3c; Tech Debt Disposition (new rows); glossary `GuidanceSessionReaper` |

### Concerns folded in

| Concern | Resolution | Changed |
|---|---|---|
| WakeScheduler seam underestimated; no fake clock possible | Dropped the seam; dispatcher owns one `min(next_fire_at)` timer with an injected `Clock`; `cron.ParseStandard` only for next times; `workflows.Scheduler` untouched; added a checkpoint after 4.2 | ADR-006 rule 3; Story 4.2 (rewritten); Tech Debt row; Pattern Decisions; Dependency Visualization |
| Event-trigger reconcile for transient kinds | Every `(type,kind)` needs a state predicate or is rejected with `no_reconcile_predicate`; completeness test | ADR-006 rule 4; Story 4.4 ACs, Task 4.4d |
| PostToolUse matcher may not cover `AskUserQuestion` | Spike S7 item (6) probes it; Task 3.4e adds the matcher if missing; `acknowledged` falls back to dialog-gone | Story 0.7, Story 3.4 AC, Task 3.4e; ADR-005 |
| "All healthy" overclaims | `no_failures_reported` copy; `finished` needs positive evidence; `stopped_unknown` kind | Glossary, Stories 1.2, 1.3, `design/ux.md` (UX-AC1, wireframe, 1.x copy) |
| Flag plumbing duplicates scope overrides; wrong file names | Reuse `Config.GetFeatureFlagScoped`; register in `server/services/feature_flag_service.go`; real RPC names `GetFeatureFlags`/`UpdateFeatureFlag`; `server/features/*.go` identified as registry metadata; client-event handler located by S7 | ADR-008; Story 0.8 (rewritten), Task 1.4a, Story 1.5 Files, Pattern Decisions, glossary `ScopedFlag` |
| Import-cycle risk `server/wakes <-> server/services` | Ports defined in wakes, `AnswerListener` func type owned by services, `go list -deps` test, exported `session.IsOneShot` | ADR-006 rule 3; Story 4.3 AC and Task 4.3g; glossary |
| Scope drift (4.6c Remind me, 3.6c harness channels) | Kept in scope, scheduled last wave (W6), marked optional-to-order, Blocked on owner where applicable; not cut | Tasks 3.6c, 4.6c, 4.10b; Unresolved Questions; Effort waves |
| Epic 4 likely overrun | Re-estimated (+43% raw), explicit checkpoint after 4.1 + 4.2, size band restated as XL | Effort estimate, Story 4.2 |
| Other architecture concerns | Dispatcher delegate interfaces and idempotent backlog delegates (Tasks 2.3e); `ListenerClass` for descriptors (Task 2.2d); event-driven resolve path re-sized (2.7a/2.7c, ADR-002 note); per-source answer side effects; `human_active` guard for visible Tier B (Task 3.4f, ADR-004); per-file `ScheduleWakeup` map (4.7); hotspot rows; `NewGuidanceRequest` constructor; glossary count, nitpicks (`session/domain/guidance_request.go`, `useFeatureFlag.ts`) corrected | Respective stories, Tech Debt Disposition, ADR-002/004 |

### UX gaps (`design/ux.md` section 8)

| Gap | Owner in plan |
|---|---|
| Background poll failure | Story 1.6, Task 1.6a |
| Offline banner | Story 1.6, Task 1.6b (consumed by Task 2.6b) |
| Departed / reaped session Open copy | Task 1.6c, Story 4.10 |
| Remind-me create / cap / past-time / duplicate failures | Story 4.10, Task 4.10b |
| Scheduled prompts load failure and missed-prompt rows | Story 4.10, Task 4.10a |
| Resume control for paused wakes | Story 4.10, Task 4.10c |
| Cancel stalled peer request / Answer for {peer} | Story 4.10, Task 4.10d (new `CANCEL_PEER_HELP`, `CANCEL_WAKE` kinds) |
| Aborted multi-question answer | Story 3.5 AC, Task 3.5c |
`design/ux.md` section 8.4 carries the same mapping.

### Reviewer claims checked and not disputed, with one nuance

All five blockers and the listed concerns were verified against the code before editing (header forgery at `server/server.go:1112-1118`; `lifecycleFiles` at `analyzer.go:39-45`; no `CancelGuidanceRequest` production caller; `workflows.Scheduler` method list and `repo == nil` early return; `server/services/hook_injector.go` has no `AskUserQuestion` matcher; `ScheduleWakeup` counts). Nuance, not a dispute: `pkg/events/bus.go` keeps an in-memory ring buffer with `EventsSince` replay for slow subscribers, but it is lost on restart, so the reviewer's conclusion that transient edge kinds cannot be reconciled after a restart stands. Left unfixed by design: the `:8543` listener remains unauthenticated; ADR-007 rule 0 scopes the credential's guarantee to agents that do not hold another session's proof.

---

## Review repair log, round 2 (2026-10-17)

Counts after round 2 (recounted with `grep -c "^# Epic"`, `grep -c "^### Story"`, `grep -c "^##### Task"`): **6 epics, 45 stories (unchanged), 135 tasks (was 132)**. New tasks: 3.3f, 4.9d, 4.9e. Effort: raw 10,600k -> 10,915k; CU with verification 17.4M -> 18.0M (XL). Each claim below was checked against code before editing (R1: `session/instance_tmux.go:655-680` and `:729-739`, `session/nudge_gate.go:133`, `server/mcp/proxy.go:66-75`, `server/mcp/server.go:170`, `server/services/hook_proof.go`; others cited in the changed text).

| # | Item (source) | Resolution | Changed |
|---|---|---|---|
| R1 | Proof delivery omits `claudeMCPConfigArgsWithToken` and routes the secret through argv and tmux env (adversarial re-review BLOCKER) | Verified: inline `--mcp-config` argv is used whenever no local token exists (`instance_tmux.go:666-667`); `tmux show-environment` returns `-e` env (`nudge_gate.go:133`). Proof now delivered only through 0600 per-session files (`mcp/<uuid>.json`, `mcp-proofs/<uuid>`); `STAPLER_SESSION_PROOF` env dropped; `claudeMCPConfigArgsWithToken` added with a spawned-session verify AC, argv-non-leak and env-non-leak tests; injected `SetMCPProofProvider` in `session`; pre-upgrade-session and `RunStdio` behavior stated as `identity_unverified`; error copy must not reuse the old "Set STAPLER_SESSION_UUID" text; `Rotate` revokes proofs. Exfiltration via `run_command` / `steer_session` + `read_session_output`: **accepted risk**, stated in ADR-007 rule 0, with best-effort redaction (not a boundary) | ADR-007 rule 0 and Consequences; Story 4.9 (ACs, Files, Tasks 4.9b, 4.9d, 4.9e); glossary `McpSessionProof`; Unresolved Questions (accepted-risk confirmation); Effort |
| a | Wake transition table incomplete; cron re-arm; stale-claim re-queue vs live cache (architecture re-review) | Full table in ADR-006 rule 2 (adds `pending->missed`, `claimed->skipped`, `cancelled` from `pending|claimed|needs_revive`, `needs_revive->skipped`); cron cycles `claimed -> pending` on the same row until `max_fires`; lease re-queue is startup-only, periodic pass skips ids held by `SteerDelivery` | ADR-006 rule 2; glossary `WakeStatus`; Story 4.1 ACs and Task 4.1c; Story 4.3 AC |
| b | Reaper scope contradicts Story 3.4; would cancel agent-created guidance | Reaper limited to `source IN (ask_user_question_hook, peer)`; `agent_mcp` session rows follow 3.4 (`saved_for_later` / stored-undelivered); event `SessionID` resolved to UUID (AC) | ADR-003 rule 1; Story 3.3 AC and Task 3.3c; glossary `GuidanceSessionReaper` |
| c | `enforceGuidanceRequestPendingCap` has no source filter (adversarial re-review) | Verified (`ent_repository_guidance.go:272-292`: filters scope, scope_key, answered, cancelled only). Added a `source` class predicate and tests in both directions | ADR-003 rule 1; Story 3.1 AC (reverse direction) and Task 3.1e |
| d | No owner for the "dialog gone on two samples" sampler | New `DialogGoneSampler` (3 s per row, 20 rows per pass, 10 min per row, then `stale`) | Story 3.3 AC, Files, Task 3.3f; ADR-003 rule 1 |
| e | Hook row without `hook_call_id` coalesces by payload hash | Stated as an accepted limitation with a documenting test | ADR-003 rule 1; Story 3.3 AC |
| f | `RecoverFromStopped` deprecated and only sets `Creating`; `reviveStoppedOwner` needs own archive check; `Admit` reserves no slot | Normal start path named: `RecoverFromStopped()` then `Start(false)` (`instance_claude.go:143-145`, `retry_state.go:335`); `transitionTo` is unexported so it cannot be the replacement; SA1019 checked via `make lint` with a `//nolint:staticcheck` reason if flagged; own `IsArchived()` check AC; cap lock spans `Admit` until the revived session is live (ACs in 4.3 and 4.8, 3-owners-1-slot race test) | ADR-007 rule 5; Story 4.3 ACs and Task 4.3d; Story 4.8 AC |
| g | Effort wording and rounding | Corrected "above 15M even at x1.5" (round 1's figures gave 14.9M) and explained the 1k/2k difference as per-epic rounding; totals restated after the added tasks | Effort estimate (table, size band, checkpoint values) |

### Round-2 claims checked: one nuance, no disputes

- Item (f) nuance, not a dispute: `Admit` does not count "live sessions" literally; `countLiveBacklogWorkSessions` (`backlog_service_triage.go:1314`) counts `in_progress` items unconditionally and `review` items with an active work session. A stopped owner whose item is `in_progress` is therefore already counted, so `Admit` may refuse such a revive even though it would not raise the count. The plan keeps `Admit` unchanged (conservative direction, matches the one-gate rule) and the lock-spanning requirement stands for owners of `review` items, where reviving does raise the count. Revisit if it blocks legitimate wakes during dogfood.
- Item (f) second nuance: the `RecoverFromStopped` doc comment's "Deprecated:" line sits inside a paragraph, not at the start of its own paragraph, so staticcheck may not treat it as a Go deprecation marker; the SA1019 outcome is UNVERIFIED until Task 4.3d runs `make lint`.
- R1 residual: filesystem exfiltration by a same-user or prompt-injected agent is not closed, by decision (accepted risk, ADR-007 rule 0, owner confirmation recorded in Unresolved Questions).

## Review repair log, round 3 (non-blocking folds, 2026-10-17)

Counts after round 3 (recounted with `grep -c "^# Epic"`, `grep -c "^### Story"`, `grep -c "^##### Task"`): **6 epics, 45 stories (unchanged), 137 tasks (was 135)**. New tasks: 0.7c (spike: does the Claude CLI echo MCP headers) and 4.9f (proof lifetime and liveness). Effort: raw 10,915k -> 10,970k; CU with verification 17,955k -> 18,137k (XL). Source: Re-review 2 (R1) CONCERNS in `adversarial-review.md`; each was checked against code (`session/instance_tmux.go` write-failure branch at lines 677-679 deletes only `Authorization` then re-emits the config inline; grep shows `RemoveFile` is only called from `server/services/hook_proof*.go`, so no `mcp/*.json` cleanup exists; `pkg/events/types.go` has `EventSessionDeleted`).

| # | Concern | Resolution | Where |
|---|---|---|---|
| 1 | Write-failure fallback could re-emit the proof inline | Branch named; must fail closed to `identity_unverified`; forced-write-failure AC and test | Story 4.9 AC, Task 4.9d |
| 2 | Redaction scope (scrollback, `search_claude_history`, notification snippets) | Decision: redaction stays at the two tools; the other readers are listed accepted residual (ADR-007 rule 0, reference doc); scope grows only if Task 0.7c fails | Story 4.9 AC, Task 4.9e, ADR-007 |
| 3 | Proof lives for the whole session; no file cleanup; stale file still verifies | Documented lifetime; delete both files on `EventSessionDeleted` (reaper seam) plus startup sweep; five tools require a live session; ACs and Task 4.9f | Story 4.9, Task 4.9f |
| 4 | Spawn-path matrix missing | Table added; proof files minted only for Claude-family tmux programs | Story 4.9 |
| 5 | Header echo in Claude debug/`/mcp` UI UNVERIFIED | Spike Task 0.7c with PASS/FAIL and fallback | Story 0.7, Task 0.7c |

---

## Review repair log, Phase 4 (pre-mortem P1s and consistency blockers, 2026-10-17)

Inputs: `implementation/pre-mortem.md`, `implementation/consistency.md`, `implementation/validation.md` (its "six deviations"). Docs only; no code was changed. Each claim was checked against files before editing (sources cited in the changed text). Counts below were produced by `grep -c "^# Epic"`, `grep -c "^### Story"`, `grep -c "^##### Task"`, `ls decisions/ADR-*.md | wc -l`, `grep -c "^- UX-AC" design/ux.md` and a glossary-row count, run after the last edit. Counts quoted in the earlier repair logs are as of those rounds and are superseded by this paragraph.

**Counts after the Phase 4 repair (superseded by the triad round 1 log below): 6 epics, 47 stories (was 45), 154 tasks (was 137), 8 ADRs, 44 glossary terms (was 39), 59 UX acceptance criteria (was 58); raw 12,265k, CU 11,529k, CU with verification 20,177k.** Per epic (stories / tasks): E0 8 / 23, E1 7 / 18, E2 9 / 28, E3 8 / 31, E4 11 / 46, E5 4 / 8. New stories: 1.7, 4.11. New tasks (17): 0.5e, 1.5b, 1.7a-c, 2.2e, 2.5d, 2.9c, 3.3g, 3.7d, 4.3h, 4.6d, 4.8d, 4.9g, 4.11a-c.

### Pre-mortem P1s

| # | Finding | Resolution | Changed |
|---|---|---|---|
| P1-a | Background `finished` unsatisfiable: the gate suppresses the routine records it needs | Verified (`server/deliverygate/policy.go` `typeClass`: TASK_COMPLETE and PROCESS_FINISHED are `ClassRoutine`; `HiddenSessionSummary` carries only `state` and `updatedAtMs`, `backgroundActivity.ts:9-12`; `useBackgroundSessions.stateOf` maps `CRASHED` to `other`). Evidence source chosen: server-computed `Session.background_outcome` (`completed|failed|unknown`) from the durable `ItemSession` (`review_verdict`, `triage_result`, `end_reason`; `GetItemSessionBySessionUUID`, `session/storage_backlog.go:386`) and `CRASHED` status. Story 1.2 fixtures rewritten to production-realistic data (no impossible history record); e2e seed has no history rows and still yields `Finished` rows; Spike S7 item (8) checks the live data read-only; alert when `finished` is zero while `stopped_unknown` is positive for 24 h | Glossary (`ActivityState`, `SessionRole`, `BackgroundOutcome`); Stories 1.2, 1.3, 1.4, 1.5 (Task 1.5b), new 1.7 (Tasks 1.7a-c), 0.7 (item 8); ADR-001 decisions 1, 3, 4, 6; `design/ux.md` 1.1; Observability alert |
| P1-b | 10-minute `stale` blocks late answers | Sampler stops polling after 10 min but the row stays open and answerable; `stale` only on a failed `DialogMatch` at answer time, confirmed dialog-gone, terminal answer or session end; 45-minute AC with `APPLIED`; cap starvation handled by one on-demand re-verification at cap time instead of a poll; backoff 3 s then 30 s; sampler documented as the one deliberate exception to "zero new poll loops" | Story 3.3 ACs, Tasks 3.3f/3.3g; ADR-003 rule 1; `design/ux.md` 3.1 delivery table and new UX-AC59 |
| P1-c | Wake typed into a live dialog | Fresh capture under the `TerminalWriteLease` before any `steer_text` write, generic `paneShowsDialog` check, `<wake` marker, `SubmitContentWithEnter`; AC and fault-injection test with the fixture dialog | Story 4.3 AC, Task 4.3c; ADR-006 rule 8 |
| P1-d | Wake cost caps have no numbers | "Wake limits" table (10 limits when written; 12 rows now, defaults and hard ceilings, enforcement point, on-hit behaviour, metric label); dispatcher enforces each; `wake_limit_hit_total{limit,phase}` and `wake_delivered_per_owner_hour`; numeric-ceiling storm test | New "Wake limits" section; Stories 4.3 (Task 4.3h) and 4.8 (Task 4.8d); ADR-006 rule 5, ADR-007 rule 6; Observability; Unresolved Questions |

### Consistency blockers

| # | Resolution | Changed |
|---|---|---|
| B1 | ADR-001 decision 3 now reads "no failures reported" and defines `finished` / `stopped_unknown` | ADR-001 |
| B2 | A harness that cannot load our MCP config cannot call the tools at all (verified: ssq passes its MCP URL per session only through Claude's `--mcp-config`, `session/instance.go:432`; other harnesses get global `llm-sync` entries that cannot carry a per-session value). Proof minting extends to every harness whose per-session MCP config ssq controls, per Spike S5 (new Task 0.5e); matrix split into proof-capable and fail-closed rows; Task 4.9g (conditional); requirements say "harnesses that support MCP with a verified credential" | Story 0.5, Task 0.5e; Story 4.9 matrix, Task 4.9g; ADR-007 rule 0; `requirements.md` Users, Feasibility Risks; Unresolved Questions |
| B3 | One decision table in ADR-006 rule 2 (owner state x action x `allow_revive`); `allow_revive=false` plus stopped owner is `skipped(owner_stopped)`, never `needs_revive`; Story 4.3 and ADR-007 rule 5 cite it | ADR-006, ADR-007 rule 5, Story 4.3 ACs, glossary `AllowRevive` |
| B4 | Revival guards scoped to `action_kind=steer_text`; `notify_operator` needs no revival. **Extra finding while verifying**: the "Remind me" INFO notification bound to a hidden session would have been suppressed by the delivery gate (`ClassRoutine`), so it is emitted as a system-level notification with no session binding and `metadata.target_session_uuid` | ADR-006 rule 2, ADR-007 rule 5, Story 4.3 AC, Story 4.6 AC, Story 2.2 gate-class table, `design/ux.md` 6.2, glossary `WakeAction` |

### Consistency concerns and other requested fixes

| Item | Resolution | Changed |
|---|---|---|
| Effort arithmetic (C12) | Raw total recomputed by script from task classes: the round-3 figure 10,970k was wrong (cells summed to 11,025k); now 12,265k; CU and verification columns, size band, and the Epic 3 / Epic 4 overrun checkpoints (6,519k and 8,369k) recomputed | Effort estimate |
| 200 ms pane SLO (REQ-18) and p95 < 1 s (REQ-19) | ACs and tasks: pane-open budget (Story 2.5, Task 2.5d; Story 1.3 first-commit AC; metric `notification_pane_open_ms`), descriptor batching (Task 2.2e), real-server latency measurement (Task 2.9c); Story 2.3 states the p95 is a Benchmark plus a deterministic guard | Stories 1.3, 1.5, 2.2, 2.3, 2.5, 2.9; Observability |
| ADR-004 tiers (C11) | `ANSWER_GUIDANCE` and both cancel kinds are Tier A, enum names used; remote-chain AC | ADR-004, Story 2.8, glossary `ActionKind` |
| Peer-help UX owner and escalation emitter (C7) | New Story 4.11 (`PeerHelpRow`, `PeerHelpEscalator` with 10-minute deadline and immediate escalation, call-time rejections create no row); gate-class table row | Story 4.11, Stories 4.5 and 4.10, UX-AC33, `design/ux.md` 4.2 and 4.3, Self-review notes |
| "kind" ambiguity (C1) | `ActivityState` / `SessionRole` / `ActionKind` / event kind / `NotificationType` disambiguated; `background_activity_rows{state}` | Glossary note, Pattern Decisions, Observability |
| UX surface ids vs spike ids (C3) | UX surfaces renamed `UX-S1` to `UX-S7` in `design/ux.md` and in the plan | `design/ux.md`, Story 4.10, Task 4.10d |
| `terminal_detect` (C4) | Dropped; non-Claude harness rows use `source=ask_user_question_hook` with the profile in `questions_json.profile` | ADR-003, glossary `QuestionSource`, Story 3.6 |
| Delivery-state copy (C8) | `design/ux.md` section 3.1 / UX-AC23 is the single source; Stories 3.4 and 3.7 cite it | Stories 3.4, 3.7 |
| `status_transition` (C9) | `(type, kind, predicate)` table enumerated in Story 4.4: bare `status_transition` is rejected, `status_reached{status}` is supported; Story 4.3 example uses `verdict_recorded` | Story 4.4, Story 4.3, ADR-006 rule 4 |
| UX "Healthy" label (C2), tab names (N8), "Question 5" (N2), surface count (N3) | Renamed "No failures reported"; tabs are Notifications and Background (verified `NotificationPanel.tsx:785`), "Needs you" is a tier; fixed the bullet reference; totals line | `design/ux.md` |
| `question_id` vs `guidance_request_id` (C5) | Two id spaces defined; descriptor carries the durable id once mirrored | Glossary `TargetKey`, `QuestionID`, `GuidanceRequestID`; ADR-002 decision 1 |
| UX acceptance criteria with no plan AC (C13) | ACs added (UX-AC15, 16, 17, 22, 28, 36, 37, 40, 54, 57, Invalid/Refresh, auto-handled STALE, 100- and 20-row caps) and a full 59-row ownership trace | Stories 2.5, 2.6, 2.9, 3.7, 4.6, 4.10, 5.3; "UX acceptance criteria trace" |
| Requirements wording (N1, C10) | "all healthy" and "fired" wording fixed; PR/CI dependency on S4 stated; Remind me and Resume scheduled prompts added to Scope | `requirements.md` |
| Validation's six deviations | All six fixed in plan text: (1) p95 is a Benchmark (Story 2.3); (2) `backgroundActivity.test.ts` is created, the existing e2e spec and page helper are extended (Stories 1.2, 1.4); (3) Tier B proof is a Go integration test since the e2e server has no `--remote-port` (Stories 0.6, 2.8); (4) fixtures live in `session/testdata/reply_dialogs/` (Stories 0.1, 3.6); (5) the pane fault double is extracted in Task 2.4a (Stories 2.4, 3.5); (6) conditional stories keep named skipped tests (Epic 0 rule and Stories 0.3, 3.4, 3.5, 3.6, 4.6, 4.9) | Respective stories; `implementation/validation.md` |

### Claims checked: disputes and nuances

- **Pre-mortem P1-c (nuance, not a dispute of the finding)**: it prescribes a "`DialogMatch`-negative capture". `session.DialogMatch(capture, q)` (`session/pane_reply.go:74`) needs a specific `QuestionDialog`, so it cannot answer "is any dialog showing". The fix keeps the intent (fresh capture under the lease, fail closed) with a generic `paneShowsDialog` check.
- **Pre-mortem P1-a (nuance)**: it suggests using "the session's exit/last-status field in `HiddenSessionSummary`". That type has only `state` and `updatedAtMs` today, and the proto `Session.status` already separates `CRASHED` from `STOPPED` but the client maps `CRASHED` to `other`. A server-computed field was chosen over N client-side per-item lookups every 30 s. UNVERIFIED: whether the 11 live sessions actually have recorded verdicts; Spike S7 item (8) records it, and the feed degrades to `stopped_unknown` rows with an alert if not.
- **Consistency C7**: it asks whether rejected peer requests persist a row. Decision recorded: no row; a typed error to the asker, counted in `peer_help_total{state="rejected"}`. This changes UX-AC33's wording (call-time rejections versus accepted-then-undeliverable).
- **Consistency C13**: the "0 hits" claim was checked by reading the whole plan (the plan directory is untracked, so no earlier revision exists to grep); no Deny-reason, Undo, "Show all" or Refresh text was present before this repair.
- **Consistency C12** is confirmed exactly: epic cells 1,115 + 925 + 2,130 + 3,060 + 3,385 + 410 = 11,025.
- **Pre-mortem P2/P3 items are not part of this repair** (#5-#20 except as noted): rows #8 and #15 are partly covered by the new batching and render-budget ACs; the rest remain to be folded into their owning stories' ACs before those stories start. Listed as open in `pre-mortem.md` "Repair status".
- The default numbers in the Wake limits table are INFERRED proposals and need the owner's one-time confirmation in W0.

---

## Review repair log, triad round 1 (2026-10-18)

Inputs: the first triad review (Product, UX, Engineering) of this directory, plus `implementation/pre-mortem.md` P2/P3 rows #5 to #20. Docs only; no code was changed or run. Each code claim was checked against the repo before it was written (sources are cited in the changed text; the checks that mattered are listed under "Claims checked"). Counts below come from `grep -c "^# Epic"`, `grep -c "^### Story"`, `grep -c "^##### Task"`, `ls decisions/ADR-*.md | wc -l`, `grep -c "^- UX-AC" design/ux.md`, a glossary-row count and a script over the task-class markers, run after the last edit.

**Current counts: 6 epics, 49 stories (was 47), 184 tasks (was 154), 8 ADRs, 48 glossary terms (was 44), 88 UX acceptance criteria (was 59), 24 metric names (was 16); raw 14,180k, CU 13,330k, CU with verification 23,327k.** Per epic (stories / tasks): E0 8 / 25, E1 8 / 23, E2 10 / 37, E3 8 / 37, E4 11 / 52, E5 4 / 10. New stories: 1.8 (Background governance), 2.10 (interaction safety). New tasks (30): 0.7d, 0.8d, 1.6d, 1.8a-d, 2.5e-f, 2.6c-e, 2.8c, 2.10a-c, 3.3h-j, 3.4g, 3.7e-f, 4.2d, 4.3i-k, 4.9h, 4.11d, 5.1d, 5.3c. Validation: 439 requirement-to-test rows and 88 UX test rows.

### UX blockers

| # | Finding | Resolution | Changed |
|---|---|---|---|
| B1 | "One tap sends" was specified on a `radiogroup`, where arrow keys change the selection, so browsing options would send an irreversible answer; UX-AC18 and UX-AC48 conflicted | Chose **option buttons** in a labelled `role="group"`. Sends = click, tap, Enter or Space on a focused option button (card then locks). Only moves focus = Tab, arrows, digits. Multi-select keeps checkboxes plus an explicit Submit; free text has an explicit Send. ARIA now matches (buttons, not radios) | `design/ux.md` 3.1, 3.2, UX-AC18/19/48/49; plan Story 3.7 ACs and Task 3.7f; `validation.md` UX-AC18 row |
| B2 | Stable-order rule existed only for Background; Needs you could reorder under a thumb; focus advance after resolve plus digit shortcuts could send a wrong answer | Defined **engaged card** and a **freeze** of the Needs-you tier while any card is engaged; arrivals queue behind "N new"; digits are inert until the operator engages a card, inert 750 ms after any programmatic focus move or list change, and only move focus (never send); a 300 ms settled-layout window ignores activation of just-rendered controls | `design/ux.md` 7.1, UX-AC60 to 62; plan Story 2.10, glossary `EngagedCard`; validation rows |
| B3 | UX-AC24 demoted an unacknowledged answer to Recent activity after 60 s (hid a possibly stalled agent); the acknowledgement signal was undefined | Unacknowledged answers **stay in Needs you** as an **Unconfirmed** sub-state until acknowledged or the operator acts. Signal defined: (a) PostToolUse receipt (needs S7 item (6)) or (b) two clean fresh captures; at 60 s one more capture decides: dialog still showing -> `delivery_failed` with Retry, gone -> acknowledged, error -> Unconfirmed. Mark as handled is browser-local | `design/ux.md` 3.1, UX-AC24; ADR-005; plan Story 3.4 ACs and Tasks 3.4e, 3.4g; validation UX-AC24 row |

### UX gaps folded in

| Gap | Resolution | Where |
|---|---|---|
| Delivery-state table vs UX-AC23; `NOT_SENT` vs `SEND_INDETERMINATE` | 14 enumerated strings at the time (15 after round 2, 17 after round 3); two distinct copies with different exits (Retry vs Open + confirmed retry); the line is not a live region | `design/ux.md` 3.1, 2.3, UX-AC11/23; Story 3.7 |
| Background: new failure vs "N new"; badge rule; permanent rows; growth; tab-name count; first run; double display | Sticky failure strip naming failures; badge composition rule; Mark reviewed (browser-local); Older (24 h) collapse + 100-row cap; tab name carries exception count only; one-time note; one fact on one surface | `design/ux.md` 1.4, UX-AC66 to 72; Story 1.8 |
| Loading, reconnect vs offline, timeout | Three-state banner, 150 ms/8 s/30 s thresholds, 10 s/30 s in-flight copy | `design/ux.md` 9, UX-AC73; Task 1.6d |
| Row click semantics | Toggle details only; navigation only through Open | `design/ux.md` 1.4, UX-AC71; Tasks 1.8d, 2.5e |
| Mobile Back; modal vs bottom sheet | No modal and no sheet anywhere: desktop non-modal popover, mobile Alerts page with one history entry; "focus-stealing" defined as event-driven focus movement | `design/ux.md` header, 11.3, UX-AC74/75; Task 2.6d |
| Confirm/undo for consequential taps; full command; Approve/Deny spacing | 3 s commit delay with Undo for Approve, Send back, Mark done; full command text with scroll region, count, Copy and fail-closed; 24 px gap, Deny first | `design/ux.md` 2.4, UX-AC63 to 65; Tasks 2.5f, 2.10c |
| Tier A/B list; away operator | Single tier table; away-operator alerting stated as a dependency on out-of-scope Slack/web-push (UNVERIFIED that they deliver today) | `design/ux.md` 2.4, UX-AC77/78; Tasks 2.8c, 5.2a |
| WCAG 2.2 additions | 1.4.10, 1.4.12, 2.5.8, one live region, headings and focus fallbacks, no virtualization, reduced motion, forced colors | `design/ux.md` 11, UX-AC81 to 86; Task 2.6c |
| Vocabulary | Operator terms table: scheduled prompt, Reminder (you), Cancel prompt/Cancel request (bare "Cancel" retired; Back/Clear/Keep), Answer for {peer} with on-behalf-of provenance, Resume scheduled prompts with "all sessions" scope | `design/ux.md` section 0, UX-AC76; Task 2.5e; all "Waiting on"/"Wakes paused"/"Wake cancelled" strings renamed in the three documents |
| Mobile keyboard matrix; shortcuts inert | Matrix per field; handlers on the panel, inert in inputs, terminal, with modifiers or IME | `design/ux.md` 11.4, 7.1, UX-AC87/88; Task 2.6e |
| Saved-for-later list | Read-only list in Recent activity, agent-created rows only (hook rows die with the session, per the reaper) | `design/ux.md` 3.4, UX-AC79; Task 3.7e |
| Peer-help escalation focus/announce | Badge +1, one announcement, no focus move, freeze-aware | `design/ux.md` 4.2, UX-AC80; Task 4.11d |
| Requirements scope 6 vs design 8.3 | Resolved: Remind me and Resume scheduled prompts are in scope with default presets; the "owner decision pending" text and the "Blocked on owner" markers and skipped tests for them are removed | `requirements.md`, `design/ux.md` 6, 8.3; plan Unresolved Questions, Stories 4.6/4.10 |
| Paired quality metric; usability plan | Delivery-failure, wrong-answer proxy and Unconfirmed rates beside "1 interaction"; usability script T1-T7 (now T1-T10 after triad round 2) | `requirements.md`; `design/ux.md` 12; Tasks 5.1d, 5.3c |

### Engineering gaps and ADRs

See "Pre-mortem P2/P3 disposition" for the per-row owner. Items that changed ADRs: ADR-008 (the contradiction between "explicit scoped value wins" and "off disables new writes" is resolved by defining "off" as "the decision point resolves false" and adding a dependency matrix), ADR-005 (acknowledgement signal, bounded-hook fail-open), ADR-004 (`HumanActivity` signal), ADR-006 (rules 9 to 11: cron timezone/DST, duplicate-delivery and busy-owner hold, reconcile digest), ADR-007 (on-behalf-of provenance, 11-row limits table), ADR-001 (one consequence line). The Wake limits table gained row 11 (busy-owner hold).

### Product gaps (`requirements.md`)

Adoption targets with baselines marked "to be measured in Epic 5 telemetry"; four named risky assumptions (A1 operator prefers the pane, A2 peer help has demand, A3 keystroke stability across Claude Code updates, A4 verdicts exist) each with its test and the consequence of being wrong; the ssq#428 polling debt and the $106.69 idle-wait incident moved into the problem statement; Open Questions 1, 3 and 4 marked resolved with ADR pointers (Question 2 stays open: it closes with Spikes S1 and S5).

### Claims checked: disputes and nuances

- **`human_active` signal source (pre-mortem #16).** Verified in code: the single web-terminal pane-input sink is `paneWriter.SendInput` (`server/services/terminal_access.go:140`), reached from `server/services/connectrpc_websocket.go` (`writerFor`) and `server/services/session_service_stream_terminal.go`. No per-session last-input timestamp exists today (grep for `lastInput`/`LastInputAt`/`HumanInput` found none), so Task 3.4f adds one. `#{client_activity}` and `#{client_control_mode}` are documented in the local tmux 3.6a manual; a `list-clients` call with those formats ran without error on a detached probe server, but there was no attached client, so live behavior with a real human attach is UNVERIFIED and is Spike S7 item (10). Input through ssq-mux is not covered (stated as residual risk in ADR-004).
- **Scoped flag semantics (pre-mortem #10).** `Config.GetFeatureFlagScoped` (`config/config.go:1925`) returns an explicit scoped value in *either* direction. The earlier ADR-008 sentence "an explicit scoped `false` always wins" was true but incomplete: a scoped `true` under a global `false` also wins, which contradicted "off disables new writes". The fix is documentation of the existing behavior plus a caller-side dependency matrix, not a new resolver.
- **Cron DST.** robfig/cron v3.0.1 supports the `CRON_TZ=` prefix (`parser.go:95`) and documents that jobs scheduled inside a spring-forward gap are not run (`doc.go:169`); the plan accepts that behavior and tests it rather than fighting it. The fall-back "fire once" and "clock jump back never refires" rules are the plan's own guards, UNVERIFIED until Task 4.2d runs.
- **jscpd baseline (#11).** This checkout has no `web-app/node_modules`, so the actual percentage and margin could not be measured; the 0.10% margin from memory is UNVERIFIED and Task 0.7d measures it before the budget table is final. The table splits the margin by share, not by absolute percentage, for that reason.
- **Flaky suites.** The bug numbers 067/079/090/099 are reused across several files in `docs/bugs/` (for example three open BUG-099 files and two open BUG-079 files); each is cited by file. The `docs/bugs/fixed/` files with the same numbers are different, already-fixed bugs and are not the ones meant here.
- **`saved_for_later` vs the reaper.** While rewriting the Unconfirmed rules I found that `design/ux.md` R3a ("stopped, durable: saved for later") and Story 3.3's reaper (cancels hook and peer rows for stopped sessions) contradicted each other for hook-mirrored rows. Resolved by scoping `saved_for_later` to `agent_mcp` guidance only; a hook-mirrored question on a stopped session reads "Session ended before this was answered." (Story 3.4 AC, `design/ux.md` 3.1 rows 8 and 10).
- **Interpretation, not a dispute:** "constant count in tab name" was read as "the accessible name must not change as informational rows arrive", implemented as exception-count-only; the session total moved to the roll-up text.
- **Not changed:** `implementation/consistency.md`, `adversarial-review.md`, `architecture-review.md` and `research/*` are earlier-round review artifacts and were left as written; their counts and strings ("Waiting on", "59 UX-ACs") are historical.

---

## Review repair log, triad round 2 (2026-10-18)

Stop rule honored: this round simplified and deleted before it added (see "Self-review notes"). Docs only; nothing was run. Counts below come from `grep -c "^# Epic"`, `grep -c "^### Story"`, `grep -c "^##### Task"`, `ls decisions/ADR-*.md | wc -l`, `grep -c "^- UX-AC" design/ux.md`, a glossary-row count and a script over the task-class markers, run after the last edit.

**Current counts: 6 epics, 49 stories (unchanged), 193 tasks (was 184), 8 ADRs, 48 glossary terms (unchanged), 88 UX acceptance criteria (unchanged; none added), 27 metric names (was 24); raw 14,675k, CU 13,795k, CU with verification 24,143k (XL; 20.7M at x1.5, 27.6M at x2.0).** Per epic (stories / tasks): E0 8 / 28, E1 8 / 24, E2 10 / 38, E3 8 / 37, E4 11 / 55, E5 4 / 11. New tasks (9): 0.7e, 0.7f, 0.7g, 1.6e, 2.10d, 4.1d, 4.9i, 4.9j, 5.1e. Delivery-state strings went from 14 to 15 (needed so "Already handled" is not shown for a different dialog). Validation: 439 requirement rows and 88 UX rows unchanged, plus a 22-row round 2 table.

### Blockers

| # | Finding | Resolution (the single place) | Changed |
|---|---|---|---|
| UX-B1 | "Engaged" meant two things; the freeze and the focus advance contradicted each other | One table in `design/ux.md` 7.1: **Engaged** (freeze only; programmatic focus never counts) and **Digit-armed** (digit guard only); a lifecycle table says what removes a resolved row (nothing engaged for 750 ms), when and where focus moves (only if it was inside the leaving row), and what is announced. Stories 2.6 and 2.10, Story 1.8, UX-AC49/50/60/61/62 cite it | `design/ux.md` 1.4, 2.2, 3.2, 7, 7.1, UX-ACs; plan glossary `EngagedCard`, Stories 2.6, 2.10 (ACs, Task 2.10d); validation |
| UX-B2 | Create rule beside Deny with an 8px gap, no tier, confirm or AC | Removed from the primary row. Desktop-only `More` menu (>= 900px, local listener), inline confirm; added to the tier table; not rendered on phone or `:8444`. Asserted in UX-AC65 and UX-AC77 (extended, no new AC), Story 2.5 AC and Task 2.5f | `design/ux.md` 2.1, 2.3, 2.4; plan Stories 2.2, 2.5 |
| UX-B3 | Story 2.5 mapped `NOT_SENT` and `SEND_INDETERMINATE` to one string | Story 2.5 now cites `design/ux.md` 3.1: two distinct strings, safe Retry vs Open session plus "Retry anyway?" confirm; `STALE` carries a reason (`dialog_gone`, `dialog_replaced`, `session_ended`) | plan Story 2.5, glossary `ActionOutcome`; `design/ux.md` 2.3 |
| ENG-B1 | ADR-006 table lacked `pending -> skipped(rate_limited)` that Wake limits and Story 4.3 require; cron conflict; 12/h cron floor vs 6/h limit | Row added to ADR-006 rule 2, glossary `WakeStatus`, Task 4.1c (all-pairs test). A rate-limited cron row skips that occurrence (stays `pending`, re-armed); only one-shot rows go terminal. **One number:** cron floor default 10 min (= 6/h), hard floor 5 min (= the 12/h per-owner ceiling) | ADR-006; "Wake limits"; Stories 4.1, 4.3, 4.8; validation; pre-mortem note |

### Product gaps

| Gap | Resolution | Where |
|---|---|---|
| Adoption measurable in shipping order; requirements vs plan sequencing | Tasks 5.1a and 5.1d (action, delivery, Undo and in-pane-share counters) moved to W4 with Story 2.5; the rest of 5.1 stays W7; requirements say so | plan waves, Story 5.1, dependency graph; `requirements.md` |
| Targets provisional; who decides; flag rule | Targets labelled provisional placeholders; owner sets final numbers; decision rule: 14-day soak, last-7-day quality metrics, in-pane share is A1 evidence not a gate | `requirements.md` |
| Epic 4 cost outcome at runtime | `wake_fires_per_session` and `agent_schedule_wakeup_calls_total{role}` (Task 5.1e); UNVERIFIED that the injected PostToolUse matcher covers `ScheduleWakeup` (Task 0.7e probes it) | `requirements.md`, plan Story 5.1, Observability |
| RICE/ICE per epic | Short ICE table, INFERRED, owner may re-score | `requirements.md` |
| A1 fallback | Owner-triggered only | `requirements.md` (A1, decision rule) |
| Plan header | "Planning complete; nine owner items proceed on documented defaults pending confirmation at the Phase 5 start gate" | plan header, "Unresolved Questions" |

### UX gaps

| Gap | Resolution | Where |
|---|---|---|
| G1 | One polite announcement per async delivery transition ("Answer not delivered" for row 5 only, "Answer sent, question still showing" for row 16, "Answer not confirmed", "Answer partly sent"; late ack silent); test | `design/ux.md` 11.1, UX-AC84; Story 3.7 AC |
| G2 | The hold is **cancelled, never flushed** on panel close, route change, Back, `visibilitychange`, `pagehide`; button `aria-disabled` and focus to Undo; card restored with a note | `design/ux.md` 2.4, UX-AC63; Story 2.10, Task 2.10c; glossary `CommitDelay` |
| G3 | Reserved status slot, `overflow-anchor`, 120 ms fling guard | `design/ux.md` 7.1, UX-AC62; Story 2.10, Task 2.10d |
| G4 | "Answer sent" vs "Answered" honest copy; "Already handled" only for dialog gone/session ended, a different dialog shows "The session is now asking something else..." (delivery row 12); sampler clears with `dialog_replaced` | `design/ux.md` 3.1, 3.3, UX-AC23; Stories 3.3, 3.7 |
| G5 | Shipped phone behavior for `live_keystroke` on `:8444` stated | `design/ux.md` 2.4; `requirements.md` |
| G6 | Decided: accepted per-browser limitation, measured by `background_reviewed_mark_total{storage}`; server-side marks only if the owner reports phantom badges | `design/ux.md` 1.4; Task 1.8c |
| G7 | One keyboard table (Up/Down = J/K, Esc precedence, focus after "N new" and Mark reviewed, WCAG 2.1.4) | `design/ux.md` 7; UX-AC48/88; Story 1.8 AC, Story 2.6 |
| G8, G9 | Controls stay disabled until the first refresh, "As of {time}", empty/loading/error copy for Needs you and Recent activity, drafts in `sessionStorage` | `design/ux.md` 9; Story 1.6, Task 1.6e; UX-AC30/56/73/74 |
| G10 | Hostile trials T8 fling-tap, T9 arrival under the thumb, T10 app switch mid-hold | `design/ux.md` 12; Task 5.3c; requirements |
| G12 | Plan assertions added for UX-AC13 (Story 2.8), 26 (Story 3.3), 27 (Story 3.4), 47 (Story 2.6) | plan Stories 2.6, 2.8, 3.3, 3.4 |

### Engineering gaps

| Gap | Resolution | Where |
|---|---|---|
| Cap lock across revive | Two locks: short `capMu` for create/claim checks (50 ms test) and a separate `reviveMu` bounded by a new Wake limits row `revive_lock` (30 s, ceiling 60 s); on expiry the row stays `needs_revive` and the next cycle re-reads the owner | ADR-006 rule 5, ADR-007 rules 5 and 6, "Wake limits" (now 12 rows), Stories 4.3, 4.8 |
| `BackgroundOutcome` failure allowlist | Allowlist built from every producer of `UpdateItemSessionEndedWithReason`/`WithDetail` at this commit; default non-failure; completed wins; test over the producer literals | ADR-001 decision 6, glossary, Story 1.7 |
| Stale counts, "10 limits", Size band, 8% vs 12% | `validation.md` counts section and the Phase 4 counts row marked superseded; `pre-mortem.md` counts marked superseded; Wake limits is 12 rows; requirements Size is XL; all effort rows use one 12% share (spike overstatement about 1.6%, stated) | validation, pre-mortem, requirements, plan Effort |
| Wall-clock blockers per story and the calendar chain | New column filled for S1, S2, S3, S4, S5, S7, 2.8, 3.4, 3.5, 3.6, 4.3, 4.4, 4.5, 4.9, 5.3; calendar chain stated in named blockers, no hours | plan Effort |
| Task 0.7a bundled nine checks; 4.9b and 4.9d exceeded 5 files | 0.7a split into 0.7a, 0.7e, 0.7f, 0.7g (at most four checks each); 4.9b split into 4.9b + 4.9i and 4.9d into 4.9d + 4.9j, each listing 3 to 5 files. Size classes: none were missing (grep found every task heading carries one) and the nine new tasks have one | plan Stories 0.7, 4.9 |
| `types.proto` collision; Story 2.3 vs 3.4 seam | Task 2.1a adds the `Session.background_outcome` field (one editor); 1.7 starts when 2.1 merges. `ANSWER_QUESTION` goes through the consumer-side `QuestionReplier`: `hiddenReplyAdapter` in W3, the `AnswerRouter` in W4 | plan waves, Stories 1.7, 2.1, 2.3 |
| ADR-008 `expires_at` carve-out | `expires_at` always wins over a false scope: the row becomes `missed` | ADR-008, flag matrix, Story 0.8 |
| `wake_triggers` retention and `dedup_key` for terminal rows | Terminal UPDATE tombstones `dedup_key` to `<key>#<id>`; terminal rows purged after 30 days on the reconcile cadence; Task 4.1d. The tombstone approach is UNVERIFIED against the ent unique index until Task 4.1d runs | ADR-006 rule 12, Story 4.1 |
| Story 1.7 lookup | One batched `ListItemSessionsBySessionUUIDs` per call (no batch method exists today) with a call-count test | Story 1.7 |
| "Tier A resume route never calls Start" | Decided: it never does (it only stores the answer); AC and spy tests; the file stays in `lifecycleFiles` as a ratchet | ADR-005 rung 3, Stories 3.4 and 4.3, Tasks 3.4d, 4.3f |
| `GetFeatureFlagScoped` cite | `config/config.go:1925` (verified; `GetFeatureFlagScopedOverride` is `:1915`) | plan, ADR-008 |
| ADR statuses | Every ADR carries one explicit accept step: Accepted when the nine owner defaults are confirmed at the Phase 5 start gate | ADR-001 to ADR-008 status lines, plan header |

### Claims checked: disputes and nuances

- **`orphaned_triage` is not an `end_reason`.** The request listed it among failure `end_reason` values. Reading every producer showed it is a stuck-reason name (`session/backlog_lifecycle.go:1414`); the allowlist uses the values the code actually writes (`classifyHeadlessCallError` buckets, `parse_error`, the Jules values). Caller-supplied `stopGuardrailSession` reasons are dynamic and default to non-failure; Spike S7 item (8) now records the distinct live `end_reason` values.
- **"Nine items".** The count is the numbered list in "Unresolved Questions". It reaches nine by listing the S1/S3 result review and the S5 harness list as owner items 8 and 9; the previous text held seven open owner items and two closed ones.
- **Tests still use `*/5`.** Repository and timer unit tests (Stories 4.1, 4.2) use `*/5 * * * *` below the create-time floor on purpose; Story 4.3 owns the floor and its tests use the new 10-minute default.
- **Not run.** Nothing here was executed. UNVERIFIED until implementation: the ent tombstone approach (Task 4.1d), `ScheduleWakeup` coverage by the injected PostToolUse matcher (Task 0.7e), the 120 ms fling window on real devices (trial T8), the 50 ms cap-check budget (Task 4.3h).
- **UX-AC count.** Round 2 added no UX-AC; behavior changes were folded into existing criteria (UX-AC23, 30, 49, 50, 56, 60 to 63, 65, 73, 74, 77, 84, 88) so the 88 count and the plan's ownership trace still hold.
- **Left unchanged by design:** `implementation/consistency.md`, `adversarial-review.md`, `architecture-review.md` and `research/*` are earlier-round review artifacts; their counts and strings are historical.

---

## Review repair log, triad round 3 (2026-10-18)

Final round. Docs only; nothing was run except read-only counting scripts. Rule applied: decide each point once, cite it from the other places, delete before adding, and after each edit re-grep every occurrence of the changed wording across `requirements.md`, this plan, `validation.md`, `design/ux.md`, the ADRs and `pre-mortem.md`.

**Current counts: 6 epics, 49 stories (unchanged), 196 tasks (was 193), 8 ADRs, 48 glossary terms (unchanged), 88 UX acceptance criteria (unchanged; none added), 27 metric names (unchanged), 17 delivery-state strings (was 15); raw 14,805k, CU 13,917k, CU with verification 24,356k (XL; 20.9M at x1.5, 27.8M at x2.0).** Per epic (stories / tasks): E0 8 / 28, E1 8 / 24, E2 10 / 39, E3 8 / 37, E4 11 / 56, E5 4 / 12. New tasks (3): 2.4c, 4.3l, 5.1f; Task 4.3d was split (guards only, now S) and every M task now carries a Files line (63 M tasks, checked by script). Execution: at most 3 concurrent workers, 21 batches over the nine dependency levels.

### UX findings

| # | Finding | Resolution (the single place) | Changed |
|---|---|---|---|
| D1 | The digit-armed 750 ms grace was restarted by a digit's own focus move, so `1` then `2` within 750 ms made `2` inert; Story 3.7 and Story 2.10 contradicted each other | A digit-initiated focus move is excluded from the grace clock, does not disarm the card and counts as operator-placed focus under engagement (a); ACs now press `1`,`2`,`3` 50 ms apart | `design/ux.md` 7.1, UX-AC49, 61; plan glossary `EngagedCard`, Stories 2.10 and 3.7, Task 2.10b; validation |
| D2 | At 60 s with the dialog still showing, the row used the "Nothing was typed" string although a digit had been written | New row 16 `delivery_failed(still_showing)`: "Sent, but the question is still showing. Check the session before retrying." with Retry behind "Retry anyway?". A single 60 s decision table covers rows 2, 5 and 6; row 6 carries the Unconfirmed chip and keeps its copy on an indeterminate capture; row 5 has no 60 s capture. Row 17 covers a session ending after a send. Strings 15 to 17 | `design/ux.md` 2.3, 3.1, 3.3, UX-AC23, 24; ADR-005; plan Stories 3.4 and 3.7, glossary `DeliveryState`; validation |
| D3 | `copyGuard` forbids "wake", "Waiting on" and a bare Cancel, but the design and plan used all three | Vocabulary fixed: section "Scheduled prompts", button "Cancel prompt" (plus "Cancel request", "Cancel reminder"), chip "Needs you (N)", empty copy "No scheduled prompts. ...", error "Couldn't load scheduled prompts. Retry". The guard applies to UI strings only; internal identifiers (`wake_trigger`, `schedule_wake`, `CANCEL_WAKE`, flag names) keep their names, and the component is renamed `ScheduledPrompts` | `design/ux.md` section 0, 1.3, 4, 5, 6, 8, UX-AC35 to 39, 72, 76; plan Stories 1.8, 2.5, 4.6, 4.10, 4.11; validation; pre-mortem #14 |
| D4 | A phone operator's Approve hold is cancelled on `pagehide`; the restored card must still reach them | The restored card is an unresolved actionable item and the badge counts it; AC and trial T10 assert it. Outside click and going offline are added as cancel triggers | `design/ux.md` 2.4, UX-AC63, T10; plan Story 2.10, Task 2.10c |
| D5 | iOS Safari does not focus buttons on tap, so a card being read was engaged only for 5 s | New engagement condition (f): operator-expanded until collapsed | `design/ux.md` 7.1; plan Story 2.10 AC, Task 2.10b |
| D6 | A late acknowledgement moved focus without an announcement | One announcement when focus was inside the leaving row ("Answer confirmed, moved to Recent activity"); silent otherwise | `design/ux.md` 7.1, 11.1; plan Story 2.10 |
| D7 | Badge treatment of state `sent` was unstated | A plain `sent` answer (row 2) under 60 s is not counted; rows 3, 6 (from the write) and 16 are | `design/ux.md` 1.4, 3.1, UX-AC67; plan Story 3.4 |
| D8 | "Session ended before this was answered" was shown for answers already sent | Row 10 only when nothing was sent; row 17 otherwise | `design/ux.md` 2.3, 3.1, 3.3; plan Stories 3.4, 3.7 |
| D9 | Free-text "Other" was drawn for `live_keystroke` and hook-mirrored cards, which can only send a digit | Rendered only on routes that carry text (`agent_mcp` durable rows; `hook_reply` if S1 passes); removed from the single-select wireframe | `design/ux.md` 3.1, UX-AC21; plan Story 3.7 |
| D10 | Wireframe said "bottom sheet"; the Deny button drew a menu caret; `research/ux.md` superseded text unmarked; More menu missing from the keyboard tables; pre-expanded first item undefined; changelog said 14 strings | Wireframe and caret fixed; `research/ux.md` carries a superseded note; More menu added to the Tab and Esc rows; pre-expanded item is neither engaged nor armed until the operator acts; this log records 14, 15 and 17 | `design/ux.md` 1.1, 2.1, 7, 7.1; `research/ux.md`; plan changelog row |

### Engineering findings

| # | Finding | Resolution | Changed |
|---|---|---|---|
| E1 | `paneShowsDialog` was defined in Epic 4 (Task 4.3c) but consumed by Epic 3 | New Task 2.4c in Story 2.4 (W1): `session.PaneShowsDialog(capture)` returns `Showing`, `Absent` or `Indeterminate`; Tasks 3.3f, 3.4e and 4.3c consume it | plan Stories 2.4, 3.3, 3.4, 4.3; ADR-005, ADR-006 rule 8; pre-mortem #3; `design/ux.md` 3.1 |
| E2 | The layout-drift canary degraded hook rows to a "durable-record route" that does not exist for them | Degraded = `read_only` (question, options, Open session) unless `hook_reply` is enabled; agent-created rows are unaffected | plan Observability alert, Story 3.3 AC and Task 3.3h; `design/ux.md` 3.3; pre-mortem #5; validation |
| E3 | `ANSWER_QUESTION` composes three flags but the matrix had only `action:<kind>` | New matrix row: `Scoped(action:ANSWER_QUESTION)` AND the route gate (`hidden_session_reply`, `stage:answer`, durable `stage:answer`, `stage:hook_reply`); Task 0.8d enumerates the permutations | plan matrix, Story 0.8, Task 0.8d; ADR-008; validation |
| E4 | The three-deferral rule had no storage | Column `rate_limit_deferrals` (default 0): incremented with each deferral, reset on a successful claim or a cron occurrence skip, kept across restart | ADR-006 rules 1 and 2; plan Migration Plan, Wake limits, Story 4.1, Tasks 4.1a and 4.1d |
| E5 | The cron floor and the per-owner limit are independent config values | The floor is derived in code: `max(5, ceil(60 / effective per-owner limit))`; a lower configured floor is clamped up; the "cannot disagree" claim is replaced by the clamp; AC and `TestEffectiveCronFloor_*` | Wake limits, ADR-006 rule 2, Story 4.3 AC and Task 4.3h |
| E6 | "Saved for later" promised delivery on resume but no task delivers | Verified `get_guidance_request` already exists (`server/mcp/tools_guidance.go`, described as the post-respawn check). Copy softened to "Your answer is saved. The agent can read it once you resume the session."; Task 3.4d gains the read-back test; nothing is pushed | `design/ux.md` 3.1 row 8; ADR-005 rung 3; plan Stories 3.4 and 3.7, Task 3.4d |
| E7 | Serialization list incomplete; W1 ran 8 workers with no cap | Intra-wave merge order added for `BackgroundActivity.tsx`/`useBackgroundActivity.ts`, `guidance_request_service.go`, `server/dependencies.go`, `instance_actor_setters.go`; cap of 3 concurrent workers (the recorded 2026-07-12 OOM pattern; backlog WIP limit 2); wave lists re-derived into 21 batches; chain stories placed first in their wave | plan Effort section |
| E8 | Task 4.3d was over the 3 to 5 file bound; no Files lines on M tasks | 4.3d split into 4.3d (guards, S) and 4.3l (revive path, M); a Files line on every M task | plan Epics 0 to 5 |
| E9 | Doc drift | `validation.md` task count; Story 3.7 Files path; `pre-mortem.md` pointer to the disposition table and component count; `S1c` defined in Story 0.1; `lifecycleFiles` lines 38 to 45; `requirements.md` "by state" | the respective files |

### Product findings

| Finding | Resolution | Where |
|---|---|---|
| No flip criteria for `background_activity_feed` and `agent_wake_triggers` | Added: keep-ON criteria for the first; per-scope criteria for the second (no lost wakes after restart, zero duplicate deliveries, per-owner limit respected, `ScheduleWakeup` calls down at least 50% against the baseline); fewer than 10 wake deliveries keeps a scope OFF with an owner-only manual path | `requirements.md` decision rule; Story 5.3 |
| `ScheduleWakeup` baseline had no task | Task 5.1f captures at least 7 days with the flag OFF; the flip PR template checks for the row | Story 5.1, Task 5.3b |
| ADR gate ordering | The Phase 5 start gate precedes W0; items 8 and 9 are reviewed when their spikes record results | header, Unresolved Questions, wall-clock list |
| Out of scope was short | Added: free-form agent chat, cross-instance peers, non-MCP harness wakes | `requirements.md` |

### Not resolved or not folded in

- **D10 polish not folded** (each needs a design choice, not a wording fix): the Undo button adjacent to Approve on mobile (the 300 ms settle window is the only guard), the permanent 44 px reserved status slot at 390 px, and the session title and age in the card header as part of its accessible name for the remote listener.
- **Still UNVERIFIED until built:** that `session.PaneShowsDialog` can positively recognise an idle composer in every Claude Code layout (`Indeterminate` is the safe default and blocks wake writes, so a miss stalls delivery rather than risking a wrong write); that the injected PostToolUse matcher covers `AskUserQuestion` and `ScheduleWakeup` (Spike S7 item 6); the ent tombstone key (Task 4.1d).
- **Counts** were recomputed by script over the headings (stories, tasks and size classes), the glossary rows, the UX-AC lines and the metric names after the last edit; the previous logs' counts are historical.

## Known gaps carried into implementation

Defined now: "list change" for the 750 ms digit-arming clock means a row inserted, removed or reordered, never a text-only update such as a relative-time refresh (`design/ux.md` 7.1).

Unfixed (the owner story is where the decision belongs):
- Hold-cancel restore marker (the "restored" note after a cancelled commit delay) needs a persisted flag (sessionStorage) so it survives pagehide or a discarded page; unspecified (Story 2.10).
- "Missed: {reason}" (UX-AC39) needs a plain-language mapping for server reason codes such as `owner_busy_timeout` and `rate_limited`; only fixtures are copy-guarded today (Story 1.x copy owner).
- "Retry anyway" semantics for multi-select and multi-question rows (row 16 copy assumes one digit), and whether row 16 counts in the Unconfirmed-rate metric (Story 3.4 / metrics owner).
- No long cap on an operator-expanded card holding the Needs-you freeze on phones (7.1 (f)); a new approval waits behind "N new" though the badge counts it.
- `design/ux.md` 1.2 says a departed row drops Open, but UX-AC6 clicks Open on one; reconcile.
- Owner design choices: Undo placement next to Approve on mobile, the permanent 44 px status slot at 390 px width, session title and age in the card header.
- Product polish from triad 3: no pre-flag frequency baseline; no Epic 1 usefulness-metric target; A2 peer-help demand evidence only arrives after the build.
