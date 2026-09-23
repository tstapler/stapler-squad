package clihelp

// Fixtures under testdata/help/ were captured with stdout+stderr merged and
// NO_COLOR=1 PAGER=cat MANPAGER=cat; see Task 1.1.3a in the cli-flag-discovery plan.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var flagNameShape = regexp.MustCompile(`^--?[A-Za-z0-9][\w-]*$`)

func readHelpFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "help", name))
	require.NoError(t, err)
	return string(b)
}

func findFlag(t *testing.T, flags []Flag, name string) Flag {
	t.Helper()
	for _, f := range flags {
		if f.Name == name {
			return f
		}
	}
	require.Failf(t, "flag not found", "no flag named %q in %d parsed flags", name, len(flags))
	return Flag{}
}

func requireNameShapes(t *testing.T, flags []Flag) {
	t.Helper()
	for _, f := range flags {
		assert.Regexp(t, flagNameShape, f.Name)
		if f.Short != "" {
			assert.Regexp(t, flagNameShape, f.Short)
		}
		for _, a := range f.Aliases {
			assert.Regexp(t, flagNameShape, a)
		}
	}
}

func TestParseHelp_should_ReturnAssigneeShortAndValue_When_GhPrListFixture(t *testing.T) {
	flags := ParseHelp(readHelpFixture(t, "gh-pr-list-2.86.0.txt"))

	assert.Equal(t, Flag{Name: "--assignee", Short: "-a", TakesValue: true, Description: "Filter by assignee"},
		findFlag(t, flags, "--assignee"))
	draft := findFlag(t, flags, "--draft")
	assert.Equal(t, "-d", draft.Short)
	assert.False(t, draft.TakesValue)
	assert.Equal(t, "-R", findFlag(t, flags, "--repo").Short)
	assert.True(t, findFlag(t, flags, "--repo").TakesValue)
}

func TestParseHelp_should_ReturnAliasOnOneEntry_When_ClaudeFixtureAllowedTools(t *testing.T) {
	flags := ParseHelp(readHelpFixture(t, "claude-2.1.278.txt"))

	got := findFlag(t, flags, "--allowedTools")
	assert.Equal(t, []string{"--allowed-tools"}, got.Aliases)
	assert.True(t, got.TakesValue)
	assert.NotEmpty(t, got.Description, "description sits on the wrapped line after the flag")
	for _, f := range flags {
		assert.NotEqual(t, "--allowed-tools", f.Name, "long-long alias must not be a separate entry")
	}
	bg := findFlag(t, flags, "--bg")
	assert.Equal(t, []string{"--background"}, bg.Aliases)
	assert.False(t, bg.TakesValue)
}

func TestParseHelp_should_ParseModelTakesValue_When_AiderFixture(t *testing.T) {
	flags := ParseHelp(readHelpFixture(t, "aider-0.78.0.txt"))

	model := findFlag(t, flags, "--model")
	assert.True(t, model.TakesValue)
	assert.Equal(t, "Specify the model to use for the main chat", model.Description,
		"[env var: ...] tail wrapped across lines is stripped")
	assert.Equal(t, []string{"--models"}, findFlag(t, flags, "--list-models").Aliases)
	assert.Equal(t, []string{"--no-verify-ssl"}, findFlag(t, flags, "--verify-ssl").Aliases)
	assert.Equal(t, []string{"--chat-mode"}, findFlag(t, flags, "--edit-format").Aliases)
	assert.Equal(t, "-h", findFlag(t, flags, "--help").Short)
	for _, f := range flags {
		assert.NotEqual(t, "--no-verify-ssl", f.Name)
	}
}

func TestParseHelp_should_MeetMinCountAndNameRegex_When_RgUvGeminiAgyFixtures(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		minCount int
		wantFlag Flag
	}{
		{"rg clap long help", "rg-15.2.0.txt", 90, Flag{Name: "--regexp", Short: "-e", TakesValue: true}},
		{"uv clap", "uv-0.9.28.txt", 15, Flag{Name: "--cache-dir", TakesValue: true, Description: "Path to the cache directory"}},
		{"gemini yargs", "gemini-0.43.0.txt", 25, Flag{Name: "--model", Short: "-m", TakesValue: true}},
		{"agy go flag", "agy-1.2.7.txt", 20, Flag{Name: "--continue", Description: "Continue the most recent conversation"}},
		{"gh top level", "gh-2.86.0.txt", 2, Flag{Name: "--version", Description: "Show gh version"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := ParseHelp(readHelpFixture(t, tt.fixture))

			assert.GreaterOrEqual(t, len(flags), tt.minCount)
			requireNameShapes(t, flags)
			got := findFlag(t, flags, tt.wantFlag.Name)
			assert.Equal(t, tt.wantFlag.Short, got.Short)
			assert.Equal(t, tt.wantFlag.TakesValue, got.TakesValue)
			if tt.wantFlag.Description != "" {
				assert.Equal(t, tt.wantFlag.Description, got.Description)
			}
		})
	}
}

