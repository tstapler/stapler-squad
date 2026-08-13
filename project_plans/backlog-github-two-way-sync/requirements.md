# Requirements: Two-Way Linkage + Status/Label Sync Between Backlog Items and GitHub Issues

Source: backlog item `be676dab-6798-410e-a3ec-708fc03758e0`
("Two-way linkage + status/label sync between imported backlog items and their GitHub issue counterparts")

Generated non-interactively (SDD pipeline mode, no user present) directly from the
item's title/description/acceptance criteria.

## Problem

`GitHubIssuesPlugin` (`session/backlog_plugin_github.go`) is pull-only and lossy:

1. `Fetch` only queries `state=open` (`backlog_plugin_github.go:90`) — an issue closed
   on GitHub is never observed by the sync loop, so its backlog item can sit in the
   board indefinitely after the source issue is resolved.
2. `MapToBacklogItem` (`backlog_plugin_github.go:166-185`) receives `item.URL` and
   `item.Labels` (both already populated in `Fetch`) but drops both — `BacklogItemData`
   (`session/repository.go:346`, actual struct starts ~line 396 per current grep) has
   no field for either, so the issue's own HTML URL and labels never reach storage.
3. `SyncLoop.SyncOne` (`session/backlog_sync.go:277-278`) explicitly never touches
   `Status` on an existing item — comment: "Status transitions are only done via
   TransitionBacklogItemStatus — no update here." There is no back-sync path in
   either direction:
   - Shipping/closing a backlog item does not close or label the originating issue.
   - Closing or labeling the GitHub issue elsewhere does not update the backlog item.
4. No UI surface (card or detail view) shows `external_id`/source linkage on the
   item itself — only Settings > Backlog Sources references sync config, not
   individual item provenance.

## Related prior work (found during triage, not yet implemented)

`project_plans/backlog-github-issue-link/` is a **fully planned but unimplemented**
prior SDD project (requirements/research/plan/ADR-001 all exist) that already
designs the exact mechanism needed for problem #2's URL half: adding
`ExternalURL *string` to `BacklogItemData`/`BacklogItemUpdate`/the ent schema, with
a bounded backfill via `UpdateBacklogItem`, and surfacing it into
`BuildSessionInitialPrompt`. Verified in code: `ExternalURL` does not exist anywhere
in `session/repository.go` today, so that plan was never executed. This project
should reuse/extend that design (cite ADR-001) rather than re-deriving the
URL-persistence approach from scratch — research phase should read that ADR fully
before proposing a schema.

## Goal

A closer-to-real two-way integration:
- An imported backlog item visibly shows where it came from (source link, labels,
  imported badge) on the card/detail view.
- Status and label changes can flow in both directions under user control:
  shipping/closing a backlog item can close (and optionally label) the GitHub issue;
  closing/labeling the GitHub issue elsewhere can reflect back onto the backlog item.
- Closed issues are eventually observed (not just `state=open`), so backlog items
  don't go stale relative to their source.

## Acceptance Criteria (initial — refined further in plan.md)

0. `GitHubIssuesPlugin.Fetch` observes closed/reopened issues, not just open ones
   (e.g. query `state=all` or otherwise detect the closed transition), and threads
   issue state through `ExternalItem` to the sync loop.
1. `BacklogItemData`/`BacklogItemUpdate`/ent schema persist the issue's `ExternalURL`
   and `Labels`; `MapToBacklogItem` populates both instead of dropping them.
2. The backlog item card and/or detail view show source provenance: an
   "imported from GitHub" indicator, a clickable link to the originating issue, and
   its labels.
3. A user-controlled setting (per source, default off) enables forward sync:
   transitioning a backlog item to done/shipped closes the linked GitHub issue
   (and optionally applies a configured label), via the existing GitHub API auth
   already used for fetch.
4. A user-controlled setting (per source, default off) enables backward sync:
   `SyncLoop.SyncOne` applies the source issue's closed/open state and label changes
   onto the backlog item's status/labels, respecting the existing local-wins
   `UserModifiedFields` guard so a user's own status/label edits aren't clobbered.
5. Sync direction settings are configurable in Settings > Backlog Sources (the
   existing config surface referenced in the description).
6. Existing already-imported backlog items are backfilled with `ExternalURL`/
   `Labels` on first sync after this ships (bounded, matching ADR-001's backfill
   approach), not left null forever.
7. No infinite sync loop: an issue closed by our own forward-sync write is not
   immediately re-processed by backward-sync as an external change (or if it is,
   it's idempotent and doesn't thrash status).

## Non-goals

- Two-way sync for the `github_prs` source plugin (out of scope; issues only,
  per the item's title and description).
- Real-time push (webhooks) — sync remains on the existing poll-based `SyncLoop`
  cadence.
- Syncing arbitrary custom fields beyond status/labels/URL.

## Constraints / Conventions to honor

- `.claude/rules/ent-schema-generation.md` — any ent schema change (new fields)
  must regenerate via `go run -mod=mod entgo.io/ent/cmd/ent generate --feature
  sql/upsert ./session/ent/schema`.
- `.claude/rules/feature-registry.md` — new RPCs/UI features need
  `docs/registry/features/*` entries + `make registry-generate`.
- `.claude/rules/prefer-go-git-over-subshells.md` — not directly applicable (GitHub
  REST API, not local git), but keep in mind if any git operations are touched.
- CSS: new UI must follow `.claude/rules/css-architecture.md` (vanilla-extract).
- Backward-sync writes must still respect `UserModifiedFields` local-wins semantics
  already established in `SyncLoop.SyncOne`.
