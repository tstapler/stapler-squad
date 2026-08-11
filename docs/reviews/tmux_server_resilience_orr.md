# Operational Readiness Review: Tmux Server Resilience Changes

**Project**: claude-squad (local session management tool, localhost:8543)  
**Scope**: New startup sequence recovery in `main.go` for tmux server resilience  
**Review Date**: 2026-04-03  
**Status**: 🟡 **Minor gaps** — Not production-blocking, but gaps exist in documentation and tight-loop prevention

---

## Executive Summary

The proposed startup sequence adds three resilience calls that are **not yet implemented**:

```go
if err := tmux.EnsureServerRunning(""); err != nil {
    log.WarningLog.Printf("Failed to ensure tmux server running: %v", err)
}
if err := tmux.CreateKeepaliveSession(""); err != nil {
    log.WarningLog.Printf("Failed to create keepalive session: %v", err)
}
if tmuxKeepServerFlag {
    if err := tmux.SetExitEmpty("", false); err != nil {
        log.WarningLog.Printf("Failed to set tmux exit-empty off: %v", err)
    }
}
```

**Key Finding**: The three functions are called but do not exist. There is no `tmux.go` function implementing these. The code structure suggests they should live in `/Users/tylerstapler/IdeaProjects/claude-squad/session/tmux/tmux.go`, but are missing entirely.

