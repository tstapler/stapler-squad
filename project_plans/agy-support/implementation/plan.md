# Implementation Plan: agy-support

**Feature**: Full Antigravity CLI (agy) and automated Gemini CLI hook support in ssq-hooks
**Date**: 2026-05-25
**Status**: Ready for implementation
**ADRs**: ADR-001-gemini-exit-code-contract.md

---

## Dependency Visualization

```
Epic 1.1 (patchBeforeToolHook helper)
    │
    ├──► Epic 1.2 (installAgy)
    │
    ├──► Epic 1.3 (installGemini auto-upgrade)
    │
    └──► Epic 2.1 (parseGeminiPayload + --gemini flag)
              │
              └──► Epic 2.2 (writeGeminiHookDecision)
                        │
                        └──► Epic 3.1 (detector.go comment)
                                  │
                                  └──► Epic 4.1 (usage + smoke tests)
```

---

## Phase 1: Hook Installation Infrastructure

### Epic 1.1: `patchBeforeToolHook()` Shared Helper
**Goal**: Extract a reusable, idempotent JSON-patcher for Gemini-style settings files (flat `hooks.BeforeTool` string). Both `installAgy` and upgraded `installGemini` depend on this helper.

#### Story 1.1.1: Implement patchBeforeToolHook
**As a** developer installing ssq-hooks for Gemini-family CLIs, **I want** a single function that safely patches any JSON settings file with a BeforeTool hook string, **so that** both agy and Gemini installs share one tested code path.

**Acceptance Criteria**:
- Creates the file (with `{}` base) and parent directories if absent
- Sets `settings["hooks"]["BeforeTool"]` to the provided hook command string
- Idempotent: prints "Hook already present, nothing to do." and returns nil if the exact string already exists
- Returns a descriptive error (does NOT silently overwrite) if `hooks.BeforeTool` exists but is a non-string type (P-2 mitigation)
- Uses atomic write: write to `settingsPath + ".tmp"` then `os.Rename` (P-4 mitigation)
- Preserves all existing keys in the JSON object (only adds/updates `hooks.BeforeTool`)

**Files**: `cmd/ssq-hooks/main.go`

##### Task 1.1.1a: Write `patchBeforeToolHook(settingsPath, hookCmd string) error` (~3 min)
- Add function after `patchClaudeSettings()` (around line 554)
- Implementation:
  ```go
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
      // Guard: existing BeforeTool must be a string, not an array/object
      if existing, ok := hooks["BeforeTool"]; ok {
          if _, ok := existing.(string); !ok {
              return fmt.Errorf("parsing %s: hooks.\"BeforeTool\" is not a string (found %T); cannot patch", settingsPath, existing)
          }
          if existing.(string) == hookCmd {
              fmt.Println("Hook already present, nothing to do.")
              return nil
          }
      }
      hooks["BeforeTool"] = hookCmd
      out, err := json.MarshalIndent(settings, "", "  ")
      if err != nil { return err }
      if err := os.MkdirAll(filepath.Dir(settingsPath), 0700); err != nil { return err }
      // Atomic write (P-4: avoid partial-read race with running agy process)
      tmpPath := settingsPath + ".tmp"
      if err := os.WriteFile(tmpPath, append(out, '\n'), 0644); err != nil { return err }
      return os.Rename(tmpPath, settingsPath)
  }
  ```
- Files: `cmd/ssq-hooks/main.go`

---

### Epic 1.2: `ssq-hooks install agy`
**Goal**: New install target that fully auto-installs the BeforeTool hook into `~/.gemini/antigravity-cli/settings.json`.

#### Story 1.2.1: Implement installAgy()
**As a** user running `ssq-hooks install agy`, **I want** the hook automatically written to my agy settings file, **so that** every agy tool call is classified without manual JSON editing.

