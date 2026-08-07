# ADR-001: Reuse `GitHubSyncedIssueUpdatedAt` As-Is for Linear/JIRA's Watermark

## Status
Accepted

## Context

`BacklogItemData.GitHubSyncedIssueUpdatedAt` (`session/repository.go:447`) /
`BacklogItemUpdate.GitHubSyncedIssueUpdatedAt` (`session/repository.go:594`),
backed by the ent column `github_synced_issue_updated_at`
(`session/ent/schema/item_source.go`'s sibling `backlog_item.go` schema,
generated into `session/ent/backlogitem/backlogitem.go`), is the
loop-prevention watermark ADR-003
(`project_plans/backlog-github-two-way-sync/decisions/ADR-003-loop-prevention-watermark-design.md`)
designed. It stores "the last external-tracker `updated_at` we've already
processed for this item's forward-sync," compared in
`session/backlog_sync.go:445,496` and written in
`server/services/backlog_github_forward_sync.go:166`.

The name is GitHub-specific in both Go and the DB column. requirements.md's
constraints section explicitly asks this pattern be "replicated" for
Linear/JIRA and flags the naming mismatch as a decision point for plan.md
(architecture.md §4, features.md §"Unstated needs" #2) rather than deferring
it silently.

Two options:
1. **Rename** to a generic name (`SourceSyncedItemUpdatedAt` /
   `source_synced_item_updated_at`) via an ent migration. Touches: the ent
   schema field, all generated `session/ent/*` accessors (~8 files per
   `grep -rl GithubSyncedIssueUpdatedAt session/ent`), `session/repository.go`
   (2 struct fields), `session/ent_repository_backlog.go` (3 call sites),
   `session/backlog_sync.go` (2 call sites), the (generalized, per ADR-002)
   forward-sync subscriber (1 call site), plus every existing test that
   references the field name.
2. **Reuse as-is.** Zero schema change, zero migration, zero touched
   call sites beyond what the feature needs anyway. Misleading name for
   non-GitHub sources.

## Decision

**Reuse `GitHubSyncedIssueUpdatedAt` as-is for Linear and JIRA's
loop-prevention watermark too.** No ent migration, no rename, in this
project.

Rationale:
- Each `BacklogItem` has exactly one `SourceID` (one tracker origin) — the
  field's *meaning* ("watermark for whichever external tracker this item
  came from") is already correct regardless of the literal Go/column name.
  There is no correctness risk, only a naming-clarity one.
- A rename touches ~15 files across three layers (ent schema/generated code,
  repository layer, sync-loop, forward-sync subscriber) purely for
  naming clarity, with zero behavior change — a large diff for a project
  whose actual functional scope (two new plugins, one subscriber
  generalization, frontend badge/filter work) is already substantial.
  Bundling an unrelated rename into this PR increases review surface and
  regression risk (an ent migration on a production table) for no
  functional benefit.
- This mirrors the project's YAGNI/smallest-diff bias: the misnaming is
  real technical debt, but it is not blocking, not a correctness bug, and
  not growing in cost by being deferred one more project.

## Consequences

- `session/backlog_plugin_linear.go` and `session/backlog_plugin_jira.go`'s
  forward-sync methods (`UpdateIssueState`/`TransitionIssue`) return a
  `time.Time` that the generalized forward-sync subscriber
  (ADR-002) writes into the same `GitHubSyncedIssueUpdatedAt` field via
  `session.BacklogItemUpdate{GitHubSyncedIssueUpdatedAt: &watermark}` —
  identical code path to GitHub's, just conceptually reused for two more
  trackers.
- New code (comments in `backlog_plugin_linear.go`/`backlog_plugin_jira.go`
  at the watermark write site) must explicitly note that the field name is
  historical/generic-in-practice, so a future reader doesn't assume it's
  GitHub-only and skip using it for a Linear/JIRA item.
- **Follow-up flagged, not scheduled**: if a fourth tracker is ever added, or
  if the misnaming causes real confusion in review, revisit this ADR and do
  the rename then — do not let "later" silently become "never" per
  `.claude/rules/fix-flaky-tests-dont-defer.md`'s spirit applied to naming
  debt. No tracking issue filed as part of this project (out of scope); note
  this ADR itself is the record to reference when someone next touches this
  field.
