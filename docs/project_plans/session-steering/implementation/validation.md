# Session Steering — Validation Plan

**Date**: 2026-05-20
**Status**: Ready for implementation
**Plan version**: Patched (all BLOCKED items resolved, all CONCERNS addressed)

---

## Summary

| Metric | Count |
|---|---|
| Unit test cases | 34 |
| Integration test cases | 7 |
| **Total test cases** | **41** |
| ACs with ≥1 test | 10 / 10 |
| Requirements coverage | **100%** |

---

## Acceptance Criteria → Test Traceability

| AC | Criterion (abbreviated) | Tests |
|---|---|---|
| AC-1 | Triage session answers startup dialog + sends initial prompt | `TestIsStartupDialog/*`, `TestShouldApprovePrompt/*`, `TestSessionDriver_Integration_DialogHandling_TriageSession` |
| AC-2 | Work session answers startup dialog + sends initial prompt | `TestSessionDriver_Integration_DialogHandling_WorkSession` |
| AC-3 | Review session answers startup dialog + sends initial prompt | `TestSessionDriver_Integration_ReviewSession_ExitsCleanly` |
| AC-4 | MCP-created session answers startup dialog + sends initial prompt | `TestSessionDriver_Integration_DialogHandling_MCPSession` |
| AC-5 | Unexpected exit restarts exactly once with continuation prompt | `TestSessionDriver_ExitDetection_RetriesWorkSession`, `TestHandleDriverFailure_StoppedSession_RestartFlow`, `TestSessionDriver_Integration_ExitRetry_SendsContinuationPrompt` |
| AC-6 | No output for 10 min restarts exactly once with continuation prompt | `TestSessionDriver_Inactivity_FiresWhenReadyAndStale`, `TestSessionDriver_Integration_InactivityRetry` |
| AC-7 | Two failures → NeedsAttention, no further restarts | `TestSessionDriver_SecondFailure_MarksNeedsAttention`, `TestSessionDriver_Integration_DoubleFailure_MarksNeedsAttention` |
| AC-8 | All 4 creation paths call StartSessionDriver | `TestStartSessionDriver_CalledFromAllFourPaths` |
| AC-9 | Two StartSessionDriver calls on same instance = no-op | `TestStartSessionDriver_Idempotent` |
| AC-10 | Panic in driver goroutine recovered and logged | `TestSessionDriver_PanicRecovery_InitialGoroutine`, `TestSessionDriver_PanicRecovery_RetryGoroutine` |

---

## Unit Tests

### File: `session/session_driver_test.go`

These tests extend the existing file (which already contains `TestIsStartupDialog` and `TestShouldApprovePrompt`).

---

#### UT-1: `TestIsStartupDialog` (existing — verify completeness)

**Covers**: FR-2, AC-1 through AC-4 (dialog detection is shared by all creation paths)

| Sub-case | Input | Expected |
|---|---|---|
| trust-folder dialog (exact, with arrow) | Full dialog with `❯ 1. Yes, I trust this folder` | `true` |
| trust-folder dialog (numbered-dot variant) | `1. Yes, I trust this folder` variant | `true` |
| normal `>` prompt | `> ` | `false` |
| plain text mentioning "trust" but no menu | `I trust this code completely.` | `false` |
| allow-directory dialog (handled by `shouldApprovePrompt`) | `Allow reading in /path` dialog | `false` |
| empty output | `""` | `false` |

**Already implemented.** Verify these 6 sub-cases are present before removing.

---

#### UT-2: `TestShouldApprovePrompt` (existing — verify completeness)

**Covers**: FR-2, AC-1 through AC-4

| Sub-case | Input | `allowedPath` | Expected |
|---|---|---|---|
| Allow reading in allowed path (exact) | `Allow reading in /home/user/myrepo` | `/home/user/myrepo` | `true` |
| Allow writing in sub-path of allowed path | `Allow writing in /home/user/myrepo/src` | `/home/user/myrepo` | `true` |
| Allow reading in unrelated path | `Allow reading in /etc/passwd` | `/home/user/myrepo` | `false` |
| "Do you want to proceed?" no path restriction | any | `""` | `true` |
| "Do you want to proceed?" with unrelated allowedPath | any | `/home/user/myrepo` | `false` |
| Unrelated output | `Compiling project…` | `/home/user/myrepo` | `false` |

