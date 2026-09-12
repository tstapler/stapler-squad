# ADR-004: Per-Item `StageConfigSnapshot` Is Derived From `BacklogStatusEvent`, Not a New `BacklogItem` Column

**Status**: Accepted
**Date**: 2026-09-11
**Review**: Adversarial review (2026-09-11) found the Decision below's original draft omitted
`PendingGates` from the fallback plumbing and undercounted write sites — both are folded into the
Decision as written now; see "Revisions from review" at the end.
**Deciders**: Tyler Stapler (via SDD Phase 3 follow-on, `backlog-custom-workflow-stages`)
**Related**: `docs/adr/013-workflow-engine-replaces-valid-transitions.md` (ADR-013), `ADR-002-configured-workflow-engine-and-gates.md` (the `WorkflowEngine` this closes a gap in)

---

## Context

Epic 2.5 (Story 2.5.1) added `BacklogStatusEvent.stage_name_snapshot`, frozen at transition-write
time so item-detail history keeps rendering a deleted custom stage's original name. Story 2.5.2 added
the *mechanism* for the other half — `ConfiguredWorkflowEngine.CanTransition`/`AllowedTransitions`
now accept an optional `*StageConfigSnapshot{StageName, AllowedTransitions}` fallback, consulted only
when the item's current stage is absent from the live cache (deleted) — but nothing yet constructs a
real snapshot or passes one in. An item parked on a deleted custom stage today gets an empty
`AllowedTransitions` result and a hard `CanTransition` `false`, not the frozen behavior Story 2.5.2's
acceptance criteria promise.

The open question: where does a per-item snapshot actually live, and who builds it?

## Decision

**No new `BacklogItem` column.** The snapshot is derived on demand from the item's own
`BacklogStatusEvent` history, which `GetBacklogItem` already eager-loads
(`BacklogItemData.StatusEvents`, `session/ent_repository_backlog.go`):

1. Add a second snapshot field, `allowed_transitions_snapshot` (`field.JSON`, `Optional`), to the
   `BacklogStatusEvent` ent schema — a sibling to `stage_name_snapshot`, not a replacement. Populate
   it at the exact same write site (`resolveStageNameSnapshot`'s caller in
   `session/ent_repository_backlog.go`), querying enabled `stage_transition` rows
   `WHERE from_stage_id = <destination stage's id>` and mapping `to_stage_id` back to slugs — the
   same data `stageConfigCache` already loads for the live engine, queried directly via the raw ent
   client already in scope there (mirrors `resolveStageNameSnapshot`'s own raw-query style rather than
   threading a `StageConfigRepository` dependency into `EntRepository`, which owns transition writes
   but has no existing relationship to the stage-config layer).
2. Add `buildStageConfigSnapshotFallback(item *BacklogItemData) *session.StageConfigSnapshot`: scans
   `item.StatusEvents` (already loaded, no new query) for the most recent event where
   `ToStatus == item.Status`, and builds the fallback from that event's two snapshot fields. Returns
   `nil` if no matching event exists (e.g. `TransitionBacklogItemStatus` predates this ADR) — `nil` is
   already `ConfiguredWorkflowEngine`'s "no fallback" case, so this degrades to today's behavior, not
   a crash.
3. Call it and pass the result as the variadic fallback argument at every RPC-layer call site that
   invokes `CanTransition`/`AllowedTransitions`/`PendingGates` against a possibly-deleted stage:
   `TransitionBacklogItemStatus`, `OverrideVerdict`, `AttachSessionToItem`
   (`server/services/backlog_service_lifecycle.go`), and `GetPendingGates`
   (`server/services/backlog_service_gates.go`).
4. **`PendingGates` also gains the `fallback ...*StageConfigSnapshot` parameter** (`CanTransition`/
   `AllowedTransitions` already have it; `PendingGates` currently does not — this was the review's
   first finding, folded in here rather than left as a follow-up). Its cache-miss branch today
   (`if !ok || len(edge.Gates) == 0 { return nil, nil }`) silently reports *zero pending gates* for a
   from-stage absent from the live cache — i.e. `ValidateGates` (a thin wrapper over `PendingGates`)
   would let an item on a deleted/disabled stage sail through every gate, inverting this project's own
   documented fail-closed-for-gates rule (plan.md's Pattern Decisions). Fix: when the live cache has
   no edge for `(item.Status, to)` but a supplied fallback's `AllowedTransitions` contains `to` (i.e.
   this edge legally existed when the item entered its current stage), return a single synthetic
   blocking `GateStatus{GateID: "stage-config-unresolvable", Kind: GateKindStructural, Satisfied:
   false, Description: "This item's current stage configuration is no longer available (the stage may
   have been deleted or disabled) — the transition to <to> cannot be automatically evaluated.",
   ActionHint: "An operator must restore or reconfigure the stage in Stages settings, or force this
   transition via Manual Override."}` — never a silent pass. When the fallback doesn't contain `to`
   either (never a legal edge), preserve today's `nil, nil` ("edge doesn't exist" — `CanTransition`
   already governs legality separately, unchanged).
