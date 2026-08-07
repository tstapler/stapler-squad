# Architecture Research: report_pr_created branch mismatch

## 1. Current control flow

`reportPRCreated` (`server/mcp/tools_backlog.go:623-726`):

1. Feature-flag check, `callerSessionUUID(ctx)`.
2. Arg parsing: `item_id`, `pr_url`, `pr_number`, `summary` (validated individually).
3. Link/role check: `GetItemSessionBySessionAndItem` then `itemSession.Role != session.SessionRoleWork` → `ErrPermissionDenied` (`:669`).
4. `GetBacklogItem` (`:673`).
5. **Idempotency**: if `item.Status == pr_pending && item.PrNumber == prNumber` → success no-op (`:682`).
6. `session.ParseGitHubURL(prURL)` + cross-check `ref.PRNumber == prNumber` (`:691-697`) — local, no network.
7. `branch, _ := h.sessionBranch(ctx, callerUUID)` (`:702`) → seam wrapping `h.resolveSessionBranch` or `storage.GetWorktreeDataBySessionUUID` (`tools_backlog.go:592-601`).
8. `matched, _ := h.verifyPR(ctx, ref.Owner, ref.Repo, prNumber, branch)` (`:707`) → seam wrapping `h.verifyPRMatchesBranch` or the package func `VerifyPRMatchesBranch` (`tools_github.go:249-281`, `tools_backlog.go:603-609`).
9. `!matched` → `ErrInvalidArgument` with the exact message quoted in the requirements (`:711-715`). **This is the hard-reject point.**
10. `storage.SetBacklogItemPRAndTransition(ctx, itemID, prURL, prNumber, summary)` (`:717`) — the write path.

`VerifyPRMatchesBranch` (`tools_github.go:272-281`) is a thin wrapper: `githubpkg.GetPRForBranch(ctx, owner, repo, expectedBranch)` then `info.Number == prNumber`. `GetPRForBranch` (`github/client.go:378-`) queries the GitHub REST API directly (`GET /repos/{owner}/{repo}/pulls?head={owner}:{branch}&state=all`) — no `gh` subprocess, explicitly to avoid fork/exec lock contention (comment at `client.go:376`). It returns `ErrNoPR` (definitive no-match) vs. a wrapped transient error, and the caller (`reportPRCreated`) already distinguishes these: transient → `ErrInternalError` "retry" (`:708-710`), definitive mismatch → the hard `ErrInvalidArgument` reject (`:711-715`).

**Existing seams** (both already overridable via struct fields, exactly as the doc comments at `tools_backlog.go:99-109` describe):
- `h.resolveSessionBranch` — overrides `h.sessionBranch` → what "this item's own branch" means.
- `h.verifyPRMatchesBranch` — overrides `h.verifyPR` → how a PR is confirmed to belong to that branch.

Both seams are function-typed struct fields defaulting to the real implementation, used today purely to avoid real git/GitHub calls in tests. This is the pattern any fix must reuse, per the requirements — no second seam mechanism.

`PRInfo` (`github/client.go:40-64`) currently carries `HeadRef`/`BaseRef` (branch **names**, no head/base **SHA**). No field carries a commit SHA anywhere in the struct.

## 2. Two architectural options

### Option (a): Relax the check to a GitHub-verified ancestry check

Replace "PR's head branch string-equals the item's tracked branch" with "PR's head is a descendant of the item's tracked branch" (or of that branch's merge-base with `origin/main`).

**Why REST compare, not go-git ancestry:** `.claude/rules/prefer-go-git-over-subshells.md` prefers go-git over shelling out, but go-git's `IsAncestor` (see `session/git/ops.go`'s `IsCommitOnMain`) requires **both commits present in the local repo object database**. A PR opened from a differently-named fallback branch (e.g. `feature/<slug>` cut fresh from `origin/main`) may never have been fetched into the calling session's worktree — go-git would need an extra `git fetch` first, which is itself a network op. GitHub's REST **compare** endpoint (`GET /repos/{owner}/{repo}/compare/{base}...{head}`) answers the ancestry question server-side, needs no local fetch, and is a natural sibling of `GetPRForBranch` — which *already* hits the REST API directly via `newGHRequest`/`ghHTTPClient` (`github/http_client.go:13,54`) specifically to avoid `gh` subprocess overhead (`client.go:376`). So option (a), done well, is *also* the go-git-avoiding choice — not a violation of that rule, an application of its spirit (avoid subshells/local fetches when a typed API called does the job).