func TestParseHelp_should_NotTreatProseInRgDescriptionsAsFlags_When_RgFixture(t *testing.T) {
	flags := ParseHelp(readHelpFixture(t, "rg-15.2.0.txt"))

	seen := map[string]bool{}
	for _, f := range flags {
		assert.False(t, seen[f.Name], "duplicate flag %s", f.Name)
		seen[f.Name] = true
		assert.NotContains(t, f.Name, "/")
	}
	assert.Equal(t, "-U", findFlag(t, flags, "--multiline").Short)
	assert.Empty(t, findFlag(t, flags, "--multiline-dotall").Short)
}

func TestParseHelp_should_StripAnsiAndOverstrike_When_ColouredVariantFixture(t *testing.T) {
	plain := ParseHelp(readHelpFixture(t, "gh-2.86.0.txt"))
	coloured := ParseHelp(readHelpFixture(t, "gh-2.86.0-ansi.txt"))

	require.NotEmpty(t, plain)
	assert.Equal(t, plain, coloured)

	tests := []struct{ name, in, want string }{
		{"csi", "\x1b[1;31m--x\x1b[0m  desc", "--x  desc"},
		{"osc bel", "\x1b]0;title\x07--x", "--x"},
		{"osc st", "\x1b]8;;http://x\x1b\\--x", "--x"},
		{"carriage return", "--x  desc\r", "--x  desc"},
		{"overstrike", "-\b--\b-verbose", "--verbose"},
		{"truncated escape", "--x\x1b[", "--x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripANSI(tt.in))
		})
	}
}

func TestParseHelp_should_ReturnEmptySliceAndNoError_When_TmuxUsageErrorOrGitFixture(t *testing.T) {
	tests := []struct{ name, text string }{
		{"tmux usage error", readHelpFixture(t, "tmux-3.6a.txt")},
		{"git command list", readHelpFixture(t, "git-2.53.0.txt")},
		{"empty input", ""},
		{"prose only", "just some words\nand more words"},
		{"lone dashes", "-\n--\n---\n- - -"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseHelp(tt.text)

			assert.NotNil(t, got)
			assert.Empty(t, got)
		})
	}
}

func TestParseHelp_should_ExpandNoBracketForm_When_LongFlagHasBracketNo(t *testing.T) {
	flags := ParseHelp("  --[no-]color   Colorize output\n  -q, --[no-]quiet\n      Be quiet\n")

	require.Len(t, flags, 2)
	assert.Equal(t, Flag{Name: "--color", Aliases: []string{"--no-color"}, Description: "Colorize output"}, flags[0])
	assert.Equal(t, Flag{Name: "--quiet", Short: "-q", Aliases: []string{"--no-quiet"}, Description: "Be quiet"}, flags[1])
}

func TestParseHelp_should_RecogniseValueHintStyles_When_CommonHelpFormats(t *testing.T) {
	tests := []struct {
		name string
		line string
		want Flag
	}{
		{"gnu short long angle", "  -o, --output <file>   Write to file", Flag{Name: "--output", Short: "-o", TakesValue: true, Description: "Write to file"}},
		{"equals hint", "  --level=<n>   Set level", Flag{Name: "--level", TakesValue: true, Description: "Set level"}},
		{"optional equals hint", "  --color[=WHEN]   Colourize", Flag{Name: "--color", TakesValue: true, Description: "Colourize"}},
		{"clap short hint then long", "  -e PATTERN, --regexp=PATTERN", Flag{Name: "--regexp", Short: "-e", TakesValue: true}},
		{"cobra type word", "  -L, --limit int   Max items (default 30)", Flag{Name: "--limit", Short: "-L", TakesValue: true, Description: "Max items (default 30)"}},
		{"boolean flag", "  -d, --draft   Draft only", Flag{Name: "--draft", Short: "-d", Description: "Draft only"}},
		{"go flag style", "  -timeout duration\n    \tHow long to wait", Flag{Name: "-timeout", TakesValue: true, Description: "How long to wait"}},
		{"count marker", "  -q, --quiet...   Less output", Flag{Name: "--quiet", Short: "-q", Description: "Less output"}},
		{"short only", "  -c   Short alias for --continue", Flag{Name: "-c", Description: "Short alias for --continue"}},
		{"yargs string tag", "  -m, --model   Model  [string]", Flag{Name: "--model", Short: "-m", TakesValue: true, Description: "Model [string]"}},
		{"tab separated", "  --tabbed\tDescription after tab", Flag{Name: "--tabbed", Description: "Description after tab"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := ParseHelp(tt.line + "\n")

			require.Len(t, flags, 1)
			assert.Equal(t, tt.want, flags[0])
		})
	}
}

