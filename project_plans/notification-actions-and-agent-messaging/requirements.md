# Requirements: notification-actions-and-agent-messaging

**Date**: 2026-10-17
**Type**: feature addition (cross-cutting: web-app, server, MCP, harness adapters)
**Complexity**: 3 — system design (multiple epics, harness integrations; builds on `durable-guidance-request`, `notification-revamp`, `notification-tray-and-hidden-session-gate`)

## Problem Statement
Four related gaps keep the operator (Tyler) opening sessions to do work the notification pane should let them do in place:

1. **Background tab looks dead.** `NotificationPanel` renders a "Background" segment (`web-app/src/components/ui/NotificationPanel.tsx:762-806`) that has never shown anything to the operator. Cause (VERIFIED against the live service on 2026-10-17): `selectBackgroundRows` (`web-app/src/lib/utils/backgroundActivity.ts:86`) emits a row only when a *hidden* session has an *unread* failure-class or needs-human history record. The live service has 11 hidden sessions (10 `review:*`, 1 `diagnose:*`, all `SESSION_STATUS_STOPPED`) and `GetNotificationHistory` holds zero records referencing any of them (0 matches per session id; history has only 2 `INPUT_REQUIRED`, none hidden). Healthy background work therefore renders as an empty tab with no explanation. Whether the gate or the producers are why no hidden-session records exist at all is an open question for research (see below).
2. **Notifications are read-only.** A pending selection prompt (AskUserQuestion / approval / `INPUT_REQUIRED`) or a backlog decision (review verdict, stuck item, rework/merge choice) can only be handled by navigating into the session or item.
3. **AskUserQuestion is under-integrated.** Questions surface as terminal text/notifications, not as structured, answerable, durable objects with their options.
4. **No agent messaging/scheduling substrate.** Sessions cannot ask for help (from the human or a peer session) or schedule their own re-prompt by time or event; they poll via `ScheduleWakeup`. Two facts make this more than a convenience gap:
   - **Polling debt (ssq#428).** Backlog verdicts already have a server-side steer and `wait_for_backlog_event` (`research/features.md:61`), but every other wait (time, PR/CI, a peer's state, a human answer) is still a client-side poll loop, and the repo's prompts forbid `ScheduleWakeup` without offering a replacement (`git grep -c ScheduleWakeup -- '*.go'` is non-zero in 8 files, see plan Story 4.7).
   - **Cost incident.** One headless triage session in an idle-wait loop cost $106.69 (242 Monitor + 293 ListAgents calls for 262 completions; `research/pitfalls.md:44`). Prompt text mitigated it (`IdleWaitTurnCeiling`, #885) but is not a cap. Any wake substrate we add must carry numeric limits (plan "Wake limits"), or it recreates that incident with a scheduler behind it.

## Baseline
- Background tab: empty unless a hidden session has unread failure/needs-human history. Operator never sees hidden-session activity.
- Notification actions today: open session, mark read/dismiss. Reply to hidden-session questions exists behind `hidden_session_reply` (`server/services/hidden_question_reply.go`) but is not generalized to visible sessions, approvals, or backlog decisions.
- `durable-guidance-request` gives unattended agents a durable question entity answerable from the web UI; no structured option rendering for AskUserQuestion, no peer-to-peer help, no wake triggers.
- Self-wake: `ScheduleWakeup` polling by sessions; `wait_for_backlog_event` MCP tool exists for backlog events only.

## Users / Consumers
- **Operator** (single human): answers prompts, decides backlog items, sees background activity.
- **Agent sessions** (Claude Code, and other supported harnesses: Aider, Codex/OpenCode/Gemini/Antigravity/pi per repo support): ask, schedule, message. Calling the messaging and wake tools requires a harness that supports MCP **and** can load an ssq MCP config carrying a per-session credential (the tools grant authority by caller identity, so an unverifiable caller is refused). Which harnesses qualify is decided by Spike S5; the rest fail closed with `identity_unverified`. Question rendering and answering (Epics 1-3) do not depend on this.
- **Backlog automation** (triage/review/diagnose hidden sessions): consume decisions and wake events.

## Success Metrics
- **Background**: with the live data above, the tab shows non-empty content (activity rows for the 11 hidden sessions) and a meaningful state when no failures are reported (never a claim of health); failure/needs-human rows stay pinned first. Anchor: today's row count is 0 with 11 hidden sessions.
- **Inline actions**: a pending AskUserQuestion with options, an approval, and a backlog decision are each resolvable from the notification pane with zero session navigation (click count to resolve: 1 interaction after the pane is open, vs. today's open-session + find-prompt + answer). Measured by e2e specs per action type. A tap on Approve, Send back or Mark done starts a 3-second Undo window and is still one interaction (Undo is optional); Deny-with-reason and Merge take two by design. The paired quality metric below keeps the count honest.
- **AskUserQuestion**: ≥1 structured question renders with its question text and option list in the pane, and the answer reaches the originating live session (tmux keystroke or harness-native channel) or the durable record for a dead one; measured by e2e + an answered-latency log field on the guidance entity (created_at→answered_at).
- **Messaging/scheduling**: a session can (a) request help from the human, (b) request help from a named peer session, (c) schedule a re-prompt by delay/cron, by backlog/PR/CI event, by another session's state, and by "human answer arrives"; wakes survive a service restart. Measured by integration tests including restart, and by zero new `ScheduleWakeup` poll loops added. Minimum bar if Spike S4 finds no PR/CI event source: backlog events, `at`/`cron`, session state and human answer ship, and PR/CI triggers return a typed `unavailable` error. Wakes are rate-limited by the numeric limits table in `implementation/plan.md`.

### Adoption targets (PROVISIONAL placeholders; baselines are not yet measured)

"To be measured in Epic 5 telemetry" means the action, delivery and in-pane-share counters (plan Tasks 5.1a and 5.1d, scheduled in wave W4 with Story 2.5, ahead of the rest of Story 5.1 in W7) ship with the first inline-action release, so adoption is measurable in the order the epics ship. **Every number in the two tables below is a provisional placeholder.** The owner (Tyler) sets the final numbers after two weeks of dogfood data with the flag on; until then they are guides, not gates. The flag-default rule that uses them is in "Decision rule for flag defaults" below.

| Metric | Baseline | Target | Source |
|---|---|---|---|
| Share of pending questions, approvals and backlog decisions resolved in the pane vs. in the session (`in_pane / (in_pane + in_session)`) | To be measured in Epic 5 telemetry (today 0% for questions on visible sessions: no pane path exists) | >= 60% of questions and >= 80% of approvals resolved in-pane after two weeks of use with the flags on | `notification_inline_action_total{outcome="APPLIED"}` vs. `guidance_question_answer_latency_seconds` rows whose answer arrived by terminal (`guidance_mirror_cleared_total{reason="answered_in_terminal"}`) |
| Answered latency (created_at to answered_at, median and p90) per source | To be measured in Epic 5 telemetry | p90 for pane-answered questions at least 50% below the p90 for terminal-answered ones, once both are measured | `guidance_question_answer_latency_seconds{source,route}` |
| Background tab opens per week that end in **Open** on a row (tab is used, not just visited) | To be measured in Epic 5 telemetry (today the tab has never shown a row) | Reported only; no target until a baseline exists | `background_activity_rows` plus the client-event sink |
| **Epic 4 cost outcome (runtime, not a grep guard):** wake deliveries per session and `ScheduleWakeup` tool calls per session | Baseline `ScheduleWakeup` calls per backlog session is captured by plan Task 5.1f from the counter running with `agent_wake_triggers` OFF, for at least 7 days, before any scope is turned on; wake deliveries start at 0 | Provisional: `ScheduleWakeup` calls per backlog session fall by at least 50% two weeks after the prompts switch to `schedule_wake`; no session exceeds the per-owner hourly limit (`wake_delivered_per_owner_hour` <= 6) | `wake_fires_per_session` (histogram on session end) and `agent_schedule_wakeup_calls_total{role}` counted from the PostToolUse receipt's tool name (plan Task 5.1e; UNVERIFIED that the injected matcher covers `ScheduleWakeup`, probed with Spike S7 item (6)) |

The phone caveat: a `live_keystroke` question on the remote listener is "Answer from desktop" in v1 (`design/ux.md` 2.4), so it is counted as resolved in-session by design; the in-pane share is reported both overall and for the local listener alone.

### Paired quality metric (so "1 interaction" cannot be met by answering wrongly)

| Metric | Definition | Target |
|---|---|---|
| Delivery-failure rate | `(NOT_SENT + SEND_INDETERMINATE + STALE-after-click) / answers attempted`, from `notification_inline_action_total{kind="ANSWER_QUESTION"}` | <= 2% over a rolling week, baseline to be measured in Epic 5 telemetry |
| Wrong-answer proxy | Share of inline actions followed by an **Undo** (commit-delay actions) or by the operator opening the same session within 60 s of the action (`reopen_after_action_total`) | <= 5%, baseline to be measured in Epic 5 telemetry; a rise after a UI change blocks the next flag flip |
| Unconfirmed rate | Answers still `sent` and unacknowledged at 60 s / answers sent | <= 3%; a high rate means the acknowledgement signal (ADR-005) is blind, not that answers fail |

A usability validation plan (ten scripted tasks T1 to T10 including three hostile trials, success criteria, and what failing means for each flag flip) is in `design/ux.md` section 12 and is executed in plan Task 5.3c.

### Decision rule for flag defaults (who decides, and on what)

The owner decides every default flip; the plan never flips one automatically. For each flag, after at least 14 days of dogfood with the flag on (the soak in plan Story 5.3). The first two bullets are the rule for the two answer flags; `background_activity_feed` and `agent_wake_triggers` have their own bullets after them:

- **Flip default ON** only when all three quality metrics above met their targets over the last 7 days (delivery-failure <= 2%, wrong-answer proxy <= 5%, Unconfirmed <= 3%) **and** no usability task T1 to T10 failed twice in a row. The in-pane share is then reported to the owner as the A1 evidence, not as a gate.
- **Keep default OFF** when any quality metric missed its target in that window, a hostile trial (T8 to T10) failed, or fewer than 20 actions were recorded (too little data); the flag stays live-settable for continued dogfood.
- **The three paired quality metrics gate the two answer flags only** (`notification_inline_actions`, `askuserquestion_structured`). The other two flags have their own criteria, below. The owner still decides every flip.
- **`background_activity_feed` (ships ON; the question is whether it stays ON).** Keep ON when, over the 14 days: (i) no open of the tab rendered an unexplained empty state (`background_tab_empty_total{variant}` shows only the explained variants), (ii) the Task 1.5b alert ("outcome evidence is empty") did not fire, or the owner accepted `stopped_unknown` rows as the permanent shape, and (iii) the owner reported no phantom badges (the accepted per-browser limitation, `design/ux.md` 1.4). Open-on-row is reported, never a gate. Any failure of (i) flips the flag OFF (legacy selector) and files a defect.
- **`agent_wake_triggers` (per scope, in the rollout order `trigger:at`/`cron`, `guidance_answered`, event and state types, `tool:request_help`).** Flip a scope ON when, over its 14-day soak: (i) zero wakes were lost after a restart (`wake_trigger_total{state="missed"}` after any restart is 0, or every miss was an operator-visible expiry), (ii) `wake_duplicate_delivery_total` is 0, (iii) `wake_delivered_per_owner_hour` never exceeded the per-owner limit and no limit-hit digest came from real use, and (iv) for the scope that retires polling, `agent_schedule_wakeup_calls_total{role}` per backlog session is at least 50% below the baseline captured before the flag was on (plan Task 5.1f; provisional, the owner sets the number). The "fewer than 20 actions" floor does not apply to wakes: fewer than 10 wake deliveries in the soak keeps the scope OFF by default, and the owner may still flip it by an explicit decision recorded in the flip PR (no metric flips it).
- **A1 fallback is owner-triggered only.** If the in-pane share stays under the provisional targets after the 14 days, the plan reports it and waits; cutting scope (Epic 3 to read-only, Epic 4 operator surface to visibility) happens only when the owner says so. No metric triggers it.

### Risky assumptions and how each is tested

| # | Assumption | Why it could be false | How it is tested | If false |
|---|---|---|---|---|
| A1 | The operator prefers resolving prompts in the web pane over Slack, the phone's push notification, or just opening the session | The existing channels already reach the operator; a third surface may be ignored | Adoption target above (in-pane vs. in-session share), measured after the Background fix (Epic 1) and the approvals flag (Epic 2) ship, before Epic 3 and Epic 4 default flips; the usability script (`design/ux.md` section 12) run on desktop and phone | The flags stay OFF by default; Epic 3 ships read-only rendering only and Epic 4's operator surface is cut to Scheduled-prompts visibility, **only when the owner decides** (the fallback is owner-triggered, never automatic) |
| A2 | Peer help has real demand: sessions would actually ask each other for help instead of asking the operator or finishing alone | No telemetry exists today for how often sessions need a second opinion; the 10 `review:*` and 1 `diagnose:*` sessions are mostly independent | `peer_help_total{state}` after `tool:request_help` is enabled, reviewed after two weeks: fewer than 5 accepted requests in that window means peer help is unproven | `request_help peer=...` is left behind its own scope (`tool:request_help`) and `PeerHelpRow`/escalator work (Story 4.11) is not given further investment; `peer=human` stays as the alias for `create_guidance_request` |
| A3 | A fixed keystroke (one digit, no Enter) reliably answers a Claude `AskUserQuestion` across Claude Code updates | The dialog layout is pinned to one fixture (v2.1.296) | Layout-drift canary and `STALE` share alert (plan Task 3.3h); delivery-failure rate above | Tier B keystroke answers for hook-mirrored questions degrade to read-only (question, options, Open session; `hook_reply` still answers if enabled); agent-created questions are unaffected |
| A4 | Hidden review/triage sessions record verdicts on `ItemSession` often enough that `Finished` rows exist | Not checked against the live DB | Spike S7 item (8); alert if `finished` is 0 while `stopped_unknown` > 0 for 24 h (Task 1.5b) | The feed ships as `stopped_unknown` rows (still non-blank) |

### Prioritization rationale per epic (ICE, INFERRED; the owner may re-score)

Impact and Confidence are scored 1 to 5, Effort is the plan's price-weighted CU with verification (plan "Effort estimate"). Score = Impact x Confidence / Effort in M CU, shown to one decimal; it explains the shipping order and is not a gate.

| Epic | Impact (why) | Confidence (why) | Effort | Score | Order |
|---|---|---|---|---|---|
| 1 Background feed | 3: repairs a surface that shows nothing today, but only informs | 5: root cause VERIFIED against the live service | 2.6 M CU | 5.8 | First |
| 2 Inline actions | 5: removes the open-session round trip for the most frequent items (approvals, decisions) | 3: A1 (operator prefers the pane) is untested | 5.0 M CU | 3.0 | Second |
| 3 Structured AskUserQuestion | 4: answerable options instead of terminal text | 2: S1, S5 open; A3 (keystroke stability) | 5.9 M CU | 1.4 | Third |
| 4 Messaging and wakes | 4: retires polling debt and bounds cost ($106.69 incident), but benefits mostly headless sessions | 2: A2 (peer-help demand) unproven, most unknowns | 7.5 M CU | 1.1 | Last |
| 0 and 5 | Enabling work (spikes, flags, observability, rollout); no independent score | | | | With their epics |

## Size
XL (about 24.4 M CU at the x1.75 verification multiplier) per ESTIMATION.md bands, informational (INFERRED); the earlier "Large (3-15M CU)" band is superseded by the plan's "Effort estimate". Full scope is the default; not a gate.

**Wall-clock blockers**: named per story, with the calendar chain, in the plan's "Effort estimate" (live spikes S1, S2, S3, S5; the S4 CI run; owner confirmation of the nine documented defaults at the Phase 5 start gate; per-flag dogfood soak; real-device checks; the single deploy window, since `make install-service` kills tmux without `--tmux-keep-server` and testing uses the manual-instance procedure in CLAUDE.md).

## Constraints
- Do not restart the live `:8543` service to test; use the isolated manual instance (ports 62871/62872, private tmux socket).
- Reuse `durable-guidance-request` entity/RPCs, the delivery gate (`server/deliverygate`), and the `OmnibarAction`/registry conventions; no parallel question store.
- Rollout flags must be live-settable via the feature-flag RPC/panel, never env vars.
- Mobile + desktop UX for every UI feature (touch targets, responsive layout).
- Hidden-session auto-revival rule (`noarchivedrevival`) applies to any wake path.
- No force push; web-app uses pnpm; mark generated code gitignore policy (proto `gen/`, ent) as in CLAUDE.md.

## Non-functional Requirements
- **Performance SLO**: pane renders ≤200ms from cached state; action round-trip p95 <1s on localhost.
- **Scalability**: tens of sessions, hundreds of notification records.
- **Security classification**: internal; actions that send keystrokes/approvals into sessions must keep the existing approval-gate and caller-identity checks (MCP surface needs server-side gate per prior instinct).
- **Data residency**: local only.

## Scope
### In Scope
1. Diagnose and fix the Background tab; activity feed + pinned exceptions (operator decision); explanatory empty state.
2. Current-state inventory (what exists across notifications, guidance requests, hidden reply, approvals, backlog actions, MCP tools).
3. Competitive/harness survey: agent-management tools (e.g. Conductor, Claude Squad, Crystal, Vibe Kanban, Cursor background agents, Devin, Codex cloud, GitHub Copilot agent, Sculptor, etc.) and what Claude Code (hooks: `Notification`, `PreToolUse` for AskUserQuestion, `Stop`; SDK permission/`canUseTool`; `ScheduleWakeup`/cron) and each supported harness expose for structured prompts, resumption, and scheduling.
4. Vision doc + ADRs for: inline-action framework, AskUserQuestion integration, messaging/wake-trigger substrate.
5. Implementation of the above in phased, flag-gated increments, with tests.
6. Added from UX research and **kept in scope** (decided; no owner answer is pending; the owner may veto at review): operator "Remind me" on informational Background rows (default presets "In 1 hour", "Tomorrow 9:00", "Pick time...", changeable in config) and an in-pane "Resume scheduled prompts" control (operator-facing name of "Resume wakes") for a paused `agent_wake_triggers` flag (inline confirm, local listener only, audit-logged). Both are scheduled in the last wave and are optional-to-order, so they never delay other stories; being late is a scheduling fact, not an open decision.
### Out of Scope
- Slack/email/push channel redesign (existing `slack-review-notifications`, `web-push-notifications` plans stand).
- Multi-user/auth or team sharing.
- Replacing tmux transport.
- Free-form agent chat: a help request is one question and one answer on a `GuidanceRequest` row, not a conversation thread between agents or between an agent and the operator.
- Cross-instance peers: a peer is a session of the same ssq instance and workspace; cross-workspace and cross-instance requests are rejected at call time.
- Wakes for harnesses that cannot load an MCP config carrying the per-session credential (non-MCP harnesses): they fail closed with `identity_unverified`. The operator's "Remind me" needs no agent credential and still works for their sessions.

## Rabbit Holes
- Answering a *live* Claude Code AskUserQuestion from outside the TUI: tmux keystroke injection vs. hook-mediated answer (`PreToolUse` deny-with-answer) vs. SDK `canUseTool`. Verify what each harness supports before designing.
- Hidden-session records may never be produced (gate suppression vs. producers); fixing the tab without fixing production yields a still-empty feed.
- Durable wake triggers vs. a session that is stopped/archived (revival policy).
- Ent schema/proto changes force `make proto-gen`/`ent-gen`; generated output stays uncommitted.
- jscpd threshold margin (0.14%) — heavily-mocked new test files can trip it.

## Alternatives Considered
- Adopt an off-the-shelf agent-management UI (covered by the survey); keep native since sessions/backlog/gate are tightly coupled.
- Event-bus subscription vs. extending `ScheduleWakeup` polling (polling rejected: ssq#428).

## Feasibility Risks
- Harness parity: non-Claude harnesses may lack structured question hooks; degrade to terminal-text detection + keystroke reply. They may also be unable to load a per-session MCP credential, in which case messaging/wake tools fail closed for them (Spike S5).
- Spike S4 may find no PR/CI event source; PR/CI wake triggers then ship as a typed `unavailable` error.
- Operator-only answerer for guidance in v1 per prior plan; agent-submitted answers (peer help) needs a new, gated MCP tool.

## Observability Requirements
- Metrics/log fields: inline-action invoked/succeeded/failed by type; question created→answered latency; wake scheduled/claimed/delivered/missed by trigger type; Background row counts by state; delivery-gate stats remain authoritative for suppression. Quality: `reopen_after_action_total`, Undo count, `guidance_answer_delivery_total{state}` (see Paired quality metric).  Added in triad round 2: `background_reviewed_mark_total{storage}` (cost of the accepted per-browser reviewed-mark limitation), `wake_fires_per_session` and `agent_schedule_wakeup_calls_total{role}` (Epic 4 runtime cost outcome).
- Alert condition: wakes missed after restart >0; actions failing delivery to session.

## Risk Control
- Feature flags (live-settable, global + per-scope): `notification_inline_actions`, `askuserquestion_structured`, `agent_wake_triggers`, `background_activity_feed`.
- Rollback: flip flag off; data additions are additive (no destructive migration).
- Staged rollout: Background fix first (lowest risk), then read-only structured rendering, then answer paths, then wake triggers.

## Open Questions
1. ~~Why do hidden review/diagnose sessions produce no history records at all?~~ **Resolved** (by code reading; the live-DB confirmation is Spike S7 item (8)): the delivery gate classes routine events (TASK_COMPLETE, PROCESS_FINISHED) as suppressed for hidden sessions (`server/deliverygate/policy.go` `typeClass`), so the history never holds them; the tab therefore derives rows from the hidden-session list and a server-computed `background_outcome`, and the gate is left unchanged. See `decisions/ADR-001-background-feed-from-hidden-session-list.md` (decisions 1, 3, 6) and plan Story 1.7.
2. Which harnesses support a native answer channel for AskUserQuestion vs. need keystroke injection? **Still open**, closed by Spikes S1 (Claude hook answer) and S5 (other harnesses); the fallback ladder is `decisions/ADR-005-askuserquestion-answer-routing-ladder.md`.
3. ~~Should peer help requests reuse the `GuidanceRequest` entity or a separate message entity?~~ **Resolved**: reuse, with `source=peer`, `answerer_kind=session` and additive columns; no separate store. `decisions/ADR-003-extend-guidance-request-as-question-and-help-store.md`; plan glossary `PeerHelpRequest`.
4. ~~Where do wake triggers persist and who evaluates them?~~ **Resolved**: a new ent table `wake_triggers`, evaluated by one `server/wakes` dispatcher (one EventBus consumer plus its own `min(next_fire_at)` timer on an injected `Clock`); `workflows.Scheduler` is not reused. `decisions/ADR-006-durable-wake-trigger-table-and-single-dispatcher.md`.
