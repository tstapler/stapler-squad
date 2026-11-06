# Verify the notification-actions rollout on an isolated instance

> **Status: nothing in this document has been executed.** Every output slot, result column and sign-off below is blank on purpose. The operator fills them in while running the checklist; paste the filled checklist into the PR description (plan Story 5.3, `project_plans/notification-actions-and-agent-messaging/implementation/plan.md`).

This checklist proves each of the four rollout flags against a real build before any default flips. It runs on a second, fully separate instance so it never touches the live service on `:8543`. The flip decision itself follows `docs/how-to/flag-default-flip-pr-template.md`; the scripted usability tasks are in `docs/how-to/run-notification-usability-script.md`.

Related: ADR-008 (`project_plans/notification-actions-and-agent-messaging/decisions/ADR-008-rollout-flag-semantics.md`), `docs/reference/wake-triggers-and-agent-messaging.md`, `docs/reference/notification-actions-observability.md` (metric definitions; written separately).

## Rules

- Never run `make install-service` to try the change. It restarts the live service and kills its tmux server and every session in it (`CLAUDE.md`, "Manual/interactive testing without touching the live deployed instance").
- Build to `~/.stapler-squad/manual-builds/manual-<N>/stapler-squad`, never to `./stapler-squad`.
- Use ports from the manual dev port block: instance 1 is `62871` (`PORT`) and `62872` (`--remote-port`); instance 2 is `62873` and `62874` (`CLAUDE.md`, "Manual dev port block").
- Set `STAPLER_SQUAD_TMUX_SOCKET` to a unique name. `STAPLER_SQUAD_INSTANCE` does not isolate tmux; without the socket variable the instance shares the live service's tmux server.
- Flags are live-settable through the feature-flag RPC or panel only. There is no environment variable for them (ADR-008, Context).

## 1. Start the instance

Run these from your worktree. Record each output in the slot below it.

1. Build into the numbered directory.

   ```bash
   mkdir -p ~/.stapler-squad/manual-builds/manual-1
   go build -o ~/.stapler-squad/manual-builds/manual-1/stapler-squad .
   ```

   Output:

   ```text
   (paste here)
   ```

2. Check that the ports are free. The command must print nothing. If it prints a line, a stale manual instance holds the port: stop it or use the next port pair (`62873`/`62874`) and `manual-2`.

   ```bash
   ss -ltnp | grep -E '62871|62872'
   ```

   Output (must be empty):

   ```text
   (paste here)
   ```

3. Start the instance with its own state directory and private tmux socket. `--remote-port` is required because its default, `8444`, collides with the live instance (`main.go:887-891`).

   ```bash
   PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1 \
     ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server --remote-access --remote-port 62872 &
   ```

   Output:

   ```text
   (paste here)
   ```

4. Confirm isolation. The startup log must contain `Using private tmux socket`, and the log file must be under `~/.stapler-squad/instances/claude-manual-test/logs/staplersquad.log` (`CLAUDE.md`, "Application Data").

   ```bash
   grep 'Using private tmux socket' ~/.stapler-squad/instances/claude-manual-test/logs/staplersquad.log
   ```

   Output:

   ```text
   (paste here)
   ```

5. Open `http://localhost:62871` in a browser. Create at least one visible session and one hidden (background) session so every row type below has something to show.

To stop the instance at the end:

```bash
kill %1
tmux -L ssq-manual-1 kill-server   # touches only the private socket
```

## 2. Flip a flag live and roll it back

The four flags are registered in `server/services/feature_flag_service.go` and wired in `server/services/feature_flags_notification_actions.go`. A flip applies at once with no restart (each flag description ends "Applies at once").

| Flag | Defaults to | Scopes (closed set) |
|---|---|---|
| `background_activity_feed` | ON | none (global only) |
| `notification_inline_actions` | OFF | `channel:remote`, `channel:remote:tierB`, `action:<kind>` for `APPROVE`, `DENY`, `ANSWER_QUESTION`, `BACKLOG_DECISION`, `ANSWER_GUIDANCE`, `CANCEL_WAKE`, `CANCEL_PEER_HELP` |
| `askuserquestion_structured` | OFF | `stage:render`, `stage:answer`, `stage:hook_reply` |
| `agent_wake_triggers` | OFF | `trigger:<at, cron, backlog_event, pr_event, ci_event, session_state, guidance_answered>`, `tool:<schedule_wake, list_wakes, cancel_wake, request_help>` |

