# Validation Plan — tmux Subprocess Optimization

**Phase**: 4 (Validation)
**Feature**: `docs/tasks/tmux-subprocess-optimization.md`
**Date**: 2026-04-24

---

## 1. Requirements Traceability Matrix

| ID | Requirement | Test(s) |
|----|-------------|---------|
| R1 | `IsDirty()` never spawns subprocess within 15s TTL window | `TestIsDirtyCache_HitWithinTTL`, `TestIsDirtyCache_ConcurrentCallsCoalesceToOneSubprocess` |
| R2 | `IsDirty()` always skips subprocess when `ClaudeController` reports active | `TestIsDirtyCache_SkipWhenClaudeActive`, `TestIsDirtyCache_ActiveSignalPreventsCallEvenOnCacheMiss` |
| R3 | `CheckGHAuth()` concurrent callers coalesce — only 1 subprocess fired | `TestCheckGHAuth_SingleflightCoalescesConcurrentCallers` |
| R4 | `CheckGHAuth()` returns cached result within 5min TTL without subprocess | `TestCheckGHAuth_HitWithinTTL`, `TestCheckGHAuth_RefreshAfterTTLExpiry` |
| R5 | `Preview()` returns cached result within 500ms TTL | `TestPreviewCache_HitWithinTTL`, `TestPreviewCache_ExpiresAfter500ms` |
| R6 | CM command dispatch sends command over stdin, receives response via `%begin`/`%end` | `TestCMDispatch_SingleCommand`, `TestCMDispatch_ResponseParsedFromBeginEnd` |
| R7 | FIFO ordering maintained under concurrent CM command requests | `TestCMDispatch_ConcurrentCommandsArriveFIFO`, `TestCMDispatch_TwoCommandsQueuedInOrder` |
| R8 | CM command fallback to subprocess when control mode is not running | `TestCMDispatch_FallbackWhenControlModeNil`, `TestGetPaneDimensions_FallbackToSubprocess` |
| R9 | Feature flag `STAPLER_SQUAD_CM_COMMANDS` correctly gates CM vs subprocess path | `TestCMFeatureFlag_OffUsesSubprocess`, `TestCMFeatureFlag_OnUsesCMPath` |
| R10 | Adaptive poller backs off to 8s when no sessions pending | `TestAdaptivePoller_BackoffToIdleInterval` |
| R11 | Adaptive poller snaps back to 2s on `EventApprovalResponse` | `TestAdaptivePoller_SnapOnApprovalResponse` |

---

## 2. Unit Tests

### 2.1 `IsDirty()` TTL Cache

**File**: `session/git/worktree_git_test.go`

All tests in this section use a `MockExecutor` (counting subprocess invocations) rather than a real git repository. The mock executor is already available in the `executor` package; tests in the `git` package can inject it via the `cmdExec` field or a new `WithExecutor` test constructor.

---

**`TestIsDirtyCache_MissOnFirstCall`**

- Assert subprocess is called exactly once on the first `IsDirty()` call.
- Assert returned value reflects the mock output.
- Setup: `GitWorktree` with counting mock executor that returns `"M file.txt\n"`.
- Teardown: none.

---

**`TestIsDirtyCache_HitWithinTTL`**

- Call `IsDirty()` twice within the 15s TTL window.
- Assert subprocess is called exactly once total across both calls.
- Assert second call returns same value as first without invoking the executor.
- Setup: Mock executor with call counter; clock not manipulated (both calls within milliseconds).

---

**`TestIsDirtyCache_ExpiryAfter15s`**

- Call `IsDirty()` once; advance the `isDirtyCacheTime` field backward by 16 seconds (direct struct field manipulation — acceptable in package-internal test); call again.
- Assert subprocess is called exactly twice.
- Setup: `GitWorktree` constructed with exported test helper or package-internal test that accesses the struct directly (file is in package `git`).

---

**`TestIsDirtyCache_SkipWhenClaudeActive`**

- Construct `GitWorktree` with empty cache (no prior call).
- Pass active-signal callback that returns `true`.
- Assert subprocess is never called; assert return value is `false` (or cached last-known).
- Note: this test validates the `isActive func() bool` callback design; the exact skip-return behavior (return `false` or return last cache) must be decided during implementation and reflected here.

