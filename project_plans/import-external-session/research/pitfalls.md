# Pitfalls Research: import-external-session

## 1. General "adoption"/"import" failure patterns (industry precedent)

- **Docker container adoption / `docker-compose up --no-recreate`**: tools that "adopt" an
  already-running container by name/label frequently misidentify a *stale* container with the
  same name as the live one (name reuse after restart), leading to operations applied to the
  wrong PID. Docker's own daemon has had bugs where a reaped/zombie container ID was reused
  before the adoption logic re-validated liveness.
- **Kubernetes orphaned-resource adoption**: controllers that "adopt" Pods/PVs matching a label
  selector without an owner reference had a well-known bug class (fixed by adding
  `UID`-based ownership checks) — adopting a resource that looks like a match but belongs to a
  different, unrelated object, then deleting it on GC. The lesson: **match on stable identity
  (UID/inode), never on a mutable label like name or path alone.**
- **tmux/screen "attach" and hijack bugs**: attaching to a tmux session that another process is
  simultaneously killing/resizing produces detach-storms or corrupted screen state; multiple
  attachers to the same pane can race on control-mode socket writes. tmux control mode itself
  warns against concurrent writers to the same pane.
  Also relevant: tmux session name reuse after `kill-session` — a new session created moments
  later can reuse the same name/socket before a stale watcher unregisters, causing a "ghost"
  adoption.
- **IDE "attach to process" (VS Code, IntelliJ, gdb `attach`)**: classic failure mode is
  attaching to the wrong PID because PIDs are recycled quickly on Linux (default
  `pid_max` wraps in seconds under load) — debuggers that cache a PID from an earlier scan and
  attach later can attach to an unrelated, newly-spawned process. Best practice (adopted by
  gdb/lldb) is to re-verify process identity (start time, cmdline, exe path) immediately before
  attaching/killing, not just at discovery time.
- **General theme across all of these**: the failure is almost always a **TOCTOU gap between
  discovery and action** — the target is validated once, then acted on later, by which time the
  underlying resource has changed identity (recycled PID, restarted container, renamed session,
  reused socket).

## 2. Stack-specific risks

### Process killing cross-platform
- `session/native_process_manager.go:206` sends `syscall.Kill(-pid, syscall.SIGTERM)` — a
  **negative PID targets the whole process group**, not just the leaf process. This is correct
  for a directly-spawned PTY child, but if we reuse this pattern for an *imported* session whose
  process group also contains the user's shell, other panes, or an unrelated sibling process
  (e.g. the tmux server itself, or a `screen`/`nohup` wrapper), a naive `-pid` kill will
  terminate more than intended.
- **tmux pane vs. Claude process vs. tmux session are three different kill targets**, and this
  feature must not conflate them:
  - `tmux kill-session` (used in `hibernateProcess`, `instance_hibernate.go:75`, via SIGKILL) —
    destroys the whole session, all panes, the shell.
  - Killing only the Claude process PID inside the pane — leaves the tmux session/pane alive
    with a dead process, requiring a follow-up decision on what happens to the empty pane.
  - Killing the pane's shell (parent of Claude) — orphans Claude as a re-parented child (to PID 1
    on Linux / launchd on macOS), which may or may not receive SIGHUP depending on
    `remain-on-exit`/`set -o huponexit` settings.
  - macOS and Linux differ on default SIGHUP delivery to background process groups on shell
    exit (`nohup`-like default varies by shell), so "kill the shell and assume Claude dies too"
    is **not** a portable assumption — must be verified per-platform rather than inferred.
