package session

import (
	"strings"
	"testing"
)

func TestIsClaude(t *testing.T) {
	cases := []struct {
		program string
		want    bool
	}{
		{"claude", true},
		{"/usr/local/bin/claude", true},
		{"env -u SOME_VAR claude", true},  // env wrapper — second token matches
		{"env claude --flag", true},        // env prefix
		{"claude-squad", false},            // basename is "claude-squad", not "claude"
		{"myclaudeapp", false},             // basename contains "claude" but is not "claude"
		{"/claude/bin/aider", false},       // "claude" is a directory component, not the binary
		{"aider", false},
		{"", false},
		{"Claude", false},                  // case-sensitive: capital C does not match
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
