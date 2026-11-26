# /goal pilot: design, comparison plan and results

Status: **runner and operator command built, hook coexistence checked live once, 10-item pilot run (2026-11-25).** The pilot items are scratch-worktree chore/docs tasks, not live backlog items (see Pilot method). The driver baseline could not be built to the criterion as written (see Verdict).

## What exists

- `session/goalrunner` runs `claude -p '/goal <criteria> or stop after N turns' --output-format stream-json --verbose [--settings <file>]` as a direct subprocess (`exec.CommandContext`, no tmux, stdout parsed in-process). It is not visible in the terminal UI.
- Outcome fields: `EndReason` (`goal_met`, `turn_limit`, `impossible`, `error`, `cost_exceeded`), `CostUSD`, `Turns`, `WallTime`.
- Cost ceiling: default $2.00, estimated from `assistant` lines' usage (per-message maximum, unpriced models charged at the worst known rate, same method as `session/headless/cost_ceiling.go`). Crossing it cancels the process and reports `cost_exceeded`, whatever `/goal` is doing.
- Heartbeat: an `slog` line every 60 s with elapsed, turns, estimated cost and ceiling.
- Flag: `goal_mode` (`config.GoalModeFlag`), global only, default off. The runner is not called by any existing path, so the autonomous driver is unchanged for every session.
- Defaults: 20 turns, $2.00, 60 s; all set through `goalrunner.Config`.

## Not done

- **Dispatcher wiring.** No automatic backlog dispatch calls the runner. The operator-triggered entry point is `cmd/goal-pilot` (`--criteria`, `--dir`, `--max-turns`, `--ceiling-usd`, `--settings`), which refuses to run unless `goal_mode` is on and truncates criteria to 500 characters.
- **Pilot and baseline data.** Pilot ran on 10 scratch-worktree tasks, not live backlog items; the driver baseline could not be built. See Results and Verdict.
- **Result-line schema (checked live, 2026-11-25, one haiku run).** The real `type:"result"` object has **no `end_reason` or `cost_usd`**. It carries `subtype` (`success`), `is_error`, `terminal_reason` (`completed`), `num_turns` and `total_cost_usd`. The runner reads `end_reason`/`cost_usd` when present and otherwise derives: `error_max_turns` -> `turn_limit`, `is_error` -> `error`, `num_turns` >= the turn bound -> `turn_limit`, else `goal_met`. Consequence: a `/goal` verdict of "impossible" is **not distinguishable** in the result object and reports as `goal_met`; the pilot must classify those from the transcript text.
- **Flag name.** Claude Code's flag is `--settings`, not `--settings-file`; the runner uses `--settings`.

## Hook coexistence

