# Requirements: goal-pilot

## Background

Claude Code's `/goal` command wraps a session-scoped Stop hook: a small model (Haiku by default)
judges after each turn whether a stated condition is met / not met / impossible. This is verified
against https://code.claude.com/docs/en/goal. Known limits:
- The judge reads only the transcript; it cannot run tools.
- No built-in turn or spend cap — must be bounded with "or stop after N turns" in the condition
  string (the judge evaluates it).
- Requires hooks enabled and auto mode to run unattended.
- Resume resets the turn count.

The current autonomous driver (`session/autonomous/autonomous_driver.go`) polls the pane,
calls a headless LLM judge on each completion, and respawns sessions on stall. It has known
failure modes (all stalls mislabelled "max turns reached", panics swallowed). The hypothesis
is that `/goal` can replace this loop for a narrow, low-risk item class, reducing complexity
and potentially reducing cost.

## Goal

Run a controlled pilot comparing `/goal`-driven sessions against the existing autonomous driver
on a low-risk item class. Record findings in `docs/explanation/` and decide whether to adopt,
extend, or discard `/goal` as a driver.

## Item Class for Pilot

Chore/docs items with machine-checkable completion criteria (e.g., `make ci` exits 0, a
specific file exists, a markdown doc is updated). These are the safest class because:
1. Completion is verifiable without subjective judgement.
2. Failed items are low-cost to discard.
3. CI provides a ground truth the judge string can reference.

## Acceptance Criteria

1. A new `GoalMode` launch option (feature-flagged, disabled by default) launches `claude`
   with `-p '/goal <criteria> or stop after N turns' --output-format stream-json --verbose`
   instead of the interactive tmux session, where `<criteria>` and `N` are configurable per
   item (default N=20).

2. The wrapper reads `stream-json` output and extracts `end_reason` (goal_met, turn_limit,
   impossible, error) and per-turn cost from the JSON stream, surfacing them as structured
   outcome fields in `AutonomousDriverOutcome` (or equivalent).

3. A cost ceiling terminates the process and records outcome `cost_exceeded` when spend passes
   a configured limit (default $2.00 per item), independent of `/goal`'s own exit.

4. A heartbeat (configurable interval, default 60 s) logs a live-status line so the
   operator can see the session is running, matching the behaviour the existing driver provides.

5. The new mode is exercised on at least 10 chore/docs items (run sequentially or in parallel,
   operator-triggered), and the results are recorded in a comparison table in
   `docs/explanation/goal-pilot-results.md` covering: turns, spend ($), end_reason, wall time,
   and "rework needed after stop" (yes/no, manually assessed).

6. `docs/explanation/goal-pilot-results.md` includes a verdict section comparing `/goal`-driven
   items against a baseline of the last 10 autonomous-driver chore/docs items on the same metrics,
   with a recommendation (adopt / extend / discard).

7. The `/goal` integration does not use tmux — it launches `claude` as a subprocess directly
   (via `session/launchcmd`), with stdout/stderr captured, and the session is not visible in the
   Stapler Squad terminal UI during the pilot (to avoid complexity).

8. Compatibility: existing autonomous-driver behaviour is unchanged for sessions not opted into
   goal mode; the feature flag is off by default and can be enabled per-item or globally.

9. Hooks compatibility is verified: Stapler Squad's own Stop hook (`ssq-hook-handler stop`) must
   not conflict with `/goal`'s Stop hook. If they do conflict, a documented workaround or fix is
   included.

## Out of Scope

- Replacing the full autonomous driver (this is a pilot/experiment, not a migration).
- UI for the new mode (operator-triggered only during the pilot phase).
- Multi-turn conversation / continuation after goal-not-met (the existing respawn logic is out).
- Applying `/goal` to feature/bugfix items (only chore/docs for the pilot).