- **Zombie processes**: if we kill a supervised child without ever calling `Wait()` on it (or the
  reaper is a different process, e.g. tmux server, not us), the child becomes a zombie until its
  *actual* parent reaps it. For imported sessions we are not the parent (tmux server or the
  user's shell is), so our termination model cannot rely on `cmd.Wait()` — it must send a signal
  and separately poll for exit (as `IsAlive()` polling patterns in this codebase already do)
  rather than assuming a `Wait()`-based cleanup path will fire.
- **Signal semantics differ**: SIGKILL on macOS vs Linux is uniform, but process-group signal
  delivery combined with `setpgid` behavior at spawn time differs subtly; a process not started
  with its own process group (e.g. inherited group from tmux server) will forward `-pid` kills to
  siblings unexpectedly. Always verify `getpgid(pid)` reflects an isolated group before using
  group-kill; fall back to single-PID `SIGTERM`→wait→`SIGKILL` otherwise.

### fsnotify / process-lifecycle races
- `session/history_linker.go` already documents the exact race class this feature will
  reintroduce: correlating a *live* PID's open files (`GetPanePID` → `detector.Detect(pid)`) to a
  JSONL path, with a *path-based fallback* that is explicitly disabled for Paused/Hibernated
  sessions because "the most recently modified [JSONL] heuristic is wrong when other sessions
  have run in the same directory... their newer JSONL files would replace the correct stored UUID
  with a different session's conversation UUID" (`history_linker.go:258-263`). Import logic doing
  path-based JSONL correlation for a plain-tmux (no wrapper) session hits the identical ambiguity
  — multiple Claude sessions run in the same project directory produce multiple JSONL files, and
  "most recent" is not a reliable signal of "this is the one running in the pane the user pointed
  at."
- **fsnotify vs. process-exit race**: if the user is importing a session and, in the same window,
  the external process is producing more JSONL writes (append-only, but the file can also be
  mid-flush), a naive "read then import" can grab a **partially-written JSONL line** (JSON parse
  error) or miss the last few turns if the read happens between an `open()` and the final
  `write()+close()`. The importer must tolerate trailing partial/truncated final lines (treat
  read-truncated as "ignore, don't corrupt/drop") rather than fail the whole import.
- **Backoff/parking interaction**: `history_linker.go`'s backoff+park mechanism assumes it owns
  the correlation lifecycle for a session it created. An imported session bypasses that — the
  importer must not silently race the `HistoryLinker`'s own polling/fsnotify pass over the same
  instance, e.g. both writing `SetHistoryInfo` with different UUIDs concurrently right after
  import. Import should either register the instance with `HistoryLinker` and let it own
  subsequent correlation, or explicitly disable correlation post-import — not both fight over it.

### JSONL correctness during import
- Copying/porting conversation history while the source file may still be open for append by the
  external process risks a **short read** (see above) and also risks a **write conflict** if the
  workflow ever writes back to the same JSONL path (must not happen — treat source JSONL as
  read-only, always).
- If two processes end up appending to the *same* JSONL after import (e.g. original process not
  actually killed, and a new managed process also resumes the same `--resume <uuid>`), the file
  will interleave two writers' lines non-deterministically — corrupting conversation replay order.
  This is the single most important invariant to defend (see must-not-happen list below).

### git worktree assumptions
- Existing worktree code in this repo assumes it created the worktree and owns its lifecycle
  (see `session/git/worktree_git.go` `IsDirty` double-checked-locking pattern in
  `.claude/rules/go-double-checked-locking.md`). An imported session's git state was **not**
  created by stapler-squad: it may not be a worktree at all (plain clone, bare checkout, detached
  HEAD, or not a git repo), may have uncommitted changes the user is mid-way through, or may be a
  worktree already registered to a *different* tool. Import must never assume
  `git worktree list` entries it didn't create are safe to prune/move, and must never run
  destructive git operations (`git worktree remove`, `git clean`) against a path it imported
  without explicit confirmation that stapler-squad now fully owns it.

## 3. Must-not-happen scenarios (explicit design constraints)

1. **Must never kill a process without re-verifying identity immediately before the kill** —
   re-check PID start time / cmdline / exe path (or tmux pane's `#{pane_pid}` +
   `#{pane_start_time}`) at kill-time, not just at discovery time. Discovery-time PID + a delay
   (user confirmation dialog, batch queue) is exactly the TOCTOU gap described in §1.
2. **Must never issue a process-group-wide kill (`-pid` / `SIGTERM` to `-pgid`) unless the group
   has been verified to contain only the target process (and its direct children)** — never
   group-kill a tmux server's or user shell's process group.
3. **Must never leave two live processes (original external + newly-imported managed one) both
   attached to or writing the same conversation JSONL / same tmux session** — the import
   transaction must be: link history → verify import succeeded end-to-end → kill original, in
   that order, never kill-then-verify.
4. **Must never delete, prune, or mutate the original external process's tmux session/pane
   before the user's explicit confirmation** — no speculative/opportunistic kills during
   discovery or "preview" of an import candidate.
5. **Must never treat "most recently modified JSONL in this project directory" as authoritative
   when multiple JSONL files exist for the same directory** — this exact heuristic is already
   known-wrong in `history_linker.go` for the paused/hibernated case; the plain-tmux/no-wrapper
   import path must not reintroduce it as the *primary* signal. Require PID/open-file
   correlation (or explicit user disambiguation) whenever more than one JSONL candidate exists.
6. **Must never partially import a batch** (multi-session import) **and leave some originals
   killed and some not** without surfacing per-session success/failure to the user — a batch
   operation must report a per-item result, not an aggregate boolean, because "3 succeeded, 1
   failed" with the failed one's original process still alive vs. accidentally killed is a
   silent-data-loss vs safe-no-op distinction the user must see.
7. **Must never write to / truncate / resume-into the source JSONL file discovered via
   correlation** — treat it strictly read-only; all conversation porting must copy, never move
   or edit in place, until the original process is confirmed dead.
8. **Must never run destructive git operations against an imported working directory's worktree
   without confirming stapler-squad-created ownership** (see git worktree assumptions above).
9. **Must never silently reap a zombie by assuming a `Wait()` will fire** — since we are not the
   OS-level parent of an imported process, cleanup must poll for actual exit (`kill -0` /
   `/proc/<pid>` existence check on Linux, `kill(pid, 0)` on macOS) rather than relying on Go's
   `cmd.Wait()`, which only works for children we directly spawned.

## 4. Existing codebase patterns to reuse (not reinvent)

- **Process-group SIGTERM-before-teardown pattern** — `session/native_process_manager.go:186-215`
  (`Close()`): stop the supervise/restart loop first (via closing a `stopCh`) *before* sending
  the kill signal, so a concurrent auto-restart doesn't race the intentional shutdown. Import's
  "kill original process" step should follow the same ordering: disable any external-discovery
  auto-relink/re-adoption for that session *before* sending the kill signal.
- **`tmux kill-session` for full-session teardown** — `session/instance_hibernate.go:74-80`
  (`hibernateProcess`) is the reference for "we own this and want it fully gone": write a
  best-effort checkpoint first, log failure but continue, then kill, treating kill failure as
  logged-but-non-fatal. Import's confirmed-kill step should mirror this shape (checkpoint/import
  first, kill second, kill failure surfaced to user rather than silently swallowed since data
  loss risk is different — the user explicitly asked to remove the original).
