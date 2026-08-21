# Research: Feature Landscape — Terminal Input Batching

Agent 2 (Features), SDD Phase 2. Scope: prior art for coalescing/batching in this
codebase, exhaustive edge-case inventory of the current `sendInput`, additional edge
cases the requirements don't spell out, unstated user needs, and prior related work.

## 1. Existing coalescing/batching/throttling patterns in this codebase

No generic "batch small writes into fewer sends on a timer-or-threshold" utility
exists yet — each call site reinvented its own. Four patterns found, ranked by
closeness to what this feature needs:

1. **`RedrawThrottler`** — `web-app/src/lib/terminal/TerminalStreamManager.ts:42-92`.
   Closest structural analog: holds a single pending payload, flushes via
   `setTimeout(this.throttleMs)` (33ms / ~30fps) if not already scheduled, and a
   `cleanup()` method that force-flushes (called on unmount/teardown). Differs from
   what's needed here in one key way: it **coalesces by replacement** (`pendingRedraw
   = chunk` overwrites, no concatenation) because a full-screen redraw supersedes the
   previous one. Input batching needs **coalescing by concatenation** (bytes appended,
   not replaced) — so this class is a good shape reference (private pending buffer +
   timer handle + flush() + cleanup()) but not directly reusable as-is.
2. **`useTerminalMetrics`'s RAF-based output batching** —
   `web-app/src/lib/hooks/useTerminalMetrics.ts:24-98`. Batches *outbound-to-React*
   terminal output text via `requestAnimationFrame` with a `flush()` escape hatch.
   Same "buffer + flush trigger + manual flush" shape, different trigger (RAF vs. a
   configurable ms timer) and different direction (server→UI, not UI→server).
3. **`useDebounce.ts`** (`useDebounce`, `useDebouncedCallback`) — trailing-edge only,
   and each call **resets** the timer (`clearTimeout` + reschedule). This is the wrong
   primitive for input batching: under sustained fast typing, a trailing debounce
   would never fire (delay keeps resetting), meaning input could be held indefinitely
   with no upper bound. The feature needs a "start timer on first buffered byte, flush
   when it fires, don't reset it on subsequent bytes" (leading + max-wait) pattern —
   closer to `RedrawThrottler`'s "if not already scheduled, schedule" guard
   (`if (!this.throttleTimer) { ... }`) than to `useDebounce`'s reset-on-every-call
   behavior. **Do not reuse `useDebounce`/`useDebouncedCallback` for this.**
4. **Existing `resize()` throttle+defer** in the same file
   (`useTerminalFlowControl.ts:197-291`) — trailing-edge deferred send with
   value-dedup, already in the file the feature modifies. Same file, same author
   conventions (refs for timers, `pendingResizeTimerRef`, cleanup in the `useEffect`
   unmount handler at lines 54-65) — the most directly relevant local convention to
   mirror for a new `pendingBatchTimerRef`.

**Recommendation for Phase 3 (plan)**: model the new batching buffer as a small
private helper (ref-based state: `batchBufferRef: Uint8Array[]` or a growable byte
array, `batchTimerRef`, `batchedByteCountRef`) inside `useTerminalFlowControl.ts`,
structurally closest to `RedrawThrottler` (schedule-once-then-flush) but
concatenating instead of replacing, and register its cleanup in the *same* existing
`useEffect` teardown block (lines 54-65) that already clears
`pendingResizeTimerRef`/`paneRequestTimerRef` — one unmount effect, three timers, not
a new one.

## 2. `sendInput` (current, main branch) — every edge case a batching layer must respect

Full function: `web-app/src/lib/hooks/useTerminalFlowControl.ts:142-195`.

| # | Line(s) | Behavior | Why it matters to batching |
|---|---|---|---|
| 1 | 143 | `if (!pushMessageRef.current \|\| !isConnectedRef.current) return;` — silent no-op guard, checked **once per `sendInput` call**, at call time. | A batching layer that defers the actual `pushMessage` to a later timer tick must re-check this guard **at flush time**, not just at buffer-time — connection state can change between buffering a keystroke and the timer firing. If it doesn't, a flush after disconnect will throw/undefined-call or (worse) silently attempt a push through a stale `pushMessageRef`. |
| 2 | 145-146 | `TextEncoder().encode(input)` — batching operates on UTF-8 **bytes**, not JS string chars/code-points. | The 32-byte early-flush thresholds and the `PASTE_CHUNK_SIZE` composition must be counted in encoded bytes, matching this line's unit, not `input.length` (which is UTF-16 code units and would misjudge multi-byte IME/emoji input — directly relevant since the problem statement calls out IME composition). |
| 3 | 148-163 | Small-input fast path: builds one `TerminalData`/`TerminalInputSchema` message per call, wrapped in try/catch → `handleError`. | The coalesced flush must go through this exact same message-construction and error-handling shape (one `TerminalInputSchema` with the *concatenated* bytes) — not bypass `handleError`. |
| 4 | 168 | `const sessionIdAtStart = sessionId;` — session ID is captured **by closure value at call time**, not read live from a ref, because `sessionId` is a plain closure variable (component prop), not a ref. | This is the sharpest correctness edge case for batching: `sendInput` is a `useCallback` re-created whenever `sessionId` changes (dep array line 195). If a batch buffer persists *across* a `sendInput` call boundary via a ref (as it must, to accumulate across keystrokes), and the session changes mid-batch, the stale closure's `sessionId` used to build the pending flush's `TerminalData.sessionId` could be wrong unless the batching layer captures/refreshes `sessionId` per-buffered-chunk the same way the chunker does at line 168. See section 3 below — this needs explicit handling, not just "it'll work because refs are stable." |
| 5 | 172 | `if (sessionId !== sessionIdAtStart) return;` — mid-chunk abort compares the **live closure `sessionId`** (captured fresh each render via the hook's normal re-invocation) against the **frozen** `sessionIdAtStart`. | Same concern as #4: a live-typed batch flush firing on a `setTimeout` after the component has re-rendered with a new `sessionId` needs the equivalent "was this batch's session superseded" check, or bytes buffered for session A will be flushed with session B's ID (or dropped silently, which is also wrong — see AC5, "no silently dropped keystrokes"). |
| 6 | 174-175 | `inputBytes.slice(offset, offset + PASTE_CHUNK_SIZE)` — chunker always cuts at exact byte boundaries, indifferent to UTF-8 codepoint boundaries. | Pre-existing behavior (out of scope to fix per Non-Goals), but the batching layer's *input* to the chunker (a concatenated `Uint8Array`) must preserve this same "slice on encoded bytes" contract — i.e., batching should concatenate raw encoded byte arrays, not re-encode a re-joined string (re-encoding after `TextDecoder`/`TextEncoder` round-trip on a byte sequence that was cut mid-codepoint would throw or corrupt data; concatenating already-encoded `Uint8Array`s avoids that entirely). |
| 7 | 186-189 | On chunk-send error, `handleError(err); return;` — **aborts the remaining chunk queue**, does not retry, does not clear `offset`. | If a batched-then-chunked flush errors partway through, the same "stop, don't retry, don't silently continue" behavior must hold — but now there's an additional question the requirements don't cover: does the un-sent remainder of *that* batch (which was never chunked yet) also need to be discarded, or should it be surfaced as a drop? (See section 3, item on error-mid-flush.) |
| 8 | 190-192 | `setTimeout(sendChunk, CHUNK_DELAY_MS)` recursive scheduling — **no cleanup ref**: if the component unmounts mid-chunk-send, this `setTimeout` is not tracked anywhere and is **not cancelled** by the unmount effect (lines 54-65 only clear `pendingResizeTimerRef`/`paneRequestTimerRef`). | This is a **pre-existing gap**, not something batching introduces — worth flagging to Phase 3/4 explicitly since AC5 ("pending batched input is flushed on disconnect/unmount") sets a bar for the *new* batch timer that the *existing* chunk timer doesn't meet. If Phase 3 adds a `batchTimerRef` cleanup, consider (as a small adjacent fix, or explicitly deferred) whether `sendChunk`'s recursive timer should get the same `useRef` + unmount-clear treatment for consistency — otherwise the codebase will have two different "pending outbound timer" reliability guarantees five lines apart. |
| 9 | 195 | `useCallback` dependency array: `[sessionId, pushMessage, pushMessageRef, isConnectedRef, handleError]` — `sendInput` identity changes whenever `sessionId` changes. | Any batch-buffer ref/timer that `sendInput` closes over must not be recreated when `sendInput`'s identity changes (i.e., must live in a `useRef`, not be reinitialized inside the callback body) — otherwise a session switch would silently drop or orphan an in-flight batch's own tracking state (distinct from the *bytes*, which is item #4/#5's concern — this is about the tracking refs themselves surviving identity churn). |

## 3. Edge cases beyond the explicit requirements

1. **Session switch while a batch is pending (buffered, not yet flushed).** Not
   covered by any AC. Options: (a) flush immediately on session-change (treat it like
   unmount — safest, matches AC5's "no silently dropped keystrokes" spirit but for a
   different trigger), or (b) discard buffered bytes for the old session (loses
   input — likely wrong, since the user typed it into what they saw as an active
   terminal). Given `sendChunk`'s existing precedent of *aborting* (dropping) chunks
   silently on session change (edge case #5 above), there's tension: does batching
   introduce a *new* class of silent-drop-on-session-change that AC5 doesn't
   anticipate, or does it get a flush-before-switch guarantee that the existing
   chunker deliberately doesn't have? **This needs an explicit decision in Phase 3**,
   not an assumption — recommend flush-on-session-change for symmetry with
   flush-on-unmount, since both are "the buffer's destination is about to become
   invalid."
2. **Resize/reconnect firing mid-batch.** `resize()` and `sendInput()` are
   independent call paths sharing only `pushMessageRef`/`isConnectedRef`, not batch
   state — a resize during a pending input batch is unaffected either way (no shared
   mutable state), **except** that resize's own `paneRequestTimerRef` flow
   (`useTerminalFlowControl.ts:250-272`) requests a fresh `CurrentPaneRequest` 100ms
   after a resize send. If a batch is still buffering when that pane-refresh request
   goes out, the pane response could visually arrive and repaint the terminal
   *before* the user's buffered-but-unflushed keystrokes are echoed back by the PTY —
   a purely cosmetic ordering wrinkle (both messages are independent envelopes on the
   same WS connection so server-side ordering is still FIFO per connection; this is
   about perceived UI staleness, not correctness) but worth a note since it's exactly
   the kind of thing a manual QA pass would surface. Reconnect (disconnect → new
   `isConnectedRef.current` cycle) is the more serious case: if a batch's timer fires
   *after* `isConnectedRef.current` flips false but *before* the component unmounts
   (e.g., a transient network drop with auto-reconnect), edge case #1 in section 2
   means the flush attempt correctly no-ops (good) — but per AC5's "no silently
   dropped keystrokes," should that no-op instead route through the existing
   `onDrop` callback? See finding in section 5 below: an `onDrop` hook already exists
   on a sibling branch specifically for this class of silent-drop, and the batching
   design should either use it or explicitly note it as out of scope.
3. **Resync (`sendFlowControl`/`isResyncingRef`) racing a pending batch.** The resync
   state machine (`isResyncingRef`, `waitingForPaneResponseRef`,
   `requestFullResync()` at lines 80-135) and `sendInput` are **entirely independent
   message paths today** — nothing in current `sendInput` checks `isResyncingRef`
   before sending. That means: even *today*, a keystroke typed while a resync is
   in-flight is sent immediately, potentially arriving at the server interleaved with
   the resync's `CurrentPaneRequest`. Batching does not change this ordering
   relationship in principle (both are still separate envelopes sent via the same
   `pushMessage` → WS connection, so relative order between input and resync
   messages is whatever order the two independent call sites push them in, same as
   today) — but batching **does change the size of the ordering window**: a delayed
   flush (up to `inputBatchDelayMs`, e.g. 256ms at the slowest setting) means
   keystrokes typed just before a resync starts could now land *after* the resync's
   pane-refresh response is processed client-side, rather than before, purely because
   they were held in the buffer longer than the resync round-trip took. This is worth
   a pre-mortem note (Phase 4) even though it's "no worse than today's independent
   ordering" in the strict causal sense — the *practical* likelihood of visible
   reordering artifacts goes up with delay.
4. **Live setting change (`inputBatchDelayMs`) while a batch is already pending.**
   Not covered by any AC. If a user opens settings and changes the delay mid-session
   while bytes are buffered, the new value should not retroactively alter an
   in-flight timer's remaining wait in a way that could either (a) never fire
   (if the hook naively reads a stale closed-over delay each render and the timer
   was scheduled with the old value, this is actually fine — old delay honored) or
   (b) double-schedule if the change effect naively clears and reschedules a timer
   that has *already* had bytes appended after its original schedule. Simplest safe
   rule to hand to Phase 3: settings changes take effect for the **next** flush cycle
   only — don't reach into an in-flight timer and mutate its delay.
5. **Error mid-flush of a batched-then-chunked send** (composition of item #7 in
   section 2 with the new batch layer): if `sendChunk`'s recursive send throws on
   chunk 2 of 3 within a single *coalesced* flush, is that "1/3 of one keystroke's
   worth of bytes" or "1/3 of N coalesced keystrokes' worth of bytes" from the user's
   mental model? Because batching means a single flushed message can now represent
   several keystrokes, a mid-chunk failure loses a chunk of *multiple logical
   keystrokes* at once rather than a single paste. Not necessarily a blocker (paste
   already has this property today), but the blast radius of one `handleError` call
   is larger under batching and worth naming explicitly rather than silently
   inheriting.
6. **Buffer growth strategy / cost of concatenation.** If bytes are accumulated as
   an array of `Uint8Array` chunks and concatenated only at flush time (rather than
   repeatedly reallocating/copying into a growing buffer on every keystroke), this
   avoids O(n²) copy cost across many small keystrokes within one delay window — a
   minor implementation-detail edge case but worth flagging since `TextEncoder`
   allocation happens per-`sendInput`-call today (line 145) and a naive "concat into
   one big string then re-encode" approach would reintroduce the UTF-8
   codepoint-splitting hazard from section 2 item #6.

## 4. Unstated user needs

- **Latency-adaptive default is explicitly out of scope but worth flagging as a
  non-decision, not an oversight.** The requirements pin the default to `0` (off) —
  matching herdr-web's own default-off stance — which sidesteps the "should local
  sessions batch less than remote/high-latency ones" question entirely by not
  batching by default for anyone. No auto-detection of session locality/latency
  exists anywhere in this codebase today (no RTT measurement surfaced to the
  frontend that this reviewer could find via search of `web-app/src` for
  `latency`/`rtt`/`ping` in a terminal context) — so an adaptive default would be new
  infrastructure, correctly out of scope per the Non-Goals.
- **Perceptible lag risk at 32ms (the smallest non-zero option) is real but
  narrow, and the requirement text already hedges correctly.** Commonly cited HCI
  thresholds (INFERRED — general knowledge, not sourced from a citation in this
  repo): ~100ms is the widely-cited threshold below which a UI response feels
  "instantaneous" (Nielsen's *Usability Engineering*, 1993, response-time limits);
  below ~20ms is imperceptible sensorimotor synchrony for tightly-coupled
  input→feedback loops like typing. A 32ms coalescing window sits below the 100ms
  "instant" threshold but is within the range some latency-sensitive users (typists
  at 100+ WPM, or anyone comparing against truly-uncoalesced feel) could notice a
  faint softening of key-echo responsiveness, especially compounded with existing
  network RTT on non-local sessions. This isn't a reason to change the proposed
  option set (0/32/64/128/256ms matches herdr-web and keeps 0 as default), but the
  UI copy for the setting should probably say something like "may introduce a few
  ms of typing lag" rather than implying it's free, since the "never introducing
  perceptible input lag at the default setting" goal is a claim about the *default*
  (0ms, verified byte-for-byte-identical per AC1) — not about every option in the
  exposed range. **Flag for Phase 3 UI copy, not a scope change.**
- **No terminal settings UI currently persists `TerminalConfig` changes anywhere.**
  Grep for `saveTerminalConfig`/`loadTerminalConfig` call sites
  (`web-app/src/lib/config/terminalConfig.ts`,
  `web-app/src/components/sessions/XtermTerminal.tsx`) turned up **only
  `loadTerminalConfig` being read** in `XtermTerminal.tsx` (lines 210, 214, 220) —
  `saveTerminalConfig` has zero call sites in `web-app/src` outside its own
  definition file. There is no existing "terminal settings panel" component in this
  repo today that lets a user edit `fontSize`/`cursorBlink`/etc. and persist the
  change — AC6 assumes one exists ("user-editable from the terminal settings UI,"
  "following the same pattern already used for `fontSize`, `cursorBlink`") but the
  *pattern* referenced is currently read-only/config-file-only, not a live settings
  form. **This is a scope gap Phase 3 needs to resolve explicitly**: either (a) a
  minimal settings UI needs to be built as part of this item (expanding the stated
  single-file-core-change complexity-2 estimate), or (b) AC6 needs to be redefined
  to mean "the `TerminalConfig` type/schema supports the field and
  `loadTerminalConfig`/`saveTerminalConfig` round-trip it correctly," with the
  actual settings-panel UI treated as a separate follow-up item. Recommend surfacing
  this explicitly in the plan rather than silently building a new settings panel
  component under a complexity-2 estimate.

## 5. Related prior/in-flight work — do not re-solve

- **`recover/phantom-input-replay` branch, commit `d1803ae63`** (not yet merged to
  `main` — confirmed via `git merge-base --is-ancestor d1803ae63 HEAD` returning
  false) touches `sendInput`'s *exact* disconnected-guard line
  (`useTerminalFlowControl.ts:143`) to add an `onDrop?: () => void` callback fired
  when a keystroke is silently discarded because the connection is already known
  down:
  ```ts
  const sendInput = useCallback((input: string) => {
    if (!pushMessageRef.current || !isConnectedRef.current) {
      onDrop?.();
      return;
    }
    ...
  ```
  Commit message explicitly scopes `onDrop` to *only* `sendInput`'s guard ("the
  other five near-identical guards in this file ... are deliberately left
  unwired — out of scope for this 'phantom keystroke' ticket"). This is directly
  relevant prior art for section 3's open question ("should a batch flush that
  no-ops due to disconnect route through `onDrop`?") — when this branch merges (or
  if it merges *before* input-batching lands), the batching implementation should
  wire its own flush-time disconnected-guard through the same `onDrop` callback for
  consistency, since it's the same semantic event (a keystroke the user typed never
  reached the server). **Sequencing note for Phase 3**: check whether
  `recover/phantom-input-replay` has merged before starting implementation — if not,
  coordinate on the interface (don't duplicate `onDrop`-equivalent logic under a
  different name) or land input-batching first and have the phantom-replay branch's
  eventual merge/rebase account for the new buffered-flush call site.
- **`cd03c7347` (`feat(phantom-keystroke-replay): Phase 4 drop-and-signal UI`)** —
  same branch, adds a `dropBatchRef` merge guard in `useTerminalStream` so multiple
  drop occurrences landing in the same React batch (reconnect queue-close,
  disconnect() close, sendInput's guard) coalesce into one `InputDropBadge`
  increment instead of firing multiple times. This is a second, independent
  precedent for "coalesce rapid same-class events into one" in the same subsystem —
  worth reading (`web-app/src/lib/hooks/useTerminalStream.ts` and
  `web-app/src/components/sessions/InputDropBadge.tsx` on that branch) if Phase 3
  wants a second reference implementation of a coalescing merge-guard, though it
  coalesces *drop notifications*, not *input bytes*, so it's a pattern reference,
  not reusable code.
- **No other GitHub issues/PRs specifically about WebSocket message volume or
  terminal input batching found.** Searched git log on `main` for
  `PASTE_CHUNK_SIZE` — it was introduced pre-existing to this investigation (no
  single origin commit isolated in the scope of this research pass; the constant
  and its `CHUNK_DELAY_MS` sibling are stable in the current file with no open
  follow-up TODOs referencing them). No other branches or commits matching
  `batch`/`coalesce`/`debounce` in a terminal-input context were found beyond the
  four patterns cataloged in section 1 and the phantom-input-replay branch above.

## Summary

- No reusable "buffer + timer-or-threshold flush" utility exists yet;
  `RedrawThrottler` (`TerminalStreamManager.ts:42-92`) is the closest structural
  shape (schedule-once-then-flush + explicit `cleanup()`) but coalesces by
  *replacement*, not *concatenation* — `useDebounce.ts` is the wrong primitive
  (resets on every call, no upper bound) and should not be reused.
- Nine concrete edge cases in the current `sendInput`
  (`useTerminalFlowControl.ts:142-195`) constrain the design — most sharply: byte
  (not char) counting, the closure-captured-`sessionId` mid-batch problem
  (mirroring the existing chunker's `sessionIdAtStart` pattern), and a pre-existing
  unmount-cleanup gap in the chunk-send `setTimeout` that AC5 will make visibly
  inconsistent once the new batch timer *does* get proper cleanup.
- Two unresolved scope questions for Phase 3: (1) session-switch-mid-batch
  behavior (flush vs. drop — recommend flush, for symmetry with unmount) and (2)
  AC6 assumes a settings-panel UI exists to edit `TerminalConfig` when in fact only
  `loadTerminalConfig` has any call sites today — `saveTerminalConfig` is currently
  unused, so building a minimal settings UI may be in scope whether or not the
  complexity-2 estimate accounted for it. An unmerged branch
  (`recover/phantom-input-replay`, commit `d1803ae63`) adds an `onDrop` callback to
  this exact function that the batching flush path should probably route through
  for consistency — check merge status before implementation.
