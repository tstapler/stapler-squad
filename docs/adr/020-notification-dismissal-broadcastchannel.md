# ADR-020: Notification Dismissal Cross-Tab Sync — BroadcastChannel vs. Server-Side Persistence

**Status**: Accepted
**Date**: 2026-06-20

## Context

Notification dismissal state is currently stored only in `localStorage` via `notificationStorage.ts`. When a user dismisses a notification in one browser tab, the other open tabs do not receive any signal — they continue to show the unread badge and indicator until a full page reload or navigation. This is the gap addressed by Epic 5 of the event-pipeline-consistency project (Stories 5.1–5.3).

Two mechanisms are viable for cross-tab dismissal sync:

**Option A — BroadcastChannel (same-browser tab sync):**
The `BroadcastChannel` API lets same-origin browsing contexts in the same browser exchange messages without a server round-trip. When a user dismisses a notification, the initiating tab writes to `localStorage` (existing behavior) and posts a typed message to a named channel. All other tabs that have subscribed receive the message and update their Redux state and badge count immediately. Tabs opened after the broadcast message was sent do not receive the message (fire-and-forget), but they re-read `localStorage` on mount and therefore start in the correct dismissed state.

`BroadcastChannel` is available in Chrome 54+, Firefox 38+, and Safari 15.4+ (MDN baseline 2022). No backend changes are needed. The same-tab sender does not receive its own message (specified by the browser API), so there is no risk of infinite loops.

**Option B — Server in-memory store + `WatchSessions` stream event (Story 5.4):**
A new `DismissNotification` RPC writes the dismissed notification ID into a server-side in-memory map (`map[string]bool`, protected by a mutex). The server emits a `EventNotificationDismissed` event on the event bus; `event_converter.go` forwards it to the `WatchSessions` stream (using next safe field number 11 in `SessionEvent.oneof`); all connected tabs receive the event and update their Redux state. The in-memory map is ephemeral — it does not survive server restarts, which is acceptable for notification state (per project requirements). Fresh tab connections re-hydrate from a new `GetDismissedNotifications` RPC. This approach is consistent with the existing event-driven architecture and would sync across any number of tabs regardless of when they opened.

The two options are mutually exclusive at runtime — shipping both would cause double-dismissal.

## Decision

Implement BroadcastChannel (Option A, Stories 5.1–5.3) as the primary mechanism. Story 5.4 (server RPC) is documented here as an optional upgrade path but is not shipped in the same PR.

Implementation:
1. A typed `BroadcastChannel` wrapper utility is created at `web-app/src/lib/utils/broadcastChannel.ts`, exposing `broadcast` and `subscribe` functions with a `NotificationSyncMessage` discriminated union. An SSR guard (`typeof window === "undefined"`) makes the utility safe in any server-rendering context.
2. At the point where `dismissNotification` writes to `localStorage`, the same call site also broadcasts `{ type: "NOTIFICATION_DISMISSED", notificationId }` to the channel. Only the initiating tab broadcasts; the subscriber handler calls `markAcknowledgedInStorage` without re-broadcasting (no loop).
3. On mount, `NotificationContext` (or `useNotifications`) subscribes to the channel and dispatches `dismissNotification` for any received message, triggering badge recalculation.

The 30-second localStorage poll (existing behavior) remains as a residual fallback.

## Consequences

### Positive
- Zero backend changes: no new RPC, no new proto field, no new event type.
- Fires instantly within the same browser — no network round-trip latency.
- Tabs opened before the dismissal receive the sync immediately via the channel message.
- Tabs opened after the dismissal receive the correct state from `localStorage` on mount.
- `BroadcastChannel` is a browser-native API with no polyfill needed for supported browsers.

### Negative / Risks
- Fire-and-forget: a tab opened in the window between when the dismissal broadcast fired and when `localStorage` was written (extremely narrow race) could miss the state. In practice this race is sub-millisecond and is not a realistic concern.
- No cross-device sync: a second device logged in to the same local server (e.g., via Tailscale) does not receive the BroadcastChannel message. This is acceptable for a single-user local-first app.
- No survival across browser restarts for in-flight dismissals: tabs from a previous browser session do not receive retroactive broadcasts. `localStorage` covers this case.
- Safari 15.4+ is required. Older Safari versions are unsupported.

### Upgrade Path (Story 5.4)
If cross-device sync or post-restart consistency becomes a requirement, the server-side alternative (Story 5.4) should replace the BroadcastChannel approach entirely. Story 5.4 adds:
- `DismissNotification` RPC in `proto/session/v1/session.proto`
- In-memory dismissed-IDs map in a new `notification_service.go` handler
- A new `EventNotificationDismissed` event forwarded via `event_converter.go` at `SessionEvent.oneof` field 11
- A `GetDismissedNotifications` RPC for fresh-tab re-hydration on `WatchSessions` reconnect
- Removal of the BroadcastChannel utility and its call sites

Do not ship both mechanisms simultaneously.
