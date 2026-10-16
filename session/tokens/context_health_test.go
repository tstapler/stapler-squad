package tokens

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
)

func ringOf(turns ...healthTurnRecord) *healthTurnRing {
	r := &healthTurnRing{}
	for _, t := range turns {
		r.push(t)
	}
	return r
}

func callTurn(name, input string) healthTurnRecord {
	return healthTurnRecord{
		Fingerprints: []uint64{toolCallFingerprint(name, json.RawMessage(input))},
		ToolNames:    []string{name},
	}
}

func TestToolCallFingerprint_IsStableAcrossWhitespaceAndCase(t *testing.T) {
	a := toolCallFingerprint("Bash", json.RawMessage(`{"command":"git status"}`))
	b := toolCallFingerprint("Bash", json.RawMessage(`  {"command":"GIT STATUS"}  `))
	assert.Equal(t, a, b)
	assert.NotEqual(t, a, toolCallFingerprint("Read", json.RawMessage(`{"command":"git status"}`)))
}

func TestExtractContextHealthSignals_CountsConsecutiveIdenticalCalls(t *testing.T) {
	sig := extractContextHealthSignals(ringOf(
		callTurn("Bash", `{"command":"git status"}`),
		callTurn("Bash", `{"command":"git status"}`),
		callTurn("Bash", `{"command":"git status"}`),
		callTurn("Read", `{"file_path":"/tmp/a.go"}`),
	))
	assert.Equal(t, 3, sig.MaxConsecutiveRepeats)
	assert.Equal(t, "Bash", sig.RepeatedToolName)
	assert.Equal(t, 4, sig.ToolCallsInWindow)
}

func TestExtractContextHealthSignals_DistinctArgsAreNotRepeats(t *testing.T) {
	sig := extractContextHealthSignals(ringOf(
		callTurn("Read", `{"file_path":"/a"}`),
		callTurn("Read", `{"file_path":"/b"}`),
		callTurn("Read", `{"file_path":"/c"}`),
	))
	assert.Equal(t, 1, sig.MaxConsecutiveRepeats)
	assert.Equal(t, "", sig.RepeatedToolName)
}

func TestExtractContextHealthSignals_CountsConfusionAcrossWindow(t *testing.T) {
	r := &healthTurnRing{}
	for i := 0; i < 20; i++ {
		rec := callTurn("Read", fmt.Sprintf(`{"i":%d}`, i))
		if i%3 == 0 && i < 18 {
			rec.ConfusionHits = []string{"apology"}
		}
		r.push(rec)
	}
	sig := extractContextHealthSignals(r)
	assert.Equal(t, 6, sig.ConfusionPhraseCount)
	assert.Equal(t, "apology", sig.LastConfusionPhrase)
}

func TestMatchConfusionPatterns_MatchesApologyPhrasing(t *testing.T) {
	assert.Equal(t, "apology", matchConfusionPatterns("I apologize — that didn't work. Let me try another approach."))
}

func TestMatchConfusionPatterns_IgnoresOrdinaryProse(t *testing.T) {
	assert.Equal(t, "", matchConfusionPatterns("Sorted imports and re-ran tests; all 42 pass"))
}

func TestMatchConfusionPatterns_FastPathSkipsLongNonMatchingText(t *testing.T) {
	assert.Equal(t, "", matchConfusionPatterns(strings.Repeat("compiling the module graph. ", 150)))
}

func TestEvaluateContextHealth(t *testing.T) {
	cfg := config.ContextHealthConfig{}.ContextHealthConfigOrDefault()
	tests := []struct {
		name       string
		sig        ContextHealthSignals
		wantLevel  ContextHealthLevel
		wantReason string
	}{
		{"below sample floor", ContextHealthSignals{ToolCallsInWindow: 3, MaxConsecutiveRepeats: 3, ConfusionPhraseCount: 9}, HealthUnknown, ""},
		{"loop at threshold", ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 3, RepeatedToolName: "Bash"}, HealthAmber, "Repeated the same Bash call 3 times in a row"},
		{"loop at 2x threshold", ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 6, RepeatedToolName: "Bash"}, HealthRed, "Repeated the same Bash call 6 times in a row"},
		{"both signals", ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 3, RepeatedToolName: "Edit", ConfusionPhraseCount: 5}, HealthRed, "Repeated the same Edit call 3 times in a row; 5 self-correction messages in the last 20 turns"},
		{"confusion at threshold", ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 1, ConfusionPhraseCount: 5}, HealthAmber, "5 self-correction messages in the last 20 turns"},
		{"confusion at 2x", ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 1, ConfusionPhraseCount: 10}, HealthRed, "10 self-correction messages in the last 20 turns"},
		{"loop just below", ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 2, RepeatedToolName: "Bash"}, HealthGreen, ""},
		{"confusion just below", ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 1, ConfusionPhraseCount: 4}, HealthGreen, ""},
		{"clean", ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 1}, HealthGreen, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateContextHealth(tc.sig, cfg)
			assert.Equal(t, tc.wantLevel, got.Level)
			assert.Equal(t, tc.wantReason, got.Reason)
		})
	}
}

