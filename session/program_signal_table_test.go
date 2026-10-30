package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/binaries"
	"github.com/tstapler/stapler-squad/session/detection/dtypes"
)

func idleDescriptions(p dtypes.StatusPatterns) map[string]bool {
	out := map[string]bool{}
	for _, pat := range p.Idle {
		out[pat.Description] = true
	}
	return out
}

func TestProgramSignalTable_Should_CoverEverySupportedProgram(t *testing.T) {
	t.Parallel()
	want := map[string]SignalKind{
		"claude": SignalHook, "agy": SignalScreen, "aider": SignalScreen,
		"gemini": SignalNone, "opencode": SignalNone, "pi": SignalNone,
	}
	require.Len(t, ProgramSignalTable, len(want))
	for program, kind := range want {
		assert.Equal(t, kind, ProgramSignalFor(program).Kind, program)
	}
}

func TestProgramSignalTable_Should_PinIdleContextsToRealPatternDescriptions(t *testing.T) {
	t.Parallel()
	claude := idleDescriptions(binaries.NewClaudeDetector().Patterns())
	for ctx := range ProgramSignalTable["claude"].IdleContexts {
		assert.True(t, claude[ctx], "claude row context %q is not a claude.go Idle description", ctx)
	}
	agy := idleDescriptions(binaries.NewAgyDetector().Patterns())
	assert.True(t, ProgramSignalTable["agy"].IdleContexts[agyIdleReadlineContext])
	for ctx := range ProgramSignalTable["agy"].IdleContexts {
		assert.True(t, agy[ctx] || SafeIdleStatusContexts[ctx], "agy row context %q is in neither agy.go nor the safe-idle allowlist", ctx)
	}
	assert.True(t, agy[agyIdleReadlineContext], "agy.go no longer describes agy_idle_readline as %q", agyIdleReadlineContext)
	assert.False(t, ProgramSignalTable["agy"].IdleContexts["Agy INSERT mode indicator in status bar (shared with Gemini idle TUI)"],
		"agy [INSERT] is ambiguous with a vim status line")
}

// A program row of none or prompt-gated screen holds only while its detector has no Idle
// pattern; adding one must force a revisit of the row.
func TestProgramSignalTable_Should_FailWhenNoneRowsGainIdlePatterns(t *testing.T) {
	t.Parallel()
	detectors := map[string]dtypes.BinaryDetector{
		"aider": binaries.NewAiderDetector(), "gemini": binaries.NewGeminiDetector(),
		"opencode": binaries.NewOpencodeDetector(), "pi": binaries.NewPiDetector(),
	}
	for program, d := range detectors {
		assert.Empty(t, d.Patterns().Idle, "%s gained an Idle pattern: update its ProgramSignalTable row", program)
	}
}

func TestCannotDetect_Should_BeTrueOnlyForNoneRowsAndUnknownPrograms(t *testing.T) {
	t.Parallel()
	for program, want := range map[string]bool{
		"claude": false, "/usr/local/bin/claude --resume": false, "agy": false, "aider --model x": false,
		"gemini": true, "opencode": true, "pi": true, "mystery-agent": true, "": true,
	} {
		assert.Equal(t, want, CannotDetect(program), program)
	}
}

type paneExpect struct {
	detected bool
	kind     TurnEndKind
}

// Fixtures are synthetic or captured panes under testdata/panes/<program>/. A name starting
// with idle_ is an idle pane; everything else (dialogs, permission prompts) must not be read as a turn end.
var paneExpectations = map[string]paneExpect{
	"claude/idle_prose_question.txt": {true, TurnEndKindQuestion},
	"claude/idle_finished.txt":       {true, TurnEndKindFinished},
	"agy/idle_finished.txt":          {true, TurnEndKindFinished},
	"aider/idle_prose_question.txt":  {true, TurnEndKindFinished}, // non-Claude: Finished until a labelled corpus exists
}

func TestScreenTurnEnd_Should_ReportPerProgramCoverageOverFixtureCorpus(t *testing.T) {
	t.Parallel()
	detector := detection.NewStatusDetector()
	files, err := filepath.Glob(filepath.Join("testdata", "panes", "*", "*.txt"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	detected := map[string]int{}
	total := map[string]int{}
	for _, f := range files {
		program := filepath.Base(filepath.Dir(f))
		key := program + "/" + filepath.Base(f)
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		content := string(raw)
		status, ctx := detector.DetectWithContextFromLines(strings.Split(content, "\n"))
		got, ok := DetectScreenTurnEnd(program, status, ctx, content)

		want := paneExpectations[key] // zero value: not detected
		require.Equal(t, want.detected, ok, "%s: detected (status=%v ctx=%q)", key, status, ctx)
		if ok {
			assert.Equal(t, want.kind, got.Kind, key)
			detected[program]++
		}
		if strings.HasPrefix(filepath.Base(f), "idle_") {
			total[program]++
		}
	}

	var rows []string
	for program := range ProgramSignalTable {
		rows = append(rows, fmt.Sprintf("%-9s row=%-10s idle fixtures=%d detected=%d cannot-detect=%v",
			program, ProgramSignalTable[program].Kind, total[program], detected[program], CannotDetect(program)))
		if CannotDetect(program) {
			assert.Zero(t, detected[program], "%s is a none row but a turn end was read", program)
		}
	}
	sort.Strings(rows)
	t.Log("\n" + strings.Join(rows, "\n"))
}
