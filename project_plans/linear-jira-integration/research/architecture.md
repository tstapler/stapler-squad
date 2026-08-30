# Architecture Research: Linear and JIRA Backlog Sources

Agent 3 (Architecture), SDD research phase for `linear-jira-integration`. No
prior hotspot/architecture-review analysis existed for this area — this is a
from-scratch codebase read.

## 1. The plugin interface (already the right abstraction)

`session/backlog_plugin.go`:

```go
type ItemSourcePlugin interface {
    PluginID() string
    Fetch(ctx context.Context, config PluginConfig, cursor string) ([]ExternalItem, string, error)
    MapToBacklogItem(item ExternalItem, sourceID string) BacklogItemData
}

type PaginatedFetcher interface { // optional capability, type-asserted
    FetchAll(ctx context.Context, config PluginConfig, cursor string) (items []ExternalItem, newCursor string, possiblyIncomplete bool, err error)
}

type PluginConfig struct { Raw string } // JSON, plugin decodes its own fields

type ExternalItem struct {
    ExternalID, Title, Description string
    Labels []string
    Priority int
    URL string
    State string          // only populated by two-way-sync-capable plugins
    IssueUpdatedAt time.Time
}
```

`NewDefaultRegistry()` (same file) registers `GitHubIssuesPlugin` and
`GitHubPRsPlugin`. Per requirements' own non-goal and
`.claude/rules/interface-pollution-checklist.md`, **no new interface is
needed** — `LinearPlugin` and `JiraPlugin` are two more concrete types
satisfying `ItemSourcePlugin` (and `PaginatedFetcher` if incremental-fetch
needs a "full resync" path), registered with one more `r.Register(...)` line
each in `NewDefaultRegistry()`.

`GitHubIssuesPlugin` (`session/backlog_plugin_github.go`, 432 lines) is the
template to mirror structurally:
- `Fetch` — single page, cursor = `since` query param / API's own
  incremental filter, returns `(items, newCursor, err)`; empty
  token/credentials ⇒ return `(nil, cursor, nil)` (disabled, not an error).
