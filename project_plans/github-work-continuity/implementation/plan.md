# Implementation Plan: GitHub Work Continuity

**Status**: Planning complete | Ready for implementation
**Date**: 2026-06-24
**Branch**: `github-work-continuity`
**Supersedes**: `docs/tasks/github-pr-status.md` (cherry-picked work noted inline)

## Current State Audit

Before reading this plan, know what's already built from `claude-squad-pr-integration`:

| Artifact | Status | File |
|---|---|---|
| `PRStatusPoller` | **COMPLETE** (398 lines, fully wired) | `session/pr_status_poller.go` |
| `ETagCache` | **COMPLETE** (105 lines) | `github/etag_cache.go` |
| `GetPRForBranch()` | **COMPLETE** | `github/client.go:316` |
| `IsForkRepo()` | **COMPLETE** | `github/client.go:369` |
| `DerivePRPriority()` | **COMPLETE** | `github/priority.go` |
| `Instance.GitHub*` fields (PR state, priority, etc.) | **COMPLETE** (fields 33–39) | `proto/session/v1/types.proto` |
| `PRStatusPoller` wired into `BuildRuntimeDeps` | **COMPLETE** | `server/dependencies.go:333,490,650` |
| `GitHubBadge.tsx` with priority, tooltip, a11y | **COMPLETE** (179 lines) | `web-app/src/components/sessions/GitHubBadge.tsx` |
| `GitHubBadge.css.ts` (vanilla-extract) | **COMPLETE** | `web-app/src/components/sessions/GitHubBadge.css.ts` |
| `SessionCard.tsx` passes all GitHub fields | **COMPLETE** (line 457–474) | `web-app/src/components/sessions/SessionCard.tsx` |
| BUG-020 (VCS mutex) | **OPEN** | `docs/bugs/open/BUG-020-vcs-status-diff-mutex-contention.md` |
| BUG-021 (CheckGHAuth mutex) | **OPEN** | `docs/bugs/open/BUG-021-check-gh-auth-mutex-contention.md` |
| `UnfinishedWorktree` GitHub PR enrichment fields | **NOT STARTED** | `proto/session/v1/types.proto` (last field 20) |
| `WorktreePRPoller` / PR enrichment for no-session worktrees | **NOT STARTED** | — |
| `UserPRCache` / `GitHubUserService` | **NOT STARTED** | — |
| GitHub PRs section in Unfinished tab | **NOT STARTED** | — |
| G5 commit/push diff preview | **NOT STARTED** | — |
| G8 hardcoded "main" (scanner uses ResolveDefaultBranch) | **DONE** (scanner.go:318 already calls it) | `session/unfinished/scanner.go` |
| B4 Dormant Branches section | **NOT STARTED** | — |

**Net result**: Epic 5 (Session Card Badge) is complete. Epic 1 pre-flight (BUG-020/021) is the
only outstanding prerequisite before Epics 2–6 add more `gh` callers.

---

## Epic Map

```
Epic 1: Pre-flight (BUG-020/021 mutex fixes) — prerequisite for everything
  └── Epic 2: WorktreePRPoller (extend PRStatusPoller to cover no-session worktrees)
      └── Epic 3: UserPR List (UserPRCache + GitHubUserService + proto + frontend)
          └── Epic 4: Unfinished Tab GitHub Enrichment (wire into UnfinishedWorkService + frontend)
              └── Epic 6: Gap Fixes (G5 diff preview, B2 merged-PR filter, G6 session↔unfinished link, B4 dormant section)
Epic 5: Session Card Badge — ALREADY COMPLETE (no work needed)
```

---

## Epic 1: Pre-flight — Fix BUG-020/BUG-021 Mutex Issues

**Goal**: Eliminate mutex contention caused by subprocess/network calls held under locks before
adding any new `gh` callers that would compound the problem.

**Depends on**: none (prerequisite)

**Why first**: The pitfalls research is explicit: "Every new GetPR* call added to the PR polling
path will make these bugs measurably worse if called inside any existing lock. The fix must be
applied before adding more gh CLI callers."

---

### Story 1.1: Fix BUG-020 — VCS Status/Diff Mutex Scope

**Goal**: Remove git subprocess calls from inside lock scopes in `GetVCSStatus` and `GetSessionDiff`.

