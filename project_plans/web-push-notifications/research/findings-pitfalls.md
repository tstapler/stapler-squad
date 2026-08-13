# Findings: Pitfalls — VAPID, Service Worker Lifecycle, Known Bugs

Status: Draft | Research phase | 2026-04-17
Sources: webpush-go GitHub issues/PRs, web.dev push notifications guide,
Apple Developer docs (push.apple.com), actual codebase inspection

---

## Summary

Four confirmed code bugs were found in the existing branch and six operational failure
modes were catalogued. Two bugs (the mutex deadlock and the 410-Gone blindness) are
severity-Critical and must be fixed before any delivery work. The service worker
architecture conflation is medium severity — it causes unnecessary SW update cycles but
does not break push delivery. The magic integer comparison and the deep-link title bug
are medium severity correctness issues. All six failure modes have clear mitigations.

The `push_service.go` comment "Only works for Chrome/Firefox push (not Safari)" is
factually wrong and should be removed. Safari 16+ supports standard VAPID push.

The `webpush-go` library has an open PR (#60, 2023-11) migrating away from the deprecated
`elliptic.Marshal` to `crypto/ecdh`. A `//nolint:staticcheck` suppression is currently
masking this in `push_service.go`. The library is otherwise active.

---

## Failure Modes Catalogued

| ID | Failure Mode | Trigger | Severity | Mitigation |
|----|-------------|---------|----------|------------|
| BUG-1 | Mutex deadlock in `PushService.Subscribe()` | First call to `Subscribe()` | **Critical** | Change `defer ps.mu.RUnlock()` → `defer ps.mu.Unlock()` |
| BUG-2 | Magic int `int32(3)` for HIGH priority | URGENT notifications (value 4) never trigger push | **Medium** | Replace with `int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)` |
| BUG-3 | SW conflates PWA caching + push handler | Any cache manifest change triggers full SW update cycle | **Medium** | Split into `cache-sw.js` and `push-sw.js` with separate versioning |
| BUG-4 | Deep-link uses `event.Session.Title` (not stable ID) | Session rename breaks all historical push deep-links | **Medium** | Use `session.ID` + `url.QueryEscape`; note proto comment: "uses title as ID for now" |
| FM-1 | VAPID key rotation — all subscriptions invalidated | Admin regenerates VAPID keys | **High** | Never rotate without a migration; document key as permanent credential |
| FM-2 | 410 Gone ignored — dead subscriptions accumulate | Browser unsubscribes or push service expires subscription | **High** | Detect 410/404 in `SendNotification`; call `Unsubscribe(endpoint)` |
| FM-3 | SW update race — push arrives during handover | New SW deployed while old SW is waiting | **Low** | Handled by browser event buffering; `skipWaiting` pattern already in place |
| FM-4 | Permission revocation undetected | User revokes notification permission in browser settings | **Medium** | Add `PermissionStatus.onchange` listener in `usePushNotifications.ts` |
| FM-5 | Safari requires HTTPS + user gesture | `requestPermission()` called outside user gesture or on HTTP | **High (Safari only)** | Gate all `requestPermission()` calls on user interaction; require HTTPS |
| FM-6 | `webpush-go` uses deprecated `elliptic.Marshal` | Go 1.21+ builds produce staticcheck warnings; future Go may remove it | **Low–Medium** | Track PR #60; apply patch or fork when crypto/ecdh migration is merged |

---

## Risk and Failure Modes (Detail)

### BUG-1: Mutex deadlock (`push_service.go:142–143`)

```go
// Current (BROKEN):
func (ps *PushService) Subscribe(sub PushSubscription) string {
    ps.mu.Lock()
    defer ps.mu.RUnlock()  // ← BUG: should be Unlock()
    ...
}
```

`sync.RWMutex.RUnlock()` panics with `sync: RUnlock of unlocked RWMutex` when called
without a prior `RLock()`. After `mu.Lock()`, calling `RUnlock()` increments the internal
writer-count decrementor incorrectly, causing all subsequent `Lock()` and `RLock()` calls
to block indefinitely.

**Fix:** Change `defer ps.mu.RUnlock()` to `defer ps.mu.Unlock()`. One character.

**Impact of not fixing:** The server deadlocks permanently on the first push subscription
attempt (typically on first page load in the web UI after enabling push). All further
HTTP requests that touch the mutex will hang.

---

### BUG-2: Magic int comparison (`subscriber.go:79`)

```go
// Current (WRONG):
if event.NotificationPriority == int32(3) { // HIGH priority
```

`NOTIFICATION_PRIORITY_HIGH = 3` is correct, but `NOTIFICATION_PRIORITY_URGENT = 4` is
never matched. The comment says "HIGH" but the intent is "at least HIGH". Any notification
published with URGENT priority will be silently skipped by the push subscriber.

**Fix:**
```go
if event.NotificationPriority >= int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH) {
```

Or better, add explicit type checking for URGENT:
```go
p := event.NotificationPriority
if p == int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH) ||
   p == int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT) {
```

Additionally, `NOTIFICATION_TYPE_APPROVAL_NEEDED` must fire push regardless of priority
(per requirements). The `EventNotification` branch must add an explicit type check:
```go
if event.NotificationType == int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED) {
    shouldNotify = true
}
```

---

### BUG-3: Service worker conflation (`public/push-sw.js`)

The single `push-sw.js` file handles:
1. Static asset caching (install event, fetch interception, `CACHE_NAME = 'stapler-squad-v1'`)
2. Push notification display (push event, notificationclick event)

These two concerns have independent change rates. A change to the cached asset list
triggers a full SW update cycle (deactivate → install → activate) that also re-registers
the push handler, potentially causing push events to be missed during the transition window.

**Fix:** Split into `cache-sw.js` (caching concerns) and `push-sw.js` (push concerns).
Register them as separate service workers at different scopes, or use a single SW that
imports both via `importScripts()`. The simpler approach for a Next.js app is to keep one
SW but version the two concerns independently.

---

### BUG-4: Deep-link uses session title, not stable ID (`subscriber.go:59,73`)

```go
// Current (BROKEN):
"url": "/?session=" + event.Session.Title  // Title can change, spaces, URL-unsafe chars
```

Line 88 (EventNotification case) already uses `event.SessionID` correctly, making the
inconsistency visible in the same file. The proto message confirms:
```proto
message Session {
  string id = 1;  // comment: "uses title as ID for now"
  string title = 2;
```

Since `id` currently equals the title, the breakage only manifests on rename. But the
fix now costs nothing:
```go
"url": "/" + "?session=" + url.QueryEscape(event.Session.Title),
// → Later, when id != title:
"url": "/" + "?session=" + url.QueryEscape(event.Session.ID),
```

Also affects `notificationTag` which uses `.Title` as a suffix — should use `.ID` for
deduplication correctness.

---

### FM-1: VAPID key rotation

VAPID key pairs are a permanent credential. If the private key stored in `vapid-keys.json`
is deleted or the `GenerateVAPIDKeys()` code path runs again, all existing browser push
subscriptions are invalidated. The push service will receive `401 Unauthorized` or
`404 Not Found` errors from push services for all existing subscriptions. There is no
migration path: clients must re-subscribe.

**Mitigation:**
- Document VAPID private key as a permanent credential in the ops guide.
- Back up `vapid-keys.json` alongside other persistent state.
- Never add logic that regenerates keys automatically.
- If rotation is ever required: generate new keys, set a `pendingVAPIDPublicKey` that
  clients detect and use to re-subscribe, then swap the active key once all clients
  have re-subscribed. This is complex and unlikely to be needed for a personal app.

---

### FM-2: 410 Gone — dead subscriptions accumulate

When a browser unsubscribes or a push service expires a subscription, the next push
attempt returns HTTP 410 (Gone) or 404 (Not Found). The current `SendNotification` code
discards the response body (`_, err = ...`) and only logs a warning on error. It never
removes stale subscriptions from `push-subscriptions.json`.

**Result:** Stale subscriptions accumulate over time. `SendNotification` makes N HTTP
requests per push event, one for each subscription (including dead ones), adding latency.

**Fix:**
```go
resp, err := webpush.SendNotification(payload, &sub.Subscription, &opts)
if err == nil {
    if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
        ps.Unsubscribe(sub.Endpoint)
    }
    resp.Body.Close()
}
```

The `webpush-go` library returns `(*http.Response, error)`. The response must be read and
closed to avoid leaking the HTTP connection.

---

### FM-3: Service worker update race

When `push-sw.js` is updated, the new SW enters "waiting" state. The existing code uses:
```js
self.addEventListener('install', (event) => {
    self.skipWaiting();  // Force immediate activation
});
self.addEventListener('activate', (event) => {
    self.clients.claim();  // Take control of open pages
});
```

Push events arriving during the ~200ms window between old SW deactivation and new SW
activation are queued by the browser's push service and redelivered. This is handled
correctly by the browser; no application-level mitigation is needed. Severity is Low.

---

### FM-4: Permission revocation undetected (`usePushNotifications.ts`)

The `usePushNotifications` hook reads `Notification.permission` at mount time but does not
listen for changes. If the user revokes permission in browser settings while the app is
open, the UI continues to show "push enabled" until the next page refresh.

More importantly, the server retains the stale subscription. The next push attempt will
either succeed (Chrome still accepts the push; the browser silently drops it) or fail
with 410 (browser has fully invalidated the subscription).

**Fix:**
```js
const status = await navigator.permissions.query({ name: 'notifications' });
status.onchange = () => {
    if (status.state === 'denied') {
        // Clear UI state and optionally call /api/push/unsubscribe
    }
};
```

---

### FM-5: Safari push requires HTTPS + user gesture

Safari 16+ requires:
1. The page must be served over HTTPS (or localhost for development).
2. `Notification.requestPermission()` must be called within a user gesture handler
   (click, keydown, etc.). Calling it programmatically on page load is blocked.
3. On iOS, the app must be installed to the Home Screen before push can be enabled.

The `push_service.go` comment `// Only works for Chrome/Firefox push (not Safari)` is
**incorrect** and should be removed. Safari 16+ uses standard Web Push via VAPID. The
push endpoint will be a `*.push.apple.com` URL; `webpush-go` handles this transparently.

---

### FM-6: webpush-go deprecated `elliptic.Marshal`

PR #60 (SherClockHolmes/webpush-go, opened November 2023): migrates from the deprecated
`crypto/elliptic.Marshal` to `crypto/ecdh`. As of April 2026, the PR is still open and
unmerged. The current code in `push_service.go` uses `//nolint:staticcheck` to suppress
the deprecation warning.

`crypto/elliptic` functions were deprecated in Go 1.20 but are not removed. They will
remain in the standard library for backward compatibility for the foreseeable future.
This is Low severity today; it becomes Medium if Go removes them in a future version.

**Mitigation:** Monitor PR #60 status. If it merges, upgrade `webpush-go`. If the PR
stalls, apply the patch locally or switch to a fork.

---

## Migration and Adoption Cost

| Bug/FM | Fix effort | Risk of fix |
|--------|-----------|------------|
| BUG-1 mutex | 1 line | Near zero — single character change, no logic change |
| BUG-2 magic int | 5 lines + 1 test | Low — additive change to conditionals |
| BUG-4 deep-link title | 3 lines | Low — additive fix, backward-compatible since id == title today |
| FM-2 410 handling | 10 lines | Low — adds response inspection to existing code |
| FM-4 permission onchange | 10 lines JS | Low — additive listener |
| FM-5 Safari comment | 1 line | None |
| BUG-3 SW split | 1–2 days | Medium — requires testing SW update lifecycle |
| FM-6 webpush-go ecdh | Monitor only | N/A until PR merges |

---

## Operational Concerns

- `push-subscriptions.json` has no file-level lock. Concurrent server restarts could
  corrupt it. The workspace isolation feature mitigates this in practice.
- `webpush-go` uses the process-default HTTP client with no explicit timeout.
  A blocked push endpoint will block the delivery goroutine until Go's default transport
  timeout (which has no hard limit without `context` cancellation). Add a per-delivery
  context with a 10-second deadline.
- The 2-second deduplication window in `subscriber.go` uses an in-memory `lastProcessed`
  map that is never pruned. In a long-running server with many unique notification tags,
  this grows without bound. Add periodic cleanup or use a bounded LRU.

---

## Prior Art and Lessons Learned

**web.dev Push Notifications Guide (Google):** HTTP 410 and 404 both signal "remove this
subscription". 413 signals payload too large (max 4096 bytes). 429 signals rate limit
with `Retry-After` header. All four status codes require distinct handling.

**web.dev Service Worker Lifecycle:** The `skipWaiting` + `clients.claim()` pattern in
the existing SW is correct. Pushes arriving during the install/activate window are
buffered by the push service and redelivered; no application-level handling needed.

**Apple Developer docs (Sending Web Push Notifications):** Safari 16+ (macOS Ventura),
iOS/iPadOS 16.4+ (home screen only), supports full W3C Web Push stack. Endpoint is
`https://*.push.apple.com`. No proprietary API needed.

**webpush-go PR #60:** The open migration PR is a known technical debt item. The lib is
otherwise well-maintained (5 issues, 7 PRs as of April 2026, v1.4.0 released Jan 2025).

---

## Open Questions

- [ ] Does `webpush.SendNotification` have an implicit HTTP timeout, or does it inherit
  the default `http.Client` with no deadline? Determines urgency of adding context-based
  timeout to `SendNotification` calls.
- [ ] Is there a `PermissionStatus.onchange` equivalent for Safari's implementation of
  the Permissions API, or does Safari require polling?
- [ ] When iOS 16.4 push subscription expires (push service returns 410), does the
  browser also fire a `pushsubscriptionchange` event in the service worker, or must the
  server detect 410 and the client re-subscribe on next open?

---

## Recommendation (mitigations priority order)

**P0 — Fix immediately before any push testing:**
1. BUG-1: `defer ps.mu.RUnlock()` → `defer ps.mu.Unlock()` in `push_service.go:143`
2. FM-2: Detect and delete stale subscriptions on HTTP 410/404 in `SendNotification`

**P1 — Fix before marking push as complete:**
3. BUG-2: Replace `int32(3)` with proto enum constant; add APPROVAL_NEEDED type-based
   override in `EventNotification` branch
4. BUG-4: Use `session.ID` (or `url.QueryEscape(session.Title)`) for deep-link URLs
5. FM-4: Add `PermissionStatus.onchange` listener in `usePushNotifications.ts`
6. FM-5: Remove incorrect Safari comment; document HTTPS + user gesture requirements

**P2 — Before shipping settings UI:**
7. BUG-3: Split `push-sw.js` into separate caching and push concerns

**P3 — Monitor, not block:**
8. FM-6: Track `webpush-go` PR #60; apply when merged
