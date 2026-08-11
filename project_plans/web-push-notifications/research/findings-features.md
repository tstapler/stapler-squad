# Findings: Features - Rich Notification Payloads + UX Patterns

**Researched:** 2026-04-17
**Sources:** MDN, Can I Use (March 2026 data), codebase audit of `server/push/subscriber.go`,
`web-app/public/push-sw.js`, `proto/session/v1/types.proto`, `proto/session/v1/session.proto`

---

## Summary

Five questions were investigated. Key answers:

1. **Payload size**: The Web Push spec mandates a minimum of 4096 bytes (4 KB) for push service support.
   Practical plaintext budget after AEAD encryption overhead is ~3900 bytes — ample for JSON metadata
   but not for raw terminal output snippets beyond ~200 characters.

2. **Actions browser support**: `Notification.actions` (the buttons in `showNotification`) are supported
   by Chrome 53+, Edge 18+, Opera 39+, and — as of Firefox 152 — Firefox. Safari and iOS Safari do
   **not** support actions as of April 2026. Global coverage is approximately 76%. Design must degrade
   gracefully: the click target on the notification body must always work even without action buttons.

3. **Session deep-link URL scheme**: The proto type comment reads "Unique identifier (uses title as ID
   for now)." The current push subscriber already uses `session.Title` as both the `sessionId` field
   in the notification data and the URL parameter. This is fragile — titles change on rename.
   `session.id` and `session.title` are the same value today, so no immediate breakage, but the URL
   scheme should be documented as title-based and a TODO raised to stabilise it behind a true opaque ID.

4. **Permission prompt UX**: Browsers suppress permission prompts called outside a user gesture on
   most platforms. Deferred, user-triggered permission (settings toggle or first-notification banner)
   is the correct pattern. Calling `requestPermission()` on page load silently fails or auto-denies
   on modern Chrome/Firefox. Once denied, the permission cannot be re-requested via the API.

5. **Tag and renotify**: Using `tag` to collapse per-session notifications is correct. `renotify: true`
   is appropriate only for approval-needed events where the user must act; it is undesirable for
   session-complete where collapsing is preferred.

---

## Options Surveyed

### Q1: Rich payload fields for a developer-tool notification

**Web Push payload budget**

The RFC 8030 / draft-ietf-webpush-protocol-10 spec requires push services to accept payloads of at
least 4096 bytes. The actual usable plaintext budget after AEAD encryption overhead (a nonce, auth
tag, and two bytes of padding header) is approximately 3900 bytes of JSON. Firefox Autopush and FCM
both honour larger payloads in practice (up to ~4 KB without issue, some services allow more), but
4096 bytes is the only guaranteed minimum.

**Fields that belong in a developer-tool push notification**

The current `PushNotification` struct in `server/services/push_service.go` contains:
`title`, `body`, `icon`, `tag`, `data`, `requireInteraction`.

Missing fields with clear value for developer tooling:

| Field | Purpose | Already in struct? |
|-------|---------|-------------------|
| `sessionId` | Stable identifier for deep-link resolution | In `data` map only |
| `sessionTitle` | Human-readable label shown in body | In `data` map only |
| `notificationType` | Enum string enabling SW to render type-specific icons/actions | No |
| `url` | Deep-link URL; SW `notificationclick` uses `data.url` | In `data` map |
| `timestamp` | When the event occurred (for ordering stale notifications) | No |
| `actions` | Type-specific action buttons (see Q2) | No — hardcoded in SW |
| `renotify` | Whether to ring again for same tag | No |

**Terminal output snippet**

Including a body snippet of recent terminal output is attractive but has constraints:
- Push payload budget is ~3900 bytes total; a full JSON envelope with a 200-char snippet fits
  comfortably. A 1000-char snippet risks exceeding the budget once JSON-encoded.
- Terminal output often contains ANSI escape codes that are unreadable as plain text.
- For `approval_needed` the relevant content is the approval prompt text, which is short.
  For `session_complete` the relevant content is the last non-empty line of output, also short.
