# Implementation Plan: new-renderer

Feature: Fix terminal escape code rendering broken by new Claude Code renderer
Date: 2026-06-24
Status: Ready for implementation
ADRs: None

---

## Context

Claude Code's new renderer is a MOSH-style State Synchronization Protocol (SSP) that
routes PTY output through `StateGenerator` / `DeltaGenerator` → proto messages →
`StateApplicator` / `DeltaApplicator` → xterm.js.  This is a fundamentally different
path from the old raw-streaming path and introduces several new corruption points.

The bugs below are confirmed by direct code inspection — not speculative.  They are
ordered by severity within each phase.

---

## Scope

### Note on Scope Phases
Phase 1 (critical fixes) is the bug fix. Phases 2–4 in plan.md (bandwidth optimization,
analytics activation, defensive maintenance) are independent improvements that CAN be
split into separate tickets. They are included in the same plan for efficiency but are
not blocking the Phase 1 fix. Phase 1 can be shipped independently.

---

## Success Metrics

### Acceptance Threshold

- Terminal renders correctly = xterm.js displays Claude Code session output with correct
  colors (SGR), correct cursor positioning, alternate-screen entry/exit without artifacts,
  and OSC hyperlinks clickable. Verified by: running a Claude Code session in browser
  post-fix and observing no garbling, no U+FFFD replacement characters, and no broken
  cursor movement.
- Measurable regression gate: zero U+FFFD (replacement character) sequences observed in
  browser DevTools → Network → WebSocket frames for a 60-second Claude Code session.

---

## Dependency Visualization

```
Task 1 (StateApplicator stream:true)  ──┐
Task 2 (ESP lookback window)           ─┤── independent, parallelisable (Phase 1 Critical)
Task 3 (ESP ED2+ED3 strip)             ─┤
Task 4 (RedrawThrottler cursor-up)     ─┤
                                        │
                                        ▼
                            Phase 1 fixes landed
                                        │
                                        ▼
Task 5 (analytics enable)  ── depends on understanding the fixed
                               pipeline; validates fixes are correct
                               and adds permanent diagnostic visibility

[Phase 4 — Defensive Maintenance, separate PR]
Task 6 (sanitizeUTF8Bytes SO/SI)   ─┐── dead code today; fix before re-wiring
Task 7 (splitInto*Lines OSC/\n)    ─┘
```

Tasks 1–4 are independent of each other and can be implemented in parallel (Phase 1 Critical).
Tasks 6–7 (formerly 5–6) are moved to Phase 3 Defensive Maintenance — they fix real bugs in
code that is not on any active execution path and cannot cause the current regression.
Task 5 (analytics) must follow Phase 1 fixes; it is used to validate the pipeline is clean.

---

## Phase 1: Critical Fixes

### Epic 1.1: Multi-byte Sequence Reassembly (TypeScript)

**Goal**: Ensure no multi-byte UTF-8 characters or multi-chunk SSP diffs are
corrupted at the TextDecoder boundary.

---

#### Story 1.1.1 — Fix `StateApplicator` TextDecoder `stream: true` (CRITICAL)

**Bug**: `StateApplicator.ts` constructs `new TextDecoder()` without options and calls
`.decode()` on every diff/line without `{ stream: true }`.  When a multi-byte UTF-8
character (or a UTF-8 string inside an OSC sequence payload) straddles two consecutive
`TerminalDiff` proto messages the decoder finalises state on the first call, emitting
`U+FFFD` (replacement character) for the dangling bytes.  The second call has no memory
of the incomplete sequence, producing visible garbage in the terminal.

**Files**: `web-app/src/lib/terminal/StateApplicator.ts`

**Changes**:
1. Leave the constructor as-is (`new TextDecoder()` — no constructor options needed).
   The `{ stream: true }` behavior is controlled entirely at each `.decode()` call site,
   not at construction time. Constructor options `{ fatal, ignoreBOM }` do not enable
   streaming and would mislead implementers into thinking the constructor change was the
   material fix.
2. Add `{ stream: true }` to EVERY `.decode()` call site on this decoder instance, not
   just the constructor. Specifically:
   - Line 231: `this.textDecoder.decode(diff.diffBytes, { stream: true })` (in `applyDiffImmediate`)
   - Line 476: `this.textDecoder.decode(line.content, { stream: true })` (in `applyIncrementalState`)
   Grep pattern: `this.textDecoder.decode(` — every occurrence in the file must gain
   `{ stream: true }`. This is the only material change needed.
