# Implementation Plan: osc-status-signals

**Feature**: Capture Claude Code's OSC window-title (spinner/✳) content as a distinct, higher-priority status signal that upgrades false-idle text-pattern results and bypasses the text-pattern debounce via its own shorter window.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: ADR-001 (OSC capture point — tail-buffer overlay, not a new read-loop hook), ADR-002 (OSC debounce-bypass policy — dedicated shorter window, not literal zero-debounce), ADR-003 (Claude-only free function, not a `BinaryDetector` interface method)

---

## Step 0.5 — Alternatives Considered (before committing to the architecture below)

Three distinct high-level approaches were evaluated for *where OSC title bytes get captured and how
they reach the status pipeline*:

1. **Tail-buffer overlay (chosen).** Extract the OSC title as a read-only scan over the same
   4096-byte raw tail buffer `ClaudeController.GetCurrentStatus`/`GetStatusAndIdleInfo`/`GetIdleState`
   already read for text-pattern detection; classify it via a free function; apply it as an
   asymmetric, upgrade-only overlay on top of the existing text-pattern result.
   - *Strength*: zero new concurrency surface, reuses the existing tailHash cache, and — because the
     buffer is already fully reassembled by the time it's read — is structurally immune to the
     split-OSC-sequence risk `research/pitfalls.md` demonstrated live against real tmux.
   - *Weakness*: OSC-derived latency is bounded by polling cadence, not by "the instant the byte
     arrives" — acceptable per `research/ux.md`'s finding that `DetectedStatus` delivery already has
     no other smoothing layer to hide behind.
2. **Push-based per-chunk capture (rejected — see ADR-001).** Extract the title inside
   `response_stream.go`'s PTY read loop into a new `atomic.Pointer[string]`, populated per `Read()`
   call (`research/architecture.md`'s literal Option A sub-step).
   - *Strength*: captures the title the instant it's written, independent of poll cadence.
   - *Weakness*: `research/pitfalls.md` §2 live-tested that OSC sequences can arrive fragmented across
     dozens of per-chunk deliveries; a per-chunk scanner would need its own reassembly buffer and
     would not participate in the existing tailHash cache — a second, uncoordinated cache input.
3. **Parallel `OSCDetector` goroutine (rejected).** Mirror `session/detection/ratelimit/`: an
   independent `Detector` type polling `GetRecentOutput` on its own timer, calling back into whatever
   consumes `DetectedStatus`, entirely outside `IdleDetector`.
   - *Strength*: the strongest *existing* precedent in this codebase for "a detector that doesn't go
     through the shared debounce."
   - *Weakness*: ratelimit's parallel detector does not feed `DetectedStatus`/`IdleState` today —
     wiring that up would mean either a second goroutine independently writing `IdleDetector` state
     (the exact "second uncoordinated writer" shape `research/pitfalls.md`'s incident history warns
     about) or duplicating the tail-read/cache logic a third time, plus a new goroutine lifecycle to
     start/stop with the session.

Approach 1 was chosen; approaches 2 and 3 are recorded as rejected alternatives in the Pattern
Decisions table below, with the same reasoning.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `OSCTitle` | The string payload of Claude Code's OSC window-title escape sequence (`\x1b]0;...` or `\x1b]2;...`, BEL- or ST-terminated), with the `ESC ] <num> ;` prefix and terminator stripped. | Not a Go type — a documentation term for "the string `ExtractLastOSC` returns." |
| `dtypes.OSCStatus` | New 3-value enum in `session/detection/dtypes/dtypes.go`: `OSCStatusNone`, `OSCStatusExecuting`, `OSCStatusIdle`. The classification of an `OSCTitle`, independent of and prior to mapping into `detection.DetectedStatus`/`detection.IdleState`. | Lives in `dtypes` (not `detection`) for the same import-cycle reason every other shared type does — `binaries` and `session` both need it without importing each other. |
| `binaries.ClassifyOSCTitle` | `func ClassifyOSCTitle(title string) (dtypes.OSCStatus, bool)` in `session/detection/binaries/claude.go`. Claude-specific glyph classifier: Braille spinner block (U+2800–U+28FF) → `OSCStatusExecuting`; `✳` (U+2733) → `OSCStatusIdle`; anything else (including empty) → `(OSCStatusNone, false)`. | Free function, not a `BinaryDetector` method — see ADR-003. |
| `ansi.ExtractLastOSC` | `func ExtractLastOSC(data string, oscNums ...string) (payload string, ok bool)` in new file `pkg/ansi/osc.go`. Scans a bounded string for the last (right-most, by end-of-match byte offset) complete OSC sequence whose numeric command matches one of `oscNums`, terminated by BEL (`\x07`) or ST (`\x1b\\`). | Bounded by the caller — does not itself cap scan length; callers must pass an already-bounded window (the existing 4096-byte tail). |
| `ClaudeController.classifyOSC` | New private method: wraps `ExtractLastOSC` + `ClassifyOSCTitle` against the controller's current raw tail, gated on `cc.IsStarted()` AND on real PTY liveness (`oscStaleThreshold`, see below) — not `IsStarted()` alone. | Returns `(dtypes.OSCStatusNone, false)` whenever the controller isn't started, the PTY has produced no bytes in over `oscStaleThreshold`, no OSC title is present, or the title is unrecognized — the fallback gate for AC7 and the liveness fix for the adversarial-review BLOCKER (stuck-Executing-forever on a crashed process). |
| `oscStaleThreshold` | New `const time.Duration = 5 * time.Second` in `session/claude_controller.go`. | Compared against `IdleDetector.GetLastActivityNs()` — a timestamp already updated on *every real PTY read* via `rs.SetOnOutput`'s existing `detector.RecordActivity()` call (line ~312), independent of status classification. A crashed/killed process stops producing PTY bytes, so this clock freezes with it — reusing it costs zero new state. 5s is ~33x `OSCDebounceDelay` (150ms) and ~10x the estimated spinner redraw interval (80-100ms), so no legitimate in-progress spinner can trip it, while a dead process's stale spinner title stops being trusted well within one operator-visible interval. |
| `applyOSCStatusOverride` | New free function in `session/claude_controller.go`: `func applyOSCStatusOverride(textStatus detection.DetectedStatus, textDesc string, osc dtypes.OSCStatus) (detection.DetectedStatus, string)`. Implements the asymmetric, upgrade-only priority policy (see Pattern Decisions). | Never demotes a higher-urgency text-pattern result; only promotes "boring" outcomes toward Executing/Idle. |
| `IdleDetector.DetectStateFromContentWithOSC` | New method on `session/detection/idle.go`'s `IdleDetector`: `func (id *IdleDetector) DetectStateFromContentWithOSC(content string, osc dtypes.OSCStatus) IdleState`. Computes the text-derived candidate state (same logic `DetectStateFromContent` uses) and, if `osc` is promotable against the text-derived `DetectedStatus` (see `IsOSCExecutingPromotable`/`IsOSCIdlePromotable`), the OSC-derived candidate state, then performs exactly **one** lock-protected write using whichever debounce window (`DebounceDelay` or `OSCDebounceDelay`) applies to the winning source. `DetectStateFromContent` becomes `return id.DetectStateFromContentWithOSC(content, dtypes.OSCStatusNone)` — a thin wrapper, so its no-OSC behavior (AC7) is unchanged by construction. | **Design correction (2026-08-28, architecture-review.md BLOCKER 1/2):** replaces the originally-planned standalone `ApplyOSCStatus(osc) IdleState` method, which the architecture review found races `lastStateChange` against `DetectStateFromContent` when both are called sequentially per poll (as Stories 4.1.3/4.1.4 originally specified), and lacked BLOCKER 2's protected-status guard. See ADR-002 for why the OSC and text-pattern gates share one `lastStateChange` field rather than tracking two. |
| `detection.IsOSCExecutingPromotable` / `detection.IsOSCIdlePromotable` | New package-level functions in `session/detection` (e.g. `detector.go` or a new `osc_priority.go`): `func IsOSCExecutingPromotable(status DetectedStatus) bool` (true for `StatusReady, StatusUnknown, StatusIdle, StatusProcessing`) and `func IsOSCIdlePromotable(status DetectedStatus) bool` (true for `StatusReady, StatusUnknown`). | **Added in the 2026-08-28 design correction.** Single source of truth for the promotable-status sets, called by both `applyOSCStatusOverride` (`DetectedStatus` side) and `DetectStateFromContentWithOSC` (`IdleState` side) — closes architecture-review.md BLOCKER 2 and the related "Concerns" note about the two overlays being able to independently drift. |
| `IdleDetectorConfig.OSCDebounceDelay` | New `time.Duration` field, default `150 * time.Millisecond`. The debounce window applied specifically to OSC-derived transitions, distinct from (and shorter than) `DebounceDelay` (500ms, text-pattern transitions). | See ADR-002. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| OSC capture point | Read-only overlay scan over the existing 4096-byte status-detection tail buffer (pull-based, inside `ClaudeController`) | `research/architecture.md` §4's own tension flag; ADR-001 | Per-chunk capture into an `atomic.Pointer[string]` in `response_stream.go`'s read loop (`architecture.md` Option A step 2) | Live-tested (`pitfalls.md` §2) that OSC sequences can arrive fragmented across per-chunk PTY/`%output` deliveries; a per-chunk scanner would need its own reassembly buffer and would not participate in the existing tailHash cache — reproducing the exact "second uncoordinated cache" risk `architecture.md` §4 itself flags. |
| OSC capture point (2nd alternative) | (same as above) | ADR-001 | Parallel `OSCDetector` goroutine mirroring `session/detection/ratelimit` (`architecture.md` Option B / `features.md` §4) | Adds a new goroutine lifecycle for a value that's naturally pull-based; ratelimit's parallel detector doesn't feed `DetectedStatus`/`IdleState` today, so wiring it in would duplicate the tail-read/cache logic a third time or introduce a second goroutine writing `IdleDetector` state — the "second uncoordinated writer" shape `pitfalls.md`'s incident history warns against. |
| Claude-specific OSC classifier | Free function `ClassifyOSCTitle` in `session/detection/binaries/claude.go` | `architecture.md` Option A; ADR-003 | Optional method added to `dtypes.BinaryDetector` (`features.md`'s more neutral lean) | `.claude/rules/interface-pollution-checklist.md`: Non-Goals exclude other binaries, so 4 of 5 `BinaryDetector` implementations would gain a permanent no-op method solely to satisfy the interface — a forwarding-only/speculative-surface smell. `ClaudeController`, the only production caller, is already Claude-only, so no registry/dispatch change is needed either way. |
| OSC-byte scanning implementation | New `pkg/ansi/osc.go` (`ExtractLastOSC`), string-based, handles both BEL and ST terminators | `build-vs-buy.md`, `stack.md` | (a) Add a capturing group to `ansiStripRegex` in `detector.go`; (b) reuse `pkg/analytics.EscapeCodeParser.parseOSC` | (a) rejected: the live per-line detection path only ever sees one line's raw bytes, not the full tail window — reintroduces the exact split/truncation risk this plan avoids by design (ADR-001). (b) rejected: `EscapeCodeParser` no-ops whenever `captureLevel == "off"` (the common no-analytics-DB case), which would make status-detection correctness silently depend on an unrelated, optional analytics subsystem — `build-vs-buy.md`'s stated reason to centralize in `pkg/ansi` instead and avoid a 4th independent OSC-scanning implementation (the BUG-025 duplication pattern). |
| Debounce bypass for OSC transitions | New `OSCDebounceDelay` (150ms default) + `IdleDetector.ApplyOSCStatus`, sharing the existing `lastStateChange` field with the text-pattern `DebounceDelay` gate | `pitfalls.md` §5; ADR-002 | Literal zero-debounce (commit every OSC-derived transition immediately, unconditionally) | `pitfalls.md` §5: this codebase has a documented, repeated incident history (`backlog-session-thrashing`, BUG-043, BUG-048) from exactly the "second, uncoordinated transition path writing to shared state" shape; a genuine rapid spinner↔✳ toggle between two back-to-back tool calls would flap the badge under literal zero-debounce. Sharing one timestamp field (rather than a second `lastOSCStateChange`) prevents the two paths from ever disagreeing about "when did state last change." |
| `DetectedStatus`/`IdleState` override policy | Asymmetric "upgrade-only" overlay: OSC may promote a low-urgency text result (Ready/Unknown/Idle/Processing) toward Executing, and may promote Ready/Unknown toward Idle — never the reverse, and never over Error/NeedsApproval/InputRequired/TestsFailing/Success/WaitingForAgent/Executing | This plan, per `pitfalls.md` §4's asymmetric-trust guidance | Pure priority short-circuit before the text-pattern chain runs (`features.md`'s literal "short-circuit BEFORE MatchLines" recommendation) | A short-circuit that always trusts OSC over text risks the costlier failure direction `pitfalls.md` names explicitly: a false OSC-idle title (nested shell, nested tool spinner) incorrectly suppressing a true Error/NeedsApproval/InputRequired text-pattern result. The upgrade-only overlay bounds a false-positive OSC signal's blast radius to "cosmetically wrong badge," never "hides a state needing user attention." |
| `✳` idle mapping target | `detection.StatusIdle` | This plan | `detection.StatusReady` (also listed as acceptable in AC4) | `PatternSet.MatchLines`'s Ready category deliberately returns `StatusUnknown` ("so the `.*` pattern renders no badge" — `pattern_set.go:134`) — i.e. `StatusReady` the enum value is effectively dead in the live pipeline. `StatusIdle` has a real, distinct UI treatment (`getDetectedStatusInfo`'s `IDLE` case) matching the intent of a definitive `✳` signal. |

