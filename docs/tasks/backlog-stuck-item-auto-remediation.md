# Backlog Stuck-Item Auto-Remediation — Design (2026-07-20)

## Problem

As of the 2026-07-19/20 `backlog-feature-improvement` audit + fix session (see
`docs/tasks/backlog-feature-improvement.md`), the backlog reconciliation loop detects 7
distinct `StuckReason`s but only 3 have any automated recovery attempt at all
(`abandoned_review`, `autonomous_stuck`, `bouncing` — the latter two just gained circuit
breakers in PRs #180/#181 today). The rest (`push_failed`, `pr_ready_unmerged`,
`orphaned_triage`, `rework_cap`, `stale_work`) are notify-only: an operator has to
manually intervene on every single occurrence, even when the fix is mechanical (retry a
transient push failure, direct-merge a PR when auto-merge silently isn't taking effect,
re-trigger triage, retry a rework cycle, restart a genuinely-dead work session).

User request (2026-07-20): build automated remediation for **every** stuck reason,
**properly rate-limited** — explicitly informed by today's incident, where the existing
`reworkCap` count-based limit alone was insufficient once a bug fed it doomed retry
cycles at 5-15 minute intervals; a count cap without time-based backoff can still burn
through its budget in well under an hour.

## Decisions (user-confirmed 2026-07-20)

1. **Scope: every stuck reason, no exceptions** - including `rework_cap` and
   `stale_work`, which were previously deliberate human-judgment stopping points. Those
   get folded into the same automated-attempt-then-park policy as everything else.
2. **Backoff shape: exponential + hard cap, sized for OOM-restart bursts.** Per open
   `BacklogStuckState` row: attempt 1 @ +30m, attempt 2 @ +2h, attempt 3 @ +8h,
   attempt 4 @ +24h, attempt 5 @ +72h, then **park**: stop attempting, send one final
   notification, require a manual reopen/override (or the reset below) to try again.

   Revised bigger than the original 5m/15m/1h/4h/12h draft (2026-07-20, user feedback):
   a meaningful fraction of today's "failures" weren't genuine task failures at all - the
   `stapler-squad` service itself gets OOM-killed and systemd-restarted under memory
   pressure (this is expected/tolerated infrastructure behavior - see `accec7ad
   chore(systemd): bound service memory and tolerate OOM-restart bursts`, which explicitly
   raised `StartLimitIntervalSec/Burst` because OOM-restart *bursts*, not just single
   events, are a known occurrence on this machine). When the process restarts, any
   in-flight `AutonomousDriver` goroutine is simply gone - no completion callback fires,
   the work session never cleanly resumes, and the next reconciliation pass treats it as
   a failed/stuck attempt indistinguishable from a genuine task failure. A short backoff
   schedule retries right back into the same pressure window and burns the attempt budget
   on infrastructure noise, not signal. 72h as the final gap before parking gives real
   headroom for a sustained-pressure episode to clear before the item gives up.

## Data model

Add two nullable-safe fields to `BacklogStuckState` (`session/ent/schema/backlog_stuck_state.go`):

- `remediation_attempts int32` (default 0) - incremented each time a remediation action
  is actually attempted (not each detection-sweep tick).
- `next_remediation_at *time.Time` (nullable) - when the row becomes eligible for the
  next attempt. `nil` after the row is first opened means "eligible immediately" (attempt
  1 fires on first detection, no initial delay). "Parked" (attempts exhausted) is
  represented purely by `remediation_attempts >= 5` - the dispatcher checks that BEFORE
  it ever looks at `next_remediation_at`, so no separate boolean is needed and
  `next_remediation_at` does not need to be forced back to `nil` on the attempt that
  parks the row; whatever schedule-derived value it holds at that point is simply never
  consulted again.

Regenerate via the exact command in `session/ent/generate.go`
(`--feature sql/upsert` - see `.claude/rules/ent-schema-generation.md`, omitting this flag
silently breaks existing upsert methods elsewhere in this schema).

## Admin reset capability (required in Phase A, not deferred)

Today there is no way to clear an item's remediation-attempt count short of editing the
DB directly - `server/services/backlog_service_stuck.go` only exposes `ListStuckBacklogItems`
and `SnoozeStuckItem` (snooze suppresses visibility/notification, it does not reset
attempts or backoff). User-requested (2026-07-20), motivated directly by the OOM-false-failure
problem above: after a service-restart storm resolves, an operator needs to bulk-clear the
attempt counters it spuriously consumed, not re-fight each item one at a time.

Two new RPCs on `BacklogService`, alongside the existing stuck-item surface:

- **`ResetStuckRemediation(item_id, reason)`** - single row: sets `remediation_attempts = 0`,
  `next_remediation_at = NULL`, clears `notified_at` (so a fresh attempt/notification cycle
  can fire immediately rather than waiting on stale dedup state). Reason must be a valid,
  currently-open `StuckReason` for that item (mirrors `SnoozeStuckItem`'s validation).
- **`BulkResetStuckRemediation(reason?, only_parked=true)`** - resets every open,
  un-resolved `BacklogStuckState` row matching the optional reason filter (all reasons if
  omitted). `only_parked` (default true) restricts to rows that actually hit the 5-attempt
  cap - the natural "something upstream broke a batch of these, give them all a fresh
  shot" admin action - but allow `false` for a full reset regardless of attempt count, for
  the rarer case of "we know a specific outage window corrupted the count of items that
  hadn't parked yet either."

Surface both from the same admin/notification affordance that already exists for
stuck-item visibility (`/unfinished`) - a "Reset all parked" bulk action alongside the
per-item snooze/resolve controls is sufficient for Phase A; a more targeted "reset items
parked between time X and Y" filter can wait for real usage to show it's needed.

## Remediation dispatcher

One new function, e.g. `attemptRemediation(ctx, er *EntRepository, row OpenStuckStateData) error`,
called from the existing periodic reconciliation sweep (`session/backlog_lifecycle.go`,
alongside `selfHealStuck`/`reconcileBouncingItems`/etc. - consider whether it replaces or
wraps those, since several already contain the "detect" half of this logic and just need
the "act" half added). For each open, un-snoozed row:

1. Skip if `remediation_attempts >= 5` (parked - already notified when it hit the cap,
   per existing notify-once dedup).
2. Skip if `next_remediation_at != nil && time.Now() < *next_remediation_at` (not yet due).
3. Skip if the reason's existing circuit breaker says stop
   (`IsRepeatedFailure`/`IsRepeatedNoVerdictFailure` for bouncing/autonomous_stuck - reuse,
   don't duplicate) - but note a circuit-breaker stop and a backoff-cap stop are different
   outcomes worth distinguishing in the parked notification's wording (one says "this
   keeps failing identically," the other says "we tried enough times, over enough time,
   without knowing why").
4. Otherwise: call the reason-specific remediation action (below), increment
   `remediation_attempts`, compute+set `next_remediation_at` from the backoff table keyed
   by the new attempt count.

## Per-reason remediation actions (Phase A wires only these 3; the rest are Phase B, out of scope)

| Reason | Current state | Remediation action to add/wrap |
|---|---|---|
| `bouncing` | Has `AutoReopenAfterFailedReview` + `IsRepeatedFailure` circuit breaker (PR #180/#181) | Wrap existing respawn call with the backoff gate above - same action, now time-limited not just count-limited |
| `autonomous_stuck` | Has `AutoRespawnAutonomousWork` + `IsRepeatedNoVerdictFailure` (PR #180) | Same - wrap with backoff gate |
| `abandoned_review` | Has `markAbandonedReview`'s respawn (PR #168) | Same - wrap with backoff gate |

(Phase B, NOT in scope for you: push_failed, pr_ready_unmerged, orphaned_triage,
rework_cap, stale_work - these currently stay notify-only, just make sure your dispatcher
is written so a follow-on PR can add their remediation functions without restructuring
the core engine.)

## Restart-grace: don't count a service-restart interruption as a genuine failure

The backoff schedule and reset RPCs above are mitigations for the OOM-restart problem -
bigger gaps and a manual undo. The actual root fix is cheaper than it sounds and belongs
in Phase A, not a future enhancement: **don't consume an attempt at all when the
detected "failure" coincides with a service restart.**

The server already knows its own boot time (process start). When the remediation
dispatcher is about to increment `remediation_attempts` for a row, check whether the
server's boot time falls after that row's `last_checked_at` (i.e. the process restarted
since the last time this item was known-good) - if so, this cycle's detection is likely a
restart artifact (an in-flight `AutonomousDriver` goroutine simply vanished mid-loop, no
completion callback, no real signal about whether the approach was working), not evidence
the remediation attempt itself failed. In that case: still *attempt* remediation (respawn
as normal), but do NOT increment `remediation_attempts` or advance `next_remediation_at`
for that one cycle - give the item exactly one "it wasn't your fault" pass per restart, not
unlimited free passes (track via a `last_restart_grace_used_at` field or similar, or a
`grace_boot_time` field storing which boot's grace this row already consumed).

This should eliminate most of the false-failure accumulation at the source. The backoff
schedule and reset RPCs remain necessary regardless - for genuine repeated task failures,
and for cleaning up any counts that already accumulated before this ships.

## Safety requirements (non-negotiable, given today's incident)

- Every remediation action MUST go through the backoff gate - no reason gets a bespoke
  bypass "just this once."
- The circuit breakers already added today (`IsRepeatedFailure`,
  `IsRepeatedNoVerdictFailure`) must be checked *before* attempting, for every reason
  where they apply - don't let the new backoff-cap policy alone carry safety; two
  independent stopping conditions is the point.
- Write a regression test proving the backoff schedule itself: an item stuck 5 times in
  rapid succession (seconds apart, simulating exactly today's failure mode) results in at
  most 5 remediation attempts total, with the 2nd-5th attempts correctly delayed, not
  immediate.

## Suggested delivery split

**Phase A (this PR):** schema migration (including restart-grace field), backoff
dispatcher with the revised 30m/2h/8h/24h/72h schedule, restart-grace logic, the two reset
RPCs (single + bulk) and a minimal `/unfinished` UI affordance for the bulk reset, and
wiring the 3 reasons that already have an existing respawn action (`bouncing`,
`autonomous_stuck`, `abandoned_review`) through the dispatcher. This validates the core
engine - including the two pieces the user asked for directly - against real,
already-working recovery paths before extending to net-new remediation logic.

**Phase B (follow-on PR, NOT your scope):** the 5 new remediation actions (`push_failed`,
`pr_ready_unmerged`, `orphaned_triage`, `rework_cap`, `stale_work`) - each is independent,
could even be split further/parallelized.

## Addendum (2026-07-20, post-brief): manual "kick off remediation now"

User request after the original design conversation: an operator-triggered escape hatch
that runs a remediation attempt immediately, without waiting on `next_remediation_at`.

Add a third RPC alongside `ResetStuckRemediation`/`BulkResetStuckRemediation`:

- **`TriggerRemediationNow(item_id, reason)`** - immediately calls the same
  reason-specific remediation action the dispatcher would call, bypassing only the
  `next_remediation_at` timer check. It still goes through every other gate: it
  increments `remediation_attempts` exactly like a normal dispatcher-triggered attempt
  (an operator-initiated attempt is still a real attempt - it counts toward the 5-attempt
  cap, so this cannot be used to quietly exceed the safety limit), and it is still subject
  to whatever circuit breaker the wrapped action itself checks
  (`IsRepeatedFailure`/`IsRepeatedNoVerdictFailure`). If the row is already parked
  (`remediation_attempts >= 5`), reject with a clear error pointing the operator at the
  reset RPC instead of silently un-parking it - "retry now" and "reset the counter" stay
  two distinct, explicit operator actions, never combined into one implicit behavior.

Frontend: a "Retry now" button next to each open stuck item in the `/unfinished` UI,
alongside the reset-related UI, calling this RPC with the same loading/error/success
feedback pattern as the other stuck-item actions on that page. Disabled (with a reason
shown) when the row is already parked, pointing at the reset action instead.

## Update — 2026-08-21: parking is no longer permanent — cold-retry heartbeat (BUG-083)

This design's original decision ("park: stop attempting... require a manual reopen/override
... to try again") turned out to have a real, live gap: nothing ever automatically un-parks a
row. Confirmed live — PR #535 fixed the bug (`classifyHeadlessCallError` misclassification)
that had been parking `orphaned_triage` items, but the 20 items it had already parked sat
stuck for up to 2 weeks until the 2026-08-21 `backlog-feature-improvement` audit noticed the
correlation and manually called `BulkResetStuckRemediation`. Sixth recorded instance of "a fix
closes the write side of a gap but not the recovery side" (`docs/tasks/backlog-feature-improvement.md`,
tracked since 2026-07-27).

**Fix**: `session/backlog_remediation.go` adds `remediationColdRetryInterval` (7 days). Once a
row parks (`remediation_attempts >= MaxRemediationAttempts`), `next_remediation_at` is
repurposed to hold a cold-retry deadline instead of going unused — `evaluateRemediation` grants
one retry (`remediationGrantedColdRetry`) whenever that deadline has passed, pins
`remediation_attempts` at the cap (so the row stays eligible for the next cold retry too,
indefinitely), and does not re-fire the one-time "auto-remediation exhausted" notification.
This lives in the one shared gate (`RemediationDue`), so it applies to every `StuckReason` that
goes through it (`orphaned_triage`, `bouncing`, `push_failed`, `stale_work`,
`abandoned_review`, ...), not just the reason from the live incident. See
`docs/bugs/fixed/BUG-083-*.md` for the full writeup.

`ResetStuckRemediation`/`BulkResetStuckRemediation` are unchanged and still useful for an
operator who wants a row's full fast 5-attempt budget back immediately rather than waiting for
the next weekly heartbeat.
