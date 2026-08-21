# Implementation Plan: Session Resumption

**Feature**: Session Resumption (history detection, cold restore, checkpoints, fork)
**Date**: 2026-04-03
**Status**: Ready for implementation
**ADRs**: ADR-001 (Session Identity), ADR-002 (Two-Tier Resume), ADR-003 (Checkpoint Storage)

---

## Dependency Visualization

```
Phase 1 MVP
===========

Epic 1.1: History Detection       Epic 1.2: Cold Restore        Epic 1.3: Checkpoints
-------------------------       ----------------------        --------------------
[1.1.1] gopsutil dep            [1.2.1] Shutdown capture       [1.3.1] Checkpoint struct
    |                               |                              |
[1.1.2] HistoryDetector            [1.2.2] Cold restore path      [1.3.2] Checkpoint service
    |                               |                              |
[1.1.3] fsnotify watcher ----->[1.2.3] Link detection to      [1.3.3] ConnectRPC endpoint
    |                           session on startup                 |
[1.1.4] Link to session                                        [1.3.4] Web UI checkpoint
    |
[1.1.5] ConnectRPC exposure


Phase 2 (outlined)
==================

Epic 2.1: Fork                  Epic 2.2: Adopted Bridge
-------------------             ------------------------
Depends on:                     Depends on:
  - Epic 1.3 (checkpoints)       - Epic 1.1 (history detection)
  - Epic 1.2 (cold restore)      - Existing claude-mux autodiscovery


Phase 3 (outlined)
==================

Epic 3.1: Read-Only Discovery   Epic 3.2: OSC 7 CWD   Epic 3.3: VT Snapshots
Depends on: Epic 1.1            Independent            Depends on: Epic 1.3
```

---

## Phase 1 MVP: Full Detail

### Epic 1.1: History File Detection and Linking

**Goal**: Detect which Claude JSONL conversation files a managed session is writing to, and link the conversation UUID to the session record.

**Rationale**: The conversation UUID is the stable identity anchor for session resumption (ADR-001). Without it, cold restore cannot use `claude --resume`.

---

#### Story 1.1.1: Add gopsutil Dependency and Process Inspector

**As a** developer, **I want** a process inspection module that can query open files, cwd, and terminal device for a given PID on macOS, **so that** we can detect history files without relying on shell-out commands.

**Acceptance Criteria**:
- `go get github.com/shirou/gopsutil/v3/process` added to go.mod
- New file `session/procinfo/inspector.go` with `ProcessInspector` struct
- Methods: `OpenFiles(pid) -> []string`, `Cwd(pid) -> string`, `CreateTime(pid) -> int64`
- Each method handles errors gracefully (process gone, permission denied)
- Unit tests with mock/stub for CI (gopsutil requires a real process for integration)

**Files**: `go.mod`, `go.sum`, `session/procinfo/inspector.go`, `session/procinfo/inspector_test.go`

##### Task 1.1.1a: Add gopsutil/v3 dependency

- Run `go get github.com/shirou/gopsutil/v3/process`
- Verify builds on macOS with CGo enabled
- Files: `go.mod`, `go.sum`

##### Task 1.1.1b: Implement ProcessInspector

- Create `session/procinfo/inspector.go`
- Struct `ProcessInspector` with methods wrapping gopsutil calls
- `OpenFiles(pid int32) ([]string, error)` — returns file paths, filters errors
- `Cwd(pid int32) (string, error)` — returns working directory
- `CreateTime(pid int32) (int64, error)` — returns epoch ms for PID disambiguation
- `IsAlive(pid int32, expectedCreateTime int64) bool` — checks PID reuse
- Files: `session/procinfo/inspector.go`

##### Task 1.1.1c: Write unit tests for ProcessInspector

- Test OpenFiles against current process (os.Getpid()) — should include at least 1 file
- Test Cwd against current process — should match os.Getwd()
- Test IsAlive with current PID and correct/incorrect create time
- Test error paths: non-existent PID returns appropriate error
- Files: `session/procinfo/inspector_test.go`

---

#### Story 1.1.2: Implement HistoryFileDetector

**As a** developer, **I want** a component that scans a running session's process for open Claude JSONL history files and extracts the conversation UUID, **so that** session records can be linked to their conversation identity.

**Acceptance Criteria**:
- New file `session/history_detector.go` with `HistoryFileDetector` struct
- `Detect(pid int32) -> (*HistoryFileInfo, error)` scans open files for `~/.claude/projects/**/*.jsonl`
- Extracts conversation UUID from filename
- Extracts project directory from parent directory name
- Handles: no JSONL found (returns nil, nil), process dead, permission errors
- `HistoryFileInfo` struct: `{ConversationUUID, HistoryFilePath, ProjectDir}`

**Files**: `session/history_detector.go`, `session/history_detector_test.go`

##### Task 1.1.2a: Define HistoryFileInfo and HistoryFileDetector structs

