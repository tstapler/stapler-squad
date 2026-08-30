# Requirements: session-retry-backoff

**Date**: 2026-08-06
**Source**: Migrated from https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/49
(backlog item `1f7a0219-b62f-4519-b4c5-d2243b674132`)
**Type**: feature — extends an existing single-retry mechanism into a configurable,
multi-attempt, backoff-gated one; adds UI surfacing
**Complexity**: 3 — touches session lifecycle, config, proto, and UI; must not
regress the existing crash-recovery path

## Problem Statement

The original request describes retry as if it doesn't exist: "a failed session stays
in a failed state until the user manually intervenes." **That premise is only partly
true.**

**Pre-implementation research finding (this triage pass, 2026-08-06):** a single-retry
crash/stall recovery mechanism already exists in `session/session_driver.go`
(`StartSessionDriver` / `handleDriverFailure`, lines ~502-570):

- Detects three of the four failure modes the issue asks for: unexpected process exit
  before the initial prompt (`session_driver.go:202-209`), unexpected exit after the
  initial prompt (`session_driver.go:216-236`), and inactivity/stall — "stuck at Ready
  for >10 min" (`driverInactivityTimeout`, `session_driver.go:44`, wired at
  `session_driver.go:423`). It does **not** distinguish `tmux_exited` (pane/session lost
  entirely, e.g. machine sleep or OOM) from a plain process crash — both routes through
  the same `handleDriverFailure` path with no tmux-liveness-specific reason code.
  Requirements below close this gap since the issue explicitly names `tmux_exited` as a
  distinct retry-on condition.
- On first failure: restarts the session **immediately** (no delay) with a
  JSONL-derived continuation prompt (`handleDriverFailure` → `runSessionDriverWithPrompt`,
  `session_driver.go:568-569`) — this already satisfies "re-use the same worktree" and
  roughly "prepend context to the resumed session" (it derives a continuation prompt from
  the prior JSONL, not a literal "Previous attempt failed due to {reason}" string).
- On second failure: gives up and calls `markSessionNeedsAttention`
  (`session_driver.go:572-583`), which adds the session to its `ReviewQueue` — there is
  no distinct `permanently_failed` state, just "needs human review."
- **What's genuinely missing** (confirmed absent by grep across `session/`, `config/`,
  `proto/`, and `web-app/src/components/sessions/SessionCard.tsx`):
  1. **Configurability.** `max_attempts` is hardcoded to 1 retry (`atomic.Bool`, not a
     counter); `initial_delay_seconds`/`backoff`/`max_delay_seconds` don't exist — retry
     is immediate, not backed off at all. No per-session or global JSON config surface
     for any of this (`config/` has no `RetryPolicy` type).
  2. **`tmux_exited` as a distinct, separately-configurable condition** vs. plain
     `crashed`/`stalled` (see above).
  3. **UI surfacing.** No retry-count badge on the session card, no retry history view,
     no manual "Retry now" action anywhere in `web-app/src/components/sessions/`.
  4. **`permanently_failed` as a first-class terminal state** distinct from the existing
     generic `NeedsAttention`/review-queue path — today "exhausted retries" and "any other
     reason a human needs to look" are the same bucket.
  5. **Integration with stale-session detection (#41).** The sibling
     `project_plans/stale-session-detection/` plan (also triaged 2026-08-06) is adding a
     *configurable* staleness threshold and notification; this item's "stale sessions can
     optionally trigger a retry" behavior should be built as a consumer of that work's
     config/threshold, not a second independent staleness detector. Sequence this item
     after (or alongside, sharing the config surface with) stale-session-detection —
     don't duplicate threshold config.

## Users / Consumers

Single user (Tyler), running 5-10 parallel Claude Code / Aider agent sessions via the
stapler-squad web UI. Consumers: the session lifecycle driver (`session_driver.go`), the
backlog automation loop (indirectly, via session state), and the web UI session card /
history view.

## Functional Requirements

1. **Configurable retry policy**, global default + optional per-session override:
   `enabled`, `max_attempts`, `backoff` (`exponential` — the only mode requested; no
   need to build a strategy-plugin system for one mode), `initial_delay_seconds`,
   `max_delay_seconds`, `retry_on` (subset of `crashed`, `stalled`, `tmux_exited`).
   Replaces the current hardcoded single-retry `atomic.Bool` in `session_driver.go` with
   an attempt counter gated against `max_attempts`.
2. **Exponential backoff delay** (`initial_delay * 2^attempt`, capped at
   `max_delay_seconds`) before each retry — replaces today's immediate restart. Must not
   block the driver goroutine in a way that prevents the session from being
   manually retried or stopped during the wait.
3. **`tmux_exited` detected as a distinct condition** from `crashed` (pane/session gone
   vs. process exited), each independently gate-able via `retry_on`.
4. **Same-worktree reuse and continuation-context prompt** on every automated retry
   (already exists — preserve, extend the existing continuation-prompt derivation to
   prepend an explicit "Previous attempt failed due to {reason}" line per the issue's
   proposed behavior, since today's continuation prompt doesn't state the failure reason).
5. **`permanently_failed` terminal state** once `max_attempts` is exhausted — distinct
   from generic `NeedsAttention` — with a user notification (reuse the existing
   notification bus per `stale-session-detection`'s research, don't build a new one).
6. **UI: retry-count badge** on the session card ("Attempt 2/3").
7. **UI: retry history** — at minimum reason + timestamp per attempt, surfaced in the
   existing session history view.
8. **UI: manual "Retry now"** action that bypasses the backoff delay and (if exhausted)
   resets from `permanently_failed`.
9. **Stale-session integration**: an optional setting (default off) that, when a session
   crosses the stale-session-detection threshold, triggers a retry attempt instead of
   only notifying — consuming that feature's threshold/config rather than adding a
   second one.

## Non-Functional / Explicitly Out of Scope

- No new retry "strategies" beyond exponential backoff — YAGNI per the issue's own
  proposal (`"backoff": "exponential"` is the only value in its example).
- No SQLite-backed retry queue (Sortie's approach, cited as competitive context) — this
  app already persists session/instance state via its existing config/state files;
  don't add a new datastore for a per-session attempt counter and a handful of history
  entries.
- No changes to the *backlog item*-level retry/backoff-gate mechanisms already in
  `session/backlog_lifecycle.go` (`retryPushFailedWithBackoffGate`,
  `retryOrphanedTriageWithBackoffGate`, etc.) — those retry backlog *automation steps*
  (push, triage), a different concept from retrying a crashed *agent session process*.
  Out of scope; no overlap to resolve beyond naming clarity.

## Acceptance Criteria (draft — refined further in plan.md)

1. A session with `retry.enabled=true, max_attempts=3` that crashes 3 times is retried
   3 times with increasing exponential delay, then transitions to `permanently_failed`
   and fires a notification.
2. A `tmux_exited` failure is retried only when `tmux_exited` is present in `retry_on`;
   a policy with only `["crashed"]` does not retry on tmux/pane loss.
3. Every automated retry re-uses the same worktree and the resumed session's first
   message states the failure reason.
4. The session card shows "Attempt N/max_attempts" once at least one retry has occurred.
5. Retry history (reason + timestamp per attempt) is visible in the session history view.
6. A manual "Retry now" action restarts the session immediately, ignoring the current
   backoff delay, including from a `permanently_failed` state.
7. Existing single-retry crash-recovery behavior is not regressed for sessions with no
   explicit retry policy configured (i.e. a sane default policy preserves at least
   today's "retry once" behavior, not a silent downgrade to zero retries).
