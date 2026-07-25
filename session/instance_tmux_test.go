package session

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestBuildSubmittableInput_UsesCarriageReturnNotNewline is a regression test
// for BUG-047: WriteToSession (the ConnectRPC handler backing the web UI's
// session chat box) and the write_to_session/run_command MCP tools appended
// a bare '\n' after input intended to submit, instead of the '\r' that
// Claude Code's raw-mode TUI actually recognizes as Enter (matching TapEnter
// and the already-working nudge/initial-prompt/autonomous-driver call
// sites). A message sent with only a trailing '\n' is written into the pane
// but never submitted — it sits in the input buffer indefinitely, silently,
// with no error surfaced to the caller. This test fails against the
// pre-fix behavior (which appended "\n") and passes against
// BuildSubmittableInput's use of EnterKeySequence ("\r").
func TestBuildSubmittableInput_UsesCarriageReturnNotNewline(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		pressEnter bool
		want       string
	}{
		{"press enter appends carriage return", "merge it", true, "merge it\r"},
		{"no press enter leaves input untouched", "merge it", false, "merge it"},
		{"empty input with press enter is just the terminator", "", true, "\r"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildSubmittableInput(tc.input, tc.pressEnter)
			if got != tc.want {
				t.Errorf("BuildSubmittableInput(%q, %v) = %q, want %q", tc.input, tc.pressEnter, got, tc.want)
			}
			if tc.pressEnter && strings.HasSuffix(got, "\n") && !strings.HasSuffix(got, "\r") {
				t.Errorf("BuildSubmittableInput(%q, true) = %q ends in bare \\n, not \\r — Claude Code's "+
					"raw-mode TUI will not treat this as a submitted line (BUG-047)", tc.input, got)
			}
		})
	}

	if EnterKeySequence != "\r" {
		t.Errorf("EnterKeySequence = %q, want \"\\r\" (0x0D) — matching TapEnter's raw byte write and the "+
			"terminal convention every working SendKeys-with-enter call site in this package already relies on",
			EnterKeySequence)
	}
}

func TestIsClaude(t *testing.T) {
	cases := []struct {
		program string
		want    bool
	}{
		{"claude", true},
		{"/usr/local/bin/claude", true},
		{"env -u SOME_VAR claude", true}, // env wrapper — second token matches
		{"env claude --flag", true},      // env prefix
		{"claude-squad", false},          // basename is "claude-squad", not "claude"
		{"myclaudeapp", false},           // basename contains "claude" but is not "claude"
		{"/claude/bin/aider", false},     // "claude" is a directory component, not the binary
		{"aider", false},
		{"", false},
		{"Claude", false}, // case-sensitive: capital C does not match
		{"CLAUDE", false},
	}
	for _, tc := range cases {
		t.Run(tc.program, func(t *testing.T) {
			got := isClaude(tc.program)
			if got != tc.want {
				t.Errorf("isClaude(%q) = %v, want %v", tc.program, got, tc.want)
			}
		})
	}
}

func TestClassifyProgram(t *testing.T) {
	cases := []struct {
		program string
		want    string // "claude" or "plain"
	}{
		{"claude", "claude"},
		{"/usr/local/bin/claude", "claude"},
		{"env -u PROXY claude", "claude"},
		{"aider", "plain"},
		{"claude-squad", "plain"},
		{"", "plain"},
	}
	for _, tc := range cases {
		t.Run(tc.program, func(t *testing.T) {
			switch classifyProgram(tc.program).(type) {
			case claudeProgram:
				if tc.want != "claude" {
					t.Errorf("classifyProgram(%q) = claudeProgram, want plainProgram", tc.program)
				}
			case plainProgram:
				if tc.want != "plain" {
					t.Errorf("classifyProgram(%q) = plainProgram, want claudeProgram", tc.program)
				}
			}
		})
	}
}

func TestBuildLaunchCommand_NonClaudeProgramUnmodified(t *testing.T) {
	inst := &Instance{
		Program:      "aider",
		Prompt:       "do something",
		MCPServerURL: "http://localhost:8543",
		AllowedTools: "read,write",
	}
	got := inst.buildLaunchCommand("")
	if got != "aider" {
		t.Errorf("non-claude program should be returned unmodified, got %q", got)
	}
}

