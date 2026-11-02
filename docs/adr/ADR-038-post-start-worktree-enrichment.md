# ADR-038: Post-start worktree enrichment instead of timeouts on the load path

- Status: Accepted
- Context PR: #1380

## Context

`Storage.LoadInstances` called `fromInstanceData`, which probed each session's path synchronously: `DetectWorktree` (git config read), an `os.Stat` of the worktree, and a GitHub owner/repo backfill. A session repo under a TCC/iCloud-protected directory blocks `open()` in the kernel until the user answers a consent prompt, so one such session held startup for 5+ minutes. A blocked `open()` cannot be cancelled by a context or a deadline.

## Decision

`LoadInstances` restores purely from persisted data and touches no session path. The probes move to `session.WorktreeEnricher`, which runs after instances exist:

- A bounded pool (4 workers, reusing `session/scanqueue`) drains one task per path; sessions sharing a path share one probe.
- `golang.org/x/sync/singleflight` keeps a blocked path to one goroutine. A probe that outlives its 10s deadline releases its worker, is logged as stuck, and is retried with exponential backoff (5s to 5m); the retry joins the blocked call, so no goroutine piles up.
- Results are applied only through the instance actor and setters, then persisted with `Storage.UpdateInstance`. Archived and terminal sessions are never enqueued or touched, and nothing here starts or revives a session (`.claude/rules/noarchivedrevival.md`).
- Server start (`BuildRuntimeDeps` Step 6) awaits each instance's first attempt, capped at 12s, before `Start(false)`, so the missing-worktree pause still lands before the session starts. A stuck session proceeds to start after the cap; unknown is not treated as missing.
- Metrics: `worktree_enrichment_total{result=ok|late|timeout}`, gauges `worktree_enrichment_queued`, `_running`, `_stuck_paths`.

## Why not a timeout around the probe

A goroutine-plus-timeout leaves the call blocked and detached, so you need marker maps to avoid piling up more, and a late result has to be applied from a detached goroutine to an instance the load goroutine is still reading. Moving the work off the load path removes the shared-state race, because nothing reads the instance while it is applied, and lets the stuck call occupy one deduplicated goroutine instead of the startup critical path.

## Consequences

- A session on a stuck path loads without GitHub info and, if its worktree is gone, is not auto-paused until a retry succeeds. A late pause is applied only to a not-yet-started instance.
- A late result is applied only if the instance is live (has an actor); a never-started paused session on a path that timed out is enriched after it next starts.
- Other `LoadInstances` callers (MCP tools, RPC handlers, health ticks) no longer re-derive the missing-worktree pause from the filesystem; they read the status persisted at boot.
- Unbounded callers outside boot remain: `NewInstance` (session creation), `WorkspacePeersBlockForPath`, and `prwatch` `detectRemote` (background poller).
- A stuck probe still holds one OS thread in `open()` until the kernel returns.

## Extension point

`WorktreeEnricherConfig.Ready(path) bool` parks a path (tasks stay registered, the Await barrier is released) and `Unpark(path)` requeues it. It is a no-op today; the planned TCC consent gate (`STAPLER_SQUAD_TCC_GATE`) plugs in here without changing the queue.
