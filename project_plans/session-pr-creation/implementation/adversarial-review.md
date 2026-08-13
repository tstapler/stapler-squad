# Adversarial Review: session-pr-creation

**Date**: 2026-08-06
**Verdict**: BLOCKED

## Blockers

- [ ] **Preview diff scope mismatch can silently understate/misdescribe the shipped PR.** `DraftPullRequest` (Task 1.3.1c) drafts the title/body from `session.GetGitDiff(ctx, wt.WorktreePath(), inst.Worktree.BaseCommitSHA)`, which is `baseSHA..HEAD` — **committed changes only** (`session/backlog_review.go:658-684`). But the SessionCard's own diff viewer that the user looks at before clicking "Create PR" (`SessionService.GetSessionDiff` → `GitWorktree.Diff()`, `session/git/diff.go:43-90`) diffs against the **working tree**, including uncommitted/dirty changes (it even runs `git add -N .` to pick up untracked files). Then `CreatePullRequest` (Task 1.4.1b) commits that same dirty state via `wt.CommitChanges(...)` immediately before pushing. Net effect: if the worktree has any uncommitted changes when the modal opens (the normal case for an "active worktree" session, which is exactly what AC1 scopes this feature to), the title/body the user reviews and edits describes a *smaller* diff than what actually gets pushed and published as the PR — directly undercutting AC1's "body generated ... against the session's diff" and the feature's stated core purpose (ux.md §5: "confidence the PR looks right before it becomes public"). This is compounded by Task 1.4.1b explicitly directing that a `CommitChanges` error is "logged (not failing)" — copied verbatim from `pushAndCreatePR`'s unattended-automation pattern — so if committing the dirty state fails, those changes are silently dropped from the PR with no error surfaced, even though AC6 requires specific failure surfacing and the user just saw those changes as "in scope" on the diff viewer. — **Recommendation**: either (a) have `DraftPullRequest` preview from the same working-tree-inclusive diff `GetSessionDiff` uses, not the committed-only `GetGitDiff`, or (b) commit dirty state (via the same `CommitChanges` call) *before* drafting, not just before creating, so preview and push draw from the same commit set; and (c) surface — not swallow — a `CommitChanges` failure in `CreatePullRequest`, since a manual review-then-publish flow has different trust requirements than the unattended backlog path this was copied from.

- [ ] **New RPC logic bypasses the codebase's own established domain-service extraction pattern, growing an already-4525-line god object.** `SessionService`'s struct (`server/services/session_service.go:74-140`) shows an active, ongoing convention of extracting RPC logic into dedicated services (`reviewQueueSvc`, `searchSvc`, `githubSvc`, `workspaceSvc`, `configSvc`, `fileSvc`, `checkpointSvc`, etc.), with `SessionService` itself keeping only thin one-line delegators (e.g. `func (s *SessionService) GetPRInfo(...) { return s.githubSvc.GetPRInfo(ctx, req) }`, line 2813). `GitHubService` (`server/services/github_service.go`) *already exists* and already owns exactly this class of work — `GetPRInfo`, `GetPRComments`, `PostPRComment`, `MergePR`, `ClosePR`. The plan (Epics 1.3/1.4) instead implements `DraftPullRequest`/`CreatePullRequest` — plus a new `prCreationInFlight sync.Map` field and a new shared `resolveSessionWorktree` helper — directly in `session_service.go`, without ever mentioning `GitHubService` in the plan (including its own Pattern Decisions table, which otherwise carefully justifies every other structural choice made). This both continues growing the file the codebase has clearly been trying to shrink via extraction, and scatters PR-creation logic away from its sibling PR RPCs for no stated reason. — **Recommendation**: implement `DraftPullRequest`/`CreatePullRequest` bodies (including the in-flight guard) as methods on `*GitHubService`, with `SessionService` keeping only the proto-mandated thin delegators, matching `GetPRInfo`'s existing pattern exactly. This should be fixed before Phase 1 starts — it touches almost every task in Epics 1.3/1.4's file lists.

## Concerns

