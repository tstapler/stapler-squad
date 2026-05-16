# Bug: Notifications not synced across devices on approval resolution

**Status**: Fixed
**Priority**: High
**Fixed in**: main (2026-05-12)

## Symptoms

1. When Device A resolves a command approval, Device B's notification panel did not update in real-time — users had to manually refresh.
2. Mobile devices that were offline/sleeping when an approval was resolved never received the notification that it had been actioned.
3. The notification history panel did not reflect updated `approval_decision` metadata after resolution.

## Root Cause

Three compounding issues:

1. **`ApprovalService.ResolveApproval()` never published to the event bus.** `EventApprovalResponse` existed in `pkg/events/types.go` and `proto/session/v1/events.proto` but was a dead event — nothing ever called `bus.Publish()` with it.

2. **`event_converter.go` had no case for `EventApprovalResponse`.** Even if the event had been published, the `convertEventToProto()` switch would have silently dropped it, so it would never reach connected WebSocket clients.

3. **`NotificationContext.tsx` merge logic skipped existing items.** When `refreshHistory()` was called after an approval response, the `useEffect` that syncs backend state was guarded by `prev.some(n => n.id === id)` — existing items were never updated with the server's authoritative version (including the stamped `approval_decision` metadata).

## Fix

**`server/services/approval_service.go`**:
- Added `eventBus *events.EventBus` field with `SetEventBus()` setter
- In `ResolveApproval()`: look up `sessionID` from the approval before resolving, then publish `events.NewApprovalResponseEvent(sessionID, approved, approvalID)` to the bus

**`server/services/session_service.go`**:
- Wire the event bus: `approvalSvc.SetEventBus(eventBus)` after construction

**`server/services/event_converter.go`**:
- Added `case events.EventApprovalResponse:` to `convertEventToProto()`, mapping it to `sessionv1.ApprovalResponseEvent`

**`web-app/src/lib/hooks/useSessionService.ts`**:
- Added `onApprovalResponse?: () => void` option
- Added `case "approvalResponse":` in `handleSessionEvent` that calls `onApprovalResponseRef.current?.()`

**`web-app/src/lib/contexts/SessionServiceContext.tsx`**:
- Passed `onApprovalResponse: refreshHistory` so approval resolution triggers a history refresh on all connected clients

**`web-app/src/lib/contexts/NotificationContext.tsx`**:
- Rewrote merge logic to use a two-pass strategy: Pass 1 walks existing local items and replaces them with the server-authoritative version (preserving local callbacks); Pass 2 appends backend items not present locally at all

## Regression Tests

- `TestResolveApproval_PublishesEventBusEvent` — asserts `EventApprovalResponse` arrives on the bus within 1s with correct sessionID, approved flag, and approval ID as context
- `TestResolveApproval_NoEventWhenApprovalNotFound` — asserts no event is published for an unknown approval ID
