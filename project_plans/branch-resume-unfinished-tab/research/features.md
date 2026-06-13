# Feature Research: Dormant Local Branches in Unfinished Work Tab

## 1. Edge Cases in Branch Discovery

### 1.1 What the Scanner Currently Does (Existing Worktree Filtering)
The unfinished scanner's `scanRepo()` method (scanner.go:302–337) enumerates all worktrees in a repo, then **skips**:
- **Bare repos** (`wt.IsBare`) — repos with no working tree
- **Detached HEAD worktrees** (`wt.IsDetached`) — orphaned branches not on any named ref
- **Prunable worktrees** (`wt.IsPrunable`) — marked for cleanup but still on disk
- **Worktrees with no branch** (`wt.Branch == ""`) — in detached state or not properly set up

The scanner then calls `scanWorktree()` for each remaining worktree and compares it against `defaultBranch` (resolved once per repo via `ResolveDefaultBranch()`).

### 1.2 Candidate Cases for the New Feature (Local Branches Without Worktrees)
To surface **dormant local branches ahead of main**, we need to discover branches that:
1. **Exist on disk** as local refs in `.git/refs/heads/`
2. **Have no worktree** — `git worktree list --porcelain` does not mention them
3. **Are ahead of the default branch** — commits not yet merged to main/master
4. **Are not detached** — bound to a named branch ref

**Edge cases and filtering questions:**

