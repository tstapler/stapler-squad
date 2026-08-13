# Findings: Architecture — Missing Wiring Design

## Summary

The server startup and shutdown wiring is more complete than requirements.md assumed. HistoryLinker is constructed in `server/dependencies.go` with instances pre-loaded, started in `server/server.go` as a goroutine, and the shutdown hook in server.go already calls `CaptureCurrentState` and `SaveInstances`. The only genuine architectural gap is the cold restore branch in `Instance.start()`: the `!firstTimeSetup` path calls `RestoreWithWorkDir` and errors out when tmux is dead, with no fallback to relaunch with `--resume`. The design for this fix is straightforward — add a dead-tmux check before `RestoreWithWorkDir` and use the existing `ResumeId` mechanism. `LatestSequence` on scrollback storage is a simple addition with one clear placement.

## Current Startup Architecture

```
main.go → serve subcommand
  → BuildRuntimeDeps() (server/dependencies.go)
      Step 1-8: storage, services, queues, etc.
      Step 8.5 (line 449): NewHistoryLinkerFromRealInspector()
                           historyLinker.SetInstances(instances)   ← instances pre-loaded
      Step 9-11: scrollback, tmux streamer, external discovery
  → Serve() (server/server.go)
      line 86-99: background component startup
        go deps.ReactiveQueueMgr.Start(serverCtx)
        deps.PRStatusPoller.Start(serverCtx)
        go deps.HistoryLinker.Start(serverCtx)   ← starts AFTER SetInstances
      line 107-127: shutdown hook registered
        → captures WorkingDir for all instances
        → SaveInstances
      HTTP server starts
```

Key: `SetInstances` is called at construction time (`BuildRuntimeDeps`) before `Start()` is called (`Serve`). No startup scan race — instances are fully populated before the first poll.

## Current Shutdown Architecture

```
SIGTERM/SIGINT
  → context cancellation (main.go: signal handler calls cancel())
  → srv.Shutdown() (graceful HTTP shutdown)
  → srv.shutdownHooks run (server/server.go:107-127):
      for each instance:
        CaptureCurrentState()      ← already implemented
      SaveInstances()              ← already implemented
  → HistoryLinker.Start() goroutine exits when context is cancelled
```

The shutdown hook is registered at server startup (not a cleanup func passed to main) — it's a `[]func()` slice on the server struct. This means it runs after `Shutdown()` returns. Important ordering constraint: `CaptureCurrentState` must complete before tmux dies; there's a 4-second deadline enforced in the hook.

## Design: Cold Restore Branch in start()

**Current code** (`session/instance.go:808-818`):
```go
if !firstTimeSetup {
    workDir := i.Path
    if i.gitManager.HasWorktree() {
        workDir = i.gitManager.GetWorktreePath()
    }
    if err := i.tmuxManager.RestoreWithWorkDir(workDir); err != nil {
        setupErr = fmt.Errorf("failed to restore existing session: %w", err)
        return setupErr  // ← dead tmux = hard failure, no --resume fallback
    }
}
```

**Missing**: When `!DoesSessionExist()` and `claudeSession.SessionID != ""`, we should start a new tmux session with `--resume`. The `ResumeId` mechanism at `NewInstance(opts)` line 624 already handles this for new instances — `FromInstanceData` loading does not use it.

**Proposed design**:
```go
if !firstTimeSetup {
    if !i.tmuxManager.DoesSessionExist() && i.HasClaudeSession() {
        // Cold restore: tmux is dead but we have a conversation UUID.
        // Reuse the existing firstTimeSetup path — it reads claudeSession.SessionID
        // and adds --resume via ClaudeCommandBuilder.
        startPath := i.resolveStartPath(i.Path)
        log.InfoLog.Printf("Cold restoring '%s' with --resume %s", i.Title, i.claudeSession.SessionID)
        if err := i.tmuxManager.Start(startPath); err != nil {
            setupErr = fmt.Errorf("cold restore Start failed: %w", err)
            return setupErr
        }
        _ = i.tmuxManager.RestoreWithWorkDir(startPath)
    } else {
        workDir := i.Path
        if i.gitManager.HasWorktree() {
            workDir = i.gitManager.GetWorktreePath()
        }
        if err := i.tmuxManager.RestoreWithWorkDir(workDir); err != nil {
            setupErr = fmt.Errorf("failed to restore existing session: %w", err)
            return setupErr
        }
    }
}
```

**Dependency**: `HasClaudeSession()` already exists (line 1918). `resolveStartPath()` already handles missing WorkingDir with fallback (line 944). `initTmuxSession()` already adds `--resume` when `claudeSession.SessionID != ""` (line 1429-1430). This wires together naturally.

