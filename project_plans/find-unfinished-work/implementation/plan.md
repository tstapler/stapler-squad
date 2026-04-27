# Implementation Plan: Unfinished Work Tab

Status: Draft | Phase: 3 - Planning complete
Created: 2026-04-25

---

## Architecture Decisions

### Goroutine Model

**Decision:** Fan-out worker pool with 4 goroutines, bounded `scanQueue` channel (capacity 50).

**Rationale:** The existing codebase uses single-goroutine central pollers (`ReviewQueuePoller`,
`ExternalSessionDiscovery`, `Daemon`) which work for small N but would create a sequential
bottleneck across 50+ repos × 4 git subprocesses each. A pool of 4 workers bounds concurrency to
at most 4 simultaneous `git` subprocesses, matching typical local I/O parallelism without spawning
hundreds of processes. The coordinator goroutine handles all trigger sources and enqueues scan
tasks; workers execute them.

**Deduplication:** Before enqueuing a task, check the in-memory TTL cache. If last scan <30s ago
and no explicit invalidation has occurred, drop the duplicate task (prevent thundering-herd when
multiple fsnotify events arrive simultaneously).

### State Persistence

**Decision:** New JSON file `~/.stapler-squad/<workspace>/unfinished_state.json`.

**Rationale:** Dismiss/snooze state belongs to the Unfinished Work scanner, not to the session
database. It follows the same pattern as `discovery.json` and `pending_approvals.json`. Written
with atomic rename (temp file + `os.Rename`) matching `saveConfig` in `config/config.go`. Kept
in memory under a `sync.RWMutex` after initial load.

**Dismiss/snooze key:** `(repoPath, branchName)` composite key — NOT `worktreePath`. This survives
worktree re-creation at a different path. Snooze-until-change stores the HEAD SHA at snooze time
and auto-clears when the HEAD SHA changes on next scan.

**State file shape:**
```json
{
  "dismissed": [
    {"repo_path": "/abs/path/to/repo", "branch": "feature-x", "dismissed_at": "2026-04-25T..."}
  ],
  "snoozed": [
    {"repo_path": "/abs/path", "branch": "fix-y", "snooze_type": "until_change",
     "snooze_since_sha": "abc123", "snoozed_at": "..."}
  ],
  "watch_dirs": ["/Users/tyler/code", "/Users/tyler/work"],
  "pinned_repos": ["/Users/tyler/special-project"],
  "ai_summary_cache": [
    {"repo_path": "...", "branch": "...", "diff_hash": "sha256:...", "summary": "...",
     "generated_at": "..."}
  ]
}
```

### ConnectRPC Pattern

**Decision:** New `UnfinishedWorkService` registered as a separate ConnectRPC handler alongside
`SessionService`. RPCs defined in new `unfinished.proto` (separate proto file, not extending
`session.proto`), with shared types in `types.proto`.

**Rationale:** The feature is large enough to warrant its own service rather than adding more
RPCs to the already 78K-line `session_service.go`. Follows the same separation as `GitHubService`.

**Streaming pattern:** `WatchUnfinishedWork` follows the exact `WatchSessions` three-step pattern:
1. Send initial snapshot from scanner's in-memory result cache.
2. Subscribe to shared `EventBus`, filter for `UnfinishedWorkUpdated` event type.
3. Forward converted events until client disconnects.

### AI Summary Approach

**Decision:** Lazy CLI subprocess: `claude -p "<prompt>" < <(git -C <path> diff HEAD)`. Called
on demand only (never auto-generated on scan). Cached by diff-hash (SHA256 of diff content) for
24 hours in `unfinished_state.json`. Rate-limited by a global semaphore of size 2.

**Rationale:** This matches the existing codebase pattern (`config/claude.go` resolves the Claude
CLI path). No new Go dependencies required. The 24h diff-hash cache prevents redundant API calls
when the user requests a summary on unchanged worktrees across page reloads.

### Source Discovery Hooks

**Three simultaneous source types:**

1. **Auto-spider (event-driven):** Subscribe to `EventBus` `SessionCreated`/`SessionUpdated`.
   On each event, call `findGitRepoRoot(event.Session.Path)` → enumerate worktrees → enqueue
   scan tasks. No polling needed — purely reactive.

2. **Watch dirs (fsnotify + periodic re-walk):** Walk user-configured root dirs at startup
   (depth ≤ 5), watch `<repoRoot>/.git/` with fsnotify (one fd per repo). On `.git/index` write
   events → enqueue repo scan. Periodic re-walk every 60s to pick up new repos created after
   startup. Fall back to polling-only if `fsnotify.NewWatcher()` fails.

3. **Pinned repos:** Treated identically to watch dirs after initial validation. Added/removed
   via settings UI; stored in `unfinished_state.json`.

---

## Proto Schema

New file: `proto/session/v1/unfinished.proto`

New types added to `proto/session/v1/types.proto` (UnfinishedWorktree, UnfinishedWorkConfig).

### New types in `types.proto`

```proto
// UnfinishedWorktree represents a single git worktree that has unfinished work.
message UnfinishedWorktree {
  // Composite key fields
  string repo_path   = 1;   // Absolute path to the repo root
  string branch      = 2;   // Branch name (e.g., "feature-auth")
  string worktree_path = 3; // Absolute path to the worktree directory

  // Display fields
  string repo_name   = 4;   // Derived from remote URL basename or dir basename
  string display_path = 5;  // Worktree path with ~ substitution

  // Status flags
  bool has_uncommitted = 6;     // git status --porcelain non-empty
  int32 commits_ahead  = 7;     // commits in HEAD not in default branch
  int32 commits_behind = 8;     // commits in default branch not in HEAD
  string default_branch = 9;    // resolved default branch (main/master/etc.)

  // Expanded detail fields (populated on demand or on expand)
  int32 changed_files  = 10;
  int32 lines_added    = 11;
  int32 lines_removed  = 12;
  repeated string ahead_commit_messages = 13; // Up to 5 short messages

  // Timestamps
  google.protobuf.Timestamp last_modified = 14; // mtime of worktree dir
  google.protobuf.Timestamp scan_time     = 15; // when this result was computed

  // Scan status (for error/timeout display in UI)
  ScanStatus scan_status  = 16;
  string scan_error_msg   = 17; // human-readable error, empty on success

  // Action state
  bool is_dismissed  = 18;
  bool is_snoozed    = 19;
  string session_id  = 20; // non-empty if an active session covers this branch
}

// ScanStatus indicates the result quality of the last scan.
enum ScanStatus {
  SCAN_STATUS_UNSPECIFIED = 0;
  SCAN_STATUS_OK          = 1;
  SCAN_STATUS_TIMEOUT     = 2;
  SCAN_STATUS_PERMISSION  = 3;
  SCAN_STATUS_ERROR       = 4;
}

// UnfinishedWorkConfig holds user-configurable source settings.
message UnfinishedWorkConfig {
  bool   auto_spider_sessions = 1;  // default: true
  repeated string watch_dirs  = 2;
  repeated string pinned_repos = 3;
}
```

