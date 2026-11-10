# ADR-004: Read path, timestamp normalization, and the created_at index

**Status**: Accepted, with one spike gate (Story 0.1.2) **Date**: 2026-11-05

## Context
Research was done on a worktree 827 commits behind `origin/main`. It found `SetMaxOpenConns(1)` (`session/ent_repository.go:164` on this branch) and concluded long reads would block writers. On `origin/main`, `NewEntRepository` opens on-disk databases through `session/dbpool` (ADR-036 in `docs/adr/ADR-036-database-pool-registry.md`) with a separate `query_only` reader pool, and plain SELECTs are routed to it. `created_at` is stored as text with mixed `+00:00` and `-07:00` offsets (247,333 vs 10,654 rows), so lexicographic comparison is wrong across the boundary.

## Decision
1. **Read path**: `session.EntFlowReader` reads through the repository's existing ent client so queries land on the dbpool reader pool, with short single-statement reads, a context timeout (default 5s), column projection (`item_id, from_status, to_status, created_at, note`), and no N+1 (`item_id IN (...)` bulk fetches). Do not open a second `mode=ro` handle: dbpool already is that, and `norawdb` lint forbids raw opens. Story 0.1.2 verifies routing (pool label via `session/dbobs`/`/actuator/db`), and the result is a hard precondition of Story 2.1.2: a test must assert the `reader` pool label or the story does not merge (revised after the pre-mortem; a writer-routed all-time read would stall the live factory on the single writer connection). Correctness does not depend on the pool, but safety does.
2. **Normalization**: window filtering and ordering happen in Go on parsed `time.Time`. SQL applies only a conservative superset prefilter: `created_at >= <(windowStart minus CollapsePad) minus 15h formatted in the stored layout>`, valid because stored text carries local wall-clock time and offsets are within +/-14h. `CollapsePad` (2h30m by default, ADR-002) is subtracted so `Collapse` sees events before the window start. **Scope (corrected after adversarial review):** only 10,654 rows before 2026-08-17 12:00 local carry `-07:00` (VERIFIED with `sqlite3 -readonly`: min 2026-08-06, max 2026-08-17 11:56 local; every later row is `+00:00`), so a 30-day window never sees a mixed-offset row. Keep the prefilter, the `(At, Seq)` ordering and the Go normalization because all-time windows need them and they are correct, but test the mixed-offset path once (ordering) and once (prefilter superset), no more. Order per item by `(parsed time, rowid)`; drop self-loops (done>done 2, in_progress>in_progress 6, review>review 2) and count them as data-quality warnings; clamp negative dwell to 0 and count it.
3. **Index (conditional)**: the 30-day window returns about 10,100 of 257,988 rows (`created_at >= '2026-10-06'`, about 4%), and the existing `(item_id, created_at)` index serves the seed query, so the `created_at` index may be unnecessary. Story 0.1.2 measures the padded 30-day read with and without `index.Fields("created_at")` and may conclude **skip the index** (Story 2.1.1 is then dropped). If kept, the spike confirms with `EXPLAIN QUERY PLAN` on a copy of the live-size DB that the plan is `SEARCH ... USING INDEX`. If the build on 258k rows blocks writers noticeably, apply at next restart (auto-migration in `Schema.Create`), not live.
4. **State at window start**: dwell is clipped to the window. Each item's status at window start is seeded by a bulk `GROUP BY item_id MAX(created_at)` query plus one fetch of the matching rows (both plain `SELECT`, served by the existing `(item_id, created_at)` index), then the latest is chosen by parsed time in Go, never by text `ORDER BY`. Exact for ranges starting after 2026-08-18; best-effort inside the mixed-offset span (documented on the method).
5. **Cache**: 30s TTL keyed on `WindowSpec` (days, offset) plus cohort, never on clock-derived timestamps, with singleflight, a 64-entry LRU, and a bypass (singleflight only) for non-default `Config` overrides or item filters so MCP-supplied arguments cannot grow the map; every response carries `as_of`, and cache hits report original `query_ms`. Rollups only if a 30-day summary exceeds 500ms at 5x current volume (revisit trigger).
6. **Row ceiling (added after the pre-mortem)**: above `MaxWindowRows` (60,000 events, so all-time at about 258k) the reader streams item batches (`ChunkItems` 25, per-batch 5s timeout, exact because collapse and dwell are per-item); above `HardRowCeiling` (1,000,000) the read is refused with `ErrWindowTooLarge` (MCP: "use window_days <= 90"); uncached computes run one at a time (Story 1.4.3), and the context cancels the statement, not just the caller. The concurrent-writer check is Story 2.1.5.
7. **Retention** of `backlog_status_events` is unknown; nothing here depends on it. If pruning is ever added it must replace thrash runs with a summary row, not delete.

