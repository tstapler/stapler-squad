# Requirements: diff-stats-file-watch-cache

**Date**: 2026-09-10
**Type**: feature addition
**Complexity**: 3 — system design

## Problem Statement

`GetSessionDiff` and `GetVCSStatus` each cache their expensive git computations
(full working-tree diff + ahead/behind commit walk; VCS status) behind a
short, purely time-based TTL (15s each — `GitWorktreeManager.DiffStatsFresh`,
added this session, and `WorkspaceService.vcsStatusCache`, pre-existing).
15s was chosen only because there is no invalidation signal for "did anything
actually change" — a fresh Pyroscope CPU profile (15-minute window) showed
`GetSessionDiff`/`Instance.UpdateDiffStats` alone at ~13% of app-wide CPU,
comparable to the entire background `unfinished.Scanner`. A file-watcher that
knows when a worktree's tracked files actually change would let both caches
be extended to a much longer TTL (backstop only, not the primary freshness
mechanism) with no loss of correctness — the same trade the `unfinished`
package's own Scanner already makes (fsnotify primary, ticker backstop), but
scoped to a single worktree per live `Instance` rather than fanning out
across every tracked repo.

## Baseline

Today: both caches recompute unconditionally whenever their 15s TTL expires
and something calls them, regardless of whether the worktree actually
changed. For an idle worktree (the common case for a background session or
an unfocused split-pane — see this session's earlier frontend fix reducing
unfocused-pane polling from 60s to 600s), this is pure waste: a full diff/status
recompute for a worktree that hasn't been touched since the last one.

## Users / Consumers

- `server/services/session_service.go`'s `GetSessionDiff` RPC (frontend
  `useSessionVcs`'s `fetchDiff`, on mount/manual refresh).
- `server/services/workspace_service.go`'s `GetVCSStatus` RPC (frontend
  `useSessionVcs`'s `fetchStatus`, on mount + focus-aware fallback interval).
- Internally: `session.Instance` (owns `GitWorktreeManager`, one per live
  session) and `WorkspaceService` (owns `vcsStatusCache`, keyed by workdir
  path, shared across all sessions pointing at that path).

## Success Metrics

- Both caches' effective TTL can be raised to 5+ minutes (matching the
  Scanner's own backstop-ticker interval) without a real edit ever being
  invisible for longer than the watcher's own latency (sub-second in
  practice — bounded by fsnotify event delivery + a small debounce, not by
  the TTL).
- `GetSessionDiff`'s and `GetVCSStatus`'s combined share of app-wide CPU
  (measured via the same Pyroscope `SelectSeries`/`SelectMergeProfile`
  methodology used earlier this session) drops measurably from the ~13% + 7.5%
  baseline for a fleet of idle/background worktrees, without new correctness
  regressions (a diff/status genuinely wrong for longer than one debounce
  window after a real edit).
- No fd or goroutine leak across process lifetime — watcher count == live
  worktree count at all times, verified by a test asserting cleanup on
  `Instance` pause/destroy.

## Appetite

Medium (1–2 weeks). *(User explicitly chose the wider scope: share one
file-watcher per worktree across both the diff-stats cache and
`GetVCSStatus`'s cache, rather than a smaller single-cache Small-appetite
version.)*

## Constraints

- Must not touch `session/unfinished/scanner.go`/`watcher.go`'s own fsnotify
  usage — that subsystem watches many repos for a different purpose
  (background "unfinished work" discovery across the whole fleet) and
  already has its own tuned backstop-ticker trade-off documented in this
  session's earlier `perf:make-it-faster` work. This feature is scoped to
  the per-`Instance`/per-workdir caches only.
- Must ship gated behind this codebase's existing generic feature-flag
  mechanism (`server/services/feature_flag_service.go`'s `knownFeatureFlags`
  + `config.GetFeatureFlagWithDefault`), not an ad hoc env var — user
  explicitly requested "wire it into the feature flagging system" over a
  bespoke env-var kill switch. Default **off** at first ship (new
  per-session OS resource at fleet scale — 134 worktrees on the dev box
  where this was scoped). The flag is flippable live to prevent *new*
  worktree setups from starting a detector; already-running detectors for
  existing sessions continue until that session is torn down (restart
  affected sessions to fully stop them) — matching the accepted, documented
  `terminal:resync-*` flag precedent for "not live-retroactive." It is not
  an instant fleet-wide kill switch.
