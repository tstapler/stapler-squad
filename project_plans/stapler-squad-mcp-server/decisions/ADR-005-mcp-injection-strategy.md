# ADR-005: MCP Server Injection into Managed Sessions

**Status**: Accepted
**Date**: 2026-04-18

## Context

Stapler Squad manages Claude Code sessions in tmux + git worktrees. For a Claude session to use the Stapler Squad MCP server, Claude Code must know the binary path and args — typically via `mcpServers` in `.claude/settings.local.json` in the session's working directory.

Two risks drive the design:
1. **Existing sessions must not be affected** unless the user explicitly opts in. Silently modifying running or paused sessions would break user trust and could corrupt mid-conversation state.
2. **Global installation is destructive in scope** — writing to `~/.claude/settings.json` affects every Claude session on the machine, including ones outside Stapler Squad.

Stapler Squad already has a precedent for this problem: `InjectHookConfig` in `server/services/approval_handler.go` writes (and merges) PermissionRequest hooks into `.claude/settings.local.json` at session creation using the same session-scoped file. That implementation is idempotent, merge-safe, and JSON-repair-capable. MCP injection must follow the same pattern.

## Decision

**Per-session, opt-in injection via `.claude/settings.local.json`**, following the `InjectHookConfig` pattern:

1. `create_session` gains an `inject_mcp` parameter (bool, default `true` when called from the MCP tool, default `false` from the web UI).
2. `update_session` gains an `inject_mcp` parameter for toggling injection on existing sessions.
3. A new `InjectMCPConfig(rootDir string, binaryPath string)` function (sibling to `InjectHookConfig`) handles the merge-and-write logic.
4. **Global install** is a separate explicit action only — a dedicated UI button / `install_mcp_globally` tool that writes to `~/.claude/settings.json`. It warns the user about scope before proceeding.
5. **Existing sessions** are never touched automatically. No migration, no background job.

The injected JSON block in `.claude/settings.local.json`:
```json
{
  "mcpServers": {
    "stapler-squad": {
      "type": "stdio",
      "command": "<absolute-path-to-stapler-squad-binary>",
      "args": ["--mcp"]
    }
  }
}
```

The binary path is resolved via `os.Executable()` at injection time, giving the absolute path of the running binary. This survives PATH changes and is stable across reboots.

## Rationale

- **Per-session injection** scopes the impact precisely — only the worktree directory is touched; other sessions in other directories are unaffected
- **Following `InjectHookConfig`** reuses proven merge logic that handles: file not existing, malformed JSON (with repair), pre-existing `mcpServers` from other tools (merged, not overwritten), and idempotency (no double-write if already present)
- **`os.Executable()` for binary path** is more reliable than `which stapler-squad` or a configured path — it always points to the actual running binary regardless of PATH
- **Separate global install** keeps the high-blast-radius action explicit and user-initiated; it should never happen automatically

## Consequences

- Sessions created from the UI do not get MCP injection by default. Users who want it must either enable it per-session via `update_session` or use global install.
- Sessions created via the MCP tool get injection by default (opt-out via `inject_mcp: false`). This makes sense — if you're already using the MCP tool, you want Claude to have access to MCP capabilities.
- The binary path baked into `.claude/settings.local.json` will break if the user moves the binary. The file can be re-injected by calling `update_session` with `inject_mcp: true` again.
- `.claude/settings.local.json` is typically gitignored — injection does not pollute git history.

## Alternatives Considered

- **CLAUDE.md injection**: Considered writing MCP server instructions and configuration context into the session's `CLAUDE.md` file. Rejected — `CLAUDE.md` is a user-authored, repo-committed file that encodes project conventions and instructions; Stapler Squad should not modify it. Appending to it would pollute git history (unlike `settings.local.json`, which is `.gitignore`'d), risk overwriting or conflating user-written instructions, and create confusing diffs. MCP configuration is structured JSON, not natural language — it belongs in a config file, not an instruction file.
- **Auto-inject into all new sessions regardless of source**: Rejected — modifies sessions the user did not explicitly opt in to. Violates the "don't affect existing ones unless the user explicitly wants it" requirement.
- **Environment variable injection at tmux session start**: Considered (`CLAUDE_MCP_SERVERS=...`). Rejected — Claude Code does not support MCP configuration via environment variables as of 2026; only settings files are respected.
- **Single global install automatically at stapler-squad startup**: Rejected — too broad; affects sessions outside Stapler Squad's scope and cannot be scoped to specific sessions.
- **Storing binary path in Stapler Squad config rather than `os.Executable()`**: Rejected — adds configuration burden; `os.Executable()` is always correct for the running binary.