### New `proto/session/v1/unfinished.proto`

```proto
syntax = "proto3";
package session.v1;
import "google/protobuf/timestamp.proto";
import "session/v1/types.proto";

// UnfinishedWorkService manages detection, display, and dismissal of
// unfinished git worktrees across all configured sources.
service UnfinishedWorkService {
  // ListUnfinishedWork returns the current snapshot of all unfinished worktrees.
  rpc ListUnfinishedWork(ListUnfinishedWorkRequest) returns (ListUnfinishedWorkResponse) {}

  // WatchUnfinishedWork streams real-time updates as worktrees are scanned.
  // Sends initial snapshot then emits events on each scan result change.
  rpc WatchUnfinishedWork(WatchUnfinishedWorkRequest) returns (stream UnfinishedWorkEvent) {}

  // ScanUnfinishedWork triggers an immediate scan of all sources.
  // Used by the manual Refresh button in the UI.
  rpc ScanUnfinishedWork(ScanUnfinishedWorkRequest) returns (ScanUnfinishedWorkResponse) {}

  // DismissWorktree permanently hides a worktree from the Unfinished list.
  rpc DismissWorktree(DismissWorktreeRequest) returns (DismissWorktreeResponse) {}

  // SnoozeWorktree hides a worktree until its HEAD SHA changes.
  rpc SnoozeWorktree(SnoozeWorktreeRequest) returns (SnoozeWorktreeResponse) {}

  // GetWorktreeAISummary generates (or returns cached) an AI summary for a worktree.
  rpc GetWorktreeAISummary(GetWorktreeAISummaryRequest) returns (GetWorktreeAISummaryResponse) {}

  // GetUnfinishedWorkConfig retrieves current source configuration.
  rpc GetUnfinishedWorkConfig(GetUnfinishedWorkConfigRequest) returns (GetUnfinishedWorkConfigResponse) {}

  // UpdateUnfinishedWorkConfig adds/removes watch dirs and pinned repos.
  rpc UpdateUnfinishedWorkConfig(UpdateUnfinishedWorkConfigRequest) returns (UpdateUnfinishedWorkConfigResponse) {}
}

// --- Request / Response pairs ---

message ListUnfinishedWorkRequest {}
message ListUnfinishedWorkResponse {
  repeated UnfinishedWorktree worktrees = 1;
  google.protobuf.Timestamp   last_scan  = 2;
}

message WatchUnfinishedWorkRequest {}
message UnfinishedWorkEvent {
  oneof payload {
    UnfinishedWorktree  worktree_updated = 1;
    UnfinishedWorktree  worktree_removed = 2;
    ScanCompleted       scan_completed   = 3;
  }
}
message ScanCompleted { google.protobuf.Timestamp completed_at = 1; }

message ScanUnfinishedWorkRequest {}
message ScanUnfinishedWorkResponse { google.protobuf.Timestamp scan_started_at = 1; }

message DismissWorktreeRequest  { string repo_path = 1; string branch = 2; }
message DismissWorktreeResponse {}

message SnoozeWorktreeRequest   { string repo_path = 1; string branch = 2; }
message SnoozeWorktreeResponse  {}

message GetWorktreeAISummaryRequest  { string repo_path = 1; string branch = 2; }
message GetWorktreeAISummaryResponse {
  string summary    = 1;
  bool   from_cache = 2;
}

message GetUnfinishedWorkConfigRequest {}
message GetUnfinishedWorkConfigResponse { UnfinishedWorkConfig config = 1; }

message UpdateUnfinishedWorkConfigRequest {
  UnfinishedWorkConfig config = 1;
}
message UpdateUnfinishedWorkConfigResponse { UnfinishedWorkConfig config = 1; }
```

---

## Epic Breakdown

### Epic 1: Git Scanning Engine

Core git subprocess layer: enumerate worktrees, detect unfinished status, parse results.

#### Story 1.1: `parseAllWorktrees` utility function
Extract a shared function that parses `git worktree list --porcelain` output into a slice of
`WorktreeInfo` structs (all worktrees for a repo, not just a specific branch). This is the
foundation all other stories depend on.

- [ ] **Task 1.1.1:** Add `WorktreeInfo` struct to `session/git/util.go` with fields:
  `Path string`, `HEAD string`, `Branch string`, `IsBare bool`, `IsDetached bool`,
  `IsPrunable bool`, `IsLocked bool`.
  AC: struct compiles; all fields covered by porcelain format.

- [ ] **Task 1.1.2:** Implement `ParseAllWorktrees(repoPath string) ([]WorktreeInfo, error)` in
  `session/git/util.go`. Runs `git worktree list --porcelain` via a new bare `exec.Command`
  (no executor needed for this utility). Skips bare and prunable entries. Returns the full list.
  AC: unit test with fixture output covering all edge cases (bare, detached, locked, normal).

- [ ] **Task 1.1.3:** Write `session/git/util_worktrees_test.go` with table-driven tests for
  `ParseAllWorktrees` covering: normal worktree, bare worktree, detached HEAD, locked worktree,
  prunable worktree, empty output (no worktrees).
  AC: all cases pass; no subprocess calls in tests (use fixture strings).

#### Story 1.2: Unfinished status detection per worktree
Per-worktree scan that produces `ScanResult` (uncommitted, ahead count, behind count, diff stats).

- [ ] **Task 1.2.1:** Create `session/unfinished/` package. Define `ScanResult` struct:
  `{RepoPath, Branch, WorktreePath, HasUncommitted bool, AheadCount int, BehindCount int,
  ChangedFiles int, LinesAdded int, LinesRemoved int, AheadMessages []string,
  LastModified time.Time, ScanTime time.Time, Status ScanResultStatus, ErrorMsg string}`.
  AC: compiles; exported from package.

- [ ] **Task 1.2.2:** Implement `scanWorktree(wt WorktreeInfo, defaultBranch string,
  exec executor.Executor) ScanResult` in `session/unfinished/scanner.go`.
  Runs: `git -C <path> status --porcelain` (5s timeout), `git -C <path> rev-list
  --left-right --count HEAD...<defaultBranch>` (3s timeout). Populates `ScanResult`.
  Skip bare/detached worktrees (return zero ScanResult with status OK).
  AC: unit test with mocked executor; all timeout/error cases handled; no panics on any error.

