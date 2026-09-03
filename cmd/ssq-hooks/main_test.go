package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// ── patchBeforeToolHook unit tests ────────────────────────────────────────────

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
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{"theme":"dark"}`), 0644))
	require.NoError(t, patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini"))
	raw, _ := os.ReadFile(settingsPath)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Equal(t, "dark", m["theme"]) // existing key preserved
	hooks := m["hooks"].(map[string]interface{})
	assert.Equal(t, "ssq-hooks check --gemini", hooks["BeforeTool"])
}

// patchBeforeToolHook_should_returnNilAndPrintAlreadyPresent_When_exactHookAlreadySet
func TestPatchBeforeToolHook_Idempotent(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	// First write
	require.NoError(t, patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini"))
	stat1, err := os.Stat(settingsPath)
	require.NoError(t, err)
	// Second write — must be no-op (file content unchanged)
	require.NoError(t, patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini"))
	content2, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	// Verify the hook is present and content is valid JSON with exactly one BeforeTool entry
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(content2, &m))
	hooks := m["hooks"].(map[string]interface{})
	assert.Equal(t, "ssq-hooks check --gemini", hooks["BeforeTool"])
	// mtime should not have changed on idempotent run (file not rewritten)
	stat2, _ := os.Stat(settingsPath)
	assert.Equal(t, stat1.ModTime(), stat2.ModTime(), "file mtime changed on second run")
}

// patchBeforeToolHook_should_returnError_When_hooksBeforeToolIsNonString
func TestPatchBeforeToolHook_NonStringBeforeTool(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{"hooks":{"BeforeTool":["array","value"]}}`), 0644))
	err := patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a string")
}

// patchBeforeToolHook_should_returnError_When_jsonIsMalformed
func TestPatchBeforeToolHook_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{not valid json`), 0644))
	err := patchBeforeToolHook(settingsPath, "ssq-hooks check --gemini")
	require.Error(t, err)
}

// ── patchAntigravityHooks unit tests ──────────────────────────────────────────

func TestPatchAntigravityHooks_NewFile(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "subdir", "hooks.json")
	err := patchAntigravityHooks(hooksPath, "/path/to/ssq-hooks")
	require.NoError(t, err)
	raw, _ := os.ReadFile(hooksPath)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	sqHook := m["stapler-squad"].(map[string]interface{})
	assert.True(t, sqHook["enabled"].(bool))
	preTool := sqHook["PreToolUse"].([]interface{})
	first := preTool[0].(map[string]interface{})
	assert.Equal(t, "*", first["matcher"])
	hooks := first["hooks"].([]interface{})
	h := hooks[0].(map[string]interface{})
	assert.Equal(t, "command", h["type"])
	assert.Equal(t, "/path/to/ssq-hooks check --antigravity", h["command"])
}

func TestPatchAntigravityHooks_Idempotent(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.json")
	require.NoError(t, patchAntigravityHooks(hooksPath, "/path/to/ssq-hooks"))
	stat1, err := os.Stat(hooksPath)
	require.NoError(t, err)

	require.NoError(t, patchAntigravityHooks(hooksPath, "/path/to/ssq-hooks"))
	stat2, err := os.Stat(hooksPath)
	require.NoError(t, err)
	assert.Equal(t, stat1.ModTime(), stat2.ModTime())
}

// ── installAgy integration tests ──────────────────────────────────────────────

// installAgy_should_copyBinaryAndPatchHooks_When_dirAbsent
func TestInstallAgy_FreshInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	installAgy()
	// Assert binary copied
	destBin := filepath.Join(home, ".local", "bin", "ssq-hooks")
	assert.FileExists(t, destBin)
	// Assert hooks patched
	hooksPath := filepath.Join(home, ".gemini", "antigravity-cli", "hooks.json")
	raw, err := os.ReadFile(hooksPath)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	sq := m["stapler-squad"].(map[string]interface{})
	assert.True(t, sq["enabled"].(bool))
	preTool := sq["PreToolUse"].([]interface{})
	first := preTool[0].(map[string]interface{})
	hooks := first["hooks"].([]interface{})
	h := hooks[0].(map[string]interface{})
	assert.Contains(t, h["command"].(string), "check --antigravity")
}

// installAgy_should_beIdempotent_When_runTwice
func TestInstallAgy_Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	installAgy()
	// Capture hooks after first run
	hooksPath := filepath.Join(home, ".gemini", "antigravity-cli", "hooks.json")
	content1, err := os.ReadFile(hooksPath)
	require.NoError(t, err)
	installAgy()
	content2, err := os.ReadFile(hooksPath)
	require.NoError(t, err)
	assert.Equal(t, string(content1), string(content2), "hooks changed on second run")
}

// installAgy_should_patchOnlyAntigravityCli_When_bothFilesExist
func TestInstallAgy_PatchesOnlyFirstFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	agyDir := filepath.Join(home, ".gemini", "antigravity-cli")
	configDir := filepath.Join(home, ".gemini", "config")
	require.NoError(t, os.MkdirAll(agyDir, 0700))
	require.NoError(t, os.MkdirAll(configDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(agyDir, "hooks.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "hooks.json"), []byte(`{"other":"value"}`), 0644))
	installAgy()
	raw1, _ := os.ReadFile(filepath.Join(agyDir, "hooks.json"))
	assert.Contains(t, string(raw1), "check --antigravity")
	raw2, _ := os.ReadFile(filepath.Join(configDir, "hooks.json"))
	assert.JSONEq(t, `{"other":"value"}`, string(raw2))
}

// installAgy_should_fallBackToConfigJson_When_antigravityCliAbsent
func TestInstallAgy_FallsBackToConfigJson(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".gemini", "config")
	require.NoError(t, os.MkdirAll(configDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "hooks.json"), []byte("{}"), 0644))
	installAgy()
	raw, _ := os.ReadFile(filepath.Join(configDir, "hooks.json"))
	assert.Contains(t, string(raw), "check --antigravity")
	_, err := os.Stat(filepath.Join(home, ".gemini", "antigravity-cli", "hooks.json"))
	assert.True(t, os.IsNotExist(err))
}

// installAgy_should_cleanStaleFallback_When_bothWerePreviouslyPatched
func TestInstallAgy_CleansUpStaleFallbackEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	agyDir := filepath.Join(home, ".gemini", "antigravity-cli")
	configDir := filepath.Join(home, ".gemini", "config")
	require.NoError(t, os.MkdirAll(agyDir, 0700))
	require.NoError(t, os.MkdirAll(configDir, 0700))
	staleHook := `{"stapler-squad":{"enabled":true,"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"/fake/ssq-hooks check --antigravity","timeout":10}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(agyDir, "hooks.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "hooks.json"), []byte(staleHook), 0644))
	installAgy()
	raw1, _ := os.ReadFile(filepath.Join(agyDir, "hooks.json"))
	assert.Contains(t, string(raw1), "check --antigravity")
	raw2, _ := os.ReadFile(filepath.Join(configDir, "hooks.json"))
	assert.NotContains(t, string(raw2), "stapler-squad")
}