func TestBuildLaunchCommand_ClaudeSessionResume(t *testing.T) {
	inst := &Instance{Program: "claude"}
	got := inst.buildLaunchCommand("conv-abc123")
	expected := "claude --resume 'conv-abc123'"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestBuildLaunchCommand_ClaudeEnvWrapper(t *testing.T) {
	inst := &Instance{Program: "env -u PROXY claude"}
	got := inst.buildLaunchCommand("conv-xyz")
	if got == "env -u PROXY claude" {
		t.Error("resume flag was not appended to env-wrapped claude command")
	}
	if len(got) == 0 {
		t.Error("expected non-empty command")
	}
}

func TestShellQuote(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "''"},
		{"plain", "do something", "'do something'"},
		{"single_quote", "it's here", `'it'\''s here'`},
		{"backtick", "run `whoami`", "'run `whoami`'"},
		{"dollar_paren", "run $(whoami)", "'run $(whoami)'"},
		{"dollar_var", "echo $HOME", "'echo $HOME'"},
		{"only_single_quote", "'", `''\'''`},
		{"newline", "line one\nline two", "'line one\nline two'"},
		{"backtick_and_quote", "don't `touch /tmp/pwned`", "'don'\\''t `touch /tmp/pwned`'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shellQuote(tc.input)
			if got != tc.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestBuildClaudeCommand_PromptWithShellMetacharactersIsSafe(t *testing.T) {
	// Regression test for the backlog/triage launch bug: the backlog prompt is
	// full of backtick-wrapped tokens and begins with "--- BACKLOG ITEM DATA ---".
	// Both must be neutralized: single-quoting stops backtick/$()/$VAR expansion,
	// and the "--" separator stops claude from parsing the leading "--" as a flag.
	// want is a hand-written literal (not shellQuote(prompt)) so this test doesn't
	// just re-verify shellQuote against itself.
	prompt := "--- BACKLOG ITEM DATA ---\nRun `/backlog/done-0` when finished, or $(rm -rf /) if you dare."
	inst := &Instance{Program: "claude", Prompt: prompt}
	got := inst.buildLaunchCommand("")

	want := "claude -- '" + prompt + "'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !strings.Contains(got, " -- ") {
		t.Errorf("expected a bare -- separator before the prompt, got %q", got)
	}
	if strings.Contains(got, "%!q") {
		t.Errorf("prompt was not properly formatted: %q", got)
	}
}

func TestBuildClaudeCommand_AppendSystemPromptWithShellMetacharactersIsSafe(t *testing.T) {
	inst := &Instance{
		Program:            "claude",
		AppendSystemPrompt: "be `helpful` and $(honest)",
	}
	got := inst.buildLaunchCommand("")
	// Hand-written literal, not shellQuote(inst.AppendSystemPrompt), for the same
	// non-circularity reason as the Prompt test above.
	want := "claude --append-system-prompt 'be `helpful` and $(honest)'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildClaudeCommand_AllowedToolsWithShellMetacharactersIsSafe(t *testing.T) {
	inst := &Instance{
		Program:      "claude",
		AllowedTools: "Bash(`whoami`),Bash($(id))",
	}
	got := inst.buildLaunchCommand("")
	want := "claude --allowedTools 'Bash(`whoami`),Bash($(id))'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildClaudeCommand_PermissionModeWithShellMetacharactersIsSafe(t *testing.T) {
	inst := &Instance{
		Program:        "claude",
		PermissionMode: "plan; `touch /tmp/pwned`",
	}
	got := inst.buildLaunchCommand("")
	want := "claude --permission-mode 'plan; `touch /tmp/pwned`'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildClaudeCommand_ResumeIdWithShellMetacharactersIsSafe(t *testing.T) {
	// claudeSessionID comes from the client-supplied resume_id RPC field with no
	// format validation, so it needs the same shell-quoting as any other
	// interpolated flag value.
	inst := &Instance{Program: "claude"}
	got := inst.buildLaunchCommand("abc`touch /tmp/pwned`123")
	want := "claude --resume 'abc`touch /tmp/pwned`123'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildLaunchCommand_PlainProgramIgnoresClaudeFlags(t *testing.T) {
	// Proves the type boundary: a non-claude program receives no flag injection
	// even when all Instance fields that would produce claude flags are set.
	inst := &Instance{
		Program:            "aider",
		MCPServerURL:       "http://localhost:8543",
		AppendSystemPrompt: "be helpful",
		AllowedTools:       "read,write",
		PermissionMode:     "auto",
		AutoYes:            true,
		OneShot:            true,
		Prompt:             "do the thing",
	}
	got := inst.buildLaunchCommand("some-conv-id")
	if got != "aider" {
		t.Errorf("plain program should not receive any claude flags, got %q", got)
	}
}

func TestBuildLaunchCommand_CLIFlagsAreShellQuoted(t *testing.T) {
	// CLIFlags comes from the client-supplied cli_flags RPC field and must be
	// shell-quoted to prevent injection. Each whitespace-delimited token is quoted
	// individually so that multi-token flag strings (--flag1 --flag2=val) still work.
	inst := &Instance{
		Program:  "claude",
		CLIFlags: "--foo --bar='; evil shell injection'",
	}
	got := inst.buildLaunchCommand("")
	// The injection payload must not appear unquoted.
	if strings.Contains(got, "; evil shell injection") {
		t.Errorf("shell injection survived quoting: %s", got)
	}
	// Each token must be present in its quoted form.
	if !strings.Contains(got, shellQuote("--foo")) {
		t.Errorf("--foo not found quoted in: %s", got)
	}
}

// promptFileRefRegex extracts the temp file path from a promptArg-generated
// `"$(cat '<path>')"` command-substitution fragment.
var promptFileRefRegex = regexp.MustCompile(`\$\(cat '([^']+)'\)`)

// withShortPromptFileCleanupDelay swaps promptFileCleanupDelay for a short
// duration for the lifetime of a test, so the background cleanup goroutine
// started by promptArg doesn't have to sleep the real 30s default while tests
// run. Restores the original value on cleanup.
func withShortPromptFileCleanupDelay(t *testing.T) {
	t.Helper()
	orig := promptFileCleanupDelay
	promptFileCleanupDelay = 10 * time.Millisecond
	t.Cleanup(func() { promptFileCleanupDelay = orig })
}

func TestBuildClaudeCommand_LargePromptUsesTempFileNotInline(t *testing.T) {
	// Regression test for the review-gate spawn bug: BacklogLifecycle kept
	// re-spawning the identical review session every ~8 minutes and tmux
	// rejected every single attempt with "command too long" because the large
	// review prompt (big description + many verbose acceptance criteria) was
	// embedded directly in the tmux new-session command string. Empirically,
	// tmux's own command-length limit sits between 16000 and 16500 bytes for
	// the *entire* new-session command -- so a large prompt embedded inline
	// blows that budget outright, no matter how it's quoted.
	withShortPromptFileCleanupDelay(t)

	// Build a prompt shaped like the real trigger: a description plus many
	// acceptance criteria each carrying a verbose implementation note, well
	// past both maxInlinePromptBytes and the ~16KB tmux limit.
	var sb strings.Builder
	sb.WriteString("--- BACKLOG ITEM DATA ---\nRich File Browser\n")
	for n := 0; n < 40; n++ {
		sb.WriteString(strings.Repeat("x", 500))
		sb.WriteString("\n")
	}
	prompt := sb.String()
	if len(prompt) <= maxInlinePromptBytes {
		t.Fatalf("test setup bug: prompt (%d bytes) must exceed maxInlinePromptBytes (%d)", len(prompt), maxInlinePromptBytes)
	}
	if len(prompt) < 16*1024 {
		t.Fatalf("test setup bug: prompt (%d bytes) should exceed the ~16KB tmux command-length limit this regression test guards against", len(prompt))
	}

	inst := &Instance{Program: "claude", OneShot: true, Prompt: prompt}
	got := inst.buildLaunchCommand("")

	// The whole point of the fix: the assembled command handed to tmux must
	// stay well clear of tmux's ~16KB new-session command-length limit,
	// regardless of how large the prompt is.
	const safeCommandBudget = 8000
	if len(got) > safeCommandBudget {
		t.Errorf("assembled command is %d bytes, want under %d (tmux's own limit sits ~16000-16500 bytes) -- large prompt was not routed through a temp file: %s", len(got), safeCommandBudget, got)
	}
	if strings.Contains(got, prompt) {
		t.Errorf("large prompt was embedded inline instead of via a temp file: %s", got)
	}

	m := promptFileRefRegex.FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("expected a $(cat '<path>') command substitution in command, got: %s", got)
	}
	path := m[1]
	t.Cleanup(func() { _ = os.Remove(path) })

	// Prove the shell will receive the full, unmodified prompt at exec time.
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read temp prompt file %q: %v", path, err)
	}
	if string(written) != prompt {
		t.Errorf("temp prompt file content does not match original prompt exactly (got %d bytes, want %d bytes)", len(written), len(prompt))
	}
}