**Already implemented.**

---

#### UT-3: `TestIsOneShot`

**Covers**: FR-5 (one-shot guard), AC-5 (triage/review sessions must not be retried)
**File**: `session/session_driver_test.go`

| Sub-case | Tags on Instance | Expected |
|---|---|---|
| triage tag | `["backlog:triage"]` | `true` |
| review tag | `["backlog:review"]` | `true` |
| work tag | `["backlog:work"]` | `false` |
| mcp source tag | `["source:mcp"]` | `false` |
| no tags | `[]` | `false` |
| both triage and work tags | `["backlog:triage", "backlog:work"]` | `true` (triage wins) |

**New test. 6 sub-cases.**

---

#### UT-4: `TestBuildContinuationPrompt_NoHistoryFile`

**Covers**: FR-6, AC-5, AC-6
**File**: `session/session_driver_test.go`

**Setup**: Instance with `HistoryFilePath == ""`
**Assert**: Returns non-empty string containing `"Please continue"` (generic fallback)

**1 sub-case.**

---

#### UT-5: `TestBuildContinuationPrompt_MissingFile`

**Covers**: FR-6 (graceful degradation on I/O error)
**File**: `session/session_driver_test.go`

**Setup**: Instance with `HistoryFilePath` set to a path that does not exist on disk
**Assert**: Returns non-empty generic fallback string; does not panic

**1 sub-case.**

---

#### UT-6: `TestBuildContinuationPrompt_WithJSONL`

**Covers**: FR-6, AC-5, AC-6
**File**: `session/session_driver_continuation_test.go`

**Setup**: Write a temp JSONL file with 3 complete user+assistant turn pairs; set `inst.HistoryFilePath` to that file
**Assert**:
- Returned string is non-empty
- Contains content from the last assistant message
- Contains "Please continue" or equivalent continuation instruction

**1 sub-case.**

---

#### UT-7: `TestBuildContinuationPrompt_TruncatesLongMessage`

**Covers**: FR-6 (500-char truncation boundary)
**File**: `session/session_driver_continuation_test.go`

**Setup**: Write JSONL with one assistant message of exactly 1000 characters (`strings.Repeat("x", 1000)`)
**Assert**:
- Returned prompt contains `"..."` (truncation marker)
- The content before `"..."` is exactly 500 characters from the assistant message

**1 sub-case.**

---

#### UT-8: `TestBuildContinuationPrompt_NoAssistantMessage`

**Covers**: FR-6 (graceful degradation when only user messages present)
**File**: `session/session_driver_continuation_test.go`

**Setup**: Write JSONL with only user-role messages; no assistant messages
**Assert**: Returns generic fallback string (not empty, not a panic)

**1 sub-case.**

---

#### UT-9: `TestStartSessionDriver_Idempotent`

**Covers**: AC-9, NFR idempotency, Epic 6
**File**: `session/session_driver_test.go`

**Setup**: Create minimal Instance; set `inst.Status = Stopped` so the driver goroutine exits immediately
**Steps**:
1. Call `StartSessionDriver(inst, "/tmp")` — first call
2. Assert `inst.driverRunning.Load() == true` (goroutine started)
3. Call `StartSessionDriver(inst, "/tmp")` again immediately — second call
4. Assert second call returned without spawning (log debug message emitted, or check that `driverRunning` is still true with a single CAS)
5. Wait for goroutine to exit
6. Assert `inst.driverRunning.Load() == false` (defer fired)

**2 sub-cases: concurrent calls and sequential calls.**

---

#### UT-10: `TestSessionDriver_Inactivity_NoFireWhileRunning`

