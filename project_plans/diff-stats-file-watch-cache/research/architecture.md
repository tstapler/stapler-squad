# Architecture Research: diff-stats-file-watch-cache

Date: 2026-09-10 (Agent 3 — Architecture, SDD Phase 2)

## Ground truth carried forward from perf-mutex-hotspots-2026-07

`project_plans/perf-mutex-hotspots-2026-07/research/architecture.md:56` already established:

> `GitWorktree` is a **long-lived, per-session struct**. It is constructed once (via `NewGitWorktree*`) when a session is created or loaded from storage, stored as `GitWorktreeManager.worktree`, and lives until the session is paused, deleted, or the process restarts. It is not created per-call.

and (`:60`) `GitWorktreeManager` is a field on `Instance`, not created per-request. This session's own diff-stats-cache work (`session/git_worktree_manager.go:27-43`) already builds on that: `diffStatsAt`/`DiffStatsFresh()` live as fields on `GitWorktreeManager`, guarded by `gm.mu` (a `deadlock.RWMutex`), exactly mirroring the earlier doc's Q2 recommendation to keep the `IsDirty` TTL cache per-instance rather than in a package-level map. That is the ground truth this design builds on: **one `GitWorktreeManager` per live `Instance`, lifecycle-bound to session create/pause/destroy, already the sole owner of the diff-stats cache.**

## 1. Where does the shared watcher live?

### The import-direction constraint is decisive, not just a style preference

Confirmed by grep: every file in `session/*.go` that mentions `server/services` does so only in comments explaining *why it can't import it* (e.g. `session/callback_dispatcher.go:8`: "session cannot import server/services — server/services imports session, so the reverse import would be a cycle"; `session/instance_tmux.go:894`, `session/jules_session_poller.go:69`, `session/pi_exported.go:26`, `session/stuck_decisions.go:43` all state the same constraint independently). `server/services/*.go` imports `"github.com/tstapler/stapler-squad/session"` in dozens of files (`analytics_store.go:14`, `approval_handler.go:22`, etc.). This is a hard, depguard-enforced one-way edge: `server/services → session`, never the reverse.

That settles the central question. A shared package that both sides import is unnecessary machinery for a problem the codebase already has a standard answer to (see `terminalResyncExecGateFastLaneFlagName`-style duplication and the `LiveInstanceFinder`-style narrow-interface pattern below): **the watcher lives in package `session`, co-located with `GitWorktreeManager`**, and `WorkspaceService` (in `server/services`, which already imports `session`) subscribes to it through a narrow callback interface it registers. The reverse — `session` depending on a `WorkspaceService` interface — is exactly the cycle the codebase has spent effort avoiding everywhere else (`session/backlog_item_change.go:9`, `session/callback_dispatcher.go`).

### Concrete design

New type `session.WorktreeWatcher`, in a new file `session/git_worktree_watcher.go` (sibling to `git_worktree_manager.go`), following the closest existing pattern — `session/unfinished/watcher.go`'s `WatchDirWatcher` (`fsnotify.NewWatcher()` + nil-fallback + walk-then-loop) — but adapted for a *single worktree's full tree* instead of a fleet of `.git`-dir-only watches:

```go
type WorktreeWatcher struct {
    watcher      *fsnotify.Watcher // nil when fsnotify unavailable — pure-TTL fallback
    worktreePath string
    matcher      gitignore.Matcher // gitignore.ReadPatterns(osfs.New(worktreePath), nil), reused from
                                    // session/unfinished/gogit_vcs_reader.go's getOrBuildUntrackedMatcher pattern
    onChange     []func()          // debounced invalidation callbacks; see §3
    debounce     *time.Timer
    stopCh       chan struct{}
}

func NewWorktreeWatcher(worktreePath string) *WorktreeWatcher { ... } // never errors; nil watcher = fallback
func (w *WorktreeWatcher) Start(ctx context.Context) { ... }         // walk + fsnotify.Add per dir, or no-op if flag off
func (w *WorktreeWatcher) OnChange(fn func())                        // register an invalidation callback
func (w *WorktreeWatcher) Active() bool { return w.watcher != nil }  // for the TTL-widening decision (§4)
func (w *WorktreeWatcher) Stop() { ... }                             // close(stopCh); watcher.Close()
```

