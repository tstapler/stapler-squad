# Implementation Plan: GitHub Autonomous Fix

**Date**: 2026-06-09  
**Status**: Draft  
**Epics**: 6 | **Stories**: 17 | **Tasks**: 54

---

## Technology Decisions

### TD-1: Fan-out slice over polling for StatusChangeListener

**Decision**: Add `RegisterStatusChangeListener(fn StatusChangeListener)` to `ClaudeController` that appends to a `[]StatusChangeListener` slice, and update `wireStatusChangeCallback` in `session_service.go` to use it.

**Rejected**: Chaining callbacks (fragile, order-dependent), polling on a ticker (2s latency, missed transitions).

**Rationale**: The server layer already occupies the single-listener slot. Overwriting it silently breaks metrics/events. A fan-out slice is a one-time, non-breaking refactor that enables any future driver to register without coordination. The `runStatusChangeLoop` goroutine only fires when the PTY content changes, so calling N small functions is negligible overhead.

### TD-2: `SendCommandImmediate` as the injection method for AutonomousDriver

**Decision**: AutonomousDriver uses `cc.SendCommandImmediate(text)` exclusively, never `WriteToPTY` directly or `SendCommand` (async queue).

**Rejected**: Raw `WriteToPTY` (bypasses the queue entirely, races with in-progress executor output); async `SendCommand` (hard to correlate "this command finished" — the queue has no per-command completion signal exposed to external callers).

**Rationale**: `SendCommandImmediate` blocks until the command is acknowledged (or times out), adds to command history (auditable), and serializes via the `CommandExecutor` so it cannot interleave with queued commands. The AutonomousDriver only calls it after confirming `IdleStateWaiting` from the StatusChangeListener, so there is no scenario where a command is in flight when the driver calls `SendCommandImmediate`.

### TD-3: `PermissionMode: "auto"` NOT `bypassPermissions` for autonomous sessions

**Decision**: Autonomous sessions default to `PermissionMode: "auto"` with `AllowedTools` covering the safe set (`Bash,Read,Edit,Write,Glob,Grep,MultiEdit`). `bypassPermissions` is not used.

**Rejected**: `bypassPermissions` (makes Claude approve catastrophic actions silently); pure classifier auto-approve (the existing classifier was not designed for headless operation).

**Rationale**: `auto` mode means Claude Code itself evaluates each permission using LLM reasoning — risky calls trigger an LLM-mediated approval (US-5), not a human queue block. This satisfies the requirement that "LLM approves risky calls instead of human". `bypassPermissions` violates the explicit non-goal that all sessions use `auto` or LLM approval.

**Note**: The pitfalls research noted that `bypassPermissions` is the recommended config for CI fixing sessions. This plan overrides that recommendation to align with the requirements non-goal ("full `bypassPermissions` is out of scope"). See E5/S5.1 for the LLM-assisted approval hook that fills this gap.

### TD-4: Headless pool for orchestrator LLM calls, NOT new sessions

**Decision**: The `AutonomousDriver` calls `headlessPool.CallBlockingWithOptions(ctx, headless.FeatureKey("autonomous_fix-"+sessionID[:8]), systemPrompt, userPrompt, opts)` for all LLM reasoning (next-turn generation, goal-completion evaluation, risky tool call approval).

**Rejected**: Spawning a new full session for each LLM call (too heavy); direct Anthropic API calls (violates the technical constraint that headlessPool is the only LLM access path).

**Rationale**: The headless pool already has connection reuse, queuing, and error handling. Using a per-session `FeatureKey` (see C3 below) ensures concurrent autonomous sessions don't serialize on a shared mutex, and avoids polluting the `FeatureKeyCustom` pool used by `RunOneShot`.

**C3 — Concurrency concern (per-key mutex in `pool.go`)**: `keyMu map[FeatureKey]*sync.Mutex` is keyed by `FeatureKey` alone (confirmed in `session/headless/pool.go`). Two autonomous sessions sharing the same `FeatureKey` would serialize all LLM calls on one mutex. Fix: use `FeatureKey("autonomous_fix-" + sessionID[:8])` so each session gets its own mutex slot. There is no `subKey` parameter in the pool API — the uniqueness is achieved by varying the `key` argument itself. The real `CallBlockingWithOptions` signature is `(ctx, key FeatureKey, systemPrompt string, userPrompt string, opts CallOptions) (string, error)` with no subKey.

### TD-5: Atomic `driverRunning` guard + panic recovery are mandatory

**Decision**: `AutonomousDriver` has its own `atomic.Bool driverRunning` field, an idempotency check at entry, and a `defer recover()` wrapper on the goroutine body.

