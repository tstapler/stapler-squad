# Test Infrastructure Stack Research

## 1. How ClaudeController and ReviewQueuePoller Are Tested Today

### ClaudeController (`session/claude_controller_test.go`)

All controller tests use **real instances, not mocks** — but at a shallow depth. The pattern is:

- Construct a real `*ClaudeController` via `NewClaudeController(instance)` (no PTY, no tmux)
- For tests that need status detection or cache logic, a `mockInstance` struct is injected directly into the `ClaudeController` struct literal (bypassing the constructor), providing a controllable `Preview()` return value
- Tests that require a running PTY (full lifecycle) are unconditionally skipped with `t.Skip("Requires full instance initialization")`

The `mockInstance` fake (lines 591–614 of `claude_controller_test.go`) already satisfies the `InstanceContext` interface and is the correct vehicle for testing anything that calls `cc.GetCurrentStatus()` or `cc.GetIdleState()` without a real PTY.

```go
type mockInstance struct {
    title      string
    preview    string
    previewErr error
}
func (m *mockInstance) Preview() (string, error)  { return m.preview, m.previewErr }
// ...implements full InstanceContext interface
```

The constructor `newControllerWithMock(preview string)` assembles a `ClaudeController` with a `mockInstance`, a real `StatusDetector`, and a real `IdleDetector` — this is the **canonical fake for unit-testing the callback path**.

### ReviewQueuePoller (`session/review_queue_poller_test.go`, `session/adaptive_poller_test.go`)

- Uses **real poller instances** (no mocks) with `NewReviewQueuePollerWithConfig`
- Sets `ReconcileInterval: 0` to disable tmux-dependent reconciliation
- Sets short `PollInterval` (10–100ms) to observe adaptive behavior without real wait times
- Calls `poller.checkSession(inst, nil)` directly (internal package tests) to unit-test the decision logic without running the goroutine

### ReactiveQueueManager (`server/review_queue_manager_test.go`)

- Uses real instances of all collaborators: `ReviewQueue`, `ReviewQueuePoller`, `events.EventBus`, ent-backed `storage`
- A shared helper `newReactiveQueueTestSetup(t)` (lines 582–603) wires everything up, uses `t.TempDir()` for the DB, and registers `t.Cleanup` for teardown
- Starts the manager in a goroutine (`go reactiveQueueMgr.Start(ctx)`) then waits for `reviewQueuePoller.IsRunning()` using `testutil.WaitForCondition`
- Sends events to the `EventBus` directly and asserts outcomes on `queue.Has()` / channel receives

---

## 2. Test Helpers and Fakes Available for Reuse

| Helper | Location | Purpose |
|---|---|---|
| `newControllerWithMock(preview)` | `session/claude_controller_test.go:606` | Builds a `ClaudeController` with a `mockInstance`; real `StatusDetector` + `IdleDetector`; no PTY needed |
| `mockInstance` struct | `session/claude_controller_test.go:591` | Satisfies `InstanceContext`; controllable `Preview()` return; correct for testing `OnOutput` callbacks |
| `newSimpleTestPoller()` | `session/review_queue_poller_test.go:287` | Builds a poller with real queue and status manager, nil storage; safe for unit tests |
| `newTestPollerInstance(title, uuid)` | `session/review_queue_poller_test.go:293` | Builds a minimal started/paused `*Instance` for poller tests |
| `newTestPoller(t, fast, slow)` | `session/adaptive_poller_test.go:14` | Builds a poller with configurable intervals and reconciliation disabled |
| `newReactiveQueueTestSetup(t)` | `server/review_queue_manager_test.go:582` | Full integration fixture: real queue, poller, event bus, ent storage with cleanup |
| `testObserver` struct | `session/review_queue_test.go:603` | `ReviewQueueObserver` implementation with function fields; reusable for observer assertions |
| `makeAcknowledgedInstance(title)` | `session/review_queue_poller_test.go:13` | Pre-built acknowledged instance for snooze-logic tests |
| `testutil.WaitForCondition` | `testutil/wait.go`, `testutil/wait/wait.go` | Poll-until-true with configurable timeout/interval; both packages exist (use `testutil/wait` inside `session/` to avoid cycle) |
| `wait.FastWaitConfig()` | `testutil/wait/wait.go` | 2s timeout, 50ms poll; for quick async assertions |

For the new **callback/event-driven path**, the most directly reusable helper is `newControllerWithMock` combined with a captured `StatusChangeListener` func. Pattern:

```go
cc, inst := newControllerWithMock(tmuxOutputWithApproval)
var gotStatus detection.DetectedStatus
cc.SetStatusChangeListener(func(s detection.DetectedStatus, ctx string) {
    gotStatus = s
})
// Simulate a PTY output event by mutating the mock and calling the OnOutput hook
inst.preview = tmuxOutputWithApproval
cc.onOutputForTest() // or trigger via the responseStream callback
// then assert gotStatus == detection.StatusNeedsApproval
```

---

## 3. Goroutine Leak Detection

The codebase does **not use `go.uber.org/goleak`** or any automated goroutine leak checker. There is no `TestMain` that runs goleak on package exit.

The project's approach to goroutine safety:

1. **`-race` flag is authoritative** — `make test` and CI run with `-race`; data races are the primary goroutine-hygiene signal
2. **Context + `defer Stop()`** — every test that starts a background goroutine (poller, manager) uses `context.WithCancel` + `defer cancel()` paired with `defer poller.Stop()` or `defer mgr.Stop()`. The pattern is consistent in `adaptive_poller_test.go` and `review_queue_manager_test.go`
3. **`t.Cleanup`** — `newReactiveQueueTestSetup` registers `t.Cleanup(func() { repo.Close() })` for DB teardown
4. **No sleep-based leak detection** — the test suite does not count goroutines before/after

