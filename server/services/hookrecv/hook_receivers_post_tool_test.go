package hookrecv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type postToolCall struct{ session, tool string }

func postToolUse(t *testing.T, h *HookReceiver, headers map[string]string, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/post-tool-use", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.HandlePostToolUse(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlePostToolUse_ShouldReportToolNameFromBody_WhenHeaderIsMissing(t *testing.T) {
	t.Parallel()
	h := NewHookReceiver()
	var calls []postToolCall
	h.SetPostToolUseObserver(func(_ context.Context, session, tool string) { calls = append(calls, postToolCall{session, tool}) })

	postToolUse(t, h, map[string]string{sessionIDHeader: "my-session"}, `{"tool_name":"AskUserQuestion","tool_use_id":"toolu_1"}`)
	postToolUse(t, h, map[string]string{sessionIDHeader: "my-session", "X-CS-Tool-Name": "Bash"}, `{"tool_name":"Read"}`)
	postToolUse(t, h, map[string]string{sessionIDHeader: "my-session"}, `not json`)

	assert.Equal(t, []postToolCall{{"my-session", "AskUserQuestion"}, {"my-session", "Bash"}, {"my-session", ""}}, calls)
}

func TestHandlePostToolUse_ShouldStillAnswer_WhenNoObserverIsSet(t *testing.T) {
	t.Parallel()
	postToolUse(t, NewHookReceiver(), nil, `{"tool_name":"AskUserQuestion"}`)
}
