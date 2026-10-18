# Research Findings: github-import-robustness

## Codebase Audit

### 1. ListGitHubIssues — no local backlog lookup

**File**: `server/services/backlog_service_query.go:301-349`

`ListGitHubIssues` calls `gh.ListRepoIssues(...)` and maps results directly to `GitHubIssueEntry` proto messages. It makes no call to `storage.GetBacklogItemsByExternalIDs` or any equivalent, so the response has no annotation indicating which issues are already in the local backlog. The proto message `GitHubIssueEntry` (`proto/session/v1/backlog.proto`) also has no `already_in_backlog` field.

**Impact**: Every open issue is shown in the picker regardless of import status. The only dedup is post-import (`ImportGitHubIssueResponse.already_existed`) — too late to filter the list.

### 2. ImportGitHubIssue — dedup at insert, not at list

**File**: `server/services/backlog_service.go` (ImportGitHubIssue handler)

`ImportGitHubIssue` does detect an already-existing item (`already_existed` in the response) and returns the existing item without creating a duplicate. However this is a reactive gate, not a proactive filter. The UI currently shows a toast "Already imported — no new items created" only after the user tries to import.

### 3. Forward-sync subscriber — gates on ForwardSyncEnabled and skips non-source items

**File**: `server/services/backlog_github_forward_sync.go:114-215`

`handleForwardSyncClose` early-returns when `current.SourceID == ""`. Items imported via `ImportGitHubIssue` (the manual "Import from GitHub Issue" UX path, not the periodic SyncLoop) have no `SourceID`, so they are permanently skipped. The comment in the code explicitly defers the "already closed" P2 optimization but does not address the no-source gap.

Additionally, the subscriber only subscribes on `payload.NewStatus != string(session.BacklogStatusDone)` — it skips `archived` status transitions.

### 4. No claim-label write-back

**File**: `session/backlog_plugin_github.go` (full file)

`GitHubIssuesPlugin` has `CloseIssue` and `PostIssueComment` but no `ClaimIssue` or `LabelIssue` method. When an issue is imported, no PATCH request is made to add a label to the GitHub issue. The `githubPluginConfig` struct has `LabelPriorityMap` for reading priority from labels, but no `claim_label` field for writing a label back.

### 5. GitHubIssuePicker — no visibility of import status

**File**: `web-app/src/components/backlog/GitHubIssuePicker.tsx`

`IssueRow` shows a "PR" vs "#" badge and disables PR-type issues. There is no "already imported" badge or filter. `picker.issues` is passed directly from the `listGitHubIssues` RPC result without any local backlog cross-reference.

### 6. Cross-host dedup exists but is reactive

**File**: `proto/session/v1/backlog.proto:1856-1877` (`ImportGitHubIssueResponse.already_claimed_elsewhere`)

The `ImportGitHubIssue` path has cross-host claim dedup — if another host already claimed the URL, the response includes `already_claimed_elsewhere` with the claiming host's ID and a deep link. But this again fires only at import time, not at list time.

### 7. ExternalID storage

**File**: `proto/session/v1/backlog.proto:231`

`BacklogItem` has `external_id` (field 14) and `external_url`. These are stored in the ent schema and indexed. `GetBacklogItemsByExternalIDs` in `session/ent_repository_backlog.go` performs a batch lookup by `(source_id, external_id)`. A batch lookup from within `ListGitHubIssues` is feasible — the issue numbers returned by `gh.ListRepoIssues` can be converted to `external_id` strings and cross-referenced against the DB.

## GitHub API Notes

- **Labels endpoint**: `PATCH /repos/{owner}/{repo}/issues/{issue_number}` accepts a `labels` array (full replacement) or `POST /repos/{owner}/{repo}/issues/{issue_number}/labels` for additive label application. The additive endpoint is safer (won't clobber existing labels). The `CloseIssue` implementation already uses PATCH with a merge strategy for labels; `AddIssueLabel` can be a separate POST call.
- **Rate limit**: Each label-add is one API call. The `norawghrequest` rule requires using the approved constructors. The existing `CloseIssue` path uses `http.NewRequestWithContext` directly — it predates the `norawghrequest` rule but this file is not in the rule's enforcement scope. New `AddIssueLabel` code should use the same pattern as `CloseIssue`.
- **Label creation**: If the label doesn't exist in the repo yet, `POST /repos/{owner}/{repo}/issues/{number}/labels` returns 422. We should either pre-create the label or handle 422 gracefully (log + skip, don't fail import).

## Relevant Existing Plans

- `project_plans/backlog-github-issue-link/` — overlaps on issue linking, already implemented
- `project_plans/backlog-github-two-way-sync/` — backward sync (GitHub state → backlog), separate
- `project_plans/cross-host-claim-dedup/` — cross-host gossip for duplicate import prevention
- `project_plans/github-issue-picker/` — initial picker implementation