// installAgy_should_createAntigravityCli_When_neitherPathExists
func TestInstallAgy_CreatesAntigravityCliWhenNeitherExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	installAgy()
	agyHooks := filepath.Join(home, ".gemini", "antigravity-cli", "hooks.json")
	assert.FileExists(t, agyHooks)
	raw, _ := os.ReadFile(agyHooks)
	assert.Contains(t, string(raw), "check --antigravity")
	_, err := os.Stat(filepath.Join(home, ".gemini", "config", "hooks.json"))
	assert.True(t, os.IsNotExist(err))
}

// ── installGemini integration tests ───────────────────────────────────────────

// installGemini_should_patchSettingsJson_When_settingsJsonExists
func TestInstallGemini_UsesSettingsJson(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsPath := filepath.Join(home, ".gemini", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(settingsPath), 0700))
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{}`), 0644))
	installGemini()
	raw, _ := os.ReadFile(settingsPath)
	assert.Contains(t, string(raw), "check --gemini")
}

// installGemini_should_fallBackToConfigJson_When_settingsJsonAbsent
func TestInstallGemini_FallsBackToConfigJson(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".gemini", "config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0700))
	require.NoError(t, os.WriteFile(configPath, []byte(`{}`), 0644))
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
	require.NoError(t, os.MkdirAll(geminiDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(geminiDir, "settings.json"), []byte(`{}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(geminiDir, "config.json"), []byte(`{"other":"value"}`), 0644))
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
	content1, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	installGemini()
	content2, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Equal(t, string(content1), string(content2))
}

// ── parseGeminiPayload unit tests ─────────────────────────────────────────────

// callParseGeminiPayloadWithStdin replaces os.Stdin with a pipe backed by input,
// calls parseGeminiPayload(), and restores os.Stdin.
func callParseGeminiPayloadWithStdin(t *testing.T, input string) classifier.PermissionRequestPayload {
	t.Helper()
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	// Write input and close write end so ReadAll sees EOF.
	_, err = io.WriteString(w, input)
	require.NoError(t, err)
	w.Close()

	return parseGeminiPayload()
}

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

// parseGeminiPayload_should_returnToolName_When_variantCPayloadProvided
func TestParseGeminiPayload_VariantC(t *testing.T) {
	input := `{"toolCall":{"name":"run_command","args":{"command":"ls -la"}},"cwd":"/some/dir"}`
	payload := callParseGeminiPayloadWithStdin(t, input)
	assert.Equal(t, "Bash", payload.ToolName)
	assert.Equal(t, "ls -la", payload.ToolInput["command"])
	assert.Equal(t, "/some/dir", payload.Cwd)
}

