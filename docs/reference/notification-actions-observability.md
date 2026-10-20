# Notification actions and agent messaging: metrics, queries and alerts

Reference for every metric the notification-actions-and-agent-messaging project
exports, the queries that judge a flag flip, and the alerts that exist. Plan:
`project_plans/notification-actions-and-agent-messaging/implementation/plan.md`
("Observability Plan", Story 5.1). Companion pages:
`docs/how-to/verify-notification-actions-rollout.md`,
`docs/how-to/flag-default-flip-pr-template.md`,
`docs/how-to/run-notification-usability-script.md`.

## Conventions

- Names are the registered OpenTelemetry (OTel) instrument names. The meter is
  a no-op until telemetry is on (`OTEL_ENABLED=true`, `docs/how-to/enable-opentelemetry.md`),
  so an instance without it exports nothing. Your backend may add unit or type
  suffixes to these names; check the series name in your backend before pasting a
  query (UNVERIFIED: no backend was queried for this page).
- Every label value is lowercase, as registered. The plan's Given/When/Then
  examples write `APPROVE` and `APPLIED`; the code writes `approve` and `applied`
  (`server/services/notification_action_service.go:509`, `outcomeLabel` at line 550).
- The label `kind` always means the action kind (`ActionKind`), never a session
  role or a notification type (plan, "The word kind").
- Client events (`background_activity_rows`, `notification_pane_open_ms`,
  `background_reviewed_mark_total`) go to the analytics sink through
  `POST /api/analytics`, not to OTel. Only `inline_action_undo_total` is also
  counted into OTel (`server/analytics/quality_metrics.go`).

## Action metrics

| Metric | Type, unit | Labels and values | Registered in |
|---|---|---|---|
| `notification_inline_action_total` | counter | `kind`: `open`, `approve`, `deny`, `answer_question`, `backlog_decision`, `answer_guidance`, `cancel_wake`, `cancel_peer_help` (`unspecified` for an unset kind). `tier`: `a`, `b`, `unknown` (`unknown` when an `answer_question` route cannot be resolved). `outcome`: `applied`, `already_resolved`, `stale`, `not_sent`, `send_indeterminate`, `rate_limited`, `forbidden`, `flag_off`, `invalid`, plus `error` for an infrastructure fault. `channel`: `web_local`, `web_remote`, `unknown` (the comment in the metrics file also names `mcp`, but no code sets it). | `server/services/notification_action_metrics.go` (`defaultActionMetrics`) |
| `notification_inline_action_duration_seconds` | histogram, seconds; buckets 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10 | `kind` (same values) | same |
| `inline_action_undo_total` | counter | `kind`: `approve`, `deny`, `answer_question`, `backlog_decision`; anything else is `unknown` | `server/analytics/quality_metrics.go` |
| `reopen_after_action_total` | counter | `kind`: always `answer_question` | `server/services/notification_quality_metrics.go` |
| `human_active_refusal_total` | counter | none | `server/services/human_activity.go` |

Notes:

- `notification_inline_action_latency_ms` no longer exists. It was renamed to
  `notification_inline_action_duration_seconds` (commit `0c1f8d633`). Update any
  saved query.
- Only the `kind` label exists on the duration histogram. It has no `outcome`
  or `channel` label.
- `inline_action_undo_total` comes from the client event `inline_action_undo`
  (`web-app/src/lib/hooks/useInlineActionTelemetry.ts`), sent to `POST /api/analytics`
  when the operator presses Undo during the 3 s commit delay. The server never sees
  that action, so the client is the only source. Approve, Send back and Mark done are
  the only actions that wait behind Undo (`web-app/src/components/ui/notification-actions/actionSpec.ts`,
  `holdKeyOf`).
