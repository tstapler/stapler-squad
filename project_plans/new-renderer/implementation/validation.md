# Validation Plan: new-renderer

Date: 2026-06-24

---

## Requirement → Test Mapping

| REQ-ID | Requirement | Story | Test Name | Type | Path |
|--------|-------------|-------|-----------|------|------|
| REQ-1 | Terminal renders correctly in browser (no `U+FFFD` corruption) | 1.1.1 | `StateApplicator_should_emitCleanText_When_multiByteSplitAcrossTwoMessages` | Unit | Happy path |
| REQ-1 | Terminal renders correctly in browser (no `U+FFFD` corruption) | 1.1.1 | `StateApplicator_should_emitReplacementChar_When_streamModeDisabled` | Unit | Error path (negative: confirms the bug exists without `{stream:true}`) |
| REQ-1 | Terminal renders correctly in browser (no `U+FFFD` corruption) | 1.1.2 | `scrollbackDecoder_should_emitCleanText_When_multiByteSplitAcrossScrollbackChunks` | Unit | Happy path |
| REQ-1 | Terminal renders correctly in browser (no `U+FFFD` corruption) | 1.1.2 | `scrollbackDecoder_should_notCorruptLiveStream_When_scrollbackChunkDecodedWithoutStream` | Unit | Error/edge path |
| REQ-2 | Escape sequences are not truncated at chunk boundaries | 1.2.1 | `findPartialEscapeAtEnd_should_bufferOSC_When_escapeByteLandsBeyond20CharsFromEnd` | Unit | Happy path |
| REQ-2 | Escape sequences are not truncated at chunk boundaries | 1.2.1 | `findPartialEscapeAtEnd_should_bufferDCS_When_splitAfterIntroducer` | Unit | Error/edge path |
| REQ-2 | Escape sequences are not truncated at chunk boundaries | 1.2.2 | `parse_should_passThrough_When_ED2andED3Combined` | Unit | Happy path |
| REQ-2 | Escape sequences are not truncated at chunk boundaries | 1.2.2 | `parse_should_passThrough_When_onlyED3Present` | Unit | Edge/regression path |
| REQ-2 | Automated CI regression gate for ED2+ED3 passthrough | 1.2.2 | `EscapeSequenceParser_should_passThrough_ED3_When_pairedWithED2` | Unit (CI gate) | `web-app/src/lib/terminal/__tests__/EscapeSequenceParser.test.ts` |
| REQ-3 | Intermediate frames not silently dropped | 1.3.1 | `RedrawThrottler_should_notThrottle_When_chunkStartsWithCursorUpOnly` | Unit | Happy path |
| REQ-3 | Intermediate frames not silently dropped | 1.3.1 | `RedrawThrottler_should_throttle_When_chunkStartsWithCursorUpFollowedByErase` | Unit | Error/edge path |
| REQ-4 | Analytics pipeline active and observable | 2.1.1 | `TestStreamViaTmuxCapturePane_Stage2TapCalled_WhenEscapeParserEnabled` | Integration | Happy path |
| REQ-4 | Analytics pipeline active and observable | 2.1.1 | `TestStreamViaTmuxCapturePane_Stage2TapSkipped_WhenEscapeParserNil` | Unit | Error/edge path |
| REQ-4 | Analytics pipeline active and observable | 2.1.2 | `TestEscapeCodeStore_RecordNoOp_WhenDisabled` | Unit | Error/edge path |
| REQ-4 | Analytics pipeline active and observable | 2.1.2 | `TestEscapeCodeStore_RecordsEntry_WhenEnabled` | Unit | Happy path |
| REQ-4 | Analytics pipeline active and observable | 2.1.2 | `TestConfig_DebugEscapeAnalyticsTrue_WhenEnvVarSet` | Integration | Happy path |
| REQ-5 | Browser smoke test: no scrollback bleed, no flicker | 1.2.2 | Manual smoke test — run Claude Code session post-merge | Browser | Required before merge |
| REQ-1,2,3 | All Phase 1 fixes work correctly together (cross-fix interactions) | 1.5.1 | `PipelineIntegration_should_passAllFourScenarios_When_PhaseOneFixes` | Integration | `web-app/src/lib/terminal/__tests__/pipeline-integration.test.ts` |

---

## Phase 1 Critical — Detailed Test Specifications

### Story 1.1.1: StateApplicator `{stream: true}` (TypeScript/Jest)

**File**: `web-app/src/__tests__/StateApplicator.test.ts`

#### Happy path — multi-byte reassembled correctly

```
StateApplicator_should_emitCleanText_When_multiByteSplitAcrossTwoMessages
```

