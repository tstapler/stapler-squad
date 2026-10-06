# Implementation Plan: escape-analytics-global-view

**Phase**: 3 (Plan) · **System type**: Web app — Go ConnectRPC backend + React/TypeScript frontend, ent ORM over a dedicated SQLite analytics DB (separate from the main session store).

## Step 0.5 — Creative Pass: Alternative Approaches Considered

| # | Approach | Sketch | Rejected? | Rationale |
|---|---|---|---|---|
| A | **Two independent ent `GroupBy`+`Aggregate` queries** sharing one `[]predicate.EscapeEvent` time filter — `GroupBy(FieldSequenceType).Aggregate(ent.Count(), ent.As(ent.Sum(FieldMangled),"mangled_count"))` for the histogram, `GroupBy(FieldSessionID)` with the same aggregates for the per-session breakdown; grand totals folded in Go from the small histogram row set. | See `research/architecture.md` code sketch | **Chosen** | Two round trips against a read-only, indexed, low-QPS analytics DB is cheap; each query does real `GROUP BY` in SQLite (satisfies the NFR); result sizes scale with distinct-type-count / distinct-session-count, not event count; no query tries to serve two different grouping keys at once, so no artificial cross-product. |
| B | **Single combined query**: one `GroupBy(FieldSessionID, FieldSequenceType, FieldMangled)` producing a `session_id × sequence_type × mangled` cross-product, then fold both the histogram and the per-session breakdown out of one result set in Go. | `GroupBy(sid, seqType, mangled).Aggregate(ent.Count())` → one row per distinct (session, type, mangled) triple | **Rejected** | Row count scales with `sessions × sequence_types × 2`, not bounded by either dimension alone — for many sessions with several sequence types each, this is a larger result set than either of approach A's two queries for no round-trip savings that matters at read-only/low-frequency scale. It also forces two independent Go-side fold passes over one row shape instead of one pass each over a query shaped for that purpose — no simpler than A, and the schema of the destination struct becomes a three-key group that's harder to unit-test in isolation. Two clear single-purpose queries is preferred to one query trying to serve two different UI shapes. |
| C | **Extend `GetEscapeAnalyticsSummary` with an `all_sessions`/omit-`session_id` flag**, reusing the existing handler and its in-Go `Select(...).All(ctx)` + map-loop aggregation. | Add `bool all_sessions` to the existing request; skip the `session_id` predicate when set. | **Rejected** | This is exactly the anti-pattern the requirements doc's NFR forbids repeating at all-sessions scale — the existing handler's data-access strategy is "pull every matching `EscapeEvent` row into Go memory, then aggregate with a map and a loop" (`server/services/analytics_escape_service.go:147-169`). Fine at single-session scale (bounded by one session's event count); not safe once "all sessions" removes that bound. `research/build-vs-buy.md` §4 independently reached the same verdict: adapt the *shape* (validation, response fields, guarded division) of `GetEscapeAnalyticsSummary`, but do not fork or extend its data-access loop. Also fails the "no per-session breakdown" gap — the existing handler has no concept of a per-session grouped result at all, since `session_id` is a fixed input, not a group key. |

**Frontend toggle — approaches considered:**

| # | Approach | Rejected? | Rationale |
|---|---|---|---|
| D | Two-tab `role="tablist"` (`"Per-Session"` / `"All Sessions"`), reusing the roving-focus pattern already in `SessionDetailView.tsx:571-612`, driving a `viewMode: "per_session" \| "all_sessions"` state var. | **Chosen** | Established, accessible pattern already in this codebase; `ux.md` explicitly recommends tabs over a dropdown or checkbox for this exact binary view switch. |
| E | Checkbox/toggle switch ("Show all sessions") layered on top of the existing session dropdown, leaving the dropdown always visible. | Rejected | `ux.md` flags this as inferior: the session dropdown becomes meaningless/misleading once "all sessions" is checked (do you keep the last-selected session highlighted? disable the dropdown? both are worse UX than hiding it), and a checkbox doesn't communicate "two views of the same data" as clearly as two tabs. |
| F | Unmount/remount the per-session hooks' consumer components on tab switch (rely on React's automatic promise abandonment on unmount) rather than adding an `enabled` flag. | Rejected | `pitfalls.md` §5 notes this needlessly recreates the ConnectRPC client (`clientRef` is set in a mount-only effect) on every tab switch, and does not, by itself, prevent the console warning / wasted `setState` from an in-flight request unless a `cancelled`-flag guard is *also* added — so it buys nothing over the `enabled`-flag approach while adding remount churn if tab-switching is frequent. |