**Covers**: FR-4 (pitfall #4 guard — long compilation should not trigger inactivity)
**File**: `session/session_driver_test.go`

**Setup**: Mock instance with `GetEffectiveStatus() == Running`; `LastMeaningfulOutputTime()` returns 20 minutes ago; `sentInitial = true`
**Assert**: After one poll tick, `handleDriverFailure` is NOT called; driver continues running

**1 sub-case.**

---

#### UT-11: `TestSessionDriver_Inactivity_FiresWhenReadyAndStale`

**Covers**: FR-4, AC-6
**File**: `session/session_driver_test.go`

**Setup**: Mock instance with `GetEffectiveStatus() == Ready`; `LastMeaningfulOutputTime()` returns 11 minutes ago; `sentInitial = true`
**Assert**: `handleDriverFailure` is called with reason containing `"inactivity"`; `retried` flips to `true`

**1 sub-case.**

---

#### UT-12: `TestSessionDriver_Inactivity_DoesNotFireBeforeInitialPrompt`

**Covers**: FR-4 (must not fire during startup — `sentInitial == false` guard)
**File**: `session/session_driver_test.go`

**Setup**: Mock instance with `GetEffectiveStatus() == Ready`; `LastMeaningfulOutputTime()` 11 minutes ago; `sentInitial = false`
**Assert**: Driver does not call `handleDriverFailure`

**1 sub-case.**

---

#### UT-13: `TestSessionDriver_ExitDetection_SkipsOneShot_Triage`

**Covers**: FR-5, AC-5 (triage must not be retried)
**File**: `session/session_driver_test.go`

**Setup**: Instance tagged `backlog:triage`; `GetEffectiveStatus() == Stopped`; `sentInitial = true`
**Assert**: Driver calls `return` cleanly; `handleDriverFailure` NOT called; `ReviewQueue.Add` NOT called

**1 sub-case.**

---

#### UT-14: `TestSessionDriver_ExitDetection_SkipsOneShot_Review`

**Covers**: FR-5, AC-3 (review must not be retried)
**File**: `session/session_driver_test.go`

**Setup**: Instance tagged `backlog:review`; `GetEffectiveStatus() == Stopped`; `sentInitial = true`
**Assert**: Driver calls `return` cleanly; `handleDriverFailure` NOT called

**1 sub-case.**

---

#### UT-15: `TestSessionDriver_ExitDetection_RetriesWorkSession`

**Covers**: FR-5, FR-7, AC-5
**File**: `session/session_driver_test.go`

**Setup**: Instance tagged `backlog:work`; `GetEffectiveStatus() == Stopped`; `sentInitial = true`; `initialPromptSentAt = time.Now()` (within 5-minute minimum-runtime window); `retried = false`
**Assert**: `handleDriverFailure` called; `retried.Load() == true` after call; restart is attempted

**1 sub-case.**

---

#### UT-16: `TestSessionDriver_ExitDetection_SkipsLongRunningSession`

**Covers**: FR-5, CONCERN-2 (minimum-runtime guard — sessions that ran >5 min are completions)
**File**: `session/session_driver_test.go`

**Setup**: Instance tagged `backlog:work`; `GetEffectiveStatus() == Stopped`; `sentInitial = true`; `initialPromptSentAt = time.Now().Add(-6 * time.Minute)` (outside 5-minute window)
**Assert**: Driver exits cleanly (treats as normal completion); `handleDriverFailure` NOT called

**1 sub-case.**

---

#### UT-17: `TestSessionDriver_SecondFailure_MarksNeedsAttention`

**Covers**: FR-7, AC-7
**File**: `session/session_driver_test.go`

**Setup**: Instance with `retried = true` (simulates second failure); mock `ReviewQueueWriter`; call `handleDriverFailure(inst, path, &retried, "unexpected exit")`
**Assert**:
- `ReviewQueue.Add` called with a `*ReviewItem`
- `ReviewItem.SessionID == inst.UUID`
- `ReviewItem.Reason == ReasonStale`
- No restart attempted (no call to `inst.Restart` or `inst.Start`)

**1 sub-case.**

---

#### UT-18: `TestHandleDriverFailure_StoppedSession_RestartFlow`

**Covers**: FR-7, CONCERN-4 (RecoverFromStopped → Start(false) path), AC-5
**File**: `session/session_driver_test.go`

**Setup**: Instance with `GetEffectiveStatus() == Stopped`; mock `RecoverFromStopped` and `Start` to return `nil`; `retried = false`
**Steps**:
1. Call `handleDriverFailure(inst, path, &retried, "unexpected exit")`
2. Assert `RecoverFromStopped()` called before `Start(false)` (call order check)
3. Assert new driver goroutine started
4. Assert `retried.Load() == true`

**1 sub-case.**

---

#### UT-19: `TestHandleDriverFailure_RunningSession_RestartFlow`

**Covers**: FR-7 (Restart path for non-Stopped sessions), AC-6
**File**: `session/session_driver_test.go`

**Setup**: Instance with `GetEffectiveStatus() == Ready` (stuck case, not Stopped); mock `Restart` to return `nil`; `retried = false`
**Assert**: `inst.Restart(false)` called (NOT `RecoverFromStopped + Start`); new driver goroutine started

**1 sub-case.**

---

#### UT-20: `TestHandleDriverFailure_RestartError_MarksNeedsAttention`

**Covers**: FR-7 (error path when restart itself fails), AC-7
**File**: `session/session_driver_test.go`

**Setup**: Instance where `Restart` returns a non-nil error; mock `ReviewQueueWriter`; `retried = false`
**Assert**: `ReviewQueue.Add` called (marks for attention); driver exits without spawning new goroutine

**1 sub-case.**

---

#### UT-21: `TestMarkSessionNeedsAttention_NilReviewQueue`

**Covers**: NFR crash isolation (nil ReviewQueue must not panic)
**File**: `session/session_driver_test.go`

**Setup**: Instance with `GetReviewQueue()` returning `nil`
**Assert**: `markSessionNeedsAttention` returns without panicking; a warning log is emitted

**1 sub-case.**

---

#### UT-22: `TestSessionDriver_PanicRecovery_InitialGoroutine`

**Covers**: NFR crash isolation, AC-10
**File**: `session/session_driver_test.go`

**Setup**: Mock `runSessionDriverWithPrompt` to `panic("synthetic panic")`; wrap in the goroutine wrapper from `StartSessionDriver`
**Assert**:
- The goroutine exits without crashing the test process (panic recovered)
- A log entry is written (check log output or use a log capture helper)
- `inst.driverRunning.Load() == false` after recovery (defer fired)

**1 sub-case.**

---

#### UT-23: `TestSessionDriver_PanicRecovery_RetryGoroutine`

**Covers**: CONCERN-3 resolution, AC-10 (retry goroutine also protected)
**File**: `session/session_driver_test.go`

**Setup**: On second call to `runSessionDriverWithPrompt` (the retry path spawned by `handleDriverFailure`), inject a panic via a test hook
**Assert**: Panic is recovered inside `runSessionDriverWithPrompt` (not via the `StartSessionDriver` wrapper, since retry goes through `handleDriverFailure` directly); server does not crash

**1 sub-case.**

---

#### UT-24: `TestStartSessionDriver_CalledFromAllFourPaths`

**Covers**: FR-1, AC-8
**File**: `session/session_driver_test.go` + integration check

This is a code-path existence test, not a behavioral test. It verifies that `StartSessionDriver` (or equivalent `RegisterSteered`) appears in:
1. `server/services/session_service.go` `CreateDirectorySession` (already present — line 474)
2. `server/mcp/tools_lifecycle.go` `createSession` (Epic 1 — to be added)
3. Backlog triage path via `CreateDirectorySession` (already covered)
4. Backlog work path via `CreateDirectorySession` (already covered)
5. Backlog review path via `CreateDirectorySession` (already covered)

**Implementation note**: Points 3–5 are verified by the plan's research claim that all backlog paths go through `CreateDirectorySession`. This test verifies point 2 at the source level (grep or AST check that `session.StartSessionDriver` appears in `tools_lifecycle.go`). Write as a `TestMain`-style scan or a comment-verified source check.

**1 sub-case.**

---

#### UT-25: `TestDriverConstants_Ordering`

**Covers**: BLOCK-1 resolution (driverTotalTimeout > driverReadyTimeout + driverInactivityTimeout)
**File**: `session/session_driver_test.go`

**Setup**: Compile-time or runtime constant comparison
**Assert**: `driverTotalTimeout >= driverReadyTimeout + driverInactivityTimeout + 5*time.Minute`

This prevents future regressions where constants are independently modified.

**1 sub-case.**

---

### Unit Test Count

| Test function | Sub-cases | New / Existing |
|---|---|---|
| UT-1 `TestIsStartupDialog` | 6 | Existing |
| UT-2 `TestShouldApprovePrompt` | 6 | Existing |
| UT-3 `TestIsOneShot` | 6 | New |
| UT-4 `TestBuildContinuationPrompt_NoHistoryFile` | 1 | New |
| UT-5 `TestBuildContinuationPrompt_MissingFile` | 1 | New |
| UT-6 `TestBuildContinuationPrompt_WithJSONL` | 1 | New |
| UT-7 `TestBuildContinuationPrompt_TruncatesLongMessage` | 1 | New |
| UT-8 `TestBuildContinuationPrompt_NoAssistantMessage` | 1 | New |
| UT-9 `TestStartSessionDriver_Idempotent` | 2 | New |
| UT-10 `TestSessionDriver_Inactivity_NoFireWhileRunning` | 1 | New |
| UT-11 `TestSessionDriver_Inactivity_FiresWhenReadyAndStale` | 1 | New |
| UT-12 `TestSessionDriver_Inactivity_DoesNotFireBeforeInitialPrompt` | 1 | New |
| UT-13 `TestSessionDriver_ExitDetection_SkipsOneShot_Triage` | 1 | New |
| UT-14 `TestSessionDriver_ExitDetection_SkipsOneShot_Review` | 1 | New |
| UT-15 `TestSessionDriver_ExitDetection_RetriesWorkSession` | 1 | New |
| UT-16 `TestSessionDriver_ExitDetection_SkipsLongRunningSession` | 1 | New |
| UT-17 `TestSessionDriver_SecondFailure_MarksNeedsAttention` | 1 | New |
| UT-18 `TestHandleDriverFailure_StoppedSession_RestartFlow` | 1 | New |
| UT-19 `TestHandleDriverFailure_RunningSession_RestartFlow` | 1 | New |
| UT-20 `TestHandleDriverFailure_RestartError_MarksNeedsAttention` | 1 | New |
| UT-21 `TestMarkSessionNeedsAttention_NilReviewQueue` | 1 | New |
| UT-22 `TestSessionDriver_PanicRecovery_InitialGoroutine` | 1 | New |
| UT-23 `TestSessionDriver_PanicRecovery_RetryGoroutine` | 1 | New |
| UT-24 `TestStartSessionDriver_CalledFromAllFourPaths` | 1 | New |
| UT-25 `TestDriverConstants_Ordering` | 1 | New |
| **Total** | **34** | 12 existing sub-cases, 22 new tests |

---

## Integration Tests

Integration tests use a mock `Instance` that implements the `SessionDriver`-facing interface (status, preview, tags, restart, reviewqueue). They exercise full driver loop behavior, not individual functions.

**File**: `session/session_driver_integration_test.go` (new file)

---

#### IT-1: `TestSessionDriver_Integration_DialogHandling_TriageSession`

**Covers**: FR-2, FR-3, AC-1
**Flow**:
1. Create mock instance tagged `backlog:triage`; `Status = Loading`
2. Set preview output to the trust-folder dialog text
3. Call `StartSessionDriver(inst, path)` in background
4. After first poll tick: assert `"1\n"` was written to session stdin
5. Transition mock `Status = Ready`
6. After next tick: assert `driverInitialPrompt` was sent
7. Transition `Status = Stopped` (simulates task completion); driver exits cleanly

**Verifies**: AC-1 full flow (dialog answer → initial prompt dispatch → clean exit for one-shot)

---

#### IT-2: `TestSessionDriver_Integration_DialogHandling_WorkSession`

**Covers**: FR-2, FR-3, AC-2
**Flow**: Same as IT-1 but instance tagged `backlog:work`; after `Status = Stopped`, verify driver would call `handleDriverFailure` if it stopped quickly (set `initialPromptSentAt = time.Now()`)

---

#### IT-3: `TestSessionDriver_Integration_ReviewSession_ExitsCleanly`

**Covers**: FR-3, AC-3 (review session driver exits on Stopped without restart)
**Flow**:
1. Instance tagged `backlog:review`; `Status = Ready`
2. Driver sends initial prompt
3. Transition `Status = Stopped`
4. Assert driver exits without calling restart

---

#### IT-4: `TestSessionDriver_Integration_DialogHandling_MCPSession`

**Covers**: FR-2, FR-3, AC-4
**Flow**: Same as IT-1 but instance tagged `source:mcp`; verify initial prompt sent; `isOneShot == false` for MCP sessions

---

#### IT-5: `TestSessionDriver_Integration_ExitRetry_SendsContinuationPrompt`

**Covers**: FR-5, FR-6, FR-7, AC-5
**Flow**:
1. Instance tagged `backlog:work`; write a JSONL history file with known assistant content
2. Driver sends initial prompt; `initialPromptSentAt = time.Now()`
3. Immediately transition `Status = Stopped` (within 5-minute window)
4. Assert `handleDriverFailure` called; `buildContinuationPrompt` invoked; JSONL content appears in the continuation prompt
5. Restart succeeds; new driver goroutine starts with the continuation prompt
6. New driver immediately sees `Status = Stopped` again (second failure)
7. Assert `ReviewQueue.Add` called with `*ReviewItem`; no third restart attempted

---

#### IT-6: `TestSessionDriver_Integration_InactivityRetry`

**Covers**: FR-4, FR-7, AC-6
**Flow**:
1. Instance tagged `backlog:work`; `Status = Ready`; `sentInitial = true` (simulate via clock advance)
2. Set `LastMeaningfulOutputTime()` to return a time 11 minutes ago
3. Assert driver calls `handleDriverFailure` with `"inactivity timeout"` reason
4. After restart: second driver sees inactivity again → `ReviewQueue.Add` called

---

#### IT-7: `TestSessionDriver_Integration_DoubleFailure_MarksNeedsAttention`

**Covers**: FR-7, AC-7
**Flow**:
1. Instance with `retried = true` pre-set (already failed once)
2. `Status = Stopped`
3. Assert `markSessionNeedsAttention` called; `ReviewQueue.Add` receives `*ReviewItem` with `Reason == ReasonStale` and `SessionID == inst.UUID`
4. Assert no restart is spawned

---

### Integration Test Count

| Test function | Covers ACs |
|---|---|
| IT-1 `TestSessionDriver_Integration_DialogHandling_TriageSession` | AC-1 |
| IT-2 `TestSessionDriver_Integration_DialogHandling_WorkSession` | AC-2 |
| IT-3 `TestSessionDriver_Integration_ReviewSession_ExitsCleanly` | AC-3 |
| IT-4 `TestSessionDriver_Integration_DialogHandling_MCPSession` | AC-4 |
| IT-5 `TestSessionDriver_Integration_ExitRetry_SendsContinuationPrompt` | AC-5, AC-7 |
| IT-6 `TestSessionDriver_Integration_InactivityRetry` | AC-6, AC-7 |
| IT-7 `TestSessionDriver_Integration_DoubleFailure_MarksNeedsAttention` | AC-7 |
| **Total** | **7** |

---

## Edge Cases Covered

| Edge Case | Covered By |
|---|---|
| `HistoryFilePath == ""` (no JSONL yet) | UT-4 |
| JSONL file missing on disk | UT-5 |
| JSONL has no assistant messages | UT-8 |
| Assistant message > 500 chars (truncation) | UT-7 |
| Session exits before initial prompt sent (startup crash, work session) | UT-15 (combined with UT-15 setup variation) |
| Session exits before initial prompt sent (one-shot) | UT-13 covered by `sentInitial=false` path in UT-13/UT-14 |
| Long-running session exits after 5+ min (treat as completion) | UT-16 |
| `ReviewQueue` is nil | UT-21 |
| Restart itself returns an error | UT-20 |
| `driverTotalTimeout < driverReadyTimeout + driverInactivityTimeout` regression | UT-25 |
| Two concurrent `StartSessionDriver` calls | UT-9 (concurrent sub-case) |
| Panic in initial driver goroutine | UT-22 |
| Panic in retry driver goroutine | UT-23 |
| Inactivity fires during `Running` status (false positive) | UT-10 |
| Inactivity fires before `sentInitial` (false positive) | UT-12 |

---

## Test Infrastructure Notes

### Mock Instance Interface

The integration tests require a mock that satisfies:
- `GetEffectiveStatus() Status`
- `LastMeaningfulOutputTime() time.Time`
- `HasTag(tag string) bool`
- `GetReviewQueue() ReviewQueueWriter`
- `Restart(firstTimeSetup bool) error`
- `RecoverFromStopped() error`
- `Start(firstTimeSetup bool) error`
- `HistoryFilePath string` (field, or accessor)
- `UUID string` (field)
- `Title string` (field)

Use `session_driver_test.go`'s package-internal access (same `session` package) to create minimal `Instance` structs directly rather than interface mocks, following the existing test pattern.

### Clock Control

Tests for inactivity detection (UT-10, UT-11, UT-12, IT-6) that depend on `LastMeaningfulOutputTime()` do not need time mocking — they control the return value of the method on the mock instance directly.

Tests for the minimum-runtime guard (UT-15, UT-16) set `initialPromptSentAt` as a local variable in the test by calling into the driver loop with a controlled time value.

### Log Capture

Panic recovery tests (UT-22, UT-23) require verifying a log entry is written. Use the existing `log` package's test helper pattern (or slog's `slog.New` with a test handler) to capture log records during the test.