#### Detached HEAD branches
- **Detached HEAD** in the main worktree (`IsDetached=true`) should **not** be surfaced; these are orphaned
- **Orphaned detached refs** left behind by deleted worktrees should **not** appear (they're dangling and likely stale)
- **Decision:** Skip any branch in detached state; only surface named refs

#### Branches pointing to the same commit as main
- **Edge case:** A branch `feature-x` and `main` both point to the same commit (e.g., cleanup after merge but before branch deletion)
- **Current behavior:** `AheadCount` would be 0; scanner already skips worktrees with no unfinished work (scanner.go:329)
- **Decision:** Apply the same logic — if `AheadCount == 0 && !HasUncommitted && BehindCount == 0`, don't show it

#### Branches from other remotes (e.g., `origin/feature-x` tracked locally via `git fetch`)
- **Edge case:** User runs `git fetch origin` which creates tracking branches; local branches can also exist
- **Question:** Should we include only truly local branches (`refs/heads/*`) or also locally-tracked remote branches (`refs/remotes/origin/*`)?
- **Discovery method:** `git branch -a` lists all; `git branch` lists only local
- **Decision:** Surface only **local branches** (`refs/heads/*`); remote tracking branches are not candidates for local work resumption
- **Rationale:** Remote tracking branches represent the state of remote refs and are not places to resume local work; if a user wants to resume work on a remote branch, they create a local tracking branch first

#### Merge branches and stale branches
- **Edge case:** User creates a branch, then it gets merged and deleted from origin, but the local ref persists
- **Detection:** Check if the branch's upstream is gone (`git rev-parse @{u}` fails) or if the branch is no longer on origin
- **Decision:** Include these — they're unfinished local work that hasn't been cleaned up. The UI can show "merged to main but not yet deleted" as metadata

#### Branches with `.` or unusual characters
- **Edge case:** Windows/macOS may have case-sensitive filesystem issues; refs can have special chars
- **Current handling:** `gogit_vcs_reader.go` reads refs from disk via `os.ReadDir()` — standard filesystem semantics apply
- **Decision:** No special filtering; git handles ref naming; pass through as-is

### 1.3 Proposed Edge Case Handling
```
For each local branch (refs/heads/*):
  IF branch is detached or IsPrunable SKIP
  IF branch points to same commit as defaultBranch AND no uncommitted work SKIP
  IF branch is locked (indicates active worktree operation) WARN
  IF branch is unreachable from any worktree SKIP
  ELSE: scan it as a candidate for dormant-branch card
```

---

## 2. Defining "No Active Session" — Precisely

### 2.1 Current Session Lifecycle (from instance.go)
Sessions have three statuses:
- **Active** — the instance has a live AI process (running or ready)
- **Paused** — the instance is paused (worktree intact, branch preserved, process not running)
- **Stopped** — terminal state; instance shut down and cannot transition further

A session's `Path` field points to a directory (worktree or regular dir); `GitManager.HasWorktree()` indicates if the session has git context.

### 2.2 Matching Sessions to Worktrees
The unfinished service's `sessionPathIndex()` (unfinished_work_service.go:56–71) builds a `worktreePath → []sessionUUID` map by loading all stored instances and matching on `inst.Path`.

**Key insight:** A session is "active" if it's in the storage and has `Status == Active`. A worktree is "associated with a session" if that session's `Path` equals the worktree's path, **regardless of session status**.

### 2.3 Definition for "No Active Session" on a Dormant Branch
For a local branch with no worktree on disk:
- **No session at all** — the branch has never had a worktree created; this is the common case
- **Session exists but is Paused** — a previous session was created, then paused (worktree still on disk, or was cleaned up)
- **Session exists but is Stopped** — the session was shut down; the worktree may or may not exist

**Proposed definition:** A dormant branch is eligible for the Resume button if:
- No **Active** sessions target this branch, **AND**
- Either (a) no worktree exists on disk for this branch, **OR** (b) the worktree exists but is not tracked by any session

**Note:** This is conservative — if a Paused or Stopped session's worktree still exists, we show the branch as dormant but the Resume button does not re-create the worktree (it uses the existing one or creates a new one based on user action via the omnibar).

---

## 3. Existing Worktree on Disk (Previous Orphaned Session)

### 3.1 The Scenario
User creates a session on a branch → creates a worktree `/home/user/.stapler-squad/worktrees/my-branch` → session becomes Active → Later, user stops the session or deletes the record without cleaning up the worktree disk state → The worktree is now orphaned

### 3.2 Current Code Paths
- **Unfinished scanner sees the orphaned worktree** — it shows up in `git worktree list --porcelain` and gets scanned (scanner.go:303–312), assigned to a ScanResult, and published if unfinished
- **Unfinished service's sessionPathIndex** — does not find a session record because it was deleted, so `SessionIDs` is empty
- **UI rendering** — the worktree appears in the Unfinished list with an empty session array; the "has active session" badge is absent

### 3.3 Resume Flow for Orphaned Worktree
When the user clicks Resume on an unfinished worktree card that has no session:
1. The omnibar is pre-filled with the worktree's `WorktreePath` (e.g., `/home/user/.stapler-squad/worktrees/my-branch`)
2. The omnibar detects it's an existing directory and routes to `create_session` with `sessionType: "existing_worktree"`
3. The session service calls `git.NewGitWorktreeFromExisting(worktreeDir)` which validates the worktree and re-registers the session

### 3.4 Implementation Detail
The omnibar can pass `sessionType: "existing_worktree"` when the path is an existing worktree (detected via `git rev-parse --git-dir` or similar). See `.claude/rules/session-creation-registry.md` for the 7 touchpoints needed.

**Decision:** The new feature can reuse the existing session creation flow — no special handling needed. The Resume button simply opens the omnibar with the worktree path and lets the omnibar's path-completion logic detect that it's an existing worktree.

---

## 4. Handling Scan Errors (Large Repos, Timeouts)

### 4.1 Current Error Handling in Scanner
The scanner has a **circuit breaker pattern** (scanner.go:639–680):
- After 3 consecutive `AheadBehind` or `HasUncommitted` timeouts, a repo enters 5-minute backoff
- A ScanResult's `Status` field captures the outcome: `ScanResultStatusOK`, `ScanResultStatusTimeout`, `ScanResultStatusError`, or `ScanResultStatusPermission`
- TimeoutResults are still published and displayed in the UI (UnfinishedItem shows an ⚠ Timeout chip)

### 4.2 For Dormant Branches (No Worktree)
When scanning a local branch (without a worktree on disk), we can't run `git status` or `git diff HEAD`. Instead, we can use **cheaper operations**:
- **List refs and resolve HEAD:** `git rev-parse <branch>` — O(1), always fast
- **Compare commits:** `git rev-list --count <branch>..<defaultBranch>` — O(commits), but typically cached in packfile
- **Get commit messages:** `git log -1 --oneline <branch>` — O(1) with packfile cache
- **Detect file changes:** We can't compute `diffstat` without a worktree; fallback to "unknown"

### 4.3 Proposed Error Strategy for Dormant Branch Cards
If a branch scan times out:
1. Show the card with `Status: TIMEOUT` and an error badge
2. Display only cached metadata (commits ahead, branch age)
3. Hide the "Files Changed" section (requires a worktree to compute)
4. Resume button still works — opens omnibar with the branch path

**Decision:** Dormant branch scanning is simpler than worktree scanning (no file operations), so timeouts should be rare. If they occur, gracefully degrade to showing structural metadata only.

---

## 5. UI Placement: Separate Section or Interleaved?

### 5.1 Industry Precedent
- **GitHub:** Branches appear in a separate "Branches" tab, not mixed with pull request reviews or commits
- **VS Code:** Git branches are in the "Source Control" sidebar; they don't appear in the file explorer or other views
- **Tower (Git GUI):** Branches in a dedicated panel; uncommitted changes in another panel

### 5.2 Stapler-Squad Current Layout (Unfinished Tab)
The unfinished work tab shows worktrees with uncommitted changes, commits ahead, etc. — all scanned worktrees grouped by repo.

### 5.3 User Expectation and Discoverability
Dormant branches are **harder to remember** than "work in progress" on open terminals. Mixing them with worktrees could:
- **Clutter the list** — a repo with 20 stale branches would dominate the UI
- **Confuse the user** — "why is this branch here if I wasn't working on it?"
- **Aid discoverability** — the user sees old branches they forgot about and can clean them up

### 5.4 Proposed UX Design
**Separate collapsible section: "Dormant Branches" below active worktrees**
- Heading: `Dormant Branches (N)` — collapsible to reduce visual clutter
- Each branch card shows: branch name, commits ahead, recent commit messages, last commit date
- No "files changed" for branches without worktrees (or marked "unknown")
- Resume button opens omnibar with the repo's main working directory (not the branch path directly), allowing the omnibar to route to `sessionType: "new_worktree"` on the branch
- Dismiss and Snooze actions available (same as worktrees)

**Rationale:** Separating dormant branches from active worktrees respects the cognitive distinction (one is active, the other is dormant) while keeping them discoverable in the same tab.

---

## 6. Unstated User Needs and Feature Completeness

### 6.1 Bulk Operations
- **Bulk delete stale branches:** User selects multiple branches and runs `git branch -D` in batch
- **Current code:** Unfinished service has `DismissWorktree()` (hides a worktree); no bulk API yet
- **Decision:** Out of scope for MVP; add as future enhancement. Dismissal alone (hide) provides the user with a way to declutter the list

### 6.2 Sorting and Filtering
- **Sort by:** Last commit date (newest first), commits ahead (most work first), branch name
- **Filter by:** Repo, age (branches older than X days), commits ahead (branches with >10 commits)
- **Current code:** Scanner sorts by `LastModified` via `SortByLastModified()`; frontend can further sort
- **Decision:** MVP shows dormant branches sorted by recency (same as worktrees). Filters can be added later

### 6.3 Integration with Cleanup Workflows
- **User pain:** "I should clean up my old branches, but I don't want to forget the work"
- **Proposed integration:** Add an action "Archive to Gist" or "Create Issue" to save the branch metadata before deletion
- **Decision:** Out of scope for MVP; the card metadata (commits, messages, dates) serves as a memory aid

### 6.4 Branch Metadata and Context
- **Show branch creator** — which user (in multi-user repos) created the branch
- **Show origin branch** — if the branch was created to track a remote ref, which one?
- **Show merge status** — is this branch fully merged to main, or does it have unique commits?
- **Decision:** MVP shows: branch name, commits ahead, recent messages, last commit date. Merge status (commits ahead of main) is the primary indicator. Creator and origin can be added as badge/metadata in future iterations

### 6.5 Proactive Suggestions
- **"This branch is fully merged to main, safe to delete"** — could run a merge check and show a "Safe to Delete" badge
- **"This branch has merge conflicts with main"** — run a `git merge-base` check and warn
- **Decision:** Out of scope for MVP. The card itself indicates "commits ahead" which the user can infer means "not merged"; automated suggestions can be added once user feedback on the basic feature arrives

---

## 7. Implementation Constraints and Decisions

### 7.1 Backend Changes Required
1. **New VCSReader method:** `ListLocalBranches(repoPath string) ([]BranchInfo, error)` where BranchInfo includes branch name, HEAD SHA, last commit date
2. **Branch scanning:** In `Scanner.scanRepo()`, after scanning worktrees, enumerate local branches not yet mentioned
3. **ScanResult enrichment:** Add a field `IsBranch bool` to distinguish from worktrees; set `WorktreePath = ""` for branches
4. **Filtering:** Skip branches with 0 commits ahead of main (clean branches)

### 7.2 Frontend Changes Required
1. **UnfinishedItem component:** Render differently if `IsBranch == true` (hide "files changed" section, show branch icon)
2. **New section:** "Dormant Branches" collapsible group below active worktrees
3. **Resume button:** Opens omnibar with repo's main path, not branch path; omnibar routes to session creation
4. **Tests:** E2E test creating a local branch ahead of main, verifying it appears, dismissing it

### 7.3 Proto Changes Required
1. **UnfinishedWorktree message:** Add optional field `is_branch: bool` (default false)
2. **ScanResult proto:** Distinguish worktree vs. branch in the response

### 7.4 No Breaking Changes
- Existing worktree cards show as before
- New branch cards appear below worktrees
- Dismiss/snooze logic reuses existing state store (keyed by repoPath|branch, which works for both)

---

## Summary of Key Decisions

| Decision | Rationale |
|---|---|
| **Skip detached/orphaned branches** | Detached refs are not resumable work; they're dangling |
| **Only surface local branches, not remote tracking** | Remote refs are not places to resume work; users create local tracking branches explicitly |
| **"No active session" = no Active status session targets this branch** | Conservative; allows Paused/Stopped sessions to exist without blocking the card |
| **Reuse existing Resume-via-omnibar flow** | Omnibar's path detection handles `existing_worktree` vs. `new_worktree` automatically |
| **Separate UI section for dormant branches** | Respects cognitive distinction; maintains discoverability in the same tab |
| **MVP shows commits ahead, recent messages, last date** | Sufficient metadata for user to recall the work; filters/merge status can be added later |
| **Graceful timeout handling** | Dormant branch scans are cheap (no worktree needed); errors are rare but show status badge |
| **Dismiss available for branches** | User can declutter if a branch is indeed stale and won't resume it |

