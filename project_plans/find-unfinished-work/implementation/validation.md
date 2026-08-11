# Validation Plan: Unfinished Work Tab

Status: Draft | Phase: 4 — Validation complete
Created: 2026-04-25

---

## Coverage Matrix

| AC  | Description (abbreviated)                                         | Test IDs                     | Type(s)              |
|-----|-------------------------------------------------------------------|------------------------------|----------------------|
| AC-1  | "Unfinished" tab exists in main nav between Sessions and Review Queue | FT-001                  | frontend             |
| AC-2  | Tab badge shows count; hidden at 0                                | FT-002, FT-003               | frontend             |
| AC-3  | Worktrees with uncommitted changes are surfaced automatically      | UT-005, IT-001               | unit, integration    |
| AC-4  | Worktrees with commits ahead of default branch are surfaced        | UT-006, IT-001               | unit, integration    |
| AC-5  | Worktrees with commits behind default branch are surfaced          | UT-007, IT-001               | unit, integration    |
| AC-6  | A worktree qualifies if ANY criterion is met                       | UT-008, IT-001               | unit, integration    |
| AC-7  | Auto-spider: active sessions' repos are scanned                    | UT-016, IT-004               | unit, integration    |
| AC-8  | Watch dir: repos in user-configured dirs (depth ≤ 5) are scanned  | UT-017, IT-005               | unit, integration    |
| AC-9  | Pinned repo source: explicitly added paths are scanned             | UT-018, IT-006               | unit, integration    |
| AC-10 | All three sources active simultaneously, same item list            | IT-007                       | integration          |
| AC-11 | Items grouped by repository name                                   | FT-010, IT-002               | frontend, integration|
| AC-12 | Within each repo, items sorted by most-recently-modified first     | UT-013, FT-011               | unit, frontend       |
| AC-13 | Each item card shows branch, abbreviated path, status chips        | FT-004, FT-005               | frontend             |
| AC-14 | Clicking item expands inline accordion without navigation          | FT-006                       | frontend             |
| AC-15 | Expanded accordion shows file count, ±lines, ≤5 commit messages   | FT-007, UT-009               | frontend, unit       |
| AC-16 | [View Files] button opens file browser for the worktree            | FT-013, MS-001               | frontend, manual     |
| AC-17 | [Open Session] creates or reattaches a session                     | FT-014, IT-008               | frontend, integration|
| AC-18 | [Open Session] reattaches if session exists; creates otherwise     | FT-015, IT-008               | frontend, integration|
| AC-19 | [Commit & Push] shortcut stages, commits, pushes                   | UT-019, IT-009               | unit, integration    |
| AC-20 | [Commit & Push] is one-shot background operation with progress     | FT-016, IT-009               | frontend, integration|
| AC-21 | Dismiss state persists across restarts                             | UT-021, IT-010               | unit, integration    |
| AC-22 | Snooze hides item until next git state change                      | UT-022, IT-011               | unit, integration    |
| AC-23 | Snooze auto-clears when HEAD SHA changes on next scan              | UT-022, IT-011               | unit, integration    |
| AC-24 | [AI Summary] generates 2-4 sentence description on demand          | UT-025, MS-002               | unit, manual         |
| AC-25 | AI summary never auto-generated on scan; only on demand            | UT-026, FT-021               | unit, frontend       |
| AC-26 | AI summary cached by diff hash for 24h                             | UT-027                       | unit                 |
| AC-27 | Background scan runs on 30-second schedule                         | UT-015, IT-012               | unit, integration    |
| AC-28 | fsnotify on `.git/` triggers immediate re-scan                     | IT-013, MS-003               | integration, manual  |
| AC-29 | Manual [Refresh] button triggers immediate full scan               | FT-019, IT-014               | frontend, integration|
| AC-30 | Filter chips (All/Uncommitted/Ahead/Behind) filter client-side     | FT-017, FT-018               | frontend             |
| AC-31 | Watch dirs configurable at `/settings/unfinished`                  | FT-022, IT-015               | frontend, integration|
| AC-32 | Pinned repos configurable at `/settings/unfinished`                | FT-023, IT-015               | frontend, integration|
| AC-33 | Auto-spider can be toggled on/off in Settings                      | FT-024, IT-016               | frontend, integration|
| AC-34 | TimeoutExecutor wraps git commands; timeouts show ⚠️ in UI         | UT-010, FT-008               | unit, frontend       |
| AC-35 | Per-repo circuit breaker backs off after 3 consecutive timeouts    | UT-011                       | unit                 |
| AC-36 | Bare, detached HEAD, prunable, locked worktrees silently skipped   | UT-001, UT-002, UT-003, UT-004 | unit               |
| AC-37 | Permission errors logged at debug level only; not shown in UI      | UT-020, IT-017               | unit, integration    |
| AC-38 | UnfinishedWorkService registered as separate ConnectRPC handler    | IT-018                       | integration          |
| AC-39 | All new CSS uses vanilla-extract; no new `.module.css` files       | MS-004                       | manual               |
| AC-40 | `make ci` passes (proto gen, Go build, tests, lint)                | MS-005                       | manual               |

---

## Test Cases

### Go Unit Tests

All unit tests live in the `session/unfinished/` package unless noted. They use no subprocess
calls — git CLI is either mocked via a fake executor or exercised against fixture strings.

---

#### UT-001: ParseAllWorktrees — bare repo skipped
**File:** `session/git/util_worktrees_test.go`
**Setup:** Fixture string containing one bare worktree entry (porcelain `bare` token).
**Asserts:**
- Returned slice has `IsBare == true` for that entry.
- `ParseAllWorktrees` itself does not filter — the scanning layer filters on `IsBare`.
- No error returned.
**Covers:** AC-36

---

#### UT-002: ParseAllWorktrees — detached HEAD worktree parsed correctly
**File:** `session/git/util_worktrees_test.go`
**Setup:** Fixture string with `detached` token, no `branch` line.
**Asserts:**
- `IsDetached == true`.
- `Branch` field is empty string.
- No error returned.
**Covers:** AC-36

---

