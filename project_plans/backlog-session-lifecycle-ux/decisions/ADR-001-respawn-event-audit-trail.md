# ADR-001: Respawn-Event Audit Trail — Embedded Append-Only Log, Not a New RPC or Episode History

**Status**: Accepted
**Date**: 2026-08-01
**Project**: backlog-session-lifecycle-ux

## Context

`AutoRespawnReview`, `AutoRespawnAutonomousWork`, `AutoRespawnTriage`, and `RemediateStaleWorkSession` (`server/services/backlog_service_triage.go`) each automatically respawn or remediate a backlog item's session, but today only write to `log.InfoLog` — there is no durable, queryable record of "session N was (re)spawned because of reason X, resulting in session M." `BacklogStuckState` tracks a live counter (`remediation_attempts`) and next-eligible-retry timestamp but is deliberately resolve-in-place (per prior ADR) and retains no per-attempt history or reason.

Two architectural questions needed resolving before task breakdown:
1. How should a client read an item's respawn history — a new RPC, or embedded on the existing `BacklogItem` message?
2. Should this reuse/extend `BacklogStuckState`'s existing counters, or be a wholly separate table?

## Decision

1. **New, separate, append-only `RespawnEvent` ent entity** (`session/ent/schema/respawn_event.go`), one row per respawn attempt: `id`, `item_id`, `reason`, `triggering_session_uuid` (optional), `resulting_session_uuid` (optional), `created_at` (immutable). Session references are plain strings, not ent edges/FKs — mirroring `ItemSession.session_uuid`'s existing "loose FK, not an edge" convention, and tolerating the case where a respawn attempt never produces a resulting session (e.g. it hits the concurrency cap and queues instead of spawning).
2. **Embedded on `BacklogItem.respawn_events`** (`repeated RespawnEvent respawn_events = 30;`), eagerly loaded only by `GetBacklogItem` (the item-detail fetch), not by the board-list `ListBacklogItems` — exactly mirroring how `status_events`/`progress_notes` are already embedded and scoped today. No new RPC is introduced.
3. **Read path is bounded**: the eager-load query caps to the 50 most recent rows (`ORDER BY created_at DESC LIMIT 50`, reversed to ascending before returning), independent of the frontend's own `useShowMore(cap=8)` display cap. This is a explicit response to a real growth risk (this instance saw 15+ restarts/day and 6-item respawn sweeps after a single restart during BUG-053 investigation), not a hypothetical one.
4. **Writes are best-effort**: `Storage.CreateRespawnEvent` logs and swallows DB failures rather than propagating them — a respawn that already succeeded must never be reported as failed merely because its audit row failed to insert. This matches `recordStatusEvent`'s existing, already-shipped contract.
5. **`BacklogStuckState` is untouched** — this is a deliberately separate concern. `BacklogStuckState.remediation_attempts` remains a live counter for gating *decisions* (should we retry, are we parked); `RespawnEvent` is a historical *record* of what was actually tried and why. Conflating the two (e.g. adding a reason column to `BacklogStuckState` and turning it append-only) would violate the resolve-in-place design a prior ADR established for that table, and mix two different lifecycles (a stuck-state row's lifecycle ends at resolution; a respawn-event row's lifecycle never ends).

## Alternatives Considered

- **Separate `ListRespawnEvents(item_id)` RPC, paginated.** Rejected: invents a new RPC shape and frontend fetch/loading-state path for a dataset (tens of events per item) that doesn't need pagination at this scale — an unjustified generic per `.claude/rules/interface-pollution-checklist.md`. If per-item volume ever grows past what a capped eager-load can serve cheaply, revisit this.
- **Add a `kind` discriminator to `BacklogStatusEvent`, reuse that table for respawn events too.** Rejected: conflates two audit concerns with different field shapes into one nullable-everything table, and directly risks the "episode history vs. resolve-in-place" conflation the requirements' Rabbit Holes section explicitly calls out to avoid.
- **Hard FK / ent edge from `RespawnEvent` to `ItemSession`.** Rejected: `ItemSession` rows are already handled as a loose string reference elsewhere in this schema family (`session_uuid`), and a hard FK would force the write path to handle "resulting session doesn't exist yet" (a respawn attempt that queued or failed before spawning) as a referential-integrity special case rather than simply leaving the field unset.

## Consequences

- One new table (`respawn_events`), migrated automatically via ent's existing auto-migrate-on-boot path — no manual SQL migration, no schema-change-detection tooling needed beyond what already exists for every other ent schema in this repo.
- `GetBacklogItem`'s response payload grows by one more array field; negligible at "tens of events, tens of items" scale, and empty/omitted for the common case (an item that never needed remediation).
- The 4 respawn call sites each gain one additional (best-effort) DB write, positioned after their respective spawn/trigger attempt completes — no new lock contention (independent `INSERT`, no read-modify-write), per pitfalls.md's confirmation that this codebase's existing TOCTOU guards (`sync.Map`-based in-flight trackers) are unaffected.
- If respawn volume per item ever exceeds what the 50-row cap usefully represents (not expected at this project's scale), the next step would be a dedicated paginated RPC — deferred, not built speculatively now.
