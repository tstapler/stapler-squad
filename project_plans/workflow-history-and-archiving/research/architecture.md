# Architecture Decisions: Workflow History and Archiving

## Plain String workflow_id (Not FK Edge)

`workflow_id` is stored as `field.String(...).Optional()` with no ent edge to the Workflow entity. This is a deliberate orphan-safety decision: if a Workflow is deleted, its historical sessions remain intact with their `workflow_id` string preserved. An FK edge with `OnDelete(CASCADE)` or `OnDelete(SET_NULL)` would either destroy run history or require a nullable FK with migration risk. The string approach also avoids the join overhead in `ListSessions`, which is a hot path — the filter is a simple string equality check on an indexed field.

## archived_at Timestamp (Not Bool)

`archived_at` is a `*time.Time` (nillable) rather than a boolean `archived` flag. This provides three concrete benefits actually used in the implementation: (1) nil vs non-nil distinguishes "never archived" from "archived then unarchived and re-archived" without losing information; (2) the timestamp is the audit record of when archiving occurred, visible in future analytics or range queries; (3) ent's `SetNillableArchivedAt` / `ClearArchivedAt` API maps cleanly to the Go `*time.Time` type without a separate cleared/set state machine. The index on `archived_at` supports future range queries (e.g., "archive sessions older than 30 days").

## autoArchiveListener as LifecycleListener vs Polling

Auto-archive is implemented as a `LifecycleListener` registered on each workflow-spawned instance rather than a polling loop. This matches the existing architecture pattern used by `BacklogLifecycleListener`. The listener fires exactly once per session exit — no timer, no missed events between poll intervals, no wasted CPU on sessions that never exit. The `OnLifecycleEvent` handler dispatches `maybeAutoArchive` to a goroutine immediately, satisfying the non-functional requirement that auto-archive not block the exit event path. The wiring is applied at three callsites: `CreateSession`, `loadInstancesWithWiring`, and the `ListSessions` fallback load, ensuring loaded sessions get the listener regardless of the code path that brought them into memory.

## Pause Kills tmux vs Detach, and the Resume Reinit Pattern

The prior implementation detached the tmux session on pause, leaving the process tree alive in the background. The new implementation calls `KillSession()`, which terminates tmux and all child processes. This is the primary memory optimization for US-6. The kill-then-reinit contract has two halves: before kill, `wireClaudeSessionIDSavedCallback` guarantees the conversation UUID is persisted to the database; after kill, `Resume()` reconstructs the `TmuxSession` object from scratch using `buildLaunchCommand(claudeSessionID)` to inject `--resume <uuid>`. The `TmuxBackend.TmuxManager().SetSession(...)` call replaces the stale session object without allocating a new `TmuxBackend`, preserving other process manager state. The fallback to `DetachSafely()` on kill failure is intentional: a detached-but-alive tmux is better than leaving the session in an inconsistent paused state with no tmux reference.