**Acceptance criteria**:
- `GetVCSStatus` reads session state under lock, releases lock, then runs git subprocess
- `GetSessionDiff` reads session state under lock, releases lock, then runs git subprocess
- Mutex profiling shows VCS handlers no longer dominate contention (< 5% of total delay)
- Existing API behavior is unchanged (same response fields, no regression)

**Files**:
- `server/services/session_service.go` — `GetVCSStatus` and `GetSessionDiff` handlers
- `docs/bugs/open/BUG-020-vcs-status-diff-mutex-contention.md` — update status to fixed

**Implementation notes**:

Pattern to follow in both handlers:
```go
// Step 1: read all needed state under lock
s.mu.RLock()
inst := s.instances[key]
worktreePath := inst.WorktreePath
branch := inst.Branch
s.mu.RUnlock()

// Step 2: run subprocess without holding any lock
output, err := runGitCommand(ctx, worktreePath, ...)
```

The existing handler likely holds `s.mu.RLock()` across the entire RPC body including the
subprocess call. Split the lock scope into (1) state read under lock, (2) subprocess outside lock.

---

### Story 1.2: Fix BUG-021 — CheckGHAuth Call-Site Mutex Scope

**Goal**: Ensure `CheckGHAuth()` is never called while any session or server mutex is held.

**Acceptance criteria**:
- `CheckGHAuth` is not called inside any function that holds `s.mu`, `inst.stateMutex`, or
  any other lock visible in the mutex profile
- `github.CheckGHAuth` is no longer in the top mutex contention nodes in pprof output
- Server builds and tests pass

**Files**:
- `github/client.go` — audit all call sites of `CheckGHAuth()` (lines: `GetPRComments`,
  `GetPRDiff`, `PostPRComment`, `MergePR`, `ClosePR`)
- `server/services/session_service.go` — verify no handler calls `CheckGHAuth` under a lock
- `docs/bugs/open/BUG-021-check-gh-auth-mutex-contention.md` — update status to fixed

**Implementation notes**:

The PRStatusPoller already caches auth state in `ghAuthState` (from the completed pr_status_poller.go)
with a 5-minute TTL, calling `CheckGHAuth()` outside any lock scope. The fix here is to audit the
*other* call sites — the RPC handlers that call `GetPRComments`, `GetPRDiff`, etc. — and ensure
those callers have released their locks before calling into the GitHub client.

Do NOT move `CheckGHAuth()` inside the GitHub client functions themselves behind their own mutex —
that would just move the contention. The fix is at the call site: release before calling.

---

### Story 1.3: Add Rate-Limit Header Monitoring to ghHTTPClient

**Goal**: Add `X-RateLimit-Remaining` inspection and `Retry-After` handling to prevent silent
failures from GitHub secondary rate limits as PR API call volume grows.

**Acceptance criteria**:
- `newGHRequest` helper (or a new `executeGHRequest` wrapper) reads `X-RateLimit-Remaining`
  from every REST response
- A warning is logged when remaining < 500
- `429` and `403` responses with `Retry-After` header trigger a configurable backoff sleep
  (default: respect `Retry-After` header value, max 60s)
- `X-GitHub-Sso` header on 403 responses surfaces a specific log message about SSO authorization

**Files**:
- `github/http_client.go` — extend response handling after each `client.Do(req)` call
- `github/etag_cache.go` — apply same header check in `GetPRInfoConditional`

---

## Epic 2: Assess & Integrate PRStatusPoller — Extend to Worktrees Without Sessions

**Goal**: Extend the existing `PRStatusPoller` (or add a `WorktreePRPoller`) so that worktrees
discovered by the scanner but without active sessions also get GitHub PR enrichment.

**Depends on**: Epic 1 (BUG-020/021 must be fixed before adding new gh callers)

---

### Story 2.1: Build WorktreePRPoller

**Goal**: Create `WorktreePRPoller` in `session/` that polls GitHub for worktrees that have no
active session — providing the PR data that `UnfinishedWorkService` needs for enrichment.

**Acceptance criteria**:
- `WorktreePRPoller` accepts a feed of `[]unfinished.ScanResult` from the scanner
- For each `ScanResult` where no session exists for the worktree path, it calls
  `GetPRForBranch(owner, repo, branch)` to discover the PR
- Discovered PR info is stored in an in-memory `map[string]*github.PRInfo` (key: `repoPath|branch`)
- ETag conditional polling runs every 60 seconds (same cadence as `PRStatusPoller`)
- Auth is checked once per 5-minute interval (not per poll) via the same `ghAuthState` pattern
- `GetPRData(repoPath, branch string) *github.PRInfo` returns current cached data
- Poller fires an `onUpdated` callback when PR data changes (for EventBus notification)

