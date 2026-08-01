# Build vs. Buy — Token Cost Tracking Remaining Gaps

## Context

Five remaining gaps close out an already-shipped feature (see `requirements.md`):
per-turn breakdown table, sortable session lists (x2), a `WatchInsights` test, registry
metadata, and an aggregate cache-hit-rate chart. This doc answers whether any of the
three UI-facing gaps justify a new dependency, and sanity-checks the storage-design
divergence from the original ask.

## 1 & 2. Per-turn breakdown table + sortable columns

**Checked:** `web-app/package.json` (dependencies + devDependencies, full list read
directly) — no data-grid/table library present. No `@tanstack/table-core`,
`@tanstack/react-table`, `ag-grid-*`, `react-table`, `material-react-table`, or similar.
The only `@tanstack/*` package is `@tanstack/react-virtual` (list virtualization, not a
table/grid abstraction — no sorting API).

**Existing convention confirmed by reading the actual sort implementations:**
- `web-app/src/components/sessions/SessionList.tsx` (lines ~346–612): sorting is plain
  `useState<SortField>`/`useState<SortDir>`, persisted to `localStorage`
  (`STORAGE_KEYS.SORT_FIELD`/`SORT_DIR`), applied via a `useMemo` that does
  `[...filteredSessions].sort((a,b) => ...)` with a `switch (sortField)`. Rendered as a
  `<select>` dropdown (line ~1116) plus a direction toggle button. This is the pattern
  AC-2 should extend — add a `"tokens"`/`"cost"` case to the existing switch and a new
  `<option>`, not a new sorting mechanism.