---

## Migration Plan

Omitted — no schema or persisted-data changes. All new state (`OSCDebounceDelay` config field,
`IdleDetector` reusing existing `lastStateChange`) is in-memory only, reset on process restart same
as the rest of `IdleDetector`'s state today.

---

## Observability Plan

- **Logs**: `session/claude_controller.go`'s `GetCurrentStatus`/`GetStatusAndIdleInfo` already log a
  `log.Debug("GetCurrentStatus: non-active result", ...)`/`"GetStatusAndIdleInfo: non-active result"`
  line whenever the result is Ready/Idle/Unknown. Task 4.1.2a/4.1.3 add one additional `log.Debug`
  call, gated on "OSC override actually changed the status vs. the text-pattern-only result," logging
  `session`, the pre-override `textStatus`, and the post-override `status` — this is the debugging aid
  `research/ux.md` and `research/features.md` both independently flagged as valuable for diagnosing
  future false-idle reports, and costs one conditional log call on an already-logged code path.
- **Metrics**: none added. `ExtractLastOSC`/`ClassifyOSCTitle` are sub-microsecond string scans over
  an already-bounded 4096-byte window — far under the "new operation >100ms" threshold that would
  warrant a new metric.
- **Alerts**: none required.

## Risk Control

- **Feature flag**: not gated. The change is additive with a built-in, tested fallback (AC7): any
  session/tail where `ExtractLastOSC` finds no match behaves identically to pre-feature code. The
  asymmetric upgrade-only override policy (Pattern Decisions) already bounds worst-case blast radius
  to "a cosmetically wrong badge briefly," not a functional regression, so a flag would add rollout
  complexity without a correspondingly larger rollback need.
