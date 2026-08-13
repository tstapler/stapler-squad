# BUG-021: CheckGHAuth Holds Mutex During External Auth Check [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-04-24
**Impact**: GitHub auth check runs under a mutex, adding ~21% of total mutex delay (2.02s cumulative). Blocks concurrent requests during what should be a read-only, lock-free operation.

## Problem Description

Mutex profiling shows `github.CheckGHAuth` accumulates 2.02s of cumulative mutex delay, representing ~21% of the 9.6s total. Auth checking is inherently a read-only operation (querying a token or running `gh auth status`) and should not require holding a mutex for its duration.

The likely cause is that `CheckGHAuth` either:
- Is called while a session or server lock is held (lock is too broad), or
- Internally acquires a mutex to cache its result but holds it during the network/subprocess call rather than using a double-checked locking pattern.

## Reproduction Steps

1. Run stapler-squad with `--profile`
2. Capture mutex profile: `curl -s --output mutex.prof http://localhost:6060/debug/pprof/mutex`
3. Inspect: `go tool pprof -top mutex.prof`
4. Expected: `github.CheckGHAuth` is not in the top mutex contention nodes
5. Actual: `github.CheckGHAuth` shows 2.02s cumulative delay (20.91% of total)

## Root Cause

Unknown — needs investigation. Likely one of:
- A broad lock held by the caller that encompasses the `CheckGHAuth` call
- A cache mutex inside `CheckGHAuth` that's held during a slow subprocess/network call instead of only protecting the cache read/write

## Files Likely Affected

- `github/` — `CheckGHAuth` implementation
- Callers of `CheckGHAuth` in `server/services/` — lock scope at call sites

## Fix Approach

1. Locate `CheckGHAuth` and its callers.
2. If the caller holds a broad lock: extract auth check outside the lock boundary.
3. If `CheckGHAuth` has an internal cache mutex: use double-checked locking — check cache without lock, lock only to write the result, use `sync/atomic` or a `singleflight` group to coalesce concurrent auth checks.
4. Consider caching the auth result with a TTL (e.g., 30s) so the check rarely runs at all.

## Verification

After fix: `github.CheckGHAuth` does not appear in mutex profiling top nodes. Unary RPC latency for operations that trigger auth checks should decrease.

## Related Tasks

- BUG-020: GetVCSStatus and GetSessionDiff mutex contention — same class of problem