**Files**:
- `session/worktree_pr_poller.go` (new file, ~250 lines)
- `session/worktree_pr_poller_test.go` (new file)

**Implementation notes**:

`WorktreePRPoller` is structurally similar to `PRStatusPoller` but operates on `ScanResult` value
types instead of `*Instance` pointers. It does not write to storage (no persistent session to update).
It only maintains an in-memory cache of PR info per worktree.

To derive `owner` and `repo` from a `ScanResult`, call `github.GetOwnerRepoFromRemote(repoPath)`.
Verify this function exists in `github/client.go`; if not, add it as a helper that runs
`git remote get-url origin` and parses the `github.com/{owner}/{repo}` pattern.

Key difference from `PRStatusPoller`: worktrees without sessions have no persistent `GitHubOwner`
field. Owner/repo must be inferred from the remote URL at discovery time.

```go
type WorktreePRPoller struct {
    etagCache  *github.ETagCache
    scanner    *unfinished.Scanner  // polled via ScanDone() + GetAllResults()
    mu         sync.RWMutex
    data       map[string]*github.PRInfo  // key: repoPath+"|"+branch
    onUpdated  func(repoPath, branch string, info *github.PRInfo)

    // Auth state: inline fields matching PRStatusPoller's pattern exactly.
    // github.CachedAuthState does NOT exist as an exported type.
    authOK        bool
    authCheckedAt time.Time
    authMu        sync.Mutex

    pollTicker *time.Ticker
    ctx        context.Context
    cancel     context.CancelFunc
    wg         sync.WaitGroup
}
```

The poll loop subscribes to `scanner.ScanDone()` to detect when a new scan has completed,
then calls `scanner.GetAllResults()` to get the current snapshot of worktrees. There is no
`chan []ScanResult` in the Scanner API — `ScanDone() <-chan time.Time` and
`GetAllResults() []ScanResult` are the correct methods (verified in `session/unfinished/scanner.go:198,492`).

```go
// Pseudocode for the poll loop:
for {
    select {
    case <-s.ctx.Done():
        return
    case <-scanner.ScanDone():
        results := scanner.GetAllResults()
        s.pollWorktrees(results)
    case <-s.pollTicker.C:
        results := scanner.GetAllResults()
        s.pollWorktrees(results)
    }
}
```

Session exclusion: skip any `ScanResult.WorktreePath` that appears in
`PRStatusPoller`'s instance list (to avoid double-polling). The service layer will
query `PRStatusPoller` for session-backed worktrees and `WorktreePRPoller` for the rest.

**Import direction — new dependency, verify before merging**:
`session/worktree_pr_poller.go` will be the FIRST file in `package session` to import
`github.com/tstapler/stapler-squad/session/unfinished`. This is an intentional new dependency
direction. `session/unfinished` must NOT import `session` (that would create a cycle).
Run `go build ./...` before merging to confirm no import cycle exists. If the project gains
an architecture-vet rule in the future, add `session/unfinished` → `session` as a forbidden edge.

---

### Story 2.2: Feed Scanner Results to WorktreePRPoller

**Goal**: Wire `WorktreePRPoller` into the server dependency graph so it receives scanner output
and starts polling.

**Acceptance criteria**:
- `WorktreePRPoller` is constructed in `BuildRuntimeDeps` (`server/dependencies.go`)
- Scanner's `GetAllResults()` is called on the poller's first tick to seed the initial worktree list
- `EventUnfinishedWorkUpdated` events (const in `session/unfinished/events.go`) trigger a re-evaluation of which worktrees need polling
- Server shutdown stops the poller cleanly via context cancellation
- Build compiles with no errors

**Files**:
- `server/dependencies.go` — add `WorktreePRPoller` to `ServiceDeps` and `RuntimeDeps`
- `server/services/unfinished_work_service.go` — add `WorktreePRPoller` field to service struct

**Implementation notes**:

In `BuildRuntimeDeps`, after constructing `prStatusPoller`:
```go
worktreePRPoller := session.NewWorktreePRPoller(
    github.NewETagCache(),  // separate cache from PRStatusPoller's cache
    prStatusPoller,         // for session exclusion
)
worktreePRPoller.Start(ctx)
```

