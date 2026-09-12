package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBacklogItemIntentDraft_ValidJSON(t *testing.T) {
	t.Parallel()
	raw := `{"title":"Add dark mode toggle","description":"Let users switch themes from settings.","acceptance_criteria":["Toggle appears in settings","Preference persists across reload"],"confidence":0.85}`
	draft, err := ParseBacklogItemIntentDraft(raw)
	require.NoError(t, err)
	assert.Equal(t, "Add dark mode toggle", draft.Title)
	assert.Equal(t, "Let users switch themes from settings.", draft.Description)
	assert.Equal(t, []string{"Toggle appears in settings", "Preference persists across reload"}, draft.AcceptanceCriteria)
	assert.InDelta(t, 0.85, draft.Confidence, 0.001)
}

func TestParseBacklogItemIntentDraft_StripsMarkdownFences(t *testing.T) {
	t.Parallel()
	raw := "```json\n{\"title\":\"fenced title\",\"description\":\"d\",\"acceptance_criteria\":[],\"confidence\":0.5}\n```"
	draft, err := ParseBacklogItemIntentDraft(raw)
	require.NoError(t, err)
	assert.Equal(t, "fenced title", draft.Title)
}

func TestParseBacklogItemIntentDraft_PreambleBeforeJSON(t *testing.T) {
	t.Parallel()
	raw := "Here's the structured item:\n\n" +
		`{"title":"preamble ok","description":"d","acceptance_criteria":[],"confidence":0.6}`
	draft, err := ParseBacklogItemIntentDraft(raw)
	require.NoError(t, err)
	assert.Equal(t, "preamble ok", draft.Title)
}

func TestParseBacklogItemIntentDraft_NotVerbatimRawText(t *testing.T) {
	t.Parallel()
	// Guards against a regression to the raw-text fast path's behavior
	// (deriveChatItemTitle truncates the first line; the LLM draft's title
	// must be a distinct, synthesized value, not the input echoed back).
	raw := `{"title":"Structured summary of the ask","description":"A fuller elaboration of the raw message, not the raw message itself.","acceptance_criteria":["Criterion A","Criterion B"],"confidence":0.7}`
	draft, err := ParseBacklogItemIntentDraft(raw)
	require.NoError(t, err)
	assert.NotEqual(t, "please add a way to export my data as csv from the settings page thanks", draft.Title)
	assert.Len(t, draft.AcceptanceCriteria, 2)
}

func TestParseBacklogItemIntentDraft_NoJSON(t *testing.T) {
	t.Parallel()
	_, err := ParseBacklogItemIntentDraft("No JSON here at all.")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ParseBacklogItemIntentDraft")
}

func TestParseBacklogItemIntentDraft_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := ParseBacklogItemIntentDraft("{not json}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ParseBacklogItemIntentDraft")
}

func TestParseBacklogItemIntentDraft_DecoyObjectSkipped(t *testing.T) {
	t.Parallel()
	// An earlier syntactically-valid-but-empty object (e.g. an illustrative
	// schema example) must not win over the real, later draft.
	raw := `Example shape: {"title":"","description":""}` + "\n\n" +
		`{"title":"real draft","description":"d","acceptance_criteria":["c"],"confidence":0.9}`
	draft, err := ParseBacklogItemIntentDraft(raw)
	require.NoError(t, err)
	assert.Equal(t, "real draft", draft.Title)
}

func TestParseBacklogItemIntentDraft_EmptyAcceptanceCriteriaOK(t *testing.T) {
	t.Parallel()
	raw := `{"title":"vague ask","description":"too sparse to derive ACs","acceptance_criteria":[],"confidence":0.2}`
	draft, err := ParseBacklogItemIntentDraft(raw)
	require.NoError(t, err)
	assert.Empty(t, draft.AcceptanceCriteria)
	assert.InDelta(t, 0.2, draft.Confidence, 0.001)
}
