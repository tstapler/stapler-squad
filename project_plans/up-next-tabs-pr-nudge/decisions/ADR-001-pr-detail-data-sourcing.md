# ADR-001: PR detail data sourcing (counts in poll, bodies lazy)

**Status**: Accepted | **Date**: 2026-10-05

## Context
Cards and tab badges need failing check names, unresolved thread count and merge-conflict state for every open PR. Comment bodies are only needed to build a nudge prompt. GraphQL has no ETag/304 savings, so cost is in query points. The viewer poll (`github/user_pr_cache.go:515`) already runs one GraphQL query per account/host and reads `statusCheckRollup{state}` only; no `reviewThreads` exists anywhere in `github/*.go`.

## Decision
1. Widen the existing viewer poll with `mergeable`, `reviewThreads(first:50){totalCount nodes{isResolved isOutdated}}` and `statusCheckRollup.contexts(first:25)` (check names/urls only). No extra requests; uses `newGHGraphQLRequestForHostWithToken` (norawghrequest rule).
2. Fetch fresh check state and unresolved-thread links (url, path, author login; **never comment bodies**) lazily at nudge click via a new token-taking `FetchPRNudgeDetail` query (`GetPRInfoGraphQL` takes no token, `github/client_graphql.go:205,222`, so it is not reused), using the token of the account that owns the PR (`UserPR.account_login`/`host`), through `AdmitOrigin` with a distinct call origin.
3. Expose unknown as unknown: `optional int32 unresolved_thread_count` plus `details_loaded`; the UI shows "?" not "0". Cap at 50 ("50+"); failing checks capped at 10.
4. Unresolved = `isResolved=false AND isOutdated=false`. REST review comments cannot express resolution.

## Alternatives rejected
- Lazy-only (counts fetched on card expand): badges would be wrong for collapsed PRs.
- Per-PR query every poll: N+1 quota burn.
- go-github / `gh` CLI: bypass ETag/rate-limit layer and the lint rule.

## Consequences / fallback
Task 1.2.1a measures `rateLimit{cost}` first and gates Story 1.2.1. Threshold: > 50 points per poll selects **degraded mode**: thread counts leave the poll (keep check names and mergeable), `details_loaded=false`, counts fetched lazily on card expand/nudge. Degraded UI contract: the thread chip reads "?", the tab badge counts failing CI + merge conflicts + changes-requested only and shows a trailing "+" ("3+"). `mergeable: UNKNOWN` maps to unknown (`optional bool has_merge_conflict` unset), never `false`, in both modes.
Product behavior of the fallback is specified in `requirements.md` ("Product fallback if the GraphQL cost gate fails"). There is no 304/conditional-request saving on the GraphQL poll, so no test claims one; the guard is "one request per account/host per poll".
Evidence: `research/architecture.md` sections 2 and 7, `research/pitfalls.md` section 2, `research/build-vs-buy.md` (b).

## Task 1.2.1a measurement (2026-10-07)
Widened query (`mergeable`, `reviewThreads(first:50)`, `contexts(first:25)`, `rateLimit{cost remaining limit nodeCount}`) run once against github.com with the user's real token (25 open PRs, `first:100` page): `rateLimit.cost` = **4**, `nodeCount` = 9,700 (500,000 ceiling; PRs x 75 upper bound = 1,875 for 25 PRs, 7,500 for the 100-PR page). Poll budget: 4 points x 30 polls/hour (2 min interval, unchanged) = 120 points/hour = 2.4% of the 5,000-point limit. Both gates pass (<= 50 per poll, <= 10% hourly), so **FULL mode is selected**; degraded mode stays implemented behind the flag.
Gap: the github.netflix.net host returned HTTP 502 at measurement time, so GHE cost was not measured; the per-host legacy-query fallback (Story 1.2.2) covers a GHE that rejects the widened query. Nudge-fetch cost (<5 points target) was not separately measured.
