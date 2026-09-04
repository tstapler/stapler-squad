# ADR-002: One Generalized Forward-Sync Subscriber, Not Three Per-Tracker Subscribers

## Status
Accepted

## Context

`server/services/backlog_github_forward_sync.go`
(`StartBacklogGitHubForwardSyncSubscriber`) is the existing pattern for
"on session completion, update the external tracker": subscribe to
`EventBus`, filter for `EventBacklogItemChanged` /
`BacklogChangeStatusTransition` → `NewStatus == done`, re-fetch the item by
ID (not the event payload — a documented stale-payload gotcha), look up its
`ItemSource`, check `ForwardSyncEnabled`, type-assert the resolved plugin
against a narrow capability interface, call the tracker-specific
close/comment methods, persist the ADR-003 watermark using the tracker's own
post-write timestamp.

Per architecture.md §4/§8, every step of that pipeline except the
type-asserted capability call is **identical** across GitHub, Linear, and
JIRA — same event filter, same re-fetch-by-ID reasoning, same
`ForwardSyncEnabled` gate, same watermark-write shape (reusing
`GitHubSyncedIssueUpdatedAt` per ADR-001). Only "how do I tell the tracker
this issue is done" differs:
- GitHub: `CloseIssue(ctx, config, externalID, existingLabels, closeLabel) (time.Time, error)`
- Linear: `UpdateIssueState(ctx, config, externalID, targetLabel string) (time.Time, error)`
  (workflow-state-by-name resolution)
- JIRA: `TransitionIssue(ctx, config, externalID, targetLabel string) (time.Time, error)`
  (GET-transitions → name-match → POST-transition dance, §2 of pitfalls.md)

All three also implement `PostIssueComment(ctx, config, externalID, body string) error`.

Two options:
1. **Three near-duplicate subscriber files** — `backlog_linear_forward_sync.go`,
   `backlog_jira_forward_sync.go`, each copy-pasting
   `backlog_github_forward_sync.go`'s ~130 lines of dispatch logic and
   changing only the type-asserted interface and struct/const names. Three
   more `Start...Subscriber` calls in `server/server.go`.
2. **One generalized subscriber** — a single
   `StartBacklogForwardSyncSubscriber` that runs the identical dispatch
   pipeline once, and inside the "call the tracker-specific method" step,
   type-asserts the resolved plugin against whichever capability interface
   it satisfies (`externalIssueCloser` for GitHub's binary-close shape, a new
   `externalIssueStateUpdater` for Linear/JIRA's parameterized-target-state
   shape), falling through with a no-op log line if neither matches (same
   behavior `GitHubPRsPlugin` already gets today from the single-tracker
   subscriber).

## Decision

**Generalize into one subscriber.** Rename/refactor
`server/services/backlog_github_forward_sync.go` into
`server/services/backlog_forward_sync.go` (or keep the filename and update
its header comment — implementation detail for plan.md's task breakdown),
exporting `StartBacklogForwardSyncSubscriber` as the single entry point.
`server/server.go` gets exactly one subscriber-start call (replacing today's
`StartBacklogGitHubForwardSyncSubscriber` call), not three.

Rationale:
- The dispatch pipeline is byte-identical per tracker today (confirmed by
  reading `handleForwardSyncClose`'s body — every line up to the
  `closer.CloseIssue(...)` call is tracker-agnostic already). Triplicating
  it means the re-fetch-by-ID fix, the `ForwardSyncEnabled` gate, and the
  ADR-003 watermark-write logic each have three copies to keep in sync —
  the exact "near-duplicate subscriber goroutines" cost architecture.md §8
  flagged as worth avoiding.
- A future bug fix or behavior change to the shared dispatch logic (e.g. a
  pre-close "is it already done" check, noted as a deferred P2 in the
  existing file's doc comment) lands once, not three times.
- The two capability interfaces stay narrow and consumer-defined
  (`.claude/rules/interface-pollution-checklist.md`) — this ADR does not
  widen `externalIssueCloser`'s existing signature (that would touch
  GitHub's already-shipped, already-tested plugin for no reason); it adds a
  second, separate interface for the shape Linear/JIRA actually need. See
  plan.md's Pattern Decisions table row 3.

## Consequences

- `handleForwardSyncClose` is renamed to something tracker-neutral (e.g.
  `handleForwardSync`) and its final step becomes a two-way type-switch:
  ```go
  switch p := plugin.(type) {
  case externalIssueCloser:
      issueUpdatedAt, err = p.CloseIssue(ctx, config, current.ExternalID, current.Labels, source.ForwardSyncCloseLabel)
  case externalIssueStateUpdater:
      issueUpdatedAt, err = p.UpdateIssueState(ctx, config, current.ExternalID, source.ForwardSyncCloseLabel)
  default:
      log.Info("backlog_forward_sync: plugin does not support forward sync, skip", "plugin_id", source.PluginID, "item", current.ID)
      return
  }
  ```
  followed by the same `PostIssueComment` + watermark-write logic today's
  code already has, called once regardless of which branch fired (both
  interfaces are required to also implement `PostIssueComment`, expressed as
  a third, separately-checked interface or by requiring both
  `externalIssueCloser`/`externalIssueStateUpdater` to embed a shared
  `PostIssueComment` method — plan.md's Story 2.2.1 finalizes the exact Go
  shape).
- `server/server.go`'s wiring comment updates from "GitHub forward-sync
  subscriber" to a tracker-neutral description; only one
  `deps.BacklogService != nil` guard block remains (todays', reused).
- Existing GitHub forward-sync tests
  (`TestForwardSyncSubscriber_NoOpWhenPluginDoesNotImplementCloser` etc.)
  move/rename alongside the file but keep asserting GitHub-specific
  behavior; new tests add Linear/JIRA-specific cases against the same
  subscriber entry point (see plan.md Story 2.2.1's task list).