- `reopen_after_action_total` counts an `applied` `ANSWER_QUESTION` followed by the
  operator's web terminal attaching to that session within 60 s
  (`reopenWindow` in `notification_quality_metrics.go`). Only `ANSWER_QUESTION` carries
  a session id, so approve, deny and backlog decisions are not tracked. A terminal
  stream that is already open produces no new attach and no count. The tracker is
  in process and resets on restart.

## Answer delivery metrics

| Metric | Type, unit | Labels and values | Registered in |
|---|---|---|---|
| `guidance_answer_delivery_total` | counter | `route`: `hook_reply`, `live_keystroke`, `resume`, `read_only`. `state`: `pending`, `sent`, `acknowledged`, `delivery_failed`, `saved_for_later`, `stale`, `aborted_partial`. | `session/guidance_answer_metrics.go` (counted in `SetDelivery`, `session/ent_repository_guidance_mirror.go`) |
| `guidance_question_answer_latency_seconds` | histogram, seconds (default buckets) | `source`: `agent_mcp`, `ask_user_question_hook`, `peer`. `route`: as above. | `session/guidance_answer_metrics.go` |
| `guidance_answer_unconfirmed_total` | counter | none | `server/services/notification_quality_metrics.go`, incremented in `server/services/guidance_ack_detector.go` |
| `guidance_mirror_cleared_total` | counter | `reason`: `session_ended`, `answered_in_terminal`, `dialog_gone`, `dialog_replaced` | `server/deliverygate/metrics.go` |
| `guidance_mirror_failed_total` | counter | `reason`: `timeout`, `error`, `cap_exceeded`, `no_payload`, `invalid_payload` | same |
| `question_dialog_match_total` | counter | `result`: `match`, `mismatch`. `claude_version`: always `unknown` today. | same |

Notes:

- `guidance_answer_unconfirmed_total` counts keystroke answers that enter the
  Unconfirmed sub-state: an indeterminate write, or no acknowledgement at 60 s
  (`ackDecideAfter`). It has no `route` label.
- `claude_version` is `unknown` in production. `DialogMatchCanary.SetVersionSource`
  has no caller outside tests (`grep -rn SetVersionSource server`), so no version
  source is wired.
- The plan lists `indeterminate` as a `result` value. The code defines only `match`
  and `mismatch` (`server/services/guidance_dialog_canary.go:26-27`).
- `guidance_mirror_cleared_total{reason="answered_in_terminal"}` is incremented from the
  `AskUserQuestion` PostToolUse receipt (`ObservePostToolUse` in
  `server/services/guidance_ack_wiring.go`). That receipt exists only for sessions that
  run the `post_tool_logging` hook; see "Wake cost baseline".

## Wake and messaging metrics

| Metric | Type, unit | Labels and values | Registered in |
|---|---|---|---|
| `wake_trigger_total` | counter | `type`: `at`, `cron`, `backlog_event`, `pr_event`, `ci_event`, `session_state`, `guidance_answered`. `state`: `claimed`, `delivered`, `needs_revive`, `missed`, `skipped`, `cancelled`. | `server/wakes/metrics.go` |
| `wake_fire_lag_seconds` | histogram, seconds (default buckets) | `type`: as above. Claim time minus `next_fire_at`. | same |
| `wake_reconcile_total` | counter | `result`: `requeued_claimed`, `fired`. | same |
| `wake_limit_hit_total` | counter | `limit`: `min_interval`, `pending_per_owner`, `max_fires`, `expires_horizon`, `generation`, `fires_per_owner_hour`, `fires_global_hour`, `peer_open_per_asker`, `payload_size`, `debounce`, `busy_hold`, `revive_lock`. `phase`: `create`, `fire`. No code increments the `debounce`, `busy_hold` or `revive_lock` values, so those series are always empty. | same; constants in `server/wakes/limits.go` |
| `wake_needs_revive_age_seconds` | gauge, seconds | none. Age of the oldest `needs_revive` row. | same |
| `wake_duplicate_delivery_total` | counter | none | same |
| `wake_reconcile_digest_total` | counter | none | same |
| `peer_help_total` | counter | `state`: `waiting`, `answered`, `rejected`, `not_sent`, `escalated`. | `server/wakes/metrics.go`; incremented in `server/mcp/tools_peer_help.go` and `server/wakes/escalator.go` |
| `mcp_caller_gate_denied_total` | counter | `tool`, `role` (`work`, `triage`, `review`, `diagnose`) | `server/wakes/metrics.go`; incremented in `server/mcp/caller_identity_gate.go` |
| `mcp_identity_unverified_total` | counter | `tool` | same |
| `wake_fires_per_session` | histogram; buckets 0, 1, 2, 3, 5, 8, 13, 21, 34, 55 | none. One sample per session, taken when the session is deleted or archived. | `server/wakes/metrics_session.go` |
| `agent_schedule_wakeup_calls_total` | counter | `role`: `work`, `triage`, `review`, `diagnose`, `jules_work`, `external`, `unlinked`, `unknown`. | `server/services/notification_quality_metrics.go` |

