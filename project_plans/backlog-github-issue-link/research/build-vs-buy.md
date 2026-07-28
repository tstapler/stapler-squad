# Research: Build vs. Buy — `closingKeywordFor` URL-shape detection

Agent 6 (Build vs. Buy). Scope: the only piece of this feature with any
"logic" — distinguishing a GitHub issue URL
(`https://github.com/owner/repo/issues/42`) from a PR URL
(`https://github.com/owner/repo/pull/42`) inside a `closingKeywordFor(url
string) string` helper that returns `"Fixes "` / `"Related: "`.

## 1. Existing OSS library in `go.mod`?

Checked `go.mod` in full (191 lines, all direct + indirect requires). There is
**no GitHub API client library** in the dependency tree at all:

- No `github.com/google/go-github` (the canonical Go GitHub SDK, which does
  have URL/reference helpers but is primarily a full REST/GraphQL client —
  massive surface area for this use case).
- No `github.com/shurcooL/githubv4`, no `github.com/xanzy/go-gitlab`
  equivalent, no lightweight "parse a GitHub URL" micro-library either.

The only GitHub-adjacent things in `go.mod` are transitive/incidental:
`github.com/cli/...` is *not* present; nothing resembling a GitHub URL parser
exists anywhere in the tree. Pulling in `google/go-github` (a ~14k-file,
multi-hundred-KB dependency covering issues, PRs, repos, actions, webhooks,
etc.) solely to classify two URL path shapes would be a large, unjustified
dependency for a one-line string check.

**Verdict: no existing library is already available, and none is worth adding.**

## 2. Stdlib-only (`net/url` + `strings`) — is it sufficient?

Yes. The only two shapes that matter, per the requirements and the actual
data source (GitHub API `html_url` field, already assembled by GitHub
itself), are:

```
https://github.com/{owner}/{repo}/issues/{number}
https://github.com/{owner}/{repo}/pull/{number}
```

Both are produced server-side by GitHub's API (`issue.HTMLURL` /
`pr.HTMLURL` per `session/backlog_plugin_github.go` /
`backlog_plugin_github_prs.go`), not typed by a user, so there's no need to
defend against adversarial/malformed input, alternate GitHub Enterprise
hostnames, `pulls` vs `pull` API-vs-web inconsistencies, or query-string
noise — GitHub has never used any other path segment for these two resource
types on `github.com` web URLs.

`net/url.Parse` + inspecting `u.Path` for `/issues/` vs `/pull/` (or even
simpler, a raw `strings.Contains` on the full string) is sufficient. Using
`net/url` at all is arguably unnecessary ceremony here since there's no need
to extract query params, host, or scheme — a substring check on the raw
string is equally correct and simpler.

**Verdict: stdlib is sufficient; in fact even bare `strings.Contains` without
`net/url` is sufficient**, since nothing downstream needs owner/repo/number —
`closingKeywordFor` only needs to pick between two literal prefixes.

## 3. Bespoke string-matching vs. regex vs. library — correctness risk

Given exactly 2 known shapes, both first-party GitHub-generated HTML URLs
(not arbitrary user input, already validated as `html_url` by GitHub's own
API response):

- **`strings.Contains(url, "/pull/")` else `"/issues/"` fallback**: Lowest
  risk. No backtracking, no compile step, trivially unit-testable with a
  handful of table cases (issue URL, PR URL, and the empty/fallback case).
  The only correctness subtlety is ordering: check `/pull/` first (or check
  both and default sensibly) since an issue URL can never contain `/pull/`
  and vice versa on `github.com` — there's no ambiguity to resolve.
- **Regex** (e.g. `regexp.MustCompile(`/(issues|pull)/\d+`)`): Strictly more
  machinery than needed — compiling a regex to distinguish two literal
  substrings adds a dependency on correct regex syntax, a compile-time or
  init-time cost, and a harder-to-read helper for zero additional
  correctness. Regex would earn its keep if the shapes were more varied
  (e.g., needing to also capture owner/repo/number for use elsewhere), but
  nothing downstream in these requirements needs extraction — only the
  literal keyword selection.
- **Library** (`go-github` or similar): Massive overkill per section 1 —
  would add a large dependency, its own transitive deps, and API surface
  wildly beyond what's needed, to replace a two-branch string check.

**Recommendation: a plain `strings.Contains(url, "/pull/")` check (with
`/issues/` as the other recognized/fallback case) is the simplest approach
that is still fully correct** for this narrow, trusted-input, two-shape
problem. This is a plumbing/naming decision, not an algorithmically hard
problem — regex and libraries both add unjustified complexity for the actual
requirement.

## 4. Existing convention: do `GitHubIssuesPlugin`/`GitHubPRsPlugin` already avoid a GitHub SDK?

Confirmed via `session/backlog_plugin_github.go`: both plugins are built on
raw `net/http` + hand-rolled JSON response structs (`githubIssue`,
`githubAPIURL` helper joining `githubAPIBaseURL` + path), not any GitHub SDK.
The file's imports are exactly `context`, `encoding/json`, `fmt`, `io`,
`net/http`, `strconv`, `strings`, `time` — all stdlib. `go.mod` corroborates
this: no GitHub client library is a dependency anywhere in the module.

This confirms the codebase has an established, deliberate convention of
avoiding a GitHub SDK dependency in favor of stdlib `net/http` + minimal
hand-rolled structs, even for the full issue/PR-fetching logic that's
considerably more involved than a URL-shape check.

## Recommended verdict

**Stdlib only, no new dependency.** Implement `closingKeywordFor` as a small
pure function using `strings.Contains` (optionally `net/url.Parse` +
`u.Path` if the team prefers structural inspection over raw substring
matching, but not required for correctness) — no regex, no GitHub SDK. This
is consistent with the existing `GitHubIssuesPlugin`/`GitHubPRsPlugin`
convention of stdlib-only GitHub integration, avoids importing a
multi-hundred-KB dependency for a two-branch string check, and is the
simplest implementation that remains fully correct given the input is
first-party, GitHub-API-generated `html_url` data with exactly two known
shapes.

Example shape (illustrative only — not a specification of the final
implementation, which belongs to the planning/implementation phase):

```go
func closingKeywordFor(url string) string {
    if strings.Contains(url, "/pull/") {
        return "Related: "
    }
    return "Fixes "
}
```
