# ADR-002: Bounded-loss analytics actor

**Status**: Accepted; tuning values provisional pending benchmark
**Date**: 2026-09-23

## Context

Classification must not wait for analytics persistence. Existing `AnalyticsStore` is non-blocking but flushes each row independently. SQLite WAL permits concurrent readers but one writer. The user accepts bounded event loss on abrupt process termination.

## Decision

Evolve the existing store into one typed actor with a bounded queue and one writer. Flush an ordered `AnalyticsBatch` by size or short deadline in one transaction. Queue saturation drops/counts rather than blocking decisions. Graceful shutdown drains within a deadline; abrupt loss is bounded by queued/in-flight events.

Provisional values are queue 1,000, batch 128, delay 2 ms. A real-WAL benchmark matrix must select final values and record the maximum loss window before shipping.

## Alternatives rejected

- Synchronous write: violates response-latency independence.
- Goroutine per event: unbounded concurrency and SQLite lock contention.
- Durable broker/spool: exceeds accepted durability and current volume.
- Actor framework: unnecessary dependency for one owner loop.

## Consequences

High write efficiency and predictable producer latency, but decision delivery no longer implies durable analytics. Metrics must expose queue age/depth, batch latency, retry, and drops.