**Acceptance Criteria**:
- Copies current binary to `~/.local/bin/ssq-hooks`
- Creates `~/.gemini/antigravity-cli/` directory if absent
- Patches `~/.gemini/antigravity-cli/settings.json` with `BeforeTool` = `"<destBin> check --gemini"`
- Idempotent: re-running is safe
- Prints: `Installed binary: <path>` and `Updated hook: <settings-path>` and `Done. Restart agy for the hook to take effect.`
- Works even when `agy` binary is not in PATH (we only install the hook, not the agy binary)

**Files**: `cmd/ssq-hooks/main.go`

##### Task 1.2.1a: Write `installAgy()` function (~3 min)
- Add after `installGemini()` (around line 584)
- Implementation:
  ```go
  func installAgy() {
      home, err := os.UserHomeDir()
      if err != nil {
          fmt.Fprintf(os.Stderr, "Error resolving home directory: %v\n", err)
          os.Exit(1)
      }
      // 1. Copy binary to ~/.local/bin/ssq-hooks
      binDir := filepath.Join(home, ".local", "bin")
      if err := os.MkdirAll(binDir, 0755); err != nil {
          fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", binDir, err)
          os.Exit(1)
      }
      destBin := filepath.Join(binDir, "ssq-hooks")
      srcBin, err := os.Executable()
      if err != nil {
          fmt.Fprintf(os.Stderr, "Error resolving current binary: %v\n", err)
          os.Exit(1)
      }
      if resolved, err := filepath.EvalSymlinks(srcBin); err == nil {
          srcBin = resolved
      }
      if err := copyBinary(srcBin, destBin); err != nil {
          fmt.Fprintf(os.Stderr, "Error copying binary to %s: %v\n", destBin, err)
          os.Exit(1)
      }
      fmt.Printf("Installed binary: %s\n", destBin)
      // 2. Patch ~/.gemini/antigravity-cli/settings.json
      settingsPath := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
      hookCmd := destBin + " check --gemini"
      if err := patchBeforeToolHook(settingsPath, hookCmd); err != nil {
          fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", settingsPath, err)
          os.Exit(1)
      }
      fmt.Printf("Updated hook:     %s\n", settingsPath)
      fmt.Println("Done. Restart agy for the hook to take effect.")
  }
  ```
- Files: `cmd/ssq-hooks/main.go`

##### Task 1.2.1b: Wire `agy` into `handleInstall()` switch and `printUsage()` (~2 min)
- In `handleInstall()` (line 394), add case before `default`:
  ```go
  case "agy":
      installAgy()
  ```
- In `handleInstall()` usage string (line 389):
  Change: `"Targets: claude, gemini, open-code, service"`
  To: `"Targets: claude, gemini, agy, open-code, service"`
- In `printUsage()` (line 55):
  Change: `"install - Install binary and register hooks (targets: claude, gemini, open-code, service)"`
  To: `"install - Install binary and register hooks (targets: claude, gemini, agy, open-code, service)"`
- Files: `cmd/ssq-hooks/main.go`

---

### Epic 1.3: `ssq-hooks install gemini` — Upgrade from Manual to Automated
**Goal**: Replace the current stub `installGemini()` (prints instructions only) with a fully automated install that mirrors `installClaude()` automation level.

#### Story 1.3.1: Upgrade installGemini() to auto-patch
**As a** user running `ssq-hooks install gemini`, **I want** the hook written automatically (not just printed as instructions), **so that** Gemini CLI setup is one command.

**Acceptance Criteria**:
- Copies current binary to `~/.local/bin/ssq-hooks`
- Discovers the Gemini settings file using priority order: `~/.gemini/settings.json` → `~/.gemini/config.json` → creates `~/.gemini/settings.json` if neither found
- Patches the first found file only (P-5 mitigation: never patches both)
- Prints which file was patched
- Idempotent
- Hook command string uses full destBin path: `"<destBin> check --gemini"`

**Files**: `cmd/ssq-hooks/main.go`

