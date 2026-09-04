# Pitfalls Research: Terminal Input Batching

Agent 4 (Pitfalls) — SDD Phase 2 research pass.

## 1. Stale-closure risk in a naive `setTimeout` batch buffer

`web-app/src/lib/hooks/useTerminalFlowControl.ts` already carries two documented
stale-closure mitigations that a batching timer must replicate:

- `pushMessageRef` (line 13, comment: *"Stored via ref to avoid stale closures"*) and the
  `pushMessage` helper (lines 73-76, comment: *"reads from ref to avoid stale closures"*)
  — every send path reads `pushMessageRef.current` at fire-time, not a captured value.
- `useTerminalStream.ts:133` — *"Bug Risk 3 mitigation: flow control reads
  `pushMessageRef.current` (not a stale closure)"* — confirms this was a real, previously
  found bug class in this exact file pairing, not a hypothetical.

**Concrete risk for a batch buffer**: `sendInput` is a `useCallback` with dependency array
`[sessionId, pushMessage, pushMessageRef, isConnectedRef, handleError]` (line 195). If
`sessionId` changes (session switch in the same mounted terminal component — this already
happens; see the existing `sessionIdAtStart` guard in the chunking path, lines 168-172),
React recreates `sendInput` with a new closure. A `setTimeout`-scheduled flush callback
created *before* that recreation still closes over the **old** `sessionId` and the **old**
`pushMessage` reference (`pushMessage` itself is stable across sessionId changes since it
only depends on `pushMessageRef`, but a batch flush needs the *current* `sessionId` for the
`TerminalDataSchema.sessionId` field, not the one captured when the timer was armed).

The existing large-input chunking path (lines 168-172) already solved this exact class of
bug for its own multi-tick send: it captures `sessionIdAtStart` and aborts
(`if (sessionId !== sessionIdAtStart) return`) rather than sending under a stale session ID
if a switch happens mid-chunk. **A batch timer must apply the identical pattern** — either
abort-and-drop-with-a-flush-of-whatever-was-buffered-under-the-old-session before switching,
or (safer, since silently dropping keystrokes violates Acceptance Criterion 5) flush the
pending buffer synchronously *before* the sessionId changes rather than relying on the timer
firing later under a mismatched ID. The safest structural fix is a `sessionIdRef` read at
flush-time (matching the `pushMessageRef` pattern) rather than trusting a value captured in
the `setTimeout` closure — reuse the ref-based idiom already established in this file, don't
invent a new one.

**Reviewer heuristic already stated by this file's own conventions**: any variable read
inside a `setTimeout` callback that isn't itself a `useRef` should be treated as suspect and
cross-checked against whether it can change between "timer armed" and "timer fires."

## 2. Ctrl-C / control-byte latency risk (flagged as out-of-scope-but-considered)

The requirements doc explicitly notes herdr-web's design (the design this feature is
modeled on) "doesn't appear to special-case" control bytes. This is a real, if narrow,
usability regression risk:

- `0x03` (ETX / Ctrl-C, SIGINT), `0x04` (EOT / Ctrl-D), `0x1a` (Ctrl-Z / SIGTSTP), and
  escape-sequence prefixes (`0x1b` for arrow keys, function keys) are all single-byte or
  short-byte-sequence inputs that xterm.js's `onData` delivers per-keystroke, identically to
  any other keystroke — **the batching layer has no protocol-level way to distinguish "user
  hammering Ctrl-C to kill a runaway process" from "user typing arrow-key repeat."**
- At the requirements doc's proposed default option set (0/32/64/128/256ms,
  §Acceptance Criterion 6), a user hitting Ctrl-C against a hung process would see it held
  in the batch buffer for up to `inputBatchDelayMs` before flush — at 256ms this is
  perceptible but arguably still tolerable; the bigger risk is compounding delay if the
  early-flush threshold (32 bytes) is *not* reached and the user is relying on the timer
  path only, or if flush is itself delayed by an unrelated event-loop stall.
