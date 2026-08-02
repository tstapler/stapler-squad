# Architecture Research: Client-Side Reconnect Hardening (phantom-keystroke-replay)

Scope note: server-side root cause is already fixed on `main` (3546c2b12,
c0e6c4ce6). This doc covers only the remaining client-side gap — bounded-scope
bug fix, not a redesign. No Event-Command-Policy table (not a multi-actor
domain).

## 1. Does `usePathCompletions.ts`'s generation-counter idiom transfer directly?

No — same primitive, different shape, needs its own variant.

**What `usePathCompletions.ts` actually guards** (`web-app/src/lib/hooks/usePathCompletions.ts:107,122,152,168`):
a *single-shot request/response race*. `generationRef` is incremented once per
effect run; the async work is a single `await client.listPathCompletions(...)`;
the guard fires exactly once, right before `setResult(...)`, to answer "is this
response still the one I care about?" It never touches the *request itself* —
an in-flight stale request is left to finish and its result is simply thrown
away. `AbortController` is *also* present but is only used for cleanup-on-unmount,
not for making the generation check redundant.

**What `useTerminalStream.ts`'s `connect()` needs to guard** is structurally
different: a *long-lived bidirectional stream loop*, not a one-shot fetch. Three
distinct checkpoints need protecting, not one:

1. **Entry guard** (already exists, but incomplete) — `isConnectedRef` /
   `isConnectingRef` at the top of `connect()` (line 163) prevent *starting* a
   second stream while one is believed active. This part is fine as-is.
2. **Iteration guard** (missing) — the `for await (const msg of stream)` loop
   (line 220) and its `finally` block (line 322) have no way to tell "am I
   still the current attempt?" If `connect()` re-enters before a prior attempt's
   `finally` has run (e.g. two overlapping calls from the visibility listener
   and a manual reconnect both slipping past the entry guard in the same tick,
   or a `finally` from attempt N racing a fresh `connect()` for attempt N+1
   triggered by its own scheduled reconnect timer), **both** async IIFEs are
   alive concurrently, both mutate the same `isConnected`/`terminalState`
   React state and the same `shouldReconnectRef`/`terminalBackoffRef`, and
   whichever `finally` runs last "wins" regardless of which attempt is
   actually current — this is exactly the double-checked-locking hazard this
   repo already has a rule for (`.claude/rules/go-double-checked-locking.md`,
   Go-specific but the same shape applies here: don't let a stale computation
   overwrite state a newer computation already owns).
3. **Resource-lifetime guard** (missing) — `messageQueueRef.current` is
   unconditionally overwritten with `new MessageQueue()` at the top of every
   `connect()` (line 185). On the explicit `disconnect()` path the old queue
   is `.close()`'d first (line 388). On the *implicit* reconnect path (stream
   ends → `finally` → scheduled `connectRef.current()`), nothing closes the
   old queue before it's replaced — it's just dereferenced and left for GC.
   Today that's inert (the old `stream` object is dead so nothing drains the
   orphaned queue), but it means "queue lifetime" and "connection epoch" are
   not currently the same concept, which is precisely what the fix needs them
   to become.

**Recommended pattern**: a `connectionEpochRef` (`useRef(0)`), incremented once
at the very top of `connect()`, with the resulting local `epoch` value closed
over by the async IIFE. Check `epoch === connectionEpochRef.current` at three
points, mirroring the three checkpoints above:
- Immediately after `firstMessage` handling / before any `setState` inside the
  `for await` loop body — if stale, `break` out of the loop (which triggers
  the `finally`, but the `finally` itself must also check the epoch before
  doing anything user-visible).
- At the top of the `finally` block — if `epoch !== connectionEpochRef.current`,
  skip `setIsConnected`/`setTerminalState`/reconnect-scheduling entirely (a
  newer attempt already owns that state) but still perform the *queue*
  cleanup for this specific epoch's `MessageQueue`/`AbortController` (resource
  cleanup must happen regardless of which epoch is "current"; only
  *user-visible state mutation* is gated).
- Inside `MessageQueue` itself is *not* the place to check the epoch — the
  queue shouldn't need to know about React's connection-epoch concept. Instead,
  make queue *closure* epoch-scoped by having `connect()` close the
  previous `messageQueueRef.current` (if any) before installing the new one,
  unconditionally — collapsing the disconnect-path/reconnect-path asymmetry
  described above into one code path. Combined with the `MessageQueue.close()`
  fix in §2, this guarantees a queue is never written to once it's no longer
  `messageQueueRef.current`.

This is the same *mechanism* as `usePathCompletities.ts` (monotonic ref,
compare-before-act) but a different *idiom*: "connection epoch gating a
stateful loop + its owned resources" rather than "request epoch gating a
single `setState` call." Reusing the exact one-shot form would under-protect
(only guards the `setIsConnected(true)` on first message, not the ongoing loop
body, not the `finally`, not the queue swap).

