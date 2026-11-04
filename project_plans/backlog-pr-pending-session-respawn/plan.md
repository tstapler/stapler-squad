# Plan: Backlog PR-Pending Session Respawn

## Architecture Decision

Extend the existing `force=true` spawn path to cover `pr_pending` items rather than creating a new RPC. `AutoReopenForPRFix` already implements the correct `pr_pending → in_progress → spawn` sequence; the fix mirrors that path inside `forceResetItem` + `SpawnSessionFromItem`'s force guard. This reuses existing infrastructure with minimal blast radius.

**Not chosen:** New `ReopenPRPendingWork` RPC — adds surface area without benefit since `SpawnSessionFromItem` with `force=true` is already the user-facing contract for "restart work."

## Task Breakdown

### Task 1 — Extend `forceResetItem` to handle `pr_pending` (Backend)

**File:** `server/services/backlog_service_triage.go`

In `forceResetItem`, add a `pr_pending` branch alongside the existing `review` branch:

```go
if item.Status == string(session.BacklogStatusPRPending) {
    s.stopLiveWorkAndReviewSessions(ctx, item.ID)
    updated, transErr := s.storage.GatedTransition(ctx, session.TransitionRequest{
        Caller:      session.CallerOperatorForceReset,
        ItemID:      item.ID,
        Observed:    item,
        To:          session.BacklogStatusInProgress,
        TriggeredBy: triggeredBy,
        Unconditional: true,
    })
    if transErr != nil {
        return nil, connect.NewError(connect.CodeInternal,
            fmt.Errorf("force-resetting pr_pending item: %w", transErr))
    }
    return updated, nil
}
```

Also clear open `rework_cap`, `pr_ready_unmerged`, `push_failed` stuck rows (same as `AutoReopenForPRFix` lines 2510–2513) during the force reset, so the item doesn't re-enter stuck state immediately after spawn.

### Task 2 — Extend `SpawnSessionFromItem` force guard to include `pr_pending` (Backend)

**File:** `server/services/backlog_service_triage.go` (lines 569–580)

Change the force guard condition from:
```go
if req.Msg.Force && (item.Status == string(session.BacklogStatusInProgress) ||
    item.Status == string(session.BacklogStatusReview)) {
```
to:
```go
if req.Msg.Force && (item.Status == string(session.BacklogStatusInProgress) ||
    item.Status == string(session.BacklogStatusReview) ||
    item.Status == string(session.BacklogStatusPRPending)) {
```

This lets `force=true` reach `forceResetItem` for `pr_pending` items, which then transitions to `in_progress`, after which the existing status gate passes normally.

### Task 3 — Add `restart_session` action for `pr_pending` in frontend (Frontend)

**File:** `web-app/src/lib/backlog/itemActions.ts`

Change the `pr_pending` case:
```typescript
case "pr_pending":
  // Session may have been deleted; allow force-restart to pick up PR work.
  actions.add("restart_session");
  break;
```

`restart_session` calls `spawnSessionFromItem(item.id, { force: true })` (already implemented in `BacklogItemDetail.tsx` lines 797-799) — no changes needed in `BacklogItemDetail.tsx`.

### Task 4 — Add "Restart work" CTA to `JumpToSessionPanel` "Session not found." state (Frontend)

**File:** `web-app/src/components/backlog/detail/JumpToSessionPanel.tsx`

