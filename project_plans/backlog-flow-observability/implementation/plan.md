# Implementation Plan: backlog-flow-observability

**Feature**: Aggregate backlog flow analytics (stage dwell, funnel, rework by cause, first-pass yield, review-gate and deadlock analysis, stuck time, thrash anomalies, before/after comparison) exposed as a `/backlog/flow` web view and as MCP tools, both served by one aggregator.
**Date**: 2026-11-05
**Status**: Ready for implementation (repaired 2026-11-05 against `architecture-review.md` and `adversarial-review.md`, then patched against `consistency-review.md`, `pre-mortem.md` and the architecture re-review; see the Repair log at the end, including the Phase 4 patch)
**ADRs**: ADR-001 (seam + single aggregator), ADR-002 (query-time thrash collapse by flip shape), ADR-003 (stuck-episode history), ADR-004 (read path, time normalization, index), ADR-005 (deadlock and reviewer-quality definitions). All in `project_plans/backlog-flow-observability/decisions/`.

**Base-branch warning (VERIFIED)**: this worktree is 827 commits behind `origin/main` (`git rev-list --count HEAD..origin/main`). Research file:line references and the `SetMaxOpenConns(1)` finding come from the stale tree. Story 0.1.1 syncs first; every `file:line` below is re-verified there.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `ItemID` | Backlog item identifier (string UUID) | Newtype in `session/flow`; not raw string |
| `Status` | `domain.BacklogStatus` (`session/domain/backlog.go`): an open string type with 9 built-ins (`idea, refining, ready, queued, in_progress, review, pr_pending, done, archived`) plus operator-configured custom stage slugs | `flow` imports `session/domain` directly (ent-free, import-pure); no second status type. Stage order is data (`StageOrder`), see `Funnel` |
| `StageOrder` | Ordered `[]domain.BacklogStatus` derived from the pipeline definition (the `WorkflowEngine` transition graph: `session.NewDefaultWorkflowEngine()` or `ConfiguredWorkflowEngine`), supplied by `ItemSource.Stages(ctx)` | A status absent from `StageOrder` lands in an `other` funnel bucket with a count; no hard-coded stage list and no exhaustive switch over statuses in `flow` |
| `Event` | One normalized `backlog_status_events` row: ItemID, From, To, At (UTC `time.Time`), Note, TriggeredBy, Seq (stable tiebreak: storage rowid, documented as an ordering key only) | Plain struct, no ent types; `Note` is raw text and is never sanitized in `flow` or the reader |
| `FlowWindow` | Half-open UTC interval `[Start, End)` | Constructed only via `NewFlowWindow(clock, days)` (days >= 1; `0` is an error), `NewAllTimeWindow(clock)` (Start = `2000-01-01T00:00:00Z`, a floor earlier than any event) or explicit bounds; validates `Start < End`. **All-time input**: RPC and MCP carry an explicit `all_time` boolean (never a bare `window_days = 0`; `window_days = 0` without `all_time=true` is an `InvalidArgument` naming `all_time`) and map it to `WindowSpec{AllTime: true}`; the UI offers it as a fourth window option (`?window=all`, sent as `all_time=true`). All-time responses carry `slow_path=true` and use the chunked read path (Story 2.1.6) |
| `FlowQuery` | Window plus filters (`ItemIDs`, `Cohort`) plus `Config` overrides | Single parameter object; no same-typed parameter piles. Carries a `WindowSpec{Days, OffsetDays, AllTime}` from which the cache key is built (never the clock-derived timestamps) |
| `ReaderLimits` | `MaxWindowRows` (60,000 events: above it the reader chunks), `HardRowCeiling` (1,000,000: above it the read is refused with typed `ErrWindowTooLarge`), `ChunkItems` (25 items per batch), `BatchTimeout` (5s) | Story 2.1.6; defaults in one place; chunked results equal unchunked results because collapse and dwell are per-item |
| `Clock` | Interface `Now() time.Time` | Injected; tests use a fixed clock; no `time.Now()` in window logic |
| `Config` | Tunables: `ThrashGap` (15m), `ThrashMinFlips` (6 events), `ThrashMergeGap` (1h), `DeadlockMinRun` (3), `DeadlockRepeatCriterion` (3 consecutive verdicts), `LowSampleN` (5), `StalledAfter` (1h). Derived: `CollapsePad = ThrashGap x ThrashMinFlips + ThrashMergeGap` (2h30m by default) | Defaults in one place; boundary-tested; the pad is derived so changing a threshold cannot silently under-pad (see Story 1.2.1) |
| `FlipPair` | Consecutive `A>B` then `B>A` events on one item with gap under `ThrashGap`, A/B in `{ready,in_progress}`, `{queued,in_progress}`, `{pr_pending,in_progress}` | Keyed on shape, never note text (ADR-002) |
| `Span` | Sealed interface with exactly two implementations: `RealSpan` (item, from, to, at) and `ThrashSpan` (item, flip pair, event count, first/last, net status, cause). Fields are unexported; only `NewRealSpan` / `NewThrashSpan` construct them, and `NewThrashSpan` rejects `count < ThrashMinFlips` or `first > last` | Output of `Collapse`. A real span cannot carry a `Cause` or `Count`, and a thrash span cannot omit them. Type switches carry a `default` that records a data-quality error, and `TestSpanImplementationsHandled` enumerates the implementations |
| `ThrashEpisode` | Maximal run of `FlipPair`s with at least `ThrashMinFlips` events (the unit is events, in the glossary, ADR-002 and tests) and every gap under `ThrashGap`; adjacent episodes on one item with the same flip pair and an inter-episode gap under `ThrashMergeGap` merge into one | Surfaced as anomaly; excluded from dwell buckets. The primary anomaly unit is the item (episode count and total events per item); the banner counts items. Live data fragments (see Story 1.2.1 ACs) |
| `Cause` | Closed enum: `DequeueSpawnFailed`, `DequeueClaim`, `PRFixReopen`, `ReviewReopen`, `Unclassified` | Label only; table-driven prefix classifier; `Unclassified` count always reported |
| `DwellBucket` | Per-stage dwell sample set (+ `thrash` bucket) | Percentiles by nearest-rank, with `N` |
| `Percentiles` | `{N, P50, P90, Max}` nearest-rank over a sorted slice | One stated definition; `LowSample` when `N < LowSampleN` |
| `Funnel` | Per stage in `StageOrder`: items entered, items advanced, dwell, with rework back-edge counts; plus an `other` bucket (count) for statuses outside `StageOrder` | Archived excluded from stages, counted in throughput |
| `MergedItem` | Item whose first `pr_pending>done` falls in the window | Cohort for throughput and FPY ("first merge" rule; `done` is not monotonic) |
| `ReworkCause` | Sum type: `ReviewFail`, `PRFix`, `Bounce`, `Other`. The real (post-collapse) `review>in_progress` events are partitioned by the note classifier: `ReviewFail` = note classified `ReviewReopen` (live 398 of 426), `Bounce` = every other real `review>in_progress` (live 28: 18 agent "request_review recovering stale/blocked review" plus 10 manual). The per-item bounce distribution (Story 1.2.2) and FPY count all real `review>in_progress` events, which is `ReviewFail + Bounce`; the funnel back-edge shows that total. `bouncing` as a stuck reason is a separate stuck-state concept, not a rework cause. `PRFix` carries an optional `PRFixTrigger` (`ci`, `conflict`, `mixed`, `review`, `feedback`, `unknown`) | CI-failure vs merge-conflict split is not derivable from events. Until Story 5.2.3 captures it, the whole bucket is `PRFixTrigger=unknown`, reported as one `PRFix` bucket and listed in `estimated[]` as `pr_fix_cause:unavailable` (owner-visible gap, see Unresolved Questions) |
| `FirstPassYield` | Share of `MergedItem`s with zero `review>in_progress`, zero real (post-collapse) `pr_pending>in_progress`, and zero thrash episodes of any cause before the first merge | Always rendered as `k/n`; denominator stated. Thrash-only items (all reopens collapsed) do not pass; the response also reports `thrash_affected = m` so the exclusion is visible |
| `VerdictClass` | Sum type: `Judgment`, `NoCommits`, `SelfCheckFailed`, `ReviewerDegraded` (no diff computed, degraded to UNVERIFIABLE, no acceptance criteria), `AutoVerified` | ADR-005. Live recount: 98 `NoCommits` + 34 `SelfCheckFailed` = 132 of 378 non-PASS; plus 18 `ReviewerDegraded` UNVERIFIABLE = 150 of 378 |
| `VerdictSequence` | Per-item ordered verdict outcomes as a string (`FRUUURURRURRRP`) | Built from the item's verdicts ordered by `created_at` (one verdict per session by schema, so the per-session step is a join-safety guard, not a dedup) |
| `VerdictMixSeries` | Weekly buckets of verdict counts by `VerdictClass` x outcome | Feeds the verdict-mix-over-time chart (Story 4.2.5) |
| `LeadTime` | Per `MergedItem`: first `* > in_progress` event to first `pr_pending>done` | `Percentiles` over merged items in the window; drill key `tiles.lead_time`; feeds the "Median lead time" tile |
| `PRFixDwell` | Per PR-fix cycle: a real (non-collapsed) `pr_pending>in_progress` event to the next `in_progress>pr_pending`, `in_progress>review` or `done` event on that item (as-of if open) | Combined across triggers in Phases 1-4 (`estimated: pr_fix_dwell:combined`); split by `PRFixTrigger` only for cycles whose reopen is after the Story 5.2.3 capture date (`estimated: pr_fix_dwell_split:since_capture`); thrash episodes are counted as episodes, not as dwell cycles |
| `DeadlockCandidate` | Item with a run of at least `DeadlockMinRun` consecutive judgment non-PASS verdicts AND a no-progress signal inside the run: the same failing criterion index in `DeadlockRepeatCriterion` consecutive verdicts, or an unchanged non-empty `diff_hash` between two consecutive verdicts. Cap exhaustion also qualifies | Heuristic; carries `Resolution`, `RunLength`, `RepeatedCriterion`, `DiffUnchangedPairs`; `AgentClaimedDone` is displayed but is not a gate (it is true for 95.7% of non-PASS criterion verdicts, so it does not discriminate; ADR-005). Lists rank by `RunLength` (ties: latest verdict first), default to the top 10, and carry a "weak filter" note in the UI and in MCP responses (`estimated` contains `deadlock_candidates:heuristic`) |
| `Resolution` | Enum: `Open, Passed, AutoVerified, HumanOverride, Archived` | |
| `StuckStat` | Per reason: distinct items, open count, time-in-stuck percentiles | `multiple_reasons` is its own bucket; `LowerBound` flag until episode capture |
| `StuckEpisode` | Append-only `(item_id, reason, opened_at, resolved_at)` row; at most one open episode per `(item_id, reason)`, enforced by a partial unique index | New ent table, Phase 5 (ADR-003). Written inside the same transaction as the `backlog_stuck_states` write |
| `SectionStatus` | The one section-status enum: `OK, Partial, Failed, Skipped, Empty` (wire strings `ok, partial, failed, skipped, empty`, lowercase) per response section. A timeout is `Failed` with the error text; `Empty` means the section ran and found nothing (denominators still present); `Skipped` means not computed for this request | Shared by RPC and MCP; the proto carries it as a string produced by `SectionStatus.String()` in the single mapper, and a totality test covers every value; the UI, UX and ADR-005 use these five strings and no others |
| `FlowMeta` | `{AsOf, WindowStart, WindowEnd, RowCounts, QueryMs, Estimated[], SlowPath, CaptureSince}` | Present on every response and on every section. `Estimated[]` entries are exactly: `deadlock_candidates:heuristic`, `stuck_history:lower_bound`, `pr_fix_cause:unavailable`, `pr_fix_cause:since_capture`, `pr_fix_dwell:combined`, `pr_fix_dwell_split:since_capture`, `pr_fix_outcome:since_capture`, `stuck_episode_drift:<n>`; one spelling everywhere (UX, plan, ADRs, tests) |
| `ListPage` | Shape of every top-N list in a response: `{items, total_count, returned_count, has_more, truncated}` | `total_count` is the size of the full set; `has_more = total_count > returned_count`; `truncated = true` when the server clamped the requested limit (default 10, max 50), so a clamped list can never be mistaken for the whole set |
| `DrillKey` | String-backed type naming the item set behind a displayed number (for example `rework.review_fail`, `stuck.pr_needs_fix`, `thrash.episode`) | Defined together with each metric; `TestEveryMetricHasDrillKey` (a registry completeness test) is what closes the set, since Go cannot |
| `FlowSummary` | Sectioned response: tiles, funnel, causes, review, pr, stuck, anomalies, trends, notes | One proto/Go shape for UI and MCP; one mapper (`server/services/backlog_flow_mapper.go`) |
| `WindowComparison` | Two `FlowSummary` plus per-metric `{A, B, Delta, N_A, N_B}` | No significance claims; counts and ratios only |
| `EventSource` | `Events(ctx, EventRange) ([]Event, error)`, `CountEvents(ctx, EventRange) (int, error)`, `StreamEvents(ctx, EventRange, fn func(batch []Event) error) error` (item-batched, for windows above `MaxWindowRows`) and `LastEventsBefore(ctx, start, ids) ([]Event, error)`; request-review events are ordinary `Event`s filtered on `To=review, TriggeredBy=agent` | Role interface, one fake per test. The caller passes `EventRange` already padded by `CollapsePad` |
| `VerdictSource` | `Verdicts(ctx, window) ([]Verdict, error)` (carries outcome, summary, per-criterion outcomes, `diff_hash`, `created_at`, item id) and `AgentDoneClaims(ctx, window)` | Role interface |
| `StuckSource` | `StuckIntervals(ctx) ([]StuckInterval, error)` | Role interface; Phase 5 episodes plug in behind it |
| `ItemSource` | `Items(ctx, ids) ([]ItemMeta, error)` (raw titles) and `Stages(ctx) ([]domain.BacklogStatus, error)` | Role interface. `session.EntFlowReader` implements all four; `Aggregator` takes `Sources{Events, Verdicts, Stuck, Items}` so a section fails independently of the others |
| `Aggregator` | Pure computation plus bounded TTL cache plus singleflight plus a one-slot limiter on uncached computes; takes `Sources`, a `Clock`, and an optional `Observer` (compute stats callback) | One instance, constructed once in `server/dependencies.go`; reached only through `BacklogFlowService` (the RPC handler) and, in-process, the MCP tools |
| `Anomaly` | Thrash episode or per-item transition-rate outlier | Excluded from averages, listed with counts |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall approach (CREATIVE pass) | Compute-on-read pure aggregator behind role-interface sources (`EventSource`, `VerdictSource`, `StuckSource`, `ItemSource`) | PoEAA Service Layer + Gateway | (A) Materialized rollup tables; (B) export to Grafana/OTel | A: second writer, backfill, collapse at write time is wrong place, SLO met without it (10k rows/30d). B: Claude cannot query it through the app's MCP; item-level drill-down is high-cardinality. Strength of chosen: one code path, two consumers; weakness: recompute cost, mitigated by index, TTL cache, revisit trigger |
| `flow.Aggregator` | Service Layer with pure functions (collapse, dwell, funnel, review, stuck) composed in one assembler | PoEAA | Domain Model with entities | Read-only analytics over events; no invariants to protect; pure functions are table-testable and stay under funlen/gocognit |
| `EventSource` / `VerdictSource` / `StuckSource` / `ItemSource`, implemented by `EntFlowReader` | Gateway / Repository split into role interfaces (ISP) | PoEAA, Hexagonal | One 7-method `Reader`; aggregator importing ent directly | Keeps `session/flow` testable without SQLite; each fake implements one or two methods; matches the section-level error isolation AC |
| Aggregation location | Load window events (about 10k rows for 30 days) into Go and compute there | ADR-004 | `research/build-vs-buy.md` recommendation of SQL-native counts via ent `GroupBy().Aggregate()` plus one small Go collapse | Deliberate departure: collapse, dwell, FPY and thrash-aware funnel need ordered per-item event streams, which SQL group-bys cannot express without window functions (and those route to the writer connection per `dbsplit`). Revisit trigger is the ADR-004 rollup trigger (30-day summary over 500ms at 5x volume) |
| RPC and MCP adapters | Adapter (thin translate-only). MCP tools call `BacklogFlowService` in-process (as `server/mcp/tools_backlog.go` does with `h.backlogSvc.ListBacklogItems`) and marshal the response with `protojson` | GoF | Separate JSON mapper in `server/mcp` plus a parity test with field-name normalization | One mapper, so parity holds by construction. MCP-only work: paging/clamping and `SanitizeForAgentContext` on free-text fields |
| Import boundary of `session/flow` | `depguard` strict allow-list (`isolate_flow_core`) | `.golangci.yml` precedent (`isolate_threatscan_stdlib_only`) | Prose-only rule | A compile-time cycle is the last line of defence, not the first (Story 1.1.3) |
| MCP wiring | Variadic `CoreOption` (`WithFlowService`) on `NewCore`, `NewHTTPHandler`, `RunServer` | functional options | An 11th positional parameter | Existing 10-argument call sites and the 7 test call sites are untouched |
| Note classifier, verdict classifier | Table-driven classifier (Strategy as data) | GoF | if-chains | gocyclo/gocognit gates; unclassified bucket reported |
| `Span` (`RealSpan` / `ThrashSpan`) | Sealed interface with validating constructors | type-driven-design | One struct with a `Kind` field and optional fields | Illegal combinations (a real span with a `Cause`) cannot be built; `exhaustive` only covers enums, so a handled-implementations test covers spans |
| `VerdictClass`, `ReworkCause`, `SectionStatus`, `Resolution` | Enum / sum type with exhaustive switch (`exhaustive` linter, `default-signifies-exhaustive: false`) | type-driven-design | Free strings | Compiler-visible unhandled cases |
| `ItemID`, `FlowWindow`, `Clock`, `FlowQuery` | Newtype / value object / parse-don't-validate | type-driven-design | `string, time.Time, time.Time` parameter piles | Primitive-obsession checklist; invalid windows unrepresentable |
| Cache | Decorator (TTL cache + singleflight around `Aggregator.compute`) | GoF | In-handler caches | One cache, shared by both transports |
| Bounded compute | Row ceiling with chunk-or-refuse, plus a one-slot limiter on uncached computes | Pre-mortem failure #2 | Unbounded all-time load on the single-connection writer | An all-time read of about 258k rows must not stall the live factory; chunking by item is exact because collapse is per-item (Story 2.1.6) |
| Percentiles | Exact nearest-rank on sorted slice | build-vs-buy research | t-digest / approximations | Exact is cheap at this N; method stated in response |
| Drill-down | Closed `DrillKey` registry, one RPC `ListFlowDrilldown` | PoEAA Registry | Per-metric endpoints | Every number gets a drill by construction |
| UI funnel and tiles | Plain HTML/CSS (vanilla-extract) with hidden data table | ux research | Sankey / recharts funnel | Cycles render badly in Sankey; best a11y; no new dep |
| UI trends | One `FlowChart` wrapper over recharts | ux research | Per-chart recharts usage | Wrapper enforces `figure`/`aria-label`/hidden table/pattern rules once |
| UI tab nav | Extract `BacklogViewTabs` component | refactoring | Copy nav a third time | jscpd gate; shared by List, Board, Flow |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `server/services/backlog_service_triage.go` (4018 lines, 29 commits/90d) | God-file; hosts the dequeue and PR-fix loops | Isolate via seam | New code lives in `session/flow` and `server/services/backlog_flow_service.go`; this file is not edited by this feature. Backoff fix is a separate follow-up item (Story 6.1.2) |
| `server/mcp/tools_backlog.go` (3255 lines) | God-file of per-item tools | Isolate via seam | New tools in `server/mcp/tools_backlog_flow.go`; `server/mcp/server.go` (233 lines) gains the `CoreOption` type, a `flowHandlers` registration inside the `backlogEnabled` block, and `opts ...CoreOption` pass-through on `NewCore`, `NewHTTPHandler`, `RunServer` |
| `proto/session/v1/backlog.proto` (1806 lines) | Oversized proto | Isolate via seam | Own `backlog_flow.proto` and `BacklogFlowService` |
| `BacklogService` struct | God object (storage, spawners, runners, mutexes) | Isolate via seam | Untouched; `BacklogFlowService` holds only the `*flow.Aggregator` |
| `session/ent_repository_backlog.go` (3,299 lines, 39 commits/90d on `origin/main`; holds `MarkStuck` at :2332 and `ResolveStuck` at :2403) | God-file; Story 5.1.2's hook points | Extend as-is with minimal seams | `MarkStuck` / `ResolveStuck` keep their exported signatures. Each gains one in-tx call to a helper in new `session/ent_repository_stuck_episode.go`; `ResolveStuck` also moves its single UPDATE into a tx (its only structural change). All episode logic lives in the new file |
| `session/ent_repository.go` (writer pool `SetMaxOpenConns(1)` at :203) | Constructs the repository and the single-connection writer pool | Not edited (reference only) | `EntFlowReader` takes the existing ent client and relies on the reader-pool routing asserted in Story 2.1.2; Story 0.1.1a re-opens the file only to confirm :203 and the routing seam. No line in it changes |
| `session/storage.go` (1,876 lines, 57 commits/90d) | `Storage.MarkStuck` / `ResolveStuck` pass-throughs at :1378/:1384 | Extend as-is (not edited) | The hook sits one layer down at the repository, so the passthroughs and the `Repository` interface are unchanged. `NewEntFlowReader(storage.GetEntClient())` (`storage.go:312`) uses the existing accessor; no new plumbing |
| `server/dependencies.go` (2,229 lines, 66 commits/90d) | Hotspot; Stories 3.1.3 and 3.2.3 edit it | Extend as-is with one construction block | One block builds the single `*flow.Aggregator` and `BacklogFlowService` and stores them on the deps struct. No other lines change |
| `server/server.go` (2,271 lines, 89 commits/90d) | Hotspot; Stories 3.1.3 and 3.2.3 edit it | Extend as-is with two one-line seams | One handler registration line next to the Insights registration (`server.go:600`) and one `servermcp.WithFlowService(...)` argument at the `NewHTTPHandler` call (`server.go:1178`) |
| `main.go` stdio fallback (`mcpserver.RunServer(...)`, :172) | Phase-1 deps only | Extend as-is with one option argument | `mcpserver.WithFlowService(services.NewFlowServiceFromStorage(storage))`; the fallback builds its own aggregator from `storage` (decision, tested in Story 3.2.3) |
| `session/backlog_lifecycle_pr.go` (PR-fix reopen site, ~:1965-1975) | High-churn hot file; Story 5.2.3 reads the trigger flags it already has | Extend as-is with one helper call | Story 5.2.3 adds one marker call where `CIFailing` / `HasConflicts` / `HasBlockingReviews` are known; no control-flow change |
| `backlog_stuck_states` unique `(item_id, reason)` | Lossy history by design (ADR-001 of stuck-visibility) | Extend as-is plus additive episode log | Do not change resolve-in-place; add append-only `BacklogStuckEpisode` (ADR-003) written in the same tx as the stuck-state write, with a partial unique index so a duplicate open episode is a constraint no-op |
| List page inline tab nav (`web-app/src/app/backlog/page.tsx`) | Nav duplicated between List and Board | Refactor-first (small) | Extract `BacklogViewTabs` in Story 4.1.1 before adding the third tab |
| `session/ent/schema/backlog_status_event.go` `time.Now` (Local) default | Mixed-offset timestamps | Extend as-is | No migration of 258k rows; normalize in Go (ADR-004). Only rows before 2026-08-17 12:00 local carry `-07:00` (verified: 10,654 rows, 2026-08-06 to 2026-08-17; all later rows `+00:00`). New schema uses `time.Now().UTC()` |

---

## Migration Plan

- **Migration files**: ent schema only, no hand SQL. (1) `session/ent/schema/backlog_status_event.go`: add `index.Fields("created_at")` (Story 2.1.1), only if Story 0.1.2 does not conclude "skip the index" (the 30-day window returns about 4% of rows and the existing `(item_id, created_at)` index may suffice). (2) `session/ent/schema/backlog_stuck_episode.go`: new additive table with a partial unique index on open episodes (Story 5.1.1). Auto-applied by `Schema.Create` at service start. Generated ent code is NOT committed; regenerate with `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`.
- **Reversibility**: both are additive. Dropping the index or table is a revert PR; no data depends on them until Phase 5 writes rows, and readers tolerate an empty or absent episode table.
- **Zero-downtime strategy**: the index builds on about 258k rows at the next service restart; Story 2.1.5 measures build time on a copy of the live DB first. No backfill, no dual-write. Do not restart the live `:8543` service to test; use a manual instance (ports 62871/62872, `STAPLER_SQUAD_INSTANCE=claude-manual-test`) with a `sqlite3 .backup` copy of the workspace DB.
- **Rollback procedure**: revert the PR. The leftover index/table is harmless, and the episode writer is a single best-effort helper call.

## Observability Plan

- **Logs**: debug-level line per aggregation: `flow aggregate window=<..> section=<..> rows=<n> query_ms=<n> cache=<hit|miss>`; warning on section failure with error and window; counters of data-quality anomalies (self-loops, negative dwell clamped, unclassified notes, unrecognized verdict summaries) logged once per compute.
- **Metrics**: `flow.aggregate.duration_ms` (summary, per section) and `flow.cache.hit` via the existing OTel `Meter()` (`telemetry/telemetry.go`), no item-level labels. Operation over 100ms: events read, seed read, verdict join, collapse. Alerts: none new (rollup revisit trigger: 30-day summary over 500ms at 5x volume, read from the debug log).
- **Self-surfacing**: `FlowMeta` (as-of, row counts, query ms, `estimated[]`) is returned on every response and rendered in the "Data notes" section and the MCP JSON.

