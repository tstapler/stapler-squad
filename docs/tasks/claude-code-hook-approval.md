# Feature Plan: Remote Hook Approval from Web UI

**Date**: 2026-02-26
**Updated**: 2026-03-13
**Status**: In Progress — Stories 1-4 substantially complete; Stories 5-6 pending
**Scope**: Enable users to approve/reject Claude Code tool use requests from the stapler-squad web UI

## Implementation Progress

### Completed (Stories 1-4 core)

**Story 1 — Backend Infrastructure**: COMPLETE
- `server/services/approval_store.go`: ApprovalStore, PendingApproval, ApprovalDecision types; all CRUD, Resolve, CancelSession, CleanupExpired methods implemented with mutex protection.

**Story 2 — Hook Endpoint and Session Injection**: COMPLETE
- `server/services/approval_handler.go`: HandlePermissionRequest HTTP endpoint, mapSessionByCwd fallback, broadcastApprovalNotification, InjectHookConfig (merges .claude/settings.local.json), StartExpirationCleanup goroutine; wired in server.go at POST /api/hooks/permission-request.
- InjectHookConfig is called from session_service.go during CreateSession.
- Session mapper uses X-CS-Session-ID header first, then cwd prefix matching.

**Story 3 — ConnectRPC API**: COMPLETE
- proto/session/v1/session.proto: ResolveApproval and ListPendingApprovals RPCs defined.
- proto/session/v1/types.proto: PendingApprovalProto message defined.
- approval_handler.go: ResolveApproval and ListPendingApprovals handlers implemented on SessionService.
- ApprovalStore wired to SessionService via approvalStore field and GetApprovalStore() accessor.
- server.go wires handler, starts cleanup goroutine.

**Story 4 — Web UI Components**: COMPLETE
- `web-app/src/components/sessions/ApprovalCard.tsx`: countdown timer, tool input preview, Approve/Deny buttons with aria labels; used in SessionDetail and ApprovalPanel.
- `web-app/src/components/sessions/ApprovalPanel.tsx`: list of ApprovalCards, count badge, refresh button, empty/error states; rendered inside SessionDetail for per-session view.
- `web-app/src/components/sessions/ApprovalNavBadge.tsx`: global badge in header showing total pending count.
- `web-app/src/lib/hooks/useApprovals.ts`: polls ListPendingApprovals every 5s, optimistic approve/deny, error rollback.
- `web-app/src/lib/hooks/useSessionNotifications.ts`: approval_id extracted from notification metadata; onApprove/onDeny callbacks call resolveApproval; wired into NotificationToast.
- `web-app/src/components/ui/NotificationToast.tsx`: shows Approve/Deny buttons for approval_needed notifications.
- `web-app/src/components/layout/Header.tsx`: ApprovalNavBadge rendered in nav.

### Known Gap — Proto Code Not Regenerated

The proto definitions include ResolveApproval, ListPendingApprovals, and PendingApprovalProto, but `make proto-gen` has NOT been run. The generated Go and TypeScript bindings in gen/ may be stale. This means:
- The Go build compiles (approval_handler.go directly accesses the store, not generated code).
- The TypeScript client (useApprovals.ts) uses generated types — this needs verification.

### Remaining Work

**Story 5 — Review Queue Integration**: NOT STARTED
- Enrich ReviewItem with pending_approval field when reason is APPROVAL_PENDING.
- Add Approve/Deny buttons to ReviewQueuePanel items.
- Optional: distinct approval notification sound.

**Story 6 — Mobile UX**: NOT STARTED
- Responsive ApprovalCard layout for mobile viewports.
- Mobile banner (vs toast) for small screens.
- Optional: vibration feedback.

### Session Hook Injection — Worktree Gap

InjectHookConfig is called in CreateSession, but its integration with worktree sessions needs validation. The function writes to `<rootDir>/.claude/settings.local.json` using `GetEffectiveRootDir()`. Worktree sessions should use the worktree directory. Needs a smoke test.

---

---

## Table of Contents