The component already receives `backlogItemId` as a prop. Add a prop `onRestartSession?: () => void` (optional, to avoid breaking callers that don't support it). When error is "Session not found." and `onRestartSession` is provided, render a button alongside the error:

```tsx
{!loading && error && (
  <div className={styles.panelError}>
    {error}
    {error === "Session not found." && onRestartSession && (
      <button type="button" className={styles.restartButton} onClick={onRestartSession}>
        Restart work
      </button>
    )}
  </div>
)}
```

Wire `onRestartSession` in `BacklogItemDetail.tsx` (the parent that renders `JumpToSessionPanel`) using the existing `spawnSessionFromItem` hook with `force: true`.

### Task 5 — Pass constituent stuck reasons to `StuckItemDetail` (Backend + Frontend)

**Phase A — Proto:** Add a repeated field `constituent_reasons` of type `StuckReason` to `StuckBacklogItem` in the proto definition (`proto/session/v1/session.proto`). Regenerate protos.

**Phase B — Backend:** In `backlog_service_stuck.go`'s `GetStuckItems` / `ListStuckItems` handler, when populating a `StuckBacklogItem` with `reason = MULTIPLE_REASONS`, look up the item's other open stuck rows and populate `constituent_reasons` from them.

**Phase C — Frontend:** 
- Update `mapStuckItem` in `useBacklogService.ts` to map `constituentReasons`
- Add `constituentReasons?: StuckReason[]` to `StuckItemDetailProps` in `StuckItemDetail.tsx`
- In `StuckItemDetail.tsx`, for `MULTIPLE_REASONS` case, render a list:
  ```tsx
  {isMultipleReasons && constituentReasons?.length > 0 && (
    <ul>
      {constituentReasons.map(r => (
        <li key={r}>
          {STUCK_REASON_LABELS[r]}
          {hasRemediationAction(r) && <button onClick={() => onTriggerRemediationNow(r)}>Retry</button>}
        </li>
      ))}
    </ul>
  )}
  ```

### Task 6 — Backend `TriggerRemediationNow` for `multiple_reasons`: better error (Backend)

**File:** `server/services/backlog_service_stuck.go`

Instead of returning a bare `CodeUnimplemented` for `multiple_reasons`, return a `CodeFailedPrecondition` with a message listing the underlying reasons:

```go
case domain.StuckReasonMultipleReasons:
    openRows, _ := s.storage.GetOpenStuckRows(ctx, itemID)
    constituent := filterNonEscalationReasons(openRows)
    return nil, connect.NewError(connect.CodeFailedPrecondition,
        fmt.Errorf("multiple_reasons is an aggregate: address each constituent reason individually: %v", constituent))
```

Or (better) — dispatch all constituent reason remediations sequentially and return an aggregate result.

### Task 7 — Tests (Test)

- Unit test: `SpawnSessionFromItem` with `force=true` on a `pr_pending` item spawns successfully
- Unit test: `SpawnSessionFromItem` with `force=false` on a `pr_pending` item still returns `CodeFailedPrecondition`
- Unit test: `forceResetItem` for `pr_pending` stops sessions and transitions to `in_progress`
- E2E: `pr_pending` item detail shows "Restart" button (data-testid `backlog-action-restart-session`)
- E2E: `JumpToSessionPanel` shows "Restart work" button on "Session not found." state (data-testid to be added)

## Implementation Order

1. Task 1 + 2 (backend) — unblock the spawn call
2. Task 3 (frontend itemActions) — make the action available  
3. Task 4 (JumpToSessionPanel CTA) — UX improvement for the stuck-in-panel experience
4. Task 7 (tests for tasks 1-4) — validate before shipping
5. Task 5 (constituent reasons in proto) — higher scope, separate PR
6. Task 6 (better error message for multiple_reasons) — quick win, same PR as Task 5

## Adversarial Review

**Concern 1 — Does force-resetting a `pr_pending` item lose PR context?**  
No. `force=true` transitions `pr_pending → in_progress` via `GatedTransition` but does not wipe the item's notes, branch, or PR number. `SpawnSessionFromItem` then creates a new session on the existing branch, same as `AutoReopenForPRFix`. The PR branch is preserved.

**Concern 2 — Does this bypass the rework cap?**  
Yes, intentionally. `force=true` is an explicit user action. The cap is a guard against automated runaway loops, not a block on operator/user-initiated explicit restarts. The same bypass already exists for `review → in_progress` force resets. If cap enforcement on user-initiated `pr_pending` restarts is required, a `BumpReworkCount` call can be inserted in `forceResetItem`, matching the pattern in `AutoReopenForPRFix`.

**Concern 3 — `restart_session` showing unconditionally for `pr_pending` even when a live session exists?**  
Possible: a user clicks "Restart" when a session is still technically live. `force=true` calls `stopLiveWorkAndReviewSessions` first, so this is safe. The session stop is already graceful.

**Concern 4 — Proto change for `constituent_reasons` is backward-compatible?**  
Adding a new repeated field to a proto message is additive and backward-compatible for both wire format and generated code. Old clients ignore the new field.
