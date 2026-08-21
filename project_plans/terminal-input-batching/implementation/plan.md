# Implementation Plan: terminal-input-batching

**Feature**: Coalesce rapid small `sendInput()` calls in `useTerminalFlowControl.ts` into fewer WebSocket messages behind an opt-in, default-off `inputBatchDelayMs` setting, without changing today's byte-identical behavior at the default or disturbing the existing >512-byte paste chunker.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: [ADR-001: Session-change mid-batch drops the pending buffer (abort), it does not flush-and-relabel](../decisions/ADR-001-session-change-mid-batch-abort.md)

---

## Step 0.5 — Creative Pass: Approaches Considered

**(a) Buffer inside `useTerminalFlowControl`'s own closure** (chosen). *Strength*: zero new hooks/files, reuses every existing stale-closure mitigation and cleanup-effect convention already in this exact function (`pushMessageRef`, `sessionIdAtStart`, the lines 54-65 unmount effect), and the batching-wraps-chunking hand-off (`flush()` → `sendBytes()`) falls out for free since both live in the same closure. *Weakness*: grows an already-large hook file (`useTerminalFlowControl.ts` is ~365 lines) by another ~50 lines.

**(b) Standalone `useInputBatcher` hook, composed into `useTerminalFlowControl`**. *Strength*: isolates the buffering logic behind its own small interface, unit-testable independent of the rest of flow control. *Weakness*: the buffer's only real job — handing a concatenated `Uint8Array` to the existing chunker — requires threading `sessionId`, `pushMessageRef`, `isConnectedRef`, and `handleError` through a second hook boundary for a piece of logic with exactly one call site; per `.claude/rules/interface-pollution-checklist.md`'s "speculative interface" and "forwarding-only wrapper" smells, a hook with one caller and no independent reuse case is exactly the abstraction this repo's own review checklist flags for rejection.

**(c) Push batching down into the WebSocket transport/message-queue layer** (`web-app/src/lib/transport/websocket-transport.ts` or `messageQueueRef`). *Strength*: would batch every outbound message type, not just input, and centralizes buffering below the protobuf-message boundary. *Weakness*: batching must compose with `PASTE_CHUNK_SIZE` chunking and per-session `sessionIdAtStart` semantics that only make sense for *input* messages specifically (a batched `TerminalResize` or `FlowControl` message would be actively wrong to coalesce) — pushing this down would require either unpacking/repacking already-built `TerminalData` protobuf messages by `data.case`, or adding input-specific special-casing at a layer that today has none, expanding the diff across two files instead of one function for no benefit the requirements ask for (Non-Goals explicitly scope this to the `sendInput` transport path, not the connection layer).

**Chosen: (a)**. Recorded as the "Batching placement" row in the Pattern Decisions table below; (b) and (c) are the Alternative Rejected entries.

---

## Step 1: System Type