Subscribe the poller to `EventUnfinishedWorkUpdated` events (`session/unfinished/events.go:11`)
from the EventBus so it re-evaluates its worktree list when the scanner finds new or removed worktrees.

---

### Story 2.3: Wire WorktreePRPoller into UnfinishedWorkService

**Goal**: `UnfinishedWorkService` uses `WorktreePRPoller.GetPRData()` and
`PRStatusPoller`'s instance PR fields to enrich scan results before sending to the frontend.

**Acceptance criteria**:
- `scanResultToProto()` (line 507 of `unfinished_work_service.go`) enriches
  `UnfinishedWorktree` proto with GitHub PR fields from either source
- For worktrees with an active session: use `Instance.GitHubPR*` fields
- For worktrees without a session: use `WorktreePRPoller.GetPRData(repoPath, branch)`
- If no PR data is available for a worktree, GitHub fields are zero/empty (no crash)
- `WatchUnfinishedWork` stream also re-emits enriched data when the poller fires `onUpdated`

**Files**:
- `server/services/unfinished_work_service.go` — update `scanResultToProto()`, add `instancePRIndex()` helper
- `proto/session/v1/types.proto` — add GitHub PR fields to `UnfinishedWorktree` (fields 21–28)
- `gen/` — regenerate via `make proto-gen`
- `web-app/src/` — TypeScript type updates auto-generated

**Implementation notes**:

Add to `UnfinishedWorktree` proto (after field 20, `session_ids`):
```protobuf
// GitHub PR enrichment fields (populated by WorktreePRPoller or PRStatusPoller)
int32  github_pr_number       = 21;
string github_pr_url          = 22;
string github_pr_priority     = 23;  // "blocking"/"ready"/"pending"/"draft"/"complete"/"no_pr"
string github_pr_state        = 24;  // "open"/"closed"/"merged"
bool   github_pr_is_draft     = 25;
int32  github_approved_count  = 26;
int32  github_changes_req_count = 27;
string github_check_conclusion = 28;
```

Add `instancePRIndex()` helper to `UnfinishedWorkService` (mirror of `sessionPathIndex()`):
```go
// instancePRIndex builds a repoPath+"|"+branch → *Instance map for PR field lookup.
// Only instances with a non-empty GitHubPRPriority are included.
func (s *UnfinishedWorkService) instancePRIndex() map[string]*session.Instance
```

In `scanResultToProto()`:
1. Check `instancePRIndex` for worktree's `repoPath|branch` key
2. If found: populate proto fields from `Instance.GitHubPR*`
3. If not found: call `s.worktreePRPoller.GetPRData(repoPath, branch)` and map fields

---

## Epic 3: UserPR List — Cache, ConnectRPC Service, Proto, Frontend

**Goal**: Expose "all open PRs authored by the current user" as a new ConnectRPC service so the
frontend can render the GitHub PRs section in the Unfinished tab.

**Depends on**: Epic 2 (needs `WorktreePRPoller` to know which PRs have local worktrees)

---

### Story 3.1: UserPRCache in github Package

**Goal**: Add a background-refreshing cache of all open (and recently closed) PRs authored by
the authenticated user, across all repos.

**Acceptance criteria**:
- `UserPRCache` struct in `github/user_pr_cache.go` with:
  - `Start(ctx context.Context)` — begins background 5-minute refresh loop
  - `GetOpenPRs() []*UserPR` — returns cached open PRs (sorted by last updated desc)
  - `GetRecentlyClosed(n int) []*UserPR` — returns n most recently closed/merged PRs
  - `GetByBranch(owner, repo, branch string) *UserPR` — lookup for local worktree linking
  - `OnUpdated(func([]*UserPR))` — callback fired when cache is refreshed
- Fetches data via `gh api graphql` subprocess (1 call per refresh, returns all fields)
- ETag support: if the search result is unchanged, HTTP 304 = no refresh, reset TTL
- Auth failure: stops refreshing and stores an error state (no crash, no infinite retry)
- Thread-safe: `sync.RWMutex` on the data store

**Files**:
- `github/user_pr_cache.go` (new file, ~200 lines)
- `github/user_pr_cache_test.go` (new file)

**Implementation notes**:

GraphQL query to use (via `gh api graphql -f query=...`):
```graphql
query($login: String!) {
  search(query: "is:pr author:$login", type: ISSUE, first: 50) {
    nodes {
      ... on PullRequest {
        number title url state isDraft
        headRefName baseRefName
        reviewDecision
        repository { owner { login } name }
        reviews(last: 10) { nodes { state author { login } } }
        statusCheckRollup { state }
        updatedAt createdAt closedAt mergedAt
      }
    }
  }
}
```

