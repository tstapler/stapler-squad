# Feature Research: Edge Cases, Failure Modes, Unstated Needs

Research pass for closing the 6 gaps identified in `requirements.md`. Findings below are
grounded in direct reads of the relevant source, not assumption.

## AC-1: Per-turn token breakdown (`SessionDetailDrawer.tsx`)

**`TurnStats` shape** (`session/tokens/types.go:29-37`):
```go
type TurnStats struct {
    Timestamp     time.Time
    Model         string
    Input         int64
    Output        int64
    CacheCreation int64
    CacheRead     int64
    ToolNames     []string // tool_use block names in this message
}
```
One entry per assistant message (`session/tokens/parser.go:138-189`), already ordered
chronologically (asserted in `parser_test.go:64-82`). `<synthetic>` turns are filtered out
before appending (`parser_test.go:153-155`), so the timeline is clean of noise turns.

**Gap is bigger than "wiring":** `TurnTimeline` is **not exposed over the wire at all**.
`SessionTokenSummary` (`proto/session/v1/insights.proto:23-42`) — the only message the
frontend receives from `ListSessionTokens`/`GetInsightsSummary` — has no turn-level field.
`SessionDetailDrawer.tsx` receives a `SessionTokenSummary`, not a `ParseResult`. Closing
AC-1 requires either:
- a new repeated `TurnStat` message added to `SessionTokenSummary` (simplest, but bloats
  the payload for every session in `ListSessionTokens`/`GetInsightsSummary` even when the
  drawer isn't open), or
- a new narrow RPC (e.g. `GetSessionTurns(conversation_id) → repeated TurnStat`) fetched
  lazily only when the drawer opens for a given session (preferred — avoids paying the
  per-turn payload cost for the whole list).

Either way this is a **proto change + `make proto-gen` + new/extended service method**, not
just a frontend edit — flag this scope difference before planning task sizing.

**Rendering pattern to reuse — already in this codebase, do not invent a new one.**
`insights/SessionsTable.tsx` (lines 43-45, 245-254) has the canonical "sortable table that
might get large" pattern:
```ts
const VIRTUOSO_THRESHOLD = 50;
// ...
) : displayed.length > VIRTUOSO_THRESHOLD ? (
  <TableVirtuoso data={displayed} fixedHeaderContent={headerContent} itemContent={renderCells} .../>
) : (
  <table>...</table>
)
```
`react-virtuoso` (`@tanstack/react-virtual` is also a dep, unused here — `react-virtuoso` is
the one actually wired up) is already a `web-app` dependency. `SessionDetailDrawer.tsx`'s
existing "Tools Breakdown" table (lines 137-160) is a plain unpaginated `.map()` — fine for
tool counts (small, bounded set) but the **wrong pattern to copy for turns**, since a long
agent session can have hundreds of assistant turns. Reuse the `VIRTUOSO_THRESHOLD` /
`TableVirtuoso` pattern from `SessionsTable.tsx` for the new per-turn table, not the Tools
Breakdown table's plain `.map()`.

**Edge cases to handle:**
- `TurnTimeline` empty (session with 0 assistant messages, or JSONL not yet parsed) — needs
  the same `emptyState` treatment already used for empty `topTools`/`skillActivations`
  (`SessionDetailDrawer.tsx:138-139`, `164-165`): `"No tools recorded for this session."`-style copy.
- `Timestamp` zero-value turns exist in principle (parser guards against it in
  `sessionTimestamps`, `insights_service.go:536-549`, treating zero timestamps as "skip for
  min/max" — the per-turn table should format a zero timestamp as `"—"` rather than
  rendering `"0001-01-01"`).
- `Model` can differ turn-to-turn within one session (documented use case: `Models []string`
  on `ParseResult` — mid-session model switches are real, not hypothetical). The per-turn
  table must show model per row, not assume the session's `primaryModel` applies uniformly.

## AC-2 / AC-3: Sortable session lists

**Backend already supports server-side sort — this is a pure frontend gap, confirmed.**
`ListSessionTokensRequest` (`proto/session/v1/insights.proto:116-123`) already has
`sort_by` (`"cost" | "tokens" | "date"`) and `sort_desc` fields, and
`InsightsService.ListSessionTokens` (`server/services/insights_service.go:424-452`) already
implements the full `sort.Slice` comparator for all three. **No backend/proto change needed
for AC-3.**

**`SessionsTable.tsx` gap is real and narrow.** Its `displayed` `useMemo`
(`SessionsTable.tsx:97-101`) hardcodes:
```ts
return [...result].sort((a, b) => {
  const at = a.lastMessageAt ? Number(a.lastMessageAt.seconds) : 0;
  const bt = b.lastMessageAt ? Number(b.lastMessageAt.seconds) : 0;
  return bt - at;
});
```
This is client-side sort over data already fetched via `GetInsightsSummary` (not
`ListSessionTokens`, which is presumably used elsewhere for pagination) — so AC-3 doesn't
need the RPC's `sort_by` field either; it needs sort **state** (field + direction) wired
to the column headers (`headerContent`, lines 118-128) and swapped into this comparator.
Null/missing handling: `lastMessageAt` already defaults absent timestamps to `0` (sorts
last in descending order) — extend the same `?? 0`-style fallback to `totalInputTokens` /
`totalOutputTokens` / `estimatedCostUsd` when adding those as sort keys (they're `int64`/
`double` proto scalars, so they're never `undefined`, only legitimately `0` — no null case
to handle there, unlike `lastMessageAt` which is an `optional Timestamp`).

**`SessionList.tsx` gap is much bigger than the requirements doc states — verify before
scoping.** `requirements.md` claims "Session card token badge | Done | `TokenBadge.tsx`,
used in `SessionList.tsx`" (line 29). This is **false**: `TokenBadge.tsx` is imported
nowhere in the app except its own test file:
```
$ grep -rln "TokenBadge" web-app/src/
web-app/src/components/shared/TokenBadge.tsx
web-app/src/components/shared/__tests__/TokenBadge.test.tsx
```
`SessionList.tsx` has zero references to `TokenBadge`, `useInsightsService`,
`GetInsightsSummary`, or `ListSessionTokens` — it has no token/cost data at all for any
session in its render tree. `TokenBadge` was built in PR #104 (token & spend monitoring
dashboard, `git log` confirms) and only ever wired into the `/insights` route, never into
the main session list. **AC-2 is not "add a sort option to an existing dropdown" — it
requires first plumbing per-session token/cost data into `SessionList.tsx`** (almost
certainly via `useInsightsService`'s `ListSessionTokens`, keyed by session ID, merged into
the existing session objects or looked up in a `Map` the way `SessionsTable.tsx` does with
`backlogIndex`), *then* adding the sort field. Flag this to the planning phase — it's
materially more work than the requirements doc implies, and touches a component
(`SessionList.tsx`, 1582 lines) already carrying 4 sort fields, localStorage persistence
(`STORAGE_KEYS.SORT_FIELD`/`SORT_DIR`, lines 346-351, 494-499), and its own `react-virtuoso`
virtualization — this is not a small/isolated component.

