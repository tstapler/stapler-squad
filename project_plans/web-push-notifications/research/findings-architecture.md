# Findings: Architecture - Multi-target Push + Preference Storage

Status: Draft
Created: 2026-04-17
Input: requirements.md, server/push/subscriber.go, server/notifications/subscriber.go,
       server/events/bus.go, server/events/types.go, config/config.go,
       server/services/push_service.go, proto/session/v1/types.proto

---

## Summary

Three architecture questions were evaluated against the actual codebase. Recommendations
are grounded in existing patterns rather than abstract preference.

**Q1 (Multi-target delivery)**: The Notifier interface slice (Option B) is the right
choice. It matches the `Appender` interface pattern already proven in the notifications
subscriber, adds zero new abstractions beyond what is already there, and costs one small
interface definition.

**Q2 (Preference storage)**: Extend `config.json` with a `notifications` section
(Option A). Config already does load/save/migrate in one place. A separate file adds
complexity for no benefit in a single-user app.

**Q3 (Trigger logic)**: Hard-coded per-type rules using proto enum constants (Option B),
with a narrow threshold-based fast path where it reads cleanly. The current magic `int32(3)`
comparison is the main correctness hazard; replacing it with named constants costs
nothing and eliminates an entire class of future bugs.

---

## Options Surveyed

### Q1: Multi-target delivery

**Option A – Flat conditional branches**
Add `if fcmService != nil { fcmService.Send(...) }` directly inside the existing
`StartPushSubscriber` goroutine. No new types; all delivery knowledge stays in one
function.

**Option B – Notifier interface slice**
Define a small interface:
```go
type Notifier interface {
    Send(ctx context.Context, n Notification) error
    Name() string
}
```
`StartPushSubscriber` (or a renamed `StartDeliverySubscriber`) accepts `[]Notifier` and
iterates over it. `WebPushNotifier` wraps `*services.PushService`; `FCMNotifier` would
wrap an FCM client when it exists.

**Option C – Fan-out dispatcher with channels/goroutines**
One EventBus subscriber fans out to N internal channels, one per delivery target, each
read by its own goroutine. Delivery targets are fully independent and can have different
backpressure semantics.

**Option D – Outbound webhook**
Push subscriber POSTs a JSON payload to a configurable URL. The RN app's backend
subscribes to that URL. No Go code changes needed when adding delivery targets.

---

### Q2: Notification preference storage

**Option A – Extend config.json**
Add a `notifications NotificationPrefs` field to the existing `Config` struct. Reads and
writes go through the already-existing `LoadConfig` / `SaveConfig` path. The
`ConfigVersion` field already provides a migration hook.

**Option B – Separate notifications-prefs.json**
New file at `~/.stapler-squad/<workspace>/notification-prefs.json`. New load/save pair.
Isolated concern but doubles the file-management surface.

**Option C – Embed prefs in NotificationHistoryStore**
Attach preference metadata to the existing `notifications.json`. Mix of live data and
configuration in one file; complicates both the store's responsibility and the API needed
to sync prefs to the browser.

**Option D – In-memory only with hardcoded defaults**
No persistence. User re-enables push on every restart. Not acceptable given the UX
requirement for a persistent subscribe/unsubscribe toggle.

---

### Q3: Trigger logic design

**Option A – Table-driven config**
A `map[NotificationType]bool` or struct in config controls which types trigger push. User
could theoretically edit `config.json` to suppress a type.

**Option B – Hard-coded per-type rules with proto enum constants**
Each case arm explicitly names the proto constant:
```go
case events.EventNotification:
    if event.NotificationPriority >= int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH) ||
       event.NotificationType == int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED) {
        shouldNotify = true
    }
```
The rules are visible in code, not in a data file, and reference the authoritative proto
definitions.

**Option C – Priority threshold only**
Push if `priority >= MEDIUM`. Simple, but `approval_needed` events can arrive with
variable priorities depending on how they are constructed, so a priority-only check could
miss approvals. Requirements explicitly say: "approval_needed always fires push
regardless of priority."

---

## Trade-off Matrix