- **Rollback procedure**: standard revert via PR close + revert commit — the change is a single,
  self-contained PR touching 6 files (`pkg/ansi/osc.go` new, `session/detection/dtypes/dtypes.go`,
  `session/detection/binaries/claude.go`, `session/detection/idle.go`, `session/claude_controller.go`,
  plus their test files), with zero call sites outside this feature depending on any of the new
  symbols.
- **Staged rollout**: full rollout on merge — this is deterministic backend logic driven purely by
  PTY bytes already flowing through the server process; there is no per-user/per-cohort variable to
  stage against.

## Unresolved Questions

- [ ] Whether Claude Code emits any OSC *progress* sequence, and under what exact command number, is
  unverified — the requirements' `\x1b]4;0` claim conflicts with OSC 4's standard meaning
  (color-palette-set) per `research/stack.md`, and ConEmu/Windows Terminal's progress convention is
  actually OSC `9;4`, not `4`. This is explicitly out of scope per the requirements' Non-Goals; no
  story in this plan implements it. **Blocks**: any future "OSC progress" story, not this plan —
  **owner**: whoever picks that story up should capture real Claude Code PTY output first, not
  implement against the requirements' literal (possibly wrong) sequence number.
- [ ] Whether this project's bundled tmux config's `set-titles`/`automatic-rename` options could cause
  tmux to itself re-emit a second, wrapped OSC-0 sequence alongside the child's own (`pitfalls.md` §6,
  explicitly flagged as "not yet confirmed by reading code"). Low risk as designed: `ExtractLastOSC`'s
  "last (right-most) match wins" semantics already tolerate a duplicate/wrapped second OSC-0 sequence
  gracefully. **Blocks**: nothing in this plan — **owner**: verify empirically post-merge only if a
  real OSC-status disagreement is reported.

---

## Dependency Visualization

```
Phase 1: pkg/ansi.ExtractLastOSC     Phase 2: dtypes.OSCStatus +
   (independent)                     binaries.ClassifyOSCTitle
        │                                   │        │
        │                                   │        ▼
        │                                   │   Phase 3: idle.go
        │                                   │   OSCDebounceDelay +
        │                                   │   IdleDetector.ApplyOSCStatus
        │                                   │        │
        └────────────────┬──────────────────┘        │
                          ▼                           │
               Phase 4: claude_controller.go ◄────────┘
               classifyOSC, applyOSCStatusOverride,
               wiring into GetCurrentStatus /
               GetStatusAndIdleInfo / GetIdleState
                          │
                          ▼
               Phase 5: full regression + AC cross-check
```

Phases 1 and 2 have no dependency on each other and can be implemented in either order (or in
parallel by two workers). Phase 3 depends only on Phase 2 (needs `dtypes.OSCStatus`). Phase 4 depends
on all three. Phase 5 depends on Phase 4.

---

## Phase 1: OSC Extraction Primitive

### Epic 1.1: `pkg/ansi.ExtractLastOSC`

**Goal**: A reusable, hardened, string-based OSC-payload extractor that handles both terminator forms
and "last (most recent) match wins" semantics, without depending on the analytics subsystem.

#### Story 1.1.1: Implement `ExtractLastOSC`

**As a** Claude-status detector, **I want** a function that extracts the payload of the last complete
OSC sequence matching a given command number from a bounded byte window, **so that** I can classify
Claude Code's window-title spinner/idle glyph without re-implementing OSC scanning a fourth time.

**Acceptance Criteria**:
- AC1 (requirements) — OSC title content is captured via a read-only pass that does not alter
  existing stripped-text behavior.
  - *Given* the raw tail string `tail = "some text\x1b]0;⠋ working\x07more text\n$ "`, *When*
    `ansi.ExtractLastOSC(tail, "0", "2")` is called *and*, separately, the existing
    `session/detection.stripANSI(tail)` (unmodified) is called, *Then* `ExtractLastOSC` returns
    `("⠋ working", true)` and `stripANSI(tail)` still returns exactly
    `"some textmore text\n$ "` — byte-for-byte identical to its pre-feature output.
- Handles both BEL (`\x07`) and ST (`\x1b\\`) terminators.
  - *Given* `data = "\x1b]0;✳\x1b\\idle"` (ST-terminated), *When* `ExtractLastOSC(data, "0")` is
    called, *Then* it returns `("✳", true)`.
- "Last wins" across multiple matches, regardless of `oscNums` argument order.
  - *Given* `data = "\x1b]2;⠋ old\x07 ... \x1b]0;✳\x07"` (an OSC-2 match earlier in the buffer, an
    OSC-0 match later), *When* `ExtractLastOSC(data, "2", "0")` is called, *Then* it returns
    `("✳", true)` — the right-most complete match by end offset, not the last-processed `oscNums`
    entry.

**Files**: `pkg/ansi/osc.go` (new)

