# Branch Resume in Unfinished Work Tab — Architecture Research

**Date**: 2026-05-31
**Feature**: Surface dormant local git branches (ahead of main, no active session) as rich cards in the Unfinished Work tab with a Resume button that pre-fills the omnibar.

---

## 1. Where Should Branch Enumeration Live?

**Decision: Extend the existing `Scanner` in `session/unfinished/` with a new `BranchScanner` helper.**

### Rationale

The `Scanner` is the central coordinator for all unfinished-work background scans. It:
- Manages a worker pool (4 goroutines) consuming tasks from a scan queue
- Maintains a `resultStore` (sync.Map) keyed by `repoPath|branch`
- Publishes events via `EventBus` on scan result changes
- Respects dismiss/snooze state via `StateStore`

**Why NOT add directly to the handler**: The RPC handler (`UnfinishedWorkService.ListUnfinishedWork` and `WatchUnfinishedWork`) is thin—it retrieves pre-computed results from the scanner and formats them for proto. Adding branch enumeration here would duplicate the scan logic and break the separation between compute (Scanner) and transport (service handler).

**Why NOT a standalone `BranchScanner`**: A separate component would need its own event bus, state store, and coordination logic. Reusing the existing Scanner framework (worker pool, event publishing, result caching) keeps code DRY and ensures consistent lifecycle management.

### Implementation Approach

1. Add a `BranchScanResult` struct (similar to `ScanResult`) to `session/unfinished/types.go`:
   ```go
   type BranchScanResult struct {
       RepoPath    string
       BranchName  string  // local branch name
       IsLocal     bool    // vs. remote-only
       IsAheadOnly bool    // no active session targeting this branch
       DefaultBranch string
       AheadCount  int
       BehindCount int
       LastModified time.Time
       // ...
   }
   ```

2. Add a `ScanLocalBranches(repoPath)` method to `Scanner`:
   - Queries `git branch -v --no-color` to list local branches
   - For each branch, calls `AheadBehind()` against the default branch
   - Filters: keep only branches with `AheadCount > 0` and `BehindCount == 0` (ahead-only)
   - Cross-references against active sessions to exclude branches with open sessions
   - Returns `[]BranchScanResult`

3. Invoke this during the existing `scanRepo()` worker:
   - After scanning worktrees, call `s.ScanLocalBranches(repoPath)`
   - Merge results into a separate `branchResultStore` (sync.Map, keyed `repoPath|branchName`)
   - Publish new event type: `EventDormantBranchUpdated` / `EventDormantBranchRemoved`

4. Expose `GetAllBranches()` and `GetBranchesByRepo(repoPath)` methods (similar to `GetAllResults()`)

---

## 2. How Does the Existing Unfinished Work Streaming RPC Work?

**Architecture: Server-side stream with EventBus pub/sub.**

### Flow

1. **Initial Snapshot** (`WatchUnfinishedWork`):
   - Client sends `WatchUnfinishedWorkRequest` (empty)
   - Server calls `scanner.GetAllResults()` to fetch all current unfinished worktrees
   - Server sends each worktree as a `worktree_updated` event immediately
   
2. **Real-time Updates**:
   - Server subscribes to `eventBus` on channel: `eventCh, subID := s.eventBus.Subscribe(ctx)`
   - Listens for three event types:
     - `EventUnfinishedWorkUpdated` → convert to `worktree_updated` event
     - `EventUnfinishedWorkRemoved` → send `worktree_removed` event
     - `EventUnfinishedScanCompleted` → send `scan_completed` event
   - Forwards each event to the connected client stream until disconnect

3. **Disconnect / Reconnect**:
   - If client connection drops, the stream ends
   - Client libraries (like `useUnfinishedWork` hook) auto-reconnect after 3 seconds

### Why Event-Based + Stream?

- **Efficiency**: Only changed items are sent; unchanged worktrees are not re-broadcast
- **Real-time**: Clients see updates within milliseconds of a scan completing
- **Memory**: EventBus uses a buffered channel (no unbounded queues)
- **Simplicity**: No polling logic; server controls update cadence

---

## 3. What Proto Changes Are Needed?

**Decision: Extend the existing `UnfinishedWorkEvent` oneof with two new payloads for branches.**

### Proto Changes (in `proto/session/v1/unfinished.proto`)

1. **Add new message** `DormantBranch`:
   ```protobuf
   message DormantBranch {
     string repo_path      = 1;
     string branch_name    = 2;  // local branch name
     string repo_name      = 3;  // e.g., "my-repo"
     string display_path   = 4;  // repo path with ~ substitution
     
     int32 commits_ahead   = 5;  // ahead of default branch
     int32 commits_behind  = 6;  // (usually 0 for dormant filter)
     string default_branch = 7;
     
     google.protobuf.Timestamp last_modified = 8;  // mtime of .git/refs/heads/<branch>
     google.protobuf.Timestamp scan_time     = 9;
   }
   ```

