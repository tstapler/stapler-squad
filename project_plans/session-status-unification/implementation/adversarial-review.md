# Adversarial Review — Session Status Unification Implementation Plan

**Verdict: BLOCKED**

**Issues found: 10** (4 blockers, 4 concerns, 2 notes)

plan.md patched: YES — see "Patches Applied" section at end.

---

## Blocker Issues

### Issue 1 — BLOCKER: `StatusChangedEvent` removal is deferred, contradicting user decision #2

**Severity:** BLOCKER
**Location:** `plan.md` Epic 4, Story 4.3 (Task 4.3.1–4.3.2) and Story 4.4

**Problem:**
The user resolved that `StatusChangedEvent` is removed IMMEDIATELY — push/subscriber.go and
analytics/subscriber.go are migrated atomically in the same PR as the removal, with no parallel
publishing. The plan directly contradicts this with:

- Story 4.3 ("Dual-emit bridge for R3.4"): instructs keeping `StatusChangedEvent` publication
  alongside `SessionUpdatedEvent` "for backward compat with the mobile app"
- Story 4.4: defers `EventSessionStatusChanged` removal to after Epic 5 is complete

The plan's rationale for deferral (mobile app at `https://onyx.staplerhome.internal:8444`) was
acknowledged and overridden by the user. Keeping dual-emit keeps the mobile app working during the
transition; the user accepted the tradeoff.

**Fix:** Epic 4 must be restructured as a single atomic story:
- Task 4.1 and 4.2 (push + analytics migration) are required prerequisites
- Task 4.3 (dual-emit bridge) is REMOVED — do not add it
- The `EventSessionStatusChanged` remove tasks from Story 4.4 are folded into Epic 4 directly
- Epic 4 becomes one atomic PR: migrate push + analytics + remove StatusChangedEvent publish +
  remove event_converter case + remove proto field + remove struct and constructor

The frontend `"statusChanged"` stream case and `updateSessionStatus` redux action become dead code
immediately after Epic 4 — they should be removed in Epic 5 Story 5.4 which is no longer
"deferred."

---

### Issue 2 — BLOCKER: Proto field number collision — fields 55 and 56 in Session are already taken

**Severity:** BLOCKER
**Location:** `plan.md` Epic 1, Story 1.1, Task 1.1.2 — `proto/session/v1/types.proto`

**Problem:**
Task 1.1.2 instructs adding `detected_status` and `detected_context` to the `Session` message at
field numbers 55 and 56. These field numbers are already assigned:

```protobuf
int64 memory_rss_mb       = 55;
int64 estimated_savings_mb = 56;
bool  hidden              = 57;
string pause_reason       = 58;
SessionGoalSummary goal   = 59;
bool autonomous_mode      = 60;
string workflow_id        = 62;
google.protobuf.Timestamp archived_at = 63;
string workflow_name      = 64;
int32 autonomous_turn     = 65;
int32 autonomous_max_turns = 66;
string autonomous_outcome = 67;
```

The last used field in the Session message is 67. Using 55 and 56 would be a silent data
corruption bug — proto3 would decode `memory_rss_mb` as `DetectedStatus` and
`estimated_savings_mb` as `detected_context`. `buf generate` may not catch this without a
`buf lint` run.

**Fix:** Use field numbers 68 and 69:
```protobuf
// in Session message:
DetectedStatus detected_status  = 68;
string         detected_context = 69;
```

---

### Issue 3 — BLOCKER: `StatusRateLimited` referenced in plan but does not exist as a Go constant

**Severity:** BLOCKER
**Location:** `plan.md` Epic 1, Story 1.2, Task 1.2.1 — `DetectedStatusToProto` mapping function

**Problem:**
The plan's `DetectedStatusToProto` switch includes:
```go
case StatusRateLimited:    return sessionv1.DetectedStatus_DETECTED_STATUS_RATE_LIMITED
```

