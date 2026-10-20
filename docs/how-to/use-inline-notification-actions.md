# Use inline notification actions

Approve, deny, answer and decide from a notification row, and see what agents
scheduled or asked for, without opening the session. This page is the operator
view. Metrics are in `docs/reference/notification-actions-observability.md`; the
rollout checklist is `docs/how-to/verify-notification-actions-rollout.md`.

## If you are away from the machine

Alerting an operator who is away depends on the Slack and web-push channels. Those
channels are outside this project, and this project did not verify that they deliver
(UX-AC78, `project_plans/notification-actions-and-agent-messaging/design/ux.md`). The
notification pane and the Background tab only help while you have the web UI open.
If you rely on being paged, test your Slack and web-push setup yourself.

## Flags

Four flags control the feature. A flip applies at once, with no restart
(`server/services/feature_flag_service.go`, the `knownFeatureFlags` entries).

| Flag | Default | What it turns on |
|---|---|---|
| `background_activity_feed` | on | The Background tab shows what each background session did, instead of the history-only selector. |
| `notification_inline_actions` | off | The action buttons on notification rows. Scopes: `channel:remote`, `channel:remote:tierB`, `action:<kind>`. |
| `askuserquestion_structured` | off | Question cards. Stages: `stage:render`, `stage:answer`, `stage:hook_reply`; each also needs the stage before it. |
| `agent_wake_triggers` | off | The wake dispatcher, the Scheduled prompts section, Remind me and peer help. Scopes: `trigger:<type>`, `tool:<name>`. |

With `notification_inline_actions` off, every row reduces to Open session and the
older approve and deny path still works. An action refused for a flag returns the copy
"This action is turned off. Open session." (`server/services/feature_flags_notification_actions.go`,
`flagOffMessage`).

### Flip a flag

Two mechanisms exist:

1. The Feature Flags page at `/settings/features`
   (`web-app/src/app/settings/features/page.tsx`). Each flag has a global toggle; flags
   with scopes also have per-scope Inherit, On and Off controls.
2. The `UpdateFeatureFlag` Connect RPC (`proto/session/v1/session.proto`). Request
   bodies, the read-back command and rollback are in
   `docs/how-to/verify-notification-actions-rollout.md`, section 2. Read the state back
   after every write.

Turning a flag on is audited and is refused while the audit sink is down
(`notificationActionsFlagAuditPolicy`). Turning it off is not refused.

Two stages stay closed whatever you set, because the code gates them on a spike result
that is not recorded yet (`server/services/feature_flags_notification_actions.go`):

- `stage:hook_reply` needs `spikeS1HookReplyPassed`, currently `false`.
- `channel:remote:tierB` needs `spikeS6TierBPassed`, currently `false`.

## Tier A and Tier B actions

Every action has a tier (`actionPolicies` in `server/services/notification_actions.go`;
ADR-004).

| Tier | What it does | Actions | On the remote listener (`:8444`) |
|---|---|---|---|
| A | Writes data only. Nothing is typed into a terminal. | `open`; `approve`; `deny`; `backlog_decision`; `answer_guidance`; `cancel_wake` (Cancel prompt); `cancel_peer_help` (Cancel request); Remind me; `answer_question` on a stopped or archived session (the answer is stored) | Available when the `channel:remote` scope is on. |
| B | Types bytes into a live terminal. | `answer_question` on a running session | Not available. Shows "Answer from desktop" (`desktop_only`). |

Notes:

- Whether an `answer_question` is A or B depends on the session when you answer
  (`answerTier`, `server/services/notification_actions.go`). A running session means
  keystrokes (B). A stopped or archived session means a stored answer (A).
- Resume scheduled prompts writes a flag, so it works on the local listener only. The
  remote listener shows "Resume from desktop".
- The local listener `:8543` has no authentication. Tier B there rests on the caller being
  on the machine, not on who the caller is (ADR-004, Consequences).

## Vocabulary

The UI avoids internal names. These are the terms you see and what they mean
(`project_plans/notification-actions-and-agent-messaging/design/ux.md`, section 0).

