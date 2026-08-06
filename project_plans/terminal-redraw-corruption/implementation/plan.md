# Implementation Plan: terminal-redraw-corruption

**Feature**: Fix stray leading-character fragments from a previous, longer status line
surviving under a new, shorter redrawn line in the web-rendered xterm.js terminal.
**Date**: 2026-08-06
**Status**: Planned (Phase 4 validated) — implementation deferred to a fresh Phase 5 session
**ADRs**: `decisions/ADR-001-instrument-first-fix-both-candidates.md`,
`decisions/ADR-002-footprint-aware-redraw-coalescing.md`

## Root Cause

Research produced two independently code-verified, plausible root causes for the same
live-stream write path, and the two research tracks disagree on which is primary:

- **Candidate A — `RedrawThrottler` frame-coalescing drop**
  (`web-app/src/lib/terminal/TerminalStreamManager.ts:42-92`). `process()` classifies a
  chunk as a "full redraw" via `/^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/` against the first 32
  bytes, then does `this.pendingRedraw = chunk` — an **unconditional overwrite** of any
  previously-pending, undelivered redraw chunk. Only the last pending chunk is ever
  flushed, on a 33ms timer. If two consecutive redraw frames have different erase
  footprints (e.g. frame 1 clears columns 0-40, frame 2 only clears columns 0-20 because
  the new status line is shorter) and frame 1 is dropped, frame 1's wider erase never
  reaches the terminal — stale glyphs from *whatever was on screen before frame 1*
  persist past frame 2's narrower erase.
  Weakness in this hypothesis: the classifier regex requires `\x1b[2K` (erase whole line)
  or `\x1b[J`/`\x1b[nJ` (erase in display) immediately after a cursor-up sequence — it does
  **not** match bare `\x1b[K`, `\x1b[0K`, or `\x1b[1K` (erase-in-line variants), which are
  the sequences this repo's own test fixtures
  (`web-app/src/lib/test-generators/escape-codes/generators.ts`) and the more common
  Ink/status-spinner redraw idiom actually use. If Claude Code's TUI uses bare EL, this
  classifier never fires and Candidate A cannot be the mechanism for this specific symptom.

