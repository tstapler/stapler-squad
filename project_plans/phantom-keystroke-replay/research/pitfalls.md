# Pitfalls Research — Phantom Repeated "1" Keystroke on Reconnect

Backlog item `04089969-0f19-499c-be34-2e8bcfc4f13e`. This document catalogs known
failure patterns for this bug class and cross-references them against the actual
code in this repo (not a generic literature review) so Phase 0 (root-cause proof)
and the eventual fix can be checked against precedent.

---

## 1. React async-loop-vs-ref races ("stale response after newer request superseded it")

### The general pattern
A `useRef`-held resource is reassigned on retry/reconnect while an in-flight async
operation from the *previous* generation (a `for await` loop, a `fetch`, a timer)
is still running and has not yet observed the swap. Any code inside that stale
operation that reads `.current` *after* the reassignment silently starts acting on
the new resource instead of erroring out or being ignored — this is the "stale
closure over a mutable ref" trap, and it cuts both ways:

- If the stale loop reads `ref.current` fresh each time (not a captured local),
  it can accidentally start writing into the *new* generation's resource — a
  "successful-looking" bug that actually causes cross-generation contamination.
- If the stale loop captured the *old* resource in a local variable, it can keep
  operating on a resource that is supposed to be dead — the classic "stale
  response" race (SWR, TanStack Query, and Mosh's predictive-echo design docs all
  solve this with a generation/epoch tag: "ignore anything whose epoch != the
  current epoch," rather than trying to hard-cancel the old operation).

Standard mitigations, in order of robustness:
1. **Epoch/generation counter** — increment an integer ref on every new
   connect/request; capture it locally at the start of the async operation;
   check `if (myEpoch !== epochRef.current) return/ignore` at every resumption
   point (after each `await`, each loop iteration). This is the same idea SWR's
   dedupe/revalidation and TanStack Query's `queryKey` + `AbortSignal` combo use:
   "the newer request always wins; a superseded one is a no-op even if it
   completes."
2. **`AbortSignal.reason`** — pass a distinct reason string/object per
   generation; check `signal.reason === expectedReason` rather than just
   `signal.aborted`, because a *new* AbortController is a different signal object
   and a stale closure checking the wrong (old) signal will never see the abort.
3. **Checking a captured local instead of `.current` inside the loop** — capture
   `const queue = messageQueueRef.current` once at the top of the async IIFE and
   compare `messageQueueRef.current === queue` before acting, instead of blindly
   reading `.current` on every iteration.

### What this repo already does (precedent to follow)
This exact pattern — an integer ref incremented per-request, checked before
acting on a stale response — is **already the established idiom in this
codebase**, just not applied to the terminal stream:

- `web-app/src/components/sessions/QuickOpenPalette.tsx:84,125,130,152,157` —
  `requestIdRef`, `if (requestId !== requestIdRef.current) return;`
- `web-app/src/components/sessions/FileTree.tsx:338,470,474,482` —
  `searchRequestIdRef`, same guard.
- `web-app/src/lib/hooks/useFileService.ts:97,117,124,130,136` — same guard,
  explicitly commented as the "generation counter" pattern.
- `web-app/src/lib/hooks/usePathCompletions.ts:83` — comment literally says
  *"Generation counter – discards responses that arrive after a newer request
  fired."*

**None of this exists in `useTerminalStream.ts`.** The `connect()` function
(`web-app/src/lib/hooks/useTerminalStream.ts:156-345`) has no epoch/generation
ref at all. When `connect()` runs a second time (e.g. from the auto-reconnect
effect in `TerminalOutput.tsx:734-747` firing `connect()` on a timer while a
prior connection is still tearing down), it reassigns:

```ts
abortControllerRef.current = new AbortController();       // line 175
messageQueueRef.current = new MessageQueue();              // line 176
```

and starts a **second** unawaited `(async () => { for await (const msg of
stream) {...} })()` IIFE (line 209) with no way to tell it apart from a
still-running prior IIFE. The `firstMessage` flag, `isConnected`/`terminalState`
setters, and all `flowControl.handle*` calls inside that loop are per-closure —
there is no check anywhere that says "am I still the active generation?" A
straightforward fix consistent with the rest of the codebase is to add a
`connectionEpochRef` (or reuse the `AbortController` identity by capturing it in
a local and comparing `abortControllerRef.current === myAbortController` before
each side effect inside the loop), mirroring `useFileService.ts`.

