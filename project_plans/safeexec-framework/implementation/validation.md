# safeexec-framework: Validation Plan

Plan date: 2026-05-05
Status: Ready for implementation
Linked plan: `implementation/plan.md`
Linked requirements: `requirements.md`

---

## 1. Requirement-to-Test Traceability Matrix

| Requirement | ID | Description Summary | Test IDs |
|---|---|---|---|
| FR-1 | Short-Lived Command API | Builder API, WaitDelay always set, output capture | T-UNIT-001, T-UNIT-002, T-UNIT-003, T-UNIT-004, T-UNIT-005, T-UNIT-006, T-INT-001, T-INT-002 |
| FR-2 | Process Group Management | Setpgid + SIGKILL to grandchildren | T-UNIT-007, T-UNIT-008, T-UNIT-009, T-INT-003, T-INT-004, T-GUARD-001, T-GUARD-004 |
| FR-3 | ManagedProcess Lifecycle | Start/Stop/Wait/PID/IsAlive, finalizer | T-UNIT-010, T-UNIT-011, T-UNIT-012, T-UNIT-013, T-UNIT-014, T-UNIT-015, T-INT-005, T-INT-006, T-GUARD-002, T-GUARD-003 |
| FR-4 | Output Capture & Streaming | Stdout/Stderr io.Reader, ScanOutput | T-UNIT-016, T-UNIT-017, T-UNIT-018, T-INT-007 |
| FR-5 | Resource Limits | RlimitConfig struct, Linux enforcement, macOS stub | T-UNIT-019, T-UNIT-020, T-INT-008, T-INT-009 |
| FR-6 | Audit Logging | AuditEntry emitted, opt-in via context, fields correct | T-UNIT-021, T-UNIT-022, T-UNIT-023, T-UNIT-024, T-INT-010 |
| FR-7 | Timeout & Deadline Propagation | WithTimeout, WaitDelay never zero, context cancel | T-UNIT-005, T-UNIT-025, T-GUARD-001, T-INT-002 |
| FR-8 | Lint Enforcement | nounmanagedprocess analyzer | T-LINT-001, T-LINT-002, T-LINT-003 |
| NFR-1 | Backwards Compatibility | CommandContext signature unchanged | T-UNIT-026 |
| NFR-2 | Zero External Dependencies | No new third-party imports | (static check — CI `go mod verify`) |
| NFR-3 | Platform Support | Build tags, macOS/Linux behavior differences | T-UNIT-019, T-UNIT-020, T-INT-008, T-INT-009 |
| NFR-4 | Testability | Interface-backed types, fakes usable | T-UNIT-021 (uses fake hook) |
| NFR-5 | Performance | Audit overhead < 1ms | T-UNIT-027 |
| SC-1 | make lint-custom passes | Zero norawexec violations | T-LINT-001, T-LINT-002, T-LINT-003 |
| SC-2 | Zero raw exec in production code | No exec.Command outside executor/ | T-LINT-001, T-LINT-002 |
| SC-3 | ManagedProcess replaces 2+ nolint sites | Sites 1 and 2 migrated | T-INT-011, T-INT-012 |
| SC-4 | Process group active for all commands | Setpgid true by default | T-UNIT-007, T-UNIT-009, T-INT-003, T-INT-004 |
| SC-5 | Audit logging works in integration runs | Entry emitted with correct fields | T-INT-010 |
| SC-6 | Unit test coverage > 80% | Measured by go test -cover | (CI coverage gate) |
| SC-7 | go vet + golangci-lint pass | Zero analyzer findings | (CI lint gate) |

Coverage: **17 of 17** requirements and success criteria covered (14 FR/NFR + 7 SC, with SC-6/SC-7/NFR-2 verified by CI gates rather than individual test cases).

---

## 2. Unit Tests

Unit tests live in `executor/` alongside production code. They must not spawn real subprocesses. Where process behavior is needed, compile `testdata/helper/main.go` once via `TestMain` and reference the binary path. All unit tests must run in `go test ./executor/... -short`.

---

```
ID: T-UNIT-001
Requirement: FR-1
Name: ShortLivedCmd_Run_appliesWaitDelay
Setup: Inspect the exec.Cmd produced by ShortLivedCmd.build() via an unexported test helper
       that returns the built *exec.Cmd without executing it.
Action: Call build() on New(ctx, "echo", []string{"hi"})
Assert: cmd.WaitDelay > 0 (specifically: == safeexec.DefaultWaitDelay)
Must fail against pre-fix code: Yes — without the fix, WaitDelay would be 0.
```

