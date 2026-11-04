# ADR-004: Durable session event log as an ent sibling table

**Status**: Proposed. Amended in the Phase 4 patch (recorder is not a single chokepoint for everything; excerpt cap 160; prune on delete only).
**Date**: 2026-10-23
**Project**: attention-inbox-consolidation

## Decision

New ent entity `SessionEvent` in `session/ent/schema/session_event.go`, modeled on `backlog_status_event.go` and the `backlog-item-activity-log` ADR-001 sibling-table pattern. No edge to `Session` (rows survive session deletion, like `SessionSummary`; cleanup is by retention and an explicit prune on session delete; archived sessions keep their rows until retention ages them out, so an archived session's history stays viewable).

Fields: `id` (uuid), `session_id` (string), `session_title`, `kind` (string enum), `from_state`, `to_state`, `reason`, `episode_id` (nullable), `excerpt` (<= 160 chars, scrubbed; the same cap applies to the persisted `turn_end_excerpt`, notification messages and every UI surface, UX-37), `source` (poller | hook | lifecycle | user), `occurred_at`, small `payload` JSON. Indexes: `(session_id, occurred_at)`, `occurred_at`.

Kinds: `StatusChanged`, `TurnEnded`, `AttentionRaised`, `AttentionCleared`, `Acknowledged`, `NotificationSuppressed` (with gate reason), `LifecycleStarted/Exited/Stopped`, `ApprovalResolved`. Notification *sends* are not events (double-count risk P5): the log records state transitions through the flag-gated `SessionEventRecorder`, with exactly one owning writer per event kind (plan.md Event Ownership table: poller result handler, `transitionTo`, lifecycle hooks, the acknowledge handler, `Instance.RecordTurnEnd`, the delivery-gate result handler, the approval handler). The recorder is NOT a single chokepoint for state: the always-on `SessionActivity` (ADR-005) advances `LastMeaningfulChangeAt` independently of `session_event_log`, and the recorder persists event rows only. With `session_event_log` off the recorder is a no-op and no feature other than the history views depends on it.

Write-on-change only (dedup by last kind+reason per session). Retention: 200 rows and 30 days per session, ~50k global, pruned by a sweeper following `server/notifications/store.go` `enforceRetention`; a `retention_dropped` counter; "older events trimmed" marker in the UI. Hidden sessions are logged but never become attention items; their events appear only in the cross-session Recent activity view behind an explicit "Include hidden sessions" toggle (plan Story 5.2.3). `ListSessionEvents` pages with a `before_occurred_at` cursor (plan Task 2.1.3d).

## Alternatives rejected

- Extend `NotificationHistoryStore` (JSON file): collapses duplicates by `(session, type)`, 7-day cap, records deliveries not transitions, no range queries.
- OpenTelemetry as the log: not a queryable per-session store; used only for counters.
- Reuse `BacklogProgressNote`/`BacklogActivityNote`: item-scoped, no session provenance.

## Migration

Additive new table only; ent `Schema.Create()` is idempotent (`session/backlog_stuck_migration_test.go:94-119` proves the pattern). Generated ent code is not committed; regenerate with `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`. Rollback: flip `session_event_log` off; table stays inert.

## Secrets

Structured fields only; excerpts capped and passed through existing redaction if present, otherwise a conservative token/`KEY=` scrubber added in Story 2.1.2 (`session_event_scrub.go`), shared with the notification message builder. No raw terminal tails.