3. Add a `reset()` method (call on reconnect / state machine reset) that replaces the
   decoder instance:
   ```typescript
   public reset(): void {
     this.textDecoder = new TextDecoder();
   }
   ```
   Wire `reset()` by calling `this.reset()` from **inside `resetSequence()` (line 629 of
   `StateApplicator.ts`)**, NOT from a separate or external cleanup location. `resetSequence()`
   already cancels in-flight RAF; `reset()` must be called from the same method so the
   decoder state and sequence-tracking state are always reset together. Do NOT wire it only
   from unmount — unmount is too late for reconnect scenarios.

**Tests**: Add a unit test in `web-app/src/__tests__/StateApplicator.test.ts` that
passes a multi-byte UTF-8 string (e.g., `\xc3\xa9` — "é") split across two
`TerminalDiff` messages and asserts no `�` appears in the write calls captured
on a mock terminal.

**Time estimate**: ~3 min

---

#### Story 1.1.2 — Fix scrollback chunk decoding in `useTerminalStream.ts` (HIGH)

**Bug**: `useTerminalStream.ts` line 308 decodes scrollback chunks with the shared
`textDecoderRef.current` but **without** `{ stream: true }`.  Since the same decoder
instance is used for the live output path (line ~271, with `{ stream: true }`), a prior
call that left an incomplete multi-byte sequence in the decoder's internal buffer will
be flushed when the next scrollback chunk is decoded without `stream: true`.  This
corrupts both the scrollback content and potentially the live output path that follows.

**File**: `web-app/src/lib/hooks/useTerminalStream.ts`

**Changes**:
1. Fix BOTH call sites that use `textDecoderRef.current` without `{ stream: true }`:
   - **Line 298**: decodes `currentPaneResponse.content` — this must be fixed. A
     `currentPaneResponse` arriving after partially-decoded live output will flush the
     decoder's internal buffer and corrupt the live path. Either add `{ stream: true }` here
     or use a dedicated decoder (see below).
   - **Line 308**: decodes scrollback chunks — this must also be fixed.
   Audit ALL call sites of `textDecoderRef.current.decode` before closing this story; do not
   assume only one call site needs the change.
2. Create a dedicated `TextDecoder` ref for scrollback/pane-response decoding, separate from
   the live-output decoder:
   ```typescript
   const scrollbackDecoderRef = useRef(new TextDecoder());
   ```
3. Replace both the line 298 and line 308 decode calls to use
   `scrollbackDecoderRef.current.decode(chunk.data, { stream: true })`.
4. Reset `scrollbackDecoderRef` on disconnect (same location as the existing decoder
   reset).

**Tests**: Existing `__tests__/useTerminalStream.test.ts` (if any) should cover this;
add a test if none exists that sends a multi-byte sequence split across two scrollback
chunks and asserts clean output.

**Time estimate**: ~2 min

---

### Epic 1.2: Escape Sequence Parser Hardening (TypeScript)

**Goal**: Fix the two known correctness bugs in `EscapeSequenceParser.ts` that cause
live streaming output to be silently corrupted or dropped.

---

#### Story 1.2.1 — Extend `findPartialEscapeAtEnd` lookback window (HIGH)

**Bug**: `EscapeSequenceParser.ts` line 83:
```typescript
const scanLength = Math.min(20, data.length);
```
The 20-character lookback is too short for:
- DCS sequences (`\x1bP...ST`) which carry payload strings
- OSC sequences with long titles (e.g., `\x1b]0;Working on feature/foo...`)
- PM/APC sequences emitted by newer TUI frameworks

If a chunk boundary falls within an OSC payload (very common with 16 KB chunking),
the `\x1b` opening byte is more than 20 characters from the end of the chunk and is
not detected as a partial sequence — the first fragment is written to xterm.js alone,
garbling the terminal.

**File**: `web-app/src/lib/terminal/EscapeSequenceParser.ts`

**Changes**:
1. Line 83: Increase the scan window to 256 characters:
   ```typescript
   // Before
   const scanLength = Math.min(20, data.length);

   // After — OSC title strings and DCS payloads can exceed 20 bytes
   const scanLength = Math.min(256, data.length);
   ```