##### Task 1.3.1a: Replace installGemini() body with automated implementation (~4 min)
- Replace the entire `installGemini()` function body (lines 556–584) with:
  ```go
  func installGemini() {
      home, err := os.UserHomeDir()
      if err != nil {
          fmt.Fprintf(os.Stderr, "Error resolving home directory: %v\n", err)
          os.Exit(1)
      }
      // 1. Copy binary to ~/.local/bin/ssq-hooks
      binDir := filepath.Join(home, ".local", "bin")
      if err := os.MkdirAll(binDir, 0755); err != nil {
          fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", binDir, err)
          os.Exit(1)
      }
      destBin := filepath.Join(binDir, "ssq-hooks")
      srcBin, err := os.Executable()
      if err != nil {
          fmt.Fprintf(os.Stderr, "Error resolving current binary: %v\n", err)
          os.Exit(1)
      }
      if resolved, err := filepath.EvalSymlinks(srcBin); err == nil {
          srcBin = resolved
      }
      if err := copyBinary(srcBin, destBin); err != nil {
          fmt.Fprintf(os.Stderr, "Error copying binary to %s: %v\n", destBin, err)
          os.Exit(1)
      }
      fmt.Printf("Installed binary: %s\n", destBin)
      // 2. Discover Gemini settings file (P-5: patch only the first found)
      candidates := []string{
          filepath.Join(home, ".gemini", "settings.json"),  // authoritative (observed live)
          filepath.Join(home, ".gemini", "config.json"),    // legacy fallback
      }
      settingsPath := ""
      for _, c := range candidates {
          if _, err := os.Stat(c); err == nil {
              settingsPath = c
              break
          }
      }
      if settingsPath == "" {
          // Neither found: create ~/.gemini/settings.json
          settingsPath = candidates[0]
      }
      // 3. Patch the selected file
      hookCmd := destBin + " check --gemini"
      if err := patchBeforeToolHook(settingsPath, hookCmd); err != nil {
          fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", settingsPath, err)
          os.Exit(1)
      }
      fmt.Printf("Updated hook:     %s\n", settingsPath)
      fmt.Println("Done. Restart Gemini CLI for the hook to take effect.")
  }
  ```
- Files: `cmd/ssq-hooks/main.go`

---

## Phase 2: Gemini/agy Payload Adapter

### Epic 2.1: `parseGeminiPayload()` — Multi-Variant Fallback Parser
**Goal**: Translate the Gemini/agy `$TOOL_INPUT` JSON (schema unconfirmed — P-1 risk) into `PermissionRequestPayload`. Must handle at least 2 candidate schemas and escalate gracefully on unknown formats.

#### Story 2.1.1: Implement multi-variant GeminiToolPayload decoder
**As a** ssq-hooks check process invoked by a Gemini/agy BeforeTool hook, **I want** the payload decoded from either known schema variant, **so that** classification rules run correctly regardless of which schema agy uses.

**Acceptance Criteria**:
- Handles **Variant A** (most likely from open-source Gemini CLI): `{"name": "run_shell_command", "args": {"command": "..."}}`
- Handles **Variant B** (Claude-compatible passthrough): `{"tool_name": "...", "tool_input": {...}}`
- On JSON decode failure or unrecognized schema: returns `PermissionRequestPayload{ToolName: "Unknown"}` — classifies as Escalate (not crash, not false-allow)
- Normalizes Gemini tool names to classifier-compatible names: `"run_shell_command"` or `"execute_bash"` → `"Bash"`
- When `STAPLER_DEBUG=1` env var is set: writes raw received payload to stderr for field-capture debugging (P-1 mitigation)
- Handles the `ask_for_user_input` Gemini guard (P-7): exits 0 immediately for user-input tools

**Files**: `cmd/ssq-hooks/main.go`

