# ADR-025: Durable Ship-Snapshot Fields Land Directly on `BacklogItem`, Not a New Ent Entity

**Status**: Accepted
**Date**: 2026-07-18
**Project**: unified-vcs-widget

**Promoted from**: `project_plans/unified-vcs-widget/decisions/ADR-002-ship-snapshot-fields-on-backlog-item.md`

**Note**: `project_plans/unified-vcs-widget/implementation/plan.md` amends this decision with a 6th field, `shipped_snapshot_capture_failed` (a dedicated boolean, distinct from `shipped_check_conclusion`), per an architecture-review BLOCKER finding — see plan.md's Story 3.1.2 and Pattern Decisions table for the amendment. This ADR's original 5-field decision is preserved below as accepted history.

## Context

The requirements' central backend gap: GitHub PR/CI/review state and per-file diff stats are never durably persisted (`session/storage.go:524-529`'s `UpdateInstancePRStatus` is a documented no-op; `session/ent/schema/backlog_item.go` has only `pr_url`/`pr_number`). Architecture research (`research/architecture.md` §2) proposed two shapes:

1. Add 5 new optional fields directly to the existing `BacklogItem` ent schema (`session/ent/schema/backlog_item.go`).
2. Add a new child entity (e.g. `GitHubSnapshot`) with a required unique edge back to `BacklogItem`, mirroring `session/ent/schema/diffstats.go`'s edge-to-`Session` pattern.

`diffstats.go` (`session/ent/schema/diffstats.go`) is a real precedent for option 2 — but it models a genuine one-to-many relationship in principle (multiple diff-stat rows could exist per session). The ship snapshot this project needs is strictly **one-to-one** with `BacklogItem`: a backlog item ships at most once (per PR cycle), the snapshot is written once at the `pr_pending → done` transition, and read back by the same RPC that already loads the `BacklogItem` row (`GetBacklogItemShipStatus`, `server/services/backlog_service_ship_status.go`).

## Decision

Add 5 new optional fields directly to `session/ent/schema/backlog_item.go`:

```go
field.String("shipped_check_conclusion").Optional(),
field.Int("shipped_approved_count").Optional().Default(0),
field.Int("shipped_changes_req_count").Optional().Default(0),
field.Time("shipped_snapshot_at").Optional().Nillable(),
field.String("shipped_file_stats").Optional().
    Comment("JSON []ShippedFileStat{Path,Status,Additions,Deletions} — per-file diff stats captured at ship time"),
```

`shipped_file_stats` is a JSON-blob string column (mirroring the existing `acceptance_criteria` JSON-blob field already on the same entity, `session/ent/schema/backlog_item.go:28`), not a child table — a bounded, write-once-then-rarely-updated list does not earn a new entity/edge per `.claude/rules/interface-pollution-checklist.md` smell #6.

Regenerate with `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` per `.claude/rules/ent-schema-generation.md` — required because the write path (`CaptureShipSnapshot`) is an upsert on the existing `BacklogItem` row (via `BacklogItemUpdate`, the same partial-update struct already used at `session/backlog_lifecycle.go:123-126` for clearing PR fields), not a new-row insert requiring `OnConflictColumns`.

## Consequences

- No new ent entity, no new edge, no join needed to read the snapshot — `GetBacklogItemShipStatus` already loads the `BacklogItem` row; the new fields ride along for free.
- `BacklogItemUpdate` (`session/repository.go:439-460`) gains 5 new optional pointer fields, following its existing pattern.
- If a future requirement needs *multiple* historical snapshots per item (e.g. re-shipped after a hotfix, want history of each), this decision must be revisited — the 1:1 field-on-entity shape only supports "most recent snapshot," which matches this project's explicit scope ("as of merge time," not a history).
- `session/ent_repository_backlog.go`'s existing `BacklogStuckState` upsert pattern (`OnConflictColumns(...).Update(...)`, line ~659) is available as a precedent if a later change needs true upsert-with-conflict semantics; this project's write path uses the simpler existing `UpdateBacklogItem` partial-update call since it's always updating a known, already-loaded row (no create-or-update branching needed).

## Alternatives Considered

- **New `GitHubSnapshot`/`VcsSnapshot` ent entity with a required unique edge to `BacklogItem`** (mirroring `DiffStats`): rejected — the relationship is 1:1 and write-once, unlike `DiffStats`' 1:many-in-principle shape; a new entity/edge/join is a layer that doesn't earn its place for this cardinality.