- `HistoryFileInfo` with fields: `ConversationUUID string`, `HistoryFilePath string`, `ProjectDir string`
- `HistoryFileDetector` with `ProcessInspector` dependency
- Constructor: `NewHistoryFileDetector(inspector *procinfo.ProcessInspector) *HistoryFileDetector`
- Files: `session/history_detector.go`

##### Task 1.1.2b: Implement Detect method

- Call `inspector.OpenFiles(pid)`
- Filter for paths matching `~/.claude/projects/**/*.jsonl` (exclude `agent-*.jsonl`)
- Extract UUID from filename (basename without .jsonl extension)
- Validate UUID format using existing `isValidUUID()` from `claude_command_builder.go`
- Extract project directory from parent dir name
- Return first matching file (a session typically has one active conversation)
- Use `filepath.EvalSymlinks()` when comparing paths (pitfall from research)
- Files: `session/history_detector.go`

##### Task 1.1.2c: Write tests for HistoryFileDetector

- Test with mock inspector returning paths matching Claude pattern
- Test with no JSONL files open (returns nil, nil)
- Test with agent-*.jsonl files (should be filtered out)
- Test UUID extraction from filename
- Test symlink path normalization
- Files: `session/history_detector_test.go`

---

#### Story 1.1.3: Add fsnotify Watcher for Claude Projects Directory

**As a** developer, **I want** a filesystem watcher on `~/.claude/projects/` that fires when new JSONL files are created, **so that** we can detect new conversations faster than polling.

**Acceptance Criteria**:
- New file `session/history_watcher.go` with `HistoryFileWatcher`
- Uses `fsnotify` with recursive flag (`fsnotify.WithRecursive`) on `~/.claude/projects/`
- On CREATE or RENAME event for `.jsonl` files, emits notification via callback
- Handles both CREATE and RENAME events (some tools use temp file + rename)
- Graceful start/stop with context cancellation
- Falls back gracefully if `~/.claude/projects/` does not exist

**Files**: `session/history_watcher.go`, `session/history_watcher_test.go`

##### Task 1.1.3a: Implement HistoryFileWatcher

- Create `session/history_watcher.go`
- `HistoryFileWatcher` struct with `fsnotify.Watcher`, callback function, context
- `Start(ctx context.Context)` — add recursive watch, start event loop goroutine
- Event loop: filter for `.jsonl` files, call registered callback with file path
- Handle both `CREATE` and `RENAME` events (pitfall: some tools use temp+rename)
- Ignore non-JSONL files and `agent-*.jsonl` files
- `Stop()` — close watcher
- If `~/.claude/projects/` does not exist, log warning and return (no error)
- Files: `session/history_watcher.go`

##### Task 1.1.3b: Write tests for HistoryFileWatcher

- Test: create a .jsonl file in a temp dir, verify callback fires
- Test: create a non-.jsonl file, verify callback does NOT fire
- Test: rename a file to .jsonl, verify callback fires
- Test: directory does not exist, verify Start returns without error
- Test: context cancellation stops the watcher
- Files: `session/history_watcher_test.go`

---

#### Story 1.1.4: Link Detected History Files to Session Records

**As a** developer, **I want** the history file detector to automatically populate `ClaudeSessionData.SessionID` on managed sessions when their JSONL file is detected, **so that** sessions have the conversation UUID available for resume.

**Acceptance Criteria**:
- New component `session/history_linker.go` with `HistoryLinker`
- Runs a background goroutine that periodically scans managed sessions
- For each running session, calls `HistoryFileDetector.Detect(pid)`
- If a conversation UUID is found and not already set, updates `ClaudeSessionData.SessionID`
- Also responds to `HistoryFileWatcher` callbacks for faster correlation
- Poll interval: 5 seconds (not performance-critical)
- Thread-safe updates to Instance via existing `stateMutex`

**Files**: `session/history_linker.go`, `session/history_linker_test.go`

##### Task 1.1.4a: Implement HistoryLinker

- Create `session/history_linker.go`
- `HistoryLinker` struct with detector, watcher, and reference to session list
- `Start(ctx context.Context)` — start poll loop (5s interval) + register watcher callback
- Poll loop: iterate sessions, call `Detect()` for each running session's PID
- When UUID found: update `instance.claudeSession.SessionID` and `HistoryFilePath` (new field)
- Thread-safe: acquire `instance.stateMutex` before writing
- `GetPIDForSession(instance) -> int32` helper: extract PID from tmux session
- Files: `session/history_linker.go`

##### Task 1.1.4b: Add HistoryFilePath field to InstanceData

- Add `HistoryFilePath string json:"history_file_path,omitempty"` to `InstanceData`
- Add corresponding field to `Instance` struct
- Update `ToInstanceData()` and `FromInstanceData()` to serialize/deserialize
- Files: `session/storage.go`, `session/instance.go`

##### Task 1.1.4c: Wire HistoryLinker into server startup

