# Resume Session Improvements -- Implementation Tasks

Epic: Resume Session Improvements
Plan: `project_plans/resume-session-improvements/implementation/plan.md`
ADRs: `project_plans/resume-session-improvements/decisions/ADR-001..003`

---

## Epic Overview

Three problem areas, each an independent story:

| Story | Problem | Solution |
|-------|---------|----------|
| 1. Workspace Switch Fix | `SwitchWorkspace()` starts fresh conversation -- CLAUDE.md from new dir not loaded with conversation context | Extract conversation ID before kill; restart with `--resume <id>` from new dir |
| 2. Resume Modal | Resume button bypasses all UI -- no name editing, no conflict prevention | New `ResumeSessionModal` pre-filled with session data; `UpdateSession` RPC carries title+tags+status atomically |
| 3. Unified Creation | Two creation paths (full-page wizard + modal) cause confusion | Redirect `/sessions/new` to home page; all entry points open same modal |

---

## Architecture Decisions

- **ADR-001**: New `ResumeSessionModal` component (not reusing `SessionWizard`). Wizard's multi-step flow is wrong for a 2-field edit. Different RPC (UpdateSession vs CreateSession).
- **ADR-002**: Kill+restart with pre-extracted conversation ID. No hot-reload mechanism exists in Claude CLI. `--resume <id>` loads CLAUDE.md from CWD at launch.
- **ADR-003**: Parenthetical numeric suffix for name uniqueness (`name (2)`, `name (3)`). Strip existing suffix before incrementing. Client-side suggestion, server-side enforcement.

---

## Story 1: Fix Workspace Switch CLAUDE.md Reload

### Task 1.1: Extract Conversation ID Before Workspace Switch Kill

**Objective**: Ensure `claudeSession.SessionID` is populated before `KillSession()` in `SwitchWorkspace()`, so the restart uses `--resume <id>`.

**Current behavior**: `SwitchWorkspace()` in `session/instance_workspace.go` kills the tmux session at line 164 and restarts at line 191. The `claudeSession` field survives the kill (it is an in-memory struct field, not tied to tmux). However, `claudeSession.SessionID` is only populated if it was set during session creation via `ResumeId` option or by a previous `tryExtractConversationUUID()` call. For sessions created without a `ResumeId`, the field is empty and the restart starts a fresh conversation.

**Target behavior**: Before killing, call the existing `tryExtractConversationUUID()` method to scan JSONL files and populate `claudeSession.SessionID`. The `ClaudeCommandBuilder` (used by `Start()` at line 191) already reads this field to append `--resume <id>`.

**Prerequisites**: None. This is the first task.

**Files to read**:
- `session/instance_workspace.go` -- Full file, focus on `SwitchWorkspace()` (lines 75-206)
- `session/instance.go` -- `tryExtractConversationUUID()` method (~line 1874), `ClaudeCommandBuilder` usage (~line 836), `claudeSession` field (line 142)
- `session/claude_command_builder.go` -- How `--resume` flag is added

**Files to modify**:
- `session/instance_workspace.go` -- Add extraction call before line 164

**Implementation**:

1. In `SwitchWorkspace()`, after the comment at line 160 (`// 4. Claude session ID is preserved...`) and before line 164 (`// 5. Kill tmux session`), add:

```go
// 4.5. Extract conversation ID from JSONL files before kill
// This ensures --resume flag is set on restart even for sessions
// that weren't created with a ResumeId.
if i.claudeSession == nil || i.claudeSession.SessionID == "" {
    log.InfoLog.Printf("[Workspace] Extracting conversation ID before kill for session '%s'", i.Title)
    i.tryExtractConversationUUID()
    if i.claudeSession != nil && i.claudeSession.SessionID != "" {
        log.InfoLog.Printf("[Workspace] Captured conversation ID '%s' for resume", i.claudeSession.SessionID)
    } else {
        log.InfoLog.Printf("[Workspace] No conversation ID found -- restart will begin fresh conversation")
    }
}
```

