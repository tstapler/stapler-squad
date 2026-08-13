# Research: Build vs. Buy — Linear and JIRA API Clients

Agent 6 (Build vs. Buy), SDD research phase for `linear-jira-integration`.
See `project_plans/linear-jira-integration/requirements.md` for full context.

## Question

For each of Linear and JIRA: hand-roll a minimal `net/http` + `encoding/json`
client (matching this repo's existing `session/backlog_plugin_github.go`
convention) vs. adopt an existing OSS library.

Required calls, per the requirements doc:
- Get single issue
- List/search issues (incremental, cursor-based)
- Get available transitions / workflow states
- Execute a transition / state update
- Add a comment

---

## 1. Existing OSS library / framework

### JIRA: `github.com/andygrunwald/go-jira`

VERIFIED via `gh api repos/andygrunwald/go-jira` (2026-08-06) and the repo's
`cloud/issue.go` source:

| Attribute | Value |
|---|---|
| Stars / forks | 1,615 / 500 |
| Open issues | 206 |
| License | MIT |
| Last push | 2026-08-01 |
| Last commit | 2026-06-14 |
| Archived | No |

API coverage (from `cloud/issue.go`'s `IssueService`, confirmed by reading
the source directly): `Get`, `Search` / `SearchV2JQL` (JQL search — `Search`
is deprecated on Jira Cloud per the library's own docs, `SearchV2JQL` is the
current endpoint), `GetTransitions`, `DoTransition` /
`DoTransitionWithPayload`, `AddComment`. All five calls the requirements doc
needs exist as first-class methods — no gaps.

**Important structural note:** the library shipped a `v2.0.0` major rewrite
in January 2026 (`MIGRATE.md`, "Guide version: written on 2026-01-12"),
splitting the single `jira` package into `github.com/andygrunwald/go-jira/v2/cloud`
and `.../v2/onpremise` — Cloud and Server/Data Center APIs have diverged
enough that the maintainer no longer models them as one client. v2 also makes
`context.Context` mandatory on every call (previously `*WithContext` variants
existed alongside context-free ones) and renames Cloud auth's `Password`
field to `APIToken`, matching Atlassian's Basic-auth-with-API-token scheme
for Cloud. Since this repo's own auth requirement is
`JIRA_EMAIL`/`JIRA_API_TOKEN` (Cloud-style), `.../v2/cloud` is the correct
subpackage — this is not a generic multi-deployment client, so importing
`onpremise` is unnecessary dead weight.

v2.0.0 being ~7 months old at the time of this research is a mild adoption
risk (less field-tested than a library with years of stable major-version
history), but the commit/release cadence (last commit 3 weeks before this
writing, active issue triage) shows the project didn't rewrite and abandon
it.

**Pros:**
- Full coverage of all five required calls, confirmed by reading the source, not just docs
- Actively maintained, MIT license, no red flags
- Handles pagination, rate-limit-adjacent response types, and JQL query building idioms that are easy to get subtly wrong by hand
- `context.Context`-first API (v2) fits this repo's own context-threading conventions

**Cons:**
- Pulls in a real dependency (plus its own transitive deps for auth transports) for what's ultimately ~5 HTTP calls
- v2's Cloud/onpremise split means committing to `.../v2/cloud` specifically — any future on-prem JIRA requirement would need a second import, not just a config flag
- Diverges from this codebase's established "hand-roll `net/http`" convention (see §3)
- v2.0.0 is young; less multi-year field exposure than the library's own v1 had

**Verdict: Viable**, not a slam-dunk default — see final recommendation in §3/§5, which weighs this against convention-consistency.

### Linear: no well-maintained Go GraphQL SDK exists

Linear's own SDKs are TypeScript and Python only (confirmed via
`linear.app/developers` — "The Linear SDK... is recommended... written in
TypeScript"). There is no first-party Go client.

Searched for community Go clients generated against Linear's public GraphQL
schema:

| Candidate | Stars | Last push | Verdict |
|---|---|---|---|
| `github.com/guillermo/linear` (`linear-api` subpackage) | 3 | 2024-03-17 | VERIFIED via `gh api repos/guillermo/linear` — essentially unmaintained, no meaningful adoption signal, generated types only (no request/mutation helpers) |

No other Go-specific Linear client surfaced in search. This confirms the
requirements doc's own assumption: **there is no viable "adopt a Linear
client" option** — the real choice is generic Go GraphQL client library vs.
hand-rolled GraphQL-over-HTTP.

### Generic Go GraphQL client libraries (for use against Linear's public schema)

| Library | Stars | Last push | License | Approach |
|---|---|---|---|---|
| `github.com/hasura/go-graphql-client` | 474 | 2026-04-11 | MIT | Runtime reflection-based query builder (fork of `shurcooL/graphql`) |
| `github.com/machinebox/graphql` | 960 | 2024-08-02 | Apache-2.0 | Thin runtime client — build the query string yourself, it just handles the HTTP/JSON envelope |
| `github.com/Khan/genqlient` | 1,319 | 2026-05-27 | MIT | Compile-time codegen from `.graphql` query files + the target schema → generates typed Go structs and functions |

