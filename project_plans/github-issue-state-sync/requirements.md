# Requirements: GitHub Issue State Sync

## Item
**Backlog ID:** 3a8d307a-fada-445a-a4ae-ef375e1f8441  
**Title:** Our backlog items should store some amount of state if they are created from a GitHub issue

## Problem Statement

Backlog items created from GitHub issues (via `import_github_issue` MCP tool or the periodic backward sync) currently have limited state linking back to the originating GitHub issue:

1. **Host tracking is opaque**: The claim index records which host *imported* an item but there is no queryable field on the BacklogItem itself, and "which host is actively working on it" is only implied by linked sessions—not stored anywhere.
2. **Status is not pushed incrementally**: The forward sync (`backlog_github_forward_sync.go`) only fires when an item transitions to `done`, closing the issue. There are no intermediate status updates pushed to the GitHub issue (e.g., "Now in progress", "Plan approved", "PR open for review").
3. **MCP-imported items lack a SourceID**: Items imported via the `import_github_issue` MCP tool (Path A) are not linked to any `ItemSource`, so they cannot use the existing forward sync mechanism at all — even for the done-transition close.
4. **No progress visibility on GitHub**: A team member watching the GitHub issue has no way to know whether an AI agent has picked up the issue, what stage it is at, or where the work is happening.

## Goals

1. Make it easy to answer "which host is working on this GitHub issue?" without querying the claim index separately.
2. Push actionable status updates to the GitHub issue as the item progresses through key lifecycle transitions.
3. Ensure MCP-imported items (without a SourceID) can also participate in forward sync when a GitHub token is available.
4. Keep the solution backward-compatible: items without an `external_url` must be unaffected.

## Non-Goals

- Real-time streaming of every micro-status to GitHub (only key transitions matter: started, PR open, done/closed).
- Modifying how the claim index gossip protocol works.
- Changing backward sync behavior (GitHub→stapler direction).

## Acceptance Criteria

### AC-1: Claiming host ID stored on BacklogItem
When a backlog item is created with a non-empty `external_url` (either via the MCP `import_github_issue` tool or the periodic backward sync), the `claiming_host_id` (a string copy of the `HostID` from the claim record) is persisted as a field on the BacklogItem entity so it can be queried and returned in list/get responses.

### AC-2: Claiming host ID exposed in API and UI
The `GetBacklogItem` and `ListBacklogItems` RPC responses include the `claiming_host_id` field when set. The backlog item detail panel in the web UI shows "Working host: <host_id>" for GitHub-imported items.

### AC-3: Status updates posted on in-progress transition
When a GitHub-source item (with `external_url` non-empty and a GitHub token resolvable) transitions to `in_progress`, a comment is posted on the linked GitHub issue: `"🤖 **[stapler-squad]** Work has started on this issue on host \`<host_id>\`."` This applies to both SourceID-linked items and MCP-imported items.

### AC-4: Status updates posted on PR-open transition
When a GitHub-source item transitions to `review` (PR opened), a comment is posted on the linked GitHub issue: `"🤖 **[stapler-squad]** A pull request has been opened: <pr_url>."` Only posted if `pr_url` is set on the item.

### AC-5: Done-transition comment includes host
The existing done-transition forward sync comment is updated to include the claiming host ID: `"🤖 **[stapler-squad]** Closed automatically — the linked backlog item was marked done on host \`<host_id>\`."` Existing behavior (closing the issue) is unchanged.

### AC-6: MCP-imported items can post status updates
When a backlog item has `external_url` pointing to a GitHub issue but no `SourceID`, the system attempts to post status update comments using the token from the GitHub keychain (via `github.GetToken(host)`) rather than requiring a configured `ItemSource`. Failure to find a token is silently skipped (logged at DEBUG).

### AC-7: Forward sync is opt-in via feature flag
A new feature flag `github_issue_progress_comments` (default `false`) gates AC-3 and AC-4 behavior. AC-5 (done close) is unchanged and not gated. The flag can be set per-scope via the existing flag RPC.

### AC-8: No duplicate comments on re-entry
If an item transitions `in_progress → queued → in_progress` (retried), a second "Work has started" comment is not posted. A `github_last_progress_comment_status` field (string, optional) on the BacklogItem records the last status for which a progress comment was successfully posted; comments are skipped when the current transition matches the stored value.

### AC-9: All new forward-sync paths respect existing loop-prevention
Any comment or state update written to GitHub via the new progress sync paths must update `github_synced_issue_updated_at` using the GitHub API's returned `updated_at` for the issue, same as the existing done-transition path, to avoid backward-sync re-ingesting the change.
