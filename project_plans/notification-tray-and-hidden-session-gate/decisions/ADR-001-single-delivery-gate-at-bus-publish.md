# ADR-001: One Delivery Gate = EventBus Publish Filter + Shared Pure Policy Function

**Status**: Proposed (revised in plan repair iteration 3: copy-on-write index, Gate owns the index and is seeded in `BuildRuntimeDeps`, consumer-gate naming)
**Date**: 2026-10-07
**Project**: notification-tray-and-hidden-session-gate

## Context

Hidden-session suppression is implemented four ways today (priority==LOW in
`server/services/notification_service.go:142`, reason-set in
`server/review_queue_manager.go:442`, unconditional boolean in
`server/services/session_service_events.go` x4 and
`server/services/autonomous_orchestration_service.go:619`, and none at all in
`server/push/subscriber.go` `buildStatusChangeNotification`). The history store
ingests every `EventNotification` (`server/notifications/subscriber.go`), and
`WatchSessions` cannot filter them because `EventNotification` carries
`Session == nil` (`pkg/events/types.go` `NewNotificationEvent`). Source:
`research/architecture.md` sections 1-2.

## Decision

1. Add `EventBus.SetPublishFilter(func(*Event) bool)` (atomic pointer) in
   `pkg/events/bus.go`. A rejected event is dropped **before** `Seq` assignment,
   so it never reaches live subscribers or the `EventsSince` replay buffer.
2. A new package `server/deliverygate` owns the pure function
   `ShouldDeliver(Visibility, Facts)` (typed `Facts{Type, Hint}`;
   no `priority`, no status field, no raw int pair) and a concrete `Gate` installed as the
   publish filter for `EventNotification` events only.
3. **The filter is I/O-free and takes no lock on the read path.** It reads an
   atomic `FlagCache` (refreshed by `UpdateFeatureFlag` and a 5s ticker, never
   `config.LoadConfig()`, which reads and parses the file on every call,
   `config/config.go:1373`) and an in-memory `VisibilityIndex` (session UUID,
   title and tmux name to `Visibility` and `HiddenKind`). **The index is
   copy-on-write**: an `atomic.Pointer` to an immutable struct, so a read is an
   atomic load and takes no lock at all. Writers (create/rename/delete feed
   points, seed, refresh) take a private writer-only mutex that guards
   copy-and-swap and calls nothing, so an `Upsert` made while the caller holds
   `Instance.mu` cannot invert with a reader. (An earlier draft kept an
   `RWMutex` and called it "lock-free by construction"; architecture re-review
   C1 correctly flagged the contradiction and the copy-on-write form was
   chosen because writes are rare, tens of session create/rename/delete per
   day, and copying a map of a few hundred sessions is cheap; a benchmark in
   Task 2.2b records the cost.) It never calls poller `FindInstance` (takes
   `rqp.mu.RLock`; `MatchesID` takes `pmMu.Lock` per instance,
   `session/instance_tmux.go:316`) or `InstanceStore`. A miss returns
   `Unresolved` (fail open) and schedules a bounded async singleflight refresh
   with a 5s minimum interval between attempts. Only positive results are cached.
4. **Installed at bus construction; the Gate owns its index from construction.**
   Producers are wired before `server/server.go:300`
   (`dependencies.go:856,865,878,1144`, `session_service.go:970`) and that block
   sits inside `if configErr == nil`. `SetPublishFilter` is called right after
   `events.NewEventBus(100)` at `session_service.go:1008,1022` with a `Gate`
   that already contains its (empty) index and `FlagCache`; there is no later
   `Bind` step. `BuildRuntimeDeps` calls `Gate.SeedFromInstances(instances)`
   synchronously right after `storage.LoadInstances()`
   (`server/dependencies.go:652`), before any instance is wired (`:876`) or
   started (`:930`), so restore-time producers (`onColdRestoreLostHistory`,
   rate-limit callbacks) resolve hidden sessions correctly (architecture C3).
   Before the seed the filter fails open and counts; that window contains no
   session-scoped producers. The filter body is wrapped in a `recover` that fails
   open, increments `notification_gate_filter_panic_total` and logs once, so a
   panic in the gate can never break the caller goroutine of a `Publish` site
   (plan Task 2.3e). All gate counters are mirrored in-process and served by
   `GetDeliveryGateStats` because the OTel meter is a no-op when telemetry is not
   initialized (`telemetry/telemetry.go:274-279`; plan Story 2.8). The primary index feed for new sessions is
   `CreateDirectorySession` (`session_service_diagnose.go:122`), the choke point
   of every `Hidden: true` producer found so far (architecture C8).
