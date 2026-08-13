# Findings: Stack — Current Codebase State Audit

## Summary

Phase 1 is more complete than the requirements.md assumed. HistoryLinker IS wired into server startup, CaptureCurrentState IS implemented and called on shutdown, CreateCheckpoint IS fully implemented, and checkpoint RPCs (Create/List/Delete) ARE in place. The proto adapter does expose `ClaudeSession.SessionId` to the web UI. The critical gaps are: (1) `HistoryFilePath` is not separately exposed via proto; (2) the cold restore path in `Start(false)` is not implemented — when tmux is dead, there is no fallback to relaunch with `--resume`; (3) `LatestSequence` does not exist on scrollback storage, so `CreateCheckpoint` always records `scrollbackSeq = 0`; (4) `ForkFromCheckpoint` IS implemented (Phase 2 ahead of plan). The cold restore gap is the most critical: sessions with a saved UUID still cannot auto-resume after a full tmux kill.

## Audit Results

### HistoryLinker startup wiring
- File: `server/dependencies.go:449`, `server/server.go:98`
- Status: **COMPLETE**
- Details: `session.NewHistoryLinkerFromRealInspector()` constructed in dependencies.go. `go deps.HistoryLinker.Start(serverCtx)` called in server.go with log confirmation. SetInstances called with loaded instances at dependencies.go:451.

### CreateCheckpoint method
- File: `session/instance.go:2053`
- Status: **COMPLETE**
- Details: Full implementation. Captures git SHA via `gitManager.GetCurrentCommitSHA()`, conversation UUID from `claudeSession.SessionID`, and conv line count by scanning history JSONL file. Accepts `scrollbackSeq uint64` parameter. Thread-safe via `stateMutex`.

### Checkpoint RPC handlers (Create/List/Delete + Fork)
- File: `server/services/session_service.go:1699, 1735, 1767`
- Status: **COMPLETE** (plus ForkFromCheckpoint at line 1791)
- Details: `CreateCheckpoint`, `ListCheckpoints`, `DeleteCheckpoint` all implemented. `ForkSession` (ForkFromCheckpoint) also implemented — Phase 2 Epic 2.1 is further along than the plan indicated. Test file: `server/services/session_service_fork_test.go`.

### Proto fields in adapter (history_file_path, claude_conversation_uuid)
- File: `server/adapters/instance_adapter.go:78-85`
- Status: **PARTIAL**
- Details: `ClaudeSession.SessionId` (the conversation UUID) IS populated from `cs.SessionID`. `ClaudeSession.ConversationId` and `ClaudeSession.ProjectName` also populated. However, `HistoryFilePath` from `inst.HistoryFilePath` is NOT included in the proto — there is no field mapping for it. The `history_file_path` proto field (if it exists in types.proto) is never set.

### CaptureCurrentState method
- File: `session/instance.go:2024`
- Status: **COMPLETE**
- Details: Guards against unstarted/paused/dead-tmux sessions. Queries `tmuxSession.GetPaneCurrentPath()`. Updates `i.WorkingDir` under `stateMutex`.

### CaptureCurrentState called on shutdown
- File: `server/server.go:117`
- Status: **COMPLETE**
- Details: Called in graceful shutdown loop: `if err := inst.CaptureCurrentState(); err != nil { log.WarningLog.Printf(...) }`. Errors are logged but do not block shutdown.

### Cold restore branch in Start(false)
- File: `session/instance.go:808-818`
- Status: **MISSING**
- Details: When `!firstTimeSetup`, `RestoreWithWorkDir(workDir)` is called. If tmux session is dead (after reboot or tmux kill), this call fails and returns an error — session cannot be served. There is no branch: "if tmux dead AND claudeSession.SessionID != '' → relaunch with --resume". The `ResumeId` mechanism (line 624) only works for newly created instances, not for loaded-from-storage instances recovering from dead tmux.

### LatestSequence on scrollback storage
- File: `session/scrollback/storage.go`
- Status: **MISSING**
- Details: No `LatestSequence()` or `CurrentSequence()` method exists. Scrollback entries have `.Sequence` fields and `Read(fromSeq)` exists, but no way to get the current high-water mark. Both callers (`workspace_service.go:286` and `session_service.go:1720`) pass `scrollbackSeq = 0`, so checkpoints always record sequence 0 — fork truncation will be inaccurate.

### ForkFromCheckpoint (Phase 2)
- File: `session/instance.go:2104`
- Status: **COMPLETE** (ahead of plan)
- Details: Creates new Instance from checkpoint. Attempts conversation fork and scrollback fork (with graceful skip on error). Phase 2 Epic 2.1 is substantially implemented.

## Trade-off Matrix

| Item | Completeness | Test Coverage | Wiring Correctness |
|---|---|---|---|
| HistoryLinker startup | Full | Indirect (linker tests) | Correct — started with server context |
| CreateCheckpoint | Full | Partial (no unit test for scrollbackSeq=0 case) | Correct |
| Checkpoint RPCs | Full + ForkSession | Yes (fork test file exists) | Correct |
| Proto adapter: ClaudeSession.SessionId | Full | N/A (integration) | Correct |
| Proto adapter: HistoryFilePath | Missing | N/A | N/A — not wired |
| CaptureCurrentState | Full | Not verified | Correct |
| Cold restore in Start(false) | Missing | Missing (instance_cold_restore_test.go absent) | N/A — code doesn't exist |
| LatestSequence | Missing | N/A | N/A — callers silently use 0 |

## Risk and Failure Modes

1. **Cold restore silently skips `--resume`**: After a machine reboot or `tmux kill-server`, sessions with saved conversation UUIDs will fail to start (`RestoreWithWorkDir` returns error). Users lose conversation context on restart despite the UUID being persisted. **Silent — no user-visible indication that cold restore was attempted or skipped.**

2. **LatestSequence = 0 in checkpoints**: All checkpoints record `scrollbackSeq = 0`. Fork truncation in `ForkFromCheckpoint` uses this 0 value — scrollback fork will copy nothing or behave incorrectly. This makes fork unreliable even though the code is otherwise complete.

3. **HistoryFilePath not exposed to web UI**: The `history_file_path` proto field is never populated. Web UI cannot show the linked JSONL file path. Minor UX gap.

## Migration and Adoption Cost

| Gap | Effort | Risk |
|---|---|---|
| Cold restore in Start(false) | M — ~50-80 LOC, existing `ResumeId` pattern to follow | Medium — needs integration test |
| LatestSequence on scrollback | S — add method to storage.go, update both callers | Low |
| HistoryFilePath in adapter | XS — one line in instance_adapter.go | Negligible |
| Cold restore integration test | M — needs mock tmux or build-tagged integration test | Medium |

## Recommendation

Priority order based on user impact:
1. **Cold restore in `Start(false)`** — closes the core "zero conversation loss" gap
2. **Cold restore integration test** — the only test that would have caught the above gap
3. **LatestSequence** — unblocks accurate fork (already built but silently broken)
4. **HistoryFilePath in adapter** — cosmetic web UI fix

Checkpoint creation, RPC handlers, and fork are all already built and wired — no work needed there beyond the LatestSequence fix.

## Pending Web Searches
(none — codebase audit, no web search needed)
