# Implementation Plan: GitHub Issue State Sync

## Overview

Three epics:
1. **Schema + API**: Store `claiming_host_id` and `github_last_progress_comment_status` on BacklogItem; expose in protos.
2. **Progress sync subscriber**: Post progress comments on `in_progress` and `review` transitions.
3. **Sourceless item support**: Allow MCP-imported items (no SourceID) to post status comments.

---

## Epic 1: Schema & API (Claiming Host + Progress Comment Watermark)

### Task 1.1 — Ent schema: add two new optional fields

**File**: `session/ent/schema/backlog_item.go`

Add after `github_synced_issue_updated_at`:
```go
field.String("claiming_host_id").
    Optional().
    Comment("HostID of the host that claimed this item from the external source; empty for locally-created items."),
field.String("github_last_progress_comment_status").
    Optional().
    Comment("Last BacklogItem status for which a GitHub progress comment was successfully posted; prevents duplicate comments on re-entry."),
```

Regenerate ent: `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (do NOT commit generated output — `.gitignore` excludes it).

### Task 1.2 — BacklogItemData struct

**File**: `session/repository.go` (BacklogItemData struct, ~line 323)

Add:
```go
ClaimingHostID                  string `json:"claiming_host_id,omitempty"`
GitHubLastProgressCommentStatus string `json:"github_last_progress_comment_status,omitempty"`
```

Update `entItemToData()` to map both fields.  
Update `Storage.CreateBacklogItem()` to populate `ClaimingHostID` from the incoming `BacklogItemData` (it's set by the MCP tool and sync path before calling Create).

### Task 1.3 — Populate ClaimingHostID at creation time

**File**: `server/mcp/tools_backlog.go` (`import_github_issue` tool, ~line 2366)

After building the claim record (around line 2426), set `item.ClaimingHostID = localHostID.String()` before `storage.CreateBacklogItem()`.

**File**: `session/backlog_sync.go` (`SyncOne` / `MapToBacklogItem`)

In `MapToBacklogItem()` or `SyncOne()`, after the claim is recorded, populate `ClaimingHostID` on the created `BacklogItemData` from the local host identity.

### Task 1.4 — Expose in proto

**File**: `proto/session/v1/session.proto`

Add to the `BacklogItem` message:
```proto
string claiming_host_id = <next_field_number>;
string github_last_progress_comment_status = <next_field_number>;
```

Run `make proto-gen` to regenerate. Update `BacklogItemToProto()` / `ProtoToBacklogItem()` in the services layer.

### Task 1.5 — Return in list/get RPC handlers

**File**: `server/services/backlog_service.go` (or wherever `BacklogItemToProto` is defined)

Map `ClaimingHostID` and `GitHubLastProgressCommentStatus` from `BacklogItemData` → proto message in the conversion function.

### Task 1.6 — Web UI: display claiming host

**File**: `web-app/src/components/BacklogItemDetail/` (or equivalent)

Add a "Working host" row in the item detail panel that renders `claiming_host_id` when non-empty. Guard behind a null check — items without a host show nothing.

---

## Epic 2: Progress Sync Subscriber

### Task 2.1 — Feature flag registration

**File**: `server/services/feature_flags.go` (or wherever flags are registered)

Register: `"github_issue_progress_comments"` with default `false`.

### Task 2.2 — New subscriber file

**New file**: `server/services/backlog_github_progress_sync.go`

Modeled on `backlog_github_forward_sync.go`:

```go
// StartBacklogGitHubProgressSyncSubscriber subscribes to BacklogChangeStatusTransition
// events and posts progress comments on linked GitHub issues for key transitions
// (in_progress, review), gated by the github_issue_progress_comments feature flag.
func StartBacklogGitHubProgressSyncSubscriber(ctx context.Context, bus *events.EventBus, flags FeatureFlagGetter, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage, hostID string) <-chan struct{}
```

On `BacklogChangeStatusTransition`:
- Check feature flag `github_issue_progress_comments`; skip if false
- Check `item.ExternalURL != ""`; skip if empty
- Switch on `NewStatus`:
  - `"in_progress"`: post `🤖 [stapler-squad] Work has started on this issue on host <host_id>.`
  - `"review"`: post `🤖 [stapler-squad] A pull request has been opened: <pr_url>.` (skip if `pr_url == ""`)
- Duplicate prevention: fetch latest item, check `GitHubLastProgressCommentStatus == NewStatus`; skip if match
- After successful post: update `GitHubLastProgressCommentStatus = NewStatus` and advance `GitHubSyncedIssueUpdatedAt`

### Task 2.3 — Token resolution helper

**New function** in `server/services/backlog_github_progress_sync.go` or `session/backlog_plugin_github.go`:

```go
// resolveGitHubTokenForItem returns a (PluginConfig, owner, repo) for posting to
// the GitHub issue linked by item.ExternalURL. First tries the SourceID path
// (decrypting token from ItemSource.Config). Falls back to github.GetToken(host).
// Returns ("", false) if no token can be resolved.
func resolveGitHubTokenForItem(ctx context.Context, item *BacklogItemData, registry *PluginRegistry, syncLoop *SyncLoop) (PluginConfig, bool)
```

### Task 2.4 — Wire subscriber into server startup

**File**: `server/dependencies.go` or `server/server.go`

Call `StartBacklogGitHubProgressSyncSubscriber(...)` alongside the existing `StartBacklogGitHubForwardSyncSubscriber(...)`.

### Task 2.5 — Tests for new subscriber

**New file**: `server/services/backlog_github_progress_sync_test.go`

Cover:
- `in_progress` transition posts comment when flag enabled
- `in_progress` transition skipped when flag disabled
- `review` transition posts comment with PR URL
- `review` transition skipped when no PR URL
- Duplicate prevention: second `in_progress` transition skips comment
- Sourceless item (no SourceID): posts comment via keychain fallback
- Sourceless item with no token: skips silently

---

## Epic 3: Existing Done-Transition Update (AC-5)

### Task 3.1 — Update done-transition comment to include host

**File**: `server/services/backlog_github_forward_sync.go`

Change `forwardSyncCloseComment` constant from:
```go
const forwardSyncCloseComment = "Closed automatically — the linked backlog item was marked done in stapler-squad."
```
to a format string:
```go
func forwardSyncCloseCommentFor(hostID string) string {
    if hostID == "" {
        return "Closed automatically — the linked backlog item was marked done in stapler-squad."
    }
    return fmt.Sprintf("Closed automatically — the linked backlog item was marked done in stapler-squad on host `%s`.", hostID)
}
```

Pass `current.ClaimingHostID` when calling this.

---

## Execution Order

1. Epic 1 (schema/API) first — all other tasks depend on `ClaimingHostID` being stored and returned.
2. Epic 2 (progress subscriber) is independent of Epic 3 (done comment update) once Epic 1 is done.
3. Epic 3 is a small, isolated change to existing code.

Epic 1 tasks 1.1–1.3 (ent + data layer) must complete before 1.4–1.6 (proto + UI).  
Epic 2 tasks 2.1–2.3 can be written in parallel; 2.4–2.5 depend on 2.1–2.3.

---

## Files Changed Summary

| File | Change |
|---|---|
| `session/ent/schema/backlog_item.go` | Add 2 optional string fields |
| `session/repository.go` | Add fields to BacklogItemData, map in entItemToData, populate in Create |
| `server/mcp/tools_backlog.go` | Set ClaimingHostID before CreateBacklogItem |
| `session/backlog_sync.go` | Set ClaimingHostID in MapToBacklogItem |
| `proto/session/v1/session.proto` | Add 2 proto fields |
| `server/services/` (proto converter) | Map new fields |
| `server/services/feature_flags.go` | Register new flag |
| `server/services/backlog_github_forward_sync.go` | Update done comment to include host |
| `server/services/backlog_github_progress_sync.go` | **New file**: progress subscriber |
| `server/services/backlog_github_progress_sync_test.go` | **New file**: tests |
| `server/dependencies.go` | Wire new subscriber |
| `web-app/src/components/BacklogItemDetail/` | Show claiming_host_id |
| `web-app/src/app/route-meta.ts` | No change needed (no new page) |
