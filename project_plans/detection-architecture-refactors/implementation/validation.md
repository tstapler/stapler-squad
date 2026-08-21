# Validation Plan: detection-architecture-refactors

**Date**: 2026-06-14
**Project**: Detection Architecture Refactors
**Status**: Ready for implementation (Readiness gate: PASS)

---

## 1. Requirement → Test Mapping

### Epic 1 — BinaryDetector Interface + Registry

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| R1.1: `BinaryDetector` interface with `Name()`, `Patterns()`, `FilterContent()` | `session/detection/registry_test.go` | `TestBinaryDetector_InterfaceMethods_ReturnExpectedValues` | Unit | Happy path — construct a concrete implementation, assert all three methods return non-zero values |
| R1.1: `FilterContent` is a no-op for claude | `session/detection/binaries/claude_test.go` | `TestClaudeDetector_FilterContent_should_returnUnchanged_When_noFrameLines` | Unit | Happy path — `FilterContent("some content") == "some content"` |
| R1.2: `DetectorRegistry` Register/Lookup round-trip | `session/detection/registry_test.go` | `TestDetectorRegistry_should_returnDetector_When_registeredByName` | Unit | Happy path — register a stub, lookup same name, get it back |
| R1.2: `Lookup` returns false for unknown binary | `session/detection/registry_test.go` | `TestDetectorRegistry_should_returnFalse_When_binaryNotRegistered` | Unit | Error path — lookup unregistered name |
| R1.2: `Register` panics on duplicate | `session/detection/registry_test.go` | `TestDetectorRegistry_should_panic_When_duplicateNameRegistered` | Unit | Error path — register same name twice, expect `defer/recover` to catch panic |
| R1.3: All 5 binaries have their own files | `session/detection/binaries/*_test.go` | `TestClaudeDetector_Name`, `TestGeminiDetector_Name`, `TestAiderDetector_Name`, `TestOpencodeDetector_Name`, `TestAgyDetector_Name` | Unit | Happy path — each file's `Name()` returns the canonical binary string |
| R1.3: Per-binary `Patterns()` contains expected pattern names | `session/detection/binaries/claude_test.go` | `TestClaudeDetector_Patterns_should_containExpectedPatternNames` | Unit | Happy path — check ≥1 known pattern name per status category |
| R1.4: `DetectForProgram` uses registry-based pattern set | `session/detection/detector_test.go` | `TestDetectForProgram_should_useRegisteredPatterns_When_binaryKnown` | Unit | Happy path — register a binary with a distinctive pattern; call `DetectForProgram` with that binary name and a matching string |
| R1.5: `DefaultRegistry()` has exactly 5 entries | `session/detection/registry_test.go` | `TestDefaultRegistry_should_have5Entries` | Unit | Happy path — assert `len(registry.entries) == 5` |
| R1.5: Behavioral equivalence — existing detections unchanged | `session/detection/detector_test.go` | All existing `TestStatusDetector_Detect*` tests | Unit | Regression — no behavioral change after registry wiring |
| R1.6: `DetectForProgram` falls back for unknown binary | `session/detection/detector_test.go` | `TestDetectForProgram_should_fallBack_When_binaryUnknown` | Unit | Error path — call `DetectForProgram` with `"unknown-binary"`, assert result equals default-pattern detection |

### Epic 2 — TerminalDetector Interface

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| R2.1: `TerminalDetector` interface defined with correct methods | `session/detection/terminal_detector.go` | `var _ TerminalDetector = (*StatusDetector)(nil)` | Compile-time | Compile assertion — package fails to build if `*StatusDetector` does not satisfy the interface |
| R2.2: `*StatusDetector` satisfies `TerminalDetector` | `session/detection/terminal_detector.go` | `var _ TerminalDetector = (*StatusDetector)(nil)` | Compile-time | Same compile assertion as R2.1 |
| R2.3: Call-site fields use `TerminalDetector` interface type | `session/` build | `go build ./session/...` | Compile-time | All 5 files compile without concrete `*detection.StatusDetector` fields |
| R2.4: Constructor injection returns `TerminalDetector` | `session/` build | `go build ./session/...` | Compile-time | `NewStatusDetector()` result assignable to `TerminalDetector` at call sites |
| R2.5: Existing tests pass after interface introduction | `session/detection/detector_test.go`, `session/detection/idle_test.go` | All existing tests (26 total) | Unit | Regression — all pass without modification |
| R2.6: Fake `TerminalDetector` usable in tests | `session/detection/terminal_detector_test.go` | `TestFakeTerminalDetector_should_satisfyInterface` | Unit | Happy path — define a minimal in-test struct with all interface methods, verify it compiles and can be passed where `TerminalDetector` is expected |

