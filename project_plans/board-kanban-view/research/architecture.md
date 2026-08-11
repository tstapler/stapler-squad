# Architecture Research: Board/Kanban View

Agent 3 (Architecture) — SDD research phase for `board-kanban-view`.

## 1. Current state management in SessionList.tsx / SessionRow.tsx

**SessionList is a "dumb-ish" presentational component, not a data hook.** It receives
`sessions: Session[]` and ~20 mutation callback props (`onPauseSession`, `onResumeSession`,
`onDeleteSession`, `onHibernateSession`, etc. — [SessionList.tsx:60-90](web-app/src/components/sessions/SessionList.tsx#L60-L90))
from its parent. It does **not** call `useSessionService()` itself. The actual RPC-calling
hook lives one level up: `web-app/src/components/pane/PaneSplitRenderer.tsx:170-174` wires
an `actions` object (backed by `useSessionService()`) into `SessionList`'s props — e.g.
`onPauseSession={actions.onPauseSession}`. The main dashboard (`web-app/src/app/page.tsx`)
follows the same shape.

All *view* state — search, status/category/tag filters, grouping strategy, sort
field/dir, visible columns, selection set, collapsed groups — is local `useState` inside
`SessionList.tsx` itself ([SessionList.tsx:317-378](web-app/src/components/sessions/SessionList.tsx#L317-L378)),
persisted to `localStorage` via a `STORAGE_KEYS` map
([SessionList.tsx:223-243](web-app/src/components/sessions/SessionList.tsx#L223-L243)).
There is **no shared `useSessions()` hook and no context that owns filter/grouping/selection
state** — it is 100% component-local. `storageKeyPrefix` is the only externalized knob, and
it is used today for **pane-scoping** (`pane-${pane.id}.` in
[PaneSplitRenderer.tsx:192](web-app/src/components/pane/PaneSplitRenderer.tsx#L192)), not
workspace-scoping — see §4 for why this matters for the "persist per workspace" requirement.

**Grouping** (`GroupingStrategy` enum: Category, Tag, Branch, Path, Program, Status,
SessionType, Project, Workflow, None — `web-app/src/lib/grouping/strategies.ts:7-18`) is a
pure function, `groupSessions(sessions, strategy, options)`
([strategies.ts:69](web-app/src/lib/grouping/strategies.ts#L69)), with no framework/component
coupling. It already supports `GroupingStrategy.Status` (single-membership, using
`getStatusDisplayName()`, [strategies.ts:209-221](web-app/src/lib/grouping/strategies.ts#L209-L221))
and orders status groups via a hardcoded `statusOrder` map
([strategies.ts:179-192](web-app/src/lib/grouping/strategies.ts#L179-L192)) — this is
directly reusable as the board's status-column ordering, and reusable unmodified as the
swimlane-axis mechanism for goal #4 ("use the existing grouping-strategy axis as the
swimlane axis instead of status").

**There is already a precedent for two render modes sharing one filter/selection pipeline**:
`SessionList` has a `viewMode?: "card" | "row"` prop
([SessionList.tsx:96](web-app/src/components/sessions/SessionList.tsx#L96), default `"row"`).
Both modes run through the *same* `filteredSessions` → `sortedSessions` → `groupedSessions`
pipeline; only the final render branches (`flatItems` for row mode vs. `cardFlatSessions`
for card mode, [SessionList.tsx:657-708](web-app/src/components/sessions/SessionList.tsx#L657-L708))
and only row mode is virtualized (`react-virtuoso` `GroupedVirtuoso` +
`@tanstack/react-virtual`). In practice `viewMode="card"` is currently exercised only in
tests (`SessionList.collapse.test.tsx`), not wired to any live UI toggle — it is a real but
dormant precedent, not a shipped feature.

**Search/bulk-select** state (`selectMode`, `selectedSessions: Set<string>`,
`handleToggleSession`, `handleSelectAll`, `computeRangeIds` for shift-click ranges) is also
local to `SessionList.tsx` ([SessionList.tsx:360-770](web-app/src/components/sessions/SessionList.tsx#L360-L770)).
`BulkActions.tsx` itself is view-agnostic and reusable as-is: it takes `selectedCount`,
`totalCount`, and a flat set of action callbacks (`onPauseAll`, `onResumeAll`, `onDeleteAll`,
`onAddTagAll`, `onSelectAll`, `onClearSelection`, `onGroupAs`) —
[BulkActions.tsx:8-20](web-app/src/components/sessions/BulkActions.tsx#L8-L20) — with zero
knowledge of rows/columns/grouping. It can be dropped into a board layout unmodified.

**Session status detection** (terminal-scraped status, distinct from the backend enum) comes
from Redux (`useAppSelector(selectDetectedStatusMap)`,
[SessionList.tsx:311](web-app/src/components/sessions/SessionList.tsx#L311)) — this is the
one piece of cross-cutting state that *is* centralized (Redux), and Board will need the same
selector for card status badges.

## 2. Session status mutation path — RPC and backend validation

There is **no separate `PauseSession`/`ResumeSession`/`StopSession` RPC**. `proto/session/v1/session.proto`
defines a single `UpdateSession(UpdateSessionRequest) returns (UpdateSessionResponse)`
([session.proto:22](proto/session/v1/session.proto#L22)), and `UpdateSessionRequest` carries
an `optional SessionStatus status = 2` field plus `pause_reason`
([session.proto:579-617](proto/session/v1/session.proto#L579-L617)).
`web-app/src/lib/hooks/useSessionService.ts` exposes `pauseSession`/`resumeSession` as thin
wrappers around `updateSession(id, { status: ... })`
([useSessionService.ts:364-382](web-app/src/lib/hooks/useSessionService.ts#L364-L382)).

**Server-side, `SessionService.UpdateSession` only wires the Paused↔non-Paused edge of the
status field** ([session_service.go:1854-1879](server/services/session_service.go#L1854-L1879)):
it calls `instance.Pause()` when the target is `Paused` and the current status isn't, and
`instance.Resume()` when leaving `Paused`. **It does not accept `Stopped`, `Hibernated`, or
`Active` (from anything other than Paused) as a target via this RPC today.** Hibernation has
its own dedicated RPC (`HibernateSession`/`ResumeHibernatedSession`,
[session_service.go:1913-1961](server/services/session_service.go#L1913-L1961)); there is no
RPC that transitions a live session straight to `Stopped` by user action — `Stopped` is
currently reached either as a `Creating→Stopped` failure path or via internal transitions
(tmux exit detection, actor-driven), not a user-facing "stop" button/RPC. `DeleteSession`
([session_service.go:2017](server/services/session_service.go#L2017)) fully **destroys**
the session (worktree cleanup, tmux kill) rather than marking it `Stopped` — it is not a
softer "stop" primitive.

**The backend already has a formal, exhaustive state machine that enforces valid
transitions** — this answers the requirements doc's open question definitively. See
`session/state_machine.go:5-72`:

```
Creating   --> Active, Stopped
Active     --> Paused, Stopped, Hibernated
Paused     --> Active, Stopped
Stopped    --> Active
Hibernated --> Active, Stopped
```

`CanTransition(from, to Status) bool` ([state_machine.go:69-72](session/state_machine.go#L69-L72))
is already exported, and every transition attempt (`Instance.Pause()`, `.Resume()`, actor
`transitionToLocked`, etc.) returns `session.ErrInvalidTransition{From, To}`
([session/types.go:271-279](session/types.go#L271-L279)) on an illegal edge — which
`server/services/session_service.go:1690-1699`'s `classifyPauseResumeErr` already converts to
a `connect.Code`. **No new validation logic is needed for the edges the state machine already
covers** (e.g. Paused→Active, Active→Paused). What *is* missing is **RPC surface area**: the
`UpdateSession` handler needs a new branch to accept `Active/Paused/Hibernated → Stopped` as a
user-triggered drag target if the "Complete" column is meant to be a valid drop zone, not just
a display bucket for sessions that reached `Stopped` on their own. This is a real, scoped gap
for the planning phase to size — it is "wire an existing state-machine edge through a new
`UpdateSession` branch," not "invent new transition-validation logic."

**"Needs Review" is not a `SessionStatus` value at all** — it is a derived condition,
confirming the requirements doc's hypothesis. `SubStatus.NEEDS_APPROVAL` (=3) is a sub-state
layered on top of `Active`
(`web-app/src/gen/session/v1/types_pb.ts`, consumed in
[SessionCard.tsx:536-546](web-app/src/components/sessions/SessionCard.tsx#L536-L546)), and
`SessionList.tsx` already computes a `reviewItemBySessionId` map from
`useReviewQueueContext()` ([SessionList.tsx:304-308](web-app/src/components/sessions/SessionList.tsx#L304-L308)).
Board's "Needs Review" column should filter on `session.subStatus === SubStatus.NEEDS_APPROVAL`
(or membership in the review-queue-context set) **within** the `Active` bucket, not on a
separate backend status — meaning "drag a Needs-Review card back to Running" is not really a
status transition at all (both are `Active`); it would need to resolve/dismiss the approval
instead (a different existing RPC — `ResolveApproval` — not `UpdateSession`). Flag this for
planning: the "Needs Review" column has a different drag-semantics shape than the other three
columns.

**No drag-and-drop library exists in the repo today.** `web-app/package.json` has no
`@dnd-kit/*`, `react-beautiful-dnd`, `react-dnd`, or similar — confirmed via grep, zero hits.
This is a real dependency decision for planning (ladder rung 4/6 per the requirements doc),
not something this architecture research resolves.

## 3. Component decomposition recommendation

**Recommendation: `SessionBoard.tsx` as a new sibling component, not a third `viewMode` value
bolted onto `SessionList.tsx`.** Reasons:

- `SessionList.tsx` is already ~1600 lines. The `viewMode="card"` precedent shows the
  "one component, branch at render time" pattern still shares filter/sort/group state
  cleanly up to the point of rendering, but board view adds an orthogonal, heavier set of
  concerns — DnD sensors/handlers, per-column virtualization, swimlane-vs-status column
  derivation, invalid-drop rejection/animation — that would push a shared component well past
  a reasonable size and mix concerns that have no overlap in *rendering* (rows/cards in a
  scroll list vs. cards in draggable columns), only in *data*.
- Per `.claude/rules/interface-pollution-checklist.md`, forking a new component only to
  re-derive filtering/sorting logic that already lives in `SessionList.tsx` would itself be a
  problem (duplicated business logic, not just duplicated JSX). The fix is **extracting the
  data pipeline, not the whole component**: pull `filteredSessions` → `sortedSessions` →
  `groupedSessions` (currently `SessionList.tsx:530-656`) into a plain function or a small
  hook (e.g. `useFilteredGroupedSessions(sessions, filterState, groupingStrategy)`) that both
  `SessionList` and `SessionBoard` call. This is a concrete-type extraction (a function with a
  narrow signature), not a speculative interface — it has two real, immediate call sites, so
  it clears the "second real implementation" bar the checklist asks for.
- **Do not introduce a `ViewModeProvider`/`BoardManager`/generic "SessionViewController"
  interface.** There is exactly one thing board and list share (the filter/group pipeline,
  search string, and selection `Set<string>`) and exactly two consumers. A plain hook
  returning `{ filtered, grouped, selection, toggleSelection, ... }` covers it; an interface
  or a forwarding-wrapper class would be pure speculation with a single implementation.

**Proposed decomposition**, following the existing file-per-concern pattern in
`web-app/src/components/sessions/`:

- `SessionBoard.tsx` — top-level board component. Owns column derivation (status- or
  grouping-strategy-based, via the same `groupSessions()` from `strategies.ts`), DnD context
  setup, and renders one `BoardColumn` per group.
- `BoardColumn.tsx` — a single column: header (label + count badge, satisfying acceptance
  criterion #3), drop-target wiring, and a scrollable card list. Column-level virtualization
  (if needed for large columns) is scoped here, not board-wide, since columns scroll
  independently (mobile requirement #10).
- **Reuse `SessionCard.tsx` for card rendering inside `BoardColumn`, do not fork a new
  `SessionBoardCard.tsx`.** `SessionCard` ([SessionCard.tsx:98](web-app/src/components/sessions/SessionCard.tsx#L98))
  is already the card-rendering component used by `viewMode="card"` and is prop-driven
  (session + callbacks), with no row/list-specific coupling. If board cards need
  column-specific chrome (e.g. a drag handle, a "move to..." fallback menu for touch —
  acceptance criterion #10), wrap `SessionCard` in a thin `BoardCard.tsx` that adds *only*
  that new behavior (drag handle + touch fallback menu), per the "wrapper must add real
  behavior at that layer" rule in the interface-pollution checklist — not a copy-paste fork of
  `SessionCard`'s ~900 lines.
- `BulkActions.tsx` is reused unmodified — it already takes counts/callbacks with no view
  coupling (§1).
- A shared `useFilteredGroupedSessions`-style hook (new, small) replaces duplicating
  `SessionList.tsx:530-656`'s filter/sort logic in `SessionBoard.tsx`.
- The List/Board toggle itself and the `b` keyboard shortcut belong in the dashboard header
  component that currently renders `SessionList` (`app/page.tsx` / `PaneSplitRenderer.tsx`),
  which already owns the `actions` object wiring — it would conditionally render
  `<SessionList>` or `<SessionBoard>` with the same `sessions` + `actions` props.

## 4. Persistence of last-used view mode (per workspace)

No existing pattern cleanly covers "per workspace" scoping. Two things confirmed:

- `storageKeyPrefix` (`SessionList.tsx:297`) is the only externalized localStorage-scoping
  knob today, and it is used for **pane** scoping (`pane-${pane.id}.` in
  `PaneSplitRenderer.tsx:192`), not workspace scoping. The main dashboard renders `SessionList`
  with no prefix at all, meaning today's filter/grouping/sort preferences are **global to the
  browser origin**, not per-workspace, despite living "inside" a component that is
  workspace-specific in practice.
- "Workspace" in this app is a `DatabaseInfo`/`workspaceId` concept surfaced via
  `useDatabases()` (`WorkspaceSwitcher.tsx:17,52`), and switching workspace **causes a full
  server restart + page reload** (per that file's own doc comment) — i.e. workspaces are not
  a client-side-only scoping concept, they're a different backend database context served
  from the same origin.

**Recommendation for planning, not resolved here**: extend the `storageKeyPrefix` mechanism
with a `ws-${currentId}.`-style prefix (mirroring the existing `pane-${pane.id}.` pattern) fed
from `useDatabases().currentId`, and add one new storage key (`VIEW_MODE: "list" | "board"`)
to the `BASE_STORAGE_KEYS` map in `SessionList.tsx`/wherever the toggle lives. This is
additive to an existing, working pattern rather than inventing a new persistence layer — but
sizing/placement (does the key live beside `SessionList`'s other keys, or move up to the
toggle-owning component?) is a planning decision, not an architecture one.

## Summary of what does NOT need new architecture

- Grouping-as-swimlane-axis: `groupSessions()` already supports this, no changes needed.
- Status-transition validation: the backend state machine (`session/state_machine.go`)
  already exhaustively validates every edge including Active↔Paused and Paused/Active→Stopped;
  no new validation logic needed for those edges.
- Bulk actions across columns: `BulkActions.tsx` is already view-agnostic.
- Search filtering across columns: the filter pipeline in `SessionList.tsx:530-589` is already
  a pure function of `sessions` + filter state; extracting it (see §3) makes it board-usable
  with no new filtering logic.

## Summary of real gaps for planning/implementation

1. `UpdateSession`'s Go handler needs a new branch to accept `→ Stopped` as a user-triggered
   target (currently only Paused↔non-Paused is wired), if "Complete" is a real drop target.
2. "Needs Review" has different drag semantics (resolving an approval via `ResolveApproval`,
   not an `UpdateSession` status change) since it's a `SubStatus` on top of `Active`, not a
   distinct `SessionStatus`.
3. No DnD library exists; one must be chosen (dependency decision, out of scope for this doc).
4. No per-workspace localStorage scoping pattern exists yet — closest precedent is
   pane-scoped `storageKeyPrefix`; extending it to workspace-scoping is additive but new.
5. Extract `SessionList.tsx:530-656`'s filter/sort/group pipeline into a shared function/hook
   before `SessionBoard.tsx` is built, to avoid duplicating business logic across the two view
   components.
