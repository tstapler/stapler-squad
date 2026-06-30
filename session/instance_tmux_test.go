package session

import (
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
	expected := "claude --resume conv-abc123"
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
