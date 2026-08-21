# Pitfalls and Risks: Triage Autonomous Migration

This document catalogs concrete risks identified from reading the source code for the two bugs being fixed:

1. **`WatchSessions` sends hidden sessions in initial snapshots without filtering**
2. **`TriggerTriage`/`TriggerReReview` use `oneShot: true` but should use `AutonomousDriver`**

---

## 1. Race Conditions in the WatchSessions Fix

### 1.1 Subscribe-Before-Snapshot Is the Right Order — Do Not Invert It

The current code already does the right thing:

```go
// server/services/session_service.go:1630
eventCh, subID := s.eventBus.Subscribe(ctx)   // ← subscribe FIRST
defer s.eventBus.Unsubscribe(subID)
// ... then build snapshot ...
for _, inst := range instances {
    stream.Send(createInitialSnapshotEvent(inst)) // snapshot SECOND
}
```

If the order were reversed (snapshot then subscribe), a session created between the snapshot read and the subscribe call would be silently dropped. The existing order is correct. **Any refactor that moves the subscribe call after the snapshot loop introduces a silent data loss race.**

### 1.2 Missing Hidden Filter in the Snapshot Phase

The snapshot loop applies `CategoryFilter` and `StatusFilter` but does not apply `IncludeHidden`:

```go
// CURRENT CODE — missing hidden filter:
for _, inst := range instances {
    if req.Msg.CategoryFilter != nil && *req.Msg.CategoryFilter != "" {
        if inst.Category != *req.Msg.CategoryFilter { continue }
    }
    if req.Msg.StatusFilter != nil && ... { ... }
    // ← no check for inst.Hidden && !req.Msg.IncludeHidden
    if err := stream.Send(createInitialSnapshotEvent(inst)); err != nil { ... }
}
```

This sends triage and review sessions (which are created with `hidden: true`) to clients that did not request hidden sessions. ListSessions has the correct pattern:

```go
// server/services/session_service.go:797-799
if inst.Hidden && !req.Msg.IncludeHidden {
    continue
}
```

The fix is a single guard added before `stream.Send`. However, `WatchSessionsRequest` does not currently have an `include_hidden` field (only `ListSessionsRequest` does at field 6). Before adding the guard, check whether the proto message needs to be extended.

**Proto file:** `proto/session/v1/session.proto` — `WatchSessionsRequest` starts at line 595. It has `category_filter`, `status_filter`, and `after_seq` but no `include_hidden`. The fix requires either:
- Adding `bool include_hidden = 4;` to `WatchSessionsRequest`, or
- Hard-coding the filter to always exclude hidden sessions from streams (simpler and almost certainly correct — streaming clients are the UI, not admin tools).

### 1.3 Missing Hidden Filter in the Live-Event Phase

The real-time event loop also lacks the hidden filter:

```go
// CURRENT CODE — live events, no hidden filtering:
case event, ok := <-eventCh:
    // CategoryFilter checked here ...
    // StatusFilter checked here ...
    // ← no Hidden check
    protoEvent := convertEventToProto(event)
    stream.Send(protoEvent)
```

Triage/review sessions fire `EventSessionCreated` and `EventSessionUpdated` events via `s.eventBus.Publish(events.NewSessionCreatedEvent(instance))` in `CreateDirectorySession`. These go into the ring buffer and reach all subscribers. Without a hidden filter here, clients receive spurious session-created events for hidden sessions even after a fresh connection that never saw them in the snapshot.

**The filter must be applied consistently in both the snapshot loop and the live-event loop.** Applying it in only one place produces an inconsistency: some clients see the sessions in the snapshot (if they connect before the fix) but not in live events, or vice versa.

### 1.4 Replay Path Also Lacks the Hidden Filter

When a client reconnects with `AfterSeq > 0`:

```go
// server/services/session_service.go:1636-1640
for _, event := range s.eventBus.EventsSince(req.Msg.AfterSeq) {
    if err := stream.Send(convertEventToProto(event)); err != nil { ... }
}
```

No filtering at all is applied here — not category, status, or hidden. Events for hidden sessions created during the disconnect window will be replayed. This is a separate gap from the snapshot bug; the fix must cover all three paths.

### 1.5 The EventBus Ring Buffer Contains Hidden Session Events Permanently

`EventBus.Publish` stores every event in the ring buffer for up to one hour (`eventBufTTL`). There is no per-event hidden flag in the `Event` struct — filtering can only happen at delivery time. This means:

- Hidden session events are permanently in the buffer.
- Any future `EventsSince` call on a reconnecting client will include those events unless explicitly filtered.

