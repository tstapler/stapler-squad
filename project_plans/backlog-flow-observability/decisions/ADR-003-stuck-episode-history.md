# ADR-003: Label historical stuck metrics as lower bounds now, and add an append-only stuck-episode log

**Status**: Accepted **Date**: 2026-11-05

## Context
`backlog_stuck_states` is unique on `(item_id, reason)` and resolved in place (`session/ent/schema/backlog_stuck_state.go`), so recurrences overwrite earlier episodes. Success metric (c) asks for historical time-in-stuck, and before/after comparison on stuck rates cannot work without episodes.

## Options
1. Lower-bound labeling only. Strength: no write path touched. Weakness: before/after on stuck metrics stays unreliable forever.
2. Add append-only `BacklogStuckEpisode` (item_id, reason, opened_at, resolved_at) written from the one `MarkStuck`/resolve path. Strength: exact data from the capture date. Weakness: new write in a hot file (`session/ent_repository_backlog.go`, 3,299 lines, 39 commits/90d), no backfill.
3. Derive episodes from `backlog_activity_notes`. Rejected: 39 rows, not a reliable signal.

## Decision
Both 1 and 2, sequenced. Phase 1-4 ship with every historical stuck metric tagged `estimated: lower_bound` and listed in `estimated[]`. Phase 5 adds the episode table (additive ent schema).

**Writer (revised after architecture review; hook points VERIFIED on `origin/main`):** `EntRepository.MarkStuck` (`session/ent_repository_backlog.go:2332`) is deliberately a single upsert plus a conditional reopen UPDATE in one tx, so that "row-dedup itself" has no read-then-write window; a read-then-insert episode writer would let two concurrent marks (reconcile tick plus PR poller) insert duplicate open episodes. So:
- **Database backstop**: a partial unique index on `(item_id, reason) WHERE resolved_at IS NULL` (ent `entsql.IndexWhere`; hand-written `CREATE UNIQUE INDEX` fallback if ent cannot express it) makes a duplicate open episode a constraint no-op.
- **Open**: inside the same tx, an ent `Create().OnConflict(DoNothing())` insert into the episode table. Rows-affected is not observable through that call (an ignored insert surfaces as `sql.ErrNoRows` or a constraint error, precedent `session/ent_repository_backlog.go:634-639`), so `EpisodeOpened = (err == nil)` and `sql.ErrNoRows` / `ent.IsConstraintError` mean "already open"; this is the primary path, with no `sql/execquery` feature. The conditional reopen UPDATE's row count gives `Reopened`. An unexported `markStuckTx` returns `stuckTransition{Reopened, EpisodeOpened}`; the exported `MarkStuck(...) (applied bool, err error)` signature is unchanged, so `Storage.MarkStuck` (`session/storage.go:1378`) and the `Repository` interface are not edited.
- **Close**: `ResolveStuck` (`:2403`) moves its single UPDATE into a tx and, when the UPDATE affects a row, closes the open episode in the same tx.
- **Deadlock guard**: the writer pool is `SetMaxOpenConns(1)`, so the helpers take the `*ent.Tx` and never touch `r.client` (tested with a 1-connection pool and a timeout).
- **Atomic, not best-effort-after**: stuck-state and episode writes commit together, so no drift window exists. A failed episode statement (non-constraint error) is logged and does not abort the stuck-state write (precedent: `recordStatusEvent`; safe because a failed statement does not abort a SQLite tx, whereas a driver where it does, such as Postgres, would need a savepoint around the episode statement); that logged case and the pre-capture legacy open rows (7 on the live DB, which get an episode at their next `MarkStuck` refresh with `opened_at = now`) are the only divergences.
- Concurrent double-mark test: 8 goroutines, exactly one open episode.

**Reader:** once data exists, the reader unions: episodes for periods after `capture_since`, legacy single-row intervals before it. The response carries `capture_since` (`FlowMeta.CaptureSince`) so the UI and MCP state exactly where exactness begins, and the reader reports `stuck_episode_drift:<n>` in `estimated[]` when open episodes differ from open stuck rows after the capture date. No backfill and no behavior change.

## Consequences
Before/after stuck comparisons are exact only for windows entirely after `capture_since`; comparisons spanning it are labeled lower-bound. Count distinct items; `multiple_reasons` is a separate bucket and is excluded from per-reason totals to avoid double counting.
