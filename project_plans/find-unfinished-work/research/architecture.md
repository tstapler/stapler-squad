# Research: Architecture — Unfinished Work Tab

## Summary

- The existing background scanning model (ReviewQueuePoller, ExternalSessionDiscovery, daemon) is a **central poller with per-session goroutines** pattern — a fan-out worker pool is the right goroutine model for the new scanner.
- ConnectRPC streaming to the web UI is already implemented in `WatchSessions`: emit initial snapshot + subscribe to `EventBus`; the `WatchUnfinishedWork` RPC should use exactly this pattern.
- Dismiss/snooze state should live in a new JSON file in the config dir (same pattern as `discovery.json`, `pending_approvals.json`), NOT in the session database — it belongs to the scanner, not to sessions.
- The "auto-spider from sessions" source should hook into the `EventBus` `SessionCreated`/`SessionUpdated` events (already published on every session mutation) to trigger re-detection of repo roots when sessions change.

## Findings

### Existing Background Scanning Structure

**ReviewQueuePoller (`session/review_queue_poller.go`):**
- A single goroutine runs a `time.NewTicker(pollInterval)` loop.
- On each tick, it iterates over all in-memory instances and checks each one (terminal content, status, diff stats).
- Per-session results are cached in `cachedContent` / `lastSeenActivity` maps protected by `sync.Mutex`.
- This is a single-goroutine central poller with per-session cache — no fan-out.

**ExternalSessionDiscovery (`session/external_discovery.go`):**
- Single polling goroutine calling `discovery.StartPolling(ctx, interval)`.
- Delegates to `mux.Discovery.StartPolling` which runs one goroutine, calls `Scan()` periodically, and fires callbacks for changes.

**Daemon (`daemon/daemon.go`):**
- Single goroutine with a `time.NewTimer` tick; iterates all sessions sequentially.
- Uses `fsnotify` to watch the state file and reload instances when it changes.

**Pattern across all three:** Centralized coordinator goroutine with a flat list, no fan-out.

**HistoryFileWatcher (`session/history_watcher.go`):**
- Uses `fsnotify.NewWatcher()` + `watcher.Add()` per directory.
- Single goroutine processes events from `watcher.Events` channel.

### Recommended Goroutine Model for Unfinished Work Scanner

**Fan-out worker pool with bounded concurrency:**

```
UnfinishedWorkScanner (coordinator goroutine)
  ├── trigger: periodic ticker (30s default)
  ├── trigger: EventBus SessionCreated/SessionUpdated (spider source)
  ├── trigger: fsnotify events (watch-dir source)
  ├── trigger: manual scan request (ScanUnfinishedWork RPC)
  │
  ├── scanQueue chan repoScanTask  (buffered, capacity = max repos)
  │
  ├── worker pool (N=4 goroutines, configurable)
  │    ├── dequeue repoScanTask
  │    ├── run git commands via TimeoutExecutor
  │    ├── compare with cached result
  │    └── publish UnfinishedWorkUpdated event to EventBus if changed
  │
  └── resultCache  sync.Map[repoPath → ScanResult + timestamp]
```

**Why worker pool, not per-repo goroutines:**
- Bounded concurrency prevents git subprocess explosion (50 repos × 4 git calls = 200 concurrent subprocesses without a pool).
- `ReviewQueuePoller` has already shown that a single-goroutine sequential approach works for small N (<20 sessions) but becomes a bottleneck; a pool of 4 workers handles up to 4 repos concurrently, giving good throughput without overhead.
- Pool size of 4 is a good default: matches typical CPU core count for subprocess I/O, and git subprocess cost is dominated by I/O, not CPU.

**Deduplication:** Before enqueuing a scan task, check the cache. If last scan was <30s ago and no explicit invalidation occurred, skip enqueue. This prevents thundering-herd when multiple events arrive simultaneously.

### ConnectRPC Streaming Pattern

The `WatchSessions` implementation in `server/services/session_service.go` is the canonical template (lines 883–955):

