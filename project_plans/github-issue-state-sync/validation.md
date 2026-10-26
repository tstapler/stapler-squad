# Validation: GitHub Issue State Sync

## Adversarial Review

### Challenge 1: Is AC-9 (loop prevention) actually enforced by the plan?

**Concern**: If the progress subscriber posts a comment on a GitHub issue and does NOT update `github_synced_issue_updated_at`, the backward sync (which polls for issues updated since the cursor) will re-fetch that issue on the next tick and potentially update the item in ways that trigger another status transition event — causing an infinite loop.

**Plan verdict**: Task 2.2 explicitly requires: "After successful post: update `GitHubLastProgressCommentStatus = NewStatus` and advance `GitHubSyncedIssueUpdatedAt`". The research confirms the existing `CloseIssue` path does this via `GetIssue()` after posting. The new subscriber must follow the same pattern.

**Verdict**: CONFIRMED risk — plan addresses it. Implementation must call `GetIssue()` after every GitHub write to get the updated `updated_at` timestamp. Add this to Task 2.2 implementation notes.

---

### Challenge 2: Does the MCP-import path actually have a local HostID available?

**Concern**: Task 1.3 says "set `ClaimingHostID = localHostID.String()`" in the MCP import handler. But does the MCP tool handler have access to the local host identity?

**Investigation**: In `server/mcp/tools_backlog.go`, the `import_github_issue` tool handler receives `toolCtx` which carries dependencies injected at startup. The `ClaimRecorder` is already injected there (it records the claim). The same `HostIdentity` used by `ClaimRecorder` would be accessible. Look at how other tools access host identity — `tools_backlog_import_tracker.go` and `claim_index.go` show `identity.ID` is available from the injected `HostIdentity` struct.

**Verdict**: PLAUSIBLE — needs to verify in implementation that `HostIdentity` is already plumbed into the MCP tools context (it likely is since the claim recorder needs it). No blocker.

---

### Challenge 3: The `review` status transition — does it reliably have `pr_url` set?

**Concern**: Task 2.2 posts a PR URL comment when `NewStatus == "review"` and `pr_url != ""`. But is `pr_url` guaranteed to be set before or at the same time as the `review` transition fires?

**Investigation**: From `session/backlog_lifecycle.go` and `session/backlog.go`, the `review` status is set by the session driver when a PR is opened. Looking at the event payload (`BacklogItemChangedPayload`), it carries `Item *BacklogItemData` with the item's state at the time of the event. The PR URL is typically set on the item just before or simultaneous with the status transition to `review`. However, there could be a race where the status transitions before the PR URL is written.

**Verdict**: PLAUSIBLE concern. The implementation should re-fetch the item after receiving the event (just like `handleForwardSyncClose` does) to get the most current `pr_url`. The plan already mentions re-fetching — make this explicit in Task 2.2.

---

### Challenge 4: Will adding `claiming_host_id` to the proto break anything?

**Concern**: Proto fields are additive in proto3. Adding optional string fields is backward compatible. Old clients simply ignore the new fields.

**Verdict**: No risk. The plan is correct.

---

### Challenge 5: Feature flag default is `false` — but the done-transition update (AC-5, Task 3.1) is NOT gated

**Concern**: Task 3.1 updates the done-transition comment to include the host ID. This is NOT behind the new `github_issue_progress_comments` flag. Is this intentional?

**Assessment**: Yes — AC-5 is a cosmetic update to an existing behavior (not new behavior), and the done-transition is already gated by `ItemSource.ForwardSyncEnabled`. The host ID addition is pure upside with no new risk. Consistent with the requirements (AC-7 only gates AC-3 and AC-4).

**Verdict**: Correct as designed.

---

### Challenge 6: Duplicate-comment prevention reset on re-opened items

**Concern**: If an item goes `done → idea` (re-opened / archived+unarchived), then `in_progress` again, the `github_last_progress_comment_status` field would still hold `"in_progress"` from the first run, preventing a new comment.

**Assessment**: This is actually the desired behavior in most cases — spamming the GitHub issue with "work has started again" comments on every retry would be noisy. However, the requirements say "reset it if the item is re-activated from done." The plan needs a step to clear `GitHubLastProgressCommentStatus` when the item transitions *back from done* (to any earlier state).

**Gap in plan**: Task 2.2 does not address clearing the watermark on backward transitions. Add a note: clear `GitHubLastProgressCommentStatus` when `NewStatus` is NOT `"in_progress"`, `"review"`, or `"done"` and the previous status was `"done"`. This is a rare edge case — can be deferred to a follow-up if it complicates the initial implementation.

---

### Challenge 7: Sourced items forwarded via existing subscriber vs. new subscriber — double-posting?

**Concern**: For items WITH a `SourceID`, the existing `handleForwardSyncClose` handles `done`. The new subscriber also subscribes to `BacklogChangeStatusTransition`. Could the two subscribers both fire on the same event?

**Assessment**: No overlap — the existing subscriber only handles `NewStatus == "done"`, and the new subscriber handles `"in_progress"` and `"review"`. They do not compete on the same status values.

**Verdict**: No conflict. Correct.

---

## Plan Validation Summary

| AC | Plan Coverage | Notes |
|---|---|---|
| AC-1 | Task 1.1, 1.2, 1.3 | Need to verify HostIdentity plumbing in MCP handler |
| AC-2 | Task 1.4, 1.5, 1.6 | Straightforward proto + UI |
| AC-3 | Task 2.1, 2.2, 2.4 | Must re-fetch item before posting; update `github_synced_issue_updated_at` after |
| AC-4 | Task 2.2 | Must re-fetch item for latest `pr_url`; update `github_synced_issue_updated_at` after |
| AC-5 | Task 3.1 | Correct — not behind flag |
| AC-6 | Task 2.3 | Token resolution helper; silent skip on no-token |
| AC-7 | Task 2.1 | Flag default false |
| AC-8 | Task 2.2 | `github_last_progress_comment_status` field + skip logic; consider reset-on-reopen as follow-up |
| AC-9 | Task 2.2 | Explicitly call `GetIssue()` after each GitHub write to advance watermark |

## Open Questions for Owner

1. **Which GitHub statuses trigger "progress" comments?** The plan covers `in_progress` and `review`. Should `queued`, `planning`, or other statuses also post? Recommending `in_progress` and `review` only to minimize noise.
2. **Comment format**: Should the bot comments use a specific emoji/prefix? The current `forwardSyncCloseComment` does not use emoji; the plan keeps consistency with that plain style for AC-5 and uses plain text for AC-3/AC-4 as well.
3. **Flag default**: Should `github_issue_progress_comments` default to `true` for items that have `ForwardSyncEnabled = true` on their source? Or always off by default?