```
ID: T-UNIT-002
Requirement: FR-1
Name: ShortLivedCmd_Output_capturesStdout
Setup: Use the testdata/helper binary compiled via TestMain; invoke it with a flag that
       causes it to print a known string to stdout and exit 0.
Action: New(ctx, helperBin, []string{"--print", "hello"}).Output()
Assert: returned []byte == []byte("hello\n"); error == nil
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-003
Requirement: FR-1
Name: ShortLivedCmd_CombinedOutput_mergesStderr
Setup: testdata/helper binary prints "out" to stdout and "err" to stderr.
Action: New(ctx, helperBin, []string{"--print-both"}).CombinedOutput()
Assert: output contains both "out" and "err"; no error
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-004
Requirement: FR-1
Name: ShortLivedCmd_Run_nonZeroExit_returnsExitError
Setup: testdata/helper with --exit-code=1 flag.
Action: New(ctx, helperBin, []string{"--exit-code", "1"}).Run()
Assert: err != nil; errors.As(err, &exec.ExitError{}); exitErr.ExitCode() == 1
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-005
Requirement: FR-1, FR-7
Name: ShortLivedCmd_WithTimeout_cancelsBeforeCompletion
Setup: testdata/helper with --sleep=10s flag (would block forever without timeout).
       Derive a context with no deadline.
Action: New(ctx, helperBin, []string{"--sleep", "10s"}, WithTimeout(50*time.Millisecond)).Run()
Assert: err != nil; errors.Is(err, context.DeadlineExceeded) OR
        errors.As(err, &exec.ExitError{}) with signal exit (acceptable: killed by timeout)
        — test must complete within 500ms
Must fail against pre-fix code: Yes — without WithTimeout routing, process runs unbounded.
```

```
ID: T-UNIT-006
Requirement: FR-1
Name: ShortLivedCmd_WithDir_setsWorkingDirectory
Setup: Create a temp directory. testdata/helper --print-cwd flag prints os.Getwd().
Action: New(ctx, helperBin, []string{"--print-cwd"}, WithDir(tmpDir)).Output()
Assert: strings.TrimSpace(string(out)) == tmpDir
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-007
Requirement: FR-2
Name: ShortLivedCmd_build_setpgidTrueByDefault
Setup: None.
Action: cmd := New(ctx, "echo", nil).build()
Assert: cmd.SysProcAttr != nil; cmd.SysProcAttr.Setpgid == true
Must fail against pre-fix code: Yes — raw exec.CommandContext does not set Setpgid.
```

```
ID: T-UNIT-008
Requirement: FR-2
Name: ShortLivedCmd_WithoutProcessGroup_noSetpgid
Setup: None.
Action: cmd := New(ctx, "echo", nil, WithoutProcessGroup()).build()
Assert: cmd.SysProcAttr == nil || cmd.SysProcAttr.Setpgid == false
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-009
Requirement: FR-2
Name: StartProcess_setpgidTrueByDefault
Setup: Use a fake processConfig-inspection path: add an exported test-only accessor
       BuildForTest() *exec.Cmd on ManagedProcess, or test via SysProcAttr inspection
       right after StartProcess returns (before any exec).
       Alternative: start a no-op process (true/cat), immediately Stop, inspect Cmd.SysProcAttr.
Action: p, _ := StartProcess(ctx, helperBin, []string{"--sleep", "10s"})
        defer p.Stop()
        inspect p.cmd.SysProcAttr.Setpgid
Assert: SysProcAttr.Setpgid == true
Must fail against pre-fix code: Yes — raw cmd.Start() without Setpgid.
```