```go
func (s *SessionService) WatchSessions(
    ctx context.Context,
    req *connect.Request[sessionv1.WatchSessionsRequest],
    stream *connect.ServerStream[sessionv1.SessionEvent],
) error {
    // 1. Send initial snapshot
    instances, _ := s.loadInstancesWithWiring()
    for _, inst := range instances {
        stream.Send(createInitialSnapshotEvent(inst))
    }
    // 2. Subscribe to EventBus
    eventCh, subID := s.eventBus.Subscribe(ctx)
    defer s.eventBus.Unsubscribe(subID)
    // 3. Loop until disconnect
    for {
        select {
        case <-ctx.Done(): return nil
        case event, ok := <-eventCh:
            stream.Send(convertEventToProto(event))
        }
    }
}
```

`WatchUnfinishedWork` should follow the same three-step pattern:
1. Send initial snapshot of all current unfinished worktrees (from scanner's in-memory state).
2. Subscribe to the shared `EventBus`; filter for `UnfinishedWorkUpdated` event type.
3. Forward converted events to the stream.

**EventBus** (`server/events/bus.go`) is already non-blocking (drops events to slow subscribers rather than blocking). Buffer size defaults to 100 per subscriber. This is sufficient for worktree scan events which arrive at most once per 30s per repo.

### Where Dismiss/Snooze State Should Live

**Options considered:**

| Option | Pro | Con |
|---|---|---|
| Session Ent DB (SQLite) | Transactional, queryable | Dismiss/snooze belongs to scanner, not sessions; adds schema migration complexity |
| `state.json` (existing) | No new file | state.json is being migrated to Ent; don't add to it |
| New JSON file in config dir | Simple, no schema migration; consistent with `discovery.json`, `pending_approvals.json` | Manual locking needed (use atomic write via temp-file rename) |
| In-memory only | Zero persistence overhead | Lost on restart — unacceptable for dismiss |

**Recommendation:** New JSON file `~/.stapler-squad/<workspace>/unfinished_state.json` with this shape:
```json
{
  "dismissed": [
    {"repo_path": "/abs/path/to/repo", "branch": "feature-x", "dismissed_at": "2026-04-25T..."}
  ],
  "snoozed": [
    {"repo_path": "/abs/path", "branch": "main", "snooze_type": "until_change", "snooze_since_sha": "abc123", "snoozed_at": "..."},
    {"repo_path": "/abs/path2", "branch": "fix-y", "snooze_type": "until_time", "until": "2026-04-26T08:00:00Z", "snoozed_at": "..."}
  ],
  "watch_dirs": ["/Users/tyler/code", "/Users/tyler/work"],
  "pinned_repos": ["/Users/tyler/special-project"],
  "ai_summary_cache": [
    {"repo_path": "...", "branch": "...", "diff_hash": "sha256:...", "summary": "...", "generated_at": "..."}
  ]
}
```

Write with atomic rename (same pattern as `saveConfig` in `config/config.go`). Read with a `sync.RWMutex`. Keep in memory after load; persist on every mutation.

**Key on dismiss/snooze:** Use `(repoPath, branchName)` as the composite key. If a worktree is deleted and re-created on the same branch, it should start fresh — store the HEAD SHA at snooze time and check if it changed to clear the snooze.

### Watch-Dir Recursive Scanning Structure

**Approach:**
```
WatchDirScanner
  ├── userWatchDirs: []string  (from unfinished_state.json)
  ├── pinnedRepos: []string    (from unfinished_state.json)
  │
  ├── On startup:
  │    ├── Walk each watchDir (depth ≤ 5)
  │    ├── Find .git directories → record repo roots
  │    ├── watcher.Add(repoRoot)   ← watch repo root for .git/index changes
  │    └── Schedule initial scan of all discovered repos
  │
  ├── fsnotify events:
  │    ├── On write to <repoRoot>/.git/index → enqueue scan for repoRoot
  │    └── On Create in watchDir (new subdir) → check if new git repo; add to watcher
  │
  └── Periodic re-walk (60s):
       └── Re-walk watchDirs to catch new repos created since startup
```

**What to watch:** Watch `<repoRoot>/.git/index` (or `<repoRoot>/.git/`) rather than the entire working tree. Index changes on every `git add`, `git commit`, `git checkout` — exactly the events that change unfinished status. This dramatically reduces fd count: one fd per repo, not per file.

**Depth limit:** Walk up to depth 5 from each watch dir. This handles typical monorepo structures (`~/code/org/repo`) without going too deep into `node_modules` or `vendor`.

**Gitignore-aware:** `filepath.WalkDir` with a check for `.git` directory stops descent into `.git/`. Additionally, skip dirs named `node_modules`, `vendor`, `.cache`, `dist`, `build` — these will never contain repos.

### Auto-Spider from Sessions

When a session is created or updated, its `Path` field contains the working directory of the session. The scanner should extract the git repo root from this path and add it to the scan set.

**Hook pattern:** In the `UnfinishedWorkScanner`, subscribe to the EventBus at startup. On `SessionCreated` or `SessionUpdated` events:
```go
case event:
    if event.Type == SessionCreated || event.Type == SessionUpdated {
        if repoRoot, err := findGitRepoRoot(event.Session.Path); err == nil {
            scanner.AddRepo(repoRoot, SourceSession)  // enqueue for scan
        }
    }
```

This means no separate polling for session paths — it's event-driven. The set of "session-derived repos" is rebuilt on each event. If a session is deleted, its repo may drop out of the set (if no watch dir covers it and no other session references it).

**Session path → git worktrees:** After finding the repo root, run `git worktree list --porcelain` to enumerate all worktrees of that repo. Each worktree becomes a scannable unit.

### Caching Strategy for Git Scan Results

**Two-level cache:**
1. **Per-worktree result cache** (in-memory, per `UnfinishedWorkScanner`):
   - Key: `worktreePath` (absolute)
   - Value: `{ScanResult, scanTime, srcSHA}` 
   - TTL: 30s
   - Invalidation: fsnotify event for `.git/index` → immediate re-scan
   - Pattern: same `sync.RWMutex` read-write lock as `IsDirtyWithHint`

2. **AI summary cache** (persisted in `unfinished_state.json`):
   - Key: `(repoPath, branch, diffHash)`
   - Value: summary string + generation time
   - TTL: 24h
   - Invalidation: diffHash changes (i.e., new commits or file changes since summary was generated)

**What NOT to cache:**
- The repo list itself should be rebuilt on each scan cycle from: session paths + watch dirs + pinned repos. This ensures new repos are picked up and deleted repos fall out.

### Service Wiring

New service `UnfinishedWorkService` follows the same sub-service extraction pattern as `GitHubService`, `WorkspaceService`, etc.:

```go
type UnfinishedWorkService struct {
    scanner    *unfinished.Scanner   // background scanner
    stateStore *unfinished.StateStore // dismiss/snooze/config persistence
    eventBus   *events.EventBus
}
```

Wire into `SessionService` during startup (or as a peer service registered in `server/server.go`). The service handles the ConnectRPC RPCs; the scanner runs independently as a background goroutine.

**Registration in `server.go`:** The new RPCs can be added to the existing `SessionService` (following the pattern of `workspaceSvc`, `githubSvc`) or as a separate service with its own ConnectRPC handler registration. Given the feature size, a separate service is cleaner.

## Recommendations

1. **Worker pool of 4 goroutines** with a `scanQueue` channel. Coordinator goroutine handles triggers (tick, fsnotify, EventBus).
2. **Watch `.git/index`** in each repo root (not the whole working tree) to minimize fd usage and get precise change signals.
3. **Periodic re-walk of watch dirs** (60s) to pick up new repos; fsnotify for immediate detection of git operations in known repos.
4. **Auto-spider** via EventBus subscription on SessionCreated/Updated events — event-driven, no polling.
5. **Dismiss/snooze state** in `unfinished_state.json` with atomic write; keep in memory with `sync.RWMutex` after load.
6. **WatchUnfinishedWork RPC** follows the exact `WatchSessions` three-step pattern (initial snapshot → EventBus subscribe → forward).

## Open Questions

- Should `UnfinishedWorkService` be added to `SessionService` (like `workspaceSvc`) or registered as a separate ConnectRPC handler? Separate is cleaner but requires proto service split.
- Should the scanner be started from `server.go` (like `reviewQueuePoller`) or from `main.go`? The former is consistent with existing patterns.
- How should the scanner handle the case where a user removes a watch dir from config while the scanner is running? (Need to clean up watcher subscriptions and prune repo list.)
- Should `unfinished_state.json` use the same workspace isolation as `sessions.db`? (Yes — it should be per-workspace, following `GetConfigDir()` resolution.)
