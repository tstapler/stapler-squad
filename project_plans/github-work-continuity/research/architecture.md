# Architecture Research: GitHub + Local Git Data Merge

**Feature**: GitHub Work Continuity
**Research Date**: 2026-06-24
**Status**: Research complete

---

## 1. How ScanResult Is Stored and Served

`session/unfinished/scanner.go` defines `ScanResult` — a pure git struct with no GitHub data:

```go
type ScanResult struct {
    RepoPath, Branch, WorktreePath, RepoName, DisplayPath string
    HasUncommitted bool
    AheadCount, BehindCount int
    DefaultBranch string
    ChangedFiles, LinesAdded, LinesRemoved int
    AheadMessages []string
    LastModified, ScanTime time.Time
    Status ScanResultStatus
    SessionIDs []string  // injected by service layer
}
```

The `Scanner` struct stores results in a `sync.Map` (keyed by `repoPath|branch`). There is no
GitHub data in the scanner or its store.

**Service-layer aggregation** happens in `server/services/unfinished_work_service.go`:
`ListUnfinishedWork` calls `scanner.GetAllResults()`, then calls `sessionPathIndex()` to inject
`SessionIDs` from loaded instances, then maps to proto via `scanResultToProto()`. This is the
correct place to also inject GitHub PR data at response time.

`WatchUnfinishedWork` streams events in the same pattern: initial snapshot via `GetAllResults()`
then EventBus subscription. Both paths run through `scanResultToProto()`.

**Key finding**: the service layer is the aggregation point. `ScanResult` is pure git; enrichment
happens in the service, not the scanner.

---

## 2. GitHub Fields on Instance

`session/instance.go` (lines 161–198) already has a rich set of GitHub fields:

**Creation fields** (set when session is created from a PR URL):
- `GitHubPRNumber int`
- `GitHubPRURL string`
- `GitHubOwner string`
- `GitHubRepo string`
- `GitHubSourceRef string`
- `GitHubIsFork bool`

**PR status fields** (populated by PRStatusPoller):
- `GitHubPRState string` — "open"/"closed"/"merged"
- `GitHubPRIsDraft bool`
- `GitHubPRPriority string` — "blocking"/"ready"/"pending"/"draft"/"complete"/"no_pr"
- `GitHubApprovedCount int`
- `GitHubChangesReqCount int`
- `GitHubCheckConclusion string`
- `GitHubPRStatusTerminal bool`
- `LastPRStatusCheck time.Time`

All of these are already in the proto `Session` message as fields 25–39.

---

## 3. PRStatusPoller Exists and Is Fully Implemented

`session/pr_status_poller.go` is complete (399 lines). Key details:

- Polls all instances on a 60-second ticker (configurable)
- Skips instances where `GitHubOwner == ""` (no GitHub info)
- Auto-discovers PR numbers via `github.GetPRForBranch()` when `GitHubPRNumber == 0`
- ETag cache (`github.ETagCache`) avoids redundant API calls (HTTP 304 = no quota cost)
- 5-concurrency semaphore respects GitHub secondary rate limits
- NoPR backoff (5 min default) to avoid hammering branchless repos
- Calls `inst.UpdatePRStatus(...)` then `storage.UpdateInstancePRStatus(...)` to persist
- Fires `onUpdated(inst)` callback when priority changes (for EventBus notification)

The poller is **session-centric**: it iterates `[]*Instance` and matches PRs to sessions by
`GitHubOwner/GitHubRepo/Branch`. It does NOT have visibility into `ScanResult` worktrees that
have no session.

**Gap**: Unfinished worktrees that have no stapler-squad session (just local git branches) get
no PR enrichment from the current poller.

---

## 4. Proto Field Numbers

`proto/session/v1/types.proto` — `Session` message current last field: **47**
Available range starts at **48+** for new GitHub PR list fields.

`proto/session/v1/types.proto` — `UnfinishedWorktree` message current last field: **20**
Available range starts at **21+** for new GitHub PR enrichment fields.

`proto/session/v1/unfinished.proto` — `UnfinishedWorkService` service has 10 RPCs, no GitHub
methods. A new `ListUserPRs` RPC or a separate `GitHubPRService` can be added cleanly.

---

## 5. Architecture Option Evaluation

### Option A: Enrichment at Scan Time
Scanner calls GitHub API during each git scan when it finds a branch.

**Problems**:
- Violates single-responsibility: scanner becomes a git+GitHub hybrid
- GitHub API calls are slow (100–500ms) and rate-limited; embedding them in the scan worker
  pool would starve git scans or require a second rate-limiter
- Scanner is triggered by fsnotify on file changes — GitHub API calls on every file save is
  absurd
- Scanner has no auth state, no ETag cache, no retry logic — all of which the existing
  PRStatusPoller already provides
- ScanResult is a value type passed around widely; adding mutable GitHub fields forces
  concurrent-access discipline everywhere it's used

### Option B: Enrichment at API Layer (RECOMMENDED)

Scanner stays pure git. PRStatusPoller hydrates GitHub data into `Instance` fields. Service
layer merges the two before returning to frontend.

**How the merge works**:

```
ScanResult (scanner)       +       Instance.GitHub* fields (PRStatusPoller)
     |                                         |
     +-----> service layer merges on (repoPath+branch) <------+
                         |
                    proto response (UnfinishedWorktree with pr_* fields)
```

The service layer already does exactly this pattern for `SessionIDs` — it joins
`scanner.GetAllResults()` with `storage.LoadInstances()`. The same join can pick up
`GitHubPRPriority`, `GitHubPRState`, `GitHubCheckConclusion` etc.

**For worktrees without sessions**: Build a lightweight branch→PRInfo in-memory index inside
`UnfinishedWorkService` (or a new `WorktreePRPoller` struct) that runs independently of the
session-based `PRStatusPoller`. Key: `repoPath|branch`. Refresh on the same 60-second cadence.
This handles the "unfinished worktree with a PR but no active session" case.

**Advantages of Option B**:
- Scanner stays zero-dependency (no `github` package import)
- PRStatusPoller already has ETag cache, rate-limit backoff, auth check, concurrency semaphore
- Service layer is already the aggregation point (SessionIDs join is the proof-of-concept)
- Adding GitHub fields to `UnfinishedWorktree` proto is additive, not a schema change
- Poller can be extended to handle branch-only (no-session) worktrees by accepting a
  `[]ScanResult` feed in addition to `[]*Instance`

**Verdict: Option B is the correct architecture.** It reuses the existing poller infrastructure,
keeps the scanner pure, and puts enrichment exactly where the existing code already aggregates
data (service layer).

---

## 6. "GitHub PRs Section" — Listing All User Open PRs

This is a distinct feature from PR status enrichment on worktrees/sessions. It requires fetching
PRs authored by the current user across all repos, not just repos tracked by the scanner.

### Where Does This Data Live?

**Recommended architecture**: A new `UserPRCache` struct in the `github` package, with:
- In-memory store: `map[string]*PRInfo` keyed by `"owner/repo#number"`
- Refresh loop: single background goroutine, no per-repo polling
- Fetch method: `gh api search/issues?q=is:pr+is:open+author:@me` (one API call returns all)
- Cache TTL: 5 minutes (configurable)
- ETag support: the search endpoint supports ETag; HTTP 304 means no quota cost on unchanged

### Proto / Service

New proto message in `proto/session/v1/`:

```protobuf
message UserPR {
  string owner       = 1;
  string repo        = 2;
  int32  number      = 3;
  string title       = 4;
  string html_url    = 5;
  string state       = 6;
  string priority    = 7;  // reuse DerivePRPriority()
  string head_ref    = 8;
  bool   is_draft    = 9;
  string check_conclusion    = 10;
  int32  approved_count      = 11;
  int32  changes_req_count   = 12;
  google.protobuf.Timestamp updated_at = 13;
  // Whether a local session exists for this PR's branch
  repeated string session_ids = 14;
  // Whether a local worktree exists for this PR's branch (no session required)
  string local_worktree_path = 15;
}

service GitHubUserService {
  rpc ListUserPRs(ListUserPRsRequest) returns (ListUserPRsResponse) {}
  rpc WatchUserPRs(WatchUserPRsRequest) returns (stream UserPREvent) {}
}
```

This is separate from `GitHubService` (which is session-centric) and
`UnfinishedWorkService` (which is worktree-centric). The `ListUserPRs` data is
GitHub-centric and can link back to sessions/worktrees as annotations.

### Cache TTL Recommendation

| Scenario | Recommended TTL |
|---|---|
| Initial load / cache miss | Fetch immediately |
| Subsequent polls | 5 minutes |
| After user action (new PR, merge) | Invalidate and re-fetch |
| ETag 304 hit | Reset TTL, no quota cost |
| Rate limit backoff | 60 seconds minimum |

---

## 7. Implementation Sequence

Given the existing architecture, the correct order is:

1. **Add `github_pr_*` fields to `UnfinishedWorktree` proto** (fields 21–27) — no breaking change
2. **Extend `UnfinishedWorkService`** to join scan results with Instance PR data (existing
   `sessionPathIndex()` pattern, add `instancePRIndex()`)
3. **Extend `PRStatusPoller`** (or create `WorktreePRPoller`) to handle worktrees without
   sessions — accepts `[]ScanResult` feed
4. **Add `UserPRCache`** in `github/` package with `gh search` endpoint
5. **Add `GitHubUserService`** in `server/services/` with `ListUserPRs` + `WatchUserPRs`
6. **New proto**: `UserPR` message + `GitHubUserService` service definition

Steps 1–3 enrich the existing Unfinished tab with PR status.
Steps 4–6 add the new "My Open PRs" section.

---

## Summary

- `ScanResult` is pure git; `PRStatusPoller` handles GitHub; `UnfinishedWorkService` is the
  right merge point — it already joins scanner output with instance data (proven by `SessionIDs`)
- `session/pr_status_poller.go` is fully implemented (399 lines) with ETag cache, rate limits,
  and auto-discovery, but only covers sessions — not bare worktrees
- Option B (enrichment at API layer) is definitively correct: scanner stays pure, poller stays
  reusable, service layer aggregates exactly as it already does for session linkage
