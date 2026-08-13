# Architecture Research — Token Cost Tracking Gap Closure

Scope: how to close AC-1..AC-6 without violating `.claude/rules/interface-pollution-checklist.md`
or `.claude/rules/go-double-checked-locking.md`. No prior architecture-review artifact existed
for `session/tokens/*` or `insights_service.go`; this is the first pass.

## AC-1 — Per-turn breakdown

**`TurnStats` is not exposed over ConnectRPC at all.** It lives only in
`session/tokens/types.go` as `ParseResult.TurnTimeline []TurnStats` (fields: `Timestamp`,
`Model`, `Input`, `Output`, `CacheCreation`, `CacheRead`, `ToolNames`). Neither
`SessionTokenSummary` nor any other message in `proto/session/v1/insights.proto` carries
per-turn data — `ListSessionTokens`/`GetInsightsSummary` only return session-level rollups.
`sessionTimestamps()` in `insights_service.go:536` already iterates `TurnTimeline` internally
(to derive first/last message time) but discards everything else per-turn.

**Minimal change**: add a new message `TurnTokenStat` (mirrors `TurnStats` 1:1) and a
`repeated TurnTokenStat turn_timeline = N` field, most naturally on `SessionTokenSummary` since
that's the message keyed to one session. Reusing `ListSessionTokens` for this would bloat every
list-page response with per-turn data most callers don't need — prefer either:
- a new unary RPC `GetSessionTurnTimeline(session_id) -> repeated TurnTokenStat`, called only
  when `SessionDetailDrawer` opens for a session, or
- adding `turn_timeline` as an *optional*, not-populated-by-default field on
  `SessionTokenSummary`, gated by a request flag (`include_turn_timeline` on
  `ListSessionTokensRequest`), fetched via a single-session lookup.

A new RPC is cleaner (keeps `ListSessionTokens` response shape stable for the list view) and
matches the existing shape of `GetProviderLimits` (single-session, on-demand query already in
`session.proto`). Recommend: new RPC, not a bolt-on field.

## AC-2/AC-3 — Sortable lists

**`ListSessionTokens` already does server-side sort** (`insights_service.go:424-452`): `sortBy`
∈ `{"cost","tokens","date"}` + `sortDesc` are already request fields on
`ListSessionTokensRequest` (`insights.proto:116-123`) and already implemented. No backend change
needed for sorting logic itself — it exists and works.

**`insights/SessionsTable.tsx`** (AC-3): receives `sessions: SessionTokenSummary[]` as a prop
(already fully loaded with Input/Output/Cache/Cost per session — confirmed columns at
`SessionsTable.tsx:120-126`) and sorts **client-side**, hardcoded to `lastMessageAt`
(`SessionsTable.tsx:97-99`). This is a pure frontend change: add sort-field state + click
handlers on the `<th>` elements, sort the already-present data client-side. No RPC round-trip
required — the existing `sortBy`/`sortDesc` request fields could be used instead, but since the
full page of data is already client-resident, client-side re-sort is simpler and consistent with
the existing (if hardcoded) pattern. **No proto/backend change needed for AC-3.**

**`SessionList.tsx`** (AC-2) is architecturally different: it sorts a `Session[]` (the
session-management/tmux domain type from `session.proto`, not `SessionTokenSummary`), and that
type carries **no token/cost fields at all** — confirmed by grepping `types.proto`/`session.proto`
for token/cost fields on `Session`; the only token-shaped fields found belong to
`ProviderLimitsProto` (`session.proto:2581-2598`, `GetProviderLimits` RPC), which is unrelated
rate-limit telemetry, not aggregated spend. `TokenBadge.tsx` — cited in requirements.md as
"used in SessionList.tsx" — is **not actually imported anywhere** except its own test file;
that line in requirements.md is stale/inaccurate.

To sort `SessionList.tsx` by cost, the component needs per-session cost joined in. Two options:
1. Fetch `ListSessionTokens` (or `GetInsightsSummary`) from `SessionList.tsx` and merge
   `SessionTokenSummary.estimated_cost_usd` into the sort comparator by `session_id`, keyed off
   `SessionTokenSummary.session_id` (already populated via the associator in
   `insights_service.go:376-379`).
2. Add a lightweight cost field directly onto `Session`/`ListSessions` — rejected: duplicates
   data that already has a canonical home in `SessionTokenSummary`, and `Session`/`ListSessions`
   already has its own large surface (session-creation-registry territory) that shouldn't grow
   token/cost concerns per single-responsibility.

Recommend option 1 — `SessionList.tsx` fetches from `InsightsService` (same client the
`/insights` page already uses) and does an in-memory join by `session_id`, extending the
existing client-side `sortedSessions` `useMemo` (`SessionList.tsx:586-612`) with a `'tokenCost'`
case in the `SortField` union (`SessionList.tsx:97`). No proto/backend change; it's a new
frontend data dependency, not a schema change.

## AC-4 — WatchInsights test

No existing test file for `InsightsService` (`server/services/insights_service.go` has zero
`_test.go` sibling). Two streaming test patterns already exist in this repo, and they differ by
transport requirement — pick the one matching WatchInsights's actual RPC shape:

- **Bidi pattern** (`session_service_stream_terminal_test.go`, `newBidiStreamTestServer`):
  `httptest.NewUnstartedServer` + `EnableHTTP2 = true` + `StartTLS()`. Required only for true
  bidirectional streams like `StreamTerminal`.
- **Server-stream pattern** (`headless_service_test.go`, `TestHeadlessService_RunHeadlessCall_*`):
  plain `httptest.NewServer(mux)` (HTTP/1.1, no TLS/HTTP2 needed) + client-side
  `for stream.Receive() { ... }` loop.

