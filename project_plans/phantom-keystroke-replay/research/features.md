# Features Research: Phantom repeated "1" keystroke on session open/reconnect

Agent 2 (Features) research for backlog item `04089969-0f19-499c-be34-2e8bcfc4f13e`.

## 1. `TerminalOutput.tsx` — `handleTerminalData` / `sendInputWithEcho` call site (~511-538)

```tsx
const handleTerminalData = useCallback((data: string) => {
  if (sspNegotiated && sendInputWithEcho) {
    const echoNum = sendInputWithEcho(data);
    ...
  } else {
    sendInput(data);
  }
}, [sendInput, sendInputWithEcho, sspNegotiated]);
```

- One xterm `onData` keystroke → exactly one call to `sendInputWithEcho` (SSP predictive-echo path) or `sendInput` (raw path). **No retry-on-error logic exists at this call site** — there is nothing here that would re-invoke `handleTerminalData` or resend `data` if the underlying push fails.
- `sendKey` (modifier-sticky path, ~526-538) is a separate producer that funnels into the same `handleTerminalData`, so it shares the same single-call guarantee.
- Conclusion: the duplication is not happening at the xterm-event → dispatch boundary. It must originate downstream (queue/connection layer) or on the server.

## 2. `useTerminalFlowControl.ts` — `sendInput` / `sendInputWithEcho` / SSP state

File: `web-app/src/lib/hooks/useTerminalFlowControl.ts` (541 lines).

- `sendInput` (line 311) and `sendInputWithEcho` (line 366) both **gate on `!pushMessageRef.current || !isConnectedRef.current`** and return early (no-op) if disconnected. This is a real guard against sending while known-disconnected — good design, but it depends entirely on `isConnectedRef` being accurate and updated synchronously with the actual connection state (see gap in §3).
- Errors from `pushMessage(...)` are caught and forwarded to `handleError`/`onError` — **there is no catch-and-retry anywhere in this file.** Confirms no client-side retry mechanism could double-send.
- `handleSspNegotiation` (line 270) only sets `sspNegotiated` and lazily constructs an `EchoOverlay` for **local, client-side predictive rendering** (`echoOverlayRef.current.showPredictiveEcho(input)` inside `sendInputWithEcho`). It does not resend or replay any previously-sent input; the echo overlay is purely a local rendering prediction keyed by an ever-incrementing `echoCounterRef`, not a resend queue. Renegotiation (a fresh `sspNegotiation` message on reconnect) does not walk back through `echoTimestampsRef`/`echoCounterRef` and resend anything — it only re-arms the overlay for future keystrokes.
- **Large-paste chunking** (`sendInput`, PASTE_CHUNK_SIZE=512B, `setTimeout(sendChunk, 10ms)` loop) re-checks `pushMessageRef.current`/`isConnectedRef.current` and `sessionId !== sessionIdAtStart` on every chunk, but note: it does **not** re-check between disconnect and a subsequent reconnect of the *same* session — if a reconnect happens mid-paste (same `sessionId`), `sendChunk` will keep firing and pushing into whatever `pushMessageRef.current` currently resolves to, i.e., the **new** queue after reconnect. This is a plausible-but-narrow edge case for multi-byte input, not exactly a "single `1`" repeat, but worth covering with a regression test per requirement #4 (chunked paste + mid-flight reconnect).

## 3. `MessageQueue.ts` + `MessageQueue.test.ts` — what `close()` does NOT do

File: `web-app/src/lib/terminal/MessageQueue.ts` (68 lines).

```ts
close() {
  this.closed = true;
  if (this.resolve) {
    this.resolve(create(TerminalDataSchema, { sessionId: "", data: { case: undefined } }));
    this.resolve = null;
  }
}
```

**Confirmed gap: `close()` never touches `this.queue` (the internal array).** It only:
1. Sets `this.closed = true`.
2. If the async generator is currently *blocked awaiting a Promise* (`this.resolve` is set), unblocks it with a filtered sentinel.

It does **not** clear/drain `this.queue`. The iterator loop is:

