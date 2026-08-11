# Detection Architecture Refactors — Implementation Plan

**Feature**: Decompose monolithic StatusDetector into pluggable, testable components
**Date**: 2026-06-14
**Status**: Ready for implementation
**ADRs**: None required (all choices follow existing ADR-011 lock-free patterns)

---

## Dependency Visualization

```
Epic 1 (BinaryDetector interface + registry)   ← no dependencies
Epic 2 (TerminalDetector interface)            ← no dependencies (parallel with Epic 1)
  ↓
Epic 3 (Dual-detector consolidation)           ← requires Epic 2 complete
  ↓
Epic 4 (SRP split: PTYNormalizer, PatternSet,  ← requires Epics 1,2,3 stable
         DetectionEventSink)
```

Epics 1 and 2 are independent and can be implemented in parallel. Epics 3 and 4 are
sequentially dependent on prior epics.

---

## Critical Pre-Implementation Constraints

These invariants must be preserved across all epics:

1. **LoadPatterns concurrent-mutation hazard**: `LoadPatterns()` mutates 9 compiled-regex
   slices outside any lock. It is only called in tests and via `NewStatusDetectorFromFile`.
   The plan mitigates this in Epic 4 by making `PatternSet` a frozen-at-construction value
   type; `LoadPatterns` must acquire an RW lock or be restricted to pre-first-Detect semantics
   until Epic 4 completes.

2. **EventRingCap must increase to ≥2000 before sharing**: When `ClaudeController` and
   `IdleDetector` share one `StatusDetector`, `detectFromLines` makes up to 50
   `appendDetectionEvent` calls per status check (one per non-blank line). At 1 Hz polling,
   a 500-slot ring drains in ~5 seconds. Epic 3 Story 3.3 must increase `EventRingCap` to
   2000 before wiring up the shared instance.

3. **Injection constructors before struct changes**: Nine `idle_test.go` tests directly
   assign `d.now`, `d.lastStateChange`, and `d.lastActivity`. Epic 3 Story 3.1 must add
   the injection constructor for `IdleDetector` BEFORE any field relocation in Epic 4.

4. **Interface location**: `TerminalDetector` and `BinaryDetector` interfaces are defined in
   `session/detection/` (consumer-of-input perspective, and multi-consumer pattern per ADR
   research). This avoids circular imports since `session/detection` does not import `session`.

---

## Epic 1: BinaryDetector Interface + Registry

**Goal**: Make it possible to register per-binary pattern sets so `DetectForProgram` can
dispatch to the correct pattern set. No behavioral change for existing binaries.

### Story 1.1: Define BinaryDetector Interface

**As a** developer adding a new AI binary, **I want** to implement a single interface and
register it, **so that** detection patterns for my binary are picked up without editing
shared code.

**Acceptance Criteria**:
- `BinaryDetector` interface is defined in `session/detection/`
- Interface has exactly: `Name() string`, `Patterns() StatusPatterns`, `FilterContent(content string) string`
- `DetectorRegistry` type exists with `Register(BinaryDetector)` and `Lookup(name string) (BinaryDetector, bool)` methods
- `DefaultRegistry()` returns a registry with all 5 existing binaries registered
- Compile-time check: existing code calling `getDefaultPatterns()` still compiles (function stays in place for now)

**Files**:
- `session/detection/binary_detector.go` (new)
- `session/detection/registry.go` (new)

##### Task 1.1.1: Create `binary_detector.go` with interface and registry (~3 min)
- Create `session/detection/binary_detector.go`
- Define `BinaryDetector` interface with three methods: `Name() string`, `Patterns() StatusPatterns`, `FilterContent(content string) string`
- Define `DetectorRegistry` struct with `map[string]BinaryDetector` field (explicit map, not `init()` side effects)
- Implement `Register(d BinaryDetector)` — panics if duplicate name in development build
- Implement `Lookup(name string) (BinaryDetector, bool)` — returns false if name not found
- Implement `DefaultRegistry() *DetectorRegistry` — stub that returns an empty registry for now; Story 1.2 will populate it with the 5 binary constructors
- Files: `session/detection/binary_detector.go`
- Tests: `session/detection/registry_test.go` (new) — verify Register/Lookup round-trip, unknown-name returns false

