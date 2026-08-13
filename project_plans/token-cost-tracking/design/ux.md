# UX Design — Token Cost Tracking Gap Closure (AC-1, AC-2, AC-3, AC-6)

Companion to `research/ux.md` (which this document builds on, not re-derives) and
`implementation/plan.md` (Phases 2, 3, 4 — this doc designs exactly those UI changes,
in implementation order of risk: low → high, matching the plan's phase ordering).

All four surfaces are **incremental additions to already-shipped screens** — no new
routes, no new modals beyond the existing drawer, no new navigation. Every wireframe
below shows only the delta against the current rendered output (confirmed by reading
each component's current source, not assumed).

---

## Surface (a) — `SessionsTable.tsx`: click-to-sort column headers (AC-3)

### Wireframe — header row, default (unsorted) state

```
┌────────────┬─────────┬──────────┬─────────┬─────────┬─────────┬──────────┐
│ Session    │ Model   │ Path     │ Input ↕ │ Output ↕│ Cache ↕ │  Cost ↕  │
├────────────┼─────────┼──────────┼─────────┼─────────┼─────────┼──────────┤
│ a1b2c3…    │ opus-4  │ ssq      │  12.3k  │   4.1k  │  62.0%  │  $0.41   │
│ orphan     │         │          │         │         │         │  unpriced│
└────────────┴─────────┴──────────┴─────────┴─────────┴─────────┴──────────┘
   ^^^^^^^^^^^^^^^^^^^^   not clickable, no aria-sort      ^^^^^^^^^^^^^^^^
                                                        clickable, aria-sort="none"
```

### Wireframe — after clicking "Cost" once (desc, default direction)

```
┌────────────┬─────────┬──────────┬─────────┬─────────┬─────────┬──────────┐
│ Session    │ Model   │ Path     │ Input ↕ │ Output ↕│ Cache ↕ │  Cost ↓  │  ← active, bold/underline optional
├────────────┼─────────┼──────────┼─────────┼─────────┼─────────┼──────────┤
│ high-cost  │ opus-4  │ ssq      │  81.0k  │  22.0k  │  40.0%  │  $2.14   │  ← highest cost first
│ mid-cost   │ sonnet  │ ssq      │  30.0k  │   9.0k  │  55.0%  │  $0.88   │
│ low-cost   │ haiku   │ ssq      │   4.0k  │   1.0k  │  70.0%  │  $0.03   │
│ unpriced-x │ gpt-5   │ ssq      │  10.0k  │   3.0k  │   0.0%  │  $0.00   │  ← unpriced badge
│            │         │          │         │         │         │ unpriced │     always LAST
└────────────┴─────────┴──────────┴─────────┴─────────┴─────────┴──────────┘
```

### Interaction flow

1. User tabs to (or clicks) the "Cost" `<th>`.
2. **1st activation** (click, or Enter/Space while focused): `sortCol` becomes `"cost"`,
   `sortAsc` defaults to `false` → table re-sorts descending (most expensive first),
   `aria-sort="descending"`, indicator changes from `↕` to `↓`.
3. **2nd activation on the same header**: `sortAsc` flips to `true` → ascending,
   `aria-sort="ascending"`, indicator `↑`. Cheapest-priced session first; unpriced
   sessions still sort last (not first), per the "unpriced ≠ $0" rule below.
4. **Activation on a different sortable header** (e.g. "Input"): `sortCol` switches,
   `sortAsc` resets to `false` (fresh descending sort on the new column). The
   previously-active header's indicator reverts to `↕`.
5. System responds instantly (client-side sort over already-fetched `SessionTokenSummary[]`
   — no network round trip, no loading state needed for the sort action itself).
6. Before any header is ever clicked (`sortCol === null`), the table's existing
   `lastMessageAt desc` order is preserved exactly as today — zero visual change for a
   user who never touches a header.

### Error / edge-case handling

| Case | Handling |
|---|---|
| Unpriced session (`unpricedModels.length > 0`) sorted by Cost | Always sorts last, in **both** asc and desc — comparator early-returns on the unpriced flag before applying the direction flip (plan's `sortCol === "cost"` special case). The existing `unpricedBadge` chip on that row is the user's explanation for why it's stranded at the bottom — no separate empty-state copy needed since the row still renders normally, just displaced. |
| Table already empty (no sessions match filters) | Unchanged — existing `empty` div ("No sessions match your filters" / "No sessions") renders before the table markup at all; sortable headers never appear over an empty table, so no incremental empty state is needed here. |
| Very large session list (virtualized via `TableVirtuoso`, >50 rows) | Sort happens once over the full `displayed` array before virtualization slices it for rendering — no per-scroll re-sort cost, no loading flicker. |
| Non-sortable columns (Session, Model, Path) | No `onClick`, no `aria-sort`, no `tabIndex` — visually and semantically inert, exactly as today. Prevents a false affordance (nothing about hovering these should suggest "click to sort"). |

### Exit path

N/A as a "flow" in the modal/wizard sense — this is a passive, always-reversible display
toggle. The "exit" is simply clicking the header again (toggles direction) or clicking a
different header (changes column); there is no state a user can get stuck in. No dead end.

---

## Surface (b) — `ModelBreakdownChart.tsx`: cache-hit-rate legend label (AC-6)

### Wireframe — legend row, current vs. new

```
Current:
┌──────────────────────────────────────────────────────┐
│ ● claude-opus-4        ● claude-sonnet-4 (pricing     │
│                            unavailable)                │
└──────────────────────────────────────────────────────┘

New:
┌──────────────────────────────────────────────────────────────────┐
│ ● claude-opus-4  72.4% cache hit    ● claude-sonnet-4  8.1% cache │
│                                          hit (pricing unavailable)│
└──────────────────────────────────────────────────────────────────┘
```

Per-family, appended after the existing `(pricing unavailable)` conditional, in the
existing muted/italic `legendItem` row — no new layout region, no new chart element.

### Interaction flow

Purely passive/informational — no click target, no state transition. User loads
`/insights`, glances at the bar chart, and now sees hit-rate context inline with each
model-family legend entry without needing to open a session drawer (today, hit rate is
only visible per-session in `SessionDetailDrawer.tsx`/`SessionsTable.tsx`).

### Error / edge-case handling

| Case | Handling |
|---|---|
| Model family with zero input+cache-read tokens (divide-by-zero) | Guarded client-side exactly like the existing Go `computeCacheHitRate`: `cacheReadTokens / (totalInputTokens + cacheReadTokens) \|\| 0` → renders "0.0% cache hit", not `NaN%` or a blank. |
| No models at all (`models.length === 0`) | Unchanged — existing early-return renders "No data" in `emptyChart`, before the legend row (and this new label) ever mounts. |
| Family already flagged `pricingUnavailable` | Both labels coexist in the same legend item (`72.4% cache hit (pricing unavailable)`) — cache hit rate is orthogonal to pricing availability (a model can have known cache behavior but unknown $/token), so suppressing one because of the other would hide real information. |

### Exit path

None needed — static, non-interactive label with no state to escape.

---

## Surface (c) — `SessionDetailDrawer.tsx`: "Per-Turn Breakdown" table (AC-1)

### Wireframe — drawer, new section inserted above "Tools Breakdown"

```
┌───────────────────────────────────────────────────┐
│  [a1b2c3…]  Session Details                    [×] │
├───────────────────────────────────────────────────┤
│  Metadata                                          │
│  Model: claude-opus-4      Total cost: $2.14       │
│  ...                                               │
├───────────────────────────────────────────────────┤
│  Per-Turn Breakdown                        ← NEW   │
│  ┌───────────┬────────┬───────┬────────┬────────┐ │
│  │ Timestamp │ Model  │ Input │ Output │ Cache  │ │
│  ├───────────┼────────┼───────┼────────┼────────┤ │
│  │ 14:32:08  │ opus-4 │ ⚠41.2k│ ⚠12.0k │  60.0% │ │  ← outlier: >2x mean
│  │ 14:31:05  │ opus-4 │  8.1k │   2.2k │  55.0% │ │
│  │ 14:30:41  │ opus-4 │  6.4k │   1.9k │  50.0% │ │
│  │ 14:29:58  │ opus-4 │  3.2k │   0.8k │  48.0% │ │
│  └───────────┴────────┴───────┴────────┴────────┘ │
│  (sorted by input+output tokens, descending)       │
├───────────────────────────────────────────────────┤
│  Tools Breakdown                                   │
│  ...                                               │
└───────────────────────────────────────────────────┘
```

### Wireframe — loading state (drawer just opened, RPC in flight)

```
│  Per-Turn Breakdown                                │
│  (no skeleton — see rationale below; renders empty │
│   state or table the instant the RPC resolves)     │
```

### Wireframe — empty state (no turn data)

```
│  Per-Turn Breakdown                                │
│  No per-turn data available for this session.      │
```

### Interaction flow

1. User clicks a session row (in `SessionsTable.tsx` or the session list) →
   `SessionDetailDrawer` opens (existing behavior, unchanged).
2. Drawer mounts with `session` set → `useSessionTurnTimeline(session.conversationId)`
   fires a `GetSessionTurnTimeline` RPC call lazily, scoped to this one session
   (not prefetched/batched with the list).
3. While the RPC is in flight: `turns` is `[]`, `loading` is `true`. The section
   renders **no visible content change** — see "loading state" design decision below.
4. On resolve: if `turns.length === 0`, render the empty-state paragraph. Otherwise,
   render the table, pre-sorted by `sortTurnsByTokensDesc` (highest-token turn first —
   "what spiked," not chronological — per `research/ux.md` §5's job-to-be-done finding).
5. User visually scans top-to-bottom; the highest-token (most expensive) turns are
   already at the top, and any turn exceeding 2× the session's mean total-tokens-per-turn
   is flagged with a warning-colored badge on its Input/Output cells.
6. User closes the drawer (× button, Escape key, or overlay click — all pre-existing) →
   drawer unmounts, turn data is discarded (no caching across drawer opens by design;
   re-opening re-fetches, acceptable since this is a point-in-time table per the plan).

### Loading-state design decision (deviating slightly from research's silence on this)

`research/ux.md` didn't specify a loading treatment. Recommendation: **no spinner, no
skeleton rows** — same "just don't show the section's content yet" treatment already
used by the rest of the drawer (which has no loading states at all today, because
`session` and its rollup fields are already fully loaded before the drawer opens). The
per-turn fetch is the first genuinely-async, drawer-triggered fetch in this component,
so there is no existing in-repo convention to copy — but the RPC is a single small
`GetByUUID` lookup against an in-memory `TokenStore` (no disk/DB read), so latency
should be sub-100ms in the overwhelming case; a skeleton would flash-and-vanish more
often than it would help. If manual testing during implementation shows a perceptible
delay (e.g. on a session with a very large JSONL), revisit with a lightweight text
placeholder (`"Loading per-turn data…"` in the `emptyState` class) rather than a full
skeleton table — flagged as a **conditional follow-up**, not blocking AC-1.

### Error / edge-case handling

| Case | Handling | Copy |
|---|---|---|
| No turn data at all (orphan session, JSONL not found, or `GetByUUID` returns nil) | Plain `<p className={emptyState}>`, matching the exact convention already used by "Tools Breakdown" (`"No tools recorded for this session."`) and "Skill Activations" — no skeleton, no spinner, no retry button. | `"No per-turn data available for this session."` |
| RPC itself errors (network/transport failure, not "found but empty") | `useSessionTurnTimeline`'s `error` state is available but, per this file's own established convention (`useInsightsService.ts`'s `console.error` pattern for `WatchInsights` stream errors), surfaced via `console.error` only — no user-visible error banner. This matches every other data-fetch failure mode already in this drawer (none of the existing `session.*` fields have a visible "failed to load" state; a failed per-turn fetch degrades gracefully to the same empty-state copy above, since `turns` stays `[]`). |
| Turn data present, but a turn legitimately used zero tokens | Not reachable — server-side, `<synthetic>` and zero-usage turns are already filtered out of `TurnTimeline` before serialization (confirmed: `parser.go:183,188`, `parser_test.go:153-155`). No client-side placeholder-row logic needed. |
| Very long session (50+ turns) | No pagination/virtualization in this pass (deliberately out of scope per plan — "adding filter UI now would be scope creep"). Table scrolls within the drawer's existing scroll container. Sorted-by-size default means the turns a user actually cares about (the expensive ones) are still above the fold without scrolling, even in a long session — this is the main usability payoff of the descending-by-cost default. |
| Subagent session (if `TurnTimeline` is structurally absent for these) | Same generic empty-state copy as "no turn data at all" — do not invent a distinct "subagent sessions unsupported" message without confirming (during implementation) that subagent JSONL is structurally different; if it turns out identical, this row is moot. |

### Outlier-highlight visual design — flagged contrast risk

The plan (Task 3.2.3b) proposes wrapping outlier Input/Output cell values in
`<span className={badgeVariant.warning}>`. Read `TokenBadge.css.ts` directly:
`badgeVariant` is built with `styleVariants()` and contains **only**
`background`/`color`/`borderColor` — the padding, `border-radius: full`,
`inline-flex`, `font-family: mono`, and `font-size: xs` all live on the **separate**
base `badge` style (`style({...})`), which callers combine as
`` `${badge} ${badgeVariant.warning}` `` (see `TokenBadge.tsx`'s actual usage, not
shown in the plan snippet). This is a concrete implementation-detail risk worth
flagging before coding starts:

- **If the base `badge` class is applied together with `badgeVariant.warning`**
  (correct usage): the outlier cell renders as a small rounded pill *inside* a table
  cell that already has its own `toolsTdRight`/`tdRight` padding. Two nested paddings
  plus a pill border inside a dense table row is visually heavier than the "Tools
  Breakdown" table's plain-text cells — acceptable for 1-2 outlier rows in a session,
  but if a session has many outliers, the table degenerates into "pill soup" that
  fights the "aesthetic and minimalist design" heuristic. **Recommendation**: apply
  `badgeVariant.warning`'s color/background only (skip the full pill chrome) via a
  new, narrower class — e.g. `outlierCell = style({ background: vars.color.warningBg,
  color: vars.color.warningText, borderRadius: vars.radii.sm, padding: \`0 4px\` })` —
  visually distinct as "this cell is flagged" without importing a badge's full pill
  affordance into a table-cell context it wasn't designed for.
- **If only `badgeVariant.warning` is applied without the base `badge` class**
  (the literal plan snippet, easy mistake): none of `badge`'s layout properties
  apply, so the span renders as inline text in `warningText` color with a
  `warningBg` background but **no padding** — background will hug the glyphs
  tightly with no breathing room, looking like a rendering bug rather than an
  intentional highlight. This is the failure mode to explicitly test for during
  implementation review.
- **Contrast itself** (not the layout risk above): `warningBg`/`warningText` pairs
  across the theme files checked (`theme.css.ts`) are all light-background/
  dark-text or dark-background/bright-text combos in the amber family (e.g. light
  theme `#fef3c7` bg / `#92400e` text) — these read as compliant AA-contrast pairs
  *when used together as background+text*, consistent with how `TokenBadge` already
  uses them today (already shipped, presumably already passed the design review that
  shipped it). The risk is **not** the color pair failing contrast on its own — it's
  losing the paired background if only the variant (not the base badge shell) is
  applied, which would leave `warningText`-colored text sitting on the table row's
  *own* background/hover-state color, a combination never contrast-tested because it
  was never intended to occur. **Action for implementation**: verify visually (or
  with an automated contrast check, e.g. Axe — already gated in CI per this repo's
  `web-app/src/` PR check) that the outlier cell's rendered background matches
  `warningBg`, not the table row's ambient background, before merging Task 3.2.3b.

### Exit path

Closing the drawer (existing × / Escape / overlay-click, all already implemented) is
the only exit needed — the per-turn table is read-only, non-modal-within-a-modal, and
introduces no new trap state (no filter/search UI in this pass means no "how do I
clear my filter" dead end either).

---

## Surface (d) — `SessionList.tsx`: "Sort: Cost" dropdown option (AC-2)

### Wireframe — sort controls, current vs. new

```
Current:
┌──────────────────────┐┌───┐
│ Sort: Last Activity ▾ ││ ↑ │
└──────────────────────┘└───┘

New:
┌──────────────────────┐┌───┐
│ Sort: Last Activity ▾ ││ ↑ │
│ Sort: Name            │└───┘
│ Sort: Created         │
│ Sort: Updated         │
│ Sort: Cost         ←NEW
└──────────────────────┘
```

Confirmed by reading `SessionList.tsx`: the direction toggle is **already a separate,
shared control** (`sortDirButton`, lines ~1129-1134) next to the field dropdown — AC-2
does not need a new direction control, only a new `<option>`. Selecting "Sort: Cost"
from the existing dropdown immediately reuses the existing ↑/↓ toggle button for
direction, exactly like the other three fields.

### Interaction flow

1. User opens the "Sort:" dropdown (existing `<select>`), selects "Sort: Cost".
2. `sortField` becomes `'tokenCost'`; the `sortedSessions` `useMemo` re-runs, invoking
   `compareSessionsByCost(a, b, costById, sortDir)` for every comparison.
3. List re-renders instantly, most-expensive-first by default (`sortDir` persists
   from whatever it was last, same as the other three fields — no forced reset).
4. User clicks the existing ↑/↓ button → `sortDir` flips → list re-sorts, with
   unpriced/not-yet-loaded sessions still pinned last in both directions.
5. **Background data dependency the other three sort fields don't have**: `costById`
   is populated from a *separate* `useInsightsSummary()` fetch that resolves
   independently of (and typically after) the `Session[]` list itself finishing its
   own, faster fetch. This means: user can select "Sort: Cost" **before** cost data
   has arrived.

### Wireframe — the "jumping list" moment (cost data arrives mid-view)

```
t=0s   Sort: Cost selected, costById still empty
       ┌─────────────────────────────────────┐
       │ session-A   (no cost badge)          │  ← all "missing" → stable
       │ session-B   (no cost badge)          │     original order (arbitrary,
       │ session-C   (no cost badge)          │     but doesn't visibly re-shuffle
       └─────────────────────────────────────┘     more than once)

t=0.4s costById resolves via GetInsightsSummary
       ┌─────────────────────────────────────┐
       │ session-B   $4.20                    │  ← moved up, now priced
       │ session-A   $0.90                    │  ← moved up, now priced
       │ session-C   (no cost badge, unpriced) │  ← stays last
       └─────────────────────────────────────┘
```

### Error / edge-case handling

| Case | Handling |
|---|---|
| Cost data not yet loaded (`costById.get(id) === undefined`) | Sorts **last**, in both asc and desc — `compareSessionsByCost`'s `aMissing !== bMissing` early return, evaluated *before* the `sortDir` flip (the specific bug class Pattern Decisions calls out: a sentinel value like `-1` fed into a generic comparator inverts position on direction flip; the early-return shape avoids that entirely). |
| Cost data loaded but genuinely unpriced (new/unknown model family) | Same trailing bucket as "not yet loaded" — both read as `costById.get(id) === undefined` today (Task 4.1.1a's join only inserts entries for summary rows; the plan does not distinguish "still loading" from "known unpriced" in the map itself). This is an acceptable simplification per the plan (both cases share one visual outcome: no cost shown, sorted last) but is a real UX seam worth naming: a user watching the list will see *some* sessions never resolve a price and permanently sit at the bottom, indistinguishable from "still loading" unless they check the session's own detail drawer for the `unpricedModels` badge. Not a blocker (AC-2 doesn't ask for a distinct label), but flagged for a possible future refinement — see below. |
| Two sessions both missing cost | Comparator returns `0` (stable order — neither jumps ahead of the other arbitrarily). |
| `SessionList.tsx` mounted with zero sessions | Existing empty state (unrelated to this change) — the sort dropdown itself doesn't need a special empty case; an empty list has nothing to sort. |
| `useInsightsSummary()` stream (`WatchInsights`) errors mid-session | Falls back to whatever `costById` was last populated with — degrades to "list stops updating cost-driven order" silently (matches the existing `console.error`-only failure mode for the `/insights` dashboard's identical hook usage — no new failure surface introduced by AC-2's second call site). |

### Recommended copy addition (not in the current plan text — flagged as a nice-to-have)

Since "still loading" and "confirmed unpriced" are visually identical today (both:
no cost shown, sorted last), and per Unresolved Question #1 in the plan (`TokenBadge`
is *not* being wired onto `SessionCard`/`SessionRow` in this pass), a user who selects
"Sort: Cost" will see some sessions sitting at the bottom with **no visible reason
why** — unlike `SessionsTable.tsx`, where the `unpricedBadge` chip is already on
screen right next to the price, explaining the row's position. Two options, both
outside AC-2's literal text so not blocking:
1. Do nothing further (ship as specified) — acceptable, since the badge/label gap is
   explicitly deferred in the plan's own Unresolved Questions, not silently missed.
2. If reviewer wants parity with `SessionsTable.tsx`'s self-explanatory badge, the
   lowest-effort fix is rendering the *existing, already-built* `TokenBadge` component
   next to session cards once `costById` is available — which is exactly Unresolved
   Question #1's proposed follow-up. Not designing net-new UI for this; pointing at
   the existing component is sufficient.

### Exit path

Selecting a different `<option>` from the same dropdown (all four pre-existing fields
remain available) is the exit — no trap, no modal, no unrecoverable state. A user who
picks "Sort: Cost" and finds it confusing (per the above ambiguity) can trivially
revert to "Sort: Last Activity" in one click.

---

## UX Acceptance Criteria (human-testable)

### Surface (a) — `SessionsTable.tsx` click-to-sort

- **UX-AC-a1**: User can sort the table by Cost in ≤1 click (click the "Cost" header),
  and toggle direction in ≤1 further click (click "Cost" again).
- **UX-AC-a2**: Each of the 4 sortable headers (Input, Output, Cache, Cost) is
  reachable via Tab and activatable via Enter or Space while focused — verify with
  mouse unplugged / keyboard-only pass.
- **UX-AC-a3**: `aria-sort` is present and correct (`"ascending"` / `"descending"` /
  `"none"`) on all 4 sortable `<th>` elements at every state — verify via browser
  devtools accessibility tree or axe.
- **UX-AC-a4**: Non-sortable headers (Session, Model, Path) have no `aria-sort`, no
  `tabIndex`, no click handler — verify they do not appear in the keyboard tab order
  between sortable headers.
- **UX-AC-a5**: A session with `unpricedModels.length > 0` appears as the visually
  last row when sorted by Cost, in both ascending and descending order — verify by
  toggling direction twice with at least one unpriced and one priced session present.
- **UX-AC-a6**: Before any header is clicked, table order is pixel-identical to
  today's `lastMessageAt desc` order — verify via before/after screenshot diff on an
  unmodified fixture.
- **UX-AC-a7**: Sort indicator glyph (`↕` / `↑` / `↓`) is visible and legible at the
  default table font size — not clipped, not overlapping adjacent header text.

### Surface (b) — `ModelBreakdownChart.tsx` cache hit rate label

- **UX-AC-b1**: Every model-family legend entry shows a cache-hit-rate percentage
  alongside its name, without requiring a click or hover.
- **UX-AC-b2**: A model family with zero cache-eligible tokens shows "0.0% cache hit",
  never `NaN%`, `Infinity%`, or a blank string.
- **UX-AC-b3**: The "(pricing unavailable)" label and the cache-hit-rate label
  coexist legibly on one legend line without visual collision, at both desktop and
  the narrowest supported viewport width (check `web-app/src/app/insights` responsive
  behavior — this repo's CI already gates Lighthouse/Axe on `web-app/src/` PRs).

### Surface (c) — `SessionDetailDrawer.tsx` per-turn breakdown

- **UX-AC-c1**: Opening the drawer for a session with turn data shows a "Per-Turn
  Breakdown" table within a perceptibly instant time (no visible loading flicker for
  a typical session) — verify by manual timing on a representative session.
- **UX-AC-c2**: Opening the drawer for a session with **no** turn data shows the exact
  copy `"No per-turn data available for this session."`, styled identically to the
  existing "No tools recorded for this session." message (same class, same
  positioning) — no dead-end call-to-action needed since this is a passive display
  panel with no action to retry or configure.
  - AC-1 asks only for a table that renders "for sessions where turn data is
    available" — this empty-state copy is the explicit, in-scope handling for the
    complementary case (turn data unavailable), not scope creep.
- **UX-AC-c3**: Turns are ordered highest-token-first by default (verify against a
  fixture with turns of varying size — the largest-total-token turn must render in
  row 1).
- **UX-AC-c4**: A turn whose total tokens exceed 2× the session's mean is visually
  distinguishable from non-outlier rows at a glance (not requiring the user to read
  and compare numbers) — verify the flagged cell's background is visibly different
  from an unflagged cell's background in the rendered DOM (not just a color-only
  change with no background, per the contrast risk flagged above).
- **UX-AC-c5 (accessibility, contrast)**: The outlier highlight's background/text
  pairing meets WCAG AA (4.5:1 for the mono/small table text) **as actually rendered
  in the table cell** — verify with an automated contrast checker (axe, or browser
  devtools) against the live DOM, not against `TokenBadge.tsx`'s own isolated
  rendering. This check must specifically confirm the cell has a *visible background
  fill*, not just colored text sitting on the table's ambient row background (the
  concrete risk identified above if `badgeVariant.warning` is applied without a
  matching layout/background wrapper).
- **UX-AC-c6**: Closing the drawer (×, Escape, or overlay click) works identically
  whether or not the per-turn table has loaded/rendered — no regression to the
  existing close behavior.

### Surface (d) — `SessionList.tsx` "Sort: Cost"

- **UX-AC-d1**: User can sort the main session list by cost in ≤1 action (select
  "Sort: Cost" from the existing dropdown) and toggle direction in ≤1 further click
  (existing ↑/↓ button — already shared across all sort fields, confirmed in source).
- **UX-AC-d2**: A session with no resolved cost (still loading, or genuinely
  unpriced) never appears interspersed among priced sessions — always trails the
  full priced set, in both sort directions — verify by selecting "Sort: Cost"
  immediately on page load (before the cost fetch resolves) and observing the list
  does not visibly reorder more than once as data streams in.
- **UX-AC-d3**: Selecting "Sort: Cost" does not throw, does not blank the list, and
  does not lose the user's current filter/search state (status, category, tag
  filters) — sort and filter must compose independently, matching the other 3 sort
  fields' existing behavior.
- **UX-AC-d4**: `sortField: 'tokenCost'` persists across a page reload exactly like
  the other 3 fields (verify `localStorage`/`STORAGE_KEYS.SORT_FIELD` round-trips the
  new value) — regression check on the existing persistence mechanism, not new
  behavior to design.

---

## Summary — flow completeness check

| Surface | Entry point | Exit path | Dead end? |
|---|---|---|---|
| (a) SessionsTable sort | Click/keyboard-activate a header | Click again (toggle) or click a different header — always reversible | No |
| (b) ModelBreakdownChart label | Passive (page load) | N/A — no interactive state to exit | No |
| (c) Per-turn breakdown | Open session drawer | Close drawer (×/Escape/overlay) — pre-existing, unaffected by table load state | No |
| (d) SessionList "Sort: Cost" | Select from existing dropdown | Select a different option from the same dropdown | No |

Every flow has an exit path, and none introduces a state the user cannot back out of
in one action. No dead ends identified across any of the four surfaces.
