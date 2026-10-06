# Pitfalls Research: session-retry-backoff

**Date**: 2026-08-06
**Scope**: generalizing `session/session_driver.go`'s existing single-retry `atomic.Bool`
mechanism into a configurable multi-attempt exponential-backoff `RetryPolicy`, plus
`tmux_exited` detection, `permanently_failed` terminal state, UI surfacing, and
stale-session-detection integration (per `project_plans/session-retry-backoff/requirements.md`).

## 1. No cancellation mechanism exists today — a naive backoff timer would be a goroutine leak, and this repo already has the fix pattern

Confirmed by reading `session/session_driver.go` in full: there is **no `context.Context`
anywhere in this file** (`grep -n "context\.\|ctx " session/session_driver.go` returns
nothing). The driver loop (`runSessionDriverWithPrompt`, lines 122-469) is a plain
`for range ticker.C` polling loop (`driverPollInterval` = 2s) that checks
`inst.GetEffectiveStatus()` every tick and returns cleanly on `Paused` (line 194-196) or
`Stopped`-with-conditions. There is no stop-channel, no cancel func, nothing an external
caller (Stop/Archive) can signal into a running driver goroutine — the loop only exits by
observing state it re-polls itself.

**The risk this creates for backoff**: if a naive implementation adds
`time.Sleep(backoffDuration)` or a `time.Timer` inside `handleDriverFailure`'s retry path
to wait out the exponential delay, that sleep is **not interruptible** by the existing
`Paused`/`Stopped` checks (which only run inside the ticker loop, not during the sleep).
Stopping or archiving a session while a multi-minute-to-multi-hour backoff wait is pending
would either (a) leak the goroutine until the sleep expires and then have it try to restart
an already-stopped/archived/deleted instance, or (b) require bolting on an ad-hoc
`stopCh`/`context.Context` specifically for this one wait — a new cancellation primitive
this file has never needed before.

**This repo already solved the equivalent problem for backlog-item remediation backoff,
and the fix is directly reusable**: `session/backlog_remediation.go`'s `RemediationDue`
(`session/backlog_remediation.go:168`) implements exponential backoff **without any
sleeping goroutine at all** — it persists a `next_remediation_at` timestamp and callers
check "is it due yet" on their own existing poll tick (`autoReopenWithBackoffGate`,
`retryPushFailedWithBackoffGate`, `retryOrphanedTriageWithBackoffGate` in
`session/backlog_lifecycle.go`). Nothing sleeps; a rejected (`due=false`) tick just does
nothing and gets re-evaluated on the next poll.

**Recommendation**: model retry backoff the same way. Store `nextRetryAt` (or equivalent)
on the instance/attempt record, compute it as `initialDelay * 2^attempt` capped at
`maxDelay` when a failure is recorded, and have the **existing** `runSessionDriverWithPrompt`
ticker loop check `time.Now().After(nextRetryAt)` on its normal 2s cadence before invoking
the restart — exactly like the `Paused`/`Stopped` checks already there. This reuses the
loop's existing clean-exit paths (`Paused` → `return`, `Stopped` after archive → falls
through existing logic) for free, and needs zero new cancellation plumbing. A manual
"Retry now" then becomes trivial: it's just setting `nextRetryAt` to the past (or a
dedicated bypass flag), which the same tick-check already honors — see §2.

## 2. Manual "Retry now" racing an in-flight automated retry — the existing CAS guards don't cover this new interaction

Two atomic idempotency guards exist today and are sound for what they cover:
- `inst.driverRunning` (`atomic.Bool`, `session/instance.go:375`) — CAS-guards against two
  `StartSessionDriver` calls both spawning a driver goroutine for the same instance
  (`session_driver.go:76`). `handleDriverFailure` explicitly re-arms it
  (`inst.driverRunning.Store(true)` at line 565) *before* spawning the retry goroutine "to
  close the race window between the old goroutine's defer `Store(false)` and the new
  goroutine starting" (comment references "Design Decision D3 mitigation" — i.e. this
  exact class of race has already been hit and fixed once here).