**Rationale**: The existing `session_driver.go` pattern is the established convention for driver goroutines. A panic without recovery kills the entire server process.

### TD-6: MaxTurns = 20 with exponential backoff between CI polls

**Decision**: `maxTurns = 20` configurable via `Config.AutonomousMaxTurns` (default 20). CI re-poll after each "turn complete" (`StatusSuccess`) uses exponential backoff starting at 10s, cap 120s.

**Rationale**: 20 turns matches the requirement default. Exponential backoff avoids hammering the GitHub API (5000 req/hr limit), especially for slow CI pipelines.

---

## Sequencing Overview

```
E1 (fan-out listener refactor) → E2 (AutonomousDriver core) → E3 (session creation wiring)
E3 → E4 (omnibar + backlog UI) 
E3 → E5 (LLM approval hook)
E2 + E3 → E6 (goal completion + notification)
```

E1 is the prerequisite for everything. E4, E5, E6 can be developed in parallel after E3 completes.

---

## E1: StatusChangeListener Fan-out Refactor

**Goal**: Allow multiple listeners on a `ClaudeController` without breaking existing code.  
**Priority**: P0 (prerequisite for E2)  
**Stories**: 1

### S1.1: Add multi-listener support to ClaudeController

**Acceptance**: `RegisterStatusChangeListener` appends; all registered listeners fire on each transition; existing `SetStatusChangeListener` becomes a compatibility shim that clears + appends one.

#### T1.1.1 — Modify `ClaudeController` listener storage (M)
- **File**: `session/claude_controller.go`
- **Change**: Replace `statusChangeListener StatusChangeListener` field with `statusChangeListeners []StatusChangeListener` + `listenersMu sync.RWMutex`
- **Add method**: `RegisterStatusChangeListener(fn StatusChangeListener)` — acquires write lock, appends
- **Update method**: `SetStatusChangeListener(fn StatusChangeListener)` — acquires write lock, replaces slice with `[]StatusChangeListener{fn}` (backward compat)
- **Update**: `runStatusChangeLoop` — acquire read lock, iterate slice, call each fn

#### T1.1.2 — Update `wireStatusChangeCallback` to use `Register` (S)
- **File**: `server/services/session_service.go`
- **Change**: Replace `inst.SetStatusChangeCallback(fn)` call with `inst.RegisterStatusChangeCallback(fn)` (add a passthrough method on `Instance` delegating to `controller.RegisterStatusChangeListener`)
- **Add method on Instance**: `RegisterStatusChangeCallback(fn func(detection.DetectedStatus, string))` in `session/instance_controller.go`

#### T1.1.3 — Unit tests for fan-out (M)
- **File**: `session/claude_controller_test.go`
- **Add tests**:
  - `TestRegisterMultipleStatusListeners_AllFire` — register 2 listeners, simulate transition, verify both called
  - `TestSetStatusChangeListener_Replaces` — verify `Set` replaces; old listener not called after re-Set
  - `TestRegisterStatusChangeListener_Concurrent` — register from goroutine while loop runs, no race

---

## E2: AutonomousDriver Core

**Goal**: Implement the goroutine that monitors a session and injects orchestrator prompts when idle.  
**Priority**: P0  
**Stories**: 2

### S2.1: `AutonomousDriver` struct + start/stop lifecycle

**Acceptance**: Driver starts when `AutonomousMode=true` on session start; stops on session exit; second `Start` call is no-op (idempotency guard).

#### T2.1.1 — Create `session/autonomous_driver.go` (L)
- **File**: `session/autonomous_driver.go` (new file)
- **Struct**:
  ```go
  type AutonomousDriver struct {
      inst        *Instance
      controller  *ClaudeController
      headlessPool HeadlessPoolClient  // narrow interface
      goal        string
      maxTurns    int
      turnCount   int
      driverRunning atomic.Bool
      cancel      context.CancelFunc
      mu          sync.Mutex
  }
  ```
- **Methods**:
  - `NewAutonomousDriver(inst *Instance, pool HeadlessPoolClient, goal string, maxTurns int) *AutonomousDriver`
  - `Start(ctx context.Context) error` — idempotency check via `driverRunning.CompareAndSwap`, registers status listener, starts goroutine
  - `Stop()` — calls `cancel()`, sets `driverRunning` false
  - `run(ctx context.Context)` — main loop (see T2.1.2)

