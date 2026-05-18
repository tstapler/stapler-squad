# Session Hibernation — Pitfalls & Failure Modes Research

## 1. Auto-Resume Pitfalls: Exact Code Paths That Break Hibernated Sessions

### 1a. Health Monitor Auto-Restart (session/health.go, line 130)

**Location**: `session/health.go` lines 108–144, within `checkSingleSession()`

**Current code** (lines 108–130):
```go
if instance.Started() {
    if !instance.TmuxAlive() {
        result.IsHealthy = false
        result.Issues = append(result.Issues, "Instance marked as started but tmux session doesn't exist")
        
        // Debounce: only recover after failureThreshold consecutive failures.
        h.failureCountsMu.Lock()
        h.failureCounts[instance.Title]++
        count := h.failureCounts[instance.Title]
        h.failureCountsMu.Unlock()
        
        if count < failureThreshold {
            // deferring recovery
        } else {
            // Threshold reached - attempt recovery
            h.failureCountsMu.Lock()
            h.failureCounts[instance.Title] = 0 // Reset counter after attempt
            h.failureCountsMu.Unlock()
            
            result.RecoveryAttempted = true
            if err := instance.Start(false); err != nil {  // <-- PITFALL LINE
```

**The problem**: When a hibernated session is loaded from disk, it has:
- `Status == Hibernated` (new status)
- `started == true` (was started before hibernation)
- No tmux session (intentionally killed during hibernation)

On the next scheduled health check interval (default 30s), the health monitor will:
1. Find `instance.Started() == true` ✓
2. Find `instance.TmuxAlive() == false` ✓ (expected for hibernated)
3. After 2 consecutive health-check failures, call `instance.Start(false)`
4. **Result**: Hibernated session is silently restarted, defeating the purpose

**Fix required**: Add early bailout:
```go
// Skip hibernated instances - they have no tmux by design
if instance.Hibernated() {
    result.Actions = append(result.Actions, "Skipped (session is hibernated)")
    return result
}
```

---

### 1b. Startup Deserialization Auto-Start (session/instance_serialization.go, lines 328–331)

**Location**: `session/instance_serialization.go` lines 304–334, within `FromInstanceData()`

**Current code** (lines 304–331):
```go
} else if instance.Status == Stopped {
    // Wire the tmux session object so DoesSessionExist() can be called.
    // ...
    if instance.tmuxManager.DoesSessionExist() {
        log.Warn("session stored as stopped but tmux is alive, recovering to running", "session", instance.Title)
        instance.setStatus(Running)
        if err := instance.Start(false); err != nil {
            log.Warn("recovery start failed, keeping stopped", "session", instance.Title, "err", err)
            instance.setStatus(Stopped)
            instance.started = true
        }
    } else {
        instance.started = true
    }
} else {
    if err := instance.Start(false); err != nil {  // <-- PITFALL LINE
        return nil, err
    }
}
```

**The problem**: The final `else` branch (lines 328–331) catches **all statuses except `Stopped` and `Paused`**. When loading a hibernated session:
1. Status is `Hibernated` (not `Stopped`, not `Paused`)
2. Control falls through to the final `else`
3. `instance.Start(false)` is called, **auto-resuming immediately**

**Fix required**: Expand the exclusion guard:
```go
} else if instance.Status == Stopped {
    // existing recovery logic
    ...
} else if instance.Status == Hibernated {
    // Wire tmux for later manual resume, but don't start
    instance.started = true
    // (no Start call)
} else {
    if err := instance.Start(false); err != nil {
        return nil, err
    }
}
```

---

## 2. All Calls to `instance.Start()` in Non-Test Code

Searched codebase excluding `_test.go` files and worktrees. **Full list**:

| File | Line | Context | Notes |
|---|---|---|---|
| `server/services/session_service.go` | ~467 | `CreateDirectorySession()` after `NewInstance()` | First-time start in new session creation |
| `server/services/session_service.go` | ~792 | `CreateSession()` after `NewInstance()` | First-time start in RPC handler |
| `session/health.go` | ~130 | `checkSingleSession()` recovery path | Auto-restart after 2 health-check failures |
| `session/instance_serialization.go` | ~320 | `FromInstanceData()` recovery for stopped sessions | When tmux is alive but status is Stopped |
| `session/instance_serialization.go` | ~329 | `FromInstanceData()` final else branch | Startup deserialization for non-Stopped/Paused |
| `session/instance_claude.go` | ~85 | `recoverFromStaleResume()` callback | Auto-restart when --resume flag points to stale conversation UUID |