// parseGeminiPayload_should_useWorkspacePathsAsCwd_When_topLevelCwdAbsent
func TestParseGeminiPayload_VariantC_WorkspacePathsFallback(t *testing.T) {
	input := `{"toolCall":{"name":"run_command","args":{"command":"ls"}},"workspacePaths":["/ws/dir"]}`
	payload := callParseGeminiPayloadWithStdin(t, input)
	assert.Equal(t, "/ws/dir", payload.Cwd)
}

// parseGeminiPayload_should_preferTopLevelCwd_When_bothCwdAndWorkspacePathsPresent
func TestParseGeminiPayload_VariantC_CwdPrecedenceOverWorkspacePaths(t *testing.T) {
	input := `{"toolCall":{"name":"run_command","args":{"command":"ls"}},"cwd":"/direct","workspacePaths":["/ws/dir"]}`
	payload := callParseGeminiPayloadWithStdin(t, input)
	assert.Equal(t, "/direct", payload.Cwd)
}

// parseGeminiPayload_should_normalizeCommandLineToCommand_When_argsCommandAbsent
func TestParseGeminiPayload_VariantC_CommandLineNormalization(t *testing.T) {
	input := `{"toolCall":{"name":"run_command","args":{"CommandLine":"ls -la"}}}`
	payload := callParseGeminiPayloadWithStdin(t, input)
	assert.Equal(t, "ls -la", payload.ToolInput["command"])
}

// parseGeminiPayload_should_pullArgsCwdIntoPayloadCwd_When_topLevelCwdAbsent
func TestParseGeminiPayload_VariantC_ArgsCwdPullUp(t *testing.T) {
	input := `{"toolCall":{"name":"run_command","args":{"command":"ls","Cwd":"/args/dir"}}}`
	payload := callParseGeminiPayloadWithStdin(t, input)
	assert.Equal(t, "/args/dir", payload.Cwd)
}

// parseGeminiPayload_should_preferTopLevelCwd_When_bothCwdAndArgsCwdPresent
func TestParseGeminiPayload_VariantC_TopLevelCwdPrecedenceOverArgsCwd(t *testing.T) {
	input := `{"toolCall":{"name":"run_command","args":{"command":"ls","Cwd":"/args/dir"}},"cwd":"/direct"}`
	payload := callParseGeminiPayloadWithStdin(t, input)
	assert.Equal(t, "/direct", payload.Cwd)
}

// parseGeminiPayload_should_preferArgsCwd_When_bothArgsCwdAndWorkspacePathsPresent
func TestParseGeminiPayload_VariantC_ArgsCwdPrecedenceOverWorkspacePaths(t *testing.T) {
	input := `{"toolCall":{"name":"run_command","args":{"command":"ls","Cwd":"/args/dir"}},"workspacePaths":["/ws/dir"]}`
	payload := callParseGeminiPayloadWithStdin(t, input)
	assert.Equal(t, "/args/dir", payload.Cwd)
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
			input := `{"name":"` + tc.geminiName + `","args":{}}`
			payload := callParseGeminiPayloadWithStdin(t, input)
			assert.Equal(t, tc.want, payload.ToolName)
		})
	}
}

// ── writeGeminiHookDecision unit tests (subprocess pattern) ──────────────────

// TestMain gates the subprocess exit-path helpers.
func TestMain(m *testing.M) {
	// Subprocess gate: if GEMINI_DECISION is set, run the decision function and exit.
	if decision := os.Getenv("GEMINI_DECISION"); decision != "" {
		reason := os.Getenv("GEMINI_DENY_REASON")
		ruleID := os.Getenv("GEMINI_DENY_RULE")
		alt := os.Getenv("GEMINI_DENY_ALT")
		var result classifier.ClassificationResult
		switch decision {
		case "allow":
			result.Decision = classifier.AutoAllow
		case "deny":
			result.Decision = classifier.AutoDeny
			result.Reason = reason
			result.RuleID = ruleID
			result.Alternative = alt
		case "escalate":
			result.Decision = classifier.Escalate
		}
		writeGeminiHookDecision(result)
		os.Exit(0) // reached only for AutoAllow / Escalate
	}
	if decision := os.Getenv("AGY_DECISION"); decision != "" {
		reason := os.Getenv("AGY_DENY_REASON")
		alt := os.Getenv("AGY_DENY_ALT")
		var result classifier.ClassificationResult
		switch decision {
		case "allow":
			result.Decision = classifier.AutoAllow
		case "deny":
			result.Decision = classifier.AutoDeny
			result.Reason = reason
			result.Alternative = alt
		case "escalate":
			result.Decision = classifier.Escalate
		}
		writeAntigravityHookDecision(result)
		os.Exit(0)
	}
	if decision := os.Getenv("OPENCODE_DECISION"); decision != "" {
		reason := os.Getenv("OPENCODE_DENY_REASON")
		ruleID := os.Getenv("OPENCODE_DENY_RULE")
		alt := os.Getenv("OPENCODE_DENY_ALT")
		var result classifier.ClassificationResult
		switch decision {
		case "allow":
			result.Decision = classifier.AutoAllow
		case "deny":
			result.Decision = classifier.AutoDeny
			result.Reason = reason
			result.RuleID = ruleID
			result.Alternative = alt
		case "escalate":
			result.Decision = classifier.Escalate
		}
		writeOpenCodeHookDecision(result)
		os.Exit(0) // reached only for AutoAllow
	}
	os.Exit(m.Run())
}

