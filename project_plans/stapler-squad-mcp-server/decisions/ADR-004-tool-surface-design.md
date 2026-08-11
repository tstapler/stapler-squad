# ADR-004: MCP Tool Surface — 15 Tools in 4 Families

**Status**: Accepted
**Date**: 2026-04-18

## Context

LLMs degrade in tool selection accuracy above 30–40 tools. Tool names, descriptions, and parameters function as prompts — they directly affect LLM decision quality.

Initial design had 13 tools. Review identified three gaps:
1. The most common workflow (run a command, get output) required 3 tool calls: `write_to_session` → `wait_for_output` → `read_session_output`. This is expensive in context and round-trips.
2. `list_sessions` with a default of 50 sessions would fill LLM context before the LLM had done anything useful.
3. There was no way to send Ctrl+C or other control characters — a practical necessity for interrupting hung commands.

## Decision

Expose 15 tools in 4 semantic families:

**Session Discovery** (read-only): `list_sessions`, `get_session`, `search_sessions`

**Session Lifecycle**: `create_session`, `pause_session`, `resume_session`, `stop_session` (requires `confirm: true`), `update_session`

**Terminal I/O**:
- `run_command` — composite: write + wait + read in one call; covers ~80% of use cases
- `read_session_output` — snapshot read with ANSI stripping, truncation metadata
- `write_to_session` — fire-and-forget PTY write; for edge cases `run_command` can't handle
- `send_control` — sends Ctrl+C/D/Z/L; required for interrupt/EOF workflows
- `wait_for_output` — pattern-matching poll; for cases where `run_command`'s output-stability heuristic doesn't fit

**VCS**: `get_session_diff`, `list_session_branches`

**Key schema changes from initial design**:
- `list_sessions`: default limit 10 (was 50), with cursor pagination (`next_cursor` field)
- `search_sessions`: promoted as the preferred first tool; has explicit note in description
- `run_command` + `send_control`: new additions

## Rationale

- 15 tools is within the ≤18 safe zone; still well under the ~30–40 degradation threshold
- `run_command` eliminates the 3-call sequence for the most common LLM workflow, reducing context consumption and round-trips
- `send_control` is required for any workflow involving long-running commands — without it, the LLM cannot interrupt a hung process
- `list_sessions` default of 10 prevents context bloat; cursor pagination lets the LLM page if needed
- Promoting `search_sessions` in its description guides the LLM toward the more efficient path

## Consequences

- Tool descriptions for `run_command`, `write_to_session`, and `send_control` must clearly explain when to use each; ambiguous descriptions will cause the LLM to pick the wrong tool
- `run_command`'s output-stability heuristic (poll until sequence stops changing for 2s) requires careful tuning — commands that produce slow/streaming output may return too early
- New capabilities post-v1 should be evaluated against the 18-tool cap before adding

## Alternatives Considered

- **Keep 3-call sequence instead of `run_command`**: Rejected — 3 tool calls × context per call × many commands = significant context waste in typical workflows
- **`send_control` as parameter of `write_to_session`**: Considered (`write_to_session` with `control_char: "C"`). Rejected — it blurs the tool boundary and makes descriptions harder to write clearly; a separate tool is more discoverable
- **`list_sessions` default of 20**: Splitting the difference. Rejected — 10 is still generous for "browse" use cases, and the LLM can always paginate or search; starting low is better than starting high