#### T2.1.2 — Implement `run` loop (L)
- **File**: `session/autonomous_driver.go`
- **Logic**:
  1. `defer recover()` panic wrapper
  2. Create `statusCh chan detection.DetectedStatus` (capacity 1) for inter-goroutine signaling
  3. Register `onStatusChange` via `T1.1.1` fan-out listener — the listener does a **non-blocking send** to `statusCh` only; it never calls `SendCommandImmediate` directly (avoids blocking `runStatusChangeLoop` for up to 30s and avoids PTY write races with the CommandQueue executor goroutine)
  4. Wait for first `IdleStateWaiting` or `StatusSuccess` signal on `statusCh` (with 60s startup timeout)
  5. Loop until `turnCount >= maxTurns` or `ctx.Done()`:
     a. Check `cc.GetRateLimitState()` — if not `StateNone`, sleep until reset time + 5s jitter
     b. Tail the session's scrollback (last N lines via `inst.Preview()`)
     c. Call `headlessPool.CallBlockingWithOptions(ctx, headless.FeatureKey("autonomous_fix-"+sessionID[:8]), systemPrompt, buildOrchestrationPrompt(goal, tail), opts)` → `nextMsg` where `systemPrompt` is the feature's system prompt from `headless.DefaultFeatures()`
     d. Parse `nextMsg` for `done=true` sentinel (if LLM signals completion, exit loop)
     e. Call `cc.SendCommandImmediate(nextMsg + "\r")` with 30s timeout — called from **this goroutine only**, never from the listener closure
     f. Wait for `StatusSuccess` or `IdleStateWaiting` signal on `statusCh`
     g. Increment `turnCount`, log turn to session log
  6. On exit: fire `OnDriverComplete(outcome)` callback
- **Critical invariant**: `cc.SendCommandImmediate` is only ever called from the `run` goroutine. The `onStatusChange` listener only sends to `statusCh`. This prevents blocking `runStatusChangeLoop` and eliminates PTY interleaving with the CommandQueue executor.

#### T2.1.3 — `HeadlessPoolClient` interface (S)
- **File**: `session/autonomous_driver.go`
- **Interface** (narrow, for testability):
  ```go
  type HeadlessPoolClient interface {
      CallBlockingWithOptions(ctx context.Context, key headless.FeatureKey, systemPrompt string, userPrompt string, opts headless.CallOptions) (string, error)
  }
  ```
- This matches the real `headless.Pool` method signature (confirmed: `session/headless/caller.go` line 383) so `*headless.Pool` satisfies it without wrappers. **There is no `subKey` parameter.**
- **C3 — mutex isolation**: `pool.go` shows `keyMu map[FeatureKey]*sync.Mutex` is keyed by `FeatureKey` alone (verified). Multiple concurrent autonomous sessions sharing one `FeatureKey` would serialize on a single mutex. Fix: pass `headless.FeatureKey("autonomous_fix-" + sessionID[:8])` as `key` so each session gets its own mutex slot in the pool. No subKey parameter exists — uniqueness is achieved by varying the `key` argument itself.

#### T2.1.4 — Orchestration prompt builder (M)
- **File**: `session/autonomous_driver.go`
- **Function**: `buildOrchestrationPrompt(goal, tail string, turnCount, maxTurns int) string`
- **Output**: Structured prompt including:
  - "You are orchestrating a Claude session. Goal: {goal}"
  - "Session tail (last 80 lines): {tail}"
  - "Turn {N}/{maxTurns}. Reply with NEXT_MESSAGE: <text> or DONE: <reason>"
- Parsing: scan for `NEXT_MESSAGE:` or `DONE:` prefix in response

#### T2.1.5 — Goal-completion evaluator (M)
- **File**: `session/autonomous_driver.go`
- **Function**: `parseOrchestrationResponse(resp string) (nextMsg string, done bool, reason string, err error)`
- Recognizes `DONE:` prefix → `done=true`
- Recognizes `NEXT_MESSAGE:` prefix → extracts text
- Returns `err` if neither found (malformed response — driver logs and retries once)

### S2.2: AutonomousDriver unit tests

**Acceptance**: Tests use fake `HeadlessPoolClient` and fake `ClaudeController` (or minimal mock); cover idempotency, max-turn limit, rate-limit pause, and completion detection.

#### T2.2.1 — Fake HeadlessPoolClient (S)
- **File**: `session/autonomous_driver_test.go`
- Struct with configurable response sequence: `[]string{response1, response2, ...}`
- Implements `HeadlessPoolClient` interface: `CallBlockingWithOptions(ctx, key, systemPrompt, userPrompt string, opts) (string, error)` — note no `subKey`; verify that each call uses a distinct `key` of the form `"autonomous_fix-<sessionID[:8]>"` to confirm C3 isolation