- Recommendation: include a `snippet` field (max 120 characters) stripped of escape codes,
  populated only for `task_complete` and `error` types. Do not send raw terminal scrollback.

**Suggested extended struct:**

```go
type PushNotification struct {
    Title              string                 `json:"title"`
    Body               string                 `json:"body"`
    Icon               string                 `json:"icon,omitempty"`
    Badge              string                 `json:"badge,omitempty"`
    Tag                string                 `json:"tag,omitempty"`
    Renotify           bool                   `json:"renotify,omitempty"`
    RequireInteraction bool                   `json:"requireInteraction,omitempty"`
    Data               map[string]interface{} `json:"data,omitempty"`
    // Data should always include: sessionId, sessionTitle, notificationType, url, timestamp
    Actions            []PushAction           `json:"actions,omitempty"`
}

type PushAction struct {
    Action string `json:"action"`
    Title  string `json:"title"`
    Icon   string `json:"icon,omitempty"`
}
```

This keeps `actions` in the Go payload so the service worker can read them from `event.data.json()`
rather than hard-coding them in the SW. The SW should fall back to `[Open, Dismiss]` if `actions`
is absent.

---

### Q2: Notification action buttons — browser support and per-type design

**Browser support matrix for `showNotification` actions (as of April 2026)**

| Browser | Support | Notes |
|---------|---------|-------|
| Chrome desktop 53+ | Full | `Notification.maxActions` typically returns 2 |
| Edge 18+ | Full | Same limit as Chrome |
| Firefox 152+ | Full | Very recently added; 151 and earlier had no support |
| Safari macOS | No | Confirmed not supported as of 26.x; TP unknown |
| Safari iOS 16.4+ | No | Partial push support but no action buttons |
| Opera 39+ | Full | Chromium-based |
| Chrome for Android 147+ | Full | |
| Firefox for Android 149 | No | Not yet supported on Android |
| Samsung Internet 6.2+ | Full | |

Global coverage with actions: approximately 76% (Can I Use, March 2026).

The current `push-sw.js` hard-codes `[{ action: 'open', title: 'Open' }, { action: 'dismiss', title: 'Dismiss' }]`
for every notification type. This is a missed opportunity for actionable types.

**Per-type action design**

| Notification type | Recommended actions | `requireInteraction` | `renotify` |
|---|---|---|---|
| `APPROVAL_NEEDED` | `[{ action: 'open', title: 'Review' }, { action: 'dismiss', title: 'Later' }]` | `true` | `true` |
| `TASK_COMPLETE` (session stopped) | `[{ action: 'open', title: 'View' }, { action: 'dismiss', title: 'Dismiss' }]` | `false` | `false` |
| `ERROR` / `FAILURE` | `[{ action: 'open', title: 'View Error' }, { action: 'dismiss', title: 'Dismiss' }]` | `true` | `false` |
| `INPUT_REQUIRED` / `CONFIRMATION_NEEDED` | `[{ action: 'open', title: 'Respond' }, { action: 'dismiss', title: 'Later' }]` | `true` | `true` |

Notes:
- Keep to two actions. `Notification.maxActions` is 2 in Chrome/Edge; a third action is silently
  dropped.
- Safari ignores actions entirely; the notification body click still opens the URL via
  `event.notification.data.url`. Always populate `data.url`.
- `requireInteraction: true` on approval notifications prevents the OS from auto-dismissing
  after a few seconds — correct for blocking events.
- An "Approve" action that calls a ConnectRPC endpoint from the service worker is technically
  possible but adds significant complexity (auth tokens, SW fetch). Do not implement in this phase;
  reserve for a future enhancement after core push is solid.

---

### Q3: Session deep-link URL scheme

**Current state**

`server/push/subscriber.go` constructs URLs as:
```go
"url": "/?session=" + event.Session.Title + "&tab=terminal"
```

`web-app/src/app/page.tsx` reads `searchParams.get("session")` and passes it to `findSessionById()`,
which tries title match, ID match, tmux prefix-stripped match, and path-based match.

`proto/session/v1/types.proto` line 10: `// Unique identifier (uses title as ID for now).`

**Problem**

