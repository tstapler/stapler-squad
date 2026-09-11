# Implementation Plan: diff-stats-file-watch-cache

**Feature**: Widen `GitWorktreeManager`'s diff-stats cache and `WorkspaceService`'s `vcsStatusCache` from a 15s pure TTL to a 5-minute TTL, backed by a per-worktree change-detection signal (`.git` fsnotify watch + a staggered 15s periodic cheap stat-walk mirroring `worktreeIsDirtyFast`), gated behind a new default-off feature flag.
**Date**: 2026-09-10
**Status**: Ready for implementation
**ADRs**: ADR-029-git-watch-plus-periodic-stat-walk-over-recursive-fsnotify.md

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `WorktreeChangeDetector` | New type, `session/git_worktree_watcher.go`. Owns one `.git`-only fsnotify watch plus one jittered 15s periodic fingerprint-comparison loop for a single worktree. | Replaces the pre-pivot `WorktreeWatcher` name from architecture.md — no longer does recursive per-directory watching, so "watcher" alone is misleading. |
| `NewWorktreeChangeDetector(worktreePath string, fingerprint fingerprintFunc, activeFunc func() bool) *WorktreeChangeDetector` | Constructor. Never errors — matches `NewWatchDirWatcher`'s nil-on-fsnotify-unavailable convention for the `.git` sub-watch only; the periodic loop always starts. `activeFunc` may be `nil` ("always active"); see Story 2.1.2. | |
| `activeFunc func() bool` | Injected closure, checked by `periodicLoop` before `fingerprint()` on every tick; `false` means skip this tick's stat-walk entirely (cheap check only). Backed by `GitWorktreeManager.lastRequestedAt` in production (Story 2.2.2) — resolves pre-mortem.md P1 item #3 ("no caller, no work" for idle worktrees). | |
| `fingerprintFunc` | `func() (dirty bool, headSHA string, err error)` — injected closure, not a hard dependency on `*git.GitWorktree`, so tests can fake it. | Lets `WorktreeChangeDetector` be unit-tested with a temp dir and no real git repo. |
| `(*WorktreeChangeDetector).Start()` | Takes an initial fingerprint synchronously (baseline), attempts the `.git` `fsnotify.Add()`, launches the periodic-loop goroutine (and the fsnotify event-loop goroutine, if the `.git` watch succeeded). | |
| `(*WorktreeChangeDetector).OnChange(fn func())` | Registers an invalidation callback. Called with no arguments — a pure "something changed" signal, matching requirements' "did *anything* change" framing. | |
| `(*WorktreeChangeDetector).Stop()` | Cancels the internal context, closes the fsnotify watcher (if any), blocks until both goroutines have exited. | Single chokepoint for goroutine/fd cleanup — see Pattern Decisions. |
| `(*WorktreeChangeDetector).GitWatchActive() bool` | True if the `.git` fsnotify `Add()` succeeded. Observability-only — does not gate TTL widening (see below). | |
| `(*GitWorktreeManager).changeDetector *WorktreeChangeDetector` | New field. Nil when the flag is off or `Setup()` hasn't run yet. | |
| `(*GitWorktreeManager).changeDetectionActive bool` | New field, guarded by `gm.mu`. True once `changeDetector` has been constructed and `Start()` called. Drives which diff-stats TTL constant `DiffStatsFresh()` uses. | |
| `diffStatsCacheTTLWithChangeDetection` | New const, `5 * time.Minute`, `session/git_worktree_manager.go`. | |
| `vcsStatusCacheTTLWithChangeDetection` | New const, `5 * time.Minute`, `server/services/workspace_service.go`. | |
| `changeDetectionStatWalkInterval` | New const, `15 * time.Second`, `session/git_worktree_watcher.go`. | |
| `worktreeChangeDetectionFlagName` | New const, `"vcs:worktree-change-detection"`. Declared once in `server/services/feature_flag_service.go` (registered in `knownFeatureFlags`) and duplicated as an unexported string constant of the identical value in `session/git_worktree_manager.go`, per the `session`-can't-import-`server/services` convention already used for other flags read from `session`. | |
| `(*GitWorktree).IsDirtyUncached() (bool, error)` | New method, `session/git/worktree_dirty_fast.go`. Calls `worktreeIsDirtyFast` directly, bypassing `IsDirtyWithHint`'s own 30s/5min/60s TTL cache. | Needed because reusing `IsDirtyWithHint`'s cached answer for the periodic tick would just relocate the staleness problem this feature exists to fix. |
| `(*Instance).OnWorktreeChange(fn func())` | New passthrough, `session/instance_worktree.go`, delegates to `i.gitManager`. | |
| `(*Instance).WorktreeChangeDetectionActive() bool` | New passthrough, delegates to `i.gitManager`. Used by `WorkspaceService` to decide whether to widen `vcsStatusCache`'s TTL for that workdir, and by `CreateDebugSnapshot`. | |
| `watchedWorkdirs sync.Map` | New field, `server/services/workspace_service.go`, alongside `vcsStatusCache`/`branchCache`. Makes the lazy `OnWorktreeChange` subscription idempotent per workdir. | |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Change-detection component shape | (b) one `WorktreeChangeDetector` per worktree, owning both the `.git` watch goroutine and its own jittered-start periodic-loop goroutine internally | Step 0.5 creative pass (below) | (a) same per-worktree shape but with an *unjittered* shared-phase ticker; (c) a single fleet-wide shared worker pool/queue (`unfinished.Scanner`-style `scanQueue`/`worker()`) that all 134 worktrees enqueue into | (a) rejected: would synchronize all 134 worktrees' stat-walks onto the same wall-clock phase if many `Setup()` calls land in the same tick window (e.g. process restart reloading 134 sessions at once) — exactly the "don't fire all 134 on one tick" risk requirements.md flags. (c) rejected: architecture.md already resolved that this component lives inside `GitWorktreeManager`, one per `Instance`, with no separate cross-cutting package — a shared worker pool would need exactly that new package/registry, undoing that decision for a check that's already cheap (O(tracked-files) `lstat`, same cost as today's existing `IsDirtyWithHint`) per worktree. Jittering each detector's own ticker start (reusing `PrimeDirtyCacheJitter`'s existing `rand.Int63n` idiom) gets the same "don't synchronize" outcome with zero new shared infrastructure. |
| Invalidation delivery (Observer) | Plain `OnChange(func())` callback list, invoked synchronously and inline from whichever goroutine (fsnotify event-loop or periodic-loop) detects the change | architecture.md §1, stack.md §4 | A pub/sub event bus (`pkgevents.EventBus`, already used by `unfinished.Scanner`) | An event bus is the right shape for many-to-many, cross-package fan-out; this is a 1-worktree-to-2-callbacks case (`GitWorktreeManager`'s own diff-stats clear, plus `WorkspaceService`'s lazily-registered `vcsStatusCache.Delete`) — a plain callback slice is the smaller, equally-testable primitive, matching the narrow-interface style already used for `LiveInstanceFinder`. |
| Debounce/coalescing | None — each callback invocation is a cheap, idempotent invalidate (`diffStatsAt = time.Time{}`, `sync.Map.Delete`), safe to call redundantly | stack.md §4 (`atomic.Bool`-style rejected too, in favor of "no timer needed at all") | Per-path `time.AfterFunc` debounce timer (mmapwatch.go's shape) | Rejected per the Design Pivot: the periodic stat-walk is edge-triggered (fires only when the fingerprint actually changes since the last tick), which already coalesces any burst within one 15s window into at most one invalidation; the `.git` watch firing redundantly on top of that costs nothing since invalidation is idempotent. Adding a timer here would be new machinery to solve a problem that doesn't exist post-pivot. |
| Fingerprint freshness | New `IsDirtyUncached()` bypassing `IsDirtyWithHint`'s cache | pitfalls.md's "advisory, not authoritative" framing applied to this codebase's existing cache layering | Call `IsDirtyWithHint(false)` directly | `IsDirtyWithHint` has its own 30s(dirty)/5min(clean) TTL — feeding its cached answer into a fingerprint comparison meant to *drive* a different cache's invalidation would mean a real edit could be invisible to the periodic loop for up to 5 minutes on top of the widened diff-stats TTL, defeating the feature. |
| VCS scope (Strategy candidate, rejected) | Git-only in v1; Jujutsu-managed worktrees keep today's unconditional 15s TTL, unconditionally, regardless of flag state | `session/vc/git_provider.go`, `session/vc/jj_provider.go`, `server/services/workspace_service.go:169-194` | A `VCSChangeFingerprint` strategy interface with a git and a jj implementation | `worktreeIsDirtyFast`/`IsDirtyUncached` are git-specific (`OpenRepo`, index, tree-hash walk) with no jj equivalent in this codebase today. `GitWorktreeManager`/`GitWorktree` (where `changeDetector` lives) are themselves git-only types — a jj-managed session never constructs one (`WorkspaceService.GetVCSStatus` falls back to `vc.NewJujutsuProvider` only when `vc.NewGitProvider` fails, `workspace_service.go:169-172`). Building a Strategy abstraction for a jj backend that doesn't exist yet is speculative generality; a jj worktree's `Instance.WorktreeChangeDetectionActive()` simply returns `false` because `gitManager.changeDetector` is never set, which already produces the correct behavior (15s TTL, unchanged) with zero new code. |
| Lifecycle cleanup | Single chokepoint: `GitWorktreeManager.Cleanup()`/`Remove()` call `gm.stopChangeDetector()` before delegating to the underlying `*git.GitWorktree` | pitfalls.md §3 "Goroutine-leak-on-exit-path risk" | Duplicating a `Stop()` call at each of `instance.go`'s ~6 call sites that currently call `gitManager.Cleanup()`/`gitManager.Remove()` | Grep confirms every existing teardown path in `session/instance.go` (lines 1443, 1703, 2017, 2058 for `Cleanup()`; 1940, 2133 for `Remove()`) already funnels through one of these two `GitWorktreeManager` methods — hooking `Stop()` inside them covers every caller for free and matches this repo's existing "one chokepoint, not N call sites" discipline. |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `vcsStatusCache`'s path-keyed `sync.Map` vs. potential multi-`Instance`-per-workdir sharing | architecture.md §2 race window 4 / §4: a workdir shared by two live `Instance`s means only the first `Instance` to call `GetVCSStatus` gets its `WorktreeChangeDetector` wired to that cache entry; the second `Instance`'s edits invalidate its own diff-stats cache but not the shared `vcsStatusCache` entry. | **Isolate via seam** — do not fix in this project. Add `WorktreeChangeDetector` behind the narrow `OnChange(func())` callback consumed lazily by `WorkspaceService`; leave `vcsStatusCache`'s path-keying, `SwitchWorkspace`'s existing `Delete(preWorkDir)` invalidation, and `branchCache` completely untouched. | This is a pre-existing, already-accepted property of `vcsStatusCache` (one workdir → one cache entry regardless of how many `Instance`s reference it) — this feature's TTL backstop (5 min max staleness even on a total callback miss) bounds it exactly as tightly as today's 15s TTL bounds it, just wider. Fixing cache ownership is a different, larger refactor (moving `vcsStatusCache` to live per-`Instance`) explicitly out of this project's scope per architecture.md §4 and requirements.md's own Scope section. Concrete seam task: Task 2.3.1a below adds `watchedWorkdirs sync.Map` purely to make the *lazy subscription* idempotent, not to change cache ownership. |

---

## Migration Plan

N/A — no schema or persisted-data changes. `GitWorktreeManager`/`WorkspaceService` fields added are runtime-only, not serialized (confirm: `GitWorktreeManager`'s existing `diffStats`/`diffStatsAt` are also not part of any `MarshalJSON`/persisted `Instance` state per `session/git_worktree_manager.go`'s own doc comments — same treatment applies to `changeDetector`/`changeDetectionActive`, which must not be serialized either, since a `*WorktreeChangeDetector` holding a live goroutine and fsnotify fd cannot survive a deserialize-on-restart round-trip).

## Observability Plan

- **Logs**:
  - `log.Warn("worktree change detector: .git fsnotify watch failed to start, falling back to periodic-check-only for this worktree", "worktreePath", ..., "err", ...)` — emitted at most once per worktree lifecycle, inside `WorktreeChangeDetector.Start()`'s `.git` `Add()` failure branch (no rate limiter needed beyond "only ever attempted once," unlike the fleet-wide `Scanner.severePressureWarned` case which retries every tick).
  - `log.Debug("worktree change detector: fingerprint changed, invalidating caches", "worktreePath", ..., "dirtyChanged", ..., "headChanged", ...)` on every fired invalidation — the features.md-recommended diagnostic trail for distinguishing "detector fired, cache still wrong" from "detector never fired" during triage.
  - `log.Debug("worktree change detector: fingerprint read failed, skipping tick", "worktreePath", ..., "err", ...)` when the injected `fingerprintFunc` errors (e.g. transient repo-unreadable) — never promoted to Warn since this is expected to self-heal next tick.
- **Metrics**: None new (this app has no metrics-export pipeline for internal dev-tool subsystems beyond structured logs + `CreateDebugSnapshot`, matching `unfinished.Scanner`'s own precedent of counters like `gitignoreMatcherInvalidations` surfaced only via debug snapshot / log, not a metrics backend).
- **Alerts**: N/A (local dev tool, no on-call).
- **Debug snapshot**: `server/services/debug_snapshot.go`'s `SessionSnapshot` struct gains two fields, populated in `collectSessionSnapshots` from the new `Instance` passthroughs:
  ```go
  WorktreeChangeDetectionActive bool `json:"worktree_change_detection_active"`
  WorktreeGitWatchActive        bool `json:"worktree_git_watch_active,omitempty"`
  ```

## Risk Control

- **Feature flag**: `vcs:worktree-change-detection`, default `false`. Registered in `server/services/feature_flag_service.go`'s `knownFeatureFlags` (see Task 1.1.1a).
- **Rollback procedure**: `UpdateFeatureFlag` back to `false` (or simply never having enabled it) — no redeploy, no data migration. Per pitfalls.md §4, this is **not live-retroactive**: a worktree whose `GitWorktreeManager.Setup()` already ran with the flag on keeps its `WorktreeChangeDetector` running (and the widened TTL) until that session is torn down and a new one created, matching the `terminal:resync-*` flags' documented precedent. The flag description string states this explicitly (Task 1.1.1a).
- **Staged rollout**: Ship default-off. Enable manually on the 134-worktree dev box first (the box this feature was scoped against) to measure real fd/goroutine consumption and CPU delta via `CreateDebugSnapshot` + the same Pyroscope methodology cited in requirements.md, before recommending default-on in a follow-up change (not part of this project). See Unresolved Questions below for this measurement's concrete owner/trigger — it is a tracked follow-up, not silently dropped.

## Unresolved Questions

- [ ] **CPU-share re-measurement is a tracked post-merge follow-up, not a task in this plan.** requirements.md's Success Metrics demand `GetSessionDiff`/`GetVCSStatus`'s combined CPU share (Pyroscope `SelectSeries`/`SelectMergeProfile` methodology, ~13%+7.5% baseline) be shown to drop "measurably." That drop is only observable under realistic, sustained idle/background-worktree load at fleet scale (134 worktrees) — a synthetic burst in a test run would not reproduce the access pattern the baseline was measured against, and Epic 3.3's tests (flag-off TTL boundary, one real-edit invalidation, a 150-cycle leak proxy) deliberately don't attempt this. Per this plan's own Risk Control §"Staged rollout," the flag ships default-off and gets enabled manually on the 134-worktree dev box first — that rollout step *is* the trigger for this measurement, so it is not silently dropped, just sequenced after merge rather than gated on it.
  - **Owner**: whoever flips `vcs:worktree-change-detection` to `true` on the dev box per the Staged rollout step.
  - **Trigger**: 48 hours after the flag is enabled on the dev box (long enough to span multiple idle/background-worktree cycles at realistic scale).
  - **Method**: re-run the same Pyroscope `SelectSeries`/`SelectMergeProfile` + `go tool pprof -top -cum` comparison used to establish the ~13%+7.5% baseline this session, comparing a pre-enable window against a post-enable window; record the before/after CPU-share delta in a short note (PR comment or follow-up issue) referencing this plan. **Attribute `WorktreeChangeDetector.periodicLoop`/`IsDirtyUncached`'s own CPU cost as a separate line item in this same profile pull** (e.g. `go tool pprof -top -cum` filtered to `session.(*WorktreeChangeDetector).periodicLoop`/`session/git.IsDirtyUncached`), not folded into the two RPCs' numbers — per pre-mortem.md failure mode #3, the periodic stat-walk is new scheduled work that a truly idle worktree never paid before this feature, so a flat or worse combined-CPU-share result must be diagnosable as "the detector's own cost offset the cache-hit savings" versus "the cache-hit savings didn't materialize," which requires the detector's cost broken out on its own rather than netted against the RPCs it feeds.
  - **Escalation if the delta doesn't materialize**: re-open this plan's Design Pivot assumptions (periodic-stat-walk cost, 15s interval, 5-minute TTL) rather than assuming the measurement methodology was wrong. If the separated periodic-loop attribution above shows the detector's own cost is the dominant driver of a flat/worse result (rather than the RPCs' own recompute cost), treat Epic 2.1's Story 2.1.2 idle-gating (below) as under-tuned first — e.g. the `coldActivityThreshold` window is too short relative to real polling cadence — before reopening the interval/TTL constants themselves.

## Dependency Visualization

```
                         +------------------------------------+
                         |   feature_flag_service.go           |
                         |   knownFeatureFlags += "vcs:..."    |
                         +------------------+-------------------+
                                            |  (string constant, duplicated)
                                            v
+------------------------+         +--------------------------+
| session/git             |         | session (package)        |
| worktree_dirty_fast.go  |<--------+ git_worktree_watcher.go  |
| + IsDirtyUncached()     |  calls  | WorktreeChangeDetector   |
+------------------------+  via    +-----------+--------------+
                             fingerprintFunc     | OnChange() registered by
                                                  v
                                     +--------------------------+
                                     | git_worktree_manager.go  |
                                     | GitWorktreeManager        |
                                     | .Setup()/.Cleanup()/      |
                                     | .Remove() start/stop it;  |
                                     | DiffStatsFresh() widens   |
                                     | TTL when active           |
                                     +-----------+--------------+
                                                  | Instance.OnWorktreeChange
                                                  | Instance.WorktreeChangeDetectionActive
                                                  v
                                     +--------------------------+
                                     | instance_worktree.go      |
                                     | Instance passthroughs     |
                                     +-----------+--------------+
                                                  | (server/services already imports session)
                                                  v
                                     +--------------------------+
                                     | workspace_service.go      |
                                     | WorkspaceService           |
                                     | lazy subscribe on cache-  |
                                     | miss; vcsStatusCache TTL  |
                                     | widens per-entry          |
                                     +--------------------------+
```

---

## Phase 1: Foundation

### Epic 1.1: Feature flag and exported git primitive

**Goal**: Register the flag and add the one new git-package primitive everything else depends on, with no behavior change yet (flag is unread by any call site until Phase 2).

#### Story 1.1.1: Register `vcs:worktree-change-detection` flag
**As a** operator, **I want** a flag I can flip without a redeploy, **so that** the new watch/stat-walk machinery can be enabled and disabled live.
**Acceptance Criteria**:
- `knownFeatureFlags` contains an entry named `worktreeChangeDetectionFlagName` with `defaultValue` omitted (false).
  - *Given* a fresh `config.json` with no `feature_flags` key, *When* `GetFeatureFlagWithDefault("vcs:worktree-change-detection", false)` is called, *Then* it returns `false`.
- The flag's description states the non-retroactivity behavior explicitly.
  - *Given* the flag is flipped from `false` to `true` while a session's `GitWorktreeManager.Setup()` already ran, *When* that session's `GetSessionDiff` is next called, *Then* it still uses the 15s TTL (no detector was started for it) until the session is destroyed and a new one created.
**Files**: `server/services/feature_flag_service.go`

##### Task 1.1.1a: Add the flag constant and registry entry (~3 min)
- Add `const worktreeChangeDetectionFlagName = "vcs:worktree-change-detection"` near the other flag-name consts (e.g. next to `terminalResyncStaggerFlagName`, line ~106).
- Add to `knownFeatureFlags`:
  ```go
  {
  	name:        worktreeChangeDetectionFlagName,
  	description: "Watch each session's .git dir via fsnotify and run a staggered 15s periodic cheap dirty/HEAD check to invalidate the diff-stats and VCS-status caches, letting both widen from a 15s to a 5-minute TTL. Applies to newly-created worktrees only; already-open sessions keep today's 15s pure-TTL behavior until restarted. Default: off.",
  },
  ```
- Files: `server/services/feature_flag_service.go`

#### Story 1.1.2: `GitWorktree.IsDirtyUncached()`
**As a** the periodic stat-walk, **I want** a fresh, uncached dirty answer, **so that** comparing it tick-over-tick actually detects real changes instead of `IsDirtyWithHint`'s own stale cache.
**Acceptance Criteria**:
- `IsDirtyUncached()` returns the same boolean `worktreeIsDirtyFast` would, with no TTL involved, while still reusing `GitWorktree`'s own `gitignoreFS`/`headTreeCache` allocation-avoidance caches (only `IsDirtyWithHint`'s outer 30s/5min policy cache is bypassed, not these).
  - *Given* a worktree with one untracked file `foo.txt` created after the last `IsDirtyWithHint` call populated its 5-minute clean-cache entry, *When* `IsDirtyUncached()` is called immediately after, *Then* it returns `true` (not the stale cached `false`).
