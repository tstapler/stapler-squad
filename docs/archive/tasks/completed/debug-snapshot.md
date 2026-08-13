# Feature Plan: Debug Snapshot

**Date**: 2026-03-13
**Status**: Draft
**Scope**: One-click diagnostic capture for troubleshooting terminal status detection and session management issues

---

## Table of Contents

- [Problem Statement](#problem-statement)
- [Research Findings: Current Implementation](#research-findings-current-implementation)
- [ADR-015: Debug Snapshot Architecture](#adr-015-debug-snapshot-architecture)
- [Architecture Overview](#architecture-overview)
- [Data Model: Snapshot JSON Schema](#data-model-snapshot-json-schema)
- [Story 1: Backend Snapshot Collector Service](#story-1-backend-snapshot-collector-service)
- [Story 2: ConnectRPC Endpoint](#story-2-connectrpc-endpoint)
- [Story 3: Web UI Integration](#story-3-web-ui-integration)
- [Known Issues and Bug Risks](#known-issues-and-bug-risks)
- [Testing Strategy](#testing-strategy)
- [Dependency Graph](#dependency-graph)

---

## Problem Statement

When users encounter problems with Stapler Squad -- terminal status detection issues, sessions stuck in wrong states, tmux integration failures, or approval flow glitches -- there is no easy way to capture the full system state for diagnosis. Users must manually SSH in, run multiple `tmux` commands, read log files, and try to reconstruct what happened. This is error-prone and time-consuming.

A one-click "Debug Snapshot" button in the existing web UI debug menu should gather all relevant diagnostic information into a single well-structured JSON file that can be shared with maintainers or analyzed offline.

**User goals:**
1. Capture all relevant system state with a single click from the debug menu.
2. Optionally attach a short note describing the problem being observed.
3. Receive a structured JSON file in `~/.stapler-squad/logs/` that is self-contained and shareable.
4. No disruption to running sessions -- the snapshot must be read-only and non-destructive.

---

## Research Findings: Current Implementation

### Debug Menu (Web UI)

**File**: `web-app/src/components/ui/DebugMenu.tsx`

The debug menu is a modal overlay opened via the wrench button in the header. It currently contains:
- **Notifications** toggle (session notifications on/off)
- **Logging** toggle (terminal stream debug logging)
- **Debug Pages** link (Escape Code Analytics at `/debug/escape-codes`)
- **Console Commands** section (localStorage commands)

The menu uses CSS module styling (`DebugMenu.module.css`) with sections, toggle rows, and a footer with a "Done" button. It accepts `isOpen` and `onClose` props. Adding a new "Diagnostics" section with a button and optional text input fits naturally into this component.

### ConnectRPC Service Definition

**File**: `proto/session/v1/session.proto`

The `SessionService` already has 30+ RPCs. Adding a `CreateDebugSnapshot` RPC follows the same pattern as existing endpoints like `GetLogs` or `RestartSession`. The service uses ConnectRPC with the handler pattern:

```
path, handler := sessionv1connect.NewSessionServiceHandler(deps.SessionService, ConnectOptions()...)
```

All RPC handlers are methods on `SessionService` in `server/services/session_service.go`, which has access to `storage`, `eventBus`, `reviewQueuePoller`, `approvalStore`, `externalDiscovery`, and `searchSvc`.

### Session State Access

**File**: `server/services/session_service.go` (lines 239-292, ListSessions pattern)

Live session instances are accessed through `s.reviewQueuePoller.GetInstances()` which returns `[]*session.Instance`. Each instance exposes:
- Status, Title, Path, Branch, Program, Tags, Category
- `ToInstanceData()` for full serialization
- `CapturePaneContent()` / `CapturePaneContentRaw()` for tmux pane output
- `GetDiffStats()` for git diff information
- Terminal timestamps: `LastTerminalUpdate`, `LastMeaningfulOutput`
- `GetClaudeSession()` for Claude Code session metadata

### tmux Command Execution

**File**: `session/tmux/tmux.go` (lines 241-257)

tmux commands are built via `t.buildTmuxCommand(args...)` which handles server socket isolation (`-L` flag). Key commands for diagnostics:
- `tmux list-sessions` -- list all tmux sessions
- `tmux list-panes -t <session>` -- list panes in a session
- `tmux capture-pane -p -t <session>` -- capture pane content
- `tmux display-message -p -t <session> "#{...}"` -- query session metadata

The `TmuxSession` struct stores `sanitizedName` and `serverSocket` for each session.

### Approval Store

**File**: `server/services/approval_store.go`

`ApprovalStore.ListAll()` returns all `[]*PendingApproval` with fields: ID, SessionID, ClaudeSessionID, ToolName, ToolInput, Cwd, PermissionMode, CreatedAt, ExpiresAt. Thread-safe via `sync.RWMutex`.

### Log System

**File**: `log/log.go`

Logs are written to `~/.stapler-squad/logs/claudesquad.log` with rotating backup via `lumberjack`. The `GetLogDir()` function returns the log directory path. `GetLogFilePath()` returns the full log file path. The file format is:
```
[instance-id] LEVEL:YYYY/MM/DD HH:MM:SS file.go:line: message
```

### Server Dependencies

**File**: `server/dependencies.go`

`ServerDependencies` struct holds all wired components. `BuildDependencies()` constructs them in order. The `SessionService` has getters: `GetStorage()`, `GetApprovalStore()`, `GetEventBus()`, `GetReviewQueueInstance()`.

---

## ADR-015: Debug Snapshot Architecture

### Context

We need to gather diagnostic data from multiple subsystems (session store, tmux, approval store, server logs) into a single JSON file. The operation must be non-destructive and complete within a reasonable timeout (30 seconds).

### Decision

**Approach: Server-side collector with single RPC endpoint**

A new `CreateDebugSnapshot` unary RPC on `SessionService` triggers a server-side collector that gathers all diagnostic data, writes it to a JSON file in the log directory, and returns the file path and a summary to the client.

### Rationale

1. **Server-side collection** is required because the data sources (tmux sessions, log files, approval store) are only accessible from the server process. The web UI cannot directly execute tmux commands or read server-side files.

2. **Single RPC** keeps the API simple. The client sends a request with an optional note, the server does all the work, and returns a response with the file path. No streaming or multi-step protocol needed.

3. **JSON file output** (rather than returning the full snapshot in the RPC response) avoids large payload issues with ConnectRPC, allows offline analysis, and keeps the snapshot available after the server restarts.

4. **Timeout per subsystem** ensures that a slow tmux command does not block the entire snapshot. Each collection step has its own context with a 5-second timeout. Partial results are still written if some subsystems time out.

### Alternatives Considered

- **Client-side collection via multiple RPCs**: Would require 5+ sequential RPC calls from the frontend, adding latency and complexity. Rejected because tmux commands must run server-side.
- **Streaming RPC**: Over-engineered for a diagnostic dump that completes in seconds. Rejected.
- **Background job with polling**: Adds unnecessary complexity for an operation that takes < 30 seconds. Rejected.

### Consequences

- The snapshot file may be large (1-5 MB) if sessions have significant scrollback. This is acceptable for a diagnostic tool.
- tmux commands run during the snapshot may briefly contend with the status monitor. The 5-second per-subsystem timeout mitigates this.
- The log directory may accumulate snapshot files over time. Users should periodically clean old snapshots (out of scope for this feature).

---

## Architecture Overview

```
Web UI (DebugMenu.tsx)                    Server
  |                                         |
  |-- CreateDebugSnapshot(note) ---------->|
  |                                         |-- Collect session metadata
  |                                         |-- Capture tmux state (list-sessions, capture-pane)
  |                                         |-- Read scrollback buffers
  |                                         |-- Read approval store state
  |                                         |-- Read recent log lines
  |                                         |-- Write JSON to ~/.stapler-squad/logs/
  |                                         |
  |<--- { file_path, summary, timestamp } --|
  |                                         |
  |  Display success toast with file path   |
```

### Components Modified

| Layer | File | Change |
|-------|------|--------|
| Proto | `proto/session/v1/session.proto` | Add `CreateDebugSnapshot` RPC, request/response messages |
| Backend | `server/services/debug_snapshot.go` | New file: snapshot collector logic |
| Backend | `server/services/session_service.go` | Add `CreateDebugSnapshot` handler method |
| Frontend | `web-app/src/components/ui/DebugMenu.tsx` | Add "Diagnostics" section with snapshot button |
| Frontend | `web-app/src/components/ui/DebugMenu.module.css` | Styles for new button, input, and status |

### Components Read-Only (no changes)

| Component | Purpose |
|-----------|---------|
| `session/instance.go` | `ToInstanceData()`, `CapturePaneContent()`, `CapturePaneContentRaw()` |
| `session/tmux/tmux.go` | tmux command execution |
| `server/services/approval_store.go` | `ListAll()` for pending approvals |
| `log/log.go` | `GetLogFilePath()`, `GetLogDir()` for log file paths |
| `server/dependencies.go` | Dependency wiring (no changes -- uses existing SessionService getters) |

---

## Data Model: Snapshot JSON Schema

The snapshot file is written to `~/.stapler-squad/logs/debug-snapshot-{timestamp}.json` where `{timestamp}` is formatted as `20260313-143025` (YYYYMMDD-HHMMSS).

```json
{
  "version": 1,
  "timestamp": "2026-03-13T14:30:25Z",
  "note": "Terminal stuck showing Running but session appears idle",
  "server": {
    "pid": 12345,
    "uptime_seconds": 3600,
    "go_version": "go1.23.0",
    "os": "darwin",
    "arch": "arm64"
  },
  "sessions": [
    {
      "title": "my-feature",
      "status": "Running",
      "program": "claude",
      "path": "/Users/dev/my-project",
      "branch": "feature-branch",
      "session_type": "new_worktree",
      "category": "Work",
      "tags": ["Frontend", "Urgent"],
      "created_at": "2026-03-13T10:00:00Z",
      "updated_at": "2026-03-13T14:20:00Z",
      "last_terminal_update": "2026-03-13T14:25:00Z",
      "last_meaningful_output": "2026-03-13T14:22:00Z",
      "last_output_signature": "abc123...",
      "last_viewed": "2026-03-13T14:15:00Z",
      "last_acknowledged": "2026-03-13T14:10:00Z",
      "terminal_dimensions": { "height": 24, "width": 80 },
      "diff_stats": { "added": 15, "removed": 3 },
      "pane_content": "$ claude\n[Claude is thinking...]\n...",
      "pane_content_raw": "\u001b[0m$ claude\n...",
      "pane_content_truncated": false,
      "instance_type": "managed",
      "github_pr_number": 0
    }
  ],
  "tmux": {
    "list_sessions_output": "claudesquad_my-feature: 1 windows (created ...)\n...",
    "per_session": [
      {
        "tmux_session_name": "claudesquad_my-feature",
        "list_panes_output": "0: [80x24] [history 1500/2000, 98304 bytes]",
        "pane_content": "$ claude\n..."
      }
    ]
  },
  "approvals": {
    "pending_count": 1,
    "pending": [
      {
        "id": "uuid-here",
        "session_id": "my-feature",
        "tool_name": "Bash",
        "tool_input": { "command": "rm -rf /tmp/test" },
        "cwd": "/Users/dev/my-project",
        "created_at": "2026-03-13T14:28:00Z",
        "expires_at": "2026-03-13T14:32:00Z"
      }
    ]
  },
  "recent_logs": {
    "log_file_path": "/Users/dev/.stapler-squad/logs/claudesquad.log",
    "line_count": 200,
    "lines": [
      "[pid-12345-1710000000] INFO:2026/03/13 14:30:00 session_service.go:100: Session started",
      "..."
    ]
  },
  "errors": [
    "tmux capture-pane failed for session 'stale-session': exit status 1"
  ]
}
```

### Schema Design Decisions

- **`version` field**: Allows future schema evolution without breaking parsers.
- **`errors` array**: Partial failures (e.g., tmux command timeout for one session) are recorded here rather than aborting the entire snapshot.
- **`pane_content` vs `pane_content_raw`**: Plain text (for human reading) and ANSI-escaped (for terminal replay tools). Both are captured because terminal status detection bugs often involve escape code parsing.
- **`recent_logs`**: Last 200 lines from the main log file. Not the full file -- enough for recent context without bloating the snapshot.
- **`server` block**: Runtime metadata (PID, Go version, OS) helps correlate with deployment environment.

---

## Story 1: Backend Snapshot Collector Service

**Goal**: Create the server-side logic that gathers all diagnostic data and writes the JSON file.
**Duration**: 3-4 hours
**Dependencies**: None (reads from existing subsystems)

### Task 1.1: Create `server/services/debug_snapshot.go` [2h]

**Scope**: New file implementing the snapshot collector.

**Key Types**:
```go
// DebugSnapshot is the top-level JSON structure written to disk.
type DebugSnapshot struct {
    Version    int                    `json:"version"`
    Timestamp  time.Time              `json:"timestamp"`
    Note       string                 `json:"note,omitempty"`
    Server     ServerInfo             `json:"server"`
    Sessions   []SessionSnapshot      `json:"sessions"`
    Tmux       TmuxSnapshot           `json:"tmux"`
    Approvals  ApprovalSnapshot       `json:"approvals"`
    RecentLogs RecentLogsSnapshot     `json:"recent_logs"`
    Errors     []string               `json:"errors"`
}
```

**Implementation approach**:
1. `CollectSnapshot(ctx context.Context, note string, instances []*session.Instance, approvalStore *ApprovalStore) (*DebugSnapshot, error)` -- main collector function.
2. Each subsystem collection runs with a derived context using `context.WithTimeout(ctx, 5*time.Second)`.
3. `collectSessionSnapshots()` -- iterates instances, calls `ToInstanceData()` for metadata, `CapturePaneContent()` and `CapturePaneContentRaw()` for terminal output. Errors per-session are appended to `Errors` array, not fatal.
4. `collectTmuxSnapshot()` -- runs `tmux list-sessions` via `exec.CommandContext`, then `tmux list-panes` and `tmux capture-pane` per session. Uses the default tmux server (no `-L` flag) since we want all tmux sessions visible, not just isolated ones.
5. `collectApprovalSnapshot()` -- calls `approvalStore.ListAll()`, serializes each `PendingApproval`.
6. `collectRecentLogs(logFilePath string, lineCount int)` -- reads the last N lines from the log file using a reverse-read approach or tail-like logic.
7. `collectServerInfo()` -- gathers PID (`os.Getpid()`), Go version (`runtime.Version()`), OS (`runtime.GOOS`), arch (`runtime.GOARCH`), uptime (computed from a package-level start time).
8. `WriteSnapshot(snapshot *DebugSnapshot, dir string) (string, error)` -- marshals to indented JSON, writes to `debug-snapshot-{timestamp}.json`, returns the file path.

**Files created**:
- `server/services/debug_snapshot.go`

**Success criteria**:
- `CollectSnapshot` returns a complete `DebugSnapshot` struct even if individual subsystems fail.
- `WriteSnapshot` produces valid, pretty-printed JSON.
- Per-subsystem timeouts prevent the snapshot from hanging indefinitely.
- Errors from individual subsystem collections are recorded in `Errors` array, not propagated as fatal errors.

### Task 1.2: Add unit tests for snapshot collector [1.5h]

**Scope**: Test the collector logic with mock data.

**Files created**:
- `server/services/debug_snapshot_test.go`

**Test cases**:
- `TestCollectServerInfo` -- verifies PID, Go version, OS, arch are populated.
- `TestCollectRecentLogs` -- tests log tail reading with a temporary log file.
- `TestCollectRecentLogs_MissingFile` -- verifies graceful handling when log file does not exist.
- `TestCollectApprovalSnapshot` -- verifies serialization of pending approvals from a test ApprovalStore.
- `TestWriteSnapshot` -- verifies JSON file is written with correct naming convention and valid JSON content.
- `TestCollectSnapshot_PartialFailure` -- verifies that if tmux commands fail, the snapshot still contains session metadata and logs.

**Success criteria**:
- All tests pass with `go test ./server/services/ -run TestDebugSnapshot -v`.
- No real tmux sessions or log files required (uses mocks/temp files).

---

## Story 2: ConnectRPC Endpoint

**Goal**: Define the protobuf messages and wire the RPC handler.
**Duration**: 1.5-2 hours
**Dependencies**: Story 1

### Task 2.1: Add proto definitions [0.5h]

**Scope**: Add `CreateDebugSnapshot` RPC to `session.proto`.

**File modified**: `proto/session/v1/session.proto`

**Additions**:
```protobuf
// CreateDebugSnapshot captures diagnostic information and writes it to a JSON file.
// Gathers session state, tmux info, pending approvals, and recent logs.
rpc CreateDebugSnapshot(CreateDebugSnapshotRequest) returns (CreateDebugSnapshotResponse) {}
```

```protobuf
message CreateDebugSnapshotRequest {
  // Optional user note describing the issue being diagnosed.
  optional string note = 1;

  // Optional: Maximum number of recent log lines to include (default: 200).
  optional int32 log_lines = 2;
}

message CreateDebugSnapshotResponse {
  // Absolute path to the written snapshot JSON file.
  string file_path = 1;

  // Human-readable summary (e.g., "Captured 5 sessions, 2 pending approvals, 200 log lines").
  string summary = 2;

  // Timestamp when the snapshot was created.
  google.protobuf.Timestamp timestamp = 3;

  // Size of the snapshot file in bytes.
  int64 file_size_bytes = 4;
}
```

**Post-edit**: Run `make proto-gen` to regenerate Go and TypeScript bindings.

**Success criteria**:
- `make proto-gen` completes without errors.
- Generated Go types visible in `gen/proto/go/session/v1/`.
- Generated TypeScript types visible in `web-app/src/gen/`.

### Task 2.2: Implement RPC handler [1h]

**Scope**: Add `CreateDebugSnapshot` method to `SessionService`.

**File modified**: `server/services/session_service.go`

**Implementation**:
```go
func (s *SessionService) CreateDebugSnapshot(
    ctx context.Context,
    req *connect.Request[sessionv1.CreateDebugSnapshotRequest],
) (*connect.Response[sessionv1.CreateDebugSnapshotResponse], error) {
    // 1. Get live instances from poller (same pattern as ListSessions)
    // 2. Determine log line count (default 200)
    // 3. Get log file path from config
    // 4. Call CollectSnapshot(ctx, note, instances, approvalStore)
    // 5. Get log directory for output
    // 6. Call WriteSnapshot(snapshot, logDir)
    // 7. Build summary string
    // 8. Return response with file path, summary, timestamp, file size
}
```

The handler uses `s.reviewQueuePoller.GetInstances()` for live session data (same safe pattern as `ListSessions`) and `s.approvalStore.ListAll()` for pending approvals.

**Success criteria**:
- `go build ./...` passes after adding the handler.
- RPC is callable via ConnectRPC client (manual test with `curl` or generated client).
- Response includes valid file path, summary, and file size.

---

## Story 3: Web UI Integration

**Goal**: Add snapshot button to the existing debug menu with optional note input and feedback.
**Duration**: 1.5-2 hours
**Dependencies**: Story 2

### Task 3.1: Add "Diagnostics" section to DebugMenu [1h]

**Scope**: Add a new section between "Debug Pages" and "Console Commands" in the debug menu.

**File modified**: `web-app/src/components/ui/DebugMenu.tsx`

**Implementation**:

New section structure:
```tsx
<div className={styles.section}>
  <h3 className={styles.sectionTitle}>Diagnostics</h3>

  {/* Optional note input */}
  <div className={styles.noteInputRow}>
    <input
      type="text"
      className={styles.noteInput}
      placeholder="Describe the issue (optional)..."
      value={snapshotNote}
      onChange={(e) => setSnapshotNote(e.target.value)}
      maxLength={500}
      aria-label="Snapshot note"
    />
  </div>

  {/* Snapshot button */}
  <button
    className={styles.snapshotButton}
    onClick={handleCreateSnapshot}
    disabled={isCapturing}
    aria-label="Capture debug snapshot"
  >
    {isCapturing ? (
      <>
        <span className={styles.spinner} /> Capturing...
      </>
    ) : (
      <>Capture Debug Snapshot</>
    )}
  </button>

  {/* Result display */}
  {snapshotResult && (
    <div className={styles.snapshotResult}>
      <div className={styles.snapshotResultText}>
        {snapshotResult.summary}
      </div>
      <code className={styles.snapshotFilePath}>
        {snapshotResult.filePath}
      </code>
    </div>
  )}

  {snapshotError && (
    <div className={styles.snapshotError}>
      {snapshotError}
    </div>
  )}
</div>
```

**State additions**:
```tsx
const [snapshotNote, setSnapshotNote] = useState("");
const [isCapturing, setIsCapturing] = useState(false);
const [snapshotResult, setSnapshotResult] = useState<{
  filePath: string;
  summary: string;
} | null>(null);
const [snapshotError, setSnapshotError] = useState<string | null>(null);
```

**Handler**:
```tsx
const handleCreateSnapshot = async () => {
  setIsCapturing(true);
  setSnapshotError(null);
  setSnapshotResult(null);

  try {
    const response = await sessionClient.createDebugSnapshot({
      note: snapshotNote || undefined,
    });
    setSnapshotResult({
      filePath: response.filePath,
      summary: response.summary,
    });
    setSnapshotNote(""); // Clear note after successful capture
  } catch (err) {
    setSnapshotError(
      err instanceof Error ? err.message : "Failed to capture snapshot"
    );
  } finally {
    setIsCapturing(false);
  }
};
```

**Success criteria**:
- Debug menu shows "Diagnostics" section with note input and button.
- Button shows loading spinner while capturing.
- On success, displays summary text and file path.
- On error, displays error message.
- Note input is optional -- snapshot works with or without a note.
- The `sessionClient` is the generated ConnectRPC TypeScript client (same pattern used in other components).

### Task 3.2: Add CSS styles for diagnostics section [0.5h]

**Scope**: Add styles for the new note input, snapshot button, result display, and error message.

**File modified**: `web-app/src/components/ui/DebugMenu.module.css`

**Additions**:
```css
.noteInputRow {
  margin-bottom: 12px;
}

.noteInput {
  width: 100%;
  padding: 10px 12px;
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  color: var(--color-text);
  font-size: 14px;
  outline: none;
  transition: border-color 0.15s ease;
}

.noteInput:focus {
  border-color: var(--color-primary);
}

.noteInput::placeholder {
  color: var(--color-text-muted);
}

.snapshotButton {
  width: 100%;
  padding: 12px 16px;
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: 8px;
  color: var(--color-text);
  font-size: 15px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.snapshotButton:hover:not(:disabled) {
  background: var(--color-bg-hover);
  border-color: var(--color-primary);
}

.snapshotButton:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.snapshotResult {
  margin-top: 12px;
  padding: 12px;
  background: var(--color-bg-secondary);
  border-radius: 8px;
  border-left: 3px solid var(--color-success);
}

.snapshotResultText {
  font-size: 13px;
  color: var(--color-text);
  margin-bottom: 8px;
}

.snapshotFilePath {
  display: block;
  font-family: "Monaco", "Courier New", monospace;
  font-size: 12px;
  color: var(--color-text-muted);
  word-break: break-all;
  padding: 6px 8px;
  background: rgba(0, 0, 0, 0.15);
  border-radius: 4px;
}

.snapshotError {
  margin-top: 12px;
  padding: 12px;
  background: var(--color-bg-secondary);
  border-radius: 8px;
  border-left: 3px solid var(--color-error, #ef4444);
  font-size: 13px;
  color: var(--color-error, #ef4444);
}
```

**Success criteria**:
- Styles match the existing debug menu visual language.
- Works in both dark and light mode.
- Spinner animation runs smoothly during capture.
- File path is selectable for copy-paste.

---

## Known Issues and Bug Risks

### Bug Risk: tmux capture-pane Timeout for Large Scrollback [SEVERITY: Medium]

**Description**: `tmux capture-pane -p` with full scrollback (`-S -`) can be slow for sessions with extensive history (thousands of lines). This could cause the per-subsystem 5-second timeout to fire, resulting in missing pane content for that session.

**Mitigation**:
- Use `tmux capture-pane -p` without `-S -` for the snapshot (visible pane only, not full scrollback).
- For sessions where full history is needed, capture separately with `-S -2000` (last 2000 lines).
- If `capture-pane` times out, record the error in `Errors` array and continue with other sessions.

**Files Likely Affected**:
- `server/services/debug_snapshot.go` (collectTmuxSnapshot function)

**Prevention Strategy**:
- Explicit `context.WithTimeout(ctx, 5*time.Second)` for each tmux command.
- Log a warning when capture is truncated due to timeout.
- Include the error in the snapshot's `errors` array so the user knows data is partial.

### Bug Risk: Concurrent Access to Session State [SEVERITY: Low]

**Description**: While the snapshot collector reads session state, other operations (status polling, terminal streaming) may concurrently modify instance fields. This could cause inconsistent data within a single session's snapshot.

**Mitigation**:
- Use `ToInstanceData()` which copies fields at a point in time (already used for serialization).
- Terminal pane capture is inherently a point-in-time snapshot from tmux.
- ApprovalStore uses `RWMutex` -- `ListAll()` takes a read lock, which is safe.

**Files Likely Affected**:
- `server/services/debug_snapshot.go`
- `session/instance.go` (ToInstanceData already handles this)

**Prevention Strategy**:
- Do not hold locks across subsystem collection boundaries.
- Copy all needed data upfront from instances before starting slow operations (tmux commands).

### Bug Risk: Large Snapshot File Size [SEVERITY: Low]

**Description**: With many sessions (10+), each with terminal content, the snapshot JSON could grow to 5-10 MB.

**Mitigation**:
- Truncate `pane_content` and `pane_content_raw` to a maximum of 10,000 characters per session.
- Limit log lines to 200 by default (configurable via request parameter).
- Include `file_size_bytes` in the response so the UI can display file size.

**Files Likely Affected**:
- `server/services/debug_snapshot.go` (truncation logic in collectSessionSnapshots)

**Prevention Strategy**:
- Document the truncation in the snapshot schema (add a `truncated` boolean field per session if content was cut).
- Log the file size after writing.

### Bug Risk: RPC Timeout for Slow Collection [SEVERITY: Medium]

**Description**: The default ConnectRPC timeout may be shorter than the snapshot collection time if many sessions exist and tmux commands are slow.

**Mitigation**:
- Use a 30-second overall context timeout for the snapshot operation.
- Individual subsystems each have 5-second timeouts, so the worst case is ~25 seconds (5 subsystems x 5 seconds).
- The HTTP server's `ReadTimeout` is 15 seconds but `WriteTimeout` is 0 (disabled for streaming). Since this is a unary RPC, the 15-second read timeout only applies to receiving the request, not the response.

**Files Likely Affected**:
- `server/services/session_service.go` (handler wraps with 30s timeout context)

**Prevention Strategy**:
- Explicitly set `ctx, cancel := context.WithTimeout(ctx, 30*time.Second)` in the handler.
- Return partial results if the overall timeout fires (the snapshot will have whatever was collected).

### Bug Risk: File Permission Issues [SEVERITY: Low]

**Description**: The log directory may not be writable if permissions changed or disk is full.

**Mitigation**:
- `log.GetLogDir()` already creates the directory with 0755 permissions.
- The snapshot writer uses `os.WriteFile` with 0644 permissions.
- If writing fails, the RPC returns a clear error message rather than a partial response.

**Files Likely Affected**:
- `server/services/debug_snapshot.go` (WriteSnapshot function)

**Prevention Strategy**:
- Check directory writability before starting collection (fail fast).
- Include disk error in the RPC error response, not silently in the snapshot.

---

## Testing Strategy

### Unit Tests (Story 1)

| Test | Description | Technique |
|------|-------------|-----------|
| `TestCollectServerInfo` | Verifies PID, Go version, OS, arch fields populated | Direct call, field assertions |
| `TestCollectRecentLogs` | Reads last N lines from temp log file | Create temp file with known lines, verify tail |
| `TestCollectRecentLogs_Empty` | Handles empty log file gracefully | Empty temp file, verify empty result |
| `TestCollectRecentLogs_Missing` | Handles missing log file gracefully | Non-existent path, verify error in Errors array |
| `TestCollectApprovalSnapshot` | Serializes pending approvals from test store | Create ApprovalStore with test data, verify output |
| `TestWriteSnapshot` | Writes valid JSON to temp directory | Create DebugSnapshot struct, write, read back, validate JSON |
| `TestWriteSnapshot_FileNaming` | Verifies timestamp format in filename | Write snapshot, check filename matches pattern |
| `TestCollectSnapshot_PartialFailure` | Snapshot succeeds even when tmux fails | Mock failing tmux, verify other sections populated |

### Integration Tests (Story 2)

| Test | Description | Technique |
|------|-------------|-----------|
| `TestCreateDebugSnapshotRPC` | End-to-end RPC call returns valid response | Start test server, call RPC, verify response fields |
| `TestCreateDebugSnapshotRPC_WithNote` | Note is included in snapshot file | Call with note, read file, verify note field |

### Manual Testing (Story 3)

| Scenario | Steps | Expected Result |
|----------|-------|-----------------|
| Basic snapshot | Open debug menu, click "Capture Debug Snapshot" | Spinner shows, then success message with file path |
| Snapshot with note | Enter note, click capture | Note appears in JSON file |
| Error handling | Stop server, click capture | Error message displayed in red |
| Multiple snapshots | Click capture twice | Two separate files with different timestamps |
| Empty note | Leave note blank, click capture | Snapshot created without note field |

---

## Dependency Graph

```
Story 1: Backend Collector
  Task 1.1: debug_snapshot.go           -- no dependencies
  Task 1.2: debug_snapshot_test.go      -- depends on Task 1.1
      |
      v
Story 2: ConnectRPC Endpoint
  Task 2.1: Proto definitions + gen     -- depends on Task 1.1 (types)
  Task 2.2: RPC handler                 -- depends on Task 1.1, Task 2.1
      |
      v
Story 3: Web UI
  Task 3.1: DebugMenu.tsx changes       -- depends on Task 2.1 (generated TS client)
  Task 3.2: DebugMenu.module.css        -- depends on Task 3.1 (class names)
```

**Critical path**: Task 1.1 -> Task 2.1 -> `make proto-gen` -> Task 2.2 -> Task 3.1

**Total estimated effort**: 6-8 hours (1 engineer, 1 day)

**Build/deploy step**: After Task 2.1, run `make proto-gen` to regenerate Go and TypeScript bindings before proceeding to Tasks 2.2 and 3.1.

---

## Implementation Checklist

- [ ] Task 1.1: Create `server/services/debug_snapshot.go` with collector logic
- [ ] Task 1.2: Create `server/services/debug_snapshot_test.go` with unit tests
- [ ] Task 2.1: Add proto definitions to `proto/session/v1/session.proto` and run `make proto-gen`
- [ ] Task 2.2: Add `CreateDebugSnapshot` handler to `server/services/session_service.go`
- [ ] Task 3.1: Add "Diagnostics" section to `web-app/src/components/ui/DebugMenu.tsx`
- [ ] Task 3.2: Add styles to `web-app/src/components/ui/DebugMenu.module.css`
- [ ] Verify: `go build ./...` passes
- [ ] Verify: `make restart-web` runs successfully
- [ ] Verify: Manual test -- open debug menu, capture snapshot, verify JSON file
