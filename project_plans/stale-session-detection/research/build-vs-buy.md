# Research: Build vs. Buy — stale-session-detection

**Agent**: 6 (Build vs. Buy)
**Date**: 2026-08-06

## Summary

This feature is internal business logic wiring (config surface, UI badge, grouping
strategy, notification event, approval-rule condition) built entirely on data the
codebase already tracks (`ReviewState.LastMeaningfulOutput`/`LastTerminalUpdate`,
`session/review_state.go:50-55`). There is no meaningful "buy" surface. All four
angles converge on: use stdlib `time`, extend the two existing threshold
comparisons, do not add a dependency.

## 1. Existing OSS library/framework (staleness/heartbeat/TTL detection)

**Candidates considered**: generic TTL-cache libraries (`patrickmn/go-cache`,
`jellydator/ttlcache`), heartbeat/liveness-check libraries, generic
"staleness detector" packages.

**Evidence**: `go.mod` has no TTL-cache or heartbeat-monitoring dependency today
(checked `grep -n "cache\|ttl\|heartbeat" go.mod` — only an indirect,
unrelated `golang/groupcache` transitively pulled in). The existing staleness
checks in the codebase (`session/review_queue_determiner.go:262`,
`session/backlog_lifecycle.go:1336,2627,2846`) are all plain
`time.Since(x) > threshold` comparisons against fields already being maintained
by other code (`ReviewState`, backlog item timestamps) — there is no cache
eviction, no expiry callback, no background sweep-and-expire semantics that a
TTL cache would provide. A TTL cache exists to answer "is this entry still
valid, and should I evict/refetch it" — this feature only needs "is this
timestamp too old to display without flagging," a read-only comparison with no
storage or eviction concern of its own.

- **Pros**: A TTL-cache library would centralize expiry logic if this codebase
  had many independent freshness checks needing consistent expiry+eviction
  semantics.
- **Cons**: Wrong abstraction — no eviction/storage needed, just a comparison;
  adds a dependency and an indirection layer for zero behavioral gain;
  `ReviewState` fields are already the source of truth and already
  mutation-serialized through the actor pattern (`session/review_state.go:19-25`),
  which a generic cache library knows nothing about and could not respect.
- **Verdict**: **Not recommended.**

## 2. SaaS/managed API (uptime/heartbeat monitoring services)

**Candidates considered**: Pingdom, Better Uptime, Healthchecks.io-style
dead-man's-switch services.

- **Pros**: None applicable — these services monitor whether a *remote process
  reports in periodically*, over the network, typically for infra/cron-job
  liveness. This feature detects staleness of an in-process session's terminal
  output, entirely within a single-user, self-hosted Go binary
  (`CLAUDE.md`: "Single-developer, self-hosted instance — no multi-tenant/auth
  considerations").
- **Cons**: Would require pushing a heartbeat per session to an external
  service over the network, adding latency, an external dependency, a new
  credential to manage, and no capability the codebase doesn't already have
  (the timestamps are already local and already wired to the notification bus
  per `server/services/notification_service.go` and
  `server/services/backlog_service_triage.go:882-967`). No angle where an
  external service replaces or improves on a local time comparison for a
  single, local process.
- **Verdict**: **Not recommended** — confirmed inapplicable, not a close call.

## 3. LLM-generated bespoke implementation vs. battle-tested library

The core logic is `now.Sub(lastOutput) > threshold`, i.e. two `time.Time`
comparisons already used verbatim elsewhere in this codebase (see evidence in
§1: `review_queue_determiner.go:262`, `backlog_lifecycle.go:1336,2627,2846`).
Go's stdlib `time` package is the correct and only tool:

- **Pros of stdlib-only**: zero new dependency, zero new API surface, matches
  existing patterns exactly (four other call sites already do this same
  comparison with plain stdlib), trivially testable with `time.Time` fixtures
  or a fake clock, no risk of a library's expiry semantics (grace periods,
  jitter, hysteresis defaults) silently disagreeing with the codebase's own.
