# Stack Research: Branch-Resume Unfinished Tab Feature

## Overview
This document answers key architectural questions for surfacing dormant local git branches (not checked out as worktrees) in the Unfinished Work tab with a Resume button that pre-fills the omnibar.

---

## Q1: Scanner Coverage — Local Branches vs. Worktrees

### Current State
**The scanner only walks checked-out worktrees**, not all local branches.

- **Where**: `session/unfinished/scanner.go:scanRepo()` (lines 302-337)
- **How**: Calls `VCSReader.ListWorktrees(repoPath)` which uses `git worktree list --porcelain`
- **Filters**: Skips bare, detached, prunable worktrees and branches with empty names (lines 322-327)

### ScanResult Structure
Located in `session/unfinished/scanner.go` (lines 34-61):
```go
type ScanResult struct {
  RepoPath     string        // repo root
  Branch       string        // branch name
  WorktreePath string        // worktree directory (required)
  ...
  AheadCount   int          // commits ahead of default branch
  ...
}
```

**Key limitation**: `WorktreePath` is required and populated from worktree info. A dormant branch with no worktree would need either:
1. A synthetic/placeholder `WorktreePath` (e.g., `repoPath` or a special marker), OR
2. A new field to indicate "branch without worktree"

### What Extension Requires
To include branches not checked out as worktrees:
- Extend the `VCSReader` interface with a new method: `ListAllLocalBranches(repoPath string) ([]string, error)` 
  - Would use `git branch --list` to enumerate all local branches
  - Call this in `scanRepo()` after/alongside the worktree scan
  - For each local branch not covered by an existing worktree result, create a synthetic `ScanResult`
- Decide on `WorktreePath` handling:
  - Option A: Set `WorktreePath = repoPath` (repo root as fallback)
  - Option B: Add a `bool hasWorktree` flag to `ScanResult` and handle in proto/frontend

---

## Q2: RPC/Proto Path — Scanner to Frontend

### Current Data Flow

1. **Scanner** (`session/unfinished/scanner.go`):
   - Scans repos in background, publishes results via `eventBus`
   - Results stored in `sync.Map` keyed by `repoPath|branch`

2. **Event Bus** (`server/events/event_bus.go`):
   - Scanner publishes `UnfinishedWorkUpdated` events
   - Service subscribes and routes to frontend

3. **UnfinishedWorkService** (`server/services/unfinished_work_service.go`):
   - `ListUnfinishedWork()` (lines 74-89): Returns snapshot of all results
   - `WatchUnfinishedWork()` (lines 93-135): Streams real-time updates
   - Converts `unfinished.ScanResult` → `sessionv1.UnfinishedWorktree` proto via `scanResultToProto()` (lines 507-550)

4. **Proto** (`proto/session/v1/unfinished.proto`):
   - Service methods return `UnfinishedWorktree` messages
   - `ListUnfinishedWorkResponse` contains `repeated UnfinishedWorktree`

5. **Frontend Hook** (`web-app/src/lib/hooks/useUnfinishedWork.ts`):
   - Calls `ListUnfinishedWork()` and watches for updates
   - Passes worktrees array to `UnfinishedTab`

### No Changes Needed to RPC Path
The existing pipeline handles arbitrary `ScanResult` objects. Adding branch-only results requires no RPC/proto changes—the proto already carries `worktree_path` which can be set to `repo_path` for branches without worktrees.

---

## Q3: ScanResult Structure — Representing Branches Without Worktrees

### Current `UnfinishedWorktree` Proto (`proto/session/v1/types.proto`)

```protobuf
message UnfinishedWorktree {
  string repo_path       = 1;  // repo root
  string branch          = 2;  // branch name
  string worktree_path   = 3;  // ← Currently assumes a checked-out worktree
  int32 commits_ahead    = 7;
  ...
}
```

### Extension Strategy

**Option A: Reuse `worktree_path` as fallback**
- For branches with no worktree: `worktree_path = repo_path`
- Frontend/scanner can detect via `worktree_path == repo_path && branch != defaultBranch`
- **Pros**: No proto changes; minimal backend logic
- **Cons**: Slightly misleading field semantics

**Option B: Add optional flag** (recommended for clarity)
- Add `bool has_worktree = 21;` to proto
- Scanner sets `has_worktree = false` for branch-only results
- Frontend can hide "Open Session" and show "Resume in Omnibar" instead
- **Pros**: Explicit, easy to extend later
- **Cons**: Proto change, but backward-compatible (proto3 defaults to false)

### Backend Changes Required
1. Extend `VCSReader` with `ListAllLocalBranches(repoPath string) ([]string, error)`
   - Implement in each reader (`git_vcs_reader.go`, `gogit_vcs_reader.go`, `jj_vcs_reader.go`)
2. In `Scanner.scanRepo()`, after processing worktrees, enumerate branches and create synthetic `ScanResult` entries
3. Set `Status = ScanResultStatusOK` only if branch has ahead/behind/uncommitted data (or create based on fresh git query)

### Checking Ahead/Behind for Branch Without Worktree
- Can't run `git status` in a non-existent worktree
- Fall back to remote branch comparison: `git rev-list --left-right --count HEAD...origin/<branch>` (if origin tracking exists)
- If no tracking: `AheadCount = 0, BehindCount = 0` (branch exists locally but no compare data)

---

## Q4: Omnibar Pre-Fill API

