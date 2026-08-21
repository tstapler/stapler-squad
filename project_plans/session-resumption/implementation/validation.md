# Validation Plan: Session Resumption

**Phase**: 4 — Validation
**Status**: Draft
**Created**: 2026-04-03
**Plan**: `project_plans/session-resumption/implementation/plan.md`
**Requirements**: `project_plans/session-resumption/requirements.md`

---

## Requirements Traceability Matrix

| Requirement | Story | Test Suite | Test Name(s) | Risk Level |
|-------------|-------|-----------|--------------|------------|
| History file detection (Must Have) | 1.1.1, 1.1.2 | Unit | `TestProcessInspector_OpenFiles_IncludesRealFile`, `TestHistoryFileDetector_Detect_MatchesClaudePattern` | High |
| History file → session linking (Must Have) | 1.1.4 | Unit, Integration | `TestHistoryLinker_LinksUUIDToSession`, `TestHistoryLinker_IdempotentRelink` | High |
| Conversation UUID extraction (Must Have) | 1.1.2 | Unit | `TestHistoryFileDetector_ExtractsUUIDFromFilename`, `TestHistoryFileDetector_RejectsAgentFiles` | High |
| PID reuse detection (Must Have) | 1.1.1 | Unit, Property | `TestProcessInspector_IsAlive_DetectsPIDReuse`, `TestProcessInspector_IsAlive_CorrectCreateTime` | High |
| fsnotify watcher on `~/.claude/projects/` (Must Have) | 1.1.3 | Unit, Integration | `TestHistoryFileWatcher_FiresOnJSONLCreate`, `TestHistoryFileWatcher_FiresOnRename` | Medium |
| Shutdown state capture (Must Have) | 1.2.1 | Unit | `TestInstance_CaptureCurrentState_UpdatesWorkingDir`, `TestInstance_CaptureCurrentState_DeadTmux` | High |
| Cold restore with `--resume <uuid>` (Must Have) | 1.2.2 | Integration | `TestColdRestore_UsesResumeFlag`, `TestColdRestore_TransitionsToRunning` | Critical |
| Cold restore falls back to fresh start when no UUID (Must Have) | 1.2.2 | Integration | `TestColdRestore_NoUUID_StartsFresh` | High |
| Missing WorkingDir fallback (Must Have) | 1.2.2 | Unit | `TestColdRestore_MissingWorkingDir_FallsBackToPath` | Medium |
| Startup scan links sessions before first poll (Must Have) | 1.2.3 | Unit | `TestHistoryLinker_InitialScanOnStart` | Medium |
| Checkpoint struct serialization (Must Have) | 1.3.1 | Unit | `TestCheckpoint_SerializationRoundtrip`, `TestInstanceData_BackwardCompatNilCheckpoints` | Medium |
| Checkpoint creation captures correct state (Must Have) | 1.3.2 | Unit | `TestCreateCheckpoint_PopulatesAllFields`, `TestCreateCheckpoint_OnlyRunning` | Medium |
| Checkpoint IDs are unique UUIDs (Must Have) | 1.3.2 | Unit | `TestCreateCheckpoint_UniqueIDs` | Low |
| Checkpoint CRUD via RPC (Must Have) | 1.3.3 | Integration | `TestCheckpointRPC_CreateListDelete` | Medium |
| Scrollback sequence captured at checkpoint time (Must Have) | 1.3.2 | Unit | `TestCreateCheckpoint_ScrollbackSeqIsSnapshot` | Medium |
| Proto fields for history linkage (Must Have) | 1.1.5 | Unit | `TestInstanceAdapter_PopulatesHistoryFields` | Low |
| Backward compat: sessions without checkpoints load (Must Have) | 1.3.1 | Unit | `TestInstanceData_BackwardCompatNilCheckpoints` | Medium |
| Partial JSONL safety (Must Have — pitfall KI-004) | Cross-cutting | Unit, Property | `TestJSONLReader_SkipsUnparseableLastLine`, `TestJSONLReader_LargeRecord_BeyondPIPEBUF` | High |
| PIPE_BUF boundary atomicity (KI-004) | Cross-cutting | Property | `TestJSONLReader_LargeRecord_BeyondPIPEBUF` | High |
| Stale socket ECONNREFUSED not looped (KI-005) | Phase 2 | Unit | `TestAdoptedBridge_StaleSocket_NoRetryLoop` | Medium |
| Input broadcast isolation in claude-mux (KI-005, pitfall) | Phase 2 | Integration | `TestMuxMultiplexer_InputIsolation_MultipleClients` | High |

---

## Test Suites

### Unit Tests

All unit tests live in `_test.go` files alongside their subject files. External dependencies (gopsutil, tmux, fsnotify, filesystem) are mocked or stubbed. Tests use the `testify` assertion library matching existing codebase patterns.