```
ID: T-UNIT-010
Requirement: FR-3
Name: ManagedProcess_stateMachine_notStarted
Setup: Attempt to call PID() before StartProcess — verifies zero-value behavior.
       This test documents the API contract: ManagedProcess must be obtained via StartProcess.
Action: var p ManagedProcess; pid := p.PID()
Assert: pid == 0 (zero value); IsAlive() returns false without panic
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-011
Requirement: FR-3
Name: StartProcess_commandNotFound_returnsError
Setup: Use a path that does not exist.
Action: _, err := StartProcess(ctx, "/nonexistent/binary/path", nil)
Assert: err != nil; err wraps exec.ErrNotFound or os.ErrNotExist (or similar)
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-012
Requirement: FR-3
Name: ManagedProcess_PID_nonZeroAfterStart
Setup: testdata/helper --sleep 10s.
Action: p, err := StartProcess(ctx, helperBin, []string{"--sleep", "10s"})
        defer p.Stop()
Assert: err == nil; p.PID() > 0
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-013
Requirement: FR-3
Name: ManagedProcess_IsAlive_trueWhileRunning_falseAfterStop
Setup: testdata/helper --sleep 10s.
Action: Start → IsAlive() → Stop() → IsAlive()
Assert: IsAlive() returns true before Stop; returns false after Stop completes
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-014
Requirement: FR-3
Name: ManagedProcess_Wait_returnsAfterNaturalExit
Setup: testdata/helper --exit-code=0 (exits immediately).
Action: p, _ := StartProcess(...); err := p.Wait()
Assert: err == nil; completes within 1s
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-015
Requirement: FR-3
Name: ManagedProcess_WithEnv_appendsToEnvironment
Setup: testdata/helper --print-env=SAFEEXEC_TEST_VAR flag prints the env var value.
Action: StartProcess(ctx, helperBin, []string{"--print-env", "SAFEEXEC_TEST_VAR"},
            WithProcessEnv("SAFEEXEC_TEST_VAR", "expected_value"))
        Read Stdout().
Assert: output contains "expected_value"
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-016
Requirement: FR-4
Name: ManagedProcess_Stdout_readsOutput
Setup: testdata/helper --print "line1\nline2\n".
Action: p, _ := StartProcess(...); r := p.Stdout(); data, _ := io.ReadAll(r)
Assert: string(data) == "line1\nline2\n"
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-017
Requirement: FR-4
Name: ManagedProcess_Stderr_readsErrors
Setup: testdata/helper --print-stderr="error output".
Action: p, _ := StartProcess(...); r := p.Stderr(); data, _ := io.ReadAll(r)
Assert: strings.Contains(string(data), "error output")
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-018
Requirement: FR-4
Name: ManagedProcess_ScanLines_callsCallbackPerLine
Setup: testdata/helper --print "line1\nline2\nline3\n".
Action: var lines []string; p.ScanLines(ctx, func(l string) { lines = append(lines, l) })
Assert: lines == []string{"line1", "line2", "line3"}; ScanLines returns nil (EOF)
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-019
Requirement: FR-5, NFR-3
Name: RlimitConfig_zerovalueIsValid
Setup: Construct RlimitConfig{} (zero value).
Action: Pass to applyRlimits (or equivalent). On Linux, verify no error.
        On non-Linux, verify the stub returns nil without error.
Assert: No error; no panics on either platform
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-020
Requirement: FR-5, NFR-3
Name: RlimitConfig_nonLinuxStub_returnsNil
Setup: Build with !linux tag (or mock the platform dispatch).
       Construct RlimitConfig{MaxOpenFiles: 10}.
Action: err := applyRlimits(cmd, cfg) on non-Linux stub
Assert: err == nil; no modification to cmd.SysProcAttr
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-021
Requirement: FR-6, NFR-4
Name: WithAuditHook_receivesEntryOnCompletion
Setup: Implement a fake AuditHook capturing all received AuditEntry values in a slice.
       testdata/helper --exit-code=0 --print="hi".
Action: ctx := WithAuditHook(context.Background(), fakeHook)
        New(ctx, helperBin, []string{"--exit-code", "0"}).Run()
Assert: fakeHook.entries has exactly 1 entry;
        entry.ExitCode == 0;
        entry.Command[0] == helperBin;
        entry.Duration > 0;
        entry.StartTime is non-zero;
        entry.KilledByCtx == false
Must fail against pre-fix code: Yes — no audit hook mechanism exists pre-implementation.
```

