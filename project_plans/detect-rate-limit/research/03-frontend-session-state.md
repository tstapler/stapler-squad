# Research: Frontend Session State and UI

## `RateLimitState` in Generated Proto Types

`SessionCard.tsx` line 6:
```typescript
import { Session, SessionStatus, ReviewItem, InstanceType, RateLimitState, CheckpointProto } 
  from "@/gen/session/v1/types_pb";
```

The `RateLimitState` enum is already imported and used. The generated TypeScript enum at `web-app/src/gen/session/v1/types_pb.ts` has all variants matching the proto:
- `RateLimitState.NONE`
- `RateLimitState.WAITING`
- `RateLimitState.RECOVERING`
- `RateLimitState.RECOVERED`
- `RateLimitState.FAILED`

The `Session` proto message has field `rateLimitState: RateLimitState` (proto field 40).

---

## `SessionCard.tsx` — Existing Rate Limit UI

Two helper functions are already fully implemented:

```typescript
const getRateLimitStateText = (state: RateLimitState): string => {
  switch (state) {
    case RateLimitState.NONE:      return "";
    case RateLimitState.WAITING:   return "Rate Limited";
    case RateLimitState.RECOVERING: return "Recovering...";
    case RateLimitState.RECOVERED: return "Recovered";
    case RateLimitState.FAILED:    return "Recovery Failed";
    default: return "";
  }
};

const getRateLimitStateColor = (state: RateLimitState): string => {
  switch (state) {
    case RateLimitState.NONE:      return "";
    case RateLimitState.WAITING:   return statusNeedsApproval;
    case RateLimitState.RECOVERING: return statusLoading;
    case RateLimitState.RECOVERED: return statusReady;
    case RateLimitState.FAILED:    return statusPaused;
    default: return "";
  }
};
```

The badge rendering already exists:
```typescript
{session.rateLimitState && session.rateLimitState !== RateLimitState.NONE && (
  <span
    className={`${status} ${getRateLimitStateColor(session.rateLimitState)}`}
    role="status"
    aria-label={`Rate limit: ${getRateLimitStateText(session.rateLimitState)}`}
  >
    {getRateLimitStateText(session.rateLimitState)}
  </span>
)}
```

**The UI is complete.** The only thing missing is the backend populating `session.rateLimitState` in `InstanceToProto()`. Once that is fixed, the badge will render automatically.

---

## Missing: Reset Time Display

The requirements mention showing "Rate limited until 11pm" with the actual reset timestamp. Currently:
- `session.rateLimitState` can be `WAITING` but there is no `resetTime` field in the `Session` proto
- The badge says "Rate Limited" but cannot show the specific reset time

**Gap**: The `Session` proto needs a `reset_time` field (a `google.protobuf.Timestamp`) populated when state is `WAITING`. The `Detector` stores `currentResetTime time.Time` but it's not accessible via `GetRateLimitState()` (only int state is returned).

To fix:
1. Add `google.protobuf.Timestamp rate_limit_reset_time = N` to `Session` proto
2. Add `GetRateLimitResetTime() time.Time` to `Instance` (calling through to ClaudeController → PTYConsumer → Manager → Detector)
3. Populate in `InstanceToProto()`
4. Update the badge text in `SessionCard` to format the time

---

## Session State Management in Frontend

### `SessionServiceContext.tsx`
```typescript
interface SessionServiceContextValue {
  sessions: Session[];
  pauseSession: (id: string) => Promise<Session | null>;
  watchSessions: (options?) => void;
  // ...
}
```
`GlobalSessionServiceProvider` calls `useSessionService` which manages a single `watchSessions` WebSocket connection. Sessions are updated in-place when `SessionUpdated` events arrive.

### `useSessionService` hook (`web-app/src/lib/hooks/useSessionService.ts`)
Handles:
- Initial session list load
- `SessionCreated` → add to list
- `SessionUpdated` → merge updated session into list (replaces by ID)
- `SessionDeleted` → remove from list
- `StatusChanged` → update status field

When `InstanceToProto()` starts populating `rateLimitState`, the `SessionUpdated` event path will automatically update `session.rateLimitState` in the React state, causing the card to re-render with the badge.

---

## Notification Components

### `NotificationToast.tsx` (`web-app/src/components/ui/NotificationToast.tsx`)
Full-featured toast component with:
- Auto-close timer (configured by `notification-policy.ts`)
- Auto-minimize to compact pill after 5s (Tier 2 default)
- Approve/Deny/View/Dismiss action buttons
- Session name + relative timestamp in header

No changes needed to `NotificationToast` for rate limit notifications — it renders any `NotificationData` generically.

### `NotificationContext.tsx` (`web-app/src/lib/contexts/NotificationContext.tsx`)
- `addNotification(Omit<NotificationData, "id" | "timestamp">)` — adds toast + history entry
- `addToHistoryOnly()` — history only
- Per-session deduplication: replaces existing toast for same `sessionId`

### `NotificationData` type (`web-app/src/lib/types/notification.ts`)
```typescript
interface NotificationData {
  id: string;
  sessionId?: string;
  sessionName?: string;
  title: string;
  message?: string;
  notificationType?: NotificationType;
  priority?: NotificationPriority;
  timestamp: number;
  onView?: () => void;
  onAcknowledge?: () => void;
}
```

Rate limit notifications just need a title like "Session X rate limited — resumes at 11pm" and will render correctly via existing toast infrastructure.

---

## Per-Session Toggle UI

### What exists
- `session.Instance.SetRateLimitEnabled(bool)` and `IsRateLimitEnabled() bool` in Go layer
- No proto field for `rateLimitEnabled` state
- No UI toggle in `SessionCard.tsx` or overflow menu

### What needs to be added
1. `bool rate_limit_enabled = N` field in `Session` proto (or toggle RPC)
2. Populate in `InstanceToProto()`
3. Add toggle to overflow menu in `SessionCard.tsx` (or session detail view)
4. Add `SetRateLimitEnabled` RPC or update-session field to the ConnectRPC service

The overflow menu already has room for a new item; the existing menu items follow a pattern of `onClick` handlers passed from the parent page component.

---

## Toast for Rate Limit — Integration Path

When rate limit is detected, the backend publishes `EventNotification` to the server event bus. The frontend receives it via the WebSocket stream in `useSessionNotifications`, which calls:

```typescript
notificationContext.addNotification({
  sessionId: event.sessionId,
  sessionName: event.sessionName,
  title: event.title,
  message: event.message,
  notificationType: mapNotificationType(event.notificationType),
  priority: mapPriority(event.priority),
});
```

This triggers a toast automatically. No new frontend code needed for the toast itself.

---

## Summary: Frontend Work Required

| Item | Status | Work Required |
|---|---|---|
| `RateLimitState` badge rendering in `SessionCard` | Done | None — just needs backend to populate |
| Reset time display ("until 11pm") | Missing | Add `resetTime` proto field + badge text update |
| Per-session toggle UI | Missing | Add to overflow menu + new RPC |
| Toast on detection | Ready | None once backend publishes EventNotification |
| Toast on recovery | Ready | None once backend publishes EventNotification |
| Session state update propagation | Ready | Works via `SessionUpdated` stream once adapter fixed |