#### UT-003: ParseAllWorktrees — prunable worktree parsed and flagged
**File:** `session/git/util_worktrees_test.go`
**Setup:** Fixture string containing `prunable` annotation.
**Asserts:**
- `IsPrunable == true`.
- Entry included in return slice with flag set.
**Covers:** AC-36

---

#### UT-004: ParseAllWorktrees — locked worktree parsed and flagged
**File:** `session/git/util_worktrees_test.go`
**Setup:** Fixture string containing `locked` annotation.
**Asserts:**
- `IsLocked == true`.
**Covers:** AC-36

---

#### UT-005: scanWorktree — uncommitted changes detected
**File:** `session/unfinished/scanner_test.go`
**Setup:** Mock executor returns non-empty output for `git status --porcelain`. Rev-list returns `0\t0`.
**Asserts:**
- `ScanResult.HasUncommitted == true`.
- `ScanResult.AheadCount == 0`, `BehindCount == 0`.
- `ScanResult.Status == ScanResultStatusOK`.
**Covers:** AC-3

---

#### UT-006: scanWorktree — commits ahead detected
**File:** `session/unfinished/scanner_test.go`
**Setup:** Mock executor returns empty `git status --porcelain`. Rev-list `--left-right` returns `3\t0`.
**Asserts:**
- `ScanResult.HasUncommitted == false`.
- `ScanResult.AheadCount == 3`.
- `ScanResult.BehindCount == 0`.
**Covers:** AC-4

---

#### UT-007: scanWorktree — commits behind detected
**File:** `session/unfinished/scanner_test.go`
**Setup:** Rev-list returns `0\t5`.
**Asserts:**
- `ScanResult.BehindCount == 5`.
- `ScanResult.AheadCount == 0`.
**Covers:** AC-5

---

#### UT-008: scanWorktree — qualifies when only one criterion met
**File:** `session/unfinished/scanner_test.go`
**Setup:** Three sub-cases as table-driven test: (1) uncommitted only, (2) ahead only, (3) behind only.
**Asserts:** Each sub-case: `IsUnfinished()` helper returns `true` when any single criterion is met; returns `false` when all three are zero.
**Covers:** AC-6

---

#### UT-009: scanWorktree — diff stats and ahead commit messages parsed
**File:** `session/unfinished/scanner_test.go`
**Setup:** Mock executor returns `git diff --shortstat HEAD` output `3 files changed, 142 insertions(+), 28 deletions(-)`. Mock `git log` returns 2 commit lines.
**Asserts:**
- `ChangedFiles == 3`, `LinesAdded == 142`, `LinesRemoved == 28`.
- `AheadMessages` slice has length 2.
- Empty diff → `ChangedFiles == 0`, `LinesAdded == 0`, `LinesRemoved == 0`.
**Covers:** AC-15

---

#### UT-010: scanWorktree — timeout sets ScanStatus to Timeout
**File:** `session/unfinished/scanner_test.go`
**Setup:** Mock executor returns `context.DeadlineExceeded` for `git status`.
**Asserts:**
- `ScanResult.Status == ScanResultStatusTimeout`.
- `ScanResult.ErrorMsg` is non-empty.
- No panic.
**Covers:** AC-34

---

#### UT-011: circuit breaker — backs off after 3 consecutive timeouts
**File:** `session/unfinished/cache_test.go`
**Setup:** Instantiate `repoBreakerState`; record 3 consecutive timeout results via `RecordTimeout()`.
**Asserts:**
- After 3rd timeout, `ShouldScan()` returns `false`.
- `ShouldScan()` returns `true` again after 5 minutes have elapsed (mocked time).
- After a successful scan result, `ShouldScan()` resets to `true` immediately.
**Covers:** AC-35

---

#### UT-012: worktreeCache — TTL expiry
**File:** `session/unfinished/cache_test.go`
**Setup:** Create cache with 30s TTL. `Set(result)`. Immediately call `Get()` → should return `(result, true)`. Advance time by 31s. Call `Get()` → should return `(_, false)`.
**Asserts:**
- Fresh entry: `Get()` returns `(result, true)`.
- Expired entry: `Get()` returns `(zeroValue, false)`.
**Covers:** (internal correctness; underpins AC-27)

---

#### UT-013: SortByLastModified — descending order
**File:** `session/unfinished/scanner_test.go`
**Setup:** Slice of 4 `ScanResult` with varying `LastModified` times.
**Asserts:**
- After sort, entries are in descending `LastModified` order.
- Equal `LastModified` values produce deterministic order (stable on `RepoPath+Branch`).
**Covers:** AC-12

---

#### UT-014: resolveDefaultBranch — uses origin/HEAD, falls back to main/master
**File:** `session/unfinished/scanner_test.go`
**Setup:**
- Sub-case A: mock executor returns `origin/main` for `git symbolic-ref refs/remotes/origin/HEAD --short` → result is `main`.
- Sub-case B: mock executor returns exit code 128 (no origin/HEAD); `git merge-base --is-ancestor main HEAD` succeeds → result is `main`.
- Sub-case C: no origin, no main/master/develop/trunk → result is `""`, no error.
**Asserts:** Each sub-case returns the expected default branch name or empty string.
**Covers:** (underpins AC-3, AC-4, AC-5; the no-default-branch edge case from pitfalls.md)

---

#### UT-015: Scanner.Start — 30-second tick enqueues repo scan
**File:** `session/unfinished/scanner_test.go`
**Setup:** Instantiate `Scanner` with a mock ticker (manually advanceable). Register one repo in the source set.
**Asserts:**
- Advancing the mock ticker by 30s results in one `scanTask` arriving in `scanQueue`.
- Context cancellation cleanly drains goroutines (no goroutine leak detected via `goleak`).
**Covers:** AC-27

---

#### UT-016: Scanner auto-spider — SessionCreated event enqueues repo scan
**File:** `session/unfinished/scanner_test.go`
**Setup:** Publish `SessionCreated` event with a known `session.Path`. Mock `findGitRepoRoot` to return a deterministic repo root.
**Asserts:**
- Exactly one `scanTask` with the repo root is enqueued within one event loop iteration.
**Covers:** AC-7

