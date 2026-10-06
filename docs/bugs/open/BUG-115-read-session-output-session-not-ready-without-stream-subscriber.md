# BUG-115: read_session_output and run_command's read half return SESSION_NOT_READY when no stream subscriber has populated scrollback [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-09-28, live smoke test for backlog item `f7201b49-8318-43fb-a22f-0ce1926f9980` (PR #832 verification).
**Tracking**: backlog item `0759bf55-019d-4a83-9068-fa0d49fd51e6`

## Problem Description

`sessionNotReadyResult` (`server/mcp/tools_terminal.go:187-194`) returns `SESSION_NOT_READY` whenever
`scrollback.CurrentSequence(sessionID) == 0`. It is called from `readSessionOutput` (~line 234) and from
`runCommand` (~line 617), the latter *after* the command was already submitted (~line 582).

In the smoke test the target pane had visible output (`tmux capture-pane` showed the replies
`WRITE_MARKER_7431`, `STEER_MARKER_5582`, `RUN_MARKER_9917`), yet `read_session_output` and `run_command`
both returned `SESSION_NOT_READY`. The command ran; the caller got an error and no output.

## Root Cause

**Hypothesis, UNVERIFIED**: the scrollback sequence only advances when a terminal stream consumer (such as
the web UI) is attached, so MCP-only sessions never populate it. Confirm by attaching a stream and re-running,
and by tracing the scrollback append path.

## Impact

Probably the real cause of the empty-output half of GitHub issue #819. MCP-only callers get no output from
`run_command` or `read_session_output`. Not the Enter-submission bug, which PR #832 fixed.

## Suggested Fix

Fall back to a live tmux pane capture when scrollback is empty, or start the scrollback feed at session
creation. Make `run_command` return its result instead of an error after a successful submit.

## Related

- PR #832 (two-write submit fix), PR #837
- GitHub issue #819
- `project_plans/session-enter-not-sent/implementation/plan.md` (Story 1.3.4 recorded verdict)