// subprocessResult holds the outcome of a subprocess invocation.
type subprocessResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// runDecisionSubprocess re-invokes the test binary (gated into TestMain by env, per
// TestMain's *_DECISION checks) to exercise a writeXHookDecision function that calls os.Exit.
// Shared by the gemini/agy/opencode decision tests — each passes its own *_DECISION env var.
func runDecisionSubprocess(t *testing.T, env ...string) subprocessResult {
	t.Helper()
	c := exec.Command(os.Args[0], "-test.run=^TestMain$") //nolint:forbidigo,norawexec // subprocess re-invoke of the test binary; context/WaitDelay not applicable to test binary re-execution
	c.Env = append(os.Environ(), env...)
	var stdout, stderr strings.Builder
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
	}
	return subprocessResult{
		ExitCode: code,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

// writeGeminiHookDecision_should_exitZero_When_autoAllow
func TestWriteGeminiHookDecision_AutoAllow(t *testing.T) {
	result := classifier.ClassificationResult{Decision: classifier.AutoAllow}
	// We test via a captured stderr buffer; AutoAllow simply returns (no os.Exit needed).
	// Capture stderr to verify nothing is written.
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	writeGeminiHookDecision(result)
	w.Close()
	os.Stderr = old
	buf, _ := io.ReadAll(r)
	assert.Empty(t, string(buf))
}

// writeGeminiHookDecision_should_exitZeroSilently_When_escalate
func TestWriteGeminiHookDecision_Escalate(t *testing.T) {
	result := classifier.ClassificationResult{Decision: classifier.Escalate}
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	writeGeminiHookDecision(result)
	w.Close()
	os.Stderr = old
	buf, _ := io.ReadAll(r)
	assert.Empty(t, string(buf))
}

// writeGeminiHookDecision_should_exitOneWithStderr_When_autoDeny
// Uses subprocess pattern because writeGeminiHookDecision calls os.Exit(1) on AutoDeny.
func TestWriteGeminiHookDecision_AutoDeny(t *testing.T) {
	res := runDecisionSubprocess(t,
		"GEMINI_DECISION=deny",
		"GEMINI_DENY_REASON=dangerous command",
	)
	assert.Equal(t, 1, res.ExitCode)
	assert.Contains(t, res.Stderr, "blocked")
	assert.Contains(t, res.Stderr, "dangerous command")
}

// writeGeminiHookDecision_should_includeDenyReasonAndRuleID_When_autoDenyWithRuleID
func TestWriteGeminiHookDecision_AutoDenyWithRuleID(t *testing.T) {
	res := runDecisionSubprocess(t,
		"GEMINI_DECISION=deny",
		"GEMINI_DENY_RULE=RULE-007",
		"GEMINI_DENY_REASON=rm -rf blocked",
	)
	assert.Equal(t, 1, res.ExitCode)
	assert.Contains(t, res.Stderr, "[rule: RULE-007]")
	assert.Contains(t, res.Stderr, "rm -rf blocked")
}

func TestWriteAntigravityHookDecision_AutoAllow(t *testing.T) {
	res := runDecisionSubprocess(t, "AGY_DECISION=allow")
	assert.Equal(t, 0, res.ExitCode)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(res.Stdout), &out))
	assert.Equal(t, "allow", out["decision"])
	assert.Equal(t, true, out["allow_tool"])
}

func TestWriteAntigravityHookDecision_AutoDeny(t *testing.T) {
	res := runDecisionSubprocess(t, "AGY_DECISION=deny", "AGY_DENY_REASON=unsafe", "AGY_DENY_ALT=try again")
	assert.Equal(t, 0, res.ExitCode)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(res.Stdout), &out))
	assert.Equal(t, "deny", out["decision"])
	assert.Equal(t, false, out["allow_tool"])
	assert.Equal(t, "unsafe try again", out["deny_reason"])
}

func TestWriteAntigravityHookDecision_Escalate(t *testing.T) {
	res := runDecisionSubprocess(t, "AGY_DECISION=escalate")
	assert.Equal(t, 0, res.ExitCode)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(res.Stdout), &out))
	assert.Equal(t, "ask", out["decision"])
	assert.Nil(t, out["allow_tool"])
}

// ── patchOpenCodeHooks unit tests ─────────────────────────────────────────────