##### Task 1.1.1a: Implement `ExtractLastOSC` and `findOSCTerminator` (~5 min)
- Create `pkg/ansi/osc.go` with:
  - `const oscBEL = 0x07` and `const oscST = "\x1b\\"` (documented alongside `csi.go`'s existing
    doc-comment style — cross-reference the BUG-025 precedent for why this centralizes here).
  - `func findOSCTerminator(data string, start int) (idx int, termLen int)` — scans `data[start:]`
    byte-by-byte for `0x07` (return `idx, 1`) or `\x1b` immediately followed by `\`
    (return `idx, 2`); returns `(-1, 0)` if neither is found before `len(data)`.
  - `func ExtractLastOSC(data string, oscNums ...string) (payload string, ok bool)`:
    - Fast path: if `!strings.Contains(data, "\x1b]")`, return `("", false)` immediately (mirrors
      `StripCSI`'s ESC-byte fast path in `csi.go`).
    - For each `num` in `oscNums`, build `prefix := "\x1b]" + num + ";"` and repeatedly
      `strings.Index` forward through `data` for `prefix`. For each occurrence, call
      `findOSCTerminator` starting right after the prefix; if no terminator is found, `break` out
      of that `num`'s inner loop (no later occurrence of the same `num` can be complete either,
      since the terminator search only moves forward). If a terminator IS found, only overwrite the
      returned `payload`/`ok` when this match's terminator index is greater than the greatest
      end-offset seen so far across *all* `oscNums` processed — this is what makes "last wins"
      correct regardless of `oscNums` argument order (see Story 1.1.1's third acceptance criterion;
      do not just take whichever `num` is processed last).
    - Document explicitly: an occurrence whose opening prefix is not present in `data` (truncated at
      a window's leading edge) is never matched — `ExtractLastOSC` only searches forward from a
      found prefix, so a stray terminator with no preceding prefix in this window is correctly
      "no title," never misattributed to unrelated content.
    - Document explicitly: `data` should be an already-bounded window (e.g. the existing 4096-byte
      status-detection tail) — this function does not itself cap scan length.
- Files: `pkg/ansi/osc.go`

#### Story 1.1.2: Test coverage for `ExtractLastOSC`

**As a** reviewer, **I want** `ExtractLastOSC`'s terminator handling, last-wins semantics, and
edge-case behavior directly tested, **so that** a future change to this function can't silently
regress the false-idle fix this whole feature exists to deliver.

**Acceptance Criteria**:
- AC9 (requirements, partial) — new behavior has test coverage.
  - *Given* the test cases enumerated in Task 1.1.2a, *When* `go test ./pkg/ansi/...` is run, *Then*
    all pass.

**Files**: `pkg/ansi/osc_test.go` (new)

##### Task 1.1.2a: Table-driven tests for `ExtractLastOSC` (~4 min)
- Create `pkg/ansi/osc_test.go` with a table-driven `TestExtractLastOSC` covering:
  - Single BEL-terminated match: `"\x1b]0;⠋ working\x07"` → `("⠋ working", true)`.
  - Single ST-terminated match: `"\x1b]0;✳\x1b\\"` → `("✳", true)`.
  - Multiple matches, same `num`, last wins: `"\x1b]0;a\x07\x1b]0;b\x07"` → `("b", true)`.
  - Multiple `oscNums`, right-most overall wins regardless of call-argument order — the exact case
    from Story 1.1.1's third AC (`ExtractLastOSC(data, "2", "0")` where the OSC-0 match is
    physically later in `data` than the OSC-2 match) → returns the OSC-0 payload.
  - No ESC byte at all: `"plain text"` → `("", false)`.
  - Unterminated (opening present, no BEL/ST anywhere after it): `"\x1b]0;stuck"` → `("", false)`.
  - Stray terminator with no preceding opening prefix in the string: `"garbage\x07more"` →
    `("", false)`.
  - Empty string: `""` → `("", false)`.
- Files: `pkg/ansi/osc_test.go`

##### Task 1.1.2b: Zero-alloc fast-path test (~2 min)
- Add `TestExtractLastOSC_ZeroAllocsOnPlainText`, mirroring `csi_test.go`'s
  `TestStripCSI_ZeroAllocsOnPlainText` (`testing.AllocsPerRun(100, func() { _, _ =
  ExtractLastOSC(input, "0") })` on ESC-byte-free input), asserting 0 allocations.
- Files: `pkg/ansi/osc_test.go`

##### Task 1.1.2c: Run and fix (~2 min)
- Run `go test ./pkg/ansi/...`; fix any failures in `osc.go`/`osc_test.go`.
- Files: `pkg/ansi/osc.go`, `pkg/ansi/osc_test.go`

---

## Phase 2: Claude-Specific OSC Classification

### Epic 2.1: `dtypes.OSCStatus` + `binaries.ClassifyOSCTitle`

**Goal**: Turn an extracted OSC title string into a definitive, Claude-specific classification, using
the exact glyphs AC3/AC4 name.

#### Story 2.1.1: Add `dtypes.OSCStatus`

**As a** shared-types consumer (`binaries`, `detection`, `session`), **I want** a minimal enum for OSC
classification results, **so that** `binaries` and `session` can exchange this value without either
importing the other's package.

**Acceptance Criteria**:
- Type exists and compiles from both `binaries` and `session`.
  - *Given* the new type in `dtypes.go`, *When* `session/detection/binaries` and `session` both
    `import "github.com/tstapler/stapler-squad/session/detection/dtypes"` and reference
    `dtypes.OSCStatusExecuting`, *Then* both packages compile with no import cycle (verified by
    Task 2.1.2c/4.1.5's `go build`).

**Files**: `session/detection/dtypes/dtypes.go`

##### Task 2.1.1a: Add `OSCStatus` type and constants (~2 min)
- Add to `session/detection/dtypes/dtypes.go`, after the existing `BinaryDetector` interface:
  ```go
  // OSCStatus represents the classification of a Claude Code OSC window-title
  // payload (see session/detection/binaries.ClassifyOSCTitle), independent of
  // and prior to mapping into DetectedStatus/IdleState by the caller.
  type OSCStatus int

  const (
      OSCStatusNone      OSCStatus = iota // no recognizable spinner/idle marker
      OSCStatusExecuting                  // Braille spinner (U+2800-U+28FF) present
      OSCStatusIdle                       // ✳ (U+2733) present
  )
  ```
- Files: `session/detection/dtypes/dtypes.go`

#### Story 2.1.2: `binaries.ClassifyOSCTitle`

**As a** false-idle bug fix, **I want** a Claude-specific classifier for OSC title strings, **so that**
a spinner or `✳` in the title maps to a definitive `OSCStatus` per AC3/AC4.

**Acceptance Criteria**:
- AC3 — Braille spinner → `OSCStatusExecuting`.
  - *Given* `title = "⠙ Thinking"` (contains U+2819, a Braille Pattern character), *When*
    `binaries.ClassifyOSCTitle(title)` is called, *Then* it returns `(dtypes.OSCStatusExecuting,
    true)`.
- AC4 — `✳` → idle status.
  - *Given* `title = "✳"` (U+2733 alone), *When* `binaries.ClassifyOSCTitle(title)` is called,
    *Then* it returns `(dtypes.OSCStatusIdle, true)`.
- Graceful fallback for unrecognized/empty titles (feeds AC7 at the caller level).
  - *Given* `title = ""` and, separately, `title = "some other terminal app title"`, *When*
    `binaries.ClassifyOSCTitle` is called on each, *Then* both return `(dtypes.OSCStatusNone,
    false)`.
- Transcription-slip guard — only the exact `✳` (U+2733) glyph counts, not visually similar
  asterisk-family glyphs already used elsewhere in this file's screen-text patterns.
  - *Given* `title = "✻"` (U+273B, used in `verb_duration_completion`) and `title = "✽"` (U+273D,
    used in `claude_thinking_verb`), *When* `binaries.ClassifyOSCTitle` is called on each, *Then*
    both return `(dtypes.OSCStatusNone, false)` — neither is mistaken for `✳`.

**Files**: `session/detection/binaries/claude.go`, `session/detection/binaries/claude_test.go`

##### Task 2.1.2a: Implement `ClassifyOSCTitle` (~5 min)
- In `session/detection/binaries/claude.go`, add `"regexp"` and `"strings"` to the import block.
- Add:
  ```go
  // oscBrailleSpinnerRegex matches any Braille Pattern character (U+2800-U+28FF).
  // Claude Code's OSC window title uses these as spinner frames while working.
  // Deliberately the full Unicode block (broader than the hand-listed frame sets
  // already used for screen-text matching in progress_indicators above) — OSC
  // titles are short, single-purpose strings, so the wider range's false-positive
  // risk is low (see osc-status-signals research/stack.md).
  var oscBrailleSpinnerRegex = regexp.MustCompile(`[\x{2800}-\x{28FF}]`)

  // oscIdleGlyph is the exact glyph (U+2733 EIGHT SPOKED ASTERISK) Claude Code's
  // OSC window title uses to signal idle/done — NOT the visually similar ✻
  // (U+273B) or ✽ (U+273D) used elsewhere in this file's screen-text patterns.
  const oscIdleGlyph = '✳'

  // ClassifyOSCTitle inspects a Claude Code OSC window-title payload (already
  // extracted via pkg/ansi.ExtractLastOSC) and returns a definitive OSC-derived
  // status if the title contains an unambiguous spinner or idle marker. Returns
  // ok=false for an empty or unrecognized title — callers must fall back to
  // text-pattern detection in that case.
  //
  // Spinner is checked before the idle glyph: if a title somehow contained both,
  // treating it as "still executing" is the lower-cost mistake (a false busy
  // indicator self-corrects on the next idle title; a false idle indicator could
  // mask genuinely active work) — see osc-status-signals research/pitfalls.md §4.
  func ClassifyOSCTitle(title string) (dtypes.OSCStatus, bool) {
      if title == "" {
          return dtypes.OSCStatusNone, false
      }
      if oscBrailleSpinnerRegex.MatchString(title) {
          return dtypes.OSCStatusExecuting, true
      }
      if strings.ContainsRune(title, oscIdleGlyph) {
          return dtypes.OSCStatusIdle, true
      }
      return dtypes.OSCStatusNone, false
  }
  ```
- Files: `session/detection/binaries/claude.go`

##### Task 2.1.2b: Tests for `ClassifyOSCTitle` (~4 min)
- Add to `session/detection/binaries/claude_test.go`, table-driven, covering:
  - A representative subset of the hand-listed spinner frames (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`) → `OSCStatusExecuting`.
  - A Braille character *outside* the hand-listed frame set (e.g. `⡇` U+2847) → `OSCStatusExecuting`
    — proves full-block coverage per AC3, not just the frames already used in screen-text patterns.
  - `"✳"` exactly → `OSCStatusIdle`.
  - `"✻"` (U+273B) and `"✽"` (U+273D) individually, with no Braille character present → `(OSCStatusNone,
    false)` — the transcription-slip guard.
  - `""` → `(OSCStatusNone, false)`.
  - Plain unrelated text (e.g. `"my-shell — bash"`) → `(OSCStatusNone, false)`.
  - A title containing both a Braille character and `✳` → `OSCStatusExecuting` — documents the
    priority decision in the doc comment above.