`SessionList.tsx`'s existing `SortField` union (`type SortField = 'lastActivity' | 'name' |
'createdAt' | 'updatedAt'`, line 97) is a plain string union with a `switch` in the
comparator (lines 590+) — mechanically easy to extend with `'tokenCost'`, but the value has
to come from somewhere new. **Null handling precedent**: none exists yet in this file for
async/possibly-missing numeric data — nearest analogue is `getTimestampMs(a.createdAt)`
used for the `createdAt`/`updatedAt` cases, worth checking what that helper returns for a
missing timestamp (likely `0`) as the convention to mirror for missing token data (new
sessions or unparsed JSONL): treat missing/loading as `0`, which — given `sortDir: 'desc'`
default — naturally sorts them last, matching `TokenBadge`'s own lack of any loading-state
prop (`TokenBadgeProps.costUsd` is a required `number`, no `| null`, no `| undefined`,
`TokenBadge.tsx:6-8` — the component was never designed for a "loading" state to begin
with, so the wiring work must also decide what to pass while the session's token summary
hasn't loaded yet: `0` is the only value the existing component signature accepts).

## AC-4: `WatchInsights` test coverage

**Streaming-RPC-under-test is a known, already-solved problem in this codebase — follow the
established pattern, don't improvise one.** `backlog_service_events.go` (lines 1-20 doc
comment) explains explicitly *why* `*connect.ServerStream[T]` can't be faked directly:
connect-go v1.19.0's `ServerStream[Res]` is a concrete struct with an unexported `conn`
field and no exported constructor. The fix used there: extract a narrow interface —
```go
type backlogItemEventSender interface {
    Send(*sessionv1.BacklogItemEvent) error
}
```
— and have the exported RPC method delegate to an unexported core-logic function
(`watchBacklogItems`) that accepts the interface instead of the concrete `ServerStream`.
Tests then drive it with a small mutex-guarded fake
(`fakeBacklogItemEventSender`, `backlog_service_events_test.go:77-87`).

`InsightsService.WatchInsights` (`insights_service.go:491-531`) currently does **not** have
this split — it calls `stream.Send(...)` directly against the concrete
`*connect.ServerStream[sessionv1.InsightsEvent]` parameter. **AC-4 requires the same
refactor** (extract `insightsEventSender` interface + unexported `watchInsights` core
function) before a unit test is possible without standing up a full HTTP/h2c server (no
existing test in this codebase does that for a streaming RPC — grepped
`server/services/*_test.go` for `httptest.NewServer` + streaming client construction and
found nothing; the interface-extraction pattern is the only precedent that exists).

**Trigger mechanism to simulate:** `TokenStore.Subscribe()` (`store.go:126-135`) returns a
buffered `chan struct{}` added to `ts.subs`; `notify()` (`store.go:149-160`) does a
non-blocking send to every subscriber whenever `parseAndCache` finishes
(`store.go:188-221`, calls `ts.notify()` at line 221) — which itself is triggered either by
`OnHistoryFileChanged` (real fsnotify callback) or, more usefully for a test, by directly
calling the already-test-proven `store.enqueue(filePath)` (used exactly this way in
`session/tokens/store_test.go:20,44` against `testdata/valid_session.jsonl`, with a
poll-until-nonempty loop against `GetAll()` since parsing happens on a worker goroutine).
**A real `TokenStore` (not the existing `fakeTokenStore` from `insights_service_test.go`,
whose `Subscribe()` returns an inert, never-notified channel at line 36) is the right test
double for AC-4**: construct one against `testdata/valid_session.jsonl`'s directory, `Start`
it, subscribe via the refactored `watchInsights`, call `store.enqueue(...)` to trigger a
real `notify()`, and assert the fake sender received an `"update"` event. The existing
`fakeTokenStore` is fine for the non-streaming `ListSessionTokens`/`GetInsightsSummary`
tests already in `insights_service_test.go` but is unsuited to AC-4 as-is because its
`Subscribe()` is a dead-end channel — either extend it with a controllable channel field, or
use a real `TokenStore` per above (real one is closer to `store_test.go`'s existing
polling-friendly conventions and requires no new fake code).

## AC-5: Feature registry entries

**The registry pipeline has more moving parts than `.claude/rules/feature-registry.md`'s
example implies — read this before hand-writing 5 files.**

`make registry-generate` runs three steps in order
(`Makefile:108`: `registry-generate-backend registry-generate-frontend registry-aggregate`):
1. `registry-generate-frontend` (`Makefile:91-100`) runs the TS scanner
   (`tools/scanner/frontend/src/main.ts`), which **scans live source for `// +feature:`
   markers** and writes directly to the monolithic `docs/registry/frontend-features.json` —
   it never reads or writes the per-feature files under `docs/registry/features/frontend/`.
   It also computes `coverage-gaps.json` at this point, from the live scan.
2. `registry-aggregate` (`Makefile:103-106`) then runs `tools/scanner/aggregate.py` against
   `docs/registry/features/frontend/**/*.json` (the hand/generator-maintained per-feature
   files) and **overwrites** `frontend-features.json` with that aggregate — so the
   final committed `frontend-features.json` reflects the per-feature files, not step 1's
   live scan. Editing/creating per-feature files by hand is therefore the mechanically
   correct way to add entries — confirmed against `aggregate.py` and `Makefile`.

**Real field shape (verified against `docs/registry/features/frontend/insights-pricing-unavailable-indicator.json` and `docs/registry/features/frontend/ui/insights-dashboard.json`)
differs from `.claude/rules/feature-registry.md`'s example** (`schema.json` is stale/backend-only —
its `Feature.type` enum is literally `["backend"]` only, and its `BackendDetails.service`/
`method`/`protoFile` nesting doesn't match the flat shape the real backend files use either,
e.g. `docs/registry/features/backend/WatchInsights.json`). Use these real fields, not the
rule doc's `filePath`:
```json
{
  "id": "kebab-case-slug",
  "type": "frontend",
  "component": "PascalCaseComponentName",
  "path": "web-app/src/app/insights/Foo.tsx",
  "markerLine": 1,
  "tested": true,
  "testIds": ["DescribeBlockName > test_description"],
  "lastModified": "ISO-8601"
}
```

**Marker collision — the real risk for AC-5.** All 5 target files
(`SessionDetailDrawer.tsx`, `ProjectedCostCard.tsx`, `DailySpendChart.tsx`,
`ModelOverTimeChart.tsx`, `SessionsTable.tsx`) currently share the exact same marker:
`// +feature: insights-dashboard` on line 1. The scanner
(`tools/scanner/frontend/src/component-scanner.ts:96` comment: "The marker may list
multiple IDs") already produces one `FrontendFeature` per file for that shared id, and an
existing aggregated entry — `docs/registry/features/frontend/ui/insights-dashboard.json` —
already claims `tested: true` with 21 `testIds` spanning `insights-dashboard`, `TokenBadge`,
and `useTopSessions` describe blocks, canonically attributed to `InsightsDashboard.tsx` (a
6th file, the page-level component, not one of the 5 named in AC-5). Two options for
closing AC-5 without breaking that existing entry:
- **(a) Hand-write the 5 new per-feature files with distinct new `id`s** (e.g.
  `insights-session-detail-drawer`, `insights-projected-cost-card`, …) **without** touching
  the source markers. Since step 2 (`registry-aggregate`) only reads whatever files exist
  under `docs/registry/features/frontend/`, this works mechanically — but leaves the live
  scanner (step 1) still reporting these files under `insights-dashboard`, a latent
  inconsistency between "generated-from-source" and "hand-maintained" state that
  `registry-diff`/`validate-registry.sh` may flag on a future run.
- **(b) Add a second, component-specific id to each file's marker** (multi-id markers are
  explicitly supported per the scanner comment above), e.g.
  `// +feature: insights-dashboard session-detail-drawer` — this makes the live scan and
  the hand-written files agree, but is a source-code change to 5 files, beyond "just add
  registry JSON," and needs a naming decision (kebab-case slugs, ideally sharing the
  `insights-` domain prefix — see next paragraph for why the prefix matters).

