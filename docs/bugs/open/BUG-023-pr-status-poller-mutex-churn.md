# BUG-023: PRStatusPoller Has Excessive Mutex Churn; Auth State Should Use atomic.Value [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-06-24
**Impact**: `session/pr_status_poller.go` uses a single `sync.Mutex` that serializes
instance-list reads, auth state reads, and poll-result writes. Auth state should be an
`atomic.Value` storing an immutable snapshot; instance list should use `sync.RWMutex`
or `atomic.Value` snapshot to allow lock-free reads during the poll tick.

## Problem Description

The poller struct contains:

```go
type PRStatusPoller struct {
    mu        sync.Mutex
    instances []PollerInstance
    // auth state stored inline — authOK bool, authCheckedAt time.Time
    // ...
}
```

Every operation — reading the instance list, checking auth freshness, updating poll results —
acquires `p.mu.Lock()`. During a poll tick the sequence is roughly:

```
p.mu.RLock()   // read auth state
p.mu.RUnlock()
// CheckGHAuth() — correctly outside the lock
p.mu.Lock()    // write new auth state
p.mu.Unlock()
for each instance:
    p.mu.RLock()   // read instances
    p.mu.RUnlock()
    // HTTP call — correctly outside the lock
    p.mu.Lock()    // write poll result
    p.mu.Unlock()
```

Problems:

1. **Auth state reads lock the same mutex as instance updates**. A concurrent
   `AddInstance()` or `RemoveInstance()` call during a poll tick can block the auth check
   read, and vice versa. These have no data dependency and should not contend.

2. **Auth state is the wrong type**. `authOK bool + authCheckedAt time.Time` are two fields
   that must be read/written atomically together. The only safe way to do this without a
   mutex is `atomic.Value` storing an immutable `authResult{ok bool, checkedAt time.Time}`.

3. **Instance list read requires a lock**. The instance list is written infrequently
   (`AddInstance`/`RemoveInstance`) but read on every poll tick. A `sync.RWMutex` (or an
   `atomic.Value` holding an immutable `[]PollerInstance` snapshot) would allow all poll
   goroutines to read lock-free.

4. **Churn amplification**: With N instances, the poller acquires and releases the mutex
   at least `2 + 2N` times per tick (auth read, auth write, N result reads, N result writes).
   Under a 20-session workload this is 42 lock/unlock pairs per 60-second cycle — low
   absolute frequency but high contention surface when `AddInstance`/`RemoveInstance` are
   called concurrently from session lifecycle events.

## Root Cause

`PRStatusPoller` was implemented before the project's preference for atomic/lock-free
structures was established. The `sync.Mutex` is used as a catch-all for all shared state.

## Files Affected

- `session/pr_status_poller.go` — entire file (~398 lines)

## Fix Approach

### Auth state: atomic.Value

```go
type authResult struct {
    ok        bool
    checkedAt time.Time
}

type PRStatusPoller struct {
    authState atomic.Value // stores authResult; zero value = not yet checked
    // ...
}

func (p *PRStatusPoller) loadAuthState() (authResult, bool) {
    v := p.authState.Load()
    if v == nil {
        return authResult{}, false
    }
    return v.(authResult), true
}

func (p *PRStatusPoller) storeAuthState(ok bool) {
    p.authState.Store(authResult{ok: ok, checkedAt: time.Now()})
}
```

### Instance list: atomic snapshot

```go
type PRStatusPoller struct {
    authState atomic.Value     // stores authResult
    instances atomic.Value     // stores []PollerInstance (immutable snapshot)
    instMu    sync.Mutex       // held only during Add/Remove to serialize writers
}

func (p *PRStatusPoller) SetInstances(list []PollerInstance) {
    cp := make([]PollerInstance, len(list))
    copy(cp, list)
    p.instances.Store(cp)
}

func (p *PRStatusPoller) AddInstance(inst PollerInstance) {
    p.instMu.Lock()
    defer p.instMu.Unlock()
    cur := p.loadInstances()
    cp := make([]PollerInstance, len(cur)+1)
    copy(cp, cur)
    cp[len(cur)] = inst
    p.instances.Store(cp)
}

func (p *PRStatusPoller) loadInstances() []PollerInstance {
    v := p.instances.Load()
    if v == nil {
        return nil
    }
    return v.([]PollerInstance)
}
```

Poll tick then reads with zero locking:
```go
instances := p.loadInstances() // lock-free atomic.Value.Load
auth, ok := p.loadAuthState()  // lock-free atomic.Value.Load
```

## Verification

After fix: `go test ./session/... -race -count=5` must pass without DATA RACE.
Add `BenchmarkPRStatusPoller_PollTick` that measures ns/op before and after.

## Related

- BUG-020: GetVCSStatus mutex contention
- BUG-021: CheckGHAuth mutex contention (same poller file)
- BUG-022: ETagCache RWMutex map
- BUG-024: SearchService branchCache RWMutex