`StatusRateLimited` does not exist in the Go `DetectedStatus` iota enum
(`session/detection/detector.go` lines 18–30). Rate limiting is handled separately via
`ratelimit.StateWaiting` in `toProtoSubStatus`. The code will not compile.

Additionally, `StatusWaitingForAgent` (iota value 10, defined at line 29) exists in the Go enum
but is ABSENT from both the proto `DetectedStatus` enum definition (Task 1.1.1) and the
`DetectedStatusToProto` mapping function. `StatusWaitingForAgent` currently maps to
`SUB_STATUS_PROCESSING` in `toProtoSubStatus` — the proto mapping function must account for it.

**Fix:**
1. Remove `case StatusRateLimited:` from `DetectedStatusToProto`
2. Add `DETECTED_STATUS_WAITING_FOR_AGENT = 11;` to the `DetectedStatus` proto enum
3. Add `case StatusWaitingForAgent: return sessionv1.DetectedStatus_DETECTED_STATUS_WAITING_FOR_AGENT` to `DetectedStatusToProto`
4. Do NOT add `StatusRateLimited` to the Go iota enum — rate limiting is a separate layer
5. Add `DETECTED_STATUS_RATE_LIMITED` to the proto enum only if there is a code path that would set it; otherwise omit it

---

### Issue 4 — BLOCKER: `StatusReady` design decision resolved — plan's Story 2.2 design assumption is wrong

**Severity:** BLOCKER
**Location:** `plan.md` Epic 2, Story 2.2 — design decision note and Task 2.2.1

**Problem:**
Story 2.2's design decision note says: "This story assumes `StatusReady` is fully replaced by
`StatusUnknown` (catch-all) + `StatusIdle` (explicit prompt), with no separate `StatusReady`
constant."

The user resolved this differently: `StatusReady` **keeps a distinct definition** — "shell prompt
explicitly detected, session awaiting input." `StatusUnknown` becomes the catch-all (`.*` pattern).
`StatusReady` is NOT removed or merged into `StatusIdle`.

Under the correct resolution:
- The `.*` pattern returns `StatusUnknown` (NOT `StatusReady`)
- `StatusReady` becomes a new distinct state (explicit readline/shell prompt detection)
- `StatusIdle` remains "idle by timeout" (unchanged)
- `StatusUnknown` renders NO badge in the frontend (user decision #1)
- `StatusReady` needs a badge rendering decision (separate from `StatusUnknown`)

Concretely, Task 2.2.1 ("add `StatusUnknown` constant") is already moot — `StatusUnknown`
already exists at iota position 0 in `detector.go` (line 19). The real task is changing the
`.*` pattern from returning `StatusReady` to returning `StatusUnknown`, and preserving
`StatusReady` for explicit prompt patterns.

**Fix:** Rewrite Story 2.2 tasks:
- Task 2.2.1: Change the `.*` pattern's return value from `StatusReady` to `StatusUnknown`
  (Category A sites). Do NOT add a new `StatusUnknown` constant — it already exists.
- Task 2.2.6: `StatusBadge` must handle two distinct non-idle states: `"Unknown"` renders null
  (no badge), `"Ready"` renders a neutral indicator (TBD by UX). Update `getDetectedStatusInfo`
  accordingly.
- Task 1.1.1: Add `DETECTED_STATUS_READY = 11` to the proto enum for the explicit-prompt case.
- Task 1.2.1: Add `case StatusReady: return sessionv1.DetectedStatus_DETECTED_STATUS_READY` to
  `DetectedStatusToProto`.
- The "Open Design Decision #1" in the plan's open decisions section is now resolved and must be
  removed.

---

## Concern Issues

### Issue 5 — CONCERN: `DetectionEventsPanel.tsx` iota map will silently misidentify statuses if iota ordering changes

**Severity:** CONCERN
**Location:** `plan.md` Epic 2, Task 2.1.5 — `web-app/src/components/sessions/DetectionEventsPanel.tsx` line 23

**Problem:**
`DetectionEventsPanel.tsx` hardcodes int→name mappings that mirror the Go iota order:
```typescript
const STATUS_INT_TO_GO: Record<number, string> = {
  0: "StatusUnknown", 1: "StatusReady", 2: "StatusProcessing", ..., 8: "StatusActive", 9: "StatusSuccess",
};
```

Task 2.1.5 says "update to reflect new constant value" but gives no concrete fix. The real risk:
after Epic 2's changes, the iota ordering will shift or extend (e.g., `StatusWaitingForAgent`
is currently 10 but not in the map). If iota values change (e.g., inserting new constants between
existing ones), every entry in this map becomes wrong simultaneously — no compiler catches it.