```
ID: T-UNIT-022
Requirement: FR-6
Name: WithAuditHook_noHook_noPanic
Setup: Context without an AuditHook value.
Action: New(context.Background(), helperBin, []string{"--exit-code", "0"}).Run()
Assert: No panic; err == nil
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-023
Requirement: FR-6
Name: LoggingAuditHook_emitsAtDebugOnSuccess
Setup: Create a test slog.Handler that captures log records. Attach to LoggingAuditHook.
Action: hook.OnExec(AuditEntry{ExitCode: 0, KilledByCtx: false, Command: []string{"echo"}})
Assert: Captured record has Level == slog.LevelDebug
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-024
Requirement: FR-6
Name: LoggingAuditHook_escalatesToInfoOnNonZeroExit
Setup: Same test slog.Handler as T-UNIT-023.
Action: hook.OnExec(AuditEntry{ExitCode: 1, KilledByCtx: false, Command: []string{"false"}})
Assert: Captured record has Level == slog.LevelInfo
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-UNIT-025
Requirement: FR-7
Name: ShortLivedCmd_WaitDelayAlwaysNonZero
Setup: Build multiple ShortLivedCmd instances with various option combinations
       (no options, WithTimeout, WithDir, WithReplaceEnv).
Action: Inspect cmd.WaitDelay on each built *exec.Cmd via build().
Assert: cmd.WaitDelay > 0 for ALL configurations
Must fail against pre-fix code: Yes — safeexec predecessor relied on CommandContext default
        which leaves WaitDelay at 0 without explicit setting.
```

```
ID: T-UNIT-026
Requirement: NFR-1
Name: CommandContext_signatureUnchanged
Setup: Import executor/safeexec in a test file.
Action: Compile-time check: call safeexec.CommandContext(ctx, "echo", "hi")
        where the third+ params are variadic strings.
Assert: Compiles without error; cmd.WaitDelay == safeexec.DefaultWaitDelay
Must fail against pre-fix code: N/A (backwards compat check).
```

```
ID: T-UNIT-027
Requirement: NFR-5
Name: AuditHook_overheadUnder1ms
Setup: Fake AuditHook with no-op OnExec. BenchmarkAuditEmit function that calls
       emitAudit 1000 times in a loop.
Action: go test -bench=BenchmarkAuditEmit -benchmem ./executor/...
Assert: ns/op < 1,000,000 (1ms) per call; zero allocations in emitAudit hot path
        (when hook is nil — the no-hook fast path)
Must fail against pre-fix code: N/A (performance regression gate).
```

---

## 3. Integration Tests

Integration tests are gated with `if testing.Short() { t.Skip() }` at the top of each test function. They live in `executor/executor_integration_test.go` with build tag `//go:build integration`. Run with:

```
go test -tags=integration ./executor/... -v -race
```

All integration tests use real subprocesses. They require macOS or Linux.

---

```
ID: T-INT-001
Requirement: FR-1
Name: ShortLivedCmd_Run_realEchoCommand
Setup: testing.Short() skip.
Action: New(context.Background(), "echo", []string{"integration-test"}).Run()
Assert: err == nil; completes in < 2s
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-INT-002
Requirement: FR-1, FR-7
Name: ShortLivedCmd_WaitDelay_preventsZombieOnContextCancel
Setup: testing.Short() skip.
       ctx, cancel := context.WithCancel(context.Background())
       Start a process that sleeps for 30 seconds.
       Cancel context immediately.
Action: New(ctx, "sleep", []string{"30"}).Run()
        cancel()
        Wait 500ms, then check /proc/<pid>/stat (Linux) or ps output (macOS).
Assert: Process is no longer in zombie state; Wait returned without blocking indefinitely.
        Test must complete within 3 seconds (WaitDelay = 2s + margin).
Must fail against pre-fix code: Yes — without WaitDelay, goroutine blocks in cmd.Wait()
        until pipes drain, which can be indefinitely.
```

```
ID: T-INT-003
Requirement: FR-2
Name: ShortLivedCmd_processGroup_grandchildKilled
Setup: testing.Short() skip.
       Shell script: sh -c "sleep 60 & wait".
       This script forks a grandchild (sleep 60) and waits for it.
Action: ctx, cancel := context.WithCancel(context.Background())
        New(ctx, "sh", []string{"-c", "sleep 60 & wait"}).Run() in goroutine.
        cancel() after 100ms.
        Wait 500ms.
        Check for surviving sleep processes via `ps aux | grep sleep`.
Assert: No "sleep 60" processes remain in the process table.
Must fail against pre-fix code: Yes — without Setpgid + group SIGKILL, grandchild
        survives as orphan after the direct child is killed.
```