Session titles are user-editable (via the `Rename` API). If a notification deep-link is constructed
at the time of the event and the user later renames the session, the stale title in the URL will
fail to resolve. This is not a hypothetical: OS notification centres can hold notifications for
hours before a user clicks them.

**Option A: Keep `/?session=<title>` (current)**
- Pros: Already working; no code change.
- Cons: Breaks on rename; titles may contain URL-unsafe characters (spaces etc.) that need encoding.

**Option B: Use `/?session=<id>` where `id` is the same as `title` today**
- Pros: The query parameter name is forward-compatible with a future opaque ID.
- Cons: No immediate benefit because `id == title` in the current proto.
- Verdict: Adopt this naming now. `findSessionById` already handles both; the subscriber should
  send `event.Session.ID` (not `.Title`) so the URL is semantically correct even though the values
  happen to be identical today.

**Option C: Full URL path scheme `/session/<id>?tab=terminal`**
- Pros: Cleaner REST-style URLs; standard practice for web apps.
- Cons: Requires Next.js routing changes; out of scope for this iteration.

**Option D: `/?session=<id>&title=<title>` dual parameter**
- Pros: The app can display the title optimistically while session list loads.
- Cons: Adds complexity, redundancy, and stale-title-in-URL risk for the title parameter.

**Recommendation: Option B** — switch the subscriber to use `session.ID` in the URL, encode the
value with `url.QueryEscape`. This costs one-line of Go change, is forward-compatible with a future
opaque UUID session ID, and keeps the frontend code unchanged.

---

### Q4: Permission prompt UX

**Browser permission model constraints**

- `Notification.requestPermission()` must be called from within or after a user gesture (click,
  keypress) on Chrome 71+ and Firefox 72+. Calls outside a gesture are silently treated as
  "denied" or the prompt is suppressed.
- If the user clicks "Block" (denied), `Notification.permission` is permanently `"denied"` until
  the user manually resets it in browser settings. The API cannot re-prompt.
- Safari 16.4+ on iOS requires the site to be installed as a PWA (add to home screen) before push
  subscriptions are allowed at all.
- macOS Safari 16+ follows the standard Web Push flow with VAPID; no special handling needed beyond
  HTTPS + service worker.

**Permission prompt timing options**

| Option | Description | Risk |
|--------|-------------|------|
| A: On page load | `requestPermission()` called during app init | Browsers suppress/auto-deny; UX anti-pattern; high denial rate |
| B: Deferred — on first relevant event | Prompt when the first approval-needed event fires in-app | User may be focused elsewhere; event occurs once and is gone |
| C: User-triggered from settings | Settings panel has "Enable push notifications" toggle that calls `requestPermission()` | Correct UX; user understands why permission is needed |
| D: Soft prompt first | Show a custom in-app banner ("Enable notifications to be alerted when sessions need approval") with a CTA button; only then call native API | Best opt-in rate; user is educated before the browser dialog appears |

**Recommendation: Option D** (soft-prompt banner + settings toggle). The `NotificationPanel`
already exists; a small banner component inside the notification panel (or header) provides context.
The subscribe CTA calls `usePushNotifications.requestPermission()` followed by `subscribe()`.

**Handling "denied"**

When `Notification.permission === "denied"`:
- The settings toggle must show a disabled state with explanatory text: "Notifications blocked.
  Open browser settings to re-enable."
- Provide a link to browser-specific instructions (Chrome: `chrome://settings/content/notifications`,
  Firefox: `about:preferences#privacy`). These cannot be deep-linked from a web page; plain
  instructional text is sufficient.
- Do not repeatedly prompt or show error toasts on every page load.

**Subscribe/unsubscribe toggle UI patterns**

The `usePushNotifications` hook already exposes `subscribe`, `unsubscribe`, `permission`, and
`isSupported`. The settings UI needs only:

```
[ ] Enable push notifications          ← checkbox or toggle
Current status: Enabled / Disabled / Blocked
```

When `isSupported === false` (Firefox < 44, older Safari): hide or grey out the toggle with
"Push notifications are not supported in this browser."