---

**`TestIsDirtyCache_ActiveSignalPreventsCallEvenOnCacheMiss`**

- Expire the cache (zero the `isDirtyCacheTime`); pass `isActive = func() bool { return true }`.
- Assert zero subprocess calls.
- This covers R2 explicitly: even on a cache miss, an active signal is a hard skip.

---

**`TestIsDirtyCache_ConcurrentCallsCoalesceToOneSubprocess`**

- Launch 50 goroutines that each call `IsDirty()` simultaneously after invalidating the cache.
- Use a counting executor that sleeps 5ms to simulate subprocess latency.
- After all goroutines complete, assert subprocess call count is 1 (or at most 2 due to the write-lock double-check pattern).
- Setup: `sync.WaitGroup` to synchronize goroutine start; `sync/atomic` counter in mock executor.

---

### 2.2 `CheckGHAuth()` Singleflight + Atomic TTL

**File**: `github/client_cache_test.go` (new file; package `github`)

Tests require injecting a fake `gh` binary via `PATH` manipulation or by making the subprocess invocation injectable. The cleanest approach for the existing code is to temporarily swap `exec.LookPath` behavior. However, since the current `CheckGHAuth()` uses `exec.Command("gh", ...)` directly, these tests are best written by:

1. Extracting the actual `gh` call into an injectable `var ghAuthChecker = func() error { ... }` (implementation decision), or
2. Using a fake `gh` binary placed on a temp `PATH` during the test.

Option 2 requires no production code changes and is the recommended approach for initial tests. See Section 7 (Test Infrastructure Needs).

---

**`TestCheckGHAuth_HitWithinTTL`**

- Call `CheckGHAuth()` once (populates cache); call again within 5 minutes.
- Assert fake `gh` binary was executed exactly once.
- Setup: fake `gh` binary that succeeds; inject into `PATH`; record invocation count.

---

**`TestCheckGHAuth_RefreshAfterTTLExpiry`**

- Call `CheckGHAuth()` once; manually advance the `authState.expiry` field to the past (requires package-level atomic to be test-injectable); call again.
- Assert fake `gh` was executed exactly twice.
- Alternative: use `InvalidateGHAuthCache()` helper (if implemented per the plan) to force expiry without time manipulation.

---

**`TestCheckGHAuth_SingleflightCoalescesConcurrentCallers`**

- Block the fake `gh` binary briefly (50ms sleep) to create a window for concurrent calls.
- Launch 20 goroutines that call `CheckGHAuth()` simultaneously after invalidating the cache.
- Assert fake `gh` was invoked exactly once across all 20 calls.
- Assert all 20 goroutines received the same (nil) error.
- This is the critical R3 test. A bug here causes the 2.02s mutex delay to persist.

---

**`TestCheckGHAuth_SingleflightDeduplicatesInFlight`**

- Distinct from the sequential-hit test: this confirms singleflight works for callers that call `Do()` while the first call is still in progress (not just when the cache is already populated).
- Use a channel to hold the fake `gh` binary open until a second goroutine is confirmed to be waiting in `Do()`.
- Assert exactly 1 invocation regardless of the number of waiters.

---

**`TestCheckGHAuth_CachePropagatesErrorToAllWaiters`**

- Fake `gh` returns exit status 1 (auth failure).
- All concurrent singleflight waiters should receive a non-nil error.
- The error result should not be cached (or if cached, must be invalidated so the next call retries).
- Implementation decision required: whether failed auth is cached or always retried.

---

**`TestCheckGHAuth_InvalidateClearsCache`**

- Call `CheckGHAuth()` once (populates cache); call `InvalidateGHAuthCache()`; call again.
- Assert fake `gh` is invoked on the second call (cache was cleared).

---

### 2.3 `Preview()` Cache

**File**: `session/review_queue_poller_cache_test.go` (new file) or added to `session/review_queue_poller_test.go`

The existing `ReviewQueuePoller` tests already use a mock `Instance`-like structure (see `session/review_queue_poller_test.go`). These tests extend that pattern.