## 2. Integration points: MessageQueue ↔ useTerminalStream ↔ InputDropBadge

Current data flow (confirmed by reading, not assumed):

```
xterm.js onData (TerminalOutput.tsx:1695, handleTerminalData)
  → sendInput(data)                                  [useTerminalStream.ts:473, re-exports flowControl.sendInput]
    → useTerminalFlowControl.sendInput                [useTerminalFlowControl.ts:142]
      → pushMessage(msg) → pushMessageRef.current(msg) [useTerminalFlowControl.ts:150,74]
        → messageQueueRef.current?.push(msg)          [useTerminalStream.ts:139, pushMessageRef effect]
          → MessageQueue.push()                       [MessageQueue.ts:28]
            → queue.push(msg)  OR  resolve(msg) directly if the async iterator is currently awaiting
```

`pushMessageRef.current` is a closure that reads `messageQueueRef.current`
*fresh on every call* (not captured at effect-setup time), so new input from
the user always reaches whatever queue is currently installed — that part is
already correct and doesn't need to change.

**Who decides a drop happened**: today, nobody — `push()` silently no-ops
once `closed` (line 29), and even before that fix, `close()` neither clears
nor reports what was discarded. `MessageQueue` is the only component with
authoritative knowledge of *what* was buffered at the moment of close (the
`queue` array's contents/length) — it's the natural owner of "did anything get
dropped, and how much." Recommend `close()` return the number of dropped
messages (or accept an `onDrop?: (droppedCount: number) => void` callback),
rather than pushing this decision up into `useTerminalStream` where the queue's
internal buffer state isn't visible.

**Who owns the "closed" flag**: stays `MessageQueue` — it's already the single
source of truth (`private closed = false`) and nothing else should duplicate
it. What changes is *when* `close()` is called: currently only from the
explicit `disconnect()` path (line 388); per §1, it should also be called
unconditionally at the top of `connect()` before the ref is overwritten, so
every queue transition — user-initiated disconnect or implicit reconnect
supersession — goes through the same "close (report drops) → discard" step.

**Who triggers the announcement**: `useTerminalStream`, not `MessageQueue` and
not a component. `MessageQueue` has no React state and shouldn't grow any (it's
a plain class, appropriately — see `.claude/rules/interface-pollution-checklist.md`
smell #1/#4, no reason to wrap it in a service/manager layer for this). The
hook is the right layer: it already owns `setError`/`setIsConnected`, is the
place `onDrop` would be wired up (`messageQueueRef.current.close(...)` call
sites live here), and is the only layer with access to both the drop event and
React state a component can subscribe to. Recommend surfacing a small piece of
state — e.g. `droppedInputEvent: { count: number; at: number } | null` (a
counter/timestamp pair, not just a boolean, so a component `useEffect` keyed
on `at` can re-fire the assertive announcement even if a second drop happens
before the first badge auto-dismisses) — returned from the hook alongside the
existing `isConnected`/`terminalState`/etc.

**Where the badge mounts**: `web-app/src/components/sessions/` (confirmed no
`InputDropBadge` or equivalent exists yet — `Glob` for `*InputDrop*` returned
nothing). `TerminalOutput.tsx` is the existing consumer of `useTerminalStream`
(line 456) and already renders adjacent transient UI for other terminal
states, so it's the natural mount point — pass the new `droppedInputEvent`
down as a prop the same way `error`/`isHardFailed` already flow. For the
assertive live-region shape, follow the *already-established* two-tier pattern
in this codebase rather than invent a third: `InlineNotice.tsx`
(`role="status" aria-live="polite"`, for routine non-blocking notices) vs.
`InlineError.tsx` (`role="alert" aria-live="assertive"`, for user-impacting
failures — see its own file comment at `InlineNotice.tsx:6-12` explicitly
contrasting the two). A dropped keystroke is user-impacting and requires the
assertive tier — pattern the new component directly on `InlineError.tsx`'s
`role="alert" aria-live="assertive"` usage (lines 55-56, 105-106), not
`InlineNotice`.

## 3. "At most once" consistency guarantee across a reconnect boundary

The requirements (AC2, AC3) define the target guarantee explicitly:
**at-most-once, best-effort delivery — deliberately not at-least-once, not
exactly-once.** This is the correct choice for keystrokes specifically (they
are ephemeral, latency-sensitive, and the user is present to notice and retype
a dropped one) and it's why AC3 pairs "drop, don't queue" with "signal the
user" — the guarantee is only acceptable *because* it's visible, not silent.

What "at most once" decomposes into, mapped to the two fixes above:

- **Never re-emit already-forwarded input.** Not a client concern at all for
  the *forwarding* half — once `MessageQueue.push()` has handed a message to
  the async iterator (either via immediate `resolve()` or via the `queue`
  array being drained), it's gone from the queue; there's no client-side
  buffer that replays sent-but-possibly-unacked input on reconnect (unlike,
  say, a message-broker outbox pattern — deliberately not needed here per the
  Non-Goals section). The actual re-emission risk the ticket describes was
  server-side (session_driver.go's cooldown-less poll loop), already fixed.
  The client-side echo of this same class of bug is §1's overlapping-epoch
  race: two concurrent `connect()` attempts could both end up with a live
  queue/stream pair accepting input, which — while not literally "replaying"
  a keystroke — has the same user-visible failure mode (input reaching tmux
  more than once, non-deterministically, depending which attempt's socket
  wins). The epoch guard is what forecloses this.
- **Never deliver queued-but-unsent input after supersession.** This is
  `MessageQueue.close()`'s bug directly: `close()` sets `closed = true` but
  never clears `queue` (`MessageQueue.ts:55-63`), and the iterator's loop
  condition `while (!this.closed || this.queue.length > 0)` (line 40) means
  anything still sitting in `queue` at close time is drained and yielded
  *after* `closed` flips true — i.e. sent to a stream that the caller believed
  was being torn down. Fix is minimal and local: clear `queue` inside
  `close()`. No other change to the iterator protocol is needed — `push()`
  already refuses new writes once closed (line 29), so clearing at `close()`
  time is sufficient to make "closed" mean "nothing more will ever be
  yielded," matching the doc comment's own stated intent (line 16, "Shutdown").
- **Server side needs no dedup logic of its own.** Confirmed by reading
  `streamViaControlMode` (`server/services/connectrpc_websocket.go:948-1087`,
  the read goroutine "Goroutine 2"): the server does not buffer client input
  across reconnects at all — it's fire-and-forget per received WebSocket frame
  straight to `instance.SendInputViaControlMode`/`sendInputToTmux` (lines
  1006-1013). Each read goroutine is scoped 1:1 to one physical WebSocket
  `conn` (closed via `defer conn.Close()` in `HandleWebSocket`,
  `connectrpc_websocket.go:348`) — structurally, two goroutines can never be
  reading for the *same* logical session from *two different* live
  connections simultaneously unless the client itself opens two sockets
  (which is the explicitly-out-of-scope multi-tab case). So "at most once" on
  the wire is entirely a client-side responsibility; the server's obligation
  is narrower — just don't leak the goroutine.
- **What "bounded read-goroutine exit" (AC4) is actually testing.** Traced the
  goroutine lifecycle precisely because the requirements doc flags this as
  unverified: the read goroutine (line 948) blocks on `stream.conn.ReadMessage()`
  inside `select { case <-doneChan: return; default: <blocking call> }` — note
  the `select` only *checks* `doneChan` once per loop iteration; while parked
  inside the blocking `ReadMessage()` call it is **not** interruptible via
  `doneChan` closing (that's a `default:`-branch pattern, not a `select` over
  both channels — closing `doneChan` from the output goroutine does not
  unblock a concurrent in-flight `ReadMessage()`). The goroutine's actual exit
  is bounded indirectly: `streamViaControlMode` returns via the `errChan`
  case → `streamTerminal`/`HandleWebSocket` return → `defer conn.Close()`
  fires (line 348) → the blocked `ReadMessage()` errors → the goroutine sends
  to `errChan` (buffered, size 2, so this never blocks even though nobody's
  still selecting on it) → returns. So it *is* bounded, but only because
  `conn.Close()` happens synchronously in the caller immediately after this
  function returns — there is no `context.Context` cancellation wired through
  to the blocking read itself. A regression test for AC4 should exercise
  exactly this: close the underlying (mock/fake) `conn` and assert the read
  goroutine actually terminates within a bound (e.g. a completion channel with
  a timeout, or a goroutine-count check before/after) — not assert that the
  server "dedupes" anything, since there's nothing to dedupe.

## Summary of concrete fix points (for the plan phase)

1. `MessageQueue.close()` — clear `this.queue = []` in addition to setting
   `closed = true`; optionally return/report the pre-clear length so
   `useTerminalStream` can surface a drop count.
2. `useTerminalStream.connect()` — add `connectionEpochRef`; capture `epoch`
   locally; gate the `for await` loop body and the `finally` block's
   `setState`/reconnect-scheduling on `epoch === connectionEpochRef.current`;
   unconditionally close the previous `messageQueueRef.current` (if set)
   before installing a new one, collapsing the disconnect-path/reconnect-path
   asymmetry.
3. New `InputDropBadge` component in `web-app/src/components/sessions/`,
   patterned on `InlineError.tsx`'s `role="alert" aria-live="assertive"`
   (not `InlineNotice`'s polite tier), driven by a new
   `droppedInputEvent`-shaped value returned from `useTerminalStream`.
4. Go regression test for the read goroutine in
   `server/services/connectrpc_websocket.go` asserting bounded exit on
   connection close — a goroutine-leak/timeout check, not a dedup check.
