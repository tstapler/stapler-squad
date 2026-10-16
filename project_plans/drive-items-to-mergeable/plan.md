# drive-items-to-mergeable — Implementation Plan

**Date:** 2026-10-11  
**Status:** Ready for implementation

---

## Scope

Three self-contained fixes, shipping as one PR:

1. **Backend correctness** — Add `CIPending bool` to `git.PRStatus`; populate it in
   `parsePRStatusPayload`; thread it through `reconcilePRPendingItem` and
   `prReadyToMergeSolo` so pending CI blocks the "ready to merge" signal.

2. **Backend observability** — Upgrade `IsPRMerged` error log from DEBUG to WARNING.

3. **Frontend UX** — `PullRequestSection.tsx` shows a dynamic status line derived from
   `item.stuckReasons` instead of the hardcoded "waiting for merge".

---

## Phase 1 — Backend: `CIPending` correctness

### 1a. Extend `git.PRStatus`

File: `session/git/worktree_git.go`

Add `CIPending bool` to `PRStatus` (beside `CIFailing`):
```go
type PRStatus struct {
    CIFailing             bool
    CIPending             bool   // NEW: any check is in_progress/queued with no conclusion
    HasConflicts          bool
    ...
}
```

### 1b. Populate `CIPending` in `parsePRStatusPayload`

In the `statusCheckRollup` iteration that already sets `CIFailing`:
```go
for _, check := range payload.StatusCheckRollup {
    conclusion := strings.ToUpper(check.Conclusion)
    status := strings.ToUpper(check.Status)
    switch conclusion {
    case "FAILURE", "TIMED_OUT", "CANCELLED":
        result.CIFailing = true
    case "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE":
        result.CIFailing = true
    case "":
        // No terminal conclusion yet — check if actively running.
        if status == "IN_PROGRESS" || status == "QUEUED" || status == "PENDING" {
            result.CIPending = true
        }
    }
}
```

Note: `ACTION_REQUIRED`, `STARTUP_FAILURE`, `STALE` are also treated as failures (this
is a latent gap — they are not currently caught, which could also cause premature
`pr_ready_unmerged`; fixing them here is low-risk, same code path).

### 1c. Thread `CIPending` into `prReadyToMergeSolo`

Option A (preferred — minimal blast radius): in `reconcilePRPendingItem`'s healthy
branch, pass `CIPending` via `github.PRInfo`:
```go
// Add CIPending field to github.PRInfo (github/types.go or equivalent):
type PRInfo struct {
    ...
    CIPending bool
}

// In reconcilePRPendingItem healthy branch:
info := &github.PRInfo{
    ...
    CIPending: prStatus.CIPending,
}
if prStatus.CIFailing {
    info.CheckConclusion = "failure"
}
```

Then in `prReadyToMergeSolo`:
```go
if info.CIPending {
    return false   // CI not done yet — don't declare ready
}
switch info.CheckConclusion {
case "success", "":
    // "" now only reached when CIPending=false (no CI configured)
```

Option B (alternative): pass `CIPending` as a second argument to `prReadyToMergeSolo`.
This avoids touching `github.PRInfo` but makes the function signature less ergonomic
for future callers. Use Option A.

### 1d. Unit tests

File: `session/git/worktree_git_test.go` (or the test file that covers `parsePRStatusPayload`)

Fixture 1: `statusCheckRollup` with `status="IN_PROGRESS"`, `conclusion=""` →
assert `CIPending=true, CIFailing=false`.

Fixture 2: `statusCheckRollup` with `status="COMPLETED"`, `conclusion="FAILURE"` →
assert `CIPending=false, CIFailing=true`.

Fixture 3: mixed — one in-progress check, one completed success →
assert `CIPending=true, CIFailing=false` (in-progress dominates over success).

File: `session/stuck_decisions_test.go`

Test: `prReadyToMergeSolo` returns `false` when `CIPending=true`, `Mergeable="MERGEABLE"`.

File: `session/backlog_lifecycle_pr_test.go` (or the existing reconciler test file)

Integration smoke test: reconciler healthy branch with `CIPending=true` item does NOT
call `markPRReadyUnmerged`.

---

## Phase 2 — Backend: Observability

File: `session/backlog_lifecycle_pr.go`

Change line 1465:
```go
// Before:
log.DebugLog().Printf("[BacklogLifecycle] ReconcilePRPending IsPRMerged item=%s pr=%d: %v", ...)

// After:
log.WarningLog().Printf("[BacklogLifecycle] ReconcilePRPending IsPRMerged item=%s pr=%d: %v", ...)
```