## Departure from build-vs-buy
`research/build-vs-buy.md` (:54, :63) recommends SQL-native counts/group-bys via ent `GroupBy().Aggregate()` plus one small Go collapse function. This ADR loads the window's events (about 10k rows for 30 days) into Go and computes collapse, dwell, funnel and FPY there. That is a deliberate departure: collapse, dwell and thrash-aware FPY need ordered per-item event streams, which a plain-`SELECT` group-by cannot express, and window functions or `WITH` route to the writer connection (`session/dbsplit`). It is tied to the rollup revisit trigger in item 5 (30-day summary over 500ms at 5x volume); crossing that trigger reopens the SQL-native option.

## Consequences
New code never uses bare `time.Now` (use `time.Now().UTC()` and the injected `Clock`). Tests seed a few hundred events with explicit thrash runs, never 260k rows.

## Spike result (Story 0.1.2, 2026-11-06)

Measured on a `sqlite3 .backup` copy of `~/.stapler-squad/workspaces/6eb0b580fa0331d5/sessions.db` (568 MB, 257,990 `backlog_status_events` rows, SQLite 3.51.0), through `session.NewEntRepository(WithDatabasePath(copy))` and a throwaway program outside the repo. The padded 30-day projection (`item_id, from_status, to_status, created_at, note`, `created_at >= 2026-10-06 06:00Z`) returns 10,139 rows. The live DB was never opened read-write.

1. **Reader pool label: `reader`.** A plain ent `client.BacklogStatusEvent.Query().Where(CreatedAtGTE).Select(...).Scan` moved only the `ent-read` pool's `dbobs` query counter (`ent-read:+1 ent:+0`, identical on both copies). Writer pool is `ent`, reader pool is `ent-read` (`dbpool` composes `PoolName` and `PoolName+"-read"` through `dbsplit`). Over a whole probe run (about 3,800 writes plus the reads) the counters ended at `ent:3828 ent-read:451`, so every read hit the reader pool. **Task 0.1.2b is not needed**: the existing repository client is sufficient, and Story 2.1.2's `reader` label test stays as the guard. (Pool names appear in `session/dbobs.Snapshot()` as `Stats.Name`; the test should assert `ent-read`.)
2. **Writer unaffected, within noise.** A writer loop inserted 50 rows/s for 20s per phase (about 920-985 inserts) with and without a back-to-back reader loop (worst case, no cache). Writer p99 on the no-index copy: no reader 42.9ms / 23.7ms (two runs, a 2x spread between identical phases), with reader 44.4ms / 20.7ms. On the index copy: no reader 20.3ms / 29.3ms, with reader 70.7ms / 42.9ms. The p99 spread between identical no-reader phases (20-43ms) is as large as any reader effect, because other processes shared the machine, so the "under 20% p99 change" criterion cannot be demonstrated on this box and is recorded as **not resolved by this measurement**. The routing evidence in item 1 is the stronger result: the reader holds no writer connection. Story 2.1.5 re-checks on the manual instance.
3. **Prefilter is a safe superset.** For five window starts (2026-08-10, 08-17 00:00, 08-17 12:00, 08-18, 10-06), the text prefilter `created_at >= (start - 2h30 - 15h)` missed 0 rows that a parsed-instant filter `datetime(created_at) >= start - 2h30` keeps, including the span with `-07:00` rows (at 08-17 00:00: Go keeps 247,419, SQL fetches 247,451, 83 of them `-07:00`).
4. **Index decision: keep `index.Fields("created_at")` (Story 2.1.1 stays).** `EXPLAIN QUERY PLAN` without it is `SCAN backlog_status_events` (full table); with it, `SEARCH ... USING INDEX backlogstatusevent_created_at (created_at>?)`. Through ent, warm, 40 reads per run: no index p50 147ms / 178ms (p90 198 / 335ms, min 99 / 121ms); with index p50 36ms / 36ms (p90 55ms). That misses the plan's 100ms "skip" bar by 1.5-1.8x unindexed and is about 4x faster indexed. (The `sqlite3` CLI showed 40-100ms for both, dominated by process start and output, so it was not used for the decision.) The index costs 10.7 MB (existing `(item_id, created_at)` index: 20.4 MB) and built in 0.33s on 257,990 rows with the CLI, so it is cheap to apply at the next restart through `Schema.Create`. The seed query (`GROUP BY item_id MAX(created_at)`, `created_at < ...`) already used `COVERING INDEX backlogstatusevent_item_id_created_at` with and without the new index (about 0.2s CLI wall, 122 groups).
5. **Fallback decision:** none needed. No reader-pool fallback (0.1.2b) and no index skip. Cold first reads were 0.27-1.3s (page cache), another reason to keep the 30s cache and the index.

