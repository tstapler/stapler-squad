# Build vs. Buy: Drag-and-Drop Mechanic for Board/Kanban View

Scope: this evaluates only the drag-and-drop interaction layer. Columns, cards,
swimlane grouping, and status-mutation logic are inherently custom to this
app's session domain regardless of which option below is chosen (per
`requirements.md` framing) and are out of scope here.

## Codebase facts checked before evaluating

- `web-app/package.json` has no DnD library — confirmed via `Read` (deps and
  devDeps lists checked in full, no `@dnd-kit/*`, `react-beautiful-dnd`,
  `@hello-pangea/dnd`, or `@atlaskit/pragmatic-drag-and-drop` present).
- The app already has a kanban-shaped board: `web-app/src/components/backlog/BacklogBoard.tsx`
  (backlog items, 5 status columns, count badges, live-update fade/flash
  transitions). **It has zero drag-and-drop code** — column membership changes
  only in response to server-pushed status events (`useWatchBacklogItems`),
  never a user drag gesture. Its column/card/count/empty-state/skeleton
  structure and CSS (`BacklogBoard.css.ts`) are a directly adaptable visual
  skeleton for the new session board, but contribute nothing to the DnD
  mechanic itself.
- The one existing native HTML5 drag-and-drop usage in the codebase,
  `web-app/src/components/pane/PaneSplitRenderer.tsx:219-246`, is a 2-element
  pane swap: `onDragStart` stashes an id in `dataTransfer`, `onDragOver` calls
  `preventDefault()`, `onDrop` reads the id back and dispatches a swap. It has
  no reordering, no drop-zone highlighting, no touch support, and no keyboard
  alternative — it works because a pane swap is a single fixed source/target
  pair, not a multi-column, multi-card, touch-and-keyboard-accessible board.
  This is useful evidence of what native HTML5 DnD *doesn't* give you for free
  once requirements grow past "swap two things."
- React is `^19.0.0` (`web-app/package.json`), styling is vanilla-extract
  (`.claude/rules/css-architecture.md`) — checked against each library's
  React peer range and CSS approach below.

## 1. Existing OSS libraries

### @dnd-kit/core + @dnd-kit/sortable — **Recommended**

