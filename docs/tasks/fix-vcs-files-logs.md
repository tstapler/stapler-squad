# Feature Plan: Fix VCS Diff, Files, and Logs Pages

## Epic Overview

Five regressions and gaps across the VCS Diff, Files, and Logs tabs in the session detail view. Three bugs share a single root cause (UUID session ID mismatch in backend lookup), one bug is a tree-toggle implementation issue in the FileTree component, and one is a configuration/path mismatch in the Logs tab.

The work is organized as three stories:
1. Fix UUID session ID mismatches in VCS, Files, and Diff backend lookups
2. Fix folder drill-down in the Files tree
3. Fix session logs page returning no results

---

## Root Cause Analysis

### Bug 1 and 2: UUID ID Mismatch in VCS and Files Services

**The migration:** When sessions were given UUIDs, `InstanceToProto` in `server/adapters/instance_adapter.go` was updated to set `proto.Id = inst.GetStableID()`. `GetStableID()` returns `inst.UUID` when present, otherwise falls back to `inst.Title`. The proto `Id` field is now a UUID for any session that has one.

**The frontend:** `SessionDetail.tsx` passes `session.id` to all tabs — `VcsPanel` (via `SessionVcsProvider`), `FilesTab`, `SessionLogsTab`, and `DiffViewer`. Since `session.id` comes from the proto, it is now a UUID.

**The backend lookups that broke:**

| Service | Method | Lookup logic | Bug |
|---|---|---|---|
| `WorkspaceService` | `GetVCSStatus`, `GetWorkspaceInfo`, `GetWorkspace` | `inst.Title == id` | Title lookup — fails when id is UUID |
| `WorkspaceService` | `GetWorkspace` (used by `FileService`) | `inst.Title == id` | Same |
| `SessionService` | `GetSessionDiff` | `inst.Title == req.Msg.Id` | Title-only, does not check UUID |
| `GitHubService` | `findInstance` | `inst.Title == id` | Title-only |

**The `SessionService.findInstance`** (used for terminal, streaming, session updates) already does the right thing: it calls `reviewQueuePoller.FindInstance(id)`, which checks `inst.Title == sessionID || inst.GetStableID() == sessionID`. So terminal streaming works; the other services do not.

**Files lookup chain:** `SessionService.ListFiles` / `GetFileContent` / `SearchFiles` all delegate to `FileService`, which calls `WorkspaceService.GetWorkspace(sessionId)`. That calls `findInstance(sessionID)` which only compares `inst.Title`. So all file operations fail when passed a UUID.

**VCS lookup chain:** `useSessionVcs.ts` calls `getVCSStatus({ id: sessionId })` and `getSessionDiff({ id: sessionId })`. `GetVCSStatus` goes through `WorkspaceService.findInstance` (Title-only). `GetSessionDiff` does its own Title-only loop.

### Bug 3: FileTree Folder Drill-Down Not Working

The `FileTree.tsx` component uses `react-arborist` for the virtual tree. `react-arborist` v3 fires `onToggle` with the **node ID** (a string) when a node is toggled open/closed. The `handleToggle` callback in `FileTree` receives that ID and calls `loadDirectory(id)`.

The issue is that `buildTreeData` gives every directory node `children: []` (an empty array, not `undefined`) as its initial state, because the code in `buildTreeData` does:

```typescript
if (loaded === undefined) {
  // Directory not yet loaded — provide empty array so it's expandable.
  return { ...node, children: [] };
}
```

`react-arborist` uses the `childrenAccessor` to determine whether a node is a leaf. The accessor is:

```typescript
childrenAccessor={(node) => {
  if (!node.isDir) return null;
  return node.children ?? [];
}}
```

Returning `[]` (empty array) for unloaded directories means arborist treats them as **empty leaf nodes, not expandable folders**. When a user clicks such a node, arborist may not fire `onToggle` at all, or it fires with no visual expand. The node appears collapsed with no children to show, even after `loadDirectory` stores results in `dirContents`.

Additionally, `handleToggle` only loads the directory if `!dirContents.has(id)`. Once a failed or empty load is stored, re-clicking will not retry. This compounds the initial rendering issue.

The correct pattern is to return `undefined` (not `[]`) for directories whose children have not been loaded, so arborist knows they are expandable-but-unloaded. Only return `[]` when the directory is confirmed empty.

### Bug 4: Session Logs Tab Returns No Entries

