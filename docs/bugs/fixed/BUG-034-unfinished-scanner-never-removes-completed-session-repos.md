# BUG-034: The Unfinished-Changes Scanner Never Removes a Repo From Its Watch List, So It Keeps Full-Rescanning Every Session/Project Ever Seen [SEVERITY: Critical]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-22 — investigating why `send_control`/`SendKeys` started failing for essentially all sessions after a routine `make install-service` restart, and why available memory in the service's cgroup dropped from 2.1G to 267.5M over ~30 minutes with "high goroutine count" warnings firing continuously.
**Fixed**: 2026-07-22 — `session/unfinished/scanner.go`, `server/services/backlog_service.go`, `server/dependencies.go`
**Impact**: `session/unfinished`'s background Scanner (the git-diff-based "uncommitted changes" detector referenced elsewhere as a peer of the not-yet-built workspace-awareness feature) registers a repo path to watch via `AddRepo` whenever it's discovered, but nothing called the corresponding `RemoveRepo` when a session completed, was archived, or its worktree was deleted. The watch list only ever grew. With 130+ sessions accumulated across many projects, every ~5-minute scan tick did full, expensive work (recursive untracked-file reads, git packfile decompression for diff-stat computation) across the entire ever-growing set — including repos for sessions that finished hours or days ago. Live-observed: 63.7GB of cumulative allocation in the ~32 minutes since a service restart, against a live in-use heap of only ~44MB (this is allocation *churn*, not a classic heap leak) — consistent with repeatedly redoing large amounts of throwaway work rather than anything actually retaining memory.

## Live Symptoms (2026-07-22, ~30-45 min post-restart)

- `send_control`/`SendKeys` failing with `"cannot send keys to instance that has not been started or is paused"` for sessions confirmed alive and working correctly at the tmux/process level (tested `dotfiles`, `wedding-party-planning-games`, and an autonomous backlog session — all failed identically).
- `systemctl --user status stapler-squad`: `Memory: 36.9G (... available: 267.5M ...)`, trending down from `34.9G (... available: 2.1G ...)` roughly 30 minutes earlier — available headroom dropped by ~1.8G in that window.
- Log flooded with `"high goroutine count detected"` warnings every ~10 seconds, holding around 310-320 goroutines.
- `curl localhost:6060/debug/pprof/heap?debug=1` header: `heap profile: 588: 43964176 [1101381: 63669690272] @ heap/1048576` — 588 objects / ~44MB **currently in use**, but **1,101,381 objects / ~63.7GB allocated cumulatively** since the process started (~32 minutes prior). The gap between "tiny live heap" and "huge cumulative allocation" is the signature of high-churn, GC-heavy work — not objects being retained forever.
- `curl localhost:6060/debug/pprof/goroutine?debug=1`: the largest goroutine groups (33 each, appearing identically across four different per-instance background loop types — `ClaudeController.runStatusChangeLoop`, `CommandExecutor.executionLoop`/`waitForCommandOrDrain`, `PTYConsumer.pollLoop`, `MangleCorrelator.StartEviction`) are ordinary per-session machinery, not obviously the leak source on their own.
- `curl localhost:6060/debug/pprof/heap?debug=1`'s largest sampled allocation call stacks trace through `session/unfinished.(*Scanner).worker` → `scanRepo` → `scanWorktree` → `GoGitVCSReader.DiffShortstat` → `diffShortstatUncached` → `walkUntrackedRec` (reads every untracked file's contents recursively) and, separately, into `go-git`'s packfile decompression path (`Packfile.getNextObject`/`fillOFSDeltaObjectContentWithBuffer`, SHA1 hashing per object) via `gogitstore.SharedObjectStore` — real, CPU/allocation-heavy git object work, not a trivial stat check.

## Root Cause

`session/unfinished/scanner.go`'s `Scanner` exposes `AddRepo`/`RemoveRepo` (and a separate `AddPinnedRepo`/`RemovePinnedRepo` pair) to manage which repo paths it periodically rescans. Grepping every call site in the codebase:

```
session/unfinished/watcher.go:133:	w.scanner.AddRepo(repoPath)          // called when a repo is first discovered
session/unfinished/scanner.go:754:	s.AddRepo(repoPath)                   // inside AddPinnedRepo
session/unfinished/scanner.go:760:	s.RemoveRepo(repoPath)                // inside RemovePinnedRepo — the ONLY RemoveRepo call site
session/unfinished/scanner.go:799:	s.AddRepo(repoRoot)                   // another add path (auto-spider on session events)
```

**`RemoveRepo` was only ever called from `RemovePinnedRepo`** — an explicit, user/UI-triggered "unpin" action for the small set of repos a user has deliberately pinned. Neither the directory-walk discovery path (`WatchDirWatcher.addRepo`) nor the session-auto-spider path (`subscribeToSessionEvents`, which already listened for `EventSessionCreated`/`EventSessionUpdated` to *add* a repo) had a corresponding removal path. Nothing called `RemoveRepo` when a backlog session's worktree was cleaned up, a backlog item was archived, or a session was deleted.

The watch list was therefore monotonically growing for the lifetime of the running process. With ~130 sessions registered (per `list_sessions`' `total_count`) spanning many distinct projects, every ~5-minute scan tick (`tickInterval: 5 * time.Minute`) did real, non-trivial work — recursive untracked-file reads plus git object decompression for a diff-stat — across the **entire accumulated set**, including paths for sessions that completed hours or days ago and had no reason to still be watched. This was the dominant driver of the allocation-churn numbers observed.

**The `send_control` connection was investigated but not conclusively resolved as part of this fix** — see Not Fixed below.

## Fix Applied

Three complementary changes, closing the gap from multiple angles rather than one call site:

1. **`session/unfinished/scanner.go`, `subscribeToSessionEvents`**: now also handles `EventSessionDeleted` (previously only `Created`/`Updated`), completing the symmetric add/remove design the code was already half-built for. `sessionRepos` (the session→repo map used for auto-spider bookkeeping) is now keyed by session **UUID** instead of Title, matching what `EventSessionDeleted` actually carries (`SessionID`, with `Session` nil for delete events). A new `forgetSessionRepo` helper only calls `RemoveRepo` once no *other* tracked session still references the same repo root, so two sessions sharing one repo don't have scanning cut out from under the surviving one.
2. **`server/services/backlog_service.go`, `cleanupItemWorktreesExcept`**: the highest-volume real path that actually deletes a worktree from disk (called on every backlog rework/reopen cycle) now tells the scanner to stop watching that path immediately after a successful `git worktree remove`, via a new nil-safe `RepoWatchRemover` interface (mirroring the existing `SessionStopper` injection pattern). Wired in `server/dependencies.go` (`backlogSvc.SetRepoWatchRemover(unfinishedScanner)`).
3. **`session/unfinished/scanner.go`, new `pruneMissingRepos`**: a self-pruning backstop, independent of the two explicit call sites above — every 5 minutes, checks whether each registered repo path still exists on disk (`os.Stat`) and removes any that don't. Catches any current or future cleanup path that doesn't remember to call `RemoveRepo` explicitly, so this class of bug can't silently reappear.

## Files Affected

- `session/unfinished/scanner.go` — `subscribeToSessionEvents` (EventSessionDeleted handling), new `forgetSessionRepo`, new `pruneMissingRepos`, `sessionRepos` now keyed by UUID
- `server/services/backlog_service.go` — new `RepoWatchRemover` interface, `repoWatchRemover` field + `SetRepoWatchRemover`, wired into `cleanupItemWorktreesExcept`
- `server/dependencies.go` — wires `unfinishedScanner` as the `BacklogService`'s `RepoWatchRemover`

## Verification

- `TestScanner_forgetSessionRepo_should_removeRepo_When_NoOtherSessionReferencesIt`, `..._should_keepRepo_When_AnotherSessionStillReferencesIt`, `..._should_beNoOp_When_SessionNeverTracked` — direct unit tests of the shared-repo-safe removal logic.
- `TestScanner_subscribeToSessionEvents_should_removeRepo_When_SessionDeletedEventReceived` — end-to-end via a real `EventBus`, publishing the exact event shape `DeleteSession` actually produces.
- `TestScanner_pruneMissingRepos_should_removeRepo_When_PathNoLongerExists`, `..._should_keepRepo_When_PathStillExists` — the self-pruning backstop.
- `TestCleanupItemWorktreesExcept_should_tellScannerToStopWatching_When_WorktreeCleanupSucceeds` — a real `git worktree add` + `cleanupItemWorktreesExcept` + real `git worktree remove`, asserting `RemoveRepo` is called with the exact path and the directory is actually gone. **Verified to fail against pre-fix code**: reverting `backlog_service.go` alone makes the test file fail to compile (`SetRepoWatchRemover undefined`), confirming the fix is load-bearing for this test.
- `TestCleanupItemWorktreesExcept_should_notTellScannerToStopWatching_When_PathIsExempted` — the reopen/rework `exceptPath` skip must not report the still-in-use worktree as removed.
- `go test ./session/unfinished/... ./server/services/...` and the broader `./session/... ./server/services/...` — full suites green, no regressions.
- `go build ./...`, `golangci-lint run ./session/... ./server/...` — clean.
- **Not yet deployed to the live system as of writing** — see Not Fixed below for why.

## Not Fixed (scoped out, tracked separately)

- **The `send_control`/`Instance.started` connection to this Scanner's churn was investigated but not proven.** The two symptoms were correlated in time and both point at the same resource-pressure story, but the exact mechanism (does GC/scheduler pressure from this Scanner's churn actually cause a session-reattachment goroutine to lose a race before setting `i.started`?) wasn't traced end-to-end. If `send_control` failures persist after this fix is deployed and the churn subsides, that's a distinct bug needing its own investigation.
- **A strategy document for making the Scanner's periodic backstop staleness-aware and persisting scan results to the database** (rather than recomputing everything from a cold cache on every restart) was written separately: `docs/tasks/unfinished-scanner-event-driven-db-cache-strategy.md`. That's a genuine architecture change (new ent schema + migration) sized for the full MDD/SDD pipeline, not something folded into this fix.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: API Contract Gap, specifically the "half-built symmetric design" shape — `subscribeToSessionEvents` already existed with `Created`/`Updated` handling and a doc comment implying the natural counterpart, but `Deleted` was simply never added. The interface (`AddRepo`/`RemoveRepo`) existed and was correct; nothing enforced that every add path have a matching remove path.