| Approach | Extensibility | Testability | Complexity | Fit with codebase |
|---|---|---|---|---|
| **Q1-A Flat branches** | Low — every new target adds an if/else inside the goroutine; function grows unboundedly | Low — must construct all service instances or use nil guards in tests | Minimal at first; degrades linearly with each new target | Poor — notifications/subscriber already uses Appender interface; inconsistency |
| **Q1-B Notifier interface** | High — new target is a new type; existing subscriber unchanged | High — mock Notifier is trivial; slice can hold fakes | Low — one interface, one loop, unchanged call sites | Excellent — mirrors Appender pattern in notifications/subscriber.go |
| **Q1-C Channel fan-out** | Medium — new goroutine per target; clean isolation | Medium — more goroutines to synchronise in tests | High — backpressure semantics differ per target; more goroutine lifecycle management | Poor — EventBus already handles fan-out via buffered channels; redundant layer |
| **Q1-D Outbound webhook** | High for RN backend; zero for browser delivery | Low — HTTP round-trip in unit tests is slow | Medium — new HTTP client, retry logic, auth for the webhook receiver | Poor — adds network hop for a single-user local server; far from existing patterns |
| **Q2-A config.json extension** | Medium — adding a new pref is one field addition | High — config already has tests; load/save is covered | None — uses existing code path | Excellent — Config struct has version migration, nil guards, and SaveConfig already |
| **Q2-B Separate file** | Medium | Medium — new load/save to test | Low-medium — one extra file, one extra load/save pair | Acceptable — consistent with how push-subscriptions.json and vapid-keys.json are stored separately |
| **Q2-C Embed in history store** | Low — prefs and history share a write lock; schema coupling | Low — store tests grow in complexity | High — responsibility confusion; notification history has its own retention + dedup logic | Poor — NotificationHistoryStore is already complex; adding config to it violates single responsibility |
| **Q2-D In-memory defaults** | N/A | N/A | None | N/A — fails the UX requirement; not viable |
| **Q3-A Table-driven config** | High — user can disable any type | Low for correctness testing — config values can drift | Medium — requires config schema, migration, UI surface | Acceptable but overkill for a solo dev tool; also means the "approval_needed always fires" rule is only enforced at config read-time |
| **Q3-B Proto constant rules** | Medium — adding a new type is one case arm edit | High — each branch is a pure bool expression testable with struct literals | None | Excellent — aligns with how notifTypeApprovalNeeded = int32(1) is already named in notifications/store.go |
| **Q3-C Priority threshold** | Medium | High | None | Poor — fails the explicit requirement that approval_needed fires regardless of priority |

---

## Risk and Failure Modes

### Q1 risks

**Notifier interface (recommended)**

- Error isolation: if FCMNotifier panics, it will kill the shared goroutine. Mitigation:
  wrap each `Send` call with `recover()` inside the loop.
- Partial delivery: if WebPushNotifier succeeds and FCMNotifier fails, the error is
  logged and delivery continues — this is the correct behaviour for independent channels.
- Registration order: callers pass `[]Notifier` at construction time; a nil element will
  panic. Mitigation: assert no nil elements in `StartDeliverySubscriber`.

**Flat branches (rejected for Q1-A)**

- The `Subscribe()` method in `PushService` already has a mutex bug (`defer ps.mu.RUnlock()`
  inside a function that acquires `Lock`, not `RLock`). Embedding more delivery logic in
  the same file risks compounding latent bugs.

### Q2 risks

**config.json extension**

- Race condition: `LoadConfig` and `SaveConfig` have no file-level lock beyond what the
  OS provides. If the web UI reads config while the user saves prefs via an API, a torn
  write is possible. Mitigation: add an `os.WriteFile` with a temp-rename (atomic write)
  to `saveConfig`, matching the pattern already used in `notifications/store.go`.
  [TRAINING_ONLY - verify whether saveConfig currently uses atomic write or not; code
  shows `json.MarshalIndent` → `os.WriteFile` directly, without temp-rename]
- Config version: any new `notifications` field must be nil-safe on load since old config
  files won't have it. The existing `LoadConfig` pattern (check nil after unmarshal)
  already handles this; follow the same pattern.

### Q3 risks

**Proto constant rules**

- `NotificationPriority` and `NotificationType` are `int32` in `events.Event` (not the
  proto enum type). The comparison `int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)`
  is valid Go but loses type-safety — a typo produces a wrong int, not a compile error.
  Mitigation: define package-level constants in the push package mirroring the proto
  values; add a unit test that asserts each constant equals the expected proto value.
- The `approval_needed` case in `StartPushSubscriber` currently fires only on
  `EventSessionStatusChanged`. The `EventNotification` path has no approval-needed check.
  If a session publishes `NOTIFICATION_TYPE_APPROVAL_NEEDED` directly via `EventNotification`,
  it will not fire push unless the rule also covers that type explicitly.

---

## Migration and Adoption Cost

### Adopting Q1-B (Notifier interface)

