# Implementation Plan: vcs-tab-redesign

**Feature**: Redesign the session-detail VCS tab from a bare branch/PR badge into a
world-class, read-only status panel — commit history, aggregate diff stats, itemized CI
checks, an all-reasons "why blocked" rollup, and reviewer feedback — by wiring
already-computed-and-discarded backend data through `VcsWidget`'s existing `full` mode,
without regressing its `compact`-mode use on backlog ship-status views.
**Date**: 2026-08-27
**Status**: Ready for implementation
**ADRs**: ADR-001 — bound and cache the new live-session go-git calls in place; do not
promote `GoGitVCSReader` (`../decisions/ADR-001-bound-and-cache-new-go-git-calls-without-promoting-gogitvcsreader.md`)

## Open Questions Resolved

1. **PR comments surfacing (requirements.md OQ1).** **In scope**, as a collapsed-by-default
   section built last (Epic 4.6), not blocking the rest of the redesign. Rationale:
   features.md confirmed `GetPRComments` (`github/client.go:556`) is already fully wired
   end-to-end with zero UI consumers — "the risk/cost calculus... is almost entirely how much
   UI space, not feasibility" — and requirements.md itself leaned in-scope-if-cheap. Sequencing
   it last means if scope needs to shrink, this is the first thing to cut without touching
   proto/backend work other stories depend on.
2. **Itemized CI checks data source (requirements.md OQ2).** **Piggyback on the existing
   60s poller (Path 2)** — add `Checks`/`Reviews` fields to `PRInfo` and thread them through
   `UpdatePRStatus` → `Session` proto, exactly like `CheckConclusion` today. Rationale:
   architecture.md's answer to this exact question is VERIFIED, not inferred — `GetPRInfoCtx`
   already unmarshals `resp.Reviews`/`resp.StatusCheckRollup` into the same HTTP/subprocess
   response used today; adding fields to `PRInfo` costs **zero new GitHub API calls** and zero
   poller-cadence changes. This overrides features.md's own "(d) unstated needs" leaning toward
   a lazy on-tab-open fetch — that note was inferential, not verified, and a lazy fetch would
   also leave the "why blocked" rollup unable to show itemized reasons for sessions not
   currently open in a tab (e.g. a session list view), which the poller-backed approach doesn't
   have.
