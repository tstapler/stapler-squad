# Research Synthesis: Web Push Notifications

Created: 2026-04-17
Input: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md

---

## Decision Required

How should the existing web push infrastructure be hardened, completed, and extended to
support rich browser notifications with Safari compatibility and a React Native-ready
backend architecture?

---

## Context

The branch already contains substantial push infrastructure (VAPID backend, EventBus
subscriber, service worker, React hook). The work is incomplete and has confirmed bugs.
The decisions below determine how to fix and extend it without rearchitecting what works.

Key constraint: self-hosted VAPID is already decided. The research confirmed this is the
correct choice for browser push and is compatible with a future FCM dispatch layer.

---

## Options Considered

### Delivery architecture

| Option | Description | Extensibility | Complexity |
|--------|-------------|--------------|-----------|
| A — Flat branches | Add `if fcmService != nil` inside existing goroutine | Low — grows unboundedly | Minimal now; degrades linearly |
| B — Notifier interface slice | `[]Notifier` passed to delivery subscriber; `WebPushNotifier` today, `FCMNotifier` later | High — new target = new type | Low — one interface, one loop |
| C — Channel fan-out | Independent goroutines per delivery target | Medium | High — unnecessary for single-process server |
| D — Webhook outbound | POST to configurable URL | High for RN backend | Medium — adds network hop to a local server |

### Notification preference storage

| Option | Persistence | Complexity | Fit |
|--------|------------|-----------|-----|
| A — Extend config.json | Yes | None — uses existing load/save | Excellent — pattern already validated |
| B — Separate file | Yes | Low | Acceptable |
| C — Embed in history store | Yes | High — mixes responsibility | Poor |
| D — In-memory defaults | No | None | Fails UX requirement |

### Push trigger rules

| Option | Correctness | Flexibility | Complexity |
|--------|------------|------------|-----------|
| A — Table-driven config | Medium — approval_needed bypass possible | High | Medium |
| B — Per-type rules with proto constants | High — explicit, testable | Medium | Low |
| C — Priority threshold only | Medium — misses APPROVAL_NEEDED at LOW priority | Low | None |

---

## Dominant Trade-off

**Completeness vs. abstraction prematurity.** The system needs to work correctly today
(fix bugs, wire the subscriber, add settings UI) before optimising for extensibility
tomorrow (FCM, RN). However, one design decision (Notifier interface) costs almost
nothing now and saves a significant rework later. The interface should be introduced
alongside the bug fixes.

The second tension is **correctness vs. configuration flexibility**. For a solo developer
tool, hard-coded trigger rules using proto enum constants are correct, explicit, and
testable. Table-driven configuration adds indirection with no current consumer.

---

## Recommendation

### Architecture: Notifier interface (Option B-Q1)

Introduce a `Notifier` interface in `server/push/`. Rename `StartPushSubscriber` to
`StartDeliverySubscriber`. The immediate implementation has one notifier (`WebPushNotifier`
wrapping `*services.PushService`). When FCM is needed, add `FCMNotifier` without touching
existing code. This mirrors the `Appender` interface already used in `notifications/subscriber.go`.

Accept this cost: one extra interface definition (~10 lines).
Reject flat branches: every new delivery target bloats the subscriber function and
requires reconstructing all services in tests.
Reject channel fan-out: the EventBus already handles pub/sub; a second fan-out layer is
redundant complexity.

### Preference storage: Extend config.json (Option A-Q2)

Add `NotificationPrefs` to the existing `Config` struct. Use the existing nil-safe load
pattern. Harden `saveConfig` to use atomic temp-rename write (matching
`notifications/store.go`) before adding API-writable preference fields.

### Trigger rules: Per-type proto constants with approval override (Option B-Q3)

Replace `int32(3)` with `int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)`.
Add explicit `>= HIGH || type == APPROVAL_NEEDED` logic. Add a unit test table.
Table-driven config would be worth adding if the user reports "I got a push for X and
didn't want it" — that feedback loop does not exist yet.

