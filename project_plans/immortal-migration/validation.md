# Validation Plan: Immortal Migration — Process Manager Modularization

Status: Ready for implementation
Updated: 2026-05-22
Inputs: requirements.md, plan.md (2026-05-22), adversarial-review.md (2026-05-22)
Adversarial verdict: CONCERNS (not BLOCKED — all three critical issues resolved in plan.md)

---

## Traceability Table

| Test ID | Type | Story | Requirement | Description | Pass Criteria |
|---------|------|-------|-------------|-------------|---------------|
| T-COMPILE-1 | COMPILE | 1.2 | R1, R5 | `var _ ProcessManager = (*TmuxBackend)(nil)` compiles | Zero compile errors; any missing method is a compilation failure |
| T-COMPILE-2 | COMPILE | 1.5 | R1, R5 | `go build ./...` after full Phase 1 migration | Exit code 0, no errors |
| T-COMPILE-3 | COMPILE | 2.1 | R3 | `var _ ProcessManager = (*NativeProcessManager)(nil)` compiles | Zero compile errors |
| T-LINT-1 | LINT | 2.2 | Constraint: norawexec | `make lint` passes including norawexec pass — Phase 2 `//nolint:norawexec` with justification comment present on PTY exec call | Exit code 0; norawexec violation count = 0 |
| T-UNIT-1 | UNIT | 1.2 | R1 | `TmuxBackend.GetSessionIdentifier()` returns same value as `TmuxManager.GetTmuxSessionName()` — value correctness for reconciliation callers | `assert.Equal(t, "staplersquad_my-session", b.GetSessionIdentifier())` passes |
| T-UNIT-2 | UNIT | 1.2 | R1, R5 | Each ProcessManager method on TmuxBackend delegates to inner TmuxManager — mock-based, one sub-case per method (~28 methods) | Mock call counters all ≥ 1; no method silently no-ops; `var _ ProcessManager = (*mockTmuxManager)(nil)` pattern validates completeness |
| T-UNIT-3 | UNIT | 1.3 | R4 | Backend factory routes `BackendTmux` ("tmux") → `*TmuxBackend` | `_, ok := pm.(*TmuxBackend); assert.True(t, ok)` |
| T-UNIT-4a | UNIT | 1.3 | R4, Constraint: backwards-compat | Backend factory uses "tmux" default when `defaultBackend` = "" or NoopProvider registered | Returns `*TmuxBackend`; does not panic |
| T-UNIT-4b | UNIT | 1.3 | R4 | Backend factory panics with explicit message when flag = "native" (Phase 1 gate) | `assert.Panics(t, func() { NewProcessManager(..., BackendNative, ...) })` |
| T-UNIT-5 | UNIT | 1.4 | R4, Constraint: backwards-compat | Config round-trip: `process_manager_backend` field survives `json.Marshal` / `json.Unmarshal` | `assert.Equal(t, "tmux", out.ProcessManagerBackend)` |
| T-UNIT-6 | UNIT | 1.4 | Constraint: backwards-compat | Config backwards-compat: empty `process_manager_backend` field unmarshals to `""` (caller supplies default, not the struct) | `assert.Equal(t, "", cfg.ProcessManagerBackend)` after `json.Unmarshal([]byte("{}"), &cfg)` |
| T-UNIT-7 | UNIT | 2.2 | R3 | `NativeProcessManager.Start()` allocates a PTY — `ptm` field is non-nil after `Start()` | `mgr.ptm != nil` after successful `Start(t.TempDir())` |
| T-UNIT-8 | UNIT | 2.3 | R3 | `NativeProcessManager` restart loop — process exits, restart goroutine relaunches within 2 seconds with a new PID | `pid1 := mgr.cmd.Process.Pid; _ = syscall.Kill(pid1, syscall.SIGKILL); time.Sleep(2*time.Second); assert.NotEqual(t, pid1, mgr.cmd.Process.Pid)` |
| T-UNIT-9 | UNIT | 2.3, 2.4 | R3 | `NativeProcessManager.Stop()` (via `Close()`) cancels restart loop — no goroutine leak after Close() | After `mgr.Close()`, runtime.NumGoroutine() returns to baseline within 500 ms; no new calls to supervise() |
| T-UNIT-10 | UNIT | 2.4 | R1 | `NativeProcessManager.GetSessionIdentifier()` returns stable non-empty string set at construction | `assert.Equal(t, "test-session", mgr.GetSessionIdentifier())` |
| T-UNIT-11 | UNIT | 2.5 | R4 | Backend factory routes `BackendNative` ("native") → `*NativeProcessManager` after Phase 2 removes panic | `_, ok := pm.(*NativeProcessManager); assert.True(t, ok)` |
| T-INTEGRATION-1 | INTEGRATION | 2.5, 2.6 | R3 | Create session with config `process_manager_backend: "native"`, verify session is alive after `Start()` | `mgr.IsAlive()` returns true; `mgr.GetSessionIdentifier()` non-empty |
| T-REGRESSION-1 | INTEGRATION | 1.5, 1.7 | R5 | `go test ./session/... -count=1` — all existing session tests pass after Phase 1 field rename | Zero new failures vs. baseline on main; `comprehensive_session_creation_test.go` line 226 fix compiles and passes |
| T-REGRESSION-2 | LINT | 1.5 | R5 | No call site of `i.tmuxManager.X()` remains in session/ — grep returns zero | `grep -rn "\.tmuxManager\." session/ --include="*.go"` exits with no matches |
| T-REGRESSION-3 | LINT | 1.5 | R5 | No call site of `GetTmuxSessionName()` remains in non-wrapper code (only `Instance.GetTmuxSessionName()` wrapper and `TmuxBackend.GetSessionIdentifier()` delegation are permitted) | `grep -rn "GetTmuxSessionName" session/ --include="*.go"` shows only 2 occurrences: TmuxBackend delegation + Instance wrapper |
| T-MANUAL-1 | MANUAL | 2.3, 2.6 | R3 | Start session with `process_manager_backend: "native"`, kill the supervised process (`kill -9 <pid>`), verify session status returns to Running in UI | Session list shows Running after ~2 s; log shows restart event |