3. **"As of" live staleness timestamp (requirements.md OQ3).** **Two separate small labels,
   not one blended timestamp** — "PR status: as of Ns ago" (from `Session.last_pr_status_check`,
   already exists) and a new local-git freshness label (from a new `VCSStatus.status_as_of`
   field, sourced from `WorkspaceService`'s existing `vcsStatusCacheEntry.cachedAt`). Rationale:
   architecture.md confirmed the poller is a fixed 60s tick (VERIFIED, `pr_status_poller.go:39`)
   independent of the 15s local-git cache (`workspace_service.go:60`) — conflating them into one
   timestamp would misrepresent one or the other. Per pitfalls.md #1/#5, the PR-status label's
   copy must say "PR status last confirmed" (not "checks last updated") since GitHub's ETag is
   scoped to the PR resource, not its check-runs/reviews sub-resources — a 304 can bump the
   timestamp without the itemized checks actually having been re-fetched.
4. **Local-only sessions with no PR yet (requirements.md OQ4).** **In scope** — this falls
   out of Epics 4.1/4.2 (commit list + aggregate stat line) with no PR-specific work needed,
   mirroring the IntelliJ Local-Changes framing (intellij-vcs-deepdive.md §3's "merged single
   timeline" recommendation, not a separate panel). Confirmed by features.md (c).7: once commit
   list and the full-mode aggregate line are wired, a no-PR session already shows branch,
   ahead/behind, dirty-status, commit history, and diff stats — genuinely informative without
   any GitHub data. **This claim holds only because `Instance.GetBaseCommitSHA()` (Task 1.1.2a)
   branches on `gitManager.HasWorktree()` and falls back to `GetDirBaseSHA()`** —
   `SessionTypeDirectory` (the default session type; no worktree is ever created for it) has its
   own base-SHA concept, and without that fallback its commit list and aggregate diff stat would
   stay silently empty forever, regardless of anything Epics 4.1/4.2 do on the frontend. Task
   1.1.2a implements the fallback explicitly so this resolution is actually true for both
   session types, not just worktree-mode ones (an earlier draft of this plan implemented only
   the worktree-mode path — see the adversarial review this revision addresses).
5. **Mobile layout (requirements.md OQ5).** **Every new disclosure section (itemized checks,
   reviewer feedback, PR comments) defaults to closed on both breakpoints**; the "why blocked"
   rollup, header, and commit list stay always-visible/open on both. Rationale: no
   viewport-detection hook exists anywhere in `web-app/src/lib/hooks/` today (confirmed by
   grep), and introducing one solely to vary a `defaultExpanded` prop by breakpoint would be an
   unjustified new abstraction for a single call site
   (`.claude/rules/interface-pollution-checklist.md`'s "speculative" smell, applied to hooks).
   A single closed-by-default convention is simpler to test, matches vim-fugitive-deepdive.md
   §5(d)'s "reserve visual weight for the 2-3 signals that actually gate readiness" argument,
   and needs no new code — CSS-only responsive layout (existing vanilla-extract patterns) still
   applies for spacing/touch targets.

---

## Dependency Visualization

```
Phase 1 — Backend Go plumbing (must land first; everything downstream reads its output)
┌─────────────────────────────┐        ┌───────────────────────────────────────────┐
│ Epic 1.1                    │        │ Epic 1.2                                   │
│ Local git: commit list,     │        │ GitHub client: itemized checks/review body/│
│ base-SHA resolution         │        │ mergeable, threaded through the poller     │
└───────────────┬─────────────┘        └───────────────────┬─────────────────────────┘
                │                                           │
                ▼                                           ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│ Phase 2 — Proto + adapter wiring                                                   │
│  Epic 2.1 proto schema  →  Epic 2.2 Go→proto mapping  →  Epic 2.3 TS types/adapters │
└───────────────────────────────────┬───────────────────────────────────────────────┘
                                     │
              ┌──────────────────────┴───────────────────────┐
              ▼                                                ▼
┌───────────────────────────┐                   ┌───────────────────────────────────┐
│ Phase 3                    │                   │ Phase 4                            │
│ mergeability.ts rollup      │──consumed by──▶  │ VcsWidget full-mode layout redesign │
│ (Epic 3.1)                  │                   │ (Epics 4.1–4.6)                    │
└───────────────────────────┘                   └───────────────────┬───────────────┘
                                                                      ▼
                                                     ┌───────────────────────────────┐
                                                     │ Phase 5 — Tests & regression   │
                                                     │ guard (Epic 5.1)               │
                                                     └───────────────────────────────┘
```

No frontend task in Phase 3/4 may start before its corresponding Phase 2 proto field exists
and `make proto-gen` has been run — each Phase 4 story below states its exact Phase 2
dependency.

---

## Phase 1: Backend Data Plumbing (Go)

### Epic 1.1: Local git — commit list, base-SHA resolution, live-session wiring

**Goal**: Populate `VCSStatus` with the session's shipped-commit list, its branch-vs-base
aggregate diff stat, and a local freshness timestamp — for both worktree- and directory-mode
sessions — sourced from already-battle-tested go-git helpers, without adding an unbounded or
unshared git call (ADR-001).

#### Story 1.1.1: Bound `ListShippedCommits`, expose truncation testably, add `DiffStatBetween`
**As a** VCS-tab viewer, **I want** the commit list request to never hang indefinitely, to tell
me when it was cut off in a way that's actually tested, and to see the branch's real
branch-vs-base change size even when my working tree is fully clean, **so that** a slow git op
can't stall the panel, a capped list doesn't silently read as "that's everything," and the
aggregate diff-stat line doesn't go blank the moment a session finishes and commits (see
Blocker 2 in the adversarial review this revision addresses).
**Acceptance Criteria**:
- `ListShippedCommits` takes a `context.Context` and returns whether it hit the cap; the cap is
  injectable so `truncated == true` is actually exercisable in a unit test without a 100+-commit
  fixture (see Task 5.1.2a).
- The one existing caller still compiles and behaves identically (ignores the new return value).
- `git.DiffStatBetween(ctx, repoPath, baseSHA, headSHA)` exists, ctx-bounded the same way, and
  returns the aggregate files-changed/additions/deletions between two commits — the
  `DiffShortstat`-equivalent for a live session's branch-vs-base range that requirements.md's
  Success Metrics ask for (ADR-001's acknowledged follow-up path, implemented now rather than
  deferred).
**Files**: `session/git/ops.go`, `session/git/ops_test.go`, `server/services/backlog_service_ship_status.go`

##### Task 1.1.1a: Add ctx + truncated return to `ListShippedCommits`, with an injectable cap (~5 min)
- In `session/git/ops.go`, change the signature at line 369 to
  `func ListShippedCommits(ctx context.Context, repoPath, baseSHA, headSHA string) ([]ShippedCommit, bool, error)`.
  Wrap the existing body's git open + walk with a `context.WithTimeout(ctx, 30*time.Second)`
  (mirror `FetchBranch`'s pattern at line 25 — the timeout only needs to bound the call, the
  existing go-git calls don't take a ctx themselves, so just `select`-guard the top-level walk
  loop isn't necessary; simplest correct version: derive `_, cancel := context.WithTimeout(ctx, 30*time.Second); defer cancel()` and check `ctx.Err()` once at the top of the `for` loop at line 387 to bail early on timeout).
- Extract the capped-walk body into an unexported `listShippedCommitsWithCap(ctx context.Context, repoPath, baseSHA, headSHA string, cap int) ([]ShippedCommit, bool, error)` that takes the
  cap as a parameter instead of reading `listShippedCommitsCap` directly; `ListShippedCommits`
  becomes a one-line wrapper: `return listShippedCommitsWithCap(ctx, repoPath, baseSHA, headSHA, listShippedCommitsCap)`.
  This is what makes Task 5.1.2a's truncation test possible without a 100+-commit fixture — the
  test calls `listShippedCommitsWithCap` directly (same package, unexported is fine from
  `ops_test.go`) with a small cap like `3` against a small fixture.
- After the loop (line 413), compute `truncated := len(commits) >= cap` and return
  `commits, truncated, nil`.
- Files: `session/git/ops.go`

##### Task 1.1.1b: Update the ship-status call site (~2 min)
- In `server/services/backlog_service_ship_status.go:131`, change
  `shipped, commitsErr := git.ListShippedCommits(item.RepoPath, wt.BaseCommitSHA, lastCommitSha)`
  to `shipped, _, commitsErr := git.ListShippedCommits(ctx, item.RepoPath, wt.BaseCommitSHA, lastCommitSha)`
  (the handler already has `ctx` in scope as the RPC context parameter). Discard the truncation
  bool — ship-status truncation messaging is out of scope per ADR-001's Consequences.
- Files: `server/services/backlog_service_ship_status.go`

##### Task 1.1.1c: Add `git.DiffStatBetween` for the branch-vs-base aggregate diff stat (~4 min)
- **Root cause this fixes**: Task 2.3.2b (as originally written) computed the full-mode
  aggregate diff-stat line from `VCSStatus.staged_files`/`unstaged_files`/`untracked_files` —
  working-tree status categories that go empty the moment everything is committed. A finished,
  PR-ready session (the single most common state a user checks this tab in) is exactly the state
  where that line would go blank, contradicting requirements.md's explicit ask for "a
  `DiffShortstat`-equivalent for the live session." The fix is a real branch-vs-base diff,
  computed server-side, parallel to `ListShippedCommits`.
- In `session/git/ops.go`, add a new exported type and function near `ListShippedCommits`:
  ```go
  // AggregateDiffStat is the files-changed/additions/deletions summary
  // returned by DiffStatBetween — a DiffShortstat-equivalent for a branch's
  // range vs. its base. Distinct from DiffStats (session/git/diff.go), which
  // holds working-tree diff content, not a committed-range summary.
  type AggregateDiffStat struct {
      FilesChanged int
      Additions    int
      Deletions    int
  }

  // DiffStatBetween returns the aggregate files-changed/additions/deletions
  // counts between baseSHA and headSHA, by summing the existing
  // FileStatsBetween's per-file counts rather than adding a second go-git
  // diff implementation. Takes a ctx so the new live-session call site
  // (WorkspaceService.GetVCSStatus) can bound it the same way Task 1.1.1a
  // bounds ListShippedCommits; FileStatsBetween's own go-git patch
  // computation doesn't accept a ctx, so the timeout guards against
  // starting the call once the caller's deadline has already passed, not
  // against a hang mid-computation.
  func DiffStatBetween(ctx context.Context, repoPath, baseSHA, headSHA string) (AggregateDiffStat, error) {
      ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
      defer cancel()
      if err := ctx.Err(); err != nil {
          return AggregateDiffStat{}, err
      }
      stats, err := FileStatsBetween(repoPath, baseSHA, headSHA)
      if err != nil {
          return AggregateDiffStat{}, err
      }
      var additions, deletions int
      for _, s := range stats {
          additions += s.Additions
          deletions += s.Deletions
      }
      return AggregateDiffStat{FilesChanged: len(stats), Additions: additions, Deletions: deletions}, nil
  }
  ```
  Deliberately does **not** change `FileStatsBetween`'s existing signature — it has 2 other
  call sites (`backlog_service_triage.go`, `backlog_lifecycle_pr.go`; `backlog_service_ship_status.go`
  only references it in a doc comment, not a call — corrected in round-2 review) that don't need
  ctx-bounding for this feature; wrapping it keeps this fix's blast radius to one new function.
- **Test coverage (round-2 review concern)**: `DiffStatBetween` ships with no dedicated unit
  test in the original patch — only indirect coverage via a `WorkspaceService` integration test
  (Task 5.1.2c). Add a table-driven `TestDiffStatBetween` in `session/git/ops_test.go` alongside
  the existing `ListShippedCommits` tests, covering: additions-only, deletions-only, mixed,
  zero-diff (`baseSHA == headSHA`), and a `FileStatsBetween` error passthrough (invalid SHA).
  This is a leaf-level unit test of the summation logic, independent of the integration test.
- Files: `session/git/ops.go`, `session/git/ops_test.go`

#### Story 1.1.2: Expose the session's base commit SHA to `server/services`, for both session types
**As a** backend developer wiring the live commit list, **I want** an exported accessor for
the session's recorded base SHA that works for directory sessions as well as worktree sessions,
**so that** `WorkspaceService` (a different package) doesn't need direct access to `Instance`'s
unexported `gitManager` field, and `SessionTypeDirectory` — the *default* session type
(`session/instance.go:836`, "Default to directory session if not specified for backward
compatibility") — isn't silently excluded from the commit list and aggregate diff stat.
**Acceptance Criteria**:
- `WorkspaceService.GetVCSStatus` can read a session's base SHA without touching unexported
  `Instance` fields.
- The accessor returns a usable base SHA for **both** worktree-mode sessions
  (`GitWorktreeManager.GetBaseCommitSHA()`) and directory-mode sessions
  (`GitWorktreeManager.GetDirBaseSHA()`, `dirBaseSHA`) — mirroring the branch
  `computeDirDiffStats` already uses (`session/instance_worktree.go:432-433`) to decide which
  base SHA a given session has. A directory session with a resolved `dirBaseSHA` must not return
  `""` (this was Blocker 1 in the adversarial review this revision addresses: the prior draft
  delegated only to the worktree-mode accessor, so every directory session's commit list and
  aggregate diff stat stayed empty forever, with no error and no log).
**Files**: `session/instance_worktree.go`

##### Task 1.1.2a: Add `Instance.GetBaseCommitSHA()`, branching on `HasWorktree()` (~3 min)
- In `session/instance_worktree.go`, near the existing `GetGitWorktree`/`GetWorktreePath`
  accessors (around line 400), add:
  ```go
  // GetBaseCommitSHA returns the commit SHA this session's branch diverged
  // from. Branches on whether the session has a git worktree: worktree-mode
  // sessions read GitWorktreeManager.GetBaseCommitSHA(); SessionTypeDirectory
  // sessions (the default session type — no worktree is ever created for
  // them) have no worktree, so this falls back to GetDirBaseSHA(), mirroring
  // computeDirDiffStats's existing HasWorktree()-gated pattern
  // (session/instance_worktree.go:432-433) for exactly the same reason.
  // Returns "" only when neither is set (e.g. a session that hasn't
  // resolved a base SHA yet). Delegates to the unexported gitManager (a
  // concrete GitWorktreeManager value, not an interface — both accessors
  // are reachable directly) so callers outside the session package (e.g.
  // WorkspaceService) don't need direct access to it.
  func (i *Instance) GetBaseCommitSHA() string {
      if i.gitManager.HasWorktree() {
          return i.gitManager.GetBaseCommitSHA()
      }
      return i.gitManager.GetDirBaseSHA()
  }
  ```
- Files: `session/instance_worktree.go`

#### Story 1.1.3: Wire commit list + aggregate diff stat + local staleness into `GetVCSStatus`
**As a** VCS-tab viewer, **I want** the branch's shipped commits, its real branch-vs-base
change size, and a local freshness indicator returned alongside the existing status fields,
**so that** the frontend can render them without a second round trip, and the aggregate stat
line stays populated even once I've committed everything.
**Acceptance Criteria**: `vc.VCSStatus` carries `Commits`/`CommitsTruncated`/`CommitsUnavailable`/
`AggregateDiffStat`; `WorkspaceService.GetVCSStatus` populates them for git sessions (worktree
**or** directory-mode, per Task 1.1.2a's fix) with a resolved base SHA; a commit-list fetch
failure sets `CommitsUnavailable` so the frontend can distinguish "computation failed" from
"genuinely zero commits" (Concern 1 in the adversarial review); the whole response is covered
by the existing 15s cache (ADR-001).
**Files**: `session/vc/types.go`, `server/services/workspace_service.go`

##### Task 1.1.3a: Add `Commits`/`CommitsTruncated`/`CommitsUnavailable`/`AggregateDiffStat` to `vc.VCSStatus` (~4 min)
- In `session/vc/types.go`, add to the `VCSStatus` struct (after `ConflictFiles`, line 99):
  ```go
  // Commits lists the branch's commits not yet on its recorded base (newest first),
  // Git only — nil for Jujutsu or when no base SHA is recorded. Populated by the
  // caller (WorkspaceService), not by GitProvider.GetStatus itself, since GetStatus
  // has no access to the session's recorded base SHA — that's an Instance-level
  // concept the vc.VCSProvider abstraction doesn't know about.
  Commits []git.ShippedCommit
  // CommitsTruncated is true when Commits was cut off by ListShippedCommits's cap.
  CommitsTruncated bool
  // CommitsUnavailable is true when a base/head SHA was resolved and a commit-list
  // fetch was attempted but failed or timed out — distinct from Commits simply being
  // empty because the branch genuinely has zero unshipped commits. Lets the frontend
  // render "couldn't load commits" instead of silently showing nothing (the failure
  // mode Concern 1 of the adversarial review names).
  CommitsUnavailable bool
  // AggregateDiffStat is the branch's total change size vs. its recorded base — a
  // DiffShortstat-equivalent, distinct from the working-tree
  // staged/unstaged/untracked/conflict file lists above, and populated even when the
  // working tree is fully clean (e.g. a finished, PR-ready session — see Blocker 2 of
  // the adversarial review, which this field exists to fix). nil when no base SHA is
  // resolved.
  AggregateDiffStat *git.AggregateDiffStat
  ```
- Add the import `"github.com/tstapler/stapler-squad/session/git"` to `session/vc/types.go`
  (confirmed no import cycle: neither `session/git` nor `session/vc` currently imports the
  other).
- Files: `session/vc/types.go`

##### Task 1.1.3b: Resolve base/head SHA and populate commits + aggregate diff stat in `GetVCSStatus` (~6 min)
- In `server/services/workspace_service.go`, inside `GetVCSStatus` (line 132), after the
  `provider.GetStatus()` call (line 176) and before the cache-store call (line 183), add:
  ```go
  if _, isGit := provider.(*vc.GitProvider); isGit {
      if baseSHA := instance.GetBaseCommitSHA(); baseSHA != "" {
          if headSHA, headErr := git.GetHeadCommitSHA(workDir); headErr != nil {
              // Round-2 review finding: a HEAD-resolution failure is the same
              // "computation failed" class as a ListShippedCommits failure below —
              // it must not be conflated with "zero unshipped commits" either.
              log.Debug("GetVCSStatus: failed to resolve HEAD commit SHA", "workDir", workDir, "err", headErr)
              status.CommitsUnavailable = true
          } else if headSHA != baseSHA {
              commits, truncated, listErr := git.ListShippedCommits(ctx, workDir, baseSHA, headSHA)
              if listErr == nil {
                  status.Commits = commits
                  status.CommitsTruncated = truncated
              } else {
                  log.Debug("GetVCSStatus: failed to list shipped commits", "workDir", workDir, "err", listErr)
                  status.CommitsUnavailable = true
              }

              if diffStat, diffErr := git.DiffStatBetween(ctx, workDir, baseSHA, headSHA); diffErr == nil {
                  status.AggregateDiffStat = &diffStat
              } else {
                  log.Debug("GetVCSStatus: failed to compute aggregate diff stat", "workDir", workDir, "err", diffErr)
              }
          }
      }
  }
  ```
  (the type assertion only gates "is this a git session" — Jujutsu has no equivalent
  commit-listing/diff-stat call, so the blank identifier discards the asserted value itself;
  `instance.GetBaseCommitSHA()` now correctly resolves for directory sessions too, per Task
  1.1.2a, so this block populates commits and the aggregate diff stat for both session types)
  A failed HEAD-SHA resolution or commit-list lookup is non-fatal but now distinguishable
  (`CommitsUnavailable`) — the rest of the status is still useful, matching the existing
  non-fatal pattern in `backlog_service_ship_status.go`'s own `ListShippedCommits` call. A
  failed diff-stat lookup is non-fatal and log-only (not flagged to the frontend) — the
  aggregate stat line simply omits itself when unset, matching the existing `aggregateStats`
  optionality contract in `web-app/src/lib/vcs/types.ts`; this wasn't called out as a distinct
  failure mode by the review the way commit-list unavailability was, so no dedicated signal was
  added for it.
- Add the import `"github.com/tstapler/stapler-squad/session/git"` to
  `server/services/workspace_service.go` (already imports `session/vc`).
- Files: `server/services/workspace_service.go`

##### Task 1.1.3c: Set `status_as_of` on both cache-hit and fresh-compute response branches (~3 min)
- In `GetVCSStatus`, the cache-hit branch (line 156) currently returns
  `vcsStatusToProto(entry.status)` directly — change to build the response, then set
  `resp.VcsStatus.StatusAsOf = timestamppb.New(entry.cachedAt)` before returning (this proto
  field is added in Task 2.1.1). The fresh-compute branch (line 185) sets
  `resp.VcsStatus.StatusAsOf = timestamppb.New(time.Now())` (or reuse the `time.Now()` already
  captured when populating `vcsStatusCache.Store`, line 183, for exact consistency between the
  stored `cachedAt` and the returned timestamp).
- This is deliberately set at the RPC-handler layer, not inside `vcsStatusToProto` or
  `vc.VCSStatus` itself, since `cachedAt` is a property of `WorkspaceService`'s own cache, not
  of the VCS provider's status computation.
- Files: `server/services/workspace_service.go`

---

### Epic 1.2: GitHub client — stop discarding itemized checks, review body, mergeable

**Goal**: Surface `ghStatusCheckItem`/`ghReviewItem.Body`/`PRInfo.Mergeable` — all already
fetched by the existing `gh pr view` call — through `PRInfo` and the poller, with zero new
GitHub API calls (per architecture.md's VERIFIED answer to OQ2).

#### Story 1.2.1: Extend `PRInfo` with itemized checks and review feedback
**As a** backend developer, **I want** typed, exported `CheckItem`/`ReviewItem` slices on
`PRInfo`, **so that** downstream code can access itemized data instead of only the collapsed
conclusion/counts.
**Acceptance Criteria**: `GetPRInfoCtx` populates `PRInfo.Checks`/`PRInfo.Reviews` from the
same response it already unmarshals; `getCheckConclusion`/`parseReviewCounts` are unchanged
(still the source of the collapsed fields, now read alongside the itemized ones, not replaced).
**Files**: `github/client.go`

##### Task 1.2.1a: Add `CheckItem`/`ReviewItem` exported types (~3 min)
- In `github/client.go`, near `ghStatusCheckItem`/`ghReviewItem` (lines 116-132), add:
  ```go
  // CheckItem is one itemized CI check from a PR's statusCheckRollup — the data
  // getCheckConclusion collapses into a single string; exported so callers that want
  // the itemized view (e.g. the VCS tab's "why blocked" rollup) don't need to.
  type CheckItem struct {
      Name       string
      Context    string
      State      string
      Status     string
      Conclusion string
  }

  // ReviewItem is one PR review — the data parseReviewCounts collapses into
  // approved/changes-requested counts, exported so callers can also read the
  // reviewer's Body text (e.g. a CHANGES_REQUESTED review's stated reason).
  type ReviewItem struct {
      Author string
      State  string
      Body   string
  }
  ```
- Files: `github/client.go`

##### Task 1.2.1b: Add `Checks`/`Reviews` fields to `PRInfo` and populate in `GetPRInfoCtx` (~3 min)
- Add `Checks []CheckItem` and `Reviews []ReviewItem` to the `PRInfo` struct (after
  `CheckStatus`, line 74).
- In `GetPRInfoCtx` (line 268), after `checkConclusion, checkStatus := getCheckConclusion(...)`
  (line 300), add:
  ```go
  checks := make([]CheckItem, len(resp.StatusCheckRollup))
  for i, c := range resp.StatusCheckRollup {
      checks[i] = CheckItem{Name: c.Name, Context: c.Context, State: c.State, Status: c.Status, Conclusion: c.Conclusion}
  }
  reviews := make([]ReviewItem, len(resp.Reviews))
  for i, r := range resp.Reviews {
      reviews[i] = ReviewItem{Author: r.Author.Login, State: r.State, Body: r.Body}
  }
  ```
  Add `Checks: checks, Reviews: reviews,` to the returned `&PRInfo{...}` literal (line 302).
- Files: `github/client.go`

#### Story 1.2.2: Thread checks/reviews/mergeable through the poller into `Instance`
**As a** VCS-tab viewer, **I want** the itemized CI checks, review feedback, and GitHub's own
mergeable signal to reach the session's in-memory state, **so that** the frontend can read them
off the `Session` proto exactly like the existing collapsed fields.
**Acceptance Criteria**: `Instance.UpdatePRStatus` carries the three new fields;
`applyPRUpdate` populates them from `PRInfo`; no new persistence is added (PR status is
already poll-refreshed on every server start, per `storage.UpdateInstancePRStatus`'s existing
no-op body — confirmed at `session/storage.go:620`).
**Files**: `session/instance_terminal.go`, `session/pr_status_poller.go`, `session/instance.go`,
`session/instance_snapshot.go`

##### Task 1.2.2a: Introduce `PRStatusUpdate` and refactor `UpdatePRStatus` to take it (~5 min)
- `UpdatePRStatus`'s existing signature (`session/instance_terminal.go:389`) already has 7
  same-typed positional params (3 strings, 2 ints, 2 bools) — adding 3 more
  (`checks []github.CheckItem`, `reviews []github.ReviewItem`, `mergeable string`) crosses the
  line `.claude/rules/primitive-obsession-checklist.md` exists to catch. Replace the signature
  with a struct:
  ```go
  // PRStatusUpdate bundles the fields PRStatusPoller writes to an Instance on each
  // successful fetch. Introduced when itemized checks/review feedback/mergeable were
  // added — the prior 7-positional-parameter signature was already at the limit this
  // repo's primitive-obsession checklist flags.
  type PRStatusUpdate struct {
      State           string
      Priority        string
      CheckConclusion string
      Mergeable       string
      ApprovedCount   int
      ChangesReqCount int
      IsDraft         bool
      Terminal        bool
      Checks          []github.CheckItem
      Reviews         []github.ReviewItem
  }

  func (i *Instance) UpdatePRStatus(update PRStatusUpdate) prUpdateResult {
      var result prUpdateResult
      _ = i.sendSyncErr(func(s *instanceState) error {
          inst := s.inst
          inst.mu.Lock()
          result.PriorityChanged = update.Priority != inst.GitHubPRPriority
          result.CheckConclusionChanged = update.CheckConclusion != inst.GitHubCheckConclusion
          inst.GitHubPRState = update.State
          inst.GitHubPRPriority = update.Priority
          inst.GitHubPRIsDraft = update.IsDraft
          inst.GitHubApprovedCount = update.ApprovedCount
          inst.GitHubChangesReqCount = update.ChangesReqCount
          inst.GitHubCheckConclusion = update.CheckConclusion
          inst.GitHubPRStatusTerminal = update.Terminal
          inst.GitHubChecks = update.Checks
          inst.GitHubReviewFeedback = update.Reviews
          inst.GitHubMergeable = update.Mergeable
          inst.LastPRStatusCheck = time.Now()
          snap := buildSnapshot(inst)
          inst.mu.Unlock()
          inst.snapshot.Store(snap)
          return nil
      })
      return result
  }
  ```
- Add `"github.com/tstapler/stapler-squad/github"` to `session/instance_terminal.go`'s imports
  if not already present (the `session` package already imports it elsewhere, e.g.
  `pr_status_poller.go`, so no new dependency).
- Files: `session/instance_terminal.go`

##### Task 1.2.2b: Add the 3 new fields to `Instance` and `InstanceSnapshot` (~3 min)
- In `session/instance.go`, near the existing `GitHubCheckConclusion` field (line 261), add:
  ```go
  // GitHubChecks is the itemized statusCheckRollup from the last successful poll.
  GitHubChecks []github.CheckItem `json:"github_checks,omitempty"`
  // GitHubReviewFeedback is the itemized review list (author/state/body) from the
  // last successful poll.
  GitHubReviewFeedback []github.ReviewItem `json:"github_review_feedback,omitempty"`
  // GitHubMergeable mirrors PRInfo.Mergeable: "mergeable"/"conflicting"/"unknown".
  GitHubMergeable string `json:"github_mergeable,omitempty"`
  ```
- In `session/instance_snapshot.go`, add the same 3 fields to the `GitHubIntegration` struct
  (after `GitHubCheckConclusion`, line 55) and to `buildSnapshot`'s `GitHub: GitHubIntegration{...}`
  literal (after `GitHubCheckConclusion: i.GitHubCheckConclusion`, line 197) — per this file's
  own doc comment, `InstanceSnapshot`'s field set must be kept in sync with `Instance`'s mutable
  fields whenever one is added, even though `InstanceToProto` reads this particular field group
  directly off `inst.*` rather than `snap.GitHub.*` (matching the existing, pre-existing
  convention for this specific sibling field group — not something this task changes).
- Files: `session/instance.go`, `session/instance_snapshot.go`

##### Task 1.2.2c: Update `applyPRUpdate` to build and pass `PRStatusUpdate` (~3 min)
- In `session/pr_status_poller.go`'s `applyPRUpdate` (line 396), replace the flat local
  variables (lines 400-409) and the `inst.UpdatePRStatus(...)` call (line 411) with:
  ```go
  update := PRStatusUpdate{Priority: priority, Terminal: terminal}
  if prInfo != nil {
      update.State = prInfo.State
      update.CheckConclusion = prInfo.CheckConclusion
      update.Mergeable = prInfo.Mergeable
      update.ApprovedCount = prInfo.ApprovedCount
      update.ChangesReqCount = prInfo.ChangesRequestedCount
      update.IsDraft = prInfo.IsDraft
      update.Checks = prInfo.Checks
      update.Reviews = prInfo.Reviews
  }
  result := inst.UpdatePRStatus(update)
  ```
  The subsequent `p.storage.UpdateInstancePRStatus(...)` call (line 414) is unchanged — it's
  already a no-op (`session/storage.go:620`), so its existing 8-positional-arg call site needs
  no update.
- Files: `session/pr_status_poller.go`

---

## Phase 2: Proto + Adapter Wiring

### Epic 2.1: Proto schema — `VCSStatus` and `Session` additions

**Goal**: Add the new fields Phase 1 populates to the wire schema, additive-only, no
renumbering (per pitfalls.md's field-numbering-rule gap — stated explicitly here since no
repo doc covers it).

#### Story 2.1.1: `VCSStatus` — commits, staleness, truncation, aggregate diff stat
**Acceptance Criteria**: `VCSStatus` carries the live commit list, an unavailability signal, a
branch-vs-base aggregate diff stat, and a local-staleness timestamp; `ShippedCommit` is defined
once (in `types.proto`, reused by `backlog.proto`) rather than duplicated field-for-field
(Concern 4 in the adversarial review this revision addresses); `make proto-gen` succeeds.
**Files**: `proto/session/v1/types.proto`, `proto/session/v1/backlog.proto`

##### Task 2.1.1a: Relocate `ShippedCommit` into `types.proto`, add `VCSStatus` fields 17-21 (~5 min)
- **Naming decision**: reuse `backlog.proto`'s existing `ShippedCommit` message rather than
  reintroducing a separately-named `VCSCommitSummary` — the two messages are field-for-field
  identical (`sha`/`summary`/`author_name`/`authored_at`), and a rename would ripple through
  `session/git.ShippedCommit` (the Go type both proto messages map to, already named
  `ShippedCommit`) and every existing Go/TS call site that already imports the generated
  `ShippedCommit` symbol (`session/backlog_lifecycle.go`, `server/services/backlog_service_ship_status.go`,
  `web-app/src/lib/vcs/adapters.ts`) for no behavioral gain. Relocating (not duplicating) also
  resolves the import-cycle concern the original `VCSCommitSummary` design worked around:
  `backlog.proto` already imports `types.proto` (`backlog.proto:6`), so moving `ShippedCommit`
  the other direction has nowhere left to cycle.
- In `proto/session/v1/types.proto`, before `message VCSStatus` (line 948), add:
  ```protobuf
  // ShippedCommit is one commit in a shipped-but-unmerged range — used both by a
  // live session's VCSStatus.commits (this file) and by BacklogItemShipStatus's
  // durable snapshot (backlog.proto, which imports this file). Relocated here from
  // backlog.proto so the two call sites share one message definition instead of two
  // independently-maintained copies (see Task 2.1.1b).
  message ShippedCommit {
    string sha = 1;
    string summary = 2; // first line of the commit message
    string author_name = 3;
    google.protobuf.Timestamp authored_at = 4;
  }

  // AggregateDiffStat is the files-changed/additions/deletions summary for a
  // branch's range vs. its base — a DiffShortstat-equivalent. Maps to
  // git.AggregateDiffStat in Go.
  message AggregateDiffStat {
    int32 files_changed = 1;
    int32 additions = 2;
    int32 deletions = 3;
  }
  ```
  Inside `message VCSStatus` (after `conflict_files = 16`, line 996), add:
  ```protobuf
  // Commits lists the branch's commits not yet on its recorded base (newest first).
  // Empty for Jujutsu sessions or when no base SHA is recorded.
  repeated ShippedCommit commits = 17;

  // True when `commits` was capped before reaching the branch's full unshipped
  // commit count (see session/git.ListShippedCommits's cap).
  bool commits_truncated = 18;

  // When this VCSStatus was computed — the local-git-status cache's timestamp,
  // independent of any GitHub PR-status staleness (see Session.last_pr_status_check).
  google.protobuf.Timestamp status_as_of = 19;

  // The branch's total change size vs. its recorded base (files/+/-), independent
  // of the working-tree file lists above — populated even when the working tree is
  // fully clean. Unset when no base SHA is recorded or the computation failed.
  AggregateDiffStat aggregate_diff_stat = 20;

  // True when a commit-list fetch was attempted but failed or timed out —
  // distinguishes "computation failed" from "genuinely zero commits" so the
  // frontend doesn't render a failure as a silent empty state.
  bool commits_unavailable = 21;
  ```
- Files: `proto/session/v1/types.proto`

##### Task 2.1.1b: Remove the now-duplicate `ShippedCommit` from `backlog.proto` (~2 min)
- In `proto/session/v1/backlog.proto`, delete the `message ShippedCommit { ... }` block (lines
  390-396). `BacklogItemShipStatus.commits` (`backlog.proto:360`,
  `repeated ShippedCommit commits = 12;`) needs no change: both files share the `session.v1`
  package, and `backlog.proto` already imports `types.proto` (line 6), so the field reference
  resolves to the relocated message with no qualification change.
- This has TS-side (but not Go-side) fallout: protobuf-es generates one `.ts` file per `.proto`
  file, so `ShippedCommit`/`ShippedCommitSchema` move from `@/gen/session/v1/backlog_pb` to
  `@/gen/session/v1/types_pb`. protoc-gen-go generates both files into the single Go package
  `sessionv1` regardless of source `.proto` file, so no Go import changes are needed. See Task
  2.3.2f for the TS import-path fix.
- Files: `proto/session/v1/backlog.proto`

#### Story 2.1.2: `Session` — itemized checks, review feedback, mergeable
**Acceptance Criteria**: `Session` carries the itemized poller data; `make proto-gen` succeeds.
**Files**: `proto/session/v1/types.proto`

##### Task 2.1.2a: Add `GithubCheckItem`/`GithubReviewFeedback` messages and `Session` fields 80-82 (~4 min)
- In `proto/session/v1/types.proto`, near the other GitHub-integration messages/fields
  (the block ending at `last_pr_status_check = 39`, before `rate_limit_state = 40`), add two
  new messages at file scope (next to `PRInfo`, line 735, is a reasonable location) and 3 new
  `Session` fields — confirmed via `grep` that 79 is the highest field number currently used in
  `message Session`, so the next available number is 80:
  ```protobuf
  message GithubCheckItem {
    string name = 1;
    string context = 2;
    string state = 3;
    string status = 4;
    string conclusion = 5;
  }

  message GithubReviewFeedback {
    string author = 1;
    string state = 2;
    string body = 3;
  }
  ```
  Inside `message Session`, add:
  ```protobuf
  // Itemized statusCheckRollup from the last successful PR-status poll.
  repeated GithubCheckItem github_checks = 80;

  // Itemized review list (author/state/body) from the last successful poll.
  repeated GithubReviewFeedback github_review_feedback = 81;

  // GitHub's own mergeable signal: "mergeable"/"conflicting"/"unknown".
  string github_mergeable = 82;
  ```
- Files: `proto/session/v1/types.proto`

##### Task 2.1.2b: Run `make proto-gen` and confirm both sides regenerate (~2 min)
- Run `make proto-gen` from the repo root. Confirm no errors and that
  `gen/proto/go/session/v1/types.pb.go` and `web-app/src/gen/session/v1/types_pb.ts` both
  contain the new fields/messages, and that `ShippedCommit` is gone from
  `gen/proto/go/session/v1/backlog.pb.go`/`web-app/src/gen/session/v1/backlog_pb.ts` and present
  in the `types.*` files instead (`rg -n "ShippedCommit|AggregateDiffStat|GithubCheckItem|StatusAsOf|statusAsOf|CommitsUnavailable" gen/proto/go/session/v1/types.pb.go gen/proto/go/session/v1/backlog.pb.go web-app/src/gen/session/v1/types_pb.ts web-app/src/gen/session/v1/backlog_pb.ts`).
- Files: none committed (gitignored generated output, per root CLAUDE.md)

---

### Epic 2.2: Go → proto mapping

**Goal**: Map the new Go struct fields onto the new proto fields.

#### Story 2.2.1: `vcsStatusToProto` — commits, truncation, unavailability, aggregate diff stat
**Acceptance Criteria**: `commits`/`commits_truncated`/`commits_unavailable`/`aggregate_diff_stat`
all populate on every `GetVCSStatus` response for a git session with a resolved base SHA
(worktree or directory-mode).
**Files**: `server/services/workspace_service.go`

##### Task 2.2.1a: Map the new `vc.VCSStatus` fields in `vcsStatusToProto` (~4 min)
- In `vcsStatusToProto` (`server/services/workspace_service.go:414`), after the existing
  per-file-list loops (line 445), add:
  ```go
  for _, c := range status.Commits {
      commit := &sessionv1.ShippedCommit{Sha: c.SHA, Summary: c.Summary, AuthorName: c.AuthorName}
      if !c.AuthorAt.IsZero() {
          commit.AuthoredAt = timestamppb.New(c.AuthorAt)
      }
      protoStatus.Commits = append(protoStatus.Commits, commit)
  }
  protoStatus.CommitsTruncated = status.CommitsTruncated
  protoStatus.CommitsUnavailable = status.CommitsUnavailable
  if status.AggregateDiffStat != nil {
      protoStatus.AggregateDiffStat = &sessionv1.AggregateDiffStat{
          FilesChanged: int32(status.AggregateDiffStat.FilesChanged),
          Additions:    int32(status.AggregateDiffStat.Additions),
          Deletions:    int32(status.AggregateDiffStat.Deletions),
      }
  }
  ```
  (uses `sessionv1.ShippedCommit`, not a separate `VCSCommitSummary` type — see Task 2.1.1a's
  relocation rationale; `StatusAsOf` is set separately in `GetVCSStatus` itself per Task 1.1.3c,
  not here, since `vcsStatusToProto` only receives `*vc.VCSStatus`, not the cache timestamp.)
- Files: `server/services/workspace_service.go`

#### Story 2.2.2: `InstanceToProto` — checks, review feedback, mergeable
**Acceptance Criteria**: The 3 new `Session` fields populate identically to their sibling
GitHub-status fields.
**Files**: `server/adapters/instance_adapter.go`

##### Task 2.2.2a: Map `inst.GitHubChecks`/`GitHubReviewFeedback`/`GitHubMergeable` (~4 min)
- In `server/adapters/instance_adapter.go`, add two small mapping helpers near the file's other
  small converters (e.g. `instanceTypeToProto`):
  ```go
  func checksToProto(checks []github.CheckItem) []*sessionv1.GithubCheckItem {
      out := make([]*sessionv1.GithubCheckItem, len(checks))
      for i, c := range checks {
          out[i] = &sessionv1.GithubCheckItem{Name: c.Name, Context: c.Context, State: c.State, Status: c.Status, Conclusion: c.Conclusion}
      }
      return out
  }

  func reviewFeedbackToProto(reviews []github.ReviewItem) []*sessionv1.GithubReviewFeedback {
      out := make([]*sessionv1.GithubReviewFeedback, len(reviews))
      for i, r := range reviews {
          out[i] = &sessionv1.GithubReviewFeedback{Author: r.Author, State: r.State, Body: r.Body}
      }
      return out
  }
  ```
  Add `"github.com/tstapler/stapler-squad/github"` to the file's imports. In the
  `protoSession := &sessionv1.Session{...}` literal, alongside the existing
  `GithubCheckConclusion: inst.GitHubCheckConclusion,` line (line 71), add:
  ```go
  GithubChecks:         checksToProto(inst.GitHubChecks),
  GithubReviewFeedback: reviewFeedbackToProto(inst.GitHubReviewFeedback),
  GithubMergeable:      inst.GitHubMergeable,
  ```
- Files: `server/adapters/instance_adapter.go`

---

### Epic 2.3: TypeScript types + adapters

**Goal**: Extend `VcsWidgetData`'s TS types and the `fromSessionVcs`/`fromSessionGithub`
adapters to consume the new proto fields, defensively (old cached data may lack them, per
pitfalls.md's additive-parsing precedent).

#### Story 2.3.1: New TS types
**Acceptance Criteria**: `CheckItemSummary`, `ReviewFeedbackSummary` exist; `GithubSummary`
and `VcsWidgetDataCommon` carry the new optional fields.
**Files**: `web-app/src/lib/vcs/types.ts`

##### Task 2.3.1a: Add `CheckItemSummary`/`ReviewFeedbackSummary`, extend `GithubSummary` (~4 min)
- In `web-app/src/lib/vcs/types.ts`, add near `CommitSummary` (line 23):
  ```ts
  export interface CheckItemSummary {
    name: string;
    context: string;
    state: string;
    status: string;
    conclusion: string;
  }

  export interface ReviewFeedbackSummary {
    author: string;
    state: string;
    body: string;
  }
  ```
  Add to `GithubSummary` (line 33-43): `mergeable: string; checks: CheckItemSummary[]; reviewFeedback: ReviewFeedbackSummary[]; lastCheckedAt?: Date;`
- Files: `web-app/src/lib/vcs/types.ts`

##### Task 2.3.1b: Add `statusAsOf`/`commitsTruncated`/`commitsUnavailable` to `VcsWidgetDataCommon` (~2 min)
- In `VcsWidgetDataCommon` (types.ts:47-75), add:
  ```ts
  /** Local-git-status freshness — independent of github.lastCheckedAt (PR staleness). */
  statusAsOf?: Date;
  /** True when `commits` was capped server-side before reaching the branch's full count. */
  commitsTruncated?: boolean;
  /**
   * True when the backend attempted to fetch the commit list and it failed or timed
   * out — distinct from `commits` simply being empty because the branch has no
   * unshipped commits yet. Lets the UI render "couldn't load commits" instead of a
   * silent empty state.
   */
  commitsUnavailable?: boolean;
  ```
  (`aggregateStats` already exists on this interface, below — Task 2.3.2b changes what
  populates it for the `fromSessionVcs` source, not its shape.)
- Files: `web-app/src/lib/vcs/types.ts`

#### Story 2.3.2: Adapter wiring — `fromSessionVcs`, `fromSessionGithub`
**Acceptance Criteria**: `fromSessionVcs` no longer hardcodes `commits: []`; `aggregateStats` is
sourced from the server-computed branch-vs-base `aggregate_diff_stat` field (not from
working-tree `fileChanges`, which goes empty exactly when a session is clean and PR-ready — see
Blocker 2 in the adversarial review this revision addresses); every new field defaults
gracefully when absent (old cached proto data).
**Files**: `web-app/src/lib/vcs/adapters.ts`

##### Task 2.3.2a: Populate `commits`, `statusAsOf`, `commitsTruncated`, `commitsUnavailable` in `fromSessionVcs` (~4 min)
- In `web-app/src/lib/vcs/adapters.ts`, replace the hardcoded `commits: []` (line 92) with
  `commits: status.commits.map(toCommitSummary),` — reusing the existing `toCommitSummary`
  helper (line 102) directly, with no new mirror helper needed: since `ShippedCommit` is now the
  single relocated proto message used by both `VCSStatus.commits` (here) and
  `BacklogItemShipStatus.commits` (`toCommitSummary`'s existing caller), `status.commits` is the
  exact same generated TS type `toCommitSummary` already accepts — an earlier draft of this task
  invented a separate `toVcsCommitSummary` mirror helper only because the two commit messages
  used to be distinct types (`VCSCommitSummary` vs. `ShippedCommit`); Task 2.1.1a's relocation
  removes the need for it.
  Add
  `statusAsOf: toDate(status.statusAsOf) ?? undefined, commitsTruncated: status.commitsTruncated, commitsUnavailable: status.commitsUnavailable,`
  to the returned object.
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 2.3.2b: Source `aggregateStats` from the server-computed `aggregate_diff_stat` field (~3 min)
- **Root cause this fixes**: the original version of this task computed `aggregateStats` from
  `flattenFileChanges(status)` — i.e. `VCSStatus.staged_files`/`unstaged_files`/`untracked_files`/
  `conflict_files`, all working-tree status categories. Those go empty the moment a session
  finishes and commits everything (`VCSStatus.is_clean` becomes true), which is the single most
  common state a user checks this tab in — "my agent finished, PR is open, is it ready?" — and
  exactly the state where requirements.md's Success Metrics ask for the aggregate diff-stat line
  to still be visible. The fix sources it from `VCSStatus.aggregate_diff_stat` (Task 1.1.1c/
  1.1.3b/2.1.1a/2.2.1a), a real branch-vs-base diff that survives being clean.
- Also in `fromSessionVcs`, before the return statement, compute:
  ```ts
  const aggregateStats = status.aggregateDiffStat
    ? {
        filesChanged: status.aggregateDiffStat.filesChanged,
        additions: status.aggregateDiffStat.additions,
        deletions: status.aggregateDiffStat.deletions,
      }
    : undefined;
  ```
  and set `aggregateStats` on the returned object. `flattenFileChanges(status)` is still used
  elsewhere in `fromSessionVcs` to populate `fileChanges` (the per-file list) — that usage is
  unaffected; only `aggregateStats`'s source changes.
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 2.3.2c: Populate `checks`/`reviewFeedback`/`mergeable`/`lastCheckedAt` in `fromSessionGithub` (~4 min)
- In `fromSessionGithub` (adapters.ts:68-81), add to the returned object:
  ```ts
  mergeable: session.githubMergeable,
  checks: session.githubChecks.map((c) => ({ name: c.name, context: c.context, state: c.state, status: c.status, conclusion: c.conclusion })),
  reviewFeedback: session.githubReviewFeedback.map((r) => ({ author: r.author, state: r.state, body: r.body })),
  lastCheckedAt: toDate(session.lastPrStatusCheck) ?? undefined,
  ```
  `session.githubChecks`/`githubReviewFeedback` default to empty arrays when absent (protobuf-es
  repeated-field default), so no extra guard is needed — matches the additive-parsing precedent
  pitfalls.md cites (`adapters.ts`'s `hasSnapshot`/`commitsOrLastCommitFallback` pattern).
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 2.3.2d: Update `fromShipStatusGithub`/`fromUnfinishedWorktreeGithub` for the new required `GithubSummary` fields (~3 min)
- `GithubSummary` (Task 2.3.1a) is a plain interface, not all-optional — `fromShipStatusGithub`
  (adapters.ts:135-152) and `fromUnfinishedWorktreeGithub` (adapters.ts:186-200) must set the 4
  new fields too, or `tsc --noEmit` fails. Set `mergeable: "unknown", checks: [], reviewFeedback: []`
  in both (historical/ship-status snapshots and the unfinished-worktree scanner never captured
  this data), and `lastCheckedAt: undefined` (or omit — it's optional).
- Files: `web-app/src/lib/vcs/adapters.ts`

##### Task 2.3.2e: Add adapter tests for the new optional/absent fields (~4 min)
- In `web-app/src/lib/vcs/adapters.test.ts`, add a test mirroring the existing
  historical-snapshot-absent-fields tests: construct a `VCSStatus`/`Session` pair with
  `commits`/`githubChecks`/`githubReviewFeedback` left at their proto zero-value (empty arrays)
  and assert `fromSessionVcs`/`fromSessionGithub` return empty arrays / `"unknown"` gracefully
  rather than throwing — the "old cached snapshot predates this feature" case pitfalls.md's
  proto-evolution finding calls out.
- Files: `web-app/src/lib/vcs/adapters.test.ts`

##### Task 2.3.2f: Update `ShippedCommit` import paths after its proto relocation (~2 min)
- Task 2.1.1b moves `ShippedCommit`/`ShippedCommitSchema` from
  `@/gen/session/v1/backlog_pb` to `@/gen/session/v1/types_pb`. Update:
  - `web-app/src/lib/vcs/adapters.ts:3` — split
    `import type { BacklogItemShipStatus, ShippedCommit, ShippedFileStat } from "@/gen/session/v1/backlog_pb";`
    into `import type { BacklogItemShipStatus, ShippedFileStat } from "@/gen/session/v1/backlog_pb";`
    and add `ShippedCommit` to the existing `@/gen/session/v1/types_pb` type-only import
    (adapters.ts:2).
  - `web-app/src/lib/vcs/adapters.test.ts:9` — same split for
    `ShippedCommitSchema` (moves to the existing `@/gen/session/v1/types_pb` import at line 8).
  - Confirmed via grep this is the full blast radius in `web-app/src` — no other file imports
    `ShippedCommit`/`ShippedCommitSchema`.
- Files: `web-app/src/lib/vcs/adapters.ts`, `web-app/src/lib/vcs/adapters.test.ts`

---

## Phase 3: Frontend "Why Blocked" Rollup Logic

### Epic 3.1: Multi-reason rollup derivation

**Goal**: Replace the single top-precedence pill's *information content* (not its existence —
`MergeabilityPill`/`deriveMergeabilityState` stay as the compact-mode/at-a-glance summary) with
a full enumeration of every currently-true blocking condition, GitLab-merge-checks style, per
requirements.md scope item 4 and intellij-vcs-deepdive.md §6's explicit endorsement of this
direction over JetBrains' own scattered/reactive model (which depends on an editable UI this
read-only panel doesn't have).

#### Story 3.1.1: `deriveBlockingReasons` — evaluate every predicate, don't short-circuit
**As a** VCS-tab viewer, **I want** to see every reason a PR is blocked at once, **so that** I
don't fix one issue only to discover another the pill didn't mention.
**Acceptance Criteria**: A PR that is simultaneously draft + changes-requested + CI-failing
returns all 3 reasons, not just the first. `deriveMergeabilityState` is unchanged and still
used for the compact-mode pill.
**Files**: `web-app/src/lib/vcs/mergeability.ts`, `web-app/src/lib/vcs/mergeability.test.ts`

##### Task 3.1.1a: Add `BlockingReason` type and `deriveBlockingReasons` (~5 min)
- In `web-app/src/lib/vcs/mergeability.ts`, add:
  ```ts
  export type BlockingReasonKey =
    | "draft"
    | "conflicted"
    | "github_diverged"
    | "changes_requested"
    | "ci_failing"
    | "ci_pending"
    | "closed_unshipped";

  export interface BlockingReason {
    key: BlockingReasonKey;
    label: string;
  }

  // Unlike deriveMergeabilityState (first-match precedence, for the compact-mode
  // pill), this evaluates every predicate independently — a PR that is
  // simultaneously draft + changes-requested + CI-failing surfaces all 3, not just
  // the first (requirements.md scope item 4).
  export function deriveBlockingReasons(data: VcsWidgetData): BlockingReason[] {
    if (data.shipped || !data.github) return [];
    const reasons: BlockingReason[] = [];
    if (data.github.isDraft) reasons.push({ key: "draft", label: "Draft" });
    if (data.fileChanges.some((f) => f.section === "conflict")) {
      reasons.push({ key: "conflicted", label: "Local merge conflicts" });
    }
    // Distinct from the local `conflicted` check above (see features.md (c).6):
    // GitHub's own mergeable computation catches base-branch divergence the local
    // worktree may not know about if it hasn't fetched/rebased.
    if (data.github.mergeable === "conflicting") {
      reasons.push({ key: "github_diverged", label: "Diverged from base branch" });
    }
    if (data.github.changesReqCount > 0) {
      reasons.push({ key: "changes_requested", label: `Changes requested (${data.github.changesReqCount})` });
    }
    if (data.github.checkConclusion === "failure") reasons.push({ key: "ci_failing", label: "Checks failing" });
    if (data.github.prState === "closed") reasons.push({ key: "closed_unshipped", label: "Closed — not merged" });
    if (data.github.checkConclusion === "pending" || data.github.checkConclusion === "") {
      reasons.push({ key: "ci_pending", label: "Checks running" });
    }
    return reasons;
  }
  ```
- Files: `web-app/src/lib/vcs/mergeability.ts`

##### Task 3.1.1b: Test the multi-reason case (~3 min)
- In `web-app/src/lib/vcs/mergeability.test.ts`, add
  `"deriveBlockingReasons_should_ReturnAllThreeReasons_When_DraftAndChangesRequestedAndCiFailingCoOccur"`
  — construct `VcsWidgetData` with `github.isDraft: true, changesReqCount: 1, checkConclusion: "failure"`
  and assert the returned array has exactly 3 entries with keys `draft`, `changes_requested`,
  `ci_failing` (order matches the function's evaluation order). Add a second test for the
  `shipped: true` short-circuit (empty array) and a third for `github_diverged` firing
  independently of local `conflicted`.
- Files: `web-app/src/lib/vcs/mergeability.test.ts`

---

## Phase 4: Frontend Layout Redesign (`VcsWidget` full mode)

Each story below depends on the Phase 2 proto field(s) named in its Acceptance Criteria having
already landed and `make proto-gen` having run.

### Epic 4.1: Aggregate diff-stat line in full mode

**Depends on**: Task 1.1.1c (`git.DiffStatBetween`) → Task 1.1.3b (branch-vs-base diff computed
server-side in `GetVCSStatus`) → Task 2.1.1a (`aggregate_diff_stat` proto field) → Task 2.2.1a
(Go→proto mapping) → Task 2.3.2b (`aggregateStats` populated by `fromSessionVcs` from the proto
field — not from working-tree `fileChanges`, per Blocker 2's fix). This chain is longer than the
original plan's single `Task 2.3.2b` dependency because the aggregate stat is now
server-computed rather than derived client-side.

#### Story 4.1.1: Render the aggregate line in full mode
**As a** VCS-tab viewer, **I want** a total files/+/- summary visible without scrolling the
file list, **so that** I can gauge change size at a glance, matching what compact mode already
shows.
**Acceptance Criteria**: The aggregate line renders in full mode when `aggregateStats` is
present; compact mode's existing rendering (and its pinned test,
`VcsWidget.test.tsx:114` "compact mode omits VcsWidgetGithubRow but shows the aggregate stat
line") is unchanged.
**Files**: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.1.1a: Add a full-mode aggregate-stat branch (~3 min)
- In `web-app/src/components/shared/VcsWidget.tsx`, next to the existing compact-only block
  (line 106-112), add a sibling block gated on `mode === "full"` instead of duplicating markup —
  factor the JSX into one line-rendering expression reused by both gates:
  ```tsx
  {(mode === "compact" || mode === "full") && data.aggregateStats && (
    <div className={styles.aggregateStatLine}>
      <span>{data.aggregateStats.filesChanged} files changed</span>
      <span className={styles.additions}>+{data.aggregateStats.additions}</span>
      <span className={styles.deletions}>-{data.aggregateStats.deletions}</span>
    </div>
  )}
  ```
  This is a strict superset of the existing compact-only condition, so it cannot regress the
  pinned compact-mode test — verify by running
  `cd web-app && npx jest VcsWidget.test.tsx --no-coverage` after the change.
- Files: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.1.1b: Add a full-mode aggregate-line test (~3 min)
- In `web-app/src/components/shared/VcsWidget.test.tsx`, add
  `"VcsWidget_should_RenderAggregateStatLine_When_FullModeWithAggregateStatsPresent"` —
  render with `mode="full"` and a populated `aggregateStats`, assert the line is present.
- Files: `web-app/src/components/shared/VcsWidget.test.tsx`

---

### Epic 4.2: Commit list — real data, HEAD decoration, truncation messaging

**Depends on**: Task 2.3.2a (`commits`/`commitsTruncated` populated).

#### Story 4.2.1: Wire real commits into `VcsWidgetCommitList` and cover full-mode rendering
**As a** VCS-tab viewer, **I want** to see the session's actual commit history, **so that** I
don't have to open GitHub to see what shipped.
**Acceptance Criteria**: `VcsWidgetCommitList` renders real data in full mode (pitfalls.md
flagged this component as never exercised with non-empty full-mode data); the existing
compact-mode 5-item cap test (`VcsWidget.test.tsx:138`) stays green.
**Files**: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.tsx`,
`web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.test.tsx`

##### Task 4.2.1a: No production-code change needed — verify wiring (~2 min)
- `VcsWidgetCommitList` already accepts `commits`/`mode` from `VcsWidget.tsx:118` unconditionally
  and already implements the 20-cap/"show all"/expand-per-row behavior (lines 39-58) — Phase 2
  wiring alone makes it render real data. This task is verification only: run
  `cd web-app && npx jest VcsWidgetCommitList.test.tsx --no-coverage` to confirm the existing
  suite still passes once `CommitSummary` data flows from real `fromSessionVcs` output.
- Files: none (verification task)

##### Task 4.2.1b: Add realistic full-mode tests (long messages, >20 commits) (~5 min)
- In `VcsWidgetCommitList.test.tsx`, add
  `"VcsWidgetCommitList_should_CapAtTwentyWithShowAllButton_When_FullModeWithTwentyFivCommits"`
  (25 synthetic commits, assert 20 visible + "Show all 25 commits" button, click it, assert all
  25 render) and
  `"VcsWidgetCommitList_should_TruncateLongSummaryUntilExpanded_When_CommitRowClicked"` (a
  200-character summary, assert the collapsed row doesn't overflow and the expanded row shows
  the full text) — per pitfalls.md's explicit call-out that full mode has never been exercised
  with realistic data before this feature.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.test.tsx`

##### Task 4.2.1c: Decorate the HEAD commit (~3 min)
- Per intellij-vcs-deepdive.md §2's "decorate, don't caption" recommendation: in
  `VcsWidgetCommitList.tsx`'s `CommitRow` (lines 15-37), accept an `isHead: boolean` prop and
  apply a bold/accent style (new `styles.headCommit` class, additive to the existing
  `.commitRow`/`.commitButton` styles) when true. In `VcsWidgetCommitList`'s render loop (line
  50), pass `isHead={i === 0}` (commits are newest-first per `ListShippedCommits`'s doc
  comment) only when `mode === "full"` and `commits[0]` — the HEAD commit is always the first
  entry in a newest-first list, so no separate SHA comparison is needed.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.tsx`,
  `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.css.ts`

#### Story 4.2.2: Truncation and unavailability messaging
**As a** VCS-tab viewer, **I want** to know when a commit list was cut off or failed to load,
**so that** "Show all N commits" doesn't falsely imply N is the true total, and a failed fetch
doesn't silently read as "this branch has zero commits."
**Acceptance Criteria**: When `commitsTruncated` is true, the "show all" affordance's copy
reflects that the shown count may not be the true total. When `commitsUnavailable` is true, the
commit list section renders a visible notice instead of nothing — even though `commits` is
empty in that case, which is exactly the silent-failure mode Concern 1 of the adversarial review
names (the component's existing `if (commits.length === 0) return null` swallows both "no
commits" and "fetch failed" identically without this fix).
**Files**: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.tsx`

##### Task 4.2.2a: Add a `truncated` prop and adjust "show all" copy (~3 min)
- Add `truncated?: boolean` to `VcsWidgetCommitListProps`; when true and `mode === "full"` and
  the full (uncapped, i.e. `showAll === true`) list is showing, render a small note below the
  list: `"Showing the {commits.length} most recent commits — there may be more."` instead of
  implying the count is exhaustive. Thread `data.commitsTruncated` through from
  `VcsWidget.tsx:118`'s existing `<VcsWidgetCommitList commits={data.commits} mode={mode} />`
  call (add `truncated={data.commitsTruncated}`).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.tsx`,
  `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.2.2b: Add an `unavailable` prop and render a notice instead of returning `null` (~3 min)
- Add `unavailable?: boolean` to `VcsWidgetCommitListProps`. Change the early-return guard from
  `if (commits.length === 0) return null;` to
  `if (commits.length === 0 && !unavailable) return null;`, and when `commits.length === 0 &&
  unavailable`, render a small notice in place of the (empty) list:
  `"Couldn't load commit history — try refreshing."` (reuse the existing neutral-notice styling
  pattern `VcsWidget.tsx`'s `styles.neutralNotice` already establishes, or a local equivalent in
  `VcsWidgetCommitList.css.ts`). Thread `data.commitsUnavailable` through from
  `VcsWidget.tsx:118`'s `<VcsWidgetCommitList ...>` call (add `unavailable={data.commitsUnavailable}`).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.tsx`,
  `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.css.ts`,
  `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.2.2c: Tests for truncation and unavailability notices (~3 min)
- In `VcsWidgetCommitList.test.tsx`, add
  `"VcsWidgetCommitList_should_RenderUnavailableNotice_When_CommitsEmptyAndUnavailableTrue"`
  (empty `commits`, `unavailable={true}`, assert the notice renders and the component does not
  return `null`) and a sibling test asserting `commits.length === 0 && unavailable === false`
  (the default, e.g. a genuinely commit-free branch) still returns `null` — i.e. the fix doesn't
  regress the existing benign-empty-state behavior.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.test.tsx`

---

### Epic 4.3: Itemized CI checks panel

**Depends on**: Task 2.3.2c (`github.checks` populated).

#### Story 4.3.1: `VcsWidgetCheckList` — one row per check, collapsed by default on all viewports
**As a** VCS-tab viewer, **I want** to see each CI check's name and conclusion (not just one
collapsed rollup), **so that** I know exactly which check is failing without leaving the app.
**Acceptance Criteria**: A new collapsible section lists every `github.checks` entry with a
status glyph; empty when `checks` is empty (old data / no checks configured).
**Files**: new `web-app/src/components/shared/vcs-widget/VcsWidgetCheckList.tsx` (+ `.css.ts`,
`.test.tsx`), `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.3.1a: Create `VcsWidgetCheckList` using `CollapsibleSection` (~5 min)
- New file `web-app/src/components/shared/vcs-widget/VcsWidgetCheckList.tsx`: accepts
  `checks: CheckItemSummary[]`, returns `null` if empty, otherwise renders a
  `<CollapsibleSection sectionKey="ci-checks" title={`Checks (${checks.length})`} defaultExpanded={false}>`
  (per Open Question 5's resolution — closed by default on all viewports) containing one row per
  check: a status glyph (reuse `ciClassName`-style mapping from `VcsWidgetGithubRow.tsx:22-31`,
  factored into a small shared helper if convenient, or duplicated once — this is the second
  usage, not yet a 3rd-usage abstraction trigger) plus `check.name`/`check.context`.
  `defaultExpanded={false}` here is set for standalone-usage clarity only — once Task 4.3.1c
  wraps this (and its two siblings) in a shared `CollapsibleGroup`, the group's own
  `defaultValue`/`value` is what actually drives initial open state (see
  `Collapsible.tsx:14-16,139-150`'s doc comments on this); this prop becomes a documentation-only
  no-op in grouped mode, not a bug.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCheckList.tsx`

##### Task 4.3.1b: Add `.css.ts` styles (~2 min)
- New file `web-app/src/components/shared/vcs-widget/VcsWidgetCheckList.css.ts`, mirroring
  `VcsWidgetGithubRow.css.ts`'s existing `ciSuccess`/`ciFailure`/`ciPending` token usage for
  consistency (per vim-fugitive-deepdive.md §4's "one canonical shape per entity, reused
  everywhere" — the same 3 status colors used for the rollup CI badge apply here).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCheckList.css.ts`

##### Task 4.3.1c: Wire into `VcsWidget.tsx` inside a shared `CollapsibleGroup` (full mode only) (~3 min)
- **Fixes**: Concern 3 of the adversarial review — three adjacent standalone
  `CollapsibleSection`s (checks here, review feedback in Task 4.4.1b, PR comments in Task
  4.6.2b) would each mount its own implicit single-item `Accordion.Root`, dropping the
  cross-header roving-tabindex keyboard nav `CollapsibleGroup` exists to provide for exactly
  this multi-sibling-section scenario (`Collapsible.tsx:6-10`, ADR-027).
- In `VcsWidget.tsx`, after the `VcsWidgetGithubRow` block (line 104), introduce the shared
  group and put the check list inside it:
  ```tsx
  {mode === "full" && (
    <CollapsibleGroup>
      <VcsWidgetCheckList checks={data.github?.checks ?? []} />
    </CollapsibleGroup>
  )}
  ```
  Import `CollapsibleGroup` from `@/components/ui/Collapsible`. Task 4.4.1b (review feedback)
  and Task 4.6.2b (PR comments) add their sections as additional children of this **same**
  `CollapsibleGroup` element — not new groups of their own — so the group keeps working with 2
  or 3 sections depending on how much of Epic 4.4/4.6 has landed (Open Question 1 leaves PR
  comments as the first thing cut if scope shrinks; this wiring stays correct either way).
- Files: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.3.1d: Tests (~4 min)
- New `VcsWidgetCheckList.test.tsx`: renders `null` for empty checks; renders N rows for N
  checks with correct status glyph per conclusion; the section starts collapsed
  (`aria-expanded="false"`) and expands on click, per Open Question 5.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetCheckList.test.tsx`

---

### Epic 4.4: Reviewer feedback panel (safe rendering)

**Depends on**: Task 2.3.2c (`github.reviewFeedback` populated).

#### Story 4.4.1: `VcsWidgetReviewFeedback` — plain-text body rendering, no `dangerouslySetInnerHTML`
**As a** VCS-tab viewer, **I want** to read a reviewer's stated reason for requesting changes,
**so that** I know what to fix without opening GitHub.
**Acceptance Criteria**: Review body text renders via ordinary JSX text interpolation (React's
default escaping) — never `dangerouslySetInnerHTML`, per pitfalls.md §6's explicit warning
about the nearby `SessionCard.tsx:993` pattern being miscopy-able.
**Files**: new `web-app/src/components/shared/vcs-widget/VcsWidgetReviewFeedback.tsx` (+
`.css.ts`, `.test.tsx`), `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.4.1a: Create `VcsWidgetReviewFeedback` (~4 min)
- New file: accepts `reviewFeedback: ReviewFeedbackSummary[]`, returns `null` if empty
  (or if every entry has `state !== "CHANGES_REQUESTED"` — only surface the reviews that
  actually explain a block, not every APPROVED review's body). Renders a
  `<CollapsibleSection sectionKey="review-feedback" title="Review feedback" defaultExpanded={false}>`
  containing, per changes-requested review, the author name and `<p>{review.body}</p>` — plain
  JSX text interpolation only, explicitly **not** `dangerouslySetInnerHTML`. Add a one-line code
  comment stating why (mirrors the doc-comment style at `SessionCard.tsx:993` but for the
  opposite choice): `{/* Plain JSX text interpolation — auto-escaped, XSS-safe. Do not switch to dangerouslySetInnerHTML for Markdown rendering without a sanitizing renderer (see pitfalls.md §6). */}`.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetReviewFeedback.tsx`

##### Task 4.4.1b: Add `.css.ts` and wire into the shared `CollapsibleGroup` from Task 4.3.1c (~3 min)
- New `.css.ts` for basic spacing/typography. In `VcsWidget.tsx`, add `<VcsWidgetReviewFeedback
  reviewFeedback={data.github?.reviewFeedback ?? []} />` as a **second child inside the same
  `CollapsibleGroup`** Task 4.3.1c introduced (not a new standalone `CollapsibleSection` and not
  a second group) — per Concern 3 of the adversarial review, sibling collapsible sections must
  share one `Accordion.Root` to get cross-header keyboard nav:
  ```tsx
  {mode === "full" && (
    <CollapsibleGroup>
      <VcsWidgetCheckList checks={data.github?.checks ?? []} />
      <VcsWidgetReviewFeedback reviewFeedback={data.github?.reviewFeedback ?? []} />
    </CollapsibleGroup>
  )}
  ```
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetReviewFeedback.css.ts`,
  `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.4.1c: Tests, including an explicit XSS-safety assertion (~4 min)
- New `.test.tsx`: renders `null` for empty/no-changes-requested feedback; renders review body
  text for a CHANGES_REQUESTED entry; a dedicated test
  `"VcsWidgetReviewFeedback_should_RenderRawHtmlAsLiteralText_When_ReviewBodyContainsHtmlTags"`
  passes a body containing `"<img src=x onerror=alert(1)>"` and asserts the DOM contains the
  literal escaped text, not an actual `<img>` element (`container.querySelector("img")` is
  `null`) — makes the safe-rendering choice from pitfalls.md §6 a regression-tested guarantee,
  not just a comment.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetReviewFeedback.test.tsx`

---

### Epic 4.5: Why-blocked rollup UI + staleness indicators

**Depends on**: Task 3.1.1a (`deriveBlockingReasons`), Task 2.3.2c (`github.mergeable`), Task
1.1.3c/2.3.2a (`statusAsOf`), Task 2.3.2c (`lastCheckedAt`).

#### Story 4.5.1: Render the all-reasons rollup alongside the existing pill, bound to its own staleness
**As a** VCS-tab viewer, **I want** every blocking reason listed, not just one, and I want to
know when those reasons might be out of date, **so that** I can answer "is this ready to merge,
and if not, exactly why" from this tab alone (requirements.md's Success Metric) without being
misled by a poll that's been silently failing.
**Acceptance Criteria**: In full mode, a new rollup list renders below `MergeabilityPill`,
showing every `deriveBlockingReasons` entry with distinct, named copy (not one collapsed
string), per intellij-vcs-deepdive.md §6. The pill itself is unchanged (still first-precedence,
still used by compact mode). **The rollup itself carries a staleness signal tied to
`github.lastCheckedAt`** — not just a sibling timestamp label elsewhere in the layout (Task
4.5.2a) — so a user can't see e.g. "Changes requested (1)" in the always-visible rollup with no
indication the underlying GitHub poll has been failing for hours (Concern 2 in the adversarial
review this revision addresses). Task 4.5.2a's separate "PR status confirmed Xs ago" label still
exists too — the two are complementary, not a replacement for each other.
**Files**: new `web-app/src/components/shared/vcs-widget/VcsWidgetBlockingReasons.tsx` (+
`.css.ts`, `.test.tsx`), `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.5.1a: Create `VcsWidgetBlockingReasons`, self-flagging when its data is stale (~5 min)
- New file: accepts `reasons: BlockingReason[]` and `lastCheckedAt?: Date`. Returns `null` if
  `reasons` is empty (independent of staleness — an empty rollup has nothing to caveat).
  Otherwise renders an always-visible (not collapsible — per Open Question 5, only the *detail*
  sections collapse; the rollup itself is a primary signal) `<ul>` with one `<li>` per reason,
  each showing `reason.label`. No icon-per-reason needed initially — keep it terse per
  vim-fugitive-deepdive.md §5(d).
- Staleness binding: compute
  `const isStale = lastCheckedAt !== undefined && Date.now() - lastCheckedAt.getTime() > STALE_THRESHOLD_MS;`
  with `const STALE_THRESHOLD_MS = 3 * 60 * 1000; // 3x PRStatusPoller's 60s cadence (session/pr_status_poller.go:41) — tolerates a couple of missed ticks before flagging, so ordinary poll jitter doesn't flicker the notice.`
  When `isStale`, render an inline notice as part of the same rollup markup (not a separate
  sibling element), e.g. a leading `<li className={styles.staleNotice} data-testid="blocking-reasons-stale">These
  reasons may be out of date — PR status hasn't refreshed recently.</li>` before the reason
  list, or an equivalent `data-stale="true"` attribute on the container plus a visible badge —
  either way, the stale signal must live inside `VcsWidgetBlockingReasons`'s own render output,
  not solely in `VcsWidget.tsx`'s separate timestamp label.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetBlockingReasons.tsx`

##### Task 4.5.1b: `.css.ts` and wiring (~2 min)
- New `.css.ts`. In `VcsWidget.tsx`, inside the `controlsRow`'s live region (near line 56-58,
  alongside `MergeabilityPill`), add
  `{mode === "full" && <VcsWidgetBlockingReasons reasons={deriveBlockingReasons(data)} lastCheckedAt={data.github?.lastCheckedAt} />}`
  (import `deriveBlockingReasons` from `@/lib/vcs/mergeability`).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetBlockingReasons.css.ts`,
  `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.5.1c: Tests, including the staleness binding (~4 min)
- New `.test.tsx`: renders `null` for an empty reasons array (even with a stale/absent
  `lastCheckedAt`); renders N `<li>` for N reasons in order; a dedicated
  `"VcsWidgetBlockingReasons_should_RenderStaleNotice_When_LastCheckedAtExceedsThreshold"` test
  passing a `lastCheckedAt` older than `STALE_THRESHOLD_MS` alongside a non-empty `reasons`
  array and asserting the stale notice renders; a sibling test with a recent `lastCheckedAt`
  asserting it does not.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetBlockingReasons.test.tsx`

#### Story 4.5.2: Dual staleness labels
**As a** VCS-tab viewer, **I want** to know separately how fresh the local git status and the
GitHub PR status are, **so that** I don't mistake a 47-second-old CI result for a live one.
**Acceptance Criteria**: Two small labels render (not one blended timestamp) — "Local status:
as of Xs ago" and "PR status: as of Ys ago" — matching Open Question 3's resolution.
**Files**: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.5.2a: Add two staleness labels in full mode (~4 min)
- In `VcsWidget.tsx`, near the existing `snapshotAt` rendering (lines 60-64, historical-only),
  add a full-mode-only, live-only pair of labels using the already-imported
  `formatRelativeTime`:
  ```tsx
  {mode === "full" && data.kind === "live" && data.statusAsOf && (
    <span className={styles.snapshotTimestamp}>Local: {formatRelativeTime(data.statusAsOf.getTime())}</span>
  )}
  {mode === "full" && data.kind === "live" && data.github?.lastCheckedAt && (
    <span className={styles.snapshotTimestamp}>PR status confirmed {formatRelativeTime(data.github.lastCheckedAt.getTime())}</span>
  )}
  ```
  "PR status confirmed", not "checks updated" — per Open Question 3's copy requirement (a 304
  response bumps this timestamp without itemized checks necessarily having changed).
- Files: `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.5.2b: Test both labels render independently (~3 min)
- In `VcsWidget.test.tsx`, add a test asserting both labels render when both timestamps are
  present, and that omitting one (e.g. `statusAsOf` undefined, an old cached session) doesn't
  hide the other.
- Files: `web-app/src/components/shared/VcsWidget.test.tsx`

---

### Epic 4.6: Bundled wins + PR comments

#### Story 4.6.1: Wire `worktreePath`/`onBrowseFiles` in `VcsPanel` (bundled free scope)
**As a** VCS-tab viewer, **I want** the worktree-path row and browse-files button that already
work in `full` mode, **so that** I get this capability for free since `VcsWidget` already
supports it.
**Acceptance Criteria**: `VcsPanel` passes `worktreePath` and `onBrowseFiles`; the existing
`VcsWidgetHeader` renders the copy/browse row (already-tested component, `VcsWidgetHeader.tsx:66-88`).
**Files**: `web-app/src/components/sessions/VcsPanel.tsx`

##### Task 4.6.1a: Pass `worktreePath`/`onBrowseFiles` from `VcsPanel` (~3 min)
- In `web-app/src/components/sessions/VcsPanel.tsx`, `VcsPanelProps` needs a `worktreePath`
  and an `onBrowseFiles` callback threaded from whatever parent already knows the session's
  effective path (check `SessionDetail`'s existing props for a pattern — the Files tab
  presumably already has this path). Pass both to `<VcsWidget .../>` (line 64-69).
- Files: `web-app/src/components/sessions/VcsPanel.tsx` (and its parent call site, wherever
  `<VcsPanel>` is instantiated — locate via `Grep` for `<VcsPanel` before editing)

#### Story 4.6.2: PR comments section (Open Question 1 — in scope, collapsed by default)
**As a** VCS-tab viewer, **I want** to read inline PR review comments, **so that** I don't have
to open GitHub for line-level feedback beyond the review-body text already shown.
**Acceptance Criteria**: A new collapsed-by-default section fetches and lists `GetPRComments`
results when expanded (lazy — not part of the poller payload, since this is genuinely optional
detail, unlike the checks/reviews decided in Open Question 2).
**Files**: new `web-app/src/components/shared/vcs-widget/VcsWidgetComments.tsx` (+ `.css.ts`,
`.test.tsx`), `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.6.2a: Create `VcsWidgetComments` with lazy fetch on expand (~5 min)
- New file: accepts `owner: string; repo: string; prNumber: number`. Renders a
  `<CollapsibleSection sectionKey="pr-comments" title="PR comments" defaultExpanded={false} onExpandedChange={...}>`
  that calls the existing `GetPRComments` RPC (via the same ConnectRPC client pattern
  `useSessionVcs.ts` uses) only on first expand, caching the result in local state — never on
  mount, since most sessions' tabs won't have this section opened.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetComments.tsx`

##### Task 4.6.2b: `.css.ts` and wiring into the shared `CollapsibleGroup` (~2 min)
- In `VcsWidget.tsx`, add `VcsWidgetComments` as a **third child of the same `CollapsibleGroup`**
  Tasks 4.3.1c/4.4.1b already populated — per Concern 3 of the adversarial review, all three
  sibling sections (checks, review feedback, PR comments) must share one `Accordion.Root` for
  cross-header keyboard nav, not each mount its own:
  ```tsx
  {mode === "full" && (
    <CollapsibleGroup>
      <VcsWidgetCheckList checks={data.github?.checks ?? []} />
      <VcsWidgetReviewFeedback reviewFeedback={data.github?.reviewFeedback ?? []} />
      {data.github && (
        <VcsWidgetComments owner={data.github.owner} repo={data.github.repo} prNumber={data.github.prNumber} />
      )}
    </CollapsibleGroup>
  )}
  ```
  If Epic 4.6 (PR comments) is cut per Open Question 1's scope-shrink note, the group still
  works correctly with just its first two children — no wiring change needed elsewhere.
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetComments.css.ts`,
  `web-app/src/components/shared/VcsWidget.tsx`

##### Task 4.6.2c: Tests (~4 min)
- New `.test.tsx`: section starts collapsed and makes no RPC call; expanding triggers exactly
  one `GetPRComments` call (mock the client); re-collapsing and re-expanding does not
  re-fetch (cached).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetComments.test.tsx`

##### Task 4.6.2d: Test the composed `CollapsibleGroup` actually starts all-collapsed (validation.md Gap 4) (~4 min)
- **Fixes**: validation.md's Gap 4 — Task 4.3.1d's `VcsWidgetCheckList`-standalone test only
  proves the *subcomponent* starts collapsed in isolation. Task 4.3.1c's own doc comment notes
  that once wrapped in the real `CollapsibleGroup` composition (this task, 4.6.2b), a child's own
  `defaultExpanded` prop becomes a documentation-only no-op — the group's own initial-state
  handling is what actually drives it once composed. Nothing exercised that composed behavior,
  which is the one gap that risks silently shipping the opposite of Open Question 5's
  default-closed resolution.
- In `web-app/src/components/shared/VcsWidget.test.tsx`, add
  `VcsWidget_should_RenderAllDisclosureSectionsCollapsed_When_FullModeWithChecksReviewsAndCommentsPresent`:
  render the full `VcsWidget` in `mode="full"` with `data.github.checks`/`reviewFeedback`
  populated and PR info present (so all three `CollapsibleGroup` children — checks, review
  feedback, PR comments — mount), and assert every section header's `aria-expanded` is
  `"false"` on initial render, on both desktop and the narrow-viewport case Open Question 5
  covers (this component doesn't branch on viewport per OQ5's resolution — uniform
  closed-by-default — so a single assertion covers both, but state that explicitly in the test
  name/comment rather than leaving it implicit).
- Files: `web-app/src/components/shared/VcsWidget.test.tsx`

---

## Phase 5: Tests & Regression Guard

### Epic 5.1: Full-mode coverage audit + compact-mode regression verification

**Goal**: Confirm every compact-mode guardrail pitfalls.md names is still green, and that the
full-mode surface added across Phase 4 has equivalent coverage.

#### Story 5.1.1: Run and confirm the existing compact-mode regression guard
**Acceptance Criteria**: All 5 tests pitfalls.md names by exact string still pass unmodified.
**Files**: `web-app/src/components/shared/VcsWidget.test.tsx`

##### Task 5.1.1a: Run the named regression-guard tests explicitly (~2 min)
- Run `cd web-app && npx jest VcsWidget.test.tsx --no-coverage -t "compact mode"` and confirm
  all of: `"compact mode never renders per-file rows even when fileChanges is populated"`
  (line 91), `"compact mode omits VcsWidgetGithubRow but shows the aggregate stat line"` (line
  114), `"caps compact-mode commit list at 5 entries"` (line 138), `"omits the View Diff
  affordance in compact mode even when onViewDiff is provided"` (line 223), and
  `"VcsWidget_should_OmitBrowseFilesButton_When_CompactModeEvenWithOnBrowseFiles"` (line 246)
  pass unmodified. If any fail, the responsible Phase 4 task's diff is the regression — fix
  there, not by loosening these tests (per this project's constraint: no regression to
  compact-mode ship-status views).
- Files: none (verification task)

#### Story 5.1.2: Backend test coverage for the new Go surface
**Acceptance Criteria**: New Go behavior (ctx-bounded `ListShippedCommits` **including an
actually-exercised `truncated == true` case**, `git.DiffStatBetween` **with its own dedicated
unit test, not just indirect integration coverage (round-2 review concern)**,
`Instance.GetBaseCommitSHA` for **both** session types, `PRStatusUpdate`, `GetVCSStatus`'s
commit-list/aggregate-diff-stat wiring, **and `CommitsUnavailable` firing on both a HEAD-SHA
resolution failure and a `ListShippedCommits` failure (round-2 review concern)**) has passing
unit coverage.
**Files**: `session/git/ops_test.go`, `session/instance_terminal_test.go`,
`server/services/workspace_service_test.go`

##### Task 5.1.2a: Test `ListShippedCommits`'s truncation via the injectable cap — no hedge (~4 min)
- **Fixes**: Concern 5 of the adversarial review — the prior version of this task hedged
  ("otherwise assert `truncated == false` ... and rely on the cap's existing coverage, if any,
  for the true-case"), which permitted shipping without ever exercising `truncated == true`.
  Task 1.1.1a's `listShippedCommitsWithCap(ctx, repoPath, baseSHA, headSHA string, cap int)`
  extraction makes this directly testable without a 100+-commit fixture.
- In `session/git/ops_test.go`, add
  `TestListShippedCommits_should_ReportTruncatedTrue_When_CommitCountExceedsCap`: build a small
  fixture repo with e.g. 5 unshipped commits (reuse the existing fixture-building helper the
  file's other `TestListShippedCommits_*` tests already use), call
  `listShippedCommitsWithCap(ctx, work, baseSHA, headSHA, 3)` directly with a small injected
  cap, and assert `len(commits) == 3` and `truncated == true`. Add a second assertion (or reuse
  the existing `TestListShippedCommits_should_ReturnNewestFirst_When_MultipleCommitsShipped`
  test) confirming `truncated == false` when the commit count is under the cap — this test
  already exists and needs no change, just confirm it still passes through the new
  `ListShippedCommits` → `listShippedCommitsWithCap` delegation.
- Files: `session/git/ops_test.go`

##### Task 5.1.2b: Test `Instance.GetBaseCommitSHA` for both session types (~3 min)
- In `session/instance_terminal_test.go` (or the nearest existing worktree-fixture test file),
  add a test asserting it returns the recorded SHA for a worktree session (via
  `GitWorktreeManager.SetWorktree`/the worktree's own base SHA), a second asserting it returns
  the recorded SHA for a **directory** session (via `GitWorktreeManager.SetDirBaseSHA`, no
  worktree set), and a third asserting `""` when neither is set — the directory-session case is
  the one Blocker 1 of the adversarial review found untested and broken.
- Files: `session/instance_terminal_test.go`

##### Task 5.1.2c: Test `GetVCSStatus`'s commit-list and aggregate-diff-stat population (~5 min)
- In `server/services/workspace_service_test.go`, add a test with a fixture git repo + a
  **directory-mode** session with a recorded base SHA behind HEAD (per Blocker 1 — this is the
  default session type and was the case the original plan never actually tested), asserting the
  `GetVCSStatusResponse`'s `VcsStatus.Commits` is non-empty, `VcsStatus.AggregateDiffStat` is set
  with non-zero `FilesChanged`, and `StatusAsOf` is set. Add a worktree-mode equivalent for
  parity. A separate test with no recorded base SHA asserts `Commits`/`AggregateDiffStat` stay
  unset without erroring (the non-fatal path from Task 1.1.3b), and
  `CommitsUnavailable`/`AggregateDiffStat` are both zero-valued in that case (not conflated with
  a fetch failure). A fourth test asserts `CommitsUnavailable == true` (with `Commits`/
  `AggregateDiffStat` both left unset) when `GetHeadCommitSHA` itself fails — e.g. point
  `workDir` at a path with no `.git` — confirming the round-2 review's HEAD-resolution-failure
  fix in Task 1.1.3b is exercised, not just the downstream `ListShippedCommits`-failure branch.
  A fifth assertion (validation.md Gap 3) calls `GetVCSStatus` twice within the 15s cache
  window and asserts the second response's `StatusAsOf` equals the first's exact value, not
  `time.Now()` at the second call — Task 1.1.3c's acceptance criteria names both the cache-hit
  and fresh-compute branches, but the original test only exercised fresh-compute.
- Files: `server/services/workspace_service_test.go`

##### Task 5.1.2d: Add a dedicated `TestDiffStatBetween` unit test (~4 min)
- **Fixes**: round-2 adversarial review concern — `git.DiffStatBetween` (Task 1.1.1c) shipped
  with no dedicated unit test, only indirect coverage via Task 5.1.2c's `WorkspaceService`
  integration test, which can't isolate a summation bug from a wiring bug.
- In `session/git/ops_test.go`, add a table-driven `TestDiffStatBetween` alongside the existing
  `ListShippedCommits`/`FileStatsBetween` tests, reusing the same fixture-repo helper, covering:
  additions-only, deletions-only, mixed additions+deletions across multiple files (asserting
  `FilesChanged` matches file count, not commit count), `baseSHA == headSHA` (zero-diff, all
  fields 0), and an invalid/unresolvable SHA (asserts the `FileStatsBetween` error passes
  through unchanged, per `DiffStatBetween`'s error-passthrough contract in Task 1.1.1c).
- Files: `session/git/ops_test.go`

#### Story 5.1.4: Backend test coverage for itemized checks/review feedback (validation.md Gap 1)
**As a** backend developer, **I want** the new `Checks`/`Reviews`/`Mergeable` plumbing added in
Epics 1.2/2.2 covered by tests in the same files that already test their surrounding functions,
**so that** the itemized-CI-checks and reviewer-body-text features (Scope-3, Scope-5, OQ2) have
actual backend coverage, not just frontend component tests exercising hand-built fixture data.
**Acceptance Criteria**: `GetPRInfoCtx`'s `Checks`/`Reviews` population, `checksToProto`/
`reviewFeedbackToProto`'s mapping, and `applyPRUpdate`'s threading of the 3 new fields into
`PRStatusUpdate` each have a passing unit test in their file's existing test suite.
**Files**: `github/client_pr_by_number_test.go`, `server/adapters/instance_adapter_test.go`,
`session/pr_status_poller_test.go`

##### Task 5.1.4a: Test `GetPRInfoCtx` populates `Checks`/`Reviews` from the mocked `gh` response (~4 min)
- In `github/client_pr_by_number_test.go`, alongside the existing
  `TestGetPRByNumber_should_ReturnPRInfo_When_PRExists`-style tests, add
  `TestGetPRInfoCtx_should_PopulateChecksAndReviews_When_StatusCheckRollupAndReviewsPresent`:
  build a mocked `gh` JSON response with a multi-entry `statusCheckRollup` and a
  `CHANGES_REQUESTED` review with a non-empty `body`, call `GetPRInfoCtx`, and assert
  `PRInfo.Checks`/`PRInfo.Reviews` contain the expected itemized entries (name/context/
  state/conclusion for checks; author/state/body for reviews) alongside the still-correct
  collapsed `CheckConclusion`/review counts from `getCheckConclusion`/`parseReviewCounts`
  (Task 1.2.1b's acceptance criteria: itemized and collapsed fields both populated, not one
  replacing the other).
- Files: `github/client_pr_by_number_test.go`

##### Task 5.1.4b: Test `checksToProto`/`reviewFeedbackToProto` mapping (~3 min)
- In `server/adapters/instance_adapter_test.go`, following the existing `TestInstanceToProto_*`
  naming convention, add `TestInstanceToProto_should_MapChecksAndReviewFeedback_When_Populated`:
  construct a `PRInfo` with populated `Checks`/`Reviews`, run it through the mapping added in
  Task 2.2.2a, and assert the resulting proto's itemized check/review-feedback fields match
  field-for-field.
- Files: `server/adapters/instance_adapter_test.go`

##### Task 5.1.4c: Test `applyPRUpdate` threads `Checks`/`Reviews`/`Mergeable` into `PRStatusUpdate` (~3 min)
- In `session/pr_status_poller_test.go`, alongside the existing
  `TestApplyPRUpdate_FiresOnUpdated_WhenCheckConclusionChangesWithoutPriorityChange`, add
  `TestApplyPRUpdate_should_ThreadChecksReviewsMergeable_When_PRInfoPopulated`: feed a `PRInfo`
  with non-empty `Checks`/`Reviews` and a `Mergeable` value through `applyPRUpdate`, and assert
  the resulting `PRStatusUpdate` (Epic 1.2's struct refactor) carries all 3 fields unchanged —
  confirming Task 1.2.2c's threading survives the round trip, since it's the one hop between
  `github.PRInfo` and the `Instance`-level fields the poller and rollup ultimately read.
- Files: `session/pr_status_poller_test.go`

#### Story 5.1.3: Full `make quick-check` pass
**Acceptance Criteria**: `make quick-check` (build + test + lint) and `cd web-app && npx jest --no-coverage`
both pass clean before this feature is considered shippable.
**Files**: none

##### Task 5.1.3a: Run `make quick-check` and the web-app test suite, fix any fallout (~5 min, or more if fallout is found)
- Run `make quick-check` from repo root, then `cd web-app && npx jest --no-coverage`. Fix any
  failures surfaced (most likely: a missed call site for `ListShippedCommits`'s new signature,
  or a TS compile error from `GithubSummary`'s new required fields not being set in a test
  fixture — grep `web-app/src/**/*.test.tsx` for `GithubSummary` / hand-built `github:` object
  literals if `tsc` flags missing fields).
- Files: whatever the fallout points to
