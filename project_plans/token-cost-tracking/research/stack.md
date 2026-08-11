# Stack Research — Token Cost Tracking Gap Closure

Answers the "what's already in use, reuse don't reintroduce" question for each of the 5
confirmed gaps. No new dependencies are needed for any gap.

## 1. Go backend — JSON parsing & streaming (`session/tokens/`)

- **Parsing**: stdlib only. `session/tokens/parser.go` uses `bufio.Scanner` +
  `encoding/json` (no third-party streaming JSON lib). `maxScannerTokenSize = 10MB`
  handles long base64-embedded JSONL lines.
- **File watching**: `github.com/fsnotify/fsnotify v1.9.0` (go.mod). Used in
  `session/tokens/store.go`'s `TokenStore` to detect new/changed JSONL files; the store
  keeps an in-memory `cachedEntry{result *ParseResult, modTime time.Time}` cache and
  exposes `Subscribe()/Unsubscribe()` (buffered `chan struct{}`) for change notification
  — this is what `WatchInsights` (gap 3) subscribes to.
- **ConnectRPC**: `connectrpc.com/connect v1.19.0`, `connectrpc.com/otelconnect v0.8.0`,
  `google.golang.org/protobuf v1.36.11`. Go toolchain: `go 1.26.3` (go.mod).
- `session/tokens/types.go` already defines `TurnStats` (per-turn model/input/output/
  cache) and `ParseResult.TurnTimeline []TurnStats` — this is the exact data AC-1 (per-turn
  breakdown UI) needs; no new Go plumbing required, just wiring `TurnTimeline` through to
  a new response field or reusing an existing one if already exposed on
  `GetInsightsSummary`/`ListSessionTokens`. Worth confirming during planning whether the
  proto response already carries `TurnTimeline` or needs a new RPC/field.

## 2. Frontend — table/sort primitives

- **`SessionList.tsx`** (main session list) already has a full sort dropdown pattern to
  copy for AC-2:
  - `type SortField = 'lastActivity' | 'name' | 'createdAt' | 'updatedAt'` (line 97) +
    `type SortDir = 'asc' | 'desc'` (line 98)
  - State persisted to localStorage via `STORAGE_KEYS.SORT_FIELD`/`SORT_DIR` (lines
    231–232, 494–499)
  - `sortedSessions` computed via `useMemo` + `Array.prototype.sort` with a `switch
    (sortField)` (lines 586–611)
  - Rendered as a `<select>` (lines 1114–1125) + a direction-toggle button using the
    `sortDirButton` vanilla-extract class (line 1129–1134)
  - **AC-2 should add `'tokens'` (or `'cost'`) as a new `SortField` union member** and one
    more `<option>` — same pattern, no new library.

- **`insights/SessionsTable.tsx`** (insights dashboard table) currently:
  - Uses `TableVirtuoso` from `react-virtuoso` for virtualized rendering (imported line
    5; `VIRTUOSO_THRESHOLD = 50` triggers virtualization only above 50 rows) — this is
    the one relevant frontend dependency already installed for this component, not
    something to introduce for the sort gap.
  - Uses `Fuse` (fuse.js) for text search over `projectPath`/backlog title (lines 63–70).
  - Sort is **hardcoded** today: `[...result].sort((a, b) => ... lastMessageAt desc)`
    (lines 97–101) — no user control, confirming the requirements doc's gap description.
  - Already renders Input/Output/Cache/Cost columns per session (per requirements.md);
    column headers use `th`/`thRight` vanilla-extract classes imported from
    `SessionsTable.css` (lines 15–16).
  - **No existing click-to-sort-header component elsewhere in web-app** to copy verbatim
    — the closest analog, `web-app/src/components/analytics/EscapeEventTable.tsx`, renders
    plain `<th>` headers with no sort affordance at all (checked; no `onClick`/sort state
    there). **AC-3's click-to-sort headers will be new UI code**, but should follow the
    same local `useState<SortField>`/`useState<SortDir>` + `useMemo` sort pattern already
    proven in `SessionList.tsx` (reuse the *pattern*, not a shared component, since none
    exists yet) — replace the hardcoded `.sort()` call at line 97 with a
    state-driven comparator and make the relevant `<th>` cells clickable.

## 3. ConnectRPC streaming — `WatchInsights` implementation & test pattern

- **Implementation** (`server/services/insights_service.go:491-531`):
  ```go
  func (s *InsightsService) WatchInsights(
      ctx context.Context,
      _ *connect.Request[sessionv1.WatchInsightsRequest],
      stream *connect.ServerStream[sessionv1.InsightsEvent],
  ) error {
      // 1. send initial parse_complete/loading event
      // 2. ch := s.store.Subscribe(); defer s.store.Unsubscribe(ch)
      // 3. for { select { case <-ctx.Done(): return nil; case _, ok := <-ch: ... stream.Send(...) } }
  }
  ```
  It sends an initial state event, subscribes to `TokenStore.Subscribe()` (a `<-chan
  struct{}`, content-less — just a tick), and forwards an `update` event on every tick
  until `ctx.Done()`.

