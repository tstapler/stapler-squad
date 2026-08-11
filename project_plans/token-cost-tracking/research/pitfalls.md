# Pitfalls Research — Closing Gaps in an Already-Shipped Feature

Research question: what commonly goes wrong when adding to a live analytics/cost
feature like this, and what should the gap-closing work (AC-1..AC-7) explicitly
design against. Grounded in git history, current code, and this repo's established
patterns — not general best-practice guessing.

## 1. "Silent wrong data" — the class of bug this codebase has already been bitten by once

`95ed72d34` (`fix(insights): close pricing gaps and unaccounted costs`, PR #280,
2026-07-27 — 5 days before this triage) is the direct precedent. Root cause pattern:
new data (gen-5 model usage) flowed through existing aggregation code, hit a lookup
that had no entry for it, and silently rendered as **plausible-looking zero** instead
of an error — `$0.00` next to a real model name looks like "this model is free," not
"data missing." The fix wasn't just adding the missing entries; it changed
`EstimateCost`/`ModelFamilyCost` to *also* return which model families had usage but
no pricing data, threaded a `pricing_unavailable` signal through 4 call sites and
every chart, and added a completeness test asserting every active Claude family has a
pricing entry — turning a future occurrence into a visible badge instead of a repeat
incident. It also filtered `<synthetic>` internal turns at the parser boundary so
they could never falsely trip the new "unpriced" signal (a **new class of false
positive** introduced by the very fix that closed the false negative).

Two more relevant details from that PR's own retrospective:
- It explicitly filed (rather than silently fixed) two *other* instances of the same
  silent-cost-zero class it found but decided were out of scope: BUG-049
  (`BacklogService.buildCostLookup`) and BUG-050 (`CapacityMonitor.estimateCost`'s
  independent, substring-matched pricing table — which feeds a live autonomous-session
  cost-budget trigger, i.e. a silent-wrong-data bug with a real-money blast radius,
  not just a UI cosmetic one).
- A `TestDefaultPricingTable_WhenCalledToday_ExpectNotStale` guard was first written
  anchored to `time.Now()`, shipped, then had to be re-fixed in the same PR to anchor
  to the pricing table's own "as of" date instead — "the test doesn't itself become a
  ticking time bomb that starts failing unrelated PRs 30 days after any pricing
  refresh." Any new completeness/staleness test this gap-closing work adds should
  anchor to data, not wall-clock time, from the start.

### Does the same silent-wrong-data risk apply to AC-1 (per-turn breakdown) and AC-2/AC-3 (sort-by-tokens)?