Rationale: `IsPRMerged` failure causes a silent no-op for the entire tick. WARNING
gives operators a searchable signal without being ERROR-level noise (errors are
transient network glitches, not data corruption).

---

## Phase 3 — Frontend: Dynamic PR status

File: `web-app/src/components/backlog/detail/PullRequestSection.tsx`

Replace the hardcoded label with a computed value:

```tsx
function prStatusLabel(stuckReasons: string[]): string {
  if (stuckReasons.includes("pr_needs_fix")) {
    return "CI failing or conflict detected — fix in progress";
  }
  if (stuckReasons.includes("pr_ready_unmerged")) {
    return "Ready to merge — awaiting action";
  }
  return "Waiting for merge";
}

// In the component:
<span className={styles.reviewContextLabel}>
  PR #{item.prNumber} — {prStatusLabel(item.stuckReasons)}
</span>
```

This requires no new API surface: `item.stuckReasons` is already in `BacklogItem`.

Add a Jest unit test in `web-app/src/` (co-located with the component or in
`__tests__/`) confirming the three label variants.

---

## Adversarial Review

**Pre-mortem 1: `CIPending` and `CIFailing` both true simultaneously.**
This can't happen with the proposed fix: once a conclusion of FAILURE is set,
`CIFailing` is true and `CIPending` is irrelevant (the healthy branch is bypassed).
If a check finishes with failure while another is still pending, `CIFailing=true`
takes the unhealthy branch regardless — `CIPending` is only consulted in the healthy
branch where `CIFailing=false`.

**Pre-mortem 2: `status="PENDING"` casing.**
The GitHub API returns `status` in UPPER_CASE (`IN_PROGRESS`, `QUEUED`); `PENDING` is
a legacy Status API value. All three are handled in the switch above. The existing
`parsePRStatusPayload` upcases via `strings.ToUpper` before its own checks — use the
same pattern for consistency.

**Pre-mortem 3: Empty `statusCheckRollup` (no CI configured).**
With empty rollup, the loop body never runs → `CIPending=false, CIFailing=false`.
`prReadyToMergeSolo` reaches the `case "success", "":` arm exactly as before — no
regression for repos with no CI.

**Pre-mortem 4: `ACTION_REQUIRED` added to CIFailing.**
`ACTION_REQUIRED` is used by GitHub Apps that require a human action (e.g., a
required approval). Treating it as `CIFailing` could spawn an unnecessary fix session.
**Resolution**: leave `ACTION_REQUIRED`, `STARTUP_FAILURE`, `STALE` for a separate
item; don't include them in this PR. Keep the change minimal to `CIPending` only.

**Pre-mortem 5: Frontend `stuckReasons` includes `multiple_reasons`.**
A label check using `includes("pr_needs_fix")` still works even when
`multiple_reasons` is ALSO in the array — `includes` is an exact-match substring
check on the string value, not the array. No issue.

**Pre-mortem 6: `prReadyToMergeSolo`'s `CIPending` check ordering.**
The function currently has an early-return pattern. The new `CIPending` check must be
added BEFORE the `CheckConclusion` switch to avoid falling through to `success/""` on
a pending-CI `PRInfo` where `CheckConclusion=""`.

---

## Risk Assessment

| Risk | Severity | Likelihood | Mitigation |
|---|---|---|---|
| Repos with no CI break (empty rollup) | High | Low | Empty loop → CIPending=false, existing behavior preserved |
| `CIPending` stays true forever (stale check) | Medium | Low | GitHub eventually sets a conclusion; Stale conclusion not yet handled (separate item) |
| Frontend shows wrong label due to stale stuckReasons | Low | Low | stuckReasons updated by WatchBacklogItems stream on next tick |
| WARNING log noise for transient gh CLI errors | Low | Medium | Acceptable — operators need visibility; one log per 60s tick max |

---

## Definition of Done

- [ ] `git.PRStatus.CIPending` field added, `parsePRStatusPayload` populates it
- [ ] `prReadyToMergeSolo` (via `github.PRInfo.CIPending`) returns `false` for pending CI
- [ ] `IsPRMerged` failure log upgraded to WARNING
- [ ] Unit tests: `parsePRStatusPayload` (pending CI fixture), `prReadyToMergeSolo` (pending CI),
      reconciler healthy branch (pending CI → no markPRReadyUnmerged call)
- [ ] `PullRequestSection.tsx` shows dynamic status from stuckReasons
- [ ] Frontend Jest test for `prStatusLabel`
- [ ] `make ci` passes (lint, test, build)
- [ ] `make ready` passes (duplication gates)
- [ ] Feature registry updated (`make registry-generate`)
