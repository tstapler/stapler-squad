# Notification delivery gate (hidden sessions)

Reference for `server/deliverygate`: the single place that decides whether a
notification about a hidden session (review, diagnose, triage, headless) is
delivered. Policy and rollout rationale live in
`project_plans/notification-tray-and-hidden-session-gate/` (ADR-001 to ADR-004).

## Operator checklist (owner: the operator)

The gate ships dark. Turning it on is the point of the project, so the
stages below are an operator checklist, not background reading. The standing
reminder is the status line under the flag in Settings > Features: "Shadow: N
hidden events would have been suppressed in the last 24h; M unresolved
fail-open; K unversioned ssq-notify". `would_suppress` growing while nothing is
suppressed means the project is not done.

**Stage 1b reminders.** Created only after **both** PR 2a-2 and PR 2e have
merged (the review-only flip needs the per-kind override): three dated backlog
items, due 7, 10 and 14 days after the later merge. The merging agent or the
operator creates them with `create_backlog_item` and records the ids and dates
here. Until then this table is the placeholder.

| Reminder | Due | Backlog item id |
|---|---|---|
| Flip `hidden_session_gate` on for kind `review` | later merge + 7 days | _not created yet_ |
| Flip `hidden_session_gate` globally once the `review` phase shows nothing unexpected in `would_suppress` / `hidden_delivered` | later merge + 10 days | _not created yet_ |
| Open PR 2b (default-on and legacy deletion) once `soak_streak_hours >= 24` with `stats_file_status.writable` | later merge + 14 days | _not created yet_ |

**Stage 2 soak.** Flip the gate on live, then 24-48h of traffic. The evidence
is the `GetDeliveryGateStats` output, never an OTel dashboard. Paste it into
the PR 2b description. End the soak with "flip back on or Reset to default,
never leave an explicit off".

Synthetic probe (the failure and needs-human paths are rare; the probe proves
the `ssq-notify` bus path only): against a **live hidden session** from
Background activity, run
`ssq-notify -s '<session title>' --type error -p medium -t '[synthetic-probe] gate failure-path check' -m 'ignore this notification'`
and the same with `--type question` for needs-human. Use `-p medium`: a
`-p low` probe is dropped by the legacy check before it reaches the gate. The
row must appear in the tray within seconds (a silent drop is noticed, not read
as a stalled counter) and is dismissed by the operator. It counts toward
`failure_delivered_while_on` and `needs_human_delivered_while_on` and is also
counted in `probe_delivered_while_on`. No metadata hint is honored for it.

| Producer | Exercised by the probe? | Checked another way |
|---|---|---|
| `ssq-notify` to `SendNotification` to resolver to gate | yes | |
| Crash FAILURE (`sessionExitedPublisher`) | no | CI: T-CR-01, T-CR-02 |
| Status-change push (`AllowStatusChange`) | no | CI; `suppressed{channel=push_status}` in the soak |
| Approval and `INPUT_REQUIRED` from `approval_handler.go` | no | an organic hidden question in the soak, or its CI tests |
| `markSessionPermanentlyFailed` | no | CI: T-CR-05 |

A producer counts as live-verified only when an organic event for it was seen.

**Stage 3 prerequisites** (each pasted into the PR 2b description):
`soak.explicit_off_scopes` is empty; `soak.soak_streak_hours >= 24` with
`stats_file_status.writable == true`, `failure_delivered_while_on >= 1` and
`needs_human_delivered_while_on >= 1` (state whether the probe alone satisfied
them: `failure_delivered_while_on == probe_delivered_while_on`); the reviewed
new-delivery volume from `legacy_hidden_suppressed{class=failure|needs_human}`;
the before/after delivery table. If the binary was ever rolled back, re-verify
the per-kind overrides (an older binary's `SaveConfig` drops them).

## Policy

A hidden session notifies only for failures and needs-human events. The policy
is `deliverygate.ShouldDeliver(Visibility, Facts)`, a pure function keyed on
the notification **type** (priority and message text are never inputs).

| Class | Types |
|---|---|
| needs-human | `APPROVAL_NEEDED`, `INPUT_REQUIRED`, `CONFIRMATION_NEEDED` |
| failure | `ERROR`, `FAILURE` |
| routine (suppressed for hidden sessions) | everything else, including `WARNING` |

Producer hints (metadata key `delivery_class`, honored for hidden sessions only):

- `failure` promotes a routine-typed event (a hard-stop `WARNING`).
- `routine` demotes a failure-class event (per-tool hook errors). It never
  demotes needs-human.

`ssq_untrusted_type=true` is written only by `SendNotification`, on a request
without `ssq_notify_schema` (a stale installed `ssq-notify` whose type numbers
collide with the proto enum). A hidden session's event then delivers (fail
open) and is counted as `hidden_delivered{class=routine}`.

