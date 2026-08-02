# Research: Feature Landscape — Client Reconnect Hardening (Agent 2)

Scope: existing reconnect/dedup/drop patterns in this codebase that the
client-side fix (MessageQueue, useTerminalStream, drop-and-signal UI, Go
read-goroutine test) should reuse rather than reinvent; edge cases the design
must handle; unstated user needs beyond the literal ACs.

## 1. Existing patterns to reuse (do not invent new idioms)

### 1.1 Generation/epoch counter — direct precedent, same file family

`web-app/src/lib/hooks/useSessionService.ts` (`WatchSessions` stream) already
solves the *exact* "overlapping reconnect" problem AC4 requires for
`useTerminalStream.ts`, and ADR-023 explicitly named this gap for the
terminal stream:

- `streamGenerationRef = useRef(0)` (line 185).
- Every (re)start does `++streamGenerationRef.current` *before* launching
  `startStream()`, then captures `const myGeneration = ++streamGenerationRef.current`
  inside the async function itself (lines 829, 833) — two increments: one to
  invalidate any in-flight call from a *prior* `watchSessions()` invocation,
  one to mint this call's own generation id.
- Every await-resumption point (`await listSessions(...)`, each loop
  iteration implicitly via the stream, and post-backoff-sleep) is followed by
  `if (!shouldReconnectRef.current || streamGenerationRef.current !== myGeneration) return;`
  (lines 844, 869, 880, 891, 914, 925, 936) — this is the guard that must be
  replicated in `useTerminalStream.ts`'s `connect()`, which currently has
  *no* generation check inside the message-processing IIFE (only a
  pre-entry `isConnectingRef` guard at line 163 that prevents *starting* a
  second `connect()` call while one is in flight, but does nothing once a
  stream's for-await loop is already running — a second connect() that
  starts after the first's `isConnectingRef.current = false` reset, e.g.
  from the visibility/online listener firing while the old stream's
  `finally` block is still unwinding, is not detected).
