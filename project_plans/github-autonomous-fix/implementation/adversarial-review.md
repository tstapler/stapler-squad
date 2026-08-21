# Adversarial Review: GitHub Autonomous Fix Implementation Plan

**Reviewer role**: Find BLOCKED issues — things that cannot work as designed, not style nitpicks.  
**Date**: 2026-06-09  
**Plan under review**: `project_plans/github-autonomous-fix/implementation/plan.md`

---

## Verdict: CONCERNS (no hard BLOCKEDs, but 4 issues require plan patches)

No issue in isolation prevents the feature from shipping, but items C1 and C2 are architectural near-BLOCKEDs that will cause silent failures if not addressed before implementation begins.

---

## C1 — NEAR-BLOCKED: `SendCommandImmediate` is not safe during `StatusChangeListener` callback

**Severity**: High (will cause PTY corruption in production)

**Problem**: TD-2 says the driver calls `cc.SendCommandImmediate(nextMsg)` after confirming `IdleStateWaiting` from the `StatusChangeListener` callback. But `runStatusChangeLoop` fires the listener **from its own goroutine** while holding no lock. `SendCommandImmediate` internally calls `executor.ExecuteImmediate`, which writes to the PTY via `WriteToPTY`. If any pending queued command is being flushed at the same time (the `CommandQueue` → `CommandExecutor` path runs on a separate goroutine), the PTY bytes are interleaved.

The plan says "driver always waits for `IdleStateWaiting` before calling" but the plan does NOT specify that the driver posts the send to a new goroutine or uses a dedicated channel. Calling `SendCommandImmediate` synchronously inside the status-change fan-out callback will block the `runStatusChangeLoop` goroutine for up to 30 seconds (the timeout), starving all other listeners of status updates.

**Fix required in plan**: T2.1.2's `run` loop must use a **channel signal** pattern, not a direct in-callback call:
1. The `onStatusChange` listener registered by `AutonomousDriver` sends to a `chan detection.DetectedStatus` (capacity 1, non-blocking)
2. The `run` goroutine selects on that channel
3. `SendCommandImmediate` is called from the `run` goroutine, not from the listener

This is consistent with how `runStatusChangeLoop` itself works (it uses a channel `statusCheckCh` rather than calling handlers inline from PTY writes). The plan describes the overall structure correctly but is ambiguous enough that an implementer could make the wrong call.

**Plan patch applied below (C1)**.

---

## C2 — NEAR-BLOCKED: `isAutonomousSession` in `ApprovalHandler` requires storage access that creates a circular dependency

**Severity**: High (architectural violation)

**Problem**: T5.1.1 says `ApprovalHandler.isAutonomousSession(sessionID)` looks up the instance in storage and reads `AutonomousMode`. The `ApprovalHandler` lives in `server/services/` and currently has **no reference to the session storage or `SessionService`**. It receives only the HTTP request context.

Adding a storage reference to `ApprovalHandler` would create a dependency cycle: `session_service.go` creates `ApprovalHandler`, and `ApprovalHandler` would then depend on the session store that `SessionService` manages — not a Go import cycle, but a construction-time circular dependency (who sets whom first).

The approval handler fires *before* the session is fully started in some edge cases (the pitfalls research notes the Session-ID-resolution hazard). If the instance isn't in storage yet, `isAutonomousSession` returns `false` and the LLM path is silently skipped with no log.

**Fix required in plan**: Instead of looking up storage, have the `ApprovalHandler` accept a `isAutonomous func(sessionID string) bool` injector at construction time (or `SetAutonomousChecker(fn func(string) bool)`). `SessionService` provides this as a closure over its own instance cache. This is already the pattern for `SetClassifier`. No circular dependency; the lookup happens at call time, not construction time.

**Plan patch applied below (C2)**.

---

## C3 — CONCERN: Headless pool `FeatureKeyAutonomousFix` sessions are scoped by `subKey = sessionID` but the pool may reuse a stale conversation

**Severity**: Medium (correctness issue, not a crash)

**Problem**: T2.1.3 calls `headlessPool.CallBlockingWithOptions(ctx, FeatureKeyAutonomousFix, sessionID, prompt, opts)`. The headless pool uses `(featureKey, subKey)` to identify a session state, and it **resumes** that session on subsequent calls (same conversation). This means every orchestration turn is a multi-turn conversation with the headless LLM — which is actually good for context.

However: if the autonomous session is restarted (e.g., hibernated + resumed), the headless pool session for `(FeatureKeyAutonomousFix, oldSessionID)` is stale. The new session has a new `sessionID`, so the pool creates a new conversation — this is correct. But if the server restarts while the headless pool state is in-memory only, the pool session is lost and the next `CallBlocking` starts a fresh conversation — also probably fine.

The real risk: the headless pool has a `keyMu map[FeatureKey]*sync.Mutex` that serializes calls per `FeatureKey`, not per `(FeatureKey, subKey)`. If two autonomous sessions run concurrently (which the requirements say is out of scope for MVP, but will happen in practice), their orchestration calls will serialize on the same mutex. This creates a hidden throughput bottleneck that is not mentioned in the plan.