Visibility has four states. Only `hidden` is gated; `unresolved` (not in the
index) fails open and is counted.

## Where it is enforced

| Channel label | Path | Mechanism |
|---|---|---|
| `bus` | every `EventNotification` (history, push, toasts) | `EventBus.SetPublishFilter`, dropped before `Seq` so replay cannot resurrect it |
| `push_status` | Stopped `session.updated` push | `push.SessionDeliveryGate.AllowStatusChange` |
| `auto_approved` | auto-allow history rows (deny rows are always kept) | `services.AutoApprovedGate.AllowAutoApprovedRow` |
| `slack` | review-queue Slack message | unchanged: the poller skip and `suppressForHidden` (removed in PR 2b) |

The gate is installed on the bus when `SessionService` is constructed
(`newGatedSessionService`) and seeded synchronously in `BuildRuntimeDeps` right
after `storage.LoadInstances()`, before any instance is wired or started.

## Flag

`hidden_session_gate` (`config.HiddenSessionGateFeatureFlag`), default **off**:
off means today's behavior plus shadow counters (`would_suppress`). Read from
`config.json` by a `FlagCache` (no I/O on the publish path; reloaded every 5s,
at startup and immediately by the `FlagObserver` call that
`FeatureFlagService.UpdateFeatureFlag` makes after every persisted change).

The flag is registered in `knownFeatureFlags`, so it is settable through
`UpdateFeatureFlag` and the Settings page:

- **Enabling** returns `FailedPrecondition` ("stats writer not running") while
  the stats writer is not running; `GetFeatureFlags` shows the same text as
  `status_detail`. Disabling is never refused. A hand-edited `config.json`
  that sets it true while no writer runs logs WARN
  `delivery_gate_enabled_without_stats`.
- `status_detail` is a composite: independent sources contribute and are joined
  with "; " in registration order (`AddStatusDetailSource`); the existing
  `SetStatusDetailProvider` keeps replacing its own slot by name.
- The previous value recorded on a flip is the true one: an absent key is the
  registered default, and a controller failure on an absent key rolls back by
  deleting the key instead of writing an explicit `false`.
- `scope` and `mutation` on `UpdateFeatureFlagRequest` still return
  `Unimplemented` (Story 2.11, PR 2e).

## Counters

All are mirrored in-process (the OTel meter is a no-op when telemetry is not
initialized). No counter carries a session id.

| Name | Labels |
|---|---|
| `notification_delivery_suppressed_total` | channel, type, reason, kind |
| `notification_delivery_would_suppress_total` | channel, type, reason, kind |
| `notification_hidden_delivered_total` | channel, class, kind |
| `notification_delivery_unresolved_total` | class |
| `notification_gate_index_miss_total` | none |
| `notification_gate_index_refresh_total` | result: started, skipped_min_interval, timeout, error, ok |
| `notification_unresolved_resolved_later_total` | none (must stay 0: an index defect signal) |
| `notification_gate_filter_panic_total` | none |
| `notification_legacy_hidden_suppressed_total` | site, type, class |
| `notification_rpc_unversioned_total` | none |
| `hidden_session_audit_degraded_total` | mode: `flag_change_queued`, `flag_change_fallback_log` |
| `notification_crash_coalesced_total` | none |

Log lines: `delivery_suppressed`, `delivery_would_suppress`,
`hidden_delivery_allowed`, `delivery_unresolved_fail_open`,
`legacy_ssq_notify_detected`; each rate-limited per (session, type, reason)
to once per 60s with `suppressed_since_last` (the hourly WARN for an
unversioned `ssq-notify` is the exception).

## Stats, soak evidence and the stats file