```
ID: T-INT-004
Requirement: FR-2
Name: ManagedProcess_Stop_killsProcessGroup
Setup: testing.Short() skip.
       Record grandchild PID by having the shell script print "grandchild PID=<pid>".
Action: p, _ := StartProcess(ctx, "sh", []string{"-c", "sleep 60 & echo PID=$!; wait"})
        Read first line from Stdout() to get grandchild PID.
        p.Stop()
        Check if grandchild PID is still alive.
Assert: Grandchild is not alive after Stop() returns.
        stop-to-dead time < gracePeriod + 1s.
Must fail against pre-fix code: Yes — without process group kill, grandchild survives.
```

```
ID: T-INT-005
Requirement: FR-3
Name: ManagedProcess_lifecycle_startStopWait
Setup: testing.Short() skip.
Action: p, err := StartProcess(ctx, "sleep", []string{"30"})
        assert err == nil
        assert p.IsAlive() == true
        assert p.PID() > 0
        err = p.Stop()
        assert err == nil
        assert p.IsAlive() == false
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-INT-006
Requirement: FR-3
Name: ManagedProcess_Wait_naturalExit_returnsExitCode
Setup: testing.Short() skip. Use testdata/helper --exit-code=42.
Action: p, _ := StartProcess(ctx, helperBin, []string{"--exit-code", "42"})
        err := p.Wait()
Assert: err is *exec.ExitError with exit code 42
Must fail against pre-fix code: N/A (new code).
```

```
ID: T-INT-007
Requirement: FR-4
Name: ManagedProcess_outputStreaming_correctAfterExit
Setup: testing.Short() skip.
       testdata/helper --print-lines=3 outputs 3 numbered lines then exits.
Action: p, _ := StartProcess(ctx, helperBin, []string{"--print-lines", "3"})
        data, _ := io.ReadAll(p.Stdout())
        _ = p.Wait()
Assert: string(data) == "line 1\nline 2\nline 3\n"
        No deadlock; io.ReadAll returns after EOF.
Must fail against pre-fix code: N/A. Also validates os.Pipe choice:
        io.Pipe would risk deadlock if buffer fills.
```

```
ID: T-INT-008
Requirement: FR-5, NFR-3
Name: RlimitConfig_NOFILE_enforced_on_linux
Setup: testing.Short() skip. Linux build tag.
       Script tries to open 500 file descriptors in a loop.
Action: New(ctx, "bash", []string{"-c", "for i in $(seq 500); do exec 3>&-; done; echo fail"},
            WithRlimits(RlimitConfig{MaxOpenFiles: 10})).CombinedOutput()
Assert: Process exits with non-zero code (EMFILE or signal);
        OR output indicates file open failure before completion.
Must fail against pre-fix code: Yes — without rlimit, the script completes successfully.
```

```
ID: T-INT-009
Requirement: FR-5, NFR-3
Name: RlimitConfig_onMacOS_noError_noEffect
Setup: testing.Short() skip. Non-Linux build.
Action: New(ctx, "echo", []string{"hi"}, WithRlimits(RlimitConfig{MaxOpenFiles: 10})).Run()
Assert: err == nil (stub is a no-op; the rlimit is silently ignored on macOS)
Must fail against pre-fix code: N/A. Ensures the cross-platform stub compiles and is safe.
```

```
ID: T-INT-010
Requirement: FR-6, SC-5
Name: AuditHook_emittedOnCompletion_fieldsCorrect
Setup: testing.Short() skip.
       Fake AuditHook that captures entries.
       ctx := WithAuditHook(context.Background(), fakeHook)
Action: p, _ := StartProcess(ctx, helperBin, []string{"--exit-code", "0"})
        p.Wait()
Assert: fakeHook has exactly 1 entry;
        entry.PID == p.PID();
        entry.ExitCode == 0;
        entry.Duration > 0 && entry.Duration < 5*time.Second;
        entry.StartTime before time.Now();
        entry.Command[0] == helperBin;
        entry.KilledByStop == false && entry.KilledByCtx == false
Must fail against pre-fix code: Yes — audit system does not exist pre-implementation.
```

