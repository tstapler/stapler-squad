# Implementation Plan: Web Push Notifications

Branch: `stapler-squad-web-push-support`
Status: Ready — start with Story 1
Full plan: `project_plans/web-push-notifications/implementation/plan.md`
ADRs: `project_plans/web-push-notifications/decisions/`

---

## Epic Overview

Complete and harden the in-progress web push notification system. Infrastructure on this branch
is partially implemented and has confirmed bugs. Goal: working push delivery with rich payloads,
a settings UI, and an architecture extensible to FCM for the future React Native app.

**Critical prerequisite**: `StartPushSubscriber` is currently dead code (not wired in server.go).
Nothing can be tested until Task 1.1 is done.

---

## Known Issues (Fix These First)

| ID | Severity | File | Bug |
|----|----------|------|-----|
| BUG-1 | 🔴 Critical | `server/services/push_service.go:143` | `defer ps.mu.RUnlock()` after `Lock()` → deadlock on first subscribe |
| FM-2 | 🔴 High | `server/services/push_service.go` | HTTP 410/404 responses ignored; dead subscriptions accumulate |
| BUG-2 | 🟡 Medium | `server/push/subscriber.go:79` | `int32(3)` misses URGENT (4); APPROVAL_NEEDED not checked in EventNotification |
| BUG-4 | 🟡 Medium | `server/push/subscriber.go:59,73` | Deep-link uses `Session.Title` not stable `Session.ID` |
| FM-4 | 🟡 Medium | `usePushNotifications.ts` | No `PermissionStatus.onchange` listener; UI stale after revoke |
| BUG-3 | 🟡 Medium | `public/push-sw.js` | SW conflates PWA caching + push handler (SW split needed) |

---

## Story 1: Wire, Smoke-test, and Fix Critical Bugs

**Status: TODO — start here**

Prereqs: none
Acceptance: push fires end-to-end in Chrome; mutex/410 bugs fixed; race detector clean

---

### Task 1.1 — Wire PushService + StartDeliverySubscriber [Small, 2h]

**Load these files:**
- `server/server.go` — find where services are constructed and started
- `server/push/subscriber.go` — `StartPushSubscriber` signature
- `server/services/push_service.go` — `NewPushService` constructor

**What to do:**
1. In `server/server.go`, construct `PushService` using the config directory path
2. Call `StartPushSubscriber(ctx, eventBus, pushService)` with the server's root context
3. Ensure the goroutine exits when the server context is cancelled
4. Add a log line: `"push delivery subscriber started"`

**Done when:**
- `make build` passes
- Manual smoke test: subscribe in Chrome, trigger an event, notification appears in the browser

---

### Task 1.2 — Fix Mutex Deadlock [Micro, 1h]

**Load these files:**
- `server/services/push_service.go` — lines 135–165 (Subscribe + Unsubscribe methods)

**What to do:**
1. Line 143: change `defer ps.mu.RUnlock()` → `defer ps.mu.Unlock()`
2. Audit `Unsubscribe()` and `GetSubscriptions()` — verify they use matching RLock/RUnlock or Lock/Unlock
3. Add test: two goroutines call `Subscribe()` concurrently; assert no panic and both subscriptions stored

**Done when:**
- `go test ./server/services/... -race` passes with no data race or deadlock reports

---

### Task 1.3 — Handle 410/404 — Delete Stale Subscriptions [Small, 2h]

**Load these files:**
- `server/services/push_service.go` — `sendToSubscription` or `SendNotification` method

**What to do:**
1. Capture `resp, err` from `webpush.SendNotification` (currently `_, err = ...`)
2. Inspect `resp.StatusCode`:
   - 201/202 → close body, continue
   - 410 or 404 → call `ps.Unsubscribe(sub.Endpoint)`; close body; log
   - 413 → log "payload too large"; close body
   - 429 → log "rate limited"; close body