**Fix required in plan**: Add a note in T2.1.3 that calls into the pool use `FeatureKeyAutonomousFix + "-" + sessionID` as the effective key (if the pool supports arbitrary string keys), OR the `FeatureKeyAutonomousFix` is per-session-scoped. Check pool implementation to confirm locking granularity. If `keyMu` is keyed only by `FeatureKey` (not subKey), escalate to research before implementation.

**Plan patch applied below (C3)**.

---

## C4 — CONCERN: `ExtractPRURL` regex fires on GitHub PR URL mentioned in the *initial prompt*, not just created PRs

**Severity**: Low-medium (produces false positive artifacts)

**Problem**: T6.1.1 scans the full session scrollback for `https://github.com/.../pull/\d+`. If the initial goal prompt includes the GitHub issue URL (which references a PR for context), or if the session is fixing CI on an existing PR (whose URL is in the prompt), `ExtractPRURL` will return the *input* PR URL, not a newly-created one.

**Fix required in plan**: `ExtractPRURL` should scan only the **last N lines** of output (e.g., the last 200 lines), or look specifically for Claude output patterns that indicate a newly created PR (e.g., "Created PR:", "Opened pull request", "gh pr create" output format). Alternatively, skip lines that appear in the initial prompt by storing the prompt character offset and only scanning after it.

**Plan patch applied below (C4)**.

---

## Minor Observations (not plan-blocking)

- **TD-3 note**: The plan correctly overrides the pitfalls.md suggestion to use `bypassPermissions`. This is the right call per requirements. However, `PermissionMode: "auto"` may not be sufficient for fully-automated sessions because "auto" mode still triggers Claude Code's internal reasoning loop per tool call, which is slower than `bypassPermissions`. Consider documenting this performance tradeoff in plan comments.

- **E7/T7.1.1**: The Playwright test "verifies session eventually exits (OneShot completes)" has a non-deterministic completion time — Claude's response time varies. The test needs a generous timeout (`timeout: 120_000`) and must wait for a `data-testid="session-status-exited"` or equivalent stable locator. The plan doesn't specify this.

- **S3.2/T3.2.2**: The plan wires the driver "after `wireStatusChangeCallback`" but the dependency table (sequencing overview) doesn't show this creates a potential issue: if `CreateDirectorySession` returns an error after `driver.Start()` but before `AddInstance`, the driver is running against an instance that was never persisted. Add a cleanup call `driver.Stop()` in the error path.

---

## Plan Patches

### Patch C1: `run` loop uses channel signal, not inline callback

In **T2.1.2**, replace:
> "Register `onStatusChange` via fan-out listener; when status transitions to `IdleStateWaiting` or `StatusSuccess`, call `cc.SendCommandImmediate`"

With:
> "Register `onStatusChange` listener that sends `status` to a `chan detection.DetectedStatus` (capacity 1, drop if full). The `run` goroutine selects on this channel. Only `run` calls `cc.SendCommandImmediate` — never the listener closure."

### Patch C2: `ApprovalHandler` uses injected checker, not storage lookup

In **T5.1.1**, replace:
> "check `isAutonomousSession(sessionID)` (look up instance in storage, read `AutonomousMode` field)"

With:
> "check `h.autonomousChecker(sessionID)` — a `func(string) bool` injected via `SetAutonomousChecker`. `SessionService` provides the closure: `func(id string) bool { inst, _ := s.storage.Get(id); return inst != nil && inst.AutonomousMode }`"

Add **T5.1.5**: Wire `SetAutonomousChecker` in `server.go` after both `SessionService` and `ApprovalHandler` are constructed.

### Patch C3: Headless pool per-session locking investigation

In **T2.1.3**, add:
> "Before implementation, verify `headless.Pool` keyMu granularity. If locking is per-FeatureKey (not per-subKey), register each autonomous session under a unique key `FeatureKeyAutonomousFix + "-" + sessionID[0:8]` to avoid serializing concurrent sessions."

### Patch C4: `ExtractPRURL` scans only tail, not full output

In **T6.1.1**, replace:
> "`ExtractPRURL(sessionOutput string) string` — scans full output"

With:
> "`ExtractPRURL(sessionOutput string) string` — scans the last 200 lines only (to avoid matching the input URL from the initial prompt). Implementation: `lines := strings.Split(output, "\n"); tail := lines[max(0, len(lines)-200):]`"

---

## Final Verdict After Patches

With the 4 patches applied to `plan.md`, all identified blockers are resolved. The plan is **CLEAN** to proceed to Phase 5 (Implementation).

**Remaining risk areas to monitor during implementation**:
1. Headless pool locking granularity (C3 — verify during T2.1.3)
2. E7/T7.1.1 test timing (add explicit timeouts in Playwright spec)
3. T3.2.2 error-path driver.Stop() (add cleanup on CreateDirectorySession failure)
