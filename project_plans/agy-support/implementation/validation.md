# Validation Plan: agy-support

**Date**: 2026-05-25

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1: ssq-hooks install agy | `cmd/ssq-hooks/main_test.go` | `patchBeforeToolHook_should_createFileAndSetHook_When_fileAbsent` | Unit | New file: missing settings file created with correct hook |
| REQ-1: ssq-hooks install agy | `cmd/ssq-hooks/main_test.go` | `patchBeforeToolHook_should_addHook_When_fileExistsWithNoHooks` | Unit | Existing file with no `hooks` key: key added, other keys preserved |
| REQ-1: ssq-hooks install agy | `cmd/ssq-hooks/main_test.go` | `patchBeforeToolHook_should_returnNilAndPrintAlreadyPresent_When_exactHookAlreadySet` | Unit | Idempotency: exact hook string already present → no-op, no duplicate |
| REQ-1: ssq-hooks install agy | `cmd/ssq-hooks/main_test.go` | `patchBeforeToolHook_should_returnError_When_hooksBeforeToolIsNonString` | Unit | `hooks.BeforeTool` is an array → return descriptive error, no overwrite |
| REQ-1: ssq-hooks install agy | `cmd/ssq-hooks/main_test.go` | `patchBeforeToolHook_should_returnError_When_jsonIsMalformed` | Unit | Malformed JSON in existing file → parse error returned |
| REQ-1: ssq-hooks install agy | `cmd/ssq-hooks/main_test.go` | `installAgy_should_copyBinaryAndPatchSettings_When_dirAbsent` | Integration | Full `installAgy()` in temp dir: binary copied, settings file patched at correct path |
| REQ-1: ssq-hooks install agy | `cmd/ssq-hooks/main_test.go` | `installAgy_should_beIdempotent_When_runTwice` | Integration | Re-running `installAgy()` produces identical file, prints "already present" |
| REQ-2: ssq-hooks install gemini | `cmd/ssq-hooks/main_test.go` | `installGemini_should_patchSettingsJson_When_settingsJsonExists` | Integration | `~/.gemini/settings.json` exists: patched with `check --gemini` hook |
| REQ-2: ssq-hooks install gemini | `cmd/ssq-hooks/main_test.go` | `installGemini_should_fallBackToConfigJson_When_settingsJsonAbsent` | Integration | `~/.gemini/settings.json` absent, `~/.gemini/config.json` exists: config.json patched |
| REQ-2: ssq-hooks install gemini | `cmd/ssq-hooks/main_test.go` | `installGemini_should_createSettingsJson_When_neitherFileExists` | Integration | Neither file present: `settings.json` created |
| REQ-2: ssq-hooks install gemini | `cmd/ssq-hooks/main_test.go` | `installGemini_should_patchOnlyFirstFound_When_bothFilesExist` | Integration | Both files exist: only `settings.json` patched, `config.json` unmodified |
| REQ-2: ssq-hooks install gemini | `cmd/ssq-hooks/main_test.go` | `installGemini_should_beIdempotent_When_runTwice` | Integration | Re-running is safe: hook not duplicated |
| REQ-3: ssq-hooks check --gemini | `cmd/ssq-hooks/main_test.go` | `parseGeminiPayload_should_returnBashTool_When_variantAPayloadProvided` | Unit | Variant A `{"name":"run_shell_command","args":{"command":"ls"}}` → `ToolName="Bash"`, `ToolInput={"command":"ls"}` |
| REQ-3: ssq-hooks check --gemini | `cmd/ssq-hooks/main_test.go` | `parseGeminiPayload_should_returnToolName_When_variantBPayloadProvided` | Unit | Variant B `{"tool_name":"Read","tool_input":{"file_path":"/etc/passwd"}}` → `ToolName="Read"` |
| REQ-3: ssq-hooks check --gemini | `cmd/ssq-hooks/main_test.go` | `parseGeminiPayload_should_returnUnknown_When_schemaUnrecognized` | Unit | Payload `{"command":"ls"}` (no `name` or `tool_name`) → `ToolName="Unknown"` |
| REQ-3: ssq-hooks check --gemini | `cmd/ssq-hooks/main_test.go` | `parseGeminiPayload_should_returnUnknown_When_inputIsEmpty` | Unit | Empty stdin `""` → JSON parse error → `ToolName="Unknown"` |
| REQ-3: ssq-hooks check --gemini | `cmd/ssq-hooks/main_test.go` | `parseGeminiPayload_should_normalizeToolNames_When_geminiNamesProvided` | Unit | `execute_bash` → `"Bash"`, `read_file` → `"Read"`, `write_file` → `"Write"` |
| REQ-3: ssq-hooks check --gemini | `cmd/ssq-hooks/main_test.go` | `writeGeminiHookDecision_should_exitZero_When_autoAllow` | Unit | `AutoAllow` decision → no stderr output, exit 0 |
| REQ-3: ssq-hooks check --gemini | `cmd/ssq-hooks/main_test.go` | `writeGeminiHookDecision_should_exitOneWithStderr_When_autoDeny` | Unit | `AutoDeny` decision → reason on stderr, exit 1 |
| REQ-3: ssq-hooks check --gemini | `cmd/ssq-hooks/main_test.go` | `writeGeminiHookDecision_should_exitZeroSilently_When_escalate` | Unit | `Escalate` decision → no stdout/stderr, exit 0 (agy shows own dialog) |
| REQ-3: ssq-hooks check --gemini | `cmd/ssq-hooks/main_test.go` | `writeGeminiHookDecision_should_includeDenyReasonAndRuleID_When_autoDenyWithRuleID` | Unit | `AutoDeny` with `RuleID` set → `[rule: <id>]` present in stderr message |
| REQ-3: ssq-hooks check --gemini | `cmd/ssq-hooks/main_test.go` | `handleCheck_gemini_should_useExitCodePath_When_geminiFlag` | Integration | `ssq-hooks check --gemini` with deny-rule payload → exits 1; without flag, same payload → Claude JSON stdout |
| REQ-4: agy detection patterns | `session/detection/detector_test.go` | `GeminiPatterns_should_matchAgyCandidateStrings_When_agySampleOutputProvided` | Unit | Known agy TUI strings (e.g. "Yes, allow once", ready indicator) match `gemini_permission`, `gemini_ready`, `gemini_working` patterns |
| REQ-4: agy detection patterns | `session/detection/detector_test.go` | `GeminiPatterns_should_returnNeedsApproval_When_permissionPromptPresent` | Unit | `gemini_permission` pattern fires for "Yes, allow once" substring → `NeedsApproval` state |
| REQ-4: agy detection patterns | `session/detection/detector_test.go` | `AgyCoverage_should_haveCommentInDetectorGo_When_noAgySpecificPatternsExist` | Unit | Source-level check: `getDefaultPatterns()` contains the `agy (Antigravity CLI)` coverage comment |

