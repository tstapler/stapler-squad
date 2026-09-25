# Architecture Research: session-list-density

## 1. Shared truncation logic: pure utility, not a hook

**Recommendation: a pure function, following the existing precedent — do not add a hook or ResizeObserver.**

The codebase already has exactly this pattern: `truncateMiddle(name, maxLen)`
(`web-app/src/lib/utils/truncateMiddle.ts:6`) — middle-truncates a string to a
caller-supplied character budget, preserving a trailing suffix (there, a file
extension). It takes a plain `number`, not a measured width, and is called
from `FileTree.tsx` and `WorkspaceSwitcher.tsx` with a `maxLen` the caller
already knows from its own layout (column width ÷ approximate char width, or
a fixed constant) — no `ResizeObserver` anywhere in that call chain.

Build the new path-truncation logic (collapsing opaque middle segments —
workspace hash, worktree UUID — while preserving repo/branch name) the same
way: a new pure function, e.g. `truncateWorkspacePath(path: string, maxLen: number): string`
in `web-app/src/lib/utils/` (new file, e.g. `truncateWorkspacePath.ts`,
sibling to `truncateMiddle.ts` — or add it to that file if the segment-aware
logic is small enough to share the ellipsis/budget math). Both
`SessionRow.tsx` and `SessionCard.tsx` import it directly; no context,
provider, or hook needed since there is no width *measurement* step — the
`maxLength` is a caller-supplied prop derived from either a fixed column
width (List row, single line) or a CSS `@container` breakpoint choosing
between one of a few fixed budgets (Card, per size class), not a live
pixel measurement.

Rationale for rejecting `useSmartTruncate` + `ResizeObserver`: the two
existing consumers of `SessionRow`/`SessionCard` truncation
(`abbreviatePath`, `SessionRow.tsx:168-170`) already work off `session.existingDir`
with no live-width dependency — the design constraint here is "fit within N
characters for this known column/breakpoint," not "fit exactly to whatever
pixel width the browser gives me right now." A `ResizeObserver` hook adds a
layout-effect + re-render cycle for a problem CSS custom properties / a
handful of `@container` breakpoints already solve more cheaply, and there is
no existing `ResizeObserver`-based *hook* precedent to extend — the seven
files matching `ResizeObserver` (`useSplitContainerSize.ts`,
`useResizeSettling.ts`, `TerminalOutput.tsx`, `XtermTerminal.tsx`, etc.) all
solve terminal/pane pixel-sizing, a genuinely different problem (xterm.js
needs real pixel dimensions to lay out a character grid) — not a template to
reuse here.

## 2. session-columns.ts / elapsed 2nd-line integration

**No change needed to the `ColumnDef`/`ColumnKey` abstraction. Visibility
stays boolean; position moves entirely in CSS/JSX, not in the column-picker
data model.**

