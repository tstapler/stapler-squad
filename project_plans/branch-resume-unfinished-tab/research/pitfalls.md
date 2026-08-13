# Pitfalls & Risks: Branch Resume Feature in Unfinished Work Tab

## Feature Overview
Surface dormant local git branches (ahead of main, no active session) as rich cards in the Unfinished Work tab, with a Resume button that pre-fills the omnibar with branch path. This requires extending the scanner to enumerate and scan branches instead of only worktrees.

## Critical Pitfalls

### 1. **Scanner Concurrency Bottleneck: Mutex Contention on go-git Repository**

**Risk Level**: HIGH

**Problem**:
The current scanner processes worktrees (which are physically checked-out directories). Each worktree already maps to a distinct path on disk. Adding branch enumeration changes the model: branches are logical Git concepts, not filesystem directories.

The `gogit_vcs_reader.go` has a **per-repo mutex** (`cachedRepo.mu` at line 51) that serializes all VCS operations on that repository:
```go
type cachedRepo struct {
    repo         *git.Repository
    mu           sync.Mutex  // ← ALL operations on this repo wait here
    accessedAtNs int64
}
```

**When branch scanning is added**:
- Current: 4 workers × n repos × m worktrees per repo (loose parallelism within a repo)
- With branches: 4 workers × n repos × (m worktrees + b branches) per repo (same 4 workers, 10x+ more tasks)
- Each branch enumeration (`ListBranches` + `ResolveDefaultBranch` + `AheadBehind` on each branch) must acquire this per-repo mutex
- A single repo with 50 branches + 5 worktrees = 55 tasks competing for one mutex
- Scanner tick (30s) now has higher latency due to queued branch work starving worktree scans

**Impact**:
- If a repo has 50+ local branches, the scanner's 4 workers (line 206) will serialize on the per-repo mutex, causing UI lag or scan timeouts
- Worktree scans for other repos block waiting for the branch-heavy repo to finish
- Profile hotspot shifts from packfile I/O to mutex contention

**Design Safeguards**:
- Add branch enumeration only if branch count < 50 (configurable threshold)
- Use separate scanner tier for branches (e.g., spawn an async branch-scan task, not inline in the worker loop)
- Consider lazy enumeration: fetch branches on-demand when a repo is first opened, cache the list, invalidate only on head changes
- Monitor `repoCacheSize` and `repoCacheTTL` carefully; branch scans could cause cache eviction churn

---

### 2. **RepoCache Memory Growth & Eviction Storm with Branch Enumeration**

**Risk Level**: HIGH

**Problem**:
The `repoCache` (line 59 in `gogit_vcs_reader.go`) has these constraints:
```go
const repoCacheMaxEntries = 100           // max entries before eviction
const repoCacheTTL = 30 * time.Minute     // evict unaccessed entries older than this
```

Each branch scan on a repo increments the "accessed" timestamp on the cached repo. **If 50 branches in one repo are scanned concurrently**, the cache entry is repeatedly accessed, but no new entries are created.

**However**, if the feature spawns many parallel branch fetches or if `reachableSet()` (line 629) constructs large in-memory commit graphs for many branches, the scanner could trigger:
1. `pruneRepoCache()` on every cache-miss (line 577)
2. A full Range iteration (line 71–80) to find old entries
3. Repeated evictions and re-opens of the same repository (expensive `git.PlainOpenWithOptions`)

**Current worktree model**: repoCache size ~= number of distinct repos (typically 5–20 in a workspace).
**With branch model**: repoCache could hold 50+ "branch scans" worth of repo references if not carefully managed.

**Impact**:
- If `repoCacheSize` reaches 100 and a new repo is scanned, `pruneRepoCache()` runs a full Range scan (~10–50 ms)
- The 30-minute TTL means cold repos (last branch scan 20 min ago) stay in cache, competing with hot repos
- Branches on a repo that was last modified 25 minutes ago might expire while branches on a different repo are still being enumerated

**Design Safeguards**:
- Cap branch enumeration per-repo: do not scan >50 branches per repo, ever
- Separate cache tier for branch work: use a time-bounded task (5-minute window) to enumerate branches, then evict immediately
- Consider shared cache key: store all branches from one repo under a single key, not per-branch
- Add cache metrics: log `repoCacheSize` on every scan to detect growth anomalies

---

### 3. **Default Branch Detection Timing & Ahead/Behind Count Reliability**

**Risk Level**: MEDIUM-HIGH

**Problem**:
In `scanRepo()` (line 302), the default branch is resolved **once per repo per scan**:
```go
defaultBranch := s.reader.ResolveDefaultBranch(repoPath)
```