Notes:

- `wake_reconcile_total` increments only `requeued_claimed` and `fired`. The
  constants `missed` and `needs_revive` exist in `server/wakes/metrics.go` but nothing
  increments them (`grep -rn ReconcileMissed server`). Use `wake_trigger_total` for
  missed and `needs_revive` counts.
- `wake_trigger_total` has no `scheduled` or `pending` state. The plan lists
  `scheduled`; the code does not emit it.
- `peer_help_total` has no `cancelled` series. `cancelled` appears only as the derived
  state `ListPeerHelpRequests` returns (`server/services/wake_trigger_peer_help.go:20`).
- `wake_fires_per_session` is an in-process tally. A restart or a dispatcher stop and
  start drops the counts of sessions that are still live, and a session that was
  never tallied records a sample of 0 (`SessionEnded`).
- `agent_schedule_wakeup_calls_total` resolves `role` from the backlog item session
  (`roleOf`): `unlinked` when the session has no backlog link, `unknown` on a lookup
  error. It also writes a `schedule_wakeup_receipt` JSON log line with `session` and
  `role`, which `scripts/wake-baseline.sh` reads because the counter resets on restart.

## Client events

These reach the analytics sink at `POST /api/analytics` (`server/handlers/analytics_handler.go`).
Read them with `GET /api/analytics/summary` or the analytics store. They are not OTel
series.

| Event | Source | Fields |
|---|---|---|
| `background_activity_rows` | `web-app/src/lib/hooks/useBackgroundTelemetry.ts` | One label per activity state that has rows (`running`, `finished`, `stopped_unknown`, `exception_failure`, `exception_needs_human`, `departed`), the value being the row count as a string. Sent once per landed poll, only when there is at least one row. |
| `notification_pane_open_ms` | `web-app/src/lib/hooks/usePaneOpenTiming.ts` | `durationMs`: open request to the first commit with an action row. One sample per open. |
| `background_reviewed_mark_total` | `web-app/src/lib/hooks/useReviewedMarks.ts` | `storage` label. |

## Plan metrics that are not implemented

Do not query these. `grep` of `server/`, `session/` and `web-app/src` found no
registration (checked at commit `3eea6139e`).

| Plan metric | Status |
|---|---|
| `background_tab_empty_total{variant}` | Not implemented. No emitter exists. |
| `wake_delivered_per_owner_hour` | Not implemented as a series. `Dispatcher.DeliveredPerOwnerHour` (`server/wakes/limits.go`) computes the value on demand and no code exports it. Flip criterion (iii) cannot be read from a metric; use `wake_limit_hit_total{limit="fires_per_owner_hour"}` and the limit-hit digest notification as the proxy. |
| `notification_inline_action_latency_ms` | Removed. Use `notification_inline_action_duration_seconds`. |
| `background_activity_rows{state}` as an OTel series | Not an OTel series. It is a client event whose labels are the state counts (see above). |
| Alert: `notification_inline_action_total{outcome=~"not_sent\|send_indeterminate"}` above 3 in 10 min raises "actions failing delivery to session" | Not implemented. No notification with that text exists. |