- **Critical blocker for AC-4, found via `backlog_service_events.go`'s doc comment**:
  `*connect.ServerStream[T]` (connectrpc.com/connect v1.19.0) is a **concrete struct with
  an unexported `conn` field and no exported constructor** — it cannot be constructed or
  faked directly in a test. The repo's established workaround (see
  `server/services/backlog_service_events.go` lines 1–19 and its test file
  `backlog_service_events_test.go`) is:
  1. Define a narrow, consumer-side `xxxEventSender` interface with just a `Send(*T)
     error` method.
  2. Keep the exported RPC method (`WatchBacklogItems`) as a thin wrapper that calls an
     **unexported core-logic function** (`watchBacklogItems(ctx, svc, req, sender)`)
     accepting that interface instead of the concrete `*connect.ServerStream[T]`.
  3. `*connect.ServerStream[T]` structurally satisfies the narrow interface for free (it
     already has a matching `Send` method), so production code passes it through with
     zero adapter code.
  4. Tests build a `fakeBacklogItemEventSender` implementing the interface and drive
     `watchBacklogItems` directly.
  - Test names follow `TestWatchBacklogItems_should_<effect>_When_<condition>`, e.g.
    `TestWatchBacklogItems_should_forwardLiveEvent_When_PublishedWhileStreamIsLive`
    (lines 304–338) is the closest structural analog to what AC-4 needs: publish a fake
    store change, then `require.Eventually` assert the sender received ≥2 sends, using
    `context.WithCancel` + a `done` channel from a `runWatchBacklogItems(...)`-style
    helper, then `requireCleanReturn(t, cancel, done)` to verify clean shutdown.
  - **`WatchInsights` currently does NOT follow this pattern** — it takes the concrete
    `*connect.ServerStream[sessionv1.InsightsEvent]` directly, so it is unit-testable only
    after the same refactor (extract `insightsEventSender` interface +
    `watchInsights(ctx, svc, req, sender)` core function). This refactor is a prerequisite
    for AC-4, not just "add a test."
  - `fakeTokenStore` (in `insights_service_test.go` lines 21–37) already implements
    `tokens.TokenStoreReader` including `Subscribe()/Unsubscribe()`, but its `Subscribe()`
    returns a **fresh unbuffered-content channel every call** (`make(chan struct{}, 1)`)
    with no way to push a tick into it from a test — it will need a small extension (e.g.
    store the channel so the test can send on it, or add a `Publish()` helper) to drive a
    live "update" event in the new WatchInsights test.

- No other `Watch*` RPC in the codebase besides `WatchBacklogItems` (`backlog_service_events.go`) uses this
  interface-extraction pattern with tests; `rules_store.go` also has Watch-shaped code
  but no test coverage was found for it. `backlog_service_events_test.go` is the one
  concrete precedent to follow.

## 4. Charting — recharts capability for AC-6 (cache hit rate at aggregate level)

- **Library**: `recharts ^3.8.1` (web-app/package.json) — the only charting library in
  the project (no d3/victory/nivo). Used consistently across `DailySpendChart.tsx`,
  `ModelBreakdownChart.tsx`, `ModelOverTimeChart.tsx`.
- **Stacking is already a proven pattern in this codebase**: `ModelOverTimeChart.tsx`
  (lines 147–159) renders a stacked `AreaChart` — one `<Area>` per model, all sharing
  `stackId="1"` — to show per-model contribution over time. `recharts`' `<Bar>`/`<Area>`
  both support `stackId`, so a cache-creation-vs-read split (2 series sharing one
  `stackId`) is directly achievable with the same primitive already in use — no new
  recharts feature or library needed.
- **Current gap**: neither `ModelBreakdownChart.tsx` nor `ModelOverTimeChart.tsx`
  currently reads/plots cache-creation vs cache-read fields (`grep` for
  `cacheRead|cacheCreation|cache_read|cacheHitRate` in both files returned nothing) — the
  per-session cache split exists today only in `SessionDetailDrawer.tsx` and
  `SessionsTable.tsx`. `computeCacheHitRate(input, cacheRead)` already exists as a Go
  helper in `insights_service.go` (lines 551–558: `cacheRead / (input + cacheRead)`) —
  AC-6 will likely need either a new field on the model-breakdown response or reuse of
  data already aggregated per model, plus a new stacked `<Bar>`/`<Area>` pair (or a
  secondary hit-rate line) in one of the two chart components, following the existing
  `stackId` pattern.

## Summary

No new dependencies required for any of the 5 gaps:
| Gap | Reuse |
|---|---|
| AC-1 (per-turn table) | `TurnStats`/`TurnTimeline` already parsed; needs wiring only |
| AC-2 (SessionList sort) | Copy `SessionList.tsx`'s existing `SortField`/localStorage/`useMemo` sort pattern verbatim, add a token/cost field |
| AC-3 (SessionsTable sort) | Same pattern as AC-2, applied inside `SessionsTable.tsx`, replacing the hardcoded `.sort()`; no existing click-to-sort component to copy, but `TableVirtuoso`/`Fuse` stay untouched |
| AC-4 (WatchInsights test) | Must first extract an `insightsEventSender` interface + unexported core function, mirroring `backlog_service_events.go`/`backlog_service_events_test.go` exactly — this is the one gap requiring a code refactor before a test can be written, not just new test code |
| AC-6 (cache hit rate chart) | recharts `stackId` stacking pattern already proven in `ModelOverTimeChart.tsx`; only new series/response field needed |
