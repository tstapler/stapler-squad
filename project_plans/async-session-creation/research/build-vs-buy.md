# Build vs. Buy: async-session-creation

## Summary Verdict

**Build, by extending existing in-repo patterns.** No external job-queue or
FSM library is warranted. The repo already has (a) a compile-time-checked
state machine (`session/instance_state.go`'s `TransitionDef`/`transitionIndex`
table), (b) a tracked-goroutine async pattern for exactly this kind of
work (`server/services/session_service.go:2397-2413`'s `trackCleanup`), (c) a
periodic sweeper pattern for staleness (`StaleSessionNotifier`), and (d) a
telemetry package (`telemetry.StartSpan`, `telemetry.GetMeter()`) already used
by other services. The engineering task is wiring these together for the
`CreateSession` path, not sourcing new infrastructure.

## 1. Existing OSS libraries for async job orchestration / state machines

### `github.com/looplab/fsm` (state machine)

- **Maturity/license**: MIT, ~2.7k GitHub stars, stable API, low churn (a
  finished-feature library, not actively growing) — safe to depend on.
- **Fit**: Not needed. This repo already has a stricter, purpose-built
  equivalent: `session/instance_state.go`'s `transitionIndex` (a
  `map[transitionKey]TransitionDef` keyed by `(from, to)` Status pairs) with
  `Guard`/`After` hooks per transition, `ErrInvalidTransition{From, To}` for
  invalid transitions, and full concurrency-tested coverage (see
  `session/instance_concurrency_test.go`'s `TestTransitionTo_ConcurrentPause/
  ConcurrentApprove/ConcurrentMixed`). looplab/fsm's event-driven API
  (`fsm.NewFSM(initial, events, callbacks)`, string-keyed events) is a worse
  fit than the table this codebase already has: it would require translating
  every `Status` (an `int`-backed enum with proto mapping via
  `adapters.InstanceToProto`) into fsm's string-event vocabulary, duplicating
  the guard/callback logic that already lives in `TransitionDef`, and losing
  the existing lock-discipline documentation (`transitionTo` vs.
  `transitionToLocked`, actor-mailbox interaction — see the extensive doc
  comments at `session/instance_state.go:39-99`).
- **What adopting it would require**: a full rip-and-replace of
  `instance_state.go`'s transition table, re-verification of every existing
  transition's guard/after semantics, and bridging the actor-mailbox pattern
  (`transitionToLocked`, `sendSyncErr`/`send`/`sendCtx`) that fsm has no
  concept of. This is strictly more work than adding two new states
  (`Creating`→resolving-phase transitions, `Failed`) to the existing table.
- **Verdict: Not recommended.**

### `github.com/riverqueue/river` (or similar durable job queue)

- **Maturity/license**: MIT, actively maintained, Postgres-backed durable job
  queue with retries, scheduling, unique jobs.
- **Fit**: Wrong shape for this problem. River (and job queues generally)
  solve *durable, restart-surviving, potentially-distributed* work — the
  requirements doc explicitly scopes this out ("Non-Goals... Multi-
  instance/distributed coordination... single-process, single-instance is
  the only deployment model today"; Non-functional Requirements: "not
  applicable — single-user-per-instance, low session creation volume"). This
  is a single Go process managing at most a handful of concurrent session
  creations. Standing up a job-queue schema (River requires its own Postgres
  tables, migrations, a worker pool, a maintenance/janitor goroutine for
  its own stale-job detection) to orchestrate what's fundamentally "run this
  goroutine, update a status field on failure" is significant new
  infrastructure for a problem the repo already solves elsewhere with plain
  goroutines. It would also conflict with the existing `session/ent`
  storage layer (sqlite via ent, not Postgres) — River requires Postgres.
- **Verdict: Not recommended — overkill for a single-process, single-user
  local dev tool**, exactly as the task description anticipated.

### `github.com/robfig/cron/v3` (scheduled staleness checks)

- **Already a direct dependency** (`go.mod:40`, `v3.0.1`). Worth checking
  whether it should back the stale-creation detector instead of a bespoke
  ticker.
- **Fit**: Poor fit for *this* sweeper specifically. cron is built for
  calendar-style/interval scheduling of independent jobs (cron expressions,
  "run at 3am", "run every 5 minutes"); the existing in-repo staleness
  sweepers (`StaleSessionNotifier.Start`, referenced as mirroring
  "SessionRetentionSweeper.Start's shape") are plain `time.NewTicker`-driven
  loops selecting on `ctx.Done()` for cooperative shutdown — a pattern the
  repo already has three independent instances of (the review queue's
  5-minute check, the rework-block gate's 15-minute check, and the 2-hour
  stuck-backlog-item detector — see `stale_session_notifier.go`'s own doc
  comment enumerating them). Introducing cron here would mean maintaining
  *two* different scheduling idioms in the same package for what is
  structurally the same kind of check (poll in-memory state on an interval,
  fire a side effect on threshold crossing).
- **Verdict: Not recommended for this checker** — extend `StaleSessionNotifier`'s
  existing ticker pattern (or add a sibling ticker-based checker) instead,
  per point 4 below. (Not a comment on cron's other uses in the codebase,
  which are presumably calendar-shaped and fine as-is.)

### `golang.org/x/sync` (errgroup, singleflight)

- **Already a direct dependency** (`go.mod:208`, `v0.22.0`).
- **Fit**: `errgroup.Group` is designed for a *bounded* set of goroutines
  that all need to be waited on and whose first error should cancel the
  rest — e.g. fan-out/fan-in. `CreateSession`'s background resolution is a
  single long-running goroutine per session creation, unbounded in count
  (one per creation, potentially many concurrent creations), with no
  "wait for all and return the first error" semantic — its result is
  reported by mutating the `Instance`'s status/`creation_progress` fields
  and publishing events, not returned to a caller that's blocked on it.
  `errgroup` doesn't fit that shape any better than a bare goroutine does.
  The repo's existing `trackCleanup` helper (`sync.WaitGroup` +
  `sync.Mutex`-guarded "closed" flag to prevent `Add`-after-`Wait` races) is
  the correct primitive here, and it's *already deployed for this exact
  purpose* — see point 4.
- **Verdict: Not recommended as a wrapper for the whole flow** — but the
  package is already in use elsewhere and needs no new dependency either
  way.

## 2. SaaS/managed API

Not applicable. `stapler-squad` is a local-only, single-user developer tool
(`localhost:8543`); there is no multi-tenant or hosted-infrastructure
component to this feature, and the requirements doc's Security
classification ("internal (local dev tool)") and Scalability note ("not a
throughput concern") rule out any managed orchestration service (e.g. a
hosted workflow engine like Temporal Cloud or AWS Step Functions) — those
solve durability/visibility problems across distributed workers, which
doesn't exist here (one process, one user, no network fan-out beyond the
GitHub clone itself).

## 3. LLM-generated implementation vs. battle-tested library

### Stale-creation detection (ticker/timeout logic)

- **Risk if bespoke from scratch**: low-to-moderate. Ticker-based polling
  loops are a well-worn Go idiom, and the specific correctness hazards
  (double-notification, races between recovery and notification, thread-
  safety of the "already notified" set) are **already solved and tested** in
  this exact codebase: `StaleSessionNotifier` (`server/services/
  stale_session_notifier.go`) implements precisely this shape — an
  edge-triggered, self-clearing, mutex-guarded `notifiedSessions` map keyed
  by stable ID, re-armed on recovery, with explicit tests for the tricky
  cases (`*_should_FireOnceOnceEnabled_When_NotifyWasDisabledDuringInitial
  StaleCrossing`, `*_should_ReNotify_When_SessionPausesThenResumesStillStale`).
  Writing new bespoke ticker code from scratch (ignoring this file) would be
  the actual risk — re-deriving edge cases this file's test suite already
  found. Extending/mirroring `StaleSessionNotifier` (or its sibling
  `SessionRetentionSweeper`, referenced in its doc comment) is safer than
  either "write it fresh" or "import a library," because there is no library
  that encodes this repo's specific state model (`Instance.Status`,
  `GetTimeSinceLastMeaningfulOutput`, event-bus notification).
- **Verdict: Build, but by extending `StaleSessionNotifier`'s pattern**, not
  writing new ticker/timeout logic from a blank file.

### Background-goroutine lifecycle management

- **Risk if bespoke from scratch**: this is the one place a naive
  LLM-generated implementation (bare `go func() {...}()`) would be a real
  correctness risk — goroutine leaks across repeated failed creations, no
  way for `Shutdown` to await in-flight work, and (per this repo's own
  documented flake) a background goroutine outliving a test and touching a
  torn-down tempdir. **This exact failure mode already happened and was
  fixed** in this codebase: the doc comment at `session_service.go:2390-2396`
  explains that the async-start goroutine is deliberately routed through
  `trackCleanup` (not a bare `go func()`) specifically because a bare
  goroutine caused `TestCreateSession_should_ComposeProfileCLIFlagsBefore
  PresetExtraArgs_When_BothPresent` to flake via cross-test tempdir/database
  access after teardown.
  `trackCleanup` (`session_service.go:320-336`) already provides: a
  `sync.WaitGroup`-backed count so `Shutdown` can drain in-flight work,
  a `sync.Mutex`-guarded `deleteCleanupClosed` flag to avoid the documented
  `Add`-after-`Wait` misuse panic, and a fallback to an untracked `go fn()`
  once shutdown has begun draining (so late-arriving cleanup doesn't block
  shutdown forever).
  `errgroup`/generic context patterns add nothing here beyond what
  `trackCleanup` already does for this specific "outlives the RPC, needs
  to be reachable by graceful shutdown" shape — introducing a second
  goroutine-lifecycle mechanism for the same purpose the codebase already
  standardized on would itself be a consistency regression.
- **Verdict: Build by extending `trackCleanup`** for the resolution
  goroutine (see point 4) rather than introducing errgroup or writing new
  lifecycle-tracking primitives.

## 4. Fork or adapt an existing in-repo pattern

**Yes — this is the recommended path, and the codebase already contains
essentially a prototype of the target architecture:**

- **`server/services/session_service.go:2397-2439`** — the async
  worktree/tmux-startup goroutine dispatched via `trackCleanup` **already
  does create-then-resolve-async** for the *startup* phase specifically: it
  publishes `SessionCreatedEvent` before the goroutine runs (line 2380,
  ahead of the `trackCleanup` call at 2397), calls `instance.SetCreationProgress(...)`
  plus `events.NewSessionUpdatedEvent(instance, []string{"creation_progress"})`
  to stream progress, and on failure sets a progress message and force-
  transitions the instance to `Stopped` (there is no dedicated `Failed`
  status today — see below) before saving and publishing the status change.
  This is the direct precedent this project is asked to generalize
  **backward** to also cover GitHub-URL resolution, alias/default
  resolution, and branch/session-type inference — work that currently still
  happens synchronously *before* this goroutine even starts. The plan
  should extend this same `trackCleanup`-dispatched goroutine (or a new one
  immediately preceding it) to wrap that earlier resolution work too, using
  the same `SetCreationProgress`/event-publish idiom already established
  here, rather than inventing a second orchestration mechanism.
- **`server/services/stale_session_notifier.go`** — as covered in point 3,
  this is the ticker-based sweeper pattern to extend (or add a sibling to)
  for stale-*creation* detection specifically. Its doc comment already
  enumerates three other independently-tuned staleness detectors in the
  codebase (review-queue 5-minute, rework-block 15-minute, stuck-backlog
  2-hour) as deliberately-separate instances of the same idiom — a fourth
  (stale-*creation*, likely a much shorter threshold given the target SLO)
  fits that established pattern rather than requiring new infrastructure.
- **`session/instance_state.go`'s `transitionIndex`** — the state machine to
  extend with new `Status` values/transitions. Today there is **no
  dedicated `Failed` status** — the async-start failure path
  (`session_service.go:2409`) and `session/health.go`'s async-crash path
  (line 62) both fall back to `ForceStatus(Stopped)` with a
  `SetCreationProgress("Session failed: ...")` message layered on top as a
  poor-man's error signal, and `ForceStatus` (used at both call sites)
  appears to bypass `transitionTo`'s guard/After table entirely — this is
  exactly the "distinguishable Failed status" gap the requirements doc's
  Open Questions flags (`SESSION_STATUS_FAILED` vs. reusing an existing
  status) and Phase 3 architecture should resolve by adding a proper
  `Failed` status + transitions to this table, not by continuing to overload
  `Stopped`+a progress string.
- **`telemetry.StartSpan`/`telemetry.GetMeter()`** (`telemetry/telemetry.go`,
  already used by `server/services/search_service.go` and `connectrpc_websocket.go`,
  and by `session/unfinished/metrics.go` for a metrics-registration
  pattern via `Int64ObservableGauge`/`RegisterCallback`) — the tracing span
  around the background resolution goroutine and the creation-outcome
  metric should follow these exact two existing idioms rather than
  initializing a new OTel provider or metric-registration path.

## Recommendation Table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| `looplab/fsm` | Mature, MIT, handles guards/callbacks | Duplicates `instance_state.go`'s existing, more tightly-integrated transition table; migration cost > benefit | Not recommended |
| `riverqueue/river` (or similar durable queue) | Durable, retries, mature | Wrong scale (distributed/durable job queue for a single-process tool); requires Postgres vs. this repo's sqlite/ent; new schema/migrations/worker pool for no real benefit | Not recommended |
| `robfig/cron/v3` for stale-check scheduling | Already a dependency; robust | Wrong shape for a state-polling sweeper (repo already has 3 ticker-based sweepers, none use cron for this) | Not recommended |
| `golang.org/x/sync/errgroup` as the goroutine wrapper | Already a dependency; well-tested | Designed for bounded fan-out/wait-and-cancel, not "long-lived tracked goroutine reachable by graceful shutdown" — `trackCleanup` already solves the latter | Not recommended (but no downside to it remaining used elsewhere in the codebase) |
| SaaS/managed workflow engine | N/A | Not applicable — local-only single-user tool | Not applicable |
| Extend `trackCleanup` + `session_service.go:2397-2439`'s async-start pattern for the new resolution goroutine | Proven in this codebase, already fixed a real goroutine-leak/test-flake bug, matches existing `SetCreationProgress`/event idiom | Requires restructuring `CreateSession`'s data flow (the "largest single risk" the requirements doc names) | **Recommended** |
| Extend `StaleSessionNotifier`'s ticker pattern for stale-creation detection | Proven, tested edge cases (re-arming, notify-disabled-then-enabled), consistent with 3 existing sweepers | Threshold needs empirical tuning (per requirements doc's Rabbit Holes) | **Recommended** |
| Add `Failed` status + transitions to `session/instance_state.go`'s `transitionIndex` | Closes the current `Stopped`-as-failure overload; guard/After hooks give a real invariant point instead of `ForceStatus` bypassing the table | Touches a shared, heavily-tested state table; needs re-verification of the 7 session-creation touchpoints per the registry doc | **Recommended** |
| Use `telemetry.StartSpan`/`telemetry.GetMeter()` for the new span/metric | Already the repo's OTel idiom, used in 3+ places | None identified | **Recommended** |