---

**`TestPreviewCache_HitWithinTTL`**

- Configure poller with a session that has no active `ClaudeController`.
- Trigger two poll ticks within 500ms.
- Assert `tmux capture-pane` subprocess is called exactly once.
- Setup: mock `tmuxManager` with a call counter on `CapturePaneContent()`.

---

**`TestPreviewCache_ExpiresAfter500ms`**

- Trigger poll tick (populates cache); wait 501ms; trigger second tick.
- Assert `CapturePaneContent()` was called exactly twice.

---

**`TestPreviewCache_NonControllerSessionsCached`**

- Verify that sessions without a `ClaudeController` (the previously uncached path) are now subject to the 500ms TTL.
- This is the most important correctness test for R5: the original code had a code path that bypassed caching for non-controller sessions.
- Setup: session with `claudeController == nil`; verify subprocess is skipped on second call within TTL.

---

**`TestPreviewCache_InvalidatedOnSessionRestart`**

- Simulate a session transitioning from `Paused` to `Running`.
- Assert that the next `Preview()` call after the transition invokes the subprocess (cache was cleared).
- Covers the cache invalidation correctness requirement from Story 1c task 3.

---

### 2.4 Control Mode State Machine

**File**: `session/tmux/control_mode_dispatch_test.go` (new file, package `tmux`)

These tests exercise the `processControlModeLine()` state machine and `sendCMCommand()` helper directly, without a real tmux process. They pipe fabricated lines through the parser.

The test helper creates a `TmuxSession` with `controlModeStdin` wired to an in-memory buffer (so `sendCMCommand()` can write to it) and then feeds lines directly to `processControlModeLine()` to simulate tmux responses.

---

**`TestCMDispatch_SingleCommand`**

- Call `sendCMCommand()` with a test command string.
- Feed `%begin 1 1 0`, `output-line`, `%end 1 1 0` to `processControlModeLine()`.
- Assert the channel returned by `sendCMCommand()` receives `cmdResult{body: "output-line"}`.

---

**`TestCMDispatch_ResponseParsedFromBeginEnd`**

- Feed a multi-line body (`line1\nline2\nline3`) between `%begin` and `%end`.
- Assert `cmdResult.body` contains all three lines concatenated correctly.
- Verifies R6.

---

**`TestCMDispatch_TwoCommandsQueuedInOrder`**

- Call `sendCMCommand()` twice before delivering any responses (simulating two commands queued).
- Feed `%begin`, `resp-A`, `%end`, then `%begin`, `resp-B`, `%end`.
- Assert first call's channel receives `resp-A` and second call's channel receives `resp-B` in order.
- Verifies R7 (FIFO ordering).

---

**`TestCMDispatch_ConcurrentCommandsArriveFIFO`**

- Launch 10 goroutines each calling `sendCMCommand()` with a unique payload.
- Feed 10 `%begin`/`%end` blocks in sequence (numbered to match send order).
- Assert each goroutine receives the correct response matching its send order.
- Verifies R7 under concurrent load.

---

**`TestCMDispatch_ErrorResponsePropagated`**

- Feed `%begin 1 1 0`, then `%error some error message`, then `%end 1 1 0` (or just `%error`).
- Assert `cmdResult.err` is non-nil and contains `"some error message"`.

---

**`TestCMDispatch_OutputNotificationDuringCommandDoesNotCorruptQueue`**

- Queue one command; feed `%begin 1 1 0`, then an `%output %0 terminal-data`, then `body-line`, then `%end 1 1 0`.
- Assert command response body contains only `body-line` (not the `%output` line).
- Assert `broadcastControlModeUpdate()` was called with `terminal-data` (subscribers still received output).
- This directly covers the known issue: `%output` arriving between `%begin` and `%end`.
- The implementation must not suppress `%output` notifications during command response windows.

---

**`TestCMDispatch_FallbackWhenControlModeNil`**

- Create a `TmuxSession` where `controlModeStdin` is nil.
- Call `sendCMCommand()`.
- Assert it returns `("", ErrControlModeNotRunning)` immediately without blocking.
- Verifies R8.

---

**`TestCMDispatch_StopDrainsInflightCommands`**