2. In `isCompleteEscapeSequence`, add detection for the missing sequence types so
   they are correctly identified as partial (and buffered) rather than falling through
   to the "unknown — assume complete" default at line 157:
   ```typescript
   // DCS: \x1bP...ST  (terminates with \x1b\x5c)
   if (secondChar === 'P') {
     return seq.includes('\x1b\\') || seq.endsWith('\x07');
   }
   // PM:  \x1b^...ST
   if (secondChar === '^') {
     return seq.includes('\x1b\\');
   }
   // APC: \x1b_...ST
   if (secondChar === '_') {
     return seq.includes('\x1b\\');
   }
   // SOS: \x1bX...ST
   if (secondChar === 'X') {
     return seq.includes('\x1b\\');
   }
   ```
   Insert these before the existing "Simple escape" branch (line ~147).

**Tests**: `web-app/src/lib/terminal/__tests__/EscapeSequenceParser.test.ts` — add test cases:
- An OSC sequence with a 30-character title split so `\x1b]` lands at byte 22 from
  the end of the chunk: assert the partial is buffered, not emitted.
- A DCS passthrough sequence split after `\x1bP` and before the ST terminator: assert
  buffered.

**Time estimate**: ~3 min

---

#### Story 1.2.2 — Remove ED2+ED3 combined-sequence stripping (HIGH)

**Bug**: `EscapeSequenceParser.ts` line 39:
```typescript
const filtered = fullData.replace(/\x1b\[2J\x1b\[3J/g, "\x1b[2J");
```
The new Claude Code renderer uses `\x1b[2J\x1b[3J` (erase screen + erase scrollback)
as a deliberate full reset — it expects the scrollback to be cleared so the next frame
renders into a clean buffer.  Stripping ED3 leaves stale scrollback visible, causing
history from the previous session to bleed through below the new render frame.

The original intent was to prevent xterm.js flickering.  With xterm.js v6 (WebGL
renderer default) and the `scrollback: 5000` option in `XtermTerminal.tsx`, the
flicker concern is no longer valid — xterm.js handles ED2+ED3 correctly without
special treatment.

**File**: `web-app/src/lib/terminal/EscapeSequenceParser.ts`

**Changes**:
1. Lines 36–39: Remove the ED3 filter entirely:
   ```typescript
   // Remove this block:
   // ED3 filter: strip ED3 (erase scrollback) when immediately preceded by ED2
   const filtered = fullData.replace(/\x1b\[2J\x1b\[3J/g, "\x1b[2J");
   ```
2. Replace with a pass-through:
   ```typescript
   const filtered = fullData; // No sequence stripping
   ```
   (The `filtered` variable name is kept to avoid a larger diff; alternatively
   inline it into the `findPartialEscapeAtEnd` call below.)

**Acceptance Criteria (required before merge)**:
- Verified in browser: xterm.js renders combined `\x1b[2J\x1b[3J` reset without flicker
  regression. Run a Claude Code session, confirm that previous-session scrollback does not
  bleed through after a reset, and confirm no flickering on alternate-screen transitions.

Automated CI gate (added):
- Unit test `EscapeSequenceParser_should_passThrough_ED3_When_pairedWithED2`:
  input = `\x1b[2J\x1b[3J` (combined ED2+ED3 reset)
  assert output contains BOTH sequences unchanged (neither is stripped)
  File: `web-app/src/lib/terminal/__tests__/EscapeSequenceParser.test.ts`
  This test runs in CI and provides an automated regression gate.

> **NOTE**: Story ships when BOTH gates pass: (1) the unit test above passes in CI, (2) the browser smoke test is completed and recorded in the PR description. CI enforces the unit test gate; the team enforces the browser gate via PR review.

> ACCEPTED RISK - Browser Smoke Test Gate
> The browser smoke test for Story 1.2.2 is enforced via PR review checklist, not automated CI.
> This is intentional: adding automated browser rendering tests (Playwright + xterm.js visual comparison)
> is out of scope for a bug fix. The unit test (EscapeSequenceParser_should_passThrough_ED3_When_pairedWithED2)
> provides automated regression coverage for the code path. Browser testing covers visual rendering behavior
> that cannot be unit-tested; PR review is the industry-standard gate for this level of testing.
> Automated visual regression is tracked as future scope in the terminal-analytics project.