---

## Phase 1 Test Details

### T-COMPILE-1: TmuxBackend compile-time interface check

File: `session/tmux_backend_test.go` (or `session/tmux_backend.go` as a package-level var)

```go
// compile-time check — zero-value compile proof
var _ ProcessManager = (*TmuxBackend)(nil)
```

Run: `go build ./session/...`
Expected: exit 0
Fail signal: compile error listing missing methods — exact list shows what was omitted

---

### T-COMPILE-2: Full build after Phase 1 migration

Run: `go build ./...`
Expected: exit 0, no errors
Fail signal: any compile error in instance.go, backend_factory.go, main.go, or config/config.go

---

### T-COMPILE-3: NativeProcessManager compile-time interface check (Phase 2)

File: `session/native_process_manager.go`

```go
var _ ProcessManager = (*NativeProcessManager)(nil)
```

Run: `go build ./session/...`
Expected: exit 0
Fail signal: compile error listing unimplemented methods

---

### T-LINT-1: norawexec compliance (Phase 2 primary, Phase 1 must not regress)

Run: `make lint`
Expected: exit 0
Key check: `tools/lint/norawexec` pass finds zero violations
Phase 1: no new bare `exec.Command` / `exec.CommandContext` introduced → trivially passes
Phase 2: `session/native_process_manager.go` PTY exec call must have `//nolint:norawexec` with justification comment matching the pattern:
```go
cmd := exec.Command(...) //nolint:norawexec long-running PTY process; WaitDelay and ManagedProcess are pipe-based only, not PTY-compatible
```

---

### T-UNIT-1: TmuxBackend.GetSessionIdentifier() value correctness

File: `session/tmux_backend_test.go`

