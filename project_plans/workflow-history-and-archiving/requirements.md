# Requirements: Workflow History, Archiving & Pause Memory Optimization

## Problem Statement

Three related gaps in the Workflows feature:

1. **No run history**: Workflow executions are invisible once the session leaves the default list. Users can't tell if a workflow ran, succeeded, or failed.
2. **Session list pollution**: Completed workflow sessions accumulate in the session list indefinitely, burying live sessions. Bulk removal isn't safe (destroys history).
3. **Paused session memory waste**: When a session is paused, the underlying tmux process stays alive consuming RAM. On memory-constrained devices (mobile, thin servers) this causes performance degradation. Additionally, after a server restart sessions in the same directory with different UUIDs could resume to the wrong Claude conversation.

---

## User Stories

### US-1: Run a workflow from the Workflows page
**As a** user on the `/workflows` page,  
**I want to** click a "Run ▶" button next to any workflow,  
**So that** a new session is created and I can jump to it immediately.

**Acceptance criteria:**
- Each workflow row has a "Run ▶" button that calls `RunWorkflow` RPC
- Button shows a loading state while the session is being created
- After creation, the session is visible in the session list

### US-2: See recent run history per workflow
**As a** user on the `/workflows` page,  
**I want to** expand a workflow row and see the last 5 sessions it spawned,  
**So that** I can check on recent executions without leaving the page.

**Acceptance criteria:**
- Each workflow row has a ▸ "Recent Runs" toggle
- Expanding it shows: status badge, session title, timestamp
- Each run is a clickable link to the session
- Empty state shown when no runs exist

### US-3: Archive completed sessions
**As a** user,  
**I want to** archive sessions I no longer need active,  
**So that** the session list stays focused on live work without losing history.

**Acceptance criteria:**
- `ArchiveSession` RPC sets `archived_at` on a session
- `UnarchiveSession` RPC clears `archived_at`
- Default `ListSessions` excludes archived sessions
- `include_archived: true` filter reveals them

### US-4: Auto-archive workflow sessions on completion
**As a** workflow operator,  
**I want to** workflow sessions disappear from the default list when they stop,  
**So that** I don't manually clean up every run.

**Acceptance criteria:**
- Sessions with a `workflow_id` are automatically archived when they exit
- Auto-archive fires via lifecycle hook (not poll)
- Non-workflow sessions are never auto-archived by this logic

### US-5: Link sessions back to their originating workflow
**As a** user or system,  
**I want** each session to know which workflow created it,  
**So that** run history, filters, and analytics can be scoped to a workflow.

**Acceptance criteria:**
- `workflow_id` optional UUID field on Session (ent schema + proto)
- `FireNow` in scheduler passes `workflow_id` when creating sessions
- `ListSessions` accepts `workflow_id` filter
- Field is orphan-safe (plain string, not FK edge — deleting workflow doesn't cascade)

### US-6: Pause kills tmux to free memory
**As a** user on a memory-constrained device,  
**I want** pausing a session to kill the underlying tmux process,  
**So that** paused sessions consume no RAM while inactive.

**Acceptance criteria:**
- Calling `Pause` (via `UpdateSession`) kills the tmux session after stopping the controller
- Claude session UUID is persisted to DB before the kill (guaranteed by `wireClaudeSessionIDSavedCallback`)
- Falls back to detach if kill fails (non-fatal, logged)
- Resumed sessions re-launch with `--resume <uuid>` using the latest stored UUID

### US-7: Resume correctly identifies Claude session after restart
**As a** user who restarts the server,  
**I want** paused sessions to resume to the correct Claude conversation,  
**So that** I don't lose conversation history.

**Acceptance criteria:**
- `Resume()` dead-tmux path rebuilds `TmuxSession` with current `claudeSession.ConversationUUID`
- `--resume <uuid>` flag injected for Claude programs
- Latest stored UUID (from DB) used on resume, not the stale launch command

---

## Scope

### In Scope
- `workflow_id` + `archived_at` fields on Session (ent schema + proto + serialization)
- `ArchiveSession` / `UnarchiveSession` RPCs
- `ListSessions` `workflow_id` + `include_archived` filters
- `FireNow` passes `workflow_id` to `CreateSession`
- Auto-archive via `autoArchiveListener` lifecycle hook in SessionService
- WorkflowsPanel: Run ▶ button
- WorkflowsPanel: Recent Runs accordion (last 5, status badge, link, timestamp)
- `archiveSession`, `unarchiveSession`, `listSessionsByWorkflow` in `useSessionService`
- `Pause()` kills tmux instead of detach; resume path reinitializes TmuxSession with latest UUID

### Out of Scope
- Bulk archive operations
- Hard-delete / TTL for archived sessions
- Workflow run notifications / webhooks
- "Show archived" toggle in main session list (deferred)
- Analytics / metrics on workflow runs

---

## Non-Functional Requirements

- `ArchiveSession` / `UnarchiveSession` respond in < 200ms
- Auto-archive must not block the exit event path (fires in goroutine)
- `ListSessions` default filter change must not break existing callers (filter only applies when `include_archived` is explicitly false and no internal caller changes needed for review queue / analytics)
- Pause kill must not leave tmux sessions as zombies on kill failure

---

## Constraints

- ent schema codegen must use `--feature sql/upsert` flag
- No new ent edge FK between Session and Workflow (orphan-safety)
- Three persistence layers (Instance, InstanceData, ent) must all be updated consistently
- Auto-archive must only fire for sessions with `WorkflowID != ""`