---

#### UT-017: WatchDirWatcher.Start — repos discovered at depth ≤ 5
**File:** `session/unfinished/watcher_test.go`
**Setup:** Create a temp dir tree with git repos at depths 1, 3, 5, and 6. Instantiate `WatchDirWatcher` with the temp root. Call `Start()` synchronously (drain the initial scan).
**Asserts:**
- Repos at depths 1, 3, 5 are discovered and enqueued.
- Repo at depth 6 is NOT discovered.
- `node_modules`, `vendor`, `.cache`, `dist`, `build` directories are skipped.
**Covers:** AC-8

---

#### UT-018: Scanner.AddPinnedRepo — invalid path returns error
**File:** `session/unfinished/scanner_test.go`
**Setup:**
- Sub-case A: path does not exist → returns `os.ErrNotExist`-wrapped error.
- Sub-case B: path exists but has no `.git` directory → returns validation error.
- Sub-case C: valid git repo path → `EnqueueRepo` called, no error returned.
**Asserts:** Error presence and type per sub-case.
**Covers:** AC-9

---

#### UT-019: QuickCommitPush — runs git add, commit, push in sequence
**File:** `session/unfinished/scanner_test.go` (or `server/services/unfinished_work_service_test.go`)
**Setup:** Mock executor captures all subprocess calls in order.
**Asserts:**
- Exactly 3 commands issued: `git add .`, `git commit -m <msg>`, `git push -u origin <branch>`.
- Empty commit message returns validation error before any git calls.
- Push failure returns human-readable error message (not a raw exec error).
**Covers:** AC-19

---

#### UT-020: Walk permission error — logged at debug level, not propagated
**File:** `session/unfinished/watcher_test.go`
**Setup:** Walk a directory tree containing one sub-directory with `chmod 000`. Capture log output.
**Asserts:**
- Walk completes without returning an error.
- Log output contains a debug-level entry for the inaccessible path.
- No error-level log entries for permission errors.
**Covers:** AC-37

---

#### UT-021: StateStore.Dismiss — persists across reload
**File:** `session/unfinished/state_test.go`
**Setup:** Create `StateStore` with temp file path. Call `Dismiss("repo/path", "feature-x")`. Create a new `StateStore` loading from the same path.
**Asserts:**
- Reloaded store: `IsDismissed("repo/path", "feature-x") == true`.
- `IsDismissed("repo/path", "other-branch") == false`.
- State file was written atomically (temp file replaced original).
**Covers:** AC-21

---

#### UT-022: StateStore.Snooze — auto-clears when SHA changes
**File:** `session/unfinished/state_test.go`
**Setup:** Call `Snooze("repo/path", "fix-y", "abc123")`.
**Asserts:**
- `IsSnoozed("repo/path", "fix-y", "abc123") == true`.
- `IsSnoozed("repo/path", "fix-y", "def456") == false` (different SHA clears snooze).
- After the second call, the snooze entry is removed from the state store (not just returning false).
**Covers:** AC-22, AC-23

---

#### UT-023: StateStore — startup cleanup removes stale dismissed entries
**File:** `session/unfinished/state_test.go`
**Setup:** Write a state JSON file with a dismissed entry whose `repo_path` does not exist on disk. Call `StateStore.Load()`.
**Asserts:**
- After load, `IsDismissed` for the non-existent path returns `false`.
- The state file is rewritten without the stale entry.
**Covers:** (Risk 4 from plan; underpins AC-21)

---

#### UT-024: StateStore — atomic write survives mid-write interruption
**File:** `session/unfinished/state_test.go`
**Setup:** Write initial state. Mock `os.Rename` to fail on first call, succeed on retry. Call `save()`.
**Asserts:**
- No corruption of original file when rename fails.
- On successful retry, original file reflects new state.
**Covers:** (AC-21 — persistence correctness)

---

#### UT-025: GetCachedSummary and CacheSummary — round-trip and TTL
**File:** `session/unfinished/state_test.go`
**Setup:**
- Sub-case A: `CacheSummary(repo, branch, "hash1", "summary text")`. Then `GetCachedSummary(repo, branch, "hash1")` → returns `("summary text", true)`.
- Sub-case B: advance time by 25 hours. `GetCachedSummary(repo, branch, "hash1")` → returns `("", false)` (expired).
- Sub-case C: `GetCachedSummary(repo, branch, "different-hash")` → returns `("", false)` (hash mismatch).
**Asserts:** Each sub-case returns expected `(string, bool)` tuple.
**Covers:** AC-26

---

#### UT-026: GetWorktreeAISummary — not triggered during scan
**File:** `session/unfinished/scanner_test.go`
**Setup:** Run `Scanner.scanRepo()` end-to-end with mock executor. Capture any subprocess calls matching `claude`.
**Asserts:**
- No subprocess call matching `claude` or AI summary is issued during scan.
- `ScanResult` has empty `AISummary` field after scan.
**Covers:** AC-25

---

#### UT-027: ComputeDiffHash — same diff produces same hash; different diff produces different hash
**File:** `session/unfinished/state_test.go`
**Setup:** Mock executor returns fixed diff output. Call `ComputeDiffHash` twice.
**Asserts:**
- Same diff content → same SHA256 hash both calls.
- Different diff content → different hash.
- Empty diff → consistent, non-empty hash (hash of empty string).
**Covers:** AC-26

---

#### UT-028: Scanner.publishResults — dismissed items excluded from events
**File:** `session/unfinished/scanner_test.go`
**Setup:** Dismiss `(repoA, branchX)`. Call `publishResults` with results including a `ScanResult` for `(repoA, branchX)`.
**Asserts:**
- `EventBus` receives no `UnfinishedWorkUpdated` event for `(repoA, branchX)`.
- Events are published for non-dismissed results.
**Covers:** (AC-21 — dismissed items excluded at publish time)

---