5. **Write-site coverage is 5 call sites, not 4** (the review's second finding): in addition to the 4
   in `session/ent_repository_backlog.go` that Story 2.5.1 already touched
   (`TransitionBacklogItemStatus`, `TransitionBacklogItemStatusWithPRFields`, `ArchiveBacklogItem`,
   `UnarchiveBacklogItem`), `session/storage_backlog.go`'s `ReconcileStuckItems` also calls
   `recordStatusEvent`/`resolveStageNameSnapshot` inside its own transaction and must populate
   `allowed_transitions_snapshot` the same way — missing it would leave auto-transitioned items'
   events silently null for this field.

## Rationale

- **Reuses Story 2.5.1's infrastructure instead of parallel-building.** The write site, the "resolve
  a stage's live config at transition time" query shape, and the eager-load path all already exist;
  this ADR extends them by one field and one helper rather than introducing a second persistence
  mechanism.
- **No migration risk to `BacklogItem` itself.** `BacklogItem` is the hottest-read table in this
  schema (every list/board/detail query touches it); `BacklogStatusEvent` is an append-only,
  per-transition log already scoped to exactly the audit-trail concern this feature is about.
- **Correct by construction for the "which snapshot" question.** A `BacklogItem` column would need
  its own invalidation story (when does it get overwritten — every transition? Only into a custom
  stage?). Deriving from "the most recent event that entered my current status" is unambiguous and
  self-maintaining: it's automatically the event for the item's current status because
  `TransitionBacklogItemStatus` and the event write are the same transaction-adjacent code path.

## Alternatives Considered

**Alt A: New `BacklogItem.stage_config_snapshot` JSON column, updated at every transition write.**
Rejected — adds a migration and a write to the hottest table for data that's already fully
reconstructable from `BacklogStatusEvent`, and duplicates rather than reuses Story 2.5.1's write
site.

**Alt B: Thread `StageConfigRepository`/`ConfiguredWorkflowEngine` into `EntRepository` so the
snapshot-write query reuses the exact same resolved-edge cache the engine maintains.** Rejected for
this ADR's scope — `resolveStageNameSnapshot` already established the raw-ent-query precedent one
epic ago; introducing a new cross-layer dependency (`EntRepository` → stage config layer) to save one
duplicated `WHERE` query is a bigger change than the problem warrants. Worth revisiting only if the
duplicated query logic drifts from the cache's in practice.

**Alt C: Compute the fallback lazily inside `ConfiguredWorkflowEngine` itself (given a repository
reference), rather than building it at the RPC layer and passing it in.** Rejected — `WorkflowEngine`
is a stateless-per-call interface today; giving it a hidden second data source (an item's own audit
log) it queries internally breaks the "engine is pure function of (item, live config)" model the
existing variadic-fallback-parameter design already established in Story 2.5.2. Passing the snapshot
in keeps that seam explicit at every call site.

## Consequences

**Positive**: closes Story 2.5.2's actual acceptance criterion end-to-end; zero new tables; the
fallback is naturally present for every item transitioned after this ADR ships, with graceful `nil`
degradation for older rows.

**Negative**: `buildStageConfigSnapshotFallback` is now duplicated logic at 4 call sites (one small
helper function, not 4 copies of the scan) — acceptable, matches this codebase's existing
per-RPC-handler composition style over a shared middleware layer.

**Neutral**: an item that transitions through several custom stages accumulates one
`allowed_transitions_snapshot` per `BacklogStatusEvent` row, not just the latest — slightly more
storage than the minimum, but consistent with the append-only audit-log's existing design and matches
`stage_name_snapshot`'s same per-row footprint.

---

## Revisions from review

Adversarial review (2026-09-11) confirmed the core decision (derive from `BacklogStatusEvent`, no new
`BacklogItem` column) but found the initial draft would have shipped a fail-open gate bug: passing the
fallback into `CanTransition`/`AllowedTransitions` but not `PendingGates` means `ValidateGates`
(a thin wrapper over `PendingGates`) would silently report zero pending gates — not blocked, not
gate-checked at all — for an item on a deleted/disabled stage, inverting this project's own
fail-closed-for-gates rule. Decision steps 4-5 above (a synthetic blocking `GateStatus` when
`PendingGates` can't resolve the live edge but the snapshot confirms it once existed, plus the 5th
write site at `ReconcileStuckItems`) are the fix, folded into the accepted decision rather than left
as a follow-up.
