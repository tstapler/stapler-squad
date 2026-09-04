# Validation Plan: osc-status-signals

**Date**: 2026-08-07

## Happy Path Scenario

Given a started `ClaudeController` whose PTY tail shows a bare shell prompt (`"$ "`, which
text-pattern matching alone would classify as idle/ready) but whose OSC window-title still
contains a Braille spinner glyph (`\x1b]0;⠋ working\x07`), when `GetCurrentStatus()` (and
`GetStatusAndIdleInfo()`) is called, then the returned `DetectedStatus` is `StatusExecuting` and
the returned `IdleStateInfo.State` is `IdleStateActive` — the false-idle bug is fixed, with the
transition committed inside the 150ms `OSCDebounceDelay` window rather than blocked by the 500ms
text-pattern `DebounceDelay`.

## Requirement → Test Mapping

**Note (2026-08-28):** the table below predates the architecture-review.md design
correction. Every `TestIdleDetector_ApplyOSCStatus_*` name below was implemented as
the equivalent `TestIdleDetector_DetectStateFromContentWithOSC_*` in
`session/detection/idle_test.go` (same scenario, same AC coverage — see plan.md's
Story 3.1.2 for why the standalone `ApplyOSCStatus` method was dropped). Two rows
were also added beyond what's listed here: `TestApplyOSCStatusOverride_FullMatrix`
(all 12 `DetectedStatus` values × both OSC directions, closing the adversarial-review
Concern that only 3 of ~20 branch combinations had coverage) and
`TestIdleDetector_DetectStateFromContentWithOSC_NonPromotableTextBlocksOverride`
(the direct BLOCKER 2 regression test).

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: OSC title captured without altering stripped-text behavior | `pkg/ansi/osc_test.go` | `TestExtractLastOSC` (BEL-terminated match subcase) | Unit | Happy: `"\x1b]0;⠋ working\x07"` → `("⠋ working", true)`; `stripANSI` output unchanged |
| AC1 | `pkg/ansi/osc_test.go` | `TestExtractLastOSC` (unterminated / stray-terminator subcases) | Unit | Edge: opening prefix with no terminator anywhere → `("", false)`; stray terminator with no preceding prefix in-window → `("", false)` |
| AC1 | — | — | Integration | N/A — pure string primitive, no controller state involved; covered end-to-end by AC2/AC7's integration tests instead |
| AC2: OSC title threaded into pipeline as a distinct input reaching `claude.go` | `session/detection/binaries/claude_test.go` | `TestClassifyOSCTitle` (spinner subcase) | Unit | Happy: extracted title string `"⠋ Thinking"` → `(OSCStatusExecuting, true)` |
| AC2 | `session/detection/binaries/claude_test.go` | `TestClassifyOSCTitle` (empty / unrelated-text subcases) | Unit | Edge: `""` and `"my-shell — bash"` → `(OSCStatusNone, false)` |
| AC2 | `session/claude_controller_test.go` | `TestClassifyOSC_StaleActivity_FallsBackToNone` | Integration | Proves `cc.classifyOSC` extracts the title from the raw tail and passes that extracted string (not the stripped screen text) into `binaries.ClassifyOSCTitle` |
| AC3: Braille spinner (U+2800–U+28FF) → `OSCStatusExecuting` | `session/detection/binaries/claude_test.go` | `TestClassifyOSCTitle` (hand-listed frames `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`) | Unit | Happy: known spinner frames → `OSCStatusExecuting` |
| AC3 | `session/detection/binaries/claude_test.go` | `TestClassifyOSCTitle` (Braille char outside hand-listed set, e.g. `⡇` U+2847) | Unit | Edge: full Unicode block coverage, not just the frames already hand-listed for screen-text patterns |
| AC3 | `session/claude_controller_test.go` | `TestGetCurrentStatus_OSCSpinnerOverridesFalseIdle` | Integration | Bare `"$ "` prompt + spinner OSC title → `GetCurrentStatus()` returns `StatusExecuting` |
| AC4: `✳` (U+2733) → idle status | `session/detection/binaries/claude_test.go` | `TestClassifyOSCTitle` (`"✳"` subcase) | Unit | Happy: exact `✳` glyph → `(OSCStatusIdle, true)` |
| AC4 | `session/detection/binaries/claude_test.go` | `TestClassifyOSCTitle` (`"✻"` U+273B / `"✽"` U+273D subcases) | Unit | Edge: transcription-slip guard — visually similar glyphs used elsewhere in this file's screen-text patterns → `(OSCStatusNone, false)` |
| AC4 | `session/claude_controller_test.go` | `TestGetCurrentStatus_OSCIdleMarker_PromotesReadyOnlyText` | Integration | Ready/Unknown-only text + `✳` OSC title → `GetCurrentStatus()` returns `StatusIdle` |
| AC5: OSC-derived status wins for the false-idle case (asymmetric, upgrade-only) | `session/detection/idle_test.go` | `TestIdleDetector_ApplyOSCStatus_BypassesTextDebounceViaShorterWindow` | Unit | Happy: OSC-derived `OSCStatusExecuting` promotes `IdleStateWaiting → IdleStateActive` where the text-pattern path would still be blocked |
| AC5 | `session/detection/idle_test.go` | `TestIdleDetector_ApplyOSCStatus_NoneIsNoOp` | Unit | Edge: `OSCStatusNone` is a caller-error no-op guard — doesn't corrupt state, defends the mechanism AC5's override policy depends on |
| AC5 | `session/claude_controller_test.go` | `TestGetCurrentStatus_OSCSpinnerOverridesFalseIdle` | Integration | The exact AC5 scenario: `"$ "` + spinner OSC title → `StatusExecuting`, not `StatusIdle`/`StatusReady`/`StatusUnknown` |
| AC5 | `session/claude_controller_test.go` | `TestGetCurrentStatus_OSCIdle_DoesNotOverrideActiveText` | Integration | Asymmetric safety: `"esc to interrupt"` (text → `StatusExecuting`) + stale/nested `✳` OSC title → status remains `StatusExecuting`, never downgraded |
| AC5 | `session/claude_controller_test.go` | `TestGetStatusAndIdleInfo_OSCPromotesIdleState` | Integration | OSC override reaches the returned `IdleStateInfo.State` (`IdleStateActive`), not just the displayed `DetectedStatus` badge |
| AC6: OSC transitions bypass the text-pattern debounce via a shorter, dedicated window | `session/detection/idle_test.go` | `TestIdleDetector_ApplyOSCStatus_BypassesTextDebounceViaShorterWindow` | Unit | Happy: 200ms elapsed satisfies the 150ms `OSCDebounceDelay` but would NOT satisfy the 500ms `DebounceDelay` — proves the two windows are independent |
| AC6 | `session/detection/idle_test.go` | `TestIdleDetector_ApplyOSCStatus_FirstTransitionNeverDebounced` | Unit | Edge: first transition out of `IdleStateUnknown` commits with no wait (parity with the existing text-pattern gate) |
| AC6 | `session/detection/idle_test.go` | `TestIdleDetector_ApplyOSCStatus_RepeatedSameStatusDoesNotChurnClock` | Unit | Edge: two consecutive same-classification calls within the window only update `lastStateChange` once — same-class spinner redraws don't churn state |
| AC6 | `session/claude_controller_test.go` | `TestGetStatusAndIdleInfo_OSCPromotesIdleState` | Integration | Proves the debounce-bypass wiring (Task 4.1.3) reaches `IdleStateInfo` end-to-end through the controller, not just the standalone `IdleDetector` |
| AC7: existing text-pattern detection unchanged when no OSC title is present (graceful fallback) | `pkg/ansi/osc_test.go` | `TestExtractLastOSC` (no ESC byte at all subcase) | Unit | Happy: `"plain text"` → `("", false)` — clean no-match path |
| AC7 | `session/claude_controller_test.go` | `TestClassifyOSC_StaleActivity_FallsBackToNone` | Unit/Controller | Edge: `classifyOSC` returns `(OSCStatusNone, false)` when the controller isn't started or the PTY has been silent past `oscStaleThreshold`, even if a spinner title is still technically present in the stale tail |
| AC7 | `session/claude_controller_test.go` | `TestGetCurrentStatus_NoOSCTitle_FallsBackToTextPattern` | Integration | `tmuxOutputSmall` fixture (no OSC sequence) → `(status, desc)` byte-for-byte identical to pre-feature output |
| AC8: no regression in existing `session/detection` test suite | `session/detection/...`, `pkg/ansi/...`, `session/...` | `go test ./session/detection/... ./pkg/ansi/... ./session/...` (Task 5.1a) | Regression | Full suite exits 0; no pre-existing assertion in `detector_test.go`, `pattern_set_test.go`, `bug_regression_test.go`, `asterism_test.go`, or `idle_test.go` was modified to make it pass |
| AC9: new behavior has test coverage (OSC parsing + priority/override behavior) | `pkg/ansi/osc_test.go` | `TestExtractLastOSC_ZeroAllocsOnPlainText` (Task 1.1.2b) | Unit | Supporting: zero-alloc fast path on ESC-byte-free input, mirroring `csi_test.go`'s existing convention |
| AC9 | `session/detection/idle_test.go` | `TestIdleDetector_ApplyOSCStatus_BumpsLastActivity` (Task 3.1.2b) | Unit | Supporting: `OSCStatusExecuting` bumps `lastActivity`, parity with `mapStatusToIdleState`'s existing `StatusExecuting` branch |
| AC9 | all files above | Task 5.1b AC-by-AC cross-check | Meta | Every AC1–AC9 re-verified against the actual diff/test list, traceable to ≥1 passing test — not from memory |

