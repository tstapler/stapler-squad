# Requirements: Web Push Notifications

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-16

## Problem Statement

Users miss important session events (completion, approval requests, errors) because they
switch contexts — move to another tab, put the device away, or work from a phone. There is
no cross-device awareness: events that need user attention stay invisible until the user
returns to the web UI. A React Native companion app is planned; its push delivery channel
(APNs/FCM) must be designed now so the backend notification architecture doesn't need to
be rebuilt when that app ships.

## Success Criteria

- Web push notifications fire in the browser (including when the tab is closed/backgrounded)
  for session-complete, approval-needed, and error events.
- Notifications are rich: session title, event type, and a meaningful body snippet are
  visible in the OS notification without opening the app.
- Clicking a notification deep-links to the correct session in the web UI.
- The Go backend payload format is compatible with FCM/APNs so the React Native app can
  subscribe to the same notification pipeline without a backend rewrite.
- A user can enable/disable push from the web UI settings, and the service worker
  subscription lifecycle (subscribe / resubscribe / unsubscribe on permission revoke) is
  handled correctly.

## Scope

### Must Have (MoSCoW)

- Fix known correctness issues in the existing `PushService` (mutex unlock mismatch,
  deduplication window gaps).
- Wire ALL notification-priority levels (not just HIGH) to push where semantically
  appropriate — specifically: approval_needed always fires push regardless of priority.
- Service worker consolidation: separate push-notification responsibilities from PWA
  caching concerns; ensure the SW handles `notificationclick` with correct deep-link URLs.
- Rich notification payload: include `sessionId`, `sessionTitle`, `notificationType`,
  `url` (deep-link), and optionally a `body` snippet in every push payload.
- Push subscription settings UI: permission request flow, subscribe/unsubscribe toggle,
  visible in a settings panel or notification preferences section of the web UI.
- Safari/macOS support: Web Push now supported in Safari 16+ via the standard Web Push API
  (not APNs-direct); ensure VAPID flow works in Safari.
- Backend payload schema documented and designed to be FCM/APNs forwardable (topic, data,
  notification envelope) so the RN app pipeline is a thin adapter, not a new design.

### Out of Scope

- React Native app itself (infrastructure only in this project).
- Email or SMS notifications.
- Push notification analytics / delivery tracking.
- Multi-user or team notification routing.
- Notification history inbox/feed UI changes (existing `NotificationPanel` is sufficient
  baseline; additive improvements only if they fall naturally from push work).

## Constraints

Tech stack: Go backend (existing), React + Next.js web UI (existing), ConnectRPC for
internal APIs, VAPID-based web push already implemented.

Dependencies: Branch `stapler-squad-web-push-support` already contains substantial
in-progress push infrastructure — this project completes and hardens that work rather
than starting fresh.

Solo developer: scope to deliverable increments; no single PR should be a mega-change.

## Context

### Existing Work

Substantial push infrastructure already exists on this branch:

| Layer | What exists | Known gaps |
|-------|-------------|------------|
| Go: `server/services/push_service.go` | VAPID keygen, subscription store, `SendNotification` | `Subscribe()` has `defer ps.mu.RUnlock()` instead of `Unlock()` — mutex bug |
| Go: `server/push/subscriber.go` | EventBus subscriber → push delivery | Only HIGH priority (int32=3) + session-stopped + needs-approval wired; magic int comparison vs. proto enum |
| Go: `server/notifications/subscriber.go` | EventBus → history store with 500ms coalescing | Separate from push subscriber; both consume the same event bus independently |
| Frontend: `usePushNotifications.ts` | VAPID subscribe / unsubscribe hook | Not wired to any settings UI yet |
| Frontend: `public/push-sw.js` | Push event handler + notificationclick + PWA caching | Conflates push SW and caching SW; deep-link URLs use session title (not ID) |
| Frontend: `NotificationPanel.tsx` | Rich in-app notification panel with approval UI | No push enable/disable toggle |
| Proto: `NotificationEvent`, `NotificationType`, `NotificationPriority` | Full type system defined | — |

Key decisions already made:
- Self-hosted VAPID (no Firebase/OneSignal dependency)
- `SherClockHolmes/webpush-go` library for push delivery
- JSON file storage for subscriptions and notification history
- EventBus decouples notification sources from delivery targets

### Stakeholders

Solo developer (project owner and sole user). React Native app is the downstream consumer
of the notification architecture decisions made in this project.

## Research Dimensions Needed

- [x] Stack — self-hosted VAPID already chosen; focus on Safari Web Push compatibility
      and FCM/APNs forwarding patterns for the RN-ready requirement
- [ ] Features — survey what rich notification payloads look like across platforms;
      notification action button patterns; session deep-link URL schemes
- [ ] Architecture — how to structure the push subscriber to be extensible (multiple
      delivery targets: web push today, FCM/APNs tomorrow); notification preference
      storage model
- [ ] Pitfalls — VAPID key rotation, service worker update lifecycle gotchas, Safari
      push quirks, subscription expiry handling, permission prompt timing UX