### Story 1.2: Split Per-Binary Pattern Files

**As a** developer, **I want** each binary's patterns in its own file, **so that** adding
Cursor or Windsurf requires creating one new file.

**Acceptance Criteria**:
- `session/detection/binaries/` package exists
- 5 files: `claude.go`, `gemini.go`, `aider.go`, `opencode.go`, `agy.go`
- Each file implements `BinaryDetector` and exports a constructor
- Each file's `Patterns()` returns the same patterns currently returned by `getDefaultPatterns()` for that binary
- `DefaultRegistry()` calls each constructor and registers it

**Files**:
- `session/detection/binaries/claude.go` (new)
- `session/detection/binaries/gemini.go` (new)
- `session/detection/binaries/aider.go` (new)
- `session/detection/binaries/opencode.go` (new)
- `session/detection/binaries/agy.go` (new)
- `session/detection/registry.go` (update `DefaultRegistry()`)

##### Task 1.2.1: Extract claude patterns to `binaries/claude.go` (~4 min)
- Create `session/detection/binaries/claude.go` in package `binaries`
- Define `claudeDetector` struct implementing `BinaryDetector`
- `Name()` returns `"claude"`
- `Patterns()` returns the claude-specific subset of `getDefaultPatterns()` (Ready, Processing, NeedsApproval, InputRequired, Error patterns currently associated with Claude)
- `FilterContent(content string) string` returns content unchanged (no special preprocessing for claude)
- Export `NewClaudeDetector() BinaryDetector`
- Files: `session/detection/binaries/claude.go`
- Tests: `session/detection/binaries/claude_test.go` — verify Name(), verify Patterns() contains expected pattern names

##### Task 1.2.2: Extract gemini, aider, opencode, agy to their own files (~4 min)
- Same structure as Task 1.2.1 for each remaining binary
- `agy.go` must implement `FilterContent` to strip agy frame lines (if applicable per current `getDefaultPatterns`)
- Files: `session/detection/binaries/gemini.go`, `aider.go`, `opencode.go`, `agy.go`
- Tests: each file's `_test.go` verifies Name() and at least one expected pattern name

##### Task 1.2.3: Wire DefaultRegistry to per-binary constructors (~2 min)
- Update `DefaultRegistry()` in `session/detection/registry.go`
- Import `session/detection/binaries` package
- Register all 5 constructors: `NewClaudeDetector()`, `NewGeminiDetector()`, etc.
- Add test in `registry_test.go`: `DefaultRegistry()` has exactly 5 entries
- Files: `session/detection/registry.go`

### Story 1.3: Wire DetectForProgram to Registry

**As a** caller of `DetectForProgram`, **I want** the program parameter to actually be used,
**so that** per-binary pattern dispatch works.

**Acceptance Criteria**:
- `DetectForProgram` looks up `program` in `DefaultRegistry()`, uses that binary's `PatternSet` if found
- Falls back to `getDefaultPatterns()` if binary not in registry
- Existing behavior preserved for all 5 known binaries
- `// DEPRECATED` comment removed from `DetectForProgram`

**Files**:
- `session/detection/detector.go`

##### Task 1.3.1: Update `DetectForProgram` to use registry (~3 min)
- Add `defaultRegistry *DetectorRegistry` field to `StatusDetector` (set in `NewStatusDetector`)
- OR use a package-level `defaultRegistry` var initialized by `DefaultRegistry()` call at package init (simpler; avoids field)
- Implement `DetectForProgram`: lookup program in registry, use its `Patterns()` if found, else use `sd.patterns`
- Remove `// DEPRECATED` comment
- Files: `session/detection/detector.go`
- Tests: `session/detection/detector_test.go` — add `TestDetectForProgram_UsesRegisteredPatterns` verifying a known binary gets its pattern set; add `TestDetectForProgram_FallsBackForUnknownBinary`

---

## Epic 2: TerminalDetector Interface at 5 Call Sites

