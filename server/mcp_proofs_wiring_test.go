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
	"github.com/tstapler/stapler-squad/server/services/approvals"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// Serial: installs process-wide proofs and a proof provider and sets the state dir.
func TestWireMCPProofs_should_InstallProviderSweepStaleAndCleanOnDelete(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	keys, err := approvals.NewHookProofs(dir)
	require.NoError(t, err)
	prevKeys, prevMcp := approvals.DefaultHookProofs(), approvals.DefaultMcpProofs()
	approvals.SetDefaultHookProofs(keys)
	t.Cleanup(func() {
		approvals.SetDefaultHookProofs(prevKeys)
		approvals.SetDefaultMcpProofs(prevMcp)
		instance.SetMCPProofProvider(nil)
	})

	// A stale file for a session that is no longer known, and one for a kept session.
	stale := approvals.NewMcpProofs(keys, dir)
	require.NoError(t, stale.EnsureFile("dead"))
	require.NoError(t, stale.EnsureFile("kept"))

	ctx, cancel := context.WithCancel(context.Background())
	bus := events.NewEventBus(16)
	deps := &ServerDependencies{EventBus: bus, Instances: []*instance.Instance{{UUID: "kept"}}}
	done := make(chan struct{})
	wireMCPProofs(ctx, deps, func(fn func()) { go func() { fn(); close(done) }() })

	proofs := approvals.DefaultMcpProofs()
	require.NotNil(t, proofs, "self-check passed and proofs were installed")
	assert.NoFileExists(t, instance.MCPProofFilePath(dir, "dead"), "startup sweep")
	assert.FileExists(t, instance.MCPProofFilePath(dir, "kept"))

	// Deleting a session removes its proof file (the subscriber is asynchronous).
	require.NoError(t, proofs.EnsureFile("doomed"))
	bus.Publish(events.NewSessionDeletedEvent("doomed"))
	wait.RequireEventually(t, func() bool {
		_, statErr := os.Stat(instance.MCPProofFilePath(dir, "doomed"))
		return os.IsNotExist(statErr)
	}, 5*time.Second, 10*time.Millisecond, "proof file removed on session delete")
	assert.FileExists(t, instance.MCPProofFilePath(dir, "kept"), "other sessions untouched")

	cancel()
	<-done
}

func TestWireMCPProofs_should_InstallNothing_When_HookProofSecretMissing(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	prevKeys, prevMcp := approvals.DefaultHookProofs(), approvals.DefaultMcpProofs()
	approvals.SetDefaultHookProofs(nil)
	approvals.SetDefaultMcpProofs(nil)
	t.Cleanup(func() {
		approvals.SetDefaultHookProofs(prevKeys)
		approvals.SetDefaultMcpProofs(prevMcp)
	})

	wireMCPProofs(context.Background(), &ServerDependencies{}, func(func()) {})

	assert.Nil(t, approvals.DefaultMcpProofs(), "fail closed: nothing verifies")
}

type fakeLiveFinder struct{ live map[string]bool }

func (f fakeLiveFinder) FindLiveInstance(id string) *instance.Instance {
	if f.live[id] {
		return &instance.Instance{UUID: id}
	}
	return nil
}

func TestNewMCPIdentityMiddleware_should_VerifyThroughDefaultProofs(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	keys, err := approvals.NewHookProofs(dir)
	require.NoError(t, err)
	proofs := approvals.NewMcpProofs(keys, dir)
	prev := approvals.DefaultMcpProofs()
	t.Cleanup(func() { approvals.SetDefaultMcpProofs(prev) })

	var sawClaim, sawProofHeader bool
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		sawClaim = r.Header.Get("X-Stapler-Session-UUID") == "live"
		sawProofHeader = r.Header.Get(instance.MCPProofHeader) != ""
	})
	mw := newMCPIdentityMiddleware(next, fakeLiveFinder{live: map[string]bool{"live": true}})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("X-Stapler-Session-UUID", "live")
	req.Header.Set(instance.MCPProofHeader, proofs.HeaderValue("live"))

	// Before wiring (no default proofs) the request is simply unverified, never an error.
	approvals.SetDefaultMcpProofs(nil)
	mw.ServeHTTP(httptest.NewRecorder(), req)
	assert.True(t, sawClaim)
	assert.True(t, sawProofHeader)
}
