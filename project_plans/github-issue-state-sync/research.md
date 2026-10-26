# Research: GitHub Issue State Sync

## Code Archaeology

### Current BacklogItem GitHub State Fields (ent schema)

From `session/ent/schema/backlog_item.go`:
- `external_id` — GitHub issue number as a string (indexed)
- `external_url` — browser-facing HTML URL (indexed)
- `labels` — GitHub issue label names as of last sync
- `github_synced_issue_updated_at` — loop-prevention watermark: the GitHub `updated_at` last synced in either direction

**Missing**: no field for which host holds the claim, no field recording the last progress comment status.

### ClaimRecord Structure

From `session/claim_index.go`:
```go
type ClaimRecord struct {
    ExternalURL    string
    ClaimingHostID HostID
    ItemDeepLink   string
    ClaimedAt      time.Time
    PublicKey      ed25519.PublicKey
    Signature      []byte
    Disputed       bool
}
```

The `ClaimingHostID` is already computed and stored in the claim index file (`claim_index.json`) at item creation time. It is not denormalized onto the BacklogItem itself. `Storage.CreateBacklogItem()` calls `ClaimRecorder` which calls `ClaimIndex.RecordClaim()`.

### Two Import Paths

**Path A — MCP `import_github_issue`** (`server/mcp/tools_backlog.go:2366`):
- Creates item with `ExternalID`, `ExternalURL`, `Notes: "Imported from <URL>"`
- **No SourceID** — not linked to any periodic sync `ItemSource`
- The forward sync subscriber (`backlog_github_forward_sync.go`) skips items where `current.SourceID == ""` (line ~133 checks `if current.SourceID == "" && current.ExternalID == ""`)

**Path B — Periodic backward sync** (`session/backlog_sync.go`, `session/backlog_plugin_github.go`):
- Items have `SourceID` linking to an `ItemSource` with `plugin_id = "github_issues"`
- `SyncOne()` upserts items; `MapToBacklogItem()` sets `SourceID`
- Forward sync subscriber handles these when `ForwardSyncEnabled`

### Existing Forward Sync Subscriber

From `server/services/backlog_github_forward_sync.go`:
- Listens for `EventBacklogItemChanged` where `Kind == BacklogChangeStatusTransition && NewStatus == "done"`
- Requires `SourceID != "" && ExternalID != ""`
- Requires `source.ForwardSyncEnabled == true`
- Calls `plugin.CloseIssue()` then `plugin.PostIssueComment()`
- After success: updates `GitHubSyncedIssueUpdatedAt` with GitHub API's post-close `updated_at`

The `PostIssueComment` method on `GitHubIssuesPlugin` (`session/backlog_plugin_github.go:371`) is already implemented and uses the configured token from `ItemSource.Config`.

### GitHub Token Resolution for MCP-Imported Items

For items without a `SourceID`, we need a different token source. `github/keychain.go` provides `GetToken(host string) (string, error)` which reads from the system keychain (or `~/.config/gh/hosts.yml` fallback). This is exactly what `import_github_issue` uses when fetching the issue (`server/mcp/tools_backlog.go:2394`).

The `github.ParseGitHubRefWithHosts()` utility extracts the host from the `external_url`. A helper can:
1. Parse the host from `external_url`
2. Call `github.GetToken(host)` to get a token
3. Build a one-off `PluginConfig` from that token + the parsed owner/repo

This pattern is used by `import_github_issue` already (lines 2394–2415).

### EventBus Status Transition Events

`BacklogChangeStatusTransition` events carry `BacklogItemChangedPayload` which includes `Item *BacklogItemData` (with `Status`, `ExternalURL`, `ExternalID`, `SourceID`, `PRUrl`, `ClaimingHostID` once we add the field).

Statuses that represent key transitions:
- `"in_progress"` — work started
- `"review"` — PR opened (typically set when `pr_url` is set)
- `"done"` — completed

The status machine is defined in `session/backlog.go` (`ValidStatusTransitions`).

### Loop-Prevention Mechanism

