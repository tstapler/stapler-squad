# Flip a notification-actions flag default: PR template and criteria checklist

> **Status: nothing in this document has been executed.** The checklists are blank templates. Copy the section for the flag you are flipping into the PR body and fill it in; do not tick a box without pasting the evidence.

A default flip is a decision the owner makes after a soak; no metric flips a flag automatically (`project_plans/notification-actions-and-agent-messaging/requirements.md`, "Decision rule for flag defaults"). This page turns that rule into a per-flag checklist. Companion pages: `docs/how-to/verify-notification-actions-rollout.md` (isolated-instance checklist), `docs/how-to/run-notification-usability-script.md` (T1 to T10 and the soak review), `docs/reference/notification-actions-observability.md` (metric queries; written separately).

The repository's existing PR template is `.github/pull_request_template.md` (Summary, Changes, Test plan, and a required semver label). This page does not add a file under `.github/`. Start from that template and paste the flag section below into "Test plan":

```bash
gh pr create --repo tstapler/stapler-squad --title "feat: default <flag> ON" --body-file <your-filled-body.md>
gh pr edit <number> --add-label "minor"   # or "patch"; the Semver Label Required check blocks merge without one
```

Ready for review, not draft: PRs in this repository default to ready (`CLAUDE.md`, "Pull Request Requirements").

## The one-line-PR rule

Each default flip is its own PR containing exactly one change: the `defaultValue` for one flag in `server/services/feature_flag_service.go` (the `knownFeatureFlags` entry), plus the matching "Default:" sentence in that entry's `description`. No other code, no second flag, no refactor (plan Story 5.3 AC 4: "Default flips ... are separate one-line PRs after the operator's soak"). Reasons: a flip must revert cleanly, and the flag's audit trail should show one decision per PR.

Persisted explicit-false values survive a default flip (ADR-008, Context, citing the delivery-gate ADR-004), so a flip changes only instances that never set the flag. An operator who set the flag OFF explicitly keeps it OFF; record in the PR whether any such override is known.

## Common gates (every flip)

- [ ] Soak length: the flag has been ON in dogfood for at least 14 days. Start date: ______ End date: ______
- [ ] The isolated-instance checklist (`docs/how-to/verify-notification-actions-rollout.md`) is complete for this flag and pasted or linked.
- [ ] Rollback verified: the flag was flipped OFF live and ON again without a restart, with read-back output recorded (verify doc, section 2).
- [ ] Revert plan stated in the PR: the live command to set the flag OFF is the `UpdateFeatureFlag` call in the verify doc, section 2, "Mechanism B".
- [ ] Owner decision recorded: Tyler approves this flip in the PR (the plan never flips a default automatically).

## Flag: `notification_inline_actions` (answer flag)

Criteria from `requirements.md`, "Decision rule for flag defaults":

- [ ] **Flip ON only when** all three paired quality metrics met their targets over the last 7 days of the soak, and no usability task T1 to T10 failed twice in a row.
  - [ ] Delivery-failure rate <= 2%. Measured: ______ (`(NOT_SENT + SEND_INDETERMINATE + STALE-after-click) / answers attempted` from `notification_inline_action_total{kind="ANSWER_QUESTION"}`). Evidence: ______
  - [ ] Wrong-answer proxy <= 5%. Measured: ______ (inline actions followed by Undo, or by opening the same session within 60 s: `inline_action_undo_total`, `reopen_after_action_total`). Evidence: ______
  - [ ] Unconfirmed rate <= 3%. Measured: ______ (answers still `sent` and unacknowledged at 60 s / answers sent; `guidance_answer_delivery_total`). Evidence: ______
  - [ ] No task of T1 to T10 failed twice in a row: ______ (recorded in the usability script document).
- [ ] **Keep OFF instead** if any of: a quality metric missed its target in the window; a hostile trial (T8, T9, T10) failed; fewer than 20 actions were recorded. Number of actions recorded in the soak: ______ (must be >= 20).
- [ ] The in-pane share is reported as the A1 evidence, not as a gate: overall ______, local listener alone ______ (`requirements.md`, adoption table and phone caveat).
- [ ] Any scope held back is listed (for example `channel:remote:tierB`, which also needs Spike S6): ______

Metric queries live in `docs/reference/notification-actions-observability.md`; paste the query output, not a paraphrase.

## Flag: `askuserquestion_structured` (answer flag)

Same three quality metrics and T1 to T10 rule as `notification_inline_actions`; the metrics gate these two flags only.

