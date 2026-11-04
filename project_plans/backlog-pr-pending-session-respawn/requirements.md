# Requirements: Backlog PR-Pending Session Respawn

**Item ID:** c2940dff-35ad-4a12-9a55-0c0af4573620  
**Date:** 2026-11-02  

## Problem Statement

When a backlog item reaches `pr_pending` status and its underlying work session is subsequently deleted, there is no user-accessible path to spawn a new work session to continue fixing the PR. The UI shows "Session not found." in the work session panel, but provides no button to create a replacement session. The only available backward transitions (`send_back_idea`, `send_back_ready`) destroy the item's PR context.

The item in the screenshot also shows a `multiple_reasons` stuck state — an aggregate synthetic reason that the backend explicitly has no remediation action for. Clicking "Retry now" on a `multiple_reasons` stuck item returns `CodeUnimplemented`.

### Root Causes

1. `getAvailableActions()` in `itemActions.ts` deliberately adds no `restart_session` or `spawn_session` for `pr_pending` status — the comment says "No status-specific primary action today — only backward transitions."
2. `JumpToSessionPanel` shows "Session not found." but has no spawn button.
3. `remediationActionByReason()` in `backlog_service_stuck.go` falls through to `default: return nil` for `StuckReasonMultipleReasons`, returning `CodeUnimplemented`.
4. The constituent stuck reasons underlying `multiple_reasons` are individually addressable, but the `multiple_reasons` UI card gives no per-reason drill-down into those constituent actions.

## User Story

> As a user with a `pr_pending` backlog item whose session has been deleted, I want a direct UI path to spawn a new work session so the agent can continue fixing the PR without losing the `pr_pending` context or having to restart from `ready`.

## Acceptance Criteria

1. When a `pr_pending` backlog item has no active work session (session is deleted/missing), a "Restart work" action is available in the backlog item detail view.
2. Triggering "Restart work" from `pr_pending` spawns a new work session with `force=true`, keeping the item in `pr_pending` status (not sending it back to `in_progress` or `ready`).
3. The new work session receives sufficient context about the existing PR branch/number so the agent can pick up where the previous session left off.
4. The `JumpToSessionPanel` "Session not found." state includes a "Spawn new session" button (or a navigation link to the action that does so) so users aren't stranded on that view.
5. The `multiple_reasons` stuck item UI displays the constituent stuck reasons with their individual remediation buttons, so users can address each one without needing to understand the aggregate label.
6. The backend `TriggerRemediationNow` RPC for `multiple_reasons` either: (a) dispatches all constituent reason remediations, or (b) returns a clearer error message listing the constituent reasons rather than a generic `CodeUnimplemented`.

## Out of Scope

- Auto-merging the PR or any GitHub PR state changes  
- Changing the bounce cap or rework cap mechanics  
- Modifying the `multiple_reasons` de-escalation threshold  
