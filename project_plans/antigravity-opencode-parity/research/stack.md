# Stack Research: Antigravity CLI + Open Code Feature Parity

## Tool Versions

| Tool | Version |
|------|---------|
| agy (Antigravity CLI) | 1.0.14 |
| opencode | 1.4.0 |

---

## R1: Agy One-Shot AI Client

**Finding:** `agy --print` flag confirmed working. Tested: `echo "hello" | agy --print` returns a response successfully.

**Current gap:** `knownCLIAgents` in `server/services/cli_ai_client.go` has `claude`, `gemini`, `opencode` but **not** `agy`.

**Fix:** Insert between gemini and opencode:
```go
{
    Name:            "agy",
    Binary:          "agy",
    Args:            func() []string { return []string{"--print"} },
    PromptSeparator: "\n\n---\n\n",
},
```

The `-p` shorthand also works. `--print-timeout` defaults to 5m which is generous.

---

## R2: Agy Hook Install Path Logic

**Finding:** Both hook paths currently exist on disk and contain identical content:
- `~/.gemini/config/hooks.json` — authoritative global path
- `~/.gemini/antigravity-cli/hooks.json` — secondary path

**Current behavior:** `installAgy()` unconditionally patches BOTH paths (lines 876–887 in `cmd/ssq-hooks/main.go`). This is divergent from `installGemini()` which uses first-found logic.

**Actual hooks.json format** (confirmed live from both paths):
```json
{
  "stapler-squad": {
    "PreToolUse": [
      {
        "hooks": [
          {
            "command": "/home/tstapler/.local/bin/ssq-hooks check --antigravity",
            "timeout": 10,
            "type": "command"
          }
        ],
        "matcher": "*"
      }
    ],
    "enabled": true
  }
}
```

**Fix strategy:** Mirror `installGemini()` — probe candidates in order, patch only the first found. Create `~/.gemini/config/hooks.json` if neither exists (it is the documented authoritative path). Candidate order:
1. `~/.gemini/config/hooks.json`
2. `~/.gemini/antigravity-cli/hooks.json`

The hook format used by `patchAntigravityHooks()` matches the live file exactly — no format change needed.

---

## R3: Agy Detection Patterns

**Finding:** `session/detection/binaries/agy.go` comment states "Agy uses the same TUI codebase as Gemini CLI". agy v1.0.14 (`--help` output) confirms it shares Gemini CLI UI patterns.

**Current patterns (3 total):**
- Ready: `(?:◇|✓).*(?:Ready|ready)` — matches "◇ Ready" or "✓ Ready"
- Processing: `(?:✦|⏲).*(?:Working|working)` — matches "✦ Working"
- NeedsApproval: `(?i)Yes, allow once` and `(?i)Allow execution of:`

**Missing states:** InputRequired, Error, Idle, Success.

**Research notes:** Since agy shares Gemini TUI codebase, patterns to add should mirror what Gemini uses. Known additional agy-specific patterns from live observation:
- The permission prompt also shows `Allow always` and `Reject` as option buttons (shared with opencode UX)
- agy shows user input prompts with readline-style indicators
- Error state likely shows error text with Unicode indicators (✗ or similar)

**Recommended additions (to be verified against live agy session output):**
- InputRequired: readline/stdin prompt when agy asks for user input (e.g., `>` prompt or similar ask-for-input pattern)
- Error: error indicators from the Gemini TUI (✗, "Error:", or similar)
- Idle: state when agy is waiting without any active prompt

---

## R4: Open Code Native Hooks

**Finding:** opencode v1.4.0 does **not** have a native pre/post-tool hook system.