| You see | Internal term | Meaning |
|---|---|---|
| Answer | `answer_question`, `answer_guidance` | Reply to a question an agent asked. |
| Approve, Deny | `approve`, `deny` | Reply to a permission request. |
| Decide | `backlog_decision` | Choose on a backlog item: Mark done (`accept`), Send back (`rework`), Merge (`merge`). |
| Scheduled prompts | wake, `WakeTrigger` | A time or event when an agent asked to be prompted again. |
| Reminder (you) | `notify_operator` wake | Your own reminder. It never types into a session. |
| Cancel prompt, Cancel request, Cancel reminder | `cancel_wake`, `cancel_peer_help` | End something an agent started or you scheduled. A bare "Cancel" is never used. |
| Back, Clear, Keep | none | Back out of a sub-step, clear a multi-select, decline a confirm. |
| Unconfirmed | Unconfirmed sub-state | A sent answer the agent has not confirmed. It is not a failure. |
| Needs you (N) | none | A Background row that has a pending decision. |

The word "kind" in metric labels always means the action kind, never a session role
or a notification type (plan, "The word kind").

## Approve, deny, answer, decide

Open the notification pane. A row with actions shows its buttons.

- **Approve.** Tap Approve. The button counts down for 3 s with Undo. Press Undo to stop
  it; nothing is sent. Leaving the panel, switching tabs, going offline or navigating
  cancels the hold and sends nothing.
- **Send back and Mark done.** Backlog decisions with the same 3 s Undo.
- **Deny, Merge, answers.** No Undo. Deny and Merge ask first, with a Back step.
- **Answer.** Pick an option on the question card. A multi-question card sends all of its
  answers as one action.

The 3 s delay is `commitDelayMs` in `web-app/src/lib/hooks/interactionTiming.ts`; the three
held actions are listed in `web-app/src/components/ui/notification-actions/actionSpec.ts`
(`holdKeyOf`).

### What the answer card tells you

For a running session the card shows the delivery state
(`web-app/src/lib/copy/notificationCopy.ts`):

| Card text | Meaning |
|---|---|
| Sending... | The write is in progress. |
| Sent - waiting for the agent | Keys were written. |
| Sent, but the agent hasn't confirmed yet. Open session. | Unconfirmed. The row stays in Needs you until the agent confirms, you mark it handled, or the session ends. |
| Not sent. Nothing was typed into the session. Retry | The write failed before the first key. Retry is safe. |
| Not sent: someone is typing in that session. Retry when they are done. | You or another person typed in that pane within the last 5 s (`humanActiveWindow`, `server/services/human_activity.go`). |
| Sent, but we couldn't confirm it arrived. Check the session before retrying. | A key may have landed. No Retry. |
| Sent, but the question is still showing. Check the session before retrying. | The dialog was still on screen after 60 s. Retry sits behind "Retry anyway?". |
| Part of your answer was sent, then the prompt changed. Open session to finish. | A multi-key answer stopped part way. No Retry. |
| Already handled in the session. No action needed. | You answered in the terminal first. |
| Your answer is saved. The agent can read it once you resume the session. | Stopped session; the answer is stored. |

Why the routes differ: `docs/explanation/askuserquestion-answer-routing.md`.

### Desktop only

On a phone or any other browser on the remote listener, a question for a running
session shows "Answer from desktop" with Open session. Approve, deny, backlog decisions
and answers to stopped sessions still work there when `channel:remote` is on.

## Scheduled prompts, Remind me and peer help

These need `agent_wake_triggers`. Reference for the tools and failures:
`docs/reference/wake-triggers-and-agent-messaging.md`.

- **Scheduled prompts.** The Background tab lists pending triggers that agents created,
  with Cancel prompt on each row. If the flag is off the section shows "Scheduled prompts
  paused. Agents will not be prompted automatically." Rows are kept while paused.
  Resume scheduled prompts turns the flag back on, from the local listener only.
- **Remind me.** On an informational Background row, Remind me schedules a reminder
  about that session. It appears in the same section as "Reminder (you)". When it is due
  you get a notification; nothing is typed into any session
  (`web-app/src/components/ui/RemindMeMenu.tsx`).
- **Peer help.** When an agent asks another session for help, the request shows as a
  row. It is informational. After 10 minutes without an answer, or at once when the
  other session is stopped, archived or deleted, you get one needs-human notification,
  and the row offers "Answer for {peer}" and Cancel request. The asking agent sees your
  answer as coming from the operator on behalf of the peer.
- **Missed prompts.** After a restart, prompts that expired or could not run are
  reported in one failure notification that names the trigger ids. See "Alerts" in
  `docs/reference/notification-actions-observability.md`.
