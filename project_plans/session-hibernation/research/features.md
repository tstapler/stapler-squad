# Session Hibernation Feature Research — UX Patterns & Status Handling

## 1. Existing Session Actions — UI Surfacing

### SessionRow Component (`web-app/src/components/sessions/SessionRow.tsx`)
- All actions are surfaced through `SessionActionsOverflow` (right-click context menu or ··· button)
- **Primary actions** (Resume/Pause) appear as:
  - A shortcut button (when `showPrimaryAction=true`)
  - Also available in the overflow menu as fallback
- No danger confirmation for Pause (immediate, non-destructive)
- Keyboard support: Enter/Space to activate row, ESC to close menu
- Session row is clickable (`onClick={onClick}`), context-menu opens at cursor (`onContextMenu=handleContextMenu`)

### SessionCard Component (`web-app/src/components/sessions/SessionCard.tsx`)
- Larger card view with detailed metadata
- Same `SessionActionsOverflow` component in footer for consistency
- Status badges rendered inline in the header via `getStatusColor()` and `getStatusText()` functions
- Card shows full session details: program, branch, path, GitHub PR info, tags, timestamps
- Terminal preview (snapshot) only shown for RUNNING sessions
- Inline title editing support

### Right-Click Context Menu Pattern (`SessionActionsOverflow.tsx`)
**Technical implementation:**
- `useImperativeHandle` for parent control: `openAt(x, y)` to position menu at cursor
- Portal rendering to document.body (escapes parent overflow hidden)
- Menu items are conditionally rendered based on session status and callback availability
- FocusTrap hook manages focus within modal dialog and menu
- Menu closes on ESC key or click outside
- All menu items use `role="menuitem"` (ARIA compliance)

**Menu structure (ordered):**
1. Resume (if paused/ready) OR Pause (if running)
2. Rename, Restart, Checkpoint, Clone, Create PR
3. Open in new pane, Edit Tags, Rate limit toggle, New Workspace, Switch Workspace
4. Clear Conversation State, Delete (danger styling)

## 2. Existing Session Status Values — Frontend Handling

### SessionStatus enum (`proto/session/v1/types.proto` and Go `session/instance.go`)
**Proto values (8 states):**
- `SESSION_STATUS_UNSPECIFIED = 0` — not used
- `SESSION_STATUS_RUNNING = 1` — agent actively working
- `SESSION_STATUS_READY = 2` — idle, waiting for user input
- `SESSION_STATUS_LOADING = 3` — starting up or loading
- `SESSION_STATUS_PAUSED = 4` — worktree removed, branch preserved (terminal state)
- `SESSION_STATUS_NEEDS_APPROVAL = 5` — waiting for user approval on prompt
- `SESSION_STATUS_CREATING = 6` — transient initialization state
- `SESSION_STATUS_STOPPED = 7` — terminal state, cannot transition further

**Frontend color/style mapping** (`SessionCard.tsx` lines 127–167):
```typescript
getStatusColor(status) => one of: statusRunning, statusReady, statusPaused, statusLoading, statusNeedsApproval, statusUnknown
getStatusText(status) => human label: "Running", "Ready", "Paused", "Loading", "Needs Approval", "Creating", "Stopped"
```

### StatusBadge Component (`StatusBadge.tsx`)
- Separate from main session status (used for review queue attention reasons)
- Shows AttentionReason or detected status (e.g., "Idle", "Error", "Tests Failing")
- Icon + label pattern: e.g., `⏰ Idle`, `❌ Error`, `✅ Complete`
- Session card shows both main status badge AND optional detected status badge

### Status Rendering Locations
- **SessionRow**: status dot only (data-status attribute for CSS styling)
- **SessionCard**: status badge in header + row + detailed display
- Both use vanilla-extract CSS (`.css.ts` files) with theme tokens

## 3. Right-Click Context Menu Architecture