When subscribed: show the unsubscribe path (toggle off) plus the `subscriptionId` (truncated) for
debugging.

---

### Q5: Tag and renotify

**Using `tag` to collapse duplicate notifications per session**

The current code in `server/push/subscriber.go` sets tags as:
- `"session-completed-" + event.Session.Title`
- `"approval-required-" + event.Session.Title`
- `"notification-" + event.NotificationID`

This is architecturally correct. A `tag` causes the browser/OS to replace an existing notification
with the same tag rather than stacking a new one. For a session that fires many rapid-fire
`approval_needed` events (unusual but possible), the notification centre will show one entry, not
many.

**When to use `renotify: true`**

`renotify: true` causes the notification to ring/vibrate again even when replacing a notification
with the same tag. Without it, the replacement is silent.

| Event type | Use `renotify` | Rationale |
|---|---|---|
| `approval_needed` | Yes | A new approval request from the same session IS a new interrupt — user must act |
| `task_complete` | No | Replacing a stale "session finished" with a fresh one is a no-op from the user's perspective |
| `error` | No | Error already shown; replacement is an update, not a new alert |
| `input_required` | Yes | Same as `approval_needed` — blocking action required |

**Tag stability note**

Current tags use `event.Session.Title` as the suffix. This has the same rename-fragility described
in Q3. If the session is renamed mid-run, a new notification will not replace the old one because
the tags differ. Low-risk in practice (renames during active sessions are rare) but should be noted.
Switching to `event.Session.ID` (same value today) as the tag suffix aligns with the Q3 fix.

**`renotify` browser support**

`renotify` is in the spec and supported by Chrome/Edge/Opera. Firefox added support in Firefox 152
alongside actions. Safari does not support it. This is a progressive enhancement: it simply won't
ring again on Safari, which is acceptable.

---

## Trade-off Matrix

| Pattern | User experience | Implementation complexity | Browser support | Relevance to dev tool |
|---|---|---|---|---|
| Rich payload (sessionId + type + snippet in data) | User sees context without opening app | Low — Go struct change + SW reads `event.data` | Payload data field: ~95% | High — developer needs "which session, what happened" at a glance |
| Type-specific action buttons | One-tap "Review" for approvals vs "View" for completions | Medium — actions added to Go payload, SW dispatches per `event.action` | 76% (no Safari/iOS) | High — approval flow benefits most; degrade gracefully to body click |
| ID-based deep-link `/?session=<id>` | Correct navigation even after rename | Trivial — one-line change in subscriber | N/A (server-side) | High — stale links are a silent failure mode |
| Deferred permission (settings toggle) | No jarring browser dialog on load | Low — hook already exists; needs settings UI | Standard across all | High — single-user tool; user controls when to opt in |
| Soft-prompt banner before native dialog | Higher opt-in rate; user understands purpose | Low-Medium — one React component | N/A | Medium — single-user, will likely enable anyway, but good practice |
| `requireInteraction` on approvals | Notification stays until user dismisses | None — field exists | Chrome/Edge/Firefox 72+; Safari partial | High — approval timeout is expensive if missed |
| `renotify: true` on approvals | Re-rings for each new approval on same session | None — field in payload | Chrome/Edge/Firefox 152+; no Safari | High — critical interrupt, must not be silent |
| Terminal output snippet (120 char, ANSI-stripped) | Context without opening app | Low-Medium — Go strips ANSI codes, populates field | Payload data field: ~95% | Medium — useful for errors; marginal for approvals |

---

## Risk and Failure Modes

**R1: Payload size overflow**
If `body` or `snippet` fields are populated with long strings (e.g., multi-line error messages,
long session titles with Unicode), the JSON payload may exceed 4096 bytes. The push service returns
HTTP 413. Mitigation: enforce maximum lengths in `SendNotification()` — `Body` ≤ 200 bytes,
`snippet` ≤ 120 bytes, titles truncated to 60 chars. Check `len(body_json) <= 3800` before sending.

