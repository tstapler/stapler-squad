# ADR-003: Hard-Coded Push Trigger Rules Using Proto Enum Constants

Status: Accepted
Date: 2026-04-17

---

## Context

The push subscriber (`server/push/subscriber.go`) must decide when to send a push
notification for each `events.Event`. Three approaches were evaluated:

- **Option A – Table-driven config**: `map[NotificationType]bool` in config.json; user-editable
- **Option B – Per-type rules with proto enum constants**: hard-coded conditionals referencing named constants
- **Option C – Priority threshold only**: push if `priority >= MEDIUM`

The requirements state: **"approval_needed always fires push regardless of priority."**
This creates an explicit exception to any pure-threshold rule, making Option C incorrect.

The current code uses `int32(3)` as a magic number for `NOTIFICATION_PRIORITY_HIGH`. The
enum value `NOTIFICATION_PRIORITY_URGENT = 4` is never matched, so URGENT notifications
are silently not delivered as push. This is the primary correctness bug to fix.

Additionally, the `approval_needed` check currently only fires in the
`EventSessionStatusChanged` case. If `NOTIFICATION_TYPE_APPROVAL_NEEDED` arrives via the
`EventNotification` path, push is not sent. Both paths must be covered.

---

## Decision

Adopt Option B: hard-coded per-type rules referencing proto enum constants.

```go
// server/push/subscriber.go
case events.EventNotification:
    p := event.NotificationPriority
    t := event.NotificationType
    if p >= int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH) ||
       t == int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED) {
        shouldNotify = true
    }
```

The `EventSessionStatusChanged` case retains its explicit `NeedsApproval` check.

Define package-level constants to avoid repeated inline casts:
```go
// server/push/trigger_constants.go
const (
    priorityHigh   = int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)
    priorityUrgent = int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT)
    typeApproval   = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED)
)
```

A unit test table asserts each (EventType, Priority, NotificationType) → shouldNotify mapping.

---

## Consequences

**Positive**
- Explicit, auditable rules in code — no config file to drift.
- Proto enum constants make typos a compile error (wrong constant name) rather than a
  silent wrong-value bug.
- Unit test table provides living documentation of all trigger rules.
- Fixes the URGENT notifications gap immediately.
- Covers `APPROVAL_NEEDED` in both EventNotification and EventSessionStatusChanged paths.

**Negative**
- Rules not user-configurable without a code change. If the user wants "no push for ERROR
  type", they cannot suppress it without editing Go.

**Deferral decision**
Table-driven config (Option A) is worth revisiting if the user reports "I got a push for X
and didn't want it." That feedback loop does not exist yet. Adding config-driven suppression
before that user need is validated is premature generalisation.

**Rejected alternatives**
- Option A (table-driven): indirection without a current consumer; "approval_needed always
  fires" becomes a config constraint rather than a code invariant — easier to accidentally
  disable.
- Option C (priority threshold): fails the explicit requirements requirement that
  `approval_needed` fires regardless of priority.

---

## Related

- findings-architecture.md: Q3 analysis
- findings-pitfalls.md: BUG-2 magic int comparison
- proto/session/v1/types.proto: NotificationPriority and NotificationType enums
- server/push/subscriber.go: current trigger logic
