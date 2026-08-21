# Detection Architecture Refactors — Requirements

## Project Overview

Refactor the `session/detection` subsystem in stapler-squad to decompose a monolithic
`StatusDetector` into well-separated, pluggable, and testable components. The work is
decomposed into four coupled items that should be implemented in dependency order.

---

## Background

`StatusDetector` (`session/detection/detector.go`, ~950 lines) currently owns 9+
responsibilities: pattern storage, regex compilation, ANSI stripping, CR-collapse, detection
dispatch, ring-buffer event logging, session-ID management, YAML I/O, and line scanning.

Two independent `*StatusDetector` instances exist per active session: one inside
`IdleDetector` (idle.go:~80) and one in `ClaudeController` (claude_controller.go:~208),
each with their own ring buffers, creating duplicated work and divergent event histories.

Callers across 5 files hold concrete `*detection.StatusDetector` fields, making the type
impossible to mock or swap without changing all call sites.

Binary detection patterns (Claude Code, Gemini, Aider, OpenCode, agy) are mixed in a
single flat `StatusPatterns` struct; adding a new AI binary requires editing shared code.

---

## Item 1 — BinaryDetector Interface + DetectorRegistry

### Problem
`DetectForProgram(output []byte, program string)` ignores the `program` parameter entirely.
All binaries share one flat pattern set. Adding Cursor/Windsurf requires modifying shared
structs rather than appending a new file.

### Requirements
- R1.1: Define a `BinaryDetector` interface in `session/detection/` with at minimum:
  - `Name() string` — canonical binary name (e.g. `"claude"`, `"gemini"`)
  - `Patterns() StatusPatterns` — per-binary pattern set
  - `FilterContent(content string) string` — optional pre-processing hook (e.g. strip agy frame lines)
- R1.2: Implement a `DetectorRegistry` that maps binary name → `BinaryDetector`
- R1.3: Split `getDefaultPatterns()` into per-binary files under `session/detection/binaries/`:
  - `claude.go`, `gemini.go`, `aider.go`, `opencode.go`, `agy.go` (or similar)
  - Each file registers itself via `init()` or explicit `Register()` in a `DefaultRegistry()`
- R1.4: `DetectForProgram()` must look up the binary in the registry and use its pattern set
- R1.5: `DefaultRegistry()` must produce an equivalent result to current `getDefaultPatterns()` — no behavioral change for existing binaries
- R1.6: Unit tests verify per-binary registration, fallback to default when binary unknown

### Out of Scope
- Changing how callers invoke detection (that is Item 2)
- Changing the StatusPatterns struct layout

---

## Item 2 — TerminalDetector Interface at 5 Call Sites

### Problem
Five files hold concrete `*detection.StatusDetector` fields:
- `session/review_queue_poller.go` — field `statusDetector`
- `session/review_queue_determiner.go` — parameter and usage
- `session/command_executor.go` (if applicable)
- `session/startup_scanner.go` (if applicable)
- `session/claude_controller.go` — field

This makes the type impossible to substitute (for testing or for a per-binary detector
returned from the registry).

### Requirements
- R2.1: Define a `TerminalDetector` interface in `session/detection/` covering the methods
  all 5 call sites actually use:
  - `DetectWithContextFromLines(lines []string) (DetectedStatus, string)`
  - `DetectFromLines(lines []string) DetectedStatus`
  - `RecentEvents(n int) []DetectionEvent`
  - `SetSessionID(id string)`
  - Any other method used by the 5 confirmed call sites (verify by reading the files)
- R2.2: `*StatusDetector` must satisfy `TerminalDetector` (already does; just make it explicit with a compile-time check: `var _ TerminalDetector = (*StatusDetector)(nil)`)
- R2.3: All 5 call-site files change their concrete `*detection.StatusDetector` fields/params to `detection.TerminalDetector`
- R2.4: Constructor injection: callers that call `detection.NewStatusDetector()` inline now receive a `TerminalDetector`
- R2.5: Existing tests must still pass; the interface should not break any test that constructs a real `*StatusDetector`
- R2.6: A minimal fake/mock implementing `TerminalDetector` should be usable in unit tests without a real `StatusDetector`