- In `server/dependencies.go` or equivalent startup code:
  - Create `ProcessInspector`
  - Create `HistoryFileDetector` with inspector
  - Create `HistoryFileWatcher`
  - Create `HistoryLinker` with detector, watcher, and session store
  - Start linker with server context
- Files: `server/dependencies.go`

##### Task 1.1.4d: Write tests for HistoryLinker

- Test: mock detector returns UUID, verify session gets updated
- Test: session already has UUID, verify no duplicate update
- Test: detector returns nil (no file found), verify session unchanged
- Test: watcher callback triggers correlation
- Files: `session/history_linker_test.go`

---

#### Story 1.1.5: Expose History File Info via ConnectRPC

**As a** web UI developer, **I want** the session's linked conversation UUID and history file path visible in the Session proto message, **so that** the UI can display history linkage status.

**Acceptance Criteria**:
- Add `history_file_path` and `claude_conversation_uuid` fields to `Session` proto message
- Populate in session service adapter when converting Instance to proto Session
- Web UI can display "Linked to conversation: <uuid>" on session card (optional in MVP)

**Files**: `proto/session/v1/types.proto`, `server/adapters/instance_adapter.go`, then `make proto-gen`

##### Task 1.1.5a: Add proto fields and regenerate

- Add to `Session` message in `types.proto`:
  - `string history_file_path = <next_field_number>;`
  - `string claude_conversation_uuid = <next_field_number>;`
- Run `make proto-gen`
- Files: `proto/session/v1/types.proto`, generated files

##### Task 1.1.5b: Populate new fields in instance adapter

- In `server/adapters/instance_adapter.go`, when converting Instance to proto Session:
  - Set `HistoryFilePath` from instance's `HistoryFilePath` field
  - Set `ClaudeConversationUuid` from `claudeSession.SessionID`
- Files: `server/adapters/instance_adapter.go`

---

### Epic 1.2: Cold Restore After Restart

**Goal**: When stapler-squad restarts and finds that a previously managed session's tmux session is dead, automatically relaunch Claude with `--resume <uuid>` in the correct working directory to restore conversation context.

**Rationale**: ADR-002 defines the two-tier resume strategy. Hot resume already works (existing `Start(false)` path). This epic implements the cold restore tier.

---

#### Story 1.2.1: Capture Session State Before Shutdown

**As a** developer, **I want** to ensure that cwd, conversation UUID, and history file path are persisted in the session record before stapler-squad shuts down, **so that** cold restore has the data it needs.

**Acceptance Criteria**:
- On graceful shutdown (SIGTERM/SIGINT), iterate running sessions
- For each: capture current cwd from tmux pane (`tmux display-message -p -t <session> '#{pane_current_path}'`)
- Update `InstanceData.WorkingDir` with captured cwd
- Conversation UUID already persisted by HistoryLinker (Epic 1.1)
- Call `SaveInstances()` to flush to storage
- All of this happens in the existing graceful shutdown handler

**Files**: `session/instance.go`, `server/server.go` (or wherever shutdown is handled)

##### Task 1.2.1a: Implement CaptureCurrentState on Instance

- Add method `Instance.CaptureCurrentState() error`
- Captures cwd from tmux: `tmux display-message -p -t <session_name> '#{pane_current_path}'`
- Updates `i.WorkingDir` with result
- If tmux session is dead, skip gracefully
- Files: `session/instance.go`

##### Task 1.2.1b: Call CaptureCurrentState during shutdown

- In the graceful shutdown handler (likely `server/server.go` or `main.go`):
  - Before calling `SaveInstances()`, iterate all running sessions and call `CaptureCurrentState()`
  - Log any errors but do not fail shutdown
- Files: `server/server.go` (or `main.go`)

##### Task 1.2.1c: Write test for CaptureCurrentState

- Test with mock tmux manager returning a directory path
- Test with dead tmux session (should not error)
- Files: `session/instance_test.go`

---

#### Story 1.2.2: Implement Cold Restore Path in FromInstanceData

**As a** developer, **I want** `FromInstanceData` to detect when a tmux session is dead and trigger a cold restore with `--resume`, **so that** session conversation context is recovered after a reboot.

**Acceptance Criteria**:
- When `FromInstanceData` calls `Start(false)` and tmux session does not exist:
  - If `ClaudeSessionData.SessionID` is set (has conversation UUID):
    - Construct command with `--resume <uuid>` using `ClaudeCommandBuilder`
    - Start new tmux session in `WorkingDir` (or `Path` as fallback)
    - Log: "Cold restoring session '<title>' with --resume <uuid>"
  - If no UUID available:
    - Start fresh session (existing behavior)
    - Log: "No conversation UUID for cold restore, starting fresh"
- Session status transitions to Running after cold restore
- Scrollback from FileScrollbackStorage is available in web UI for historical context

**Files**: `session/instance.go`

##### Task 1.2.2a: Detect tmux-dead condition and branch to cold restore