**Goal**: All concrete `*detection.StatusDetector` fields become `detection.TerminalDetector`
interface fields, enabling test injection and future per-binary detector swapping.

### Story 2.1: Define TerminalDetector Interface

**As a** test author, **I want** to inject a fake `TerminalDetector`, **so that** unit
tests for ClaudeController, ReviewQueuePoller, and related types don't require a real
`*StatusDetector`.

**Acceptance Criteria**:
- `TerminalDetector` interface in `session/detection/` with 4 methods:
  - `DetectWithContext(output []byte) (DetectedStatus, string)`
  - `DetectWithContextFromLines(lines []string) (DetectedStatus, string)`
  - `RecentEvents(n int) []DetectionEvent`
  - `SetSessionID(id string)`
- Compile-time check: `var _ TerminalDetector = (*StatusDetector)(nil)` in same file
- All existing `*StatusDetector` tests pass unchanged

**Files**:
- `session/detection/terminal_detector.go` (new)

##### Task 2.1.0: Audit actual method calls at all 5 call sites (~2 min)
- Before writing the interface, grep all 5 files for every method call on their `*detection.StatusDetector` or `statusDetector` field:
  - `session/review_queue_poller.go`
  - `session/review_queue_determiner.go`
  - `session/claude_controller.go`
  - `session/command_executor.go`
  - `session/startup_scanner.go`
  - `server/services/session_service.go`
- Collect the complete set of distinct method names. Expected from research: `DetectWithContext`, `DetectWithContextFromLines`, `RecentEvents`, `SetSessionID`. If `DetectFromLines` appears at any call site, add it to the interface below.
- Files: read-only audit, no file changes
- Tests: n/a

##### Task 2.1.1: Create `terminal_detector.go` with interface and compile-time check (~2 min)
- Create `session/detection/terminal_detector.go`
- Define `TerminalDetector` interface with the methods confirmed in Task 2.1.0 (at minimum the 4 listed above; add `DetectFromLines(lines []string) DetectedStatus` if found at any call site)
- Add compile-time assertion: `var _ TerminalDetector = (*StatusDetector)(nil)`
- Files: `session/detection/terminal_detector.go`
- Tests: None required — the compile-time assertion is the test; the `go build` step in Story 2.3 confirms all call sites are covered

### Story 2.2: Update `StatusDeterminer` Interface and `DefaultStatusDeterminer`

**As a** developer, **I want** `StatusDeterminer.Determine` to accept `TerminalDetector`
instead of `*detection.StatusDetector`, **so that** the determiner can accept any detector
implementation.

**Acceptance Criteria**:
- `StatusDeterminer` interface parameter changes from `*detection.StatusDetector` to `detection.TerminalDetector`
- `DefaultStatusDeterminer.Determine` signature updated to match
- Callers in `review_queue_poller.go` compile without changes (they pass their `statusDetector` field, which changes type in Story 2.3)
- Existing `review_queue_determiner_test.go` tests pass unchanged

**Files**:
- `session/review_queue_determiner.go`

##### Task 2.2.1: Update `StatusDeterminer` interface parameter type (~2 min)
- Change `Determine(inst *Instance, content string, statusInfo InstanceStatusInfo, detector *detection.StatusDetector) DetectionResult`
  to `Determine(inst *Instance, content string, statusInfo InstanceStatusInfo, detector detection.TerminalDetector) DetectionResult`
- Update `DefaultStatusDeterminer.Determine` signature to match
- Verify `review_queue_determiner.go` compiles (`detector.DetectWithContextFromLines` is on `TerminalDetector`)
- Files: `session/review_queue_determiner.go`
- Tests: run `go build ./session/...` to confirm no errors; existing tests verify behavior

### Story 2.3: Change All 5 Call-Site Fields/Parameters to TerminalDetector

**As a** developer, **I want** no concrete `*detection.StatusDetector` fields outside the
detection package, **so that** the type is fully substitutable.

