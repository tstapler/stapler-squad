# ADR-038: Probe worktree info off the load path, not under a timeout

- Status: Accepted
- Context PR: #1380

## Context

`Storage.LoadInstances` -> `fromInstanceData` -> `Instance.DetectAndPopulateWorktreeInfo` -> `DetectWorktree` blocked startup for 5+ minutes. The goroutine dump showed the stack ending in the git backend Router `GetConfig` -> gogit `readRef` -> `readSmallFile` `os.Open` (`session/git/backend/gogit/refstore.go:154`), blocked in `open()` because a session's repo lived under a TCC/iCloud-protected path whose consent prompt was pending.

`DetectWorktree` (`session/repo_path.go`) does an `os.Stat` of `<path>/.git`, may `os.ReadFile` a `.git` file, then calls `gitprovider.Backend().GetConfig(...)` under a 10s context. The Router dispatches that to the gogit backend (in-process file reads, which a context cannot interrupt) or the cli backend (a git subprocess, which the context can kill) by cohort. VERIFIED in code only: `parseCohortMap` (`session/gitwiring/parse.go`) resolves unspecified cohorts to the CLI, and `~/.stapler-squad/config.json` is absent on this machine. The dump's gogit `readRef` frame therefore implies `git_backend_cohorts` was opted into gogit on the affected machine; I did not inspect that machine's config.

## Finding that shapes the design

VERIFIED on one Mac only (research reported by the coordinator; not re-run for this ADR): found that `stat()`/`statfs()` never prompt for or block on TCC-protected paths: `open()`, `readdir()` and `access()` do, and a denial returns EPERM immediately. So the hang is the `open()`-based `DetectWorktree`, not `os.Stat`. The missing-worktree check in `fromInstanceData` is a bare `os.Stat` and stays synchronous.

## Decision

- `fromInstanceData` keeps the `os.Stat` missing-worktree pause exactly as before. Every load path (`FromInstanceData`, `FromInstanceDataDeferred`, `LoadInstances`) therefore behaves as it did, and `Start(false)` still never runs against a missing worktree. No ordering barrier is needed.
- Only the `open()`-based `DetectWorktree` moves off the load path, into `session.WorktreeEnricher`:
  - Workers (a bounded `scanqueue` pool, 4) only fill an in-memory result cache keyed by path. They never touch an `Instance`, so there is no actor, lock or snapshot race.
  - Results are capped at 1024 entries (oldest evicted), a panicking probe becomes an error result, and a cached "no .git" result is never applied to an instance (a recreated worktree within the TTL must not flip a persisted `IsWorktree`).
  - `fromInstanceData` reads that cache synchronously (memory only) and applies a ready result to the instance it is still constructing (goroutine-local). On a miss it queues a probe and leaves the GitHub fields empty. Results expire after 5 minutes, like `DetectWorktree`'s own cache.
  - `singleflight` per path keeps a blocked path to one goroutine. The 10s deadline starts when the probe starts; on expiry the worker is released and the path is retried with backoff (5s to 5m). The retry joins the still-blocked call. The cache is filled by the call itself when it finally returns.
  - With no enricher registered (CLI paths in `main.go`, most tests) detection stays synchronous as before.
- Head-of-line bound: the first attempt on each distinct blocked path holds a worker for the full 10s deadline, so N blocked paths delay other first probes by about N*10s/4 at boot (they only delay GitHub info, never load). Retries of already-stuck paths may hold at most 2 of the 4 workers; the rest return to backoff immediately.
- `Stop` is wired to a server shutdown hook; pending retry timers then fire as no-ops, and installing a new enricher stops the previous one. In-flight probes blocked in the kernel cannot be interrupted and are abandoned.
- Metrics: `worktree_enrichment_total{result=ok|late|error|timeout}`; gauges `worktree_enrichment_queued`, `_running`, `_stuck_paths` (read from the registered enricher).

## Why not a timeout around the probe

A goroutine-plus-timeout leaves the call blocked and detached, needs a marker map to stop duplicates, and a late result must be applied from that detached goroutine to an instance still being read. Here the blocked call is a deduplicated singleflight and nothing mutates an instance from a worker.

## Consequences

- A session on a stuck path loads without GitHub info; it gets it on a load after the probe returns. Boot instances that become live are not re-loaded from disk, so their GitHub info appears after the next restart (or from the other resolvers that fill it). Persisting from the worker through the actor was left out to avoid the races above.
- A stuck probe still holds one OS thread in `open()` until the kernel returns.
- `prwatch` `detectRemote` no longer calls `DetectWorktree`: it reads the enricher cache, queues a probe on a miss, and treats the path as unresolved (retried next poll, not cached) until a result lands. `prwatchSessions.remoteOf` also no longer holds its mutex across the resolver. With no enricher registered the path stays unresolved.
- Unbounded `DetectWorktree` callers remain, deliberately: `NewInstance` (session creation) and `WorkspacePeersBlockForPath`. Both run once per user-initiated create on a path the same request goes on to use with git, and both need the answer synchronously (the instance's GitHub fields; the peers lookup key), so a cache-first read would change behavior (no GitHub info on a fresh session, no peers nudge) rather than just bound it. Neither is reachable from boot or a poller.

## Extension point

`WorktreeEnricherConfig.Ready(path) bool` parks a path (nothing runs) and `Unpark(path)` requeues it. It is a no-op today; the planned TCC consent gate (`STAPLER_SQUAD_TCC_GATE`) plugs in here without changing the queue.
