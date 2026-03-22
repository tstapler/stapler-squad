# Conductor Feature Parity Plan

Comprehensive implementation plan for closing the feature gap between Stapler Squad and Conductor (conductor.build). Features are ordered by user value and implementation priority.

---

## Table of Contents

1. [Epic 1: Per-Turn Checkpoints](#epic-1-per-turn-checkpoints)
2. [Epic 2: Run Scripts / Dev Server Integration](#epic-2-run-scripts--dev-server-integration)
3. [Epic 3: Built-in Diff Viewer](#epic-3-built-in-diff-viewer)
4. [Epic 4: Spotlight Testing](#epic-4-spotlight-testing)
5. [Epic 5: MCP Integration](#epic-5-mcp-integration)
6. [Dependency Visualization](#dependency-visualization)
7. [Global Known Issues](#global-known-issues)

---

## Epic 1: Per-Turn Checkpoints

### Overview

**User Value**: Before each AI agent response, automatically snapshot the workspace state to a private git ref namespace. This gives users the ability to revert to any previous turn without polluting the branch history visible in `git log`. This is the single most differentiating feature from Conductor and addresses the fundamental problem of AI agents making irreversible changes.

**Success Metrics**:
- Checkpoint creation adds less than 500ms latency to turn detection
- Users can list and revert to any checkpoint within 2 seconds via the UI
- Zero impact on visible `git log` output (private ref namespace)
- Checkpoint storage uses less than 2x the base worktree disk usage

### ADR-001: Private Git Ref Namespace for Checkpoints

**Status**: Proposed

**Context**: We need to store per-turn workspace snapshots without polluting the user's git history. Options considered:
1. Stash-based storage (`git stash`)
2. Private ref namespace (`refs/stapler-squad/checkpoints/`)
3. Separate shadow repository
4. File-system level snapshots (APFS clones on macOS)

**Decision**: Use **private git ref namespace** at `refs/stapler-squad/checkpoints/<session-name>/<sequence>`.

**Rationale**:
- Git refs under custom namespaces are invisible to `git log`, `git branch`, and most standard git commands
- Refs survive garbage collection indefinitely (unlike dangling commits)
- Standard `git show`, `git diff`, and `git checkout` work against refs
- No additional tooling or filesystem features required
- Compatible with all git hosting providers (refs are not pushed by default)
- Cleanup is straightforward with `git for-each-ref --format='delete %(refname)' refs/stapler-squad/ | git update-ref --stdin`

**Consequences**:
- Slightly increased `.git` directory size (one commit object per checkpoint)
- Need to implement pruning for old checkpoints
- Non-worktree sessions (SessionTypeDirectory) must detect if path is a git repo

### ADR-002: Turn Detection via Controller Integration

**Status**: Proposed

**Context**: To create checkpoints "before each AI agent response," we need to detect when a turn boundary occurs. Options:
1. Poll terminal output for prompt patterns (existing `HasUpdated` + `detectAndTrackPrompt` flow)
2. Use ClaudeController's status transitions (Running -> Ready -> Running)
3. Hook into Claude Code's hook system (post-tool-use hooks)
4. Time-based periodic snapshots

**Decision**: Use **ClaudeController status transitions** as the primary trigger, with the existing `InstanceStatusManager` detecting `Running -> Ready` transitions (agent finished a response) and `Ready -> Running` transitions (agent starting new work). Checkpoint is created when state transitions to `Running` (i.e., just as the agent begins a new turn of work).

**Rationale**:
- ClaudeController already detects idle/prompt states with proven reliability
- Status transitions are the semantic representation of turn boundaries
- No new polling mechanisms needed -- piggybacks on existing infrastructure
- Works for all supported programs (claude, aider) that produce detectable prompts

**Consequences**:
- Checkpoint timing is approximate (triggered by status transition detection, not exact API boundaries)
- Programs without detectable prompts cannot trigger checkpoints
- Need to debounce rapid status transitions to avoid excessive checkpoints

### Story Breakdown

#### Story 1.1: Checkpoint Storage Layer
**As a** developer, **I want** workspace state saved to private git refs **so that** I can revert to any previous turn.

**Acceptance Criteria**:
- Given a session with a git worktree, when a checkpoint is triggered, then all uncommitted changes are committed to a ref under `refs/stapler-squad/checkpoints/<session>/<seq>`
- Given a checkpoint exists, when I query it, then I can see the diff between that checkpoint and the current state
- Given 50+ checkpoints exist, when I list them, then they are returned ordered by sequence number with timestamps
- Given a checkpoint ref, when I inspect it with `git show`, then it contains a valid commit with metadata (turn number, timestamp, trigger reason)

**Tasks**:

**Task 1.1.1: Implement CheckpointManager in session/git/**
- Duration: 2-4h
- Files: `session/git/checkpoint.go`, `session/git/checkpoint_test.go`
- Description: Create `CheckpointManager` struct with methods:
  - `CreateCheckpoint(worktreePath, sessionName string, metadata CheckpointMetadata) (Checkpoint, error)` -- stages all changes, creates commit on detached HEAD, stores as ref
  - `ListCheckpoints(repoPath, sessionName string) ([]Checkpoint, error)` -- lists refs under namespace
  - `GetCheckpoint(repoPath, sessionName string, seq int) (Checkpoint, error)` -- reads specific checkpoint
  - `DiffFromCheckpoint(worktreePath string, checkpoint Checkpoint) (string, error)` -- diff between checkpoint and current state
  - Uses atomic sequence counter stored in ref metadata
- INVEST Validation:
  - Independent: Pure git operations, no session lifecycle dependency
  - Negotiable: Metadata format can evolve
  - Valuable: Core persistence mechanism
  - Estimable: Well-defined git operations
  - Small: Single file, clear interface
  - Testable: Unit tests with git test fixtures

**Task 1.1.2: Implement Checkpoint Revert Logic**
- Duration: 2-4h
- Files: `session/git/checkpoint.go`, `session/git/checkpoint_test.go`
- Description: Add revert methods to `CheckpointManager`:
  - `RevertToCheckpoint(worktreePath string, checkpoint Checkpoint) error` -- hard resets worktree to checkpoint state
  - `RevertToCheckpointSoft(worktreePath string, checkpoint Checkpoint) error` -- applies reverse diff (preserves history)
  - `PruneCheckpoints(repoPath, sessionName string, keepLast int) error` -- delete old refs
  - `DeleteAllCheckpoints(repoPath, sessionName string) error` -- cleanup on session destroy
- INVEST Validation:
  - Independent: Builds on 1.1.1 but testable in isolation
  - Valuable: Core undo functionality
  - Testable: Revert outcomes verifiable via git status

**Task 1.1.3: Wire CheckpointManager into GitWorktreeManager**
- Duration: 1-2h
- Files: `session/git_worktree_manager.go`, `session/instance.go`
- Description: Add `checkpointManager *git.CheckpointManager` field to `GitWorktreeManager`. Add delegation methods to Instance:
  - `CreateCheckpoint(metadata) error`
  - `ListCheckpoints() ([]Checkpoint, error)`
  - `RevertToCheckpoint(seq int) error`
  - Initialize checkpoint manager in `setupFirstTimeWorktree()` and `FromInstanceData()`
- INVEST Validation:
  - Independent: Integration task, but narrow scope
  - Small: Thin delegation wrappers

#### Story 1.2: Automatic Checkpoint Triggering
**As a** developer, **I want** checkpoints created automatically before each agent turn **so that** I have a safety net without manual intervention.

**Acceptance Criteria**:
- Given a running session, when the agent transitions from Ready to Running, then a checkpoint is created if the worktree is dirty
- Given rapid status transitions (within 5 seconds), then at most one checkpoint is created (debouncing)
- Given a non-git session (SessionTypeDirectory without git), then no checkpoint is attempted and no error occurs
- Given checkpoint creation fails (e.g., permission error), then the agent turn proceeds normally and a warning is logged

**Tasks**:

**Task 1.2.1: Add Checkpoint Hook to ClaudeController**
- Duration: 2-3h
- Files: `session/claude_controller.go`, `session/checkpoint_hook.go`
- Description: Create `CheckpointHook` that subscribes to status transitions via the existing ClaudeController event flow. On `Ready -> Running` transition:
  1. Check if instance has a git worktree
  2. Check if worktree is dirty (skip if clean)
  3. Debounce: skip if last checkpoint was within 5 seconds
  4. Create checkpoint asynchronously (non-blocking to avoid delaying the agent)
  5. Log checkpoint creation or failure
- INVEST Validation:
  - Independent: Hook pattern decouples from controller internals
  - Valuable: Zero-effort checkpointing for users
  - Testable: Mock status transitions, verify checkpoint creation

**Task 1.2.2: Add Checkpoint Configuration Options**
- Duration: 1-2h
- Files: `config/config.go`, `session/git/checkpoint.go`
- Description: Add config fields:
  - `CheckpointsEnabled bool` (default: true for worktree sessions)
  - `CheckpointMaxCount int` (default: 50 per session)
  - `CheckpointDebounceMs int` (default: 5000)
  - Auto-prune when exceeding max count
- INVEST Validation:
  - Independent: Config-only changes
  - Small: Few fields, clear defaults

#### Story 1.3: Checkpoint API and UI
**As a** developer, **I want** to view and revert checkpoints from the web UI **so that** I can quickly undo agent changes.

**Acceptance Criteria**:
- Given a session with checkpoints, when I open the session detail, then I see a timeline of checkpoints with sequence numbers and timestamps
- Given a checkpoint in the timeline, when I click "View Diff," then I see the diff between that checkpoint and the current state
- Given a checkpoint in the timeline, when I click "Revert," then the workspace is restored to that checkpoint state and the agent session is restarted
- Given a revert operation, when I confirm it, then a new checkpoint is created before reverting (safety net)

**Tasks**:

**Task 1.3.1: Add Checkpoint Proto Definitions and RPC Endpoints**
- Duration: 2-3h
- Files: `proto/session/v1/session.proto`, `proto/session/v1/types.proto`, `server/services/checkpoint_service.go`
- Description: Add proto messages:
  - `Checkpoint` message (sequence, timestamp, metadata, dirty_files_count)
  - `ListCheckpointsRequest/Response`
  - `RevertToCheckpointRequest/Response`
  - `GetCheckpointDiffRequest/Response`
  - Implement service handler delegating to session.Instance methods
  - Run `make proto-gen` to regenerate
- INVEST Validation:
  - Independent: Proto definitions are self-contained
  - Valuable: Enables UI integration
  - Testable: RPC handler unit tests

**Task 1.3.2: Build Checkpoint Timeline UI Component**
- Duration: 3-4h
- Files: `web-app/src/components/sessions/CheckpointTimeline.tsx`, `web-app/src/components/sessions/CheckpointTimeline.module.css`
- Description: React component showing vertical timeline of checkpoints:
  - Each checkpoint shows: sequence number, timestamp, dirty file count
  - "View Diff" button opens diff in existing DiffViewer component
  - "Revert" button with confirmation dialog
  - Auto-refreshes when new checkpoints arrive (via WatchSessions event)
  - Integrated into SessionDetail panel as a new tab
- INVEST Validation:
  - Independent: Self-contained React component
  - Valuable: Primary user interaction surface
  - Testable: Component rendering tests

**Task 1.3.3: Integrate Checkpoint Timeline into Session Detail**
- Duration: 1-2h
- Files: `web-app/src/components/sessions/SessionDetail.tsx`
- Description: Add "Checkpoints" tab to the session detail panel. Wire up the CheckpointTimeline component with the session's ConnectRPC client. Show checkpoint count badge on the tab.

### Known Issues

**Concurrency Risk: Checkpoint Creation During Active Git Operations [SEVERITY: High]**

Description: If the agent is modifying files while a checkpoint is being created, the checkpoint may capture a partially-written state. The `git add .` + `git commit` in checkpoint creation is not atomic with respect to filesystem writes.

Mitigation:
- Create checkpoints on the `Ready -> Running` transition (agent is idle at checkpoint moment)
- Use `git stash create` (which is more atomic than add+commit) as the snapshot mechanism
- Then store the stash tree as a ref: `git update-ref refs/stapler-squad/checkpoints/... $(git stash create)`
- This avoids staging area conflicts with the agent's own git operations

Files Likely Affected: `session/git/checkpoint.go`

**Data Integrity Risk: Checkpoint Refs Accumulating Unbounded [SEVERITY: Medium]**

Description: Without pruning, long-running sessions could accumulate hundreds of checkpoint refs, consuming disk space and slowing ref enumeration.

Mitigation:
- Enforce `CheckpointMaxCount` (default 50) with FIFO eviction
- Auto-prune during checkpoint creation
- Add `PruneCheckpoints` to session cleanup/destroy flow
- Log pruning events for observability

Files Likely Affected: `session/git/checkpoint.go`, `session/instance.go` (Destroy method)

**Edge Case: Non-Git Directory Sessions [SEVERITY: Low]**

Description: `SessionTypeDirectory` sessions may not have a git repository. Checkpoint operations must gracefully no-op.

Mitigation:
- Guard all checkpoint operations with `HasWorktree()` or `IsGitRepo()` check
- Log debug message and return nil (not error) for non-git sessions
- Unit test the no-op path explicitly

---

## Epic 2: Run Scripts / Dev Server Integration

### Overview

**User Value**: Define lifecycle scripts (`setup`, `run`, `archive`) per workspace via a `stapler-squad.json` config file. The `run` script launches a dev server with allocated ports, enabling agents to test their changes against a running application. This closes the gap between writing code and validating it.

**Success Metrics**:
- Dev server starts within 10 seconds of session creation (for `setup` + `run`)
- Port allocation never conflicts between concurrent sessions
- Dev server health is monitored and reported in the UI
- `archive` script runs reliably on session pause/destroy

### ADR-003: Workspace Configuration File Format

**Status**: Proposed

**Context**: We need a way for users to declare per-workspace lifecycle scripts. Options:
1. JSON config file (`stapler-squad.json`) at workspace root
2. YAML config file
3. Global config in `~/.stapler-squad/config.json` with per-path overrides
4. `.stapler-squad/` directory with individual script files

**Decision**: Use a **`stapler-squad.json`** file at the workspace root, with optional global defaults in the existing `config.json`.

**Rationale**:
- JSON is already the config format for Stapler Squad (`config.json`)
- Root-level file is discoverable and version-controllable
- Per-workspace overrides are clean and intuitive
- No new dependencies (Go stdlib JSON)

**Format**:
```json
{
  "setup": "npm install",
  "run": "npm run dev -- --port $STAPLER_SQUAD_PORT",
  "archive": "npm run build",
  "run_mode": "concurrent",
  "port_count": 1,
  "health_check": {
    "url": "http://localhost:$STAPLER_SQUAD_PORT/health",
    "timeout_seconds": 30,
    "interval_seconds": 5
  }
}
```

**Consequences**:
- Users must create `stapler-squad.json` in their repo (not automatic)
- Config is tied to the repo, not the session (multiple sessions from same repo share config)
- `$STAPLER_SQUAD_PORT` environment variable convention must be documented

### Story Breakdown

#### Story 2.1: Workspace Config Parser
**As a** developer, **I want** to declare lifecycle scripts in a config file **so that** my dev server starts automatically with each session.

**Acceptance Criteria**:
- Given a `stapler-squad.json` at the workspace root, when a session starts, then the config is parsed and validated
- Given no config file exists, when a session starts, then it proceeds normally without errors
- Given an invalid config file, when a session starts, then a clear error message identifies the problem
- Given `$STAPLER_SQUAD_PORT` in a script, when the script runs, then the variable is replaced with the allocated port

**Tasks**:

**Task 2.1.1: Implement WorkspaceConfig Parser**
- Duration: 1-2h
- Files: `session/workspace_config.go`, `session/workspace_config_test.go`
- Description: Define `WorkspaceConfig` struct matching the JSON schema. Implement:
  - `LoadWorkspaceConfig(workspacePath string) (*WorkspaceConfig, error)` -- reads and validates `stapler-squad.json`
  - `Validate() error` -- checks required fields, valid run_mode values
  - RunMode enum: `concurrent` (one per session), `nonconcurrent` (shared across sessions)
  - Default values for optional fields
- INVEST Validation:
  - Independent: Pure parsing, no session dependency
  - Valuable: Foundation for all lifecycle scripts
  - Testable: Parse various JSON inputs

**Task 2.1.2: Implement Port Allocator**
- Duration: 2-3h
- Files: `session/port_allocator.go`, `session/port_allocator_test.go`
- Description: Thread-safe port allocator:
  - `PortAllocator` struct with configurable base port range (default 18000-18999)
  - `Allocate(sessionName string, count int) ([]int, error)` -- allocates N consecutive ports
  - `Release(sessionName string) error` -- releases ports when session ends
  - `IsAvailable(port int) bool` -- checks if port is free (also probes OS-level binding)
  - Persists allocations to `~/.stapler-squad/port_allocations.json` for crash recovery
  - Validates ports are not already in use at OS level via `net.Listen`
- INVEST Validation:
  - Independent: No session/git dependencies
  - Valuable: Prevents port conflicts
  - Testable: Concurrent allocation tests

#### Story 2.2: Lifecycle Script Executor
**As a** developer, **I want** my setup/run/archive scripts executed at the right times **so that** I don't have to manually start/stop services.

**Acceptance Criteria**:
- Given a `setup` script, when a session starts for the first time, then `setup` runs before the agent starts
- Given a `run` script, when setup completes, then `run` starts as a background process with `STAPLER_SQUAD_PORT` set
- Given a running dev server, when the session is paused or destroyed, then `archive` runs and the dev server is terminated
- Given a `run` script that crashes, when it is detected, then the failure is logged and the session continues (non-fatal)
- Given `run_mode: nonconcurrent`, when a second session starts in the same workspace, then it reuses the existing dev server

**Tasks**:

**Task 2.2.1: Implement ScriptRunner**
- Duration: 3-4h
- Files: `session/script_runner.go`, `session/script_runner_test.go`
- Description: `ScriptRunner` manages script lifecycle:
  - `RunSetup(ctx context.Context, config WorkspaceConfig, env map[string]string) error` -- synchronous, blocks until complete or timeout
  - `StartRunScript(ctx context.Context, config WorkspaceConfig, env map[string]string) (*RunProcess, error)` -- launches background process, returns handle
  - `RunArchive(ctx context.Context, config WorkspaceConfig, env map[string]string) error` -- synchronous
  - `RunProcess` struct: PID tracking, stdout/stderr capture to log files, health check goroutine
  - Scripts execute via `sh -c` with the workspace as working directory
  - Environment variables: `STAPLER_SQUAD_PORT`, `STAPLER_SQUAD_SESSION`, `STAPLER_SQUAD_WORKSPACE`
  - Timeout handling for setup/archive (default 60s)
- INVEST Validation:
  - Independent: Process management only, no git/tmux dependency
  - Valuable: Core execution engine
  - Testable: Test with shell scripts that echo/exit

**Task 2.2.2: Implement Health Check Monitor**
- Duration: 1-2h
- Files: `session/health_check.go`, `session/health_check_test.go`
- Description: Background goroutine for dev server health:
  - `HealthChecker` struct with configurable URL, timeout, interval
  - Polls health endpoint, tracks consecutive failures
  - Emits status events: `Healthy`, `Unhealthy`, `Crashed`
  - Integrates with `InstanceStatusManager` for UI reporting
  - Stops automatically when session pauses/destroys
- INVEST Validation:
  - Independent: HTTP polling only
  - Small: Single-purpose
  - Testable: Mock HTTP server

**Task 2.2.3: Wire Lifecycle Scripts into Session Start/Pause/Destroy**
- Duration: 2-3h
- Files: `session/instance.go`, `session/script_runner.go`
- Description: Integrate ScriptRunner into Instance lifecycle:
  - In `start()`: After worktree setup, load workspace config, run setup, start run script
  - In `Pause()`: Run archive script, stop run process, release ports
  - In `Destroy()`: Run archive script, stop run process, release ports
  - Add `scriptRunner *ScriptRunner` and `portAllocator *PortAllocator` fields to Instance
  - Add `runProcess *RunProcess` field for tracking the background dev server
  - Non-fatal: script failures log warnings but don't block session lifecycle
- INVEST Validation:
  - Dependent on 2.1.1, 2.1.2, 2.2.1
  - Valuable: Ties everything together
  - Small: Integration wiring, not new logic

#### Story 2.3: Dev Server Status in UI
**As a** developer, **I want** to see my dev server's status and port in the UI **so that** I know if my service is running.

**Acceptance Criteria**:
- Given a session with a run script, when viewing the session card, then I see the dev server status (Starting, Running, Unhealthy, Stopped)
- Given a running dev server, when viewing the session detail, then I see the allocated port(s) and health status
- Given a dev server URL, when I click it, then it opens in a new browser tab

**Tasks**:

**Task 2.3.1: Add Dev Server Proto Fields and RPC Endpoint**
- Duration: 1-2h
- Files: `proto/session/v1/types.proto`, `proto/session/v1/session.proto`, `server/services/session_service.go`
- Description: Add to Session proto message:
  - `DevServerStatus dev_server` field with status enum, port, health_url, last_health_check timestamp
  - Populate from Instance's run process state during session serialization
  - Run `make proto-gen`
- INVEST Validation:
  - Independent: Proto changes only
  - Small: Few fields

**Task 2.3.2: Build Dev Server Status UI Component**
- Duration: 2-3h
- Files: `web-app/src/components/sessions/DevServerBadge.tsx`, `web-app/src/components/sessions/DevServerBadge.module.css`
- Description: Badge component for session cards:
  - Color-coded status indicator (green=running, yellow=starting, red=unhealthy, gray=stopped)
  - Shows port number
  - Clickable URL opens dev server in new tab
  - Tooltip with last health check time
  - Integrated into SessionCard alongside existing diff stats badge
- INVEST Validation:
  - Independent: Self-contained component
  - Valuable: Immediate visibility
  - Testable: Component tests

### Known Issues

**Resource Leak: Orphaned Dev Server Processes [SEVERITY: High]**

Description: If Stapler Squad crashes or is killed with SIGKILL, the background dev server processes will be orphaned. They will continue running and holding ports.

Mitigation:
- Use process groups (`syscall.SysProcAttr{Setpgid: true}`) so child processes can be killed as a group
- On startup, scan `port_allocations.json` for stale allocations and verify if processes are still running
- Implement a reaper goroutine that periodically checks if allocated ports have live processes
- Document manual cleanup: `lsof -i :18000-18999` to find orphaned servers

Files Likely Affected: `session/script_runner.go`, `session/port_allocator.go`

**Concurrency Risk: Port Allocation Race Condition [SEVERITY: Medium]**

Description: Between checking if a port is available (`net.Listen`) and the dev server actually binding to it, another process could claim the port.

Mitigation:
- Use the `net.Listen` approach to actually bind the port, then pass the listener to the child process via file descriptor inheritance (complex)
- Simpler: Allocate from a reserved range (18000-18999) and retry on EADDRINUSE with exponential backoff
- Maximum 3 retries before reporting allocation failure

Files Likely Affected: `session/port_allocator.go`

**Edge Case: Nonconcurrent Mode Shared Server [SEVERITY: Medium]**

Description: When `run_mode: nonconcurrent`, multiple sessions share one dev server. If the "owner" session pauses, the server should not be killed if other sessions depend on it.

Mitigation:
- Implement reference counting for nonconcurrent servers
- Only stop the server when the last referencing session pauses/destroys
- Store reference count in `port_allocations.json`
- On crash recovery, reconcile reference counts with active sessions

Files Likely Affected: `session/script_runner.go`, `session/port_allocator.go`

---

## Epic 3: Built-in Diff Viewer

### Overview

**User Value**: Provide a rich, side-by-side diff viewer within the web UI for reviewing agent changes before merge. While a basic `DiffViewer.tsx` component already exists, it needs significant enhancement to match the quality expected for a primary code review surface.

**Success Metrics**:
- Diff renders within 1 second for files up to 10,000 lines
- Side-by-side and unified view modes available
- Syntax highlighting for common languages (JS/TS, Go, Python, Rust)
- File tree navigation for multi-file diffs

**Current State Analysis**: The existing `DiffViewer.tsx` (`web-app/src/components/sessions/DiffViewer.tsx`) parses unified diff format and renders inline hunks. It lacks:
- Side-by-side view
- Syntax highlighting
- File tree navigation
- Per-file collapse/expand
- Copy/download functionality

### Story Breakdown

#### Story 3.1: Enhanced Diff Parser and File Tree
**As a** developer, **I want** a file tree showing all changed files **so that** I can navigate large diffs efficiently.

**Acceptance Criteria**:
- Given a diff with 20+ files, when the diff viewer opens, then I see a collapsible file tree on the left
- Given a file in the tree, when I click it, then the diff scrolls to that file
- Given file stats, then each file shows +/- line counts and a change type badge (Added, Modified, Deleted, Renamed)
- Given a large diff, when the component mounts, then the file tree loads instantly (diff parsing is deferred per-file)

**Tasks**:

**Task 3.1.1: Refactor Diff Parser into Separate Module**
- Duration: 2-3h
- Files: `web-app/src/lib/diff-parser.ts`, `web-app/src/lib/diff-parser.test.ts`
- Description: Extract and enhance the `parseDiff` function from `DiffViewer.tsx` into a standalone module:
  - Parse file rename detection (`rename from`/`rename to` headers)
  - Parse binary file changes
  - Parse file mode changes (permissions)
  - Return structured `ParsedDiff` with `files: ParsedFile[]` containing metadata + hunks
  - Lazy hunk parsing: parse file headers eagerly, parse hunk content on demand
  - Add comprehensive test cases for edge cases (empty files, binary, renames)
- INVEST Validation:
  - Independent: Pure function, no React dependency
  - Valuable: Foundation for all diff features
  - Testable: Unit tests with real diff fixtures

**Task 3.1.2: Build FileTree Component**
- Duration: 2-3h
- Files: `web-app/src/components/sessions/DiffFileTree.tsx`, `web-app/src/components/sessions/DiffFileTree.module.css`
- Description: File tree sidebar component:
  - Tree structure from file paths (group by directory)
  - Expand/collapse directories
  - Per-file stats: additions, deletions, change type icon
  - Click to scroll to file in diff view
  - Keyboard navigation (arrow keys, Enter to select)
  - Sticky positioning within the diff viewer layout
- INVEST Validation:
  - Independent: Self-contained component
  - Valuable: Navigation for large diffs
  - Testable: Component rendering tests

#### Story 3.2: Side-by-Side View Mode
**As a** developer, **I want** a side-by-side diff view **so that** I can compare old and new code simultaneously.

**Acceptance Criteria**:
- Given a diff, when I toggle "Side by Side" mode, then old and new code appear in synchronized columns
- Given synchronized columns, when I scroll one side, then the other side scrolls to the matching line
- Given a line addition on the new side, then a blank placeholder appears on the old side (and vice versa for deletions)
- Given a unified view toggle, when I click it, then the view switches to standard inline diff

**Tasks**:

**Task 3.2.1: Implement Side-by-Side Diff Renderer**
- Duration: 3-4h
- Files: `web-app/src/components/sessions/DiffSplitView.tsx`, `web-app/src/components/sessions/DiffSplitView.module.css`
- Description: Two-column diff renderer:
  - Split hunks into left (old) and right (new) line arrays
  - Align additions/deletions with empty placeholders
  - Synchronized scroll via `onScroll` event binding
  - Line numbers on both sides
  - Gutter with change indicators (+/-/~)
  - Virtual scrolling for large files (react-window or intersection observer)
- INVEST Validation:
  - Independent: Self-contained renderer
  - Valuable: Primary review mode
  - Testable: Snapshot tests for alignment

**Task 3.2.2: Add View Mode Toggle and Layout Shell**
- Duration: 2-3h
- Files: `web-app/src/components/sessions/DiffViewer.tsx`, `web-app/src/components/sessions/DiffViewer.module.css`
- Description: Refactor existing DiffViewer to be a layout shell:
  - Toolbar with: view mode toggle (unified/split), expand all/collapse all, file filter
  - File tree on left (from 3.1.2)
  - Diff content on right (unified or split from 3.2.1)
  - Store view mode preference in localStorage
  - Responsive: collapse file tree on narrow viewports
- INVEST Validation:
  - Dependent on 3.1.2 and 3.2.1
  - Valuable: Ties everything together
  - Small: Layout/wiring only

#### Story 3.3: Syntax Highlighting
**As a** developer, **I want** syntax-highlighted diffs **so that** code is easier to read.

**Acceptance Criteria**:
- Given a diff of a `.tsx` file, when it renders, then JSX/TypeScript tokens are syntax highlighted
- Given a diff of a `.go` file, when it renders, then Go keywords and types are highlighted
- Given an unknown file type, when it renders, then it falls back to plain text (no errors)

**Tasks**:

**Task 3.3.1: Integrate Syntax Highlighting Library**
- Duration: 2-3h
- Files: `web-app/src/lib/syntax-highlight.ts`, `web-app/src/components/sessions/DiffSplitView.tsx`, `web-app/src/components/sessions/DiffViewer.tsx`
- Description: Integrate `highlight.js` or `shiki` (wasm-based, more accurate):
  - Detect language from file extension
  - Apply highlighting to diff line content (after diff markers are stripped)
  - Preserve diff line coloring (green/red background) with syntax colors overlaid
  - Lazy load language grammars (only load when file type is encountered)
  - Support at minimum: JavaScript, TypeScript, Go, Python, Rust, Java, CSS, HTML, JSON, YAML, Markdown
- INVEST Validation:
  - Independent: Library integration
  - Valuable: Significant readability improvement
  - Testable: Visual snapshot tests

### Known Issues

**Performance Risk: Large Diff Rendering [SEVERITY: Medium]**

Description: Diffs with 10,000+ lines can cause browser lag if rendered as a single DOM tree. Syntax highlighting adds additional overhead per line.

Mitigation:
- Implement virtual scrolling (react-window) to only render visible lines
- Defer syntax highlighting to a web worker
- Lazy-parse hunks (only parse when file is expanded/scrolled to)
- Set a maximum diff size limit (e.g., 50,000 lines) with a "diff too large" message

Files Likely Affected: `web-app/src/components/sessions/DiffSplitView.tsx`

**Edge Case: Binary File Diffs [SEVERITY: Low]**

Description: Binary files in diffs produce unparseable content. The parser must detect and handle these gracefully.

Mitigation:
- Detect `Binary files ... differ` in diff output
- Show "Binary file changed" placeholder instead of attempting to render
- Already partially handled in existing parser but needs explicit test coverage

---

## Epic 4: Spotlight Testing (Experimental)

### Overview

**User Value**: Allow agents to run tests against the main repository's build artifacts by syncing worktree changes back to the repo root without performing a full merge. This avoids the cost of a complete build in the worktree while still validating changes.

**Priority**: Lower priority -- experimental feature. Implementation should be simple and non-invasive.

### High-Level Architecture

The spotlight testing approach:
1. User triggers "Spotlight Test" on a session
2. System creates a temporary overlay of the worktree changes on top of the main repo
3. Tests run in the main repo context with the overlaid changes
4. Results are captured and displayed in the session detail
5. The overlay is removed (main repo is restored)

**Implementation Approach**: Use `git stash` + `git stash apply` pattern on the main repo:
1. In the main repo: `git stash` any existing changes
2. Copy changed files from worktree to main repo (via `git diff --name-only` + file copy)
3. Run the test command in the main repo
4. Restore main repo: `git checkout .` + `git stash pop`

**Key Risks**:
- Main repo corruption if restore fails (mitigated by creating a safety commit before overlay)
- Test pollution if tests write to the filesystem
- Race condition if multiple sessions spotlight-test simultaneously (serialize via mutex)

### Stories (High-Level)

1. **Story 4.1**: Implement `SpotlightSync` that overlays worktree changes onto the main repo with safety commit and rollback
2. **Story 4.2**: Implement `SpotlightTestRunner` that executes a test command in the main repo context and captures output
3. **Story 4.3**: Add `SpotlightTest` RPC endpoint and UI button in session detail
4. **Story 4.4**: Add serialization mutex to prevent concurrent spotlight tests on the same repo

### Known Issues

**Data Integrity Risk: Main Repo Corruption [SEVERITY: Critical]**

Description: If the overlay application or restoration fails (e.g., due to merge conflicts, disk full, process crash), the main repo could be left in a dirty state. This is the user's primary working copy.

Mitigation:
- Create a safety commit before applying overlay: `git commit -m "spotlight-safety-<timestamp>" --allow-empty`
- After test, hard reset to safety commit: `git reset --hard spotlight-safety-<hash>`
- Delete safety commit afterward
- Use file-level locking to prevent concurrent spotlight operations on the same repo
- Clearly warn users in the UI that this is experimental

---

## Epic 5: MCP Integration

### Overview

**User Value**: Surface MCP (Model Context Protocol) server configuration in the UI. Allow users to view which MCP servers are connected to their Claude Code sessions and manage them.

**Priority**: Lowest priority -- nice-to-have for visibility into agent capabilities.

### High-Level Architecture

MCP server configuration lives in Claude Code's own config files:
- `~/.claude/claude_desktop_config.json` (global)
- `.claude/settings.json` (per-project)

Stapler Squad already has `GetClaudeConfig`/`ListClaudeConfigs`/`UpdateClaudeConfig` RPC endpoints that can read and write these files.

### Stories (High-Level)

1. **Story 5.1**: Parse MCP server entries from Claude config files (both global and per-project)
2. **Story 5.2**: Add `ListMCPServers` RPC endpoint returning server name, transport type, status
3. **Story 5.3**: Build MCPServersPanel UI component showing connected servers per session
4. **Story 5.4**: Allow enabling/disabling MCP servers per session via config file updates

### Known Issues

**Integration Risk: Claude Code Config Format Changes [SEVERITY: Medium]**

Description: The MCP server configuration format in Claude Code's settings files may change across versions. Parsing assumptions could break.

Mitigation:
- Implement defensive parsing with graceful fallback
- Version-detect Claude Code and adapt parser accordingly
- Treat MCP panel as informational (read-only preferred) to minimize write-related breakage

---

## Dependency Visualization

```
Epic 1: Per-Turn Checkpoints
    Task 1.1.1: CheckpointManager (git ops)
         |
    Task 1.1.2: Revert Logic
         |
    Task 1.1.3: Wire into GitWorktreeManager
         |          \
    Task 1.2.1: Hook into ClaudeController ---- Task 1.2.2: Config Options
         |
    Task 1.3.1: Proto + RPC Endpoints
         |
    Task 1.3.2: Checkpoint Timeline UI
         |
    Task 1.3.3: Integrate into SessionDetail

Epic 2: Run Scripts / Dev Server
    Task 2.1.1: WorkspaceConfig Parser ---- Task 2.1.2: Port Allocator
                    \                           /
    Task 2.2.1: ScriptRunner ---- Task 2.2.2: Health Check Monitor
                    |
    Task 2.2.3: Wire into Instance Lifecycle
                    |
    Task 2.3.1: Proto Fields ---- Task 2.3.2: Dev Server Badge UI

Epic 3: Diff Viewer Enhancement
    Task 3.1.1: Refactored Diff Parser
         |
    Task 3.1.2: FileTree Component
         |          \
    Task 3.2.1: Side-by-Side Renderer ---- Task 3.3.1: Syntax Highlighting
                    |
    Task 3.2.2: Layout Shell + View Toggle

Epic 4: Spotlight Testing (depends on Epic 1 checkpoint safety patterns)
    [High-level stories, not broken into tasks]

Epic 5: MCP Integration (independent)
    [High-level stories, not broken into tasks]
```

### Cross-Epic Dependencies

- **Epic 4 (Spotlight Testing) depends on Epic 1**: Uses checkpoint safety-commit pattern for main repo protection
- **Epic 3 (Diff Viewer) enhances Epic 1**: Checkpoint diff viewing reuses the enhanced DiffViewer
- **Epic 2 (Run Scripts) independent**: Can be developed in parallel with others
- **Epic 5 (MCP) independent**: Can be developed at any time

### Recommended Implementation Order

**Phase 1** (Parallel):
- Epic 1, Stories 1.1 + 1.2 (Checkpoint storage + auto-triggering)
- Epic 3, Story 3.1 (Diff parser + file tree)

**Phase 2** (Parallel):
- Epic 1, Story 1.3 (Checkpoint UI)
- Epic 3, Stories 3.2 + 3.3 (Side-by-side + syntax highlighting)
- Epic 2, Stories 2.1 + 2.2 (Config parser + script runner)

**Phase 3**:
- Epic 2, Story 2.3 (Dev server UI)
- Epic 4 (Spotlight testing -- experimental)

**Phase 4**:
- Epic 5 (MCP integration)

---

## Global Known Issues

### Cross-Cutting Concerns

**Git Operation Serialization [SEVERITY: High]**

Description: Multiple features (checkpoints, diffs, spotlight testing) perform git operations on the same worktree. Concurrent git operations on the same repository can cause lock contention (`index.lock` errors) and corrupt state.

Mitigation:
- Implement a per-worktree operation mutex in `GitWorktreeManager`
- All git operations (checkpoint, diff, commit, revert) acquire the mutex before executing
- Use `context.Context` with timeout to prevent indefinite blocking
- Log contention events for observability

Files Likely Affected: `session/git_worktree_manager.go`

**Disk Space Monitoring [SEVERITY: Medium]**

Description: Checkpoints, dev server logs, and worktree data all consume disk space. Without monitoring, users could run out of space silently.

Mitigation:
- Add a periodic disk usage check in the health check system
- Warn users when `~/.stapler-squad/` exceeds a configurable threshold (default 5GB)
- Auto-prune oldest checkpoints when disk pressure is detected
- Add disk usage to the debug snapshot

**Test Coverage for New Git Operations [SEVERITY: Medium]**

Description: New git operations (checkpoint creation, revert, spotlight overlay) are difficult to test without real git repositories. Mocking at the git level risks missing integration issues.

Mitigation:
- Create a shared test fixture that initializes a real git repo with commits in a temp directory
- Use `testing.T.TempDir()` for automatic cleanup
- Test against both regular repos and worktrees
- Add integration test tag for CI: `go test -tags=integration ./session/git/...`

### Bug Prevention Checklist

- [ ] All new git operations acquire per-worktree mutex
- [ ] Checkpoint creation uses `git stash create` (not `git add . && git commit`) to avoid staging area conflicts
- [ ] Port allocator validates OS-level port availability, not just internal tracking
- [ ] Script runner uses process groups for reliable child cleanup
- [ ] Dev server health check respects context cancellation on session destroy
- [ ] Diff parser handles binary files, renames, and empty diffs without panic
- [ ] Side-by-side view uses virtual scrolling for diffs over 1000 lines
- [ ] All new proto fields have backward-compatible default values
- [ ] Spotlight testing creates safety commit before modifying main repo
- [ ] All new config fields have sensible defaults and migration from older config versions