- [ ] **No timeout budget anywhere in the request path.** `DraftPullRequest`'s call to `headless.DraftPRDescription` → `pool.CallBlocking` runs on whatever ctx the connect handler receives; the HTTP server sets `WriteTimeout: 0` (`server/server.go:76`, deliberately unbounded for streaming), and the plan's frontend hook (Task 2.2.1a) explicitly mirrors `runOneShot`'s "exact shape" — which itself passes no `timeoutMs` option to the connect client call (unlike `createSession`, which sets an explicit `CREATE_SESSION_TIMEOUT_MS = 160_000`, `useSessionService.ts:53`). If the underlying `claude`/LLM subprocess stalls, `DraftPullRequest` can hang the "open modal" experience indefinitely with `isDrafting=true` and no cancel affordance. Separately, `CreatePullRequest`'s `CommitChanges` + `PushBranch` (60s) + `CreatePR` (60s, +30s fallback) sequence can legitimately run up to ~150s worst case with no ctx deadline wrapping the whole handler. — Recommend a bounded ctx in both handlers plus a `timeoutMs` on both frontend calls (following the `createSession` precedent, not the timeout-less `runOneShot` one), and a UI timeout/cancel state for the drafting spinner.

- [ ] **Epic 2.5's dead-prop removal (7+ files) is broader than what AC7 requires.** AC7 requires removing the two confusing UI entry points, which Epics 2.3/2.4 do. Epic 2.5 additionally strips `onRunOneShot` prop threading from `SessionCard`, `SessionRow`, `SessionList`, `PaneHeader`, `PaneSplitRenderer`, `page.tsx`, and `review-queue/page.tsx` — legitimate dead-code hygiene, but not functionally required by any acceptance criterion, and it meaningfully expands the review surface of an already large plan (proto changes, a Go signature change, two new RPCs, six touched frontend components). — Recommend either explicitly scoping Epic 2.5 out to a fast-follow PR, or keeping it but flagging in the PR description that it's pure cleanup so reviewers calibrate attention.

- [ ] **Epic 1.1's file list undercounts its own test blast radius.** The `prCreator` interface (`session/backlog_lifecycle.go:249`) is implemented by `fakePRCreator` in `session/backlog_lifecycle_test.go:2242` (`CreatePR(title, body string) (string, int, error)`, plus a direct 2-arg call at line 2451) and used via `SetPRCreatorFactory` in ~25 call sites across both `session/backlog_lifecycle_test.go` and `session/backlog_lifecycle_stuck_test.go`. Task 1.1.1b's interface signature change (3 args) will break `fakePRCreator`'s interface satisfaction and fail to compile, but neither test file is listed in Epic 1.1's "Files" section (which names only `worktree_git.go`, `worktree_git_test.go`, `backlog_lifecycle.go`). Self-healing via `go build`/`go test` failure, but the plan should list these files so the implementer isn't surprised mid-task.

- [ ] **Accessor-method naming mismatch.** Tasks 1.3.1b/1.3.1c/1.4.1c reference `wt.RepoPath()`, `wt.WorktreePath()`, `wt.BranchName()` and instruct "add if one doesn't already exist... check first." `GitWorktree` already has these, under different names: `GetRepoPath()`, `GetWorktreePath()`, `GetBranchName()` (`session/git/worktree.go:216-236`). The plan's own "check first" caveat wasn't actually followed when the plan was written — risk of either confusing duplicate accessors or an implementer silently correcting the plan text mid-task. Trivial fix, but flag before Phase 1 so tasks reference the real method names.

- [ ] **`DraftPullRequest`'s existing-PR short-circuit trusts only the session's cached field, not a live lookup.** Task 1.3.1b's early-return checks `inst.GitHubPrUrl != ""` — a locally cached field, not a fresh `findExistingPR()` call. If that cache is stale/empty (plausible — it's exactly the class of drift the plan's own BUG-040-style persist-failure handling for `CreatePullRequest`'s writes exists to guard against), the preview modal shows the full editable create form for a session that actually already has a PR on GitHub. On submit, `CreatePR`'s internal `findExistingPR` check still prevents a duplicate — but it also silently discards the user's edited title/body (no `gh pr edit` call) and returns the pre-existing PR's original title/body, with `already_existed` incorrectly reported `false` per Task 1.4.1c's own acknowledged fast-path-only detection heuristic. Net effect: a confusing "success" that quietly threw away everything the user just typed with no distinct signal. — Recommend either a live existing-PR check in `DraftPullRequest`, or having `CreatePullRequest`'s response distinguish "returned a pre-existing PR your edits were not applied to" from a real create/update.