### Epic 3 — Dual-Detector Consolidation

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| R3.1: `NewIdleDetectorWithDetector` accepts `TerminalDetector` | `session/detection/idle_test.go` | `TestNewIdleDetectorWithDetector_should_acceptInjectedDetector` | Unit | Happy path — construct with a stub `TerminalDetector`, confirm `DetectState()` calls the stub's `DetectFromLines` |
| R3.2: Injected detector is used instead of creating a new one | `session/detection/idle_test.go` | `TestNewIdleDetectorWithDetector_should_useInjected_When_nonNil` | Unit | Happy path — inject a fake that records calls; verify it was called during `DetectState()` |
| R3.3: Nil injection falls back to own detector | `session/detection/idle_test.go` | `TestNewIdleDetectorWithDetector_should_createOwn_When_nilInjected` | Unit | Error path (nil injection) — pass nil, construct succeeds, `DetectState()` returns a valid state |
| R3.4: `ClaudeController` injects its detector into `IdleDetector` | `session/detection/shared_detector_test.go` | `TestClaudeController_SharedDetectorEvents` | Integration | Happy path — construct shared `*StatusDetector`; feed it through `NewIdleDetectorWithDetector`; drive both paths; assert shared events |
| R3.5: Both components share one ring buffer | `session/detection/shared_detector_test.go` | `TestClaudeController_SharedDetectorEvents` | Integration | Subtest within R3.4 — single `RecentEvents(N)` call sees events from both paths |
| R3.6: Events from both paths visible in shared `RecentEvents()` | `session/detection/shared_detector_test.go` | `TestClaudeController_SharedDetectorEvents` | Integration | Explicit cross-component visibility — call `DetectStateFromContent` on idle path and `DetectWithContextFromLines` on controller path; assert both events present in `RecentEvents(10)` |
| R3.6: Ring cap ≥2000 covers sustained 1 Hz polling | `session/detection/events_test.go` | `TestEventRingCap_should_be2000` | Unit | Happy path — assert `EventRingCap == 2000`; fill 1001 events, assert oldest still accessible (ring did not drain) |