---

#### `session/procinfo/inspector_test.go`

**Subject**: `session/procinfo/inspector.go` — `ProcessInspector`

**Test 1: `TestProcessInspector_OpenFiles_IncludesRealFile`**
- What: Calls `OpenFiles(os.Getpid())` against the current test process
- Mocks/stubs: None — uses real gopsutil against the test binary itself
- Pass criteria: Returned slice is non-empty; at least one entry is a `.go` test binary or well-known file descriptor
- Notes: Test binary has files open; this validates gopsutil works end-to-end on macOS

**Test 2: `TestProcessInspector_Cwd_MatchesOsGetwd`**
- What: Calls `Cwd(os.Getpid())` and compares to `os.Getwd()`
- Mocks/stubs: None
- Pass criteria: Paths match (after `filepath.EvalSymlinks` normalization on both sides)

**Test 3: `TestProcessInspector_CreateTime_ReturnsPositive`**
- What: Calls `CreateTime(os.Getpid())`
- Mocks/stubs: None
- Pass criteria: Returns a positive int64 representing epoch ms; value is within 24 hours of `time.Now().UnixMilli()`

**Test 4: `TestProcessInspector_IsAlive_CorrectCreateTime`**
- What: Calls `IsAlive(os.Getpid(), <correct createTime>)`
- Mocks/stubs: None
- Pass criteria: Returns `true`

**Test 5: `TestProcessInspector_IsAlive_DetectsPIDReuse`**
- What: Calls `IsAlive(<realPID>, <wrong createTime>)` with a create time that is clearly in the past (e.g., unix epoch 0)
- Mocks/stubs: None
- Pass criteria: Returns `false`, demonstrating PID reuse detection
- Covers: **KI-003** (PID reuse during history correlation)

**Test 6: `TestProcessInspector_NonExistentPID_ReturnsError`**
- What: Calls `OpenFiles(int32(99999999))` for a PID that definitely does not exist
- Mocks/stubs: None
- Pass criteria: Returns empty slice and non-nil error (or nil error with empty slice — document expected behavior); does NOT panic

**Test 7: `TestProcessInspector_PermissionDenied_ReturnsEmptyNotError`**
- What: Calls `OpenFiles` for a PID owned by root (e.g., PID 1 on macOS)
- Mocks/stubs: None
- Pass criteria: Returns empty slice without propagating an unhandled permission error to the caller; inspector does not crash

---

#### `session/history_detector_test.go`

**Subject**: `session/history_detector.go` — `HistoryFileDetector`

The tests use a mock `ProcessInspector` that implements the same interface, returning configurable file lists.

**Test 8: `TestHistoryFileDetector_Detect_MatchesClaudePattern`**
- What: Mock inspector returns `["/Users/user/.claude/projects/abc123/550e8400-e29b-41d4-a716-446655440000.jsonl"]`
- Mocks/stubs: Mock `ProcessInspector.OpenFiles` returning a matching path
- Pass criteria: Returns `HistoryFileInfo` with `ConversationUUID = "550e8400-e29b-41d4-a716-446655440000"`, `ProjectDir = "abc123"`, `HistoryFilePath` set

**Test 9: `TestHistoryFileDetector_Detect_NoJSONLReturnsNilNil`**
- What: Mock inspector returns a list with no `.jsonl` paths
- Mocks/stubs: Mock `ProcessInspector.OpenFiles` returning `["/usr/lib/something.so", "/tmp/foo.txt"]`
- Pass criteria: Returns `(nil, nil)` — not an error, just no match

**Test 10: `TestHistoryFileDetector_Detect_FiltersAgentJSONL`**
- What: Mock inspector returns `["~/.claude/projects/hash/agent-550e8400.jsonl"]`
- Mocks/stubs: Mock `ProcessInspector.OpenFiles`
- Pass criteria: Returns `(nil, nil)` — agent files excluded
- Covers: Plan Task 1.1.2b (exclude `agent-*.jsonl`)

**Test 11: `TestHistoryFileDetector_ExtractsUUIDFromFilename`**
- What: Table-driven test with multiple UUID filenames and one invalid filename
- Input cases: valid UUID filename, UUID with dashes, non-UUID filename (e.g., `session.jsonl`)
- Pass criteria: Valid UUIDs extracted correctly; non-UUID filenames cause the file to be skipped (returns nil)

**Test 12: `TestHistoryFileDetector_Detect_SymlinkPathNormalized`**
- What: Mock inspector returns a symlink path; test verifies `filepath.EvalSymlinks` is called and the resolved path is stored
- Mocks/stubs: Create a real temp directory with a symlink pointing into a Claude-like path structure; mock inspector returns the symlink path
- Pass criteria: `HistoryFilePath` in result contains the resolved (non-symlink) path
- Covers: **Pitfall** (lsof returns symlink paths, not canonical paths)

