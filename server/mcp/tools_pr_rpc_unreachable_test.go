package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services"
)

// uiOnlyPRRPCs are the PR-set mutations a dispatched agent must never reach:
// they are the operator's decisions.
var uiOnlyPRRPCs = map[string]string{
	"PinSessionPullRequest":      "pin_session_pull_request",
	"RefreshSessionPullRequests": "refresh_session_pull_requests",
	"DetachSessionPullRequest":   "detach_session_pull_request",
	"AttachSessionPullRequest":   "attach_session_pull_request",
	"AskSessionToFixPullRequest": "ask_session_to_fix_pull_request",
	"DetachItemPullRequest":      "detach_item_pull_request",
}

func TestMCPTools_ShouldNotRouteToPRMutatingRPCs(t *testing.T) {
	storage := newTestBacklogStorage(t)
	svc := services.NewSessionService(storage, nil)
	core := NewCore(&stubStore{}, svc, nil, storage, nil, nil, nil, nil, nil, nil)

	for name := range core.ListTools() {
		for rpc, snake := range uiOnlyPRRPCs {
			assert.NotContainsf(t, strings.ToLower(name), strings.ReplaceAll(snake, "_", ""), "tool %q looks like %s", name, rpc)
			assert.NotEqualf(t, snake, name, "tool %q exposes %s", name, rpc)
		}
	}

	// Handlers are closures, so also forbid naming the RPCs anywhere in the
	// package's non-test sources (a tool could call the service method directly).
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		for rpc := range uiOnlyPRRPCs {
			assert.NotContainsf(t, string(src), rpc, "%s references the UI-only RPC %s", f, rpc)
		}
	}
}
