# Pitfalls Research: Resilience, Process Management & Goroutine Safety

## 1. Circuit Breaker Pattern (`executor/circuit_breaker.go`)

The codebase has a full `CircuitBreakerExecutor` wrapping an `Executor` interface:
- **States**: `CircuitClosed → CircuitOpen → CircuitHalfOpen → CircuitClosed`
- **Trip condition**: `FailureThreshold` consecutive failures (default 3)
- **Recovery**: exponential backoff with `RecoveryTimeout` base (default 30s) and optional `MaxRecoveryTimeout` cap
- **Customizable failure classification**: `IsFailure func(commandClass string, output []byte, err error) bool` — can distinguish "claude returned error" from "claude exited non-zero for a valid reason"

For the headless pool, using a circuit breaker per FeatureKey is appropriate:
- `FailureThreshold: 3` — rotate session after 3 consecutive errors
- `IsFailure`: treat non-zero exit as failure but NOT `context.Canceled` (caller cancelled, not claude's fault)
- **Do not** share one circuit breaker across all feature keys — each key should have independent state

Test pattern in `executor/circuit_breaker_test.go`:
```go
type mockExecutor struct {
    runFunc            func(cmd *exec.Cmd) error
    outputFunc         func(cmd *exec.Cmd) ([]byte, error)
    combinedOutputFunc func(cmd *exec.Cmd) ([]byte, error)
}
```
Use this same mock pattern for testing `ClaudeRunner` without spawning real processes.

## 2. Zombie Process Risks

### Problem: Grandchildren holding pipes open

`safeexec.CommandContext` sets `WaitDelay = 2s`. If `claude -p` spawns grandchildren (e.g., node scripts, tool processes) that hold stdout/stderr open, `cmd.Wait()` will block for up to `WaitDelay` after SIGKILL. This is bounded and acceptable.

### Problem: Multiple `cmd.Wait()` calls

The `ManagedProcess` design explicitly prevents this with a single **reaper goroutine** that exclusively owns `cmd.Wait()`. Never call `cmd.Wait()` directly — use `p.Wait()` which reads from `p.waitErr` channel.

### Problem: Finalizer-only cleanup

If a session pool entry's `ManagedProcess` is replaced (on rotation) without calling `Stop()`, the old process leaks until GC finalizer fires. The headless pool MUST call `p.Stop()` before replacing a pool slot.

### Solution for headless pool

Use `CommandContextPG` (process group) so context cancellation sends SIGTERM to the entire `claude` process tree, not just the direct child. For per-call processes (non-resumed), use `ShortLivedCmd` which handles this automatically.

## 3. Context Cancellation for Subprocesses

The `safeexec` package handles context cancellation correctly:
- `CommandContext` installs a `watchCtx` goroutine that sends SIGKILL when ctx fires
- `CommandContextPG` extends this to send SIGTERM/SIGKILL to the process group

**Critical for the pool**: When `Call(ctx, ...)` is called and `ctx` is cancelled by the RPC client disconnecting, the subprocess must also be killed. The design requirement says "calls on the same feature key are serialized" — this means the pool's per-key mutex ensures only one outstanding subprocess. When the caller's context is cancelled:
1. The pool's running subprocess should be killed (via context or explicit `Stop()`)
2. The session_id for that feature key should be invalidated (next call gets a fresh session)
3. The streaming channel should be closed (done by process exit)

**Pitfall**: If the pool holds a long-running `ManagedProcess` across multiple calls (for session persistence), it must NOT use the caller's per-call context to start the process. Use a pool-level `context.Background()`-derived context for the process, and use the caller's context only for the blocking `ScanLines` / channel-read operation.

## 4. Existing Test Patterns for Subprocess Management

### `executor/testdata/helper` binary

The executor package compiles a test helper binary (`executor/testdata/helper/main.go`) in `TestMain`, then uses it for all subprocess tests. The helper supports flags like `--sleep`, `--exit-code`, `--print-lines`, `--trap-sigterm` to produce predictable behaviors.

For headless package tests, the `FakeRunner` approach (from requirements) is better than a helper binary — it avoids the compile step and works with the `io.Reader` interface. However, for integration tests, consider a `--print-json` mode that outputs a valid `{"type":"result","session_id":"uuid","result":"..."}` response.

### `readAllWithStop` / `waitWithStop` test helpers

In `managed_process_test.go`:
```go
func readAllWithStop(t *testing.T, r io.Reader, p *ManagedProcess, timeout time.Duration) ([]byte, error) {
    ch := make(chan result, 1)
    go func() {
        data, err := io.ReadAll(r)
        ch <- result{data, err}
    }()
    select {
    case res := <-ch:
        return res.data, res.err
    case <-time.After(timeout):
        _ = p.Stop()
        t.Fatalf("io.ReadAll timed out after %v", timeout)
        return nil, nil
    }
}
```
This pattern (bound all blocking I/O with a timeout in tests) should be used in headless package tests.

## 5. Goroutine/Channel Leak Risks in Streaming

### Risk 1: Blocked sender goroutine

If `Call()` launches a goroutine that pushes to `chan StreamChunk` and the reader abandons the channel (context cancelled), the sender blocks forever. Fix: always use a buffered channel (buffer ≥ 1) AND a `select` with `ctx.Done()` in the sender:

```go
go func() {
    defer close(ch)
    for scanner.Scan() {
        select {
        case ch <- StreamChunk{Text: scanner.Text()}:
        case <-ctx.Done():
            _ = proc.Stop() // kill the subprocess
            return
        }
    }
}()
```

### Risk 2: Scanner goroutine blocked in `bufio.Scanner.Scan()`

If the subprocess hangs (e.g., waiting for input that never comes), `scanner.Scan()` blocks. The solution: use `ManagedProcess.Stop()` from context cancellation, which closes the pipe (`WaitDelay` applies), causing `scanner.Scan()` to return EOF.

### Risk 3: Pool serialization without timeout

If the pool serializes calls per feature key with a mutex and the current call takes hours, subsequent callers block indefinitely. Ensure all pool operations respect the caller's context. Use `select` with `ctx.Done()` before acquiring per-key locks if the wait could be long.

### Risk 4: No `goleak` in CI

`go.uber.org/goleak v1.3.0` is in `go.sum` (transitive dependency) but NOT imported or used in any test file — neither `executor/`, `session/`, nor `server/` packages. The codebase relies on bounded timeouts in tests (`readAllWithStop`) rather than automated goroutine leak detection.

Recommendation: add `goleak.VerifyTestMain(m)` to `TestMain` in the headless package's `_test.go` file to catch leaks early.

## 6. Non-Zero Exit Error Details in stdout

From the requirements:
> **PITFALL**: check stdout for error details (non-zero exit puts details in stdout, not stderr)

`CombinedOutput()` captures both streams but loses the separation. For the headless package:
- Use separate `Stdout()` and `Stderr()` from `ManagedProcess` (or `cmd.Stdout`/`cmd.Stderr` separately)
- On non-zero exit, log the captured stdout as the error detail
- Map `claude -p` exit codes: `0=success`, `1=LLM error/refusal`, `2=usage error`, `130=SIGINT`

The current `RunOneShot` uses `cmd.CombinedOutput()` and captures stdout+stderr together in `outputStr`. The headless package should separate them.

## 7. Session ID Validity After Rotation

**PITFALL**: Never share one session across concurrent workers (from requirements).

The pool's `callCount` tracking must be atomic. When rotating:
1. Acquire per-key write lock
2. Atomically replace `sessionID` with `""`
3. Reset `callCount` to 0
4. Kill old subprocess if it's still alive (it shouldn't be for one-shot calls)

**PITFALL**: rotate every ~25 calls to cap context growth.

The 25-call threshold is a heuristic. If the review prompt includes a 40KB diff on every call, context grows fast. Consider rotating based on estimated token count (from `--output-format json`'s `cost_usd` field or a byte-count heuristic) rather than a fixed call count.

## 8. Binary Path Resolution

`RunOneShot` calls `exec.LookPath("claude")` on every invocation. The headless pool should resolve the path once at construction time and store it. If `LookPath` fails at pool creation, return `ErrClaudeNotFound` immediately (fail fast at startup, not at first call).

```go
var ErrClaudeNotFound = errors.New("claude binary not found in PATH")

func NewPool(cfg PoolConfig) (*Pool, error) {
    bin, err := exec.LookPath("claude")
    if err != nil {
        return nil, ErrClaudeNotFound
    }
    // ...
}
```