- [ ] **Task 1.2.3:** Implement default branch resolution in `session/unfinished/scanner.go`:
  try `git symbolic-ref refs/remotes/origin/HEAD --short` first, then fall back to
  main/master/develop/trunk via `git merge-base`. Cache per-repo.
  AC: resolves correctly for repos with origin/HEAD; falls back gracefully; returns empty
  string (not error) when no default branch found.

- [ ] **Task 1.2.4:** Implement diff stats fetch: `git -C <path> diff --shortstat HEAD`
  (or `git diff --numstat HEAD` for per-file count). Parse into `ChangedFiles`, `LinesAdded`,
  `LinesRemoved`. Also fetch up to 5 ahead commit messages via `git log <defaultBranch>..HEAD
  --oneline --max-count=5`.
  AC: returns zero stats on empty diff; handles "not a git repository" error gracefully.

- [ ] **Task 1.2.5:** Implement `os.Stat(worktreePath).ModTime()` for `LastModified` — no git
  subprocess needed. Add mtime sort helper `SortByLastModified([]ScanResult)`.
  AC: sort is descending (most recent first); handles equal mtimes deterministically.

#### Story 1.3: Per-worktree result cache
30-second TTL in-memory cache matching the `IsDirtyWithHint` pattern.

- [ ] **Task 1.3.1:** Define `worktreeCache` struct in `session/unfinished/cache.go`:
  `{mu sync.RWMutex, result ScanResult, scanTime time.Time, ttl time.Duration}`.
  Implement `Get() (ScanResult, bool)` (returns false if stale) and `Set(ScanResult)`.
  AC: unit test proves TTL expiry and double-checked-lock pattern is correct.

- [ ] **Task 1.3.2:** Implement per-repo circuit breaker: wrap per-repo scan attempts; after 3
  consecutive `ScanResultStatus == Timeout`, set backoff to 5 minutes for that repo.
  Reuse or adapt `executor.CircuitBreaker`.
  AC: after 3 timeouts, `ShouldScan()` returns false for 5 minutes; resets on success.

---

### Epic 2: Source Management

Background scanner coordinator, watch dir discovery, auto-spider from sessions, fsnotify watcher.

#### Story 2.1: Scanner coordinator goroutine and worker pool
The central goroutine that dispatches scan tasks and manages the worker pool.

- [ ] **Task 2.1.1:** Implement `Scanner` struct in `session/unfinished/scanner.go`:
  `{scanQueue chan scanTask, resultStore sync.Map[string]ScanResult, eventBus *events.EventBus,
  stateStore *StateStore}`. `scanTask = {repoPath string, source SourceType}`.
  AC: `New(eventBus, stateStore) *Scanner` compiles.

- [ ] **Task 2.1.2:** Implement `Scanner.Start(ctx context.Context)` — starts coordinator
  goroutine (30s ticker + signal channels) and 4 worker goroutines that consume `scanQueue`.
  Each worker calls `scanRepo(repoPath)` which enumerates worktrees via `ParseAllWorktrees`
  then calls `scanWorktree` per entry.
  AC: goroutines exit cleanly when context is cancelled; no goroutine leaks (verified in test).

- [ ] **Task 2.1.3:** Implement `Scanner.EnqueueRepo(repoPath string)` — checks TTL cache,
  deduplicates, sends to `scanQueue` without blocking (drops if full, logs warning).
  AC: calling EnqueueRepo twice within 30s only results in one scan task; channel never blocks.

- [ ] **Task 2.1.4:** Implement `Scanner.publishResults(results []ScanResult)` — for each
  changed result (compare with stored), publish `UnfinishedWorkUpdated` event to EventBus.
  Remove dismissed/snoozed items from published results before emitting.
  AC: only changed results fire events; dismissed items are excluded.

#### Story 2.2: fsnotify watch dir watcher
Recursive walk + fsnotify for immediate change detection.

- [ ] **Task 2.2.1:** Implement `WatchDirWatcher` in `session/unfinished/watcher.go`:
  `{watcher *fsnotify.Watcher, scanner *Scanner, stateStore *StateStore}`.
  On `Start(ctx)`: walk each watch dir (depth ≤ 5), for each found repo root call
  `watcher.Add(repoRoot+"/.git")`, then call `scanner.EnqueueRepo(repoRoot)` for initial scan.
  AC: compiles; walk skips `node_modules`, `vendor`, `.cache`, `dist`, `build` dirs.

- [ ] **Task 2.2.2:** Implement fsnotify event loop: on `Write` or `Create` events under a
  watched `.git/` path → `scanner.EnqueueRepo(repoRoot)`. On `Create` events in a watch dir
  root → check if new subdirectory is a git repo, add to watcher if so.
  AC: no fd leaks; watcher.Close() called on ctx cancellation.

- [ ] **Task 2.2.3:** Implement periodic re-walk (60s ticker): re-walk watch dirs to discover
  new repos created since startup. Add newly found repos to the watcher.
  AC: repos added after scanner start are discovered within 60s.

- [ ] **Task 2.2.4:** Implement `useFallback` mode: if `fsnotify.NewWatcher()` returns error,
  fall back to pure 60s polling (no fsnotify). Mirror the pattern in `mux/autodiscover.go`.
  AC: scanner functions correctly with no fsnotify available (tested by passing a nil watcher).

#### Story 2.3: Auto-spider from sessions
Event-driven repo discovery from active session paths.

- [ ] **Task 2.3.1:** In `Scanner.Start()`, subscribe to EventBus for `SessionCreated` and
  `SessionUpdated` events. On each event, extract `session.Path`, call `findGitRepoRoot`,
  and `EnqueueRepo`. Track the set of session-derived repos in a `sync.Map`.
  AC: creating a new session triggers a scan of its repo within one event loop iteration.

- [ ] **Task 2.3.2:** On `SessionDeleted` event, remove the repo from the session-derived set
  if no other session references it and it is not in a watch dir or pinned repos list.
  AC: deleted session's repo is removed from scan set when it has no other source coverage.

#### Story 2.4: Watch dir and pinned repo configuration management
Add/remove watch dirs and pinned repos at runtime (called from settings UI via RPC).

- [ ] **Task 2.4.1:** Implement `Scanner.AddWatchDir(path string) error` and
  `Scanner.RemoveWatchDir(path string)`: update `stateStore`, persist, walk new dir and add
  to watcher (add case) or remove from watcher and drop from repo set (remove case).
  AC: add triggers immediate scan of new dir; remove prunes repos not covered by other sources.

- [ ] **Task 2.4.2:** Implement `Scanner.AddPinnedRepo(path string) error` and
  `Scanner.RemovePinnedRepo(path string)`: validate that path is a git repo, update stateStore,
  enqueue for scan.
  AC: invalid (non-git) path returns error; valid path is scanned within one tick.

