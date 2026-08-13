# Session Status Unification — Implementation Plan

**Total scope:** 7 epics · 22 stories · 63 tasks

---

## Dependency Graph

```
Epic 1 (proto foundation)
  ├── Epic 2 (Go rename + CommandExecutor fix)    [can run in parallel with Epic 3]
  └── Epic 3 (extend SessionUpdatedEvent)         [can run in parallel with Epic 2]
        └── Epic 4 (migrate StatusChangedEvent consumers)
              └── Epic 5 (frontend typed enum + Redux unification)
                    └── Epic 6 (remove WorkingState server-side derivation)
                          └── Epic 7 (exhaustive lint + type enforcement)
```

---

## Epic 1: Proto Foundation (DetectedStatus enum + codegen)

**Depends on:** nothing (first epic)
**Blocks:** all other epics
**Estimated test impact:** proto binding regeneration only; no existing tests should break

### Story 1.1 — Add `DetectedStatus` enum to `proto/session/v1/types.proto`

**Task 1.1.1** — Edit `proto/session/v1/types.proto`: add the following enum after the `WorkingState` enum definition. Use proto3 naming convention (`DETECTED_STATUS_` prefix) and explicitly assign field numbers starting at 0:

```protobuf
enum DetectedStatus {
  DETECTED_STATUS_UNSPECIFIED       = 0;
  DETECTED_STATUS_IDLE              = 1;
  DETECTED_STATUS_PROCESSING        = 2;
  DETECTED_STATUS_EXECUTING         = 3;  // was StatusActive; tool/command running
  DETECTED_STATUS_NEEDS_APPROVAL    = 4;
  DETECTED_STATUS_INPUT_REQUIRED    = 5;
  DETECTED_STATUS_ERROR             = 6;
  DETECTED_STATUS_TESTS_FAILING     = 7;
  DETECTED_STATUS_SUCCESS           = 8;
  DETECTED_STATUS_UNKNOWN           = 9;  // (.*) catch-all fallback — renders NO badge
  DETECTED_STATUS_READY             = 10; // readline/shell prompt explicitly detected
  DETECTED_STATUS_WAITING_FOR_AGENT = 11; // waiting for background agents to finish
}
```

**Design decisions resolved (no action required before implementing Task 1.1.1):**
- `DETECTED_STATUS_UNKNOWN` (the `.*` catch-all) renders NO badge in the frontend (return `null`).
- `DETECTED_STATUS_READY` is a distinct value for "readline-style prompt explicitly detected,
  session awaiting input" — different from `DETECTED_STATUS_IDLE` (idle by timeout).
- `DETECTED_STATUS_RATE_LIMITED` is NOT added — rate limiting is handled by the `ratelimit.StateWaiting`
  check in `toProtoSubStatus`, not via the `DetectedStatus` enum. No Go `StatusRateLimited`
  constant exists.
- `DETECTED_STATUS_WAITING_FOR_AGENT` IS added — `StatusWaitingForAgent` exists in the Go iota
  (detector.go line 29) and must appear in the proto enum and the mapping function.

**Task 1.1.2** — Add `detected_status` and `detected_context` fields to the `Session` message in `proto/session/v1/types.proto`. The `Session` message field numbers 55–67 are already taken (`memory_rss_mb=55`, `estimated_savings_mb=56`, `hidden=57`, `pause_reason=58`, `goal=59`, `autonomous_mode=60`, `workflow_id=62`, `archived_at=63`, `workflow_name=64`, `autonomous_turn=65`, `autonomous_max_turns=66`, `autonomous_outcome=67`). Use field numbers 68 and 69:

```protobuf
// in Session message:
DetectedStatus detected_status  = 68;
string         detected_context = 69;
```

**Task 1.1.3** — Add `detected_status` and `detected_context` fields to `SessionUpdatedEvent` in `proto/session/v1/events.proto`. `SessionUpdatedEvent` currently has fields 1 (`session`) and 2 (`updated_fields`). Use fields 3 and 4:

```protobuf
// in SessionUpdatedEvent message:
DetectedStatus detected_status  = 3;
string         detected_context = 4;
```

Note: These fields on `SessionUpdatedEvent` are a transitional shortcut for the migration period. Once `StatusBadge` is migrated to read from `session.detected_status` (Epic 5), these event-level fields are redundant. They can be deprecated in Epic 7 but must NOT be removed until Epic 5 is complete.

### Story 1.2 — Add `detection.DetectedStatusToProto` mapping function

**Task 1.2.1** — Create (or extend) `session/detection/proto_mapping.go` with a function that converts the internal `detection.DetectedStatus` iota to the proto enum `sessionv1.DetectedStatus`. This function is the single authoritative mapping; do not duplicate the logic in adapters or converters.

