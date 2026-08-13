# Stack Research: Subprocess Execution & Streaming

## 1. Existing Subprocess Management

### safeexec package (`executor/safeexec/`)

The project has a thin `safeexec` wrapper around `os/exec` that enforces two invariants:

- **`safeexec.CommandContext(ctx, name, args...)`** — wraps `exec.CommandContext` but pre-sets `WaitDelay = 2s`. This prevents zombie accumulation when a grandchild holds stdout/stderr pipes open after SIGKILL: `cmd.Wait()` will return and close pipes within 2 seconds regardless.
- **`safeexec.CommandContextPG(ctx, name, args...)`** — same as above, plus `SysProcAttr.Setpgid = true`. Placing the child in a new process group means the internal `watchCtx` goroutine sends SIGTERM to the entire process group on context cancellation, killing grandchildren too. **Do not use for PTY-attached processes** (causes SIGTTIN/SIGTTOU).

For headless LLM calls, `CommandContextPG` is the correct choice — `claude -p` is a background process with no controlling terminal.

### `executor.ShortLivedCmd` (`executor/shortlived.go`)

Wraps one-shot subprocess invocations: `Run()`, `Output()`, `CombinedOutput()`. Applies `safeexec`, optional per-command timeout (shorter of context deadline and configured timeout), optional process group, stdin/env/dir config, and audit logging.

The existing `CLIAIClient.Complete()` (in `server/services/cli_ai_client.go`) already uses `executor.New(ctx, bin, args, executor.WithStdin(...), executor.WithTimeout(55s)).Output()` — this is the one-shot pattern.

### `executor.ManagedProcess` (`executor/managed_process.go`)

For long-running subprocesses. Key design:
- Uses `os.Pipe()` pairs (not `CombinedOutput`) so stdout and stderr are separately accessible as `io.Reader`
- Single **reaper goroutine** exclusively owns `cmd.Wait()` — prevents multiple `Wait()` races
- `done chan struct{}` closed by reaper when process exits
- `waitErr chan error` (buffered 1) — written once by reaper
- GC finalizer as last-resort kill if `Stop()` never called
- `Stop()` sends SIGTERM via `p.cancel()`, then SIGKILL to process group after `gracePeriod`
- **`ScanLines(ctx, fn)`** — reads lines from `Stdout()` via `bufio.Scanner` in a goroutine, returns on EOF or `ctx.Done()`. This is the correct streaming primitive.

For the headless package, `ManagedProcess` (with `ScanLines`) is the right building block for streaming stdout from `claude -p`. Alternatively, for the simpler blocking `CallBlocking()` path, `ShortLivedCmd.Output()` suffices.

## 2. stdout Streaming Patterns

The `ManagedProcess.ScanLines` implementation shows the canonical pattern:

```go
scanner := bufio.NewScanner(p.stdoutReader)
scanDone := make(chan error, 1)
go func() {
    for scanner.Scan() {
        fn(scanner.Text())
    }
    scanDone <- scanner.Err()
}()
select {
case err := <-scanDone:
    return err
case <-ctx.Done():
    return ctx.Err()
}
```

For the `Call()` method returning `<-chan StreamChunk`, the headless package should:
1. Start `ManagedProcess` (or pipe stdout from `ShortLivedCmd`)
2. Goroutine reads stdout line by line, pushes to buffered channel
3. Close channel when goroutine exits (process EOF or error)
4. Use `context.WithCancel` to kill the process if the caller abandons the channel

**Key risk**: if the channel reader stops consuming, the goroutine blocks. Always buffer the channel (at minimum 1) and use a `select` with `ctx.Done()` in the send loop.

## 3. `--output-format json` vs `--output-format stream-json`

Based on the requirements document and the claude CLI flag conventions:

- **`--output-format json`**: Single JSON object emitted after the call completes. Schema: `{"type":"result","result":"...","session_id":"uuid","cost_usd":0.012}`. Used for the **first call** to capture `session_id`. Blocks until done.

- **`--output-format stream-json`**: JSON event stream emitted incrementally. Each line is a JSON event (assistant text chunk, tool call, etc.). Used for streaming output. The final event includes `session_id`.

For the headless pool:
- First call: `--output-format json` → parse `session_id` from the single JSON result
- Resumed calls: plain output (or `stream-json` for streaming chunks) with `--resume <session_id>`

No existing usages of `--output-format` for claude in the codebase — this is net-new. The RunOneShot handler uses `cmd.CombinedOutput()` with no format flag.

## 4. Existing Server-Streaming RPC Patterns

Three server-streaming RPCs exist in `proto/session/v1/`:

```protobuf
rpc WatchSessions(WatchSessionsRequest) returns (stream SessionEvent) {}
rpc WatchReviewQueue(WatchReviewQueueRequest) returns (stream ReviewQueueEvent) {}
rpc WatchInsights(WatchInsightsRequest) returns (stream InsightsEvent) {}   // insights.proto
rpc WatchUnfinishedWork(WatchUnfinishedWorkRequest) returns (stream UnfinishedWorkEvent) {}  // unfinished.proto
```

Handler signature pattern (from `WatchSessions` and `WatchInsights`):
```go
func (s *SessionService) WatchSessions(
    ctx context.Context,
    req *connect.Request[sessionv1.WatchSessionsRequest],
    stream *connect.ServerStream[sessionv1.SessionEvent],
) error {
    // subscribe before building snapshot so no events are lost
    eventCh, subID := s.eventBus.Subscribe(ctx)
    defer s.eventBus.Unsubscribe(subID)

    for {
        select {
        case <-ctx.Done():
            return nil
        case event, ok := <-eventCh:
            if !ok { return nil }
            if err := stream.Send(protoEvent); err != nil {
                return fmt.Errorf("failed to send event: %w", err)
            }
        }
    }
}
```

`WatchInsights` shows a simpler pattern without an event bus: subscribe to a channel, forward events until `ctx.Done()`.

The `RunHeadlessCall` RPC should follow exactly the `WatchInsights` pattern: subscribe to the `<-chan StreamChunk` from `headless.Call()`, send each chunk via `stream.Send()`, return on `ctx.Done()` or channel close.

Registration in `server/server.go` requires adding the new service handler (e.g., a `HeadlessService`) with `sessionv1connect.NewHeadlessServiceHandler(...)`. Optionally register a `StreamingWSBridge` entry if browser SSE support is needed (see `watchSessionsPath` pattern).
