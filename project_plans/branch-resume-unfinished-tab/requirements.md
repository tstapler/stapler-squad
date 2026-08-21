# Requirements: branch-resume-unfinished-tab

**Date**: 2026-05-30
**Type**: feature addition

## Problem Statement

All stapler-squad users accumulate local git branches with real in-progress work that have no active session. These branches are currently invisible in the UI — the only way to rediscover them is to run `git branch` and manually cross-reference against active worktrees. The Unfinished Work tab shows file-level VCS changes in active worktrees but does not surface dormant branches at all, so work is routinely forgotten or rediscovered only when browsing raw git output.

## Users / Consumers

All stapler-squad users — anyone running stapler-squad across one or more repos who creates sessions on branches and later ends those sessions without merging.

## Success Metrics

- A user can see all dormant local branches (ahead of main, no active session) in the Unfinished Work tab without running any git commands
- Any dormant branch can be resumed in one click — clicking Resume opens the omnibar pre-filled with the branch's working directory path, ready for session creation
- Zero dormant branches require git command-line discovery after this feature ships

## Constraints

- No hard deadline
- No performance or SLA targets beyond: branch scan must not block the UI on repos with many branches
- No compliance requirements

## Scope

### In Scope

- Scan all **local** branches in every tracked repo that are:
  - Ahead of the repo's default branch (have commits not in main)
  - **Not** currently checked out in any active session worktree
- Display each dormant branch as a rich card in the Unfinished Work tab alongside existing file-level entries, showing:
  - Branch name
  - Commit count ahead of main
  - Most recent commit messages (up to 5, matching existing `AheadMessages` field)
  - Files changed (insertions/deletions) — same `DiffShortstat` data the scanner already collects
  - Last commit date / time since last activity
- **Resume button** on each card: opens the omnibar pre-filled with the repo's working directory path so the user can complete session creation (choose program, confirm, etc.)
- Cards should visually match the existing unfinished work card style for consistency

### Out of Scope

- Remote-only branches (branches that exist on origin but not locally)
- Branches already checked out in an active session worktree (those are already visible as active sessions)
- Auto-creating sessions without user confirmation
- Snooze / hide / archive individual branch cards
- Cross-repo deduplication or merging of cards

## Open Questions

- Where exactly in the Unfinished Work tab should dormant branch cards appear — interleaved with file-level entries sorted by recency, or in a distinct "Dormant Branches" section?
- Does the existing `Scanner` in `session/unfinished/` already walk all local branches, or does it only walk worktrees that are checked out? (Determines whether we extend the scanner or add a separate branch enumerator.)
- How should the scanner determine "default branch" per repo when there is no active session — fall back to `ResolveDefaultBranch()` in the VCS reader, or read from config?
- Should the Resume button open the omnibar with `sessionType: "existing_worktree"` (reuse the branch's worktree if one exists on disk) or `sessionType: "new_worktree"` (create a fresh worktree)?
- How frequently should the dormant branch scan run? The existing unfinished-work scanner has its own cadence — should dormant branches use the same interval or a slower one (branches change less often than file-level diffs)?