`ResolveDefaultBranch()` (line 160 in `gogit_vcs_reader.go`) checks:
1. `refs/remotes/origin/HEAD` (symbolic ref, slowest)
2. Hardcoded list: `origin/main`, `origin/master`, etc.
3. Falls back to local refs

**When branch scanning is added**:
- If `origin/HEAD` is stale or absent, fallback to `origin/main` is used
- A branch ahead of `origin/main` that should be compared to `origin/develop` (the actual default) shows misleading ahead/behind counts
- If the scanner runs while a repo has no remotes (fresh clone, no `git fetch`), default branch is "" (empty) — all branches appear neither ahead nor behind

**Compounded Risk**:
- The UI shows a branch with "5 commits ahead" relative to "" (empty default)
- A Resume button pre-fills the omnibar with the branch path, but the context about which branch to compare against is lost
- User resumes the branch session and later merges against the wrong default branch

**Impact**:
- Cards display misleading ahead/behind counts if remotes are out of sync
- Ahead/behind count becomes stale if remotes are fetched during the scan window (scan started before fetch, reads old remotes)

**Design Safeguards**:
- Call `ResolveDefaultBranch()` **before** enqueuing any branch tasks, cache it per-repo
- Re-resolve default branch if it changes mid-scan (unlikely, but guard against it)
- Mark cards with "?" if default branch is empty or unresolved
- On Resume, pass the resolved default branch to the omnibar so context is preserved (proto field: `defaultBranchUsedForComparison`)

---

### 4. **Omnibar Dedup & Session Creation with Non-Worktree Branch Paths**

**Risk Level**: MEDIUM

**Problem**:
The omnibar uses a dedup system: when you click "Resume" on a branch card, it calls `createSession({ path: "/repo", branch: "feature-x" })`.

The backend (`session_service.go`, line 302) calls `FindGitRepoRootSimple()` (line 608) to locate the repo root, then creates a session at that root, checking out the branch.

**But the branch is not a worktree yet** — it's a local branch ref. The session creation flow expects:
- `sessionType: "new_worktree"` → create a **new** worktree for the branch
- Path is the repo root, not a worktree path

**Current code flow**:
1. `CreateSession` with `sessionType=NEW_WORKTREE, path="/repo", branch="feature-x"`
2. Creates a new worktree at `/repo/.git/worktrees/feature-x/`
3. Session.Path points to the worktree dir, not the repo root

**When omnibar Resume is called**:
- Omnibar sends `path: "/repo", branch: "feature-x", sessionType: "new_worktree"`
- Backend sees "new_worktree" → `git worktree add /repo/.git/worktrees/feature-x /repo/feature-x` 
- ✅ Works fine if branch exists locally and is clean

**Edge Cases**:
1. **Branch was deleted locally** between scan and Resume → `git worktree add ... feature-x` fails, session creation fails, user sees spinner
2. **Branch diverged from remote** → `git worktree add` creates the worktree, but it's tracking a stale local ref, not origin/feature-x
3. **User has multiple repos** → omnibar pre-fill only has path (repo root), no repo name or identity. If two repos have the same branch name, ambiguous Resume action

**Impact**:
- Resume button can fail silently if branch is deleted between scan and click
- Session UI shows "Failed to create session" with no guidance
- User may think the branch is gone when it's actually a race condition

**Design Safeguards**:
- Store `repoPath` **separately** on branch cards (not just derived from `path`)
- On Resume, validate that branch still exists: `git rev-parse --verify feature-x` before calling createSession
- If validation fails, show a toast: "Branch was deleted or moved"
- Pass `repoPath` and `branch` as explicit fields to omnibar, not encoded in `path`
- Add integration test: Resume → branch deleted → handler returns graceful error

---

### 5. **Scan Timeout & Error Handling Under Branch Load**

**Risk Level**: MEDIUM

**Problem**:
The scanner has a circuit breaker (lines 641–680) that backs off after 3 consecutive timeouts:
```go
if b.consecutiveTimeouts >= 3 {
    b.backoffUntil = time.Now().Add(5 * time.Minute)
}
```

When `ListWorktrees()` times out (line 305), it's recorded:
```go
s.recordTimeout(repoPath)
```

**With branch enumeration**, a new codepath emerges:
- If `ListBranches()` times out, which timeout counter increments? `repoPath` or `repoPath|branch`?
- If a repo has 100 branches and each one times out, do we record 100 separate timeouts (one per branch) or one per repo?
- Current code groups timeouts by repo, so 100 branch timeouts = 100 recorded, circuit breaker trips immediately

