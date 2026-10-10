# ADR-002: Hidden-Session Policy Is Keyed on NotificationType; WARNING Needs an Explicit Class Stamp; Unresolvable IDs Fail Open

**Status**: Proposed (revised in plan repair iteration 3: crash is a FAILURE bus event, residual-leak bound stated, untrusted-type hint, O2 decided)
**Date**: 2026-10-07
**Project**: notification-tray-and-hidden-session-gate

## Context

Operator policy: a hidden session notifies only on **failure** or
**needs-human**. Priority is not a usable discriminator: the same
"Session Token Ceiling Exceeded" title was stored at priority 1 on some rows
and 3 on others (`research/pitfalls.md` section 1). WARNING (8) covers both
advisory noise (stale, steer, compaction) and hard stops (rate-limit,
guardrail stop). Lookup can fail for deleted sessions (84 of 319 stored rows).

## Decision

1. **Allow list by type** for a hidden session: APPROVAL_NEEDED(1),
   INPUT_REQUIRED(2), CONFIRMATION_NEEDED(3) = needs-human; ERROR(7),
   FAILURE(9) = failure. Everything else (TASK_COMPLETE, PROCESS_*, WARNING,
   INFO, DEBUG, STATUS_CHANGE, AUTO_APPROVED, CUSTOM) = routine, suppressed.
2. **WARNING is routine by default.** A producer that knows a WARNING is a hard
   stop stamps metadata `delivery_class=failure` (new constant in
   `pkg/events/notification_metadata.go`); the policy honors the stamp for
   hidden sessions only. Rejected: "WARNING with priority >= HIGH passes"
   (research suggestion) because it is priority-keyed and would let 7 of 9
   ceiling rows through, contradicting the zero-routine success metric.
   Producers to stamp are decided by Spike 1.3 (capacity guardrail stops,
   rate-limit detect); stale-session and steer notices stay routine and
   surface only in Background activity.
   A producer that knows an ERROR is routine noise stamps
   `delivery_class=routine`, honored for hidden sessions only: the
   `ssq-hook-handler` post-tool hook (one `--type error` per failing tool call,
   L528) and subagent `task_failed` hooks (L472, L475) do this. Without it,
   ADR-003's enum fix turns every failed `grep` in a hidden review session into
   an always-allowed ERROR(7). Hints are parsed once into a typed `ClassHint`
   at the filter boundary.
   A hidden session that crashes or permanently fails is failure-class
   because the review-queue poller skips hidden sessions (`shouldSkipSession`),
   so its ErrorState/stuck detection never fires for them. This needs no
   status concept in the policy: `PermanentlyFailed` already publishes an ERROR
   notification (`markSessionPermanentlyFailed`,
   `session/session_driver.go:1004`), and a **Crashed** transition gets one new
   FAILURE(9) producer in `sessionExitedPublisher`
   (`server/services/session_service_events.go:109`; plan Story 2.10). A crash is
   silent on every channel today because `buildStatusChangeNotification` returns
   false unless `Status == Stopped` (`server/push/subscriber.go:173`); building
   the crash as a bus event reuses the gate, history, toast and push paths.
   The new notification also fires for visible sessions (recorded plan
   assumption: same standard as PermanentlyFailed; restricting it to hidden
   sessions is a one-line condition).
   A fourth hint, `HintUntrustedType`, is stamped server-side by
   `SendNotification` on a request without `ssq_notify_schema` (an old
   `ssq-notify` whose type numbers collide with the proto enum, ADR-003): a
   hidden session's event then delivers (fail open, counted) instead of being
   suppressed as routine, so a skew can never swallow a failure.
3. **Fail open.** Visibility has four states: `Visible`, `Hidden`,
   `NotASession` (positively defined: empty ID, `item_id` metadata, or a member
   of a closed set of system IDs; hook IDs are session titles, so no ID-shape
   rule can tell a real session from a system ID), `Unresolved` (anything else
   not found). Only `Hidden` is gated. Deleted hidden sessions stay `Hidden`
   for a bounded tombstone period (24h, LRU 10,000) so late routine events do
   not leak. `Unresolved` delivers
   and logs at WARN with a counter, so a deleted-hidden-session leak is
   observable instead of silent. Resolution order: the event's `SessionID`
   against the index first, then the `item_id` metadata value against the index
   (PermanentlyFailed publishes through `Notify(inst.UUID, ...)`, which stamps
   `item_id=<session UUID>`), and only then the `NotASession` rules.
   **Accepted residual leak (stated bound)**: zero for sessions created or
   restored by this process (the index is seeded synchronously before any
   instance runs and updated in the create path); non-zero only through
   enumerated stragglers, each counted by
   `notification_delivery_unresolved_total{class}`: events after a restart for a
   hidden session deleted before the restart (tombstones are in-memory, 24h TTL),
   the first event of a session created by a path the index feed missed, and a
   renamed session before its rename is fed. An unresolved ID that exists in
   storage after the refresh is an index defect, not an accepted leak. Rationale: fail-closed recreates the dead-end
   where a failure is lost.
4. The policy never inspects the title or message text.

## Alternatives Considered

- Priority-keyed rule: rejected (above).
- Fail closed for unresolved: rejected; loses failures from deleted sessions.
- Treat WARNING as failure for hidden sessions: rejected; stale and compaction
  advisories would notify.

## Consequences

- A hidden session's `INPUT_REQUIRED` "Claude has a question" is allowed
  (needs-human). **Operator decision O2 is decided: add an audited Reply
  control now.** The read-only view (ADR-005) gets two UI write exceptions: the
  Reply, `ReplyToPendingQuestion`, specified in ADR-010, and the backlog steer of
  a live review session (operator decision O7, ADR-005 decision 5); the prune tool
  keeps these rows regardless.
- Unit test enumerates every `sessionv1.NotificationType` value and fails if a
  new enum value has no explicit class (compile-time-style exhaustiveness).
- **Accepted risk (Phase 6 verify):** `delivery_class` and `auto_remediating` are
  caller-supplied metadata on an unauthenticated local RPC, so any localhost
  caller can demote its own failure-class event to routine and have a hidden
  session's ERROR suppressed. Needs-human events are not demotable. The hook
  handler is itself an HTTP caller, so the server cannot tell it from another
  client; narrowing this needs an in-process-only producer path (operator call).