Source for the table: `server/services/feature_flags_notification_actions.go` (scope constants and `knownFeatureFlags` entries in `server/services/feature_flag_service.go:312-330`). `channel:remote:tierB` and `stage:hook_reply` stay closed regardless of the flag until the Spike S6 and S1 constants in that file (`spikeS6TierBPassed`, `spikeS1HookReplyPassed`) are set to true in code; record their current value in step 2.1.

Precedence: an explicit scoped value wins in either direction, then the explicit global value, then the default (ADR-008, Decision). Dependencies: `stage:answer` needs `stage:render`; `stage:hook_reply` needs `stage:answer` and Spike S1; `channel:remote:tierB` needs `channel:remote` and Spike S6; for `agent_wake_triggers` a `trigger:<type>` or `tool:<name>` scope is effective only while the dispatcher runs.

### Mechanism A: the Feature Flags page

Open `http://localhost:62871/settings/features` (`web-app/src/app/settings/features/page.tsx`). Each flag shows a global toggle; flags with scopes show per-scope Inherit/On/Off controls. Turning a flag ON is a loosening change: it is audit-fsynced and refused while the audit sink is down (`notificationActionsFlagAuditPolicy` in `server/services/feature_flags_notification_actions.go`).

### Mechanism B: the Connect RPC

Handlers are mounted under `/api` (`docs/reference/backlog-dependencies.md`). The service is `session.v1.SessionService`; request fields are in `proto/session/v1/session.proto` (`UpdateFeatureFlagRequest`). A set needs an explicit scope: `global` for the global value (`server/services/feature_flag_mutation.go:112-114`).

```bash
BASE=http://127.0.0.1:62871/api/session.v1.SessionService

# Read current state (global value and persisted scope overrides)
curl -s -X POST $BASE/GetFeatureFlags -H 'Content-Type: application/json' -d '{}'

# Set the global value ON (use FLAG_MUTATION_SET_DISABLED for OFF)
curl -s -X POST $BASE/UpdateFeatureFlag -H 'Content-Type: application/json' \
  -d '{"name":"notification_inline_actions","mutation":"FLAG_MUTATION_SET_ENABLED","scope":"global"}'

# Set one scope ON
curl -s -X POST $BASE/UpdateFeatureFlag -H 'Content-Type: application/json' \
  -d '{"name":"askuserquestion_structured","mutation":"FLAG_MUTATION_SET_ENABLED","scope":"stage:render"}'

# Remove a scope override (inherit)
curl -s -X POST $BASE/UpdateFeatureFlag -H 'Content-Type: application/json' \
  -d '{"name":"askuserquestion_structured","mutation":"FLAG_MUTATION_CLEAR_SCOPE","scope":"stage:render"}'

# Remove the explicit global value (registry default applies)
curl -s -X POST $BASE/UpdateFeatureFlag -H 'Content-Type: application/json' \
  -d '{"name":"notification_inline_actions","mutation":"FLAG_MUTATION_RESET_GLOBAL"}'
```

Read the state back after every write. An accepted request that changed nothing looks the same as success, so the check below is part of the step, not optional.

### Per-flag flip and rollback record

For each flag, run: read state, flip ON, read state back, exercise the behavior in the "Check while ON" column, flip OFF, read state back, confirm the "Check while OFF" column. Paste the `GetFeatureFlags` output for the flag after each flip.

