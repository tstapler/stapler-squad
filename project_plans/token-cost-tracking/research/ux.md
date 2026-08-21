# UX Research — Token Cost Tracking Gaps (AC-1, AC-2, AC-3, AC-6)

## 1. Comparable in-codebase patterns

### Expandable/detail table pattern (for AC-1: per-turn breakdown)

`SessionDetailDrawer.tsx` already has the exact pattern to clone for a per-turn table:
the **"Tools Breakdown"** section (lines 136–160):

```tsx
<div className={section}>
  <h3 className={sectionTitle}>Tools Breakdown</h3>
  {session.topTools.length === 0 ? (
    <p className={emptyState}>No tools recorded for this session.</p>
  ) : (
    <table className={toolsTable}>
      <thead><tr>...</tr></thead>
      <tbody>{session.topTools.map((t, i) => <tr key={i}>...</tr>)}</tbody>
    </table>
  )}
</div>
```

Reuse `toolsTable`/`toolsTh`/`toolsTd`/`toolsTdRight`/`emptyState` classes from
`SessionDetailDrawer.css.ts` for a new "Per-Turn Breakdown" section directly below
it (or above Tools Breakdown, since it's the more actionable data) — same
empty-state convention (`<p className={emptyState}>`), same table styling, zero new
CSS needed.

**Blocking gap for AC-1**: `SessionTokenSummary` (proto) does **not** carry
`TurnTimeline` — the Go struct `tokens.ParseResult.TurnTimeline []TurnStats`
(`session/tokens/types.go:20`) exists and is populated by the parser, but nothing in
`insights_service.go` or `insights.proto` serializes it into the RPC response. This
is not purely a frontend wiring gap as the requirements doc states — it needs:
1. A new proto message (e.g. `TurnEntry`) + a `repeated TurnEntry turn_timeline`
   field, most naturally on a **new dedicated RPC** (`GetSessionTurns` or similar)
   rather than bloating `SessionTokenSummary` for the list view, since turn-level
   detail is only needed when a drawer is open for one session. `ListSessionTokens`
   / `GetInsightsSummary` should NOT carry it — those already return N sessions at
   once and turn arrays would balloon payload size for no benefit (all sessions in
   list view never render turns).
2. `make proto-gen` + a handler in `insights_service.go` that reads
   `TurnStats{Timestamp, Model, Input, Output, CacheCreation, CacheRead, ToolNames}`
   and maps 1:1 to the new message.
3. Frontend: fetch turns on-demand when the drawer opens (lazy — matches "Tools
   Breakdown" which is already eagerly embedded per-session, but turn timelines can
   be large for long sessions, so lazy-fetch-on-open is the safer default; the
   `WatchInsights` streaming architecture note in the requirements doesn't apply
   here — this is a point-in-time table, not a live feed).

### Click-to-sort column pattern (for AC-3)

**No shared `useSortableTable` hook exists.** Two independent, hand-rolled
implementations of the same pattern already live in this codebase — pick the
`app/backlog/page.tsx` variant as the template since it is the more complete/correct
one (see accessibility section below):

- `web-app/src/app/backlog/page.tsx` (lines 40, 302–304, 490–505, 510–515,
  619–621, 740–750) — `SortColumn` union type, `sortCol`/`sortAsc` state,
  `handleSortClick(col)` toggles direction on re-click, `sortIndicator(col)` renders
  `↑`/`↓`, and **each `<th>` sets `aria-sort`**.
- `web-app/src/components/sessions/ApprovalRulesPanel.tsx` (lines 10–11, 109–110,
  164–205, 508–528) — near-identical `SortKey`/`SortDir` state shape,
  `handleSort(key)`, `sortIcon(key)` (uses `↕` for the inactive/unsorted state,
  a nicer touch than backlog's blank), but **no `aria-sort` attribute** — a
  regression relative to the backlog page's version.

Recommendation for `SessionsTable.tsx`: replicate the `backlog/page.tsx` shape
(`SortColumn` type = `"input" | "output" | "cache" | "cost"`, `sortCol`/`sortAsc`
state, `handleSortClick`, `aria-sort` on every sortable `<th>`) but borrow
`ApprovalRulesPanel`'s `↕` neutral-state icon for a clearer affordance on
un-sorted columns. Since this is now the third occurrence of the identical
state-machine, it's a reasonable candidate to extract into a shared
`useSortableTable<T>()` hook afterward — flagging as a nice-to-have, not blocking
AC-3, since the acceptance criteria only asks for `SessionsTable.tsx` sortability
and duplicating the ~15-line pattern a third time is consistent with existing
codebase practice.

One SessionsTable-specific wrinkle: the table already has a hardcoded
`.sort((a,b) => bt - at)` by `lastMessageAt` inside the `displayed` `useMemo`
(lines 97–101) that runs *after* search/filter. Click-to-sort needs to replace
that fixed comparator with one driven by `sortCol`/`sortAsc`, defaulting to the
current `lastMessageAt desc` behavior so existing users see no change until they
click a header.

## 2. User mental model — "tokens" vs "cost" labeling

Checked `TokenBadge.tsx` and `SummaryCards.tsx` for existing label conventions:

- `TokenBadge.tsx` renders **cost only** (`fmtCost`), used on session cards in
  `SessionList.tsx`. Its prop is literally named `costUsd`, and its tooltip says
  "Estimated cost: $X". There is no token-count badge anywhere in the session-card
  UI today — cost is the only per-session number surfaced outside `/insights`.
- `SummaryCards.tsx` (the `/insights` dashboard) shows **four distinct, separately
  labeled cards**: "Total Cost" (fmtCost), "Input Tokens" (fmtTokens), "Output
  Tokens" (fmtTokens), "Cache Hit Rate" (fmtPct) — i.e. this codebase's existing
  convention never conflates "tokens" and "cost" as one number; they're always
  presented as siblings.

**Implication for AC-2** (`SessionList.tsx` sort dropdown): given `TokenBadge` is the
only token-related UI already on session cards and it displays **cost**, a sort
option literally labeled "Sort: Tokens" would be ambiguous (total = input+output+cache?
just input+output?) and inconsistent with what's visually on the card the user is
scanning. Label the new option **"Sort: Cost"** (matching `TokenBadge`'s displayed
metric, sorted by `estimatedCostUsd` descending as default direction — mirroring
`ApprovalRulesPanel`'s convention of defaulting numeric/"bigger is more interesting"
columns to `desc`, unlike the alphabetic fields which default `asc`). If a raw-token
option is wanted later, it should be a second, separately labeled option ("Sort:
Total Tokens") rather than overloading one "tokens" label — but the requirements'
own AC-2 text ("Token usage" or "Cost") only asks for one, and "Cost" is the
better match to what's already on-screen in `TokenBadge`.

## 3. Accessibility — click-to-sort headers

Confirmed pattern already exists correctly in this codebase — **mirror
`app/backlog/page.tsx`, not `ApprovalRulesPanel.tsx`**:

```tsx
<th
  scope="col"
  className={styles.tableHeaderCell}
  onClick={() => handleSortClick("title")}
  style={{ cursor: "pointer" }}
  aria-sort={sortCol === "title" ? (sortAsc ? "ascending" : "descending") : "none"}
>
  Title{sortIndicator("title")}
</th>
```

Gaps even in the backlog reference implementation worth closing in the new
`SessionsTable.tsx` implementation (don't copy these two flaws forward):
- **No keyboard activation.** The `<th>` has `onClick` but no `tabIndex={0}`,
  `role="button"`, or `onKeyDown` for Enter/Space — a mouse-only sort control,
  which fails WCAG 2.1.1 (Keyboard). `SessionsTable.tsx` itself already has the
  correct keyboard pattern one level down, for row activation
  (`handleRowKeyDown`, `tabIndex={onSessionClick ? 0 : undefined}`,
  `role="button"` on `<tr>`, lines 111–116, 184–187) — apply that same
  Enter/Space `onKeyDown` + `tabIndex={0}` + `role="button"` treatment to the
  sortable `<th>` elements too. Per `.claude/rules/css-architecture.md`, use a
  vanilla-extract class (already have `thRight`/`th` in `SessionsTable.css.ts`)
  rather than the backlog page's inline `style={{ cursor: "pointer" }}`.
- `aria-sort="none"` should be set explicitly on all sortable headers (not
  omitted) even when unsorted, matching backlog's ternary — confirmed correct
  there, keep it.
- Non-sortable columns (`Session`, `Model`, `Path` in `SessionsTable.tsx`) should
  not receive `aria-sort` or the click handler at all — only `Input`/`Output`/
  `Cache`/`Cost` need it per AC-3's "at least one" requirement, though doing all
  four is cheap given the shared handler.

## 4. Error/empty states

### Per-turn breakdown (AC-1) empty/missing states

Three distinct cases, and they should not collapse into one generic empty state:
1. **No turn data available at all** (orphan sessions, or JSONL not found) —
   `SessionTokenSummary.is_orphan` already flags this at the session level, and
   `emptyState` class + copy pattern already exists ("No tools recorded for this
   session."). Use the same treatment: `"No per-turn data available for this
   session."` Do not show a table skeleton — mirrors the Tools Breakdown
   convention of a plain `<p>`, not a spinner or greyed table.
2. **Turn data present but a specific turn used zero tokens** (the codebase
   already special-cases this — `insights_service.go:238` comment: "a zero-usage
   turn has..." — implying the parser/service intentionally filters certain
   zero-usage turns from `TurnTimeline` in production, e.g. synthetic messages;
   `parser_test.go:153` confirms `<synthetic>` turns are excluded). Since
   filtering already happens server-side, the frontend table only ever sees turns
   with real usage — no client-side placeholder-row logic needed for this case.
3. **Subagent sessions** — check whether these currently populate `TurnTimeline`
   at all before assuming they need special UI; if `ParseResult` is only built for
   top-level Claude Code JSONL files (not subagent transcripts), the correct
   empty-state message is the same generic "not available" copy as case 1, not a
   distinct message — don't invent a "subagent sessions aren't supported" message
   without confirming subagent JSONL is structurally different.

### Sort-by-cost (AC-2/AC-3) with zero/unknown values

Recommendation: **sort zero/unpriced sessions last, regardless of sort direction**
(not interspersed at literal value 0, which would place unpriced sessions between
free and cheap on an ascending sort — misleading, since "$0 cost" and "cost
unknown" are semantically different but numerically identical in a naive sort).
Concretely: sessions with `unpricedModels.length > 0` (already tracked per-session,
rendered today via the `unpricedBadge` in `SessionsTable.tsx` line 162) should sort
after all priced sessions in both asc and desc order, using a comparator like:

```ts
(a, b) => {
  const aUnpriced = a.unpricedModels.length > 0;
  const bUnpriced = b.unpricedModels.length > 0;
  if (aUnpriced !== bUnpriced) return aUnpriced ? 1 : -1; // unpriced always last
  const cmp = a.estimatedCostUsd - b.estimatedCostUsd;
  return sortAsc ? cmp : -cmp;
}
```
This reuses the existing `unpricedBadge`/`unprisedModels` signal already visible
in the same cell — no new backend field needed, and keeps the "unpriced" visual
flag and the "sorts last" behavior semantically linked from the user's
perspective (the badge they already see explains why the row is at the bottom).

## 5. Job-to-be-done — per-turn breakdown

The requirements doc frames this as "debugging which turn spiked cost," not
idle curiosity (the original backlog item's problem statement: "no visibility
into token consumption... only discoverable after the fact"). A flat
chronological list of every turn satisfies the letter of AC-1 but not the job:
a developer scanning a 50-turn session for the expensive one has to eyeball
every row. Recommend, without expanding AC-1's scope:

- **Sort the per-turn table by total tokens (or cost) descending by default**,
  not chronological order — chronological is available as a secondary sort/toggle
  if wanted, but "what spiked" is a max-first question, not a timeline-reading
  question. (Contrast with `ModelOverTimeChart.tsx`, which is correctly
  chronological because *that* component's job is trend-over-time, not
  outlier-finding — different job, different default order.)
- **Visually flag outlier turns** — e.g. a turn whose token count is
  meaningfully above the session's per-turn average (simple threshold: >2x mean,
  computable client-side from the same `TurnEntry[]` array, no new backend field)
  gets a subtle highlight (reuse the `warning`/`alert` variant colors already
  defined in `TokenBadge.css.ts` — `badgeVariant.warning`/`badgeVariant.alert` —
  for palette consistency rather than inventing new colors).
- Do **not** add filtering/search to the per-turn table for this pass — AC-1 only
  asks for the table to render; a 50+ row session is still scannable once sorted
  by size, and adding filter UI now would be scope creep relative to what's
  actually blocking (the missing proto field, section 1 above).

## Summary of concrete UX decisions for implementation

| Gap | Decision |
|---|---|
| AC-1 table shell | Clone `SessionDetailDrawer.tsx`'s Tools Breakdown table pattern (same CSS classes, same empty-state convention) |
| AC-1 data source | New proto field/RPC needed — `SessionTokenSummary` does not carry `TurnTimeline` today; must add before frontend work is possible |
| AC-1 empty state | `"No per-turn data available for this session."`, plain `<p className={emptyState}>`, no skeleton |
| AC-1 default order | Sort by tokens/cost descending, not chronological — serves the "what spiked" job |
| AC-1 outlier flag | Highlight turns >2x session mean using existing `TokenBadge.css.ts` warning/alert colors |
| AC-2 label | "Sort: Cost" (not "Tokens") — matches `TokenBadge`'s displayed metric on the same cards |
| AC-2/AC-3 unpriced handling | Unpriced sessions always sort last in both directions, using existing `unpricedModels` flag |
| AC-3 pattern source | Mirror `app/backlog/page.tsx`'s `aria-sort` + toggle-direction pattern, not `ApprovalRulesPanel.tsx`'s (which lacks `aria-sort`) |
| AC-3 accessibility fix | Add `tabIndex={0}` + `role="button"` + Enter/Space `onKeyDown` to sortable `<th>`s — missing in both existing references, borrow `SessionsTable.tsx`'s own row-level keyboard pattern instead |
| AC-6 data gap | `ModelBreakdown` proto message has `cache_read_tokens` but no `cache_creation_tokens` — needed to show a creation-vs-read split, not just a hit-rate percentage |
