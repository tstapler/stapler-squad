# ADR-007: Shared cache lifetime, byte budget, epoch-atomic eviction

**Status**: Accepted (plan phase; amended in repair iterations 1-3 and the Phase 4 patch) · **Plan**: Stories 1.1.2, 1.1.3, 2.1.1, 2.2.1

## Context
Hash-keyed products never need invalidation, only eviction; id-relative products (reach bitsets, tree maps) are positions in an interner and are valid only for one epoch (architecture-review B3). Existing machinery: `repoCache` budget 3 GB / ~96 MB per repo, pressure tiers, `PruneToMemoryBudget` every minute, `UnderSeverePressure`, `ClearCache` (research/architecture.md §4; `G:315,459,475`). Model: `blobCacheLRU` (`G:114`).

## Decision
- `RepoState` lifetime = the `SharedObjectStore`'s (extension slot, ADR-001): refcount, idle TTL, LRU trim and budget are the store's; the `RepoState`'s `Bytes()` is included in the store's accounting. `ForgetWorktree` does not touch shared data. A refcount-balance invariant (`assertRefsBalanced` over `Registry.RefCounts()`) is checked after every worktree-opening test.
- Every product lives in a generic `repoCache[K,V]` with a bytes estimate, byte-budget eviction (mutex-guarded LRU first; CLOCK with atomic reference bits if Story 2.3.1's mutex-profile gate opens, ADR-009 item 7) and counters; keys are struct values containing the cap and `ShallowStamp` where relevant; id-relative products also carry `GraphEpoch`/`PathEpoch`. Negative/missing-object results are never cached.
- Eviction order in `PruneToMemoryBudget` (via `Registry.ExtensionsDo`): (a) product LRUs by bytes; (b) the graph — **atomically** with every id-relative product: one CAS replaces the `generation` pointer (ADR-009 item 5), and the `GraphEpoch` in id-relative keys makes any escaped value miss, so a racing reader cannot read a stale bitset; (c) the `PathID` interner likewise (`PathEpoch`); (d) never the mmap'd commit-graph base segment (page cache). Idle zero-refcount stores are pruned by the registry itself. Under severe pressure drop (a)+(b)+(c) and skip graph fill; callers use the legacy walk.
- Hard caps: `Options.MaxGraphCommits` (>= 50,000; a constructor option, not a package var), per-cache entry caps, interner byte budget. Over-cap fills return an error and the caller falls back per call to `plainWalk`. Bytes and counters exported via `RepoCacheStats` and `metrics.go` (`unfinished_repostate_*`, zero when off).
- Memory-regression gates: `Graph.Bytes()` per-row ceiling (G7), churned-history bounded-growth test, rebuild-after-eviction decode cost <= the committed-old walk, `inuse_space` <= budget + 10% in the A/B.

## Alternatives rejected
`weak`/`unique` (no budget); ristretto (probabilistic admission fights deterministic tests); tying eviction to `ForgetWorktree` (penalizes siblings); a separate refcount registry (ADR-001).