3. Always `defer resp.Body.Close()` — fix the connection leak
4. Wrap each `SendNotification` call with a `context.WithTimeout(ctx, 10*time.Second)`

**Done when:**
- Unit test: mock HTTP server returns 410 → subscription removed from storage
- Unit test: mock HTTP server returns 201 → subscription retained
- `go test ./server/services/... -race` passes

---

## Story 2: Notifier Interface Refactor + P1 Bug Fixes

**Status: TODO — after Story 1**

Prereqs: Story 1 complete
Acceptance: Notifier interface wired; URGENT + APPROVAL_NEEDED covered; deep-link uses session.ID; unit test table passes

---

### Task 2.1 — Define Notifier Interface + WebPushNotifier [Small, 2h]

**Load these files:**
- `server/notifications/subscriber.go` — read the Appender interface as the pattern to mirror
- `server/push/subscriber.go` — current function signature
- `server/services/push_service.go` — SendNotification method signature

**What to do:**
1. Create `server/push/notifier.go`:
   ```go
   type DeliveryNotification struct {
       Title, Body, Icon, Tag string
       Renotify           bool
       RequireInteraction bool
       Data               map[string]interface{}
       Actions            []DeliveryAction
       TTL                int
   }
   type DeliveryAction struct { Action, Title, Icon string }
   type Notifier interface {
       Send(ctx context.Context, n DeliveryNotification) error
       Name() string
   }
   type WebPushNotifier struct { svc *services.PushService }
   ```
2. Implement `WebPushNotifier.Send`: convert `DeliveryNotification` → `services.PushNotification`, call `svc.SendNotification`
3. Rename `StartPushSubscriber` → `StartDeliverySubscriber`; change parameter from `*services.PushService` to `[]Notifier`
4. Replace direct `pushService.SendNotification` call with:
   ```go
   for _, n := range notifiers {
       if err := n.Send(ctx, notification); err != nil {
           log.Printf("[push] %s delivery error: %v", n.Name(), err)
       }
   }
   ```
5. Update the call site in `server/server.go` from Task 1.1

**Done when:**
- `make build` passes
- `go test ./server/push/...` with a `mockNotifier` that records calls
- A stub `FCMNotifier` implementing `Notifier` compiles (proves extensibility)

---

### Task 2.2 — Fix Trigger Rules — Proto Constants + URGENT + APPROVAL_NEEDED [Small, 2h]

**Load these files:**
- `server/push/subscriber.go` — shouldNotify logic (~lines 60–110)
- `proto/session/v1/types.proto` — NotificationPriority and NotificationType enum values

**What to do:**
1. Create `server/push/trigger_constants.go`:
   ```go
   const (
       priorityHigh   = int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)   // 3
       priorityUrgent = int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT) // 4
       typeApproval   = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED) // 1
   )
   ```
2. In `EventNotification` case, replace `int32(3)` condition:
   ```go
   p := event.NotificationPriority
   if p >= priorityHigh || event.NotificationType == typeApproval {
       shouldNotify = true
   }
   ```
3. Remove the wrong comment `// Only works for Chrome/Firefox push (not Safari)` from push_service.go
4. Write `server/push/subscriber_test.go` test table:
   ```
   | EventType              | Priority | NotificationType | shouldNotify |
   | EventNotification      | LOW      | GENERIC          | false        |
   | EventNotification      | HIGH     | GENERIC          | true         |
   | EventNotification      | URGENT   | GENERIC          | true         |
   | EventNotification      | LOW      | APPROVAL_NEEDED  | true         |
   | EventSessionStopped    | any      | any              | true         |
   ```

**Done when:**
- All test table cases pass
- `go test ./server/push/... -v` shows labelled test cases

---

### Task 2.3 — Fix Deep-Link URL + Tag Suffix — Use session.ID [Micro, 1h]

**Load these files:**
- `server/push/subscriber.go` — URL and tag construction