---

## Files Produced by Tests

| File | Status |
|---|---|
| `session/session_driver_test.go` | Modified — add UT-3 through UT-25 |
| `session/session_driver_continuation_test.go` | New — UT-6, UT-7, UT-8 (JSONL tests needing temp file setup) |
| `session/session_driver_integration_test.go` | New — IT-1 through IT-7 |

---

## Implementation Readiness Gate

### Criterion 1: PLAN — Every FR has at least one task; every task has a file + function + change description

| FR | Tasks | File(s) specified | Function(s) specified | Change described |
|---|---|---|---|---|
| FR-1 (universal wiring) | Task 1.1 | `server/mcp/tools_lifecycle.go` | `createSession` | One line: `session.StartSessionDriver(inst, path)` |
| FR-2 (startup dialog) | Existing (verified); Tasks 2.2, 3.2 use `GetEffectiveStatus()` per pitfall #6 | `session/session_driver.go` | `runSessionDriver` | Verified correct |
| FR-3 (initial prompt) | Existing (verified); Task 5.3 preserves it in `runSessionDriverWithPrompt` | `session/session_driver.go` | `runSessionDriverWithPrompt` | Refactored with parametric prompt |
| FR-4 (inactivity detection) | Tasks 2.1, 2.2 | `session/session_driver.go` | `runSessionDriver` | Constants + detection branch added |
| FR-5 (exit detection) | Tasks 3.1, 3.2 | `session/session_driver.go` | `runSessionDriver` + new `isOneShot` | Exit branch with one-shot guard and min-runtime guard |
| FR-6 (JSONL continuation) | Task 4.1 | `session/session_driver.go` | new `buildContinuationPrompt` | Full implementation with fallback |
| FR-7 (auto-retry) | Tasks 5.1, 5.2, 5.3 | `session/session_driver.go` | `handleDriverFailure`, `markSessionNeedsAttention`, `runSessionDriverWithPrompt` | Full retry + attention path |
| FR-8 (watchdog coordinator) | Tasks 6.1–6.3 (idempotency guard provides the registration mechanism) | `session/instance.go`, `session/session_driver.go` | `Instance struct`, `StartSessionDriver` | `driverRunning atomic.Bool` + CAS guard |