### Current Implementation (`SessionActionsOverflow.tsx`)
**File structure:**
- Export `SessionActionsOverflowHandle` interface: `openAt(x: number, y: number): void`
- Export `SessionActionsOverflowProps` interface: all action callbacks + UI props
- Forwardref component for imperative menu positioning

**Callback props** (all optional):
```typescript
onResume?: () => void
onPause?: () => void
onDelete?: () => Promise<void>
onRestart?: (sessionId: string) => Promise<boolean | void>
onCreateCheckpoint?: (sessionId: string, label: string) => Promise<boolean>
onClone?: () => void
onOpenInNewPane?: () => void
onNewWorkspace?: () => void
onRunOneShot?: (sessionId: string) => Promise<void>
onSetRateLimitEnabled?: (sessionId: string, enabled: boolean) => void
onClearConversationState?: (sessionId: string) => Promise<boolean>
onUpdateTags?: (sessionId: string, tags: string[]) => void
onRenameRequest?: () => void
onWorkspaceSwitchRequest?: () => void
```

**Conditional rendering logic:**
```typescript
{!(isPaused || isReady) && onResume && <Resume button>}
{!isRunning && onPause && <Pause button>}
{condition && onCallback && <Menu item>}
```

**Confirmation dialogs:**
- Restart: modal dialog with confirmation
- Delete: modal dialog with confirmation + error message display
- Checkpoint: modal dialog with label input + error message
- Others: immediate action (no confirmation)

### To Add "Hibernate" Action
**Minimal additions:**
1. Add callback: `onHibernate?: () => void`
2. Add conditional menu item (similar to Pause pattern)
3. Call from parent with `useSessionService().hibernateSession(id)`

**Conditional placement logic:**
- Show "Hibernate" when: `isRunning && onHibernate`
- Hide "Pause" when: hibernation available (mutual exclusivity? or both?)
- No confirmation dialog needed (non-destructive, async checkpoint happens in background)

## 4. Pause Feature — End-to-End Flow (Reference Implementation)

### Proto Definition (`proto/session/v1/session.proto`)
**Request message:**
```protobuf
message UpdateSessionRequest {
  string id = 1;
  optional SessionStatus status = 2;  // Can send PAUSED or RUNNING
  optional string category = 3;
  optional string title = 4;
  // ... other optional fields
}

message UpdateSessionResponse {
  Session session = 1;  // Updated session with new status
}
```

**Service RPC:**
```protobuf
rpc UpdateSession(UpdateSessionRequest) returns (UpdateSessionResponse) {}
```

### Go Backend Handler (`server/services/session_service.go`)
**Likely structure** (based on requirements.md pattern):
1. Load session by ID
2. Validate new status transition (e.g., PAUSED allowed from RUNNING or READY, not from STOPPED)
3. If status = PAUSED: kill tmux session, preserve git worktree/branch
4. Update session.Status field in database
5. Broadcast SessionEvent (WatchSessions stream gets notified)
6. Return updated Session proto

### Frontend Hook (`web-app/src/lib/hooks/useSessionService.ts`)
**pauseSession implementation:**
```typescript
const pauseSession = useCallback(
  async (id: string): Promise<Session | null> => {
    return updateSession(id, {
      status: SessionStatus.PAUSED,
    });
  },
  [updateSession]
);
```

**resumeSession implementation:**
```typescript
const resumeSession = useCallback(
  async (id: string, updates?: { title?: string; tags?: string[] }): Promise<Session | null> => {
    return updateSession(id, {
      status: SessionStatus.RUNNING,
      ...(updates?.title ? { title: updates.title } : {}),
      ...(updates?.tags && updates.tags.length > 0 ? { tags: updates.tags } : {}),
    });
  },
  [updateSession]
);
```

**updateSession wrapper** (calls client.updateSession RPC):
- Dispatches `upsertSession(session)` to Redux store on success
- Dispatches error to Redux on failure
- Handles retries via ConnectRPC transport