- In `instance.start()` (the internal start method), when `firstTimeSetup == false`:
  - Before calling `tmuxManager.RestoreWithWorkDir()`, check `tmuxManager.DoesSessionExist()`
  - If session does not exist AND `claudeSession != nil && claudeSession.SessionID != ""`:
    - Set `claudeSession` on the instance (it was restored from InstanceData)
    - `initTmuxSession()` will use `ClaudeCommandBuilder` which already adds `--resume`
    - Call `tmuxManager.Start(workDir)` instead of `RestoreWithWorkDir()`
    - Log cold restore action
  - If session does not exist AND no UUID:
    - Start fresh tmux session (no `--resume` flag)
    - Log warning
- Files: `session/instance.go`

##### Task 1.2.2b: Ensure WorkingDir is used as cold restore cwd

- When cold restoring, determine start path:
  - Prefer `i.WorkingDir` if set and directory exists on disk
  - Fall back to `i.Path`
  - Fall back to worktree path if git worktree is available
- Use `resolveStartPath()` which already handles this logic
- Verify `WorkingDir` is populated from shutdown capture (Story 1.2.1)
- Files: `session/instance.go`

##### Task 1.2.2c: Write integration test for cold restore

- Create instance, set `ClaudeSessionData.SessionID` to a valid UUID
- Do NOT create a tmux session (simulating dead tmux)
- Call `Start(false)`
- Verify: new tmux session created with program containing `--resume <uuid>`
- Verify: session transitions to Running
- Files: `session/instance_cold_restore_test.go`

---

#### Story 1.2.3: Link History Detection to Session on Startup

**As a** developer, **I want** the HistoryLinker to scan all sessions at startup before the first poll interval, **so that** sessions restored from storage have their conversation UUID populated immediately.

**Acceptance Criteria**:
- `HistoryLinker.Start()` performs an initial scan of all sessions synchronously
- This runs after sessions are loaded from storage but before they are served to the web UI
- If a session is already cold-restored with `--resume`, the linker detects the new JSONL file it writes

**Files**: `session/history_linker.go`

##### Task 1.2.3a: Add initial scan on HistoryLinker start

- In `HistoryLinker.Start()`, before entering the poll loop:
  - Call `scanAllSessions()` synchronously
  - This iterates all running sessions and calls `Detect()` for each
- Files: `session/history_linker.go`

---

### Epic 1.3: Checkpoint Metadata Creation

**Goal**: Allow users to create named checkpoints (bookmarks) on active sessions that capture the current state (scrollback position, git SHA, timestamp). No fork yet -- just bookmarks.

**Rationale**: ADR-003 defines the checkpoint storage model. Checkpoints are the prerequisite for fork (Phase 2).

---

#### Story 1.3.1: Define Checkpoint Struct and Storage Fields

**As a** developer, **I want** a `Checkpoint` data struct and corresponding fields on `InstanceData`, **so that** checkpoint metadata can be persisted alongside sessions.

**Acceptance Criteria**:
- `Checkpoint` struct in `session/checkpoint.go` with fields from ADR-003
- `InstanceData` gains: `Checkpoints []Checkpoint`, `ActiveCheckpoint string`, `ForkedFromID string`
- `Instance` gains corresponding in-memory fields
- `ToInstanceData()` and `FromInstanceData()` handle serialization
- Backward compatible: existing sessions without checkpoints load normally

**Files**: `session/checkpoint.go`, `session/storage.go`, `session/instance.go`

##### Task 1.3.1a: Create Checkpoint struct

- New file `session/checkpoint.go`
- Define `Checkpoint` struct per ADR-003 schema
- Add `CheckpointList` type alias `[]Checkpoint` with helper methods:
  - `FindByID(id string) *Checkpoint`
  - `FindByLabel(label string) *Checkpoint`
  - `Latest() *Checkpoint` (most recent by timestamp)
- Files: `session/checkpoint.go`

##### Task 1.3.1b: Add checkpoint fields to InstanceData and Instance

- Add to `InstanceData`:
  - `Checkpoints []Checkpoint json:"checkpoints,omitempty"`
  - `ActiveCheckpoint string json:"active_checkpoint,omitempty"`
  - `ForkedFromID string json:"forked_from_id,omitempty"`
- Add to `Instance`:
  - `Checkpoints CheckpointList`
  - `ActiveCheckpoint string`
  - `ForkedFromID string`
- Update `ToInstanceData()`: copy checkpoint fields
- Update `FromInstanceData()`: copy checkpoint fields
- Files: `session/storage.go`, `session/instance.go`

##### Task 1.3.1c: Write serialization tests

- Test: create Instance with checkpoints, convert to InstanceData and back
- Test: load InstanceData with no checkpoints (backward compat)
- Test: CheckpointList helper methods (FindByID, FindByLabel, Latest)
- Files: `session/checkpoint_test.go`

---

#### Story 1.3.2: Implement Checkpoint Creation Service

