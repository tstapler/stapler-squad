# Architecture Research: kanban-board-view

Agent 3 (Architecture) — SDD Phase 2 research. Scope: how a `SessionBoard.tsx`
view should be wired into the existing dashboard, not business-logic
EventStorming (this is a UI feature with simple state transitions per
`requirements.md`'s Appetite section).

## 1. What feeds `SessionList.tsx` today (and what `SessionBoard.tsx` needs too)

`web-app/src/components/sessions/SessionList.tsx:57-97` (`SessionListProps`)
takes `sessions: Session[]` plus ~20 `onX` callback props (pause/resume/
delete/clone/checkpoint/etc.) as **props**, not internal fetches — the
component is presentational over data the caller (`app/page.tsx`) already
holds. Inside, it additionally pulls from four hooks/contexts:

- `useReviewQueueContext()` (`lib/contexts/ReviewQueueContext.tsx`) — review-queue items, indexed by `sessionId` for badges.
- `useApprovalsContext()` (`lib/contexts/ApprovalsContext.tsx`) — `approvals: PlainApproval[]` (RTK Query, 5s polling) plus `clearedSessions` for optimistic-approval suppression.
- `useAppSelector(selectDetectedStatusMap)` (`lib/store/sessionsSlice`) — terminal-detected status overlay.
- `useInsightsSummary()` (`lib/hooks/useInsightsService.ts`) — cost/insights data.

**Implication for `SessionBoard.tsx`**: it needs the same four data sources
to compute the "Needs Review" column and render cards with parity to the
list view. Both `ReviewQueueContext` and `ApprovalsContext` are sessionId-
keyed pending-approval signals — per `requirements.md`'s open question, the
board's "Needs Review" column should be defined as the union of
`reviewItemBySessionId` membership and `approvals` entries for that
session, mirroring exactly how `SessionCard`/`SessionRow` already decide
whether to show a review badge — not a new derivation.

`sessions` itself is *not* fetched inside `SessionList.tsx` — it's fed from
`app/page.tsx`, which owns `useSessionService({ autoWatch: true, ... })`.
That hook opens a ConnectRPC server-stream (`WatchSessions`, see §5) and
dispatches `upsertSession`/`removeSession` into `sessionsSlice`, so `sessions`
is already continuously live-updated. `SessionBoard.tsx` should receive the
exact same `sessions` prop from `app/page.tsx`, not open its own watch.

## 2. Session-control RPC call sites (pause/resume/stop)

Actions flow: `app/page.tsx` → `useSessionService()` (the RPC layer,
`lib/hooks/useSessionService.ts`) → thin per-session adapter
`useSessionActions(sessionId)` (`lib/hooks/useSessionActions.ts`) used by
individual cards.

Exact functions (`lib/hooks/useSessionService.ts:97-111`):
```ts
pauseSession: (id: string) => Promise<Session | null>;
resumeSession: (id: string, updates?: { title?: string; tags?: string[] }) => Promise<Session | null>;
deleteSession: (id: string, force?: boolean) => Promise<boolean>;
archiveSession: (id: string) => Promise<boolean>;
unarchiveSession: (id: string) => Promise<boolean>;
```
`pauseSession`/`resumeSession` (lines 364-383) are thin wrappers around a
single shared `updateSession(id, { status })` call — there is no separate
Pause/Resume RPC, just `SessionService.UpdateSession` with a new `status`.

**Finding — no `StopSession` RPC exists.** `proto/session/v1/session.proto`
has no `StopSession` method (confirmed via `grep rpc.*Session`); the closest
equivalents are `DeleteSession` (destructive) and `ArchiveSession` (soft,
matches a "Complete" column better). `requirements.md`'s Constraints section
says "pause_session, resume_session, stop_session **equivalents**" — Phase 3
planning must pick which real RPC backs a drag-to-"Complete"/"Stop" action;
`ArchiveSession` is the closer semantic match to a kanban "Done" column than
`DeleteSession`, which is destructive and already gated behind a confirm
modal elsewhere in the UI.

`useSessionActions(sessionId)` is the existing per-session adapter pattern
(`pause()`, `resume(updates?)`, `delete(force?)`, `restart()`, etc., each
just binding `sessionId`) — the board's drag handler should call through
this same adapter (or the raw `useSessionService` methods directly, since
the board operates on many sessions rather than one bound id) rather than
introducing a third calling convention.

## 3. Existing view-mode toggle precedent

`SessionList.tsx` already has a two-mode toggle: `viewMode?: "card" | "row"`
(prop, default `"row"`, `SessionList.tsx:95-96`), branching render logic at
lines 658-696/1224. Two things worth reusing/matching:

- **It's a plain string-union prop controlled entirely by the parent**
  (`app/page.tsx`) — `SessionList.tsx` itself never persists `viewMode` to
  `localStorage` (confirmed: `viewMode` is absent from `BASE_STORAGE_KEYS`,
  `SessionList.tsx:223-236`). Persistence, if any, is the caller's
  responsibility. A new List/Board toggle should follow the same shape:
  `app/page.tsx` owns the `"list" | "board"` state and passes down which
  component to render, rather than either child component owning it.
- `SessionList.tsx`'s own internal state (search, filters, grouping,
  column visibility) **is** persisted, via `makeStorageKeys(prefix)` +
  `loadFromStorage`/`saveToStorage` helpers (`SessionList.tsx:223-267`),
  keyed under `stapler-squad-*` string constants, with an optional
  `storageKeyPrefix` prop so multiple instances (e.g. split view,
  `PaneSplitRenderer.tsx`) don't collide. The dashboard-level List/Board
  toggle should use this exact pattern: a new top-level
  `stapler-squad-dashboard-view-mode` key read/written with the same
  `loadFromStorage`/`saveToStorage`-shaped helper, not a bespoke mechanism.
  There's no separate per-"workspace" storage scope in this codebase today —
  each workspace already runs as its own instance/origin (`STAPLER_SQUAD_INSTANCE`,
  see `.claude/docs/state-isolation.md`), so plain `localStorage` is already
  workspace-scoped by virtue of being per-origin.