From `session/ent/schema/backlog_item.go`: `github_synced_issue_updated_at` — after writing to GitHub, we must fetch the issue's current `updated_at` and store it, so the backward sync poll sees `issue.updated_at <= stored_watermark` and skips re-ingesting our own write.

The `CloseIssue` implementation in `backlog_plugin_github.go` already does this (fetches updated_at after the PATCH). The new progress comment paths must do the same via `GetIssue()` after posting.

### Feature Flag Infrastructure

The existing feature flag system (`server/services/feature_flags.go`, `session/ent/schema/`) supports:
- Per-scope flag overrides (global + per-user)
- `GetFlag(ctx, flagKey) bool` pattern
- New flags are registered in a `flagRegistry` map

### Existing API Surface

Proto definitions in `proto/session/v1/session.proto`:
- `BacklogItem` message — needs a new `claiming_host_id` string field
- `GetBacklogItem`, `ListBacklogItems` responses — already return `BacklogItem` messages

### Forward Sync Skip Condition for MCP-Imported Items

Current skip condition (line ~133, `handleForwardSyncClose`):
```go
if current.SourceID == "" && current.ExternalID == "" {
    log.Debug("backlog_github_forward_sync: item has no source, skip", ...)
    return
}
```

Wait — the actual check requires **both** to be empty. So items with `ExternalURL` but no `SourceID` are currently **not** skipped by this check alone (they'd be skipped later when looking up the source). We need to add a separate fallback path for sourceless items.

## Key Design Decisions

### D1: Store ClaimingHostID on BacklogItem vs. query ClaimIndex

**Option A**: Denormalize `claiming_host_id` onto the BacklogItem ent entity.  
**Option B**: Query the ClaimIndex at runtime whenever the API needs to return it.

**Decision: Option A** — denormalization is simpler and consistent with the existing pattern (`external_id` is also denormalized from the claim process). The ClaimIndex is a file-backed gossip structure not suitable for SQL joins.

**Implementation**: Add `claiming_host_id` (string, optional) to `session/ent/schema/backlog_item.go`. Populate it in `Storage.CreateBacklogItem()` from `createInput.ClaimingHostID` (already plumbed via `BacklogItemData`).

### D2: Trigger progress comments from EventBus vs. item lifecycle hooks

**Option A**: Add a second subscriber to the EventBus for `BacklogChangeStatusTransition` events.  
**Option B**: Inline calls in the `StartSession`/`TransitionStatus` service handlers.

**Decision: Option A** — matches existing forward sync pattern exactly. Keeps the concern separate and testable. A new `StartBacklogGitHubProgressSyncSubscriber` in `server/services/backlog_github_progress_sync.go` mirrors the existing `backlog_github_forward_sync.go`.

### D3: Token resolution for sourceless (MCP-imported) items

For items without a `SourceID`, parse host from `external_url` and call `github.GetToken(host)` (same as the MCP import tool does). If no token: log at DEBUG and skip — don't fail the transition.

### D4: Feature flag scope

Gate new progress comment behavior (`in_progress` and `review` transitions) behind `github_issue_progress_comments` (default off). Done-transition close is NOT gated — it already has its own `ForwardSyncEnabled` gate on the ItemSource.

### D5: Duplicate-comment prevention

Track in a new optional `github_last_progress_comment_status` field on BacklogItem. Set it after a successful comment post. Skip posting if `current.Status == stored value`. Reset it if the item is re-activated from done (edge case: re-opened items).

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Loop: progress comment triggers backward sync re-ingest | Med | Med | AC-9: update `github_synced_issue_updated_at` after every GitHub write |
| Token not found for MCP-imported items | High | Low | Log + skip (AC-6); never fail the transition |
| Ent schema migration needed | Certain | Low | Two new optional fields, no existing data migration required |
| Proto breaking change | Low | Low | Add optional fields with default values |
| Rate-limit exhaustion from frequent comments | Low | Med | Only 2 new comment triggers (in_progress, review); gated behind flag |