- [Problem Statement](#problem-statement)
- [Research Findings: Claude Code Hooks API](#research-findings-claude-code-hooks-api)
- [Research Findings: Existing Codebase](#research-findings-existing-codebase)
- [ADR-011: Hook Approval Architecture](#adr-011-hook-approval-architecture)
- [ADR-012: Hook Script Blocking Strategy](#adr-012-hook-script-blocking-strategy)
- [Architecture Overview](#architecture-overview)
- [Story 1: Approval Request Backend Infrastructure](#story-1-approval-request-backend-infrastructure)
- [Story 2: Hook Script for PermissionRequest Blocking](#story-2-hook-script-for-permissionrequest-blocking)
- [Story 3: ConnectRPC API for Approval Workflow](#story-3-connectrpc-api-for-approval-workflow)
- [Story 4: Web UI Approval Components](#story-4-web-ui-approval-components)
- [Story 5: Review Queue Integration](#story-5-review-queue-integration)
- [Story 6: Mobile UX and Progressive Enhancement](#story-6-mobile-ux-and-progressive-enhancement)
- [Known Issues and Bug Risks](#known-issues-and-bug-risks)
- [Testing Strategy](#testing-strategy)
- [Dependency Graph](#dependency-graph)

---

## Problem Statement

Stapler Squad currently **detects** when Claude Code needs approval (via terminal output
pattern matching in `session/detection/`) and **notifies** the user through the review
queue and toast notifications. However, the user cannot **respond** to these approval
requests from the web UI. The only options are "Focus Window", "View Session", and
"Dismiss" -- all of which require the user to physically interact with the terminal
where Claude Code is running.

The goal is to close this loop: when Claude Code pauses for permission approval, the
user should be able to click "Approve" or "Reject" directly from the web UI (including
on a phone or tablet), and Claude Code should resume execution accordingly.

---

## Research Findings: Claude Code Hooks API

Source: https://code.claude.com/docs/en/hooks

### Relevant Hook Events

**`PermissionRequest`** -- Fires when a permission dialog is shown to the user. This is
the primary hook for intercepting approval requests. It supports decision control:

- **Input**: Receives `tool_name`, `tool_input`, and `permission_suggestions` as JSON
  on stdin. Common tools: `Bash`, `Edit`, `Write`, `Read`, `Glob`, `Grep`, `WebFetch`.
- **Output**: Returns JSON with `hookSpecificOutput.decision.behavior` set to
  `"allow"` or `"deny"`.
- **Matcher**: Matches on tool name (regex), e.g., `"Bash"`, `"Edit|Write"`, `".*"`.

**`PreToolUse`** -- Fires before any tool call executes. Can also block tool execution.

- **Output**: Returns JSON with `hookSpecificOutput.permissionDecision` set to
  `"allow"`, `"deny"`, or `"ask"` (escalate to user).
- Less suitable than `PermissionRequest` because it fires for ALL tool calls, not just
  ones requiring permission.

### Hook Types Available

Four hook types exist: `command`, `http`, `prompt`, and `agent`.

**HTTP Hooks** (`type: "http"`) are the most relevant for this feature:

- Send the event's JSON input as an HTTP POST request body to a URL.
- Return decisions via JSON response body (same schema as command hooks).
- 2xx with JSON body containing decision fields controls behavior.
- Non-2xx or timeout = non-blocking error (execution continues).
- Timeout is configurable (default 30s for HTTP hooks).
- Headers support environment variable interpolation.

**Command Hooks** (`type: "command"`) are the fallback option:

- Execute a shell command with JSON on stdin.
- Exit code 0 = success (parse stdout for JSON decision).
- Exit code 2 = blocking error (deny the permission).
- Can block indefinitely while waiting for external input (up to timeout).
- Default timeout: 600 seconds (10 minutes).

### Critical Design Constraint

Hooks are **synchronous blocking** by default. When a `PermissionRequest` hook runs,
Claude Code waits for it to complete before proceeding. This is exactly the behavior
we need -- the hook can block while waiting for the user's decision from the web UI.

The `async: true` option exists but is NOT suitable here because async hooks cannot
return decisions (the action proceeds immediately).

### Hook Configuration Locations

Hooks can be defined in:
- `~/.claude/settings.json` (all projects)
- `.claude/settings.json` (per-project, committable)
- `.claude/settings.local.json` (per-project, gitignored)

For stapler-squad managed sessions, the hook configuration should be injected into the
session's settings when the session is created.

### JSON Protocol Details

**PermissionRequest Input** (received on stdin or as POST body):
```json
{
  "session_id": "abc123",
  "transcript_path": "/path/to/transcript.jsonl",
  "cwd": "/path/to/project",
  "permission_mode": "default",
  "hook_event_name": "PermissionRequest",
  "tool_name": "Bash",
  "tool_input": {
    "command": "rm -rf node_modules",
    "description": "Remove node_modules directory"
  },
  "permission_suggestions": [
    { "type": "toolAlwaysAllow", "tool": "Bash" }
  ]
}
```

**PermissionRequest Output** (to allow):
```json
{
  "hookSpecificOutput": {
    "hookEventName": "PermissionRequest",
    "decision": {
      "behavior": "allow"
    }
  }
}
```

**PermissionRequest Output** (to deny):
```json
{
  "hookSpecificOutput": {
    "hookEventName": "PermissionRequest",
    "decision": {
      "behavior": "deny",
      "message": "User rejected from web UI"
    }
  }
}
```

---

## Research Findings: Existing Codebase

### Current Notification Flow (Read-Only)

The existing system detects approval needs but cannot respond to them:

1. **Detection**: `session/detection/StatusDetector` pattern-matches terminal output
   for approval prompts (`StatusNeedsApproval`).
2. **Review Queue**: `session/review_queue_poller.go` polls sessions every 2 seconds,
   detects `ReasonApprovalPending`, and adds items to the review queue.
3. **Notification**: `web-app/src/components/ui/NotificationToast.tsx` shows toast with
   "Focus Window", "View Session", "Dismiss" buttons.
4. **Hook Handler**: `scripts/ssq-hook-handler` processes `PermissionRequest` hooks
   but currently only sends a notification via `ssq-notify` -- it does NOT block or
   return a decision.

### Existing Hook Infrastructure

`session/mux/hooks.go` already generates hook configuration files for claude-mux sessions:

- Generates JSON hooks config with `PermissionRequest` handler.
- Sets up environment variables (`CS_MUX_SOCKET_PATH`, `CS_MUX_TMUX_SESSION`, etc.).
- Calls `scripts/ssq-hook-handler permission` which just sends a notification.

### Terminal Input Path

`session/instance.go` provides `WriteToPTY()` and `SendKeys()` which write directly to
the tmux PTY. This is how the web UI currently sends terminal input via
`StreamTerminal` (bidirectional streaming in `server/services/session_service.go`).

However, for hook-based approval, we do NOT want to simulate keystrokes. We want the
hook script itself to return the decision to Claude Code via its stdout/response.

### ConnectRPC Service Layer

`server/services/session_service.go` implements all RPC endpoints. The existing
`SendNotification` RPC shows the pattern for cross-process communication. The review
queue has `WatchReviewQueue` for server-streaming real-time updates.

### Protobuf Definitions

All proto definitions are in `proto/session/v1/`. The existing `NotificationType` enum
includes `APPROVAL_NEEDED`. The `ReviewItem` includes `AttentionReason.APPROVAL_PENDING`.

---

## ADR-011: Hook Approval Architecture

**Status**: Proposed

**Context**: We need a mechanism where:
1. Claude Code fires a `PermissionRequest` hook and blocks waiting for a response.
2. The hook communicates the pending request to stapler-squad's server.
3. The web UI displays the request and collects the user's decision.
4. The decision flows back to the hook, which returns it to Claude Code.

**Options Considered**:

### Option A: HTTP Hook Pointing at Stapler Squad Server

Configure Claude Code with an HTTP hook (`type: "http"`) that POSTs directly to the
stapler-squad server. The server holds the HTTP connection open (long-poll) until the
user responds via the web UI, then returns the decision as the HTTP response.

Pros:
- Native Claude Code feature, no custom scripts needed.
- Clean request/response semantics -- the HTTP response IS the decision.
- No filesystem coordination or polling.
- Works with both managed and external sessions (claude-mux).

Cons:
- HTTP hook timeout defaults to 30 seconds. Even with custom timeout, there is an
  upper bound. If the user takes too long, Claude Code treats it as a non-blocking
  error and continues (shows the permission dialog normally).
- The hook runs during session initialization, so the server must be running.
- HTTP hooks require editing settings JSON directly (no /hooks menu support).

### Option B: Command Hook with Filesystem Semaphore

Configure a command hook that writes the request to a well-known file, then polls
a response file until the user's decision appears. The web UI writes the decision
to the response file via an API call.

Pros:
- Command hooks have 600-second (10 minute) default timeout.
- Simple to understand and debug.
- No long-lived HTTP connections.

Cons:
- Filesystem polling introduces latency (100ms-1s per poll cycle).
- Cleanup complexity (stale files from crashed processes).
- Race conditions around file reads/writes.
- Not suitable for remote/tunneled setups.

### Option C: HTTP Hook with Extended Timeout (Selected)

Use an HTTP hook pointing at a dedicated endpoint on the stapler-squad server. Set the
timeout to 300 seconds (5 minutes). The server holds the connection open until the
user responds. If timeout occurs, the hook returns a non-blocking error, and Claude
Code falls back to showing the permission dialog in the terminal.

**Decision**: Option C.

**Rationale**:
- HTTP hooks are a first-class Claude Code feature with clean semantics.
- 5-minute timeout is generous for remote approval scenarios.
- Graceful degradation: if timeout expires, the user can still approve in the terminal.
- No filesystem coordination, polling, or cleanup needed.
- Works with any session type (managed, claude-mux, external).
- The server endpoint is simple: hold connection, wait for decision, respond.

**Fallback Strategy**: If the HTTP hook times out (5 minutes), Claude Code shows the
normal permission dialog in the terminal. The user can still approve there. This means
the feature degrades gracefully -- it never blocks Claude Code permanently.

---

## ADR-012: Hook Script Blocking Strategy

**Status**: Proposed

**Context**: Within Option C (HTTP hook), we need to decide the exact endpoint design
and how approval requests are tracked.

**Decision**: Implement a dedicated `/api/hooks/permission-request` endpoint on the
stapler-squad server that:

1. Receives the `PermissionRequest` JSON from Claude Code's HTTP hook.
2. Creates a `PendingApproval` record in an in-memory store (with mutex protection).
3. Broadcasts the pending approval to all connected web UI clients via the event bus.
4. Holds the HTTP connection open using a Go channel that blocks until:
   a. The user responds (approve/deny) via the web UI, which calls a resolve endpoint.
   b. The context is canceled (client disconnect or server shutdown).
   c. An internal timeout is reached (4 minutes, less than the hook's 5-minute timeout).
5. Returns the user's decision as the HTTP response body in the hooks JSON format.

**Key Data Structure**:
```go
type PendingApproval struct {
    ID              string                 // UUID
    SessionID       string                 // Stapler Squad session identifier
    ClaudeSessionID string                 // Claude Code's internal session_id
    ToolName        string                 // e.g., "Bash"
    ToolInput       map[string]any         // e.g., {"command": "npm test"}
    CreatedAt       time.Time
    ExpiresAt       time.Time
    DecisionCh      chan ApprovalDecision   // Blocks until resolved
}

type ApprovalDecision struct {
    Behavior string // "allow" or "deny"
    Message  string // Optional reason (shown to Claude on deny)
}
```

**Session Mapping**: The hook input includes `cwd` (working directory) and optionally
custom HTTP headers like `X-CS-Session-ID`. The endpoint uses these to map the hook
request to a stapler-squad session. For managed sessions, the `X-CS-Session-ID` header
(injected at session creation) provides a direct match. For claude-mux sessions, cwd
matching against session paths provides a fallback.

---

## Architecture Overview

```
Claude Code Session (tmux)
    |
    | (PermissionRequest fires)
    |
    v
HTTP Hook (type: "http")
    |
    | POST /api/hooks/permission-request
    |   Body: { tool_name, tool_input, cwd, session_id, ... }
    |   Header: X-CS-Session-ID: <session-title>
    |
    v
Stapler Squad Server (Go)
    |
    +-- Creates PendingApproval record
    |     (in-memory map, mutex-protected)
    |
    +-- Publishes ApprovalRequestEvent via EventBus
    |     (to all connected WebSocket clients)
    |
    +-- Blocks on PendingApproval.DecisionCh
    |     (HTTP connection stays open, up to 4 min)
    |
    v                                   Web UI (React)
    |                                       |
    |                                       | (receives ApprovalRequestEvent
    |                                       |  via WatchReviewQueue stream)
    |                                       |
    |                                       v
    |                                   Approval Dialog
    |                                   [Tool: Bash]
    |                                   [Command: npm test]
    |                                   [Approve] [Reject]
    |                                       |
    |                                       | RPC: ResolveApproval
    |                                       |   { approval_id, decision }
    |                                       |
    |   <------  DecisionCh receives  ------+
    |
    v
HTTP Response (to Claude Code hook)
    {
      "hookSpecificOutput": {
        "hookEventName": "PermissionRequest",
        "decision": { "behavior": "allow" }
      }
    }
    |
    v
Claude Code proceeds (or blocks the tool call)
```

---

## Story 1: Approval Request Backend Infrastructure

**Goal**: Create the in-memory pending approval store and core data types.

### Tasks

#### Task 1.1: Define approval data types

Create `server/services/approval_store.go` with the core types for tracking pending
approval requests.

**Data Types**:
```go
// PendingApproval represents a hook approval request waiting for a user decision.
type PendingApproval struct {
    ID              string                 `json:"id"`
    SessionID       string                 `json:"session_id"`
    ClaudeSessionID string                 `json:"claude_session_id"`
    ToolName        string                 `json:"tool_name"`
    ToolInput       map[string]interface{} `json:"tool_input"`
    Cwd             string                 `json:"cwd"`
    PermissionMode  string                 `json:"permission_mode"`
    CreatedAt       time.Time              `json:"created_at"`
    ExpiresAt       time.Time              `json:"expires_at"`

    // Internal: channel for blocking until resolved
    decisionCh chan ApprovalDecision
}

// ApprovalDecision is the user's response to a pending approval.
type ApprovalDecision struct {
    Behavior string `json:"behavior"` // "allow" or "deny"
    Message  string `json:"message"`  // Optional reason
}

// ApprovalStore manages pending approval requests with thread-safe access.
type ApprovalStore struct {
    mu        sync.RWMutex
    pending   map[string]*PendingApproval // keyed by approval ID
    bySession map[string][]string         // session ID -> approval IDs
}
```

**Methods on ApprovalStore**:
- `NewApprovalStore() *ApprovalStore`
- `Create(approval *PendingApproval) error` -- adds to store, initializes channel
- `Get(id string) (*PendingApproval, bool)` -- retrieves by ID
- `GetBySession(sessionID string) []*PendingApproval` -- all pending for a session
- `ListAll() []*PendingApproval` -- all pending approvals
- `Resolve(id string, decision ApprovalDecision) error` -- sends on channel, removes
- `Remove(id string)` -- cleanup without resolving (e.g., on timeout)
- `CleanupExpired()` -- removes approvals past their ExpiresAt

**Acceptance Criteria**:
- Store is thread-safe (all methods protected by mutex).
- `Create` allocates a buffered channel of size 1 for the decision.
- `Resolve` sends the decision on the channel and removes the entry.
- `Resolve` on a non-existent ID returns an error (not a panic).
- Double-resolve returns an error on the second call.
- `CleanupExpired` removes entries and closes channels (deny with timeout message).

#### Task 1.2: Add session mapping logic

The hook request includes `cwd`, `session_id` (Claude Code internal), and optionally
injected HTTP headers. We need to map these to a stapler-squad session.

Create `server/services/approval_session_mapper.go`:

**Mapping Strategy** (in priority order):
1. **HTTP header `X-CS-Session-ID`**: If the hook configuration includes this header
   (injected by stapler-squad when creating sessions), use it directly.
2. **`cwd` matching**: Compare `cwd` from hook input against each session's `Path`
   and `WorkingDir`. Use longest prefix match for worktree sessions.
3. **Tmux session name**: If `X-CS-MUX-Session` appears in hook headers,
   match against session titles.
4. **Fallback**: If no match found, create a "floating" approval that appears in the
   UI but is not linked to a specific session.

**Acceptance Criteria**:
- Mapper correctly resolves managed sessions by `X-CS-Session-ID` header.
- Mapper handles worktree paths (which differ from the original `Path`).
- Mapper handles external/mux sessions via tmux session name.
- Unknown sessions produce a valid approval request (just without session linkage).

#### Task 1.3: Add expiration cleanup goroutine

Create a background goroutine that periodically cleans up expired approvals. This
prevents memory leaks from requests where the user never responds and the HTTP
connection timed out.

**Design**:
- Run every 30 seconds.
- Call `CleanupExpired()` on the store.
- Log warnings for expired approvals (indicates users are not responding in time).
- Deny expired approvals with a timeout message so the channel is properly closed.

**Acceptance Criteria**:
- Goroutine starts when the server starts and stops on shutdown.
- Expired approvals are cleaned up within 30 seconds of expiration.
- No goroutine leaks on server shutdown.

---

## Story 2: Hook Script for PermissionRequest Blocking

**Goal**: Configure Claude Code sessions to send `PermissionRequest` hooks to the
stapler-squad server as HTTP hooks, so the server can intercept and respond.

### Tasks

#### Task 2.1: Add HTTP hook endpoint for permission requests

Create a new HTTP handler in `server/services/approval_handler.go` that receives
`PermissionRequest` events from Claude Code's HTTP hook mechanism.

**Endpoint**: `POST /api/hooks/permission-request`

**Request Body**: The standard Claude Code `PermissionRequest` JSON (received as-is
from the HTTP hook).

**Response**: Blocks until user responds, then returns:
```json
{
  "hookSpecificOutput": {
    "hookEventName": "PermissionRequest",
    "decision": {
      "behavior": "allow"
    }
  }
}
```

**Flow**:
1. Parse the hook JSON from request body.
2. Extract `X-CS-Session-ID` header for session mapping.
3. Map to a stapler-squad session using the session mapper (Task 1.2).
4. Create a `PendingApproval` record in the store (4-minute expiry).
5. Publish an `ApprovalRequestEvent` via the event bus.
6. Block on the `PendingApproval.decisionCh` channel with a select/timer.
7. When resolved, format the response in the hooks JSON format and return 200.
8. On context cancellation or timeout, return 200 with empty body (Claude Code
   treats this as "allow" since the hook did not deny).

**Important**: The endpoint must return 2xx even when denying. Non-2xx = non-blocking
error in Claude Code's HTTP hook handling, meaning the tool call would proceed anyway.

**Acceptance Criteria**:
- Endpoint correctly parses all standard `PermissionRequest` fields.
- Blocks until decision is received or context canceled.
- Returns properly formatted hookSpecificOutput JSON.
- Always returns 2xx (deny decisions use JSON body, not HTTP status).
- Handles concurrent requests from multiple sessions.
- 4-minute server-side timeout is less than the 5-minute hook timeout.

#### Task 2.2: Add resolve endpoint for user decisions

Add a new ConnectRPC method that the web UI calls when the user clicks Approve or Reject.

```protobuf
rpc ResolveApproval(ResolveApprovalRequest) returns (ResolveApprovalResponse) {}
```

**Flow**:
1. Validate the approval ID exists in the store.
2. Call `store.Resolve(id, decision)`.
3. The blocked HTTP handler (Task 2.1) unblocks and returns the decision.
4. Publish an `ApprovalResolvedEvent` via the event bus.
5. Return success.

**Acceptance Criteria**:
- Resolving a valid approval returns success and unblocks the hook handler.
- Resolving a non-existent approval returns NotFound error.
- Resolving an already-resolved approval returns AlreadyExists error.
- Concurrent resolve attempts for the same ID are handled safely.

#### Task 2.3: Add list-pending-approvals endpoint

Create an endpoint to list all currently pending approvals for the web UI.

```protobuf
rpc ListPendingApprovals(ListPendingApprovalsRequest) returns (ListPendingApprovalsResponse) {}
```

**Acceptance Criteria**:
- Returns all pending approvals, optionally filtered by session.
- Includes tool name and input for display in the UI.
- Does not include internal channels or implementation details.

#### Task 2.4: Inject HTTP hook configuration into managed sessions

When stapler-squad creates a new session (in `CreateSession`), inject the HTTP hook
configuration into the session's working directory.

**Approach**: Write a `.claude/settings.local.json` file in the session's working
directory (or worktree) with the hook configuration:

```json
{
  "hooks": {
    "PermissionRequest": [
      {
        "hooks": [
          {
            "type": "http",
            "url": "http://localhost:8543/api/hooks/permission-request",
            "timeout": 300,
            "headers": {
              "X-CS-Session-ID": "<session-title>"
            }
          }
        ]
      }
    ]
  }
}
```

**Important considerations**:
- The URL uses `localhost:8543` (stapler-squad's default port).
- The `X-CS-Session-ID` header identifies which stapler-squad session this is.
- The timeout is 300 seconds (5 minutes) to give users time to respond.
- Write to `.claude/settings.local.json` so it does not get committed to git.
- For worktree sessions, write to the worktree directory.
- Must not overwrite existing settings -- merge with any existing hooks config.

**Acceptance Criteria**:
- New sessions automatically get the HTTP hook configured.
- Existing hooks in `.claude/settings.local.json` are preserved (merged).
- Worktree sessions get the config in their worktree directory.
- The session title is correctly passed in the header.
- The hook file is cleaned up when the session is destroyed.

#### Task 2.5: Update claude-mux hook configuration for external sessions

Update `session/mux/hooks.go` to add the HTTP hook for `PermissionRequest` alongside
the existing command hook. The HTTP hook provides the blocking behavior needed for
remote approval, while the command hook continues to send notifications.

**Changes to `GenerateHooksFile`**:
- Add an HTTP hook entry for `PermissionRequest` that points to the stapler-squad server.
- Keep the existing command hook for notifications (it fires in parallel).
- Inject `X-CS-MUX-Session` as a header for session mapping.

**Acceptance Criteria**:
- Claude-mux sessions get both the notification hook and the approval HTTP hook.
- The HTTP hook has a 300-second timeout.
- Session mapping works via the session name header.

---

## Story 3: ConnectRPC API for Approval Workflow

**Goal**: Define the protobuf messages and wire up ConnectRPC endpoints for the
approval workflow.

### Tasks

#### Task 3.1: Add protobuf definitions for approval workflow

Add to `proto/session/v1/session.proto`:

```protobuf
// ResolveApproval responds to a pending tool approval request.
rpc ResolveApproval(ResolveApprovalRequest) returns (ResolveApprovalResponse) {}

// ListPendingApprovals returns all pending tool approval requests.
rpc ListPendingApprovals(ListPendingApprovalsRequest) returns (ListPendingApprovalsResponse) {}
```

Add message types to `proto/session/v1/session.proto`:

```protobuf
message ResolveApprovalRequest {
  string approval_id = 1;
  string decision = 2;         // "allow" or "deny"
  optional string message = 3; // Reason shown to Claude on deny
}

message ResolveApprovalResponse {
  bool success = 1;
  string message = 2;
}

message ListPendingApprovalsRequest {
  optional string session_id = 1; // Filter by session
}

message ListPendingApprovalsResponse {
  repeated PendingApprovalProto approvals = 1;
}
```

Add to `proto/session/v1/types.proto`:

```protobuf
// PendingApprovalProto represents a Claude Code tool use request awaiting user decision.
message PendingApprovalProto {
  string id = 1;
  string session_id = 2;
  string tool_name = 3;
  map<string, string> tool_input = 4;
  string cwd = 5;
  string permission_mode = 6;
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp expires_at = 8;
  int32 seconds_remaining = 9;
}
```

Add to `proto/session/v1/events.proto`:

```protobuf
// ApprovalRequestEvent is emitted when a new tool approval request is received.
message ApprovalRequestEvent {
  PendingApprovalProto approval = 1;
}

// ApprovalResolvedEvent is emitted when a pending approval is resolved.
message ApprovalResolvedEvent {
  string approval_id = 1;
  string session_id = 2;
  string decision = 3;    // "allow" or "deny"
  string resolved_by = 4; // "user", "timeout", "disconnect"
}
```

**Acceptance Criteria**:
- Proto definitions compile without errors.
- Generated Go and TypeScript code is correct.
- Events integrate into the existing `SessionEvent` oneof.

#### Task 3.2: Implement ConnectRPC handlers

Wire the new RPC methods into `SessionService`:

- `ResolveApproval`: Delegates to `ApprovalStore.Resolve()`.
- `ListPendingApprovals`: Delegates to `ApprovalStore.ListAll()` or `GetBySession()`.

**Acceptance Criteria**:
- Both endpoints return correct proto responses.
- Error codes match ConnectRPC conventions (NotFound, InvalidArgument, etc.).
- Approval events are published to the event bus.

#### Task 3.3: Integrate approval events into event streaming

Add `ApprovalRequestEvent` and `ApprovalResolvedEvent` to the existing event stream
so web UI clients receive real-time updates about pending approvals.

**Design choice**: Rather than creating a separate streaming endpoint, piggyback on
the existing `WatchReviewQueue` server-streaming RPC and event bus. This leverages
the existing real-time infrastructure.

**Changes**:
- Add `approval_request` and `approval_resolved` to the `SessionEvent` oneof.
- Add approval events to the event bus publishing.
- Web UI event handlers receive these events and update state.

**Acceptance Criteria**:
- New approval requests appear in real-time on connected clients.
- Resolved approvals are removed in real-time.
- Multiple connected clients all see the same approval state.

---

## Story 4: Web UI Approval Components

**Goal**: Build the React components for displaying and responding to approval requests.

### Tasks

#### Task 4.1: Create ApprovalCard component

Create `web-app/src/components/sessions/ApprovalCard.tsx` that displays a single
pending approval request with Approve/Reject buttons.

**Component Design**:
```tsx
interface ApprovalCardProps {
  approval: PendingApprovalProto;
  onApprove: (id: string) => void;
  onDeny: (id: string, message?: string) => void;
  compact?: boolean; // For mobile/toast rendering
}
```

**Visual Elements**:
- Tool name badge (e.g., "Bash", "Edit", "Write") with color coding.
- Tool input preview (truncated command for Bash, file path for Edit/Write).
- Session name and working directory context.
- Countdown timer showing seconds remaining before expiry.
- "Approve" button (green, prominent).
- "Deny" button (red, secondary).

**Acceptance Criteria**:
- Clearly displays what tool Claude wants to use and with what arguments.
- Approve/Deny buttons are large enough for touch interaction (44x44px minimum).
- Countdown timer updates every second.
- Component disables buttons and shows loading state after user clicks.
- Works in both panel and toast contexts.

#### Task 4.2: Create ApprovalPanel component

Create `web-app/src/components/sessions/ApprovalPanel.tsx` that lists all pending
approvals, similar to the existing ReviewQueuePanel.

**Design**:
- Shows a count badge of pending approvals.
- Lists approvals sorted by creation time (oldest first).
- Each item is an ApprovalCard.
- Empty state: "No pending approvals".
- Real-time updates via the event stream.

**Acceptance Criteria**:
- Panel updates in real-time as approvals arrive and are resolved.
- Smooth animations for adding/removing items.
- Accessible keyboard navigation.

#### Task 4.3: Add approval actions to NotificationToast

Modify `web-app/src/components/ui/NotificationToast.tsx` to show Approve/Deny buttons
when the notification type is `approval_needed` and a corresponding `PendingApproval`
exists.

**Changes to NotificationData**:
```tsx
interface NotificationData {
  // ... existing fields ...
  /** Pending approval ID if this notification is for a tool approval */
  pendingApprovalId?: string;
  /** Callback when user approves */
  onApprove?: (approvalId: string) => void;
  /** Callback when user denies */
  onDeny?: (approvalId: string) => void;
}
```

**Visual Changes**:
- When `pendingApprovalId` is set, replace "Focus Window" / "Dismiss" buttons with
  "Approve" / "Deny" buttons.
- Keep "View Session" button for context.
- Show tool name and command in the notification body.
- Disable auto-close for approval toasts (user must explicitly respond).

**Acceptance Criteria**:
- Approval toasts show Approve/Deny buttons.
- Approval toasts do not auto-close.
- Clicking Approve/Deny calls the resolve API and closes the toast.
- Loading state shown while waiting for API response.
- If the approval expires while the toast is shown, the buttons disable with an
  "Expired" message.

#### Task 4.4: Create useApprovals hook

Create `web-app/src/lib/hooks/useApprovals.ts` that manages the client-side state
for pending approvals.

**Interface**:
```tsx
interface UseApprovalsReturn {
  pendingApprovals: PendingApprovalProto[];
  loading: boolean;
  error: Error | null;
  approveRequest: (id: string) => Promise<void>;
  denyRequest: (id: string, message?: string) => Promise<void>;
  getApprovalForSession: (sessionId: string) => PendingApprovalProto | undefined;
}
```

**Behavior**:
- On mount, fetch current pending approvals via `ListPendingApprovals`.
- Subscribe to approval events via the event stream.
- Optimistically update the list when user approves/denies.
- Handle race conditions (approval expires while user is clicking).

**Acceptance Criteria**:
- Hook provides real-time pending approval state.
- Approve/deny actions call the ConnectRPC endpoint.
- Optimistic updates provide instant UI feedback.
- Error handling for network failures and expired approvals.

---

## Story 5: Review Queue Integration

**Goal**: Integrate approval requests into the existing review queue workflow so users
have a unified attention management experience.

### Tasks

#### Task 5.1: Enhance review queue items with approval context

When a `ReviewItem` has reason `APPROVAL_PENDING` and a corresponding `PendingApproval`
exists, enrich the review queue item with the approval details (tool name, input,
approval ID, expiry countdown).

**Changes to ReviewItem proto** (in `types.proto`):
```protobuf
message ReviewItem {
  // ... existing fields ...
  // Populated when reason is APPROVAL_PENDING and a hook approval is pending.
  optional PendingApprovalProto pending_approval = 20;
}
```

**Backend changes**:
- When building `ReviewItem` for sessions with `ReasonApprovalPending`, check the
  `ApprovalStore` for a matching pending approval.
- If found, attach the approval details to the `ReviewItem`.

**Acceptance Criteria**:
- Review queue items show Approve/Deny buttons when approval data is available.
- Items without hook-based approval (detection-only) show the existing behavior.
- The transition from detection-based to hook-based approval is seamless.

#### Task 5.2: Add Approve/Deny buttons to review queue items

Modify `web-app/src/components/sessions/ReviewQueuePanel.tsx` to show Approve/Deny
action buttons on review queue items that have pending approvals.

**Visual Design**:
- Add an "actions" section below the existing item content.
- Show Approve (green) and Deny (red) buttons.
- Show the tool name and a summary of the input.
- Show countdown timer for expiry.

**Acceptance Criteria**:
- Approve/Deny buttons appear on items with pending approvals.
- Buttons work correctly and resolve the approval.
- Item is removed from the queue after resolution.
- Countdown timer counts down and shows "Expired" when time runs out.

#### Task 5.3: Add approval-specific notification sound

Add a distinct notification sound for approval requests to differentiate them from
informational review queue updates. The existing `NotificationSound.DING` is used
for all review queue items.

**Design**:
- Add `NotificationSound.APPROVAL` to the sound enum.
- Play a more urgent/distinct sound for approval notifications.
- Respect existing notification sound preferences.

**Acceptance Criteria**:
- Approval notifications play a distinct sound.
- Sound can be muted via existing notification settings.
- Sound does not play if the approval is for a session the user is already viewing.

---

## Story 6: Mobile UX and Progressive Enhancement

**Goal**: Ensure the approval workflow works well on mobile devices and small screens.

### Tasks

#### Task 6.1: Responsive approval card layout

Ensure the ApprovalCard component works on mobile viewports (< 768px).

**Design**:
- Full-width card layout on mobile.
- Approve/Deny buttons take up full width and are stacked vertically.
- Tool input preview is scrollable if it overflows.
- Minimum touch target size of 44x44px for all interactive elements.

**Acceptance Criteria**:
- ApprovalCard renders correctly on 320px-width viewport.
- Buttons are easy to tap on touch screens.
- No horizontal scrolling on the page.

#### Task 6.2: Mobile-optimized notification banner

On mobile, show approval requests as a persistent top banner (rather than a
dismissible toast in the corner) since toasts are harder to interact with on small
screens.

**Design**:
- When viewport < 768px, approval notifications show as a banner at the top.
- Banner is sticky and does not auto-dismiss.
- Shows tool name, session name, and Approve/Deny buttons inline.
- Tapping the banner expands to show full tool input details.

**Acceptance Criteria**:
- Banner renders correctly on mobile viewports.
- Approve/Deny buttons are accessible and functional.
- Banner dismisses after user responds.

#### Task 6.3: Add vibration feedback for approval requests

Use the Web Vibration API (where available) to add haptic feedback when an approval
request arrives on a mobile device.

**Acceptance Criteria**:
- Devices with vibration support get a short buzz on approval notification.
- Feature-detect vibration API (no errors on unsupported devices).
- Vibration respects notification mute settings.

---

## Known Issues and Bug Risks

### Bug Risk: Race Condition Between Hook Timeout and User Response [SEVERITY: High]

**Description**: The HTTP hook has a 5-minute timeout. If the user clicks Approve at
4:59 and the hook times out at 5:00, the approval may be processed by the server but
the hook has already returned (non-blocking error), so Claude Code shows the permission
dialog in the terminal. The user sees "approved" in the web UI but Claude Code is
still waiting for terminal input.

**Mitigation**:
- Set the server-side timeout to 4 minutes (60 seconds less than the hook timeout).
- When the server timeout expires, deny with a message: "Approval timed out. Please
  respond in the terminal."
- The web UI shows a "Expired" state on the approval card.
- Add a 30-second warning countdown in the UI before expiry.

**Prevention Strategy**:
- Use a single server-side timeout that is strictly less than the hook timeout.
- Close the decision channel on server timeout (do not leave it open).
- Test with simulated slow responses to verify the timing.

**Files Likely Affected**:
- `server/services/approval_handler.go`
- `server/services/approval_store.go`

### Bug Risk: Stale Approvals After Session Restart [SEVERITY: Medium]

**Description**: If a session is restarted while an approval is pending, the old
approval remains in the store but the hook connection is gone. The user might approve
a stale request that no longer corresponds to an active Claude Code permission dialog.

**Mitigation**:
- When a session restarts (via `RestartSession`), cancel all pending approvals for
  that session.
- Clean up approvals when the HTTP connection closes (context cancellation).
- The store should remove approvals whose HTTP handler has disconnected.

**Prevention Strategy**:
- Use context cancellation propagation from the HTTP handler.
- When the handler's context is canceled, automatically remove the approval from the
  store and publish an `ApprovalResolvedEvent` with `resolved_by: "disconnect"`.

**Files Likely Affected**:
- `server/services/approval_store.go`
- `server/services/session_service.go` (RestartSession)

### Bug Risk: Multiple Approval Requests for Same Session [SEVERITY: Medium]

**Description**: Claude Code may fire multiple `PermissionRequest` hooks in rapid
succession (e.g., if it queues several tool calls). Each creates a separate pending
approval. The web UI must handle multiple approvals per session correctly.

**Mitigation**:
- The `bySession` index in `ApprovalStore` tracks multiple approvals per session.
- The UI shows all pending approvals, not just the first one.
- Each approval has its own ID and must be resolved independently.

**Prevention Strategy**:
- Design the UI to handle a list of approvals, not a single approval.
- Test with scenarios where Claude Code requests Bash, then Edit, then Write in
  rapid succession.

**Files Likely Affected**:
- `web-app/src/components/sessions/ApprovalCard.tsx`
- `web-app/src/components/sessions/ApprovalPanel.tsx`

### Bug Risk: Session Mapping Failure [SEVERITY: Medium]

**Description**: The hook request's `cwd` may not match any known session, especially
for external Claude processes started outside of stapler-squad. This could result in
approval requests with no session context, confusing the user.

**Mitigation**:
- "Floating" approvals (no session match) are still displayed in the UI with
  whatever context is available (cwd, tool name).
- Log a warning when session mapping fails.
- The existing `ssq-hook-handler` notification flow provides a parallel notification
  path for context.

**Prevention Strategy**:
- Use the `X-CS-Session-ID` header for managed sessions (injected at session creation).
- Fall back to cwd matching only for external sessions.
- Include the full cwd in the approval card for disambiguation.

**Files Likely Affected**:
- `server/services/approval_session_mapper.go`

### Bug Risk: Server Not Running When Hook Fires [SEVERITY: Low]

**Description**: If stapler-squad is not running when Claude Code fires a
`PermissionRequest` hook, the HTTP request fails. Claude Code's HTTP hook handling
treats connection failures as non-blocking errors and shows the normal permission
dialog in the terminal.

**Mitigation**: This is actually the desired graceful degradation behavior. No fix needed.

**Prevention Strategy**: Document that the feature requires stapler-squad to be running.

### Bug Risk: Concurrent Resolve from Multiple Browser Tabs [SEVERITY: Low]

**Description**: If the user has stapler-squad open in multiple browser tabs, both
tabs show the same approval. If the user clicks Approve in both tabs simultaneously,
the second resolve call will fail.

**Mitigation**:
- The `Resolve` method uses mutex protection and removes the approval atomically.
- The second call returns an error ("approval already resolved").
- The web UI handles this error gracefully (show "Already resolved" message).
- The `ApprovalResolvedEvent` broadcasts to all clients, so both tabs update.

**Prevention Strategy**:
- Use optimistic UI updates with error rollback.
- Broadcast resolved events so all tabs stay in sync.

**Files Likely Affected**:
- `server/services/approval_store.go`
- `web-app/src/lib/hooks/useApprovals.ts`

### Bug Risk: Hook Configuration Conflicts [SEVERITY: Low]

**Description**: Writing to `.claude/settings.local.json` might conflict with
user-created hooks. If the user already has `PermissionRequest` hooks configured,
our injected hook could interfere.

**Mitigation**:
- Read existing `.claude/settings.local.json` before writing.
- Merge our hook into the existing `PermissionRequest` array (hooks run in parallel).
- Tag our hook with a `statusMessage` field for identification.
- Claude Code deduplicates identical HTTP hooks by URL.

**Prevention Strategy**:
- Use a read-modify-write pattern for the settings file.
- Add a `"statusMessage": "stapler-squad approval"` field to identify our hook.
- Test with pre-existing user hooks to verify coexistence.

**Files Likely Affected**:
- `session/instance.go` (session creation)
- `session/mux/hooks.go`

---

## Testing Strategy

### Unit Tests

| Component | Test Cases |
|:---|:---|
| `ApprovalStore` | Create, Get, Resolve, double-resolve, expired cleanup, concurrent access |
| `ApprovalSessionMapper` | Managed session by header, cwd match, worktree match, mux session, no match |
| `ApprovalHandler` | Parse hook JSON, block until resolved, timeout handling, deny formatting |
| Proto serialization | Round-trip PendingApproval proto, event serialization |

### Integration Tests

| Scenario | Description |
|:---|:---|
| End-to-end approve | Simulate hook POST, resolve via API, verify response |
| End-to-end deny | Simulate hook POST, deny via API, verify deny response |
| Timeout | Simulate hook POST, wait for server timeout, verify cleanup |
| Concurrent | Multiple simultaneous approval requests from different sessions |
| Session restart | Pending approval canceled when session restarts |
| Event streaming | Approval events received by connected WebSocket clients |

### Manual Testing Checklist

- [ ] Create a session with auto_yes=false, trigger a Bash command that needs approval.
- [ ] Verify the approval notification appears in the web UI within 2 seconds.
- [ ] Click Approve and verify Claude Code proceeds with the command.
- [ ] Trigger another approval, click Deny, verify Claude Code reports the denial.
- [ ] Wait for the approval to expire, verify "Expired" state in UI.
- [ ] Test on a mobile device (phone/tablet) -- verify buttons are tappable.
- [ ] Test with multiple sessions having concurrent approval requests.
- [ ] Test with claude-mux external session.
- [ ] Kill stapler-squad while an approval is pending, restart, verify cleanup.

---

## Dependency Graph

```
Story 1: Backend Infrastructure
    |
    +---> Task 1.1: Data types (ApprovalStore)
    |         |
    +---> Task 1.2: Session mapper
    |         |
    +---> Task 1.3: Expiration cleanup
    |
    v
Story 2: Hook Script & Endpoints (depends on Story 1)
    |
    +---> Task 2.1: HTTP hook endpoint (permission-request)
    |         |
    +---> Task 2.2: Resolve endpoint
    |         |
    +---> Task 2.3: List pending endpoint
    |         |
    +---> Task 2.4: Inject hook config into managed sessions
    |         |
    +---> Task 2.5: Update claude-mux hook config
    |
    v
Story 3: ConnectRPC API (depends on Story 2)
    |
    +---> Task 3.1: Proto definitions
    |         |
    +---> Task 3.2: ConnectRPC handlers
    |         |
    +---> Task 3.3: Event stream integration
    |
    v
Story 4: Web UI Components (depends on Story 3)
    |
    +---> Task 4.1: ApprovalCard component
    |         |
    +---> Task 4.2: ApprovalPanel component
    |         |
    +---> Task 4.3: NotificationToast approval buttons
    |         |
    +---> Task 4.4: useApprovals hook
    |
    v
Story 5: Review Queue Integration (depends on Story 4)
    |
    +---> Task 5.1: Enrich review items with approval data
    |         |
    +---> Task 5.2: Approve/Deny in review queue
    |         |
    +---> Task 5.3: Approval notification sound
    |
    v
Story 6: Mobile UX (depends on Story 4)
    |
    +---> Task 6.1: Responsive layout
    |         |
    +---> Task 6.2: Mobile notification banner
    |         |
    +---> Task 6.3: Vibration feedback
```

**Critical Path**: Stories 1 -> 2 -> 3 -> 4 form the minimum viable feature.
Stories 5 and 6 are enhancements that can be deferred.

**MVP (Minimum Viable Product)**: Stories 1-4 deliver the core functionality:
- Claude Code sends approval requests to stapler-squad via HTTP hooks.
- The web UI shows pending approvals with Approve/Deny buttons.
- User decisions flow back to Claude Code.
- Graceful timeout/degradation when user does not respond.

Stories 5-6 improve the experience but are not required for the feature to work.