`SessionLogsTab.tsx` passes `sessionId` (a UUID) to `getLogs({ sessionId })`. The backend `GetLogs` in `utility_service.go` calls `log.GetSessionLogFilePath(cfg, sid)`. That builds `session_<sanitized-sid>.log`. For a UUID like `3f2a1b4c-...`, the path would be `session_3f2a1b4c----.log` (dashes preserved, since `-` is allowed in the sanitizer).

The session log files are **written** using `inst.Title` as the session ID (see `review_queue_poller.go:660`: `log.LogForSession(inst.Title, ...)`). So log files are named `session_<title>.log`. When queried with a UUID, the backend opens a nonexistent file path and returns an empty result with no error.

There are two sub-problems:
1. `GetLogs` receives a UUID but log files are written with the Title as the key.
2. Session logs may not be written at all for most activity — `LogForSession` is only called from a handful of places in the poller. Most session activity goes to the global log file, not per-session files.

---

## Story Breakdown

### Story 1: Fix UUID Session ID References in Backend Services

**Goal:** All VCS and file service backend lookups must resolve instances by both UUID and Title, matching the behavior already present in `ReviewQueuePoller.FindInstance`.

**Acceptance Criteria:**
- Opening the VCS tab for any session (old Title-ID or new UUID-ID) shows git status and file changes
- Opening the Diff tab shows the unified diff
- Opening the Files tab loads the root directory
- File content can be read from any file in the tree
- File search returns results

**Files to modify:**
- `server/services/workspace_service.go`
- `server/services/session_service.go`

---

### Story 2: Fix Folder Drill-Down in Files View

**Goal:** Clicking a folder in the Files tree expands it and loads its children from the server. Clicking again collapses it.

**Acceptance Criteria:**
- Clicking any directory expands it and shows children
- Children load via the `listFiles` RPC with the directory path
- Subdirectories within expanded folders are also expandable
- "Collapse all" button collapses all open directories
- Git status badges propagate up to parent directories

**Files to modify:**
- `web-app/src/components/sessions/FileTree.tsx`

---

### Story 3: Fix Logs Tab Showing No Results

**Goal:** The Logs tab shows actual session-scoped log messages, correctly resolved by the session's UUID.

**Acceptance Criteria:**
- The Logs tab shows log entries when the session has log activity
- The "No logs recorded" message only appears when there genuinely are no logs
- Refresh and live-tail work correctly

**Files to modify:**
- `server/services/utility_service.go`
- `server/services/workspace_service.go` (helper method, reused)

---

## Atomic Tasks

### Task 1.1: Add UUID-aware instance resolution helper to WorkspaceService

**Objective:** Replace the Title-only lookup in `WorkspaceService.findInstance` with one that also checks `inst.GetStableID()`, consistent with `ReviewQueuePoller.FindInstance`.

**Files to modify:**
- `server/services/workspace_service.go`

**Implementation steps:**

1. The current `findInstance` iterates `storage.LoadInstances()` and returns where `inst.Title == id`. Change the condition to also match on `inst.GetStableID() == id`:

```go
// Before (line 52):
if inst.Title == id {

// After:
if inst.Title == id || inst.GetStableID() == id {
```

2. Update the comment on `findInstance` to document the dual-match behavior.

**Validation:**
- `go test ./server/services/ -run TestWorkspace` passes
- Manual: open VCS tab for a session; status loads without error

---

### Task 1.2: Fix GetSessionDiff to resolve by UUID

**Objective:** `GetSessionDiff` has its own inline Title-only lookup. It should use the same dual-match logic.

**Files to modify:**
- `server/services/session_service.go`

**Implementation steps:**

1. Locate the inline loop at line ~1242:

```go
for _, inst := range instances {
    if inst.Title == req.Msg.Id {
        instance = inst
        break
    }
}
```

2. Change to also match `inst.GetStableID()`:

```go
for _, inst := range instances {
    if inst.Title == req.Msg.Id || inst.GetStableID() == req.Msg.Id {
        instance = inst
        break
    }
}
```

3. Note: `GetSessionDiff` calls `s.loadInstancesWithWiring()` (storage-backed). Ideally, this should use the live in-memory poller like `GetSession` does, but that is a separate improvement. The minimal fix is the dual-match.

**Validation:**
- `go test ./server/services/ -run TestGetSessionDiff` passes (add test if missing)
- Manual: open Diff tab for a session with uncommitted changes; diff renders

---

### Task 1.3: Fix GitHubService.findInstance to resolve by UUID

**Objective:** `GitHubService.findInstance` also uses Title-only. Fix for completeness and future-proofing.