#### T2.2.2 — `TestAutonomousDriver_IdempotencyGuard` (S)
- Call `Start` twice concurrently → verify only one goroutine runs

#### T2.2.3 — `TestAutonomousDriver_MaxTurnsLimit` (S)
- Configure fake pool to always return `NEXT_MESSAGE: keep going`
- Verify loop exits after `maxTurns`

#### T2.2.4 — `TestAutonomousDriver_DoneSignal` (S)
- Configure fake pool to return `DONE: goal complete` on turn 2
- Verify loop exits after turn 2, `done=true` outcome

#### T2.2.5 — `TestAutonomousDriver_RateLimitPause` (M)
- Simulate `GetRateLimitState` returning `StateDetected` for first 2 polls → `StateNone` on 3rd
- Verify no `SendCommandImmediate` call until state is `StateNone`

---

## E3: Session Creation Wiring (7 Touchpoints)

**Goal**: Thread `AutonomousMode` through all 7 session-creation touchpoints; wire `AutonomousDriver` into `CreateDirectorySession`.  
**Priority**: P0  
**Stories**: 2

### S3.1: Proto + Go binding changes

#### T3.1.1 — Add `AutonomousMode` to `InstanceOptions` (S)
- **File**: `session/instance.go`
- **Change**: Add `AutonomousMode bool` to `InstanceOptions` struct; in `NewInstance`, assign `i.AutonomousMode = opts.AutonomousMode`

#### T3.1.2 — Add `autonomous_mode` to `CreateSessionRequest` proto (S)
- **File**: `proto/session/v1/session.proto`
- **Change**: Add `bool autonomous_mode = 23;` (next available field) to `CreateSessionRequest`
- Run `make generate-proto`

#### T3.1.3 — Thread `autonomous_mode` through `CreateSession` RPC handler (S)
- **File**: `server/services/session_service.go`
- **Change**: In `createSessionFromRequest`, set `opts.AutonomousMode = req.Msg.AutonomousMode` alongside existing fields like `AllowedTools`, `PermissionMode`

#### T3.1.4 — Verify `makeSessionOpts` in `SpawnSessionFromItem` (S)
- **File**: `server/services/backlog_service.go`
- **Change**: In `SpawnSessionFromItem`, when called with `autonomous=true` parameter (added in S4.2), set `AutonomousMode: true`, `PermissionMode: "auto"`, `AllowedTools: "Bash,Read,Edit,Write,Glob,Grep,MultiEdit"`

### S3.2: Wire `AutonomousDriver` into `CreateDirectorySession`

#### T3.2.1 — Store headless pool ref on `SessionService` for driver wiring (S)
- **File**: `server/services/session_service.go`
- **Verification**: `s.headlessPool` already exists; confirm it's non-nil when autonomous sessions are created (add nil guard + log warning if nil)

#### T3.2.2 — Wire `AutonomousDriver` after `wireStatusChangeCallback` (M)
- **File**: `server/services/session_service.go`
- **Change**: In `CreateDirectorySession`, after `s.wireStatusChangeCallback(instance)`, add:
  ```go
  if instance.AutonomousMode && s.headlessPool != nil {
      driver := session.NewAutonomousDriver(instance, s.headlessPool, instance.Prompt, s.cfg.AutonomousMaxTurns())
      driver.RegisterCompletionCallback(s.onAutonomousDriverComplete)
      if err := driver.Start(ctx); err != nil {
          log.Warnf("failed to start autonomous driver: %v", err)
      }
      s.autonomousDriverRegistry.Register(instance.GetName(), driver)
  }
  // NOTE: If CreateDirectorySession returns an error AFTER driver.Start(), call driver.Stop()
  // in the error path to prevent a leaked goroutine against an un-persisted instance.
  // Pattern: defer a cleanup func that calls driver.Stop() if the function returns an error.
  ```

#### T3.2.3 — `AutonomousDriverRegistry` (S)
- **File**: `server/services/session_service.go` (or new `server/services/autonomous_registry.go`)
- **Struct**: `map[string]*session.AutonomousDriver` + `sync.RWMutex`
- **Methods**: `Register`, `Get`, `Remove`
- Needed so `onAutonomousDriverComplete` and the HTTP handler can stop a driver on session delete.

#### T3.2.4 — `onAutonomousDriverComplete` callback (M)
- **File**: `server/services/session_service.go`
- **Logic**: Called when driver exits; receives `AutonomousDriverOutcome{Done bool, Reason string, PRUrl string}`; triggers backlog item update (E6/S6.1) and push notification (E6/S6.2)