**As a** developer, **I want** a service method that creates a checkpoint on an active session, capturing the current scrollback position, git SHA, and conversation UUID, **so that** users can bookmark session state.

**Acceptance Criteria**:
- `Instance.CreateCheckpoint(label string) (*Checkpoint, error)` method
- Captures: scrollback line count, current git HEAD SHA, conversation UUID, current timestamp
- Generates UUID for checkpoint ID
- Appends to `Instance.Checkpoints` slice
- Thread-safe (uses `stateMutex`)
- Returns error if session is not running

**Files**: `session/instance.go` (or `session/checkpoint.go`)

##### Task 1.3.2a: Implement CreateCheckpoint on Instance

- Add method `Instance.CreateCheckpoint(label string) (*Checkpoint, error)`
- Under `stateMutex` write lock:
  - Verify instance is started and running
  - Get scrollback sequence from FileScrollbackStorage (need to expose a `LineCount` or `LatestSeq` method)
  - Get git HEAD SHA from `gitManager.GetCurrentCommitSHA()` (may need to add this)
  - Get conversation UUID from `claudeSession.SessionID`
  - Generate checkpoint UUID via `google/uuid`
  - Create Checkpoint struct, append to `i.Checkpoints`
  - Set `i.ActiveCheckpoint` to new checkpoint ID
- Files: `session/instance.go` (or `session/checkpoint.go`)

##### Task 1.3.2b: Add git HEAD SHA helper

- Add `GitWorktreeManager.GetCurrentCommitSHA() (string, error)` if not already available
- Runs `git rev-parse HEAD` in the worktree directory
- Returns empty string if no worktree (directory-only sessions)
- Files: `session/git_worktree_manager.go` (or equivalent)

##### Task 1.3.2c: Add scrollback sequence count method

- Add `FileScrollbackStorage.LatestSequence(sessionID string) (uint64, error)`
- Reads the last entry from the scrollback JSONL file to get the current sequence number
- Or count lines if sequence tracking is not easily accessible
- Files: `session/scrollback/storage.go`

##### Task 1.3.2d: Write tests for CreateCheckpoint

- Test: create checkpoint on running session, verify all fields populated
- Test: create checkpoint on paused session, verify error returned
- Test: create multiple checkpoints, verify they append correctly
- Test: checkpoint IDs are unique UUIDs
- Files: `session/checkpoint_test.go`

---

#### Story 1.3.3: Add ConnectRPC Endpoint for Checkpoint CRUD

**As a** web UI developer, **I want** RPC endpoints to create, list, and delete checkpoints, **so that** the web UI can provide checkpoint management.

**Acceptance Criteria**:
- New RPC methods in `session.proto`:
  - `CreateCheckpoint(session_id, label) -> Checkpoint`
  - `ListCheckpoints(session_id) -> []Checkpoint`
  - `DeleteCheckpoint(session_id, checkpoint_id) -> success`
- Proto message `Checkpoint` with fields matching Go struct
- Service implementation delegates to `Instance.CreateCheckpoint()`
- Proto generation succeeds (`make proto-gen`)

**Files**: `proto/session/v1/session.proto`, `proto/session/v1/types.proto`, `server/services/session_service.go`

##### Task 1.3.3a: Add Checkpoint proto messages

- Add to `types.proto`:
  ```
  message Checkpoint {
    string id = 1;
    string session_id = 2;
    string parent_id = 3;
    string label = 4;
    int64 scrollback_seq = 5;
    string scrollback_path = 6;
    string claude_conv_uuid = 7;
    string git_commit_sha = 8;
    google.protobuf.Timestamp timestamp = 9;
  }
  ```
- Add to `session.proto`:
  ```
  rpc CreateCheckpoint(CreateCheckpointRequest) returns (CreateCheckpointResponse) {}
  rpc ListCheckpoints(ListCheckpointsRequest) returns (ListCheckpointsResponse) {}
  rpc DeleteCheckpoint(DeleteCheckpointRequest) returns (DeleteCheckpointResponse) {}
  ```
- Define request/response messages
- Run `make proto-gen`
- Files: `proto/session/v1/types.proto`, `proto/session/v1/session.proto`

##### Task 1.3.3b: Implement checkpoint RPC handlers

- In `server/services/session_service.go`:
  - `CreateCheckpoint`: find instance by ID, call `instance.CreateCheckpoint(label)`, save, return
  - `ListCheckpoints`: find instance, return `instance.Checkpoints` as proto
  - `DeleteCheckpoint`: find instance, remove checkpoint from slice, save
- Add adapter methods to convert between Go Checkpoint and proto Checkpoint
- Files: `server/services/session_service.go`, `server/adapters/instance_adapter.go`

---

#### Story 1.3.4: Web UI Checkpoint Button (Minimal)

**As a** user, **I want** a "Create Checkpoint" button on the session detail panel, **so that** I can bookmark the current state of a session.

