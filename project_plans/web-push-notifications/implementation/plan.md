# Implementation Plan: Web Push Notifications

Status: Ready for implementation
Phase: 3 - Planning complete
Date: 2026-04-17

Input artifacts:
- requirements.md
- research/findings-stack.md
- research/findings-features.md
- research/findings-architecture.md
- research/findings-pitfalls.md
- research/synthesis.md

Output artifact: this file
Next: `/quality:test-planner` → implementation/validation.md

---

## Architecture Decisions

| # | File | Decision |
|---|------|----------|
| ADR-001 | decisions/ADR-001-notifier-interface.md | Use Notifier interface slice for multi-target delivery; WebPushNotifier today, FCMNotifier later |
| ADR-002 | decisions/ADR-002-preference-storage-config-json.md | Extend config.json with NotificationPrefs; harden saveConfig with atomic temp-rename |
| ADR-003 | decisions/ADR-003-trigger-rules-proto-constants.md | Hard-coded trigger rules referencing proto enum constants; URGENT fix; approval override |

---

## Epic Overview

**User value**: Users receive browser push notifications for session completions,
approvals, and errors even when the tab is backgrounded or the device is away.
Clicking a notification deep-links directly to the relevant session.

**Success metrics**:
- Push fires in Chrome, Firefox, Edge for session-complete, approval-needed, error events
- Notifications survive tab-closed state (service worker receives push)
- Deep-link navigates to correct session on click
- Settings panel shows subscribe/unsubscribe toggle with correct permission state
- Safari 16+ macOS receives push (VAPID, zero backend changes)
- Backend payload format is FCM-compatible (data map with sessionId, notificationType, url)

**Scope**: Harden and complete the existing in-progress push infrastructure. No new
third-party services. No React Native app (infrastructure only).

**Key constraint**: StartPushSubscriber is currently dead code (not wired in server.go).
Wiring is the prerequisite for all other work.

---

## Known Bugs (Pre-existing)

| ID | Severity | File | Description | Fix in |
|----|----------|------|-------------|--------|
| BUG-1 | Critical | server/services/push_service.go:143 | `defer ps.mu.RUnlock()` after `Lock()` → deadlock | Story 1, Task 1.2 |
| BUG-2 | Medium | server/push/subscriber.go:79 | `int32(3)` misses URGENT (4) | Story 2, Task 2.3 |
| BUG-3 | Medium | public/push-sw.js | SW conflates PWA caching + push handler | Story 4, Task 4.3 |
| BUG-4 | Medium | server/push/subscriber.go:59,73 | Deep-link uses Session.Title not stable ID | Story 2, Task 2.4 |
| FM-2 | High | server/services/push_service.go | 410 Gone responses not handled; dead subscriptions accumulate | Story 1, Task 1.3 |
| FM-4 | Medium | usePushNotifications.ts | Permission revocation not detected (no onchange listener) | Story 3, Task 3.1 |

---

## Story Breakdown

### Story 1: Wire, Smoke-test, and Fix Critical Bugs [~3 days]

**Prerequisite story. Nothing works until push is wired.**

Acceptance criteria:
- `PushService` is constructed and `StartDeliverySubscriber` is registered in server.go
- A test push fires end-to-end in Chrome (subscription received → notification displayed)
- The mutex deadlock bug is fixed; `Subscribe()` no longer panics or deadlocks
- HTTP 410/404 responses from push endpoints cause the subscription to be removed from storage
- `resp.Body.Close()` is called on all SendNotification responses (no connection leaks)

#### Task 1.1: Wire PushService + StartDeliverySubscriber in server.go [Small, 2h]

**Objective**: Register push infrastructure in the server startup path so delivery is no longer dead code.

Files (primary + supporting):
- `server/server.go` — add PushService construction + StartDeliverySubscriber call
- `server/push/subscriber.go` — verify StartPushSubscriber signature; adapt if needed
- `server/services/push_service.go` — verify constructor signature

Prerequisites:
- Read server.go to understand existing service construction pattern
- Read subscriber.go to understand current StartPushSubscriber signature