The `$login` value comes from a single `GET /user` REST call at cache initialization — cache
this in a `CachedGitHubUser` struct in `github/http_client.go`.

Map GraphQL response to a `UserPR` struct:
```go
type UserPR struct {
    Owner, Repo, Branch, BaseRef, Title, URL string
    Number                                   int
    State                                    string  // "OPEN"/"CLOSED"/"MERGED"
    IsDraft                                  bool
    Priority                                 PRPriority  // derived via DerivePRPriority
    ApprovedCount, ChangesReqCount           int
    CheckConclusion                          string
    UpdatedAt, ClosedAt, MergedAt            time.Time
}
```

Call `DerivePRPriority()` on each result during cache population (reuse existing function).

---

### Story 3.2: Link UserPR Data to Local Sessions and Worktrees

**Goal**: Annotate each `UserPR` in the cache with local session IDs and worktree paths so the
frontend can surface the "Start session" or "Open session" action.

**Acceptance criteria**:
- `UserPRCache.Annotate(sessions []*session.Instance, worktrees []unfinished.ScanResult)` method
  sets `SessionIDs` and `LocalWorktreePath` on each cached `UserPR`
- Called after each cache refresh AND after `PRStatusPoller` fires an update (via callback)
- Matching logic: `UserPR.Branch == instance.Branch && UserPR.Owner == instance.GitHubOwner`
- Matching logic for worktrees: `UserPR.Branch == scanResult.Branch && ownerFromRemote == UserPR.Owner`

**Files**:
- `github/user_pr_cache.go` — add `Annotate()` method
- `server/dependencies.go` — wire annotation callback in `BuildRuntimeDeps`

---

### Story 3.3: UserPR Proto Message + GitHubUserService Proto Definition

**Goal**: Define the new proto types required for the UserPR section.

**Acceptance criteria**:
- New `UserPR` message defined in `proto/session/v1/types.proto` (fields as specified)
- New `GitHubUserService` service defined in a new `proto/session/v1/github_user.proto` file
  with `ListUserPRs` (unary) and `WatchUserPRs` (server streaming) RPCs
- `make proto-gen` produces Go and TypeScript files with no errors
- No field number conflicts with existing `Session` or `UnfinishedWorktree` messages

**Files**:
- `proto/session/v1/github_user.proto` (new file)
- `proto/session/v1/types.proto` — add `UserPR` message
- `gen/` — regenerated files

**Implementation notes**:

`UserPR` proto message (add to `types.proto`):
```protobuf
message UserPR {
  string owner              = 1;
  string repo               = 2;
  int32  number             = 3;
  string title              = 4;
  string html_url           = 5;
  string state              = 6;  // "OPEN"/"CLOSED"/"MERGED"
  string priority           = 7;  // reuse PRPriority string values
  string head_ref           = 8;
  string base_ref           = 9;
  bool   is_draft           = 10;
  string check_conclusion   = 11;
  int32  approved_count     = 12;
  int32  changes_req_count  = 13;
  google.protobuf.Timestamp updated_at   = 14;
  google.protobuf.Timestamp closed_at    = 15;
  repeated string session_ids            = 16;
  string local_worktree_path             = 17;
}
```

`github_user.proto`:
```protobuf
service GitHubUserService {
  rpc ListUserPRs(ListUserPRsRequest) returns (ListUserPRsResponse) {}
  rpc WatchUserPRs(WatchUserPRsRequest) returns (stream UserPREvent) {}
  rpc GetGitHubAuthState(GetGitHubAuthStateRequest) returns (GetGitHubAuthStateResponse) {}
}
```

`GetGitHubAuthState` returns: `{available: bool, username: string, error_message: string}` — used
by the frontend to show/hide the GitHub PRs section and render the auth banner.

---

### Story 3.4: GitHubUserService ConnectRPC Implementation

**Goal**: Implement the `GitHubUserService` handler that serves `UserPR` data from `UserPRCache`.

**Acceptance criteria**:
- `ListUserPRs` returns all open PRs + last 5 closed/merged from cache (sorted by priority then updated_at)
- `WatchUserPRs` streams `UserPREvent` (added/updated/removed) when `UserPRCache.OnUpdated` fires
- `GetGitHubAuthState` returns auth availability, GitHub username, and human-readable error if unauthenticated
- Service is registered in the HTTP mux alongside existing services
- Unauthenticated state returns an empty PR list with `auth_state.available = false`, not an error

