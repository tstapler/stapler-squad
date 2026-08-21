# Research: Stack — Linear & JIRA Go Clients

Agent 1 (Stack) research for `project_plans/linear-jira-integration`. Covers
library/client choice, versions, and auth schemes for the `LinearPlugin` and
`JiraPlugin` `session.ItemSourcePlugin` implementations.

## Existing pattern to match

`session/backlog_plugin_github.go`'s `GitHubIssuesPlugin` is hand-rolled
`net/http` + `encoding/json` — no GitHub SDK dependency, despite one being
available (`google/go-github`). Structure per plugin file:

- `<x>PluginConfig` struct decoded from `PluginConfig.Raw` JSON, with token
  resolved from `github.GetKeychainTokenForHost` first, config field as
  fallback (`decodeGithubIssuesFetchConfig`/`decodeGithubIssuesConfig`).
- A raw wire-shape struct (`githubIssue`) decoded straight off the response.
- `Fetch` (single page, cursor-bounded) + optional `FetchAll` (`PaginatedFetcher`,
  capped-page aggregation for preview-impact use cases).
- `convertGithubIssues`-style pure mapping function: wire shape → `ExternalItem`,
  computing the new cursor as `max(cursor, item.UpdatedAt)` string-wise (works
  because GitHub's `updated_at` is RFC3339, which sorts lexicographically).
- `CloseIssue`/`PostIssueComment` for forward-sync, each returning the
  tracker's own post-write timestamp (not local wall-clock) for the ADR-003
  watermark.
- `MapToBacklogItem` truncates title (200) / description (2000), sets
  `Status: string(BacklogStatusIdea)`.

`go.mod` currently has **no GraphQL library, no JIRA client, no Linear SDK** —
this integration starts from a clean slate on both fronts. It does already
depend on `golang.org/x/net`, `net/http` (stdlib), `encoding/json` (stdlib) —
everything the hand-rolled GitHub pattern needs is already present.

## Linear: hand-rolled GraphQL over `net/http`, not a client library

**Recommendation: match the GitHub pattern exactly — a small hand-rolled
GraphQL POST helper (`net/http` + `encoding/json`), no SDK, no codegen.**

Options considered:

| Option | Verdict |
|---|---|
| Hand-rolled `net/http` POST with `{"query":..., "variables":...}` JSON body | **Recommended.** Matches existing plugin style exactly; the plugin only needs 2-3 fixed queries/mutations (list issues, update issue state, — optionally add a comment via `commentCreate`), so there's no benefit to a generated client for such a small, static surface. |
| [`Khan/genqlient`](https://github.com/Khan/genqlient) | Rejected. Type-safe generated client requires a build-time codegen step (schema file + `.graphql` query files + `go generate`), a new dev-tool dependency, and CI wiring — disproportionate for 2-3 static operations. Would also be the *first* codegen-based client pattern in a codebase whose one existing external-API plugin is deliberately hand-rolled. |
| [`hasura/go-graphql-client`](https://github.com/hasura/go-graphql-client) | Rejected. Runtime reflection-based client (struct-tag-driven query building) — adds an external dependency for functionality (`http.Post` + JSON marshal/unmarshal) the codebase already writes by hand elsewhere. No maintenance-status concern, just unnecessary for this scope. |
| Official/community Linear Go SDK | None well-maintained exists. Linear does not publish an official Go SDK (their own docs recommend the raw GraphQL endpoint or their TypeScript SDK); community Go wrappers are thin, low-adoption, and would be an unvetted third-party dependency for a single-tenant internal tool. Not recommended. |

**Linear API specifics:**

- Endpoint: `https://api.linear.app/graphql` (single endpoint, POST only,
  `Content-Type: application/json`, body `{"query": "...", "variables": {...}}`).
  [Linear Developers — Getting started](https://linear.app/developers/graphql)
- Auth: `Authorization: <API_KEY>` header — **no `Bearer` prefix**, unlike
  most REST APIs (including JIRA's PAT scheme below). This is a real
  footgun to flag in the plan/implementation phase — copy-pasting the
  GitHub (`"token "+cfg.Token`) or JIRA (`"Bearer "+token`) header-format
  pattern verbatim onto Linear will silently 401. [Linear API Key guide](https://unified.to/blog/linear_api_key_how_to_generate_and_use_it_graphql_guide_for_developers)
- Personal API keys are long-lived, user-scoped, non-expiring by default,
  created under Settings > Account > Security & Access — matches this
  project's existing single-token-per-host keychain shape well (no OAuth
  refresh-token complexity needed for goal 1/2/3 in requirements.md).
- Pagination is cursor-based (`endCursor`/`hasNextPage`), no offset support —
  fits the existing cursor-in-`PluginConfig`/watermark pattern used for
  GitHub's `since` param, just via a GraphQL `after` variable instead of a
  REST query string.
- Rate limiting: 3,000,000 points/hour for API-key-authenticated requests,
  10,000-point cap per single query, cost model is ~0.1/property, ~1/object,
  multiplied by pagination size for connections. [Linear rate limiting docs](https://linear.app/developers/rate-limiting)
  Practical implication for the plugin: request explicit `first: N` page
  sizes (don't rely on the default), and sort/filter by `updatedAt` server-side
  (Linear's `issues(filter: {updatedAt: {gt: $cursor}})`) rather than
  fetching everything and filtering client-side — mirrors the GitHub
  plugin's `since=` incremental-fetch design goal (req. #3).
- Suggested query shape (illustrative, to refine in `plan.md`):
  ```graphql
  query Issues($after: String, $updatedAfter: DateTimeOrDuration) {
    issues(
      first: 50
      after: $after
      filter: { updatedAt: { gt: $updatedAfter } }
      orderBy: updatedAt
    ) {
      nodes {
        id identifier title description url
        state { name type }
        priority
        labels { nodes { name } }
        updatedAt
      }
      pageInfo { hasNextPage endCursor }
    }
  }
  ```
- Forward-sync (goal 3, requirement #5): a state change is a mutation
  (`issueUpdate(id: $id, input: { stateId: $stateId })`), plus a
  `commentCreate` mutation for the visible-comment convention. Both return
  the issue's own `updatedAt` in the mutation response payload — use that
  for the ADR-003 watermark, exactly like `CloseIssue` uses GitHub's
  response `updated_at` rather than local time.

## JIRA: `andygrunwald/go-jira/v2` (cloud or onpremise sub-package), not hand-rolled

**Recommendation: use `github.com/andygrunwald/go-jira/v2` — specifically
the `cloud` sub-package for Cloud, with the `onpremise` sub-package as the
model to add later if Server/Data Center support becomes a real requirement.**
This is a deliberate deviation from the GitHub plugin's hand-rolled style,
justified below.

| Option | Verdict |
|---|---|
| [`andygrunwald/go-jira`](https://github.com/andygrunwald/go-jira) v2 (`v2/cloud`, `v2/onpremise`) | **Recommended.** Actively maintained — v2.0.0 (migration guide dated 2026-01-12) was a deliberate rewrite specifically to track Cloud REST API v3 vs. Server/Data Center divergence with **separate packages**, `context.Context`-first method signatures throughout, and `IssueService.SearchV2JQL`/`SearchV2JQLWithContext` for the current (non-deprecated) search endpoint. The Cloud/onpremise split directly matches this project's `JIRA_BASE_URL`-supports-both requirement (see below) — the library has already done the "these are different APIs with different auth" modeling work this plugin needs. [go-jira MIGRATE.md](https://github.com/andygrunwald/go-jira/blob/main/MIGRATE.md), [pkg.go.dev](https://pkg.go.dev/github.com/andygrunwald/go-jira) |
| Hand-rolled REST client (GitHub-plugin style) | Viable fallback, not recommended as first choice. JIRA's REST surface (issue search/JQL, field schema quirks, ADF — Atlassian Document Format — for rich-text `description`/`comment` bodies) is materially more complex than GitHub's flat JSON REST API. Hand-rolling would mean reimplementing ADF marshaling and JQL query construction that `go-jira` already provides tested types for. Reasonable to fall back to this if `go-jira/v2`'s API turns out to be awkward for the specific fields this plugin needs — flag as a decision point for `plan.md`, not a blocker now. |
| `go-jira/v1` (`github.com/andygrunwald/go-jira`, pre-v2 import path) | Rejected — superseded by v2; v1's `Search()` targets JIRA's deprecated `/rest/api/2/search` GET-based search which Atlassian has been sunsetting in favor of the POST-based JQL endpoints v2's `SearchV2JQL` targets. |

**JIRA API specifics:**

- **Cloud vs Server/Data Center is a real fork, not a detail** — flag this
  explicitly for `plan.md`, since requirements.md's `JIRA_BASE_URL` env var
  implies supporting both:
  - **Cloud**: base URL `https://<site>.atlassian.net`, REST API v3 at
    `/rest/api/3`, auth = HTTP Basic with **email as username + API token as
    password** (`Authorization: Basic base64(email:token)`). API tokens
    created at `id.atlassian.com`. Atlassian is phasing in scoped API
    tokens (narrower permissions) over classic unscoped ones — worth a
    forward-looking note but not a blocker. [Atlassian Cloud basic-auth docs](https://developer.atlassian.com/cloud/jira/software/basic-auth-for-rest-apis/)
  - **Server/Data Center**: auth = Personal Access Token (PAT) as a Bearer
    token (`Authorization: Bearer <PAT>`), **no email needed** — structurally
    different from Cloud's basic-auth-with-email scheme, not just a
    different token format. [Atlassian Server PAT docs](https://developer.atlassian.com/server/jira/platform/basic-authentication/)
  - Practical implication: `JiraPluginConfig` needs a discriminator (either
    an explicit `deployment_type: "cloud"|"server"` field, or infer from
    whether `JIRA_EMAIL` is set) so the plugin picks Basic-with-email vs.
    Bearer-without-email — this can't be papered over as "just try both."
    `go-jira/v2`'s `cloud`/`onpremise` package split maps onto exactly this
    fork, which is the strongest argument for using the library over
    hand-rolling: the auth-scheme divergence is handled at the package-choice
    level rather than needing bespoke conditional logic in this plugin.
- Incremental fetch / cursor: JIRA JQL supports `updated >= "<timestamp>"`
  in the query string, ordered `ORDER BY updated ASC`, paginated via
  `nextPageToken`-based cursor on the current search endpoints (the old
  `startAt` offset-based pagination is deprecated on the JQL search API
  path `go-jira/v2` targets) — fits the same cursor-column pattern as
  GitHub's `since=` and Linear's `after`.
- Forward-sync (goal 3, requirement #5): status transition is
  `POST /rest/api/3/issue/{id}/transitions` (JIRA models "close/resolve" as
  a workflow transition, not a direct status field write — transitions are
  workflow-specific, so the plugin needs to resolve the available transition
  ID by name/target-status rather than assuming a fixed transition ID),
  and `POST /rest/api/3/issue/{id}/comment` for the visible-comment
  convention (comment `body` must be ADF, not plain text — `go-jira/v2`
  likely has ADF helper types worth checking; the GitHub plugin's plain-text
  `PostIssueComment` doesn't have an ADF-equivalent problem, which is another
  spot this integration is not a straight one-to-one copy of the GitHub
  pattern). Response-side, the transitioned/commented issue's own
  post-write timestamp (from a follow-up `GET` or where the API returns it
  inline) should back the ADR-003 watermark, same as `CloseIssue` does for
  GitHub.

## Credential storage (requirement #6)

Both plugins should follow `github.GetKeychainTokenForHost`'s shape rather
than reinvent one:

- `LINEAR_API_KEY` → single token, no host variance (Linear has one fixed
  API host) — simpler than GitHub's per-host case; likely needs a small
  Linear-specific keychain helper (e.g. `linear.GetKeychainToken()`) styled
  after `github.GetKeychainToken()`'s legacy single-slot path, not the full
  per-host/per-account machinery GitHub needs for GHES.
- `JIRA_BASE_URL` / `JIRA_EMAIL` / `JIRA_API_TOKEN` → JIRA is multi-tenant by
  base URL (unlike Linear), so this needs the per-host shape
  (`GetKeychainTokenForHost`-equivalent), keyed by `JIRA_BASE_URL`'s host,
  storing the `(email, token)` pair or `(PAT)` depending on deployment type.
  This is closer to GitHub's per-host model than Linear's single-token case.
- Neither should land in `PluginConfig.Raw` JSON in plaintext, per
  requirement #6 — config should carry non-secret fields only
  (owner/repo-equivalents: Linear team ID/key, JIRA project key, base URL,
  label→priority map), with credentials resolved from keychain at fetch
  time exactly like `decodeGithubIssuesFetchConfig` does today.

## go.mod impact

Adding `andygrunwald/go-jira/v2` is the only new direct dependency this
plan needs — Linear's client stays dependency-free (stdlib `net/http` +
`encoding/json` only, no new `require` line). Run
`go get github.com/andygrunwald/go-jira/v2` and pin to the latest v2.x
tag at implementation time (recommendation, not implementation — this
research doc does not touch `go.mod`).

## Summary of recommendations for `plan.md`

1. **Linear**: hand-rolled GraphQL client, one internal helper
   (`session/backlog_plugin_linear.go`, package-level `linearGraphQLURL`
   override var for tests, mirroring `githubAPIBaseURL`), auth header
   `Authorization: <key>` (no `Bearer`/`token` prefix — the one true footgun
   to flag loudly in the plan/implementation phase).
2. **JIRA**: adopt `andygrunwald/go-jira/v2`, `cloud` sub-package first
   (Data Center/`onpremise` as a documented but not-yet-implemented
   extension point), with an explicit Cloud-vs-Server discriminator in
   `JiraPluginConfig` driving Basic+email vs Bearer-PAT auth selection.
3. **Credentials**: new small Linear keychain helper (single-token,
   Linear-specific service/key) + a JIRA keychain helper shaped like
   `GetKeychainTokenForHost` (per-`JIRA_BASE_URL`-host), neither stored in
   `PluginConfig.Raw`.
