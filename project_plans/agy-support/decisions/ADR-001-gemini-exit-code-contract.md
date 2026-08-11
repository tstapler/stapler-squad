# ADR-001: Gemini/agy Hook Output: Exit Code Only (No JSON Stdout)

**Date**: 2026-05-25
**Status**: Accepted
**Context**: agy-support feature — `ssq-hooks check --gemini` output format

---

## Context

Claude Code hooks communicate decisions via JSON on stdout (`hookSpecificOutput` with `permissionDecision: "allow"|"deny"`). Gemini CLI and agy use a simpler `BeforeTool` hook format where the hook is a shell string receiving `$TOOL_INPUT` as an env var.

The question: does Gemini/agy interpret the hook result via:
1. Exit code (0 = allow, non-zero = deny), OR
2. Stdout JSON (parsing a structured response), OR
3. Both?

## Evidence

- `install-gemini-hook.sh` (in this codebase) installs `echo "$TOOL_INPUT" | ssq-hooks check` — the result of this command is `ssq-hooks check`'s stdout (Claude JSON format). The script does not capture or post-process stdout, suggesting it was written for a Gemini version that ignores stdout.
- The existing `installGemini()` stub (before this feature) similarly does not hint at a structured stdout response.
- Architecture research (`architecture.md` section 3a): "Allow: exit 0, no stdout (or stdout ignored)" and "Deny: exit non-zero (or specific stdout — TBD from Gemini docs)".
- No official Gemini CLI / agy documentation was available at planning time to confirm definitively.

## Decision

For `--gemini` mode, `writeGeminiHookDecision()` uses **exit codes only**:
- `AutoAllow` → exit 0, no stdout
- `Escalate` → exit 0, no stdout (agy shows its own permission dialog)
- `AutoDeny` → exit 1, denial reason written to **stderr** (not stdout)

Rationale for stderr over stdout on deny:
- If agy ever starts interpreting non-empty stdout as a block signal, writing JSON to stdout on AutoAllow would produce false denials. Stderr is safer.
- Denial reason on stderr is visible in the user's terminal without risk of being parsed as a Gemini protocol message.

## Consequences

- The Claude-compatible `writeHookDecision()` (JSON stdout) is untouched; existing Claude deployments are unaffected.
- If future agy versions require structured stdout for denial messages (e.g., to display formatted errors), `writeGeminiHookDecision()` will need to be extended.
- The assumption should be validated by triggering a known-deny rule against a live agy session before the PR is merged (see Task 4.1.2 in plan.md).

## Alternatives Considered

- **Write Claude JSON on stdout in --gemini mode**: Rejected — risk of false denials if Gemini interprets non-empty stdout as a block.
- **Write a minimal plain-text stdout on deny**: Rejected — adds stdout output with no confirmed benefit; stderr is sufficient.
- **Always exit 0 and rely on escalation**: Rejected — defeats the purpose of automated deny rules.
