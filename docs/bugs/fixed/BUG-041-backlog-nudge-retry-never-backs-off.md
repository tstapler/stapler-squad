# BUG-041: Backlog Work-Session Nudge Retries Forever at Full Speed on Send Failure, Never Backs Off or Gives Up [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-07-22)
**Discovered**: 2026-07-22/23, log review requested during `backlog-feature-improvement` follow-up
**Fixed**: 2026-07-22 — `session/session_driver.go`
**Impact**: `SessionDriver`'s idle-nudge logic (`session/session_driver.go`) sends a one-time "you appear to have paused" reminder to a backlog work session once it's been idle past `driverBacklogNudgeDelay`. If that `SendKeys` call fails, the driver retries the *identical* send on every subsequent driver tick, indefinitely — no backoff, no attempt cap, no escalation to a stuck-reason or notification. Live: **392 consecutive failed sends over ~13 minutes** (2026-07-22T20:51:57 → 21:04:59, roughly every 2s) against tmux session `stapler-squad-expose-backlog-item-id-r8`, every one `err: "invalid argument"` — the signature of a dead/gone tmux pane (same failure shape as this project's other previously-documented dead-pane bugs, e.g. the `9264efe7` review-pane-death investigation in `docs/tasks/backlog-feature-improvement.md`). This session belongs to backlog item `693c2700` ("Expose ID functionality in Backlog"), one of the items currently cycling through the new bounded remediation-backoff system — this bug is a smaller, adjacent busy-loop sitting underneath that cycle, wasting driver-tick cycles hammering a pane that cannot possibly succeed.

## Live Evidence

```
{"time":"2026-07-22T20:51:57...","level":"WARN","msg":"SessionDriver: failed to send backlog nudge","session":"stapler-squad-expose-backlog-item-id-r8","err":"invalid argument"}
{"time":"2026-07-22T20:51:59...","level":"WARN","msg":"SessionDriver: failed to send backlog nudge","session":"stapler-squad-expose-backlog-item-id-r8","err":"invalid argument"}
... (392 total, same session, same error, ~every 2s, spanning ~13 minutes)
{"time":"2026-07-22T21:04:59...","level":"WARN","msg":"SessionDriver: failed to send backlog nudge","session":"stapler-squad-expose-backlog-item-id-r8","err":"invalid argument"}
```

## Root Cause

`session/session_driver.go:409-424` (pre-fix):

```go
if inst.HasTag(TagBacklogWork) && nudgeSentAt.IsZero() && idle > driverBacklogNudgeDelay {
    nudge := "..."
    if sendErr := inst.SendKeys(nudge + "\r"); sendErr != nil {
        log.Warn("SessionDriver: failed to send backlog nudge", "session", inst.Title, "err", sendErr)
    } else {
        log.Info("SessionDriver: sent backlog nudge", ...)
        nudgeSentAt = time.Now()
    }
    continue
}
```

`nudgeSentAt` — the guard that's supposed to make this a one-time nudge — was only assigned in the success (`else`) branch. On a `SendKeys` error, `nudgeSentAt` stayed at its zero value, so the very next driver tick re-evaluated the same `nudgeSentAt.IsZero() && idle > driverBacklogNudgeDelay` condition as still true and retried immediately. Since the underlying cause was permanent (the tmux pane was dead — `invalid argument` is tmux's signature for a gone pane), this retried at the driver's tick interval (`driverPollInterval`, 2s) forever, with no backoff, no attempt cap, and no path to surface "this session can't be nudged" as an actionable signal.

## Fix Applied

Extracted the nudge-send-and-timestamp logic into a new `attemptBacklogNudge(inst *Instance, idle time.Duration) time.Time` helper (`session/session_driver.go`) that **always** returns a non-zero, current timestamp — on a failed send exactly the same as on a successful one:

```go
func attemptBacklogNudge(inst *Instance, idle time.Duration) time.Time {
	nudge := "You appear to have paused. Run `/backlog/status` to see remaining " +
		"acceptance criteria. Mark each complete criterion with `/backlog/done-N`, " +
		"then submit with `/backlog/review` once all are done."
	if sendErr := inst.SendKeys(nudge + "\r"); sendErr != nil {
		log.Warn("SessionDriver: failed to send backlog nudge, will not retry — falling through to inactivity timeout",
			"session", inst.Title, "err", sendErr)
	} else {
		log.Info("SessionDriver: sent backlog nudge", "session", inst.Title, "idle", idle.Round(time.Second))
	}
	return time.Now()
}
```