```go
func TestTmuxBackend_GetSessionIdentifier_MatchesTmuxSessionName(t *testing.T) {
    mock := &mockTmuxManager{tmuxSessionName: "staplersquad_my-session"}
    b := NewTmuxBackend(mock)
    assert.Equal(t, "staplersquad_my-session", b.GetSessionIdentifier())
    assert.Equal(t, 1, mock.getSessionNameCalls, "must delegate to GetTmuxSessionName")
}
```

Run: `go test ./session/... -run TestTmuxBackend_GetSessionIdentifier`
Expected: PASS
Tracing: reconciliation callers in `review_queue_poller.go` (lines 406, 550, 576) call
`inst.GetTmuxSessionName()` on `*Instance`; that method wraps `i.processManager.GetSessionIdentifier()`.
Value must be identical to the old `TmuxProcessManager.GetTmuxSessionName()` output.

---

### T-UNIT-2: TmuxBackend delegation coverage (~28 sub-cases)

File: `session/tmux_backend_test.go`

Pattern for each method:
```go
func TestTmuxBackend_DelegatesIsAlive(t *testing.T) {
    mock := &mockTmuxManager{isAliveReturn: true}
    b := NewTmuxBackend(mock)
    assert.True(t, b.IsAlive())
    assert.Equal(t, 1, mock.isAliveCalls)
}

func TestTmuxBackend_DelegatesHasSession(t *testing.T) {
    mock := &mockTmuxManager{hasSessionReturn: true}
    b := NewTmuxBackend(mock)
    assert.True(t, b.HasSession())
}

func TestTmuxBackend_DelegatesGetCurrentWorkingDirectory(t *testing.T) {
    mock := &mockTmuxManager{cwdReturn: "/home/user/project"}
    b := NewTmuxBackend(mock)
    cwd, err := b.GetCurrentWorkingDirectory()
    assert.NoError(t, err)
    assert.Equal(t, "/home/user/project", cwd)
}
```

Required sub-cases (one per ProcessManager method):
Start, RestoreWithWorkDir, Close, IsAlive, GetSessionIdentifier, HasSession,
GetCurrentWorkingDirectory, GetPTY, SendKeys, TapEnter, SendPromptWithEnter,
CapturePaneContent, CapturePaneContentRaw, CapturePaneContentWithOptions, CaptureViewport,
GetCursorPosition, GetPaneDimensions, SetWindowSize, SetDetachedSize, RefreshClient,
GetPanePID, HasUpdated, FilterBanners, HasMeaningfulContent,
StartControlMode, StopControlMode, SubscribeToControlModeUpdates, UnsubscribeFromControlModeUpdates,
Attach, DetachSafely, SetOnExitCallback, ResetExitOnce

Mock struct:
```go
type mockTmuxManager struct {
    TmuxManager // embed for methods not explicitly tested
    isAliveReturn      bool
    isAliveCalls       int
    hasSessionReturn   bool
    tmuxSessionName    string
    getSessionNameCalls int
    cwdReturn          string
    sendKeysReturn     int
    lastSendKeysInput  string
    // ... one field per method
}
func (m *mockTmuxManager) IsAlive() bool        { m.isAliveCalls++; return m.isAliveReturn }
func (m *mockTmuxManager) HasSession() bool      { return m.hasSessionReturn }
func (m *mockTmuxManager) GetTmuxSessionName() string { m.getSessionNameCalls++; return m.tmuxSessionName }
func (m *mockTmuxManager) SendKeys(k string) (int, error) {
    m.lastSendKeysInput = k; return m.sendKeysReturn, nil
}
// ... one implementation per method under test
```

Run: `go test ./session/... -run TestTmuxBackend_Delegates`
Expected: all sub-cases PASS

---

### T-UNIT-3: Factory routes "tmux" → TmuxBackend

File: `session/backend_factory_test.go`

```go
func TestNewProcessManager_ReturnsTmuxBackend_WhenFlagIsTmux(t *testing.T) {
    RegisterBackendProvider(BackendTmux)
    pm := NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{
        SessionName: "test", Prefix: "test_", ServerSocket: "",
    })
    _, ok := pm.(*TmuxBackend)
    assert.True(t, ok, "expected *TmuxBackend when flag is 'tmux'")
}
```

