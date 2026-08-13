# Build vs. Buy: Reconnect-Safe Outgoing Message Pipeline

Agent 6 research for backlog item `04089969-0f19-499c-be34-2e8bcfc4f13e` (phantom
repeated "1" keystroke on session open/reconnect).

## Scope

Evaluates whether to replace the hand-rolled `MessageQueue`
(`web-app/src/lib/terminal/MessageQueue.ts`, 68 lines) — or the surrounding
connect/reconnect machinery in `useTerminalStream.ts` /
`websocket-transport.ts` / `connectrpc_websocket.go` — with an existing
library, versus applying a small targeted guard.

## Current mechanics (relevant to the decision)

- `MessageQueue` is a single-producer/single-consumer async queue: `push()`
  either resolves a pending promise or buffers into an array; the
  `Symbol.asyncIterator` generator is handed directly to
  `client.streamTerminal(queue)` as the ConnectRPC bidi-stream input. It has
  no notion of "which connection" a buffered item belongs to.
- `useTerminalStream.connect()` creates a **new** `MessageQueue` and a new
  `AbortController` per connection attempt, stores them in
  `messageQueueRef`/`abortControllerRef`, and starts a `streamTerminal(...)`
  call plus a detached `(async () => { for await (const msg of stream) ... })()`
  read loop. `pushMessageRef.current` always dereferences
  `messageQueueRef.current` at call time (not a stale closure), so keystrokes
  typed *after* a reconnect correctly go to the new queue.
- The gap is on the **old** connection's lifecycle: `disconnect()` calls
  `messageQueueRef.current.close()` and awaits an abort with a 1s grace
  timeout, but nothing prevents a *new* `connect()` from starting, and the
  old stream's write-side async generator (`websocket-transport.ts`'s
  `for await (const msg of req.message)` loop inside `next()`) keeps draining
  whatever was already buffered in the old `MessageQueue` until it observes
  the abort signal or the queue closes. Under the ticket's "flapping"
  condition (connect → stopped → reconnect in quick succession), there is a
  window where an old queue's buffered/pending input and a new queue's
  handshake+input can both reach tmux, or a keystroke queued right at the
  disconnect boundary gets delivered by the old pipe *and* effectively
  replayed by app-level retry/resync logic — this is the suspected
  duplication point, not proven here (that's a separate agent's job).
- Server side (`connectrpc_websocket.go`, `streamViaControlMode`): each
  WebSocket upgrade spawns **fresh** goroutines (`doneChan`, `errChan`,
  `resizeCh`) scoped to that one HTTP/WS connection; nothing on the Go side
  persists or replays input across reconnects — a dropped connection's
  goroutines simply exit. The Go side is not a re-emission risk for replay
  *across* connections; at most it needs a context-cancellation guard so a
  slow `SendInputViaControlMode` from a dying connection can't race a new
  connection's writes to the same tmux pane (mentioned in requirements as
  in-scope).

## Option 1 — Existing OSS async queue / cancellation libraries

Candidates considered: `p-queue`, RxJS `Subject` + `takeUntil`, Web Streams
`ReadableStream`/`WritableStream` with `cancel()`, `AbortController`-based
hand patterns (already in use).

**Pros**
- RxJS `Subject`+`takeUntil` and Web Streams both have well-defined,
  battle-tested cancellation semantics (`unsubscribe()` / `cancel()` reliably
  stop delivery).
- Web Streams `WritableStream` maps almost 1:1 onto what `MessageQueue` does
  (a queue of writes consumed by one reader), and is a web platform standard,
  not a dependency.

**Cons**
- None of `p-queue`, RxJS, or Web Streams are in `web-app/package.json`
  today — every option here is a **new dependency** (Web Streams the API is
  built into the browser/Node runtime, but adopting its idioms still means
  rewriting the ConnectRPC bridging code and its tests).
- ConnectRPC's `streamTerminal()` call wants a plain `AsyncIterable`, not an
  RxJS `Observable` or a `ReadableStream` — every option needs an adapter
  layer back to `AsyncIterable`, which reintroduces exactly the kind of
  hand-rolled bridging code we're trying to eliminate, just wrapped around a
  bigger dependency.
- None of these libraries know about "ConnectRPC stream generation N was
  superseded by generation N+1" — that domain concept (this session's
  *current* connection attempt) has to be modeled explicitly regardless of
  the underlying queue primitive. Swapping the queue implementation doesn't
  remove the need to write the epoch-guard logic; it just moves where it's
  written.
