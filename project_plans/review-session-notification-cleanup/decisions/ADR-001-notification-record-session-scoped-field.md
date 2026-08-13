# ADR-001: Add an Explicit `SessionScoped` Field to `NotificationRecord`

**Status**: Accepted
**Date**: 2026-07-25
**Project**: review-session-notification-cleanup

## Context

AC3 requires pruning `NotificationRecord`s (`server/notifications/store.go`) whose
referenced session no longer exists. The obvious-looking predicate —
"look up `record.SessionID` in the session registry; not found ⇒ prune" — is
unsafe. `NotificationRecord.SessionID` is deliberately overloaded: for
review-queue-originated notifications (`server/review_queue_manager.go`'s
`OnItemAdded`) it holds a real session UUID/title, but for a whole family of
operator-facing notifications — rework-cap-hit, repeated-failure,
spawn-rollback-failed, triage-persist-failure, branch-drift-blocked,
codebase-dir-missing (`server/services/backlog_service_triage.go:145,180,214,239,987,2041,2132`,
via `backlog_notifier.go`'s `EventBusNotifier.Notify`) — `SessionID` is
deliberately set to a **backlog item ID** instead, so the notification
subscriber's coalescing key differentiates between different backlog items.
Both are UUID-formatted and therefore format-indistinguishable. A naive
existence check would silently delete every one of these item-scoped records
on the very first prune pass.

Two candidate fixes were considered:

1. **ID-prefix sniffing** — treat the `review-queue-<sessionID>-<timestamp>`
   prefix used at `review_queue_manager.go:339` as the positive signal.
   Rejected: fragile (a new producer could pick a colliding or differently-shaped
   ID scheme without anyone noticing it needs to be excluded from pruning), and
   requires auditing every existing ID scheme across ~9 `NewNotificationEvent`
   call sites to prove none of them accidentally start with that prefix or a
   look-alike — an audit this ADR would rather not make a permanent, silent
   correctness dependency.
2. **A new explicit field on `NotificationRecord`** (chosen) — a boolean the
   record's producer sets deliberately, so "is this record's `SessionID` a
   real session identifier eligible for existence-checking" is data, not
   inferred from string shape.

## Decision

Add `SessionScoped bool \`json:"session_scoped,omitempty"\`` to
`NotificationRecord` (`server/notifications/store.go`, alongside the existing
`Metadata`/`OccurrenceCount` fields).

The field is populated at the EventBus → persisted-record boundary, not by
widening every `events.NewNotificationEvent(...)` call site's signature (that
would touch ~9 unrelated producers for a distinction only 2 of them need).
Instead:

- The two producers whose `SessionID` genuinely is a session identifier —
  `server/review_queue_manager.go`'s `OnItemAdded` and
  `server/services/autonomous_orchestration_service.go`'s generic
  done/stuck notifier (line ~540) — set `metadata["session_scoped"] = "true"`
  in the `map[string]string` they already pass to `NewNotificationEvent`.
- `server/notifications/subscriber.go`'s `eventToRecord` (the sole function
  that converts an `*events.Event` into a `*NotificationRecord` before
  `Append`) reads that key and sets `record.SessionScoped =
  event.NotificationMetadata["session_scoped"] == "true"`.
- Every other producer leaves the key unset, so `SessionScoped` defaults to
  `false` and is never candidate for the existence check.

`NotificationHistoryStore.PruneOrphaned`'s predicate becomes:

```go
record.SessionScoped && record.Metadata["item_id"] == "" && !exists(record.SessionID)
```

Both `SessionScoped` (correct axis to check) and `item_id == ""` (don't delete
a record that still has a live "View in Backlog" link even if its `SessionID`
happens to be stale) are required — they answer two different questions and
neither alone is sufficient.

## Consequences

- **Persisted-data-shape change, no ent migration.** `NotificationRecord` is
  a plain Go struct serialized to a flat JSON file
  (`~/.stapler-squad/.../notifications.json`), entirely separate from the
  ent/SQLite schema — `.claude/rules/ent-schema-generation.md` does not apply.
  Old records on disk simply decode with `SessionScoped` defaulting to its
  zero value (`false`), which is the safe default (never pruned) — no
  migration script needed.
- Any future producer that wants its notifications eligible for orphan
  pruning must explicitly opt in via `metadata["session_scoped"] = "true"`,
  making the eligibility decision visible at the call site instead of
  inferred.
- `PruneOrphaned` and `eventToRecord` both need small, targeted changes (see
  `implementation/plan.md` Epic 3); no new package dependency is introduced —
  `server/notifications` still does not import `session`, the existence
  predicate is injected as a plain `func(string) bool`.