---

### Epic 3: State Persistence

Dismiss, snooze, AI summary cache, and config persistence.

#### Story 3.1: StateStore — JSON persistence layer

- [ ] **Task 3.1.1:** Implement `StateStore` struct in `session/unfinished/state.go`:
  `{mu sync.RWMutex, path string, state UnfinishedState}`.
  `UnfinishedState` mirrors the JSON shape from the Architecture Decision section.
  Implement `Load() error` and `save() error` (atomic write via temp file + `os.Rename`).
  AC: round-trip test: write → read → compare; atomic write survives a simulated crash
  mid-write (temp file present, rename not completed).

- [ ] **Task 3.1.2:** Implement dismiss operations: `Dismiss(repoPath, branch string) error`,
  `IsDismissed(repoPath, branch string) bool`, `UndismissAll() error` (for future use, add now).
  AC: dismissed entry persists across StateStore reload; IsDismissed returns true after Dismiss.

- [ ] **Task 3.1.3:** Implement snooze operations: `Snooze(repoPath, branch, headSHA string) error`,
  `IsSnoozed(repoPath, branch, currentSHA string) bool` (auto-clears when SHA differs),
  `UnsnoozeStaleSnoozedItems()` (called on startup to clean stale entries).
  AC: snooze with SHA "abc" → IsSnoozed returns true; same call with SHA "def" returns false
  (auto-cleared); state is cleaned up in storage.

- [ ] **Task 3.1.4:** Implement startup cleanup: validate all dismissed/snoozed `repoPath`
  entries on load — if the path is not a valid git repo, remove the entry.
  AC: startup with stale entries (paths removed from filesystem) produces clean state file.

#### Story 3.2: AI summary cache

- [ ] **Task 3.2.1:** Implement `GetCachedSummary(repoPath, branch, diffHash string) (string, bool)`
  and `CacheSummary(repoPath, branch, diffHash, summary string) error` in `StateStore`.
  Cache TTL: 24h from `generated_at`. Evict expired entries on load.
  AC: cache hit returns (summary, true); expired entry returns (_, false); eviction on load.

- [ ] **Task 3.2.2:** Implement `ComputeDiffHash(worktreePath string, exec executor.Executor) (string, error)`:
  run `git -C <path> diff HEAD`, SHA256-hash the output bytes.
  AC: empty diff → consistent hash; same diff → same hash; different diff → different hash.

---

### Epic 4: ConnectRPC Service (Backend)

`UnfinishedWorkService` implementing all 7 RPCs, wired into `server.go`.

#### Story 4.1: Service struct and ListUnfinishedWork

- [ ] **Task 4.1.1:** Create `server/services/unfinished_work_service.go`. Implement
  `UnfinishedWorkService` struct: `{scanner *unfinished.Scanner, stateStore *unfinished.StateStore,
  eventBus *events.EventBus}`. `New(scanner, stateStore, eventBus) *UnfinishedWorkService`.
  AC: compiles; no circular imports.

- [ ] **Task 4.1.2:** Implement `ListUnfinishedWork`: read current scan results from
  `scanner.GetAllResults()`, filter out dismissed/snoozed items, convert to proto, return sorted
  by repo then by `last_modified` descending.
  AC: returns empty list when no repos scanned; dismissed items excluded; correct sort order.

#### Story 4.2: WatchUnfinishedWork streaming RPC

- [ ] **Task 4.2.1:** Implement `WatchUnfinishedWork` following the exact `WatchSessions` pattern:
  1. Send initial snapshot (all current results from `scanner.GetAllResults()`).
  2. Subscribe to EventBus, filter for `UnfinishedWorkUpdated` event type.
  3. Loop: forward events to stream until ctx.Done().
  AC: client receives initial snapshot immediately; subsequent scan results arrive as events;
  client disconnect does not leak goroutine.

#### Story 4.3: Mutation RPCs (Dismiss, Snooze, Scan)

- [ ] **Task 4.3.1:** Implement `DismissWorktree(req)`: call `stateStore.Dismiss(repoPath, branch)`,
  emit `UnfinishedWorkRemoved` event so watching clients remove the item.
  AC: item disappears from subsequent `ListUnfinishedWork` calls; event fires on stream.

- [ ] **Task 4.3.2:** Implement `SnoozeWorktree(req)`: look up current HEAD SHA for the worktree
  from scanner results, call `stateStore.Snooze(repoPath, branch, headSHA)`, emit
  `UnfinishedWorkRemoved` event.
  AC: snoozed item hidden; reappears after a new scan detects different HEAD SHA.

- [ ] **Task 4.3.3:** Implement `ScanUnfinishedWork(req)`: call `scanner.TriggerScan()` (signals
  the coordinator to run a full scan immediately), return `scan_started_at`.
  AC: manual scan starts within 100ms of RPC call; emits `ScanCompleted` event on stream when done.

#### Story 4.4: AI Summary and Config RPCs

- [ ] **Task 4.4.1:** Implement `GetWorktreeAISummary(req)`:
  1. Compute diff hash via `ComputeDiffHash`.
  2. Check `stateStore.GetCachedSummary` → return if found.
  3. Acquire global semaphore (size 2) and per-worktree mutex.
  4. Run `claude -p "Summarize these git changes in 2-4 sentences." < <(git diff HEAD)` via
     `exec.CommandContext` with 30s timeout, using the resolved Claude CLI path from `config/claude.go`.
  5. Cache result, release locks, return.
  AC: concurrent requests for same worktree only run one subprocess; semaphore limits to 2 global
  concurrent calls; timeout returns user-friendly error message.

- [ ] **Task 4.4.2:** Implement `GetUnfinishedWorkConfig` and `UpdateUnfinishedWorkConfig`:
  read/write `stateStore` config fields, call `scanner.AddWatchDir`/`RemoveWatchDir` as needed.
  AC: adding a watch dir persists and triggers immediate scan; removing cleans up watcher.

#### Story 4.5: Server registration

- [ ] **Task 4.5.1:** Add `UnfinishedWorkService` to `server/runtime_deps.go` (or equivalent
  deps struct). Instantiate `Scanner` and `StateStore` alongside existing background services.
  Start `scanner.Start(ctx)` in `server.go` following the same pattern as `reviewQueuePoller`.
  AC: server starts without error; scanner starts background goroutines.

- [ ] **Task 4.5.2:** Register `UnfinishedWorkServiceHandler` in `server/server.go`:
  generate handler from new proto with `unfinishedv1connect.NewUnfinishedWorkServiceHandler(...)`,
  register at `/api/unfinished/v1/`.
  AC: `curl /api/unfinished/v1/...` returns ConnectRPC response; `make ci` passes.