Client-side transport buffering optimization inside a single React hook
(`useTerminalFlowControl.ts`). Not a service, not a data model, no proto/RPC surface,
no backend change — see Non-Goals in `requirements.md`.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `inputBatchDelayMs` | Option on `UseTerminalFlowControlOptions` (and, transitively, `UseTerminalStreamOptions` and `TerminalConfig`) controlling how long buffered input bytes wait before an automatic flush. `0` (default) disables batching entirely. | Persisted field name in `TerminalConfig`; also the runtime hook option name — same identifier at both layers. |
| `TERMINAL_INPUT_BATCH_DELAY_OPTIONS_MS` | `[0, 32, 64, 128, 256] as const` — the fixed set of valid millisecond values for `inputBatchDelayMs`, matching herdr-web's option set exactly. | Lives in `terminalConfig.ts`; used to clamp/validate a loaded value and as the source of truth for any future settings UI. |
| `TERMINAL_INPUT_BATCH_MAX_BYTES` | `32` — the early-flush byte threshold. Once buffered bytes reach this count, `flush()` fires immediately regardless of the timer. | Lives inside `useTerminalFlowControl.ts`, alongside `PASTE_CHUNK_SIZE`, matching that constant's existing in-hook-body placement (not module-level). |
| `PASTE_CHUNK_SIZE` | Existing constant (512 bytes) — the max size of a single outbound `TerminalInput` message before the existing chunker splits it. | Unchanged by this feature; batching composes in front of it. |
| `CHUNK_DELAY_MS` | Existing constant (10ms) — delay between successive chunk sends in the existing large-input chunker. | Unchanged. |
| `pendingBytesRef` | `useRef<Uint8Array[]>([])` — accumulated, not-yet-flushed input chunks, stored as an array (not a single re-allocated buffer) to avoid O(n²) copy cost across many small keystrokes in one window. | New ref, scoped to the hook instance. |
| `pendingByteLengthRef` | `useRef<number>(0)` — running total of bytes currently in `pendingBytesRef`, updated incrementally so the early-flush check never re-sums the array. | New ref. |
| `pendingSessionIdRef` | `useRef<string \| null>(null)` — the `sessionId` captured when the *current* batch started (first byte pushed onto an empty `pendingBytesRef`). Used at flush time to detect a session change mid-batch and abort (see ADR-001). | New ref; mirrors `sendChunk`'s `sessionIdAtStart` local, but must survive across multiple `sendInput` calls, so it needs to be a ref, not a local. |
| `flushTimerRef` | `useRef<ReturnType<typeof setTimeout> \| null>(null)` — handle for the pending timer-driven flush. Typed identically to `pendingResizeTimerRef`/`paneRequestTimerRef` per this file's established convention. | New ref. |
| `latestFlushRef` | `useRef<(() => void) \| null>(null)` — always holds the most recently created `flush` callback, kept in sync via a `useEffect(() => { latestFlushRef.current = flush }, [flush])`. Exists solely so the unmount cleanup effect (which has an empty `[]` dependency array and therefore only ever sees the *first* render's closures) calls the *current* `flush`, not a stale one closed over an old `sessionId`. | New ref; a stale-closure guard, same idiom as `pushMessageRef`. |
| `chunkTimerRef` | `useRef<ReturnType<typeof setTimeout> \| null>(null)` — handle for the existing chunk-sender's recursive `setTimeout(sendChunk, CHUNK_DELAY_MS)`, previously untracked. Added as a small collateral-debt fix (features.md finding #8) so it can be cancelled on unmount like every other deferred send in this file. | New ref; fixes a pre-existing gap, not itself part of the batching feature. |
| `sendBytes(bytes: Uint8Array)` | New `useCallback` — a verbatim extraction of `sendInput`'s current body (the `if (inputBytes.length <= PASTE_CHUNK_SIZE) {...} else {...}` block), taking already-encoded bytes. Both the batching-off path and `flush()` call this; it is the single hand-off point into the existing (unmodified) chunking logic. | Pure refactor — zero logic changes versus today's `sendInput` lines 148-194. |
| `flush()` | New `useCallback` — clears `flushTimerRef`, checks the `pendingSessionIdRef` guard (ADR-001), concatenates `pendingBytesRef` into one `Uint8Array`, clears all three pending refs, and calls `sendBytes(merged)`. | The single flush trigger, invoked from three places: the early-flush check, the timer callback, and the unmount cleanup effect (via `latestFlushRef`). |
| `concatUint8Arrays(chunks: Uint8Array[])` | New module-scope pure helper (outside the hook, no closure needed) — sums lengths once, allocates one `Uint8Array`, `.set()`s each chunk at its offset. Modeled on, not copied from, `websocket-transport.ts:375-380`'s `append()` (that function reallocates per-chunk; this one allocates once). | Single call site (inside `flush()`); not exported, not shared. |
| `EARLY_FLUSH` | Shorthand used in this plan for "the byte-threshold flush path" (as opposed to the timer-driven flush path) — both ultimately call the same `flush()` function. | Not a code symbol — just plan vocabulary. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Batching placement | Plain `useRef`/`useCallback` state inside `useTerminalFlowControl`'s existing closure — no new hook, no new file, no class | `research/architecture.md` §3, `research/features.md` §1 | (b) standalone `useInputBatcher` hook | Single call site, no independent reuse case — a dedicated hook here is a speculative/forwarding-only abstraction per `.claude/rules/interface-pollution-checklist.md` smells #1 and #4 |
| Batching placement (layer) | Sits in `useTerminalFlowControl.ts`, the same file/layer as the existing chunker | `research/architecture.md` §4 | (c) push batching into the WebSocket transport/message-queue layer | Batching must compose with input-specific `PASTE_CHUNK_SIZE`/`sessionIdAtStart` semantics that don't apply to other message types (resize, flow control); the transport layer has no such per-message-type special-casing today and shouldn't gain it for this |
| `sendInput`/chunker composition | Extract `sendBytes(bytes: Uint8Array)` as a verbatim lift of the existing immediate-send/chunked-send branch; both the batching-off path and `flush()` call it | `research/architecture.md` §4, `research/stack.md` §3 | Special-case batching inline inside `sendInput` without extracting a shared helper | Would either duplicate the chunking logic in two places or force `flush()` to call back into `sendInput` (re-entrancy risk per `research/pitfalls.md` §5) |
| Buffer accumulation strategy | `pendingBytesRef: Uint8Array[]` (array of chunks), single concatenation pass at flush time via `concatUint8Arrays` | `research/stack.md` §4 | A single growing `Uint8Array`, reallocated/copied on every push (the `websocket-transport.ts:375-380` `append()` model) | O(n²) copy cost across many small keystrokes within one batch window; that model is acceptable for low-frequency WS frame reassembly but wrong for a per-keystroke hot path |
| Flush trigger semantics | Fixed window: arm the timer once on the *first* buffered byte, never reset it on subsequent bytes (leading-edge, schedule-once-then-flush, matching `RedrawThrottler`'s `if (!this.throttleTimer) {...}` guard) | `research/features.md` §1 item 1, `research/architecture.md` §3 | Reuse `useDebounce.ts` / `useDebouncedCallback` (trailing-edge, resets on every call) | A reset-on-every-call debounce has no upper bound — sustained fast typing could hold the buffer indefinitely, violating the "coalesce within a *window*" intent of AC2 |
| Session-change-mid-batch handling | Capture `pendingSessionIdRef` at batch-start; abort (drop) the flush if `sessionId` has changed by flush-time, mirroring the existing chunker's `sessionIdAtStart` guard | ADR-001 | Flush the buffer immediately under the *new* session's id when a change is detected (features.md's original lean) | Would silently mislabel bytes typed for session A as session B's input — worse than dropping them; see ADR-001 for full reasoning |
| Unmount cleanup for the flush timer | Extend the *existing* single cleanup `useEffect` (lines 54-65) with a third block that flushes (not just cancels) — reads the current callback via `latestFlushRef` to avoid a stale-closure bug from the effect's empty `[]` dependency array | `research/pitfalls.md` §1, §3 | A second, independent `useEffect` for the batch timer | Consolidates all "cancel/flush deferred sends on unmount" logic in one place per this file's existing one-cleanup-block convention; avoids ordering ambiguity between two cleanup effects |
| Pre-existing chunk-timer unmount gap (`sendChunk`'s untracked `setTimeout`) | Fix in this change: track via `chunkTimerRef`, cancel (not flush — a dropped mid-paste chunk on unmount is pre-existing accepted behavior) in the same extended cleanup effect | `research/features.md` §2 item 8 (collateral debt, in-scope per user's standing "fix collateral debt found while working" instruction) | Leave it as a documented-but-unfixed gap | Same file, same function, ~10-line fix; leaving it would mean two structurally identical "pending outbound timer" mechanisms five lines apart with different cleanup guarantees for no reason |
| Ctrl-C / control-byte latency | Accept the risk; document it with an inline code comment, no special-casing of `0x03`/`0x04`/`0x1a` | `research/pitfalls.md` §2 | Force-flush on detecting a control byte in the buffered/incoming bytes | Requirements explicitly scope this as a pure transport change with no protocol awareness (Non-Goals); herdr-web's own reference design doesn't special-case control bytes either — implementing it here would be scope creep, so it's declared as an accepted trade-off instead |
| Settings UI exposure (AC6's "user-editable" clause) | Data-model + hook mechanism is committed scope this plan implements (Phase 1-2); a minimal settings UI touchpoint is Phase 3, explicitly marked optional/stretch | `research/ux.md` §1, §6; `research/features.md` §4 | Build a full settings panel as part of this item's core scope | No `TerminalConfig` settings UI exists anywhere in the codebase today (`saveTerminalConfig` has zero call sites) — building the *first* one is a materially larger, separate scope than a complexity-2 estimate anticipated; UX research recommends a toggle, not the full 5-value dropdown, if/when built |
| No new npm dependency | Hand-write, adapting herdr-web's `terminalInputTransport.ts` design (confirmed via live source fetch) from string-batching to byte-batching | `research/build-vs-buy.md` | `lodash.debounce` (with `maxWait`) | `maxWait` is time-only; it has no concept of payload size and cannot satisfy AC3's byte-threshold early-flush on its own — the byte-accounting logic has to be hand-written regardless, so adopting lodash would add a dependency without removing the risky part of the work |

---

## Observability Plan

- **Logs**: One `console.log` in `flush()` mirroring the existing verbosity level of `resize()`/`requestFullResync()` in this same file (e.g. `[useTerminalFlowControl] Flushing batched input: N bytes`), gated by nothing extra — consistent with every other dispatch function in this file, none of which are currently gated behind a debug flag.
- **Metrics**: None. No frontend telemetry/metrics pipeline exists for this kind of client-side transport counter in this codebase (confirmed by `research/stack.md`/`research/architecture.md` — no RTT/latency instrumentation surfaced to the frontend for terminal input). Out of scope to add one for this change.
- **Alerts**: None applicable — purely client-side, no server-side signal to alert on.

## Risk Control

- **Feature flag**: `inputBatchDelayMs` defaulting to `0` *is* the flag — batching code is entirely bypassed (not batch-of-one) when the option is at its default, per AC1 and the `if (inputBatchDelayMs <= 0) { sendBytes(inputBytes); return; }` guard at the top of the batching branch. No separate flag infrastructure needed.
- **Rollback procedure**: Revert the PR (or set `DEFAULT_TERMINAL_CONFIG.inputBatchDelayMs` back to `0` if a partial rollback is preferred) — additive-only change to `TerminalConfig` (new optional-with-default field), so `loadTerminalConfig()`'s existing `{...DEFAULT_TERMINAL_CONFIG, ...config}` merge means old persisted configs without the field continue to work with no migration needed.
- **Staged rollout**: Not applicable — frontend-only change shipped via normal PR merge; risk is inherently bounded by the default-off setting, so there is no user-facing behavior change until a user (or a future settings UI) explicitly opts in.

## Unresolved Questions

- [ ] Has `recover/phantom-input-replay` (commit `d1803ae63`) merged to `main` yet? It touches the exact `sendInput` disconnected-guard line this plan's `sendBytes` extraction (Task 1.2.1a) also touches. — blocks Task 1.2.1a — owner: implementer, run `git merge-base --is-ancestor d1803ae63 HEAD && echo merged || echo not-merged` before starting Phase 1; if merged, thread the extracted `sendBytes`'s guard through the same `onDrop?.()` callback that branch adds; if not merged, proceed as planned and flag the conflict for whichever branch rebases second.
- [ ] Is the Phase 3 "minimal settings UI touchpoint" epic in scope for the PR that ships Phase 1-2, or a separate follow-up item? — blocks Story 3.1.1 — owner: whoever picks up implementation/review; this plan's default position (per `research/ux.md`'s recommendation and the settings-UI gap both research passes independently found) is: ship Phase 1-2 only, file Phase 3 as a follow-up backlog item rather than including it in this PR.
- [ ] Should `flush()`'s `console.log` be removed/downgraded before merge, given this file's existing logs are already fairly verbose? — blocks nothing (cosmetic), owner: code reviewer at Task 1.3.1b review time — default: keep it, matching existing convention, unless review feedback says otherwise.

## Dependency Visualization

```
Epic 1.1  TerminalConfig field (inputBatchDelayMs + OPTIONS_MS)
    |
    v
Epic 1.2  Extract sendBytes(bytes) — pure refactor, verbatim lift
    |
    v
Epic 1.3  Batch buffer: accumulate -> early-flush -> flush() -> sendBytes()
    |         (Story 1.3.1 buffer+timer, 1.3.2 chunk composition,
    |          1.3.3 session-change abort per ADR-001)
    v
Epic 1.4  Unmount flush-then-clear (1.4.1) + chunk-timer cleanup debt fix (1.4.2)
    |
    v
Epic 1.5  Wire inputBatchDelayMs: TerminalOutput -> useTerminalStream -> useTerminalFlowControl
    |
    v
Epic 2.1  AC8 test-coverage closeout (default-off, ordering, full regression run)
    |
    v
Epic 3.1  [OPTIONAL/STRETCH] Minimal settings toggle in Appearance tab
```

---

## Phase 1: Core Batching Mechanism

### Epic 1.1: TerminalConfig Data Model

**Goal**: `inputBatchDelayMs` exists as a validated, persisted field on `TerminalConfig`, satisfying the data-model half of AC6.

#### Story 1.1.1: Add and validate `inputBatchDelayMs` on `TerminalConfig`
**As a** developer wiring up terminal input batching, **I want** `inputBatchDelayMs` to round-trip through `loadTerminalConfig`/`saveTerminalConfig` like every other terminal setting, **so that** the value persists across sessions the same way `fontSize`/`cursorBlink` already do.

**Acceptance Criteria**:
- `TerminalConfig` has an `inputBatchDelayMs: number` field, defaulting to `0` in `DEFAULT_TERMINAL_CONFIG`.
  - *Given* a fresh browser with no `stapler-squad-terminal-config` localStorage key, *When* `loadTerminalConfig()` is called, *Then* the returned object includes `inputBatchDelayMs: 0`.
- A stored config predating this change (no `inputBatchDelayMs` key at all) still loads cleanly.
  - *Given* `localStorage.getItem("stapler-squad-terminal-config")` returns `'{"fontSize":18,"cursorBlink":false}'` (no `inputBatchDelayMs` key — simulating a config saved before this feature shipped), *When* `loadTerminalConfig()` is called, *Then* the returned object has `inputBatchDelayMs: 0` (merged from `DEFAULT_TERMINAL_CONFIG`) alongside `fontSize: 18` and `cursorBlink: false`.
- A saved non-default value round-trips.
  - *Given* `saveTerminalConfig({ inputBatchDelayMs: 128 })` is called, *When* `loadTerminalConfig()` is called afterward, *Then* it returns `inputBatchDelayMs: 128`.

**Files**: `web-app/src/lib/config/terminalConfig.ts`

##### Task 1.1.1a: Add the field, default, and options constant (~4 min)
- In `TerminalConfig` interface, add `inputBatchDelayMs: number;` with a doc comment: "Milliseconds to buffer rapid small keystrokes before sending as one WebSocket message. 0 = off (default, byte-for-byte identical to unbatched sends). One of `TERMINAL_INPUT_BATCH_DELAY_OPTIONS_MS`."
- Add `inputBatchDelayMs: 0,` to `DEFAULT_TERMINAL_CONFIG`.
- Export `export const TERMINAL_INPUT_BATCH_DELAY_OPTIONS_MS = [0, 32, 64, 128, 256] as const;` near `DEFAULT_TERMINAL_CONFIG`, matching herdr-web's option set (confirmed accurate against live source in `research/build-vs-buy.md` §4).
- Files: `web-app/src/lib/config/terminalConfig.ts`

##### Task 1.1.1b: Clamp/validate in `loadTerminalConfig` (~4 min)
- In `loadTerminalConfig`'s returned object, add: `inputBatchDelayMs: TERMINAL_INPUT_BATCH_DELAY_OPTIONS_MS.includes(config.inputBatchDelayMs as typeof TERMINAL_INPUT_BATCH_DELAY_OPTIONS_MS[number]) ? config.inputBatchDelayMs! : DEFAULT_TERMINAL_CONFIG.inputBatchDelayMs,` — mirrors the existing `fontSize`/`scrollbackLines` clamp pattern immediately above it, but validates against the fixed option set (a discrete enum) rather than a numeric range.
- Files: `web-app/src/lib/config/terminalConfig.ts`

---

### Epic 1.2: Extract `sendBytes` (Pure Refactor)

**Goal**: `sendInput`'s existing immediate-send/chunked-send logic is lifted verbatim into a reusable `sendBytes(bytes: Uint8Array)` helper, with zero behavior change — the prerequisite for both the batching-off passthrough (AC1) and the batching-on flush hand-off (AC4).

#### Story 1.2.1: Extract `sendBytes(bytes: Uint8Array)` from `sendInput`
**As a** developer adding batching, **I want** the existing chunking logic isolated behind a byte-array entry point, **so that** both the direct (batching-off) path and the new `flush()` function can call the identical, already-tested send logic.

**Acceptance Criteria**:
- Behavior at `inputBatchDelayMs <= 0` (this task's scope — batching option doesn't exist yet, so this is really "behavior is unchanged, period") is byte-for-byte identical to today.
  - *Given* `sendInput('hello')` is called (no batching logic exists yet at this task), *When* it runs, *Then* `pushMessage` is called exactly once with `msg.sessionId === 'test-session'`, `msg.data.case === 'input'`, and `msg.data.value.data` deep-equal to `Uint8Array.from([104, 101, 108, 108, 111])` (UTF-8 for "hello") — identical to the pre-refactor assertion.

**Files**: `web-app/src/lib/hooks/useTerminalFlowControl.ts`, `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

##### Task 1.2.1a: Extract the body into `sendBytes` (~5 min)
- Before starting, run `git merge-base --is-ancestor d1803ae63 HEAD` to check whether `recover/phantom-input-replay` has merged (see Unresolved Questions) — if merged, thread its `onDrop?.()` call through the extracted guard; if not, proceed without it.
- Rename the guarded block (current lines 148-194: the `if (inputBytes.length <= PASTE_CHUNK_SIZE) {...} else {...}` immediate/chunked logic, including the `pushMessageRef`/`isConnectedRef` guard at its top) into `const sendBytes = useCallback((bytes: Uint8Array) => { if (!pushMessageRef.current || !isConnectedRef.current) return; ...same body, using `bytes` instead of `inputBytes`... }, [sessionId, pushMessage, pushMessageRef, isConnectedRef, handleError]);`.
- `sendInput` becomes: `const sendInput = useCallback((input: string) => { const encoder = new TextEncoder(); const inputBytes = encoder.encode(input); sendBytes(inputBytes); }, [sendBytes]);` — no batching logic yet, this task is a pure extraction.
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.2.1b: Regression-gate the extraction (~2 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="useTerminalFlowControl.test"` and confirm both existing `sendInput` tests ("should call pushMessage with correct TerminalData", "should not send when disconnected") still pass unchanged — this is the gate proving the extraction introduced no behavior change before any batching logic is added on top.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` (verification only, no edits)

---

### Epic 1.3: Batch Buffer, Early Flush, and Chunk Composition

**Goal**: `inputBatchDelayMs > 0` coalesces consecutive `sendInput` calls into fewer `sendBytes` invocations, flushes early at the 32-byte threshold, preserves byte order, and composes transparently with the existing >512-byte chunker (AC2, AC3, AC4, AC7).

#### Story 1.3.1: Buffer, timer, and early-flush mechanics
**As a** user on a constrained connection who has opted into batching, **I want** my rapid small keystrokes coalesced into one message per short window, **so that** fewer WebSocket frames are sent for the same input.

**Acceptance Criteria**:
- Consecutive small `sendInput` calls within the delay window coalesce into one message (AC2).
  - *Given* `inputBatchDelayMs: 64` and `sendInput('ab')` then `sendInput('cd')` are called back-to-back (no timer advance in between), *When* `jest.advanceTimersByTime(64)` runs inside `act(...)`, *Then* `pushMessage` is called exactly once, with `msg.data.value.data` deep-equal to `Uint8Array.from([97, 98, 99, 100])` (i.e. "abcd" — bytes from both calls, in order, in one message).
- Buffered bytes flush immediately once they cross the byte threshold, without waiting for the timer, and the pending timer is cancelled so it doesn't double-send (AC3).
  - *Given* `inputBatchDelayMs: 64`, `sendInput('x'.repeat(20))` is called first (buffers 20 bytes, arms a 64ms `flushTimerRef`), then `sendInput('y'.repeat(20))` is called before the timer fires (pushes the running total to 40, crossing `TERMINAL_INPUT_BATCH_MAX_BYTES = 32`), *When* the second `sendInput` call returns, *Then* `pushMessage` has already been called exactly once, synchronously (no `advanceTimersByTime` needed), with `msg.data.value.data.length === 40`, and `jest.getTimerCount() === 0` (the 64ms timer was cancelled by the early flush — advancing time further produces no additional `pushMessage` calls).

**Files**: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.3.1a: Add option, constant, and refs (~3 min)
- Add `inputBatchDelayMs = 0,` to the destructured hook parameters (with default), and `inputBatchDelayMs?: number;` to `UseTerminalFlowControlOptions`.
- Add `const TERMINAL_INPUT_BATCH_MAX_BYTES = 32;` inside the hook body, next to `PASTE_CHUNK_SIZE`/`CHUNK_DELAY_MS`.
- Add four new refs alongside the existing ones (lines 42-49): `pendingBytesRef = useRef<Uint8Array[]>([])`, `pendingByteLengthRef = useRef(0)`, `pendingSessionIdRef = useRef<string | null>(null)`, `flushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)`.
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.3.1b: Implement `flush()` and `concatUint8Arrays` (~5 min)
- Above the hook (module scope, no closure needed), add:
  ```ts
  function concatUint8Arrays(chunks: Uint8Array[]): Uint8Array {
    const total = chunks.reduce((sum, c) => sum + c.length, 0);
    const merged = new Uint8Array(total);
    let offset = 0;
    for (const chunk of chunks) {
      merged.set(chunk, offset);
      offset += chunk.length;
    }
    return merged;
  }
  ```
- Inside the hook, add:
  ```ts
  const flush = useCallback(() => {
    if (flushTimerRef.current) {
      clearTimeout(flushTimerRef.current);
      flushTimerRef.current = null;
    }
    if (pendingBytesRef.current.length === 0) return;
    // ADR-001: a session change since this batch started means these bytes
    // would be mislabeled if sent now — drop, mirroring sendChunk's existing
    // sessionIdAtStart abort, rather than sending under the new session.
    if (pendingSessionIdRef.current !== null && pendingSessionIdRef.current !== sessionId) {
      pendingBytesRef.current = [];
      pendingByteLengthRef.current = 0;
      pendingSessionIdRef.current = null;
      return;
    }
    const merged = concatUint8Arrays(pendingBytesRef.current);
    pendingBytesRef.current = [];
    pendingByteLengthRef.current = 0;
    pendingSessionIdRef.current = null;
    console.log(`[useTerminalFlowControl] Flushing batched input: ${merged.length} bytes`);
    sendBytes(merged);
  }, [sessionId, sendBytes]);
  ```
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.3.1c: Wire `sendInput`'s batching branch (~5 min)
- Replace `sendInput`'s body (from Task 1.2.1a) with:
  ```ts
  const sendInput = useCallback((input: string) => {
    const encoder = new TextEncoder();
    const inputBytes = encoder.encode(input);

    if (inputBatchDelayMs <= 0) {
      sendBytes(inputBytes);
      return;
    }

    if (pendingBytesRef.current.length === 0) {
      pendingSessionIdRef.current = sessionId;
    }
    pendingBytesRef.current.push(inputBytes);
    pendingByteLengthRef.current += inputBytes.length;

    if (pendingByteLengthRef.current >= TERMINAL_INPUT_BATCH_MAX_BYTES) {
      flush();
      return;
    }

    if (!flushTimerRef.current) {
      flushTimerRef.current = setTimeout(flush, inputBatchDelayMs);
    }
  }, [inputBatchDelayMs, sessionId, sendBytes, flush]);
  ```
  Note the early-flush byte check runs *after* pushing the new bytes (per `research/pitfalls.md` §5 — checking before append would misjudge the threshold), and an already-armed timer is never reset on subsequent bytes (leading-edge, not debounce — see Pattern Decisions).
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.3.1d: Write the coalescing test (~4 min)
- Add test `'coalesces two sendInput calls within the delay window into one message'` per Story 1.3.1's first Given/When/Then, in a new `describe('sendInput batching')` block within `useTerminalFlowControl.test.ts`, passing `inputBatchDelayMs: 64` via `createTestOptions({ inputBatchDelayMs: 64 })` (works automatically — `createTestOptions`'s `...overrides` spread already threads arbitrary new keys through, no helper change needed).
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

##### Task 1.3.1e: Write the early-flush test (~4 min)
- Add test `'flushes immediately once buffered bytes cross the 32-byte threshold, cancelling the pending timer'` per Story 1.3.1's second Given/When/Then, asserting the synchronous `pushMessage` call plus `jest.getTimerCount() === 0` after the second `sendInput` call, and no further `pushMessage` calls after `jest.advanceTimersByTime(64)`.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

#### Story 1.3.2: Batched flush composes with the existing >512-byte chunker
**As a** user who pastes while batching is on, **I want** a flush that happens to exceed 512 bytes to still be split into safe chunks, **so that** tmux's PTY write buffer is never overrun.

**Acceptance Criteria**:
- A flush larger than `PASTE_CHUNK_SIZE` (512) still routes through the existing chunked-send path (AC4).
  - *Given* `inputBatchDelayMs: 256`, 10 bytes are already buffered (`sendInput('0123456789')`, under the 32-byte early-flush threshold, timer armed but not yet fired), *When* `sendInput('P'.repeat(600))` is then called (a 600-byte paste, pushing the running total to 610 bytes), *Then* the early-flush threshold fires synchronously within that same call (610 ≥ 32), `flush()` calls `sendBytes` with the full 610-byte merged array, and — because 610 > `PASTE_CHUNK_SIZE` — `sendBytes` takes its existing chunked branch: `pushMessage` is called twice total, first with a 512-byte chunk, then (after `jest.advanceTimersByTime(CHUNK_DELAY_MS)`, i.e. 10ms) with the remaining 98-byte chunk — and the first 10 bytes ("0123456789") are the leading bytes of the first chunk, not dropped or reordered.

**Files**: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` (no new production code — this falls out of Story 1.3.1's `flush()` → `sendBytes()` hand-off by construction, per `research/architecture.md` §4)

##### Task 1.3.2a: Write the chunk-composition test (~5 min)
- Add test `'a flushed batch exceeding PASTE_CHUNK_SIZE still gets split into ≤512-byte chunks'` per the Given/When/Then above; assert exactly 2 `pushMessage` calls, first chunk length 512, second chunk length 98, second call only observed after `act(() => jest.advanceTimersByTime(10))`.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

#### Story 1.3.3: Session-change mid-batch aborts the pending flush (ADR-001)
**As a** maintainer of this hook, **I want** a batch buffer to silently drop rather than mislabel its bytes if the session changes before it flushes, **so that** the same safety guarantee the existing chunker already provides also covers the new buffer.

**Acceptance Criteria**:
- A pending batch is dropped, not sent under the new session's id, if `sessionId` changes before the flush timer fires.
  - *Given* `inputBatchDelayMs: 128`, `sendInput('a')` is called while the hook's `sessionId` is `'session-A'` (buffers 1 byte, `pendingSessionIdRef.current` becomes `'session-A'`, `flushTimerRef` armed for 128ms), and the hook is then `rerender()`-ed with `sessionId: 'session-B'` (same instance, no unmount), *When* `jest.advanceTimersByTime(128)` runs, *Then* `pushMessage` is **not** called (the flush aborted per ADR-001), and `pendingBytesRef`/`pendingByteLengthRef`/`pendingSessionIdRef` are all reset — a subsequent `sendInput('b')` call under `'session-B'` starts a fresh batch (verified by a follow-up `sendInput` + `advanceTimersByTime` producing exactly one `pushMessage` call tagged `sessionId: 'session-B'`).

**Files**: `web-app/src/lib/hooks/useTerminalFlowControl.ts`, `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

##### Task 1.3.3a: Guard already implemented — confirm placement (~1 min)
- The `pendingSessionIdRef` capture (on starting a new batch) and the abort check in `flush()` were both already added in Tasks 1.3.1a/1.3.1b. This task is a verification-only checkpoint: re-read the two spots to confirm the guard is present and matches ADR-001 before writing the test in Task 1.3.3b — no code change expected.
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts` (verification only)

##### Task 1.3.3b: Write the session-change abort test (~5 min)
- Add test `'drops a pending batch flush if sessionId changes before the flush timer fires (mirrors existing chunker sessionIdAtStart abort)'` per Story 1.3.3's Given/When/Then, using RTL's `rerender()` to change `sessionId` on the same hook instance without unmounting (same pattern as the existing `resize()`/`requestFullResync()` tests use for their own `sessionId`-stability assumptions).
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

---

### Epic 1.4: Unmount Flush and Collateral-Debt Cleanup

**Goal**: Pending batched input is flushed (not just cancelled) on unmount (AC5), and the pre-existing untracked chunk-send timer gets the same cancel-on-unmount treatment the rest of this file's deferred sends already have.

#### Story 1.4.1: Flush-then-clear on unmount
**As a** user closing or navigating away from a session, **I want** any keystrokes still sitting in the batch buffer to be sent before the component tears down, **so that** typed input is never silently dropped just because I closed the panel at the wrong moment.

**Acceptance Criteria**:
- Unmounting with a pending batch flushes it (AC5).
  - *Given* `inputBatchDelayMs: 128`, `sendInput('x')` is called (1 byte buffered, `flushTimerRef` armed for 128ms, not yet fired), *When* the hook is unmounted (`result.current` torn down via RTL's `unmount()`) before the 128ms elapses, *Then* `pushMessage` is called exactly once with `msg.data.value.data` deep-equal to `Uint8Array.from([120])` (UTF-8 for "x") — flushed by the cleanup effect, not by the timer.
- The cleanup-triggered flush doesn't double-send if the (already-cleared) timer would otherwise have fired.
  - *Given* the same setup as above, *When* `jest.advanceTimersByTime(128)` is called *after* unmount, *Then* `pushMessage`'s call count is unchanged from immediately after unmount (still exactly 1) — the timer was cancelled as part of the flush-then-clear cleanup, so it cannot fire a second time.

**Files**: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.4.1a: Add `latestFlushRef` and sync effect (~3 min)
- Add `const latestFlushRef = useRef<(() => void) | null>(null);` alongside the other new refs.
- Add `useEffect(() => { latestFlushRef.current = flush; }, [flush]);` right after `flush`'s definition. This exists because the unmount cleanup effect (Task 1.4.1b) has an empty `[]` dependency array and therefore only ever closes over the *first* render's `flush` — reading through `latestFlushRef.current` at cleanup time guarantees the *current* `flush` (current `sessionId`) is used, avoiding the stale-closure class of bug this file's own conventions (`pushMessageRef`'s doc comment) already guard against elsewhere.
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.4.1b: Extend the unmount cleanup effect to flush (~4 min)
- In the existing `useEffect(() => { return () => {...} }, [])` (lines 54-65), add a third block after the existing `pendingResizeTimerRef`/`paneRequestTimerRef` cancellation:
  ```ts
  if (flushTimerRef.current) {
    clearTimeout(flushTimerRef.current);
    flushTimerRef.current = null;
  }
  latestFlushRef.current?.();
  ```
  Add a comment: "Unlike resize/pane-request timers (safe to drop on unmount), buffered input bytes are user keystrokes the server has never seen (AC5) — cleanup must flush, not just cancel." `flush()` itself already no-ops safely if `pendingBytesRef` is empty or if `pushMessageRef.current`/`isConnectedRef.current` are falsy (via `sendBytes`'s existing guard), so no extra guard is needed here.
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.4.1c: Add the documented Ctrl-C accepted-risk comment (~2 min)
- Add a short comment near the batching branch in `sendInput` (Task 1.3.1c's code): "Batching has no protocol awareness — a Ctrl-C (0x03) typed while a batch is buffering waits up to `inputBatchDelayMs` before flushing, same as any other byte. Accepted trade-off, matching herdr-web's reference design (no control-byte special-casing); out of scope per requirements.md Non-Goals." This is documentation only, no behavior change.
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.4.1d: Write the flush-on-unmount tests (~5 min)
- Add both tests from Story 1.4.1's Given/When/Then (flush-on-unmount, and no-double-send-after-unmount) to `useTerminalFlowControl.test.ts`, using RTL's `unmount()` return value from `renderHook`.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

#### Story 1.4.2: Fix the pre-existing untracked chunk-send timer (collateral debt)
**As a** maintainer, **I want** the existing >512-byte chunk sender's `setTimeout` handle tracked and cancelled on unmount, **so that** this file doesn't carry two structurally identical "pending outbound timer" mechanisms with different cleanup guarantees five lines apart.

**Acceptance Criteria**:
- A pending mid-chunk send is cancelled on unmount (not a requirements.md AC — collateral debt fix per `research/features.md` finding #8, scoped in per this repo's standing "fix collateral debt found while working" convention).
  - *Given* a 600-byte input triggers `sendBytes`'s chunked branch (chunk 1 of 2 sent immediately, chunk 2 scheduled via `setTimeout(sendChunk, 10)`), *When* the hook unmounts before the 10ms elapses, *Then* `jest.advanceTimersByTime(10)` after unmount produces no additional `pushMessage` calls (previously, this `setTimeout` was untracked and would have fired against the torn-down instance).

**Files**: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.4.2a: Track the chunk-send timer (~4 min)
- Add `const chunkTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);` alongside the other refs.
- In `sendBytes`'s chunked branch (the `sendChunk` closure), add `chunkTimerRef.current = null;` at the top of `sendChunk` (mirroring `paneRequestTimerRef`'s existing self-null-on-fire pattern at line 251), and change `setTimeout(sendChunk, CHUNK_DELAY_MS);` to `chunkTimerRef.current = setTimeout(sendChunk, CHUNK_DELAY_MS);`.
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.4.2b: Cancel it in the extended unmount cleanup effect (~2 min)
- In the same cleanup block extended in Task 1.4.1b, add: `if (chunkTimerRef.current) { clearTimeout(chunkTimerRef.current); chunkTimerRef.current = null; }`. Cancel-only (no flush) — a dropped mid-paste chunk on unmount was already the accepted pre-existing behavior; only the "never cancelled, could fire against a torn-down instance" leak is being fixed.
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 1.4.2c: Write the chunk-timer-cancelled-on-unmount test (~4 min)
- Add test `'cancels a pending chunk-send timer on unmount (no fire against a torn-down instance)'` per Story 1.4.2's Given/When/Then.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

---

### Epic 1.5: Wire `inputBatchDelayMs` Through the Hook Chain

**Goal**: The persisted `TerminalConfig.inputBatchDelayMs` value actually reaches `useTerminalFlowControl` at runtime (the mechanism half of AC6 — no settings UI required for this epic).

#### Story 1.5.1: Thread the option from `TerminalOutput` through `useTerminalStream` to `useTerminalFlowControl`
**As a** developer, **I want** `TerminalConfig.inputBatchDelayMs` to flow all the way to the hook that uses it, **so that** a persisted (even if only manually-set, pending a settings UI) value takes effect.

**Acceptance Criteria**:
- A persisted `inputBatchDelayMs` value reaches `useTerminalFlowControl` (AC6, mechanism half).
  - *Given* `loadTerminalConfig()` returns `{ ...DEFAULT_TERMINAL_CONFIG, inputBatchDelayMs: 64 }` (e.g. from a prior `saveTerminalConfig({ inputBatchDelayMs: 64 })` call, or a manually-edited localStorage value), *When* `TerminalOutput` mounts and calls `useTerminalStream({ ..., inputBatchDelayMs: loadTerminalConfig().inputBatchDelayMs })`, *Then* `useTerminalFlowControl` receives `inputBatchDelayMs: 64` and `sendInput` batches accordingly — verified at the `useTerminalStream`/`useTerminalFlowControl` prop-threading level (unit test), not a full E2E render of `TerminalOutput`.

**Files**: `web-app/src/lib/hooks/useTerminalStream.ts`, `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.5.1a: Thread the option through `useTerminalStream` (~3 min)
- Add `inputBatchDelayMs?: number;` to `UseTerminalStreamOptions`, destructure with `inputBatchDelayMs = 0,` default (matching the other destructured-with-default options like `scrollbackLines = 1000`), and pass it into the `useTerminalFlowControl({...})` call (~line 144-150): `useTerminalFlowControl({ sessionId, getTerminal: ..., pushMessageRef, isConnectedRef, onError, inputBatchDelayMs })`.
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 1.5.1b: Read the config value in `TerminalOutput` (~4 min)
- Import `loadTerminalConfig` alongside the existing `DEFAULT_TERMINAL_CONFIG` import (line 54).
- At the `useTerminalStream({...})` call site (~line 456), add `inputBatchDelayMs: loadTerminalConfig().inputBatchDelayMs,`. This is a one-time-per-render synchronous read, matching the existing precedent in this same file (`DEFAULT_TERMINAL_CONFIG.fontSize` used the same way at line ~892) and in `XtermTerminal.tsx:210` (`loadTerminalConfig()` read directly, not the live `useTerminalConfig()` hook) — not a live-reactive subscription. A future settings UI (Phase 3) that wants live updates without a full remount would need to upgrade this call site to `useTerminalConfig()`; out of scope here.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

---

## Phase 2: AC8 Test-Coverage Closeout

### Epic 2.1: Remaining Required Test Categories

**Goal**: All four AC8-mandated test categories (default-off passthrough, coalescing, early-flush, flush-on-unmount) are present and passing, plus explicit ordering (AC7) coverage and a full-suite regression run.

#### Story 2.1.1: Explicit default-off passthrough regression test
**As a** reviewer, **I want** an explicit test proving the default (`inputBatchDelayMs` omitted/`0`) never coalesces, **so that** AC1 has a standing regression lock, not just an implicit pass from the extraction gate in Task 1.2.1b.

**Acceptance Criteria**:
- Two `sendInput` calls with batching off produce two messages, not one (AC1, AC8).
  - *Given* `createTestOptions()` is used with no `inputBatchDelayMs` override (defaults to `0` via the hook's own destructuring default), and `sendInput('hi')` then `sendInput('there')` are called back-to-back with no timer advance, *When* both calls complete, *Then* `pushMessageFn.mock.calls.length === 2` (one message per call, no coalescing) and `jest.getTimerCount() === 0` (no batch timer was ever armed, confirming the `inputBatchDelayMs <= 0` branch bypassed the batching machinery entirely rather than running a "batch of one").

**Files**: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

##### Task 2.1.1a: Write the test (~3 min)
- Add the test described above to the `describe('sendInput')` block (existing, not the new `describe('sendInput batching')` block from Story 1.3.1, since this is explicitly about the *absence* of batching).
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

#### Story 2.1.2: Ordering across the coalesce boundary
**As a** user, **I want** batched keystrokes to arrive at the PTY in the order I typed them, **so that** coalescing never scrambles my input.

**Acceptance Criteria**:
- Byte order is preserved across three coalesced calls (AC7).
  - *Given* `inputBatchDelayMs: 64` and `sendInput('ab')`, then `sendInput('cd')`, then `sendInput('ef')` are called in that order within the window (6 bytes total, below the 32-byte early-flush threshold), *When* `jest.advanceTimersByTime(64)` runs, *Then* `pushMessage` is called exactly once with `msg.data.value.data` deep-equal to `Uint8Array.from([97, 98, 99, 100, 101, 102])` — "abcdef" in call order, not reordered or interleaved.

**Files**: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

##### Task 2.1.2a: Write the ordering test (~4 min)
- Add the test described above.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

#### Story 2.1.3: Full-suite regression run
**As a** reviewer, **I want** confirmation the entire test file (existing resize/resync/flow-control tests plus every new batching test) passes together, **so that** AC8's "existing tests continue to pass" clause is verified, not assumed.

**Acceptance Criteria**:
- The full file passes with zero failures (AC8).
  - *Given* all tasks in Phase 1 and Epics 2.1.1-2.1.2 are complete, *When* `cd web-app && npx jest --no-coverage --testPathPatterns="useTerminalFlowControl.test"` is run, *Then* the command exits 0 with every test in the file (pre-existing `resize`/`requestFullResync`/`sendFlowControl` tests plus all new `sendInput`/`sendInput batching` tests) reporting pass, and the reported test count reflects every test added across Tasks 1.2.1b, 1.3.1d, 1.3.1e, 1.3.2a, 1.3.3b, 1.4.1d, 1.4.2c, 2.1.1a, 2.1.2a.

**Files**: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` (verification only)

##### Task 2.1.3a: Run the full suite and confirm (~2 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="useTerminalFlowControl.test"`, capture the pass/fail summary line, confirm 0 failures.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` (verification only)

---

## Phase 3: [OPTIONAL / STRETCH] Minimal Settings UI Touchpoint

**Not required to satisfy this plan's committed scope** (Phase 1-2 fully satisfy AC1-AC5, AC7, AC8, and the data-model half of AC6). Included here, per the scope-decision instruction in this task's brief, so the UI gap is an explicit, documented deferral rather than a silently dropped acceptance criterion. Recommend filing as a separate follow-up backlog item rather than bundling into the same PR as Phase 1-2 (see Unresolved Questions).

### Epic 3.1: Toggle in the Appearance Settings Tab

**Goal**: A minimal, discoverable way for a user to opt into batching, per `research/ux.md`'s recommendation of a single on/off toggle (mapping "on" → 32ms, "off" → 0) rather than exposing the full 5-value millisecond dropdown in the UI layer.

#### Story 3.1.1: Add a "Reduce typing network traffic" toggle
**As a** user on a slow or metered connection, **I want** a simple way to turn on input batching, **so that** I don't have to manually edit localStorage to benefit from it.

**Acceptance Criteria**:
- A toggle in the Appearance settings tab persists the setting via the existing `TerminalConfig` mechanism (AC6, UI half).
  - *Given* the user opens Settings → Appearance and toggles "Reduce typing network traffic" on, *When* the toggle's `onChange` fires, *Then* `saveTerminalConfig({ inputBatchDelayMs: 32 })` is called (32ms — herdr-web's own early-flush threshold, per `research/ux.md` §2's recommended single sane default), and toggling it back off calls `saveTerminalConfig({ inputBatchDelayMs: 0 })`.

**Files**: `web-app/src/app/settings/page.tsx`, a new `web-app/src/components/settings/TerminalInputBatchingToggle.tsx` (or equivalent, colocated with other Appearance-tab settings components)

##### Task 3.1.1a: Build the toggle component (~5 min)
- New component using `useTerminalConfig()` (the existing reactive hook — first real caller of its setter, per `research/ux.md` §1's finding that it currently has zero callers), rendering a native `<input type="checkbox" id="input-batching-toggle" checked={config.inputBatchDelayMs > 0} onChange={...} />` paired with `<label htmlFor="input-batching-toggle">Reduce typing network traffic</label>`, following `PushNotificationSettings.tsx:54-63`'s exact id/htmlFor pattern (per `research/ux.md` §4's accessibility guidance — native checkbox, not a custom switch). One-line help text below: "Batches rapid keystrokes into fewer network messages. Off by default; most users won't notice a difference."
- Files: `web-app/src/components/settings/TerminalInputBatchingToggle.tsx` (new)

##### Task 3.1.1b: Mount it in the Appearance tab (~3 min)
- Add `<TerminalInputBatchingToggle />` to `page.tsx`'s Appearance tab content, alongside the existing `ThemePicker`/`PushNotificationSettings` (`page.tsx:120-129`).
- Files: `web-app/src/app/settings/page.tsx`