**Tests**: `web-app/src/lib/terminal/__tests__/EscapeSequenceParser.test.ts`:
- Existing test "ED3 filter" — update: the combined `\x1b[2J\x1b[3J` sequence should
  now pass through unchanged.
- Add: verify `\x1b[3J` alone passes through (was never filtered; just confirm no
  regression).

**Time estimate**: ~1 min + browser smoke test (~5 min)

---

### Epic 1.3: Redraw Throttler Fix (TypeScript)

**Goal**: Prevent the `RedrawThrottler` from discarding intermediate animation frames
that carry meaningful state from the new renderer.

---

#### Story 1.3.1 — Scope `RedrawThrottler` detection to explicit full-screen redraws only (HIGH)

**Bug**: `TerminalStreamManager.ts` line 56:
```typescript
const isFullRedraw = /^\x1b\[\d+A/.test(chunk);
```
Any chunk that starts with a cursor-up sequence is classified as a "full redraw" and
may be discarded.  The new Claude Code renderer (Ink-based) emits `\x1b[\d+A` on every
incremental line update — not just full-screen redraws.  This means ordinary progress
bar updates, diff outputs, and spinner frames are dropped whenever two arrive within
the 100 ms throttle window.

**File**: `web-app/src/lib/terminal/TerminalStreamManager.ts`

**Changes**:
1. Tighten the full-redraw detection to require the cursor-up sequence to be followed
   by a screen-clear or cursor-home sequence within the first 32 bytes — i.e., it is
   a true full-screen redraw, not a partial scroll:
   ```typescript
   // Before
   const isFullRedraw = /^\x1b\[\d+A/.test(chunk);

   // After — only throttle genuine full-screen redraws (cursor-up + erase-screen)
   // NOTE: \x1b[H (cursor-home) is intentionally excluded — it is also emitted during
   // non-full-screen interactive prompts (e.g. repositioning to top of a diff block) and
   // would over-throttle those. Only erase-screen sequences (\x1b[2J or \x1b[J) are
   // reliable indicators of a genuine full-screen redraw.
   const isFullRedraw = /^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/.test(
     chunk.substring(0, 32)
   );
   ```
2. Reduce `throttleMs` from `100` to `33` (30 FPS cap vs 10 FPS) so that even genuine
   full-screen redraws are flushed quickly enough that fast-moving TUI frames are not
   noticeably dropped.  Update the comment at line 47 accordingly.

**Tests**: `web-app/src/__tests__/TerminalStreamManager.test.ts`:
- A chunk starting with `\x1b[5A` alone (cursor-up, no clear) should NOT be throttled.
- A chunk starting with `\x1b[5A\x1b[2K` (cursor-up + clear-line) SHOULD be throttled.
- Two genuine full-redraw chunks arriving within 33 ms: only the second should reach
  xterm.js.

**Time estimate**: ~3 min

---

### Epic 1.5: Combined Integration Test

**Goal**: Verify that all four Phase 1 fixes work correctly together in a single
end-to-end pipeline test, catching any cross-fix interactions before merge.

---

#### Story 1.5.1 — Combined Phase 1 Pipeline Integration Test (~45 min)

**As a developer**, I want an end-to-end test that exercises all Phase 1 fixes together,
so that cross-fix interactions are caught before merge.

**File**: `web-app/src/lib/terminal/__tests__/pipeline-integration.test.ts` (new file)

**Test scenario**: The test simulates the full Phase 1 pipeline in a single test run by
combining all four bug scenarios:

1. **Multi-byte escape sequence split across two proto frames** — exercises the
   `StateApplicator` `TextDecoder { stream: true }` fix (Story 1.1.1).
2. **Sequence > 20 chars from end of chunk** — exercises the `findPartialEscapeAtEnd`
   lookback window extension to 256 (Story 1.2.1).
3. **Combined ED2+ED3 reset** — exercises the ED3 passthrough fix, confirming the
   combined sequence reaches xterm.js unchanged (Story 1.2.2).
4. **Cursor-up frame between two content frames** — exercises the `RedrawThrottler`
   narrowing fix, confirming the cursor-up-only frame is not throttled (Story 1.3.1).

**Acceptance Criteria**:
- Test simulates: multi-byte escape sequence split across two proto frames (tests
  TextDecoder fix) + sequence > 20 chars (tests lookback fix) + combined ED2+ED3 reset +
  cursor-up frame between two content frames (tests throttler fix)