### How It Works
Location: `web-app/src/components/sessions/Omnibar.tsx`

1. **Props**: `Omnibar` accepts `initialInput?: string` (line 40)
2. **Effect** (lines 450-455):
   ```typescript
   useEffect(() => {
     if (isOpen && initialInput) {
       setInput(initialInput);
     }
   }, [isOpen, initialInput]);
   ```
3. **Route**: Called from `web-app/src/app/page.tsx` (lines 204-207):
   ```typescript
   } else if (worktreePath) {
     openOmnibar(worktreeBranch ? `${worktreePath}@${worktreeBranch}` : worktreePath);
   }
   ```

### Resume Button Integration
In `UnfinishedItemDetail.tsx`, replace/augment the "Open Session" button:
- Instead of `router.push(routes.newSessionFromWorktree(...))`, call a callback prop `onResume(repoPath, branch)`
- Parent (`UnfinishedTab`) passes `onResume` that calls `openOmnibar(...)` with pre-filled path
- Format: `${worktreePath}@${branch}` (e.g., `/home/user/repo@feature-x`)

### No API Changes Needed
The omnibar already handles `initialInput` correctly. Detection logic will interpret `/path@branch` as a `PathWithBranch` type and auto-fill form fields.

---

## Q5: Existing Card Component — UnfinishedItem Props

Location: `web-app/src/components/unfinished/UnfinishedItem.tsx`

### Current Props (lines 8-14)
```typescript
interface UnfinishedItemProps {
  worktree: UnfinishedWorktree;     // proto message
  isExpanded: boolean;
  onToggleExpand: () => void;
  onDismiss: (repoPath: string, branch: string) => void;
  onSnooze: (repoPath: string, branch: string) => void;
}
```

### What's Inside the Card
- Header: branch name, path (line 66-67)
- Status chips: "Uncommitted", "↑N" (ahead), "↓N" (behind) (lines 81-89)
- Action buttons: Dismiss, Snooze (lines 93-110)
- Expanded detail: `UnfinishedItemDetail` component (lines 114-116)

### UnfinishedItemDetail Actions (lines 126-169)
Current buttons:
- "Open Session" / "Reattach Session" — opens existing session or navigates to creation
- "View Diff"
- "Commit & Push"
- "Summarize" (AI summary)

### Adding Resume for Branch-Only Results
**Option 1**: Add a new button in `UnfinishedItemDetail`
- Conditional: Only show "Resume" if `has_worktree == false`
- Callback: `onResume(repoPath, branch)` → opens omnibar pre-filled with `${repoPath}@${branch}`

**Option 2**: Modify "Open Session" behavior
- If `has_worktree == false`: Label = "Resume", action = omnibar pre-fill
- If `has_worktree == true` && no active sessions: Label = "Open Session", action = omnibar pre-fill
- If sessions exist: Label = "Reattach Session", action = session picker

**Recommendation**: Option 1 is clearer. Add `onResume` prop to both `UnfinishedItem` and `UnfinishedItemDetail`.

---

## Summary: Implementation Roadmap

### Backend (Go)

1. **Extend VCSReader interface** (`session/unfinished/vcsreader.go`)
   - Add `ListAllLocalBranches(repoPath string) ([]string, error)`

2. **Implement in all readers**
   - `git_vcs_reader.go`: `git branch --list`
   - `gogit_vcs_reader.go`: Use go-git branch API
   - `jj_vcs_reader.go`: Use jj branch API

3. **Update Scanner.scanRepo()** (`session/unfinished/scanner.go`)
   - After worktree scan, call `ListAllLocalBranches()`
   - For each branch not in worktrees set, create synthetic `ScanResult`
   - Set `WorktreePath = repoPath` (or add proto flag in Option B)

4. **Optional Proto enhancement**
   - Add `bool has_worktree = 21;` to `UnfinishedWorktree` for clarity

### Frontend (React/TypeScript)

1. **Update UnfinishedItemDetail** (`web-app/src/components/unfinished/UnfinishedItemDetail.tsx`)
   - Add `onResume?: (repoPath: string, branch: string) => void` prop
   - Add conditional "Resume" button (show when `has_worktree == false`)

2. **Update UnfinishedItem** (`web-app/src/components/unfinished/UnfinishedItem.tsx`)
   - Pass `onResume` to `UnfinishedItemDetail`

3. **Update UnfinishedTab** (`web-app/src/app/unfinished/UnfinishedTab.tsx`)
   - Import `openOmnibar` from layout context
   - Implement `handleResume(repoPath, branch)` → `openOmnibar(\`${repoPath}@${branch}\`)`
   - Pass to `UnfinishedRepoGroup`/`UnfinishedItem`

4. **Omnibar already supports** the pre-fill via `initialInput` prop—no changes needed there

---

## Key Findings

1. **Scanner scope**: Currently worktree-only; needs extension to enumerate local branches without worktrees
2. **RPC pipeline**: Already capable of handling new `ScanResult` types; no proto changes required (but optional `has_worktree` flag recommended for clarity)
3. **ScanResult struct**: Can represent branches without worktrees by using `repo_path` as fallback for `worktree_path`
4. **Omnibar**: Already accepts pre-filled input via `initialInput` prop; format is `${path}@${branch}`
5. **Card component**: Existing `UnfinishedItem` and detail component can be extended with a new `onResume` callback and conditional "Resume" button for branch-only results
