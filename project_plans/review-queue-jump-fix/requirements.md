# Requirements: review-queue-jump-fix

**Date**: 2026-06-22
**Type**: bug fix

## Problem Statement

When a user opens a session from the review queue list (clicking a row), the review queue modal immediately jumps to the next queue item after the user clicks into the session terminal. This makes the review queue unusable for reviewing and interacting with sessions.

The bug is triggered by a race between two systems:
1. The review queue panel filters out sessions whose `workingState` is ACTIVE or PROCESSING (to hide "actively working" sessions that don't need attention)
2. When a session's `detectedStatus` transitions to EXECUTING/ACTIVE (e.g., the backend pushes a status update via WatchReviewQueue), the session gets filtered from the visible `items` list
3. The parent page's "deleted externally" auto-advance effect watches `reviewQueueItems` (derived from the filtered visible items) — when the session disappears from this filtered list, it incorrectly fires `handleAutoAdvance(sessionId, true)`

The effect fires even though the session is still in the underlying review queue (it was just filtered from the visible items due to status transition), causing the UI to jump to the next session.

## Users / Consumers

End users (Tyler and other stapler-squad users) using the review queue feature to triage and interact with AI sessions that need attention.

## Success Metrics

- Bug is gone: opening a session from the review queue stays on that session when the user clicks into the terminal
- Regression test prevents recurrence: a test verifies that transitioning a session to ACTIVE/PROCESSING does NOT trigger auto-advance while the session is selected

## Constraints

No hard constraints — fix it correctly.

## Scope

### In Scope
- Fix the "deleted externally" auto-advance effect to use the unfiltered queue items (from `useReviewQueueContext().items`) instead of the filtered visible items (`reviewQueueItems`) to determine if the selected session is still in the queue
- Add a test that verifies the effect does NOT fire when a session is filtered from the visible queue due to status transition

### Out of Scope
- Redesigning the review queue filter logic
- Changing the auto-advance behavior for acknowledged/deleted sessions (that should still work)
- Changing the backend review queue data model

## Open Questions
- Does the backend always send `removeItem` events when a session is truly resolved? (Assumed yes for the fix to be correct)
- Should the auto-advance effect fire when the session stays in `allItems` but is removed from the visible queue for non-status reasons (e.g., filter changes)? (Assumed no — only fire on actual queue removal)
