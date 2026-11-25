# /goal pilot: design, comparison plan and results

Status: **runner and operator command built, hook coexistence checked live once, pilot not yet run.** No pilot item has been executed, so the results table and verdict below are empty on purpose; nothing here is a measured claim.

## What exists

- `session/goalrunner` runs `claude -p '/goal <criteria> or stop after N turns' --output-format stream-json --verbose [--settings <file>]` as a direct subprocess (`exec.CommandContext`, no tmux, stdout parsed in-process). It is not visible in the terminal UI.
- Outcome fields: `EndReason` (`goal_met`, `turn_limit`, `impossible`, `error`, `cost_exceeded`), `CostUSD`, `Turns`, `WallTime`.
- Cost ceiling: default $2.00, estimated from `assistant` lines' usage (per-message maximum, unpriced models charged at the worst known rate, same method as `session/headless/cost_ceiling.go`). Crossing it cancels the process and reports `cost_exceeded`, whatever `/goal` is doing.
- Heartbeat: an `slog` line every 60 s with elapsed, turns, estimated cost and ceiling.
- Flag: `goal_mode` (`config.GoalModeFlag`), global only, default off. The runner is not called by any existing path, so the autonomous driver is unchanged for every session.
- Defaults: 20 turns, $2.00, 60 s; all set through `goalrunner.Config`.

## Not done

- **Dispatcher wiring.** No automatic backlog dispatch calls the runner. The operator-triggered entry point is `cmd/goal-pilot` (`--criteria`, `--dir`, `--max-turns`, `--ceiling-usd`, `--settings`), which refuses to run unless `goal_mode` is on and truncates criteria to 500 characters.
- **Pilot and baseline data (AC4, AC5).** Not collected: it needs 10 live runs (up to $2 each) on real items and a driver baseline. The only live data is the single 4-turn, $0.0145 run above, which is a smoke test, not a pilot item.
- **Result-line schema (checked live, 2026-11-25, one haiku run).** The real `type:"result"` object has **no `end_reason` or `cost_usd`**. It carries `subtype` (`success`), `is_error`, `terminal_reason` (`completed`), `num_turns` and `total_cost_usd`. The runner reads `end_reason`/`cost_usd` when present and otherwise derives: `error_max_turns` -> `turn_limit`, `is_error` -> `error`, else `goal_met`. Consequence: a `/goal` verdict of "impossible", or a stop by the in-condition turn bound, is **not distinguishable** in the result object and reports as `goal_met`; the pilot must classify those from the transcript text.
- **Flag name.** Claude Code's flag is `--settings`, not `--settings-file`; the runner uses `--settings`.

## Hook coexistence

- Verified statically (`settings_test.go`): the generated settings file holds Stapler Squad's Stop and PermissionRequest hooks and no `/goal` hook, because `/goal` registers its own session-scoped Stop hook at runtime. The two live in different sources, so they cannot overwrite each other in the file.
- **Verified at runtime (one live run, `claude -p '/goal the file hello.txt exists ... or stop after 3 turns' --output-format stream-json --verbose --settings s.json --max-budget-usd 0.30 --model haiku`):** `s.json` held only a Stop hook that appends a line to a marker file. Outcome: exit 0, result `subtype=success`, 4 turns, `total_cost_usd` 0.0145, goal met (`hello.txt` created), and the marker file had exactly one line, so the settings-file Stop hook fired once alongside `/goal`'s without error. Single run, one condition; it does not show behaviour when the goal is not met.

## Comparison plan

Run the pilot on at least 10 chore/docs items with machine-checkable criteria. Baseline: the last 10 chore/docs items finished by the autonomous driver. Same metrics for both.

| Item | Mode | Turns | Spend ($) | end_reason | Wall time | Rework needed |
|---|---|---|---|---|---|---|
| _pilot 1-10_ | goal | | | | | |
| _baseline 1-10_ | driver | | | | | |

Rework is assessed by hand: yes if a human had to fix or redo the result.

## Verdict

Pending data. Recommendation (adopt / extend / discard): **not yet decided.**