**Acceptance Criteria**:
- `ReviewQueuePoller.statusDetector` field type: `detection.TerminalDetector`
- `ClaudeController.statusDetector` atomic field type: `atomic.Pointer[detection.TerminalDetector]` — BUT see note below
- `session/startup_scanner.go`: any `*detection.StatusDetector` field changed to `detection.TerminalDetector`
- `session/command_executor.go`: any `*detection.StatusDetector` field changed to `detection.TerminalDetector`
- `server/services/session_service.go`: `GetStatusDetector()` return type becomes `detection.TerminalDetector`
- `ClaudeController.GetStatusDetector()` return type becomes `detection.TerminalDetector`
- `make build` passes

**Note on `atomic.Pointer` and interface types**: `atomic.Pointer[T]` in Go requires T to be
a non-interface, non-pointer base type — `atomic.Pointer[TerminalDetector]` is invalid because
`TerminalDetector` is an interface. The correct approach: keep `atomic.Pointer[detection.StatusDetector]`
internally in `ClaudeController` (concrete type, valid for atomic.Pointer); expose the interface
exclusively through the `GetStatusDetector() detection.TerminalDetector` accessor. Callers outside
the package never see `*StatusDetector` directly — only `TerminalDetector`. Do not use
`Locked[detection.TerminalDetector]` as an alternative; it would add unnecessary lock contention
on read-only access paths that the current `atomic.Pointer` design deliberately avoids (ADR-011).

**Files**:
- `session/review_queue_poller.go`
- `session/review_queue_determiner.go` (already done in Story 2.2)
- `session/claude_controller.go`
- `session/startup_scanner.go` (if applicable)
- `session/command_executor.go` (if applicable)
- `server/services/session_service.go`

##### Task 2.3.1: Update ReviewQueuePoller field and constructor (~3 min)
- Change `statusDetector *detection.StatusDetector` to `statusDetector detection.TerminalDetector` in `ReviewQueuePoller` struct
- Update `NewReviewQueuePollerWithConfig` to accept `detection.TerminalDetector` OR keep `NewStatusDetector()` internal construction
  (preferred: keep internal construction for now, just change the field type since `NewStatusDetector()` satisfies `TerminalDetector`)
- Update the field declaration; the construction (`detection.NewStatusDetector()`) returns `*StatusDetector` which satisfies `TerminalDetector`
- Files: `session/review_queue_poller.go`
- Tests: `go build ./session/...` confirms no breakage

##### Task 2.3.2: Update ClaudeController and expose TerminalDetector via accessor (~3 min)
- `ClaudeController.statusDetector` remains `atomic.Pointer[detection.StatusDetector]` internally
- Change `GetStatusDetector() *detection.StatusDetector` to `GetStatusDetector() detection.TerminalDetector`
- Update `server/services/session_service.go` call site: return type becomes `detection.TerminalDetector` and the caller uses `RecentEvents()` only (already on the interface)
- Files: `session/claude_controller.go`, `server/services/session_service.go`
- Tests: `go build ./...` confirms no breakage; existing `GetDetectionEvents` RPC test passes

##### Task 2.3.3: Update command_executor.go and startup_scanner.go (~3 min)
- Read each file; if `statusDetector *detection.StatusDetector` field exists, change to `detection.TerminalDetector`
- Update any constructor injection to accept `detection.TerminalDetector`
- Files: `session/command_executor.go`, `session/startup_scanner.go`
- Tests: `make build` passes; existing tests unaffected

---

## Epic 3: Dual-Detector Consolidation

**Goal**: `ClaudeController` and `IdleDetector` share one `StatusDetector` instance per
session. Requires Epic 2 complete (TerminalDetector interface).

### Story 3.1: Add Injection Constructor to IdleDetector

**Acceptance Criteria**:
- `NewIdleDetectorWithDetector(sessionName string, ptyAccess PTYReader, config IdleDetectorConfig, detector TerminalDetector) *IdleDetector` exists
- When `detector` is non-nil, it is used instead of `NewStatusDetector()`
- When `detector` is nil, falls back to `NewStatusDetector()` (backward compat)
- `IdleDetector.statusDetector` field type changes to `TerminalDetector` (interface)
- All 9 existing `idle_test.go` fake-clock tests pass unchanged (no struct field changes in this story — `d.now`, `d.lastStateChange`, `d.lastActivity` stay put until Epic 4)