**Files to modify:**
- `server/services/github_service.go`

**Implementation steps:**

Change line 36:
```go
// Before:
if inst.Title == id {

// After:
if inst.Title == id || inst.GetStableID() == id {
```

**Validation:**
- `go test ./server/services/ -run TestGitHub` passes
- No regression on PR info for sessions that have PRs

---

### Task 2.1: Fix FileTree lazy loading — return undefined for unloaded directories

**Objective:** Directories that have not been loaded must return `undefined` from `childrenAccessor`, not `[]`, so `react-arborist` knows they are expandable. Returning `[]` marks them as empty leaves.

**Files to modify:**
- `web-app/src/components/sessions/FileTree.tsx`

**Implementation steps:**

1. In `buildTreeData`, the branch that handles unloaded directories currently returns `children: []`:

```typescript
// Line 89-91 — current code:
if (loaded === undefined) {
  // Directory not yet loaded — provide empty array so it's expandable.
  return { ...node, children: [] };
}
```

Change to return `undefined`, which signals react-arborist that children exist but are not yet loaded:

```typescript
if (loaded === undefined) {
  // undefined signals react-arborist: expandable but children not yet fetched
  return { ...node, children: undefined };
}
```

2. In the `childrenAccessor` prop on `<Tree>`, the current code is:

```typescript
childrenAccessor={(node) => {
  if (!node.isDir) return null;
  return node.children ?? [];
}}
```

`null` tells arborist it is a leaf. `[]` tells arborist it has zero children (collapsed leaf). `undefined` is not handled well by all versions. The safest approach is to return `null` explicitly for unloaded dirs to let arborist show the expand arrow, then trigger loading on toggle. However, different arborist versions handle this differently.

The more reliable fix is to keep the pattern but ensure `handleToggle` is actually called. Confirm whether arborist fires `onToggle` for nodes with `children: []` vs `children: undefined` by examining the `handleToggle` flow. If `children: []` prevents the toggle, switch to `undefined`:

```typescript
childrenAccessor={(node) => {
  if (!node.isDir) return null;
  // null returned for dirs with no loaded children lets arborist show expand arrow
  if (node.children === undefined) return null;
  return node.children;
}}
```

3. Update `TreeNode.children` type documentation to clarify: `undefined = not loaded (expandable)`, `[] = loaded and empty`.

**Validation:**
- Click a directory in the Files tab; a loading spinner shows and children appear
- Subdirectories are also expandable
- The root collapse button collapses all open dirs

---

### Task 2.2: Fix handleToggle to correctly load subdirectory paths

**Objective:** Verify the path passed to `loadDirectory` matches what the backend expects. The `FileTree` stores node IDs as relative paths from the workspace root (e.g., `src/components`). The backend `ListFiles` accepts `path` as a relative path. These should already match, but the retry logic is worth hardening.

**Files to modify:**
- `web-app/src/components/sessions/FileTree.tsx`

**Implementation steps:**

1. In `handleToggle`, the current guard `if (node?.isDir && !dirContents.has(id))` prevents retry after any load (including failed loads). A failed load sets `errorPaths`, so the node cannot be retried. Add a check: if the path is in `errorPaths`, allow re-toggle to clear the error and retry:

```typescript
const handleToggle = useCallback(
  (id: string) => {
    if (searchResults !== null) return;
    const allNodes = buildTreeData(rootNodes, dirContents);
    const node = findNode(allNodes, id);
    if (node?.isDir) {
      // Load if not yet loaded, or if previous load errored (allow retry)
      if (!dirContents.has(id) || errorPaths.has(id)) {
        loadDirectory(id);
      }
    }
  },
  [rootNodes, dirContents, loadDirectory, searchResults, errorPaths]
);
```

2. Confirm `node.id` for subdirectories is set to the full relative path from the root (e.g., `src/components/sessions`), not just the name. In `fileNodeToTreeNode`:

```typescript
id: fn.path || fn.name,
```

`fn.path` comes from the backend as `filepath.ToSlash(relPath)` — the relative path from the workspace root. This is correct. The `loadDirectory` call passes this path directly to `fetchDirectoryFiles`.

**Validation:**
- Expand `src`, then expand `src/components` within it — children load correctly
- Expanding a directory that previously errored shows a retry attempt

---

### Task 3.1: Fix GetLogs to resolve session log path by Title when given UUID

**Objective:** When `GetLogs` is called with a UUID, look up the session's Title and use that to find the log file, since logs are written keyed by Title.