- The `pushMessageRef.current` indirection (`useTerminalStream.ts:125-133`,
  called from `useTerminalFlowControl.ts:88-90`) is explicitly commented "Bug
  Risk 3 mitigation... to prevent stale closure issues on reconnect" — but this
  only solves *which queue receives a live keystroke typed right now*. It does
  **not** address old data already sitting in a queue instance getting drained
  into a *new* stream (see §4 / MessageQueue analysis below), nor does it stop a
  stale for-await loop from calling `flowControl.handle*` with effects that
  matter (e.g. state resync, echo-ack bookkeeping) against the wrong generation.

---

## 2. tmux control-mode: ambiguous ack on retry-fallback double-delivery

### The general pitfall
In any request/response protocol layered over a single ordered stream (tmux
control mode's `%begin`/`%end`/`%error` framing is a textbook example), a command
can be sent successfully and *actually execute* on the far end, but the
caller-side plumbing never receives (or times out waiting for) the
confirmation — because the connection dies mid-response, the FIFO
correlation gets desynced, or the ack races a `%exit`. A naive
"call failed → retry via a different path" then re-sends the *same logical
input* through a second channel, and the target (the tmux pane / the shell
reading from it) sees it twice.

### What this repo's control_mode.go actually does
`session/tmux/control_mode.go` correlates commands to responses via a **strict
FIFO queue** (`pendingCmds`), not a per-command ID: `%begin` always pops the
head of `pendingCmds` (`processControlModeLine`, `control_mode.go:341-347`).
This means:

- The response ↔ request correlation is purely positional. If a
  `resultCh` is ever appended to `pendingCmds` out of the actual send order
  (e.g. two goroutines racing to enqueue), the wrong caller gets the wrong
  ack/error. `runCMSender` is the single writer to stdin and the single
  appender to `pendingCmds` (`control_mode.go:463-482`), which correctly
  prevents this misattribution for **queued commands**, but:
- **`SendInputViaControlMode` (input path used by the ticket's flapping scenario)
  is explicitly fire-and-forget** (`control_mode.go:652-682`): it enqueues a
  `send-keys` command with a buffered(1) `resultCh` that **nobody ever reads**
  (comment at line 671-673: *"nobody reads it, and Go GCs it"*). This means the
  input path has **no ack correlation at all** — the caller
  (`server/services/connectrpc_websocket.go:916`) cannot distinguish "tmux
  received and executed the keystroke" from "the enqueue itself failed" except
  by the `error` return of `SendInputViaControlMode`, which only reflects
  whether the request made it onto `highPriSendCh` (or the ctx timed out
  waiting to enqueue) — **it says nothing about whether tmux actually executed
  the `send-keys` command**. So by design, a "successfully enqueued but tmux
  never got to run it before %exit" case and a "cleanly executed" case are
  indistinguishable to the caller from the return value alone.
- The fallback: on any non-nil error from `SendInputViaControlMode`, the caller
  (`connectrpc_websocket.go:911-923`) unconditionally calls
  `sendInputToTmux(tmuxSessionName, input.Data)` (a `tmux send-keys` subprocess).
  **This is the exact "ambiguous ack → double-delivery" shape called out in the
  requirements' ruled-out list.** Specifically:
  - `SendInputViaControlMode` returns non-nil in two distinguishable-but-
    conflated cases: (a) `highPriSendCh` is nil / control mode not running
    (never delivered — safe to retry), and (b) `ctx.Done()` fires while
    waiting to enqueue **after** the command may have already been placed on
    `highPriSendCh` in a previous call, or after tmux already executed it in a
    prior attempt (timing-dependent — unsafe to blindly retry).
  - Because `runCMSender.process()` writes to stdin (`fmt.Fprintf(stdin,
    "%s\n", req.line)`, line 480) **before** any ack is available to the caller,
    and the caller's 2s context (`connectrpc_websocket.go:915`) has already
    elapsed by the time the fallback fires, there is a real window where tmux
    *did* receive and process the `send-keys -H <hex bytes>` command via CM,
    and the subprocess fallback then sends the identical hex bytes again via
    `tmux send-keys` — a textbook double-delivery of the same keystroke into
    the live pane, which matches the ticket's reproduction log lines exactly
    (`CM input failed, retrying via subprocess`).
  - Note also: unlike queued commands via `sendCMCommand`/`enqueueCMCommand`
    (which do consume a real ack through `resultCh`), the fire-and-forget input
    path never even reads back whether tmux's `%error`/`%end` for the
    `send-keys` command indicated success — so "would have detected a real tmux
    error" isn't available as a signal to suppress the fallback either.

### Established precedent in this repo for the *symmetric* problem
`docs`/git history shows this general failure mode ("retry re-fires an
identical send that already landed, or fires forever without backoff") has
already bitten this codebase once and been fixed:
- Commit `006b45a` *"fix(control-mode): eliminate 3s timeout race and blank
  terminal on dead session"* — fixed exactly the "`%exit` vs EOF drain race
  leaves an orphaned `resultCh` in `pendingCmds`" bug that is *still described
  in the comments* at `control_mode.go:399-403` and `control_mode.go:466-469`
  ("there is a race window between `%exit` and EOF where `runCMSender` can
  append a new resultCh to `pendingCmds` after the EOF drain has already run").
  This is the same *class* of bug (ack delivery vs. teardown ordering), applied
  to command-response commands rather than the fire-and-forget input path — a
  strong signal that the input path deserves the same rigor rather than being
  fire-and-forget with a blind retry-on-any-error fallback.
- Commit `b8763c63d` *"fix(session): rate-limit backlog nudge retry on SendKeys
  failure (BUG-041)"* — a **different** subsystem (autonomous driver nudges)
  hit the sibling failure mode: a failed `SendKeys` was retried on every driver
  tick forever with no backoff (392 consecutive failed sends over ~13 minutes
  against a dead pane). The fix there was to always advance a timestamp
  regardless of send outcome and let an existing grace-timeout mechanism take
  over, rather than looping the identical send. Relevant precedent for "don't
  let an ambiguous-failure retry loop run unbounded," though the terminal input
  path's problem is a single conflated retry, not an unbounded loop.

---

## 3. Go goroutine lifecycle: orphaned readers/forwarders after supersession

### General pitfall
A goroutine reading from a channel/queue and forwarding to a downstream sink
(here: the WebSocket-read goroutine in `connectrpc_websocket.go` that forwards
`input.Data` to `SendInputViaControlMode`/`sendInputToTmux`) must have a single,
unambiguous way to know it has been superseded by a newer connection/goroutine
for the *same session*. If two such goroutines can be alive concurrently for
the same tmux session (old connection's reader not yet exited when the new
connection's reader starts), both can forward keystrokes to the same pane.

Idiomatic fixes, in increasing rigor:
1. **`context.Context` cancellation propagated into a `select`** — every blocking
   operation in the goroutine (`stream.conn.ReadMessage()`, channel sends) races
   against `ctx.Done()` so the goroutine exits promptly when superseded.
   `connectrpc_websocket.go`'s reader loop already does this for the *read*
   side via `doneChan` (line 858-861), but note `stream.conn.ReadMessage()`
   itself (line 863) is **not** interruptible by `doneChan` — it's a blocking
   OS call, so the `select` guard only takes effect *between* reads, not during
   one. This is a known Gorilla-websocket-style pitfall: `select { case
   <-doneChan: return; default: }` immediately followed by a blocking call
   means a `doneChan` close only takes effect the next time the loop reaches
   the top — one more `ReadMessage()` can still complete and be acted on after
   shutdown was signaled.
2. **Closing a done-channel that the goroutine's blocking calls are wired
   into** (e.g. via `conn.SetReadDeadline` + close-triggered forced socket
   close, or wiring `ctx` into the underlying I/O) so the blocking call itself
   unblocks rather than relying on the next loop iteration.
3. **A generation/session-token check before every forward** — analogous to §1's
   epoch pattern, applied on the Go side: tag each WebSocket connection/session
   attach with a monotonically increasing token, store the "current" token on
   the `TmuxSession`/instance, and have the input-forwarding call
   (`instance.SendInputViaControlMode`) no-op if the caller's token is stale.
   **This does not currently exist anywhere in the Go session/websocket code**
   — there is no concept of "which WebSocket connection currently owns input
   forwarding for this tmux session" at the Go layer. Multiple concurrent
   `streamViaControlMode` goroutines (one per WebSocket connection, e.g. during
   a client-side reconnect where the old socket hasn't been torn down by the
   server yet) can each independently call `instance.SendInputViaControlMode`
   for the same session with no cross-goroutine awareness of each other. This
   is a materially different exposure from the client-side MessageQueue issue
   in §1/§4 — it means even a perfect client-side fix could still leave two
   *server-side* goroutines racing to deliver input for overlapping WebSocket
   connections during a flap, if the server doesn't close out the old
   connection's goroutine before/while accepting the new one.

### Confirming whether this repo's shutdown path actually exhibits this
`readControlModeOutput()` (`control_mode.go:203-215`) has the identical
`select { case <-doneCh: return; default: scanner.Scan() }` shape — same
"only checked between blocking calls" caveat, but `bufio.Scanner.Scan()` here
is bounded by the underlying pipe being closed in `StopControlMode()`
(`control_mode.go:179-183`), which *does* force the blocking read to
return — so this particular goroutine is not vulnerable the same way a
websocket `ReadMessage()` without a wired-in deadline/cancel would be. This is
worth explicitly re-verifying for the WebSocket reader goroutine in
`connectrpc_websocket.go`, since gorilla/websocket connections need
`conn.Close()` (not just closing `doneChan`) to unblock a pending
`ReadMessage()`.

---

## 4. MessageQueue: buffered array survives across reconnect boundaries

Not explicitly one of the five framed questions, but a directly relevant
finding from reading `web-app/src/lib/terminal/MessageQueue.ts`:

```ts
export class MessageQueue {
  private queue: TerminalData[] = [];   // in-memory FIFO array
  ...
  push(msg: TerminalData) {
    if (this.closed) return;
    if (this.resolve) { this.resolve(msg); this.resolve = null; }
    else { this.queue.push(msg); }       // buffers indefinitely if no consumer awaiting
  }
```

If `push()` is called while nobody is awaiting (the `for await` consumer of
`Symbol.asyncIterator` hasn't reached its next `await` yet, or is momentarily
between iterations), the message sits in `this.queue` until the iterator is
pumped again. Per requirement Goal 3 ("Input typed while disconnected must be
dropped, not queued"), **this in-memory array is exactly the kind of
buffer that must never accumulate speculative input across a reconnect
boundary.** Today, `connect()` (`useTerminalStream.ts:176`) always constructs a
*brand-new* `MessageQueue()` on each call and `disconnect()`
(`useTerminalStream.ts:363-366`) always closes and nulls the old one out before
that — so in the *current* code, a new `MessageQueue` instance starting empty
is what actually prevents already-queued-but-undelivered messages from a
prior connection surviving into the next one. This means the buffering
behavior is not itself dangerous *today* as long as `disconnect()` always
runs to completion and nulls `messageQueueRef.current` **before** `connect()`
re-assigns it — which is exactly the ordering that a race between the
auto-reconnect timer (`TerminalOutput.tsx:734-747`, which calls `connect()`
directly, not through `disconnect()`) and any leftover `connect()`/`disconnect()`
call could violate. If `connect()` is ever invoked without a properly awaited
`disconnect()` immediately before it (e.g., overlapping calls during a flap),
the new `MessageQueue()` reassignment (`messageQueueRef.current = new
MessageQueue()`, line 176) simply orphans whatever was in the old queue's
`this.queue` array along with the old for-await consumer — messages already
buffered there are neither delivered nor explicitly dropped-with-signal, they
just silently vanish when GC'd. This is different from "replay" but is the
mirror-image gap called out in Goal 3: there is currently no user-visible/
audible signal anywhere in this file or `useTerminalStream.ts` for "input was
dropped because you were disconnected." Confirmed by grep: no
sound/toast/notification call sites exist in `MessageQueue.ts` or
`useTerminalStream.ts`.

---

## 5. Test-simulation pitfalls for "reconnect during an input send"

### Over-mocking hides the exact bug class in question
`web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` currently
**mocks `MessageQueue` entirely**:
```ts
jest.mock('@/lib/terminal/MessageQueue', () => ({ ... }));
```
(`useTerminalStream.test.ts:45`). A test written against this file's existing
mocking style to "simulate a reconnect during an input send" would exercise a
*fake* `MessageQueue` and could not observe the real queue's buffering
behavior (§4) or the real async-iterator hand-off semantics — it would end up
testing that `connect()` calls `new MessageQueue()` the expected number of
times, not that stale-generation messages are actually dropped/never
delivered. **The regression test required by Goal 4 needs the real
`MessageQueue` implementation in the loop** (only mocking the ConnectRPC
`streamTerminal`/transport layer, as `MessageQueue.test.ts` already does in
isolation), or it will pass while the underlying bug remains.

### Fake timers vs. microtask/async-generator ordering
`MessageQueue`'s iterator (`Symbol.asyncIterator`) resolves a `Promise` that is
driven by `push()` calling `this.resolve(msg)` synchronously — this is a
microtask-scheduled continuation, not a macrotask/timer. Jest fake timers
(`jest.useFakeTimers()`) only control `setTimeout`/`setInterval`/`Date.now`
scheduling; they do **not** advance microtasks queued via native Promises.
A test that does:
```ts
jest.useFakeTimers();
queue.push(msgA);
jest.advanceTimersByTime(1000); // does nothing for the pending microtask
expect(iteratorGotMsgA).toBe(true); // may fail — microtask hasn't flushed
```
needs an explicit `await Promise.resolve()` / `await
flushMicrotasks()`-equivalent (or `await null` in a loop) between the
push and the assertion, *in addition to* any timer advancement used to drive
the reconnect's `setTimeout`-based backoff (`TerminalOutput.tsx:740` uses a
real `setTimeout` for the reconnect delay, which fake timers *do* control).
Mixing both — real timer control for the reconnect delay and microtask
flushing for the queue's promise resolution — in the same test is easy to get
wrong; a common false-negative is advancing timers but never awaiting a tick,
so the assertion silently checks pre-push state and passes for the wrong
reason.

### Asserting against the wrong queue instance after a reconnect swap
Because `connect()` creates a **new** `MessageQueue` object on each call
(`messageQueueRef.current = new MessageQeueue()`), a test that captures a
reference to the queue instance *before* triggering the simulated reconnect
(e.g. by spying on the mock constructor's first return value) and then asserts
against that captured reference will be silently asserting against a queue
that the component has already abandoined — it needs to re-fetch
`messageQueueRef.current` (via whatever the test's access path is — a spy on
the `MessageQueue` constructor capturing all instances, not just the first)
after the reconnect fires, or it will report "message never arrived" for the
right reason (it's on the *new* queue) but the wrong diagnosis (looks like a
delivery bug when it's actually a stale-reference-in-the-test bug). This is
the test-authoring mirror image of the production bug in §1 — the harness
must not fall into the same "stale ref" trap it's supposed to be catching.

### False negative from asserting on send *count* instead of send *content/target*
Given the actual reported symptom is a duplicate delivery **to tmux**, not
just a duplicate WebSocket frame, a Jest test operating purely at the
`MessageQueue`/`useTerminalStream` layer can prove "the client only ever
pushed the keystroke once" while the actual duplication happens server-side
(§2/§3, Go layer, tmux control-mode racing subprocess fallback) — the Jest
regression test for Goal 4 needs to be paired with a **Go** test asserting
`SendInputViaControlMode` + subprocess-fallback never both execute for the
same logical input, not just a frontend test; a frontend-only green test
would give false confidence that the class of bug is closed when the
higher-likelihood root cause (per the ticket's captured log lines) is on the
Go side.

---

## 6. Repo bug history: prior fixes for this exact pattern class

Confirmed by grepping `docs/adr/`, `CHANGELOG.md`, and `git log`:

| Precedent | Relevance |
|---|---|
| `006b45a` *fix(control-mode): eliminate 3s timeout race and blank terminal on dead session* | Same file (`control_mode.go`), same failure shape: a resultCh/ack could be orphaned across the `%exit`/EOF teardown race. The fix (synchronous drain + `controlModeExited` check before appending) is documented in the *current* code's own comments (`control_mode.go:399-403`, `466-469`) — meaning the file already has a "teardown ordering" bug-fix precedent that the input fire-and-forget path (§2) was not brought under the same discipline. |
| `b8763c63d` *fix(session): rate-limit backlog nudge retry on SendKeys failure (BUG-041)* | Sibling subsystem (autonomous driver), same root shape: a failed send retried unconditionally with no backoff/ambiguity handling, 392 consecutive failures observed live. Precedent for "a bare retry-on-error is not safe without idempotency or backoff," directly applicable to the CM→subprocess fallback in `connectrpc_websocket.go:911-923`. |
| CHANGELOG: *"terminal: sync cursor position after snapshot replay and break resize oscillation"* (`347d991`) | Same general area (terminal snapshot/replay path) previously had a distinct replay-correctness bug; worth reviewing that diff if more historical context on "replay" semantics in this terminal stack is needed — not re-read in depth here since it concerns cursor-position sync rather than input duplication. |
| CHANGELOG: *"events: add sequence numbers and 1-hour catch-up replay to EventBus"* (`03bbfc7`) | A different subsystem (EventBus) already solved "safe replay" via explicit sequence numbers — an existing in-repo example of using monotonic sequence numbers to make replay idempotent, which is a reasonable model for the input path if replay/queuing is ever intentionally reintroduced (though Goal 3 explicitly wants drop-not-replay for input, not idempotent replay). |
| `docs/adr/` | No ADR exists yet for reconnect/replay/idempotency semantics on the terminal input path specifically — `010-frontend-modularity.md`, `008-redux-toolkit-frontend-state.md`, `013-*` matched only tangentially (state management and unrelated workflow-engine ADRs). This suggests the fix for this ticket may warrant a short ADR given it's establishing a new invariant ("at-most-once input delivery across reconnects") that isn't currently documented anywhere. |

---

## Summary of actionable pitfalls for the fix + test phases

1. **No epoch/generation guard exists in `useTerminalStream.ts`'s `connect()`**,
   unlike the four other hooks in this exact codebase (`useFileService.ts`,
   `usePathCompletions.ts`, `QuickOpenPalette.tsx`, `FileTree.tsx`) that already
   use this pattern — the fix should follow that established convention rather
   than inventing a new one.
2. **The CM-input fire-and-forget + blind subprocess-fallback-on-any-error
   in `connectrpc_websocket.go:911-923` is the most likely concrete
   double-delivery mechanism** matching the ticket's log lines verbatim — its
   error is inherently ambiguous (enqueue failure vs. ctx-timeout-after-possible-
   send vs. tmux-side failure) and this repo has fixed the identical
   ack-ambiguity shape once already in the same file (`006b45a`).
3. **No server-side concept of "which connection owns input forwarding"**
   exists for a given tmux session — a purely client-side fix does not close
   this gap if two WebSocket goroutines can be alive concurrently during a flap.
4. **Existing Jest infra mocks `MessageQueue` away entirely** — the new
   regression test must use the real `MessageQueue` (as `MessageQueue.test.ts`
   already does) to actually exercise the bug, and must pair fake-timer control
   (reconnect backoff) with explicit microtask flushing (queue promise
   resolution) — plus a Go-side test, since a frontend-only test cannot observe
   the server-side double-send path in §2/§3.
5. **No user-visible/audible "input dropped" signal exists anywhere** in
   `MessageQueue.ts` or `useTerminalStream.ts` today — Goal 3's requirement for
   visible/audible feedback is a net-new UI surface, not a tweak to an existing one.
