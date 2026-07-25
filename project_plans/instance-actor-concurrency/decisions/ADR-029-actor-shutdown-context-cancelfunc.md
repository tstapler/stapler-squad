# ADR-029: Actor Stop Signal via `context.Context`/`CancelFunc`, Matching Existing Poller Idiom

**Status**: Accepted
**Date**: 2026-06-30
**Deciders**: Tyler Stapler
**Relates to**: `project_plans/instance-actor-concurrency/implementation/plan.md` (Open Decisions §2)

---

## Context

`research/stack.md` flagged the actor's stop-signal mechanism (`context.Context` vs. a dedicated
`stopCh`) as not confirmed in that research pass, deferring to verification against this repo's
existing shutdown machinery before Epic 3 (the first epic that spins up a real actor goroutine).

Checked directly: `server/server.go`'s `Shutdown()` (line 701) cancels `s.connCtxCancel`
(a `context.WithCancel` derived context) first, then runs `s.shutdownHooks`, then calls
`s.httpServer.Shutdown(ctx)` with a 10s timeout context. Separately, `session/pr_status_poller.go`
(lines 73-74, 142, 148, 157) already establishes the exact per-worker idiom: a `ctx
context.Context` / `cancel context.CancelFunc` field pair, a `Start(ctx context.Context)` that
derives its own cancellable child via `context.WithCancel(ctx)`, and a `Stop()` method that calls
`cancel()`. `session/review_queue_poller.go` and `server/services/capacity_monitor.go` follow the
same shape.

## Decision

Each `Instance` actor's run loop selects on a `context.Context` (derived via `context.WithCancel`
at actor-start time), not a dedicated `stopCh`, matching `PRStatusPoller`'s existing pattern
exactly: a `ctx`/`cancel` field pair on the actor's owning struct, a `Stop()` method that calls
`cancel()`, and the run loop's `select` including `case <-ctx.Done(): return`.

Two distinct triggers call `Stop()`, per `research/pitfalls.md`'s finding that actor lifecycle
should track session deletion, not pause/hibernate:
- **Primary**: `RemoveInstance` call sites (`server/services/session_service.go:1827,1832`) —
  the actor goroutine must not outlive its `Instance` being removed from storage.
- **Secondary, safety net**: register a `shutdownHooks` entry (the same mechanism
  `server.Shutdown()` already uses for "persist instance state" today) that stops every live
  actor on full server shutdown, so a process exit doesn't rely solely on OS process teardown to
  reclaim goroutines — useful for clean `goleak` results in tests that exercise full server
  startup/shutdown.

## Consequences

### Positive
- Zero new shutdown idiom introduced — an engineer who already understands
  `PRStatusPoller`/`ReviewQueuePoller`/`CapacityMonitor`'s lifecycle immediately understands the
  actor's.
- `context.Context` composes with the `sendCtx` context-bounded mailbox-send variant
  (`research/architecture.md` §5) using the same `ctx` value, rather than needing a second
  cancellation primitive threaded alongside a `stopCh`.

### Negative / Accepted tradeoffs
- `context.Context` carries a small overhead (interface allocation, parent-chain walk) versus a
  bare `chan struct{}` close-to-signal `stopCh` — irrelevant at this workload's scale (a few
  hundred actors, infrequent start/stop), consistent with ADR-026's reasoning for rejecting a
  lock-free queue on the same grounds.
- Goroutine-leak verification (the `goleak` test recommended in `research/features.md` §4 and
  scheduled in Epic 3) must explicitly call `Stop()` and wait for the run loop to exit before
  asserting no leaked goroutines — `context.Done()` firing does not, by itself, guarantee the
  goroutine has finished its current command and returned.

### Addendum (2026-07-01, per fourth adversarial review pass, finding 3)

Re-verified directly against `session/pr_status_poller.go:157-164`: `PRStatusPoller.Stop()` does
**not** stop at `cancel()` — it also calls `p.wg.Wait()` immediately after, which blocks the
caller until `pollLoop()`'s goroutine has actually returned. This ADR's Context/Consequences
sections above understated that: the "must explicitly call `Stop()` and wait for the run loop to
exit" language described the *caller's* responsibility as if `Stop()` itself only cancels, when
in fact `PRStatusPoller.Stop()` already does the waiting internally.

This matters for `instance-actor-concurrency`'s Registry-managed actor (`LiveInstance.stopActor()`,
`implementation/plan.md` Epic 2.5 Story 2.5.3 / Epic 3 Task 3.1b): Design A's acquire-during-
teardown synchronization (`research/pitfalls-registry.md` §3) requires `stopActor()` to block
until the actor's run loop has actually exited, so that `Registry.release()`'s guarantee ("an
`Acquire` for this ID after `release()` returns is guaranteed a fresh, independent `LiveInstance`")
holds as implemented, not merely as commented. Per this addendum, that requirement is **consistent
with the actual `PRStatusPoller` precedent** (cancel, then wait for the goroutine to exit) — not a
deliberate per-callsite exception to it. `stopActor()`'s implementation is `i.cancel(); <-i.done`,
where `i.done` is a channel closed via `defer close(i.done)` inside `runActor()` — the same
cancel-then-wait shape `PRStatusPoller.Stop()` already uses, expressed with a `done` channel
instead of a `sync.WaitGroup` (either primitive works; `done` is chosen here since a single actor
goroutine, unlike `PRStatusPoller`'s pool-style workers, has nothing else to `Add()`/`Done()` for).
