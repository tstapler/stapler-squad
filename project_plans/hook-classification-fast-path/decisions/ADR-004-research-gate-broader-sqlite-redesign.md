# ADR-004: Research-gate broader SQLite redesign

**Status**: Accepted gate; implementation decision pending benchmarks
**Date**: 2026-09-23

## Context

The workspace DB is large and analytics contribute material growth. A dedicated analytics database, immutable shards, or a global write actor could help, but each expands migration and operational scope. Current observed hook volume is about 4,419/day with bursts around 40/minute.

## Decision

Mandatory scope batches hook analytics behind `AnalyticsBatchSink`. Before moving analytics to a dedicated DB, benchmark representative data for latency, WAL growth, query complexity, disk, backup, migration, and rollback. Before routing unrelated writes through a shared actor, benchmark real writer wait/commit behavior.

If evidence favors either change, produce a separate migration plan/ADR update. Do not copy live SQLite files; use online backup or closed immutable shards if later selected.

## Alternatives rejected

- Full commitment now: speculative scope and migration risk.
- Permanently reject redesign: ignores existing DB growth evidence.

## Gate record

Pending Phase 4 benchmark results. Record commands, fixtures, numbers, and proceed/defer verdict here before implementation is considered complete.