- Must degrade gracefully if `fsnotify.NewWatcher()` or a per-directory
  `Add()` fails (permission error, platform without inotify/FSEvents
  support, fd exhaustion) — fall back to the existing pure-TTL behavior for
  that worktree only, matching `session/unfinished/watcher.go`'s existing
  `w.watcher == nil` → polling-fallback convention. Never error out or serve
  indefinitely-stale data.
- fd/goroutine budget: up to 3 goroutines per worktree (periodicLoop,
  gitWatchLoop, and the `Start()`-internal `wg.Wait`/`close(stopped)`
  goroutine) — ~400 at 134-worktree fleet scale, still far below typical
  goroutine-count limits — plus a bounded number of open watch descriptors
  per live worktree — must scale to 100+ concurrent sessions on a single dev
  box without exhausting platform fd limits (macOS default `ulimit -n` is
  often 256-2560; Linux inotify has a separate `max_user_watches` ceiling,
  commonly 8192-524288).
- ~~Must respect `.gitignore` when walking a worktree to add recursive
  watches~~ — **superseded by the Design Pivot below**: the chosen design
  drops recursive per-directory watching entirely (only `.git` is watched
  via fsnotify), so there is no gitignore-filtered walk to build, and no
  `node_modules`/build-output churn risk from watching ignored paths.

## Non-functional Requirements

- **Performance SLO**: cache-hit path (watcher active, no invalidation
  pending) must stay at the current ~0-allocation, sub-microsecond cost —
  do not reintroduce per-call overhead the earlier TTL fix just removed.
- **Scalability**: must not regress at 100+ concurrent live sessions (this
  dev box currently runs 134 worktrees under stapler-squad's own
  session-per-worktree model).
- **Security classification**: internal (local dev tool, no network-facing
  change).
- **Data residency**: not applicable.

## Design Pivot (post-Phase-2-research)

Phase 2 research (`research/stack.md`, `research/pitfalls.md`) quantified the
original full-recursive-fsnotify design as infeasible at this box's actual
scale: this repo's own worktree alone has ~1,300 non-ignored directories, so
134 concurrent worktrees would need ~174,000 fsnotify watch descriptors —
21x past Linux's default `fs.inotify.max_user_watches` (8,192), and worse on
macOS's tighter per-process fd `ulimit`. Presented with this, the user chose
to drop full recursive per-directory watching entirely in favor of a
**cheap aggregate signal**: watch `.git` only (matching
`session/unfinished/scanner.go`/`watcher.go`'s existing, proven, low-fd-cost
convention — 1 descriptor per worktree, a single non-recursive
`fsnotify.Watcher.Add()` on the `.git` directory itself, catches commits/
checkouts/staged changes immediately) **plus** a cheap periodic re-check for
plain working-tree edits that never touch `.git`.

The cheap periodic re-check does **not** need new machinery: this codebase
already has exactly the right primitive — `GoGitVCSReader`/`GitWorktreeManager`'s
`HasUncommitted`-style check is an O(tracked-files) `os.Lstat` walk (mtime/size
comparison against the index, no content read, no full diff) — already far
cheaper than the full working-tree diff + ahead/behind commit walk this
feature is trying to cache longer. Plan phase should confirm and design
around: run that cheap stat-walk on a short interval (e.g. every 10-15s) as
the trigger for invalidating the *expensive* diff-stats/vcs-status caches,
rather than building new recursive-watch/gitignore-matcher/debounce
infrastructure. This sidesteps the entire fd-budget problem, since the cost
no longer scales with directory count watched — it scales with tracked-file
count per check, same as today's existing `HasUncommitted`, which is already
proven affordable at this repo's scale.

This replaces the "Recursive watch setup"/"new-subdirectory detection"/
"debounce window" scope items below with the simpler two-signal design.
Section text below is left as originally written (with this pivot noted)
so Phase 3 planning has both the original ask and the fd-budget-driven
correction visible side by side, rather than silently rewriting history.

## Scope

