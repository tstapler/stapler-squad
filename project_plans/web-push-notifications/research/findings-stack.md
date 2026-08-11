# Findings: Stack — Safari Web Push + FCM/APNs Forwarding

Status: Draft | Research phase | 2026-04-17
Sources: webkit.org blog, Apple Developer docs, GitHub (SherClockHolmes/webpush-go),
Firebase FCM REST reference, web-push-libs/web-push

---

## Summary

Safari 16+ on macOS and iOS 16.4+ (home screen web apps) fully support the standard W3C Web
Push stack (Push API + Notifications API + Service Workers) using VAPID. No Apple Developer
Program membership is required. The self-hosted VAPID backend already running in
stapler-squad will work with Safari without modification — the only operational gotcha is
that push endpoints for Safari route through `*.push.apple.com`, which must be allowed at
any network/firewall layer.

Self-hosted VAPID cannot directly forward to FCM for React Native Android delivery. FCM uses
a separate registration token model (not VAPID endpoints), a different HTTP API
(`https://fcm.googleapis.com/v1/projects/{projectId}/messages:send`), and requires a
Firebase service-account credential for authentication. The cleanest pattern for
Web Push + FCM/APNs coverage is a **dispatcher model**: the Go backend stores Web Push
subscriptions (current) and will later also store FCM device tokens (one extra field per
subscription record), then dispatches to both delivery paths independently. No hub or
relay is needed; the same notification event triggers two independent outbound calls.

The `SherClockHolmes/webpush-go` library is active as of late 2025 (v1.4.0 released
January 2025, race-condition fix merged November 2025). It is the right choice to keep for
the Web Push delivery path.

---

## Options Surveyed

### Option A — Current: Self-hosted VAPID only (webpush-go)

The existing path. The Go backend generates a VAPID key pair, stores browser push
subscriptions (endpoint + auth + p256dh), and calls `webpush.SendNotification` for each
subscriber.

Browser coverage: Chrome, Firefox, Edge, Opera — full. Safari 16+ macOS and iOS 16.4+
home screen web apps — confirmed supported with standard VAPID since Safari 16.1 / iOS 16.4.