2. Verify that `tryExtractConversationUUID()` does not require the tmux session to be alive (it reads JSONL files from `~/.claude/projects/`, independent of tmux).

3. Verify that `Start(false)` at line 191 flows through to `ClaudeCommandBuilder` which reads `i.claudeSession.SessionID`.

**Validation**:
- `go test ./session -run TestSwitchWorkspace` (add test if not exists)
- Manual test: Create a session, run a few commands, switch workspace. Check tmux pane output for `--resume <uuid>` in the claude command.
- Check logs for "Captured conversation ID" message.

**Test strategy**:
- Add a test in `session/instance_workspace_test.go` that creates an Instance with a mock tmux manager, calls `SwitchWorkspace()`, and verifies `claudeSession.SessionID` is non-empty after the call.
- Mock `tryExtractConversationUUID()` by pre-populating a JSONL file in a temp directory.

---

### Task 1.2: Add Concurrent Switch Guard in Workspace Service

**Objective**: Prevent two concurrent `SwitchWorkspace` RPCs on the same session from causing state corruption.

**Current behavior**: The `Instance.SwitchWorkspace()` method holds `stateMutex` (line 76), which serializes calls at the instance level. However, `WorkspaceService.SwitchWorkspace()` in `server/services/workspace_service.go` does `LoadInstances -> findInstance -> operate -> SaveInstances` without an outer lock. Two concurrent requests for different sessions could interleave their `SaveInstances` calls, causing one to overwrite the other.

**Target behavior**: Add a per-session "switching in progress" guard in the service layer. If a switch is already in progress for a given session, return `CodeUnavailable`.

**Prerequisites**: Task 1.1 (conceptually independent but should land together).

**Files to read**:
- `server/services/workspace_service.go` -- Full file, focus on `SwitchWorkspace()` handler (~line 207-291)
- `session/instance_workspace.go` -- `stateMutex` usage at line 76-77

**Files to modify**:
- `server/services/workspace_service.go` -- Add `sync.Map` for in-progress tracking

**Implementation**:

1. Add a field to `WorkspaceService`:
```go
type WorkspaceService struct {
    // ... existing fields ...
    switchingMu sync.Map // map[string]bool -- session IDs with switch in progress
}
```

2. At the start of the `SwitchWorkspace` handler, before `findInstance()`:
```go
// Guard against concurrent switches on the same session
if _, loaded := s.switchingMu.LoadOrStore(req.Msg.Id, true); loaded {
    return nil, connect.NewError(connect.CodeUnavailable,
        fmt.Errorf("workspace switch already in progress for session '%s'", req.Msg.Id))
}
defer s.switchingMu.Delete(req.Msg.Id)
```

3. This is lightweight (no mutexes held across I/O) and only blocks the same session.

**Validation**:
- `go test ./server/services -run TestSwitchWorkspace`
- Add a test that starts two goroutines calling `SwitchWorkspace` on the same session ID. Verify one returns `CodeUnavailable`.

**Test strategy**:
- Create a mock instance that has a slow `SwitchWorkspace()` (e.g., sleep 100ms). Start two concurrent RPC calls. Assert exactly one succeeds and one returns `CodeUnavailable`.

---

## Story 2: Resume Session Modal

### Task 2.1: Extend UpdateSessionRequest Proto with Tags Field

**Objective**: Add `tags` to `UpdateSessionRequest` so the resume modal can update name, tags, and status in a single atomic RPC call.

**Current behavior**: `UpdateSessionRequest` has fields: `id`, `status`, `category`, `title`, `program`. The `tags` field is missing, so updating tags requires a separate RPC or direct Instance field manipulation.

**Target behavior**: `UpdateSessionRequest` includes `repeated string tags = 6;`. The `UpdateSession` handler applies tag changes alongside other field updates. When the frontend sends `{id, status: RUNNING, title: "new name", tags: ["tag1", "tag2"]}`, all three are applied atomically before saving.

**Prerequisites**: None. This is the first task in Story 2.