The driver loop now does `nudgeSentAt = attemptBacklogNudge(inst, idle)` unconditionally. This deliberately does **not** retry the nudge send itself — a `SendKeys` failure here is very likely a dead/gone pane that retrying cannot fix. Instead, setting `nudgeSentAt` (success or failure) makes the *existing* `graceTimeout`/idle check just below it (which already switches to `driverBacklogNudgeGrace` once `nudgeSentAt` is non-zero) take over: after `driverBacklogNudgeGrace` (5 minutes) of continued silence, it logs "session stuck — no output for inactivity timeout" and calls `handleDriverFailure`, which restarts the session once and marks it for human attention (`markSessionNeedsAttention` → `ReviewQueue`) on a second failure. That restart also gives the session a fresh tmux pane, which is the actual remedy for a dead-pane failure — no new stuck-reason/notification plumbing was needed; the fix simply stops blocking the mechanism that already existed.

## Files Affected

- `session/session_driver.go` — extracted `attemptBacklogNudge`; driver loop's nudge block now three lines instead of duplicating the send/log/timestamp logic inline
- `session/session_driver_test.go` — two new regression tests

## Verification

- `TestAttemptBacklogNudge_FailedSend_StillReturnsNonZeroTime` — a failed `SendKeys` (simulated via an `Instance` that was never started, so `SendKeys` deterministically returns "cannot send keys to instance that has not been started or is paused" — the same permanent-failure shape as a dead pane) must still produce a non-zero timestamp.
- `TestAttemptBacklogNudge_FailedSend_RateLimitsRetry` — asserts the actual driver-loop guard condition (`nudgeSentAt.IsZero() && idle > driverBacklogNudgeDelay`) is closed immediately after one failed attempt, i.e. the identical send would not fire again on the very next tick.
- **Verified to fail against pre-fix code**: temporarily reverted `attemptBacklogNudge` to only set `nudgeSentAt` on success (mirroring the original bug) — both new tests failed with the expected diagnostic messages; restored the fix and both pass again.
- `go build ./session/...`, `make build` (full proto/ent/web/Go build) — clean.
- `go test ./session/...` — full package suite green (see below).

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Framework Pattern Misuse / API Contract Gap — a boolean "did I already do this" latch (`nudgeSentAt.IsZero()`) was updated only along the success path of a fallible operation, silently treating "attempted and failed" as equivalent to "never attempted." The two are different states (the former should be rate-limited/bounded; the latter should retry), and the code conflated them.

**Earliest achievable enforcement**: The regression test is the practical level here — this is inherently a runtime behavior (an external `SendKeys` call can fail) that no static check can catch. Extracting `attemptBacklogNudge` as a small, pure-ish, directly-testable unit (rather than leaving the logic inline in the 300-line ticker loop) is what made a fast, non-time-dependent unit test possible at all; before the extraction, testing this would have required either a live tmux backend or waiting out real 5-minute constants inside the ticker loop.

**Recurring shape**: "A one-shot latch is only set on the success branch of a fallible operation, so a failure re-triggers the same operation forever." This is a first documented instance of this *specific* shape in this codebase's bug history (distinct from the base-commit/`IsCommitOnMain` conflation family in BUG-036/BUG-039, and distinct from BUG-030's silent-no-op-instead-of-error spawn pattern) — but the general family ("guard variable only updated on the happy path") is worth watching for elsewhere any code does `if !attempted { try(); if err == nil { attempted = true } }`. No existing lint/structural check would catch this pattern generically without excessive false positives, so this is flagged as a watch-item for `quality:reflect-and-fix`/architecture-review rather than something to enforce today.

## Related

- Independent of BUG-040 (`docs/bugs/open/BUG-040-pr-pending-item-loses-pr-reference-dead-end.md`) — different file (`session/backlog_lifecycle.go`), different mechanism (`pr_pending` dead-end vs. this driver's nudge retry loop) — fixed in a separate parallel worktree.
- Same live incident's dead-pane signature (`err: "invalid argument"` from tmux) as the `9264efe7` review-pane-death investigation referenced in `docs/tasks/backlog-feature-improvement.md`.