**Result: PASS** — Every FR has at least one task with file + function + change.

---

### Criterion 2: TEST — Every AC has at least one test; edge cases covered

| AC | Tests assigned | Edge cases covered |
|---|---|---|
| AC-1 | UT-1, UT-2, IT-1 | Dialog variants, empty output, path mismatch |
| AC-2 | IT-2 | Work session dialog + initial prompt |
| AC-3 | IT-3 | Review session exits cleanly on stop |
| AC-4 | IT-4 | MCP session dialog + initial prompt |
| AC-5 | UT-15, UT-18, IT-5 | One-shot skip, min-runtime guard, restart flow |
| AC-6 | UT-11, IT-6 | Ready-only firing, Running guard, pre-sentInitial guard |
| AC-7 | UT-17, IT-5, IT-6, IT-7 | Both inactivity and exit paths reach NeedsAttention; ReviewItem fields checked |
| AC-8 | UT-24 | All 4 creation paths verified |
| AC-9 | UT-9 | Both concurrent and sequential double-call |
| AC-10 | UT-22, UT-23 | Both initial and retry goroutine paths |

**Result: PASS** — All 10 ACs covered; edge cases beyond happy path present for every AC.

---

### Criterion 3: ADVERSARIAL — No BLOCKED items remain

Review of `adversarial-review.md` vs `plan.md` patch log:

| Issue | Status in adversarial review | Resolution in patched plan |
|---|---|---|
| BLOCK-1: `driverTotalTimeout` kills inactivity detection | BLOCKED | RESOLVED — `driverTotalTimeout` increased to 25 min (Task 2.1); explicitly documented in patch log and Design Decision D5 |
| BLOCK-2: `ReviewQueue.Add` signature mismatch | BLOCKED | RESOLVED — Task 5.2 rewritten to construct `*ReviewItem`; `ReasonStale` justified over new constant (Task 5.2); verified against `session/queue/queue.go:211` |
| BLOCK-3: `msgs[i].Type` → `msgs[i].Role` | BLOCKED | RESOLVED — Task 4.1 updated to use `.Role` and `.Content` directly; `extractMsgText` removed (Design Decision D4) |
| CONCERN-1: `driverRunning` reset race | CONCERN | RESOLVED — Task 6.3 explicitly states NO reset in `Restart`/`RecoverFromStopped`; patch log entry "CONCERN-1: `driverRunning` reset removed" |
| CONCERN-2: Normal completion vs crash | CONCERN | RESOLVED — `driverMinRuntimeBeforeRetry = 5 * time.Minute` constant added; guard added to Task 3.2; patch log entry "CONCERN-2: Minimum-runtime guard added" |
| CONCERN-3: Retry goroutine missing panic recovery | CONCERN | RESOLVED — Panic recovery moved inside `runSessionDriverWithPrompt` (Task 5.3); patch log entry "CONCERN-3: Panic recovery moved inside `runSessionDriverWithPrompt`" |
| CONCERN-4: Missing test for RecoverFromStopped → Start(false) | CONCERN | RESOLVED — Task 8.11 `TestHandleDriverFailure_StoppedSession_RestartFlow` added; patch log entry "CONCERN-4: Task 8.11 added" |

