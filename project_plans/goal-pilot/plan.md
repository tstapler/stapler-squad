# Plan: goal-pilot

## Approach

Minimal-footprint experiment: add a `GoalMode` runner in a new package
(`session/goalrunner`) that launches `claude` as a subprocess with `/goal` and
`--output-format stream-json`, wrapped in a cost ceiling and heartbeat. Gate it
behind a feature flag. Wire it to a new backlog item class filter. Collect results
from 10+ chore/docs items and write findings.

**Not** replacing the existing autonomous driver — this is additive.

## Package Structure

```
session/goalrunner/
  runner.go          — GoalRunner struct: launch, stream-json parse, cost ceiling, heartbeat
  runner_test.go     — table-driven unit tests with fake process output
  outcome.go         — GoalOutcome type (end_reason, cost, turns, wall_time)
  outcome_test.go    — marshal/unmarshal
```

New flag in `server/services/flagsvc`:
- `goal_mode_enabled` — bool, default false; gates the new runner globally.
- `goal_mode_item_class` — string, default `"chore"` — which item class gets the new runner.

## Tasks

### T1: `session/goalrunner` package (backend)

Implement `GoalRunner.Run(ctx, item) GoalOutcome`:
1. Build an `ExecCommand` via `launchcmd`: `claude -p '/goal <criteria> or stop after N turns' --output-format stream-json --verbose --settings-file <tmpfile>`.
2. Write a minimal `<tmpfile>` (temp `settings.json`) containing only the `/goal` stop hook and
   the SSQ permission-request hook (no SSQ stop notification hook, to avoid double-firing).
3. Exec the process with `exec.CommandContext`, capturing stdout line by line.
4. Parse each NDJSON line; on `type=result` extract `end_reason`, `cost_usd`, `turns`.
5. Watch cumulative cost (from per-turn usage fields); kill and return `cost_exceeded` if
   the ceiling ($2.00 default) is crossed.
6. Log a heartbeat line every 60 s.
7. Return `GoalOutcome` with all fields populated.

Estimate: 4h

### T2: Feature flag wiring (backend)

Add `goal_mode_enabled` and `goal_mode_item_class` to `server/services/flagsvc`.
Register in the flag spec registry. Add to `server/dependencies.go` wiring.

Estimate: 1h

### T3: Backlog dispatcher hook (backend)

In `session/backlogstore/status_writers.go` or its caller in `server/services/`,
check: when an item of the configured class is dispatched, and `goal_mode_enabled`
is true, launch `GoalRunner.Run` in a goroutine instead of the standard autonomous driver.
Write the outcome back as a `StopReason` on the item when done.

Estimate: 2h

### T4: Hooks conflict verification (manual + test)

Write a small integration test (`session/goalrunner/hooks_compat_test.go`) that:
1. Writes a temp `settings.json` with both a fake SSQ Stop hook and a `/goal` Stop hook.
2. Verifies Claude Code's hook system would call both (by dry-running `claude --print-config`
   or by reading the produced settings file and asserting both commands are present).
3. Documents whether the two hooks can coexist.

Estimate: 1h

### T5: Pilot run + data collection

Operator-triggered: enable the flag for 10 chore/docs items in the real backlog.
Record results in a spreadsheet/markdown table. Assess "rework needed" manually.

Estimate: operator time, not engineering time.

### T6: Results doc (docs)

Write `docs/explanation/goal-pilot-results.md`:
- Comparison table (turns, spend, end_reason, wall time, rework) for pilot vs baseline.
- Verdict: adopt / extend / discard.
- If adopt: list what needs to change before full rollout (UI, error handling, resume,
  multi-turn respawn).

Estimate: 1h

### T7: Unit tests for GoalRunner (test)

Table-driven tests covering:
- Normal `goal_met` stream → outcome has correct fields.
- `turn_limit` → outcome records `turn_limit`.
- `impossible` → outcome records `impossible`.
- Cost ceiling exceeded → process killed, outcome `cost_exceeded`.
- Heartbeat fires at correct interval (fake clock).
- Malformed JSON line → skipped, no panic.

Estimate: 2h

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `/goal` Stop hook conflicts with SSQ Stop hook | Medium | High | Use `--settings-file` with only needed hooks; verified in T4 |
| `stream-json` output schema changes in a CC update | Low | Medium | Pin CC version in Brewfile; test against pinned version |
| Haiku judge cost is higher than expected | Low | Low | Cost ceiling ($2) caps per-item spend |
| Pilot items are not representative of real chore/docs work | Medium | Medium | Pick 10 items spanning 3+ different chore/doc types |
| Resume resets turn count (documented but unverified) | Low | Low | Do not use resume during pilot; treat each run as fresh |

## Adversarial Review (Self)

**Q: Why not just run the existing autonomous driver's LLM judge with a machine-checkable
condition instead of using /goal?**
A: `/goal` is Anthropic's supported, maintained mechanism for this exact use case. Building
our own judge over the same pattern is duplicating work and missing the efficiency gain (Haiku
vs Sonnet judge). The pilot is specifically to measure whether `/goal`'s judge is accurate
enough for our use case.

**Q: Is 10 items a sufficient sample for a verdict?**
A: It's the minimum stated in the item's description. With a narrow item class (chore/docs)
the variance is low. The verdict explicitly carries confidence levels; if the data is noisy
we can say so.

**Q: Does the pilot need UI integration?**
A: No, explicitly out of scope. The pilot is operator-triggered via the flag; full UI can wait
for the adopt decision.

**Q: What if the subprocess lacks permission to write the temp settings file?**
A: It writes to `os.TempDir()`. If that fails, the runner returns `error` and falls through
to the existing autonomous driver. No new failure mode is introduced.
