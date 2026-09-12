package services

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/headless"
)

func validBacklogIntentJSON() string {
	return `{"title":"Add CSV export","description":"Let users export their data as CSV from settings.","acceptance_criteria":["Export button appears in settings","Downloaded file is valid CSV"],"confidence":0.8}`
}

// TestParseBacklogItemIntent_should_ReturnStructuredDraft_When_HeadlessCallSucceeds
// is the AC0 regression guard: the draft's fields must come from the LLM's
// JSON output, not be the raw message truncated/echoed (deriveChatItemTitle's
// behavior on the existing raw-text fast path).
func TestParseBacklogItemIntent_should_ReturnStructuredDraft_When_HeadlessCallSucceeds(t *testing.T) {
	t.Parallel()
	svc := newBacklogService(t)
	pool := &fakeHeadlessPool{response: validBacklogIntentJSON()}
	svc.SetHeadlessPool(pool)

	resp, err := svc.ParseBacklogItemIntent(t.Context(), connect.NewRequest(&sessionv1.ParseBacklogItemIntentRequest{
		Message: "please add a way to export my data as csv from the settings page thanks",
	}))
	require.NoError(t, err)
	require.Empty(t, resp.Msg.Error)
	require.NotNil(t, resp.Msg.Draft)
	assert.Equal(t, "Add CSV export", resp.Msg.Draft.Title)
	assert.NotEqual(t, "please add a way to export my data as csv from the settings page thanks", resp.Msg.Draft.Title)
	assert.Len(t, resp.Msg.Draft.AcceptanceCriteria, 2)
	assert.InDelta(t, 0.8, resp.Msg.Draft.Confidence, 0.01)

	require.Len(t, pool.calls, 1)
	assert.Equal(t, headless.FeatureKeyBacklogIntentParse, pool.calls[0].key)
}

// TestParseBacklogItemIntent_should_FallBackToErrorField_When_Unsuccessful
// asserts the best-effort contract across every non-happy-path shape (LLM
// call error, unparseable output, no pool wired): none becomes a Connect
// error, so the client can always fall back to CreateBacklogItemFromChat
// instead of losing the user's typed text.
func TestParseBacklogItemIntent_should_FallBackToErrorField_When_Unsuccessful(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		wirePool bool
		pool     *fakeHeadlessPool
	}{
		{"headless call errors", true, &fakeHeadlessPool{err: assert.AnError}},
		{"output has no parseable JSON", true, &fakeHeadlessPool{response: "I couldn't structure this, sorry."}},
		{"no headless pool wired", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := newBacklogService(t)
			if tt.wirePool {
				svc.SetHeadlessPool(tt.pool)
			}

			resp, err := svc.ParseBacklogItemIntent(t.Context(), connect.NewRequest(&sessionv1.ParseBacklogItemIntentRequest{
				Message: "some free-text task description",
			}))
			require.NoError(t, err)
			assert.Nil(t, resp.Msg.Draft)
			assert.NotEmpty(t, resp.Msg.Error)
		})
	}
}

func TestParseBacklogItemIntent_should_RejectEmptyMessage_When_MessageIsWhitespaceOnly(t *testing.T) {
	t.Parallel()
	svc := newBacklogService(t)

	_, err := svc.ParseBacklogItemIntent(t.Context(), connect.NewRequest(&sessionv1.ParseBacklogItemIntentRequest{
		Message: "   ",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