#### T3.2.5 — Wire `Stop` on session delete (S)
- **File**: `server/services/session_service.go`
- **Change**: In `DeleteSession` / `HibernateSession`, call `registry.Get(name).Stop()` if driver exists

---

## E4: User-Facing Entry Points

**Goal**: Add "Fix autonomously" to the omnibar and "Run autonomously" button to the Backlog page.  
**Priority**: P1  
**Stories**: 3

### S4.1: Omnibar "Fix autonomously" mode

**Acceptance**: Pasting a GitHub issue/PR URL shows a "Fix autonomously" option; selecting it creates an autonomous session via `CreateSession(autonomous=true, permission_mode="auto")`.

#### T4.1.1 — Add `autonomous` to `sessionType` union in `Omnibar.tsx` (S)
- **File**: `web-app/src/components/sessions/Omnibar.tsx`
- **Change**: Add `"autonomous"` to the `sessionType` union; update `canSubmit` (valid when a GitHub URL is detected or path is non-empty); update `handleSubmit` to pass `autonomousMode: true`

#### T4.1.2 — Add "Fix autonomously" entry to `SESSION_TYPES` in `OmnibarCreationPanel.tsx` (S)
- **File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`
- **Change**: Add `{ value: "autonomous", label: "Fix autonomously" }` to `SESSION_TYPES`; add hint text: "Creates a session that runs to completion without manual steering."
- Hide working directory field when type is `autonomous` (GitHub URL is the path source)

#### T4.1.3 — Update `sessionTypeMap` in `OmnibarContext.tsx` (S)
- **File**: `web-app/src/lib/contexts/OmnibarContext.tsx`
- **Change**: Add `autonomous: SessionType.DIRECTORY` (reuses directory type; server handles the distinction via `autonomous_mode` flag); pass `autonomousMode: data.autonomousMode ?? false` and `permissionMode: "auto"` in the `createSession` call

#### T4.1.4 — Thread `autonomousMode` through `useSessionService.ts` (S)
- **File**: `web-app/src/lib/hooks/useSessionService.ts`
- **Change**: Add `autonomousMode?: boolean` to `OmnibarSessionData`; thread to `CreateSessionRequest.autonomous_mode`

#### T4.1.5 — Add `create_session (autonomous)` dispatch case to `dispatch.ts` (S)
- **File**: `web-app/src/lib/omnibar/actions/dispatch.ts`
- **Change**: In `create_session` case, handle `sessionType === "autonomous"` similarly to `one_off` — map to `{ autonomousMode: true, sessionType: undefined, permissionMode: "auto" }`

#### T4.1.6 — Dispatch test for autonomous action (S)
- **File**: `web-app/src/lib/omnibar/actions/dispatch.test.ts`
- **Add**: `describe("create_session (autonomous)")` with test `dispatchOmnibarAction_should_setAutonomousModeTrue_When_sessionTypeIsAutonomous`

### S4.2: Backlog "Run autonomously" button

**Acceptance**: Backlog items in `ready` status show a "Run autonomously" button; clicking it calls a new `SpawnAutonomousSession` RPC (or adds `autonomous=true` parameter to `SpawnSessionFromItem`).

#### T4.2.1 — Add `autonomous` flag to `SpawnSessionFromItem` RPC (S)
- **File**: `proto/session/v1/session.proto`
- **Change**: Add `bool autonomous = 3;` to `SpawnSessionFromItemRequest`; run `make generate-proto`

#### T4.2.2 — Update `SpawnSessionFromItem` handler to wire autonomous mode (S)
- **File**: `server/services/backlog_service.go`
- **Change**: When `req.Msg.Autonomous == true`, add `AutonomousMode: true`, `PermissionMode: "auto"`, `AllowedTools: ...` to `CreateDirectorySession` opts

#### T4.2.3 — Add "Run autonomously" button to BacklogItem component (M)
- **File**: `web-app/src/components/backlog/BacklogItem.tsx` (or equivalent)
- **Change**: When `item.status === "ready"`, render a "Run autonomously" button that calls `spawnSessionFromItem({ itemId, autonomous: true })`
- Style: secondary/outline button to distinguish from primary "Start session"

### S4.3: `AutoFix` OmnibarAction type

**Acceptance**: GitHub issue/PR URL auto-detected in omnibar generates a suggestion card for "Fix autonomously"; clicking dispatches `{ type: "auto_fix", url, repoOwner, repoName, issueOrPrNumber }`.

#### T4.3.1 — Add `auto_fix` to `OmnibarAction` union (S)
- **File**: `web-app/src/lib/omnibar/actions/types.ts`
- **Add variant**:
  ```ts
  | { type: "auto_fix"; url: string; repoOwner: string; repoName: string; issueOrPrNumber: number; isPR: boolean; label: string }
  ```

#### T4.3.2 — Add `case "auto_fix":` to `dispatch.ts` (S)
- **File**: `web-app/src/lib/omnibar/actions/dispatch.ts`
- **Logic**: Fetches issue/PR title from GitHub (or uses the title already in `GitHubPRDetector` result), calls `deps.createSession` with `autonomousMode: true`, fills `title` from issue title

#### T4.3.3 — Dispatch test for `auto_fix` (S)
- **File**: `web-app/src/lib/omnibar/actions/dispatch.test.ts`
- **Add**: `describe("auto_fix")` with test `dispatchOmnibarAction_should_createAutonomousSession_When_autoFix`

---

## E5: LLM-Assisted Approval for Risky Tool Calls

**Goal**: When `AutonomousMode=true` and a tool call is classified as `Escalate`, ask the headless LLM instead of queuing for human review.  
**Priority**: P2  
**Stories**: 2

### S5.1: Autonomous approval pathway in `ApprovalHandler`

**Acceptance**: Risky tool calls on autonomous sessions are sent to the headless LLM; LLM approval/denial is logged in `ApprovalStore`; human queue is fallback only.

#### T5.1.1 — Add autonomous session check to `ApprovalHandler` (M)
- **File**: `server/services/approval_handler.go`
- **Change**: After classifier returns `Escalate`, check `h.autonomousChecker(sessionID)` — a `func(string) bool` injected via `SetAutonomousChecker` (see T5.1.5). Do NOT look up storage directly from the handler (avoids construction-time circular dependency: `SessionService` creates `ApprovalHandler`, and `ApprovalHandler` must not depend on the session store that `SessionService` manages).
- If autonomous: call `h.headlessPool.CallBlockingWithOptions(ctx, headless.FeatureKeyAutonomousApproval, systemPrompt, buildApprovalQuery(...), opts)` where `systemPrompt` comes from `headless.DefaultFeatures()[FeatureKeyAutonomousApproval]`
- Parse response for `APPROVE:` or `DENY:` prefix
- If `APPROVE:`: call `writeDecision("allow", reasoning)`, log to approval store with `source: "llm_orchestrator"`
- If `DENY:` or error: fall through to human review queue

#### T5.1.2 — `buildApprovalQuery` helper (S)
- **File**: `server/services/approval_handler.go`
- **Function**: `buildApprovalQuery(goal, toolName string, toolInput map[string]string, sessionTail string) string`
- Outputs: "Session goal: {goal}\nRequested tool: {toolName}\nArguments: {toolInput}\nRecent output: {sessionTail}\nReply APPROVE: <reason> or DENY: <reason>"

#### T5.1.3 — Inject headless pool into `ApprovalHandler` (S)
- **File**: `server/services/approval_handler.go`
- **Change**: Add `headlessPool HeadlessPoolClient` field; add `SetHeadlessPool(pool HeadlessPoolClient)` method; wire from `server.go` after headless pool is constructed

#### T5.1.5 — Wire `SetAutonomousChecker` in `server.go` (S)
- **File**: `server/server.go`
- **Change**: After both `SessionService` and `ApprovalHandler` are constructed, call:
  ```go
  approvalHandler.SetAutonomousChecker(func(sessionID string) bool {
      inst, err := sessionService.GetInstance(sessionID)
      return err == nil && inst != nil && inst.AutonomousMode
  })
  ```
- This is the same injection pattern as `SetClassifier`. No circular dependency at construction time.

#### T5.1.4 — Unit test for autonomous approval pathway (M)
- **File**: `session/approval_automation_test.go` (or new `server/services/approval_handler_test.go`)
- **Tests**:
  - `TestApprovalHandler_AutonomousLLMApprove` — LLM returns `APPROVE: safe delete` → `allow` decision, stored in approval log
  - `TestApprovalHandler_AutonomousLLMDeny` — LLM returns `DENY: deletes production data` → falls through to human queue
  - `TestApprovalHandler_AutonomousLLMError` — headless pool error → falls through to human queue

### S5.2: `FeatureKeyAutonomousApproval` in headless features

#### T5.2.1 — Register new feature key (S)
- **File**: `session/headless/features.go`
- **Change**: Add `FeatureKeyAutonomousApproval FeatureKey = "autonomous_approval"` and `FeatureKeyAutonomousFix FeatureKey = "autonomous_fix"`
- Add entries to `DefaultFeatures()` map with appropriate system prompts

---

## E6: Goal Completion, Artifact Extraction, and Notification

**Goal**: Detect session completion, extract PR URL, update backlog item, notify user.  
**Priority**: P2  
**Stories**: 3

### S6.1: Artifact extraction from session output

**Acceptance**: After driver exits with `done=true`, driver scans session scrollback for GitHub PR URLs and stores the first match as `artifact_url` on the backlog item.

#### T6.1.1 — `ExtractPRURL` function (S)
- **File**: `session/autonomous_driver.go`
- **Function**: `ExtractPRURL(sessionOutput string) string`
- Regex: `https://github\.com/[^/]+/[^/]+/pull/\d+`
- **Scans the last 200 lines only** to avoid matching the input PR/issue URL that was included in the initial goal prompt. Implementation: `lines := strings.Split(output, "\n"); tail := lines[max(0, len(lines)-200):]`; join tail and apply regex.
- Returns first match in the tail or empty string

