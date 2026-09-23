# Adversarial Review: vcs-tab-redesign

**Date**: 2026-08-27
**Round**: 3 (both round-2 concerns patched directly, in-line, by the SDD coordinator)
**Verdict**: CLEAN

## Round 3 — Both Round-2 Concerns Closed

Both round-2 concerns were structural gaps in `plan.md`'s task text, not open design questions,
so they were patched directly rather than re-dispatching a full planner pass:

- **`CommitsUnavailable` / `GetHeadCommitSHA` failure path**: Task 1.1.3b's proposed code now
  branches explicitly on `headErr != nil` first and sets `status.CommitsUnavailable = true` (with
  a debug log) before falling through to the `headSHA != baseSHA` check — the HEAD-resolution
  failure path no longer silently reproduces the empty-vs-failed ambiguity `CommitsUnavailable`
  exists to remove. Task 5.1.2c gained a fourth sub-case (`CommitsUnavailable == true` when
  `GetHeadCommitSHA` fails, e.g. `workDir` with no `.git`) and Story 5.1.2's acceptance criteria
  now name this explicitly.
- **No dedicated `DiffStatBetween` unit test**: new Task 5.1.2d adds a table-driven
  `TestDiffStatBetween` in `session/git/ops_test.go` (additions-only, deletions-only, mixed,
  zero-diff, error-passthrough), independent of Task 5.1.2c's integration-level coverage. Story
  5.1.2's acceptance criteria and Task 1.1.1c both reference it.
