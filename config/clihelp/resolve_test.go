package clihelp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolve_should_SkipEnvAssignmentsAndExpandTilde_When_EnvPrefixAndTildePath(t *testing.T) {
	got, err := Resolve(`FOO=1 "~/my bin/x" --y`, "/h")
	assert.Equal(t, ResolveOK, err)
	assert.Equal(t, Target{Name: "/h/my bin/x", IsAbsolute: true}, got)
}

func TestResolve_should_ReturnResolveError_When_EmptyOnlyEnvUnbalancedOrLeadingDash(t *testing.T) {
	cases := []struct {
		name, command string
		want          ResolveError
	}{
		{"empty", "", ResolveEmpty},
		{"blank", "  \t ", ResolveEmpty},
		{"only assignments", "A=1 B=2", ResolveOnlyEnvAssignments},
		{"unbalanced double", `claude "x`, ResolveUnbalancedQuote},
		{"unbalanced single", `'claude`, ResolveUnbalancedQuote},
		{"leading dash", "-x", ResolveInvalidToken},
		{"nul", "cla\x00ude", ResolveInvalidToken},
		{"newline in token", "\"a\nb\"", ResolveInvalidToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.command, "/h")
			assert.Equal(t, tc.want, err)
			assert.Equal(t, Target{}, got)
		})
	}
}

func TestResolve_should_KeepLiteralClaudeSemicolonAndNoArgs_When_ShellMetacharacters(t *testing.T) {
	got, err := Resolve("claude; rm -rf ~", "/h")
	assert.Equal(t, ResolveOK, err)
	assert.Equal(t, Target{Name: "claude;"}, got)
}

func TestResolve_should_ReturnRelativePath_When_DotSlashBinSlashDotDotOrTildeUser(t *testing.T) {
	for _, command := range []string{"./x", "bin/x", "../x", "~bob/x", "~bob", "."} {
		t.Run(command, func(t *testing.T) {
			_, err := Resolve(command, "/h")
			assert.Equal(t, ResolveRelativePath, err)
		})
	}
	_, err := Resolve("~/x", "")
	assert.Equal(t, ResolveRelativePath, err, "~ with unknown home cannot be expanded")
}

func TestResolve_should_ExpandTildeAndAcceptAbsoluteAndBare_When_ValidTokens(t *testing.T) {
	cases := map[string]Target{
		"~":          {Name: "/h", IsAbsolute: true},
		"~/bin/t":    {Name: "/h/bin/t", IsAbsolute: true},
		"/usr/bin/x": {Name: "/usr/bin/x", IsAbsolute: true},
		"claude":     {Name: "claude"},
	}
	for command, want := range cases {
		got, err := Resolve(command, "/h")
		assert.Equal(t, ResolveOK, err, command)
		assert.Equal(t, want, got, command)
	}
}

func TestResolve_should_FlagWrapper_When_EnvNpxOrBuiltinProxyCommand(t *testing.T) {
	commands := []string{
		"env -u CLAUDE_CODE_USE_BEDROCK ANTHROPIC_BASE_URL=http://localhost:47000 claude",
		"npx foo",
		"FOO=1 /usr/bin/env claude",
	}
	for _, command := range commands {
		got, err := Resolve(command, "/h")
		assert.Equal(t, ResolveOK, err, command)
		assert.True(t, got.Wrapper, command)
	}
	got, _ := Resolve("claude --x", "/h")
	assert.False(t, got.Wrapper)
}
