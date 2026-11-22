package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callRegisteredTool invokes a tool's handler as registered on s and returns
// the parsed JSON result.
func callRegisteredTool(t *testing.T, s *mcpserver.MCPServer, name string) map[string]interface{} {
	t.Helper()
	tool := s.GetTool(name)
	require.NotNil(t, tool, "%s must be registered", name)
	res, err := tool.Handler(context.Background(), makeToolReq(nil))
	require.NoError(t, err)
	return parseResult(t, res)
}

func isRateLimited(m map[string]interface{}) bool {
	e, ok := m["error"].(map[string]interface{})
	return ok && e["code"] == string(ErrRateLimitExceeded)
}

// A nil createLimiter means "unlimited", so a production module constructor
// that forgets to pass it would silently disable rate limiting. This drives
// the real defaultToolModules and checks the limiter is non-nil and shared:
// three draws through create_session leave create_session_for_pr limited.
func TestDefaultToolModules_should_ShareOneCreateLimiter_When_LifecycleAndGitHubAreRegistered(t *testing.T) {
	t.Parallel()
	cfg := newCoreConfig([]CoreOption{WithUserPRCache(&fakeUserPRSource{})})
	s := mcpserver.NewMCPServer("test", "0.0.0")
	registerModules(s, defaultToolModules(coreDeps{store: &stubStore{}, cfg: cfg}))

	for i := 0; i < 3; i++ {
		assert.False(t, isRateLimited(callRegisteredTool(t, s, "create_session")), "draw %d is within burst", i)
	}
	assert.True(t, isRateLimited(callRegisteredTool(t, s, "create_session")), "lifecycle bucket must be non-nil and finite")
	assert.True(t, isRateLimited(callRegisteredTool(t, s, "create_session_for_pr")), "github tool must share the lifecycle bucket")
}

// create_session_for_pr must draw from its injected bucket before doing any
// other work, so an exhausted bucket short-circuits it.
func TestCreateSessionForPR_should_ReturnRateLimitExceeded_When_InjectedBucketExhausted(t *testing.T) {
	t.Parallel()
	tb := newCreateSessionLimiter()
	for tb.allow("global") {
	}
	gh := &githubHandlers{cache: &fakeUserPRSource{}, store: &stubStore{}, createLimiter: tb}
	res, err := gh.createSessionForPR(context.Background(), mcpgo.CallToolRequest{})
	require.NoError(t, err)
	assert.True(t, isRateLimited(parseResult(t, res)))
}