```
ID: T-INT-011
Requirement: SC-3
Name: ExternalTmuxStreamer_controlMode_usesStartProcess
Setup: testing.Short() skip. Requires tmux available on PATH.
       Initialize an ExternalTmuxStreamer with a test tmux session.
Action: Start control mode; verify the underlying process is a ManagedProcess (not *exec.Cmd).
        Call Stop(). Verify no zombie in process table after 500ms.
Assert: No regression in startControlMode behavior;
        s.controlModeProcess != nil && s.controlModeCmd == nil (old field removed)
Must fail against pre-fix code: N/A (migration test, validates the migration in Epic 5).
```

```
ID: T-INT-012
Requirement: SC-3
Name: ServerRegistry_controlMode_usesStartProcess
Setup: testing.Short() skip. Requires tmux.
       Initialize a tmux server registry.
Action: Start registry; verify the tmux process is a *ManagedProcess.
        Stop; verify process table is clean.
Assert: No regression in existing tmux command routing behavior;
        server_registry uses ManagedProcess for the tmux server process.
Must fail against pre-fix code: N/A (migration test, validates the migration in Epic 5).
```

---

## 4. Pitfall Guard Tests

Pitfall guard tests are labeled `T-GUARD-NNN` and are specifically designed to fail against code that does NOT have the defensive fix. Each test documents an exact sharp edge identified during design and must continue to pass on every future change.

---

```
ID: T-GUARD-001
Requirement: FR-2, FR-7
Name: GrandchildPipe_waitDelayFiresEvenWhenGrandchildHoldsPipe
Setup: testing.Short() skip.
       Shell script: sh -c "sleep 60 | cat".
       The grandchild (cat) holds open the write end of the pipe.
       Without WaitDelay, cmd.Wait() blocks forever because the pipe never closes.
Action: ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
        defer cancel()
        err := New(ctx, "sh", []string{"-c", "sleep 60 | cat"}).Run()
Assert: Run() returns within 3 seconds (WaitDelay kicks in);
        err is non-nil (killed by timeout or WaitDelay);
        err is NOT context.DeadlineExceeded waiting forever
Must fail against pre-fix code: Yes — without WaitDelay, Run() blocks indefinitely
        because the grandchild holds the pipe open after the parent child exits.
```

```
ID: T-GUARD-002
Requirement: FR-3, plan §6.4
Name: ErrWaitDelay_notReturnedToCaller
Setup: Start a process that traps SIGTERM and ignores it (never exits on SIGTERM).
       Set a very short grace period so WaitDelay fires quickly.
Action: p, _ := StartProcess(ctx, helperBin, []string{"--trap-sigterm", "--sleep", "60"},
            WithGracePeriod(200*time.Millisecond))
        err := p.Stop()
Assert: err == nil OR err is a typed ErrKilledByTimeout sentinel;
        errors.Is(err, exec.ErrWaitDelay) == false
        (ErrWaitDelay must NOT leak through to the caller)
Must fail against pre-fix code: Yes — without filtering in the reaper goroutine,
        Stop() returns exec.ErrWaitDelay, which callers don't expect.
```

```
ID: T-GUARD-003
Requirement: FR-3
Name: ManagedProcess_Stop_idempotent
Setup: Start a process that exits immediately.
Action: p, _ := StartProcess(ctx, helperBin, []string{"--exit-code", "0"})
        _ = p.Wait()
        err1 := p.Stop()
        err2 := p.Stop()
        // Also test concurrent Stop from two goroutines
        var wg sync.WaitGroup
        wg.Add(2)
        go func() { defer wg.Done(); _ = p.Stop() }()
        go func() { defer wg.Done(); _ = p.Stop() }()
        wg.Wait()
Assert: No panic; no deadlock; err1 and err2 are nil or identical non-nil errors;
        goroutine count does not grow
Must fail against pre-fix code: Yes — without atomic.Bool guard, concurrent Stop()
        calls could double-cancel or panic on closed channel.
```

```
ID: T-GUARD-004
Requirement: FR-2
Name: ProcessGroupSIGKILL_onContextCancel_notJustDirectChild
Setup: testing.Short() skip.
       Shell script records its PID and forks a long-lived grandchild:
       sh -c "echo grandchild_pid=$(sleep 60 & echo $!); wait"
       Read grandchild PID from stdout.
Action: ctx, cancel := context.WithCancel(context.Background())
        p, _ := StartProcess(ctx, "sh", args); readGrandchildPID from Stdout()
        cancel() // triggers context cancellation → Stop path
        time.Sleep(200*time.Millisecond)
        check if grandchild PID is still in process table
Assert: grandchild is NOT alive (kill(-pgid, SIGKILL) reached it)
Must fail against pre-fix code: Yes — SIGKILL only to cmd.Process.Pid leaves
        the process group alive; grandchildren survive.
```

