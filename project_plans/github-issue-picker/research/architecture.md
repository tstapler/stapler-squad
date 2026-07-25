# GitHub Issue Picker — Architecture Research

Date: 2026-07-03
Feature: Smart GitHub Issue Picker (replaces bare URL input on Import from GitHub Issue modal)

---

## 1. Integration Points with Existing Systems

### 1.1 Session / Worktree Repo Path Storage

Sessions store repo metadata in `InstanceData` (defined in `session/storage.go`):

```go
Path          string          // absolute path to workspace repo root
GitHubOwner   string          // e.g. "tstapler"
GitHubRepo    string          // e.g. "stapler-squad"
Worktree      GitWorktreeData // RepoPath, WorktreePath, BranchName
```

These fields are persisted in the SQLite DB via `ent_repository.go` (ent ORM columns `github_owner`, `github_repo`). `ListInstanceData()` in `session/storage.go` (line 359) returns raw `[]InstanceData` from the DB without spawning processes — the cheap read path to use for populating the local repo list.

The frontend already consumes this: `useSessionRepoPaths` hook (`web-app/src/lib/hooks/useSessionRepoPaths.ts`) reads from the Redux `sessionsSlice` store and returns a deduplicated list of session `path` values. The store is populated by `useSessionService` via the `ListSessions` RPC stream. To surface `owner/repo` pairs (not just paths), the new backend RPC can query `ListInstanceData` and filter for non-empty `GitHubOwner`.

### 1.2 How BacklogService Accesses Session Data for Repo Paths

`BacklogService.resolveRepoPathInput()` (`server/services/backlog_service.go`, line 150) converts a GitHub URL/shorthand to a local clone path via `session.ResolveGitHubInput` (which calls `RepoPathManager.EnsureRepoCloned`). This is the existing path used by `ImportGitHubIssue`.

The new `SearchGitHubRepos` RPC should call `s.storage.ListInstanceData()` (the same storage interface used by `SessionService`) to enumerate repos from sessions, supplemented by `gh repo list` for the GitHub search tier. `BacklogService` already holds a reference to the storage layer.

### 1.3 Existing Proto Message Patterns

`backlog.proto` follows this pattern for list RPCs:
- Request: filter params (repeated status, priority, etc.)
- Response: `repeated <Entity> entities = 1;`

New RPCs should match:

```protobuf
message SearchGitHubReposRequest {
  string query = 1;           // optional freetext for gh repo list search
  bool local_only = 2;        // if true, skip gh repo list call
  int32 limit = 3;            // max results (default 30)
}

message GitHubRepoEntry {
  string owner = 1;
  string repo  = 2;
  bool   is_local = 3;        // true = derived from sessions/worktrees
  string local_path = 4;      // populated when is_local=true
  int32  open_issue_count = 5; // from gh repo view --json (optional, may be 0)
  string description = 6;
  string updated_at  = 7;
}

message SearchGitHubReposResponse {
  repeated GitHubRepoEntry repos = 1;
}

message ListGitHubIssuesRequest {
  string owner  = 1;
  string repo   = 2;
  string state  = 3;   // "open" | "closed" | "all" (default "open")
  string labels = 4;   // comma-separated label filter (passed to gh CLI)
  int32  limit  = 5;   // max (default 30)
}

message GitHubIssueEntry {
  int32  number    = 1;
  string title     = 2;
  string state     = 3;
  string url       = 4;
  repeated string labels = 5;
  string assignee  = 6;
  string body      = 7;   // may be empty for list view; populated by gh issue view
  string updated_at = 8;
}

message ListGitHubIssuesResponse {
  repeated GitHubIssueEntry issues = 1;
}
```

Where to add: `proto/session/v1/backlog.proto` (new messages + new RPCs on `BacklogService`). Run `make generate-proto` after.

### 1.4 How safeexec.CommandContext Is Used

Canonical example from `backlog_service.go` lines 1721–1730:

```go
ghCtx, ghCancel := context.WithTimeout(ctx, 30*time.Second)
defer ghCancel()
cmd := safeexec.CommandContext(ghCtx, "gh", "issue", "view", number,
    "--repo", owner+"/"+repo,
    "--json", "number,title,body,labels,url,state")
out, err := cmd.Output()
```

For `ListGitHubIssues`, the equivalent would be:
```go
cmd := safeexec.CommandContext(ghCtx, "gh", "issue", "list",
    "--repo", owner+"/"+repo,
    "--state", state,
    "--limit", strconv.Itoa(limit),
    "--json", "number,title,state,labels,url,assignees,updatedAt")
```

For `SearchGitHubRepos` (GitHub tier):
```go
cmd := safeexec.CommandContext(ghCtx, "gh", "repo", "list",
    "--limit", "30",
    "--json", "owner,name,description,updatedAt,openIssueCount")
// optionally with --source --fork flags to control repo types
```

Both services use `safeexec.CommandContext` (not `exec.Command`) — this must be preserved. Import path: `github.com/tstapler/stapler-squad/executor/safeexec`.

---

## 2. Data Flow Design

### 2.1 RPC Return Shapes