**Test 13: `TestHistoryFileDetector_ProcessDeadReturnsNilNil`**
- What: Mock inspector's `OpenFiles` returns an error simulating process-not-found
- Mocks/stubs: Mock `ProcessInspector.OpenFiles` returns `(nil, os.ErrProcessDone)` or similar
- Pass criteria: `Detect` returns `(nil, nil)` — dead process is not an error for the caller

---

#### `session/history_watcher_test.go`

**Subject**: `session/history_watcher.go` — `HistoryFileWatcher`

These tests use a real `fsnotify` watcher against a temp directory (not a mock), since testing filesystem event delivery requires real filesystem operations.

**Test 14: `TestHistoryFileWatcher_FiresOnJSONLCreate`**
- What: Starts watcher on a temp dir, creates a `<uuid>.jsonl` file, waits for callback
- Mocks/stubs: None — real temp directory, real fsnotify
- Pass criteria: Registered callback fires within 3 seconds with the correct file path
- Timeout: 5 seconds total

**Test 15: `TestHistoryFileWatcher_DoesNotFireOnNonJSONL`**
- What: Creates a `foo.txt` file and a `bar.log` file in watched directory
- Mocks/stubs: None
- Pass criteria: Callback NOT called within 1 second of file creation

**Test 16: `TestHistoryFileWatcher_FiresOnRename`**
- What: Creates `tmp.file`, then renames it to `<uuid>.jsonl`
- Mocks/stubs: None
- Pass criteria: Callback fires for the final `.jsonl` filename
- Covers: **Pitfall** (some tools use temp file + rename, fires RENAME not CREATE)

**Test 17: `TestHistoryFileWatcher_DirectoryNotExist_NoError`**
- What: Calls `Start` with a directory that does not exist on disk
- Mocks/stubs: None
- Pass criteria: `Start` returns without error; watcher is in a degraded (polling-fallback) state; no panic

**Test 18: `TestHistoryFileWatcher_ContextCancellationStopsWatcher`**
- What: Starts watcher, cancels context, verifies goroutine exits
- Mocks/stubs: None
- Pass criteria: After context cancel, creating a new `.jsonl` file does NOT trigger the callback; no goroutine leak (verified via `goleak` or test timeout)

**Test 19: `TestHistoryFileWatcher_FiltersAgentJSONL`**
- What: Creates `agent-uuid.jsonl` in watched directory
- Mocks/stubs: None
- Pass criteria: Callback NOT called for agent files

---

#### `session/history_linker_test.go`

**Subject**: `session/history_linker.go` — `HistoryLinker`

Uses mock `HistoryFileDetector` and mock session list.

**Test 20: `TestHistoryLinker_LinksUUIDToSession`**
- What: Mock detector returns a UUID for a session's PID; verify the session's `ClaudeSessionData.SessionID` is updated
- Mocks/stubs: Mock `HistoryFileDetector`, list of `*Instance` with one running session
- Pass criteria: After calling `scanAllSessions()`, the instance's `claudeSession.SessionID` equals the detected UUID

**Test 21: `TestHistoryLinker_IdempotentRelink`**
- What: Session already has a `SessionID` set; detector returns a different UUID for the same PID
- Mocks/stubs: Mock detector
- Pass criteria: Existing UUID is NOT overwritten — once linked, linking is idempotent
- Covers: Plan Task 1.1.4d (no duplicate update)

**Test 22: `TestHistoryLinker_DetectorReturnsNil_SessionUnchanged`**
- What: Mock detector returns `(nil, nil)` for all PIDs
- Mocks/stubs: Mock detector
- Pass criteria: Session's `SessionID` remains empty; no error logged as critical

**Test 23: `TestHistoryLinker_WatcherCallbackTriggersCorrelation`**
- What: Watcher fires a callback with a JSONL path; verify linker attempts correlation for sessions with open files matching that path
- Mocks/stubs: Mock detector that returns a UUID when queried; mock watcher that fires callback synchronously
- Pass criteria: Session is updated within one callback invocation cycle

**Test 24: `TestHistoryLinker_InitialScanOnStart`**
- What: Calls `HistoryLinker.Start()` and verifies that `scanAllSessions()` is called before the first 5-second poll interval
- Mocks/stubs: Mock detector with a configured response; check session state immediately after `Start()` returns
- Pass criteria: Session UUID is populated without waiting 5 seconds
- Covers: Story 1.2.3 (startup scan before first poll)

**Test 25: `TestHistoryLinker_PIDReuseDetected_SkipsCorrelation`**
- What: Mock inspector says a PID has the wrong `CreateTime`; verify linker discards the correlation
- Mocks/stubs: Mock `ProcessInspector.IsAlive` returns `false`
- Pass criteria: Session UUID is NOT updated despite detector returning a UUID
- Covers: **KI-003** (PID reuse during history correlation)