- Files: `session/detection/binaries/claude_test.go`

##### Task 2.1.2c: Run and fix (~2 min)
- Run `go test ./session/detection/...`; fix any failures.
- Files: `session/detection/binaries/claude.go`, `session/detection/binaries/claude_test.go`

---

## Phase 3: Debounce-Bypass Path

### Epic 3.1: `OSCDebounceDelay` + `IdleDetector.ApplyOSCStatus`

**Goal**: Give OSC-derived idle-state transitions their own, much shorter debounce window per AC6 and
ADR-002, without introducing a second, uncoordinated state-change clock.

#### Story 3.1.1: `OSCDebounceDelay` config field

**As an** `IdleDetector` owner, **I want** a distinct, shorter debounce duration for OSC-sourced
transitions, **so that** `ApplyOSCStatus` (Story 3.1.2) has a documented, tunable threshold to gate
on instead of reusing `DebounceDelay`.

**Acceptance Criteria**:
- Config field exists with the documented default.
  - *Given* `detection.DefaultIdleDetectorConfig()`, *When* it is called, *Then* the returned
    `IdleDetectorConfig.OSCDebounceDelay == 150 * time.Millisecond` (alongside the existing
    `DebounceDelay == 500 * time.Millisecond`, unchanged).

**Files**: `session/detection/idle.go`

##### Task 3.1.1a: Add `OSCDebounceDelay` field and default (~2 min)
- In `session/detection/idle.go`'s `IdleDetectorConfig` struct, add:
  ```go
  // OSCDebounceDelay gates OSC-derived state transitions (see ApplyOSCStatus)
  // with its own, much shorter window than DebounceDelay (which gates
  // text-pattern-derived transitions). Both compare elapsed time against the
  // same lastStateChange timestamp — see osc-status-signals ADR-002 for why a
  // second, separately-tracked clock was deliberately avoided.
  OSCDebounceDelay time.Duration
  ```
- In `DefaultIdleDetectorConfig()`, add `OSCDebounceDelay: 150 * time.Millisecond,` alongside the
  existing fields, with an inline comment cross-referencing ADR-002 for the 150ms figure's
  reasoning (order-of-magnitude of a spinner redraw interval, ~3x faster than `DebounceDelay`).
- Files: `session/detection/idle.go`

#### Story 3.1.2: `IdleDetector.DetectStateFromContentWithOSC`

**As a** `ClaudeController`, **I want** to feed an OSC-derived classification into the idle state
machine alongside the text-derived one, resolved as a single write, **so that** AC5/AC6's
false-idle fix reaches `IdleState` (not just the displayed `DetectedStatus`) without the two
signals racing `lastStateChange` (architecture-review.md BLOCKER 1) or bypassing the
protected-status guard (BLOCKER 2).

**Acceptance Criteria**:
- AC6 — OSC transitions bypass the text-pattern debounce.
  - *Given* an `IdleDetector` with `currentState = IdleStateWaiting` and `lastStateChange` set 200ms
    in the past (using the package's existing fake-clock test helper), *When*
    `id.DetectStateFromContentWithOSC(content, dtypes.OSCStatusExecuting)` is called with `content`
    whose text-pattern result is itself promotable (e.g. a bare prompt), *Then* `id.currentState`
    becomes `IdleStateActive` immediately — even though the same 200ms elapsed would NOT satisfy
    the text-pattern path's 500ms `DebounceDelay` (only the 150ms `OSCDebounceDelay` needs to have
    elapsed, and 200ms ≥ 150ms).
- First transition out of `IdleStateUnknown` is never debounced (parity with the existing gate).
  - *Given* a freshly-constructed `IdleDetector` (`currentState == IdleStateUnknown`), *When*
    `id.DetectStateFromContentWithOSC(content, dtypes.OSCStatusExecuting)` is called immediately,
    *Then* `id.currentState` becomes `IdleStateActive` with no wait.
- `OSCStatusExecuting` bumps `lastActivity` when it wins (parity with `mapStatusToIdleState`'s
  existing `StatusExecuting` branch).
  - *Given* an `IdleDetector` with a known `lastActivity` and promotable text content, *When*
    `id.DetectStateFromContentWithOSC(content, dtypes.OSCStatusExecuting)` is called, *Then*
    `id.GetLastActivityNs()` reflects the call time.
- `OSCStatusNone` reduces to plain text-pattern behavior (no-op guard, and the byte-for-byte
  equivalence AC7 requires).
  - *Given* any `IdleDetector` state and content, *When*
    `id.DetectStateFromContentWithOSC(content, dtypes.OSCStatusNone)` is called, *Then* the result
    is identical to `id.DetectStateFromContent(content)` on a detector in the same state (proven
    directly since `DetectStateFromContent` delegates to this method with `OSCStatusNone`).
- Non-promotable text status blocks the OSC override (BLOCKER 2 regression test).
  - *Given* content whose text pattern resolves to `StatusNeedsApproval` (→ `IdleStateWaiting` via
    `mapStatusToIdleState`), *When*
    `id.DetectStateFromContentWithOSC(content, dtypes.OSCStatusExecuting)` is called, *Then*
    `id.currentState` stays `IdleStateWaiting` — `StatusNeedsApproval` is not in
    `IsOSCExecutingPromotable`'s set, so the OSC spinner does not force `IdleStateActive` over a
    state that needs user attention.

**Files**: `session/detection/idle.go`, `session/detection/idle_test.go`, `session/detection/detector.go` (or a new `session/detection/osc_priority.go`)

##### Task 3.1.2a: Add `IsOSCExecutingPromotable`/`IsOSCIdlePromotable` (~3 min)
- Add to `session/detection` (e.g. a new small file `session/detection/osc_priority.go`):
  ```go
  package detection

  // IsOSCExecutingPromotable reports whether a text-pattern-derived DetectedStatus
  // is eligible to be promoted to Executing by an OSC-derived spinner signal.
  // Single source of truth shared by applyOSCStatusOverride (session package,
  // DetectedStatus side) and IdleDetector.DetectStateFromContentWithOSC (IdleState
  // side) — see osc-status-signals architecture-review.md BLOCKER 2.
  func IsOSCExecutingPromotable(status DetectedStatus) bool {
      switch status {
      case StatusReady, StatusUnknown, StatusIdle, StatusProcessing:
          return true
      }
      return false
  }

  // IsOSCIdlePromotable reports whether a text-pattern-derived DetectedStatus is
  // eligible to be promoted to Idle by an OSC-derived ✳ signal.
  func IsOSCIdlePromotable(status DetectedStatus) bool {
      switch status {
      case StatusReady, StatusUnknown:
          return true
      }
      return false
  }
  ```
- Files: `session/detection/osc_priority.go`

##### Task 3.1.2b: Implement `DetectStateFromContentWithOSC` (~6 min)
- In `session/detection/idle.go`, add `"github.com/tstapler/stapler-squad/session/detection/dtypes"`
  to the import block.
