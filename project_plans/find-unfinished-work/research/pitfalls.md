# Research: Pitfalls — Unfinished Work Tab

## Summary

- Git command hangs are the primary risk: network-mounted filesystems (`git status` can block for minutes), large repos with FSMonitor disabled (up to 85s without cache), and `.git` lock files (block status indefinitely). The existing `TimeoutExecutor` (5s default) must wrap every scan subprocess.
- fsnotify on macOS hits file descriptor limits when watching many directories; watching repo roots only (`.git/index`) instead of entire trees limits exposure. 4096-path macOS internal limit applies.
- Git edge cases (bare repos, detached HEAD, no upstream remote, custom default branches) are all handled by failing gracefully with an empty/partial result rather than crashing the scanner.
- The "500 git status calls per minute" concern is manageable with a TTL cache (30s) and worker pool (4 goroutines), reducing peak concurrency to ~8 subprocess calls per minute even for 50 repos.

## Findings

### Git Command Hangs

**Root causes:**
1. **Network-mounted filesystems (NFS, SMB, AFP):** `git status` must stat every file in the working tree. On a networked filesystem, each stat is a network round-trip. A 10K-file repo can take minutes. There is no way to detect this upfront.
2. **Large repos without FSMonitor:** Research shows `git status` takes 17–85s on repos with 400K–2M files without `core.fsmonitor` enabled. For repos within a user's watch dir, this is an unknown.
3. **Locked `.git` index:** If another process holds a write lock on `.git/index.lock`, `git status` blocks until the lock is released. Common during long `git merge`, `git rebase`, or CI operations.
4. **Submodule traversal:** `git status` with submodules can multiply scan time by the number of submodules.

**Existing codebase handling:**
- `executor/timeout_executor.go` implements `TimeoutExecutor` with `context.WithTimeout` and `cmd.Process.Kill()`. The global default is **5 seconds** (in `config/config.go`).
- `IsDirtyWithHint` already uses this pattern through `g.runGitCommand` (which calls `g.cmdExec.CombinedOutput`).
- `session/mux/autodiscover.go` uses a 50ms sleep before probing a socket — the only explicit timing concern.

**Recommendations:**
- Use the existing `TimeoutExecutor` with **5s** for `git status --porcelain` (same as current default).
- Use **3s** for `git rev-list --count` (reads pack data only, should be fast).
- On timeout, log a warning with the repo path and mark the scan result as `{Status: Timeout, Message: "git command timed out"}`. Display this in the UI as a ⚠️ indicator rather than crashing the item.
- Add a **per-repo circuit breaker**: if a repo times out 3 consecutive scans, back off to 5-minute scan interval for that repo (exponential backoff). The existing `executor/circuit_breaker.go` can be adapted.
- Never retry a timed-out scan immediately — exponential backoff is essential.

### Permission Errors in Watch Dirs

**When a user configures `~/code` as a watch dir, subdirectories may include:**
- Repos they don't own (deployed code, CI artifacts owned by a different user).
- Repos inside Docker volumes or virtual filesystem paths.
- Repos with `chmod 000` or similar restrictive permissions.

**Handling:**
- `filepath.WalkDir` returns errors per path. Catch `*os.PathError` with `os.IsPermission(err)` → skip silently (log at debug level only, not warning, to avoid log spam).
- `git -C <path> status` will return exit code 128 with "fatal: not a git repository" or "Permission denied". Parse the error message; any non-zero exit → mark as `{Error: "permission denied"}`, skip the repo in subsequent scans for 10 minutes.
- Never surface a permission error to the user unless they explicitly added the path as a pinned repo (in which case, show a one-time warning in the UI).

### Git Worktree Edge Cases

| Edge case | Detection | Handling |
|---|---|---|
| **Bare repo** | `git worktree list --porcelain` output includes `bare` flag | Skip bare worktrees (no working tree to check uncommitted changes) |
| **Detached HEAD** | `branch` field absent in porcelain output; `detached` flag present | Skip or show with branch = "(detached HEAD)"; ahead/behind not applicable |
| **Prunable worktree** | `prunable` flag in porcelain output (worktree dir no longer exists) | Skip prunable worktrees; optionally offer to prune |
| **Locked worktree** | `locked` flag in porcelain output | Skip locked worktrees (another process owns them) |
| **Worktree from another user** | Path is outside `~` and has different owner | Access will fail at `git status` with permission error; handle as above |
| **Worktree with no tracking branch** | `git rev-list main..HEAD` returns fatal: ambiguous argument | Catch error; show only uncommitted status, skip ahead/behind |
| **No default branch** (no main/master/develop/trunk) | All merge-base attempts fail | Show `Has uncommitted changes` chip only; skip ahead/behind |
| **Worktree directory deleted** | `git worktree list --porcelain` shows `prunable` | Skip; also remove from in-memory cache |