**Files to read**:
- `proto/session/v1/session.proto` -- `UpdateSessionRequest` message (lines 262-277)
- `server/services/session_service.go` -- `UpdateSession` handler (lines 597-699)
- `session/instance.go` -- `SetTags()` method and `Tags` field (lines 105-108)

**Files to modify**:
- `proto/session/v1/session.proto` -- Add `repeated string tags = 6;` to `UpdateSessionRequest`
- `server/services/session_service.go` -- Add tags handling in `UpdateSession` handler
- Generated files via `make generate-proto`

**Implementation**:

1. In `proto/session/v1/session.proto`, add to `UpdateSessionRequest`:
```protobuf
message UpdateSessionRequest {
  string id = 1;
  optional SessionStatus status = 2;
  optional string category = 3;
  optional string title = 4;
  optional string program = 5;
  // Update session tags. If non-empty, replaces all existing tags.
  repeated string tags = 6;
}
```

2. Run `make generate-proto` to regenerate Go and TypeScript code.

3. In `server/services/session_service.go`, in the `UpdateSession` handler, add after the program update block (~line 672):
```go
// Handle tags update
if len(req.Msg.Tags) > 0 {
    instance.SetTags(req.Msg.Tags)
    updatedFields = append(updatedFields, "tags")
}
```

4. Verify `SetTags()` exists on Instance (it does -- backed by `TagManager`).

**Important ordering note**: The handler processes status change (including resume) AFTER metadata updates. Currently status is processed first (line 632). For the resume modal UX, we need title and tags to be applied BEFORE the resume so the session resumes with the new name. Reorder the handler to process title/category/tags/program first, then status change last. This is a safe reorder because the current code applies all changes before saving (line 676).

Actually, looking at the code more carefully: the status change (Pause/Resume) at line 632-647 happens before title update at line 657. This means if resume fails, the title would still be updated -- but the save at line 676 would still persist the title change. To make it atomic, **move the status change block to after all metadata updates** (after the program block). This way, if resume fails, we return an error before saving any changes.

Revised handler order:
1. Title update (with uniqueness check)
2. Category update
3. Tags update
4. Program update
5. Status change (pause/resume) -- last, because Resume() can fail
6. Save to storage

**Validation**:
- `make generate-proto` succeeds without errors
- `go build .` compiles
- `go test ./server/services -run TestUpdateSession`
- Add test: call `UpdateSession` with `{id, status: RUNNING, title: "new-name", tags: ["a", "b"]}`, verify instance has all three applied after the call

**Test strategy**:
- Unit test with mock storage: verify tags are set on the instance after `UpdateSession`
- Unit test: verify that if resume fails, title and tags are NOT persisted (handler returns error before save)

---

### Task 2.2: Create ResumeSessionModal React Component

**Objective**: Build a lightweight single-panel modal for editing session name and tags before resuming.

**Current behavior**: The Resume button in `SessionCard.tsx` (line 494) calls `onResume?.()` which flows through `SessionList -> page.tsx -> resumeSession(id)` which calls `updateSession(id, { status: RUNNING })` directly. No UI interaction.

**Target behavior**: The Resume button triggers a modal. The modal shows: editable name field (auto-focused), editable tags, read-only context (branch, program, path). "Resume Session" button confirms. If name conflicts with existing session, auto-suggest a unique name.

**Prerequisites**: Task 2.1 (proto change, so the RPC accepts tags).

**Files to read**:
- `web-app/src/components/sessions/SessionCard.tsx` -- Current `onResume` prop usage (line 15, 494)
- `web-app/src/components/sessions/SessionWizard.tsx` -- For design patterns and existing modal style
- `web-app/src/lib/hooks/useSessionService.ts` -- `resumeSession` function (line 230-237)
- Any existing modal/dialog component in the codebase for style consistency

**Files to create**:
- `web-app/src/components/sessions/ResumeSessionModal.tsx`
- `web-app/src/components/sessions/ResumeSessionModal.module.css`
- `web-app/src/utils/sessionNameUtils.ts`

