package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// TestCreateSession_HonorsSessionNameOverrideMap verifies that the
// lifecycleHandlers.createSession MCP tool wires session.ResolveSessionBackend
// (tymux-bundled-integration Epic 4.4.1): with no per-request override field
// on this MCP tool's schema, a TymuxSessionOverrides entry keyed by the
// sanitized tmux session name still forces the resulting instance's backend
// even though the process-wide default is registered as tymux.
func TestCreateSession_HonorsSessionNameOverrideMap(t *testing.T) {
	session.RegisterBackendProvider(session.BackendTymux)
	t.Cleanup(func() { session.RegisterBackendProvider(session.BackendTmux) })

	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

	const title = "lifecycle-session-override-map-test"
	sessionKey := tmux.NewSessionName(title, tmux.TmuxPrefix).String()
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config.json"),
		[]byte(`{"default_program": "claude", "tymux_session_overrides": {"`+sessionKey+`": false}}`), 0o644))

	path := t.TempDir()
	store := &stubStore{}
	handlers := &lifecycleHandlers{store: store}

	res, err := handlers.createSession(context.Background(), makeToolReq(map[string]interface{}{
		"title": title,
		"path":  path,
	}))
	require.NoError(t, err)
	result := parseResult(t, res)
	require.Equal(t, true, result["success"], "createSession must succeed: %+v", result)
	t.Cleanup(func() {
		if len(store.instances) > 0 {
			_ = store.instances[0].Destroy()
		}
	})

	require.Len(t, store.instances, 1)
	assert.Equal(t, session.BackendTmux, store.instances[0].Backend,
		"a TymuxSessionOverrides entry keyed by the sanitized tmux session name must force the backend even though the process-wide default is tymux")
}