- All four scenarios pass in a single test run
- Test is in `web-app/src/lib/terminal/__tests__/pipeline-integration.test.ts`

**Time estimate**: ~45 min

---

## Phase 2: Bandwidth Optimization

> **Note on dead code audit**: The dead code in `server/terminal/state.go` and
> `server/terminal/delta.go` should be audited — if `delta.go` is actually the delta.go
> for this feature (not dead code), it may already be partially wired. The architecture
> review found `DeltaApplicator.ts` and `server/terminal/delta.go` exist and the delta
> streaming infrastructure is already ~80% built but not activated. Hash-based change
> detection already exists server-side but is not used to suppress no-op snapshots.

### Epic 2.2: Activate Delta Streaming (already ~80% built)

**Goal**: Activate the existing delta streaming infrastructure to fix bandwidth
efficiency and make the pipeline renderer-agnostic (raw bytes in, not format-specific
snapshots). The current default path sends full snapshots (~8–20 KB) on every change —
up to 400 KB/s at 20 changes/sec. The infrastructure to fix this already exists.

---

#### Story 2.2.1 — Enable hash-gated snapshot suppression (~30 min)

**Context**: The `streamViaTmuxCapturePane` path compares full terminal screen strings
to detect changes (`content != s.lastContent` in `external_tmux_streamer.go:414`), but
it does not suppress sending a snapshot when the terminal is idle. Adding a hash check
before emission prevents any bytes from being sent when the terminal state has not
changed.

**Files**: `session/` or wherever `streamViaTmuxCapturePane` is implemented; likely
`server/services/connectrpc_websocket.go` and `session/external_tmux_streamer.go`

**Changes**:
1. Before emitting a full snapshot, compute a hash (FNV-1a or xxHash) of the current
   terminal state bytes.
2. If the hash matches the last-sent hash, suppress the snapshot emission entirely —
   return without calling `stream.WriteMessage`.
3. Store the last-sent hash alongside (or instead of) the full `lastContent` string to
   reduce memory allocation for the comparison.

**Acceptance Criteria**:
- When the terminal is idle (no PTY output), zero bytes are sent to the browser.
  Verify with the browser network tab: no WebSocket frames during idle periods.
- When the terminal changes (new output), the snapshot is sent as normal.

**Time estimate**: ~30 min

---

#### Story 2.2.2 — Activate server-side delta path as default (~1 hr)

**Context**: `server/terminal/delta.go` implements delta generation and
`web-app/src/lib/terminal/DeltaApplicator.ts` implements client-side delta application.
These are wired but not the default path. The `streamingMode` field on
`CurrentPaneRequest` already supports signaling from client to server. Activating the
delta path as default reduces per-update WebSocket traffic from 8–20 KB (full screen)
to 100–2000 bytes (changed cells only).

**Files**: `server/terminal/delta.go`, ConnectRPC streaming handler
(`server/services/connectrpc_websocket.go`), `web-app/src/lib/hooks/useTerminalStream.ts`

**Changes**:
1. Make the delta path the default for the capture-pane streaming path: after capturing
   the current terminal state, diff it against the previous state using the existing
   delta generation logic in `server/terminal/delta.go` and send a `TerminalDiff` proto
   message instead of a full snapshot.
2. Fall back to a full snapshot (`TerminalState` proto) on:
   - Reconnect (client has no prior state)
   - Hash mismatch exceeding a configurable threshold (e.g., > 50% of cells changed —
     a delta larger than the snapshot offers no benefit)
3. The `TerminalData` oneof in `proto/session/v1/session.proto` already carries
   `TerminalState` and `TerminalDiff` — no proto changes required.
4. Verify that `DeltaApplicator.ts` on the frontend handles the delta messages
   correctly and that the fallback full snapshot correctly resets applicator state.

**Acceptance Criteria**:
- After 100 lines of stable terminal output (only cursor movement, no new text), delta
  size is < 200 bytes per update vs. 8–20 KB for full snapshots.
- Verify with browser network tab: WebSocket frames during active output are
  substantially smaller than 8 KB.
- On reconnect, a full snapshot is sent (verify by disconnecting and reconnecting the
  WebSocket, confirming the first message is a `TerminalState`, not `TerminalDiff`).

**Time estimate**: ~1 hr

---

#### Story 2.2.3 — Integration test for delta vs. snapshot fallback (~30 min)

