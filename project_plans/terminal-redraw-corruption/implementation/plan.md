# Implementation Plan: terminal-redraw-corruption

**Feature**: Fix stray leading-character fragments from a previous, longer status line
surviving under a new, shorter redrawn line in the web-rendered xterm.js terminal.
**Date**: 2026-08-06
**Status**: Phase 3 complete — plan drafted and adversarially reviewed across 9 rounds; round 9 verdict is CLEAN (0 blockers, 0 concerns, 1 minor addressed inline). Phase 4 (validation) not yet run; implementation deferred to a fresh Phase 5 session.
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
  the `RedrawThrottler` class (or as a module-level helper above it) that tests only the
  same leading structural window `isFullRedraw` already scopes to (**Round 8 review
  finding (Minor, fixed here)**: this window is the cursor-repositioning prefix —
  cursor-up, CR, or absolute CUP, per Round 6/7's widening, not "cursor-up sequence"
  alone as an earlier draft of this task said — plus the immediately-following erase
  sequence — not an unbounded scan of the whole chunk body) for presence of `\x1b[K`,
  `\x1b[0K`, `\x1b[1K`, `\x1b[2K`, `\x1b[J`,
  `\x1b[2J`, `\x1b[3J`, and returns the matching labels as `["EL0", "ED2"]` etc. — bare
  `\x1b[K` and `\x1b[0K` are both labeled `EL0` (they are defined as equivalent: "erase
  from cursor to end of line"), `\x1b[1K` as `EL1` ("erase start of line to cursor"), and
  `\x1b[2K` as `EL2` ("erase entire line"). **Round 5 review finding (Concern #2, fixed
  here)**: an earlier draft lumped all EL forms under one shared `"EL"` label, which would
  have let Task 2.1.2a's `isFootprintCovered` treat a narrower erase (e.g. `EL1`,
  start-of-line-to-cursor) as fully covering a wider one (e.g. `EL2`, whole line) purely
  because both mapped to the same label — the same class of "erase variants are not
  fungible" bug Round 4 caught in the Go-side `hasEraseSequence` substring matcher, one
  layer up. Distinguishing the labels by parameter closes that gap. Scoping the scan to
  the same window `isFullRedraw` uses (rather than whole-chunk substring scanning) avoids
  a false footprint match from an escape-like byte sequence appearing later in a large
  chunk (e.g. inside captured subprocess output rendered as literal text) — this repo has
  been burned before by fragile whole-content ANSI-sniffing (research pitfall #1).
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

##### Task 1.2.1a: Add drop/evict counter fields and shared erase-detection helper to `TmuxSession` (~4 min)

- Add `controlModeDroppedUpdates atomic.Int64` and `controlModeEvictedStale atomic.Int64`
  (or equivalent, matching this file's existing atomic-field conventions) to the
  `TmuxSession` struct definition. The eviction counter is used by Phase 3 (Task 3.1.1a);
  defining it now keeps both counters in one place.
- Add a small package-level helper `hasEraseSequence(data []byte) bool`. **Round 4
  adversarial review finding (BLOCKER, fixed here)**: a naive
  `bytes.Contains(data, []byte("\x1b[K")) || bytes.Contains(data, []byte("\x1b[J"))` check
  does NOT match the parameterized EL/ED variants (`\x1b[0K`, `\x1b[1K`, `\x1b[2K`,
  `\x1b[2J`, `\x1b[3J`) — the parameter digit sits between `[` and the letter, breaking the
  bare-form substring match — and Claude Code's TUI is understood (per this plan's own
  Phase 1/Phase 2 research and the JS-side `summarizeEraseFootprint` fixture list in Task
  1.1.1a) to emit exactly these parameterized forms. A helper that misses them would
  silently defeat Story 3.1.1's entire purpose. Use a regex (compiled once at package
  init) instead of bare substring checks:
  ```go
  var eraseSequenceRE = regexp.MustCompile(`\x1b\[[0-3]?[KJ]`)

  func hasEraseSequence(data []byte) bool {
      return eraseSequenceRE.Match(data)
  }
  ```
  This matches all of `\x1b[K`, `\x1b[0K`, `\x1b[1K`, `\x1b[2K` (EL forms) and `\x1b[J`,
  `\x1b[2J`, `\x1b[3J` (ED forms) in one pass. Add `"regexp"` to this file's imports.
  Phase 1 telemetry and the Phase 3 eviction-safety check (Task 3.1.1a) share this one
  definition instead of duplicating the byte-matching logic. Unlike the JS-side
  `summarizeEraseFootprint` (Task 1.1.1a), this intentionally scans the *whole* payload
  rather than a leading structural window: it is a boolean safety gate for "is it ever
  unsafe to evict this frame," where whole-payload scanning is over-inclusive in the safe
  direction (a false positive only means Task 3.1.1a is more conservative about evicting,
  never less) — it does not drive a behavioral rewrite the way the JS-side footprint
  comparison does in Phase 2, so the whole-chunk-scan risk research pitfall #1 warns about
  does not apply the same way here.
  **Note (Round 5 review, Minor #1)**: this regex's `[0-3]?` parameter range is
  intentionally broader than the JS-side `summarizeEraseFootprint`/`isFullRedraw` fixture
  list (Task 1.1.1a), which is scoped to the specific literal sequences `\x1b[K`,
  `\x1b[0K`, `\x1b[1K`, `\x1b[2K`, `\x1b[J`, `\x1b[2J`, `\x1b[3J` and so does not accept a
  nonstandard `\x1b[3K` (EL only defines parameters 0-2) the way this Go regex does. This
  asymmetry is harmless — the Go side being over-inclusive only makes it more conservative
  about not evicting, consistent with the "false positive is safe" direction stated above —
  it is called out here so a future reader doesn't mistake the difference for an
  intentional, meaningful design choice.
- Files: `session/tmux/control_mode.go`

##### Task 1.2.1b: Increment counter and add structured log fields on drop (~5 min)

- In `broadcastControlModeUpdate`'s `default:` branch (line ~692), before/alongside the
  existing `log.Warn` call:
  ```go
  dropped := t.controlModeDroppedUpdates.Add(1)
  log.Warn("control mode subscriber channel full, dropping update",
      "subscriber", subscriberID, "session", t.sanitizedName,
      "dropped_bytes", len(data), "session_drop_total", dropped,
      "contains_erase_sequence", hasEraseSequence(data))
  ```
- `"regexp"` (added by Task 1.2.1a's helper) and `sync/atomic` are already imported by the
  time this task runs — no new imports needed here.
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
  `broadcastControlModeUpdate` directly (call it directly — do not route through the real
  tmux control-mode read loop; the harness pattern already exists for this and a live
  trigger path adds no coverage this task needs), assert the counter increments and the
  dropped-bytes/erase-detection fields are correct for a fixture payload containing
  `\x1b[K`.
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
  pins the current (pre-fix) buggy behavior using `test.failing(...)` (Jest's built-in
  inverted-expectation mechanism — the test body asserts the *correct* post-fix behavior
  directly, e.g. "onFlush receives both frames' content," and `test.failing` makes Jest
  treat that assertion's current failure as expected; once Epic 2.1 lands and the
  assertion starts passing, rename it to `test(...)` so CI fails loudly if this is
  forgotten). This is a real Jest 29+ API, not a comment convention, so a CI run between
  Phase 1 and Phase 2 landing sees an expected, documented failure rather than a red
  build.
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

##### Task 1.3.1c: Manual live-repro checkpoint before treating Phase 1 as done (~5 min, no files changed)

- Per CLAUDE.md's "Manual/interactive testing" section, run a second, isolated
  `stapler-squad` instance (`PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test`), enable
  `localStorage.setItem("debug-terminal", "true")` in the browser dev console against a
  live session, and deliberately trigger a fast status-line redraw (e.g. a Claude Code
  session doing rapid tool-call spinner updates) while watching the browser console and
  server logs for the Epic 1.1/1.2 telemetry to actually fire. This is the step ADR-001's
  own Consequences section flags as required and not automatically scheduled — do not mark
  Phase 1 complete in a real Phase 5 session without having run it at least once.
- If the original "stapler-squad-perf" session (or its scrollback/recording, if retained)
  is available, replay its literal captured bytes through the Epic 1.3 harnesses in
  addition to the synthetic generator-built frames — the requirements' captured fragments
  ("r ", "ss ", "x) ", "o) ") look like they may span multiple redrawn rows/positions
  (e.g. a multi-line Ink status block), not one row repeatedly redrawn, so the synthetic
  single-row cases in Epic 1.3/4.1 should be treated as necessary-but-not-sufficient
  coverage until confirmed against real captured bytes or a live repro. If no such capture
  exists, note this explicitly in the Phase 1 findings rather than silently treating the
  single-row synthetic case as fully representative.
- **Hard gate, added per Round 6 adversarial review (Blocker)**: as part of this checkpoint,
  explicitly capture and record which cursor-repositioning idiom the live redraw actually
  uses — relative cursor-up (`\x1b[<n>A`), bare carriage return (`\r`), absolute positioning
  (`\x1b[<row>;<col>H`/`\x1b[H`), or none (cursor already correctly positioned) — relative to
  the erase sequence. Compare this against Task 2.1.1a's broadened classifier (Story
  2.1.1's acceptance criteria below). **If the observed byte pattern is not one the
  classifier recognizes, this blocks Phase 5 sign-off on Story 2.1.1** — do not proceed to
  implementation with a classifier confirmed not to match the actual bug shape; instead the
  classifier must be extended to cover the observed pattern before Phase 5 starts. This
  upgrades this checkpoint from an informational note to a required, blocking result,
  because Task 2.1.1a's regex is itself an unconfirmed hypothesis about which idiom Claude
  Code's TUI uses (see `research/pitfalls.md` Fix constraints item 2 and
  `research/stack.md:104-105`).
- **Second, independent hard-gate dimension, added per Round 8 adversarial review
  (Blocker)**: capturing "which repositioning idiom" is not sufficient on its own —
  `research/pitfalls.md` §6 item 2 names a second, independent classifier gap: **byte
  order**, i.e. whether the erase sequence appears *before or after* the repositioning
  sequence. Task 2.1.1a's regex (both the Round 6 form-widening and the Round 7
  required-prefix fix) hard-codes reposition-then-erase order; it does not match an
  `ansi-escapes`-style erase-then-cursor-up (or erase-then-CR, erase-then-CUP) ordering,
  which is the exact idiom research cited as a reason the original classifier was
  insufficient. This checkpoint must therefore explicitly answer a second question,
  separate from "which idiom": **does the erase sequence come before or after the
  repositioning sequence in the captured bytes?** Do not infer the order from which idiom
  is present — record it as its own explicit observation, because the failure mode this
  gate exists to catch is exactly an implementer defaulting to "reposition-then-erase"
  out of habit without checking. If the observed order is erase-then-reposition, this
  blocks Phase 5 sign-off on Story 2.1.1 in the same way an unrecognized repositioning
  idiom does — do not proceed with a classifier confirmed not to match the actual byte
  order; the regex must be extended with a second alternation branch (erase followed by
  one of the three repositioning forms) before Phase 5 starts, applying the same
  data-loss trade-off analysis Round 7 required for the prefix-form widening (i.e. verify
  through `isFootprintCovered`/Task 2.1.2b that the new branch cannot let a non-redraw
  chunk that merely starts with an erase byte get silently discarded — the erase-then-
  reposition branch is still bounded by requiring one of the three named repositioning
  forms to immediately follow, so it carries the same narrowness argument as the
  reposition-then-erase branch, not the unbounded "any bare erase" shape Round 7 rejected).
- **Destination artifact for the observation (Round 7 review concern, fixed here)**: record
  the captured byte pattern and its comparison against Task 2.1.1a's classifier as a dated
  addendum appended to this plan's Root Cause section (`## Root Cause`, above), under a
  `### Task 1.3.1c Live-Repro Finding (<date run>)` heading — not just as an unwritten
  verbal confirmation. This gives a future reviewer (or a Phase 5 implementer) a concrete,
  checkable record of whether the gate was actually satisfied, rather than having to trust
  that the checkpoint happened at all.
- No files changed by this task — it is a verification checkpoint, not new code.

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
- `isFullRedraw` (or its replacement) recognizes a required cursor-repositioning prefix —
  any of `\x1b[<n>A` (relative cursor-up), `\r` (carriage return), or
  `\x1b[<row>;<col>H`/`\x1b[H` (absolute CUP) — followed by any of `\x1b[K`, `\x1b[0K`,
  `\x1b[1K`, `\x1b[2K`, `\x1b[J`, `\x1b[2J`, `\x1b[3J`. This widens both dimensions the
  original single-shape regex missed: erase-type alternation (fixed as originally planned)
  and the cursor-repositioning idiom itself (added per Round 6 adversarial review's
  Blocker — see Task 2.1.1a for why the cursor-up-only prefix was insufficient). The prefix
  itself stays **required, not optional** — a bare erase with no repositioning is
  deliberately NOT matched (see Task 2.1.1a's Round 7 fix for why making it optional was a
  data-loss regression).
- Existing tests in `TerminalStreamManager.test.ts` covering the current regex's
  matched cases still pass unmodified.
- New test cases added for each newly-recognized EL variant, AND for each newly-recognized
  cursor-repositioning prefix form (`\r`+EL, absolute CUP+EL) — AND a negative test
  confirming a bare erase with no repositioning prefix is NOT classified as a full redraw
  (Round 7 regression guard, see Task 2.1.1a/2.1.1b).
- **Gated on Task 1.3.1c's live-repro result**: if the live-repro checkpoint observes a
  cursor-repositioning idiom not covered by this list, this criterion is not satisfied until
  the classifier is extended to cover it — Phase 5 must not proceed on this story with a
  classifier known not to match the real-world byte pattern. If the observed idiom turns out
  to be a bare erase with no repositioning, this must be paired with making Story 2.1.2's
  coalesce-replace path always flush-before-replace (see Task 2.1.1a's note) — not added to
  `isFullRedraw` alone.
- **Second live-repro gate dimension, added per Round 8 adversarial review (Blocker)**:
  the classifier above hard-codes reposition-then-erase byte order. `research/pitfalls.md`
  §6 item 2 names erase-then-reposition (an `ansi-escapes`-style ordering) as an
  independent, equally-plausible idiom this classifier does not yet match. This criterion
  is not satisfied until Task 1.3.1c's live-repro checkpoint has explicitly recorded which
  order the real redraw uses — if it observes erase-then-reposition, the classifier must
  gain a second alternation branch (erase followed by one of the three repositioning
  forms) before this story can be signed off for Phase 5, following the same
  `isFootprintCovered` narrowness analysis Task 2.1.1a's Round 7 fix required.

**Files**:
- `web-app/src/lib/terminal/TerminalStreamManager.ts`
- `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`

##### Task 2.1.1a: Broaden the full-redraw regex (~4 min)

- **Round 6 review finding (BLOCKER, fixed here)**: an earlier draft of this task only
  widened the erase-type alternation while leaving a hard-required leading `\x1b[<n>A`
  (relative cursor-up) prefix untouched. `research/pitfalls.md`'s "Fix constraints" item 2
  and `research/stack.md:104-105` both explicitly warn that this exact shape — cursor-up
  immediately followed by erase — is not the only redraw idiom in the wild: a same-row
  spinner overwrite via bare `\r` (carriage return), an `ansi-escapes`-style erase-then-
  cursor-up ordering, or absolute cursor positioning (`\x1b[<row>;<col>H`) would all fail to
  match a cursor-up-only prefix, and the requirements' own captured fragments (`"r "`,
  `"ss "`, `"x) "`, `"o) "`) are single-row prefix survivals consistent with a same-row,
  no-cursor-up overwrite — not necessarily the multi-line cursor-up case. Shipping a fix
  whose classifier only recognizes the cursor-up shape risks Phase 2 never engaging for the
  actual bug while still looking complete (Phase 4's regression test, built on the same
  generator helpers, would pass without ever exercising the real redraw shape).
- Replace the `isFullRedraw` test in `process()` (line ~53) with a pattern (or a small
  named-alternation helper, kept inline — this is a fixed classification of well-known VT
  sequences already used elsewhere in this file, not new content-mutating parsing logic,
  so it does not violate pitfall #1's "no new regex to patch parsing" guidance) that treats
  the cursor-repositioning prefix as **required, but multi-form**, not hard-required
  cursor-up specifically:
  `/^(?:\x1b\[\d+A|\r|\x1b\[\d+;\d+H|\x1b\[H)(?:\x1b\[[0-2]?K|\x1b\[[0-3]?J)/`.
  This recognizes, in addition to the original cursor-up+erase form: bare `\r`+erase
  (carriage-return-then-EL same-row overwrite), and absolute CUP (`\x1b[row;colH` or bare
  `\x1b[H`) + erase.
  **Round 7 review finding (BLOCKER, fixed here)**: an earlier draft of this task also made
  the entire prefix group optional, so a bare leading erase sequence with *no*
  repositioning at all would match too. That was a genuine new data-loss risk, not a safe
  over-match: Story 2.1.2's flush-before-replace logic only flushes the old `pendingRedraw`
  when the new chunk's footprint does *not* cover it — when the new chunk's footprint *does*
  cover the old one (the "legitimate redundant" branch), the old chunk is silently discarded
  without ever being flushed (Task 2.1.2b). An ordinary, non-redraw chunk that happens to
  start with a bare erase (e.g. a TUI clearing part of a line as one step in a larger,
  non-redraw update) would have been misclassified as a redraw candidate and placed in
  `pendingRedraw`; if a subsequent *genuine* full-redraw chunk's footprint were a superset,
  that misclassified chunk's content would be permanently discarded — the exact class of bug
  this whole plan exists to fix, reintroduced by the fix itself. The no-prefix branch is
  removed: a repositioning prefix (cursor-up, CR, or absolute CUP) is required, matching the
  three concrete idioms research named, and dropping the speculative fourth (bare erase,
  no repositioning) that research never actually described as an observed redraw shape.
- **Required regression test (Round 7 fix)**: Task 2.1.1b must include a negative test
  asserting that a bare erase sequence with no repositioning prefix (e.g. a chunk starting
  with `\x1b[K` alone, no leading `\r`/cursor-up/CUP) is classified as `isFullRedraw ===
  false` — guarding against a future re-introduction of the no-prefix branch this round
  removed.
- Because this classifier shape is itself a hypothesis (this repo has not yet confirmed
  which byte pattern Claude Code's TUI actually emits), Task 1.3.1c's manual live-repro
  checkpoint is upgraded from an informational note to a **hard gate**: see the updated
  Acceptance Criteria on Story 2.1.1 and Task 1.3.1c below — a live repro that shows a
  redraw shape this regex still doesn't match must block Phase 5 sign-off on Story 2.1.1,
  not just get silently noted. If a live-repro result shows the actual idiom is a bare erase
  with no repositioning after all, re-add that branch *together with* changing Story 2.1.2's
  coalesce-replace path to always flush before replacing (never silently discard), so the
  widening cannot reintroduce this same data-loss shape — do not re-add the no-prefix branch
  on its own.
- **Known, research-named gap not yet addressed by this regex (Round 8 adversarial review,
  BLOCKER)**: the alternation above hard-codes reposition-then-erase order — the first
  group (repositioning) must match before the second (erase) is even attempted.
  `research/pitfalls.md` §6 item 2 explicitly names the reverse order — an
  `ansi-escapes`-style erase-then-cursor-up (or erase-then-CR, erase-then-CUP) idiom — as a
  second, independent dimension the original single-shape regex was too narrow on,
  distinct from the repositioning-*form* widening Round 6/7 already fixed. This regex does
  not yet handle that ordering. This is gated on Task 1.3.1c's live-repro checkpoint the
  same way the "bare erase, no repositioning" contingency above is: the checkpoint must
  explicitly record which byte order (erase-first or reposition-first) the real redraw
  uses, not just which repositioning form. If live-repro confirms erase-then-reposition
  ordering occurs, this regex must gain a second alternation branch matching erase followed
  by one of the three repositioning forms (e.g.
  `(?:\x1b\[[0-2]?K|\x1b\[[0-3]?J)(?:\x1b\[\d+A|\r|\x1b\[\d+;\d+H|\x1b\[H)` as an additional
  alternative to the existing reposition-then-erase form) — applying the same
  `isFootprintCovered`/Task 2.1.2b narrowness analysis Round 7 required before accepting
  this widening: the erase-then-reposition branch is still bounded by requiring one of the
  three named repositioning forms to immediately follow the erase, so it carries the same
  "three concrete, research-named idioms" narrowness argument as the existing branch, not
  the unbounded "any bare erase" shape Round 7 rejected. Do not ship this ordering gap
  silently — it must be resolved (via live-repro confirming it doesn't occur, or via the
  added branch above) before Phase 5 sign-off on Story 2.1.1. **(Round 9 minor)**: if this
  branch is ever added, revisit Task 1.1.1a's scan-window prose too — it currently
  describes one ordering (prefix then erase); `summarizeEraseFootprint` itself is
  presence-based, not order-sensitive, so no behavior change is needed, but the prose
  should say so explicitly rather than silently going stale again the way it did between
  Round 6 and Round 8.
- Files: `web-app/src/lib/terminal/TerminalStreamManager.ts`

##### Task 2.1.1b: Add tests for each newly-recognized variant (~5 min)

- Add cases for `\x1b[K`, `\x1b[0K`, `\x1b[1K`, `\x1b[3J` (with a cursor-up prefix) inputs
  to confirm they now route through the throttle path (return `null` from `process()` and
  get held in `pendingRedraw`) rather than being passed through immediately.
- Add cases for `\r`+`\x1b[2K` and `\x1b[H`+`\x1b[2K` (CR- and absolute-CUP-prefixed erases)
  confirming they also route through the throttle path.
- Add the Round 7 regression guard: a bare `\x1b[2K` with no repositioning prefix at all
  must NOT be classified as a full redraw (`isFullRedraw` returns `false` / the chunk is
  passed through immediately rather than stored in `pendingRedraw`).
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
  lost) — remove its `test.failing(...)` wrapper (see Task 1.3.1a; this plan previously
  called it an "`EXPECTED TO FAIL` comment," which was imprecise terminology for the same
  Jest `test.failing` mechanism — corrected here per Round 4 review Minor #3) and convert
  it to a normal `test(...)` with the now-passing assertion.
- No new setTimeout/interval added — the flush-before-replace is synchronous within
  `process()`.

**Files**:
- `web-app/src/lib/terminal/TerminalStreamManager.ts`
- `web-app/src/lib/terminal/__tests__/TerminalStreamManager.test.ts`

##### Task 2.1.2a: Implement footprint-subset check helper (~5 min)

- Add `isFootprintCovered(old: string[], next: string[]): boolean` — returns true if
  every label in `old` is present in `next` (exact label match — `EL0`, `EL1`, `EL2` are
  each their own label per Task 1.1.1a's Round 5 fix and are NOT treated as covering one
  another; only `EL2` (whole-line erase) is a strict superset of `EL0`/`EL1` in terms of
  screen region cleared, so also treat `next` containing `EL2` as covering an `old`
  footprint of `EL0` and/or `EL1`), OR if `next` contains `\x1b[2J`/`\x1b[3J` (a
  full-display erase trivially covers any narrower erase). Treat an empty `old` footprint
  (non-erase redraw chunk) as always covered — note: this branch is expected to be
  unreachable in practice, since `pendingRedraw` can only be populated by chunks that
  already matched the (now-broadened) `isFullRedraw` erase-sequence requirement, so `old`
  should never legitimately be empty here; keep the check as defensive dead code rather
  than asserting/throwing, since a false assumption about `isFullRedraw`'s guarantees
  should degrade to "treat as covered" (today's behavior), not a runtime crash.
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

#### Story 3.1.1: Minimal scoped mitigation — evict a stale *non-erase* frame instead of dropping the newest, never discard an already-buffered erase-bearing frame, and make the behavior instantly revertible

**As a** user watching a live agent session, **I want** a full control-mode subscriber
channel to prefer discarding older, non-erase data over either the newest frame or an
already-buffered erase-bearing frame, **so that** an erase sequence already sitting in
the buffer is never the payload silently lost — it may be redelivered slightly
out of order relative to frames queued behind it when the buffer is already saturated,
but it is never dropped — while giving on-call a way to instantly revert to the old
behavior if this mitigation misbehaves under real load, since it changes the hot
broadcast path for every live session process-wide. (Scoped down from an earlier,
overstated "never lost in either direction" framing per adversarial review round 3's
concern #1 — see the round 3 design correction below for what is and isn't guaranteed.)

**Design correction from the original draft (adversarial review blockers #1/#2/#3)**:
draining the oldest buffered frame unconditionally just relocates data loss — if the
evicted frame happened to carry the erase sequence, the visible corruption is identical,
only shifted earlier in the stream. The fix must (a) inspect the oldest frame before
evicting it and refuse to evict an erase-bearing one, falling back to the original
drop-new+log behavior instead; (b) emit a distinct, countable event on every successful
eviction, not only on double-failure, so Epic 1.2's telemetry stays meaningful; and (c) be
gated behind an instantly-revertible switch given the blast radius.

**Design correction, round 2 (adversarial review round-2 blockers)**: a raw Go `chan
[]byte` has no peek or push-to-front operation — dequeuing the oldest frame to inspect it
and then `ch <- oldest`-ing it back always re-appends it at the **tail**, silently
reordering the stream relative to frames already queued behind it. Separately, the
revert flag must not be a package-level `var` computed once at init from `os.Getenv` —
`t.Setenv` in a test cannot affect an already-computed value, which was silently making
Task 3.1.1b's Test 3 untestable. The flag is now a function that re-reads the env var on
each full-buffer event (not hot-path-sensitive: it only executes when a subscriber buffer
is already full, not on every broadcast).

**Design correction, round 3 (adversarial review round-3 blocker — reverts round 2's
transport swap)**: round 2's fix (replacing the raw `chan []byte` with a mutex-protected
`controlModeQueue` + `sync.Cond`) was itself rejected on round 3 review: the consumer
(`server/services/connectrpc_websocket.go`) reads subscriber updates via `select { case
data, ok := <-updateChan: ... }`, and `SubscribeToControlModeUpdates() (string, chan
[]byte)` is a `ProcessManager` interface method with 4 implementations
(`session/tmux/control_mode.go`, `session/native_process_manager.go`,
`session/tmux_process_manager.go`, `session/tmux_backend.go`). A `sync.Cond` cannot
participate in a `select`, so swapping the transport type would either break that
interface across all 4 implementations and the websocket consumer, or require an
undesigned bridging goroutine with its own close/teardown-on-unsubscribe logic — exactly
the class of goroutine-leak bug this codebase already had to fix once
(`aeb5c1a6f`). That is too large a blast radius for this bug fix's actual scope.

The corrected, minimal-blast-radius design **keeps the raw `chan []byte`** and the
existing `SubscribeToControlModeUpdates` interface completely unchanged. It accepts the
consequence flagged by round 2's own "option (a)" — dequeue-then-reinsert on a channel
cannot guarantee front position is preserved — and instead of eliminating that
possibility, states it honestly: an erase-bearing frame that is dequeued to be inspected
and found ineligible for eviction is put back via `ch <- oldest`, which appends it at the
tail of whatever is currently buffered. This is **strictly better than the pre-Phase-3
behavior only in one respect (no data is ever discarded)**; it does not guarantee FIFO
order is preserved when a full buffer already forces a compromise. Given the buffer is
100 slots and this branch only executes when the buffer is already saturated (an already
-degraded state pre-Phase-3 would have silently dropped data in), a same-frame reorder by
at most one position among up to 100 buffered frames is judged an acceptable, explicitly
documented trade-off — not silently discovered later.

**Acceptance Criteria**:
- `broadcastControlModeUpdate`'s `default:` (channel-full) branch dequeues the oldest
  buffered frame via a non-blocking `select`/`default` receive and checks it with the
  shared `hasEraseSequence` helper (Task 1.2.1a):
  - If the oldest frame **carries an erase sequence**, it is re-sent via `ch <- oldest`
    (a non-blocking send guarded by `select`/`default`, since there is now exactly one
    free slot) and the **new** frame is dropped instead, via the existing
    `controlModeDroppedUpdates` counter + `log.Warn` path. **Documented trade-off**: this
    re-send places the frame at the tail of the current buffer, not back at the front —
    order relative to frames already queued behind it is not preserved. This is
    explicitly weaker than "never disturbed" and is the reason this branch is judged
    CONCERN-worthy but acceptable: no data is lost, but delivery order across a
    already-saturated buffer is not guaranteed.
  - If the oldest frame **does not** carry an erase sequence, it is discarded (not
    re-queued) and the new frame is sent in its place via a non-blocking `select`. This is
    a genuinely new class of event — a stale non-erase eviction — and increments the
    **new** `controlModeEvictedStale` counter (Task 1.2.1a) with its own structured log
    line (`session_evict_total`, `evicted_bytes`), every time it happens, not only on a
    subsequent double-failure.
  - The re-send-after-dequeue (`ch <- oldest`) and the send-after-eviction (`ch <- data`)
    each carry a defensive `select`/`default` fallback to the drop-new+log path in case the
    channel unexpectedly has no room. Under the current single-producer-per-session
    concurrency model (see the concurrency note below) this fallback is unreachable in
    practice — no other goroutine can consume the one slot just freed except the real
    consumer, which only frees more room — but it is kept as a defensive guard rather than
    an unchecked `ch <- x` that would panic if that invariant is ever violated.
- The whole eviction strategy is gated behind `controlModeEvictStaleEnabled()`, a
  **function** (not a package-level `var`) that reads
  `os.Getenv("STAPLER_SQUAD_CONTROL_MODE_EVICT_STALE") != "false"` fresh on every call —
  checked at the top of the `default:` branch. If it returns false,
  `broadcastControlModeUpdate` falls straight back through to the pre-Phase-3
  drop-new+log behavior with no eviction attempt at all, so on-call can revert via env var
  + process restart without a code rollback if this misbehaves under real load, and so
  `t.Setenv` in tests actually takes effect (each call re-reads the environment, unlike
  round 1's package-level `var` which computed the value once at init and could not
  observe a test's `t.Setenv`).
- An erase-bearing frame is not the one **discarded** by this mechanism during normal
  operation (single control-mode generation) — it may be reordered relative to frames
  already queued behind it (documented above), but it is delivered. This replaces both the
  original overstated "still consistent... under sustained backpressure" claim and round
  1/2's overstated "never... in either direction" framing (neither accounted for the
  reordering this design accepts). **Scoped down per Round 5 review (Blocker #1)**: this
  guarantee does not extend across a `StopControlMode`/`StartControlMode` reconnect race
  (see the Concurrency note below and "Deferred / Out of Scope") — in that known, narrow,
  pre-existing window a frame can still be dropped via the defensive `default:` fallback,
  same as the pre-Phase-3 baseline behavior.
- **Concurrency note**: this branch performs up to two sends (`ch <- oldest`, then `ch <-
  data`) inside the same `controlModeSubMu` `RLock` window the existing single-send path
  already runs under. The existing invariant — a `WLock`-held `UnsubscribeFromControlModeUpdates`
  cannot run concurrently with any `RLock`-held send, so no send-on-closed-channel race —
  is unchanged by adding more sends inside the same already-held `RLock`; this is called
  out explicitly per round-2's concurrency concern rather than left implicit.
  **Round 4 addition, corrected by Round 5 review (Blocker #1)**: within a single,
  uninterrupted control-mode generation, `broadcastControlModeUpdate` is the *only* sender
  to `ch` — `readControlModeOutput`'s tmux control-mode reader is single-threaded per
  session, so there is exactly one producer for that generation. Combined with the
  RLock/WLock mutual exclusion above, capacity freed by the dequeue-and-reinsert sequence
  cannot be consumed by a concurrent sender *within that generation* between the dequeue
  and the re-send.
  However, Round 5 review found this single-producer property is **not** guaranteed across
  a `StopControlMode()`/`StartControlMode()` reconnect cycle: `StopControlMode()`
  (`session/tmux/control_mode.go:133-235`) joins the sender goroutine via an explicit
  `cmSenderExited` channel but has no equivalent join for the reader goroutine
  (`readControlModeOutput`) before releasing `controlModeStartMu`, and `StartControlMode()`
  performs no check that a prior generation's reader has actually exited before spawning a
  new one. Because `bufio.Scanner` can have multiple lines already buffered in userspace,
  a stale-generation reader can keep dispatching to `broadcastControlModeUpdate` for a
  scheduler-dependent number of iterations after a new generation has started — briefly
  producing two concurrent producers. `StartControlMode`/`StopControlMode` are invoked on
  ordinary WebSocket connect/disconnect (`server/services/connectrpc_websocket.go:621,625,1172,1176`),
  so a page refresh or brief network blip is a realistic trigger, not a hypothetical one.
  **This plan does not fix that race** — fixing the reader-goroutine join in
  `StartControlMode`/`StopControlMode` is out of scope for this plan (see "Deferred / Out
  of Scope" below); the `default:` fallbacks on the re-insert sends below are therefore
  a **known-reachable-but-rare defensive guard**, not proven-unreachable dead code. The
  guard degrades safely (drops-and-logs, no panic/no send-on-closed-channel) if hit, so
  this narrows a claim rather than leaving a crash risk.
- Existing tests in `control_mode_dispatch_test.go` /
  `control_mode_refcount_test.go` continue to pass unmodified — the subscriber channel
  type and interface are unchanged.
- New tests confirm: (1) given a full channel whose oldest frame has no erase sequence,
  the new frame is delivered and `controlModeEvictedStale` increments; (2) given a full
  channel whose oldest frame *does* carry an erase sequence, that frame remains
  deliverable (drained from the channel afterward, confirming no data loss — order is
  NOT asserted, per the documented trade-off above) and the new frame is dropped via the
  existing counter/log path; (3) with `STAPLER_SQUAD_CONTROL_MODE_EVICT_STALE=false` set
  via `t.Setenv` before the call, behavior matches pre-Phase-3 drop-new+log exactly
  regardless of frame content — verified to actually respond to `t.Setenv` (this is the
  regression test for round 2's untestable-flag blocker).

**Files**:
- `session/tmux/control_mode.go`
- `session/tmux/control_mode_dispatch_test.go` (or the file from Task 1.2.1c)

##### Task 3.1.1a: Add the revert flag function and erase-safe drain-oldest-then-send logic in `broadcastControlModeUpdate` (~5 min)

- Add `func controlModeEvictStaleEnabled() bool { return os.Getenv("STAPLER_SQUAD_CONTROL_MODE_EVICT_STALE") != "false" }`
  (a function, not a package-level `var` — re-read on every call so `t.Setenv` in tests
  takes effect; only called on the already-rare full-channel path, not per-broadcast).
- Replace the `default:` branch body with:
  ```go
  default:
      if !controlModeEvictStaleEnabled() {
          dropped := t.controlModeDroppedUpdates.Add(1)
          log.Warn("control mode subscriber channel full, dropping update",
              "subscriber", subscriberID, "session", t.sanitizedName,
              "dropped_bytes", len(data), "session_drop_total", dropped,
              "contains_erase_sequence", hasEraseSequence(data))
          break
      }
      select {
      case oldest := <-ch:
          if hasEraseSequence(oldest) {
              // Never silently discard an erase-bearing frame — put it back
              // (may land at the tail, not necessarily the front — see the
              // documented reordering trade-off in Story 3.1.1) and drop the
              // new frame instead.
              select {
              case ch <- oldest:
              default:
                  // Defensive guard, not proven-unreachable dead code (see
                  // Round 5 adversarial review, Blocker #1). Within a single
                  // control-mode generation, sends to `ch` only happen here,
                  // and this block holds controlModeSubMu's RLock, which is
                  // mutually exclusive with the WLock
                  // UnsubscribeFromControlModeUpdates takes — so the receiving
                  // consumer is the only other party that can touch capacity,
                  // and it only ever *frees* room. BUT: StopControlMode()
                  // does not join the reader goroutine before
                  // StartControlMode() spawns a new one (a separate,
                  // pre-existing gap in session/tmux/control_mode.go, out of
                  // scope for this plan — see "Deferred / Out of Scope"), so
                  // a stale-generation reader can in rare cases still be
                  // draining buffered lines into this same channel during a
                  // fast reconnect, acting as a second producer. If this path
                  // is ever hit, it degrades safely (drop-and-log below, no
                  // panic/no send-on-closed-channel) — treat it as a signal
                  // to prioritize the reconnect-race follow-up, not as a
                  // logic bug in this eviction code itself.
              }
              dropped := t.controlModeDroppedUpdates.Add(1)
              log.Warn("control mode subscriber channel full, oldest buffered frame "+
                  "carries an erase sequence; dropping new update instead",
                  "subscriber", subscriberID, "session", t.sanitizedName,
                  "dropped_bytes", len(data), "session_drop_total", dropped,
                  "contains_erase_sequence", hasEraseSequence(data))
              break
          }
          evicted := t.controlModeEvictedStale.Add(1)
          log.Warn("control mode subscriber channel full, evicting stale non-erase frame",
              "subscriber", subscriberID, "session", t.sanitizedName,
              "evicted_bytes", len(oldest), "session_evict_total", evicted)
          select {
          case ch <- data:
          default:
              dropped := t.controlModeDroppedUpdates.Add(1)
              log.Warn("control mode subscriber channel full after eviction, dropping update",
                  "subscriber", subscriberID, "session", t.sanitizedName,
                  "dropped_bytes", len(data), "session_drop_total", dropped,
                  "contains_erase_sequence", hasEraseSequence(data))
          }
      default:
          // Channel was drained concurrently by the consumer; try a direct send.
          select {
          case ch <- data:
          default:
              dropped := t.controlModeDroppedUpdates.Add(1)
              log.Warn("control mode subscriber channel full, dropping update",
                  "subscriber", subscriberID, "session", t.sanitizedName,
                  "dropped_bytes", len(data), "session_drop_total", dropped,
                  "contains_erase_sequence", hasEraseSequence(data))
          }
      }
  ```
- `"os"` is already imported by this file — no new imports needed for this task.
- Files: `session/tmux/control_mode.go`

##### Task 3.1.1b: Add tests for erase-safe eviction, non-erase eviction, and the revert flag (~5 min)

- Test 1 (non-erase eviction): fill a subscriber channel to capacity with non-erase
  sentinel payloads, call `broadcastControlModeUpdate` once more, assert the new data is
  now in the channel, `controlModeEvictedStale` incremented by 1, and
  `controlModeDroppedUpdates` did not change.
- Test 2 (erase-safe refusal, no-data-loss only — order NOT asserted per the documented
  trade-off): table-driven over multiple erase-bearing payload fixtures — at minimum
  `\x1b[K`, `\x1b[0K`, `\x1b[1K`, `\x1b[2K`, `\x1b[J`, `\x1b[2J`, `\x1b[3J` (the
  parameterized forms are the ones the Round 4 review found the naive substring matcher
  missed — this test's whole purpose is to pin the regex-based `hasEraseSequence` against
  regression back to substring matching). For each fixture: fill the channel so the oldest
  buffered item contains that sequence, call `broadcastControlModeUpdate` again, drain the
  whole channel afterward and assert the erase-bearing item is present somewhere in the
  drained contents (it may now be at the tail rather than the front — this test
  intentionally does not assert position). Also assert the new data was NOT delivered,
  `controlModeDroppedUpdates` incremented by 1, and `controlModeEvictedStale` did not
  change.
- Test 3 (revert flag, function-based): first assert this test fails against a
  package-`var`-based implementation (documented in a comment, not asserted in code) to
  confirm the fix is real; with `STAPLER_SQUAD_CONTROL_MODE_EVICT_STALE=false` set via
  `t.Setenv` before calling `broadcastControlModeUpdate`, repeat Test 1's full-queue setup
  and assert the pre-Phase-3 drop-new behavior occurs instead (new data dropped, front
  item unchanged, `controlModeDroppedUpdates` incremented, `controlModeEvictedStale`
  unchanged). Because `controlModeEvictStaleEnabled()` re-reads the env var per call,
  `t.Setenv`'s effect is directly observable — no test-ordering or init-time dependency.
- Test 4 (concurrent-drain interaction, added per Round 4 review Concern #2, **restructured
  per Round 5 review Concern #1 to avoid a vacuous pass**): the real production consumer
  (`server/services/connectrpc_websocket.go`'s control-mode read loop) concurrently
  receives from the same channel `broadcastControlModeUpdate`'s eviction path reads from —
  no existing test exercises both readers racing. The original design (a drain goroutine
  that returns at the first empty receive) risked the drain finishing before any send ran,
  in which case the channel is never actually full at send time, the eviction/`default:`
  branch never executes, and a "counters only monotonically increase" assertion would pass
  trivially at zero increments — validating nothing. Fixed shape:
  - Fill the channel to capacity with non-erase sentinel payloads.
  - Start a drain goroutine that runs for a **fixed, generous number of iterations** (e.g.
    200), each iteration doing a non-blocking `select { case <-ch: default: }` receive
    followed by `runtime.Gosched()` — i.e. it does NOT return early on an empty receive,
    so it keeps contending for the channel across its whole bounded run instead of racing
    the test goroutine to see who touches the channel first.
  - From the test goroutine, call `broadcastControlModeUpdate` enough times (e.g. 50, each
    re-filling with a fresh non-erase sentinel first if the channel isn't already full) to
    make it very likely the channel is at-or-near capacity for at least part of the drain
    goroutine's lifetime.
  - Assert: no panic, no send-on-closed-channel, **and explicitly assert
    `controlModeEvictedStale.Load() + controlModeDroppedUpdates.Load() > 0` at the end**
    (a sanity check that the eviction/drop code path actually executed at least once —
    this is the assertion that closes the vacuous-pass gap; a passing test with zero
    increments across both counters must now fail, not silently pass) — in addition to the
    original monotonic-non-negative-progression check.
  - `go test -race` must pass for this test — add `-race` to this file's test invocation
    notes if not already covered by `make test`.
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
- **tmux crash / WebSocket disconnect mid-instrumentation-or-eviction** — neither Epic 1.1
  (`pendingRedraw`/drop counter) nor Epic 3.1 (eviction) acceptance criteria addresses
  cleanup ordering if the control-mode session exits or the client disconnects mid-flight
  (e.g. whether `RedrawThrottler.cleanup()`'s flush-on-teardown could fire a stale flush
  after the session's own cleanup has run). Worth a quick look during Phase 5
  implementation before shipping, but not a scope item for this plan — none of this plan's
  fixes change session teardown ordering, so the existing (pre-this-plan) behavior in that
  area is unaffected either way. (Re-confirmed accurate after the round-3 design
  correction to Story 3.1.1: the final design keeps the existing `chan []byte` transport
  and `SubscribeToControlModeUpdates` interface completely unchanged — no new type, no
  bridging goroutine, no new close/teardown semantics were introduced. An earlier
  round-2-only draft of this plan would have made this bullet stale, per adversarial
  review round 3's concern #2; that draft was abandoned before this plan reached this
  state.)
- **`StopControlMode`/`StartControlMode` reader-goroutine reconnect race** (found by Round
  5 adversarial review, Blocker #1): `StopControlMode()` (`session/tmux/control_mode.go:133-235`)
  joins the sender goroutine via an explicit `cmSenderExited` channel before returning, but
  has no equivalent join for the reader goroutine (`readControlModeOutput`) — and
  `StartControlMode()` performs no check that a prior generation's reader has fully exited
  before spawning a new one. Because `bufio.Scanner` can have multiple lines already
  buffered in userspace, a stale-generation reader can keep dispatching to
  `broadcastControlModeUpdate` for a scheduler-dependent number of iterations after a new
  generation has started, briefly producing two concurrent producers on the new
  generation's subscriber channels. This is triggered by ordinary WebSocket
  connect/disconnect (page refresh, brief network blip) via
  `server/services/connectrpc_websocket.go:621,625,1172,1176` — not just error-cleanup
  paths. It has two consequences this plan explicitly does not fix: (1) it invalidates the
  strict "single producer" framing Task 3.1.1a's eviction logic would otherwise rely on
  (mitigated here by scoping that claim down rather than fixing the race — see Task
  3.1.1a's Concurrency note); (2) more importantly, a stale-generation reader could deliver
  old-session bytes into a freshly-reconnected client's stream, which is plausibly a
  distinct, additional contributor to the terminal-corruption symptom class this whole
  project targets — a separate mechanism from this plan's RedrawThrottler/channel-drop
  root-cause candidates. Recommended follow-up: give `readControlModeOutput()` an explicit
  exit-signal channel (mirroring `cmSenderExited`) and have `StopControlMode()` join it,
  with a bounded timeout consistent with the file's existing 2s patterns, before releasing
  `controlModeStartMu`. Track as its own bug/plan rather than folding into this one, since
  it touches `StartControlMode`/`StopControlMode` — functions outside this plan's stated
  blast radius (Epic 1.1/2.1/3.1's `broadcastControlModeUpdate`/`RedrawThrottler`/
  `EscapeSequenceParser` scope).
- **Capture-pane snapshot vs. live control-mode stream "dual-writer" race** (found by
  Round 6 adversarial review, Concern #1): `research/pitfalls.md` §4b names `terminal-jank.md`
  Story 2's quiescence-gated `tmux capture-pane` snapshot path (used on cold-start/reconnect)
  as a **plausible secondary contributor** — if a stale snapshot were ever concatenated with,
  rather than atomically replacing, fresh live-stream output, that alone could produce
  stray-tail-character corruption resembling this bug's symptom, independent of the
  RedrawThrottler/EscapeSequenceParser mechanisms Phases 1-3 target.
  Requirements.md's Open Questions section asks the same thing directly. This plan does not
  investigate or fix that path: it is judged secondary for the *specific* steady-state
  symptom this project was opened for — per pitfalls.md §4b's own reasoning, the screenshot
  evidence's redraw cadence (rapid, varying-length spinner lines mid-tool-call) implies an
  already-connected, continuously-streaming session, not a cold-start/reconnect snapshot
  window — so it is unlikely to be the dominant cause of the reported symptom even though it
  remains a real, separately-worth-checking hazard. It is out of this plan's blast radius
  (which is scoped to `broadcastControlModeUpdate`/`RedrawThrottler`/`EscapeSequenceParser`,
  none of which touch the capture-pane snapshot path in
  `server/services/connectrpc_websocket.go`). Recommended follow-up: a reconnect-timing
  correlation check (per pitfalls.md §4b) against real corruption reports, tracked as its own
  investigation rather than folded into this plan.

## No New Dependencies

This plan introduces no new external dependencies (npm packages or Go modules). All
changes are confined to existing files: `TerminalStreamManager.ts`, its test suite,
`control_mode.go`, and its test suite. Per Step 2 of the planning brief, no
dependency-justification ADR is required; the two ADRs in this plan instead document the
architectural/process decisions (instrument-first-fix-both, and the footprint-coverage
coalescing strategy).
