# Requirements: ssq CLI Subcommands for Session/Worktree Automation

**Item ID:** 06ed687a-e634-4658-838b-ab6948876101  
**Category:** Feature  
**Priority:** P3 (Normal)

## Problem Statement

The `stapler-squad`/`ssq` binary's CLI surface today is entirely about managing the server process itself: `reset`, `debug`, `version`, `test-pty`, `list`, `print-qr-codes`, plus a recently-added `get-session` command (in `cmd/commands/get_session.go`). There is no `ssq` subcommand to *create* a session, manage worktrees, steer a running session, or retrieve a git diff from a plain shell script or CI job.

The only programmatic control surface that mirrors the full session API is the MCP tool set (`mcp__stapler-squad__create_session`, `steer_session`, etc.), which requires an MCP-capable LLM client in the loop — it cannot be called from a bare shell.

This feature adds a thin CLI layer over the existing ConnectRPC `SessionService` that the web UI and MCP server already consume, making Stapler Squad scriptable without any agent in the loop.

## Goals

1. Scriptable session lifecycle from CI pipelines, cron jobs, shell aliases, and git hooks — without needing an LLM agent present.
2. Machine-readable output (`--json`) for programmatic consumers alongside human-readable defaults.
3. Consistent server-address story across all subcommands (`--server` flag / `SSQ_SERVER` env var, defaulting to `localhost:8543`).
4. Parity with the MCP tool set for the four most critical verbs: create, list, steer, diff.

## Non-Goals

- Streaming terminal I/O (that stays in the web UI / MCP / `StreamTerminal` RPC).
- Authentication/token management (out of scope for this iteration; local loopback is unauthenticated as today).
- Full feature parity with every RPC in the proto (MVP is the four verbs named in the ACs).
- Breaking changes to existing subcommands.

## Existing Codebase Context

| Artifact | Location | Notes |
|---|---|---|
| Cobra root command | `main.go:79` | Registers all subcommands |
| Existing pattern for ConnectRPC client commands | `cmd/commands/get_session.go` | Blueprint — uses `sessionv1connect.NewSessionServiceClient` |
| `cmd/commands/` package | `cmd/commands/` | `get_session.go`, `ensure_ports_free.go` today |
| Session service proto | `proto/session/v1/session.proto` | `CreateSession`, `UpdateSession` (steer_message), `GetSessionDiff`, `ListWorktrees` RPCs |
| Default server port | `main.go:320` | `localhost:8543` |
| MCP steer | `UpdateSessionRequest.steer_message` (field 11) | Only applies when `autonomous_mode=true` |

**Note:** the existing `get-session` command hard-codes `http://localhost:8080/api` — the correct default is `localhost:8543` per `main.go:320`.

## Acceptance Criteria

### AC-1 — `ssq create-session` subcommand
- Accepts `--title` (required), `--path` (required), `--branch`, `--program`, `--category`, `--prompt`, `--auto-yes`, `--one-shot` flags.
- Calls `SessionService.CreateSession` on the running server.
- On success, prints session ID (plain) or full `Session` object (`--json`).
- Non-zero exit code on error with an error message to stderr.

### AC-2 — `ssq list` correctness and JSON output
- The existing `list` command (in `main.go`) or a new `ssq list-sessions` wraps `SessionService.ListSessions`.
- `--json` flag emits a JSON array of session objects.
- `--status` filter flag (active/paused/all) is forwarded to `ListSessionsRequest`.

### AC-3 — `ssq steer` subcommand
- Accepts `<session-id>` positional arg and `--message` flag (required).
- Calls `SessionService.UpdateSession` with `steer_message` set.
- Returns success/error; non-zero exit on failure.
- Documents in `--help` that steering only takes effect when the session is in autonomous mode.

### AC-4 — `ssq get-diff` subcommand
- Accepts `<session-id>` positional arg.
- Calls `SessionService.GetSessionDiff`.
- Outputs raw unified diff (default) or JSON `GetSessionDiffResponse` (`--json`).

### AC-5 — Shared `--server` flag / `SSQ_SERVER` env var
- All new (and retrofitted `get-session`) subcommands accept `--server` (or `-s`) for the base URL.
- Defaults to `http://localhost:8543/api` (or `SSQ_SERVER` env var if set).
- Fix the hard-coded `localhost:8080` in the existing `get-session` command.

### AC-6 — Script-friendly output contract
- Default (non-`--json`) output: one key value per line or minimal prose, suitable for `awk`/`cut`.
- `--json` output: valid JSON to stdout, errors to stderr only.
- All commands are pipeable (no spinners, no ANSI unless stdout is a TTY).

### AC-7 — Usage documentation
- Each subcommand has a concise `--help` string referencing the server requirement.
- `README.md`'s CLI reference section is updated with usage examples for each new subcommand.

## Out-of-Scope for This Item

- `ssq watch` (streaming events).
- `ssq worktree create/list` (separate backlog item pending `backlog-branch-from-latest-main`).
- Remote auth tokens (the web UI's remote-access HTTPS path uses WebAuthn; not needed here for local use).
- Shell completion (nice-to-have, not blocking).
