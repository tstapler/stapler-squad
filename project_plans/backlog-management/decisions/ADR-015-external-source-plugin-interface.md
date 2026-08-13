# ADR-015: External Source Plugin Interface

**Status**: Accepted
**Date**: 2026-05-10

## Context

Backlog items can originate from external sources — initially GitHub Issues, with other sources (Linear, Jira, plain CSV) anticipated post-MVP. The integration must:

- Pull items from the source on a configurable polling interval
- Deduplicate items across syncs using a stable external identifier
- Apply a conflict resolution model that preserves user edits (local-wins)
- Produce a sync log entry per run for user visibility
- Allow new sources to be added without modifying core backlog logic

Four integration architectures were evaluated. The critical constraints are that Stapler Squad runs on `localhost:8543` with no guaranteed inbound network access (ruling out webhooks as a primary transport), and the codebase is a single Go binary with no plugin process infrastructure.

## Decision

Define a `BacklogSource` Go interface in `session/backlog_source.go`. Register implementations in a source registry at startup. GitHub Issues is the only implementation for MVP.

**Interface definition**:

```go
// BacklogSource is implemented by each external source plugin.
// Plugins are registered at startup and polled by the SourceSyncer goroutine.
type BacklogSource interface {
    // SourceID returns the stable plugin identifier used as the discriminator
    // in the ItemSource ent schema (e.g., "github_issues").
    SourceID() string

    // Sync fetches items from the external source since the opaque cursor.
    // cursor is empty on the first call; the returned cursor must be stored
    // and passed unchanged on the next call. Returning an empty cursor resets pagination.
    // items is the complete set of new or updated items observed since cursor.
    Sync(ctx context.Context, cfg SourceConfig, cursor string) (items []RawItem, nextCursor string, err error)

    // ExternalID returns the stable identifier for an item within this source.
    // Used as the deduplication key: (SourceID, ExternalID) must be globally unique.
    ExternalID(item RawItem) string

    // ToDraft converts a RawItem to a BacklogItemDraft for upsert.
    // The plugin controls field mapping; core controls conflict resolution.
    ToDraft(item RawItem) BacklogItemDraft
}

// RawItem is an opaque map of source-provided fields.
// Plugins read from it using typed accessors; core never inspects the contents.
type RawItem map[string]any

// BacklogItemDraft is the subset of BacklogItem fields a plugin is allowed to populate.
// The core service applies conflict resolution against user_modified_fields before saving.
type BacklogItemDraft struct {
    Title       string
    Description string   // raw body from the source; never auto-parsed for AC
    Labels      []string
    SourceURL   string
    ExternalID  string
}

// SourceConfig is the plugin-specific configuration stored in JSON in the ItemSource row.
type SourceConfig struct {
    Raw json.RawMessage // parsed by each plugin into its own config struct
}
```

**Source registry**:

```go
// In session/backlog_registry.go:
type SourceRegistry struct {
    sources map[string]BacklogSource
}

func NewSourceRegistry() *SourceRegistry {
    r := &SourceRegistry{sources: make(map[string]BacklogSource)}
    r.Register(NewGitHubIssuesSource())
    return r
}

func (r *SourceRegistry) Register(s BacklogSource) {
    r.sources[s.SourceID()] = s
}

func (r *SourceRegistry) Get(id string) (BacklogSource, bool) {
    s, ok := r.sources[id]
    return s, ok
}
```

**GitHub Issues implementation** (`session/sources/github_issues.go`):

```go
type GitHubIssuesSource struct{}

func (g *GitHubIssuesSource) SourceID() string { return "github_issues" }

type githubConfig struct {
    Owner          string `json:"owner"`
    Repo           string `json:"repo"`
    LabelFilters   []string `json:"label_filters,omitempty"`
    SyncIntervalMin int    `json:"sync_interval_minutes"`
}

func (g *GitHubIssuesSource) Sync(ctx context.Context, cfg SourceConfig, cursor string) ([]RawItem, string, error) {
    // cursor = last seen issue number (as string) or ""
    // uses GET /repos/{owner}/{repo}/issues?state=open&since=<cursor>&per_page=100
    // respects X-RateLimit-Remaining; returns nextCursor = highest issue number seen
}
```

**Sync loop** (`session/backlog_syncer.go`):

The `SourceSyncer` goroutine runs a ticker per configured `ItemSource` row. On each tick it:

