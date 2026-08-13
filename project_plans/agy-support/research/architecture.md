# Research: Architecture — Full agy Support

**Status**: Completed | **Phase**: 2 — Research  
**Created**: 2026-05-25

---

## 1. Existing Architecture Context

### `cmd/ssq-hooks/main.go` — Single Binary, Multiple Subcommands
The `ssq-hooks` binary is a multi-subcommand Go CLI:
```
ssq-hooks check   — stdin JSON → classify → stdout JSON
ssq-hooks serve   — HTTP classification server
ssq-hooks proxy   — transparent wrapper (open-code pattern)
ssq-hooks install — install binary + register hooks
ssq-hooks version — version info
```

All new work lives in this file. No new packages are required.

### `handleCheck()` — Current Flow
```
flag.Parse → json.Decode(stdin) → PermissionRequestPayload
→ AskUserQuestion guard → loadStorage → loadClassifier
→ c.BuildContext(cwd) → c.Classify(payload, ctx)
→ recordResult → writeHookDecision
```

### `handleInstall()` — Current Dispatch
```go
switch target {
case "claude":   installClaude()
case "gemini":   installGemini()   // currently prints instructions only
case "open-code": installOpenCode()
case "service":  installService()
}
```

---

## 2. Architectural Changes Required

### 2a. `handleInstall()` — Add `agy` Target
```go
case "agy":
    installAgy()
case "gemini":
    installGeminiAuto()  // replace stub with real implementation
```

Update `printUsage()` to list `agy` as a valid target.
Update `handleInstall()` usage string: `"Targets: claude, gemini, agy, open-code, service"`.

### 2b. Shared `patchBeforeToolHook()` Helper

Extract a new private function for patching Gemini-style settings (flat `hooks.BeforeTool` string):

```go
// patchBeforeToolHook patches settingsPath to set hooks.BeforeTool = hookCmd.
// Creates the file and parent directory if absent. Idempotent.
func patchBeforeToolHook(settingsPath, hookCmd string) error {
    raw, err := os.ReadFile(settingsPath)
    if err != nil {
        if !os.IsNotExist(err) { return err }
        raw = []byte("{}")
    }
    var settings map[string]interface{}
    if err := json.Unmarshal(raw, &settings); err != nil {
        return fmt.Errorf("parsing %s: %w", settingsPath, err)
    }
    hooks, _ := settings["hooks"].(map[string]interface{})
    if hooks == nil {
        hooks = map[string]interface{}{}
        settings["hooks"] = hooks
    }
    // Idempotency check
    if existing, _ := hooks["BeforeTool"].(string); existing == hookCmd {
        fmt.Println("Hook already present, nothing to do.")
        return nil
    }
    hooks["BeforeTool"] = hookCmd
    out, err := json.MarshalIndent(settings, "", "  ")
    if err != nil { return err }
    if err := os.MkdirAll(filepath.Dir(settingsPath), 0700); err != nil { return err }
    return os.WriteFile(settingsPath, append(out, '\n'), 0644)
}
```

Both `installAgy()` and upgraded `installGeminiAuto()` call this helper.

### 2c. `installAgy()` Structure
```
1. copyBinary(srcBin, ~/.local/bin/ssq-hooks)
2. settingsPath = ~/.gemini/antigravity-cli/settings.json
3. os.MkdirAll(~/.gemini/antigravity-cli/, 0700)
4. hookCmd = destBin + " check --gemini"
5. patchBeforeToolHook(settingsPath, hookCmd)
6. Print confirmation
```

### 2d. Upgraded `installGeminiAuto()` Structure
```
1. copyBinary(srcBin, ~/.local/bin/ssq-hooks)
2. configPath = first found among:
   - ~/.gemini/settings.json  (checked first — exists in live env)
   - ~/.gemini/config.json    (fallback)
   - ~/.gemini/antigravity-cli/settings.json (last resort, omit — agy-specific)
   If none found: create ~/.gemini/settings.json
3. hookCmd = destBin + " check --gemini"
4. patchBeforeToolHook(configPath, hookCmd)
5. Print confirmation
```

### 2e. `handleCheck()` — `--gemini` Flag

```go
func handleCheck() {
    checkCmd := flag.NewFlagSet("check", flag.ExitOnError)
    dbPath := checkCmd.String("db", getDefaultDBPath(), "...")
    geminiMode := checkCmd.Bool("gemini", false, "Translate Gemini/agy TOOL_INPUT payload")
    checkCmd.Parse(os.Args[2:])

    var payload classifier.PermissionRequestPayload
    if *geminiMode {
        payload = parseGeminiPayload()
    } else {
        if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil { ... }
    }
    // ... rest unchanged
}
```