func TestBuildClaudeCommand_LargePromptTempFileIsCleanedUpAfterDelay(t *testing.T) {
	withShortPromptFileCleanupDelay(t)

	prompt := strings.Repeat("y", maxInlinePromptBytes+1)
	inst := &Instance{Program: "claude", OneShot: true, Prompt: prompt}
	got := inst.buildLaunchCommand("")

	m := promptFileRefRegex.FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("expected a $(cat '<path>') command substitution in command, got: %s", got)
	}
	path := m[1]

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return // cleaned up as expected
		}
		if time.Now().After(deadline) {
			_ = os.Remove(path)
			t.Fatalf("temp prompt file %q was not cleaned up within the expected delay", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestBuildClaudeCommand_LargePromptTempFileContentIsExactExcludingTrailingNewline(t *testing.T) {
	// Documents an accepted, deliberate caveat of the temp-file/command-
	// substitution path (see promptArg's doc comment): POSIX command
	// substitution strips ALL trailing newlines from its output, so a prompt
	// ending in "\n" is delivered to claude with that trailing newline gone.
	// This asserts the caveat is exactly what's documented -- content
	// otherwise fully intact, only trailing newlines affected -- so a future
	// change that silently starts corrupting more than trailing whitespace
	// (the actual bug class this whole fix targets) fails this test.
	withShortPromptFileCleanupDelay(t)

	prompt := strings.Repeat("w", maxInlinePromptBytes+1) + "\n\n"
	inst := &Instance{Program: "claude", OneShot: true, Prompt: prompt}
	got := inst.buildLaunchCommand("")

	m := promptFileRefRegex.FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("expected a $(cat '<path>') command substitution in command, got: %s", got)
	}
	path := m[1]
	t.Cleanup(func() { _ = os.Remove(path) })

	// The file on disk must be byte-identical to the original prompt --
	// promptArg itself performs no trimming. Any stripping happens later,
	// only when a shell evaluates $(cat ...), which this test does not do.
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read temp prompt file %q: %v", path, err)
	}
	if string(written) != prompt {
		t.Errorf("promptArg must write the prompt to disk unmodified (trimming, if any, happens only in the $(cat ...) shell evaluation, not here); got %d bytes, want %d bytes", len(written), len(prompt))
	}
}