**R2: Safari ignores actions (silently)**
Safari drops the `actions` array. If the SW relies on `event.action === 'open'` to detect the
body-click vs button-click, Safari always triggers the `else` (default open) branch, which is
correct. Risk: low. Existing `notificationclick` handler already handles missing action by falling
back to `data.url`.

**R3: Permission denied — no recovery path**
A user who clicks "Block" loses push until they manually open browser settings. The settings UI
must distinguish `denied` from `default` and not show a subscribe button that calls the API (it
will throw). Mitigation: check `Notification.permission === "denied"` before rendering the CTA.

**R4: Service worker caching conflict**
`push-sw.js` doubles as a PWA caching service worker (it has `install`, `activate`, `fetch`
listeners alongside `push`). A push SW update cycle (new version deployed) can cause a brief
period where the old SW handles push events while the new version is `installing`. If the old SW
lacks a new action type handler, notifications fall back to the default open action. Low risk but
noted. Requirements doc flags "service worker consolidation" as a must-have — separating caching
and push concerns removes this coupling.

**R5: Tag rename fragility (see Q5)**
Tags use session title. Rename during an active session means a second `approval_needed`
notification stacks rather than replaces. Very low probability; fix is trivially part of the Q3
subscriber change.

**R6: Mutex bug in `PushService.Subscribe()`**
`server/services/push_service.go` line 143 calls `defer ps.mu.RUnlock()` inside a write-locked
`Lock()` context. This is a deadlock under concurrent subscribe calls. The requirements doc
identifies this. It must be fixed before any production use.

---

## Migration and Adoption Cost

**Backend changes (Go)**

| Change | File | Effort |
|---|---|---|
| Add `Actions`, `Renotify`, `Badge` fields to `PushNotification` struct | `push_service.go` | 5 min |
| Switch tag suffix and URL param to `session.ID` | `push/subscriber.go` | 5 min |
| Add per-type action construction + `renotify` + `requireInteraction` logic | `push/subscriber.go` | 30 min |
| Add `notificationType`, `timestamp` to `data` map | `push/subscriber.go` | 10 min |
| Add ANSI-strip + truncation for optional `snippet` field | new utility function | 30 min |
| Fix mutex bug (`RUnlock` → `Unlock`) | `push_service.go` line 143 | 1 min |
| Add payload size guard (`len(json) <= 3800`) | `push_service.go` | 10 min |

**Frontend changes (TypeScript/JS)**

| Change | File | Effort |
|---|---|---|
| SW reads `event.data.json().actions` and uses them in `showNotification` | `push-sw.js` | 20 min |
| SW dispatches per `event.action` in `notificationclick` (currently only `dismiss` handled) | `push-sw.js` | 15 min |
| Settings panel toggle: subscribe/unsubscribe + permission state display | new component | 2-3 hrs |
| Soft-prompt banner in notification panel header | small component | 1 hr |

**No breaking changes**: existing subscriptions continue to work; the payload is additive JSON. The
SW change is backward-compatible — `actions` absent means fall back to default.

---

## Operational Concerns

- **VAPID key rotation**: if VAPID keys are rotated, all existing `PushSubscription` records become
  invalid. Push service returns 401 or 410. The subscriber must handle 410 by deleting the
  subscription from the store. Currently `sendToSubscription` ignores the HTTP status from
  `webpush.SendNotification`. A 410 handler should be added.
- **Subscription expiry**: push subscriptions can expire or be invalidated by the push service
  without the server knowing. Implement periodic re-subscription prompts (every 30 days) or
  detect 404/410 from the push service and prompt re-subscribe. [TRAINING_ONLY - verify exact
  expiry behaviour of Firefox Autopush and Chrome FCM endpoints]
- **Single subscriber**: this is a single-user tool. There is no multi-tenant or fan-out concern.
  The `SendNotification` broadcast loop over all subscriptions is adequate.
- **TTL**: current code uses `TTL: 60 * 60 * 24` (24 hours). For `approval_needed`, a 24-hour TTL
  means a notification can arrive a day late when the device was offline — the approval is long
  expired. Consider `TTL: 60 * 60 * 2` (2 hours) for approval events, `TTL: 60 * 60 * 24` for
  completions.