**Integration test**: `session/instance_cold_restore_test.go` — create Instance with `claudeSession.SessionID` set, do not create tmux session, call `Start(false)`, verify new tmux session created with `--resume <uuid>` in program string.

## Design: LatestSequence on Scrollback Storage

**Current state**: `session/scrollback/storage.go` has `Read(sessionID, fromSeq)` but no `LatestSequence(sessionID)`.

**Proposed design**: Add to `FileScrollbackStorage`:
```go
// LatestSequence returns the sequence number of the most recently written
// entry for the given session, or 0 if no entries exist.
func (s *FileScrollbackStorage) LatestSequence(sessionID string) (uint64, error) {
    entries, err := s.Read(sessionID, 0)  // read all entries
    if err != nil || len(entries) == 0 {
        return 0, err
    }
    return entries[len(entries)-1].Sequence, nil
}
```

Or more efficiently (read only the last line):
```go
func (s *FileScrollbackStorage) LatestSequence(sessionID string) (uint64, error) {
    // Use file lock, then scan last line of JSONL for Sequence field
}
```

**Callers to update**:
- `server/services/session_service.go:1720` — get scrollbackSeq from scrollback manager
- `server/services/workspace_service.go:286` — same

**Note**: `SessionService` already has access to `ScrollbackManager` via the service struct. `CreateCheckpoint` RPC can call `scrollbackManager.LatestSequence(sessionID)` before calling `inst.CreateCheckpoint(label, seq)`.

## Trade-off Matrix

| Design decision | Option A | Option B | Recommended |
|---|---|---|---|
| Cold restore trigger condition | `!DoesSessionExist() && HasClaudeSession()` | `!DoesSessionExist()` (always cold restore) | **Option A** — only use --resume when UUID is known |
| Cold restore startup path | Reuse tmuxManager.Start + initTmuxSession | Duplicate start logic inline | **Reuse** — initTmuxSession already handles --resume |
| LatestSequence implementation | Read all entries, return last | Seek to end of file, parse last line | **All entries** for simplicity; optimize if slow |
| Shutdown hook ordering | CaptureCurrentState before Shutdown() | CaptureCurrentState after Shutdown() | **Before** (current) — tmux must still be alive |

## Risk and Failure Modes

**Cold restore to deleted WorkingDir**: `resolveStartPath()` already has this guard — falls back to `i.Path` if `WorkingDir` doesn't exist. Covered.

**Race between CaptureCurrentState and SIGKILL**: If the OS kills tmux before the 4-second deadline, `GetPaneCurrentPath` returns an error and is logged but doesn't block. The saved `WorkingDir` from a previous successful capture is used as fallback.

**HistoryLinker.Start() uses context.Background()** (`server/server.go:89`): The `serverCtx` is `context.Background()` — it is never cancelled. The linker goroutine runs forever. This is intentional (the linker should run as long as the process is alive) but means the linker is not cleaned up on graceful shutdown. Low severity — process exits anyway.

**New external sessions added after HistoryLinker.Start()**: The `externalDiscovery.OnSessionAdded` callback at `dependencies.go:478` calls `historyLinker.AddInstance(instance)`. New external sessions are correctly registered. No gap.

## Migration and Adoption Cost

| Change | Files | Effort | Breaking? |
|---|---|---|---|
| Cold restore branch in `start()` | `session/instance.go` | S (30-50 LOC) | No |
| Cold restore integration test | `session/instance_cold_restore_test.go` (new) | M (new file, mock tmux or build-tagged) | No |
| LatestSequence on scrollback | `session/scrollback/storage.go`, session_service.go, workspace_service.go | S (20 LOC) | No |
| HistoryFilePath in adapter | `server/adapters/instance_adapter.go` | XS (2 LOC) | No |

## Recommendation

The architecture is sound. No redesign needed. Three targeted changes close all gaps:

1. **`session/instance.go:808`** — add dead-tmux + HasClaudeSession() check before `RestoreWithWorkDir`. Branch to `tmuxManager.Start(startPath)` for cold restore. (S)

2. **`session/scrollback/storage.go`** — add `LatestSequence(sessionID) (uint64, error)`. Update two callers in session_service.go and workspace_service.go to pass real sequence numbers. (S)

3. **`server/adapters/instance_adapter.go:55`** — add `HistoryFilePath: inst.HistoryFilePath` to the proto struct literal. (XS)

4. **`session/instance_cold_restore_test.go`** (new) — integration test for the cold restore path. (M)

## Pending Web Searches

1. `Go graceful shutdown context cancel cleanup goroutines best practices 2024` — verify whether context.Background() for long-running goroutines is standard [TRAINING_ONLY - verify]
2. `Go integration test build tag tmux mock subprocess testing` — verify test isolation patterns for tmux-dependent tests [TRAINING_ONLY - verify]