Run: `go test ./session/... -run TestNewProcessManager_ReturnsTmuxBackend`
Expected: PASS

---

### T-UNIT-4a: Factory default fallback (backwards-compat)

File: `session/backend_factory_test.go`

```go
func TestNewProcessManager_UsesDefault_WhenProviderUnregistered(t *testing.T) {
    // Seed with noop to simulate fresh startup with no provider registered
    openfeature.SetProvider(openfeature.NoopProvider{})
    pm := NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{
        SessionName: "test", Prefix: "test_",
    })
    _, ok := pm.(*TmuxBackend)
    assert.True(t, ok, "expected *TmuxBackend default when provider unregistered")
}
```

Run: `go test ./session/... -run TestNewProcessManager_UsesDefault`
Expected: PASS

---

### T-UNIT-4b: Factory panics for "native" in Phase 1

File: `session/backend_factory_test.go`

```go
func TestNewProcessManager_PanicsForNative_UntilPhase2(t *testing.T) {
    RegisterBackendProvider(BackendNative)
    assert.Panics(t, func() {
        NewProcessManager(context.Background(), BackendNative, ProcessManagerOptions{})
    }, "native backend should panic in Phase 1 with an explicit message")
}
```

Run: `go test ./session/... -run TestNewProcessManager_PanicsForNative`
Expected: PASS (Phase 1 only — test deleted or updated when Phase 2 removes panic)

---

### T-UNIT-5: Config round-trip

File: `config/config_test.go`

```go
func TestConfig_ProcessManagerBackend_RoundTrip(t *testing.T) {
    cfg := Config{ProcessManagerBackend: "tmux"}
    data, err := json.Marshal(cfg)
    require.NoError(t, err)
    var out Config
    require.NoError(t, json.Unmarshal(data, &out))
    assert.Equal(t, "tmux", out.ProcessManagerBackend)
}
```

Run: `go test ./config/... -run TestConfig_ProcessManagerBackend`
Expected: PASS

---

### T-UNIT-6: Config backwards-compat

File: `config/config_test.go`

```go
func TestConfig_ProcessManagerBackend_DefaultsToEmpty(t *testing.T) {
    var cfg Config
    require.NoError(t, json.Unmarshal([]byte(`{}`), &cfg))
    assert.Equal(t, "", cfg.ProcessManagerBackend,
        "empty JSON must leave field empty; main.go supplies the 'tmux' default")
}
```

Run: `go test ./config/... -run TestConfig_ProcessManagerBackend_DefaultsToEmpty`
Expected: PASS

---

### T-UNIT-7: NativeProcessManager.Start() allocates PTY (Phase 2)

File: `session/native_process_manager_test.go`

```go
func TestNativeProcessManager_StartAllocatesPTY(t *testing.T) {
    if testing.Short() {
        t.Skip("requires PTY allocation")
    }
    mgr := NewNativeProcessManager(ProcessManagerOptions{
        SessionName: "test-pty",
        Program:     "bash",
        Args:        []string{"-c", "sleep 600"},
    })
    err := mgr.Start(t.TempDir())
    require.NoError(t, err)
    defer mgr.Close()
    assert.NotNil(t, mgr.ptm, "PTY master fd must be allocated after Start()")
}
```

Run: `go test ./session/... -run TestNativeProcessManager_StartAllocatesPTY`
Expected: PASS (macOS and Linux)

---

### T-UNIT-8: NativeProcessManager crash-restart (Phase 2 — primary supervision test)

File: `session/native_process_manager_test.go`

