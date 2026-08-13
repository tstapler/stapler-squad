# Research: Existing Features & Industry Patterns for Board/Kanban View

Agent 2 (Features) — SDD research phase for `board-kanban-view`.

## 1. Backend status model — what "Running/Needs Review/Paused/Complete" actually maps to

### `SessionStatus` (the real enum, `session/instance.go:24-47`, mirrored at
`web-app/src/gen/session/v1/types_pb.ts:3414-3494`)

```go
Creating   Status = 0  // transient, before first start
Active     Status = 1  // replaces legacy Running/Ready
Paused     Status = 2  // worktree removed, branch preserved
Stopped    Status = 3  // terminal, cannot transition further
Hibernated Status = 4  // checkpointed, tmux killed
Restoring  Status = 5  // transient, never persisted
```

There is **no `Complete`/`NeedsReview` value** in `SessionStatus`. The item's four proposed
columns map like this:

| Item's column | Real backend concept |
|---|---|
| Running | `SessionStatus.ACTIVE` |
| Paused | `SessionStatus.PAUSED` |
| Complete | `SessionStatus.STOPPED` (best fit — terminal state). `HIBERNATED` is a distinct terminal-ish state (checkpoint written, resumable) that arguably needs its own column or a merged "Paused/Hibernated" swimlane — open question for planning. `CREATING`/`RESTORING` are transient and probably render as a brief loading state on whatever column they're headed to, not a durable column. |
| Needs Review | **Not a `SessionStatus` value at all.** See §2. |

### 2. "Needs Review" is a derived condition, and there are *two* independent implementations of it already

**(a) Per-session field, used by the existing list-view filter** (`SessionList.tsx:572`):
```ts
session.status === SessionStatus.ACTIVE &&
  (session.subStatus === SubStatus.NEEDS_APPROVAL || session.subStatus === SubStatus.INPUT_REQUIRED)
```
`SubStatus` (`types_pb.ts:3716+`) is a *second* enum on `Session` — `IDLE`, `PROCESSING`,
`NEEDS_APPROVAL` (3), `ERROR` (4), etc. — orthogonal to `SessionStatus`. This is what backs
the "Needs approval" checkbox filter already in `SessionList.tsx` (`FILTER_NEEDS_APPROVAL`
storage key). It's cheap: computed client-side from fields already on the `Session` object
returned by `ListSessions`/`WatchSessions`, no extra RPC.

**(b) A separate backend-computed priority queue**, `GetReviewQueue`/`WatchReviewQueue` RPCs
(`session.proto`, consumed via `useReviewQueue.ts`), returning `ReviewQueue { items: ReviewItem[] }`
where each `ReviewItem` carries an `AttentionReason` (`APPROVAL_PENDING`, `INPUT_REQUIRED`,
`ERROR_STATE`, `IDLE_TIMEOUT`, `TASK_COMPLETE`, …) and a `Priority`. This is the backing for
the dedicated Review Queue page/panel (`ReviewQueuePanel.tsx`, `ReviewQueueBadge.tsx`,
`ReviewQueueContext.tsx`) and has its own WebSocket push stream + `AcknowledgeSession` RPC.