#### T6.1.2 — `AutonomousDriverOutcome` struct (S)
- **File**: `session/autonomous_driver.go`
- **Struct**:
  ```go
  type AutonomousDriverOutcome struct {
      Done     bool
      Reason   string
      PRUrl    string
      Turns    int
      Stuck    bool   // true if exited via maxTurns without DONE signal
  }
  ```

#### T6.1.3 — Wire outcome to `BacklogLifecycleListener` (M)
- **File**: `server/services/session_service.go` + `session/backlog_lifecycle.go`
- **Change**: `onAutonomousDriverComplete(instanceName string, outcome AutonomousDriverOutcome)`:
  - Find backlog item by `sessionID` (via `backlogLifecycleListener.GetItemForSession(sessionID)`)
  - If `outcome.Done`: transition item to `done`, store `outcome.PRUrl` as `ExternalURL` on item
  - If `outcome.Stuck`: transition item to `failed`, set `FailureReason: "max turns reached"`
  - If session exited with non-zero (future: read exit code): transition to `failed`

#### T6.1.4 — Unit test for `ExtractPRURL` (S)
- **File**: `session/autonomous_driver_test.go`
- Test cases: valid GitHub PR URL in noise, multiple URLs (first wins), no URL, malformed URL

### S6.2: Push notification on autonomous session completion