```ts
async *[Symbol.asyncIterator]() {
  while (!this.closed || this.queue.length > 0) {
    if (this.queue.length > 0) {
      yield this.queue.shift()!;
    } else {
      ... await new Promise(resolve => { this.resolve = resolve; }) ...
    }
  }
}
```

The loop condition is `!this.closed || this.queue.length > 0` — **explicitly designed to keep draining any items already sitting in `this.queue` even after `close()` was called**, as long as the same async-generator instance (the same `for await` consumer, i.e., the same in-flight ConnectRPC `streamTerminal` call) is still being pumped. There is a real (if narrow) race window: `push()` appends to `this.queue` instead of resolving the pending promise whenever `this.resolve` is `null` at push time (i.e., between the generator finishing one loop iteration and re-arming its `await new Promise(...)`). Any message that lands in `this.queue` during that window will be delivered to the **old** stream even if `close()` is called immediately afterward, for as long as that old stream's consumer keeps running (i.e., until `abortController.abort()` actually tears down the ConnectRPC call — see §4, this is not always immediate).

- `MessageQueue.test.ts` (136 lines) exercises: ordering, push-while-iterating, close-unblocks-waiting-iterator, sentinel filtering, push-after-close-is-noop, `isClosed()`. **There is no existing test that pushes a message, then calls `close()` before the generator drains it, and asserts the message is or isn't delivered** — i.e., no test currently locks in behavior for "message queued but not yet yielded at close time." This is exactly the gap requirement #4 asks to cover ("queued-message-drop-on-close interleaving").
- Per requirements goal #3 ("input typed while disconnected must be dropped, not queued... including input sitting in a MessageQueue superseded by a reconnect"), the fix will need either (a) `close()` clearing `this.queue = []` outright, or (b) a generation/epoch check so a superseded queue's remaining items are never handed to a *new* stream and the old stream is guaranteed torn down before any residual items can be read.

## 4. `useTerminalStream.ts` — the actual reconnect/epoch gap

File: `web-app/src/lib/hooks/useTerminalStream.ts` (419 lines). This is the composition hook that owns `messageQueueRef`, `abortControllerRef`, `isConnectedRef`, and `connect()`/`disconnect()`. It was not in the assigned reading list but is the necessary link between §1-3 and the requirements' "epoch guard" ask, so documenting it here.

- `connect()` (line 156) guards only with `if (isConnectedRef.current || !sessionId) return;` — **`isConnectedRef.current` is only set `true` after the *first message* arrives from the new stream** (line 214, inside the `for await` loop, `firstMessage` check). Between calling `connect()` and receiving that first message, `isConnectedRef.current` is still `false`. **There is no generation/epoch counter** — nothing stops a second `connect()` call (StrictMode double-invoke of the mount effect at line 390-399, or an external reconnect trigger firing while still `CONNECTING`) from running through the guard and creating a second `messageQueueRef.current = new MessageQueue()` (line 176) and a second `clientRef.current.streamTerminal(...)` (line 201) — **while the first stream's message-processing loop (`(async () => { for await (const msg of stream) ... })()`, line 209) is still alive and independently running against the old stream/old queue.**
- `disconnect()` (line 352) has a related gap: it unconditionally closes and nulls `messageQueueRef.current` (lines 363-366), but then only aborts `abortControllerRef.current` if `isConnectedRef.current` is true when the 1s timeout fires (lines 368-383) — if `isConnectedRef.current` is `false` (still `CONNECTING`, never got `firstMessage`), the function **resolves immediately without ever aborting**, leaving the old `AbortController`/old stream connection unaborted and dangling. A subsequent `connect()` then starts a brand-new stream while the old one is still technically live end-to-end (old server-side goroutine, old ConnectRPC call). This is precisely the class of race the requirements' hypothesis calls out for `streamViaControlMode`/`control_mode.go` — **two live server-side streams to the same tmux pane, one from each of two overlapping frontend connect attempts**, matching requirement #4's "overlapping-connect epoch guard" and "triple-rapid-connect no-throw" test asks.
- `pushMessageRef.current` (line 129) always resolves to `messageQueueRef.current?.push(msg)` — i.e., whichever queue is *currently* referenced, always the newest one. This means normal keystrokes during the overlap window go only to the new queue (not literally duplicated client-side), but it does **not** guarantee the old, now-orphaned stream/connection is torn down before it can still deliver anything already sitting in its own now-detached `MessageQueue.queue` array per §3, nor does it prevent the *server side* from having two live control-mode read/write paths bound to the same tmux session simultaneously.