#### UT-029: Scanner.EnqueueRepo — deduplication within TTL window
**File:** `session/unfinished/scanner_test.go`
**Setup:** Call `EnqueueRepo("repo/path")` twice within 30 seconds (no cache expiry).
**Asserts:**
- Exactly one task arrives in `scanQueue`; second call is dropped.
- Third call after 31 seconds results in a second task.
**Covers:** (AC-27 — thundering-herd prevention; underpins AC-8, AC-7)

---

### Go Integration Tests

Integration tests use real temp git repositories and real subprocess calls. All tests use
`os.MkdirTemp` for isolation and call `t.Cleanup(os.RemoveAll)`.

---

#### IT-001: Full scan — uncommitted, ahead, and behind worktrees all appear in ListUnfinishedWork
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Create temp git repo with `main` branch.
2. Create worktree `wt-uncommitted` with an untracked file.
3. Create worktree `wt-ahead` with one committed-but-not-pushed commit beyond `main`.
4. Create worktree `wt-behind` on a branch that has not merged one commit from `main`.
5. Start `Scanner` with the repo as a pinned repo. Wait for scan to complete.
**Asserts:**
- `ListUnfinishedWork` response contains exactly 3 entries.
- `wt-uncommitted` has `HasUncommitted == true`, `AheadCount == 0`, `BehindCount == 0`.
- `wt-ahead` has `AheadCount > 0`.
- `wt-behind` has `BehindCount > 0`.
- All entries have `ScanStatus == SCAN_STATUS_OK`.
**Covers:** AC-3, AC-4, AC-5, AC-6

---

#### IT-002: ListUnfinishedWork — results grouped by repo name with correct sort order
**File:** `server/services/unfinished_work_service_test.go`
**Setup:** Two repos, each with 2 worktrees. Worktrees have different mtimes (controlled via `os.Chtimes`).
**Asserts:**
- Distinct `repo_name` values appear in response; each has the correct worktrees.
- Within each repo, worktrees ordered by `last_modified` descending.
**Covers:** AC-11, AC-12

---

#### IT-003: StateStore persistence — dismiss survives reload; undismiss restores
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Call `DismissWorktree(repoPath, branch)` via RPC.
2. `ListUnfinishedWork` → dismissed item absent.
3. Reload `StateStore` from disk. `ListUnfinishedWork` again → still absent.
4. Call `UndismissWorktree(repoPath, branch)` via RPC. `ListUnfinishedWork` → item present.
**Asserts:** Item visibility matches expected state at each step.
**Covers:** AC-21

---

#### IT-004: Auto-spider — SessionCreated event triggers repo scan
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Create temp repo with one uncommitted worktree.
2. Instantiate `Scanner` with auto-spider enabled but no pre-configured watch dirs.
3. Publish `SessionCreated` event with `Path` pointing into the temp repo.
4. Wait up to 5s for scan to complete.
**Asserts:**
- `ListUnfinishedWork` returns the uncommitted worktree after the event fires.
**Covers:** AC-7

---

#### IT-005: Watch dir source — repos discovered within depth ≤ 5
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Create temp root dir with a nested git repo at depth 3 that has an uncommitted change.
2. `UpdateUnfinishedWorkConfig` to add the root dir as a watch dir.
3. Wait for initial walk scan to complete.
**Asserts:**
- Repo at depth 3 appears in `ListUnfinishedWork`.
**Covers:** AC-8

---

#### IT-006: Pinned repo source — valid path scanned; invalid path returns error
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
- Sub-case A: `UpdateUnfinishedWorkConfig` with a valid git repo path → next `ListUnfinishedWork` includes that repo's worktrees.
- Sub-case B: `UpdateUnfinishedWorkConfig` with a non-git directory path → RPC returns error with descriptive message.
**Asserts:** Sub-case A returns results; sub-case B returns a non-nil error response.
**Covers:** AC-9

---

#### IT-007: All three sources simultaneously active
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Three repos: one from session auto-spider, one from watch dir, one as pinned repo.
2. All three have uncommitted changes.
3. Scanner started with all sources enabled.
**Asserts:**
- `ListUnfinishedWork` returns items from all three repos in a single response.
**Covers:** AC-10

---

#### IT-008: OpenSession — creates new session for worktree with no existing session; reattaches if session exists
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
- Sub-case A: worktree has `session_id == ""`. Trigger "Open Session" equivalent (`CreateSession` with `existing_worktree`). Verify a new session is created.
- Sub-case B: pre-create a session covering the worktree branch. Verify the `UnfinishedWorktree.session_id` is populated. Verify clicking Open Session does not create a second session.
**Asserts:**
- Sub-case A: new session created, `session_id` populated in subsequent `ListUnfinishedWork`.
- Sub-case B: existing session ID returned; session count unchanged.
**Covers:** AC-17, AC-18

---

#### IT-009: QuickCommitPush — real git commit and push via mock remote
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Create two temp repos: `origin` (bare) and `local` (cloned from origin).
2. Create an uncommitted file in `local`.
3. Call `QuickCommitPush` RPC with commit message "test commit".
4. Inspect `origin` via `git log`.
**Asserts:**
- `origin` has exactly one new commit with message "test commit".
- `local` has no unstaged changes after the operation.
- Empty commit message returns RPC error before any git operation.
**Covers:** AC-19, AC-20

---

#### IT-010: Dismiss — dismissed item absent from list; persists across state reload
**File:** `server/services/unfinished_work_service_test.go`
**Setup:** Same as IT-003 but specifically validates `EventBus` emits `UnfinishedWorkRemoved` after dismiss.
**Asserts:**
- `WatchUnfinishedWork` stream emits a `worktree_removed` event after `DismissWorktree` call.
**Covers:** AC-21

---

#### IT-011: Snooze — item hidden; reappears when HEAD SHA changes
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Snooze worktree with current HEAD SHA.
2. `ListUnfinishedWork` → worktree absent.
3. Make a new commit in the worktree (changes HEAD SHA).
4. Trigger a re-scan.
5. `ListUnfinishedWork` → worktree present.
**Asserts:** Item reappears without any user action after SHA change.
**Covers:** AC-22, AC-23

---

