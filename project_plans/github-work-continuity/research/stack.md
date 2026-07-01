# Stack Research: GitHub Work Continuity

**Date:** 2026-06-24
**Researcher:** research agent (automated)
**Scope:** Evaluate GitHub data integration approaches for listing/tracking PRs authored by the user across repos.

---

## 1. gh CLI subprocess vs. direct REST HTTP client

The codebase uses **both** approaches today with a clear, documented split:

| Approach | Used for | File |
|---|---|---|
| `gh` CLI subprocess | `GetPRInfoCtx` (reviews + statusCheckRollup), `GetPRComments`, `GetPRDiff`, `PostPRComment`, `MergePR`, `ClosePR`, `CloneRepository`, `IsForkRepo` | `github/client.go` |
| Native `net/http` via `ghHTTPClient` | `GetPRForBranch` (branch → PR lookup), `GetPRInfoConditional` (ETag polling) | `github/client.go`, `github/etag_cache.go` |

Comments in `GetPRForBranch` and `etag_cache.go` explicitly state the rationale: "Uses native net/http instead of a gh subprocess to **avoid forkExec lock contention**." This is the established pattern for any hot polling path.

### (a) Listing all PRs authored by user across repos

**Recommendation: REST HTTP client (native), not gh subprocess.**

- The REST Search API (`GET /search/issues?q=is:pr+author:LOGIN+is:open`) returns all open PRs for a user across all repos in a single paginated call with no per-repo iteration needed. It requires no subprocess fork.
- The gh CLI equivalent (`gh pr list --author @me --state open`) only searches within a single repo context and would require enumerating repos first, making it worse for cross-repo discovery.
- The Search API is rate-limited separately (30 req/min for authenticated search queries, not counted against the 5000/hr REST quota), so this is actually the cheapest path for a periodic full refresh.
- **Caveat:** The Search API returns `issues` endpoint shape (not `pulls` shape), so it lacks `headRefName`/`baseRefName`/`mergeable`. A follow-up `GET /repos/{owner}/{repo}/pulls/{number}` (or the ETag conditional variant) is needed for full `PRInfo`. This is fine since the list refresh is infrequent (5 min cadence).

### (b) Per-session PR status polling

**Recommendation: REST HTTP client with ETag caching (already implemented).**

`GetPRInfoConditional` in `etag_cache.go` is exactly the right tool: it sends `If-None-Match` headers so unchanged PRs cost **zero rate-limit quota** (GitHub 304 responses are exempt). For reviews and CI status, which require `statusCheckRollup` (not available in the raw REST pulls endpoint), the existing pattern falls through to a `gh pr view --json` subprocess only when the ETag check confirms a change. This is the correct hybrid: cheap conditional check first, expensive subprocess only on actual change.

### (c) Fetching user identity

**Recommendation: REST HTTP client (`GET /user`).**

The `newGHRequest` helper in `http_client.go` already provides an authenticated request builder. A single `GET /user` call returns `login`, `name`, `email`, `avatar_url`. This should be cached with a long TTL (e.g., the existing `ghTokenTTL` of 1 hour is appropriate). No subprocess needed. The `gh api user --jq .login` pattern visible in `GetPRForBranch` would also work but introduces unnecessary fork overhead for a simple identity lookup.

---

## 2. ETag-based conditional polling status

**Already fully implemented.** `github/etag_cache.go` contains a complete, production-quality ETag cache:

- `ETagCache` struct with `sync.RWMutex` for concurrent access
- `GetPRInfoConditional` function implementing the full conditional request lifecycle: sends `If-None-Match`, handles `304 Not Modified` (returns cached value, no rate-limit cost), handles `200 OK` (stores new ETag + fetches fresh data), handles auth errors explicitly
- The comment block documents the design intent clearly: "304 Not Modified responses that cost zero rate-limit quota when the PR has not changed"

The ETag cache is scoped to `(owner, repo, prNumber)` keys. It will need to be extended or a parallel cache created for the "list all user PRs" result set (which has a different cache key shape: `author:LOGIN`).

---

## 3. GitHub GraphQL API vs. REST for "list all my PRs"

**GraphQL is worth using for the full cross-repo PR list, but not for the per-PR polling path.**

### The case for GraphQL here

The `search` GraphQL query can return PR fields including `headRefName`, `baseRefName`, `reviewDecision`, and `statusCheckRollup` in **a single call**, eliminating the two-phase REST approach (search → per-PR detail fetch):

```graphql
{
  search(query: "is:pr is:open author:LOGIN", type: ISSUE, first: 50) {
    nodes {
      ... on PullRequest {
        number
        title
        headRefName
        baseRefName
        state
        isDraft
        reviewDecision
        statusCheckRollup { contexts { ... } }
        repository { nameWithOwner }
      }
    }
  }
}
```

This is 1 call vs. N calls (1 search + N detail fetches). For a user with 20 open PRs, that is 20x fewer API calls for the refresh cycle.