This panel is a debug tool, but displaying wrong Go constant names while debugging detection
issues is actively harmful.

**Fix:**
1. Replace the hardcoded integer map with a reverse lookup generated from the typed
   `DetectedStatus` proto enum (after Epic 1 lands). Import `DetectedStatus` from the generated
   TypeScript and build the reverse map from enum name→value at module load.
2. Add `StatusWaitingForAgent` to the map now (currently missing).
3. Task 2.1.5 must be explicit: after Epic 1 codegen, switch `DetectionEventsPanel` to use
   `DetectedStatus` proto enum values for the int→name map.

---

### Issue 6 — CONCERN: `assertNever` in `StatusBadge.tsx` requires the type to be a discriminated TypeScript enum, not `number`

**Severity:** CONCERN
**Location:** `plan.md` Epic 5, Story 5.2, Task 5.2.1; Epic 7, Story 7.2

**Problem:**
The plan adds `assertNever(status)` to the `default:` branch of the `getDetectedStatusInfo`
switch. For `assertNever` to cause a TypeScript compile error when a case is missing, TypeScript
must narrow `status` to `never` after all arms are handled.

This works correctly for `protoc-gen-es` v2 TypeScript numeric enums (confirmed by research
`01-stack.md` §3). The `DetectedStatus` enum generated by `protoc-gen-es` IS a standard TypeScript
numeric enum, so `assertNever` will work as intended.

However, there is a subtle gap: the `assertNever` guard only catches unhandled cases at compile
time when the switch is over the exact TypeScript enum type. If `StatusBadge` receives the value
via a prop typed as `number` (which is assignable to a numeric enum), TypeScript will NOT narrow
it correctly and `assertNever` becomes unreachable dead code rather than a compile error.

**Fix:** Task 5.2.2 is critical — `detectedStatus?: DetectedStatus` (not `number`) must be the
prop type. The plan has this task but it must be sequenced strictly before 5.2.1, or done
atomically. Verify that no call site passes a `number` literal to the `detectedStatus` prop.

Also: the plan's Task 7.2.2 says `assertNever` in `deriveWorkingState.ts` inner switch, but the
inner switch is over `SubStatus`, not `DetectedStatus`. `deriveWorkingState` uses `session.subStatus`
(a `SubStatus` value) for the outer switch and only falls through to `detectedStatus` in the
`UNSPECIFIED` case. The `assertNever` pattern is not structurally applicable to the fallthrough
logic as written. Task 7.2.2 needs to be more precise about which switch gets `assertNever` and
whether both the `subStatus` switch and the `detectedStatus` fallback need it.

---

### Issue 7 — CONCERN: Analytics subscriber transition produces no old_status — analytics event loses transition information

**Severity:** CONCERN
**Location:** `plan.md` Epic 4, Story 4.2, Task 4.2.1

**Problem:**
The analytics subscriber (`server/analytics/subscriber.go:73–87`) records `session.status_changed`
with `old_status` and `new_status` labels. `SessionStatusChangedEvent` explicitly carries
`OldStatus`. `SessionUpdatedEvent` does NOT carry old status — it only has the current session
state.

Task 4.2.1 offers two options: (a) store last-known status per session in-memory, (b) add
`old_status` field to `SessionUpdatedEvent`. The plan says "option (a) is simpler."