- INFERRED (general terminal-emulator domain knowledge, not verified against this repo):
  most production terminal multiplexer/relay implementations that do input coalescing (e.g.
  mosh's local-echo prediction engine, various SSH connection multiplexers) special-case at
  minimum Ctrl-C to bypass batching/prediction entirely, precisely because SIGINT latency is
  a correctness-adjacent UX expectation (users expect it to be "as fast as physically
  possible"), not just a nice-to-have.

**Recommendation to flag for the plan phase** (not to implement here, since requirements
explicitly scope batching as a pure transport change with no protocol awareness): consider
forcing an immediate flush of the batch buffer plus the triggering byte whenever the
buffered/incoming byte sequence contains `0x03` (Ctrl-C), even though the *original* backlog
item and herdr-web's reference design don't call this out. If the plan phase declines this
(matching upstream scope), it should be a documented, explicit trade-off in `plan.md`, not a
silent gap — the same treatment this repo's `.claude/rules/` gives to "considered but
declined" categories elsewhere (e.g. autonomous-mode's flag-not-enum choice in
`session-creation-registry.md`).

## 3. React timer lifecycle pitfalls (StrictMode, unmount, leaks)

The existing cleanup `useEffect` (lines 54-65) is the canonical pattern to copy:

```ts
useEffect(() => {
  return () => {
    if (pendingResizeTimerRef.current) {
      clearTimeout(pendingResizeTimerRef.current);
      pendingResizeTimerRef.current = null;
    }
    if (paneRequestTimerRef.current) {
      clearTimeout(paneRequestTimerRef.current);
      paneRequestTimerRef.current = null;
    }
  };
}, []);
```

A new `inputBatchTimerRef` (or similar) **must** be added to this same cleanup block, not a
separate `useEffect` — a second independent cleanup effect risks ordering ambiguity (React
runs cleanup effects in the order they were declared during unmount, so a separate effect is
technically safe, but consolidating avoids the question entirely and matches the existing
one-cleanup-block convention in this file).

Two additional risks beyond simple `clearTimeout`-on-unmount:

- **StrictMode double-invoke (dev only)**: React 18 StrictMode mounts, unmounts, and
  remounts every component once in development to surface effect cleanup bugs. If the
  mount→unmount→remount cycle happens while a batch timer is pending, the *first* mount's
  cleanup must fire before the second mount's timer is armed, or two competing timers (one
  orphaned from the first mount, one from the remount) could both eventually fire and
  double-send buffered bytes. The existing `pendingResizeTimerRef`/`paneRequestTimerRef`
  pattern already handles this correctly (ref is nulled in both the timer callback itself
  *and* the cleanup effect), so mirroring that exact double-null discipline for the batch
  timer avoids introducing a new StrictMode-specific bug.
- **Flush-on-unmount is a functional requirement, not just cleanup hygiene** (Acceptance
  Criterion 5: "Pending batched input is flushed on disconnect/unmount"). This is stricter
  than the existing resize/pane-request timers, which are safe to simply *drop* on unmount
  (a resize that never got sent because the component unmounted is harmless — there's
  nothing to resize anymore). A dropped keystroke batch is data loss. The cleanup effect for
  the input-batch timer must call the **flush** function (send whatever bytes are currently
  buffered) rather than just `clearTimeout`+null, and must do so guarded by the same
  `pushMessageRef.current && isConnectedRef.current` checks every other send path in this
  file already uses (since by the time cleanup runs, the connection may already be torn
  down — sending into a dead ref should be a no-op, not a throw).

## 4. Testing pitfalls: real vs. fake timers

`web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` already establishes the
pattern to reuse verbatim:

```ts
// lines 65-75
describe('useTerminalFlowControl', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });
```

And the resize-throttle tests exercise the exact shape needed for batch-timer tests —
`jest.advanceTimersByTime(...)` to cross a threshold and assert the deferred send fires
(e.g. `resize` "should throttle to 200ms" at line 119, "should send follow-up
CurrentPaneRequest after 100ms delay" using `jest.advanceTimersByTime(100)` at line 148, and
the bounce-back-cancellation test using `jest.advanceTimersByTime(300)` at line 282).

