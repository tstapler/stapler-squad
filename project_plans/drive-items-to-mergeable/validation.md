# drive-items-to-mergeable — Validation

**Date:** 2026-10-11

---

## Requirements Coverage

| AC | Covered by plan | Notes |
|---|---|---|
| AC-1: CIPending prevents premature pr_ready_unmerged | Phase 1 (a–d) | Unit tests for parsePRStatusPayload + prReadyToMergeSolo + reconciler healthy branch |
| AC-2: Detection failures at WARNING level | Phase 2 | IsPRMerged error log upgraded |
| AC-3: UI shows actual PR health state | Phase 3 | stuckReasons-driven label + Jest test |

All three acceptance criteria are covered by the plan.

---

## Open Questions

**Q1: Should `CIPending=true` also suppress the `pr_ready_unmerged` *resolve*?**

The healthy branch currently resolves `pr_ready_unmerged` when `prReadyToMergeSolo`
returns false:
```go
} else {
    l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRReadyUnmerged, "ReconcilePRPending")
}
```
With the fix, `prReadyToMergeSolo` returns `false` when CI is pending, so
`pr_ready_unmerged` is resolved every tick while CI is pending. This is correct
behavior — the item is NOT ready, so the stuck reason should not be open.

No issue here.

**Q2: Can `CIPending` and `HasConflicts` be true simultaneously?**

Yes — a PR can have conflicts AND in-progress checks. The guard at line 1675 is:
```go
if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts && !hasNewFeedback {
```
`HasConflicts` takes the unhealthy branch regardless of `CIPending`. The `CIPending`
fix only matters in the healthy branch. No interaction issue.

**Q3: Is `github.PRInfo.CIPending` the right place to carry this field?**

`github.PRInfo` is already used as the argument to `prReadyToMergeSolo`. Adding
`CIPending` to it is the least-invasive path. `DerivePRPriority` (the other consumer
of `PRInfo`) already handles CI-pending via its own `PRPriorityPending` check and
doesn't read `CheckConclusion` at all — no regression risk.

**Q4: Should the reconciler use `WorktreePRPoller` data instead of `gh pr view`?**

`WorktreePRPoller` has ETag-cached REST API calls and is already running for these
worktrees. Wiring its data into `ReconcilePRPending` would eliminate the duplicate
CLI calls and get caching for free. This is a worthwhile efficiency improvement but
is architecturally non-trivial (it would require passing the poller into
`BacklogLifecycleListener`, creating a new interface, and changing the
`prPendingCheckerFactory` pattern). Deferred to a follow-up item — not a blocker for
the correctness fix.

---

## Verdict

Plan is sound. Three phases are independent and can be reviewed/merged as one PR or
split into two (backend + frontend). The adversarial review in `plan.md` pre-mortems
the four highest-risk cases; all are resolved. No blockers identified.

Estimated scope: small-to-medium. The correctness fix (Phase 1) touches 3 files with
localized changes; the observability fix (Phase 2) is a one-line change; the frontend
fix (Phase 3) is ~20 lines + a test.