**The existing `resolveBaseCommitSHA()` in `session/git/diff.go`** already handles the no-default-branch case by trying `main`, `master`, `develop`, `trunk` in order. The Unfinished scanner should use the same fallback sequence.

**No upstream remote:** Some local repos have no `origin` remote. `git rev-list main..HEAD` still works for local branch comparison. Ahead/behind vs. remote requires `git fetch` — which should **never** be called automatically (network risk). Use `@{upstream}` tracking state if available; otherwise report only ahead/behind vs. local `main`.

### Performance Limits at Scale

**Scenario:** 50 repos × 10 worktrees each = 500 worktrees total.

**Without optimization:**
- 500 × `git status --porcelain` = 500 subprocess calls
- At ~50ms each (normal repo, local filesystem) = 25 seconds per scan cycle
- At 5 second timeout each = 2500 seconds worst case (unacceptable)

**With 30s TTL cache + 4-worker pool:**
- Only scan worktrees whose cache is stale (>30s old)
- With 4 workers scanning in parallel: 500 worktrees / 4 workers × 50ms = ~6 seconds per full cycle
- In steady state (cache hits): scan only changed worktrees (those whose `.git/index` has a write event) → typically 0–5 worktrees per minute
- **Conclusion:** 500 worktrees is feasible with caching; without caching it is not.

**Additional optimization: separate `git rev-list` from `git status`:**
- Run `git status --porcelain` only if the working tree has changed (fsnotify event on `.git/index`).
- Run `git rev-list` only if `HEAD` SHA has changed (compare against cached HEAD SHA).
- This eliminates the vast majority of subprocess calls in steady state.

**Worker pool sizing:**
- 4 workers × 5s timeout = maximum 4 blocked goroutines at any time.
- Scan queue depth: 50 (buffer enough for all repos to queue simultaneously without blocking coordinator).

### AI Summary Rate Limiting and Caching

**Risks:**
- User clicks "Summarize" on 20 worktrees → 20 simultaneous Claude subprocess calls → system overload.
- Same diff gets re-summarized every session restart → wasteful API/CLI calls.
- Claude CLI hangs (API rate limit, network issue) → goroutine leak.

**Mitigations:**
1. **Global concurrency limit on AI summary calls:** Use a `semaphore` (buffered channel of size 2) to limit simultaneous Claude subprocess calls to 2.
2. **Per-worktree mutex:** Prevent double-summarizing the same worktree if user clicks Summarize twice.
3. **Cache by diff hash:** Hash the output of `git diff HEAD` (SHA256 of the diff content). If the hash matches the cached entry, return cached summary. TTL: 24h or until diff changes.
4. **Timeout on Claude subprocess:** 30s timeout (summaries should be fast; if not, something is wrong). Use `TimeoutExecutor`.
5. **Error UI:** If Claude CLI fails or times out, show an error message inline: "Summary unavailable — try again."

### Stale Dismiss/Snooze State After Worktree Deletion

**Scenario:** User dismisses worktree `/Users/tyler/.stapler-squad/worktrees/feat-x_1234`. Later, the session is deleted and the worktree directory is removed. Even later, a new session creates a new worktree for the same branch at a different path.

**Risks:**
- Dismiss record with old path is now stale.
- New worktree for same branch is also dismissed (unwanted).

**Key design decision:** Dismiss key should be `(repoPath, branchName)`, NOT `(worktreePath)`. This is more stable because:
- `repoPath` is the main repo root (stable across worktree create/delete).
- `branchName` identifies the work (stable across worktree path changes).

**Snooze cleanup:**
- Snooze key: `(repoPath, branchName, snooze_since_sha)` where `snooze_since_sha` is the HEAD SHA at snooze time.
- When the scanner evaluates a worktree: if the current HEAD SHA differs from `snooze_since_sha`, clear the snooze automatically.
- If the worktree directory no longer exists: prune the dismiss/snooze record from state (scan the state file on startup to remove entries for repos that no longer exist).

**State file cleanup:**
- On each startup, validate all dismiss/snooze entries: check if `repoPath` is a valid git repo. If not, remove the entry.
- This prevents unbounded growth of the state file over time.

### No `main` Branch Edge Cases

The existing `resolveBaseCommitSHA()` tries `main`, `master`, `develop`, `trunk` in order. This covers the common cases. Additional edge cases:

- **Repository using `production` or `release` as default:** Not covered by the fallback. Detection: `git symbolic-ref refs/remotes/origin/HEAD` returns `refs/remotes/origin/<default-branch>`. This is the most reliable method.
- **No commits at all:** New repo with no commits. `git rev-list` will fail. Detect: `git rev-parse HEAD` fails → show `Uncommitted (new repo)`.
- **Orphan branches:** Branch with no common ancestor with main. `git merge-base` returns nothing. Show only the `Has commits` chip; skip ahead/behind.