**What to do:**
1. Add `"net/url"` to imports if missing
2. Find all three locations that use `event.Session.Title` in URL or tag fields:
   - `"url": "/?session=" + event.Session.Title + ...` → `"/?session=" + url.QueryEscape(event.Session.ID) + ...`
   - `tag` suffix using `.Title` → use `.ID` directly (no encode needed for the tag string)
3. Also update `notificationTag` variable if it uses `.Title` as suffix

**Done when:**
- `make build` passes
- Code review confirms no remaining `event.Session.Title` in URL/tag positions

---

## Story 3: Push Settings UI + Permission Lifecycle

**Status: TODO — can start after Task 1.1 (independent of Story 2)**

Prereqs: Task 1.1 (push is wired); understanding of usePushNotifications hook API
Acceptance: settings toggle works; permission revocation detected; prefs persist in config

---

### Task 3.1 — Add PermissionStatus.onchange Listener [Micro, 1h]

**Load these files:**
- `web-app/src/hooks/usePushNotifications.ts` — full file

**What to do:**
1. In the hook's `useEffect`, after checking initial permission, add:
   ```ts
   if (navigator.permissions) {
     const status = await navigator.permissions.query({ name: 'notifications' as PermissionName });
     status.onchange = () => setPermission(status.state);
     return () => { status.onchange = null; };
   }
   ```
2. The cleanup function must null the onchange handler to prevent memory leaks
3. Confirm the existing `permission` state variable is updated by this listener

**Done when:**
- TypeScript compiles
- Manual: revoke Chrome notifications permission while app open → UI updates without reload

---

### Task 3.2 — Add NotificationPrefs to Config + Atomic saveConfig [Small, 2h]

**Load these files:**
- `config/config.go` — Config struct, LoadConfig, saveConfig
- `server/notifications/store.go` — find the atomic write pattern to copy

**What to do:**
1. Add `NotificationPrefs struct { PushEnabled bool \`json:"pushEnabled"\` }` to config.go
2. Add `Notifications NotificationPrefs \`json:"notifications,omitempty"\`` to `Config`
3. In `LoadConfig`, after unmarshal, add nil-safe init (no-op since struct has zero values, but add comment for consistency)
4. Bump `ConfigVersion` constant to 2; add a log line in the migration switch for v1→v2
5. Refactor `saveConfig` (or its equivalent):
   ```go
   tmp := configPath + ".tmp"
   if err := os.WriteFile(tmp, data, 0600); err != nil { return err }
   return os.Rename(tmp, configPath)
   ```
6. Write test: marshal a `Config` with `Notifications.PushEnabled = true`, unmarshal, assert value preserved

**Done when:**
- `go test ./config/... -race` passes
- Round-trip test passes for `NotificationPrefs`

---

### Task 3.3 — Build PushNotificationSettings Component [Medium, 3h]

**Load these files:**
- `web-app/src/hooks/usePushNotifications.ts` — hook exports
- `web-app/src/app/globals.css` — available CSS tokens
- `web-app/.claude/rules/css-architecture.md` — vanilla-extract rules

**What to do:**
1. Create `web-app/src/components/settings/PushNotificationSettings.tsx`:
   ```tsx
   const { subscribe, unsubscribe, isSubscribed, permission, isSupported } = usePushNotifications();

   // State: not supported
   if (!isSupported) return <p>Push notifications not supported in this browser.</p>;

   // State: permission denied
   if (permission === 'denied') return (
     <div>
       <p>Notifications blocked. To re-enable, open browser settings.</p>
       <p>Chrome: Settings → Privacy → Notifications</p>
     </div>
   );

   // State: can subscribe/unsubscribe
   return (
     <label>
       <input type="checkbox" checked={isSubscribed} onChange={isSubscribed ? unsubscribe : subscribe} />
       Push notifications {isSubscribed ? 'enabled' : 'disabled'}
     </label>
   );
   ```