- `p-queue` is for concurrency-limited task scheduling (running N jobs with
  a max-concurrency cap) — it does not model "cancel everything belonging to
  a stale generation" at all; would need to be paired with manual
  bookkeeping anyway.

**Verdict: Not recommended.** The bug is not "the queue data structure is
unreliable," it's "nothing tags queued/in-flight items with a connection
identity." No queue library ships that concept for free, and every option
adds a dependency plus an `AsyncIterable` adapter shim for a net decrease in
simplicity.

## Option 2 — SaaS / managed resilient-WebSocket libraries

Candidates: `reconnecting-websocket`, Socket.IO (with its ack/replay
buffer), managed "exactly-once WebSocket" services.

**Pros**
- Socket.IO's manager does have a real "offline buffer + ack" concept
  that's conceptually close to what's being asked for.

**Cons**
- These operate **below** ConnectRPC's transport abstraction (raw socket
  reconnect/backoff), while the actual transport in this repo,
  `createWebsocketBasedTransport` (`web-app/src/lib/transport/websocket-transport.ts`),
  implements the ConnectRPC streaming protocol on top of `it-ws` — envelope
  framing, `EndStream` handling, Connect-specific headers. Adopting
  `reconnecting-websocket` or Socket.IO means replacing this whole transport,
  not augmenting it: Socket.IO has its own wire protocol and its own framing,
  incompatible with ConnectRPC's envelope format and the Go server's
  `streamViaControlMode`/`streamViaTmuxCapturePane` handlers that speak raw
  Connect-over-WebSocket.
- Reconnection in this app is **already handled** one layer up — by
  `useTerminalStream`'s own `connect()`/`disconnect()`/state machine — and
  reconnect policy is coupled to app-specific concerns (tmux session
  existence, control-mode vs. capture-pane routing, resize nudges,
  scrollback replay) that a generic reconnecting-socket library knows
  nothing about.
- No hosted/SaaS "exactly-once WebSocket" offering fits an on-prem,
  single-binary Go server talking ConnectRPC to a Next.js SPA; this would
  mean routing through a third-party relay for terminal keystrokes, which is
  both a latency and a trust boundary problem for a tool that pipes input to
  an agent's shell.

**Verdict: Not recommended.** Wrong layer (raw socket vs. ConnectRPC
protocol) and no drop-in candidate actually models this app's reconnect
semantics (tmux-aware resync, scrollback, SSP negotiation) that already live
in `useTerminalStream`/`useTerminalFlowControl`.

## Option 3 — Bespoke epoch/generation guard vs. "battle-tested library" concern

The fix shape needed: (a) client — a monotonically increasing generation
number bumped on every `connect()`, captured by the in-flight message loop
and by `MessageQueue`, so a superseded generation's queue is inert (won't
deliver into a new stream, won't accept stale pushes); (b) server — thread
`context.Context` cancellation through `SendInputViaControlMode` so a
slow send from a dying connection can't land after a new connection's send
for the same session.

This is a **guard on identity of the current attempt**, not an algorithm.
There's no concurrent-consensus problem (no need for CRDT-style
conflict resolution — inputs are strictly ordered per browser tab, and
multi-tab concurrency is explicitly a non-goal), no need for causal/vector
clocks, no distributed agreement. The correctness property is simple and
locally checkable: "a message tagged with generation G is only delivered if
G equals the current generation at delivery time," which is a single integer
compare guarded by whatever lock/ref discipline already protects the queue.

**Pros of bespoke fix**
- ~10–15 line diff on `MessageQueue` (add a `generation` field, bump on
  `close()`/reset, compare-and-drop in `push()`/the iterator) plus a
  generation capture in `useTerminalStream.connect()` and a `context.Context`
  per-connection on the Go side — all fully within the existing test
  surfaces (`MessageQueue.test.ts`, `useTerminalStream.test.ts`,
  `connectrpc_websocket_test.go`).
- Directly testable with the exact regression scenario in the requirements
  (Goal 4): fire `connect()`, queue an input, force a "stopped" transition,
  `connect()` again, assert exactly one delivery — no library API to
  understand or mock.
- Zero new dependencies, zero new supply-chain surface, zero bundle-size
  cost (relevant: `web-app/package.json` has a `size-limit` budget on the JS
  bundle).

**Cons of bespoke fix**
- Discipline required to apply the guard at *every* mutation point
  (push, iterate, close) — a missed spot reintroduces the bug. Mitigated by
  keeping the guard colocated in the ~50-line class and covering it with the
  Jest regression test the requirements already mandate.

**Verdict: Recommended.** The "prefer battle-tested library" heuristic is
for algorithmically hard problems (consensus, CRDTs, cryptography, complex
scheduling). An epoch/generation guard is a textbook simple-enough-to-hand-
roll pattern, and — as shown below — this exact pattern is already
established and reviewed in this codebase.

