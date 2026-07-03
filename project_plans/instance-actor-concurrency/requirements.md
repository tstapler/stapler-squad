# Instance Concurrency Model — Requirements

## Project Overview

Replace the ad hoc `sync.RWMutex` (`stateMutex`) protecting `session.Instance` with two coordinated patterns: an actor model (single owning goroutine per `Instance`, mutations delivered via a buffered channel "mailbox") for writes, and `atomic.Pointer[InstanceSnapshot]` copy-on-write for reads. The work is decomposed into two items: an immediate, isolated deadlock fix (Item 1), and the larger concurrency-model migration (Item 2).

---

## Background

This started as a user report: "our UI on localhost is not loading." Live debugging (browser devtools + `go tool pprof` against the running service's `/debug/pprof` endpoint) found:

- The session list/UI hang correlates with `Instance.StopController()` (`session/instance_controller.go:137`) holding a write lock on `stateMutex` while reader goroutines (`ListSessions`, `WatchSessions`, status polling) queue up behind it. A mutex profile capture showed ~169s of cumulative reader wait time attributed to this path.
- Two follow-up Explore-agent audits (one cataloguing all 75 `stateMutex` critical sections across 12 files, one sweeping `server/services/` for direct field access) found the lock provides **mostly false confidence**:
  - `adapters.InstanceToProto` (every `ListSessions`/`WatchSessions` response) and `Instance.ToInstanceData()` (every `SaveInstances` persist) acquire **no lock at all**.
  - `server/services/session_service.go` has ~123 direct unguarded field accesses on `*Instance`, ~28 of them writes, spanning RPC handler goroutines, `AutonomousDriver`'s background goroutine (two callbacks), and an async `CreateSession` goroutine — one race already worked around manually with a code comment (`session_service.go:1280-1281`).
  - `GitHubPRURL`/`GitHubPRNumber` are locked on one writer (`PRStatusPoller`) and unlocked on another (`RunOneShot`) — same fields, inconsistent discipline.
  - `ReviewState` fields are written unlocked from `review_queue_poller.go` while sibling fields in the same embedded struct are written under `stateMutex` elsewhere.
  - `CapacityMonitor` (60s ticker), `ReviewQueuePoller` (2-8s ticker), and the per-connection WebSocket streaming handler read fields with zero locking.
- A **confirmed, reachable self-deadlock**, independent of the above: `Instance.SwitchWorkspace()` (`session/instance_workspace.go:85-219`) holds `i.stateMutex.Lock()` for its entire ~140-line body and, while still holding it, calls `i.Start(false)` synchronously three times (lines 148, 197, 206). `Start()` itself acquires the same (non-reentrant) `stateMutex.Lock()` at `instance.go:900`. Reachable via the `WorkspaceService.SwitchWorkspace` RPC (`server/services/workspace_service.go:320`) — a real path, not dead code.

Given the breadth of inconsistent locking (6+ goroutine classes touching shared state with no reliable synchronization), a piecemeal fix (e.g. converting just the originally-diagnosed hot fields to `atomic.Pointer`) would leave most of the false-confidence surface area in place. An actor-model migration fixes the race by construction instead of requiring every call site to remember correct locking — which this codebase has already proven it doesn't do reliably.

### Why an actor, not just a correctly-applied mutex

Per the `go-concurrency` skill's ladder, the alternative worth ruling out explicitly is rung 4: keep a `sync.RWMutex`, but make every `Instance` field unexported and force all ~30+ external call sites through locked accessor methods, fixing the inconsistent-locking problem by encapsulation rather than by removing the mutex. That *would* fix the false-confidence/race problem — encapsulation, not the specific primitive, is what prevents the bypassing seen today.

It would **not** fix the problem that started this investigation: `go tool pprof`'s mutex profile showed real, measurable contention from `RWMutex.RLock()` — every read-lock acquisition does an atomic increment on shared internal state, so readers across cores contend with each other even though they aren't logically blocking each other. `ListSessions`/`WatchSessions` read every `Instance` on every call; under load this reader-side cache-line contention is exactly what showed up as UI hangs. `atomic.Pointer[InstanceSnapshot].Load()` has no equivalent cost — it's a single atomic read with no shared mutable counter to contend on.

So the actor + atomic-snapshot model is chosen for two independent reasons, not one: it fixes the encapsulation/false-confidence problem (which a properly-applied mutex would also fix), **and** it fixes the reader-contention performance problem (which a properly-applied mutex would not). Validated against three independent research passes (stack, features, architecture — see `research/`): a single shared actor for all Instances was explicitly considered and rejected (it reproduces the same head-of-line blocking as today's mutex, per the client-go `SharedInformer` comparison in `research/features.md`); one actor goroutine per `Instance` was confirmed as the right granularity for this codebase's scale (hundreds of long-lived sessions, not thousands).

---

## Item 1 — Fix the `SwitchWorkspace` Reentrant Deadlock

### Problem
`SwitchWorkspace` holds `stateMutex.Lock()` across calls to `Start()`, which tries to acquire the same non-reentrant lock on the same goroutine → guaranteed deadlock on every call that reaches this path.

### Requirements
- R1.1: Restructure `SwitchWorkspace` (`session/instance_workspace.go:85-219`) so `i.stateMutex.Unlock()` happens before any of its 3 calls to `i.Start(false)` (lines 148, 197, 206).
- R1.2: No behavior change to `SwitchWorkspace`'s external contract — same return values, same error handling, same side effects (directory change, git worktree/revision switch, tmux session restart).
- R1.3: Add a regression test that exercises `SwitchWorkspace` end-to-end with a timeout, so the test fails fast (not hangs forever) if the reentrancy bug is reintroduced. `go test -race` will not catch this — it's a deadlock, not a data race.
- R1.4: Ship as its own PR, independent of Item 2, so this hang is fixed regardless of how long the larger migration takes.

### Out of Scope
- Any change to the actor/atomic-snapshot model (that is Item 2).

---

## Item 2 — Actor Model + Atomic Snapshot Migration

### Problem
~30+ call sites across `session/instance*.go`, `server/services/session_service.go`, `session/autonomous_driver.go`, `session/pr_status_poller.go`, `server/services/capacity_monitor.go`, `session/review_queue_poller.go`, and `server/services/connectrpc_websocket.go` mutate or read `Instance` fields with inconsistent or absent locking.

### Requirements

- R2.1: Define `InstanceSnapshot` — an immutable struct covering the full set of mutable `Instance` fields currently read by `ToInstanceData()` and `InstanceToProto()`. Genuinely immutable-after-construction fields (`ID`, `UUID`, `CreatedAt`) may live outside the snapshot.
- R2.2: Each `Instance` owns exactly one goroutine (the actor) and a buffered channel mailbox. All mutation is expressed as a command sent to the mailbox; no other goroutine mutates `Instance` fields directly after construction.
- R2.3: The actor goroutine, after applying a command, builds a new `InstanceSnapshot` and publishes it via `atomic.Pointer[InstanceSnapshot].Store()`.
- R2.4: All read paths (`InstanceToProto`, `ToInstanceData`, `CapacityMonitor`, `ReviewQueuePoller`, `ConnectRPCWebSocketHandler`, `PRStatusPoller`) read via `Load()` on the snapshot pointer — no blocking, no lock.
- R2.5: Compound operations (`transitionTo`'s guard/hook state machine, `UpdatePRStatus`'s 8-field write, checkpoint creation, `SwitchWorkspace`) become single commands processed atomically by the actor — one command, one resulting snapshot, no partial-update visibility.
- R2.6: `stateMutex` is removed entirely from `Instance` once migration is complete — no parallel locking scheme left behind.
- R2.7: The mailbox is a plain buffered Go channel (multi-producer/single-consumer), not a third-party lock-free queue — this workload (RPC calls, 2-60s poller ticks) does not need a lock-free ring buffer's throughput; revisit only if profiling shows the channel itself is hot.
- R2.8: `instance_tmux.go`'s 10 sites that currently hold `RLock()` across tmux subprocess I/O are migrated to read a snapshot field for the precondition check, then perform I/O without holding anything.
- R2.9: No behavior change visible to the UI — `ListSessions`/`WatchSessions`/`UpdateSession` (pause/resume/rename/program-switch) must behave identically from the client's perspective.
- R2.10 (SUPERSEDED — see R2.11-R2.17 below): `FromInstanceData`/`Storage.LoadInstances()` is called, beyond the one canonical live-registry construction path (`server/dependencies.go:438`), from ~20 other call sites that construct a throwaway `[]*Instance` and discard it. The original disposition (classify each call site as "convert to a read path" or "call `Stop()` after use," per `decisions/ADR-030-lightweight-read-path-over-decoupled-activation.md` and `implementation/plan.md` Epic 2.5) was found by a third adversarial review pass to be incomplete: `LoadInstances()` constructs the *entire* persisted session list per call, so per-match `Stop()`-after-use leaves every sibling `Instance` from the same call leaking its actor; two ticker-driven full-registry reconstructions (`daemon/daemon.go`) can independently construct a live actor for the same session simultaneously — a correctness bug (two actors racing to own one tmux target), not merely a leak. Both of ADR-030's original options relied on getting call-site discipline right, which this migration exists to stop requiring. Superseded by the design below.

### Lifecycle Ownership: `Registry` + `LiveInstance` (replaces R2.10's call-site classification)

Applying "make illegal states unrepresentable" (see the `type-driven-design` skill) to the actual defect: today `*Instance` conflates two roles — a disposable read projection of persisted data, and the one-and-only live, actor-owning handle for a session — with nothing in the type system distinguishing which role a caller holds, so nothing prevents constructing the live role twice for one session. The fix is a type split with a smart constructor that makes duplication impossible to construct, not merely discouraged.

Per `architecture-best-practices.md`'s Dependency Inversion principle and this repo's own existing manual-DI convention (`server/dependencies.go`'s staged `BuildCoreDeps` → `BuildServiceDeps` → `BuildRuntimeDeps` → `BuildDependencies`, itself dependency-injecting each stage's dependencies via constructor parameters rather than package globals), the registry is built and injected the same way, not introduced as a new global-state pattern:

- R2.11: `session.InstanceData` (already exists) remains the free-to-construct, read-only value type. It carries no lifecycle and needs no cleanup — used for every call site that only needs session data, not a live handle.
- R2.12: Introduce `session.LiveInstance` as the actor-owning handle (may be a renamed/refactored `Instance`, or a new type wrapping it — implementation's choice, decided during planning). The **only** way to obtain a `*LiveInstance` is `Registry.Acquire(sessionID string) (*LiveInstance, release func(), error)`. No other exported constructor for a live, actor-owning handle exists anywhere in the `session` package — per Go's unexported-field smart-constructor idiom (`go-development` skill), holding a `*LiveInstance` is itself proof that lifecycle is correctly managed.
- R2.13: `Registry` holds `map[sessionID]*entry{instance *LiveInstance; refcount int}` behind a single mutex that guards **map access only** (not per-field `Instance` state — this is a different, much narrower lock than the `stateMutex` this migration eliminates). `Acquire` either returns the existing entry with `refcount++`, or constructs the `LiveInstance` and spawns its actor exactly once. This makes `daemon/daemon.go`'s duplicate-actor bug a compile-time/structural impossibility: there is no code path that can produce a second live actor for a session that already has one.
- R2.14: `release()` (returned by `Acquire`) decrements the refcount; the registry stops the actor and removes the map entry only when the count reaches zero. This replaces R2.10's per-call-site `Stop()`-after-use requirement — callers uniformly call `Acquire`/`release()` regardless of whether they're a one-off lookup or the canonical long-lived owner; the registry, not the caller, decides when the actor's lifecycle actually ends. An idle-timeout in the registry is a second-order safety net for refcount-accounting bugs, not the primary reap mechanism.
- R2.15: `Registry` is constructed once in `server/dependencies.go` (the appropriate stage — likely `BuildServiceDeps` or `BuildRuntimeDeps`, decided during planning) and injected into every service that currently calls `FromInstanceData`/`LoadInstances()` directly (`SessionService`, `WorkspaceService`, `GitHubService`, `UnfinishedWorkService`, the MCP tool handlers, `HealthChecker`, `HibernationSweeper`, `daemon.Daemon`, etc.) as a constructor parameter — no package-level global `Registry` variable, consistent with `architecture-best-practices.md`'s "avoid mutable global state" and this repo's existing constructor-injection convention.
- R2.16: All ~30+ existing call sites (the ~20 from R2.10 plus the canonical `server/dependencies.go:438` path plus any call site not yet enumerated) convert to either `InstanceData` (read-only) or the injected `Registry`'s `Acquire`/`release()` (needs a live handle) — no call site is trusted to get actor lifecycle right on its own; the type system enforces it structurally instead.
- R2.17: `Registry` is itself a small, focused interface at its call sites (Interface Segregation — `go-development`/`architecture-best-practices` skills): callers depend on an `Acquire(sessionID string) (*LiveInstance, func(), error)`-shaped abstraction, not a concrete struct with unrelated methods, so it can be faked in unit tests without a real tmux/git backend.
- R2.18a: `Registry.Register(instance *LiveInstance) (release func(), error)` — the construction-time counterpart to `Acquire`. `CreateSession` builds a brand-new `LiveInstance` (via `NewInstance`, no persisted data to look up yet) and hands it to `Register`, which inserts it into the registry's map with refcount 1 (erroring if an entry for that session ID already exists, preventing double-registration) and returns a `release()` with the same semantics as `Acquire`'s. This keeps "there is exactly one way to get a live handle into the registry" true for both the lookup-existing path (`Acquire`) and the construct-new path (`Register`), rather than letting `CreateSession` bypass the registry's dedup guarantee.
- R2.18: `DeleteSession` force-invalidates immediately: it tears down the session's actor regardless of how many other callers currently hold a reference via `Registry.Acquire`, rather than waiting for all holders to `release()` naturally. In-flight callers observe an error on their next interaction with that session (e.g. "session was deleted"). Chosen over waiting-for-drain because delete should mean gone now from the user's perspective, and blocking/queuing delete behind a slow or stuck holder would reintroduce the same class of ordering hazard this project has already hit twice (see `research/pitfalls-registry.md` §4).

### Out of Scope
- Replacing the channel mailbox with a third-party lock-free queue (rejected — see R2.7).
- Any change to the proto/wire format or RPC contracts.
- Performance work beyond removing the lock contention identified here (no new caching layers, no batching).

---

## Verification

- `go build ./... && make test` passes.
- `go test -race ./session/... ./server/services/...` run **before** any change to baseline existing races, and again after to confirm they're gone.
- Targeted regression test for `SwitchWorkspace` (Item 1), timeout-bounded.
- Re-run the `go tool pprof` mutex/block profile capture (this session's method) against a running instance under realistic session load — contention attributed to `Instance` access should drop to ~0 post-migration (no mutex left to contend on).
- Manual UI check: pause/resume, rename, program switch, and the session list/watch stream behave identically pre- and post-migration.