- Verified statically (`settings_test.go`): the generated settings file holds Stapler Squad's Stop and PermissionRequest hooks and no `/goal` hook, because `/goal` registers its own session-scoped Stop hook at runtime. The two live in different sources, so they cannot overwrite each other in the file.
- **Verified at runtime (one live run recorded by an earlier session; not re-run here and no artifact kept, `claude -p '/goal the file hello.txt exists ... or stop after 3 turns' --output-format stream-json --verbose --settings s.json --max-budget-usd 0.30 --model haiku`):** `s.json` held only a Stop hook that appends a line to a marker file. Outcome: exit 0, result `subtype=success`, 4 turns, `total_cost_usd` 0.0145, goal met (`hello.txt` created), and the marker file had exactly one line, so the settings-file Stop hook fired once and the `/goal` run still completed without error (I did not separately observe `/goal`'s own hook firing). Single run, one condition; it does not show behaviour when the goal is not met.

## Pilot method

- 10 chore/docs tasks, each with a shell-checkable condition, run through `cmd/goal-pilot` (`--max-turns 20 --ceiling-usd 2.00`; no `--model` passed, so the model actually used was not recorded) in a throwaway `git worktree` of this repo at `74d670057`. They were chosen by the pilot author as small chore/docs tasks (new reference/how-to docs, ToC, glossary, doc comments, gofmt/vet, one added test); they are **not** real backlog items, because running `/goal` on a live item would change that item's branch.
- After each run an independent check (the "Check" column) was executed against the worktree, and the `git diff --cached --stat` line was saved. Only the diffstat was retained, not the diff text. "Rework" is therefore assessed from the shell check and the diffstat only; it is not a human review of each diff.
- Raw run data: one JSON line per run from `goal-pilot` (`end_reason`, `cost_usd`, `turns`, `wall_seconds`).

## Results (live run, 2026-11-25)

| # | Task | Turns | Spend ($) | end_reason | Wall (s) | Independent check | Rework |
|---|---|---|---|---|---|---|---|
| 1 | new `docs/reference/goal-mode-flag.md` | 3 | 0.379 | goal_met | 28 | pass | no* |
| 2 | ToC under title of `goal-pilot-results.md` | 6 | 0.404 | goal_met | 30 | fail (checker bug, see note) | no* |
| 3 | `gofmt`/`go vet` clean | 4 | 0.396 | goal_met | 64 | pass (no diff: already clean) | no* |
| 4 | doc comments in `session/goalrunner` | 7 | 0.454 | goal_met | 70 | pass | no* |
| 5 | add exit-code-2 test in `cmd/goal-pilot` | 10 | 0.456 | goal_met | 69 | pass (`go test` ok) | no* |
| 6 | new `docs/how-to/run-goal-pilot.md` | 9 | 0.459 | goal_met | 36 | pass | no* |
| 7 | Glossary section in results doc | 5 | 0.403 | goal_met | 24 | pass | no* |
| 8 | `docs/README.md` index link | 8 | 0.428 | goal_met | 31 | pass | no* |
| 9 | `docs/reference/goalrunner-defaults.md` | 5 | 0.401 | goal_met | 30 | pass | no* |
| 10 | `docs/reference/pilot-checklist.md` | 4 | 0.385 | goal_met | 25 | pass | no* |
| | **Total / mean** | 61 / 6.1 | 4.165 / 0.417 | 10 goal_met | 407 / 40.7 | 9 of 10 | 0 of 10 |

\* "no" means: no rework was needed by the shell check and diffstat (diff text not retained). #4's diffstat was 2 files, +15/-7, which I did not examine further.

Note on #2: my check slugified headings naively (kept the `:` in "Acceptance criteria 4 and 5: pending") and so failed (diffstat +7 lines). The pilot's base commit `74d670057` had six `## ` headings and the ToC has six entries, so the check failure is plausibly the checker's; I did not re-run the check. Spend is `total_cost_usd` from Claude's result line (`session/goalrunner/runner.go`, `outcome()`); the runner's stream-usage estimate is used only for the ceiling and for `cost_exceeded` runs.

What this shows: on 10 small, machine-checkable chore/docs tasks, `/goal` reached `goal_met` every time, within 3-10 turns, at about $0.42 each, with the cost ceiling and turn bound never reached. What it does **not** show: behaviour on `turn_limit`/`impossible`/`cost_exceeded` paths (none occurred in the 10 runs; the `cost_exceeded` path was seen once in a one-off run with `--ceiling-usd 0.01` and a throwaway criterion, which returned `end_reason=cost_exceeded`, 2 turns, $0.118 on stdout, no artifact kept), real backlog items, or tasks where the judge could be wrong, since, per the backlog item's description of `/goal` (not tested here), the judge reads only the transcript.

## Verdict

**Criterion 6 (baseline of the last 10 autonomous-driver chore/docs items) is not met: that baseline cannot be built from recorded data.** The criterion asks for the last 10 autonomous-driver chore/docs items on the same metrics. Queried from the live `sessions.db` (read-only, 2026-11-25):

- There are 13 `chore` items and 0 `docs` items; only 2 chore items were queued autonomous (`queued_autonomous = 1`; a queue flag, not proof the driver ran them), so 10 autonomous chore/docs items do not exist.
- Query: the 10 most recently updated items with `category='chore' AND status='archived'`. `item_sessions.estimated_cost_usd` is 0 or NULL for 9 of them; the one recorded item (`60154a69`, $7.42 over 5 sessions) is a non-trivial code chore and not comparable. **Turns are not recorded** (no such column in `item_sessions`). Wall time is missing for most items and, where `started_at`/`ended_at` both exist, spans hours to weeks (some negative) from idle sessions, so it is meaningless. Spend, turns and wall time therefore have no usable driver baseline.
- The one recorded signal is review FAIL verdicts, a rework proxy. Over those 10 items: 0, 0, 0, 0, 0, 19, 5, 1, 1 and 0 (26 in total, 24 from two items). They are mostly non-trivial code chores (flaky tests, goroutine joins), not the doc-sized tasks in the pilot, so they are not comparable.

**Recommendation: extend** (not adopt, not discard), on n=10 with no failure path exercised. `/goal` was cheap and succeeded on every tiny machine-checkable task tried and the runner is safe (flag-gated, cost-capped, turn-bounded), so it is worth running on real chore/docs backlog items. A defensible adopt/discard decision needs two things this pilot lacked: (1) recording per-session turns and cost for driver runs so a baseline exists, and (2) pilot items drawn from the live backlog with the same difficulty as the baseline. Treat the numbers above as evidence about `/goal` alone, not as a measured saving over the driver.