**Files**:
- `server/services/github_user_service.go` (new file, ~150 lines)
- `server/server.go` or `server/routes.go` — register new service handler
- `server/dependencies.go` — add `UserPRCache` and `GitHubUserService` to deps

---

### Story 3.5: Frontend — GitHub PRs Section in Unfinished Tab

**Goal**: Add a "GitHub PRs" collapsible section at the top of the Unfinished Work tab showing
open PRs, each with a priority badge, and a "Recent" row for closed PRs.

**Acceptance criteria**:
- New `GitHubPRsSection.tsx` component in `web-app/src/components/unfinished/`
- Calls `WatchUserPRs` via the generated client; shows loading state on first load
- PR cards show: PR number, title, repo (`owner/repo`), priority badge, review counts, CI status
- Each PR card links to the local session if `session_ids` is non-empty, or shows "Start session" if not
- "Recent" subsection: 5 most recently closed/merged PRs as compact rows (not expandable)
- If `auth_state.available == false`: renders a dismissible banner ("GitHub not connected") instead of the list
- Section collapses/expands with state persisted in localStorage
- Section heading is "GitHub PRs" with a PR icon

**Files**:
- `web-app/src/components/unfinished/GitHubPRsSection.tsx` (new file)
- `web-app/src/components/unfinished/GitHubPRsSection.css.ts` (new file, vanilla-extract)
- `web-app/src/components/unfinished/GitHubPRCard.tsx` (new file)
- `web-app/src/pages/UnfinishedTab.tsx` or equivalent — import and render `GitHubPRsSection`

**Implementation notes**:

Use the generated `useWatchUserPRs` hook (ConnectRPC client hook pattern already used in the app).
Render the section above the existing worktree list. Reuse `GitHubBadge` for the priority badges.
Reuse the existing `PriorityBadge` or create a new `PRPriorityBadge` using the same vanilla-extract
recipe from `GitHubBadge.css.ts`.

---

## Epic 4: Unfinished Tab GitHub Enrichment

**Goal**: Wire `WorktreePRPoller` PR data into the Unfinished tab so worktree cards show GitHub
PR badges; merged/closed PR worktrees are deprioritized.

**Depends on**: Epic 2 (WorktreePRPoller), Epic 3 (UserPRCache for `GetByBranch` fallback)

---

### Story 4.1: Frontend — PR Badge on Worktree Cards in Unfinished Tab

**Goal**: Unfinished worktree cards render a `GitHubBadge` using the new `github_pr_*` fields
added in Story 2.3.

**Acceptance criteria**:
- Worktree cards with `github_pr_priority` set show a `GitHubBadge` (reuse existing component)
- Worktrees with `github_pr_state == "merged"` or `"closed"` are visually de-emphasized
  (reduced opacity, moved to "Completed" section at bottom)
- Worktrees with `github_pr_priority == "blocking"` are visually promoted (ordering: blocking
  first, then ready, then pending, then draft, then no_pr)
- `session_ids` links from the worktree card to the session (existing behavior, verify still works)

**Files**:
- `web-app/src/components/unfinished/WorktreeCard.tsx` — add `GitHubBadge` rendering
- `web-app/src/pages/UnfinishedTab.tsx` — add sort/filter logic for merged worktrees

---

### Story 4.2: Backend — B2 Filter: Hide Worktrees with Merged PRs

**Goal**: When a worktree's PR is merged/closed, the worktree item is moved to a separate
"Completed" bucket in the `ListUnfinishedWorkResponse` rather than removed entirely.

**Acceptance criteria**:
- `ListUnfinishedWorkResponse` proto gains a new `repeated UnfinishedWorktree completed_worktrees = 3`
  field alongside the existing `worktrees` field (field 2 is already `last_scan` — do NOT use 2)
- `UnfinishedWorkService.ListUnfinishedWork` splits worktrees into `worktrees` (active) and
  `completed_worktrees` (merged/closed PR) based on `github_pr_state`
- `WatchUnfinishedWork` stream includes `completed_worktrees` in its initial snapshot
- Items are not permanently dismissed — if the PR is reopened, they move back to `worktrees`

**Files**:
- `proto/session/v1/unfinished.proto` — add `completed_worktrees = 3` to `ListUnfinishedWorkResponse`
  (reserved field numbers: `worktrees = 1`, `last_scan = 2`; next available is 3)
