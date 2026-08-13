# ADR-025: One Actor Goroutine Per Instance, Not a Single Shared Actor

**Status**: Accepted
**Date**: 2026-06-30
**Deciders**: Tyler Stapler
**Relates to**: `project_plans/instance-actor-concurrency/requirements.md` (Item 2)

---

## Context

Having decided to replace `stateMutex` with an actor model (ADR-024), the granularity of "one
actor" needed to be chosen: one goroutine per `Instance`, or a single shared actor serializing
work for all `Instance`s, routed by key.

`research/features.md` evaluated client-go's `SharedInformer` as the closest real-world precedent
for the latter shape: one `Reflector` → one `DeltaFIFO` (a single queue of deltas for *all*
watched objects) → one `processLoop` goroutine that applies deltas to a `ThreadSafeStore` (an
ordinary `RWMutex`-guarded map, not per-object actors) and fans out to handlers **on that same
goroutine**. The documented production failure mode (Render engineering blog, corroborated by
k8s issue threads) is head-of-line blocking: a slow or blocking handler for any one object stalls
delta delivery for every other object in the cluster, mitigated in practice only by bolting a
`workqueue.Interface` onto the handler so it does nothing but enqueue a key for separate worker
goroutines.

This is structurally the same failure this migration exists to fix — `StopController()` holding
`stateMutex` for the duration of tmux teardown while blocking unrelated `ListSessions`/
`WatchSessions` readers (`requirements.md` background). A single shared actor for all `Instance`s
would just move that bottleneck from a mutex to a channel; it would not solve the problem
motivating this migration.

## Decision

One actor goroutine and one mailbox per `Instance` (R2.2), not a single shared actor routing
commands to all `Instance`s by key.

This is the correct granularity at this codebase's scale: a few hundred long-lived `Instance`s,
not thousands or millions. Per Bryan Mills' "Rethinking Classical Concurrency Patterns"
(GopherCon 2018, cited in `research/features.md` §4), goroutines are cheap enough (~2-8KB stacks)
that per-unit goroutines are the default-correct choice at this cardinality, not something that
needs to be justified by scale — Go's M:N scheduler is squarely designed for "tens of thousands"
of goroutines, and this migration needs a few hundred. A slow poller tick or `SwitchWorkspace`
call for `Instance` A cannot delay a command or read for `Instance` B, because there is no shared
queue or shared lock between them — the isolation is structural, not the product of careful
handler discipline.

A sharded/partitioned middle ground (N actor goroutines each owning a partition of `Instance`s via
a routed channel) was also considered and rejected: it is a well-documented pattern for *sharding
locks* to reduce contention, but it would only be motivated by goroutine-count or scheduling
pressure this project does not have at hundreds of `Instance`s. Adding a partitioning layer here
solves a contention problem the system doesn't have, at the cost of extra indirection.

## Consequences

### Positive
- Head-of-line blocking is eliminated by construction, not by careful handler discipline: no
  command or read for one `Instance` can be delayed by another `Instance`'s slow work.
- No new failure mode to design around (unlike a shared-actor model, which would need a
  `workqueue`-style decoupling layer to avoid reproducing the informer's documented incident
  history).
- Matches existing precedent in this codebase: every long-lived background goroutine in
  `session/` already runs as an independent per-entity worker pattern conceptually (pollers,
  not a single dispatcher).

### Negative / Accepted tradeoffs
- No global ordering guarantee across `Instance`s — channels guarantee FIFO per sender, not a
  total order across all senders to different `Instance`s. Accepted: `Instance`s are independent
  agent sessions with no documented cross-Instance ordering requirement.
- A few hundred goroutines remain parked on an idle channel receive between commands (e.g. while
  a session is paused/hibernated). This costs only a few KB of stack each — negligible at this
  scale (`research/architecture.md` §6) — and is not a reason to reach for a shared/sharded model.
- The "keep each command handler fast" discipline the informer's retrofitted `workqueue` had to
  learn the hard way still applies *within* a single `Instance`'s own command loop — slow tmux
  subprocess I/O must not run inside a command closure that blocks that one `Instance`'s mailbox
  (R2.5, R2.8); per-Instance actors remove the *cross-Instance* version of this problem, not the
  need for handler-level care.