- `retried` (`*atomic.Bool`, passed by pointer through every retry generation) — CAS-guards
  against a third restart generation (`handleDriverFailure`, line 510): `retried.CompareAndSwap(false, true)`.

**What's new and unguarded**: a multi-attempt counter plus a pending backoff wait
introduces a state ("waiting to retry, attempt N of M, not yet restarted") that persists
across driver ticks, not just across goroutine generations. A manual "Retry now" UI action
(FR-8) is by definition a **second, independent trigger path** into the same restart logic
the automated backoff-expiry check would also trigger. If both are naive:

- **Double-restart race**: automated backoff-expiry fires `inst.Restart()` on the same tick
  the manual action's RPC handler also calls it (classic TOCTOU — both read "attempt not
  yet consumed" state, both proceed). `inst.Restart()`/`inst.Start()` are not obviously
  idempotent against concurrent invocation for the same instance (they mutate live tmux
  session/controller state), so a double-fire risks two concurrent restarts of the same
  worktree — likely worse than either the old `atomic.Bool` bug this replaces or a normal
  crash.
- **Attempt-count lost update** — this is the exact class of bug
  `.claude/rules/go-double-checked-locking.md` names ("read-lock → cache miss → compute →
  write-lock → conditional store... always return the locally-computed value, not the cache
  slot"). A naive `currentAttempt := inst.RetryAttempt; inst.RetryAttempt = currentAttempt + 1`
  done from two goroutines (the ticker loop's automated path and the manual-retry RPC
  handler) without a single atomic CAS/increment can lose an update: both read attempt=1,
  both compute 2, one write clobbers the other, and one real attempt goes uncounted —
  letting `max_attempts` be silently exceeded, or (worse for the "3/3 → permanently_failed"
  acceptance criterion in requirements.md #1) never being reached because the counter
  under-counts.

**Recommendation**: use a single `atomic.Int32` (not a raw field) for the attempt counter,
mutated only via `Add`/CAS, and route *both* the automated-backoff-expiry restart and the
manual "Retry now" restart through the same one restart function gated by the same CAS
(e.g. a `retryInFlight atomic.Bool` guard analogous to `driverRunning`, claimed before
either path calls `inst.Restart()`). "Retry now bypasses backoff" (FR-8) should mean
"immediately satisfy the `nextRetryAt` check" (set it to now, or add a bypass bool the same
tick-check reads), not "call `inst.Restart()` directly from a second code path" — that
second path is exactly what reintroduces the CAS-guard gap.

## 3. `tmux_exited` does not exist yet, and naively wiring it into `retry_on` creates thundering-herd risk on every routine service restart

Confirmed by grep: `tmux_exited`/`TmuxExited`/`TmuxSessionLost` appear nowhere in
`session/` or `server/` today — this is a wholly new condition, not a rename of an
existing one, so it inherits no existing false-positive-avoidance logic for free.

**The known, already-documented failure mode this collides with**:
`.claude/rules/tmux-keep-server-on-restart.md` states plainly that a full service restart
kills the tmux server and **every** live tmux session simultaneously unless
`--tmux-keep-server` is passed — and that this class of bug was live and confirmed on this
exact repo (`make install-service` run twice destroyed every session including the one
running the Claude Code conversation debugging it). If a routine `make install-service`
deploy runs *without* that flag correctly wired (the rule file documents this has actually
happened), every active session's pane disappears in the same instant. A `tmux_exited`
detector with no way to distinguish "the whole tmux server just died because of a deploy"
from "this one pane died because of an OOM kill or a laptop sleep" will misclassify N
simultaneous single-pane losses as N independent crashes, and if `retry_on` includes
`tmux_exited` for those sessions, the driver would kick off N independent retry sequences
— worse, if backoff has no jitter, all N sessions computed their `initial_delay` from the
same failure timestamp and will fire their first retry (and cross the same
`driverMinRuntimeBeforeRetry`-style thresholds) at approximately the same moment, hammering
tmux session creation, worktree git operations, and any external APIs (GitHub, LLM
provider) all at once right after the process that hosts them finished restarting — the
worst possible time for a burst of concurrent work.

**This repo already has the fix pattern for exactly this shape of problem**, one file over
from the backoff-gate precedent in §1: `session/backlog_remediation.go`'s
`evaluateRemediation` has an explicit **restart-grace** case
(`remediationGrantedRestartGrace`, lines 79-84, 104-108) — when `bootTime.After(row.LastCheckedAt)`
(the server restarted since this row was last checked), the pending remediation is granted
*without consuming an attempt* from the budget, exactly to avoid punishing every stuck item
for a routine process restart. `serverStartTime` is captured once at package init
(`session/backlog_remediation.go:51`) specifically so this comparison is cheap and
in-memory.

**Recommendation**: (a) reuse or mirror the `serverStartTime` restart-grace pattern for
`tmux_exited` specifically — a pane loss detected within some short window of process start
should not consume a retry attempt (or, at minimum, should not count toward `max_attempts`
the same way an in-process crash does), and (b) add jitter on top of the exponential delay
(`initial_delay * 2^attempt * (1 ± jitter_fraction)`) so that even in the case where several
sessions *do* legitimately need to retry around the same time, they don't all restart in
the same instant. Requirements.md's exponential-backoff FR (#2) doesn't mention jitter —
flag this explicitly for Phase 3 planning as a gap, since "exponential backoff alone" does
not prevent a synchronized retry storm when the trigger (a service restart, or a shared
upstream outage) hits every session at once.

## 4. Retrying into a broken worktree can make things worse each attempt, and today's driver has no signal for "is the worktree itself the problem"

The existing `handleDriverFailure` restart path (`session_driver.go:534-552`) always calls
`inst.Restart()`/`inst.Start()` unconditionally on the same worktree — reasonable for a
single retry, but generalizing to `max_attempts` up to (say) 5 means a session whose
*actual* failure cause is a corrupted/conflicted worktree (e.g. a bad merge state, a lock
file left over from a killed git process, disk full) will be restarted into the same broken
state repeatedly, each time re-sending a continuation prompt derived from a conversation
that never got past the same blocking error. Nothing in `buildContinuationPrompt` or the
restart path inspects *why* the process exited beyond the coarse
`crashed`/`stalled`/(new) `tmux_exited` bucket — there's no distinction between "transient"
(process OOM-killed, tmux pane lost to a laptop sleep — worth retrying) and "deterministic"
(git worktree in an unrecoverable state — retrying is guaranteed to fail identically every
time and just burns the attempt budget while a human waits for `permanently_failed`
instead of finding out immediately).

**Recommendation**: not necessarily in scope to *detect* worktree corruption specifically
(that's a bigger feature), but Phase 3 planning should at minimum note this as a known
limitation, and consider whether the continuation prompt on retry ≥2 should mention
"this is retry attempt N; if this looks like the same failure as before, investigate the
worktree state directly" — cheap insurance against burning the full backoff schedule (which
per the sibling `stale-session-detection` project's own schedule precedent, `session/backlog_remediation.go`'s
`remediationBackoffSchedule`, can span up to 72h between later attempts) on a failure mode
no amount of retrying can fix.

## 5. Notification spam on `permanently_failed` — apply the sibling project's edge-triggered lesson

The sibling `project_plans/stale-session-detection/research/pitfalls.md` (§2, read for this
research) already worked out the general shape of this problem for a related feature: fire
notifications **edge-triggered** (once on the transition into the terminal condition), not
**level-triggered** (re-evaluated and re-fired every time some poller tick observes the
condition still true). The existing `markSessionNeedsAttention` → `rq.Add()`
(`session/session_driver.go:576-596` → `session/queue/queue.go:215`) is naturally
idempotent *for the review queue itself* — `Add` is keyed by `SessionID` and only fires an
`OnQueueUpdated` observer callback when something in the item actually changed (reason,
priority, context, or `LastActivity`) — but that dedup is scoped to the review-queue's own
observers, not to whatever new notification-bus call `permanently_failed` is meant to
trigger (per requirements FR-5, "reuse the existing notification bus"). A naive
implementation that calls the notification bus every time the terminal state is
(re-)computed — e.g. on every read of a session whose status is already `permanently_failed`
— will spam. Fire the notification exactly once, at the moment the state transitions into
`permanently_failed` (the same tick `max_attempts` is exhausted), and treat "Retry now"
resetting out of `permanently_failed` (FR-8) as re-arming that one-shot so a *second*
exhaustion later fires a fresh notification — the same "resolve/reopen" shape
`MarkStuck`/`MarkStuckNotified` and the sibling project's pitfalls doc both already
converged on.

## 6. Existing test coverage in this area — no known flakes found, but the surface area is about to change materially

`grep -rn "TestSessionDriver\|TestHandleDriverFailure\|TestStartSessionDriver" session/*_test.go`
found tests in `session/session_driver_test.go` (`TestStartSessionDriver_Idempotent`,
`TestSessionDriver_SecondFailure_MarksNeedsAttention`) and a reference comment in
`session/instance_approve_deny_test.go:132`. No `t.Skip`/`Flaky` markers exist in this file
today (`grep -rln "flaky\|Flaky\|t.Skip" session/session_driver_test.go` returned nothing) —
this is a currently-clean test area, not a pre-existing flake to inherit. However,
`TestSessionDriver_SecondFailure_MarksNeedsAttention` is testing the exact `atomic.Bool`
"retried" mechanism this feature replaces with a multi-attempt counter — that test (and
whatever table-driven tests cover `handleDriverFailure`'s CAS behavior) will need to be
rewritten, not just left passing incidentally, since the underlying state shape it asserts
against (`retried.CompareAndSwap`) is being removed. Per
`.claude/rules/fix-flaky-tests-dont-defer.md`'s general spirit (root-cause instability
rather than deferring it), Phase 5 implementation should treat any test that goes red
because of the `atomic.Bool` → counter migration as a signal to re-derive the equivalent
assertion for the new shape, not to relax or skip it.

## Summary of concrete recommendations for Phase 3 planning

1. **No new sleeping goroutine for backoff.** Persist `nextRetryAt`/attempt state and check
   it from the existing 2s ticker loop in `runSessionDriverWithPrompt`, mirroring
   `RemediationDue`'s poll-based gate (`session/backlog_remediation.go:168`). This sidesteps
   the goroutine-leak/cancellation gap entirely rather than requiring a new
   `context.Context`/stop-channel this file has never needed.
2. **Route manual "Retry now" through the same CAS-guarded restart path as automated
   backoff-expiry**, not a second independent call to `inst.Restart()` — use an
   `atomic.Int32` attempt counter (never a bare read-increment-write) and a
   `retryInFlight`-style `atomic.Bool` guard analogous to the existing `driverRunning`
   pattern, to close the double-restart and lost-update races described in §2.
3. **Give `tmux_exited` a restart-grace exemption** mirroring
   `evaluateRemediation`'s `remediationGrantedRestartGrace` (keyed off the same
   `serverStartTime`-at-init pattern), and **add jitter** to the exponential backoff — the
   requirements' FR-2 only specifies exponential backoff, which alone does not prevent a
   synchronized retry storm when many sessions fail at once (deploy-triggered tmux-server
   loss, or a shared upstream outage).
4. Note (don't necessarily solve) the "retrying into a permanently-broken worktree burns
   the whole attempt budget with guaranteed-identical failures" risk in the plan, and
   consider surfacing "this is retry N" in the continuation prompt as low-cost mitigation.
5. Fire the `permanently_failed` notification edge-triggered (once per exhaustion episode,
   re-armed by a manual reset), following the same resolve/reopen shape the sibling
   `stale-session-detection` pitfalls research already converged on for its own
   notification path.
6. Expect `TestSessionDriver_SecondFailure_MarksNeedsAttention` and related `atomic.Bool`
   -shaped tests to need rewriting (not incidental breakage to shrug off) once the counter
   replaces the boolean.