- Replace `DetectStateFromContent`'s body with a thin delegation, and add the new method:
  ```go
  // DetectStateFromContent analyzes provided terminal content and returns the current idle state.
  // Equivalent to DetectStateFromContentWithOSC(content, dtypes.OSCStatusNone).
  func (id *IdleDetector) DetectStateFromContent(content string) IdleState {
      return id.DetectStateFromContentWithOSC(content, dtypes.OSCStatusNone)
  }

  // DetectStateFromContentWithOSC analyzes content the same way DetectStateFromContent
  // does, then folds in an OSC-derived signal (see ClaudeController.classifyOSC) as an
  // asymmetric, upgrade-only overlay — gated by IsOSCExecutingPromotable/IsOSCIdlePromotable,
  // the same predicates applyOSCStatusOverride uses for DetectedStatus, so the two overlays
  // can never disagree about which text-pattern statuses are eligible for promotion. Computes
  // both candidate states and performs exactly one lock-protected write with the correct
  // debounce window (DebounceDelay or OSCDebounceDelay) for whichever source is authoritative
  // on this call — see osc-status-signals architecture-review.md BLOCKER 1 for why two
  // sequential state-committing calls sharing lastStateChange is unsafe.
  func (id *IdleDetector) DetectStateFromContentWithOSC(content string, osc dtypes.OSCStatus) IdleState {
      if content == "" {
          id.mu.RLock()
          s := id.currentState
          id.mu.RUnlock()
          return s
      }

      lines := strings.Split(content, "\n")
      textStatus := id.statusDetector.DetectFromLines(lines)

      id.mu.Lock()
      defer id.mu.Unlock()

      newState := id.mapStatusToIdleState(textStatus)
      debounce := id.config.DebounceDelay

      switch {
      case osc == dtypes.OSCStatusExecuting && IsOSCExecutingPromotable(textStatus):
          id.lastActivity = id.timeNow()
          id.lastActivityNs.Store(id.lastActivity.UnixNano())
          newState = IdleStateActive
          debounce = id.config.OSCDebounceDelay
      case osc == dtypes.OSCStatusIdle && IsOSCIdlePromotable(textStatus):
          idleDuration := id.timeNow().Sub(id.lastActivity)
          if idleDuration > id.config.IdleThreshold {
              newState = IdleStateTimeout
          } else {
              newState = IdleStateWaiting
          }
          debounce = id.config.OSCDebounceDelay
      }

      if newState != id.currentState {
          if id.currentState == IdleStateUnknown || id.timeNow().Sub(id.lastStateChange) >= debounce {
              id.currentState = newState
              id.lastStateChange = id.timeNow()
          }
      }
      return id.currentState
  }
  ```
- Files: `session/detection/idle.go`

