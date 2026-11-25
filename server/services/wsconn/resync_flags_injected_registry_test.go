package wsconn

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// fastLaneOffRegistry is the default registry with the resync fast-lane default inverted to off, so a
// site that read the default (fast lane on) is distinguishable from one that read this.
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
	require.True(t, defaultFlags().Default(featureflags.TerminalResyncExecGateFastLaneFlagName), "precondition: default is on")

	assert.False(t, currentResyncOptions(fastLaneOffRegistry()).UseFastLane, "injected registry wins")
	assert.True(t, currentResyncOptions(defaultFlags()).UseFastLane, "default registry has the fast lane on")
}

func TestConnectRPCWebSocketHandler_ShouldTakeRegistryInjectedIntoSessionService(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	reg := fastLaneOffRegistry()
	svc, err := services.NewSessionServiceFromConfig(reg)
	require.NoError(t, err)

	h := NewConnectRPCWebSocketHandler(svc, nil, nil)

	assert.Same(t, reg, h.flags)
	assert.False(t, h.resyncOptionsFor(nil).UseFastLane, "mid-stream resync sites resolve through the handler's registry")
	assert.NotNil(t, NewConnectRPCWebSocketHandler(nil, nil, nil).flags, "no session service: construction-time default registry")
}

func TestCapturePaneStreamParams_ShouldResolveResyncOptionsThroughItsRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.False(t, capturePaneStreamParams{flags: fastLaneOffRegistry()}.resyncOptions().UseFastLane)
	assert.True(t, capturePaneStreamParams{flags: defaultFlags()}.resyncOptions().UseFastLane)
}

func TestShellStreamParams_ShouldResolveResyncOptionsThroughItsRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.False(t, shellStreamParams{flags: fastLaneOffRegistry()}.resyncOptions().UseFastLane)
	assert.True(t, shellStreamParams{flags: defaultFlags()}.resyncOptions().UseFastLane)
}

func TestHandleCapturePaneCurrentPaneRequest_ShouldRouteLaneByInjectedRegistry(t *testing.T) {
	for _, tc := range []struct {
		name         string
		flags        *featureflags.Registry
		wantFastLane bool
	}{
		{"injected default off", fastLaneOffRegistry(), false},
		{"default registry fast lane on", defaultFlags(), true},
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

// resyncFake is a fakePanePTY that also satisfies resyncTarget.
type resyncFake struct{ *fakePanePTY }

func (resyncFake) IsBackendProcessAlive() bool { return true }

func testWriterFn() access.TerminalWriter { return testWriter() }

func TestCurrentPaneAnswerer_ShouldRouteLaneByHandlerRegistry(t *testing.T) {
	for _, tc := range []struct {
		name         string
		flags        *featureflags.Registry
		wantFastLane bool
	}{
		{"injected default off", fastLaneOffRegistry(), false},
		{"default registry fast lane on", defaultFlags(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svctest.WithIsolatedStateDir(t)
			target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
			h := &ConnectRPCWebSocketHandler{flags: tc.flags}

			answer := h.currentPaneAnswerer("s", resyncFake{target}, testWriterFn)
			_, err := answer(context.Background(), &sessionv1.CurrentPaneRequest{})

			require.NoError(t, err)
			assert.Equal(t, tc.wantFastLane, target.capturePanePriorityCalled > 0)
			assert.Equal(t, !tc.wantFastLane, target.capturePaneCalled > 0)
		})
	}
}

func TestWithStreamDeps_ShouldCarryHandlerRegistry(t *testing.T) {
	reg := fastLaneOffRegistry()
	h := &ConnectRPCWebSocketHandler{flags: reg}
	assert.Same(t, reg, h.withStreamDeps(capturePaneStreamParams{}).flags)
	assert.Same(t, reg, h.withShellStreamDeps(shellStreamParams{}).flags)
	assert.Nil(t, (&ConnectRPCWebSocketHandler{}).withShellStreamDeps(shellStreamParams{}).flags)
}

func TestHandleShellMidStreamCurrentPaneRequest_ShouldRouteLaneByInjectedRegistry(t *testing.T) {
	for _, tc := range []struct {
		name         string
		flags        *featureflags.Registry
		wantFastLane bool
	}{
		{"injected default off", fastLaneOffRegistry(), false},
		{"default registry fast lane on", defaultFlags(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svctest.WithIsolatedStateDir(t)
			stream, _, cleanup := createTestWebSocketPair(t)
			defer cleanup()
			target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
			p := shellStreamParams{
				writer:         testWriter(),
				stream:         stream,
				flags:          tc.flags,
				cursorTarget:   target,
				sessionID:      "s",
				shellID:        "sh",
				resizeSettling: new(atomic.Bool),
			}

			handleShellMidStreamCurrentPaneRequest(p, &sessionv1.CurrentPaneRequest{})

			assert.Equal(t, tc.wantFastLane, target.capturePanePriorityCalled > 0)
			assert.Equal(t, !tc.wantFastLane, target.capturePaneCalled > 0)
		})
	}
}

// The stream closures are not drivable without a full stream, so pin that every production resync
// goes through the registry-aware resolvers: no site builds options or answers a pane on its own.
func TestResyncOptionsConstruction_ShouldStayInsideRegistryAwareResolvers(t *testing.T) {
	count := func(needle string) (n int) {
		files, err := filepath.Glob("*.go")
		require.NoError(t, err)
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			require.NoError(t, err)
			n += strings.Count(string(b), needle)
		}
		return n
	}
	// definition + the one call inside currentPaneAnswerer
	assert.Equal(t, 2, count("answerCurrentPane("), "answerCurrentPane is called only by currentPaneAnswerer")
	// definition + handler/capture/shell resolvers
	assert.Equal(t, 4, count("currentResyncOptionsFor("), "options are built only by the three registry-aware resolvers")
	assert.Equal(t, 2, count("h.currentPaneAnswerer("), "the two stream closures")
}