---

#### `session/instance_test.go` — CaptureCurrentState additions

**Test 26: `TestInstance_CaptureCurrentState_UpdatesWorkingDir`**
- What: Calls `CaptureCurrentState()` on an instance with a mock tmux manager that returns a path
- Mocks/stubs: Mock `TmuxManager.DisplayMessage` returning `/home/user/myproject`
- Pass criteria: `instance.WorkingDir == "/home/user/myproject"` after the call

**Test 27: `TestInstance_CaptureCurrentState_DeadTmux_NoError`**
- What: Calls `CaptureCurrentState()` when tmux session does not exist
- Mocks/stubs: Mock `TmuxManager.DoesSessionExist` returns `false`
- Pass criteria: Method returns `nil` (no error); `WorkingDir` unchanged

---

#### `session/checkpoint_test.go`

**Subject**: `session/checkpoint.go` and checkpoint methods on `Instance`

**Test 28: `TestCheckpoint_SerializationRoundtrip`**
- What: Creates a `Checkpoint` struct with all fields populated, marshals to JSON, unmarshals back, compares
- Mocks/stubs: None — pure data test
- Pass criteria: All fields preserved identically after roundtrip; `google.protobuf.Timestamp` equivalent handled correctly

**Test 29: `TestInstanceData_BackwardCompatNilCheckpoints`**
- What: Deserializes an `InstanceData` JSON blob that has NO `checkpoints` field (old format)
- Mocks/stubs: None — use `json.Unmarshal` directly
- Pass criteria: `FromInstanceData` returns a valid `Instance` with `Checkpoints == nil` or empty slice; no error

**Test 30: `TestCheckpointList_FindByID`**
- What: Creates a `CheckpointList` with 3 checkpoints, calls `FindByID` with each ID and a non-existent ID
- Mocks/stubs: None
- Pass criteria: Correct checkpoint returned for valid IDs; `nil` returned for non-existent ID

**Test 31: `TestCheckpointList_FindByLabel`**
- What: Same as above but tests `FindByLabel`; also verifies behavior when two checkpoints share the same label
- Mocks/stubs: None
- Pass criteria: First matching checkpoint returned when label is duplicated

**Test 32: `TestCheckpointList_Latest`**
- What: Creates checkpoints with different timestamps, calls `Latest()`
- Mocks/stubs: None
- Pass criteria: Returns checkpoint with the most recent timestamp

**Test 33: `TestCreateCheckpoint_PopulatesAllFields`**
- What: Calls `instance.CreateCheckpoint("my-label")` on a running instance
- Mocks/stubs: Mock `FileScrollbackStorage.LatestSequence`, mock `GitWorktreeManager.GetCurrentCommitSHA`, instance has `claudeSession.SessionID` set
- Pass criteria: Returned checkpoint has non-empty `ID` (valid UUID), `Label == "my-label"`, `ScrollbackSeq > 0`, `GitCommitSHA` set, `ClaudeConvUUID` set, `Timestamp` within 1 second of `time.Now()`

**Test 34: `TestCreateCheckpoint_OnlyRunning`**
- What: Calls `CreateCheckpoint` on a paused instance
- Mocks/stubs: Instance with `Status == Paused`
- Pass criteria: Returns a non-nil error; no checkpoint appended to the list

**Test 35: `TestCreateCheckpoint_UniqueIDs`**
- What: Calls `CreateCheckpoint` 10 times in rapid succession
- Mocks/stubs: None — test uniqueness
- Pass criteria: All 10 returned checkpoint IDs are distinct UUIDs

**Test 36: `TestCreateCheckpoint_MultipleCheckpoints_AppendCorrectly`**
- What: Creates 3 checkpoints; verifies the full list
- Pass criteria: `len(instance.Checkpoints) == 3`; order is insertion order; `ActiveCheckpoint` equals the most recently created ID

---

#### JSONL Partial-Read Safety Tests

**Subject**: Shared utility or existing `session/history.go` pattern

**Test 37: `TestJSONLReader_SkipsUnparseableLastLine`**
- What: Creates a JSONL byte stream where the last line is a partial JSON object (simulates mid-write). Parses with `bufio.Scanner`.
- Input: `{"a":1}\n{"b":2}\n{"c":3` (no closing brace on last line)
- Mocks/stubs: `bytes.NewReader` — no filesystem needed
- Pass criteria: Parser returns 2 valid records (`a` and `b`); no error returned; partial line silently skipped
- Covers: **KI-004** (partial JSONL line during concurrent read)