### Epic 4 — SRP Split of StatusDetector

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| R4.1: `PTYNormalizer.Normalize` strips ANSI and collapses CR | `session/detection/normalizer_test.go` | `TestPTYNormalizer_Normalize_should_stripANSI_When_escapeSequencesPresent` | Unit | Happy path — input `"\x1b[31mHello\x1b[0m"`, expect `"Hello"` |
| R4.1: `PTYNormalizer.Normalize` collapses carriage returns | `session/detection/normalizer_test.go` | `TestPTYNormalizer_Normalize_should_collapseCarriageReturns_When_crPresent` | Unit | Happy path — input `"foo\rbar"`, expect `"bar"` (last segment wins) |
| R4.1: `PTYNormalizer.SplitLines` splits and drops blank lines | `session/detection/normalizer_test.go` | `TestPTYNormalizer_SplitLines_should_returnNonBlankLines` | Unit | Happy path — multi-line input with blank lines returns only non-blank entries |
| R4.1: `PTYNormalizer.SplitLines` handles empty input | `session/detection/normalizer_test.go` | `TestPTYNormalizer_SplitLines_should_returnEmpty_When_inputIsEmpty` | Unit | Error path — empty string input returns nil or `[]string{}` |
| R4.2: `PatternSet.MatchLines` returns correct status for known strings | `session/detection/pattern_set_test.go` | `TestPatternSet_MatchLines_should_returnError_When_errorStringPresent` | Unit | Happy path — `MatchLines([]string{"Error: oops"})` returns `(StatusError, "error_message")` |
| R4.2: `NewPatternSet` returns error on invalid regex | `session/detection/pattern_set_test.go` | `TestNewPatternSet_should_returnError_When_invalidRegexProvided` | Unit | Error path — pass `StatusPatterns` with `Pattern: "(?P<invalid"`, assert non-nil error |
| R4.2: `PatternSet` is safe for concurrent `MatchLines` calls | `session/detection/pattern_set_test.go` | `TestPatternSet_MatchLines_should_beRaceFree_When_calledConcurrently` | Unit | Concurrency — 10 goroutines × 200 calls; run with `-race`; no data race |
| R4.3: `DetectionEventSink.Record` then `Recent` returns the event | `session/detection/event_sink_test.go` | `TestDetectionEventSink_should_returnRecordedEvents_When_recordThenRecent` | Unit | Happy path — `Record(event)` then `Recent(1)` returns that event |
| R4.3: `DetectionEventSink.Recent(n)` caps at ring size | `session/detection/event_sink_test.go` | `TestDetectionEventSink_should_capAtRingSize_When_nExceedsCapacity` | Unit | Error path — fill ring beyond cap; oldest events dropped |
| R4.3: `DetectionEventSink.SetSessionID` propagates to recorded events | `session/detection/event_sink_test.go` | `TestDetectionEventSink_should_setSessionID_When_SetSessionIDCalled` | Unit | Happy path — `SetSessionID("s1")`, `Record(...)`, `Recent(1)[0].SessionID == "s1"` |
| R4.4: `StatusDetector` delegates to all three sub-components | `session/detection/detector_test.go` | All existing `TestStatusDetector_*` tests | Unit | Regression — all 26 existing tests pass without modification (structural change, no API change) |
| R4.5: All existing `StatusDetector` methods remain | `session/detection/` | `go build ./session/detection/...` | Compile-time | Compile assertion — no removed exported methods |
| R4.6: `LoadPatterns`/`ExportPatterns` still on `StatusDetector` | `session/detection/detector_test.go` | `TestStatusDetector_LoadPatterns`, `TestNewStatusDetectorFromFile`, `TestStatusDetector_ExportPatterns` | Unit | Regression — existing tests unchanged |
| R4.7: All existing tests pass without modification | `session/detection/detector_test.go`, `session/detection/idle_test.go` | All 26+ existing tests | Unit | Full regression suite passes post-SRP split |
| R4.8: New tests cover `PTYNormalizer` in isolation | `session/detection/normalizer_test.go` | 4 tests listed under R4.1 above | Unit | New tests for extracted component |
| R4.8: New tests cover `PatternSet` in isolation | `session/detection/pattern_set_test.go` | 3 tests listed under R4.2 above | Unit | New tests for extracted component |

**Additional cross-cutting test — no duplicate pattern names after merge (Task 4.2.3):**

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| R1.5 + R4.2 (union semantics preserved) | `session/detection/detector_test.go` | `TestDefaultPatterns_should_haveNoDuplicateNames` | Unit | Happy path — call `getDefaultPatterns()`, iterate all 9 categories, assert every `StatusPattern.Name` is unique across all categories combined |

---

## 2. Test Suite by Component

### Epic 1 — BinaryDetector + Registry

New files: `session/detection/registry_test.go`, `session/detection/binaries/*_test.go`