Current shape (`web-app/src/components/sessions/session-columns.ts:1-22`):
```ts
export type ColumnKey = "agent" | "memory" | "elapsed" | "diff" | "branch";
export interface ColumnDef {
  key: ColumnKey;
  label: string;
  gridWidth: string;       // consumed by buildRowGridTemplate()
  defaultVisible: boolean;
}
```
`elapsed`'s `defaultVisible` should stay `true` — the requirement is "always
show elapsed, but on a 2nd line," not "hide it behind the Columns picker."
`gridWidth` only matters for columns still rendered as a **grid cell** in the
1-line-row layout; moving `elapsed` to a 2nd line means `SessionRow.tsx`
stops placing it in the grid column matching `elapsed`'s `gridWidth` and
instead renders it inside a second flex/block row beneath the main grid row
(same pattern the requirements doc's "variable row height" already implies).
`buildRowGridTemplate()` (`session-columns.ts:29-38`) would need one line
changed: skip `elapsed` when iterating `COLUMN_DEFS` for the grid template
(or split `COLUMN_DEFS` into "grid columns" vs. "always-2nd-line" — a `layout:
"grid" | "secondary-line"` field is the minimal extension if a *third*
placement is ever needed, but for this feature a one-line `if (def.key ===
"elapsed") continue;` in the template builder, paired with hardcoding the
2nd-line `elapsed` render in JSX, is enough and keeps the `ColumnDef`
abstraction unchanged).

The Columns picker (`ColumnPicker.tsx`, referenced from `SessionList.tsx:22`)
keeps working unmodified either way, since it only reads/writes
`ColumnKey`/`defaultVisible` — it has no opinion on *where* a visible column
renders.

## 3. Virtualization: already variable-height-capable in both modes — low risk

`SessionList.tsx` runs **two different virtualizers depending on
`viewMode`** (confirmed via import grep, `SessionList.tsx:5-6`):

- **List/row mode**: `@tanstack/react-virtual`'s `useVirtualizer`
  (`SessionList.tsx:646-648`):
  ```ts
  estimateSize: (i) => (flatItems[i]?.kind === "header" ? 40 : 50),
  overscan: 8,
  measureElement: (el) => el.getBoundingClientRect().height,
  ```
  `measureElement` is already wired (also referenced at
  `SessionList.tsx:1190` as a ref callback on each rendered row). This means
  the virtualizer **already treats row height as dynamic** — `estimateSize`
  is only a first-paint guess; the real height is measured from the DOM
  after render and the virtualizer's internal offsets are corrected
  automatically. Making `SessionRow` wrap to 2 lines (name overflow) or grow
  a 2nd line (elapsed) does not require new virtualizer wiring — it's exactly
  the scenario `measureElement` exists for. The only real risk is scroll-jank
  during the measure-and-reflow correction if row height varies a lot
  session-to-session; `overscan: 8` already buffers for this.
- **Card/board mode**: `react-virtuoso`'s `GroupedVirtuoso`
  (`SessionList.tsx:6`, rendered per `SessionList.tsx:1366`'s comment).
  Virtuoso measures each item's real DOM height per-item natively (that's
  its core design, unlike `react-window`) — no `estimateSize`/fixed-height
  assumption exists in this codebase for card mode to begin with, so
  variable-height `SessionCard`/`BoardCard` content is a non-issue.

**Conclusion: the "variable row height + virtualization" feasibility risk
flagged in requirements.md is resolved — both virtualizers already support
it.** No virtualizer-facing code changes are anticipated; the work is
confined to `SessionRow`/`SessionCard`'s own JSX/CSS.

## 4. Container queries with vanilla-extract: precedent already exists

`FilesTab.css.ts:9-10` already does exactly what
`SessionRow.css.ts`/`SessionCard.css.ts` would need:
```ts
export const container = style({
  containerType: "inline-size",
  containerName: "filesTab",
  ...
});
...
"@container": {
  [`filesTab ${NARROW}`]: { /* narrow-layout overrides */ },
}
```
This is a working, shipped vanilla-extract + CSS container-query pattern in
this exact package (`web-app`, same vanilla-extract version). The
requirements doc's "Container-query support/tooling with vanilla-extract —
not confirmed" risk is resolved: copy `FilesTab.css.ts`'s
`containerType`/`containerName`/`"@container"` shape onto `SessionRow.css.ts`
(new `containerName`, e.g. `"sessionRow"`) rather than inventing a new
approach.

## 5. Complexity/churn hotspot signal

No kibitzer hotspot (complexity × churn ranking) data exists for this repo
yet — `tstapler/kibitzer#15` (cited in this repo's own root `CLAUDE.md`) is
the tracking issue for that feature and it isn't implemented. Falling back to
raw complexity signals via `mcp__kibitzer__architecture_assessment` (scoped
to `web-app/src/components/sessions/**`) surfaced:

- `SessionCard.tsx:244` — the main component body **spans 1015 lines** as a
  single function (`long-function`, `deep-nesting` to 5 levels).
- `SessionRow.tsx:183` — main render body **spans 483 lines**
  (`long-function`).
- `SessionList.tsx:285` — a `long-function` finding recurring 4x (60-line
  bodies), plus a `deep-nesting` finding at `:346` (5 levels, ×3 more
  elsewhere).

These are pre-existing `[advisory]`-severity findings (not blocking), but
they corroborate the requirements doc's own file-size risk signal (694/1554
lines) with a complexity dimension: the render bodies are not just long
files, they are long *functions* with deep conditional nesting — the kind of
code where inserting new inline logic (a new grid-vs-2nd-line branch, a new
truncation call site, a new container-query class toggle) is easy to get
subtly wrong (wrong branch, wrong nesting level) and hard to review by diff
alone.

## 6. Disposition: Isolate via seam

**Recommend: Isolate via seam — do not refactor `SessionRow.tsx`/`SessionCard.tsx`/`SessionList.tsx` first, and do not extend by piling more inline logic into their existing giant render functions either.**

Rationale:
- The truncation logic (§1) and the CSS layout mechanics (§4) are naturally
  extractable as *new*, small, independently-testable units
  (`truncateWorkspacePath.ts`, new `SessionRow.css.ts`/`SessionCard.css.ts`
  container-query classes) that the existing giant components merely *call*
  — this is the seam. It adds no burden to first untangle the 483-/1015-line
  render bodies, and it doesn't add to their complexity debt either, since
  the new logic lives outside them.
- `session-columns.ts` (§2) is already a clean, small, well-isolated module
  (38 lines, no findings) — extending it in place is fine and matches its
  existing "small pure data + one builder function" shape; no seam needed
  there.
- The row/card JSX edits *inside* `SessionRow.tsx`/`SessionCard.tsx` (2nd
  line for `elapsed`, wrap instead of ellipsis, dropping agent/memory from
  the default grid, touch-target sizing) are unavoidably inline changes to
  the existing long functions — but they're narrow, mechanical edits (JSX
  restructuring + class swaps) guided by the new small utilities/CSS classes
  from the seam above, not new business logic grafted into the 400-1000-line
  bodies. This keeps the diff reviewable without requiring an
  Extract-Function refactor of the whole component first, consistent with
  the requirements doc's Complexity-2 framing ("focused feature,
  presentation-layer only").
- A `Refactor-first` disposition (breaking up `SessionCard`'s 1015-line body
  before touching it) would be disproportionate to a Complexity-2 feature
  and risks scope creep the requirements doc explicitly rules out ("Out of
  Scope: other sidebar UI"). `Extend as-is` (bolt everything on inline with
  no new files) would add to the existing `long-function`/`deep-nesting`
  debt in files kibitzer already flags. `Isolate via seam` gets the benefit
  of small, testable, reviewable new units without taking on an
  unrequested refactor.

## 7. Board/Card reuse — confirms requirements.md's "does BoardCard suffer the same problem?" question

Per prior research already on file for a related feature
(`project_plans/board-kanban-view/research/architecture.md:172-180` and
`project_plans/kanban-board-view/research/architecture.md:226-229`):
`BoardCard.tsx` is a **thin wrapper** around `SessionCard` — confirmed again
directly in this session (`BoardCard.tsx:35-71`): it adds only a drag handle
and a `MoveToMenu`, then renders `<SessionCard {...sessionCardProps} />`
unchanged inside `cardBody`. **Any column-trimming/truncation change made to
`SessionCard.tsx` automatically applies to the board view with no separate
`BoardCard.tsx` edit required** — the requirements doc's "Does BoardCard
actually suffer the same wasted-space problem?" is answered yes, and the fix
surface is `SessionCard.tsx` alone.