```go
func TestNativeProcessManagerRestartsAfterKill(t *testing.T) {
    if testing.Short() {
        t.Skip("requires process supervision; takes ~2s")
    }
    mgr := NewNativeProcessManager(ProcessManagerOptions{
        SessionName: "test-restart",
        Program:     "bash",
        Args:        []string{"-c", "echo alive; sleep 600"},
    })
    err := mgr.Start(t.TempDir())
    require.NoError(t, err)
    defer mgr.Close()

    pid1 := mgr.cmd.Process.Pid
    require.Greater(t, pid1, 0)

    // Kill the supervised process
    err = syscall.Kill(pid1, syscall.SIGKILL)
    require.NoError(t, err)

    // Wait for restart (supervisor has 500 ms backoff + launch time)
    time.Sleep(2 * time.Second)

    mgr.mu.Lock()
    pid2 := mgr.cmd.Process.Pid
    mgr.mu.Unlock()

    assert.NotEqual(t, pid1, pid2,
        "process should have restarted with a new PID after kill")
    assert.Greater(t, pid2, 0)
}
```

Run: `go test ./session/... -run TestNativeProcessManagerRestartsAfterKill -timeout 30s`
Expected: PASS — pid1 != pid2, pid2 > 0

This test is the primary demonstration that crash-restart works (requirement R3).

---

### T-UNIT-9: NativeProcessManager.Close() stops restart loop (Phase 2)

File: `session/native_process_manager_test.go`

```go
func TestNativeProcessManager_CloseStopsRestartLoop(t *testing.T) {
    if testing.Short() {
        t.Skip("goroutine measurement")
    }
    baseline := runtime.NumGoroutine()
    mgr := NewNativeProcessManager(ProcessManagerOptions{
        SessionName: "test-close",
        Program:     "bash",
        Args:        []string{"-c", "sleep 600"},
    })
    err := mgr.Start(t.TempDir())
    require.NoError(t, err)

    err = mgr.Close()
    require.NoError(t, err)

    // Give goroutines time to exit
    time.Sleep(500 * time.Millisecond)

    // Should be back to baseline (±2 for scheduler variance)
    assert.InDelta(t, baseline, runtime.NumGoroutine(), 2,
        "goroutines should return to baseline after Close()")
}
```

Run: `go test ./session/... -run TestNativeProcessManager_CloseStopsRestartLoop -timeout 30s`
Expected: PASS — goroutine count returns to baseline

---

### T-UNIT-10: NativeProcessManager.GetSessionIdentifier() stability (Phase 2)

File: `session/native_process_manager_test.go`

```go
func TestNativeProcessManager_GetSessionIdentifier_IsStable(t *testing.T) {
    mgr := NewNativeProcessManager(ProcessManagerOptions{
        SessionName: "my-stable-session",
        Program:     "bash",
    })
    assert.Equal(t, "my-stable-session", mgr.GetSessionIdentifier())
    // Must return same value without Start() — construction-time stability
    assert.Equal(t, "my-stable-session", mgr.GetSessionIdentifier())
}
```

Run: `go test ./session/... -run TestNativeProcessManager_GetSessionIdentifier`
Expected: PASS

---

### T-UNIT-11: Factory routes "native" → NativeProcessManager (Phase 2)

File: `session/backend_factory_test.go`

```go
func TestNewProcessManager_ReturnsNativeProcessManager_WhenFlagIsNative(t *testing.T) {
    RegisterBackendProvider(BackendNative)
    pm := NewProcessManager(context.Background(), BackendNative, ProcessManagerOptions{
        SessionName: "test-native",
    })
    _, ok := pm.(*NativeProcessManager)
    assert.True(t, ok, "expected *NativeProcessManager when flag is 'native' (Phase 2)")
}
```

Run: `go test ./session/... -run TestNewProcessManager_ReturnsNativeProcessManager`
Expected: PASS (Phase 2 only — replaces T-UNIT-4b)

---

### T-INTEGRATION-1: Native backend session is alive after Start (Phase 2)

File: `session/native_process_manager_test.go`

