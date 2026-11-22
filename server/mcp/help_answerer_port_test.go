package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session/domain"
)

// recordingHelpAnswerer is the second HelpAnswerer implementation (the first is
// *services.GuidanceRequestService): it pins the arguments answer_help forwards and
// reaches the error branches a real store cannot produce on demand.
type recordingHelpAnswerer struct {
	id, answer, who string
	data            *domain.GuidanceRequestData
	applied         bool
	err             error
}

func (f *recordingHelpAnswerer) AnswerAsSession(_ context.Context, id, answer, sessionUUID string) (*domain.GuidanceRequestData, bool, error) {
	f.id, f.answer, f.who = id, answer, sessionUUID
	return f.data, f.applied, f.err
}

func TestAnswerHelp_should_ForwardTheVerifiedCallerAsTheAnswerer(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	fake := &recordingHelpAnswerer{data: &domain.GuidanceRequestData{ID: "req-1"}, applied: true}
	r.deps.Answerer = fake

	m := r.call(as("B"), toolAnswerHelp, map[string]any{"request_id": "req-1", "answer": "do X"})

	require.True(t, m["success"].(bool), "%v", m)
	assert.Equal(t, [3]string{"req-1", "do X", "B"}, [3]string{fake.id, fake.answer, fake.who})
	assert.Equal(t, true, m["applied"])
	assert.EqualValues(t, 1, r.metrics.Value(wakes.MetricPeerHelp, "answered"))
}

func TestAnswerHelp_should_MapAnswererErrors_When_TheAnswererFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"forbidden sentinel", domain.ErrAnswererForbidden, CodeForbidden},
		{"wrapped forbidden", errors.Join(errors.New("ctx"), domain.ErrAnswererForbidden), CodeForbidden},
		{"not found", domain.ErrNotFound, ErrItemNotFound},
		{"other", errors.New("store closed"), ErrInternalError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newToolRig(t)
			r.deps.Answerer = &recordingHelpAnswerer{err: tc.err}

			m := r.call(as("B"), toolAnswerHelp, map[string]any{"request_id": "req-1", "answer": "do X"})

			assert.Equal(t, tc.wantCode, resultCode(m))
			assert.EqualValues(t, 0, r.metrics.Value(wakes.MetricPeerHelp, "answered"))
		})
	}
}