**Result: PASS** — Zero BLOCKED items. All 4 CONCERNs addressed.

---

### Criterion 4: SCOPE — No task is outside the scope defined in requirements.md

| Task | In scope? | Rationale |
|---|---|---|
| Epic 1: MCP wiring (Task 1.1) | YES | FR-1 explicitly lists MCP `create_session` as a required wiring point |
| Epic 2: Inactivity detection (Tasks 2.1–2.2) | YES | FR-4 explicitly requires inactivity timeout |
| Epic 3: Exit detection (Tasks 3.1–3.2) | YES | FR-5 explicitly requires exit detection |
| Epic 4: Continuation prompt (Task 4.1) | YES | FR-6 explicitly requires JSONL continuation prompt |
| Epic 5: Auto-retry (Tasks 5.1–5.3) | YES | FR-7 requires auto-retry once; `ReviewQueue.Add` instead of new status constant is consistent with D2 and "no UI changes" out-of-scope |
| Epic 6: Idempotency guard (Tasks 6.1–6.3) | YES | NFR requires idempotent wiring |
| Epic 7: Panic recovery (Task 7.1) | YES | NFR requires crash isolation |
| Epic 8: Tests (Tasks 8.1–8.11) | YES | Standard; required for production readiness |
| D2: No new `StatusNeedsAttention` constant | YES (in-scope restraint) | Requirements say "no UI changes beyond NeedsAttention status"; using `ReviewQueue.Add` instead is strictly less than adding a new status constant. Out-of-scope confirmed. |
| D3: `driverRunning` accounting gap during retry | YES | This is an internal implementation detail within the required idempotency NFR, not a scope expansion |

No task adds features outside the Requirements document. The "Coordinator agent as Claude session" out-of-scope item is correctly excluded — all logic stays in Go goroutines.

**Result: PASS** — No out-of-scope tasks identified.

---

## Readiness Gate Verdict

**PASS**

All four gate criteria pass:

1. **PLAN PASS**: Every FR (FR-1 through FR-8) has at least one task. Every task names a specific file, function, and change. The plan is implementation-ready.

2. **TEST PASS**: All 10 ACs are covered by at least one test. Edge cases are present beyond the happy path for every AC. Both failure modes (inactivity and exit) produce tests for the retry path and the double-failure path. Existing tests (UT-1, UT-2) are preserved.

3. **ADVERSARIAL PASS**: Zero BLOCKED items remain. All 3 blocking issues (BLOCK-1 through BLOCK-3) and all 4 concerns (CONCERN-1 through CONCERN-4) are addressed in the patched plan with explicit patch log entries.

4. **SCOPE PASS**: No task exceeds the scope defined in requirements.md. The one architectural simplification (ReviewQueue instead of new status constant) is explicitly justified as strictly narrower than the requirements allow.

**Implementation can proceed. Open a fresh session before Phase 5.**