**Context**: The reconnect behavior (full snapshot on reconnect, deltas after) is a
correctness invariant that must not regress. A Go integration test covers this path
automatically.

**Files**: Go test in `server/services/` or `session/` (whichever package houses the
streaming handler logic)

**Test cases**:
1. **Happy path — deltas after initial state**: Start a full-screen terminal session,
   write 100 lines of output, then scroll. Assert that after the initial full snapshot,
   subsequent messages are `TerminalDiff` protos, not `TerminalState`.
2. **Reconnect fallback**: Connect, receive initial snapshot + deltas. Disconnect.
   Reconnect. Assert that the first message on reconnect is a `TerminalState` (full
   snapshot), not a `TerminalDiff`.
3. **High-churn fallback**: Send output that changes > 50% of terminal cells in one
   update. Assert that a `TerminalState` (full snapshot) is sent instead of a large
   `TerminalDiff`.

**Time estimate**: ~30 min

---

## Phase 3: Analytics Activation

### Epic 3.1: Enable Escape Code Analytics in Capture-pane Path

**Goal**: Activate the already-implemented `MangleCorrelator` and `EscapeCodeStore`
for the `streamViaTmuxCapturePane` path to provide permanent diagnostic visibility and
to validate that the Phase 1 fixes are effective.

---

#### Story 3.1.1 — Connect Stage 2 tap in `streamViaTmuxCapturePane` (MEDIUM)

**Context**: Stage 2 analytics is already wired in `streamViaControlMode`
(`connectrpc_websocket.go` lines ~765–770).  The `streamViaTmuxCapturePane` path has
no equivalent tap, so escape code corruption in the capture-pane path is invisible.

**File**: `server/services/connectrpc_websocket.go`

**Changes**:
1. At the start of `streamViaTmuxCapturePane` (after the instance handle is resolved),
   fetch the escape parser once:
   ```go
   escapeParser := instance.GetEscapeParser()
   ```
2. Inside the output goroutine (lines ~1166–1193), immediately before the
   `stream.WriteMessage` call that sends `fullContent`, add the Stage 2 tap:
   ```go
   if escapeParser != nil && escapeParser.IsEnabled() {
       escapeParser.ParseStage2([]byte(fullContent), instance.GetTotalBytesWritten())
   }
   ```
   This mirrors the existing tap in `streamViaControlMode`.
3. No other changes needed — Stage 1 is already wired in `response_stream.go` and
   fires before `streamViaTmuxCapturePane` processes the bytes.

**Time estimate**: ~2 min

---

#### Story 3.1.2 — Enable analytics by default in debug/dev config (MEDIUM)

**Context**: `EscapeCodeStore.enabled` is `false` by default (line 44 of
`escape_code_store.go`).  `EscapeCodeParser.IsEnabled()` gates both Stage 1 and Stage
2 taps.  Neither stage fires in production, making the analytics system permanently
dormant and providing no diagnostic value.

**Files**: `pkg/analytics/escape_code_store.go`, `session/response_stream.go`
(or wherever `newEscapeParserForSession()` is defined)

**Changes**:
1. Locate `newEscapeParserForSession()` (search in `session/response_stream.go` or
   `session/instance_controller.go`).
2. Add a config gate: if `config.DebugEscapeAnalytics` is true (a new bool field in
   `config/config.go`, default `false`, env var `STAPLER_SQUAD_DEBUG_ESCAPE_ANALYTICS`),
   call `escapeParser.SetEnabled(true)` after construction.
3. In `config/config.go`, add:
   ```go
   DebugEscapeAnalytics bool `json:"debugEscapeAnalytics,omitempty"`
   ```
   Env var override in the config loader:
   ```go
   if os.Getenv("STAPLER_SQUAD_DEBUG_ESCAPE_ANALYTICS") == "true" {
       cfg.DebugEscapeAnalytics = true
   }
   ```
4. Document in `CLAUDE.md` or `.claude/docs/` that setting
   `STAPLER_SQUAD_DEBUG_ESCAPE_ANALYTICS=true` activates escape-sequence analytics
   logging for debugging rendering regressions.

**Tests**: Unit test in `pkg/analytics/escape_code_store_test.go` — assert that
`Record()` is a no-op when `enabled = false` and records entries when `enabled = true`.
(These tests may already exist; confirm and extend if not.)

**Time estimate**: ~3 min

---

