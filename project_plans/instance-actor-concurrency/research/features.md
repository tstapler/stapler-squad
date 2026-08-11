# Comparable Systems Research — Instance Actor/Snapshot Concurrency Model

Research into how other real-world Go systems manage many independent, long-lived,
mutable "session"-like objects concurrently, evaluated against the planned migration:
one owning goroutine per `session.Instance` (actor, commands via buffered channel
mailbox) + `atomic.Pointer[InstanceSnapshot]` copy-on-write for lock-free reads.

Workload shape that matters for evaluating "borrowable-ness": a few hundred concurrent
`Instance`s (not thousands/millions), each receiving bursty low-frequency commands (RPC
calls, 2–60s poller ticks), with reads (`ListSessions`, `WatchSessions`, websocket
streaming) far outnumbering writes.

---

## 1. `net/http.Server` — goroutine-per-connection (partial match)

`net/http/server.go`'s `conn` struct is the closest stdlib precedent for "one goroutine
owns a value, nothing else touches it directly." Each accepted connection gets
`go c.serve(connCtx)`, and `conn`'s fields split cleanly:

- **Immutable**: `server`, `rwc net.Conn` — set once.
- **Single-goroutine-owned**: `remoteAddr`, `r *connReader`, `bufr`, `bufw`,
  `lastMethod` — written/read only inside `serve()`.
- **Explicitly synchronized**: `hijackedv bool` behind `mu sync.Mutex` (because
  `Hijack()` can race with `serve()` from a handler goroutine), plus `curReq`,
  `curState` as `atomic.Pointer`/`atomic.Uint64` so `Server.ConnState` hooks and
  `/debug` introspection can read cheaply without blocking the serve loop.

**Borrowable**: This validates the *shape* of the design — one owning goroutine, a tiny
number of fields exposed to outsiders via atomics rather than a general-purpose mutex.
It's a direct precedent for "the actor owns everything; only the snapshot pointer
crosses the goroutine boundary."

**Where it doesn't map**: `conn` is short-lived (one request/response cycle, then
close). `session.Instance` lives for the agent's entire lifecycle and today is mutated
not just by "its own" logic but by `server/services/session_service.go` RPC handlers,
`AutonomousDriver`'s background goroutine, `PRStatusPoller`, `ReviewQueuePoller`, and
`CapacityMonitor` — i.e., multiple *external* writers, which is exactly the problem
`net/http.conn` never had to solve. The mailbox is the part of this migration with no
stdlib precedent; it has to come from the actor-model sources in §4.

Source: https://go.dev/src/net/http/server.go

---

## 2. `database/sql` connection pooling — mostly a red herring

`sql.DB.mu sync.Mutex` guards `freeConn []*driverConn`, `connRequests
map[uint64]chan connRequest`, and `numOpen`. A few stats counters
(`waitDuration atomic.Int64`, `numClosed atomic.Uint64`) are atomics, but there is no
per-connection actor goroutine. Connections are fungible — any goroutine may acquire any
free `driverConn`; concurrency is handled by a mutex plus a channel-based "promise"
handoff (`connRequest`), not by per-connection ownership.

**Verdict**: Not a useful structural comparison for this migration. `database/sql` is
solving "pool of identical, swappable resources," not "N independently-stateful
long-lived entities, each needing serialized access to its own state." The one
transferable idea — a goroutine blocking on a private reply channel waiting for another
goroutine to hand it a result — is the same request/reply shape as a mailbox command's
response channel, but it's Pike's pattern (§4), not something specific to `database/sql`.

Source: https://go.dev/src/database/sql/sql.go

---

## 3. client-go SharedInformer — single shared actor, not per-object actors

Architecture: `Reflector` → `DeltaFIFO` (one queue of deltas for *all* watched objects)
→ a single `processLoop` goroutine that pops deltas, applies them to a
`ThreadSafeStore` (just an `RWMutex`-guarded map — not per-object actors), then calls
`sharedProcessor.distribute()` to fan out to registered `ResourceEventHandler`s — **still
on the same processLoop goroutine**. Reads (`Get`/`List`/`ByIndex`) just `RLock()` the
whole store.

This is the inverse of what's planned here: **one** goroutine serializes processing for
**all** objects; the store is a conventional locked map, not N actors. The documented
failure mode (Render engineering blog, corroborated by k8s issue threads) is head-of-line
blocking: a slow or blocking event handler stalls delta delivery for every other object
in the cluster, and naive unbounded buffering of pending deltas has caused real
memory-blowup incidents. The standard mitigation is to bolt a `workqueue.Interface` onto
the handler — the handler does nothing but enqueue a key, and separate worker goroutines
do the real (possibly slow) work off the shared processing goroutine.

