package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	os.Exit(m.Run())
}

// subprocessResult holds the outcome of a subprocess invocation.
type subprocessResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func runGeminiDecisionSubprocess(t *testing.T, env ...string) subprocessResult {
	t.Helper()
	// Re-invoke the test binary, gated by GEMINI_DECISION env var.
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

func runAgyDecisionSubprocess(t *testing.T, env ...string) subprocessResult {
	t.Helper()
	c := exec.Command(os.Args[0], "-test.run=^TestMain$") //nolint:forbidigo,norawexec
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
	res := runGeminiDecisionSubprocess(t,
		"GEMINI_DECISION=deny",
		"GEMINI_DENY_REASON=dangerous command",
	)
	assert.Equal(t, 1, res.ExitCode)
	assert.Contains(t, res.Stderr, "blocked")
	assert.Contains(t, res.Stderr, "dangerous command")
}

// writeGeminiHookDecision_should_includeDenyReasonAndRuleID_When_autoDenyWithRuleID
func TestWriteGeminiHookDecision_AutoDenyWithRuleID(t *testing.T) {
	res := runGeminiDecisionSubprocess(t,
		"GEMINI_DECISION=deny",
		"GEMINI_DENY_RULE=RULE-007",
		"GEMINI_DENY_REASON=rm -rf blocked",
	)
	assert.Equal(t, 1, res.ExitCode)
	assert.Contains(t, res.Stderr, "[rule: RULE-007]")
	assert.Contains(t, res.Stderr, "rm -rf blocked")
}

func TestWriteAntigravityHookDecision_AutoAllow(t *testing.T) {
	res := runAgyDecisionSubprocess(t, "AGY_DECISION=allow")
	assert.Equal(t, 0, res.ExitCode)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(res.Stdout), &out))
	assert.Equal(t, "allow", out["decision"])
	assert.Equal(t, true, out["allow_tool"])
}

func TestWriteAntigravityHookDecision_AutoDeny(t *testing.T) {
	res := runAgyDecisionSubprocess(t, "AGY_DECISION=deny", "AGY_DENY_REASON=unsafe", "AGY_DENY_ALT=try again")
	assert.Equal(t, 0, res.ExitCode)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(res.Stdout), &out))
	assert.Equal(t, "deny", out["decision"])
	assert.Equal(t, false, out["allow_tool"])
	assert.Equal(t, "unsafe try again", out["deny_reason"])
}

func TestWriteAntigravityHookDecision_Escalate(t *testing.T) {
	res := runAgyDecisionSubprocess(t, "AGY_DECISION=escalate")
	assert.Equal(t, 0, res.ExitCode)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(res.Stdout), &out))
	assert.Equal(t, "ask", out["decision"])
	assert.Nil(t, out["allow_tool"])
}