**Test 38: `TestJSONLReader_LargeRecord_BeyondPIPEBUF`**
- What: Creates a JSONL record that is larger than 4096 bytes (Linux PIPE_BUF) to validate the bufio.Scanner `MaxScanTokenSize` is set high enough
- Input: Single JSONL line with a 64KB string value
- Mocks/stubs: None — pure in-memory
- Pass criteria: Record is parsed successfully without `bufio.ErrTooLong`; if the scanner has a size limit, it is at least `bufio.MaxScanTokenSize` (64KB) or configurable
- Covers: **KI-004**, **Pitfall** (large JSONL entries can exceed PIPE_BUF / scanner default buffer)

**Test 39: `TestJSONLReader_EmptyFile_ReturnsEmptySlice`**
- What: Parses an empty byte stream
- Pass criteria: Returns empty slice, no error, no panic

---

### Integration Tests

Integration tests require real infrastructure components (real filesystem, real tmux if available). They are tagged with `//go:build integration` and run separately via `go test -tags=integration ./...`.

---

#### Cold Restore Integration Tests — `session/instance_cold_restore_test.go`

**Test 40: `TestColdRestore_UsesResumeFlag`** [CRITICAL]
- What: Creates an `Instance` with `claudeSession.SessionID = "550e8400-e29b-41d4-a716-446655440000"`, does NOT create a real tmux session (simulates dead session), calls `instance.Start(false)`.
- Infrastructure: Real tmux must be available (tagged `integration`); mock tmux manager that records the command used to start the session
- Pass criteria: The tmux start command contains `--resume 550e8400-e29b-41d4-a716-446655440000`; instance status transitions to `Running`
- Covers: Story 1.2.2, **Must Have** requirement "Resume after restart"

**Test 41: `TestColdRestore_TransitionsToRunning`** [CRITICAL]
- What: Same setup as Test 40; after `Start(false)`, check status
- Pass criteria: `instance.Status == Running` within 10 seconds; no error returned from `Start`

**Test 42: `TestColdRestore_NoUUID_StartsFresh`**
- What: Creates an `Instance` with NO `claudeSession.SessionID`, simulates dead tmux session, calls `Start(false)`
- Pass criteria: Start command does NOT contain `--resume`; a fresh session is launched; log contains "No conversation UUID for cold restore, starting fresh"
- Covers: Story 1.2.2 no-UUID branch

**Test 43: `TestColdRestore_MissingWorkingDir_FallsBackToPath`**
- What: Creates instance with `WorkingDir` pointing to a non-existent directory; calls cold restore
- Pass criteria: Session starts using `instance.Path` as the working directory; no error from Start; log contains fallback warning
- Covers: **KI-007** (cold restore with missing working directory)

**Test 44: `TestColdRestore_WorkingDirUsedWhenPresent`**
- What: Creates instance with valid `WorkingDir` that exists on disk; calls cold restore
- Pass criteria: New tmux session is started in `WorkingDir`, not in `instance.Path`
- Covers: Story 1.2.2b (WorkingDir used as cold restore cwd)

---

#### HistoryLinker Integration Tests — `session/history_linker_integration_test.go`

**Test 45: `TestHistoryLinker_Integration_DetectsRealProcessFiles`**
- What: Starts a goroutine that opens a temp file whose path matches `~/.claude/projects/testproject/<uuid>.jsonl`. Creates a real `HistoryLinker` with a real `ProcessInspector`. Calls `Detect` for the test process PID.
- Infrastructure: Real filesystem, real gopsutil
- Pass criteria: Detector returns the correct `ConversationUUID` extracted from the temp file path

**Test 46: `TestHistoryLinker_Integration_WatcherAndLinkerTogether`**
- What: Starts a real `HistoryFileWatcher` on a temp directory structured like `~/.claude/projects/`. Creates a new `<uuid>.jsonl` file. Verifies that the `HistoryLinker` callback fires and links the UUID to a test session.
- Infrastructure: Real filesystem, real fsnotify
- Pass criteria: Session's `SessionID` is populated within 5 seconds of file creation

---

#### Checkpoint RPC Integration Tests — `server/services/session_service_checkpoint_test.go`

**Test 47: `TestCheckpointRPC_CreateListDelete`**
- What: Uses a real in-memory session store; calls `CreateCheckpoint`, `ListCheckpoints`, `DeleteCheckpoint` via the service layer (not raw HTTP)
- Infrastructure: In-memory session store, no network required
- Pass criteria:
  - After `CreateCheckpoint("v1")`: `ListCheckpoints` returns 1 checkpoint with `Label == "v1"`
  - After `DeleteCheckpoint(checkpointID)`: `ListCheckpoints` returns 0 checkpoints

**Test 48: `TestCheckpointRPC_SessionNotFound_ReturnsError`**
- What: Calls `CreateCheckpoint` with a session ID that does not exist
- Pass criteria: Returns a `connect.CodeNotFound` error