| # | Flag (scope) | Check while ON | Check while OFF | Read-back after ON | Read-back after OFF | Result (pass / defect link) |
|---|---|---|---|---|---|---|
| 2.1 | `background_activity_feed` (global) | Background tab shows per-session rows with outcome chips | Legacy history-only selector returns; no data lost (ADR-008 table) | `(paste)` | `(paste)` | |
| 2.2 | `notification_inline_actions` (global) | Approve, Deny, Answer and Decide controls appear on notification rows | Rows reduce to Open session; the legacy Approve/Deny path still works; invoking an action returns `FLAG_OFF` with copy "This action is turned off. Open session." | `(paste)` | `(paste)` | |
| 2.3 | `notification_inline_actions` (`action:APPROVE` OFF under global ON) | Kill switch: only Approve reduces to Open session; other actions still work | Clear the scope and confirm Approve returns | `(paste)` | `(paste)` | |
| 2.4 | `askuserquestion_structured` (`stage:render`) | A structured question card renders with its options | New card creation refused; existing rows stay readable | `(paste)` | `(paste)` | |
| 2.5 | `askuserquestion_structured` (`stage:answer`, after `stage:render`) | Answer control on a card sends an answer (section 4 below) | Answer control gone; the server refuses with `FLAG_OFF` | `(paste)` | `(paste)` | |
| 2.6 | `askuserquestion_structured` (`stage:hook_reply`) | Only if `spikeS1HookReplyPassed` is true. Record its value: `(paste)` | Hook-reply route stops | `(paste)` | `(paste)` | |
| 2.7 | `agent_wake_triggers` (`trigger:at`, then `tool:schedule_wake`) | `schedule_wake` with `at` succeeds and shows in Scheduled prompts | `schedule_wake` returns the typed `flag_off` error; rows kept (`docs/reference/wake-triggers-and-agent-messaging.md`, "Flag flips apply at runtime") | `(paste)` | `(paste)` | |
| 2.8 | `agent_wake_triggers` (global) | Dispatcher runs for all types | Dispatcher stops; pending rows past `expires_at` become `missed` at the next reconcile, others are held | `(paste)` | `(paste)` | |

Flip order for staged flags follows `project_plans/notification-actions-and-agent-messaging/requirements.md`: wake scopes `trigger:at`/`cron`, then `guidance_answered`, then event and state types, then `tool:request_help`.

Also verify the flag survives a restart of the manual instance: stop it, start it with the step 1.3 command, read the flag state back. Output:

```text
(paste here)
```

## 3. Notification surface checks

Run each against the manual build with the relevant flag ON. Record the command or the observed result.

| # | Check | Flag state | Evidence (paste output / screenshot path) | Result |
|---|---|---|---|---|
| 3.1 | Background tab shows rows for hidden sessions, each with an outcome chip, and never claims "Finished" for a stopped session with no verdict (`design/ux.md` 1.3) | `background_activity_feed` ON | | |
| 3.2 | Approve inline: start a session that requests a permission, tap Approve in the pane; the 3-second Undo window appears and the session proceeds | `notification_inline_actions` ON | | |
| 3.3 | Question inline: a pending AskUserQuestion is answered from the pane with zero session navigation | `notification_inline_actions` ON and `askuserquestion_structured` `stage:render`+`stage:answer` ON | | |
| 3.4 | Backlog decision inline: decide a backlog item from the pane | `notification_inline_actions` ON | | |
| 3.5 | Remote listener on a phone: Tier A resolves; a `live_keystroke` question shows "Answer from desktop" (`design/ux.md` 2.4) | `channel:remote` ON | | |

## 4. Structured question answered on a live session

Goal: an `AskUserQuestion` shown in the pane reaches the originating live session and the session continues.

1. In a manual-instance session running Claude Code, ask the agent to call `AskUserQuestion` with a single-select question and a multi-select question.
2. Answer both from the pane.
3. Record the guidance row's `created_at` and `answered_at` and the delivery state shown.

Evidence:

```text
(paste here)
```

Result: ______

### Worked example: live-Claude tabbed-dialog byte check (measured 2026-10-19)

This is a measured result from the Phase 5 work, kept as the model for how to run the check. It is a record of what was done, not a result for your run.

What it proved: on 2026-10-19, with Claude Code 2.1.303 in a tmux session named `live` on the private socket `ssq-manual-3`, a two-question tabbed dialog (a Cache single-select and a Letters multi-select) was answered Memcached / B, C through Review > Submit answers. It passed.

The bytes were sent by hand with tmux, not by ssq:

```bash
tmux -L ssq-manual-3 send-keys -t live -H 0d          # Enter
tmux -L ssq-manual-3 send-keys -t live -H 1b 5b 42    # Down (ESC [ B)
```

