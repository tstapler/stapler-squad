# ADR-006: Dirty tracking — Tier A default (never skips the stat pass), Tier B opt-in off, asymmetric-safety fallback

**Status**: Accepted for Tier A and the fallback policy (amended in repair iteration 1 and the Phase 4 patch: `RefSnapshot` and `ScanSignature` reuse never outlive one 5-minute tick, because a branch-ref move in the common dir can produce no watched event); Tier B build pending owner decision and a kqueue spike · **Plan**: Epics 4.2, 4.3

## Context
A false "clean" hides unfinished work; a false "dirty" costs one extra scan. Today only `<repo>/.git` is watched (`scanner.go:495-509`), linked-worktree gitdirs are not, `w.Errors` is only logged (`S:564-569`), `Rename` is dropped (`S:542`). fsnotify v1.10.1 (`go.mod:28`) on darwin uses kqueue (one fd per watch; directory watches do not report in-place file writes — INFERRED, UNVERIFIED for this version). **Tier A watches only `.git/worktrees/<name>/` (HEAD, index): editing a tracked file in place never produces an event there.**

## Decision
- **Hard rule**: `DirtyGen` (Tier A) may gate only (a) the parsed-index cache and (b) reuse of committed-state fields. **Skipping the tracked-file stat pass requires Tier B armed and healthy** (off by default), so by default `HasUncommitted` runs its stat pass on every scan. The clause "an armed healthy watcher with no loss since the last full pass" applies to Tier B only. Test: Tier A armed, edit a tracked file in place, the next scan reports dirty.
- **Tier A (default, rollback unit `watch_worktree_gitdirs`)**: watch `.git/worktrees/<name>/`, handle `Errors`/overflow/`Rename`/pointer-file repos, bump `DirtyGen`.
- **Tier B (`dirty_watch_tierb`, default off, hot worktrees only, fd-budgeted)**: built only after a spike measures fd cost and in-place-edit visibility; never enabled by default; owner decision required.
- **Fallback (`FullWalkFallback`)**: a full walk is forced on: not armed, overflow/error, directory create/rename/remove, or >= 5 minutes since the last full walk. A watcher hit may return dirty immediately.
- **Untracked per-dir pruning (Story 4.2.3)** keys each directory verdict on `(dir mtime, ctime, matcher identity, exclude fingerprint, tracked-set generation)`: the stat signatures of every consulted `.gitignore`, of `.git/info/exclude` and the global excludes file, and the index signature + entry count. `.gitignore` edits, exclude edits and `git rm --cached`-style index changes (go-git storer equivalent in tests) therefore invalidate; a seeded model-based oracle with random event drops compares skip-enabled verdicts with a full walk.
- Index-signature skip and untracked pruning carry a racy-timestamp guard (mtime within 2 s of store time is untrusted), as in git.
- Worst-case false-clean window per mechanism is stated in the plan (5 minutes, equal to the backstop tick `S:253`, versus today's 30 s `diffStatCacheTTL`); engagement of the skip is measured, and counted as 0 in the CPU projection if < 10%.

## Consequences
Worst-case staleness of a missed event is bounded by 5 minutes for changes that leave every cache key unchanged; the stat-bound CPU bucket is owned by the single-stat-pass work (Story 4.2.4) or recorded as accepted, not by the watcher; Tier B may deliver little on macOS and is allowed to end as "untracked-set only".