Setup:
- Construct `StateApplicator` with a mock `ITerminalAdapter` that records `.write()` calls.
- Build two synthetic `TerminalDiff` proto messages:
  - `diff1.diffBytes` = `\xc3` (first byte of UTF-8 "é")
  - `diff2.diffBytes` = `\xa9` (second byte of UTF-8 "é")
- Call `applyDiffImmediate(diff1)` then `applyDiffImmediate(diff2)`.

Assert:
- No write call contains `�` (U+FFFD replacement character).
- The combined write output includes `"é"`.

#### Error/regression path — confirms bug without fix

```
StateApplicator_should_emitReplacementChar_When_streamModeDisabled
```

Setup:
- Same as above, but patch the internal decoder to call `.decode()` WITHOUT `{ stream: true }` (simulate the unfixed code).

Assert:
- At least one write call contains `�`.

This test documents the pre-fix behavior and will pass only if the regression check is disabled; it serves as a documented negative test. Annotate with `// @regression-doc: remove if negative test pattern is not desired`.

#### Integration (reset path)

```
StateApplicator_should_notLeakDecoderState_When_resetCalledBetweenSessions
```

Setup:
- Apply a split multi-byte sequence (`\xc3` via `applyDiffImmediate`), leaving decoder in mid-sequence state.
- Call `resetSequence()` (which must internally call `reset()` per the plan).
- Apply `\xc3\xa9` (complete "é") in a single new diff.

Assert:
- No `�` in second session's output.
- First incomplete sequence is fully discarded — no partial byte leaks into second session.

---

### Story 1.1.2: `useTerminalStream` scrollback decoder (TypeScript/Jest)

**File**: `web-app/src/__tests__/useTerminalStream.test.ts`

#### Happy path — scrollback multi-byte split across chunks

```
scrollbackDecoder_should_emitCleanText_When_multiByteSplitAcrossScrollbackChunks
```

Setup:
- Render the hook with a mock terminal and mock RPC stream.
- Send scrollback chunk 1: `\xc3` (first byte of "é").
- Send scrollback chunk 2: `\xa9` (second byte of "é").

Assert:
- `term.write` or equivalent receives `"é"` with no `�`.

#### Error/edge path — live stream not corrupted by scrollback decoder

```
scrollbackDecoder_should_notCorruptLiveStream_When_scrollbackChunkDecodedWithoutStream
```

Setup:
- Render the hook.
- Send a live-stream chunk that leaves a partial multi-byte sequence in the live decoder.
- Immediately send a scrollback/pane-response chunk (line 298 path).
- Send the completing byte for the partial sequence on the live path.

Assert:
- The live-stream output does NOT contain `�`.
- The scrollback output is clean.

This tests that `scrollbackDecoderRef` is separate from the live `textDecoderRef`.

#### Integration — reset on disconnect

```
scrollbackDecoder_should_resetDecoder_When_disconnected
```

Setup:
- Feed a partial multi-byte sequence into the scrollback decoder.
- Trigger a disconnect (simulate the relevant cleanup path).
- Reconnect and send a complete "é" sequence via scrollback.

Assert:
- No `�` after reconnect.

---

### Story 1.2.1: `EscapeSequenceParser` lookback 20→256 + DCS/PM/APC (TypeScript/Jest)

**File**: `web-app/src/__tests__/EscapeSequenceParser.test.ts`

#### Happy path — OSC with title beyond 20-char window is buffered

```
findPartialEscapeAtEnd_should_bufferOSC_When_escapeByteLandsBeyond20CharsFromEnd
```

Setup:
- Build a chunk string: 25 bytes of ASCII followed by `\x1b]0;some title` (no BEL or ST terminator).
- The `\x1b` is 14 bytes from the end — beyond the old 20-char window start at char 25, within the new 256-char window.

Assert:
- `findPartialEscapeAtEnd` returns the partial escape starting at `\x1b`.
- The returned partial is not empty.

#### Error/edge path — DCS sequence buffered when split after introducer

```
findPartialEscapeAtEnd_should_bufferDCS_When_splitAfterIntroducer
```

Setup:
- Build a chunk: `\x1bP` followed by 30 bytes of ASCII payload (no ST terminator `\x1b\\`).

Assert:
- `findPartialEscapeAtEnd` identifies the `\x1bP` as an incomplete DCS sequence.
- `isCompleteEscapeSequence` returns `false` for the fragment.

#### Integration — PM and APC variants also buffered

```
isCompleteEscapeSequence_should_returnFalse_When_PMorAPCLacksStringTerminator
```

Setup:
- PM fragment: `\x1b^partial payload` (no `\x1b\\`).
- APC fragment: `\x1b_partial payload` (no `\x1b\\`).

Assert:
- `isCompleteEscapeSequence` returns `false` for both.
- After providing ST (`\x1b\\`), returns `true`.

---

### Story 1.2.2: Remove ED2+ED3 stripping (TypeScript/Jest + browser)