#### IT-012: Background scheduler — scan triggered without manual intervention
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Start scanner with 200ms tick interval (override for test).
2. Create an uncommitted file in a watched repo.
3. Wait 400ms (two ticks).
**Asserts:**
- `ListUnfinishedWork` returns the newly-uncommitted worktree without calling `ScanUnfinishedWork`.
**Covers:** AC-27

---

#### IT-013: fsnotify — `.git/index` write triggers immediate re-scan
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Start scanner. Verify initial state is empty for a clean repo.
2. Write a file in the worktree → git index changes.
3. Wait up to 2s for fsnotify event and subsequent scan.
**Asserts:**
- `ListUnfinishedWork` returns the worktree within 2 seconds of the file write.
- No 30-second wait required.
**Covers:** AC-28

---

#### IT-014: ScanUnfinishedWork RPC — triggers immediate scan
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Start scanner with 24-hour tick interval (never auto-scans).
2. Create uncommitted file.
3. Call `ScanUnfinishedWork` RPC.
4. Wait up to 5s for `ScanCompleted` event on stream.
**Asserts:**
- `ScanCompleted` event received on `WatchUnfinishedWork` stream within 5s.
- `ListUnfinishedWork` returns the uncommitted worktree after scan completes.
**Covers:** AC-29

---

#### IT-015: UpdateUnfinishedWorkConfig — watch dir and pinned repo persist
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Call `UpdateUnfinishedWorkConfig` to add a watch dir and a pinned repo.
2. Call `GetUnfinishedWorkConfig` → assert both are present.
3. Reload `StateStore` from disk. Call `GetUnfinishedWorkConfig` again.
**Asserts:**
- Config round-trips correctly through RPC and survives StateStore reload.
**Covers:** AC-31, AC-32

---

#### IT-016: Auto-spider toggle — disabling stops new session repos from being scanned
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Disable auto-spider via `UpdateUnfinishedWorkConfig`.
2. Publish `SessionCreated` event.
3. Wait 500ms.
**Asserts:**
- `scanQueue` receives no tasks from the auto-spider path after disabling.
**Covers:** AC-33

---

#### IT-017: Permission error on watch dir subdirectory — not surfaced in ListUnfinishedWork
**File:** `server/services/unfinished_work_service_test.go`
**Setup:**
1. Create a temp root dir with a `chmod 000` subdirectory.
2. Configure as a watch dir.
3. Trigger a scan.
**Asserts:**
- `ListUnfinishedWork` returns no error.
- No item in the response has `ScanStatus == SCAN_STATUS_PERMISSION` (only applies to repos the user explicitly pinned).
- Log output at debug level contains the permission-denied path.
**Covers:** AC-37

---

#### IT-018: Service registration — ConnectRPC handler reachable at `/api/unfinished/v1/`
**File:** `server/services/unfinished_work_service_test.go` (or a server-level smoke test)
**Setup:** Start test HTTP server (via `httptest.NewServer`) with all handlers registered.
**Asserts:**
- `POST /api/unfinished/v1/session.v1.UnfinishedWorkService/ListUnfinishedWork` returns HTTP 200 (not 404).
- The handler is distinct from `SessionService` (separate handler path).
**Covers:** AC-38

---

### Frontend Tests

All frontend tests use React Testing Library (RTL) + Jest. Mock hooks (jest.mock) for ConnectRPC
streaming hooks to avoid real network calls. Test files live colocated with components
(`__tests__/` subdirectory or `.test.tsx` sibling).

---

#### FT-001: UnfinishedTab renders in nav between Sessions and Review Queue
**File:** `web-app/src/components/layout/__tests__/Header.test.tsx`
**Setup:** Render `<Header />` with mock store providing 0 unfinished items.
**Interaction:** None.
**Asserts:**
- Navigation contains "Unfinished" link.
- Link `href` equals `/unfinished`.
- Visually between Sessions and Review Queue links (DOM order check).
**Covers:** AC-1

---

#### FT-002: UnfinishedNavBadge — shows count when non-zero
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedNavBadge.test.tsx`
**Setup:** Mock `useUnfinishedWork` to return 7 items.
**Interaction:** None.
**Asserts:**
- Badge element visible with text "7".
**Covers:** AC-2

---

#### FT-003: UnfinishedNavBadge — hidden when count is 0
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedNavBadge.test.tsx`
**Setup:** Mock `useUnfinishedWork` to return 0 items.
**Asserts:**
- Badge element not in the document (or has `display: none`).
**Covers:** AC-2

---

#### FT-004: UnfinishedItem — renders branch name, abbreviated path, status chips
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItem.test.tsx`
**Setup:** Render `<UnfinishedItem>` with mock worktree: `branch="feature-auth"`, `worktreePath="/Users/tyler/code/repo/feature-auth"`, `hasUncommitted=true`, `commitsAhead=4`, `commitsBehind=0`.
**Asserts:**
- Text "feature-auth" present.
- Path displayed with `~` substitution: `~/code/repo/feature-auth`.
- "Uncommitted" chip present.
- "↑4" or "Ahead 4" chip present.
- No "Behind" chip (count is 0 — AC-13 specifies chips appear only for relevant signals).
**Covers:** AC-13

---

#### FT-005: UnfinishedItem — no "Ahead 0" or "Behind 0" chips shown
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItem.test.tsx`
**Setup:** Render with `commitsAhead=0`, `commitsBehind=0`, `hasUncommitted=true`.
**Asserts:**
- "Ahead 0" text is absent from the DOM.
- "Behind 0" text is absent from the DOM.
**Covers:** AC-13

---

#### FT-006: UnfinishedItem — accordion expands inline on click; does not navigate
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItem.test.tsx`
**Setup:** Render within a mock router context. Click the item card.
**Asserts:**
- `UnfinishedItemDetail` section becomes visible.
- `window.location.pathname` unchanged (no navigation).
- Clicking again collapses the detail section.
**Covers:** AC-14

---

#### FT-007: UnfinishedItemDetail — shows diff stats and commit messages
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItemDetail.test.tsx`
**Setup:** Render `<UnfinishedItemDetail>` with `changedFiles=3`, `linesAdded=142`, `linesRemoved=28`, `aheadCommitMessages=["WIP: extract builder", "Add tests"]`.
**Asserts:**
- "3 files changed" text present.
- "+142" and "−28" present.
- Both commit messages displayed.
- Renders without error when `aheadCommitMessages` is empty (no ahead commits).
- Shows "No uncommitted changes" when `changedFiles=0` and `hasUncommitted=false`.
**Covers:** AC-15

