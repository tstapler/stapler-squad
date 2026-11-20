package wsconn

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// fastLaneOffRegistry is the process registry with the resync fast-lane default inverted to off, so a
// site that read the global (default on, installed by TestMain) is distinguishable from one that read this.
func fastLaneOffRegistry() *featureflags.Registry {
	specs := services.FlagRegistry().Specs()
	for i := range specs {
		if specs[i].Name == featureflags.TerminalResyncExecGateFastLaneFlagName {
			specs[i].Default = false
		}
	}
	return featureflags.NewRegistry(specs)
}

func TestCurrentResyncOptions_ShouldResolveFastLaneDefaultThroughInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.True(t, featureflags.Current().Default(featureflags.TerminalResyncExecGateFastLaneFlagName), "precondition: global default is on")

	assert.False(t, currentResyncOptions(fastLaneOffRegistry()).UseFastLane, "injected registry wins")
	assert.True(t, currentResyncOptions(nil).UseFastLane, "nil falls back to the process registry at read time")
}

func TestConnectRPCWebSocketHandler_ShouldTakeRegistryInjectedIntoSessionService(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	reg := fastLaneOffRegistry()
	svc, err := services.NewSessionServiceFromConfig(reg)
	require.NoError(t, err)

	h := NewConnectRPCWebSocketHandler(svc, nil, nil)

	assert.Same(t, reg, h.flags)
	assert.False(t, h.resyncOptionsFor(nil).UseFastLane, "mid-stream resync sites resolve through the handler's registry")
	assert.Nil(t, NewConnectRPCWebSocketHandler(nil, nil, nil).flags, "no injected registry stays nil; the fallback is lazy")
}

func TestCapturePaneStreamParams_ShouldResolveResyncOptionsThroughItsRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.False(t, capturePaneStreamParams{flags: fastLaneOffRegistry()}.resyncOptions().UseFastLane)
	assert.True(t, capturePaneStreamParams{}.resyncOptions().UseFastLane)
}

func TestShellStreamParams_ShouldResolveResyncOptionsThroughItsRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.False(t, shellStreamParams{flags: fastLaneOffRegistry()}.resyncOptions().UseFastLane)
	assert.True(t, shellStreamParams{}.resyncOptions().UseFastLane)
}

func TestHandleCapturePaneCurrentPaneRequest_ShouldRouteLaneByInjectedRegistry(t *testing.T) {
	for _, tc := range []struct {
		name         string
		flags        *featureflags.Registry
		wantFastLane bool
	}{
		{"injected default off", fastLaneOffRegistry(), false},
		{"nil falls back to global default on", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svctest.WithIsolatedStateDir(t)
			stream, _, cleanup := createTestWebSocketPair(t)
			defer cleanup()
			target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
			p := capturePaneStreamParams{
				writer:              testWriter(),
				stream:              stream,
				flags:               tc.flags,
				cpt:                 capturePaneTarget{sessionID: "s", target: target},
				paneCaptureSettling: new(atomic.Bool),
			}

			handleCapturePaneCurrentPaneRequest(p, &sessionv1.CurrentPaneRequest{})

			assert.Equal(t, tc.wantFastLane, target.capturePanePriorityCalled > 0)
			assert.Equal(t, !tc.wantFastLane, target.capturePaneCalled > 0)
		})
	}
}