## Risk Control

- **Feature flag**: not gated separately. The Flow route inherits the existing `backlog` feature flag via `web-app/src/app/backlog/layout.tsx`; MCP tools register inside the existing `backlogEnabled` block.
- **Rollback procedure**: standard revert via PR close and revert commit.
- **Staged rollout**: three PRs, each with its own verification point before it is marked ready (see "Delivery plan: three PRs"). PR-A (capture writers) is verified on a manual instance against a `.backup` copy of the live DB (DDL, row appearance, forgery and rate-limit tests) and merges first so the soak clock starts. PR-B (Phases 0-4, the read layers) is verified by Story 6.1.1 on a manual instance before it is marked ready. PR-C (capture readers) re-runs the reader-related steps of Story 6.1.1 after PR-A has accumulated rows. `make ready` (Story 6.1.3) runs on every PR. No PR is verified after it merges.

## Unresolved Questions

- [ ] Does ent `client.Query...` from `EntFlowReader` actually route to the dbpool reader pool on `origin/main`? — **hard precondition of Story 2.1.2**: the story may not merge unless a test asserts the flow reads carry the `reader` pool label (a writer-routed read is a merge blocker, not a note) — owner: spike Task 0.1.2a
- [ ] Does `EXPLAIN QUERY PLAN` use a new `created_at` index with the padded 15h-superset prefilter, or is the index unnecessary for the ~4% 30-day slice (existing `(item_id, created_at)` index)? — decides whether Story 2.1.1 exists; blocks Story 2.1.2 only for the prefilter shape — owner: Task 0.1.2a
- [ ] After sync, do `session-multi-pr-tracking` changes (item PR sets) alter the `pr_pending` reopen paths or notes that the classifier table assumes? — blocks Story 1.1.2 — owner: Task 0.1.1a
- [ ] Which store can hold PR-fix markers **with a system-written kind/author discriminator** (a dedicated column or table that `post_backlog_update` cannot write)? A text prefix inside `backlog_activity_notes` is forgeable by any agent through `post_backlog_update`, so it is not acceptable as the marker. — blocks Stories 5.2.2 and 5.2.3 — owner: Task 5.2.1a
- [ ] Retention of `backlog_status_events` — unknown; the plan does not depend on it (ADR-004 item 7) — owner: follow-up, non-blocking
- [x] Phase 5.2 (PR-fix outcome capture touches `backlog_service_pr_fix_steer.go`, a hot file) is in scope behind one-line helpers; Story 5.2.3 is in scope and its writer half ships in the early PR-A, ahead of the Phase 1-4 read layers, so the soak clock (`FlowMeta.CaptureSince`) starts as early as possible (owner direction recorded in the Phase 4 patch; ordering corrected in the Final triad patch because the first-wave claim was hollow while Phase 5 waited on the whole Phases 1-4 PR)
- [ ] **Recorded gap, requirement not met in Phases 1-4 (forward-only fix in scope)**: the requirements list "PR CI failure" and "merge conflict" as separate rework causes, and ask for CI-failure and merge-conflict dwell separately, but status events do not record which fired (the reopen note is the constant `auto-reopened for PR fix (CI/review)`, `server/services/backlog_service_triage.go:2525`). Phases 1-4 report one combined `PRFix` bucket (`estimated: pr_fix_cause:unavailable`) and one combined PR-fix dwell (Story 1.2.3, `estimated: pr_fix_dwell:combined`). Story 5.2.3 closes both gaps going forward by recording the trigger at the PR poller's decision site (`session/backlog_lifecycle_pr.go`, where `CIFailing` / `HasConflicts` / `HasBlockingReviews` are already computed) as a separate trigger record, **not** by changing the constant reopen note in `backlog_service_triage.go`; readers join each reopen status event to the nearest preceding trigger record for the same item (see Story 5.2.3); the split exists only for cycles after the capture date and no backfill is possible. Compare windows that straddle the capture date return `incomparable` for the split metrics (Story 1.4.2)
- [ ] Which `WorkflowEngine` accessor yields stage order for `ItemSource.Stages` (`DefaultWorkflowEngine` keeps its graph in an unexported `transitions` map; `ConfiguredWorkflowEngine` reads `StageConfigRepository`)? — blocks Story 2.1.4 — owner: Task 0.1.1a records the accessor, or 2.1.4a adds a small exported `PipelineStages(engine)` in `session/`

## Dependency Visualization

```
0.1.1 sync main ──┬─> 0.1.2 spike (read path, index) ─────────────┐
                  ├─> 0.1.3 golden fixtures ──┐                    │
                  └─> 1.1.1 types ─┬> 1.1.2 classifiers/percentiles │
                                   └> 1.1.3 depguard rule           │
                                   │                               │
        ┌──────────────────────────┼───────────────┐               │
        v                          v               v               │
 1.2.1 normalize+collapse   1.3.1 verdict class   1.4.1 stuck stats │
        v                          v               │               │
 1.2.2 dwell/funnel/rework  1.3.2 deadlock         │               │
        └──────────┬───────────────┴───────────────┘               │
                   v                                               │
        1.2.3 PR-fix dwell + lead time (in 1.2.2/1.2.3) ──> 1.4.2        │
 1.4.2 assemble + compare + drill ─> 1.4.3 cache            │
                   │                                               │
 1.4.2a result types frozen ─> 3.1.1 proto ─┐   2.1.1 index ─> 2.1.2..2.1.4 reader <┘
                       v                  2.1.6 row ceiling + chunking ─> v
                  3.1.2 service             2.1.5 perf check (gate, not a code
                       v                    dependency of 3.1.2; runs in parallel)
              3.1.3 wiring+registry ─> 3.2.1/3.2.2 MCP ─> 3.2.3 CoreOption wiring
                       v
 4.1.1 tabs ─> 4.1.2 hook+header ─> 4.1.3 FlowChart ─> 4.2.1..4.2.5 sections (+4.2.6 item timeline) ─> 4.3.x tests
                                                                 │
 PR-A (starts after 0.1.1 only; no Phase 1-4 code dependency):
 0.1.1 ─> 5.2.1 spike (store + discriminator) ─┬> 5.2.3a trigger writer (dedupe) ─┐
 0.1.1 ─> 5.1.1 schema ─> 5.1.2a/b episode writer ─────────────────────────────────┼> PR-A merge ─> soak clock starts
                                                5.2.2a outcome writer ─────────────┘
 PR-B (Phases 0-4) rebases onto PR-A;  1.4.2a result-type freeze ─> 3.1.1 proto ─> 3.1.2 ...
 PR-C: PR-B merged + PR-A rows ─> 5.1.3 / 5.2.2b / 5.2.3b readers
                                      PR-B: 6.1.1 manual verify ─> 6.1.3 gates ─> merge;  PR-C: partial 6.1.1 re-run ─> gates ─> merge
                                      6.1.2 follow-up backlog items x3 (6.1.2c gated by 6.1.2a landing; owner Tyler's backlog)
```

## Delivery plan: three PRs

Capture work has no code dependency on Phases 1-4 (the writers touch `session/ent_repository_backlog.go`, `server/services/backlog_service_pr_fix_steer.go`, `session/backlog_lifecycle_pr.go` and new `session/ent/schema` files; the readers do). It therefore ships first, so real stuck-episode and PR-fix data accrues while the read layers are still being built; the soak clock (`FlowMeta.CaptureSince`) starts at PR-A's deploy, not after about 9M CU of read-layer work.

| PR | Contents | Verification point (before marked ready) | Why this order |
|---|---|---|---|
| **PR-A: capture writers** (early, separate) | 0.1.1 sync, 5.2.1 spike, 5.1.1, 5.1.2, 5.2.2a (writer), 5.2.3a (writer) | Manual instance on a `.backup` copy: partial-index DDL on the live-shaped DB (Story 5.1.1 AC), `go test -race` of the stuck tests plus the new writer tests, the forgery and rate-limit ACs (Stories 5.2.1-5.2.3), sample rows visible via `sqlite3 -readonly`. `make ready`. | Starts the soak clock; each writer is a one-line helper call or a tx-scoped helper, revertable without touching read layers. |
| **PR-B: Phases 0-4** (read layers: `session/flow`, reader, transports, UI) | 0.1.2, 0.1.3, 1.x, 2.x, 3.x, 4.x, 6.1.1, 6.1.2, 6.1.3 | Story 6.1.1 on a manual instance (live-size copy), precision and comprehension checks, `make ready`. | Needs the spike results and fixtures; independent of PR-A except that it rebases onto it. |
| **PR-C: capture readers** | 5.1.3, 5.2.2b, 5.2.3b (readers, sections, dwell split) | Re-run the stuck-drift, PR-health and dwell-split steps of Story 6.1.1 on a manual instance with the rows PR-A has accumulated; `make ready`. | Readers need the aggregator and sections from PR-B. |

Per-PR verification replaces the earlier single "verify before the PR is ready" gate, which contradicted the plan to merge Phases 1-4 before Phase 5 started.

---

## Effort estimate

Bands and weights INFERRED (`ESTIMATION.md`); recalibrate after the first implementer run. Cost class tokens: XS 20k, S 55k, M 130k. Assumed output share 12% for implementation runs, so CU = raw tokens x 1.48 (round 1.5), then x 1.75 verification multiplier (per-story spec/quality review, `6-verify` layers, CI reruns). Story CU below = raw x 1.5 x 1.75 = raw x 2.625.

| Story | Tasks | Cost class | Raw tokens (k) | Output share | CU (k, incl. verif.) | Wall-clock blocker |
|---|---|---|---|---|---|---|
| 0.1.1 Sync with origin/main | 1 | S | 55 | 8% | 144 | none |
| 0.1.2 Read-path and index spike, plus conditional reader-pool fallback (Task 0.1.2b) | 2 | S+S | 110 | 8% | 289 | none |
| 0.1.3 Golden fixtures | 1 | S | 55 | 15% | 144 | none |
| 1.1.1 Domain types and spans | 1 | S | 55 | 15% | 144 | none |
| 1.1.2 Classifiers and percentiles | 1 | S | 55 | 15% | 144 | none |
| 1.1.3 depguard import boundary (new) | 1 | XS | 20 | 10% | 53 | none |
| 1.2.1 Normalize and collapse (padding, merge) | 2 | S+M | 185 | 12% | 486 | none |
| 1.2.2 Dwell, funnel, rework, lead time | 2 | M+M | 260 | 12% | 683 | none |
| 1.2.3 PR-fix dwell, combined (new) | 1 | S | 55 | 12% | 144 | none |
| 1.3.1 Verdict classification and mix series (was S) | 2 | S+S | 110 | 12% | 289 | none |
| 1.3.2 Deadlock and reviewer quality | 1 | M | 130 | 12% | 341 | none |
| 1.4.1 Stuck metrics | 1 | S | 55 | 12% | 144 | none |
| 1.4.2 Assembly, trends, compare, drill | 2 | M+M | 260 | 12% | 683 | none |
| 1.4.3 Cache (bounded) | 1 | S | 55 | 12% | 144 | none |
| 2.1.1 created_at index (conditional on 0.1.2) | 1 | XS | 20 | 10% | 53 | none |
| 2.1.2 Event reader (was M) | 1 | S | 55 | 12% | 144 | none |
| 2.1.3 Verdict reader | 1 | M | 130 | 12% | 341 | none |
| 2.1.4 Stuck, items, stage order | 1 | S | 55 | 12% | 144 | none |
| 2.1.5 Perf verification and concurrent-writer check (was XS) | 1 | S | 55 | 8% | 144 | none |
| 2.1.6 Row ceiling, chunked read, single in-flight compute (new) | 2 | S+M | 185 | 12% | 486 | none |
| 3.1.1 Proto | 1 | M | 130 | 15% | 341 | none |
| 3.1.2 Service adapter | 1 | M | 130 | 12% | 341 | none |
| 3.1.3 Wiring and registry | 1 | S | 55 | 10% | 144 | CI queue (registry gate) |
| 3.2.1 MCP aggregate tools (S, back to M in Triad repair 1) | 1 | M | 130 | 12% | 341 | none |
| 3.2.2 MCP list tools (was M) | 1 | S | 55 | 12% | 144 | none |
| 3.2.3 CoreOption wiring (HTTP and stdio) | 1 | S | 55 | 12% | 144 | none |
| 4.1.1 Tabs and route | 1 | S | 55 | 12% | 144 | none |
| 4.1.2 Data hook, header controls and states (was S) | 1 | M | 130 | 12% | 341 | none |
| 4.1.3 FlowChart wrapper | 1 | S | 55 | 12% | 144 | none |
| 4.2.1 Tiles and funnel | 1 | M | 130 | 15% | 341 | none |
| 4.2.2 Causes and review | 1 | M | 130 | 15% | 341 | none |
| 4.2.3 PR, stuck, anomalies, notes (split in two) | 2 | M+M | 260 | 15% | 683 | none |
| 4.2.4 Drilldown drawer | 1 | M | 130 | 15% | 341 | none |
| 4.2.6 Item timeline view, "View events" (new) | 1 | S | 55 | 15% | 144 | none |
| 4.2.5 Trends charts (new) | 1 | M | 130 | 15% | 341 | none |
| 4.3.1 Jest tests | 1 | M | 130 | 15% | 341 | none |
| 4.3.2 E2E, a11y checks and Axe (split in two) | 2 | M+S | 185 | 12% | 486 | CI queue (Playwright, Axe, Lighthouse) |
| 4.3.3 Frontend registry | 1 | XS | 20 | 10% | 53 | none |
| 5.1.1 Stuck-episode schema and partial index | 1 | S | 55 | 12% | 144 | none |
| 5.1.2 Episode writer (tx, concurrency; split S+M) | 2 | S+M | 185 | 12% | 486 | none |
| 5.1.3 Episode reader | 1 | S | 55 | 12% | 144 | soak: exact data accrues only after deploy |
| 5.2.1 PR-fix marker spike | 1 | S | 55 | 8% | 144 | none |
| 5.2.2 PR-fix outcome capture (writer in PR-A, reader in PR-C; was one M) | 2 | S+S | 110 | 12% | 289 | soak window |
| 5.2.3 PR-fix trigger kind (writer with dedupe in PR-A, M; reader and dwell split in PR-C, S) | 2 | M+S | 185 | 12% | 486 | soak window |
| 6.1.1 Manual-instance verification and precision check | 1 | S | 55 | 8% | 144 | owner reads the first data and hand-labels about 10 candidates |
| 6.1.2 Follow-up backlog items (three) | 3 | XS+XS+XS | 60 | 10% | 158 | none |
| 6.1.3 Gates (`make ready`) | 1 | S | 55 | 5% | 144 | CI queue |

- **History**: the Phase 4 patch total was 11,609k CU (pre-patch 10,494k, +10.6%); Triad repair 1 recounted it to 12,003k; the Final triad patch recount is the latest bullet below. Size band: **Large (3-15M)**; the request implied Medium (0.5-3M); `requirements.md` states Large with the current figure. Noted and proceeding, not a gate; the scope was kept, not downscaled.
- **Triad repair 1 recount (awk over the table above, `awk -F'|'` summing the Tasks, Raw and CU columns and splitting the cost-class column on `+`)**: **57 tasks across 47 stories (22 M, 29 S, 6 XS)**, 4,575k raw tokens, **12,003k CU (about 12.0M)**; the Phase 4 patch total was 55 tasks, 4,425k raw, 11,609k CU, so this repair adds 2 tasks, +150k raw, +394k CU (+3.4%). Size band unchanged: Large (3-15M). What moved: Story 3.2.1 S to M (+197k: absolute-date compare, drill-key schema enum, description contract, `sections`/`compact` and the size-budget test); Story 4.3.2 split into Task 4.3.2a (M, click counts and non-happy flows) and new Task 4.3.2b (S, criteria 21/26/27/30/33 and Axe) (+145k); Story 6.1.2 gains Task 6.1.2c (XS, post-ship checkpoint item, +52k). Absorbed without a size change, and named as the likeliest underestimates: min-sample and absolute anchors and section selection in Story 1.4.2 (M+M), the extra proto fields in Story 3.1.1, and the stale, collapsed-section and invalid-URL ACs in Story 4.1.2 (M). The new UX ACs in Stories 4.1.3, 4.2.1 to 4.2.4 and 6.1.1 are absorbed in their existing tasks. No wave changes: Task 4.3.2b runs in W6 with 4.3.2a, Task 6.1.2c runs in W9 after 6.1.2a.
- **What moved in the Phase 4 patch**: added Story 1.2.3 PR-fix dwell (+144k), Story 2.1.6 row ceiling, chunked read and single in-flight compute (+486k), Story 4.2.6 item timeline for "View events" (+144k), a second follow-up backlog item in 6.1.2 (+53k); re-sized 4.1.2 S to M for the header-control ACs (+197k) and 2.1.5 XS to S for the concurrent-writer check (+91k). Absorbed without a size change (stated, not hidden): lead time in 1.2.2b, ReviewFail/Bounce partition in 1.2.2, weak-filter note and top-10 default in 1.3.2/3.2.2/4.2.2, `incomparable` compare rule in 1.4.2, single in-flight limiter AC in 1.4.3, `total_count`/`truncated` in 3.2.1/3.2.2, parity fixtures in 3.2.3, reconciliation AC in 5.1.3, dwell split AC in 5.2.3, precision check in 6.1.1. Earlier repair movements (0.1.2, 2.1.2, 3.2.1, 3.2.2 each M to S; 1.1.3, 1.3.1, 4.2.5, 5.2.3 added; 4.2.3 and 5.1.2 split) are unchanged.
- **Final triad patch recount (awk over the table above, same method as Triad repair 1)**: **60 tasks across 47 stories (22 M, 32 S, 6 XS)**, 4,740k raw, 12,438k CU (about 12.4M); Triad repair 1 was 57 tasks, 4,575k raw, 12,003k CU, so this patch adds 3 tasks, +165k raw, +435k CU (+3.6%). What moved: Task 0.1.2b (new, S, conditional reader-pool fallback, +145k); Task 5.2.3a re-sized S to M and its reader and dwell-split half split out as Task 5.2.3b (S): M+S, +342k; Story 5.2.2 split from one M into writer 5.2.2a (S) and reader 5.2.2b (S), -52k (the original M covered both halves; a larger split would be a guess). Absorbed without a size change and named as the likeliest underestimates: the generic sanitizer plus reflection walk, the measured-size-budget fixture test and the `drill_key` and half-open wording tests in Task 3.2.1a (M); the `ErrWindowTooLarge` and compare-disabled states in Task 4.1.2a (M); the result-type freeze in Task 1.4.2a (M); and the kind/author discriminator plus dedupe and thrash-loop tests in Tasks 5.2.1a, 5.2.2a and 5.2.3a.
- **Critical path, stated honestly** (serial agent runs, not waves; counted from the chains below, parallel branches excluded): PR-B longest chain is `0.1.1a` -> `1.1.1a` -> `1.2.1a` -> `1.2.1b` -> `1.2.2a` -> `1.2.2b` -> `1.2.3a` -> `1.4.2a` (result types frozen in its first commit) -> `3.1.1a` -> `3.1.2a` -> `3.1.3a` -> `3.2.1a` -> `3.2.3a` -> `4.1.2a` -> `4.2.x` -> `4.3.1a` -> `4.3.2a` -> `6.1.1a` -> `6.1.3a`, which is **19 sequential runs**; PR-C adds `5.x reader` -> partial `6.1.1` re-run -> gates, **3 more**, so about **22 sequential runs end to end**, plus review and CI latency per PR. Parallel to this: PR-A (`0.1.1a` -> `5.1.1a` -> `5.1.2a` -> `5.1.2b`, and `5.2.1a` -> `5.2.3a`, at most 4 serial runs) runs alongside the first part of PR-B and merges first; `0.1.2a`/`0.1.2b`, `0.1.3a`, `1.1.2a`, `1.1.3a`, `1.3.x`, `1.4.1a`, `1.4.3a`, `1.4.2b`, `2.1.x` and `3.2.2a` hang off that chain as parallel branches. "About 10 waves" in earlier drafts counted waves of parallel work and hid the serial chains inside them (for example `1.2.1a` -> `1.2.1b` -> `1.2.2a` -> `1.2.2b` -> `1.2.3a` is five runs inside one wave). Story `2.1.5` (perf) is a gate, not a code dependency of `3.1.2`. Serialization points: `server/mcp/server.go`, `server/server.go`, `server/dependencies.go` and `main.go` edits (Stories 3.1.3, 3.2.3), the proto file (3.1.1), and `session/ent_repository_backlog.go` (5.1.2, in PR-A) must not run in parallel with other edits to the same file; the seam-file edits in `3.1.3`/`3.2.3` ship as one short-lived PR stacked under PR-B (`make ready` is its only gate).
- **Overrun checkpoint**: if spend exceeds the table value by more than 25% for completed stories (for example more than 1.0M CU over after the first 4M CU), pause and report the delta and its cause. Informational only.
- **Wall-clock blockers** (owner): (1) CI queue runs for registry/complexity/Axe/e2e gates (CI); (2) owner reads the first real data on the manual instance and decides which tuning hypotheses to act on (Tyler); (3) soak window of real item flow after deploy so stuck-episode and PR-fix-outcome data accumulates before before/after comparisons are exact (Tyler); (4) PR review latency (Tyler).

---

## Phase 0: Baseline and spikes

### Epic 0.1: Establish a trustworthy base
**Goal**: Remove stale-tree uncertainty and answer the two spike questions before any reader code.

#### Story 0.1.1: Sync with origin/main and re-verify references
**As a** maintainer, **I want** the branch merged onto current `origin/main` and every cited file:line re-checked, **so that** the plan's integration points are real.
**Acceptance Criteria**:
- Branch contains `origin/main` and builds.
  - *Given* branch `stapler-squad-bklg` 827 commits behind, *When* `git merge origin/main` then `make build`, *Then* build exits 0 and `git rev-list --count HEAD..origin/main` prints `0`.
- `get_backlog_stuck_report` location and name collisions are confirmed.
  - *Given* the merged tree, *When* grepping `registerStuckReportTools` and `NewTool("` names in `server/mcp/`, *Then* `server/mcp/tools_stuck_report.go` is found and no existing tool name contains `flow` other than `*_workflow`.
- Classifier assumptions hold after the PR-set changes.
  - *Given* `server/services/backlog_service_triage.go` post-merge, *When* grepping the literals `"dequeued: WIP slot freed"`, `"dequeue spawn failed"`, `"auto-reopened for PR fix (CI/review)"`, `"auto-reopened after failed review verdict"`, `"all work sessions ended"`, *Then* each literal is found, or the corrected literal is recorded in the Domain Glossary `Cause` row.
- Stuck-episode hook points and pipeline accessor are located.
  - *Given* the merged tree, *When* reading `session/ent_repository_backlog.go` and `session/workflow_engine.go`, *Then* the result records `MarkStuck` (`:2332` on `origin/main`), `ResolveStuck` (`:2403`), whether ent supports `entsql.IndexWhere` plus `sql.ConflictWhere` for a partial unique index, and which accessor returns pipeline stage order.
- Base is pinned and re-checked per wave.
  - *Given* the merged tree, *When* Story 0.1.1 finishes, *Then* the `origin/main` SHA it merged is recorded in this plan's Repair log; and at the start of each wave that edits a serialization-point file (`server/mcp/server.go`, `server/server.go`, `server/dependencies.go`, `main.go`, `proto/session/v1/`, `session/ent_repository_backlog.go`, `session/backlog_lifecycle_pr.go`) the implementer runs `git rev-list --count HEAD..origin/main -- <those files>` and rebases first when it is non-zero.
**Files**: none new; record results as a comment block at the top of `session/flow/classify_notes.go` (created in 1.1.2).

##### Task 0.1.1a: Merge main, rebuild, grep-verify (S)
- Use `git merge origin/main` (not rebase; the branch is shared). Resolve conflicts only if any; run `make build`.
- Re-open `session/ent_repository.go`, `session/dbpool/open.go`, `server/mcp/server.go`, `server/server.go`, `server/dependencies.go`; confirm `NewCore` signature (10 parameters on main), `NewHTTPHandler` (10 parameters, `server/mcp/server.go:120`), `RunServer` (11 parameters including `ctx`, `:156`; stdio caller `main.go:172`), and that `registerBacklogTools` still sits inside the `backlogEnabled` block.
- Files: none edited.

#### Story 0.1.2: Read-path and index spike
**As a** developer, **I want** measured answers on reader-pool routing and index use, **so that** ADR-004 rests on evidence.
**Acceptance Criteria**:
- Flow reads leave the writer connection free.
  - *Given* a `sqlite3 .backup` copy of `~/.stapler-squad/workspaces/6eb0b580fa0331d5/sessions.db` opened through `NewEntRepository`, *When* a 30-day padded event projection (about 10k rows) runs while a writer loop inserts 50 rows/s, *Then* writer p99 latency changes by under 20% versus no reader, and the pool label for the read is `reader` per `session/dbobs`.
- The index is used, or is shown to be unnecessary, and the prefilter is a safe superset.
  - *Given* the copy, *When* running `EXPLAIN QUERY PLAN` for the padded window query with and without a `created_at` index, *Then* the result states which plan SQLite chose and the wall time of each; the spike may conclude "skip the index" (Story 2.1.1 is then dropped) if the unindexed 30-day read is under 100ms. The prefilter (window start minus `CollapsePad` minus 15h) must return every row a Go-parsed `At >= start - CollapsePad` filter keeps, including `-07:00` rows.
