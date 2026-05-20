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
