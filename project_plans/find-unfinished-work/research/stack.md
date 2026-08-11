# Research: Stack — Unfinished Work Tab

## Summary

- `git worktree list --porcelain` is already parsed in `session/git/worktree.go` and is the right primitive for enumerating worktrees; the codebase has a working parser for it.
- The codebase runs git CLI subprocesses exclusively (no pure-Go libgit2/go-git for status/diff); `git status --porcelain` and `git rev-list` are the right tools for unfinished detection, and the existing `TimeoutExecutor` (5s default) is the correct wrapper.
- fsnotify v1.9.0 is already in `go.mod` and is used in two places (`session/mux/autodiscover.go` and `session/history_watcher.go`); the recursive-watching pattern from `history_watcher.go` (walking subdirs on startup) is the template to follow.
- No new Go dependencies are needed; all required primitives (git CLI, fsnotify, go-git for repo detection, EventBus, ConnectRPC streaming) already exist.

## Findings

### git worktree list --porcelain — Format and Fields

The porcelain output (one attribute per line, records separated by blank lines) exposes:

| Field | Example | Notes |
|---|---|---|
| `worktree` | `worktree /home/user/repo` | Absolute path |
| `HEAD` | `HEAD abc1234` | Current commit SHA |
| `branch` | `branch refs/heads/feature-x` | Full ref; absent if detached |
| `bare` | `bare` | Boolean flag — no value |
| `detached` | `detached` | Boolean flag — no value |
| `locked` | `locked reason` | Optional reason string |
| `prunable` | `prunable gitdir file points to...` | Optional reason |

The codebase already has two complete parsers for this format: `parseWorktreeListForBranch` in `session/git/worktree.go` (package-level) and `findWorktreeForBranch` on `*GitWorktree` in `worktree_ops.go`. Both use the same split-on-blank-line / prefix-match pattern. A new utility function should reuse this approach to enumerate **all** worktrees for a repo, returning structs with `{Path, HEAD, Branch, IsBare, IsDetached}`.

### Minimal Git Commands for Unfinished Detection

The requirements specify three signals. Optimal commands per worktree:

```
# 1. Uncommitted changes (fast — reads index + disk, not network)
git -C <worktree> status --porcelain
# Non-empty stdout → has uncommitted changes

# 2. Commits ahead of main
git -C <worktree> rev-list main..HEAD --count
# N > 0 → ahead by N commits

# 3. Behind main
git -C <worktree> rev-list HEAD..main --count
# N > 0 → behind by N commits

# Combined ahead/behind in one call (if remote tracking branch exists):
git -C <worktree> rev-list --left-right --count HEAD...main
# Output: "A\tB" where A=ahead, B=behind
```

**Performance characteristics:**
- `git status --porcelain` on a small/medium repo: 10–100ms. On a large monorepo (400K+ files): up to 85s without FSMonitor, <1s with it. For normal repos: acceptable. Use 5s timeout (already the codebase default).
- `git rev-list --count`: reads pack files only, no working tree walk. Typically <50ms even on large repos.
- `git merge-base`: used by existing `initBaseCommitSHA()` to find main/master/develop/trunk. The same loop can identify the default branch name per repo.

**Existing caching pattern:** `IsDirtyWithHint` in `worktree_git.go` uses a 15s TTL read-write mutex cache. The Unfinished scanner should use the same pattern — per-worktree struct with `sync.RWMutex`, `lastScan time.Time`, `result ScanResult`, and TTL of ~30s.

### fsnotify for Watch Directories

`go.mod` has `github.com/fsnotify/fsnotify v1.9.0`. Already used in:
- `session/mux/autodiscover.go` — watches `/tmp/` for socket files; flat dir, no recursion needed.
- `session/history_watcher.go` — watches `~/.claude/projects/` recursively by calling `filepath.WalkDir` at startup to `watcher.Add()` each subdirectory, plus handles `Create` events for new subdirs inline.

**Key limitations on macOS (kqueue backend in fsnotify):**
- fsnotify is NOT natively recursive on macOS; it uses kqueue, which requires one file descriptor per watched path.
- The `history_watcher.go` pattern (walk + add each dir on start) works but has a race: new subdirectories created after start are not automatically watched.
- macOS internal limit: 4096 watched paths via FSEvents API.
- File descriptor exhaustion: watching 300+ directories can hit `ulimit`. The codebase's use in `history_watcher.go` is safe (watches only project subdirs under `~/.claude/projects/`).

**Recommendation for watch dirs:** Walk the user-configured root dirs at startup and add every `<dir>/.git` parent to the watcher. For detecting newly-added repos within a watch root, a periodic re-scan (every 60s) is more reliable than trying to watch recursive creation events. Limit depth to 5 levels to avoid fd exhaustion. Do NOT watch the entire tree for file changes — only watch at the repo `.git` level and use polling to trigger re-scans.

### AI Summary Generation

