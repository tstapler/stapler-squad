# Notification delivery gate (hidden sessions)

Reference for `server/deliverygate`: the single place that decides whether a
notification about a hidden session (review, diagnose, triage, headless) is
delivered. Policy and rollout rationale live in
`project_plans/notification-tray-and-hidden-session-gate/` (ADR-001 to ADR-004).

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
`config.json` by a `FlagCache` (no I/O on the publish path; reloaded every 5s
and at startup). The flag is not yet registered in the feature-flag service:
until that PR lands it can only be set by editing `config.json`.

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

Log lines: `delivery_suppressed`, `delivery_would_suppress`,
`hidden_delivery_allowed`, `delivery_unresolved_fail_open`,
`legacy_ssq_notify_detected`; each rate-limited per (session, type, reason)
to once per 60s with `suppressed_since_last` (the hourly WARN for an
unversioned `ssq-notify` is the exception).

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

## Guards

- `make test-delivery-guards` runs the `sinkguard`-tagged scans: the
  sink-enumeration guard (`sink_guard_test.go`) and the no-env-var flag scan
  (`flag_env_guard_test.go`).
- `server/deliverygate/testdata/sinkfixture` is the scan's negative control.