- ADR-023 (`docs/adr/ADR-023-client-reconnect-browser-lifecycle.md`) recorded
  this as a known "stale closure / dual-stream race" risk and applied the
  generation-ref fix to the watch stream (item 3 of the "What Approach B
  does" list) but the terminal stream's Phase 3 item (line 57-61 of the ADR)
  never got the same treatment — confirms this is a known, previously-scoped
  gap, not a new invention.

### 1.2 Generation/epoch counter — second precedent (fetch/debounce)

`web-app/src/lib/hooks/usePathCompletions.ts` uses the same shape for a
simpler case: `generationRef = useRef(0)`, `const generation = ++generationRef.current`
at effect-start, then `if (generation !== generationRef.current) return;`
after both the cache-hit path and the awaited RPC response (lines 122, 152,
168). Combined with an `AbortController` for the in-flight request. This is
the "three-layer protection" model documented in its own docstring
(debounce → abort → generation) — a good template for the drop badge's
question of *when* to fire (should not fire on a superseded-but-successful
reconnect, only on an actual drop).

### 1.3 Backoff / retriable-close-code utility

`web-app/src/lib/utils/backoff.ts` (`BackoffState`, `isRetriableCloseCode`,
`getWsCloseCode`) is already shared between `useSessionService.ts` and
`useTerminalStream.ts`. No new backoff logic should be added; the epoch
guard is orthogonal to backoff and composes with the existing
`terminalBackoffRef` / `shouldReconnectRef` state already in
`useTerminalStream.ts`.

### 1.4 "One-shot, cannot be replayed" queue design is intentional, not a bug to route around

ADR-023's "Rejected approaches" section (Approach A) explicitly documents
*why* `MessageQueue` is one-shot per connection: "each terminal reconnect
must construct a fresh `MessageQueue` and re-run the full handshake." This
confirms the correct fix for the replay bug is **not** to make `MessageQueue`
reusable/long-lived across reconnects — `useTerminalStream.ts`'s `connect()`
already constructs a fresh instance per call (line 185: `messageQueueRef.current = new MessageQueue()`).
The bug is narrower: `close()` on the *old* instance must guarantee nothing
still sitting in its buffer is yielded, not that instances be shared.

### 1.5 Drop-and-signal UI precedent

No `InputDropBadge` exists, but `MemoryPressureCallout.tsx`
(`web-app/src/components/sessions/MemoryPressureCallout.tsx`) is the closest
existing "transient, dismissible, screen-reader-announced status" component
in `components/sessions/`:
- `role="alert" aria-live="polite"` on the container (line 66) — note: for
  AC3's *assertive* announcement requirement, the new badge should use
  `aria-live="assertive"` explicitly (or rely on `role="alert"`'s implicit
  assertive semantics and *not* override it with `aria-live="polite"` the
  way this component does — this component's polite override is likely
  intentional for a lower-urgency case and should not be copied verbatim).
- Per-item dismissal persisted to `sessionStorage`, `useState<Set<string>>`.
- Conditional `null` return when nothing to show, rather than a
  mount/unmount-driven visibility prop — matches React idiom used elsewhere
  in this component directory.
- `SessionCard.tsx` also has a visually-hidden `role="status" aria-live="polite"`
  live-region pattern (line 792) using the clip-path screen-reader-only CSS
  trick, useful if the badge itself is visual but needs a separate
  screen-reader-only announcement node (common a11y pattern when the visible
  badge shouldn't be `aria-live` directly, e.g. to avoid re-announcing on
  every re-render).

### 1.6 Go side: no existing bounded-goroutine-exit test in this file; nearest patterns

`server/services/connectrpc_websocket.go`'s `streamViaControlMode` (the
handler this bug's server-side fix already touched) spawns goroutines tied
to a `doneChan`/`errChan` pair (lines 745-858, 865+, 948+) — `doneChan` is
closed via `defer close(doneChan)` in the output-forwarding goroutine, and
other goroutines `select` on `<-doneChan` to exit. This is the idiom AC4's
"Go bounded read-goroutine exit test" should assert against: on
stream-close/reconnect, the read-goroutine (client→server input forwarding)
must observably exit within a bound, not leak.
`connectrpc_websocket_test.go` has no existing goroutine-leak/bounded-exit
test (checked: no `goleak`, no `runtime.NumGoroutine` usage in that file).
Precedent for that style of test *does* exist elsewhere in the repo (grep
hits on `sync.WaitGroup` / `runtime.NumGoroutine` in `pkg/events/bus_test.go`,
`log/async_handler_test.go`, `executor/circuit_breaker_test.go`,
`session/actor_test.go`) — worth sampling one of those for the assertion
style (WaitGroup + timeout channel, or explicit done-channel receive with
`select`/`t.Fatal` on timeout) rather than reaching for `goleak` fresh (not a
current dependency).

## 2. Edge cases the design must handle

1. **Reconnect mid-send**: `sendInput` (in `useTerminalFlowControl.ts`)
   already gates on `pushMessageRef.current && isConnectedRef.current`
   (checked at call time, e.g. line 143), and `pushMessageRef.current`
   dereferences `messageQueueRef.current` fresh on every call (not a stale
   closure) — so a keystroke typed *during* the brief window between
   `disconnect()` starting and `isConnectedRef.current` flipping false is
   the actual risk surface. `isConnectedRef.current` is set false in
   `connect()`'s `finally` block (line 323) *before* the `setIsConnected(false)`
   state setter, specifically commented "sync ref before state setter to
   prevent reconnect guard race" — but `disconnect()` (line 371) does not
   itself synchronously flip `isConnectedRef.current` before awaiting the
   graceful-close promise; it only sets `isConnected` state at line 409
   after the up-to-1s wait. Confirm whether a keystroke fired in that window
   still passes the `isConnectedRef.current` check in `sendInput` — if so,
   it gets pushed to a queue about to be closed, which is exactly the
   `MessageQueue.close()`-with-buffered-items bug path.

2. **Rapid triple-reconnect** (explicit AC4 requirement): three overlapping
   `connect()` calls (e.g. visibility flapping fast) must not produce three
   live message-processing loops or three live queues. The epoch guard
   (1.1) makes stale-generation loops early-return at their next
   await-resumption point, but the *time until* that resumption point (e.g.
   mid `for await` on a slow/absent stream) means a stale loop can still be
   "live" (holding an open `MessageQueue`/`AbortController`) for a window —
   must decide whether triple-reconnect should also proactively `abort()`
   /`close()` superseded generations rather than relying solely on them
   self-detecting staleness at their next checkpoint.

3. **Input queued but never flushed before a second reconnect supersedes the
   first** (explicit AC3/AC4 requirement): if generation N's `MessageQueue`
   has buffered input when generation N+1 starts, that buffered input must
   be dropped (with badge/announcement), not silently carried into or
   flushed onto generation N+1's queue. This means the fix must NOT copy
   pending queue contents into the new `MessageQueue` on reconnect — the
   fresh-instance-per-connect design (1.4) already avoids this by
   construction as long as no code path manually migrates queue state.

4. **Tab backgrounded during reconnect**: `visibilitychange`/`online`
   handlers in `useTerminalStream.ts` (lines 432-458) debounce 200ms and
   call `connectRef.current()`. If the tab goes background *again*
   mid-reconnect (e.g. quick alt-tab), this can itself trigger the
   triple-reconnect case (2) via the same debounced handler firing twice in
   quick succession around a visibility flap — same guard applies, but
   worth an explicit test since it's a distinct *trigger* even if the
   *mechanism* is shared with case 2.

5. **Drop signaled but connection then succeeds on the very next attempt**:
   distinguish "input dropped because connection superseded/closed" from
   "briefly disconnected, reconnected, no data lost" — the badge must not
   fire for every backoff retry cycle, only when there was actually
   buffered-but-undelivered input at the moment of drop. This bounds when
   the badge condition should even be evaluated (only meaningful at
   `MessageQueue.close()` if `queue.length > 0` at that instant, not on
   every `disconnect()`/`connect()` pair).

## 3. Unstated user needs beyond the literal ACs

- **Badge auto-dismiss**: `MemoryPressureCallout.tsx`'s persistent/manual
  dismiss pattern is wrong for this case — a transient one-off drop event,
  not an ongoing condition. The badge should very likely auto-dismiss after
  a few seconds (toast-like), not require manual dismissal or persist across
  reloads via `sessionStorage` the way the memory callout does. No existing
  "toast" component was found in `components/sessions/` in this pass —
  worth Agent-1/Agent-3 confirming whether a toast/snackbar primitive exists
  elsewhere in `web-app/src/components/` before building a bespoke
  auto-dismiss timer inline in the new component.
- **Recoverability of dropped input**: the literal AC only requires the
  drop be *signaled*, not that the dropped text be recoverable/re-typeable.
  Given the whole point of this bug report was a corrupted/unusable session,
  users will likely want the dropped characters echoed back in the badge
  (e.g. "3 characters not sent: `ls -la`") so they know what to retype,
  rather than just "some input was dropped." This is a natural extension of
  AC3's requirement and low-cost to add (the dropped `TerminalData[]` is
  already sitting in `MessageQueue`'s `queue` array at drop time) — worth
  flagging to Agent-1 (UX) as a candidate enhancement over the bare minimum.
- **Distinguish input-drop from output-loss**: users conflate "my keystroke
  didn't work" with "the terminal looks broken" (the original ticket's
  framing). The badge copy should be unambiguous that *input* was dropped
  (not that output/scrollback was lost), since `useTerminalStream.ts`
  separately handles scrollback/resync (`requestFullResync`,
  `markResyncComplete`) for the output side — conflating the two in the UI
  would misdirect users during debugging.