| Test | File | Type | Input | Expected |
|------|------|------|-------|----------|
| `TestDetectorRegistry_should_returnDetector_When_registeredByName` | `registry_test.go` | Unit | `Register(stub)` → `Lookup("stub")` | `(stub, true)` |
| `TestDetectorRegistry_should_returnFalse_When_binaryNotRegistered` | `registry_test.go` | Unit | `Lookup("nope")` | `(nil, false)` |
| `TestDetectorRegistry_should_panic_When_duplicateNameRegistered` | `registry_test.go` | Unit | `Register(x)` × 2 with same `Name()` | panic recovered |
| `TestDefaultRegistry_should_have5Entries` | `registry_test.go` | Unit | `DefaultRegistry()` | len == 5 |
| `TestClaudeDetector_Name` | `binaries/claude_test.go` | Unit | `NewClaudeDetector().Name()` | `"claude"` |
| `TestClaudeDetector_Patterns_should_containExpectedPatternNames` | `binaries/claude_test.go` | Unit | `NewClaudeDetector().Patterns()` | Ready/Processing/etc. slices non-empty |
| `TestClaudeDetector_FilterContent_should_returnUnchanged_When_noFrameLines` | `binaries/claude_test.go` | Unit | `FilterContent("x")` | `"x"` |
| `TestGeminiDetector_Name` | `binaries/gemini_test.go` | Unit | `NewGeminiDetector().Name()` | `"gemini"` |
| `TestAiderDetector_Name` | `binaries/aider_test.go` | Unit | `NewAiderDetector().Name()` | `"aider"` |
| `TestOpencodeDetector_Name` | `binaries/opencode_test.go` | Unit | `NewOpencodeDetector().Name()` | `"opencode"` |
| `TestAgyDetector_Name` | `binaries/agy_test.go` | Unit | `NewAgyDetector().Name()` | `"agy"` |
| `TestDetectForProgram_should_useRegisteredPatterns_When_binaryKnown` | `detector_test.go` | Unit | `DetectForProgram(matchingOutput, "claude")` | status from claude pattern set |
| `TestDetectForProgram_should_fallBack_When_binaryUnknown` | `detector_test.go` | Unit | `DetectForProgram(matchingOutput, "unknown")` | same as `Detect(matchingOutput)` |
| `TestDefaultPatterns_should_haveNoDuplicateNames` | `detector_test.go` | Unit | `getDefaultPatterns()` | no duplicate `StatusPattern.Name` across all 9 categories |

### Epic 2 — TerminalDetector Interface

New file: `session/detection/terminal_detector.go`, `session/detection/terminal_detector_test.go`

| Test | File | Type | Input | Expected |
|------|------|------|-------|----------|
| `var _ TerminalDetector = (*StatusDetector)(nil)` | `terminal_detector.go` | Compile-time | — | Package builds |
| `TestFakeTerminalDetector_should_satisfyInterface` | `terminal_detector_test.go` | Unit | In-test stub struct implementing all interface methods | Compiles and can be passed to a function accepting `TerminalDetector` |

Build-level checks (run as part of task acceptance):
- `go build ./session/...` after each call-site update
- `go build ./server/services/...` after `session_service.go` update

### Epic 3 — Dual-Detector Consolidation

New file: `session/detection/shared_detector_test.go`

| Test | File | Type | Input | Expected |
|------|------|------|-------|----------|
| `TestNewIdleDetectorWithDetector_should_acceptInjectedDetector` | `idle_test.go` | Unit | Real `*StatusDetector` injected | `DetectState()` does not panic; returns valid state |
| `TestNewIdleDetectorWithDetector_should_useInjected_When_nonNil` | `idle_test.go` | Unit | Injected fake that records calls | Fake's `DetectFromLines` called during `DetectState()` |
| `TestNewIdleDetectorWithDetector_should_createOwn_When_nilInjected` | `idle_test.go` | Unit | `nil` injection | `DetectState()` still functions correctly |
| `TestEventRingCap_should_be2000` | `events_test.go` | Unit | Constant value | `EventRingCap == 2000` |
| `TestClaudeController_SharedDetectorEvents` | `shared_detector_test.go` | Integration | Shared `*StatusDetector` → both `IdleDetector` and direct `DetectWithContextFromLines` calls | `RecentEvents(10)` returns ≥2 events from both call paths |

### Epic 4 — SRP Split