The codebase has a `config/claude.go` file that resolves the Claude CLI command (proxy-claude, claude, claude-code). The existing pattern for AI integration is CLI subprocess (`claude` command), not the Go Anthropic SDK.

**Options ranked by fit:**
1. **Claude CLI subprocess** (recommended): `claude -p "Summarize these git changes in 2-3 sentences" < <(git -C <path> diff HEAD)`. Already how the app invokes Claude. No new deps. Rate-limit by a simple per-repo mutex + cooldown (e.g., 5 min per worktree).
2. **Go Anthropic SDK**: Not in `go.mod`. Would add a new dependency. Overkill for this feature.
3. **Existing ConnectRPC Claude integration**: Not present in the backend; Claude is invoked via tmux sessions, not the API.

The summary should be generated lazily (on demand per item, not on every scan), cached in the dismiss/snooze state file with a TTL (24h), and rate-limited to avoid calling Claude repeatedly on unchanged worktrees.

### Proto Schema Patterns

The service definition in `proto/session/v1/session.proto` follows these conventions:
- Each RPC has a dedicated `<Verb><Noun>Request` / `<Verb><Noun>Response` pair.
- Streaming RPCs use: `rpc WatchX(WatchXRequest) returns (stream XEvent) {}`
- Unary RPCs for mutations: `rpc UpdateX(UpdateXRequest) returns (UpdateXResponse) {}`
- Types are in `types.proto`; events in `events.proto`; RPCs in `session.proto`.
- New feature types should go in `types.proto` as new message definitions.

**New RPCs needed:**
```proto
rpc ListUnfinishedWork(ListUnfinishedWorkRequest) returns (ListUnfinishedWorkResponse) {}
rpc WatchUnfinishedWork(WatchUnfinishedWorkRequest) returns (stream UnfinishedWorkEvent) {}
rpc DismissWorktree(DismissWorktreeRequest) returns (DismissWorktreeResponse) {}
rpc SnoozeWorktree(SnoozeWorktreeRequest) returns (SnoozeWorktreeResponse) {}
rpc ScanUnfinishedWork(ScanUnfinishedWorkRequest) returns (ScanUnfinishedWorkResponse) {}
rpc GetWorktreeAISummary(GetWorktreeAISummaryRequest) returns (GetWorktreeAISummaryResponse) {}
rpc UpdateUnfinishedWorkConfig(UpdateUnfinishedWorkConfigRequest) returns (UpdateUnfinishedWorkConfigResponse) {}
```

### Existing Code to Reuse

| What | Where | How to reuse |
|---|---|---|
| Porcelain parser | `session/git/worktree.go:parseWorktreeListForBranch` | Extract to shared util, add all-worktrees variant |
| IsDirty with TTL cache | `session/git/worktree_git.go:IsDirtyWithHint` | Pattern for per-worktree scan result cache |
| Git repo root detection | `session/git/util.go:findGitRepoRoot` | Exported variant for spider/watch-dir scanning |
| IsGitRepo | `session/git/util.go:IsGitRepo` | Already exported, use directly |
| merge-base resolution | `session/git/diff.go:resolveBaseCommitSHA` | Reuse loop over main/master/develop/trunk |
| TimeoutExecutor | `executor/timeout_executor.go` | Wrap all scan git commands |
| fsnotify watcher | `session/history_watcher.go` | Template for watch-dir recursive watching |
| mux AutoDiscovery | `session/mux/autodiscover.go` | Template for fsnotify + polling fallback pattern |
| EventBus | `server/events/bus.go` | Publish scan results → WatchUnfinishedWork stream |
| WatchSessions pattern | `server/services/session_service.go:WatchSessions` | Exact pattern for WatchUnfinishedWork streaming |

## Recommendations

1. Create `session/unfinished/` package (or `server/unfinished/`) to house the scanner, with a clean interface separate from `session/git/` to avoid coupling.
2. Add a shared `parseAllWorktrees(repoPath string) ([]WorktreeInfo, error)` function to `session/git/util.go` (or a new file).
3. Use `git status --porcelain` + `git rev-list --left-right --count HEAD...main` as the two-command scan per worktree; wrap both in `TimeoutExecutor` with 5s timeout.
4. Model the scanner caching on the `IsDirtyWithHint` pattern: per-worktree struct, 30s TTL, `sync.RWMutex`.
5. AI summary: implement as a CLI subprocess to `claude -p ...`, called on demand only, cached 24h, gated behind a `sync.Mutex` per worktree.

## Open Questions

- Should `session/unfinished/` be a new top-level package, or extend `session/git/`? (Architecture question for planning.)
- What is the maximum number of watch-dir repos to support before refusing to add more? (UX + fd limit interaction.)
- Should `git fetch --dry-run` be used to get accurate behind-main counts, or should we only check local tracking branch state (which may be stale)? (Requires network; risk of hanging.)
- Is the 5s per-subprocess timeout acceptable for watch-dir repos on network-mounted filesystems? (Pitfalls question.)