- Result recorded.
  - *Given* the above, *When* the spike finishes, *Then* ADR-004 gains a `Spike result` section with numbers (rows, ms, index build time if built) and the decision on the index and on fallback.
**Files**: `project_plans/backlog-flow-observability/decisions/ADR-004-read-path-time-normalization-and-index.md` (append result); throwaway script outside the repo.

##### Task 0.1.2a: Measure and record (S)
- Copy DB with `sqlite3 .backup`; run EXPLAIN and the concurrent-writer probe with and without the index. Sized S, not M: the 30-day window holds about 10,100 events (`created_at >= '2026-10-06'`), so this is a short measurement, not a design exercise.
- Files: ADR-004 only.

##### Task 0.1.2b: Reader-pool fallback (S, conditional on 0.1.2a finding writer-routed reads)
- Concrete fallback, built only if Task 0.1.2a shows ent queries from `EntFlowReader` carry the `writer` pool label: `session.NewEntFlowReader` takes its own read-only ent client opened through the `session/dbpool` reader constructor (a separate `*sql.DB` on the same file with `mode=ro` and `_pragma=query_only(1)`, its own small connection cap), so flow reads never use the writer's single connection. Wiring: `server/dependencies.go` builds that client once and closes it on shutdown (inside the Story 3.1.3 construction block, no extra seam-file edit). The hard precondition test of Story 2.1.2 (pool label is `reader`) then runs against this client.
- Cost is carried in the effort table (S, 55k raw) whether or not the branch is taken; if 0.1.2a finds reads already reader-routed, the task is closed as not needed and its budget is released.
- Files: `session/ent_flow_reader.go` (constructor), `session/dbpool/` (only if no read-only constructor exists), `server/dependencies.go` (inside the Story 3.1.3 block).

#### Story 0.1.3: Golden fixtures from live numbers
**As a** developer, **I want** small synthetic fixtures that reproduce the live shapes, **so that** tests assert real numbers without 260k rows.
**Acceptance Criteria**:
- Fixture reproduces the thrash shape.
  - *Given* `session/flow/testdata/events_thrash.json`, *When* loaded, *Then* it contains, for synthetic item `thrash-a`, two `ready<->in_progress` flip runs (217 and 62 events at one-minute cadence) separated by a gap between 1h and 6h (live: 2 episodes at both 15m and 1h merge gaps, 1 at 6h), mirroring live item `0791f5b4` (330 events; 2 episodes at the 15-minute gap, verified by simulation 2026-11-05); for item `thrash-b`, 17 episodes at the 15-minute gap that merge to 5 at the 1h merge gap, mirroring live `a90b99cb` (8,691 events, `pr_pending<->in_progress`, empty notes); 4 mixed `-07:00`/`+00:00` rows; 2 self-loops; and 17 real `review>in_progress` bounces. Live counts drift, so the generator script recounts with `sqlite3 -readonly` and the fixture asserts shapes, not live totals.
- Fixture reproduces the verdict shapes.
  - *Given* `session/flow/testdata/verdicts.json`, *When* loaded, *Then* it has item `0791f5b4` with sequence `FRUUURURRURRRP`, item `5eaaa473` with `FFFFFFFFFRUFRFFFFFFFFUFP` ending in a human override, one "no committed changes" FAIL, one "codebase-read self-check failed" UNVERIFIABLE, one "Review blocked: no diff could be computed" UNVERIFIABLE, one "Auto-verified by reconcileBouncingItems" PASS, a run with the same failing `criterion_index` in 3 consecutive verdicts, and a run with an unchanged non-empty `diff_hash` between two consecutive verdicts. One verdict per session (the schema allows no more).
**Files**: `session/flow/testdata/events_thrash.json`, `session/flow/testdata/verdicts.json`, `session/flow/testdata/stuck.json`, `session/flow/fixtures_test.go`.

##### Task 0.1.3a: Author fixtures and loader (S)
- Generate by script from the live read-only DB shapes, with item titles and note bodies synthesized (no real note text). Re-run the count queries first; the live values on 2026-11-05 were 528 verdicts (FAIL 186, PARTIAL 82, PASS 150, UNVERIFIABLE 110), 378 non-PASS, 98 no-commits FAIL, 34 self-check UNVERIFIABLE, 18 other degraded UNVERIFIABLE (7 no-diff, 4 degraded, 7 no-acceptance-criteria), 23 auto-verified PASS.
- Files: the four above.

---

## Phase 1: `session/flow` pure core

### Epic 1.1: Types and helpers
**Goal**: The vocabulary, the injected clock, and the closed classifiers.

#### Story 1.1.1: Domain types
**As a** developer, **I want** newtypes and sealed enums for the glossary, **so that** invalid states do not compile.
**Acceptance Criteria**:
- Invalid windows are unrepresentable.
  - *Given* `Clock` fixed at `2026-11-05T12:00:00Z`, *When* `NewFlowWindow(clock, 30)` is called, *Then* it returns Start `2026-10-06T12:00:00Z`, End `2026-11-05T12:00:00Z`; and `NewFlowWindow(clock, 0)` returns an error.
- Defaults are in one place.
  - *Given* `DefaultConfig()`, *When* read, *Then* `ThrashGap=15m`, `ThrashMinFlips=6`, `ThrashMergeGap=1h`, `DeadlockMinRun=3`, `DeadlockRepeatCriterion=3`, `LowSampleN=5`, and `CollapsePad()` returns `2h30m`; with `ThrashGap=30m` it returns `4h`.
- Illegal spans do not compile or construct.
  - *Given* `NewThrashSpan` with `count=5`, *Then* it returns an error; `RealSpan` has no `Cause` or `Count` accessor.
- Statuses reuse the domain type.
  - *Given* `flow` source files, *Then* they import `session/domain` and define no `Status` type.
- All-time is an explicit input, not a zero-day window.
  - *Given* `NewAllTimeWindow(clock)`, *Then* it returns Start `2000-01-01T00:00:00Z`, End = clock now, and `WindowSpec{AllTime: true}` has a cache key distinct from every `Days` value; `NewFlowWindow(clock, 0)` still errors. RPC and MCP `all_time=true` maps to the all-time constructor and `window_days=0` without it is rejected (Stories 3.1.1, 3.2.1); the UI offers it as `?window=all` (Story 4.1.2).
**Files**: `session/flow/types.go`, `session/flow/span.go`, `session/flow/clock.go`, `session/flow/window.go`, `session/flow/config.go`, `session/flow/types_test.go`.

##### Task 1.1.1a: Types, clock, window, config, spans (S)
- Implement per Glossary; no `time.Now()` outside `SystemClock`. Table tests.
- Files: as above.