1. Define the `Notifier` interface in `server/push/` (new file, ~10 lines).
2. Create `WebPushNotifier` wrapping `*services.PushService` (~20 lines).
3. Rename `StartPushSubscriber` to `StartDeliverySubscriber`; change signature to
   accept `[]Notifier`. Internal logic shrinks — the payload construction stays; the
   `pushService.SendNotification` call becomes `for _, n := range notifiers { n.Send(...) }`.
4. Update the single call site (currently absent from `server.go` — `PushService` is
   constructed but `StartPushSubscriber` is not called there yet, so the wiring step is
   not a change, it is an addition).
5. Test: write a `mockNotifier` that records calls; existing subscriber tests can be
   adapted with minimal changes.

Estimated scope: ~60 lines of production code changed/added; 0 existing tests broken.

### Adopting Q2-A (config.json extension)

1. Add `NotificationPrefs` struct to `config/config.go`.
2. Add `Notifications NotificationPrefs` field to `Config` struct.
3. Add nil-safe initialisation in `LoadConfig` (follow `SessionDefaults` pattern).
4. Bump `ConfigVersion` (currently 1 → 2); add no-op migration since all fields have
   zero-value defaults.
5. Expose a ConnectRPC RPC to read/write prefs (needed for the settings UI).

Estimated scope: ~30 lines of production code; existing config tests remain valid.

### Adopting Q3-B (proto constant rules)

1. In `server/push/subscriber.go`, replace `int32(3)` with
   `int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)`.
2. Add explicit `NOTIFICATION_TYPE_APPROVAL_NEEDED` case to the `EventNotification`
   branch so the rule matches the requirement: "approval_needed always fires push
   regardless of priority".
3. Add a unit test for `shouldNotify` logic: given a table of (EventType, Priority,
   NotificationType), assert expected bool.

Estimated scope: ~15 lines changed; 1 new test file.

---

## Operational Concerns

### Single-user process isolation

The EventBus is in-process. All delivery runs in the same server process. If
`SendNotification` (web push) blocks on an unresponsive push endpoint, it will block the
delivery goroutine for that batch until the HTTP timeout. The webpush-go library uses
`http.Client` with no explicit timeout in the current code [TRAINING_ONLY - verify TTL
option vs. HTTP client timeout in SherClockHolmes/webpush-go]. Mitigation: wrap each
`Notifier.Send` call with a per-delivery context with a 10-second timeout.

### Fan-out semantics of the current EventBus

`EventBus.Publish` is non-blocking: if a subscriber's channel buffer is full, the event
is dropped silently. The buffer size is set at `NewEventBus(bufferSize)`. With two
subscribers (notifications + push) at default 100-event buffers, a burst of 200+ events
will drop events from whichever subscriber drains slower. This is acceptable for
notifications (coalescing already tolerates drops) but push may miss events during bursts
if it is slow. Mitigation: ensure `Notifier.Send` is fast or runs in a sub-goroutine;
keep delivery latency below the buffer drain rate.

### State file contention

`push-subscriptions.json` is written by `PushService.saveSubscriptions()` which holds no
file lock (just the in-process mutex). Concurrent server instances (workspace isolation
can create them) sharing the same config dir would race on this file. Mitigation:
workspace isolation (`STAPLER_SQUAD_INSTANCE` / workspace hash) already prevents this in
practice; document the assumption explicitly.

---

## Prior Art and Lessons Learned

### The Appender interface in notifications/subscriber.go

The notifications subscriber already follows exactly the Notifier interface pattern.
`StartSubscriberWithInterval` accepts `store Appender` (an interface), not a concrete
`*NotificationHistoryStore`. This was presumably done to enable test injection. The push
subscriber should follow the same evolution: it currently accepts a concrete
`*services.PushService`; moving to a `Notifier` interface is the same step the
notifications code already took.

This is the strongest evidence in favour of Q1-B: the codebase already validated the
pattern in an adjacent module.

### EventBus drop-on-full semantics

`bus.go` line 58-60: the publisher silently drops events when a subscriber's buffer is
full. This was a deliberate design choice. Any delivery system layered on top must be
aware of it and size its buffer accordingly, or accept that low-frequency notifications
(which are typical) will never hit the limit.

### Config versioning pattern

`config.go` already has `ConfigVersion int` and init code in `LoadConfig` that nil-guards
newly added collection fields. The pattern is established and simple. New config fields
should follow the same init-after-unmarshal guard rather than requiring schema migration
logic.

### Separate file for push data (vapid-keys.json, push-subscriptions.json)

`PushService` already stores VAPID keys and subscriptions as separate JSON files in the
config directory. Notification preferences are a different concern (user intent vs. live
credential data), so following Option A (config.json) rather than Option B (another
separate file) keeps the distinction: config.json = user preferences; individual .json
files = live mutable state.