- **AC-1 (per-turn breakdown):** Lower risk than the pricing case, but not zero.
  `ParseResult.TurnTimeline`'s `TurnStats` entries are already computed today and
  simply unwired into the UI — no new aggregation logic, so there's no new lookup
  table that can silently 404. The one real analogue: if any turns are `<synthetic>`
  (the same internal-turn category filtered out of model-family aggregation in
  #280), a naive "iterate `TurnTimeline`" implementation could either double-count
  them into a per-turn total that doesn't match the already-shown session rollup, or
  render a confusing zero-cost row with no explanation. Whatever renders the table
  should reuse the same synthetic-turn exclusion (or explicitly label
  synthetic turns) so the per-turn sum is reconcilable against the existing
  session-level total shown elsewhere in `SessionDetailDrawer.tsx` — a table that
  silently doesn't sum to the header total is its own "silent wrong data" bug.
- **AC-2/AC-3 (sort by tokens/cost):** No new numeric computation, so the pricing-gap
  failure mode doesn't directly repeat. The analogous risk here is UX-shaped, not
  data-shaped — see §3 below.

## 2. Concurrency: `TokenStore` locking under a sort-by-tokens query

`session/tokens/store.go` uses a single `sync.RWMutex` (`ts.mu`) guarding both the
main cache (`map[string]*cachedEntry`) and the secondary `byUUID` index, plus a
**separate** `subsMu sync.RWMutex` for the subscriber list. Read paths (`GetAll`,
`GetByUUID`, `IsLoading`) all take `RLock`; the only writer path is
`parseAndCache`, which briefly `RLock`s to check staleness then `Lock`s to write.

For a sort-by-tokens query (AC-2/AC-3), the read path is `GetAll()` → build a
`[]*ParseResult` snapshot → sort it in the service/handler layer. Because `GetAll`
copies pointers into a fresh slice while holding only a **read** lock, and Go's
`RWMutex` allows unlimited concurrent readers, this is not a contention risk in the
classic sense — hundreds of concurrent `GetAll()` calls (e.g. many browser tabs open
on `/insights`, or `WatchInsights` streams triggering re-fetches) can proceed in
parallel with each other. The actual contention point is **writer starvation**: Go's
`sync.RWMutex` gives waiting writers priority once one is queued (new readers block
behind a pending writer), so under a sustained burst of `GetAll()` reads (e.g. a
poorly-throttled sort/re-sort loop on every keystroke in a search box, or a page that
re-fetches on every `WatchInsights` `update` event without debouncing) a
`parseAndCache` writer racing to land a live fsnotify-triggered update could be
delayed. Given `historyDir` walks and fsnotify events are not latency-critical (a few
ms of writer delay is invisible to the user), this is not a functional risk, but it's
worth **not** making the new sort a per-render `GetAll()` call from a component that
re-renders on every keystroke — memoize/debounce the query (`SessionsTable.tsx` and
`InsightsDashboard.tsx` already receive a materialized array as a prop and sort with
a `useMemo`, which is the correct existing pattern to extend, not to bypass with a
fresh service round-trip per interaction).

One structural point worth flagging for AC-4's new test: `notify()` is a **non-blocking
best-effort** fan-out (`select { case ch <- struct{}{}: default: }`), and each
subscriber channel has a bounded buffer (`subChanSize = 64`). A slow test consumer
that doesn't drain promptly can silently drop notifications rather than block — this
is a deliberate design choice (documented as "the caller should drain the channel
promptly"), but it means a naive test that does `store.Subscribe()` then triggers 100
rapid file changes without reading in between could observe fewer `update` events
than changes made. The test should either trigger one change at a time and drain
before the next, or assert "received at least one update," not an exact count.

## 3. Sorting on async/not-always-present data — the "jumping list" risk

This is a real and differentiated risk between AC-2 and AC-3, because the two
components get their data very differently:

- **`insights/SessionsTable.tsx` (AC-3) — low risk.** Its `sessions` prop is already
  a fully-materialized `SessionTokenSummary[]` array passed down whole from the
  dashboard's one-shot `GetInsightsSummary`/`ListSessionTokens` call — there is no
  per-row async fetch happening after initial render. The existing sort
  (`displayed = useMemo(() => [...result].sort(...), [...])`, currently hardcoded to
  `lastMessageAt`) is exactly the right place to add a user-selectable sort key;
  extending it to `cost`/`tokens` needs no new async plumbing and cannot itself cause
  reordering-while-reading, because all rows have their cost/token fields present
  from the same response that produced the list. **Also notable:** the backend RPC
  already supports this — `ListSessionTokensRequest.sort_by` (`proto/session/v1/insights.proto`,
  values `"cost" | "tokens" | "date"`) plus `sort_desc` already exist and are unused by
  this component, which currently does its own hardcoded client-side sort instead.
  AC-3 may be cheaper to satisfy by wiring the existing `sort_by`/`sort_desc` request
  fields through than by duplicating sort logic client-side — worth checking whether
  `ListSessionTokens` (vs `GetInsightsSummary`) is the actual data source before
  building a second sort implementation.

- **`SessionList.tsx` (AC-2) — real risk, and the harder half of this gap.** The
  `Session` proto message (`proto/session/v1/types.proto`) that backs
  `SessionList.tsx`'s `sessions: Session[]` prop carries **no token or cost fields at
  all**. Confirmed independently: `TokenBadge.tsx` (the "session card token badge,"
  listed as "Done, used in SessionList.tsx" in the pre-implementation triage) is a
  pure presentational component (`costUsd` prop in, pill out) that is **not actually
  rendered anywhere** in the app outside its own unit test —
  `grep -rn "TokenBadge" web-app/src` turns up only the definition file and its test
  file, zero usage sites. So the triage's "Done" status for the token badge is
  inaccurate: the component exists, but the per-session cost data has never been
  plumbed into `SessionList.tsx` to feed it. This means AC-2 is not "add a sort
  option to an existing sorted-and-displayed value" — it requires a **new data
  merge**: cost/token data will have to be fetched (most likely a
  `ListSessionTokens`/`GetInsightsSummary` call keyed by session ID) and joined onto
  the `Session[]` array that already renders, and that fetch will almost certainly
  resolve *after* the initial (fast, tmux/session-metadata-only) session list paint.
  That is exactly the setup for the classic "jumping list" bug: sort by cost with
  most rows initially cost-less (data still loading) — they'd render in whatever
  order the join fell out in, then reflow/jump as costs trickle in and the sort
  recomputes underneath a user who's mid-read or mid-click.

  **No existing pattern in this codebase directly solves this** (grepped for
  `stable sort|jumping|reorder|layout shift` across `web-app/src` — the one hit,
  `backlog/page.tsx:481`, is about a *new item* animating into its natural position,
  not about re-sorting a list as async values arrive). Given that gap, the design
  should borrow the general pattern already implicit in `SessionList.tsx`'s existing
  sort UI (lines ~1114-1134: a `sortField`/`sortDir` pair persisted to
  `localStorage`, applied via a single `useMemo`) and add one deliberate guard not
  currently needed for the existing sort fields (name/created/updated, which are all
  present synchronously): **do not resort mid-render as cost data streams in for
  individual rows.** Concretely: freeze the sort order once computed for a given
  render pass (e.g. sort using `costUsd ?? -1` so unloaded rows sort to a stable
  position rather than shuffling in place — see `TokenBadge`'s own numeric-cost prop
  contract, which already assumes cost is a plain number, not a nullable/pending
  value, implying whoever wires this join should resolve "not yet loaded" to a
  defined sentinel before it reaches sort comparators, not leave it undefined and let
  `Array.sort`'s ordering of `undefined` do something implementation-defined).

## 4. Registry pitfall: `docs/registry/schema.json` does not actually describe frontend entries

`.claude/rules/feature-registry.md`'s own example frontend JSON block:

```json
{
  "id": "kebab-case-id",
  "type": "frontend",
  "name": "Component name",
  "filePath": "web-app/src/...",
  "tested": false,
  "testIds": []
}
```

...does **not** match either `docs/registry/schema.json` (which only defines a
`Feature` object whose `type` enum is `["backend"]` — line 35 — with a required
nested `backend` object; there is no `frontend`/`FrontendDetails` definition in the
file at all, despite the file's own title being generic "Backend Feature Registry")
or the shape real frontend files under `docs/registry/features/frontend/*.json`
actually use in practice. Inspecting existing committed frontend entries (e.g.
`backlog-category-selector.json`, `backlog-pipeline-mode-selector.json`) shows the
**actual required shape** the generator (`make registry-generate-frontend`,
Makefile target) and its validator (`tools/scanner/validate-registry.sh`, invoked by
`make registry-diff`) expect:

```json
{
  "id": "kebab-case-id",
  "type": "frontend",
  "name": "Human-readable feature name",
  "component": "ComponentName",
  "path": "web-app/src/path/to/Component.tsx",
  "filePath": "web-app/src/path/to/Component.tsx",
  "markerLine": 0,
  "tested": true,
  "testIds": ["Exact describe/test string(s) from the Jest file"]
}
```

Key differences from both `schema.json` and the rule doc's example — both of which
are **stale/incomplete** relative to what's actually enforced:
- `component` (bare component name) **and** `path`/`filePath` (identical values,
  both present) are both required in practice — the rule doc's example only shows
  `filePath`.
- `markerLine` (integer, line number of the `// +feature:` marker, `0` if not found
  via automated scan) is present in every real entry but absent from both the
  schema and the rule doc's example.
- `testIds` values are **exact** `describe > it` string concatenations from the Jest
  file (see `backlog-category-selector.json`'s entries, which read like
  `"BacklogItemForm — category selector > selecting a category..."`), not bare
  function names as the backend convention uses.

**Action for AC-5:** don't write the 5 new frontend registry files (`SessionDetailDrawer`,
`ProjectedCostCard`, `DailySpendChart`, `ModelOverTimeChart`, `SessionsTable`) against
`schema.json` or the rule doc's example — copy the field set from an existing
committed frontend file (e.g. `backlog-pipeline-mode-selector.json`) and confirm
placement with `make registry-diff` before running the real `make registry-generate`,
since `docs/registry/schema.json` will not catch a malformed frontend entry (it has
no frontend definition to validate against) and a shape mismatch would only surface
downstream in the generator/validator or silently produce a broken aggregate file.
Also note 3 of the 5 targets (`ProjectedCostCard`, `DailySpendChart`,
`ModelOverTimeChart`) already have `.test.tsx` files in
`web-app/src/app/insights/` — their `testIds` can and should be populated
(`tested: true`) from day one rather than filed as `tested: false` placeholders,
since untested-but-testable entries are exactly what `coverage-gaps.json` flags per
AC-5's "no unexplained growth" condition.

## 5. Test flakiness: fsnotify-driven tests, and the specific problem for AC-4 (`WatchInsights`)

Two distinct patterns already exist in this codebase for fsnotify-adjacent testing,
and one **already-solved structural blocker** that directly applies to `WatchInsights`.

**Pattern A — direct fsnotify watcher tests** (`session/history_watcher_test.go`):
drive a real `fsnotify.Watcher` against a real `t.TempDir()`, write a file, and use
`testify`'s `assert.Eventually(fn, timeout, pollInterval, msg)` for positive
assertions ("the callback fired") — generous timeout (2s), short poll (50ms). For
*negative* assertions ("the callback should NOT fire for non-.jsonl files"), the repo
uses a different, bounded-wait helper (`testutil/wait.WaitForCondition`) with a
short, fixed timeout (200ms) rather than `assert.Eventually` — because a negative
assertion has no positive event to poll for; it can only wait out a short window and
then check the condition never became true. This is the correct pattern to reuse for
any AC-4 test asserting "no update event fires for X" (e.g. non-.jsonl files, or the
`agent-*.jsonl` exclusion already present in `TokenStore.OnHistoryFileChanged`).

**Pattern B — decoupling streaming RPC tests from the concrete `connect.ServerStream[T]`
type entirely, avoiding fsnotify/timing in the RPC-layer test altogether**
(`server/services/backlog_service_events.go` / `_test.go`, Epic 3.2 of
`backlog-event-driven-updates`): this is the more important precedent for AC-4, and
it's a **structural**, not just timing, fix. Its doc comment states directly:

> `connectrpc.com/connect` v1.19.0's `ServerStream[Res]` is a concrete struct with an
> unexported `conn` field and no exported constructor, so a hand-rolled fake
> `ServerStream[T]` is not buildable outside the connect package.

The fix: extract the RPC handler's core logic into an unexported method
(`watchBacklogItems`) that accepts a narrow, package-local interface
(`backlogItemEventSender`, one method: `Send(*sessionv1.BacklogItemEvent) error`)
instead of the concrete `*connect.ServerStream[T]`. The exported `WatchBacklogItems`
RPC method stays a two-line wrapper that just forwards to the core method with the
real stream. `*connect.ServerStream[T]` already has a matching `Send` method, so it
satisfies the interface structurally with zero adapter code in production, while
tests pass a hand-rolled, mutex-guarded fake sender that records every sent message —
sidestepping both the un-constructibility problem *and* most of the fsnotify-timing
flakiness class, because the fake sender lets the test drive/assert the event
fan-out synchronously instead of racing a real filesystem watcher.

**`server/services/insights_service.go`'s current `WatchInsights` has the identical
problem `backlog_service_events.go` already solved once**: it takes a concrete
`stream *connect.ServerStream[sessionv1.InsightsEvent]` directly (lines 491-494) with
all logic inline — no extracted core method, no sender interface. AC-4's test cannot
be written directly against `WatchInsights` as it stands today for the same reason
noted in `unfinished_work_test.go`'s comment about `Scanner`'s fsnotify-backed cache
having "no exported test seam reachable from this package" — before writing the
test, the handler needs the same refactor: extract an unexported `watchInsights(ctx,
sender insightsEventSender) error` core method (interface: `Send(*sessionv1.InsightsEvent)
error`), keep `WatchInsights` as the thin public wrapper. That refactor is a
prerequisite for AC-4, not an optional nicety — attempting to test the RPC as
currently structured means either standing up a real ConnectRPC server+client (heavy,
and still timing-sensitive over a real network stream) or not testing the actual
streaming path at all.

Once that seam exists, the remaining timing risk is genuinely small and bounded:
`TokenStore.Subscribe()`/`notify()` is in-process, non-blocking, buffered
(`subChanSize = 64`) — a test can call `store.Subscribe()`, trigger exactly one
`parseAndCache` (either via a real temp-dir fsnotify write using Pattern A's
`assert.Eventually`, or more directly by calling the store's own already-tested
enqueue/parse path once) and assert the fake sender received one `"update"` event
with `assert.Eventually`, avoiding both a real network round-trip and multi-event
buffer-drop ambiguity (§2 above).

## Summary of design constraints this implies for the plan

1. **AC-1:** exclude/label `<synthetic>` turns in the per-turn table the same way
   #280 excluded them from model-family aggregation; the table's sum should
   reconcile against the session-level total already shown.
2. **AC-2:** requires new data plumbing (`Session[]` has no cost/token fields today —
   `TokenBadge` is unwired, not "Done" as triaged); design the join to resolve
   "not yet loaded" to a defined sentinel before sorting, and do not resort rows
   silently as costs stream in mid-scroll.
3. **AC-3:** lower risk — data is already batch-loaded; prefer wiring the
   already-existing `ListSessionTokens.sort_by`/`sort_desc` request fields over
   duplicating a second client-side sort implementation.
4. **AC-4:** requires extracting `WatchInsights`'s core logic behind a narrow
   `insightsEventSender` interface (mirroring `backlogItemEventSender`) before any
   test can be written against it; use `assert.Eventually` for positive event
   assertions and `testutil/wait.WaitForCondition` for negative ones.
5. **AC-5:** copy field shape from an existing committed frontend registry file
   (not `schema.json`, not the rule doc's example — both are stale/incomplete);
   populate real `testIds` for the 3 targets that already have `.test.tsx` files.
6. **AC-6:** no backend change needed — `ModelBreakdown` (in `GetInsightsSummaryResponse`)
   already carries `cache_read_tokens` and `total_input_tokens` per model family, so
   cache hit rate (`cache_read / (input + cache_read)`, same formula already used at
   the session level) is a pure frontend computation from data already returned.
