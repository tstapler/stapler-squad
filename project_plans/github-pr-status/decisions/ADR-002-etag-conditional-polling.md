# ADR-002: ETag Conditional Polling at 60-Second Interval

**Status:** Accepted
**Date:** 2026-04-09
**Decision:** Poll every 60 seconds using HTTP ETag conditional requests (`If-None-Match`), not naive 5-minute polling.

## Context

GitHub REST API rate limit is 5,000 requests/hour (authenticated). With N sessions polling every T seconds:

```
requests/hour = (3600 / T) * N
```

At T=300s (5 min): 12 * N requests/hour. Safe for ~416 sessions.
At T=60s (1 min): 60 * N requests/hour. Exceeds limit at ~83 sessions.

However, GitHub REST API supports `ETag` / `If-None-Match` conditional requests:

- Every GET response includes an `ETag` header.
- Subsequent requests with `If-None-Match: <cached-etag>` return `304 Not Modified` if unchanged.
- **304 responses cost zero rate limit quota.**

This means unchanged PRs (the vast majority during any 60-second window) consume no rate limit. Only PRs that actually changed count against the 5,000/hour budget.

## Decision

- Default poll interval: **60 seconds** (configurable).
- Store ETag per `(owner, repo, prNumber)` in the poller's in-memory cache.
- Pass ETag via `gh api --header "If-None-Match: <etag>"`.
- On 304: skip processing, update `LastPRStatusCheck` timestamp only.
- On 200: parse new data, update ETag cache, process status change.
- Semaphore: max 5 concurrent `gh` calls to respect secondary rate limits.
- On rate limit error: pause all polling for 60 seconds.

## Consequences

### Positive

- **60-second freshness** instead of 5-minute staleness -- status changes appear within 1 minute.
- **Near-zero rate limit cost** for inactive PRs (304s are free).
- **Scales to hundreds of sessions** in practice -- only the changed subset costs quota.
- **Graceful degradation** on rate limit: pause-and-resume, not crash.

### Negative

- **ETag cache memory:** O(N) for N sessions. Negligible -- one string per session.
- **`gh api` instead of `gh pr view`:** ETag support requires the lower-level `gh api` subcommand. Slightly more complex invocation.
- **No ETag on first call:** First poll for any session always costs one rate-limited request. Acceptable.

## Alternatives Considered

### Naive 5-minute polling (no ETag)

Rejected. 5-minute staleness is poor UX for a monitoring tool. ETag makes aggressive polling cheap.

### `gh pr view --json` (no ETag support)

Rejected for status polling. `gh pr view` does not expose response headers. `gh api` is needed for header access. `gh pr view` remains appropriate for the initial data fetch and PR discovery.

### WebSocket / GraphQL subscription

Not available. GitHub has no push mechanism for local tools.