- Also fixed in passing (round-2's MINOR citation note): Task 1.1.1c's "3 other call sites" claim
  for `FileStatsBetween` corrected to the verified 2
  (`backlog_service_triage.go`, `backlog_lifecycle_pr.go`) — `backlog_service_ship_status.go`
  only mentions the function in a doc comment, confirmed via the round-2 grep.

No new blockers or concerns introduced by this patch — the changes are additive (new branch, new
test task) and don't touch the round-1/round-2-verified control flow elsewhere in Task 1.1.3b.
The remaining round-2 MINORs (a few off-by-1/2 line-number citations, and `FileStatsBetween`'s
pre-existing binary-file-omission behavior surfacing in a new UI location) are cosmetic and
carried forward unresolved — low enough impact not to warrant another patch pass.

---

## Round 2 (for reference — superseded by Round 3 above for the 2 concerns it raised)

**Verdict at Round 2**: CONCERNS

Round 1 found 2 BLOCKERs and 5 CONCERNs. This pass re-verified each claimed fix against the
actual source (`session/git_worktree_manager.go`, `session/instance_worktree.go`,
`session/git/ops.go`, `github/client.go`, `session/instance_terminal.go`,
`session/pr_status_poller.go`, `session/storage.go`, `session/instance.go` /
`instance_snapshot.go`, `server/services/workspace_service.go`,
`server/adapters/instance_adapter.go`, `proto/session/v1/types.proto` /
`backlog.proto`, `web-app/src/components/ui/Collapsible.tsx`,
`web-app/src/lib/vcs/adapters.ts`, `web-app/src/components/shared/VcsWidget.tsx`,
`web-app/src/components/shared/vcs-widget/VcsWidgetCommitList.tsx`) — not just the plan prose —
and independently re-checked the ~6 new / ~20 touched tasks for new issues. Both blockers are
solidly resolved; the concerns are resolved with one partial exception. No new BLOCKERs found;
two new/residual CONCERNs and a few MINOR citation inaccuracies surfaced.

## Round 1 Findings — Resolution Status

- **Blocker 1 (directory-mode sessions never get a commit list)**: **RESOLVED.**
  `GitWorktreeManager.HasWorktree()` (`session/git_worktree_manager.go:42`),
  `.GetBaseCommitSHA()` (`:113`), and `.GetDirBaseSHA()` (`:39`, backed by field `dirBaseSHA`)
  all exist exactly as cited. `computeDirDiffStats`'s existing fallback pattern is real, at
  `session/instance_worktree.go:432-433` (`if !i.gitManager.HasWorktree() { dirBase :=
  i.gitManager.GetDirBaseSHA() ... }`), confirming Task 1.1.2a's proposed
  `Instance.GetBaseCommitSHA()` mirrors a pattern that already exists and works. No existing
  `Instance.GetBaseCommitSHA` method collides with the new one (only `GetGitWorktree`/
  `GetWorktreePath` exist at line 400 today). `gitManager` is a concrete (not interface) field
  on `Instance` per `session/instance.go:428`, matching the doc comment's claim.
- **Blocker 2 (aggregate diff-stat blanks out when clean)**: **RESOLVED.** Confirmed the actual
  bug: `VcsWidget.tsx:106` gates the aggregate line on `mode === "compact"` exactly as claimed,
  and `adapters.ts:92` currently hardcodes `commits: []`. The fix's building blocks are real:
  `FileStatsBetween(repoPath, baseSHA, headSHA)` exists at `session/git/ops.go:434` with the
  claimed signature (no ctx), so `DiffStatBetween`'s wrap-and-sum approach is coherent.
  `session/vc/types.go` does not import `session/git` today and `session/git` does not import
  `session/vc` — the claimed "no import cycle" is verified. `WorkspaceService.GetVCSStatus`
  (`server/services/workspace_service.go:132`) has `provider.GetStatus()` at line 176 and the
  cache-store at line 183 exactly as cited — Task 1.1.3b's insertion point is real and correct.
  `vc.GitProvider` exists as `*GitProvider` (`session/vc/git_provider.go:21,28`), so the
  `provider.(*vc.GitProvider)` type assertion is valid.
- **Concern 1 (commits vs. failed-fetch ambiguity)**: **PARTIALLY RESOLVED.** The
  `CommitsUnavailable` field and its wiring are real and correctly cover a
  `ListShippedCommits` failure. But Task 1.1.3b's proposed code only enters the
  commit/diff-stat block when `git.GetHeadCommitSHA(workDir)` succeeds
  (`headErr == nil && headSHA != baseSHA`) — if HEAD resolution itself fails, `Commits`,
  `CommitsTruncated`, and `CommitsUnavailable` all stay at their zero values, silently
  reproducing the exact "failure reads as empty" ambiguity this fix exists to eliminate, just
  one call earlier in the chain. See new Concern below.
- **Concern 2 (staleness binding for "why blocked" rollup)**: **RESOLVED.** `GithubSummary`
  gains `lastCheckedAt?: Date` (Task 2.3.1a) and `VcsWidgetBlockingReasons` (Task 4.5.1a) binds
  its own stale-notice to it directly in the component's render output, not just a sibling
  label — matches the finding's requirement that staleness live inside the rollup itself.
- **Concern 3 (CollapsibleGroup keyboard nav)**: **RESOLVED**, and stronger than expected:
  `CollapsibleGroup` is not a proposed abstraction — it already exists in
  `web-app/src/components/ui/Collapsible.tsx` with the exact shared-`Accordion.Root`/context
  mechanism the plan describes (`CollapsibleGroupContext`, `openKeys`, and `CollapsibleSection`'s
  own doc comment noting `defaultExpanded` is "architecturally dead in grouped mode"). Tasks
  4.3.1c/4.4.1b/4.6.2b correctly nest all three new sections as siblings inside one
  `CollapsibleGroup` element, added incrementally in a way that degrades cleanly if Epic 4.6 is
  cut.
- **Concern 4 (duplicate ShippedCommit / VCSCommitSummary)**: **RESOLVED.** `backlog.proto`'s
  `ShippedCommit` is real at line 391 (plan cites 390-396, off-by-one on the leading comment
  line), `backlog.proto` imports `types.proto` at line 6 (confirmed), and `types.proto` imports
  only `google/protobuf/timestamp.proto` — it does not import `backlog.proto`, so relocating
  `ShippedCommit` the other direction is genuinely cycle-safe, not just asserted to be.
  `BacklogItemShipStatus.commits = 12` at line 360 needs no change under the relocation, as
  claimed. No leftover `VCSCommitSummary` references remain anywhere in the current plan text.
- **Concern 5 (100-commit truncation case untested)**: **RESOLVED, no hedge remains.** Current
  `ListShippedCommits` (`session/git/ops.go:369`) has the exact loop shape
  (`for len(queue) > 0 && len(commits) < listShippedCommitsCap`) the extraction in Task 1.1.1a
  assumes, so `listShippedCommitsWithCap(ctx, ..., cap int)` with `truncated := len(commits) >=
  cap` is a coherent, minimal refactor. Task 5.1.2a's test calls the unexported helper directly
  with an injected small cap — genuinely exercises `truncated == true`, and the existing
  `TestListShippedCommits_should_ReturnNewestFirst_When_MultipleCommitsShipped`
  (`session/git/ops_test.go:421`) is real and confirmed to still pass through the delegation.

## Blockers

*(none)*

## Concerns

- [ ] **`CommitsUnavailable` doesn't cover the `GetHeadCommitSHA` failure path.** In Task
  1.1.3b's proposed `GetVCSStatus` code, the whole commit-list/aggregate-diff-stat block is
  gated on `headErr == nil`. If `git.GetHeadCommitSHA(workDir)` fails for any reason, `Commits`
  and `CommitsUnavailable` both stay at zero value — indistinguishable from "genuinely zero
  commits," the exact ambiguity `CommitsUnavailable` was introduced to remove (round-1 Concern
  1). Recommendation: also set `CommitsUnavailable = true` (with a debug log, matching the
  existing pattern) when `headErr != nil` and `baseSHA != ""`, or note explicitly in the task
  that this path is accepted as out-of-scope because `GetHeadCommitSHA` already has a
  CLI-fallback mitigation for its one known failure mode (the torn-read race ADR-001 cites) and
  is expected to essentially never fail otherwise — but the plan currently does neither; it's
  silent on this branch.