**Impact**:
- A repo with many slow branches causes the circuit breaker to trip prematurely
- The repo is excluded from scanning for 5 minutes, even if worktree scans would succeed
- User sees "no unfinished work" for 5 minutes, then work reappears

**Design Safeguards**:
- Use a separate timeout counter for branches vs. worktrees: `timeoutCounterForBranches`, `timeoutCounterForWorktrees`
- Cap branch work at the task level: if ListBranches takes >5 seconds, skip it for this cycle and log a warning
- Add a config option to disable branch scanning if `repoTimeoutCount > 2` (avoid thrashing on slow repos)

---

### 6. **LCS Diff Algorithm Scaling on Large Branches**

**Risk Level**: LOW-MEDIUM

**Problem**:
`DiffShortstat()` (line 412) uses `LinesDiff()` (line 487) which computes the longest common subsequence (LCS) in O(n*m) time and O(min(n,m)) space.

**Current use**: diff between HEAD and working tree of a single worktree (usually <1000 lines changed).
**With branches**: if we add diffstat to branch cards, we might compute diff between `origin/main` and a branch HEAD, which could be:
- 10,000 line file with 5,000 changes = 50M DP operations per file
- A repo with 50 branches × 10 changed files = 500 LCS computations per scan cycle

**Current cache**: `diffStatCache` (line 33) has a 30-second TTL and no size limit.
- The cache is only keyed by `worktreePath`
- If we add branches, the cache key becomes `repoPath|branch`, and size could grow unbounded

**Impact**:
- Scanner CPU spikes when scanning many branches with large diffs
- Memory allocates for LCS DP tables (see line 504: `prev := make([]int, len(b)+1)`)
- Cache may grow to thousands of entries if many branches are scanned

**Design Safeguards**:
- Do **NOT** compute diffstat for dormant branches (it's expensive and rarely needed for display)
- If diffstat is needed for branch cards, reuse the cached worktree value for checked-out branches
- Cap LCS computation: skip diffstat if either file count or line count exceeds threshold (e.g., skip if >5000 lines)
- Bound diffStatCache size: keep only the 200 most-recently-used entries

---

### 7. **Integration Test Coverage Gaps for Branch Enumeration**

**Risk Level**: MEDIUM

**Problem**:
Current test coverage in `scanner_test.go` (247 lines) tests:
- `ParseAllWorktrees()` with normal, bare, detached, prunable, locked worktrees
- `ScanResult.IsUnfinished()` logic
- No tests for `ListBranches()`, `AheadBehind()` on dormant branches
- No concurrency/race tests for many branches in one repo
- No tests for default branch resolution when remotes are stale

`unfinished_work_test.go` (100+ lines) tests:
- Config get/set
- GetWorktreeAISummary with caching
- No e2e tests for branch Resume flow

**When branch feature is added**:
- New code path: `ListBranches(repoPath)` must be tested with 0, 1, 10, 100, 1000 branches
- Race condition: concurrent Resume on same branch while branch is being scanned
- Error case: `AheadBehind(branch-path, default-branch)` when default branch doesn't exist
- Integration: branch card + Resume button + omnibar pre-fill + session creation

**Impact**:
- Silent regressions: branch enumeration could scan but produce no cards (e.g., all branches dismissed)
- Flaky tests: concurrency bugs surface only under load or race detector
- Missing coverage means branch feature ships with untested error paths

**Design Safeguards**:
- Add test for ListBranches with 50+ branches (ensure no timeout, no mutex stall)
- Add race detector test: concurrently Resume + delete branch
- Add integration test in e2e: `branch-resume.spec.ts` covering full flow
- Extend `scanner_test.go` with `BenchmarkListBranches` similar to `vcsreader_bench_test.go`

---

## Mitigation Summary

| Pitfall | Mitigation Strategy | Priority |
|---------|-------------------|----------|
| **Mutex Contention** | Lazy enumeration + branch scan tier | HIGH |
| **Cache Memory Growth** | Capped branch count + separate cache tier | HIGH |
| **Default Branch Staleness** | Cache default branch before branch enumeration, mark uncertain cards | HIGH |
| **Omnibar Session Create Failures** | Validate branch exists before Resume, explicit repoPath/branch fields | MEDIUM-HIGH |
| **Timeout Storm** | Separate timeout counters for branches vs. worktrees | MEDIUM |
| **LCS Perf Regression** | Don't compute diffstat for branches; cap LCS on large files | MEDIUM |
| **Test Gaps** | Add branch-specific tests + e2e Resume flow + race detector | MEDIUM |