##### Task 2.1.1a: Write `parseGeminiPayload()` function (~4 min)
- Add after `writeHookDecision()` (around line 132)
- The `GeminiToolPayload` struct handles both variants via Go's zero-value behavior:
  ```go
  // parseGeminiPayload reads the Gemini/agy $TOOL_INPUT JSON from stdin
  // and translates it to a PermissionRequestPayload.
  // 
  // Supported schemas:
  //   Variant A (Gemini CLI open-source): {"name": "run_shell_command", "args": {"command": "..."}}
  //   Variant B (Claude-compatible):      {"tool_name": "Bash", "tool_input": {"command": "..."}}
  //
  // Falls back gracefully to PermissionRequestPayload{ToolName: "Unknown"} on any
  // parse error or unrecognized schema — results in Escalate (not crash, not false-allow).
  func parseGeminiPayload() classifier.PermissionRequestPayload {
      raw, err := io.ReadAll(os.Stdin)
      if err != nil {
          fmt.Fprintf(os.Stderr, "SSQ-Hooks: error reading stdin: %v\n", err)
          return classifier.PermissionRequestPayload{ToolName: "Unknown"}
      }
      // Debug: dump raw payload when STAPLER_DEBUG=1 (P-1: field capture on first real run)
      if os.Getenv("STAPLER_DEBUG") == "1" {
          fmt.Fprintf(os.Stderr, "SSQ-Hooks [debug] raw $TOOL_INPUT: %s\n", string(raw))
      }
      // GeminiToolPayload covers both known schema variants.
      // Zero values for absent fields allow detecting which variant is present.
      type GeminiToolPayload struct {
          // Variant A: {"name": "run_shell_command", "args": {"command": "..."}}
          Name string                 `json:"name"`
          Args map[string]interface{} `json:"args"`
          // Variant B: {"tool_name": "...", "tool_input": {...}}
          ToolName  string                 `json:"tool_name"`
          ToolInput map[string]interface{} `json:"tool_input"`
      }
      var p GeminiToolPayload
      if err := json.Unmarshal(raw, &p); err != nil {
          fmt.Fprintf(os.Stderr, "SSQ-Hooks: failed to parse Gemini payload: %v\n", err)
          return classifier.PermissionRequestPayload{ToolName: "Unknown"}
      }
      // Prefer Variant B (Claude-compatible) if present
      if p.ToolName != "" {
          payload := classifier.PermissionRequestPayload{
              ToolName:  p.ToolName,
              ToolInput: p.ToolInput,
          }
          // P-7: pass-through user-input tool (equivalent of AskUserQuestion guard)
          if strings.EqualFold(payload.ToolName, "ask_for_user_input") {
              os.Exit(0)
          }
          return payload
      }
      // Fall back to Variant A: name + args
      if p.Name == "" {
          // Neither variant matched: unknown schema
          fmt.Fprintf(os.Stderr, "SSQ-Hooks: unrecognized Gemini payload schema (no 'name' or 'tool_name' field)\n")
          return classifier.PermissionRequestPayload{ToolName: "Unknown"}
      }
      toolName := p.Name
      // Normalize Gemini tool names to classifier-expected names
      switch strings.ToLower(toolName) {
      case "run_shell_command", "execute_bash", "run_bash_command":
          toolName = "Bash"
      case "read_file", "read_many_files":
          toolName = "Read"
      case "write_file":
          toolName = "Write"
      case "ask_for_user_input":
          // P-7: pass-through user-input tool (equivalent of AskUserQuestion guard)
          os.Exit(0)
      }
      return classifier.PermissionRequestPayload{
          ToolName:  toolName,
          ToolInput: p.Args,
      }
  }
  ```
- Note: requires adding `"io"` to the import block
- Files: `cmd/ssq-hooks/main.go`

---

### Epic 2.2: `--gemini` Flag in `handleCheck()` + Gemini Exit Code Output
**Goal**: Wire the `--gemini` flag into `handleCheck()` and implement a separate `writeGeminiHookDecision()` that uses exit codes instead of Claude's JSON stdout format.