No dedicated `useViewMode`/`ViewToggle` hook or component exists yet to
import — this would be new, small, shared code (see §7).

## 4. A kanban precedent already exists in this codebase: `BacklogBoard.tsx`

`web-app/src/components/backlog/BacklogBoard.tsx` (routed at
`app/backlog/board`) is a **status-column board for backlog items** —
structurally the closest thing to what this feature needs, and should be
the template to mirror rather than designing from scratch:

- `COLUMNS: { status; label }[]` is a plain static array (`idea`/`ready`/
  `in_progress`/`review`/`done`), with a pure `stageOf(status)` mapping
  function deciding which column a given item's status resolves to
  (`BacklogBoard.tsx:21-24`, delegating to `deriveStageDisplay` from
  `StageTracker.tsx`) — directly analogous to the Running/Needs
  Review/Paused/Complete precedence-rule mapping this feature's Rabbit
  Holes section calls out as needing definition.
- Columns are rendered by filtering the full item list per column
  (`items.filter((i) => stageOf(i.status) === column.status)`,
  `BacklogBoard.tsx:301-311`) — no separate per-column data source.
- **It is explicitly read-only / no drag-and-drop.** State changes happen
  via `onAction(action, itemId)` button clicks on `BacklogItemCard`, not
  drag. This is exactly the "smaller first cut" alternative
  `requirements.md`'s Alternatives Considered section flags for Phase 3 to
  size — it already exists in-repo as a working, shipped pattern for a
  structurally identical problem (peer-live-synced status columns), so it's
  a legitimate normal-appetite fallback if drag-and-drop pricing exceeds
  Medium.
