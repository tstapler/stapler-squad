# Research: Pitfalls — Full agy Support

**Status**: Completed | **Phase**: 2 — Research  
**Created**: 2026-05-25

---

## PITFALL-1: Unknown Gemini/agy `$TOOL_INPUT` Schema (CRITICAL)

### Risk
The exact JSON structure of `$TOOL_INPUT` in Gemini/agy `BeforeTool` hooks is **not confirmed** in this codebase. The requirements note it must be discovered from either live capture or the open-source codebase.

### Known candidates (from code archaeology)

**Variant A** (most likely — from Gemini CLI open-source):
```json
{"name": "run_shell_command", "args": {"command": "ls -la", "description": "List files"}}
```

**Variant B** (speculative — Claude-compatible fields):
```json
{"tool_name": "Bash", "tool_input": {"command": "ls -la"}}
```

**Variant C** (observed in the existing hook script `install-gemini-hook.sh`):
```bash
echo "$TOOL_INPUT" | ssq-hooks check ...
```
Note: `echo` vs `printf '%s'` matters if the payload contains backslashes or `-e` flags.

### Mitigation
1. **Graceful fallback**: If JSON decode fails or neither expected field is present, return `PermissionRequestPayload{ToolName: "Unknown"}` which classifies as `Escalate` (not crash, not false-allow).
2. **`printf '%s'`** in the hook command (not `echo`) avoids backslash interpretation.
3. **Add stderr logging** in `--gemini` mode that prints the raw received payload when `STAPLER_DEBUG=1` is set — enables field-capture during first real run.
4. **Try live capture** before implementation: `agy` is installed at `/home/tstapler/.local/bin/agy`. Create a minimal test hook `printf '%s' "$TOOL_INPUT" > /tmp/agy-payload.json` to capture a real payload.

---

## PITFALL-2: JSON Patching Destroys Non-String `hooks.BeforeTool` (MEDIUM)

### Risk
`patchBeforeToolHook()` will fail or produce incorrect output if `settings["hooks"]["BeforeTool"]` is already present as a **non-string** (e.g., array, object). In agy's case this seems unlikely (BeforeTool is specified as a string), but in Gemini's case the field could be anything since the settings file is user-managed.

### Mitigation
```go
if existing, ok := hooks["BeforeTool"]; ok {
    if _, ok := existing.(string); !ok {
        return fmt.Errorf("parsing %s: hooks.\"BeforeTool\" is not a string (found %T); cannot patch", settingsPath, existing)
    }
}
```
Return a clear error; do not silently overwrite.

---

## PITFALL-3: JSON Pretty-Print Destroys Existing File Formatting (LOW)

### Risk
`json.MarshalIndent(settings, "", "  ")` re-serializes the entire JSON with 2-space indentation, potentially changing:
- Existing indentation (tabs → spaces, 4-space → 2-space)
- Key ordering (Go maps have no ordering; keys may reorder)
- Trailing whitespace, blank lines

This is an existing issue in `patchClaudeSettings()` too — accepted as a known trade-off.

### Mitigation
Document the behavior in code comments. For agy's `settings.json` the impact is minimal (small file, machine-generated). For Gemini's hand-crafted `settings.json` it is more visible but not harmful.

---

## PITFALL-4: Race Between `os.WriteFile` and Running agy Process (LOW)

### Risk
If the user runs `ssq-hooks install agy` while an `agy` session is active, `agy` may be reading `settings.json` at the same moment as `os.WriteFile` patches it. This can produce a partial read.

### Mitigation
The existing `patchClaudeSettings` uses atomic rename: write to `settingsPath + ".tmp"` then `os.Rename`. Implement the same for `patchBeforeToolHook`:
```go
tmpPath := settingsPath + ".tmp"
os.WriteFile(tmpPath, data, 0644)
os.Rename(tmpPath, settingsPath)  // atomic on same filesystem
```

---

## PITFALL-5: Gemini Settings File Has Multiple Candidate Paths — Wrong One Patched (MEDIUM)

### Risk
`~/.gemini/settings.json` (observed live) and `~/.gemini/config.json` (referenced in old scripts) both exist in some environments. Patching both would set duplicate `BeforeTool` hooks; agy/Gemini would apply both (undefined behavior).

Live observation shows `~/.gemini/settings.json` is the authoritative file for Gemini CLI. `~/.gemini/config.json` does not currently exist in the dev environment.

### Mitigation
- Patch **only the first found** file in priority order: `settings.json` → `config.json`
- Print clearly which file was patched
- Check both files for existing hook before patching (to avoid duplicates if user has manually patched one)

---