#### Story 2.2.1: Add --gemini flag and exit-code output path
**As a** agy/Gemini BeforeTool hook, **I want** ssq-hooks to communicate decisions via exit codes (0=allow, 1=deny), **so that** agy honors the block decision without needing to parse JSON stdout.

**Acceptance Criteria**:
- `handleCheck()` parses a new `--gemini` bool flag
- When `--gemini` is set: calls `parseGeminiPayload()` instead of `json.NewDecoder(os.Stdin).Decode`
- When `--gemini` is set: calls `writeGeminiHookDecision()` instead of `writeHookDecision()`
- `writeGeminiHookDecision()`: AutoDeny → writes reason to stderr + `os.Exit(1)`; AutoAllow → exit 0 (no stdout); Escalate → exit 0 (no stdout, agy shows its own dialog)
- Existing `ssq-hooks check` (no flag) is completely unchanged — Claude-compatible path untouched
- The `--gemini` cwd: use `os.Getwd()` as fallback since Gemini payload may not include cwd

**Files**: `cmd/ssq-hooks/main.go`

##### Task 2.2.1a: Add `--gemini` flag to `handleCheck()` (~3 min)
- Modify `handleCheck()` (lines 59–88):
  ```go
  func handleCheck() {
      checkCmd := flag.NewFlagSet("check", flag.ExitOnError)
      dbPath := checkCmd.String("db", getDefaultDBPath(), "Path to SQLite database")
      geminiMode := checkCmd.Bool("gemini", false, "Translate Gemini/agy TOOL_INPUT payload (exit-code output)")
      checkCmd.Parse(os.Args[2:])

      var payload classifier.PermissionRequestPayload
      if *geminiMode {
          payload = parseGeminiPayload()
          // Gemini payload typically lacks cwd; fall back to process working directory
          if payload.Cwd == "" {
              payload.Cwd, _ = os.Getwd()
          }
      } else {
          if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
              fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
              os.Exit(1)
          }
          // AskUserQuestion guard (Claude-only path)
          if strings.EqualFold(payload.ToolName, "AskUserQuestion") {
              os.Exit(0)
          }
      }

      storage := loadStorage(*dbPath)
      defer storage.Close()

      c := loadClassifier(storage)
      ctx := c.BuildContext(payload.Cwd)
      result := c.Classify(payload, ctx)

      recordResult(storage, payload, result, 0)

      if *geminiMode {
          writeGeminiHookDecision(result)
      } else {
          writeHookDecision(result)
      }
  }
  ```
- Files: `cmd/ssq-hooks/main.go`

##### Task 2.2.1b: Write `writeGeminiHookDecision()` function (~2 min)
- Add after `writeHookDecision()` (around line 132):
  ```go
  // writeGeminiHookDecision communicates the classification result to Gemini/agy via exit codes.
  //
  // Gemini/agy BeforeTool hook contract (confirmed from install-gemini-hook.sh + architecture.md):
  //   exit 0  — allow the tool call (also used for Escalate: agy shows its own permission dialog)
  //   exit 1  — deny the tool call; agy blocks execution
  //
  // Denial reason is written to stderr (not stdout) to avoid being misinterpreted as
  // a blocking output signal if Gemini ever inspects stdout.
  func writeGeminiHookDecision(result classifier.ClassificationResult) {
      switch result.Decision {
      case classifier.AutoDeny:
          reason := result.Reason
          if result.Alternative != "" {
              reason += " " + result.Alternative
          }
          ruleInfo := ""
          if result.RuleID != "" {
              ruleInfo = fmt.Sprintf(" [rule: %s]", result.RuleID)
          }
          fmt.Fprintf(os.Stderr, "SSQ-Hooks: blocked%s — %s\n", ruleInfo, reason)
          os.Exit(1)
      default:
          // AutoAllow or Escalate: exit 0.
          // Escalate: empty stdout/stderr — agy shows its own permission dialog.
      }
  }
  ```