- Queue one command via `sendCMCommand()` (blocks on channel).
- Call `StopControlMode()` from another goroutine before feeding any response.
- Assert the blocked goroutine unblocks and receives `cmdResult{err: ErrControlModeStopped}` (or similar).
- Assert no goroutine leak (verified via `goleak` or timeout assertion).
- Covers the known "StopControlMode while command in-flight" concurrency issue.

---

**`TestCMDispatch_DoubleBeginResetsState`**

- Feed `%begin 1 1 0`, `partial-line`, then another `%begin 2 2 0` before `%end`.
- Assert the first pending command receives an error result (not corrupted data).
- Assert the state machine continues cleanly for subsequent commands.
- Covers the known "cmdBodyBuf corruption on unexpected %begin" issue.

---

**`TestCMFeatureFlag_OffUsesSubprocess`**

- Set `STAPLER_SQUAD_CM_COMMANDS=false` (or the atomic flag to false).
- Call `GetPaneDimensions()`.
- Assert the mock executor received a `display-message` subprocess call.
- Assert `controlModeStdin` was not written to.
- Verifies R9.

---

**`TestCMFeatureFlag_OnUsesCMPath`**

- Set `STAPLER_SQUAD_CM_COMMANDS=true`.
- Wire `controlModeStdin` to a capture buffer.
- Call `GetPaneDimensions()` while feeding a simulated CM response.
- Assert `controlModeStdin` received the `display-message` command.
- Assert mock executor was not called (no subprocess).
- Verifies R9.

---

**`TestGetPaneDimensions_FallbackToSubprocess`**

- `STAPLER_SQUAD_CM_COMMANDS=true` but `controlModeStdin` is nil (control mode not started yet).
- Call `GetPaneDimensions()`.
- Assert subprocess fallback fires (mock executor received the call).
- Verifies R8.

---

## 3. Integration Tests

### 3.1 TTL Caching With Real Subprocess

**File**: `session/git/worktree_git_integration_test.go` (new file, build tag `//go:build integration`)

**`TestIsDirtyIntegration_SubprocessCountWithRealWorktree`**

- Create a real git repository in `t.TempDir()`.
- Create a `GitWorktree` pointing to it.
- Call `IsDirty()` 10 times in 500ms (well within 15s TTL).
- Assert actual `git status --porcelain` subprocess invocations total 1 (measured via process tracing or a wrapper around `runGitCommand` that counts calls and delegates to real exec).
- Teardown: `t.Cleanup(func() { os.RemoveAll(worktreePath) })`.

---

**File**: `github/client_integration_test.go` (new file, build tag `//go:build integration`)

**`TestCheckGHAuthIntegration_SingleSubprocessForNConcurrentCallers`**

- Install a fake `gh` binary in a temp dir that writes its PID and timestamp to a log file on every invocation and exits 0.
- Prepend the temp dir to `PATH`; restore after test.
- Invalidate the auth cache; launch 20 goroutines that each call `CheckGHAuth()` simultaneously.
- After all goroutines complete, count lines in the fake `gh` log file.
- Assert line count is 1 (singleflight worked).

---

### 3.2 Control Mode Dispatch With Real tmux

These tests require a real tmux server and use the existing `testutil.CreateIsolatedTmuxServer` fixture (see `/Users/tylerstapler/IdeaProjects/stapler-squad/testutil/tmux.go:169`). All tests skip if `tmux` is not in `PATH`.

**File**: `session/tmux/control_mode_integration_test.go` (new file, build tag `//go:build integration`)

---

**`TestCMIntegration_GetPaneDimensions_MatchesSubprocess`**

- Create an isolated tmux server and a real session via `testutil.CreateIsolatedTmuxServer`.
- Start control mode (`StartControlMode()`).
- Call `GetPaneDimensions()` via the CM path.
- Call `GetPaneDimensions()` via the subprocess path.
- Assert both return identical width and height values.
- Verifies R6 for the pilot function.

---

**`TestCMIntegration_CapturePaneContent_MatchesSubprocess`**

