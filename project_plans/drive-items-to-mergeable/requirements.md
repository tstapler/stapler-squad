# drive-items-to-mergeable — Requirements

**Item title:** Drive items to mergeable  
**Item ID:** f8b61186-b4d2-4597-ba84-facd12e4dca2  
**Date:** 2026-10-11

---

## Problem Statement

The backlog item system is not correctly detecting when pull requests have failing CI
checks or merge conflicts, and it emits false-positive "ready to merge" signals when CI is
still pending. The result is items being flagged with contradictory stuck reasons
simultaneously — most visibly `pr_ready_unmerged` + `pr_needs_fix` → escalation to
`multiple_reasons` (confirmed in a live screenshot, 18m duration, "+2 more").

Specific gaps identified through codebase inspection:

### Gap 1 — CI-pending treated as CI-passing (`prReadyToMergeSolo`)

`parsePRStatusPayload()` (`session/git/worktree_git.go:926`) populates `git.PRStatus`.
It sets `CIFailing = true` only for terminal-failure conclusions
(`FAILURE`, `TIMED_OUT`, `CANCELLED`). There is no `CIPending` field.

In `reconcilePRPendingItem`'s healthy branch (`backlog_lifecycle_pr.go:1684`), the code
synthesises a `github.PRInfo` and passes it to `prReadyToMergeSolo`. It sets
`info.CheckConclusion = "failure"` only when `prStatus.CIFailing`, leaving
`info.CheckConclusion = ""` (empty string) when CI is pending.

`prReadyToMergeSolo` (`session/stuck_decisions.go`) switches on `CheckConclusion`:
```
case "success", "":
    // green or no CI configured — proceed.
```
Empty string is treated as "no CI" → the function returns `true` when `Mergeable == "MERGEABLE"`.
A PR with in-flight CI checks is incorrectly declared ready.

After 30 minutes (`prReadyThreshold`) this fires `pr_ready_unmerged`. When CI then
finishes and fails, `pr_needs_fix` fires on the next 60-second tick. For up to 60 seconds
both stuck reasons are open simultaneously, satisfying `multiReasonThreshold = 2`, which
fires `StuckReasonMultipleReasons` after a 60-second dwell — and that escalation persists
until the self-heal sweep clears it, resulting in the "18m multiple reasons" observed.

### Gap 2 — Silent detection failures

`reconcilePRPendingItem` line 1464:
```go
if mergedErr != nil {
    log.DebugLog().Printf(...)  // ← DEBUG, invisible to operators
    return
}
```
Any `gh` CLI auth expiry, network error, or misconfiguration silently drops every
detection tick for that item. There is no WARNING-level signal or retry counter.

### Gap 3 — UI always shows "waiting for merge"

`PullRequestSection.tsx` (line 51) hardcodes:
```tsx
PR #{item.prNumber} — waiting for merge
```
for every `pr_pending` item regardless of whether CI is failing or a conflict has been
detected. The item already has `stuckReasons` available via the `BacklogItem` type —
there is no API change needed to surface this; the display logic is missing.

### What IS already working

- `HasConflicts` detection via `mergeStateStatus == "DIRTY"` OR
  `mergeable == "CONFLICTING"` (implemented in `backlog-pr-conflict-detection`)
- `CIFailing` detection via `statusCheckRollup` terminal-conclusion parsing
- `remediatePRFixWithBackoffGate()` spawning fix sessions when issues are detected
- `StuckReasonPRNeedsFix` set immediately on first detection
- `WorktreePRPoller` covering ETag-cached REST polling for worktrees without active sessions

---

## Acceptance Criteria

### AC-1 — CIPending field prevents premature pr_ready_unmerged

`parsePRStatusPayload()` sets `PRStatus.CIPending = true` when any
`statusCheckRollup` entry has a pending/in-progress/queued state and no terminal
conclusion.

`prReadyToMergeSolo` (or the check that calls it in `reconcilePRPendingItem`)
returns `false` when `CIPending` is true, regardless of `Mergeable`.

A unit test for `parsePRStatusPayload` with a fixture that has in-progress checks
confirms `CIPending = true, CIFailing = false`.

A unit test for `prReadyToMergeSolo` (or the reconciler healthy-branch path) confirms
it returns `false` when CI is pending.

### AC-2 — Detection failures surface at WARNING level

`reconcilePRPendingItem`'s `IsPRMerged` failure log is upgraded to `WARNING`.
The `GetPRStatus` failure path (currently not reached in the main detection branch
since errors cause early return) logs at `WARNING` when it occurs.

### AC-3 — UI shows actual PR health state

`PullRequestSection.tsx` renders a dynamic status label based on `item.stuckReasons`:
- When `stuckReasons` includes `pr_needs_fix`: show "CI failing — fix in progress" or
  similar (exact copy TBD).
- When `stuckReasons` includes `pr_ready_unmerged`: show "Ready to merge — pending action".
- Default (no applicable stuck reason): retain "waiting for merge".

---

## Out of Scope

- **AutoMergePolicy per-item flag**: tracked in `backlog-pr-mergeability-policy` plan
  (phases 0–6 not yet shipped); that work builds on the CIPending fix but is
  independent of it. Not included here.
- **Global kill-switch for autonomous merging**: same plan, out of scope.
- **Auto-PR creation on PASS verdict**: out of scope.
- **`WorktreePRPoller` data sharing with ReconcilePRPending**: an efficiency improvement
  that would eliminate duplicate `gh pr view` CLI calls; out of scope for this bug fix.