---

### Epic 5: Proto Schema

Define all new proto types, generate Go and TypeScript code.

#### Story 5.1: Proto definitions

- [ ] **Task 5.1.1:** Add `UnfinishedWorktree`, `ScanStatus`, `UnfinishedWorkConfig` messages
  to `proto/session/v1/types.proto` (exact fields listed in Proto Schema section above).
  AC: `make generate-proto` succeeds; no compile errors in generated Go code.

- [ ] **Task 5.1.2:** Create `proto/session/v1/unfinished.proto` with `UnfinishedWorkService`
  definition and all request/response pairs (exact definitions listed in Proto Schema section).
  AC: `make generate-proto` produces `gen/proto/go/session/v1/unfinished*.go` and
  `gen/proto/go/session/v1/unfinishedv1connect/*.go`; TypeScript client types generated.

- [ ] **Task 5.1.3:** Verify generated TypeScript types appear in `web-app/src/gen/` and are
  importable. Run `cd web-app && npx tsc --noEmit` to confirm no TypeScript errors from new types.
  AC: TypeScript compilation succeeds; no `any` escape hatches needed for new types.

---

### Epic 6: Frontend — Unfinished Tab + Item List

New Next.js page, navigation entry, repo-grouped list rendering.

#### Story 6.1: Route and navigation

- [ ] **Task 6.1.1:** Add `unfinished: "/unfinished"` to `web-app/src/lib/routes.ts`.
  Create `web-app/src/app/unfinished/page.tsx` (thin shell that renders `<UnfinishedTab />`).
  AC: navigating to `/unfinished` renders without 404; page title is "Unfinished Work".

- [ ] **Task 6.1.2:** Add "Unfinished" nav link to `Header.tsx` between Sessions and Review Queue.
  Add `UnfinishedNavBadge` component (shows count of unfinished items as a number badge,
  updates reactively). Badge is hidden when count is 0.
  AC: badge shows correct count; updates when scan results change; hidden at zero.

- [ ] **Task 6.1.3:** Create `web-app/src/components/layout/BottomNav` entry for Unfinished
  tab (mobile nav). Follow the existing BottomNav pattern.
  AC: mobile nav shows Unfinished link with badge.

#### Story 6.2: ConnectRPC client hook and data layer

- [ ] **Task 6.2.1:** Create `web-app/src/lib/hooks/useUnfinishedWork.ts`. Subscribe to
  `WatchUnfinishedWork` streaming RPC (mirror the `useWatchSessions` hook pattern).
  Maintain local state: `Map<string, UnfinishedWorktree>` keyed by `${repoPath}/${branch}`.
  Expose: `worktrees`, `lastScanTime`, `isScanning`, `triggerScan()`.
  AC: hook reconnects on disconnect; `worktrees` updates reactively on each stream event.

- [ ] **Task 6.2.2:** Create `web-app/src/lib/hooks/useUnfinishedWorkConfig.ts`. Fetches
  `GetUnfinishedWorkConfig` on mount; exposes `config`, `updateConfig(patch)`.
  AC: config loads on first render; `updateConfig` call persists to backend.

#### Story 6.3: UnfinishedTab and repo-grouped list

- [ ] **Task 6.3.1:** Create `web-app/src/app/unfinished/UnfinishedTab.tsx` and
  `UnfinishedTab.css.ts`. Renders: tab header (last scan time, Refresh button, + Watch Dir button),
  filter chips (All / Uncommitted / Ahead / Behind), repo-grouped item list.
  AC: renders with mock data; filter chips toggle correctly; Refresh button calls `triggerScan()`.

- [ ] **Task 6.3.2:** Create `UnfinishedRepoGroup.tsx`: repo name as section header with item
  count, collapsed by default, expand/collapse on click. Shows child `UnfinishedItem` components.
  AC: groups expand/collapse; item count badge updates; keyboard-accessible (Enter/Space to toggle).

- [ ] **Task 6.3.3:** Create `UnfinishedItem.tsx`: item card showing branch name, abbreviated
  worktree path, status chips (Uncommitted, ↑N ahead, ↓N behind), hover-reveal action buttons
  (dismiss ×, snooze 🕐). `ScanStatus == Timeout` shows ⚠️ instead of chips.
  AC: chips only appear for relevant signals (no "Ahead 0" shown); abbreviated path uses `~`;
  hover actions are accessible (focus-visible state).

- [ ] **Task 6.3.4:** Create `UnfinishedItem.css.ts` using vanilla-extract. Use tokens from
  `vars` for all colors, spacing, and typography. No hardcoded hex values.
  AC: `make ci` passes `lint:css`; dark mode renders correctly.

---

### Epic 7: Frontend — Expanded Item (Accordion)

Inline accordion expansion showing diff stats, commit messages, and action buttons.

#### Story 7.1: Accordion expansion

- [ ] **Task 7.1.1:** Implement accordion toggle in `UnfinishedItem.tsx`: clicking the item
  card (not action buttons) toggles `isExpanded` local state. Expanded items show
  `UnfinishedItemDetail` component below.
  AC: only one item per repo group can be expanded at a time (controlled by parent); keyboard
  navigation — Enter/Space expands; Escape collapses.

- [ ] **Task 7.1.2:** Create `UnfinishedItemDetail.tsx`: shows diff stats row (N files changed,
  +X −Y lines), list of up to 5 ahead commit messages, and action button row.
  AC: renders correctly with zero ahead commits; zero diff stats show "No uncommitted changes".

- [ ] **Task 7.1.3:** Fetch expanded detail on accordion open (if `changed_files` is 0 in
  current scan result, call `ListUnfinishedWork` with `include_detail: true` flag, or rely on
  initial snapshot having full data). Decision: include diff stats in every scan result to avoid
  a second RPC call on expand.
  AC: no loading spinner required on expand (data already present in scan result).

---

### Epic 8: Frontend — Actions

All action buttons in the expanded item: Open Session, Commit & Push, View Files, Dismiss,
Snooze, AI Summary.

#### Story 8.1: Open Session

- [ ] **Task 8.1.1:** Implement `[Open Session]` button handler: if `session_id` is non-empty,
  navigate to existing session via `routes.sessionDetail(sessionId)`. If empty, call
  `CreateSession` RPC with `session_type: EXISTING_WORKTREE`, `existing_worktree: worktreePath`,
  then navigate to the new session.
  AC: existing session → navigates without creating duplicate; no session → creates and navigates.

#### Story 8.2: Commit & Push shortcut