Implementation:
1. Construct `PushService` in server startup (follow pattern of other services; use config dir path)
2. Call `StartDeliverySubscriber(ctx, eventBus, []Notifier{webPushNotifier})` — or the current `StartPushSubscriber(ctx, eventBus, pushService)` if the Notifier refactor is deferred to Story 2
3. Ensure the push subscriber goroutine is started with the server context so it stops on shutdown
4. Log startup confirmation: "push delivery subscriber started"

Validation:
- `make build` passes
- Manual smoke test: subscribe in Chrome, trigger a session-complete event, notification appears
- No goroutine leak on server shutdown (subscriber exits when context is cancelled)

#### Task 1.2: Fix Mutex Deadlock in PushService.Subscribe [Micro, 1h]

**Objective**: Fix the one-character bug that causes a deadlock on the first subscription attempt.

Files:
- `server/services/push_service.go` — fix line 143
- `server/services/push_service_test.go` — add concurrent subscribe test

Prerequisites: read push_service.go lines 135–155

Implementation:
1. Change `defer ps.mu.RUnlock()` → `defer ps.mu.Unlock()` at line 143
2. Add a test that calls `Subscribe()` concurrently from two goroutines and asserts no panic/deadlock
3. Verify `Unsubscribe()` and `GetSubscriptions()` use matching Lock/Unlock or RLock/RUnlock pairs

Validation:
- `go test ./server/services/... -race` passes (race detector catches wrong-lock combinations)
- Subscribe no longer deadlocks when called twice in the same process

#### Task 1.3: Handle HTTP 410/404 in SendNotification — Remove Dead Subscriptions [Small, 2h]

**Objective**: Stop accumulating stale subscriptions that waste HTTP calls.

Files:
- `server/services/push_service.go` — SendNotification + sendToSubscription
- `server/services/push_service_test.go` — 410/404 handling test

Prerequisites: read sendToSubscription; understand webpush.SendNotification return type `(*http.Response, error)`

Implementation:
1. Capture the `*http.Response` from `webpush.SendNotification` (currently discarded with `_`)
2. After the call, check `resp.StatusCode`:
   - 201 / 202: success, close body
   - 410 or 404: call `ps.Unsubscribe(sub.Endpoint)`; close body; log "removed stale subscription"
   - 413: log "payload too large"; close body
   - 429: log "rate limited"; read Retry-After header if present; close body
   - other errors: log with status code; close body
3. Always call `resp.Body.Close()` in all branches (connection leak fix)
4. Add a context with 10-second deadline to each `SendNotification` call to prevent indefinite blocking

Validation:
- Unit test with mock HTTP server returning 410 verifies subscription is deleted
- Unit test with 201 response verifies subscription is retained
- No HTTP connection leaks under `-race`

---

### Story 2: Notifier Interface Refactor + P1 Bug Fixes [~3 days]

Acceptance criteria:
- `Notifier` interface defined in `server/push/`; `WebPushNotifier` wraps `PushService`
- `StartDeliverySubscriber` accepts `[]Notifier` (matches ADR-001)
- URGENT notifications trigger push (proto enum constant replaces magic `int32(3)`)
- `approval_needed` via `EventNotification` path also triggers push
- Deep-link URLs use `url.QueryEscape(session.ID)` not `.Title`
- Wrong Safari comment removed from push_service.go
- Unit test table covers all (EventType, Priority, NotificationType) → shouldNotify rules

#### Task 2.1: Define Notifier Interface + WebPushNotifier [Small, 2h]

**Objective**: Introduce the extensibility interface that lets FCMNotifier be added later without touching the subscriber.

Files:
- `server/push/notifier.go` (new) — Notifier interface + DeliveryNotification struct + WebPushNotifier
- `server/push/subscriber.go` — update to use Notifier; rename StartPushSubscriber
- `server/services/push_service.go` — remove direct send call (delegated to WebPushNotifier)