// patchOpenCodeHooks_should_writePluginFileWithBinPath_When_fileAbsent
func TestPatchOpenCodeHooks_NewFile(t *testing.T) {
	dir := t.TempDir()
	pluginPath := filepath.Join(dir, "plugins", "ssq-hooks.js")
	err := patchOpenCodeHooks(pluginPath, "/path/to/ssq-hooks")
	require.NoError(t, err)
	raw, err := os.ReadFile(pluginPath)
	require.NoError(t, err)
	content := string(raw)
	assert.Contains(t, content, `"tool.execute.before"`)
	assert.Contains(t, content, `check`)
	assert.Contains(t, content, `--opencode`)
	assert.Contains(t, content, "/path/to/ssq-hooks")

	// Substring assertions above don't catch a template edit that breaks JS syntax (bad
	// escape, missing comma/brace) — verify the generated file actually parses as JS when a
	// Node runtime is available. Skips silently if `node` isn't on PATH rather than failing,
	// since this isn't a hard build dependency of the Go toolchain.
	if nodePath, err := exec.LookPath("node"); err == nil {
		out, err := exec.Command(nodePath, "--check", pluginPath).CombinedOutput() //nolint:forbidigo,norawexec // test-only JS syntax check
		assert.NoErrorf(t, err, "generated plugin is not valid JS: %s", out)
	}
}

// patchOpenCodeHooks_should_produceByteIdenticalOutput_When_runTwice
func TestPatchOpenCodeHooks_Idempotent(t *testing.T) {
	dir := t.TempDir()
	pluginPath := filepath.Join(dir, "plugins", "ssq-hooks.js")
	require.NoError(t, patchOpenCodeHooks(pluginPath, "/path/to/ssq-hooks"))
	content1, err := os.ReadFile(pluginPath)
	require.NoError(t, err)
	require.NoError(t, patchOpenCodeHooks(pluginPath, "/path/to/ssq-hooks"))
	content2, err := os.ReadFile(pluginPath)
	require.NoError(t, err)
	assert.Equal(t, string(content1), string(content2))
}

// ── installOpenCode integration tests ─────────────────────────────────────────

// installOpenCode_should_installBinaryAndPlugin_When_freshInstall
func TestInstallOpenCode_FreshInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No `opencode` binary resolvable — keeps installOpenCode's best-effort version probe a
	// no-op instead of shelling out to whatever's actually on the host machine's PATH (slow,
	// and makes the test's behavior depend on host state).
	t.Setenv("PATH", t.TempDir())
	installOpenCode()
	destBin := filepath.Join(home, ".local", "bin", "ssq-hooks")
	assert.FileExists(t, destBin)
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "ssq-hooks.js")
	raw, err := os.ReadFile(pluginPath)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "check")
	assert.Contains(t, string(raw), "--opencode")
	assert.Contains(t, string(raw), destBin)
}

// installOpenCode_should_removeStaleWrapper_When_oldWrapperPresent
func TestInstallOpenCode_RemovesStaleWrapper(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	binDir := filepath.Join(home, ".local", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))
	staleWrapper := filepath.Join(binDir, "open-code")
	staleContent := "#!/usr/bin/env bash\nCMD=$(/old/path/ssq-hooks proxy -- open-code \"$@\")\neval \"$CMD\"\n"
	require.NoError(t, os.WriteFile(staleWrapper, []byte(staleContent), 0755))

	installOpenCode()

	_, err := os.Stat(staleWrapper)
	assert.True(t, os.IsNotExist(err), "expected stale open-code wrapper to be removed")
	// New plugin should still be installed.
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "ssq-hooks.js")
	assert.FileExists(t, pluginPath)
}

// removeStaleOpenCodeWrapper_should_leaveUnrelatedFileAlone_When_contentDoesNotMatch
func TestRemoveStaleOpenCodeWrapper_LeavesUnrelatedFileAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "open-code")
	require.NoError(t, os.WriteFile(path, []byte("#!/usr/bin/env bash\necho hi\n"), 0755))
	require.NoError(t, removeStaleOpenCodeWrapper(path))
	assert.FileExists(t, path)
}

// removeStaleOpenCodeWrapper_should_noop_When_fileAbsent
func TestRemoveStaleOpenCodeWrapper_NoopWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "open-code")
	require.NoError(t, removeStaleOpenCodeWrapper(path))
}

// ── ssqApprovalExtensionContent unit tests ────────────────────────────────────

// ssqApprovalExtensionContent_should_safelyEmbedBothURLs_When_rendered
func TestSsqApprovalExtensionContent_EmbedsBothURLs(t *testing.T) {
	content := ssqApprovalExtensionContent("http://localhost:8543/api/hooks/permission-request", "http://localhost:8543/api/hooks/pi-extension-loaded")
	assert.Contains(t, content, `pi.on("tool_call"`)
	assert.Contains(t, content, `const permissionURL = "http://localhost:8543/api/hooks/permission-request";`)
	assert.Contains(t, content, `const healthURL = "http://localhost:8543/api/hooks/pi-extension-loaded";`)
	assert.Contains(t, content, "block: true")
	assert.Contains(t, content, "block: false")

	// Substring assertions above don't catch a template edit that breaks JS syntax — verify
	// the generated file actually parses as JS when a Node runtime is available, mirroring
	// TestPatchOpenCodeHooks_NewFile's node --check approach.
	if nodePath, err := exec.LookPath("node"); err == nil {
		dir := t.TempDir()
		extPath := filepath.Join(dir, "ssq-approval.ts")
		require.NoError(t, os.WriteFile(extPath, []byte(content), 0644))
		out, err := exec.Command(nodePath, "--check", extPath).CombinedOutput() //nolint:forbidigo,norawexec // test-only JS syntax check
		assert.NoErrorf(t, err, "generated extension is not valid JS: %s", out)
	}
}