**Acceptance Criteria**:
- "Bookmark" or "Checkpoint" button on session card or detail view
- Clicking opens a small input for checkpoint label (default: timestamp)
- Calls `CreateCheckpoint` RPC
- Success: shows brief toast notification
- Checkpoint list visible on session detail (shows label, timestamp, git SHA prefix)

**Files**: `web-app/src/components/sessions/SessionCard.tsx` (or new component), `web-app/src/lib/hooks/useSessionService.ts`

##### Task 1.3.4a: Add checkpoint hook to useSessionService

- Add `createCheckpoint(sessionId, label)` method to session service hook
- Add `listCheckpoints(sessionId)` method
- Add `deleteCheckpoint(sessionId, checkpointId)` method
- Files: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 1.3.4b: Add checkpoint UI to session detail

- Add "Create Checkpoint" button (bookmark icon)
- Label input popover on click
- Display checkpoint list below session info (label, time, git SHA[0:7])
- Delete button per checkpoint
- Files: `web-app/src/components/sessions/SessionCard.tsx` (or new checkpoint component)

---

## Phase 2: Outlined (Story-Level)

### Epic 2.1: Checkpoint Fork

**Depends on**: Epic 1.3 (Checkpoint Metadata), Epic 1.2 (Cold Restore)

#### Story 2.1.1: Implement ForkScrollback

- `session/scrollback/fork.go` with `ForkScrollback(srcPath, upToSeq, dstPath) error`
- bufio.Scanner copy of first N lines to new JSONL file
- Temp file + atomic rename for safety
- Tests with real JSONL files

#### Story 2.1.2: Implement ForkClaudeConversation

- `session/history_fork.go` with `ForkClaudeConversation(srcConvPath, upToMessage, dstDir) (newUUID, error)`
- Copy JSONL truncated to message index, write to new UUID filename
- OR: use `claude --fork` if CLI supports it (investigate during implementation)
- Temp file + atomic rename
- Tests with sample JSONL content

#### Story 2.1.3: Implement ForkSession on Instance

- `Instance.ForkFromCheckpoint(checkpointID, newTitle, newBranch) (*Instance, error)`
- Creates new Instance with `ForkedFromID` set
- Calls `ForkScrollback()` and `ForkClaudeConversation()`
- Creates new git branch from checkpoint's git SHA
- Starts new tmux session with `--resume <newUUID>`

#### Story 2.1.4: Add ForkSession RPC and Web UI

- New RPC: `ForkSession(session_id, checkpoint_id, new_title, new_branch) -> Session`
- Web UI: "Fork from checkpoint" button in checkpoint list
- Prompts for new session title and branch name

### Epic 2.2: Adopted Session Bridge Improvements

**Depends on**: Epic 1.1 (History Detection), existing claude-mux autodiscovery

#### Story 2.2.1: Implement AdoptedBridge with Retry Logic

- `session/bridge/adopted_bridge.go` with `AdoptedBridge` struct
- `Run(ctx)` goroutine: connect to claude-mux socket with retry+backoff
- Route Output messages to scrollback + web UI stream
- Route web UI Input to socket
- Handle disconnect/reconnect

#### Story 2.2.2: Socket Registry for Fast Reconnection

- `~/.stapler-squad/mux-registry.json` mapping session title to socket path
- Written by HistoryLinker when new sockets are discovered
- Read on startup for instant reconnection (skip filesystem scan)
- Stale entries pruned on startup

#### Story 2.2.3: Verify Input Isolation in claude-mux

- Audit `session/mux/multiplexer.go` for input broadcast behavior
- Ensure Input messages from web UI do NOT echo to original terminal
- If broadcast exists, fix to route input only to PTY, not to other clients
- Add integration test with two concurrent clients

---

## Phase 3: Outlined (Story-Level)

### Epic 3.1: Read-Only External Process Discovery

#### Story 3.1.1: Process Scanner for Non-Mux Claude Sessions

- gopsutil scan for processes named "claude" or "node" with Claude patterns
- Match against known managed sessions to identify unmanaged ones
- Display as "Observed (read-only)" in UI with metadata (cwd, history file, terminal)
- No PTY control — metadata display only

#### Story 3.1.2: Web UI Read-Only Session Cards

- Visual differentiation for observed sessions (eye icon, grayed controls)
- Show: cwd, conversation UUID, process uptime, history file path
- "Adopt" button disabled with tooltip explaining claude-mux requirement

### Epic 3.2: OSC 7 CWD Tracking

#### Story 3.2.1: Parse OSC 7 Sequences from PTY Output

- Add OSC 7 parser to scrollback capture pipeline
- When `\033]7;file://<host><path>\033\\` is detected, update session cwd
- Eliminates need for polling cwd on shells that emit OSC 7

### Epic 3.3: VT State Snapshots

#### Story 3.3.1: Terminal State Serialization at Checkpoint Time

- At checkpoint creation, serialize the rendered terminal state (visible pane content + cursor position)
- Store as `TerminalState []byte` field on `Checkpoint`
- On cold restore from checkpoint, write serialized state to tmux pane for O(1) restore
- Avoids O(N) scrollback replay for large sessions

