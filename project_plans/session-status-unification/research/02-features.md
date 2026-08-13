# Session Status Unification — Features / Comparable Patterns Research

## 1. WatchSessions Stream Handler (Frontend)

**File:** `web-app/src/lib/hooks/useSessionService.ts` — `handleSessionEvent` callback (lines 702–761)

The stream handler is a `switch` on `event.event.case` with the following branches:

| Stream case | Redux actions dispatched | Notes |
|---|---|---|
| `"sessionCreated"` | `dispatch(upsertSession(session))` | Entity adapter deduplication via upsertOne |
| `"sessionUpdated"` | `dispatch(upsertSession(session))` | Full session proto → entity adapter |
| `"sessionDeleted"` | `dispatch(removeSession(sessionId))`, `dispatch(removeReviewQueueItem(sessionId))`, calls `onSessionDeletedRef.current` | Marks id in `deletedIds` guard |
| `"statusChanged"` | `dispatch(updateSessionStatus({ sessionId, newStatus, detectedStatus?, detectedContext? }))` | Does NOT call `upsertSession`; writes to a separate reducer path |
| `"notification"` | calls `onNotificationRef.current(event.event.value)` | No Redux state change |
| `"approvalResponse"` | calls `onApprovalResponseRef.current(approvalId, sessionId)` | No Redux state change |

The `statusChanged` branch is the architectural split point. It dispatches `updateSessionStatus` rather than `upsertSession`, which means the session entity and `detectedStatusMap` are written by two different reducers.

**Key structural observation:** `handleSessionEvent` captures `dispatch` but not `sessions`, which is intentional — it avoids stale-closure reconnects on every session list change. The `updateSessionStatus` action was added specifically to allow status mutation inside the reducer without a sessions closure.

## 2. WatchSessions Server-Side Stream

**File:** `server/services/session_service.go` lines 1673–1767

The server subscribes to ALL event types from the EventBus before sending the initial snapshot:

```go
eventCh, subID := s.eventBus.Subscribe(ctx)
```

For fresh connections it sends each instance as a `SessionCreatedEvent` via `createInitialSnapshotEvent()` (which calls `adapters.InstanceToProto`). For reconnecting clients (afterSeq > 0) it replays buffered events via `s.eventBus.EventsSince(afterSeq)`.

The main loop receives from `eventCh` and calls `convertEventToProto(event)` for all event types. The EventBus publishes the following internal types that WatchSessions forwards:

- `EventSessionCreated` → `SessionCreatedEvent` (carries full Session proto)
- `EventSessionUpdated` → `SessionUpdatedEvent` (carries full Session proto + `updatedFields`)
- `EventSessionDeleted` → `SessionDeletedEvent` (carries `sessionId`)
- `EventSessionStatusChanged` → `SessionStatusChangedEvent` (carries `sessionId`, `oldStatus`, `newStatus`, optional `detectedStatus` string, optional `detectedContext` string, `workingState`)
- `EventNotification` → `NotificationEvent`
- `EventApprovalResponse` → `ApprovalResponseEvent`

`EventUserInteraction` and `EventSessionAcknowledged` exist in the bus but there is no `convertEventToProto` case for them — they are silently dropped.

## 3. StatusChangedEvent End-to-End Path

### Go publish sites

There are exactly two places that publish `EventSessionStatusChanged`:

**Site 1 — `UpdateSession` RPC** (`server/services/session_service.go` lines 1454–1467):
```go
if oldStatus != instance.Status && oldStatus != 0 {
    statusEvent := events.NewSessionStatusChangedEvent(instance, oldStatus, instance.Status)
    if s.statusManager != nil {
        statusInfo := s.statusManager.GetStatus(instance)
        if statusInfo.IsControllerActive {
            statusEvent.DetectedStatus = statusInfo.ClaudeStatus.String()  // raw string
            statusEvent.DetectedContext = statusInfo.StatusContext
        }
    }
    s.eventBus.Publish(statusEvent)
}
// Also publish general update event
s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, updatedFields))
```