5. The two paths the bus does not reach take **single-method, consumer-side
   interfaces** implemented by `Gate`: `push.SessionDeliveryGate`
   (`AllowStatusChange(sessionID)`) for `buildStatusChangeNotification`, and
   `services.AutoApprovedGate` (`AllowAutoApprovedRow`), declared beside the
   approval handler (`approval_handler.go` is `package services`; there is no
   `approval` package), for `AppendAutoApproved`. Only `server/push` is
   guaranteed not to import `server/deliverygate`. The Slack/review-queue path needs no gate:
   `ReviewQueuePoller.shouldSkipSession` and `Determine()` already exclude
   hidden routine reasons, so `suppressForHidden` is deleted (plan Tech Debt).
6. Legacy per-site checks are deleted **in the same commit that makes the gate
   default-on** (ADR-004), not in the PR that introduces the gate.
7. No-bypass proof: a matrix test with real subscribers plus a **type-based**
   sink guard (`go/packages`: `Notifier` implementers, `EventNotification`
   consumers, store `Append*` callers vs an allowlist), plus a lock-order
   `-race` test that publishes while holding poller and instance locks.

## Alternatives Considered

- **Producer-side check at `NewNotificationEvent`**: rejected. 47 call sites (non-test, recounted by `grep` at `013856269`),
  cannot see `EventSessionUpdated`-derived push, and is the status quo that
  regressed three times.
- **Each subscriber calls a shared helper**: rejected as primary. A fourth
  subscriber silently bypasses it. Retained only for the three non-bus paths.
- **Gate in `store.Append` only**: rejected. Covers history, not push or toast.
- **Synchronous poller/DB lookup inside the filter** (earlier draft with a TTL cache): rejected; lock-order and I/O risk on lifecycle-lock-holding `Publish` callers.
- **Stamp `Hidden` on the event at publish time**: rejected. Needs the same
  lookup at the same place, and a stale stamp would outlive a flag flip.

## Consequences

- Analytics/MCP bus consumers no longer see suppressed hidden notifications
  (acceptable: only `EventNotification` consumers are `notifications`, `push`,
  `event_converter`, analytics; verified in architecture.md section 2).
- The filter runs synchronously on the caller goroutine of ~108 `Publish`
  sites (recounted), some under lifecycle locks, so it takes no lock (atomic
  loads only; the lock-order test also exercises `Upsert` under `Instance.mu`).
  Its latency budget is **measured, not guessed** (triad iteration 1, plan Task
  2.3f): the filter's p99 must stay within 5% of an unfiltered `Publish` in
  `BenchmarkPublishWithFilter` versus `BenchmarkPublishNoFilter`, with absolute
  numbers recorded after the first run (the 5% ratio is the initial, INFERRED
  threshold).
- A new hidden session must be in the index before its first hook can fire:
  the session create path upserts synchronously. A miss before that is
  delivered (fail open), counted, and not cached.
- Adds one package and one bus method; removes seven per-site checks (in PR 2b): the LOW check in `notification_service.go`, four `inst.Hidden` branches in `session_service_events.go`, `autonomous_orchestration_service.go:619` and the `suppressForHidden` predicate in `review_queue_manager.go` (plus its raw `inst.Hidden` read at L413, deleted with it). The earlier "six" miscounted: the list in this ADR's Context and in plan Task 2.4c always named seven.