- [ ] **Task 8.2.1:** Create `CommitPushModal.tsx`: modal with commit message textarea (required),
  [Cancel] and [Commit & Push] buttons. Calls a new `QuickCommitPush` RPC (or reuses
  `PushChanges` via session service). Shows progress indicator during operation.
  AC: empty message prevents submit; success closes modal; error shows inline error message.

- [ ] **Task 8.2.2:** Implement backend `QuickCommitPush` RPC in `UnfinishedWorkService`:
  stage all changes (`git add .`), commit, push (`git push -u origin <branch>`). Uses
  `TimeoutExecutor` with 60s timeout for push. Returns success or error message.
  AC: RPC runs all three git operations; times out gracefully; returns human-readable error.

- [ ] **Task 8.2.3:** Add `QuickCommitPush` RPC to `unfinished.proto`. Fields: `repo_path`,
  `branch`, `commit_message`. Returns `{success bool, error_message string}`.
  AC: proto compiles; generated code correct.

#### Story 8.3: View Files

- [ ] **Task 8.3.1:** Implement `[View Files]` button: opens existing file browser component
  (already present in codebase) for the worktree path. Identify the existing `FileService`
  route/component and pass the `worktreePath` as the root.
  AC: clicking View Files opens file browser at the worktree root; no new file browser needed.

#### Story 8.4: Dismiss and Snooze

- [ ] **Task 8.4.1:** Implement dismiss action: call `DismissWorktree` RPC, update local state
  optimistically (remove item from list immediately), show brief toast "Dismissed. Undo?" with
  5s undo window (calls `UndismissWorktree` if clicked — add this RPC).
  AC: item disappears immediately on click; undo within 5s restores item.

- [ ] **Task 8.4.2:** Implement snooze action: call `SnoozeWorktree` RPC, remove item from list
  optimistically. No undo for snooze (item returns automatically on next git change).
  AC: item disappears immediately; item reappears on next scan if HEAD SHA changed.

- [ ] **Task 8.4.3:** Add `UndismissWorktree` RPC to proto and service: removes the dismiss
  record from `StateStore`.
  AC: calling UndismissWorktree makes the worktree appear in the next `ListUnfinishedWork`.

#### Story 8.5: AI Summary

- [ ] **Task 8.5.1:** Implement `[Summarize]` button in `UnfinishedItemDetail.tsx`. On click:
  show spinner, call `GetWorktreeAISummary` RPC, display 2-4 sentence result inline below
  action buttons. Show "Summary unavailable — try again." on error.
  AC: summary appears inline (no modal); loading state shown during generation; error handled.

- [ ] **Task 8.5.2:** Cache AI summary in React state keyed by `${repoPath}/${branch}` so
  re-expanding the accordion doesn't re-fetch on the same page session.
  AC: second expand shows cached summary immediately without API call.

---

### Epic 9: Frontend — Settings Integration

Watch dir and pinned repo management in the Settings page.

#### Story 9.1: Unfinished Work Sources section in Settings

- [ ] **Task 9.1.1:** Create `web-app/src/app/settings/unfinished/page.tsx` and
  `UnfinishedSourcesSettings.tsx` component. Shows: auto-spider toggle, watch dirs list with
  remove buttons, Add Dir input, pinned repos list with remove buttons, Add Repo input.
  AC: component renders; all three source types configurable; changes persist to backend.

- [ ] **Task 9.1.2:** Add "Unfinished Sources" link to the Settings page sidebar/navigation.
  Follow the existing Settings layout pattern (`settings/layout.tsx`).
  AC: link appears in settings nav; page is reachable at `/settings/unfinished`.

- [ ] **Task 9.1.3:** Implement directory picker: use an `<input type="text">` with path
  completion (reuse existing `PathCompletionService` RPC) for watch dir and pinned repo inputs.
  AC: path completion suggests valid directories; invalid paths show validation error.

- [ ] **Task 9.1.4:** Add "+ Watch Dir" button in the Unfinished tab header (shortcut to settings).
  On click, navigate to `/settings/unfinished` with focus on the Add Dir input.
  AC: button click navigates correctly; settings page is accessible from the tab header.

---

### Epic 10: Integration & Polish

Wiring it together, keyboard navigation, badge count, background refresh indicator.

#### Story 10.1: Keyboard navigation

- [ ] **Task 10.1.1:** Implement `j`/`k` (or arrow keys) navigation between items in
  `UnfinishedTab.tsx`. Maintain `focusedItemKey` state. Auto-scroll focused item into view.
  AC: pressing `j` moves focus to next item; `k` to previous; focus wraps at list boundaries.

- [ ] **Task 10.1.2:** Implement keyboard shortcuts on focused item: `Enter`/`Space` to
  expand/collapse, `o` to open session, `s` to snooze, `d` to dismiss, `r` to refresh.
  AC: each shortcut fires the correct action; shortcuts are documented in keyboard shortcuts help.

- [ ] **Task 10.1.3:** Add keyboard shortcut documentation for Unfinished tab shortcuts to
  the existing help panel (accessible via `?` key).
  AC: `?` panel lists all 7 unfinished tab shortcuts.

#### Story 10.2: Filter chips

- [ ] **Task 10.2.1:** Implement filter chips: `All / Uncommitted / Ahead / Behind`. Filter
  is applied client-side (no new RPC needed — filter the `worktrees` array from the hook).
  AC: selecting "Uncommitted" shows only items with `has_uncommitted: true`; "All" shows all.

#### Story 10.3: Background refresh indicator

- [ ] **Task 10.3.1:** Show "Last scanned N seconds ago" in the tab header, updating every
  second via `setInterval`. Show a spinning indicator during active scan (`isScanning` state).
  AC: counter updates every second; spinner visible during scan; resets to 0 on scan complete.

#### Story 10.4: End-to-end integration test

- [ ] **Task 10.4.1:** Write a Go integration test in `server/services/unfinished_work_service_test.go`:
  Create a temp git repo, make a commit, create a worktree with an uncommitted file. Start
  scanner with the repo as a pinned repo. Call `ListUnfinishedWork` and assert the worktree
  appears with `has_uncommitted: true`.
  AC: test passes; no file system residue after test; uses isolated temp dir.

- [ ] **Task 10.4.2:** Write a Go integration test for dismiss: call `DismissWorktree`, then
  `ListUnfinishedWork` — assert dismissed item is absent. Restart the StateStore, reload from
  disk — assert dismiss persists.
  AC: dismiss survives StateStore reload; undismiss restores item.

- [ ] **Task 10.4.3:** Write a Go integration test for snooze: snooze with SHA "abc", assert
  item is hidden. Simulate new scan with SHA "def", assert item reappears.
  AC: snooze clears automatically on SHA change; item reappears without user action.

---

## Data Flow Diagram