// ssqApprovalExtensionContent_should_safelyEscapeURLsContainingQuotesOrSpecialChars_When_rendered
func TestSsqApprovalExtensionContent_SafelyEscapesSpecialCharacters(t *testing.T) {
	// A URL containing a double quote and backslash would break naive string interpolation
	// (e.g. fmt's %q or a raw Sprintf) but must survive json.Marshal's escaping intact.
	// encoding/json's default HTML-safe escaping additionally renders '<'/'>' as </>
	// in the output text — still valid, safely-embedded JS, just not the literal input bytes,
	// so the assertions below match that actual rendering rather than the raw input.
	content := ssqApprovalExtensionContent(`http://localhost:8543/"quote`, `http://localhost:8543/back\slash`)
	assert.Contains(t, content, `const permissionURL = "http://localhost:8543/\"quote";`)
	assert.Contains(t, content, `const healthURL = "http://localhost:8543/back\\slash";`)
}

// TestSsqApprovalExtensionContent_SetsSourcePi verifies pi-support Epic 4.3's
// Story 4.3.1c: the permission-request body identifies its source as "pi" so
// audit/analytics records can distinguish pi from Claude's unmodified curl
// hook (which never sends the field at all).
func TestSsqApprovalExtensionContent_SetsSourcePi(t *testing.T) {
	content := ssqApprovalExtensionContent("http://localhost:8543/api/hooks/permission-request", "http://localhost:8543/api/hooks/pi-extension-loaded")
	assert.Contains(t, content, `source: "pi"`)
}

// TestSsqApprovalExtensionContent_ReSendsHealthPingPeriodically verifies
// pi-support Story 4.2.3: the health ping is scheduled via setInterval, not
// just sent once at load, so a server restart doesn't permanently strand a
// live session's health badge at Unknown.
func TestSsqApprovalExtensionContent_ReSendsHealthPingPeriodically(t *testing.T) {
	content := ssqApprovalExtensionContent("http://localhost:8543/api/hooks/permission-request", "http://localhost:8543/api/hooks/pi-extension-loaded")
	assert.Contains(t, content, "setInterval(sendHealthPing")
	// The re-ping interval must be well inside the tracker's grace window
	// (piExtensionHealthGraceWindow = 2x piExtensionRepingInterval, per
	// server/services/pi_extension_health.go) — sanity-check the literal here
	// so a future edit to one side can't silently desync from the other.
	assert.Contains(t, content, "healthRepingIntervalMs = 120000")
}

// ── patchPiExtension unit tests ───────────────────────────────────────────────

// patchPiExtension_should_writeExtensionFileWithBothURLs_When_fileAbsent
func TestPatchPiExtension_NewFile(t *testing.T) {
	dir := t.TempDir()
	extPath := filepath.Join(dir, "extensions", "ssq-approval.ts")
	err := patchPiExtension(extPath, "http://localhost:8543/api/hooks/permission-request", "http://localhost:8543/api/hooks/pi-extension-loaded")
	require.NoError(t, err)
	raw, err := os.ReadFile(extPath)
	require.NoError(t, err)
	content := string(raw)
	assert.Contains(t, content, "permission-request")
	assert.Contains(t, content, "pi-extension-loaded")
	assert.Contains(t, content, `pi.on("tool_call"`)
}

// patchPiExtension_should_produceByteIdenticalOutput_When_runTwice
func TestPatchPiExtension_Idempotent(t *testing.T) {
	dir := t.TempDir()
	extPath := filepath.Join(dir, "extensions", "ssq-approval.ts")
	require.NoError(t, patchPiExtension(extPath, "http://localhost:8543/api/hooks/permission-request", "http://localhost:8543/api/hooks/pi-extension-loaded"))
	content1, err := os.ReadFile(extPath)
	require.NoError(t, err)
	require.NoError(t, patchPiExtension(extPath, "http://localhost:8543/api/hooks/permission-request", "http://localhost:8543/api/hooks/pi-extension-loaded"))
	content2, err := os.ReadFile(extPath)
	require.NoError(t, err)
	assert.Equal(t, string(content1), string(content2))
}