- **Double-checked re-validation under lock after I/O** — `session/hibernation_sweeper.go`
  `sessionMemoryCache.GetOrFetch` (lines 64-84) and `HibernationSweeper.SystemMemoryPct`
  (144-173): release the lock before slow I/O, then **re-check state after reacquiring the lock**
  before committing the result, always returning the locally-computed value per
  `.claude/rules/go-double-checked-locking.md`. Any "confirm PID is still the same process, then
  act" logic in the importer must use this same re-check-after-I/O shape rather than trusting a
  stale read taken before the I/O.
- **Actor/message-passing serialization for state transitions** — `session/instance_hibernate.go`
  routes all status transitions through `sendSyncErr`/`transitionToLocked` (the Instance actor),
  never touching state directly from arbitrary goroutines. A new `Instance` created via import
  must go through the same state-machine entry points (Active/Hibernated/etc. transitions) rather
  than constructing/mutating fields directly, to avoid a second, ad hoc write path racing the
  actor's serialized one.
- **Idempotent linking (`SetHistoryInfo`) with logging only on actual change** —
  `session/history_linker.go:293-306`: setting history info is a no-op if UUID/path already
  match, and a log line only fires when the value actually changes. Import's "attach history to
  new Instance" step should reuse `SetHistoryInfo` (or the same idempotency discipline) rather
  than writing a parallel one-off field-set that could race the linker's own background pass.
- **Backoff + park for unlinkable sessions** (`history_linker.go` `historyLinkerBackoffThreshold`
  / `historyLinkerParkThreshold`) — reuse this mechanism (or explicitly register the imported
  instance with the same `HistoryLinker`) instead of building a separate polling loop for
  post-import re-correlation.
- **PID-scoped open-file inspection as the trustworthy signal, path-mtime as fallback-only**
  (`history_linker.go:242-273`, `history_detector.go` `Detect(pid)`): the codebase already
  encodes the lesson from §1 (prefer stable identity over name/path heuristics) — the importer
  for plain-tmux/no-wrapper sessions should use the same PID-based `Detect()` path as the primary
  signal and only fall back to path/mtime heuristics with explicit ambiguity handling (not silent
  "pick most recent").
