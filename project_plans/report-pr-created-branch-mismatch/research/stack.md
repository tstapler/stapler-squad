# Stack Research: report_pr_created branch-mismatch fix

Project: `report-pr-created-branch-mismatch`. Scope: confirm what GitHub-fetch and
git-ancestry primitives already exist in this repo before the plan phase proposes new
code.

## 1. `github/client.go` — what's already returned, and can `headRefOid` be added cheaply?

- `PRInfo` struct ([`github/client.go:40-64`](../../../github/client.go#L40-L64)) does **not**
  currently expose the PR's head commit SHA. It has `HeadRef` (branch name, from
  `headRefName`) but no SHA field.
- `GetPRInfoCtx` ([`github/client.go:245-304`](../../../github/client.go#L245-L304)) fetches
  via `gh pr view <prRef> --repo <repoRef> --json <fields>` (a `safeexec.CommandContext` gh
  CLI subprocess, not a Go GitHub client library — see §3). The current `fields` string
  (line 255) is:
  `number,title,body,headRefName,baseRefName,state,url,createdAt,updatedAt,isDraft,mergeable,additions,deletions,changedFiles,author,labels,reviews,reviewDecision,statusCheckRollup`
- **Verified directly against the installed `gh` CLI (v2.86.0) in this environment**:
  `gh pr view --json foo` (deliberately invalid field, to trigger the CLI's own field list)
  confirms `headRefOid` and `baseRefOid` are both valid `--json` fields, alongside every
  field already in use. This is a **field-list addition only** — no new API call, no new gh
  invocation. The change is:
  1. Add `HeadRefOid string \`json:"headRefOid"\`` to the unexported `ghPRResponse` struct
     ([`github/client.go:78-102`](../../../github/client.go#L78-L102)).
  2. Add `"headRefOid"` to the `fields` string at line 255.
  3. Add `HeadSHA string` (or similar) to the exported `PRInfo` struct and populate it in
     the return literal at [`github/client.go:281-303`](../../../github/client.go#L281-L303).
- `GetPRForBranch` ([`github/client.go:375-428`](../../../github/client.go#L375-L428)) is a
  **different transport** — it hits the GitHub REST API directly over HTTP (`newGHRequest` +
  `ghHTTPClient.Do`, no `gh` subprocess) to list PRs by `head=owner:branch`, then delegates to
  `GetPRInfoCtx` for the full record (line 427). So `GetPRForBranch`'s returned `PRInfo` would
  automatically pick up `HeadSHA` for free once `GetPRInfoCtx` is extended, since it's just a
  thin wrapper. The REST `pulls?head=...` list response itself also already contains a `head.sha`
  field GitHub-side, but the code currently only unmarshals `number` and `updated_at` from it
  ([`github/client.go:409-412`](../../../github/client.go#L409-L412)) — not needed since
  `GetPRInfoCtx` covers it.
- Conclusion: exposing the PR's head commit SHA is a **small, additive, backward-compatible
  change** to `PRInfo`/`ghPRResponse`/the `fields` string — not a new API surface.

## 2. Git ancestry-checking utilities already in the repo

- `session/git/ops.go` has the primitives the CLAUDE.md rule references:
  - `IsCommitOnMain(repoPath, mainBranch, sha string) (bool, error)`
    ([`session/git/ops.go:47-71`](../../../session/git/ops.go#L47-L71)) — opens the repo with
    go-git (`git.PlainOpenWithOptions`), best-effort fetches `mainBranch` from origin, resolves
    `sha` to a `*object.Commit`, and checks ancestry against both the local and
    `origin/<mainBranch>` refs.
  - `isAncestorOfRef(repo *git.Repository, commit *object.Commit, ref plumbing.ReferenceName) (bool, error)`
    ([`session/git/ops.go:73-85`](../../../session/git/ops.go#L73-L85)) — **unexported**,
    resolves a ref name to its tip commit and calls `commit.IsAncestor(target)` (go-git's
    `object.Commit.IsAncestor`, the actual ancestry primitive). It takes a `plumbing.ReferenceName`
    (a branch/tag ref), not an arbitrary SHA-to-SHA comparison, and it's private to the `git`
    package.
  - `BranchAheadBehind` ([`session/git/ops.go:97-102+`](../../../session/git/ops.go#L97)) and
    `countCommitsNotAncestorOf` ([`session/git/ops.go:190`](../../../session/git/ops.go#L190))
    also build on `object.Commit.IsAncestor`.
- **There is no exported, reusable "is commit X an ancestor of commit Y" (SHA-to-SHA) helper.**
  Everything exported takes a branch/mainBranch name and resolves it to a ref internally. A fix
  under AC1 (verify PR-head-SHA descends from the item's tracked-branch lineage) would need
  either: (a) a small new exported helper in `session/git/ops.go` that takes two raw SHAs/refs
  and calls `commit.IsAncestor(target)` directly (following the same go-git-not-subshell
  convention as `IsCommitOnMain`), or (b) reuse `IsCommitOnMain`-style logic against the item's
  tracked branch instead of `mainBranch`.
- **Caveat for AC1, worth flagging to the plan phase**: the bug's actual failure mode is a
  *polluted* tracked branch — the reported recovery is to cut a brand-new branch from
  `origin/main`, not from the polluted tracked branch. In that recovery flow the PR's head SHA
  is **not** a descendant of the polluted tracked branch's tip (it shares an ancestor further
  back, at whatever `origin/main` commit both diverged from) — so a strict "PR head is a
  descendant of the tracked branch tip" check would still fail for the exact scenario in the bug
  report. A looser check ("PR head and tracked branch share a common ancestor reachable from
  `origin/main`", or simply "PR head is reachable from `origin/main`") would pass, but is a much
  weaker guarantee against a hallucinated/mistyped PR number — worth the plan phase weighing
  explicitly against AC2's manual-override path.

## 3. gh CLI subprocess vs. Go GitHub API library

- No Go GitHub client library (`google/go-github` or similar) is imported in `github/client.go`
  — grep of the import block ([`github/client.go:1-20`](../../../github/client.go#L1-L20)) shows
  only stdlib (`encoding/json`, `net/http`, `net/url`, `os/exec`, etc.),
  `github.com/tstapler/stapler-squad/executor/safeexec`, and `golang.org/x/sync/singleflight`.
- Two access patterns coexist in this file, both already established conventions to match:
  1. **`gh` CLI subprocess** via `safeexec.CommandContext(ctx, "gh", "pr", "view", ..., "--json", fields)`
     — used by `GetPRInfoCtx` (full PR detail), `GetPRComments`, `GetPRDiff`, `PostPRComment`,
     `MergePR`, `ClosePR`.
  2. **Raw GitHub REST API over `net/http`** via `newGHRequest` + `ghHTTPClient` (a shared HTTP
     client, presumably token-authenticated) — used by `CheckGHAuth`, `GetCurrentUserLogin`,
     `GetPRForBranch`, `IsForkRepo`. Comment at
     [`github/client.go:376`](../../../github/client.go#L376) explicitly notes this path exists
     "to avoid forkExec lock contention."
- Whichever path a fix extends (most likely `GetPRInfoCtx`'s `gh pr view --json` fields, per §1),
  it should follow the existing pattern for that function rather than introduce a third access
  style.

## 4. Existing "manual override" / admin-bypass pattern in `server/mcp/tools_backlog.go`?

- **None found.** There are exactly three session roles gating MCP backlog tools —
  `SessionRoleWork`, `SessionRoleTriage`, `SessionRoleReview`
  ([`session/backlog.go:50-52`](../../../session/backlog.go#L50-L52)) — no `admin`/`operator`
  role exists anywhere in the session package or the MCP tool surface.
- Grepping `tools_backlog.go` for override/force/admin/manual/bypass turns up only prose in tool
  descriptions (e.g. "manual `gh pr create`" at line 213/1015) — no code path that skips a
  validation check for privileged callers.
- The six registered backlog MCP tools (`get_backlog_item`, `report_progress`,
  `request_review`, `submit_review_verdict`, `report_pr_created`, `submit_triage_result` —
  [`server/mcp/tools_backlog.go:922-1040`](../../../server/mcp/tools_backlog.go#L922)) are all
  role-gated, session-linked operations; none is an unauthenticated/operator-only escape hatch.
- **Implication for AC2**: a manual-override tool would be a genuinely new pattern in this file,
  not an existing one to imitate. However, the *write path* it would need is already shared and
  reusable: `Storage.SetBacklogItemPRAndTransition(ctx, itemID, prURL, prNumber, summary) error`
  ([`session/storage.go:758`](../../../session/storage.go#L758)) is the same primary-write
  function `reportPRCreated` calls today ([`server/mcp/tools_backlog.go:717`](../../../server/mcp/tools_backlog.go#L717))
  and that the reconciliation backstop also uses per its doc comment
  ([`server/mcp/tools_backlog.go:619-621`](../../../server/mcp/tools_backlog.go#L619-L621)). An
  override tool would still call this same function — it would just skip or replace the
  `VerifyPRMatchesBranch` gate (line 707) with a weaker but still GitHub-verified check (e.g.
  "does PR #N exist on GitHub at all and is it merged/open in this repo," via the already-existing
  `GetPRInfoCtx`), satisfying the requirement's constraint not to trust an unverified caller claim.

## Existing handler flow (for reference)

`reportPRCreated` ([`server/mcp/tools_backlog.go:623-726`](../../../server/mcp/tools_backlog.go#L623-L726)):
role check (line 669) → idempotency no-op check (line 682) → URL/number cross-check (line 691) →
resolve session's own tracked branch via `sessionBranch`/`GetWorktreeDataBySessionUUID` (line 702)
→ `verifyPR` → `VerifyPRMatchesBranch` → `GetPRForBranch` (line 707) → hard reject on mismatch
(line 711, the exact error text from the bug report) → `SetBacklogItemPRAndTransition` (line 717).
The error message at line 712-714 currently does **not** mention any workaround (AC3 requirement).

## Summary for plan phase

- **AC1 (relax branch match to ancestry check)**: primitives exist (go-git `IsAncestor`,
  `PRInfo.HeadSHA` addable via one `--json` field) but the specific recovery scenario in the bug
  report (PR cut fresh from `origin/main`, not from the polluted tracked branch) means a naive
  ancestry check against the tracked branch tip would still fail. Would need ancestry-against-
  `origin/main` instead, which is a materially weaker guarantee than "PR branch descends from
  item's own branch."
- **AC2 (manual override tool)**: no existing bypass pattern to copy, but the storage write path
  (`SetBacklogItemPRAndTransition`) is already shared/reusable, and a GitHub-verified-but-looser
  check (PR exists + is open/merged, via existing `GetPRInfoCtx`) is straightforward to build
  without introducing a Go GitHub client library or new transport.
- Both options are buildable with primitives that already exist in this repo; neither requires a
  new dependency.