**Files to modify:**
- `server/services/utility_service.go`
- `server/services/workspace_service.go`

**Implementation steps:**

1. Add a `ResolveTitleByID` method to `WorkspaceService` (or a shared helper) that accepts either a UUID or Title and returns the session's Title:

```go
// ResolveTitleByID looks up an instance by UUID or Title and returns its Title.
// Returns empty string if not found.
func (ws *WorkspaceService) ResolveTitleByID(id string) string {
    instances, err := ws.storage.LoadInstances()
    if err != nil {
        return ""
    }
    for _, inst := range instances {
        if inst.Title == id || inst.GetStableID() == id {
            return inst.Title
        }
    }
    return ""
}
```

2. Inject `WorkspaceService` (or a `TitleResolver` interface) into `UtilityService`:

```go
type TitleResolver interface {
    ResolveTitleByID(id string) string
}

type UtilityService struct {
    approvalStore     *ApprovalStore
    reviewQueuePoller *session.ReviewQueuePoller
    titleResolver     TitleResolver  // optional; nil = no resolution
}
```

3. In `GetLogs`, when `sid` is non-empty, resolve it to a Title before building the log path:

```go
if sid := req.Msg.GetSessionId(); sid != "" {
    // Resolve UUID to Title for log file naming (logs are written with Title as key)
    resolvedID := sid
    if us.titleResolver != nil {
        if title := us.titleResolver.ResolveTitleByID(sid); title != "" {
            resolvedID = title
        }
    }
    logFilePath, err = log.GetSessionLogFilePath(cfg, resolvedID)
}
```

4. Wire `WorkspaceService` as the `TitleResolver` in `server/server.go` (or wherever `UtilityService` is constructed).

**Alternative simpler approach:** Since `UtilityService` already has access to `reviewQueuePoller`, use it directly instead of adding a new interface:

```go
if sid := req.Msg.GetSessionId(); sid != "" {
    resolvedID := sid
    if us.reviewQueuePoller != nil {
        if inst := us.reviewQueuePoller.FindInstance(sid); inst != nil {
            resolvedID = inst.Title  // log files always written with Title
        }
    }
    logFilePath, err = log.GetSessionLogFilePath(cfg, resolvedID)
}
```

The `reviewQueuePoller` is already set via `SetReviewQueuePoller`. This approach adds zero new dependencies and is the minimal change.

**Validation:**
- Open Logs tab for a session that has had git activity; log entries appear
- `go test ./server/services/ -run TestGetLogs` passes with UUID input

---

### Task 3.2 (Optional Improvement): Write session logs using GetStableID

**Objective:** If a future goal is to query logs by UUID directly (no Title resolution step), log files should be written using `GetStableID()` instead of `Title`. This is a follow-on improvement, not required for the current fix.

**Note:** This would require migrating existing log files, which is out of scope for this plan. Document as a known technical debt item.

---

## Dependency Visualization

```
Task 1.1 (WorkspaceService.findInstance UUID fix)
  |
  +-- Task 1.3 (GitHubService.findInstance) [independent, same pattern]
  |
  +-- Task 1.2 (GetSessionDiff UUID fix) [independent, same pattern]

Task 2.1 (FileTree children: undefined)
  |
  +-- Task 2.2 (handleToggle retry logic) [depends on 2.1 for toggle to fire]

Task 3.1 (GetLogs UUID resolution)
  -- independent, uses existing reviewQueuePoller
```

Tasks 1.x, 2.x, and 3.x are completely independent of each other and can be worked in parallel.

---

## Known Issues and Potential Bugs

### Bug 1: GetSessionDiff Uses loadInstancesWithWiring — Potential Session Restart Side Effect [SEVERITY: Medium]

**Description:** `GetSessionDiff` calls `s.loadInstancesWithWiring()` to find the instance. This method loads from storage and may call `Start()` on sessions that are not in the Paused state. For the diff fix, the correct approach is to use the live in-memory poller (`s.findInstance`), just as `GetSession` does. The current fix (Task 1.2) patches the matching condition but does not address the underlying `loadInstancesWithWiring` call.

**Mitigation:**
- File a follow-up task to refactor `GetSessionDiff` to use `s.findInstance` (poller-backed)
- The `loadInstancesWithWiring` issue is pre-existing and not introduced by the UUID fix

**Files Likely Affected:**
- `server/services/session_service.go` (~line 1235)

---

### Bug 2: Session Logs May Be Sparse Even After Fix [SEVERITY: Low]

