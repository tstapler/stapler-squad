# Feature Landscape Research: report_pr_created branch mismatch

Agent 2 (Features) — SDD research phase for `report-pr-created-branch-mismatch`.

## 1. Existing `TestReportPRCreated_*` contract (must not regress)

`server/mcp/tools_backlog_test.go:805-1050+`, function `reportPRCreated` at
`server/mcp/tools_backlog.go:623-726`. Six tests define the current behavior:

| Test | Behavior locked in |
|---|---|
| `TestReportPRCreated_should_TransitionToPRPending_When_ValidPR` (:805) | Happy path: role=work session, `resolveSessionBranch` + `verifyPRMatchesBranch` (both overridable seams) return match → item transitions to `pr_pending`, `PrNumber`/`PrURL` persisted. |
| `TestReportPRCreated_should_ReturnError_When_PersistFails` (:843) | If the storage write itself fails, the tool returns `success:false` rather than silently claiming success — mirrors BUG-040's `TestPushAndCreatePR_PRFieldsPersistFails_StaysInReview_AndNotifies`. |
| `TestReportPRCreated_should_NoOp_When_AlreadyPRPendingSamePR` (:885) | **Idempotency contract**: if `item.Status == pr_pending && item.PrNumber == prNumber`, short-circuit to a no-op success *before* calling `verifyPR` at all (`verifyCalled` assertion). Any fix must keep this early-return ordering — it's also a cost optimization (no GitHub call on retry). |
| `TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork` (:936) | Role guard: `SessionRoleReview` (or any non-`work` role) callers get `ErrPermissionDenied`. Checked via `GetItemSessionBySessionAndItem` + `itemSession.Role != session.SessionRoleWork`, *before* any GitHub call. |
| `TestReportPRCreated_should_RejectCall_When_BranchMismatch` (:978) | The exact scenario this bug is about: `verifyPRMatchesBranch` returns `(false, nil)` → `ErrInvalidArgument`, item is left completely untouched (status stays `review`, `PrNumber` stays 0). This is the test that will need a new sibling (not a replacement) once a "different but legitimate branch" path is added — the pure hallucination case must keep failing exactly like this. |
| `TestReportPRCreated_should_ReturnRetryableError_When_GitHubLookupTransientlyFails` (:1019) | Distinguishes `(false, err)` (transient — rate limit/network) from `(false, nil)` (definitive mismatch): transient → `ErrInternalError` (retryable), definitive → `ErrInvalidArgument` (don't retry the identical call). This distinction is structural in `VerifyPRMatchesBranch`'s return contract (`server/mcp/tools_github.go:264-271`) and must be preserved by any new verification path too. |

Two testing seams already exist and should be reused/extended rather than replaced:
`backlogHandlers.resolveSessionBranch` (func override) and
`backlogHandlers.verifyPRMatchesBranch` (func override), wired via
`h.sessionBranch()` (:592) and `h.verifyPR()` (:605).

## 2. Other branch-pollution recovery scenarios / existing reconciliation backstop

Grepped `.claude/rules/*.md` for "pollut/reconcil/stuck/diverged" — **no other
documented worktree/branch-pollution recovery pattern exists in the rules
directory**. The only prior art is code-level, in `session/backlog_lifecycle.go`.

**The reconciliation backstop referenced in the requirements is
`reconcileOrphanedAgentPRs`** (`session/backlog_lifecycle.go:2592-2672`, Epic 3.2
of "PR Metadata Capture Fix", `project_plans/backlog-agent-communication`). It
runs every reconcile tick (~60s) over `review`-status items with `PrNumber==0`,
no active session, finds the item's last work session's tracked branch via
`GetWorktreeDataBySessionUUID` (the **same** source `report_pr_created`'s
`sessionBranch()` uses), calls `github.GetPRForBranch(ctx, repoPath, wt.BranchName)`,
and on an open-PR match calls `SetBacklogItemPRAndTransition` — the same
primary-write path `report_pr_created` itself calls at :717.

**Critical finding: this backstop has the exact same blind spot as the bug
being fixed.** It looks up GitHub PRs by the item's *tracked* branch name
(`wt.BranchName`), not by any fallback/rescue branch. If a session opens its
PR from `feature/<slug>` instead of `backlog/<slug>`, `reconcileOrphanedAgentPRs`
will **also** silently never find it — it isn't a working fallback path for
this bug's scenario, it's a second instance of the identical limitation. Any
fix to `report_pr_created`'s branch-matching logic should be designed so it
can be **shared by** (or at minimum kept consistent with) this reconciler —
otherwise the codebase ends up with two independent, divergent definitions of
"this PR belongs to this item." Concretely: whatever new "is this PR
legitimately this item's PR" predicate gets built for `report_pr_created`
(descendant-of-lineage check, or an override path) should live somewhere
`reconcileOrphanedAgentPRs` can call it too, closing its matching blind spot
in the same change or a fast-follow.

No other reconciler (`reconcileDriftedPRItems`, `reconcileBouncingItems`,
`reconcilePushFailedItems`, etc. — see :3438, :2795, :3457) does a from-scratch
open-PR-by-branch lookup the way `reconcileOrphanedAgentPRs` does; the rest
operate on PR data the item *already has recorded*.

## 3. Edge cases for the fix design

**(a) PR shares no git history with the tracked branch (must stay rejected).**
No existing helper in this codebase does a merge-base/ancestry check against a
*branch name* directly — the closest primitive is `session/git/ops.go`'s
`IsCommitOnMain(repoPath, mainBranch, sha string) (bool, error)` (go-git,
`CommitObject` + `IsAncestor`, with an `origin/main` fallback for remotely-merged
commits — see `.claude/rules/prefer-go-git-over-subshells.md`). A lineage
check would need something structurally similar but generalized to "is commit
X an ancestor of (or does X share a merge-base with) branch Y" rather than
hardcoded to main. `github.PRInfo` (`github/client.go:40-60`) does **not**
carry a head SHA field today — only `HeadRef`/`BaseRef` (branch names) — so a
descendant-of-lineage check would need either (1) an additional `gh api`/GraphQL
call to get the PR's head commit SHA, or (2) a local git lookup of
`origin/<headRef>` after a fetch. This is a real implementation cost the plan
phase needs to size.

**(b) Two sessions racing to report a PR for the same item.** `reportPRCreated`
has no explicit locking; it relies on `SetBacklogItemPRAndTransition`
(`session/storage.go`) as the sole write path plus the idempotency short-circuit
at the top (`item.Status == pr_pending && item.PrNumber == prNumber`). Today,
two *different* PR numbers racing for the same item would both pass the
per-request checks (both see `item.Status == review` before either write lands)
and the second write would silently overwrite the first's PR reference — this
is a pre-existing gap, not new to this bug, but a manual-override path (AC
option 2) that skips the strict-match guard is a more attractive target for a
racing/duplicate call than the current strict-match tool is, since it lowers
the bar for what gets accepted. Any override path should keep the identical
idempotency short-circuit and ideally re-check `item.Status`/`PrNumber`
immediately before the write (or rely on the underlying storage layer's own
compare-and-swap semantics, if any — not confirmed in this pass).

**(c) A fallback branch cut from a *stale* origin/main could be gamed to
falsely "confirm" an unrelated PR.** This is the sharpest edge case. If the
"descendant of tracked branch's lineage" check is implemented naively as "PR
head branch contains the tracked branch's *first* commit" or "shares *any*
common ancestor with the tracked branch," it would trivially pass for **any**
branch ever cut from this repo's main line, including a completely unrelated
PR — because every branch in the repo shares old history with `main`, which
the tracked branch also descends from. A same-repo, same-lineage check is not
sufficient to prove "this PR is *this item's* work"; it would defeat the
anti-hallucination purpose entirely (any real PR number from the same repo
would pass). The check needs to be anchored to something item-specific, not
repo-generic — e.g.: the fallback branch's *divergence point* must be at or
after the tracked branch's own branch-off point from main (not just "shares
history with main" at all), and/or the fallback branch's commits must
reference/touch the same files as the tracked branch's own commits, and/or
(cheaper and more robust) the manual-override path should require the PR
number to be supplied by a human/operator context, not by an LLM-driven
session self-reporting on its own initiative — see item 4 below.

## 4. Unstated needs

- **Auditability if an override path bypasses the anti-hallucination check.**
  `VerifyPRMatchesBranch`'s doc comment (`server/mcp/tools_github.go:249-271`)
  is explicit that the guard's entire purpose is stopping a hallucinated/stale/
  mistyped PR reference from an LLM-driven session from silently poisoning the
  record. An override tool that skips this check needs to compensate with
  *something* — the current `reportPRCreated` only does a plain `log.InfoLog`
  line (:721) on success; an override path should almost certainly log at a
  more visible level (or a distinct log-line prefix/marker) and should
  probably still require the caller to state *why* the strict check couldn't
  pass (e.g., an explicit `override_reason` argument), so the audit trail
  shows this wasn't the default path.
- **Should "role=work" alone gate the override, or something stronger?**
  Every other guard in this handler (idempotency, role, branch match) is
  automatic/mechanical, evaluated by the LLM-driven session itself with no
  human in the loop. An override path that trusts a bare PR number without a
  GitHub-side branch check is a strictly *weaker* guarantee than what exists
  today for every other item. The requirements' own constraint ("Do not weaken
  the guard so any caller-supplied PR number is trusted without a GitHub-side
  check") suggests option 1 (lineage-verified match) is the intended primary
  fix, with option 2 (manual override) as a narrower fallback that still needs
  *some* GitHub-side check (e.g., "PR exists, is open/merged, and its base
  branch is main" is a much weaker but still real check) rather than trusting
  the caller wholesale.
- **The error message itself is a first-class deliverable** (AC #3): today's
  message (`server/mcp/tools_backlog.go:712-714`) gives no indication that a
  workaround exists; a session hitting this in a loop has no way to discover
  the fallback-branch pattern or an override tool from the error text alone.
  Whatever the fix, the rejection message needs to name the concrete next step
  (tool name, or the documented fallback-branch recovery flow) so a future
  session doesn't retry the identical failing call indefinitely — this is
  explicitly called out as a required AC, not just a nice-to-have.
- **No existing "attach PR to item" manual tool.** Confirmed by scanning the
  MCP tool surface (`mcp__stapler-squad__*` in this session's tool list, and
  `server/mcp/tools_backlog.go`) — there is genuinely no tool today that lets
  an operator (or a session) attach a PR URL to an item without going through
  `report_pr_created`'s strict path, matching the requirements' "There is no
  alternate MCP tool" claim.

## Summary of design tension for the planning phase

The two ACs pull in different directions: option 1 (lineage-verified branch
descendant check) keeps the guarantee automatic but is real implementation
work with a sharp gaming edge case (3c above) that needs careful anchoring, not
just "shares git history." Option 2 (manual override tool) is cheaper to build
but weakens the anti-hallucination guarantee unless paired with its own
GitHub-side check and stronger auditing than the current single `log.InfoLog`
line. Whichever is chosen, it should be designed to also close
`reconcileOrphanedAgentPRs`' identical blind spot (finding #2 above), not just
`report_pr_created`'s.