## Option 4 — Fork/adapt an existing in-repo pattern

Searched `session/` (Go) and `web-app/src/lib/` for "epoch"/"generation"
guards on superseded async work. Two relevant precedents found:

1. **`web-app/src/lib/hooks/usePathCompletions.ts`** (lines ~83, 107, 122,
   152, 168) — this is the closest and most directly reusable precedent.
   It documents itself explicitly as "Generation counter – discards
   responses that arrive after a newer request fired":
   ```ts
   const generationRef = useRef(0);
   // ...
   const generation = ++generationRef.current;
   // ... later, after an await:
   if (generation !== generationRef.current) return;  // stale response, drop it
   ```
   This is precisely the "epoch guard on a superseded async loop" pattern
   the requirements ask to check for, already written, already reviewed, and
   already idiomatic for this codebase's React hooks. It combines with
   `AbortController` (also already used there) exactly as proposed for the
   fix.

2. **`useTerminalFlowControl.ts`'s `isResyncingRef`/`waitingForPaneResponseRef`**
   — a *boolean* (not counter) guard preventing duplicate resync requests
   while one is in flight (`if (!isResyncingRef.current) { requestFullResync(...) }`).
   Useful precedent for "don't double-fire," but it's a single-flight lock,
   not a generation/epoch counter — it doesn't retroactively invalidate
   already-queued work the way `usePathCompletions`'s counter does, so it's
   a weaker template for this bug (which needs to invalidate a whole
   *superseded connection's* queued messages, not just prevent re-entrancy).

No equivalent Go-side pattern (context-scoped generation guard) was found in
`session/`; the closest existing idiom is the per-connection `doneChan`
already used in `streamViaControlMode`/`streamViaTmuxCapturePane` to scope
goroutines to one WebSocket connection — the fix should extend that same
per-connection `context.Context` into `SendInputViaControlMode` rather than
inventing a new mechanism.

**Verdict: Recommended — copy, don't re-derive.** `usePathCompletions.ts`'s
`generationRef` pattern is the template to lift directly into
`MessageQueue`/`useTerminalStream`: bump a generation ref in `connect()`,
capture it in the closure that owns the current `MessageQueue`, and drop
any push/yield whose generation doesn't match current. This is lower-risk
than designing a new guard from scratch because it's already passed review
in this repo and its failure mode (stale-response drop) is exactly
isomorphic to this bug's failure mode (stale-message replay).

## Final Recommendation

**Do not add a new dependency.** Apply a targeted generation/epoch counter
directly to `MessageQueue.ts` (and thread the generation through
`useTerminalStream.connect()`/`disconnect()`), modeled on the existing
`generationRef` pattern in `usePathCompletions.ts`. Pair it with a Go-side
`context.Context` cancellation guard scoped per-WebSocket-connection in
`streamViaControlMode`'s input-handling goroutine, using the same
`doneChan`-per-connection scoping already present in that function, extended
into `SendInputViaControlMode`.

Concretely:
- `MessageQueue`: add a `generation: number` set at construction (or a
  `push(msg, generation)` overload); reject `push()` calls tagged with a
  stale generation instead of silently queueing them, and make `close()`
  bump-and-invalidate so any promise still parked in `this.resolve` resolves
  to the sentinel rather than a real message from a queue nobody should be
  reading anymore.
- `useTerminalStream`: bump a `connectionGenerationRef` in `connect()`
  before creating the new `MessageQueue`; the detached read-loop and any
  deferred/chunked `sendInput` chunk-sender in `useTerminalFlowControl.ts`
  (`sendChunk`, which already has a similar guard via
  `sessionIdAtStart !== sessionId`) should compare against the captured
  generation before delivering.
- Go: pass a per-connection `context.Context` (derived from the WS
  handler's lifetime, cancelled when `doneChan` closes) into
  `SendInputViaControlMode` so an in-flight send from a superseded
  connection can't land after a newer connection has taken over the same
  session/tmux pane.

Rejecting new dependencies here is not merely "avoid the small win" —
introducing RxJS, Web Streams-as-a-library-idiom, `p-queue`, or a
reconnecting-socket wrapper would require rewriting
`createWebsocketBasedTransport`'s hand-written ConnectRPC-over-WebSocket
protocol implementation (envelope framing, `EndStream` handling) to bridge
back to `AsyncIterable`, net-adding surface area and risk to fix a bug whose
correct resolution is a same-file, ~15-line, already-precedented guard.
