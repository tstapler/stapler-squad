# ADR-001: Use `list-sessions` as the new RPC-backed listing verb; keep `list` as-is

**Date**: 2026-10-18
**Status**: Accepted

## Context

The existing `list` command (`main.go`, `listSessionsCmd`) reads the SQLite database directly — it opens `session.NewEntRepository`, constructs a `session.Storage`, and calls `LoadInstances()`. It does not need a running server. It is the fallback an operator uses to inspect session state when the server process itself is crashed or unavailable.

The new AC-2 requirement is for a command that:
- calls the live `SessionService.ListSessions` RPC,
- supports `--json` output with correct proto enum serialisation,
- accepts `--status`, `--category`, and `--search` filter flags forwarded to `ListSessionsRequest`.

Two sub-options exist for naming:
1. Replace `list` — change `Use: "list"` to point at the new RPC-backed implementation.
2. Add `list-sessions` — keep `Use: "list"` unchanged, introduce `Use: "list-sessions"` as the new verb.

## Decision

Add `list-sessions` as a new subcommand. Do not modify or remove `list`.

## Rationale

- The existing `list` command works without a running server. It is a useful operator escape hatch precisely because it bypasses the RPC layer. Removing it degrades the debugging story.
- Replacing `list`'s implementation silently changes an existing command's behaviour and dependencies (now requires a live server, previously did not), which is a breaking change for any script or alias relying on `ssq list` working without the daemon.
- `list-sessions` is parallel with `get-session` (already established in the codebase), making the naming convention consistent: `{verb}-session(s)`.
- The cost of documenting two list commands is low; the cost of regressing an operator tool is high.

## Consequences

- `ssq list` continues to read SQLite directly — useful for debugging a crashed daemon.
- `ssq list-sessions` requires a running server — useful for scripting and CI.
- `--help` output on the root command shows both; their `Short` descriptions disambiguate:
  - `list`: `"List all sessions (reads local database; no server required)"`
  - `list-sessions`: `"List sessions via the running server (supports --json, --status, --search)"`