- `web-app/src/app/insights/SessionsTable.tsx` (276 lines total): a bespoke `<table>`
  (native HTML `<table>`/`<td>` elements, styled via CSS modules/vanilla-extract
  classes) with sorting currently hardcoded to `lastMessageAt` inside a `useMemo`
  (`[...result].sort((a,b) => ...)`, lines ~97–99). It already renders Input/Output/
  Cache/Cost columns (per requirements.md). AC-3 needs a `sortField`/`sortDir` state
  pair (mirroring `SessionList.tsx`'s pattern) plus clickable `<th>` headers — no
  structural change to the table itself.
- The per-turn breakdown (AC-1) is materially the same shape: a small, one-off table
  rendering `ParseResult.TurnTimeline` rows inside `SessionDetailDrawer.tsx`. No
  pagination, virtualization, column resizing, or filtering requirement exists in the
  acceptance criteria — it's a static list of turns for one session.

**Pros of adopting a table library (e.g. TanStack Table):**
- Built-in sort/filter/pagination primitives, one abstraction reused across all three
  tables.
- Removes some boilerplate if a fourth or fifth sortable table shows up later.

**Cons:**
- New dependency (`@tanstack/react-table` + its adapter code) for what's currently 2
  sortable tables and 1 static table — bundle size and a new API surface to learn/
  maintain for a codebase that has deliberately avoided this class of dependency so far
  (confirmed absent from a fairly large `package.json`, 3 Radix packages, react-hook-
  form, etc. — so it's not an oversight, table abstraction was never reached for).
- `SessionList.tsx`'s sort dropdown is the established, working convention already used
  for the *other* main list in this app — introducing a second sorting paradigm
  (declarative table-library state) for `SessionsTable.tsx` while `SessionList.tsx`
  keeps manual state creates inconsistency between two sibling session-list views
  rather than reducing it.
- All three tables are small, session-scoped (bounded by how many sessions/turns
  exist), not needing virtualization beyond what `@tanstack/react-virtual` (already a
  dependency) already covers if it ever becomes necessary.
- Migrating `SessionsTable.tsx`'s existing manual sort to a table library mid-gap-close
  would be a larger diff than the acceptance criteria calls for (AC-3 just asks for
  click-to-sort on existing columns).

**Verdict: Not recommended.** Extend the existing manual `useState` + `useMemo` sort
pattern in both `SessionList.tsx` and `SessionsTable.tsx`, and add a plain `<table>` (or
reuse `SessionsTable.tsx`'s existing table CSS classes) for the per-turn breakdown. This
is squarely a YAGNI case — two tables needing sort, one needing a static list, all
already served by patterns proven elsewhere in the same codebase.

## 3. Cache hit rate chart

**Checked:** `recharts` (`^3.8.1`) is already a dependency and already used in
`web-app/src/app/insights/ModelBreakdownChart.tsx`, `ModelOverTimeChart.tsx`, and
`DailySpendChart.tsx`. `ModelBreakdownChart.tsx` already imports `BarChart`/`Bar` from
recharts and renders a per-model bar chart (`<Bar dataKey="cost" radius={[4,4,0,0]}>`).
recharts' `<Bar>` supports a `stackId` prop natively for stacked/grouped bars — no
plugin or additional package needed to render a cache-creation-vs-read split per model,
which is exactly the shape AC-6 needs (stack `cacheCreationTokens` and `cacheReadTokens`
under a shared `stackId`, or render hit-rate as a derived percentage bar).

`cacheHitRate` is also already computed at the per-session level (confirmed at
`web-app/src/app/insights/SessionsTable.tsx:159`, `fmtPct(s.cacheHitRate)`) — AC-6 is
aggregation/wiring work (roll the existing per-session field up to per-model/per-day),
not new charting capability.

**Verdict: Recommended (use recharts, no new dependency).** Add a stacked `<Bar>` pair
(or a derived hit-rate series) to the existing `ModelBreakdownChart.tsx` and/or
`ModelOverTimeChart.tsx` — the charting library already does everything this needs.

## 4. Storage design divergence from the original ask

The original backlog item proposed parsing "Claude Code's JSONL output... or the
agent's stdout structured events" and storing "cumulative token_usage per session" —
phrased ambiguously enough to read as a new ent schema column set alongside existing
session storage.

What's actually implemented (`session/tokens/store.go`) parses the same JSONL files but
caches results in-memory, keyed off the JSONL file paths, invalidated via `fsnotify`
watching those files for changes — no new ent schema/DB columns. `requirements.md`
already calls this out explicitly as an intentional divergence ("Non-Goals": "Adding a
new ent schema `token_usage` column set... would be redundant storage with a
sync-consistency problem").

**This is confirmed as a strictly better fit for this codebase, not a gap:**
- The JSONL files (Claude Code's own transcript output) are already the single source
  of truth for token usage — Anthropic's CLI writes them, this app doesn't own them.
  Any ent-column mirror would need a write-path that re-parses JSONL on every session
  turn anyway, so the ent columns would just be a redundant cache with its own
  invalidation problem — the exact bug class `fsnotify` + in-memory cache was chosen to
  avoid.
- ent/DB storage would need explicit invalidation logic whenever the underlying JSONL
  changes (e.g. a resumed session appending new turns); the `fsnotify`-driven cache
  gets that invalidation for free by watching the actual file.
- No cross-session aggregation requirement in the acceptance criteria needs SQL-level
  joins/queries against a token_usage table — `GetInsightsSummary`/`ListSessionTokens`
  already serve the dashboard's aggregate needs by iterating the in-memory store.

**Nothing to flag as a real gap here** — the divergence from the original ask is
correctly identified as a design improvement in `requirements.md`'s Non-Goals section,
and this research confirms that reasoning holds: JSONL is the source of truth Anthropic
already writes, and mirroring it into ent would introduce a second, sync-fragile copy
of the same data with no acceptance criterion that needs it.

## Summary Table

| Gap | New dependency? | Verdict |
|---|---|---|
| Per-turn breakdown table (AC-1) | No | Not recommended — plain `<table>`, matches `SessionsTable.tsx` styling |
| Sortable `SessionList.tsx` (AC-2) | No | Not recommended — extend existing `sortField`/`sortDir` `useState` + `useMemo` pattern |
| Sortable `SessionsTable.tsx` (AC-3) | No | Not recommended — same manual-sort pattern, click-to-sort `<th>` headers |
| Cache hit rate chart (AC-6) | No | Recommended — recharts `<Bar stackId>`, already a dependency, already used for bar charts in this exact directory |
| JSONL+fsnotify vs. ent storage (design divergence) | N/A | Confirmed better fit — avoids duplicate source of truth and cache-invalidation bugs; not a real gap vs. the original ask |