func TestEvaluateContextHealth_ZeroConfigUsesDefaults(t *testing.T) {
	sig := ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 3, RepeatedToolName: "Bash"}
	assert.Equal(t,
		EvaluateContextHealth(sig, config.ContextHealthConfig{}.ContextHealthConfigOrDefault()),
		EvaluateContextHealth(sig, config.ContextHealthConfig{}))
}

func TestEvaluateContextHealth_ConcurrentCallsAgree(t *testing.T) {
	sig := ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 3, RepeatedToolName: "Bash"}
	var wg sync.WaitGroup
	got := make([]ContextHealthVerdict, 8)
	for i := range got {
		wg.Add(1)
		go func() { defer wg.Done(); got[i] = EvaluateContextHealth(sig, config.ContextHealthConfig{}) }()
	}
	wg.Wait()
	for _, v := range got {
		assert.Equal(t, got[0], v)
	}
}

func assistantLine(model, content string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":"2026-01-01T00:00:00Z","message":{"role":"assistant","model":%q,"content":[%s]}}`, model, content) + "\n"
}

func bashUse(cmd string) string {
	return fmt.Sprintf(`{"type":"tool_use","name":"Bash","input":{"command":%q}}`, cmd)
}

func parseLines(t *testing.T, lines ...string) *ParseResult {
	t.Helper()
	res, err := NewParser().ParseReader(strings.NewReader(strings.Join(lines, "")))
	require.NoError(t, err)
	return res
}

func TestParseReader_PopulatesContextHealthSignals(t *testing.T) {
	l := assistantLine("claude-sonnet-4", bashUse("npm test"))
	res := parseLines(t, l, l, l)
	assert.Equal(t, 3, res.ContextHealth.MaxConsecutiveRepeats)
	assert.Equal(t, 3, res.ContextHealth.ToolCallsInWindow)
}

func TestParseReader_OnlyTrailingWindowContributes(t *testing.T) {
	var lines []string
	for i := 0; i < 4; i++ {
		lines = append(lines, assistantLine("claude-sonnet-4", bashUse("npm test")))
	}
	for i := 0; i < 25; i++ {
		lines = append(lines, assistantLine("claude-sonnet-4",
			fmt.Sprintf(`{"type":"tool_use","name":"Read","input":{"file_path":"/f%d"}}`, i)))
	}
	res := parseLines(t, lines...)
	assert.Equal(t, 1, res.ContextHealth.MaxConsecutiveRepeats)
	assert.Equal(t, healthWindowTurns, res.ContextHealth.ToolCallsInWindow)
}

func TestParseReader_SkipsSyntheticTurnsInHealthWindow(t *testing.T) {
	res := parseLines(t,
		assistantLine("claude-sonnet-4", bashUse("npm test")),
		assistantLine(syntheticModelSentinel, bashUse("something else")),
		assistantLine("claude-sonnet-4", bashUse("npm test")),
	)
	assert.Equal(t, 2, res.ContextHealth.MaxConsecutiveRepeats)
}

func TestParseReader_CountsConfusionInTextBlocks(t *testing.T) {
	l := assistantLine("claude-sonnet-4", `{"type":"text","text":"I apologize, that didn't work."}`)
	res := parseLines(t, l, l)
	assert.Equal(t, 2, res.ContextHealth.ConfusionPhraseCount)
	assert.Equal(t, "apology", res.ContextHealth.LastConfusionPhrase)
}

func TestParseReader_DoesNotRetainToolInput(t *testing.T) {
	res := parseLines(t, assistantLine("claude-sonnet-4", bashUse("export SECRET_TOKEN=abc123")),
		assistantLine("claude-sonnet-4", `{"type":"text","text":"SECRET_TOKEN=abc123 sorry"}`))
	out := fmt.Sprintf("%+v", res)
	assert.NotContains(t, out, "SECRET_TOKEN")
	assert.NotContains(t, out, "abc123")
}
