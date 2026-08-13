# Research: existing retry/recovery landscape for session-retry-backoff

## 1. Existing "retry/recovery" mechanisms beyond `session_driver.go`

### 1a. The current single-retry mechanism (in-memory, not persisted)

`session/session_driver.go`:

- `runSessionDriver` (`session_driver.go:76-125`) declares `var retried atomic.Bool`
  (`session_driver.go:111`) as a **goroutine-local variable**, not a field on `Instance`.
  It's threaded by pointer through `runSessionDriverWithPrompt` and `handleDriverFailure`.
- `handleDriverFailure` (`session_driver.go:509-570`) does `retried.CompareAndSwap(false, true)`:
  first failure → restart immediately + spawn a new driver goroutine that inherits the
  same `*atomic.Bool` (so it won't retry again); second failure →
  `markSessionNeedsAttention` (adds to `ReviewQueue`, `session_driver.go:576-596`).
- **Nothing here survives an app/service restart.** `retried` lives only in process
  memory tied to the goroutine; there is no DB column, JSON field, or `Instance` struct
  field recording "this session has already used its one retry." A restart of the
  stapler-squad service resets every in-flight session's retry count to zero implicitly
  (the goroutine is gone; a new driver goroutine, if started, allocates a fresh
  `atomic.Bool`).
- `isOneShot()` (`session_driver.go:640-645`) excludes `backlog:triage` and
  `backlog:review` tagged sessions from ALL auto-retry — "retrying them could corrupt
  backlog state by re-triggering lifecycle transitions." Both call sites that gate on
  `retried.Load()` also gate on `isOneShot(inst)` first (`session_driver.go:203, 216`).

### 1b. The backlog-item-level backoff-gate — a mature, reusable, DB-persisted pattern

`session/backlog_remediation.go` implements **the actual pattern this feature should
mirror** (per the requirements doc's own instruction), even though it's scoped to
backlog *automation steps* (push, triage respawn, stale-work respawn, PR-fix respawn),
not agent *process* retries. Four call sites in `session/backlog_lifecycle.go` already
use it: `retryPushFailedWithBackoffGate` (:3963), `retryOrphanedTriageWithBackoffGate`
(:2786), `remediateStaleWorkWithBackoffGate` (:2465), `remediatePRFixWithBackoffGate`
(:4292), plus `autoReopenWithBackoffGate` (:1132) and `markAbandonedReview` (:2041).

Key shape (`session/backlog_remediation.go:1-227`):

- **Backoff schedule is a `[]time.Duration` table, not a formula**:
  `remediationBackoffSchedule = []time.Duration{30m, 2h, 8h, 24h, 72h}` (:31-37) — gap
  BEFORE each numbered attempt after the first (attempt 1 is always immediate).
  `MaxRemediationAttempts = len(remediationBackoffSchedule)` (:45), derived not
  hardcoded twice.
- **State is DB-persisted**, not in-memory: a `BacklogStuckState` row per
  `(itemID, reason)` tracks `RemediationAttempts int32`, `NextRemediationAt *time.Time`,
  `LastCheckedAt`, `GraceBootTime *time.Time` (via `Storage.MarkStuck` /
  `RecordRemediationAttempt` / ent-backed `EntRepository`). This directly answers "does
  timer state survive restart" for the sibling mechanism: **yes**, because it's a DB row,
  not a goroutine-local variable — the opposite of `session_driver.go`'s current design.
- **Explicit restart-grace handling** (`evaluateRemediation`, :96-110): if the process
  restarted since the row was last checked (`bootTime.After(row.LastCheckedAt)`) AND
  this boot hasn't already consumed its one grace pass for this row
  (`GraceBootTime`), it grants an immediate attempt **without consuming attempt budget**
  (`remediationGrantedRestartGrace`). This is neither "ignore the backoff on restart"
  nor "silently lose the state" — it's a third designed behavior: retry once per boot,
  for free, then resume honoring the persisted schedule. This is a strong candidate
  pattern for the "app restart while mid-backoff-delay" edge case in this feature (see §2).
- **Atomic gate + record in one call**: `RemediationDue` (:168-193) checks-and-records
  in a single DB round trip specifically so that async/concurrent callers "can never
  double-count across overlapping sweep ticks or concurrent event callbacks" (doc
  comment, :150-153) — directly relevant to the "concurrent retries when multiple
  sessions crash simultaneously" edge case, though that scenario is naturally
  per-session there (each session has its own row) so cross-session contention isn't
  actually the risk; same-session concurrent evaluation (e.g. two sweep ticks racing)
  is what it guards.
- **`RemediationBlocked`** (:213-227) is a read-only, non-mutating peek used when one
  reason's action is only useful if a different reason's gate would also allow the
  downstream step — a "check without spending a budget unit" primitive. Not obviously
  needed for session-level retry (single reason per session, not chained), but worth
  knowing exists if `tmux_exited`/`crashed`/`stalled` end up as separate gated reasons
  competing for the same session.
- **Manual reset primitives already exist at the backlog-item level**:
  `ResetStuckRemediation` / `BulkResetStuckRemediation` (referenced in the
  `ErrRemediationParked` doc comment, :53-58) — precedent for this feature's "manual
  Retry now… including resetting from `permanently_failed`" (req #8): an operator-driven
  reset-then-retry action, not a second independent code path.

**Recommendation for planning**: don't literally call `Storage.RemediationDue` for
agent-session retries (it's keyed to `(itemID, domain.StuckReason)` = backlog items, not
sessions) — but the *shape* (duration-table schedule, DB-persisted attempt
count + next-eligible-at, restart-grace pass, atomic check-and-record, manual reset)
is exactly the reusable pattern to replicate for session-level retry state, for
consistency and because it already solved the restart-persistence problem this feature
currently lacks.

## 2. Edge cases

| Edge case | Current-code answer / gap |
|---|---|
| Session manually stopped mid-backoff-wait | No backoff-wait state exists today (retry is immediate), so there's nothing to interrupt yet — new code entirely. Design must ensure a manual Stop during the delay window cancels the pending retry goroutine/timer rather than racing a delayed restart against a user-initiated stop. `handleDriverFailure`'s existing restart path already checks `inst.GetEffectiveStatus()` (`:536`) before restarting — but that's a synchronous decision at restart time, not a delay-then-restart with a stop-cancellation hook. |
| Session manually stopped during an active retry attempt | Existing restart path (`handleDriverFailure` → `inst.RecoverFromStopped()` / `inst.Restart(false)`, `:537-552`) has no explicit interruption handling for "user hit Stop while the restart itself is in flight" — `inst.Start`/`inst.Restart` would presumably just run to completion; no cancellation context is threaded through `handleDriverFailure`. Worth checking whether a manual stop concurrent with `inst.Restart(false)` can leave the instance in an inconsistent status (this predates the feature but backoff makes the window longer and more likely to be hit by a human). |
| App restart while mid-backoff-delay | Confirmed **not persisted today** for the single-retry case (§1a) — `retried` is goroutine memory, and there is no delay/timer state at all today since retry is immediate. For the new multi-attempt version, the backlog-remediation pattern (§1b) is the model: persist `attempt count` + `next_retry_at` per session (DB or `config/state.go`-style JSON, per the requirements' "no new datastore" NFR — likely alongside existing session state persistence, not a new SQLite table) and apply the same restart-grace logic rather than either (a) silently losing all retry progress on every restart (today's behavior) or (b) blindly honoring a stale `next_retry_at` that might now be wildly wrong relative to wall-clock if the box was asleep/OOM for hours. |
| Concurrent retries when multiple sessions crash simultaneously | Each session already runs its own independent driver goroutine (`inst.driverRunning atomic.Bool`, `instance.go:373-375`, one per `Instance`) — no shared retry budget or global rate limiter exists across sessions today, and the requirements don't ask for one. The backlog-remediation gate's atomicity guard is about a *single* item/reason race, not cross-session load-shedding; if 5-10 sessions crash at once (e.g. an OOM event, which the remediation schedule's doc comment explicitly calls out as the motivating scenario, `backlog_remediation.go:28-30`), each session's driver independently retries per its own policy — no coordination point currently exists to stagger/throttle a thundering herd of simultaneous restarts. Worth flagging as a design question even if out of scope. |
| Session hits stale-detection AND crash-retry at the same time | The sibling `stale-session-detection` project (`project_plans/stale-session-detection/`) defines a **new, fourth, independent staleness threshold** for the main session-list card indicator (default 30min, per its ADR-001), distinct from the Review Queue's 5min badge threshold (`review_queue_poller.go:49`) and `backlog_lifecycle.go`'s `maxWorkSessionStaleness` (2h, `:2098`). None of those three existing thresholds currently trigger a retry — they only notify/flag. This feature's req #9 asks to make staleness *optionally* trigger a retry as a *consumer* of stale-session-detection's config, not a second detector. Concretely: if a session is mid-backoff-wait (already retrying due to a crash) and ALSO crosses the staleness threshold, the design needs a single source of truth for "is this session currently under active retry management" so the staleness-triggered retry path doesn't double-fire a redundant restart on top of an already-scheduled one — likely a shared "retry in progress" flag/state check before the staleness trigger acts. |
| Worktree deleted/corrupted between retries | Not handled today — `handleDriverFailure`'s restart path calls `inst.Restart(false)` / `inst.Start(false)` (`:541,551`) with no explicit worktree-existence check before restarting; a failed restart falls through to `markSessionNeedsAttention` reactively (`restartErr != nil` branch, `:554-559`) rather than proactively detecting a missing/corrupt worktree before attempting the retry. Multi-attempt backoff makes this more likely to matter (more elapsed time between attempts = more opportunity for something external to remove the worktree, e.g. a `git worktree prune` or manual cleanup). |
| Retrying a one-shot/triage/review session type | `isOneShot()` (`session_driver.go:640-645`) currently excludes exactly two tags: `backlog:triage` and `backlog:review`. This exclusion should carry over unchanged — the requirements doc doesn't ask to change one-shot semantics, and the rationale ("retrying them could corrupt backlog state by re-triggering lifecycle transitions") applies identically to a 3-attempt exponential-backoff retry as to today's 1-attempt retry. The new `retry_on`/`max_attempts` config should be short-circuited (or simply not consulted) for `isOneShot(inst) == true` sessions, same gate structure as today (`session_driver.go:203, 216`). |

## 3. Unstated needs

- **Backlog work sessions likely need different retry behavior than plain interactive
  sessions**, mirroring the existing asymmetry already in `session_driver.go`: backlog
  work sessions get a pre-restart *nudge* step (`driverBacklogNudgeDelay`/
  `driverBacklogNudgeGrace`, `attemptBacklogNudge`, `:409-417, 485-499`) before being
  treated as stuck, and `isOneShot()` fully excludes `backlog:triage`/`backlog:review`
  sessions from retry altogether. A generic `retry_on`/`max_attempts` config applied
  uniformly risks either (a) retrying a `backlog:work`-tagged session in a way that
  conflicts with `BacklogLifecycleListener`'s own stuck-item remediation (§1b) —
  two independent retry mechanisms acting on the same session-turned-backlog-item would
  be a real regression risk the requirements' NFR #3 explicitly wants to avoid — or
  (b) needing an explicit carve-out so backlog-tagged sessions defer entirely to
  `backlog_lifecycle.go`'s remediation gate instead of this feature's driver-level
  retry, the same way `isOneShot()` already carves out triage/review. This needs an
  explicit decision in plan.md, not just "reuse the config for all session types."
- **UI "why is this in backoff-wait" visibility** — requirements list a retry-count
  badge (after the fact, req #6) and retry history (req #7), but not a *live* "retrying
  in 45s" indicator while the delay is actively counting down. Given the single-user,
  5-10-parallel-sessions context (per requirements' Users/Consumers section), a user
  staring at a session card during an active backoff wait with no visible countdown or
  "why is nothing happening" explanation is a plausible confusion point — worth raising
  as a candidate addition to plan.md's UI scope (e.g. a `retrying in Ns` state on
  `StatusBadge`/`SessionCard`, sourced from the persisted `next_retry_at`), even though
  it wasn't explicitly requested in the source issue.
- **Notification content for `permanently_failed`** — req #5 says "reuse the existing
  notification bus," but doesn't specify whether the notification should include the
  full retry history (all N failure reasons) or just the final one. Given retry history
  is being built anyway (req #7), the notification should likely link to/summarize it
  rather than repeat only the last failure reason, so a user gets the full picture from
  the notification alone without having to open the session.
- **Interaction with `driverMinRuntimeBeforeRetry`** (`session_driver.go:56-59`, 5-minute
  floor before an unexpected exit is treated as a crash worth retrying vs. a normal
  completion) — this existing heuristic should be preserved as-is under the new policy;
  it's a "was this actually a crash" classifier that sits upstream of `retry_on`
  eligibility, not something the requirements ask to change, but the plan should say so
  explicitly so it isn't accidentally dropped during the `handleDriverFailure` rewrite.

## 4. `Status` enum fit for `permanently_failed`

`session/instance.go:24-47` defines `Status int` with exactly six values: `Creating`,
`Active`, `Paused`, `Stopped` (doc comment: "a terminal state: the instance has been
shut down and cannot transition further"), `Hibernated`, `Restoring`. There is **no
`NeedsAttention` (or similar) `Status` value today** — the existing single-retry
give-up path does NOT change `Status` at all; it only adds an entry to the separate
`ReviewQueue` (`session/review_queue.go`, `ReviewItem{Reason: ReasonStale, ...}` via
`markSessionNeedsAttention`, `session_driver.go:576-596`). `ReasonStale` is reused there
as "the closest existing reason" per an explicit code comment (`:587`) — i.e. today's
give-up state is a mislabeled "Stale" review item, not a distinct reason at all.

Implications for `permanently_failed`:
- It does **not fit cleanly as a new `Status` value** the way `Stopped` does — `Stopped`
  is explicitly documented as terminal/non-transitionable, and a `permanently_failed`
  session is still logically "stopped" at the process level (the tmux/PTY process is
  gone) but needs its own distinguishable state for the UI/retry-reset flow (req #8:
  "manual Retry now… including resetting from `permanently_failed`" implies transitions
  OUT of this state, unlike `Stopped`'s documented one-way terminality).
- Two viable directions, to weigh in plan.md: (a) add a new `Status` constant
  (e.g. `PermanentlyFailed Status = 6`) alongside the existing six, updating every
  exhaustive `switch` over `Status` in the codebase (worth a `grep -rn "case Stopped:"`
  sweep in plan.md's task breakdown to find all call sites that need a new case) — or
  (b) keep `Status = Stopped` and add a *separate* first-class field (e.g.
  `RetryState`/`FailureReason` struct on `Instance`, alongside `PauseReason string`
  which is the closest existing precedent, `instance.go:284`) that distinguishes
  "stopped because retries exhausted" from "stopped because the user stopped it,"
  without touching the `Status` enum's transition semantics. Given `ReasonStale` was
  already reused as a stand-in for "no better reason exists" (the exact anti-pattern
  req #5 wants `permanently_failed` to fix), a dedicated field/reason is more consistent
  with the codebase's existing `PauseReason`-style precedent than growing the `Status`
  enum.
- The `config/config.go:597` / `session/repository.go:374-379`
  `ReworkCapOverride *int` pattern (global default + per-item override, nil-pointer =
  "use global default," non-nil = "this item's own value replacing the global") is the
  established convention for "global default + optional per-session override" that
  requirement #1 asks for — a `*RetryPolicy` pointer field (nil = inherit global) on the
  session's persisted repository struct is the idiomatic fit, not a copy-everything
  struct.