- `server/services/unfinished_work_service.go` — split logic in `ListUnfinishedWork`
- `web-app/src/pages/UnfinishedTab.tsx` — render completed section at bottom

---

## Epic 5: Session Card Badge — COMPLETE

**Goal**: Session cards show a color-coded GitHub PR status badge.

**Status**: **ALREADY DONE.** All tasks from the superseded `docs/tasks/github-pr-status.md` have
been implemented on the `claude-squad-pr-integration` branch and are present on main:

- `PRStatusPoller` (398 lines, fully wired into server lifecycle)
- `GitHubBadge.tsx` (179 lines, with priority variants, tooltip, accessibility)
- `GitHubBadge.css.ts` (vanilla-extract recipe)
- `SessionCard.tsx` passes all GitHub fields (lines 457–474)
- Proto fields 33–39 on `Session` message
- `ETagCache`, `GetPRForBranch`, `IsForkRepo`, `DerivePRPriority` in `github/` package

**No work required in this Epic.** If session card badges are not rendering in testing, check:
1. `PRStatusPoller.IsRunning()` via server health endpoint
2. Session's `GitHubOwner` field is populated (only sessions created from a GitHub PR URL have it pre-set; for other sessions, `GetPRForBranch` auto-discovery runs on first poll tick)

---

## Epic 6: Gap Fixes

**Goal**: Address the triad-review gaps not covered by Epics 1–4.

**Depends on**: Epic 4 (for G6 navigation; Epics can be done independently otherwise)

---

### Story 6.1: G5 — Commit & Push Diff Preview Modal

**Goal**: The "Quick Commit & Push" shortcut shows a diff preview modal before executing, to
prevent accidental commits of unintended files.

**Acceptance criteria**:
- Clicking "Commit & Push" on a worktree card opens a modal showing:
  - File list: each changed file with +/- line count
  - Diff view (collapsible per file, shows full diff)
  - Commit message input with validation (non-empty)
  - "Confirm & Push" and "Cancel" buttons
- The `GetWorktreeDiff` RPC (already defined in `unfinished.proto`) is called to populate the modal
- Only on "Confirm & Push" click does `QuickCommitPush` execute
- Existing `QuickCommitPush` RPC in `unfinished.proto` is used unchanged

**Files**:
- `web-app/src/components/unfinished/CommitPushModal.tsx` (new file)
- `web-app/src/components/unfinished/CommitPushModal.css.ts` (new file)
- `web-app/src/components/unfinished/WorktreeCard.tsx` — replace direct commit action with modal trigger

---

### Story 6.2: G6 — Session Card → Unfinished Tab Deep Link

**Goal**: A session card shows a "View in Unfinished" link that navigates to the worktree item
in the Unfinished tab for that session's branch.

**Acceptance criteria**:
- Session cards with a corresponding worktree in the Unfinished tab show a small "Unfinished ↗"
  link in the card footer
- Clicking navigates to the Unfinished tab and scrolls to / highlights the matching worktree card
- Matching is by `session.WorktreePath == worktree.WorktreePath` (already available in proto)
- If the Unfinished tab does not have a matching item, the link is not shown

**Files**:
- `web-app/src/components/sessions/SessionCard.tsx` — add link element
- `web-app/src/pages/UnfinishedTab.tsx` — add URL hash / scroll-to support

**Implementation notes**:

Use a URL parameter (e.g., `?focusWorktree=<repoPath|branch>`) to deep-link into the Unfinished
tab. The SessionCard builds this URL using `session.WorktreePath` + `session.Branch`.
The UnfinishedTab reads the parameter on mount and scrolls the matching card into view.

---

### Story 6.3: B4 — Dormant Branches Section in Unfinished Tab

**Goal**: Branches that are ahead of default but have no active worktree session render in a
distinct "Dormant Branches" section, visually separated from worktree items.

**Acceptance criteria**:
- Worktrees with `status == SCAN_STATUS_AHEAD_ONLY` (no uncommitted changes, just ahead commits,
  no active session) are rendered in a separate "Dormant Branches" section
- Section has a distinct heading "Dormant Branches" and is collapsible
- Each dormant branch card shows: branch name, repo, commits ahead count, and a "Resume" button
  (same as existing behavior, just sectioned separately)
