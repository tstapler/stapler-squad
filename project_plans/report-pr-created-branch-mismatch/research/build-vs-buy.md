# Build vs. Buy: report_pr_created branch-mismatch fix

## Verdict up front

**Build.** This is validation logic in a single internal MCP tool handler
(`server/mcp/tools_backlog.go:623`, `reportPRCreated`). There is no external
system to integrate, no vendor evaluation to run, and the "buy" framing
doesn't really apply — the only live question is *which existing internal
primitive* (go-git ancestry vs. a `gh`/GitHub-API comparison call) the fix
should be built on. That's what the rest of this doc answers.

## Root cause, precisely

`VerifyPRMatchesBranch` (`server/mcp/tools_github.go:272`) calls
`githubpkg.GetPRForBranch(ctx, owner, repo, expectedBranch)`
(`github/client.go:378`). `GetPRForBranch` queries the GitHub REST API with
`head=owner:expectedBranch` as a **server-side filter**:

```go
apiPath := fmt.Sprintf("repos/%s/%s/pulls?head=%s&state=all&per_page=10",
    url.PathEscape(owner), url.PathEscape(repo),
    url.QueryEscape(owner+":"+branch))
```

If the PR's actual head branch differs from `expectedBranch` (the backlog
item's tracked branch), GitHub returns zero results and `GetPRForBranch`
returns `githubpkg.ErrNoPR` — which `VerifyPRMatchesBranch` maps to
`(false, nil)`, a "definitive mismatch." `reportPRCreated` then hard-rejects
with no path to override. The check never even looks at what branch the PR
*actually* has — it just asks GitHub "is there a PR whose head is exactly
branch X," which is the wrong question once the real head branch has
diverged from the tracked one.

## Q1 — Does go-git give ancestry/merge-base for a remote GitHub branch, and is a local clone available in the MCP handler?

Yes to both, with one caveat (a small incremental fetch is needed).

- **API surface exists.** `go-git v5.14.0`
  (`github.com/go-git/go-git/v5/plumbing/object/merge_base.go`) exposes,
  on `*object.Commit`:
  - `func (c *Commit) IsAncestor(other *Commit) (bool, error)`
  - `func (c *Commit) MergeBase(other *Commit) ([]*Commit, error)`

  Both operate on any two commit objects already present in the local
  object store — they are not restricted to local branch refs. This is
  exactly the pattern `session/git/ops.go`'s `IsCommitOnMain` (lines 47–85)
  already uses: open the repo (`git.PlainOpenWithOptions` with
  `DetectDotGit`), best-effort `repo.Fetch` a specific ref, resolve both
  commits, call `IsAncestor`.

- **A local clone is available in the handler's request context.**
  `reportPRCreated` already resolves the calling session's own worktree via
  `h.sessionBranch` → `h.storage.GetWorktreeDataBySessionUUID` (`tools_backlog.go:592`),
  and that worktree data (`session/worktree_pr_poller.go:22-24`) carries both
  `RepoPath` and `WorktreePath`. This is the *same* repo clone the polluted
  tracked branch and the clean recovery branch both live in (or can be
  fetched into) — no new clone is required, only an extra `git fetch` of one
  ref, mirroring what `IsCommitOnMain` already does for `mainBranch`.

- **The caveat:** unlike `IsCommitOnMain` (which always fetches a *known*
  branch name, `mainBranch`), here the PR's actual head branch name has to
  be learned first (see Q2 — `gh pr view <N> --json headRefName`), then
  fetched by name into the local object store before `IsAncestor`/`MergeBase`
  can see it. That's one extra step versus `IsCommitOnMain`, but it's the
  same `config.RefSpec("+refs/heads/%s:refs/remotes/origin/%s", ...)` +
  `repo.Fetch` idiom already in `ops.go:53-54`, just parameterized on a
  branch name discovered at runtime instead of hardcoded to main.