**Summary**: 6 call sites, split into:
- **2 intentional first-time starts** (session creation)
- **4 auto-restart paths** (health recovery, stale resume, deserialization recovery)

---

## 3. SIGTERM Handling and Graceful Shutdown Risks

**File**: `session/instance_claude.go` — **Does NOT contain SIGTERM handling.**

**Where SIGTERM would be sent**: 
- During hibernation via `instance.KillSession()` → calls `tmux kill-session`
- Or via `instance.Destroy()` → calls `tmux kill-session`

**Actual signal path**: 
1. Stapler-squad calls `tmux kill-session -t <name>` (via `session/instance_tmux.go:110`)
2. tmux **forcibly terminates all panes** in the session (no SIGTERM sent to child process)
3. Claude process is **killed abruptly** (SIGKILL, not SIGTERM)

**Graceful shutdown handling**: **None detected**. Claude processes do not get:
- SIGTERM for graceful shutdown
- Signal handlers to flush buffers
- Time to save state before death

**Risks**:
1. **Orphan processes**: If Claude spawns child processes (e.g., subprocess, npm tasks), they may not be reaped (tmux only kills the immediate shell, not its descendants)
2. **Unflushed data**: Claude's in-memory buffers (if any) are lost without graceful close
3. **Incomplete checkpoint**: If we checkpoint terminal output **after** SIGKILL, we might miss final output (race condition)

**Mitigation strategy** (from requirements):
- Requirements already specify SIGTERM → wait 10s → SIGKILL pattern
- **Implementation detail**: This should happen in the hibernation code, not here
- Use `tmux send-signal` (if available) or `kill -TERM` before `kill -SESSION`

---

## 4. Resource-Pressure Hibernation: Risks & Trade-Offs

### 4a. Race Conditions

**Race 1: Check → Act window**
```
Time 0:  check_memory() → 87% → decide to hibernate oldest session
         schedule goroutine to hibernate session_A
         
Time 50ms: User clicks "Resume" on session_A
           session_A starts starting up (Start call pending)
           
Time 100ms: Hibernation goroutine fires
            Tries to kill already-starting session_A
            Result: Session killed mid-startup, or partial tmux state
```

**Mitigation**: Use session lock/state transition to prevent concurrent start+hibernate.

### 4b. Hibernating the Wrong Session

**Problem**: If two sessions have identical "oldest idle time", which one do we hibernate?

**Current risk**: No tie-breaking logic → depends on map iteration order (non-deterministic in Go)

**Example failure**:
```
Session A: idle 120 minutes, last used by user at 10:00am
Session B: idle 120 minutes, last used by automation at 10:00am

Under memory pressure, code hibernates Session B (automation).
User is surprised: their active debugging session is paused.
```

**Mitigation**: Sort by multiple criteria:
1. Idle time (oldest first)
2. If tied: last manual interaction (vs. automated output)
3. If tied: session creation time

### 4c. Threshold Tuning Instability

**Problem**: Binary threshold at 85% → oscillation

```
Time 0:    Memory = 84% → do nothing
Time 30s:  Memory = 86% → hibernate session_A
Time 60s:  Memory = 85% (freed from session_A death delay) → do nothing
Time 90s:  Memory = 87% → hibernate session_B
Time 120s: Memory = 83% (more freed) → do nothing, but too late (already hibernated 2)
Result: Thrashing, unnecessary hibernations
```

**Mitigation**: 
- Add hysteresis: re-enable starts only when memory drops to 75% (not 85%)
- Log each hibernation with pre/post memory snapshots
- Make threshold configurable with sane defaults

### 4d. gopsutil vs /proc/meminfo Portability

**Risk**: gopsutil may not be installed or may fail on some Linux variants.

**Fallback options**:
1. Parse `/proc/meminfo` directly (Linux only)
2. Use `free` command output
3. Gracefully degrade: if memory check fails, skip resource-pressure hibernation

**Tested platforms needed**:
- Linux (glibc, musl)
- macOS (no /proc/meminfo)
- Docker containers (cgroup limits, not true system memory)

---

## 5. Existing Session Lifecycle Test Patterns to Extend

### 5a. From `session/session_restart_test.go`