2. Create `PushNotificationSettings.css.ts` with vanilla-extract styles using `vars` from theme
3. Handle loading state if `subscribe`/`unsubscribe` are async (disable toggle while pending)
4. iOS hint: if `navigator.userAgent` includes "iPhone" and `!isSupported`, show "Add to Home Screen to enable push on iOS"

**Done when:**
- TypeScript compiles with no errors
- All three render states are covered (not supported, denied, toggle)
- Manual: toggle enables push; toggle again disables; page reload shows persisted state

---

### Task 3.4 — Wire PushNotificationSettings into Settings Panel [Small, 2h]

**Load these files:**
- The existing settings panel (search for `Settings` component in `web-app/src/components/`)
- `web-app/src/components/settings/PushNotificationSettings.tsx` from Task 3.3

**What to do:**
1. Find the settings panel component
2. Import `PushNotificationSettings`
3. Add a "Notifications" section with `<PushNotificationSettings />` inside
4. Confirm the component renders in the settings panel in the browser

**Done when:**
- Push settings section visible in the settings panel
- No layout regressions in the settings panel

---

## Story 4: Rich Payload Enrichment + Service Worker Split

**Status: TODO — after Story 2**

Prereqs: Task 2.1 (Notifier interface; DeliveryNotification struct exists); Task 2.2 (trigger rules)
Acceptance: per-type actions/renotify in payload; SW reads dynamic actions; SW split complete

---

### Task 4.1 — Enrich Push Payload per Notification Type [Small, 2h]

**Load these files:**
- `server/push/subscriber.go` — payload construction
- `server/push/notifier.go` — DeliveryNotification struct (from Task 2.1)
- `research/findings-features.md` — Q2 per-type action table

**What to do:**
1. Add `Actions []DeliveryAction` and `TTL int` to `DeliveryNotification` if not already present
2. In subscriber, construct per-type notifications:

   | Event | requireInteraction | renotify | TTL | actions |
   |---|---|---|---|---|
   | APPROVAL_NEEDED | true | true | 7200 (2h) | [{action:"open",title:"Review"},{action:"dismiss",title:"Later"}] |
   | session-stopped | false | false | 86400 (24h) | [{action:"open",title:"View"},{action:"dismiss",title:"Dismiss"}] |
   | ERROR | true | false | 86400 | [{action:"open",title:"View Error"},{action:"dismiss",title:"Dismiss"}] |

3. Add `notificationType` and `timestamp` (Unix seconds) to `data` map
4. Add payload size guard: if `len(json) > 3800`, truncate `Body` to fit

**Done when:**
- `go test ./server/push/...` test table verifies per-type fields
- No payload exceeds 3900 bytes

---

### Task 4.2 — Update Service Worker for Dynamic Actions + Click Dispatch [Medium, 3h]

**Load these files:**
- `web-app/public/push-sw.js` — full file

**What to do:**
1. In `push` event handler, read full JSON payload and use server-provided actions:
   ```js
   const payload = event.data ? event.data.json() : {};
   const defaultActions = [{ action: 'open', title: 'Open' }];
   event.waitUntil(
     self.registration.showNotification(payload.title || 'Stapler Squad', {
       body: payload.body,
       icon: payload.icon || '/icon-192.png',
       tag: payload.tag,
       renotify: payload.renotify ?? false,
       requireInteraction: payload.requireInteraction ?? false,
       actions: payload.actions || defaultActions,
       data: payload.data || {},
     })
   );
   ```
2. In `notificationclick` handler:
   ```js
   event.notification.close();
   const url = event.notification.data?.url || '/';
   if (event.action === 'dismiss') return;
   event.waitUntil(
     clients.matchAll({ type: 'window' }).then(cs => {
       const existing = cs.find(c => c.url.includes(url) && 'focus' in c);
       if (existing) return existing.focus();
       return clients.openWindow(url);
     })
   );
   ```

**Done when:**
- Manual Chrome desktop: approval_needed push shows "Review" + "Later" buttons
- Clicking "Review" opens app at the correct session URL
- Clicking "Dismiss" closes notification without opening app
- Safari: body click opens app at `data.url` (no actions rendered, fallback works)