## Phase 4: Defensive Maintenance

> Stories in this phase fix real bugs confirmed by code inspection but are **not on any active
> code path**. `server/terminal/state.go` and `server/terminal/delta.go` are currently dead
> code — the `server/terminal` package is not imported by any production Go file. These bugs
> cannot be the cause of the current rendering regression. Deprioritize from the critical path;
> address in a follow-up maintenance PR after the Phase 1 regression is confirmed fixed.
>
> **Note**: If Phase 2 (Bandwidth Optimization) confirms that `delta.go` is in fact being
> activated as part of Epic 2.2, re-evaluate the stories below — they may graduate from
> defensive maintenance to active fixes.

### Epic 4.1: Server-side SSP Pipeline Fixes (Go) — Defensive Maintenance

**Goal**: Fix byte-level corruption in `StateGenerator` and `splitInto*Lines` helpers so
that if/when the `server/terminal` package is re-wired into an active path, these bugs do
not surface.

> **Scope note**: These were originally classified as "Phase 1 Critical" but architecture
> review confirmed that `StateGenerator` and `DeltaGenerator` are not instantiated anywhere
> in the current production codebase. Real bugs, but not regression causes; moved to
> Phase 4 defensive maintenance.

---

#### Story 4.1.1 — Preserve SO/SI and other control characters in `sanitizeUTF8Bytes` (MEDIUM)

**Bug**: `server/terminal/state.go` lines 332–340:
```go
default:
    // Replace other control characters with space to prevent parsing issues
    result.WriteByte(' ')
```
`SO` (`\x0e`) and `SI` (`\x0f`) — the character-set shift-in/shift-out bytes used by
some terminal drawing programs — are silently replaced with spaces.  Any terminal
program that relies on SO/SI for box-drawing characters (including some versions of
ncurses) will render garbage.

**File**: `server/terminal/state.go`

**Changes**:
1. Extend the `switch r` in `sanitizeUTF8Bytes` to preserve `\x0e` and `\x0f`:
   ```go
   case '\t', '\n', '\r':
       result.WriteRune(r) // Keep tab, newline, carriage return
   case 7, 8:
       result.WriteRune(r) // Keep BEL and BS
   case 0x0e, 0x0f:
       result.WriteRune(r) // Keep SO (shift out) and SI (shift in) for character sets
   ```
2. Document the intentional omission of other control bytes (`\x01`–`\x06`,
   `\x10`–`\x1a`, `\x1c`–`\x1f`) with a comment explaining they are not used by
   xterm.js-targeted terminal programs.

**Tests**: `server/terminal/state_test.go` — add table-driven test cases for `SO`
(`\x0e`) and `SI` (`\x0f`) asserting they survive `sanitizeUTF8Bytes` unchanged.

**Time estimate**: ~2 min

---

#### Story 4.1.2 — Replace naive `\n` split with escape-sequence-aware line splitter (MEDIUM)

**Bug**: `server/terminal/delta.go` line 249 and `server/terminal/state.go` line 181:
```go
lines := bytes.Split(output, []byte("\n"))
```
OSC sequences can legitimately contain `\n` bytes in their parameter strings (OSC 52
clipboard content, OSC 133 shell integration prompts, multi-line OSC 8 hyperlinks).
`bytes.Split` truncates these sequences mid-parameter, producing malformed OSC output
that xterm.js silently drops or misinterprets.

**Files**: `server/terminal/delta.go`, `server/terminal/state.go`

**Changes**:
1. Create a new function `splitTerminalLines(output []byte) [][]byte` in
   `server/terminal/state.go` (or a shared `server/terminal/util.go`):
   - Walk the byte slice character by character.
   - Track `inOSC bool`: set to `true` on `\x1b]`, cleared on `\x07` (BEL) or
     `\x1b\\` (ST — two bytes).
   - Track `inEscape bool`: set to `true` on `\x1b`, cleared on the sequence
     terminator.
   - Only split on `\n` when `!inOSC && !inEscape`.
   - Preserve the same "remove trailing empty line" logic.
2. Replace `bytes.Split(output, []byte("\n"))` in both files with a call to
   `splitTerminalLines(output)`.
3. In `server/terminal/state.go`, the function `splitIntoTerminalLines` wraps
   `splitTerminalLines` and converts the result to `[]*sessionv1.TerminalLine`.

