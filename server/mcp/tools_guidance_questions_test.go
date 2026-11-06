package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

func newGuidanceCaller(t *testing.T) (*backlogHandlers, *session.Storage, context.Context) {
	t.Helper()
	storage := newTestGuidanceStorage(t)
	return &backlogHandlers{storage: storage}, storage, WithSessionUUID(context.Background(), uuid.New().String())
}

func errCode(t *testing.T, m map[string]interface{}) string {
	t.Helper()
	require.False(t, m["success"].(bool), "expected failure, got: %v", m)
	errMap, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	return errMap["code"].(string)
}

func TestCreateGuidanceRequest_should_StoreQuestionsJSON_When_QuestionsGiven(t *testing.T) {
	t.Parallel()
	h, storage, ctx := newGuidanceCaller(t)

	result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
		"scope": "standalone",
		"questions": []interface{}{map[string]interface{}{
			"text":    "Region?",
			"options": []interface{}{map[string]interface{}{"label": "us"}, map[string]interface{}{"label": "eu"}},
		}},
	}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.True(t, m["success"].(bool), "got: %v", m)
	assert.Equal(t, "Region?", m["question_text"])
	assert.Equal(t, "multiple-choice", m["question_type"])
	assert.Len(t, m["questions"], 1)

	id, perr := uuid.Parse(m["id"].(string))
	require.NoError(t, perr)
	row, gerr := storage.GetGuidanceRequest(context.Background(), id)
	require.NoError(t, gerr)
	assert.Contains(t, row.QuestionsJSON, `"Region?"`)
	assert.Equal(t, `["us","eu"]`, row.Options)
}

func TestCreateGuidanceRequest_should_AcceptMultiSelectAndStringOptions(t *testing.T) {
	t.Parallel()
	h, _, ctx := newGuidanceCaller(t)

	result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
		"scope":         "session",
		"question_text": "Pick",
		"questions": []interface{}{
			map[string]interface{}{"text": "Which?", "options": []interface{}{"a", "b"}, "multi_select": true},
			map[string]interface{}{"text": "And?", "options": []interface{}{"c"}},
		},
	}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.True(t, m["success"].(bool), "got: %v", m)
	qs := m["questions"].([]interface{})
	require.Len(t, qs, 2)
	assert.Equal(t, true, qs[0].(map[string]interface{})["multiSelect"])
	assert.Equal(t, "Pick", m["question_text"], "an explicit question_text wins")
}

func TestCreateGuidanceRequest_should_RejectInvalidQuestions(t *testing.T) {
	t.Parallel()
	opt := []interface{}{map[string]interface{}{"label": "x"}}
	q := func(text string, opts []interface{}) map[string]interface{} {
		return map[string]interface{}{"text": text, "options": opts}
	}
	cases := map[string]interface{}{
		"empty array":      []interface{}{},
		"not an array":     "nope",
		"five questions":   []interface{}{q("1", opt), q("2", opt), q("3", opt), q("4", opt), q("5", opt)},
		"no options":       []interface{}{q("Q?", nil)},
		"empty text":       []interface{}{q("", opt)},
		"label too long":   []interface{}{q("Q?", []interface{}{map[string]interface{}{"label": strings.Repeat("x", 201)}})},
		"nine options":     []interface{}{q("Q?", make9())},
		"question not obj": []interface{}{"x"},
	}
	for name, qs := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, _, ctx := newGuidanceCaller(t)
			result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
				"scope": "standalone", "question_text": "fallback", "questions": qs,
			}))
			require.NoError(t, err)
			assert.Equal(t, ErrInvalidArgument, errCode(t, parseResult(t, result)))
		})
	}
}

func make9() []interface{} {
	out := make([]interface{}, 9)
	for i := range out {
		out[i] = map[string]interface{}{"label": string(rune('a' + i))}
	}
	return out
}

func TestCreateGuidanceRequest_should_RequireQuestionTextOrQuestions(t *testing.T) {
	t.Parallel()
	h, _, ctx := newGuidanceCaller(t)
	result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{"scope": "standalone", "question_type": "yes-no"}))
	require.NoError(t, err)
	assert.Equal(t, ErrInvalidArgument, errCode(t, parseResult(t, result)))
}

func TestCreateGuidanceRequest_should_KeepOwnershipCheck_When_QuestionsGivenForBacklogItem(t *testing.T) {
	t.Parallel()
	h, storage, ctx := newGuidanceCaller(t)
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{Title: "unlinked structured"})
	require.NoError(t, err)
	result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
		"scope": "backlog-item", "item_id": item.ID,
		"questions": []interface{}{map[string]interface{}{"text": "Q?", "options": []interface{}{"a"}}},
	}))
	require.NoError(t, err)
	assert.Equal(t, ErrPermissionDenied, errCode(t, parseResult(t, result)))
}

func TestCreateGuidanceRequest_should_BehaveAsBefore_When_QuestionsOmitted(t *testing.T) {
	t.Parallel()
	h, storage, ctx := newGuidanceCaller(t)
	result, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]interface{}{
		"scope": "standalone", "question_text": "Plain?", "question_type": "yes-no",
	}))
	require.NoError(t, err)
	m := parseResult(t, result)
	require.True(t, m["success"].(bool))
	assert.NotContains(t, m, "questions")
	id, _ := uuid.Parse(m["id"].(string))
	row, gerr := storage.GetGuidanceRequest(context.Background(), id)
	require.NoError(t, gerr)
	assert.Empty(t, row.QuestionsJSON)
}