**Files**: `session/git/worktree_dirty_fast.go`

##### Task 1.1.2a: Add `IsDirtyUncached` (~3 min)
- Add:
  ```go
  // IsDirtyUncached reports whether the worktree has uncommitted changes,
  // bypassing IsDirtyWithHint's own TTL cache (IsDirtyCacheTTL/IsDirtyCleanCacheTTL)
  // -- but still reusing g.gitignoreFS/g.headTreeCache, the per-GitWorktree
  // allocation-avoidance caches worktreeIsDirtyFast itself needs. Those two are
  // safe to share here without reintroducing staleness: headTreeCache is keyed
  // by HEAD's own commit hash (a hit can never be stale) and gitignoreFS has its
  // own independent invalidation (InvalidateDirtyCache) unrelated to the
  // 30s/5min TTL-staleness problem this method's cache bypass exists to fix.
  // Used by session.WorktreeChangeDetector's periodic tick, which needs a fresh
  // per-tick answer to compare against the previous tick -- reusing
  // IsDirtyWithHint's cached *answer* here would just relocate the staleness
  // problem this feature exists to fix, but discarding the inner caches too
  // would reintroduce the exact CPU/allocation cost (full HEAD-tree walk +
  // gitignore-pattern re-read on every 15s tick, across up to 134 worktrees)
  // this session's own commit 4cd1d384a and gitignoreFSCache eliminated.
  func (g *GitWorktree) IsDirtyUncached() (bool, error) {
  	return worktreeIsDirtyFast(g.GetWorktreePath(), &g.gitignoreFS, &g.headTreeCache)
  }
  ```
  directly below `IsDirtyWithHint` (`session/git/worktree_git.go:316`) or in `worktree_dirty_fast.go` next to `worktreeIsDirtyFast` itself — place in `worktree_dirty_fast.go` since that's where the algorithm it calls lives.