**Planning must pick one, explicitly** — they are not guaranteed to agree (the queue includes
`TASK_COMPLETE`/`IDLE_TIMEOUT` reasons that aren't "needs approval" in the `SubStatus` sense,
and the queue can contain items whose underlying `SessionStatus` isn't `ACTIVE`). Reusing (a)
keeps the board consistent with the existing list-view filter and needs no new data
dependency; reusing (b) gets parity with the dedicated Review Queue page/badge but pulls in
its own stream/poll lifecycle and a broader definition of "needs attention" than pure
approval-pending. Recommend defaulting to (a) for the "Needs Review" *column membership* test
(cheapest, most consistent with `SessionList`'s own existing language), while still
reflecting arrival in queue (b) as a visual badge on the card if the item also appears there.

### 3. Valid drag transitions ride entirely on the existing `UpdateSession` RPC

`UpdateSessionRequest` (`session.proto:579-617`) already has `optional SessionStatus status = 2`
plus `optional string pause_reason = 9` (defaults to `"manual"` server-side if empty when
pausing). **There is no separate `PauseSession`/`ResumeSession`/`StopSession` RPC** — the
open question in requirements.md naming those three RPCs is based on a wrong assumption; they
don't exist as distinct RPCs. All status-changing drags are the same one RPC,
`UpdateSession({id, status})`. Column-drop → RPC call is therefore uniform code, not a
per-column special case — the interesting work is client-side validation of which transitions
are legal *before* firing the call (so an illegal drag can be rejected instantly without a
round trip), which needs a `SessionStatus → SessionStatus[]` legal-transition table. No such
table currently exists in the frontend; it will need to be authored fresh (backend-side
validation presumably already rejects illegal transitions — confirm exact rejected cases in
`server/services/session_service.go`'s `UpdateSession` handler during planning, not assumed
here).

There is no RPC input for "drag into Needs Review" — since that's a derived state, not a
`SessionStatus` value, a card cannot be dragged *into* that column by calling `UpdateSession`;
the column populates only from cards that already satisfy the `SubStatus` condition. Dragging
*out* of Needs Review most likely means resuming/answering the approval (not a status RPC at
all) — this needs explicit non-goal framing in the plan: "Needs Review" is a read-only-as-
drop-target column, or dragging out simply changes `SessionStatus` and happens to also clear
`SubStatus` as a side effect of the underlying action.

## 4. What Board must integrate with in the existing List view

### `web-app/src/components/sessions/SessionList.tsx` already has an analogous "card" mode

Surprising and directly relevant precedent: `SessionList` **already has a `viewMode?: "card" | "row"` prop** (line 96) — a full-card grid rendering, as an alternative to compact table rows, both driven by the *same* `groupedSessions`/`filteredSessions` pipeline (`viewMode` gates which branch of `useMemo` builds `cardFlatSessions` vs `flatItems`, lines 658-696). This is not the kanban board (it's a single-axis grid of cards, not status columns), but it proves the codebase's existing pattern for "same underlying session list, second rendering mode" and is the nearest neighbor to crib rendering/interaction/CSS conventions from, and to decide whether Board should be a third `viewMode` value inside `SessionList` (maximizing state/logic reuse) or a sibling component (cleaner separation, but must re-plumb search/filter/selection state).

**State reuse implication:** `SessionList` owns `selectMode`, `selectedSessions` (a `Set<string>`, lines 360-362), `searchQuery`, `groupingStrategy`, and all the filter/sort state internally — none of it is lifted to a shared hook or Redux slice. If Board is built as a *separate* component (not a third `viewMode` inside `SessionList`), all of search/bulk-select/grouping-strategy state needs to be either lifted out of `SessionList` into a shared hook, or duplicated. This is the single biggest architecture fork for the planning phase: **extend `SessionList`'s existing `viewMode` union** (least duplication, but couples board's column-layout unlike its two siblings which are both flat/grouped-list layouts) **vs. extract shared state into a hook consumed by both `SessionList` and a new `SessionBoard`** (cleaner, more upfront work). Given the acceptance criteria explicitly require search/bulk-select *parity*, not just similarity, extracting shared state now avoids two independently-drifting copies of that logic later.

### Grouping strategy reuse (`web-app/src/lib/grouping/strategies.ts`)

`GroupingStrategy` enum: `Category, Tag, Branch, Path, Program, Status, SessionType, Project,
Workflow, None`. `groupSessions(sessions, strategy, options)` returns `GroupedSessions[]`
(`{groupKey, displayName, sessions}`), with **multi-membership support for `Tag`** — a session
can appear in more than one group simultaneously. This directly affects board swimlanes: if
the swimlane axis is `Tag`, a session could need to render in *multiple* swimlane rows at
once, which has no natural equivalent for kanban cards (a card is normally single-instanced on
a board). Planning must decide: dedupe to first tag, render duplicate cards across swimlanes,
or restrict the swimlane-axis selector to exclude `Tag` when in board view (simplest, but a
Board-specific carve-out on an otherwise-shared selector, which the requirements say should be
"the existing grouping-strategy selector" — an unreconciled tension worth flagging explicitly
in the plan rather than silently picking one).

Also note `GroupingStrategy.Status` already exists as a *grouping* option — i.e. list view can
already group flatly by status today. Board's default status-column view is conceptually
"grouping by Status, rendered as columns instead of stacked sections" — worth checking whether
`groupSessions(sessions, GroupingStrategy.Status)` can be reused as-is for the default board
column computation (its group keys would need to be reconciled with the 4-column Running/Needs
Review/Paused/Complete scheme, since `GroupingStrategy.Status` presumably groups by raw
`SessionStatus` enum value, not the "Needs Review" derived condition).

### Bulk select / search (`BulkActions.tsx`, `SessionList.tsx`)

`BulkActions` is a dumb, fully prop-driven toolbar (`selectedCount`, `onPauseAll`,
`onResumeAll`, `onDeleteAll`, `onAddTagAll`, `onSelectAll`, `onClearSelection`, `onGroupAs`) —
it has zero knowledge of rows/cards/columns, so it drops into a board layout unchanged as long
as the board computes the same `selectedCount`/`totalCount`/callbacks. Selection membership
today (`SessionList.tsx:640`) is `activeSelection = selectedSessions ∩ filteredSessionIds` —
i.e. selection already survives filter changes by intersecting against whatever's currently
visible; a board would need the equivalent "currently visible across all columns" set for the
same behavior. No separate cross-column concern exists today because there's only one flat
list — this is genuinely new logic for Board (a Set that spans column boundaries), but the
intersection-with-filtered pattern to copy already exists.

Search is a single `searchQuery` state filtered client-side (not the backend) inside
`SessionList`'s `useMemo` filter chain — trivially reusable by a board fed from the same
filtered array.

## 5. Persistence pattern for "last-used view per workspace"

**No per-workspace-ID-keyed localStorage pattern exists anywhere in the frontend.** Searched
`web-app/src/lib/hooks`, `web-app/src/lib/contexts`, and all `localStorage` call sites — none
key by a workspace identifier. The one scoping mechanism that exists is `SessionList`'s
`storageKeyPrefix` prop (`SessionList.tsx:91-92, 223-243`): a plain string prefix prepended to
a fixed `BASE_STORAGE_KEYS` map (e.g. `stapler-squad-search-query` →
`pane-{id}.stapler-squad-search-query`), used today only to disambiguate **multiple
`SessionList` instances in the same page** (split-pane view, `PaneSplitRenderer.tsx:192`,
prefix `pane-${pane.id}.`) — not per-workspace.

Critically: **`workspace` in this codebase is a server-side, per-process concept, not a
client-side multi-tenant switcher.** Per `.claude/docs/state-isolation.md`, each server
instance binds its own state under `~/.stapler-squad/workspaces/{SHA256(cwd)}/` (or an
explicit `STAPLER_SQUAD_INSTANCE`), and — per the project's own multi-instance testing
convention in `CLAUDE.md` — different workspaces are reached by running a **separate server
process on a separate port**, which is a **separate browser origin**. Plain `localStorage`
(unprefixed, exactly like `SessionList`'s existing `BASE_STORAGE_KEYS`) is already
origin-scoped by the browser, so it is *already* per-workspace for free — no new workspace-ID
key-prefixing scheme is needed. **Recommendation: persist the view-mode toggle with a plain
`localStorage` key (`stapler-squad-session-view-mode`, following the existing naming
convention) exactly like the rest of `BASE_STORAGE_KEYS`, not a workspace-ID-keyed scheme** —
matches the codebase's own existing (implicit) assumption and needs zero new
infrastructure. Flag this explicitly in the plan since the requirements doc's Open Questions
section assumed a workspace-scoped mechanism needed to be found/built.

## 6. Keyboard shortcut precedent

Existing single-letter shortcuts are registered ad hoc per-page (not a central registry) — e.g.
`page.tsx:477-488` handles a `'d'`-triggered delete-confirm modal with local `onKeyDown`
checking `e.key`. No centralized "keyboard shortcut map" exists to consult for collisions;
research did not find an existing binding on `b` in `web-app/src/app/page.tsx` or
`SessionList.tsx`, but the planning phase should grep the actual shortcut dispatch site in
`page.tsx` (and the omnibar's own key handling, which intercepts most single keys while
focused) for the full live set before wiring `b`, and must guard against firing while any
text input/omnibar has focus (the item's own AC #1 already states this requirement).

## 7. Industry patterns: Trello / Linear / GitHub Projects / Jira / Vibe-Kanban

(General knowledge of these tools' established UX conventions — not sourced from a specific
verified document for this research pass; flag as informed-by-general-product-knowledge, not
independently re-verified against current product behavior.)

### Failure modes / edge cases these tools all had to solve

- **Empty columns**: rendered with a visible drop-zone affordance (dashed border/placeholder
  text) rather than collapsing to zero width — a zero-width column is not a droppable target a
  user can see. Board's "Needs Review" column will frequently be empty and must not visually
  disappear (that would make it look like the feature is broken, not "nothing needs review").
- **Huge column counts / huge card counts per column**: Trello and GitHub Projects both
  virtualize or lazy-render cards beyond a threshold (Trello historically capped visible cards
  and paginated with "show more"; GitHub Projects virtualizes). With hundreds of sessions
  possible in this app (this is an *agent-orchestration* dashboard, not a small human task
  board), an un-virtualized column list is a real perf risk — same concern the existing list
  view already had to solve (see `SessionList.tsx` skeleton/pagination patterns) and Board
  must not regress.
- **Real-time updates from other clients while dragging**: Linear and GitHub Projects both use
  optimistic local reordering during drag, then reconcile against server state on drop;
  if another client moves the same card mid-drag, the dropped mutation can race a
  server-side state that no longer matches what the dragging client saw. The safe pattern is:
  drag interaction operates on a local snapshot; on drop, send the mutation; if the
  server rejects (session already transitioned, e.g. underlying process exited and flipped to
  `STOPPED` mid-drag) snap the card back and show an inline error — which is exactly AC #5's
  required behavior, so the existing WebSocket session-event stream (`WatchSessions`) needs to
  be reconciled against in-flight drag state rather than blindly re-rendering out from under
  the user's cursor.
- **Stale drag targets**: if a column disappears (e.g. swimlane axis changes mid-drag, or a
  session gets deleted from another client) during an active drag, the drop handler must
  tolerate a target that no longer exists rather than throwing.
- **Cards with long content**: Trello/Linear both truncate card body text with an
  expand-on-hover or click-through-to-detail pattern rather than growing card height
  unboundedly — relevant here since a session card likely wants to show truncated
  goal/task text, tags, and status badges without the column scrolling unpredictably.

### Unstated needs likely to surface in usage (not in the explicit ACs)

- **WIP limits per column** (classic Kanban discipline, present in Jira/Trello Power-Ups) —
  explicitly out of scope per Non-Goals ("no free-form add-a-column builder") but a *soft*
  visual WIP indicator (count badge going red past a threshold) is cheap and commonly
  requested once a board ships; worth a one-line non-goal callout rather than silently
  omitting it.
- **Column collapse** (Linear, GitHub Projects both support collapsing a column to a thin
  strip) — useful once four-plus columns are visible on a laptop-width screen; interacts with
  the mobile-scroll requirement in AC #10.
- **Sort within column** (by last-activity, token cost, etc. — `SessionList` already supports
  these sort fields) — users will expect the same sort control inside each column that exists
  in list view; not mentioned in ACs but a natural parity gap once columns exist.
- **Multi-select drag** (drag N selected cards to a new column at once) — natural once
  bulk-select works across columns (AC #8); dragging one card while N are selected is
  ambiguous UX (does it move just that card, or the whole selection?) that Trello/Linear both
  resolve by moving the whole selection if the dragged card is part of it. Should be an
  explicit decision in planning, not left implicit.
- **Non-drag fallback for touch** (AC #10 already anticipates this) — GitHub Projects' mobile
  web UI uses a per-card "..." menu → "Move to..." submenu as the touch-friendly equivalent;
  same pattern is directly applicable here and matches the existing per-row action-menu
  pattern already present in `SessionRow.tsx`.

## 8. Drag-and-drop library

Confirmed via `web-app/package.json`: **no drag-and-drop library is currently a dependency**
(`@dnd-kit/*`, `react-beautiful-dnd`, `react-dnd`, `sortablejs`, `dragula` — none present).
Planning must make an explicit build vs. dependency call (ladder rung 4/6 per this project's
usual dependency-addition bar): `@dnd-kit/core` is the modern standard (actively maintained,
accessible via keyboard, `react-beautiful-dnd` is unmaintained) and directly supports the
touch-fallback and cross-column-with-virtualization needs called out above; hand-rolled HTML5
DnD is lower-dependency but would need to reimplement keyboard accessibility and touch
handling from scratch to meet AC #10's stated requirement for touch parity.

## Summary of open items for the planning phase

1. Pick the "Needs Review" data source: `SubStatus`-derived (cheap, consistent with existing
   list filter) vs. `ReviewQueue`/`AttentionReason` (broader, matches dedicated review page) —
   recommend `SubStatus`-derived as primary column membership test.
2. Decide Board's architecture: third `SessionList.viewMode` value vs. new `SessionBoard`
   component + extracted shared state hook (search/select/grouping) — recommend extraction
   given the "parity not similarity" bar in the ACs.
3. Author a `SessionStatus → SessionStatus[]` legal-transition table client-side (none exists
   today); confirm against the actual `UpdateSession` handler's server-side validation in
   `server/services/session_service.go` during planning, not assumed here.
4. Resolve the `Tag` grouping strategy's multi-membership vs. single-card-per-board tension for
   swimlanes.
5. Confirm `HIBERNATED` and `CREATING`/`RESTORING` treatment in the 4-column scheme (merge into
   Paused/transient-overlay respectively, or add columns).
6. Persist view-mode with a plain (workspace-origin-scoped) localStorage key, not a new
   workspace-ID keying scheme — no such scheme exists elsewhere in the codebase to mirror.
7. Choose `@dnd-kit/core` (or equivalent) vs. hand-rolled DnD, weighed against the touch-fallback
   and keyboard-accessibility requirements in AC #10.
8. Decide multi-select-drag semantics (move whole selection vs. just the dragged card).