- Send a known string to the pane using `SendKeys()`.
- Wait 100ms for tmux to render.
- Call `CapturePaneContent()` via CM path; call it again via subprocess path.
- Assert outputs are byte-for-byte identical.
- This is the highest-value correctness test for Story 2c.

---

**`TestCMIntegration_CapturePaneContentWithOptions_RangeFlags`**

- Send multi-line output; call `CapturePaneContentWithOptions("-5", "0")`.
- Assert only the last 5 lines are returned (confirms `-S`/`-E` flags work over CM stdin).
- Covers the open verification question from ADR-001.

---

**`TestCMIntegration_RefreshClient_NoError`**

- Call `RefreshClient()` via CM path on a real session.
- Assert no error returned and the terminal redraws (verify indirectly by checking `GetPaneDimensions()` returns consistent values afterward).
- Covers the `refresh-client` self-reference open question from Story 2d.

---

**`TestCMIntegration_FeatureFlagOff_SubprocessPathUsed`**

- Set `STAPLER_SQUAD_CM_COMMANDS=false`.
- Start control mode.
- Call `GetPaneDimensions()`.
- Assert the call succeeded and `controlModeStdin` was not written (verified by interposing a counting writer on `controlModeStdin`).
- Verifies R9 with a real tmux server.

---

### 3.3 Concurrency Stress

**File**: `session/tmux/control_mode_stress_test.go` (new file, build tag `//go:build stress`)

---

**`TestIsDirtyStress_100ConcurrentCallsWithin1TTLWindow`**

- Real git repository; 100 goroutines; all call `IsDirty()` within the same 15s TTL window.
- Assert total subprocess invocations ≤ 2 (1 expected; 2 allowed for the write-lock double-check race).
- Uses `sync/atomic` counter in a subprocess-counting wrapper around `runGitCommand`.

---

**`TestCMStress_20ConcurrentCommands_AllReceiveCorrectResponses`**

- Real tmux server with a running session.
- 20 goroutines each send a unique `display-message` command over CM.
- Assert all 20 receive a response with no timeout.
- Assert responses are semantically correct (not cross-contaminated).
- Assert no goroutine leaks (use `goleak` or assert goroutine count before/after).
- Verifies R7 under real concurrent load.

---

## 4. End-to-End Tests

**File**: `tests/subprocess_optimization_e2e_test.go` (new file, build tag `//go:build e2e`)

These require `tmux` and the full `stapler-squad` binary. They are not part of `make test` but are run as `make test-e2e` or manually.

---

**`TestE2E_SubprocessRateAtIdle`**

- Start stapler-squad with real tmux server; create 5 sessions (using the existing `testutil` helpers).
- Run `ReviewQueuePoller` for 10 seconds with profiling enabled.
- Count `os/exec.(*Cmd).Start` invocations using `runtime/trace` or by interposing a counter on `exec.Command`.
- Assert total subprocess spawns < 20 (baseline without caching: ~1380 for 10s × 5 sessions × 2s poll × 1 `IsDirty` + 1 `Preview` + `CheckGHAuth`).
- This is the primary success metric validation.

---

**`TestE2E_TerminalRenderWithCMDispatch`**

- Start stapler-squad with `STAPLER_SQUAD_CM_COMMANDS=true`.
- Open the web UI terminal for a session.
- Type a command that produces visible output.
- Assert terminal content is rendered correctly in the web UI (captured via the WebSocket streaming path).
- Assert no discrepancy warnings in logs (parallel-path comparison from Story 2a).

---

**`TestE2E_FeatureFlagRuntimeToggle`**

- Start with `STAPLER_SQUAD_CM_COMMANDS=false`; confirm subprocess path via log evidence.
- Restart with `STAPLER_SQUAD_CM_COMMANDS=true`; confirm CM path via log evidence.
- This is a manual verification step, not an automated test — document the log patterns to look for.

---

## 5. Property-Based Tests

**File**: `session/git/worktree_git_property_test.go` (new file, package `git`)

Using `testing/quick` (stdlib) or `pgregory.net/rapid` (if added as a dependency).

---

**Property: TTL Invariant**

For any TTL duration `T` in [1ms, 60s] and any `N` calls in a window of `k × T` elapsed time, the number of subprocess calls must be exactly `ceil(k)` (where `k = elapsed / T`).