```
Source Detection
──────────────────────────────────────────────────────────────────
EventBus (SessionCreated/Updated)
        │  (event-driven)
        ▼
  findGitRepoRoot(session.Path)
        │
        └──────────────────────┐
                               │
fsnotify (.git/ write events)  │  periodic 30s tick
        │                      │       │
        └──────────────────────┤       │
                               ▼       ▼
                     UnfinishedWorkScanner
                      ┌──── coordinator goroutine ────┐
                      │  scanQueue chan scanTask (50)  │
                      └───────────────────────────────┘
                               │
                      ┌────────┴────────┐
                      ▼                 ▼
               worker pool (4 goroutines)
                      │
                      ▼
              scanRepo(repoPath)
                      │
              ParseAllWorktrees()  → skip bare/detached/prunable
                      │
              [per worktree: check TTL cache]
                      │ (cache miss)
                      ▼
         ┌──────────────────────────────┐
         │  git -C <path>               │  (TimeoutExecutor 5s)
         │    status --porcelain        │
         │  git -C <path>               │  (TimeoutExecutor 3s)
         │    rev-list --left-right     │
         │    --count HEAD...<default>  │
         │  git -C <path>               │  (TimeoutExecutor 3s)
         │    diff --shortstat HEAD     │
         └──────────────────────────────┘
                      │
              ScanResult → worktreeCache.Set()
                      │
              [filter dismissed/snoozed via StateStore]
                      │
              EventBus.Publish(UnfinishedWorkUpdated)
                      │
ConnectRPC Transport
──────────────────────────────────────────────────────────────────
                      │
         WatchUnfinishedWork (server-streaming RPC)
              │  (subscriber goroutine per connected client)
              ▼
         UnfinishedWorkEvent → HTTP/2 stream → browser
                      │
React UI
──────────────────────────────────────────────────────────────────
                      │
         useUnfinishedWork() hook
              │  (maintains Map<key, UnfinishedWorktree>)
              ▼
         UnfinishedTab
              │
         UnfinishedRepoGroup (one per distinct repo_name)
              │
         UnfinishedItem  ←── filter chips applied here
              │
         [Expand] UnfinishedItemDetail
              │
         ┌────┴─────────────────────────────┐
         │  [View Files]  [Open Session]    │
         │  [Commit & Push] [Summarize]     │
         │  [Snooze] [Dismiss]              │
         └──────────────────────────────────┘
                      │
         User Action → ConnectRPC mutation RPC
              │         (DismissWorktree / SnoozeWorktree /
              │          QuickCommitPush / GetWorktreeAISummary)
              ▼
         Optimistic UI update → scanner re-scan (triggered by
                                 git change or explicit scan RPC)
```

---

## Files to Create or Modify

### Create (new files)

| Path | What |
|---|---|
| `proto/session/v1/unfinished.proto` | New service + all request/response message types |
| `session/unfinished/scanner.go` | `Scanner` struct, `ScanResult`, `scanWorktree`, worker pool, coordinator |
| `session/unfinished/cache.go` | `worktreeCache` TTL cache, circuit breaker integration |
| `session/unfinished/watcher.go` | `WatchDirWatcher`, fsnotify event loop, periodic re-walk |
| `session/unfinished/state.go` | `StateStore`, `UnfinishedState`, JSON persistence, dismiss/snooze ops |
| `session/unfinished/state_test.go` | Unit tests for StateStore (round-trip, dismiss, snooze, cleanup) |
| `session/unfinished/scanner_test.go` | Unit tests for scanWorktree, cache, circuit breaker |
| `server/services/unfinished_work_service.go` | `UnfinishedWorkService` implementing all 7 RPCs |
| `server/services/unfinished_work_service_test.go` | Integration tests (temp git repos) |
| `web-app/src/app/unfinished/page.tsx` | Route shell |
| `web-app/src/app/unfinished/UnfinishedTab.tsx` | Main tab component |
| `web-app/src/app/unfinished/UnfinishedTab.css.ts` | vanilla-extract styles |
| `web-app/src/components/unfinished/UnfinishedRepoGroup.tsx` | Repo section header + item list |
| `web-app/src/components/unfinished/UnfinishedRepoGroup.css.ts` | vanilla-extract styles |
| `web-app/src/components/unfinished/UnfinishedItem.tsx` | Item card with status chips |
| `web-app/src/components/unfinished/UnfinishedItem.css.ts` | vanilla-extract styles |
| `web-app/src/components/unfinished/UnfinishedItemDetail.tsx` | Accordion detail panel |
| `web-app/src/components/unfinished/UnfinishedItemDetail.css.ts` | vanilla-extract styles |
| `web-app/src/components/unfinished/CommitPushModal.tsx` | Commit & Push modal |
| `web-app/src/components/unfinished/CommitPushModal.css.ts` | vanilla-extract styles |
| `web-app/src/components/unfinished/UnfinishedNavBadge.tsx` | Nav count badge |
| `web-app/src/components/unfinished/UnfinishedNavBadge.css.ts` | vanilla-extract styles |
| `web-app/src/lib/hooks/useUnfinishedWork.ts` | WatchUnfinishedWork streaming hook |
| `web-app/src/lib/hooks/useUnfinishedWorkConfig.ts` | Config fetch/update hook |
| `web-app/src/app/settings/unfinished/page.tsx` | Settings page for sources |
| `web-app/src/components/settings/UnfinishedSourcesSettings.tsx` | Settings component |
| `web-app/src/components/settings/UnfinishedSourcesSettings.css.ts` | vanilla-extract styles |

### Modify (existing files)

| Path | What changes |
|---|---|
| `proto/session/v1/types.proto` | Add `UnfinishedWorktree`, `ScanStatus`, `UnfinishedWorkConfig` messages |
| `session/git/util.go` | Add `WorktreeInfo` struct and `ParseAllWorktrees()` function |
| `server/server.go` | Register `UnfinishedWorkServiceHandler`; start `scanner.Start(ctx)` |
| `web-app/src/lib/routes.ts` | Add `unfinished: "/unfinished"` and `settingsUnfinished: "/settings/unfinished"` |
| `web-app/src/components/layout/Header.tsx` | Add Unfinished nav link and `UnfinishedNavBadge` |
| `web-app/src/components/layout/BottomNav.tsx` | Add Unfinished entry for mobile nav |
| `web-app/src/app/settings/layout.tsx` | Add Unfinished Sources link to settings nav |

### Generated (do not edit manually)

| Path | Produced by |
|---|---|
| `gen/proto/go/session/v1/unfinished*.go` | `make generate-proto` |
| `gen/proto/go/session/v1/unfinishedv1connect/` | `make generate-proto` |
| `web-app/src/gen/` (TypeScript client) | `make generate-proto` |

---

## Risk Register