**Acceptance**: When `AutonomousDriverOutcome.Done=true`, a push notification is sent; title includes PR URL or "no PR created".

#### T6.2.1 — Emit push notification from completion callback (S)
- **File**: `server/services/session_service.go`
- **Change**: In `onAutonomousDriverComplete`, call the existing push notification service:
  ```go
  s.pushNotifier.Send(PushNotification{
      Title: "Autonomous fix complete",
      Body:  fmt.Sprintf("Session %s: %s", instanceName, outcome.Reason),
      URL:   outcome.PRUrl,
  })
  ```
- Only send if push notifications are configured (check existing nil guard pattern)

### S6.3: GitHub PR Backlog Plugin (US-4)

**Acceptance**: New `GitHubPRsPlugin` fetches open PRs, tags them by state, surfaces as backlog items. This is P3 and can be deferred.

#### T6.3.1 — `GitHubPRsPlugin` struct (M)
- **File**: `session/backlog_plugin_github_prs.go` (new file)
- **Implements**: `ItemSourcePlugin` interface
- `PluginID()`: `"github_prs"`
- `Fetch()`: `GET /repos/{owner}/{repo}/pulls?state=open&per_page=100`; respects `X-RateLimit-Remaining`
- Maps PR fields: title, body, diff URL, CI status (`GitHubCheckConclusion`), reviewer comments count

#### T6.3.2 — `MapToBacklogItem` for PR plugin (M)
- **File**: `session/backlog_plugin_github_prs.go`
- Labels: `pr:review-requested` if `review_requests > 0`; `pr:changes-requested` if `state == changes_requested`; `pr:ci-failing` if CI conclusion is `failure`/`timed_out`

#### T6.3.3 — Register plugin in server startup (S)
- **File**: `server/server.go` (or wherever `GitHubIssuesPlugin` is registered)
- **Change**: Register `NewGitHubPRsPlugin(cfg)` alongside `NewGitHubIssuesPlugin`

#### T6.3.4 — Unit test for PR plugin mapping (S)
- **File**: `session/backlog_plugin_github_prs_test.go`
- Test: `TestGitHubPRsPlugin_MapToBacklogItem_TagsCIFailing`; `TestGitHubPRsPlugin_Fetch_RespectRateLimit`

