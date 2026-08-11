# ADR-001: Gemini/agy BeforeTool Hook Exit-Code Contract

**Status**: Accepted
**Date**: 2026-05-25

---

## Context

Gemini CLI and Antigravity CLI (agy) support a `BeforeTool` hook in their settings files (`~/.gemini/settings.json` and `~/.gemini/antigravity-cli/settings.json`). When a tool call is about to execute, the CLI invokes the hook shell command, passing the tool input via the `$TOOL_INPUT` environment variable.

The hook communicates its decision to the CLI via **exit code only**:

- `exit 0` — allow the tool call to proceed
- Non-zero exit (typically `exit 1`) — deny the tool call; the CLI blocks execution

This differs from the Claude Code `PreToolUse` hook contract, which reads a structured JSON object from the hook's **stdout** to determine the decision (`permissionDecision: "allow"` / `"deny"`).

The `ssq-hooks check` subcommand was originally written for Claude Code's JSON-stdout contract. Extending it directly to Gemini/agy would require changing the output format, breaking existing Claude deployments.

---

## Decision

Add a `--gemini` flag to `ssq-hooks check` that activates an alternate output path:

1. **Input**: reads `$TOOL_INPUT` from stdin (passed via `printf '%s' "$TOOL_INPUT" | ssq-hooks check --gemini`)
2. **Classification**: runs the same classifier and rules as the Claude path
3. **Output**: communicates the decision via exit code only — no JSON written to stdout

The `writeGeminiHookDecision()` function implements this contract:

| Decision   | Exit code | stdout | stderr                              |
|------------|-----------|--------|-------------------------------------|
| AutoAllow  | 0         | empty  | empty                               |
| Escalate   | 0         | empty  | empty (agy shows its own dialog)    |
| AutoDeny   | 1         | empty  | denial reason + rule ID (if set)    |

Denial reason is written to **stderr** (not stdout) because Gemini/agy behavior with non-empty stdout on a blocked hook is unspecified across versions — using stderr is the safe choice.

Escalate exits 0 (allow) because the correct behavior when ssq-hooks cannot classify a request is to let agy show its own native permission dialog, not to silently block.

---

## Consequences

- `writeGeminiHookDecision()` is a separate function from `writeHookDecision()` — they share no output path
- The `--gemini` flag is required for agy and Gemini installs; Claude installs must NOT use it
- If a future Gemini/agy version switches to a JSON-stdout contract like Claude's, `writeGeminiHookDecision()` can be updated independently without affecting the Claude path
- `ssq-hooks install gemini` and `ssq-hooks install agy` both write `<destBin> check --gemini` as the hook command string