Option (a) has a significant caveat: the analytics subscriber is a stateless event handler
today. Adding per-session in-memory state creates a new memory growth vector (unbounded map
of session ID → last status) and a subtle bug: if the server restarts or the analytics
subscriber is replaced, the in-memory state is lost and the next status transition will record
`old_status: ""` (zero) until the session status changes again.

Since `StatusChangedEvent` is being removed immediately (Issue 1 fix), option (a) is the
only viable path without a proto change. The plan must acknowledge the tradeoff explicitly.

**Fix:** Task 4.2.1 must:
1. Specify option (a) explicitly as the chosen approach
2. Add a note that the in-memory map is bounded by active session count (low risk in practice)
3. Ensure the map is cleaned up on `EventSessionDeleted` to prevent unbounded growth
4. Accept that old_status will be empty string for the first transition after server restart

---

### Issue 8 — CONCERN: `upsertSession` with ACTIVE + UNSPECIFIED detectedStatus leaves stale map entry — covers a subtle case incorrectly

**Severity:** CONCERN
**Location:** `plan.md` Epic 5, Story 5.1, Task 5.1.1

**Problem:**
The plan's pseudocode for `upsertSession` says:
```typescript
// Active but UNSPECIFIED detectedStatus: leave map unchanged
// (partial update — don't clobber an earlier detection from statusChanged)
```