- **Maturity/maintenance**: `npm view @dnd-kit/core` → v6.3.1, MIT, peer dep
  `react: ">=16.8.0"` (VERIFIED via `npm view`, run 2026-08-06). Last publish
  per npm registry metadata is Dec 2024 — about 20 months stale at time of
  writing, not "2 years" as some search snippets estimated. `@dnd-kit/sortable`
  is separately versioned at v10.0.0, peer dep `@dnd-kit/core: ^6.3.0`, so the
  sortable layer has seen more recent iteration than core. 5.2M weekly
  downloads, 282 dependents (WebSearch, npmjs.com) — heavily used despite the
  slow core release cadence. An open GitHub issue
  ([clauderic/dnd-kit#1511](https://github.com/clauderic/dnd-kit/issues/1511))
  tracks explicit "officially support React 19" as unresolved, but the peer
  range is a loose `>=16.8.0` floor with no upper bound, so it **installs and
  runs on React 19 without a peer-dep override** — the gap is "not yet
  advertised as tested," not a hard incompatibility. A newer `@dnd-kit/react`
  package (v0.5.0, 2 months old per WebSearch) signals a rewrite in progress,
  which is a maintenance-trajectory signal worth re-checking before or during
  implementation, not a launch blocker.
- **License**: MIT — no encumbrance.
- **Bundle size**: `@dnd-kit/core` is 14.2KB gzip (measured via bundlephobia
  API, 2026-08-06); `@dnd-kit/utilities` adds 1.6KB gzip. `@dnd-kit/sortable`
  size wasn't retrievable (bundlephobia rate-limited the request) but is
  documented as a thin layer over core. Well inside the project's 5MB total
  JS budget (`web-app/package.json`'s `size-limit` config) and comparable to
  already-shipped deps like `fuse.js` or `cronstrue`.
- **Fit to React 19 functional components + vanilla-extract**: hook-based API
  (`useDraggable`, `useDroppable`, `useSortable`, `DndContext`) — no class
  components, no CSS framework coupling, so it composes cleanly with
  vanilla-extract (styling stays entirely in `.css.ts` files; dnd-kit only
  contributes inline transform styles for the dragged element, which is the
  same "CSS custom property bridge" pattern `.claude/rules/css-architecture.md`
  already prescribes for other runtime-dynamic values).
- **Accessibility**: keyboard sensor with arrow-key movement + Space/Enter to
  confirm, built-in live-region screen-reader announcements
  (`@dnd-kit/accessibility`), customizable announcement text — meets WCAG 2.1
  drag-and-drop alternative guidance out of the box (WebSearch, multiple
  sources incl. dndkit.com docs). This directly satisfies req. AC #10's touch
  fallback need in spirit (keyboard path), though a "move to..." menu action
  is still needed for touch per AC #10's own text (see pitfalls.md if this
  concern is covered there).
- **Verdict: Recommended.** Best fit for a sortable multi-column board with
  React 19 functional components, small bundle cost, accessibility handled
  for the primary (keyboard) alternative path already required by the repo's
  own AC #10.

### react-beautiful-dnd — **Not recommended**

- **Maintenance**: officially deprecated by Atlassian; repository **archived**
  Aug 18, 2025 (read-only, no further PRs/issues) (WebSearch, github.com
  issue #2672 + npm deprecation notice). No React 19 support.
- **Verdict: Not recommended.** Dead project; adopting an archived dependency
  for a new feature is a maintenance liability from day one.

### @hello-pangea/dnd — **Viable, but not first choice**

- **Maintenance**: community-maintained fork of react-beautiful-dnd, v18.0.1
  (npm view, 2026-08-06), Apache-2.0. Peer deps `react: "^18.0.0 || ^19.0.0"`
  — explicit, tested React 19 support, which is cleaner than dnd-kit's
  situation. However WebSearch flagged "critically low" contributor activity
  (1 active contributor in the last quarter) — a single-maintainer
  bus-factor risk.
- **API/fit**: inherits react-beautiful-dnd's list-reordering-first API model
  (`DragDropContext`/`Droppable`/`Draggable`), which maps naturally to kanban
  columns-as-lists, but the library was designed around "physicality"
  (spring-animated reordering) that Atlassian itself moved away from when
  building pragmatic-drag-and-drop — see the reasoning in the deprecation
  notice. Multi-axis (column + swimlane) boards are a less natural fit than
  with dnd-kit's more primitive sensor/context model.
- **Verdict: Viable** as a fallback if dnd-kit's React 19 ambiguity becomes a
  real blocker during implementation, but the explicit React 19 peer support
  is offset by thin maintenance bandwidth on a fork of an already-abandoned
  project.

### Atlassian pragmatic-drag-and-drop — **Viable, strong on paper, higher integration cost**

- **Maintenance**: actively developed — v2.0.2 (npm view, 2026-08-06),
  Apache-2.0, and per WebSearch it's the direct dependency of Jira,
  Confluence, and Trello's own drag-and-drop, i.e. production-proven at
  large scale.
- **Bundle size**: Atlassian's own docs describe the core package as
  "tiny (< 4KB)" — smallest option here — achieved via granular entry-point
  imports rather than one monolithic package (atlassian.design docs,
  WebSearch).
- **API/fit**: lower-level than dnd-kit or hello-pangea/dnd — it's explicitly
  a primitives toolkit (drag source, drop target, hitbox utilities,
  autoscroll as separate optional packages) rather than an opinionated
  sortable-list abstraction. That gives more control but means more
  hand-assembly of the exact kanban behavior (column reflow, reorder-within-
  column, autoscroll-during-drag) compared to `@dnd-kit/sortable`, which
  already packages that behavior.
- **Accessibility**: WebSearch/DeepWiki summary of its own accessibility
  guidelines explicitly favors **action-menu alternatives over directional
  keyboard dragging** — i.e. it deliberately does *not* ship an arrow-key
  drag mode the way dnd-kit does, and instead recommends building a
  non-drag "move to..." action per item. That happens to align exactly with
  this project's AC #10 requirement for a non-drag mobile fallback, but means
  keyboard accessibility isn't "free" the way it is with dnd-kit's built-in
  keyboard sensor — it's a UI affordance this project would build either way
  per AC #10, just confirmed as the officially recommended pattern rather
  than dnd-kit's arrow-key mode.
- **Verdict: Viable**, arguably the safer long-term maintenance bet (backed by
  Atlassian's own production kanban surfaces) but a larger integration
  lift than `@dnd-kit/sortable` for the same v1 scope, since more of the
  sortable-column behavior has to be assembled from primitives rather than
  consumed from a pre-built hook.

### Native HTML5 drag events — **Not recommended for this feature**

- The codebase already has a working example
  (`PaneSplitRenderer.tsx:219-246`) proving native DnD is usable here for a
  narrow case (fixed 1:1 swap). But the kanban board's requirements go well
  past that: multi-column drop-zone detection with visual feedback (AC #4),
  rejected-transition visual feedback (AC #5), touch-device fallback (AC
  #10), and no built-in keyboard path at all. HTML5 native DnD (`draggable`,
  `dragstart`/`dragover`/`drop`) has well-documented deficiencies for exactly
  these cases: no touch event support at all (mobile requires a completely
  separate touch-event reimplementation), no accessibility story, and
  inconsistent/finicky drag-image and scroll-during-drag behavior across
  browsers.
- **Verdict: Not recommended.** Every one of AC #4, #5, and #10 would require
  hand-building what a library already solves; see section 3 below for the
  correctness-risk case in more detail.

## 2. SaaS/managed API

No hosted "kanban board" widget/API is a reasonable fit here, and none was
found worth naming as a real candidate. This feature is not an embeddable,
generic kanban surface — it's a drag layer over this app's own session
domain objects (`SessionStatus`, `GroupingStrategy`, bulk-select, instant
search, per-workspace persistence — requirements.md goals 2-5), all of which
require direct access to this app's React state, ConnectRPC session-update
mutations, and existing grouping/filtering code. A hosted widget would need
either a heavyweight two-way data sync layer or an iframe boundary, both of
which cost more integration complexity than they save, and neither is
consistent with "compose with existing session-list features" (requirements
Goal 4). **Verdict: Not recommended / not a real option** — correctly ruled
out by the requirements doc's own framing, not force-fit into this comparison
for completeness.

## 3. LLM-generated custom DnD vs. a tested library

Hand-rolling drag-and-drop for this feature means correctly implementing, at
minimum:

- Mouse drag lifecycle (start/move/end) with drop-zone hit-testing across N
  columns, not just a fixed pair like `PaneSplitRenderer`'s swap.
- Touch event handling (`touchstart`/`touchmove`/`touchend`) as a **separate**
  code path from mouse — HTML5 native drag events do not fire on touch
  devices at all, so "native DnD" for this feature actually means "write two
  independent gesture systems," not one.
- Keyboard alternative (arrow keys + confirm) satisfying AC #10 and general a11y
  expectations, including live-region announcements so screen readers report
  card movement.
- Scroll-during-drag (auto-scroll a column or the board when dragging near an
  edge) — a well-known hard-to-get-right interaction (velocity curves, edge
  thresholds, RAF loop cleanup) that both dnd-kit and pragmatic-drag-and-drop
  ship as tested, opt-in modules.
- Rejected-drop visual feedback and snap-back animation (AC #5) without
  janky intermediate states.

Each of these is a solved problem with edge cases that are easy to miss on a
first pass (and easy for an LLM in particular to miss, since the failure
modes — a dropped touch listener, a missed `preventDefault()`, a stale closure
in a RAF loop — don't show up as compile errors or even obvious runtime
errors; they show up as "drag felt janky" or "screen reader users can't use
this," which are exactly the kind of regression this repo's own review
practice — `.claude/rules/interface-pollution-checklist.md`'s sibling
discipline for structure, applied here to interaction correctness — is meant
to catch before merge, not after).

Weighed against the repo's own lazy-engineering ladder (`ponytail` skill):
rung 4 ("already-installed dependency solves it") doesn't apply — nothing
installed today solves this. But the ladder's spirit — stdlib/native before
custom, custom only as a last resort — argues for treating "add one
well-chosen, purpose-built dependency" as the step before rung 6 ("only then,
the minimum code that works"), not for jumping straight to hand-rolled touch
+ keyboard + autoscroll code that reinvents a well-solved problem. Custom
code would only be worth it here if the interaction surface were genuinely
trivial (e.g. `PaneSplitRenderer`'s fixed swap) — a multi-column, touch- and
keyboard-accessible, auto-scrolling kanban board is not that case.

**Verdict: adopt a library (dnd-kit).** Hand-rolled DnD is not "the minimum
code that works" here — it's re-implementing a large surface a maintained
library already covers, with correctness risk concentrated exactly in the
places (touch, keyboard, scroll) that are hardest to notice broken in review.

## 4. Fork or adapt existing code

- **`BacklogBoard.tsx`/`BacklogBoard.css.ts`** (this codebase): directly
  adaptable for the *visual* board skeleton — column layout, count badges,
  empty-column state, loading skeleton, live-update fade/flash transition
  pattern, and the "board subscribes to the same live store as the list
  view" architecture (Epic 5.2's stated principle, `BacklogBoard.tsx:146-150`)
  that this feature's requirements.md Goal 4 already asks for by analogy
  (compose with existing session-list features rather than forking). **It
  contributes nothing to the drag-and-drop mechanic itself** — it has no
  drag code at all; column changes are server-event-driven, not
  gesture-driven. Reuse its structural/CSS patterns; still need a DnD
  library for the interaction layer.
- **`PaneSplitRenderer.tsx`'s native HTML5 drag code**: not reusable beyond
  serving as a design-pattern precedent that the codebase is comfortable with
  `draggable`/`onDragStart`/`onDrop` for the narrowest case. Extending it to
  a full board would mean building everything section 3 describes from
  scratch — effectively the same custom-code path already recommended
  against.
- No other kanban/DnD component exists elsewhere in the dependency tree or
  was found in a sibling internal project search.
- **Verdict**: adapt `BacklogBoard`'s structural/CSS patterns for the board
  shell; add `@dnd-kit/sortable` net-new for the drag mechanic. No existing
  code covers the mechanic itself.

## Summary table

| Option | Verdict | Why |
|---|---|---|
| `@dnd-kit/core` + `@dnd-kit/sortable` | **Recommended** | MIT, small (~16KB gzip combined core+utilities), hook-based fit for React 19 functional components + vanilla-extract, built-in keyboard a11y + live-region announcements, heavily used despite slow core release cadence |
| `react-beautiful-dnd` | Not recommended | Deprecated, archived Aug 2025, no React 19 support |
| `@hello-pangea/dnd` | Viable (fallback) | Explicit React 19 peer support, Apache-2.0, but single-maintainer bus-factor risk and react-beautiful-dnd's older "physicality" model |
| Atlassian `pragmatic-drag-and-drop` | Viable | Smallest bundle (<4KB core), actively developed at Jira/Trello scale, but lower-level primitives mean more integration work for the same v1 scope; deliberately favors action-menu a11y over keyboard-drag |
| Hosted kanban SaaS/widget | Not recommended / not a real option | Feature requires deep session-domain integration (status mutations, grouping, search, bulk-select) that no embeddable widget can provide without a costly sync/iframe layer |
| Hand-rolled custom DnD | Not recommended | Reinvents touch, keyboard, autoscroll, and drop-rejection handling that a library already solves and tests; correctness risk concentrated in hard-to-review failure modes |
| Fork/adapt existing code | Partial | `BacklogBoard.tsx` reusable for board shell/CSS/live-update patterns; no existing code (including `PaneSplitRenderer`'s native DnD) covers the drag mechanic itself |

## One-line recommendation

Add `@dnd-kit/core` + `@dnd-kit/sortable` (MIT, ~16KB gzip, React-19-compatible
peer range, built-in keyboard/screen-reader support) as the drag mechanic, and
reuse `BacklogBoard.tsx`'s column/card/live-update structure for the board
shell — do not hand-roll DnD and do not pursue a hosted kanban widget.