```go
// session/detection/proto_mapping.go
package detection

import sessionv1 "github.com/.../gen/proto/go/session/v1"

func DetectedStatusToProto(s DetectedStatus) sessionv1.DetectedStatus {
    switch s {
    case StatusIdle:             return sessionv1.DetectedStatus_DETECTED_STATUS_IDLE
    case StatusProcessing:       return sessionv1.DetectedStatus_DETECTED_STATUS_PROCESSING
    case StatusExecuting:        return sessionv1.DetectedStatus_DETECTED_STATUS_EXECUTING         // renamed from StatusActive in Epic 2
    case StatusNeedsApproval:    return sessionv1.DetectedStatus_DETECTED_STATUS_NEEDS_APPROVAL
    case StatusInputRequired:    return sessionv1.DetectedStatus_DETECTED_STATUS_INPUT_REQUIRED
    case StatusError:            return sessionv1.DetectedStatus_DETECTED_STATUS_ERROR
    case StatusTestsFailing:     return sessionv1.DetectedStatus_DETECTED_STATUS_TESTS_FAILING
    case StatusSuccess:          return sessionv1.DetectedStatus_DETECTED_STATUS_SUCCESS
    case StatusUnknown:          return sessionv1.DetectedStatus_DETECTED_STATUS_UNKNOWN           // the .* catch-all; StatusReady was here before Epic 2
    case StatusReady:            return sessionv1.DetectedStatus_DETECTED_STATUS_READY             // explicit readline/shell prompt
    case StatusWaitingForAgent:  return sessionv1.DetectedStatus_DETECTED_STATUS_WAITING_FOR_AGENT
    default:                     return sessionv1.DetectedStatus_DETECTED_STATUS_UNSPECIFIED
    }
}
```

Note: `StatusExecuting` is the name as it will exist after Epic 2's rename of `StatusActive`.
Write the switch with current names (`StatusActive` for the `EXECUTING` case) and update in Epic 2
atomically. `StatusRateLimited` does NOT exist as a Go constant — rate limiting is handled via
`ratelimit.StateWaiting` in `toProtoSubStatus`, not here. Do not add it.

### Story 1.3 — Run codegen and verify

**Task 1.3.1** — Run `make generate-proto`. Verify that:
- `gen/proto/go/session/v1/types.pb.go` contains `DetectedStatus_DETECTED_STATUS_*` constants
- `web-app/src/gen/session/v1/types_pb.ts` exports a `DetectedStatus` TypeScript enum with members `UNSPECIFIED`, `IDLE`, `PROCESSING`, `EXECUTING`, `NEEDS_APPROVAL`, `INPUT_REQUIRED`, `RATE_LIMITED`, `ERROR`, `TESTS_FAILING`, `SUCCESS`, `UNKNOWN`
- `web-app/src/gen/session/v1/events_pb.ts` has the new fields on `SessionUpdatedEvent`
- `web-app/src/gen/session/v1/types_pb.ts` has the new fields on `Session`

**Task 1.3.2** — Run `make build` to confirm no compilation errors from the new proto fields (the new fields are optional in proto3 and will have zero values in all existing callers; no existing Go code needs to change at this point).

**Task 1.3.3** — Run `cd web-app && npx tsc --noEmit` to confirm no TypeScript errors from regenerated bindings.

---

## Epic 2: Go Rename + CommandExecutor Fix

**Depends on:** Epic 1 (proto generated; `DetectedStatusToProto` mapping function exists)
**Can run in parallel with:** Epic 3
**Blocks:** Epic 7 (exhaustive lint)
**Atomic constraint:** The `StatusActive → StatusExecuting` Go rename, the `StatusBadge.tsx` case update (`"Active"` → `"Executing"`), and `getDetectedStatusInfo` update MUST be in the same commit. The frontend badge silently breaks between the Go rename and the frontend update.

### Story 2.1 — Rename `StatusActive → StatusExecuting` in all Go source

**Task 2.1.1** — In `session/detection/detector.go`, rename the constant declaration and all 8 references (lines 27, 636, 684, 768, 789, 793, 795, 801). This is the canonical declaration site.

**Task 2.1.2** — Update remaining Go source files (all must be done in the same commit as 2.1.1):
- `session/detection/events.go` line 67 — switch case
- `session/detection/idle.go` line 182 — switch case
- `session/detection/pattern_set.go` lines 125, 136 — return values
- `session/claude_controller.go` lines 662, 672 — fallback assignment
- `session/instance_status.go` line 165 — switch case
- `session/review_queue_determiner.go` line 230 — switch case
- `session/status_mapping.go` line 32 — case group
- `server/adapters/instance_adapter.go` lines 215, 393 — `toProtoSubStatus` and `MapDetectedStatusToWorkingState`

**Task 2.1.3** — Update test files:
- `session/claude_controller_test.go`
- `session/review_queue_reactive_test.go`
- `session/status_mapping_test.go`

**Task 2.1.4 (ATOMIC with 2.1.1–2.1.3)** — Update `StatusBadge.tsx` string switch in `web-app/src/components/sessions/StatusBadge.tsx` line 54: change `case "Active":` to `case "Executing":`. This must ship in the same commit as the Go rename. During the migration to the typed enum (Epic 5), this string-based switch will be replaced entirely — but until then the string must match the Go `.String()` output.

**Task 2.1.5 (ATOMIC with 2.1.1–2.1.4)** — Update `DetectionEventsPanel.tsx` hardcoded string map (e.g., `8: "StatusActive"` → `8: "StatusExecuting"` or update to reflect new constant value).

**Task 2.1.6** — Update `DetectedStatusToProto` in `session/detection/proto_mapping.go` (Task 1.2.1): change `case StatusActive:` to `case StatusExecuting:`.

### Story 2.2 — Replace `StatusReady` (.*) catch-all with `StatusUnknown`

**Design decision resolved:** `StatusUnknown` (iota 0) already exists in `detector.go` — no new
constant is needed. `StatusReady` keeps a distinct definition: "readline/shell prompt explicitly
detected, session awaiting input." The `.*` catch-all pattern changes from returning `StatusReady`
to returning `StatusUnknown`. `StatusUnknown` renders NO badge (null). `StatusReady` renders a
neutral "Ready" badge (UX to confirm styling). `StatusIdle` remains "idle by timeout."