##### Task 3.1.2c: Tests for `DetectStateFromContentWithOSC` (~6 min)
- In `session/detection/idle_test.go`, using the existing `newDetectorWithFakeClock` helper, add:
  - `TestIdleDetector_DetectStateFromContentWithOSC_FirstTransitionNeverDebounced` — from
    `IdleStateUnknown`, an OSC-executing call with promotable content commits immediately.
  - `TestIdleDetector_DetectStateFromContentWithOSC_BypassesTextDebounceViaShorterWindow` — the
    exact AC6 scenario: set `currentState = IdleStateWaiting`, advance the fake clock 200ms past
    `lastStateChange`, call with `dtypes.OSCStatusExecuting` and promotable content, assert
    `IdleStateActive`. In an adjacent test, advance a *fresh* detector's clock by the same 200ms
    and call `DetectStateFromContent` (no OSC) with content that would newly compute
    `StatusExecuting`; assert it is still blocked by the unmet 500ms `DebounceDelay` — the contrast
    that proves the two windows are genuinely independent.
  - `TestIdleDetector_DetectStateFromContentWithOSC_BumpsLastActivity` — asserts
    `GetLastActivityNs()` after an `OSCStatusExecuting` call that wins.
  - `TestIdleDetector_DetectStateFromContentWithOSC_NoneMatchesPlainDetectStateFromContent` —
    `dtypes.OSCStatusNone` produces the identical result to `DetectStateFromContent` on an
    equivalently-seeded detector.
  - `TestIdleDetector_DetectStateFromContentWithOSC_NonPromotableTextBlocksOverride` — the BLOCKER 2
    regression test: `StatusNeedsApproval`-shaped content + `OSCStatusExecuting` stays
    `IdleStateWaiting`, not `IdleStateActive`.
  - `TestIdleDetector_DetectStateFromContentWithOSC_RepeatedSameStatusDoesNotChurnClock` — two
    consecutive same-classification calls within the `OSCDebounceDelay` window: `lastStateChange`
    only updates on the first, since `newState == id.currentState` on the second skips the gate.
  - `TestIdleDetector_DetectStateFromContentWithOSC_IdleNeverDowngradesActive` — text content that
    resolves to `IdleStateActive` (e.g. `StatusExecuting`) plus `dtypes.OSCStatusIdle`: state stays
    `IdleStateActive` (the idle-direction symmetric counterpart to AC5's asymmetric-safety test;
    `StatusExecuting` is not in `IsOSCIdlePromotable`'s set, so this also exercises the "idle
    direction" gap the adversarial review flagged as untested).
- Files: `session/detection/idle_test.go`

##### Task 3.1.2d: Run and fix (~2 min)
- Run `go test ./session/detection/...`; fix any failures.
- Files: `session/detection/idle.go`, `session/detection/idle_test.go`, `session/detection/osc_priority.go`

---

## Phase 4: Controller Wiring

### Epic 4.1: `classifyOSC` / `applyOSCStatusOverride` and integration into the three status entry points

**Goal**: Make `ClaudeController`'s three status/idle read paths actually consult the OSC signal, per
AC2/AC5/AC7.

#### Story 4.1.1: Helper functions

**As a** maintainer of `session/claude_controller.go`, **I want** the OSC-classification and
override-policy logic factored into two small, independently testable helpers, **so that** the three
call sites (Stories 4.1.2–4.1.4) don't each hand-roll the same logic.

**Acceptance Criteria**:
- AC2 — the OSC title reaches `binaries.ClassifyOSCTitle` as a distinct input.
  - *Given* a started `ClaudeController` whose PTY tail contains `"$ \x1b]0;⠋ working\x07"`, *When*
    `cc.classifyOSC(tail)` is called, *Then* it internally calls
    `binaries.ClassifyOSCTitle("⠋ working")` (the extracted title string, not the stripped screen
    text) and returns `(dtypes.OSCStatusExecuting, true)`.

**Files**: `session/claude_controller.go`

##### Task 4.1.1a: Add imports and `classifyOSC` (~4 min)
- Add to `session/claude_controller.go`'s import block:
  `"github.com/tstapler/stapler-squad/pkg/ansi"`,
  `"github.com/tstapler/stapler-squad/session/detection/binaries"`,
  `"github.com/tstapler/stapler-squad/session/detection/dtypes"`.
- Add, near `GetCurrentStatus`:
  ```go
  // oscStaleThreshold bounds how long a stale OSC title is trusted after the
  // PTY stops producing real output (crash/kill without a clean Stop() call —
  // see osc-status-signals adversarial-review.md's BLOCKER: cc.IsStarted() alone
  // stays true for an indefinite period after an unexpected exit, since Stop()
  // is only called from deliberate operator/driver paths, never from the
  // onEOFCallback/control-mode exit paths that fire on a crash). Compared
  // against IdleDetector.GetLastActivityNs(), which is already updated on every
  // real PTY read via rs.SetOnOutput's RecordActivity() call — independent of
  // this feature, so this reuses an existing liveness clock rather than adding
  // a new one. Set well above OSCDebounceDelay/spinner redraw cadence so no
  // legitimate in-progress spinner can trip it.
  const oscStaleThreshold = 5 * time.Second

  // classifyOSC extracts and classifies the OSC window-title payload from tail,
  // gated on the session still being started AND on recent real PTY activity —
  // an exited process's last-written title could otherwise be misread as
  // current state forever (see oscStaleThreshold doc above). Returns ok=false
  // whenever the controller isn't started, the PTY has been silent longer than
  // oscStaleThreshold, no OSC title is present in tail, or the title matches
  // neither recognized marker — callers must fall back to text-pattern
  // detection in that case (AC7).
  func (cc *ClaudeController) classifyOSC(tail string) (dtypes.OSCStatus, bool) {
      if !cc.IsStarted() {
          return dtypes.OSCStatusNone, false
      }
      if detector := cc.idleDetector.Load(); detector != nil {
          if ns := detector.GetLastActivityNs(); ns > 0 && time.Since(time.Unix(0, ns)) > oscStaleThreshold {
              return dtypes.OSCStatusNone, false
          }
      }
      title, ok := ansi.ExtractLastOSC(tail, "0", "2")
      if !ok {
          return dtypes.OSCStatusNone, false
      }
      return binaries.ClassifyOSCTitle(title)
  }
  ```
- Files: `session/claude_controller.go`

##### Task 4.1.1a-test: Test the staleness guard (~4 min)
- Add `TestClassifyOSC_StaleActivity_FallsBackToNone`: use `newControllerWithMock` with a tail
  containing a spinner OSC title, `cc.started.Store(true)`, and an `IdleDetector` whose
  `lastActivity`/`lastActivityNs` is set (via the existing fake-clock test helper) to more than
  `oscStaleThreshold` in the past; assert `cc.classifyOSC(tail)` returns `(dtypes.OSCStatusNone,
  false)`. Add a second case with `lastActivity` recent (within threshold) asserting the OSC title
  is still classified normally — proves the guard doesn't fire on legitimate in-progress work.
  This is the direct regression test for the adversarial-review BLOCKER (stuck-Executing-forever on
  a crashed process).
- Files: `session/claude_controller_test.go`

##### Task 4.1.1b: Add `applyOSCStatusOverride` (~4 min)
- Add, near `classifyOSC`:
  ```go
  // applyOSCStatusOverride applies osc as an asymmetric, upgrade-only overlay on
  // top of textStatus/textDesc: OSCStatusExecuting may promote a low-urgency
  // text result toward StatusExecuting; OSCStatusIdle may only promote
  // Ready/Unknown toward StatusIdle. Neither direction ever demotes a
  // higher-urgency text-pattern result (Error, NeedsApproval, InputRequired,
  // TestsFailing, Success, WaitingForAgent, Executing) — see osc-status-signals
  // plan.md Pattern Decisions for why this is asymmetric rather than a pure
  // OSC-wins short-circuit. Uses detection.IsOSCExecutingPromotable/
  // IsOSCIdlePromotable — the same predicates IdleDetector.DetectStateFromContentWithOSC
  // uses for the IdleState side — so the two overlays can never independently
  // drift (architecture-review.md BLOCKER 2).
  func applyOSCStatusOverride(textStatus detection.DetectedStatus, textDesc string, osc dtypes.OSCStatus) (detection.DetectedStatus, string) {
      switch osc {
      case dtypes.OSCStatusExecuting:
          if detection.IsOSCExecutingPromotable(textStatus) {
              return detection.StatusExecuting, "osc_title: spinner glyph detected"
          }
      case dtypes.OSCStatusIdle:
          if detection.IsOSCIdlePromotable(textStatus) {
              return detection.StatusIdle, "osc_title: idle marker (✳) detected"
          }
      }
      return textStatus, textDesc
  }
  ```
- Files: `session/claude_controller.go`

#### Story 4.1.2: Wire into `GetCurrentStatus`

**As a** frontend status badge consumer, **I want** `GetCurrentStatus`'s returned `DetectedStatus` to
reflect the OSC signal, **so that** AC5's false-idle scenario is fixed for the primary status-read
path.

**Acceptance Criteria**:
- AC5 — OSC wins for the false-idle case.
  - *Given* `ClaudeController.GetCurrentStatus()` is called on a started controller whose PTY tail is
    `"$ \x1b]0;⠋ working\x07"` (a bare shell-looking prompt, but the OSC title still shows the
    spinner), *When* `GetCurrentStatus()` runs, *Then* it returns `detection.StatusExecuting` — not
    `StatusIdle`/`StatusReady`/`StatusUnknown`, which is what text-pattern matching on `"$ "` alone
    would produce.
- AC7 — unchanged fallback when no OSC title is present.
  - *Given* the existing `tmuxOutputSmall` fixture (no OSC sequence), *When* `GetCurrentStatus()` is
    called before and after this feature's changes, *Then* the returned `(status, desc)` is
    byte-for-byte identical.

**Files**: `session/claude_controller.go`

##### Task 4.1.2a: Apply OSC override in `GetCurrentStatus` (~3 min)
- In `GetCurrentStatus`, after the existing Case A/Case B spinner-verb-activity fallback block and
  before `cc.statusCache.Store(...)`, add:
  ```go
  if osc, ok := cc.classifyOSC(tail); ok {
      newStatus, newDesc := applyOSCStatusOverride(status, desc, osc)
      if newStatus != status {
          log.Debug("GetCurrentStatus: OSC override changed status", "session", cc.sessionName, "text_status", status, "osc_status", newStatus)
      }
      status, desc = newStatus, newDesc
  }
  ```
- Files: `session/claude_controller.go`

#### Story 4.1.3: Wire into `GetStatusAndIdleInfo`

**As a** consumer of the combined status+idle call, **I want** both the returned `DetectedStatus` and
`IdleStateInfo` to reflect the OSC signal, **so that** AC5/AC6 are satisfied for the idle-timeout
pipeline too, not just the displayed badge.

**Acceptance Criteria**:
- AC5 (idle-state side) — OSC wins for false-idle in the returned `IdleStateInfo`, too.
  - *Given* a started controller with tail `"$ \x1b]0;⠋ working\x07"`, *When*
    `GetStatusAndIdleInfo()` is called, *Then* the returned `detection.IdleStateInfo.State ==
    detection.IdleStateActive` (not `IdleStateWaiting`).
- Asymmetric safety — OSC `✳` never downgrades an already-Active idle state.
  - *Given* tail = `"esc to interrupt\x1b]0;✳\x07"` (text pattern says Active; OSC title happens to
    show the idle glyph — e.g. a stale/nested title), *When* `GetStatusAndIdleInfo()` is called,
    *Then* the returned `IdleStateInfo.State` remains `IdleStateActive` — the OSC idle marker does
    not override it.

**Files**: `session/claude_controller.go`

##### Task 4.1.3: Apply OSC override in `GetStatusAndIdleInfo` (~5 min)
- In `GetStatusAndIdleInfo`, immediately after `filtered, _ := filterTmuxMetadata(tail)` (shared by
  both the status and idle branches below it), add one shared computation:
  `osc, oscOK := cc.classifyOSC(tail)`.
- In the `!statusHit` branch, after the existing Case A/Case B block, apply the same override as
  Task 4.1.2a (`if oscOK { status, desc = applyOSCStatusOverride(status, desc, osc); ... }`, with
  the same conditional debug log).
- In the `!idleHit` branch, replace the existing
  `idleState = id.DetectStateFromContent(filtered)` call with:
  ```go
  if oscOK {
      idleState = id.DetectStateFromContentWithOSC(filtered, osc)
  } else {
      idleState = id.DetectStateFromContent(filtered)
  }
  ```
  (**Design correction, 2026-08-28**: this replaces the originally-planned two-call
  `DetectStateFromContent` then `ApplyOSCStatus` sequence, which architecture-review.md's BLOCKER 1
  found races the shared `lastStateChange` clock. `DetectStateFromContentWithOSC` resolves both
  candidates and performs one write — see Story 3.1.2. The asymmetric safety policy this task's
  second acceptance criterion tests is now enforced inside `DetectStateFromContentWithOSC` via
  `IsOSCIdlePromotable`, not by an `idleState != IdleStateActive` guard at this call site.)
- Files: `session/claude_controller.go`

#### Story 4.1.4: Wire into `GetIdleState`

**As a** caller of the standalone idle-state accessor (used independently of `GetCurrentStatus`), **I
want** the same OSC-aware idle-state logic applied here too, **so that** all three read paths stay
consistent (no path where OSC is silently ignored).

**Acceptance Criteria**:
- Consistency — `GetIdleState()` and `GetStatusAndIdleInfo()` agree on `IdleState` for the same tail.
  - *Given* the same tail as Story 4.1.3's first scenario (`"$ \x1b]0;⠋ working\x07"`), *When*
    `GetIdleState()` is called, *Then* it returns `detection.IdleStateActive` — matching what
    `GetStatusAndIdleInfo()` returns for the same content.

**Files**: `session/claude_controller.go`

##### Task 4.1.4a: Apply OSC override in `GetIdleState` (~3 min)
- In `GetIdleState`'s cache-miss branch (`if n > 0 { ... state = id.DetectStateFromContent(filtered)
  ... }`), compute `osc, oscOK := cc.classifyOSC(tail)` and replace
  `state = id.DetectStateFromContent(filtered)` with the same
  `if oscOK { state = id.DetectStateFromContentWithOSC(filtered, osc) } else { state =
  id.DetectStateFromContent(filtered) }` pattern as Task 4.1.3's idle branch (2026-08-28 design
  correction — see that task for why).
- Files: `session/claude_controller.go`

#### Task 4.1.5: Build check (~3 min)
- Run `go build ./...` to confirm the new imports and wiring compile cleanly across `session`,
  `session/detection`, `session/detection/binaries`, `session/detection/dtypes`, and `pkg/ansi`. Fix
  any compile errors (import cycles are not expected — `dtypes` has zero dependents among the
  packages this plan touches, `binaries` only imports `dtypes`, `session` already imports
  `detection`).
- Files: any of the above, as needed to fix errors.

### Epic 4.2: Integration test coverage

**Goal**: Directly exercise AC5/AC7 (and the asymmetric safety policy) at the `ClaudeController`
level, using the existing `newControllerWithMock` test fixture.

#### Story 4.2.1: `ClaudeController`-level OSC tests

**As a** reviewer, **I want** end-to-end tests proving the false-idle scenario is actually fixed
(not just its constituent helpers), **so that** AC5/AC7/AC9 have concrete, runnable proof, not just
unit-level coverage of the pieces.

**Acceptance Criteria**:
- AC5, AC7, AC9 — see the Given-When-Then examples already stated in Stories 4.1.2–4.1.4; this
  story's tasks are what makes those runnable.

**Files**: `session/claude_controller_test.go`

##### Task 4.2.1a: False-idle override test (~5 min)
- Add `TestGetCurrentStatus_OSCSpinnerOverridesFalseIdle`: use `newControllerWithMock("$ \x1b]0;⠋
  working\x07")`, call `cc.started.Store(true)` (same-package test, unexported field), call
  `cc.GetCurrentStatus()`, assert `status == detection.StatusExecuting`.
- Files: `session/claude_controller_test.go`

##### Task 4.2.1b: Idle-marker and asymmetric-safety tests (~4 min)
- Add `TestGetCurrentStatus_OSCIdleMarker_PromotesReadyOnlyText`: content with no distinguishing
  text pattern (falls through to Ready/Unknown) plus `\x1b]0;✳\x07`; `cc.started.Store(true)`;
  assert `status == detection.StatusIdle`.
- Add `TestGetCurrentStatus_OSCIdle_DoesNotOverrideActiveText`: content containing `"esc to
  interrupt"` (text pattern → `StatusExecuting`) plus `\x1b]0;✳\x07`; assert `status ==
  detection.StatusExecuting` — proves the asymmetric policy from `applyOSCStatusOverride`'s `case
  dtypes.OSCStatusIdle` switch (Executing is not in the promotable set).
- Files: `session/claude_controller_test.go`

##### Task 4.2.1c: No-OSC fallback test (~3 min)
- Add `TestGetCurrentStatus_NoOSCTitle_FallsBackToTextPattern`: reuse the existing
  `tmuxOutputSmall` fixture (no OSC sequence present), assert the result matches the same assertion
  already made by the pre-existing `TestGetCurrentStatus_CacheHit`/`TestGetCurrentStatus_TailOnlyProcessed`
  tests for that fixture — i.e. confirm zero behavior change (AC7) via a direct comparison, not just
  "doesn't crash."
- Files: `session/claude_controller_test.go`

##### Task 4.2.1d: `GetStatusAndIdleInfo` idle-state wiring test (~4 min)
- Add `TestGetStatusAndIdleInfo_OSCPromotesIdleState`: `newControllerWithMock("$ \x1b]0;⠋
  working\x07")`, `cc.started.Store(true)`, call `cc.GetStatusAndIdleInfo()`, assert
  `idleInfo.State == detection.IdleStateActive` (proves Task 4.1.3's idle-branch wiring, not just
  the status branch already covered by 4.2.1a).
- Files: `session/claude_controller_test.go`

##### Task 4.2.1e: Run and fix (~2 min)
- Run `go test ./session/...`; fix any failures.
- Files: `session/claude_controller.go`, `session/claude_controller_test.go`

---

## Phase 5: Validation

### Epic 5.1: Full regression and AC cross-check

**Goal**: Prove AC8 (no regression) and give the plan's own reviewer a final, explicit cross-check
against all 9 acceptance criteria before calling this done.

#### Story 5.1.1: Full regression + lint + AC cross-check

**As the** implementer, **I want** a final full-suite run and a deliberate re-read of every
acceptance criterion against what was actually built, **so that** "done" is backed by evidence, not
assumption.

**Acceptance Criteria**:
- AC8 — no regression in the existing `session/detection` test suite.
  - *Given* all Phase 1-4 changes committed, *When* `go test ./session/detection/... ./pkg/ansi/...
    ./session/...` is run, *Then* it exits 0, and no pre-existing assertion in `detector_test.go`,
    `pattern_set_test.go`, `bug_regression_test.go`, `asterism_test.go`, or `idle_test.go` was
    modified to make it pass (only new tests/new fields were added).

**Files**: none new — validation only.

##### Task 5.1a: Full test run + lint (~3 min)
- Run `go test ./session/detection/... ./pkg/ansi/... ./session/...` and `make lint` (or the
  project's `golangci-lint run ./session/... ./pkg/ansi/...` equivalent). Fix any issues found.
- Files: any touched in Phases 1-4, as needed.

##### Task 5.1b: AC-by-AC cross-check (~3 min)
- Re-read requirements.md's 9 acceptance criteria against the actual diff and test list one more
  time (not from memory): confirm each of AC1-AC9 has at least one passing test whose name or
  location is traceable to that AC (per the Given-When-Then examples embedded in Stories 1.1.1
  through 4.2.1 above). No code changes expected unless this surfaces a genuine gap — if it does,
  add the missing test/behavior before considering this plan complete.
- Files: none (review only), unless a gap is found.