The `Event` struct carries `Session *session.Instance`, so the receiver can check `event.Session.Hidden` — but only if the instance pointer is still valid. For `EventSessionDeleted`, `event.Session` is `nil` and the instance is gone; the receiver cannot determine if it was hidden. Consider adding a `Hidden bool` field to `Event` or treating deleted hidden sessions as a non-event for non-hidden clients.

---

## 2. Nil headlessPool Risks

### 2.1 Current TriggerTriage Does Not Need headlessPool — But the Proposed Fix Does

Today, `TriggerTriage` (in `backlog_service.go:1073`) calls `CreateDirectorySession` with `oneShot: true` and never touches `headlessPool`. The `headlessPool` field lives on `SessionService`, not `BacklogService`. `BacklogService` only has the `AutonomousDriverStarter` interface:

```go
// server/services/backlog_service.go:31-33
type AutonomousDriverStarter interface {
    StartAutonomousDriverForInstance(inst *session.Instance)
}

// BacklogService field:
autonomousStarter AutonomousDriverStarter
```

`StartAutonomousDriverForInstance` on `SessionService` already nil-checks `headlessPool`:

```go
// server/services/session_service.go:673-676
func (s *SessionService) StartAutonomousDriverForInstance(inst *session.Instance) {
    if s.headlessPool == nil {
        log.Warn("[SessionService] StartAutonomousDriverForInstance: headlessPool is nil", "session", inst.Title)
        return
    }
```

**Risk:** If `TriggerTriage` is modified to call `s.autonomousStarter.StartAutonomousDriverForInstance(inst)` and `autonomousStarter` is nil (not wired), this panics with a nil pointer dereference. The pattern used in `SpawnSessionFromItem` is safe:

```go
// server/services/backlog_service.go:940-942
if req.Msg.Autonomous && s.autonomousStarter != nil {
    s.autonomousStarter.StartAutonomousDriverForInstance(inst)
}
```

**The nil guard `s.autonomousStarter != nil` must be present in TriggerTriage and TriggerReReview.** No conditional flag like `req.Msg.Autonomous` exists for triage sessions — they always want the driver — so the guard becomes:

```go
if s.autonomousStarter != nil {
    s.autonomousStarter.StartAutonomousDriverForInstance(inst)
} else {
    log.Warn("[TriggerTriage] AutonomousDriverStarter not wired; triage session will run without driver")
}
```

Omitting this check will panic in test environments where `autonomousStarter` was never wired.

### 2.2 headlessPool Nil on AutonomousDriver Construction

`NewAutonomousDriver` (in `autonomous_driver.go:55`) accepts `nil` for `pool` without panicking at construction time. The nil check happens in `Start`:

```go
// session/autonomous_driver.go:92-95
func (d *AutonomousDriver) Start(ctx context.Context) error {
    if d.headlessPool == nil {
        return fmt.Errorf("AutonomousDriver: headlessPool is nil for session %q", d.inst.Title)
    }
```

This means the driver object is safe to create with a nil pool (e.g., in tests), but `Start` returns an error. `StartAutonomousDriverForInstance` checks the pool before construction, so the error from `Start` should not occur in production. However, if a future caller bypasses `StartAutonomousDriverForInstance` and calls `NewAutonomousDriver(..., nil, ...).Start(...)` directly, the error is not fatal and is logged at Warn level — the triage session will run without orchestration.

---

## 3. BacklogLifecycleListener Interactions

### 3.1 The Listener Ignores Triage and Review Sessions on Exit — By Design

`onSessionExited` in `backlog_lifecycle.go:159` records end time for all roles but only drives state transitions for work sessions:

```go
// session/backlog_lifecycle.go:177-180
// Only drive in_progress→review/done transitions for work sessions.
if is.SessionRole != SessionRoleWork {
    return
}
```

This means when the triage session exits (after calling `submit_triage_result`), the `BacklogLifecycleListener` only updates `ended_at` on the `ItemSession` record. It does NOT transition the backlog item status. The status transition from `idea` → `ready` happens separately — **it is not clear from the code who drives this transition for triage sessions.**

Looking at `submit_triage_result` in `tools_backlog.go:402-536`: it saves the triage result JSON and fires a notification, but does **not** call `TransitionBacklogItemStatus`. The item remains in `idea` status until an operator manually advances it or until some other mechanism runs.

**Risk with the new AutonomousDriver approach:** If the plan is for the `AutonomousDriver`'s completion callback (`onAutonomousDriverComplete`) to drive the status transition, check what it does:

```go
// server/services/session_service.go:3452-3456
toStatus := session.BacklogStatusReview
if _, transErr := concreteStorage.TransitionBacklogItemStatus(ctx, item.ID.String(), toStatus, nil); transErr != nil {
```

