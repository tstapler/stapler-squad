package clihelp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

func FuzzParseHelp_should_NotPanicAndKeepNameInvariant_When_ArbitraryBytes(f *testing.F) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "help", "*.txt"))
	require.NoError(f, err)
	for _, path := range fixtures {
		b, err := os.ReadFile(path)
		require.NoError(f, err)
		if len(b) > 4096 {
			b = b[:4096]
		}
		f.Add(string(b))
	}
	f.Add("")
	f.Add("--[no-]\n-\b-\x1b[")
	f.Add(strings.Repeat("-", 3000))

	f.Fuzz(func(t *testing.T, text string) {
		flags := ParseHelp(text)

		require.NotNil(t, flags)
		require.LessOrEqual(t, len(flags), maxFlags)
		requireNameShapes(t, flags)
		for _, fl := range flags {
			require.True(t, utf8.ValidString(fl.Description))
			require.LessOrEqual(t, utf8.RuneCountInString(fl.Description), maxDescriptionChars)
		}
	})
}

func TestParseHelp_should_ReturnEmptyWithin100ms_When_256KBSingleLineOfDashes(t *testing.T) {
	input := strings.Repeat("-", 256*1024)

	start := time.Now()
	got := ParseHelp(input)

	assert.Empty(t, got)
	assert.Less(t, time.Since(start), wait.ScaleTimeout(100*time.Millisecond))
}

func TestParseHelp_should_ReturnWithin100ms_When_10kTinyLines(t *testing.T) {
	input := strings.Repeat("-a\n", 10_000)

	start := time.Now()
	got := ParseHelp(input)

	assert.LessOrEqual(t, len(got), maxFlags)
	assert.Less(t, time.Since(start), wait.ScaleTimeout(100*time.Millisecond))
}

func TestParseHelp_should_ReturnWithin100ms_When_NoNewlinesAtAll(t *testing.T) {
	input := strings.Repeat("--x  y ", 40_000)

	start := time.Now()
	got := ParseHelp(input)

	assert.Empty(t, got, "one giant line is over the line limit")
	assert.Less(t, time.Since(start), wait.ScaleTimeout(100*time.Millisecond))
}

// Allocation counts, unlike wall time, do not flake under load, so they catch
// a quadratic parser: doubling the input must stay well under 4x the allocs.
func TestParseHelp_should_ScaleLinearly_When_InputSizeDoubles(t *testing.T) {
	inputs := map[string]func(n int) string{
		"rejected flag-like lines": func(n int) string { return strings.Repeat("--x y z w\n", n) },
		"wrapped prose":            func(n int) string { return "  --a   d\n" + strings.Repeat("      more prose\n", n) },
		"ansi noise":               func(n int) string { return strings.Repeat("\x1b[1m-\b-\x1b[0m\n", n) },
	}
	for name, gen := range inputs {
		t.Run(name, func(t *testing.T) {
			small, large := gen(2000), gen(4000)

			allocsSmall := testing.AllocsPerRun(3, func() { ParseHelp(small) })
			allocsLarge := testing.AllocsPerRun(3, func() { ParseHelp(large) })

			assert.Less(t, allocsLarge, 3*allocsSmall+50,
				"allocs small=%v large=%v", allocsSmall, allocsLarge)
		})
	}
}
