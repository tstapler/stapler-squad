# ADR-001: Notification Dedup Collapses Regardless of Read State (AUTO_APPROVED Carve-Out, Retention Keyed on LastOccurredAt)

**Status**: Accepted
**Date**: 2026-09-04
**Project**: notification-revamp

## Context

`NotificationHistoryStore.Append()` (`server/notifications/store.go:138-176`)
only collapses a repeat `(sessionID, notificationType)` event into an
existing row while that row is unread — the in-file "ADR-003" comment
(`store.go:130-133`) documents this as deliberate. Once a notification is
read, every later recurrence forks a brand-new row, which is Failure Mode 1
in requirements.md: unbounded growth of low-value rows that never reflect a
resolved state.

Two things complicate a naive "always collapse" fix:

1. `AppendAutoApproved` (`store.go:181-207`) writes `NOTIFICATION_TYPE_AUTO_APPROVED`
   records pre-read by design, documented to "never appear in the active
   notification feed." If the collapse branch unconditionally flips
   `IsRead` back to `false` on any recurrence, a second identical
   auto-approval would resurrect a silent record into the main feed.
2. `enforceRetention()` (`store.go:511-522`) prunes by `CreatedAt`. Once
   `Append()` always collapses instead of forking, a frequently-recurring
   record keeps its *original* `CreatedAt` forever — the moment that
   crosses the 7-day `MaxNotificationAge`, the whole record (current
   `OccurrenceCount` included) is deleted, even though it occurred seconds
   ago. This reintroduces the exact "unbounded fresh rows" symptom the fix
   is meant to eliminate, just on a ~7-day period instead of per-read.

## Decision

1. **Dedup matches `(sessionID, notificationType)` regardless of read
   state.** `findUnreadDuplicate` is renamed `findDuplicate` and drops its
   `!r.IsRead` clause.
2. **The collapse branch resets `IsRead`/`ReadAt`, except for
   `notifTypeAutoApproved`.** A recurrence of any other type is new
   information the user hasn't seen; a recurrence of an auto-approved
   record stays silent, preserving `AppendAutoApproved`'s existing contract
   with one explicit type-check rather than a parallel mechanism.
3. **Retention keys off `LastOccurredAt`, falling back to `CreatedAt` when
   nil** (old, pre-migration records). A record that's still actively
   recurring never ages out just because its first occurrence is old.
4. **`deduplicateExisting()`'s startup migration gets the same predicate
   change** (Story 1.1.3), so records forked under the old behavior
   consolidate on next load rather than lingering individually until they
   age out under the old per-row 7-day clock.

## Alternatives Considered

- **Leave `AppendAutoApproved` special-casing to the caller instead of the
  store.** Rejected: every future producer of a notification would need to
  remember the carve-out; keeping it inside `Append()`'s one collapse branch
  means the invariant can never be forgotten by a new call site.
- **Add a new "resolved" boolean distinct from `IsRead`** (PagerDuty-style
  dedup-key-until-resolved, per features.md's industry comparable) instead
  of reusing `IsRead`. Rejected as unnecessary for this project's actual
  bug: `IsRead` already means "the user has seen the current state of this
  event," which is exactly what should flip on a genuine recurrence — a
  parallel `Resolved` field would duplicate that meaning without adding a
  distinction anything in this codebase currently needs.
- **Prune by `max(CreatedAt, LastOccurredAt)` computed once and cached on
  the record.** Rejected: `LastOccurredAt` is already maintained on every
  `Append()`; introducing a second derived/cached field for the same
  purpose is unnecessary state to keep in sync.

## Consequences

- No schema change — `OccurrenceCount`/`LastOccurredAt` already exist;
  this is a ~10-line diff across `Append()`, `enforceRetention()`, and
  `deduplicateExisting()`.
- `web-app/src/lib/utils/notificationGrouping.ts`'s `count = max(occurrenceCount,
  group.length)` fallback becomes a permanent no-op post-fix (there will
  only ever be one record per key) — no frontend change required, confirmed
  by direct read of that utility.
- A long-lived, frequently-recurring notification type (e.g. "Claude turn
  complete" for an always-on session) can now accumulate an
  `OccurrenceCount` indefinitely rather than being pruned — this is the
  intended behavior change per requirements.md's Success Metrics, not a
  regression; `MaxNotifications = 500` still bounds total row count across
  all sessions/types.