```go
func TestNativeProcessManager_IsAliveAfterStart(t *testing.T) {
    if testing.Short() {
        t.Skip("integration: requires PTY")
    }
    RegisterBackendProvider(BackendNative)
    pm := NewProcessManager(context.Background(), BackendNative, ProcessManagerOptions{
        SessionName: "integration-test",
        Program:     "bash",
        Args:        []string{"-i"},
    })
    native := pm.(*NativeProcessManager)
    err := pm.Start(t.TempDir())
    require.NoError(t, err)
    defer pm.Close()

    assert.True(t, pm.IsAlive(), "session must be alive after Start()")
    assert.NotEmpty(t, pm.GetSessionIdentifier())
    assert.NotNil(t, native.ptm, "PTY master must be allocated")
}
```

Run: `go test ./session/... -run TestNativeProcessManager_IsAliveAfterStart -timeout 30s`
Expected: PASS

---

### T-REGRESSION-1: Existing session tests pass after Phase 1

Run:
```bash
go test ./session/... -count=1 -timeout=120s
```

Expected: same pass count as on main branch before migration; zero new failures
Critical: `session/comprehensive_session_creation_test.go` line 226 must be updated to:
```go
// Before (breaks after field rename):
instance.tmuxManager.session = mockTmuxSession
// After (Story 1.7 fix):
instance.processManager.(*TmuxBackend).TmuxManager().(*TmuxProcessManager).SetSession(mockTmuxSession)
```

Note: the plan's preferred alternative is to add `TmuxBackend.SetTmuxSession()` as a test helper.
Either approach is acceptable; the test file must compile and the test must pass.

---

### T-REGRESSION-2: No `tmuxManager` call sites remain in session/

Run:
```bash
grep -rn "\.tmuxManager\." session/ --include="*.go"
```

Expected: zero matches
Run immediately after completing Story 1.5.
Any match means at least one call site was not renamed — a potential nil-deref at runtime.

---

### T-REGRESSION-3: GetTmuxSessionName() only in sanctioned locations

Run:
```bash
grep -rn "GetTmuxSessionName" session/ --include="*.go"
```

Expected: exactly 2 matches:
1. `session/tmux_backend.go` — `func (b *TmuxBackend) GetSessionIdentifier() string { return b.mgr.GetTmuxSessionName() }`
2. `session/instance.go` — `func (i *Instance) GetTmuxSessionName() string { return i.processManager.GetSessionIdentifier() }` (the public wrapper)

Any other match is a missed migration.

---

### T-MANUAL-1: Native backend crash-restart end-to-end (Phase 2)

Prerequisites: Phase 2 complete, `process_manager_backend: "native"` in config.json

Steps:
1. Start stapler-squad: `./stapler-squad`
2. Create a session (omnibar → Directory → `/tmp`)
3. Identify the supervised bash PID from logs or UI: `grep "Starting session" ~/.stapler-squad/logs/stapler-squad.log`
4. Kill it: `kill -9 <pid>`
5. Wait ~2 seconds
6. Observe session status in UI: must return to Running
7. Observe log: must show restart event

Pass: session status shows Running; log shows at least one restart entry
Fail: session stays Dead/Stopped; or no log entry; or UI shows error

---

## Readiness Gate Checks

### Criterion 1: Artifact Completeness

- [x] `project_plans/immortal-migration/requirements.md` exists and is active (Status: Active)
- [x] `project_plans/immortal-migration/plan.md` exists (Status: Updated, 2026-05-22)
- [x] `project_plans/immortal-migration/adversarial-review.md` exists (Verdict: CONCERNS, not BLOCKED)
- [x] `project_plans/immortal-migration/validation.md` — this document
- [x] plan.md incorporates adversarial review fixes: HasSession() in interface, GetCurrentWorkingDirectory() in interface, Session()/SetSession() type-assert strategy documented

### Criterion 2: Must-Have Requirements Coverage