Scope of the claim: the check exercised the same bytes ssq writes (Enter `0d`, Down `1b 5b 42`), and showed that Claude Code 2.1.303 accepts them in this dialog. It did not exercise the ssq writer (`paneWriter.SendInput` path) itself. Treat it as evidence for the key encoding only; item 3.3 and the table in section 5 cover the ssq path.

To repeat the byte check after a Claude Code update:

1. Start a manual-instance session on a private socket and name the tmux session `live` (or substitute your own name in `-t`).
2. Prompt the agent to call `AskUserQuestion` with a single-select question and a multi-select question.
3. Drive the dialog with `tmux -L <socket> send-keys -t live -H <hex bytes>`: `0d` for Enter, `1b 5b 42` for Down. Run `tmux -L <socket> capture-pane -p -t live` between keys.
4. Pass criterion: the answers you intended are what the agent reports it received, via Review > Submit answers.

| Date | Claude Code version (`claude --version`) | Socket | Dialog | Intended answers | Agent-reported answers | Result |
|---|---|---|---|---|---|---|
| 2026-10-19 (measured, reference only) | 2.1.303 | `ssq-manual-3` | Cache single-select + Letters multi-select | Memcached / B, C | Memcached / B, C | PASS |
| | | | | | | |

## 5. Claude Code layout canary after a Claude Code update (Task 3.3h)

The dialog matcher is pinned to fixtures captured from Claude Code `v2.1.296` (`session/testdata/reply_dialogs/claude_auq_*.txt`; version stated in plan Story 3.3). After any Claude Code update, re-check. A mismatch makes every `DialogMatch` fail closed, and the canary degrades hook-mirrored question rows to `read_only` (`server/services/guidance_dialog_canary.go`: more than half of at least 5 attempts in 1 hour mismatch).

1. Record the new version.

   ```bash
   claude --version
   ```

   Output: `(paste)`

2. Re-run the fixture tests against current captures. First capture a fresh dialog from the new version into a scratch file with `tmux -L ssq-manual-1 capture-pane -p -t <session>`, then compare it by eye with `session/testdata/reply_dialogs/claude_auq_twoq_step1_single_select.txt`. Then run the existing matcher tests:

   ```bash
   go test ./session -run 'TestClaudeProfile' -count=1
   ```

   Output:

   ```text
   (paste here)
   ```

3. Answer one question from the pane on the manual instance. Check whether the canary fired by reading the log and the metric `question_dialog_match_total{result,claude_version}` (definition in `docs/reference/notification-actions-observability.md`):

   ```bash
   grep -i 'dialog' ~/.stapler-squad/instances/claude-manual-test/logs/staplersquad.log | tail -20
   ```

   Output:

   ```text
   (paste here)
   ```

4. If the layout changed: do not edit the matcher in this checklist. File a defect, leave the canary to degrade rows to `read_only`, and re-run the section 4 byte check to see whether the key encoding still works.

| Date | Claude Code version | Fixture comparison (same / differs) | Matcher tests | Canary fired? | Result / defect link |
|---|---|---|---|---|---|
| | | | | | |

UNVERIFIED in this worktree: the source of the Claude Code version for the `claude_version` label (Spike S7 item 9, plan Task 0.7g). If it is `unknown`, rely on the mismatch share.

## 6. Wake survives a restart

Requires `agent_wake_triggers` with `trigger:at` and `tool:schedule_wake` ON (step 2.7). Behavior reference: ADR-006 and `docs/reference/wake-triggers-and-agent-messaging.md`.

1. In a manual-instance work session, have the agent call `schedule_wake` with an `at` time 10 minutes ahead. Confirm the trigger appears in Background > Scheduled prompts as pending. Record the trigger and time:

   ```text
   (paste here)
   ```

2. Stop the instance (`kill %1`) and start it again with the step 1.3 command, so the restart lands before the `at` time. Keep `--tmux-keep-server` so the session survives.
3. After restart, confirm the trigger is still pending (not `missed`) and read the metric `wake_trigger_total{state="missed"}` (must not have increased; criterion (i) in `requirements.md`, "Decision rule for flag defaults").

   ```text
   (paste here)
   ```

4. Wait for the `at` time. Confirm exactly one delivery to the session (no duplicate; `wake_duplicate_delivery_total` stays 0).

   ```text
   (paste here)
   ```