`WatchInsights` is unary-request → server-stream (`rpc WatchInsights(WatchInsightsRequest)
returns (stream InsightsEvent)`), structurally identical to `RunHeadlessCall`, not to
`StreamTerminal`. **Follow the `headless_service_test.go` pattern exactly**: register
`sessionv1connect.NewInsightsServiceHandler` (already wired in `server/server.go:364`) on a plain
`httptest.NewServer`, call `WatchInsights`, then loop `stream.Receive()` — inject a fake/small
`TokenStoreReader` (the interface already exists at `session/tokens/types.go:57-63`, purpose-
built for this: `GetAll/GetByUUID/IsLoading/Subscribe/Unsubscribe`) and push through its
`Subscribe()` channel to trigger the "update" event branch (`insights_service.go:518-529`),
verifying the client receives an `InsightsEvent{EventType: "update"}`. Then flip
`docs/registry/features/backend/WatchInsights.json` `tested: true` with the new test's name in
`testIds`.

## AC-5 — Registry entries

Confirmed pure metadata/config change — no runtime architecture impact. Create 5 new
`docs/registry/features/frontend/<component>.json` files (`SessionDetailDrawer`,
`ProjectedCostCard`, `DailySpendChart`, `ModelOverTimeChart`, `SessionsTable`) per the schema in
`.claude/rules/feature-registry.md`, then run `make registry-generate`. No code path is touched.

## AC-6 — Aggregate-level cache hit rate

Per-model cache data **is already aggregated server-side** — `ModelBreakdown`
(`insights.proto:68-78`) has `cache_read_tokens` and `total_input_tokens` per model family,
computed in `GetInsightsSummary`. What's missing is only the **derived ratio**: no
`cache_hit_rate` field on `ModelBreakdown` (unlike `SessionTokenSummary`, which already has one
at `insights.proto:33`, computed via `computeCacheHitRate()` in `insights_service.go:551-558`).

Two placement options:
1. Add `double cache_hit_rate = 8` to the `ModelBreakdown` proto message, computed server-side by
   reusing the existing `computeCacheHitRate(totalInput, cacheRead)` helper at the same point
   `ModelBreakdown` entries are built in `GetInsightsSummary` — consistent with how
   `SessionTokenSummary.cache_hit_rate` is already computed, avoids a second computation path.
2. Derive it client-side in `ModelBreakdownChart.tsx`/`ModelOverTimeChart.tsx` from the raw
   `cache_read_tokens`/`total_input_tokens` fields already present on `ModelBreakdown` — no
   backend change, pure frontend arithmetic (`cacheRead / (totalInput + cacheRead)`), identical
   one-line formula to what the Go helper already does.

Recommend **option 2 (frontend derivation)**: `computeCacheHitRate` is a pure, stateless
one-liner (`cacheRead / (input + cacheRead)`, `insights_service.go:551-558`) — the raw inputs it
needs are already on the wire in `ModelBreakdown`. Adding a server-side field for a value the
client can derive from data it already has is the kind of redundant-plumbing this repo's
interface-pollution conventions push against implicitly (no-op forwarding of a value that adds no
new information, just router boilerplate). Reserve the proto change (option 1) only if this ratio
needs to be *sorted or filtered on* server-side later — not currently a requirement.

## Interface-pollution / idiom check

None of the above changes introduce a new Go interface, wrapper type, or generic — they are
proto field/RPC additions, a Go test file, and frontend-only logic. `TokenStoreReader`
(`session/tokens/types.go:57-63`) already exists as the correct "interface defined at the
consumer" pattern for the WatchInsights test's fake — no new interface needed for AC-4. No
double-checked-locking-shaped code appears in any of these gaps (no cache-recompute-under-lock
pattern is being touched).

## Touchpoint scope confirmation

**Closing all 5 gaps does NOT require touching `server/services/session_service.go` or
`session/instance.go`.** Confirmed explicitly: every gap is read-path/display-only against
`InsightsService`/`session/tokens/*` (a separate service from `SessionService`) and frontend
components under `web-app/src/app/insights/` and `web-app/src/components/sessions/`. No new
session creation mode, no `SessionType` enum value, no `CreateSessionRequest` field — the
7-touchpoint session-creation registry (`.claude/rules/session-creation-registry.md`) does not
apply to this work. The only proto file touched is `proto/session/v1/insights.proto` (new
`TurnTokenStat` message + RPC for AC-1); `proto/session/v1/session.proto`
(session-creation-registry's proto file) is untouched.

## Summary of concrete recommendations

| AC | Backend change | Frontend change |
|---|---|---|
| AC-1 | New `TurnTokenStat` message + new unary RPC (e.g. `GetSessionTurnTimeline`) in `insights.proto` | New table in `SessionDetailDrawer.tsx` calling it on drawer open |
| AC-2 | None | `SessionList.tsx` fetches `ListSessionTokens`, joins by `session_id`, adds `'tokenCost'` to `SortField` |
| AC-3 | None (existing `sortBy`/`sortDesc` request fields already unused-but-available as an alternative) | `SessionsTable.tsx` adds sort state + `<th>` click handlers over already-present props |
| AC-4 | New `insights_service_test.go` following `headless_service_test.go`'s plain-`httptest.NewServer` + `stream.Receive()` pattern (not the bidi/TLS pattern) | None |
| AC-5 | None | 5 new `docs/registry/features/frontend/*.json` files + `make registry-generate` |
| AC-6 | None (recommended) | `ModelBreakdownChart.tsx`/`ModelOverTimeChart.tsx` derive hit rate client-side from existing `cache_read_tokens`/`total_input_tokens` |

`make proto-gen` is required only for AC-1 (new message + RPC in `insights.proto`). No other AC
touches the proto layer.
