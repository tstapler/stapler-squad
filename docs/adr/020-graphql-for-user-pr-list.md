# ADR-020: GraphQL via Direct HTTP for User PR List

**Status**: Accepted (revised 2026-06-24 — subprocess approach superseded)
**Date**: 2026-06-24

## Context

The github-work-continuity feature needs to fetch the authenticated user's open pull requests across all repositories. The GitHub API offers two approaches:

1. **REST**: Use the search endpoint (`GET /search/issues?q=is:pr+is:open+author:@me`) to get a list of PRs, then issue N additional `GET /repos/{owner}/{repo}/pulls/{pull_number}` requests to retrieve the detail fields (head branch, merge status, review state, etc.) needed by the feature.

2. **GraphQL**: Use a `POST https://api.github.com/graphql` request with a single query on `viewer.pullRequests` to return all needed fields — title, URL, state, headRefName, repository name and owner, reviewDecision, mergeable — in one round trip.

Features must use **direct GitHub HTTP API calls** (using `ghHTTPClient` and `newGHRequest` / `newGHPostRequest` from `github/http_client.go`), not `gh` subprocess calls. Subprocess-based features are not distributable and introduce forkExec lock contention. See `CheckGHAuth` in `github/client.go` for the canonical example of converting a subprocess call to a direct HTTP API call.

## Decision

Use a direct `POST https://api.github.com/graphql` HTTP call (via `ghHTTPClient`) to fetch the authenticated user's open PRs across all repositories in a single call.

The query targets `viewer { pullRequests(first: 100, states: [OPEN]) { nodes { ... } } }` and requests exactly the fields the enrichment layer needs. Pagination via `pageInfo.endCursor` is implemented for users with more than 100 open PRs, but a single call covers the overwhelming majority of cases.

The token is sourced from `getGHToken(ctx)` (GITHUB_TOKEN env → GH_TOKEN env → `gh auth token` cached 1 hour). The request is constructed using a `newGHPostRequest` helper in `github/http_client.go` that mirrors `newGHRequest` but uses `POST` with a JSON body.

## Consequences

### Positive
- One network round trip regardless of how many repositories the user has open PRs in; REST would require 1 search call + N detail fetches.
- All required fields are returned in a single response; no secondary requests are needed to enrich individual PRs.
- No subprocess invocation — no forkExec contention, distributable without requiring `gh` installed.
- The query shape is explicit and versioned in source — the fields fetched are visible without inspecting a chain of REST calls.
- Consistent with `newGHRequest` / `ghHTTPClient` pattern used by `GetPRInfoConditional` and `CheckGHAuth`.

### Negative / Risks
- GitHub's GraphQL API enforces a complexity budget per query; very large result sets (hundreds of open PRs) require cursor-based pagination, adding conditional logic compared to a flat REST list.
- GraphQL errors are returned with HTTP 200 and a top-level `errors` array, requiring explicit error checking rather than relying on non-2xx status codes.
- The query must be updated if required fields change; REST endpoints are more stable across API versions.
- Requires a `newGHPostRequest` helper in `github/http_client.go`.

### Mitigations
- Pagination is implemented from the start using `pageInfo { hasNextPage endCursor }`, so large PR counts are handled correctly.
- The GraphQL wrapper checks for a top-level `errors` key in the JSON response and surfaces them as Go errors before the caller processes any data.
