# Requirements: Full Antigravity CLI (agy) Support

## Context

Stapler Squad manages AI agent sessions (Claude Code, Gemini CLI, Aider, etc.) in isolated tmux sessions. Antigravity CLI (`agy`) is Google's successor to Gemini CLI — same TUI codebase, rewritten core in Go, shipped as `agy` binary, config at `~/.gemini/antigravity-cli/settings.json`. Gemini CLI shuts down June 18, 2026.

Partial `agy` support was already added in this branch:
- `agy` listed in `GetAvailablePrograms()` candidates (`config/config.go`)
- `agy` entry in `recursiveEvalPrograms` (`pkg/classifier/command_parser.go`)
- `agy` label and emoji in frontend (`programs.ts`, `SessionRow.tsx`)
- Frontend program dropdowns now fetch dynamically from `/api/server-info`

## Out of scope (already done)

- UI label "Antigravity" in program dropdown
- `◆` emoji in session row
- Dynamic program list in frontend dropdowns
- `agy` in `GetAvailablePrograms()` candidates

## In-Scope Requirements

### REQ-1: `ssq-hooks install agy` — automated hook install

**Description**: New install target that fully auto-installs the `BeforeTool` hook into `~/.gemini/antigravity-cli/settings.json`.

**Acceptance criteria**:
- `ssq-hooks install agy` copies the binary to `~/.local/bin/ssq-hooks` (same as claude target)
- Patches `~/.gemini/antigravity-cli/settings.json` with a `BeforeTool` hook entry using the correct agy/Gemini hook schema
- Idempotent: re-running does not duplicate the hook entry
- Creates `~/.gemini/antigravity-cli/` directory if it doesn't exist
- Prints confirmation of what was written and where
- Usage string updated to include `agy` as a valid target

### REQ-2: `ssq-hooks install gemini` — upgrade from manual to automated

**Description**: Current `installGemini()` only prints manual instructions. Upgrade it to auto-patch the settings file, matching the same level of automation as the Claude install.

**Acceptance criteria**:
- `ssq-hooks install gemini` copies the binary to `~/.local/bin/ssq-hooks`
- Patches the first found Gemini config file (`~/.gemini/settings.json`, falling back to `~/.gemini/config.json`) with a `BeforeTool` hook entry
- Idempotent
- Prints confirmation

### REQ-3: `ssq-hooks check --gemini` — Gemini/agy payload adapter

**Description**: The existing `ssq-hooks check` subcommand reads Claude's `PermissionRequestPayload` JSON (fields: `tool_name`, `tool_input`, `cwd`, `session_id`, etc.). Gemini CLI and `agy` pass `$TOOL_INPUT` in a different schema. A `--gemini` flag must add a parsing path that translates the Gemini/agy payload into the classifier's internal format.

**Acceptance criteria**:
- `ssq-hooks check --gemini` reads the Gemini/agy payload from stdin
- Translates it to `PermissionRequestPayload` before classification
- Falls back gracefully if the payload is unrecognized (escalate, do not crash)
- The install targets for `gemini` and `agy` use `ssq-hooks check --gemini` in the hook command string
- Existing `ssq-hooks check` (no flag) remains unchanged for Claude compatibility

**Note on Gemini/agy payload schema**: The exact `$TOOL_INPUT` format must be confirmed by either:
  (a) Running `agy` and triggering a tool call with the hook present to capture real output, OR
  (b) Reading the Gemini CLI open-source codebase to find the hook payload serialization
  
  The implementation must handle at minimum: `tool_name`/`name` field for tool identification, and a `command` or `args.command` field for Bash tool calls.

### REQ-4: Detection patterns for `agy` in `detector.go`

**Description**: Confirm or add TUI status patterns for `agy`. Since `agy` uses the same TUI codebase as Gemini CLI, the existing `gemini_*` patterns (`gemini_ready`, `gemini_working`, `gemini_permission`, `gemini_allow_execution`) should cover `agy`. Verify this assumption and add `agy`-specific patterns only if they differ.

**Acceptance criteria**:
- Existing `gemini_*` patterns tested against a real `agy` session screenshot or log output
- If patterns are sufficient: add a comment in `detector.go` noting that `agy` is covered by Gemini patterns
- If patterns differ: add `agy_*` pattern variants with correct regex
- At minimum, the `NeedsApproval` pattern `gemini_permission` ("Yes, allow once") covers the agy permission prompt

## Technical Constraints

- Go: follow existing patterns in `cmd/ssq-hooks/main.go`
- New install functions must be idempotent (safe to re-run)
- Hook command strings: `agy` and Gemini use `BeforeTool` as a shell string (not Claude's JSON array format)
- `ssq-hooks check` exit codes and stdout format must remain Claude-compatible for existing deployments
- No new dependencies

## Non-Goals

- Resume/conversation-ID support for `agy` sessions (not needed, `agy` uses `--continue` flag differently)
- MCP configuration for `agy` (separate concern)
- Migrating existing Gemini sessions to `agy` in the session database