#### Story 1.1.3: Enforce the `session/flow` import boundary
**As a** maintainer, **I want** the "flow imports no ent, session, proto or server" rule checked by the linter, **so that** the next contributor cannot add `session.SanitizeForAgentContext` into `flow` and surface a cycle only at compile time.
**Acceptance Criteria**:
- Strict allow-list.
  - *Given* a `depguard` rule `isolate_flow_core` in `.golangci.yml` with `list-mode: strict`, `files: ["**/session/flow/**", "!**/*_test.go"]` (the bare `**/X/**` glob form the file's own comment prescribes), `allow: [$gostd, github.com/tstapler/stapler-squad/session/domain, golang.org/x/sync/singleflight]`, *When* a file in `session/flow` imports `github.com/tstapler/stapler-squad/session`, *Then* `make lint` fails naming the rule; with the import removed it passes.
- Transitive check.
  - *Given* the package, *When* `go list -deps ./session/flow | grep -E 'entgo|connectrpc|stapler-squad/(server|session$|session/ent)'`, *Then* it prints nothing.
**Files**: `.golangci.yml`.

##### Task 1.1.3a: Add rule and prove it fires (XS)

#### Story 1.1.2: Classifiers and percentiles
**As a** developer, **I want** table-driven note classification and nearest-rank percentiles, **so that** cause labels and dwell stats are testable.
**Acceptance Criteria**:
- Known notes map to a `Cause`; unknown notes land in `Unclassified` and are counted.
  - *Given* notes `"dequeued: WIP slot freed"`, `"dequeue spawn failed"`, `"auto-reopened for PR fix (CI/review)"`, `"auto-reopened after failed review verdict"`, `"some new note"`, *When* classified, *Then* causes are `DequeueClaim`, `DequeueSpawnFailed`, `PRFixReopen`, `ReviewReopen`, `Unclassified` and the unclassified count is 1.
- Percentiles are nearest-rank with sample size.
  - *Given* dwell samples `[1,2,3,4,100]` hours, *When* `Percentiles` runs, *Then* `N=5, P50=3, P90=100, Max=100, LowSample=false`; and with `[1,2,3]`, `LowSample=true`.
- A snapshot test fails on a new live note literal.
  - *Given* a committed list of distinct production note prefixes, *When* a literal is absent from the classifier table, *Then* `TestClassifierCoversKnownNotes` fails naming it.
**Files**: `session/flow/classify_notes.go`, `session/flow/percentile.go`, `session/flow/classify_notes_test.go`, `session/flow/percentile_test.go`.

##### Task 1.1.2a: Classifier table and percentile helper (S)
- Classifier is a slice of `{prefix, Cause}`; membership of thrash never depends on it (ADR-002).
- Files: as above.

### Epic 1.2: Events to spans to dwell

#### Story 1.2.1: Normalize events and collapse thrash
**As an** analyst, **I want** 258k raw events reduced to real transitions plus thrash episodes, **so that** totals and dwell are meaningful.
**Acceptance Criteria**:
- Normalization is order-safe across mixed offsets.
  - *Given* events `2026-08-10 01:00:00-07:00` and `2026-08-10 07:30:00+00:00` for one item, *When* normalized, *Then* the first (08:00Z) sorts after the second (07:30Z), and `At` values are UTC.
- Self-loops and negative gaps are counted, not used.
  - *Given* the fixture's 2 self-loops, *When* normalized, *Then* they are dropped and `DataQuality.SelfLoops == 2`.
- Collapse keys on shape.
  - *Given* a 217-event run of alternating `ready>in_progress`/`in_progress>ready` events one minute apart, *When* `Collapse` runs, *Then* it emits exactly 1 `ThrashSpan` with `Count=217`, `Cause=DequeueSpawnFailed`.
  - *Given* thousands of `in_progress>pr_pending` events with empty notes interleaved with `pr_pending>in_progress` with gaps under 15 minutes, *When* collapsed, *Then* they form thrash episodes with `Cause=PRFixReopen` despite empty notes.
- Live-shaped fragmentation is handled by merging.
  - *Given* the `thrash-b` fixture (17 episodes at the 15-minute gap, mirroring live `a90b99cb`), *When* collapsed with `ThrashMergeGap=1h`, *Then* it yields 5 episodes; with `ThrashMergeGap=0` it yields 17. The per-item row reports `episodes=5, events=<total>`; the banner counts the item once.
  - *Given* `thrash-a` (two runs, gap between 1h and 6h), *Then* 2 episodes at the default merge gap.
  - Reference (simulation re-run in the Phase 4 patch against the live DB, `/tmp` script, rerun in Story 6.1.1): **all causes together**, 103 episodes across 35 of 160 items that have events at the 15-minute gap, 61 after the 1h merge. By flip-pair family after the merge: `ready<->in_progress` 44 episodes (26 items), `queued<->in_progress` 10 (8 items), `pr_pending<->in_progress` 7 (3 items); 44 + 10 + 7 = 61. So the PR-fix thrash count is 7 episodes on 3 items, not 61. Of the 4,437 `pr_pending>in_progress` events, 4,341 sit inside thrash episodes and 96 are real reopens on 43 items.
- Boundaries are exact.
  - *Given* 5 alternating flip events, *Then* no episode (normal flow); *Given* 6 events, *Then* one episode. *Given* gaps of 14m59s then 15m01s between pairs, *Then* the run splits at the 15m01s gap. *Given* two episodes with a 59m59s vs 1h01m inter-episode gap, *Then* they merge vs stay separate.
- Collapse runs on padded events so boundary-straddling episodes are not truncated.
  - *Given* a flip run that starts 40 minutes before the window start and continues 10 minutes into it, *When* the reader supplies events from `window.Start - CollapsePad` and `Collapse` runs on them, *Then* the run is one `ThrashSpan` (with `StartsBeforeWindow=true`; reported `Count` is clipped to events inside the window) and none of its in-window flips is counted as a real `PRFix` or `DequeueClaim` transition.
  - *Given* the same run but events fetched only from `window.Start`, *Then* (negative test) the remainder of fewer than 6 events would be counted as real, which is what the padding prevents.
  - *Why the pad is enough*: a chain continuing across `window.Start - ThrashGap x ThrashMinFlips` with every gap under `ThrashGap` has at least `ThrashMinFlips` events inside the pad, so classification as an episode is exact; the extra `ThrashMergeGap` lets merging see the preceding episode. `TestCollapsePadDerivedFromConfig` fails if `CollapsePad` is not `ThrashGap x ThrashMinFlips + ThrashMergeGap`.
- Real transitions survive.
  - *Given* 17 `review>in_progress` events in the fixture, *When* collapsed, *Then* all 17 remain `RealSpan`s.
- Totals reconcile.
  - *Given* the fixture, *Then* `raw_events == real_transitions + collapsed_events + self_loops` (in-window events only; pad events are excluded from the equation).
**Files**: `session/flow/normalize.go`, `session/flow/collapse.go`, `session/flow/normalize_test.go`, `session/flow/collapse_test.go`.

##### Task 1.2.1a: Normalize (S)
- Parse tolerant layouts, order by `(At, Seq)`, drop self-loops, clamp/negate counts into `DataQuality`. Scope note: only the 10,654 rows before 2026-08-17 12:00 local carry `-07:00`, so a 30-day window never sees one; keep exactly one ordering test and one prefilter-superset test for the mixed-offset path (all-time windows only) and do not over-test it.
- Files: `session/flow/normalize.go`, `session/flow/normalize_test.go`.

##### Task 1.2.1b: Collapse (M)
- Pure `Collapse(events []Event, window FlowWindow, cfg Config) []Span` per ADR-002; events arrive padded by `cfg.CollapsePad()`; per-item pass, then merge pass (`ThrashMergeGap`), then clip to the window; small helper functions to stay under funlen/gocognit. Property test: random event streams keep `raw == real + collapsed + self` for in-window events.
- Files: `session/flow/collapse.go`, `session/flow/collapse_test.go`.

#### Story 1.2.2: Dwell, funnel, rework, first-pass yield
**As an** analyst, **I want** per-stage dwell, funnel counts, rework by cause, and FPY over collapsed spans, **so that** I can see where items leak.
**Acceptance Criteria**:
- Dwell is clipped to the window and thrash time is separate.
  - *Given* item in `in_progress` from `2026-10-01T00:00Z` with a thrash episode `2026-10-20T10:00Z..10:30Z`, window `[2026-10-06, 2026-11-05)`, *When* dwell runs, *Then* `in_progress` dwell starts at `2026-10-06T00:00Z`, the 30 minutes appear only in the `thrash` bucket, and seeded start status `in_progress` comes from the supplied last-event-before.
- Repeat visits and non-monotonic done are handled.
  - *Given* an item `done>archived>done>done`-style sequence with a `done>ready` reopen, *When* funnel runs, *Then* `MergedItem` membership uses only the first `pr_pending>done`.
- Open intervals use the as-of time.
  - *Given* an item still in `review` and clock `2026-11-05T12:00Z`, *When* dwell runs, *Then* its open interval ends at as-of and is flagged `open`.
- Rework counts follow cause rules.
  - *Given* a merged-cohort fixture with 105 items, bounce distribution 0:39, 1:13, 2:17, 3:5, 4:10, 5:7, 6:4, 7:3, 8:1, 9:1, 10+:5, *When* rework runs, *Then* zero-bounce count is 39, median bounces is 2, and the `thrash-b` item's thousands of PR-fix flips count as its 5 thrash episodes (not thousands of reopens) plus its real reopens.
- FPY is a ratio with denominator and does not pass thrash-only items.
  - *Given* 105 merged items of which 32 have zero bounces, zero real PR-fix reopens and zero thrash episodes before first merge, *When* FPY runs, *Then* it returns `32/105` (30.5%).
  - *Given* a merged item whose every PR-fix reopen collapsed into a `PRFixReopen` thrash episode (zero real reopens, zero bounces), *When* FPY runs, *Then* the item is not first-pass, and the response reports `thrash_affected` including it.
- Small samples are flagged.
  - *Given* a window with 3 merged items, *Then* `LowSample=true` and the percentage is still carried with `N=3`.
- Statuses outside the pipeline are not dropped.
  - *Given* `StageOrder = [idea, refining, ready, in_progress, review, pr_pending, done]` and an event into custom stage `qa`, *When* funnel runs, *Then* `qa` lands in the `other` bucket with its count and no panic; and `refining` appears as a funnel stage because it is in `StageOrder`.
- Review rework splits into `ReviewFail` and `Bounce` without double counting.
  - *Given* 426 real `review>in_progress` events of which 398 carry the note `auto-reopened after failed review verdict` and 28 do not (18 agent recoveries, 10 manual), *When* rework runs, *Then* `ReviewFail=398`, `Bounce=28`, the funnel back-edge and the bounce distribution use 426, and `ReviewFail + Bounce` equals the back-edge count.
- Median lead time has a backing metric.
  - *Given* merged items whose first `* > in_progress` event precedes first `pr_pending>done` by 1d, 3d and 5d, *When* lead time runs, *Then* `LeadTime` is `{N=3, P50=3d, P90=5d, Max=5d, LowSample=true}`, an item with no `in_progress` event is excluded and counted in `lead_time_excluded`, and the metric registers `DrillKey` `tiles.lead_time`.
**Files**: `session/flow/dwell.go`, `session/flow/funnel.go`, `session/flow/rework.go`, `session/flow/leadtime.go`, `session/flow/dwell_test.go`, `session/flow/funnel_test.go`, `session/flow/rework_test.go`, `session/flow/leadtime_test.go`.

##### Task 1.2.2a: Dwell (M)
- Per-stage `DwellBucket` + `thrash` bucket, clip, seed, open-interval, clamp negatives (counted).
- Files: `session/flow/dwell.go`, `session/flow/dwell_test.go`.

##### Task 1.2.2b: Funnel, rework causes, FPY, drill keys (M)
- Funnel over the supplied `StageOrder` (archived excluded from stages, included in throughput; unknown statuses to `other`), `ReworkCause` buckets (the `ReviewFail`/`Bounce` partition above), lead time, per-item transition-rate anomaly list, FPY with thrash exclusion; every metric registers its `DrillKey` and item set.
- Files: `session/flow/funnel.go`, `session/flow/rework.go`, `session/flow/leadtime.go`, plus tests.

#### Story 1.2.3: Combined PR-fix dwell
**As an** analyst, **I want** how long items sit in PR-fix cycles, **so that** the requirement for merge-conflict and CI-failure dwell has at least its combined form from day one.
**Acceptance Criteria**:
- Dwell per cycle, thrash excluded.
  - *Given* an item with real `pr_pending>in_progress` at T, then `in_progress>pr_pending` at T+6h, and a second real reopen at T+2d then `in_progress>review` at T+2d+3h, *When* PR-fix dwell runs, *Then* it has 2 cycles of 6h and 3h (`PRFixDwell` `{N=2, P50=3h, P90=6h, Max=6h, LowSample=true}`); a reopen still open at as-of ends at as-of and is flagged `open`; reopens inside a thrash episode produce episodes (Story 1.2.1), not dwell cycles.
- Combined, labeled as estimated, and the gap is stated.
  - *Given* no trigger capture, *Then* the section reports one combined dwell with `Estimated` containing `pr_fix_dwell:combined`, `pr_fix_cause:unavailable`, and Data notes stating "requirement asks for CI-failure and merge-conflict dwell separately; the split exists only for cycles after <capture date> (Story 5.2.3)".
- Drill.
  - *Given* the metric, *Then* it registers `DrillKey` `pr.fix_dwell` returning the items behind the cycles.
**Files**: `session/flow/prfix_dwell.go`, `session/flow/prfix_dwell_test.go`.

##### Task 1.2.3a: PR-fix dwell over real reopens (S)
- Pure function over spans; the trigger split is added by Story 5.2.3 (same function, grouped by `PRFixTrigger`).

### Epic 1.3: Review-gate analysis

#### Story 1.3.1: Verdict classification and per-session aggregation
**As an** analyst, **I want** non-judgment verdicts separated from real judgments, **so that** FAIL/UNVERIFIABLE rates are not misleading.
**Acceptance Criteria**:
- Closed classification with raw and adjusted rates.
  - *Given* summaries "no committed changes" (FAIL), "codebase-read self-check failed" (UNVERIFIABLE), "Review blocked: no diff could be computed" (UNVERIFIABLE), "Auto-verified by reconcileBouncingItems" (PASS), and a normal FAIL, *When* classified, *Then* classes are `NoCommits`, `SelfCheckFailed`, `ReviewerDegraded`, `AutoVerified`, `Judgment`, and the adjusted FAIL rate excludes the first three and the auto-verified one.
- Live totals reconcile.
  - *Given* the verdict fixture scaled to 528 verdicts (FAIL 186, PARTIAL 82, PASS 150, UNVERIFIABLE 110), *When* summarized, *Then* raw FAIL is 186 (35.2%), 132 of 378 non-PASS verdicts are non-judgments (98 `NoCommits` + 34 `SelfCheckFailed`), a further 18 UNVERIFIABLE are `ReviewerDegraded` (150 of 378 in total), and anything matching no table row is counted in `unrecognized`. The adjusted FAIL rate is 88 of 355 `Judgment` verdicts (24.8%; 355 = 528 - 98 - 34 - 18 - 23). The same 132 / 98 / 34 figures are used in Story 4.2.2 and ADR-005.
- Verdict join is safe, not a dedup.
  - *Given* an item with 9 sessions and 4 verdicts (each session has at most one verdict, enforced by the unique index `review_verdicts_item_session_review_verdict`), *When* the join runs, *Then* the item counts 4 verdicts, not 36. The per-session step is a join-safety guard only.
- Verdict mix over time.
  - *Given* verdicts across 5 ISO weeks, *When* `VerdictMixSeries` runs, *Then* it returns one bucket per week with counts per `VerdictClass` x outcome, empty weeks present with zeros (the zeros are real, the week had no verdicts), and bucket totals equal the window total.
**Files**: `session/flow/verdict.go`, `session/flow/verdict_series.go`, `session/flow/verdict_test.go`.

##### Task 1.3.1a: Verdict classes, join-safe aggregation, override rate (S)
- Table-driven summary classifier with `unrecognized` count; do not read `diff_truncated`.
- Files: `session/flow/verdict.go`, `session/flow/verdict_test.go`.

##### Task 1.3.1b: Weekly verdict-mix series (S)
- Files: `session/flow/verdict_series.go` and tests.

#### Story 1.3.2: Deadlock candidates and reviewer quality
**As an** operator, **I want** reviewer-vs-agent deadlock candidates, **so that** I can find items that grind without converging.
**Acceptance Criteria**:
- Candidate rule per ADR-005 uses a discriminating second signal, not `agent_claimed_done`.
  - *Given* a fixture item with run `F, R, F` (three judgment non-PASS verdicts) where criterion 3 fails in all three, *When* evaluated, *Then* it is a candidate with `RunLength=3`, `RepeatedCriterion=3`.
  - *Given* a run of 3 judgment non-PASS verdicts where each fails a different criterion and no two consecutive `diff_hash` values are equal and non-empty, *Then* it is not a candidate.
  - *Given* a run of 3 where two consecutive verdicts carry the same non-empty `diff_hash` and different failing criteria, *Then* it is a candidate with `DiffUnchangedPairs=1`.
  - *Given* item `0791f5b4` with `FRUUURURRURRRP` and an auto-verified final PASS, *Then* `Resolution=AutoVerified`; `AgentClaimedDone` is carried but removing it from the input does not change candidacy.
- Live yield is stated before the UI table is wired.
  - Reference (simulation on the live DB 2026-11-05, rerun in Story 6.1.1): 31 items have a run of at least 3 consecutive judgment non-PASS verdicts; 27 of them also meet a second signal (26 of the 34 runs have the same failing criterion in 3 consecutive verdicts; 11 items have an unchanged non-empty `diff_hash`). `diff_hash` is non-empty on only 140 of 528 verdicts and `item_sessions.last_commit_sha` is empty on all verdict sessions, so the diff signal is weak and the criterion signal carries most of the weight. The second signal therefore removes only 4 of 31; the table ranks by `RunLength` and shows both signals as columns so the owner can judge, and Data notes says so.
- Non-judgments do not break or extend runs.
  - *Given* verdicts `F, F(no committed changes), F, F`, *When* evaluated, *Then* the judgment run is 3 (the NoCommits is skipped).
- Human override resolution.
  - *Given* item `5eaaa473` with `FFFFFFFFFRUFRFFFFFFFFUFP` and final override by `user`, *Then* `Resolution=HumanOverride`.
- Reviewer-side streak is separate.
  - *Given* 3 consecutive real UNVERIFIABLE verdicts, *Then* the item appears under `ReviewerUnverifiableStreak`, not as a deadlock candidate.
- In-flight items are not called stalled.
  - *Given* an item in `review` whose latest verdict is 10 minutes old, *Then* it is flagged `InFlight` and excluded from the stalled count.
- Output is labeled heuristic and the default list is short and ranked.
  - *Given* any candidate list, *Then* `Estimated` contains `deadlock_candidates:heuristic`, the list is ranked by `RunLength` (ties: latest verdict first), the default page is 10, and the page carries `total_count` and a `weak_filter` note ("the no-progress signal removes few candidates; treat as a ranked review-fail streak list, not a verdict on the reviewer").
**Files**: `session/flow/deadlock.go`, `session/flow/deadlock_test.go`.

##### Task 1.3.2a: Deadlock and reviewer-streak detection (M)
- Inputs via `VerdictSource` (verdicts with per-criterion outcomes and `diff_hash`, `AgentDoneClaims`), `EventSource` (agent-triggered `in_progress>review` events) and `StuckSource` (cap exhaustion).
- Files: as above.

### Epic 1.4: Stuck metrics, assembly, comparison, cache

#### Story 1.4.1: Stuck metrics
**As an** analyst, **I want** current and historical stuck reasons with time-in-stuck, **so that** I can see which reasons linger.
**Acceptance Criteria**:
- Distinct items, composite bucket separate.
  - *Given* rows `(i1, autonomous_stuck)`, `(i1, multiple_reasons)`, `(i2, autonomous_stuck)`, *When* aggregated, *Then* `autonomous_stuck` has 2 distinct items and `multiple_reasons` is its own bucket with 1, not added to others.
- Tail-focused stats and lower-bound flag.
  - *Given* `bounce_cap_exhausted` rows with durations `[287.7h median shape, p90 815.1h, max 887.7h]`, *When* aggregated, *Then* P50/P90/Max/N are reported with `LowerBound=true` and `estimated` includes `stuck_history:lower_bound`.
- Open rows use as-of.
  - *Given* an unresolved `pr_needs_fix` first detected 3h before as-of, *Then* its time-in-stuck is 3h and it is counted in `open`.
**Files**: `session/flow/stuck.go`, `session/flow/stuck_test.go`.

##### Task 1.4.1a: Stuck stats (S)
- Interval source abstraction (`StuckInterval`) so Phase 5 episodes plug in without changing callers.
- Files: as above.

#### Story 1.4.2: Aggregator assembly, comparison, drill-down
**As a** consumer, **I want** one sectioned `FlowSummary`, a window comparison, and a drill-down for every number, **so that** UI and MCP stay identical.
**Acceptance Criteria**:
- Result shapes are declared first and frozen before the proto is written.
  - *Given* Task 1.4.2a, *When* its first commit lands, *Then* `session/flow/summary_types.go` declares the final Go result types (`FlowSummary` with every section and `FlowMeta`, `CompareResult`, `DrilldownPage`, `ItemTimeline`, `ListPage`, the `DrillKey` registry type) with no logic; Story 3.1.1 starts only after that commit, and any later field change to those types is a change to the proto in the same PR (the proto is a serialization point, so late field edits are not allowed to ripple silently into 3.1.2 and 3.2.x).
- Per-section status and meta.
  - *Given* a fake `VerdictSource` that errors (the other three fakes succeed), *When* `Summary` runs, *Then* the review section has `Status=Failed` with the error text, other sections are `OK`, and `FlowMeta.Estimated` lists the heuristic metrics.
- Thrash does not drown totals.
  - *Given* the thrash fixture (`thrash-a`, `thrash-b`), *When* `Summary` runs, *Then* `anomalies.thrash_items == 2` with per-item `episodes` and `events`, aggregates exclude collapsed events, and `notes.raw_events` reconciles with collapsed counts.
- Trends section feeds the charts.
  - *Given* a 30-day window, *When* `Summary` runs, *Then* `trends` carries weekly series for throughput (merged items per ISO week), verdict mix (`VerdictMixSeries`) and rework by cause, each with `N` per bucket, and the series totals equal the headline tiles' totals.
- Comparison returns raw numbers.
  - *Given* windows A `[2026-09-06, 2026-10-06)` and B `[2026-10-06, 2026-11-05)`, *When* `Compare` runs, *Then* each metric returns `{A, B, Delta, N_A, N_B}`, and an empty A yields `Delta = nil` rendered `n/a`, never infinity.
- Comparison accepts absolute anchors as well as offsets.
  - *Given* `CompareQuery` built from explicit bounds A `[2026-10-06, 2026-10-20)` and B `[2026-10-20, 2026-11-03)`, *When* `Compare` runs, *Then* it equals the result of the equivalent day-offset query; reversed or empty bounds return a validation error naming the field; a B end after as-of is clamped to as-of and the clamp is reported in `FlowMeta`; day-offset and explicit forms supplied together are rejected.
- Minimum sample is enforced, and no significance is claimed.
  - *Given* `MinCompareItems=20` and `MinCompareVerdicts=30` in `flow.Config`, *When* a per-item metric has `N_A=8` and `N_B=26`, *Then* that metric returns state `incomparable` with reason `low sample (N_A=8, N_B=26, need 20)`, still carries raw A, B, N_A, N_B and `Delta=nil`; verdict-rate metrics use 30; at exactly 20 (30) both windows compare normally (boundary test at 19/20). A reflection test fails if the compare result type has a field named like `significance`, `p_value`, `confidence`, `improved` or `worse`.
- Section selection and compact mode are applied here, once.
  - *Given* `FlowQuery.Sections=[tiles, review]`, *When* `Summary` runs, *Then* only those sections are computed, the others carry `Status=Skipped`, and the fake sources not needed by the selected sections record zero calls; an unknown section name returns a validation error listing the valid names (`tiles, funnel, causes, review, pr, stuck, anomalies, trends, notes`). *Given* `Compact=true`, *Then* `trends` bucket arrays are omitted (totals kept) and every top-N list is limited to 3 with the usual `total_count`/`has_more`.
- Every number has a drill.
  - *Given* each metric in `FlowSummary`, *When* iterated, *Then* each carries a `DrillKey` and `Drilldown(key, query)` returns its item IDs with title, status, reason and duration; a test fails if a metric lacks a key.
- Compare is undefined for the all-time window.
  - *Given* a `CompareQuery` whose window spec is all-time (`AllTime=true`), *When* `Compare` runs, *Then* it returns a validation error `ErrCompareAllTime` ("compare needs a bounded window; pick 7, 30 or 90 days"), never a previous-period guess.
- Item timeline collapses thrash.
  - *Given* fixture item `thrash-a` (two flip runs of 217 and 62 events), *When* `ItemTimeline` runs, *Then* it returns at most 50 spans with the 279 flips as 2 episodes and no raw flip events.
- Metrics that straddle the capture date are not compared as numbers.
  - *Given* windows A and B where `FlowMeta.CaptureSince` falls inside either window, *When* `Compare` runs on a stuck-history or PR-fix-split metric, *Then* that metric returns `incomparable` with the reason "window straddles capture_since <date>" instead of `{A, B, Delta}`; metrics not tied to capture are unaffected.
- Lists carry totals.
  - *Given* 35 thrash items and a default page of 10, *Then* the anomalies list has `total_count=35`, `returned_count=10`, `has_more=true`, `truncated=false`; with a requested limit of 200 clamped to 50, `truncated=true`.
**Files**: `session/flow/summary_types.go`, `session/flow/aggregator.go`, `session/flow/summary.go`, `session/flow/compare.go`, `session/flow/drilldown.go`, `session/flow/timeline.go`, `session/flow/sources.go`, `session/flow/trends.go`, `session/flow/aggregator_test.go`, and tests `session/flow/compare_test.go`, `session/flow/drilldown_test.go`, `session/flow/timeline_test.go`.

##### Task 1.4.2a: Source interfaces, summary assembly, sections (M)
- First commit: `summary_types.go` (result types only, see the freeze AC; Story 3.1.1 waits for it). Then define `EventSource`, `VerdictSource`, `StuckSource`, `ItemSource` and `Sources`; assemble sections from the pure functions; section-level error isolation; `Estimated[]`; weekly `trends`. Events are requested with `EventRange.From = window.Start - cfg.CollapsePad()`.
- Files: `session/flow/sources.go`, `session/flow/aggregator.go`, `session/flow/summary.go`, `session/flow/trends.go`, `session/flow/aggregator_test.go`. Section selection and compact mode live here (absorbed, no size change).

##### Task 1.4.2b: Compare, drill-down, item timeline (M)
- Same functions on two windows (day-offset or explicit bounds); minimum-sample rule; drill registry; timeline capped at 50.
- Files: `session/flow/compare.go`, `session/flow/drilldown.go`, `session/flow/timeline.go`, plus tests.

#### Story 1.4.3: TTL cache and singleflight
**As an** operator, **I want** repeated reads coalesced, **so that** UI refreshes and MCP polling cannot stack queries.
**Acceptance Criteria**:
- Coalescing.
  - *Given* 10 concurrent `Summary` calls for the same `(window spec, filters)` and fake sources counting calls, *Then* each source is invoked once.
- Key is bounded and clock-independent.
  - *Given* two calls 1s apart with `WindowSpec{Days: 30}`, *Then* they share one cache entry (the key is built from `WindowSpec`, not from clock-derived `Start`/`End`).
  - *Given* 200 distinct keys, *Then* the cache holds at most 64 entries (LRU, plus TTL eviction) and evicts the least recently used.
  - *Given* a query with non-default `Config` overrides or an `ItemIDs` filter (arbitrary MCP arguments), *Then* it bypasses the cache and is coalesced by singleflight only, so an agent looping over thresholds cannot grow the map.
- One uncached compute at a time.
  - *Given* 5 concurrent uncached `Summary` calls with 5 distinct non-default `Config` overrides and fake sources that block on a barrier, *Then* exactly one source read is in flight at any moment (the others wait on the one-slot limiter), a waiter whose context expires returns `ErrBusy`, and cache hits never take the slot.
- TTL with injected clock.
  - *Given* a fixed `Clock` advanced 31s, *When* `Summary` runs again, *Then* the reader is called again; at 29s it is not.
- Hits report original compute time.
  - *Given* a cache hit, *Then* `FlowMeta.QueryMs` equals the original compute value and `AsOf` is unchanged.
**Files**: `session/flow/cache.go`, `session/flow/cache_test.go`.

##### Task 1.4.3a: Cache decorator (S)
- `golang.org/x/sync/singleflight` (allow-listed in the depguard rule); key from `WindowSpec` plus cohort; 64-entry LRU; non-default `Config` or item-filtered queries bypass the cache.
- Files: as above.

---

## Phase 2: SQL reader

### Epic 2.1: `EntFlowReader`
**Goal**: Implement the four `flow` source interfaces (`EventSource`, `VerdictSource`, `StuckSource`, `ItemSource`) in one `EntFlowReader` over the repository client per ADR-004.

#### Story 2.1.1: created_at index
**As a** developer, **I want** an additive index, **so that** window reads are not full scans.
**Acceptance Criteria**:
- Conditional on Story 0.1.2: this story is dropped if the spike concludes "skip the index".
- Schema adds the index and no generated code is committed.
  - *Given* `session/ent/schema/backlog_status_event.go`, *When* `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` then `go build ./...`, *Then* build passes and `git status` shows only the schema file changed.
**Files**: `session/ent/schema/backlog_status_event.go`.

##### Task 2.1.1a: Add `index.Fields("created_at")` (XS, conditional)

#### Story 2.1.2: Event reader
**As the** aggregator, **I want** bounded padded event reads and a last-event-before seed, **so that** collapse is exact at window start and dwell is correct there.
**Acceptance Criteria**:
- Prefilter is a superset, Go does the exact filter, the range is padded.
  - *Given* seeded events at `2026-10-05 20:00:00-07:00` (=2026-10-06T03:00Z) and `2026-10-05 20:00:00+00:00` (=2026-10-05T20:00Z), `EventRange.From = window.Start - CollapsePad` (2026-10-05T21:30Z for a 2026-10-06T00:00Z start), *When* `Events(ctx, range)` runs, *Then* only the first is returned after Go filtering, and the SQL fetched both.
- Seed query is bulk and parses before comparing.
  - *Given* an item whose last pre-range event is `ready>in_progress`, *When* `LastEventsBefore(ctx, before, ids)` runs, *Then* it returns that event using one `GROUP BY item_id MAX(created_at)` query plus one fetch of the matching rows (both plain `SELECT`, so both route to the reader pool), then picks the latest by parsed time in Go. It is exact for ranges starting after 2026-08-18 and best-effort inside the mixed-offset span (documented on the method).
- Reader-pool routing is a test-asserted precondition, not a note (pre-mortem P1 #2).
  - *Given* the Task 0.1.2a spike result, *When* Story 2.1.2 is proposed for merge, *Then* `TestEntFlowReader_should_LabelReadsReaderPool_When_PlainSelectsOnly` passes, asserting via `session/dbobs` that `Events`, `CountEvents`, `StreamEvents` and `LastEventsBefore` all carry the `reader` pool label; if the spike found the ent client routes these to the writer, the story first routes the reader through the repository's existing reader handle, and a writer-labeled flow read fails the test and blocks the merge.
- Bounded, and cancellation reaches the statement.
  - *Given* a context timeout of 5s, *When* the query is cancelled, *Then* a typed error surfaces, the cancelled context is the one passed to the driver's `QueryContext` (the statement is interrupted, not just the caller released), and no connection is leaked.
**Files**: `session/ent_repository_flow.go`, `session/ent_repository_flow_events.go`, `session/ent_repository_flow_test.go`.

##### Task 2.1.2a: Events and seed queries (S)
- Sized S, not M: about 10k rows for 30 days, five projected columns plus rowid. Use `sql.Selector` through the ent client; no raw DB opens (`norawdb` lint); only plain `SELECT ... IN (...)` (`WITH`, `RETURNING` and window functions route to the writer per `session/dbsplit`); assert the pool label in a test.
- Files: `session/ent_repository_flow.go`, `session/ent_repository_flow_events.go`, `session/ent_repository_flow_test.go`.

#### Story 2.1.3: Verdict reader
**As the** aggregator, **I want** verdict and agent-done-claim inputs, **so that** deadlock analysis works.
**Acceptance Criteria**:
- Verdict-to-item join goes through `item_sessions`.
  - *Given* 3 sessions for item X, each with 1 verdict, plus a fourth session with no verdict and no `ended_at` (`review_verdicts` is unique per session, so one session cannot hold 2 verdicts), *When* `Verdicts(ctx, window)` runs, *Then* it returns 3 verdicts for X, each with `per_criterion` outcomes, `diff_hash` and `created_at`, and the verdictless session contributes nothing and does not error.
- Agent done-claims.
  - *Given* notes `in_progress` at T1 and `done` at T2 for criterion 3 and a verdict at T3 > T2, *Then* `AgentDoneClaims` returns `done` for that verdict (informational only; it does not gate deadlock candidacy).
- Request-review events come from `EventSource`, not here.
  - *Given* an `in_progress>review` event with `triggered_by=agent` and note `request_review from ...`, *When* `Events` returns it, *Then* `Event.TriggeredBy` and `Event.Note` carry the raw values and the aggregator's classifier flags `AgentTriggered`; a note "all work sessions ended" is flagged `ReconcilerInferred`.
**Files**: `session/ent_repository_flow_verdicts.go`, `session/ent_repository_flow_verdicts_test.go`.

##### Task 2.1.3a: Verdict and done-claim queries (M)
- Bulk `IN (...)` queries; no N+1.
- Files: as above.

#### Story 2.1.4: Stuck rows, item metadata, stage order
**As the** aggregator, **I want** stuck rows, raw item status/title and the pipeline's stage order, **so that** stuck, funnel and drill-down sections work.
**Acceptance Criteria**:
- Stuck rows including resolved.
  - *Given* 717 rows incl. 7 unresolved in a seeded DB, *When* `StuckIntervals(ctx)` runs, *Then* all rows are returned with `ResolvedAt` nil for exactly 7.
- Items return raw titles; sanitization happens once, in the MCP adapter.
  - *Given* an item title of 500 chars with a newline, *When* `Items(ctx, ids)` runs, *Then* the title is returned unmodified (the web UI shows what the item says), and a test asserts `session.SanitizeForAgentContext` is not referenced in `ent_repository_flow*.go`.
- Stage order comes from the pipeline definition.
  - *Given* the default workflow engine, *When* `Stages(ctx)` runs, *Then* it returns the built-in stages in pipeline order including `refining`; *Given* a `ConfiguredWorkflowEngine` with custom stage `qa`, *Then* `qa` is present at its configured position.
**Files**: `session/ent_repository_flow_stuck.go`, `session/ent_repository_flow_items.go`, `session/ent_repository_flow_stuck_test.go`, and (only if Task 0.1.1a finds no exported accessor) a small `PipelineStages(engine)` in `session/`.

##### Task 2.1.4a: Stuck, items and stages queries (S)

#### Story 2.1.5: Performance and concurrent-writer verification
**As an** owner, **I want** the SLO proven on live-size data and proof that an all-time read cannot stall writes, **so that** "under 1s for 30 days" is evidence and the live factory is safe.
**Acceptance Criteria**:
- SLO on the live-size copy.
  - *Given* a `.backup` copy of the live DB (about 258k events), *When* `Aggregator.Summary` runs for a 30-day window cold, *Then* wall time is under 1s; the number and plan are pasted in the PR.
- A concurrent writer is not blocked by an all-time read (pre-mortem P1 #2).
  - *Given* the `.backup` copy (env-gated `SSQ_FLOW_PERF_DB`) and a writer loop doing `recordStatusEvent`-style inserts, *When* an all-time `Summary` runs (chunked, Story 2.1.6), *Then* every concurrent write completes within 100ms and none returns `SQLITE_BUSY`.
  - *Given* a CI-resident test on a temp on-disk DB with the flow read paused on a barrier inside `StreamEvents`, *When* a write is issued, *Then* it commits without waiting on the barrier (proves the read holds no writer connection; no sleeps).
- Debug logging.
  - *Given* debug log level, *When* a summary computes, *Then* a log line carries `rows`, `query_ms`, `cache=miss`.
**Files**: `session/ent_repository_flow_perf_test.go` (live-size tests skipped unless `SSQ_FLOW_PERF_DB` is set), `session/ent_repository_flow_concurrency_test.go` (CI-resident).

##### Task 2.1.5a: Perf harness, concurrent-writer tests, log lines (S)
- Env-gated live-size test plus one deterministic CI test plus one log line; the `Observer` callback is wired in Story 3.1.3.

#### Story 2.1.6: Row ceiling, chunked read, one uncached compute at a time
**As an** operator, **I want** big windows chunked or refused and uncached computes serialized, **so that** an all-time view or an agent loop cannot load 258k rows at once or stack queries (pre-mortem P1 #2).
**Acceptance Criteria**:
- Row ceiling decides the read shape.
  - *Given* `ReaderLimits{MaxWindowRows: 60000, HardRowCeiling: 1000000, ChunkItems: 25}` and `CountEvents` returning 10,138 (30-day), 258k (all-time) and 1.2M (synthetic), *Then* the aggregator uses `Events` for the first, `StreamEvents` (item-batched) for the second, and returns typed `ErrWindowTooLarge` for the third; the MCP adapter maps `ErrWindowTooLarge` to the actionable error "window too large, use window_days <= 90" and the RPC to `connect.CodeResourceExhausted`.
- Chunked equals unchunked.
  - *Given* the thrash and verdict fixtures with `ChunkItems=1` and `ChunkItems=1000`, *When* `Summary` runs, *Then* the two summaries are identical (collapse, dwell and FPY are per-item, so item batches are exact), and memory holds collapsed spans, not raw events, between batches.
- Per-batch timeout and cancellation.
  - *Given* a batch that exceeds `BatchTimeout` (5s), *Then* the query is cancelled at the driver, the section fails with a typed error naming the batch, and no connection is leaked.
- Only one uncached compute at a time.
  - *Given* the limiter from Story 1.4.3, *When* two all-time requests with different item filters arrive together, *Then* one runs and the other waits or returns `ErrBusy`; neither runs concurrently with the other.
**Files**: `session/flow/limits.go`, `session/flow/stream.go`, `session/ent_repository_flow_stream.go`, `session/flow/limits_test.go`, `session/ent_repository_flow_stream_test.go`.

##### Task 2.1.6a: Limits, `CountEvents`, `ErrWindowTooLarge` mapping (S)

##### Task 2.1.6b: `StreamEvents` and per-batch collapse in the aggregator (M)

---

## Phase 3: Transports

### Epic 3.1: ConnectRPC `BacklogFlowService`
**Goal**: Thin adapter over the aggregator.

#### Story 3.1.1: Proto
**As a** client, **I want** typed messages for the sectioned summary, **so that** the UI is generated from one contract.
**Ordering**: written after the Task 1.4.2a type-freeze commit (`session/flow/summary_types.go`), not after 1.1.1; the messages mirror those frozen Go types field for field, so no proto field is invented before the shapes are final.
**Acceptance Criteria**:
- Service surface.
  - *Given* `proto/session/v1/backlog_flow.proto`, *When* `make proto-gen`, *Then* `BacklogFlowService` exposes `GetFlowSummary`, `ListDeadlockCandidates`, `ListThrashEpisodes`, `GetItemFlowTimeline`, `CompareFlowWindows`, `ListFlowDrilldown`, and `backlog.proto` is unchanged (`git diff --stat` shows 0 lines).
- Meta on every response.
  - *Given* a `GetFlowSummaryResponse`, *Then* it has `FlowMeta` (as_of, window, row_counts, query_ms, estimated[], slow_path, capture_since) and per-section `SectionStatus`.
- JSON casing is snake_case end to end.
  - *Given* the proto field names are snake_case (`as_of`, `row_counts`, `query_ms`, `capture_since`, `total_count`), *Then* every consumer that emits JSON uses `protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}` from one exported `FlowJSONOptions()` helper (the MCP adapter and the parity test both call it), so no lowerCamelCase (`asOf`) form exists anywhere; the UI's generated TypeScript client uses its own camelCase and is not part of the MCP contract.
- Window and list contract.
  - *Given* `all_time = true`, *Then* the request means all-time (documented on the field, `window_days` ignored when set); *Given* `window_days = 0` and `all_time = false`, *Then* the service returns `InvalidArgument` ("use all_time=true for all-time"); every list message carries `total_count`, `returned_count`, `has_more`, `truncated` (`ListPage`); section status is the string enum `ok|partial|failed|skipped|empty`.
- Every number carries its drill key in the contract.
  - *Given* the proto, *Then* every metric message (`MetricValue` and each tile, funnel edge, cause row, verdict class, stuck reason and PR-health count) has a `drill_key` string field, so the JSON an agent reads has a key beside every number; a descriptor-walk test fails if a message that carries a numeric `value`, `count` or `ratio` field lacks `drill_key` (explicitly listed exemptions: `row_counts`, `query_ms`, `ListPage` fields).
- Request and legend fields exist so the MCP adapter stays a thin pass-through.
  - *Given* the proto, *Then* `GetFlowSummaryRequest` has `window_days`, `all_time`, `repeated string sections` and `bool compact`; `CompareFlowWindowsRequest` has either `a_days`/`b_days`/`b_offset_days` or `a_start`/`a_end`/`b_start`/`b_end` (`google.protobuf.Timestamp`, each pair read as half-open `[start, end)`; the field comments say "end is exclusive"), and supplying both forms is `InvalidArgument`; the request has no `all_time` field, and the service maps `ErrCompareAllTime` to `InvalidArgument`; `FlowMeta` has `map<string,string> estimated_legend` (code to one-line meaning); `ListFlowDrilldownRequest.key` is validated against the `DrillKey` registry and an unknown key returns `InvalidArgument` with the message `unknown drill key "<k>"; valid keys: <comma-separated list>`.
- Trends and per-item anomaly rows exist in the contract.
  - *Given* the response, *Then* it has a `trends` section (weekly throughput, verdict mix, rework by cause) and anomaly rows keyed by item (`episodes`, `events`, `first_at`, `last_at`).
**Files**: `proto/session/v1/backlog_flow.proto` (generated code is gitignored).

##### Task 3.1.1a: Author proto and run `make proto-gen` (M)

#### Story 3.1.2: Service adapter
**As a** developer, **I want** a translate-only handler, **so that** no math lives in transport.
**Acceptance Criteria**:
- Translation only, one mapper.
  - *Given* a fake summarizer (a small interface satisfied by `*flow.Aggregator`) returning a `FlowSummary`, *When* `GetFlowSummary` is called with `days=30`, *Then* the response fields equal the summary's and the fake received `FlowQuery{Window: 30d}`. `server/services/backlog_flow_mapper.go` is the only place `FlowSummary` becomes `sessionv1` messages (the MCP tools reuse the service, Story 3.2.1).
- Error mapping.
  - *Given* nil aggregator or storage, *Then* `connect.CodeFailedPrecondition`; given an invalid window, `CodeInvalidArgument`.
- Markers.
  - *Given* each handler, *Then* it carries `// +api: backlog_flow:<action>`.
- Constructors for both wiring paths.
  - *Given* `NewBacklogFlowService(agg)` and `NewFlowServiceFromStorage(storage)` (builds `session.NewEntFlowReader(storage.GetEntClient())` and its own `flow.Aggregator`), *Then* both return a working service; the second is the stdio-fallback path.
**Files**: `server/services/backlog_flow_service.go`, `server/services/backlog_flow_mapper.go`, `server/services/backlog_flow_service_test.go`.

##### Task 3.1.2a: Handlers and mappers (M)

#### Story 3.1.3: Wiring and registry
**As a** maintainer, **I want** the service registered and CI-green, **so that** the PR passes the registry gate.
**Acceptance Criteria**:
- One aggregator instance, constructed in `server/dependencies.go`.
  - *Given* `server/dependencies.go`, *When* deps are built, *Then* one block creates `session.NewEntFlowReader(storage.GetEntClient())`, one `flow.Aggregator` (with an `Observer` that emits the `flow aggregate ...` debug log and the `flow.aggregate.duration_ms` / `flow.cache.hit` OTel instruments) and one `BacklogFlowService`, stored on the deps struct; a test asserts the HTTP MCP core and the RPC handler hold the same service pointer (so one cache serves both).
- Registered like Insights.
  - *Given* `server/server.go`, *When* the server starts, *Then* the log shows `Registered BacklogFlowService handler` and a `GetFlowSummary` call over `/api` returns 200 on a seeded test server.
- Registry gate.
  - *Given* `docs/registry/features/backend/*Flow*.json` generated by `make registry-generate`, *When* hand-edited to `tested:true` with `testIds` naming the handler tests and re-generated, *Then* `git diff --exit-code docs/registry/features/` is clean and "Check new RPCs have tests" passes.
**Files**: `server/server.go` (one registration line), `server/dependencies.go` (one construction block), `docs/registry/features/backend/GetFlowSummary.json` (and one per RPC), `docs/registry/features/` index files regenerated.

##### Task 3.1.3a: Wire service, registry JSON (S)

### Epic 3.2: MCP tools

#### Story 3.2.1: Aggregate MCP tools
**As Claude**, **I want** `get_flow_summary`, `compare_flow_windows`, `list_flow_drilldown`, **so that** one call answers each success metric.
**Acceptance Criteria**:
- Compact, labeled JSON.
  - *Given* `get_flow_summary` with `window_days=30`, *When* called, *Then* the JSON (marshalled with `FlowJSONOptions()`, snake_case) includes `as_of`, `row_counts`, `query_ms`, `estimated[]`, per-section `status` (`ok|partial|failed|skipped|empty`), and every top-N list has default 10, max 50 and the `ListPage` fields `total_count`, `returned_count`, `has_more`, `truncated`, so a clamped list cannot be mistaken for the whole set.
  - *Given* `all_time=true` (a boolean in the tool schema; `window_days` 1..365 otherwise, default 30), *Then* the tool returns the all-time summary with `slow_path=true` through the chunked path, or the `ErrWindowTooLarge` actionable error when the hard ceiling is exceeded; *Given* `window_days=0`, *Then* the tool returns an error saying "use all_time=true for all-time", so no magic number reaches the agent.
- Comparison, by offset or by absolute date.
  - *Given* `compare_flow_windows` with `a_days=30,b_days=30,b_offset_days=30`, *Then* each metric has A, B, delta, N_A, N_B.
  - *Given* `compare_flow_windows` with `a_start=2026-10-06,a_end=2026-10-20,b_start=2026-10-20,b_end=2026-11-03` (`YYYY-MM-DD` read as UTC midnight, or RFC3339), *Then* the result equals the equivalent offset call, which lets Claude anchor on a tuning commit's ship date. Bounds are half-open `[start, end)`: the example's A covers Oct 6 to Oct 19 and B covers Oct 20 to Nov 2, so Oct 20 is counted in B only; the tool description says "end is exclusive; to include Oct 19 as the last day of A pass a_end=2026-10-20" and the example call in the description is annotated with the covered days. A test builds an event at `2026-10-20T00:00:00Z` and asserts it lands in B and not in A, and asserts the description contains the substring "end is exclusive"; mixing the two forms, or giving `all_time`, returns an actionable error naming both forms; a metric below the minimum sample returns `incomparable` with reason (Story 1.4.2) and the tool description says deltas are descriptive and no significance test is computed.
- Drill keys are enumerable and a bad key is self-correcting.
  - *Given* the registered `list_flow_drilldown` schema, *Then* `key` is a JSON-schema `enum` generated at registration from the `DrillKey` registry (`flow.DrillKeys()`), so Claude cannot guess; a test fails if the schema enum differs from the registry.
  - *Given* `list_flow_drilldown{key:"rework.reviewfail"}`, *Then* the tool returns the tool error `unknown drill key "rework.reviewfail"; valid keys: <list>` (the RPC `InvalidArgument` text passed through, no stack trace).
- Tool descriptions are the ergonomics contract.
  - *Given* the six registered tool descriptions, *Then* `get_flow_summary` says to start there and names the next tool for each follow-up (`list_flow_drilldown` for the items behind a number, `compare_flow_windows` for before/after, the list tools for offenders); each description contains one example call payload; the summary description states that `as_of` is the cache age (the stdio process has its own cache); `list_flow_deadlock_candidates` is described as a heuristic ranked review-fail streak list. A test asserts these substrings.
  - *Given* any summary response, *Then* every numeric metric in the JSON has a `drill_key` string beside it (the same field the proto carries, Story 3.1.1), and a test walks the marshalled fixture response and fails on any object with a numeric `value`, `count` or `ratio` and no sibling `drill_key` (exemptions listed in the test: `row_counts`, `query_ms`, `ListPage` fields), and that every emitted `drill_key` is a member of the `flow.DrillKeys()` enum, so `get_flow_summary` chains to `list_flow_drilldown` without guessing.
  - *Given* any summary response, *Then* `estimated_legend` explains every code present in `estimated[]` (for example `deadlock_candidates:heuristic`, `pr_fix_cause:unavailable`, `stuck_history:lower_bound`), and a test fails if a code in the `Estimated[]` vocabulary (glossary `FlowMeta`) has no legend entry.
- Response size is bounded and selectable.
  - *Given* `get_flow_summary{sections:["tiles","review"]}`, *Then* only those sections are populated and the rest show `status:"skipped"`; `compact:true` applies the Story 1.4.2 compact rules. *Given* the live-shaped fixture (35 thrash items, 41 candidates, 5 weekly buckets), *Then* the marshalled JSON stays under a budget derived from measurement, not a guess: the test marshals the golden fixtures' default and compact payloads at run time, the budget constants (set once from the first measurement as `ceil(measured x 1.25)` and committed with the measured bytes in a comment) must satisfy `size <= constant` and `constant <= size_now x 1.5`, so a budget can neither be exceeded nor quietly loosened; the 32 KB and 8 KB figures of earlier drafts were guesses and are retired.
- Drill-down is paged.
  - *Given* `list_flow_drilldown{key:"rework.review_fail", limit:200}`, *Then* the limit is clamped to 50, `truncated=true`, `total_count` is the full item count, and `has_more` is true when more exist.
- In-process service call, `protojson` output, no second mapper.
  - *Given* `get_flow_summary`, *When* the handler runs, *Then* it calls `flowSvc.GetFlowSummary(ctx, connect.NewRequest(...))` (as `tools_backlog.go:1076` does for `ListBacklogItems`) and marshals the response with `protojson`. A test asserts `server/mcp/tools_backlog_flow*.go` contains no `FlowSummary`-to-JSON field mapping. MCP-only code: argument clamping, paging, sanitization.
- Untrusted text is sanitized once, here, for every string that can reach the agent.
  - *Given* an item title containing `ignore previous instructions\n`, *Then* the MCP adapter passes the title and note through `SanitizeForAgentContext` and truncation (300 chars) before marshalling; the reader and the RPC response keep raw titles.
  - *Given* the other free text that originates from agents, verdicts or stored notes (`repeated_criterion`, verdict `summary` and `resolution`, stuck `reason` detail, section `error` text, echoed `drill_key` and `key` arguments, `weak_filter` and legend strings built from stored values), *Then* the adapter does not sanitize field by field: it walks the response message by reflection (`protoreflect`, every string leaf, including those in maps, repeated fields and nested messages) and sanitizes and truncates every string leaf, except leaves on an explicit allowlist of server-generated safe fields (enum-valued strings such as `status`, `estimated[]` codes, `drill_key` values validated against the registry, timestamps, `window`) that is kept in one table next to the walker.
  - *Given* the proto descriptors, *Then* a reflection test walks every string field reachable from the six MCP responses and fails if a field is on neither the allowlist nor the sanitized path, so a string field added to the proto later cannot bypass sanitization; a fixture puts `ignore previous instructions\n` in every string field it can reach and asserts none survives in the marshalled JSON.
  - *Given* an unknown drill key echoed in an error, *Then* the echoed key is sanitized and truncated like any other leaf.
**Files**: `server/mcp/tools_backlog_flow.go`, `server/mcp/tools_backlog_flow_test.go`.

##### Task 3.2.1a: Three tools plus tests alongside `tools_backlog_test.go` conventions (M)
- Re-sized S to M in Triad repair 1: handlers are still clamp, call service, sanitize, `protojson`, but the schema enum, description contract, absolute dates and size-budget tests are real added work.

#### Story 3.2.2: List MCP tools
**As Claude**, **I want** `list_flow_deadlock_candidates`, `list_flow_thrash_episodes`, `get_item_flow_timeline`, **so that** I can inspect offenders.
**Acceptance Criteria**:
- Collapsed episodes only.
  - *Given* fixture item `thrash-a` with 279 raw flips in 2 runs, *When* `get_item_flow_timeline` runs, *Then* the JSON has 2 thrash spans, no raw events, at most 50 spans, `total_count` of spans, `has_more` set when truncated.
  - *Given* 35 thrash items, *When* `list_flow_thrash_episodes` runs with defaults, *Then* 10 items return with `total_count=35`, `has_more=true`, so an agent cannot conclude there are only 10.
- Candidates are paged and labeled.
  - *Given* 41 candidates, *When* `list_flow_deadlock_candidates` is called with defaults, *Then* 10 returned ranked by `run_length`, `total_count=41`, `has_more=true`, a `weak_filter` note string (same text as the UI banner), each with `verdict_sequence`, `resolution`, `run_length`, `repeated_criterion`, `diff_unchanged_pairs`, `agent_claimed_done`, `estimated` containing `deadlock_candidates:heuristic`.
- Free-form notes untrusted.
  - *Given* a request_review note of 800 chars, *Then* it is truncated to 300 and sanitized, and is omitted from aggregate responses.
**Files**: `server/mcp/tools_backlog_flow.go` (list handlers in `server/mcp/tools_backlog_flow_lists.go` if the file exceeds 300 lines), `server/mcp/tools_backlog_flow_lists_test.go`.

##### Task 3.2.2a: Three list tools plus tests (S)
- Sized S, not M: same shape as 3.2.1a, with `ListDeadlockCandidates`, `ListThrashEpisodes`, `GetItemFlowTimeline` called in-process.

#### Story 3.2.3: Option-based wiring for the HTTP and stdio paths
**As a** maintainer, **I want** the flow service injected through a variadic option on all three MCP entry points, **so that** no positional parameter is added to a function already at 10 and one cache serves both transports.
**Acceptance Criteria**:
- Signatures.
  - *Given* `server/mcp/server.go`, *Then* `type CoreOption func(*coreConfig)`, `WithFlowService(*services.BacklogFlowService) CoreOption`, and `opts ...CoreOption` is appended to `NewCore` (10 params on `origin/main`), `NewHTTPHandler` (10 params, `:120`) and `RunServer` (`:156`, 11 params including `ctx`); each forwards `opts` to `NewCore`. `git diff --stat` shows zero edits to the 7 existing `NewCore` test call sites.
- Optional injection.
  - *Given* `NewCore(...)` without `WithFlowService`, *When* `get_flow_summary` is called, *Then* it returns `ErrUnavailable`; with `WithFlowService(svc)` it returns data.
- HTTP path shares the RPC instance.
  - *Given* `server/server.go:1178` passes `servermcp.WithFlowService(deps.BacklogFlowService)`, *Then* the MCP core and the RPC handler hold the same service (and hence the same aggregator and cache).
- Stdio fallback decision (one instance, not shared): builds its own from storage.
  - *Given* `main.go:172` passes `mcpserver.WithFlowService(services.NewFlowServiceFromStorage(storage))` (storage is the only dependency the stdio path builds), *When* `get_flow_summary` runs over the stdio path against a seeded DB, *Then* it returns data; *Given* `storage == nil`, *Then* flow tools are not registered (they sit inside the `storage != nil` block). The stdio process has its own cache; its `as_of` makes that visible.
- Registered inside the backlog flag.
  - *Given* backlog disabled, *Then* flow tools are not registered.
- Parity holds by construction.
  - *Given* one `FlowQuery`, *When* the RPC handler and the MCP tool both run against the same fake summarizer, *Then* the MCP JSON equals `protojson.MarshalOptions{UseProtoNames:true, EmitUnpopulated:true}` (`FlowJSONOptions()`) of the RPC response for a fixture with clean titles (no field-name normalization step exists).
  - *Given* a fixture with non-clean titles (a title with `ignore previous instructions\n`, a 500-character title, a title with control characters), *Then* the MCP JSON equals the same marshal of the RPC response after applying the adapter's reflection-walk sanitizer and 300-character truncation to every non-allowlisted string leaf of the RPC response (Story 3.2.1), and the test names every field it expects to differ (titles, notes, and any other string leaf the fixture makes unclean), so sanitization cannot silently break parity.
  - *Given* an over-limit fixture (35 thrash items, 41 deadlock candidates), *Then* the MCP list returns `has_more=true`, `total_count=35` (41), `returned_count=10` while the RPC returns the full count, so truncation is never mistaken for the whole set.
**Files**: `server/mcp/server.go`, `server/mcp/tools_backlog_flow.go`, `server/server.go` (one argument), `main.go` (one argument), `server/mcp/tools_backlog_flow_parity_test.go`.

##### Task 3.2.3a: `CoreOption`, entry-point pass-through, two call-site arguments, tests (S)

---

## Phase 4: Web UI

### Epic 4.1: Route, data, chart wrapper

#### Story 4.1.1: Tabs and route
**As Tyler**, **I want** a "Flow" tab beside List and Board, **so that** I reach it from the backlog.
**Acceptance Criteria**:
- Shared tabs.
  - *Given* `BacklogViewTabs` extracted, *When* rendering `/backlog`, `/backlog/board`, `/backlog/flow`, *Then* all three show List/Board/Flow with `aria-current="page"` on the active one and no duplicated nav markup (jscpd clean).
- Flag inheritance.
  - *Given* the `backlog` feature flag off, *When* visiting `/backlog/flow`, *Then* the layout gate applies as for the Board.
- Marker.
  - *Given* `web-app/src/app/backlog/flow/page.tsx`, *Then* the first 10 lines contain `// +feature: backlog:flow`.
**Files**: `web-app/src/components/backlog/BacklogViewTabs.tsx`, `web-app/src/app/backlog/page.tsx`, `web-app/src/app/backlog/board/page.tsx`, `web-app/src/app/backlog/flow/page.tsx`, `web-app/src/app/backlog/backlog.css.ts`.

##### Task 4.1.1a: Extract tabs, add route shell (S)
- Audit every render site of the tab strip (memory: audit all call sites on shared gating).

#### Story 4.1.2: Data hook, URL state, state matrix
**As Tyler**, **I want** correct loading, stale, error and empty states, **so that** I never read a fake zero.
**Acceptance Criteria**:
- No fake zeros.
  - *Given* first load pending, *Then* tiles show skeletons, never "0".
- Stale and partial.
  - *Given* a refresh fails after success, *Then* last data stays dimmed with `Showing data from 14:02 - refresh failed (Retry button)`; *Given* the review section has `Status=Failed`, *Then* only that section shows an error block with a `Reload` button (not "Retry section": it re-issues the whole RPC, so the label says reload).
  - *Given* `FLOW_POLL_INTERVAL_MS = 60000` (exported constant in `useBacklogFlow.ts`; polling runs only while the tab is visible and never announces), *When* the fake clock advances 121s since the last successful fetch with every poll failing, *Then* the page is stale: dimmed with the "Stale" text label and the banner `Showing data from 14:02 - refresh failed [Retry]`; at 119s it is not stale. Staleness is measured from the last successful client fetch, not from `as_of` (server clock skew and the 30s server TTL would distort it). A successful poll clears the state.
- URL carries state.
  - *Given* `?window=30&compare=prev&drill=rework.review_fail`, *When* the page loads, *Then* the selectors and drawer reflect those values and back-button restores prior state.
- Freshness from server.
  - *Given* a response with `as_of` 14:02:11, *Then* the header shows that time and relative age, not the client clock.
- Window and compare selectors (UX S2).
  - *Given* the header, *Then* window (`7d`, `30d`, `90d`, `All time`) and compare (`none`, `previous period`) are `role=radiogroup` with accessible names; ArrowRight/ArrowLeft move the selection and update the URL (`?window=all` for all-time, sent as `all_time=true`); selecting All time shows a one-line "slow path" note when the response has `slow_path=true`.
  - *Given* `?window=13`, *Then* the page uses 30 and shows the inline note "Unsupported window, showing 30d".
- All time over the row ceiling gets its own state (UX criterion 45).
  - *Given* window All time and the RPC returns `ResourceExhausted` (the service's mapping of `ErrWindowTooLarge`, Story 2.1.6), *Then* the page shows a `window-too-large` state, not the generic hard error: the text "All time has too many events to load (N rows, limit M). Pick a shorter window." with the 7d, 30d and 90d choices as buttons that set `window` (focus moves to the first), the header selector stays usable, and no Retry button is offered for this state (retrying cannot succeed). A Jest test stubs the `ResourceExhausted` reject and asserts the text, the three buttons, absence of Retry, and that the URL becomes `?window=30` after clicking 30d; the e2e stub route covers the same.
- Compare is not offered for All time (UX criterion 46).
  - *Given* window All time, *Then* the compare radio `previous period` is `aria-disabled` with the visible hint "Compare needs a 7, 30 or 90 day window", the selection reads `none`, and the request never sends a compare for all-time; *Given* `?window=all&compare=prev` in the URL, *Then* the page loads with compare `none`, removes `compare` from the URL and shows the same hint; *Given* the owner selects `previous period` and then All time, *Then* compare resets to `none` with that hint; selecting a bounded window again does not restore it silently. Tested in Jest.
- Refresh, live region, jump list (UX S2).
  - *Given* the Refresh button, *When* clicked, *Then* it re-fetches without changing selection, shows "Refreshing..." with `aria-disabled` (still in the tab order), and a `LiveRegion` announces "Flow updated, as of 14:02" once; background polls announce nothing.
  - *Given* the jump list (`Leaks | Causes | Review | PR | Stuck | Anomalies`), *When* a link is activated, *Then* the page scrolls to the `h2` and focus moves to it (`tabindex=-1`).
  - *Given* "2 metrics estimated" in the header, *When* activated, *Then* Data notes opens and receives focus.
- Lower sections are collapsed by default (density rule, UX criterion 43).
  - *Given* a successful response, *When* the page renders, *Then* sections A to E (tiles, funnel, causes, review, PR health) are expanded and sections F (Stuck reasons) and G (Anomalies) render as `Collapsible` with their `h2` and a one-line summary (for example "8 open, 46 historical episodes"; "35 items thrashing"); *Given* the jump list or the thrash banner View button targets a collapsed section, *Then* it expands and focus moves to its `h2`; *Given* a collapsed section has `Status=Failed|Partial`, *Then* it renders expanded; the funnel `h2` is in the 1080p viewport (criterion 1) with the collapsed sections closed. Tested in Jest (rule) and e2e (viewport).
- Invalid URL state.
  - *Given* `?window=13&drill=bogus`, *Then* the page shows "Unsupported window, showing 30d" and "Unknown drill target ignored", removes the bad params and does not throw (criterion 18).
- Page-state surfaces (UX S1, S3).
  - *Given* an empty 7-day window, *Then* the section shows "No items finished in the last 7 days" with a "Widen to 30d" button that sets `window=30`; *Given* the server returns `unavailable`, *Then* an `ErrorState` shows the server message with Retry and a Board link; *Given* the route throws, *Then* the Next error boundary shows a "Back to Backlog" link.
**Files**: `web-app/src/lib/hooks/useBacklogFlow.ts`, `web-app/src/lib/api/idempotentMethods.ts` (register the read RPCs), `web-app/src/app/backlog/flow/FlowPage.tsx`, `web-app/src/app/backlog/flow/FlowHeader.tsx`, `web-app/src/app/backlog/flow/error.tsx`.

##### Task 4.1.2a: Hook, URL state, header controls, state matrix (M)

#### Story 4.1.3: FlowChart wrapper
**As a** developer, **I want** one accessible chart wrapper, **so that** no chart can skip a11y rules.
**Acceptance Criteria**:
- Required a11y structure.
  - *Given* `<FlowChart>` with data, *Then* it renders `figure` + `figcaption`, `role="img"` with a takeaway `aria-label`, an adjacent hidden `<table>` with the same values, and non-color encoding (pattern or marker).
- Chart states.
  - *Given* empty data, *Then* the figure renders "No data in this window" text, not an empty box; *Given* the chart body throws, *Then* an error boundary renders the data table only with the note "Chart unavailable, showing data"; *Given* any chart, *Then* a "Show data table" toggle button (`aria-expanded`) reveals the table for sighted users while the table stays available to assistive tech either way.
- Theme tokens only.
  - *Given* the CSS, *Then* `pnpm run lint:css-vars` passes; no raw colors.
- Consumers exist.
  - *Given* the wrapper, *Then* Story 4.2.5 (verdict-mix chart, throughput and rework sparklines) is its consumer; if 4.2.5 is dropped, this story is dropped with it.
**Files**: `web-app/src/components/backlog-flow/FlowChart.tsx`, `FlowChart.css.ts`, `FlowChart.test.tsx`.

##### Task 4.1.3a: Wrapper plus test (S)

### Epic 4.2: Sections

#### Story 4.2.1: Health tiles and funnel
**As Tyler**, **I want** tiles and a funnel with rework back-edges, **so that** I see whether it is healthy and where it leaks.
**Acceptance Criteria**:
- Tiles show counts with ratios and deltas.
  - *Given* FPY `32/105`, *Then* the tile shows "32/105 (30.5%)" with `data-testid="flow-tile-fpy"`; with `N=3` it greys the percentage and shows "low sample".
- Review FAIL tile leads with the adjusted rate.
  - *Given* raw FAIL 186/528 and adjusted 88/355, *Then* the tile (`data-testid="flow-tile-review-fail"`) shows the adjusted rate as the primary value "88/355 (24.8%)" labeled "FAIL, judgment verdicts only" and the raw rate as a secondary line "raw 186/528 (35.2%)"; both are drill buttons.
- Compare empty and low sample.
  - *Given* compare window empty, *Then* delta shows "n/a (no data in previous period)"; *Given* a metric returned `incomparable` for low sample, *Then* the delta shows "n/a (low sample: 8 vs 26 items, need 20)" with the raw values still visible and no better/worse word.
- Funnel back-edges show real and excluded values, both labeled.
  - *Given* `pr_pending -> in_progress` with 96 real reopens and 4,341 raw events in 7 thrash episodes, *Then* the back-edge reads "pr_pending -> in_progress: 96 real reopens (4,341 raw events in 7 thrash episodes excluded)" with the real count as the primary number; the raw 4,437 appears only inside the label.
- Funnel stages come from the response, not a constant.
  - *Given* a response whose `stage_order` is `idea, refining, ready, in_progress, review, pr_pending, done` and an `other` bucket of 3 items, *Then* each stage renders a bar showing entered, advanced, median/p90 dwell and back-edges such as "review > in_progress 426", plus an "Other stages: 3" row, with a visually hidden table; `data-testid="flow-funnel-stage-<name>"`. A test with a custom `stage_order` renders those stages and no hard-coded ones.
**Files**: `web-app/src/components/backlog-flow/FlowTiles.tsx`, `FlowFunnel.tsx`, `flow.css.ts`, `flowFormatters.ts`.

##### Task 4.2.1a: Tiles and funnel (M)

#### Story 4.2.2: Causes and review gate
**As Tyler**, **I want** ranked rework causes and a review section with raw and adjusted rates, **so that** I do not misread the 35% FAIL headline.
**Acceptance Criteria**:
- Ranked causes with drill.
  - *Given* causes ReviewFail 398 (93 items with at least one `review>in_progress`), PRFix with 7 thrash episodes on 3 items plus 96 real reopens on 43 items, Bounce 28, *Then* rows are ranked, show items affected, and each count is a button opening the drawer with `drill=rework.review_fail`. (PRFix is not 61: 61 is the all-cause thrash episode count.)
  - *Given* the Causes table, *Then* column headers are sort buttons (Enter/Space) that set `aria-sort` (`ascending|descending|none`) on the active `th`, default count descending, and a Jest test asserts the attribute changes (UX criterion 41).
- Raw vs adjusted rates.
  - *Given* raw FAIL 186/528 (35.2%) and an adjusted rate over `Judgment` verdicts only (528 minus 98 `NoCommits`, 34 `SelfCheckFailed`, 18 `ReviewerDegraded` and 23 `AutoVerified` = 355; adjusted FAIL 88/355, 24.8%; 132 of 378 non-PASS are non-judgments, 150 of 378 with the degraded ones), and raw UNVERIFIABLE 110/528 (20.8%), *Then* the adjusted rate is primary and the raw rate secondary, both shown with a note "98 'no committed changes' and 34 self-check failures excluded from adjusted; 18 reviewer-degraded results and 23 auto-verified passes also excluded".
- Deadlock table, named for what it can prove.
  - *Given* the Story 6.1.1 precision check has not promoted the name, *Then* the section heading is "Review-fail streaks (heuristic)" and the "Agent claimed done" column is hidden by default behind a "Show agent-claimed-done (info only)" toggle; *Given* the recorded hit rate is at least 50%, *Then* the heading is "Deadlock candidates" (one display-name constant, no key or `estimated` tag change).
  - *Given* candidates, *Then* the table shows item, sequence `FRUUURURRURRRP`, resolution, run length, repeated-criterion and unchanged-diff columns, and agent-claimed-done (informational), wrapped in `EstimatedValue` with the heuristic explained in Data notes (including that the second signal removes few live candidates). The list defaults to the top 10 by run length with a visible "weak filter" note ("treat as a ranked review-fail streak list, not a verdict on the reviewer"), "Showing 10 of N", and a Show all control.
**Files**: `web-app/src/components/backlog-flow/FlowCauses.tsx`, `FlowReview.tsx`, `DeadlockTable.tsx`.

##### Task 4.2.2a: Causes, review, deadlock table (M)

#### Story 4.2.3: PR health, stuck, anomalies, data notes
**As Tyler**, **I want** the remaining sections plus a thrash banner, **so that** anomalies cannot be missed.
**Acceptance Criteria**:
- Thrash banner and section count items, not episodes, archived items included and labeled.
  - *Given* 35 items with episodes of which 2 are archived (the all-time view), *Then* the banner reads `35 items thrashing (2 archived); excluded from averages`, the Anomalies rows for archived items carry an "archived" text chip, and archived thrashers count in the banner because their events are excluded from the aggregates just as live ones are (the funnel still excludes archived items from stages and includes them in throughput).
  - *Given* 2 thrashing items in the window (one with 2 episodes, one with 5), *Then* a banner `2 items thrashing; excluded from averages` with a View button appears under the header and section Anomalies lists one row per item: item, flip pair, episodes, total events, duration, last seen, with `#anomalies` anchor. The all-time view on live data shows 35 items; the banner copy is not fixture-specific.
- Stuck section labels lower bound.
  - *Given* `stuck_history:lower_bound` in `estimated[]`, *Then* the stuck table shows an `EstimatedValue` marker and text "history before <capture_since> is a lower bound".
- PR health.
  - *Given* PR-fix reopens of 7 thrash episodes (3 items) plus 96 real reopens and no trigger capture (`pr_fix_cause:unavailable` in `estimated[]`), *Then* the section shows the reopen count and a single "PR fix (CI/review/conflict not separable)" cause with the gap stated ("CI vs conflict split not recorded before <capture date>").
  - *Given* `PRFixDwell` and `pr_fix_dwell:combined` in `estimated[]` (Story 1.2.3), *Then* the section shows combined PR-fix dwell (median, p90, N) as an `EstimatedValue` with the text "CI and conflict dwell are not separable before <capture date>"; each number is a drill button (`pr.fix_dwell`).
  - *Given* no outcome capture exists yet (Story 5.2.2 not recording), *Then* the outcomes block shows "Outcome tracking not yet recording" and no numerals, never zeros.
  - *Given* trigger capture exists (Story 5.2.3), *Then* the section splits the bucket and the dwell by `PRFixTrigger` (`ci`, `conflict`, `mixed`, `review`, `feedback`, `unknown`) for events after the capture date, flagged `estimated: pr_fix_cause:since_capture` and `pr_fix_dwell_split:since_capture`.
- Unavailable variants never read as "nothing found".
  - *Given* anomaly data failed, *Then* the banner is replaced by "Anomaly data unavailable [Reload]" (never absent), and the Anomalies section shows its own error block.
  - *Given* `FlowMeta` is missing, *When* Data notes opens, *Then* it shows "Methodology details unavailable" (never blank).
- Data notes.
  - *Given* the closed-by-default `Collapsible`, *When* opened, *Then* it shows collapse rules, sources, estimated list, row counts, query ms, and "Archived excluded from stages, included in throughput".
**Files**: `web-app/src/components/backlog-flow/FlowPR.tsx`, `FlowStuck.tsx`, `FlowAnomalies.tsx`, `FlowDataNotes.tsx`. The anomalies "View events" action is Story 4.2.6.

##### Task 4.2.3a: PR health and stuck sections (M)
- Files: `FlowPR.tsx`, `FlowStuck.tsx`.

##### Task 4.2.3b: Anomalies section, thrash banner, data notes (M)
- Split from one task: four sections in one M task was the likeliest underestimate in the UI phase.
- Files: `FlowAnomalies.tsx`, `FlowDataNotes.tsx`, banner in `FlowPage.tsx`.

#### Story 4.2.5: Trends (the `FlowChart` consumers)
**As Tyler**, **I want** weekly verdict mix, throughput and rework-cause trends, **so that** I can see whether a change moved the numbers, and the `FlowChart` wrapper has real consumers.
**Acceptance Criteria**:
- Verdict mix over time.
  - *Given* `trends.verdict_mix` with 5 weekly buckets, *Then* a stacked `FlowChart` renders PASS / FAIL / PARTIAL / UNVERIFIABLE segments with distinct patterns plus text labels, a takeaway `aria-label` ("FAIL share fell from 41% to 28% over 5 weeks"), a hidden table, and an empty week renders as a labelled zero, not a gap.
- Throughput and rework sparklines.
  - *Given* `trends.throughput` and `trends.rework_by_cause`, *Then* the Causes table rows (S7) and the Throughput tile each embed a `FlowChart` sparkline with first/last weekly values printed as text.
- Coverage guard.
  - *Given* the page, *Then* a test asserts `FlowChart` is rendered by at least these three consumers (no dead scaffolding).
**Files**: `web-app/src/components/backlog-flow/FlowTrends.tsx`, `FlowVerdictMix.tsx`, tests.

##### Task 4.2.5a: Verdict-mix chart and sparklines (M)

#### Story 4.2.6: Item timeline view ("View events")
**As Tyler**, **I want** the collapsed event timeline of one thrashing item, **so that** "View events" in the anomalies table shows what happened without 87,000 raw rows.
**Acceptance Criteria**:
- Timeline opens from the anomalies row.
  - *Given* an anomalies row for `a90b99cb`, *When* "View events" is activated, *Then* a dialog calls `GetItemFlowTimeline` for that item and lists at most 50 spans (thrash episodes as one row each with count, first/last, flip pair, cause), Escape closes it and focus returns to the invoking button; `data-testid="flow-item-timeline"`.
- States.
  - *Given* a failing RPC, *Then* the dialog shows "Could not load events [Retry] [Close]"; *Given* more than 50 spans, *Then* it shows "Showing 50 of N spans"; *Given* a loading call, *Then* skeleton rows with `aria-busy`.
**Files**: `web-app/src/components/backlog-flow/FlowItemTimeline.tsx`, `web-app/src/lib/hooks/useFlowItemTimeline.ts`, `FlowItemTimeline.test.tsx`.

##### Task 4.2.6a: Timeline dialog and hook (S)

#### Story 4.2.4: Drill-down drawer
**As Tyler**, **I want** every number to open the items behind it, **so that** I can tell a real leak from one pathological item.
**Acceptance Criteria**:
- Drawer contract.
  - *Given* a click on "Review-FAIL rework, 30d", *Then* a dialog opens headed "Review-FAIL rework, last 30d, 12 items" listing title, status chip, reason, duration; Escape closes; focus returns to the invoking button.
- Drawer states, each with an action.
  - *Given* the RPC is pending, *Then* skeleton rows with `aria-busy`; *Given* the RPC fails, *Then* "Could not load items [Retry] [Close]"; *Given* the header count is 12 and zero rows return, *Then* "No items match (the count may have changed since as-of 14:02) [Refresh]"; *Given* the header count differs from the row count, *Then* "Counts changed since 14:02; refresh"; *Given* 240 items with a cap of 100 rows, *Then* "Showing 100 of 240 items" plus an "Open full list in List view" link when a List filter exists for the key, else the note alone.
  - *Given* `?drill=bogus` (a key not in the registry, validated client-side against the generated key list and server-side by the RPC), *Then* the drawer does not open, the inline notice "Unknown drill target ignored" shows and the param is removed.
- Close removes the param.
  - *Given* the drawer is open from `?drill=rework.review_fail`, *When* closed by Escape, X or Close, *Then* `drill` is absent from the URL (criterion 20).
- Item link.
  - *Given* a row, *Then* it links to `?item=<id>` of the existing detail panel.
- Coverage guard.
  - *Given* every numeric element rendered in the page tests, *Then* a test asserts each has an associated `DrillKey` button (no orphan numbers).
**Files**: `web-app/src/components/backlog-flow/FlowDrilldownDrawer.tsx`, `FlowDrilldownDrawer.test.tsx`.

##### Task 4.2.4a: Drawer (M)

### Epic 4.3: Tests and registry

#### Story 4.3.1: Jest tests
**As a** maintainer, **I want** component tests for populated, empty, error, partial and low-sample states.
**Acceptance Criteria**:
- State matrix covered.
  - *Given* fixtures for first load, success, stale, hard error, partial, empty window, low sample, compare-empty, compare low-sample (`incomparable`), thrash present (live and archived), anomaly-unavailable, methodology-unavailable, drawer states (loading, error, empty with mismatch, truncated, bad key), `FlowChart` states (empty, throwing, table toggle), collapsed lower sections, *When* `cd web-app && pnpm exec jest --no-coverage --testPathPatterns="backlog-flow"`, *Then* all pass.
- Duplication gate.
  - *Given* the new files, *When* `pnpm run lint:duplicates`, *Then* it passes (threshold 0.1%).
**Files**: `web-app/src/components/backlog-flow/*.test.tsx`, `web-app/src/app/backlog/flow/__tests__/FlowPage.test.tsx`.

##### Task 4.3.1a: Jest suites plus shared fixtures (M)

#### Story 4.3.2: E2E and Axe
**As a** maintainer, **I want** an e2e spec that follows conventions and passes Axe in both themes.
**Acceptance Criteria**:
- Conventions.
  - *Given* `tests/e2e/backlog-flow.spec.ts`, *Then* line 1 is `// @feature backlog:flow, ...`, no `waitForTimeout`, only `data-testid`/ARIA locators, helper in `tests/e2e/pages/BacklogFlowPage.ts`.
- Flow.
  - *Given* a seeded isolated test server with one thrash item, *When* opening `/backlog/flow?window=30`, *Then* the thrash banner shows "1 item thrashing" (singular copy) and clicking the FPY tile opens the drawer.
- Click counts for the task-completion criteria are asserted, not assumed.
  - *Given* the page helper counts clicks, *Then* criterion 2 (items behind a number in 1 click, detail panel in 2), criterion 3 (compare in 1 click), criterion 4 (banner View in 1 click) and criterion 5 (Flow from Backlog in 1 click) each assert the exact count, and criterion 1 asserts the funnel is in the 1080p viewport with collapsed sections closed.
- Non-happy flows via `page.route` stubs.
  - *Given* a failed refresh after success, *Then* the stale banner and Retry show and a later success clears them; *Given* a hard error on first load, *Then* server text, Retry and the Board link show; *Given* an empty 7-day window, *Then* "Widen to 30d" sets `window=30`; *Given* `?window=30&compare=prev&drill=rework.review_fail`, *When* the window changes and the browser goes back, *Then* the previous selectors and drawer return; *Given* a failed drawer call, *Then* "Could not load items [Retry] [Close]".
- Accessibility checks that Axe cannot do (UX criteria 21, 26, 27, 30, 33).
  - *Given* the populated page, *Then*: keyboard-only traversal reaches tabs, header, jump list and tiles in reading order and Enter opens the drawer (21); Axe `color-contrast` passes in both themes, with a manual chart-mark contrast pass recorded in Story 6.1.1 (26); every button, link and radio has `boundingBox()` width and height of at least 24 (27); with `reducedMotion: 'reduce'` the computed `transition-duration` of bars and charts is 0s (30); at a 320x800 viewport `documentElement.scrollWidth <= clientWidth` with tables scrolling inside labelled regions (33).
- Axe.
  - *Given* populated, empty, error and partial states in light and dark, *Then* Axe reports no WCAG AA violations.
**Files**: `tests/e2e/backlog-flow.spec.ts`, `tests/e2e/pages/BacklogFlowPage.ts`.

##### Task 4.3.2a: Spec, page helper, click-count and non-happy flows (M)

##### Task 4.3.2b: Accessibility checks for criteria 21/26/27/30/33 and Axe runs (S)
- Split in Triad repair 1: the original single M task was the likeliest underestimate once the click-count and state flows were added.

#### Story 4.3.3: Frontend registry entry
**As a** maintainer, **I want** the `backlog:flow` feature registered `tested:true`.
**Acceptance Criteria**:
- *Given* `make registry-generate`, *When* run, *Then* `docs/registry/features/frontend/` contains `backlog:flow` with `tested:true` and testIds naming the jest and e2e specs, and `git diff --exit-code docs/registry/features/` is clean afterwards.
**Files**: `docs/registry/features/frontend/` (generated entry, hand-edited tested fields).

##### Task 4.3.3a: Registry edit and regenerate (XS)

---

## Phase 5: Capture gaps (additive; writers ship first as PR-A, readers last as PR-C)

Capture has no code dependency on the Phase 1-4 read layers, so the **writer halves ship first as PR-A** (Stories 5.1.1, 5.1.2, 5.2.1, and the writer tasks 5.2.2a and 5.2.3a; see "Delivery plan: three PRs") and the soak clock starts at PR-A's deploy. The **reader halves** (5.1.3, 5.2.2b, 5.2.3b) depend on the aggregator and ship as PR-C after PR-B. Phase 5.1 stays revertable on its own: the episode writer is a tx-scoped helper call and the readers tolerate an empty table. No Phase 5 story edits `server/services/backlog_service_triage.go`.

### Epic 5.1: Stuck-episode log (ADR-003)

#### Story 5.1.1: Schema
**As a** developer, **I want** an append-only episode table.
**Acceptance Criteria**:
- Fields and indexes.
  - *Given* `session/ent/schema/backlog_stuck_episode.go` with fields `item_id`, `reason`, `opened_at` (`time.Now().UTC()`), `resolved_at` nullable, an index `(item_id, reason, opened_at)`, and a **partial unique index** on `(item_id, reason)` where `resolved_at IS NULL` (ent `index.Fields("item_id","reason").Unique().Annotations(entsql.IndexWhere("resolved_at IS NULL"))`), *When* ent generate with `--feature sql/upsert` and `go build ./...`, *Then* build passes and only the schema file is committed.
- At most one open episode per pair is a database fact.
  - *Given* a migrated test DB, *When* two open rows for the same `(item_id, reason)` are inserted directly, *Then* the second fails with a unique-constraint error; a resolved row plus a new open row is accepted.
- The DDL is confirmed on a copy of the live DB, not only a fresh test DB.
  - *Given* a `sqlite3 .backup` copy of the live DB, *When* `Schema.Create` runs through `NewEntRepository`, *Then* the partial unique index exists (`sqlite_master` shows `WHERE resolved_at IS NULL`), the existing 717 `backlog_stuck_states` rows are untouched, and the paste goes in the PR.
- If ent cannot express the partial index (Task 0.1.1a records this), fall back to a hand-written `CREATE UNIQUE INDEX ... WHERE resolved_at IS NULL` in the schema's migration hook, and record that in ADR-003.
**Files**: `session/ent/schema/backlog_stuck_episode.go`.

##### Task 5.1.1a: Schema and partial unique index (S, PR-A)

#### Story 5.1.2: Episode writer
**As an** analyst, **I want** episodes recorded on mark/resolve, **so that** recurring stalls are no longer lost.
**Hook points (VERIFIED on `origin/main`)**: `EntRepository.MarkStuck` (`session/ent_repository_backlog.go:2332`) and `EntRepository.ResolveStuck` (`:2403`). `Storage.MarkStuck` / `ResolveStuck` (`session/storage.go:1378/:1384`) are pass-throughs and stay unedited. `MarkStuck` is an upsert plus a conditional reopen UPDATE inside one tx, deliberately with no read-then-write, and the writer pool has `SetMaxOpenConns(1)` so any call on `r.client` while the tx is open deadlocks.
**Acceptance Criteria**:
- The helper takes the tx (deadlock guard).
  - *Given* the helper `openStuckEpisodeTx(ctx, tx *ent.Tx, itemID, reason, openedAt)` and `closeStuckEpisodeTx(ctx, tx *ent.Tx, itemID, reason, resolvedAt)`, *Then* neither accepts or uses `r.client`; a test calls `MarkStuck` and `ResolveStuck` with the writer pool at 1 connection and a 2s timeout and passes (a `r.client` call inside the tx would hang).
- `MarkStuck` yields a created-or-reopened signal without a read-then-write.
  - *Given* the upsert and the conditional reopen UPDATE, *Then* an internal `markStuckTx` returns `stuckTransition{Reopened bool, EpisodeOpened bool}` where `Reopened` is `rowsAffected > 0` of the reopen UPDATE and `EpisodeOpened = (err == nil)` of the ent `Create().OnConflict(DoNothing())` insert into `backlog_stuck_episodes`. Rows-affected is not observable through that call (precedent: `session/ent_repository_backlog.go:634-639`), so `sql.ErrNoRows` and `ent.IsConstraintError(err)` (the partial unique index firing) both mean "already open" and yield `EpisodeOpened=false` without an error; any other error is the logged non-fatal case below. This is the primary path, it needs no `sql/execquery` feature, and it keeps the ent generate command unchanged. The exported `MarkStuck` signature `(applied bool, err error)` is unchanged, so no interface or caller changes.
- Open on first detection and on recurrence.
  - *Given* no row for `(i1, pr_needs_fix)`, *When* `MarkStuck(i1, pr_needs_fix)`, *Then* one open episode exists with `opened_at` equal to the tx's `now` in UTC (the same instant `MarkStuck` writes to `first_detected_at` on create and reopen); *Given* it was resolved and then `MarkStuck` runs again, *Then* a second episode is inserted (2 total, first closed).
- Idempotent re-marks and concurrent marks.
  - *Given* an already-open episode, *When* `MarkStuck` runs again, *Then* no new row is created and `EpisodeOpened=false`.
  - *Given* 8 goroutines calling `MarkStuck(i1, pr_needs_fix)` simultaneously on a fresh item (reconcile tick plus PR poller are both real callers), *Then* exactly 1 open episode exists afterwards and no call returns an error.
- Close on resolve, atomically.
  - *Given* an open episode, *When* `ResolveStuck` runs, *Then* the state UPDATE and the episode close commit in one tx, `resolved_at` is set on the open episode only (older closed episodes untouched), and `ResolveStuck` still returns `n > 0` from the state UPDATE; if the state UPDATE affects 0 rows no episode is touched.
- Never blocks the transition, and drift is bounded.
  - Driver note (one line): continuing after a failed statement inside the tx is safe on SQLite because a failed statement does not abort the tx; on a driver where it does (Postgres) the commit would be poisoned, so a non-SQLite driver would need a savepoint around the episode statement or the error must propagate. The repository is SQLite-only today.
  - *Given* the episode statement fails with a non-constraint error (for example the table is missing), *Then* the stuck-state write still commits, a warning is logged (precedent: `recordStatusEvent`), and the episode is absent. Because both statements share one tx there is no drift window between `backlog_stuck_states` and episodes; the only divergence is this logged-failure case and the pre-capture legacy open rows (7 on the live DB), which get an episode on their next `MarkStuck` refresh (the INSERT-or-ignore opens one with `opened_at = now`, so that episode starts at capture and the pre-capture interval stays covered by the legacy row under the Story 5.1.3 union rule).
- Call sites stay small.
  - *Given* the diff of `session/ent_repository_backlog.go`, *Then* `MarkStuck` gains one helper call plus the `stuckTransition` return from an extracted `markStuckTx`, `ResolveStuck` gains a tx and one helper call, and all episode logic lives in `session/ent_repository_stuck_episode.go`.
**Files**: `session/ent_repository_stuck_episode.go`, `session/ent_repository_stuck_episode_test.go`, `session/ent_repository_backlog.go` (the two functions above).

##### Task 5.1.2a: Episode helpers and partial-index schema fallback (S, PR-A)
- `openStuckEpisodeTx` / `closeStuckEpisodeTx`; the insert is `Create().OnConflict(DoNothing())` and `EpisodeOpened = (err == nil)`, with `sql.ErrNoRows` and constraint errors mapped to "already open" (primary path, not a fallback); unit tests including the concurrent double-mark.

##### Task 5.1.2b: Hook into `markStuckTx` and `ResolveStuck` (M, PR-A)
- Extract `markStuckTx`, add the transition result, wrap `ResolveStuck` in a tx; verify the existing stuck tests are unchanged and green, and run them plus the new episode tests under `go test -race` before the hook merges.

#### Story 5.1.3: Episode reader
**As the** aggregator, **I want** episodes unioned with legacy rows, **so that** stuck history is exact after `capture_since`.
**Acceptance Criteria**:
- Union rule.
  - *Given* `capture_since = 2026-11-10` and an episode on 2026-11-12 plus a legacy single-row interval on 2026-10-02, *Then* the stuck section uses both, `LowerBound` is true only for the pre-capture part, and `FlowMeta.CaptureSince` is set.
- Timestamps are normalized before the union.
  - *Given* a legacy row with a local-time `first_detected_at` (`MarkStuck` writes `time.Now()`) and an episode with a UTC `opened_at`, *Then* the reader converts both to UTC before comparing against `capture_since`.
- Absent table is tolerated.
  - *Given* the episode table is empty, *Then* behavior equals Phase 1-4 (`LowerBound=true`, `CaptureSince` unset).
- Divergence is visible in the product, not only in logs.
  - *Given* 3 open episodes and 4 unresolved `backlog_stuck_states` rows (after the capture date), *Then* the stuck section's Data notes shows a warning "open episodes differ from open stuck rows by 1" and `Estimated` contains `stuck_episode_drift:1`; with equal counts no entry appears. Legacy open rows from before `capture_since` are excluded from the comparison.
**Files**: `session/ent_repository_flow_stuck.go`, `session/flow/stuck.go`, tests.

##### Task 5.1.3a: Reader union (S, PR-C)

### Epic 5.2: PR-fix outcome marker

#### Story 5.2.1: Spike the store
**As a** developer, **I want** to know which existing append-only store can hold a PR-fix steer outcome, **so that** no parallel store is built.
**Acceptance Criteria**:
- *Given* `steerActiveSessionForPRFix` in `server/services/backlog_service_pr_fix_steer.go`, *When* reading the activity-log API (`backlog_activity_notes`) and `post_backlog_update`, *Then* the task records the chosen store as the resolution of the matching Unresolved Questions item, and the choice has a **system-written kind/author discriminator**: either a dedicated column on the notes table that only server code sets (and that `post_backlog_update` always writes as `agent`), or a dedicated marker table. A typed text prefix (`pr_fix_outcome:steered`) inside free-text notes is rejected as the discriminator, because any agent can post a note with that prefix through `post_backlog_update`.
- The spike proves the forgery path is real or closed.
  - *Given* the chosen design, *When* a note is posted through the `post_backlog_update` code path with text starting `pr_fix_outcome:steered` or `pr_fix_trigger:ci`, *Then* the spike records whether the marker reader would count it; the chosen design must answer "ignored".
**Files**: none edited.

##### Task 5.2.1a: Read-only spike (S, PR-A)

#### Story 5.2.2: Capture and read PR-fix outcomes
**As Tyler**, **I want** nudge-to-resolution outcomes, **so that** I can tell if steering works.
**Split**: Task 5.2.2a (writer, PR-A) and Task 5.2.2b (reader and section, PR-C).
**Acceptance Criteria**:
- Outcome written, with a system-written discriminator.
  - *Given* a PR-fix reopen that steers an active session, *When* the steer returns, *Then* one record of kind `pr_fix_outcome` with value `steered` exists for the item with the PR number, written by server code into the store chosen in Story 5.2.1 (kind and author set by the writer, never parsed from note text).
- Agent-posted text cannot forge a marker.
  - *Given* an agent note posted through `post_backlog_update` whose text is `pr_fix_outcome:steered` (and one `pr_fix_trigger:ci`), *When* the reader counts outcomes for the window, *Then* the counts are unchanged and the forged note is ignored; a test posts both and asserts the reader returns the same result as before.
- Bounded writes.
  - *Given* the shared marker writer limiter (Story 5.2.3: per-key interval plus global hourly cap, injected clock), *When* 1,000 steer decisions for one item and PR arrive inside one fake-clock hour, *Then* the number of stored `pr_fix_outcome` records is at most 3 per distinct value.
- Call site is one line per branch.
  - *Given* the diff in `backlog_service_pr_fix_steer.go`, *Then* each branch adds one helper call; no behavior change (existing tests unchanged and green).
- Surfaced.
  - *Given* 5 `steered`, 2 `respawned`, 1 `skipped_active` records in the window, *Then* the PR section shows those counts and "resolved within 24h" count, flagged `estimated: pr_fix_outcome:since_capture`.
**Files**: `server/services/backlog_service_pr_fix_steer.go` (one-line calls), `session/flow/prfix.go`, tests; and `session/flow_marker.go`, `session/ent_repository_flow_markers.go` (the marker writer, limiter and reader; if Story 5.2.1 selects a kind column on `backlog_activity_notes`, the schema edit is in `session/ent/schema/` instead of a new table).

##### Task 5.2.2a: Marker writer, limiter, call sites (S, PR-A)
- Helper with the discriminator, one-line calls per branch, forgery and bounded-write tests. The shared limiter (`session/flow_marker_limiter.go`) is created here and reused by Task 5.2.3a.

##### Task 5.2.2b: Outcome reader and PR-health section counts (S, PR-C)
- Reader that counts only discriminated records; PR section counts and the "resolved within 24h" figure, flagged `estimated: pr_fix_outcome:since_capture`.

#### Story 5.2.3: PR-fix trigger kind (CI vs conflict vs review)
**As Tyler**, **I want** the PR-fix reopen's trigger recorded, **so that** the requirement to separate "PR CI failure" from "merge conflict" is met from the capture date onward.
**Split**: Task 5.2.3a (writer, classifier, dedupe; PR-A) and Task 5.2.3b (reader, section, dwell split; PR-C).
**Where the trigger is captured, and why `backlog_service_triage.go` stays untouched**: the status event's note is the constant `auto-reopened for PR fix (CI/review)` written inside `autoReopenForPRFix` (`server/services/backlog_service_triage.go:2525` on `origin/main`), which receives only a free-text `fixContext`, not the flags. Changing that note would put a diff in `backlog_service_triage.go` and conflict with Story 6.1.2's no-diff rule and the separate backoff item that owns that file. The flags (`CIFailing`, `HasConflicts`, `HasBlockingReviews`) exist only in `session/backlog_lifecycle_pr.go`, so the trigger is recorded there as its own record, **before** it calls the spawner, and the readers join it to the reopen status event by time (below). The reopen note stays a constant; no code in this feature changes it.
**Acceptance Criteria**:
- Trigger recorded at the poller's decision site.
  - *Given* the PR poller decides to spawn or reopen for a fix in `session/backlog_lifecycle_pr.go` (where `prStatus.CIFailing`, `prStatus.HasConflicts`, `prStatus.HasBlockingReviews` and `hasNewFeedback` are already evaluated, `~:1965-1975` on `origin/main`; the call sites into `AutoReopenForPRFix` are `:1571`, `:1625` and the closed-PR branch), *When* the decision is made, *Then* one record of kind `pr_fix_trigger` with value `<ci|conflict|review|feedback|mixed|unknown>` is written through the store and discriminator chosen in Story 5.2.1 (server-written kind, never parsed from note text), carrying the item id and PR number, via a one-line helper call.
- Agent-posted text cannot forge a trigger.
  - *Given* an agent note `pr_fix_trigger:ci` posted through `post_backlog_update`, *Then* the reader's trigger counts are unchanged (ignored).
- Writes are deduped and bounded (thrash loops cannot flood the single-connection writer).
  - *Given* `TriggerRecordMinInterval = 30m` and a per-hour global cap `MarkerMaxWritesPerHour = 600` in the shared limiter (Task 5.2.2a), with an injected clock, *When* the poller reports the same `(item, PR, trigger)` 10,000 times inside one fake-clock hour, *Then* at most 3 records are stored (one at the first decision, then at most one per 30 minutes), and a decision whose trigger value differs from the last recorded one for that `(item, PR)` is always written (a state change, subject only to the global cap).
  - *Given* a thrash-loop fixture shaped like `a90b99cb` (4,325 PR-fix decisions over about 5 days), *Then* the stored trigger records number at most `ceil(duration / 30m)` plus the number of trigger-value changes, and `go test` runs the 10,000-decision case through a real `EntRepository` with `SetMaxOpenConns(1)` and a 2s timeout and passes (the writes neither deadlock nor queue unboundedly).
  - *Given* the global cap is hit, *Then* excess records are dropped (not queued), a warning `flow_marker_dropped` is logged once per minute, and the decision path proceeds unchanged (a dropped marker never blocks or changes PR-fix behavior).
- Classification is table-driven and total.
  - *Given* flags `{CIFailing:true, HasConflicts:false}`, `{false,true}`, `{true,true}`, `{false,false,review:true}`, `{false,false,false,feedback:true}`, *Then* the trigger is `ci`, `conflict`, `mixed`, `review`, `feedback`; two or more flags set together (e.g. CI failing + blocking review, or all four) are `mixed`; `unknown` is reserved for no flag set (e.g. the closed-PR branch) and is counted. An exhaustive 16-combination test asserts exactly this.
- No behavior change.
  - *Given* the diff in `backlog_lifecycle_pr.go`, *Then* it adds one helper call per decision site and no control-flow change; existing tests unchanged and green; and `git diff --stat origin/main -- server/services/backlog_service_triage.go` is empty for PR-A.
- Reopen events join to a trigger by time, forward-only.
  - *Given* a `pr_pending>in_progress` status event with the constant reopen note and trigger records for the same item, *When* the reader joins, *Then* the cycle takes the value of the latest trigger record for that item at or before the event within `TriggerRecordMinInterval + 5m` (35m; the dedupe interval guarantees a constant-trigger loop always has a record that recent), and `unknown` when none exists; a test covers a hit, a miss (older than 35m), a deduped loop event, and two triggers in a row.
- Surfaced and honest.
  - *Given* trigger records exist, *Then* the PR section splits the `PRFix` bucket for events after the capture date and labels the split `estimated: pr_fix_cause:since_capture`; events before the capture date stay `unknown`.
- Dwell splits by trigger, forward-only.
  - *Given* trigger records and PR-fix cycles from Story 1.2.3, *Then* each cycle whose reopen is after the capture date is grouped by the joined trigger (CI-failure dwell and merge-conflict dwell become separately reportable), labeled `estimated: pr_fix_dwell_split:since_capture`; earlier cycles stay in the combined `unknown` dwell. This closes the requirement's CI-vs-conflict dwell ask going forward; it cannot be backfilled.
- The soak date is exposed.
  - *Given* the first trigger record, *Then* `FlowMeta.CaptureSince` reflects the earliest trigger record so comparison windows that straddle it return `incomparable` (Story 1.4.2).
**Files**: `session/backlog_lifecycle_pr.go` (one call per decision site), `session/flow_marker_limiter.go` (from Task 5.2.2a), `session/flow/prfix.go`, `session/flow/prfix_dwell.go`, tests.

##### Task 5.2.3a: Trigger writer, classifier, dedupe and thrash-loop tests (M, PR-A)
- Re-sized S to M in the Final triad patch: classifier, discriminated store write, the dedupe and cap behavior with the `SetMaxOpenConns(1)` thrash test, and forgery test are all in the writer. Depends on the 5.2.1 store decision and the shared limiter from 5.2.2a; the PR-poller flags are read, not changed.

##### Task 5.2.3b: Trigger reader, time join, section and dwell split (S, PR-C)
- Reader with the 35-minute join, the PR-section split, and `prfix_dwell.go` grouping; `CaptureSince` wiring.

---

## Phase 6: Verification and hand-off

### Epic 6.1: Prove it, then ship

#### Story 6.1.1: Manual-instance verification against live-size data
**As Tyler**, **I want** today's investigation reproduced via the tools, **so that** I trust the feature.
**Acceptance Criteria**:
- Numbers reproduce.
  - *Given* a manual instance (`PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test`, binary at `~/.stapler-squad/manual-builds/manual-1/stapler-squad`) loaded with a `.backup` copy of the live DB, *When* calling `get_flow_summary` over the 30-day and all-time windows, *Then* `in_progress>review` is about 636 and `review>in_progress` about 426 (within drift of the live table; recount with `sqlite3 -readonly` the same day), verdict mix is FAIL 186/PARTIAL 82/PASS 150/UNVERIFIABLE 110 (within drift), and thrash is reported per item: the 30-day view lists `a90b99cb` (8,691 events, 17 episodes at the 15-minute gap, 5 after merge; active through 2026-11-06) under anomalies, not in the dwell totals, and the **all-time** view lists the 35-item set including the two largest offenders `d91477d5` (87,435 events) and `8a53ce39` (87,122 events), both archived with last events 2026-10-05 and 2026-10-08, which fall outside a 30-day window starting 2026-10-06. `0791f5b4` has only about 330 events and is a small offender, not one of the "two items with ~175k events" in the requirements.
- Simulation parity.
  - *Given* a throwaway Python re-implementation (outside the repo) of the ADR-002 episode collapse and the ADR-005 judgment-run counting, as used to produce this plan's reference numbers (the 2026-11-05 scripts were not kept), *When* run the same day against the same copy, *Then* the aggregator's all-time episode count (103 at `ThrashMergeGap=0`, 61 at 1h on 2026-11-05), thrash item count (35) and judgment-run item count (31, of which 27 meet a second signal) match within drift; any gap is explained in the PR.
- Deadlock precision is measured before ship (pre-mortem P1 #1).
  - *Given* the top-10 deadlock candidates from `list_flow_deadlock_candidates` on the manual instance, *When* Tyler hand-labels each as `real deadlock`, `reviewer too strict`, `agent gave up`, or `noise` (an agent pre-fills labels from the verdict sequences and the owner confirms or edits them), *Then* the hit rate (`real deadlock` plus `reviewer too strict`, the two outcomes the metric exists to find, over 10) is recorded in the PR; the section ships named "Review-fail streaks (heuristic)" and is promoted to "Deadlock candidates" only if the hit rate is at least 50% (display name only; the `deadlock_candidates` key, the MCP tool name and the `estimated` tag stay either way).
  - *Given* the default list, *Then* it shows 10 rows with the "weak filter" note in both the UI and the MCP response, and `total_count` shows the full 27-31 candidate set.
- Owner comprehension check on the first live render (UX).
  - *Given* the manual instance's first render of `/backlog/flow?window=30` with no prior briefing, *When* Tyler answers the four key questions (Is it healthy? Where does it leak? Why? Which items?) using only the page, *Then* the answers, the time taken and every misreading are pasted in the PR; pass means all four are answered without SQL or docs in under 5 minutes and agree with the known numbers (adjusted FAIL, thrash banner, top rework cause); each misreading is filed as a UX fix item before the PR is marked ready.
- Manual accessibility passes that automation cannot do.
  - *Given* the populated page, *Then* a screen-reader pass (criterion 21), a grayscale screenshot check (criterion 25) and a chart-mark contrast check in both themes (criterion 26) are recorded in the PR.
- The shipped-definition numbers become the explicit checkpoint baseline.
  - *Given* the manual instance, *When* `get_flow_summary` runs for the all-time window and the last 30 days, *Then* the PR records, as the **checkpoint baseline**, the shipped-definition values (not the rough SQL table in `requirements.md`): first-pass yield (zero thrash, first `pr_pending>done` counted, per Story 1.2.2), bounces per done item, and the non-judgment share of non-PASS verdicts, each with N and `drill_key`, plus the delta against the rough table with a one-line reason for each difference (the table is thrash-contaminated). Story 6.1.2c copies this block into the checkpoint item, and the checkpoint compares against it, never against the rough table.
- Verification scope per PR.
  - *Given* PR-B, *Then* every AC of this story runs before PR-B is marked ready; *Given* PR-C, *Then* only the steps that read capture data run on a manual instance with the rows PR-A has accumulated (stuck-drift note, PR-health outcome counts, trigger-split dwell, `CaptureSince`), recorded in PR-C; *Given* PR-A, *Then* its own verification is in the Delivery plan table, not this story.
- Live service untouched.
  - *Given* the live `:8543` unit, *Then* `make install-service` was not run.
**Files**: none (evidence pasted in the PR).

##### Task 6.1.1a: Run and record (S)

#### Story 6.1.2: Follow-up backlog items (NOT part of this feature)
**As Tyler**, **I want** separate tracked items for the missing spawn-failure backoff, for commit capture at review time and for the dated post-ship checkpoint, **so that** this feature only surfaces the loops and the deadlock filter's weak second signal has a filed fix.
**Acceptance Criteria**:
- Item exists, is separate, and is clearly marked.
  - *Given* the repo's backlog, *When* `create_backlog_item` is called, *Then* a new item titled "Fix: per-item backoff after worktree-creation/spawn failure in DequeueNextQueuedItems and AutoReopenForPRFix (follow-up of backlog-flow-observability, NOT bundled)" exists with body naming `server/services/backlog_service_triage.go` (`DequeueNextQueuedItems`, `autoReopenForPRFix`), the evidence (item `0791f5b4` 278 events on 2026-11-05; item `a90b99cb` 4,325 PR-fix flips 2026-10-30..11-04, recount before filing as the live counts have grown to 330 and 8,691 total events respectively; failure `CreateBacklogWorktree: setup: failed to create worktree from branch`), and the acceptance criterion "no item produces more than 6 flips per 15 minutes".
- Commit capture at review time is filed (pre-mortem P1 #1).
  - *Given* the repo's backlog, *When* `create_backlog_item` is called, *Then* a second item titled "Capture commit SHA (and diff basis) at review time so deadlock candidates have a discriminating no-progress signal (follow-up of backlog-flow-observability, NOT bundled)" exists, with a body that states the evidence (`item_sessions.last_commit_sha` is empty for every verdict session; `diff_hash` is non-empty on 140 of 528 verdicts, so the second signal removes only 4 of 31 candidate items; ADR-005) and the acceptance criterion "the verdict row records the reviewed commit SHA, and a flow query can tell whether two consecutive verdicts reviewed the same commit". It is an item, not a prose note in an ADR.
- The backoff item has an owner and gates the checkpoint, with no calendar date.
  - *Given* the backoff item (Task 6.1.2a), *Then* its owner is Tyler's backlog (it is a normal item in the repo's own backlog, picked up by the factory or by Tyler), its body says "gates the post-ship checkpoint of backlog-flow-observability", and it carries no due date; the checkpoint item is blocked by it through `add_backlog_item_dependency`, so the blocker is named (the item id) and not dated.
- A checkpoint item holds the outcome test and a window rule that cannot fail for sample size (Task 6.1.2c).
  - *Given* the backlog, *When* `get_backlog_item` is called on the checkpoint item, *Then* its body carries: the checkpoint start rule, the window-choice and fallback rule, the Story 6.1.1 shipped-definition baseline block, the decision-rule table and the minimum-sample rule from `requirements.md`; it is blocked by the backoff item through `add_backlog_item_dependency`. The start rule is "30 days after the later of the PR-B merge and the backoff item landing"; a calendar date is written into the body only once both blockers have resolved.
- No code in this feature touches those paths.
  - *Given* each of PR-A, PR-B and PR-C, *Then* `git diff --stat origin/main -- server/services/backlog_service_triage.go` is empty (the Story 5.2.3 trigger is recorded in `session/backlog_lifecycle_pr.go` and joined by time, so the constant reopen note is untouched; the backoff item, not this feature, edits `backlog_service_triage.go`).
**Files**: none (MCP `create_backlog_item`).

##### Task 6.1.2a: File the backoff item (XS)
- Owner is Tyler's backlog; no due date; states in its body that it gates the checkpoint (blocker, not a date).

##### Task 6.1.2b: File the commit-capture item (XS)

##### Task 6.1.2c: File the post-ship checkpoint item (XS)
- A third item, titled "Flow checkpoint: run compare_flow_windows around the backlog-flow-observability ship date and record a tuning decision (follow-up of backlog-flow-observability, NOT bundled)", is created via `create_backlog_item` with the due date (ship date plus 30 days) in its body, the baseline table from `requirements.md` ("Outcome baseline"), the decision-rule table, the minimum-sample rule, and the acceptance criterion "a `compare_flow_windows` call anchored on an absolute date is pasted, and a tuning decision (act on a named hypothesis, or explicit no change) is recorded". It depends on the backoff item (Task 6.1.2a) landing first (`add_backlog_item_dependency`) so the first compare is not confounded by the loops stopping; the checkpoint start rule is 30 days after the later of the PR-B merge and the backoff item landing (named blockers, no calendar date until both resolve). **Window choice and fallback (so the first compare cannot be `incomparable` for sample size alone)**: anchor on the shipped tuning change's date T with W = 30 days (A = `[T-W, T)`, B = `[T, T+W)`); if the primary per-item metrics return `incomparable`, widen both windows symmetrically to W = 60, then W = 90 days; if still `incomparable`, fall back to a single-window read: call `get_flow_summary` for B (or the post-change span) and compare its first-pass yield, bounces and non-judgment share to the Story 6.1.1 shipped-definition baseline, quoting raw values and N, no significance claim. A recorded decision that cites an `incomparable` compare plus this fallback read is valid evidence for the meta-assumption; only the absence of any recorded decision falsifies it. The baseline is the Story 6.1.1 block, not the rough table in `requirements.md`. This item is the test of the meta-assumption in `requirements.md` ("flow data will change tuning decisions").

#### Story 6.1.3: Gates
**As a** maintainer, **I want** all CI gates green locally before the PR.
**Acceptance Criteria**:
- *Given* each PR (PR-A, PR-B, PR-C) before it is marked ready, *When* `make ready` runs on that PR's complete diff, *Then* it exits 0 (lint, `ready-complexity-gate` for gocyclo 25 / gocognit 40 / funlen / dupl, `lint:duplicates` for jscpd, registry diff clean); and `git status` shows no committed generated ent or proto output.
**Files**: none.

##### Task 6.1.3a: Run gates and fix findings (S)


---

## Repair log

Repairs made 2026-11-05 to `plan.md`, ADR-001 to ADR-005 and `design/ux.md`; no code touched. Evidence for changed numbers: `sqlite3 -readonly` on the live DB and two throwaway Python simulations (episode collapse, deadlock runs; scripts not kept, to be re-implemented for Story 6.1.1). Code references were checked with `git show origin/main:<path>` (the worktree is 827 commits behind).

### Architecture review

| Finding | Change made |
|---|---|
| Blocker: hotspots missing from Tech Debt Disposition | Added rows for `session/ent_repository_backlog.go` (3,299 lines, 39 commits/90d; Extend as-is, minimal seams), `session/storage.go` (Extend as-is, not edited), `server/dependencies.go` (one construction block), `server/server.go` (two one-line seams), plus `main.go` (one option argument) and `session/backlog_lifecycle_pr.go` (one helper call, Story 5.2.3). Sizes and commit counts re-measured on `origin/main`. Story 0.1.1 now records the hook points. |
| Blocker: Story 5.1.2 episode writer race and deadlock | Story 5.1.2 rewritten: helpers take `*ent.Tx` (1-connection-pool test); open via `INSERT ... ON CONFLICT DO NOTHING` whose rows-affected is the created-or-reopened signal, with an unexported `markStuckTx` returning `stuckTransition{Reopened, EpisodeOpened}` (exported `MarkStuck` signature unchanged); `ResolveStuck` closes the episode in the same tx; Story 5.1.1 adds a partial unique index on open episodes; 8-goroutine double-mark test; exact hook locations named (`ent_repository_backlog.go:2332/:2403`). ADR-003 updated. |
| Concern: no depguard for `session/flow` | New Story 1.1.3 (XS): `isolate_flow_core` strict allow-list in `.golangci.yml` plus a `go list -deps` check. |
| Concern: duplicate status type, hard-coded funnel stages | Glossary reuses `domain.BacklogStatus` (9 built-ins including `refining`); new `StageOrder` from the pipeline definition (`ItemSource.Stages`); unknown statuses go to an `other` bucket; Story 1.2.2, 2.1.4, 4.2.1 and `design/ux.md` S6 updated. Which accessor yields stage order is an Unresolved Question (Task 0.1.1a). |
| Concern: 7-method `Reader` | Split into `EventSource`, `VerdictSource`, `StuckSource`, `ItemSource`; `Aggregator` takes `Sources`; request-review events come from `EventSource` (no separate method). Glossary, Pattern Decisions, Stories 1.3.2, 1.4.2, 2.1.x, ADR-001. |
| Concern: two DTO mappings | MCP tools call `BacklogFlowService` in-process and marshal with `protojson` (one mapper); parity test reduced to a construction check; Stories 3.1.2, 3.2.1, 3.2.3, ADR-001. |
| Concern: aggregator wiring half-specified | Story 3.2.3 rewritten: variadic `CoreOption` / `WithFlowService` on `NewCore`, `NewHTTPHandler` (:120) and `RunServer` (:156; the review called it `RunStdio`, the real name is `RunServer`); construction site in `server/dependencies.go`; HTTP path shares the RPC service; stdio fallback builds its own from `storage` (decision, tested); 7 test call sites untouched. |
| Concern: `Span` allows illegal states | `Span` is a sealed interface with `RealSpan` / `ThrashSpan` and validating constructors; `SpanKind` removed; Story 1.1.1 ACs and a handled-implementations test. |
| Concern: build-vs-buy deviation unrecorded | Pattern Decisions row plus an ADR-004 "Departure from build-vs-buy" section tied to the rollup revisit trigger. |
| Concern: sanitization in the reader | Story 2.1.4 returns raw titles and asserts the reader does not reference `SanitizeForAgentContext`; sanitization happens once in the MCP adapter (Story 3.2.1). |
| Concern: unbounded cache key | Story 1.4.3: key from `WindowSpec` (not clock timestamps), 64-entry LRU, non-default `Config` / item-filtered queries bypass the cache; ADR-004 item 5. |
| Nitpick: events vs flips unit | "Events" used everywhere (glossary, ADR-002, tests). |
| Nitpick: Story 5.2.2 files | New-store files listed only "if Story 5.2.1 selects a new store". |
| Nitpick: `DrillKey` closure | Glossary: `TestEveryMetricHasDrillKey` closes the set. |
| Nitpick: `Seq` leaks storage detail | Glossary documents it as a stable ordering key only. |
| Nitpick: effort band | No action (already acknowledged in the plan). |

### Adversarial review

| Finding | Change made |
|---|---|
| C1: thrash fragments, biggest-offender claim wrong | Verified by simulation: 103 episodes across 35 of 160 items at the 15-minute gap (`0791f5b4` 2, `a90b99cb` 17, `d91477d5` 18, `8a53ce39` 17); 61 after a 1h merge. Added `ThrashMergeGap` (1h) and a merge pass; the anomaly unit is now the item (episodes, total events), the banner counts items with singular copy. Fixtures and ACs rewritten (`thrash-a` = two runs, `thrash-b` = 17 episodes merging to 5). The "~175k events" items are `d91477d5` (87,435) and `8a53ce39` (87,122), both archived; their last events are 2026-10-05 and 2026-10-08 (the review said both 10-05; live data says 8a53ce39 ran to 10-08). Story 6.1.1 now checks the all-time view. |
| C2: collapse on window-truncated events | `CollapsePad = ThrashGap x ThrashMinFlips + ThrashMergeGap` (derived, 2h30m); reader fetches from `Start - CollapsePad`; collapse then clip; ACs for straddling episodes and a negative test; the pad's sufficiency is argued in Story 1.2.1; ADR-002 and ADR-004. |
| C3: FPY passes thrash-only items | FPY now requires zero thrash episodes (any cause) before first merge; `thrash_affected` reported; AC added (Story 1.2.2, Glossary). |
| C4: three different non-judgment counts | 132 (98 + 34) of 378 used everywhere (plan, ADR-005, UX); live recount also found 18 reviewer-degraded UNVERIFIABLEs (7 no-diff, 4 degraded, 7 no-acceptance-criteria; the review estimated about 11) and added a `ReviewerDegraded` class (150 of 378 total). Story 4.2.2's "102" removed. |
| C5: impossible Story 2.1.3 AC | AC changed to 3 sessions with 1 verdict each plus a session with no verdict; the per-session step is described as a join-safety guard, not a dedup (Story 1.3.1, ADR-005). Verified: 528 verdicts, 528 distinct sessions. |
| C6: deadlock signal does not discriminate | Redefined (ADR-005, Story 1.3.2): run of at least 3 judgment non-PASS verdicts plus either the same failing criterion in 3 consecutive verdicts or an unchanged non-empty `diff_hash`; `agent_claimed_done` is informational. Measured on live data: 31 items with such a run, 27 with a second signal, so the second signal removes only 4 of 31; that weakness is stated in the ADR, ACs and Data notes rather than hidden. |
| C7a: verdict mix over time has no producer or consumer | `VerdictMixSeries` (Story 1.3.1b), weekly `trends` section (Stories 1.4.2, 3.1.1), new Story 4.2.5 with the `FlowChart` consumers (verdict mix, throughput and rework sparklines); `FlowChart` story states its consumers. |
| C7b: CI vs conflict split dropped | Kept as an explicit owner-visible gap (Unresolved Questions, `estimated: pr_fix_cause:unavailable`, UX S9) and added Story 5.2.3 (S): record the trigger at the reopen site in `session/backlog_lifecycle_pr.go`, where `CIFailing` / `HasConflicts` / `HasBlockingReviews` are already computed; no backfill. |
| C8: episode writer under-specified | Covered by the architecture blocker above; additionally "helper takes tx" is an explicit AC, the "exactly one helper call" AC is replaced (insert-if-none in `MarkStuck`, a tx-wrapped close in `ResolveStuck`), and the file reference now names `session/ent_repository_backlog.go`. |
| C9: effort estimate | Honest recount: 50 tasks (19 M, 26 S, 5 XS), 4,000k raw, 10,494k CU (about 10.5M; was 44 tasks and 10,203k). Re-sized down: 0.1.2, 2.1.2, 3.2.1, 3.2.2 (M to S) and 2.1.5 (S to XS), against the measured 10k-row 30-day window and in-process `protojson` reuse. Re-sized up or added: 4.2.3 and 5.1.2 split, 1.3.1 gains a series task, new 1.1.3, 4.2.5, 5.2.3. Net +290k CU. The reader and perf tasks were not removed (padded reads, bulk seed and pool-routing assertions remain; the perf harness is the only evidence for the 1s SLO). |
| C10: mixed-offset machinery over-implied | Verified (10,654 `-07:00` rows, 2026-08-06 to 2026-08-17 11:56 local, zero later). Scoped in ADR-004, the Tech Debt row and Task 1.2.1a to one ordering test and one prefilter test; Story 0.1.2 may now conclude "skip the index" and Story 2.1.1 is marked conditional. |
| Minor: `dbsplit` routing, text-order seed, mixed local/UTC stuck timestamps, 3.1.2 depends on 2.1.5 | Plain-`SELECT`-only rule and a pool-label assertion added to Task 2.1.2a; seed picks the latest by parsed time in Go (ADR-004 item 4); episode `opened_at` uses UTC; the Dependency Visualization shows 2.1.5 as a gate in parallel with 3.1.2. Not separately changed: legacy `first_detected_at` (local time) is still normalized in the reader, as the review suggested. |
| Minor: fixture counts drift | Fixtures assert shapes, not live totals; the generator recounts first (Task 0.1.3a lists the 2026-11-05 values). |

### UX (`design/ux.md`)

Funnel uses `stage_order` and an "Other stages" row (S6); banner counts items with singular copy (S4); anomalies table is one row per item with episodes and total events and refreshed mock data (S11); deadlock table gains run-length, same-criterion and diff-unchanged columns (S8); verdict mix and sparklines are named `FlowChart` consumers of Story 4.2.5 (S5, S7, S8); PR health explains the pre-capture single bucket and the post-capture split (S9); inventory story references and assumptions updated.

### Findings not fixed, or fixed differently

- **Architecture review's `RunStdio`**: the function is `RunServer`; fixed under the real name.
- **"Episodes fragment: add merging or relabel"**: both done (merge at 1h plus item-level rows). Merging does not make episodes whole (live data still has 5 and 8 episodes after the 1h merge for the largest items), which is why the item is the displayed unit.
- **Deadlock discrimination is only partly fixed**: no live signal separates candidates well (the commit SHA column is empty for all verdict sessions and `diff_hash` is empty for 388 of 528 verdicts). The plan labels the weakness instead of claiming a fix.
- **95.7% "agent claimed done" figure** is cited from `requirements.md` and was not re-derived here.
- **`ux.md` Total: 37 criteria** and the surface count (14 interactive) were left as they are: the repair added no surface, only content to existing ones (S4, S5, S6, S7, S8, S9, S11); no new UX acceptance criterion was written for trends beyond Story 4.2.5's ACs.
- **ent partial-index support and the stage-order accessor** are UNVERIFIED and recorded as Task 0.1.1a checks with fallbacks, not asserted.

### Phase 4 patch (2026-11-05)

Patched against `implementation/consistency-review.md` (6 blockers, 12 concerns, 5 nitpicks), `implementation/pre-mortem.md` (P1 #1, #2 with their stated prevention; P2 #3, #4, #5 mitigations), the `architecture-review.md` re-review (2 concerns plus the `session/ent_repository.go` line) and the `validation.md` notes. Counts were re-read with `sqlite3 -readonly` on the live workspace DB and a throwaway Python simulation (`/tmp/flowsim/sim.py`, not kept); live counts drift, so every figure below is dated 2026-11-05. The `origin/main` SHA for Story 0.1.1 is not known yet and is recorded when that story runs.

**Authoritative counts (one set, used in requirements, plan, ADRs, UX)**: `backlog_status_events` 257,988 (238,553 `ready<->in_progress`); verdicts 528 = FAIL 186 + PARTIAL 82 + PASS 150 + UNVERIFIABLE 110 (5 overrides); 378 non-PASS; non-judgments 98 + 34 = 132, reviewer-degraded 18, so 150 of 378; 23 auto-verified PASS; adjusted FAIL 88/355 = 24.8% (355 = 528 - 98 - 34 - 18 - 23), raw FAIL 35.2%, raw UNVERIFIABLE 20.8%; `in_progress>review` 636, `review>in_progress` 426 (398 `ReviewReopen` note, 28 other; 93 items), `pr_pending>in_progress` 4,437 (4,341 inside thrash episodes, 96 real reopens on 43 items), `pr_pending>done` 106 (105 items), `done>archived` 160; items 161 (archived 153, done 1, idea 2, ready 3, review 2). Thrash, all causes: 103 episodes on 35 of 160 items-with-events at 15m, 61 after the 1h merge = 44 `ready<->in_progress` + 10 `queued<->in_progress` + 7 `pr_pending<->in_progress` (3 items). So the PR-fix thrash count is 7, not 61.

| Finding | Change made |
|---|---|
| CR B1 size band | `requirements.md` Size is Large, about 11.6M CU per this plan (scope kept, not a downscale). The plan's own total moved from 10.5M to 11.6M in this patch (see the effort table); requirements follow the plan. |
| CR B2 deadlock definition | `requirements.md` success metric (d) and the In Scope bullet follow ADR-005 (judgment run of 3 plus a no-progress signal; `agent_claimed_done` informational). |
| CR B3 / B4 CI vs conflict rework cause and dwell | Recorded in `requirements.md` (Known gap, PR-health bullet) and the Unresolved Questions gap item; new Story 1.2.3 (combined PR-fix dwell, `pr_fix_dwell:combined`); Story 5.2.3 is in scope (owner direction), runs first in Phase 5, and splits count and dwell by trigger forward-only (`pr_fix_dwell_split:since_capture`); Story 4.2.3 shows combined dwell; the requirement gap (no CI-vs-conflict dwell before the capture date) is stated, not hidden. |
| CR B5 adjusted FAIL in UX | UX S5 and S8 now show adjusted 88/355 (24.8%) with 98 + 34 excluded and 18 degraded plus 23 auto-verified also excluded; Story 1.3.1, 4.2.2, ADR-005 carry the same figures. |
| CR B6 verdict totals | 528 / 150 / 110 everywhere; UX sums to 528 (186+82+150+110) and `rows=` is 528; requirements refreshed. |
| CR C1 | Rabbit-holes bullet no longer lists `analytics.db`/`stage_transitions` as reuse targets. |
| CR C2 | Requirements evidence refreshed (257,988 rows, named `d91477d5`/`8a53ce39`, `0791f5b4` as a small offender in Open Question 3). |
| CR C3 | "61 episodes" relabeled all-cause (Story 1.2.1, ADR-002); Stories 4.2.2/4.2.3 and UX S6/S7 use 7 thrash episodes plus 96 real reopens for PR-fix. |
| CR C4 | UX S7 mock now 398 across 93 items (consistent with the 105-item cohort fixture); UX section 7 says mock values are illustrations. |
| CR C5 | One spelling `deadlock_candidates:heuristic` (UX S15, ADR-005); full `Estimated[]` vocabulary listed in the `FlowMeta` glossary row. |
| CR C6 | One section-status enum `OK, Partial, Failed, Skipped, Empty` (wire strings lowercase) in the glossary, UX S3/S15, Story 3.1.1; timeout is `Failed` with text. |
| CR C7 | Casing decided: snake_case via `protojson` `UseProtoNames: true, EmitUnpopulated: true`, one `FlowJSONOptions()` helper shared by the MCP adapter and the parity test (Stories 3.1.1, 3.2.1, 3.2.3; UX S15). |
| CR C8 | All-time input defined: `window_days = 0` / `WindowSpec{AllTime: true}` / `NewAllTimeWindow`, UI `?window=all` as a fourth radio (Stories 1.1.1, 3.1.1, 3.2.1, 4.1.2; UX S2). |
| CR C9 | Glossary `ReworkCause`: `ReviewFail` = `ReviewReopen`-noted real `review>in_progress` (398), `Bounce` = the rest (28); totals 426 feed FPY, bounce distribution and the back-edge; `bouncing` stuck reason is separate. AC in Story 1.2.2. The 398/28 split is from a live note count; Story 0.1.1a's literal grep covers drift. |
| CR C10 | `mixed` added to the glossary `PRFixTrigger` list, Story 4.2.3 and UX S9. |
| CR C11 | Story 1.2.2 gains `LeadTime` (`tiles.lead_time`) behind the UX tile; absorbed in the existing M task. |
| CR C12 | UX S16 uses the plan's log line (`window`, `section`, `rows`, `query_ms`, `cache`); `status=failed` only on the warn path. |
| CR N1 | Story 4.1.2 gained ACs for window/compare radiogroups, Refresh, live region, jump list, Widen, unsupported window, `unavailable` and the error-boundary link (S2, S3, S1); story re-sized S to M. |
| CR N2 | UX S9 row lists Story 5.2.2 and 1.2.3; Story 4.2.3 has the "not yet recording" no-numbers AC. |
| CR N3 | New Story 4.2.6 (item timeline dialog, `GetItemFlowTimeline`) for S11 "View events". |
| CR N4 | UX S15 names the six MCP tools next to the six RPCs; the asymmetric naming is left as is (one-to-one mapping). |
| CR N5 | ADR-003 uses `capture_since` / `FlowMeta.CaptureSince`. |
| PM P1 #1 deadlock precision | Story 6.1.1: hand-labeled top-10 check with a 50% hit-rate rule and relabel to "review-fail streaks"; Stories 1.3.2, 3.2.2, 4.2.2: top-10 default, ranked by run length, `total_count`, visible "weak filter" note in UI and MCP; Story 6.1.2 second item for commit SHA capture at review time (a filed item, not a prose note); ADR-005 updated. |
| PM P1 #2 writer stall | Story 2.1.2: reader-pool label test is a merge precondition (and cancellation reaches the driver); new Story 2.1.6: `MaxWindowRows` 60k chunk by item batch, `HardRowCeiling` 1M refuse with `ErrWindowTooLarge`, per-batch timeout; Story 1.4.3: one uncached compute in flight; Story 2.1.5: concurrent-writer AC (CI-resident barrier test plus env-gated 100ms check on the live-size copy); ADR-004 item 6. |
| PM P2 #3 base drift | Story 0.1.1 AC: SHA pinned and per-wave `rev-list` check on serialization-point files; seam-file edits as one early short-lived PR; Phases 1-4 merge before Phase 5 (effort section, Phase 5 intro). |
| PM P2 #4 stuck write path | Story 5.1.3 drift AC (`stuck_episode_drift:n`); Task 5.1.2b `-race`; Story 5.1.1 DDL confirmed on a live-DB copy; 5.1.x ships as its own PR (Phase 5 intro). |
| PM P2 #5 headline gaps and parity | Owner decision recorded in Unresolved Questions; 5.2.3 first in Phase 5; Story 1.4.2 returns `incomparable` for metrics straddling `capture_since`; Story 3.2.x/3.2.3 clamp reports `total_count`/`truncated`/`has_more`; parity test covers non-clean titles and an over-limit fixture. |
| AR re-review 1 (`EpisodeOpened`) | Story 5.1.2, Task 5.1.2a, ADR-003: `EpisodeOpened = (err == nil)`, `sql.ErrNoRows` and constraint error mean "already open", primary path. |
| AR re-review 2 (non-constraint error in tx) | One-line driver note in Story 5.1.2 and ADR-003 (SQLite safe; Postgres needs a savepoint). |
| AR re-review line (`session/ent_repository.go`) | Tech Debt Disposition row: not edited, reference only (pool constructor, `:203`). `architecture-review.md` itself was not edited (outside this patch's file set). |
| VAL note 1 (Story 1.4.2 Files) | `compare_test.go`, `drilldown_test.go`, `timeline_test.go` added to Story 1.4.2 Files; `validation.md` note updated. |
| VAL (mapping completeness and counts) | 30 rows added (91 to 121, recounted with awk); effort table recounted: 47 stories, 55 tasks (21 M, 29 S, 5 XS), 4,425k raw, 11,609k CU. |

**Not fixed in this patch**: (1) pre-mortem P3 items (injection allow-list descriptor test, `unclassified > 10%` amber warning, 30-minute-cadence thrash AC) were not requested and are not added. (2) `validation.md` still assumes `session/backlog_lifecycle_pr_test.go` as the Story 5.2.3 test file; the real file name is confirmed only after Story 0.1.1. (3) The 95.7% "agent claimed done" figure is still cited, not re-derived. (4) Story 4.2.2's earlier "Review FAIL 425" in the pre-patch plan text is superseded by 398/426, but the earlier repair-log rows above are left as the historical record.

### Triad repair 1 (2026-11-05)

Patched against the three triad lens analyses (product, UX, engineering; the engineering lens was not part of this repair request). No code touched; edits only under `project_plans/backlog-flow-observability/`. Evidence: baselines were re-derived with `sqlite3 -readonly` on the live workspace DB (counts drift; see `requirements.md` "Outcome baseline").

| Finding | Change made |
|---|---|
| Product: no measured outcome baseline | `requirements.md` gains an "Outcome baseline" table: 48 of 137 done items with zero review rework (35.0%; 29.2% if PR-fix reopens count, thrash-contaminated floor), median 2 bounces per done item (3 among the 89 that bounced), non-judgment share 150 of 378 non-PASS (39.7%; only the 98 "no committed changes" was re-run today). Manual nudges per week is recorded as UNMEASURED (no source). Story 6.1.1 AC re-derives the table. |
| Product: no outcome metric or post-ship checkpoint | `requirements.md` Success Metrics: outcome metric plus a dated checkpoint (ship + 30 days); new Task 6.1.2c files the checkpoint item (due date, baseline, decision rules, blocked by the backoff item) and Story 6.1.2 has an AC for it. |
| Product: meta-assumption unnamed | `requirements.md` "Risky Assumptions" names "flow data will change tuning decisions" with its test (the checkpoint item holds a recorded decision) and falsifier; deadlock heuristic and sample size listed with their tests. |
| Product: no minimum sample for compares | Rule in `requirements.md` (20 items, 30 verdicts); Story 1.4.2 ACs (`incomparable` with reason, raw values kept, 19/20 boundary, reflection test for significance-like fields); UX S5 and Story 4.2.1 render it; validation rows added. Live evidence: 26 merged items in the last 30 days vs 8 in the 30 before (approximate, string-ordered timestamps), so a naive 30-vs-30 FPY compare is `incomparable` today. |
| Product: no decision rules, no ordering | `requirements.md` decision-rule table (7 rows; thresholds are starting guesses, INFERRED) and an ordering section (thrash backoff fix first, first compare after it, commit capture next). |
| UX: drawer, FlowChart, S4, S12, S7 aria-sort ACs missing | Story 4.2.4 (drawer states, bad key, close removes param), Story 4.1.3 (chart states), Story 4.2.3 (anomaly-unavailable, methodology-unavailable), Story 4.2.2 (aria-sort); UX criteria 38 to 41 added. |
| UX: stale rule undefined | `FLOW_POLL_INTERVAL_MS = 60000`; stale after 121s since the last successful client fetch (not `as_of`, because of clock skew and the 30s server TTL); AC in Story 4.1.2, UX S3 and criterion 42. |
| UX: 21/26/27/30/33 unverified, e2e thin | Story 4.3.2 gains ACs for click counts (criteria 1 to 5), error/stale/empty/back-restore flows and criteria 21/26/27/30/33; split into Tasks 4.3.2a and 4.3.2b; Story 6.1.1 adds manual screen-reader, grayscale and chart-mark contrast passes; Story 4.3.1 fixture list extended; `validation.md` mapping updated (44 criteria, 135 mapping rows). |
| UX: deadlock overclaim | Default display name "Review-fail streaks (heuristic)", agent-claimed-done column hidden by default; "Deadlock candidates" only if the Story 6.1.1 hit rate is at least 50% (ADR-005, requirements, Stories 4.2.2 and 6.1.1, UX S8). Key, MCP tool name and `estimated` tag unchanged. |
| UX: misleading headline tile | Adjusted FAIL (88/355) is the primary tile value, raw 186/528 secondary (UX S5, S8, Stories 4.2.1, 4.2.2, criterion 35). |
| UX: thrash messaging conflict | Funnel back-edge shows the real count primary and the excluded raw events in the label; banner counts archived items and says how many ("35 items thrashing (2 archived)"), with an archived chip on rows (UX S4, S6, S11, Stories 4.2.1, 4.2.3). |
| UX: "Retry section" | Relabeled "Reload" (UX S3, Story 4.1.2, criterion 9). The anomaly-unavailable banner also says Reload. |
| UX: no owner comprehension check | Story 6.1.1 AC and UX criterion 44: the four key questions answered from the first live render in under 5 minutes, misreadings filed as fix items. |
| UX: density | Collapsed-by-default rule for Stuck and Anomalies with expand-on-jump, expand-on-failure and a Jest plus e2e test (Story 4.1.2, UX criterion 43). Whether criterion 1 actually holds at 1080p is still unmeasured until the e2e runs. |
| MCP: absolute date anchors | `compare_flow_windows` accepts `a_start/a_end/b_start/b_end` (`YYYY-MM-DD` or RFC3339) alongside offsets; proto, Stories 1.4.2, 3.1.1, 3.2.1. |
| MCP: `all_time` | Explicit boolean at the RPC and MCP surfaces; `window_days=0` now errors with "use all_time=true". Updated in the glossary, Stories 1.1.1, 3.1.1, 3.2.1, 4.1.2, UX S2, ADR-002 and validation. Earlier repair-log rows that say `window_days = 0` are history. |
| MCP: drill keys | JSON-schema enum generated from the registry, test that they match, invalid-key error lists valid keys (Stories 3.1.1, 3.2.1). |
| MCP: tool descriptions, legend, size | AC for call order starting at `get_flow_summary`, an example payload per tool, `as_of` cache note, `estimated_legend` in `FlowMeta`; `sections[]` and `compact` request fields; 32 KB default and 8 KB compact budgets enforced by a test. The budgets are initial values, not measured. |
| Effort | Recounted by awk: 57 tasks, 47 stories (22 M, 29 S, 6 XS), 4,575k raw, 12,003k CU (see the effort section). |

**Not fixed in Triad repair 1**: (1) Manual-nudge frequency has no data source, so no baseline exists; recorded as UNMEASURED. (2) The engineering lens gaps (typed safe-string enforcement for the MCP sanitizer, event retention, interim mitigation for the running thrash loops beyond the ordering rule) were outside this request and are unchanged. (3) The decision-rule thresholds, 20/30 minimum sample, 60s poll interval and 32/8 KB budgets are chosen defaults, not measured values; each has a test or a 6.1.1 step that would expose a bad choice. (4) The 34 self-check and 18 reviewer-degraded counts in the baseline were not re-run today.

### Final triad patch (2026-11-05)

Patched against the round 2 triad lens analyses (engineering, UX, product), real defects only. No code touched; edits only under `project_plans/backlog-flow-observability/`. Totals are recounted by awk at the end of the patch: 60 tasks, 47 stories (22 M, 32 S, 6 XS), 4,740k raw, 12,438k CU; `validation.md` 142 mapping rows and 46 UX criteria.

| Finding | Change made |
|---|---|
| Eng 1: critical path understated | The critical-path bullet now lists the serial chain explicitly (19 sequential runs in PR-B, about 22 with PR-C) and says why "about 10 waves" hid the serial chains. |
| Eng 2: soak clock hollow; gate-order contradiction | New "Delivery plan: three PRs" section: PR-A (capture writers: 0.1.1, 5.2.1, 5.1.1, 5.1.2, 5.2.2a, 5.2.3a) ships first; PR-B (Phases 0-4); PR-C (capture readers 5.1.3, 5.2.2b, 5.2.3b). Each PR has its own verification point; Risk Control, Phase 5 intro, dependency graph and effort table updated; Stories 5.2.2 and 5.2.3 split into writer and reader tasks. |
| Eng 3: proto before final shapes | Story 1.4.2 gains the result-type freeze AC (`session/flow/summary_types.go`, first commit of Task 1.4.2a); Story 3.1.1 states it starts after that commit and mirrors the frozen types. |
| Eng 4: forgeable markers | Story 5.2.1 requires a system-written kind/author discriminator (kind column or dedicated table), rejects the text prefix, and records whether the `post_backlog_update` path can forge it; Stories 5.2.2 and 5.2.3 gain an AC that an agent-posted note with the prefix is ignored; validation row updated (the old row assumed a typed prefix). |
| Eng 5: unbounded writes | Story 5.2.3 gains the dedupe and cap ACs (30m per-key interval, state change always written, 600/hour global cap, injected clock, 10,000-decision case through a `SetMaxOpenConns(1)` repository with a 2s timeout, drop-and-warn on cap); the shared limiter is created in Task 5.2.2a and also bounds outcome records. |
| Eng 6: sanitization covers two fields | Story 3.2.1 now sanitizes every string leaf by reflection walk except an explicit allowlist, with a descriptor-walk test that fails on any unlisted string field and a fixture that injects into every reachable string field; the parity AC (3.2.3) updated. |
| Eng 7: 6.1.2 no-diff vs 5.2.3 | Resolved explicitly in Story 5.2.3: the trigger is recorded in `session/backlog_lifecycle_pr.go` (flags live only there; the constant reopen note at `backlog_service_triage.go:2525` receives only a free-text `fixContext`) as its own record and joined to the reopen status event by time (latest trigger within 35m, else `unknown`); the reopen note is unchanged, and the no-diff check applies to PR-A, PR-B and PR-C. Verified on `origin/main`: the note is at `backlog_service_triage.go:2525` and the flag sites are in `backlog_lifecycle_pr.go` (`:1430` `CIFailing`; spawner calls at `:1571` and `:1625`). |
| Eng 8: sizing, fallback, budgets | Task 5.2.3a re-sized S to M and the reader half split to 5.2.3b (S); new conditional Task 0.1.2b (S) gives the reader-pool fallback a concrete design and cost; the 32 KB and 8 KB budgets are replaced by a measured-fixture gate (`constant = ceil(measured x 1.25)`, asserted `size <= constant <= size_now x 1.5`); `validation.md` row and `design/ux.md` text updated. |
| Eng 9: stale residue | Removed the superseded 55/47 and 50/44 sums from the effort section (history sentence kept without the stale figures); `pre-mortem.md` row 3 no longer states "50 tasks, about 10.5M CU" as current; `validation.md` replaced the pre-repair 121-row count with a history sentence and a current recount; all current totals recounted by awk. Earlier repair-log rows are left as the historical record. |
| UX: all-time over the row ceiling | Story 4.1.2 AC and UX S3 row and criterion 45: a `window-too-large` state with "Pick a shorter window", 7d/30d/90d buttons, no Retry; Jest and e2e row in `validation.md`. |
| UX: Compare with All time | Story 1.4.2 `ErrCompareAllTime`, Story 3.1.1 (no `all_time` on the compare request), Story 4.1.2 AC and UX S2 text and criterion 46: compare is `aria-disabled` with a hint, resets to `none`, and a `window=all&compare=prev` URL is normalized. |
| UX: `drill_key` beside every number | Story 3.1.1 proto AC (descriptor-walk test) and Story 3.2.1 AC (JSON walk test, keys must be in `flow.DrillKeys()`). |
| UX: half-open wording | Story 3.1.1 field comments, Story 3.2.1: the description says "end is exclusive", the example is annotated with the covered days, and a boundary test shows an event at the shared bound counts in B only. |
| Product: checkpoint can fail on sample size | `requirements.md` and Story 6.1.2c define the window choice (T-anchored 30-day windows, widen to 60 then 90, then single-window read against the baseline) and state that an `incomparable` result with the fallback read is valid evidence; only no recorded decision falsifies the meta-assumption. |
| Product: baseline definitions | Story 6.1.1 gains an AC that records the shipped-definition numbers (with N and drill keys) as the explicit checkpoint baseline, and the checkpoint compares against it, not the rough SQL table. |
| Product: backoff item owner and gate | Story 6.1.2 and Task 6.1.2a: owner is Tyler's backlog, no due date, it gates the checkpoint (blocker named by item id); the checkpoint starts 30 days after the later of the PR-B merge and the backoff landing; `requirements.md` Ordering and checkpoint text updated. |

**Deferred polish (listed, not fixed, by instruction)**: tab-stop count for "every number a button" and the 24px-target exemption for inline links; the "Review FAIL (judg.)" tile label jargon and the three fail-ish percentages in the first row; sparkline behavior with only 1-2 weekly buckets on the 7-day window; time-zone label on the as-of time and UTC-midnight MCP dates; dimmed-previous-data and slow all-time loading copy during window changes; criterion 1 asserting the whole funnel in the viewport and a test for the "better/worse" delta wording; adoption telemetry (MCP call counts) and a consultation cadence.

### Owner decisions (2026-11-05, interview)

These override any conflicting text above (the 50% figure appears in Stories 4.2.2 and 6.1.1, ADR-005, requirements and UX S8; read it as 70%).

- **Bounce definition**: keep the plan default (real `review>in_progress` events without the `ReviewReopen` note).
- **Deadlock precision bar**: 70%, not 50%. Tyler hand-labels about 10 candidates; below 70% the section keeps the name "Review-fail streaks (heuristic)".
- **Thrash-backoff fix**: a parallel, separate PR (not gating PR-B). The ship+30d checkpoint still starts after the later of the PR-B merge and the backoff landing.
- **PR strategy**: three PRs, A (capture writers) then B then C, as in "Delivery plan: three PRs".
- **PR-fix trigger classifier, `mixed` bucket (owner decision 2026-11-06, deviation from the earlier table)**: two or more trigger flags together classify as `mixed` instead of `unknown`; `unknown` now means no flag set or an unrecoverable case. Story 5.2.3's classification AC and `validation.md` updated; `session/flow_marker.go` `ClassifyPRFixTrigger` and `TestPrFixTrigger_should_ClassifyTotalTable_When_FlagCombinations` changed test-first.

**Not fixed, with reasons**: (1) the outcome metric still has no numeric target, because the baseline sample is small (26 vs 8 merged items per 30 days) and a threshold would be invented; the decision-rule table stays the contract. (2) Manual nudges per week stays UNMEASURED (no data source). (3) The first window's CI-vs-conflict split stays unmet until the soak accrues; PR-A starts the clock earlier, which is the only mitigation available. (4) The 600/hour cap, 30m interval and 35m join window are chosen defaults, not measured values; each has a test that exposes a bad choice.