5. Overdue case: schedule another `at` 2 minutes ahead, stop the instance for longer than that, start it. Per ADR-008 re-enable semantics and ADR-006 rule 4, a trigger within `expires_at` fires once and one past `expires_at` becomes `missed` with a notification.

   ```text
   (paste here)
   ```

Result: ______

## 7. UX sweep that automation cannot prove

Each row ends with a recorded pass or a filed defect (plan Story 5.3 AC 2). Do not tick a row without evidence.

### 7.1 Exit controls on every error and terminal state (UX-AC55)

Automated half: the parametrized `ux-error-exits.spec.ts` under `tests/e2e/`. Manual half: for each section, trigger every scenario in its table (the "Operator sees" and "Exit path" columns in `project_plans/notification-actions-and-agent-messaging/design/ux.md`) and confirm at least one exit control (Retry, Open, Dismiss, Back, Keep, Cancel prompt, Cancel request, or a link) is visible and works.

NOTE: `tests/e2e/ux-error-exits.spec.ts` does not exist in the checkout this document was written against (`ls tests/e2e | grep ux-` returned nothing). Run `ls tests/e2e/ux-error-exits.spec.ts` first; if it is still missing, record that as a defect against plan Story 5.4 before using the automated half.

| Section | Surface | Scenarios triggered (count) | Exit control missing for any scenario? | Automated spec result | Pass / defect link |
|---|---|---|---|---|---|
| 1.3 | Background feed (`design/ux.md:76`) | | | | |
| 2.3 | Inline action rows (`design/ux.md:145`) | | | | |
| 3.3 | GuidanceRequestCard family (`design/ux.md:283`) | | | | |
| 4.3 | Peer-help rows (`design/ux.md:341`) | | | | |
| 5.3 | Scheduled prompts (`design/ux.md:375`) | | | | |
| 6.3 | Remind me (`design/ux.md:405`) | | | | |

### 7.2 Copy review (UX-AC58)

No copy may claim the operator is authenticated on the local listener, or promise delivery to an archived session.

1. Run the guard test (`web-app/src/lib/copy/copyGuard.test.ts`):

   ```bash
   cd web-app && npx jest --no-coverage --testPathPatterns="copyGuard"
   ```

   Output:

   ```text
   (paste here)
   ```

2. Read every string visible on the new surfaces on the manual instance (Background rows, action outcomes, saved-for-later, Unconfirmed, Tier B, Scheduled prompts) for the two forbidden claims.

Result: ______ Defects: ______

### 7.3 Real-device checks on iOS Safari and Android Chrome (UX-AC43, UX-AC46, UX-AC54)

Use the phone on the remote listener (`https://<host>:62872`, private CA cert installed) or the LAN address. Test one iOS Safari device and one Android Chrome device.

| Device and OS | Browser version | UX-AC43: soft keyboard up on a text field: Submit full width and visible above the keyboard; options stack vertically below 900 px | UX-AC46: bell panel is one scroll region, no nested scroll trap | UX-AC54: focus ring visible and never hidden behind a sticky action bar | Pass / defect link |
|---|---|---|---|---|---|
| iOS: | | | | | |
| Android: | | | | | |

### 7.4 Contrast measurement (UX-AC52)

Every state chip and badge needs an icon, visible text and an `aria-label`; text contrast >= 4.5:1 and icon/border contrast >= 3:1 in light and dark themes. Measure with browser devtools' contrast picker or Axe, and record the ratios.

| Chip / badge | Theme | Text ratio (>= 4.5) | Icon/border ratio (>= 3) | `aria-label` present | Pass / defect link |
|---|---|---|---|---|---|
| | light | | | | |
| | dark | | | | |

## 8. Close-out

- Usability script T1 to T10 executed and recorded: `docs/how-to/run-notification-usability-script.md`. Done: [ ]
- 14-day soak and quality-metric review done before any flip: same document, section "Soak review". Done: [ ]
- Each default flip is a separate one-line PR using `docs/how-to/flag-default-flip-pr-template.md`. Done: [ ]
- Instance stopped and private tmux server dropped (section 1). Done: [ ]

Operator: ______ Date: ______