### Bug fixes (sequenced by severity)

**P0 — Must fix before any testing:**
1. `push_service.go:143`: `defer ps.mu.RUnlock()` → `defer ps.mu.Unlock()` (deadlock)
2. `SendNotification`: Detect HTTP 410/404 and call `Unsubscribe(endpoint)` (subscription leak)
3. Wire `StartDeliverySubscriber` in `server/server.go` — currently unregistered / dead code

**P1 — Fix before marking push complete:**
4. Replace `int32(3)` with proto enum constant; add `APPROVAL_NEEDED` type override
5. Replace `event.Session.Title` with `event.Session.ID` (+ `url.QueryEscape`) in deep-link URLs
6. Add `PermissionStatus.onchange` listener in `usePushNotifications.ts`
7. Remove incorrect Safari comment; document HTTPS + user gesture requirements

**P2 — Before shipping settings UI:**
8. Split `push-sw.js`: separate push handler from PWA caching concern
9. Add push enable/disable toggle to settings panel using `usePushNotifications` hook

**P3 — Monitor:**
10. Track `webpush-go` PR #60 (crypto/ecdh migration); apply when merged

### Payload schema (RN-ready today)

The existing `PushNotification.Data` map already carries `sessionId`, `sessionTitle`,
`notificationType`, `url`. This is FCM `data` map-compatible. No restructuring needed.
When FCM dispatch is added, `FCMNotifier` maps this directly to FCM's `data` field.
The current struct is sufficient; add `timestamp` and `occurrenceCount` fields to
improve notification content richness (see findings-features.md).

### Safari support (zero backend changes)

Safari 16+ uses standard VAPID. The existing backend works. Required frontend changes:
- Gate all `Notification.requestPermission()` calls on user gesture
- Serve the app over HTTPS for non-localhost addresses
- Add iOS home screen installation hint to settings UI [TRAINING_ONLY - verify iOS 17/18 status]

---

## Open Questions Before Committing

- [ ] Is `StartPushSubscriber` / `PushService` currently registered anywhere in `server.go`
  or `server/dependencies.go`? If not, the first task is wiring it, and all other work
  depends on verifying that delivery actually fires end-to-end.
- [ ] Does `webpush-go`'s `GenerateVAPIDKeys` produce a 65-byte uncompressed P-256 key
  required by Safari? (Verify before claiming Safari support is done.)
- [ ] Does iOS 17/18 lift the home screen requirement? (Affects settings UI copy and
  whether iOS in-browser push is blocked by design.)

---

## Implementation Order (Phase 3 input)

Recommended task sequence for `plan.md`:

1. **Wire and smoke-test (Prerequisite)**: Confirm `PushService` + `StartDeliverySubscriber`
   are registered; send a test push end-to-end in Chrome.
2. **P0 bugs**: Mutex fix + 410 cleanup (can be done in one small PR).
3. **Notifier interface refactor**: Introduce `Notifier` interface, `WebPushNotifier` wrapper.
4. **P1 bugs**: Trigger rule fixes, deep-link ID fix, permission onchange listener, Safari comment.
5. **Settings UI**: Subscribe/unsubscribe toggle wired to `usePushNotifications` hook.
6. **Payload enrichment**: Add `notificationType`, `timestamp` to push payload; update SW
   to render type-specific action buttons.
7. **SW split**: Separate caching and push concerns in service worker.
8. **Config preferences**: Add `NotificationPrefs` to config; expose RPC; wire to settings UI.

---

## Sources

- `findings-stack.md` — Safari Web Push + FCM/APNs forwarding
- `findings-features.md` — Rich notification payloads + UX patterns
- `findings-architecture.md` — Multi-target push + preference storage
- `findings-pitfalls.md` — VAPID, service worker lifecycle, known bugs
- `requirements.md` — Project requirements and existing infrastructure inventory