1. Loads the `ItemSource` record from the DB (plugin ID, config JSON, cursor).
2. Looks up the `BacklogSource` implementation from the registry by plugin ID.
3. Calls `source.Sync(ctx, cfg, cursor)`.
4. For each returned `RawItem`, calls `source.ExternalID(item)` to get the dedup key, then calls `source.ToDraft(item)` to get the draft.
5. Upserts the draft using the `(source_id, external_id)` unique key via ent's `sql/upsert` feature, skipping any field present in the existing row's `user_modified_fields` JSON set.
6. Advances the cursor and writes a `SourceSyncEvent` record.

The sync loop does not know about GitHub, Linear, or any specific source — it only knows the `BacklogSource` interface.

## Alternatives Considered

**Option A: Hard-coded GitHub support (no interface)**

Implement GitHub sync directly in the service layer without an abstraction. Simpler for MVP, avoids premature generalization.

Rejected because the requirements explicitly anticipate post-MVP sources (US-11–US-13 describe GitHub; the plugin architecture goal in the feature goals section calls for new sources without core changes). Hard-coding GitHub support means the GitHub-specific logic (pagination, rate limit handling, label mapping) is entangled with the conflict resolution logic, making both harder to test and harder to replace. The interface is small (4 methods) and imposes no runtime overhead; the cost of adding it now is minimal relative to the cost of extracting it later.

**Option B: Webhook-based push model**

External sources push events to a Stapler Squad webhook endpoint rather than Stapler Squad polling them.

Rejected because Stapler Squad runs on `localhost:8543` with no guaranteed public network access. GitHub cannot deliver webhooks to a localhost endpoint. Even with Tailscale or ngrok, requiring an inbound tunnel for a core feature creates an unreasonable operational burden for single-user deployment. Polling is the correct primary transport. Webhooks can be added as an optional enhancement (a second `BacklogSourceTransport` interface) without changing the `BacklogSource` interface.

**Option C: gRPC plugin process**

Each source runs as a separate process that Stapler Squad communicates with via a local gRPC socket (similar to Terraform provider plugins or HashiCorp's go-plugin library).

Rejected as over-engineering for the current scope. Stapler Squad is a single Go binary with no plugin process infrastructure. Introducing gRPC plugin processes would require: a plugin discovery mechanism, process lifecycle management, IPC error handling, separate build and distribution for each plugin, and versioning of the plugin protocol. For MVP with a single implementation, this is an order of magnitude more complexity than an in-process interface. Out-of-process plugins can be revisited if sources need to be developed and distributed independently.

## Consequences

**Positive**

- Clean separation: the sync loop knows nothing about any specific source. Adding a new source requires implementing one interface and calling `registry.Register()` — zero changes to `SourceSyncer`, conflict resolution, or the service layer.
- Testable with mock sources: tests for the sync loop, conflict resolution, and upsert logic can use a `MockBacklogSource` that returns controlled `RawItem` slices without making network calls.
- GitHub Issues implementation is fully isolated in `session/sources/github_issues.go`; rate limit handling, label mapping, and pagination are contained in one file.
- Cursor-based pagination is source-agnostic: the opaque cursor string is stored in the `ItemSource` row and passed back to the plugin unchanged. GitHub uses an issue number cursor; other sources can use timestamps, ETags, or page tokens.
- Polling avoids firewall/tunnel requirements: no inbound network exposure needed for MVP.

**Negative**

- Polling has inherent latency: new GitHub issues appear in the backlog only after the next sync tick (default: 15 minutes). Acceptable for a planning tool; not suitable for real-time alerting.
- `RawItem` as `map[string]any` is untyped. Plugins that misuse the map (wrong key names, wrong value types) produce runtime errors rather than compile-time errors. Mitigated by typed accessor helpers on `RawItem` and comprehensive unit tests for each plugin's `ToDraft` and `ExternalID` methods.
- The `SourceConfig.Raw json.RawMessage` approach requires each plugin to unmarshal its own config, which duplicates boilerplate across plugins. Mitigated by a shared `UnmarshalConfig[T any](cfg SourceConfig) (T, error)` generic helper.
- In-process plugins cannot be updated independently of the Stapler Squad binary. Acceptable for MVP; out-of-process plugin support deferred to a future milestone.
- Polling-only means GitHub Issues that are closed and deleted between two sync ticks may never be seen. Mitigated by storing `source_last_seen_at` on each synced `BacklogItem` and surfacing items not seen in the last N ticks as "possibly deleted" in the UI.