```go
// TestIsDirtyProperty_SubprocessCountMatchesCeilElapsedOverTTL
// Uses rapid.Int() for elapsed time and rapid.Int() for TTL to generate cases.
// Manipulates isDirtyCacheTime directly to simulate elapsed time.
```

---

**Property: FIFO Invariant for CM Commands**

For any sequence of CM commands `[c1, c2, ..., cN]` sent in order, the responses arrive in the same order `[r1, r2, ..., rN]`.

```go
// TestCMDispatch_FIFOPropertyHolds
// Uses rapid.SliceOf(rapid.String()) to generate arbitrary command sequences.
// Feeds matching responses in order; asserts each goroutine's channel gets the matching response.
```

These are aspirational for Phase 2. If `rapid` is not an approved dependency, use `testing/quick`.

---

## 6. Known Risks and Edge Case Test Coverage

The following known issues from the feature plan map to specific tests:

| Known Issue | Severity | Covered By |
|---|---|---|
| `%output` notification arriving between `%begin` and `%end` | High | `TestCMDispatch_OutputNotificationDuringCommandDoesNotCorruptQueue` |
| `StopControlMode()` while command in-flight | High | `TestCMDispatch_StopDrainsInflightCommands` |
| `cmdBodyBuf` corruption on unexpected `%begin` | Medium | `TestCMDispatch_DoubleBeginResetsState` |
| `display-message` format string quoting over CM stdin | High (blocks 2b) | `TestCMIntegration_GetPaneDimensions_MatchesSubprocess` (interactive pre-check in Story 2a) |
| `refresh-client` self-reference when CM is the attached client | Medium | `TestCMIntegration_RefreshClient_NoError` |
| `pendingCmds` channel growth / goroutine leak on CM exit | Medium | `TestCMDispatch_StopDrainsInflightCommands`, `TestCMStress_20ConcurrentCommands_AllReceiveCorrectResponses` |
| `IsDirty()` stale dirty state for up to 15s after manual commit | Low (by design) | Not tested — documented in ADR-002 as accepted trade-off |
| `cmdBodyBuf` unbounded growth with large `capture-pane` output | Low | Not a correctness bug; validate with `TestCMIntegration_CapturePaneContent_MatchesSubprocess` using a large pane (1000+ lines) |

### Additional Edge Cases Without Corresponding Known Issues

**`TestIsDirtyCache_ActiveSignalTransitionMidTTL`**
- Cache is populated with `isActive = false`; before TTL expires, `isActive` becomes `true`.
- Assert the next call skips the subprocess even though TTL has not expired (active signal is a hard override, not a cache-key condition).

**`TestCheckGHAuth_GhNotInstalled`**
- `exec.LookPath("gh")` fails.
- Assert `CheckGHAuth()` returns the "not installed" error immediately without populating the cache.

**`TestPreviewCache_PausedSessionSkipped`**
- Session is in `Paused` state; `Preview()` should return `("", nil)` immediately without any subprocess or cache lookup.
- Regression guard for the `if !i.started || i.Status == Paused` guard at `session/instance.go:1242`.

---

## 7. Test Infrastructure Needs

### 7.1 Existing Infrastructure (Available Now)

| Infrastructure | Location | Used By |
|---|---|---|
| `testutil.CreateIsolatedTmuxServer` | `testutil/tmux.go:169` | All CM integration tests |
| `testutil.TmuxTestServer` | `testutil/tmux.go:160` | Session creation in integration tests |
| `MockPtyFactory` | `session/tmux/tmux_test.go:18` | tmux unit tests needing PTY |
| `MockCmdExec` | `session/tmux/tmux_test.go` | Subprocess call counting |
| `executor.MakeExecutor()` | `executor/` package | Can be wrapped for counting |
| EventBus | `server/events/bus_test.go` | Adaptive poller tests |

### 7.2 New Infrastructure Needed

**Fake `gh` binary for `CheckGHAuth` tests**