**Files**:
- `session/detection/idle.go`

##### Task 3.1.1: Add injection constructor without touching struct fields (~3 min)
- Add `NewIdleDetectorWithDetector(sessionName string, ptyAccess PTYReader, config IdleDetectorConfig, detector TerminalDetector) *IdleDetector`
- Change `IdleDetector.statusDetector` field type to `TerminalDetector` (interface)
- Update `NewIdleDetectorWithConfig` to assign `NewStatusDetector()` to the field (nil injection = own detector)
- `NewIdleDetectorWithDetector` assigns the passed-in detector to the field
- Verify all `idle_test.go` tests still compile: `d.now`, `d.lastStateChange`, `d.lastActivity` are NOT moved in this story
- Files: `session/detection/idle.go`
- Tests: run `go test ./session/detection/...` — all idle tests must pass

### Story 3.2: Increase EventRingCap to 2000

**MUST complete before Story 3.3** — the ring cap increase must be in place before the shared
instance is wired in; otherwise the shared 500-slot ring drains in ~5 seconds at 1 Hz polling.

**Acceptance Criteria**:
- `EventRingCap` constant in `session/detection/events.go` is 2000 (was 500)
- Rationale comment added: "Increased from 500: ClaudeController and IdleDetector share one ring; detectFromLines makes up to 50 appendDetectionEvent calls per status check at 1 Hz, draining a 500-slot ring in seconds"
- No test changes required (ring tests use the constant)

**Files**:
- `session/detection/events.go`

##### Task 3.2.1: Update EventRingCap constant (~1 min)
- Change `EventRingCap = 500` to `EventRingCap = 2000`
- Add rationale comment
- Files: `session/detection/events.go`
- Tests: `go test ./session/detection/...` — ring buffer tests use the constant and pass

### Story 3.3: Wire ClaudeController to Inject Its Detector

**Depends on Story 3.2 (EventRingCap = 2000) being merged first.**

**Acceptance Criteria**:
- `ClaudeController.Start()` calls `NewIdleDetectorWithDetector(..., sd)` passing the controller's own `*StatusDetector`
- Both `cc.statusDetector` and `cc.idleDetector` internal components reference the same underlying `*StatusDetector` instance
- Events from both call paths appear in `RecentEvents()` via the shared ring buffer
- Existing `ClaudeController` tests pass
- New test `TestClaudeController_SharedDetectorEvents` confirms cross-component event visibility (required by R3.6)

**Files**:
- `session/claude_controller.go`
- `session/claude_controller_test.go` (or nearest test file for ClaudeController)

##### Task 3.3.1: Update Start() to inject shared detector into IdleDetector (~3 min)
- In `ClaudeController.Start()`, change:
  ```go
  id := detection.NewIdleDetector(cc.sessionName, pa)
  ```
  to:
  ```go
  id := detection.NewIdleDetectorWithDetector(cc.sessionName, pa, detection.DefaultIdleDetectorConfig(), sd)
  ```
- Remove any `SetSessionID` call on a separate idle detector instance (only the shared `sd` needs it)
- Files: `session/claude_controller.go`
- Tests: `make test` — confirm existing ClaudeController tests pass

##### Task 3.3.2: Add shared-events integration test (R3.6) (~4 min)
- Add `TestClaudeController_SharedDetectorEvents` in the ClaudeController test file
- Test structure: construct a `*StatusDetector` with `SetSessionID("test-session")`;
  call `NewIdleDetectorWithDetector` with that detector; drive one detect call through
  the idle detector path (`DetectStateFromContent`) and one through the controller path
  (`DetectWithContextFromLines`); assert that both events appear in a single
  `RecentEvents(10)` call on the shared detector
- This verifies R3.6: "events recorded via the injected detector appear in RecentEvents() from both sides"
- Files: `session/detection/idle_test.go` OR a new `session/detection/shared_detector_test.go`
- Tests: `go test ./session/detection/...` — new test passes

### Story 3.4 (renumbered from original 3.3): No-op — EventRingCap moved to Story 3.2