This transitions to `review`, not `ready`. Triage sessions should transition to `ready`, not `review`. **The completion callback must be either specialized for triage sessions or the triage session must not be linked as a work ItemSession.**

The resolution depends on the design intent:
- If the `AutonomousDriver` for triage fires `onAutonomousDriverComplete`, and the item's `ItemSession` has role `triage`, the callback's lookup via `GetItemSessionBySessionUUID` will find it. The callback currently always transitions to `review`. A role-aware check is needed.

### 3.2 Double-Completion Risk: AutonomousDriver + BacklogLifecycleListener

When the triage session exits naturally (Claude Code calls `submit_triage_result` and then exits), two completion paths may fire:

1. **`BacklogLifecycleListener.onSessionExited`** — fires from `EventExited` via the lifecycle listener shim. It records `ended_at` and returns early because the role is `triage` (not `work`). Safe.

2. **`AutonomousDriver.fireCompletion`** — fires when the driver detects the session is `Done` (via DONE signal from LLM) or when the driver loop exhausts max turns. The completion callback `onAutonomousDriverComplete` then attempts to transition the backlog item.

These two paths do not conflict on the `BacklogLifecycleListener` side because the listener early-returns for non-work sessions. However, if the AutonomousDriver fires `Done` at the same time the Claude Code process is exiting (two concurrent events), both `AutonomousDriver.fireCompletion` and `BacklogLifecycleListener.onSessionExited` fire. The listener does nothing for triage roles, so there is no double-transition risk from the listener.

**The real double-completion risk is AutonomousDriver firing DONE before `submit_triage_result` completes.** The DONE signal is detected by the LLM orchestrator reading the session output. If the orchestrator detects DONE based on terminal output before `submit_triage_result` has been processed by the MCP server, the driver fires its completion callback while the MCP handler is still writing to the database. This is a race between the LLM-detected DONE and the async DB write in the MCP handler.

### 3.3 AutonomousDriver Stop After Session Exit

`AutonomousDriver.Stop` cancels the driver's context:

```go
// session/autonomous_driver.go:125-133
func (d *AutonomousDriver) Stop() {
    d.mu.Lock()
    cancel := d.cancel
    d.mu.Unlock()
    if cancel != nil {
        cancel()
    }
    d.driverRunning.Store(false)
}
```

If the triage session exits (Claude Code process dies) and the driver's `waitForIdle` is currently blocking in a long LLM call (`CallBlockingWithOptions`), the context cancellation propagates into the pool call. This is the intended behavior per the comment on `Stop`. But if `Stop` is called from `stopAndDeregisterDriver` in `onAutonomousDriverComplete`, and `onAutonomousDriverComplete` was called by the driver's own goroutine (from `fireCompletion`), there is a subtle ordering question: `fireCompletion` fires, then `onAutonomousDriverComplete` calls `stopAndDeregisterDriver`, which calls `d.Stop()`, which calls `d.driverRunning.Store(false)`. The goroutine running `d.run` already returned at this point (`fireCompletion` is called after `return`), so `d.driverRunning.Store(false)` in the `defer` in `Start` also fires. The double-Store is safe because `atomic.Bool.Store` is idempotent.

---

## 4. isOneShot() Tag Logic Risks

### 4.1 isOneShot() Checks Tags, Not the OneShot Field

The critical implementation of `isOneShot` in `session_driver.go:491-493`:

```go
// session/session_driver.go:491-493
func isOneShot(inst *Instance) bool {
    return inst.HasTag("backlog:triage") || inst.HasTag("backlog:review")
}
```

It checks **tags**, not `inst.OneShot`. This is the discriminator for the no-retry behavior in `runSessionDriverWithPrompt`. Currently, `TriggerTriage` calls `CreateDirectorySession` with both `oneShot: true` AND the `backlog:triage` tag:

```go
// server/services/backlog_service.go:1179-1183
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
    []string{"backlog:triage"}, true, true)
//                              ^^^^ oneShot=true, hidden=true
```

**If the migration removes `oneShot: true` from the `CreateDirectorySession` call** (to switch triage sessions from oneShot mode to AutonomousDriver mode), the tag `backlog:triage` still causes `isOneShot()` to return true. The no-retry behavior is preserved by the tag, not by the `OneShot` field.

However, there are two behaviors gated on `inst.OneShot` that ARE different from `isOneShot()`:

1. **`tryExtractClaudeSessionID`** (session_driver.go:170-173): called when the session stops. It only runs if `inst.OneShot` is true:
   ```go
   if inst.OneShot {
       tryExtractClaudeSessionID(inst)
   }
   ```
   Removing `oneShot: true` means the claude session ID is not extracted for potential `--resume`. For triage sessions this is irrelevant (they are single-run, no resume needed), but it is a behavioral change.

2. **`ClaudeCommandBuilder`**: `inst.OneShot` controls whether `claude -p` (non-interactive, print-and-exit) is used. If `oneShot` is removed, the session would run `claude` in interactive mode instead of `claude -p`. **This is a major behavioral change.** The AutonomousDriver path requires interactive mode (`claude` without `-p`) because it injects prompts into the running session. So this change is actually *correct* for the AutonomousDriver approach, but it must be intentional.

**Summary:** Removing `oneShot: true` for triage sessions changes the Claude invocation from `claude -p <prompt>` to interactive `claude`. The `isOneShot()` tag guard still prevents retry loops. This is the right change for AutonomousDriver, but every test that mocks `CreateDirectorySession` with `oneShot=true` will need updating.

### 4.2 Retry Loop Risk if Tag Is Missing

If a triage session somehow launches **without** the `backlog:triage` tag AND without `oneShot: true` (e.g., a misconfigured call), `isOneShot()` returns false and the session driver will attempt to restart the session on unexpected exit. This could:
- Trigger a second triage run on the same item
- Create confusion with the BacklogLifecycleListener (two triage ItemSessions open simultaneously)

The orphan guard in `TriggerTriage` (lines 1111-1131) handles this by closing open triage ItemSessions before spawning a new one, but only if called again via the RPC. An auto-restart from the session driver would bypass this guard.

**Mitigation:** Ensure that `CreateDirectorySession` always receives `[]string{"backlog:triage"}` as the tags argument. Add a test that verifies the tag is present on the spawned instance.

---

## 5. AutonomousDriver Completion Signal Risks

### 5.1 submit_triage_result Does Not Stop the AutonomousDriver

`submit_triage_result` in `tools_backlog.go` saves results, fires a notification, and returns. It does NOT signal the `AutonomousDriver` to stop. The driver keeps running, injecting more prompts, unless the Claude Code session exits.

In the current `oneShot: true` design, `claude -p` exits after completing the task, which causes the PTY EOF, which fires `EventExited`, which the driver (if any) would observe via context cancellation. In the new AutonomousDriver design with interactive `claude`, the session stays alive until the driver or the user stops it.

**Risk:** After `submit_triage_result` completes, the AutonomousDriver may inject another orchestration prompt asking the agent to do more work. The orchestrator LLM sees the triage result was submitted and should respond with `DONE: ...`, but if it sees ambiguous terminal output it may issue a `NEXT_MESSAGE` instead, triggering an unnecessary second round of triage work.

**Mitigation options:**
- After `submit_triage_result` completes, send a special signal via the event bus that causes the registered AutonomousDriver for that session to stop. This requires wiring the event bus into the MCP handler and having the driver listen for it.
- The simpler option: the driver's LLM orchestrator prompt should explicitly know the triage is complete (it can read the terminal output which will contain `submit_triage_result` success text) and emit `DONE`.
- Make the triage prompt tell the agent to exit after calling `submit_triage_result` (e.g., "After submitting, type `/exit` to close the session"). This ensures the PTY exits, the driver's context is cancelled, and the driver fires its completion callback.

### 5.2 AutonomousDriver Fires Before Session Is Fully Started

`AutonomousDriver.Start` waits up to 60 seconds for the initial idle state:

```go
// session/autonomous_driver.go:157-164
startupCtx, startupCancel := context.WithTimeout(ctx, 60*time.Second)
if !waitForIdle(startupCtx, statusCh, d.controller) {
    startupCancel()
    log.Warn("AutonomousDriver: timed out waiting for initial idle state", ...)
    d.fireCompletion(sessionName, AutonomousDriverOutcome{Stuck: true, Reason: "startup timeout"})
    return
}
```

For triage sessions that spawn multiple research subagents in parallel, the session may not be idle for an extended period. The 60-second window may be insufficient for a slow machine or a session that starts with trust dialogs. If the driver times out at startup, it fires `Stuck: true` — which triggers `onAutonomousDriverComplete`, which transitions the backlog item to `review` status. The triage was never actually performed.

**Risk:** A startup timeout on a valid triage session looks identical to a completed-but-stuck work session. The item ends up in `review` with no triage results, no plan.md, and a confusing state.

**Mitigation:** Either increase the startup timeout for triage sessions, or handle the `Stuck` case differently for triage roles in `onAutonomousDriverComplete`.