Prerequisites:
- Complete Task 1.1 (push is wired)
- Read findings-architecture.md Q1 section
- Read notifications/subscriber.go Appender interface as the pattern to mirror

Implementation:
1. Create `server/push/notifier.go`:
   - Define `DeliveryNotification` struct (Title, Body, Icon, Tag, Renotify, RequireInteraction, Data map, Actions)
   - Define `Notifier interface { Send(ctx, DeliveryNotification) error; Name() string }`
   - Implement `WebPushNotifier struct { svc *services.PushService }`
   - `WebPushNotifier.Send` converts `DeliveryNotification` → `PushNotification` and calls `svc.SendNotification`
2. Rename `StartPushSubscriber` → `StartDeliverySubscriber`; change signature to accept `[]Notifier`
3. Replace `pushService.SendNotification(...)` call with `for _, n := range notifiers { n.Send(ctx, notification) }`
4. In the loop, use a per-iteration context with 10s deadline; log error from each Send but continue loop

Validation:
- `make build` passes
- `go test ./server/push/...` with a `mockNotifier` that records calls
- FCMNotifier stub (empty struct implementing interface) compiles — proves extensibility

#### Task 2.2: Define Trigger Constants + Fix shouldNotify Logic [Small, 2h]

**Objective**: Replace magic `int32(3)` with named constants; fix URGENT gap; add approval override in EventNotification branch.

Files:
- `server/push/subscriber.go` — shouldNotify logic
- `server/push/trigger_constants.go` (new) — package-level proto-mirrored constants
- `server/push/subscriber_test.go` — unit test table for shouldNotify

Prerequisites: read types.proto for enum values; read subscriber.go lines 60-100

Implementation:
1. Create `trigger_constants.go` with package-level `const` block mirroring proto enums
2. In `subscriber.go` EventNotification case:
   - Replace `int32(3)` with `priorityHigh`; change `==` to `>=` to cover URGENT
   - Add `|| event.NotificationType == typeApproval` to the condition
3. In `EventSessionStatusChanged` case: verify NeedsApproval check is correct; add explicit constant reference
4. Write test table in `subscriber_test.go` covering: LOW priority generic → no push, HIGH → push, URGENT → push, APPROVAL_NEEDED at LOW priority → push, session-stopped → push

Validation:
- All test table cases pass
- `go test ./server/push/... -v` shows each case labelled
- Remove the incorrect `// Only works for Chrome/Firefox push (not Safari)` comment from push_service.go

#### Task 2.3: Fix Deep-Link URL and Tag Suffix — Use session.ID [Micro, 1h]

**Objective**: Forward-compatible URLs that survive session renames.

Files:
- `server/push/subscriber.go` — URL and tag construction (3 locations)

Prerequisites: confirm `event.Session.ID` is available in the Event struct

Implementation:
1. Import `net/url` if not already present
2. Replace `event.Session.Title` with `url.QueryEscape(event.Session.ID)` in URL construction
3. Replace `event.Session.Title` suffix in `tag` fields with `event.Session.ID` (no encode needed for tag)
4. Update `notificationTag` variable to use `.ID` as suffix

Validation:
- `make build` passes (compilation confirms field access)
- No functional change observable today (ID == Title); forward-compatibility verified by reading proto comment

---

### Story 3: Push Settings UI + Permission Lifecycle [~3 days]

Acceptance criteria:
- `usePushNotifications.ts` detects permission revocation while app is open
- Settings panel shows subscribe/unsubscribe toggle with three states: enabled, disabled, blocked
- Toggle invokes `requestPermission()` from a user gesture
- Denied state shows instructional text, not a broken subscribe button
- `NotificationPrefs` is persisted in config.json; `saveConfig` uses atomic write

#### Task 3.1: Add Permission onchange Listener in usePushNotifications.ts [Micro, 1h]

**Objective**: Detect mid-session permission revocation so the UI reflects actual state.

Files:
- `web-app/src/hooks/usePushNotifications.ts` — add Permissions API listener

Prerequisites: read the full hook to understand current state management