## 5. Bug #164 / "infinite-resize loop" — searched, not the same bug

- No file named or referencing a numbered "bug #164" exists anywhere under `project_plans/` or `docs/bugs/`.
- The only "resize loop" hits are about the **xterm.js + `visualViewport` resize loop** (mobile keyboard show/hide), documented in:
  - `project_plans/ux-overhaul/requirements.md:170`
  - `project_plans/ux-overhaul/research/findings-pitfalls.md:92,141`
  - `project_plans/ux-overhaul/implementation/plan.md:417,557,577` (already has a shipped mitigation: `isFittingRef` guard + 400ms mobile debounce, Story 1.6.2)
  - `project_plans/stapler-squad-painpoints/research/findings-pitfalls.md:300` (notes Stapler already debounces resize)
- **This is a distinct mechanism** (CSS/layout resize-event feedback loop) from input replay, and none of these docs implicate the reconnect/input path. The `#164` reference in the requirements.md is speculative ("may share a root cause with... bug #164") — no existing documentation substantiates a shared root cause. Treat as unconfirmed; do not assume overlap without runtime evidence from Phase 0.

## 6. `project_plans/terminal-robustness/` and `project_plans/tmux-session-robustness/` — relevant prior art

**`tmux-session-robustness/research/synthesis.md`** documents an **already-identified, structurally analogous race**: a "double-start race" between the health checker and the restore path, both able to call `instance.Start(false)` on the same session concurrently (`health.go:85`). The prescribed/shipped fix pattern is a **per-instance `startMu sync.Mutex`** (Phase 2a, "eliminates the double-start race... via double-checked locking"). This is directly relevant precedent: the codebase already has one confirmed case of "two concurrent code paths both acting on the same tmux session because nothing serializes/fences them," and the established fix idiom in this codebase is a per-session mutex/guard, not a queue-level dedup. The phantom-keystroke fix (both client epoch-guard and any server-side `control_mode.go` fix) should likely follow the same idiom for consistency.

- Also noted in the same synthesis: "PTY stale-pointer race in `streamLoop` (`pty` snapshot escapes RLock before `SetReadDeadline`)" — deferred, mitigated only by a string-match error handler absorbing the symptom. Relevant because it shows this codebase has a precedent of "the real race is deferred and only the symptom is currently guarded" — exactly the pattern the phantom-keystroke bug's ticket describes (`capture-pane failed` / `CM input failed, retrying via subprocess` messages look like guard/fallback code reacting to a race rather than preventing it).

**`terminal-robustness/research/features.md`** — comparative survey of GoTTY/ttyd/Wetty/VS Code terminal reconnect models. Key takeaway relevant here: **none of the surveyed reference implementations replay buffered input on reconnect** — GoTTY/ttyd/Wetty give the user a fresh terminal view with no input replay, and even VS Code's more sophisticated "persistent terminal" pattern only replays **server-side output** into xterm.js, never client-typed input. This confirms input replay-on-reconnect has no legitimate design precedent anywhere surveyed — it is unambiguously a bug, not an intentional (if surprising) feature interaction, and supports a fix design that fully drops any input associated with a superseded connection rather than trying to salvage/redeliver it.

- No content in `terminal-robustness/research/architecture.md` or `pitfalls.md` specifically discusses input-side duplication/replay (that research thread was scoped to output/scrollback delivery, iOS text selection, mobile gestures — see its ADRs 012/013/014). Nothing there contradicts or duplicates this investigation.

## 7. Edge cases the fix's design must explicitly handle

Derived from the mechanisms above:

1. **Rapid reconnect while user is actively typing.** Keystrokes typed in the gap between old-connection-death and new-connection-established must be either (a) rejected/dropped client-side (current `sendInput`/`sendInputWithEcho` guards on `isConnectedRef` already do this correctly at the *dispatch* layer) or (b) if buffered anywhere pending flush, explicitly discarded on reconnect rather than flushed into the new stream — per §3, `MessageQueue.close()` currently fails to guarantee this.
2. **Browser tab backgrounded then foregrounded.** No visibility-change (`document.visibilitychange`) handling was found in `useTerminalStream.ts`/`useTerminalFlowControl.ts`/`TerminalOutput.tsx` in the reviewed ranges — a backgrounded tab's WebSocket may be throttled/closed by the browser without the app's own `disconnect()` path running first, meaning `isConnectedRef`/`messageQueueRef` state could go stale relative to the actual socket. Needs explicit handling or at minimum test coverage confirming the epoch guard is robust to an out-of-band close.
3. **Session paused server-side mid-send.** `sendInput`/`sendInputWithEcho` only check *client-side* `isConnectedRef`, not any server-reported "paused" state — matching the ticket's literal server error (`cannot send input to instance that has not been started or is paused`). A message can be client-side "connected" but still get rejected server-side; the fix must ensure this rejection cannot trigger any client-side retry/re-queue (confirmed: currently it doesn't, since there's no retry logic — but this should be locked in with a test, since a future change could add one).
4. **WebSocket `onclose` firing after a new `connect()` has already started.** This is the central gap identified in §4 — `connect()` has no generation/epoch counter, and `disconnect()`'s abort-on-timeout logic can skip aborting the old controller entirely when `isConnectedRef.current` is false. A stale `onclose`/stream-teardown from connection N-1 must be a no-op once connection N has started, and connection N-1's `MessageQueue`/`AbortController` must be unconditionally torn down (not conditionally, on an `isConnectedRef` check) whenever superseded.
5. **StrictMode double-invoke of the `connect` effect in dev.** The mount effect at `useTerminalStream.ts:390-399` (`connect()` then cleanup `disconnect()` then `connect()` again) exercises exactly the overlapping-connect path in §4 twice in immediate succession. Any epoch-guard fix must specifically pass this scenario (this maps directly to requirement #4's "triple-rapid-connect no-throw" test ask — StrictMode's double-invoke plus a genuine reconnect is effectively a triple-connect burst).

## Summary of confirmed findings (with file:line references)

| Layer | File | Finding |
|---|---|---|
| Dispatch | `TerminalOutput.tsx:511-538` | Single keystroke → single `sendInput`/`sendInputWithEcho` call; no retry logic. |
| Flow control | `useTerminalFlowControl.ts:311-401` | Both send functions gate on `isConnectedRef`; no replay of buffered/predicted input on SSP renegotiation (echo overlay is render-only). |
| Queue | `MessageQueue.ts:39-63` | `close()` does not clear `this.queue`; iterator loop condition `!this.closed \|\| this.queue.length > 0` explicitly drains post-close items if the generator is still being pumped. No test locks in close-with-pending-items behavior. |
| Connection lifecycle | `useTerminalStream.ts:156-183, 352-387` | `connect()` has no epoch/generation guard beyond `isConnectedRef` (false until first message); `disconnect()` skips aborting the old `AbortController` when `isConnectedRef.current` is false at timeout — old stream/goroutine can outlive the new one it was superseded by. |
| Prior art | `tmux-session-robustness/research/synthesis.md` | Codebase already has one confirmed "two concurrent paths acting on same session" bug (health-checker vs. restore double-start), fixed via per-instance `startMu sync.Mutex` — same idiom likely applicable here. |
| Prior art | `terminal-robustness/research/features.md` | No reference terminal implementation (GoTTY/ttyd/Wetty/VS Code) replays client input on reconnect — confirms replay is unambiguously a bug to eliminate, not a feature to preserve in degraded form. |
| Bug #164 | n/a | No documentation found connecting the resize-loop issue to input replay; treat the requirements' "may share a root cause" note as unconfirmed pending Phase 0 runtime evidence. |