All VERIFIED via `gh api repos/<owner>/<repo>` (2026-08-06).

**Pros (any of the three over hand-rolled):**
- Response JSON unmarshaling into typed structs is handled generically
- `genqlient` specifically gives compile-time type safety against Linear's actual schema (would need to vendor/fetch Linear's public schema SDL once)

**Cons:**
- `machinebox/graphql` is stale (2 years no push) — same "abandoned library" risk profile as hand-rolling, minus the benefit of a maintained upstream
- `hasura/go-graphql-client`'s reflection-based query construction is a debugging-unfriendly abstraction layer for what is, per the requirements doc, only 4-5 named GraphQL operations (get issue, list issues since cursor, list workflow states, update issue state, create comment)
- `genqlient` requires a codegen step wired into the build (`go generate` + `.graphql` query files + a schema snapshot to keep in sync with Linear's evolving API) — meaningfully more machinery than this repo's existing plugins use for GitHub
- None of the three save meaningful code over hand-rolling: a GraphQL request is just `POST /graphql` with `{"query": "...", "variables": {...}}` and a Bearer/API-key header — there's no REST-style pagination-header parsing, retry-after handling, or multi-endpoint routing that a library would meaningfully abstract, unlike JIRA's REST API surface

**Verdict: Not recommended.** The cost (new dependency, plus for genqlient a codegen pipeline) isn't justified by what's actually saved — GraphQL-over-HTTP for a handful of fixed queries is not where these libraries earn their keep. This point is more thoroughly justified in §3 below.

---

## 2. SaaS / managed API angle

Does not apply. Linear and JIRA *are* the SaaS APIs being integrated with —
there is no "hosted alternative to hand-rolling the integration" the way
there might be for, say, adopting a managed email-sending API instead of
hand-rolling SMTP. Noted per the research brief and set aside.

---

## 3. LLM-generated implementation vs. battle-tested library

### Established convention in this codebase

`session/backlog_plugin_github.go` hand-rolls `net/http` + `encoding/json`
against the GitHub REST API rather than using `google/go-github` (the
de facto standard, actively maintained Go GitHub client). This is a
deliberate existing precedent, not an oversight — the plugin is ~430 lines
covering fetch/paginate, close-issue, post-comment, and label-merge logic,
all readable in one file with no external SDK indirection. `go.mod` has no
`google/go-github` dependency today (VERIFIED — `grep -riE "linear|jira"
go.mod go.sum` and a scan of the module's requires found no issue-tracker
SDK of any kind currently vendored).

This matters for both trackers, but the correctness risk differs sharply:

### JIRA: the two-step transition flow is the risk case

JIRA's status-change API is not "set status to X" — it's:
1. `GET /issue/{id}/transitions` → returns the *currently available*
   transition IDs for that issue's current status (JIRA workflows are
   directed graphs; not all transitions are legal from all states, and the
   ID-to-name mapping is workflow-specific, not a fixed enum)
2. Match the desired target status by name (case/whitespace handling,
   possible duplicate names across different transition IDs in complex
   workflows)
3. `POST /issue/{id}/transitions` with the resolved transition ID

Hand-rolling this correctly requires reproducing: request/response shape for
both calls, JIRA's error-response envelope (used to distinguish "transition
not available from current state" from auth/network failures), and the
name-matching edge cases. `go-jira`'s `GetTransitions`/`DoTransition` pair
already encodes this shape and has presumably absorbed years of edge-case bug
reports (206 open issues is evidence of a large, exercised user base filing
edge cases, not a red flag by itself for a project this size and age).
Additionally, JIRA Cloud's `SearchV2JQL` migration (the JQL search endpoint
was deprecated and replaced) is exactly the kind of upstream API-surface
churn that's easy to miss by hand and that a maintained library tracks for
you — `go-jira` v2 already made this switch.

**Correctness-risk verdict: hand-rolling JIRA's transition flow is
meaningfully riskier than hand-rolling GitHub's close-issue flow.** GitHub's
`PATCH /issues/{n} {"state":"closed"}` is a single idempotent call with no
discovery step; JIRA's is a stateful two-call protocol with a workflow-graph
constraint in the middle. This is a real asymmetry between the two trackers,
not just "JIRA is more code."

### Linear: hand-rolled GraphQL query construction is comparatively low-risk

Linear's GraphQL API is a single endpoint (`https://api.linear.app/graphql`)
accepting `{"query": "...", "variables": {...}}` over POST with an API-key
header (no OAuth dance needed for a personal/workspace API key, matching the
`LINEAR_API_KEY` credential the requirements doc specifies). The operations
needed are a fixed, small set:
- `issue(id: ...)` query — get one issue
- `issues(filter: {updatedAt: {gt: $cursor}})` query — incremental list
- `workflowStates` query — get available states for the team (Linear's
  equivalent of JIRA transitions, but simpler: states are a flat list per
  team, not a per-issue directed-graph lookup)