Create `testdata/fake-gh/main.go` (a small Go binary that records invocations to a temp file and exits 0). Build it as part of `TestMain` in `github/client_cache_test.go` using `os/exec` to `go build` into `t.TempDir()`. Prepend to `PATH` for the test duration.

```go
// Pattern:
func buildFakeGH(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()
    out := filepath.Join(dir, "gh")
    cmd := exec.Command("go", "build", "-o", out, "./testdata/fake-gh")
    require.NoError(t, cmd.Run())
    t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
    return out
}
```

The fake binary should:
1. Append a line to `$GH_INVOCATION_LOG` (env var set by the test) with timestamp + PID.
2. Sleep `$GH_DELAY_MS` milliseconds if set (for singleflight timing tests).
3. Exit with code `$GH_EXIT_CODE` (default 0).

---

**Subprocess call counter for `IsDirty` tests**

Create a test helper in `session/git/testing_helpers_test.go` (package-internal):

```go
// countingGitRunner wraps runGitCommand to count invocations.
type countingGitRunner struct {
    mu    sync.Mutex
    count int
    orig  func(dir string, args ...string) ([]byte, error)
}
```

Inject it by temporarily replacing the `runGitCommand` field (if extracted to a function variable) or by wrapping the executor passed to `GitWorktree`. The exact mechanism depends on the implementation of Story 1a.

---

**Claude-active signal injection for `IsDirty` tests**

The `isActive func() bool` callback parameter on `IsDirty()` (per the Story 1a design) can be injected directly in tests:

```go
isDirty, err := worktree.IsDirty(func() bool { return true }) // always active
isDirty, err := worktree.IsDirty(func() bool { return false }) // never active
```

No special infrastructure needed beyond the callback parameter existing.

---

**CM stdin capture buffer for unit tests**

For `processControlModeLine()` unit tests, wire `controlModeStdin` to an `io.Pipe()` or `bytes.Buffer` so tests can observe what `sendCMCommand()` writes without a real tmux process:

```go
// In test setup:
pr, pw := io.Pipe()
session.controlModeStdin = pw
// Feed responses by calling processControlModeLine() directly on the test goroutine.
// Read what sendCMCommand() wrote from pr in the assertion goroutine.
```

---

**Goroutine leak detection**

For `TestCMDispatch_StopDrainsInflightCommands` and stress tests, use `go.uber.org/goleak` (already in the dependency graph as part of `nilaway`). Add to test files as:

```go
func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}
```

---

### 7.3 No New tmux Server Fixture Needed

The existing `testutil.CreateIsolatedTmuxServer` is sufficient for all integration tests. It creates a unique tmux socket per test, registers a `t.Cleanup()` that kills the server, and provides `CreateSession()` for session creation. All integration tests must call:

```go
if _, err := exec.LookPath("tmux"); err != nil {
    t.Skip("tmux not available")
}
server := testutil.CreateIsolatedTmuxServer(t)
```

---

## 8. Test Execution Guidance

### Phase 1 Tests (TTL Caching)

Run after implementing Stories 1a, 1b, 1c:

```bash
# Unit tests (fast, no tmux required)
go test ./session/git/... -run TestIsDirtyCache
go test ./github/... -run TestCheckGHAuth
go test ./session/... -run TestPreviewCache

# Integration tests (requires tmux + real git)
go test -tags integration ./session/git/... -run TestIsDirtyIntegration
go test -tags integration ./github/... -run TestCheckGHAuthIntegration
```

### Phase 2 Tests (CM Dispatch)

Run after implementing Story 2a state machine:

```bash
# Unit tests (no tmux required — uses fake CM responses)
go test ./session/tmux/... -run TestCMDispatch

# Integration tests (requires tmux)
go test -tags integration ./session/tmux/... -run TestCMIntegration
```

### Phase 3 Tests (Adaptive Poller)

```bash
go test ./session/... -run TestAdaptivePoller
```

### Phase 1 Integration Checkpoint Verification

After Phase 1 merges, run the profiling sequence defined in the feature plan:

```bash
make restart-web-profile
# Wait 2 minutes with 5 active sessions
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > /tmp/goroutines.txt
grep "os/exec.*Start" /tmp/goroutines.txt | wc -l
# Must be < 50 (was 277)
```