`GitWorktreeManager` gains a `watcher *WorktreeWatcher` field, constructed in `Setup()` (`session/git_worktree_manager.go:152-158`) right after `wt.Setup()` succeeds — i.e. after the worktree path definitely exists on disk, so the initial recursive walk doesn't race a not-yet-created directory. `GitWorktreeManager.OnChange(fn func())` becomes a public passthrough so `WorkspaceService` — which only holds a `*session.Instance` per session, never a `GitWorktreeManager` directly — needs one more hop: `Instance` already exposes `GetGitWorktree()`/`HasGitWorktree()` (`session/instance_worktree.go:442-453`); add `Instance.OnWorktreeChange(fn func())` alongside them, delegating to `i.gitManager`.

`GitWorktreeManager.OnChange`'s own registered callback (added once, in `Setup()`, before any external subscriber) does the diff-stats half of the invalidation: `gm.mu.Lock(); gm.diffStatsAt = time.Time{}; gm.mu.Unlock()` — clearing `diffStatsAt` is cheaper and correctner than recomputing eagerly (see §2 for why eager recompute-on-event is explicitly rejected).

`WorkspaceService`'s half: **register lazily, at first use**, per the requirements doc's own framing ("WorkspaceService registers against when it first computes a VCS status for that workdir"). Concretely, in `GetVCSStatus` (`server/services/workspace_service.go:133-224`), after the cache-miss path computes `status` and calls `ws.vcsStatusCache.Store(workDir, ...)` (`:219`), add:

```go
if _, already := ws.watchedWorkdirs.LoadOrStore(workDir, struct{}{}); !already {
    instance.OnWorktreeChange(func() { ws.vcsStatusCache.Delete(workDir) })
}
```

`watchedWorkdirs sync.Map` is a new field alongside `vcsStatusCache`/`branchCache` (`server/services/workspace_service.go:78-81`) — it exists only to make the subscription idempotent per workdir, mirroring the existing `branchCache`/`vcsStatusCache` sync.Map-per-concern style already in this struct. This keeps `WorkspaceService` completely ignorant of `fsnotify`/`WorktreeWatcher` internals — it only ever sees a `func()`-shaped seam, the same narrow-interface discipline the codebase already uses for `LiveInstanceFinder` (`server/services/workspace_service.go:40-42`, wired the opposite direction via `SetLiveFinder`).

### Why not a separate registry package keyed by workdir?

Rejected. It would need to be a *third* package importing both `session`-shaped types and being imported by `server/services`, but it would duplicate exactly the lifecycle `GitWorktreeManager` already owns (start-on-setup, stop-on-cleanup) in a new place with its own concurrent-map bookkeeping (multiple `Instance`s could in theory share a workdir path, per the requirements doc's "potentially (rarely) shared across Instances" note — but ownership of *starting and stopping the watcher* must still belong to whichever `Instance`/`GitWorktreeManager` owns that worktree's disk lifecycle; a path-keyed registry would have to reference-count across Instances for no real benefit, since the 134-worktree/100+-session scale target already implies effectively-unique paths in practice — worktrees are per-session directories, not shared checkouts). Keeping the watcher inside `GitWorktreeManager` means `Cleanup()`/`Remove()` stopping it is a one-line addition to code that already exists, instead of a new cross-package deregistration path.

## 2. Data flow and consistency

