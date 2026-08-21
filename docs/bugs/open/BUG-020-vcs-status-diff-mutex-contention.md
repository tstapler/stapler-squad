# BUG-020: GetVCSStatus and GetSessionDiff Run Git Under Lock [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-04-24
**Impact**: Git subprocess calls in VCS status and diff RPCs are executed while holding a mutex, serializing all concurrent callers and adding latency to every session diff/status request.

## Problem Description

Mutex profiling shows `GetVCSStatus` and `GetSessionDiff` are the dominant sources of mutex contention, together accounting for ~24% of cumulative mutex delay (9.6s total). These RPC handlers are making outbound git subprocess calls (`executor.Exec.Output`, `executor.CircuitBreakerExecutor.Output`) while holding a lock, which means:

1. Concurrent requests for different sessions block each other unnecessarily.
2. Any git command that runs slowly (large repo, slow disk) holds the lock for its full duration.
3. The circuit breaker wrapper doesn't help with latency under the lock.

## Reproduction Steps

1. Run stapler-squad with `--profile`
2. Capture mutex profile: `curl -s --output mutex.prof http://localhost:6060/debug/pprof/mutex`
3. Inspect: `go tool pprof -top mutex.prof`
4. Expected: mutex contention traces to lock/unlock of short critical sections, not git subprocess calls
5. Actual: `GetVCSStatus` and `GetSessionDiff` handlers show up as top mutex contention sources via `executor.Exec.Output`

## Root Cause

The session service likely acquires a per-session (or global) RWMutex to read session state, then calls git while still holding the lock. The fix is to read the necessary data (worktree path, branch, etc.) under the lock, release it, then execute the git command lock-free.

## Files Likely Affected

- `server/services/` — GetVCSStatus and GetSessionDiff handler implementations
- `session/instance.go` — where per-session locks are held during git operations
- `session/git/` — git command execution

## Fix Approach

1. Identify the lock scope in the VCS status and diff handlers.
2. Extract only the data needed from the locked session struct (path, branch ref, etc.).
3. Release the lock before invoking any git subprocess.
4. Re-acquire lock only if writing results back to session state.

For the diff handler specifically, consider caching the last diff result with a short TTL (e.g., 1s) to avoid redundant git calls on rapid re-requests.

## Verification

After fix: `GetVCSStatus` and `GetSessionDiff` no longer appear in the top mutex contention nodes. Response latency for diff/status RPCs should decrease, especially under concurrent load.

## Related Tasks

- BUG-021: CheckGHAuth mutex contention
