# ADR-001: Defer per-tick GraphQL PR-batching; migrate only `GetPRInfoCtx`'s own implementation

**Status**: Accepted
**Date**: 2026-09-08
**Project**: github-call-optimization

## Context

Requirements' Scope item 3 originally asked for "GraphQL batching + `GetPRInfoCtx` migration,"
read by an early pass as one aliased GraphQL query per poller tick, replacing the pollers'
per-instance fetch fan-out.

Phase 2 research (`research/pitfalls.md` §0, `research/architecture.md` §1b, `research/stack.md`
§2a) established two facts that change the shape of this decision:

1. GitHub's GraphQL API has no ETag/If-None-Match/304 support — this is REST-only. Both pollers'
   actual per-tick hot path is `github.GetPRInfoConditional` (REST + ETag,
   `github/etag_cache.go:53-130`), not `GetPRInfoCtx` directly. `GetPRInfoCtx` only fires as
   `GetPRInfoConditional`'s "PR changed" full-refetch fallback (`etag_cache.go:109,122`).
2. `github/user_pr_cache.go:516`'s existing GraphQL query is not a precedent for the aliasing
   technique batching would need — it's a single list-field fetch, not N aliased single-item
   lookups. A real batched query is new design work with its own risks: GitHub's per-query cost
   model scales roughly linearly with aliased PRs but not exactly 1:1 (nested `reviews`/`commits`
   connections add cost per subtree), and GitHub added a second, independent
   `RESOURCE_LIMITS_EXCEEDED` failure mode in September 2025 for wide/deep queries — a distinct
   error-classification branch `isGHRateLimited` (`github/http_client.go:183`) doesn't recognize.

## Decision

Migrate only `GetPRInfoCtx`'s own implementation (the `gh pr view --json ...` shell-out) to a
native, single-PR GraphQL query. `GetPRInfoConditional`'s REST+ETag path — the pollers' actual
per-tick mechanism — is unchanged. No per-tick aliased batch query (whether over the full tracked
set or only the "changed" subset) ships as part of this project.

## Consequences

- Full 304 savings are preserved for the common "nothing changed" tick — the primary lever behind
  Success Metric #2 (reduced steady-state call volume) stays intact.
- The migration still delivers value: every "PR changed" full-refetch and every interactive RPC
  (`GetPRInfo`, and indirectly `MergePR`/`ClosePR`/`PostPRComment` via `RefreshPRInfo`) gets one
  GraphQL round trip instead of a `gh` subprocess shell-out, and folds into `DefaultRateLimiter`
  for the first time (per requirements' explicit callout).
- Call-count-per-tick is unchanged by this project — it was never the lever; call *cost/shape* is.
- **Future enhancement, explicitly out of scope here**: aliasing the "changed" subset discovered
  within a single tick (should more than one PR change simultaneously) into one GraphQL call
  instead of N sequential `GetPRInfoCtx` calls. This is legitimate follow-on work once Phase 1's
  observability data shows how often multiple PRs actually change in the same tick — if that's
  rare (plausible at Tyler's single-user scale), the added complexity (grouping-by-repo design,
  `RESOURCE_LIMITS_EXCEEDED` handling, partial-alias-failure handling per `research/features.md`
  §2) would not have paid for itself.

## Alternatives Considered

See plan.md's Step 0.5 and Pattern Decisions table for the full three-way comparison (wholesale
per-tick batch / single-PR migration only / changed-subset batching).