## Queries

Window `[7d]` matches the soak in `project_plans/notification-actions-and-agent-messaging/requirements.md`.

Delivery-failure rate of answers:

```promql
sum(increase(notification_inline_action_total{kind="answer_question",outcome=~"not_sent|send_indeterminate|stale"}[7d]))
/
sum(increase(notification_inline_action_total{kind="answer_question"}[7d]))
```

Unconfirmed rate:

```promql
sum(increase(guidance_answer_unconfirmed_total[7d]))
/
sum(increase(guidance_answer_delivery_total{state="sent"}[7d]))
```

`state="sent"` counts every route that reaches `sent`. Add `route="live_keystroke"`
to the denominator to match the numerator, which counts keystroke answers only.

In-pane share for questions, `A / (A + B)`:

```promql
# A: questions answered from the notification pane
sum(increase(notification_inline_action_total{kind="answer_question",outcome="applied"}[7d]))
# B: questions answered in the terminal
sum(increase(guidance_mirror_cleared_total{reason="answered_in_terminal"}[7d]))
```

B counts only hook-mirrored questions, and only for sessions that send the
`AskUserQuestion` PostToolUse receipt. A counts every channel. Read the ratio as
a lower bound on terminal answers.

In-pane share for approvals is UNMEASURABLE. No counter records an approval
answered in the terminal, so there is no B for approvals. Closing the gap needs a
counter for terminal-side approval resolution; none exists.

Wrong-answer proxies:

```promql
# Undo rate. backlog_decision includes Merge, which has no Undo, so this understates the rate.
sum(increase(inline_action_undo_total[7d]))
/
(sum(increase(inline_action_undo_total[7d]))
 + sum(increase(notification_inline_action_total{kind=~"approve|backlog_decision"}[7d])))

# Reopen rate
sum(increase(reopen_after_action_total[7d]))
/
sum(increase(notification_inline_action_total{kind="answer_question",outcome="applied"}[7d]))
```

Undone actions never reach the server, so the undo denominator adds them back.

Latency SLO, p95 under 1 s on localhost:

```promql
histogram_quantile(0.95,
  sum by (le) (rate(notification_inline_action_duration_seconds_bucket[5m]))) < 1
```

Wake cost:

```promql
# ScheduleWakeup calls by role
sum by (role) (increase(agent_schedule_wakeup_calls_total[7d]))

# Median wake deliveries per ended session
histogram_quantile(0.5, sum by (le) (increase(wake_fires_per_session_bucket[7d])))

# Deliveries by trigger type
sum by (type) (increase(wake_trigger_total{state="delivered"}[7d]))

# Missed wakes (flip criterion i)
sum(increase(wake_trigger_total{state="missed"}[7d]))
```

## Alerts

Alerts are operator notifications, not pages. These exist:

| Condition | What the operator sees | Source |
|---|---|---|
| Triggers missed or skipped during a restart reconcile | One failure notification per reconcile. The message gives the count, up to 5 session names, and the trigger ids (up to 5, then "and N more"). An owner already digested in the last hour is left out. | `digest.build` in `server/wakes/notify.go` (`digestOwnersListed = 5`, `digestOwnerInterval = time.Hour`) |
| A pending trigger past `expires_at` | One failure notification per trigger, naming its id. `ListWakeTriggers` sweeps expired rows as missed before it lists (`wakes.ExpireOverdue`, `server/wake_trigger_wiring.go:26`), so this works while the dispatcher is stopped. The next dispatcher start also marks them missed in its reconcile. | `server/wakes/expire.go` |
| A `needs_revive` row waits more than 1 h | "Scheduled prompt waiting to revive a session", once per row | `needsReviveAlertAge` in `server/wakes/reconcile.go` |
| A wake limit is hit | One failure digest per owner per hour | `notifyLimitOnce` in `server/wakes/limits.go` |
| More than half of at least 5 dialog-match attempts in 1 h mismatch | "Question answers paused"; hook-mirrored question rows degrade to read only | `server/services/guidance_runtime_wiring.go`, `server/services/guidance_dialog_canary.go` |
| `finished` is 0 while `stopped_unknown` is above 0 | "Background outcome evidence is empty" | `web-app/src/components/ui/NotificationPanel.tsx:371` |
| A peer-help request is unanswered for 10 min, or its answerer is gone | One needs-human notification per request | `server/wakes/escalator.go` |