Implementation:
1. After mounting, query `navigator.permissions.query({ name: 'notifications' })`
2. Set `permissionStatus.onchange = () => setPermission(permissionStatus.state)`
3. Clean up the listener in the hook's cleanup return
4. Handle the case where `navigator.permissions` is unavailable (Firefox < 46; graceful skip)

Validation:
- Manual test: revoke permission in Chrome settings while app is open → UI updates without reload
- TypeScript compiles with no errors

#### Task 3.2: Add NotificationPrefs to Config + Atomic saveConfig [Small, 2h]

**Objective**: Persist notification preferences (per ADR-002); harden config writes.

Files:
- `config/config.go` — NotificationPrefs struct + Config field + saveConfig hardening
- `config/config_test.go` — round-trip test + concurrent write test

Prerequisites:
- Read config.go to understand existing Config struct and version migration pattern
- Read notifications/store.go for atomic write pattern to copy

Implementation:
1. Add `NotificationPrefs` struct with `PushEnabled bool`
2. Add `Notifications NotificationPrefs` field to `Config` with `json:"notifications,omitempty"`
3. Add nil-safe init for `Notifications` in `LoadConfig` (after unmarshal, set defaults if zero)
4. Bump `ConfigVersion` to 2; add an empty migration step (no-op, for v1 → v2 log message)
5. Refactor `saveConfig` to write to a `.tmp` file then `os.Rename` atomically
6. Add `sync.Mutex` (or confirm existing mutex) for in-process concurrent access to config

Validation:
- `go test ./config/... -race` passes
- Round-trip: marshal → unmarshal → assert `Notifications` present with correct defaults
- `ConfigVersion` migration test: load a v1 config, assert v2 fields have defaults

#### Task 3.3: Build PushNotificationSettings React Component [Medium, 3h]

**Objective**: Settings panel UI for the push subscribe/unsubscribe lifecycle.

Files:
- `web-app/src/components/settings/PushNotificationSettings.tsx` (new)
- `web-app/src/components/settings/PushNotificationSettings.css.ts` (new) — vanilla-extract

Prerequisites:
- Read `usePushNotifications.ts` to understand hook API (subscribe, unsubscribe, permission, isSupported)
- Read existing settings panel component to understand layout conventions
- Read css-architecture.md rule for vanilla-extract patterns

Implementation:
1. Component reads `{ subscribe, unsubscribe, isSubscribed, permission, isSupported }` from hook
2. Three render states:
   - **Not supported** (`isSupported === false`): informational text "Push notifications require a modern browser with service worker support"
   - **Denied** (`permission === 'denied'`): text "Notifications blocked in browser settings" + link to browser help
   - **Supported + default/granted**: toggle showing current subscription state; on toggle, call `subscribe()` or `unsubscribe()`
3. Toggle must call `subscribe()` from within a button click handler (user gesture requirement)
4. Use vanilla-extract for styles; import from shared theme vars

Validation:
- TypeScript compiles with no errors
- All three states render without runtime errors
- Manual: toggle enables push in Chrome; toggle disables and server-side subscription is removed

#### Task 3.4: Wire PushNotificationSettings into Settings Panel [Small, 2h]

**Objective**: Make the push settings discoverable in the existing app settings UI.

Files:
- Existing settings panel component (identify location in codebase)
- `web-app/src/components/settings/PushNotificationSettings.tsx` — import
- `web-app/src/hooks/usePushNotifications.ts` — hook is already used; confirm provider location

Prerequisites:
- Find the settings panel file (search `settings` in web-app/src/components)
- Confirm `usePushNotifications` hook is correctly instantiated (context vs. direct use)

Implementation:
1. Import `PushNotificationSettings` component into the settings panel
2. Add a "Notifications" section heading
3. Render `<PushNotificationSettings />` within that section
4. Verify the component receives the hook correctly

Validation:
- Settings panel renders with the push toggle section visible
- No TypeScript errors
- Toggle interacts with the push subscription API end-to-end

---

### Story 4: Rich Payload Enrichment + Service Worker Split [~4 days]