| Step | Component | Detail |
|---|---|---|
| 1. Worktree created | `GitWorktreeManager.Setup()` (`session/git_worktree_manager.go:152`) | `wt.Setup()` succeeds → directory exists on disk. |
| 2. Watcher starts | `GitWorktreeManager.Setup()` (new code, gated by feature flag — §4) | `gm.watcher = NewWorktreeWatcher(wt.GetWorktreePath()); gm.watcher.Start(ctx)`. Initial walk adds an fsnotify watch per directory, skipping `.git` and gitignored paths (via `gitignore.Matcher`, same as `session/unfinished/gogit_vcs_reader.go:964-965`). Registers the diff-stats-clearing callback (§1). |
| 3. File edited | OS / fsnotify backend | inotify (Linux) / FSEvents (macOS) / ReadDirectoryChangesW (Windows) delivers a Write/Create/Remove/Rename event to `w.watcher.Events`. |
| 4. Event received | `WorktreeWatcher`'s event loop (mirrors `unfinished/watcher.go:145-188`'s `fsnotifyLoop`) | Filter: skip events under `.git/` (shouldn't be watched at all — not added in step 2) and gitignored paths (checked against the cached matcher). A `Create` event for a new directory triggers an `Add`-per-subdirectory call (new-subdirectory detection), not just a cache invalidation. |
| 5. Debounce | `WorktreeWatcher` (new — no direct precedent in `unfinished/watcher.go`, which debounces at the *scan-enqueue* layer via `Scanner`, not in the watcher itself) | Reset a single `time.Timer` (e.g. 500ms–1s window; see sizing note below) on every qualifying event; only fire the invalidation callbacks when the timer elapses with no further events. This coalesces an editor's save-then-format-then-lint burst into one invalidation instead of N. |
| 6. Cache invalidated | Both caches, via two independently-registered callbacks fired from the same debounce tick — **not the same operation** | `GitWorktreeManager`'s own callback clears `diffStatsAt` (not `diffStats` itself — leave the last-known value visible until a fresh one replaces it, matching `ClearDiffStats`'s existing distinction between "no worktree" (`:311-318`, clears both) and "stale" (this case, timestamp only)). `WorkspaceService`'s callback does `vcsStatusCache.Delete(workDir)` — a real delete, not a timestamp reset, because `vcsStatusCacheEntry` has no separate "stale but keep value" state today (`server/services/workspace_service.go:52-55`). |
| 7. Next RPC call | `GetSessionDiff` → `Instance.RefreshDiffStatsIfStale()` (`session/instance_worktree.go:481-486`) or `GetVCSStatus` (`server/services/workspace_service.go:154-167`) | Cache miss (diffStatsAt zero, or `vcsStatusCache.Load` misses) → recomputes for real. |
| 8. Cache repopulated | Same call sites | `GitWorktreeManager.UpdateDiffStats()` (`:273-287`) stamps a fresh `diffStatsAt`; `GetVCSStatus` stamps a fresh `cachedAt` (`:217-219`). |

### Race windows

