# Features Research: diff-stats-file-watch-cache

## 1. What `session/unfinished` already handles (and punts on)

`session/unfinished/watcher.go`'s `WatchDirWatcher` and `scanner.go`'s own
`fsnotifyLoop` are the closest analog — fsnotify primary + ticker backstop,
exactly this feature's stated model — but solve a *coarser* problem: they
watch only each repo's `.git/` directory (not the working tree), so they only
care about ref/index changes, never individual file edits.

Handled today:
- **Graceful fsnotify-unavailable fallback**: `NewWatchDirWatcher` swallows
  `fsnotify.NewWatcher()` errors, sets `watcher = nil`, logs a warning, and
  every caller (`Start`, `RemoveWatchDir`, `addRepo`) nil-checks before use —
  falls back to the ticker-only `periodicReWalk` (60s). This is the exact
  degrade-gracefully pattern the requirements ask for (`watcher.go:29-36`,
  `:49-53`, `:62-64`, `:135-137`).
- **Best-effort `Add`/`Remove`**: both are logged-and-ignored on error, never
  fatal (`watcher.go:66`, `:139-141`).
- **Directory-vs-file event resolution**: `fsnotifyLoop` walks up from the
  event path to find the nearest `.git` component rather than assuming the
  event name is exactly the watched dir (`watcher.go:156-180`, and
  `scanner.go:502` comment references mirroring this).
- **New-subdirectory detection under a watched root**: not done here (only
  `.git` itself is watched, not the tree) — but `session/history_watcher.go`
  (`watchNewDir`, line 139) and `session/detection/plugin_watcher.go` do
  handle "fsnotify doesn't watch subdirs created after the initial Add," via
  an explicit rewalk-and-add-new-dirs step. **This is the pattern the new
  per-worktree watcher must replicate for the whole tree**, not just one
  dir — `session/unfinished/watcher.go` never needed it because it only ever
  watches a single fixed dir (`.git`) per repo.
- **Debounce**: `session/unfinished/gogitstore/mmapwatch.go`'s
  `packWatchLoop` is the one existing example of debouncing a *burst* of
  fsnotify events into one action (comment at `mmapwatch.go:65-68` explicitly
  mirrors `scanner.go`'s "goroutine-exit" pattern) — worth reusing its timer
  shape rather than `session/services/claude_settings_watcher.go`'s (which
  debounces a single hot file, not directory bursts).
- **Punted / out of scope in the existing subsystem**: symlink handling,
  watch-target deleted-and-recreated, and `.gitignore` content changes are
  *never addressed* in `watcher.go`/`scanner.go` — because they only ever
  watch `.git/`, which git itself manages and whose churn patterns (locks,
  ref updates) don't include a `.gitignore` file changing shape or a symlink
  swap. **This new feature is the first to need those cases** since it
  watches the *working tree*, not `.git/`.

## 2. Git-specific edge cases for a working-tree watcher

- **Branch checkout / rebase (atomic multi-file churn)**: fsnotify will
  deliver one event per touched file — potentially thousands for a big
  checkout. This is exactly the "rapid burst" rabbit hole called out in
  requirements.md; the `mmapwatch.go` debounce-timer pattern (coalesce into
  one invalidation) is the right shape, not a per-event invalidate.