- Files: `cmd/ssq-hooks/main.go`

---

## Phase 3: Detector Coverage Confirmation

### Epic 3.1: REQ-4 — Confirm agy Coverage in detector.go
**Goal**: Add a code comment to `session/detection/detector.go` confirming that existing `gemini_*` patterns cover agy sessions, closing REQ-4 with documentation rather than new regex.

#### Story 3.1.1: Add agy coverage comment to getDefaultPatterns()
**As a** future maintainer reading detector.go, **I want** a clear comment explaining why there are no `agy_*` patterns, **so that** I don't add redundant patterns assuming the coverage gap was an oversight.

**Acceptance Criteria**:
- Comment added near the `gemini_ready` pattern in `getDefaultPatterns()` (line ~392 of `session/detection/detector.go`)
- Comment covers all four `gemini_*` patterns and explains the shared TUI basis
- Comment includes a note about what to do if agy diverges in a future version

**Files**: `session/detection/detector.go`

##### Task 3.1.1a: Add agy-coverage comment block to detector.go (~2 min)
- In `getDefaultPatterns()`, immediately before the `gemini_ready` pattern (line 392), add:
  ```go
  // NOTE: agy (Antigravity CLI) — agy uses the same TUI codebase as Gemini CLI
  // (requirements confirmed: "same TUI codebase, rewritten core in Go"). The four
  // gemini_* patterns below (gemini_ready, gemini_working, gemini_permission,
  // gemini_allow_execution) cover agy sessions without additional patterns.
  //
  // If agy introduces divergent UI strings (e.g. rebranded permission dialog text)
  // in a future version, add agy_* pattern variants alongside the gemini_* entries
  // at the same priority levels.
  ```
- Files: `session/detection/detector.go`

---

## Phase 4: Polish, Verification, and Integration

### Epic 4.1: Build Verification and Manual Smoke Test
**Goal**: Confirm the full implementation compiles, passes lint, and works end-to-end.

#### Story 4.1.1: Build and lint verification
**As a** developer shipping this feature, **I want** the build and linter to pass before marking the PR ready, **so that** CI does not reveal avoidable failures.

**Acceptance Criteria**:
- `make build` passes
- `make lint` passes (no new warnings)
- `go vet ./cmd/ssq-hooks/...` clean

**Files**: `cmd/ssq-hooks/main.go`

##### Task 4.1.1a: Add `"io"` to import block (~1 min)
- `parseGeminiPayload()` uses `io.ReadAll` — add `"io"` to the import block in `cmd/ssq-hooks/main.go`
- Files: `cmd/ssq-hooks/main.go`

##### Task 4.1.1b: Run `make build && make lint` (~2 min)
- Fix any compile errors or lint warnings
- Files: `cmd/ssq-hooks/main.go`

#### Story 4.1.2: P-1 Live Payload Capture (Recommended Pre-Merge)
**As a** developer finalizing the `--gemini` adapter, **I want** to capture a real `$TOOL_INPUT` payload from agy, **so that** the schema variants in `parseGeminiPayload()` match reality.

**Acceptance Criteria**:
- A real agy `$TOOL_INPUT` payload has been captured and compared against Variant A and Variant B
- If the real schema differs from both variants, `parseGeminiPayload()` is updated accordingly
- The capture result is documented in a comment in `parseGeminiPayload()`

**Files**: `cmd/ssq-hooks/main.go`

##### Task 4.1.2a: Capture live agy payload (~3 min)
- Create a temporary hook to capture the raw payload:
  ```bash
  # In ~/.gemini/antigravity-cli/settings.json temporarily:
  {"hooks": {"BeforeTool": "printf '%s' \"$TOOL_INPUT\" > /tmp/agy-payload.json; exit 0"}}
  ```
