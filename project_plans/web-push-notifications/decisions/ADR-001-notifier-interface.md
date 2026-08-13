# ADR-001: Notifier Interface for Multi-Target Push Delivery

Status: Accepted
Date: 2026-04-17

---

## Context

The push delivery system must support two delivery targets today (Web Push via VAPID) and
a future target (FCM for Android React Native). The existing `StartPushSubscriber` function
accepts a concrete `*services.PushService` and calls `pushService.SendNotification` directly.

Three extensibility patterns were evaluated:

- **Option A – Flat conditional branches**: add `if fcmService != nil { ... }` inside the goroutine
- **Option B – Notifier interface slice**: define `Notifier interface { Send(...) error }`, accept `[]Notifier`
- **Option C – Channel fan-out**: independent goroutines per delivery target

The adjacent `notifications/subscriber.go` module already uses an `Appender` interface to
decouple the subscriber from its store. This precedent is the strongest signal: the codebase
has already validated the interface injection pattern for an identical structural problem.

---

## Decision

Adopt Option B: define a `Notifier` interface in `server/push/` and refactor
`StartPushSubscriber` to `StartDeliverySubscriber`, accepting `[]Notifier`.

```go
// server/push/notifier.go
type Notifier interface {
    Send(ctx context.Context, n DeliveryNotification) error
    Name() string
}
```

`WebPushNotifier` wraps `*services.PushService` and is the sole implementation today.
When FCM is added, `FCMNotifier` is a new file — no existing code changes.

The `DeliveryNotification` struct (not `PushNotification`) is the canonical input to all
notifiers. `WebPushNotifier` maps it to the webpush-go payload internally. This design
avoids coupling the Notifier interface to Web Push-specific fields.

---

## Consequences

**Positive**
- New delivery targets (FCM, APNs relay) are additive; existing subscriber is unchanged.
- Tests inject a `mockNotifier` — no real push endpoints needed in unit tests.
- Mirrors the established `Appender` interface pattern already proven in the codebase.
- Error isolation: each notifier can fail independently; partial delivery is logged and continues.

**Negative**
- One extra interface definition (~10 lines). Minor complexity increase vs. flat branches.
- A nil element in `[]Notifier` will panic; must assert no nil elements at startup.

**Mitigations**
- Wrap each `n.Send(ctx, notification)` call with a recover-on-panic inside the loop.
- Assert no nil elements in `StartDeliverySubscriber` at construction time.
- Add a per-delivery context with a 10-second deadline to prevent blocked HTTP calls from
  hanging the delivery goroutine indefinitely.

**Rejected alternatives**
- Option A (flat branches): grows unboundedly with each new target; inconsistent with the
  codebase's existing Appender pattern; harder to test.
- Option C (channel fan-out): the EventBus already handles pub/sub fan-out; a second fan-out
  layer adds goroutine lifecycle complexity for no benefit in a single-process server.
- Option D (webhook): adds a network hop for a local server; requires auth and retry logic.

---

## Related

- findings-architecture.md: Q1 analysis
- synthesis.md: Notifier interface recommendation
- notifications/subscriber.go: Appender interface precedent