## PITFALL-6: `--gemini` Hook Output Format Incompatibility (MEDIUM)

### Risk
`writeHookDecision()` outputs Claude Code's `hookSpecificOutput` JSON format. Gemini/agy `BeforeTool` hooks may interpret this JSON as a denial reason (if they parse stdout at all), or may ignore it entirely. If Gemini interprets non-empty stdout as a block signal, then `AutoAllow` decisions would incorrectly block every tool.

### Known Gemini BeforeTool contract (from codebase research)
The `install-gemini-hook.sh` script says "echo stdin | ssq-hooks check" without capturing stdout — suggesting Gemini uses **exit code only**: 0 = allow, non-zero = block.

### Mitigation
For `--gemini` mode, implement a separate output function:
```go
func writeGeminiHookDecision(result classifier.ClassificationResult) {
    switch result.Decision {
    case classifier.AutoDeny:
        fmt.Fprintf(os.Stderr, "SSQ-Hooks: Blocked by rule %s: %s\n", result.RuleID, result.Reason)
        os.Exit(1)  // non-zero = Gemini blocks tool
    default:
        // AutoAllow or Escalate: exit 0, no stdout
        // Escalate: let Gemini show its own dialog
    }
}
```
**Verify exit code semantics** against live `agy` behavior before finalizing.

---

## PITFALL-7: `AskUserQuestion` Equivalent in Gemini/agy (LOW)

### Risk
`handleCheck()` has a guard: if `ToolName == "AskUserQuestion"`, exit 0 immediately (don't classify, let Claude Code handle it). Gemini has an equivalent "ask user a question" tool (`ask_for_user_input` or similar). Without a similar guard, these would classify as `Escalate` (harmless but noisy).

### Mitigation
In `parseGeminiPayload()`, after mapping to `PermissionRequestPayload`, check if `toolName` matches known Gemini user-input tools and exit 0 immediately:
```go
if strings.EqualFold(payload.ToolName, "ask_for_user_input") ||
   strings.EqualFold(payload.ToolName, "AskUserQuestion") {
    os.Exit(0)
}
```
The exact Gemini tool name must be confirmed from live capture or source.

---

## PITFALL-8: `agy` Settings Schema May Evolve Pre-GA (MEDIUM)

### Risk
`agy` is Google's pre-GA successor to Gemini CLI (shutting down June 18, 2026). Its config schema may still be changing. The current `~/.gemini/antigravity-cli/settings.json` already uses a simpler schema than `~/.gemini/settings.json` — but a future agy update could add a `hooks` key with a different format (array, not string).

### Mitigation
- Use the same `patchBeforeToolHook` non-string guard (PITFALL-2)
- Version-stamp the install: print agy binary version at install time for debugging
- Design `parseGeminiPayload()` with a multi-variant fallback (try schema A, then B, then graceful escalate)

---

## PITFALL-9: Idempotency on Already-Patched Files With Different Binary Path (LOW)

### Risk
If the user previously ran `ssq-hooks install agy` from a different build location (e.g., `/tmp/ssq-hooks`), the installed hook command is `"/tmp/ssq-hooks check --gemini"`. Re-running from the installed location (`~/.local/bin/ssq-hooks`) produces a different `hookCmd` string → idempotency check fails → two `BeforeTool` entries if we append instead of replace.

Since `BeforeTool` is a **string** (not array), this is handled correctly: the second install simply **overwrites** the string value. Unlike Claude's array (where we check for duplicates), the string overwrite is idempotent by construction. No action needed — just confirm `patchBeforeToolHook` overwrites the existing string unconditionally (after the "already present" check uses exact equality).

---

## Summary of Risk Levels

| Pitfall | Severity | Action |
|---------|----------|--------|
| P-1: Unknown $TOOL_INPUT schema | CRITICAL | Capture live payload before coding; graceful fallback always |
| P-3: JSON key reorder | LOW | Document; accepted trade-off |
| P-4: Write race | LOW | Atomic rename (temp file) |
| P-5: Wrong config file patched | MEDIUM | First-found priority; print which file |
| P-6: Hook output format incompatibility | MEDIUM | Verify Gemini exit code contract; use exit-only output |
| P-2: Non-string BeforeTool | MEDIUM | Type-guard before patching |
| P-7: AskUserQuestion equivalent | LOW | Add guard after schema mapping |
| P-8: agy schema evolution | MEDIUM | Multi-variant parse; graceful fallback |
| P-9: Path-change idempotency | LOW | String overwrite handles automatically |

**Top 3 to address first**: P-1 (confirm payload schema via live capture), P-6 (verify Gemini exit code contract), P-5 (config file discovery order).