Acceptance criteria:
- Push payload includes `notificationType`, `sessionId` (session.ID), `timestamp`, `renotify`, `requireInteraction`, and `actions` array
- Service worker reads `event.data.json().actions` and uses them in `showNotification`
- `notificationclick` handler dispatches per `event.action` value
- Push service worker and PWA caching service worker are separated into distinct files
- SW registration updated to register the push SW independently

#### Task 4.1: Enrich Push Payload — Actions, Types, renotify, TTL [Small, 2h]

**Objective**: Make the notification informative and correctly interrupt the user.

Files:
- `server/push/subscriber.go` — payload construction per event type
- `server/services/push_service.go` — PushNotification struct extension

Prerequisites:
- Read findings-features.md Q1 and Q2 for per-type action table
- Read current PushNotification struct definition

Implementation:
1. Add `Actions []PushAction`, `Renotify bool`, `Badge string` to `PushNotification` struct
2. Add `PushAction struct { Action, Title, Icon string }` to push_service.go
3. In subscriber.go, add per-type payload construction:
   - `APPROVAL_NEEDED`: `requireInteraction=true, renotify=true, TTL=7200, actions=[{Review, open}, {Later, dismiss}]`
   - `session-stopped/task-complete`: `requireInteraction=false, renotify=false, TTL=86400, actions=[{View, open}, {Dismiss, dismiss}]`
   - `ERROR`: `requireInteraction=true, renotify=false, TTL=86400, actions=[{View Error, open}]`
4. Add `notificationType` and `timestamp` (Unix seconds) to the `data` map
5. Add a payload size guard: if `len(jsonPayload) > 3800` bytes, truncate `Body` to fit

Validation:
- `go test ./server/push/...` passes; test table covers per-type field values
- No payload exceeds 3900 bytes in tests

#### Task 4.2: Update Service Worker — Payload-Driven Actions + Click Dispatch [Medium, 3h]

**Objective**: SW displays the server-provided actions instead of hard-coded [Open, Dismiss]; handles action clicks correctly.

Files:
- `web-app/public/push-sw.js` — push event handler and notificationclick handler

Prerequisites:
- Read the current push event handler in push-sw.js
- Understand `event.data.json()` format after payload enrichment from Task 4.1

Implementation:
1. In `push` event handler:
   - Read `const payload = event.data.json()`
   - Pass `payload.actions || defaultActions` to `showNotification`
   - Pass `requireInteraction: payload.requireInteraction ?? false`
   - Pass `renotify: payload.renotify ?? false`
   - Pass `data: { url: payload.data?.url, ... }`
2. In `notificationclick` event handler:
   - If `event.action === 'dismiss'`: close notification, do nothing
   - If `event.action === 'open'` or no action: open/focus the app at `event.notification.data.url`
   - Wrap `clients.openWindow` in `event.waitUntil`
3. Add fallback default actions `[{ action: 'open', title: 'Open' }]` for any payload missing the field

Validation:
- Manual: receive a push → correct actions appear on Chrome desktop
- Manual: click "Review" on approval_needed → app opens at correct session URL
- Safari: notification body click still works (no actions rendered, fallback click to `data.url`)

#### Task 4.3: Split Service Worker — Separate Push from PWA Caching [Large, 4h]

**Objective**: Decouple the caching SW update lifecycle from the push handler update lifecycle.

Files:
- `web-app/public/push-sw.js` — retain only push/notificationclick handlers
- `web-app/public/cache-sw.js` (new) — extract install, activate, fetch caching handlers
- SW registration code (Next.js app layout or `_app`) — register both separately

Prerequisites:
- Read all of push-sw.js to identify exactly which events belong to each concern
- Understand how Next.js registers the service worker (check next.config.js and/or layout)

Implementation:
1. Create `cache-sw.js` with: `install` (cache assets), `activate` (cleanup old caches), `fetch` (serve from cache)
2. Strip `push-sw.js` to only: `push` event handler, `notificationclick` event handler, `skipWaiting`
3. Keep `self.skipWaiting()` in `push-sw.js` install; keep `self.clients.claim()` in activate
4. Update SW registration to register both files:
   - Push SW at scope `/` for notification delivery
   - Cache SW at scope `/` for asset caching (may need different scope strategy)
