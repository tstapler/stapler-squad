# Stack Research: diff-stats-file-watch-cache

## Existing codebase idiom (read first, match this style)

`go.mod:28` pins `github.com/fsnotify/fsnotify v1.9.0`. Two existing consumers set the pattern any
new watcher should follow — do not invent a different shape:

- `session/unfinished/watcher.go` (`WatchDirWatcher`): `fsnotify.NewWatcher()` in the constructor;
  on error, log a warning and leave `w.watcher = nil` — every method thereafter (`RemoveWatchDir`,
  `addRepo`, the event loop) nil-checks and no-ops rather than panicking. Watches are added
  per-`.git`-dir only (not recursive over the whole worktree), discovered via a depth-capped
  (`depth > 5`) manual `os.ReadDir` walk that skips `node_modules`, `vendor`, `.cache`, `dist`,
  `build`, `.git` by name (`walkDir`, lines 76-129). A `periodicReWalk` ticker (60s) re-walks to
  catch new repos fsnotify might have missed — the ticker-as-backstop pattern this feature should
  reuse for its own widened TTL.
- `session/unfinished/scanner.go`'s `Scanner`: same `NewWatcher`-or-nil-with-warning idiom
  (`Start`, lines 270-277), plus the coalescing primitive worth reusing directly: `inFlight
  sync.Map` (comment at lines 155-159) dedupes a burst of triggers (fsnotify + manual + periodic
  tick landing close together) into one scan per repo, and `triggerCh chan struct{}` is a
  size-1 buffered channel so a full-scan signal coalesces naturally (extra sends while one is
  pending are dropped, not queued). This is a simpler, cheaper coalescing pattern than a
  per-path debounce timer and is likely sufficient here since the requirement is "did *anything*
  change" (bool invalidation), not "which files changed."

Gitignore primitive already in use: `session/unfinished/gogit_vcs_reader.go` imports
`github.com/go-git/go-git/v5/plumbing/format/gitignore` (aliased `gitignore` in that file) and
calls `gitignore.ReadPatterns(osfs.New(worktreePath), nil)` to build a `gitignore.Matcher`
(`getOrBuildUntrackedMatcher`, line 964). This is the primitive the requirements doc points at
reusing — it's go-git's own gitignore package, not a separate third-party gitignore library, and
it's already a transitive dependency (go-git is imported throughout `session/unfinished/`).

`GitWorktreeManager.DiffStatsFresh()` (`session/git_worktree_manager.go:47`) and
`WorkspaceService.vcsStatusCache` (`server/services/workspace_service.go`, referenced at
`git_worktree_manager.go:38`) are the two TTL caches in scope — both currently pure time-based,
15s TTL, no invalidation signal.

## 1. Does fsnotify v1.9.0 support recursive watches natively?

No, on any platform. Confirmed from fsnotify's own docs (pkg.go.dev/github.com/fsnotify/fsnotify)
and README FAQ: "Recursive watching is not currently enabled through fsnotify's public API" — the
FAQ states plainly "you must add watches for any directory you want to watch (a recursive watcher
is on the roadmap: fsnotify/fsnotify#18)". Issue #18 has been open for years with no merged
resolution as of Sept 2026; two draft PRs (#751 "recursive fen backend", #752 "recursive kqueue",
both opened Apr 2026) and an older PR #676 ("recursive backend wrapping any non-recursive backend",
Mar 2025) are still unmerged/under discussion. So: every platform requires manual per-directory
`Add()` today, matching what `session/unfinished/watcher.go` already does — no library upgrade
unlocks recursion for free.

## 2. Current stable version / known issues relevant to recursive high-directory-count watching

