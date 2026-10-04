# ADR-001: `GuidanceRequest` Is a Standalone Entity, Not a `BacklogStuckState`/`StuckReason` Extension

**Date**: 2026-09-12
**Status**: Accepted

## Context

`durable-guidance-request` needs a durable, resolve-in-place row backing a question/answer cycle. The codebase already has exactly this shape — atomic upsert, unique key, resolve-in-place — in `session/ent/schema/backlog_stuck_state.go` (`BacklogStuckState`, `MarkStuck`/`ResolveStuck`/`MarkStuckNotified` in `session/ent_repository_backlog.go:1901-2014`). The obvious question: extend `BacklogStuckState` with a new `StuckReason` value (e.g. `StuckReasonAwaitingGuidance`) instead of adding a new entity.

## Decision

`GuidanceRequest` is a new, standalone ent entity (`session/ent/schema/guidance_request.go`). We reuse `BacklogStuckState`'s *pattern* (atomic `OnConflictColumns` upsert, plain unique index, resolve-in-place, separate idempotent `notified_at`/answered setters) but not the table itself.

## Rationale

1. **Scope mismatch.** `BacklogStuckState` has a `Required()` edge to exactly one `BacklogItem` (`item_id` is non-nullable). Two of `GuidanceRequest`'s three scopes — `session` and `standalone` — have no owning `BacklogItem` at all. Forcing them through a `BacklogItem`-edged table means either a nullable FK (breaking the existing `Required()` edge and every other `BacklogStuckState` reader that assumes it) or a synthetic placeholder item, both worse than a new table.
2. **Different payload shape.** A `StuckReason` row's terminal state is boolean (`resolved_at` set or not) plus free-text `context`. A `GuidanceRequest` needs a typed `answer` value, an `question_type` (yes-no/multiple-choice/short-answer), and `options` (JSON-in-string for multiple-choice) — fields meaningless for every existing `StuckReason` value and that would pollute `BacklogStuckState`'s schema for all other consumers (remediation, dashboards).
3. **Precedent**: this codebase already made this exact call once — `InfraIssueReport` was added as its own entity rather than folded into `StuckReason`, for the same "different scope + different payload" reasoning (ADR-002 in that entity's origin project, cited by `research/architecture.md`).

## Consequences

- One more ent schema file, one more generated package, one more repository file — acceptable, matches existing per-concept-entity granularity (`BacklogStuckState`, `ApprovalRule`, `ReviewVerdict`, `ItemSession` are all separate entities already).
- `GuidanceRequest` cannot reuse `FindOpenStuckStates`/stuck-state dashboards directly; it gets its own query methods (`ListPendingGuidanceRequests`, etc.) — see plan.md Epic 1.1.
- The ownership FK design is net-new (no existing `session/ent/schema/*.go` has an owner-scoped optional FK of this shape) — tracked as its own task, not inherited for free from `BacklogStuckState`. Unlike `BacklogItem`'s other child tables, the `item_id` edge deliberately does NOT cascade-delete: `BacklogItem` supports genuine hard deletion (`DeleteBacklogItem`), and a cascade would silently destroy a pending or answered-but-undelivered `GuidanceRequest` row with no trace, contradicting this feature's own durability bar. The FK is left to go stale on a hard delete instead, and the self-heal sweep (plan.md Task 8.1.1a) treats "item not found" as cancel-worthy, same as "item archived" — see plan.md Task 1.1.1b for the full rationale.

## Alternatives Rejected

| Alternative | Rejected because |
|---|---|
| Extend `BacklogStuckState` with a new `StuckReason` | Cannot represent `session`/`standalone` scope (no `BacklogItem`); no field for typed answer/options; would leak guidance-specific fields into every stuck-state consumer. |
| Extend `PendingApproval`/`ApprovalStore` | In-memory/JSON-file-backed; its only restart-survival behavior is deleting an orphaned pending approval (`Orphaned: true`) — it can never *deliver* an answer through a restart, the opposite of what durable notification requires (confirmed via direct read of `server/services/approval_store.go:75-461`). |
| Reuse `ApprovalRulesPanel`/approval-rule schema | No item/session scoping concept at all; would need the same amount of new schema work anyway. |
