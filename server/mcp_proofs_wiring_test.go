package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// Serial: installs process-wide proofs and a proof provider and sets the state dir.
func TestWireMCPProofs_should_InstallProviderSweepStaleAndCleanOnDelete(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	keys, err := services.NewHookProofs(dir)
	require.NoError(t, err)
	prevKeys, prevMcp := services.DefaultHookProofs(), services.DefaultMcpProofs()
	services.SetDefaultHookProofs(keys)
	t.Cleanup(func() {
		services.SetDefaultHookProofs(prevKeys)
		services.SetDefaultMcpProofs(prevMcp)
		session.SetMCPProofProvider(nil)
	})

	// A stale file for a session that is no longer known, and one for a kept session.
	stale := services.NewMcpProofs(keys, dir)
	require.NoError(t, stale.EnsureFile("dead"))
	require.NoError(t, stale.EnsureFile("kept"))

	ctx, cancel := context.WithCancel(context.Background())
	bus := events.NewEventBus(16)
	deps := &ServerDependencies{EventBus: bus, Instances: []*session.Instance{{UUID: "kept"}}}
	done := make(chan struct{})
	wireMCPProofs(ctx, deps, func(fn func()) { go func() { fn(); close(done) }() })

	proofs := services.DefaultMcpProofs()
	require.NotNil(t, proofs, "self-check passed and proofs were installed")
	assert.NoFileExists(t, session.MCPProofFilePath(dir, "dead"), "startup sweep")
	assert.FileExists(t, session.MCPProofFilePath(dir, "kept"))

	// Deleting a session removes its proof file (the subscriber is asynchronous).
	require.NoError(t, proofs.EnsureFile("doomed"))
	bus.Publish(events.NewSessionDeletedEvent("doomed"))
	wait.RequireEventually(t, func() bool {
		_, statErr := os.Stat(session.MCPProofFilePath(dir, "doomed"))
		return os.IsNotExist(statErr)
	}, 5*time.Second, 10*time.Millisecond, "proof file removed on session delete")
	assert.FileExists(t, session.MCPProofFilePath(dir, "kept"), "other sessions untouched")

	cancel()
	<-done
}

func TestWireMCPProofs_should_InstallNothing_When_HookProofSecretMissing(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	prevKeys, prevMcp := services.DefaultHookProofs(), services.DefaultMcpProofs()
	services.SetDefaultHookProofs(nil)
	services.SetDefaultMcpProofs(nil)
	t.Cleanup(func() {
		services.SetDefaultHookProofs(prevKeys)
		services.SetDefaultMcpProofs(prevMcp)
	})

	wireMCPProofs(context.Background(), &ServerDependencies{}, func(func()) {})

	assert.Nil(t, services.DefaultMcpProofs(), "fail closed: nothing verifies")
}

type fakeLiveFinder struct{ live map[string]bool }

func (f fakeLiveFinder) FindLiveInstance(id string) *session.Instance {
	if f.live[id] {
		return &session.Instance{UUID: id}
	}
	return nil
}

func TestNewMCPIdentityMiddleware_should_VerifyThroughDefaultProofs(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	keys, err := services.NewHookProofs(dir)
	require.NoError(t, err)
	proofs := services.NewMcpProofs(keys, dir)
	prev := services.DefaultMcpProofs()
	t.Cleanup(func() { services.SetDefaultMcpProofs(prev) })

	var sawClaim, sawProofHeader bool
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		sawClaim = r.Header.Get("X-Stapler-Session-UUID") == "live"
		sawProofHeader = r.Header.Get(session.MCPProofHeader) != ""
	})
	mw := newMCPIdentityMiddleware(next, fakeLiveFinder{live: map[string]bool{"live": true}})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("X-Stapler-Session-UUID", "live")
	req.Header.Set(session.MCPProofHeader, proofs.HeaderValue("live"))

	// Before wiring (no default proofs) the request is simply unverified, never an error.
	services.SetDefaultMcpProofs(nil)
	mw.ServeHTTP(httptest.NewRecorder(), req)
	assert.True(t, sawClaim)
	assert.True(t, sawProofHeader)
}