### UI Layer (`SessionRow.tsx`, `SessionCard.tsx`)
**Props pattern:**
```typescript
interface SessionRowProps {
  session: Session
  onPause?: () => void
  onResume?: () => void
  // ... other props
}
```

**Parent integration** (wherever SessionRow/Card rendered, e.g., SessionList):
```typescript
onPause={() => sessionService.pauseSession(session.id)}
onResume={() => sessionService.resumeSession(session.id)}
```

**Menu item dispatch:**
```typescript
onClick={(e) => { e.stopPropagation(); close(); onPause?.(); }}
```

## 5. Frontend ConnectRPC Client Pattern

### Client creation (`useSessionService.ts`):
```typescript
const client = createClient(SessionService, transport);

// Unary RPC call
const response = await client.updateSession({
  id: sessionId,
  status: newStatus,
});

// Server-streaming RPC
client.watchSessions({...}, {
  onMessage: (event) => { /* handle SessionEvent */ },
  onError: (err) => { /* handle error */ },
});
```

### Request/Response handling:
- RPC methods return a Promise of the response message
- Errors thrown on non-2xx HTTP or proto parse errors
- Store dispatch on success to keep Redux state in sync
- Status updates broadcast via WatchSessions stream (real-time sync across tabs)

### Session store updates:
```typescript
// On successful mutation
dispatch(upsertSession(response.session))

// On stream event  
dispatch(updateSessionStatus({ id, status: newStatus }))
```

---

## UX Integration Plan for Hibernation

### Option 1: Hibernation as a Pause Alternative
- Show "Hibernate" button in place of "Pause" when running
- Saves scrollback + metadata before killing process
- User-visible action (no auto-hibernation in initial click)
- Resume is the same "Resume" button (works for both paused and hibernated)

### Option 2: Hibernation as a Sub-menu
- "Pause" menu item with submenu or dropdown:
  - Pause (immediate, just stop)
  - Hibernate (save checkpoint + stop)

### Option 3: Dual Actions (Pause + Hibernate)
- Show both "Pause" and "Hibernate" in menu
- Pause: stop process, keep memory footprint (branch/worktree active)
- Hibernate: save to disk, free all memory (process killed, scrollback persisted)

**Recommendation:** Option 1 (clearest UX, matches requirement to replace Pause concept)

### Status Badge Rendering
**New SessionStatus enum value needed:**
```protobuf
enum SessionStatus {
  SESSION_STATUS_HIBERNATED = 8;
}
```

**Frontend status handling additions:**
```typescript
case SessionStatus.HIBERNATED:
  return "hibernated";  // statusDot value

case SessionStatus.HIBERNATED:
  return statusHibernated;  // color class

case SessionStatus.HIBERNATED:
  return "Hibernated";  // display text
```

**Icon suggestion:** ❄️ (snowflake) or 😴 (sleeping face) to visually distinguish from Paused

---

## Key Files for Implementation

| Component | File | Key Patterns |
|---|---|---|
| Status enum | `proto/session/v1/types.proto` | Add `SESSION_STATUS_HIBERNATED = 8` |
| Go status constants | `session/instance.go` | Add `Hibernated Status = iota` |
| UpdateSession RPC | `proto/session/v1/session.proto` | Already supports arbitrary status values |
| Handler logic | `server/services/session_service.go` | Pattern: validate → mutate → broadcast |
| Frontend hook | `web-app/src/lib/hooks/useSessionService.ts` | Add `hibernateSession()` callback (similar to `pauseSession`) |
| UI component | `web-app/src/components/sessions/SessionActionsOverflow.tsx` | Add `onHibernate` prop + conditional menu item |
| Status display | `web-app/src/components/sessions/SessionCard.tsx` | Add hibernated case to `getStatusColor/Text` |
| Status styling | `web-app/src/components/sessions/SessionCard.css.ts` | Add `statusHibernated` color (light blue or gray-blue) |
| Status dot styling | `web-app/src/components/sessions/SessionRow.css.ts` | Add `[data-status="hibernated"]` CSS rule |