---

## 5. Lint Rule Tests

Lint rule tests exercise the `nounmanagedprocess` analyzer implemented in `tools/lint/nounmanagedprocess/`. They follow the `go/analysis/analysistest` pattern used by the existing `norawexec` analyzer.

---

```
ID: T-LINT-001
Requirement: FR-8, SC-1, SC-2
Name: NounmanagedprocessAnalyzer_flags_cmdStart_withoutNolint
Setup: Create a testdata package `testdata/src/flagged/flagged.go` with:
       import "os/exec"
       func bad() {
           cmd := exec.Command("echo", "hi")
           cmd.Start() // want "use ManagedProcess instead of cmd.Start()"
       }
Action: Run the analyzer via analysistest.Run(t, "./testdata", analyzer)
Assert: Analyzer reports exactly one diagnostic on the cmd.Start() line.
        Diagnostic message matches the expected pattern.
Must fail against pre-fix code: Yes — without the analyzer, this violation is silent.
```

```
ID: T-LINT-002
Requirement: FR-8, SC-1, SC-2
Name: NounmanagedprocessAnalyzer_silent_withNolintJustification
Setup: Create testdata/src/exempted/exempted.go:
       import "os/exec"
       func legitUse() {
           cmd := exec.Command("tmux", "-C", "attach-session")
           //nolint:nounmanagedprocess pty.Start() requires raw *exec.Cmd; ManagedProcess cannot be used here
           cmd.Start()
       }
Action: Run analyzer via analysistest.Run(t, "./testdata", analyzer)
Assert: Zero diagnostics emitted (nolint comment suppresses the finding).
Must fail against pre-fix code: N/A (new rule). Verifies the escape hatch works.
```

```
ID: T-LINT-003
Requirement: FR-8, SC-1
Name: NounmanagedprocessAnalyzer_silent_forManagedProcessStart
Setup: Create testdata/src/managed/managed.go:
       import "executor"
       func good(ctx context.Context) {
           p, err := executor.StartProcess(ctx, "sleep", []string{"10"})
           _ = p; _ = err
       }
Action: Run analyzer via analysistest.Run(t, "./testdata", analyzer)
Assert: Zero diagnostics (StartProcess is the approved API — analyzer only flags
        raw exec.Cmd.Start() calls).
Must fail against pre-fix code: N/A. Validates the analyzer does not produce false positives.
```

---

## 6. Success Criteria Mapping

Each of the 7 success criteria from `requirements.md` maps to one or more tests that verify it.

| SC | Description | Verification Test(s) | Pass Condition |
|---|---|---|---|
| SC-1 | `make lint-custom` passes, zero norawexec violations | T-LINT-001, T-LINT-002 | Analyzer emits zero diagnostics on the migrated codebase |
| SC-2 | Zero raw `exec.CommandContext`/`exec.Command` in production outside `executor/` | T-LINT-001 + CI `make lint-custom` | norawexec findings == 0 after migration |
| SC-3 | ManagedProcess replaces at least 2 nolint sites | T-INT-011, T-INT-012 | Integration tests pass without `//nolint:norawexec` on Sites 1 and 2 |
| SC-4 | Process group management active for all commands | T-UNIT-007, T-UNIT-009, T-INT-003, T-INT-004, T-GUARD-004 | Setpgid confirmed in unit; grandchild killed in integration |
| SC-5 | Audit logging works in integration runs | T-INT-010, T-UNIT-021 | AuditEntry received with correct PID, exit code, duration fields |
| SC-6 | Unit test coverage > 80% | CI coverage gate (`go test -cover`) | `go test -cover ./executor/... \| grep -E "coverage: [89][0-9]\|100"` |
| SC-7 | `go vet` and `golangci-lint` pass cleanly | CI lint gate | Both tools exit 0 with no findings on `./executor/...` |

---

## 7. Test Infrastructure

### 7.1 `testdata/helper/main.go`

Compile once in `TestMain`. Supports these flags (add more as needed):