---

### E2E Tests

E2E tests simulate the complete user-facing flow. They require tmux, and optionally a real Claude binary (which will be mocked). Tagged `//go:build e2e`.

**Test 49: `TestE2E_HistoryDetectionWithinTenSeconds`** [CRITICAL]
- What: Start a managed session running a program that opens a file matching `~/.claude/projects/testproject/<uuid>.jsonl`. Verify the session's `SessionID` is populated.
- Infrastructure: Real tmux, real filesystem, mocked Claude binary that opens a JSONL file
- Pass criteria: `instance.claudeSession.SessionID` is non-empty within 10 seconds of session start
- Covers: **Success Criterion 4** ("History files detected, tracked, and linked")

**Test 50: `TestE2E_ShutdownCaptureThenColdRestore`** [CRITICAL]
- Flow:
  1. Create a managed session with a known `SessionID` (simulated by writing a temp JSONL file)
  2. Trigger graceful shutdown (call `CaptureCurrentState` + `SaveInstances`)
  3. Simulate tmux session death (kill the tmux session)
  4. Call `FromInstanceData` to reload the session
  5. Verify cold restore path executes
- Infrastructure: Real tmux session required
- Pass criteria: After reload, session command contains `--resume <uuid>`; `WorkingDir` matches the captured path from step 2
- Covers: **Success Criterion 1** ("After restart, session can be resumed — scrollback, cwd, conversation history intact")

**Test 51: `TestE2E_CheckpointCreateAndResumeFromCheckpoint`**
- Flow:
  1. Start session, link history file, create checkpoint labeled "before-refactor"
  2. Restart stapler-squad (reload from storage)
  3. List checkpoints — verify "before-refactor" is present
  4. Cold restore from checkpoint (Phase 2 — mark this as pending Phase 2 implementation but specify now)
- Pass criteria (Phase 1 scope): Checkpoint "before-refactor" survives shutdown/restart cycle

---

### Property-Based Tests

Property-based tests use `gopkg.in/check.v1` or table-driven tests with generated inputs. Given the codebase uses `testify`, table-driven tests provide equivalent coverage.

**Test 52: `TestPropertyBased_PIDReuseDetection_AlwaysDisambiguated`**
- What: For 100 random `(pid, createTimeMs)` pairs, verify `IsAlive(pid, wrongCreateTime)` always returns `false` when `wrongCreateTime` does not match the actual `CreateTime` of any running process
- Method: Table-driven with a mix of real PIDs (current process) with correct/incorrect create times, and non-existent PIDs
- Pass criteria: No false positives — never returns `true` for a wrong create time

**Test 53: `TestPropertyBased_JSONLPartialLineSkip_RandomInputs`**
- What: Generates JSONL content with 0-50 valid complete lines followed by 0 or 1 partial (truncated) lines. Runs the JSONL reader over each.
- Method: Table-driven with generated inputs: empty file, all valid lines, last line partial at various truncation points (1 byte, middle of key, middle of value, missing closing `}`)
- Pass criteria: Parser always returns exactly N valid lines (not N+1, not N-1); never panics; never returns an error for partial last lines

**Test 54: `TestPropertyBased_CheckpointIDUniqueness`**
- What: Creates 1000 checkpoints in rapid succession and verifies all IDs are unique
- Pass criteria: `len(seenIDs) == 1000` — no UUID collisions

---

## Critical Path Tests (Must Pass Before Merge)

These 10 tests are the minimum required to merge the Phase 1 implementation. Each maps to a high-risk requirement or a pitfall from the research.

| Priority | Test Name | File | Risk Addressed |
|----------|-----------|------|----------------|
| 1 | `TestColdRestore_UsesResumeFlag` | `session/instance_cold_restore_test.go` | Core cold restore correctness — `--resume` flag must be present |
| 2 | `TestColdRestore_TransitionsToRunning` | `session/instance_cold_restore_test.go` | Session must be usable after cold restore |
| 3 | `TestHistoryLinker_LinksUUIDToSession` | `session/history_linker_test.go` | UUID linkage — without this, cold restore has no UUID to use |
| 4 | `TestProcessInspector_IsAlive_DetectsPIDReuse` | `session/procinfo/inspector_test.go` | KI-003: PID reuse causing wrong session correlation |
| 5 | `TestHistoryLinker_PIDReuseDetected_SkipsCorrelation` | `session/history_linker_test.go` | KI-003: End-to-end PID reuse safety in linker |
| 6 | `TestJSONLReader_SkipsUnparseableLastLine` | Unit test alongside JSONL reader | KI-004: Partial JSONL line causes panic/data loss |
| 7 | `TestJSONLReader_LargeRecord_BeyondPIPEBUF` | Unit test alongside JSONL reader | KI-004: Large JSONL records exceed bufio default buffer size |
| 8 | `TestInstance_CaptureCurrentState_UpdatesWorkingDir` | `session/instance_test.go` | Shutdown capture provides WorkingDir for cold restore |
| 9 | `TestInstanceData_BackwardCompatNilCheckpoints` | `session/checkpoint_test.go` | Backward compat — existing sessions must load without checkpoints field |
| 10 | `TestColdRestore_NoUUID_StartsFresh` | `session/instance_cold_restore_test.go` | Graceful degradation when no UUID available — must not block startup |