## Story 2.1.5 results (2026-11-06)

Run by `session/ent_repository_flow_perf_test.go` (env-gated `SSQ_FLOW_PERF_DB`) against a fresh `sqlite3 -readonly .backup` copy of the live workspace DB (257,990 events; the test copies it again into a temp dir because the writer rounds insert rows; the live DB was never opened read-write). `EntFlowReader` serves all four sources with `SystemClock`. "Cold" is a new `Aggregator` (empty summary cache); "warm" is a second new `Aggregator` right after. The file was just written, so the OS page cache was not truly cold.

| Window | Events read | Cold | Warm | Path |
|---|---|---|---|---|
| 30 days | 10,126 | 69 ms | 83 ms | single `Events` read |
| All time | 257,990 | 918 ms | 850 ms | chunked `StreamEvents` (25 items per batch), `slow_path=true` |

The 30-day cold number meets the 1 s SLO by about 14x.

Writer latency with a concurrent reader: 5 alternating rounds of (all-time `Summary` running while a writer inserts one `backlog_status_events` row every 2 ms) and (a same-length period with no reader). Writer p99 per round with reader: 1.5, 6.2, 7.9, 8.7, 2.6 ms (median **6.2 ms**); without reader: 7.2, 9.6, 5.3, 6.2, 1.1 ms (median **6.2 ms**). Every write in every with-reader round stayed under the 100 ms assertion and none returned `SQLITE_BUSY`. The round-to-round spread (1-10 ms) exceeds any reader effect, so the result is "no measurable reader effect", not a tight bound. Under the concurrent writer the all-time read took a median 1.86 s (0.85-0.92 s alone): the writer slows the reader, not the reverse.

The CI-resident proof that a read holds no writer connection is `TestFlowRead_should_NotBlockWriter_When_StreamEventsPausedOnBarrier`. Not measured here: partial-index build time (Phase 5).

## Manual verification (PR-B, Story 6.1.1, 2026-11-06)

Binary built from `f420fe03b` at `~/.stapler-squad/manual-builds/manual-2/stapler-squad`, run as `PORT=62873 STAPLER_SQUAD_INSTANCE=claude-manual-test2 --tmux-keep-server` on a `sqlite3 .backup` copy of the live workspace DB (live `:8543` and `make install-service` untouched). The instance read the copy: `row_counts.events` = 257,990 = `select count(*) from backlog_status_events` on the copy. The backlog flag was enabled in the instance config; the 6 non-archived items (2 idea, 3 ready, 1 review) were set to `archived` in the instance DB only, so the copy could not spawn triage or work sessions. Consequence: "open stuck" and open-item resolutions in the UI run read 0; all numbers below were taken before that edit via the RPC (backlog feature off), and the MCP and UI runs after it matched on every compared field (30-day tiles, thrash list, raw FAIL).

| Check | Result |
|---|---|
| Raw recount (`sqlite3`, same day) | `in_progress>review` 636, `review>in_progress` 426, `pr_pending>done` 107; verdicts FAIL 186 / PARTIAL 82 / PASS 151 / UNVERIFIABLE 110 (529; plan had PASS 150) |
| Timings (curl, ConnectRPC) | 7d 33 ms, 30d 46 ms, 90d 0.86 s, all-time 0.76 s cold then under 1 ms cached (`slow_path=true` on 90d and all-time). MCP: 30d 116 ms, all-time 1.05 s, compact all-time 1.07 s |
| Thrash collapse, all-time | 61 episodes (35 items, 255,716 collapsed events); independent Python re-implementation (outside repo): 103 at M=0, 61 at 1h, 35 items, 255,716 events; by family at 1h 44/10/7 episodes on 26/8/3 items. Exact match to the plan's live figures |
| Thrash, 30d | 13 items (18 episodes); `a90b99cb` 5 episodes, 8,597 events, last 2026-11-04, listed under anomalies; `d91477d5`/`8a53ce39` absent. All-time list has `d91477d5` 87,378 and `8a53ce39` 87,063 events; `0791f5b4` is a small offender (279 events in 30d) |
| Judgment runs | Aggregator: 30 candidates (`total_count`, 10 shown with the weak-filter note in RPC and MCP). Python: 33 items with a judgment run of 3+, 29 with a second signal; the aggregator's 30 are those 29 plus `1828108a` (cap-exhausted). Plan reference 31/27; difference is data growth (529 vs 528 verdicts), not a rule gap |

