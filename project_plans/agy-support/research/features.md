# Research: Features — Full agy Support

**Status**: Completed | **Phase**: 2 — Research  
**Created**: 2026-05-25

---

## 1. Existing Feature Inventory (What's Already Done)

### 1a. agy in GetAvailablePrograms (`config/config.go:680`)
`agy` is in the `candidates` slice alongside `proxy-claude`, `claude`, `claude-code`, `gemini`. The `GetAvailablePrograms()` function uses `exec.LookPath` to filter to installed binaries only. **Done** — no action needed.

### 1b. agy in recursiveEvalPrograms (`pkg/classifier/command_parser.go:428-431`)
```go
"agy": {
    flagArgs:           map[string]bool{},
    passthroughSubcmds: map[string]bool{"proxy": true},
},
```
This mirrors the `rtk` entry verbatim. **Done** — means `agy proxy <cmd>` is unwrapped before classification.

### 1c. Frontend label/emoji
- `programs.ts:15`: `{ value: "agy", label: "Antigravity", description: "Antigravity CLI (agy)" }`
- `SessionRow.tsx:81`: `◆` emoji for `agy`/`antigravity`
**Done** — UI shows correctly.

### 1d. Dynamic program dropdowns
Frontend dropdowns fetch from `/api/server-info`, which calls `GetAvailablePrograms()`. `agy` appears only when the binary is installed. **Done**.

---

## 2. Features Required (In-Scope Work)

### REQ-1: `ssq-hooks install agy`

**New function `installAgy()`** following the `installClaude()` pattern:

1. `copyBinary(srcBin, "~/.local/bin/ssq-hooks")`
2. `os.MkdirAll("~/.gemini/antigravity-cli/", 0700)`
3. `patchAgySettings("~/.gemini/antigravity-cli/settings.json", "ssq-hooks check --gemini")`
4. Print confirmation message

The `patchAgySettings` function:
- Top-level `hooks.BeforeTool` string (not array)
- Idempotency check: `existing["hooks"]["BeforeTool"] == hookCmd`
- Creates file if absent with `{}` base
- `os.MkdirAll(filepath.Dir(settingsPath), 0700)` for the directory

**Differences from Claude patching**:
- Claude: `hooks.PreToolUse` is an **array of objects** with matcher+hooks sub-structure
- Gemini/agy: `hooks.BeforeTool` is a **plain string** — simpler

### REQ-2: `ssq-hooks install gemini` — upgrade from manual to automated

Current `installGemini()` only prints instructions. Upgrade:

1. Same `copyBinary` step
2. Try `~/.gemini/settings.json` first (exists in live env), fall back to `~/.gemini/config.json`
3. `patchGeminiSettings(foundPath, "ssq-hooks check --gemini")` — same string-valued `BeforeTool` patch
4. Create file + dir if absent
5. Print confirmation

**Reuse opportunity**: `patchAgySettings` and `patchGeminiSettings` are identical except for the config file path. Extract a shared `patchBeforeToolHook(settingsPath, hookCmd string) error` helper.

### REQ-3: `ssq-hooks check --gemini`

**New flag in `handleCheck()`**:
```go
geminiMode := checkCmd.Bool("gemini", false, "Translate Gemini/agy TOOL_INPUT payload")
```

When `--gemini` is set:
- Decode stdin as `GeminiToolPayload` (new struct, see architecture.md)
- Map to `classifier.PermissionRequestPayload`
- Continue with existing classification + `writeHookDecision()`

**Gemini/agy `$TOOL_INPUT` schema** (must be confirmed — see pitfalls.md):
- Best estimate from open-source Gemini CLI codebase: `{"name": "run_shell_command", "args": {"command": "...", "description": "..."}}`
- Alternative field: `{"tool_name": "...", "tool_input": {...}}` (Claude-style, may already work)

Fallback on parse error: escalate (do not crash — exit 0 with escalate decision per existing `Escalate` path).

**AskUserQuestion equivalent in Gemini**: Gemini uses `ask_for_user_input` or similar tool name — add guard same as Claude's `AskUserQuestion` check.

### REQ-4: Detection patterns for `agy`

**Assessment**: agy uses the same TUI as Gemini CLI (confirmed in requirements: "same TUI codebase"). Existing patterns in `detector.go`:
- `gemini_ready`: `(?:◇|✓).*(?:Ready|ready)` — covers agy
- `gemini_working`: `(?:✦|⏲).*(?:Working|working)` — covers agy
- `gemini_permission`: `(?i)Yes, allow once` — covers agy
- `gemini_allow_execution`: `(?i)Allow execution of:` — covers agy

**Action**: Add a comment block in `getDefaultPatterns()` stating that `agy` shares Gemini TUI patterns. No new regex needed unless agy branding changes the permission dialog strings.

---

## 3. User-Facing Edge Cases

### Install with no settings.json yet
Both `patchAgySettings` and `patchGeminiSettings` must handle the "file doesn't exist" path by creating `{}` base. Existing `patchClaudeSettings` already does this.

### agy not installed
`ssq-hooks install agy` should still work even if `agy` binary isn't in `PATH` — it only installs the hook into settings.json, not the `agy` binary itself. The binary copy step is for `ssq-hooks`, not `agy`.

### Multiple Gemini config files
If both `~/.gemini/settings.json` and `~/.gemini/config.json` exist, patch only the first found. Print which file was patched.

### Re-running install (idempotency)
If `hooks.BeforeTool` already equals the target hook command, print "Hook already present, nothing to do." and return without modifying the file (same UX as `patchClaudeSettings`).

### Hook command with full path
If `ssq-hooks` is not in PATH, the hook command `ssq-hooks check --gemini` would fail at runtime. The install function should use the full path `destBin` (e.g., `~/.local/bin/ssq-hooks check --gemini`) in the hook string, consistent with `installClaude()` behavior (`hookCmd := binPath + " check"`). For Gemini mode, `hookCmd := binPath + " check --gemini"`.

---

## Summary

- **Key reuse**: `patchClaudeSettings()` pattern covers 80% of the new patching logic; extract `patchBeforeToolHook()` shared helper for Gemini/agy (simpler — string not array)
- **REQ-3 blocker**: Gemini `$TOOL_INPUT` schema must be confirmed before writing the `--gemini` adapter; graceful fallback (escalate) handles the unknown-schema case
- **REQ-4**: No new patterns needed; add a comment in `detector.go` confirming agy coverage
