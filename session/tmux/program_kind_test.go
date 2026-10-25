package tmux

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/launchcmd"
)

// legacyKind is the pre-typed check, applied to the rendered launch string.
func legacyKind(program string) ProgramKind {
	switch {
	case program == "claude":
		return KindClaude
	case strings.HasPrefix(program, "aider"):
		return KindAider
	case strings.HasPrefix(program, "gemini"):
		return KindGemini
	}
	return KindOther
}

func TestKindOfShellText_MatchesLegacyStringChecks(t *testing.T) {
	for _, program := range []string{"claude", "claude --resume x", "aider", "aider --yes", "gemini", "opencode", ""} {
		require.Equal(t, legacyKind(program), kindOfShellText(program), program)
	}
}

func TestProgramKindOf_ClassifiesByProgramBasename(t *testing.T) {
	uuid := "123e4567-e89b-12d3-a456-426614174000"
	env, err := launchcmd.NewEnvVar("FOO", "1")
	require.NoError(t, err)
	cases := []struct {
		name string
		cmd  launchcmd.LaunchCommand
		want ProgramKind
	}{
		{"bare claude", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("claude")}}, KindClaude},
		{"quoted literal claude", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Literal("claude")}}, KindClaude},
		{"quoted path with spaces to claude", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Literal("/opt/my tools/bin/claude")}}, KindClaude},
		{"claude-wrapper", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("claude-wrapper")}}, KindOther},
		{"aider absolute path", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("/usr/bin/aider")}}, KindAider},
		{"aider-chat is not aider", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("aider-chat")}}, KindOther},
		{"claude resume", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("claude"), launchcmd.MustFlag("--resume"), launchcmd.Word(uuid)}}, KindClaude},
		{"claude stderr log", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("claude")}, StderrAppend: "/tmp/e.log"}, KindClaude},
		{"env prefix claude", launchcmd.ExecCommand{Env: []launchcmd.EnvVar{env}, Argv: []launchcmd.Arg{launchcmd.Word("claude")}}, KindClaude},
		{"absolute path claude", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("/usr/bin/claude")}}, KindClaude},
		{"bare aider", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("aider")}}, KindAider},
		{"aider with flag", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("aider"), launchcmd.MustFlag("--yes-always")}}, KindAider},
		{"quoted aider", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Literal("aider")}}, KindAider},
		{"gemini", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("gemini")}}, KindGemini},
		{"custom program", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("opencode")}}, KindOther},
		{"hostile quoting", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Literal("claude'; touch /tmp/x; '")}}, KindOther},
		{"aider with hostile arg", launchcmd.ExecCommand{Argv: []launchcmd.Arg{launchcmd.Word("aider"), launchcmd.Literal("; rm -rf /")}}, KindAider},
		{"empty", launchcmd.ExecCommand{}, KindOther},
		{"nil", nil, KindOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ProgramKindOf(tc.cmd))
		})
	}
}