---

## Known Issue Coverage (from findings-pitfalls.md)

Each pitfall from the research has at least one dedicated test case:

| Pitfall | Source | Test(s) | Severity |
|---------|--------|---------|---------|
| **macOS ptrace limitations** — no TTY re-parenting; no reptyr equivalent | findings-pitfalls.md §1 | No test needed — confirmed out of scope in requirements. Implementation must NOT attempt ptrace. | N/A |
| **lsof symlink paths vs canonical paths** | findings-pitfalls.md §2.3 | `TestHistoryFileDetector_Detect_SymlinkPathNormalized` (Test 12) | Medium |
| **lsof transient failures on process startup** | findings-pitfalls.md §2.2 | `TestHistoryFileDetector_ProcessDeadReturnsNilNil` (Test 13); `TestProcessInspector_NonExistentPID_ReturnsError` (Test 6) | Medium |
| **FSEvents coalescing** — up to 3s latency on new file creation | findings-pitfalls.md §3.1 | `TestHistoryFileWatcher_FiresOnJSONLCreate` (Test 14) uses a 5s timeout, not a 100ms one, accounting for coalescing. `TestHistoryLinker_InitialScanOnStart` (Test 24) validates polling fallback exists independently of watcher. | Low |
| **fsnotify recursive watch required for new subdirs** | findings-pitfalls.md §3.4 | `TestHistoryFileWatcher_FiresOnJSONLCreate` (Test 14) must be run against a nested path to exercise recursive watch | Low |
| **RENAME vs CREATE event** — temp file + rename pattern | findings-pitfalls.md §3.5 | `TestHistoryFileWatcher_FiresOnRename` (Test 16) | Medium |
| **PID reuse — macOS recycles PIDs aggressively** | findings-pitfalls.md §4.1 | `TestProcessInspector_IsAlive_DetectsPIDReuse` (Test 5), `TestHistoryLinker_PIDReuseDetected_SkipsCorrelation` (Test 25) | High |
| **KI-003: PID reuse during history correlation** | plan.md KI-003 | Tests 5 and 25 (above) | High |
| **KI-004: Partial JSONL line during concurrent read** | plan.md KI-004 | `TestJSONLReader_SkipsUnparseableLastLine` (Test 37), `TestJSONLReader_LargeRecord_BeyondPIPEBUF` (Test 38) | High |
| **KI-005: Stale claude-mux socket ECONNREFUSED loop** | plan.md KI-005 | `TestAdoptedBridge_StaleSocket_NoRetryLoop` (Phase 2 — deferred to Phase 2 implementation) | Medium |
| **KI-006: Scrollback write contention during fork** | plan.md KI-006 | Covered by existing `FileScrollbackStorage` mutex tests; deferred Phase 2 fork test in `session/scrollback/fork_test.go` | Low |
| **KI-007: Cold restore with missing WorkingDir** | plan.md KI-007 | `TestColdRestore_MissingWorkingDir_FallsBackToPath` (Test 43) | Medium |
| **Buffer loss window on socket adoption** | findings-pitfalls.md §5.1 | Deferred to Phase 2 (`TestAdoptedBridge_ReadsExistingHistoryBeforeSocketStream`) | Medium |
| **Socket creation timing race** | findings-pitfalls.md §5.2 | Deferred to Phase 2 (`TestAdoptedBridge_RetryBackoffOnSocketNotReady`) | Medium |
| **Input broadcast to multiple claude-mux clients** | findings-pitfalls.md §5.3 | `TestMuxMultiplexer_InputIsolation_MultipleClients` — run against existing `session/mux/multiplexer.go` as part of Phase 1 audit | High |
| **O_APPEND PIPE_BUF boundary** — large writes not atomic | findings-pitfalls.md §3 | `TestJSONLReader_LargeRecord_BeyondPIPEBUF` (Test 38) | High |
| **File handle kept alive after REMOVE** | findings-pitfalls.md §3.2 | `TestHistoryFileWatcher_DirectoryNotExist_NoError` (Test 17) for the watcher side; graceful detach is a runtime behavior, not unit-testable in isolation | Low |

---

## Test Environment Requirements