No FCM dependency. No APNs private key needed for browser push. Push endpoints are
browser-vendor-controlled (Google's FCM Web Push relay for Chrome, Mozilla's autopush for
Firefox, Apple's `push.apple.com` for Safari).

### Option B — Add FCM as a parallel dispatch target for React Native Android

FCM v1 HTTP API (`projects.messages:send`). Requires:
- A Firebase project (free tier is sufficient).
- A service-account JSON key for OAuth 2.0 access token generation.
- Per-device FCM registration token (obtained from the RN app on first launch, stored
  server-side against the user/device record).
- A separate dispatch call from the Go backend, independent of the VAPID path.

Payload shape for FCM v1 is a `Message` object with optional platform-specific override
blocks (`android`, `apns`, `webpush`). The top-level `notification` object (title + body)
is cross-platform. The stapler-squad notification payload (`sessionId`, `sessionTitle`,
`notificationType`, `url`) maps directly to FCM's `data` map — no restructuring needed.

### Option C — Use a unified push hub (OneSignal, Expo Push, etc.)

Hosted services that accept a single API call and fan out to Web Push, FCM, and APNs.
Incompatible with the self-hosted constraint that is already a key decision for this project.

### Option D — Replace webpush-go with a different Go library

No credible alternative Go library for Web Push exists. `webpush-go` at v1.4.0 (Jan 2025)
is the clear choice to keep.

---

## Trade-off Matrix

| Option | Browser compatibility | Self-hosted burden | Payload portability to RN | Safari support |
|--------|----------------------|--------------------|--------------------------|----------------|
| A — VAPID only (current) | Chrome, Firefox, Edge, Safari 16+ | Minimal: one VAPID key pair, no external creds | None out of the box; RN needs its own path | Yes — standard W3C API, push.apple.com endpoint |
| B — VAPID + FCM dispatch | Same as A for browsers; adds Android RN | Moderate: Firebase project + service-account key; FCM token storage | Direct: same notification event dispatches to FCM token | Same as A |
| C — Unified hub | Hub-dependent | Low ops, external dependency | Yes, hub handles it | Hub-dependent |
| D — Different Go library | Same as A | No change | Same as A | Same as A |

---

## Risk and Failure Modes

### Safari-specific risks

**iOS requires home screen installation.** Web Push on iOS 16.4+ is restricted to web
apps added to the Home Screen. Push permission cannot be requested from within Safari
browser on iOS. [TRAINING_ONLY - verify whether iOS 17/18 lifted this restriction]

**Safari permission prompt requires a user gesture.** Safari enforces that
`Notification.requestPermission()` must be called within a user interaction handler.
Calling it on page load will silently fail or be blocked.

**VAPID key format.** Safari requires uncompressed Base64url-encoded VAPID public keys
(65-byte P-256 uncompressed point). `webpush-go` generates these correctly.
[TRAINING_ONLY - verify GenerateVAPIDKeys output format for Safari]

**Push endpoint domain.** Safari push endpoints route via `*.push.apple.com`. If
stapler-squad runs behind a firewall, these domains must be explicitly allowed outbound.

**No background push without notification.** Safari requires every push message to result
in a visible notification (`userVisibleOnly: true` is mandatory). Silent pushes skip
`showNotification()` and cause the subscription to be revoked.

**Comment in push_service.go is wrong.** The comment `// Only works for Chrome/Firefox push (not Safari)`
is incorrect — Safari 16+ uses standard VAPID and will work with the existing backend.

### FCM dispatch risks

**Two credential systems to manage.** Adding FCM requires a Firebase service account JSON
key stored on the server, separate from the VAPID key pair.

**FCM legacy API is shut down.** The legacy HTTP API (`/fcm/send`) was shut down June 2024.
Any FCM integration must target the v1 API (`projects/{id}/messages:send`).

**FCM registration token churn.** FCM tokens expire or are rotated by the OS. The server
must handle `UNREGISTERED` and `NOT_REGISTERED` error responses by deleting the stored token.

---

## Migration and Adoption Cost

**Safari support (Option A, no backend changes):** Frontend audit only. Gate permission
prompt on user gesture. Allow `push.apple.com` in outbound rules. Document iOS home screen
requirement in settings UI. Remove misleading Safari comment in `push_service.go`.
Estimated: 0.5–1 day.

**FCM dispatch layer (Option B, future sprint):** New `FCMNotifier` struct. Add optional
`fcm_token` field to subscription storage record now for schema forward-compatibility.
Estimated: 2–3 days Go + Firebase project setup.

---

## Operational Concerns

- VAPID private key must be persisted outside git. Loss invalidates all existing browser subscriptions.
- 410 Gone cleanup: `webpush-go` returns the raw HTTP response; deletion of stale subscriptions is the caller's responsibility (currently not implemented).
- `push.apple.com` outbound: verify the host running stapler-squad can reach `https://*.push.apple.com:443`.
- Service worker scope must match the app's base path.
- iOS home screen UX: add a hint in the settings panel.

---

## Prior Art and Lessons Learned

**Apple's own guidance (webkit.org, 2022):** "As long as you've coded your web application
to the standards you will be able to reach Safari 16 users on macOS Ventura. You don't
need to join the Apple Developer Program."

**iOS 16.4 confirmation (webkit.org, 2023):** "This is the same W3C standards-based Web
Push that was added in Safari 16.1 for macOS Ventura. If you've implemented
standards-based Web Push for your web app with industry best practices — such as using
feature detection instead of browser detection — it will automatically work on iPhone and iPad."

**FCM v1 cross-platform design:** The `notification.title` + `notification.body` renders
everywhere. The `data` map is the free-form key-value payload. The stapler-squad payload
maps directly — no restructuring needed when FCM dispatch is added later.

---

## Open Questions

- [ ] Does iOS 17 or iOS 18 lift the home screen requirement for Web Push on iPhone?
- [ ] Does `webpush-go`'s `GenerateVAPIDKeys` produce a 65-byte uncompressed P-256 public key (required by Safari)?
- [ ] Which Go library is recommended for FCM v1 dispatch — `google.golang.org/api` or `firebase.google.com/go`?
- [ ] Does stapler-squad need HTTPS for service worker registration on non-localhost addresses?

---

## Recommendation

**Keep `SherClockHolmes/webpush-go`.** Actively maintained, no credible alternative in Go.

**Safari Web Push requires zero backend changes.** Frontend: gate permission on user
gesture; allow `push.apple.com` outbound; document iOS home screen requirement; remove the
wrong Safari comment.

**For RN-ready architecture: design dispatcher model now, implement FCM later.** Refactor
EventBus → push subscriber → VAPID send into EventBus → notification dispatcher →
[VAPID sender, FCM sender (stub)]. Add optional `fcm_token` to subscription storage
schema now. The payload schema is already FCM-compatible.

**Do not introduce a hosted hub.** Self-hosted VAPID + future FCM direct is consistent
with the existing self-hosted constraint.

---

## Pending Web Searches

1. `site:webkit.org web push iOS 17 iOS 18 home screen requirement` — verify whether Apple lifted the iOS home screen restriction
2. `SherClockHolmes webpush-go VAPID public key format P256 uncompressed Safari` — confirm key format for Safari
3. `firebase admin go FCM v1 API 2025` — identify the current recommended Go approach for FCM v1
4. `FCM legacy API shutdown June 2024 confirmation` — confirm the legacy `/fcm/send` shutdown
