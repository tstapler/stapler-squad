# Implementation Plan: github-import-robustness

## Architecture

Three largely independent work streams that can proceed in parallel after the proto change lands:

1. **Stream A — Import picker deduplication** (AC-1, AC-3): annotate `ListGitHubIssues` response + UI filter
2. **Stream B — Claim label write-back** (AC-2, AC-3 server-side): `AddIssueLabel` on import + `RemoveIssueLabel` on archive
3. **Stream C — Forward-sync hardening** (AC-4, AC-5): fix source-less items + archived status trigger

---

## Phase 0 — Proto & schema changes (unblocks all streams)

### Task 0.1 — Add `already_in_backlog` field to `GitHubIssueEntry`

**File**: `proto/session/v1/backlog.proto`

```proto
message GitHubIssueEntry {
  // existing fields...
  bool already_in_backlog = 10;  // next free field number — verify
}
```

Run `make proto-gen` to regenerate Go and TS bindings.

### Task 0.2 — Add `claim_label` to `githubPluginConfig`

**File**: `session/backlog_plugin_github.go`

```go
type githubPluginConfig struct {
  // existing fields...
  ClaimLabel string `json:"claim_label"` // label applied when issue is imported; default "stapler-squad"
}
```

---

## Phase 1 — Stream A: Import picker deduplication

### Task 1.1 — Batch local-backlog lookup in `ListGitHubIssues`

**File**: `server/services/backlog_service_query.go`

After fetching issues from `gh.ListRepoIssues`, extract issue numbers as `[]string` and call `storage.repo.GetBacklogItemsByExternalIDs(ctx, sourceID, issueNumbers)`. The `sourceID` can be looked up from `storage.ListItemSources` filtered to the matching `owner/repo` (if any ItemSource is configured for this repo), falling back to an empty-string source ID for manually-imported items (which store the issue URL but may not have a source).

A cleaner approach: add a new storage method `GetBacklogItemsByExternalURLs(ctx, urls []string) (map[string]*BacklogItemData, error)` using the `external_url` field, which is always set regardless of whether the item has a source. Issue URLs are deterministic (`https://github.com/owner/repo/issues/N`), making this a reliable cross-reference without needing the source ID.

Set `already_in_backlog = true` on the `GitHubIssueEntry` for any issue whose URL is in the returned map.

### Task 1.2 — UI: filter already-imported issues in picker default view

**File**: `web-app/src/components/backlog/GitHubIssuePicker.tsx`
**File**: `web-app/src/lib/hooks/useGitHubIssuePicker.ts`

Add `showAlreadyImported: boolean` state to the picker hook (default `false`). Filter `picker.issues` to exclude `issue.alreadyInBacklog` when the toggle is off. Add a toggle button in the filter bar: `Show already imported (N)` where N is the count of hidden issues.

`IssueRow` already has a `listItemDisabled` style — reuse it for already-imported rows when they're shown.

### Task 1.3 — Tests

- Unit test in `server/services/backlog_github_rpc_test.go`: `TestListGitHubIssues_AlreadyInBacklog` — storage has item with matching external URL, verify `already_in_backlog=true` on that entry.
- Frontend: update `GitHubIssuePicker.test.tsx` to cover the filter toggle.

---

## Phase 2 — Stream B: Claim label write-back

### Task 2.1 — Add `AddIssueLabel` to `GitHubIssuesPlugin`

**File**: `session/backlog_plugin_github.go`

```go
// AddIssueLabel appends label to the GitHub issue's label list.
// Uses POST /repos/{owner}/{repo}/issues/{number}/labels (additive, not replacement).
// Returns nil on success or if the issue already has the label.
// Returns a non-fatal error on label-not-found (422) — callers should log Warn, not propagate.
func (g *GitHubIssuesPlugin) AddIssueLabel(ctx context.Context, config PluginConfig, externalID string, label string) error
```

