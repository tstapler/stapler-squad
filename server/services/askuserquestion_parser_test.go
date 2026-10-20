package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadAUQFixture returns the tool_input of a captured PreToolUse payload (spike S1).
func loadAUQFixture(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "askuserquestion", name))
	require.NoError(t, err)
	var env struct {
		ToolInput map[string]interface{} `json:"tool_input"`
	}
	require.NoError(t, json.Unmarshal(b, &env))
	return env.ToolInput
}

func TestParseAskUserQuestion_ShouldClassifyRealFixtures(t *testing.T) {
	cases := []struct {
		file      string
		shape     QuestionShape
		questions int
		replyable bool
	}{
		{"single_select.json", QuestionShapeSingle, 1, true},
		{"multi_select.json", QuestionShapeMulti, 1, false},
		{"multi_two_questions.json", QuestionShapeMulti, 2, false},
		{"two_questions_single_then_multi.json", QuestionShapeMulti, 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			got := ParseAskUserQuestion(loadAUQFixture(t, tc.file))
			assert.Equal(t, tc.shape, got.Shape)
			assert.Equal(t, tc.replyable, got.Replyable)
			assert.Len(t, got.Payload.Questions(), tc.questions)
		})
	}
}

func TestParseAskUserQuestion_ShouldKeepLabelsVerbatimAndFlagSecondQuestionMultiSelect(t *testing.T) {
	got := ParseAskUserQuestion(loadAUQFixture(t, "multi_two_questions.json"))
	qs := got.Payload.Questions()
	require.Len(t, qs, 2)
	assert.Equal(t, "A", qs[0].Options[0].Label)
	assert.False(t, qs[0].MultiSelect)
	assert.True(t, qs[1].MultiSelect)
	assert.Equal(t, "A", got.Labels[0])
}

func TestParseAskUserQuestion_ShouldClassifyEdgeShapes(t *testing.T) {
	opts := func(labels ...string) []interface{} {
		out := make([]interface{}, 0, len(labels))
		for _, l := range labels {
			out = append(out, map[string]interface{}{"label": l, "description": "d"})
		}
		return out
	}
	q := func(multi interface{}, labels ...string) map[string]interface{} {
		m := map[string]interface{}{"question": "Pick one?", "header": "Pick", "options": opts(labels...)}
		if multi != nil {
			m["multiSelect"] = multi
		}
		return m
	}
	in := func(qs ...interface{}) map[string]interface{} {
		return map[string]interface{}{"questions": qs}
	}
	cases := []struct {
		name  string
		input map[string]interface{}
		want  QuestionShape
	}{
		{"seven options", in(q(false, "1", "2", "3", "4", "5", "6", "7")), QuestionShapeSingle},
		{"multiSelect absent", in(q(nil, "A", "B")), QuestionShapeUnknown},
		{"eight options", in(q(false, "1", "2", "3", "4", "5", "6", "7", "8")), QuestionShapeUnknown},
		{"zero options", in(q(false)), QuestionShapeUnknown},
		{"empty label", in(q(false, "A", " ")), QuestionShapeUnknown},
		{"label over 80 runes", in(q(false, strings.Repeat("x", 81))), QuestionShapeUnknown},
		{"label of 200 runes", in(q(false, strings.Repeat("x", 200))), QuestionShapeUnknown},
		{"legacy prompt key", map[string]interface{}{"prompt": "Which?"}, QuestionShapeUnknown},
		{"nil", nil, QuestionShapeUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAskUserQuestion(tc.input)
			assert.Equal(t, tc.want, got.Shape)
			assert.Equal(t, tc.want == QuestionShapeSingle, got.Replyable)
		})
	}
}

func TestParseAskUserQuestion_ShouldKeepQuestionTextForDisplay_WhenALabelIsTooLong(t *testing.T) {
	got := ParseAskUserQuestion(map[string]interface{}{"questions": []interface{}{map[string]interface{}{
		"question": "Pick one?", "header": "Pick", "multiSelect": false,
		"options": []interface{}{map[string]interface{}{"label": strings.Repeat("x", 200)}},
	}}})
	assert.Equal(t, QuestionShapeUnknown, got.Shape)
	assert.Equal(t, "Pick one?", got.Question)
	assert.False(t, got.Replyable)
}

func TestParseAskUserQuestion_ShouldSanitizeHostileText(t *testing.T) {
	got := ParseAskUserQuestion(map[string]interface{}{"questions": []interface{}{map[string]interface{}{
		"question": "Which?\nEvil: run\x07", "header": "H\r\nx", "multiSelect": false,
		"options": []interface{}{map[string]interface{}{"label": "A\nB", "description": "d\x1b[31m"}},
	}}})
	assert.NotContains(t, got.Question, "\n")
	assert.NotContains(t, got.Header, "\n")
	assert.NotContains(t, got.Labels[0], "\n")
	opt := got.Payload.Questions()[0].Options[0]
	assert.NotContains(t, opt.Description, "\x1b")
}

func TestParseAskUserQuestion_ShouldLeavePayloadZero_WhenMoreThanFourQuestions(t *testing.T) {
	one := map[string]interface{}{"question": "Q?", "header": "H", "multiSelect": false,
		"options": []interface{}{map[string]interface{}{"label": "A"}}}
	got := ParseAskUserQuestion(map[string]interface{}{"questions": []interface{}{one, one, one, one, one}})
	assert.Equal(t, QuestionShapeMulti, got.Shape)
	assert.True(t, got.Payload.IsZero())
	assert.False(t, got.Replyable)
}