| Requirement | Tests |
|-------------|-------|
| R1: Session lifecycle abstraction (create, stop, pause, resume through interface) | T-COMPILE-1, T-COMPILE-2, T-UNIT-1, T-UNIT-2, T-REGRESSION-1 |
| R2: Terminal streaming abstraction (PTY/output to web UI) | T-UNIT-2 (StartControlMode, Subscribe/Unsubscribe delegation), T-UNIT-7, T-INTEGRATION-1 |
| R3: Process supervision and restart (native backend actually restarts) | T-UNIT-7, T-UNIT-8, T-UNIT-9, T-INTEGRATION-1, T-MANUAL-1 |
| R4: Config-driven backend selection | T-UNIT-3, T-UNIT-4a, T-UNIT-4b, T-UNIT-5, T-UNIT-6, T-UNIT-11 |

All 4 Must-Have requirements: COVERED (4 of 4)

### Criterion 3: Story Coverage

| Story | Tests |
|-------|-------|
| 1.1 Create session/process_manager.go | T-COMPILE-1, T-COMPILE-2 (interface definition verified by compilation) |
| 1.2 Create session/tmux_backend.go | T-COMPILE-1, T-UNIT-1, T-UNIT-2 |
| 1.3 OpenFeature factory | T-UNIT-3, T-UNIT-4a, T-UNIT-4b |
| 1.4 Config wiring + main.go | T-UNIT-5, T-UNIT-6 |
| 1.5 instance.go field rename + 89 call sites | T-COMPILE-2, T-REGRESSION-1, T-REGRESSION-2, T-REGRESSION-3 |
| 1.6 Remove Session()/SetSession() from TmuxManager interface | T-COMPILE-2, T-REGRESSION-2 |
| 1.7 Fix comprehensive_session_creation_test.go | T-REGRESSION-1 |
| 2.1 NativeProcessManager struct | T-COMPILE-3 |
| 2.2 Start() / PTY launch | T-UNIT-7, T-LINT-1 |
| 2.3 supervise() restart loop | T-UNIT-8, T-UNIT-9 |
| 2.4 Key ProcessManager methods on NativeProcessManager | T-UNIT-10, T-INTEGRATION-1 |
| 2.5 Wire NativeProcessManager in factory | T-UNIT-11, T-INTEGRATION-1 |
| 2.6 Test / manual verification of crash-restart | T-UNIT-8, T-MANUAL-1 |

All 13 stories: COVERED

### Criterion 4: Adversarial Review Verdict

Adversarial verdict: CONCERNS (not BLOCKED)
Critical issues from adversarial review and their test coverage:
- Issue #1 (Session()/SetSession() strategy) → T-COMPILE-2, T-REGRESSION-1: compilation + test suite confirm type-assert pattern works
- Issue #2 (GetCurrentWorkingDirectory missing from interface) → T-UNIT-2 sub-case for GetCurrentWorkingDirectory delegation
- Issue #3 (HasSession() missing from interface) → T-UNIT-2 sub-case for HasSession() delegation
- Issue #7 (DoesSessionExist line 580 semantics) → T-REGRESSION-1 covers the full test suite; developer must explicitly verify line 580 semantics during Story 1.5 implementation

Verdict: NOT BLOCKED — proceed to implementation.

---

## Test Pyramid Summary

| Type | Count | Command |
|------|-------|---------|
| COMPILE | 3 (T-COMPILE-1, T-COMPILE-2, T-COMPILE-3) | `go build ./...` |
| LINT | 2 (T-LINT-1, T-REGRESSION-2, T-REGRESSION-3) | `make lint`, `grep` |
| UNIT — adapter delegation | ~28 sub-cases (T-UNIT-2) | `go test ./session/... -run TestTmuxBackend_Delegates` |
| UNIT — factory routing | 4 (T-UNIT-3, T-UNIT-4a, T-UNIT-4b, T-UNIT-11) | `go test ./session/... -run TestNewProcessManager` |
| UNIT — config | 2 (T-UNIT-5, T-UNIT-6) | `go test ./config/...` |
| UNIT — NativeProcessManager | 5 (T-UNIT-7, T-UNIT-8, T-UNIT-9, T-UNIT-10, T-UNIT-1) | `go test ./session/... -run TestNativeProcessManager` |
| INTEGRATION | 2 (T-REGRESSION-1, T-INTEGRATION-1) | `go test ./session/... -count=1` |
| GREP/REGRESSION | 2 (T-REGRESSION-2, T-REGRESSION-3) | `grep` commands |
| MANUAL | 1 (T-MANUAL-1) | Manual verification |