2. **Extend `UnfinishedWorkEvent` oneof**:
   ```protobuf
   message UnfinishedWorkEvent {
     oneof payload {
       UnfinishedWorktree worktree_updated = 1;
       UnfinishedWorktree worktree_removed = 2;
       ScanCompleted      scan_completed   = 3;
       DormantBranch      branch_updated   = 4;  // NEW
       DormantBranch      branch_removed   = 5;  // NEW
     }
   }
   ```

3. **Update response messages**:
   - `ListUnfinishedWorkResponse`: Add `repeated DormantBranch branches = 3`
   - Or: Create a new union response message if design prefers separation

### Rationale

- **Unified event stream**: Branches and worktrees can be streamed in a single RPC, reducing client complexity
- **Minimal proto churn**: Reuses existing patterns (oneof payloads, timestamp fields)
- **Separation of concerns**: Each event type has its own message, so the frontend can filter/group independently

### Alternative Considered (Rejected)

Creating a separate `DormantBranchService` with its own streaming RPC would require:
- Duplicate EventBus infrastructure
- Two separate streams on the frontend (two watchers)
- Duplicate dismiss/snooze state management

---

## 4. How Does the Frontend Currently Render the Unfinished Work Tab?

**Architecture: Live-streaming React component with real-time map synchronization.**

### Data Flow

1. **Hook** (`useUnfinishedWork` in `web-app/src/lib/hooks/useUnfinishedWork.ts`):
   - Subscribes to `WatchUnfinishedWork` RPC stream
   - Maintains a local `Map<key, UnfinishedWorktree>` (key = `${repoPath}|${branch}`)
   - On `worktree_updated`: inserts/updates map entry
   - On `worktree_removed`: deletes map entry
   - On `scan_completed`: updates `isScanning` flag and `lastScanTime`

2. **Component** (`UnfinishedTab.tsx`):
   - Calls `useUnfinishedWork()` → gets `{ worktrees, lastScanTime, isScanning, triggerScan }`
   - Converts map to sorted array (by `lastModified` descending)
   - Groups by `repoName` → renders `UnfinishedRepoGroup` components
   - Each group renders individual `UnfinishedItem` cards

3. **Card** (`UnfinishedItem.tsx`):
   - Displays branch name, path, status chips (uncommitted, ahead, behind)
   - Expandable to show `UnfinishedItemDetail` (diff stats, commit messages, action buttons)
   - Action buttons: "Open Session", "View Diff", "Commit & Push", "Summarize"

### Where Branch Cards Will Be Inserted

**Option A: Separate Section**
```
Unfinished Work
├─ Worktrees
│  └─ [existing repo groups with worktree cards]
└─ Dormant Branches
   └─ [new repo groups with branch cards]
```

**Option B: Mixed in Repo Groups** (simpler)
```
Unfinished Work
└─ Repo 1
   ├─ [worktree: main branch, feature-x with uncommitted]
   ├─ [worktree: feature-auth, 3 commits ahead]
   └─ [dormant branch: feature-old-api, 8 commits ahead, no session]
```

**Recommendation**: Option B (mixed) — simplifies the UI model and treats branches and worktrees as related items within the same repo context. The frontend can visually distinguish them with a badge ("No Session" vs. "Session: ID-123") or icon.

### Implementation

1. Extend `useUnfinishedWork` to track branches:
   ```ts
   export function useUnfinishedWork(): UseUnfinishedWorkReturn {
     const [worktreeMap, setWorktreeMap] = useState<Map<string, UnfinishedWorktree>>(new Map());
     const [branchMap, setBranchMap] = useState<Map<string, DormantBranch>>(new Map());  // NEW
     // ... handle branch_updated / branch_removed events
     return { worktrees, branches, lastScanTime, isScanning, triggerScan };
   }
   ```

2. In `UnfinishedTab.tsx`, merge worktrees and branches when grouping by repo:
   ```ts
   const allItems = [...worktrees, ...branches.map(b => ({ ...b, _isGitBranch: true }))];
   const groups = groupByRepoName(allItems);
   ```

3. In `UnfinishedRepoGroup.tsx`, render branch items differently:
   - Show "Dormant" or "No Active Session" label
   - Include a "Resume" button (instead of "Open Session")

---

## 5. How Does the Omnibar Get Its Initial Path Pre-Filled?

**Architecture: Context + URL state propagation.**

### Pre-fill Flow

1. **OmnibarContext** (`web-app/src/lib/contexts/OmnibarContext.tsx`):
   - Exposes `openOmnibar(initialInput?: string)` function
   - Caller can pass a path, branch spec, or URL
   - Context stores this in `[initialInput, setInitialInput]` state
   - Passes to `<Omnibar initialInput={initialInput} />`

2. **Omnibar Component** (`web-app/src/components/sessions/Omnibar.tsx`):
   - On mount, if `initialInput` is provided:
     - Sets `setInput(initialInput)` (displayed in text field)
     - Runs `detect(initialInput)` immediately
     - Auto-populates form fields based on detected type (path, branch, etc.)
   - Example: `initialInput = "/path/to/repo@feature-x"` → detects as `PathWithBranch`

