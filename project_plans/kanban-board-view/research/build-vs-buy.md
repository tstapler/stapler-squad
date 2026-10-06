# Build vs. Buy: Drag-and-Drop for Kanban Board View

Agent 6 research for `project_plans/kanban-board-view`. Scope: the drag-and-drop /
board-layout mechanism only — swimlane data derivation, column↔status mapping, and
RPC wiring are covered elsewhere in this research phase.

## Current state (verified in-repo)

- `web-app/package.json` has **no** dnd/drag/kanban/sortable package installed —
  confirmed via `grep -iE '"(.*dnd.*|.*drag.*|.*kanban.*|.*sortable.*)"' web-app/package.json`
  (empty result). This is a net-new dependency decision, as requirements.md §Rabbit
  Holes anticipated.
- Existing virtualization: `@tanstack/react-virtual@^3.13.25` and
  `react-virtuoso@^4.18.7` are both installed and in active use (`SessionList.tsx`
  per requirements.md:30). Any DnD choice must coexist with one of these.
- React 19 (`^19.0.0`), Next.js 15.3.2 — a modern stack that rules out any library
  without explicit React 19 peer-dep support.
- **Prior art found and inspected**: `web-app/src/components/backlog/BacklogBoard.tsx`
  (331 lines) + `web-app/src/app/backlog/board/page.tsx` (131 lines) already implement
  a 5-column status board (Idea/Ready/In Progress/Review/Done) for the backlog
  feature, with skeleton loading, exit-fade/enter-flash column transitions
  (`EXIT_TRANSITION_MS`, `ENTER_FLASH_MS`), and real-time updates via
  `useWatchBacklogItems`. **It has zero drag-and-drop** — confirmed via
  `grep -in 'drag\|dnd\|drop'` on both files (no matches). State changes happen only
  through per-card action buttons (`onAction`), the same button-driven pattern the
  kanban-board-view requirements doc says is today's only mechanism for sessions too.

## Option 1: Existing OSS library — @dnd-kit

**@dnd-kit/core@6.3.1 + @dnd-kit/sortable@10.0.0** (verified via `npm view`, 2026-08-06):

- License: MIT (both packages).
- Peer deps: `react: '>=16.8.0'`, `react-dom: '>=16.8.0'` — open-ended range, no
  React 19 blocker.
- Last publish: `@dnd-kit/core` 2024-12-05, `@dnd-kit/sortable` 2024-12-04 (per
  `npm view <pkg> time.modified`). No 2025/2026 release, but the API has been stable
  and the GitHub repo (`clauderic/dnd-kit`) remains the de facto standard successor
  to react-beautiful-dnd in the React ecosystem — not archived, unlike the
  alternative below.
- Unpacked size: `@dnd-kit/core` ~1.07 MB, `@dnd-kit/sortable` ~234 KB unpacked
  (npm registry metadata, not gzipped bundle size — real gzipped addition to the
  app bundle is materially smaller, typically cited around 10-15 KB gzipped for
  core+sortable combined in community bundle-size reports).
- Accessibility: keyboard sensor (`KeyboardSensor`), screen-reader announcements,
  and pointer/touch sensors are first-class, built-in — not bolted on.
- Virtualization fit: the known-good pattern for combining `@dnd-kit` with a
  virtualized list (`react-virtuoso` or `@tanstack/react-virtual`) is to decouple
  the dragged item's visual representation from the virtualized DOM via
  `<DragOverlay>` — the overlay renders a portal-based copy of the dragged card so
  the underlying virtualized list can continue to mount/unmount rows during scroll
  without corrupting the drag gesture. This is a documented, widely-used pattern
  (not novel integration work), but it is still integration work this project
  would have to do — dnd-kit does not ship virtualization-awareness out of the box.

**Pros:**
- Only actively-relevant option with genuine touch support (`PointerSensor` handles
  both mouse and touch), keyboard accessibility, and MIT license.
- No transitive dependency baggage — `@dnd-kit/core`'s only real runtime need is
  `react`/`react-dom`.
- Composable primitives (`DndContext`, `useDraggable`, `useDroppable`,
  `useSortable`) match this codebase's general preference for small, focused pieces
  over a monolithic framework (see `interface-pollution-checklist.md` — avoid
  adopting a big all-in-one abstraction where a few composable hooks suffice).

**Cons:**
- No release since December 2024 — worth monitoring, though the API surface is
  mature and the repo isn't marked archived.