---

### Task 4.3 — Split Service Worker: Separate Push from PWA Caching [Large, 4h]

**Load these files:**
- `web-app/public/push-sw.js` — full file (identify caching vs. push sections)
- `next.config.js` and `web-app/src/app/layout.tsx` (or `_app`) — find SW registration code

**What to do:**
1. Create `web-app/public/cache-sw.js`:
   - Move `install` handler (asset caching), `activate` handler (cache cleanup), `fetch` handler (cache-first serve)
   - Keep its own `CACHE_NAME` constant with a version suffix
2. Trim `web-app/public/push-sw.js` to contain only:
   - `push` event handler (from Task 4.2)
   - `notificationclick` event handler
   - `install: self.skipWaiting()`; `activate: self.clients.claim()`
3. Update SW registration code to register `cache-sw.js` for caching and keep push SW registration for push
4. Verify VAPID subscription endpoint is tied to the push SW scope (check `navigator.serviceWorker.register('/push-sw.js', { scope: '/' })`)

**Done when:**
- Browser DevTools → Application → Service Workers shows the push SW (for push events)
- Asset caching still works after a `cache-sw.js` change + reload
- Push notifications still received after a `push-sw.js` change + reload
- `make build` passes; no lint errors

---

## Context Preparation Guide

### Loading context for Story 1
```
Read: server/server.go (service construction patterns)
Read: server/push/subscriber.go (StartPushSubscriber signature)
Read: server/services/push_service.go (constructor + Subscribe + SendNotification)
Concepts: EventBus pub/sub; VAPID subscription lifecycle; sync.RWMutex
```

### Loading context for Story 2
```
Read: server/push/subscriber.go (full event handler)
Read: server/notifications/subscriber.go (Appender interface pattern)
Read: proto/session/v1/types.proto (NotificationPriority + NotificationType enums)
Read: project_plans/web-push-notifications/decisions/ADR-001-notifier-interface.md
Concepts: Go interface injection; proto enum int32 casting
```

### Loading context for Story 3
```
Read: web-app/src/hooks/usePushNotifications.ts (hook API)
Read: config/config.go (Config struct + version migration)
Read: server/notifications/store.go (atomic write pattern)
Find: settings panel component location
Concepts: Permissions API; vanilla-extract CSS; React hook patterns
```

### Loading context for Story 4
```
Read: web-app/public/push-sw.js (full)
Read: server/push/notifier.go (DeliveryNotification struct from Story 2)
Read: research/findings-features.md Q2 (per-type action table)
Find: SW registration in next.config.js or layout
Concepts: Service Worker events; Web Push payload format; ServiceWorkerClients API
```

---

## Success Criteria

- [ ] All atomic tasks completed and validated
- [ ] `make build` and `make test` pass on final state
- [ ] End-to-end smoke test: Chrome receives push for approval_needed, task-complete, error events
- [ ] Deep-link navigates to correct session
- [ ] Settings panel shows push toggle; state persists across restarts
- [ ] Safari 16+ macOS receives push (VAPID — no backend changes needed)
- [ ] Race detector clean: `go test ./... -race`
- [ ] No connection leaks: all resp.Body.Close() calls in place
- [ ] SW split: push and caching concerns independently versioned
- [ ] Unit test table covers all trigger rules

---

## Deferred (Post-MVP)

- ANSI-stripped terminal output `snippet` field in push payload (nice-to-have; payload budget adequate)
- Per-type TTL fine-tuning (approval 2h vs. completion 24h — wired in Task 4.1 already)
- `PermissionStatus.onchange` Safari compatibility check (Safari may need polling fallback)
- iOS home screen installation hint in settings UI (pending iOS 17/18 home screen requirement status)
- FCMNotifier implementation (add when React Native app is in progress; interface is ready)
- webpush-go PR #60 (crypto/ecdh migration) — monitor; apply when merged