3. **Example Usage** (from `page.tsx`):
   ```ts
   const { openOmnibar } = useOmnibar();
   openOmnibar(`${worktreePath}@${worktreeBranch}`);
   ```

### Detection System

The omnibar uses `detect()` function (in `web-app/src/lib/omnibar/`) with a `DetectorRegistry`:
- **LocalPathDetector**: `/path/to/repo`
- **PathWithBranchDetector**: `/path/to/repo@branch-name`
- **GitHubPRDetector**: `https://github.com/owner/repo/pull/123`
- **SessionSearchDetector**: fallback (search by session name)

When input is detected as a path or path@branch:
- Form auto-fills `workingDir` = path
- If branch is detected, auto-fills `branch` = branch name
- Form stays in "discovery" mode until user submits

---

## 6. What Is the Data Flow for "User Clicks Resume"?

**Architecture: useOmnibar context hook + onCreateSession callback chain.**

### Sequence Diagram

```
1. User clicks "Resume" button on branch card
   ↓
2. Component calls `useOmnibar().openOmnibar(repoPath + "@" + branchName)`
   ↓
3. OmnibarContext updates state: initialInput = "repoPath@branchName", isOpen = true
   ↓
4. Omnibar mounts/opens, receives initialInput prop
   ↓
5. Omnibar.useEffect detects input as PathWithBranch
   ↓
6. Form auto-fills: workingDir = repoPath, branch = branchName
   ↓
7. User clicks "Create Session" (or presses Enter with auto-submit)
   ↓
8. onCreateSession(data) called with:
   {
     path: repoPath,
     branch: branchName,
     sessionType: "new_worktree" (or "directory"),
     title: branchName (auto-filled from branch)
   }
   ↓
9. OmnibarContext.handleCreateSession → createSession() RPC
   ↓
10. Backend creates session, returns session ID
   ↓
11. Frontend navigates: router.push(`/?session=${sessionId}`)
```

### Component Touchpoints

1. **UnfinishedItem / UnfinishedItemDetail**:
   - Add a "Resume" button
   - Click handler: `useOmnibar().openOmnibar(`${worktree.worktreePath}@${branchName}`)`
   - No async logic needed; delegate to context

2. **OmnibarContext.tsx**:
   - Already has `openOmnibar()` method
   - Already has `handleCreateSession()` callback
   - No changes needed (open/close/navigation already implemented)

3. **Omnibar.tsx**:
   - Already supports `initialInput` prop
   - Already runs detection on mount
   - Already auto-fills form fields from detected values
   - No changes needed (detection + auto-fill already implemented)

### Key Insight: Reuse Existing Infrastructure

The omnibar was designed to accept a `PathWithBranch` input (e.g., `/path@branch`). The branch resume feature simply leverages this:
- Pass `repoPath + "@" + branchName` to `openOmnibar()`
- The existing `PathWithBranchDetector` parses it
- Form auto-fills as if user typed it manually
- Submit creates a session normally

**No additional callbacks or state needed** — the Resume button just calls `openOmnibar()` with a formatted string.

---

## Summary: Integration Points

| Component | Changes | File |
|---|---|---|
| Backend: Scanner | Add `BranchScanResult` type; add `ScanLocalBranches()` method; add `branchResultStore` | `session/unfinished/scanner.go` + new `types.go` |
| Backend: Service | Handle `branch_updated` / `branch_removed` events in `convertUnfinishedEvent()`; return branches in `ListUnfinishedWork` | `server/services/unfinished_work_service.go` |
| Proto | Add `DormantBranch` message; extend `UnfinishedWorkEvent` oneof | `proto/session/v1/unfinished.proto` |
| Frontend: Hook | Track `branchMap` in addition to `worktreeMap`; handle new event cases | `web-app/src/lib/hooks/useUnfinishedWork.ts` |
| Frontend: Tab | Merge branches into repo groups; render branch cards with "Resume" button | `web-app/src/app/unfinished/UnfinishedTab.tsx` + `UnfinishedRepoGroup.tsx` |
| Frontend: Card | Add branch card component (similar to `UnfinishedItem`); wire Resume button to `useOmnibar().openOmnibar()` | `web-app/src/components/unfinished/` (new or extended) |
| Frontend: Context | No changes (already supports `openOmnibar(initialInput)`) | `web-app/src/lib/contexts/OmnibarContext.tsx` |
| Frontend: Omnibar | No changes (already detects PathWithBranch and auto-fills) | `web-app/src/components/sessions/Omnibar.tsx` |

---

## Testing Strategy

1. **Backend Unit Tests**:
   - `ScanLocalBranches()` returns correct branches with ahead/behind counts
   - Branches with active sessions are filtered out
   - Events are published correctly

2. **Integration Tests**:
   - Full flow: scanner detects dormant branch → event published → service sends to client → frontend receives and displays

3. **Frontend Unit Tests**:
   - `useUnfinishedWork` correctly updates `branchMap` on events
   - Branch cards render with correct data and button state

4. **E2E Tests**:
   - User clicks Resume on dormant branch → omnibar opens with pre-filled path@branch → session is created