**Task 2.2.1** — In `session/detection/detector.go`: change the `.*` pattern's return value from
`StatusReady` to `StatusUnknown` (Category A sites from pitfalls research). Do NOT add a new
`StatusUnknown` constant — it already exists at iota position 0 (line 19).

**Task 2.2.2** — Update `session/detection/pattern_set.go` line 147: change `return StatusReady, ...` to `return StatusUnknown, ...` (Category A).

**Task 2.2.3** — Update Category A references in `session/detection/events.go` line 83 and `session/detection/idle.go` line 197.

**Task 2.2.4 (BLOCKING — critical fix)** — Update `session/command_executor.go`:
- Line 52 (`DefaultExecutionOptions`): replace `StatusReady` with `StatusIdle` in the terminal-status list. This is the status that signals "command completed, prompt is ready for input." Using `StatusUnknown` (catch-all) as the terminal signal would be semantically wrong.
- Line 381: `result.Success = (status == detection.StatusReady)` must become `result.Success = (status == detection.StatusIdle)`. This is the BLOCKING dependency noted in the project constraints — without this change, all `CommandExecutor` invocations report `Success = false` after the rename.

**Task 2.2.5** — Update Category B semantic sites. For each, determine whether `StatusUnknown`, `StatusIdle`, or both is the correct replacement:
- `session/autonomous_driver.go` line 323 (`isIdleOrComplete`): add `StatusUnknown` alongside `StatusIdle` only if catch-all completion is a valid resting state; otherwise remove the `StatusReady` reference.
- `session/claude_controller.go` lines 670, 678 (spinner detection guard that upgrades `StatusReady → StatusActive`): update to `StatusUnknown → StatusExecuting` since the guard promotes the catch-all when a spinner verb is detected.
- `session/instance_status.go` lines 123, 161: update proto mapping switch cases — `StatusReady → StatusUnknown` for the catch-all path, verify `SubStatus_SUB_STATUS_UNKNOWN` is used (or map to `UNSPECIFIED` if UNKNOWN is not a SubStatus value).

**Task 2.2.6** — Update `StatusBadge.tsx`: add a `"Unknown"` case that returns `null` (no badge)
so the catch-all fallback renders nothing. Keep the `"Ready"` case as-is (it now maps to the
explicit-prompt `StatusReady` which still returns `"Ready"` from `.String()`). After Epic 5
migrates `StatusBadge` to use the typed proto enum, these string cases are replaced by
`case DetectedStatus.UNKNOWN: return null` and `case DetectedStatus.READY: return { label: "Ready", ... }`.

**Task 2.2.7** — Update `DetectedStatusToProto`: `case StatusUnknown: return sessionv1.DetectedStatus_DETECTED_STATUS_UNKNOWN`.

**Task 2.2.8** — Run `make build && make test` and verify `CommandExecutor` tests pass with the new terminal-status semantics.

### Story 2.3 — Update `toProtoSubStatus` for the renamed constants

**Task 2.3.1** — In `server/adapters/instance_adapter.go` `toProtoSubStatus` function: the existing switch already covers the renamed constant values (after 2.1.2 updates the names). Verify the mapping is still complete:
- `StatusExecuting` (was `StatusActive`) → `SUB_STATUS_PROCESSING` (unchanged grouping)
- `StatusUnknown` (was `StatusReady`) → `SUB_STATUS_UNSPECIFIED` (NOT `SUB_STATUS_READY` — the `.*` fallback should not advertise as "Ready")

**Design decision required (flagged):** Currently `StatusReady` maps to `SUB_STATUS_READY`. If `StatusUnknown` replaces `StatusReady` and maps to `SUB_STATUS_UNSPECIFIED`, the `SUB_STATUS_READY` slot becomes unused. Decide: (a) remove `SUB_STATUS_READY` from the `SubStatus` enum (breaking proto change), (b) keep it unused but deprecated, or (c) repurpose it for a genuinely-ready explicit-prompt detection. This affects `SubStatusChip.tsx` as well.

---

## Epic 3: Extend `SessionUpdatedEvent` with Typed `DetectedStatus`

**Depends on:** Epic 1 (proto fields exist; `DetectedStatusToProto` exists)
**Can run in parallel with:** Epic 2
**Blocks:** Epic 4

### Story 3.1 — Populate detection fields when publishing `SessionUpdatedEvent`

**Task 3.1.1** — Extend the internal `Event` struct in `pkg/events/types.go` if `DetectedStatus` and `DetectedContext` are not already fields on `EventSessionUpdated`. Inspect the struct; the research notes these fields exist on the `EventSessionStatusChanged` struct. If the `EventSessionUpdated` struct is separate, add the same fields. If they share a base struct, no change is needed.

**Task 3.1.2** — Update `server/services/event_converter.go`, `EventSessionUpdated` case (lines 27–33): after calling `adapters.InstanceToProto`, also populate the new `SessionUpdatedEvent.DetectedStatus` and `SessionUpdatedEvent.DetectedContext` fields. Read the detection state from the event struct (`event.DetectedStatus`, `event.DetectedContext`) — do NOT re-read from `InstanceStatusManager` here (the publisher already has the authoritative value at publish time):