**Tests**: New table-driven test `TestSplitTerminalLines` in
`server/terminal/state_test.go`:
- Normal output with plain `\n`: same result as `bytes.Split`.
- OSC 8 hyperlink containing `\n` in the URL: asserts the OSC sequence is not split.
- OSC 133 prompt annotation with embedded newline: asserts the annotation byte
  survives intact.

**Time estimate**: ~5 min

---

## Test Coverage Summary

| Story | New Tests | File |
|---|---|---|
| 1.1.1 | `TestStateApplicatorMultiByteAcrossDiff` | `web-app/src/__tests__/StateApplicator.test.ts` |
| 1.1.2 | `TestScrollbackDecoderIsolation` (covers lines 298 + 308) | `web-app/src/__tests__/useTerminalStream.test.ts` |
| 1.2.1 | OSC/DCS partial detection with >20-char lookback | `web-app/src/lib/terminal/__tests__/EscapeSequenceParser.test.ts` |
| 1.2.2 | ED3 passthrough (update existing) + browser smoke test | `web-app/src/lib/terminal/__tests__/EscapeSequenceParser.test.ts` |
| 1.3.1 | Partial cursor-up not throttled; full redraw (cursor-up + erase) throttled | `web-app/src/__tests__/TerminalStreamManager.test.ts` |
| 1.5.1 | All four Phase 1 scenarios in a single pipeline integration test run | `web-app/src/lib/terminal/__tests__/pipeline-integration.test.ts` |
| 2.2.1 | Idle terminal sends zero bytes (network tab / mock WebSocket) | Go test in `server/services/` or `session/` |
| 2.2.2 | Delta size < 200 bytes on stable output; full snapshot on reconnect | Go test in `server/services/` or `session/` |
| 2.2.3 | `TestDeltaVsSnapshotFallback` (happy path, reconnect, high-churn) | Go test in `server/services/` or `session/` |
| 3.1.2 | Analytics enabled/disabled gate | `pkg/analytics/escape_code_store_test.go` |
| 4.1.1 | `TestSanitizeUTF8BytesPreservesSOSI` | `server/terminal/state_test.go` |
| 4.1.2 | `TestSplitTerminalLinesOSC` | `server/terminal/state_test.go` |

---

## Risk Flags

1. **ED3 removal (Story 1.2.2)**: The original ED3 filter existed to prevent xterm.js
   scrollback flickering.  With xterm.js v6 (WebGL renderer) this is believed to be
   no longer needed, but this must be manually validated by running a Claude Code
   session in the browser after the change (see the explicit AC added to Story 1.2.2).
   If flickering returns, re-introduce the filter but guard it behind an option rather
   than applying it unconditionally.  **Do not merge Story 1.2.2 without browser validation.**

2. **Delta path audit needed before Phase 2 (Epic 2.2)**: Architecture review identified
   `server/terminal/delta.go` as dead code (not imported by any production Go file), but
   the same review found `DeltaApplicator.ts` and the SSP infrastructure are ~80% built.
   Before implementing Story 2.2.2, audit whether `delta.go` is the same delta.go
   intended for this feature — it may already be partially wired. If it is dead code
   unrelated to this feature, a new implementation may be needed.

3. **`sanitizeUTF8Bytes` is dead code (Story 4.1.1)**: Architecture review confirmed that
   `server/terminal/state.go` and `server/terminal/delta.go` are not on any active
   production code path — the `server/terminal` package is not imported by any non-test
   Go file.  Story 4.1.1 fixes a real bug but it cannot be the cause of the current
   rendering regression.  Moved to Phase 4 Defensive Maintenance. If Phase 2 activates
   `delta.go`, this story graduates to an active fix.

4. **`splitTerminalLines` performance (Story 4.1.2)**: The new character-by-character
   splitter is O(n) like `bytes.Split` but with higher constant factor.  For 4 KB
   capture-pane snapshots at 10 Hz this is negligible.  If the SSP path processes
   longer buffers, benchmark with `go test -bench` before merging.  (Phase 4 only.)

5. **Analytics overhead (Story 3.1)**: `EscapeCodeParser.Parse` / `ParseStage2` are
   passthrough observers and add no latency when disabled.  When enabled
   (`STAPLER_SQUAD_DEBUG_ESCAPE_ANALYTICS=true`) they walk every byte — acceptable
   for development/diagnosis, but must remain opt-in for production.
