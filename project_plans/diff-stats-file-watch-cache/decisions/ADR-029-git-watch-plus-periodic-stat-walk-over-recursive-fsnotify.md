# ADR-029: `.git`-Only fsnotify Watch + Periodic Stat-Walk, Not Full Recursive Watching

**Status**: Accepted
**Date**: 2026-09-10
**Project**: diff-stats-file-watch-cache

## Context

`GetSessionDiff`'s diff-stats cache and `GetVCSStatus`'s `vcsStatusCache` are both purely time-based (15s TTL), recomputing unconditionally whenever polled regardless of whether the worktree actually changed. The original design (`requirements.md`'s pre-pivot text, left in place for history) was a full recursive, `.gitignore`-aware fsnotify watch of each worktree's entire tree — one `fsnotify.Add()` per non-ignored directory — so both caches could widen their TTL to 5+ minutes with a real invalidation signal backing the extension, mirroring `session/unfinished/scanner.go`'s existing fsnotify-primary/ticker-backstop trade-off.

Phase 2 research (`research/stack.md` §5, `research/pitfalls.md` §2) quantified this repo's own worktree as **6,858 tracked files across 1,300 non-ignored directories** (`git ls-files | wc -l`, `git ls-files | xargs -n1 dirname | sort -u | wc -l`, run 2026-09-10). A single worktree's recursive watch already costs ~1,300 inotify watch descriptors — 16% of Linux's common `fs.inotify.max_user_watches` default of 8,192. At this feature's stated target scale (134 concurrent worktrees on the dev box this was scoped against), that's **~174,000 watch descriptors** — 21x past the default ceiling, and worse on macOS, where kqueue needs one open fd per watched entry against a default `ulimit -n` of 256.

## Decision

Drop full recursive per-directory fsnotify watching. Replace it with two cheap, already-cost-bounded signals, combined per worktree in a new `session.WorktreeChangeDetector`:

1. **`.git`-only fsnotify watch** — a handful of descriptors per worktree (matching `session/unfinished/watcher.go`'s existing, proven `.git`-dir-only convention), catching commits/checkouts/staged-index changes with sub-second latency.
2. **A jittered 15s periodic fingerprint comparison** (dirty-state via a new `GitWorktree.IsDirtyUncached()`, bypassing `IsDirtyWithHint`'s own cache, plus current HEAD SHA) — an O(tracked-files) `lstat` walk, the same asymptotic cost this codebase already pays today for `IsDirtyWithHint`/`worktreeIsDirtyFast`. This is the *primary* signal for plain working-tree edits that never touch `.git`; the fsnotify watch is a latency optimization on top of it, not a required dependency.

Both caches widen their TTL from 15s to 5 minutes only while a `WorktreeChangeDetector` is active for that worktree (flag on, `Setup()` succeeded in starting the periodic loop). Gated behind a new feature flag, `vcs:worktree-change-detection`, default off.

## Consequences

- The fd/watch-descriptor cost no longer scales with directory count watched — it scales with tracked-file count per periodic check, already proven affordable at this repo's scale by the pre-existing `IsDirtyWithHint`/`worktreeIsDirtyFast` code path. 134 worktrees now cost ~134 `.git` watches (a couple thousand descriptors total, comparable to `session/unfinished/watcher.go`'s existing fleet-wide footprint) instead of ~174,000.
- Trade-off accepted: staleness after a real edit is now bounded by the 15s periodic-check interval (plus, when the `.git` watch is active, often much less), not by true recursive per-file watching's near-instant delivery. This still meets requirements.md's Success Metrics — the periodic check's own cost is far below the ~13%+7.5% CPU baseline this feature exists to reduce, and it's the same cost this codebase already accepts today for `IsDirtyWithHint`.
- No `.gitignore`-aware matcher, no new-subdirectory-detection machinery, and no per-burst debounce timer are needed — all three were load-bearing complexity in the original recursive-watch design (per `research/features.md` §1-§2 and `research/build-vs-buy.md`) and are made moot by watching only `.git` (a single, shallow, git-managed directory) plus an edge-triggered fingerprint comparison that already naturally coalesces any burst within one 15s window.
- A plain working-tree edit is invisible to the `.git` watch by construction — the periodic check, not the fsnotify watch, is now load-bearing for correctness. If the periodic check's own goroutine were ever to stop (a bug, not an expected operating mode), the diff-stats/status caches would silently serve stale data for up to 5 minutes with no independent backstop shorter than that — this is the same trade-off `session/unfinished/scanner.go` already accepts for its own fsnotify-plus-ticker design, not a new risk class.
- Jujutsu-managed worktrees get no change-detection signal in v1 (the fingerprint's dirty-check is git-specific) and keep the unconditional 15s TTL regardless of the flag — see `implementation/plan.md`'s Pattern Decisions table for why this isn't a Strategy-pattern gap worth closing now.

## Alternatives Considered

- **Full recursive per-directory fsnotify watch of the whole worktree** (original design): rejected — infeasible at stated scale (~174,000 watch descriptors needed vs. Linux's 8,192 default), per `research/stack.md`/`research/pitfalls.md`.
- **Adopt `rjeczalik/notify` or a thin recursive-fsnotify wrapper** for the (rejected) recursive design: rejected independently in `research/build-vs-buy.md` even before the pivot — none of the surveyed libraries solve `.gitignore` filtering or debouncing, the actual bulk of that design's complexity, and `rjeczalik/notify`'s Linux backend still walks and `Add()`s per-directory internally, so it wouldn't have escaped the fd-budget problem either.
- **Rely on the `.git` watch alone, no periodic check**: rejected — misses the dominant case for a live session's diff view (plain uncommitted working-tree edits, which never touch `.git`).
- **Poll every tracked file's mtime as the sole signal, at a short interval, replacing the cache entirely**: rejected as the cache's *replacement* — this is exactly `IsDirtyWithHint`'s existing cost, fine as a periodic *invalidation trigger* (which is what this ADR adopts) but would defeat the purpose of caching at all if it replaced the cached diff/status computation itself.