```go
case events.EventSessionUpdated:
    updatedEvent := &sessionv1.SessionUpdatedEvent{
        Session:       adapters.InstanceToProto(event.Session, nil),
        UpdatedFields: event.UpdatedFields,
    }
    if event.DetectedStatus != detection.StatusUnknown {  // or use proto zero value check
        updatedEvent.DetectedStatus = detection.DetectedStatusToProto(event.DetectedStatus)
        updatedEvent.DetectedContext = event.DetectedContext
    }
    protoEvent.Event = &sessionv1.SessionEvent_SessionUpdated{SessionUpdated: updatedEvent}
```

**Task 3.1.3** — Update `InstanceToProto` in `server/adapters/instance_adapter.go` to populate `Session.detected_status` (field 55) and `Session.detected_context` (field 56) that were added in Epic 1. Only populate when `inst.Status == session.Active` (mirror `toProtoSubStatus`'s guard):

```go
if inst.Status == session.Active {
    detectedStatus, detectedContext := inst.GetDetectedStatus(), inst.GetDetectedContext()
    proto.DetectedStatus = detection.DetectedStatusToProto(detectedStatus)
    proto.DetectedContext = detectedContext
}
```

**Task 3.1.4** — Update the `sessionExitedPublisher` in `server/services/session_service.go` (lines 3615–3623). When publishing `NewSessionUpdatedEvent`, include the current detection state. Since the session has exited, the detected status should be cleared (set to zero/UNSPECIFIED) or reflect the terminal state. Verify what `inst.GetDetectedStatus()` returns after exit and set accordingly. This was the original "Thinking…" linger bug site.

**Task 3.1.5** — Update `UpdateSession` RPC publish site (`server/services/session_service.go` lines 1454–1469): the `NewSessionUpdatedEvent` call on line 1469 (just after the `StatusChangedEvent` publish) should include the detection info so the `SessionUpdatedEvent` always carries detection state when published alongside a status change.

**Task 3.1.6** — Update the rate-limit callbacks publish site (`server/services/session_service.go` line 3665): if the `Event` struct supports it, populate detection fields from `inst`. Rate-limit events do not need detection context urgently, but consistency is preferred.

### Story 3.2 — Update `NewSessionUpdatedEvent` constructor(s) in `pkg/events/types.go`

**Task 3.2.1** — If `NewSessionUpdatedEvent` takes an `instance` argument, update it (or create an overload `NewSessionUpdatedEventWithDetection`) to accept and store `DetectedStatus` and `DetectedContext`. Ensure all three call sites (Tasks 3.1.4, 3.1.5, 3.1.6) use the appropriate form.

---

## Epic 4: Migrate `StatusChangedEvent` Consumers to `SessionUpdatedEvent`

**Depends on:** Epic 3 (SessionUpdatedEvent now carries detection info)
**Blocks:** Epic 5
**ATOMIC CONSTRAINT (user decision):** `StatusChangedEvent` is removed IMMEDIATELY in this epic —
push/subscriber.go and analytics/subscriber.go are migrated in the SAME PR as the removal.
No parallel publishing ("dual-emit bridge") is introduced. The mobile app at
`https://onyx.staplerhome.internal:8444` will stop receiving `statusChanged` stream events after
this PR ships; old mobile clients will see stale badges until updated. This tradeoff is accepted.

All four migration tasks (push, analytics, event publisher removal, event converter removal) MUST
land in one atomic commit. Do not split them.

### Story 4.1 — Migrate `server/push/subscriber.go` to `EventSessionUpdated`

**Task 4.1.1** — In `server/push/subscriber.go` lines 92, 111, 195: the push notification subscriber currently handles `EventSessionStatusChanged` to fire "Session Completed" push notifications when `newStatus == Stopped`. Migrate this to handle `EventSessionUpdated` by checking `event.Session.Status == session.Stopped` (or equivalent) and the same `shouldNotify` conditions.

Specific changes:
- `shouldNotify` function (line 92): change the type assertion from `EventSessionStatusChanged` to `EventSessionUpdated`; read `event.Session.Status` for `newStatus`
- `buildDeliveryNotification` (line 111): use `event.Session` fields directly instead of `StatusChangedEvent` fields
- `buildNotificationForSession` (line 195): update accordingly

**Task 4.1.2** — Add a test (or update existing) in `server/push/` verifying that when a `EventSessionUpdated` with `Session.Status == Stopped` is published, a "Session Completed" push notification is sent.

### Story 4.2 — Migrate `server/analytics/subscriber.go` to `EventSessionUpdated`

**Task 4.2.1** — In `server/analytics/subscriber.go` lines 73–87: the analytics subscriber records `session.status_changed` when `EventSessionStatusChanged` fires. Update to listen for `EventSessionUpdated` with a status transition. To detect transitions (old vs. new status), either: (a) store the last-known status per session in-memory and compare, or (b) add an `old_status` field to `EventSessionUpdated` for analytics purposes. Option (a) is simpler and avoids a proto change.

**Task 4.2.2** — Verify analytics subscriber still records events with the correct labels after migration. Update or add a test in `server/analytics/`.

### Story 4.3 — Remove `EventSessionStatusChanged` (atomic with Stories 4.1 and 4.2)

This story executes IN THE SAME COMMIT as Stories 4.1 and 4.2. No dual-emit bridge is
introduced. After this story the `StatusChangedEvent` no longer exists in the system.

**Task 4.3.1** — Remove the `EventSessionStatusChanged` publish from `UpdateSession` RPC
(`server/services/session_service.go` lines 1454–1467). The `SessionUpdatedEvent` published
immediately after (line 1469) now carries the detection info (Epic 3) and is sufficient.

**Task 4.3.2** — Remove the `EventSessionStatusChanged` case from `event_converter.go`
(lines 43–55). The `status_changed` stream event type will no longer be sent to clients.

**Task 4.3.3** — Remove `SessionStatusChangedEvent` from `proto/session/v1/events.proto`:
replace the `status_changed = 5` oneof arm with a `reserved 5;` and `reserved "status_changed";`
declaration to prevent field number reuse.

**Task 4.3.4** — Remove `EventSessionStatusChanged` struct and `NewSessionStatusChangedEvent`
constructor from `pkg/events/types.go`.

**Task 4.3.5** — Run `make generate-proto`, `make build`, `make test`. Verify no remaining
references to `EventSessionStatusChanged` in non-test Go code (search for the string).

Note: The frontend `"statusChanged"` stream case in `useSessionService.ts` becomes unreachable
dead code after this epic. It is removed in Epic 5 Story 5.4 (which is no longer deferred).

---

## Epic 5: Frontend — Typed `DetectedStatus`, Single Redux Path, `upsertSession` Fixes

**Depends on:** Epics 3 and 4 (typed detection fields on `SessionUpdatedEvent`; consumers migrated)
**Blocks:** Epic 6
**Atomic constraint:** `StatusBadge` migration to typed enum + `upsertSession` redux unification + `SessionCard` string equality removal must ship together (they form a consistent frontend state model). They can be split across two PRs if needed, but within a PR they should all be present.

### Story 5.1 — Update `upsertSession` reducer to sync `detectedStatusMap`

**Task 5.1.1** — In `web-app/src/lib/store/sessionsSlice.ts`, update the `upsertSession` case reducer (currently only calls `sessionsAdapter.upsertOne`). Add detection-state sync logic per R4.2/R4.3:

```typescript
import { DetectedStatus } from "@/gen/session/v1/types_pb";

// Inside upsertSession case:
sessionsAdapter.upsertOne(state, session);

if (session.status !== SessionStatus.ACTIVE) {
  // R4.2: non-active session — clear badge unconditionally
  delete state.detectedStatusMap[session.id];
} else if (
  session.detectedStatus !== undefined &&
  session.detectedStatus !== DetectedStatus.UNSPECIFIED
) {
  // R4.3: active with typed detection info — update map from proto field
  state.detectedStatusMap[session.id] = {
    detectedStatus: session.detectedStatus,  // store the typed enum value; see Task 5.1.2
    detectedContext: session.detectedContext ?? "",
  };
} else {
  // ACTIVE + UNSPECIFIED: clear the map entry.
  // StatusChangedEvent was removed in Epic 4 — there is no dual-path to "leave stale for."
  // UNSPECIFIED from the server is the authoritative signal that no detection is available.
  delete state.detectedStatusMap[session.id];
}
```

**Task 5.1.2** — Change `detectedStatusMap` value type from `{ detectedStatus: string; detectedContext: string }` to `{ detectedStatus: DetectedStatus; detectedContext: string }`. Update all read sites:
- `web-app/src/components/sessions/SessionList.tsx` lines 177, 1142–1143
- `web-app/src/components/sessions/SessionCard.tsx` lines 509–512
- `web-app/src/components/sessions/StatusBadge.tsx`
- Any other consumer of `detectedStatusMap`

**Task 5.1.3** — Update `updateSessionStatus` reducer: keep it as an internal implementation detail for now (it still handles the `statusChanged` stream event from old servers during the transition). Update its `detectedStatusMap` write to use the typed `DetectedStatus` enum instead of the raw string. The `detectedStatus?: string` field from `SessionStatusChangedEvent` must be parsed to the enum — add a helper `parseDetectedStatusString(s: string): DetectedStatus` that maps `"Executing"` → `DetectedStatus.EXECUTING`, etc. This helper is a transitional shim and is removed in Story 5.4.

### Story 5.2 — Update `StatusBadge.tsx` to use typed `DetectedStatus` enum

**Task 5.2.1** — Rewrite `getDetectedStatusInfo` in `web-app/src/components/sessions/StatusBadge.tsx` to accept `DetectedStatus` (the TypeScript enum) instead of `string`. Replace the raw string switch with a typed switch:

```typescript
import { DetectedStatus } from "@/gen/session/v1/types_pb";

function getDetectedStatusInfo(status: DetectedStatus): StatusInfo | null {
  switch (status) {
    case DetectedStatus.EXECUTING:       return { label: "Executing", icon: "⚡", variant: "active" };
    case DetectedStatus.PROCESSING:      return { label: "Thinking…", icon: "...", variant: "processing" };
    case DetectedStatus.NEEDS_APPROVAL:  return { label: "Needs Approval", icon: "⚠️", variant: "warning" };
    case DetectedStatus.INPUT_REQUIRED:  return { label: "Input Required", icon: "⌨️", variant: "warning" };
    case DetectedStatus.IDLE:            return { label: "Idle", icon: "●", variant: "idle" };
    case DetectedStatus.SUCCESS:         return { label: "Success", icon: "✅", variant: "success" };
    case DetectedStatus.ERROR:           return { label: "Error", icon: "✗", variant: "error" };
    case DetectedStatus.TESTS_FAILING:   return { label: "Tests Failing", icon: "✗", variant: "error" };
    case DetectedStatus.RATE_LIMITED:    return { label: "Rate Limited", icon: "⏸", variant: "warning" };
    case DetectedStatus.UNKNOWN:         return null;  // .*  catch-all: show nothing
    case DetectedStatus.UNSPECIFIED:     return null;
    default:
      return assertNever(status);  // compile error if a new enum value is unhandled
  }
}
```

Add the `assertNever` helper function in this file or import from a shared utility.

**Task 5.2.2** — Update the `StatusBadge` component props: change `detectedStatus?: string` to `detectedStatus?: DetectedStatus`.

**Task 5.2.3** — Update `SessionCard.tsx` lines 509–512: pass `detectedStatusMap[id]?.detectedStatus` (now typed `DetectedStatus`) to `<StatusBadge>`. Remove the string equality suppression checks (`detectedStatus === "Needs Approval"` and `detectedStatus === "Input Required"`) — replace with typed enum comparisons (`detectedStatus === DetectedStatus.NEEDS_APPROVAL`).

### Story 5.3 — Update the `WatchSessions` stream handler (`useSessionService.ts`)

**Task 5.3.1** — In `web-app/src/lib/hooks/useSessionService.ts` lines 718–720, the `"sessionUpdated"` case currently dispatches `upsertSession(session)`. No change is needed to the dispatch call itself — `upsertSession` now handles `detectedStatusMap` internally (Story 5.1). Verify that the `session` object from the event includes the new `detectedStatus` field (it will after Epic 1's proto changes and Epic 3's serialization changes).

**Task 5.3.2** — Remove the `"statusChanged"` case (lines 730–740) from `handleSessionEvent`.
`StatusChangedEvent` was removed atomically in Epic 4; this case is now unreachable dead code.
The `updateSessionStatus` action is removed in Story 5.4 (same PR).

**Task 5.3.3** — (Subsumed by Story 5.4 — no separate action needed.)

### Story 5.4 — Remove `updateSessionStatus` action (execute in Epic 5 — not deferred)

`StatusChangedEvent` was removed atomically in Epic 4. The `"statusChanged"` stream case is now
dead code. This story executes in the same Epic 5 PR as Stories 5.1–5.3.

**Task 5.4.1** — Remove `updateSessionStatus` from `sessionsSlice.ts`.
**Task 5.4.2** — Remove `"statusChanged"` case from `useSessionService.ts`.
**Task 5.4.3** — Remove the `parseDetectedStatusString` transitional shim (from Task 5.1.3).
**Task 5.4.4** — Optionally remove `detectedStatusMap` from Redux state entirely if all consumers
now read from `session.detectedStatus` on the entity (see architecture research §5). This is
optional — `detectedStatusMap` can remain as a derived cache layer.

### Story 5.5 — Update Jest tests for Redux and StatusBadge changes

**Task 5.5.1** — Update `web-app/src/lib/store/sessionsSlice.test.ts` (if it exists): add tests for `upsertSession` correctly clearing `detectedStatusMap` when `status !== ACTIVE`, and populating from `detectedStatus` proto field when active.

**Task 5.5.2** — Update `web-app/src/components/sessions/__tests__/StatusBadge.test.tsx` (if it exists): change props from string to `DetectedStatus` enum values.

**Task 5.5.3** — Run `cd web-app && npx jest --no-coverage` and confirm all tests pass.

---

## Epic 6: Remove `WorkingState` Server-Side Derivation

**Depends on:** Epic 5 (`session.detectedStatus` is now on the wire; frontend can derive)
**Blocks:** Epic 7
**Critical constraint:** Write `deriveWorkingState` on the frontend and wire it to all consumers BEFORE removing `MapDetectedStatusToWorkingState` from the server. If removed from server first, `ReviewItem.WorkingState` goes to zero and `ReviewQueuePanel` filtering breaks.

### Story 6.1 — Add `deriveWorkingState` frontend utility

**Task 6.1.1** — Create `web-app/src/lib/utils/deriveWorkingState.ts`:

```typescript
import { DetectedStatus, SubStatus, WorkingState } from "@/gen/session/v1/types_pb";

export function deriveWorkingState(session: { subStatus: SubStatus; detectedStatus?: DetectedStatus }): WorkingState {
  switch (session.subStatus) {
    case SubStatus.PROCESSING:
    case SubStatus.NEEDS_APPROVAL:
    case SubStatus.INPUT_REQUIRED:
    case SubStatus.ERROR:
    case SubStatus.TESTS_FAILING:
    case SubStatus.RATE_LIMITED:
      return WorkingState.PROCESSING;
    case SubStatus.IDLE:
    case SubStatus.READY:
      return WorkingState.IDLE;
    case SubStatus.UNSPECIFIED:
      // fall through to detectedStatus
      break;
  }
  // detectedStatus-based fallback
  if (session.detectedStatus === DetectedStatus.EXECUTING ||
      session.detectedStatus === DetectedStatus.PROCESSING) {
    return WorkingState.ACTIVE;
  }
  return WorkingState.UNSPECIFIED;
}
```

**Design decision required (flagged):** The current `MapDetectedStatusToWorkingState` maps `StatusActive/StatusProcessing/StatusWaitingForAgent → ACTIVE`, `StatusNeedsApproval/StatusInputRequired → WAITING`, and `StatusIdle/StatusReady → IDLE`. Confirm the desired frontend mapping before finalizing `deriveWorkingState`. The pseudocode above is a reasonable approximation; adjust to match exact existing behavior.

**Task 6.1.2** — Add Jest tests for `deriveWorkingState` in `web-app/src/lib/utils/__tests__/deriveWorkingState.test.ts` covering each mapping case.

### Story 6.2 — Wire `deriveWorkingState` into `ReviewQueuePanel` and `reviewQueueSlice`

**Task 6.2.1** — In `web-app/src/lib/store/reviewQueueSlice.ts` lines 89–106: `selectNeedsAttentionItems` and `selectQueueCountsByWorkingState` currently use `item.workingState` from the entity (which comes from `ReviewItem.working_state` on the proto). Replace `item.workingState` reads with `deriveWorkingState(item)`. Import `deriveWorkingState` from the utility.

**Task 6.2.2** — In `web-app/src/components/sessions/ReviewQueuePanel.tsx` lines 12, 182–183, 190, 200–201, 409: update all `WorkingState` usage to use derived values from `deriveWorkingState`. The `WorkingState` import can be kept temporarily (for the filter comparison values) but the entity field access changes to the utility function.

**Task 6.2.3** — Update `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` line 279: test fixture currently sets `workingState: 0`. Since derivation is now frontend-only, set the `subStatus` field in the fixture to produce the expected `WorkingState` output instead.

### Story 6.3 — Remove server-side `MapDetectedStatusToWorkingState`

**Task 6.3.1** — In `server/adapters/instance_adapter.go`: remove `MapDetectedStatusToWorkingState` (lines 388–420) and `MapIdleStateToWorkingState` (lines 372–384). Remove all call sites that populate `working_state` on `ReviewItem` proto in `review_queue_adapter.go`.

**Task 6.3.2** — Remove `WorkingState working_state = 6` from `SessionStatusChangedEvent` in `proto/session/v1/events.proto` line 66. (This field is also being deprecated along with the whole event in Epic 4 Story 4.4; coordinate timing.)

**Task 6.3.3** — Deprecate (do NOT yet delete) `WorkingState working_state = 50` from `Session` in `proto/session/v1/types.proto` line 163 and `WorkingState working_state = 20` from `ReviewItem` in `proto/session/v1/types.proto` line 557. Mark as `deprecated = true` in proto options, but keep the field numbers reserved so they don't get reused.

**Task 6.3.4** — Optionally remove the `WorkingState` enum definition from `proto/session/v1/types.proto` once no Go server code references it. This is a breaking proto change — do it only after confirming the mobile app has updated and no clients depend on the field. If in doubt, keep the enum definition and mark values as deprecated.

**Task 6.3.5** — Run `make generate-proto`, `make build`, `make test`.

---

## Epic 7: Type Enforcement (Exhaustive Lint, TypeScript `never` Checks, Tests)

**Depends on:** All previous epics
**Final epic — no blockers**

### Story 7.1 — Go exhaustive switch enforcement

**Task 7.1.1** — Add `exhaustive` to the enabled linters in `.golangci.yml`:

```yaml
linters:
  enable:
    # ... existing entries ...
    - exhaustive
  settings:
    exhaustive:
      default-signifies-exhaustive: false
      package-scope-only: false
```

**Task 7.1.2** — For switches on the internal `detection.DetectedStatus` iota enum: the `exhaustive` linter will now report missing cases. Fix any unhandled cases surfaced across:
- `session/detection/detector.go`
- `session/detection/events.go`
- `session/detection/idle.go`
- `server/adapters/instance_adapter.go`
- `session/review_queue_determiner.go`
- `session/status_mapping.go`
- `session/detection/proto_mapping.go`

**Task 7.1.3** — For switches on proto-generated `sessionv1.DetectedStatus` (which is an `int32`, not an iota enum): the `exhaustive` linter will NOT cover these. Add `default: panic(fmt.Sprintf("unhandled DetectedStatus: %v", s))` to the `event_converter.go` proto conversion switch and `InstanceToProto` adapter switch to catch runtime regressions.

**Task 7.1.4** — Run `make lint` and confirm clean output.

### Story 7.2 — TypeScript `never` exhaustiveness checks

**Task 7.2.1** — Add `assertNever` utility to `web-app/src/lib/utils/assertNever.ts`:

```typescript
export function assertNever(x: never): never {
  throw new Error(`Unhandled case: ${String(x)}`);
}
```

**Task 7.2.2** — Apply `assertNever` in the `default:` branch of every `switch` over `DetectedStatus` in the frontend:
- `web-app/src/components/sessions/StatusBadge.tsx` — `getDetectedStatusInfo` (done in Story 5.2.1)
- `web-app/src/lib/utils/deriveWorkingState.ts` — inner switch (done in Story 6.1.1)
- Any other component added during Epic 5 that switches on `DetectedStatus`

**Task 7.2.3** — Run `cd web-app && npx tsc --noEmit` and confirm no compile errors.

### Story 7.3 — ESLint rule: no raw detected-status string literals

**Task 7.3.1** — Add an ESLint rule or `no-restricted-syntax` config to `web-app/.eslintrc.js` (or the project's ESLint config) that flags raw string literals equal to known detected-status names: `"Active"`, `"Executing"`, `"Processing"`, `"Needs Approval"`, `"Input Required"`, `"Idle"`, `"Ready"`, `"Unknown"`, `"Success"`, `"Error"`, `"Tests Failing"`, `"Rate Limited"`. This prevents future regressions where a developer adds a new string comparison instead of using the typed enum.

Example using `no-restricted-syntax`:
```json
{
  "no-restricted-syntax": [
    "error",
    {
      "selector": "Literal[value=/^(Executing|Processing|Needs Approval|Input Required|Idle|Unknown|Success|Error|Tests Failing|Rate Limited)$/]",
      "message": "Use the DetectedStatus typed enum from @/gen/session/v1/types_pb instead of raw strings."
    }
  ]
}
```

Note: This rule will produce false positives for strings that happen to match (e.g., display labels in non-status contexts). Consider scoping to specific directories or using a comment-based suppression mechanism.

**Task 7.3.2** — Run `cd web-app && npx eslint src/` and verify no violations outside of intentionally-suppressed sites.

### Story 7.4 — Integration tests for the full pipeline

**Task 7.4.1** — Add a Go integration test (or extend existing) in `server/services/` verifying that when a session transitions from ACTIVE to STOPPED:
1. The `SessionUpdatedEvent` carries `DetectedStatus = DETECTED_STATUS_UNSPECIFIED` (cleared on stop)
2. The `detectedContext` is empty
3. No `StatusChangedEvent` is published (post-Epic 4 Story 4.4)

**Task 7.4.2** — Add a Jest test (or extend `sessionsSlice.test.ts`) verifying `upsertSession` behavior:
1. When `session.status !== ACTIVE`, `detectedStatusMap[id]` is deleted
2. When `session.status === ACTIVE` and `session.detectedStatus === DetectedStatus.EXECUTING`, `detectedStatusMap[id].detectedStatus === DetectedStatus.EXECUTING`
3. When `session.status === ACTIVE` and `session.detectedStatus === DetectedStatus.UNSPECIFIED`, `detectedStatusMap[id]` is unchanged

**Task 7.4.3** — Run `make ci` (full CI pipeline) and confirm clean.

---

## Acceptance Criteria Checklist

Referenced from requirements.md:

- [ ] `make build` passes
- [ ] `make test` passes
- [ ] `make lint` passes (including `exhaustive` linter — added in Epic 7)
- [ ] `cd web-app && npx jest --no-coverage` passes
- [ ] No raw detected-status string literals in frontend (ESLint rule — Epic 7 Story 7.3)
- [ ] `StatusBadge` switch uses typed `DetectedStatus` enum, not strings (Epic 5 Story 5.2)
- [ ] `upsertSession` clears `detectedStatusMap` when `status !== ACTIVE` (Epic 5 Story 5.1)
- [ ] `WorkingState` is no longer computed server-side (Epic 6 Story 6.3)
- [ ] `StatusActive` name no longer exists in Go source (Epic 2 Story 2.1)
- [ ] `StatusReady` is no longer the `.*` catch-all pattern (Epic 2 Story 2.2)
- [ ] Existing behavior: sessions detect the same statuses as before (verified by `make test`)
- [ ] A stopped session that was previously "Thinking…" shows no chip or badge after stopping (verified by `upsertSession` fix in Epic 5 + `sessionExitedPublisher` fix in Epic 3)

---

## Open Design Decisions (Require User Input Before Implementation)

1. ~~**`DETECTED_STATUS_READY` as a distinct value?**~~ **RESOLVED:** `StatusReady` keeps a
   distinct definition — "readline/shell prompt explicitly detected, session awaiting input."
   `StatusUnknown` is the `.*` catch-all. `DETECTED_STATUS_READY = 10` is added to the proto
   enum. See Task 1.1.1.

2. **`SUB_STATUS_READY` fate:** `StatusReady` now maps to `DETECTED_STATUS_READY` AND still maps
   to `SUB_STATUS_READY` in `toProtoSubStatus`. Recommended: keep `SUB_STATUS_READY = 8` and
   continue mapping `StatusReady → SUB_STATUS_READY` — no breaking proto change needed.
   `SubStatusChip.tsx` line 116 already handles `SubStatus.READY`. Only remove if there is a
   confirmed requirement to do so. Affects Task 2.3.1.

3. ~~**`DETECTED_STATUS_UNKNOWN` badge rendering:**~~ **RESOLVED:** `StatusUnknown` (the `.*`
   catch-all) renders NO badge — return `null` from `getDetectedStatusInfo`. See Task 2.2.6
   and Task 5.2.1.

4. ~~**Removal timing for `EventSessionStatusChanged`:**~~ **RESOLVED:** Removed immediately and
   atomically in Epic 4 (same PR as push + analytics migration). No dual-emit bridge. Mobile app
   stale-badge regression is accepted. See Epic 4 constraint note.

5. **`WorkingState` enum deletion:** Epic 6 Task 6.3.4 defers deleting the `WorkingState` enum
   definition. If no external clients (mobile app, any API consumers) depend on
   `ReviewItem.working_state`, it can be deleted in the same PR as Epic 6. If uncertain, keep
   it deprecated for one release cycle.

6. **`deriveWorkingState` exact mapping:** The pseudocode in Epic 6 Task 6.1.1 is an
   approximation. Confirm the exact `ACTIVE / PROCESSING / IDLE / WAITING / UNSPECIFIED` mapping
   against the current `MapDetectedStatusToWorkingState` in
   `server/adapters/instance_adapter.go` lines 388–420 before implementing.