5. Verify push subscription endpoint is associated with the push SW registration

Validation:
- Browser DevTools → Application → Service Workers shows two workers registered
- Push notifications still received after cache-sw.js update cycle
- Asset caching still works after push-sw.js update cycle
- `make build` passes

---

## Dependency Visualization

```
Story 1 (Wire + P0 bugs) — PREREQUISITE
├── Task 1.1: Wire server.go ──────────────────────────────┐
├── Task 1.2: Fix mutex (can parallel with 1.1)            │
└── Task 1.3: 410 handling (can parallel with 1.2)         │
                                                            ▼
Story 2 (Notifier refactor + P1 bugs)            depends on Story 1
├── Task 2.1: Notifier interface ──────────────────────────┐
├── Task 2.2: Trigger constants (parallel with 2.1)        │
└── Task 2.3: Deep-link fix (parallel with 2.2)            │
                                                            │
Story 3 (Settings UI)                            depends on Story 1
├── Task 3.1: onchange listener (independent)              │
├── Task 3.2: Config prefs (independent)                   │
├── Task 3.3: React component (depends on 3.1)             │
└── Task 3.4: Wire component (depends on 3.3)              │
                                                            │
Story 4 (Rich payload + SW split)                depends on Story 2
├── Task 4.1: Payload enrichment (depends on 2.1, 2.2) ───┤
├── Task 4.2: SW actions (depends on 4.1)                  │
└── Task 4.3: SW split (independent of 4.1, 4.2)          │
                                                            ▼
                                                        COMPLETE

Stories 2, 3, and 4 can interleave: Stories 3 and 4 share no dependency
on each other and can be worked in parallel with Story 2.
```

---

## Integration Checkpoints

**After Story 1**: End-to-end smoke test. Subscribe in Chrome → trigger session event → push fires. The mutex fix and 410 cleanup must both be confirmed via the race detector.

**After Story 2**: All five unit test table cases pass in subscriber_test.go. URGENT and APPROVAL_NEEDED notifications can be triggered by constructing test events. The Notifier interface compiles with a stub FCMNotifier.

**After Story 3**: Subscribe/unsubscribe toggle works in the settings panel. Revoking permission in Chrome settings while app is open causes the UI to update without reload.

**After Story 4 (final)**: Full end-to-end:
- Receive an `approval_needed` push → notification has "Review" button + stays on screen (`requireInteraction`)
- Click "Review" → navigates to correct session
- Push SW and cache SW have independent update cycles (verify in DevTools)
- All requirements.md success criteria are met

---

## Open Questions (must resolve before Story 3/4)

1. **Settings panel location**: where does the existing settings panel live in web-app/src? Find it before Task 3.4.
2. **SW registration mechanism**: does Next.js use next-pwa, a custom `_app`, or app router layout to register the SW? This determines how Task 4.3 updates the registration. Check `next.config.js` and the app layout.
3. **iOS 17/18 home screen requirement**: does iOS 17/18 lift the PWA install requirement for Web Push? Affects copy in the PushNotificationSettings component (whether to show an iOS hint).

---

## Non-Functional Requirements

**Performance**: `SendNotification` must not block the EventBus delivery goroutine. Each notifier call must run with a 10-second context deadline. The EventBus buffer (100 events default) is ample for the notification rate of a single-user session manager.

**Reliability**: Dead subscriptions cleaned up on 410. Connection pool not leaked (resp.Body.Close on all paths).

**Security**: VAPID private key never regenerated automatically. push-subscriptions.json written through the in-process mutex only.

**Maintainability**: Trigger rules covered by unit test table. Notifier interface makes FCMNotifier additive. Config schema versioned with nil-safe migration.

**Browser compatibility**: Degrade gracefully on Safari (no actions rendered; body click works). On unsupported browsers (`isSupported === false`), hide the subscribe CTA.