Note: Both a `StatusChangedEvent` AND a `SessionUpdatedEvent` are published on every status change here. The `UpdateSession` RPC publishes both in sequence.

**Site 2 — NOT FOUND (wireSessionExitedPublisher):**
The `sessionExitedPublisher.OnLifecycleEvent` (lines 3615–3623) publishes only `NewSessionUpdatedEvent`, NOT a `StatusChangedEvent`. This was the bug that caused "Thinking..." to linger — fixed by this publisher.

### Wire serialization

**File:** `server/services/event_converter.go` lines 43–55

```go
case events.EventSessionStatusChanged:
    statusChangedProto := &sessionv1.SessionStatusChangedEvent{
        SessionId: event.SessionID,
        OldStatus: adapters.StatusToProto(event.OldStatus),
        NewStatus: adapters.StatusToProto(event.NewStatus),
    }
    if event.DetectedStatus != "" {
        statusChangedProto.DetectedStatus = &event.DetectedStatus  // optional string
        statusChangedProto.DetectedContext = &event.DetectedContext
    }
    protoEvent.Event = &sessionv1.SessionEvent_StatusChanged{...}
```

### Proto definition

**File:** `proto/session/v1/events.proto` lines 51–67

```protobuf
message SessionStatusChangedEvent {
    string session_id = 1;
    SessionStatus old_status = 2;
    SessionStatus new_status = 3;
    optional string detected_status = 4;   // raw string from DetectedStatus.String()
    optional string detected_context = 5;
    WorkingState working_state = 6;        // computed by MapDetectedStatusToWorkingState
}
```

`detected_status` is a raw string (e.g., `"Active"`, `"Needs Approval"`) produced by `detection.DetectedStatus.String()`. There is no proto enum for it today.

### Frontend dispatch

**File:** `web-app/src/lib/hooks/useSessionService.ts` lines 730–740

```typescript
case "statusChanged": {
    const { sessionId, newStatus, detectedStatus, detectedContext } = event.event.value;
    dispatch(updateSessionStatus({
        sessionId,
        newStatus,
        detectedStatus: detectedStatus ?? undefined,
        detectedContext: detectedContext ?? undefined,
    }));
    break;
}
```

### Redux update

**File:** `web-app/src/lib/store/sessionsSlice.ts` — `updateSessionStatus` reducer (lines 62–87)

The reducer:
1. Updates `entity.status` and clears `entity.subStatus` when `newStatus !== ACTIVE`
2. Clears `detectedStatusMap[sessionId]` when not active
3. Updates `detectedStatusMap[sessionId]` with raw string when active and `detectedStatus` is provided

### StatusBadge consumption

**File:** `web-app/src/components/sessions/StatusBadge.tsx` — `getDetectedStatusInfo(status: string)` (lines 38–61)

Switches on raw string literals: `"Ready"`, `"Processing"`, `"Needs Approval"`, `"Input Required"`, `"Error"`, `"Tests Failing"`, `"Idle"`, `"Active"`, `"Success"`. Renaming any Go enum constant with no corresponding frontend update silently breaks badge rendering (default branch returns `{ label: status, icon: "●", variant: "unknown" }`).

**Files touched by StatusChangedEvent end-to-end:**
1. `pkg/events/types.go` — `Event` struct fields `DetectedStatus`, `DetectedContext`
2. `server/services/session_service.go` — publish sites (UpdateSession RPC)
3. `server/services/event_converter.go` — `EventSessionStatusChanged` case
4. `proto/session/v1/events.proto` — `SessionStatusChangedEvent` message
5. `web-app/src/gen/session/v1/events_pb.ts` — generated TypeScript type
6. `web-app/src/lib/hooks/useSessionService.ts` — `"statusChanged"` case in `handleSessionEvent`
7. `web-app/src/lib/store/sessionsSlice.ts` — `updateSessionStatus` reducer
8. `web-app/src/components/sessions/SessionList.tsx` — reads `detectedStatusMap` from store (line 177, 1142–1143)
9. `web-app/src/components/sessions/SessionCard.tsx` — renders `<StatusBadge detectedStatus={...}>` (lines 509–512)
10. `web-app/src/components/sessions/StatusBadge.tsx` — `getDetectedStatusInfo` string switch

