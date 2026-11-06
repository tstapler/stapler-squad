# Stack Research: session-list-density

## 1. CSS container queries in vanilla-extract

**No new dependency needed.** The codebase already has a working, in-repo precedent:
`web-app/src/components/sessions/FilesTab.css.ts` uses vanilla-extract's native
`@container` at-rule support directly — no plugin or wrapper package:

```ts
export const container = style({
  containerType: "inline-size",
  containerName: "filesTab",
});

const NARROW = "(max-width: 767px)";

export const treePane = style({
  "@container": {
    [`filesTab ${NARROW}`]: { width: "100% !important" },
  },
});
```

`@vanilla-extract/css` is `^1.20.1` (`web-app/package.json`) — container-query
support in vanilla-extract's `style()` at-rule map has existed since well before 1.0;
no version bump required. Pattern to reuse for `SessionRow.css.ts` /
`SessionCard.css.ts`: give the row/card a `containerType: "inline-size"` +
`containerName`, then gate narrow-layout overrides behind `@container` blocks keyed
to that name, exactly as `FilesTab.css.ts` does. This directly retires the
"Rabbit Hole" flagged in requirements.md ("Container queries are new to this
codebase... need a spike") — it isn't new, it's an established pattern with one
existing consumer to copy from.

Browser support caveat (worth one line in the plan, not a spike): CSS container
queries (`@container` + `container-type`) have been supported in all evergreen
browsers (Chrome/Edge 105+, Firefox 110+, Safari 16+) since early 2023. No
`.browserslistrc` was found in `web-app/` constraining below that baseline, and
this is a personal single-user tool (per requirements.md Constraints), so no
polyfill or fallback is warranted.

## 2. Smart/dynamic (middle-ellipsis) text truncation in React

**No new dependency needed — a pure utility function already exists and is proven
in production use.** `web-app/src/lib/utils/truncateMiddle.ts`:

```ts
export function truncateMiddle(name: string, maxLen: number): string {
  // preserves head + extension/tail, ellipsis in the middle
  // e.g. truncateMiddle("very-long-filename.tsx", 18) → "very-lon…name.tsx"
}
```

Currently consumed by `FileTree.tsx` (`src/components/sessions/FileTree.tsx:9,356`)
to truncate filenames in the file browser tree. It's a plain, dependency-free
string function (no DOM measurement, no ResizeObserver) driven by a caller-supplied
`maxLen` — which the caller derives from available width (e.g. via a resize
handler or container query breakpoint).

Recommendation: **extend/reuse `truncateMiddle`, don't add a library.** The
common small libraries for this (`react-text-truncate`, `react-truncate-markup`,
`fittruncate`) are either unmaintained, DOM-measurement-heavy (expensive inside a
virtualized list — see §3), or solve string-anywhere truncation rather than the
specific "preserve meaningful segments" rule this feature needs (workspace hash,
worktree UUID suffix, branch name). None is "battle-tested" enough to justify a
new dependency over the existing in-repo utility, especially given the
requirement's own "Rabbit Hole" warning about scope creep into a general
path-formatting utility — a concrete, testable, path-aware rule layered on top of
`truncateMiddle` (e.g. split on `/`, collapse an opaque hex/UUID-looking middle
segment first, keep first path segment + branch/session name + filename) is a
better fit than pulling in a generic truncation library. There's also a distinct
existing utility, `truncateGoal` (`src/lib/utils/string.ts`, used in
`SessionRow.tsx:56` and `SessionCard.tsx:197`), which does simple end-truncation
for note/goal text — leave that one alone; it's for free-text notes, not
paths/names, and the requirement only calls out path truncation.

Existing display of names/paths uses plain CSS `text-overflow: ellipsis;
white-space: nowrap` (e.g. `SessionRow.css.ts:115-116, 132-133, 280-281,
425-426`) — replacing this with computed `truncateMiddle`-based text (or removing
truncation in favor of wrapping, per requirements' "name/path wrap instead of
ellipsis") is a straightforward swap, not an architecture change.

## 3. List virtualization in SessionList.tsx — compatibility constraint

**Two virtualization libraries are both already in use, and variable-height rows
must be compatible with both:**

- `@tanstack/react-virtual` (`^3.13.25`) — used directly via `useVirtualizer` in
  `SessionList.tsx:5,643`, with a **fixed** `estimateSize` callback:
  ```ts
  const rowVirtualizer = useVirtualizer({
    ...
    estimateSize: (i) => (flatItems[i]?.kind === "header" ? 40 : 50),
  });
  ```
  This is List view's row virtualizer. `estimateSize` is only an *estimate* —
  `@tanstack/react-virtual` v3 supports genuinely dynamic/variable row heights via
  `virtualizer.measureElement` (a ref callback attached to each rendered row) which
  re-measures actual DOM height after render and corrects scroll offsets. This is
  the mechanism to adopt for "variable row height" per requirements.md — no version
  bump needed (dynamic measurement has been in v3 since its initial release), but
  it does require attaching `ref={rowVirtualizer.measureElement}` to each row's
  root DOM node and switching `estimateSize` to a real estimate rather than a
  hardcoded constant, since row height will now vary with wrapped name/path text.

- `react-virtuoso` (`^4.18.7`) — used for Board (card) view via `GroupedVirtuoso`
  (`SessionList.tsx:6,1367`). Virtuoso's core design already assumes and measures
  variable-height items out of the box (it does not require a fixed/estimated size
  callback the way react-window does) — so Board view's card-height variability
  from the redesign is lower-risk than List view's; it needs no library-level
  change, just correct card markup.

**No `react-window` or `react-virtual` (v2/legacy) present** — confirmed via repo
grep; only the two libraries above are in `package.json`/imported. Do not add
`react-window`: it would introduce a third virtualization approach and only
supports fixed or externally-computed variable sizes (`VariableSizeList`) with
manual cache-invalidation calls (`resetAfterIndex`) — strictly more manual work
than what `@tanstack/react-virtual`'s `measureElement` already gives this codebase
for free.

**Feasibility risk validated:** requirements.md's flagged risk ("variable-height
rows + virtualization interaction") is real but has a known, idiomatic solution
already available in the installed version — `measureElement` — not a version
upgrade or new dependency. The remaining implementation risk is scroll-jump/jitter
during height transitions (e.g. expanding a row), which `measureElement`-based
re-measurement handles but should be verified manually (or via an e2e test) once
rows can wrap to multiple lines.

## Summary of dependency changes needed

| Need | New dependency? | Approach |
|---|---|---|
| Container queries | No | Native vanilla-extract `@container`, pattern already in `FilesTab.css.ts` |
| Smart truncation | No | Extend `src/lib/utils/truncateMiddle.ts` with a path-aware segment rule |
| Variable-height virtualized rows (List) | No | `@tanstack/react-virtual`'s `measureElement` API (already installed, v3) |
| Variable-height virtualized cards (Board) | No | `react-virtuoso`'s `GroupedVirtuoso` already handles variable heights |

**Net result: zero new npm dependencies required for this feature.** Everything
needed is either a native browser/vanilla-extract CSS feature or an existing
utility/library already in `web-app/package.json`.
