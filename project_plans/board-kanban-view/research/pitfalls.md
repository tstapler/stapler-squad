# Research: Pitfalls — Drag-and-Drop Kanban Backed by Live Server State

Agent 4 (Pitfalls), SDD research phase for `board-kanban-view`.

## 1. Optimistic update vs. server confirmation races

The backend already treats status transitions as fallible: `UpdateSession` in
[`server/services/session_service.go:1702`](/home/tstapler/Programming/stapler-squad/server/services/session_service.go#L1702)
returns `connect.CodeInvalidArgument` for bad input, and the comment at line 1863
(`// (permission denied, invalid transition).`) confirms transition legality is
checked server-side, not just client-side. This means acceptance criterion 5
("dragging to an invalid column is rejected, card returns to its original column
with a visible error indication") is a real code path, not a hypothetical — the
board must handle rejection, not just the happy path.

Known failure modes when combining optimistic drag-drop with async confirmation:

- **Snap-back double-move.** If the card is optimistically moved to column B, the
  RPC is rejected, and in the interim `watchSessions()` push (see §2) delivers an
  unrelated update for a *different* field on the same session, a naive
  "reconcile from server state" can flicker the card twice — once snapping back
  to column A (rejection), once re-rendering from the fresh watch payload. Fix:
  reconcile in a single state-update pass keyed by session ID, not by column
  membership; treat the rejection response and any concurrent watch push as one
  merge, not two independent re-renders.
- **Stale toast/undo affordance.** If the rejection arrives after the user has
  already navigated away from the board (e.g. filtered the search, switched to
  list view) the "snapped back" error toast should still be actionable/dismissible
  without requiring the board to still be mounted — otherwise it either throws
  (setState on unmounted component) or silently vanishes with no user-visible
  explanation of why their drag didn't stick.
- **Distinguish "rejected by server" from "changed again by someone else."** A
  card that moves back to its original column could be either (a) an invalid
  transition rejection or (b) a workspace peer or the session's own auto-transition
  (e.g. Active → Complete) racing the same field. These need visually distinct
  treatment — (a) is "your action failed," (b) is "someone/something else changed
  this out from under you" — collapsing them into one generic error message will
  read as a bug ("I dragged it and it just... went back?") even when the system
  behaved correctly.
- **Idempotency / duplicate mutation on retry.** If the optimistic-UI layer
  retries a timed-out `UpdateSession` call automatically, ensure the retry doesn't
  fire a second real transition after the first one actually succeeded server-side
  (classic dropped-response-not-dropped-request race). Check whether
  `UpdateSession` is idempotent for repeated identical status transitions before
  wiring any automatic retry into the drag handler.

## 2. Real-time push racing an in-progress drag

Confirmed: this app has a live push mechanism, not just polling. The
`GlobalSessionServiceProvider` in
[`web-app/src/lib/contexts/SessionServiceContext.tsx:48-81`](/home/tstapler/Programming/stapler-squad/web-app/src/lib/contexts/SessionServiceContext.tsx#L48)
exposes `watchSessions()` / `stopWatching()` with `autoWatch: true`, backed by a
WebSocket-based ConnectRPC streaming transport
(`web-app/src/lib/transport/watch-ws-transport.ts`) with its own reconnect/backoff
handling (`web-app/src/lib/utils/backoff.ts`). This is the same stream the list
view presumably already consumes — the board view must consume the *same* single
global watch connection (not open a second one) per the existing
`GlobalSessionServiceProvider` singleton pattern.

Pitfalls specific to this stack:

- **Mid-drag repositioning.** If a `watchSessions` push updates the dragged
  session's status while the user's pointer is still down (e.g. the session
  auto-transitions Active → Complete server-side mid-drag, independent of the
  user's own drag target), a naive re-render keyed on "column = f(status)" will
  rip the card out from under the user's cursor — cancelling or corrupting most
  DnD libraries' internal drag state (HTML5 native DnD is especially fragile
  here: removing the dragged DOM node during a drag silently ends the drag with
  no `dragend`/`drop` firing in some browsers). **Mitigation pattern**: freeze the
  dragged card's rendered column position for the duration of the drag gesture
  (track "session ID currently being dragged" in local UI state and suppress
  server-driven column reassignment for that ID until drop/cancel), then
  reconcile against the latest known server state once the drag ends.
- **Push arriving for a card the user just dropped, before the RPC response.**
  Since `watchSessions` is a separate stream from the `UpdateSession` RPC's own
  response, the push for the user's own change could arrive *before* the RPC
  response resolves (or vice versa). The board needs one canonical merge order
  (e.g. "watch push always wins for status, but optimistic move suppresses stale
  watch pushes older than the drop timestamp") — otherwise the card can flicker
  between old and new column while both signals race to update the same piece of
  state.
- **Reconnect gaps.** `watch-ws-transport.ts`'s reconnect logic (backoff.ts) means
  there's a window after a dropped WebSocket where the client has no live view of
  server state. If a drag happens during that window, the optimistic update has no
  "ground truth" push to confirm/deny against until reconnect — decide whether to
  block dragging while `ConnectionIndicator` (`web-app/src/components/layout/ConnectionIndicator.tsx`)
  shows disconnected, similar to how other write actions might already guard on
  connection state.

## 3. Accessibility pitfalls of drag-and-drop

No existing DnD library is in `web-app/package.json` (confirmed by `grep` —
zero matches for dnd/drag/sortable), so this will be a **new** accessibility
surface for the app, not an extension of an existing one.

- **Native HTML5 Drag and Drop API is not keyboard-accessible at all.**
  `draggable="true"` + `dragstart`/`dragover`/`drop` events only fire from mouse
  (and inconsistently, touch) input. There is no keyboard equivalent baked into
  the spec — a keyboard-only user simply cannot move a card between columns
  unless the app author builds a fully separate keyboard interaction path. This
  is the single most common kanban-board accessibility bug reported against
  hand-rolled HTML5 DnD implementations.
- **Screen readers get no signal from a native drag.** `dragstart`/`dragover`
  don't reliably fire accessibility-tree events; a screen reader user has no
  indication a drag is in progress, which column is currently the drop target,
  or that a drop succeeded/failed. Needs `aria-live` region announcements at
  minimum ("Session moved to Needs Review column") — most naive implementations
  skip this entirely.
- **Library choice materially changes the accessibility floor.** `@dnd-kit`
  ships a `KeyboardSensor` out of the box (Space/Enter to pick up, arrow keys to
  move between droppable zones, Space/Enter to drop, Escape to cancel) plus hooks
  for wiring `aria-live` announcements (`announcements` option on `DndContext`).
  `react-beautiful-dnd` also has keyboard support but the project is
  **unmaintained/archived by Atlassian** (superseded by their own
  `@atlaskit/pragmatic-drag-and-drop`) — a real risk if it's selected: no future
  bug fixes for browser regressions, and it has known React 18 concurrent-mode
  compatibility issues reported upstream. Since acceptance criterion 10 already
  requires a non-drag "move to..." fallback control for touch devices, that same
  fallback control (a menu/select on each card) can double as the keyboard-only
  and screen-reader-only path — cheaper than building full DnD-library keyboard
  wiring, and directly satisfies WCAG 2.1.1 (keyboard operable) without depending
  on a specific library's sensor implementation.
- **Focus management after drop.** After a card moves columns (drag or keyboard),
  focus must be programmatically moved to (or kept on) the card in its new
  location — losing focus back to `<body>` on drop is a common regression that
  breaks keyboard flow for the next action.

## 4. Mobile/touch drag pitfalls

Per the mobile+desktop UX requirement (`feedback_mobile_desktop_ux` memory) and
AC 10 (viable non-drag fallback on touch), this app already has prior art for
the exact underlying conflict — touch-drag vs. scroll-gesture — in
[`web-app/src/lib/hooks/useTouchScroll.ts`](/home/tstapler/Programming/stapler-squad/web-app/src/lib/hooks/useTouchScroll.ts)
and `useSwipe.ts`, both of which explicitly manage `{ passive: false }` +
conditional `preventDefault()` to arbitrate which gesture wins. That pattern
(explicit passive/preventDefault arbitration, not "just add a DnD library and
hope") should be reused/consulted when wiring touch sensors for the board.

- **Touch-hold-to-drag vs. vertical column scroll.** A card's touch-drag start
  and the column's own vertical scroll both begin with a `touchstart`/`touchmove`
  on the same element. Without an explicit long-press/hold threshold before
  drag-mode engages, every attempt to scroll a column on mobile will instead
  drag the first card the finger lands on. `@dnd-kit`'s `TouchSensor` supports a
  configurable `delay` + `tolerance` (hold N ms before drag starts, cancel if the
  finger moves more than N px in that window) specifically for this — a bare
  HTML5 DnD or a naive touch handler will not have this by default and needs it
  hand-rolled.
- **Horizontal column scroll vs. horizontal card drag.** If columns are
  horizontally scrollable on mobile (per AC 10's "independently horizontally
  scrollable" option), a card drag that also allows horizontal movement between
  columns competes with the container's own horizontal scroll gesture on the
  same axis — this is a strictly harder conflict than the vertical case above
  and has no clean threshold-based fix; most kanban mobile implementations
  resolve it by making cross-column movement on mobile *only* available via the
  non-drag "move to..." action, and disabling true cross-column drag-scroll on
  touch entirely.
- **`touch-action: none` requirement.** Any element that becomes a drag handle
  needs `touch-action: none` (or `pan-y`/`pan-x` as appropriate) in CSS, or the
  browser's own default touch scrolling will fight the JS-driven drag on some
  browsers even with `preventDefault()` called in the handler — an easy-to-miss
  CSS-only fix that's independent of which DnD library is chosen.

## 5. Performance pitfalls with many cards/columns

`SessionList.tsx` is already ~1600 lines
([`web-app/src/components/sessions/SessionList.tsx`](/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionList.tsx)),
which is a proxy signal that real workspaces run enough concurrent/historical
sessions for list-view complexity to matter — the board view should assume the
same order of magnitude, not a toy dataset.

- **DOM node count scales with cards × columns, not cards.** A flat list of N
  sessions becomes N DOM subtrees regardless of view, but a kanban board also
  re-renders column containers, drop-zone highlight state, and per-card drag
  listeners — if every card mounts its own `useSortable`/`useDraggable` hook
  instance (typical dnd-kit usage), N sessions means N sets of active listeners
  and measured layout rects, which is more render/measure work than the
  equivalent flat list. If a workspace has hundreds of sessions across a fine
  status/grouping split, watch for jank on `dragstart` (dnd-kit measures every
  droppable's bounding rect at drag start by default — an O(columns) but
  frequently O(cards)-adjacent cost depending on configuration).
- **Reflow on every `dragover`.** Auto-scroll-while-dragging and reordering
  within a column both trigger layout reflow on `dragover`; without
  virtualization, dragging over a column with hundreds of visible cards can
  visibly stutter. If the list view doesn't already virtualize (worth
  confirming in the stack-research phase), the board view will hit this ceiling
  sooner because it renders N cards *per column* to see, whereas list view can
  paginate/collapse more aggressively.
- **Reconciliation cost of live pushes against a large board.** Every
  `watchSessions` push that changes any session's status has to be diffed
  against which column/swimlane it currently renders in — a linear "recompute
  grouping for entire session set on every push" implementation (as opposed to
  patching just the changed session's membership) will get measurably slower as
  session count grows, independent of the drag/DnD library chosen. This is the
  same class of cost the existing `web-app/src/lib/grouping/strategies.ts`
  grouping computation already has to pay for list view's grouped mode — check
  whether it already memoizes per-session-change deltas (it should be revisited
  in planning if it currently regroups the entire array on every state update
  and the board reuses it for swimlanes, since the board makes that recompute
  fire on every live push in addition to every user action).

## 6. State-persistence pitfalls (per-workspace last-used view)

**No existing pattern for workspace-scoped localStorage keys was found.** Every
current UI-preference persistence example in `web-app/src/lib` uses a single
global, unscoped key:

- `web-app/src/lib/contexts/NavigationContext.tsx:13` — `const STORAGE_KEY = "nav-drawer-open"`
- `web-app/src/lib/hooks/useListColumnWidth.ts` — `const STORAGE_KEY = "cockpit.listColumnWidth"`

Neither embeds a workspace/database identifier in the key. A `grep` for any
`localStorage` key that interpolates a workspace ID (`workspaceId`,
`currentWorkspace`, template-literal key construction) across
`web-app/src/lib` and `web-app/src/components` returned **zero matches**. The
only workspace-adjacent concept found is the workspace/database switcher hook
(`web-app/src/lib/hooks/useDatabase.ts`), which is about which backend dataset
is active, not about scoping browser storage keys to it.

Implication for AC 9 ("last-used view mode persists **per workspace**"): this
is not a "reuse the existing per-workspace storage pattern" task — **there is no
existing pattern to reuse**, and requirements.md's own open-questions section
assumed one might exist. Planning needs to actually design this, and get it
right the first time, because the collision failure mode is silent and easy to
miss in testing:

- **Cross-workspace key collision.** If the implementation naively does
  `localStorage.setItem("viewMode", "board")` (following the existing
  unscoped-key convention seen elsewhere in this codebase), switching workspaces
  will not reset or correctly scope the view preference — every workspace will
  share one global "last used view," silently violating AC 9 the first time a
  user with 2+ workspaces sets them to different views. This is exactly the kind
  of bug that passes single-workspace manual testing and only surfaces for
  multi-workspace users.
  the current workspace/database identifier (from `useDatabase.ts` or wherever
  the active workspace ID is threaded through context) into the storage key,
  e.g. `` `cockpit.viewMode.${workspaceId}` ``, following the naming convention
  (`cockpit.*`) already used by `useListColumnWidth.ts`.

## 7. Stack-specific risks (dnd-kit / react-beautiful-dnd general knowledge)

Since Agent 1's stack-evaluation findings weren't available at research time,
noting known issues for the two most likely candidates from general knowledge,
for planning to weigh:

- **`react-beautiful-dnd`**: officially archived by Atlassian (superseded by
  `@atlaskit/pragmatic-drag-and-drop`); known incompatibilities with React 18
  Strict Mode double-invoked effects and concurrent rendering; virtualization
  support (`react-window`/`react-virtualized` integration) is a documented pain
  point requiring workarounds, not first-class support. Given this repo's likely
  React 18+ baseline, this is a meaningful adoption risk independent of feature
  fit.
- **`@dnd-kit`**: actively maintained, has built-in keyboard sensor + screen
  reader announcement hooks (addresses §3 directly), and composes reasonably
  with virtualization (each sortable item needs a stable ID mapping, but there's
  no structural incompatibility). Known rough edges: multi-container (cross-column)
  drag with dynamic list virtualization requires careful `id`↔`index` bookkeeping
  since `dnd-kit` computes collision detection from rendered/measured DOM rects —
  if the board virtualizes off-screen cards (per §5), collision detection against
  columns whose cards aren't currently mounted needs explicit handling (can't
  drop "onto" a card that isn't in the DOM).
- **Hand-rolled HTML5 DnD** (no library): cheapest dependency-wise but inherits
  every pitfall in §3 (no keyboard path at all) and has well-documented
  Safari/Firefox inconsistencies around custom drag images and `dataTransfer`
  behavior on touch-hybrid devices. Given AC 10 already mandates a non-drag
  fallback control regardless of library choice, the marginal accessibility
  benefit of a library (vs. leaning entirely on the fallback control for
  keyboard/mobile) is a real trade-off planning should make explicitly rather
  than defaulting to "no new dependency" by inertia.

## Summary of load-bearing findings for planning

1. "Needs Review" is **not** a first-class `SessionStatus` value — proto
   (`proto/session/v1/types.proto:340-341`) shows `SESSION_STATUS_NEEDS_APPROVAL`
   is deprecated; the comment states "NeedsApproval is now a sub-status."
   Dragging a card *into* the Needs Review column has no single corresponding
   status to set — this column is populated by a derived condition, and a drag
   *out* of it must decide what status to actually mutate (planning must resolve
   this before touching drag-drop code, not defer it to implementation).
2. The app already has one global live-push connection
   (`watchSessions`/`GlobalSessionServiceProvider`) the board must share, not
   duplicate — and it must freeze the dragged card's column during an
   in-progress drag gesture to avoid the live push yanking cards mid-drag.
3. There is no existing per-workspace localStorage scoping pattern anywhere in
   the codebase — AC 9 needs a new, workspace-ID-keyed storage key, not a
   "reuse what's there" approach.