- `FetchAll` — bounded-page aggregation for `PreviewBackwardSyncImpact`
  (only needed if Linear/JIRA implement backward preview; not required by
  this project's goals — see §5).
- `decodeXFetchConfig` vs `decodeXConfig` split: the **Fetch path** treats a
  missing credential as "source disabled" (empty result, no error); the
  **forward-sync path** (`CloseIssue`/`PostIssueComment` equivalents) treats
  it as a hard error, since forward-sync is only ever invoked once a source
  is already known-enabled.
- `MapToBacklogItem` — truncates title/description, sets
  `Status: BacklogStatusIdea`, carries `SourceID`, `ExternalID`,
  `ExternalURL`, `Labels`, `Priority`.

## 2. How the registry is consumed (wiring)

```
server/dependencies.go:1000  syncRegistry := session.NewDefaultRegistry()
server/dependencies.go:1005  backlogCtrl := session.NewBacklogController(..., syncRegistry, keyFunc)
server/dependencies.go:1034  backlogSvc.SetPluginRegistry(syncRegistry)   // same registry, for manual TriggerSync
server/server.go:643         services.StartBacklogGitHubForwardSyncSubscriber(
                                  serverCtx, deps.EventBus,
                                  deps.BacklogService.Registry(),
                                  deps.BacklogService.SyncLoopForForwardSync(),
                                  deps.Storage)
```

One registry instance is shared by the periodic `SyncLoop` (owned internally
by `session.BacklogController`) and the forward-sync `EventBus` subscriber.
Registering `LinearPlugin`/`JiraPlugin` in `NewDefaultRegistry()` is
sufficient to wire them into both the polling loop and (if they implement
the forward-sync capability interface — §4) the forward-sync subscriber; no
other call site needs to change.

**Per-source config is an ent entity, not a config file** —
`session/ent/schema/item_source.go` (`ItemSource`):

```go
plugin_id, display_name, config (JSON, optional, "encrypted PAT"),
enabled, forward_sync_enabled, backward_sync_enabled,
forward_sync_close_label, sync_cursor, last_synced_at,
created_at, updated_at
```

This schema is already fully generic across plugins (`plugin_id` is a
free-form string key into the registry) — **no ent schema change is needed
to add Linear/JIRA as sources**, only new rows with
`plugin_id = "linear"` / `"jira"`. `forward_sync_close_label` is GitHub-label
-shaped but optional/unused by plugins that don't need it.

## 3. Sync loop and the field-level backward-sync path

`session/backlog_sync.go`'s `SyncLoop.SyncOne` (via `runAllSources`) is
plugin-agnostic for the *create/update* path: it calls `plugin.Fetch`,
diffs against the existing item, and applies `Title`/`Description`/
`Priority`/`Labels` updates gated by `UserModifiedFields` (local-wins) and
`source.BackwardSyncEnabled`. This part requires no per-plugin special-casing
— Linear/JIRA plugins get it for free once `Fetch` and `MapToBacklogItem`
are implemented.

**Important scope boundary found in this pass:** the *status*-level
backward-sync block (`backlog_sync.go:432-511`) — auto-archiving a backlog
item when `extItem.State == "closed"`, logging a reopen — is written
GitHub-issue-specific inline in `SyncOne`, not behind a plugin capability
interface. It reads `extItem.State` (a raw string GitHub populates as
`"open"`/`"closed"`) and calls `determineBackwardSyncTarget` (ADR-002).
Requirements' non-goals explicitly scope this project to **one-way
forward-sync only** (session-completion → external tracker), not
external-state → local status backward-sync, so this block does **not**
need to run for Linear/JIRA. Two consequences for planning:
- `LinearPlugin.Fetch`/`JiraPlugin.Fetch` can simply leave `ExternalItem.State`
  at its zero value (`""`) — exactly what `GitHubPRsPlugin` already does for
  the same "two-way sync out of scope" reason, per `ExternalItem.State`'s own
  doc comment.
- If a future project wants backward-sync for Linear/JIRA, this inline block
  is the thing that would need generalizing behind a capability interface
  first — worth flagging as follow-up scope, not blocking this project.

## 4. Forward-sync subscriber pattern (the template for AC5)

`server/services/backlog_github_forward_sync.go` (169 lines) is the
reference implementation for "on session completion, update the external
tracker":

- Subscribes to `EventBus` for `EventBacklogItemChanged` /
  `BacklogChangeStatusTransition` → `NewStatus == BacklogStatusDone`.
- Type-asserts the registry-resolved plugin against a **narrow,
  consumer-defined interface**:
  ```go
  type externalIssueCloser interface {
      CloseIssue(ctx, config, externalID string, existingLabels []string, closeLabel string) (time.Time, error)
      PostIssueComment(ctx, config, externalID string, body string) error
  }
  ```
  `GitHubPRsPlugin` doesn't implement this and the subscriber cleanly
  no-ops — this is the exact "define the interface where it's consumed"
  pattern from `.claude/rules/interface-pollution-checklist.md`. Linear and
  JIRA plugins implementing a similarly-shaped (but not identical —
  see below) pair of methods would satisfy their own consumer-side
  interfaces the same way.
- Re-fetches the item by ID (not trusting the event payload's `SourceID`,
  documented stale-payload gotcha), looks up `ItemSource` by `SourceID`,
  checks `ForwardSyncEnabled`, resolves the plugin from the registry, decrypts
  config via `syncLoop.DecryptConfigToken`, calls close-equivalent + comment
  post (comment failure is best-effort, non-blocking), then persists the
  loop-prevention watermark using **the tracker's own post-write timestamp**
  (not local wall-clock) — ADR-003, called out explicitly in requirements.

**Where Linear and JIRA diverge from "CloseIssue"**, relevant for plan.md's
interface design:
- **Linear**: no universal "closed" state — teams have custom workflow
  states (e.g. "Done", "Cancelled", "Won't Fix") queried per-team via
  GraphQL (`workflowStates`). A `LinearPlugin` forward-sync method is more
  naturally `UpdateIssueState(ctx, config, externalID, targetStateID string) (time.Time, error)` than a parameterless "close" — the *which state* mapping
  either lives in plugin config (a `state_id` or `state_name` field, mirroring
  `forward_sync_close_label`) or a sensible default ("Done"-named state,
  resolved once and cached).
- **JIRA**: forward-sync is a *transition* (workflow-specific transition ID,
  not a fixed target status) — the JIRA REST API's
  `POST /rest/api/3/issue/{key}/transitions` requires resolving available
  transitions first (`GET .../transitions`) and picking one by name (e.g.
  "Done") since transition IDs are project-workflow-specific, not global
  constants.
- Both still fit the "define a narrow consumer interface, type-assert" shape
  — just with a differently-shaped method than GitHub's binary close, e.g.:
  ```go
  type externalIssueStateUpdater interface {
      UpdateIssueState(ctx context.Context, config session.PluginConfig, externalID string, targetLabel string) (time.Time, error)
      PostIssueComment(ctx context.Context, config session.PluginConfig, externalID string, body string) error
  }
  ```
  This can either be a second interface the subscriber also checks, or
  `externalIssueCloser` could be renamed/widened — a plan.md decision, not
  an architecture blocker either way, since both are single-purpose,
  consumer-defined interfaces per the interface-pollution checklist.

### Watermark field is GitHub-named, not generic — a real gap

`BacklogItemData.GitHubSyncedIssueUpdatedAt` / `BacklogItemUpdate.GitHubSyncedIssueUpdatedAt`
(`session/repository.go:444,594`) is a **GitHub-specific field name**, backed
by an ent column literally named `github_synced_issue_updated_at`
(`session/ent/schema` generated code, `session/ent/backlogitem/backlogitem.go:80-81`).
It's used today only by the status-backward-sync block (§3) and by
`handleForwardSyncClose`'s watermark write
(`backlog_github_forward_sync.go:166`).

Requirements' constraints section explicitly asks for the ADR-003 watermark
pattern to be "replicated" for Linear/JIRA forward-sync. Since each backlog
item has exactly one `SourceID` (one tracker origin), a single generic field
is architecturally sufficient — there's no need for
`LinearSyncedIssueUpdatedAt` + `JiraSyncedIssueUpdatedAt` as separate
columns. **Plan.md needs to decide between two options:**
1. **Rename** the field/column to something generic
   (`SourceSyncedItemUpdatedAt` / `source_synced_item_updated_at`) via an ent
   migration — touches the schema file, every generated `session/ent/*`
   file that references it (~30 call sites per the grep below), and
   `session/ent_repository_backlog.go`. Cleaner long-term, larger diff.
2. **Reuse the existing GitHub-named field as-is** for Linear/JIRA's
   watermark too (it's just "last external-tracker-updated_at we've already
   processed", semantically identical regardless of which tracker) —
   zero schema change, but a misleading field name for non-GitHub sources.

Given each item has one source, option 2 is functionally correct today but
is the kind of misnamed-field debt this repo's own conventions
(`.claude/rules/interface-pollution-checklist.md`'s spirit, applied to naming
rather than interfaces) would flag in review. Recommend flagging this as an
explicit decision point in plan.md rather than defaulting silently either
way. (Full call-site list: `grep -rn GithubSyncedIssueUpdatedAt session/ server/` —
concentrated in `session/repository.go`, `session/ent_repository_backlog.go`,
`session/backlog_sync.go`, `server/services/backlog_github_forward_sync.go`,
plus generated ent code.)

## 5. Credentials: GitHub's keychain pattern is GitHub-specific, not generalized

`github/keychain.go` (`package github`) wraps `github.com/zalando/go-keyring`
(OS keychain: macOS Keychain / Secret Service over D-Bus / Windows
Credential Manager) behind a package-level mutex. Its data model is
**GitHub-account-shaped**, not tracker-agnostic:

- `AccountRef{Username, Host}` — supports multiple named GitHub accounts
  across multiple hosts (github.com + GHE instances).
- `GetKeychainTokenForHost(host)` — the one function backlog plugins call
  (`session/backlog_plugin_github.go:161,279`, `session/backlog_plugin_github_prs.go:78`,
  `session/repo_path.go:188`) — walks the account list for a host match,
  falls back to a legacy single-account slot for github.com.
- Keychain service name is hardcoded `"stapler-squad"`; keys are
  `"github-token:<username>"` / `"github-token:<host>:<username>"`.

This **cannot be reused as-is** for Linear or JIRA:
- **Linear**: single global API key per workspace, no per-host or
  per-username concept at all — `GetKeychainTokenForHost` doesn't fit.
- **JIRA**: needs a *pair* (`email` + `api_token`), not a single string, and
  is host-keyed (self-hosted/Cloud instance base URL matters, like GHE).
  go-keyring stores a single string value per key — a JSON-encoded
  `{"email":...,"token":...}` value (mirroring how `PluginConfig.Raw` already
  stores structured JSON) is the natural fit, analogous to `AccountToken`.

**Recommendation for plan.md**: add a new small package (e.g.
`linear/keychain.go`, `jira/keychain.go`, or a shared `trackers/keychain.go`
if the shape is similar enough) following the exact pattern in
`github/keychain.go` — same `keychainMu`-guarded `keyringGet/Set/Delete`
wrappers, same `"stapler-squad"` service name (shared keychain namespace,
different key prefixes: `"linear-api-key"`, `"jira-token:<host>:<email>"`).
This satisfies AC6/AC4's "existing per-host keychain pattern... rather than a
differently shaped auth story" — same *pattern*, necessarily different
concrete shape per tracker, which the requirements doc already anticipates
(env var names `LINEAR_API_KEY` / `JIRA_BASE_URL`+`JIRA_EMAIL`+`JIRA_API_TOKEN`
imply this asymmetry).

### The `PluginConfig.Raw` encrypted-token fallback is a second, existing credential path

Separately from the OS keychain, `SyncLoop.DecryptConfigToken`
(`session/backlog_sync.go:134`) already supports a **per-source encrypted
token embedded in `ItemSource.Config` JSON** (`{"token":"...","encrypted":true}`,
decrypted via `cfg.GetOrCreateEncryptionKey`). GitHub's plugin *prefers* the
keychain and only falls back to this for "sources configured before the
keychain migration" (see `decodeGithubIssuesFetchConfig`'s doc comment).
Both new plugins should follow the same precedence (keychain first,
encrypted-config fallback) for consistency, but the **primary** credential
path should be the new keychain functions per AC6's plaintext-config
prohibition.

### Frontend credential-field shape doesn't yet support JIRA's two-field secret

`web-app/src/components/settings/backlogSourceSchemas.ts`'s `PluginSchema`
has exactly one `tokenLabel`/`token` slot
(`BacklogSourcesSettings.tsx:489-504` renders a single `<input type="password">`
bound to one `token` state variable). JIRA needs two secret-ish fields
(email + API token) plus a non-secret `base_url`. Two options:
1. Treat `base_url` as a plain schema `fields[]` entry (like `owner`/`repo`
   today — non-secret, goes in `PluginConfig.Raw`) and extend the
   single-token assumption to accept a *compound* credential — e.g. widen
   `PluginSchema` with an optional `tokenFields: SourceFieldSchema[]` (plural)
   used when a tracker's secret isn't a single string, falling back to the
   current singular `tokenLabel` for GitHub/Linear-style single tokens.
2. Simpler: since AC4/AC6 push all real secrets into the keychain (not
   `PluginConfig.Raw`) anyway, the JIRA email+token pair could be entered via
   a dedicated small settings sub-page (mirroring GitHub's
   `credentialsManagedExternally` link-out to "Settings → GitHub Accounts"),
   rather than extending the generic schema-driven form's token field at all.
   This matches the existing `credentialsManagedExternally: true` escape
   hatch already in the schema (`backlogSourceSchemas.ts:17`) — worth
   preferring since it needs zero changes to the generic form component,
   only a new small JIRA-credentials settings section + the schema flag.

## 6. One-off MCP import (AC4 — mirrors `import_github_issue`)

`server/mcp/tools_backlog.go:1005-1058` (`importGitHubIssue`) is the
template — registered as an MCP tool at line 1652
(`mcpgo.NewTool("import_github_issue", ...)`):
1. Feature-flag check, caller-session auth via `callerSessionUUID(ctx)`.
2. Parse/validate a tracker URL (`githubpkg.ParseGitHubRefWithHosts`).
3. Fetch the single issue live from the tracker's API
   (`githubpkg.GetIssue`) — **not** via the `ItemSourcePlugin.Fetch`
   incremental path; this is a direct one-off API call.
4. `storage.CreateBacklogItem(...)` with `Title`/`Description`/`Notes:
   "Imported from <url>"` — note this hand-built path does **not** set
   `SourceID`/`ExternalID`/`ExternalURL` on the created item (only `Notes`
   mentions the URL as free text) — worth checking with plan.md whether
   that's an intentional gap in the existing GitHub tool or something the
   Linear/JIRA equivalents should do differently (populating `ExternalURL`/
   `ExternalID` would make the imported item show the same provenance badge
   §7 describes, which seems like the more correct behavior and matches
   what the *polling* sync path already does via `MapToBacklogItem`).
5. `h.backlogSvc.MaybeTriggerTriage(...)` (auto-triage hookup, unconditional
   for all import paths).

New tools `import_linear_issue` / `import_jira_issue` (or one combined tool
disambiguated by URL shape, mirroring how `import_github_issue` only accepts
GitHub URLs) follow this exact shape, needing:
- A Linear GraphQL single-issue-by-ID/URL query, or JIRA
  `GET /rest/api/3/issue/{key}` call.
- New per-feature registry files under `docs/registry/features/backend/`
  per `.claude/rules/feature-registry.md` (AC9).
- `docs/registry/features/backend/import_github_issue.json`'s existing shape
  is the template to copy.

## 7. Frontend touchpoints (AC7 — source badge + filter + clickable URL)

Three components render GitHub-issue provenance today, **all hardcoded to
GitHub**, not schema-driven:

1. **`web-app/src/components/backlog/BacklogItemCard.tsx:197-209`** — list-view
   provenance badge:
   ```tsx
   <a href={item.externalUrl} ... aria-label={`Imported from GitHub issue #${item.externalId}`}>
     <CircleDot aria-hidden="true" size={12} />
     #{item.externalId}
   </a>
   ```
   Hardcoded icon (`CircleDot`, chosen because lucide-react's pinned version
   has no GitHub brand glyph — see the comment in `SourceSection.tsx`) and
   hardcoded `aria-label`/`#` prefix text.

2. **`web-app/src/components/backlog/detail/SourceSection.tsx`** —
   detail-view "Source" section, same hardcoding: `title="Open on GitHub"`,
   `CircleDot` icon, `Issue #<id>` text.

3. **`web-app/src/components/settings/backlogSourceSchemas.ts`** — already
   **the correct, schema-driven extension point** (its own doc comment says
   so explicitly): *"Adding a source type for a new plugin (e.g. Jira,
   Linear) means adding one entry here."* This is where `github_issues`/
   `github_prs` are declared with `fields`, `requiresToken`,
   `credentialsManagedExternally`. `linear`/`jira` entries go here — see §5
   for the JIRA two-field-credential wrinkle.

**Gap for plan.md**: (1) and (2) need to become source-aware (branch on
`item.pluginId`/`item.sourceId`'s plugin type, or add a small
per-plugin-family badge-config lookup keyed by plugin ID — icon, label
prefix, "Open on X" text) rather than the current single hardcoded
GitHub rendering. This is a **UI generalization that has to happen
regardless of backend plugin count** — it's the actual AC7 work, not
incidental. The plugin ID is already threaded through
(`useBacklogSourcesService.ts:11,59,84,137` carries `pluginId`;
`proto/session/v1/backlog.proto:132` has `source_id` on the item, `:509+`
has `ItemSource` with presumably a `plugin_id` field) — so the data needed
to branch is already available, just not consumed by these two components
yet.

**Filter by source**: no dedicated "filter backlog items by source" UI or
RPC param was found in this pass (`grep` for `sourceFilter`/sourceId filter
params in backlog list hooks came back empty — the only `sourceFilter` hit
is `useApprovalRules.ts`, an unrelated feature). AC7's "filterable by source"
likely needs a new filter param on the backlog list RPC/query (mirroring how
status/priority filters presumably already work) plus a UI control — flag
this as a real (not just cosmetic) scope item for plan.md, not just the
badge rendering.

## 8. End-to-end data flow summary

**(a) Polling sync** (existing, generalizes for free once plugins exist):
`BacklogController`'s periodic `SyncLoop.runAllSources` → for each enabled
`ItemSource` → `SyncLoop.SyncOne` → `registry.Get(source.PluginID)` →
`plugin.Fetch(ctx, PluginConfig{Raw: decrypted config}, source.SyncCursor)`
→ diff against existing items by `ExternalID` → create new / update
unlocked fields → persist new `sync_cursor` + `last_synced_at` → emit
`SourceSyncEvent` (aggregate counts).

**(b) One-off MCP import** (new tools, same shape as `import_github_issue`):
MCP tool call → parse/validate tracker URL → direct single-item API fetch
(not through the plugin's `Fetch`) → `storage.CreateBacklogItem` → optional
auto-triage trigger. Should additionally populate `SourceID`/`ExternalID`/
`ExternalURL` (see §6 point 4) so the imported item gets the same
badge/filter treatment as polling-sync-imported items.

**(c) Forward-sync on session completion** (new subscriber, mirrors
`StartBacklogGitHubForwardSyncSubscriber`): `EventBus` publishes
`EventBacklogItemChanged` (`BacklogChangeStatusTransition` → `Done`) on
session completion → subscriber re-fetches the item by ID → looks up its
`ItemSource` → checks `ForwardSyncEnabled` → type-asserts the resolved
plugin against a Linear/JIRA-appropriate narrow interface (§4) → posts a
visible comment *and* updates state/transitions the issue (no-silent-action
convention) → persists the watermark using the tracker's own post-write
timestamp (§4's naming caveat applies here).

New wiring needed: one more
`services.StartBacklog{Linear,Jira}ForwardSyncSubscriber(...)` call
alongside the existing GitHub one in `server/server.go:643`, or — cleaner,
since the dispatch logic (event filter, item re-fetch, source lookup,
`ForwardSyncEnabled` check, watermark persistence) is identical across all
three trackers and only the "how do I close/transition the issue" part
differs — a single generalized subscriber that type-asserts against
whichever tracker-specific capability interface the resolved plugin
satisfies. The latter avoids near-duplicate subscriber goroutines/event-loop
boilerplate three times over; worth raising as a design choice in plan.md
rather than assuming duplication is the default.

## 9. EventStorming — skipped, per instructions

This is "implement two more plugins conforming to an existing interface,"
not a new multi-actor business domain — the existing forward-sync
subscriber pattern (§4) is already the full Event → Policy → Command chain
(`BacklogChangeStatusTransition` event → `ForwardSyncEnabled` policy check →
`CloseIssue`/`UpdateIssueState` command) and doesn't reveal new actors or
policies when extended to two more tracker types. No Event-Command-Policy
table added.

## 10. Summary of concrete integration points for plan.md

| # | File | Change |
|---|---|---|
| 1 | `session/backlog_plugin_linear.go` (new) | `LinearPlugin` implementing `ItemSourcePlugin` (+ forward-sync capability interface) |
| 2 | `session/backlog_plugin_jira.go` (new) | `JiraPlugin` implementing `ItemSourcePlugin` (+ forward-sync capability interface) |
| 3 | `session/backlog_plugin.go` | Register both in `NewDefaultRegistry()` |
| 4 | `linear/keychain.go`, `jira/keychain.go` (new) | Tracker-shaped credential storage mirroring `github/keychain.go`'s pattern |
| 5 | `session/repository.go` + ent schema | Decide: reuse `GithubSyncedIssueUpdatedAt` as-is, or rename to a generic watermark field (migration) |
| 6 | `server/services/backlog_linear_forward_sync.go`, `backlog_jira_forward_sync.go` (new), or a generalized single subscriber | Forward-sync subscribers, mirrors `backlog_github_forward_sync.go` |
| 7 | `server/server.go` | Wire new subscriber(s) alongside the GitHub one |
| 8 | `server/mcp/tools_backlog.go` | New `import_linear_issue`/`import_jira_issue` MCP tools, mirroring `importGitHubIssue` |
| 9 | `web-app/src/components/settings/backlogSourceSchemas.ts` | Add `linear`/`jira` `PluginSchema` entries (already the documented extension point) |
| 10 | `web-app/src/components/backlog/BacklogItemCard.tsx`, `detail/SourceSection.tsx` | Generalize hardcoded GitHub icon/label to branch on plugin/source type |
| 11 | Backlog list RPC + filter UI | Add filter-by-source (no existing mechanism found) |
| 12 | `docs/registry/features/backend/*.json` | New entries for the two MCP tools, per `.claude/rules/feature-registry.md` |
