# Research: Notification and Event Infrastructure

## Event Bus (`server/events/bus.go`)

```go
type EventBus struct {
    mu          sync.RWMutex
    subscribers map[string]chan *Event
    bufferSize  int
}
```

- `Subscribe(ctx context.Context) (<-chan *Event, string)` — creates a per-subscriber channel with configurable buffer (default 100)
- `Publish(event *Event)` — non-blocking fan-out; drops events for slow subscribers
- `Unsubscribe(id string)` — closes the subscriber channel; also auto-called on ctx cancel
- **This is the server-level bus** shared across all session events (different from ratelimit package's internal `EventBus`)

---

## Event Types (`server/events/types.go`)

```go
const (
    EventSessionCreated         EventType = "session.created"
    EventSessionUpdated         EventType = "session.updated"
    EventSessionDeleted         EventType = "session.deleted"
    EventSessionStatusChanged   EventType = "session.status_changed"
    EventUserInteraction        EventType = "session.user_interaction"
    EventSessionAcknowledged    EventType = "session.acknowledged"
    EventApprovalResponse       EventType = "session.approval_response"
    EventNotification           EventType = "session.notification"
)
```

The `Event` struct carries all event data including `NotificationTitle`, `NotificationMessage`, `NotificationMetadata`, `NotificationType`, `NotificationPriority` fields.

No `EventRateLimitDetected` or `EventRateLimitRecovered` type exists yet. The preferred approach for rate limit notifications is to either:
1. Reuse `EventNotification` — publish a notification event with appropriate type/title
2. Add new event types — add `EventRateLimitDetected`, `EventRateLimitRecovered` to the const list

---

## `NotificationService` (`server/services/notification_service.go`)

```go
type NotificationService struct {
    notificationStore       *notifications.NotificationHistoryStore
    notificationRateLimiter *NotificationRateLimiter
    eventBus                *events.EventBus
    reviewQueuePoller       *session.ReviewQueuePoller
}
```

Primary method: `SendNotification(ctx, req)` — validates localhost origin, rate-limits, publishes `EventNotification` to the event bus.

The `NotificationRateLimiter` already prevents spam; `Allow(sessionID)` must return true for the notification to be sent.

**Programmatic notification path** (what we need for rate limit):
```go
event := events.NewNotificationEvent(sessionID, sessionName, notificationID,
    int32(notificationType), int32(priority), title, message, metadata)
eventBus.Publish(event)
```

This is already how `SendNotification` works — we can call `eventBus.Publish` with a `NewNotificationEvent` directly from the session layer, bypassing the RPC method. But to respect `NotificationRateLimiter`, we should either use the service or replicate the Allow check.

---

## `event_converter.go` — Go events → proto events

`convertEventToProto(*events.Event)` maps event types to protobuf `SessionEvent` oneof variants:

- `EventSessionCreated` → `SessionEvent_SessionCreated` (calls `adapters.InstanceToProto`)
- `EventSessionUpdated` → `SessionEvent_SessionUpdated` (calls `adapters.InstanceToProto`)
- `EventSessionDeleted` → `SessionEvent_SessionDeleted`
- `EventSessionStatusChanged` → `SessionEvent_StatusChanged`
- `EventNotification` → `SessionEvent_Notification`

**No rate-limit-specific event type in the oneof.** Rate limit state changes need to go through one of:
- `EventSessionUpdated` with `RateLimitState` populated (standard — any session field change triggers this)
- `EventNotification` for toast/desktop notification (already handled)

The session update path is the right one for surfacing `RateLimitState` to the frontend: when rate limit is detected, publish `EventSessionUpdated` so `InstanceToProto()` gets called and the populated `RateLimitState` field flows to all connected clients.

---

## WebSocket path: events → frontend

1. `SessionService.WatchSessions()` subscribes to the server `EventBus`
2. Receives `*events.Event` from the channel
3. Calls `convertEventToProto()` to produce `SessionEvent`
4. Sends over ConnectRPC streaming response

Frontend receives `SessionEvent` via the WebSocket bridge, which was recently added (commit `a374322b` "feat(streaming): WebSocket bridge for Watch* RPCs + global session context").

---

## Frontend: how sessions are consumed

`SessionServiceContext.tsx`:
- `GlobalSessionServiceProvider` mounts a single persistent `watchSessions` connection
- Calls `useSessionService` (the ConnectRPC hook) and `useSessionNotifications`
- `useSessionNotifications` handles `EventNotification`-type events and calls `NotificationContext.addNotification()`

`NotificationContext.tsx` (`web-app/src/lib/contexts/NotificationContext.tsx`):
- `addNotification()` adds a toast and to history
- `addToHistoryOnly()` — history only, no toast
- `showSessionNotification()` — for ReviewItem-typed events
- Toast rendering via `NotificationToast` component

**Rate limit state changes** would arrive as `SessionUpdated` events in `useSessionService`, causing the session list to update. The `SessionCard` would then render the `rateLimitState` badge automatically (code already exists in `SessionCard.tsx`).

---

## `NotificationToast` (`web-app/src/components/ui/NotificationToast.tsx`)

Vanilla-extract styled toast component with:
- Auto-close timer (centralized in `notification-policy.ts`)
- Auto-minimize to compact pill
- Visible/exiting/minimized states
- Approval action buttons (approve/deny/view)
- Relative timestamp display

This component is fully ready to display rate limit notifications — no changes needed to the toast component itself.

---

## `NotificationRateLimiter` (server-side)

Found in `server/services/` — `Allow(sessionID string) bool`. Already gates the `SendNotification` RPC to prevent spam per-session.

---

## Integration Points for Rate Limit Notifications

To send a desktop/toast notification when rate limit is detected:

**Option A — Via `NotificationService.SendNotification` RPC** (not ideal: localhost-only, requires HTTP call from session layer)

**Option B — Direct `eventBus.Publish` from Manager callback**:
```go
// In Manager.handleDetection():
notificationEvent := events.NewNotificationEvent(
    sessionID, sessionTitle, uuid, 
    int32(sessionv1.NotificationType_NOTIFICATION_TYPE_RATE_LIMIT),
    int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
    "Rate limited — resumes at 11pm",
    "", nil,
)
serverEventBus.Publish(notificationEvent)
```
The Manager would need a reference to the server `events.EventBus` — passed in at construction time.

**Option C — Callback wiring from `Integration`**:
Add a `OnDetection func(Detection)` callback to `Integration`; wire it from `session.Instance` which already has access to the server event bus.

Option C is the cleanest: no new dependencies pushed into the ratelimit package.

---

## Status Change Events for Rate Limit State

When `RateLimitState` changes:
1. Call `inst.SetRateLimitEnabled()` is already wired
2. Need to publish `EventSessionUpdated` (or `EventSessionStatusChanged`) so connected WebSocket clients get the updated session proto with `RateLimitState` populated
3. The `reviewQueuePoller` already publishes `EventSessionUpdated` on poll — if rate limit state is populated in `InstanceToProto()`, it would flow through automatically on the next poll cycle

For real-time updates (< 2s per US-1), we need to trigger an `EventSessionUpdated` publish immediately on detection, not wait for the next poll cycle.
