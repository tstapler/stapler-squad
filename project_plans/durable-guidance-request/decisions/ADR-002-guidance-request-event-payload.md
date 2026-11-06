# ADR-002: New `GuidanceRequestEvent`/Payload Type, Not Piggybacked on `BacklogItemChangedEvent`

**Date**: 2026-09-12
**Status**: Accepted

## Context

Requirements AC2/AC5 need durable, event-bus-based notification when a `GuidanceRequest` is answered (Constraint: "ride the existing `BacklogItemEventPublisher`/`EventBus` pattern", not a new poll loop). The existing `events.BacklogItemChangedEvent`/`BacklogItemEventPayload` (`pkg/events/types.go`, adapted by `server/services/backlog_item_event_publisher.go`) is the concrete precedent for "domain change → bus event." Two designs were considered:

- **(A)** Piggyback: for `backlog-item`-scoped requests, publish a `BacklogChangeGuidanceRequestAnswered` kind through the existing `BacklogItemChangedEvent`/`BacklogItemEventPayload`, wired through `mapBacklogChangeKind`; `session`/`standalone` scopes (which have no `BacklogItem`) get a second, bespoke event type.
- **(B)** New, scope-uniform: one new `events.GuidanceRequestEvent`/`GuidanceRequestEventPayload` type (new `EventType` constant, e.g. `EventTypeGuidanceRequestAnswered`) used identically for all three scopes.

## Decision

**(B)** — one new, scope-uniform event/payload type.

## Rationale

- **`mapBacklogChangeKind` panics on an unmapped kind** (`server/services/backlog_item_event_publisher.go:83-85`, confirmed by direct read) and is caught only by the adapter's own top-level `recover()` — silently dropping the event, never delivered. Adding a `BacklogChangeGuidanceRequestAnswered` kind there is safe for the `backlog-item` scope, but design (A) then requires a *second*, differently-shaped event type anyway for `session`/`standalone` — two consumers (`wait_for_backlog_event`-style waiters, live-delivery/respawn logic) would need two different subscription/parsing paths for what is conceptually one event ("this guidance request was answered").
- Design (A)'s "uniform-looking API, forked implementation" is exactly the shape `ux.md`/`architecture.md` warn against for the shared React component (Rabbit Hole: "the shared UI component ... no existing 'one component reused verbatim across 3 views' pattern"). The same discipline applies to the delivery-side event: one `GuidanceRequestID` + `Scope` + `Answer` payload, one subscriber code path, branching only on `Scope` where the *consumer* (triage respawn vs. `write_to_session` vs. UI refetch) actually differs — not on which event type arrived.
- `BacklogItemEventPayload` carries a full `*session.BacklogItemData` (`Item` field) — meaningless and would have to be nil-guarded everywhere for `session`/`standalone` scope events, reintroducing exactly the awkward optional-field shape ADR-001 rejected for the entity itself.

## Consequences

- New `events.EventType` constant + `events.GuidanceRequestEventPayload{ID, Scope, ItemID *string, SessionUUID *string, QuestionType, Answer, AnsweredAt}` + `events.NewGuidanceRequestAnsweredEvent(payload)` constructor, mirroring `NewBacklogItemChangedEvent`'s shape (`pkg/events/types.go`).
- A small, new adapter method (not a `mapBacklogChangeKind` branch) — `GuidanceRequestEventPublisher.PublishAnswered` or an added method on `BacklogItemEventPublisher` — wraps its own `recover()` per the existing best-effort-side-channel idiom (`server/services/backlog_item_event_publisher.go:21-37`).
- `backlog-item`-scoped UI/MCP consumers that already watch `BacklogItemChangedEvent` (e.g. `wait_for_backlog_event`) do NOT automatically see guidance-answered events — any call site that wants "notify me either way" subscribes to both event types explicitly. Acceptable: no current call site needs a merged view, and an explicit dual-subscribe is clearer than an implicit dual-purpose event.

## Alternatives Rejected

| Alternative | Rejected because |
|---|---|
| (A) Piggyback on `BacklogItemChangedEvent` for item-scope + bespoke type for session/standalone | Forks into two consumer code paths for one conceptual event; `mapBacklogChangeKind`'s panic-on-unmapped-kind pattern only covers the item-scope half; leaves `Item *session.BacklogItemData` nil-guarded for two of three scopes. |
| Reuse `BacklogItemChangedEvent` for all three scopes, adding nullable session/standalone fields to `BacklogItemEventPayload` | Pollutes the existing payload (already consumed by unrelated subscribers) with guidance-specific optional fields; violates the same "don't leak into an unrelated shared type" reasoning as ADR-001. |