- Column-transition animation (`exitingItems`/`enteringIds` state, a
  `useLayoutEffect` diffing previous vs. current item status per render,
  `BacklogBoard.tsx:168-276`) is a real cost this board pattern already
  pays for smooth cross-column moves driven by *live* (not drag-initiated)
  updates — i.e., when a peer's action moves a card, this session's board
  animates the transition. `SessionBoard.tsx` will want the equivalent for
  peer-driven moves (see §6), and could likely reuse this logic almost
  verbatim (it's currently backlog-item-shaped but not overly coupled to
  backlog specifics beyond the `stageOf`/status types).

## 5. Peer sync: already solved at the data layer, no new mechanism needed

Sessions already have a live push mechanism: `SessionService.WatchSessions`
is a ConnectRPC server-stream RPC (`proto/session/v1/session.proto:29`),
consumed today by `useSessionService.ts` (`watchSessions`/`stopWatching`,
using `createWatchTransport` from `lib/transport/watch-ws-transport.ts`,
with the `BackoffState`/reconnect handling already built in). Every event
dispatches `upsertSession`/`removeSession` into the Redux `sessionsSlice`,
which is where `app/page.tsx`'s `sessions` array already comes from.

Since `SessionBoard.tsx` would receive the same `sessions` prop as
`SessionList.tsx` (§1), **if peer A drags a card and triggers `pauseSession`
→ `UpdateSession` RPC, peer B's board re-renders automatically** the moment
peer B's own `WatchSessions` stream delivers the resulting `SessionEvent` —
no board-specific subscription, websocket, or polling needs to be added.
This mirrors `BacklogBoard.tsx`'s own note at lines 146-150: "the board
subscribes to the same live stream/normalized store as the list page... a
status-change event moves an item's column membership purely by this filter
re-evaluating on the updated item, no board-specific refetch involved."

What *would* be new: the animated exit/enter transition (§4) needs a
`liveVersion`-style signal to distinguish "this session's column changed
because of a live/remote event" from "it changed because of a normal
render" — `sessionsSlice` should be checked for whether an equivalent
version counter already exists per-session (backlog items have
`liveVersion`; confirm whether `sessionsSlice` has an analog, or whether
event-vs-prop-diffing needs a new field) — flagged for Phase 3 planning,
not resolved here.

## 6. Optimistic vs. pessimistic UI

The codebase's existing pattern for session state changes is **pessimistic,
not optimistic**. `pauseSession`/`resumeSession` (`useSessionService.ts:364-
383`) both delegate to `updateSession()` (lines 301-330), which calls the
RPC, awaits the response, and only then dispatches `upsertSession(response.
session)` into the store (line 326) — there is no local/speculative store
write before the RPC resolves anywhere in this call path. `BacklogBoard.tsx`
follows the same philosophy but softens the wait with a `pending:
Record<string, string>` prop (itemId → in-flight action key,
`BacklogBoard.tsx:30,71,83,127`) that `BacklogItemCard` uses to show a
busy/disabled state on the specific card without moving it.