func TestParseHelp_should_RejectLine_When_NameShapeOrIndentIsWrong(t *testing.T) {
	tests := []struct{ name, text string }{
		{"slash joined forms", "  -e/--regexp flag, then things"},
		{"dot short name", "  -., --hidden   Show hidden"},
		{"deeply indented", "                 --deep   Not a candidate"},
		{"prose after name", "  --pre flag will disable this behavior"},
		{"trailing punctuation hint", "  --pre-glob flag."},
		{"double dash only", "  --   end of options"},
		{"non-ascii name", "  --é   accents"},
		{"usage line bracketed", "  [--foo | --no-foo]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Empty(t, ParseHelp(tt.text+"\n"))
		})
	}
}

func TestParseHelp_should_JoinWrappedDescription_When_ContinuationIsIndentedDeeper(t *testing.T) {
	text := "  --long <v>   First part\n" +
		"               second part\n" +
		"\n" +
		"               separate paragraph is dropped\n" +
		"  --next   Next flag\n"

	flags := ParseHelp(text)

	require.Len(t, flags, 2)
	assert.Equal(t, "First part second part", flags[0].Description)
	assert.Equal(t, "Next flag", flags[1].Description)
}

func TestParseHelp_should_PreferEntryWithDescription_When_FlagAppearsTwice(t *testing.T) {
	text := "Usage:\n  --dup <v>\nOptions:\n  --dup <v>   Described\n  --dup   Later\n"

	flags := ParseHelp(text)

	require.Len(t, flags, 1)
	assert.Equal(t, "Described", flags[0].Description)
	assert.True(t, flags[0].TakesValue)
}

func TestParseHelp_should_CapFlagsAt500AndDescriptionAt300_When_HugeInput(t *testing.T) {
	var b strings.Builder
	for i := range 1000 {
		fmt.Fprintf(&b, "  --flag-%d   %s\n", i, strings.Repeat("é", 1000))
	}

	flags := ParseHelp(b.String())

	require.Len(t, flags, maxFlags)
	for _, f := range flags {
		assert.Len(t, []rune(f.Description), maxDescriptionChars)
	}
}

func TestParseHelp_should_SkipLine_When_LineExceedsLimit(t *testing.T) {
	long := "  --big   " + strings.Repeat("x", maxLineBytes)

	flags := ParseHelp(long + "\n  --ok   fine\n")

	require.Len(t, flags, 1)
	assert.Equal(t, "--ok", flags[0].Name)
}

// TestParseHelp_should_ParseAtLeast3Of5ValueRiskTargets_When_GateG1 is the
// Gate G1 kill-criterion check: continue into M2 only if >=3 of the five
// value-risk targets yield more than 0 flags. Run with -v to see the counts.
func TestParseHelp_should_ParseAtLeast3Of5ValueRiskTargets_When_GateG1(t *testing.T) {
	targets := []struct{ tool, fixture string }{
		{"claude", "claude-2.1.278.txt"},
		{"aider", "aider-0.78.0.txt"},
		{"gemini", "gemini-0.43.0.txt"},
		{"agy", "agy-1.2.7.txt"},
		{"gh", "gh-2.86.0.txt"},
		{"gh pr list", "gh-pr-list-2.86.0.txt"},
	}
	passing := 0
	for _, tg := range targets {
		n := len(ParseHelp(readHelpFixture(t, tg.fixture)))
		t.Logf("G1 %-11s %3d flags parsed", tg.tool, n)
		if n > 0 && tg.tool != "gh pr list" {
			passing++
		}
	}
	t.Logf("G1: %d of 5 value-risk targets parsed >0 flags", passing)
	assert.GreaterOrEqual(t, passing, 3)
}
