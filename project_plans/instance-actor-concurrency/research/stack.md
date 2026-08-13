# Stack Research — Instance Actor + Atomic Snapshot Migration

Scope: stdlib primitives, third-party actor libraries, shutdown patterns, and
request/response-over-channel prior art for Item 2 (actor model + `atomic.Pointer`
copy-on-write migration for `session.Instance`).

---

## 1. `sync/atomic`'s `atomic.Pointer[T]` — exact API and gotchas

### Confirmed API (Go 1.19+, stdlib `sync/atomic`)

```go
type Pointer[T any] struct { /* no exported fields */ }

func (x *Pointer[T]) Load() *T
func (x *Pointer[T]) Store(val *T)
func (x *Pointer[T]) Swap(new *T) (old *T)
func (x *Pointer[T]) CompareAndSwap(old, new *T) (swapped bool)
```

`Pointer[T]` was the standard library's first public generic type (Go 1.19). It wraps the same
atomic primitives `atomic.Value`/`unsafe.Pointer` used previously, but is statically
type-safe — no `interface{}` assertion, no panic risk from a mismatched stored type — and
compiles down to the same hardware atomic load/store instructions, so there's no performance
cost to preferring it.

### Zero-value behavior

The zero value of `atomic.Pointer[T]` is a pointer whose `Load()` returns `nil` (a `*T` nil,
not a zero-valued `T`). `i.snapshot.Load()` returns `nil` until the actor's first `Store()`
call. **Fix at construction, not at every read site:** have `NewInstance`/the actor's
`Start()` perform a synchronous "publish initial snapshot" step *before* returning the
`*Instance` to any caller, so `Load()` is guaranteed non-nil by the time any other goroutine
can reach it. Don't rely on every read path (`InstanceToProto`, `ToInstanceData`,
`CapacityMonitor`, etc.) defensively nil-checking forever — that reintroduces the same kind of
"works by accident" inconsistency this migration is meant to eliminate. One invariant
("snapshot is never nil after construction returns"), enforced once, at construction.

### Gotchas with non-comparable fields (slices, maps, embedded structs)

`atomic.Pointer[T]` does not require `T` to be comparable — `CompareAndSwap` compares
**pointer values** (`*T == *T`, identity), not deep/structural equality of `T`'s contents. So
`T` containing slices, maps, or other non-comparable types is completely fine — `Pointer[T]`
never compares `T` values themselves, only pointers to them.

The actual gotcha is not in the atomic primitive — it's in **snapshot construction
discipline**. The audited `Instance` fields that need explicit handling:

| Field | Type | Snapshot copy rule |
|---|---|---|
| `Tags` | `[]string` | Copy the slice header **and** allocate a new backing array (`append([]string(nil), src...)`), or two snapshots alias the same backing array and a later in-place mutation (e.g. by `TagManager`) becomes visible in an "old" snapshot a reader still holds. |
| `Checkpoints` | `CheckpointList` (`[]Checkpoint`) | Same rule — `Checkpoint` is a plain value struct (no nested pointers/slices), so a shallow element copy suffices, but the slice header must still be reallocated, not shared. |
| `Permissions` | `InstancePermissions` (bools + a map for `RequiresConfirmation`) | Copies by value on assignment, but any `map[string]bool` field shares its header — same gotcha as `Tags`. Needs explicit map copy if mutated post-construction. |
| `ExternalMetadata` | `*ExternalInstanceMetadata` (pointer) | Copying the snapshot struct only copies the pointer, not the pointee. Set once at discovery and not mutated afterward today, so sharing is safe only if the actor enforces "never mutate in place after first publish." Safer default: deep-copy on every snapshot build for uniform safety, consistent with the "no shared mutable state across snapshots" invariant. |
| `EnvVars` | `map[string]string` | Same map-aliasing gotcha — copy key-by-key into a new map on every snapshot build if ever mutated post-construction. |

**General rule for `InstanceSnapshot` construction:** every reference type (slice, map,
pointer to a mutable struct) embedded in the snapshot must be either (a) deep-copied into a
fresh allocation on every `Store()`, or (b) provably never mutated in place after the
snapshot is published. Given this codebase's history ("6+ goroutine classes touching shared
state with no reliable synchronization... has already proven it doesn't do reliably" —
requirements.md), prefer deep-copy uniformly over "this one's safe because nothing mutates it"
reasoning — that produced the current bug surface. The cost (one extra slice/map allocation
per command) is negligible at this workload's scale (RPC calls + 2–60s poller ticks).