- **Cons**: none identified for the comparison itself.
- **Possible exception — debounce/hysteresis**: if Phase 3 planning decides the
  card badge or notification needs hysteresis (e.g., don't flip
  stale→fresh→stale on every single line of output, or debounce the
  notification so it doesn't refire every poll cycle once past threshold),
  that's a few extra lines of state (a "last notified at" timestamp, mirroring
  the existing `LastAddedToQueue`-style debounce field already in
  `ReviewState`) — still stdlib, not a library concern. No debounce/hysteresis
  library is justified at this scale; the existing `notifyIfActiveWorkSessionStale`
  (`server/services/backlog_service_triage.go:882-967`) is the reference
  pattern for how this codebase already handles "don't notify repeatedly for
  the same stale condition" without any external dependency.
- **Verdict**: **stdlib only — Recommended.** No external dependency is
  justified anywhere in this feature.

## 4. Fork or adapt existing in-repo implementation

Two existing implementations are directly relevant and should be extended, not
duplicated:

1. **Review-queue "Stale" badge** — `session/review_queue_poller.go:38,49`
   (`DefaultReviewQueuePollerConfig().StalenessThreshold`, currently
   hardcoded 5 min) feeding `ReasonStale` in
   `session/review_queue_determiner.go:262-272`.
2. **Backlog stuck-work detector** — `session/backlog_lifecycle.go:2098`
   (`maxWorkSessionStaleness = 2 * time.Hour`, hardcoded) feeding
   `StuckReasonStaleWork` and the backlog-stuck UI panel.

Both already do the same `time.Since(lastX) > threshold` shape against
`ReviewState`/backlog timestamps; both are hardcoded Go constants with no
config surface. The requirements doc (`requirements.md:25-33`) explicitly
flags these as pre-existing, uncoordinated thresholds and points at
`project_plans/review-gate-stale-session-rework/` as the place that already
root-caused the conflict and recommends decoupling/recalibrating rather than
unifying into one value.

- **Pros of adapting**: A shared helper — e.g. a small
  `func IsStale(last time.Time, threshold time.Duration) bool` (or a
  `StalenessChecker` reading from `config/`) — would let all 3-4 planned
  consumers (card badge, grouping/filter strategy, notification, approval-rule
  condition) share one comparison implementation and one config-read path,
  instead of writing `time.Since(x) > y` four separate times with drift risk
  (e.g. one site using `>` and another `>=`, or reading the wrong timestamp
  field). This matches the requirements doc's explicit constraint: "No
  duplicate threshold/detector is introduced where an existing one... already
  covers the same signal — new work extends/config-drives existing mechanisms
  rather than forking them" (`requirements.md:72-75`).
- **Cons**: The two existing thresholds are intentionally *different values for
  different semantics* (5 min informational badge vs. 2 hr hard stuck-detector)
  per the ADR referenced in `review_queue_poller.go:49`
  (`project_plans/review-gate-stale-session-rework/decisions/ADR-001-staleness-threshold-recalibration.md`)
  — a shared comparison helper must take the threshold as a parameter (already
  true of both existing call sites) rather than trying to unify the *value*,
  which the requirements doc explicitly says is out of scope
  (`requirements.md:122-134`, "Out of Scope").
- **Verdict**: **Recommended** — extract the comparison (not the threshold
  value) into one small shared helper function in the `session` package,
  parameterized by threshold, and have the new card-badge/grouping/notification/
  approval-rule consumers call it alongside the two existing sites, each
  passing its own config-driven threshold. This is a Phase 3 (planning) design
  detail, not a new research finding — flagging it here so planning doesn't
  re-derive it independently.

## Cross-cutting conclusion

| Angle | Verdict |
|---|---|
| 1. OSS TTL/heartbeat library | Not recommended |
| 2. SaaS uptime/heartbeat service | Not recommended (confirmed inapplicable) |
| 3. Bespoke stdlib vs. library | stdlib only — Recommended |
| 4. Fork/adapt existing in-repo code | Recommended — extract shared comparison helper, keep threshold *values* separate per `review-gate-stale-session-rework`'s ADR-001 |

No external dependency (library or SaaS) is justified for this feature at any
layer. The only design decision for Phase 3 is how much of the *comparison
logic* (not the threshold values) to factor into one shared, config-parameterized
helper versus leaving the two existing call sites untouched and only adding new
stdlib comparisons at the new call sites.