| Flag | Behavior |
|---|---|
| `--print <string>` | Writes string to stdout and exits 0 |
| `--print-stderr <string>` | Writes string to stderr and exits 0 |
| `--print-both` | Writes "out" to stdout, "err" to stderr, exits 0 |
| `--print-cwd` | Prints `os.Getwd()` to stdout and exits 0 |
| `--print-env <VAR>` | Prints `os.Getenv(VAR)` to stdout and exits 0 |
| `--print-lines <n>` | Prints n lines ("line 1\n"..."line n\n") then exits 0 |
| `--exit-code <n>` | Exits with code n immediately |
| `--sleep <duration>` | Sleeps for duration then exits 0 |
| `--trap-sigterm --sleep <d>` | Installs SIGTERM trap (ignores it), sleeps for d |

Compile snippet in TestMain:

```go
func TestMain(m *testing.M) {
    exe, err := buildHelper()
    if err != nil {
        fmt.Fprintf(os.Stderr, "failed to build testdata/helper: %v\n", err)
        os.Exit(1)
    }
    helperBin = exe
    os.Exit(m.Run())
}

func buildHelper() (string, error) {
    tmpDir, _ := os.MkdirTemp("", "safeexec-helper-*")
    exe := filepath.Join(tmpDir, "helper")
    cmd := exec.Command("go", "build", "-o", exe, "./testdata/helper")
    return exe, cmd.Run()
}
```

### 7.2 Build Tags and Test Files

| File | Build tag | Contains |
|---|---|---|
| `executor/shortlived_test.go` | (none) | T-UNIT-001 through T-UNIT-008, T-UNIT-025, T-UNIT-026 |
| `executor/managed_process_test.go` | (none) | T-UNIT-009 through T-UNIT-018 |
| `executor/audit_test.go` | (none) | T-UNIT-021 through T-UNIT-024, T-UNIT-027 |
| `executor/rlimit_test.go` | (none) | T-UNIT-019, T-UNIT-020 |
| `executor/rlimit_linux_test.go` | `//go:build linux` | T-INT-008 |
| `executor/executor_integration_test.go` | `//go:build integration` | T-INT-001 through T-INT-012 |
| `executor/pitfall_test.go` | `//go:build integration` | T-GUARD-001 through T-GUARD-004 |
| `tools/lint/nounmanagedprocess/analyzer_test.go` | (none) | T-LINT-001 through T-LINT-003 |

### 7.3 Race Detector

Run all non-integration tests under the race detector:

```
go test -race ./executor/...
```

Run `TestManagedProcess_Stop_idempotent` (T-GUARD-003) with `-count=20` to surface races:

```
go test -race -count=20 ./executor/... -run TestManagedProcess_Stop_idempotent
```

### 7.4 goroutine Leak Detection

Use `go.uber.org/goleak` in `TestMain` to detect goroutine leaks across all tests:

```go
func TestMain(m *testing.M) {
    // ... build helper ...
    goleak.VerifyTestMain(m)
}
```

This catches forgotten reaper goroutines that linger after test processes exit.

---

## 8. Coverage Targets

| Package | Target | How to Verify |
|---|---|---|
| `executor/` (all new files) | ≥ 80% statement coverage | `go test -cover ./executor/...` |
| `executor/safeexec/` (new files only) | ≥ 80% | `go test -cover ./executor/safeexec/...` |
| `tools/lint/nounmanagedprocess/` | ≥ 70% (analyzer logic) | `go test -cover ./tools/lint/nounmanagedprocess/...` |

---

## 9. Test Count Summary

| Type | Count | File(s) |
|---|---|---|
| Unit (T-UNIT-NNN) | 27 | shortlived_test.go, managed_process_test.go, audit_test.go, rlimit_test.go |
| Integration (T-INT-NNN) | 12 | executor_integration_test.go (incl. linux-only T-INT-008) |
| Pitfall guard (T-GUARD-NNN) | 4 | pitfall_test.go |
| Lint (T-LINT-NNN) | 3 | tools/lint/nounmanagedprocess/analyzer_test.go |
| **Total** | **46** | |

**Requirements coverage:** 17 of 17 requirements and success criteria covered (FR-1 through FR-8, NFR-1 through NFR-5, SC-1 through SC-7). NFR-2 (zero dependencies) and SC-6/SC-7 (coverage and vet gates) are enforced via CI rather than individual test cases.
