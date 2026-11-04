# Validation: Backlog PR-Pending Session Respawn

## Plan Completeness Checklist

### Does the plan address all ACs from requirements.md?

| AC | Plan Coverage |
|----|--------------|
| AC1: "Restart work" action available when `pr_pending` + no active session | Task 3 adds `restart_session` to `pr_pending` unconditionally in `getAvailableActions()` ✓ |
| AC2: Triggering keeps item in `pr_pending` context (not backward) | Task 1+2: `force=true` → `forceResetItem` transitions to `in_progress`, then spawn picks up from there. Item never goes to `ready`. ✓ |
| AC3: New session receives PR branch context | `SpawnSessionFromItem` uses existing branch on the item; notes are preserved through force reset. ✓ (no new work needed) |
| AC4: `JumpToSessionPanel` "Session not found." has a spawn button | Task 4 adds "Restart work" button with `onRestartSession` prop ✓ |
| AC5: `multiple_reasons` UI shows constituent reasons with per-reason buttons | Task 5 (proto + backend + frontend) ✓ |
| AC6: `TriggerRemediationNow` for `multiple_reasons` returns better error or dispatches all | Task 6 ✓ |

### Does the plan have any unaddressed risks?

1. **Rework cap bypass** — Acknowledged in adversarial review. If operators need cap enforcement on user-initiated restarts, insert `BumpReworkCount` in `forceResetItem`. Currently not required based on existing `review` force-reset behavior.

2. **`pr_pending` → `in_progress` transition validity** — `GatedTransition` with `Unconditional: true` bypasses the normal FSM guard. Verify that no audit-trail or state machine invariant requires `pr_pending → in_progress` to be gated. `AutoReopenForPRFix` already uses this exact transition path, so it's a proven path.

3. **`restart_session` showing when session exists and is live** — Safe because `force=true` stops live sessions first. No deadlock risk since `stopLiveWorkAndReviewSessions` is non-blocking.

4. **`constituent_reasons` proto field performance** — Requires a secondary DB query per `StuckBacklogItem` in the list response. Add a batch join query to avoid N+1.

### Open Questions

1. Should `restart_session` on a `pr_pending` item with an *existing live session* stop that session before spawning? Currently `force=true` does stop it (`stopLiveWorkAndReviewSessions`) — but this is not communicated to the user in the UI. The button label "Restart" implies stopping; consider a confirmation dialog if a live session exists.

2. Should the new work session spawned from `pr_pending` force-restart be `autonomous=true` (like `AutoReopenForPRFix`) or `autonomous=false` (like other user-initiated restarts)? Current plan inherits the `autonomous` flag from the `SpawnSessionFromItem` call, defaulting to `false` for user-initiated spawns.

3. Task 5 (constituent reasons) is higher scope. Is it in scope for this item or should it be a follow-up? The item's immediate blocker is the session respawn (Tasks 1-4), not the `multiple_reasons` display gap.

## Scope Validation

- Tasks 1-4 + 7 (core session respawn): ~2-3 days implementation, low risk
- Tasks 5-6 (constituent reasons + better error): ~1-2 additional days, medium scope due to proto change
- Recommend shipping Tasks 1-4 first as a focused fix; Tasks 5-6 as a follow-up improvement

## Test Coverage Gaps

- No existing test for `SpawnSessionFromItem` with `pr_pending` status (confirmed: no test case in `backlog_service_triage_test.go` exercises this path)
- `JumpToSessionPanel` has no test for "Session not found." state with a spawn CTA
- Both gaps should be filled in Task 7 before shipping