**Recommendation:** Add `git symbolic-ref refs/remotes/origin/HEAD --short` (strips `origin/` prefix) as the first attempt in the default branch resolution sequence, before falling back to main/master/develop/trunk.

### fsnotify Pitfalls on macOS

**Confirmed limitations (from fsnotify issue tracker):**
1. **No native recursion on macOS kqueue:** Must manually add each subdirectory. New subdirectories created after `Start()` are not automatically watched.
2. **File descriptor exhaustion:** Each watched path = one fd. Typical macOS `ulimit -n` is 256 (soft) or 10240 (hard). Watching 300 directories can hit the soft limit.
3. **macOS FSEvents internal limit:** 4096 watched paths. Beyond this, `watcher.Add()` returns an error.
4. **Stale events after sleep/wake:** macOS kqueue may deliver stale events after system sleep. The scanner should ignore events that don't correspond to actual file changes (compare timestamps).

**For the Unfinished Work scanner specifically:**
- Watch only repo roots (`.git/` directory), not the entire working tree: 50 repos = 50 fds. Well within limits.
- Use periodic re-walk (60s) to discover new repos inside watch dirs, rather than trying to watch the entire watch-dir tree.
- Implement `useFallback` mode (as in `mux/autodiscover.go`): if `fsnotify.NewWatcher()` fails, fall back to pure polling at 60s interval.

### How the Existing Codebase Handles Subprocess Errors and Timeouts

**Pattern in `session/git/worktree_git.go` (`runGitCommand`):**
```go
func (g *GitWorktree) runGitCommand(path string, args ...string) (string, error) {
    cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
    output, err := g.cmdExec.CombinedOutput(cmd)
    if err != nil {
        return "", fmt.Errorf("git command failed: %s (%w)", output, err)
    }
    return string(output), nil
}
```
- Wraps stderr in the error message — useful for logging.
- Caller must check the error string for specific git error messages (e.g., `strings.Contains(err.Error(), "not a git repository")`).
- `g.cmdExec` is a `TimeoutExecutor` with 5s timeout.

**Pattern in `session/git/diff.go` (`Diff`):**
- Checks for `"not a git repository"` in error string → return empty stats.
- Checks for `"unable to read"` (SHA not found) → clears `baseCommitSHA` for re-resolution.
- Never panics; always returns a partial/empty result on error.

**Pattern in `executor/circuit_breaker.go`:**
- Circuit breaker wraps an executor and tracks consecutive failures.
- Opens the circuit after N failures, preventing further calls for a configurable duration.
- The Unfinished Work scanner should use this pattern for repos that consistently timeout or fail.

**Recommendation for the new scanner:**
1. All git subprocesses: wrap in `TimeoutExecutor`.
2. Per-error classification:
   - `"timed out"` → mark repo as `{Status: Timeout}`, apply backoff.
   - `"not a git repository"` → remove from scan set (repo was deleted or unmounted).
   - `"Permission denied"` → mark as `{Status: PermissionError}`, skip for 10 minutes.
   - `"fatal: ambiguous argument"` → branch/ref doesn't exist; skip ahead/behind check, show only uncommitted status.
   - Other non-zero exit → log warning, mark as `{Status: Error, Message: trimmed stderr}`.
3. Never surface raw git error messages to the user — translate to human-readable messages.

## Recommendations

1. Wrap every git subprocess in `TimeoutExecutor`: 5s for `git status`, 3s for `git rev-list`.
2. Use per-repo exponential backoff after consecutive timeouts; adapt existing `circuit_breaker.go`.
3. Dismiss/snooze key = `(repoPath, branchName)` — not worktree path — to survive worktree re-creation.
4. Snooze-until-next-change: store HEAD SHA at snooze time; clear automatically when HEAD SHA changes.
5. AI summary: semaphore of 2, per-worktree mutex, 30s Claude subprocess timeout, cache by diff hash for 24h.
6. fsnotify: watch only `.git/` dirs (1 fd per repo); periodic re-walk (60s) for new repo discovery; `useFallback` polling mode when watcher unavailable.
7. State file cleanup on startup: validate all dismiss/snooze `repoPath` entries against filesystem.
8. Default branch detection: try `git symbolic-ref refs/remotes/origin/HEAD` first, then fall back to main/master/develop/trunk.

## Open Questions

- Should the scanner ever call `git fetch` to get accurate "behind remote" counts? (Network risk vs. accuracy.)
- What is the maximum number of watch-dir repos to support before capping? (100? 500? Needs a UX decision.)
- Should timed-out repos show a warning indicator in the UI, or silently retry in the background? (UX decision.)
- If a repo is mounted on an NFS filesystem, should the user be warned? (Detection: `statfs` call or `/proc/mounts` parsing — complex; may not be worth it for v1.)
- What happens if the user's `claude` CLI is not authenticated when an AI summary is requested? The existing `checkGHCLI()` pattern in `session/git/worktree_git.go` shows how to pre-check; a similar check should gate the AI summary button.