Implement via `POST .../labels` with `{"labels": [label]}`. Handle 422 (label doesn't exist in repo) by logging a Warn and returning nil, so the import does not fail.

### Task 2.2 — Add `RemoveIssueLabel` to `GitHubIssuesPlugin`

**File**: `session/backlog_plugin_github.go`

```go
// RemoveIssueLabel removes label from the GitHub issue's label list.
// Uses DELETE /repos/{owner}/{repo}/issues/{number}/labels/{label}.
// Returns nil if the label was not present (404 = already gone, not an error).
func (g *GitHubIssuesPlugin) RemoveIssueLabel(ctx context.Context, config PluginConfig, externalID string, label string) error
```

### Task 2.3 — Call `AddIssueLabel` on import

**File**: `server/services/backlog_service.go` (ImportGitHubIssue handler)
OR `session/backlog_sync.go` (SyncOne for source-driven imports)

After the backlog item is created (`already_existed == false`), fire a best-effort goroutine or inline call to `AddIssueLabel`. The claim label is derived from:
1. Source config `claim_label` if the item has a source.
2. A global default `"stapler-squad"` otherwise.

This must not block or fail the import — log Warn on error, return the created item regardless.

### Task 2.4 — Call `RemoveIssueLabel` on archive

**File**: `server/services/backlog_github_forward_sync.go`

Extend `handleForwardSyncClose` (or add a parallel `handleForwardSyncArchive`) to call `RemoveIssueLabel` when the transition is to `archived`. Only remove if the item was not merged (no PR) — if the item is `done`, leave the label (the issue is closed anyway).

### Task 2.5 — Tests

- `session/backlog_plugin_github_test.go`: `TestAddIssueLabel_*`, `TestRemoveIssueLabel_*`
- Integration test: import an issue, verify label add is called; archive an item, verify label remove is called.

---

## Phase 3 — Stream C: Forward-sync hardening

### Task 3.1 — Close issues for source-less items

**File**: `server/services/backlog_github_forward_sync.go`

Remove the `if current.SourceID == ""` early-return. Instead, add a separate code path: when `SourceID` is empty but `ExternalURL` is non-empty and parses as a GitHub issue URL, derive close credentials from `github.GetKeychainTokenForHost(host)` directly (no source lookup). Build a synthetic `PluginConfig` from the keychain token and call `CloseIssue` directly.

This requires extracting the owner/repo/number from `ExternalURL` via `github.ParseGitHubRef` (already exists in `github/url_parser.go`).

### Task 3.2 — Fire forward-sync on `archived` transition

**File**: `server/services/backlog_github_forward_sync.go`

Change the subscriber filter from:
```go
payload.NewStatus != string(session.BacklogStatusDone)
```
to:
```go
payload.NewStatus != string(session.BacklogStatusDone) &&
payload.NewStatus != string(session.BacklogStatusArchived)
```

(i.e., drop the `!= ... Archived` exclusion — fire on either status.)

Add a `archiveComment` constant for the comment text. The `handleForwardSyncClose` function passes the comment string in, so threading a different comment for archived vs done is straightforward.

### Task 3.3 — Tests

- `server/services/backlog_github_forward_sync_test.go`: `TestForwardSync_SourcelessItem_ClosesIssue`, `TestForwardSync_ArchiveTransition_ClosesIssue`
- Verify no regression on the existing `TestForwardSyncSubscriber_NoOpWhenForwardSyncDisabled` style tests.

---

## Adversarial Review

**Risk 1 — Rate limit pressure from `ListGitHubIssues` adding a DB lookup**
The `GetBacklogItemsByExternalURLs` call adds a DB read per `ListGitHubIssues` call. This is in-process SQLite (fast, no network hop). The issue list page size is ≤100 rows; the DB lookup is a single IN-clause query. No meaningful latency concern.

**Risk 2 — `AddIssueLabel` fails silently, cross-host dedup still relies on gossip**
The claim label is best-effort. If the label add fails (repo permissions issue, token missing write scope, rate limit), the issue remains unlabeled and another Stapler Squad instance could pick it up. This is acceptable — the label is a convenience signal, not the primary dedup gate (the DB lookup in AC-1 handles same-instance dedup; cross-host gossip handles cross-instance dedup). Document this explicitly.

**Risk 3 — `archived` forward-sync closes issues users may want to re-open**
Closing an archived item's issue is permanent (from GitHub's perspective). A user who archives a backlog item speculatively and then changes their mind now has a closed GitHub issue. Mitigation: add a note in the close comment: "If this was archived in error, you can reopen the issue manually." The `RemoveIssueLabel` path (AC-6) runs on archive, which removes the claim, so reopening is clean.

**Risk 4 — Source-less `CloseIssue` path bypasses `norawghrequest` rule**
The existing `CloseIssue` uses `http.NewRequestWithContext` directly — it predates the `norawghrequest` rule and its file (`session/backlog_plugin_github.go`) is outside the rule's enforcement scope. New code in `backlog_github_forward_sync.go` calling the plugin directly (not via an http.NewRequest) is fine. No violation.

**Risk 5 — `GetBacklogItemsByExternalURLs` query is non-indexed**
`external_url` may not be indexed in the ent schema. Check `session/ent/schema/` — if there's no index on `external_url`, add one or use a range query bounded by prefix match (`https://github.com/owner/repo/issues/`). The `GetBacklogItemsByExternalIDs` method uses `(source_id, external_id)` which is indexed. Adding a `GetBacklogItemsByExternalURLs` method using `external_url` needs an index on that field in the ent schema to avoid a full table scan on large backlogs.
