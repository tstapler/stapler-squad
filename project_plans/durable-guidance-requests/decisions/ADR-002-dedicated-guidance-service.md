# ADR-002: New Standalone `GuidanceService` Proto/RPC, Not an Extension of `BacklogService` or `SessionService`

**Status**: Accepted (Auto Mode — no open questions)
**Date**: 2026-09-11

## Context

`research/architecture.md` §1 found that this codebase splits ConnectRPC services by
bounded concept (`BacklogService`, `SessionService`, `UnfinishedWorkService`,
`TymuxRolloutService`, etc., each its own `.proto` file and its own
`NewXServiceHandler` registration in `server/server.go`), and cited
`NotificationService` as a precedent for "a cross-cutting, non-backlog-owned
service."

That citation needs a correction, verified by reading the actual code rather than
re-trusting the research doc: `NotificationService`'s RPCs
(`GetNotificationHistory`, `MarkNotificationRead`, `SendNotification`,
`ClearNotificationHistory`) are declared inside `proto/session/v1/session.proto`'s
`SessionService` block (`session.proto:150-152` for `GetNotificationHistory`), and
`*NotificationService` (`server/services/notification_service.go`) is a plain Go
struct **embedded as a field on `*SessionService`**
(`server/services/session_service.go:110,682`) whose RPC methods are thin
delegations (e.g. `func (s *SessionService) GetNotificationHistory` at
`server/services/session_service.go:5098`, forwarding to `s.notificationSvc`).
There is no `NewNotificationServiceHandler` anywhere in `server/server.go` —
`NotificationService` is not independently registered as a ConnectRPC service at
all.

So this codebase actually has two different precedents for "a cross-cutting
concern that isn't really about backlog items or session lifecycle":

1. **Dedicated top-level service** (`BacklogService`): own `.proto` file, own
   `NewBacklogServiceHandler` registration (`server/server.go:593`), own
   feature-flag interceptor (`"backlog"`, `server/server.go:588-591`), own
   `StreamingWSBridge` for its `WatchBacklogItems` streaming RPC
   (`server/server.go:598-605`).
2. **Composed-into-an-existing-service** (`NotificationService`): RPCs declared
   in `session.proto`, implementation delegated from `*SessionService` to an
   embedded struct.

## Decision

Model `GuidanceService` on **pattern 1** (`BacklogService`), not pattern 2
(`NotificationService`): a new `proto/session/v1/guidance.proto` file declaring
`service GuidanceService`, a new `server/services/guidance_service.go` with its own
`*GuidanceService` struct (not embedded into `SessionService` or `BacklogService`),
registered independently in `server/server.go` via
`sessionv1connect.NewGuidanceServiceHandler(deps.GuidanceService, ...)`, its own
`guidance_requests` feature-flag interceptor, and its own `StreamingWSBridge` for a
new `WatchGuidanceRequests` streaming RPC.

## Reasoning

- **`GuidanceRequest` is not owned by `SessionService` or `BacklogService`.** Per
  the requirements, a request can be scoped to a backlog item, a session, *or*
  neither. `NotificationService`'s delegation-into-`SessionService` pattern makes
  sense there because notifications are fundamentally about *sessions*
  (`SendNotification` resolves a session's display name via
  `ReviewQueuePoller`/`storage` — `server/services/notification_service.go`'s own
  doc comment). `GuidanceRequest` has no such single natural owner — exactly the
  condition under which this codebase already reaches for pattern 1
  (`BacklogService` itself isn't owned by `SessionService` either, despite
  backlog items routinely spawning sessions).
- **A feature flag needs a clean boundary.** `BacklogService`'s whole-service
  feature-flag interceptor (`server/server.go:588-591`) is the existing precedent
  for "this entire cross-cutting feature can be toggled off independently,"
  which the requirements' constraint of "no existing behavior regresses" wants
  for a first-iteration feature: a `guidance_requests` flag that's easy to
  disable if the abuse/spam risk (`research/pitfalls.md` §5) surfaces in
  production. That's naturally a service-level concern, not an
  RPC-inside-`SessionService` concern.
- **A dedicated `WatchGuidanceRequests` stream is needed regardless of the
  RPC-hosting decision**, because `BacklogItemEvent`'s `oneof` (proto's existing
  live-fan-out shape) is item-scoped only (`item_id` is present on every event
  sub-message) and cannot represent a session-scoped or standalone guidance
  request without an awkward optional-item-id retrofit of a message family that
  several other consumers already assume is item-scoped. A new
  `GuidanceRequestEvent` message and its own streaming RPC, filterable by
  `item_id` (optional), `session_id` (optional), or neither, cleanly covers all
  three scope kinds from one channel.

## Consequences

**Positive**: `GuidanceService` can be feature-flagged, tested, and reviewed in
total isolation from `BacklogService`/`SessionService` — no risk of destabilizing
either. The registration recipe is a mechanical copy of `BacklogService`'s
(down to the `StreamingWSBridge` wiring), which is a known-working pattern with an
existing WebSocket bridge already handling the browser's 6-connections-per-origin
limit.

**Negative**: one more service to keep in sync in `server/dependencies.go`'s
`RuntimeDeps`/`ServerDeps` structs (mechanical, low-risk) and one more `make
proto-gen` surface. A `GuidanceRequest` answered while `BacklogItemDetail.tsx` is
open triggers a second, independent stream subscription
(`useWatchGuidanceRequests` alongside `useWatchBacklogItems`) rather than arriving
on an already-open one — an acceptable and structurally identical tradeoff to how
`GoalPanel`/session goals already work as their own concern independent of
`WatchSessions`.

## Alternatives Considered

**Add guidance RPCs to `BacklogService`.** Rejected: a standalone-scoped guidance
request has no backlog item at all, so `BacklogService`'s `"backlog"` feature flag
and item-shaped request/response messages would apply to something that is, by
definition, not always about a backlog item.

**Delegate into `SessionService` like `NotificationService`.** Rejected: a
session-scoped guidance request is a legitimate case, but standalone requests
have no session either, and `SessionService` is already the largest, most
actively-changing service file in the codebase (`server/services/session_service.go`
is 5000+ lines per the line numbers cited above) — adding a third unrelated
concern (after shells and notifications) increases its blast radius further
rather than following the "split by bounded concept" convention this repo
otherwise follows for exactly this kind of new, independently-scoped entity.