## 4. SessionUpdatedEvent End-to-End Path

**Go publish sites (multiple):**
- `UpdateSession` RPC after status change: `events.NewSessionUpdatedEvent(instance, updatedFields)` — line 1469
- `sessionExitedPublisher` (unexpected exit): `events.NewSessionUpdatedEvent(l.inst, []string{"status"})` — line 3621
- Rate limit callbacks: `events.NewSessionUpdatedEvent(inst, []string{"rate_limit_state", "rate_limit_reset_time"})` — line 3665

**Wire serialization** (`event_converter.go` lines 27–33):
```go
case events.EventSessionUpdated:
    protoEvent.Event = &sessionv1.SessionEvent_SessionUpdated{
        SessionUpdated: &sessionv1.SessionUpdatedEvent{
            Session:       adapters.InstanceToProto(event.Session, nil),
            UpdatedFields: event.UpdatedFields,
        },
    }
```

`InstanceToProto` populates the full `Session` proto including `subStatus` (computed by `toProtoSubStatus`), `workingState`, `status`, and all other fields. **It does NOT include a raw `detectedStatus` string field.**

**Frontend dispatch** (`useSessionService.ts` line 718–720):
```typescript
case "sessionUpdated": {
    const session = event.event.value.session;
    dispatch(upsertSession(session));
    break;
}
```

This calls `upsertSession` which writes to the entity adapter only — it does NOT touch `detectedStatusMap`.

**What `SessionUpdatedEvent` carries today:**
- Full `Session` proto (all fields including `subStatus`, `workingState`, `status`, etc.)
- `updatedFields` list (strings)
- `subStatus` is derived at read time by `toProtoSubStatus()` from `inst.GetDetectedStatus()`
- **Does NOT carry `detectedStatus` raw string** — that only travels via `StatusChangedEvent`

## 5. Gap Analysis: What Changes to Merge the Two Paths

### Information currently in StatusChangedEvent NOT in SessionUpdatedEvent

| Field | StatusChangedEvent | SessionUpdatedEvent |
|---|---|---|
| `detectedStatus` (raw string or future proto enum) | YES (optional field 4) | NO — missing entirely |
| `detectedContext` (human-readable string) | YES (optional field 5) | NO — missing entirely |
| `oldStatus` | YES | NO (not needed for unification) |
| `newStatus` (as an explicit field, not session.status) | YES | Implicit in Session.status |

The `Session` proto already carries `subStatus` (computed from `DetectedStatus` at snapshot time) and `workingState`. But neither of those is the same as the raw `detectedStatus` string that flows through `StatusChangedEvent` to `detectedStatusMap` in Redux.

The critical gap: `upsertSession` writes the entity (updating `session.subStatus`) but leaves `detectedStatusMap` alone. `updateSessionStatus` writes `detectedStatusMap` but leaves the entity only partially updated (status + subStatus; does NOT do a full `InstanceToProto` snapshot). These two operations are always split across two different events.

### What must change for R3 (SessionUpdatedEvent carries detection info)