1. **Setup-to-watcher-start gap.** Between `wt.Setup()` returning (worktree directory now exists and is writable) and `gm.watcher.Start(ctx)` completing its initial walk (which is itself not instantaneous for a large tree), an edit could land and be missed by the *watch registration* — the walk hasn't `Add`ed that directory to fsnotify yet. **Acceptable**: the TTL backstop (§4) still exists specifically for this reason — the requirements doc calls the widened TTL a "backstop only" concern, and a miss here means the RPC serves stale data for at most the backstop TTL (5+ min), then self-corrects on the next poll regardless of watcher state. This is the same trade-off `session/unfinished/watcher.go` already accepts for its own `.git`-dir-only watch plus 60s `periodicReWalk` (`:191-205`) — not a new risk class, an extension of an already-accepted one.
2. **New-subdirectory-created-and-immediately-written race.** A `Create` event for a new directory and a near-simultaneous `Write` inside it (e.g. `git checkout` materializing a whole new subtree) could see the write event arrive before the `Add` for the new directory completes, if they're not on the same fsnotify delivery ordering guarantee cross-platform. Mitigated, not eliminated, by adding the directory watch synchronously inside the same event-loop goroutine that processes the `Create` event (no separate goroutine hand-off), and backstopped identically by the TTL.
3. **Debounce-timer-fires-during-Stop() race.** `GitWorktreeManager.Cleanup()`/`Remove()` stopping the watcher (closing `stopCh`) while a debounce timer is mid-flight could fire a callback against a `GitWorktreeManager` that's mid-teardown. Mitigated by having `Stop()` call `debounce.Stop()` before closing the event-loop channel, and by the callbacks themselves being cheap, idempotent map/field operations (`Delete` on a `sync.Map`, zeroing a `time.Time`) that are safe to run against a torn-down-but-not-yet-GC'd structure.
4. **Two Instances, same workdir path (rare, per requirements doc).** If two live `Instance`s somehow share a workdir (e.g. a bug, or a directory-mode session pointed at the same path as another), each has its own `GitWorktreeManager` and thus its own `WorktreeWatcher`, so `WorkspaceService`'s `watchedWorkdirs` `LoadOrStore` means only the *first* `Instance` to call `GetVCSStatus` for that path gets its watcher's invalidation wired to that cache entry. The second `Instance`'s edits would go through its own watcher, invalidate its own `GitWorktreeManager`'s diff-stats cache correctly, but not this shared `vcsStatusCache` entry. **Acceptable**: this is a pre-existing edge case in the current path-keyed `sync.Map` design (`server/services/workspace_service.go:81`, which already assumes one workdir → one cache entry regardless of how many Instances reference it) — this feature doesn't worsen it, and the TTL backstop still bounds the staleness.

## 3. Integration points (every call site that changes)