**Implementation**:

1. **Create `web-app/src/utils/sessionNameUtils.ts`**:

```typescript
/**
 * Generate a unique session name by appending parenthetical numeric suffix.
 * Strips existing suffix before incrementing: "name (3)" -> "name (4)".
 */
export function generateUniqueName(baseName: string, existingNames: string[]): string {
  const existingSet = new Set(existingNames);

  // Strip any existing numeric suffix: "name (3)" -> "name"
  const stripped = baseName.replace(/\s*\(\d+\)$/, '').trim();

  // Try stripped name first
  if (!existingSet.has(stripped)) return stripped;

  // Try original name if different from stripped
  if (baseName !== stripped && !existingSet.has(baseName)) return baseName;

  // Increment suffix until unique
  let counter = 2;
  while (existingSet.has(`${stripped} (${counter})`)) counter++;
  return `${stripped} (${counter})`;
}

/**
 * Check if a session name conflicts with existing sessions.
 * Excludes the current session's own name from the check.
 */
export function hasNameConflict(
  name: string,
  existingNames: string[],
  currentSessionId?: string,
  sessions?: Array<{ id: string; title: string }>
): boolean {
  if (sessions && currentSessionId) {
    return sessions.some(s => s.id !== currentSessionId && s.title === name);
  }
  return existingNames.includes(name);
}
```

2. **Create `web-app/src/components/sessions/ResumeSessionModal.tsx`**:

Key design decisions:
- Use `key={session.id}` on the component to force remount when session changes
- `useState(() => ...)` initializer pattern to capture initial values once
- Auto-detect conflict on mount and auto-suggest unique name
- Show hint: "Name updated to avoid conflict with existing session"
- Enter submits, Escape cancels
- Auto-focus the name input field
- Reuse existing tag pill pattern from the codebase
- Read-only fields: branch, program, path (displayed as context)

Props interface:
```typescript
interface ResumeSessionModalProps {
  session: Session;
  sessions: Session[]; // all sessions, for conflict detection
  onConfirm: (updates: { title: string; tags: string[] }) => void;
  onCancel: () => void;
}
```

3. **Create `web-app/src/components/sessions/ResumeSessionModal.module.css`**:
- Follow existing modal patterns in the codebase
- Dark/light mode support using CSS variables
- Responsive layout

**Validation**:
- Unit test for `generateUniqueName`:
  - `("foo", ["foo"]) -> "foo (2)"`
  - `("foo (2)", ["foo", "foo (2)"]) -> "foo (3)"`
  - `("foo", []) -> "foo"`
  - `("foo (3)", ["foo"]) -> "foo (3)"` (original name available since stripped is taken)
- Unit test for `ResumeSessionModal`:
  - Renders with session data pre-filled
  - Shows conflict hint when name matches existing session
  - Calls `onConfirm` with updated title and tags on submit
  - Calls `onCancel` on Escape

**Test strategy**:
- Jest unit tests for `sessionNameUtils.ts` (pure functions, easy to test)
- React Testing Library tests for `ResumeSessionModal.tsx`

---

### Task 2.3: Wire ResumeSessionModal into Session List and Page

**Objective**: Replace the direct `resumeSession(id)` RPC call with a modal-first flow. The modal opens, user edits, confirms, and the RPC is sent with updated metadata.

**Current behavior**: In `web-app/src/app/page.tsx` (line 256), `onResumeSession={resumeSession}` passes the hook's `resumeSession(id)` function which calls `updateSession(id, { status: RUNNING })`. In `SessionList.tsx` (line 508), `onResume={() => onResumeSession?.(session.id)}` passes just the session ID string.

**Target behavior**: Clicking Resume opens `ResumeSessionModal`. On confirm, calls `resumeSession(id, { title, tags })`. The `useSessionService.resumeSession` function sends `updateSession(id, { status: RUNNING, title, tags })`.

**Prerequisites**: Task 2.1 (proto change), Task 2.2 (modal component).