### Always Required (unit tests, `go test ./...`)

- Go toolchain with CGo enabled (`CGO_ENABLED=1`) — required by gopsutil on macOS
- macOS 12+ (Monterey or later) — for `proc_pidinfo` access via gopsutil
- No special permissions — all tests run as the current user

### Integration Tests (`-tags=integration`)

- tmux installed and available in `PATH` — verified by `exec.LookPath("tmux")`; tests skip gracefully if absent
- `~/.claude/projects/` directory — tests create a temp structure that mimics this path; actual `~/.claude` is NOT modified by tests
- File system with inotify/FSEvents support — all macOS and Linux environments meet this requirement

### E2E Tests (`-tags=e2e`)

- tmux available
- A mock Claude binary or a shell script that mimics Claude's JSONL file creation behavior:
  ```bash
  # mock-claude: opens a JSONL file and blocks
  #!/bin/bash
  mkdir -p ~/.claude/projects/testproject
  UUID="550e8400-e29b-41d4-a716-446655440000"
  touch ~/.claude/projects/testproject/${UUID}.jsonl
  sleep 30
  ```
- Test cleanup must remove all temp files and tmux sessions created during E2E runs

### CI Configuration

```yaml
# Required CI environment variables
CGO_ENABLED: "1"
STAPLER_SQUAD_INSTANCE: "test"  # isolate test state

# Recommended CI commands
go test ./session/procinfo/... -v -run TestProcessInspector
go test ./session/... -v -run 'TestHistoryFile|TestHistoryLinker|TestCheckpoint|TestColdRestore|TestJSONL'
go test -tags=integration ./session/... -timeout=60s
```

### Build Tags Summary

| Tag | Purpose | Command |
|-----|---------|---------|
| (none) | Unit tests — always run | `go test ./...` |
| `integration` | Tests requiring tmux or real filesystem events | `go test -tags=integration ./...` |
| `e2e` | Full end-to-end flow tests | `go test -tags=e2e ./...` |
| `darwin` | Build constraint on `session/procinfo/inspector.go` | automatic |

---

## Success Criteria

The implementation is ready for merge when:

1. **All 10 critical path tests pass** without modification to production code during test runs
2. **No regression** in existing tests: `go test ./...` passes cleanly before and after the feature branch
3. **Integration test pass rate is 100%** for the `session/procinfo`, `session/history_*`, and `session/instance_cold_restore_test.go` suites when run with `-tags=integration` and tmux available
4. **KI-003 (PID reuse) and KI-004 (partial JSONL)** each have passing tests demonstrating the mitigation works
5. **Backward compatibility confirmed**: existing sessions loaded from pre-feature JSON storage files load without error (`TestInstanceData_BackwardCompatNilCheckpoints`)
6. **Manual test checklist** verified by the developer (from plan.md):
   - [ ] Start a Claude session, verify history file detected within 10 seconds (validates Test 49)
   - [ ] SIGTERM stapler-squad, verify hot resume (tmux alive) continues without `--resume` re-invocation
   - [ ] Kill tmux server, restart stapler-squad, verify cold restore uses `--resume <uuid>` (validates Test 50)
   - [ ] Create a checkpoint, kill and restart, verify checkpoint persists in storage (validates Test 51)
   - [ ] Kill stapler-squad, check sessions.json or SQLite — verify `history_file_path` and `working_dir` are persisted

---

## Phase 2 Test Stubs (Deferred)

These tests are specified now but deferred to Phase 2 implementation. Their names are recorded here to ensure they are not forgotten.

| Test Name | File | Covers |
|-----------|------|--------|
| `TestForkScrollback_CopiesLinesUpToSeq` | `session/scrollback/fork_test.go` | Story 2.1.1 |
| `TestForkScrollback_AtomicRename` | `session/scrollback/fork_test.go` | Story 2.1.1, KI-006 |
| `TestForkClaudeConversation_TruncatesAtMessageIndex` | `session/history_fork_test.go` | Story 2.1.2 |
| `TestForkSession_CreatesNewInstanceWithForkedFromID` | `session/instance_fork_test.go` | Story 2.1.3 |
| `TestAdoptedBridge_StaleSocket_NoRetryLoop` | `session/bridge/adopted_bridge_test.go` | KI-005 |
| `TestAdoptedBridge_RetryBackoffOnSocketNotReady` | `session/bridge/adopted_bridge_test.go` | Pitfall §5.2 |
| `TestAdoptedBridge_ReadsExistingHistoryBeforeSocketStream` | `session/bridge/adopted_bridge_test.go` | Pitfall §5.1 |
| `TestMuxMultiplexer_InputIsolation_MultipleClients` | `session/mux/multiplexer_test.go` | Pitfall §5.3, Story 2.2.3 |