**File**: `web-app/src/__tests__/EscapeSequenceParser.test.ts`

#### Happy path — combined ED2+ED3 passes through unchanged

```
parse_should_passThrough_When_ED2andED3Combined
```

Setup:
- Input: `"\x1b[2J\x1b[3J"` (erase screen + erase scrollback).

Assert:
- Parsed/filtered output equals `"\x1b[2J\x1b[3J"` — no modification.
- Does NOT equal the old `"\x1b[2J"` (regression guard).

#### Edge/regression path — ED3 alone passes through

```
parse_should_passThrough_When_onlyED3Present
```

Setup:
- Input: `"\x1b[3J"` (erase scrollback alone).

Assert:
- Output equals `"\x1b[3J"` — passes through unchanged.

#### Browser validation (manual, required before merge)

- Start the stapler-squad server with the change.
- Open a Claude Code session.
- Confirm that scrollback from a previous session does NOT bleed through after a new session's reset.
- Confirm NO visible flicker on alternate-screen transitions (e.g., `vim` open/close).
- Record verdict in PR description: "Browser smoke test: PASS — no bleed, no flicker on [date]".

---

### Story 1.3.1: `RedrawThrottler` cursor-up narrowing (TypeScript/Jest)

**File**: `web-app/src/__tests__/TerminalStreamManager.test.ts`

#### Happy path — partial cursor-up (incremental update) is NOT throttled

```
RedrawThrottler_should_notThrottle_When_chunkStartsWithCursorUpOnly
```

Setup:
- Create `TerminalStreamManager` with mock terminal.
- Send chunk: `"\x1b[5A"` (cursor-up 5, no following erase sequence).

Assert:
- The mock terminal `.write()` is called immediately (within the same tick or before throttle window).
- Chunk is not discarded.

#### Error/edge path — genuine full-screen redraw IS throttled

```
RedrawThrottler_should_throttle_When_chunkStartsWithCursorUpFollowedByErase
```

Setup:
- Send chunk 1: `"\x1b[5A\x1b[2K..."` (cursor-up + erase line — matches full-redraw regex).
- Within 33 ms, send chunk 2: another full-redraw chunk.

Assert:
- Only one write reaches the terminal (the second; first is dropped by throttle).

#### Integration — 33 ms throttle window respected

```
RedrawThrottler_should_emitBothFrames_When_fullRedrawsArriveAfterThrottleWindow
```

Setup:
- Send full-redraw chunk 1.
- Advance fake timers by 34 ms.
- Send full-redraw chunk 2.

Assert:
- Both writes reach the terminal.
- `throttleMs` used is 33 (not 100).

---

### Story 1.5.1: Combined Phase 1 Pipeline Integration Test (TypeScript/Jest)

**File**: `web-app/src/lib/terminal/__tests__/pipeline-integration.test.ts`

#### Integration — all four Phase 1 scenarios in a single test run

```
PipelineIntegration_should_passAllFourScenarios_When_PhaseOneFixes
```

Setup:
- Construct a mock terminal adapter that records all `.write()` calls.
- Wire together `StateApplicator`, `EscapeSequenceParser`, and `TerminalStreamManager`
  in the same order they execute in production.
- Use Jest fake timers to control throttle window timing.

Scenario 1 — multi-byte split (TextDecoder fix):
- Feed `\xc3` (first byte of "é") via one proto `TerminalDiff`, then `\xa9` (second byte)
  via a second `TerminalDiff`.
- Assert no `U+FFFD` (U+FFFD) in write calls; assert "é" present.

Scenario 2 — lookback window (ESP fix):
- Build a chunk: 25 ASCII bytes + `\x1b]0;some title` (no BEL/ST).
- Feed through `EscapeSequenceParser.processChunk`.
- Assert the partial OSC is buffered, not emitted immediately.

Scenario 3 — ED2+ED3 passthrough (ED3 strip removal):
- Feed `"\x1b[2J\x1b[3J"` through `EscapeSequenceParser.processChunk`.
- Assert output equals `"\x1b[2J\x1b[3J"` (not `"\x1b[2J"`).

Scenario 4 — cursor-up not throttled (RedrawThrottler fix):
- Feed a chunk starting with `"\x1b[5A"` (cursor-up only, no erase) to
  `TerminalStreamManager`.
- Assert the chunk reaches the mock terminal immediately (not discarded by throttle).

Assert (overall):
- All four scenarios pass without any assertion failure.
- Test completes in a single run (no separate files needed).

---

## Phase 2 Analytics — Detailed Test Specifications

### Story 2.1.1: Wire Stage 2 tap in `streamViaTmuxCapturePane` (Go)

**File**: `server/services/connectrpc_websocket_test.go` (new or existing)

#### Happy path — Stage 2 tap called when parser is enabled