Evidence:
1. `~/.config/opencode/opencode.json` contains only `"permission": {}` — no hooks field
2. `opencode.json` schema (from `$schema` reference https://opencode.ai/config.json) shows no hooks configuration
3. `opencode debug config` output and `opencode --help` show no hook-related flags
4. The `opencode run` subcommand has `--dangerously-skip-permissions` but no hook registration
5. The DCP plugin at `~/.config/opencode/dcp.jsonc` is a turn-protection layer, not a hooks API

**Conclusion:** The proxy wrapper approach in `installOpenCode()` is correct for v1.4.0. Document this explicitly so the decision is revisited when opencode adds native hooks.

The current wrapper at `~/.local/bin/open-code`:
```bash
#!/usr/bin/env bash
set -euo pipefail
CMD=$(ssq-hooks proxy -- open-code "$@")
eval "$CMD"
```

This intercepts the `open-code` binary name in PATH, but opencode's binary is named `opencode` (no hyphen). The wrapper targets `open-code` as a command alias — verify whether this is intentional or should target `opencode` instead.

---

## R5: Open Code Detection Patterns

**Finding:** `session/detection/binaries/opencode.go` has Processing, NeedsApproval, InputRequired but is missing Ready, Error, Idle, Success.

However, the existing tests in `session/detection/detector_opencode_test.go` reveal that opencode detection is handled via the **global** `NewStatusDetector()`, not just via `OpencodeDetector.Patterns()`. This means some patterns are already in the global detector layer.

**Existing patterns (from opencode.go and confirmed by tests):**
- Processing: `→\s*(Read|Write|Edit|Create|Delete)\b` (arrow-prefixed tool actions)
- NeedsApproval: `\[\s*Allow\s*\([aA]\)\s*\]` 
- InputRequired: `┃\s*\d+\.\s+\w` (bar-prefixed numbered options) and `Allow\s+once.*Allow\s+always|Allow\s+always.*Allow\s+once`

**Additional patterns observed in tests (already in global detector, need to confirm source):**
- Processing: `┃\s*Thinking:` (bar-prefixed thinking), `Thinking:` (plain thinking)
- Executing (Active): `esc interrupt`, `(esc to interrupt)`, `esc to interrupt`
- InputRequired: `Allow once.*Allow always` permission buttons

**Missing in OpencodeDetector:**
- Ready: opencode shows a readline/chat input prompt when idle and ready for input. Pattern likely `>\s*$` or similar prompt indicator. Needs live observation.
- Error: opencode shows errors inline in the TUI; pattern not yet identified.
- Success: no clear "done" state indicator identified yet.

---

## R6: Open Code One-Shot Client Validation

**Finding:** `opencode run [message..]` takes the message as **positional arguments**, not stdin.

The current `CLIAIClient` implementation delivers prompts via stdin (`WithStdin`). For opencode, `Args: func() []string { return []string{"run"} }` means the combined prompt is sent to stdin but opencode run may not read from stdin.

**Issue:** When `opencode run` is called with no positional message and no stdin piping support, it will likely launch interactively (or fail). The current implementation needs verification.

**Alternatives to investigate:**
1. Pass prompt as positional args: `opencode run "combined prompt"` — but prompt can be very long
2. Use `--command` flag: `opencode run --command "prompt"` — may have length limits
3. Confirm whether opencode run reads from stdin when no positional args given

**Current test result:** `opencode run` exits with config error (unrelated skill config issue), so behavior with stdin cannot be confirmed in this session. The `--format json` flag is available to get machine-readable output which could help parse responses.

---

## Summary of Key Facts

| Item | Value |
|------|-------|
| agy version | 1.0.14 |
| opencode version | 1.4.0 |
| agy one-shot flag | `--print` (shorthand: `-p`) |
| agy hook format | `hooks.json` with `{"namespace": {"PreToolUse": [{matcher, hooks:[{command,timeout,type}]}], enabled}}` |
| agy authoritative hooks path | `~/.gemini/config/hooks.json` |
| agy secondary hooks path | `~/.gemini/antigravity-cli/hooks.json` |
| opencode native hooks | None in v1.4.0 |
| opencode config location | `~/.config/opencode/opencode.json` |
| opencode one-shot | `opencode run [message]` (positional args, stdin unconfirmed) |
| opencode --format | `default` or `json` (useful for parsing) |
| opencode permission skip | `--dangerously-skip-permissions` flag |