**Shipped-definition checkpoint baseline** (drill keys in brackets; N = den):

| Metric | All-time | Last 30 days |
|---|---|---|
| First-pass yield (zero thrash, first `pr_pending>done`) [`fpy.first_pass`] | 30/106 (28.3%) | 3/26 (11.5%) |
| Thrash-affected merged items [`fpy.thrash_affected`] | 16 | 9 |
| Bounces per merged item [`causes.bounce_buckets`] | median 1 (40 of 106 zero; median among the 66 that bounced: 3); 28 bounce-cause rework events [`rework.bounce`] | median 1 (10 of 26 zero) |
| Non-judgment share of non-PASS verdicts [`review.class.no_commits`, `.self_check_failed`, `.reviewer_degraded`] | 142/378 (37.6%) | 20/100 (20.0%) |
| Raw / adjusted FAIL [`review.raw_fail`, `review.adjusted_fail`] | 186/529 (35.2%) / 89/364 (24.5%) | 15/154 / 13/125 |

Delta to the rough `requirements.md` table (reasons are INFERRED from definitions, not separately re-run): first-pass 30/106 vs 48/137 because the denominator is merged items and thrash-affected items fail first-pass; median bounces 1 vs 2 because bounces exclude `ReviewReopen`-noted events and thrash; non-judgment share 37.6% vs 39.7% from the 2 extra verdict classes and verdict growth.

**Owner actions, still OPEN (not faked):** (1) hand-label the top-10 deadlock candidates below (bar is 70%: below it the section stays "Review-fail streaks (heuristic)"); (2) first-render comprehension check on `/backlog/flow?window=30` (four questions, under 5 minutes); (3) screen-reader, grayscale and chart-mark contrast passes. Agent screenshots (`/tmp/flow-manual/flow-30d-{default,light,dark}.png`) were taken in a narrow viewport with the onboarding modal open, so they do not replace (3).

Top-10 candidates (all-time, ranked by run length; F fail, R partial, U unverifiable, P pass; the first column is the item id prefix):

| Item | Sequence | Run | Title |
|---|---|---|---|
| 5eaaa473 | FFFFFFFFFRUFRFFFFFFFFUFP | 15 | Auto-run ArgoCD CRD-client integration tests on preview deploy |
| 0791f5b4 | FRUUURURRURRRP | 10 | feat: session federation, view and drive sessions from multiple instances |
| c442abc7 | FRUFUUUURP | 7 | backlog page: unable to scroll |
| 735dcbee | RRRRRRRPRRRR | 7 | gh-signal inbox follow-ups |
| a90b99cb | FUUUFFUPRRRP | 7 | gh-signal: PR state reconciler (thrash item) |
| eaf40918 | FUUUUPRFFFUUU | 7 | Fix data race, withShrunkIdleSettleTimers cleanup |
| 0e775762 | FFFUFUURUPFF | 7 | Flaky: TestDeleteSession_PublishesDeletedEvent |
| 8a53ce39 | FUUUUUFFRFFF | 6 | Fix TestWorkspace_UsesWorktreePath (thrash item) |
| dc2c9bba | FFFUFFFRU | 6 | Fix TestCreateSession BeforePresetExtraArgs flake |
| 2846ce8a | RRRPUUURU | 5 | Internal-only Kafka ingress bridge |

Agent pre-fill (INFERRED from sequences only; owner confirms): `5eaaa473` real deadlock (15 fails, cap exhausted, human override); `0791f5b4` real deadlock; `eaf40918` real deadlock; `0e775762`, `dc2c9bba` agent gave up or noise (long U runs suggest reviewer trouble); `735dcbee`, `2846ce8a` reviewer too strict (PARTIAL-heavy); `a90b99cb`, `8a53ce39` noise (also thrash items); `c442abc7` unclear.