## Wake cost baseline (Task 5.1f)

Flip criterion (iv) in `requirements.md` compares `ScheduleWakeup` calls per backlog
session after the flip against a baseline captured while `agent_wake_triggers` is off.
`docs/how-to/flag-default-flip-pr-template.md` requires the row below to carry real
numbers before an `agent_wake_triggers` flip PR passes review.

### How to capture it

```bash
scripts/wake-baseline.sh [--days N] [--config FILE] [--sessions-file FILE] [LOG...]
scripts/wake-baseline.sh --self-test
```

- The script reads the `schedule_wakeup_receipt` lines of the JSON log. The default
  log is `~/.stapler-squad/logs/staplersquad.log`, or the `instances/$STAPLER_SQUAD_INSTANCE/logs/`
  one when that variable is set. Pass workspace-mode logs explicitly.
- It exits 2 and prints no numbers when the observed window is shorter than `--days`
  (default 7) or when `agent_wake_triggers` is on, globally or in any scope.
- Log rotation limits the window: the earliest record in the log is the earliest the
  window can start. Pass the rotated files too.
- Without `--sessions-file`, the population is only sessions that made at least one
  call, which overstates the median. To count sessions with zero calls, pass a file
  with one session id per line for every session alive in the window.
- It needs `bash`, `jq`, `grep` and `tac`.

### Receipt coverage

The receipt is the `PostToolUse` hook that the `post_tool_logging` hook injects
(`HookPostToolLogging`, `server/services/hook_injector.go:26`). Its endpoint is
`/api/hooks/post-tool-use`, and `ObservePostToolUse` counts the `ScheduleWakeup`
tool name (`server/services/guidance_ack_wiring.go:82`).

Nothing injects `post_tool_logging` by default. The only callers of `InjectHooksConfig`
are the MCP `create_session` tool, which injects the names in its `hooks` argument
(default `permission_approval` and `stop_notification`,
`server/mcp/tools_lifecycle.go:70,243`), `server/mcp/tools_github.go:273` (no hook
names), and the triage spawn path, which injects only `git_drift_check`
(`server/services/backlog_service_triage.go:1145`). A session counts toward the
baseline only if it was launched with `post_tool_logging`, or if a hook outside this
repository posts to the same endpoint (UNVERIFIED for the operator's own
`~/.claude/settings.json`).

Spike S7 item 6 showed that the generated matcher fires for `ScheduleWakeup`
(`project_plans/notification-actions-and-agent-messaging/research/spike-S7.md:79`).
The gap is injection, not the matcher.

If the capture returns no calls because no session ran the hook, the metric has no
instrument. In that case Task 5.1f says flip criterion (iv) is dropped by the owner,
not silently. Record that decision in `requirements.md` and in the flip PR. This page
does not make it.

### Baseline row

| Date captured | Session count | Window (days) | `agent_wake_triggers` | Median calls per session | p90 calls per session | Captured by |
|---|---|---|---|---|---|---|
| NOT CAPTURED | - | - | - | - | - | - |

The operator replaces the row with the script's `sessions`, `window_days`,
`calls_per_session_median` and `calls_per_session_p90` lines, plus the date. Until
then the flip PR check fails by design.