**Risks to avoid when writing the new batching tests**:
- Do not use real timers (`setTimeout` + `await new Promise(r => setTimeout(r, N))`) for
  batch-delay assertions — flaky under CI load, and inconsistent with every other timer test
  already in this file. Use `jest.advanceTimersByTime(inputBatchDelayMs)` inside `act(...)`
  (React state updates inside a fake-timer-advanced callback need to be wrapped in `act` to
  avoid "not wrapped in act" warnings — check how the existing resize tests wrap their
  `advanceTimersByTime` calls, since `sendInput` doesn't currently call `act()` internally).
- Watch for **cross-test timer leakage**: since `jest.useFakeTimers()` is reset per-test via
  `beforeEach`/`afterEach`, a batch timer left pending at the end of one test (i.e. the test
  didn't call `jest.advanceTimersByTime` far enough to flush it, and didn't unmount the
  hook to trigger cleanup) is invisible under fake timers but could mask a real leak — the
  existing tests appear to `act()`-wrap+advance to explicit completion rather than leaving
  dangling timers, so new batching tests should do the same.
- The early-flush-by-byte-threshold path is *not* timer-driven (it fires synchronously once
  the buffered byte count crosses 32 bytes on a `sendInput` call, per the requirements doc),
  so those tests don't need fake timers at all — only the timer-driven coalescing and
  flush-on-unmount tests do. Mixing the two styles in the same `describe` block is fine as
  long as each test is clear about which path it's exercising.

## 5. Ordering / re-entrancy / data-integrity risk

Two concrete re-entrancy scenarios to design against:

- **Flush called from within the flush callback itself.** If the flush function is
  implemented as "concat buffered bytes → call `sendInput`-equivalent → clear buffer," and
  `sendInput` itself is the *entry point* that also does the buffering (per requirements:
  batching sits in front of the existing chunking path, not as a separate function), a naive
  implementation risks the flush's own internal call back into `sendInput` re-triggering the
  buffering logic recursively. The buffer-clear step must happen in a way that's atomic with
  respect to the send call — e.g. capture-and-clear the buffer into a local variable *before*
  handing bytes to the existing >512-byte chunking path, so any bytes arriving *during* the
  chunking path's own multi-tick `setTimeout(sendChunk, CHUNK_DELAY_MS)` sequence (line 191)
  start a **new** batch rather than corrupting the one currently being flushed.
- **A paste event firing while a keystroke batch is pending.** Per the requirements doc's
  own problem statement (lines 21-25), paste already arrives as one large `onData` call, so
  it hits the existing `> PASTE_CHUNK_SIZE` branch (line 148) directly — but only if the
  *pending batched bytes* aren't prepended first. If a user has a few keystrokes buffered
  (say, 10 bytes, under both the 32-byte early-flush threshold and the timer) and then
  pastes, the correct behavior is: buffered bytes + pasted bytes are sent **in order**, with
  the combined length now correctly routed through chunking if it exceeds 512 bytes. A bug
  here would either (a) send the paste first and the stale buffered keystrokes after
  (reordering — violates Acceptance Criterion 7), or (b) drop the buffered keystrokes
  entirely if the paste branch bypasses the batch buffer instead of flushing it first Both
  are data-integrity violations, not just cosmetic. The safest implementation flushes any
  pending batch **synchronously before** evaluating the size of a new incoming `sendInput`
  call, then treats the new call independently — never lets two "pending sends" coexist.
- **Double-counting / duplicate send risk**: if the byte-threshold check and the timer-based
  flush aren't mutually exclusive (e.g. the timer fires in the same tick the byte-threshold
  early-flush already ran), the same buffered bytes could be sent twice unless the buffer is
  cleared and the timer is cancelled as a single atomic step. This is the same "cancel then
  check" ordering bug class already documented and fixed for `resize`'s
  `pendingResizeTimerRef` cancellation (lines 203-212 include an explicit comment: cancelling
  the pending timer **must** run before the value-dedup early return, or a stale send can
  fire later with the wrong data) — the batching implementation should apply the identical
  discipline: clear the timer ref and clear the byte buffer together, before either path
  (early-flush or timer-flush) does anything else.

## 6. General domain pitfalls with keystroke/input batching in terminal emulators

(Broad domain knowledge — INFERRED, not verified against this codebase or web search per
task instructions.)

- **Local echo / prediction mismatches**: some terminal batching designs (mosh-style) pair
  coalescing with local echo prediction to hide latency from the user; this feature does
  *not* do that (xterm.js already renders keystrokes locally via its own terminal buffer
  independent of the round-trip, per typical PTY-over-WebSocket architectures), so the
  "typing feels laggy" risk here is specifically about *remote-side* effects (shell
  completion timing, readline redraw sync) rather than the user's own visible echo — worth
  confirming that stapler-squad's xterm.js integration does client-side echo and doesn't
  wait for a server round-trip to render typed characters, since if it does, batching would
  directly increase perceived typing latency by up to `inputBatchDelayMs`, not just backend
  message latency. This should be verified during Phase 2/3, not assumed.
- **IME composition interaction**: the requirements doc flags IME composition as a batching
  target (non-Latin input firing many small `onData` events). A common pitfall in this space
  is coalescing bytes *mid-composition* in a way that splits a multi-byte UTF-8 sequence
  across a flush boundary if the buffer is byte-oriented rather than codepoint-aware — the
  proposed design already encodes via `TextEncoder` per-call (line 145) and concatenates
  `Uint8Array`s, so as long as concatenation happens at the `Uint8Array` level (not by
  re-decoding/re-encoding partial strings), UTF-8 byte sequences remain intact regardless of
  where the flush boundary falls, since `TextEncoder.encode` per call always produces
  complete, valid UTF-8 for that call's string. Risk only exists if some future change tries
  to inspect/manipulate the buffered bytes as text before flush.
- **Backpressure interaction**: this file already has a `sendFlowControl`/pause mechanism
  (line 318) for output flow control from the server. Input batching is a client-to-server
  concern and orthogonal to that, but worth noting for the plan phase: a paused/congested
  connection (`isConnectedRef.current === false`) during a pending batch means the batch
  should hold (not drop) until reconnection, or explicitly drop with a warning — the current
  `sendInput` silently no-ops when disconnected (`if (!pushMessageRef.current ||
  !isConnectedRef.current) return`, line 143); a batched version needs to decide explicitly
  whether "disconnected while bytes are buffered" means drop-on-flush-attempt (current
  behavior, extended) or hold-until-reconnect (new behavior) — this is a design decision for
  Phase 3, not something to assume.
