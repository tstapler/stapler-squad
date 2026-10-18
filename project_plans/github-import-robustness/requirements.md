# Requirements: github-import-robustness

## Source

Backlog item: "Can you help me make sure that we have a more robust import process for GitHub?"

## Problem Statement

The GitHub issue import dialog (`GitHubIssuePicker`) has three interconnected robustness gaps:

1. **No upfront deduplication in the picker**: `ListGitHubIssues` returns all GitHub issues without indicating which are already imported as backlog items. The user only discovers an issue is already imported after clicking "Import," getting an "Already imported — no new items created" toast. This is backwards — the dialog should filter or visually mark already-imported issues before the user commits.

2. **Issues are not marked on GitHub when picked up**: When a backlog item is created from a GitHub issue (either via `ImportGitHubIssue` or via the scheduled `SyncLoop`), nothing is written back to GitHub to mark the issue as claimed. Other Stapler Squad instances on other hosts — or any human — scanning the issue list would have no signal that this issue is already being worked.

3. **Agents do not reliably close linked GitHub issues**: The forward-sync subscriber (`StartBacklogGitHubForwardSyncSubscriber`) closes the linked GitHub issue when a backlog item reaches `done`, but this only fires when `source.ForwardSyncEnabled == true`. Issues imported manually via `ImportGitHubIssue` (not via an ItemSource sync) have no source at all, so the forward-sync subscriber silently skips them. Additionally, the subscriber only triggers on `done`; it does not trigger on `archived`, meaning abandoned items leave their GitHub issues permanently open.

## Acceptance Criteria

### AC-1: Import picker hides already-imported open issues by default

When `ListGitHubIssues` is called and `state=open`, the response annotates each `GitHubIssueEntry` with `already_in_backlog: bool`. The `GitHubIssuePicker` UI filters out issues where `already_in_backlog == true` from the default list. A toggle ("Show already imported") reveals them, grayed out and non-selectable.

### AC-2: Imported issues are annotated with a "claimed" GitHub label

When an issue is created as a backlog item (via `ImportGitHubIssue` RPC or `SyncOne` upsert), a configurable label (default: `"stapler-squad"`) is applied to the GitHub issue via `PATCH /repos/{owner}/{repo}/issues/{number}`. The label is applied as a best-effort fire-and-forget — failure to apply it is logged at Warn level but does not fail the import. The label name is configurable per-source in the plugin config (`claim_label` field).

### AC-3: Claimed label filters the import dialog

The `ListGitHubIssues` backend filters out (or marks) issues that already carry the configured `claim_label` from the response by default. This is a second dedup layer that catches issues claimed by *other* Stapler Squad instances (or other tools using the same label) that are not in this instance's local backlog.

### AC-4: Agents close linked GitHub issues on done

The forward-sync subscriber closes the GitHub issue linked to a backlog item when the item transitions to `done`, regardless of whether the item has a `SourceID` (i.e., also for items imported via `ImportGitHubIssue` with no backing `ItemSource`). For items without a source, the subscriber derives the close credentials from the GitHub keychain token for the issue's host.

### AC-5: Forward-sync also fires on archived

The forward-sync subscriber additionally fires when a backlog item transitions to `archived` (not just `done`). For archived items, the GitHub issue is closed with a different comment: `"Closed automatically — the linked backlog item was archived in stapler-squad."` Items that transition directly to archived without a PR being opened or work being done should still not leave their issues perpetually open.

### AC-6: Already-imported label is removed on archive/done if not merging

If the `claim_label` was applied at import time and the item is later archived (abandoned), the label is removed from the GitHub issue so other instances can pick it up. If the item is done (PR merged), the label is left in place (the issue is closed anyway, so it no longer appears in the open-issues picker).

## Out of Scope

- Multi-host gossip / cross-host claim dedup (covered by `cross-host-claim-dedup` plan).
- Label-to-priority mapping changes.
- Linear or JIRA claim-label equivalents (future follow-up).
- `BackwardSync` behavior (covered by `backlog-github-two-way-sync` plan).