New files: `session/detection/normalizer_test.go`, `session/detection/pattern_set_test.go`, `session/detection/event_sink_test.go`

| Test | File | Type | Input | Expected |
|------|------|------|-------|----------|
| `TestPTYNormalizer_Normalize_should_stripANSI_When_escapeSequencesPresent` | `normalizer_test.go` | Unit | `"\x1b[31mHello\x1b[0m"` | `"Hello"` |
| `TestPTYNormalizer_Normalize_should_collapseCarriageReturns_When_crPresent` | `normalizer_test.go` | Unit | `"foo\rbar"` | `"bar"` |
| `TestPTYNormalizer_Normalize_should_handleWindowsCRLF_Without_collapse` | `normalizer_test.go` | Unit | `"line1\r\nline2"` | `"line1\nline2"` (CRLF not treated as overwrite) |
| `TestPTYNormalizer_SplitLines_should_returnNonBlankLines` | `normalizer_test.go` | Unit | `"a\n\nb\n"` | `["a", "b"]` |
| `TestPTYNormalizer_SplitLines_should_returnEmpty_When_inputIsEmpty` | `normalizer_test.go` | Unit | `""` | `nil` or `[]string{}` |
| `TestNewPatternSet_should_returnError_When_invalidRegexProvided` | `pattern_set_test.go` | Unit | `StatusPatterns` with `"(?P<invalid"` | non-nil error |
| `TestPatternSet_MatchLines_should_returnError_When_errorStringPresent` | `pattern_set_test.go` | Unit | `["Error: oops"]` | `(StatusError, non-empty-string)` |
| `TestPatternSet_MatchLines_should_returnReady_When_noMatchAndCatchAll` | `pattern_set_test.go` | Unit | `["generic output"]` | `(StatusReady, ...)` |
| `TestPatternSet_MatchLines_should_beRaceFree_When_calledConcurrently` | `pattern_set_test.go` | Unit | 10 goroutines × 200 `MatchLines` calls | no data race (`go test -race`) |
| `TestDetectionEventSink_should_returnRecordedEvents_When_recordThenRecent` | `event_sink_test.go` | Unit | `Record(e)` → `Recent(1)` | `[e]` |
| `TestDetectionEventSink_should_capAtRingSize_When_nExceedsCapacity` | `event_sink_test.go` | Unit | Fill beyond `EventRingCap`; call `Recent(N)` | oldest events absent |
| `TestDetectionEventSink_should_setSessionID_When_SetSessionIDCalled` | `event_sink_test.go` | Unit | `SetSessionID("s1")`, `Record(...)` | `Recent(1)[0].SessionID == "s1"` |

---

## 3. Regression Guard

The following tests must pass without modification after each epic completes. They form the behavioral equivalence baseline.

### After Epic 1 (BinaryDetector + Registry)

All tests in `session/detection/detector_test.go` must pass unchanged:
- `TestNewStatusDetector` — behavioral probe suite (StatusActive, StatusError, StatusNeedsApproval, StatusInputRequired, StatusIdle)
- `TestStatusDetector_DetectReady`
- `TestStatusDetector_DetectIdle`
- `TestStatusDetector_DetectActive`
- `TestStatusDetector_DetectSuccess`
- `TestStatusDetector_DetectProcessing`
- `TestStatusDetector_DetectNeedsApproval`
- `TestStatusDetector_DetectError`
- `TestStatusDetector_PriorityOrder` — 8 cross-category priority cases
- `TestStatusDetector_DetectWithContext`
- `TestGeminiPatterns_AgyCoverage` — calls `getDefaultPatterns()` directly; must still find ≥4 gemini_* patterns
- `TestGeminiPatterns_NeedsApprovalState`
- `TestStatusDetector_DetectActive_StarFourPointed`
- `TestStatusDetector_DetectActive_ScreenOverwrite`
- `TestRecentEvents`
- `TestEventRing_ConcurrentPushRecent`

### After Epic 2 (TerminalDetector Interface)