- **iOS Web Push on PWA**: iOS Safari 16.4+ supports Web Push only when the site is added to the
  home screen as a PWA. The `manifest.json` and `push-sw.js` registration must be correct for this
  path to work. If the user accesses via Safari without installing as PWA, `isSupported` will be
  `false` and the settings toggle must explain this.

---

## Prior Art and Lessons Learned

**GitHub (web)**: Uses `requireInteraction: false` for all notifications; relies on notification
centre for persistence. Notification body contains repo name and event type. Actions are "View" only.
Lesson: developer tools don't need "approve from notification" in v1; "View" is sufficient.

**Linear (web)**: Uses aggressive deduplication via `tag`. Sends one notification per issue, not
per event on that issue. Body contains the issue title and a short excerpt. No action buttons.
Lesson: a single clear notification per entity is better than a stream of per-event notifications.

**VS Code (via OS notifications)**: Sends notifications with a single action ("Open") plus body
text showing file path and event. No custom icons per notification type.
Lesson: for a dev tool where the user knows the context, minimal text beats verbose descriptions.

**Progressive Web App patterns (Google web.dev)**: Recommends showing the permission prompt only
after explaining the benefit, using an in-app opt-in screen before the browser dialog. Documents
that opt-in rates are 2-3x higher with context than with bare `requestPermission()` on load.

**MDN "Using the Notifications API"**: Recommends checking `Notification.permission !== "denied"`
before calling `requestPermission()`. This matches the settings toggle pattern above.

---

## Open Questions

1. **Should `snippet` content be stripped server-side (Go) or client-side (SW)?** Server-side is
   simpler and keeps the SW small. The ANSI stripping utility is ~20 lines of Go.

2. **What is the correct TTL for each notification type?** 24 hours for approvals is probably wrong
   (approval timeouts are typically minutes). Needs a decision in the planning phase.

3. **Should actions in the Go payload drive the SW, or should the SW hard-code them by type?**
   Driving from Go payload allows future server-side A/B testing and keeps the SW minimal.
   Recommended: Go payload drives, SW falls back to defaults if `actions` absent.

4. **When will `session.id` diverge from `session.title`?** The proto comment says "uses title as
   ID for now." If/when a true UUID is introduced, the deep-link URL scheme in Q3 is already
   forward-compatible. No action needed now beyond noting the TODO.

5. **Safari Web Push + PWA install requirement on iOS**: is the app being shipped as an installable
   PWA with `manifest.json`? If not, iOS push is unavailable regardless. This should be confirmed
   in the stack research findings.

6. **Should `approval_needed` notifications bypass the 500ms coalescing window in the notification
   history subscriber?** Currently both `notifications.subscriber.go` and `push/subscriber.go` are
   independent. The push subscriber does its own 2-second deduplication window. Confirm these do
   not interact to suppress urgent approvals.

---

## Recommendation

**Immediate (this feature branch):**

1. Fix the mutex bug in `PushService.Subscribe()` (1-line, must-fix before any production use).

2. Extend `PushNotification` struct with `Actions []PushAction`, `Renotify bool`, `Badge string`.
   Populate `data` map with `notificationType`, `sessionId` (using `session.ID`), `timestamp`.

3. In `push/subscriber.go`, switch URL and tag suffix to `url.QueryEscape(event.Session.ID)`.
   Add per-type `requireInteraction` and `renotify` values. Add type-specific actions per the
   table in Q2.

4. Update `push-sw.js` to read `event.data.json().actions` and pass to `showNotification` rather
   than using the hard-coded `[Open, Dismiss]` fallback. Handle `event.action` dispatch for any
   action beyond `dismiss`.

5. Add a settings panel toggle wired to `usePushNotifications` hook (soft-prompt banner +
   subscribe/unsubscribe). Gate the subscribe CTA behind `Notification.permission !== "denied"`.

**Deferred:**
- ANSI-stripped `snippet` field (nice-to-have; low payload budget pressure).
- 410 response handling for expired subscriptions.
- Per-type TTL tuning.
- Full SW/caching separation (flagged in requirements as must-have but independent of payload work).
