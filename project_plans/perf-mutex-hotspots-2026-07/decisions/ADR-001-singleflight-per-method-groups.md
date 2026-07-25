# ADR-001: Use Separate singleflight.Group Per Method in GoGitVCSReader

**Date**: 2026-07-01
**Status**: Accepted
**Context**: perf-mutex-hotspots-2026-07

---

## Context

`GoGitVCSReader` has three hot methods — `AheadBehind`, `DiffShortstat`, `HasUncommitted` — each of which holds `entry.mu` for the duration of a go-git packfile read. The scanner runs 4 concurrent workers per repo; when the TTL cache expires, all 4 workers attempt to acquire `entry.mu` simultaneously. This thundering herd was profiled at ~1.05T cycles, 7000+ events.

`golang.org/x/sync/singleflight` is already in `go.mod` (v0.20.0) and is used by `github/user_pr_cache.go` in this codebase.

## Decision

Add **three separate `singleflight.Group` fields** to `GoGitVCSReader`, one per method:

```go
aheadBehindSF    singleflight.Group
diffStatSF       singleflight.Group
hasUncommittedSF singleflight.Group
```

Each method's slow path (post-TTL-check) is wrapped in `<method>SF.Do(key, func() (any, error) { ... })`.

`entry.mu.Lock()` is placed **inside** the `Do` body. A deferred `recover()` inside the `Do` body converts go-git panics to errors.

## Alternatives Considered

### Single shared singleflight.Group with method-prefixed keys

One `Group` field; keys like `"ab:"+cacheKey`, `"ds:"+worktreePath`.

**Rejected**: Creates false-coalescing risk if key-prefix logic ever drifts between methods. Harder to reason about coalescing boundaries. No measurable benefit over separate groups.

### entry.mu outside Do body

Acquire `entry.mu` before calling `Do`, release after.

**Rejected**: Placing the mutex acquisition outside `Do` means N-1 coalesced goroutines all try to reacquire the mutex after `Do` returns — exactly the thundering herd we are trying to eliminate. The singleflight pattern only helps when the expensive operation (including mutex acquisition) is entirely inside `Do`.

## Consequences

- **Positive**: Up to 4x reduction in `entry.mu` lock contention on cache miss. Panics inside go-git are caught and returned as errors instead of propagating to all 4 scanner workers.
- **Positive**: Zero new dependencies (singleflight already in go.mod).
- **Neutral**: Adds 3 struct fields (`singleflight.Group` is ~56 bytes each). Zero-value safe; no constructor change needed.
- **Negative**: Slightly more verbose slow-path wrappers. Mitigated by the small number of methods (3) and the clear `Do` call pattern.

## Implementation Notes

- The `Do` return value is typed `(any, error)`. Use a local named result type (e.g. `abResult`) for `AheadBehind` to avoid unsafe type assertions on `int` pairs.
- The third return value of `Do` (`shared bool`) is intentionally discarded (`_`). We do not need to distinguish shared from solo results.
- The `//nolint:exhaustruct` comment is required on each `singleflight.Group` field because the linter requires struct fields to be explicitly zeroed.