## UX Acceptance Tests

N/A — pure backend detection logic (Go), no user-facing surface change in this feature. No
`ux.md` exists for this project; nothing in `web-app/` is touched by this plan.

## Test Stack

- **Unit**: Go stdlib `testing`, table-driven tests (`pkg/ansi/osc_test.go`,
  `session/detection/binaries/claude_test.go`, `session/detection/idle_test.go` using the
  existing `newDetectorWithFakeClock` fake-clock helper)
- **Integration**: Go stdlib `testing`, existing `newControllerWithMock` fixture in
  `session/claude_controller_test.go` — exercises `GetCurrentStatus`/`GetStatusAndIdleInfo`/
  `GetIdleState` end-to-end against synthetic PTY tail content
- **E2E / UX**: N/A

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/detection/... ./pkg/ansi/... ./session/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

- All public functions/methods introduced by this plan (`ExtractLastOSC`, `ClassifyOSCTitle`,
  `IdleDetector.DetectStateFromContentWithOSC`, `ClaudeController.classifyOSC`, `applyOSCStatusOverride`,
  `IsOSCExecutingPromotable`, `IsOSCIdlePromotable` — the last two added in the 2026-08-28 design
  correction, see architecture-review.md):
  happy path + error/edge paths covered
- External-facing integration points (`GetCurrentStatus`, `GetStatusAndIdleInfo`, `GetIdleState`):
  each has at least one controller-level integration test exercising the OSC override path
- `make lint` (or `golangci-lint run ./session/... ./pkg/ansi/...`) clean — required alongside
  the coverage run per Task 5.1a