// patchPiExtension_should_logErrorWithTargetPathAndUnderlyingError_When_writeFails
func TestPatchPiExtension_LogsErrorWithTargetPath_WhenWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits do not restrict writes the same way on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permission bits do not restrict writes, so this failure cannot be simulated")
	}

	dir := t.TempDir()
	roDir := filepath.Join(dir, "extensions")
	require.NoError(t, os.MkdirAll(roDir, 0o755))
	extPath := filepath.Join(roDir, "ssq-approval.ts")
	// Strip the write bit so os.WriteFile's temp-file creation fails.
	require.NoError(t, os.Chmod(roDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	var buf bytes.Buffer
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })

	err := patchPiExtension(extPath, "http://localhost:8543/api/hooks/permission-request", "http://localhost:8543/api/hooks/pi-extension-loaded")
	require.Error(t, err)

	logged := buf.String()
	assert.Contains(t, logged, "level=ERROR")
	assert.Contains(t, logged, extPath)
	assert.Contains(t, logged, err.Error())
}

// ── installPi integration tests ───────────────────────────────────────────────

// installPi_should_installBinaryAndGlobalExtension_When_freshInstall
func TestInstallPi_FreshInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No `pi` binary resolvable — keeps installPi's best-effort version probe a no-op instead
	// of shelling out to whatever's actually on the host machine's PATH.
	t.Setenv("PATH", t.TempDir())
	installPi()
	destBin := filepath.Join(home, ".local", "bin", "ssq-hooks")
	assert.FileExists(t, destBin)
	// Per ADR-002: global scope (~/.pi/agent/extensions/), not project-local.
	extPath := filepath.Join(home, ".pi", "agent", "extensions", "ssq-approval.ts")
	raw, err := os.ReadFile(extPath)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "permission-request")
}

// installPi_should_produceByteIdenticalExtension_When_runTwice
func TestInstallPi_Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	extPath := filepath.Join(home, ".pi", "agent", "extensions", "ssq-approval.ts")

	installPi()
	content1, err := os.ReadFile(extPath)
	require.NoError(t, err)

	installPi()
	content2, err := os.ReadFile(extPath)
	require.NoError(t, err)
	assert.Equal(t, string(content1), string(content2))
}

// ── parseOpenCodePayload unit tests ───────────────────────────────────────────

func callParseOpenCodePayloadWithStdin(t *testing.T, input string) classifier.PermissionRequestPayload {
	t.Helper()
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()
	_, err = io.WriteString(w, input)
	require.NoError(t, err)
	w.Close()
	return parseOpenCodePayload()
}

// parseOpenCodePayload_should_returnToolNameAndInput_When_validPayload
func TestParseOpenCodePayload_Valid(t *testing.T) {
	input := `{"tool_name":"bash","tool_input":{"command":"echo hi"},"cwd":"/tmp","session_id":"ses_123"}`
	payload := callParseOpenCodePayloadWithStdin(t, input)
	assert.Equal(t, "bash", payload.ToolName)
	assert.Equal(t, "echo hi", payload.ToolInput["command"])
	assert.Equal(t, "/tmp", payload.Cwd)
	assert.Equal(t, "ses_123", payload.SessionID)
}

// parseOpenCodePayload_should_returnUnknown_When_toolNameMissing
func TestParseOpenCodePayload_MissingToolName(t *testing.T) {
	input := `{"tool_input":{"command":"echo hi"}}`
	payload := callParseOpenCodePayloadWithStdin(t, input)
	assert.Equal(t, "Unknown", payload.ToolName)
}

// parseOpenCodePayload_should_returnUnknown_When_malformedJSON
func TestParseOpenCodePayload_MalformedJSON(t *testing.T) {
	payload := callParseOpenCodePayloadWithStdin(t, "{not json")
	assert.Equal(t, "Unknown", payload.ToolName)
}

// parseOpenCodePayload_should_returnUnknown_When_inputEmpty
func TestParseOpenCodePayload_EmptyInput(t *testing.T) {
	payload := callParseOpenCodePayloadWithStdin(t, "")
	assert.Equal(t, "Unknown", payload.ToolName)
}

// parseOpenCodePayload_should_leaveToolInputNil_When_keyAbsent
//
// A zero-arg tool call has output.args === undefined in the plugin, and JSON.stringify drops
// undefined-valued keys entirely — so the real wire payload for that case has no "tool_input"
// key at all, not `{}`. normalizeOpenCodeToolInput's nil-map guard exists for exactly this.
func TestParseOpenCodePayload_ToolInputKeyAbsent(t *testing.T) {
	input := `{"tool_name":"bash","cwd":"/tmp"}`
	payload := callParseOpenCodePayloadWithStdin(t, input)
	assert.Equal(t, "bash", payload.ToolName)
	assert.Nil(t, payload.ToolInput)
}