**SearchGitHubRepos** returns `[]GitHubRepoEntry` with `is_local=true` for repos derived from sessions/worktrees and `is_local=false` for repos from `gh repo list`. Local repos are always fast; GitHub repos require the CLI subprocess. The backend returns them merged in a single response with local repos sorted first (lower index = shown first in the picker).

**ListGitHubIssues** returns `[]GitHubIssueEntry` from `gh issue list --json`. No body field in list view (omit unless requested) to keep response compact. The existing `ghIssueJSON` struct in backlog_service.go is a private type — the new handler will need its own struct or the existing one can be made package-internal.

### 2.2 Caching: Backend vs Frontend

**Recommendation: Frontend localStorage caching.**

Rationale:
- This is a single-user local process. There is no shared cache benefit across users.
- The backend already holds `sync.Map` caches for branches (session_service.go line 117) and worktree detection (repo_path.go line 25). Adding more in-memory caches for a transient UI feature increases backend complexity without benefit.
- localStorage with TTL is the established pattern in the codebase (review-queue auto-advance preference, theme preference). The app already uses localStorage for UI state.
- Frontend caching survives page navigation within the same browser session; backend in-memory caching would survive server restarts but be lost on window refresh anyway.
- Cache keys are well-defined: `ssq:gh-repos:v1` (4h TTL), `ssq:gh-issues:{owner}/{repo}:{state}:{labels}` (5m TTL).
- `gh repo list` is slow (~1–3s); the 4h localStorage TTL avoids re-running it on every modal open.

Pattern: On modal open, read from localStorage first. If stale or absent, call the RPC. Store RPC response in localStorage with an expiry timestamp. Parallel: fire `SearchGitHubRepos(local_only: true)` immediately (instant, from DB) to populate the local tier while the full GitHub tier loads lazily.

### 2.3 Surfacing Local Repos

**Recommendation: Backend returns them as a separate `is_local` flag, frontend renders them in a "From your sessions" section.**

The backend `SearchGitHubRepos` handler queries `s.storage.ListInstanceData()`, deduplicates by `{GitHubOwner, GitHubRepo}`, and returns those entries with `is_local=true`. No network call needed for this tier. The GitHub tier (`gh repo list`) is fetched separately when `local_only=false`. The frontend merges the two tiers client-side: local repos appear first (no badge needed, just order), GitHub-only repos appear below. This avoids a complex two-phase streaming response while keeping the picker immediately useful.

---

## 3. Component Architecture

### 3.1 Composition vs Monolith

**Recommendation: Composed components.**

```
GitHubIssuePicker (modal wrapper, state owner)
├── RepoSelector (tiered list + search input)
│   └── RepoOption (single row: owner/repo, local badge, issue count)
├── IssueList (virtualized list with keyboard nav)
│   ├── IssueFilterBar (state/label toggles — client-side filter)
│   └── IssueRow (number, title, labels, state chip)
└── IssueDetail (optional: show body on hover/selection)
```

This mirrors the existing pattern of `RepoPathInput` (which composes `PathCompletionDropdown`) and `BacklogItemForm` (which uses `RepoPathInput`).

### 3.2 State Ownership

| State | Location | Reason |
|---|---|---|
| `selectedRepo` | `GitHubIssuePicker` component state | Drives both RepoSelector display and IssueList fetch |
| `repoList` (GitHub tier) | `useGitHubIssuePicker` hook | Encapsulates caching + RPC call |
| `issueList` (for selected repo) | `useGitHubIssuePicker` hook | Encapsulates caching + RPC call |
| `stateFilter`, `labelFilter` | `IssueFilterBar` local state + lifted to hook | Client-side derived filter; no new RPC |
| `searchQuery` (repo search) | `RepoSelector` local state | Local filter on already-fetched list |
| Cache state | localStorage (via hook utility) | Survives re-renders, shared across instances |

No context is needed — the picker is a modal with a single mount point. A custom hook `useGitHubIssuePicker` encapsulates all async state (repo list, issue list, loading, error).

### 3.3 How useBacklogService Pattern Works

From `web-app/src/lib/hooks/useBacklogService.ts`:
- `useRef` holds the ConnectRPC client (initialized once in `useEffect`).
- Each method is a `useCallback(..., [])` — stable reference, no re-creation on renders.
- The hook returns a `useMemo`-wrapped object so the object reference only changes when `lastError` changes.
- No Redux store — all state is local to the hook and component subtree.

The new `useGitHubIssuePicker` hook should follow the same pattern: `useRef` for the client, `useState` for loading/error/data, `useCallback` for fetch methods, caching via localStorage in the fetch methods before/after the RPC call.

---

## 4. Tiered Repo Design — Implementation Details

### Phase 1: Instant (on modal open)
1. Call `SearchGitHubRepos({ local_only: true })` — backend queries `ListInstanceData()`, returns repos with non-empty `GitHubOwner`. Response typically <50ms (SQLite in-process).
2. Render local repos immediately in `RepoSelector`.