Latest tagged release per `go list -m -versions github.com/fsnotify/fsnotify`: **v1.10.1**
(repo pinned at v1.9.0 is one minor behind, not the latest). No fd-exhaustion or event-coalescing
bug specific to v1.9.0 surfaced in search — the fd/watch-exhaustion behavior is inherent to the
underlying OS primitives (inotify watch-count limits on Linux, one fd-per-watched-entry on
kqueue/macOS — see §5), not a library defect that got fixed between 1.9.0 and 1.10.1. Worth
checking before implementation: fsnotify/fsnotify#721 ("Does fsnotify automatically fall back to
polling when 'too many open files' occurs?") — answer per the docs is **no**, fsnotify does not
auto-fallback; the caller must catch the `Add()` error itself, which matches this feature's own
constraint ("degrade gracefully if `Add()` fails — fall back to pure-TTL for that worktree only").
Recommend bumping to v1.10.1 as part of this work (routine minor-version bump, changelog is
bugfix/cross-platform-behavior-only per fsnotify's release notes) rather than treating 1.9.0 as
load-bearing.

## 3. Off-the-shelf recursive + gitignore-aware watcher library vs. build from primitives

No well-maintained single package combines recursive fsnotify watching *and* gitignore-aware
filtering. Options surveyed:

- **`farmergreg/rfsnotify`** (and forks `dietsche/rfsnotify`, `jotsen/rfsnotify`): thin wrapper
  adding `AddRecursive`/`RemoveRecursive` to fsnotify's API — recursion only, no gitignore
  awareness, and last meaningful activity is old/low-maintenance (fork proliferation is itself a
  signal of an unmaintained original).
  wraps `inotifywait`, not this project's Go fsnotify usage).

**Recommendation: build from primitives**, not adopt a wrapper library:
1. The recursive-walk-plus-Add() logic already exists in this codebase
   (`WatchDirWatcher.walkDir`, `session/unfinished/watcher.go:76-129`) — it needs generalizing
   (walk the whole worktree, not just to find `.git` dirs; skip via `gitignore.Matcher` instead of
   a hardcoded `skipDirs` map) rather than replacing with a new dependency.
2. The gitignore primitive (`go-git`'s `gitignore.Matcher`) is already a dependency and already
   used for exactly this kind of walk-filtering (`walkUntrackedRec`,
   `session/unfinished/gogit_vcs_reader.go:1821`).
3. Pulling in a third wrapper library (on top of fsnotify + go-git) to save ~100 lines of walk
   code adds a maintenance/audit surface (the constraint doc's fd-budget and gitignore-respecting
   requirements are project-specific enough that a generic wrapper wouldn't fit without
   patching anyway) for a genuinely small amount of saved code.

## 4. Idiomatic debounce/coalescing pattern

Two viable shapes, both idiomatic in real Go fsnotify consumers:

- **Size-1 buffered trigger channel + in-flight dedup set** — this repo's own existing pattern
  (`Scanner.triggerCh`, `Scanner.inFlight`, `session/unfinished/scanner.go:155-165`, `:223`).
  Simplest fit here: the cache-invalidation signal is a pure boolean ("worktree changed, drop the
  cache"), not per-file, so there's no need to key a debounce by path — a single per-worktree
  "pending invalidation" flag plus a short timer (or just an unconditional immediate invalidate,
  since invalidating a cache is idempotent and cheap, unlike a rebuild) is enough.
- **Per-path `time.AfterFunc` debouncer** (seen in multiple external write-ups, e.g. a
  mutex-guarded `pending`/`timer` struct that resets on every matching event) — appropriate for
  workloads that batch *many* distinct file changes before acting (e.g. a build tool). Overkill
  for this feature since the action on invalidation is "clear the cached value," which is safe to
  do many times in a row with no batching benefit — the *next* `GetSessionDiff`/`GetVCSStatus`
  call recomputes once regardless of how many invalidations preceded it.

**Recommendation:** an `atomic.Bool` "dirty" flag (or reuse `Scanner`'s literal `cacheDirty
atomic.Bool` pattern, `scanner.go:200`) set unconditionally on any fsnotify event for a worktree,
checked by `DiffStatsFresh()`/`vcsStatusCache`'s freshness check alongside the widened TTL. No
timer-based coalescing needed for invalidation itself — the burst-coalescing already happens
implicitly because setting an already-`true` flag is a no-op, and the *consumers* of the flag
(RPC calls) are naturally rate-limited by whatever polls them. This avoids introducing a new
per-worktree goroutine/timer purely for debounce, keeping the "one goroutine per worktree" fd/goroutine
budget constraint intact (the goroutine budget is spent on the fsnotify event-read loop itself, not a debounce timer).

## 5. Platform fd / watch-descriptor limits and a per-worktree directory cap

- **Linux (inotify)**: `fs.inotify.max_user_watches` sysctl — commonly defaulted to **8192** on
  many distros (per watchexec's own documented guidance), though some modern distros raise this
  default significantly higher; it is a per-*user* limit shared across every inotify-based tool
  running as that user (editors, other file watchers, this app's own `session/unfinished` watcher
  which already holds one watch per tracked repo's `.git` dir). Each watch costs ~1080 bytes of
  kernel memory on 64-bit. `fs.inotify.max_user_instances` separately caps the number of
  `fsnotify.NewWatcher()` instances per user — relevant because this feature's design (one watcher
  per live `Instance`/worktree) means concurrent-session count directly consumes this budget too,
  not just per-directory watch count.
- **macOS (kqueue)**: no inotify-style watch-count sysctl, but kqueue needs **one open file
  descriptor per watched entry** — worse than Linux because a watched directory *and* every file
  in it (if watched individually) each cost an fd. Per-process soft `ulimit -n` **defaults to 256**
  on stock macOS (raisable per-session via `ulimit -n`, but hard-capped by `kern.maxfilesperproc`,
  commonly 24576 system-wide default) — 256 is trivially exhausted by a single worktree-recursive
  watch on a repo this size, so any macOS deployment of this feature *must* raise the watcher's own
  effective fd budget or aggressively cap directories watched, not rely on the OS default.
- **This repo's own scale as worst case**: `git ls-files | wc -l` → **6,858 tracked files**;
  `git ls-files | xargs dirname | sort -u | wc -l` → **1,300 unique tracked directories**. Since
  fsnotify requires one `Add()` (one watch descriptor, and on macOS one fd) per directory, a fully
  recursive gitignore-respecting watch of this repo's worktree costs **~1,300 watch
  descriptors/fds** — comfortably under Linux's typical 8192 `max_user_watches` for a *single*
  worktree, but the constraint doc's "100+ concurrent sessions" scale multiplies this: 100 worktrees
  × 1,300 dirs = **130,000 watch descriptors**, which blows past both the common 8192 Linux default
  and any realistic macOS fd ceiling by two orders of magnitude.
- **Recommended cap**: given the above, a per-worktree **directory-count cap** (e.g. a few hundred
  to ~1,000 watched directories, well under this repo's own 1,300) with graceful fallback to
  pure-TTL for that worktree when the walk exceeds the cap (reusing the existing
  "degrade to TTL-only on `Add()` failure" pattern, just triggered proactively by a
  count check before starting to `Add()`, rather than reactively after failures start) is the
  safest way to keep 100+-concurrent-session fd/watch consumption bounded — this is a design
  decision for Agent 3 (plan), not something resolvable purely from stack research, since it
  trades off watch coverage (a big monorepo worktree might exceed the cap and silently degrade to
  pure-TTL) against fd safety.

## Version bump note

Recommend bumping `github.com/fsnotify/fsnotify` from v1.9.0 → **v1.10.1** (latest stable) as a
small preparatory change alongside this feature, since the pinned version is one minor behind and
no reason surfaced to stay pinned.
