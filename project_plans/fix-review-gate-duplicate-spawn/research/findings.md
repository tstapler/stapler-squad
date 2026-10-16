# Research findings (VERIFIED by code read, 2026-10-05; no runtime repro yet)

- `ReviewGateRunner.Run` (session/review_gate.go ~L257-455) has no existing-review check: it computes diff, calls `SpawnReviewSession`, and only then `CreateItemSession(role=review)` (L443). Window = diff computation + session spawn (seconds).
- `FindReviewItemsWithoutGate` (session/storage_backlog.go:1028) excludes items with ANY review ItemSession row, so it is safe once the row exists, but blind during the window above. A tick landing in the window re-spawns. Also two overlapping reconcile ticks each spawn in goroutines.
- `onSessionExited` (backlog_lifecycle.go ~L940) is gated by an optimistic-precondition status transition in_progress->review (only one winner) — guards double exit, not cross-entry-point.
- `TriggerReviewForSession` (backlog_lifecycle_review.go:270) checks only SkipReviewGate; no status check, no existing-review check. Call sites: server/mcp/tools_backlog.go:1407, autonomous_orchestration_service.go:569/83.
- `reviewSem` (cap 8) only bounds concurrency; the semaphore is released when `Run` returns, so it does not cover the row-persist gap either.
- `AutoRespawnReview` (server/services/backlog_service_triage.go:2408) checks `findActiveReviewSession` from persisted rows — same row-lag blind spot, and in a different package, so cannot see an in-memory listener reservation unless exposed via an interface.
- Related prior art: project_plans/review-gate-stale-session-rework; memory note on check-then-reserve needing a process-wide lock.

## Design options
A. In-memory `inflightReview map[itemID]struct{}` + one `sync.Mutex` on the listener (reserve in `spawnReviewGate`, release by defer) plus DB check for open review row inside the same critical section. Simple; lost on restart but the DB row check covers post-persist state.
B. DB-level uniqueness (partial unique index on item_id where role=review and ended_at null): durable, but needs row before spawn (reorder: create placeholder row first). Larger migration; ent schema change.
Chosen: A, with the guard exposed via a small interface method so BacklogService re-review paths can call it. Note A is single-process only; acceptable (one server per state dir).