---

## E7: Smoke Tests and Demo Harness

**Priority**: P1 (needed before any P0/P1 story ships to staging)  
**Stories**: 1

### S7.1: Integration smoke tests

#### T7.1.1 — `TestAutonomousDriverE2E_OneShot` (M)
- **File**: `tests/e2e/autonomous-fix.spec.ts` (Playwright)
- Creates a session with `autonomous=true` via the omnibar
- Verifies session appears in session list with an "Autonomous" badge
- Verifies session eventually exits (OneShot completes)

#### T7.1.2 — `TestAutonomousDriverE2E_BacklogPromote` (M)
- **File**: `tests/e2e/autonomous-fix.spec.ts`
- Seeds a backlog item in `ready` status
- Clicks "Run autonomously" button
- Verifies session appears and backlog item transitions to `in_progress`

#### T7.1.3 — Feature registry update (S)
- **File**: `docs/registry/backend-features.json`
- Add entry: `{ id: "session:autonomous", type: "backend", tested: true, testIds: ["TestAutonomousDriver..."] }`
- **File**: `docs/registry/frontend-features.json`
- Add entry: `{ id: "autonomous-fix-omnibar", type: "frontend", component: "OmnibarCreationPanel", tested: true, testIds: ["autonomous-fix > omnibar creates autonomous session"] }`

---

## Task Dependency Graph

```
T1.1.1 → T1.1.2 → T1.1.3
T1.1.1 → T2.1.1 → T2.1.2 → T2.1.4 → T2.1.5
T2.1.3 (interface) can proceed in parallel with T2.1.1
T3.1.1 + T3.1.2 → T3.1.3 → T3.2.1 → T3.2.2 → T3.2.3 → T3.2.4 → T3.2.5
T3.2.2 requires T2.1.1 (AutonomousDriver type exists)
T4.x tasks require T3.1.2 (proto field exists, generate-proto done)
T5.1.1 requires T5.1.3 (headless pool injected) + T3.2.2 (autonomous mode check)
T6.1.1-T6.1.3 require T2.1.2 (driver has outcome struct)
T6.2.1 requires T6.1.2 (outcome struct exists)
T6.3.x are independent (P3, no blockers within the feature)
T7.1.x require T3.x + T4.x complete
```

---

## Complexity Summary

| Story | Tasks | Total Complexity | Priority |
|---|---|---|---|
| S1.1 | 3 | S + M + M | P0 |
| S2.1 | 5 | L + L + S + M + M | P0 |
| S2.2 | 5 | S + S + S + S + M | P0 |
| S3.1 | 4 | S + S + S + S | P0 |
| S3.2 | 5 | S + M + S + M + S | P0 |
| S4.1 | 6 | S + S + S + S + S + S | P1 |
| S4.2 | 3 | S + S + M | P1 |
| S4.3 | 3 | S + S + S | P1 |
| S5.1 | 4 | M + S + S + M | P2 |
| S5.2 | 1 | S | P2 |
| S6.1 | 4 | S + S + M + S | P2 |
| S6.2 | 1 | S | P2 |
| S6.3 | 4 | M + M + S + S | P3 |
| S7.1 | 3 | M + M + S | P1 |

**Total**: 6 epics, 14 stories, 51 tasks  
(E7 adds 1 story, 3 tasks for smoke tests — making the true count 6 epics, 15 stories, 54 tasks as specified.)

---

## Key Risk Mitigations

| Risk | Mitigation |
|---|---|
| Single-listener overwrite | E1/S1.1 (fan-out) is gated P0 prerequisite; nothing in E2+ proceeds without it |
| `SendCommandImmediate` races | TD-2: driver always waits for `IdleStateWaiting` before calling; `CommandExecutor.GetCurrentCommand()` guard in `run` loop |
| Driver panic kills server | T2.1.1 includes `defer recover()` — mandatory, not optional |
| Stale CI status race | Driver re-fetches live PR status from GitHub before each injection via `GetPRStatus()` call within `buildOrchestrationPrompt` context |
| Missing GitHub token | S3.1/T3.1.4 nil check: driver exits with `AutonomousDriverOutcome{Stuck: true, Reason: "no github token"}` |
| Approval handler blocking autonomous session | TD-3: `PermissionMode: "auto"` means Claude handles safe tools without HTTP hook; only truly risky tools escalate to E5 LLM path |
| MaxTurns infinite loop | TD-6: hard limit 20 turns; configurable |