- Trigger an agy tool call (e.g., ask agy to list files)
- Read `/tmp/agy-payload.json` and compare to expected schema variants
- Update the `GeminiToolPayload` struct comment in `parseGeminiPayload()` with confirmed schema
- Restore the real hook by running `ssq-hooks install agy`
- Files: `cmd/ssq-hooks/main.go` (comment update only)

---

## Exit Code Contract Reference (P-6)

**Confirmed from `install-gemini-hook.sh` + architecture research:**

| Decision    | Claude path                        | Gemini/agy path (`--gemini`) |
|-------------|-----------------------------------|-------------------------------|
| AutoAllow   | stdout JSON `"allow"`             | exit 0, no stdout             |
| AutoDeny    | stdout JSON `"deny"` + reason     | exit 1, reason to stderr      |
| Escalate    | no stdout (Claude shows dialog)   | exit 0, no stdout (agy shows dialog) |

The key difference: Claude reads stdout JSON; Gemini/agy reads exit code only. Writing Claude-format JSON on stdout in `--gemini` mode would be harmless IF Gemini ignores stdout — but since this is unconfirmed for all agy versions, `writeGeminiHookDecision()` produces no stdout (only stderr on deny) to be safe.

---

## Config File Priority Reference (P-5)

For `ssq-hooks install gemini`:
1. `~/.gemini/settings.json` — check first (observed authoritative in live env)
2. `~/.gemini/config.json` — fallback
3. If neither: create `~/.gemini/settings.json`

**Never patch both** — only the first found is patched. The reason: if both files exist, agy/Gemini might load only one, and having two `BeforeTool` hooks in two different files is undefined behavior.

For `ssq-hooks install agy`:
- Fixed path: `~/.gemini/antigravity-cli/settings.json` (no discovery needed)

---

## P-1 Unknown Schema: Multi-Variant Fallback Design

The `parseGeminiPayload()` function implements a **try-variants-in-order** strategy:

```
stdin JSON
    │
    ├── JSON decode fails?     → return {ToolName: "Unknown"} → Escalate
    │
    ├── p.ToolName != "" ?     → Variant B (Claude-compatible)
    │       └── ask_for_user_input? → os.Exit(0)
    │       └── return {ToolName: p.ToolName, ToolInput: p.ToolInput}
    │
    ├── p.Name != "" ?         → Variant A (Gemini CLI open-source)
    │       └── normalize tool name (run_shell_command → Bash)
    │       └── ask_for_user_input? → os.Exit(0)
    │       └── return {ToolName: normalized, ToolInput: p.Args}
    │
    └── neither field present  → return {ToolName: "Unknown"} → Escalate
```

**Escalate is the safe fallback** — it causes agy to show its own permission dialog, which is the correct behavior when the hook cannot classify the request. It is not a crash (no os.Exit(1)) and not a false-allow.

---

## Task Summary

| Phase | Epic | Stories | Tasks | Key Files |
|-------|------|---------|-------|-----------|
| 1 | 1.1 patchBeforeToolHook | 1 | 1 | cmd/ssq-hooks/main.go |
| 1 | 1.2 installAgy | 1 | 2 | cmd/ssq-hooks/main.go |
| 1 | 1.3 installGemini upgrade | 1 | 1 | cmd/ssq-hooks/main.go |
| 2 | 2.1 parseGeminiPayload | 1 | 1 | cmd/ssq-hooks/main.go |
| 2 | 2.2 --gemini flag + output | 1 | 2 | cmd/ssq-hooks/main.go |
| 3 | 3.1 detector.go comment | 1 | 1 | session/detection/detector.go |
| 4 | 4.1 build + smoke test | 2 | 3 | cmd/ssq-hooks/main.go |

**Totals: 4 Phases, 7 Epics, 8 Stories, 11 Tasks**