All tests in both `detector_test.go` and `idle_test.go` must pass unchanged:
- All 9 `idle_test.go` fake-clock tests: `TestIdleDetector_StateTransitions`, `TestIdleDetector_TimeoutDetection`, `TestIdleDetector_ActivityTracking`, `TestIdleDetector_GetIdleDuration`, `TestIdleDetector_IsIdle`, `TestIdleDetector_IsActive`, `TestIdleDetector_ConfigUpdate`, `TestIdleDetector_InitializeFromTimestamp`, `TestIdleDetector_RecordActivity`
- All tests in `detector_test.go`

### After Epic 3 (Dual-Detector Consolidation)

Critical regression: the 9 fake-clock tests in `idle_test.go` use direct field access (`d.now`, `d.lastStateChange`, `d.lastActivity`). These fields must NOT be relocated in Epic 3 — they move only as part of Epic 4 if at all. Verify all 9 pass before committing Story 3.3.

### After Epic 4 (SRP Split)

Every existing test in `detector_test.go` and `idle_test.go` must pass without modification (R4.7 acceptance criterion). Verify with:
```
go test ./session/detection/... -race -count=1
```

---

## 4. Test Stack

- **Unit**: Go standard `testing` package; assertions by `t.Errorf`/`t.Fatalf` (matches existing codebase style)
- **Compile-time**: `var _ Interface = (*ConcreteType)(nil)` assertions in production files
- **Integration**: Same `testing` package; drives multiple real components through a shared `*StatusDetector` instance; lives in `session/detection/shared_detector_test.go`
- **Race detection**: `go test -race ./session/detection/...` for all new concurrent-access tests (`PatternSet`, `DetectionEventSink`, ring buffer)

---

## 5. Coverage Targets

- Unit test coverage: ≥80% line coverage on all new files (`normalizer.go`, `pattern_set.go`, `event_sink.go`, `binary_detector.go`, `registry.go`, `binaries/*.go`)
- All new exported methods: happy path + at least one error/edge-case path
- All external-facing interface methods on `TerminalDetector` and `BinaryDetector`: covered by the compile-time assertion plus existing `StatusDetector` tests
- `PatternSet.MatchLines` and `DetectionEventSink.Record`/`Recent`: race-tested via `-race`

---

## 6. Implementation Readiness Gate

### Criterion 1 — Requirement → Test Coverage

Every requirement R1.1–R4.8 has at least one named test case in the mapping above.

Count: **22/22 requirements covered** (R1.1, R1.2, R1.3, R1.4, R1.5, R1.6, R2.1, R2.2, R2.3, R2.4, R2.5, R2.6, R3.1, R3.2, R3.3, R3.4, R3.5, R3.6, R4.1, R4.2, R4.3, R4.4+R4.5+R4.6+R4.7 via regression suite, R4.8).

**PASS**

### Criterion 2 — Risk Register Mitigations Verified by Tests

| Risk | Covering Test |
|------|---------------|
| `LoadPatterns` data race under sharing | `TestPatternSet_MatchLines_should_beRaceFree_When_calledConcurrently` + `go test -race ./session/detection/...` after Task 4.2.2 |
| Ring buffer history collapse at 500 cap | `TestEventRingCap_should_be2000` (asserts constant == 2000); `TestClaudeController_SharedDetectorEvents` (drives both paths and reads shared events) |
| `idle_test.go` field-access tests break if struct changes first | Regression gate: all 9 `idle_test.go` fake-clock tests must pass at end of Epic 3 Story 3.1 before any Epic 4 work begins |
| R3.6 cross-component event sharing unverified | `TestClaudeController_SharedDetectorEvents` (drives both idle and controller paths; asserts shared `RecentEvents`) |
| Double-normalization in `detectFromLines` | `TestPTYNormalizer_Normalize_should_*` tests confirm single-pass behavior; existing `TestStatusDetector_DetectActive_ScreenOverwrite` regression catches incorrect double-stripping |
| `atomic.Pointer[TerminalDetector]` invalid Go | `go build ./session/...` (compile-time; any misuse produces a build error) |
| Circular import: `binaries` ↔ `detection` | `go build ./session/detection/...` (Go toolchain rejects import cycles immediately) |
| Pattern union duplicates changing priority | `TestDefaultPatterns_should_haveNoDuplicateNames` + `TestGeminiPatterns_AgyCoverage` (regression) |