**Files to read**:
- `web-app/src/app/page.tsx` -- `resumeSession` usage (lines 48, 256), `sessions` state
- `web-app/src/components/sessions/SessionList.tsx` -- `onResumeSession` prop type and usage (lines 16, 508)
- `web-app/src/components/sessions/SessionCard.tsx` -- `onResume` prop (line 15)
- `web-app/src/components/sessions/BulkActions.tsx` -- `onResumeAll` (line 8)
- `web-app/src/lib/hooks/useSessionService.ts` -- `resumeSession` (lines 230-237)

**Files to modify**:
- `web-app/src/lib/hooks/useSessionService.ts` -- Update `resumeSession` signature
- `web-app/src/app/page.tsx` -- Add modal state and rendering
- `web-app/src/components/sessions/SessionList.tsx` -- Change callback to pass full session object

**Implementation**:

1. **Update `useSessionService.ts`**:

Change `resumeSession` to accept optional metadata:
```typescript
const resumeSession = useCallback(
  async (id: string, updates?: { title?: string; tags?: string[] }): Promise<Session | null> => {
    return updateSession(id, {
      status: SessionStatus.RUNNING,
      ...(updates?.title ? { title: updates.title } : {}),
      ...(updates?.tags ? { tags: updates.tags } : {}),
    });
  },
  [updateSession]
);
```

