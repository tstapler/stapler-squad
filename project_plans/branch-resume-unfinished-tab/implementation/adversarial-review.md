# Adversarial Review: branch-resume-unfinished-tab Implementation Plan

**Date**: 2026-05-31
**Reviewer**: adversarial architecture review
**Verdict**: CONCERNS (0 blockers / 7 concerns / 5 minors)

---

## Summary

The plan is structurally sound and follows existing patterns. No single issue is a hard blocker, but several concerns could silently produce incorrect behavior or significantly increase implementation complexity. The biggest risks are: key collision between the existing `resultStore` and the proposed `branchResultStore` (shared keyspace), a poorly-specified "skip branches checked out in a worktree" filter that relies on a wrong data source, unspecified dismiss/snooze semantics for branches, and a 5-minute scan cadence that is too slow to match the stated UX goal of "no git commands required."

---

## CONCERNS

### C-1: `resultStore` and `branchResultStore` share the same key format — collision risk

**Where**: Phase 2, Task 2.1.1a — `branchResultStore sync.Map` with key implied as `repoPath+"|"+branchName`.

**Problem**: The existing `resultStore` already uses `repoPath+"|"+branch` as its key (scanner.go line 445). A dormant branch (e.g. `~/proj|feature-auth`) could collide with an active worktree on the same branch if both stores are queried by the same key. More critically, the `GetAllResults()` method and `GetResultByKey()` method only look in `resultStore`; if branch results land in a separate store, the dismiss/snooze RPCs (`DismissWorktree`, `SnoozeWorktree`) that call `scanner.RemoveResult()` will not find branch entries. The user will dismiss a branch card but it will reappear after the next scan because `RemoveResult` only deletes from `resultStore`.

**Evidence**: `scanner.go:526` — `RemoveResult` deletes from `s.resultStore`, not from any `branchResultStore`. `state.go:122` — `IsDismissed(repoPath, branch)` is reused, but `publishBranchResults` task (2.2.1c) is described as "dismissal check" without specifying which store or which RPCs service branches.

**Required fix**: Either reuse `resultStore` (marking entries as branch-type via a new `IsDormantBranch bool` field on `ScanResult`), or define and expose new dismiss/snooze RPCs specifically for dormant branches and update `DismissWorktree` to check both stores. The plan mentions `publishBranchResults — dismissal check` but does not specify the mechanism.

---

### C-2: "Skip branches checked out in active session worktrees" filter is specified against the wrong data source

**Where**: Phase 2, Task 2.2.1a — `scanRepoBranches() — skips branches in resultStore (already a worktree)`.

**Problem**: `resultStore` contains worktrees that have *unfinished work* (uncommitted changes, ahead count > 0). A branch can be checked out in a clean worktree (no unfinished work) and therefore absent from `resultStore`, causing it to appear as "dormant" when it is actively checked out. The filter must compare against the live worktree list, not the unfinished-work result store.

**Evidence**: `scanner.go:329` — `scanRepo` explicitly skips clean worktrees: `if result.Status == ScanResultStatusOK && !result.IsUnfinished() { continue }`. So `resultStore` only holds dirty worktrees.

**Required fix**: `scanRepoBranches` must call `reader.ListWorktrees(repoPath)` to get the authoritative checked-out branch list, then exclude those branches from dormant candidates. This is an additional VCS call per repo scan cycle (already happens in `scanRepo`, so it may be shareable if both paths are colocated).

---

### C-3: The `per-repo mutex` in `GoGitVCSReader` serializes all VCS reads for a repo — adding `ListLocalBranches` (which walks all branch refs) blocks existing worktree scans

**Where**: Phase 1, Tasks 1.1.1a–1.1.1b — `ListLocalBranches` on `GoGitVCSReader`.

**Problem**: Every go-git operation acquires `entry.mu` (the per-repo mutex, `gogit_vcs_reader.go:44`). A `ListLocalBranches` call that walks all refs and calls `countCommitsTo` for each branch (up to 50 branches × O(divergence) BFS) holds the lock for potentially hundreds of milliseconds. During that time, any concurrent `HasUncommitted` or `AheadBehind` call for the same repo's worktrees is blocked. The plan's 2 branch workers run concurrently with the existing 4 scan workers — they will contend on the same per-repo mutex.