**Recommendation**: don't introduce optimistic card-move as a new pattern
for this feature. Follow the existing convention — drag-drop triggers the
RPC, the card shows a `pending` (busy) state in its *origin* column while
in flight (mirroring `BacklogBoard`'s `pending` prop), and only moves once
`WatchSessions`/the RPC response updates `sessionsSlice`. On RPC failure,
nothing needs to "snap back" because nothing moved yet — surface an error
toast (existing session-action error-handling pattern, per
`requirements.md`'s Observability Requirements) and clear the pending flag.
This sidesteps the Rabbit Holes section's "revert on failure" complexity
entirely by never speculatively moving the card, at the cost of a
perceptible RPC round-trip delay before the card visibly moves — acceptable
given `SessionService` calls are already local/fast (no remote network
hop; this is a local dashboard per the Security Classification note).

## 7. Recommended component split

Given `SessionList.tsx` is already 1601 lines and a known hotspot
(`requirements.md`'s Feasibility Risks), and per its own explicit steer
("default to a sibling `SessionBoard.tsx` component sharing hooks/state via
extraction, not inline branching"):

**Recommended: sibling `SessionBoard.tsx` + extracted shared hook**, not a
combined `BoardOrListContainer`. Concretely:

- `app/page.tsx` (or a thin new wrapper) owns the `"list" | "board"` mode
  state (§3) and conditionally renders `<SessionList>` or `<SessionBoard>`
  — a simple ternary at the call site, not a new container component. This
  matches how `viewMode="card"|"row"` is already just a prop-driven branch
  inside `SessionList`, and avoids a container whose only job is an
  if/else (an unjustified wrapper per
  `.claude/rules/interface-pollution-checklist.md`'s "forwarding-only
  wrapper" smell).
- Extract a shared hook — e.g. `useSessionListState()` — that owns the
  state and logic genuinely common to both views: search query, the
  `reviewItemBySessionId`/`approvals` "needs review" derivation (§1),
  `groupSessions()`/`GroupingStrategy` state (`lib/grouping/strategies.ts:69
  `, already a pure function taking `sessions` + `strategy` + returning
  `GroupedSessions[]` — directly reusable for computing board columns when
  swimlane axis is switched to a `GroupingStrategy` per Scope), and bulk
  selection. `SessionList.tsx` should be refactored to consume this hook
  too (not just `SessionBoard.tsx`), which is what actually shrinks the
  1601-line hotspot rather than just avoiding growing it further.
- `SessionBoard.tsx` itself stays focused on: column layout, drag-and-drop
  wiring, per-column virtualization (react-virtuoso already a dependency,
  per Non-functional Requirements), and the column-transition animation
  adapted from `BacklogBoard.tsx` (§4).
- Card rendering: `SessionCard`/`SessionRow` (`components/sessions/
  SessionCard.tsx`, `SessionRow.tsx`) already exist and are what
  `SessionList.tsx` renders per-session in card/row mode respectively —
  `SessionBoard.tsx` should reuse `SessionCard` inside each column rather
  than building a third card component, consistent with `BacklogBoard.tsx`
  reusing `BacklogItemCard` rather than inventing a board-specific card.

## 8. Feasibility confirmation: no drag-and-drop library present

Checked `web-app/package.json` for `@dnd-kit/*`, `react-dnd`,
`react-beautiful-dnd`, `sortablejs`, `@atlaskit/*` — none present.
Existing relevant deps: `react-virtuoso` (list/board virtualization),
`@tanstack/react-virtual`, `react-arborist` (tree component, has its own
internal drag-to-reorder for tree nodes but is not a general-purpose DnD
library and isn't a fit here). This confirms `requirements.md`'s Feasibility
Risks note — introducing a DnD library is a real new-dependency decision,
which `requirements.md` correctly defers to Phase 2 build-vs-buy research
outside this document's scope (architecture, not library selection).

## Summary of key files

| Concern | File |
|---|---|
| List view + all session-action props | `web-app/src/components/sessions/SessionList.tsx` |
| Session-control RPCs (pause/resume/update/archive/delete) | `web-app/src/lib/hooks/useSessionService.ts` |
| Per-session action adapter | `web-app/src/lib/hooks/useSessionActions.ts` |
| Grouping strategies (reusable for board swimlane axis) | `web-app/src/lib/grouping/strategies.ts` |
| Existing read-only kanban precedent | `web-app/src/components/backlog/BacklogBoard.tsx` |
| Live backlog-board sync hook (structural analog for §5/§6) | `web-app/src/lib/hooks/useWatchBacklogItems.ts` |
| Review-queue / approvals context (needs-review derivation) | `web-app/src/lib/contexts/ReviewQueueContext.tsx`, `web-app/src/lib/contexts/ApprovalsContext.tsx` |
| Proto RPC surface (confirms no `StopSession`) | `proto/session/v1/session.proto` |
| Dashboard page wiring `SessionList` today | `web-app/src/app/page.tsx` |