**Acceptance Criteria**:
- `EventRingCap` constant in `session/detection/events.go` is 2000 (was 500)
- Rationale comment added: "Increased from 500 because ClaudeController and IdleDetector share one ring; detectFromLines makes up to 50 calls per status check"
- No test changes required (ring tests use the constant)

**Files**:
- `session/detection/events.go`

##### Task 3.3.1: Update EventRingCap constant (~1 min)
- Change `EventRingCap = 500` to `EventRingCap = 2000`
- Add rationale comment
- Files: `session/detection/events.go`
- Tests: `go test ./session/detection/...` — ring buffer tests use the constant and pass

---

## Epic 4: SRP Split of StatusDetector

**Goal**: `StatusDetector` becomes a thin orchestrator delegating to `PTYNormalizer`,
`PatternSet`, and `DetectionEventSink`. Requires Epics 1, 2, 3 stable.
This is the largest, riskiest epic; all existing tests must pass without modification.

### Story 4.1: Extract PTYNormalizer

**Acceptance Criteria**:
- `PTYNormalizer` struct in `session/detection/normalizer.go`
- Methods: `Normalize(content string) string`, `SplitLines(content string) []string`
- `StatusDetector` delegates ANSI stripping and CR-collapse to `PTYNormalizer`
- No double-normalization: `detectFromLines` calls `detectFromText` directly with pre-normalized lines (not via `DetectWithContext` which would re-normalize)
- All existing `detector_test.go` tests pass unchanged
- New tests cover `PTYNormalizer` in isolation

**Files**:
- `session/detection/normalizer.go` (new)
- `session/detection/detector.go` (update detection hot path)
- `session/detection/normalizer_test.go` (new)

##### Task 4.1.1: Create `normalizer.go` with PTYNormalizer struct (~3 min)
- Create `session/detection/normalizer.go`
- Define `PTYNormalizer` struct (no exported fields)
- `Normalize(content string) string` — calls `stripANSI(collapseCarriageReturns(content))` in sequence
- `SplitLines(content string) []string` — splits normalized content into non-blank lines (matches current `lastNLines` logic)
- Do NOT make `PTYNormalizer` an interface (avoids heap allocation on hot path per pitfall research)
- Files: `session/detection/normalizer.go`
- Tests: `session/detection/normalizer_test.go` — test Normalize with ANSI sequences, test SplitLines empty/multiline

##### Task 4.1.2: Wire PTYNormalizer into StatusDetector (~4 min)
- First verify: confirm `detectFromText` exists as a distinct private method on `StatusDetector` (separate from `detectFromLines` and `DetectWithContext`). If it does not exist as a standalone method, extract it first by splitting the inline normalization + matching logic in `DetectWithContext` into two steps: `sd.normalizer.Normalize(...)` and then the pattern matching call.
- Add `normalizer PTYNormalizer` field to `StatusDetector`
- Initialize in `NewStatusDetector()`: `normalizer: PTYNormalizer{}`
- Update `Detect` and `DetectWithContext` to call `sd.normalizer.Normalize(string(output))` instead of inline `stripANSI(collapseCarriageReturns(...))`
- Update `detectFromLines` to call the pattern-matching step directly with pre-normalized string lines — NOT via `DetectWithContext` — to prevent double-normalization (each line would otherwise be re-stripped)
- Files: `session/detection/detector.go`
- Tests: run `go test ./session/detection/...` — all existing tests must pass

### Story 4.2: Extract PatternSet

**Acceptance Criteria**:
- `PatternSet` struct in `session/detection/pattern_set.go`
- Owns all 9 compiled regex slices and the `StatusPatterns` struct
- Method: `MatchLines(lines []string) (DetectedStatus, string)` — wraps current `detectFromText` logic
- `StatusDetector` holds a `PatternSet` field and delegates all pattern matching to it
- `LoadPatterns` on `StatusDetector` must acquire a write lock before mutating `PatternSet` fields (addresses LoadPatterns concurrent-mutation hazard)
- All existing tests pass

**Files**:
- `session/detection/pattern_set.go` (new)
- `session/detection/detector.go` (delegate to PatternSet)
- `session/detection/pattern_set_test.go` (new)

