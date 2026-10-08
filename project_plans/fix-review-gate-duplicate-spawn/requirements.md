# Requirements: fix-review-gate-duplicate-spawn

## Problem
Review reviewers for one backlog item can be spawned more than once. Entry points (all reach `ReviewGateRunner.Run`, `session/review_gate.go`, via `spawnReviewGate`):
1. `onSessionExited` (`session/backlog_lifecycle.go` ~L940)
2. `TriggerReviewForSession` (`session/backlog_lifecycle_review.go:270`; MCP `request_review`, autonomous driver)
3. `ReconcileStuckItems` re-spawn via `FindReviewItemsWithoutGate` (`session/backlog_lifecycle.go` ~L1190)
Separately, the headless/interactive re-review path (`AutoRespawnReview`, `TriggerReReview` in `server/services/backlog_service_triage.go`) has its own check (`findActiveReviewSession`) that is not shared with the above.
Only bound today: global `reviewSem` (cap 8), not per item. Each duplicate is a full-context session (quota waste, conflicting verdicts).

## Acceptance criteria
0. A reproducing test (two concurrent triggers for one item, incl. onSessionExited + reconcile tick) shows today's duplicate spawn, then passes after the fix.
1. At most one review spawn per item is in flight at any time, across all three listener entry points.
2. The guard is check-then-reserve under a process-wide lock (not per-item lock alone) and is released on every exit path (spawn error, early return, panic).
3. The reservation covers the window between `SpawnReviewSession` returning and the review ItemSession row being persisted.
4. `TriggerReviewForSession` no-ops when the item already has an open (EndedAt nil) review session or an in-flight reservation, and when item is not in `review`.
5. The headless re-review paths (`AutoRespawnReview`, `TriggerReReview`) honor the same guard.
6. Skipped duplicates are logged at info level; a legitimate re-review after the prior reviewer ended still spawns.
7. `make ready` passes (lint, dupl gate, race tests).
