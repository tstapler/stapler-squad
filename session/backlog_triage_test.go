package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ParseHeadlessTriageResult ────────────────────────────────────────────────

func TestParseHeadlessTriageResult_ValidJSON(t *testing.T) {
	raw := `{"summary":"Nice summary","suggestions":[{"text":"do X","rationale":"why"}],"tasks":[{"text":"task 1","estimate":"2h","category":"backend"}]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "Nice summary", result.Summary)
	require.Len(t, result.Suggestions, 1)
	assert.Equal(t, "do X", result.Suggestions[0].Text)
	require.Len(t, result.Tasks, 1)
	assert.Equal(t, "2h", result.Tasks[0].Estimate)
}

func TestParseHeadlessTriageResult_StripsMarkdownFences(t *testing.T) {
	raw := "```json\n{\"summary\":\"fenced\",\"suggestions\":[]}\n```"
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "fenced", result.Summary)
}

func TestParseHeadlessTriageResult_InvalidJSON(t *testing.T) {
	_, err := ParseHeadlessTriageResult("{not json}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ParseHeadlessTriageResult")
}

func TestParseHeadlessTriageResult_CapsTasksAt12(t *testing.T) {
	tasks := make([]string, 0, 15)
	for i := range 15 {
		tasks = append(tasks, `{"text":"t`+string(rune('0'+i))+`","estimate":"1h","category":"backend"}`)
	}
	raw := `{"summary":"x","suggestions":[],"tasks":[` + strings.Join(tasks, ",") + `]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Len(t, result.Tasks, maxHeadlessTriageTasks)
}

func TestParseHeadlessTriageResult_EmptySuggestionsOK(t *testing.T) {
	raw := `{"summary":"minimal","suggestions":[]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "minimal", result.Summary)
	assert.Empty(t, result.Tasks)
}

// ─── BuildHeadlessTriagePrompt ────────────────────────────────────────────────

func TestBuildHeadlessTriagePrompt_ContainsTitle(t *testing.T) {
	item := &BacklogItemData{Title: "My Feature", ID: "abc-123"}
	prompt := BuildHeadlessTriagePrompt(item, "/tmp/artifacts")
	assert.Contains(t, prompt, "My Feature")
}

func TestBuildHeadlessTriagePrompt_ContainsArtifactPath(t *testing.T) {
	item := &BacklogItemData{Title: "Test", ID: "id-1"}
	prompt := BuildHeadlessTriagePrompt(item, "/repo/docs/tasks/test")
	assert.Contains(t, prompt, "/repo/docs/tasks/test")
}

func TestBuildHeadlessTriagePrompt_InstructsJSONOutput(t *testing.T) {
	item := &BacklogItemData{Title: "Test", ID: "id-1"}
	prompt := BuildHeadlessTriagePrompt(item, "/tmp/art")
	// Headless mode must instruct JSON output, not call an MCP tool.
	assert.Contains(t, prompt, "output ONLY a JSON object",
		"headless prompt must instruct JSON output mode")
}

func TestBuildHeadlessTriagePrompt_IncludesAcceptanceCriteria(t *testing.T) {
	// ParseAcCriteria expects JSON-encoded criteria.
	acJSON := `[{"index":1,"text":"User can log in","status":"pending"},{"index":2,"text":"User can log out","status":"pending"}]`
	item := &BacklogItemData{
		Title:              "AC Test",
		ID:                 "id-2",
		AcceptanceCriteria: acJSON,
	}
	prompt := BuildHeadlessTriagePrompt(item, "/tmp")
	assert.Contains(t, prompt, "User can log in")
}

func TestBuildHeadlessTriagePrompt_NoAcSection_WhenEmpty(t *testing.T) {
	item := &BacklogItemData{Title: "No AC", ID: "id-3"}
	prompt := BuildHeadlessTriagePrompt(item, "/tmp")
	assert.NotContains(t, prompt, "Acceptance Criteria")
}