- **Non-flapping case must stay silent**: Constraints section already
  requires not regressing SSP UX for the normal case; concretely this means
  the badge/announcement must never fire on an ordinary clean disconnect
  with no pending input (e.g. idle tab, no keystrokes in flight) — the
  guard in edge case 5 above is what prevents this, and should be a named
  Jest test ("no badge on clean disconnect with empty queue"), not just
  implied by the drop-detection logic.

## 4. Files referenced

- `web-app/src/lib/terminal/MessageQueue.ts` (bug: close() doesn't clear buffered queue)
- `web-app/src/lib/hooks/useTerminalStream.ts` (no epoch guard in message-processing loop; `connect()`/`disconnect()`)
- `web-app/src/lib/hooks/useTerminalFlowControl.ts` (`sendInput`, `pushMessageRef`, `isConnectedRef` gating)
- `web-app/src/lib/hooks/useSessionService.ts` (canonical `streamGenerationRef` pattern, lines 185, 829-938)
- `web-app/src/lib/hooks/usePathCompletions.ts` (secondary generation-ref precedent, debounce+abort+generation)
- `web-app/src/lib/utils/backoff.ts` (shared `BackoffState`, close-code helpers)
- `web-app/src/components/sessions/MemoryPressureCallout.tsx` (nearest existing alert/badge component; dismiss pattern to *not* copy verbatim)
- `web-app/src/components/sessions/SessionCard.tsx` (visually-hidden `aria-live` live-region pattern, line 792)
- `docs/adr/ADR-023-client-reconnect-browser-lifecycle.md` (documents the terminal-stream generation-guard gap as already-known/scoped, and the "MessageQueue is intentionally one-shot" rationale)
- `server/services/connectrpc_websocket.go` (`streamViaControlMode`, `doneChan`/`errChan` goroutine-coordination idiom for the Go bounded-exit test)
- `server/services/connectrpc_websocket_test.go`, `server/services/session_service_stream_terminal_test.go` (existing test file/naming conventions; no goroutine-leak test present yet)
- Existing test files to extend (not replace): `web-app/src/lib/terminal/__tests__/MessageQueue.test.ts`, `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` (already uses `functionName_should_effect_When_condition` naming for reconnect-related tests, e.g. `connect_should_scheduleReconnectOnlyOnce_When_retriableErrorThrown`)