- **Candidate B — `broadcastControlModeUpdate` silent channel-drop**
  (`session/tmux/control_mode.go:683-697`). Each subscriber has a 100-slot buffered
  channel (`SubscribeToControlModeUpdates`, line ~707). Broadcast is a non-blocking
  `select`/`default` — if the channel is full, the update is dropped and only a
  `log.Warn` is emitted; there is no gap-detection, resync signal, or backpressure to the
  producer. A dropped `%output` frame is a **whole tmux control-mode line dropped**
  (control-mode frames are newline-delimited and forwarded atomically, so this cannot be
  a mid-escape-sequence tear — it's a whole-frame loss). Confirmed via grep that
  `TerminalData_FlowControl`/`pauseCh`/`ptyPaused` (`server/services/session_service.go`)
  is unreachable from browser clients — `connectrpc_websocket.go` has zero references to
  `FlowControl|Paused|pause` — so there is **no backpressure mechanism at all** on the
  live tmux-backed web terminal path today. A dropped frame containing an EL/ED erase
  sequence would reproduce exactly this symptom: the client never received the erase, so
  the next (shorter) frame's content is drawn over stale glyphs.

**Resolution (see ADR-001)**: rather than picking one candidate on the strength of static
code reading alone, this plan front-loads a dedicated **Instrumentation & Repro epic
(Phase 1)** that adds production-safe, debug-gated telemetry to both mechanisms so a real
occurrence can be attributed to one, the other, or both. Phase 2 and Phase 3 then implement
independent fixes for Candidate A and Candidate B respectively — **both fixes proceed
regardless of instrumentation outcome**, because both are independently real reliability
gaps per the research pitfalls (item 4: a coalescer must never drop a frame whose erase
footprint differs from its replacement; item 7: a silent-drop backpressure path must be
diagnosable at minimum). Instrumentation determines *prioritization/sequencing* of the two
fix epics in a real Phase 5 implementation session (whichever telemetry fires more/first
under load ships first), not *whether* either fix happens. Phase 4 adds the regression
test required by pitfall item 6.

## Dependency Visualization

```
Phase 1: Instrumentation & Repro
  Epic 1.1: RedrawThrottler discard telemetry ──────┐
  Epic 1.2: broadcastControlModeUpdate drop telemetry┤
  Epic 1.3: Byte-capture repro test harness ─────────┤
                                                      ▼
                                    (evidence gathered; determines
                                     Phase 2/3 priority order in
                                     the real Phase 5 session —
                                     both still ship)
                                                      │
        ┌─────────────────────────────────────────────┼─────────────────────────────────┐
        ▼                                             ▼
Phase 2: Fix Candidate A                    Phase 3: Fix Candidate B
  Epic 2.1: Sequence-aware,                   Epic 3.1: Gap-detection counter +
  footprint-preserving RedrawThrottler          structured drop event on
  (flush-before-replace when footprint          broadcastControlModeUpdate
  differs; ADR-002)                             (pitfall #7 — diagnosability,
        │                                        not full backpressure)
        └─────────────────────┬───────────────────────┘
                               ▼
                    Phase 4: Regression Coverage
                      Epic 4.1: Monotonic row-erase-coverage
                      test across coalesced frames (pitfall #6)

Phase 5 (out of scope for this plan): deferred stretch items
  - 16KB chunk-boundary escape-awareness in writeDirectWithFlowControl/enqueueWrite
  - WebGL→canvas renderer fallback verification (mitigation only; addon already present)
  - DEC mode 2026 synchronized output (requires upstream Claude Code/Ink cooperation)
  - requestFullResync()/CurrentPaneRequest as a bounded escape-hatch (from
    terminal-visibility-resync/) — noted as a possible belt-and-suspenders mitigation,
    not required by this fix
```

---

## Phase 1: Instrumentation & Repro

### Epic 1.1: RedrawThrottler discard telemetry

**Goal**: Make every dropped `pendingRedraw` overwrite observable, including the erase
footprint of both the discarded chunk and the chunk that replaced it, gated behind the
existing `localStorage.getItem("debug-terminal") === "true"` debug flag already used
elsewhere in this file (see `TerminalStreamManager.ts:181,196,219,341` for the established
pattern) so it is production-safe by default.

#### Story 1.1.1: Log discarded pending-redraw chunks with footprint metadata

**As a** developer investigating terminal corruption reports, **I want** the
`RedrawThrottler` to log what it discards and what replaced it, **so that** I can confirm
whether Candidate A fires for a real corruption occurrence without needing a live repro
session.

**Acceptance Criteria**:
- When `process()` overwrites a non-null `this.pendingRedraw` with a new chunk, and the
  debug flag is enabled, a `console.log('[RedrawThrottler]', ...)` fires with: length of
  discarded chunk, length of new chunk, first 40 chars of each (escaped/visible), and a
  best-effort erase-footprint summary (which erase sequences — `\x1b[K`, `\x1b[0K`,
  `\x1b[1K`, `\x1b[2K`, `\x1b[J`, `\x1b[2J`, `\x1b[3J` — appear in each chunk).
- No behavior change to what is flushed — this task only adds observability.
- `npx jest --no-coverage --testPathPatterns="TerminalStreamManager.test"` still passes.

**Files**:
- `web-app/src/lib/terminal/TerminalStreamManager.ts`

##### Task 1.1.1a: Add footprint-summary helper (~4 min)

- Add a small private function `summarizeEraseFootprint(chunk: string): string[]` near
  the `RedrawThrottler` class (or as a module-level helper above it) that tests
  `chunk` for presence of `\x1b[K`, `\x1b[0K`, `\x1b[1K`, `\x1b[2K`, `\x1b[J`, `\x1b[2J`,
  `\x1b[3J` and returns the matching labels, e.g. `["EL", "ED2"]`.
- No control-flow change — pure function, callable from the logging line added in
  Task 1.1.1b.
- Files: `web-app/src/lib/terminal/TerminalStreamManager.ts`

##### Task 1.1.1b: Log discard event in `RedrawThrottler.process()` (~5 min)

- In `process()`, immediately before `this.pendingRedraw = chunk;` (line ~67), add:
  ```ts
  if (this.pendingRedraw !== null && typeof window !== "undefined" &&
      localStorage.getItem("debug-terminal") === "true") {
    console.log('[RedrawThrottler] discarding pending redraw', {
      discardedLength: this.pendingRedraw.length,
      discardedPreview: this.pendingRedraw.slice(0, 40),
      discardedFootprint: summarizeEraseFootprint(this.pendingRedraw),
      newLength: chunk.length,
      newPreview: chunk.slice(0, 40),
      newFootprint: summarizeEraseFootprint(chunk),
    });
  }
  ```
- Files: `web-app/src/lib/terminal/TerminalStreamManager.ts`

##### Task 1.1.1c: Add unit test asserting the discard log fires (~5 min)

- Add a test to `TerminalStreamManager.test.ts` (or a new
  `RedrawThrottler.test.ts` colocated in `__tests__/` if the class isn't already
  independently testable — check existing exports first) that: enables the debug flag via
  `localStorage.setItem`, feeds two consecutive full-redraw chunks with differing erase
  sequences within the 33ms throttle window, and asserts `console.log` was called with
  `'[RedrawThrottler] discarding pending redraw'` and the expected footprint arrays.
- Files: `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`

---

### Epic 1.2: `broadcastControlModeUpdate` drop telemetry

**Goal**: Turn the existing `log.Warn` on channel-full into structured, countable
telemetry that can correlate a drop with a client-visible corruption report (pitfall #7 —
diagnosability first, full backpressure is out of scope for this plan).

#### Story 1.2.1: Add a per-session drop counter and structured log fields

**As a** developer investigating terminal corruption reports, **I want** dropped
control-mode updates to be counted and logged with enough context to correlate against a
client-side timestamp, **so that** I can confirm whether Candidate B fires for a real
corruption occurrence.

**Acceptance Criteria**:
- `TmuxSession` gains an atomic drop counter, incremented on every `default:` branch hit
  in `broadcastControlModeUpdate`.
- The existing `log.Warn` call gains structured fields: dropped-frame byte length,
  running total drop count for this session, and (best-effort) whether the dropped data
  contains an erase sequence (`\x1b[K` family or `\x1b[J` family), reusing the same
  labeling approach as Task 1.1.1a but in Go.
- A new exported or test-visible accessor exposes the counter for tests (no need to wire
  it into a metrics/RPC surface in this plan — that's a stretch item, not required here).
- `go test ./session/tmux/...` still passes.

**Files**:
- `session/tmux/control_mode.go`
- `session/tmux/control_mode_dispatch_test.go` (or a new
  `control_mode_broadcast_test.go` if a cleaner home — check for an existing
  broadcast-focused test file first; none currently exists per the file listing of
  `session/tmux/*control_mode*`)

##### Task 1.2.1a: Add drop counter field to `TmuxSession` (~3 min)

- Add `controlModeDroppedUpdates atomic.Int64` (or equivalent, matching this file's
  existing atomic-field conventions) to the `TmuxSession` struct definition.
- Files: `session/tmux/control_mode.go`

##### Task 1.2.1b: Increment counter and add structured log fields on drop (~5 min)

- In `broadcastControlModeUpdate`'s `default:` branch (line ~692), before/alongside the
  existing `log.Warn` call:
  ```go
  dropped := t.controlModeDroppedUpdates.Add(1)
  hasErase := bytes.Contains(data, []byte("\x1b[K")) ||
      bytes.Contains(data, []byte("\x1b[J"))
  log.Warn("control mode subscriber channel full, dropping update",
      "subscriber", subscriberID, "session", t.sanitizedName,
      "dropped_bytes", len(data), "session_drop_total", dropped,
      "contains_erase_sequence", hasErase)
  ```
- Add `"bytes"` and `sync/atomic` imports if not already present in this file.
- Files: `session/tmux/control_mode.go`

##### Task 1.2.1c: Add a small accessor and unit test for the counter (~5 min)

- Add `func (t *TmuxSession) ControlModeDroppedUpdates() int64 { return
  t.controlModeDroppedUpdates.Load() }` (or package-visible equivalent — follow this
  file's existing exported-accessor conventions; keep it unexported test-only if no
  existing caller needs it, per the interface-pollution-checklist's no-speculative-API
  guidance).
- Add a test using the `newDispatchTestSession` harness pattern from
  `control_mode_dispatch_test.go` (or a new file mirroring its setup): create a session,
  register a subscriber with a channel filled to capacity, call
  `broadcastControlModeUpdate` directly (or via the real trigger path if more direct),
  assert the counter increments and the dropped-bytes/erase-detection fields are correct
  for a fixture payload containing `\x1b[K`.
- Files: `session/tmux/control_mode_dispatch_test.go` (or new adjacent test file)

---

### Epic 1.3: Byte-capture repro harness (stretch, if live repro infeasible)

**Goal**: Since a live Claude Code session repro may not be feasible within a single
implementation session, provide a deterministic way to feed a captured/synthetic byte
sequence through both `RedrawThrottler.process()` and `broadcastControlModeUpdate`
independently and assert what each does with it — using the existing
`web-app/src/lib/test-generators/escape-codes/` fixtures as the source of realistic
EL/ED sequences (`generators.ts`, `library.ts`).

#### Story 1.3.1: Synthetic two-frame corruption repro test

**As a** developer validating a root-cause hypothesis, **I want** a test that constructs
two consecutive redraw frames (a longer line, then a shorter line, differing in erase
footprint) and asserts on the *current* (pre-fix) buggy behavior, **so that** Phase 2/3
fixes have a concrete before/after to verify against.

**Acceptance Criteria**:
- Test constructs frame 1 (`\x1b[<n>A\x1b[K<long text>`) and frame 2
  (`\x1b[<n>A\x1b[K<short text>`) using the existing escape-code generator helpers where
  possible, feeds both through `RedrawThrottler.process()` within the 33ms window, and
  documents (via a failing-then-fixed assertion, or a clearly labeled
  `// EXPECTED TO FAIL until Epic 2.1 lands` comment) that frame 1's content is currently
  lost.
- Equivalent Go-side test feeds the same two frames through
  `broadcastControlModeUpdate` with an artificially saturated subscriber channel and
  documents the current silent-drop behavior.
- This task does not fix anything — it only pins down current (buggy) behavior as a
  reference point.

**Files**:
- `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`
- `session/tmux/control_mode_dispatch_test.go` (or the file created in Task 1.2.1c)

##### Task 1.3.1a: Frontend two-frame repro test (~5 min)

- Files: `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`

##### Task 1.3.1b: Backend two-frame repro test (~5 min)

- Files: `session/tmux/control_mode_dispatch_test.go`

---

## Phase 2: Fix Candidate A — Footprint-Aware Redraw Coalescing

### Epic 2.1: Sequence-aware, non-lossy `RedrawThrottler`

**Goal**: Per pitfall #4 option (a) and ADR-002, never let a pending redraw be silently
replaced if its erase footprint differs from the replacement's — flush the pending frame
first in that case, otherwise coalesce as before. Per pitfall #2, base the "is this a
full redraw" and "what does it erase" classification on the actual erase/cursor
sequences present (reusing the `summarizeEraseFootprint` helper from Task 1.1.1a), not a
single fixed-alternation regex — extend the classifier to also recognize bare `\x1b[K`,
`\x1b[0K`, `\x1b[1K` (currently only `\x1b[2K`/`\x1b[J` are matched).

#### Story 2.1.1: Extend full-redraw classification to bare EL sequences

**As a** user watching a live agent session, **I want** the throttler to correctly
recognize Claude Code's actual EL-based spinner redraw as a "full redraw" candidate,
**so that** the coalescing logic even applies to the sequence class this bug is built on.

**Acceptance Criteria**:
- `isFullRedraw` (or its replacement) recognizes `\x1b[<n>A` followed by any of
  `\x1b[K`, `\x1b[0K`, `\x1b[1K`, `\x1b[2K`, `\x1b[J`, `\x1b[2J`, `\x1b[3J` — not just the
  current two alternatives.
- Existing tests in `TerminalStreamManager.test.ts` covering the current regex's
  matched cases still pass unmodified.
- New test cases added for each newly-recognized EL variant.

**Files**:
- `web-app/src/lib/terminal/TerminalStreamManager.ts`
- `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`

##### Task 2.1.1a: Broaden the full-redraw regex (~4 min)

- Replace the `isFullRedraw` test in `process()` (line ~53) with a pattern (or a small
  named-alternation helper, kept inline — this is a fixed classification of well-known VT
  sequences already used elsewhere in this file, not new content-mutating parsing logic,
  so it does not violate pitfall #1's "no new regex to patch parsing" guidance) covering:
  `/^\x1b\[\d+A(?:\x1b\[[0-2]?K|\x1b\[[0-3]?J)/`.
- Files: `web-app/src/lib/terminal/TerminalStreamManager.ts`

##### Task 2.1.1b: Add tests for each newly-recognized variant (~5 min)

- Add cases for `\x1b[K`, `\x1b[0K`, `\x1b[1K`, `\x1b[3J` inputs to confirm they now
  route through the throttle path (return `null` from `process()` and get held in
  `pendingRedraw`) rather than being passed through immediately.
- Files: `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`

#### Story 2.1.2: Flush pending redraw before replacing it if footprint differs

**As a** user watching a live agent session, **I want** a pending redraw with a wider
erase footprint to never be discarded in favor of a narrower one, **so that** stale
glyphs outside the new frame's erased region are never left on screen.

**Acceptance Criteria**:
- Before `this.pendingRedraw = chunk` executes, if `this.pendingRedraw !== null` and
  `summarizeEraseFootprint(this.pendingRedraw)` is not a subset of
  `summarizeEraseFootprint(chunk)` (i.e. the new chunk's erase does not cover everything
  the old one would have erased), the old `pendingRedraw` is flushed via `this.onFlush`
  immediately (synchronously, before storing the new chunk), rather than being replaced.
- If the new chunk's footprint fully covers (is a superset of or equal to) the old
  chunk's, the existing coalesce-and-replace behavior is preserved (this is the
  legitimate "these are truly redundant" case — no observable behavior change here vs.
  today).
- The Task 1.3.1a repro test from Phase 1 now passes (frame 1's content is no longer
  lost) — update its assertion/remove its `EXPECTED TO FAIL` comment.
- No new setTimeout/interval added — the flush-before-replace is synchronous within
  `process()`.

**Files**:
- `web-app/src/lib/terminal/TerminalStreamManager.ts`
- `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`

##### Task 2.1.2a: Implement footprint-subset check helper (~5 min)

- Add `isFootprintCovered(old: string[], next: string[]): boolean` — returns true if
  every label in `old` is present in `next`, OR if `next` contains `\x1b[2J`/`\x1b[3J`
  (a full-display erase trivially covers any narrower erase). Treat an empty `old`
  footprint (non-erase redraw chunk) as always covered.
- Files: `web-app/src/lib/terminal/TerminalStreamManager.ts`

##### Task 2.1.2b: Wire flush-before-replace into `process()` (~5 min)

- Replace the discard-log block from Task 1.1.1b with:
  ```ts
  if (this.pendingRedraw !== null) {
    const oldFootprint = summarizeEraseFootprint(this.pendingRedraw);
    const newFootprint = summarizeEraseFootprint(chunk);
    if (!isFootprintCovered(oldFootprint, newFootprint)) {
      this.flushPending(); // synchronous — writes old pendingRedraw now
    }
    // (debug-log block from Task 1.1.1b stays, now only fires for the
    // legitimate coalesce case, i.e. after the flush check above)
  }
  this.pendingRedraw = chunk;
  ```
- Note `flushPending()` already clears `throttleTimer` and `pendingRedraw`, so the
  subsequent `if (!this.throttleTimer)` block below correctly re-arms a fresh timer for
  the newly-stored chunk.
- Files: `web-app/src/lib/terminal/TerminalStreamManager.ts`

##### Task 2.1.2c: Update/un-skip the Phase 1 repro test and add direct unit tests (~5 min)

- Update the Task 1.3.1a test to assert the previously-lost frame is now flushed (i.e.
  both frames' content reach `onFlush`, in order, rather than only the second).
- Add a direct unit test constructing `pendingRedraw` with an EL footprint and replacing
  it with a chunk whose footprint doesn't cover the old one — assert `onFlush` is called
  twice (once for the flushed old chunk, once for the new one after its own throttle
  window elapses).
- Files: `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`

---

## Phase 3: Fix Candidate B — Control-Mode Drop Diagnosability

### Epic 3.1: Confirm Phase 1 telemetry is sufficient; escalate only if needed

**Goal**: Per ADR-001, Candidate B's fix in this plan is scoped to the diagnosability
gap (pitfall #7) already closed by Epic 1.2 — this repo's `TerminalData_FlowControl`
plumbing being entirely unreachable from browser clients is a larger architectural gap
than this bug fix should take on. This phase's job is to decide, using Epic 1.2's
counter, whether anything beyond telemetry is warranted for this specific bug, and if the
counter shows drops correlating with erase-sequence content in practice, size a minimal,
scoped mitigation — not full backpressure.

#### Story 3.1.1: Minimal scoped mitigation — grow the subscriber buffer or drop the least-recent frame instead of the newest

**As a** user watching a live agent session, **I want** a full control-mode subscriber
channel to prefer dropping older, less-relevant data over the newest frame, **so that**
what reaches the client is still consistent even under sustained backpressure.

**Acceptance Criteria**:
- `broadcastControlModeUpdate`'s `default:` branch, instead of dropping the *new*
  incoming `data` when the channel is full, attempts to drain one stale item from the
  channel (non-blocking) and then send the new data — i.e. prefer freshness over
  ordering-perfect delivery, since a stale frame is strictly less useful to a live
  terminal viewer than the newest one. (This is a bounded, local change — not a redesign
  of the flow-control system.)
- If the drain-and-retry also fails (extremely unlikely with a single-consumer channel),
  fall back to the existing log-and-drop-new behavior, still incrementing the Epic 1.2
  counter.
- Existing tests in `control_mode_dispatch_test.go` /
  `control_mode_refcount_test.go` continue to pass.
- New test confirms: given a full channel, the newest frame is delivered (possibly
  displacing the oldest buffered one) rather than being the one silently discarded.

**Files**:
- `session/tmux/control_mode.go`
- `session/tmux/control_mode_dispatch_test.go` (or the file from Task 1.2.1c)

##### Task 3.1.1a: Implement drain-oldest-then-send in `broadcastControlModeUpdate` (~5 min)

- Replace the `default:` branch body with:
  ```go
  default:
      select {
      case <-ch:
          // Dropped the oldest buffered frame to make room; retry send.
      default:
      }
      select {
      case ch <- data:
      default:
          dropped := t.controlModeDroppedUpdates.Add(1)
          hasErase := bytes.Contains(data, []byte("\x1b[K")) ||
              bytes.Contains(data, []byte("\x1b[J"))
          log.Warn("control mode subscriber channel full, dropping update",
              "subscriber", subscriberID, "session", t.sanitizedName,
              "dropped_bytes", len(data), "session_drop_total", dropped,
              "contains_erase_sequence", hasErase)
      }
  ```
- Files: `session/tmux/control_mode.go`

##### Task 3.1.1b: Add a test for drain-oldest-then-send behavior (~5 min)

- Construct a subscriber with a small channel filled to capacity with sentinel values,
  call `broadcastControlModeUpdate` once more, and assert: the channel's contents now
  include the new data (not the case that it was dropped), and the counter only
  increments in the true double-failure case.
- Files: `session/tmux/control_mode_dispatch_test.go` (or file from Task 1.2.1c)

---

## Phase 4: Regression Coverage

### Epic 4.1: Monotonic row-erase-coverage assertion across coalesced frames

**Goal**: Per pitfall #6, add a regression test that would have caught this class of bug
directly — asserting that across any sequence of coalesced `RedrawThrottler` frames, the
union of erased footprints actually flushed to the terminal is never a strict subset of
what the original (uncoalesced) sequence would have erased.

#### Story 4.1.1: Property-style coverage test using existing escape-code generators

**As a** developer maintaining the terminal streaming pipeline, **I want** an automated
test that fails if a future change reintroduces lossy coalescing, **so that** this bug
class cannot silently regress.

**Acceptance Criteria**:
- Test generates N (e.g. 5-10) synthetic full-redraw frames with varying erase
  footprints and lengths using `web-app/src/lib/test-generators/escape-codes/generators.ts`
  helpers, feeds them through `RedrawThrottler` at intervals inside and outside the 33ms
  window, collects every chunk passed to `onFlush`, and asserts the union of erase labels
  across all flushed chunks is a superset of the union of erase labels across all input
  frames.
- Test is deterministic (no reliance on real wall-clock timing beyond fake timers —
  use `jest.useFakeTimers()` consistent with any existing timer-based tests in this
  suite).
- Runs as part of `cd web-app && npx jest --no-coverage --testPathPatterns="TerminalStreamManager"`.

**Files**:
- `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`

##### Task 4.1.1a: Write the coverage-union property test (~5 min)

- Files: `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`

##### Task 4.1.1b: Run and confirm green under `make quick-check` scope (~3 min)

- Run `cd web-app && npx jest --no-coverage --testPathPatterns="TerminalStreamManager"`
  and `go test ./session/tmux/...` to confirm all Phase 1-4 tests pass together.
- No files changed — verification only.

---

## Deferred / Out of Scope (noted, not planned in detail here)

These were surfaced by research as real but secondary/contributing factors, or as
mitigations belonging to sibling plans. Per the requirements' scope constraints, they are
explicitly **not** part of this plan and should be tracked separately if pursued:

- **16KB chunk-boundary escape-awareness** in `writeDirectWithFlowControl`/`enqueueWrite`
  (`TerminalStreamManager.ts:361-495`) — no escape-sequence awareness at slice
  boundaries. Not implicated as primary here (both candidates operate upstream of
  chunking, on whole frames), but worth a follow-up ticket if future evidence points to
  it.
- **WebGL renderer stress** — `@xterm/addon-canvas ^0.7.0` is already a dependency as a
  fallback; verifying/wiring an automatic WebGL→canvas fallback trigger is tracked
  separately (`terminal-jank.md` / `terminal-robustness/`), not this plan.
- **DEC mode 2026 synchronized output** — xterm.js 6.0 supports it, but it requires
  Claude Code/Ink to emit the synchronized-output wrapper sequences; this is not a
  stapler-squad-side fix and is out of scope.
- **`requestFullResync()`/`CurrentPaneRequest`** (from `terminal-visibility-resync/`) —
  an existing "nuke and repaint" primitive that could serve as a bounded escape hatch if
  Phase 1 telemetry shows drops/discards happening at a rate the Phase 2/3 fixes don't
  fully address in practice. Not required by this plan's acceptance criteria; note for a
  future session if telemetry warrants it.

## No New Dependencies

This plan introduces no new external dependencies (npm packages or Go modules). All
changes are confined to existing files: `TerminalStreamManager.ts`, its test suite,
`control_mode.go`, and its test suite. Per Step 2 of the planning brief, no
dependency-justification ADR is required; the two ADRs in this plan instead document the
architectural/process decisions (instrument-first-fix-both, and the footprint-coverage
coalescing strategy).
