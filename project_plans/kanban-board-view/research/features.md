# Research: Feature Landscape — kanban-board-view

## 1. What state/logic SessionList.tsx already owns (share vs. duplicate)

`web-app/src/components/sessions/SessionList.tsx` (1601 lines) owns, via `useState` + a
`localStorage` persistence layer (`STORAGE_KEYS` built from `makeStorageKeys(storageKeyPrefix)`,
`SessionList.tsx:223,302`):

- **Search**: `searchQuery` (`SessionList.tsx:317`), applied as a substring filter inside the big
  `useMemo` filter pipeline (`SessionList.tsx:536-585`).
- **Filters**: `selectedStatus`, `selectedCategory`, `selectedTag`, `hidePaused`, `showArchived`,
  `filterNeedsApproval` (`SessionList.tsx:318-338`) — note `filterNeedsApproval` already exists as
  a *filter* toggle, separate from any true derived-approval state (see §3).
- **Grouping**: `groupingStrategy` (`SessionList.tsx:340`), persisted, applied via
  `groupSessions(sortedSessions, groupingStrategy)` (`SessionList.tsx:649-650`) from
  `web-app/src/lib/grouping/strategies.ts`. `collapsedGroups` tracks per-group collapse state
  (`SessionList.tsx:345`).
- **Bulk select**: `selectMode`, `selectedSessions: Set<string>`, `activeSelection` (intersection
  of `selectedSessions` with currently-filtered session IDs, `SessionList.tsx:632-641`),
  `bulkFeedback`. Rendered via `<BulkActions />` (`web-app/src/components/sessions/BulkActions.tsx`),
  a **dumb prop-driven component**: `selectedCount`, `onPauseAll`, `onResumeAll`, `onDeleteAll`,
  `onAddTagAll`, `onSelectAll`, `onClearSelection`, `totalCount`, `feedback`, `onGroupAs?`
  (`BulkActions.tsx:9-20`). It has no internal session-fetching logic, so it is directly reusable
  by a board view as long as the board computes the same `selectedCount`/`activeSelection` inputs.
- **Sort**: `sortField`/`sortDir` (list-view only concept, would not carry over 1:1 to a
  swimlane board — a board's ordering is more naturally per-column, likely sortable independently
  or reusing the same field).
