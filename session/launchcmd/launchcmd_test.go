package launchcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// shellArgv is the oracle: it asks a real /bin/sh to word-split cmd and
// returns the words it produced, so a renderer is judged by what a shell
// does with its output rather than by string shape.
func shellArgv(t *testing.T, cmd string) []string {
	t.Helper()
	out, err := safeexec.CommandContext(context.Background(), "/bin/sh", "-c", "printf '%s\\0' "+cmd).Output()
	require.NoError(t, err, "shell rejected %q", cmd)
	out = bytes.TrimSuffix(out, []byte{0})
	if len(out) == 0 {
		return nil
	}
	return strings.Split(string(out), "\x00")
}

func exec1(args ...Arg) ExecCommand { return ExecCommand{Argv: args} }

func TestQuote_MatchesLegacyShellQuoteCases(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, in, want string }{
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
			t.Parallel()
			assert.Equal(t, tc.want, Quote(tc.in))
		})
	}
}

func TestExecCommand_ShellString_Golden(t *testing.T) {
	t.Parallel()
	id, err := ParseConversationID("A364F374-4C68-4DD0-A836-770A191973E5")
	require.NoError(t, err)
	cmd := exec1(Literal("claude"), MustFlag("--resume"), Literal(id.String()))
	assert.Equal(t, `'claude' --resume 'a364f374-4c68-4dd0-a836-770a191973e5'`, cmd.ShellString())

	env, err := NewEnvVar("OPENCODE_CONFIG", "/tmp/it's.json")
	require.NoError(t, err)
	withEnv := ExecCommand{Env: []EnvVar{env}, Argv: []Arg{Literal("opencode")}}
	assert.Equal(t, `OPENCODE_CONFIG='/tmp/it'\''s.json' 'opencode'`, withEnv.ShellString())
}

func TestExecCommand_ShellString_ShellParsesBackToTheSameArgv(t *testing.T) {
	t.Parallel()
	hostile := []string{
		"", " ", "two words", "it's", `say "hi"`, `back\slash`, "$HOME", "$(touch /tmp/pwned)",
		"`whoami`", "a;b", "a && b", "a|b", "a>b", "*", "~", "#comment", "!", "line1\nline2", "héllo ✓",
		"true; touch /tmp/pwned", "-n", "--",
	}
	args := make([]Arg, 0, len(hostile)+3)
	args = append(args, Literal("prog"))
	want := make([]string, 0, len(hostile)+3)
	want = append(want, "prog")
	for _, h := range hostile {
		args = append(args, Literal(h))
		want = append(want, h)
	}
	args = append(args, MustFlag("--model"), Literal("sonnet"))
	want = append(want, "--model", "sonnet")

	got := shellArgv(t, exec1(args...).ShellString())
	assert.Equal(t, want, got)
}

func TestExecCommand_ShellString_AgreesWithHandWrittenLegacyStrings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		legacy string
		cmd    ExecCommand
	}{
		{"claude resume", `claude --resume 'abc'`,
			exec1(Literal("claude"), MustFlag("--resume"), Literal("abc"))},
		{"program with flags", `claude --model sonnet --resume 'abc'`,
			exec1(Literal("claude"), Literal("--model"), Literal("sonnet"), MustFlag("--resume"), Literal("abc"))},
		{"quoted prompt", `claude -- 'fix the bug; it'\''s urgent'`,
			exec1(Literal("claude"), Literal("--"), Literal("fix the bug; it's urgent"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, shellArgv(t, tc.legacy), shellArgv(t, tc.cmd.ShellString()))
		})
	}
}

func TestFileContents_ShellAndArgsAgree(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "prompt.txt")
	require.NoError(t, os.WriteFile(path, []byte("multi word\nprompt with 'quotes' and $VARS\n\n"), 0o600))
	cmd := exec1(Literal("claude"), Literal("--"), FileContents(path))

	viaShell := shellArgv(t, cmd.ShellString())
	viaArgs, err := cmd.Args(os.ReadFile)
	require.NoError(t, err)

	assert.Equal(t, "multi word\nprompt with 'quotes' and $VARS", viaShell[2])
	assert.Equal(t, viaShell[1:], viaArgs, "Args drops argv[0] only; trailing newlines trimmed like $(cat)")
}

func TestExecCommand_Args(t *testing.T) {
	t.Parallel()
	t.Run("literals and flags pass through", func(t *testing.T) {
		t.Parallel()
		got, err := exec1(Literal("p"), MustFlag("-p"), Literal("a b")).Args(nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"-p", "a b"}, got)
	})
	t.Run("oversize file argument is rejected", func(t *testing.T) {
		t.Parallel()
		big := func(string) ([]byte, error) { return bytes.Repeat([]byte("x"), MaxArgBytes), nil }
		_, err := exec1(Literal("p"), FileContents("/f")).Args(big)
		assert.ErrorIs(t, err, ErrArgTooLarge)
	})
	t.Run("oversize literal is rejected", func(t *testing.T) {
		t.Parallel()
		_, err := exec1(Literal("p"), Literal(strings.Repeat("x", MaxArgBytes))).Args(nil)
		assert.ErrorIs(t, err, ErrArgTooLarge)
	})
	t.Run("unreadable file is a wrapped error, not an empty arg", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("boom")
		_, err := exec1(Literal("p"), FileContents("/gone")).Args(func(string) ([]byte, error) { return nil, boom })
		require.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "/gone")
	})
	t.Run("empty argv", func(t *testing.T) {
		t.Parallel()
		_, err := ExecCommand{}.Args(nil)
		assert.Error(t, err)
	})
}

