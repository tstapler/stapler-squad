# ADR-003: Fix `ssq-notify` Type Mapping (Hook Events Are Sent With Wrong Enum Numbers)

**Status**: Proposed
**Date**: 2026-10-07
**Project**: notification-tray-and-hidden-session-gate

## Context

`scripts/ssq-notify` `type_to_enum()` (lines 82-96) emits legacy numbers that
do not match `NotificationType` in `proto/session/v1/types.proto:992-1016`
(`info`->1 but 1 is APPROVAL_NEEDED; `task_complete`->5 but 5 is
PROCESS_STARTED; `task_failed`->6 but 6 is PROCESS_FINISHED; `error`->3 but 3
is CONFIRMATION_NEEDED; `warning`->4 but 4 is TASK_COMPLETE; `question`->8
but 8 is WARNING). `NotificationService.SendNotification`
(`server/services/notification_service.go:157-166`) passes the integer through
unchanged. The stored "Claude Notification" row has type 1 / priority 2
(`research/pitfalls.md` section 0), consistent with this: Claude's idle
"waiting for your input" hook is stored as **APPROVAL_NEEDED**, which the
client never dedups or demotes (`useSessionNotifications.ts:120-122`) and the
push subscriber always pushes. Research (features.md section 2) believed the
hook sent INFO. VERIFIED by reading both files; not yet verified end to end
(Spike 1.3).

A type-keyed gate (ADR-002) depends on correct numbers: a hidden session's
`task_failed` hook currently arrives as PROCESS_FINISHED (routine, would be
suppressed) and its idle hook as APPROVAL_NEEDED (would be allowed).

## Decision

1. Rewrite `type_to_enum()` to the proto numbering: info=10,
   approval_needed=1, question=2, error=7, warning=8, task_complete=4,
   task_failed=9 (FAILURE), progress=5 (PROCESS_STARTED), reminder=10,
   system=10, custom=100, default=10.
2. Claude's `Notification` hook (`scripts/ssq-hook-handler:445`) stays
   `--type info`. After the fix it is INFO, so visible sessions' idle toast
   demotes to the tray via the existing `HISTORY_ONLY_TYPES`
   (`useSessionNotifications.ts:19-26`), and a hidden session's idle prompt is
   suppressed. Permission prompts are already announced by the approval
   handler, so no hook-side INPUT_REQUIRED is added.
3. A Go parity test parses the `case` table out of `scripts/ssq-notify` and
   compares it with `sessionv1.NotificationType_value`, so the two cannot
   drift again.
4. Ship the script fix as its own commit (`fix(scripts):`) at the start of
   Epic 2; the gate flag stays off until it is deployed to
   `~/.local/bin/ssq-notify` (`Makefile:267-270` installs it).

## Alternatives Considered

- Remap legacy numbers server-side: rejected. The server cannot tell a legacy
  caller from a correct one (`10` is unambiguous but `1..9` collide).
- Make the idle hook INPUT_REQUIRED: rejected; pins a toast and pushes on every
  idle prompt from visible sessions, the noise the toast work removes.

## Consequences

- **Visible behavior change for visible sessions**: idle/"Task Complete"/"Task
  Failed" hook events change type. Idle and complete become tray-only; failure
  becomes FAILURE (toast + pinned). Call this out in the PR description.
- Historical rows keep their wrong types; they age out under 7-day retention.
- Version skew: until `ssq-notify` is reinstalled, hook events keep the old
  numbers. The gate flag rollout checklist includes confirming the installed
  copy matches the repo.

## Addendum (plan repair, iteration 1)

- Version skew is detectable at runtime: the script sends metadata
  `ssq_notify_schema=2`; `SendNotification` counts requests from the hook path
  without it (`notification_rpc_unversioned_total`) and logs one WARN per hour.
  The gate-flag-on checklist requires this counter to be zero.
- The hook handler stamps `delivery_class=routine` on post-tool errors and
  subagent `task_failed` so the corrected `error->7` mapping does not make
  per-tool noise failure-class (ADR-002).

## Addendum 2 (plan repair, iteration 3): skew-safe rule

Detection is not prevention (adversarial N4): with the gate on and an old
script installed, a hidden session's `task_failed` arrives as 6
(PROCESS_FINISHED, routine) and is suppressed. So `SendNotification` stamps a
server-side `HintUntrustedType` on any request without `ssq_notify_schema`; for
a **hidden** session the policy then delivers it (fail open, counted by
`hidden_delivered{class=routine}` and the unversioned counter, WARN once per
hour). Visible sessions are unchanged. The Stage 2 checklist also compares the
installed `~/.local/bin/ssq-notify` hash with `scripts/ssq-notify`. Once the
unversioned counter is zero the rule never fires. Spike 1.3h confirms
`ssq-notify` is the only caller that omits the field
(`web-app/src/app/notifications/NotificationsPage.tsx` also references
`SendNotification`).
