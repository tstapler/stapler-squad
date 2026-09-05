# Research: STACK — insights-cost-intelligence

## Bottom line

No new dependencies are needed for any of the four in-scope workstreams. Everything
(waste-findings panel, per-tool cost, richer sort/search, drill-down route) is buildable
with what's already in `web-app/package.json` and Go's stdlib + existing `session/tokens`
package. This matches the requirements doc's own stated preference (constraints section)
and the "no new library" bias for a Large-appetite-but-single-operator tool.

## Frontend — confirmed exact versions in use (`web-app/package.json`)

| Package | Version | Relevance |
|---|---|---|
| `next` | `15.3.2` | App Router, already used for every route under `web-app/src/app/` |
| `react` | `^19.0.0` | |
| `typescript` | `^5.9.3` | |
| `recharts` | `^3.8.1` | Existing charts (`DailySpendChart`, `ModelBreakdownChart`, `ModelOverTimeChart`) — no new chart type needed for a findings *panel* (it's a ranked list, not a chart), but reuse recharts if a small sparkline/bar accompanies a finding |
| `react-virtuoso` | `^4.18.7` | `TableVirtuoso` already drives `SessionsTable.tsx` (web-app/src/app/insights/SessionsTable.tsx:5) — reuse as-is for richer sort/search, no new virtualization lib |
| `fuse.js` | `^7.3.0` | Already powers text search in `SessionsTable.tsx` (fuzzy search over `FuseDoc` = `{session, backlogTitle}`, web-app/src/app/insights/SessionsTable.tsx:59-77) |
| `@connectrpc/connect` / `connect-web` | `^2.1.1` | RPC client, unchanged |
| `@vanilla-extract/css` / `recipes` / `next-plugin` | `^1.20.1` / `^0.5.7` / `^2.5.1` | Styling — ADR-009 requires `.css.ts` for all new components (waste-findings panel, drill-down route, new table columns) |

No new frontend package is warranted:
- Waste-findings panel is a ranked list of severity-tagged cards/rows — plain React + vanilla-extract, same pattern as `SummaryCards.tsx`/`TopNTables.tsx`.
- Per-tool cost breakdown extends the existing `TopNTables.tsx`/table pattern, no new viz lib.
- Sort/search extension reuses `TableVirtuoso` + `Fuse.js` already wired into `SessionsTable.tsx`.
- Drill-down route reuses `SessionDetailDrawer.tsx`'s existing rendering, just given a route shell.

## Next.js App Router — dynamic route precedent

**Finding: there is no existing `[id]`/`[...slug]` dynamic segment anywhere in `web-app/src/app/`.**
Confirmed via `find web-app/src/app -type d -name '[*]'` (no matches) and a full recursive
listing for bracket paths — none exist in this codebase today. Every other "detail view" in
this app (backlog board, session summary, notifications) uses **query params** via
`useSearchParams`/`useRouter`, not path segments — e.g. `sessions/summary/page.tsx`,
`backlog/board/page.tsx`, `insights/InsightsDashboard.tsx` itself (`useState` for
`selectedSession`, no URL sync at all today).

Since requirement #4 explicitly asks for a **path-based, bookmarkable/shareable** route
(`/insights/session/[sessionId]`), this is new ground for the app, not an established
pattern to copy verbatim — but it is standard, unremarkable Next.js 15 App Router:

- Create `web-app/src/app/insights/session/[sessionId]/page.tsx` — a Server Component
  reading `params: Promise<{ sessionId: string }>` (Next 15 made route `params` a Promise;
  `await params` before use — this is the one version-specific gotcha to get right, since
  Next 14 code samples online still show synchronous `params`).
  ```ts
  export default async function SessionDetailPage({
    params,
  }: {
    params: Promise<{ sessionId: string }>;
  }) {
    const { sessionId } = await params;
    return <SessionDetailPageClient sessionId={sessionId} />;
  }
  ```
- The actual data-fetching/rendering body should be a thin wrapper around the existing
  `SessionDetailDrawer.tsx` content (per the requirements doc's Rabbit Hole: scope this to
  navigation/URL state only, don't refactor `WatchInsights` merge logic in the same pass).
- `next/navigation`'s `useRouter().push(...)` / `<Link href={...}>` from `SessionsTable.tsx`
  row clicks replaces (or supplements) the current `setSelectedSession` state setter.
- `generateMetadata` (Next 15, also async/Promise-based) can supply a per-session
  `<title>` if desired — matches the `export const metadata` pattern already used in
  `insights/page.tsx`, just made dynamic.
- No new routing library — this is 100% built-in App Router file conventions.

## Backend — Go stdlib + existing package only

`session/tokens/` (`parser.go`, `pricing.go`, `association.go`, `skill_detector.go`,
`store.go`, `types.go`) already owns JSONL parsing, pricing, and the `TokenStore` cache.
No new Go dependency needed:
- **Waste-pattern heuristics engine**: pure functions over `[]*ParseResult`/`SessionTokenSummary` —
  stdlib only (`math`, `sort`). No rules-engine library; the requirements doc explicitly
  rejects a config/rules UI and wants hardcoded constants "à la all four reference tools."
- **Per-tool cost attribution**: extends `TopToolEntry` (proto) computed from data already
  parsed by `parser.go`'s turn-level records — no new parsing dependency. The proto only
  currently carries `tool_name`/`call_count`/`mcp_server` (`proto/session/v1/insights.proto:50-54`);
  adding a `cost_usd` field (or a session-level per-tool-type cost map) is a proto change
  requiring `make proto-gen` per the repo's own CLAUDE.md ent/proto rules and the feature-registry
  update rule already flagged as a recurring gap in `token-cost-tracking/requirements.md`.
- Module versions already in `go.mod`/`go.sum` relevant here: `connectrpc.com/connect v1.20.0`,
  `google.golang.org/protobuf v1.36.12`, `github.com/stretchr/testify v1.11.1` — all current,
  no bump needed.

### `ListSessionTokens` sort — already server-side, unused by frontend

`proto/session/v1/insights.proto:122-129` (`ListSessionTokensRequest`) already declares
`sort_by` (`"cost" | "tokens" | "date"`), `sort_desc`, `page_size`/`page_token` pagination —
implemented server-side per the requirements doc's Rabbit Holes note, but the frontend's
`SessionsTable.tsx` does a full client-side scan/sort instead. Extending `sort_by` to
`duration` / `cost_per_message` / `cache_roi` / `waste_score` is an additive proto enum-string
change, not a new dependency — but per the requirements doc's own open question, whether to
switch the frontend to actually call this endpoint (vs. extending client-side sort) is a
Phase 3 planning decision, not a stack question. Either path uses zero new libraries.

## Go test/fixture pattern for a new heuristics-engine test suite

Every `session/tokens/*_test.go` file follows the same shape — this is the pattern a new
`session/tokens/heuristics_test.go` (or wherever the waste-finding engine lands) should copy:

- **testify** (`github.com/stretchr/testify/{assert,require}`) throughout — `require` for
  fatal preconditions (`require.Len`, `require.NotEmpty`), `assert` for the actual checks.
- **`t.Parallel()`** at the top of every test function (see `store_test.go:58,83`).
- **Descriptive `Test<Type>_When<Condition>_Expect<Outcome>` naming** —
  e.g. `TestTokenStore_WhenFileNotCached_ExpectParseOnGetAll`,
  `TestTokenStore_WhenFileCached_ExpectCacheHitSkipsReparse` (`store_test.go:57,82`).
- **Fixture files under `testdata/`** (`session/tokens/testdata/`) for JSONL-shaped input —
  a heuristics suite will instead want **synthetic in-memory `SessionTokenSummary`/`ParseResult`
  structs** built with table-driven test helpers (no JSONL fixture needed, since the heuristics
  operate on already-parsed/aggregated data) — this matches the requirements doc's Feasibility
  Risks note that "fixture-based test approach (synthetic `SessionTokenSummary` data with known
  expected findings)... doesn't exist yet in this codebase and will need to be established."
  `pricing_test.go` is the closest existing precedent for constructing synthetic structs
  in-line rather than loading fixtures from disk — follow its table-driven style for the new
  heuristics tests (a `[]struct{name string; input SessionTokenSummary; wantFindings []Finding}`
  table, one `t.Run(tt.name, ...)` per case).
- **`captureLogs(t)` helper** (`store_test.go:46-55`, wraps a mutex-guarded buffer around
  `slog`'s default handler) is available if a heuristics test needs to assert on structured
  log output — not needed unless the engine logs computation errors per the Observability
  Requirements section.
- Concurrency-safety pattern: `syncBuffer` (`store_test.go:23-44`) wrapping `bytes.Buffer` with
  a `sync.Mutex` is the repo's established idiom any time a test buffer might see concurrent
  writes — reuse if the new suite needs a similar buffer, don't hand-roll another one.

## Summary of decisions this research settles

1. **No new npm or Go dependencies for any of the four workstreams.**
2. **Dynamic route**: no existing `[id]` precedent in this codebase — this will be the first
   one — but it's standard Next.js 15 App Router (`params` is a `Promise` in this version,
   the one gotcha to watch for versus older tutorials/blog posts).
3. **Test pattern**: follow `session/tokens/*_test.go`'s testify + `t.Parallel()` +
   `Test<Type>_When<Condition>_Expect<Outcome>` + table-driven-with-synthetic-structs
   convention (closest precedent: `pricing_test.go`) for the new heuristics suite.
