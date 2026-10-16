# ADR-001: Bound and cache the new live-session go-git calls in place — don't promote `GoGitVCSReader`

**Date**: 2026-08-27
**Status**: Accepted
**Context project**: vcs-tab-redesign

## Context

The redesign needs to call `git.ListShippedCommits` (`session/git/ops.go:369`) from the
live-session VCS-status path (`WorkspaceService.GetVCSStatus`,
`server/services/workspace_service.go:132`) to populate a commit list — a call site that does
not exist today. `session/git/ops.go`'s functions each open a fresh `*git.Repository` per call
with no per-repo mutex, no result cache, and (per `.claude/skills/code-go-git`/pitfalls.md) no
`context.Context`/timeout. A second implementation, `session/unfinished/gogit_vcs_reader.go`'s
`GoGitVCSReader`, already has a hardened per-repo-mutex + `singleflight` + 30s-TTL-cache layer,
but it lives in a package named `unfinished`, is scoped to the backlog scanner's use cases
(`AheadBehind`, `CommitMessages`, `DiffShortstat`, `HasUncommitted`), and has no
`ListShippedCommits`-equivalent returning full commit metadata (SHA/summary/author/date).

Two options were on the table:
1. Promote/generalize `GoGitVCSReader` out of `session/unfinished` (or backport its
   mutex+singleflight+TTL pattern into `session/git/ops.go`) so the new live-session commit-list
   call goes through hardened, cross-call-coalescing infrastructure.
2. Call `session/git.ListShippedCommits` directly from `WorkspaceService.GetVCSStatus`, add a
   bounded `context.Context` timeout to the new call (matching `FetchBranch`'s existing pattern),
   and rely on `WorkspaceService`'s own already-existing 15s `vcsStatusCache`
   (`workspace_service.go:56-60,152-160`) — which already wraps the exact code path being
   extended — to amortize repeat calls for the same working directory.

## Decision

**Option 2.** Extend `vc.VCSStatus` with the new commit-list fields, populate them inside
`WorkspaceService.GetVCSStatus` using `session/git.ListShippedCommits` directly, add a
`context.Context` parameter with a bounded timeout to `ListShippedCommits` (mirroring
`FetchBranch`'s 30s-timeout shape), and do **not** attempt to promote or generalize
`GoGitVCSReader` as part of this feature.

## Rationale

- **The caching gap this redesign actually has is already closed by an existing cache at the
  right scope.** `vcsStatusCacheTTL` (15s, `workspace_service.go:60`) wraps the entire
  `GetVCSStatus` computation per `workDir` — the new commit-list call, once folded into that same
  function before the cache is populated, is amortized by the *same* cache with zero new
  infrastructure. `GoGitVCSReader`'s TTL cache exists to solve the same class of problem
  (redundant packfile-index parses across concurrent callers) for a *different* call path (the
  backlog scanner, which has no equivalent per-workDir cache in front of it) — this feature does
  not inherit that gap.
- **No MUST-FIX crash risk applies.** Per `code-go-git`/pitfalls.md, the go-git concurrent-map
  crash (issue #1121) requires concurrent calls sharing one cached `*git.Repository`.
  `ListShippedCommits` opens a fresh, unshared `*git.Repository` per call — safe as-is; the cost
  is redundant work, not a correctness bug, and the 15s cache already bounds that cost for this
  path's actual call pattern (repeated polls of the same session's VCS tab).
- **Promoting `GoGitVCSReader` is real, separately-scoped surgery.** Per architecture.md §(c),
  this requires either renaming/relocating a package literally called `unfinished` (a
  cross-cutting rename with its own blast radius) or duplicating its locking pattern into
  `session/git/ops.go` (introducing a second cache with its own invalidation story, cutting
  against "avoid redundant GitHub API calls or redundant go-git traversals" only by adding a
  second layer where a first one — the existing 15s cache — already suffices for this feature's
  actual load pattern). Neither is required to ship a read-only status panel; both are legitimate
  follow-ups if go-git index-parse cost is later measured as an actual hotspot (the
  `code-hotspot-analysis` skill / `tstapler/kibitzer#15`'s territory), not a prerequisite for it.
- **The torn-read race during an actively-committing agent is mitigated by an existing, narrower
  fix, not by caching.** `git.GetHeadCommitSHA` (`session/git/util.go:281`) already has the
  documented CLI fallback for a ref-read race — using it (rather than a raw go-git `repo.Head()`
  call) to resolve the live HEAD SHA for `ListShippedCommits`'s `headSHA` argument is the correct,
  targeted mitigation; a shared-repo cache would not have prevented this race either, since the
  race is about a write landing mid-read, not about redundant reads.

## Consequences

- `ListShippedCommits(ctx context.Context, repoPath, baseSHA, headSHA string) ([]ShippedCommit, bool, error)`
  gains a `context.Context` parameter and a `truncated bool` return value (see plan.md Task
  1.1.1a); its one existing caller (`server/services/backlog_service_ship_status.go:131`) is
  updated to pass `context.Background()` and discard the new return value, since ship-status
  truncation messaging is out of scope for this feature.
- `WorkspaceService.GetVCSStatus` becomes the sole live-session call site for
  `ListShippedCommits`; no new mutex, singleflight group, or TTL cache is introduced in this
  feature.
- **Update (adversarial-review pass):** the "acknowledged follow-up path" mentioned below —
  a real branch-vs-base diff stat for the live-session aggregate line — is implemented in this
  iteration, not deferred. It follows the exact same Option 2 pattern rather than opening a new
  decision: `git.DiffStatBetween(ctx, repoPath, baseSHA, headSHA)` (plan.md Task 1.1.1c) is a new
  `session/git` function, ctx-bounded like `ListShippedCommits`, called from the same
  `WorkspaceService.GetVCSStatus` code path and covered by the same 15s `vcsStatusCache` — not a
  route through `GoGitVCSReader.DiffShortstat`, and not a new cache. It wraps the existing
  `FileStatsBetween` (summing per-file counts) rather than adding a second go-git diff
  implementation, so it introduces no new go-git traversal logic. This was necessary because the
  original plan sourced the aggregate stat from client-side working-tree file-change lists
  (`VCSStatus.staged_files`/etc.), which go empty exactly when a session is clean and PR-ready —
  the review's Blocker 2.
- If a future profiling pass shows this call site is a measurable hotspot at the scale of "many
  concurrent VCS-tab opens against one shared physical repo," the fix is still available as a
  follow-up: either raise `session/git/ops.go` to `GoGitVCSReader`'s hardening level, or route
  this call through a promoted/relocated `GoGitVCSReader`. Neither is blocked by this decision.
