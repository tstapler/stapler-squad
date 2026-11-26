# Research: goal-pilot

## Existing Autonomous Driver (Verified)

**Files:** `session/autonomous/autonomous_driver.go`, `session/driver/session_driver.go`

The current autonomous driver runs sessions through a tmux-based loop:
1. Sends an initial prompt via the driver (`session/driver/`), waits for the ">" ready prompt.
2. Polls the pane's output every ~2 seconds looking for completion signals.
3. After each apparent completion, calls a headless LLM judge (`session/headless/`) to verify done/stuck.
4. On stuck: fires a CompletionCallback → `AutonomousDriverOutcome{Stuck: true}` → session respawns.
5. On done: fires callback with `{Done: true, PRUrl: ...}`.

Known failure modes (from `docs/explanation/autonomous-stall-causes.md`, VERIFIED):
- All non-DONE exits report `"max turns reached"` regardless of actual cause.
- Panics inside the loop are swallowed; session stuck with `AutonomousMode == true` but no driver.
- Externally-requested stops (user toggle, server shutdown) show as `Stuck`.

## /goal Command (Verified against https://code.claude.com/docs/en/goal)

`/goal <condition>` injects a Stop hook into the session's `~/.claude/settings.json`:
```json
{
  "hooks": {
    "Stop": [
      {"type": "command", "command": "claude --goal-eval '<condition>'"}
    ]
  }
}
```
The judge (Haiku by default) reads the transcript and returns:
- `goal_met` → Claude Code exits cleanly.
- `goal_not_met` → Claude Code continues (re-enters the agentic loop).
- `impossible` → Claude Code exits with an impossible signal.
- No built-in turn or cost cap.

Invocation pattern for unattended pilot:
```
claude -p '/goal <criteria> or stop after 20 turns' \
  --output-format stream-json --verbose
```

The `--output-format stream-json` flag causes Claude Code to emit a NDJSON stream where each
line is a JSON object. The `end_reason` field on the final `result` object indicates why the
session stopped.

## Hook Conflict Analysis (VERIFIED from `server/services/hook_injector.go`)

Stapler Squad already injects a Stop hook at `/api/hooks/stop` (via `HookStopNotification`).
Claude Code supports multiple hooks per event (they're a list of `{type, command}` objects).
So `/goal`'s Stop hook and Stapler Squad's Stop notification hook can coexist in the same
`Stop` array — they are both run by Claude Code in order.

**Risk:** If the pilot runs `claude` as a subprocess (not managed by Stapler Squad's session
wiring), the per-session hooks are injected via a temporary `--settings-file` override, not
the global `~/.claude/settings.json`. The global hook with `/api/hooks/stop` will fire for any
unmanaged subprocess unless we avoid writing global hooks for the pilot sessions. This is
manageable by using `--settings-file <temp-path>` for the subprocess, containing only the
`/goal` configuration and the approval hook, without the SSQ notification stop hook.

## LaunchCmd and Subprocess Mode (Verified from `session/launchcmd/launchcmd.go`)

`ExecCommand` supports:
- `Env []EnvVar` — environment overrides.
- `Argv []Arg` — typed argument list (Literal, Word, Flag, FileContents, RawShell).
- `StdinFile string` — stdin redirect from file.
- `StderrAppend string` — stderr append redirect.

The subprocess can be launched via `exec.CommandContext` without tmux, capturing stdout/stderr
directly. This is simpler than the existing tmux-based driver loop for the pilot.

## stream-json Output Schema (VERIFIED from claude --help)

Each line from `--output-format stream-json --verbose` is one of:
- `{"type":"system","subtype":"init",...}` — startup
- `{"type":"assistant","message":{...},"turn_number":N}` — assistant turn
- `{"type":"result","subtype":"success","end_reason":"goal_met","cost_usd":X,"turns":N}` — final
- `{"type":"result","subtype":"error","error":"..."}` — failure

The `end_reason` values: `goal_met`, `turn_limit` (from "or stop after N turns"), `impossible`,
`error`, `interrupted`.

## Cost Ceiling Pattern (Verified from `session/headless/cost_ceiling.go`)

The headless pool already has a `CostCeiling` mechanism. For the subprocess pilot, we need a
similar pattern: watch the stream for cumulative cost from per-turn `usage_input_tokens` and
`usage_output_tokens`, kill the process if the ceiling is exceeded, and record `cost_exceeded`
as the outcome.

## Comparison Metrics Available

For 10+ pilot items:
- Turns (from `result.turns`)
- Cost in USD (from `result.cost_usd`)
- End reason (`goal_met` / `turn_limit` / `impossible` / `cost_exceeded` / `error`)
- Wall time (measured by the wrapper)
- Rework needed (manual boolean — did the generated code/doc require a follow-up fix?)

Baseline: last 10 chore/docs items run via current autonomous driver — their outcomes are in
the DB via `nudge_outcome` rows and in the logs as `autonomous_stuck`/`done` events.

## Open Questions

1. Does `/goal` work correctly when the Stop hook judge is called while another Stop hook
   (Stapler Squad's) is also registered? Need empirical verification with a real `--settings-file`.
2. What is the actual per-session cost of a Haiku judge call? Not in docs; must run one real
   item to measure.
3. Can `--output-format stream-json` be combined with the interactive `-p` prompt injection
   pattern Stapler Squad uses, or does it require a pure CLI flow?
4. Does resume (`--resume <session-id>`) reset the `/goal` turn count in the Stop hook? 
   The description says yes; not independently verified.
