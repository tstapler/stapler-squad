# Run the notification usability script (T1 to T10) and the 14-day soak review

> **Status: nothing in this document has been executed.** All result slots are blank. The operator runs the script on the isolated manual instance and records each outcome here, then pastes the filled tables into the flag-flip PR.

This is the structured dogfood protocol from `project_plans/notification-actions-and-agent-messaging/design/ux.md` section 12 (plan Task 5.3c). It runs per flag, before each default flip, on desktop and on the phone. The operator is a single person, so this is a scripted self-test with instrumentation, not a recruited study.

Prerequisites: an instance started as in `docs/how-to/verify-notification-actions-rollout.md` section 1, with the flag under test ON. Where the flip rule is applied: `docs/how-to/flag-default-flip-pr-template.md`.

## How to record a trial

For each task and device, write `pass` or `fail`, then the date. A fail needs a filed defect link. "Failed twice in a row" means two consecutive runs of the same task on the same device both failed, counting runs across the soak, so keep every run, not only the last.

Run every task once on desktop and once on the phone (remote listener, `https://<host>:62872`). Tasks T8 to T10 are phone-only in practice (touch gestures); run them on a desktop with touch emulation only if no phone is available, and say so.

## Tasks

Source of each task, criterion and failure meaning: `design/ux.md` section 12 (`project_plans/notification-actions-and-agent-messaging/design/ux.md:685`).

| # | Task | Success criterion | If it fails | Desktop (pass/fail, date, defect) | Phone (pass/fail, date, defect) |
|---|---|---|---|---|---|
| T1 | Open the Background tab cold and say, without clicking Open, whether anything failed | Correct in under 10 s; "no failures reported" is not read as "all succeeded" | Rewrite the roll-up copy; flag stays ON (read-only change) | | |
| T2 | Answer a 3-option question from a notification | Correct option sent; no navigation; under 5 s from pane open | Change option-button layout and spacing before `askuserquestion_structured` `stage:answer` is enabled | | |
| T3 | While a card is open, have a second question arrive; answer the first | Answer lands on the intended card: 0 wrong-card answers in 10 trials | Tighten the freeze and grace rules; block the flip | | |
| T4 | Approve a command longer than 10 lines | Operator reads the whole command before tapping (observed scroll or Copy) | Redesign the command region; block the flip | | |
| T5 | Cause an Unconfirmed answer (drop the dialog fixture) and recover | Operator opens the session or marks handled within 60 s without prompting | Rewrite the Unconfirmed copy | | |
| T6 | Handle a stopped-session answer and a saved-for-later answer | Operator states the answer was saved and the agent has not received it yet | Rewrite the Saved copy | | |
| T7 | From the phone only, resolve one Tier A and see one Tier B marked "Answer from desktop" | Operator does not try the Tier B action twice | Rewrite the Tier B copy | n/a | |
| T8 | Hostile: scroll-fling a list of 30 items and tap the glass to stop it, 10 trials | 0 activations of any control | Widen the 120 ms fling guard; block the flip | | |
| T9 | Hostile: arrival under the thumb. With a thumb about to tap a card below, make a new item and a failure arrive at the top, 10 trials | 0 wrong-card activations and 0 row movement while engaged | Tighten the freeze, reserved slot or settle window; block the flip | | |
| T10 | Hostile: app switch mid-hold. Tap Approve, switch to another app within 3 s, return, 10 trials | No approval is sent; the card is restored with the cancellation line and the bell badge still counts it | Fix the hold-cancel logic; block the flip | | |

### Per-task setup notes

These describe how to produce each condition; the pass criteria are in the table.

- T1: use a manual instance with one failed hidden session and one that stopped without a recorded verdict. Open the tab in a fresh browser tab.
- T2, T3: ask a session to call `AskUserQuestion` with 3 options (T2) and trigger a second question while the first card is open (T3). Requires `askuserquestion_structured` `stage:render` and `stage:answer` ON and `notification_inline_actions` ON.
- T4: use a session that requests approval for a Bash command of more than 10 lines.
- T5: to produce Unconfirmed, make the pane stop showing the dialog after the answer is sent, so the acknowledgement signal stays indeterminate (the Unconfirmed state is row 3 of the delivery-state table in `design/ux.md` section 3.1). Record which method you used: ______
- T6: stop (pause) a session that has a pending question, then answer it from the pane; separately use an agent-created question for the saved-for-later case.
- T7: open the remote listener from the phone; a `live_keystroke` question on the remote listener reads "Answer from desktop" in v1 (`design/ux.md` 2.4).
- T8: 30 notification items are needed; create them with the manual instance, then fling-scroll and tap to stop.
- T9: have a second session produce a new item and a failure while the thumb rests above a card lower in the list.
- T10: needs a commit-delay action (Approve). Switch apps inside the 3-second window.