---

## Test Details

### Unit: `patchBeforeToolHook()`

All tests use `t.TempDir()` to create an isolated directory, calling `patchBeforeToolHook` with an absolute path inside it. No filesystem side-effects outside the temp dir.

```go
// patchBeforeToolHook_should_createFileAndSetHook_When_fileAbsent
func TestPatchBeforeToolHook_NewFile(t *testing.T) {
    dir := t.TempDir()
    settingsPath := filepath.Join(dir, "subdir", "settings.json")
    err := patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini")
    require.NoError(t, err)
    raw, _ := os.ReadFile(settingsPath)
    var m map[string]interface{}
    require.NoError(t, json.Unmarshal(raw, &m))
    hooks := m["hooks"].(map[string]interface{})
    assert.Equal(t, "ssq-hooks check --gemini", hooks["BeforeTool"])
}

// patchBeforeToolHook_should_addHook_When_fileExistsWithNoHooks
func TestPatchBeforeToolHook_ExistingFileNoHook(t *testing.T) {
    dir := t.TempDir()
    settingsPath := filepath.Join(dir, "settings.json")
    os.WriteFile(settingsPath, []byte(`{"theme":"dark"}`), 0644)
    require.NoError(t, patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini"))
    raw, _ := os.ReadFile(settingsPath)
    var m map[string]interface{}
    require.NoError(t, json.Unmarshal(raw, &m))
    assert.Equal(t, "dark", m["theme"])  // existing key preserved
    hooks := m["hooks"].(map[string]interface{})
    assert.Equal(t, "ssq-hooks check --gemini", hooks["BeforeTool"])
}

// patchBeforeToolHook_should_returnNilAndPrintAlreadyPresent_When_exactHookAlreadySet
func TestPatchBeforeToolHook_Idempotent(t *testing.T) {
    dir := t.TempDir()
    settingsPath := filepath.Join(dir, "settings.json")
    // First write
    require.NoError(t, patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini"))
    stat1, _ := os.Stat(settingsPath)
    // Second write — must be no-op (file unchanged)
    require.NoError(t, patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini"))
    stat2, _ := os.Stat(settingsPath)
    assert.Equal(t, stat1.ModTime(), stat2.ModTime(), "file mtime changed on second run")
}

// patchBeforeToolHook_should_returnError_When_hooksBeforeToolIsNonString
func TestPatchBeforeToolHook_NonStringBeforeTool(t *testing.T) {
    dir := t.TempDir()
    settingsPath := filepath.Join(dir, "settings.json")
    os.WriteFile(settingsPath, []byte(`{"hooks":{"BeforeTool":["array","value"]}}`), 0644)
    err := patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "not a string")
}

// patchBeforeToolHook_should_returnError_When_jsonIsMalformed
func TestPatchBeforeToolHook_MalformedJSON(t *testing.T) {
    dir := t.TempDir()
    settingsPath := filepath.Join(dir, "settings.json")
    os.WriteFile(settingsPath, []byte(`{not valid json`), 0644)
    err := patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini")
    require.Error(t, err)
}
```

### Unit: `parseGeminiPayload()`

Tests inject stdin via `os.Pipe()` or by temporarily redirecting `os.Stdin` to a `*os.File` backed by a `strings.Reader`. For exit-triggering cases (`ask_for_user_input`), use `exec.Command(os.Args[0], "-test.run=TestParseGeminiPayloadAskUserInput")` subprocess pattern with env flag gating.

```go
// parseGeminiPayload_should_returnBashTool_When_variantAPayloadProvided
func TestParseGeminiPayload_VariantA(t *testing.T) {
    input := `{"name":"run_shell_command","args":{"command":"ls -la"}}`
    payload := callParseGeminiPayloadWithStdin(t, input)
    assert.Equal(t, "Bash", payload.ToolName)
    assert.Equal(t, "ls -la", payload.ToolInput["command"])
}

// parseGeminiPayload_should_returnToolName_When_variantBPayloadProvided
func TestParseGeminiPayload_VariantB(t *testing.T) {
    input := `{"tool_name":"Read","tool_input":{"file_path":"/etc/passwd"}}`
    payload := callParseGeminiPayloadWithStdin(t, input)
    assert.Equal(t, "Read", payload.ToolName)
}

// parseGeminiPayload_should_returnUnknown_When_schemaUnrecognized
func TestParseGeminiPayload_UnknownSchema(t *testing.T) {
    input := `{"command":"ls"}` // no 'name' or 'tool_name'
    payload := callParseGeminiPayloadWithStdin(t, input)
    assert.Equal(t, "Unknown", payload.ToolName)
}

// parseGeminiPayload_should_returnUnknown_When_inputIsEmpty
func TestParseGeminiPayload_EmptyInput(t *testing.T) {
    payload := callParseGeminiPayloadWithStdin(t, "")
    assert.Equal(t, "Unknown", payload.ToolName)
}

// parseGeminiPayload_should_normalizeToolNames_When_geminiNamesProvided
func TestParseGeminiPayload_ToolNameNormalization(t *testing.T) {
    cases := []struct{ geminiName, want string }{
        {"run_shell_command", "Bash"},
        {"execute_bash", "Bash"},
        {"run_bash_command", "Bash"},
        {"read_file", "Read"},
        {"read_many_files", "Read"},
        {"write_file", "Write"},
        {"some_unknown_tool", "some_unknown_tool"}, // passthrough
    }
    for _, tc := range cases {
        t.Run(tc.geminiName, func(t *testing.T) {
            input := fmt.Sprintf(`{"name":%q,"args":{}}`, tc.geminiName)
            payload := callParseGeminiPayloadWithStdin(t, input)
            assert.Equal(t, tc.want, payload.ToolName)
        })
    }
}
```

Helper `callParseGeminiPayloadWithStdin(t, input string)` replaces `os.Stdin` with a pipe, writes `input`, calls `parseGeminiPayload()`, and restores `os.Stdin`.

### Unit: `writeGeminiHookDecision()`

Because the function calls `os.Exit`, tests use the subprocess pattern: run the test binary as a subprocess with an env flag that triggers only the exit path under test, then assert the exit code and stderr.

```go
// writeGeminiHookDecision_should_exitZero_When_autoAllow
func TestWriteGeminiHookDecision_AutoAllow(t *testing.T) {
    cmd := runInSubprocess(t, "GEMINI_DECISION=allow")
    assert.Equal(t, 0, cmd.ExitCode)
    assert.Empty(t, cmd.Stderr)
}

// writeGeminiHookDecision_should_exitOneWithStderr_When_autoDeny
func TestWriteGeminiHookDecision_AutoDeny(t *testing.T) {
    cmd := runInSubprocess(t, "GEMINI_DECISION=deny", "GEMINI_DENY_REASON=dangerous command")
    assert.Equal(t, 1, cmd.ExitCode)
    assert.Contains(t, cmd.Stderr, "blocked")
    assert.Contains(t, cmd.Stderr, "dangerous command")
}

// writeGeminiHookDecision_should_exitZeroSilently_When_escalate
func TestWriteGeminiHookDecision_Escalate(t *testing.T) {
    cmd := runInSubprocess(t, "GEMINI_DECISION=escalate")
    assert.Equal(t, 0, cmd.ExitCode)
    assert.Empty(t, cmd.Stderr)
    assert.Empty(t, cmd.Stdout)
}

// writeGeminiHookDecision_should_includeDenyReasonAndRuleID_When_autoDenyWithRuleID
func TestWriteGeminiHookDecision_AutoDenyWithRuleID(t *testing.T) {
    cmd := runInSubprocess(t, "GEMINI_DECISION=deny", "GEMINI_DENY_RULE=RULE-007", "GEMINI_DENY_REASON=rm -rf blocked")
    assert.Equal(t, 1, cmd.ExitCode)
    assert.Contains(t, cmd.Stderr, "[rule: RULE-007]")
    assert.Contains(t, cmd.Stderr, "rm -rf blocked")
}
```

### Integration: `installAgy()` and `installGemini()`

Integration tests override the home directory by patching `os.UserHomeDir` via a test wrapper or by setting `HOME` env var to `t.TempDir()` before the call. The actual binary copy uses the test binary itself as `os.Executable()`.

```go
// installAgy_should_copyBinaryAndPatchSettings_When_dirAbsent
func TestInstallAgy_FreshInstall(t *testing.T) {
    home := t.TempDir()
    t.Setenv("HOME", home)
    installAgy()  // must not call os.Exit on success
    // Assert binary copied
    destBin := filepath.Join(home, ".local", "bin", "ssq-hooks")
    assert.FileExists(t, destBin)
    // Assert settings patched
    settingsPath := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
    raw, err := os.ReadFile(settingsPath)
    require.NoError(t, err)
    var m map[string]interface{}
    require.NoError(t, json.Unmarshal(raw, &m))
    hooks := m["hooks"].(map[string]interface{})
    assert.Contains(t, hooks["BeforeTool"].(string), "check --gemini")
}

// installAgy_should_beIdempotent_When_runTwice
func TestInstallAgy_Idempotent(t *testing.T) {
    home := t.TempDir()
    t.Setenv("HOME", home)
    installAgy()
    // Capture file state after first run
    settingsPath := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
    content1, _ := os.ReadFile(settingsPath)
    installAgy()
    content2, _ := os.ReadFile(settingsPath)
    assert.Equal(t, string(content1), string(content2), "settings changed on second run")
}

// installGemini_should_patchSettingsJson_When_settingsJsonExists
func TestInstallGemini_UsesSettingsJson(t *testing.T) {
    home := t.TempDir()
    t.Setenv("HOME", home)
    settingsPath := filepath.Join(home, ".gemini", "settings.json")
    os.MkdirAll(filepath.Dir(settingsPath), 0700)
    os.WriteFile(settingsPath, []byte(`{}`), 0644)
    installGemini()
    raw, _ := os.ReadFile(settingsPath)
    assert.Contains(t, string(raw), "check --gemini")
}

// installGemini_should_fallBackToConfigJson_When_settingsJsonAbsent
func TestInstallGemini_FallsBackToConfigJson(t *testing.T) {
    home := t.TempDir()
    t.Setenv("HOME", home)
    configPath := filepath.Join(home, ".gemini", "config.json")
    os.MkdirAll(filepath.Dir(configPath), 0700)
    os.WriteFile(configPath, []byte(`{}`), 0644)
    installGemini()
    raw, _ := os.ReadFile(configPath)
    assert.Contains(t, string(raw), "check --gemini")
    // settings.json must NOT have been created
    _, err := os.Stat(filepath.Join(home, ".gemini", "settings.json"))
    assert.True(t, os.IsNotExist(err))
}

// installGemini_should_createSettingsJson_When_neitherFileExists
func TestInstallGemini_CreatesFreshSettingsJson(t *testing.T) {
    home := t.TempDir()
    t.Setenv("HOME", home)
    installGemini()
    settingsPath := filepath.Join(home, ".gemini", "settings.json")
    assert.FileExists(t, settingsPath)
    raw, _ := os.ReadFile(settingsPath)
    assert.Contains(t, string(raw), "check --gemini")
}

// installGemini_should_patchOnlyFirstFound_When_bothFilesExist
func TestInstallGemini_PatchesOnlyFirstFound(t *testing.T) {
    home := t.TempDir()
    t.Setenv("HOME", home)
    geminiDir := filepath.Join(home, ".gemini")
    os.MkdirAll(geminiDir, 0700)
    os.WriteFile(filepath.Join(geminiDir, "settings.json"), []byte(`{}`), 0644)
    os.WriteFile(filepath.Join(geminiDir, "config.json"), []byte(`{"other":"value"}`), 0644)
    installGemini()
    // settings.json patched
    raw1, _ := os.ReadFile(filepath.Join(geminiDir, "settings.json"))
    assert.Contains(t, string(raw1), "check --gemini")
    // config.json untouched
    raw2, _ := os.ReadFile(filepath.Join(geminiDir, "config.json"))
    assert.JSONEq(t, `{"other":"value"}`, string(raw2))
}

// installGemini_should_beIdempotent_When_runTwice
func TestInstallGemini_Idempotent(t *testing.T) {
    home := t.TempDir()
    t.Setenv("HOME", home)
    installGemini()
    settingsPath := filepath.Join(home, ".gemini", "settings.json")
    content1, _ := os.ReadFile(settingsPath)
    installGemini()
    content2, _ := os.ReadFile(settingsPath)
    assert.Equal(t, string(content1), string(content2))
}

// handleCheck_gemini_should_useExitCodePath_When_geminiFlag (subprocess test)
func TestHandleCheck_GeminiFlag_UsesExitCodePath(t *testing.T) {
    // Spawn subprocess: ssq-hooks check --gemini
    // Pipe in a Variant A deny-triggering payload (e.g. rm -rf rule in test DB)
    // Assert exit code 1 and stderr contains "blocked"
    // Then repeat without --gemini flag and assert exit code 0 + JSON on stdout
}
```

### Unit: REQ-4 Detector patterns

```go
// GeminiPatterns_should_matchAgyCandidateStrings_When_agySampleOutputProvided
func TestGeminiPatterns_AgyCoverage(t *testing.T) {
    patterns := getDefaultPatterns()
    agyCandidateLines := []string{
        "╰─ Yes, allow once",
        "◆ Thinking...",
        "◆ Ready",
    }
    for _, line := range agyCandidateLines {
        t.Run(line, func(t *testing.T) {
            matched := false
            for _, p := range patterns {
                if strings.Contains(p.Name, "gemini") {
                    if p.Pattern.MatchString(line) {
                        matched = true
                    }
                }
            }
            assert.True(t, matched, "no gemini_* pattern matched agy sample line: %q", line)
        })
    }
}

// GeminiPatterns_should_returnNeedsApproval_When_permissionPromptPresent
func TestGeminiPatterns_NeedsApprovalState(t *testing.T) {
    // uses existing detector_test helpers; asserts gemini_permission pattern
    // fires on "Yes, allow once" and maps to NeedsApproval session state
}

// AgyCoverage_should_haveCommentInDetectorGo_When_noAgySpecificPatternsExist
func TestDetector_AgyCoverageCommentPresent(t *testing.T) {
    // Read session/detection/detector.go source; assert "agy (Antigravity CLI)" substring present
    // This is a canary: if someone removes the comment or the coverage changes, test fails
    src, err := os.ReadFile("../../session/detection/detector.go")
    require.NoError(t, err)
    assert.Contains(t, string(src), "agy (Antigravity CLI)")
}
```

---

## Test Stack

- **Unit**: Go standard `testing` package + `github.com/stretchr/testify/assert` + `github.com/stretchr/testify/require`
- **Integration**: Same Go `testing` framework; subprocess invocations use `os/exec`; filesystem isolation via `t.TempDir()` + `t.Setenv("HOME", ...)`
- **API/E2E**: Not applicable — `ssq-hooks` is a CLI binary; end-to-end validation is manual (see Manual Validation Steps)

---

## Coverage Targets

- Unit test coverage: ≥80% line coverage for `cmd/ssq-hooks/main.go` new functions
- All public/package-level functions introduced by this feature: happy path + ≥1 error path
- All external integrations (filesystem I/O, exit codes): unit mocked via `t.TempDir()` / subprocess pattern

---

## Manual Validation Steps

The following behaviors cannot be fully verified by automated tests because they depend on a live running `agy` process:

### M-1: Live payload capture and schema confirmation (P-1)

**Prerequisite**: `agy` binary installed and licensed.

```bash
# Step 1: install temporary capture hook
cat > ~/.gemini/antigravity-cli/settings.json <<'EOF'
{"hooks": {"BeforeTool": "printf '%s' \"$TOOL_INPUT\" > /tmp/agy-payload.json; exit 0"}}
EOF

# Step 2: start agy and trigger a Bash tool call (ask it to list files)
agy
> List all files in /tmp

# Step 3: inspect captured payload
cat /tmp/agy-payload.json
# Compare field names to Variant A and Variant B defined in parseGeminiPayload()
# Update parseGeminiPayload() GeminiToolPayload struct comment with confirmed schema

# Step 4: restore real hook
ssq-hooks install agy
```

**Pass criteria**: Captured payload matches Variant A or Variant B. If a third schema is found, `parseGeminiPayload()` must be updated before merge.

### M-2: Exit code contract verification (P-6 concern from adversarial review)

**Prerequisite**: `agy` installed, a deny rule configured in the ssq-hooks SQLite DB.

```bash
# Configure a deny rule for rm -rf
ssq-hooks rules add --deny --pattern "rm -rf"

# In agy session, ask it to run: rm -rf /tmp/test-dir
# Expected: agy blocks the action (shows "blocked" or refuses to execute)
# If agy executes the command despite exit 1 from the hook → exit code contract unconfirmed
```

**Pass criteria**: `agy` refuses to execute the command when `ssq-hooks check --gemini` exits 1.

### M-3: Idempotent install from a real binary

```bash
# Install first time
ssq-hooks install agy
# Install second time — should print "Hook already present, nothing to do."
ssq-hooks install agy
# Verify no duplication
cat ~/.gemini/antigravity-cli/settings.json
```

**Pass criteria**: Second run prints idempotency message; settings file unchanged.

### M-4: Gemini `BeforeTool` hook fires on tool use

```bash
# Requires Gemini CLI installed
ssq-hooks install gemini
# Start gemini CLI; ask it to run a Bash command
# Observe: ssq-hooks check --gemini is invoked (check /tmp or add stderr logging)
```

**Pass criteria**: Hook fires and classifies the tool call without crashing Gemini.

---

## Notes on `os.Exit` Testability

`writeGeminiHookDecision()` and `parseGeminiPayload()` (for `ask_for_user_input`) call `os.Exit()`. The adversarial review flagged this as harder to test. The mitigation is the subprocess pattern: each exit-code test spawns the test binary itself with a sentinel env var that triggers a small `TestMain` helper running only the exit path. The subprocess's exit code and stdout/stderr are asserted by the parent test. This is the established Go pattern for testing `os.Exit` paths (see `installAgy()` analogue in existing ssq-hooks tests).