**Is this a better model than per-Instance actors? No, for this codebase.** It's the
right tradeoff when the "objects" are transient cache deltas with no need for continuous
ownership. `session.Instance` is the opposite: it needs a long-lived owner (tmux
lifecycle, pollers, status state machine) for its whole existence, which is precisely
why one-actor-per-Instance was chosen over a shared queue. A single shared actor for all
~hundreds of Instances would reintroduce exactly the same head-of-line blocking problem
that motivated this migration in the first place (`StopController()` holding
`stateMutex` and blocking unrelated readers, per `requirements.md`) — just moved from a
mutex to a queue.

**Borrowable**: The mitigation discipline — keep each unit of work in the
single-consumer loop fast, defer anything slow — is directly applicable *inside* each
Instance's own command loop. R2.5 already groups `transitionTo`, `UpdatePRStatus`,
checkpoint creation, and `SwitchWorkspace` into single atomic commands; the informer
lesson reinforces that none of those command handlers should block on slow I/O (tmux
subprocess calls, git operations) while holding up that Instance's mailbox — long
operations should kick off and report back via a follow-up command/snapshot update
rather than executing synchronously inside the `case` arm if they can take more than
tens of milliseconds. `instance_tmux.go`'s 10 `RLock()`-across-subprocess-I/O sites
(R2.8) are exactly the pattern to avoid carrying into the actor's command loop.

Sources: https://render.com/blog/kubernetes-informers,
https://pkg.go.dev/k8s.io/client-go/tools/cache

---

## 4. "Actor model in Go" prior art

- **Rob Pike, "Go Concurrency Patterns" (Google I/O 2012)** —
  https://go.dev/talks/2012/concurrency.slide. Establishes the request/response-over-channel
  idiom this migration's mailbox descends from: a request struct carrying its own reply
  channel (`type request struct { args []int; resultChan chan int }`), sent to a single
  goroutine that owns the relevant state and replies asynchronously. This is the direct
  ancestor of "send a command struct with an embedded reply/ack channel to the Instance's
  mailbox."

- **Bryan C. Mills, "Rethinking Classical Concurrency Patterns" (GopherCon 2018)** —
  https://www.youtube.com/watch?v=5zXAHh5tJqQ. Central thesis: goroutines are cheap
  enough that "start one when you have concurrent work" beats pre-allocated worker pools
  built for the era of expensive OS threads; "share memory by communicating" rather than
  synchronizing shared memory. This is the strongest available argument *for*
  goroutine-per-Instance at this project's scale — Mills' point is precisely that
  per-unit goroutines don't need to be justified by scale, they're the default-correct
  choice unless profiling shows otherwise.