### Trial log (hostile trials, 10 trials each)

Record each of the 10 trials for T3 and T8 to T10 so "failed twice in a row" can be read off.

| Task | Device | Trial 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | Activations / wrong-card / sent (count) |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| T3 | | | | | | | | | | | | |
| T8 | | | | | | | | | | | | |
| T9 | | | | | | | | | | | | |
| T10 | | | | | | | | | | | | |

## Soak review (14 days, per flag)

The review decides whether the flip PR can be opened. The thresholds are in `requirements.md`, "Decision rule for flag defaults"; the PR checklist is `docs/how-to/flag-default-flip-pr-template.md`.

1. Confirm the flag has been ON for at least 14 days on the instance used for dogfood. Record start and end date: ______
2. Read the three paired quality metrics for the last 7 days of the soak. The queries are defined in `docs/reference/notification-actions-observability.md`; run them as written there and paste the output. Do not re-derive them here.
3. Compare each result to its target.
4. Check the usability trial log above for any task that failed twice in a row, or any hostile trial failure.
5. Decide per the flag's rule in the PR template. Record the decision and the reason.

### Metrics to read (names only; queries are in the observability reference)

| Metric | Used for | Which flags |
|---|---|---|
| `notification_inline_action_total` | Delivery-failure rate (`kind="ANSWER_QUESTION"`), action count (the 20-action floor), in-pane share | `notification_inline_actions`, `askuserquestion_structured` |
| `notification_inline_action_duration_seconds` | Time to resolve in the pane (supports T2's 5 s criterion) | the two answer flags |
| `reopen_after_action_total` | Wrong-answer proxy: the session was opened within 60 s of an action | the two answer flags |
| `inline_action_undo_total` | Wrong-answer proxy: Undo after a commit-delay action | the two answer flags |
| `guidance_answer_delivery_total` | Unconfirmed rate and delivery outcomes | the two answer flags |
| `wake_trigger_total` | `state="missed"` after a restart (criterion i) and wake delivery count (the 10-delivery floor) | `agent_wake_triggers` |
| `wake_fires_per_session` | Wake deliveries per session (histogram on session end) | `agent_wake_triggers` |
| `agent_schedule_wakeup_calls_total` | `ScheduleWakeup` calls per backlog session against the Task 5.1f baseline (criterion iv) | `agent_wake_triggers` |

Other metrics named in the flip criteria, for completeness: `wake_duplicate_delivery_total`, `wake_delivered_per_owner_hour`, `background_tab_empty_total{variant}`, `question_dialog_match_total{result,claude_version}`. All of these exist in the observability reference except `wake_delivered_per_owner_hour` and `background_tab_empty_total`, which are not implemented as series (see its "not implemented" table). Confirm every metric name exists in the observability reference before using it; if a name differs there, the reference wins and this table needs a one-line fix.

### Soak record

| Flag | Soak start | Soak end | Days ON | Actions recorded (floor 20; wakes floor 10) | Delivery-failure (<= 2%) | Wrong-answer proxy (<= 5%) | Unconfirmed (<= 3%) | Any T1-T10 failed twice in a row? | Hostile trial failed? | Decision (flip / keep OFF) and reason |
|---|---|---|---|---|---|---|---|---|---|---|
| `notification_inline_actions` | | | | | | | | | | |
| `askuserquestion_structured` | | | | | | | | | | |
| `background_activity_feed` | n/a | | | n/a | n/a (own criteria) | n/a | n/a | | | |
| `agent_wake_triggers` (scope: ) | | | | | n/a (own criteria) | n/a | n/a | | | |

For `background_activity_feed` and `agent_wake_triggers`, record the own-criteria values in the corresponding PR template section instead of the three quality metrics.

After the A1 review (in-pane share against the provisional targets), the owner decides; no metric triggers scope cuts.