### 5.3 Controller Nil Check Timing

In `AutonomousDriver.Start`:

```go
// session/autonomous_driver.go:104-111
d.mu.Lock()
d.cancel = cancel
d.controller = d.inst.GetController()
d.mu.Unlock()

if d.controller == nil {
    d.driverRunning.Store(false)
    cancel()
    return fmt.Errorf("AutonomousDriver: no controller available for session %q", d.inst.Title)
}
```

`GetController()` returns nil if the session has not yet started its controller. `CreateDirectorySession` calls `session.StartSessionDriver` (which starts the controller) before calling `StartAutonomousDriverForInstance`, so in the normal flow the controller should exist. But if `StartAutonomousDriverForInstance` is called concurrently or out of order, it will return an error and the triage session runs without orchestration.

---

## 6. Hidden Session Event Filtering Scope

### 6.1 All Clients Should Be Shielded, Not Just Some

The `ListSessions` RPC (lines 797-798) already filters hidden sessions per-client based on `IncludeHidden`. The same filtering must apply to `WatchSessions`. A client that calls `ListSessions` without `include_hidden=true` and then subscribes to `WatchSessions` will see a consistent view only if both endpoints apply the same filter.

If hidden sessions are excluded from the snapshot but not from live events, the client may receive spurious `session.created` events for sessions it should never see. The client-side code typically upserts sessions on `session.created` events, so the hidden triage session would appear in the UI.

### 6.2 Clients That Connect After a Hidden Session Is Created

A client connecting after a triage session is created and completed will NOT see the session in the snapshot (assuming the fix is applied to the snapshot loop). If a `session.updated` event for the triage session is still in the ring buffer (within the 1-hour TTL), and the client reconnects with `AfterSeq`, the event will be replayed without filtering. This is the replay-path gap described in section 1.4.

### 6.3 session.deleted Events for Hidden Sessions

When a triage session is deleted, `NewSessionDeletedEvent` is published. The `Event` struct for delete events has `Session: nil` (only `SessionID` is set). There is no way to determine at delivery time whether the deleted session was hidden. Clients that never received a `session.created` for the session will receive an orphaned `session.deleted` — this is harmless (no session to remove from the UI) but generates noise. Clients that did receive the session (because the filter was not applied) will attempt to remove it from their UI.

A future improvement would be to include a `hidden bool` field in `Event` or in `NewSessionDeletedEvent`. For now, the simpler fix is to ensure hidden sessions are never sent in the first place, making this a non-issue.

---

## 7. Existing Test Coverage Gaps

- **`TestWatchSessions`**: Does not exist. There are no tests for `WatchSessions` filtering behavior at all — neither for `CategoryFilter`, `StatusFilter`, nor the missing `IncludeHidden` filter.
- **`TestTriggerTriage` with AutonomousDriver**: The existing `TestTriggerTriage_DoubleTriggerGuard` (backlog_service_test.go:366) tests the orphan guard but does not test that an AutonomousDriver is started.
- **`TestTriggerReReview` with AutonomousDriver**: No tests exist for the AutonomousDriver being wired to re-review sessions.
- **`isOneShot` behavior with tags only**: No test verifies that removing `oneShot: true` while keeping the `backlog:triage` tag still prevents retry loops.

---

## 8. Summary of High-Risk Pitfalls

| # | Risk | Severity | Location |
|---|------|----------|----------|
| 1 | `WatchSessions` snapshot sends hidden sessions — no `IncludeHidden` guard | High | `session_service.go:1655` |
| 2 | `WatchSessions` live-event loop sends hidden session events | High | `session_service.go:1678` |
| 3 | `WatchSessions` replay path (`EventsSince`) sends hidden session events | Medium | `session_service.go:1636` |
| 4 | `TriggerTriage` does not nil-check `autonomousStarter` before calling it | Critical | `backlog_service.go:1179` |
| 5 | `onAutonomousDriverComplete` always transitions to `review`; triage should go to `ready` | High | `session_service.go:3452` |
| 6 | Removing `oneShot: true` switches triage from `claude -p` to interactive `claude` | High | `backlog_service.go:1179` |
| 7 | `submit_triage_result` does not stop AutonomousDriver; driver may inject extra turns | Medium | `tools_backlog.go:402` |
| 8 | 60-second startup timeout may expire during triage subagent parallelization | Medium | `autonomous_driver.go:158` |
| 9 | Proto `WatchSessionsRequest` lacks `include_hidden` field | Medium | `session.proto:595` |
| 10 | `BacklogLifecycleListener` records triage end-time but no test verifies this | Low | `backlog_lifecycle.go:177` |