### Risk 1: git command hangs on network-mounted filesystems
**Likelihood:** Medium (many developer machines use NFS/SMB for project dirs).
**Impact:** High — scanner goroutines block; backlog builds; UI shows stale data.
**Mitigation:**
- `TimeoutExecutor` with 5s for `git status`, 3s for `git rev-list` (already codebase default).
- Per-repo circuit breaker: 3 consecutive timeouts → 5-minute backoff (Epic 1, Task 1.3.2).
- Display `ScanStatus_TIMEOUT` as ⚠️ in UI rather than crashing the item.
- Never retry a timed-out repo immediately (circuit breaker handles this).

### Risk 2: fsnotify file descriptor exhaustion on macOS
**Likelihood:** Low for typical use (<50 repos); Medium for power users with large watch dirs.
**Impact:** Medium — watcher fails to start; fall back to polling.
**Mitigation:**
- Watch only `<repoRoot>/.git/` (1 fd per repo, not per file). 50 repos = 50 fds.
- Depth limit of 5 during walk prevents discovering excessive repos.
- Implement `useFallback` mode: if `fsnotify.NewWatcher()` fails, use 60s polling (Epic 2, Task 2.2.4).
- Consider adding a soft cap (e.g., warn at >100 repos in watch dirs) — UX decision for v1.1.

### Risk 3: AI summary subprocess hangs or rate limits
**Likelihood:** Medium (Claude CLI can hang on network issues; users may click Summarize repeatedly).
**Impact:** Low-Medium — UI shows loading spinner; background goroutines could accumulate.
**Mitigation:**
- Global semaphore of size 2 for concurrent AI subprocess calls (Epic 4, Task 4.4.1).
- Per-worktree mutex prevents double-summarizing the same worktree.
- 30s `exec.CommandContext` timeout on Claude subprocess; on timeout return error UI.
- 24h diff-hash cache eliminates re-calls for unchanged worktrees.

### Risk 4: Stale dismiss/snooze state after worktree re-creation
**Likelihood:** High (stapler-squad frequently deletes and recreates worktrees for the same branch).
**Impact:** Low-Medium — dismissed items might stay hidden when re-created at a new path.
**Mitigation:**
- Dismiss key is `(repoPath, branchName)` not `worktreePath` — stable across re-creation.
- Snooze key includes `snooze_since_sha`; auto-clears when HEAD SHA changes (Epic 3, Task 3.1.3).
- Startup cleanup validates all dismiss/snooze `repoPath` entries against filesystem (Task 3.1.4).

### Risk 5: Proto schema split requires new ConnectRPC service registration
**Likelihood:** Certain (deliberate architectural choice).
**Impact:** Medium — adds complexity to `server.go` registration and generated code path.
**Mitigation:**
- Follow exact same pattern as the existing `SessionService` handler registration (lines 202-205 in `server.go`).
- New service at `/api/unfinished/v1/` avoids any path conflicts with existing service.
- `make generate-proto` regenerates all client/server code atomically; `make ci` catches mismatches.
- ProtoBuffer file versioning: use `session.v1` package name for consistency (no separate package needed).

---

## Acceptance Criteria

The following numbered criteria map directly to requirements.md and will be referenced by
`validation.md` test cases.

1. A dedicated "Unfinished" tab exists in the main navigation header between Sessions and Review Queue.
2. The Unfinished tab displays a badge count showing the number of unfinished items (hidden when 0).
3. All git worktrees with uncommitted changes (`git status --porcelain` non-empty) are surfaced automatically.
4. All git worktrees with commits ahead of the default branch (`rev-list` count > 0) are surfaced.
5. All git worktrees with commits behind the default branch (`rev-list` count > 0) are surfaced.
6. A worktree qualifies if ANY of the three criteria (uncommitted, ahead, behind) are met.
7. Auto-spider source: for every active stapler-squad session, all git worktrees of its repo are scanned.
8. Watch dir source: all git repos (`.git` dirs) found within user-configured watch dirs (depth ≤ 5) are scanned.
9. Pinned repo source: explicitly added repo paths are scanned.
10. All three sources can be active simultaneously and contribute to the same item list.
11. Items are grouped by repository name (repo name as section header).
12. Within each repo group, items are sorted by most-recently-modified worktree first.
13. Each item card shows: branch name, abbreviated worktree path, and status chips (Uncommitted / ↑N ahead / ↓N behind).
14. Clicking an item card expands it inline (accordion) without navigating away.
15. The expanded accordion shows: changed file count, total lines added/removed, and up to 5 ahead commit messages.
16. The expanded accordion provides a [View Files] button that opens the existing file browser for the worktree.
17. The expanded accordion provides an [Open Session] button that creates or reattaches a stapler-squad session.
18. [Open Session] reattaches to an existing session if one already covers the branch; creates a new session otherwise.
19. A [Commit & Push] shortcut allows staging all changes, entering a commit message, and pushing — without opening a session.
20. [Commit & Push] runs as a one-shot background operation with progress indication and error reporting.
21. Items can be dismissed (permanently hidden); the dismiss state persists across application restarts.
22. Items can be snoozed (hidden until the next git state change in that worktree).
23. Snooze auto-clears when the worktree's HEAD SHA changes on the next scan.
24. An [AI Summary] button generates a 2-4 sentence natural-language description of the unfinished work, on demand.
25. The AI summary appears inline in the accordion; it is never auto-generated on scan.
26. The AI summary is cached by diff hash for 24 hours (same diff does not re-call Claude).
27. The Unfinished tab scans on a background schedule (default: every 30 seconds).
28. fsnotify on `.git/` directories triggers an immediate re-scan when a repo's git state changes.
29. A manual [Refresh] button in the tab header triggers an immediate full scan.
30. Filter chips (All / Uncommitted / Ahead / Behind) filter the item list client-side.
31. Watch dirs are configurable via the Settings page at `/settings/unfinished`.
32. Pinned repos are configurable via the Settings page at `/settings/unfinished`.
33. Auto-spider can be toggled on/off in the Settings page.
34. git commands are wrapped with TimeoutExecutor (5s for status, 3s for rev-list); timeouts show ⚠️ in UI.
35. Per-repo circuit breaker backs off to 5-minute interval after 3 consecutive timeouts.
36. Bare repos, detached HEAD worktrees, prunable worktrees, and locked worktrees are silently skipped.
37. Permission errors on watch dir subdirectories are logged at debug level only; they do not surface as UI errors.
38. The UnfinishedWorkService is registered as a separate ConnectRPC handler distinct from SessionService.
39. All new CSS uses vanilla-extract (`.css.ts` files); no new `.module.css` files are created.
40. `make ci` passes with all new code including proto generation, Go build, tests, and lint.