- Requires custom column/swimlane markup — dnd-kit gives you drag mechanics, not a
  pre-built kanban board component, so column layout is still hand-built (this repo
  already has that half done in `BacklogBoard.tsx`'s column structure).
- `DragOverlay` + virtualization integration is a well-trodden pattern but still
  needs to be implemented and tested here specifically; not a zero-effort drop-in.

**Verdict: Recommended.**

## Option 2: react-beautiful-dnd (and its fork @hello-pangea/dnd)

**react-beautiful-dnd@13.1.1** (verified via `npm view`):
- `npm view react-beautiful-dnd deprecated` returns: *"react-beautiful-dnd is now
  deprecated. Context and options:
  https://github.com/atlassian/react-beautiful-dnd/issues/2672"* — Atlassian
  archived the project.
- Peer deps cap at `react: '^16.8.5 || ^17.0.0 || ^18.0.0'` — **no React 19
  support**, which alone disqualifies it for this codebase (`react: ^19.0.0`).
- Documented, longstanding limitation: react-beautiful-dnd requires all draggable
  items to be present in the DOM to compute drag measurements, which is fundamentally
  at odds with windowed/virtualized lists — the requirements doc's own Rabbit Holes
  section (line 55) calls this combination "non-trivial," and rbd is the library
  where that trouble is best documented upstream.

**@hello-pangea/dnd@18.0.1** (community-maintained fork, verified via `npm view`):
- License: Apache-2.0. Peer deps: `react: '^18.0.0 || ^19.0.0'` — does support
  React 19, and is actively published (last modified 2025-02-09).
- Inherits react-beautiful-dnd's API and its core virtualization limitation — it is
  a maintenance fork, not a redesign, so the same "all items must be in the DOM"
  constraint applies.

**Pros:**
- API is well-documented from rbd's years as the incumbent standard; lower learning
  curve for anyone who's used it before.
- `@hello-pangea/dnd` specifically does support React 19 and is maintained.

**Cons:**
- Original package is archived/deprecated by its author — a hard no for new code.
- Even the maintained fork carries forward the virtualization-hostile architecture
  that is a named risk in this project's own requirements doc.

**Verdict: Not recommended** (react-beautiful-dnd itself — deprecated, React 19
incompatible). `@hello-pangea/dnd` is **Viable** only as a fallback if `@dnd-kit`
integration with `react-virtuoso`/`@tanstack/react-virtual` proves harder than
expected during implementation — but should not be the starting choice given its
virtualization limitation is exactly the risk requirements.md flags.

## Option 3: Native HTML5 Drag-and-Drop API

No dependency — uses `draggable`, `dragstart`/`dragover`/`drop` events directly.

**Pros:**
- Zero bundle cost, zero new dependency to track for security/maintenance.

**Cons:**
- No built-in touch support at all (native HTML5 DnD is a desktop-mouse-only spec;
  requirements.md itself flags touch/mobile as a real open question, not an
  automatic "desktop-only," so a solution with literally no touch story forecloses
  that option rather than deferring it).
- No built-in keyboard/accessibility support — everything (focus management,
  screen-reader announcements, keyboard move) must be hand-built to reach the bar
  `@dnd-kit` provides out of the box.
- Custom-built drag-and-drop is a well-known class of hard-to-get-right UI problem:
  ghost-image styling quirks across browsers, drop-zone highlight state, scroll-while-
  dragging, and virtualization interaction all become bespoke code this team owns
  and debugs, rather than logic exercised by a library's existing test suite and
  wide user base.

**Verdict: Not recommended.** Saves a dependency but reintroduces every hard problem
(a11y, touch, virtualization interplay) that `@dnd-kit` has already solved, with no
compensating benefit for this project.

## Option 4: Full kanban component library (react-trello)

**react-trello@2.2.11** (verified via `npm view`):
- Last published 2022-06-26 — no update in 4 years.
- License: MIT, but its `peerDependencies`/`dependencies` pull in a legacy stack
  this codebase doesn't otherwise use: `react-redux@>=5.0.7`, `redux@>=4.0.0`,
  `redux-actions`, `redux-logger`, `styled-components@>=4.0.3`, plus
  `trello-smooth-dnd`, `immutability-helper`, `autosize`. None of `redux`,
  `react-redux`, or `styled-components` appear in `web-app/package.json` today —
  adopting react-trello means adopting an entire second state-management paradigm
  and a runtime-CSS-in-JS library the project's own CSS architecture rule
  (`.claude/rules/css-architecture.md`) explicitly forbids ("Runtime CSS-in-JS
  (styled-components, emotion) — incompatible with React Server Components").
- No React 19 peer constraint listed (`peerDependencies.react: '*'`), but an
  unmaintained package with an open-ended peer range is not a meaningful compatibility
  signal — it simply predates the versions it would be tested against.

**Pros:**
- Would ship board layout, columns, and cards as a turnkey component, in theory
  saving layout work.

**Cons:**
- Unmaintained since 2022; drags in `redux` + `styled-components`, both foreign to
  and in one case explicitly disallowed by this codebase's architecture.
- A pre-built kanban component is also the wrong shape for this feature: this board
  needs to render *this project's* `SessionCard`-equivalent, reuse `groupSessions()`
  for a switchable swimlane axis, and reuse `BulkActions.tsx` across columns
  (requirements.md:40-42) — an opinionated third-party board component would fight
  those integration points more than it would save, versus composing dnd-kit's
  primitives around the existing session-list rendering code.

**Verdict: Not recommended.**

## Option 2 (SaaS/managed API)

Not applicable. Drag-and-drop board interaction is inherently client-side UI state;
no hosted API/service performs this function. Noted per task instructions and moving on.

## Option 3 (LLM-generated implementation vs. battle-tested library) — explicit risk call

Hand-rolling drag-and-drop (whether via bespoke pointer-event tracking or the native
HTML5 API) means owning, from scratch, several problems each individually flagged as
a known hard UI category:
- **Accessibility**: keyboard-only reordering and screen-reader announcements for
  drag state changes are non-obvious to get right and easy to omit entirely under
  time pressure — `@dnd-kit`'s `KeyboardSensor` and built-in ARIA live-region
  announcements solve this by construction.
- **Touch/mobile**: requirements.md itself names touch drag-and-drop as a "known
  deep rabbit hole" (line 52) requiring an explicit Phase 3 scope decision
  (full touch / tap-to-move fallback / desktop-only v1). A hand-rolled
  implementation makes all three paths equally expensive to build; `@dnd-kit`'s
  `PointerSensor` gives the "full touch support" path materially cheaper, which
  changes the Phase 3 trade-off in favor of not preemptively cutting mobile.
- **Virtualization interplay**: requirements.md flags this explicitly (line 55) as
  "non-trivial." `@dnd-kit`'s `DragOverlay` pattern is the documented solution;
  reinventing it from scratch is strictly worse than adopting the pattern a widely-
  used library already ships.
- **Rollback/failure UX on a failed state-change RPC** (line 54) is application
  logic regardless of DnD library choice — this part is genuinely project-specific
  and neither `@dnd-kit` nor any alternative buys it away. It should be built here,
  wrapping whichever DnD primitive is chosen, not sourced.

**Verdict**: For the drag mechanics themselves, adopting `@dnd-kit` is lower-risk
than hand-rolling. For the failure/rollback UX around the state-change RPCs, that is
correctly scoped as build-it-yourself regardless of library choice — it's applying
existing session-control RPC error handling (already the pattern in
`SessionList.tsx` per requirements.md:66), not a drag-and-drop concern per se.

## Option 4 (Fork or adapt)

`BacklogBoard.tsx` / `board/page.tsx` (see "Current state" above) is the closest
existing pattern in this repo — a 5-column status board with loading/transition
states — but it is **not a fork candidate for the drag-and-drop mechanism itself**
since it has none. It IS a strong adaptation candidate for everything *except* drag:
column skeleton/empty states, the `EXIT_TRANSITION_MS`/`ENTER_FLASH_MS`
enter/exit-animation pattern for cards moving between columns (directly reusable
for post-drop card-settle animation), and the `columnCards`/`role="list"` structural
approach. Recommend the implementation phase reuse `BacklogBoard.tsx`'s column
shell/transition patterns and layer `@dnd-kit` drag mechanics on top, rather than
building either from a blank file.

## Summary Table

| Option | License | React 19 | Maintained | A11y/Touch | Verdict |
|---|---|---|---|---|---|
| `@dnd-kit/core` + `@dnd-kit/sortable` | MIT | Yes (open peer range) | Stable, last publish Dec 2024, not archived | Built-in keyboard + pointer/touch sensors | **Recommended** |
| `react-beautiful-dnd` | Apache-2.0 | No (`^18` cap) | Deprecated/archived by Atlassian | Keyboard yes, but virtualization-hostile | Not recommended |
| `@hello-pangea/dnd` (rbd fork) | Apache-2.0 | Yes | Actively maintained | Same virtualization limitation as rbd | Viable (fallback only) |
| Native HTML5 DnD API | N/A (no dep) | N/A | N/A | No touch support, no built-in a11y | Not recommended |
| `react-trello` | MIT | Untested/stale | Unmaintained since 2022, drags in redux + styled-components (disallowed by this repo's CSS rules) | Unknown/untested | Not recommended |
| SaaS/managed API | — | — | — | — | Not applicable |
| Fork existing repo code | — | — | — | — | No DnD to fork; adapt `BacklogBoard.tsx`'s column/transition shell only |

**Overall recommendation**: `@dnd-kit/core` + `@dnd-kit/sortable`, layered onto a
column/card shell adapted from `BacklogBoard.tsx`'s existing patterns, with
`DragOverlay` used to bridge the virtualization gap against whichever of
`react-virtuoso`/`@tanstack/react-virtual` the board columns end up using.