### Out of Scope
- Implementing the mock (just the interface; a test double can be added separately)

---

## Item 3 — Dual-Detector Consolidation

### Problem
`IdleDetector` creates its own `*StatusDetector` internally (`idle.go:~80`).
`ClaudeController` creates a separate one (`claude_controller.go:~208`).
Per session: two regex engines, two ring buffers, two event histories. Any detection event
observed by `ClaudeController` is invisible to `IdleDetector`'s ring buffer and vice versa.

### Requirements
- R3.1: `NewIdleDetectorWithConfig` must accept a `TerminalDetector` parameter for injection
- R3.2: When a `TerminalDetector` is injected, `IdleDetector` must use it instead of creating a new `StatusDetector`
- R3.3: When no detector is injected (nil), `IdleDetector` creates its own (backward-compatible default)
- R3.4: `ClaudeController` must inject its own detector into `NewIdleDetectorWithConfig` at construction time
- R3.5: Both components now share one regex engine, one ring buffer, one event history
- R3.6: Tests confirm that events recorded via the injected detector appear in `RecentEvents()` from both the controller and idle detector sides

### Depends On
- Item 2 (TerminalDetector interface) must be complete before this item

---

## Item 4 — SRP Split of StatusDetector

### Problem
`StatusDetector` has 9 responsibilities. This is the largest, riskiest refactor.

### Requirements
- R4.1: Extract `PTYNormalizer` — owns ANSI stripping and CR-collapse logic
  - Methods: `Normalize(content string) string`, `SplitLines(content string) []string`
  - Used by `StatusDetector` internally; callers never interact with it directly
- R4.2: Extract `PatternSet` — owns one binary's compiled regexes + match dispatch
  - Built from a `StatusPatterns` struct
  - Method: `MatchLines(lines []string) (DetectedStatus, string)`
  - This enables the `BinaryDetector` registry (Item 1) to produce swappable `PatternSet`s
- R4.3: Extract `DetectionEventSink` — owns ring buffer and session ID
  - Methods: `Record(event DetectionEvent)`, `Recent(n int) []DetectionEvent`, `SetSessionID(id string)`
  - Shareable between `StatusDetector` and `IdleDetector` (enabling Item 3)
- R4.4: `StatusDetector` becomes a thin orchestrator: `PTYNormalizer` → `PatternSet` → `DetectionEventSink`
- R4.5: All existing `StatusDetector` methods remain on the struct (no API change to callers)
- R4.6: YAML I/O (`LoadPatterns`, `ExportPatterns`) stays on `StatusDetector` — no justification to move it
- R4.7: All existing tests pass without modification (internal split, not API change)
- R4.8: New unit tests cover `PTYNormalizer` and `PatternSet` in isolation

### Depends On
- Items 1, 2, 3 should be stable before starting this item (reduces merge complexity)

---

## Implementation Order

Items are coupled by dependency:

```
Item 1 (BinaryDetector interface + registry)
  ↓
Item 2 (TerminalDetector interface — 5 call sites)
  ↓
Item 3 (Dual-detector consolidation — inject shared detector)
  ↓
Item 4 (SRP split — extract PTYNormalizer, PatternSet, DetectionEventSink)
```

Items 1 and 2 are independent and can be implemented in parallel.
Items 3 and 4 depend on Item 2 being complete.

---

## Non-Goals

- Changing detection pattern content (what regexes match)
- Adding new binary detectors (Cursor, Windsurf, etc.) — the registry enables this but is not in scope
- Changing the proto/RPC surface
- Frontend changes

---

## Acceptance Criteria

- `make build` passes (all 4 items)
- `make test` passes (all 4 items)
- `make lint` passes (all 4 items)
- No behavioral change: sessions detect the same statuses as before
- `TerminalDetector` interface compile-time check present
- `BinaryDetector` registry has ≥5 registered binaries (existing 5)
- `IdleDetector` + `ClaudeController` share one `StatusDetector` instance per session
- `StatusDetector` delegates to `PTYNormalizer`, `PatternSet`, `DetectionEventSink`
- Existing test suite passes without modification (except new tests added)
