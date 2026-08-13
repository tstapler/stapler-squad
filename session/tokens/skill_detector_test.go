package tokens

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillDetector_WhenSlashCommandInHumanTurn_ExpectIsCommandTrue(t *testing.T) {
	jsonl := `{"parentUuid":null,"isSidechain":false,"type":"user","message":{"role":"user","content":[{"type":"text","text":"/plan:feature implement auth"}]},"uuid":"human-1","timestamp":"2026-05-15T10:00:00.000Z","sessionId":"skill-test"}` + "\n"

	p := NewParser()
	result, err := p.ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(result.SkillActivations), 1)
	found := false
	for _, a := range result.SkillActivations {
		if a.Name == "plan:feature" && a.IsCommand {
			found = true
			break
		}
	}
	assert.True(t, found, "expected to find plan:feature command activation")
}

func TestSkillDetector_WhenSkillMdReadToolResult_ExpectIsCommandFalse(t *testing.T) {
	jsonl := `{"parentUuid":null,"isSidechain":false,"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"Contents of /Users/alice/.claude/skills/code-review.md loaded."}]}]},"uuid":"human-1","timestamp":"2026-05-15T10:00:00.000Z","sessionId":"skill-test"}` + "\n"

	p := NewParser()
	result, err := p.ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(result.SkillActivations), 1)
	found := false
	for _, a := range result.SkillActivations {
		if a.Name == "code-review" && !a.IsCommand {
			found = true
			break
		}
	}
	assert.True(t, found, "expected to find code-review skill activation with IsCommand=false")
}

// TestDetectCommandsInText_ZeroAllocsWhenNoSlash enforces Pattern F: the fast-path
// guard (`!strings.ContainsRune(text, '/')`) must make the common case allocation-free.
// If this test fails, the guard was removed or hoisted below an allocating statement.
func TestDetectCommandsInText_ZeroAllocsWhenNoSlash(t *testing.T) {
	text := "what is the status of the build"
	allocs := testing.AllocsPerRun(100, func() {
		_ = detectCommandsInText(text, 0)
	})
	if allocs != 0 {
		t.Errorf("detectCommandsInText no-slash path: got %.0f allocs, want 0", allocs)
	}
}

// BenchmarkDetectCommandsInText measures the fast path (no slash) and the slow
// path (contains a /command). The no-slash case must be allocation-free.
func BenchmarkDetectCommandsInText(b *testing.B) {
	b.Run("NoSlash", func(b *testing.B) {
		text := "what is the status of the build"
		b.ReportAllocs()
		for b.Loop() {
			_ = detectCommandsInText(text, 0)
		}
	})
	b.Run("WithCommand", func(b *testing.B) {
		text := "please run /plan:feature and then /code:review"
		b.ReportAllocs()
		for b.Loop() {
			_ = detectCommandsInText(text, 0)
		}
	})
}

func TestSkillDetector_WhenRegularMessage_ExpectNoActivations(t *testing.T) {
	jsonl := `{"parentUuid":null,"isSidechain":false,"type":"user","message":{"role":"user","content":[{"type":"text","text":"what is the status of the build?"}]},"uuid":"human-1","timestamp":"2026-05-15T10:00:00.000Z","sessionId":"skill-test"}` + "\n"

	p := NewParser()
	result, err := p.ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)

	assert.Len(t, result.SkillActivations, 0)
}

func TestSkillDetector_WhenMultipleCommandsInOneTurn_ExpectAllDetected(t *testing.T) {
	jsonl := `{"parentUuid":null,"isSidechain":false,"type":"user","message":{"role":"user","content":[{"type":"text","text":"run /plan:feature then /code:review"}]},"uuid":"human-1","timestamp":"2026-05-15T10:00:00.000Z","sessionId":"skill-test"}` + "\n"

	p := NewParser()
	result, err := p.ParseReader(strings.NewReader(jsonl))
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(result.SkillActivations), 2)

	names := make(map[string]bool)
	for _, a := range result.SkillActivations {
		names[a.Name] = true
	}
	assert.True(t, names["plan:feature"], "expected plan:feature")
	assert.True(t, names["code:review"], "expected code:review")
}