- **`git worktree remove` mid-session / directory deleted out from under a
  live watch**: fsnotify's Linux inotify backend reports `IN_IGNORED` (surfaced
  by `fsnotify` as the watch descriptor silently dropping — no explicit
  "watch removed" event type in this library's API) when a watched directory
  is deleted; `Watcher.Errors` does **not** reliably fire for this case. The
  existing subsystems never had to handle it because `.git/` outliving the
  process was a safe assumption for a background scanner; this feature's
  worktree can be torn down by `Cleanup()`/`Remove()` while the watcher is
  still running, so the lifecycle wiring (start/stop pairing with worktree
  setup/cleanup/destroy/pause from requirements.md's Scope) must explicitly
  call `watcher.Close()` on teardown rather than relying on fsnotify to
  notice the directory is gone — do not depend on the fs to self-report.
- **Directory replaced at the same path (same path, different inode)**: a
  `git worktree remove` + re-`add` at the identical path, or an external tool
  that recreates the dir, leaves fsnotify still holding a watch on the *old*
  inode on some platforms (kernel-level inotify semantics) — new writes to
  the new directory may not be observed. Needs an explicit re-`Add` on
  worktree-setup lifecycle events rather than assuming a long-lived watch
  survives a path's directory being swapped.
- **Symlinks inside a worktree**: fsnotify (inotify-backed) does **not**
  follow symlinks — it watches the link's target only if `Add` is called
  with the resolved path; watching the symlink path itself watches the link
  file, not directory contents through it. `session/git/worktree_dirty_fast.go`
  and `gogit_vcs_reader.go`'s untracked-file walks already have to reason
  about this (git itself treats a symlink as a blob, not a dir to recurse
  into) — the new watcher's recursive-watch-setup should mirror that: treat
  symlinks as leaf entries, never recurse through them, consistent with how
  git (and this repo's existing untracked-walk code) already treats them.
- **`.gitignore` changing** (new ignores, or previously-ignored files
  becoming tracked): this repo has a **direct, working precedent** —
  `gogit_vcs_reader.go:345-356` and its `gitignoreEntriesChanged` check
  (`gogit_vcs_reader.go:896-905`) rebuild the compiled gitignore matcher only
  when HEAD moves *and* a `.gitignore` blob hash actually changed between the
  two tree-hash maps — a cheap comparison against a full re-walk. The new
  watcher should reuse this exact idea: detect a `.gitignore` write via a
  plain fsnotify event (cheap), and only then pay for re-deriving the
  ignore-matcher / re-walking to add previously-skipped watches — never
  rewalk the whole tree on every unrelated file write.

## 3. Unstated needs / risks beyond "cache lives longer"

- **Trust erosion from visibly stale numbers**: `GetSessionDiff`/`GetVCSStatus`
  feed directly into the frontend's `useSessionVcs` (`fetchDiff`/`fetchStatus`)
  which users read as ground truth for "did my agent's edits actually land."
  A missed invalidation (e.g. the deleted-watch-directory case above, or an
  event dropped during a debounce window that's tuned too wide) would present
  as "the diff didn't update" right after a git operation — worse than the
  current 15s TTL's bounded staleness, because widening the backstop TTL
  (the whole point of this feature) removes the safety net that currently
  catches watcher failures within 15s. The debounce window and the backstop
  TTL are in tension: too generous a backstop makes a watcher bug invisible
  for the full extended TTL instead of resolving itself in 15s.
- **Diagnosability regression for unrelated bugs**: today, "the diff was
  wrong" always self-heals within 15s, so a genuine unrelated cache-bug (bad
  key, stale HEAD comparison) is easy to spot — it never lasts more than one
  TTL cycle. Once staleness is watcher-gated with a long backstop, "usually
  fresh" becomes the norm, so a *different* invalidation bug elsewhere could
  masquerade as "just the watcher being slow" and go unnoticed far longer.
  Worth logging (at Debug) every watcher-driven invalidation with enough
  context (path, event type) to distinguish "watcher fired, cache still
  wrong" from "watcher never fired" during triage — this repo already does
  this kind of diagnostic logging for the analogous `gitignoreMatcherInvalidations`
  atomic counter in `gogit_vcs_reader.go:903`.
- **fd/goroutine budget matches a real, already-solved constraint**: the
  100+ concurrent session target isn't new to this codebase — `gogitstore`'s
  `Registry` (shared object store, ref-counted) exists specifically to bound
  per-worktree resource use at that same scale. The new watcher should be
  designed with the same "one thing per live `Instance`, released on
  destroy" discipline, not a per-RPC-call watcher.

## 4. Existing comparable "invalidate cache on file change, per-instance" features

- **`session/unfinished/gogitstore/mmapwatch.go`** — closest structural
  analog for a *cache* invalidated by a *directory* fsnotify watch (its
  `objects/pack` dir), with graceful degrade documented explicitly:
  "fsnotify unavailable, mmap index staleness will only be caught by Registry
  TTL eviction" (`mmapwatch.go:52`) — precisely the fallback story requirements.md
  asks for GetSessionDiff/GetVCSStatus.
- **`session/history_watcher.go`** (`HistoryFileWatcher`) — per-directory
  fsnotify watch with explicit new-subdirectory detection (`watchNewDir`,
  handles `fsnotify.Create` on a dir to extend the watch set live) — the
  closest existing example of "recursive watch that grows as new dirs
  appear," which this feature's Scope explicitly calls out as needed.
- **`session/detection/plugin_watcher.go`** — fsnotify + periodic-rescan
  fallback pair with an explicit doc comment about the "editor overwritten
  file" caveat losing its per-file watch after the first save — a known
  fsnotify gotcha worth checking against for `.gitignore`'s own watch (an
  editor save can replace-then-rewrite a file, silently dropping the watch on
  some platforms).
- **`session/git/gitignore_fs_cache.go`** + **`worktree_dirty_fast.go`** —
  not fsnotify-based, but the existing *purely TTL-based* cache this feature
  is meant to obsolete for gitignore-pattern reads specifically; its doc
  comments (`gitignoreCacheTTL`, `IsDirtyCleanCacheTTL`) already discuss the
  exact same staleness/TTL tension enumerated in section 3, so their
  reasoning ("acceptable trade-off... matching this file's other TTL-based
  staleness tolerances") is a template for justifying the new watcher's own
  backstop TTL choice.
- **Feature flag precedent**: `config/config.go`'s `FeatureFlags map[string]bool`
  + `GetFeatureFlagWithDefault`, with named consts like `NativeWorktreeFeatureFlag`
  (default off, ADR-002-gated) and `TymuxFeatureFlag` (default off, "no
  rollback rehearsal has happened yet") are the exact pattern to follow for
  this feature's "ship gated behind a feature flag, default off" constraint —
  `TymuxFeatureFlag`'s comment is a good model for the rationale string.
