# ADR-038: Probe worktree info off the load path, not under a timeout

- Status: Accepted
- Context PR: #1380

## Context

`Storage.LoadInstances` -> `fromInstanceData` -> `Instance.DetectAndPopulateWorktreeInfo` -> `DetectWorktree` blocked startup for 5+ minutes. The goroutine dump showed the stack ending in the git backend Router `GetConfig` -> gogit `readRef` -> `readSmallFile` `os.Open` (`session/git/backend/gogit/refstore.go:154`), blocked in `open()` because a session's repo lived under a TCC/iCloud-protected path whose consent prompt was pending.

`DetectWorktree` (`session/repo_path.go`) does an `os.Stat` of `<path>/.git`, may `os.ReadFile` a `.git` file, then calls `gitprovider.Backend().GetConfig(...)` under a 10s context. The Router dispatches that to the gogit backend (in-process file reads, the stack in the dump, which a context cannot interrupt) or the cli backend (a git subprocess, which the context can kill) depending on cohort and fallback. I did not verify which cohort is the default.

## Finding that shapes the design

Research on this Mac (reported by the coordinator; not re-run for this ADR) found that `stat()`/`statfs()` never prompt for or block on TCC-protected paths: `open()`, `readdir()` and `access()` do, and a denial returns EPERM immediately. So the hang is the `open()`-based `DetectWorktree`, not `os.Stat`. The missing-worktree check in `fromInstanceData` is a bare `os.Stat` and stays synchronous.

## Decision

- `fromInstanceData` keeps the `os.Stat` missing-worktree pause exactly as before. Every load path (`FromInstanceData`, `FromInstanceDataDeferred`, `LoadInstances`) therefore behaves as it did, and `Start(false)` still never runs against a missing worktree. No ordering barrier is needed.
- Only the `open()`-based `DetectWorktree` moves off the load path, into `session.WorktreeEnricher`:
  - Workers (a bounded `scanqueue` pool, 4) only fill an in-memory result cache keyed by path. They never touch an `Instance`, so there is no actor, lock or snapshot race.
  - `fromInstanceData` reads that cache synchronously (memory only) and applies a ready result to the instance it is still constructing (goroutine-local). On a miss it queues a probe and leaves the GitHub fields empty. Results expire after 5 minutes, like `DetectWorktree`'s own cache.
  - `singleflight` per path keeps a blocked path to one goroutine. The 10s deadline starts when the probe starts; on expiry the worker is released and the path is retried with backoff (5s to 5m). The retry joins the still-blocked call. The cache is filled by the call itself when it finally returns.
  - With no enricher registered (CLI paths in `main.go`, most tests) detection stays synchronous as before.
- `Stop` is wired to a server shutdown hook. In-flight probes blocked in the kernel cannot be interrupted and are abandoned.
- Metrics: `worktree_enrichment_total{result=ok|late|error|timeout}`; gauges `worktree_enrichment_queued`, `_running`, `_stuck_paths` (read from the registered enricher).

## Why not a timeout around the probe

A goroutine-plus-timeout leaves the call blocked and detached, needs a marker map to stop duplicates, and a late result must be applied from that detached goroutine to an instance still being read. Here the blocked call is a deduplicated singleflight and nothing mutates an instance from a worker.

## Consequences

- A session on a stuck path loads without GitHub info; it gets it on a load after the probe returns. Boot instances that become live are not re-loaded from disk, so their GitHub info appears after the next restart (or from the other resolvers that fill it). Persisting from the worker through the actor was left out to avoid the races above.
- A stuck probe still holds one OS thread in `open()` until the kernel returns.
- Unbounded `DetectWorktree` callers remain: `NewInstance` (session creation), `WorkspacePeersBlockForPath`, and `prwatch` `detectRemote` (a background poller that holds a mutex while detecting).

## Extension point

`WorktreeEnricherConfig.Ready(path) bool` parks a path (nothing runs) and `Unpark(path)` requeues it. It is a no-op today; the planned TCC consent gate (`STAPLER_SQUAD_TCC_GATE`) plugs in here without changing the queue.