---

## Known Issues

### KI-001: gopsutil CGo Requirement [SEVERITY: Medium]

**Description**: `gopsutil/v3` requires CGo on macOS for `proc_pidinfo` access. This affects cross-compilation and may cause build issues in pure Go CI environments.

**Mitigation**:
- Ensure CI builds have CGo enabled (`CGO_ENABLED=1`)
- The project already uses CGo dependencies (SQLite via `mattn/go-sqlite3`)
- Build tags can gate procinfo package for macOS only if needed

**Files Affected**: `session/procinfo/inspector.go`, `go.mod`

**Prevention**: Add build constraint `//go:build darwin` to procinfo package. Provide stub implementation for non-Darwin platforms.

### KI-002: FSEvents Coalescing May Delay History File Detection [SEVERITY: Low]

**Description**: macOS FSEvents coalesces rapid file changes into single events with 1-3 second latency. A newly created JSONL file may not trigger the fsnotify callback for up to 3 seconds.

**Mitigation**:
- The poll-based HistoryLinker runs every 5 seconds as a fallback
- fsnotify is a fast-path optimization, not the only detection mechanism
- Combined detection: fsnotify for speed + polling for reliability

**Files Affected**: `session/history_watcher.go`

**Prevention**: Never rely solely on fsnotify for critical state changes. Always have a polling fallback.

### KI-003: PID Reuse During History Correlation [SEVERITY: High]

**Description**: Between detecting a PID's open files and writing the session record, the PID could be reused by a different process. This would link the wrong conversation to a session.

**Mitigation**:
- Always verify `Process.CreateTime()` matches expected value before trusting PID results
- Store `{pid, createTimeMs}` composite, not PID alone (ADR-001)
- If CreateTime mismatch detected, discard the correlation result
- The HistoryLinker only updates sessions that are actively managed (known tmux sessions), reducing the window for false matches

**Files Affected**: `session/history_linker.go`, `session/procinfo/inspector.go`

**Prevention**: `ProcessInspector.IsAlive(pid, expectedCreateTime)` check before every PID-dependent operation.

### KI-004: Partial JSONL Line During Concurrent Read [SEVERITY: Medium]

**Description**: When stapler-squad reads a Claude JSONL file while Claude is actively writing, the last line may be incomplete (partial JSON). If naively parsed, this produces a JSON unmarshal error.

**Mitigation**:
- Use `bufio.Scanner` for line-by-line reading
- Skip the last line if `json.Unmarshal()` fails (it is likely mid-write)
- Track byte offset of last successfully parsed line for resumable reads
- The existing `ClaudeSessionHistory.parseConversationFile()` in `session/history.go` already handles this with Scanner + skip

**Files Affected**: `session/history_detector.go`, `session/scrollback/fork.go` (Phase 2)

**Prevention**: All JSONL readers must use the skip-last-unparseable-line pattern. Add a shared utility: `jsonl.ReadValidLines(reader) -> [][]byte`.

### KI-005: Stale claude-mux Socket After Process Crash [SEVERITY: Medium]

**Description**: If `claude-mux` is killed with SIGKILL, the socket file `/tmp/claude-mux-<PID>.sock` remains on disk. stapler-squad sees it, tries to connect, gets `ECONNREFUSED`, and enters a reconnect loop.

**Mitigation**:
- On `ECONNREFUSED`, immediately mark socket as stale and remove from active list
- Do not retry on connection refused (different from connection timeout)
- The existing `autodiscover.go` handles socket removal via fsnotify REMOVE events
- For crash cases: probe socket with non-blocking connect before entering bridge relay

**Files Affected**: `session/mux/autodiscover.go`, `session/bridge/adopted_bridge.go` (Phase 2)

**Prevention**: Distinguish between "socket exists but nobody listening" (stale) vs "socket exists but not ready yet" (startup race). Use kqueue EVFILT_PROC to detect process exit and immediately clean up.

### KI-006: Scrollback Write Contention During Fork [SEVERITY: Low]

**Description**: When fork reads scrollback JSONL while the live session is appending to it, there is potential for read/write contention.