- Files: `session/git/worktree_dirty_fast.go`

##### Task 1.1.2b: Unit test for `IsDirtyUncached` bypassing the cache (~5 min)
- New test `TestIsDirtyUncached_BypassesIsDirtyWithHintCache` in `session/git/worktree_git_test.go`: create a temp git repo/worktree, call `IsDirtyWithHint(false)` once while clean (populates the 5-min clean cache), create a new untracked file, then assert `IsDirtyUncached()` returns `true` while a same-instant `IsDirtyWithHint(false)` still returns the stale cached `false` — proving the two are genuinely decoupled.
- Add a second assertion (or a sibling test, `TestIsDirtyUncached_ReusesGitWorktreeCaches`) proving the *inner* caches are still shared, not just bypassed: call `IsDirtyUncached()` once to populate `g.headTreeCache`, then call it again with `g.headTreeCache` inspected (e.g. via a same-package test that checks the cache's stored HEAD hash matches, or by asserting a second call is a no-op on an unchanged HEAD) — the point is catching a future regression back to `nil, nil` that Task 1.1.2a's first version had.
- Files: `session/git/worktree_git_test.go`

---

## Phase 2: `WorktreeChangeDetector` and `GitWorktreeManager` wiring

### Epic 2.1: `WorktreeChangeDetector` type

**Goal**: A self-contained, independently-testable type with no dependency on `Instance`/`GitWorktreeManager` internals — takes a path and a fingerprint closure, exposes `Start`/`Stop`/`OnChange`/`GitWatchActive`.

#### Story 2.1.1: Construct and start the detector
**As a** `GitWorktreeManager`, **I want** a detector I can start with one call, **so that** wiring it into `Setup()` is a small, safe addition.
**Acceptance Criteria**:
- `Start()` takes a baseline fingerprint synchronously before returning, so the first periodic tick doesn't fire a spurious invalidation for state that was already fresh.
  - *Given* a worktree at `/tmp/wt-1` that is clean with HEAD `abc123`, *When* `NewWorktreeChangeDetector("/tmp/wt-1", fp).Start()` is called and no file changes occur, *Then* no `OnChange` callback fires for at least `changeDetectionStatWalkInterval` (15s).
- `.git` watch failure degrades to periodic-only, logged once.
  - *Given* `fsnotify.NewWatcher()` succeeds but `watcher.Add(filepath.Join("/tmp/wt-1", ".git"))` returns `ENOSPC`, *When* `Start()` runs, *Then* `GitWatchActive()` returns `false`, exactly one `Warn` log line is emitted, and the periodic loop still starts.
- A panicking callback doesn't take down the detector or any other registered callback.
  - *Given* two callbacks are registered via `OnChange` — the first panics unconditionally, the second increments a counter — *When* `fire()` runs (from either the periodic loop or the `.git` watch loop), *Then* the panic is recovered and logged, the second callback still runs and increments its counter, and the detector's goroutines remain alive for the next tick.
**Files**: `session/git_worktree_watcher.go`

##### Task 2.1.1a: Define the type and constructor (~5 min)
- Create `session/git_worktree_watcher.go`:
  ```go
  package session

  import (
  	"context"
  	"path/filepath"
  	"sync"
  	"time"

  	"github.com/fsnotify/fsnotify"
  	"github.com/tstapler/stapler-squad/log"
  )

  // changeDetectionStatWalkInterval is how often WorktreeChangeDetector re-checks
  // a worktree's dirty/HEAD fingerprint. Matches today's pre-pivot
  // diffStatsCacheTTL/vcsStatusCacheTTL (15s) deliberately -- this is the same
  // cadence the caches already recomputed on unconditionally, just now gating
  // an edge-triggered invalidation instead of a blind recompute.
  const changeDetectionStatWalkInterval = 15 * time.Second

  // fingerprintFunc returns the current dirty/HEAD state for one worktree.
  // Injected rather than taking a *git.GitWorktree directly so
  // WorktreeChangeDetector can be unit-tested without a real git repo.
  type fingerprintFunc func() (dirty bool, headSHA string, err error)

  // WorktreeChangeDetector watches one worktree's .git dir via fsnotify (cheap,
  // ~10-20 descriptors, mirrors session/unfinished/watcher.go's proven
  // .git-only pattern) and runs a jittered-start periodic fingerprint
  // comparison as the primary signal for plain working-tree edits that never
  // touch .git. See project_plans/diff-stats-file-watch-cache/decisions/
  // ADR-029-... for why this replaces full recursive per-directory watching.
  type WorktreeChangeDetector struct {
  	worktreePath string
  	fingerprint  fingerprintFunc
  	activeFunc   func() bool // nil means "always active"; see Story 2.1.2

  	mu       sync.Mutex
  	onChange []func()

  	gitWatcher *fsnotify.Watcher // nil if unavailable or Add() failed
  	cancel     context.CancelFunc
  	stopped    chan struct{} // closed when both goroutines have exited (or immediately if neither started)
  }

  // NewWorktreeChangeDetector never errors -- matches
  // session/unfinished/watcher.go's NewWatchDirWatcher nil-on-unavailable
  // convention for the .git sub-watch; the periodic loop always starts.
  // activeFunc, if non-nil, is checked at the top of every periodic tick
  // (before fingerprint()) -- returning false skips that tick's stat-walk
  // entirely, so a worktree nobody has asked GetSessionDiff/GetVCSStatus for
  // recently doesn't pay IsDirtyUncached's O(tracked-files) cost. See Story
  // 2.1.2 (this codebase's fix for pre-mortem.md P1 item #3).
  func NewWorktreeChangeDetector(worktreePath string, fingerprint fingerprintFunc, activeFunc func() bool) *WorktreeChangeDetector {
  	return &WorktreeChangeDetector{
  		worktreePath: worktreePath,
  		fingerprint:  fingerprint,
  		activeFunc:   activeFunc,
  		stopped:      make(chan struct{}),
  	}
  }

  // OnChange registers an invalidation callback. Must be called before Start().
  func (d *WorktreeChangeDetector) OnChange(fn func()) {
  	d.mu.Lock()
  	defer d.mu.Unlock()
  	d.onChange = append(d.onChange, fn)
  }

  // GitWatchActive reports whether the .git fsnotify watch is running.
  // Observability-only -- does not gate TTL widening (the periodic loop is
  // the primary, always-on signal post-pivot).
  func (d *WorktreeChangeDetector) GitWatchActive() bool {
  	d.mu.Lock()
  	defer d.mu.Unlock()
  	return d.gitWatcher != nil
  }

  // fire invokes every registered OnChange callback, recover()-guarding each
  // one individually -- matching this codebase's own repeated convention for
  // cross-boundary callback dispatch (session/ent_repository_backlog.go's
  // publishItemChanged, session/chain_firer.go:160, session/backlog_lifecycle.go's
  // runStuckDetector, session/autonomous_driver.go:285). This detector invokes
  // multiple independently-registered callbacks (GitWorktreeManager's own
  // diff-stats-clearing callback, plus WorkspaceService's lazily-registered
  // vcsStatusCache.Delete closure per Story 2.3.2) -- an unrecovered panic in
  // any one of them would otherwise crash the whole process (Go terminates the
  // program on an unrecovered panic in any goroutine), a disproportionate
  // failure mode for a default-off, best-effort background caching optimization.
  func (d *WorktreeChangeDetector) fire() {
  	d.mu.Lock()
  	callbacks := d.onChange
  	d.mu.Unlock()
  	for _, fn := range callbacks {
  		func() {
  			defer func() {
  				if rec := recover(); rec != nil {
  					log.Warn("worktree change detector: OnChange callback panicked (recovered)", "worktreePath", d.worktreePath, "panic", rec)
  				}
  			}()
  			fn()
  		}()
  	}
  }
  ```
- Files: `session/git_worktree_watcher.go`

##### Task 2.1.1b: `Start()` -- baseline fingerprint, `.git` watch attempt, goroutine launch (~5 min)
- Add to `session/git_worktree_watcher.go`:
  ```go
  // Start takes a baseline fingerprint, attempts the .git fsnotify watch, and
  // launches the periodic-loop goroutine (plus the fsnotify event loop, if the
  // .git watch succeeded). Never blocks past the baseline fingerprint read.
  func (d *WorktreeChangeDetector) Start() {
  	ctx, cancel := context.WithCancel(context.Background())
  	d.cancel = cancel

  	lastDirty, lastHead, err := d.fingerprint()
  	if err != nil {
  		log.Debug("worktree change detector: initial fingerprint read failed", "worktreePath", d.worktreePath, "err", err)
  	}

  	watcher, err := fsnotify.NewWatcher()
  	gitWatchStarted := false
  	if err != nil {
  		log.Warn("worktree change detector: fsnotify unavailable, falling back to periodic-check-only for this worktree", "worktreePath", d.worktreePath, "err", err)
  	} else if addErr := watcher.Add(filepath.Join(d.worktreePath, ".git")); addErr != nil {
  		log.Warn("worktree change detector: .git fsnotify watch failed to start, falling back to periodic-check-only for this worktree", "worktreePath", d.worktreePath, "err", addErr)
  		_ = watcher.Close()
  	} else {
  		d.mu.Lock()
  		d.gitWatcher = watcher
  		d.mu.Unlock()
  		gitWatchStarted = true
  	}

  	var wg sync.WaitGroup
  	wg.Add(1)
  	go d.periodicLoop(ctx, &wg, lastDirty, lastHead)
  	if gitWatchStarted {
  		wg.Add(1)
  		go d.gitWatchLoop(ctx, &wg, watcher)
  	}
  	go func() {
  		wg.Wait()
  		close(d.stopped)
  	}()
  }
  ```
- Files: `session/git_worktree_watcher.go`

##### Task 2.1.1c: `periodicLoop` -- jittered start, edge-triggered fire (~5 min)
- Add:
  ```go
  func (d *WorktreeChangeDetector) periodicLoop(ctx context.Context, wg *sync.WaitGroup, lastDirty bool, lastHead string) {
  	defer wg.Done()

  	// Jitter the first tick so 134 worktrees whose Setup() calls land in the
  	// same burst (e.g. process restart) don't all stat-walk on the same
  	// wall-clock tick -- mirrors GitWorktreeManager.PrimeDirtyCacheJitter's
  	// existing rand.Int63n staggering idiom.
  	initialDelay := time.Duration(rand.Int63n(int64(changeDetectionStatWalkInterval)))
  	timer := time.NewTimer(initialDelay)
  	defer timer.Stop()

  	for {
  		select {
  		case <-ctx.Done():
  			return
  		case <-timer.C:
  			dirty, head, err := d.fingerprint()
  			if err != nil {
  				log.Debug("worktree change detector: fingerprint read failed, skipping tick", "worktreePath", d.worktreePath, "err", err)
  			} else if dirty != lastDirty || head != lastHead {
  				log.Debug("worktree change detector: fingerprint changed, invalidating caches", "worktreePath", d.worktreePath, "dirtyChanged", dirty != lastDirty, "headChanged", head != lastHead)
  				lastDirty, lastHead = dirty, head
  				d.fire()
  			}
  			timer.Reset(changeDetectionStatWalkInterval)
  		}
  	}
  }
  ```
- Add `"math/rand"` to the import block.
- Files: `session/git_worktree_watcher.go`

##### Task 2.1.1d: `gitWatchLoop` -- inline invalidation, no debounce (~5 min)
- Add:
  ```go
  // gitWatchLoop mirrors session/unfinished/watcher.go's fsnotifyLoop shape.
  // No debounce: each fire() call is a cheap, idempotent invalidate, so a
  // burst of raw fsnotify events firing repeatedly costs nothing.
  func (d *WorktreeChangeDetector) gitWatchLoop(ctx context.Context, wg *sync.WaitGroup, watcher *fsnotify.Watcher) {
  	defer wg.Done()
  	defer func() { _ = watcher.Close() }()

  	for {
  		select {
  		case <-ctx.Done():
  			return
  		case event, ok := <-watcher.Events:
  			if !ok {
  				return
  			}
  			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
  				d.fire()
  			}
  			// Deliberately do not call watcher.Remove() here even for a
  			// Remove/Rename of the watched .git dir itself -- per
  			// fsnotify#40, the OS has already dropped the watch; calling
  			// Remove() on an already-gone path is at best a no-op and at
  			// worst risks the PR#73 deadlock. Stop() (context cancellation)
  			// is the only teardown path for this watcher.
  		case _, ok := <-watcher.Errors:
  			if !ok {
  				return
  			}
  			// Errors channel surfaces ErrEventOverflow -- advisory only per
  			// pitfalls.md; the periodic loop remains authoritative.
  		}
  	}
  }
  ```
- Files: `session/git_worktree_watcher.go`

##### Task 2.1.1e: `Stop()` (~3 min)
- Add:
  ```go
  // Stop cancels both goroutines and blocks until they have exited. Safe to
  // call multiple times or on a detector that failed to start any goroutine.
  func (d *WorktreeChangeDetector) Stop() {
  	if d.cancel != nil {
  		d.cancel()
  	}
  	<-d.stopped
  }
  ```
- Files: `session/git_worktree_watcher.go`

##### Task 2.1.1f: Unit tests -- start/stop, fsnotify-unavailable fallback, edge-triggering (~5 min, may split into 2 tasks if needed)
- New file `session/git_worktree_watcher_test.go`:
  - `TestWorktreeChangeDetector_FiresOnDirtyFlip`: fake `fingerprintFunc` returning `(false, "sha1", nil)` then, after a manual trigger, `(true, "sha1", nil)`; assert `OnChange` fires exactly once after the flip and zero times before it (poll with a short `time.Sleep`-free channel signal from the callback, not a real 15s wait -- override the interval via an injected duration, see Task 2.1.1g).
  - `TestWorktreeChangeDetector_NoFireWhenFingerprintUnchanged`: fake fingerprint always returns the same tuple; assert zero fires across several ticks.
  - `TestWorktreeChangeDetector_GitWatchUnavailableStillRunsPeriodicLoop`: point `worktreePath` at a path with no `.git` subdirectory (so `Add()` fails with ENOENT); assert `GitWatchActive()` is `false` but the periodic loop still fires on a fingerprint change.
  - `TestWorktreeChangeDetector_StopReleasesGoroutines`: use `go.uber.org/goleak` (already a dep per `golang-testing` conventions) around `Start()`/`Stop()` to assert no leaked goroutine.
  - `TestWorktreeChangeDetector_PanickingCallbackDoesNotStopOtherCallbacksOrDetector`: register two `OnChange` callbacks, the first panicking unconditionally and the second incrementing a counter; trigger a fingerprint flip; assert the counter still increments and a second, subsequent flip still fires both callbacks again (proving the detector's goroutine survived the panic, not just that one `fire()` call tolerated it).
- Files: `session/git_worktree_watcher_test.go`

##### Task 2.1.1g: Make the periodic interval injectable for tests (~3 min)
- Add an unexported field `statWalkInterval time.Duration` to `WorktreeChangeDetector`, defaulted in the constructor to `changeDetectionStatWalkInterval`, and a test-only setter `func (d *WorktreeChangeDetector) setStatWalkInterval(dur time.Duration)` (unexported, same-package test file only) so Task 2.1.1f's tests run in milliseconds instead of waiting out a real 15s tick -- per this repo's `deterministic-fast-tests` skill, never sleep out a real interval in a test.
- Files: `session/git_worktree_watcher.go`, `session/git_worktree_watcher_test.go`

### Story 2.1.2: Skip the periodic tick's real work when nobody has asked recently

**Goal** (resolves pre-mortem.md P1 item #3): a truly idle/backgrounded worktree — one neither `GetSessionDiff` nor `GetVCSStatus` has been called for since it went cold — must not pay `IsDirtyUncached`'s O(tracked-files) stat-walk cost on every tick just because a detector is running. "No caller, no work" (today's behavior) must hold even with the flag on.

**As a** genuinely idle session, **I want** my periodic tick to do nothing but reschedule when nobody is polling my diff/status, **so that** this feature's own background cost never offsets or exceeds the cache-hit savings it exists to deliver.

**Design**: `WorktreeChangeDetector` gains an optional injected `activeFunc func() bool` (same shape/injection style as `fingerprintFunc` — nil-safe, defaults to "always active" so Epic 2.1's existing tests that don't set it are unaffected). `periodicLoop` calls `activeFunc()` first, before `fingerprint()`, on every tick; if it returns `false`, the tick does nothing but reset the timer — `fingerprint()` (and therefore `IsDirtyUncached`'s stat-walk) is never invoked that tick. This is the "cheap check, not the full walk" the pre-mortem calls for: `activeFunc` reads a plain `time.Time` field under a mutex, no filesystem access.

`GitWorktreeManager` (Epic 2.2) supplies the real `activeFunc`, backed by a new `lastRequestedAt time.Time` field it updates whenever *either* RPC's cache actually gets asked for (see Story 2.2.2) — this is the "does a cache entry currently exist" signal the pre-mortem asks for, generalized slightly to "has been asked within `coldActivityThreshold`" so a worktree that goes idle mid-session still stops paying the cost rather than being considered permanently "warm" from one request months ago.

**Acceptance Criteria**:
- A tick with `activeFunc` returning `false` never calls `fingerprint()`.
  - *Given* a `WorktreeChangeDetector` started with an `activeFunc` that always returns `false` and a `fingerprintFunc` that increments a call counter, *When* several `statWalkInterval` ticks elapse, *Then* the fingerprint call counter stays at 1 (only the `Start()`-time baseline read, which happens unconditionally before the first tick — see Task 2.1.1b) and no `OnChange` callback ever fires.
- A tick with `activeFunc` returning `true` (or unset) behaves exactly as Epic 2.1's existing tests expect — no regression to the edge-triggered fire behavior.
- The gate is re-evaluated every tick, not just once — a worktree that goes cold mid-session stops paying the cost on the very next tick, and one that goes warm again resumes on the next tick after that (no "sticky cold" state, no missed change: `lastRequestedAt` being touched by an incoming RPC call is independent of and races harmlessly with the periodic loop's read of it, since a cache-miss RPC call recomputes directly rather than depending on the periodic loop having already fired).

**Files**: `session/git_worktree_watcher.go`, `session/git_worktree_watcher_test.go`

##### Task 2.1.2a: Add `activeFunc` and gate the tick (~4 min)
- Add a field `activeFunc func() bool` to `WorktreeChangeDetector` (Task 2.1.1a's struct), and a parameter/setter to wire it in — extend `NewWorktreeChangeDetector`'s signature to `NewWorktreeChangeDetector(worktreePath string, fingerprint fingerprintFunc, activeFunc func() bool) *WorktreeChangeDetector` (nil `activeFunc` is valid and treated as "always active", so existing call sites/tests from Task 2.1.1f that don't care about this gate pass `nil`).
- In `periodicLoop` (Task 2.1.1c), at the top of the `case <-timer.C:` branch, before calling `d.fingerprint()`:
  ```go
  case <-timer.C:
  	if d.activeFunc != nil && !d.activeFunc() {
  		timer.Reset(changeDetectionStatWalkInterval)
  		continue
  	}
  	dirty, head, err := d.fingerprint()
  	...
  ```
- Files: `session/git_worktree_watcher.go`

##### Task 2.1.2b: Unit test — cold worktree never pays the stat-walk cost (~4 min)
- `TestWorktreeChangeDetector_ColdWorktree_SkipsFingerprintOnPeriodicTick`: construct with a fingerprint closure that increments an `int32` counter (via `atomic.AddInt32`) and an `activeFunc` that always returns `false`; use the injectable short interval from Task 2.1.1g; let several ticks elapse (channel-signaled, not `time.Sleep`-timed, per `deterministic-fast-tests`); assert the counter never exceeds 1 (the unconditional `Start()`-time baseline read) and no `OnChange` fire is observed.
- `TestWorktreeChangeDetector_WarmWorktree_StillFingerprintsEveryTick`: same shape with `activeFunc` always `true`, asserting the counter does advance — a regression guard proving Task 2.1.2a's gate doesn't accidentally suppress the warm case too.
- Files: `session/git_worktree_watcher_test.go`

---

### Epic 2.2: Wire the detector into `GitWorktreeManager`

**Goal**: `Setup()` starts it (flag-gated), `Cleanup()`/`Remove()` stop it, `DiffStatsFresh()` widens its TTL when active.

#### Story 2.2.1: Start/stop lifecycle
**As a** live session, **I want** the detector's lifecycle bound to my worktree's own lifecycle, **so that** no goroutine or fd outlives my session.
**Acceptance Criteria**:
- Flag off: `Setup()` never constructs a detector.
  - *Given* `config.GetFeatureFlagWithDefault("vcs:worktree-change-detection", false)` returns `false`, *When* `GitWorktreeManager.Setup()` runs, *Then* `gm.changeDetector` stays `nil` and `gm.changeDetectionActive` stays `false`.
- Flag on: `Cleanup()` stops the detector before delegating to the underlying worktree.
  - *Given* the flag is `true` and `Setup()` already started a detector for `/tmp/wt-1`, *When* `Cleanup()` is called, *Then* `Stop()` returns (goroutines exited) before `wt.Cleanup()` runs, and `gm.changeDetector` is set back to `nil`.
**Files**: `session/git_worktree_manager.go`

##### Task 2.2.1a: Add fields and the flag-name duplicate constant (~2 min)
- Add to `GitWorktreeManager` struct: `changeDetector *WorktreeChangeDetector` and `changeDetectionActive bool` (both guarded by `gm.mu`, same as `diffStatsAt`).
- Add near `diffStatsCacheTTL`:
  ```go
  // worktreeChangeDetectionFlagName must stay byte-identical to
  // server/services/feature_flag_service.go's constant of the same name --
  // session cannot import server/services (see session/instance_tmux.go:892-894
  // for the established reason), so this is a deliberate duplication, not a typo.
  const worktreeChangeDetectionFlagName = "vcs:worktree-change-detection"

  // diffStatsCacheTTLWithChangeDetection is used instead of diffStatsCacheTTL
  // when a WorktreeChangeDetector is active for this worktree (see
  // DiffStatsFresh). 5 minutes matches session/unfinished.Scanner's own
  // backstop-ticker interval -- the periodic stat-walk (15s) and .git watch
  // are the real freshness mechanism; this is purely a backstop in case both
  // somehow stop firing.
  const diffStatsCacheTTLWithChangeDetection = 5 * time.Minute
  ```
- Files: `session/git_worktree_manager.go`

##### Task 2.2.1b: Start the detector in `Setup()` (~5 min)
- Modify `Setup()`:
  ```go
  func (gm *GitWorktreeManager) Setup() error {
  	wt := gm.GetWorktree()
  	if wt == nil {
  		return fmt.Errorf("git worktree not initialized")
  	}
  	if err := wt.Setup(); err != nil {
  		return err
  	}
  	if config.LoadConfig().GetFeatureFlagWithDefault(worktreeChangeDetectionFlagName, false) {
  		gm.startChangeDetector(wt)
  	}
  	return nil
  }

  // startChangeDetector constructs and starts a WorktreeChangeDetector,
  // registering GitWorktreeManager's own diff-stats-clearing callback before
  // any external subscriber (e.g. WorkspaceService) gets a chance to register
  // its own -- order doesn't affect correctness here (both callbacks are
  // independent, idempotent invalidations) but matches architecture.md's
  // documented ordering.
  func (gm *GitWorktreeManager) startChangeDetector(wt *git.GitWorktree) {
  	detector := NewWorktreeChangeDetector(wt.GetWorktreePath(), func() (bool, string, error) {
  		dirty, err := wt.IsDirtyUncached()
  		if err != nil {
  			return false, "", err
  		}
  		sha, shaErr := gm.GetCurrentCommitSHA()
  		if shaErr != nil {
  			return false, "", shaErr
  		}
  		return dirty, sha, nil
  	}, gm.hasRecentActivity) // activeFunc -- see Story 2.2.2
  	detector.OnChange(func() {
  		gm.mu.Lock()
  		gm.diffStatsAt = time.Time{}
  		gm.mu.Unlock()
  	})
  	detector.Start()

  	gm.mu.Lock()
  	gm.changeDetector = detector
  	gm.changeDetectionActive = true
  	gm.mu.Unlock()
  }
  ```
- Add `"github.com/tstapler/stapler-squad/config"` to the import block.
- Files: `session/git_worktree_manager.go`

##### Task 2.2.1c: Stop the detector in `Cleanup()`/`Remove()` (~4 min)
- Modify both:
  ```go
  func (gm *GitWorktreeManager) Cleanup() error {
  	gm.stopChangeDetector()
  	wt := gm.GetWorktree()
  	if wt == nil {
  		return nil
  	}
  	return wt.Cleanup()
  }

  func (gm *GitWorktreeManager) Remove() error {
  	gm.stopChangeDetector()
  	wt := gm.GetWorktree()
  	if wt == nil {
  		return fmt.Errorf("git worktree not initialized")
  	}
  	return wt.Remove()
  }

  // stopChangeDetector is the single chokepoint for change-detector teardown,
  // called from both Cleanup() and Remove() -- every existing session/instance.go
  // call site already funnels through one of these two methods (grep-confirmed:
  // lines 1443, 1703, 2017, 2058 for Cleanup; 1940, 2133 for Remove), so hooking
  // in here covers every teardown path with no per-call-site duplication. No-op
  // if no detector was ever started.
  func (gm *GitWorktreeManager) stopChangeDetector() {
  	gm.mu.Lock()
  	detector := gm.changeDetector
  	gm.changeDetector = nil
  	gm.changeDetectionActive = false
  	gm.mu.Unlock()
  	if detector != nil {
  		detector.Stop()
  	}
  }
  ```
- Files: `session/git_worktree_manager.go`

##### Task 2.2.1d: Widen `DiffStatsFresh()`'s TTL when active (~2 min)
- Modify:
  ```go
  func (gm *GitWorktreeManager) DiffStatsFresh() bool {
  	gm.mu.RLock()
  	defer gm.mu.RUnlock()
  	if gm.diffStatsAt.IsZero() {
  		return false
  	}
  	ttl := diffStatsCacheTTL
  	if gm.changeDetectionActive {
  		ttl = diffStatsCacheTTLWithChangeDetection
  	}
  	return time.Since(gm.diffStatsAt) < ttl
  }
  ```
- Files: `session/git_worktree_manager.go`

##### Task 2.2.1e: Add `WorktreeChangeDetectionActive`/`GitWatchActive` accessors and `OnChange` passthrough on `GitWorktreeManager` (~3 min)
- Add:
  ```go
  // WorktreeChangeDetectionActive reports whether a WorktreeChangeDetector is
  // currently running for this worktree (flag on and Setup() succeeded in
  // starting one). Used by WorkspaceService to decide its own cache's TTL and
  // by CreateDebugSnapshot for troubleshooting.
  func (gm *GitWorktreeManager) WorktreeChangeDetectionActive() bool {
  	gm.mu.RLock()
  	defer gm.mu.RUnlock()
  	return gm.changeDetectionActive
  }

  // GitWatchActive reports whether the .git fsnotify sub-watch specifically is
  // running (observability-only -- see WorktreeChangeDetector.GitWatchActive's
  // doc comment for why this doesn't gate TTL widening).
  func (gm *GitWorktreeManager) GitWatchActive() bool {
  	gm.mu.RLock()
  	detector := gm.changeDetector
  	gm.mu.RUnlock()
  	return detector != nil && detector.GitWatchActive()
  }

  // OnChange registers fn to run whenever the active WorktreeChangeDetector
  // fires. No-op if change detection isn't active for this worktree -- the
  // caller (WorkspaceService) doesn't need to check WorktreeChangeDetectionActive
  // itself first.
  func (gm *GitWorktreeManager) OnChange(fn func()) {
  	gm.mu.RLock()
  	detector := gm.changeDetector
  	gm.mu.RUnlock()
  	if detector != nil {
  		detector.OnChange(fn)
  	}
  }
  ```
- Files: `session/git_worktree_manager.go`

##### Task 2.2.1f: Add the new methods to the `GitManager` interface (~2 min)
- Add `WorktreeChangeDetectionActive() bool`, `GitWatchActive() bool`, `OnChange(fn func())` to the `GitManager` interface (`session/git_worktree_manager.go:339-369`). (`RecordRequest()`, needed by Task 2.2.2b, is added to this same interface at that later task rather than here, since it doesn't exist yet at this point in the plan's dependency order — Epic 2.2's `hasRecentActivity`/`lastRequestedAt` machinery is Story 2.2.2, after this task.)
- Check/update any existing test fake implementing `GitManager` (grep `GitManager interface` implementers, e.g. a mock in `session/*_test.go`) to add no-op implementations.
- Files: `session/git_worktree_manager.go`, plus whatever test-fake file(s) `grep -rn "GitManager = " session/*_test.go` turns up.

##### Task 2.2.1g: Unit tests -- flag-off unchanged, flag-on lifecycle, no leak (~5 min)
- `TestGitWorktreeManager_Setup_FlagOff_NoDetectorConstructed`: assert `changeDetector == nil`, `DiffStatsFresh()` uses 15s TTL.
- `TestGitWorktreeManager_Setup_FlagOn_DetectorStartedAndStoppedByCleanup`: set the flag via `config` test helper, call `Setup()`, assert `WorktreeChangeDetectionActive()` is `true`; call `Cleanup()`, assert it's `false`.
- `TestGitWorktreeManager_SetupCleanupCycle_NoGoroutineLeak`: wrap 20 Setup/Cleanup cycles (flag on) in `goleak.VerifyNone(t)` -- this is the requirements.md Success-Metrics-mandated leak test.
- Files: `session/git_worktree_manager_test.go`

### Story 2.2.2: `lastRequestedAt` — the "does a cache entry currently exist" signal

**Goal** (resolves pre-mortem.md P1 item #3, part (b)): give `startChangeDetector`'s `activeFunc` argument (Task 2.2.1b) a real, cheap signal for "has either `GetSessionDiff` or `GetVCSStatus` actually asked for this worktree recently" — reusing the same natural "cache entry exists" markers the two caches already have (`diffStatsAt` non-zero locally; a `vcsStatusCache` entry existing in `WorkspaceService`, cross-package), rather than inventing new cross-cutting infrastructure.

**As a** `WorktreeChangeDetector`'s periodic tick, **I want** a cheap `bool` telling me whether anyone has asked for this worktree's diff/status within a bounded recent window, **so that** I can skip the stat-walk entirely for a worktree nobody is polling.

**Acceptance Criteria**:
- `hasRecentActivity()` is `true` immediately after `UpdateDiffStats` computes a fresh answer (the existing `diffStatsAt`-setting path), and stays `true` for `coldActivityThreshold` afterward even if a subsequent change-fire zeroes `diffStatsAt` itself (since a change invalidating the *value* doesn't mean nobody is *asking* — `lastRequestedAt` is a distinct field from `diffStatsAt`, never zeroed by invalidation).
  - *Given* `UpdateDiffStats` ran at `t0`, *When* `hasRecentActivity()` is called at `t0 + 4m`, *Then* it returns `true` (within the 5-minute `coldActivityThreshold`); at `t0 + 6m` with no further requests, it returns `false`.
- A `GetVCSStatus` call for this worktree also counts as activity, even though `vcsStatusCache` itself lives in a different package/service.
  - *Given* only `GetVCSStatus` (never `GetSessionDiff`) has been called for a worktree, at `t0`, *When* `hasRecentActivity()` is called at `t0 + 1m`, *Then* it still returns `true` — proving the signal isn't scoped to only one of the two RPCs.
**Files**: `session/git_worktree_manager.go`, `session/instance_worktree.go`

##### Task 2.2.2a: Add `lastRequestedAt` and `hasRecentActivity()` to `GitWorktreeManager` (~4 min)
- Add field `lastRequestedAt time.Time` to `GitWorktreeManager` (guarded by `gm.mu`, same as `diffStatsAt`).
- Add const next to `diffStatsCacheTTLWithChangeDetection` (Task 2.2.1a): `const coldActivityThreshold = diffStatsCacheTTLWithChangeDetection // 5 minutes -- deliberately matches the widened TTL: if nobody has asked within one full widened-TTL window, there is by definition no live cache entry left to protect the freshness of.`
- In `UpdateDiffStats` (the existing method that sets `gm.diffStatsAt = time.Now()` on a fresh compute), add `gm.lastRequestedAt = time.Now()` alongside it, under the same `gm.mu` critical section — set unconditionally on every call (cache-hit *and* cache-miss), not just on recompute, since a cache-hit is still evidence someone is actively polling.
- Add:
  ```go
  // hasRecentActivity reports whether either GetSessionDiff or GetVCSStatus
  // has asked for this worktree within coldActivityThreshold. Backs
  // WorktreeChangeDetector's activeFunc (Story 2.1.2) -- a cheap mutex-guarded
  // time.Time read, not a filesystem stat-walk, so calling it every periodic
  // tick costs nothing. Distinct from diffStatsAt: a change-fire zeroes
  // diffStatsAt (the cached *value* is stale) but must not zero
  // lastRequestedAt (the fact that someone is *asking* is unrelated to
  // whether the last answer given is still valid).
  func (gm *GitWorktreeManager) hasRecentActivity() bool {
  	gm.mu.RLock()
  	defer gm.mu.RUnlock()
  	return !gm.lastRequestedAt.IsZero() && time.Since(gm.lastRequestedAt) < coldActivityThreshold
  }
  ```
- Files: `session/git_worktree_manager.go`

##### Task 2.2.2b: `Instance.RecordVCSStatusRequest()` passthrough for the cross-package RPC (~2 min)
- Add near `OnWorktreeChange` (Task 2.3.1a, `session/instance_worktree.go`):
  ```go
  // RecordVCSStatusRequest marks this instance's worktree as recently active
  // for WorktreeChangeDetector's idle-gating (Story 2.2.2) -- called by
  // WorkspaceService.GetVCSStatus on every call (hit or miss) so a workdir
  // being polled only via GetVCSStatus (never GetSessionDiff) still counts
  // as "has a cache entry" for the periodic tick's activeFunc check. No-op if
  // there's no worktree.
  func (i *Instance) RecordVCSStatusRequest() {
  	i.gitManager.RecordRequest()
  }
  ```
- Add a corresponding `RecordRequest()` method on `GitWorktreeManager` (sets `lastRequestedAt` the same way `UpdateDiffStats` does in Task 2.2.2a — factor the two into one unexported `gm.touchRequestedAt()` helper to avoid duplicating the mutex/time.Now() pair) and to the `GitManager` interface (alongside the three methods added in Task 2.2.1f).
- Files: `session/instance_worktree.go`, `session/git_worktree_manager.go`

##### Task 2.2.2c: Unit test — `GetVCSStatus`-only activity keeps the gate open (~3 min)
- `TestGitWorktreeManager_HasRecentActivity_TracksBothRPCPaths`: table test — (a) only `UpdateDiffStats` called, assert `hasRecentActivity()` true then false after threshold; (b) only `RecordRequest()` called (proxy for `GetVCSStatus`-only traffic), assert the same true-then-false shape; (c) a change-fire zeroing `diffStatsAt` between two `RecordRequest()` calls does not reset `lastRequestedAt`.
- Files: `session/git_worktree_manager_test.go`

---

### Epic 2.3: `Instance` passthroughs and `WorkspaceService` wiring

#### Story 2.3.1: `Instance` exposes the detector to `server/services`
**As a** `WorkspaceService`, **I want** a narrow, `fsnotify`-ignorant way to subscribe to worktree changes, **so that** I never need to know `WorktreeChangeDetector`/`GitWorktreeManager` exist.
**Acceptance Criteria**:
- `Instance.OnWorktreeChange` delegates without panicking when there's no worktree.
  - *Given* a directory-mode `Instance` with `HasGitWorktree() == false`, *When* `OnWorktreeChange(fn)` is called, *Then* it returns without calling `fn` and without panicking (delegates to `gm.OnChange`, which is itself a no-op when `changeDetector` is nil).
**Files**: `session/instance_worktree.go`

##### Task 2.3.1a: Add the two `Instance` passthroughs (~3 min)
- Add near `GetGitWorktree`/`HasGitWorktree` (`session/instance_worktree.go:443-453`):
  ```go
  // OnWorktreeChange registers fn to run whenever this instance's worktree
  // change-detector fires. No-op if change detection isn't active (flag off,
  // detector failed, or no worktree at all).
  func (i *Instance) OnWorktreeChange(fn func()) {
  	i.gitManager.OnChange(fn)
  }

  // WorktreeChangeDetectionActive reports whether change detection is active
  // for this instance's worktree -- see GitWorktreeManager.WorktreeChangeDetectionActive.
  func (i *Instance) WorktreeChangeDetectionActive() bool {
  	return i.gitManager.WorktreeChangeDetectionActive()
  }

  // WorktreeGitWatchActive reports whether the .git fsnotify sub-watch
  // specifically is running -- observability-only, see debug_snapshot.go.
  func (i *Instance) WorktreeGitWatchActive() bool {
  	return i.gitManager.GitWatchActive()
  }
  ```
- Files: `session/instance_worktree.go`

#### Story 2.3.2: `WorkspaceService` widens `vcsStatusCache`'s TTL when a detector backs the entry
**As a** `GetVCSStatus` caller, **I want** the cache to stay 15s-fresh unless a real change-detector is watching, **so that** the widened TTL never silently applies to a workdir with no invalidation signal.
**Acceptance Criteria**:
- Cache-miss path subscribes lazily, once per workdir.
  - *Given* `GetVCSStatus("/tmp/wt-1")` is called twice in a row for the same session (flag on), *When* the second call happens, *Then* `watchedWorkdirs.Load("/tmp/wt-1")` already exists and `instance.OnWorktreeChange` was called exactly once (not twice).
- Widened TTL only applies when the entry's own detector was active at store time.
  - *Given* `changeDetectionActive` was `false` when a `/tmp/wt-2` entry was cached (jj-managed worktree, or flag off), *When* `GetVCSStatus("/tmp/wt-2")` is called 20 seconds later, *Then* it recomputes (does not serve the 20s-old cached value) because the entry's stored TTL was 15s, not 5 minutes.
**Files**: `server/services/workspace_service.go`

##### Task 2.3.2a: Add `watchedWorkdirs` field and `vcsStatusCacheEntry.changeDetectionActive` (~2 min)
- Add `watchedWorkdirs sync.Map // map[string]struct{}` next to `vcsStatusCache` (`workspace_service.go:78-81`).
- Add field to `vcsStatusCacheEntry` (`:51-55`): `changeDetectionActive bool`.
- Add const next to `vcsStatusCacheTTL` (`:59-61`):
  ```go
  // vcsStatusCacheTTLWithChangeDetection mirrors
  // session.diffStatsCacheTTLWithChangeDetection -- see that constant's doc
  // comment for the 5-minute/Scanner-backstop rationale.
  const vcsStatusCacheTTLWithChangeDetection = 5 * time.Minute
  ```
- Files: `server/services/workspace_service.go`

##### Task 2.3.2b: Freshness check honors the per-entry flag (~2 min)
- Modify the cache-hit check (`:154-156`):
  ```go
  if cached, ok := ws.vcsStatusCache.Load(workDir); ok {
  	entry := cached.(vcsStatusCacheEntry)
  	ttl := vcsStatusCacheTTL
  	if entry.changeDetectionActive {
  		ttl = vcsStatusCacheTTLWithChangeDetection
  	}
  	if time.Since(entry.cachedAt) < ttl {
  ```
- Files: `server/services/workspace_service.go`

##### Task 2.3.2c: Store the flag at cache-miss time and subscribe lazily (~5 min)
- After the existing `ws.vcsStatusCache.Store(workDir, ...)` (`:219`), find the resolved `instance` (already obtained earlier in `GetVCSStatus` via `ws.liveFinder`/`findInstanceFast` per architecture.md's data-flow table -- reuse that existing local variable, do not do a second lookup) and:
  ```go
  active := false
  if instance != nil {
  	active = instance.WorktreeChangeDetectionActive()
  }
  ws.vcsStatusCache.Store(workDir, vcsStatusCacheEntry{status: status, cachedAt: now, changeDetectionActive: active})

  if active {
  	if _, already := ws.watchedWorkdirs.LoadOrStore(workDir, struct{}{}); !already {
  		instance.OnWorktreeChange(func() {
  			ws.vcsStatusCache.Delete(workDir)
  		})
  	}
  }
  ```
- Files: `server/services/workspace_service.go`

##### Task 2.3.2d: Clear `watchedWorkdirs` alongside the existing `SwitchWorkspace` invalidation (~2 min)
- At the existing `ws.vcsStatusCache.Delete(preWorkDir)` (`:431`), add `ws.watchedWorkdirs.Delete(preWorkDir)` immediately after -- if the workdir changes out from under a session, the stale subscription for the old path should also be droppable (a later `GetVCSStatus` call for that same old path, if some other session still uses it, will re-subscribe idempotently via `LoadOrStore`, which is harmless -- the point is not leaving a permanently-`true` `watchedWorkdirs` entry for a path this particular instance no longer owns).
- Files: `server/services/workspace_service.go`

##### Task 2.3.2e: Unit tests (~5 min)
- `TestGetVCSStatus_FlagOff_UsesUnchangedTTL`: flag off end-to-end, assert cache entry's `changeDetectionActive` is `false` and a 20s-later call recomputes.
- `TestGetVCSStatus_FlagOn_SubscribesOnceLazily`: flag on, call `GetVCSStatus` twice for the same workdir, assert exactly one `OnWorktreeChange` registration (inject a counting fake `Instance`-shaped `LiveInstanceFinder`, or a real `Instance` with a real detector and count via the fired-callback side effect).
- `TestGetVCSStatus_ChangeFires_CacheInvalidated`: flag on, populate the cache, fire the registered callback directly (simulating the detector), assert the next `GetVCSStatus` call recomputes rather than serving the deleted entry.
- Files: `server/services/workspace_service_test.go`

##### Task 2.3.2f: Call `instance.RecordVCSStatusRequest()` on every `GetVCSStatus` call, hit or miss (~2 min)
- Feeds Story 2.2.2's idle-gating: a workdir polled only via `GetVCSStatus` (never `GetSessionDiff`) must still register as "recently active" so `WorktreeChangeDetector`'s periodic tick doesn't skip its stat-walk for it.
- At the top of `GetVCSStatus`, immediately after resolving `instance` (the same local variable Task 2.3.2c reuses) and before the cache-hit check (Task 2.3.2b), add:
  ```go
  if instance != nil {
  	instance.RecordVCSStatusRequest()
  }
  ```
  Called unconditionally (cache hit or miss, flag on or off) — cheap (one mutex-guarded `time.Time` write per Task 2.2.2a) and a no-op-cost `nil` check when there's no worktree; only matters in practice when change detection is active, since `hasRecentActivity()` is only ever consulted by a running detector's periodic tick.
- `TestGetVCSStatus_CacheHit_StillRecordsActivity`: flag on, prime the cache, call `GetVCSStatus` again (cache hit), assert `instance.RecordVCSStatusRequest`'s effect (`GitWorktreeManager.hasRecentActivity()`) is still `true` afterward — proving the recording isn't accidentally gated behind the cache-miss branch only.
- Files: `server/services/workspace_service.go`, `server/services/workspace_service_test.go`

---

## Phase 3: Observability, version bump, end-to-end tests

### Epic 3.1: `fsnotify` version bump

#### Story 3.1.1: Bump to v1.10.1
**As a** maintainer, **I want** the dependency current, **so that** the new watch code isn't built on a known-one-minor-behind version for no reason.
**Acceptance Criteria**:
- `go.mod`/`go.sum` reflect v1.10.1; `go build ./...` and the full existing `session/unfinished` test suite still pass unmodified.
  - *Given* `go.mod:28` currently pins `github.com/fsnotify/fsnotify v1.9.0`, *When* `go get github.com/fsnotify/fsnotify@v1.10.1 && go mod tidy` is run, *Then* `go test ./session/unfinished/...` passes with no code changes required (stack.md confirms the v1.9.0→v1.10.1 diff is bugfix/cross-platform-behavior-only, no breaking API change).
**Files**: `go.mod`, `go.sum`

##### Task 3.1.1a: Bump and verify (~3 min)
- Run `go get github.com/fsnotify/fsnotify@v1.10.1 && go mod tidy`.
- Run `go build ./... && go test ./session/unfinished/... ./session/... -run Watcher`.
- Files: `go.mod`, `go.sum`

### Epic 3.2: `CreateDebugSnapshot` exposure

#### Story 3.2.1: Surface change-detection status per session
**As a** troubleshooter, **I want** to see which sessions have an active detector, **so that** a fleet-wide fd/goroutine problem is visible without attaching a debugger.
**Acceptance Criteria**:
- A debug snapshot for a flag-on session with a running `.git` watch shows both fields `true`.
  - *Given* a session at `/tmp/wt-1` with the flag on and a successfully-started `.git` watch, *When* `CreateDebugSnapshot` is called, *Then* the resulting JSON's matching `sessions[].worktree_change_detection_active` is `true` and `worktree_git_watch_active` is `true`.
- A flag-off session shows `false`/omitted.
  - *Given* a session with the flag off, *When* `CreateDebugSnapshot` runs, *Then* `worktree_change_detection_active` is `false` and `worktree_git_watch_active` is omitted from the JSON (`omitempty`, value `false`).
**Files**: `server/services/debug_snapshot.go`

##### Task 3.2.1a: Add the two `SessionSnapshot` fields and populate them (~4 min)
- Add to `SessionSnapshot` (`debug_snapshot.go:47-66`):
  ```go
  WorktreeChangeDetectionActive bool `json:"worktree_change_detection_active"`
  WorktreeGitWatchActive        bool `json:"worktree_git_watch_active,omitempty"`
  ```
- In `collectSessionSnapshots` (`:146-...`), where `ss := SessionSnapshot{...}` is built, add:
  ```go
  ss.WorktreeChangeDetectionActive = inst.WorktreeChangeDetectionActive()
  ss.WorktreeGitWatchActive = inst.WorktreeGitWatchActive()
  ```
- Files: `server/services/debug_snapshot.go`

##### Task 3.2.1b: Extend the existing `CreateDebugSnapshot` test (~3 min)
- Extend `TestCreateDebugSnapshot_Succeeds` (`server/services/window_debug_test.go:69`) or add a sibling test asserting the new fields round-trip through the written JSON file for both a flag-on and flag-off fixture session.
- Files: `server/services/window_debug_test.go`

### Epic 3.3: End-to-end correctness tests (Success Metrics from requirements.md)

#### Story 3.3.1: Flag-off byte-for-byte-unchanged behavior
**As a** reviewer, **I want** proof this feature is purely additive when off, **so that** shipping it default-off carries zero regression risk.
**Acceptance Criteria**:
- With the flag off, `DiffStatsFresh()`'s TTL boundary is provably unchanged at exactly 15s.
  - *Given* `diffStatsAt` set to `time.Now().Add(-16 * time.Second)` and the flag off, *When* `DiffStatsFresh()` is called, *Then* it returns `false` (same as pre-feature behavior) -- and with `diffStatsAt` at `-14s`, it returns `true`.
**Files**: `session/git_worktree_manager_test.go`

##### Task 3.3.1a: Add the boundary test (~4 min)
- `TestDiffStatsFresh_FlagOff_15sTTLBoundaryUnchanged`: table test at `-14s` (fresh) and `-16s` (stale), flag off, no detector constructed.
- Files: `session/git_worktree_manager_test.go`

#### Story 3.3.2: Real file edit invalidates within the periodic-check interval, and a real git commit invalidates via the `.git` watch
**As a** developer, **I want** both the periodic stat-walk *and* the `.git` fsnotify watch to actually catch a real change end-to-end, **so that** the widened TTL is safe in practice for both signal paths Success Metric (b) names, not just in theory.
**Acceptance Criteria**:
- A real untracked-file creation inside a real temp-dir worktree fires the registered callback within one `changeDetectionStatWalkInterval` (test uses the injectable interval from Task 2.1.1g, e.g. 50ms, not the real 15s).
  - *Given* a `WorktreeChangeDetector` started against a real temp git worktree with `statWalkInterval` set to 50ms, *When* a new file `bar.txt` is written to the worktree root, *Then* the registered `OnChange` callback fires within 200ms (4 ticks' worth of margin), verified via a buffered channel the callback sends on, not a `time.Sleep` guess.
- A real git commit against a real `.git` directory fires the registered callback via the fsnotify-driven `.git` watch specifically, not via the periodic loop.
  - *Given* a `WorktreeChangeDetector` started against a real temp git worktree with a real (unfaked) `.git` fsnotify watch active (`GitWatchActive() == true`) and the periodic interval set long enough (e.g. 30s) that it cannot plausibly fire first, *When* a real commit is made (or a `git add` that touches `.git/index`), *Then* the registered `OnChange` callback fires within a short bounded time (assert within 1-2 seconds via a buffered channel + `select`/timeout, not exactly sub-second, to avoid CI flakiness) — proving the fsnotify-driven path works end-to-end, not just via a faked `fingerprintFunc` trigger (Task 2.1.1f's tests) or a forced-fallback path (`.Add()` pointed at a nonexistent path).
**Files**: `session/git_worktree_watcher_test.go`

##### Task 3.3.2a: Add the real-edit integration test (~5 min)
- `TestWorktreeChangeDetector_RealFileEdit_InvalidatesWithinInterval`: `t.TempDir()`, `git init`, one commit so `HEAD` resolves, real `fingerprintFunc` backed by `git.OpenWorktree`/`IsDirtyUncached` + `rev-parse`, short injected interval, write a new file, assert the callback channel receives within a bounded `select` with a generous-but-finite timeout (not an indefinite block).
- Files: `session/git_worktree_watcher_test.go`

##### Task 3.3.2b: Add the real-`.git`-watch integration test (~5 min)
- `TestWorktreeChangeDetector_RealGitCommit_FiresViaGitWatch`: sibling to Task 3.3.2a, same `t.TempDir()`/`git init`/initial-commit setup, but this time: set `statWalkInterval` long (e.g. 30s, effectively "won't tick during this test") so a fire can only be attributed to the `.git` watch, not the periodic loop; start the detector with its real (unfaked) `.git` fsnotify watch and assert `GitWatchActive()` is `true` before proceeding; make a real commit against the real repo (e.g. write a tracked file, `git add`, `git commit`) using the same real `fingerprintFunc` as 3.3.2a; assert the `OnChange` callback fires within 1-2 seconds via a buffered channel + bounded `select` (not `time.Sleep`, per this repo's `deterministic-fast-tests` skill) — proving the fsnotify-driven `.git` path actually works end-to-end, addressing the requirements.md Success Metric (b) sub-second-latency path that Task 3.3.2a's plain-edit test does not cover.
- Files: `session/git_worktree_watcher_test.go`

#### Story 3.3.3: No fd/goroutine leak across many Setup/Cleanup cycles (fleet-scale proxy)
**As a** long-running dev-box process, **I want** confidence at 100+-session scale, **so that** this feature doesn't reintroduce the fd-exhaustion problem the pivot was meant to avoid.
**Acceptance Criteria**:
- 150 sequential Setup/Cleanup cycles (proxy for 134 concurrent worktrees, run sequentially to keep the test fast and deterministic) leave zero leaked goroutines and zero leaked fsnotify watchers.
  - *Given* 150 iterations of `{construct a temp git worktree, GitWorktreeManager.SetWorktree, Setup() with the flag on, Cleanup()}`, *When* the loop finishes, *Then* `goleak.Find()` reports no matching goroutines and (on Linux) `/proc/self/fd` count after the loop is within a small constant of the count before the loop started (not linear in 150).
**Files**: `session/git_worktree_manager_test.go`

##### Task 3.3.3a: Add the fleet-scale leak proxy test (~5 min)
- `TestGitWorktreeManager_150SetupCleanupCycles_NoLeak`: builds on Task 2.2.1g's smaller 20-cycle test but raises the count and adds the fd-count assertion (Linux-only via `runtime.GOOS` guard, matching this repo's existing platform-conditional test patterns, e.g. grep for `runtime.GOOS == "linux"` in existing tests for the exact conditional-skip idiom to match).
- Files: `session/git_worktree_manager_test.go`