After Epic 4 removes `StatusChangedEvent` (user decision #2), there are NO MORE `statusChanged`
events on the wire. The "don't clobber an earlier detection from statusChanged" rationale
evaporates. Every session update now comes via `SessionUpdatedEvent` which carries the full
session proto including `detectedStatus`. If the server sends `UNSPECIFIED`, that IS the correct
authoritative value — the map should be cleared, not left stale.

The only valid reason to leave the map unchanged is when the event is a partial update that
intentionally omits detection fields. After Epic 3 ensures all `SessionUpdatedEvent` publish
sites include detection state, there is no partial-detection-omitting event.

**Fix:** After Epic 4 is complete (StatusChangedEvent removed), change the `upsertSession` logic
for `ACTIVE + UNSPECIFIED` to CLEAR the map entry, not leave it stale:
```typescript
if (session.status !== SessionStatus.ACTIVE) {
  delete state.detectedStatusMap[session.id];
} else if (session.detectedStatus !== undefined &&
           session.detectedStatus !== DetectedStatus.UNSPECIFIED) {
  state.detectedStatusMap[session.id] = { ... };
} else {
  // ACTIVE + UNSPECIFIED: clear — server explicitly has no detection info
  delete state.detectedStatusMap[session.id];
}
```

If Epic 5 runs concurrently with an old server that omits detection fields (rolling deploy), the
brief badge flicker is acceptable — it is safer than a permanently stale badge.

---

## Note Issues

### Issue 9 — NOTE: Open Design Decisions #1, #3 in plan are now resolved by user and must be updated

**Severity:** NOTE
**Location:** `plan.md` "Open Design Decisions" section items 1 and 3

**Problem:**
Open Design Decision #1 ("DETECTED_STATUS_READY as a distinct value?") and #3
("DETECTED_STATUS_UNKNOWN badge rendering: what displays?") are now resolved:

- Decision #1: `StatusReady` keeps a distinct definition (explicit prompt detection). `StatusUnknown` is the `.*` catch-all. Add `DETECTED_STATUS_READY = 11` to proto enum.
- Decision #3: `StatusUnknown` renders NO badge (return `null` from `getDetectedStatusInfo`).
  `StatusBadge.tsx` Task 5.2.1 must return `null` for `DetectedStatus.UNKNOWN`.

**Fix:** Remove items #1 and #3 from the Open Design Decisions section. Task 2.2.6 must be
updated to be concrete: `case DetectedStatus.UNKNOWN: return null;` (no badge).

---

### Issue 10 — NOTE: `SUB_STATUS_READY` slot becomes orphaned — no plan for it

**Severity:** NOTE
**Location:** `plan.md` Epic 2, Task 2.3.1 — design decision required (flagged)

**Problem:**
Task 2.3.1 flags the `SUB_STATUS_READY` question (what happens to the existing SubStatus proto
value when `StatusReady` gets its new meaning?) but does not resolve it. This is Open Design
Decision #2. Given that `StatusReady` now means "explicit readline prompt detected," the existing
`SUB_STATUS_READY` value (SubStatus = 8 in the generated TypeScript) could map to the new
`StatusReady` via `toProtoSubStatus`. `SubStatusChip.tsx` already has `case SubStatus.READY`
handling (line 116) which can be reused.

The plan should make a concrete recommendation here rather than leaving it fully open. The least
risky path: keep `SUB_STATUS_READY = 8` in `SubStatus` and map `StatusReady` → `SUB_STATUS_READY`
in `toProtoSubStatus`, which preserves the existing chip display. Only remove `SUB_STATUS_READY`
from the proto if there is a confirmed reason to do so.

**Fix:** Resolve Open Design Decision #2 as: keep `SUB_STATUS_READY` mapped from `StatusReady`
(explicit prompt). No breaking proto change needed for SubStatus. Update Task 2.3.1 accordingly.

---

## Summary Table

| # | Severity | Location | One-line description |
|---|---|---|---|
| 1 | BLOCKER | Epic 4, Story 4.3/4.4 | StatusChangedEvent removal deferred; user said atomic immediate |
| 2 | BLOCKER | Epic 1, Task 1.1.2 | Session fields 55/56 are taken; use 68/69 |
| 3 | BLOCKER | Epic 1, Task 1.2.1 | StatusRateLimited doesn't exist; StatusWaitingForAgent missing from proto enum |
| 4 | BLOCKER | Epic 2, Story 2.2 | StatusReady must keep distinct definition; StatusUnknown already exists at iota 0 |
| 5 | CONCERN | Epic 2, Task 2.1.5 | DetectionEventsPanel hardcoded iota map will silently misidentify statuses |
| 6 | CONCERN | Epic 5, Task 5.2.1 / Epic 7, Task 7.2.2 | assertNever works only if prop type is DetectedStatus not number; Task 7.2.2 logic unclear |
| 7 | CONCERN | Epic 4, Task 4.2.1 | Analytics old_status is lost without explicit in-memory state management |
| 8 | CONCERN | Epic 5, Task 5.1.1 | upsertSession ACTIVE+UNSPECIFIED "leave stale" rationale breaks after StatusChangedEvent removal |
| 9 | NOTE | Open Design Decisions #1, #3 | Both are now resolved by user; remove from open list |
| 10 | NOTE | Epic 2, Task 2.3.1 | SUB_STATUS_READY fate: recommend keeping it mapped from new StatusReady |

---

## Patches Applied to plan.md

The following targeted patches have been applied:

1. **Proto field numbers (Issue 2):** Task 1.1.2 updated to use fields 68 and 69.
2. **DetectedStatusToProto mapping (Issue 3):** Task 1.2.1 updated to remove `StatusRateLimited`,
   add `StatusWaitingForAgent`, add `StatusReady`.
3. **Proto enum (Issue 3 + 4):** Task 1.1.1 updated to add `DETECTED_STATUS_READY = 11` and
   `DETECTED_STATUS_WAITING_FOR_AGENT = 12`.
4. **StatusReady design decision (Issue 4):** Story 2.2 note updated to reflect resolved decision.
   Task 2.2.1 corrected (StatusUnknown already exists; task is to change catch-all return value).
   Task 2.2.6 made concrete.
5. **StatusChangedEvent immediate removal (Issue 1):** Story 4.3 removed. Story 4.4 promoted to
   main Epic 4 body. Epic 4 dependency note updated to call out atomicity requirement.
6. **Open Design Decisions (Issue 9):** Items #1 and #3 removed from open decisions section.
7. **upsertSession UNSPECIFIED logic (Issue 8):** Task 5.1.1 comment updated to reflect that
   ACTIVE+UNSPECIFIED should CLEAR the map after StatusChangedEvent removal.