**Mitigation**:
- `FileScrollbackStorage` already uses per-session mutex locks (`getFileLock(sessionID)`)
- Fork reads only lines [0..N] where N was the count at checkpoint time; new appends after N are not read
- Use a snapshot of the sequence number at checkpoint time, not a live read
- Fork operates on a different file path (writes to a new session's file)

**Files Affected**: `session/scrollback/fork.go` (Phase 2), `session/scrollback/storage.go`

**Prevention**: Fork always takes `ScrollbackSeq` from the Checkpoint struct, never reads "current count" during the copy.

### KI-007: Cold Restore with Missing Working Directory [SEVERITY: Medium]

**Description**: After a cold restore, the saved `WorkingDir` may no longer exist on disk (e.g., if a worktree was cleaned up during a previous pause, or the directory was manually deleted).

**Mitigation**:
- `resolveStartPath()` already handles this: falls back to `basePath` if `WorkingDir` does not exist
- For worktree sessions: the worktree is recreated during `Start()` before the tmux session launches
- Log a warning when falling back from saved cwd

**Files Affected**: `session/instance.go`

**Prevention**: Always validate directory existence before passing to tmux. The existing `resolveStartPath()` method already has this guard.

---

## Implementation Notes

### New Dependencies

| Package | Purpose | CGo Required |
|---------|---------|-------------|
| `github.com/shirou/gopsutil/v3/process` | Process inspection (open files, cwd, create time) | Yes (macOS) |

`fsnotify` and `google/uuid` are already in `go.mod`.

### Files Created (Phase 1)

| File | Epic | Purpose |
|------|------|---------|
| `session/procinfo/inspector.go` | 1.1.1 | Process inspection wrapper |
| `session/procinfo/inspector_test.go` | 1.1.1 | Inspector tests |
| `session/history_detector.go` | 1.1.2 | JSONL file to PID correlation |
| `session/history_detector_test.go` | 1.1.2 | Detector tests |
| `session/history_watcher.go` | 1.1.3 | fsnotify watcher for ~/.claude/projects/ |
| `session/history_watcher_test.go` | 1.1.3 | Watcher tests |
| `session/history_linker.go` | 1.1.4 | Background service linking detection to sessions |
| `session/history_linker_test.go` | 1.1.4 | Linker tests |
| `session/checkpoint.go` | 1.3.1 | Checkpoint struct and helpers |
| `session/checkpoint_test.go` | 1.3.1, 1.3.2 | Checkpoint tests |
| `session/instance_cold_restore_test.go` | 1.2.2 | Cold restore integration tests |

### Files Modified (Phase 1)

| File | Epic | Changes |
|------|------|---------|
| `go.mod`, `go.sum` | 1.1.1 | Add gopsutil dependency |
| `session/storage.go` | 1.1.4, 1.3.1 | Add HistoryFilePath, Checkpoint fields to InstanceData |
| `session/instance.go` | 1.1.4, 1.2.1, 1.2.2, 1.3.2 | Add fields, CaptureCurrentState, cold restore logic, CreateCheckpoint |
| `session/scrollback/storage.go` | 1.3.2 | Add LatestSequence method |
| `server/dependencies.go` | 1.1.4 | Wire HistoryLinker into startup |
| `server/server.go` | 1.2.1 | Call CaptureCurrentState during shutdown |
| `server/services/session_service.go` | 1.3.3 | Add checkpoint RPC handlers |
| `server/adapters/instance_adapter.go` | 1.1.5, 1.3.3 | Populate new proto fields, checkpoint conversion |
| `proto/session/v1/types.proto` | 1.1.5, 1.3.3 | Add history fields, Checkpoint message |
| `proto/session/v1/session.proto` | 1.3.3 | Add checkpoint RPC methods |
| `web-app/src/lib/hooks/useSessionService.ts` | 1.3.4 | Add checkpoint hooks |
| `web-app/src/components/sessions/SessionCard.tsx` | 1.3.4 | Add checkpoint UI |

### Suggested Implementation Order

1. **Story 1.1.1** (gopsutil + ProcessInspector) — foundational dependency
2. **Story 1.1.2** (HistoryFileDetector) — uses ProcessInspector
3. **Story 1.3.1** (Checkpoint struct + storage fields) — independent, unblocks later stories
4. **Story 1.1.3** (HistoryFileWatcher) — independent, uses fsnotify
5. **Story 1.1.4** (HistoryLinker) — combines detector + watcher
6. **Story 1.2.1** (Shutdown capture) — independent
7. **Story 1.2.2** (Cold restore path) — depends on 1.1.4 for UUID availability
8. **Story 1.2.3** (Startup scan) — small addition to HistoryLinker
9. **Story 1.1.5** (Proto exposure) — depends on 1.1.4 for fields being populated
10. **Story 1.3.2** (Checkpoint creation service) — depends on 1.3.1
11. **Story 1.3.3** (Checkpoint RPC) — depends on 1.3.2
12. **Story 1.3.4** (Web UI) — depends on 1.3.3

### Testing Strategy

- **Unit tests**: Every new file gets a `_test.go` companion. Mock external dependencies (gopsutil, tmux).
- **Integration tests**: Cold restore test with real tmux session (requires tmux available in CI). Mark with build tag `//go:build integration`.
- **Manual test checklist**:
  1. Start a Claude session, verify history file detected within 10 seconds
  2. Restart stapler-squad (SIGTERM), verify hot resume (tmux alive)
  3. Kill tmux server, restart stapler-squad, verify cold restore with `--resume`
  4. Create checkpoint, verify it appears in checkpoint list
  5. Kill stapler-squad, verify `WorkingDir` was captured in storage