- [ ] **Modal-closed-mid-flight behavior is undesigned and untested.** `CommitChanges`/`PushBranch`/`CreatePR` all use `context.Background()` internally rather than the request's ctx, so they run to completion server-side even if the browser navigates away or the connect call is aborted — the in-flight guard still releases via `defer`, and `GitHubPrUrl` eventually updates through the normal event-bus path. This is probably the right behavior, but it's stated nowhere in the plan and has zero test coverage (Epic 1.5's tests are all synchronous request/response). Add a one-line note plus a comment at the call sites so a future reader doesn't assume cancellation aborts the git operations.

- [ ] **No explicit test for a force-pushed/deleted branch between `DraftPullRequest` and `CreatePullRequest`.** Adequately covered mechanically by the generic push-failure error path (AC6 — `PushBranch`'s error surfaces verbatim), so not a design gap, just missing from Epic 1.5/3.2's test list. This race is specifically more likely for a live/active session (this feature's stated scope) than for the backlog-automation path the pattern was copied from — worth one more explicit test case (e.g. `TestCreatePullRequest_should_SurfaceSpecificError_When_PushRejected_NonFastForward`).

## Minors

- The `sync.Map`-based in-flight guard is process-local by design, which is fine given the product runs as a single systemd user service (`.claude/rules/systemd-user-service.md`) — all browser tabs hit the same process, so this isn't the "multiple replicas" gap it might look like at first glance. It does not survive a process restart, but that's a negligible edge case given the deployment model.
- Epic 2.4 (wiring into `ReviewQueuePanel.tsx`) is justified, not scope creep — requirements.md's own baseline section names `review-queue/page.tsx:343` as an existing entry point for the old `onRunOneShot` flow, so AC7's "no second, confusing entry point" requirement directly obligates touching this file too.
- Frontend Jest coverage for `CreatePullRequestModal.tsx` is present and thorough (Epic 3.1, Tasks 3.1.1a-e) — not e2e-only. No gap here.
- ADR-001's gh-CLI-over-go-github reasoning is sound, well cross-referenced against `.claude/rules/prefer-go-git-over-subshells.md`'s own exception clause, and correctly scoped. No concerns.
- Base-branch selection deliberately stays a plain text input / small fixed list rather than fetching the full repo branch list (ux.md §2) — good scope discipline against an unscoped fetch-all-branches feature.

## Re-review (iteration 1, 2026-08-06)

**New verdict: CONCERNS.**

- Blocker (preview diff scope mismatch, committed-only vs. working-tree) —
  **RESOLVED, revised twice.** The first fix (a shared `ensureCommitted` step
  run by both handlers) was itself flagged as a new P1 by `sdd:4-validate`'s
  pre-mortem — it made the "read-only" `DraftPullRequest` silently commit
  the working tree just by opening the modal. Final fix: `DraftPullRequest`
  never commits; it previews via the same working-tree-inclusive diff path
  `GetSessionDiff`/`GitWorktree.Diff()` already uses, so the preview matches
  the diff viewer without mutating anything. `CreatePullRequest` alone still
  commits (as in the pre-review plan), and its `CommitChanges` error now
  surfaces as `connect.CodeInternal` (AC6) instead of being silently logged.
- Blocker (RPC logic bolted onto `SessionService` god object) —
  **RESOLVED**. Post-Review Revisions #3 + new Epic 1.0 extract
  `PRCreationService`, matching this codebase's own extraction convention.
- Concern (timeout budget) — **not addressed**, carried forward as a
  fast-follow: add bounded ctx + `timeoutMs` per the original recommendation.
  Not blocking — `RunOneShot` (the code this replaces) has the identical gap
  today, so this isn't a regression, just an opportunity.
- Concern (Epic 2.5 scope breadth) — **not addressed**, carried forward
  as-is; recommend flagging Epic 2.5 as pure cleanup in the PR description.
- Concern (stale cached existing-PR short-circuit) — **not addressed**,
  carried forward; the `DraftPullRequest`/`CreatePullRequest` split combined
  with `CreatePR`'s own `findExistingPR` check means a stale-cache path still
  degrades to a confusing but non-destructive outcome (edits silently
  discarded, `already_existed` misreported) rather than a duplicate PR or
  data loss — acceptable to ship and revisit if it's observed in practice.
- Minor findings (accessor naming, `sync.Map` process-locality, Epic 2.4
  justification, frontend coverage, ADR-001 soundness) — unchanged, still
  accurate against the revised plan.

No remaining BLOCKERs. Proceed to `sdd:4-validate`.