Total: ~47 test cases + 3 compile checks + 1 regression suite + 2 grep checks + 1 manual verification

---

## Pass Criteria (all must hold before claiming Phase 1 complete)

- [ ] T-COMPILE-1: `var _ ProcessManager = (*TmuxBackend)(nil)` in tmux_backend.go compiles
- [ ] T-COMPILE-2: `go build ./...` exits 0
- [ ] T-UNIT-1: `GetSessionIdentifier()` value matches old `GetTmuxSessionName()` output
- [ ] T-UNIT-2: All ~28 TmuxBackend delegation sub-cases pass
- [ ] T-UNIT-3: Factory "tmux" → `*TmuxBackend`
- [ ] T-UNIT-4a: Factory default fallback returns `*TmuxBackend`
- [ ] T-UNIT-5: Config round-trip for `process_manager_backend` field
- [ ] T-UNIT-6: Empty config field → empty string (not "tmux")
- [ ] T-LINT-1: `make lint` passes (norawexec, Phase 1 — no new violations)
- [ ] T-REGRESSION-1: `go test ./session/... -count=1` — zero new failures; line 226 fix confirmed
- [ ] T-REGRESSION-2: `grep -rn "\.tmuxManager\." session/` → zero matches
- [ ] T-REGRESSION-3: `grep -rn "GetTmuxSessionName" session/` → exactly 2 matches

## Pass Criteria (Phase 2 gate — all must hold before claiming Phase 2 complete)

- [ ] T-COMPILE-3: `var _ ProcessManager = (*NativeProcessManager)(nil)` compiles
- [ ] T-LINT-1: `make lint` passes with `//nolint:norawexec` justification comment on PTY exec
- [ ] T-UNIT-7: PTY master non-nil after Start()
- [ ] T-UNIT-8: Crash-restart test passes — new PID within 2 s of kill
- [ ] T-UNIT-9: Close() stops restart loop — goroutine baseline restored
- [ ] T-UNIT-10: GetSessionIdentifier() stable across calls
- [ ] T-UNIT-11: Factory "native" → `*NativeProcessManager`
- [ ] T-INTEGRATION-1: Native session IsAlive() after Start()
- [ ] T-MANUAL-1: Kill-and-observe restart in running application

---

## Readiness Gate Verdict

- [x] requirements.md + plan.md + validation.md exist and are consistent
- [x] Every Must-Have requirement maps to ≥ 1 test (4 of 4 requirements covered)
- [x] Every story in the plan maps to ≥ 1 test (13 of 13 stories covered)
- [x] Adversarial review verdict is CONCERNS, not BLOCKED; all critical issues resolved in plan.md

Verdict: **PASS** — ready for a fresh implementation session.

---

## Notes for Implementation Session

1. Open a fresh session. Do not carry planning context into implementation.
2. Start at `project_plans/immortal-migration/plan.md` — the Implementation Order section (step 1–13) is the authoritative sequence.
3. Run `go build ./...` after each file change. This catches missing method implementations before they accumulate.
4. Story 1.5 is the largest change (~89 call sites). Verify `grep -c "tmuxManager" session/instance.go` returns 0 before marking it done.
5. Line 580 of instance.go: explicitly verify that `IsAlive()` semantics are equivalent to `DoesSessionExist()` for pre-start instances before substituting. Read both implementations in `session/tmux_process_manager.go`.
6. The test mock for `TmuxManager` must embed the full `TmuxManager` interface to avoid implementing all 30+ methods in every test. Pattern: `type mockTmuxManager struct { TmuxManager; ... }`.
7. T-UNIT-8 and T-UNIT-9 run real processes — do not run with `-short` in CI unless skipped. Mark these with `if testing.Short() { t.Skip(...) }`.
