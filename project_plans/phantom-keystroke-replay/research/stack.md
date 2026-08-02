# Stack Research: phantom-keystroke-replay (client-side reconnect hardening)

Scope: what existing patterns/utilities in this codebase should the fix reuse,
for (1) an epoch/generation guard in `useTerminalStream.ts`'s `connect()`,
(2) drop semantics in `MessageQueue.ts`, (3) a drop-and-signal UI badge, and
(4) a bounded read-goroutine exit test for
`server/services/connectrpc_websocket.go`.

## 1. Generation-counter / epoch guard — three existing precedents, one is a near-perfect template

This idiom already exists **three times** in the codebase, not just the one
cited in requirements.md:

- `web-app/src/lib/hooks/usePathCompletions.ts:107,122,152,168` — `useRef(0)`
  counter, incremented at request-start (`const generation =
  ++generationRef.current`), checked at every async resumption point
  (`if (generation !== generationRef.current) return;`) both on the success
  path and in the catch block. Guards a debounced fetch against
  out-of-order/stale RPC responses racing a newer keystroke's request.
- `web-app/src/lib/hooks/useWorktreeSuggestions.ts:35,45,62,67` — identical
  shape, same guard-at-every-checkpoint pattern, different data source.
- `web-app/src/lib/hooks/useSessionService.ts:185`
  (`streamGenerationRef`) — **this is the closer analog for
  `useTerminalStream.ts`**, not `usePathCompletions`, because it guards a
  *long-lived streaming reconnect loop* (`watchSessions`'s WebSocket stream),
  the same shape of problem `useTerminalStream.connect()` has. Comment: "
  Monotonically-increasing stream generation counter; checked at every await
  checkpoint." Worth reading the surrounding `watchSessions`/reconnect logic
  in that file directly when implementing, as it already solves "overlapping
  reconnect starts a second stream loop before the first's cleanup finishes"
  for the sibling Watch* stream.

**Applied to `useTerminalStream.ts`:** `connect()` (lines 162–361) already has
partial guards — `isConnectedRef`/`isConnectingRef` booleans (line 163) and a
comment at lines 102–106 explicitly naming the race ("Guards against two
independent visibility/focus-triggered reconnect paths ... both calling
connect() for the same disconnected session before either handshake
completes") — but these are booleans, not a monotonic counter, so they can't
distinguish "this callback belongs to the *current* connect() attempt" from
"this callback belongs to a *superseded* one" once a second connect() attempt
is allowed to proceed (e.g. after a fast disconnect+reconnect where the
boolean gets reset). The fix should add a `connectionGenerationRef =
useRef(0)`, incremented at the top of `connect()`, captured into a local
`const generation = ++connectionGenerationRef.current`, and checked (a) in
the `firstMessage` handling branch, (b) in the message-processing loop's
`finally` block before flipping `isConnected`/`terminalState`/scheduling a
reconnect, and (c) in `disconnect()` before it nulls out
`messageQueueRef.current`. This directly closes the "overlapping reconnects"
gap named in requirements.md.

## 2. `MessageQueue.close()` — drop semantics fix location

Confirmed root cause (already stated in requirements.md, re-verified by
direct read of `web-app/src/lib/terminal/MessageQueue.ts`):

- `close()` (lines 55–63) sets `this.closed = true` and resolves any pending
  waiter with a sentinel, but **never touches `this.queue`**.
- `[Symbol.asyncIterator]`'s loop condition (line 40) is `while
  (!this.closed || this.queue.length > 0)` — by design this drains the queue
  even after `closed` flips, which is exactly the bug: any `TerminalData`
  still sitting in `queue` at `close()` time is yielded (and therefore sent
  over the new/old stream) after the queue was supposed to be dead.
- Fix is a one-line addition to `close()`: clear `this.queue = []` (or set a
  `dropped` counter for the drop-and-signal badge/announcement to consume —
  see §3). Must happen *before* resolving any pending waiter, otherwise a
  waiter that resolves via the still-queued path could still see stale
  entries in a race (though in the current single-resolve-slot design this
  is low risk, worth being explicit in the diff).

**Existing test conflict to resolve, not just extend:**
`web-app/src/lib/terminal/__tests__/MessageQueue.test.ts` lines 34–51
(`'should yield messages in order'`) **currently asserts the buggy
behavior** — it pushes 2 messages, calls `close()`, and asserts both are
still yielded. That test will need to be rewritten (not just supplemented)
once `close()` clears the queue; per requirements.md AC3 the new behavior is
"input typed while disconnected is dropped, not queued/flushed." The other
existing tests in that file (`'push after close' → no-op`,
`'should unblock a waiting iterator'`) already assert the *target* semantics
and should keep passing unmodified.

## 3. Drop-and-signal UI — LiveRegion + badge component precedent

- `web-app/src/components/ui/LiveRegion.tsx` already exists and does exactly
  what AC3/AC4 require for "audible" signaling: a `role="status"
  aria-live={politeness} aria-atomic="true"` div, visually hidden via
  `srOnly` (see `LiveRegion.css`), plus a paired `useLiveRegion()` hook that
  exposes `{ message, announce }` — `announce()` sets the message and clears
  it after 1s so repeated identical announcements still fire (React state
  identity would otherwise suppress a second identical string). **Use
  `politeness="assertive"`** for the drop announcement — the component
  already supports it as a prop, no new primitive needed. This is a call site
  concern (wire `announce("Input dropped — reconnecting")` into whatever
  component owns the terminal + hooks `useTerminalStream`/`MessageQueue`),
  not a component to build from scratch.
- No `InputDropBadge` exists yet under `web-app/src/components/sessions/`
  (confirmed via directory listing — nearest naming neighbors are
  `GitHubBadge.tsx`, `OmnibarModeBadge.css.ts`, `MemoryNavBadge.tsx`,
  `ApprovalNavBadge.tsx`). `GitHubBadge.tsx` is the best structural template
  to copy: plain function component, props-driven visibility (`if (!hasPR &&
  !hasRepo) return null;` — i.e. renders nothing when inactive, not a
  hidden/visible toggle), a small inline SVG icon, `title` for the tooltip,
  and an explicit `aria-label` string built from state — and a paired
  `GitHubBadge.css.ts` (per ADR-009/`css-architecture.md`, new components use
  vanilla-extract `.css.ts`, not CSS Modules). `InputDropBadge` should follow
  the same shape: renders `null` when no drop has occurred/badge not active,
  otherwise a small badge with `title`/`aria-label` describing the drop, and
  its own `InputDropBadge.css.ts` pulling tokens from `vars` per
  `theme.css.ts` (see `.claude/rules/css-architecture.md` — no hardcoded hex,
  no `var(--undefined-var)` strings).

## 4. Go: bounded read-goroutine exit for `connectrpc_websocket.go`

`streamViaControlMode` (`server/services/connectrpc_websocket.go:540–1097`)
runs two goroutines coordinated by `errChan := make(chan error, 2)` and
`doneChan := make(chan struct{})` (lines 746–747):

- **Goroutine 1** (output forwarder, starts line 752) — `defer
  close(doneChan)`.
- **Goroutine 2** (the read goroutine in scope for this gap, starts line 948)
  — blocks in `stream.conn.ReadMessage()` (line 954) inside a `for { select
  { case <-doneChan: return; default: ... } }` loop. Note the `select`
  against `doneChan` only takes effect *between* blocking reads — while
  actually parked in `ReadMessage()`, goroutine 2 cannot observe `doneChan`
  closing at all; it only exits when `ReadMessage()` itself returns (an
  error, e.g. from the socket closing).
- The function returns via `select { case err := <-errChan: return err; case
  <-doneChan: return nil }` (lines 1091–1096) as soon as **either** goroutine
  signals — it does **not** wait for goroutine 2 to have actually exited when
  goroutine 1 wins the race (e.g. output-side error/close while goroutine 2
  is still parked in `ReadMessage()`). The caller, `HandleWebSocket` (line
  341), has `defer conn.Close()` (line 348) which is what eventually
  unblocks `ReadMessage()` — but only once `HandleWebSocket`'s own function
  body returns, which is not synchronized with goroutine 2's actual exit.
  Net effect: goroutine 2's lifetime is not bounded by
  `streamViaControlMode`'s return, i.e. exactly the "unbounded read-goroutine
  exit" gap named in requirements.md.

**Existing precedent for the fix, already in this codebase** — do not invent
a new idiom:

- `server/services/session_service.go:2542` — `waitWithTimeout(wg
  *sync.WaitGroup, timeout time.Duration) bool`: spins a small goroutine that
  does `wg.Wait(); close(done)`, then `select`s that against
  `time.After(timeout)`, returning `false` (not blocking forever) if the
  timeout elapses. This is the exact "bounded" part of "bounded read-goroutine
  exit."
  Tested directly and cheaply, with no tmux dependency, in
  `server/services/session_service_stream_terminal_test.go:179–192`
  (`TestWaitWithTimeout`) — two subtests, one where `wg.Done()` fires (expect
  `true`) and one where it deliberately never fires (expect `false` within
  a short timeout). **This is the template for the new
  connectrpc_websocket.go test**: no live tmux session needed, just a
  `sync.WaitGroup` stood in for goroutine 2's completion signal.
- `server/services/session_service.go:2569` — `logSlowShutdown(wg
  *sync.WaitGroup, warnAfter time.Duration, sessionID, reason string)`: the
  unconditional-wait sibling of `waitWithTimeout`, used where giving up isn't
  safe (StreamTerminal's two goroutines both still call `stream.Send()` /
  `stream.Receive()` after return, so returning early risks racing
  connect-go's end-of-stream trailer write). Its doc comment (lines
  2556–2568) is directly relevant background reading: it explains *why*
  `streamViaControlMode`-style code can't just `return` the moment one
  goroutine finishes.
- `server/services/connectrpc_websocket_test.go:513`
  (`TestSnapshotCacheConcurrentAccess`) shows this file's existing
  `sync.WaitGroup` test convention (`wg.Add(2)` per iteration pair, `go
  func(id string) { defer wg.Done(); ... }(id)`, single `wg.Wait()` at the
  end) — match this style for the new test rather than introducing a
  different one.

**Recommended shape for the new test:** add a `sync.WaitGroup` (or reuse
`errChan`/`doneChan` plumbing) around goroutine 2's exit inside
`streamViaControlMode`, incrementing before the `go func() { ... }()` and
`Done()` in that goroutine's own deferred cleanup, then have
`streamViaControlMode` call something like `waitWithTimeout(&wg,
<bound>)` before it returns, logging (not silently swallowing) a
timeout via the `logSlowShutdown`-style pattern if goroutine 2 doesn't exit
in time. The regression test itself does not need a real tmux session or
websocket — it can construct the minimal goroutine-coordination scaffold
directly (mirroring `TestWaitWithTimeout`) and assert goroutine 2 has exited
(via the WaitGroup) within a bound after `doneChan`/`errChan` fires, using a
fake/mock connection whose `ReadMessage()` can be made to return on demand
(check whether `connectWebSocketStream`/`stream.conn` in this file already
has a mockable interface before writing a new one — a quick
`grep -n "type connectWebSocketStream\|conn  *\*websocket.Conn\|conn  *websocketConn" server/services/connectrpc_websocket.go`
during implementation will confirm whether `conn` is already an interface or
needs one introduced for testability).

## Summary of concrete file targets

| Concern | File | Existing pattern to reuse |
|---|---|---|
| Reconnect epoch guard | `web-app/src/lib/hooks/useTerminalStream.ts` (`connect()`, lines 162–361) | `useSessionService.ts`'s `streamGenerationRef` (closer analog than `usePathCompletions.ts`); also `usePathCompletions.ts`/`useWorktreeSuggestions.ts` for the generic idiom |
| Drop queued input on close | `web-app/src/lib/terminal/MessageQueue.ts` (`close()`, lines 55–63) | N/A — first instance of this exact fix; must also rewrite the conflicting assertion in `MessageQueue.test.ts:34-51` |
| Assertive announcement | wherever `useTerminalStream`/`MessageQueue` is consumed | `web-app/src/components/ui/LiveRegion.tsx` — use existing `useLiveRegion()` + `politeness="assertive"`, no new primitive |
| Drop badge component | new: `web-app/src/components/sessions/InputDropBadge.tsx` + `.css.ts` | Structural template: `GitHubBadge.tsx` (render-null-when-inactive, `title`+`aria-label`, vanilla-extract per ADR-009) |
| Bounded read-goroutine test | `server/services/connectrpc_websocket.go` (goroutine 2, lines 948–1086) + new test in `connectrpc_websocket_test.go` | `waitWithTimeout`/`logSlowShutdown` (`session_service.go:2542,2569`) and their existing test `TestWaitWithTimeout` (`session_service_stream_terminal_test.go:179`) as the direct template; match `sync.WaitGroup` test style already used in `connectrpc_websocket_test.go:513` |
