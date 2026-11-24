package mcp

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewCore must hand the real storage to the guidance tools; the recording-fake
// tests build the handlers directly and would not notice a missing wire.
func TestNewCore_should_ServeCreateGuidanceRequestFromTheStorage_When_StorageIsWired(t *testing.T) {
	t.Parallel()
	storage := newTestGuidanceStorage(t)
	s := NewCore(&stubStore{}, nil, nil, storage, nil, nil, nil, nil, nil)
	tool := s.GetTool("create_guidance_request")
	require.NotNil(t, tool)

	res, err := tool.Handler(WithSessionUUID(context.Background(), uuid.NewString()), makeToolReq(map[string]any{
		"scope": "standalone", "question_text": "Wired?", "question_type": "yes-no",
	}))
	require.NoError(t, err)

	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "%v", m)
	id, perr := uuid.Parse(m["id"].(string))
	require.NoError(t, perr)
	row, gerr := storage.GetGuidanceRequest(context.Background(), id)
	require.NoError(t, gerr)
	assert.Equal(t, "Wired?", row.QuestionText)
}