For the new implementation, the recommended guard is:

```go
// At top of test:
before := runtime.NumGoroutine()
// ... start controller/poller ...
// At end of test:
poller.Stop()
time.Sleep(50*time.Millisecond) // allow goroutines to exit
after := runtime.NumGoroutine()
if after > before+1 { t.Errorf("goroutine leak: %d goroutines after stop", after-before) }
```

Or, for the new idle timer goroutine specifically, verify it is stopped by observing `poller.IsRunning() == false` after `Stop()` within a 2s deadline (the existing `TestReviewQueuePoller_StartStop` pattern).

---

## 4. Recommended Test Pattern for Status-Change Callback Firing on PTY Output

The canonical test structure for R1 (Controller Status Change Events) is:

```go
func TestClaudeController_StatusChangeCallback_FiresOnApprovalPrompt(t *testing.T) {
    // 1. Build controller with mock PTY content that does NOT yet show approval
    cc, inst := newControllerWithMock(tmuxOutputSmall) // tmuxOutputSmall → StatusActive

    // 2. Register the callback under test
    var (
        mu        sync.Mutex
        callbacks []detection.DetectedStatus
    )
    cc.SetStatusChangeListener(func(s detection.DetectedStatus, ctx string) {
        mu.Lock()
        defer mu.Unlock()
        callbacks = append(callbacks, s)
    })

    // 3. Force first status read to establish baseline (populates statusCache + lastEmittedStatus)
    _, _ = cc.GetCurrentStatus()

    // 4. Mutate mock to simulate approval prompt arriving
    inst.preview = tmuxOutputWithApprovalPrompt // StatusNeedsApproval

    // 5. Simulate the OnOutput hook firing (what RecordActivity currently does,
    //    plus the new status-detection call the implementation will add)
    cc.onOutputCallback() // method to be added by implementation

    // 6. Assert callback was fired exactly once with the correct status
    mu.Lock()
    defer mu.Unlock()
    if len(callbacks) != 1 {
        t.Fatalf("expected 1 callback, got %d", len(callbacks))
    }
    if callbacks[0] != detection.StatusNeedsApproval {
        t.Errorf("expected StatusNeedsApproval, got %v", callbacks[0])
    }
}

func TestClaudeController_StatusChangeCallback_SuppressedOnNoChange(t *testing.T) {
    // Verify R1.3: identical status → no spurious second callback
    cc, inst := newControllerWithMock(tmuxOutputSmall)
    var count int32
    cc.SetStatusChangeListener(func(_ detection.DetectedStatus, _ string) {
        atomic.AddInt32(&count, 1)
    })
    _, _ = cc.GetCurrentStatus()      // establish baseline
    inst.preview = tmuxOutputSmall    // no change
    cc.onOutputCallback()
    cc.onOutputCallback()
    if atomic.LoadInt32(&count) != 0 {
        t.Errorf("expected 0 callbacks for unchanged status, got %d", count)
    }
}
```

This pattern is a direct extension of the existing `TestGetCurrentStatus_CacheHit` / `TestGetCurrentStatus_CacheMissOnChange` tests and requires no new infrastructure.

For the `InstanceStatusManager` wiring (R1.5), add a test in `server/review_queue_manager_test.go` extending `newReactiveQueueTestSetup` — wire a fake `ClaudeController` with a manually-invoked callback and assert that `poller.CheckSession` is called and the item appears in the queue within `wait.FastWaitConfig()` timeout.

---

## 5. Integration Tests to Update

### Must update (directly affected by event path):

| Test | File | Why |
|---|---|---|
| `TestReviewQueuePoller_AcknowledgedSession_RemovedOnNextPoll` | `session/review_queue_poller_test.go:525` | Calls `poller.checkSession` directly; will still pass but should gain a sibling test verifying the same removal happens via the event callback, not only via poll |
| `TestReactiveQueueManagerIntegration` | `server/review_queue_manager_test.go:17` | Currently verifies queue changes via direct `queue.Add()`; add a variant that fires the callback via a `ClaudeController` and asserts the same `ItemAdded` event arrives |
| `TestOnItemAdded_EventBusBehavior_BUG001` | `server/review_queue_manager_test.go:152` | No change needed — tests `OnItemAdded` behavior, not the source of the call |

### New tests needed (not currently covered):

| Test name | Package | What it verifies |
|---|---|---|
| `TestClaudeController_StatusChangeCallback_FiresOnApprovalPrompt` | `session` | R1.1: callback fires when PTY content changes to approval |
| `TestClaudeController_StatusChangeCallback_SuppressedOnNoChange` | `session` | R1.3: no callback when status unchanged |
| `TestClaudeController_StatusChangeCallback_UsesHashCache` | `session` | R1.2: detection skipped on cache hit |
| `TestInstanceStatusManager_WiresCallbackToReactiveQueue` | `server` | R1.5: wiring at session creation; callback → CheckSession → queue item |
| `TestReviewQueuePoller_ControllerSessions_SkipFastPath` | `session` | R3.2: sessions with active controller are skipped by poller fast-path |
| `TestAdaptivePoller_IdleTimerFiresWithoutPoll` | `session` | R2.1: idle event delivered before next poll tick |

### Regression tests that MUST pass unchanged (from requirements R5.4):
- All `TestReviewQueue*` tests in `session/review_queue_test.go`
- All `TestReviewQueuePoller*` tests in `session/review_queue_poller_test.go`
- All `TestReactiveQueueManager*` tests in `server/review_queue_manager_test.go`