**Description:** Even with the UUID-to-Title resolution fixed, the Logs tab may show very few entries. `LogForSession(inst.Title, ...)` is only called from `review_queue_poller.go` in one place (line 660, a git check warning). Most session activity is written to the global log file. The Logs tab UI implies per-session logs, but the actual logging coverage is minimal.

**Mitigation:**
- After Task 3.1, the Logs tab will correctly show what is there
- A separate investigation should identify which events should be written to per-session logs and expand coverage
- Consider adding log entries in `instance.go` at key lifecycle events (start, stop, prompt sent, response received)

---

### Bug 3: react-arborist onToggle May Not Fire for Empty Children Array [SEVERITY: High, for Task 2.1]

**Description:** The exact behavior of `onToggle` when `childrenAccessor` returns `[]` depends on the react-arborist version (^3.4.3). In some versions, nodes with `children: []` are treated as collapsed leaves and no toggle fires. In others, the toggle fires but produces no visible change.

**Mitigation:**
- Test Task 2.1 change with the actual installed version before shipping
- If `children: undefined` causes arborist to not render the expand arrow at all, the correct fix is to use `null` from `childrenAccessor` for unloaded dirs (which all arborist versions treat as "has children, not loaded"), and load on `onActivate` instead of `onToggle`
- Add an explicit test in the browser for directory expand behavior

---

### Bug 4: WorkspaceService.findInstance Calls storage.LoadInstances — Performance Cost [SEVERITY: Low]

**Description:** `WorkspaceService.findInstance` calls `storage.LoadInstances()` on every VCS status poll (every 10 seconds). This reads and parses the sessions JSON file each time. As the session count grows, this becomes noticeable. `SessionService.findInstance` correctly uses the in-memory poller.

**Mitigation:**
- For now, the fix in Task 1.1 maintains the existing storage-backed pattern
- A follow-up task should inject the live poller into `WorkspaceService` and use it instead of `storage.LoadInstances()`

---

### Bug 5: File Content Cache Uses UUID as Key — Will Become Stale on Session Rename [SEVERITY: Low]

**Description:** `useGetFileContent` in `useFileService.ts` caches results with key `${sessionId}:${filePath}`. Since `sessionId` is now the UUID (stable), this is actually better than using Title. No action needed.

---

## Files Modified Summary

| File | Task | Change |
|---|---|---|
| `server/services/workspace_service.go` | 1.1, 3.1 | findInstance: add UUID match; add TitleResolver if taking that approach |
| `server/services/session_service.go` | 1.2 | GetSessionDiff inline loop: add UUID match |
| `server/services/github_service.go` | 1.3 | findInstance: add UUID match |
| `server/services/utility_service.go` | 3.1 | GetLogs: resolve UUID to Title via poller |
| `web-app/src/components/sessions/FileTree.tsx` | 2.1, 2.2 | buildTreeData: return undefined; handleToggle: retry on error |

No proto changes required. No schema migrations required. No new dependencies required.

---

## Testing Validation

### Manual Test Scenarios

**VCS Diff Tab:**
1. Open any session detail; click Diff tab
2. Expected: unified diff renders (or "No changes" if clean)
3. Failure before fix: "session not found" error or loading spinner never resolves

**VCS Panel Tab:**
1. Open any session detail; click VCS tab
2. Expected: branch name, staged/unstaged file lists render
3. Failure before fix: "No VCS information available" with underlying 404

**Files Tab - Root Load:**
1. Open any session detail; click Files tab
2. Expected: root directory files and folders list
3. Failure before fix: "Failed to load files" error

**Files Tab - Folder Drill-Down:**
1. Open Files tab; click a folder with children
2. Expected: folder expands, children load, nested folders also expandable
3. Failure before fix: folder does not expand on click

**Files Tab - File Search:**
1. Open Files tab; type 2+ characters in search box
2. Expected: matching files highlighted in tree with count displayed
3. Failure before fix: search errors or no results (depends on session ID resolution)

**Logs Tab:**
1. Open any session detail; click Logs tab
2. Expected: log entries appear, or "No logs recorded" only if genuinely empty
3. Failure before fix: always shows "No logs recorded" regardless of activity

### Automated Test Coverage

- `server/services/workspace_service_test.go` — add test for `findInstance` with UUID input
- `server/services/session_service_test.go` — add test for `GetSessionDiff` with UUID input
- `server/services/utility_service.go` — add test for `GetLogs` with UUID input resolving to Title
- `web-app/src/components/sessions/__tests__/` — add test for FileTree toggle/expand behavior with unloaded dir nodes