Also update the `updateSession` call to include `tags` in the request payload (the generated TypeScript client should already have the field from Task 2.1's proto change).

2. **Update `SessionList.tsx`**:

Change `onResumeSession` prop type:
```typescript
// Before:
onResumeSession?: (sessionId: string) => void;

// After:
onResumeSession?: (session: Session) => void;
```

Update the callback at line 508:
```typescript
// Before:
onResume={() => onResumeSession?.(session.id)}

// After:
onResume={() => onResumeSession?.(session)}
```

Update bulk resume at line 280-281 to pass full session objects (or keep bulk resume as direct RPC without modal -- ADR-001 says bulk resume skips the modal for speed).

3. **Update `page.tsx`**:

Add state and handlers:
```typescript
const [resumeTarget, setResumeTarget] = useState<Session | null>(null);

const handleResumeRequest = useCallback((session: Session) => {
  setResumeTarget(session);
}, []);

const handleResumeConfirm = useCallback(async (updates: { title: string; tags: string[] }) => {
  if (!resumeTarget) return;
  await resumeSession(resumeTarget.id, updates);
  setResumeTarget(null);
}, [resumeTarget, resumeSession]);
```

Pass to SessionList:
```typescript
onResumeSession={handleResumeRequest}
```

Render the modal:
```tsx
{resumeTarget && (
  <ResumeSessionModal
    key={resumeTarget.id}
    session={resumeTarget}
    sessions={sessions}
    onConfirm={handleResumeConfirm}
    onCancel={() => setResumeTarget(null)}
  />
)}
```

4. **Bulk resume behavior**: Keep `BulkActions.onResumeAll` as direct RPC calls without modal. Bulk operations should not prompt per-session. If name conflicts occur during bulk resume, the server rejects and the user can retry individually.

**Validation**:
- Manual test: Click Resume on a paused session card. Modal appears with name and tags.
- Manual test: Edit name, click "Resume Session". Session resumes with new name.
- Manual test: Resume a session whose name matches another session. Modal shows auto-suggested unique name.
- Manual test: Bulk-select paused sessions, click "Resume All". Sessions resume without modal (direct RPC).
- `make build && make test` passes.

**Test strategy**:
- Integration test: Mount `page.tsx` with mock session service, simulate click on Resume button, verify modal renders, simulate confirm, verify `resumeSession` called with correct args.

---

## Story 3: Unified Session Creation Modal

### Task 3.1: Redirect `/sessions/new` to Home Page with Modal Trigger

**Objective**: Replace the full-page wizard at `/sessions/new` with a redirect to the home page that auto-opens the creation modal.

**Current behavior**: `/sessions/new` renders `NewSessionPage` which mounts `SessionWizard` as a full-page form with its own header and layout. Navigating to `/sessions/new?duplicate=<id>` pre-fills the wizard with the source session's data.

**Target behavior**: `/sessions/new` redirects to `/?new=true`. `/sessions/new?duplicate=<id>` redirects to `/?duplicate=<id>`. The home page detects these query params and auto-opens the `SessionWizard` modal.

**Prerequisites**: None. Independent of Stories 1 and 2.

**Files to read**:
- `web-app/src/app/sessions/new/page.tsx` -- Current full-page wizard (lines 1-133)
- `web-app/src/app/page.tsx` -- Home page, where the wizard modal already exists
- Look for where `showWizard` state or similar is managed in `page.tsx`

**Files to modify**:
- `web-app/src/app/sessions/new/page.tsx` -- Replace with redirect component
- `web-app/src/app/page.tsx` -- Add query param detection to auto-open modal

**Implementation**:

1. **Replace `sessions/new/page.tsx`** with a minimal redirect:
```typescript
"use client";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, Suspense } from "react";

function RedirectContent() {
  const router = useRouter();
  const searchParams = useSearchParams();

  useEffect(() => {
    const duplicate = searchParams.get("duplicate");
    if (duplicate) {
      router.replace(`/?duplicate=${duplicate}`);
    } else {
      router.replace("/?new=true");
    }
  }, [router, searchParams]);

  return <div>Redirecting...</div>;
}

export default function NewSessionPage() {
  return (
    <Suspense fallback={<div>Redirecting...</div>}>
      <RedirectContent />
    </Suspense>
  );
}
```

2. **In `page.tsx`**, detect query params and auto-open wizard:
- Add `useSearchParams()` to read `new` and `duplicate` params
- On mount, if `new=true` or `duplicate=<id>`, set `showWizard = true`
- For `duplicate`, load the session and pass as `initialData`
- After wizard closes (cancel or complete), clear the query params with `router.replace('/')`

3. Find the existing wizard modal rendering in `page.tsx` (search for `SessionWizard` or `showWizard`) and ensure `initialData` is passed when in duplicate mode.

**Validation**:
- Navigate to `/sessions/new` -- verify redirect to `/?new=true` and wizard modal auto-opens
- Navigate to `/sessions/new?duplicate=foo` -- verify redirect to `/?duplicate=foo` and modal opens with pre-filled data
- Direct bookmark to `/?new=true` works (modal opens)
- After closing the modal, URL becomes `/` (no lingering query params)

**Test strategy**:
- E2E test: Navigate to `/sessions/new`, verify URL changes to `/?new=true`, verify wizard modal is visible.

---

### Task 3.2: Update All Entry Points to Open Modal

**Objective**: Audit all "New Session" triggers and ensure they open the wizard modal rather than navigating to a page route.

**Current behavior**: Multiple entry points trigger session creation:
- Header "New Session" button (may use `router.push('/sessions/new')`)
- Session list context menu "Duplicate" (uses `router.push('/sessions/new?duplicate=<id>')`)
- Omnibar quick create (direct RPC, no wizard)
- History browser "Resume" button (direct RPC via `CreateSession`)

**Target behavior**: All entry points that currently navigate to `/sessions/new` instead trigger the modal overlay on the current page.

**Prerequisites**: Task 3.1 (redirect is in place, but this task converts navigation to direct modal triggers).

**Files to read**:
- `web-app/src/components/layout/Header.tsx` -- New Session button
- `web-app/src/components/sessions/SessionList.tsx` -- Duplicate handler
- `web-app/src/components/sessions/Omnibar.tsx` -- Quick create (likely keep as-is since it's a power-user shortcut)
- `web-app/src/app/page.tsx` -- Modal state management

**Files to modify**:
- `web-app/src/components/layout/Header.tsx` -- Change from navigation to callback
- `web-app/src/components/sessions/SessionList.tsx` -- Change duplicate handler
- `web-app/src/app/page.tsx` -- Add callbacks for the Header and duplicate triggers

**Implementation**:

1. **Audit**: Search for all `router.push.*sessions/new` and `/sessions/new` references in `web-app/src/`.

2. **Header.tsx**: If the "New Session" button does `router.push('/sessions/new')`, change it to call an `onNewSession` callback prop. The parent (page.tsx or layout) sets `showWizard = true`.

3. **SessionList.tsx**: If "Duplicate" does `router.push('/sessions/new?duplicate=<id>')`, change to call `onDuplicateSession(sessionId)` callback. The parent loads the session data and opens the wizard modal with `initialData`.

4. **Omnibar.tsx**: Leave as-is. The omnibar is a power-user feature that creates sessions via direct RPC (no wizard needed).

5. **Verify**: No remaining `router.push('/sessions/new')` calls in the codebase after changes.

**Validation**:
- Click "New Session" in header: wizard modal opens (no page navigation)
- Click "Duplicate" in session context menu: wizard modal opens with pre-filled data
- Omnibar create still works independently
- `grep -r "sessions/new" web-app/src/` returns only the redirect page and import paths

**Test strategy**:
- Manual verification of each entry point
- `grep` audit to confirm no lingering navigation calls

---

## Dependency Visualization

```
    Story 1                    Story 2                    Story 3
    (Backend)                  (Full-Stack)               (Frontend)
    =========                  ============               =========

    Task 1.1                   Task 2.1                   Task 3.1
    Extract conv ID            Proto + handler            Redirect /new
    [instance_workspace.go]    [session.proto]            [sessions/new/page.tsx]
    [instance.go]              [session_service.go]       [page.tsx]
         |                          |                          |
         v                          v                          v
    Task 1.2                   Task 2.2                   Task 3.2
    Concurrent guard           Modal component            Update entry points
    [workspace_service.go]     [ResumeSessionModal.tsx]   [Header.tsx]
                               [sessionNameUtils.ts]      [SessionList.tsx]
                                    |
                                    v
                               Task 2.3
                               Wire into page
                               [page.tsx]
                               [SessionList.tsx]
                               [useSessionService.ts]
```

Stories 1, 2, and 3 are fully independent and can proceed in parallel.
Within each story, tasks must be done sequentially.

---

## Context Preparation Guide

### Before Starting Any Task

1. Read this file's task description for the specific task
2. Read the referenced ADRs if the task relates to an architecture decision
3. Read the files listed under "Files to read" for that task
4. Do NOT read the entire codebase -- the file lists are curated

### Per-Task Context

| Task | Must Read | Nice to Have |
|------|-----------|--------------|
| 1.1 | `session/instance_workspace.go`, `session/instance.go` (lines 1874+, 836, 142) | `session/claude_command_builder.go` |
| 1.2 | `server/services/workspace_service.go` | `session/instance_workspace.go` (line 76) |
| 2.1 | `proto/session/v1/session.proto` (lines 262-277), `server/services/session_service.go` (lines 597-699) | `session/instance.go` (Tags/SetTags) |
| 2.2 | `web-app/src/components/sessions/SessionCard.tsx`, any existing modal component | `web-app/src/components/sessions/SessionWizard.tsx` (for style reference) |
| 2.3 | `web-app/src/app/page.tsx`, `web-app/src/components/sessions/SessionList.tsx`, `web-app/src/lib/hooks/useSessionService.ts` | `web-app/src/components/sessions/BulkActions.tsx` |
| 3.1 | `web-app/src/app/sessions/new/page.tsx`, `web-app/src/app/page.tsx` | -- |
| 3.2 | `web-app/src/components/layout/Header.tsx`, `web-app/src/components/sessions/SessionList.tsx` | `web-app/src/components/sessions/Omnibar.tsx` |

---

## Known Issues

### Bug: Concurrent Workspace Switches Overwrite Storage [SEVERITY: High]

**Description**: The `LoadInstances -> modify -> SaveInstances` pattern in `workspace_service.go` is not atomic. Two concurrent switches on DIFFERENT sessions can interleave: Thread 1 loads instances, Thread 2 loads instances, Thread 1 saves, Thread 2 saves (overwriting Thread 1's changes).

**Mitigation**: Task 1.2 adds a per-session guard preventing double-switch on the SAME session. The cross-session race requires storage-level transactions (out of scope).

**Files affected**: `server/services/workspace_service.go` (lines 256-276)

**Prevention**: Design storage operations as atomic read-modify-write. Future: migrate to SQLite with transactions.

---

### Bug: TOCTOU Race in Name Uniqueness Check [SEVERITY: Medium]

**Description**: Client generates unique name from current session list. Between the client check and the server receiving the RPC, another session could be created or renamed to the same name. The `UpdateSession` handler checks uniqueness again (server-side guard), but the user sees a confusing error after the modal confirmed the name was available.

**Mitigation**: Server-side uniqueness check is authoritative. If server rejects, frontend should show an inline error in the modal and let the user try again (do not close the modal on failure).

**Files affected**: `web-app/src/components/sessions/ResumeSessionModal.tsx`, `server/services/session_service.go` (lines 657-663)

**Prevention**: Keep the modal open on server error. Show the specific error message ("session with title 'X' already exists") and let the user pick a new name.

---

### Bug: Resume Double-Submit Race [SEVERITY: Medium]

**Description**: User clicks "Resume Session" in the modal, then clicks again before the RPC completes. Two `UpdateSession` calls fire. The `Instance.Resume()` method checks `Status != Paused` under `stateMutex`, so the second call should fail. However, there is a TOCTOU window in `UpdateSession` between reading status (line 632) and calling `Resume()` (line 643) -- no mutex is held at the service layer.

**Mitigation**: Disable the "Resume Session" button in the modal after first click (standard loading state pattern). Server-side `Resume()` has its own status guard that prevents double-resume.

**Files affected**: `web-app/src/components/sessions/ResumeSessionModal.tsx`, `server/services/session_service.go` (lines 632-647)

**Prevention**: Add `isSubmitting` state to the modal. Set to `true` on submit, disable button. Reset on error.

---

### Bug: Conversation ID Missing for Young Sessions [SEVERITY: Low]

**Description**: If workspace switch happens within seconds of session creation (before Claude writes JSONL entries), `tryExtractConversationUUID()` returns empty. The restart starts a fresh conversation.

**Mitigation**: Acceptable degradation. Log a warning. Sessions this young have minimal conversation to preserve.

**Files affected**: `session/instance_workspace.go` (Task 1.1 addition)

**Prevention**: None needed. This is an inherent timing constraint of the JSONL persistence model.

---

### Bug: Orphaned Worktree on Resume Failure [SEVERITY: Medium]

**Description**: `Resume()` sets up a git worktree (line 1215), then starts tmux (line 1252). If tmux start fails, cleanup is attempted (line 1255-1259) but can itself fail, leaving an orphaned worktree that blocks the next resume attempt.

**Mitigation**: Existing behavior. Not changed by this feature. Error details are returned to the user. Manual cleanup: `git worktree remove <path>`.

**Files affected**: `session/instance.go` (lines 1215-1280)

**Prevention**: Future: make `gitManager.Cleanup()` idempotent and add retry logic. Out of scope for this feature.

---

### Bug: Bulk Resume Ignores Name Conflicts [SEVERITY: Low]

**Description**: Bulk resume (via `BulkActions.onResumeAll`) skips the modal and resumes all selected sessions with their current names. If two paused sessions have the same name (possible via external sessions), one resume will fail.

**Mitigation**: Server-side uniqueness check rejects the duplicate. User sees error for the failed session and can retry individually with the modal.

**Files affected**: `web-app/src/components/sessions/BulkActions.tsx`

**Prevention**: Future: bulk resume could generate unique names automatically for conflicts. Out of scope for this feature.
