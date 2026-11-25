# /goal pilot: design, comparison plan and results

Status: **runner built, pilot not yet run.** No pilot item has been executed, so the results table and verdict below are empty on purpose; nothing here is a measured claim.

## What exists

- `session/goalrunner` runs `claude -p '/goal <criteria> or stop after N turns' --output-format stream-json --verbose [--settings <file>]` as a direct subprocess (`exec.CommandContext`, no tmux, stdout parsed in-process). It is not visible in the terminal UI.
- Outcome fields: `EndReason` (`goal_met`, `turn_limit`, `impossible`, `error`, `cost_exceeded`), `CostUSD`, `Turns`, `WallTime`.
- Cost ceiling: default $2.00, estimated from `assistant` lines' usage (per-message maximum, unpriced models charged at the worst known rate, same method as `session/headless/cost_ceiling.go`). Crossing it cancels the process and reports `cost_exceeded`, whatever `/goal` is doing.
- Heartbeat: an `slog` line every 60 s with elapsed, turns, estimated cost and ceiling.
- Flag: `goal_mode` (`config.GoalModeFlag`), global only, default off. The runner is not called by any existing path, so the autonomous driver is unchanged for every session.
- Defaults: 20 turns, $2.00, 60 s; all set through `goalrunner.Config`.

## Not done

- **Dispatcher wiring.** Nothing yet calls `goalrunner.Run` for a backlog item, so the flag has no effect. The pilot needs an operator-triggered caller that passes the item's description (first 500 characters) as `Criteria`.
- **Result-line schema.** `end_reason` and `cost_usd` are read as the item specifies; `is_error`, `subtype=error_max_turns` and `total_cost_usd` are accepted as fallbacks. This has been tested against fake streams only, not a live `claude`; check one real result object before trusting the mapping.
- **Flag name.** Claude Code's flag is `--settings`, not `--settings-file`; the runner uses `--settings`.

## Hook coexistence

- Verified statically (`settings_test.go`): the generated settings file holds Stapler Squad's Stop and PermissionRequest hooks and no `/goal` hook, because `/goal` registers its own session-scoped Stop hook at runtime. The two live in different sources, so they cannot overwrite each other in the file.
- **Unverified at runtime:** that Claude Code runs both Stop hooks without interference in `-p` mode. Confirm with one live run and record it here.

## Comparison plan

Run the pilot on at least 10 chore/docs items with machine-checkable criteria. Baseline: the last 10 chore/docs items finished by the autonomous driver. Same metrics for both.

| Item | Mode | Turns | Spend ($) | end_reason | Wall time | Rework needed |
|---|---|---|---|---|---|---|
| _pilot 1-10_ | goal | | | | | |
| _baseline 1-10_ | driver | | | | | |

Rework is assessed by hand: yes if a human had to fix or redo the result.

## Verdict

Pending data. Recommendation (adopt / extend / discard): **not yet decided.**
