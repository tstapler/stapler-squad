# Implementation Plan: linear-jira-integration

**Feature**: Two new `ItemSourcePlugin` implementations (Linear, JIRA) with polling sync, one-off MCP import, forward-sync-on-completion, and source-aware backlog UI, extending the existing GitHub Issues integration pattern.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: [ADR-001: Reuse `GitHubSyncedIssueUpdatedAt` As-Is](../decisions/ADR-001-reuse-github-named-watermark-field.md), [ADR-002: One Generalized Forward-Sync Subscriber](../decisions/ADR-002-generalized-forward-sync-subscriber.md)

---

## Scope Correction vs. Research

`research/ux.md` framed requirements.md's Goal 5 / AC7 as "issue source badge
**on session cards**, filter **sessions** by issue source" and scoped a large
amount of work (denormalizing source fields onto the `Session` proto, a new
`IssueSourceBadge` component parallel to `GitHubBadge.tsx`, a `SessionList.tsx`
filter select) around that reading.

Requirements.md's actual Goal 5 and AC7 text says **backlog items**, not
sessions: *"backlog items sourced from Linear/JIRA show a source badge... and
are filterable by source"* / *"Backlog items sourced from Linear or JIRA
display a source badge in the web UI and are filterable by source."* Per this
task's own instruction ("requirements.md wins" on conflicts), this plan scopes
Phase 4 to **backlog items only** — `BacklogItemCard.tsx`, `SourceSection.tsx`,
and the backlog list/board, not `Session`/`SessionCard.tsx`/`SessionList.tsx`.
This removes the `Session` proto denormalization, `GitHubBadge`-parallel
component, and session-list filter work `research/ux.md` scoped — none of it
is required by any AC in requirements.md. If a future project wants
session-level provenance (closing the "user has to click through to the
backlog item" gap `research/ux.md` §5 identifies), that's a distinct,
separately-scoped follow-up, not silently folded into this one.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `ItemSourcePlugin` | Existing Go interface (`session/backlog_plugin.go:9`) every tracker integration implements: `PluginID()`, `Fetch(ctx, config, cursor)`, `MapToBacklogItem(item, sourceID)`. | No changes to this interface — `LinearPlugin`/`JiraPlugin` are two more implementations. |
| `PaginatedFetcher` | Optional capability interface (`session/backlog_plugin.go:25`) with `FetchAll`, type-asserted only by `PreviewBackwardSyncImpact`. | Not required by any AC in requirements.md; not implemented by either new plugin in this plan. |
| `ExternalItem` | Tracker-agnostic shape (`session/backlog_plugin.go:39`) `Fetch` returns: `ExternalID`, `Title`, `Description`, `Labels`, `Priority`, `URL`, `State`, `IssueUpdatedAt`. | `State` left at zero value (`""`) for both new plugins — see Pattern Decision row 8. |
| `PluginConfig` | `{Raw string}` (`session/backlog_plugin.go:34`) — plugin-owned JSON blob, decoded independently by each plugin. | Never carries secrets directly per AC6; secrets resolve from keychain. |
| `BacklogItemData` | Internal Go struct (`session/repository.go:356`) a plugin's `MapToBacklogItem` produces; persisted via `storage.CreateBacklogItem`/`UpdateBacklogItem`. | |
| `ItemSource` | ent entity (`session/ent/schema/item_source.go`) — one row per configured source (`plugin_id`, `config`, `enabled`, `forward_sync_enabled`, `forward_sync_close_label`, `sync_cursor`). | No schema change needed — `plugin_id = "linear"` / `"jira"` are just new rows. |
| `PluginRegistry` / `NewDefaultRegistry()` | In-memory map (`session/backlog_plugin.go:60,79`) from `plugin_id` to `ItemSourcePlugin`, shared by the polling `SyncLoop` and the forward-sync subscriber. | Gains two more `r.Register(...)` calls. |
| `LinearPlugin` | New type in `session/backlog_plugin_linear.go` implementing `ItemSourcePlugin` (+ forward-sync capability, §Epic 2.1) via hand-rolled GraphQL over `net/http`. | |
| `JiraPlugin` | New type in `session/backlog_plugin_jira.go` implementing `ItemSourcePlugin` (+ forward-sync capability) via `github.com/andygrunwald/go-jira/v2/cloud`. | |
| `linearPluginConfig` | Decoded `PluginConfig.Raw` shape for Linear: `TeamID`/`TeamKey`, `LabelPriorityMap` (unused v1, native priority only — Pattern Decision row omitted from table, see Story 1.2.2), no token field (resolved via keychain). | Mirrors `githubPluginConfig` (`session/backlog_plugin_github.go:46`). |
| `jiraPluginConfig` | Decoded `PluginConfig.Raw` shape for JIRA: `BaseURL`, `ProjectKey`, no email/token fields (resolved via keychain). | Cloud-only in v1 — see Unresolved Questions / Pattern Decision row 6. |
| `linearIssue` | Wire-shape struct decoded from Linear's GraphQL `issues`/`issue` response `nodes`. | Mirrors `githubIssue` (`session/backlog_plugin_github.go:53`). |
| `cloud.Issue` | `go-jira/v2/cloud`'s typed issue struct — `JiraPlugin` maps this to `ExternalItem`/`BacklogItemData` rather than hand-decoding raw JSON. | |
| `externalIssueCloser` | Existing narrow interface (`server/services/backlog_forward_sync.go`, formerly `backlog_github_forward_sync.go`) — `CloseIssue` + `PostIssueComment`. `GitHubIssuesPlugin` implements it; unchanged by this plan. | |
| `externalIssueStateUpdater` | New narrow interface (same file) — `UpdateIssueState(ctx, config, externalID, targetLabel string) (time.Time, error)` + `PostIssueComment(...)`. `LinearPlugin` and `JiraPlugin` both implement it (JIRA's method is named `TransitionIssue` internally but also satisfies this interface's `UpdateIssueState` method name — see Story 2.1.2 for the exact Go method naming). | Introduced per ADR-002. |
| `GitHubSyncedIssueUpdatedAt` | Existing loop-prevention watermark field (`session/repository.go:447`, ent column `github_synced_issue_updated_at`) — reused as-is for Linear/JIRA per ADR-001, despite the GitHub-specific name. | Not renamed in this project. |
| ADF (Atlassian Document Format) | JIRA Cloud's structured-JSON rich-text representation for `description`/`comment` bodies (`{type:"doc", version:1, content:[...]}`) — not a plain string. | New `jira/adf.go` (or inline in `backlog_plugin_jira.go`) walker converts ADF → plaintext (inbound) and wraps plaintext → minimal ADF (outbound comments). |
| Linear `WorkflowState` | Team-scoped GraphQL object (`id`, `name`, `type`) representing one column in a Linear team's workflow — Linear's equivalent of a JIRA status. Resolved via a `workflowStates` query, matched by name against `ForwardSyncCloseLabel`. | |
| JIRA transition | A directed edge in a JIRA project's workflow graph, addressed by a workflow-specific integer/string ID, discovered via `GET /issue/{id}/transitions` and resolved by name-matching against `ForwardSyncCloseLabel`. | Not a fixed enum — see pitfalls.md §2. |
| `ForwardSyncCloseLabel` | Existing `ItemSource` field (`session/ent/schema/item_source.go`), originally "which GitHub label to apply on close." Reused for Linear/JIRA as "target workflow-state/transition name" (e.g. `"Done"`). | Field name stays as-is (same reuse-don't-rename reasoning as ADR-001; flagged, not renamed). |
| `SyncCursor` / cursor | `ItemSource.sync_cursor` (opaque string) — for Linear, a GraphQL `after` pagination cursor combined with an `updatedAt` filter value; for JIRA, an `updated >= "<cursor>"` JQL clause value. | |
| `SourcePluginID` | New field 33 on the `BacklogItem` proto message (`plugin_id` on the wire) — denormalized from the item's already-eager-loaded `ItemSource` edge, so the frontend can badge/style by tracker without a client-side join. | Pattern Decision row 11. |
| `TriggeredByLinearSync` / `TriggeredByJiraSync` | New constants in `session/backlog.go` alongside `TriggeredByGitHubSync` (`session/backlog.go:96`) — per-tracker attribution string in `BacklogStatusEvent.TriggeredBy`. | |
| `linear.GetKeychainToken()` | New function in `linear/keychain.go` — single global token, no host/username concept (unlike GitHub). | |
| `jira.GetKeychainTokenForHost(baseURL)` | New function in `jira/keychain.go` — per-`base_url`-keyed compound secret (`{"email":"...","token":"..."}` JSON string stored as one keyring value). | |
| `linearGraphQLURL` | New package-level var in `session/backlog_plugin_linear.go`, mirrors `githubAPIBaseURL` (`session/backlog_plugin_github.go:30`) — test-only override, not parallel-safe (documented inline, same caveat carried forward). | |
| `StartBacklogForwardSyncSubscriber` | Generalized subscriber entry point (renamed from `StartBacklogGitHubForwardSyncSubscriber` per ADR-002) — one `EventBus` subscription dispatching to whichever capability interface (`externalIssueCloser` / `externalIssueStateUpdater`) the resolved plugin satisfies. | |
| `source_plugin_filter` | New `repeated string` field on `ListBacklogItemsRequest` and `WatchBacklogItemsRequest` protos — server-side filter-by-tracker, mirroring the existing `status_filter`/`category_filter` fields. | Pattern Decision row 12. |
| `credentialsManagedExternally` | Existing `PluginSchema` flag (`backlogSourceSchemas.ts:17`) that swaps a generic token input for a link to a dedicated credentials page. Reused for JIRA's compound (email+token) secret via a new small JIRA-credentials settings sub-section. | Pattern Decision row 9. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Plugin file structure | Independent per-tracker files (`backlog_plugin_linear.go`, `backlog_plugin_jira.go`), no shared `trackers` package | GitHub plugin precedent; build-vs-buy.md (GraphQL vs. SDK-wrapper, no shared transport shape) | Shared `trackers` HTTP/keychain/error-wrapping scaffolding package | Only 2 call sites with structurally different HTTP shapes (hand-rolled GraphQL vs. `go-jira` SDK wrapper) — a shared package would be a near-empty, speculative abstraction (interface-pollution-checklist.md smell #1). |
| Forward-sync subscriber | One generalized `StartBacklogForwardSyncSubscriber` dispatching via type-switch on two capability interfaces (ADR-002) | architecture.md §4/§8 — dispatch pipeline is byte-identical per tracker today | 3 separate near-duplicate subscriber files mirroring GitHub's file 1:1 | Re-fetch-by-ID gotcha, `ForwardSyncEnabled` gate, and ADR-003 watermark-write logic would each need 3 copies kept in sync; a future dispatch-level fix lands once instead of three times. |
| Forward-sync capability interface shape | Two narrow consumer-defined interfaces: `externalIssueCloser` (existing, unchanged) + new `externalIssueStateUpdater` | architecture.md §4 — GitHub's binary close doesn't fit Linear/JIRA's parameterized target-state model | Widen `externalIssueCloser`'s signature to take a target-state parameter for all three trackers | GitHub's `CloseIssue(existingLabels, closeLabel)` shape is shipped and tested; changing it touches a working, unrelated plugin for no reason. New interface for new behavior, per interface-pollution-checklist.md item 2. |
| Watermark field | Reuse `GitHubSyncedIssueUpdatedAt` as-is (ADR-001) | architecture.md's explicit decision point | ent migration renaming to `SourceSyncedItemUpdatedAt` | ~15-file rename for a pure naming fix with zero behavior change; each `BacklogItem` has exactly one `SourceID` so the field is already functionally correct. Smallest-diff bias, flagged via ADR for future revisit. |
| Credentials | New small `linear/keychain.go` (single global token) + `jira/keychain.go` (per-base-URL compound secret) packages, mirroring `github/keychain.go`'s `keyringGet/Set/Delete` + mutex pattern | stack.md / architecture.md §5 | One shared generic `trackers/keychain.go` keyed by `(plugin_id, instance_id)` | Linear (no host/username concept) and JIRA (host-keyed compound secret) shapes differ enough that a generic keyed-tuple abstraction would need `interface{}` payload type-erasure for zero real reuse beyond 3-line wrapper functions already trivial to duplicate. |
| JIRA HTTP client | Adopt `github.com/andygrunwald/go-jira/v2/cloud` | build-vs-buy.md final recommendation | Hand-rolled `net/http` + `encoding/json` matching GitHub's style | JIRA's GET-transitions→name-match→POST-transition dance and JQL search-endpoint churn is materially higher correctness-risk than GitHub's single idempotent PATCH; a maintained, exercised library absorbs edge cases a first-cut hand-roll would reintroduce. |
| Linear HTTP client | Hand-rolled `net/http` + `encoding/json` GraphQL POST helper, package-level `linearGraphQLURL` test override | build-vs-buy.md / stack.md | `Khan/genqlient` codegen client or `hasura/go-graphql-client` reflection client | No well-maintained Go SDK exists for Linear; ~5 fixed GraphQL operations don't justify a codegen pipeline or reflection-based abstraction. Matches GitHub plugin's zero-dependency convention. |
| Backward-sync state classification | Leave `ExternalItem.State` at zero value (`""`) for both plugins — `backlog_sync.go`'s status-backward-sync block never fires for Linear/JIRA | requirements.md non-goals (one-way forward-sync only); architecture.md §3 | `ClassifyState(rawState string) bool` capability interface generalizing `determineBackwardSyncTarget` | Explicitly out of scope per requirements.md non-goals; building the classification hook now is speculative interface work for a feature not requested. |
| JIRA compound credential (frontend) | `credentialsManagedExternally: true` escape hatch — dedicated small JIRA-credentials settings sub-section | architecture.md §5 option 2 | Widen `PluginSchema` with plural `tokenFields: SourceFieldSchema[]` | Zero changes needed to the generic schema-driven form component; a plural-fields widening touches `BacklogSourcesSettings.tsx` rendering logic for a case only JIRA needs today. The escape hatch already exists and is proven (GitHub uses it). |
| MCP tool shape | Two new tools: `import_linear_issue`, `import_jira_issue` | requirements.md AC4 ("(or tools)"); existing 1:1 `import_github_issue` precedent | One `import_issue` tool dispatching by URL shape | `import_github_issue`'s params are GitHub-URL-specific; a dispatch tool still needs per-tracker URL/key-sniffing logic internally and loses tracker-specific parameter docs (e.g. JIRA keys like `PROJ-456` aren't URLs at all). Matches "one tool per external system" convention. |
| Backlog item source badge data source | Denormalize `plugin_id` onto `BacklogItem` proto (field 33), populated from the already-eager-loaded `Edges.Source` in `backlogItemToData` (`session/ent_repository_backlog.go:172-213`) | architecture.md §7 gap; confirmed eager-load at both `GetBacklogItem` and `ListBacklogItems` (`session/ent_repository_backlog.go:331,445`) | Frontend joins `item.sourceId` against a separately-fetched sources list via context | `Edges.Source` is already loaded server-side for both read paths (zero extra queries) — denormalizing one string field is cheaper than threading a sources-by-id map through `BacklogBoard` → `BacklogItemCard` → `SourceSection`. |
| Backlog list source filter | Add `source_plugin_filter` (repeated string) to `WatchBacklogItemsRequest`/`ListBacklogItemsRequest`, mirroring existing `status_filter`/`category_filter` | architecture.md §7 — no existing filter-by-source mechanism found | Client-side-only filtering (fetch all items, filter in the browser) | `WatchBacklogItemsRequest` already has a server-side filter precedent (status/category) for the identical "don't push the full snapshot + every live event to clients that only care about a subset" reason — the pattern already exists and is low-cost to extend. |
| JIRA deployment type | Cloud only (`.../v2/cloud`, Basic auth with email+API token) | stack.md / build-vs-buy.md; requirements.md's `JIRA_EMAIL`/`JIRA_API_TOKEN` credential list implies Cloud's auth shape | Support both Cloud and Server/Data Center (`.../v2/onpremise`) in v1 | Requirements.md's credential list (`JIRA_EMAIL` + `JIRA_API_TOKEN`) is Cloud-shaped; Server/DC's PAT-only auth has no `email` field at all. Server/DC is not requested and `go-jira/v2` already models it as a second, separate package — adding it later is additive, not a rework. |

---

## Migration Plan

**No ent schema migration** (see ADR-001 — the watermark field is reused
as-is, and `ItemSource`'s existing schema is already generic across
`plugin_id` values per architecture.md §2, confirmed: no new ent fields
needed for `ItemSource` itself).

**Proto changes** (additive, non-breaking, require `make proto-gen` — no
migration, just regenerated bindings):
- `BacklogItem.plugin_id` (new field 33, string) — Epic 4.1.
- `ListBacklogItemsRequest.source_plugin_filter` (new field 6, repeated string) — Epic 4.1.
- `WatchBacklogItemsRequest.source_plugin_filter` (new field 4, repeated string) — Epic 4.1.

**`go.mod` change**: add `github.com/andygrunwald/go-jira/v2` (pin latest
v2.x tag at implementation time per stack.md) — Story 1.3.1.

## Observability Plan
- **Logs**: Both new plugins follow `GitHubIssuesPlugin`'s existing
  discipline of *never* logging inside plugin code (confirmed zero `log.*`
  calls in `session/backlog_plugin_github.go` per pitfalls.md §6) — all
  logging happens at the subscriber/sync-loop layer, which already never
  interpolates `cfg.Token`/`config.Raw` into log lines. New code must be
  checked against this same rule at review time (Story 1.2.1/1.3.1 tasks
  include an explicit "no log.* calls, no error string interpolates a raw
  header/token" check).
- **Metrics**: No new metrics system — reuses existing
  `RecordSourceSyncFailure`/`CreateSourceSyncEvent` (`session/backlog_sync.go`,
  `session/ent_repository_backlog.go:1742`), surfaced today via the
  Settings UI's sync-history table and `isAuthFailure` banner. Pitfalls.md
  §6's warning (persisted raw `error.Error()` strings can leak request
  details) is addressed by Story 1.2.1/1.3.1's client-error-audit tasks.
- **Alerts**: None new — existing per-source `role="alert"` "Sync failing"
  banner (`BacklogSourcesSettings.tsx:320-328`) is the only alerting surface
  and is reused (Epic 4.5 verifies Linear/JIRA error strings actually match
  `isAuthFailure`'s heuristic, per pitfalls.md §3).

## Risk Control
- **Feature flag**: None needed beyond the existing implicit one — a
  `LinearPlugin`/`JiraPlugin` registered in `NewDefaultRegistry()` is
  completely inert until a user creates an `ItemSource` row for it via
  Settings (same as `GitHubPRsPlugin` today: registered but dormant absent
  configured sources). No build-tag or config-flag gating needed.
- **Rollback procedure**: Revert the PR (plugins simply stop being
  registered; any already-created Linear/JIRA `ItemSource` rows become
  inert — `registry.Get(source.PluginID)` returns `ok=false`, `SyncOne`
  and the forward-sync subscriber both already no-op cleanly on that path,
  confirmed by reading `handleForwardSyncClose`'s existing `!ok` branch).
  No data migration to reverse (ADR-001 means no schema change to roll back).
- **Staged rollout**: Ship all phases behind normal PR review; no
  progressive-rollout mechanism needed since the feature is opt-in per user
  (they must add credentials and an `ItemSource` row before anything syncs).
  Recommend landing Phase 1 (fetch/mapping) and Phase 3 (one-off import)
  before Phase 2 (forward-sync writes to external trackers) in separate PRs
  if the implementer wants an extra manual-verification checkpoint before
  granting write access to Linear/JIRA — not required by any AC, a
  sequencing suggestion only.

## Unresolved Questions
- [ ] Should `import_linear_issue`/`import_jira_issue` also gain an
      equivalent ConnectRPC method (mirroring `ImportGitHubIssue`'s dual
      RPC + MCP-tool existence, `server/services/backlog_service_sync.go:222`
      + `server/mcp/tools_backlog.go:1131`), enabling a future "Import from
      Linear/JIRA" button in the web UI? Requirements.md AC4 only requires
      an MCP tool; this plan scopes Epic 3.1/3.2 to MCP-tool-only. — blocks
      nothing in this plan, but blocks a future UI import button — owner:
      Tyler (product call, not specified in requirements.md).
- [ ] Confirm exact current JIRA Cloud rate-limit header names before
      implementing Story 1.3.3's rate-limit detector — pitfalls.md §4
      flags this as an active migration (phased rollout starting March 2,
      2026); this doc's snapshot may already be stale by implementation
      time. — blocks Task 1.3.3b — owner: implementer, verify against
      developer.atlassian.com/cloud/jira/platform/rate-limiting at task time.
- [ ] Confirm whether `go-jira/v2/cloud` exposes ADF-native helper types
      for building the outbound comment body, or whether Story 1.3.2/2.1.2
      must hand-roll the minimal ADF wrapper — pitfalls.md §7 flags this as
      unconfirmed ("likely has ADF helper types worth checking"). — blocks
      Task 1.3.2a's exact approach — owner: implementer, resolve via
      `go doc github.com/andygrunwald/go-jira/v2/cloud` once the dependency
      is vendored (Task 1.3.1a).

## Dependency Visualization
```
Phase 1: Backend Plugin Foundations
  Epic 1.1 (keychain pkgs) ──┐
  Epic 1.2 (LinearPlugin)  ──┼──> Epic 1.3 (JiraPlugin, parallel-safe with 1.2)
                              │
Phase 2: Forward-Sync (needs Phase 1 plugins to exist)
  Epic 2.1 (tracker methods) ──> Epic 2.2 (generalized subscriber, ADR-002)

Phase 3: One-off MCP Import (needs Phase 1 Fetch/Mapping code for shared
          decode helpers; independent of Phase 2)
  Epic 3.1 (Linear tool) ──┐
  Epic 3.2 (Jira tool)   ──┼──> Epic 3.3 (registry entries)

Phase 4: Frontend (needs Epic 1.1-1.3 for schema entries to be meaningful;
          Epic 4.1's proto fields are independent of Phase 1-3 Go code)
  Epic 4.1 (proto denorm + filter) ──> Epic 4.3 (badges) ──> Epic 4.4 (filter UI)
  Epic 4.2 (PLUGIN_SCHEMAS)        ──> Epic 4.5 (settings genericization)

Phase 5: Testing & Registry (spans all phases, sequenced last per story)
  Epic 5.1 (plugin unit tests) — depends on Phase 1
  Epic 5.2 (subscriber tests)  — depends on Phase 2
  Epic 5.3 (e2e test)          — depends on Phase 4
```

---

## Phase 1: Backend Plugin Foundations

### Epic 1.1: Credential Storage Packages
**Goal**: Linear and JIRA credentials are readable from the OS keychain, never from `PluginConfig.Raw` plaintext, following `github/keychain.go`'s pattern (AC6).

#### Story 1.1.1: Linear keychain helper
**As a** backlog administrator, **I want** my Linear API key stored in the OS keychain, **so that** it never appears in plaintext config or logs.
**Acceptance Criteria**:
- A single global Linear API key can be stored and retrieved via `linear.SetKeychainToken`/`linear.GetKeychainToken`.
  - *Given* no Linear token has been stored yet, *When* `linear.GetKeychainToken()` is called, *Then* it returns `""` (no error), mirroring `github.GetKeychainToken`'s empty-slot behavior.
**Files**: `linear/keychain.go` (new), `linear/keychain_test.go` (new), `linear/main_test.go` (new)

##### Task 1.1.1a: Create `linear/keychain.go` (~5 min)
- New package `linear` at repo root (sibling to `github/`). Copy `github/keychain.go`'s `keychainMu sync.Mutex` + `keyringGet/Set/Delete` wrapper trio verbatim (same `"stapler-squad"` service name, different key: `keychainTokenKey = "linear-api-key"`).
- `GetKeychainToken() string` / `SetKeychainToken(token string) error` / `DeleteKeychainToken() error` — no `AccountRef`/host machinery (Linear has one workspace secret, no per-host variance).
- Files: `linear/keychain.go`

##### Task 1.1.1b: Test coverage + `TestMain` keyring mock (~4 min)
- `linear/main_test.go`: `TestMain` calling `keyring.MockInit()`, mirroring `github/main_test.go:17`.
- `linear/keychain_test.go`: round-trip set/get, delete, empty-slot-returns-empty-string.
- Files: `linear/main_test.go`, `linear/keychain_test.go`

#### Story 1.1.2: JIRA keychain helper
**As a** backlog administrator, **I want** my JIRA email+API-token pair stored per JIRA base URL, **so that** multiple JIRA Cloud sites can be configured without credential collision.
**Acceptance Criteria**:
- A `(email, token)` pair can be stored and retrieved keyed by JIRA base URL host.
  - *Given* `jira.SetKeychainTokenForHost("https://acme.atlassian.net", "alice@acme.com", "tok_abc")` has been called, *When* `jira.GetKeychainTokenForHost("https://acme.atlassian.net")` is called, *Then* it returns `("alice@acme.com", "tok_abc", true)`.
**Files**: `jira/keychain.go` (new), `jira/keychain_test.go` (new), `jira/main_test.go` (new)

##### Task 1.1.2a: Create `jira/keychain.go` (~5 min)
- New package `jira` at repo root. Same `keychainMu`/`keyringGet/Set/Delete` trio, service `"stapler-squad"`, key shape `"jira-token:<normalized-host>"`.
- Value stored as a JSON string `{"email":"...","token":"..."}` (mirrors `PluginConfig.Raw`'s own JSON-blob convention, since go-keyring only stores one string per key).
- `GetKeychainTokenForHost(baseURL string) (email, token string, ok bool)` / `SetKeychainTokenForHost(baseURL, email, token string) error` / `DeleteKeychainTokenForHost(baseURL string) error`.
- Files: `jira/keychain.go`

##### Task 1.1.2b: Test coverage + `TestMain` keyring mock (~4 min)
- `jira/main_test.go`: `TestMain` with `keyring.MockInit()`.
- `jira/keychain_test.go`: round-trip, missing-host-returns-ok-false, two different hosts don't collide.
- Files: `jira/main_test.go`, `jira/keychain_test.go`

---

### Epic 1.2: LinearPlugin — Fetch & Mapping
**Goal**: `LinearPlugin` fetches Linear issues incrementally via GraphQL and maps them to `BacklogItemData` (AC1, AC3).

#### Story 1.2.1: Config decode + GraphQL Fetch
**As a** backlog sync loop, **I want** `LinearPlugin.Fetch` to return new/updated Linear issues since the last cursor, **so that** the polling sync loop doesn't refetch the whole team every tick.
**Acceptance Criteria**:
- Empty/missing Linear token results in a disabled no-op fetch, not an error.
  - *Given* a `PluginConfig{Raw: '{"team_id":"eng"}'}` with no Linear token stored in the keychain and no fallback token in config, *When* `LinearPlugin.Fetch(ctx, config, "")` is called, *Then* it returns `(nil, "", nil)`.
- Incremental fetch advances the cursor to the latest `updatedAt` seen.
  - *Given* Linear's GraphQL response contains two issues with `updatedAt` `"2026-08-01T00:00:00Z"` and `"2026-08-03T00:00:00Z"`, *When* `LinearPlugin.Fetch(ctx, config, "2026-07-01T00:00:00Z")` is called, *Then* the returned cursor is `"2026-08-03T00:00:00Z"`.
**Files**: `session/backlog_plugin_linear.go` (new)

##### Task 1.2.1a: `linearPluginConfig` decode + keychain resolution (~5 min)
- `linearPluginConfig{TeamID string; LabelPriorityMap map[string]int}` (no token field). `decodeLinearFetchConfig(config PluginConfig) (cfg linearPluginConfig, disabled bool, err error)` — token resolved via `linear.GetKeychainToken()` first, `PluginConfig.Raw`'s legacy `token`/`encrypted` fallback second (mirrors `decodeGithubIssuesFetchConfig`'s precedence, architecture.md §5's "keychain first, encrypted-config fallback"). Missing token ⇒ `disabled=true`, no error.
- Files: `session/backlog_plugin_linear.go`

##### Task 1.2.1b: GraphQL request helper + `linearGraphQLURL` test override (~5 min)
- `linearGraphQLURL = "https://api.linear.app/graphql"` package var (test override, same non-parallel-safe caveat as `githubAPIBaseURL`, comment carried forward verbatim per pitfalls.md §8).
- `doLinearGraphQL(ctx, token string, query string, variables map[string]any, out any) error` — `POST`, `Content-Type: application/json`, `Authorization: <token>` header **with no `Bearer`/`token` prefix** (pitfalls.md §5 — comment this explicitly to prevent a future "helper unification" regression). Decodes `{"data":...}` or surfaces `{"errors":[...]}` as a Go error.
- Files: `session/backlog_plugin_linear.go`

##### Task 1.2.1c: `Fetch` — issues query + cursor advancement (~5 min)
- Static query template (illustrative shape from stack.md §Linear, `issues(first: 50, filter: {updatedAt: {gt: $cursor}}, orderBy: updatedAt) { nodes { id identifier title description url state{name type} priority labels{nodes{name}} updatedAt } pageInfo{hasNextPage endCursor} } }`).
- `Fetch` calls `doLinearGraphQL`, converts `nodes` → `[]ExternalItem` (pure `convertLinearIssues`-style function, mirrors `convertGithubIssues`), cursor = `max(cursor, latest updatedAt)` string-wise (RFC3339 sorts lexicographically, same trick GitHub's plugin uses).
- Files: `session/backlog_plugin_linear.go`

##### Task 1.2.1d: Auth-failure and rate-limit error engineering (~5 min)
- Per pitfalls.md §3: when `doLinearGraphQL` surfaces a GraphQL `errors[].extensions.type == "authentication_error"` or `"forbidden"`, wrap it into a Go error whose string contains a literal `"401"` token (e.g. `fmt.Errorf("linear: authentication failed (401): %s", msg)`) so the frontend's existing `isAuthFailure()` heuristic matches without any frontend change.
- Per pitfalls.md §4: when `extensions.type == "ratelimited"`, return a distinct `"linear: rate limited"`-prefixed error (own detector, not `resp.StatusCode`-based — Linear's rate limit surfaces via the GraphQL error payload, not an HTTP status).
- Audit: confirm no error path interpolates the raw `Authorization` header value (Observability Plan / pitfalls.md §6).
- Files: `session/backlog_plugin_linear.go`

#### Story 1.2.2: `MapToBacklogItem` incl. priority mapping
**As a** backlog sync loop, **I want** Linear's native 0-4 priority mapped to backlog's 1-5 scale, **so that** imported items sort sensibly against GitHub/manually-created items.
**Acceptance Criteria**:
- Linear priority `1` (Urgent) maps to a backlog priority at least as high as GitHub's default priority 3, and priority `0` (No priority) maps to the backlog default.
  - *Given* an `ExternalItem` derived from a Linear issue with native priority `1` ("Urgent"), *When* `LinearPlugin.MapToBacklogItem(item, sourceID)` is called, *Then* the returned `BacklogItemData.Priority` is `5` (highest).
**Files**: `session/backlog_plugin_linear.go`

##### Task 1.2.2a: Priority mapping table + `MapToBacklogItem` (~5 min)
- Fixed table: Linear `0` (No priority) → backlog `3` (`DefaultBacklogPriority`), `1` (Urgent) → `5`, `2` (High) → `4`, `3` (Medium) → `3`, `4` (Low) → `2`. Mandatory mapping (not best-effort/opt-in) since Linear issues always carry a native priority value, per features.md §2.
- `MapToBacklogItem` truncates title (200)/description (2000) matching `GitHubIssuesPlugin`'s constants, sets `Status: string(BacklogStatusIdea)`, carries `SourceID`, `ExternalID` (Linear's `identifier`, e.g. `"ENG-123"`), `ExternalURL` (Linear's `url`), `Labels`.
- Files: `session/backlog_plugin_linear.go`

#### Story 1.2.3: Registration + tests
**As a** backlog administrator, **I want** `LinearPlugin` available in the default registry, **so that** I can configure it as an `ItemSource` from Settings.
**Acceptance Criteria**:
- `NewDefaultRegistry()` includes a plugin whose `PluginID()` returns `"linear"`.
  - *Given* `registry := session.NewDefaultRegistry()`, *When* `registry.Get("linear")` is called, *Then* it returns `(a *LinearPlugin, true)`.
**Files**: `session/backlog_plugin.go`

##### Task 1.2.3a: Register `NewLinearPlugin()` in `NewDefaultRegistry` (~2 min)
- Add `func NewLinearPlugin() *LinearPlugin { return &LinearPlugin{} }` to `backlog_plugin_linear.go`; add `r.Register(NewLinearPlugin())` to `NewDefaultRegistry()` (`session/backlog_plugin.go:79-84`).
- Files: `session/backlog_plugin.go`, `session/backlog_plugin_linear.go`

---

### Epic 1.3: JiraPlugin — Fetch & Mapping
**Goal**: `JiraPlugin` fetches JIRA issues incrementally via JQL search using `go-jira/v2/cloud`, with ADF descriptions converted to plaintext (AC1, AC3).

#### Story 1.3.1: `go.mod` dependency + config decode + JQL `Fetch`
**As a** backlog sync loop, **I want** `JiraPlugin.Fetch` to return new/updated JIRA issues since the last cursor, **so that** the polling sync loop doesn't refetch the whole project every tick.
**Acceptance Criteria**:
- Empty/missing JIRA credentials result in a disabled no-op fetch, not an error.
  - *Given* a `PluginConfig{Raw: '{"base_url":"https://acme.atlassian.net","project_key":"ENG"}'}` with no JIRA credentials stored for that base URL, *When* `JiraPlugin.Fetch(ctx, config, "")` is called, *Then* it returns `(nil, "", nil)`.
- Incremental fetch uses a JQL `updated >=` clause built from the cursor.
  - *Given* cursor `"2026-07-01 00:00"`, *When* `JiraPlugin.Fetch(ctx, config, "2026-07-01 00:00")` is called, *Then* the JQL query sent to `SearchV2JQL` contains `project = ENG AND updated >= "2026-07-01 00:00" ORDER BY updated ASC`.
**Files**: `session/backlog_plugin_jira.go` (new), `go.mod`, `go.sum`

##### Task 1.3.1a: Add `go-jira/v2` dependency (~2 min)
- `go get github.com/andygrunwald/go-jira/v2` (Cloud sub-package), pin latest v2.x tag.
- Files: `go.mod`, `go.sum`

##### Task 1.3.1b: `jiraPluginConfig` decode + keychain resolution (~5 min)
- `jiraPluginConfig{BaseURL, ProjectKey string}` (no email/token fields). `decodeJiraFetchConfig(config PluginConfig) (cfg jiraPluginConfig, disabled bool, err error)` — email+token resolved via `jira.GetKeychainTokenForHost(cfg.BaseURL)` first, `PluginConfig.Raw` encrypted-token fallback second. Missing credentials ⇒ `disabled=true`.
- Construct `cloud.Client` with `BasicAuthTransport{Username: email, APIToken: token}` per v2's Cloud auth scheme (stack.md).
- Files: `session/backlog_plugin_jira.go`

##### Task 1.3.1c: `Fetch` — JQL search + cursor advancement (~5 min)
- `client.Issue.SearchV2JQL(ctx, jql, opts)` with `jql = fmt.Sprintf("project = %s AND updated >= %q ORDER BY updated ASC", cfg.ProjectKey, cursor)` (empty cursor ⇒ no `AND updated >=` clause).
- Convert `[]cloud.Issue` → `[]ExternalItem` (`convertJiraIssues`), cursor = `max(cursor, latest Fields.Updated)`.
- Files: `session/backlog_plugin_jira.go`

##### Task 1.3.1d: Rate-limit + auth-failure detection (~5 min)
- Per pitfalls.md §4 (verify current header names — Unresolved Questions item 2): detect `429` and JIRA's points/burst headers, return a `"jira: rate limited"`-prefixed error, own predicate (not shared with GitHub's `fetchIssuesPage` check).
- JIRA Cloud auth failures surface as real HTTP 401 — confirm `isAuthFailure()` matches unmodified (pitfalls.md §3's "closer to GitHub's shape" claim) by asserting the wrapped error string contains `"401"`.
- Files: `session/backlog_plugin_jira.go`

#### Story 1.3.2: ADF→plaintext mapper + `MapToBacklogItem`
**As a** backlog sync loop, **I want** JIRA's ADF `description` field converted to plaintext, **so that** the backlog item shows readable text instead of a raw JSON blob.
**Acceptance Criteria**:
- A JIRA issue's ADF description renders as plain text, not JSON.
  - *Given* a JIRA issue with `Fields.Description` = `{type:"doc",version:1,content:[{type:"paragraph",content:[{type:"text",text:"Fix the login bug"}]}]}`, *When* `JiraPlugin.MapToBacklogItem(item, sourceID)` is called, *Then* `BacklogItemData.Description == "Fix the login bug"` (not the raw JSON).
**Files**: `session/backlog_plugin_jira.go`, `jira/adf.go` (new)

##### Task 1.3.2a: ADF→plaintext walker (~5 min)
- `jira.ADFToPlainText(doc json.RawMessage) string` — walks `content[].content[].text` nodes for `paragraph`/`heading`/`text`/`bulletList`/`orderedList` node types, joins with newlines; unknown node types skipped gracefully (no panic on unrecognized ADF extensions). Resolve Unresolved Questions item 3 first (check `go-jira/v2/cloud` for existing ADF types before hand-rolling the walker's input type).
- Files: `jira/adf.go`

##### Task 1.3.2b: `MapToBacklogItem` incl. priority + issue-key/URL split (~5 min)
- Priority: JIRA's native `Highest`/`High`/`Medium`/`Low`/`Lowest` → backlog 1-5 (`Highest`→5, `Lowest`→1), nil/absent priority field → `DefaultBacklogPriority` (3).
- `ExternalID` = JIRA issue key (`"PROJ-123"`, used in API paths); `ExternalURL` = `<base_url>/browse/PROJ-123` (constructed, not returned directly by the API) — mirrors GitHub's `Number`-vs-`HTMLURL` split (features.md §2).
- Description via `jira.ADFToPlainText`; title/description truncated to 200/2000 matching GitHub's constants.
- Files: `session/backlog_plugin_jira.go`

#### Story 1.3.3: Registration + tests
**As a** backlog administrator, **I want** `JiraPlugin` available in the default registry, **so that** I can configure it as an `ItemSource` from Settings.
**Acceptance Criteria**:
- `NewDefaultRegistry()` includes a plugin whose `PluginID()` returns `"jira"`.
  - *Given* `registry := session.NewDefaultRegistry()`, *When* `registry.Get("jira")` is called, *Then* it returns `(a *JiraPlugin, true)`.
**Files**: `session/backlog_plugin.go`

##### Task 1.3.3a: Register `NewJiraPlugin()` in `NewDefaultRegistry` (~2 min)
- Add `func NewJiraPlugin() *JiraPlugin { return &JiraPlugin{} }`; add `r.Register(NewJiraPlugin())` to `NewDefaultRegistry()`.
- Files: `session/backlog_plugin.go`, `session/backlog_plugin_jira.go`

---

## Phase 2: Forward-Sync on Completion

### Epic 2.1: Tracker-Specific Forward-Sync Methods
**Goal**: `LinearPlugin` and `JiraPlugin` each expose a state-update method + comment-post method returning the tracker's own post-write timestamp for the ADR-003 watermark (AC5).

#### Story 2.1.1: `LinearPlugin.UpdateIssueState` + `PostIssueComment`
**As a** backlog item transitioning to done, **I want** the linked Linear issue's workflow state updated and a comment posted, **so that** the Linear team sees the automated action (no-silent-action convention).
**Acceptance Criteria**:
- Updating state resolves the target `WorkflowState` by name and returns Linear's own `updatedAt`.
  - *Given* a Linear team whose workflow states include one named `"Done"` (`type: "completed"`), and `source.ForwardSyncCloseLabel == "Done"`, *When* `LinearPlugin.UpdateIssueState(ctx, config, "ENG-123", "Done")` is called, *Then* it issues an `issueUpdate` mutation with the resolved `stateId` and returns the mutation response's `issue.updatedAt` as the watermark `time.Time` (not local wall-clock).
- No matching workflow state is a normal sync failure, not a panic/bug.
  - *Given* `source.ForwardSyncCloseLabel == "Shipped"` but no team `WorkflowState` is named `"Shipped"`, *When* `UpdateIssueState` is called, *Then* it returns a non-nil `error` describing "no workflow state named %q found" (caller records this via `RecordSourceSyncFailure`, does not crash).
**Files**: `session/backlog_plugin_linear.go`

##### Task 2.1.1a: `workflowStates` query + name-resolution + caching (~5 min)
- `workflowStates(filter: {team:{id:{eq:$teamId}}}) { nodes { id name type } }` query, resolves `ForwardSyncCloseLabel` (case-insensitive match) to a `stateId`. No cross-call caching in v1 (one extra GraphQL call per forward-sync event is acceptable — matches GitHub's existing "no is-it-already-closed pre-check" simplicity bias, features.md §Forward-sync-on-completion "Known deferred gap").
- Files: `session/backlog_plugin_linear.go`

##### Task 2.1.1b: `issueUpdate` mutation + watermark extraction (~5 min)
- `issueUpdate(id: $id, input: {stateId: $stateId}) { success issue { updatedAt } }` — request `issue { updatedAt }` in the selection set specifically so the response gives the watermark in the same round-trip (pitfalls.md §1). Signature: `UpdateIssueState(ctx context.Context, config session.PluginConfig, externalID string, targetLabel string) (time.Time, error)`.
- Files: `session/backlog_plugin_linear.go`

##### Task 2.1.1c: `PostIssueComment` via `commentCreate` mutation (~4 min)
- `commentCreate(input: {issueId: $id, body: $body})` — plain Markdown body (no ADF needed for Linear). Failure is logged/returned as error but treated as best-effort by the caller (subscriber), matching GitHub's `PostIssueComment` contract.
- Files: `session/backlog_plugin_linear.go`

#### Story 2.1.2: `JiraPlugin.TransitionIssue` + `PostIssueComment` (ADF)
**As a** backlog item transitioning to done, **I want** the linked JIRA issue transitioned to its "Done"-equivalent status and a comment posted, **so that** the JIRA team sees the automated action.
**Acceptance Criteria**:
- Transitioning resolves the available transition by name and performs GET→match→POST→GET-for-watermark (pitfalls.md §1/§2's 3-call dance).
  - *Given* `GET /issue/PROJ-456/transitions` returns transitions `[{id:"31",name:"Done"},{id:"21",name:"In Progress"}]` and `source.ForwardSyncCloseLabel == "Done"`, *When* `JiraPlugin.TransitionIssue(ctx, config, "PROJ-456", "Done")` is called, *Then* it `POST`s `{"transition":{"id":"31"}}`, then issues a follow-up `GET /issue/PROJ-456?fields=updated` to obtain the watermark (since the transition POST itself returns 204 No Content).
- An already-transitioned issue (no matching transition from current state) is a normal, expected failure.
  - *Given* the issue is already in a terminal "Done" status with no `"Done"`-named transition available from that state, *When* `TransitionIssue` is called, *Then* it returns a non-nil `error`, and the caller records it via `RecordSourceSyncFailure` rather than treating it as a bug (pitfalls.md §2/features.md's "already closed" edge case).
**Files**: `session/backlog_plugin_jira.go`

##### Task 2.1.2a: `GetTransitions` + name-match (~4 min)
- `client.Issue.GetTransitions(ctx, externalID)`, case-insensitive name match against `targetLabel`. No match ⇒ returns a descriptive error (not a panic).
- Files: `session/backlog_plugin_jira.go`

##### Task 2.1.2b: `DoTransition` + follow-up `GET` for watermark (~5 min)
- `client.Issue.DoTransition(ctx, externalID, transitionID)` (204 No Content on success, no body — pitfalls.md §1). On success, follow-up `client.Issue.Get(ctx, externalID, &cloud.GetQueryOptions{Fields: "updated"})` to obtain the watermark. Signature: `TransitionIssue(ctx context.Context, config session.PluginConfig, externalID string, targetLabel string) (time.Time, error)`.
- TOCTOU note (pitfalls.md §2): a POST 400 "transition id not valid" between GET and POST is treated as an ordinary failure return, not retried automatically in v1.
- Files: `session/backlog_plugin_jira.go`

##### Task 2.1.2c: `PostIssueComment` via ADF wrapper (~4 min)
- Wrap plaintext comment body into minimal ADF (`jira.PlainTextToADF(text string) json.RawMessage` in `jira/adf.go`) before calling `client.Issue.AddComment(ctx, externalID, &cloud.Comment{Body: adfDoc})` (pitfalls.md §7 — resolve Unresolved Questions item 3 for whether `go-jira` already provides this).
- Files: `session/backlog_plugin_jira.go`, `jira/adf.go`

---

### Epic 2.2: Generalized Forward-Sync Subscriber (ADR-002)
**Goal**: One subscriber dispatches forward-sync for all three trackers via type-switch, replacing per-tracker triplication.

#### Story 2.2.1: Refactor subscriber to dispatch on two capability interfaces
**As a** backlog item transitioning to done, **I want** forward-sync to fire regardless of which tracker the item came from, **so that** GitHub, Linear, and JIRA all get the same completion-time write-back behavior.
**Acceptance Criteria**:
- A `LinearPlugin`-backed item triggers `UpdateIssueState` + `PostIssueComment` + watermark write, same as GitHub's `CloseIssue` path does today.
  - *Given* a `BacklogItemData` with `SourceID` pointing at an `ItemSource{PluginID: "linear", ForwardSyncEnabled: true, ForwardSyncCloseLabel: "Done"}`, *When* the item transitions to `BacklogStatusDone` and the `EventBus` publishes `BacklogChangeStatusTransition`, *Then* `StartBacklogForwardSyncSubscriber`'s handler calls `LinearPlugin.UpdateIssueState`, then `PostIssueComment`, then persists `GitHubSyncedIssueUpdatedAt` from the mutation's returned timestamp.
- A plugin implementing neither capability interface (e.g. `GitHubPRsPlugin`) is skipped with a log line, not an error.
  - *Given* `source.PluginID == "github_prs"`, *When* the dispatch handler runs, *Then* it logs `"plugin does not support forward sync, skip"` and returns without calling `RecordSourceSyncFailure`.
**Files**: `server/services/backlog_github_forward_sync.go` → renamed `server/services/backlog_forward_sync.go`, `server/services/backlog_github_forward_sync_test.go` → renamed/extended, `server/server.go`

##### Task 2.2.1a: Rename file, add `externalIssueStateUpdater` interface (~4 min)
- `git mv server/services/backlog_github_forward_sync.go server/services/backlog_forward_sync.go`. Add `externalIssueStateUpdater{UpdateIssueState(...) (time.Time, error); PostIssueComment(...) error}` alongside the existing `externalIssueCloser`. Update file header comment to describe the generalized scope.
- Files: `server/services/backlog_forward_sync.go`

##### Task 2.2.1b: Type-switch dispatch in the handler (~5 min)
- Rename `StartBacklogGitHubForwardSyncSubscriber` → `StartBacklogForwardSyncSubscriber`, `handleForwardSyncClose` → `handleForwardSync`. Replace the single `closer, ok := plugin.(externalIssueCloser)` check with a type-switch trying `externalIssueCloser` then `externalIssueStateUpdater`, each calling its own close/update method, then falling through to shared `PostIssueComment` + watermark-write logic (ADR-002's code sketch).
- Files: `server/services/backlog_forward_sync.go`

##### Task 2.2.1c: Update `server/server.go` wiring (~2 min)
- Replace the `StartBacklogGitHubForwardSyncSubscriber(...)` call (`server/server.go:647`) with `StartBacklogForwardSyncSubscriber(...)`, update the surrounding comment from "GitHub forward-sync subscriber" to tracker-neutral wording.
- Files: `server/server.go`

##### Task 2.2.1d: `TriggeredByLinearSync`/`TriggeredByJiraSync` constants (~2 min)
- Add alongside `TriggeredByGitHubSync` (`session/backlog.go:96`), threaded into `TransitionBacklogItemStatus` calls where the sync loop (not forward-sync) triggers a status change — confirm actual call sites via `grep TriggeredByGitHubSync session/backlog_sync.go` before wiring, since forward-sync itself doesn't call `TransitionBacklogItemStatus` (it only updates the external tracker, not the local item's status).
- Files: `session/backlog.go`

---

## Phase 3: One-off MCP Import

### Epic 3.1: `import_linear_issue` MCP tool
**Goal**: A single Linear issue can be imported as a backlog item without a configured polling `ItemSource` (AC4).

#### Story 3.1.1: Tool handler + registration
**As an** operator, **I want** to paste a Linear issue URL or identifier into an MCP tool call, **so that** it becomes a backlog item immediately, mirroring `import_github_issue`.
**Acceptance Criteria**:
- A Linear issue URL creates a backlog item with title/description/labels populated and `ExternalID`/`ExternalURL` set (fixing the dedup-visibility gap GitHub's own tool has, per features.md §"Unstated needs" #4 — `SourceID` intentionally left unset since resolving which `ItemSource` a one-off import belongs to is out of scope, see Unresolved Questions).
  - *Given* an MCP call `import_linear_issue({issue_ref: "https://linear.app/acme/issue/ENG-123/fix-login-bug"})` with a valid caller session and a working Linear API key in the keychain, *When* the handler runs, *Then* it creates a `BacklogItemData` with `Title` from the fetched issue, `ExternalID: "ENG-123"`, `ExternalURL: "https://linear.app/acme/issue/ENG-123/fix-login-bug"`, and `Notes: "Imported from https://linear.app/acme/issue/ENG-123/fix-login-bug"`.
**Files**: `server/mcp/tools_backlog.go`

##### Task 3.1.1a: `importLinearIssue` handler — parse + fetch (~5 min)
- Accept `issue_ref` (full Linear issue URL or bare identifier like `"ENG-123"`); parse identifier out of a URL if given (simple path-segment split, no need for a full URL-parsing library — Linear URLs are `linear.app/<workspace>/issue/<identifier>/<slug>`). Fetch the single issue via a new `LinearPlugin`-internal or package-level `linear.GetIssue(ctx, token, identifier)` helper reusing `doLinearGraphQL`.
- Files: `server/mcp/tools_backlog.go`

##### Task 3.1.1b: `storage.CreateBacklogItem` + auto-triage wiring (~4 min)
- Same shape as `importGitHubIssue` (`server/mcp/tools_backlog.go:1131-1170`): `Title`/`Description`/`Priority: DefaultBacklogPriority`/`Status: BacklogStatusIdea`/`RepoPath`/`Notes`, plus new `ExternalID`/`ExternalURL` fields (the fix noted above). `h.backlogSvc.MaybeTriggerTriage(...)` call, same `skip_triage` contract.
- Files: `server/mcp/tools_backlog.go`

##### Task 3.1.1c: Register `import_linear_issue` MCP tool (~3 min)
- `mcpgo.NewTool("import_linear_issue", ...)` with `issue_ref` (required string), `repo_path` (optional), `skip_triage` (optional bool) — mirrors `import_github_issue`'s registration block (`server/mcp/tools_backlog.go:1781-1794`).
- Files: `server/mcp/tools_backlog.go`

### Epic 3.2: `import_jira_issue` MCP tool
**Goal**: A single JIRA issue can be imported as a backlog item without a configured polling `ItemSource` (AC4).

#### Story 3.2.1: Tool handler + registration
**As an** operator, **I want** to paste a JIRA issue key or browse URL into an MCP tool call, **so that** it becomes a backlog item immediately, mirroring `import_github_issue`.
**Acceptance Criteria**:
- A JIRA issue key creates a backlog item with title/description (ADF-converted)/`ExternalID`/`ExternalURL` set.
  - *Given* an MCP call `import_jira_issue({issue_key: "PROJ-456", base_url: "https://acme.atlassian.net"})` with valid JIRA credentials in the keychain for that base URL, *When* the handler runs, *Then* it creates a `BacklogItemData` with `Title` from the fetched issue, `Description` as plaintext (via `jira.ADFToPlainText`), `ExternalID: "PROJ-456"`, `ExternalURL: "https://acme.atlassian.net/browse/PROJ-456"`.
**Files**: `server/mcp/tools_backlog.go`

##### Task 3.2.1a: `importJiraIssue` handler — parse + fetch (~5 min)
- Accept `issue_key` (bare key like `"PROJ-456"`, or a full browse URL — parse the key out of `/browse/<key>` if given a URL) + `base_url` (required, since a bare key alone can't identify which JIRA site). Fetch via `cloud.Client.Issue.Get(ctx, key, nil)`.
- Files: `server/mcp/tools_backlog.go`

##### Task 3.2.1b: `storage.CreateBacklogItem` + auto-triage wiring (~4 min)
- Same shape as Task 3.1.1b, using `jira.ADFToPlainText` for `Description`.
- Files: `server/mcp/tools_backlog.go`

##### Task 3.2.1c: Register `import_jira_issue` MCP tool (~3 min)
- `mcpgo.NewTool("import_jira_issue", ...)` with `issue_key` (required), `base_url` (required), `repo_path` (optional), `skip_triage` (optional).
- Files: `server/mcp/tools_backlog.go`

### Epic 3.3: Feature registry entries
**Goal**: New MCP surfaces are tracked per `.claude/rules/feature-registry.md` (AC9).

#### Story 3.3.1: Registry files for both new tools
**As a** repo maintainer, **I want** the new MCP tools tracked in the feature registry, **so that** coverage-gap tooling sees them.
**Acceptance Criteria**:
- `docs/registry/features/backend/` gains entries for both tools.
  - *Given* `import_linear_issue` and `import_jira_issue` are now registered MCP tools with a `+api:` marker each, *When* `make registry-generate` runs, *Then* `docs/registry/features/backend/ImportLinearIssue.json` and `ImportJiraIssue.json` exist with `markerFound: true`.
**Files**: `docs/registry/features/backend/ImportLinearIssue.json` (new), `docs/registry/features/backend/ImportJiraIssue.json` (new)

##### Task 3.3.1a: Add `// +api:` markers + run `make registry-generate` (~3 min)
- Add `// +api: import_linear_issue` / `// +api: import_jira_issue` comments above each handler function (matching `importGitHubIssue`'s marker convention if present, or `ImportGitHubIssue`'s `// +api: ImportGitHubIssue` at `backlog_service_sync.go:221`). Run `make registry-generate`, commit the generated per-feature JSON files.
- Files: `server/mcp/tools_backlog.go`, `docs/registry/features/backend/ImportLinearIssue.json`, `docs/registry/features/backend/ImportJiraIssue.json`

---

## Phase 4: Frontend — Backlog Item Source Awareness

### Epic 4.1: Backend denormalization (`plugin_id`) + source filter proto fields
**Goal**: The frontend can badge/filter by tracker without a client-side join (supports AC7).

#### Story 4.1.1: `BacklogItem.plugin_id` denormalization
**As a** frontend component, **I want** each `BacklogItem` to carry its source tracker's `plugin_id`, **so that** I can render the right badge/icon without fetching the sources list separately.
**Acceptance Criteria**:
- A backlog item sourced from Linear carries `plugin_id: "linear"` on the wire.
  - *Given* a `BacklogItemData` whose `SourceID` points at an `ItemSource{PluginID: "linear"}`, *When* `GetBacklogItem` or `ListBacklogItems` is called, *Then* the returned `BacklogItem` proto has `plugin_id == "linear"`.
**Files**: `proto/session/v1/backlog.proto`, `session/repository.go`, `session/ent_repository_backlog.go`, `server/services/backlog_service.go`

##### Task 4.1.1a: Add `plugin_id` field 33 to `BacklogItem` proto (~2 min)
- `string plugin_id = 33;` with a doc comment explaining it's denormalized from the item's `ItemSource` edge, empty for locally-created items. Run `make proto-gen`.
- Files: `proto/session/v1/backlog.proto`

##### Task 4.1.1b: Populate `SourcePluginID` in `backlogItemToData` (~3 min)
- Add `SourcePluginID string` to `BacklogItemData` (`session/repository.go:356` struct); set `data.SourcePluginID = item.Edges.Source.PluginID` alongside the existing `data.SourceID = item.Edges.Source.ID.String()` (`session/ent_repository_backlog.go:212-214`).
- Files: `session/repository.go`, `session/ent_repository_backlog.go`

##### Task 4.1.1c: Map into `backlogItemToProto` (~2 min)
- Add `PluginId: item.SourcePluginID` to the proto struct literal in `backlogItemToProto` (`server/services/backlog_service.go:659`).
- Files: `server/services/backlog_service.go`

#### Story 4.1.2: `source_plugin_filter` on list/watch RPCs
**As a** frontend filter control, **I want** to request only items from a specific tracker, **so that** the backlog board can show "Linear only" without client-side filtering.
**Acceptance Criteria**:
- Filtering by `["linear"]` excludes GitHub- and JIRA-sourced items.
  - *Given* a `WatchBacklogItemsRequest{SourcePluginFilter: []string{"linear"}}`, *When* the initial snapshot is computed, *Then* only items whose `plugin_id == "linear"` are included (mirrors `statusFilter`'s existing `slices.Contains` check at `server/services/backlog_service_events.go:194`).
**Files**: `proto/session/v1/backlog.proto`, `session/repository.go`, `session/ent_repository_backlog.go`, `server/services/backlog_service_query.go`, `server/services/backlog_service_events.go`

##### Task 4.1.2a: Add `source_plugin_filter` to both request protos (~3 min)
- `repeated string source_plugin_filter = 6;` on `ListBacklogItemsRequest` (next after `include_archived = 5`); `repeated string source_plugin_filter = 4;` on `WatchBacklogItemsRequest` (next after `after_seq = 3`). Run `make proto-gen`.
- Files: `proto/session/v1/backlog.proto`

##### Task 4.1.2b: `BacklogItemFilter.SourcePluginIDs` + `ListBacklogItems` wiring (~4 min)
- Add `SourcePluginIDs []string` to `BacklogItemFilter` (`session/repository.go:508`); apply as a `backlogitem.HasSourceWith(itemsource.PluginIDIn(...))` predicate in `ListBacklogItems` (`session/ent_repository_backlog.go:412`) when non-empty.
- Files: `session/repository.go`, `session/ent_repository_backlog.go`

##### Task 4.1.2c: `WatchBacklogItems` snapshot + live-event filter (~4 min)
- Add a `sourcePluginFilter` check alongside `statusFilter`/`categoryFilter` in `server/services/backlog_service_events.go:194-197`, using the new `plugin_id` field on the proto item.
- Files: `server/services/backlog_service_events.go`, `server/services/backlog_service_query.go`

---

### Epic 4.2: `PLUGIN_SCHEMAS` entries + JIRA compound credential settings
**Goal**: Linear and JIRA sources can be added from Settings using the existing schema-driven form (AC6, AC7).

#### Story 4.2.1: Linear and JIRA entries in `backlogSourceSchemas.ts`
**As a** backlog administrator, **I want** "Linear" and "JIRA" as addable source types in Settings, **so that** I can configure polling without editing JSON by hand.
**Acceptance Criteria**:
- The Add-a-Source form offers Linear (API-key-only, `credentialsManagedExternally`) and JIRA (base URL + email + token, `credentialsManagedExternally`) options.
  - *Given* `PLUGIN_SCHEMAS` includes an entry `{id: "linear", label: "Linear", fields: [{key:"team_id",...}], requiresToken: true, credentialsManagedExternally: true}`, *When* the Settings form renders, *Then* selecting "Linear" shows a Team ID field and a link to the Linear credentials page instead of a raw token input.
**Files**: `web-app/src/components/settings/backlogSourceSchemas.ts`

##### Task 4.2.1a: Add `linear` schema entry (~3 min)
- `{id: "linear", label: "Linear", fields: [{key: "team_id", label: "Team ID", placeholder: "Team ID (e.g. ENG)"}], requiresToken: true, tokenLabel: "Linear API key", credentialsManagedExternally: true}`.
- Files: `web-app/src/components/settings/backlogSourceSchemas.ts`

##### Task 4.2.1b: Add `jira` schema entry (~3 min)
- `{id: "jira", label: "JIRA", fields: [{key: "base_url", label: "Base URL", placeholder: "https://acme.atlassian.net"}, {key: "project_key", label: "Project Key", placeholder: "PROJ"}], requiresToken: true, tokenLabel: "JIRA API token", credentialsManagedExternally: true}`.
- Files: `web-app/src/components/settings/backlogSourceSchemas.ts`

#### Story 4.2.2: Dedicated JIRA credentials settings sub-section
**As a** backlog administrator, **I want** a small settings page to enter my JIRA email + API token, **so that** the compound secret is stored via keychain, not the generic single-token form field.
**Acceptance Criteria**:
- Submitting email + token on the JIRA credentials page results in a stored keychain entry, not a `PluginConfig.Raw` field.
  - *Given* the JIRA credentials sub-section form with `email="alice@acme.com"`, `token="tok_abc"`, `base_url="https://acme.atlassian.net"`, *When* submitted, *Then* it calls a new RPC (`SetJiraCredentials` or similar) that ultimately invokes `jira.SetKeychainTokenForHost`, and no plaintext secret appears in the created `ItemSource.Config` JSON.
**Files**: `web-app/src/components/settings/JiraCredentialsSettings.tsx` (new), `proto/session/v1/backlog.proto`, `server/services/backlog_service_sources.go` (or nearest existing sources-RPC file)

##### Task 4.2.2a: New RPC to write JIRA keychain credentials (~5 min)
- `SetJiraCredentialsRequest{base_url, email, api_token}` / `SetJiraCredentialsResponse{}` — handler calls `jira.SetKeychainTokenForHost`. Mirrors whatever RPC GitHub's "Settings > GitHub Accounts" page already uses for its keychain writes (locate via `grep -n "AddGitHubAccountWithToken" server/services/*.go` at implementation time and follow the same handler shape).
- Files: `proto/session/v1/backlog.proto`, `server/services/backlog_service_sources.go` (exact file TBD at implementation time — locate GitHub's account-token RPC handler first)

##### Task 4.2.2b: `JiraCredentialsSettings.tsx` form component (~5 min)
- Three fields (base URL, email, API token), submit button, success/error state — small, single-purpose settings sub-section (not a generic schema-driven form), linked from the JIRA `PluginSchema` entry's `credentialsManagedExternally` link-out.
- Files: `web-app/src/components/settings/JiraCredentialsSettings.tsx`

##### Task 4.2.2c: Wire the settings route/link (~3 min)
- Add a route entry (mirrors GitHub Accounts' settings route) and update the JIRA `PluginSchema`/`BacklogSourcesSettings.tsx` link target.
- Files: `web-app/src/lib/routes.ts`, `web-app/src/components/settings/BacklogSourcesSettings.tsx`

##### Task 4.2.2d: Feature registry entries for the new RPC + settings page (~3 min)
- `SetJiraCredentials` is a new RPC surface (`.claude/rules/feature-registry.md`) — add `// +api: SetJiraCredentials` marker (Task 4.2.2a) and run `make registry-generate` to produce `docs/registry/features/backend/SetJiraCredentials.json`. Add a frontend entry for the new settings page with `// +feature: settings:jira-credentials` in `JiraCredentialsSettings.tsx`'s first 10 lines, then `docs/registry/features/frontend/jira-credentials-settings.json`.
- Files: `server/services/backlog_service_sources.go` (marker), `web-app/src/components/settings/JiraCredentialsSettings.tsx` (marker), `docs/registry/features/backend/SetJiraCredentials.json` (new), `docs/registry/features/frontend/jira-credentials-settings.json` (new)

---

### Epic 4.3: Generalize backlog item provenance badges
**Goal**: `BacklogItemCard.tsx` and `SourceSection.tsx` render tracker-appropriate label/aria-text instead of hardcoded "GitHub" (AC7).

#### Story 4.3.1: Source badge config lookup
**As a** backlog user, **I want** the provenance badge to say "Linear" or "JIRA" (not "GitHub") for non-GitHub items, **so that** the badge is accurate.
**Acceptance Criteria**:
- A Linear-sourced item's badge reads "Imported from Linear issue ENG-123", not "GitHub issue #ENG-123".
  - *Given* a `BacklogItem` with `pluginId: "linear"`, `externalId: "ENG-123"`, `externalUrl: "https://linear.app/..."`, *When* `BacklogItemCard` renders its provenance badge, *Then* the rendered `aria-label` is `"Imported from Linear issue ENG-123"` and the visible text is `"ENG-123"` (no `#` prefix — matches ux.md §3's "not just the ticket ID alone" + per-tracker ID-format guidance).
**Files**: `web-app/src/lib/backlog/sourceBadge.ts` (new), `web-app/src/components/backlog/BacklogItemCard.tsx`, `web-app/src/components/backlog/detail/SourceSection.tsx`

##### Task 4.3.1a: `sourceBadge.ts` config lookup (~4 min)
- `SOURCE_BADGE_CONFIG: Record<string, {label: string; idPrefix: string; openLabel: string}>` — `github_issues: {label:"GitHub", idPrefix:"#", openLabel:"Open on GitHub"}`, `linear: {label:"Linear", idPrefix:"", openLabel:"Open in Linear"}`, `jira: {label:"JIRA", idPrefix:"", openLabel:"Open in JIRA"}`. Fallback for unknown/empty `pluginId` reuses today's literal "GitHub" strings (backward-compat for any pre-existing item without `plugin_id` populated, e.g. items created before this migration or via a code path that doesn't set it).
- Files: `web-app/src/lib/backlog/sourceBadge.ts`

##### Task 4.3.1b: `BacklogItemCard.tsx` badge generalization (~4 min)
- Replace the hardcoded `aria-label={`Imported from GitHub issue #${item.externalId}`}` / `#{item.externalId}` (`BacklogItemCard.tsx:197-209`) with `SOURCE_BADGE_CONFIG[item.pluginId ?? "github_issues"]`-driven label/prefix. Icon stays `CircleDot` for all three (ux.md §"New IssueSourceBadge component" — no brand icons available, stay token-neutral per `.claude/rules/css-architecture.md`).
- Files: `web-app/src/components/backlog/BacklogItemCard.tsx`

##### Task 4.3.1c: `SourceSection.tsx` badge generalization (~4 min)
- Same substitution for `title="Open on GitHub"` / `Issue #<id>` (`SourceSection.tsx`), plus threading `pluginId` as a new prop from `BacklogItemDetail.tsx` (the parent that currently only passes `externalUrl`/`externalId`/`labels`).
- Files: `web-app/src/components/backlog/detail/SourceSection.tsx`, `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

### Epic 4.4: Backlog source filter UI
**Goal**: Users can filter the backlog view by tracker (AC7).

#### Story 4.4.1: "Filter by source" control on the backlog board
**As a** backlog user, **I want** to filter the board to only Linear (or JIRA, or GitHub) items, **so that** I can focus on one tracker's work.
**Acceptance Criteria**:
- Selecting "Linear" in the source filter hides GitHub/JIRA items from the board.
  - *Given* the backlog board is showing items from all three trackers, *When* the user selects "Linear" from a "Filter by issue source" control, *Then* `useWatchBacklogItems` is called with `sourcePluginFilter: ["linear"]` and only Linear-sourced cards render.
**Files**: `web-app/src/components/backlog/BacklogBoard.tsx`, `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.4.1a: Thread `sourcePluginFilter` through `useWatchBacklogItems` (~5 min)
- Add `sourcePluginFilter?: string[]` to the hook's filter params (`web-app/src/lib/hooks/useWatchBacklogItems.ts:75-76` pattern), pass through to the `WatchBacklogItemsRequest` (`:199,356`) and into the dependency-array memoization key (`:135-136` pattern).
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 4.4.1b: "Filter by issue source" `<select>` in `BacklogBoard.tsx` (~5 min)
- New sibling control (no existing filter select group on the board today — confirmed by architecture.md §7/ux.md §2 — this is the first filter control on this page, so it introduces a small `filterControls`-style wrapper rather than joining an existing group). Options: `All Sources` sentinel + `GitHub` / `Linear` / `JIRA` (derived from which plugin IDs are actually present in current data, mirroring the Tag-filter-derives-from-live-data precedent ux.md §2 cites from `SessionList.tsx:1075`). `aria-label="Filter by issue source"`.
- Files: `web-app/src/components/backlog/BacklogBoard.tsx`

---

### Epic 4.5: `BacklogSourcesSettings.tsx` genericization
**Goal**: The per-source management UI stops hardcoding "GitHub" once Linear/JIRA rows can exist (AC6, AC7).

#### Story 4.5.1: Genericize hardcoded "GitHub" copy
**As a** backlog administrator, **I want** the sync-settings copy to say "Linear"/"JIRA" for those sources, **so that** a JIRA source row doesn't confusingly say "Sync with GitHub".
**Acceptance Criteria**:
- A Linear source row's sync-settings copy says "Sync with Linear", not "Sync with GitHub".
  - *Given* an `ItemSource{PluginID: "linear", DisplayName: "My Linear Team"}` row is rendered, *When* the "Sync with..." label is shown, *Then* it reads `"Sync with Linear"` (derived from `PLUGIN_SCHEMAS.find(s => s.id === source.pluginId)?.label`), not the literal string `"GitHub"`.
**Files**: `web-app/src/components/settings/BacklogSourcesSettings.tsx`

##### Task 4.5.1a: Replace 4 hardcoded "GitHub" strings with schema-label lookups (~5 min)
- `"Sync with GitHub"` (line 352), `"Close GitHub issues when I finish here"` (365), `"reflecting GitHub status back"` aria-label (390), `"Reflect GitHub status back here"` (392) → template-literal using the matching `PluginSchema.label` for `source.pluginId`.
- Files: `web-app/src/components/settings/BacklogSourcesSettings.tsx`

#### Story 4.5.2: Verify `isAuthFailure()` against Linear/JIRA error shapes
**As a** backlog administrator, **I want** the "Sync failing — check credentials" banner to appear for a revoked Linear/JIRA credential, **so that** I notice without expanding history (matches AC6's spirit — credentials are discoverably broken, not silently failing).
**Acceptance Criteria**:
- A Linear auth failure (engineered per Story 1.2.1d to contain `"401"`) triggers the banner.
  - *Given* `historyBySource["linear-source-id"].events[0].errorMessage === "linear: authentication failed (401): invalid API key"`, *When* `isAuthFailure(errorMessage)` is called, *Then* it returns `true` and the row-level alert banner renders.
**Files**: `web-app/src/components/settings/BacklogSourcesSettings.tsx`, `web-app/src/components/settings/BacklogSourcesSettings.test.tsx`

##### Task 4.5.2a: Add Linear/JIRA auth-failure test cases (~4 min)
- New `isAuthFailure` test cases asserting the exact error strings Story 1.2.1d/1.3.1d's backend error-wrapping produces are matched (or, if a mismatch is found, extend `isAuthFailure`'s pattern list — this task is the verification step architecture.md §"Frontend credential-field shape" flagged as unconfirmed).
- Files: `web-app/src/components/settings/BacklogSourcesSettings.test.tsx`

---

## Phase 5: Testing & Registry Wrap-up

### Epic 5.1: Plugin unit tests
**Goal**: `LinearPlugin`/`JiraPlugin` have test coverage matching `backlog_plugin_github_test.go`'s pattern (AC8).

#### Story 5.1.1: `session/backlog_plugin_linear_test.go`
**As a** maintainer, **I want** `LinearPlugin` covered the same way `GitHubIssuesPlugin` is, **so that** regressions are caught the same way.
**Acceptance Criteria**:
- Empty-token, missing-config, priority-mapping, cursor-advancement, and auth/rate-limit error-string cases are all covered.
  - *Given* an `httptest.Server` stubbing Linear's GraphQL endpoint with a `{"errors":[{"message":"...","extensions":{"type":"authentication_error"}}]}` response, *When* `TestLinearPlugin_Fetch_should_ReturnAuthError_When_KeyRevoked` runs, *Then* the returned error's `.Error()` string contains `"401"`.
**Files**: `session/backlog_plugin_linear_test.go` (new)

##### Task 5.1.1a: GraphQL-envelope test fixtures + `httptest.Server` helper (~5 min)
- `withLinearTestServer(t *testing.T, handler http.HandlerFunc)` swapping `linearGraphQLURL`, restored via `t.Cleanup`, non-parallel caveat comment carried forward from `withGitHubTestServer` (pitfalls.md §8).
- Files: `session/backlog_plugin_linear_test.go`

##### Task 5.1.1b: Fetch/mapping/priority/cursor test cases (~5 min)
- Empty-token no-op, missing-team-id error, priority table (0-4 → 1-5), cursor advances to latest `updatedAt`, `MapToBacklogItem` truncation.
- Files: `session/backlog_plugin_linear_test.go`

##### Task 5.1.1c: Auth-failure and rate-limit error-string test cases (~4 min)
- Asserts Task 1.2.1d's error-wrapping produces `"401"`-containing / `"rate limited"`-containing strings.
- Files: `session/backlog_plugin_linear_test.go`

#### Story 5.1.2: `session/backlog_plugin_jira_test.go`
**As a** maintainer, **I want** `JiraPlugin` covered the same way `GitHubIssuesPlugin` is, **so that** regressions are caught the same way.
**Acceptance Criteria**:
- Empty-credentials, missing-config, ADF-conversion, priority-mapping, and 2-call-transition-dance cases are covered.
  - *Given* an `httptest.Server` returning `GET /issue/PROJ-1/transitions → [{id:"5",name:"Done"}]` then `POST /issue/PROJ-1/transitions → 204`, *When* `TestJiraPlugin_TransitionIssue_should_PostResolvedID_When_NameMatches` runs, *Then* the test asserts the POST body was `{"transition":{"id":"5"}}`.
**Files**: `session/backlog_plugin_jira_test.go` (new), `jira/adf_test.go` (new)

##### Task 5.1.2a: Config-carries-base-URL test setup, no package var needed (~3 min)
- Per pitfalls.md §8: JIRA tests point `jiraPluginConfig.BaseURL` directly at each test's `httptest.Server` URL — no shared package-level override var, parallel-safe for free (`t.Parallel()` usable, unlike the Linear/GitHub pattern).
- Files: `session/backlog_plugin_jira_test.go`

##### Task 5.1.2b: Fetch/mapping/priority test cases (~5 min)
- Empty-credentials no-op, missing-project-key error, priority table (Highest→5...Lowest→1, nil→3), issue-key-vs-browse-URL split.
- Files: `session/backlog_plugin_jira_test.go`

##### Task 5.1.2c: 2-call transition dance + already-transitioned edge case (~5 min)
- Happy path (GET→match→POST 204→GET watermark), no-matching-transition error case, TOCTOU 400-on-POST case treated as ordinary failure.
- Files: `session/backlog_plugin_jira_test.go`

##### Task 5.1.2d: ADF walker unit tests (~4 min)
- `ADFToPlainText` on nested paragraphs/lists/unknown-node-types; `PlainTextToADF` round-trips a simple string into the expected minimal doc shape.
- Files: `jira/adf_test.go`

### Epic 5.2: Forward-sync subscriber tests
**Goal**: The generalized subscriber (Epic 2.2) is covered for all three dispatch branches (AC5).

#### Story 5.2.1: `backlog_forward_sync_test.go` dispatch coverage
**As a** maintainer, **I want** the generalized subscriber's type-switch covered for closer, state-updater, and neither-interface cases, **so that** ADR-002's consolidation doesn't silently regress GitHub's existing behavior.
**Acceptance Criteria**:
- All three branches (GitHub close, Linear/JIRA state-update, no-op) are exercised against one subscriber entry point.
  - *Given* three `BacklogItemData` fixtures backed by `github_issues`, `linear`, and `github_prs`-sourced items respectively, *When* each transitions to done, *Then* the first two produce a watermark write via their respective interface method and the third logs a no-op skip with zero `RecordSourceSyncFailure` calls.
**Files**: `server/services/backlog_forward_sync_test.go` (renamed/extended from `backlog_github_forward_sync_test.go`)

##### Task 5.2.1a: Rename test file, add Linear/JIRA fake-plugin fixtures (~4 min)
- `git mv server/services/backlog_github_forward_sync_test.go server/services/backlog_forward_sync_test.go`. Add minimal fake types implementing `externalIssueStateUpdater` for direct dispatch-logic testing without a real Linear/JIRA HTTP round-trip (unit-level, not integration).
- Files: `server/services/backlog_forward_sync_test.go`

##### Task 5.2.1b: Dispatch-branch test cases (~5 min)
- Three cases per Acceptance Criteria above, reusing the existing `TestForwardSyncSubscriber_NoOpWhenPluginDoesNotImplementCloser`-style structure extended to also check the new `externalIssueStateUpdater` branch.
- Files: `server/services/backlog_forward_sync_test.go`

### Epic 5.3: E2E test for the new frontend surface
**Goal**: At least one new user-facing feature has Playwright coverage per `.claude/rules/feature-registry.md`.

#### Story 5.3.1: Backlog source badge + filter e2e spec
**As a** repo maintainer, **I want** an e2e test proving the source badge/filter actually renders in a real browser, **so that** the feature registry's "every new user-facing feature must have at least one Playwright e2e test" rule is satisfied.
**Acceptance Criteria**:
- A Linear-sourced backlog item shows its badge and can be filtered to.
  - *Given* a seeded backlog item with `pluginId: "linear"` in the isolated test server's state dir, *When* the Playwright test navigates to the backlog board and selects "Linear" in the source filter, *Then* `expect(page.getByTestId("backlog-item-card")).toContainText("ENG-123")` passes and non-Linear cards are not visible.
**Files**: `tests/e2e/backlog-source-badge.spec.ts` (new)

##### Task 5.3.1a: Write the e2e spec (~5 min)
- `// @feature backlog:item-source-badge, backlog:filter-by-source` header (per `.claude/rules/e2e-test-conventions.md`). Uses `data-testid`/ARIA locators only, no `waitForTimeout`. Seeds test data via the existing test-mode backlog-item creation helper (locate the pattern other backlog e2e specs use, e.g. `tests/e2e/pages/` helpers, before writing from scratch).
- Files: `tests/e2e/backlog-source-badge.spec.ts`

##### Task 5.3.1b: Add `data-testid="backlog-item-card"` if missing + registry entries (~3 min)
- Confirm `BacklogItemCard.tsx` already exposes a stable test ID (grep first); add per-feature registry files (`docs/registry/features/frontend/backlog-item-source-badge.json`, `backlog-filter-by-source.json`) with `testIds` referencing the new spec's `describe`/`test` names.
- Files: `web-app/src/components/backlog/BacklogItemCard.tsx` (if needed), `docs/registry/features/frontend/backlog-item-source-badge.json` (new), `docs/registry/features/frontend/backlog-filter-by-source.json` (new)