- If the dormant branch has a GitHub PR (from `github_pr_priority` field), the PR badge is shown
- Worktrees with merged PRs (`github_pr_state == "merged"`) are excluded from this section
  (they go to the "Completed" section from Story 4.2)
- Section ordering: GitHub PRs section → Active worktrees → Dormant Branches → Completed

**Files**:
- `web-app/src/pages/UnfinishedTab.tsx` — add section split logic and "Dormant Branches" rendering
- `web-app/src/components/unfinished/DormantBranchesSection.tsx` (new file)

---

### Story 6.4: B2 — WorktreePRPoller Triggers Refresh on Merge

**Goal**: When `WorktreePRPoller` detects that a worktree's PR has been merged or closed, it
fires an event that causes the Unfinished tab to re-evaluate that item's placement.

**Acceptance criteria**:
- `WorktreePRPoller.onUpdated` callback fires when PR state changes (including → merged/closed)
- `EventUnfinishedWorkUpdated` event (const in `session/unfinished/events.go`) is published with the updated `ScanResult` including new
  `github_pr_state == "merged"` field
- `WatchUnfinishedWork` stream subscribers receive the updated item and the frontend moves it
  to the "Completed" section without requiring a page reload

**Files**:
- `session/worktree_pr_poller.go` — verify `onUpdated` fires on state → merged/closed transition
- `server/services/unfinished_work_service.go` — handle poller callback → EventBus publish

**Note**: This story is largely a correctness check on Stories 2.1 and 2.3. The key new behavior
is that "merge detection" also updates the `WatchUnfinishedWork` stream (not just `ListUnfinishedWork`).

---

## Architectural Decisions Requiring ADRs

The following decisions made in this plan need ADRs before implementation begins:

| ADR # | Decision | Rationale |
|---|---|---|
| ADR-004 | `WorktreePRPoller` as a separate struct (not extending `PRStatusPoller`) | `PRStatusPoller` is session-centric with persistent state; worktrees are ephemeral. Mixing the two creates coupling between `session.Instance` lifecycle and `unfinished.ScanResult` lifecycle. A separate struct with a clean interface avoids this. |
| ADR-005 | `gh api graphql` subprocess for `UserPRCache` refresh (not direct REST HTTP) | GraphQL returns all required fields (CI, reviews, headRefName) in one call vs. N+1 REST calls. Subprocess avoids adding a GraphQL library dependency. Acceptable for a 5-minute cadence. |
| ADR-006 | `GitHubUserService` as a new top-level ConnectRPC service (not extending `UnfinishedWorkService`) | UserPR data is GitHub-centric and independent of the local git scanner. Embedding it in `UnfinishedWorkService` would create a mixed responsibility (local git scan + GitHub API). New service keeps the distinction clean and allows independent testing. |
| ADR-007 | Enrichment at API layer (service) not at scanner | Already decided in research/architecture.md Option B, but needs an ADR to record the rejected alternative (scan-time enrichment) and the rationale. |

---

## Delivery Sequence

Each Epic is independently releasable (no frontend breakage at intermediate states):

```
Week 1: Epic 1 (BUG-020/021 + rate limit monitoring)
Week 2: Epic 2 (WorktreePRPoller + service wiring + proto fields on UnfinishedWorktree)
Week 3: Epic 3 (UserPRCache + GitHubUserService + GitHub PRs section frontend)
Week 4: Epic 4 (Unfinished tab enrichment + B2 merged PR filter)
Week 5: Epic 6 (Gap fixes: G5 diff modal, G6 deep link, B4 dormant section)
```

Epic 5 is skipped — it's already done.

---

## Risk Register

| Risk | Severity | Mitigation |
|---|---|---|
| BUG-020/021 not fixed before Epic 2 adds new gh callers | High | Epic 1 is a hard prerequisite gate — do not start Epic 2 until BUG-020 and BUG-021 PRs are merged |
| `gh api graphql` subprocess output parsing for UserPRCache is brittle | Medium | Unit test with captured real output samples; graceful fallback to REST two-phase if GraphQL parse fails |
| WorktreePRPoller duplicates polling for worktrees that DO have sessions | Medium | Session exclusion list passed from PRStatusPoller; keyed on `WorktreePath` |
| Proto field conflicts between parallel branches | Low | Reserve field ranges in comments; `make proto-gen` fails on conflict |
| `GetOwnerRepoFromRemote` may not handle SSH remote URLs (`git@github.com:...`) | Low | Test against both HTTPS and SSH remote URL formats in `github/client.go` |