**Does non-comparability block anything?** No — `atomic.Pointer[T]`'s only constraint on `T`
is `any`. It's irrelevant to whether `atomic.Pointer[InstanceSnapshot]` compiles; it only
matters for the aliasing concern above. The actor is the sole writer, so `Store()` — not
`CompareAndSwap` — is the only operation needed; there's no concurrent-writer race to
arbitrate via CAS (R2.2: "no other goroutine mutates `Instance` fields directly after
construction").

---

## 2. Third-party Go "actor model" libraries — hand-roll vs. adopt

Evaluated the most-cited Go actor libraries against the actual requirement here: one actor per
`Instance` (dozens of long-lived, in-process, non-distributed actors), synchronous
request/response semantics for several methods, no clustering/remoting/supervision-tree need.

| Library | What it is | Fit assessment |
|---|---|---|
| [`asynkron/protoactor-go`](https://github.com/asynkron/protoactor-go) | Distributed actor platform (Go/C#/Java/Kotlin), protobuf-based remoting, 2M+ msgs/sec across nodes. Actively maintained (commits as recent as Jan 2026). | Massive overkill — built for distributed clustering, remoting, cross-language actor systems. Large transitive dependency surface for a problem with no remoting/multi-process actors. |
| [`anthdm/hollywood`](https://github.com/anthdm/hollywood) | "Blazingly fast and light-weight" engine, 10M+ msgs/sec single-node. | Closer on weight, but still its own actor lifecycle/mailbox/supervision abstractions built for high-rate actor *swarms* (e.g. game entities) — this project has dozens of actors processing low-frequency commands (RPC calls, 2–60s poller ticks), not millions of msgs/sec. Adopting means conforming to its actor/PID/context API for what a plain channel solves in ~50 lines. |
| [`ergo-services/ergo`](https://github.com/ergo-services/ergo) | Erlang-inspired, "zero external dependencies," network-transparent, distributed/clustered (21M+ msgs/sec local, ~5M/sec networked). | Built for Erlang-style distributed OTP semantics (supervision trees, network transparency, hot code reload). Far more machinery than "one goroutine owns one struct's mutations." |
| [`Tochemey/goakt`](https://github.com/Tochemey/goakt) | Distributed actor/grain framework, protobuf typed messages, supervision, remoting, clustering, CRDTs, streams, observability. | Same overkill profile — production distributed-systems framework. |
| [`vladopajic/go-actor`](https://github.com/vladopajic/go-actor) | Minimal single-process actor library — actors are goroutines + channels, "no learning required if you know Go channels." | Closest conceptual fit (in-process only), but still a dependency + abstraction layer (`Actor`/`Mailbox` interfaces) for something expressible directly as one `for cmd := range mailbox` loop per `Instance`, with full control over command shape and the response-channel pattern needed for `Pause() error`-style synchronous calls. |

### Conclusion: hand-roll, confirmed

All four libraries surveyed target **distributed** or **high-throughput** actor systems
(clustering, remoting, supervision trees, millions of msgs/sec) — none of that machinery
applies here: this is single-process/in-memory only (R2.2 confines mutation to one goroutine
within the process); message rate is low (RPC calls + 2–60s poller ticks, not a
high-frequency stream); and the actual requirement — one owning goroutine per `Instance`,
commands via buffered channel, occasional synchronous request/response, periodic
`atomic.Pointer` snapshot publish — is the textbook "share memory by communicating" pattern,
not a distinct "actor framework" problem. Hand-rolling also avoids a new dependency to
vet/update, a learning curve for a library's PID/context/supervision abstractions, and
constraining `InstanceSnapshot`'s publish mechanism to whatever the library expects instead of
a plain `atomic.Pointer[T]` (already specified by R2.3 independent of any actor library).

Estimated hand-rolled surface: a `command` interface (or closed set of command structs) + a
`mailbox chan command` field + one `run()` goroutine with a `for cmd := range i.mailbox` loop +
a `switch` per command type + the response-channel plumbing in §4. Consistent with the
"~50-100 lines" working assumption — confirmed, not refuted.

---

## 3. `golang.org/x/sync/errgroup` and `context.Context` for actor goroutine shutdown

### Is `errgroup` relevant here?

`errgroup.Group` is designed for launching N goroutines that all need to complete (or the
first error cancels the rest) before the calling goroutine proceeds — a fan-out with a single
combined error. That shape doesn't match the actors' own lifetime: `Instance` actors are
**long-lived** (matching session creation → deletion, not "do N units of work then return"),
and they're independent — one actor erroring has no reason to cancel every other session's
actor, the opposite of `errgroup`'s first-error-cancels-all semantics.

**Where `errgroup` genuinely helps:** the *shutdown sequence itself* — waiting for all N actor
goroutines to drain their mailboxes and exit cleanly during process shutdown (`SIGTERM`,
`Storage.Close()`). That's a genuine bounded "wait for N things to finish" problem:

```go
func (s *Storage) Shutdown(ctx context.Context) error {
    eg, ctx := errgroup.WithContext(ctx)
    for _, inst := range s.instances {
        inst := inst
        eg.Go(func() error {
            return inst.StopActor(ctx) // signals mailbox close, waits for drain or ctx.Done()
        })
    }
    return eg.Wait()
}
```

This is a reasonable place to use `errgroup` — but it's a thin wrapper around the shutdown
fan-out, not part of each actor's own run loop.

### `context.Context` for per-actor lifecycle

More directly relevant: each actor's `run()` loop should select on both the mailbox and a
`context.Context` (or a dedicated `done chan struct{}`) so it can be told to stop without
relying solely on closing the mailbox channel (closing a channel that other goroutines may
still be sending on is itself a footgun — see "stop signal" pattern below):

```go
type Instance struct {
    mailbox chan command
    stopCh  chan struct{} // closed once, signals run() to exit after draining
    // ...
}

func (i *Instance) run() {
    for {
        select {
        case cmd := <-i.mailbox:
            i.apply(cmd)
        case <-i.stopCh:
            i.drainAndExit()
            return
        }
    }
}
```

A dedicated `stopCh` (closed exactly once, via `sync.Once` or a CAS-guarded bool) is simpler
here since there's no cross-actor cancellation tree — but if the server already threads a root
`context.Context` through shutdown, reusing `ctx.Done()` avoids a second cancellation idiom.
Pick whichever matches `server/server.go`'s existing shutdown path (verify during planning —
not confirmed in this research pass).

**Summary:** `errgroup` belongs at the shutdown-orchestration layer (waiting for N actors to
stop), not inside each actor's own loop; `context.Context` (or a `stopCh`) belongs inside each
actor for per-actor stop signaling, paired with a drain step so in-flight commands aren't
dropped silently.

---

## 4. Prior art: channel-based actor wrapping a mutable struct with synchronous request/response

This is a well-established Go idiom, sometimes called the "call pattern" — predates any actor
library and appears in Go's own standard library design discussions (e.g. `net/rpc`'s internal
`Call` struct, and the canonical "Go channels are for handing off ownership of work" framing
from Rob Pike's concurrency talks).

### The pattern

Each command carries its own reply channel. The caller sends the command, then **synchronously
blocks reading from the reply channel it just created** — "send a message, wait for the actor
to process it, get a typed result back" with no external correlation/ID bookkeeping needed (no
`pending map[uint64]*Call` like `net/rpc` uses over the wire — sender and receiver are in the
same process and the reply channel itself is the correlation token):

```go
// command is the closed set of operations the actor processes.
type command interface{ apply(i *instanceState) }

type pauseCmd struct {
    reply chan error
}

func (c pauseCmd) apply(s *instanceState) {
    err := s.doPause() // mutate in-memory state, no lock needed — only the actor touches it
    c.reply <- err
}

// Pause is the public, synchronous method other goroutines call.
func (i *Instance) Pause() error {
    reply := make(chan error, 1) // buffered: actor's send never blocks even if caller times out/gives up
    i.mailbox <- pauseCmd{reply: reply}
    return <-reply
}
```

### Idiomatic details that matter

1. **Buffer the reply channel with capacity 1.** If `Pause()`'s caller times out or is
   canceled (e.g. an HTTP request context expires) before reading the reply, an unbuffered
   channel would leak the actor goroutine forever on `c.reply <- err`. Capacity 1 lets the
   actor's send always succeed immediately, regardless of whether the caller is still listening.
2. **The command type is the message; the reply channel is embedded in it**, not tracked in a
   side table — simpler than `net/rpc`'s `Call`+pending-map approach since there's no wire
   serialization or out-of-order delivery: the mailbox is a single ordered Go channel, so
   replies always correspond to the command that produced them by construction.
3. **For commands with no meaningful return value** but needing "this has been applied"
   confirmation, use `chan struct{}` instead of `chan error`, or a small `result struct{ err
   error; snapshot *InstanceSnapshot }` if the caller wants the resulting snapshot back
   immediately rather than waiting for a separate `Load()`.
4. **Context-aware variant**, for call sites that should respect cancellation/timeout
   (recommended for RPC-handler-issued commands, matching this codebase's existing
   `context.Context`-threaded handler style):

   ```go
   func (i *Instance) Pause(ctx context.Context) error {
       reply := make(chan error, 1)
       select {
       case i.mailbox <- pauseCmd{reply: reply}:
       case <-ctx.Done():
           return ctx.Err()
       }
       select {
       case err := <-reply:
           return err
       case <-ctx.Done():
           return ctx.Err()
       }
   }
   ```

   Guards both ends: enqueueing the command (mailbox might be full/actor stuck) and waiting
   for the reply (actor might be slow). Per R2.5, compound operations (`transitionTo`,
   `UpdatePRStatus`, checkpoint creation, `SwitchWorkspace`) become single commands, some of
   which may take non-trivial time (e.g. `SwitchWorkspace`'s git/tmux I/O) — the caller should
   not block forever if the actor is wedged on a slow external call.

5. **Compound/multi-field updates as one command, one struct.** Directly satisfies R2.5 ("one
   command, one resulting snapshot, no partial-update visibility") — e.g. `UpdatePRStatusCmd`
   carries all 8 fields `UpdatePRStatus` writes, applied in one `apply()` call before the
   single resulting snapshot is built, so no reader ever observes a `Load()` with some fields
   updated and others stale.

### Where this pattern is documented elsewhere

- [hassansin: "Request-response pattern over asynchronous protocol using Go channels"](https://hassansin.github.io/request-response-pattern-using-go-channles) — the general
  technique, including the ID-correlation variant needed when requests/responses interleave or
  travel over a wire (not needed here — in-process, ordered mailbox — but the broader pattern).
- `net/rpc`'s internal `Call` struct (stdlib, `net/rpc/client.go`) is the most authoritative
  "client sends, blocks on a per-call channel, server replies" example in the standard library,
  despite targeting the across-the-wire case.
- Go community consensus (golang-nuts "Channels vs Actors" thread, multiple blog writeups)
  treats "goroutine owns a struct + processes commands off a channel + replies via an embedded
  channel" as *the* idiomatic Go realization of the actor model, distinct from formal
  Erlang/Akka-style frameworks Go doesn't need a library to approximate for in-process use.

---

## 5. Go version check — `atomic.Pointer[T]` support

```
$ cat go.mod | head -3
module github.com/tstapler/stapler-squad
go 1.25.0
```

Confirmed: **Go 1.25.0**, far above the **Go 1.19** minimum required for generic
`atomic.Pointer[T]`. No toolchain upgrade needed. (Go 1.19 also introduced the rest of the
typed-atomics family — `atomic.Bool`/`Int32`/`Int64`/`Uint32`/`Uint64` — already in use
elsewhere in this codebase, e.g. `Instance.driverRunning atomic.Bool` at
`session/instance.go:338`, confirming the team is already comfortable with this API family.)

---

## Sources

[Go 1.19 atomic.Pointer](https://utcc.utoronto.ca/~cks/space/blog/programming/GoSyncAtomicPointerGeneric) · [go101.org: Atomic Operations](https://go101.org/article/concurrent-atomic-operation.html) · [protoactor-go](https://github.com/asynkron/protoactor-go) · [hollywood](https://github.com/anthdm/hollywood) · [ergo](https://github.com/ergo-services/ergo) · [goakt](https://github.com/Tochemey/goakt) · [go-actor](https://github.com/vladopajic/go-actor) · [hassansin: request-response over channels](https://hassansin.github.io/request-response-pattern-using-go-channles) · [golang-nuts: Channels vs Actors](https://groups.google.com/g/golang-nuts/c/uJxcfNsxh-0) · Local: `session/instance.go`, `session/checkpoint.go`, `session/types.go`, `go.mod`