This review rates the **design and intended behavior**, not the implementation (since it doesn't exist yet).

---

## 1. LOGGING: Appropriate Levels?

### Assessment: ✅ **Good**

**Observations**:
- All three startup calls log at `WARNING` level only
- No `INFO` level success logs
- This is appropriate for a local developer tool

**Rationale**:
- INFO logs from startup are typically not actionable (user expects the app to start)
- WARNING is appropriate for "attempted but may not have succeeded" recovery actions
- Tool logs to `~/.stapler-squad/logs/stapler-squad.log` where dev can debug if needed

**Recommendation**: Unchanged. The design is sound.

**Caveat**: If one of these functions throws a panic or gets called in a tight loop (see below), log volume could spike. Consider adding a "skipped (already running)" INFO log if appropriate.

---

## 2. RESILIENCE: Risk of Tight Loop?

### Assessment: 🔴 **Real Risk**

**The Problem**:

1. **`DoesSessionExist()` is called every few seconds** by:
   - `session/instance.go` → `ReviewQueuePoller` (checks session status in loop)
   - `session/tmux_process_manager.go` → periodically queried by service handlers
   - Any other polling logic

2. **If the proposed recovery calls `DoesSessionExist()` internally**:
   ```
   startup: EnsureServerRunning() 
            ↓
            DoesSessionExist() [to verify it's running] — cache is 500ms TTL
            ↓
   3 seconds later: DoesSessionExist() cache expires
            ↓
   Another poller calls DoesSessionExist()
            ↓
   Circuit breaker on "tmux list-sessions" hits 3 failures
            ↓
   Recovery logic detects ErrCircuitOpen
            ↓
   Calls EnsureServerRunning() again
            ↓
   Rapid loop every 500ms (cache TTL)
   ```

3. **Logging output in tight loop**:
   - Each cycle logs `WarningLog.Printf("Failed to ensure tmux server...")` (50+ times per second)
   - Log file rapidly fills with identical messages
   - User sees no useful debugging info

**Mitigation Strategies** (if functions are implemented):

1. **Add debounce/backoff**:
   ```go
   var lastRecoveryAttempt time.Time
   var recoveryBackoff = 5 * time.Second
   
   if err != nil && time.Since(lastRecoveryAttempt) > recoveryBackoff {
       if err := tmux.EnsureServerRunning(""); err != nil {
           // log only if backoff permits
       }
       lastRecoveryAttempt = time.Now()
   }
   ```

2. **Idempotent recovery**:
   - `EnsureServerRunning()` must be idempotent (calling twice in rapid succession should be safe)
   - Circuit breaker should not trip on a second call that confirms server is already running

3. **Don't call from `DoesSessionExist()`**:
   - Separate recovery logic from existence checks
   - Recovery triggered by explicit circuit breaker detection, not from within polling

### Recommendation:

**Before implementation**, add:
- Explicit backoff/debounce to prevent rapid retry cycles
- Document that recovery is NOT called from hot polling paths
- Add a global `recoveryAttemptTime` to rate-limit to once per 10 seconds minimum

**Current Risk Level**: Medium → Low after mitigation

---

## 3. STARTUP ORDERING: Correct Sequence?

### Assessment: ✅ **Correct**

**Current Order** (from `main.go` lines ~190-202):
```go
1. EnsureServerRunning()
2. CreateKeepaliveSession()
3. SetExitEmpty() [if flag]
4. srv := server.NewServer(address)  // not shown in snippet
5. return srv.Start(ctx)
```

**Analysis**:
- `EnsureServerRunning()` before sessions are created ✅ (ensures server exists)
- `CreateKeepaliveSession()` before `srv.Start()` ✅ (app is ready but before serving)
- `SetExitEmpty()` before `srv.Start()` ✅ (tmux config before operations begin)
- All three before session loading ✅ (good — session checks won't find stale server)

**Why This Ordering Works**:
1. Tmux server is guaranteed to exist before any operation attempts to use it
2. Keepalive session exists so server won't auto-exit if all user sessions close
3. Exit-empty setting configured before any sessions are created
4. Session pollers (ReviewQueuePoller, etc.) can assume server is healthy

### Recommendation: **No changes. Ordering is sound.**

---

## 4. FAILURE TOLERANCE: Is "Silent Failure" OK?

### Assessment: ✅ **Good for Local Tool**

**Current Behavior**:
- All three startup calls log warnings but don't return errors
- Application continues even if all three fail
- Sessions can still be created and managed (with degraded resilience)

**Why This Is OK**:
1. **Local tool context**: stapler-squad is developer-facing, not production infrastructure
2. **Graceful degradation**: If tmux server doesn't auto-restart, user can restart it manually
3. **Observable via UI**: Broken tmux operations will show in session status (red status, errors)
4. **No hidden failures**: Users will quickly notice if sessions stop working

**Why This Is NOT OK** (in different context):
- In a cloud service: would need explicit error returns and monitoring alerts
- In a daemon: would need recovery logic or supervisor restart
- In multi-tenant: would need isolation to prevent one failure cascading

### Recommendation: **Unchanged. Appropriate for local tool.**

**However**: Consider adding optional strict mode:
```go
if os.Getenv("STAPLER_SQUAD_STRICT_STARTUP") == "true" {
    if err := tmux.EnsureServerRunning(""); err != nil {
        return fmt.Errorf("failed to ensure tmux server: %w", err)  // Hard fail
    }
}
```

---

## 5. FLAG DOCUMENTATION: Is `--tmux-keep-server` Discoverable?

### Assessment: 🔴 **Missing Documentation**

**Current State**:
- Flag exists in code: `if tmuxKeepServerFlag {` (line ~43 in main.go)
- But NOT defined in `init()` function with `rootCmd.Flags().BoolVar()`
- Not exposed to users via `--help`

**Impact**:
- Users cannot discover the flag
- Cannot tune `exit-empty` setting without modifying code
- `SetExitEmpty(_, false)` call won't execute unless user hardcodes it

**What's Missing**:
```go
// This should be in init() function but ISN'T:
rootCmd.Flags().BoolVar(&tmuxKeepServerFlag, "tmux-keep-server", false,
    "Prevent tmux server from exiting when all sessions are closed (sets exit-empty off)")
```

### Recommendation:

**Critical gap**: Before shipping, add flag definition:
```go
func init() {
    // ... existing flags ...
    rootCmd.Flags().BoolVar(&tmuxKeepServerFlag, "tmux-keep-server", false,
        "Keep tmux server running (sets exit-empty off). Useful if tmux server "+
            "frequently crashes due to load or memory pressure.")
}
```

Then update startup help text or docs to explain when to use it.

---

## 6. Circuit Breaker Integration

### Assessment: ✅ **Well-Designed**

**Existing Infrastructure**:
- `CircuitBreakerExecutor` exists and wraps tmux operations (lines 124-128 in `session/tmux/tmux.go`)
- Returns `ErrCircuitOpen` when breaker is open (see `executor/circuit_breaker.go`)
- Per-command-class tracking (e.g., "tmux-list-sessions", "tmux-capture-pane")

**Recovery Integration Point**:
```go
// In DoesSessionExist or polling loop, detect:
if err == executor.ErrCircuitOpen {
    // Trigger recovery: EnsureServerRunning()
}
```

**Advantage of This Approach**:
- Not a tight polling loop — only recovers when circuit actually trips
- Observability: can inspect circuit breaker state via debug endpoint
- Per-command isolation: "tmux list-sessions" failure doesn't affect "tmux attach-session"

**Existing Circuit Breaker State** (from `executor/circuit_breaker.go`):
- Failure threshold: 3 consecutive failures
- Recovery timeout: 30 seconds
- Half-open probe mechanism prevents thundering herd on recovery

### Recommendation: **Excellent. Build recovery on top of this.**

Design pattern:
```go
func (t *TmuxSession) DoesSessionExist() bool {
    err := t.cmdExec.Run(cmd)  // May return ErrCircuitOpen
    if err == executor.ErrCircuitOpen {
        go t.attemptRecovery()  // Non-blocking recovery attempt
        return false
    }
    // ... normal logic
}

func (t *TmuxSession) attemptRecovery() {
    tmux.EnsureServerRunning()  // With debounce applied
}
```

---

## 7. Observability / Debug Endpoint

### Assessment: ✅ **Circuit Breaker State Exposed**

**Existing Capability**:
- `CircuitBreakerExecutor.AllBreakers()` method returns snapshot
- State includes: `CircuitState`, `ConsecutiveFailures`, `LastStateChange`, `Config`
- Could be exposed via HTTP debug endpoint

**Current Usage**:
- HTTP profiling endpoint at `:6060/debug/pprof/` (when `--profile` flag set)
- Could add new endpoint: `/debug/circuit-breakers` to show tmux executor state

**Recommendation**: **Optional enhancement**
```go
// Add to profiling.go or server/debug.go
http.HandleFunc("/debug/circuit-breakers", func(w http.ResponseWriter, r *http.Request) {
    breakers := executor.GetGlobalRegistry().CircuitBreakerState()
    json.NewEncoder(w).Encode(breakers)
})
```

This would let users diagnose tmux server health without logs.

---

## Summary: Gap Analysis

| Aspect | Status | Impact | Action |
|--------|--------|--------|--------|
| Logging levels | ✅ OK | Low | None |
| Tight loop risk | 🔴 Real | Medium | Add debounce before implementation |
| Startup ordering | ✅ OK | Low | None |
| Silent failures | ✅ OK | Low | Optional: strict mode flag |
| Flag documentation | 🔴 Missing | Medium | Add `--tmux-keep-server` flag definition + docs |
| Circuit breaker design | ✅ OK | Low | None |
| Observability | ✅ OK | Low | Optional: `/debug/circuit-breakers` endpoint |

---

## Recommendations (Prioritized)

### 🔴 **BLOCKER** — Must fix before shipping:
1. **Add `--tmux-keep-server` flag definition** in `init()`
   - Currently referenced but never declared
   - Users cannot enable it

### 🟡 **MUST DO** — Before implementation:
2. **Implement debounce/backoff** in recovery attempt path
   - Prevent rapid retry cycles if server keeps dying
   - Minimum 5–10 second gap between recovery attempts

3. **Document recovery trigger point**
   - Show where `ErrCircuitOpen` is detected
   - Clarify that recovery is NOT called from `DoesSessionExist()` itself

### 🟢 **NICE TO HAVE** — After shipping:
4. **Add `/debug/circuit-breakers` endpoint**
   - Expose circuit breaker state for user debugging
   - Help diagnose "why did tmux server become unhealthy?"

5. **Optional strict startup mode**
   - `STAPLER_SQUAD_STRICT_STARTUP=true` hard-fails on tmux init errors

---

## Implementation Checklist (When Functions Are Built)

- [ ] `EnsureServerRunning("")` — Start tmux server if not running, idempotent
- [ ] `CreateKeepaliveSession("")` — Create session that keeps server alive
- [ ] `SetExitEmpty("", false)` — Configure tmux `set-option exit-empty off`
- [ ] All three callable from startup sequence in `main.go`
- [ ] Add debounce global state + recovery backoff logic
- [ ] Define `--tmux-keep-server` flag in `init()`
- [ ] Update CLAUDE.md with flag usage and when to use it
- [ ] Test with manual tmux server kill during app runtime
- [ ] Verify no tight loops in logs under stress
- [ ] Confirm circuit breaker state is observable

---

## Verdict: 🟡 Production-Ready Once Gaps Fixed

**Current State**: Design is sound; implementation is incomplete.

**Shipping Blocker**: Missing `--tmux-keep-server` flag definition.

**Resilience Risk**: Mitigated if debounce is added before implementation.

**Local Tool Context**: Appropriate failure handling for a developer tool; would not pass cloud SaaS review, but perfectly acceptable for localhost usage.

**Recommendation**: ✅ **Approve design. Require two fixes before shipping.**