##### Task 4.2.1: Create `pattern_set.go` with PatternSet struct (~5 min)
- Create `session/detection/pattern_set.go`
- Define `PatternSet` struct with: `patterns StatusPatterns`, 9 compiled regex slice fields, `mu sync.RWMutex` (for LoadPatterns safety)
- `NewPatternSet(p StatusPatterns) (*PatternSet, error)` — compiles all regexes, returns error on compile failure
- `MatchLines(lines []string) (DetectedStatus, string)` — runs the current `detectFromText` priority chain under `mu.RLock()`
- `GetPatternNames() []string` and `HasPattern(name string) bool` — delegate to PatternSet
- Files: `session/detection/pattern_set.go`
- Tests: `session/detection/pattern_set_test.go` — test MatchLines with known-status strings, test NewPatternSet with invalid regex

##### Task 4.2.2: Refactor StatusDetector to use PatternSet (~4 min)
- Replace the 9 compiled regex fields in `StatusDetector` with a single `patternSet *PatternSet` field
- Update `NewStatusDetector()` to call `NewPatternSet(getDefaultPatterns())`
- Update `NewStatusDetectorFromFile` to call `NewPatternSet(patterns)`
- Update `LoadPatterns` using the definitive locking strategy: after parsing YAML, call
  `NewPatternSet(patterns)` to build a fully-compiled new PatternSet (no in-place mutation),
  then add a `patternSetMu sync.RWMutex` field on `StatusDetector` and acquire
  `sd.patternSetMu.Lock()` before swapping `sd.patternSet = newSet`. Detection callers
  (`Detect`, `DetectWithContext`, `detectFromLines`) acquire `sd.patternSetMu.RLock()` before
  reading `sd.patternSet`. This is the only locking strategy used — do not combine with
  PatternSet's internal `mu` for the pointer swap (that would double-lock).
  PatternSet's internal `mu` guards its own field reads (MatchLines); the StatusDetector's
  `patternSetMu` guards the pointer-level swap on LoadPatterns.
- Update `detectFromText`/`detectFromLines` to acquire `sd.patternSetMu.RLock()` and delegate to `sd.patternSet.MatchLines()`
- Files: `session/detection/detector.go`
- Tests: run `go test ./session/detection/...` — all existing tests pass; add a data-race test via `go test -race ./session/detection/...`

##### Task 4.2.3: Migrate `getDefaultPatterns` to use binaries package (~3 min)
- `getDefaultPatterns()` now assembles `StatusPatterns` by merging patterns from all registered binaries in `DefaultRegistry()` (maintains backward compatibility for callers not using per-binary dispatch)
- Merge strategy: for each of the 9 `StatusPatterns` categories (Ready, Processing, etc.), append each binary's slice into the combined slice. Preserve the same order as the current hand-written `getDefaultPatterns()` so the priority chain is unchanged.
- After merging, assert no duplicate pattern names: collect all `StatusPattern.Name` fields across all categories into a map and panic (or return an error) if any name appears more than once. This catches accidental pattern duplication introduced when splitting per-binary files.
- Files: `session/detection/detector.go`
- Tests: `TestGeminiPatterns_AgyCoverage` still calls `getDefaultPatterns()` and must still find expected pattern names; add `TestDefaultPatterns_NoDuplicateNames` to `detector_test.go` that calls `getDefaultPatterns()` and asserts all pattern names are unique across all 9 categories

### Story 4.3: Extract DetectionEventSink

**Acceptance Criteria**:
- `DetectionEventSink` struct in `session/detection/event_sink.go`
- Fields: `sessionID string`, `ring eventRing`
- Methods: `Record(event DetectionEvent)`, `Recent(n int) []DetectionEvent`, `SetSessionID(id string)`
- `StatusDetector` holds a `DetectionEventSink` field and delegates ring operations to it
- The `sessionID` field moves from `StatusDetector` to `DetectionEventSink`
- All existing tests pass

**Files**:
- `session/detection/event_sink.go` (new)
- `session/detection/detector.go` (delegate to DetectionEventSink)