**Existing patterns**:
- `TestSessionRestartWithConversationContinuity()` — full lifecycle: start → kill → restart
- Sub-tests for: valid session UUID, invalid UUID, non-Claude programs, health checker restart, lazy recovery
- Health checker simulation in `testHealthCheckerAutoRestart()` (lines 213–269)
  - Starts session
  - Kills tmux session
  - Calls `instance.Start(false)` to simulate recovery
  - Asserts session is running after restart

### 5b. New Tests Needed for Hibernation

**Test 1: Hibernated Session Not Auto-Restarted on Deserialization**
```go
func testHibernatedSessionNotAutoStartedOnDeserialize(t *testing.T) {
    // Create & serialize a hibernated session
    instance := createAndHibernateSession(t)
    data := instance.ToInstanceData()
    
    // Deserialize fresh (simulates server restart)
    restored, err := FromInstanceData(data)
    require.NoError(t, err)
    
    // Assert: NOT started automatically
    assert.False(t, restored.Started())
    assert.Equal(t, Hibernated, restored.Status)
    assert.False(t, restored.TmuxAlive())  // No tmux session exists
}
```

**Test 2: Health Check Skips Hibernated Sessions**
```go
func testHealthCheckerSkipsHibernatedSessions(t *testing.T) {
    instance := createAndHibernateSession(t)
    checker := NewSessionHealthChecker(storage)
    
    results, err := checker.CheckAllSessions()
    require.NoError(t, err)
    
    // Find the hibernated session in results
    hibernatedResult := findResultByTitle(results, instance.Title)
    
    // Assert: marked as healthy (skipped), no recovery attempted
    assert.True(t, hibernatedResult.IsHealthy)
    assert.False(t, hibernatedResult.RecoveryAttempted)
    assert.Contains(t, hibernatedResult.Actions, "Skipped (session is hibernated)")
}
```

**Test 3: Manual Hibernation → Resume Cycle**
```go
func testHibernationAndManualResume(t *testing.T) {
    instance := createRunningSession(t)
    
    // Manually hibernate
    err := instance.Hibernate(ctx)
    require.NoError(t, err)
    
    assert.Equal(t, Hibernated, instance.Status)
    assert.False(t, instance.TmuxAlive())
    
    // Check checkpoint exists
    checkpointPath := getCheckpointPath(instance.UUID)
    assert.FileExists(t, filepath.Join(checkpointPath, "scrollback.txt"))
    assert.FileExists(t, filepath.Join(checkpointPath, "checkpoint.json"))
    
    // Resume manually
    err = instance.Resume(ctx)
    require.NoError(t, err)
    
    assert.Equal(t, Running, instance.Status)
    assert.True(t, instance.TmuxAlive())
}
```

**Test 4: Stale Resume Auto-Recovery Doesn't Break Hibernation**
```go
func testStaleResumeDoesNotWakeHibernatedSession(t *testing.T) {
    // Create session with stale Claude conversation UUID
    instance := createSessionWithStaleConversationUUID(t)
    
    // Hibernate it
    err := instance.Hibernate(ctx)
    require.NoError(t, err)
    
    // Simulate stale-resume auto-recovery path finding it
    // (This would normally call recoverFromStaleResume → instance.Start(false))
    
    // Assert: No auto-restart happens because Hibernated sessions
    // shouldn't be checked for stale resume in the first place
    assert.Equal(t, Hibernated, instance.Status)
}
```

### 5c. Integration Test Pattern

Add to CI pipeline:
```bash
# Test: hibernation + server restart + manual resume
make test-hibernation-lifecycle
```

---

## Summary: Blocking Pitfalls Before Implementation

| Pitfall | Severity | Fix Complexity | Must-Fix Before Launch |
|---------|----------|---|---|
| Health monitor auto-restart on hibernated sessions | Critical | Low (1-line guard) | **YES** |
| Deserialization auto-start on hibernated sessions | Critical | Medium (3-line else branch) | **YES** |
| Stale-resume auto-recovery path (instance_claude.go line 85) | High | Medium (guard check) | **YES** |
| SIGTERM vs SIGKILL ordering (graceful shutdown) | Medium | Low (wrapper around kill) | Recommended before v1 |
| Resource-pressure race conditions (concurrent start+hibernate) | Medium | High (locking/transitions) | **YES** (before FR-3 enabled) |
| Threshold oscillation (hysteresis) | Low | Low (config param) | Post-v1 |
| gopsutil portability on non-Linux | Low | Medium (fallback strategy) | Recommended |