### Phase 2: Deferred (on user interaction or 500ms delay)
1. Check localStorage for `ssq:gh-repos:v1` with TTL check (4h). If valid, merge with local repos and render.
2. If stale/absent, call `SearchGitHubRepos({ local_only: false })`, store response in localStorage, merge with local repos.

### Phase 3: Issue fetch (on repo selection)
1. Check localStorage for `ssq:gh-issues:{owner}/{repo}:open:` with TTL (5m). If valid, render.
2. If stale/absent, call `ListGitHubIssues({ owner, repo, state: "open", limit: 30 })`, store in localStorage.
3. Client-side filter by `stateFilter` and `labelFilter` (no re-fetch needed for open/closed toggle — fetch both and filter).

### Keyboard Navigation
The `RepoPathInput` + `PathCompletionDropdown` pattern implements keyboard nav with `selectedIndex` state and `ArrowDown/ArrowUp/Enter/Escape` key handlers. The `GitHubIssuePicker` should follow the same pattern for both `RepoSelector` and `IssueList`, with focus management between the two panes (Tab to move from repo selector to issue list).

### TTL Cache Key Schema
```
ssq:gh-repos:v1                              → { repos: GitHubRepoEntry[], expiry: number }
ssq:gh-issues:{owner}/{repo}:{state}:{labels} → { issues: GitHubIssueEntry[], expiry: number }
```

---

## 5. Where the New RPC Handler Lives

Add `SearchGitHubRepos` and `ListGitHubIssues` to `BacklogService` (not `SessionService`) because:
- They directly support the backlog import workflow.
- `BacklogService` already has the `resolveRepoPathInput` / storage access pattern.
- `SessionService` is already large; backlog concerns belong in `BacklogService`.

Handler skeleton follows the `ImportGitHubIssue` pattern:
```go
func (s *BacklogService) SearchGitHubRepos(ctx context.Context, req *connect.Request[sessionv1.SearchGitHubReposRequest]) (*connect.Response[sessionv1.SearchGitHubReposResponse], error) {
    // 1. Query local sessions for repos (always)
    localRepos := s.localReposFromStorage(ctx)
    if req.Msg.LocalOnly {
        return connect.NewResponse(&sessionv1.SearchGitHubReposResponse{Repos: localRepos}), nil
    }
    // 2. Fetch from gh repo list
    ghRepos := s.fetchGitHubRepos(ctx, req.Msg.Query, req.Msg.Limit)
    // 3. Merge: local first, deduplicate
    return connect.NewResponse(&sessionv1.SearchGitHubReposResponse{Repos: merge(localRepos, ghRepos)}), nil
}
```

---

## 6. Files to Create/Modify

### Backend
- `proto/session/v1/backlog.proto` — add `SearchGitHubReposRequest/Response`, `ListGitHubIssuesRequest/Response`, new RPCs on `BacklogService`
- `server/services/backlog_service.go` — implement `SearchGitHubRepos`, `ListGitHubIssues`
- `make generate-proto` — regenerate Go + TypeScript bindings

### Frontend
- `web-app/src/components/backlog/GitHubIssuePicker.tsx` — new modal component
- `web-app/src/components/backlog/GitHubIssuePicker.css.ts` — vanilla-extract styles
- `web-app/src/lib/hooks/useGitHubIssuePicker.ts` — hook (RPC calls + localStorage caching)
- `web-app/src/app/backlog/page.tsx` — replace bare `<input type="url">` in `formMode === "github"` with `<GitHubIssuePicker>`
- `web-app/src/lib/hooks/useBacklogService.ts` — no changes needed (importGitHubIssue RPC already exists)

### Tests
- `server/services/backlog_service_test.go` — unit tests for `SearchGitHubRepos`, `ListGitHubIssues`
- `web-app/src/components/backlog/GitHubIssuePicker.test.tsx` — RTL component tests
- `tests/e2e/github-issue-picker.spec.ts` — Playwright e2e

### Registry Updates
- `docs/registry/backend-features.json` — add `backlog:search-github-repos`, `backlog:list-github-issues`
- `docs/registry/frontend-features.json` — add `github-issue-picker`

---

## 7. Key Risks and Constraints

1. **gh CLI availability**: `gh` must be authenticated. The existing `ImportGitHubIssue` already has this dependency; the new RPCs inherit the same risk. No mitigation needed beyond matching existing error handling pattern.
2. **`gh repo list` latency**: Can be 1–3 seconds. The localStorage cache (4h TTL) mitigates repeat calls. The tiered design ensures the picker is immediately usable with local repos while the GitHub tier loads.
3. **localStorage size**: Issue bodies can be large. Cache only list-view fields (no body) to stay well under the ~5MB localStorage quota.
4. **Session store `githubOwner`**: The Redux sessions store (populated by `ListSessions`) includes `githubOwner` / `githubRepo` fields from the proto `Session` message (`types.proto` lines 90–93). The existing `useSessionRepoPaths` hook reads only `path`; a new `useSessionRepos` hook could also return `{owner, repo}` pairs from the store without any new RPC. This could replace the `SearchGitHubRepos(local_only:true)` call for the local tier, avoiding a round-trip entirely. Consider this optimization during implementation.
