# BUG-090: `StreamHub` teardown reaches `HubTornDown` state before/without `StopControlMode` being called [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-08-24, commit `39751d4c6`)
**Resolution**: `ForceTeardown` now flips `h.state` to `HubTornDown` only after `StopControlMode` returns (or is skipped for a nil controller), using a separate `teardownInFlight` guard instead of the old state field. Verified `go test ./session/streamhub/... -run TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches -race -count=50` all green.
**Discovered**: 2026-08-23
**Impact**: `session/streamhub`'s grace-period teardown test flakes (~13% locally under `-race` with CPU contention, and confirmed in PR #605's CI). This is `session/streamhub`'s core session-teardown path — the mechanism that stops a tmux control-mode session after its last subscriber detaches — so if the underlying ordering gap is real (not just a slow test timer), it could mean a hub occasionally reports itself torn down without actually having stopped the control-mode process, in production, not just in tests.

## Problem Description

`TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches` (`session/streamhub/lifecycle_test.go:105-134`) attaches a subscriber, detaches it (triggering a 30ms-grace-period scheduled teardown), then does:
```go
if !waitFor(t, 5*time.Second, func() bool { return hub.State() == streamhub.HubTornDown }) {
    t.Fatalf("expected HubTornDown after grace period elapses, got %v", hub.State())
}
if calls := controller.stopCalls.Load(); calls != 1 {
    t.Fatalf("expected StopControlMode called exactly once, got %d calls", calls)
}
```
This assumes: by the time `hub.State()` first reports `HubTornDown`, `controller.StopControlMode()` has already been called exactly once. That assumption is failing — the test observes `HubTornDown` state with `stopCalls == 0`.

**This is not a fresh flake — it has already been "fixed" once and the fix was incomplete.** The test's own comment (lines 125-127) already documents an earlier occurrence of exactly this failure in CI, and the fix at the time was widening the wait from 1s to 5s ("CPU contention from other packages' tests can delay this hub's 30ms teardown timer well past 1s"). That framing assumes the *timer* is just slow. But a 5s wait for a 30ms grace period is generous — if `waitFor`'s poll loop is still observing `HubTornDown` with 0 `stopCalls` even within a 5-second window, the more likely explanation is not "the timer hasn't fired yet" but "the state transition to `HubTornDown` and the `StopControlMode()` call are not ordered relative to each other" — e.g., the hub sets its internal state to `HubTornDown` in one place, and calls `StopControlMode()` in a different goroutine or a later step, with no happens-before relationship the test (or a real caller) can rely on.

## Reproduction Steps

1. `go test ./session/streamhub/... -run 'TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches' -race -count=30 -v`
2. Observed locally: 4/30 runs (~13%) fail with `lifecycle_test.go:132: expected StopControlMode called exactly once, got 0 calls`.
3. Also observed in PR #605's CI (`Test` job, `github.com/tstapler/stapler-squad/session/streamhub`, run at 2026-08-23T17:38:14Z) — same assertion, same failure shape, unrelated to that PR's diff (`session/streamhub` is untouched by PR #605).

## Root Cause

Not fully confirmed — needs source-level investigation of the hub's teardown implementation (likely `session/streamhub/hub.go` or `session/streamhub/session_controller.go`, wherever the grace-period timer callback lives). The working hypothesis, based on the test's own history (a timing-tolerance fix that didn't fully resolve the failure) and the assertion ordering: the hub's internal state field is set to `HubTornDown` either (a) before calling `StopControlMode()`, or (b) on a different goroutine than the one that calls `StopControlMode()`, without a synchronization point a test (or caller) observing `State() == HubTornDown` can rely on to also guarantee `StopControlMode()` has completed (or even started).

## Files Likely Affected

- `session/streamhub/lifecycle_test.go:105-134` — the flaky test itself.
- `session/streamhub/hub.go` (or wherever the grace-period teardown timer/goroutine and state transition live) — the likely actual bug, if the hypothesis above is correct.

## Fix Approach

1. Read the teardown implementation to find exactly where `HubTornDown` is set relative to where `StopControlMode()` is called.
2. If they're not ordered (e.g., state set first, then `StopControlMode()` called asynchronously, or vice versa with no synchronization), fix the ordering so `StopControlMode()` is guaranteed to have been *called* (started, not necessarily completed — check `controller.stopCalls` semantics) before the state transition to `HubTornDown` becomes observable to callers of `State()`.
3. If investigation shows the ordering is actually correct and this really is "just" a slow-timer issue even at 5s (e.g., a `time.AfterFunc` callback getting starved on a heavily loaded runner), consider a event-driven design (a channel closed by the teardown goroutine itself) instead of polling `State()` — would remove the flake without further widening an already-generous timeout.

## Verification

After the fix: `go test ./session/streamhub/... -run 'TestStreamHub_should_ScheduleTeardownAfterGracePeriod_When_LastSubscriberDetaches' -race -count=100` with zero failures (100 runs chosen to comfortably exceed the ~13% observed flake rate's detection threshold).

## Related Tasks

Discovered while running PR #605's (`stapler-squad-web-transport` branch, project `web-transport-architecture-review`) remote CI gate — confirmed unrelated to that PR's diff (`session/streamhub` is untouched). `session/streamhub` is the core deliverable of `project_plans/terminal-multi-connection-streaming/`, currently in its dark-launch rollout per that project's own plan — this bug is directly in that rollout's critical path (session teardown), so may warrant elevated priority relative to other open bugs filed alongside this one (BUG-087, BUG-088, BUG-089).