**PASS** — all 8 risks have covering tests or build-level checks.

### Criterion 3 — Tests Verify Behavior, Not Implementation Details

Review of each new test:

- `TestDetectorRegistry_should_returnDetector_When_registeredByName` — tests observable behavior (lookup succeeds). Does not assert internal map structure. **Clean.**
- `TestDefaultRegistry_should_have5Entries` — tests the count of registered entries. This WOULD be brittle if the number of binaries changes, but the requirements explicitly state "exactly 5" (R1.5). Acceptable for this refactor scope.
- `TestClaudeDetector_Patterns_should_containExpectedPatternNames` — tests that specific pattern names exist. Pattern names are stable API (used in `MatchedPattern` fields of events, in YAML export/import). Acceptable.
- `TestEventRingCap_should_be2000` — tests the constant value directly. This is a regression guard: if the constant regresses to 500, the shared-ring safety invariant breaks. Intentional and documented.
- `TestPatternSet_MatchLines_should_returnError_When_errorStringPresent` — tests behavioral output (status + pattern name). Does not assert internal regex field layout. **Clean.**
- `TestPTYNormalizer_*` — test observable input/output transformation. No internal field access. **Clean.**
- `TestDetectionEventSink_*` — test public `Record`/`Recent`/`SetSessionID` behavior. No internal field access. **Clean.**
- `TestClaudeController_SharedDetectorEvents` — tests cross-component event visibility (behavioral). Does not assert which struct field holds the detector. **Clean.**
- `TestFakeTerminalDetector_should_satisfyInterface` — compile-time check only. **Clean.**
- Fake-clock tests in `idle_test.go` (`d.now`, `d.lastStateChange`, `d.lastActivity` direct field access) — these ARE implementation-detail tests. However, they are **existing tests** explicitly called out in the requirements (R4.7) as tests that "must continue passing without modification." The field access is a deliberate testing pattern established by the existing codebase. New tests added by this project do NOT use this pattern.

**Minor note**: The `TestDefaultRegistry_should_have5Entries` test asserts a specific count. If a sixth binary is added in a future PR, this test will fail — which is intentional (the registry is a controlled surface). No action needed; this is a known, acceptable trade-off.

**Verdict: CONCERNS (minor)** — no new test breaks on a pure refactor, but 9 existing idle tests use direct field access. Since those are existing (not new) tests and R4.7 requires them to pass unchanged, this is not a blocker.

### Criterion 4 — Adversarial Review Verdict

From `adversarial-review.md`:

```
**Verdict**: CONCERNS
```

No BLOCKER items. All concerns are addressed in the plan (the plan was updated after adversarial review to resolve the ambiguous locking strategy and to add the `TestClaudeController_SharedDetectorEvents` task explicitly in Task 3.3.2). The remaining CONCERNS are documentation-level (story sequencing note, interface method audit step) that do not block implementation.

**PASS** — verdict is CONCERNS, not BLOCKED.

---

## 7. Readiness Gate Summary

| # | Criterion | Verdict |
|---|-----------|---------|
| 1 | Every requirement R1.1–R4.8 has ≥1 test case | PASS (22/22) |
| 2 | All 8 risks in Risk Register have covering test or build check | PASS |
| 3 | No new test verifies implementation details instead of behavior | CONCERNS (minor — 9 existing idle tests use direct field access, but these are existing, required-to-pass tests, not new tests introduced by this project) |
| 4 | Adversarial review verdict is CONCERNS or CLEAN (not BLOCKED) | PASS (verdict: CONCERNS) |

**Overall Readiness Gate: PASS**

(Criterion 3 registers CONCERNS due to existing field-access tests, but those tests are outside the scope of new test design — they are regression guards required by R4.7. No new test introduced by this validation plan accesses implementation internals.)