### The case against (for this codebase)

- No GraphQL client library exists in `go.mod`. The codebase uses raw `net/http`; adding GraphQL requires either a library (e.g., `shurcooL/githubv4`) or raw HTTP POST with JSON body construction — both are non-trivial additions.
- The existing `gh CLI` subprocess can execute GraphQL queries via `gh api graphql` (already used and allowlisted in the classifier — see `classifier.go` line 927). This is a zero-dependency way to run GraphQL queries but is a subprocess, which conflicts with the hot-path philosophy.
- **Verdict:** For the 5-minute full-list refresh (low frequency, not hot path), using `gh api graphql -f query=...` via subprocess is acceptable and avoids adding a library. For a future high-frequency path, a direct GraphQL POST via `net/http` would be better. Do not add `shurcooL/githubv4` unless the query complexity grows significantly.

---

## 4. Existing GitHub API libraries

**No `google/go-github` or GraphQL client library in `go.mod`.** The direct dependencies relevant to GitHub are:

- `golang.org/x/sync` (singleflight, already used for token dedup and auth dedup)
- `go-git/go-git/v5` (local git operations only, not GitHub API)

**Adding `google/go-github` is not worth it.** The codebase has a tight, focused HTTP client abstraction (`http_client.go` + `etag_cache.go`) that is already idiomatic and well-commented. `google/go-github` v60+ is a large dependency (~200 types) that would introduce significant transitive bloat and a different request/response model from what is established. The raw HTTP approach is simpler and already proven in the codebase.

**Decision: keep raw `net/http` for REST, use `gh api graphql` subprocess for the infrequent full GraphQL query.**

---

## 5. Rate limit budget analysis

### Assumptions
- 15 sessions polled every 60 seconds (per-PR ETag conditional checks)
- 1 full PR list fetch every 5 minutes
- Authenticated token (5000 req/hr REST quota; Search API separate at 30 req/min)

### Per-session polling (ETag conditional)

- Polls per hour: 15 sessions × (3600s / 60s) = **900 polls/hr**
- With ETag: unchanged PRs return 304 and cost **0 rate-limit quota** per GitHub docs. Only changed PRs consume quota. Assuming 10% change rate (conservative): ~90 actual-cost requests/hr.
- Even worst-case (all PRs always changing): **900 req/hr** of the 5000 budget.

### Full PR list refresh

- Calls per hour: 60 min / 5 min = 12 refreshes/hr
- Using REST Search API: 12 req/hr (against the 30 req/min Search quota — well within limits)
- Using GraphQL (gh api graphql): 12 GraphQL calls/hr — GraphQL is rate-limited at 5000 "points" per hour, with a single query typically costing 1 point. Negligible.
- Per-PR detail fetch if using REST two-phase: 12 × (avg 20 open PRs) = 240 req/hr

### Total worst-case REST budget

| Source | Req/hr |
|---|---|
| ETag polling (all PRs always changing) | 900 |
| Full list refresh (REST Search API) | 12 |
| Per-PR detail after list (worst case) | 240 |
| Identity fetch (cached 1hr) | 1 |
| **Total** | **~1,153 req/hr** |

**This is 23% of the 5000 req/hr limit.** With realistic ETag cache hit rates (>80% steady state), actual usage will be closer to 400–500 req/hr. There is comfortable headroom even if session count doubles.

### Rate limit monitoring recommendation

Add `X-RateLimit-Remaining` header reading to `newGHRequest` responses and expose a metric/log warning when remaining < 500. The existing `ghHTTPClient` response path in `GetPRForBranch` already reads the response body; a small helper to extract rate limit headers would complete the picture.

---

## Summary

**Key findings:**

1. **Use native REST HTTP client for all new hot-path operations.** The existing `http_client.go` + `etag_cache.go` infrastructure is the correct foundation. ETag conditional polling is already fully implemented and will keep polling costs near zero for unchanged PRs. The `gh` CLI subprocess pattern should only be used for operations that genuinely require it (reviews/statusCheckRollup via `gh pr view --json`) and are triggered by confirmed changes, not on the polling interval itself.

2. **GraphQL for cross-repo PR list is worth it but only via `gh api graphql` subprocess** (already allowlisted in the command classifier). The single-call advantage (1 GraphQL call returning all PR fields including CI/review status) vs. two-phase REST (search + N detail fetches) is significant for the 5-minute refresh cycle. Do not add `shurcooL/githubv4` or `google/go-github` — the raw HTTP approach is already idiomatic in this codebase and neither library justifies the dependency weight.

3. **Rate limit budget is not a concern at current scale.** Worst-case: ~1,150 req/hr against a 5,000/hr limit (23%). With ETag caching in steady state, expect 400–500 req/hr. The Search API for the full list refresh is accounted for separately (12 calls/hr vs. 1,800/hr limit). Add `X-RateLimit-Remaining` header monitoring as a defensive measure.
