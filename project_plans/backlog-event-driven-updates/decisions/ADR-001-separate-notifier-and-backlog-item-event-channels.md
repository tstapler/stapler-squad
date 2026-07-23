# ADR-001: Keep `Notifier`/`EventBusNotifier` and `BacklogItemEvent`/`ItemChangePublisher` as Separate Channels

**Status**: Accepted
**Date**: 2026-07-21
**Project**: `project_plans/backlog-event-driven-updates/`

## Context

This project adds real-time push updates for backlog item state to `/backlog`, `/backlog/board`, `BacklogItemDetail`, and `BacklogItemPanel`. The mechanism is a new `BacklogItemEvent` (a `oneof` of per-concern sub-messages: status changed, verdict recorded, session attached, item updated, item archived, item removed) published from repository-layer mutation hooks through the existing `pkg/events.EventBus`, and consumed via a new `WatchBacklogItems` streaming RPC.

This codebase already has a live, shipping event-publishing path for backlog items: `session.Notifier` (interface defined in `session/backlog_lifecycle.go:28`), implemented by `EventBusNotifier` (`server/services/backlog_notifier.go`), which also publishes to the *same* underlying `pkg/events.EventBus`. Its call sites are narrow and specific: `session/backlog_lifecycle.go:538` and `session/review_gate.go:161,199,237`, covering exactly three conditions — an item bouncing (repeated review↔rework cycling), an item stuck, and an abandoned review. Its consumer is `server/notifications/subscriber.go`, which feeds a toast/notification-history store that **coalesces** same-type notifications for the same item within a 500ms window (`sessionID:notificationType` coalescing key — see the bug this key design already had to fix, documented in `EventBusNotifier.Notify`'s doc comment, where `itemID` had to be threaded as the event's `sessionID` specifically to avoid cross-item coalescing collisions).

Given both channels now publish to the same bus from some of the same broad call-site neighborhoods (e.g. `session/backlog_lifecycle.go`), the question of whether to unify them into one event model was explicitly left open in `requirements.md` for this planning phase to resolve, with a caution against letting "Notifier/event unification" balloon into an unscoped rabbit hole (`requirements.md` Rabbit Holes section).

## Decision

**Keep `Notifier`/`EventBusNotifier` and the new `session.ItemChangePublisher`/`BacklogItemEvent` as two deliberately separate, independently-evolving channels.** Both may fire from the same underlying repository mutation in some cases (e.g. a stuck-detection reconciler might both call `Notifier.Notify` for the alert and trigger a status transition that fires a `BacklogItemStatusChangedEvent`), but they are two distinct interfaces, two distinct proto message families, and two distinct frontend consumption paths — not merged into one discriminated union or one wider interface.

Concretely:
- `session.ItemChangePublisher` is a new, narrow interface (`PublishItemChanged(item *BacklogItemData, change BacklogItemChange)`), defined in the `session` package next to `Notifier`, following the exact same shape and placement convention.
- Its `server/services` adapter (`BacklogItemEventPublisher`) is structurally parallel to `EventBusNotifier` but produces a different generic-bus payload (`events.EventBacklogItemChanged` / `BacklogItemEventPayload`, not `events.EventNotification`).
- Wiring happens via its own `SetItemChangePublisher` setter call in `server/dependencies.go`, placed next to (not replacing) the existing `backlogLifecycleListener.SetNotifier(&services.EventBusNotifier{Bus: eventBus})` call.

## Consequences

**Positive:**
- Each channel keeps a payload shape that exactly matches its consumer's needs: `Notifier.Notify`'s flat `(itemID, title, message, notificationType, priority)` signature stays simple for human-readable alerts; `BacklogItemEvent`'s typed oneof stays rich enough for client-side upsert (item id + updated fields + full current item) without either shape compromising for the other's use case.
- The existing 500ms coalescing behavior in `server/notifications/subscriber.go` continues to apply only to alert-worthy conditions, where collapsing repeated notifications is correct UX. Routine status transitions — which must never be silently coalesced away, since every one of them is a state the UI must reflect — are structurally incapable of being coalesced because they never enter that pipeline.
- Follows this repo's own interface-pollution precedent (`.claude/rules/interface-pollution-checklist.md`): a narrow, single-method interface defined in the consumer package (`session`), not a generalized/widened interface trying to serve two different obligations.
- Cheaply reversible: because both channels are thin adapters over the same `pkg/events.EventBus`, a future decision to unify them (should a concrete need arise) would touch two adapters and two call-site wiring points, not a large refactor.

**Negative / accepted trade-offs:**
- Some repository-layer call sites now make two separate calls (one to `Notifier`, one to `ItemChangePublisher`) instead of one unified call — slightly more wiring surface per mutation site where both apply (in practice, only the stuck/bounce-adjacent call sites in `session/backlog_lifecycle.go` and `session/review_gate.go` overlap; the majority of the 9 hooked repository methods — `TransitionBacklogItemStatus`, `UpdateBacklogItem`, `ArchiveBacklogItem`, `DeleteBacklogItem` in `session/ent_repository_backlog.go`, plus `SaveReviewVerdict`, `CreateItemSessionWithVerdict`, `CreateItemSession`, `UpdateItemSessionSessionUUID`, `UpdateItemSessionTriageResult` in `session/storage_backlog.go` — only ever call the new publisher).
- Two parallel proto message families (`NotificationEvent`-shaped vs. `BacklogItemEvent`-shaped) to maintain going forward, rather than one.
- A future contributor unfamiliar with this ADR might be tempted to unify them "for consistency" — this document exists specifically to make that a deliberate, informed re-decision rather than an accidental refactor.

## Alternatives Considered

1. **Unify into one `BacklogItemEvent`-style mega-event** covering both alert conditions and routine transitions, with a `severity`/`kind` discriminator and a single subscription path.
   - Rejected: would force the coalescing consumer (`server/notifications/subscriber.go`) and the non-coalescing consumer (list/board/detail upsert) to either share one coalescing policy (wrong for one side or the other) or add per-event-kind coalescing exceptions into what is currently simple, uniform coalescing logic — increasing complexity in the one place (`subscriber.go`) that currently has none.

2. **Widen the existing `Notifier` interface** to add a second method for routine transitions (e.g. `Notifier.PublishItemChanged(...)` alongside `Notifier.Notify(...)`), avoiding a new interface entirely.
   - Rejected: violates this repo's own interface-pollution guidance — an interface should be defined and scoped to what its specific consumer needs, not grown to serve a second, structurally different consumer (`server/notifications/subscriber.go` vs. the new `WatchBacklogItems`/frontend-upsert consumer) bolted onto the same type. It would also make `EventBusNotifier`'s single production implementation responsible for two unrelated payload-construction paths, increasing the odds of the exact `itemID`-as-coalescing-key class of bug the doc comment on `EventBusNotifier.Notify` already had to fix once, this time applied to a payload shape that was never designed for coalescing at all.

3. **Route routine transitions through `Notifier` today, but instruct `server/notifications/subscriber.go` to special-case bypass coalescing for certain notification types.**
   - Rejected: still couples two independently-evolving concerns onto one wire format and one interface; the "bypass coalescing for kind X" special-casing tends to accumulate exceptions over time as more routine-transition kinds are added (verdict recorded, session attached, item updated, archived, removed), whereas keeping the channels separate means the coalescing consumer never needs to know those kinds exist at all.
