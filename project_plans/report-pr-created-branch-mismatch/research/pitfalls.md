# Pitfalls Research: report_pr_created branch-mismatch fix

Project: `report-pr-created-branch-mismatch`. Scope: known failure modes and risks specific
to (a) an ancestry-based relaxation of `VerifyPRMatchesBranch`, (b) a manual-override tool,
and (c) regression/rate-limit exposure of either approach.

## 1. Ancestry check ("PR head commit descends from tracked branch") — failure modes

The repo already has the exact primitive this fix would reuse: `IsCommitOnMain`
([`session/git/ops.go:47-71`](../../../session/git/ops.go#L47-L71)) and its helper
`isAncestorOfRef` (lines 73-85), built on go-git's `object.Commit.IsAncestor`. Per
[`.claude/rules/prefer-go-git-over-subshells.md`](../../../.claude/rules/prefer-go-git-over-subshells.md)
this is the canonical pattern to extend, not a subshell `git merge-base`. Its failure modes,
verified against the actual implementation:

- **Ancestry-from-main is too weak, confirmed by the code itself.** `IsCommitOnMain` checks
  ancestry against `mainBranch`, not against the item's *tracked* branch
  (`backlog/<item-slug>`). If the fix's check is "is the PR's head commit a descendant of
  `origin/main`" (the naive reuse of this exact function), it is true for **every** PR in the
  repo, always — every branch forks from main by construction. This does not distinguish "PR
  opened from a legitimate recovery branch for *this* item" from "PR opened for a completely
  unrelated item." The check must instead compute ancestry/merge-base relationship against
  the *tracked branch's own commits* (e.g. shared merge-base commits unique to this item's
  work, or the tracked branch's fork-point from main), not against main itself. The research
  question's hypothesis is confirmed by reading the one ancestry primitive that exists.
- **Squash-merge / rebase breaks shared history entirely.** `IsAncestor` walks the commit
  graph by parent pointers. A squash-merged or rebased branch produces a *new* commit with no
  parent-child relationship to the original commits — `IsAncestor` returns `false` even
  though the work is provably "the same." If the fallback-branch recovery flow involves any
  rebase/squash (plausible — it's cutting a clean branch from `origin/main` specifically to
  shed a polluted branch's history), a pure ancestry check will reject exactly the case this
  bug report is about, because the fallback branch is often *deliberately* history-severed
  from the tracked branch's polluted history — that's the whole reason to cut a clean branch.
  An ancestry check is therefore not obviously the right primitive at all for the described
  scenario (clean-branch-cut-from-main) — it addresses "branch renamed but same history"
  cases better than "branch intentionally rebuilt from main" cases.
- **Fetch-completeness, not depth, is the operative risk.** `session/git/` has no shallow
  clone usage (`grep -rn "Depth:\|--depth" session/` returns nothing) — worktrees are full
  clones, so shallow-clone truncation is not demonstrated in this codebase today. The real
  risk is narrower but still live: `IsCommitOnMain`'s fetch only pulls
  `refs/heads/<mainBranch>` into `refs/remotes/origin/<mainBranch>`
  ([`session/git/ops.go:53`](../../../session/git/ops.go#L53)). A new check comparing the
  tracked branch against the PR's head commit needs **both** refs' commit objects resolvable
  locally — the tracked branch (which may only exist in a *different* session's worktree, not
  the caller's) and the PR head commit (which may be on a fork/branch never fetched into this
  worktree at all). Any new ancestry check must explicitly fetch both by name/SHA before
  calling `IsAncestor`, or `repo.CommitObject(...)` fails with "object not found" — which the
  handler must treat as a definitive-vs-transient case correctly (see §3).
- **A GitHub-side "compare" call is more robust than local ancestry, and is likely
  available for free.** `stack.md` (§1, this research phase) already found that `PRInfo` can
  cheaply expose `headRefOid` via the `gh pr view` field list. GitHub's own
  `GET /repos/{owner}/{repo}/compare/{base}...{head}` endpoint reports `status: identical |
  ahead | behind | diverged` and doesn't require a local clone to have any particular ref
  fetched — it computes merge-base server-side. This avoids both the local-fetch-completeness
  problem above and doesn't fall into the "everything is a descendant of main" trap, *if* the
  base of the compare is the tracked branch (not main). Worth flagging to the plan phase as
  an alternative to local go-git ancestry.

## 2. Manual-override tool — who could call it, and what actually gates "operator use"

- **There is no operator/admin role in this codebase.** `session.SessionRole` has exactly
  three values —
  `SessionRoleWork`, `SessionRoleTriage`, `SessionRoleReview`
  ([`session/backlog.go:50-52`](../../../session/backlog.go#L50-L52)) — all three are
  LLM-driven session roles (`IsTmuxBackedSessionRole` at line 74 confirms work/review both run
  in a tmux-backed agent session). There is no fourth role, no human/operator flag, and no
  distinct auth check anywhere in `server/mcp/` beyond "does `STAPLER_SESSION_UUID` resolve to
  a session linked to this item with role X" (`callerSessionUUID`,
  [`server/mcp/tools_backlog.go:36`](../../../server/mcp/tools_backlog.go#L36)). A grep for
  `operator|admin|IsAdmin` in `server/mcp/*.go` and `session/backlog.go` turns up only prose
  ("shown to the reviewer/operator") — never a role check. **AC2's "operator use" has no
  existing mechanism to gate on** — building a `report_pr_created_override` MCP tool with a
  role check modeled on the existing pattern would be exactly as callable by the same
  work-role session that got the original branch wrong, because "work" is the only role that
  is ever linked to an item at the point this bug occurs. A role-based gate cannot express
  "human, not agent" since role is orthogonal to that distinction in this data model.
- **The only channel that actually implies a human is the web UI, not an MCP tool.**
  `SetBacklogItemPRAndTransition` ([`session/storage.go:758`](../../../session/storage.go#L758))
  is a plain, auth-free `Storage` method already reused by both the agent-facing MCP tool and
  the system's own reconciliation backstop
  (`BacklogLifecycleListener.reconcileOrphanedAgentPRs`,
  [`session/backlog_lifecycle.go:2611`](../../../session/backlog_lifecycle.go#L2611)). A
  ConnectRPC endpoint under `server/services/` reachable only from `web-app/` (i.e. requires a
  browser session, not `STAPLER_SESSION_UUID`) is the one channel in this architecture an
  LLM-driven work session structurally cannot reach — MCP tools are the *only* surface
  exposed to agent sessions (`server/mcp/server.go`). If AC2's "operator override" is meant
  literally, it should be a new backend RPC + UI affordance (e.g. an "attach PR manually"
  action on the backlog item detail view), not a new MCP tool — otherwise the same hallucinating
  work session that mistyped/misjudged the original PR can simply call the override tool next
  and defeat the entire point of the guard the requirements doc says must be preserved.
- **If it must be an MCP tool anyway** (e.g. because triage/review sessions are the intended
  "operator" proxy), it should at minimum require a *different* role than the one that
  performed the original failing call — but nothing today distinguishes "this session is
  being driven by a human at a terminal" from "this session is autonomous." That distinction
  doesn't exist in `SessionRole` at all; inventing it is out of scope for this bug fix per the
  requirements' framing, which is a strong argument for the UI-RPC path over a new MCP tool.

## 3. Regression risk — what the two existing tests pin down

Read directly
([`server/mcp/tools_backlog_test.go:974-1053`](../../../server/mcp/tools_backlog_test.go#L974-L1053)):

- `TestReportPRCreated_should_RejectCall_When_BranchMismatch` — a **definitive** mismatch
  (`verifyPRMatchesBranch` returns `(false, nil)`) must still produce `ErrInvalidArgument`
  and leave the item's `Status`/`PrNumber` completely untouched. Any fix must preserve a
  reachable "no, this really doesn't match" outcome — i.e. the new check cannot become
  unconditionally permissive. If the fix is "ancestry OR exact match," there must still be a
  code path where ancestry also fails and the call is rejected with the *same* error class.
- `TestReportPRCreated_should_ReturnRetryableError_When_GitHubLookupTransientlyFails` — a
  transient lookup error (rate limit / network) must produce `ErrInternalError`, distinct
  from `ErrInvalidArgument`, so the calling agent retries instead of "correcting" a
  self-report it already got right. **This distinction gets harder to preserve, not easier,
  if the fix adds a second GitHub call** (e.g. fetch tracked-branch's PR *and* the reported
  PR's head SHA): now there are two calls that can each fail transiently, and both failure
  paths must map to `ErrInternalError`, while a definitive "not related" result from either
  must map to `ErrInvalidArgument`. A partial failure (call 1 succeeds definitively-false,
  call 2 errors transiently before a final verdict is reached) needs an explicit precedence
  rule in the plan — the current code's binary `(matched, err)` return shape doesn't have a
  slot for "one signal says no, the other signal is unknown."
- Both tests inject `resolveSessionBranch` and `verifyPRMatchesBranch` as function-field seams
  on `backlogHandlers`
  ([`server/mcp/tools_backlog.go:592-610`](../../../server/mcp/tools_backlog.go#L592-L610)).
  Any new ancestry-check dependency (e.g. a new `verifyPRAncestry` field, or a widened
  `verifyPRMatchesBranch` signature) needs the same seam pattern so these two tests keep
  compiling/passing without becoming integration tests against real GitHub.

## 4. Rate limiting — `VerifyPRMatchesBranch` is not actually rate-limit-aware today

- `GetPRForBranch` ([`github/client.go:378-428`](../../../github/client.go#L378-L428)) already
  issues **two** HTTP calls per invocation: a list call (`pulls?head=...`), then
  `GetPRInfoCtx(ctx, owner, repo, prs[0].Number)` for the full record (line 427) — so
  `report_pr_created` already makes 2 GitHub calls per attempt today, before this fix adds
  anything.
- **`github.DefaultRateLimiter` is effectively dead code on this call path.** `RateLimiter.Update`
  ([`github/rate_limit.go:49`](../../../github/rate_limit.go#L49)) is documented as "called
  automatically by rateLimitTransport on every response" (line 23's comment), but **no type
  named `rateLimitTransport` exists anywhere in the repo** (`grep -rln rateLimitTransport
  --include='*.go' .` matches only the comment itself in `rate_limit.go`), and `.Update(` is
  never called from any `.go` file. `ghHTTPClient` ([`github/http_client.go:13`](../../../github/http_client.go#L13))
  is a plain `&http.Client{Timeout: 30*time.Second}` with no custom `Transport` at all — so
  `DefaultRateLimiter`'s `rateLimitedUntil` state is never populated and `IsLimited()` always
  reports `false`. It's checked correctly by the two background pollers
  (`session/pr_status_poller.go:190`, `session/worktree_pr_poller.go:187`) before *they*
  dispatch work, but that check is a no-op given `Update` is never called — those pollers are
  currently unprotected against real rate limiting too, and `VerifyPRMatchesBranch` doesn't
  call `IsLimited()` at all before firing.
- **Net effect for this fix**: adding a second/third API call (e.g. an explicit compare call,
  or fetching `headRefOid`) does not make an already-rate-limit-*aware* path "more aware" —
  it simply adds unguarded requests on top of an unguarded path. This isn't a regression this
  fix introduces, but the fix should not be described as "safe because `VerifyPRMatchesBranch`
  is already rate-limit-aware" — it verifiably is not, today. If the plan phase wants real
  protection, wiring `DefaultRateLimiter.Update` into `ghHTTPClient`'s transport (a
  `http.RoundTripper` wrapper) is a prerequisite, not a side effect, of adding more calls here.
  A 403 from GitHub's secondary rate limit on the *added* call surfaces today as a bare
  `ErrInternalError` (§3) with no backoff — acceptable per the existing retryable-error test,
  but worth noting it's a raw propagate, not an actual rate-limit-aware retry.

## 5. Fallback branch deleted after merge (`gh pr merge --delete-branch`)

- `VerifyPRMatchesBranch` / `GetPRForBranch` query GitHub's PR list by `head=owner:branch`
  ([`github/client.go:379-381`](../../../github/client.go#L379-L381)). GitHub's PR API
  continues to report a PR's `headRefName` and (via `headRefOid`) its head commit **even
  after the branch ref is deleted** — the PR object itself is permanent; only the ref is
  gone. So a lookup keyed on the PR number (not the branch name) survives branch deletion
  fine. A lookup keyed on re-deriving the branch name and querying `head=owner:<branch>`
  would **fail to find the PR at all** once the branch is deleted (`ErrNoPR`), since that
  query filters by a live ref.
- This matters directly for the fix: if the new "operator override" or "ancestry" path
  works by taking the caller's `pr_number` and calling `GetPRInfoCtx(ctx, owner, repo,
  prNumber)` directly (get-by-number, as `GetPRForBranch` itself does after listing), branch
  deletion is a non-issue. If instead it re-queries by branch name (e.g. re-resolving
  `ref.HeadRef` from a freshly parsed URL, or re-listing PRs for the tracked branch to compute
  ancestry against "the tracked branch's current PR"), branch deletion after merge — the
  common `gh pr merge --delete-branch` case named in the research question — breaks the
  lookup with a false-negative `ErrNoPR`, which today maps to a **definitive** mismatch
  (`(false, nil)` in `VerifyPRMatchesBranch`, line 275-277), i.e. a hard reject, not a
  retryable error. That's the wrong classification for "the branch is gone because it already
  merged," and would need its own explicit handling in the plan (e.g. treat `ErrNoPR` on a
  *tracked-branch* lookup as "cannot verify by branch, fall through to number-based lookup"
  rather than an immediate hard reject).
- The local git ancestry check (§1) has the mirror problem: if the tracked branch's local ref
  was deleted (branch cleanup after merge) and never fetched from a remote that also deleted
  it, `repo.Reference(plumbing.NewBranchReferenceName(branch), true)` fails outright — the
  commit to compare against may only be resolvable by a raw SHA saved earlier (e.g.
  `wt.BranchName` resolves to a name, not a SHA, per `sessionBranch`
  ([`server/mcp/tools_backlog.go:592-601`](../../../server/mcp/tools_backlog.go#L592-L601))),
  which doesn't survive branch deletion at all unless the SHA was captured before deletion.

## 6. A concrete, verified non-atomicity risk for any override path reusing `SetBacklogItemPRAndTransition`

Not explicitly asked but directly implicated by AC2/AC3 and the "idempotency must be
preserved" constraint — worth flagging to the plan phase:

`SetBacklogItemPRAndTransition` ([`session/storage.go:758-786`](../../../session/storage.go#L758-L786))
is **not atomic between its two writes**. Its only idempotency guard is at the top: `item.Status
== PRPending && item.PrNumber == prNumber` (line 763) — an exact match on the *same* PR
number short-circuits as a no-op. Any other starting state (including "already pr_pending
with a *different* PR number" — exactly the state an override tool exists to correct) falls
through to `UpdateBacklogItem` (line 768, **unconditional**, no precondition) *before*
`TransitionBacklogItemStatus` is attempted with `ExpectedStatus: Review` (line 775-776). If
the item is already `pr_pending` (not `review`), the field write at line 768 still succeeds —
silently overwriting the previously-recorded `PrURL`/`PrNumber` — and only then does the
transition call fail its precondition and return an error. The caller sees `ErrInternalError`
("transition to pr_pending: ...") and would reasonably read that as "nothing happened," but
the PR fields were already clobbered. A manual-override tool built on this exact primitive,
called against an item that's already linked to a (correct or incorrect) PR, needs its own
precondition check before calling this method — reusing it as-is for the "re-attach a
different PR" override scenario reproduces the CLAUDE.md-documented anti-pattern of "an API
that accepts a write and silently ignores it is indistinguishable from success" in reverse: an
API that reports failure while a partial write already landed.

## Summary of what the plan phase must decide

1. Ancestry-from-main is confirmed too weak (§1) — any check needs to be relative to the
   tracked branch's own history/fork-point, not `origin/main`, and must account for
   squash/rebase severing history entirely (in which case ancestry may be the wrong tool —
   consider GitHub's `compare` API with the tracked branch as base, per §1's last point).
2. "Operator override" has no existing role/auth primitive to hang off of (§2) — the
   requirements' intent likely wants a UI-driven ConnectRPC path, not a new MCP tool, or the
   guard is trivially bypassable by the same session that caused the bug.
3. The two existing tests pin a strict `ErrInvalidArgument` (definitive) vs `ErrInternalError`
   (transient) split that gets harder, not easier, to preserve once a second signal source is
   added (§3) — the plan needs an explicit precedence rule for "definitive-no + unknown."
4. The rate limiter this fix might lean on for safety is not wired up at all (§4) — don't cite
   it as protection without fixing the wiring first.
5. Branch-name-keyed lookups break on post-merge branch deletion; number/SHA-keyed lookups do
   not (§5) — prefer number/SHA resolution paths wherever the fix touches GitHub lookups.
6. Any override tool must add its own precondition before writing PR fields — the shared
   `SetBacklogItemPRAndTransition` primitive is not safe to call unconditionally against an
   already-`pr_pending` item (§6).