---

## Open Questions

1. **Push subscriber wiring**: `StartPushSubscriber` exists in `server/push/subscriber.go`
   but there is no call to it in `server/server.go` or `server/dependencies.go`. The
   `PushService` itself is not constructed or registered anywhere visible in the server
   startup path. Is the push subscriber currently dead code, or is it wired elsewhere?
   This must be confirmed before the Notifier refactor; if unwired, the first task is
   wiring, not refactoring.

2. **`PushService.Subscribe` mutex bug**: `Subscribe()` calls `ps.mu.Lock()` then
   `defer ps.mu.RUnlock()` — this is a lock/unlock mismatch that will panic (unlock of
   unlocked RWMutex) or corrupt the mutex state. This must be fixed before any delivery
   work. Confirmed in source at `server/services/push_service.go` lines 142-143.

3. **Approval-needed event path**: `EventNotification` with type
   `NOTIFICATION_TYPE_APPROVAL_NEEDED` and `EventSessionStatusChanged` with
   `NewStatus == NeedsApproval` are two separate code paths that can both represent
   "approval needed". The push subscriber handles `EventSessionStatusChanged` but not
   `EventNotification` for this type. Should both paths trigger push? If so, do they need
   deduplication? The 2-second deduplication window in the subscriber uses
   `notificationTag` as a key but the two paths generate different tag formats.

4. **Notifier.Send signature**: the `PushNotification` struct in `push_service.go` uses
   `map[string]interface{}` for `Data`. If the Notifier interface accepts this struct,
   FCMNotifier must translate it. It may be cleaner to define a normalised
   `DeliveryNotification` struct that both notifiers accept, and have `WebPushNotifier`
   convert internally.

5. **config.json atomic write**: `saveConfig` uses `os.WriteFile` directly without a
   temp-rename. Under concurrent access this is not atomic. Given the plan to write
   notification prefs via an API RPC, the RPC handler and config auto-save could race.
   Should be hardened to match the `notifications/store.go` atomic write pattern before
   adding API-writable config fields.

---

## Recommendation

### Q1: Use the Notifier interface (Option B)

Define a `Notifier` interface in `server/push/`. Rename `StartPushSubscriber` to
`StartDeliverySubscriber`. The immediate implementation has one notifier (`WebPushNotifier`);
when the RN app requires FCM, a new notifier is added without touching existing code.
This matches the Appender pattern already used in the adjacent notifications subscriber
and is the minimum necessary abstraction for the stated extensibility requirement.

Do not use Option C (channel fan-out). The EventBus already handles pub/sub fan-out;
a second fan-out layer adds goroutine lifecycle complexity for no benefit in a
single-process server.

Do not use Option D (webhook). Adds a network hop for a local server, requires auth and
retry logic, and is harder to test.

### Q2: Extend config.json (Option A)

Add a `NotificationPrefs` struct to the `Config` struct. This is a single-digit line
change that uses existing load/save infrastructure. No new files, no new load/save pairs.

Before adding the field, harden `saveConfig` to use atomic temp-rename write (matching
`notifications/store.go`) to prevent torn writes when the settings API writes prefs
concurrently with other config changes.

### Q3: Hard-code per-type rules with proto enum constants (Option B), with a priority fast path

Replace `int32(3)` with `int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)`.
Add an explicit `NOTIFICATION_TYPE_APPROVAL_NEEDED` arm in the `EventNotification` case
to satisfy the requirement that approvals always fire push. Keep the rest of the logic
as-is for now; there is no user-configurable preference surface needed at this stage.

Table-driven config (Option A) is worth revisiting if the user asks "why did I get a push
for X?", but that feedback loop does not exist yet. Adding config-driven rules before
that user need is validated is premature generalisation.

---

## Pending Web Searches

These claims were derived from training data and source code inspection; they should be
verified before implementation if correctness matters:

- `SherClockHolmes/webpush-go` HTTP client timeout behaviour: does the library set a
  default timeout on the underlying `http.Client`, or does it inherit the process default
  (infinite)? If infinite, a blocked push endpoint will block the delivery goroutine
  indefinitely.
- FCM HTTP v1 API payload envelope format: is the `notification` + `data` structure in
  the FCM v1 API compatible with the existing `PushNotification` struct fields, or does
  it require a different top-level shape? Determines whether a shared `DeliveryNotification`
  struct is feasible.
- Safari 16+ VAPID support: does Safari's push endpoint return standard 201/410 HTTP
  status codes that `webpush-go` already handles, or are there Safari-specific response
  codes that require special-casing in `sendToSubscription`?