- **View mode**: existing `viewMode?: "card" | "row"` prop (`SessionList.tsx:96-97`) is *not* a
  board — it toggles card-vs-row *within* the current list, driving two different virtualization
  paths (`react-virtuoso` `GroupedVirtuoso` for `row`, `@tanstack/react-virtual`'s `useVirtualizer`
  for `card`, both gated by `viewMode` at `SessionList.tsx:658,681,698-708`). A new "board" mode
  is a third, orthogonal axis — do not overload the existing `viewMode` prop; the requirements'
  "List / Board" toggle is a layer above `card`/`row`, not a third value squeezed into the same enum
  (or if it is a third value, `SessionList.tsx`'s virtualization branches need a matching third
  branch that renders a wholly different tree, which argues for keeping board rendering in a
  sibling component per the plan's stated risk).
- **State-change actions**: SessionList does **not** call `useSessionService` RPCs directly for
  pause/resume/stop — it receives them as callback props (`onPauseSession`, `onResumeSession`,
  `onDirectResumeSession`, wired from `web-app/src/components/pane/PaneSplitRenderer.tsx:172-174`
  from an `actions` object). `handlePauseSelected`/`handleResumeSelected` (`SessionList.tsx:817-845`)
  are thin loops over `activeSelection` calling those props. **Note `handleStopSelected =
  handlePauseSelected` (`SessionList.tsx:845`)** — there is no separate bulk "stop" action wired
  today; "stop" as a board column-drop target needs its own prop/RPC path, it cannot reuse the
  existing bulk-pause alias.

**Sharing implication**: a `SessionBoard.tsx` sibling component (per the plan's own risk note)
should accept the *same* filtered/sorted session array and the *same* action-callback props
SessionList already receives from its parent, plus the same `selectMode`/`selectedSessions`
lifted into a shared hook (e.g. extract `useSessionListState` covering search/filter/selection out
of `SessionList.tsx` so both `SessionList` and `SessionBoard` consume one hook) rather than each
component keeping parallel `useState` calls that can drift. This is the direct implementation of
the requirement "Existing list-view capabilities are not degraded or duplicated with divergent
behavior when board view ships."

## 2. `groupSessions()` / `GroupingStrategy` — reuse for swimlane axis

`web-app/src/lib/grouping/strategies.ts` (249 lines, read in full):

- `GroupingStrategy` enum: `Category | Tag | Branch | Path | Program | Status | SessionType |
  Project | Workflow | None` (`strategies.ts:7-18`).
- `groupSessions(sessions, strategy, options?)` returns `GroupedSessions[]` — `{ groupKey,
  displayName, sessions }[]`, already sorted with a **status-specific column ordering** baked in
  (`statusOrder` map at `strategies.ts:179-192`: Active → Ready → Loading/Creating → Needs
  Approval → Paused → Stopped → Hibernated → Unknown). This is a near-exact precedent for board
  column ordering when swimlane axis = Status.
- **Multi-membership**: `Tag` strategy puts a session in every one of its tag groups
  simultaneously (`strategies.ts:101-110`) — the same session object is pushed into multiple
  arrays, not cloned. A board view using Tag as its swimlane axis must decide whether a
  drag-and-drop from a multi-membership column still makes sense (dragging a session out of one
  tag column doesn't remove the tag from the other column it's also in) — this is a real UX
  ambiguity `Tag`-axis boards inherit for free, since `groupSessions` was written for a flat/
  grouped *list*, not a board where card position implies exclusive state.
- The function is a **pure function over an array**, no React state — trivially reusable by a
  board component via the same `useMemo` pattern SessionList already uses
  (`SessionList.tsx:649-650`).
- `getStatusDisplayName()` (`strategies.ts:209-221`) is the canonical status→label mapping,
  already used for `Status` grouping; the board's default 4-column view is a *different*,
  hand-picked derived-state mapping (see §3), not simply `groupSessions(sessions,
  GroupingStrategy.Status)` — that would produce 8 status-derived groups (Active, Ready, Loading,
  Paused, Needs Approval, Creating, Stopped, Hibernated), not the requirements' 4 swimlanes.

## 3. `SessionStatus` enum + how "Needs Review" is actually derived today

`proto/session/v1/types.proto:320-350` (`SessionStatus` enum, read in full):

```
SESSION_STATUS_UNSPECIFIED = 0
SESSION_STATUS_ACTIVE = 1        (replaces legacy RUNNING(1)/READY(2))
SESSION_STATUS_RUNNING = 1       [deprecated, alias of ACTIVE]
SESSION_STATUS_READY = 2         [deprecated]
SESSION_STATUS_LOADING = 3       [deprecated, → CREATING]
SESSION_STATUS_PAUSED = 4
SESSION_STATUS_NEEDS_APPROVAL = 5 [deprecated — "NeedsApproval is now a sub-status"]
SESSION_STATUS_CREATING = 6
SESSION_STATUS_STOPPED = 7        (terminal)
SESSION_STATUS_HIBERNATED = 8
SESSION_STATUS_RESTORING = 9      (transient, never persisted)
```

Confirms the requirements doc's constraint (`requirements.md:27`): `NEEDS_APPROVAL` is deprecated
wire-value-only, kept for backward compat. **Nothing in `SessionList.tsx` currently cross-references
live approvals against sessions** — `SessionCard.tsx` still branches on
`session.status === SessionStatus.NEEDS_APPROVAL` for badge color/text
(`SessionCard.tsx:193,256`, `getStatusColor`/`getStatusText`), which is the *deprecated* signal,
not the real one.

The real, current "has a pending approval" signal lives in:
- **`ApprovalsContext.tsx`** (`web-app/src/lib/contexts/ApprovalsContext.tsx`, read in full) — a
  single RTK-Query polling singleton (5s interval, `useGetApprovalsQuery`,
  `ApprovalsContext.tsx:40-42`) exposing `approvals: PlainApproval[]` and `pendingCount`. Each
  `PlainApproval` carries a `sessionId` (used for `clearForSession`/`clearedSessions` filtering,
  `ApprovalsContext.tsx:61-86`).
- **`useApprovals({ sessionId })`** (`web-app/src/lib/hooks/useApprovals.ts`, read in full) — thin
  wrapper that filters `ApprovalsContext`'s global `approvals` array down to one session
  (`useApprovals.ts:52-58`). Currently only consumed by `ApprovalPanel.tsx` and
  `ApprovalDrawer.tsx` — **not** by `SessionCard.tsx` or `SessionList.tsx`.
- **`ReviewQueueContext.tsx`** (19 lines, read in full) — separate provider wrapping
  `useReviewQueue({ baseUrl, useWebSocketPush: true, autoRefresh: true })`, backing the standalone
  `/review-queue` page and `ReviewQueuePanel.tsx`/`ReviewQueueBadge.tsx`. This is a **second,
  parallel** review-tracking mechanism from `ApprovalsContext` — worth clarifying in planning
  whether "Needs Review" column membership should be sourced from `ApprovalsContext` (tool-use
  approvals, session-scoped) or `ReviewQueueContext` (broader review-queue items, which may not
  all be 1:1 with a `sessionId`). The open question in `requirements.md:72` ("any pending approval
  vs. primary status computation") maps directly onto this: **the board's "Needs Review" column
  must be built by joining `sessions[]` against `useApprovalsContext().approvals` on
  `sessionId`**, not by reading `session.status`, since the deprecated enum value is not reliably
  populated going forward. Confirm with `ReviewQueueContext` whether it's a superset before basing
  the column on `ApprovalsContext` alone.

### Column ↔ status precedence problem (rabbit hole flagged in requirements.md:53)

A session can be `SESSION_STATUS_ACTIVE` (running) **and** have a pending `PlainApproval` at the
same time — these are orthogonal booleans (`status` + `hasApproval`), not mutually exclusive enum
states. Since the board slots each card into exactly one column, planning must pick a precedence
order. The existing `groupSessions` `statusOrder` table (`strategies.ts:179-192`) already encodes
one plausible precedence (`Needs Approval` sorts after `Active`/`Ready`/`Loading` but before
`Paused`) — reusing that ordering as the column-precedence rule (approval-pending beats
Running/Active for column placement, but Paused/Stopped/Hibernated as a raw status always wins
over a stale/leftover approval) is a reasonable default to propose in planning, but it is a
product decision, not something inferable purely from the code.

## 4. Session statuses that don't map to the 4 default columns

Given the 9-value enum, only `ACTIVE`(1) → Running, `PAUSED`(4) → Paused, and `STOPPED`(7) →
Complete map directly. Left over:
- `CREATING`(6) / `RESTORING`(9) — transient startup states. Neither "Running" (not yet active)
  nor any other column cleanly fits; likely needs a "Starting…" sub-badge inside the Running
  column (mirrors `SessionCard.tsx:213`'s existing `"Starting…"` label for `CREATING`) rather than
  a 5th column, to avoid violating the requirements' fixed 4-column scope.
- `HIBERNATED`(8) — `SessionCard.tsx:198-199` already visually collapses this into the same style
  bucket as `STOPPED` ("no distinct style yet; reuses paused" — comment is stale/misleading, it
  actually reuses the *stopped-style* `statusPaused` token, distinct from `statusPausedDistinct`
  used for real `PAUSED`). Board planning should decide: Hibernated → Complete column (matches
  existing visual precedent) or Hibernated → Paused column (matches the "session is idle, could be
  resumed" semantics, since `resumeSession`/`hibernateSession` in `useSessionService.ts:97-99,386`
  treat Hibernated as resumable, unlike terminal Stopped).
- `READY`(2) / `LOADING`(3) — deprecated aliases, effectively dead going forward; treat as folding
  into `ACTIVE`/`CREATING` respectively for column purposes, consistent with how `SessionCard.tsx`
  already has to carry a fallback for them.
- `UNSPECIFIED`(0) — defensive-only, should never appear in practice; needs *some* fallback column
  (likely Running or an "Unknown" 5th bucket only shown if non-empty) so a session doesn't
  silently vanish from the board.

## 5. Drag-and-drop library — confirmed absent (feasibility risk validated)

`web-app/package.json` dependencies were greped for `dnd`, `drag`, `sortable`, `beautiful-dnd`,
`arborist` — **no drag-and-drop library is present**. The one hit, `react-arborist@3.4.3`
(`package.json:84`), is a tree-view component (used for a file/directory browser elsewhere in the
app per its name), not a drag-and-drop primitives library, though `react-arborist` does have
internal drag-to-reorder tree support — irrelevant here since kanban card-between-columns DnD is
a different interaction model (columns, not tree nesting). Confirms `requirements.md:62`'s
feasibility risk: introducing `@dnd-kit/core` (or similar) is a real new-dependency decision for
Phase 2/3 build-vs-buy, not something already half-available in the codebase.
`react-virtuoso@4.18.7` (list virtualization, already a dependency) has no built-in DnD story;
combining a DnD library's sortable-list abstraction with `react-virtuoso`'s windowing inside each
column is exactly the "non-trivial" integration the requirements doc already flags
(`requirements.md:55`) — most DnD libraries (dnd-kit, react-beautiful-dnd's unmaintained
successor `@hello-pangea/dnd`, `pragmatic-drag-and-drop`) assume all draggable items are mounted;
windowed lists that unmount off-screen cards break drop-target measurement unless the DnD library
explicitly supports virtualization (dnd-kit does, via manual sensor/virtualization integration,
but it requires custom work, not a drop-in).

## 6. Competitor tools named in the original issue (general knowledge, no live research)

- **Vibe-Kanban** / **Agent-Kanban** / **Dorothy** — all three are recent (2025-era) "AI coding
  agent session board" tools in the same space as stapler-squad itself: they visualize concurrent
  AI agent runs (Claude Code / Aider / similar) as kanban cards moving through
  Queued→Running→Review→Done-style lanes, with the card representing a whole agent
  session/task rather than a human-authored ticket. The common shape across this class of tool:
  - Columns are *status-derived*, not user-defined (matches this project's "default 4 columns,
    no custom columns" scope decision).
  - Cards show live/streaming state (token cost, last output line, elapsed time) — something
    `SessionCard.tsx` already renders in list view (rate-limit state, snapshot terminal preview)
    that a board card should carry over in a condensed form, not drop.
  - Drag-and-drop in these tools is typically used as a *shortcut* for the same action a button
    already performs (pause/approve/stop), matching this project's explicit constraint of not
    introducing new state-transition RPCs — DnD is sugar over `pause_session`/`resume_session`/
    equivalent, exactly as scoped here.
  - None of these tools generally support real touch drag-and-drop reordering across columns in
    v1 — they lean on desktop-primary UX, consistent with this project's rabbit-hole note flagging
    touch DnD as out of scope for a reasonable v1.

## 7. Edge cases the design must handle (synthesis)

- **Active + pending-approval simultaneity** — needs an explicit precedence rule (see §3);
  recommend Needs-Review wins column placement over Running, sourced from `ApprovalsContext`, not
  `session.status`.
- **CREATING / RESTORING / HIBERNATED / STOPPED-vs-terminal** — none map 1:1 to the 4 columns; see
  §4 for a proposed folding, needs a planning decision, not a code-inferable answer.
- **Empty columns** — with only 4 fixed columns (vs. today's dynamic `groupSessions` output which
  only emits non-empty groups), a board will very plausibly render a genuinely empty "Needs
  Review" column much of the time; needs an explicit empty-state (not just a blank div) so it
  reads as "0 sessions need review," not "broken."
- **Large columns (100+ sessions)** — `requirements.md:30` already flags this; `react-virtuoso`
  windowing exists today for the flat/grouped list but per-column virtualization inside a
  drag-and-drop board is new integration work (see §5) — a naive first cut risks silently
  rendering 100+ un-virtualized DOM nodes per column once several long-running sessions pile up in
  "Complete."
- **Invalid drag targets / state-transition legality** — dragging a `STOPPED` (terminal, per
  proto comment `types.proto:337` "cannot transition further") session back to Running has **no
  corresponding RPC** (`useSessionService.ts` exposes `pauseSession`, `resumeSession`,
  `hibernateSession`, and presumably a stop/delete path, but nothing "un-stops" a session). The
  board must either (a) not register Complete→Running/Paused as valid drop targets at all (grey
  out / reject-with-shake), or (b) if the underlying model actually supports it via a different
  action (e.g., "resume from stopped" might really mean spawning a *new* session against the same
  branch/worktree, not resuming the old one) — this needs explicit confirmation against
  `session/instance.go`'s state machine before drag targets are wired, since the requirements
  explicitly forbid inventing new RPCs (`requirements.md:25`) and this may reveal that some
  column-pairs simply have no legal drag path with existing RPCs.
- **Drag failure/rollback** — flagged in requirements as a rabbit hole
  (`requirements.md:54`); given `SessionList.tsx`'s existing bulk-action pattern is
  fire-and-forget with a toast (`showFeedback`, no visible rollback UI on RPC failure for
  pause/resume today), the board should likely follow the same "toast on error, but the pattern
  for an optimistic card move needs its own snap-back state" — this is new UX the existing list
  view doesn't have to solve (list view doesn't visually move a row when you click Pause; a board
  drag *is* the move).

## 8. Unstated user needs beyond explicit requirements

- **Column session counts** — every kanban tool in this class shows an N-in-column badge in the
  column header; trivial to add (`groupedSessions[i].sessions.length`) and directly serves the
  stated success metric ("identify... how many sessions are in each... without scrolling").
  Should be treated as in-scope even though not explicitly called out, since it's the literal
  mechanism by which the success metric is satisfied — a board without visible counts only
  partially satisfies "identify how many... without scrolling" (you'd have to count cards).
- **WIP limits** — classic kanban feature (cap column card count, flag over-limit); **not**
  requested and should be explicitly called out as out-of-scope in planning, since "Complete"
  columns in an agent-session context are naturally unbounded (completed sessions accumulate) and
  a WIP limit doesn't map to any existing session-lifecycle concept here — avoid scope creep.
- **Column collapsing** — `SessionList.tsx` already has `collapsedGroups: Set<string>`
  (`SessionList.tsx:345`) for the grouped list view; a board arguably wants the same per-column
  collapse affordance (useful once one column has 100+ cards per §7), and since the state shape
  already exists and is persisted, extending it to board columns is low-cost reuse rather than a
  new feature — worth flagging as a "cheap to include" addition in planning even though not
  explicitly requested.
- **Persisted swimlane axis choice** — requirements say the board can switch its axis to any
  `GroupingStrategy` (`requirements.md:40`), but doesn't say whether that choice persists
  independently from list view's `groupingStrategy` (`STORAGE_KEYS.GROUPING_STRATEGY`) or shares
  the same stored value. Given the `storageKeyPrefix` pattern already used for split-view
  instances (`SessionList.tsx:91`), the natural approach is a *separate* stored key for
  board-axis vs. list-grouping so switching views doesn't silently reorder the other view's state
  — worth an explicit planning decision.
- **View toggle keyboard shortcut collision** — requirements specify `b` for List/Board toggle
  (`requirements.md:37`); `SessionList.tsx` already binds several keys at the document level
  (e.g. `Escape` clears selection, `SessionList.tsx:800-806`) — Phase 3 planning should verify `b`
  isn't already bound elsewhere (e.g. inside an omnibar or text input focus context) before
  wiring a document-level listener, and should scope it to "not when a text input has focus" the
  way the existing `Escape` handler likely already needs to (not verified in this pass — flag for
  planning/implementation to check `document.activeElement` guards on the new handler).