- `issueUpdate(id: ..., input: {stateId: ...})` mutation — set state
- `commentCreate(input: {issueId: ..., body: ...})` mutation — add comment

Unlike JIRA, there's no discovery-then-act two-step — Linear's states are
fetched once (or cached) per team, not re-queried per transition, and
`issueUpdate` accepts a `stateId` directly. Query strings are static
(`variables` carry the dynamic parts), so there's no dynamic query-string
assembly to get wrong — each operation is one hardcoded query template plus
a JSON-decoded response struct, structurally identical in shape to what
`convertGithubIssues` already does for GitHub's REST responses in this repo.

**Correctness-risk verdict: hand-rolling Linear's GraphQL calls carries
about the same risk profile as the existing GitHub REST plugin** — fixed
request shapes, typed response decoding, no stateful multi-call protocol.

### Consistency argument

Following the same hand-rolled convention for both new plugins keeps all
three tracker integrations (`GitHubIssuesPlugin`, a future `LinearPlugin`,
a future `JiraPlugin`) at the same abstraction level: one file per tracker,
`net/http` + `encoding/json`, no SDK-specific error types or client-object
lifecycle leaking into `ItemSourcePlugin` implementations. A future
maintainer reading `session/backlog_plugin_*.go` sees one pattern
repeated three times, not "two hand-rolled plus one wrapping a third-party
client with a different error-handling shape." This directly serves
`.claude/rules/interface-pollution-checklist.md`'s spirit even though that
rule is about interfaces specifically — it's the same "don't introduce a
structurally different abstraction than the rest of the codebase uses for
the same job" principle.

---

## 4. Fork or adapt

Searched this repo and its `github` package (`github.GetKeychainTokenForHost`,
referenced by the existing plugin) for any existing Linear/JIRA API code:
none found (`grep -riE "linear|jira" -r --include="*.go" .` returned zero
matches outside this research doc and the requirements doc itself). There is
no sibling stapler-squad-adjacent project or internal package to fork from —
this is greenfield for both trackers in this codebase.

---

## Final Recommendation

**JIRA: adopt `github.com/andygrunwald/go-jira/v2/cloud`.**

This is the one place where the "hand-roll for consistency" argument loses
to correctness risk. The transition-lookup-then-transition flow, JIRA's
evolving JQL search endpoint (the very deprecation `go-jira` v2 already
tracked), and its error-response conventions are exactly the kind of
"looks simple, has sharp edges" surface where a library with hundreds of
resolved issues has already absorbed bugs a first implementation would
reintroduce. The dependency cost is small (MIT, actively maintained, no
transitive bloat beyond standard `net/http` auth transports), and the
`JiraPlugin` implementation can still be a thin ~150-line file that
constructs a `cloud.Client` with `APIToken` auth from the keychain and maps
its typed responses to `ExternalItem`/`BacklogItemData` — the
`ItemSourcePlugin` interface boundary stays exactly as small as it is
today; only the internals of `Fetch`/`MapToBacklogItem`/the forward-sync
methods call into `go-jira` instead of raw `net/http`. Pin to v2.0.0+ and
budget a MIGRATE.md read if a later v3 ever ships.

**Linear: hand-roll `net/http` + `encoding/json` GraphQL calls, matching
`backlog_plugin_github.go`'s existing convention.**

No maintained Go SDK exists for Linear (the one candidate found is a
3-star, 2-year-stale repo), so "adopt a library" isn't actually on the table
in the way it is for JIRA — the real choice is generic-GraphQL-client vs.
hand-rolled, and none of the three generic clients evaluated
(`hasura/go-graphql-client`, `machinebox/graphql`, `Khan/genqlient`) pay for
their weight against ~5 fixed, non-stateful GraphQL operations. Hand-rolling
keeps `LinearPlugin` at the same file-per-tracker,
`net/http`-plus-`encoding/json` shape as `GitHubIssuesPlugin`, and Linear's
actual protocol (single endpoint, static query templates, no
discovery-then-act step) doesn't carry the correctness risk that justified
reaching for a library on the JIRA side.

**Net effect:** the two new plugins will *not* use the same underlying HTTP
strategy as each other — `JiraPlugin` wraps `go-jira/v2/cloud`,
`LinearPlugin` hand-rolls GraphQL — but both still implement the same
`session.ItemSourcePlugin` interface with the same file-per-tracker
placement (`session/backlog_plugin_linear.go`,
`session/backlog_plugin_jira.go`) as `GitHubIssuesPlugin`. The asymmetry is
justified per-tracker by actual API shape and available-library maturity,
not by inconsistent taste — and it doesn't leak into the plugin interface
consumers see.