**Backend changes:**
1. Add `detected_status` (typed proto enum, per R1) and `detected_context` fields to `SessionUpdatedEvent` in `proto/session/v1/events.proto`
2. Run `make generate-proto`
3. `NewSessionUpdatedEvent` constructor in `pkg/events/types.go` needs to accept (or the caller must set) `DetectedStatus` and `DetectedContext` on the `Event` struct — or a new constructor overload
4. `event_converter.go` `EventSessionUpdated` case must populate the new fields from `event.Session`'s detection state. Currently it calls `adapters.InstanceToProto` which does include `subStatus` but not raw `detectedStatus`. To populate the new wire fields, either: (a) read `inst.GetDetectedStatus()` in the converter, or (b) store the raw value on the `Event` struct at publish time.
5. Every `SessionUpdatedEvent` publish site that involves a status change must include detection info. The critical one is `sessionExitedPublisher` — it currently only passes `[]string{"status"}` and the `Event.DetectedStatus` field is not set (empty).

**Frontend changes:**
1. `handleSessionEvent` `"sessionUpdated"` case must also update `detectedStatusMap` from the new typed field — either inside `upsertSession` (R4.2/R4.3) or as a separate dispatch after
2. `upsertSession` reducer must atomically clear or update `detectedStatusMap` based on `session.status` and the new `detectedStatus` field (R4.2/R4.3)
3. `updateSessionStatus` action becomes redundant once `upsertSession` owns both paths

### What the StatusChangedEvent carries that SessionUpdatedEvent does not need

`oldStatus` — no consumer on the frontend uses `oldStatus` from the event. The Redux `updateSessionStatus` reducer ignores it. It is safe to drop once unified.

`workingState` on `SessionStatusChangedEvent` — this is already present on `Session` proto via field 50 (`working_state`). No additional change needed.

## 6. Key Invariants / Pitfalls

- `toProtoSubStatus` returns `SUB_STATUS_UNSPECIFIED` for non-Active sessions — this is the correct behavior and must be preserved in R4.2 (clearing `detectedStatusMap` when not active)
- `StatusActive` (Go constant 8) maps to "Active" string, which maps to `getDetectedStatusInfo` returning `{ label: "Active", icon: "⚡", variant: "active" }` in StatusBadge — this is one of the rename targets (R2.1)
- `StatusReady` (Go constant 1) is the `.*` catch-all but renders as "Ready" (green ✅) in StatusBadge — the misleading catch-all problem noted in R2.2
- `SessionCard.tsx` lines 509–512 has string equality checks on `detectedStatus` for suppression logic: `detectedStatus === "Needs Approval"` and `detectedStatus === "Input Required"` — these will break if the raw strings change without frontend updates (exactly the fragility R6.1 targets)
- The `DetectionEventsPanel.tsx` (debug panel) also hardcodes string→SubStatus mappings (e.g., `8: "StatusActive"`) that would need updating

## Files Requiring Changes for R3/R4 Unification

**Go (backend):**
- `proto/session/v1/types.proto` — add `DetectedStatus` proto enum (R1.1)
- `proto/session/v1/events.proto` — add `detected_status` typed field + `detected_context` to `SessionUpdatedEvent`
- `pkg/events/types.go` — `Event` struct already has `DetectedStatus`/`DetectedContext` fields (used by StatusChangedEvent); no struct change needed — the converter just needs to read them for SessionUpdated events too
- `server/services/event_converter.go` — populate new fields in `EventSessionUpdated` case
- `server/services/session_service.go` — `sessionExitedPublisher` must set `DetectedStatus`/`DetectedContext` on the event before publishing

**TypeScript (frontend):**
- `web-app/src/gen/` — regenerated bindings (auto)
- `web-app/src/lib/hooks/useSessionService.ts` — `"sessionUpdated"` case may remain as-is if `upsertSession` handles `detectedStatusMap`
- `web-app/src/lib/store/sessionsSlice.ts` — `upsertSession` reducer gets `detectedStatusMap` write logic; `updateSessionStatus` becomes deprecated
- `web-app/src/components/sessions/StatusBadge.tsx` — switch on typed enum (R6.2)
- `web-app/src/components/sessions/SessionCard.tsx` — remove string equality suppression checks (R6.1)
- `web-app/src/components/sessions/DetectionEventsPanel.tsx` — update hardcoded string map