// parseOpenCodePayload_should_populateFilePathKey_When_toolInputUsesCamelCaseFilePath
//
// Regression test for a bug caught only by a live end-to-end test (not a synthetic unit test):
// OpenCode's write/edit tool args use camelCase "filePath"
// (`{"content":"...","filePath":"/tmp/.env"}`), but classifier.Classify's FilePattern rules
// match against `payload.ToolInput["file_path"]` (snake_case, Claude's convention) — see
// pkg/classifier/classifier.go:735. Without normalizeOpenCodeToolInput, every OpenCode
// .env/.git write-protection rule silently never matched (empty-string lookup, no error).
func TestParseOpenCodePayload_NormalizesCamelCaseFilePath(t *testing.T) {
	input := `{"tool_name":"write","tool_input":{"content":"x","filePath":"/tmp/.env"},"cwd":"/tmp"}`
	payload := callParseOpenCodePayloadWithStdin(t, input)
	assert.Equal(t, "/tmp/.env", payload.ToolInput["file_path"])
}

// parseOpenCodePayload_should_preferExistingSnakeCaseFilePath_When_bothKeysPresent
func TestParseOpenCodePayload_PrefersExistingSnakeCaseFilePath(t *testing.T) {
	input := `{"tool_name":"write","tool_input":{"file_path":"/tmp/explicit.txt","filePath":"/tmp/other.txt"},"cwd":"/tmp"}`
	payload := callParseOpenCodePayloadWithStdin(t, input)
	assert.Equal(t, "/tmp/explicit.txt", payload.ToolInput["file_path"])
}

// parseOpenCodePayload_should_denyEnvFileWrite_When_writeToolTargetsEnvFile
//
// End-to-end regression test through the real classifier (not a mock): proves the
// camelCase-filePath normalization actually makes the shared seed-deny-env-write rule fire for
// an OpenCode-shaped payload, the exact gap the live opencode session test caught.
func TestParseOpenCodePayload_EnvFileWriteClassifiesAsAutoDeny(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	input := `{"tool_name":"write","tool_input":{"content":"SECRET=x","filePath":"/tmp/project/.env"},"cwd":"/tmp/project"}`
	payload := callParseOpenCodePayloadWithStdin(t, input)

	storage := loadStorage(dbPath)
	defer storage.Close()
	c := loadClassifier(storage)
	ctx := c.BuildContext(payload.Cwd)
	result := c.Classify(payload, ctx)

	assert.Equal(t, classifier.AutoDeny, result.Decision)
	assert.Equal(t, "seed-deny-env-write", result.RuleID)
}

// ── writeOpenCodeHookDecision tests ───────────────────────────────────────────

// writeOpenCodeHookDecision_should_exitZeroSilently_When_autoAllow
func TestWriteOpenCodeHookDecision_AutoAllow(t *testing.T) {
	result := classifier.ClassificationResult{Decision: classifier.AutoAllow}
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	writeOpenCodeHookDecision(result)
	w.Close()
	os.Stderr = old
	buf, _ := io.ReadAll(r)
	assert.Empty(t, string(buf))
}

// writeOpenCodeHookDecision_should_exitOneWithReasonOnStderr_When_autoDeny
// Uses subprocess pattern because writeOpenCodeHookDecision calls os.Exit(1) on AutoDeny.
func TestWriteOpenCodeHookDecision_AutoDeny(t *testing.T) {
	res := runDecisionSubprocess(t,
		"OPENCODE_DECISION=deny",
		"OPENCODE_DENY_REASON=dangerous command",
		"OPENCODE_DENY_RULE=RULE-042",
	)
	assert.Equal(t, 1, res.ExitCode)
	assert.Contains(t, res.Stderr, "blocked")
	assert.Contains(t, res.Stderr, "[rule: RULE-042]")
	assert.Contains(t, res.Stderr, "dangerous command")
}

// writeOpenCodeHookDecision_should_omitRuleInfo_When_ruleIDEmpty
func TestWriteOpenCodeHookDecision_AutoDenyWithoutRuleID(t *testing.T) {
	res := runDecisionSubprocess(t,
		"OPENCODE_DECISION=deny",
		"OPENCODE_DENY_REASON=dangerous command",
	)
	assert.Equal(t, 1, res.ExitCode)
	assert.Contains(t, res.Stderr, "blocked")
	assert.NotContains(t, res.Stderr, "[rule:")
	assert.Contains(t, res.Stderr, "dangerous command")
}

// writeOpenCodeHookDecision_should_exitOneWithDistinctReason_When_escalate
// Per ADR-027, Escalate maps to deny (fail-closed) with a reason distinct from AutoDeny's,
// naming the lack of an ask/dialog fallback rather than a specific rule match.
func TestWriteOpenCodeHookDecision_Escalate(t *testing.T) {
	res := runDecisionSubprocess(t, "OPENCODE_DECISION=escalate")
	assert.Equal(t, 1, res.ExitCode)
	assert.Contains(t, res.Stderr, "requires manual review")
	assert.Contains(t, res.Stderr, "no ask/dialog fallback")
	assert.NotContains(t, res.Stderr, "SSQ-Hooks: blocked", "escalate reason should use its own prefix, not AutoDeny's")
}