**Coverage-gaps risk (the AC-5 "no unexplained growth" condition).**
`gap-reporter.ts`'s matching is explicitly "advisory/best-effort" domain-prefix matching
(`gap-reporter.ts:34-42` doc comment): a backend id like `insights:*`-domained RPCs get
matched to frontend ids sharing the same domain token. If the 5 new ids don't carry an
`insights-` prefix (e.g. bare `"SessionDetailDrawer"` instead of
`"insights-session-detail-drawer"`), they risk not matching the `insights` domain the
backend `GetInsightsSummary`/`ListSessionTokens`/`WatchInsights` features live under,
which could shift `coverage-gaps.json`'s `unmatchedFrontend`/`unmatchedBackend` counts in
either direction — worth an explicit `make registry-diff` / before-and-after count check in
the plan's acceptance-verification step, not just a visual scan of the new files.

## AC-6: Cache hit rate at aggregate/model level

**No proto change required — the data is already present, just not computed client-side.**
`ModelBreakdown` (`proto/session/v1/insights.proto:68-78`) already carries
`cache_read_tokens` and `total_input_tokens` per model family. `DailyTokenBucket`
(lines 51-65) has the same two fields per day. The per-session formula already exists
server-side —
```go
// insights_service.go:551-558
func computeCacheHitRate(input, cacheRead int64) float64 {
    denom := input + cacheRead
    if denom == 0 { return 0 }
    return float64(cacheRead) / float64(denom)
}
```
— but there is no `cache_hit_rate` field on `ModelBreakdown`/`DailyTokenBucket`
message types, only on `SessionTokenSummary` (line 33). AC-6 can be closed **entirely
client-side** in `ModelBreakdownChart.tsx`/`ModelOverTimeChart.tsx` by porting this exact
formula into TS and computing it from the `cache_read_tokens`/`total_input_tokens` fields
already being received — no `make proto-gen` needed. (If a `cache_creation_tokens` split is
also wanted at this level — the requirements doc says "cache-creation-vs-read split" as an
alternative framing — that field genuinely doesn't exist on `ModelBreakdown`/
`DailyTokenBucket` today and *would* need a proto addition; flag this fork explicitly in
planning: hit-rate-only is free, creation-vs-read split is not.)

## Cross-cutting notes

- `SessionList.tsx` already depends on `react-virtuoso` for its own virtualization
  (confirmed via grep — it's one of six files under `web-app/src` importing it, alongside
  `SessionsTable.tsx`, `LogViewer.tsx`, `VirtualLogList.tsx`) — so adding a new sort field
  there is low-risk with respect to render performance; the real risk is the missing data
  plumbing described under AC-2 above, not virtualization.
- No backlog-list or other non-insights table in this codebase currently implements
  click-to-sort column headers — `SessionsTable.tsx`'s `VIRTUOSO_THRESHOLD` pattern is the
  only "sortable + possibly-virtualized" precedent to mirror for both AC-1 and AC-3; there
  is no separate backlog-side pattern to reconcile against.