- **golang-nuts "Channels vs Actors" thread** —
  https://groups.google.com/g/golang-nuts/c/uJxcfNsxh-0. Useful terminology check: in a
  true actor model (Erlang-style), the mailbox *is* the actor's address, and senders
  don't block on receipt. Go's channels simulate this only when a goroutine maintains
  a strict 1:1 relationship with "its" channel and is the sole receiver — which is
  exactly the discipline R2.2 requires ("each Instance owns exactly one goroutine and a
  buffered channel mailbox; no other goroutine mutates Instance fields directly").

- **gorilla/websocket "hub" pattern** (the de facto standard production pattern for
  Go WebSocket session management) — https://websocket.org/guides/languages/go/. One
  goroutine owns a connection's write side; `gorilla/websocket` explicitly documents that
  concurrent calls to `WriteMessage` on the same connection are a bug it will panic on,
  so production code funnels all writes for one connection through a single owning
  goroutine, fed by a channel, with separate read-pump/write-pump goroutines per
  connection. This is the most directly analogous "session manager" prior art found:
  structurally identical to one-actor-per-Instance, just for a different kind of
  long-lived per-client object. No single canonical "actors in Go" blog post from a named
  engineering org (Cloudflare/Uber/Segment) describing a session-manager-per-goroutine
  pattern verbatim was found; the websocket hub pattern is the closest production-grade
  precedent and is widely repeated across Go web framework documentation.

**Tradeoffs explicitly named across these sources, applied to this migration:**

| Concern | What the sources say | Application here |
|---|---|---|
| Goroutine scaling limit | ~2–8KB stacks; comfortable into tens of thousands (Mills) | A few hundred `Instance` actors is a non-issue, not worth sharding over |
| Ordering | Channels guarantee FIFO **per sender**, not a global order across senders (golang-nuts thread) | Multiple RPC handlers/pollers sending to the same Instance's mailbox get serialized one-at-a-time processing, but no guaranteed relative order between two different callers' concurrent sends — acceptable; nothing in the requirements needs cross-caller ordering, only "no partial-update visibility" (R2.5), which serialization already gives |
| Backpressure | A buffered channel blocks the sender once full unless you add a `select default:` drop path; unbounded buffering risks the informer's memory-blowup failure mode | R2.7 already specifies a plain buffered channel; size it deliberately and let `Send` block as the natural backpressure signal for this low-frequency bursty workload — do not make it unbounded |
| Shutdown / goroutine leaks | Universal pattern: `context.WithCancel` propagated into the actor's `select` loop, channel close or `ctx.Done()` as the exit signal, cleanup via `defer` inside the owning goroutine itself (never from outside) | Directly applicable to session deletion/teardown; consider adding `goleak` (https://github.com/uber-go/goleak) to the test suite for the migration to catch a leaked actor goroutine when a session is deleted |

---

## 5. One-actor-per-instance vs. a single shared actor with routing

The informer comparison in §3 *is* the direct head-to-head: "one goroutine processes a
queue for all objects, routed by key" vs. this migration's "N goroutines, each pinned to
one key (Instance)." Synthesized tradeoffs:

- **Head-of-line blocking**: a single shared actor (informer-style) stalls *all* keys
  when any one key's handler is slow — this is documented production-incident behavior
  (Render post), and it is structurally the same failure this migration exists to fix
  (`StopController()` holding `stateMutex` for the duration of tmux teardown, blocking
  unrelated `ListSessions`/`WatchSessions` reader goroutines — see `requirements.md`
  background). A single shared actor for all Instances would just move that same
  bottleneck from a mutex to a channel; it does not solve the problem motivating this
  migration. Per-Instance actors eliminate it by construction: a slow poller tick or
  `SwitchWorkspace` for Instance A cannot delay a command or read for Instance B.
- **Memory/scheduling cost**: negligible at "a few hundred" goroutines per Mills'
  argument — squarely inside the regime Go's M:N scheduler is designed for. Not a
  consideration worth weighing against the isolation benefit above.
- **Ordering guarantees**: a single shared actor gives a global total order across all
  keys "for free" (useful only if cross-Instance invariants existed); N independent
  actors give per-Instance ordering only, with no defined relative order across
  different Instances. Stapler-squad's domain has no documented cross-Instance ordering
  requirement — Instances are independent agent sessions — so this is not a cost.
- **Sharded/partitioned middle ground**: well-documented as a general pattern for
  *locks* (sharded mutex maps, e.g. hashing keys across N mutex-protected shards to
  reduce contention by ~N: https://strebkov.dev/posts/shard-your-locks/), but no
  Go-specific literature was found describing a "sharded actor pool" (N actor goroutines
  each owning a partition of keys via routed channel) as a distinct named pattern. It
  would only be motivated by goroutine-count or scheduling pressure that doesn't exist
  at this project's cardinality (hundreds of Instances, not tens of thousands) — adding
  a partitioning layer here would be solving a contention problem the system doesn't
  have, at the cost of extra indirection. **Recommendation: do not adopt sharding; one
  actor goroutine per `Instance` as already specified in R2.2 is the simpler and
  correctly-scoped choice.**

---

## Summary: what's directly borrowable

1. **`net/http.conn`'s shape** (owning goroutine + atomics for the few cross-goroutine
   reads) confirms the general design pattern is stdlib-precedented, not novel — but the
   mailbox-for-external-writers part has no stdlib analog and must come from Pike/Mills.
2. **`database/sql`'s pool** contributes nothing structurally beyond the
   request/reply-channel idiom it shares with Pike's original pattern; do not treat it as
   a model to follow.
3. **client-go's informer** is the most useful *negative* example: it demonstrates
   exactly the head-of-line-blocking failure this migration is meant to avoid, validating
   the choice of per-Instance actors over a single shared actor — and its retrofitted
   workqueue mitigation is a direct lesson for keeping each Instance's own command-loop
   handlers fast (R2.5, R2.8: don't hold the actor loop hostage to tmux subprocess I/O).
4. **Pike's request/reply-channel pattern and Mills' "goroutines are cheap" argument**
   are the strongest direct justification for the chosen design at this project's scale;
   the gorilla/websocket hub pattern is the closest production-grade precedent for
   "one owning goroutine arbitrates all mutation of one long-lived per-client object,"
   reinforcing R2.2's single-writer discipline.
5. **No sharding layer is warranted.** One actor per `Instance`, a plain buffered channel
   mailbox (R2.7), and blocking-send backpressure are the right level of mechanism for a
   few hundred Instances receiving bursty, low-frequency commands. Add `goleak`-style
   leak detection to the migration's test plan to guard actor-goroutine teardown on
   session deletion.