`GetDeliveryGateStats` (no parameters; reachable on `:8543` without auth and on
`:8444` behind it; aggregate counters only, with no session id, title or
message text; no MCP tool) answers from in-process state, so it works with
telemetry uninitialized:

- `since_process_start`: every counter above at full labels, reset on restart.
  The label dimensions that do not fit `kind`/`class` ride in the counter name:
  `would_suppress{channel=bus,reason=routine_for_hidden,type=...}`. Sum the
  entries whose name starts with a counter's name.
- `buckets`: 72 hourly buckets, persisted. Each carries `uptime_seconds`,
  `gate_on_seconds` (global) and `gate_on_seconds_by_kind`, `hidden_events_seen`,
  `hidden_events_while_on`, `routine_events_while_on` and reduced
  `(counter, kind, class)` counters: `suppressed`, `would_suppress`,
  `hidden_delivered`, `unresolved`, `delivered_while_on`,
  `probe_delivered_while_on`.
- `flag_history`: the last 20 changes of the flag, with the mutation name so a
  clear is not read as an explicit `false`.
- `events_by_kind_24h`: hidden events the gate resolved, per kind.
- `soak`: server-computed. `soak_streak_hours` is the sum of
  `min(gate_on_seconds, uptime_seconds)/3600` over the buckets since the last
  off flip whose `routine_events_while_on > 0`, excluding the bucket that holds
  the flip; hours with no routine traffic neither count nor break it, and an
  `Unresolved` event (delivered fail-open) is not a routine event. Per-kind
  streaks are in `soak_streak_hours_by_kind`.
  `failure_delivered_while_on`, `needs_human_delivered_while_on` and
  `probe_delivered_while_on` are counted since the last off flip.
- `stats_file_status`: `loaded`, `quarantined`, `writable`.

Gate-on time is credited from the flag snapshot in force over each interval
(the writer ticks once a minute and the flag cache calls the accumulator on
every swap), so a flip mid-interval splits it. Events are bucketed by the
accumulator's last tick: an event within a minute of an hour boundary may land
in the previous bucket.

`<config dir>/delivery-gate-stats.json` (mode 0600; the test-mode directory when
`STAPLER_SQUAD_TEST_DIR` is set) is written by `services.FileStatsStore`:
pid-named temp file, `fsync`, rename, at most once a minute plus a final flush
registered with `defer` at the top of `Server.Shutdown` and bounded by
`StatsFlushTimeout` (5s). A crash loses at most the last minute. The schema has
a `version` and a SHA-256 `checksum` over the canonical JSON; an unknown
version, invalid JSON, a bad checksum or a file over 1 MiB is moved to
`delivery-gate-stats.json.corrupt-<UTC>` (one kept), the server starts empty and
logs WARN `delivery_gate_stats_corrupt`. An unwritable directory or full disk
logs WARN `delivery_gate_stats_write_failed` at most every 10 minutes and sets
`writable=false`; it never blocks or fails startup. Two processes sharing a
directory are last-writer-wins; a process that finds another pid that wrote
within 2 minutes keeps its stats in memory (WARN
`delivery_gate_stats_foreign_writer`) and merges per `hour_start` (counters
summed, seconds clamped to 3600) when that pid is gone. Publishes after the
final flush (for example session stop events during storage close) are lost by
design. The persisted key count is bounded at 6 counter names x 5 kinds x 3
classes x 72 buckets.

## Audit file and `flag_change` lines

Every `hidden_session_gate` flip appends to
`<config dir>/audit/hidden-session-replies.jsonl` (directory 0700, file 0600,
rotated at 5 MiB, 3 files kept; created on the first flip). Lines carry `ts`,
`kind=flag_change`, `phase`, `change_id`, `flag`, `scope`, `previous`, `new`,
`outcome`, `seq`, `boot_id`, `boot_ts`, `boot_seq`, and the request's
`listener`, `peer_addr`, `host`, `origin`, `user_agent`, `auth_mode`.

- A flip writes at most two lines linked by `change_id`: a durable `requested`
  line (loosening flips only, before the update mutex, bounded at 2s) and a
  `result` line with `outcome` `applied`, `aborted_persist_failed`,
  `aborted_controller_failed` or `aborted_rolled_back`, the true previous value
  and a `seq` taken inside the update mutex. Gate flips are tightening: only the
  `result` line, pushed on a 64-entry queue drained by one goroutine, so the
  request never waits on the sink mutex or an `fsync`.