## Step 1 — System Type

Web application: Go backend (ConnectRPC handlers in `server/services/`, ent ORM v0.14.5 generating type-safe query builders over a dedicated SQLite analytics DB distinct from the main session-state DB) + React 19/TypeScript SPA frontend (`web-app/`, hand-rolled hooks over `@connectrpc/connect-web`, vanilla-extract CSS-in-TS).

## Step 2 — Domain Glossary

| Term | Go/TS identifier | Definition |
|---|---|---|
| Global escape analytics summary | `GetEscapeAnalyticsGlobalSummary` (RPC), `GetEscapeAnalyticsGlobalSummaryResponse` | Aggregate escape-sequence statistics across **all** sessions (optionally time-filtered), as opposed to the existing single-session summary. |
| Sequence histogram | `Histogram []*EscapeSequenceCount` | Counts of escape sequences grouped by `sequence_type`, existing message reused unchanged. |
| Per-session breakdown | `PerSession []*SessionEscapeSummary` | One row per session that has ≥1 matching `EscapeEvent`, with that session's own totals/mangle rate — the "spot the outlier" table. |
| Session escape summary (row) | `SessionEscapeSummary{SessionId, TotalSequences, TotalMangled, MangleRate}` (new proto message) | A single per-session-breakdown-table row. |
| Mangle rate | `MangleRate float64` | `totalMangled / totalSequences`, guarded to `0` when `totalSequences == 0`; computed both globally and per breakdown row. |
| Escape event | `ent.EscapeEvent` / `escapeevent.*` field constants | A single recorded terminal escape-sequence parse event (existing entity, `session/ent/schema/escape_event.go`). |
| Time-range filter | `StartTime`, `EndTime *timestamppb.Timestamp` (request fields) | Optional inclusive bounds (`WallTimeGTE`/`WallTimeLTE`) applied identically to both aggregate queries. |
| View mode | `viewMode: "per_session" \| "all_sessions"` (frontend state) | Which tab of `EscapeAnalyticsPage` is active; gates which hooks are `enabled`. |
| Hook enable flag | `enabled?: boolean` (new hook param, default `true`) | Suspends a hook's fetch effect and exposed callbacks without unmounting its consumer, per the chosen pause strategy. |
| Stale-request guard | `cancelled` (closure-scoped bool in an effect) | Prevents a late-resolving fetch from a superseded render/tab-state from calling `setState`; existing pattern in `useEscapeEvents`, to be backported into `useEscapeAnalyticsSummary` and reused in the new hook. |
| Analytics client | `SessionService.analyticsClient *ent.Client` | The ent client bound to the separate analytics SQLite DB; `nil` until `SetAnalyticsClient` is called — same guard reused by the new handler. |
| GroupBy/Aggregate builder | `Query().GroupBy(field...).Aggregate(fns...).Scan(ctx, &dest)` | ent's generated real-SQL `GROUP BY` mechanism (`session/ent/escapeevent_query.go:259-305`); the chosen aggregation mechanism (Pattern Decision, Step 3). |
| Dominant contributor / outlier session | (UI-only concept, no new identifier) | The breakdown row with the highest mangle rate — surfaced via default descending sort + `MangleRateIndicator` reuse, not a distinct backend field. |

## Step 3 — Pattern Decisions

### 3a. Aggregation mechanism (resolves requirements.md's Rabbit Hole / Open Question)

