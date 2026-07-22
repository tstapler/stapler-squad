# Requirements: backlog-event-driven-updates

**Date**: 2026-07-21
**Type**: feature addition
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement

The backlog UI (`/backlog` list, `/backlog/board` Kanban, and `BacklogItemDetail`'s side panel) does not update when a backlog item's state changes underneath it — a status transition (`idea→ready→in_progress→review→done→archived`), a review verdict landing, or a session attaching. Since this app's backlog pipeline is largely autonomous (reconcilers, headless review, auto-reopen, auto-ship all mutate items server-side with no user action), an operator looking at any of these views has no way to know their screen is stale without manually refreshing.

## Baseline

Today:
- `/backlog` and `/backlog/board` fetch the item list once on mount and never again unless the user navigates away and back or manually triggers a reload.
- `BacklogItemDetail.tsx` polls (`shouldPoll`, ~line 245) only under three narrow conditions: triage is running, status is `review` with no/PENDING verdict, or status is `pr_pending`. It never polls while `in_progress`, and stops polling the moment those conditions no longer hold.
- A status change driven by a reconciler (`reconcileBouncingItems`, `ReconcilePRPending`, auto-reopen, etc.) — which happens continuously and is the normal way most items progress — is invisible in any open view until the user manually refreshes.
- Existing precedent for doing this correctly already exists and ships today: `WatchSessions` (generic session event stream) and `ReviewQueueEvent`/`WatchReviewQueue` (a domain-specific manager built on the same generic `pkg/events.EventBus`) — see `project_plans/review-queue-event-driven/` for the prior art this project mirrors.
- The existing backlog `Notifier`/`EventBusNotifier` (`server/services/backlog_notifier.go`) already publishes to the same event bus, but only for alert-worthy conditions (bouncing, stuck) — not for routine status transitions or verdicts.

## Users / Consumers

- The human operator watching `/backlog`, `/backlog/board`, or an item's detail panel while autonomous backlog work runs in the background.
- Multiple browser tabs/sessions open on the same backlog items concurrently (a change made from one tab must be visible in another without a refresh).

## Success Metrics

- A status transition, verdict recording, or session attach — regardless of whether it originates from the RPC handler or an internal reconciler calling the storage layer directly — becomes visible in every open `/backlog`, `/backlog/board`, and `BacklogItemDetail` view within roughly the same latency `WatchSessions` already delivers for session state (sub-second to a few seconds), without any manual refresh or page reload.
- `BacklogItemDetail`'s `shouldPoll` polling logic is removed/replaced by the push mechanism; `/backlog` and `/backlog/board` gain live updates where they previously had none.
- No regression in existing polling-covered behavior (triage progress, review verdict arrival, pr_pending state) — everything `shouldPoll` covers today must still update at least as promptly under the new mechanism.

## Appetite

Large (3–6 weeks)

*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

Given the Large appetite, scope explicitly includes (beyond the minimum "wire three views to a new stream"):
- Deciding whether to unify the existing `Notifier`/alert-condition event path (bouncing, stuck) with the new general-purpose status-change event into one coherent backlog event model, or keep them as deliberately distinct channels — this decision itself is in scope for Phase 3 planning, not pre-decided here.
- Deeper multi-instance / workspace-isolation hardening for the new stream (see Feasibility Risks) — this codebase supports workspace-based multi-instance state isolation (`.claude/docs/state-isolation.md`), and `WatchSessions` presumably already filters or scopes correctly for it; this project must positively confirm (not assume) the new stream behaves the same way, and fix it if it doesn't.

## Constraints

- No hard deadline. No external compliance/regulatory constraint (internal, single-operator tool).
- Must not regress the "no ad-hoc PRs from main session" / worktree-based session workflow this repo already uses for feature work.

## Non-functional Requirements

- **Performance SLO**: stream push latency should match `WatchSessions`' existing latency characteristics (not separately specified — treat "as fast as the existing session-event stream" as the target; do not introduce a new, slower path). Concrete, falsifiable target: **p95 end-to-end latency (server-side event publish to client-visible UI update) ≤ 2 seconds under normal load**, matching the order of magnitude `WatchSessions` already achieves in production use — derived from `pkg/events.EventBus`'s actual transport characteristics (non-blocking, buffered-channel `Publish`; no network hop; no serialization delay beyond ConnectRPC's own framing, `pkg/events/bus.go:51-72`), which impose no meaningful latency floor beyond what `WatchSessions` already demonstrates is achievable on this same bus.
- **Scalability**: same order of magnitude as current session/notification event volume — this is a single-operator tool, not a multi-tenant service; no special scaling design needed beyond following the existing bus's patterns.
- **Security classification**: internal.
- **Data residency**: not applicable.

## Scope

### In Scope
- A new domain-specific event type and streaming RPC for backlog item changes (working name: `BacklogItemEvent` / `WatchBacklogItems`), mirroring `ReviewQueueEvent`/`WatchReviewQueue`'s shape (item id + updated fields + full current item, so clients can upsert without a round-trip).
- Publishing that event from every code path that actually mutates backlog item state — critically, the **storage layer** (`TransitionBacklogItemStatus` and equivalent ent-repository methods), not just the RPC handler, since `reconcileBouncingItems`, `ReconcilePRPending`, and other internal reconcilers call storage directly and bypass the RPC handler.
- A shared frontend subscription hook (e.g. `useWatchBacklogItems`) consumed identically by `/backlog`, `/backlog/board`, `BacklogItemDetail`, and `BacklogItemPanel` (the fourth consumer, shown inside `SessionDetail`/`SessionDetailView` for a session's linked backlog item — found during Phase 2 research), replacing `shouldPoll` and the fetch-once list/board pattern with upsert-on-event plus `after_seq`-based reconnect/replay (mirroring `WatchSessions`' existing reconnect story).
- Deciding (in Phase 3 planning, with an ADR) whether/how to fold the existing `Notifier` alert-condition events into this new event model.
- Positive verification of workspace/multi-instance event scoping for the new stream (not assumed correct by inheritance from `WatchSessions`).

### Out of Scope
- Changing the actual backlog state machine, reconciliation logic, or review pipeline itself (this project is purely about how state *changes* reach the UI, not what causes them or how correctly — see the already-completed `ef7489d1` and `AttachSessionToItem` isolation-guard work this session for pipeline-correctness fixes).
- Building a general-purpose "watch anything" abstraction beyond backlog items — scope is backlog item events specifically, following the existing per-domain pattern (`WatchSessions` for sessions, `WatchReviewQueue` for the review queue, this new one for backlog items) rather than a premature generic framework.
- Mobile app changes (the mobile client, if any, is out of scope unless research finds it already consumes backlog data live).

## Rabbit Holes

- **Notifier/event unification** (explicitly in scope per the Large-appetite decision, but has real depth): merging two event concepts that currently serve different purposes (user-facing alert toast vs. silent state-sync) could balloon if not scoped tightly in Phase 3 — the plan must draw a clear line on what "unified" means before implementation starts.
- **Workspace/multi-instance event scoping**: if `WatchSessions` turns out to need special-casing for multi-instance isolation that isn't immediately obvious from reading the code, replicating that correctly (not just copy-pasting the pattern) could take longer than expected. Research phase must trace this concretely, not assume.
- **`after_seq` replay semantics** for a second, independently-sequenced event stream: confirm the existing sequencing/replay mechanism generalizes cleanly to a second stream type rather than being implicitly tied to `SessionEvent` specifically.

## Alternatives Considered

- **Keep/extend polling**: reject — doesn't fix the list/board views' total lack of live updates, and existing narrow polling on the detail view is already known to miss states (e.g. `in_progress`); polling harder doesn't fix the architectural mismatch with how the rest of the app already does this.
- **Piggyback on the existing `SessionEvent`/`WatchSessions` stream** instead of a new backlog-specific one: rejected in favor of mirroring `ReviewQueueEvent` — keeps backlog concerns out of the session-shaped proto and matches the codebase's own established per-domain pattern instead of overloading an unrelated one.

## Feasibility Risks

- Workspace-based multi-instance isolation (`.claude/docs/state-isolation.md`) may impose event-scoping requirements on the new stream that aren't yet confirmed — flagged as unconfirmed by the investigation that seeded this project; Phase 2 research must resolve this concretely.
- Hooking the publish call into the storage layer (not just the RPC handler) touches a wide surface of call sites (every reconciler, every `TransitionBacklogItemStatus` caller) — risk of missing a call site and leaving a "silent" mutation path that still doesn't push an event. Phase 3 planning should enumerate all call sites explicitly rather than relying on a single choke point that might not actually be a choke point.

## Observability Requirements

- Log stream connect/disconnect and replay-from-`after_seq` events for `WatchBacklogItems`, matching whatever logging convention `WatchSessions` already uses (grep and follow it, don't invent a new one).
- No new oncall alert condition — this is an internal dev-tool feature; existing service health checks are sufficient.

## Risk Control

Direct ship, no feature flag. Rationale: this is a single-operator tool (not multi-tenant SaaS); the new stream is additive and read-only from the client's perspective, and the existing fetch-on-mount behavior remains a safe fallback if the stream fails to connect — no staged rollout mechanism is needed. If the stream misbehaves post-ship, revert is a normal `git revert`, not a flag flip.

## Open Questions

*(All three resolved during Phase 2 research — see `research/architecture.md` for the first two, and the note below for the third.)*

- ~~Should `Notifier`'s existing alert-condition events (bouncing, stuck) be unified into `BacklogItemEvent`, or kept as a deliberately separate, narrower channel?~~ **Resolved: keep separate.** Different consumers (coalescing toast store vs. non-coalesced UI upsert), different payload shapes, and this repo's own interface-pollution precedent argues for a narrow new `session.ItemChangePublisher`-style interface rather than widening `Notifier` or merging event types.
- ~~Does `WatchSessions` do anything special for workspace/multi-instance scoping that `WatchBacklogItems` must replicate?~~ **Resolved: no special scoping exists or is needed.** `pkg/events.EventBus` has zero scoping logic; isolation is achieved entirely by one `EventBus` instance per per-workspace OS process. `WatchBacklogItems` inherits correct scoping for free, same as `WatchSessions`.
- ~~Are there other frontend consumers of backlog item data beyond the three named views?~~ **Resolved: yes, one more — `BacklogItemPanel.tsx`**, shown inside `SessionDetail`/`SessionDetailView` for a session's linked backlog item. In scope alongside the three originally named views (`/backlog`, `/backlog/board`, `BacklogItemDetail`).