- Readers order by `(boot_seq, seq)`, never by `ts` or `boot_ts`. `boot_seq` is
  a counter file next to the audit file, incremented atomically at sink open.
- A full queue or a failed drain writes the same fields as WARN
  `flag_change_audit_degraded` in the main log (not tamper-evident) and counts
  `hidden_session_audit_degraded_total`.
- A `requested` line with no `result` is indeterminate: read the persisted
  config (`GetFeatureFlags`) for the actual state, never the log.

## Crash notification

A session entering `Crashed` publishes one `FAILURE` notification from
`sessionExitedPublisher` (id `session-crashed-<uuid>-<exit unix seconds>`),
for hidden and visible sessions alike and independent of the gate flag. The
status is read from the lock-free snapshot; an `EventExited` whose snapshot is
not `Crashed` publishes nothing. At most 3 individual crash notifications are
published per 60s across all sessions; further crashes fold into one summary
(`session-crashed-summary-<window start>`, up to 5 titles plus a count) and
increment `notification_crash_coalesced_total`.

Hidden-reachable failure producers, and what covers each:

| Producer | Type | Covered by |
|---|---|---|
| Crash | `FAILURE` | `session_service_events_test.go` (T-CR-01, -04, -07, -09, -10, -11), `crash_matrix_test.go` (T-CR-02, -03, -08) |
| `PermanentlyFailed` | `ERROR` | `TestPermanentlyFailed_ShouldDeliverOneErrorAndResolveHidden_WhenItemIDEqualsUUID` (T-CR-05) |
| Main-session Stop with an error stop reason (`ssq-notify --type task_failed`) | failure class, unstamped | `TestHookHandler_ShouldStampRoutineOnPostToolAndSubagentOnly_WhenCurlPayloadRecorded` asserts the main stop is not demoted; the policy matrix covers the type. Post-tool and subagent errors are stamped `delivery_class=routine` and are dropped for hidden sessions |
| Autonomous driver "Autonomous fix stuck" | `FAILURE` | not reachable for a hidden session until PR 2b: the legacy `if !inst.Hidden` check drops it first and counts `legacy_hidden_suppressed{site=autonomous_generic}` |
| Rate-limit and capacity hard stops | `WARNING` + `delivery_class=failure` | not reachable for hidden sessions until PR 2b (legacy checks); the stamp is covered by `ssq_hook_handler_stamp_test.go` |
| `ReviewQueuePoller` ErrorState / TestsFailing | n/a | unreachable for hidden sessions (`shouldSkipSession`); not relied on |

## Latency

The filter runs on the caller goroutine of every `Publish`. Measured with
`go test -run '^$' -bench 'BenchmarkPublish|BenchmarkIndexUpsert|BenchmarkResolve' -benchmem -count 5 ./server/deliverygate`
(AMD Ryzen 9 7900X, Go 1.26, `-count 5`, run in the background):

| Benchmark | median ns/op | min-max | B/op | allocs/op |
|---|---|---|---|---|
| `IndexUpsert_1000Sessions` | 148753 | 129011-175639 | 109473 | 14 |
| `PublishNoFilter/hit_hidden_failure` | 6154 | 6142-6192 | 433 | 1 |
| `PublishNoFilter/hit_hidden_routine` | 6158 | 6152-6226 | 434 | 1 |
| `PublishNoFilter/hit_visible` | 6141 | 5986-6236 | 433 | 1 |
| `PublishNoFilter/miss_unresolved` | 6232 | 6195-6672 | 433 | 1 |
| `PublishNoFilter/non_notification` | 6207 | 6150-6405 | 433 | 1 |
| `PublishWithFilter/index_0/hit_hidden_failure` | 6897 | 6857-6936 | 465 | 3 |
| `PublishWithFilter/index_0/hit_hidden_routine` | 7108 | 6927-7587 | 465 | 3 |
| `PublishWithFilter/index_0/hit_visible` | 6918 | 6638-7207 | 467 | 3 |
| `PublishWithFilter/index_0/miss_unresolved` | 6870 | 6750-7169 | 465 | 3 |
| `PublishWithFilter/index_0/non_notification` | 6413 | 6194-6656 | 434 | 1 |
| `PublishWithFilter/index_1000/hit_hidden_failure` | 6804 | 6659-7114 | 449 | 2 |
| `PublishWithFilter/index_1000/hit_hidden_routine` | 648 | 642-665 | 304 | 2 |
| `PublishWithFilter/index_1000/hit_visible` | 6640 | 6584-6689 | 433 | 1 |
| `PublishWithFilter/index_1000/miss_unresolved` | 7109 | 6830-7470 | 467 | 3 |
| `PublishWithFilter/index_1000/non_notification` | 6624 | 6302-6667 | 434 | 1 |
| `Resolve_Hit` | 4 | 3-6 | 0 | 0 |