```
TestStreamViaTmuxCapturePane_Stage2TapCalled_WhenEscapeParserEnabled
```

Setup:
- Construct a mock `EscapeParser` (interface with `IsEnabled() bool` and `ParseStage2([]byte, int64)`).
- Set `IsEnabled()` to return `true`.
- Run `streamViaTmuxCapturePane` with a capture-pane response containing `fullContent = "hello\x1b[0m"`.

Assert:
- `ParseStage2` was called exactly once with content matching `"hello\x1b[0m"`.

#### Error/edge path — nil parser does not panic

```
TestStreamViaTmuxCapturePane_Stage2TapSkipped_WhenEscapeParserNil
```

Setup:
- Configure `instance.GetEscapeParser()` to return `nil`.
- Run `streamViaTmuxCapturePane` with valid content.

Assert:
- No panic.
- `WriteMessage` is still called (output stream not interrupted).

#### Integration — Stage 2 mirrors existing `streamViaControlMode` tap

```
TestStreamViaTmuxCapturePane_Stage2BehaviorMatchesControlMode_WhenBothPathsRun
```

Setup:
- Use a single real `EscapeParser` instance with `SetEnabled(true)`.
- Route identical content through both `streamViaControlMode` (Stage 2 already wired) and `streamViaTmuxCapturePane`.

Assert:
- Both paths record the same content bytes in the parser.
- Parser accumulates two Stage 2 calls total (one per path).

---

### Story 2.1.2: Enable analytics via env var (Go)

**Files**: `pkg/analytics/escape_code_store_test.go`, `config/config_test.go`

#### Happy path — `Record()` stores entry when enabled

```
TestEscapeCodeStore_RecordsEntry_WhenEnabled
```

Setup:
- Construct `EscapeCodeStore{}` with `enabled = true`.
- Call `Record(someEntry)`.

Assert:
- Store contains exactly one entry matching `someEntry`.

#### Error/edge path — `Record()` is no-op when disabled

```
TestEscapeCodeStore_RecordNoOp_WhenDisabled
```

Setup:
- Construct `EscapeCodeStore{}` with `enabled = false` (default).
- Call `Record(someEntry)`.

Assert:
- Store length remains 0.
- No panic, no error.

#### Integration — env var activates analytics

```
TestConfig_DebugEscapeAnalyticsTrue_WhenEnvVarSet
```

Setup:
- Set `STAPLER_SQUAD_DEBUG_ESCAPE_ANALYTICS=true` in the test process environment.
- Call the config loader (`LoadConfig()` or equivalent).

Assert:
- `cfg.DebugEscapeAnalytics == true`.

Teardown:
- Unset `STAPLER_SQUAD_DEBUG_ESCAPE_ANALYTICS` after the test.

---

## Test Stack

| Layer | Framework | Notes |
|-------|-----------|-------|
| Unit (TypeScript) | Jest + `@testing-library/react` | For hooks, use `renderHook` from `@testing-library/react`. Fake timers (`jest.useFakeTimers()`) required for throttle tests. |
| Unit (Go) | `go test` + `github.com/stretchr/testify` | Table-driven tests preferred. |
| Integration (Go) | `go test` with in-process mock dependencies | No real SQLite or ent required for these stories — mock interfaces suffice. Real config loader used for env var test. |
| Browser validation | Manual smoke test | Required for Story 1.2.2 before merge. Record verdict in PR description. |

---

## Coverage Targets

| Target | Threshold | Notes |
|--------|-----------|-------|
| Unit test coverage | ≥ 80% for all modified files | Enforced per project CI; do not submit below threshold |
| TextDecoder call sites | 100% | Every `textDecoderRef.current.decode` and `scrollbackDecoderRef.current.decode` call must be exercised with a split multi-byte sequence |
| Escape sequence types | 100% of newly added branches | CSI: covered by existing tests; OSC, DCS, PM, APC: must be covered by new tests in Story 1.2.1 |
| Analytics gate | Both enabled=true and enabled=false paths | Both branches of `if escapeParser != nil && escapeParser.IsEnabled()` must be hit |
| Throttle boundary | Both `≤ throttleMs` and `> throttleMs` timing | Use `jest.useFakeTimers()` to test both sides of 33 ms boundary |

---

## Pre-Merge Checklist

- [ ] All Jest tests pass: `cd web-app && npx jest --no-coverage`
- [ ] All Go tests pass: `make build && make test`
- [ ] Coverage at or above 80% for each modified TypeScript file
- [ ] Browser smoke test completed for Story 1.2.2 (scrollback bleed + flicker) — recorded in PR description
- [ ] `STAPLER_SQUAD_DEBUG_ESCAPE_ANALYTICS` env var documented (Story 2.1.2)
- [ ] No `�` visible in a live Claude Code session after Phase 1 fixes applied