---

#### FT-008: UnfinishedItem — timeout status shows ⚠️ indicator instead of chips
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItem.test.tsx`
**Setup:** Render with `scanStatus="SCAN_STATUS_TIMEOUT"`.
**Asserts:**
- A warning indicator (⚠️ or `aria-label="scan timed out"`) is present.
- "Uncommitted", "Ahead", "Behind" chips are absent.
**Covers:** AC-34

---

#### FT-009: UnfinishedRepoGroup — expands/collapses on click; keyboard accessible
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedRepoGroup.test.tsx`
**Setup:** Render `<UnfinishedRepoGroup>` with 3 child items.
**Interaction:** (1) Click group header. (2) `fireEvent.keyDown(header, { key: 'Enter' })`.
**Asserts:**
- After click: children visible (not hidden).
- After second click: children hidden.
- Enter key also toggles expansion.
- Space key also toggles expansion.
**Covers:** (AC-11 — repo grouping UX)

---

#### FT-010: UnfinishedTab — items grouped by repo name
**File:** `web-app/src/app/unfinished/__tests__/UnfinishedTab.test.tsx`
**Setup:** Mock `useUnfinishedWork` to return 5 worktrees across 2 repo names ("repo-a" × 3, "repo-b" × 2).
**Asserts:**
- Two `UnfinishedRepoGroup` elements rendered.
- "repo-a" group has 3 items; "repo-b" group has 2 items.
- Item count badges match.
**Covers:** AC-11

---

#### FT-011: UnfinishedTab — within-repo items sorted by most-recently-modified
**File:** `web-app/src/app/unfinished/__tests__/UnfinishedTab.test.tsx`
**Setup:** Mock data with two items in "repo-a": item A has `lastModified` 5 minutes ago, item B has `lastModified` 1 minute ago.
**Asserts:**
- Item B rendered before item A within the "repo-a" group.
**Covers:** AC-12

---

#### FT-012: UnfinishedTab — Refresh button calls triggerScan
**File:** `web-app/src/app/unfinished/__tests__/UnfinishedTab.test.tsx`
**Setup:** Mock `useUnfinishedWork` hook exposing a spy `triggerScan`. Render tab.
**Interaction:** Click "Refresh" button.
**Asserts:**
- `triggerScan` spy called exactly once.
**Covers:** AC-29

---

#### FT-013: [View Files] button opens file browser for worktree
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItemDetail.test.tsx`
**Setup:** Mock the file browser navigation function. Render expanded item.
**Interaction:** Click "View Files".
**Asserts:**
- File browser navigation function called with `worktreePath` as root.
**Covers:** AC-16

---

#### FT-014: [Open Session] — calls CreateSession when session_id is empty
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItemDetail.test.tsx`
**Setup:** Mock `CreateSession` RPC. Render with `session_id=""`.
**Interaction:** Click "Open Session".
**Asserts:**
- `CreateSession` called with `existing_worktree == worktreePath`.
**Covers:** AC-17

---

#### FT-015: [Open Session] — navigates to existing session when session_id is set
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItemDetail.test.tsx`
**Setup:** Render with `session_id="existing-session-123"`. Mock router navigation.
**Interaction:** Click "Open Session".
**Asserts:**
- Navigation goes to session detail route with ID "existing-session-123".
- `CreateSession` is NOT called.
**Covers:** AC-18

---

#### FT-016: CommitPushModal — empty message prevents submit; success closes modal; error shown inline
**File:** `web-app/src/components/unfinished/__tests__/CommitPushModal.test.tsx`
**Setup:** Mock `QuickCommitPush` RPC.
**Interaction A:** Open modal, leave message empty, click "Commit & Push".
**Interaction B:** Enter message, click "Commit & Push" → mock returns success.
**Interaction C:** Enter message, click → mock returns error "push rejected".
**Asserts:**
- A: submit button disabled or validation message shown; RPC not called.
- B: modal closes after success.
- C: modal stays open; error message "push rejected" visible inline.
**Covers:** AC-19, AC-20

---

#### FT-017: Filter chips — "Uncommitted" shows only uncommitted items
**File:** `web-app/src/app/unfinished/__tests__/UnfinishedTab.test.tsx`
**Setup:** Mock data with 3 items: uncommitted-only, ahead-only, behind-only.
**Interaction:** Click "Uncommitted" chip.
**Asserts:**
- Only the uncommitted item card is visible.
- No RPC call is made (filtering is client-side).
**Covers:** AC-30

---

#### FT-018: Filter chips — "All" shows all items
**File:** `web-app/src/app/unfinished/__tests__/UnfinishedTab.test.tsx`
**Interaction:** Click "Uncommitted" then click "All".
**Asserts:**
- All 3 items visible after clicking "All".
**Covers:** AC-30

---

#### FT-019: Background refresh indicator — counter updates every second; spinner during scan
**File:** `web-app/src/app/unfinished/__tests__/UnfinishedTab.test.tsx`
**Setup:** Mock `useUnfinishedWork` returning `lastScanTime` and `isScanning=false`. Use `jest.useFakeTimers()`.
**Interaction:** Advance time by 3 seconds.
**Asserts:**
- "Last scanned 3 seconds ago" text updates.
- When `isScanning=true`, a spinner element is present.
- When `isScanning=false`, spinner absent.
**Covers:** AC-29 (manual refresh trigger), (background scan UX — related to AC-27)

---

#### FT-020: [Dismiss] — item removed optimistically; undo toast shown
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItem.test.tsx`
**Setup:** Mock `DismissWorktree` RPC. Render item.
**Interaction:** Click dismiss button (× or "Dismiss").
**Asserts:**
- Item removed from DOM immediately (optimistic update).
- Toast with "Dismissed. Undo?" appears.
- Clicking "Undo" in toast calls `UndismissWorktree` RPC.
- Item reappears in list after undo.
**Covers:** AC-21