- [ ] **No dedicated unit test for the new `git.DiffStatBetween` function.** Story 1.1.1 adds
  `DiffStatBetween` (Task 1.1.1c) but no task under it adds a `session/git/ops_test.go` unit
  test exercising it directly (e.g., additions/deletions summation across multiple files, the
  inherited binary-file-omission behavior from `FileStatsBetween`, or a ctx-cancellation case
  mirroring `ListShippedCommits`'s bounding). The only coverage is Task 5.1.2c's
  `WorkspaceService`-level integration test, which asserts non-zero `FilesChanged` but not the
  function's own correctness in isolation. Recommendation: add a small
  `TestDiffStatBetween_should_SumAdditionsAndDeletions_When_MultipleFilesChanged`-style test
  alongside Task 1.1.1c, consistent with how `ListShippedCommits`'s own behavior is unit-tested
  independently of its `WorkspaceService` caller.

## Minors

- Task 1.1.1c's rationale claims `FileStatsBetween` "has 3 other call sites
  (`backlog_service_triage.go`, `backlog_lifecycle_pr.go`, `backlog_service_ship_status.go`)."
  Verified via grep: only 2 real call sites exist —
  `server/services/backlog_service_triage.go:145` and
  `session/backlog_lifecycle_pr.go:1122`. `backlog_service_ship_status.go` only *mentions*
  `FileStatsBetween` in a doc comment (`fileStatStatusToProto maps git.FileStatsBetween's
  plain-string status...`, line 185) — it doesn't call the function (it calls
  `git.ListShippedCommits` instead, which is a different function this task doesn't touch).
  Doesn't change the fix's correctness (the "don't change the signature" decision is still
  right), just a citation inaccuracy worth fixing before this becomes a stale cross-reference.
- Several other line-number citations are off by 1-2 (e.g., `ghReviewItem`/`ghStatusCheckItem`
  cited as "lines 116-132" vs. actual 117/126; `FetchBranch`'s timeout pattern cited as line 25
  vs. actual 24; the poller's 60s interval cited as line 39 in one place and line 41 in another
  — the field itself, `PollInterval: 60 * time.Second`, is at line 41). All immaterial to
  correctness — every citation checked resolved to the right function/field, just occasionally
  the wrong exact line.
- The aggregate diff stat (`DiffStatBetween` → `FileStatsBetween`) silently omits binary files
  from its files-changed count, per `FileStatsBetween`'s own documented behavior. This is
  pre-existing behavior being inherited, not introduced by this patch, and the plan doesn't
  claim otherwise — flagging only because the new UI surface (Epic 4.1) will display this
  possibly-undercounted number to users for the first time.
