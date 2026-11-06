# Research: Backlog PR-Pending Session Respawn

## Key Code Locations

### Backend — Session Spawn Gate

**`server/services/backlog_service_triage.go`**, `SpawnSessionFromItem` (lines 518+):

```go
// 3. Validate status. Allow ready (first spawn) or in_progress (re-spawn after reopen).
if item.Status != string(session.BacklogStatusReady) && !isReopen {
    return nil, connect.NewError(connect.CodeFailedPrecondition,
        fmt.Errorf("item must be in %q or %q status to spawn a session, got %q"))
}
```

`pr_pending` is hard-rejected here. `force=true` only handles `in_progress` and `review` statuses.

**`forceResetItem` (line 1366–1382):** transitions `review` → `in_progress` before spawn, stops live sessions. Does NOT handle `pr_pending`.

### Backend — Existing PR-Fix Respawn Logic

**`AutoReopenForPRFix` (line 2436):** Already implements the correct sequence for `pr_pending` items:
1. Requires `pr_pending` status
2. If no active session and cap not reached: `GatedTransition(pr_pending → in_progress)`, clears `rework_cap`/`pr_ready_unmerged`/`push_failed` stuck rows, calls `spawnPRFixSession`
3. If spawn fails: rollback to `pr_pending`

This function is the right building block — but it is only called by the automated stuck-reason reconciler (`TriggerRemediationNow → remediationActionByReason → AutoReopenForPRFix`) and only for the `pr_needs_fix` / `pr_pending_no_pr` stuck reasons. It is not user-callable in the general case.

### Frontend — Action Availability

**`web-app/src/lib/backlog/itemActions.ts`**, `getAvailableActions()`:
```typescript
case "in_progress":
  if (item.linkedSessions.length > 0) {
    actions.add("restart_session");
  }
  break;
case "review":
  actions.add("restart_session"); // unconditional — session may just be missing
  break;
case "pr_pending":
  // No status-specific primary action today — only backward transitions.
  break;
```

`pr_pending` adds zero primary actions. `review` unconditionally adds `restart_session`.

### Frontend — JumpToSessionPanel Error State

**`web-app/src/components/backlog/detail/JumpToSessionPanel.tsx`**:
- Desktop: `<div className={styles.panelError}>{error}</div>` — no button
- Mobile: portal with `<button>Close</button>` only
- No affordance to spawn a new session from the error state

### Frontend — StuckItemDetail for MULTIPLE_REASONS

**`web-app/src/components/backlog-stuck/StuckItemDetail.tsx`**:
- `MULTIPLE_REASONS` has no specific UI block — falls through to generic rows only
- `StuckItemDetail` receives only a single `StuckBacklogItem` with `reason = MULTIPLE_REASONS`
- The proto `StuckBacklogItem` has no `constituent_reasons` field
- Constituent items are available as sibling cards in `StuckItemsSection` but not passed down to `StuckItemDetail`

## Remediation Gap Summary

| Gap | Where | Fix Approach |
|-----|--------|-------------|
| `pr_pending` items cannot spawn a session | `SpawnSessionFromItem` status gate + `forceResetItem` | Extend `force=true` path to handle `pr_pending` by calling `forceResetItem` (same as `review` → `in_progress` transition) |
| No UI action for `pr_pending` without session | `itemActions.ts` `pr_pending` case | Add `restart_session` unconditionally (mirrors `review`) |
| "Session not found." has no CTA | `JumpToSessionPanel.tsx` | Add "Restart work" button near error, calls `spawnSessionFromItem(item.id, { force: true })` |
| `MULTIPLE_REASONS` UI shows no constituent remediation | `StuckItemDetail.tsx` + proto | Pass constituent reasons through; render per-reason retry links |

## Existing Patterns to Reuse

- `forceResetItem` in `backlog_service_triage.go` — already handles `review → in_progress` stop+transition; extend for `pr_pending`
- `AutoReopenForPRFix`'s PR context forwarding (`steerActiveSessionForPRFix`, prepend PR notes) — use same approach for user-initiated reopen
- `restart_session` action in `ActionsSection.tsx` (lines 331–346) — reuse for `pr_pending`; just add the action ID to `getAvailableActions`
- `otherReasonsCount`/`otherReasonLabels` already computed in `StuckItemsSection` for sibling cards — extend this to pass constituent items to `StuckItemDetail`

## Risk Assessment

- **Status guard change in `SpawnSessionFromItem`**: Low risk — `force=true` is already opt-in, callers explicitly set it
- **`forceResetItem` extension**: Low risk — `pr_pending → in_progress` transition already used by `AutoReopenForPRFix`; same `GatedTransition` path
- **Frontend action gating**: Medium risk — adding `restart_session` to `pr_pending` unconditionally changes which items show the button; need to ensure the backend extension lands first or action is gated on session being missing
- **`StuckItemDetail` constituent reasons**: Medium scope — requires proto change + backend propagation + frontend prop threading