- **Real limitation to flag for the plan phase (not a go-git gap, a semantic
  one):** the bug report's actual failure mode is a PR opened from a
  **clean branch cut from `origin/main`** with the work re-committed fresh —
  not a branch that literally contains the polluted tracked branch's commits
  as ancestors. Pure `IsAncestor`/`MergeBase` between the tracked branch tip
  and the PR head branch tip may find *no* git-level relationship at all in
  that scenario, even though the PR is legitimate. Ancestry checking answers
  "is the PR head literally downstream of the tracked branch" — it does not
  answer "is this PR morally the same piece of work." The plan phase should
  decide whether to (a) treat "both branches diverge from a common
  origin/main ancestor within N commits, same session/base commit" as good
  enough evidence, or (b) lean more on the manual-override path (acceptance
  criterion 2) for this specific fresh-branch case, using ancestry checking
  only for the "branch was rebased/renamed but still shares history" case.

## Q2 — Lighter-weight alternative reusing the existing `gh`/GitHub-API call patterns

Yes, and it's arguably the better first primitive because it fixes the
*actual* root cause (the handler never looks at the PR's real head branch)
without touching git internals at all:

- `GetPRInfoCtx` (`github/client.go:247`, used throughout the package) already
  fetches `headRefName` and `baseRefName` for a **specific PR number** via
  `gh pr view <N> --repo <owner>/<repo> --json headRefName,baseRefName,...`
  and returns them on `PRInfo.HeadRef`/`PRInfo.BaseRef` (`github/client.go:44-45`).
  Since `reportPRCreated` already has `prNumber` from the caller's self-report,
  it can call this directly (or a trimmed sibling) instead of routing through
  `GetPRForBranch`'s branch-filtered list endpoint. This alone lets the
  handler see "the PR really exists, and its real head branch is Y" instead
  of silently getting zero results when `Y != expectedBranch`.
- For a genuine ahead/behind/diverged comparison between two branches without
  pulling any git objects locally, the codebase's existing native-REST-call
  convention (`ghHTTPClient` + `newGHRequest`, `github/http_client.go:13,54` —
  already used by `GetPRForBranch` itself, see `github/client.go:383-388`)
  extends directly to GitHub's compare endpoint,
  `GET repos/{owner}/{repo}/compare/{base}...{head}`, which returns
  `status` (`identical`/`ahead`/`behind`/`diverged`), `ahead_by`, `behind_by`,
  and `merge_base_commit`. No such call exists in the codebase yet, but it's
  a one-function addition following the exact pattern already at
  `github/client.go:378-428` (`GetPRForBranch`) — same auth, same error
  handling, same JSON-unmarshal-into-struct shape.
- Either of these avoids the local-fetch step Q1 needs, and both are cheaper
  network calls than the current `pulls?head=` list query which was actually
  the wrong endpoint for this use case anyway.

## Q3 — No new external dependency needed

Confirmed. Between `go-git` (already a direct dependency, `go.mod:17`,
already used for ancestry checks in `session/git/ops.go`) and the existing
`gh` CLI wrapping / native GitHub REST call convention (`github/client.go`,
`github/http_client.go`), the codebase already has both a git-object-level
primitive and a GitHub-API-level primitive that can answer "is this PR
related to that branch." There is no gap that would justify pulling in a
dedicated GitHub API SDK (e.g. `google/go-github`) or any other new library
— this is a same-day fix within one internal tool file plus, at most, one
new small function in `github/client.go` following patterns that already
exist twice over in that file.

## Q4 — Recommendation for the plan phase

Build the fix on **`GetPRInfoCtx`/`PRInfo.HeadRef` first** (Q2, first bullet)
to fix the actual root cause: `reportPRCreated` should look up the PR by
number directly (it already has `prNumber`), read the PR's real
`headRefName`, and only then decide whether that differs from the tracked
branch. If it matches, behavior is unchanged (fast path, no new network
calls beyond what's already made). If it differs, layer in **one** of the
two relatedness checks as the second primitive — the GitHub `compare`
endpoint (Q2, second bullet) is the lighter-weight, easier-to-test option
since it needs no local fetch and mirrors `GetPRForBranch`'s existing
REST-call shape almost line for line; `go-git`'s `IsAncestor`/`MergeBase`
against the session's own worktree clone (Q1) is the fallback if the plan
phase decides commit-object-level proof is worth the extra local fetch.
Either way, satisfy acceptance criterion (2) — a manual-override path — as a
belt-and-suspenders escape hatch for the "fresh branch, no real git
ancestry" case Q1 flagged, and rewrite the rejection message
(`tools_backlog.go:712-714`) to name both the real head branch it found and
the override path, satisfying acceptance criterion (3) regardless of which
relatedness check ships.
