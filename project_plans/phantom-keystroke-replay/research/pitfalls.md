# Pitfalls Research — Agent 4

Scope: client-side reconnect hardening (`MessageQueue.ts`, `useTerminalStream.ts`),
drop-and-signal UI, and Go/Jest regression tests (requirements.md AC3/AC4).

## 1. Async-iterator queue bridging a network stream — known failure modes

`MessageQueue.ts` (`web-app/src/lib/terminal/MessageQueue.ts:23-68`) is an
async-iterable pushed straight into `client.streamTerminal(queue)`. Confirmed
bug: `close()` (line 55) sets `closed = true` but never clears `queue` — the
iterator's loop condition `while (!this.closed || this.queue.length > 0)`
(line 40) means anything already buffered when `close()` fires is still
`yield`ed and therefore still sent over the wire after the caller believed
the connection was torn down.

General pitfalls for this pattern, several of which apply directly here:

- **Close doesn't mean "stop yielding," only "stop accepting new pushes."**
  Any queue-drain loop with a "drain remaining then stop" condition (as
  written here) is optimized for *graceful* shutdown (flush pending output)
  but is wrong for *abrupt* shutdown (reconnect superseding this queue) —
  those need different semantics and the single `close()` method conflates
  them. Fix must distinguish "close, flush what's queued" from "close,
  discard what's queued" — this bug needs the latter for reconnect but the
  former is presumably still wanted for normal disconnect-with-in-flight-ack
  cases (don't regress that without checking).
- **Resolve-callback races with close.** `push()` and `close()` both touch
  `this.resolve` unsynchronized (single-threaded JS, but ordering across
  microtask boundaries still matters). If a `push()` happens between
  `close()` setting `closed = true` and the iterator's next loop check, the
  pushed message goes into `queue` (line 35, since `this.resolve` was
  already nulled by `close()`) and is never drained — because `closed` is
  already `true` and no consumer is asking for more via the resolve path.
  Depending on the exact fix, "closed" must also gate `push()` doing
  anything besides get silently ignored (it already does, line 29) *and*
  the drop needs to be observable by the caller (today it's silent — no
  return value, no callback), which is exactly what AC3's badge/announcement
  requirement is compensating for.
- **The "sentinel message to unblock the iterator" trick (line 58-61) is
  fragile.** It works by exploiting the `data.case === undefined` check at
  line 48, i.e. it depends on TerminalData's default-constructed shape never
  colliding with a real message. Any refactor of the schema (e.g. a future
  `TerminalData` variant with `case: undefined` as valid state) silently
  breaks the unblock mechanism and the iterator hangs forever, leaking the
  underlying `for await` consumer. Prefer an explicit sentinel type/symbol
  over relying on schema shape, if reworking this file.
- **Queue clearing must happen atomically with the closed flag**, not as an
  afterthought — if `close()` sets `closed = true` and *then* clears
  `queue`, and something else reads `queue.length` in between (there isn't
  currently a concurrent reader in JS's single-threaded model beyond the
  iterator itself, but adding one later — e.g. a queue-depth metric — would
  reintroduce the race). Keep the invariant "closed queues are always empty"
  enforced inside `close()` itself, not by convention at call sites.

## 2. React reconnect-generation guard — off-by-one and race pitfalls

The established idiom in this repo is `usePathCompletions.ts:107,122,152,168`:
increment a `useRef` counter once per operation start, capture the value in a
local `const`, and compare `capturedValue !== ref.current` at every
resumption point after an `await`. That pattern is single-writer (one effect,
one async chain) — `useTerminalStream.ts`'s `connect()` is more hazardous
because of things `usePathCompletions` doesn't have to deal with:

- **Increment-point placement is the single most common bug.** If the
  generation is incremented *inside* the async IIFE (the `(async () => {...
  })()` message-processing loop, `useTerminalStream.ts:217-354`) rather than
  synchronously at the top of `connect()` before any `await`, two overlapping
  calls to `connect()` can both read the *same* pre-increment value before
  either reaches the increment, defeating the guard. The increment must
  happen synchronously, before the first `await`/microtask yield, in
  `connect()` itself — mirroring `usePathCompletions.ts:122`, which
  increments before the `setTimeout`-guarded async work starts.
- **Guard must cover every resumption point, not just the "happy path" one.**
  `usePathCompletions.ts` checks the generation in *both* the success branch
  (line 152) and the catch branch (line 168) — a common bug is guarding only
  the success path and letting a stale generation's error handler still run
  `setError()`/`onError?.()` against current state. `useTerminalStream.ts`'s
  message loop has multiple exit points (the `for await` loop's `catch`
  block at line 313, and the `finally` block at line 322) that mutate shared
  state (`isConnectedRef`, `setIsConnected`, `setTerminalState`,
  reconnect-scheduling via `setTimeout`) — an epoch guard added here must
  wrap **all** of them, not just message dispatch, or a stale generation's
  `finally` block can still schedule a reconnect timer for a connection
  attempt that's already been superseded.
- **Existing guards are boolean flags, not a generation counter — they don't
  compose.** `useTerminalStream.ts` already has `isConnectingRef` (line 106,
  guards two independent visibility/focus reconnect callers) and
  `shouldReconnectRef` / `isDisconnectingRef`. These are *presence* flags
  ("is a connect in progress") not *identity* flags ("which connect attempt
  is this"). A boolean can't distinguish "attempt #3 is still connecting" 
  from "attempt #4 just started" — only a monotonic counter can, which is
  exactly the gap: a reconnect can (in the flapping scenario from the
  ticket) start attempt N+1 while attempt N's `messageQueueRef`/`stream`/
  message-loop is still unwinding. Introducing a generation counter here
  needs to *replace/augment* these flags carefully — a common regression is
  leaving the old boolean flags in place with slightly different semantics,
  so the two guards disagree at the boundary and one of them wins
  unpredictably depending on scheduling.
- **`messageQueueRef.current` is a single mutable slot shared across
  reconnects.** `connect()` (line 185) does
  `messageQueueRef.current = new MessageQueue()` unconditionally. If a new
  `connect()` call replaces `messageQueueRef.current` while the *old*
  queue's async iterator is still being drained by the in-flight ConnectRPC
  call from the previous `streamTerminal()` invocation, the old iterator
  keeps running against a queue object no longer referenced by
  `messageQueueRef` — fine for the old stream's own traffic, but `sendInput`
  reads `pushMessageRef.current` (`useTerminalFlowControl.ts:74-76`), which
  itself reads `messageQueueRef.current` indirectly via the `useEffect` at
  `useTerminalStream.ts:137-142` — **that effect has an empty dependency
  array**, so `pushMessageRef.current`'s closure is fixed once and always
  reads `messageQueueRef.current` *at call time* (good — no stale closure),
  but it means input typed in the gap between old-queue-being-superseded and
  new-queue-being-assigned can silently target whichever queue happens to be
  in the ref at that instant, not necessarily the one the user's keystroke
  was intended for. This is the concrete "race between closing the old queue
  and starting the new one" the task description asks about — the fix needs
  either a swap that closes-and-drops the old queue atomically with
  installing the new one, or a per-connection identity check before
  `pushMessage` writes.
- **The sibling stream's guard uses a deliberate *double* increment — porting
  only a single increment silently weakens it.** `useSessionService.ts`'s
  `WatchSessions` reconnect loop (added by ADR-023, the closest analog to
  `useTerminalStream.ts` since both are long-lived reconnecting streams, not
  a one-shot fetch like `usePathCompletions`) increments
  `streamGenerationRef` *twice*: once synchronously at the top of the
  public `watchSessions()` call (`useSessionService.ts:829`, comment:
  *"Invalidate any in-flight startStream from prior call"*) and again inside
  the internal `startStream()` closure itself
  (`useSessionService.ts:833`: `const myGeneration =
  ++streamGenerationRef.current`). The first increment invalidates a
  previous *external* caller's attempt; the second stamps *this specific*
  attempt (including the internal self-rescheduled retries in the `finally`-
  equivalent blocks at lines 891/936). A naive port that increments only
  once — either only at the top of `connect()` or only inside the message-
  loop IIFE — collapses these two distinct invalidation events into one,
  which either fails to invalidate a fresh external `connect()` call made
  while an old one is still tearing down, or fails to distinguish the
  internal reconnect-retry loop from a brand-new explicit connect. Get both
  increments right, matching `useSessionService.ts`'s shape, not just
  `usePathCompletions.ts`'s simpler single-increment shape.
- **Off-by-one on the comparison operator.** `usePathCompletions.ts` uses
  strict inequality (`generation !== generationRef.current`) evaluated
  *after* the async work, which correctly drops stale results. The classic
  off-by-one mistake is comparing with `>` or `>=` against a value captured
  *before* increment vs. after, or forgetting that `++ref.current` (prefix)
  vs `ref.current++` (postfix) changes what the captured `generation` value
  actually is relative to what's stored in the ref at that instant — a
  prefix increment (used here) means the captured value *equals* the new
  current value at capture time, so the guard is "has anything newer started
  since," not "am I the most recent minus one." Any rewrite must keep prefix
  increment-then-capture, not swap to postfix, or every guard check is
  permanently one generation behind and never matches.

## 3. WebSocket/ConnectRPC bidi streaming lifecycle pitfalls (Go side)

`server/services/connectrpc_websocket.go` has three structurally-duplicated
handlers (`streamViaControlMode` ~450, `streamShellViaControlMode` ~1104,
`streamViaTmuxCapturePane` ~1494), each spinning up an output goroutine and
an input/read goroutine coordinated via a shared `doneChan`/`errChan` pair,
no `sync.WaitGroup`.

- **The read goroutine's `select { case <-doneChan: return; default:
  ReadMessage() }` pattern (line 948-963, and the two duplicates at ~1330,
  ~1694) cannot observe `doneChan` closing while blocked inside
  `ReadMessage()`.** The `select`/`default` only runs *before* the blocking
  call, once per loop iteration. Once inside `ReadMessage()`, the goroutine
  is only unblocked by the underlying `net.Conn`/`websocket.Conn` actually
  closing (which produces a read error) — not by `doneChan`. If the output
  goroutine exits and closes `doneChan` for a reason unrelated to the
  websocket connection itself dying (e.g. a send error to a *different*
  channel, or an application-level decision to end the stream), the read
  goroutine can leak indefinitely, still blocked in `ReadMessage()`, unless
  something *also* explicitly closes/aborts the connection. This is the
  single most likely goroutine-leak vector to test against for AC4's "Go:
  bounded read-goroutine exit test" — the test needs to assert the read
  goroutine actually terminates within a bound, driven by closing the
  underlying connection (or cancelling context) rather than just closing
  `doneChan` and hoping.
- **No `sync.WaitGroup` means "the stream handler returned" does not mean
  "both goroutines have exited."** The outer `select { case err :=
  <-errChan: ...; case <-doneChan: ... }` (lines ~1092/1451/1948) returns as
  soon as *either* signal arrives — it does not wait for the other goroutine
  to actually finish. In a bounded-exit test, waiting on the handler
  function to return is not sufficient proof the read goroutine exited; the
  test needs its own explicit signal (e.g. instrument with a `WaitGroup` or
  return channel in the test double, or use `-race` + a bounded polling
  assertion on goroutine count) rather than trusting the handler's return as
  a proxy.
- **`errChan` is buffered to exactly 2** (`make(chan error, 2)`, e.g. line
  746) — sized for "at most one error from each of the two goroutines."
  Any test or future change that adds a third goroutine emitting to the same
  `errChan` without resizing the buffer reintroduces a goroutine-leak
  surface (a blocked send on a full unbuffered-in-effect channel, with
  nothing left reading it after the first `select` case fires and the
  function returns).
- **Reconnect on the Go side is a brand-new HTTP upgrade / new goroutine
  set per WebSocket connection** — there is no persistent per-session
  goroutine that survives a reconnect; each `HandleWebSocket` call spins up
  its own `doneChan`/`errChan`/goroutine pair scoped to that specific
  connection's lifetime. This means the *Go-side* reconnect story is
  actually simple (old goroutines are supposed to fully die when the old
  connection closes, a fresh set starts on the new connection) — the risk
  is entirely in whether the old set's death is bounded and guaranteed
  (see above), not in cross-connection state confusion the way the client
  side has to worry about (`messageQueueRef` reuse, generation guards,
  etc). Don't over-engineer a generation counter into the Go side to mirror
  the client fix — the actual gap here is goroutine-exit boundedness, not
  epoch tracking.

## 4. Specific risk in this stack: ConnectRPC-over-WebSocket bidi framing

This is not a native browser `WebSocket` used directly — it's ConnectRPC's
bidi-stream semantics tunneled over a WebSocket transport (framed messages
via `protocol.CreateEnvelope`/`protocol.ParseEnvelope`, an `EndStreamFlag`
sentinel, a `websocket-transport.ts` on the client). Two things that don't
show up in a generic "WebSocket reconnect" mental model:

- **`EndStreamFlag` is an application-level "this logical stream is done"
  signal distinct from the WebSocket close frame** (line 973-976:
  `if envelope.Flags&protocol.EndStreamFlag != 0 { errChan <- nil; return }`).
  A client-side fix that only guards against *connection*-level
  close/reconnect (raw WS close code) but doesn't also handle a mid-stream
  `EndStream` envelope arriving right as a new `connect()` is starting will
  miss a valid drop-window: the server can logically end a stream without
  the underlying socket ever closing in a way the client's epoch guard was
  designed to catch, if the guard is keyed only off WS `onclose`/abort
  events rather off "did we start a new MessageQueue since this envelope was
  queued."
- **Coalescing on the output side (`streamViaControlMode` lines 799-820)
  batches multiple PTY frames into a single proto message.** This doesn't
  directly affect the *input*-replay bug (input isn't coalesced), but it
  matters for AC5's manual-repro verification: because output is batched and
  can arrive after a reconnect boundary in a single envelope, a
  reconnect-timing test needs to account for output arrival timing being
  decoupled from real-time PTY writes by up to the coalesce window — don't
  assume "if no phantom keystroke appears within N ms of reconnect, it's
  fixed" without also accounting for coalesced-output latency skewing when
  a human tester perceives the reconnect as "complete."
- **Two divergent live-region patterns already coexist, and the only
  reconnect-domain precedent doesn't use the shared component.**
  `LiveRegion.tsx` (`web-app/src/components/ui/LiveRegion.tsx`) exports a
  reusable `<LiveRegion message politeness>` + `useLiveRegion()` pair, but
  its only current consumer, `components/layout/ConnectionIndicator.tsx`
  (the component closest in domain to this feature — it's literally the
  connection-status indicator), doesn't use it: it hand-rolls its own
  `<div aria-live="polite" aria-atomic="true">` (`ConnectionIndicator.tsx:62-68`)
  instead, and only ever uses `polite`, never `assertive`. Two implications:
  pick `LiveRegion`/`useLiveRegion()` deliberately for the new
  `InputDropBadge` rather than inventing a third ad-hoc pattern, and be
  aware this repo has *no* existing precedent for an `assertive`
  announcement actually shipping — AC3's assertive requirement is new
  ground, not a proven pattern, so it deserves real cross-browser/AT
  verification rather than assuming it "just works" like the existing
  `polite` usages.
- **`role="status"` overridden to `aria-live="assertive"` is a non-standard
  combination.** WAI-ARIA's implicit live-region semantics: `role="status"`
  implies `polite`, `role="alert"` implies `assertive` — they're meant to be
  paired, not mixed. `LiveRegion`'s `politeness` prop overrides the explicit
  `aria-live` attribute while leaving `role="status"` fixed
  (`LiveRegion.tsx:22-24`). Most browsers/AT honor the explicit `aria-live`
  override, but it's an unusual combination less consistently tested across
  AT than a plain `role="alert"` element. If AC3's assertive announcement
  genuinely needs interrupt-and-announce semantics, consider `role="alert"`
  outright rather than `LiveRegion` with `politeness="assertive"` layered on
  `role="status"`.
- **Accessibility live-region ordering guarantee is weaker than the visual
  UI's.** `LiveRegion.tsx` (`web-app/src/components/ui/LiveRegion.tsx`)
  already exists with `useLiveRegion()` — `announce(msg)` sets `message`
  then clears it after a fixed 1000 ms `setTimeout` (line 39). If the
  drop-and-signal path calls `announce()` for two drops within that 1000 ms
  window (very plausible during rapid/triple reconnect flapping — the exact
  scenario in the ticket), the second `announce()` call's `setMessage`
  happens while the first message is still visible/announced, and depending
  on timing the *cleared* `setTimeout` from the first call can fire *after*
  the second message was set, wiping the second announcement early. Screen
  readers are not guaranteed to re-announce a `role="status"` region if the
  text is set to the same string twice in a row, either (some AT
  implementations only fire on text *change*), so a naive "drop happened"
  message on every drop, without a differentiating counter/timestamp
  appended to the text, may silently swallow repeated announcements during
  exactly the flapping condition this bug is about. Consider a
  monotonically-changing suffix (e.g. drop count) in the announced string,
  or a debounce/queue on the announce calls themselves, given the direct
  precedent of `useLiveRegion`'s fixed-1000ms-clear design not being written
  with rapid-repeat-announcement in mind.

## 5. Designing against the "two fixes, still missed the client" pattern

The project history is the strongest signal here: `3546c2b12` and
`c0e6c4ce6` each correctly fixed a *specific* server-side re-send mechanism
(driver polling cooldown, then stale-preview-buffer cooldown generalization)
but neither one's author verified the client-side relay path was also
idempotent/drop-safe — because the bug's *symptom* (repeated "1") was fully
explained by the server-side mechanism in the reported case, so the fix was
scoped to "make the symptom stop reproducing" rather than "make replay
structurally impossible across every layer that could reproduce it."

What would make the same pattern happen again here, and what to design
against:

- **Fixing MessageQueue.close() in isolation, without an end-to-end test
  that spans a full reconnect (old connection dies mid-send → new connection
  established → assert exactly one delivery), reproduces the same gap**:
  a unit test on `MessageQueue.close()` alone (assert `queue` is empty after
  `close()`) proves the *class* is now correct but not that
  `useTerminalStream.ts`'s *usage* of it is — e.g. if `connect()` still
  calls `new MessageQueue()` and reassigns `messageQueueRef.current` in a
  way that leaves the previous queue's async iterator running independently
  (see §2's `messageQueueRef` race), the queue-level fix is necessary but
  not sufficient. AC4 already asks for a Jest test on "queued-message-drop-
  on-close interleaving" — that test must exercise the *hook*, not just the
  queue class, to actually close this class of gap.
- **The requirements doc's own AC3 language is a trap worth reading
  carefully**: it was "marked done" against the backlog item before this
  session despite `MessageQueue.close()` demonstrably not clearing `queue`
  — i.e. the *previous* round of work on this exact bug already believed
  the client side was covered and was wrong. That means the regression test
  added this round needs to be strong enough that a future reviewer/session
  can't repeat the same false-positive marking — assert on behavior
  (message never reaches the mock transport after close/supersede), not on
  the presence of a generation ref or a `queue = []` line, which can be
  refactored away while silently reintroducing the bug the same way the
  original two commits' narrow scoping did.
- **The clearest, most concrete evidence this exact gap was already known
  and scoped out once before: ADR-023 (`docs/adr/ADR-023-client-reconnect-browser-lifecycle.md`).**
  This ADR predates this backlog item by roughly two months and explicitly
  names the race class at stake — "Stale closure / dual-stream race... two
  overlapping `visibilitychange` triggers could open two concurrent
  streams" (ADR-023 Context section) — and ships a fix for it, but *only*
  for `useSessionService.ts`'s `WatchSessions` stream (`streamGenerationRef`,
  added per ADR-023 §3). Its own Phase 3 plan for `useTerminalStream.ts`
  (ADR-023 §5, "Decision" section) lists only `shouldReconnectRef` +
  `terminalBackoffRef` (jittered reconnect + backoff) — no generation guard
  — despite the *same document*, one paragraph earlier (§Rejected approaches,
  "Approach A"), stating outright that `MessageQueue` "is one-shot — it
  cannot be replayed after `.close()`... Each terminal reconnect must
  construct a fresh `MessageQueue` and re-run the full handshake." In other
  words: the ADR that shipped the reconnect infrastructure this bug lives in
  *named the precondition for this exact bug* (fresh MessageQueue per
  reconnect, no replay) in the same breath as fixing the identical race for
  a sibling stream, and then didn't carry the fix over. This is the
  textbook version of "a mitigation adopted for one of two structurally
  similar streams, with the other silently left as an implicit non-issue."
  **Design against**: whenever a race/lifecycle fix lands for one of several
  structurally similar reconnecting streams in this codebase (there are at
  least two now — watch and terminal — and the three Go read-goroutines in
  §3 are a third instance of the same "one fix, N-1 near-duplicates
  unaudited" shape), explicitly enumerate the siblings and confirm each one
  either has the fix or has a documented reason it doesn't need it, rather
  than treating the fixed one as representative of the class.
- **Both landed server-side fixes were "add a cooldown/latch to the specific
  mechanism that was over-firing"** — a locally-scoped fix to the exact
  reported code path. The client fix here risks the same trap in the other
  direction: hardening `MessageQueue`/`useTerminalStream` against *this*
  reconnect scenario without also checking `useTerminalFlowControl.ts`'s
  `sendInput` (`useTerminalFlowControl.ts:142-163`), which **already**
  silently drops input when `!isConnectedRef.current` (line 143) with zero
  user feedback — i.e. there is a *second*, pre-existing silent-drop path in
  the same input pipeline that AC3's badge/announcement requirement should
  also cover, or the "visibly and audibly signaled" goal is only half-met
  (queued-then-dropped input gets a badge, immediately-rejected input during
  a known-disconnected state stays silent, and a user watching the ticket's
  exact flapping scenario could still see input vanish with no feedback).
  Explicitly design the drop-signal to fire from *both* rejection points —
  `sendInput`'s early return and `MessageQueue`'s close-time discard — not
  just the one this task's requirements doc happened to name.