- [ ] Delivery-failure rate <= 2% over the last 7 days. Measured: ______ Evidence: ______
- [ ] Wrong-answer proxy <= 5% over the last 7 days. Measured: ______ Evidence: ______
- [ ] Unconfirmed rate <= 3% over the last 7 days. Measured: ______ Evidence: ______
- [ ] No task of T1 to T10 failed twice in a row: ______ T2 (answer a 3-option question) must pass before `stage:answer` is enabled (`design/ux.md` section 12).
- [ ] Fewer than 20 actions keeps the flag OFF. Actions recorded: ______
- [ ] Stages: record which stage this PR flips (`stage:render`, `stage:answer`, `stage:hook_reply`) and that each earlier stage is already ON (`stage:answer` needs `stage:render`; `stage:hook_reply` needs `stage:answer` and Spike S1). Stage: ______
- [ ] Layout canary clean: `question_dialog_match_total{result="mismatch"}` share did not trip the 50%-of-5 alert in the soak, and the Claude Code layout check (verify doc, section 5) is current for the Claude Code version in use. Version: ______

## Flag: `background_activity_feed` (ships ON; the question is whether it stays ON)

Own criteria (`requirements.md`). Keep ON when, over the 14 days:

- [ ] (i) No open of the tab rendered an unexplained empty state: `background_tab_empty_total{variant}` shows only the explained variants. Evidence: ______
- [ ] (ii) The Task 1.5b alert ("outcome evidence is empty") did not fire, or the owner accepted `stopped_unknown` rows as the permanent shape. Owner decision: ______
- [ ] (iii) The owner reported no phantom badges (the accepted per-browser limitation, `design/ux.md` 1.4). Owner statement: ______
- Open-on-row is reported, never a gate: ______
- A failure of (i) flips the flag OFF (legacy selector) and files a defect: Defect link: ______

This flag already defaults ON (`backgroundActivityFeedDefault` in `server/services/feature_flags_notification_actions.go`). A PR for it is a flip to OFF, or confirms the default; an OFF to ON flip after an explicit OFF is a loosening change and is audit-fsynced.

## Flag: `agent_wake_triggers` (per scope)

Own criteria (`requirements.md`). Flip a scope ON when, over its 14-day soak, in the rollout order `trigger:at`/`cron`, `guidance_answered`, event and state types, `tool:request_help`. Scope in this PR: ______

- [ ] (i) Zero wakes lost after a restart: `wake_trigger_total{state="missed"}` after any restart is 0, or every miss was an operator-visible expiry. Evidence: ______
- [ ] (ii) `wake_duplicate_delivery_total` is 0. Evidence: ______
- [ ] (iii) `wake_delivered_per_owner_hour` never exceeded the per-owner limit (provisionally 6), and no limit-hit digest came from real use. Evidence: ______
- [ ] (iv) For the scope that retires polling: `agent_schedule_wakeup_calls_total{role}` per backlog session is at least 50% below the baseline captured before the flag was on. Baseline: ______ Soak value: ______ Reduction: ______ (provisional; the owner sets the number)
- [ ] **Baseline row present.** `docs/reference/notification-actions-observability.md` contains the Task 5.1f baseline row: median and p90 of `agent_schedule_wakeup_calls_total{role}` per backlog session, with session count and a capture window of at least 7 days, captured with `agent_wake_triggers` OFF. **If this row is missing, the PR fails review for an `agent_wake_triggers` flip.** Row found at: ______ Session count: ______ Window (days): ______
  - If Task 0.7e found the injected matcher does not cover `ScheduleWakeup` and the fallback instrument also fails, the metric is reported as "no instrument" and criterion (iv) is dropped by the owner explicitly, not silently. Owner decision, if applicable: ______
- [ ] Fewer than 10 wake deliveries in the soak keeps the scope OFF by default. Deliveries: ______ The owner may still flip it by an explicit decision recorded here (no metric flips it): ______
- [ ] The "fewer than 20 actions" floor does not apply to wakes.
- [ ] `wake_fires_per_session` reviewed (histogram on session end): ______

## A1 fallback is owner-triggered only

If the in-pane share stays under the provisional targets after the 14 days, the plan reports it and waits. Cutting scope (Epic 3 to read-only, Epic 4 operator surface to visibility) happens only when the owner says so; record any such statement here: ______

## Reviewer checklist

- [ ] The diff is one flag's default and its description sentence, nothing else.
- [ ] Every checked box above has pasted evidence, not a summary.
- [ ] For `agent_wake_triggers`: the baseline row exists in `docs/reference/notification-actions-observability.md`.
- [ ] Semver label present.
