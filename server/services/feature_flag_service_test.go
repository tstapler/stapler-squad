package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/interceptors"
)

// TestFeatureFlagService_should_ToggleForwardingBehaviorForNextRequestOnly_When_UpdateFeatureFlagCalledMidSession
// is REQ-13's integration test (validation.md): terminalAppScrollForwardingClaudeFlagName
// is re-read fresh from config on every scrollbackResultForRequest call, not
// cached from an earlier request in the same session -- flipping it mid-session
// (the same live-settable path UpdateFeatureFlag drives) changes only the next
// request's behavior. Reuses the nil-*session.Instance-panic proof
// TestScrollbackResultForRequest_should_SkipAppScrollGate_When_FlagIsOff and
// TestScrollbackResultForRequest_should_AttemptAppScrollGate_When_FlagIsOn
// (connectrpc_websocket_test.go) already established for "was AppScrollGate
// reached" -- decisive because AppScrollGate(nil, ...) panics immediately.
func TestFeatureFlagService_should_ToggleForwardingBehaviorForNextRequestOnly_When_UpdateFeatureFlagCalledMidSession(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalAppScrollForwardingClaudeFlagName, true))
	assertScrollbackRequestReachesAppScrollGate(t, "native")

	// Flip the flag mid-session, the same live-settable path UpdateFeatureFlag
	// drives (never an env var/restart gate, per this repo's rollout-flag
	// convention).
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalAppScrollForwardingClaudeFlagName, false))
	assertScrollbackRequestSkipsAppScrollGate(t)
}

// assertScrollbackRequestReachesAppScrollGate asserts a nil-*session.Instance
// request reaches AppScrollGate -- decisive because AppScrollGate(nil, ...)
// panics immediately (inst.GetProgram() dereferences a nil receiver's
// field), mirroring TestScrollbackResultForRequest_should_AttemptAppScrollGate_When_FlagIsOn's
// (connectrpc_websocket_test.go) proof technique.
func assertScrollbackRequestReachesAppScrollGate(t *testing.T, fallbackContent string) {
	t.Helper()
	reachedGate := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				reachedGate = true
			}
		}()
		_, _ = scrollbackResultForRequest(scrollbackRequestParams{
			startLine: "-100",
			endLine:   "-1",
			logPrefix: "[test]",
			fallback:  func(string, string) (string, error) { return fallbackContent, nil },
		})
	}()
	require.True(t, reachedGate, "expected AppScrollGate to be reached")
}

// assertScrollbackRequestSkipsAppScrollGate asserts the fallback runs and
// AppScrollGate is never reached, mirroring
// TestScrollbackResultForRequest_should_SkipAppScrollGate_When_FlagIsOff's
// (connectrpc_websocket_test.go) proof technique.
func assertScrollbackRequestSkipsAppScrollGate(t *testing.T) {
	t.Helper()
	fallbackCalled := false
	result, err := scrollbackResultForRequest(scrollbackRequestParams{
		startLine: "-100",
		endLine:   "-1",
		logPrefix: "[test]",
		fallback: func(string, string) (string, error) {
			fallbackCalled = true
			return "tmux-native content", nil
		},
	})

	require.NoError(t, err)
	require.True(t, fallbackCalled, "expected the fallback to be used instead of AppScrollGate")
	require.Nil(t, result.AppScroll)
}

type probeStubHandler struct {
	sessionv1connect.UnimplementedSessionServiceHandler
}

func (probeStubHandler) ProbeProgram(context.Context, *connect.Request[sessionv1.ProbeProgramRequest]) (*connect.Response[sessionv1.ProbeProgramResponse], error) {
	return connect.NewResponse(&sessionv1.ProbeProgramResponse{Found: true}), nil
}

// TestProgramCLIFlagProbe_should_GateProbeProgramPerRequest_When_FlagToggled drives a real
// Connect handler wrapped with the same scoped interceptor server.go registers.
func TestProgramCLIFlagProbe_should_GateProbeProgramPerRequest_When_FlagToggled(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	path, handler := sessionv1connect.NewSessionServiceHandler(probeStubHandler{}, connect.WithInterceptors(
		interceptors.NewScopedFeatureFlagInterceptor(programCLIFlagProbeFlagName, ProgramCLIFlagProbeEnabled, ProgramCLIFlagProbeGatedMethod)))
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	client := sessionv1connect.NewSessionServiceClient(ts.Client(), ts.URL)
	call := func() error {
		_, err := client.ProbeProgram(context.Background(), connect.NewRequest(&sessionv1.ProbeProgramRequest{Command: "claude"}))
		return err
	}

	require.NoError(t, call(), "default is on when never persisted")
	require.NoError(t, config.LoadConfig().SetFeatureFlag(programCLIFlagProbeFlagName, false))
	require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(call()))
	require.NoError(t, config.LoadConfig().SetFeatureFlag(programCLIFlagProbeFlagName, true))
	require.NoError(t, call())
}

func TestFeatureFlagService_should_ListProbeFlagDefaultOn_When_NeverPersisted(t *testing.T) {
	require.True(t, featureFlagDefault(programCLIFlagProbeFlagName))
}
