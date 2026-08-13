# ADR-003: Client-Side Caching and Debounce Strategy

Status: Accepted
Date: 2026-04-08

## Context

Path completion triggers an API call on every keystroke (after debounce). Without caching, navigating back to a parent directory or re-typing a previously visited path generates redundant server calls. Without proper debounce and cancellation, stale responses can overwrite fresh ones (race condition).

## Decision

Implement a **three-layer protection** in the `usePathCompletions` hook:

1. **Debounce (150ms)**: Timer resets on each keystroke. Only the final value triggers an API call. Uses `setTimeout` in a `useEffect` cleanup pattern.

2. **Request cancellation (AbortController)**: Each new request aborts the previous in-flight request. Prevents stale responses from arriving after a newer request has already been dispatched.

3. **Module-level LRU cache (100 entries, 30s TTL)**: A `Map<string, CachedResponse>` keyed by `pathPrefix`. Cache hits skip the API call entirely. Entries expire after 30 seconds (filesystem state may change). LRU eviction when capacity exceeded.

Additionally, a **generation counter** (incrementing integer ref) ensures that even if a response arrives after a newer request was made AND the abort didn't fire in time, the response is silently discarded if its generation number doesn't match the current one.

## Consequences

- Typical "navigate into directory then navigate back" workflow hits cache, feeling instant.
- 150ms debounce matches perceived responsiveness research (below 200ms threshold).
- AbortController prevents the React state-update-on-unmounted-component warning.
- Cache invalidation is time-based only (no filesystem watch). Acceptable: session creation is not a long-lived interaction.
- No shared cache between hook instances (each Omnibar mount gets fresh cache). Acceptable: Omnibar is a singleton modal.

## Alternatives Rejected

1. **React Query / TanStack Query**: Would add a dependency for a single cache use case. The hook is ~40 lines with a manual cache.
2. **No cache, debounce only**: Navigating back to parent re-fetches. Noticeable delay for repeated paths.
3. **WebSocket for push-based updates**: Over-engineered for a modal that lives less than 30 seconds.