### In Scope
- A shared per-worktree change-detection component (new, since `GitWorktreeManager`
  is per-`Instance` but `vcsStatusCache` is keyed by workdir path in a
  different service — needs a home reachable by both without entangling the
  two services' cache lifecycles). **Post-pivot**: this component watches
  `.git` via fsnotify (cheap, proven pattern) and runs a short-interval cheap
  stat-walk (reusing/mirroring `HasUncommitted`'s existing algorithm) instead
  of a full recursive per-file/per-directory watch.
- Invalidation hook: on a real change (from either the `.git` watch or the
  periodic stat-walk detecting a dirty-state flip or HEAD move), clear/mark-stale
  both `GitWorktreeManager`'s diff-stats cache (`diffStatsAt`/`DiffStatsFresh`)
  and `WorkspaceService.vcsStatusCache`'s entry for that workdir.
- Lifecycle wiring: start when a worktree is set up
  (`GitWorktreeManager.Setup`/equivalent), stop and release all fds/goroutines
  on `Cleanup`/`Remove`/session pause/destroy.
- New feature flag (e.g. `vcs:worktree-change-detection`, default off) gating
  whether the periodic cheap-check + `.git` watch runs at all; both caches'
  TTL only widens when the flag is on (when off, unchanged current 15s-TTL,
  watcher-less behavior).
- Extending both caches' TTL constant (only meaningfully long when
  change-detection is active and the flag is on — see Rabbit Holes for the
  flag-off-TTL question).

### Out of Scope
- ~~Recursive per-directory fsnotify watching of the full worktree~~ —
  dropped in the design pivot above; superseded by the cheap-periodic-check
  approach.
- Changing `session/unfinished/scanner.go`/`watcher.go`'s own fsnotify
  usage or backstop-ticker interval.
- Any change to what data the diff/status RPCs return — this is purely a
  caching-lifetime change, output format and semantics are unchanged.
- Watching for content *inside* a file changing at a finer grain than
  "this file's mtime/size changed" (matches existing `HasUncommitted`'s own
  stat-based dirty check — no need to diff file content to decide "maybe
  stale").

## Rabbit Holes

- **What TTL applies when the flag is off, or the `.git` watcher fails to
  start for a specific worktree?** Must stay at today's 15s in both cases —
  do not let a global "widen to 5 minutes" constant apply unconditionally,
  or a watcher failure silently produces 5-minute-stale data with no
  invalidation signal backing it. The periodic cheap-check still runs
  independently of the `.git` watch's success/failure (it's the primary
  signal now, not a backstop for it), so this rabbit hole is smaller
  post-pivot but the same discipline applies.
- ~~Debounce window for a rapid burst of file events~~ — largely moot
  post-pivot: the periodic cheap-check is already interval-based (naturally
  coalesces any burst within one interval into a single recompute), and the
  `.git` watch only needs the same coarse dirty-flag coalescing
  `session/unfinished/scanner.go`'s `cacheDirty`/`inFlight` pattern already
  uses — no new debounce-timer machinery required.
- **Where does the shared component actually live?** `GitWorktreeManager` is
  per-`Instance`; `vcsStatusCache` is a `sync.Map` inside `WorkspaceService`,
  a singleton keyed by workdir path (potentially shared across multiple
  `Instance`s if two sessions ever point at the same path, though that's
  rare in this app's worktree-per-session model). Research/plan phase must
  resolve whether it lives on `Instance`/`GitWorktreeManager` (one per
  session, simplest lifecycle) and calls into `WorkspaceService` via a
  narrow interface, or as a separate small registry keyed by workdir that
  both sides depend on.
- ~~fd ceiling under recursive watch~~ — resolved by the pivot: no longer
  applicable, since only `.git` (a handful of descriptors) is watched via
  fsnotify; the rest is a plain stat-walk with no fd cost per directory.

## Alternatives Considered

- **Pure TTL bump with no watcher** (rejected — a "much longer" TTL with no
  invalidation signal risks serving stale data indefinitely for an actively
  edited worktree).
- **Full recursive per-directory fsnotify watch of the whole worktree**
  (originally chosen, then rejected after Phase 2 research quantified the
  fd/watch-descriptor cost as infeasible at this box's scale — see Design
  Pivot above).
- **Reuse `session/unfinished`'s existing `.git`-dir-only fsnotify as the
  sole signal** (rejected on its own — catches git-level changes but misses
  plain working-tree edits, the dominant case for a live session's diff
  view — which is why the chosen design pairs it with the cheap periodic
  stat-walk rather than relying on it alone).
- **Poll-based mtime scanning across every tracked file, every poll**
  (rejected as the *sole* signal at a short interval — this is exactly
  `HasUncommitted`'s existing O(files) cost, which is fine as the cheap
  *invalidation trigger* run periodically, but would defeat the purpose if
  it replaced the cache itself rather than gating it).

## Feasibility Risks

- ~~Cross-platform fsnotify recursive-watch semantics~~ — largely moot
  post-pivot: `.git` is a single, shallow directory, so cross-platform
  differences in *deep recursive* watch behavior no longer apply. The
  `.git`-only watch already runs today in `session/unfinished/watcher.go`
  across the same platforms with no reported issue.
- **New residual risk from the pivot**: the periodic cheap stat-walk's own
  cost, multiplied by worktree count and check frequency, is the new cost
  driver (replacing the fd-ceiling risk). At 134 worktrees × a 10-15s
  interval × an O(tracked-files) lstat walk each, plan phase must confirm
  this stays well under the ~13%+7.5% CPU baseline this whole feature is
  meant to reduce — i.e. don't accidentally reintroduce a smaller version of
  the same problem by checking too often or too many worktrees at once
  (stagger checks across worktrees rather than firing all 134 on the same
  tick, mirroring `unfinished.Scanner`'s own worker-pool/queue pattern
  rather than a single synchronized ticker).

## Observability Requirements

- Log (Warn, rate-limited like the existing "skipping scans under severe
  memory pressure" pattern in `unfinished/scanner.go`) when a worktree's
  watcher fails to start or is dropped due to an fd/directory-count ceiling,
  so a fleet-wide fd problem is visible instead of silently degrading to
  15s-TTL everywhere with no signal.
- Expose current watcher count (or "watcher active: bool" per session) via
  the existing debug-snapshot mechanism (`CreateDebugSnapshot` RPC) for
  troubleshooting, mirroring how other background subsystems' health is
  surfaced there.

## Risk Control

Feature flag `vcs:worktree-change-detection` (name TBD-confirm in planning,
registered in `knownFeatureFlags`), default **off**. When off for a
worktree that has not yet had `Setup()` run: today's behavior (15s TTL, no
watcher/periodic check) is completely unchanged, reversible via
`UpdateFeatureFlag` with no redeploy. This is **not** a live-retroactive
kill switch, though: flipping the flag off only prevents *new* worktree
setups from starting a detector — a detector already running for an
existing session keeps running (and that session's caches keep the widened
TTL) until the session is torn down. Restarting affected sessions is the
documented incident-response action to fully stop an already-running
detector, matching the accepted `terminal:resync-*` flag precedent for
"not live-retroactive" already established elsewhere in this codebase.

## Open Questions

- ~~Exact debounce window size~~ *(resolved by pitfalls.md/architecture.md
  research + the design pivot: no per-path debounce timer needed — the
  periodic stat-walk naturally coalesces bursts within its own interval,
  and the `.git` watch only needs a simple atomic dirty-flag, mirroring
  `unfinished.Scanner`'s existing `cacheDirty`/`inFlight` pattern.)*
- Exact widened TTL value, and exact periodic-stat-walk interval, when
  change-detection is active *(unresolved after Phase 2 research — plan
  phase must pick concrete numbers, e.g. TTL 5+ min / stat-walk every
  10-15s per worktree, staggered across worktrees per the new Feasibility
  Risks entry above, and justify the ratio between them: the stat-walk
  interval bounds real-world staleness, the TTL is now just a backstop in
  case the periodic check itself is ever paused/fails.)*
- ~~Final flag name and whether a `FeatureController` is needed~~
  *(resolved by pitfalls.md: no `FeatureController` needed — matches the
  existing `terminal:resync-*` flags' precedent of "not live-retroactive for
  already-open sessions," which is an accepted, documented pattern in this
  codebase. Exact flag name still TBD-confirm in planning.)*
