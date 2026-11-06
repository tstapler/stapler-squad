# ADR-002: Collapse thrash at query time, keyed on flip shape, not note text

**Status**: Accepted **Date**: 2026-11-05

## Context
258k `backlog_status_events` rows, 238k of them `ready<->in_progress` flips. The PR-fix rollback `in_progress>pr_pending` has an empty note in 4,372 of 4,381 rows, so note-keyed collapse misses the second loop family (`research/features.md` section 4).

## Decision
- Collapse in `session/flow` as a pure function `Collapse(events, window, cfg) []Span`, never at write time (the event log is an audit trail, and write-time dedupe changes pipeline behavior, which is out of scope).
- **Flip pair**: consecutive events on one item that are `A>B` then `B>A` for A/B in `{ready,in_progress}`, `{queued,in_progress}`, `{pr_pending,in_progress}`, with gap under `G` (default 15 min).
- **Thrash episode**: maximal run of flip pairs with each gap under `G` and at least `N` events (default 6; the unit is events everywhere: glossary, tests, this ADR). Emits one `ThrashSpan` (sealed `Span` implementation; a `RealSpan` cannot carry a cause or count) with count, first/last, cause, net status.
- **Episode merging (added after adversarial review)**: adjacent episodes on one item with the same flip pair and an inter-episode gap under `M` (`ThrashMergeGap`, default 1h) merge. Verified by simulation on the live DB (2026-11-05): at `G=15m` there are 103 episodes across 35 of 160 items, not "one per item" (`a90b99cb`: 17 episodes; `0791f5b4`: 2; `d91477d5`: 18; `8a53ce39`: 17); merging at 1h gives 61 (`a90b99cb`: 5), at 6h 3 for each of the three large items. These counts are all-cause: by family after the 1h merge, `ready<->in_progress` 44, `queued<->in_progress` 10 and `pr_pending<->in_progress` 7 (on 3 items), so 61 is not the PR-fix count (re-simulated in the Phase 4 patch). 1h is a compromise that fixes dequeue-cadence jitter without hiding a real pause. Because fragmentation remains, the anomaly unit shown to users is the **item** (episodes, total events, first/last), and the banner counts items.
- **Collapse runs on events padded before the window start.** `CollapsePad = G x N + M` (2h30m by default). A chain that continues across `Start - G x N` with every gap under `G` has at least `N` events inside the pad, so its episode classification is exact; the `M` term lets the merge pass see the preceding episode. Spans are clipped to the window only after collapse (`Count` reported clipped, `StartsBeforeWindow` flagged). Without this, a boundary-straddling episode loses its head and a remainder under `N` events would be counted as real rework.
- Real transitions (`in_progress>review`, `review>in_progress`, `review>pr_pending`, `pr_pending>done`, `done>archived`) are never members of a flip pair, so are never collapsed.
- Dwell inside an episode goes to a separate `thrash` bucket, never to `ready`/`in_progress`.
- Note text is used only to label `Cause` (closed prefix table plus an always-reported `unclassified` bucket), never to decide membership.
- Runs under N events are normal flow and appear in raw counts and a per-item transition-rate anomaly list.

## Consequences
Totals reconcile as `raw_events = real_transitions + collapsed_events + self_loops`. Thresholds `G`, `N` and `M` live in `flow.Config`, tested at boundaries (5 vs 6 events; 14:59 vs 15:01 gap; 59:59 vs 1:01:00 merge gap). `CollapsePad` is derived from them, with a test that fails if it is hard-coded. An all-time window (`all_time = true`, `WindowSpec{AllTime: true}`) is a slow path flagged `slow_path: true`; it reads through the item-batched chunked path (plan Story 2.1.6), which is exact because collapse is per item.