func TestBuildClaudeCommand_PromptJustUnderThresholdStaysInline(t *testing.T) {
	// Boundary check: a prompt one byte under the threshold must keep the
	// existing (pre-fix) inline shell-quoted form.
	prompt := strings.Repeat("z", maxInlinePromptBytes-1)
	inst := &Instance{Program: "claude", Prompt: prompt}
	got := inst.buildLaunchCommand("")
	want := "claude -- " + shellQuote(prompt)
	if got != want {
		t.Errorf("prompt under threshold should be inlined verbatim; got a %d-byte command, want a %d-byte command", len(got), len(want))
	}
}

func TestClaudeMCPConfigArgs_HTTPFormat(t *testing.T) {
	inst := &Instance{
		Program:      "claude",
		MCPServerURL: "http://localhost:8543/mcp",
		UUID:         "test-uuid-123",
	}
	flag, val := inst.claudeMCPConfigArgs()
	if flag != "--mcp-config" {
		t.Errorf("flag = %q, want --mcp-config", flag)
	}
	// val is shell-quoted; strip the outer single quotes to get the raw JSON.
	if !strings.HasPrefix(val, "'") || !strings.HasSuffix(val, "'") {
		t.Fatalf("val should be single-quoted JSON, got %q", val)
	}
	raw := val[1 : len(val)-1]
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("val is not valid JSON: %v\nval=%q", err, raw)
	}
	servers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing mcpServers key in %q", raw)
	}
	entry, ok := servers["stapler-squad"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing stapler-squad entry in mcpServers")
	}
	if got := entry["type"]; got != "http" {
		t.Errorf("type = %q, want http", got)
	}
	if got := entry["url"]; got != "http://localhost:8543/mcp" {
		t.Errorf("url = %q, want http://localhost:8543/mcp", got)
	}
	headers, _ := entry["headers"].(map[string]interface{})
	if headers["X-Stapler-Session-UUID"] != "test-uuid-123" {
		t.Errorf("X-Stapler-Session-UUID = %q, want test-uuid-123", headers["X-Stapler-Session-UUID"])
	}
}