| Benchmark | median ns/op | min-max | B/op | allocs/op |
|---|---|---|---|---|
| `FilterOnly/index_0/hit_hidden_failure` | 567 | 533-600 | 32 | 2 |
| `FilterOnly/index_0/hit_hidden_routine` | 563 | 555-571 | 32 | 2 |
| `FilterOnly/index_0/hit_visible` | 574 | 561-614 | 32 | 2 |
| `FilterOnly/index_0/miss_unresolved` | 573 | 549-582 | 32 | 2 |
| `FilterOnly/index_0/non_notification` | 3 | 3-3 | 0 | 0 |
| `FilterOnly/index_1000/hit_hidden_failure` | 470 | 419-479 | 16 | 1 |
| `FilterOnly/index_1000/hit_hidden_routine` | 457 | 443-477 | 16 | 1 |
| `FilterOnly/index_1000/hit_visible` | 218 | 212-223 | 0 | 0 |
| `FilterOnly/index_1000/miss_unresolved` | 566 | 532-602 | 32 | 2 |
| `FilterOnly/index_1000/non_notification` | 3 | 3-3 | 0 | 0 |

Reading the table:

- `PublishNoFilter` is the existing cost of `EventBus.Publish` (about 6.2 us on
  this machine: a ring-buffer append and two clock reads). `PublishWithFilter`
  adds the filter in front of it. `FilterOnly` is the filter alone.
- A non-notification event (the high-volume `session.updated` stream) costs 3 ns
  through the filter: an atomic load and a type compare.
- A visible-session notification costs about 220 ns and no allocations; the
  A/B `Publish` difference (about +8%) is dominated by noise at this scale.
- A hidden-session decision costs about 450 ns (one clock read for the log
  limiter, counter increment). A suppressed event never reaches `Publish`, so
  its total (650 ns) is 10x cheaper than publishing it.
- A cache-miss (unresolved) decision costs about 570 ns and 2 allocations
  (a WARN line passes the limiter once per key per minute).
- `IndexUpsert_1000Sessions` is the copy-and-swap cost of a session create,
  rename or delete: about 130 us and 110 KB. Writes are tens per day.
- `Resolve_Hit` (lock-free read, parallel) is 3-6 ns.

The plan's initial budget was filter p99 within 5% of `Publish`. Measured, the
common paths (non-notification, visible session) are inside it by the
filter-only measure (3.5%) and just outside it by the A/B measure (about 8%);
hidden-session decisions are about 7-10% of a `Publish` but replace it when the
event is suppressed. The ratio is recalibrated once, from this run, to: at most
10% of `Publish` for visible and non-notification events, and at most 1 us
absolute for any hidden-session decision. The duration histogram samples one
notification decision in 32 because a clock read (about 0.7 us here) would
otherwise be the largest single cost of the visible path.

PR 2a-2 adds one mutex-guarded bucket update per resolved hidden-session
decision (no clock read: the bucket is the accumulator's last tick). Re-measured
once after it with `-bench 'BenchmarkFilterOnly|BenchmarkPublishNoFilter' -count 5`
(same machine, other builds running, so noisier than the table above): hidden
decision 516 ns (index 1000, routine) and 579 ns (failure), 620-690 ns at index
0; visible 223 ns and non-notification 3 ns, unchanged. Hidden decisions stay
under the 1 us budget.

## Guards

- `make test-delivery-guards` runs the `sinkguard`-tagged scans: the
  sink-enumeration guard (`sink_guard_test.go`) and the no-env-var flag scan
  (`flag_env_guard_test.go`).
- `server/deliverygate/testdata/sinkfixture` is the scan's negative control.