**Chosen**: ent's generated `GroupBy(...).Aggregate(ent.Count(), ent.As(ent.Sum(escapeevent.FieldMangled), "mangled_count")).Scan(ctx, &dest)`, run as **two independent queries** sharing one `[]predicate.EscapeEvent` time-filter slice (histogram grouped by `sequence_type`, breakdown grouped by `session_id`). This is Approach A from Step 0.5.

- **Rejected alternative**: raw `sql.Selector`/hand-written SQL escape hatch. `research/build-vs-buy.md` §1 and `research/pitfalls.md` confirm ent's generated builder already executes real `GROUP BY` SQL (verified against `session/ent/escapeevent_query.go:259-305`'s `sqlScan`), and it absorbs SQLite dialect quirks (`bool`-as-`INTEGER`, no `FILTER (WHERE ...)`) that a hand-written query would need to special-case. No injection surface either way since all identifiers are typed ent field constants — reaching for raw SQL here would be introducing a lower-level tool with strictly more footguns for zero benefit (an "unjustified generic" in spirit, per `.claude/rules/interface-pollution-checklist.md`'s over-engineering smells).
- **`ent.Sum(FieldMangled)`** (single-key `GroupBy(sequence_type)` with a summed boolean-as-integer column) is chosen over the two-key `GroupBy(sequence_type, mangled)` + Go-side fold approach stack.md raised — `architecture.md`'s worked code sketch uses `ent.Sum`, and it needs one fewer Go-side fold step per histogram bucket (one row per sequence type with two aggregate columns, vs. two rows per sequence type needing a merge). Both are valid `GROUP BY` mechanisms and satisfy the NFR; `ent.Sum` is simply the leaner of the two once both were on the table, per PoEAA's preference for a design that pushes computation to the data layer when the data layer already expresses it directly.
- Guarded division (`if totalSeq > 0 { rate = mangled/total } else { rate = 0 }`) is a one-line pure helper, applied **globally once** and **once per breakdown row** — factored as a small unexported helper (`escapeMangleRate(total, mangled int64) float64`) in `analytics_escape_service.go` rather than duplicated inline twice, since it is genuinely called from two sites with identical logic (not a speculative abstraction — see `.claude/rules/interface-pollution-checklist.md` smell #5, "unjustified generic": this is the opposite case, two real call sites for the same 1-line logic, which does earn extraction).

### 3b. Proto message shape

New RPC added to `SessionService` in `proto/session/v1/session.proto`, placed immediately after the existing `GetEscapeAnalyticsSummary` RPC (~line 332):

```protobuf
// GetEscapeAnalyticsGlobalSummary returns aggregate escape sequence statistics
// across all sessions, plus a per-session breakdown to spot outliers.
rpc GetEscapeAnalyticsGlobalSummary(GetEscapeAnalyticsGlobalSummaryRequest) returns (GetEscapeAnalyticsGlobalSummaryResponse) {}
```

New messages (mirroring `GetEscapeAnalyticsSummaryRequest`/`Response` minus the per-session `session_id` input, plus the new breakdown list):

```protobuf
message GetEscapeAnalyticsGlobalSummaryRequest {
  optional google.protobuf.Timestamp start_time = 1;
  optional google.protobuf.Timestamp end_time = 2;
}

message GetEscapeAnalyticsGlobalSummaryResponse {
  repeated EscapeSequenceCount histogram = 1;
  int64 total_sequences = 2;
  int64 total_mangled = 3;
  double mangle_rate = 4;
  repeated SessionEscapeSummary per_session = 5;
}

message SessionEscapeSummary {
  string session_id = 1;
  int64 total_sequences = 2;
  int64 total_mangled = 3;
  double mangle_rate = 4;
}
```

`EscapeSequenceCount` is the existing message reused verbatim (no changes needed). This is a **type-driven design**, PoEAA-"Data Transfer Object"-shaped choice: `SessionEscapeSummary` is a small, explicit value object with 4 same-shaped fields as `GetEscapeAnalyticsSummaryResponse`'s top level — declared as its own named message (not a bare `map<string, double>` or 3 parallel `repeated` scalar fields) specifically to avoid the primitive-obsession-adjacent smell of parallel arrays, per `.claude/rules/primitive-obsession-checklist.md`'s spirit applied to wire messages.

### 3c. Frontend state management

- **Tab pattern**: reuse `SessionDetailView.tsx:571-612`'s `role="tablist"` + roving `ArrowLeft`/`ArrowRight` focus pattern (Approach D, Step 0.5), driven by `viewMode: "per_session" | "all_sessions"` local state in `EscapeAnalyticsPage.tsx`. The reused pattern is missing `aria-controls`/`aria-labelledby` wiring on its tab buttons/panels today (confirmed by reading the referenced lines — no such attributes present); the new implementation adds them (`aria-controls="tabpanel-{id}"` on each tab button, `id="tabpanel-{id}"` + `role="tabpanel"` + `aria-labelledby="tab-{id}"` on each panel) since `ux.md` calls this out as required for the full ARIA Tabs pattern, and it costs nothing extra here vs. propagating the existing gap.
- **Hook suspension**: add an `enabled: boolean = true` parameter to **both** `useEscapeAnalyticsSummary` and the new `useEscapeAnalyticsGlobalSummary`, guarding the fetch effect and any exposed `refresh()` callback. `viewMode === "all_sessions"` maps to `enabled: false` on the per-session hooks (`useEscapeAnalyticsSummary`, `useEscapeEvents`) and `enabled: true` on the new global hook, and vice versa — never both enabled at once. Chosen over unmounting (Approach F, Step 0.5 — rejected).
- **Cancellation-guard backport**: `useEscapeAnalyticsSummary`'s fetch effect currently has no cleanup/`cancelled` flag (`web-app/src/lib/hooks/useEscapeAnalytics.ts:64-66`) unlike `useEscapeEvents` (lines 110-151, which has the correct `let cancelled = false; ...; return () => { cancelled = true; }` pattern). Both `useEscapeAnalyticsSummary`'s effect and the new `useEscapeAnalyticsGlobalSummary`'s effect get this same guard, closing the exact race `pitfalls.md` §"React hook" flags: a switch away from a session (or away from "All Sessions") must not let an in-flight response overwrite state for a view the user has since left.
- **Clearing stale state**: when `enabled` transitions to `false`, the hook does **not** need to clear its own state (its output simply isn't rendered while its tab is inactive — the panel is unmounted via `viewMode` conditional rendering, per ARIA tabpanel convention), but the `cancelled` guard above still prevents the wasted/stale `setState` call itself from firing after the fact.

## Step 3.5 — Unresolved Questions Now Resolved

**Per-session breakdown pagination (requirements.md's Open Question, explicitly deferred to Phase 3)**: **Resolved — no pagination for MVP.** Rationale, consistent with `ux.md`'s explicit recommendation and `architecture.md`'s structural note:
- The breakdown query is `GROUP BY session_id`, so row count is bounded by *distinct sessions with ≥1 matching escape event in the filtered time range* — not total event count. This is materially smaller than an unbounded event-scale result set, and matches the existing Insights Dashboard precedent, which also renders its full per-session table unpaginated.
- If real-world distinct-session counts prove this wrong (e.g. thousands of one-off sessions each with a handful of events), the fix is additive (client-side virtualization or a `LIMIT`/cursor added to the existing `GroupBy` query) and does not require a proto/handler redesign — deferring the *decision to add pagination* to a follow-up is safe; deferring the *decision of whether MVP needs it* is not, since it blocks writing the frontend table component now.
- Documented as a follow-up trigger, not an ADR: "if `len(PerSession) > ~200` becomes common in practice, revisit pagination or client-side virtualization for the breakdown table."

## Migration Plan

No schema migration. No new proto file. One proto file edited (`session.proto`), regenerated via `make proto-gen` (regenerates `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`). No `session/ent/schema/` changes — `EscapeEvent`'s existing fields and indexes (`session_id`, `sequence_type`, `mangled`, `wall_time`) already cover both new queries; confirmed no new index needed.

## Observability Plan

Not required — complexity 2 per requirements.md. Standard ConnectRPC interceptor/middleware logging (already applied to every `SessionService` method) is sufficient; no new metrics, traces, or dashboards are added for this feature.

## Risk Control

Not required — this is a read-only, additive feature (new RPC, new frontend tab) with no writes, no schema migration, and no changes to any existing RPC's behavior or response shape. Existing `GetEscapeAnalyticsSummary`/`QueryEscapeAnalytics` behavior is untouched. No feature flag or rollback plan needed beyond a normal revert.

## Dependency Visualization

```
Epic 1: Backend RPC
  Story 1.1 (proto) ──► Story 1.2 (make proto-gen) ──► Story 1.3 (handler) ──► Story 1.4 (backend registry) ─┐
                                                              │                                                │
                                                              └────────────► Story 1.5 (Go unit tests) ◄──────┘
                                                                                    │
Epic 2: Frontend                                                                   │  (handler must exist for
  Story 2.1 (hook cancellation backport, independent) ──────────────────────────┐  │   frontend to call against)
  Story 2.2 (new global hook)  ◄── depends on Story 1.2 (generated TS types) ───┤  │
  Story 2.3 (tab CSS)          ◄── independent, can start anytime ─────────────┤  ▼
  Story 2.4 (EscapeAnalyticsPage toggle wiring) ◄── depends on 2.1, 2.2, 2.3 ──┤
  Story 2.5 (frontend registry) ◄── depends on 2.4 ──────────────────────────────┘
  Story 2.6 (Jest tests for 2.1/2.2/2.4)  ◄── depends on 2.1, 2.2, 2.4

Epic 3: E2E
  Story 3.1 (Playwright spec) ◄── depends on Epic 1 + Epic 2 fully merged (needs a running server + built frontend)
```

Backend (Epic 1) and the independent frontend prep stories (2.1, 2.3) can start in parallel; Story 2.2 needs generated TS types from Story 1.2 (which only needs the proto edit, not the Go handler, to exist); Story 2.4 is the integration point gating everything downstream of it.

## Phase / Epic / Story / Task Hierarchy

### Epic 1 — Backend RPC

#### Story 1.1: Add proto RPC + messages
*Given* the proto file lacks a global escape summary RPC, *when* `GetEscapeAnalyticsGlobalSummary` + its request/response/`SessionEscapeSummary` messages are added to `session.proto`, *then* `make proto-gen` succeeds and generates a `GetEscapeAnalyticsGlobalSummary` method on the generated `SessionServiceClient`/`SessionServiceHandler` Go interfaces and a matching TS client method + message types.

- Task 1.1.1: Edit `proto/session/v1/session.proto` — add the RPC line after the existing `GetEscapeAnalyticsSummary` RPC (~line 332) and the three new messages (`GetEscapeAnalyticsGlobalSummaryRequest`, `GetEscapeAnalyticsGlobalSummaryResponse`, `SessionEscapeSummary`) near the existing `GetEscapeAnalyticsSummaryRequest`/`Response` messages. (1 file)

#### Story 1.2: Regenerate bindings
*Given* the proto edit from 1.1, *when* `make proto-gen` is run, *then* `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` contain the new RPC/message types and `go build ./...` / `pnpm --dir web-app exec tsc --noEmit` both succeed with no other code changes yet.

- Task 1.2.1: Run `make proto-gen`; commit generated files. (2 files: generated `.go` + generated `.ts`, mechanical)

#### Story 1.3: Implement the handler
*Given* the analytics client and time-range predicate pattern already established by `GetEscapeAnalyticsSummary`, *when* `GetEscapeAnalyticsGlobalSummary` is implemented in `server/services/analytics_escape_service.go` using two `GroupBy`/`Aggregate` queries, *then* it returns correct histogram/totals/per-session data for a multi-session fixture, returns `connect.CodeUnavailable` when `analyticsClient` is `nil`, and never divides by zero.

- Task 1.3.1: Add `// +api: analytics:get-escape-global-summary` marker + method signature + nil-`analyticsClient` guard (mirrors lines 132-134). (1 file)
- Task 1.3.2: Build the shared `timeFilters []predicate.EscapeEvent` slice from optional `StartTime`/`EndTime` (mirrors lines 139-144). (1 file, same as above)
- Task 1.3.3: Run the histogram query — `GroupBy(escapeevent.FieldSequenceType).Aggregate(ent.As(ent.Count(),"count"), ent.As(ent.Sum(escapeevent.FieldMangled),"mangled_count")).Scan(...)` into a destination struct; fold into `[]*sessionv1.EscapeSequenceCount` + running `totalSeq`/`totalMangled`. (1 file, same)
- Task 1.3.4: Run the per-session breakdown query — same aggregate shape, `GroupBy(escapeevent.FieldSessionID)`; fold into `[]*sessionv1.SessionEscapeSummary`, applying the guarded-division helper (Task 1.3.5) per row. (1 file, same)
- Task 1.3.5: Add the shared `escapeMangleRate(total, mangled int64) float64` helper (guards `total == 0`); use it for both the global rate and each per-session row. (1 file, same)
- Task 1.3.6: Assemble and return `connect.NewResponse(&sessionv1.GetEscapeAnalyticsGlobalSummaryResponse{...})`. (1 file, same)
- Task 1.3.7: Confirm RPC registration — `SessionService` (`server/services/session_service.go`) implements the full generated `SessionServiceHandler` interface; a new method added to that struct satisfies the interface automatically (no separate registration call needed, unlike a brand-new service). Verify with `go build ./...` after 1.3.1–1.3.6 — a missing/mis-signatured method fails the interface-satisfaction compile check in `server/server.go` where `sessionv1connect.NewSessionServiceHandler(sessionService)` is constructed. No code change expected here; this task is a verification step, not an edit.

#### Story 1.4: Backend feature registry entry
*Given* the new `// +api: analytics:get-escape-global-summary` marker from 1.3.1, *when* `make registry-generate` is run, *then* `docs/registry/features/backend/analytics/get-escape-global-summary.json` exists with `markerFound: true` and `docs/registry/coverage-gaps.json`'s count does not increase net of the new (initially untested) entry — resolved once Story 1.5 lands and `tested`/`testIds` are populated by a second `make registry-generate` run.

- Task 1.4.1: Run `make registry-generate`; commit the new/changed registry files. (1-2 files, mechanical)

#### Story 1.5: Go unit tests
*Given* a multi-session, multi-sequence-type fixture in the analytics test DB, *when* `TestGetEscapeAnalyticsGlobalSummary_*` tests run, *then* histogram totals, per-session totals, the time-range boundary, the zero-events divide-by-zero guard, and the nil-`analyticsClient` guard are all covered.

- Task 1.5.1: `TestGetEscapeAnalyticsGlobalSummary_should_ReturnUnavailable_When_AnalyticsClientNil` (1 file: `analytics_escape_service_test.go`)
- Task 1.5.2: `TestGetEscapeAnalyticsGlobalSummary_should_AggregateAcrossSessions_When_MultipleSessionsHaveEvents` — 2-3 session fixture, asserts histogram + `total_sequences`/`total_mangled`/`mangle_rate` + `per_session` rows. (same file)
- Task 1.5.3: `TestGetEscapeAnalyticsGlobalSummary_should_ExcludeEventsOutsideBoundary_When_TimeRangeSet` — one event exactly at `WallTimeGTE`/`WallTimeLTE` boundary, one just outside; closes the untested boundary case `pitfalls.md` flags as missing even on the existing per-session handler. (same file)
- Task 1.5.4: `TestGetEscapeAnalyticsGlobalSummary_should_ReturnZeroRate_When_NoEventsMatch` — empty histogram/breakdown, `mangle_rate: 0`, no panic/NaN. (same file)
- Task 1.5.5: Run `go test ./server/services -run TestGetEscapeAnalyticsGlobalSummary`; update Story 1.4's registry entry `tested: true` + `testIds`, re-run `make registry-generate`. (1 file: the registry JSON)
- Task 1.5.6 (pre-mortem P1 #3): `TestGetEscapeAnalyticsGlobalSummary_should_ReturnExactMangledCount_When_FixtureHasMixedTrueFalseMangled` — a fixture with a known, non-trivial mix of `mangled: true`/`false` rows (not all-same-value), asserting the exact numeric `mangled_count`/`total_mangled` returned by `ent.Sum(escapeevent.FieldMangled)` through the `Scan(ctx, &dest)` call. This exists specifically to catch a driver/destination-type mismatch (SQLite's `SUM()` over an INTEGER-stored bool may return a type that doesn't match the destination struct field) that a same-value or all-zero fixture would not surface. (same file)

### Epic 2 — Frontend

#### Story 2.1: Backport cancellation guard into `useEscapeAnalyticsSummary`
*Given* `useEscapeAnalyticsSummary`'s fetch effect currently has no cleanup, *when* a `cancelled` flag (mirroring `useEscapeEvents`) is added to its effect, *then* a fetch that resolves after the effect re-runs (session or `enabled` change) does not call `setState`.

- Task 2.1.1: Add `enabled: boolean = true` param; guard the fetch effect and `fetchSummary`/`refresh` callback on it. (1 file: `useEscapeAnalytics.ts`)
- Task 2.1.2: Add the `cancelled` flag + cleanup function to the fetch effect (same file, same pattern as `useEscapeEvents` lines 119, 133, 139, 149). (same file)

#### Story 2.2: New `useEscapeAnalyticsGlobalSummary` hook
*Given* the generated `GetEscapeAnalyticsGlobalSummaryRequestSchema`/response types (Story 1.2) and the cancellation pattern from 2.1, *when* `useEscapeAnalyticsGlobalSummary(enabled, {startTime?, endTime?})` is added, *then* it exposes `{histogram, totalSequences, totalMangled, mangleRate, perSession, loading, error, refresh}` and only fetches while `enabled`.

- Task 2.2.1: Implement the hook in `useEscapeAnalytics.ts`, structurally mirroring `useEscapeAnalyticsSummary` post-2.1 (client ref, `cancelled` guard, `enabled` guard) but calling `getEscapeAnalyticsGlobalSummary` and additionally exposing `perSession: SessionEscapeSummary[]`. (1 file, same as 2.1)

#### Story 2.3: Tab + breakdown table CSS
*Given* ADR-009/vanilla-extract conventions, *when* new `.css.ts` files are added, *then* no hardcoded colors/`var()` strings appear and `pnpm --dir web-app run lint:css`-equivalent passes.

- Task 2.3.1: Add tab-specific styles to `EscapeAnalyticsPage.css.ts` (existing file) reusing `vars.*` tokens for the tablist/tab/active states (same visual language as `SessionDetailView`'s tabs, but this page has its own `.css.ts` — do not import `SessionDetailView.css.ts` styles directly, replicate the token references). (1 file)
- Task 2.3.2: Add a new `SessionEscapeBreakdownTable.css.ts` for the sortable `<th>` buttons + non-color-only outlier highlight (icon/text + `vars.color.errorBg`/`warningBg` tint), per `ux.md`'s WCAG 1.4.1 requirement. (1 file)

#### Story 2.4: Wire the toggle into `EscapeAnalyticsPage.tsx`
*Given* Stories 2.1–2.3, *when* a `viewMode` tablist is added above the existing session selector, *then* selecting "All Sessions" hides the session dropdown/event table, suspends `useEscapeAnalyticsSummary`/`useEscapeEvents` (`enabled: false`), enables `useEscapeAnalyticsGlobalSummary`, and renders the aggregate histogram + `MangleRateIndicator` + a per-session breakdown table default-sorted by `mangleRate` descending with all columns sortable; selecting "Per-Session" reverses all of the above.

- Task 2.4.1: Add `viewMode` state + `role="tablist"` markup (2 tabs) with `aria-controls`/`aria-labelledby`/roving-focus wiring, above the existing `sessionSelectorRow`. (1 file: `EscapeAnalyticsPage.tsx`)
- Task 2.4.2: Thread `enabled: viewMode === "per_session"` into `useEscapeAnalyticsSummary`/`useEscapeEvents` calls; conditionally render the existing per-session JSX only when `viewMode === "per_session"` (wrapped in a `role="tabpanel"`). (same file)
- Task 2.4.3: Call `useEscapeAnalyticsGlobalSummary(viewMode === "all_sessions")`; render its aggregate histogram (`SequenceHistogram`, reused) + `MangleRateIndicator` (reused) inside a second `role="tabpanel"`, plus the 4 empty/error states from `ux.md` (global-empty, filtered-empty, RPC-failure via existing `errorBanner`/`role="alert"` convention, dominant-outlier as informational text). (same file)
- Task 2.4.4: New `SessionEscapeBreakdownTable.tsx` component — native `<table>`, sortable `<button>`-in-`<th>` + `aria-sort` for all 4 columns, default sort `mangleRate` desc, reuses `MangleRateIndicator` per row instead of a bare percentage. No pagination (Step 3.5). (1 new file)

#### Story 2.5: Frontend feature registry entries
*Given* Story 2.4's new component + page changes, *when* `// +feature: escape-analytics-global-view` markers are added and `make registry-generate` is run, *then* `docs/registry/features/frontend/escape-analytics-global-view.json` (page toggle) and `docs/registry/features/frontend/session-escape-breakdown-table.json` exist.

- Task 2.5.1: Add `// +feature: escape-analytics-global-view` marker near the top of the modified section of `EscapeAnalyticsPage.tsx` and to the top of `SessionEscapeBreakdownTable.tsx`; run `make registry-generate`; commit. (2-3 files)

#### Story 2.6: Jest tests
*Given* Stories 2.1, 2.2, 2.4, *when* the hook/toggle test suites run, *then* the cancellation guard, `enabled`-gating, and tab-switch suspension behaviors are covered.

- Task 2.6.1: `useEscapeAnalyticsSummary_should_IgnoreStaleResponse_When_SessionChangesBeforeFetchResolves` + `useEscapeAnalyticsSummary_should_NotFetch_When_Disabled` (1 file: `useEscapeAnalytics.test.ts`, new or extended)
- Task 2.6.2: `useEscapeAnalyticsGlobalSummary_should_Fetch_When_Enabled` + `..._should_NotFetch_When_Disabled` + `..._should_IgnoreStaleResponse_When_DisabledBeforeFetchResolves` (same file)
- Task 2.6.3: `EscapeAnalyticsPage_should_SuspendPerSessionHooks_When_AllSessionsTabActive` (RTL test asserting the per-session fetch mock is not called / the global fetch mock is called, on tab switch) (1 file: `EscapeAnalyticsPage.test.tsx`, new or extended)

### Epic 3 — E2E

#### Story 3.1: Playwright spec
*Given* Epics 1 and 2 merged, *when* `tests/e2e/escape-analytics-global-view.spec.ts` runs against the isolated test server, *then* it verifies: tab switch renders the global summary; per-session view is hidden while "All Sessions" is active; breakdown table default-sorts by mangle rate descending; clicking a column header re-sorts; time-range filter (if a UI control exists — else RPC-level only) narrows results.

- Task 3.1.1: `// @feature analytics:get-escape-global-summary, escape-analytics-global-view` header + page-object helper additions under `tests/e2e/pages/` for the new tablist + breakdown table (locators via `data-testid`/ARIA roles only, no `waitForTimeout`). (2-3 files: spec + 1 page helper, possibly extending an existing escape-analytics page helper if one exists)

## Task/Story/Epic Counts (for Step 6 summary)

- Epics: 3
- Stories: 12 (1.1–1.5, 2.1–2.6, 3.1)
- Tasks: 29