##### Task 4.3.1: Create `event_sink.go` with DetectionEventSink struct (~3 min)
- Create `session/detection/event_sink.go`
- Define `DetectionEventSink` struct: `sessionID string`, `ring eventRing`
- `SetSessionID(id string)` — acquires `ring.mu`, sets `sessionID`
- `Record(e DetectionEvent)` — acquires `ring.mu`, calls `ring.pushLocked`
- `Recent(n int) []DetectionEvent` — delegates to `ring.recent(n)`
- Files: `session/detection/event_sink.go`
- Tests: add `TestDetectionEventSink_RecordAndRecent` in new `event_sink_test.go`

##### Task 4.3.2: Wire DetectionEventSink into StatusDetector (~3 min)
- Remove `sessionID string` and `ring eventRing` fields from `StatusDetector`
- Add `sink DetectionEventSink` field
- Update `SetSessionID` to delegate to `sd.sink.SetSessionID`
- Update `RecentEvents` to delegate to `sd.sink.Recent`
- Update `appendDetectionEvent` to delegate to `sd.sink.Record`
- Files: `session/detection/detector.go`
- Tests: `go test ./session/detection/...` — all existing tests pass

### Story 4.4: Final Validation Pass

**Acceptance Criteria**:
- `make build` passes
- `make test` passes
- `make lint` passes
- `make quick-check` passes clean
- No behavioral changes: integration tests produce same status detection results

**Files**: none (validation only)

##### Task 4.4.1: Run full build and test suite (~3 min)
- `make build && make test && make lint`
- Fix any compilation errors from the decomposition
- Files: any file with remaining compilation issues
- Tests: `make ci` passes

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| `LoadPatterns` data race under sharing | Medium | High | Epic 4 Story 4.2 uses new PatternSet + pointer-swap under `patternSetMu sync.RWMutex`; single strategy, no ambiguity |
| Ring buffer history collapse at 500 cap | High (if sharing without cap increase) | Medium | Epic 3 Story 3.2 increases `EventRingCap` to 2000; Story 3.3 (wiring) explicitly depends on 3.2 being merged first |
| `idle_test.go` field-access tests break | High (if struct fields move first) | High | Epic 3 Story 3.1 adds injection constructor BEFORE any field relocation in Epic 4 |
| R3.6 cross-component event sharing unverified | Medium | Medium | Task 3.3.2 adds `TestClaudeController_SharedDetectorEvents` confirming shared RecentEvents |
| Double-normalization in `detectFromLines` | Medium | Low-Medium | Task 4.1.2 routes `detectFromLines` through pattern-match step directly, not `DetectWithContext` |
| `atomic.Pointer[TerminalDetector]` invalid Go | Certain if used | Medium | Keep `atomic.Pointer[StatusDetector]` internally; expose only `TerminalDetector` interface via accessor |
| Circular import: `binaries` ↔ `detection` | High if done wrong | High | Import direction is `detection` → `detection/binaries` only; `binaries` imports only `detection.StatusPatterns` and `detection.BinaryDetector` |
| Pattern union duplicates changing priority | Low-Medium | Low | Task 4.2.3 adds `TestDefaultPatterns_NoDuplicateNames` + duplicate-detection assertion in merge logic |

---

## Acceptance Criteria

- `make build` passes (all 4 epics)
- `make test` passes (all 4 epics)
- `make lint` passes (all 4 epics)
- `TerminalDetector` interface compile-time check present (`var _ TerminalDetector = (*StatusDetector)(nil)`)
- `BinaryDetector` registry has exactly 5 registered binaries (claude, gemini, aider, opencode, agy)
- `DefaultRegistry()` unit test passes with 5-entry assertion
- `IdleDetector` and `ClaudeController` share one `StatusDetector` instance per session (verified by test showing shared RecentEvents)
- `StatusDetector` delegates to `PTYNormalizer`, `PatternSet`, `DetectionEventSink` (structural, confirmed by code review)
- `EventRingCap` is 2000
- No behavioral change: sessions detect the same statuses as before (confirmed by full test suite)
- `LoadPatterns` is safe for concurrent use with `Detect` calls (PatternSet `RWMutex`)
- Existing test suite passes without modification (except new tests added)
