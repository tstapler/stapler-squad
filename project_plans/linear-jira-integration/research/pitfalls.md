# Research: Pitfalls — Linear/JIRA Integration

Agent 4 (Pitfalls). Grounds every finding in either code already read in this
repo (cited by path:line) or a live web search (cited by URL) — no claim
here is from training-data memory alone.

## 1. Loop-prevention watermark (ADR-003 equivalent)

`GitHubIssuesPlugin.CloseIssue` (`session/backlog_plugin_github.go:291-356`)
returns the issue's post-close `updated_at` **from GitHub's own response
body**, specifically so the forward-sync subscriber
(`server/services/backlog_github_forward_sync.go:159-165`) can persist a
watermark that isn't subject to clock skew or read-after-write lag. The
doc comment is explicit: local wall-clock time is only a fallback for the
narrow case where the response body failed to decode.

**Linear**: `issueUpdate` is a GraphQL mutation — the caller controls the
response shape via the query's selection set, so requesting
`issue { updatedAt }` in the mutation response gives the same
write-then-read-back-in-one-round-trip guarantee GitHub's PATCH response
gives. This is safe to replicate as-is: [Getting started – Linear
Developers](https://linear.app/developers/graphql).

**JIRA is the harder case**: a successful `POST
/rest/api/3/issue/{id}/transitions` returns **204 No Content — no body at
all**. There is no `updated_at` to read back from the write response,
unlike GitHub's PATCH or Linear's mutation. Confirmed: "A successful
transition returns 204 No Content, an empty body... fetch the issue again
if you need its new state" ([Solved: Cannot transition an issue via Rest
API](https://community.atlassian.com/forums/Jira-questions/Cannot-transition-an-issue-via-Rest-API/qaq-p/1194157)).
**Design implication**: `JiraPlugin.TransitionIssue` needs a *third* API
call beyond the two already required for the transition itself (see §2) —
a follow-up `GET /issue/{id}?fields=updated` after the 204 — purely to
obtain the watermark. Skipping this and falling back to local wall-clock
time for every JIRA forward-sync (not just a decode-failure edge case, as
GitHub's fallback is) reopens the exact reimport race ADR-003 was written
to close, for every single call.

## 2. JIRA transition requires a two-call dance (confirmed, not just "well-known")

Confirmed against Atlassian's own docs and community threads: JIRA has no
"set status" endpoint. The workflow is: `GET
/rest/api/3/issue/{issueIdOrKey}/transitions` returns the set of
transition IDs valid **from the issue's current status**, then `POST
.../transitions` with body `{"transition":{"id":"<transition_id>"}}`
performs it ([Get Transitions | Jira - Reference | Postman API
Network](https://www.postman.com/api-evangelist/atlassian-jira/request/07ilh2i/get-transitions),
[Rest API says Transition id is not valid for this
issue](https://community.atlassian.com/forums/Jira-questions/Rest-API-says-Transition-id-is-not-valid-for-this-issue-but-I/qaq-p/2158194)).

Two design consequences beyond "remember to call GET first":
- **The valid-transition set is workflow-specific per project**, not a
  fixed enum — `JiraPlugin` cannot hardcode "done" → transition ID N the
  way `CloseIssue`'s `body := map[string]interface{}{"state": "closed"}`
  hardcodes GitHub's fixed two-state model
  (`session/backlog_plugin_github.go:314`). The forward-sync config needs
  a *target status name* (e.g. `ForwardSyncCloseLabel`'s JIRA analogue),
  resolved to a transition ID by name-matching against the GET response at
  call time — and that resolution can legitimately fail (no matching
  transition from the issue's current status, e.g. already closed via a
  different workflow path), which is a normal, expected error case, not a
  bug.
- **TOCTOU window**: the valid-transition list can change between the GET
  and the POST (someone else transitions the issue concurrently, or a
  workflow post-function fires). The POST can 400 with "transition id is
  not valid for this issue" even though the ID came from a GET made
  seconds earlier — this must be handled as a normal retryable failure
  path (log + `RecordSourceSyncFailure`), not treated as a programming
  bug, mirroring how GitHub's `CloseIssue` already treats a non-2xx
  response as an ordinary forward-sync failure
  (`session/backlog_plugin_github.go:341-344`).

## 3. GitHub-shaped auth-failure detection breaks for GraphQL (Linear)

The web UI's `isAuthFailure()` heuristic
(`web-app/src/components/settings/BacklogSourcesSettings.tsx:34-45`)
string-matches `errorMessage` for `"401"`, `"403"`, `"bad credentials"`,
etc., and explicitly excludes anything containing `"rate limit"` — because
GitHub's rate-limit response is *also* an HTTP 403, and the code comment
there notes this was a real false-positive bug already fixed once
(lines 27-33).

**This heuristic will not fire correctly for Linear at all** unless the
plugin code manually engineers it to. Linear's GraphQL API can return
**HTTP 200** with an `errors[]` array even for authentication failures —
confirmed: "GraphQL queries can partially succeed with a 200 HTTP status...
authentication errors may not always return a specific
authentication-related HTTP status code, but will instead appear in the
errors array of a 200 response," with `extensions.type` set to
`"authentication_error"`, `"forbidden"`, or `"ratelimited"` ([Errors –
Linear Developers](https://linear.app/developers/sdk-errors)). If
`LinearPlugin.Fetch` just wraps the raw GraphQL error message into a Go
`error`, `isAuthFailure()`'s `lower.includes("401")` / `("403")` checks
will silently never match a genuine expired-key case — the row-level
"credentials revoked" warning (Story 4.3.2, referenced in the same file's
comment) simply won't appear for Linear sources. **Design requirement**:
`LinearPlugin`'s error wrapping must translate
`extensions.type == "authentication_error"` (or `"forbidden"`) into an
error string containing a token `isAuthFailure()` actually matches (e.g.
literally include `"401"` or extend the frontend heuristic with a
Linear-specific phrase) — this can't be left to "the error message will
naturally contain the right words" the way GitHub's does.

JIRA is closer to GitHub's shape (real HTTP 401 for expired/invalid Cloud
API tokens or Server/DC PATs), so `isAuthFailure()` likely works
unmodified there — but confirm against a real 401 response body rather
than assuming from REST convention alone.

## 4. Rate limiting: neither tracker looks like GitHub's

`fetchIssuesPage` (`session/backlog_plugin_github.go:194-198`) treats
`429` or (`403` + `X-RateLimit-Remaining: 0`) as the rate-limit signal —
this is GitHub-specific and must **not** be reused verbatim for either new
plugin; both use fundamentally different limiting models:

- **Linear**: complexity-point leaky bucket, not a request count. API-key
  auth gets 250,000 points/hour/user (or up to 3,000,000 for some
  authenticated contexts), OAuth apps 200,000 points/hour/user+app,
  unauthenticated 10,000 points/hour/IP — "Linear uses the leaky bucket
  algorithm for its rate limiters... tokens are refilled with a constant
  rate" ([Rate limiting – Linear
  Developers](https://linear.app/developers/rate-limiting)). Rate-limit
  errors surface via GraphQL's `errors[].extensions.type ==
  "ratelimited"` per §3 — not an HTTP status — so `LinearPlugin` needs its
  own rate-limit detector reading the GraphQL error payload, structurally
  different from checking `resp.StatusCode`/`resp.Header`.
- **JIRA Cloud**: a newer three-tier model — points-based hourly quota
  (work-weighted, not per-request) plus a separate per-second burst cap,
  phased in starting **March 2, 2026** for REST and rolling out to
  GraphQL after — and apps can be pooled into shared per-tier quotas
  rather than strictly per-tenant ("In Tier 1, all installations of an app
  share one global hourly pool") ([Scaling responsibly: evolving our API
  rate limits](https://www.atlassian.com/blog/development/evolving-api-rate-limits),
  [Rate limiting - Jira Cloud
  platform](https://developer.atlassian.com/cloud/jira/platform/rate-limiting/)).
  Given the March 2026 phased rollout, this is an active migration at the
  time of writing — check current `Retry-After`/`X-RateLimit-*` header
  names against Atlassian's docs at implementation time rather than
  trusting this doc's snapshot.

Bottom line: each plugin needs its own rate-limit predicate function
(mirroring the isolated check at
`session/backlog_plugin_github.go:194-198`), not a shared one — and
Linear's in particular can't be detected from `resp.StatusCode` at all.

## 5. Auth header shape differs across all three trackers *and* within JIRA itself

- **Linear API key**: sent as the raw value in `Authorization`, **without**
  a `Bearer` prefix — `Authorization: lin_api_...`. Linear's **OAuth**
  tokens *do* need the prefix — `Authorization: Bearer <token>`. Mixing
  these up (e.g. reusing a shared "always prefix with Bearer" HTTP client
  helper written for GitHub/JIRA) silently 401s: "If you accidentally send
  `Bearer lin_api_...`, authentication will fail" ([Linear API Key: How to
  Generate and Use
  It](https://unified.to/blog/linear_api_key_how_to_generate_and_use_it_graphql_guide_for_developers)).
- **JIRA Cloud** uses HTTP Basic Auth with `base64(email:api_token)` as the
  credential — this needs both `JIRA_EMAIL` and `JIRA_API_TOKEN` (matching
  requirements.md's AC6 field list), not a bearer token alone.
- **JIRA Server/Data Center** uses a bearer Personal Access Token instead —
  a structurally different auth header for what looks like "the same
  product." If this plugin is meant to support both deployment types (the
  requirements doc's `JIRA_BASE_URL` field doesn't distinguish them), the
  config needs an explicit deployment-type field or must infer it from the
  base URL shape (`*.atlassian.net` vs self-hosted) — silently assuming
  Cloud-style Basic Auth against a Server/DC instance fails opaquely.

Compare to GitHub's own precedent here: `githubPluginConfig.Host` already
exists precisely to distinguish github.com from GitHub Enterprise Server,
each requiring a different base URL
(`session/backlog_plugin_github.go:37-43`, `github.RestBaseURLForHost`) —
JIRA needs the equivalent distinction for auth *header shape*, not just
base URL.

## 6. Secrets never enter logs or persisted error messages — verify the same discipline holds

Grepped `session/backlog_plugin_github.go` and
`server/services/backlog_github_forward_sync.go` for every `log.*` call
near token-handling code
(`session/backlog_plugin_github.go` has none — no `log.*` calls at all in
that file) and every place an error string is formatted
(`fmt.Errorf("github_issues: rate limited closing issue %s (retry-after=%s)", ...)`,
`session/backlog_plugin_github.go:339` — never interpolates `cfg.Token`).
This is a positive finding worth calling out explicitly as a pattern to
replicate, but it's fragile: `RecordSourceSyncFailure`
(`session/ent_repository_backlog.go:1742-1753`) and `CreateSourceSyncEvent`
(called from `session/backlog_sync.go:337` with `fetchErr.Error()`
directly as the persisted `errMsg`) **persist the raw `error.Error()`
string to the database**, and it's rendered verbatim in the Settings UI
(`BacklogSourcesSettings.tsx:305`, `:438`). Any error type from a
third-party Linear/JIRA client library that happens to embed request
details (e.g. some HTTP client wrapper libraries render the full request,
including headers, in transport-level errors) would leak a token straight
into the backlog_sources_settings UI and the sync-history table. **Design
requirement**: whichever HTTP/GraphQL client is chosen for Linear/JIRA,
audit its error `.Error()` output specifically for whether it ever embeds
request headers or the raw request URL (JIRA Basic Auth credentials are
never in the URL, but some legacy/community JIRA Go clients do put a
`?os_authType=basic`-style query param or similar in older API versions —
confirm the client used doesn't). Never hand-roll an error path that does
`fmt.Errorf("...: %s", req.Header)` or similar.

## 7. Data mapping: JIRA descriptions are ADF, not a string — GitHub's naive mapping doesn't port

`GitHubIssuesPlugin.MapToBacklogItem` does `desc := item.Description`
directly (`session/backlog_plugin_github.go:417`), because GitHub's
`body` field is a plain markdown string end to end.

**JIRA Cloud REST API v3 requires `description` (and other rich-text
fields) to be structured JSON — Atlassian Document Format — not a plain
string**: "Jira Cloud REST API v3 requires description and rich text
fields to be submitted as Atlassian Document Format (ADF) — a structured
JSON document type" ([Atlassian Document
Format](https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/)).
`issue.fields.description` in a v3 API response is a nested JSON object
(`{type: "doc", version: 1, content: [...]}`), not a string. A naive
`item.Description = issue.Fields.Description` (typed as `interface{}` or
`json.RawMessage`) would store a JSON blob as the backlog item's
description text — visibly broken in the UI, not silently wrong, but
still needs an explicit ADF→plaintext/markdown walker in `JiraPlugin`
before this ships (no existing ADF conversion utility exists in this repo
per a scan of `go.mod`/vendor — this is new code, not a wrapper around an
existing dependency). Two related consequences worth flagging up front:
- The **reverse** direction also applies: `PostIssueComment`'s JIRA
  equivalent (`POST /issue/{id}/comment`) needs its `body` field submitted
  as ADF too in v3 — the forward-sync close-comment text ("Closed
  automatically...", mirroring
  `forwardSyncCloseComment` in
  `server/services/backlog_github_forward_sync.go:32`) needs to be wrapped
  in a minimal ADF document (`{type:"doc", version:1, content:[{type:
  "paragraph", content:[{type:"text", text: "..."}]}]}`), not sent as a
  raw string.
- `description` is optional in JIRA's `createmeta` (`"required":false`)
  but *when present* must be ADF-shaped — so a partially-filled JIRA issue
  with no description is not an edge case to special-case away, it's the
  common case for some workflows.

Linear has no equivalent pitfall here — Linear issue `description` is
plain Markdown, matching GitHub's shape closely enough that
`MapToBacklogItem`-style direct assignment is fine.

## 8. Testing: `githubAPIBaseURL`-style override doesn't generalize identically

`withGitHubTestServer` (`session/backlog_plugin_github_test.go:26-33`)
works by swapping the **package-level `var githubAPIBaseURL`** to point at
an `httptest.Server`, restored via `t.Cleanup`, with an explicit warning
in the source comment that mutating tests must not run `t.Parallel()`
since it's unsynchronized package state (`session/backlog_plugin_github.go:27-32`).

- **JIRA** doesn't need a new package-level var at all — its base URL is
  already meant to be per-source config (`JIRA_BASE_URL`, matching how
  GitHub Enterprise's `Host` field already flows through
  `githubPluginConfig.Host` → `githubAPIURL`). Tests can just point
  `JIRA_BASE_URL` at the `httptest.Server`'s URL per-testcase — no shared
  mutable state, and parallel-safe for free. This is strictly better than
  the GitHub pattern; don't copy the package-var override for JIRA out of
  habit.
- **Linear** has exactly one real endpoint (`https://api.linear.app/graphql`)
  with no per-workspace URL — the workspace is determined by which API key
  is sent, not by the URL. So `LinearPlugin` *does* need a
  `githubAPIBaseURL`-style package-level override var (e.g.
  `linearAPIBaseURL`) to redirect tests to an `httptest.Server`, and
  inherits the same non-parallel caveat the GitHub tests already documented
  — worth carrying that exact comment forward rather than rediscovering the
  footgun.
- Regardless of transport, `LinearPlugin`'s tests need to construct
  **GraphQL response envelopes** (`{"data": {...}}` or `{"errors": [...]}`
  bodies) rather than the plain JSON arrays GitHub's REST fixtures use —
  the httptest handler shape is reusable, but the fixture *payload* shape
  is not; don't copy `githubIssue`-shaped JSON fixtures for Linear tests.

## Summary of design requirements this analysis surfaces (for plan.md)

1. `LinearPlugin`'s `issueUpdate` mutation must request `issue { updatedAt }`
   in its response selection to get a watermark; `JiraPlugin`'s transition
   call must issue a follow-up `GET` after the 204 to get one at all (§1).
2. `JiraPlugin` needs GET-transitions → name-match → POST-transition as one
   unit, with the name-match failure treated as a normal sync-failure
   outcome, not a bug (§2).
3. `LinearPlugin`'s error wrapping must surface auth failures in a form the
   existing `isAuthFailure()` frontend heuristic can actually match — this
   needs either backend-side error-string engineering or a frontend change,
   decided explicitly, not left implicit (§3).
4. Rate-limit detection is per-plugin, not shared: `LinearPlugin` reads
   GraphQL `errors[].extensions.type`, `JiraPlugin` reads Atlassian's
   points/burst headers (verify exact header names at implementation time —
   active migration as of March 2026) (§4).
5. Auth header construction must be config/deployment-aware: Linear key
   (no prefix) vs Linear OAuth (`Bearer` prefix) vs JIRA Cloud (Basic
   `email:token`) vs JIRA Server/DC (Bearer PAT) — four shapes, not two (§5).
6. Before choosing an HTTP/GraphQL client library for either plugin, check
   its error `.Error()` output never embeds credentials — persisted error
   strings are shown verbatim in the Settings UI (§6).
7. `JiraPlugin` needs an ADF→plaintext mapper for inbound descriptions and
   a plaintext→minimal-ADF wrapper for outbound comments — net-new code,
   no existing dependency covers this (§7).
8. Test-double strategy differs per plugin: JIRA reuses the
   config-carries-base-URL pattern (no shared package var, parallel-safe);
   Linear needs its own `githubAPIBaseURL`-style var with the same
   non-parallel caveat, plus GraphQL-envelope-shaped fixtures rather than
   GitHub's flat JSON-array fixtures (§8).