**Concrete changes:**
- `github/client.go`: new function, e.g. `func BranchIsAncestorOfPR(ctx context.Context, owner, repo string, prNumber int, branch string) (bool, error)` — or lower-level `ComparePRHeadAgainstBranch`. Needs the PR's head SHA: either (i) a new REST call `GET /repos/{o}/{r}/pulls/{n}` for `head.sha`, or (ii) reuse `GetPRInfoCtx`'s `gh pr view` (subprocess — inconsistent with the REST-only pattern `GetPRForBranch` set) — prefer (i), a small REST addition matching the existing `newGHRequest` pattern. Then a second REST call: `GET /repos/{o}/{r}/compare/{branch}...{headSHA}` and check `status` is `"ahead"` or `"identical"` (base is an ancestor of head) — see GitHub's compare-commits API semantics. `ahead_by`/`behind_by`/`merge_base_commit.sha` are also available for a merge-base variant.
- `PRInfo` does *not* strictly need a new field if the new function does its own two-call round trip internally and returns only a bool — keeps `PRInfo` unchanged, smaller diff. (A `HeadSHA` field could be added for reuse/testability, but isn't required.)
- `tools_github.go`: `VerifyPRMatchesBranch` itself changes semantics — either it gains a new sibling (`VerifyPRDescendsFromBranch`) or its body is rewritten to try exact-branch-match first (fast path, keeps existing behavior/tests for the common case) and fall back to the ancestry check only when the exact match fails. The doc comment (`:249-271`) needs a rewrite to explain the loosened contract — this is where AC3's "why" belongs architecturally, not just the error string.
- `tools_backlog.go`: `h.verifyPR`/`h.verifyPRMatchesBranch` seam signature is unchanged (`(ctx, owner, repo, prNumber, expectedBranch) (bool, error)`) — the relaxed logic lives entirely inside what that seam calls. **No new seam, no new struct field** — reuses the existing one exactly as the requirements ask.
- Test seams: existing `verifyPRMatchesBranch` override in tests continues to work unchanged; new unit tests target the new `githubpkg` function directly (mocked `ghHTTPClient` transport, matching how `GetPRForBranch` is presumably tested).

**Risk/cost:** two GitHub API round-trips instead of one; ancestry-via-compare is a real GitHub-verified check (satisfies the "do not weaken to trust any caller-supplied PR number" constraint) but is *conceptually* a bigger semantic change to a function whose doc comment explicitly frames it as an anti-hallucination guard — reviewers must confirm "descends from" still rules out a hallucinated/unrelated PR (it does: an unrelated PR's head commit will essentially never have the item's branch tip as an ancestor).

### Option (b): Separate manual-override tool

Add a new MCP tool, e.g. `link_pr_manually`, alongside `report_pr_created` rather than changing its behavior.

**Concrete changes:**
- `server/mcp/tools_backlog.go`: new handler `linkPRManually` (mirrors `reportPRCreated`'s structure) plus a new `mcpgo.AddTool(...)` registration block (pattern at `:1013-1037`). Reuses: feature-flag check, `callerSessionUUID`, arg parsing, the same role/link check (`itemSession.Role != session.SessionRoleWork`, `:669` — requirements explicitly keep this restriction), `ParseGitHubURL` + number cross-check, and the terminal write via `storage.SetBacklogItemPRAndTransition` (unchanged).
- **What it drops**: the `h.sessionBranch`/branch-equality step entirely — no `expectedBranch` argument to a verify call.
- **What it keeps as the GitHub-side check** (to satisfy "must keep a real GitHub-verified check" and "not weaken the guard so any caller-supplied PR number is trusted"): call `githubpkg.GetPRInfoCtx` (or a lighter REST `GetPRByNumber`) for `(owner, repo, prNumber)` directly — confirm the PR **exists**, belongs to the **same owner/repo** as the item/session, and optionally is **open or merged** (not e.g. a random closed/unrelated PR number). This is a *weaker* check than branch-match (no branch relationship required at all), which is exactly the point — it's the deliberate manual/operator escape hatch — but it's still a real, non-bypassable GitHub API call, not blind trust of the argument.
- Could additionally require the item to currently be "stuck"/have no PR recorded (`item.PrNumber == 0` or a stuck-reason present) to keep this from being a general-purpose override for the strict path — requirements list this as optional ("maybe requires the item to currently have no PR / be stuck").
- New seam needed: `h.verifyPRExists` (or reuse a renamed/generalized existing seam) — a **second** function-typed field alongside `verifyPRMatchesBranch`/`resolveSessionBranch`, following the identical pattern (default to real `githubpkg` call, overridable in tests). This is additive to the existing seam set, not a competing mechanism — same shape, new field, matches requirements' "don't add a second competing seam mechanism" (that clause is about *how* to override, not about "no new fields ever").
- `docs/registry/features/backend/` needs a new per-feature JSON entry (new RPC/tool) per `.claude/rules/feature-registry.md`; MCP tool docstring must tell agents this exists (mirrors `:213-225` "how to report a PR" hint block).

**Risk/cost:** a second tool to document, test, and keep in the "how to report a PR" hint text (`tools_backlog.go:213-225`) in sync with `report_pr_created`; more surface area than modifying one function, but the blast radius of option (a)'s relaxed semantics is fully contained to the new tool — the original `report_pr_created`'s strict branch-match behavior is untouched for the common (unpolluted-branch) case.

## 3. Where AC3 (better error message) lives

- **If option (a) is chosen**: the existing `errResult(...)` call at `tools_backlog.go:711-715` is still the single reject point — ancestry check failing produces the same shape of rejection, so AC3 is a **one-line message change there** (e.g. append "workaround: cut a clean branch from origin/main, open the PR, then call report_pr_created again — the head only needs to descend from your original branch, not match it exactly" — should stay accurate to whatever the actual accepted relationship ends up being). No structural change needed beyond the message string.
- **If option (b) is chosen**: AC3 lives in **two places** — `reportPRCreated`'s existing `errResult` at `:711-715` (told to mention the new `link_pr_manually` tool as the workaround) **and** the new tool's own docstring/description, which must not itself trigger the same loop (i.e. the new tool's failure messages need to be equally clear, since it's the escape hatch). This is strictly more surface than (a) for satisfying AC3 alone.
- **If hybrid**: both — quick fast-path message pointing at the manual tool, tool itself self-documenting.

## 4. Does `SetBacklogItemPRAndTransition` need changes?

No. `session/storage.go:758-796` is a pure data-write path: idempotency check on `(status, PrNumber)`, `UpdateBacklogItem` for `PrURL`/`PrNumber`, `TransitionBacklogItemStatus` with a `BacklogItemPrecondition{ExpectedStatus: review}`, then best-effort progress-note append + stuck-row resolution. Nothing in it inspects *how* the PR was verified — it receives already-validated `(itemID, prURL, prNumber, summary)` and both options above call it identically, unchanged, as the final step. Confirmed by reading the full function body (`storage.go:758-796`) — no branch/verification-related logic present at all.

One nuance for option (b): its precondition is still `ExpectedStatus: review` (`storage.go:775`) — if a stuck item has drifted to some other status (not `review`), the manual-override tool would hit the same precondition failure `report_pr_created` would. That's *out of scope* for this fix per the requirements (they don't ask for relaxing the status precondition) but worth flagging for whoever implements: if the "stuck" scenario the bug report is about also involves a non-`review` status, option (b) alone doesn't fully unblock it and `SetBacklogItemPRAndTransition`'s precondition would need separate attention — not assumed here since the requirements' repro is specifically about the branch check, not status drift.

## 5. Recommendation

**Recommend option (a)**, specifically the narrow form: keep `VerifyPRMatchesBranch`'s exact-branch-match as the fast path (preserves 100% of existing behavior/tests for the normal case), and only when that returns "no PR for this exact branch," fall back to a new ancestry check via GitHub's compare API before giving up. Rationale, ranked by the repo's own stated priorities:

1. **Minimal diff, one seam, no new tool surface.** The existing `h.verifyPR`/`h.verifyPRMatchesBranch` seam already has the exact shape needed (`(ctx, owner, repo, prNumber, expectedBranch) (bool, error)`) — the fix is entirely inside what that seam calls, satisfying "reuse the existing seam pattern, don't add a second competing mechanism" with zero new struct fields on `backlogHandlers`.
2. **No new MCP tool to register, document, keep in sync with `report_pr_created`'s hint text, or add to the feature registry** (`.claude/rules/feature-registry.md` would require a new per-feature JSON + e2e test for option (b)'s new tool — real, non-trivial overhead for what the requirements call "a small, contained bug fix").
3. **Still a real GitHub-verified check**, satisfying "do not weaken the guard so any caller-supplied PR number is trusted" — ancestry-via-compare is *harder* to spoof than option (b)'s "PR exists in the right repo" check, since a hallucinated/unrelated PR number essentially never has the item's branch as an ancestor.
4. **No go-git/subshell tension.** Fits the "prefer go-git over shelling out" rule's actual intent (avoid process-boundary/parsing overhead) by extending the REST-only pattern `GetPRForBranch` already established, rather than requiring a local `git fetch` of a branch the worktree may never have seen.

Option (b) is the right call only if product/ops wants an explicit, auditable "operator manually attached this PR, bypassing normal verification" trail distinct from the automated report — that's a legitimate but *different* product decision than what this bug report asks for (its repro is entirely about a same-lineage branch rename, which option (a) resolves directly). Treat (b) as a future addition if a case arises where the PR genuinely has no git relationship to the tracked branch at all (e.g. a full rewrite/squash from scratch) — not needed for this fix.
