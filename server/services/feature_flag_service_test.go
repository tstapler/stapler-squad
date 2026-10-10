package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestFeatureFlagService_should_ListNotificationTrayV2DefaultOff_When_NeverPersisted(t *testing.T) {
	registered := false
	for _, kf := range knownFeatureFlags {
		if kf.name == notificationTrayV2FlagName {
			registered = true
		}
	}
	require.True(t, registered, "notification_tray_v2 must be in knownFeatureFlags")
	require.False(t, featureFlagDefault(notificationTrayV2FlagName))
}

// recordingObserver is a deliverygate.FlagObserver that records the names it is told about.
type recordingObserver struct {
	mu    sync.Mutex
	names []string
}

func (r *recordingObserver) OnFlagChanged(name string) {
	r.mu.Lock()
	r.names = append(r.names, name)
	r.mu.Unlock()
}

func (r *recordingObserver) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.names...)
}

func flagUpdate(svc *FeatureFlagService, name string, enabled bool) error {
	_, err := svc.UpdateFeatureFlag(context.Background(),
		connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: name, Enabled: enabled}))
	return err
}

func statusDetailOf(t *testing.T, svc *FeatureFlagService, name string) string {
	t.Helper()
	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	for _, f := range resp.Msg.Flags {
		if f.Name == name {
			return f.StatusDetail
		}
	}
	t.Fatalf("flag %q not listed", name)
	return ""
}

func TestUpdateFeatureFlag_ShouldNotifyTheFlagObserverAfterEveryPersistedChange_WhenFlagIsFlippedOrRolledBack(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	obs := &recordingObserver{}
	svc.SetFlagObserver(obs)

	require.NoError(t, flagUpdate(svc, "backlog", true))
	require.Equal(t, []string{"backlog"}, obs.seen())

	svc.SetFeatureController("backlog", &fakeFeatureController{failDisable: errors.New("boom")})
	require.Error(t, flagUpdate(svc, "backlog", false))
	require.Equal(t, []string{"backlog", "backlog"}, obs.seen(), "the rollback also reaches the observer")
}

func TestUpdateFeatureFlag_ShouldRefuseOnlyEnablingWithFailedPrecondition_WhenEnableGuardReportsAReason(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	reason := "stats writer not running"
	svc.SetEnableGuard("backlog", func() string { return reason })

	err := flagUpdate(svc, "backlog", true)
	require.Error(t, err)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	require.Contains(t, err.Error(), reason)
	_, ok := config.LoadConfig().GetFeatureFlagOverride("backlog")
	require.False(t, ok, "a refused enable persists nothing")

	require.NoError(t, flagUpdate(svc, "backlog", false), "disabling is never refused by the guard")

	reason = ""
	require.NoError(t, flagUpdate(svc, "backlog", true), "allowed once the precondition holds")
}

func TestStatusDetail_ShouldJoinEverySourceInRegistrationOrderAndKeepTheSetterSlot_WhenCompositeProviderUsed(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	sources := map[string]func() string{
		"stats writer not running":    func() string { return "stats writer not running" },
		"Gate is OFF":                 func() string { return "Gate is OFF" },
		"Gate is OFF for kind review": func() string { return "Gate is OFF for kind review" },
	}
	names := []string{"stats writer not running", "Gate is OFF", "Gate is OFF for kind review"}
	perms := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, p := range perms {
		svc := NewFeatureFlagService()
		want := make([]string, 0, 3)
		for _, i := range p {
			svc.AddStatusDetailSource("backlog", sources[names[i]])
			want = append(want, names[i])
		}
		require.Equal(t, strings.Join(want, "; "), statusDetailOf(t, svc, "backlog"), "order %v", p)
	}

	svc := NewFeatureFlagService()
	require.Equal(t, "", statusDetailOf(t, svc, "backlog"), "an empty detail means no source contributed")
	svc.AddStatusDetailSource("backlog", func() string { return "" })
	require.Equal(t, "", statusDetailOf(t, svc, "backlog"))

	svc.SetStatusDetailProvider("backlog", func() string { return "paused by quota" })
	svc.AddStatusDetailSource("backlog", func() string { return "extra" })
	svc.SetStatusDetailProvider("backlog", func() string { return "resumed" })
	require.Equal(t, "resumed; extra", statusDetailOf(t, svc, "backlog"), "the setter replaces only its own slot")
}
