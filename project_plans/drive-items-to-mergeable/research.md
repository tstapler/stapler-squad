# drive-items-to-mergeable — Research

**Date:** 2026-10-11  
**Status:** Complete

---

## Architecture Overview

The backlog item PR-pending lifecycle uses two distinct paths:

| Path | Owner | API method | ETag-cached? | Coverage |
|---|---|---|---|---|
| `PRStatusPoller` | `session/pr_status_poller.go` | `GetPRInfoConditional` (REST) | Yes | Live session instances only; skips suspended sessions |
| `ReconcilePRPending` | `session/backlog_lifecycle_pr.go` | `gh pr view` (CLI) | No | Backlog items in `pr_pending` whose work session has ended |
| `WorktreePRPoller` | `session/worktree_pr_poller.go` | `GetPRInfoConditional` (REST) | Yes | Worktrees with no active session — covers the gap between the two above |

When a work session finishes and the item transitions to `pr_pending`, the session
becomes suspended (`IsSuspended() == true`), so `PRStatusPoller` skips it. Detection
then falls entirely to `ReconcilePRPending` on a 60-second ticker.

`WorktreePRPoller` was added to cover the ETag-cached REST path for the same
worktrees, but its data (`GetPRData(repoPath, branch)`) is **not wired** into
`reconcilePRPendingItem` — that function independently shells out to `gh pr view`.

---

## Root Cause Analysis

### Primary bug: `CIPending` not tracked

**Code path:**

1. `parsePRStatusPayload()` (`session/git/worktree_git.go:926`):
   - `CIFailing` = true if ANY check in `statusCheckRollup` has conclusion in
     `{FAILURE, TIMED_OUT, CANCELLED}` or state `FAILURE/ERROR`.
   - `CIPending` field: **does not exist**.
   - A pending check has `status: "in_progress"` and `conclusion: null/""` → neither
     `CIFailing` nor any pending marker is set.

2. `reconcilePRPendingItem` healthy branch (`backlog_lifecycle_pr.go:1675`):
   ```go
   if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts && !hasNewFeedback {
       info := &github.PRInfo{
           ...
           Mergeable: prStatus.Mergeable,
       }
       if prStatus.CIFailing {
           info.CheckConclusion = "failure"
       }
       // info.CheckConclusion = "" when CI is pending
       if prReadyToMergeSolo(info) {  // ← returns true for pending CI
           l.markPRReadyUnmerged(...)
       }
   ```

3. `prReadyToMergeSolo()` (`session/stuck_decisions.go`):
   ```go
   switch info.CheckConclusion {
   case "success", "":
       // "" treated as "no CI configured" — BUG: also matches pending CI
   ```

**Causal chain for the screenshot:**
1. PR created, CI starts running (checks: `status=in_progress, conclusion=""`)
2. `ReconcilePRPending` detects: no failures, no conflicts → healthy branch
3. `info.CheckConclusion = ""` (not set) → `prReadyToMergeSolo` returns `true`
4. After 30min (`prReadyThreshold`): `pr_ready_unmerged` fired and open
5. CI finishes and FAILS
6. Next tick: `CIFailing = true` → unhealthy branch → `pr_ready_unmerged` resolved,
   `pr_needs_fix` set
7. During the one-tick window (up to 60s) where both are open, `multiReasonThreshold=2`
   is met; after a 60s dwell (`multiReasonNotifyDwell`), `multiple_reasons` fires
8. `multiple_reasons` persists until self-heal sweep runs (which requires ALL open
   non-escalation reasons to be resolved, which happens on the very next tick)

In the screenshot: "18m" of `multiple_reasons` suggests either:
- The self-heal sweep was delayed, OR
- The stuck reasons kept re-opening (e.g., `pr_needs_fix` keeps firing because no
  fix session has started yet), keeping `multiple_reasons` alive

### Secondary bug: Silent `IsPRMerged` failure

`backlog_lifecycle_pr.go:1464`:
```go
if mergedErr != nil {
    log.DebugLog().Printf(...)  // ← operator-invisible
    return
}
```
If `gh` CLI fails (auth expired, network error, wrong binary path), every 60-second
tick silently returns without detection. The item stays in `pr_pending` indefinitely
with no signal.

The `GetPRStatus` failure at line 1479-1481 IS logged at `WARNING` level (for the
ship-snapshot path), but the main detection path's `GetPRStatus` error (line 1449
factory + implied failure) also returns without surfacing — however this is partly
because `IsPRMerged` acts as a guard before `GetPRStatus` is called. Both need
WARNING-level coverage.

### UI gap: Static PR status display

`PullRequestSection.tsx:51` always renders:
```tsx
PR #{item.prNumber} — waiting for merge
```

The `BacklogItem` React type includes `stuckReasons: string[]` populated from
`ListBacklogItems`/`WatchBacklogItems`. When `pr_needs_fix` is in `stuckReasons`,
the UI has enough information to show "CI failing" — no new API surface needed.

---

## Existing Plan Overlap

`project_plans/backlog-pr-mergeability-policy/` describes the `CIPending` fix as
"Blocker-1" with the full fix in Phase 3. That plan is status "Ready for implementation"
but NOT shipped. This item scopes down to just the correctness fix (CIPending, logging)
and the UI display — NOT the full AutoMergePolicy/auto-merge automation from that plan.

`project_plans/backlog-pr-conflict-detection/` introduced `HasConflicts` (already
shipped per code inspection). That plan is complete.

---

## `statusCheckRollup` JSON schema (GitHub)

Relevant fields per GitHub API docs (confirmed by inspecting `parsePRStatusPayload`):

```json
{
  "statusCheckRollup": [
    {
      "name": "CI / build",
      "status": "IN_PROGRESS",       // QUEUED | IN_PROGRESS | COMPLETED
      "conclusion": null,            // null while pending; FAILURE/SUCCESS/etc when done
      "state": null                  // for legacy Status API checks
    }
  ]
}
```

Pending detection: `status == "IN_PROGRESS"` OR `status == "QUEUED"` AND `conclusion == ""/"null"`.

`parsePRStatusPayload` already iterates `statusCheckRollup` for the `CIFailing` check;
the `CIPending` check is an additional branch in the same loop.

---

## `github.PRInfo` downstream consumers

`prReadyToMergeSolo` is the only consumer of `CheckConclusion` on the backlog path.
The `PRStatusPoller` path uses `github.DerivePRPriority()` which has its own
`CIPending` handling via `PRPriorityPending` (already correct). No regression risk
from adding a `CIPending` field to `PRInfo` — `DerivePRPriority` doesn't read it.
