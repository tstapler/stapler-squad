# ADR-024: Actor-per-Instance + atomic.Pointer[InstanceSnapshot], Not an Encapsulated Mutex

**Status**: Accepted
**Date**: 2026-06-30
**Deciders**: Tyler Stapler
**Relates to**: `project_plans/instance-actor-concurrency/requirements.md` (Item 2)

---

## Context

`session.Instance` is currently protected by `stateMutex` (a `github.com/linkdata/deadlock`-wrapped
`RWMutex`), but the protection is mostly false confidence, not real synchronization. Two
Explore-agent audits found: `adapters.InstanceToProto` and `Instance.ToInstanceData()` (every
`ListSessions`/`WatchSessions`/persist) take **no lock at all**; `server/services/session_service.go`
has ~123 direct unguarded field accesses on `*Instance` (~28 writes) across RPC handlers,
`AutonomousDriver`'s background goroutine, and an async `CreateSession` goroutine; `GitHubPRURL`/
`GitHubPRNumber` are locked on one writer and unlocked on another; `ReviewState` fields are written
unlocked from `review_queue_poller.go` while sibling fields in the same struct are locked elsewhere;
`CapacityMonitor`, `ReviewQueuePoller`, and the websocket streaming handler read with zero locking.
6+ goroutine classes touch shared state with no reliable discipline.

A live `go tool pprof` mutex-profile capture, taken while debugging a user-reported "UI not
loading" hang, attributed ~169s of cumulative reader wait time to `Instance.StopController()`
(`session/instance_controller.go:137`) holding `stateMutex.Lock()` for the duration of tmux
teardown while `ListSessions`/`WatchSessions`/status-polling readers queued behind it.

## Decision

Adopt one actor goroutine per `Instance` (all mutation expressed as commands delivered to a
buffered channel mailbox, processed single-threaded) publishing a fresh
`atomic.Pointer[InstanceSnapshot]` after every command; all reads (`InstanceToProto`,
`ToInstanceData`, `CapacityMonitor`, `ReviewQueuePoller`, the websocket handler, `PRStatusPoller`)
load the snapshot pointer with no blocking and no lock. `stateMutex` is removed entirely once
migration completes (R2.6).

### Rejected alternative: encapsulate, don't replace, the mutex

The alternative seriously considered — rung 4 of the `go-concurrency` skill's ladder — was to
**keep** `stateMutex` (a `sync.RWMutex`), unexport every `Instance` field, and force all ~30+
external call sites through locked accessor methods. This *would* fix the false-confidence
problem: encapsulation, not the specific primitive, is what prevents the bypassing seen today,
and a disciplined "every field private, every access through a locked method" rule would close
the same 75-site catalog of gaps.

It does **not** fix the problem that started this investigation. The pprof mutex profile showed
real, measurable contention from `RWMutex.RLock()` itself — every read-lock acquisition does an
atomic increment/decrement on shared internal reader-count state, so readers across cores
contend with each other even though they are not logically blocking one another. `ListSessions`/
`WatchSessions` read every `Instance` on every call; under load this reader-side cache-line
contention is exactly what manifested as the reported UI hang. `atomic.Pointer[InstanceSnapshot].
Load()` has no equivalent cost: it is a single atomic read with no shared mutable counter for
concurrent readers to contend on.

So the central tradeoff is: a correctly-encapsulated mutex fixes encapsulation only; the actor +
atomic-snapshot model fixes both encapsulation **and** reader-side contention. These are two
independent reasons, not one, and the encapsulated-mutex alternative satisfies only the first.

## Consequences

### Positive
- Every read path becomes a single non-blocking atomic load — no lock, no cache-line contention
  among readers, directly resolving the measured `StopController` pileup.
- Mutation is structurally centralized in one goroutine per `Instance`; there is no call site
  left that can "forget" to lock, because there is no lock to forget — the compiler/closure
  structure is the enforcement mechanism instead of a 75-site manual audit.
- Compound operations (`transitionTo`, `UpdatePRStatus`'s 8-field write, checkpoint creation,
  `SwitchWorkspace`) become one command processed atomically by the actor — no partial-update
  visibility (R2.5), which an encapsulated mutex would also provide but only if every call site
  remembered to hold the lock across the whole compound operation, which is the exact discipline
  that has already failed in this codebase.

### Negative
- A new failure mode replaces the old one: a command handler that calls another mailbox-routed
  method on its own `Instance` and blocks on the reply self-deadlocks, the same shape as
  `stateMutex`'s reentrant-lock bug (the confirmed `SwitchWorkspace` → `Start()` deadlock,
  Item 1), but `linkdata/deadlock` actively detects the mutex version — nothing watches a stuck
  channel send the same way. Mitigation: internal `xxxLocked(s *instanceState, ...)` twins,
  enforced by an `ast-grep` lint rule (see `research/architecture.md` §4).
- Requires a multi-PR, sequenced migration (`research/architecture.md` §7) rather than a
  mechanical search-and-replace, because all writers of a given `Instance` must convert in one
  atomic cutover (`research/pitfalls.md` §4) — a half-converted write path is strictly worse than
  today's status quo (deterministic clobbering instead of a probabilistic race).
