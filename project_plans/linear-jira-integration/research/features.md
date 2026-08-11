# Research: Existing Patterns & Edge Cases — Linear/JIRA Integration

Agent 2 (Features) research pass. Covers what already exists to mirror, and
edge cases/failure modes/unstated needs the plan phase should account for.

## 1. What already exists to mirror

### `ItemSourcePlugin` interface (`session/backlog_plugin.go`)

```go
type ItemSourcePlugin interface {
    PluginID() string
    Fetch(ctx context.Context, config PluginConfig, cursor string) ([]ExternalItem, string, error)
    MapToBacklogItem(item ExternalItem, sourceID string) BacklogItemData
}
```

Plus the optional `PaginatedFetcher` capability interface (`FetchAll`, used
only by `PreviewBackwardSyncImpact` for the Settings "what will backward sync
affect" preview). `ExternalItem` is the platform-agnostic shape both new
plugins must populate: `ExternalID`, `Title`, `Description`, `Labels`,
`Priority` (1-5), `URL`, `State` (raw string, only meaningful for two-way-sync
plugins), `IssueUpdatedAt` (the loop-prevention watermark comparison value).
`NewDefaultRegistry()` (`session/backlog_plugin.go:79`) is a 3-line function
that just calls `r.Register(...)` for each plugin — trivial to extend with
`NewLinearPlugin()` / `NewJiraPlugin()`.

**Confirms requirements.md's non-goal is correct**: the interface is already
tracker-agnostic. This is "write two more implementations," not "design a new
abstraction."

### `GitHubIssuesPlugin` (`session/backlog_plugin_github.go`) — the pattern to mirror

- `Fetch` is single-page, cursor = ISO8601 `since` timestamp passed to the API
  and advanced to `max(cursor, latest updated_at seen)` (`convertGithubIssues`,
  line 214).
- `FetchAll` — bounded-page aggregation (`maxPreviewFetchPages = 20`, a `var`
  not `const` so tests can shrink it) for the one non-incremental consumer
  (`PreviewBackwardSyncImpact`).
- Config decode has two flavors with different semantics for a missing token:
  `decodeGithubIssuesFetchConfig` (Fetch path) treats missing token as
  `disabled=true` → empty result, not an error, so a source with no
  credentials configured yet doesn't spam sync-failure logs. `decodeGithubIssuesConfig`
  (forward-sync path, `CloseIssue`/`PostIssueComment`) treats missing token as
  a hard error — those are only ever called once a source is already known
  to have `ForwardSyncEnabled` + credentials.
- `CloseIssue` returns the **API response's own `updated_at`**, not local wall
  time — this return value is what the forward-sync subscriber uses for the
  ADR-003 watermark (see §3 below). Falls back to a zero time only when the
  close succeeded (status < 300) but the body failed to decode; callers treat
  zero specially, not as a real timestamp.
- `PostIssueComment` failures are logged but non-fatal — the close already
  succeeded by the time comment-posting runs.
- Rate limiting: checks `429` or (`403` + `X-RateLimit-Remaining: 0`) and
  surfaces a distinct error string (`"github_issues: rate limited..."`) —
  `.claude/rules/fix-flaky-tests-dont-defer.md`-style discipline: this exact
  string is asserted on in tests, so the two new plugins need their own
  rate-limit detection wired the same way (Linear returns HTTP 430 for
  GraphQL complexity limits in some cases + standard 429; JIRA returns 429
  with `Retry-After`).
- `MapToBacklogItem` truncates title to 200 chars, description to 2000 chars
  — a hard contract both new plugins must replicate (`BacklogItemData` column
  limits presumably match; not independently verified here, but the constants
  live at `backlog_plugin_github.go:413/418` and should be shared/extracted
  rather than re-literaled if a third plugin follow-up ever lands).

### Forward-sync-on-completion (`server/services/backlog_github_forward_sync.go`)

- EventBus subscriber (`StartBacklogGitHubForwardSyncSubscriber`) listens for
  `EventBacklogItemChanged` with `Kind == BacklogChangeStatusTransition` and
  `NewStatus == done`.
- Gates on `source.ForwardSyncEnabled` (per-`ItemSource` bool from ent schema)
  **and** a type assertion: `plugin.(externalIssueCloser)` — a narrow
  interface (`CloseIssue` + `PostIssueComment`) defined in the *consumer*
  package (`server/services`), not next to `GitHubIssuesPlugin` — this is
  the `.claude/rules/interface-pollution-checklist.md` pattern already
  correctly applied here (smell #2 avoided). A `LinearPlugin`/`JiraPlugin`
  implementing the same two-method shape will satisfy this interface for
  free with zero changes to the subscriber itself, **provided the method
  signatures match exactly** — `externalIssueCloser` is structural, so a
  Linear "close" with a different signature (e.g. taking a target state
  string instead of an implicit "closed") either needs to conform to this
  exact shape or the subscriber needs a second type-assertion branch.
- Re-fetches the item by ID rather than trusting the event payload — payload
  snapshots can have a stale/empty `SourceID` (documented gotcha in the code
  comment, worth preserving in any Linear/JIRA equivalent subscriber).
- Skips silently (no forward-sync) when `current.SourceID == "" || current.ExternalID == ""`
  — i.e. locally-created items are never touched.
- Watermark write at the end always prefers `issueUpdatedAt` from the API
  response; falls back to `time.Now().UTC()` only if that's zero.
- **Known deferred gap** (documented in the file): there's no "is this issue
  already closed" pre-check before calling `CloseIssue` — an extra live API
  call was judged not worth it for Phase 1. This will matter more for JIRA,
  where "transition to closed" against an issue already in a terminal state
  can be a no-op-vs-error depending on the workflow (see §2).

### `SyncLoop` / cursor mechanics (`session/backlog_sync.go`)

- Per-source mutex (`syncSourceLocks`, a package-level `sync.Map`) serializes
  concurrent `SyncOne` calls for the same source — covers the periodic tick
  racing a manual `TriggerSync` RPC. Applies automatically to new plugins;
  no plugin-side work needed.
- `SyncOne` does the create-or-update-with-local-wins dance: `UserModifiedFields`
  gates which columns backward sync is allowed to overwrite (title/description/
  priority/labels — labels additionally gated on `source.BackwardSyncEnabled`).
  `ExternalURL` is the one field that's *never* gated — always backfilled once
  known (ADR-001 decision, `backlog_sync.go:412`).
- Backward-sync status transitions (`determineBackwardSyncTarget`,
  `backlog_sync.go:551`) only fire for GitHub currently: closed issue on a
  pre-work item → `archived`; there is no `done` target (would require
  cross-package `HasUnshippedCode` logic, explicitly out of scope). **This
  logic is GitHub-issue-state-shaped** (`extItem.State == "closed"`/`"open"`)
  — Linear/JIRA's richer state models (see §2) don't map onto a binary
  open/closed cleanly, so a generalized `determineBackwardSyncTarget` (or a
  per-plugin variant) is a real design question for plan.md, not just a
  wiring exercise.
- `TriggeredByGitHubSync` (`session/backlog.go:96`) is a GitHub-specific
  constant threaded into `TransitionBacklogItemStatus`'s audit trail
  (`BacklogStatusEvent.TriggeredBy`). New plugins need their own
  (`TriggeredByLinearSync`, `TriggeredByJiraSync`) for the same audit
  provenance, unless a single generic `TriggeredBySync` is judged sufficient
  (loses per-tracker attribution in the history view — `backlog_review.go:192`
  renders this string directly).
- `GitHubSyncedIssueUpdatedAt` — **the loop-prevention watermark field is
  named after GitHub specifically**, both in Go (`BacklogItemData.GitHubSyncedIssueUpdatedAt`)
  and in the ent schema/DB column (`github_synced_issue_updated_at`,
  `SetNillableGithubSyncedIssueUpdatedAt`). This is a real schema decision
  for plan.md: either (a) rename to a generic `SourceSyncedItemUpdatedAt` via
  an ent migration (touches every read/write site: `ent_repository_backlog.go`,
  `backlog_sync.go`, `backlog_github_forward_sync.go`, tests), or (b) add
  parallel `LinearSyncedIssueUpdatedAt`/`JiraSyncedIssueUpdatedAt` columns
  (avoids a migration but triples the watermark-write code with three
  near-identical field names, and every future new plugin repeats this). ADR-003
  (`project_plans/backlog-github-two-way-sync/decisions/ADR-003-loop-prevention-watermark-design.md`)
  is the design doc for the existing field; it does not anticipate a
  multi-tracker future.

### One-off MCP import (`server/mcp/tools_backlog.go`, `importGitHubIssue` ~L997-1058)

- Parses the URL via `githubpkg.ParseGitHubRefWithHosts`, fetches via
  `githubpkg.GetIssue`, then calls the *exact same* `storage.CreateBacklogItem`
  the plugin sync path uses — but **does not set `SourceID`/`ExternalID`** on
  the created item. It stores provenance only as free text: `Notes: "Imported
  from <url>"`. This means one-off-imported items are **not** deduplicated
  against a later poll-based `ItemSource` sync for the same repo (no
  `ExternalID` set → `GetBacklogItemByExternalID` lookup in `SyncOne` can
  never match it) — a pre-existing gap in the GitHub path that Linear/JIRA
  should either inherit knowingly or fix in the same PR (requirements.md
  doesn't call this out; worth flagging to the planner).
- Explicitly documented as **not supporting GitHub Enterprise** — "that
  config lives on `BacklogService`, which this package-level handler has no
  access to." The Linear/JIRA equivalents need the analogous host/instance
  config (JIRA especially — see §2) threaded through `backlogHandlers`, or
  they inherit this same limitation for self-hosted/data-center JIRA.
- Auto-triage: `h.backlogSvc.MaybeTriggerTriage(...)` is called after create,
  gated on `repo_path` being set and `skip_triage` not being set — same
  contract, trivially reusable.

### Test pattern (`session/backlog_plugin_github_test.go`)

Covers: `PluginID()` value, empty-token → empty result (no-op, not error),
missing-required-config → error, priority-from-label-map + default-priority
fallback, cursor advancement to latest `updated_at`, `IssueUpdatedAt` parses
the same value used for the cursor, closed-issue state decoding. Uses
`httptest.NewServer` + a package-level base-URL override var
(`githubAPIBaseURL`) that tests swap and restore via `t.Cleanup` — **not
parallel-safe** (package-level mutable state), documented inline. Also
depends on `TestMain`'s `keyring.MockInit()` (in `integration_test.go`) to
keep `GetKeychainTokenForHost` from reading a real OS keychain during tests —
any Linear/JIRA keychain equivalent needs the same mock-init coverage or
tests will flake/leak real credentials depending on the dev machine.

### Credentials pattern (`github/keychain.go`)

`GetKeychainTokenForHost(host)` is **GitHub-package-specific** — it reads from
a `github.com`-namespaced keychain service (`"stapler-squad"` service,
`"github-token"`/`"github-token:<host>:<username>"` keys) and iterates
`ListKeychainAccounts()` (a GitHub-specific `[]AccountRef{Username, Host}`
list). There is **no generic keychain package** for a Linear API key (which
has no "host" or "username" — a single per-workspace secret) or a JIRA
triple (`base_url` + `email` + `api_token`, where multiple JIRA instances are
plausible and `base_url` is the natural key, not `host` alone since Atlassian
Cloud sites are subdomains under `atlassian.net`). Requirements.md's AC6 says
"read from the existing keychain/secrets mechanism" — this is **not a direct
reuse**, it's "build an analogous per-tracker keychain helper following the
same `go-keyring` + `stapler-squad` service-name convention," likely a new
small package (`linear/keychain.go`, `jira/keychain.go`) or a shared generic
one keyed by `(plugin_id, instance_id)` rather than `(host, username)`.

### Frontend: schema-driven plugin config UI already anticipates this

`web-app/src/components/settings/backlogSourceSchemas.ts` — `PLUGIN_SCHEMAS`
is explicitly commented: *"Adding a source type for a new plugin (e.g. Jira,
Linear) means adding one entry here."* `BacklogSourcesSettings.tsx` already
renders fields generically from this schema (no owner/repo hardcoding) and
supports a `credentialsManagedExternally` flag that swaps a token input for a
link to a dedicated credentials page — this is the extension point for
Linear/JIRA's keychain-backed auth (mirroring how `github_issues`/`github_prs`
already set `credentialsManagedExternally: true` and link to Settings > GitHub
Accounts).

### Frontend: no existing per-item source badge or source filter

Contrary to what requirements.md's Goal 5 might imply already existing for
GitHub, there is **no per-backlog-item source badge or source filter** in the
current UI. `web-app/src/components/sessions/SourceBadge.tsx` is an unrelated
component (session-defaults provenance: "from global/directory/profile"
config, not backlog item tracker source). `BacklogItemBadge.tsx`
(`web-app/src/components/backlog/`) renders status chip + AC count + title
only — no source indicator, and its own doc comment notes it's already at
its width/complexity budget (explicitly deferred a 4th inline element,
`BlockerChip`, for the same reason). Goal 5's badge is genuinely new frontend
work, not a copy of an existing GitHub-item badge, and needs its own UX
decision on where it fits (the badge is full, per that comment) — possibly
the detail panel rather than the list badge, or an icon-only compact variant.
No source-based filter control exists anywhere in the backlog list either
(`ExternalURL` is stored but nothing today reads `SourceID`/`plugin_id` to
build a filter dropdown).

## 2. Linear/JIRA-specific edge cases the design must handle

### JIRA

- **No fixed status names.** JIRA statuses are per-project, per-workflow —
  "In Review"/"Done" are conventions, not guarantees. A forward-sync "mark
  done" needs either (a) a configured target status **string** per source
  (validated against that project's actual workflow via the JIRA API at
  source-setup time, since a typo'd status name fails silently/late
  otherwise), or (b) a transition-ID-based approach (JIRA's `POST
  /issue/{id}/transitions` requires a transition ID, not a status name
  directly — the caller must first `GET /issue/{id}/transitions` to resolve
  which transition ID reaches the desired status from the issue's *current*
  status, since not every status is reachable from every other status in a
  workflow). This is meaningfully more complex than GitHub's binary
  open/closed `PATCH {state: closed}`.
- **Issue keys vs URLs.** JIRA issues are addressed by `PROJECT-123` (key) in
  most API calls, but the browser URL is `https://x.atlassian.net/browse/PROJECT-123`.
  `ExternalID` should likely store the key (stable, used in API calls)
  while `URL`/`ExternalURL` stores the browse link — mirrors GitHub's
  `Number` (used in API paths) vs `HTMLURL` split, so this is a straight
  copy of an existing pattern, not a new problem.
- **Pagination**: `startAt`/`maxResults` (JQL search), not cursor-based —
  functionally equivalent to GitHub's `page`/`per_page`, but the "cursor"
  concept for incremental fetch has to be JQL's own filter, e.g. `updated >=
  "<cursor>"` in the JQL query string, analogous to GitHub's `since=`.
- **Multi-project instances**: a single JIRA Cloud site can host many
  projects; source config needs a `project_key` (or JQL filter) the same way
  GitHub's config needs `owner`+`repo`. Self-hosted/Data Center JIRA also
  means `base_url` is mandatory config (unlike GitHub where `host` defaults
  to github.com) — every request must be built against a configurable base
  URL, and Basic Auth (email + API token) rather than a GitHub-style bearer
  token is JIRA Cloud's actual auth mechanism.
- **Rate limiting**: JIRA Cloud returns `429` with `Retry-After`; less
  aggressive than GitHub's secondary limits but still needs the same
  detection branch as `fetchIssuesPage`'s `429`/`403` check.
- **"Already closed" transition attempts**: unlike GitHub's idempotent
  `PATCH {state: closed}` (closing an already-closed issue is a no-op, still
  200), JIRA's transition-ID API can **error** if the requested transition
  isn't valid from the issue's current status (e.g. already in "Done," no
  "Done → Done" transition exists) — the forward-sync subscriber's "no
  is-it-already-closed pre-check" deferral (documented gap in
  `backlog_github_forward_sync.go`) is riskier to inherit as-is for JIRA;
  it may surface as a routine, expected error rather than a true failure and
  needs distinct handling (log-and-skip, not `RecordSourceSyncFailure`).

### Linear

- **Workflow states are per-team, not fixed.** Like JIRA, "Done"/"In Review"
  are team-configurable `WorkflowState` objects with a `type` enum
  (`backlog`/`unstarted`/`started`/`completed`/`canceled`) — the `type` field
  is the one semi-stable thing to key off for backward-sync mapping (GitHub's
  binary open/closed ≈ Linear's `completed`/`canceled` vs everything else),
  but forward-sync "mark this issue done" still needs a specific
  `WorkflowState` ID to set, resolved from the team's actual states (fetched
  via GraphQL) rather than a hardcoded name.
- **GraphQL API, not REST.** All access is a single `POST /graphql` endpoint
  with query/mutation bodies — structurally different HTTP shape from
  GitHub's REST plugin; `fetchIssuesPage`'s pattern (build URL, GET, decode
  JSON array) doesn't transplant directly. Cursor-based pagination is native
  (`after: <cursor>`, `pageInfo.hasNextPage`/`endCursor`) — actually a closer
  match to `FetchAll`'s existing page-loop shape than JIRA's `startAt`.
- **Priority scale mismatch.** Linear's native priority is `0-4` (`0` = No
  priority, `1` = Urgent, `2` = High, `3` = Medium, `4` = Low) — inverted
  scale and different cardinality from GitHub's label-derived 1-5 used here
  (`ExternalItem.Priority`, backlog's own 1-5 P-scale). Needs an explicit
  mapping table (not a 1:1 pass-through), and unlike GitHub's
  `label_priority_map` (opt-in, defaults to priority 3), Linear issues
  *always* have a native priority value — so the mapping is mandatory, not
  best-effort.
- **Identifiers**: human-facing `ENG-123` (`identifier` field) vs internal
  GraphQL node ID (UUID, used for mutations). Same "human key for
  display/API-readable-path vs opaque ID for mutations" split as JIRA;
  `ExternalID` likely wants the `identifier` (stable, human-meaningful,
  matches the URL) while mutations resolve/require the node ID — worth
  checking whether Linear's GraphQL mutations accept the identifier directly
  or require the UUID (if the latter, `ExternalID` storage needs to be the
  UUID instead, breaking the "matches the URL" convenience GitHub enjoys).
- **Cycles**: out of scope per requirements.md (no mention), but Linear issues
  belonging to a cycle/sprint is a Linear-native concept with no GitHub
  Issues equivalent — worth an explicit non-goal note in plan.md so it isn't
  silently assumed needed.
- **Multi-team workspaces**: like JIRA's multi-project instances, a source
  config needs a `team_id` (or `project_id`, Linear also has "Projects"
  nested under teams) — not just an API key.
- **Rate limiting**: Linear enforces a complexity-budget GraphQL rate limit
  (not just request-count) — a single query can be rejected for being "too
  expensive" independent of request frequency; harder to detect/back off from
  than a simple `429`, and the plugin's Fetch query shape (how many nested
  fields/how large a page) directly affects whether this triggers.

## 3. Unstated needs surfaced by this comparison

1. **Per-tracker keychain package** — AC6 implies reuse but the actual
   GitHub keychain helper (`github.GetKeychainTokenForHost`) is
   package-specific and keyed on `(host, username)`, which doesn't fit
   Linear (single workspace secret, no username concept) or JIRA (keyed
   more naturally on `base_url` + `email`). This needs its own small design
   decision, not literal reuse — flag for plan.md.
2. **Watermark field naming** — `GitHubSyncedIssueUpdatedAt` is hardcoded to
   GitHub in both Go and the ent schema. Plan.md needs to explicitly choose
   generalize-via-migration vs. parallel-fields-per-tracker, since ADR-003
   (the existing watermark design doc) didn't anticipate multiple trackers.
3. **`determineBackwardSyncTarget`'s binary open/closed assumption** doesn't
   generalize to JIRA/Linear's richer state models without either (a) a
   per-plugin status-classification function each plugin supplies (e.g. an
   optional `ClassifyState(rawState string) (closed bool)` capability
   interface, following the `PaginatedFetcher` optional-interface precedent)
   or (b) requiring Linear/JIRA plugins to normalize into GitHub's binary
   `State` field ("open"/"closed") at `ExternalItem` construction time,
   losing fidelity but requiring zero `backlog_sync.go` changes. The second
   is much lower-risk for a first cut and matches the existing
   `ExternalItem.State` doc comment's framing ("open"/"closed"-shaped), but
   plan.md should decide explicitly rather than let it default silently.
4. **Deduplication for one-off MCP imports vs. later plugin-poll sync** — the
   existing `importGitHubIssue` MCP handler doesn't set `SourceID`/`ExternalID`
   on the created item, so it's invisible to `GetBacklogItemByExternalID`'s
   dedup lookup. If a Linear/JIRA `ItemSource` is later configured for the
   same tracker after a one-off import already happened, the poller will
   create a **duplicate** backlog item. Worth deciding whether the new
   one-off import tools fix this (set `SourceID` by looking up/creating a
   matching `ItemSource` row, or at minimum set `ExternalID` even without a
   `SourceID`) rather than perpetuating the gap.
5. **Credential validity/expiry handling** — none of the code read here
   (GitHub included) has an explicit "credentials are invalid/expired"
   path beyond generic HTTP error propagation into `RecordSourceSyncFailure`
   (visible in sync history) — no distinct UI treatment (e.g. "reconnect"
   prompt) for auth failures vs. transient network errors. JIRA Basic Auth
   (email+token) and Linear API keys both expire/get-revoked in ways GitHub
   PATs do too, so this isn't strictly new, but if requirements.md's "no
   silent automated action" convention extends to auth failures, a
   distinguishable error class (401/403 vs. 5xx/timeout) may be worth adding
   for both new plugins and retrofitting to GitHub's.
6. **Per-source enable/disable already exists** (`ItemSource.Enabled`,
   gates `runAllSources`) — no new mechanism needed, confirmed reusable
   as-is.
7. **Label/priority mapping config per tracker** — GitHub's
   `label_priority_map` is bespoke to label-based priority. Linear needs the
   inverse (native `0-4` priority → backlog's 1-5 scale) and JIRA needs
   either its own `priority` field mapping (JIRA has a native `Highest`
   `High`/`Medium`/`Low`/`Lowest` priority scheme, separate from labels) or a
   label-based fallback like GitHub's — these are three different priority
   *sources* (labels vs. native enum vs. native enum with different values),
   not a shared config shape; `PluginConfig.Raw` per-plugin JSON already
   supports arbitrary shapes here, so no interface change needed, just
   distinct mapping logic per plugin.

## Key files referenced

- `session/backlog_plugin.go` — `ItemSourcePlugin`, `PaginatedFetcher`, `ExternalItem`, `NewDefaultRegistry`
- `session/backlog_plugin_github.go` — pattern to mirror (Fetch/FetchAll/MapToBacklogItem/CloseIssue/PostIssueComment)
- `session/backlog_plugin_github_test.go` — test pattern to mirror
- `session/backlog_sync.go` — `SyncLoop`, `SyncOne`, cursor/watermark/local-wins mechanics, `determineBackwardSyncTarget`
- `session/backlog.go` — `TriggeredBy*` constants
- `server/services/backlog_github_forward_sync.go` — forward-sync-on-completion subscriber, `externalIssueCloser` interface
- `server/mcp/tools_backlog.go` (~L895-1058) — `importGitHubIssue` one-off import MCP tool pattern
- `session/ent/schema/item_source.go` — `ItemSource` ent schema (`forward_sync_enabled`, `backward_sync_enabled`, `forward_sync_close_label`, `sync_cursor`, `config`)
- `session/ent_repository_backlog.go` — `GithubSyncedIssueUpdatedAt` field wiring (naming gotcha)
- `github/keychain.go` — GitHub-specific keychain helper (not directly reusable)
- `web-app/src/components/settings/backlogSourceSchemas.ts` — schema-driven plugin config UI, already anticipates Jira/Linear
- `web-app/src/components/backlog/BacklogItemBadge.tsx` — confirms no existing per-item source badge
- `project_plans/backlog-github-two-way-sync/decisions/ADR-003-loop-prevention-watermark-design.md` — watermark design rationale