**Evidence**: `gogit_vcs_reader.go:204–211` — `HasUncommitted` acquires `entry.mu.Lock(); defer entry.mu.Unlock()`. `AheadBehind` does the same (line 329). `countCommitsTo` is called inside this lock.

**Required fix**: Either (a) perform `ListLocalBranches` outside the per-repo mutex using a read-only reference iterator (go-git's `repo.References()` is concurrent-safe for read), or (b) schedule branch scans only when no worktree scan is in flight for the same repo (coordination overhead). Option (a) is strongly preferred.

---

### C-4: 5-minute branch scan cadence creates stale state that undermines the "no git commands" UX promise

**Where**: Phase 2, Tasks 2.1.1a (`branchTickInterval=5min`) and Phase 2 overall.

**Problem**: The requirements state "A user can see all dormant local branches… without running any git commands." If a developer creates a branch and makes commits, the Unfinished Work tab will not show it for up to 5 minutes. The existing worktree scanner runs every 30s. The 5-minute choice is presumably to reduce load, but there is no mechanism to invalidate the branch cache when a new commit is detected (unlike the existing `InvalidateCache` path triggered by `EnqueueRepo`). The plan also proposes no `TriggerScan` integration for branch results.

**Evidence**: `scanner.go:192` — `SetTickInterval` exists for the worktree scanner. The existing `TriggerScan` (line 234) calls `enqueueAll()` which enqueues worktree scans only.

**Required fix**: Either (a) lower the cadence to match worktree scans (30s), or (b) make `TriggerScan` also trigger a branch scan so the existing "Refresh" button delivers instant branch results. Without this, the Refresh button in the UI (UnfinishedTab.tsx line 116) provides no benefit for branch cards.

---

### C-5: No `DismissWorktree`/`SnoozeWorktree` equivalent for dormant branches — missing RPC or adaptor

**Where**: Phase 3, Task 3.1.1a — `DormantBranch.is_dismissed` field proposed, but no dismiss/snooze RPC is specified for branches.

**Problem**: The existing `DismissWorktree` and `SnoozeWorktree` RPCs are keyed by `(repoPath, branch)`. The plan adds `DormantBranch` with an `is_dismissed` field, implying dismiss is needed, but no new RPCs are specified and the existing RPCs operate on `ScanResult` / `resultStore`, not on branch results. Without dismiss/snooze support, users have no way to hide stale branch cards, making the section noisy for repos with many old branches.

**Required fix**: Explicitly specify whether dormant branches reuse the existing dismiss/snooze RPCs (requires the C-1 store unification fix) or require new RPCs. The plan must also specify `StateStore` changes for persisting branch dismissals.

---

### C-6: Resume button format `repoPath@branchName` relies on `PathWithBranchDetector` — but the detector only validates paths starting with `/` or `~`, and branch names with `/` (e.g. `feature/login`) break the greedy `@` split

**Where**: Phase 6, Task 6.1.1a — `Resume button calls router.push(routes.sessionsWithOmnibar(repoPath@branchName))`.

**Problem**: `PathWithBranchDetector` (detector.ts line 164) uses pattern `/^(.+)@([a-zA-Z0-9_/.-]+)$/`. For a branch name `feature/login`, the regex captures correctly, but `isValidBranchName` rejects names containing `//` (line 202). More importantly, the detector does not require the path part to start with `/` or `~`, so `/home/user/proj@feature/login` correctly matches. However, branch names containing characters like `#`, `{`, `}`, or Unicode (valid git branch name chars) will fail `isValidBranchName` and the detector will return `null`, leaving the omnibar with dead unrecognized input. The requirements say "pre-filled with the branch's working directory path" — the plan assumes `repoPath@branchName` always resolves, but does not validate this assumption against real branch naming.

**Evidence**: `detector.ts:201–204` — `isValidBranchName` rejects several valid git branch name characters. Git allows `#`, `{`, `}`, `@{`, etc. in branch names. The plan has no fallback if detection fails.

**Required fix**: Either (a) fall back to `routes.newSessionFromWorktree(repoPath, branchName)` (which already exists and is used by `UnfinishedItemDetail.tsx:64`), or (b) extend `isValidBranchName` to cover the full git branch name specification. Option (a) is simpler and already tested.

**Note**: `routes.newSessionFromWorktree` already exists in `routes.ts:28` and handles `path@branch` via query params. The plan should use this instead of a new `routes.sessionsWithOmnibar` helper (Task 6.4.1a is likely unnecessary scope).

---

### C-7: `GoGitVCSReader.ListLocalBranches` — `countCommitsTo` is unbounded on repos without a merge base within `mergeBaseBFSLimit=2000`

**Where**: Phase 1, Task 1.1.1b — "uses existing countCommitsTo/findMergeBase helpers".

**Problem**: `findMergeBase` is bounded at `mergeBaseBFSLimit=2000` commits per side. If the merge base is not found within this limit, it returns `plumbing.ZeroHash` and an error. `ListLocalBranches` will propagate this error and either skip the branch (silently dropping it from results) or surface an error. In a monorepo with a long history where a branch diverged more than 2000 commits ago, all such branches will be silently absent from the dormant list — precisely the branches most likely to need attention. The plan does not specify error handling for this case.

**Required fix**: Explicitly specify that branches where `findMergeBase` exceeds the BFS limit are included with `CommitsAhead = -1` (unknown) and a `ScanErrorMsg`, so the user sees them rather than losing them silently.

---

## MINORS

### M-1: Two separate ticker goroutines (worktree + branch) doubles coordinator complexity for marginal benefit

The plan adds `branchTickInterval` as a separate field and a second ticker in the coordinator. The existing `enqueueAll` → `worker` pattern could be extended with a "branch scan" flag on `scanTask` or a separate queue but sharing the same coordinator `select`. A separate goroutine is a maintenance burden when a simpler modulo-N scheme (scan branches every Nth worktree tick) would work. Low priority if the separate goroutine is already the team's preference.

---

### M-2: Phase 5 frontend hook extends `useUnfinishedWork` — adding `branchMap` to a hook already managing `worktreeMap` will make it harder to test either concern in isolation

The hook currently manages one concern (worktree events). Adding `branchMap`, `branchUpdated`, `branchRemoved` creates a hook that needs to be mocked for any test touching either data type. Consider a separate `useDormantBranches` hook that can be tested independently and composed in `UnfinishedTab`.

---

### M-3: `GitVCSReader` implementation (Task 1.1.1c) uses `git for-each-ref` with a 3s timeout — no cross-check against `ListWorktrees` result

Unlike `GoGitVCSReader`, `GitVCSReader.ListLocalBranches` must independently call `ListWorktrees` (or an equivalent) to filter checked-out branches. The plan does not specify this and may result in duplicate code or incorrect filtering in the fallback reader.

---

### M-4: `DormantBranch.scan_time` field in proto (Task 3.1.1a) conflicts with existing `ScanResult.ScanTime` naming — consider consistency

The existing `UnfinishedWorktree` proto uses `scan_time` (via `scanResultToProto`). The plan proposes the same field name, which is fine, but `BranchScanResult` should also include `LastModified` (last commit time) distinct from `ScanTime` (when the scanner ran), and the plan lists both `last_commit_time` and `scan_time` — ensure the Go struct matches the proto field names to avoid conversion confusion.

---

### M-5: E2E test (Task 7.3.1a) described as "mock stream + section visible" — mocking the streaming RPC in Playwright is non-trivial

The existing e2e tests use a real test server (`http://localhost:8544`). Mocking the streaming RPC requires either intercepting at the network level (MSW) or running a test server that emits synthetic events. The plan should explicitly state which approach and whether a test fixture (pre-populated branch state) is sufficient instead of mocking.

---

## Not-Raised (Confirmed Adequate)

- The `newSessionFromWorktree` route + page.tsx query-param handling already exists and is tested — the Resume flow is mechanically straightforward once C-6 is resolved.
- The circuit breaker separation (C-2 in the plan's safeguards) is a sound design as long as it is genuinely separate from the worktree circuit breaker; the plan correctly identifies this.
- The cap at 50 branches per repo is reasonable; no concern.
- JJ stub returning `nil, nil` is consistent with how JJ repos lack local branches in the git sense.