**Earliest achievable enforcement**: Unit/integration tests are the practical level — "does every path that can make a repo stop being relevant also tell the scanner" is a cross-cutting behavioral property, not something a type system or lint rule can verify generically. The three layers of fix (explicit event handling, explicit call-site wiring, self-pruning backstop) are themselves the enforcement: even if a *future* fourth cleanup path is added without remembering to call `RemoveRepo`, `pruneMissingRepos` catches it within 5 minutes rather than leaking forever.

**Recurring shape**: Sixth instance today of "something is created/registered but the corresponding cleanup never fires" (alongside BUG-029's stale review sessions, BUG-030's stranded items, BUG-032's superseded PRs, BUG-033's escaped WorkingDir) — but at a different layer (a long-lived background service's own internal watch-list bookkeeping, not a backlog item's state machine). This is the strongest evidence yet that this shape is systemic to the codebase, not particular to the backlog reconciliation loop — flagged for the `quality:architecture-review` pass already recommended by every other bug fixed today, this time scoped broadly rather than just to `session/backlog_lifecycle.go`.

## Related

- Backlog item `e1fb6825-39b2-4f06-9bf8-c9d1678a6824` ("workspace peer awareness") explicitly scoped itself against "the existing `session/unfinished/` git-diff-based scanner" as a sibling mechanism — this is that same scanner.
- `docs/tasks/unfinished-scanner-event-driven-db-cache-strategy.md` — the follow-up architecture strategy for closing the remaining "full rescan on every restart" gap.