```go
// parseGeminiPayload reads the Gemini/agy $TOOL_INPUT JSON from stdin
// and translates it to a PermissionRequestPayload.
// Falls back gracefully if schema is unrecognized.
func parseGeminiPayload() classifier.PermissionRequestPayload {
    // GeminiToolPayload covers observed schemas from open-source Gemini CLI
    type GeminiToolPayload struct {
        // Schema variant A: {"name": "run_shell_command", "args": {"command": "..."}}
        Name string                 `json:"name"`
        Args map[string]interface{} `json:"args"`
        // Schema variant B: flat {"tool_name": "...", "tool_input": {...}} (Claude-compatible)
        ToolName  string                 `json:"tool_name"`
        ToolInput map[string]interface{} `json:"tool_input"`
    }

    var raw GeminiToolPayload
    if err := json.NewDecoder(os.Stdin).Decode(&raw); err != nil {
        // Unrecognized input: return escalate-friendly empty payload
        return classifier.PermissionRequestPayload{ToolName: "Unknown"}
    }

    // Prefer Claude-compatible fields if present (future-proofing)
    if raw.ToolName != "" {
        return classifier.PermissionRequestPayload{
            ToolName:  raw.ToolName,
            ToolInput: raw.ToolInput,
        }
    }

    // Map Gemini schema A: name + args
    toolName := raw.Name
    toolInput := raw.Args
    // Normalize tool name: "run_shell_command" → "Bash" for classifier rules
    if strings.EqualFold(toolName, "run_shell_command") ||
       strings.EqualFold(toolName, "execute_bash") {
        toolName = "Bash"
    }

    return classifier.PermissionRequestPayload{
        ToolName:  toolName,
        ToolInput: toolInput,
    }
}
```

---

## 3. Integration Points

### 3a. `writeHookDecision()` and Gemini Exit Code Semantics
Current `writeHookDecision()` outputs Claude Code's `hookSpecificOutput` JSON format. For Gemini/agy `BeforeTool` hooks:
- **Allow**: exit 0, no stdout (or stdout ignored)
- **Deny**: exit non-zero (or specific stdout — TBD from Gemini docs)
- **Escalate**: exit 0 with no stdout → Gemini shows its own permission dialog

**Option A** (minimal change): Keep `writeHookDecision()` as-is; add a `--gemini` output mode that writes a plain-text denial reason or exits non-zero on deny.
**Option B** (safer): In `--gemini` mode, suppress Claude-format JSON on deny/allow and instead use exit codes only.

The safest approach for `--gemini` mode: exit 1 on deny (Gemini honors non-zero exit to block), exit 0 on allow/escalate. This matches the simpler Gemini hook contract.

### 3b. REQ-4: `detector.go` Comment

In `getDefaultPatterns()`, add a comment block near the `gemini_*` patterns:
```go
// NOTE: agy (Antigravity CLI) uses the same TUI codebase as Gemini CLI.
// The gemini_* patterns (gemini_ready, gemini_working, gemini_permission,
// gemini_allow_execution) cover agy sessions without additional patterns.
// If agy introduces divergent UI strings in a future version, add agy_* variants here.
```

---

## 4. Data Flow Diagram

```
agy session (tmux)
    │
    │ tool call → BeforeTool hook fires
    │
    ▼
$TOOL_INPUT (JSON env var)
    │
    │ printf '%s' "$TOOL_INPUT" | ssq-hooks check --gemini
    │
    ▼
parseGeminiPayload()
    → GeminiToolPayload{name: "run_shell_command", args: {command: "rm -rf /tmp/x"}}
    → PermissionRequestPayload{ToolName: "Bash", ToolInput: {"command": "rm -rf /tmp/x"}}
    │
    ▼
RuleBasedClassifier.Classify()
    → ClassificationResult{Decision: AutoAllow/Deny/Escalate}
    │
    ▼
--gemini exit code mode:
    AutoAllow  → exit 0 (agy continues)
    AutoDeny   → exit 1 (agy blocks tool) + stderr reason
    Escalate   → exit 0 (agy shows own dialog)
```

---

## Summary

- **Architecture**: All changes in `cmd/ssq-hooks/main.go`; extract `patchBeforeToolHook()` shared helper; add `parseGeminiPayload()` adapter; add `installAgy()` and upgrade `installGeminiAuto()`
- **Integration**: Gemini/agy hook uses exit codes (not Claude JSON format); `--gemini` mode needs its own output path in `writeHookDecision`-equivalent
- **Detector**: Comment-only change confirming agy is covered by `gemini_*` patterns