1. **`session/git_worktree_manager.go`** — add `watcher *WorktreeWatcher` field to `GitWorktreeManager`; `Setup()` (`:152-158`) starts it (flag-gated); `Cleanup()` (`:162-168`) and `Remove()` (`:171-177`) stop it; add `OnChange(fn func())` passthrough; `DiffStatsFresh()` (`:47-51`) gains a second condition — see §4.
2. **New `session/git_worktree_watcher.go`** — the `WorktreeWatcher` type itself (§1).
3. **`session/instance_worktree.go`** — add `Instance.OnWorktreeChange(fn func())` (delegates to `i.gitManager`), placed near the existing `GetGitWorktree`/`HasGitWorktree`/`CleanupWorktree` cluster (`:432-453`). `RefreshDiffStatsIfStale()` (`:481-486`) itself needs no change — it already just checks `DiffStatsFresh()`, which gains the watcher-aware TTL logic internally.
4. **`server/services/workspace_service.go`** — add `watchedWorkdirs sync.Map` field (`:67-82`); in `GetVCSStatus` (`:133-224`), register the lazy invalidation callback on cache-miss (§1); the existing `ws.vcsStatusCache.Delete(preWorkDir)` at `:431` (SwitchWorkspace's existing invalidation) is unaffected — it already handles the "workdir changed out from under this session" case, orthogonal to file-content changes.
5. **`server/services/feature_flag_service.go`** — new entry in `knownFeatureFlags` (`:127-...`, alongside e.g. `terminalResyncExecGateFastLaneFlagName`'s pattern at `:176-179`): name e.g. `"diff-stats:file-watch-cache"`, `description` explaining the widened-TTL/fsnotify trade-off, `defaultValue: false` (per requirements: "Default off").
6. **`session` package's own flag read** — per the `session` package's established duplication convention for flags it needs but can't import from `server/services` (`session/instance_tmux.go:975`, `:892-894` comment), define a matching unexported string constant inside `session/git_worktree_manager.go` or `git_worktree_watcher.go` with the identical string value as the one added in step 5, and read it via `config.LoadConfig().GetFeatureFlag(...)` directly (the `config` package is already imported by both `session` and `server/services` — no cycle). `GitWorktreeManager.Setup()` checks this flag before constructing/starting a `WorktreeWatcher` at all.
7. **`server/dependencies.go`** — no change needed for wiring `WorkspaceService` itself (`NewWorkspaceService(storage, eventBus)` and `SetLiveFinder` already exist and are untouched); the new callback registration happens lazily inside `GetVCSStatus`, not at construction time, so there's no new constructor-time dependency to wire.

## 4. Disposition: Isolate via seam, not Refactor-first

**Recommendation: Isolate via seam.** A new `WorktreeWatcher` component behind a narrow `OnChange(func())` callback interface, added to `GitWorktreeManager` (whose lifecycle methods — `Setup`/`Cleanup`/`Remove` — already exist and just gain one more call each) and consumed by `WorkspaceService` through a lazy per-workdir subscription. Neither existing struct's internals change shape; `vcsStatusCache` stays exactly where it is, keyed exactly as it is today.

**Why not the deeper refactor** (moving `vcsStatusCache` to live per-`Instance`, eliminating the path-keyed `sync.Map`): that would fix a *different*, not-in-scope problem — the "potentially shared across Instances" ambiguity the requirements doc flags as a pre-existing property of `vcsStatusCache`, not something this feature introduces or worsens (§2, race window 4). Moving the cache would mean:
- `WorkspaceService` no longer needing a workdir-keyed map at all (each `Instance` owns its own entry) — genuinely simpler in isolation — but
- `WorkspaceService.GetVCSStatus` would need a live-instance handle for the cache read/write path it doesn't currently need one for in the same way (`findInstanceFast` already gets one, so this is not a blocker, but it's still a change to a hot RPC path's data ownership for a problem this feature doesn't need to solve), and
- it would touch `SwitchWorkspace`'s existing `vcsStatusCache.Delete(preWorkDir)` invalidation (`:431`) and any other code that reads `vcsStatusCache` by path rather than by instance — an audit this project's scope (per its own Rabbit Holes section) didn't ask for and the requirements doc explicitly scopes to "a SHARED per-worktree file-watcher component," not a cache-ownership migration.

The balance point: this is infrastructure/caching, not a business-domain change, and the existing violation (path-keyed cache theoretically shared across Instances) is narrow, already-accepted, and orthogonal to correctness of the watcher itself — TTL backstop still bounds it either way. Refactoring `vcsStatusCache`'s ownership model would be gold-plating relative to what the requirements ask for; the seam-based design solves the stated problem (extend TTL safely, invalidate both caches on real changes) without deepening or touching that existing ambiguity.

## Debounce window and fd-ceiling notes (Feasibility Risks)

- **Debounce sizing**: 500ms–1s, matching common editor/tool save-burst durations (format-on-save, git hooks rewriting files) without meaningfully delaying "changes are visible" for a human watching the diff tab. No existing precedent in this codebase sets this number — `session/unfinished/watcher.go` debounces at the scan-enqueue layer (`Scanner.EnqueueRepo`, not shown here) rather than the fsnotify-event layer, so this is a new constant to tune, not a value to import.
- **fd ceiling**: one `fsnotify.Watcher` per worktree, with one inotify watch descriptor per non-ignored directory in that worktree (not per file — inotify watches are directory-scoped, matching `session/unfinished/watcher.go`'s existing per-`.git`-dir `Add` calls, just applied to every non-ignored subdirectory instead of just `.git`). At 134 concurrent worktrees this is 134 goroutines + (descriptor count = sum of non-ignored subdirectory counts per repo) — no measured number exists yet (per requirements' Feasibility Risks, "no measured fd headroom"); this needs a real measurement pass against the dev box's actual worktree set before shipping, gated by the feature flag defaulting off specifically so it can be measured in isolation before wider rollout.