func TestExecCommand_Program(t *testing.T) {
	t.Parallel()
	p, ok := exec1(Literal("claude"), Literal("x")).Program()
	assert.True(t, ok)
	assert.Equal(t, "claude", p)
	_, ok = ExecCommand{}.Program()
	assert.False(t, ok)
	_, ok = exec1(FileContents("/f")).Program()
	assert.False(t, ok)
}

func TestShellScript(t *testing.T) {
	t.Parallel()
	text, err := NewShellText("cd ~/repo && exec claude 2>>'/tmp/log'")
	require.NoError(t, err)
	assert.Equal(t, "cd ~/repo && exec claude 2>>'/tmp/log'", ShellScript{Text: text}.ShellString())
}

func TestConstructorsRejectInvalidInput(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "resume", "---", "--a b", "--a;b", "-", "--$x", "--é"} {
		_, err := NewFlag(bad)
		assert.Error(t, err, "flag %q", bad)
	}
	for _, good := range []string{"-p", "--", "--resume", "--allowedTools", "--mcp-config"} {
		_, err := NewFlag(good)
		assert.NoError(t, err, "flag %q", good)
	}
	for _, bad := range []string{"", "1A", "A-B", "A B", "A=B"} {
		_, err := NewEnvVar(bad, "v")
		assert.Error(t, err, "env key %q", bad)
	}
	for _, bad := range []string{"", "  \n", "a\x00b"} {
		_, err := NewShellText(bad)
		assert.Error(t, err, "shell text %q", bad)
	}
	for _, bad := range []string{"", "not-a-uuid", "a364f374-4c68-4dd0-a836-770a191973e", "a364f374-4c68-4dd0-a836-770a191973e5; ls"} {
		_, err := ParseConversationID(bad)
		assert.Error(t, err, "uuid %q", bad)
	}
	assert.Panics(t, func() { MustFlag("bad flag") })
}

func TestParseProgram_ResolvesQuotingLikeAShell(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		in      string
		env     map[string]string
		argv    []string
		needsSh bool
	}{
		{name: "bare", in: "claude", argv: []string{"claude"}},
		{name: "flags", in: "claude --model sonnet", argv: []string{"claude", "--model", "sonnet"}},
		{name: "double quoted", in: `claude --append-system-prompt "a b"`, argv: []string{"claude", "--append-system-prompt", "a b"}},
		{name: "single quoted", in: `claude -p 'it is'`, argv: []string{"claude", "-p", "it is"}},
		{name: "escaped space", in: `claude a\ b`, argv: []string{"claude", "a b"}},
		{name: "env prefix", in: `ANTHROPIC_BASE_URL=http://x:1 claude`, env: map[string]string{"ANTHROPIC_BASE_URL": "http://x:1"}, argv: []string{"claude"}},
		{name: "env wrapper", in: "env -u VAR claude", argv: []string{"env", "-u", "VAR", "claude"}},
		{name: "variable", in: "$HOME/bin/claude", needsSh: true},
		{name: "quoted variable", in: `claude "$HOME"`, needsSh: true},
		{name: "command substitution", in: "claude $(cat x)", needsSh: true},
		{name: "glob", in: "claude *.md", needsSh: true},
		{name: "tilde", in: "~/bin/claude", needsSh: true},
		{name: "brace", in: "claude {a,b}", needsSh: true},
		{name: "pipe", in: "claude | tee x", needsSh: true},
		{name: "second command", in: "claude; touch /tmp/pwned", needsSh: true},
		{name: "and chain", in: "cd x && claude", needsSh: true},
		{name: "redirect", in: "claude 2>/dev/null", needsSh: true},
		{name: "background", in: "claude &", needsSh: true},
		{name: "syntax error", in: `claude "unterminated`, needsSh: true},
		{name: "empty", in: "", needsSh: true},
		{name: "only env", in: "A=1", needsSh: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseProgram(tc.in)
			if tc.needsSh {
				assert.ErrorIs(t, err, ErrNeedsShell)
				return
			}
			require.NoError(t, err)
			wantEnv := map[string]string{}
			for _, e := range got.Env {
				wantEnv[e.Key()] = e.Value
			}
			assert.Equal(t, orEmpty(tc.env), wantEnv)
			args, err := got.Args(nil)
			require.NoError(t, err)
			prog, ok := got.Program()
			require.True(t, ok)
			assert.Equal(t, tc.argv, append([]string{prog}, args...))
			// What a real shell makes of the original text agrees.
			assert.Equal(t, shellArgv(t, tc.in), shellArgv(t, got.ShellString()))
		})
	}
}

func orEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func TestWord_QuotesOnlyWhenNeeded(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "--model", exec1(Word("--model")).ShellString())
	assert.Equal(t, "/usr/bin/claude", exec1(Word("/usr/bin/claude")).ShellString())
	assert.Equal(t, `'a b'`, exec1(Word("a b")).ShellString())
	assert.Equal(t, `'$HOME'`, exec1(Word("$HOME")).ShellString())
	assert.Equal(t, `''`, exec1(Word("")).ShellString())
}

func TestRawShellAndStderr(t *testing.T) {
	t.Parallel()
	cmd := ExecCommand{Argv: []Arg{RawShell("$HOME/bin/claude"), Word("-x")}, StderrAppend: "/tmp/it's.log"}
	assert.True(t, cmd.RequiresShell())
	assert.Equal(t, `$HOME/bin/claude -x 2>>'/tmp/it'\''s.log'`, cmd.ShellString())
	_, err := cmd.Args(nil)
	assert.ErrorIs(t, err, ErrRequiresShell)
	assert.False(t, exec1(Word("p")).RequiresShell())
}