---

#### FT-021: [Snooze] — item removed immediately; no undo option
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItem.test.tsx`
**Setup:** Mock `SnoozeWorktree` RPC.
**Interaction:** Click snooze button.
**Asserts:**
- Item removed from DOM immediately.
- No undo toast appears.
- `SnoozeWorktree` RPC called with correct `repo_path` and `branch`.
**Covers:** AC-22

---

#### FT-022: Settings — Watch Dirs list renders; add and remove work
**File:** `web-app/src/components/settings/__tests__/UnfinishedSourcesSettings.test.tsx`
**Setup:** Mock `useUnfinishedWorkConfig` hook returning a config with one watch dir.
**Interaction A:** Click "✕" next to existing watch dir → mock `updateConfig`.
**Interaction B:** Type new path in Add Dir input, click "Add" → mock `updateConfig`.
**Asserts:**
- A: `updateConfig` called with watch_dirs missing the removed entry.
- B: `updateConfig` called with watch_dirs including the new path.
**Covers:** AC-31

---

#### FT-023: Settings — Pinned Repos list renders; add and remove work
**File:** `web-app/src/components/settings/__tests__/UnfinishedSourcesSettings.test.tsx`
**Setup/Interaction/Asserts:** Mirrors FT-022 but for `pinned_repos`.
**Covers:** AC-32

---

#### FT-024: Settings — Auto-spider toggle persists on/off
**File:** `web-app/src/components/settings/__tests__/UnfinishedSourcesSettings.test.tsx`
**Setup:** Mock hook returning `autoSpiderSessions=true`.
**Interaction:** Click the toggle to disable.
**Asserts:**
- `updateConfig` called with `auto_spider_sessions=false`.
**Covers:** AC-33

---

#### FT-025: AI Summary — inline display; loading state; error handling; no re-fetch on re-expand
**File:** `web-app/src/components/unfinished/__tests__/UnfinishedItemDetail.test.tsx`
**Setup:**
- Sub-case A: Mock `GetWorktreeAISummary` to return a summary. Click "Summarize". Assert spinner shown during loading, then summary text appears inline.
- Sub-case B: Mock RPC returns error. Assert "Summary unavailable — try again." shown.
- Sub-case C: Collapse and re-expand accordion after successful summary. Assert RPC not called a second time (React state cache).
**Asserts:** Per sub-case as described.
**Covers:** AC-24, AC-25 (UI layer)

---

### Manual Scenarios

Manual scenarios cover behaviors that require a real running application, or that verify
visual/CSS constraints not easily tested with RTL.

---

#### MS-001: [View Files] opens correct file browser
**Steps:**
1. Start stapler-squad with a real git repo containing a worktree.
2. Navigate to the Unfinished tab.
3. Expand a worktree item.
4. Click "View Files".
**Expected:** The existing file browser opens rooted at the worktree directory. File listing shows actual files from that worktree.
**Covers:** AC-16

---

#### MS-002: AI Summary generates coherent 2-4 sentence description
**Steps:**
1. Have a worktree with ≥3 changed files and ≥1 commit ahead of main.
2. Click "Show AI Summary" (or "Summarize") in the expanded accordion.
3. Wait for generation (up to 30s).
**Expected:** A 2-4 sentence paragraph appears describing the nature of the changes in plain language. No error message. Summary is coherent and references actual file/function names from the diff.
**Covers:** AC-24

---

#### MS-003: fsnotify triggers re-scan in under 2 seconds
**Steps:**
1. Start stapler-squad. Navigate to Unfinished tab. Note "Last scanned N seconds ago".
2. In a terminal, create a new untracked file in a watched worktree: `touch /path/to/worktree/new-file.txt`.
3. Observe the Unfinished tab.
**Expected:** The worktree appears (or its chip updates) within 2 seconds of creating the file — without manually clicking Refresh.
**Covers:** AC-28

---

#### MS-004: All new CSS uses vanilla-extract; no `.module.css` files added
**Steps:**
1. Run: `find web-app/src/components/unfinished web-app/src/app/unfinished -name "*.module.css"`.
2. Run: `make ci` and confirm the `lint:css` step passes.
**Expected:**
- The `find` command returns no results.
- `make ci` passes without CSS lint errors.
**Covers:** AC-39

---

#### MS-005: `make ci` passes end-to-end
**Steps:**
1. From the repo root, run: `make ci`.
2. Observe each step: proto check, web build, Go build, tests, lint.
**Expected:** All steps pass with exit code 0. No generated proto files are stale (proto check passes).
**Covers:** AC-40

---

## Test Infrastructure Requirements

### What Needs Building

**Fake git repo factory** (`testutil/gitrepo/gitrepo.go`)
A helper function `NewTestRepo(t *testing.T) *TestRepo` that:
- Creates a temp dir with `git init`.
- Configures `user.email` and `user.name` so commits work.
- Provides `MakeCommit(message, files)`, `CreateWorktree(branch)`, `AddUntrackedFile(name)` helpers.
- Calls `t.Cleanup(os.RemoveAll)`.
Used by: IT-001 through IT-017.

**Mock executor** (`session/unfinished/testutil_test.go`)
A `mockExecutor` implementing `executor.Executor` that:
- Stores expected command → output/error mappings.
- Fails the test if an unexpected command is called.
Used by: UT-005 through UT-019, UT-026.

**Mock ticker** (inline in scanner_test.go)
A manually-advanceable ticker channel replacing `time.NewTicker`. Lets tests control the 30s scan schedule without real waits.
Used by: UT-015.

**Mock ConnectRPC client** (Jest mock — `__mocks__/connectrpc.ts`)
A Jest module mock for the generated ConnectRPC client. Provides `mockListUnfinishedWork`, `mockWatchUnfinishedWork`, etc. as Jest spy functions that frontend tests can configure.
Used by: FT-001 through FT-025.

**Test wrapper for hooks** (`web-app/src/test-utils/unfinished-wrapper.tsx`)
A React wrapper that provides a mocked `useUnfinishedWork` hook via React context, mirroring the pattern in `web-app/src/lib/contexts/__tests__/NotificationContext.test.tsx`.
Used by: FT-010 through FT-025.

### What Needs Stubbing

| Dependency | Test Layer | Stub Strategy |
|---|---|---|
| `git` CLI subprocess | Unit | `mockExecutor` captures commands; returns fixture strings |
| `git` CLI subprocess | Integration | Real git processes against temp repos |
| `claude` CLI subprocess (AI summary) | Unit (UT-025, UT-026) | `mockExecutor` returns fixture summary text |
| `claude` CLI subprocess | Manual only | Real Claude CLI required |
| `fsnotify.Watcher` | Unit (UT-017) | Real fsnotify against temp dirs (acceptable; fast) |
| `time.Now` / ticker | Unit (UT-015) | Inject clock/ticker interface; use mock in tests |
| ConnectRPC transport | Frontend | Jest mock of the generated TS client |
| `useUnfinishedWork` hook | Frontend | `jest.mock` in each component test file |

### Test Fixture Files

`session/git/testdata/worktree_porcelain_normal.txt` — sample `git worktree list --porcelain` output with one normal worktree.
`session/git/testdata/worktree_porcelain_bare.txt` — output containing a bare worktree.
`session/git/testdata/worktree_porcelain_detached.txt` — output containing a detached HEAD.
`session/git/testdata/worktree_porcelain_locked.txt` — output containing a locked worktree.
`session/git/testdata/worktree_porcelain_prunable.txt` — output containing a prunable worktree.
`session/git/testdata/worktree_porcelain_mixed.txt` — output with all five types in one response.
`session/unfinished/testdata/diff_shortstat_sample.txt` — sample `git diff --shortstat HEAD` output.

---

## Edge Case Coverage

The following edge cases from `research/pitfalls.md` are mapped to test cases.

| Edge Case | Risk Level | Test Cases |
|---|---|---|
| `git status` hangs on network-mounted filesystem | High | UT-010 (timeout → ScanResultStatusTimeout); IT-009 uses 60s timeout on push |
| `.git/index.lock` held by another process | High | UT-010 (treated as any DeadlineExceeded) |
| 3 consecutive timeouts → circuit breaker backoff | High | UT-011 |
| Permission error walking watch dir subdirectory | Medium | UT-020, IT-017 |
| Bare repo in scan path | Medium | UT-001 (parse), UT-017 (skip in walker) |
| Detached HEAD worktree | Medium | UT-002 (parse), scanWorktree returns empty result |
| Prunable worktree (path deleted) | Medium | UT-003 (parse); scanner skips IsPrunable |
| Locked worktree | Low | UT-004 (parse); scanner skips IsLocked |
| No `main` branch — falls back to `master` | Medium | UT-014 (sub-case B) |
| No default branch found at all | Medium | UT-014 (sub-case C); ahead/behind chips omitted |
| Worktree deleted after snooze; stale path in state | High | UT-023 (startup cleanup removes stale entries) |
| Dismiss/snooze key stable across worktree re-creation | High | UT-021 (key is repoPath+branch, not worktreePath) |
| Dismiss key is `(repoPath, branchName)` not `worktreePath` | High | UT-021 |
| AI summary subprocess rate limiting (semaphore = 2) | Medium | UT-025 (concurrent calls → only one subprocess; semaphore verified) |
| AI summary subprocess timeout (30s) | Medium | UT-025 (timeout sub-case) |
| Same diff re-requested after cache hit | Medium | UT-025 (sub-case C), FT-025 (sub-case C) |
| Empty watch dir (no git repos found) | Low | UT-017 (no repos at any depth → empty scan set, no error) |
| fsnotify fd exhaustion → fallback to polling | Low | UT-017 (test with nil watcher uses polling mode); MS-003 covers happy path |
| ThundeRing herd: multiple fsnotify events for same repo | Medium | UT-029 (deduplication within TTL window) |
| Atomic write: temp file present, rename not completed | Medium | UT-024 |
| 500 worktrees: TTL cache prevents unbounded subprocess calls | Medium | UT-012 (TTL logic), UT-029 (deduplication) |

---

## Execution Order

For a full CI run, execute in this order:

### 1. Proto Generation (prerequisite)
```
make generate-proto
```
Verifies: generated Go and TypeScript files are up to date. Blocks all other steps if it fails.

### 2. Go Unit Tests (fast, no external deps)
```
go test ./session/git/... ./session/unfinished/...
```
Tests: UT-001 through UT-029.
Duration: ~5 seconds.
Dependencies: none beyond Go toolchain.

### 3. Go Integration Tests (require temp git repos)
```
go test ./server/services/... -run TestUnfinished -timeout 120s
```
Tests: IT-001 through IT-018.
Duration: ~30–60 seconds (real git subprocess calls + fsnotify).
Dependencies: `git` binary on PATH, writable `/tmp`.

### 4. Frontend Unit Tests (require built components and Jest config)
```
cd web-app && npx jest --testPathPattern="unfinished|UnfinishedNav|CommitPush|UnfinishedSource" --no-coverage
```
Tests: FT-001 through FT-025.
Duration: ~20 seconds.
Dependencies: Node.js, npm dependencies installed, compiled vanilla-extract (via `npm run build` or `jest transform`).

### 5. Full CI Validation
```
make ci
```
Covers: proto check, web build, Go build, all Go tests, golangci-lint.
Duration: ~3–5 minutes.

### 6. Manual Scenarios (pre-ship checklist)
Executed once before merging. Not part of automated CI.

| Scenario | When |
|---|---|
| MS-001: View Files opens correct browser | Before merging Epic 7 |
| MS-002: AI Summary generates coherent text | Before merging Epic 8 |
| MS-003: fsnotify triggers re-scan in <2s | Before merging Epic 2 |
| MS-004: No `.module.css` files added | Before merging Epic 6/7/8/9 |
| MS-005: `make ci` passes end-to-end | Before every PR merge |
