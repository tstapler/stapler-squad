package services

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/interceptors"
	"github.com/tstapler/stapler-squad/session"
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
	withIsolatedStateDir(t)
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
			writer:    testWriter(),
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
		writer:    testWriter(),
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
	withIsolatedStateDir(t)
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

// Pre-mortem #3: the flag service and the quota RPC both read-modify-write
// config.json; both must go through config.UpdateConfig so neither loses a write.
func TestFeatureFlagWrites_AndUpdateQuotaConfig_NoLostUpdate_UnderRace(t *testing.T) {
	withIsolatedStateDir(t)
	const flag = "browser-passthrough"
	const iterations = 50
	flags := NewFeatureFlagService()
	quota := NewQuotaSettingsService()

	errCh := make(chan error, 2*iterations)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 1; i <= iterations; i++ {
			errCh <- flagUpdate(flags, flag, i%2 == 0)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 1; i <= iterations; i++ {
			_, err := quota.UpdateQuotaConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateQuotaConfigRequest{
				Settings: &sessionv1.QuotaSettings{Enabled: proto.Bool(i%2 == 0)},
			}))
			errCh <- err
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	cfg := config.LoadConfig()
	got, ok := cfg.GetFeatureFlagOverride(flag)
	require.True(t, ok, "flag write lost")
	require.True(t, got, "flag holds its last written value")
	require.True(t, cfg.Quota.Enabled, "quota Enabled lost to a flag write")

	resp, err := flags.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	for _, f := range resp.Msg.GetFlags() {
		if f.GetName() == flag {
			require.True(t, f.GetEnabled(), "the service reads back the stored value")
		}
	}
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
	withIsolatedStateDir(t)
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
	withIsolatedStateDir(t)
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
	withIsolatedStateDir(t)
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

func TestFeatureFlagStatusDetail_ReportsInertAndSource_WhenStallNudgeOnDiagnoseOff(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()

	require.Equal(t, "Default: Off (not set explicitly)", statusDetailOf(t, svc, config.AutonomousStallNudgeFeatureFlag))
	require.Equal(t, "Default: Off (not set explicitly)", statusDetailOf(t, svc, config.DiagnoseNudgeFeatureFlag))

	require.NoError(t, flagUpdate(svc, config.AutonomousStallNudgeFeatureFlag, true))
	require.Equal(t, "inactive: requires diagnose_nudge_enabled; Set explicitly: On",
		statusDetailOf(t, svc, config.AutonomousStallNudgeFeatureFlag))

	require.NoError(t, flagUpdate(svc, config.DiagnoseNudgeFeatureFlag, true))
	require.Equal(t, "Set explicitly: On", statusDetailOf(t, svc, config.AutonomousStallNudgeFeatureFlag))
	require.Equal(t, "Set explicitly: On", statusDetailOf(t, svc, config.DiagnoseNudgeFeatureFlag))

	require.Empty(t, statusDetailOf(t, svc, "backlog"), "other flags gain no status line")
}

// --- workflow engine mode flags (Story 1.1.3) --------------------------------

type modeTestClock struct{ now time.Time }

func (c *modeTestClock) Now() time.Time          { return c.now }
func (c *modeTestClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func TestModeResolver_ReturnsShadow_WhenNoFlagSetAndReaderIsProductionReader(t *testing.T) {
	withIsolatedStateDir(t)
	cfg := config.LoadConfig()
	require.NotContains(t, cfg.FeatureFlags, config.WorkflowEngineShadowFeatureFlag, "fixture: nothing is set")
	require.NotContains(t, cfg.FeatureFlags, config.WorkflowEngineEnforceFeatureFlag, "fixture: nothing is set")

	src := NewWorkflowEngineModeSource(config.LoadConfig, nil)

	require.Equal(t, session.ModeShadow, src.Mode(), "an unset shadow flag resolves to its registered default (on), not Off")
}

func TestModeFlags_DefaultShadowOnEnforceOff_AndComposeToThreeLegalModes(t *testing.T) {
	t.Parallel()
	require.True(t, featureFlagDefault(config.WorkflowEngineShadowFeatureFlag))
	require.False(t, featureFlagDefault(config.WorkflowEngineEnforceFeatureFlag))
	require.Equal(t, config.WorkflowEngineShadowDefault, featureFlagDefault(config.WorkflowEngineShadowFeatureFlag), "declared once in config")
	require.Equal(t, config.WorkflowEngineEnforceDefault, featureFlagDefault(config.WorkflowEngineEnforceFeatureFlag), "declared once in config")

	set := func(flags map[string]bool) *config.Config { return &config.Config{FeatureFlags: flags} }
	shadow, enforce := config.WorkflowEngineShadowFeatureFlag, config.WorkflowEngineEnforceFeatureFlag
	cases := []struct {
		name string
		cfg  *config.Config
		want session.EngineMode
	}{
		{"nil config", nil, session.ModeShadow},
		{"nothing set", set(nil), session.ModeShadow},
		{"shadow off", set(map[string]bool{shadow: false}), session.ModeOff},
		{"enforce on", set(map[string]bool{enforce: true}), session.ModeEnforce},
		{"enforce on, shadow off", set(map[string]bool{shadow: false, enforce: true}), session.ModeEnforce},
		{"both explicit on", set(map[string]bool{shadow: true, enforce: true}), session.ModeEnforce},
		{"both explicit off", set(map[string]bool{shadow: false, enforce: false}), session.ModeOff},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, WorkflowEngineModeFromConfig(tc.cfg), tc.name)
	}
}

func TestWorkflowEngineModeFlags_AreLive_WhenToggledThroughUpdateFeatureFlag(t *testing.T) {
	withIsolatedStateDir(t)
	clock := &modeTestClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	src := NewWorkflowEngineModeSource(config.LoadConfig, clock)
	svc := NewFeatureFlagService()
	require.Equal(t, session.ModeShadow, src.Mode())

	require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceFeatureFlag, true))
	require.Equal(t, session.ModeShadow, src.Mode(), "the cached mode is reused inside the TTL")
	clock.Advance(workflowEngineModeTTL + time.Millisecond)
	require.Equal(t, session.ModeEnforce, src.Mode(), "enforce applies after at most the TTL, no restart")

	require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceFeatureFlag, false))
	clock.Advance(workflowEngineModeTTL + time.Millisecond)
	require.Equal(t, session.ModeShadow, src.Mode())
}

func TestWorkflowEngineModeFlags_RegistryDefaultsMatchConfigAndProductionReader(t *testing.T) {
	withIsolatedStateDir(t)
	got := map[string]bool{}
	for _, kf := range knownFeatureFlags {
		got[kf.name] = kf.defaultValue
	}
	require.Equal(t, config.WorkflowEngineShadowDefault, got[config.WorkflowEngineShadowFeatureFlag])
	require.Equal(t, config.WorkflowEngineEnforceDefault, got[config.WorkflowEngineEnforceFeatureFlag])

	// GetFeatureFlags reports the same defaults the production reader resolves.
	resp, err := NewFeatureFlagService().GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	reported := map[string]bool{}
	for _, f := range resp.Msg.Flags {
		reported[f.Name] = f.Enabled
	}
	require.True(t, reported[config.WorkflowEngineShadowFeatureFlag])
	require.False(t, reported[config.WorkflowEngineEnforceFeatureFlag])
	require.Equal(t, session.ModeShadow, NewWorkflowEngineModeSource(config.LoadConfig, nil).Mode())
}

// plainFlagReads returns "file:line" for each plain GetFeatureFlag call (which
// returns false for an unset flag) whose argument names one of the mode flags.
func plainFlagReads(t *testing.T, filename, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	require.NoError(t, err)
	names := map[string]bool{
		config.WorkflowEngineShadowFeatureFlag: true, config.WorkflowEngineEnforceFeatureFlag: true,
		"WorkflowEngineShadowFeatureFlag": true, "WorkflowEngineEnforceFeatureFlag": true,
	}
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "GetFeatureFlag" {
			return true
		}
		for _, arg := range call.Args {
			ast.Inspect(arg, func(a ast.Node) bool {
				switch v := a.(type) {
				case *ast.BasicLit:
					if s, err := strconv.Unquote(v.Value); err == nil && names[s] {
						hits = append(hits, fset.Position(v.Pos()).String())
					}
				case *ast.Ident:
					if names[v.Name] {
						hits = append(hits, fset.Position(v.Pos()).String())
					}
				}
				return true
			})
		}
		return true
	})
	return hits
}

func TestWorkflowEngineModeFlags_NoNonTestCodeReadsThemWithPlainGetFeatureFlag(t *testing.T) {
	t.Parallel()
	bad := `package x
func f(c *C) bool { return c.GetFeatureFlag("backlog:workflow-engine-enforce") || c.GetFeatureFlag(config.WorkflowEngineShadowFeatureFlag) }`
	require.Len(t, plainFlagReads(t, "bad.go", bad), 2, "the scanner must catch both literal and constant reads")
	good := `package x
func f(c *C) bool { return c.GetFeatureFlagWithDefault(config.WorkflowEngineShadowFeatureFlag, true) }`
	require.Empty(t, plainFlagReads(t, "good.go", good))

	root := filepath.Join("..", "..")
	skip := map[string]bool{"node_modules": true, "web-app": true, "gen": true, ".git": true, "ent": true, "tests": true, "docs": true, "project_plans": true, ".claude": true}
	var hits []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] && path != root {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path) // #nosec G304 -- walking this repo's own source
		if err != nil {
			return err
		}
		if !strings.Contains(string(src), "GetFeatureFlag(") {
			return nil
		}
		hits = append(hits, plainFlagReads(t, path, string(src))...)
		return nil
	}))
	require.Empty(t, hits, "read the mode flags with GetFeatureFlagWithDefault and the registered default")
}

// modeFlagObservable runs one CanTransition through a selector driven by the
// production mode reader and returns what a caller can observe.
func modeFlagObservable(t *testing.T, flag string, enabled bool, observe func(*session.SwitchableWorkflowEngine, *countingShadow) any) any {
	t.Helper()
	clock := &modeTestClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	svc := NewFeatureFlagService()
	require.NoError(t, flagUpdate(svc, config.WorkflowEngineShadowFeatureFlag, true))
	require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceFeatureFlag, false))
	require.NoError(t, flagUpdate(svc, flag, enabled))
	rec := &countingShadow{}
	sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), nil,
		NewWorkflowEngineModeSource(config.LoadConfig, clock), session.WithShadowRecorder(rec))
	return observe(sel, rec)
}

type countingShadow struct{ calls atomic.Int64 }

func (c *countingShadow) RecordShadowCall(session.EngineMethod, session.CallPath) { c.calls.Add(1) }
func (c *countingShadow) RecordDivergence(session.EngineMethod, session.DivergenceClass, string, session.CallPath) {
}
func (c *countingShadow) RecordNotCompared(session.NotComparedReason) {}

func TestConfigSurfaceProof_WorkflowEngineShadow_ChangesShadowCallsRecorded(t *testing.T) {
	withIsolatedStateDir(t)
	assertConfigChangesBehavior(t, func(setting any) any {
		return modeFlagObservable(t, config.WorkflowEngineShadowFeatureFlag, setting.(bool),
			func(sel *session.SwitchableWorkflowEngine, rec *countingShadow) any {
				sel.CanTransition(session.BacklogStatusIdea, session.BacklogStatusReady, nil)
				return rec.calls.Load()
			})
	}, false, true)
}

func TestConfigSurfaceProof_WorkflowEngineEnforce_ChangesTransitionAnswer(t *testing.T) {
	withIsolatedStateDir(t)
	assertConfigChangesBehavior(t, func(setting any) any {
		return modeFlagObservable(t, config.WorkflowEngineEnforceFeatureFlag, setting.(bool),
			func(sel *session.SwitchableWorkflowEngine, _ *countingShadow) any {
				// archived->archived is system-required: only the enforce composition allows it.
				return sel.CanTransition(session.BacklogStatusArchived, session.BacklogStatusArchived, nil)
			})
	}, false, true)
}

// --- Story 1.5.2: ModeFor(class) over the class-to-flag table ------------------

// TestModeFor_ComposesFourFlagsPerClass_AndDefaultsToShadow covers every class's
// enforce flag: operator (Story 1.5.2), agent (Task 1.5.3c), system (Task 1.5.8g).
func TestModeFor_ComposesFourFlagsPerClass_AndDefaultsToShadow(t *testing.T) {
	t.Parallel()
	shadow, enforce, enforceAgent := config.WorkflowEngineShadowFeatureFlag, config.WorkflowEngineEnforceFeatureFlag, config.WorkflowEngineEnforceAgentFeatureFlag
	enforceSystem := config.WorkflowEngineEnforceSystemFeatureFlag
	set := func(flags map[string]bool) *config.Config { return &config.Config{FeatureFlags: flags} }

	cases := []struct {
		name  string
		class session.CallerClass
		cfg   *config.Config
		want  session.EngineMode
	}{
		{"operator, empty config", session.ClassOperator, set(nil), session.ModeShadow},
		{"operator, nil config", session.ClassOperator, nil, session.ModeShadow},
		{"operator, shadow off", session.ClassOperator, set(map[string]bool{shadow: false}), session.ModeOff},
		{"operator, enforce wins over shadow on", session.ClassOperator, set(map[string]bool{shadow: true, enforce: true}), session.ModeEnforce},
		{"operator, enforce wins over shadow off", session.ClassOperator, set(map[string]bool{shadow: false, enforce: true}), session.ModeEnforce},
		{"operator, shadow on enforce off", session.ClassOperator, set(map[string]bool{shadow: true, enforce: false}), session.ModeShadow},
		{"agent, empty config", session.ClassAgent, set(nil), session.ModeShadow},
		{"agent, nil config", session.ClassAgent, nil, session.ModeShadow},
		{"agent, shadow off", session.ClassAgent, set(map[string]bool{shadow: false}), session.ModeOff},
		{"agent, enforce wins over shadow on", session.ClassAgent, set(map[string]bool{shadow: true, enforceAgent: true}), session.ModeEnforce},
		{"agent, enforce wins over shadow off", session.ClassAgent, set(map[string]bool{shadow: false, enforceAgent: true}), session.ModeEnforce},
		{"agent, shadow on enforce off", session.ClassAgent, set(map[string]bool{shadow: true, enforceAgent: false}), session.ModeShadow},
		// The operator flag must not leak into the agent class, nor the agent flag into the operator.
		{"agent, operator enforce on", session.ClassAgent, set(map[string]bool{enforce: true}), session.ModeShadow},
		{"operator, agent enforce on", session.ClassOperator, set(map[string]bool{enforceAgent: true}), session.ModeShadow},
		{"system, empty config", session.ClassSystem, set(nil), session.ModeShadow},
		{"system, nil config", session.ClassSystem, nil, session.ModeShadow},
		{"system, shadow off", session.ClassSystem, set(map[string]bool{shadow: false}), session.ModeOff},
		{"system, enforce wins over shadow on", session.ClassSystem, set(map[string]bool{shadow: true, enforceSystem: true}), session.ModeEnforce},
		{"system, enforce wins over shadow off", session.ClassSystem, set(map[string]bool{shadow: false, enforceSystem: true}), session.ModeEnforce},
		{"system, shadow on enforce off", session.ClassSystem, set(map[string]bool{shadow: true, enforceSystem: false}), session.ModeShadow},
		// No class's flag leaks into another.
		{"system, agent enforce on", session.ClassSystem, set(map[string]bool{enforceAgent: true}), session.ModeShadow},
		{"system, operator enforce on", session.ClassSystem, set(map[string]bool{enforce: true}), session.ModeShadow},
		{"agent, system enforce on", session.ClassAgent, set(map[string]bool{enforceSystem: true}), session.ModeShadow},
		{"operator, system enforce on", session.ClassOperator, set(map[string]bool{enforceSystem: true}), session.ModeShadow},
		// A class with no registered enforce flag resolves from the shadow flag alone.
		{"unknown class, system enforce on", session.CallerClass("bogus"), set(map[string]bool{enforceSystem: true}), session.ModeShadow},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, session.ModeForClassFlags(tc.class, tc.cfg), tc.name)
	}
}

func TestModeFor_OperatorRowMatchesProductionReader_ForEveryFlagCombination(t *testing.T) {
	t.Parallel()
	shadow, enforce := config.WorkflowEngineShadowFeatureFlag, config.WorkflowEngineEnforceFeatureFlag
	values := []*bool{nil, ptrBool(false), ptrBool(true)}
	for _, s := range values {
		for _, e := range values {
			flags := map[string]bool{}
			if s != nil {
				flags[shadow] = *s
			}
			if e != nil {
				flags[enforce] = *e
			}
			cfg := &config.Config{FeatureFlags: flags}
			require.Equalf(t, WorkflowEngineModeFromConfig(cfg), session.ModeForClassFlags(session.ClassOperator, cfg), "flags=%v", flags)
		}
	}
}

func ptrBool(b bool) *bool { return &b }

func TestClassEnforceFlagTable_EveryRowIsARegisteredFlagWithTheRegisteredDefault(t *testing.T) {
	t.Parallel()
	name, def, ok := session.ClassEnforceFlag(session.ClassOperator)
	require.True(t, ok)
	require.Equal(t, config.WorkflowEngineEnforceFeatureFlag, name, "operator is bound to the existing enforce flag")
	require.Equal(t, featureFlagDefault(name), def, "table default equals the registry default")

	for _, class := range []session.CallerClass{session.ClassOperator, session.ClassAgent, session.ClassSystem} {
		name, _, ok := session.ClassEnforceFlag(class)
		if !ok {
			continue // not registered yet: Task 1.5.8g (system)
		}
		registered := false
		for _, kf := range knownFeatureFlags {
			registered = registered || kf.name == name
		}
		require.Truef(t, registered, "class %s row names %q, which must be in knownFeatureFlags", class, name)
	}
	agentName, agentDef, ok := session.ClassEnforceFlag(session.ClassAgent)
	require.True(t, ok, "the agent flag is registered in Task 1.5.3c")
	require.Equal(t, config.WorkflowEngineEnforceAgentFeatureFlag, agentName)
	require.Equal(t, featureFlagDefault(agentName), agentDef, "table default equals the registry default")
	require.False(t, agentDef, "the agent flag ships off")
	systemName, systemDef, ok := session.ClassEnforceFlag(session.ClassSystem)
	require.True(t, ok, "the system flag is registered in Task 1.5.8g")
	require.Equal(t, config.WorkflowEngineEnforceSystemFeatureFlag, systemName)
	require.Equal(t, featureFlagDefault(systemName), systemDef, "table default equals the registry default")
	require.False(t, systemDef, "the system flag ships off")
}

func TestFlagClassModeSource_IsLivePerClass_AndOperatorIsMode(t *testing.T) {
	withIsolatedStateDir(t)
	clock := &modeTestClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	src := session.NewFlagClassModeSource(config.LoadConfig, workflowEngineModeTTL, clock)
	svc := NewFeatureFlagService()
	require.Equal(t, session.ModeShadow, src.ModeFor(session.ClassOperator))
	require.Equal(t, session.ModeShadow, src.ModeFor(session.ClassAgent))
	require.Equal(t, session.ModeOff, src.ModeFor(session.CallerClass("bogus")))

	require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceFeatureFlag, true))
	require.Equal(t, session.ModeShadow, src.ModeFor(session.ClassOperator), "cached inside the TTL")
	clock.Advance(workflowEngineModeTTL + time.Millisecond)
	require.Equal(t, session.ModeEnforce, src.ModeFor(session.ClassOperator))
	require.Equal(t, session.ModeEnforce, src.Mode(), "Mode() is the operator class")
	require.Equal(t, session.ModeShadow, src.ModeFor(session.ClassAgent), "the operator flag does not enforce another class")
	require.Equal(t, session.ModeShadow, src.ModeFor(session.ClassSystem))

	require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceAgentFeatureFlag, true))
	clock.Advance(workflowEngineModeTTL + time.Millisecond)
	require.Equal(t, session.ModeEnforce, src.ModeFor(session.ClassAgent))
	require.Equal(t, session.ModeShadow, src.ModeFor(session.ClassSystem), "the agent flag does not enforce the system class")

	require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceSystemFeatureFlag, true))
	clock.Advance(workflowEngineModeTTL + time.Millisecond)
	require.Equal(t, session.ModeEnforce, src.ModeFor(session.ClassSystem))
}

func TestConfigSurfaceProof_WorkflowEngineEnforceSystem_ChangesSystemTransitionAnswer(t *testing.T) {
	withIsolatedStateDir(t)
	spec, ok := session.LookupTransitionCaller(session.CallerAutonomousComplete) // system, unchecked today
	require.True(t, ok)
	assertConfigChangesBehavior(t, func(setting any) any {
		clock := &modeTestClock{now: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
		svc := NewFeatureFlagService()
		require.NoError(t, flagUpdate(svc, config.WorkflowEngineShadowFeatureFlag, true))
		require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceFeatureFlag, false))
		require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceAgentFeatureFlag, false))
		require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceSystemFeatureFlag, setting.(bool)))
		sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), nil, NewWorkflowEngineClassModeSource(config.LoadConfig, clock))
		// idea->done has no edge: an unchecked system write is allowed until its class enforces.
		return sel.ForCaller(spec).CanTransition(session.BacklogStatusIdea, session.BacklogStatusDone, nil)
	}, true, false)
}

func TestConfigSurfaceProof_WorkflowEngineEnforceAgent_ChangesAgentTransitionAnswer(t *testing.T) {
	withIsolatedStateDir(t)
	spec, ok := session.LookupTransitionCaller(session.CallerMCPResumeWork) // agent, unchecked today
	require.True(t, ok)
	assertConfigChangesBehavior(t, func(setting any) any {
		clock := &modeTestClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
		svc := NewFeatureFlagService()
		require.NoError(t, flagUpdate(svc, config.WorkflowEngineShadowFeatureFlag, true))
		require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceFeatureFlag, false))
		require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceAgentFeatureFlag, setting.(bool)))
		sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), nil, NewWorkflowEngineClassModeSource(config.LoadConfig, clock))
		// idea->done has no edge: an unchecked agent write is allowed until its class enforces.
		return sel.ForCaller(spec).CanTransition(session.BacklogStatusIdea, session.BacklogStatusDone, nil)
	}, true, false)
}

func TestSelectorModeFor_UsesTheClassAwareSource(t *testing.T) {
	withIsolatedStateDir(t)
	clock := &modeTestClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	svc := NewFeatureFlagService()
	require.NoError(t, flagUpdate(svc, config.WorkflowEngineEnforceFeatureFlag, true))
	sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), nil,
		session.NewFlagClassModeSource(config.LoadConfig, workflowEngineModeTTL, clock))
	require.Equal(t, session.ModeEnforce, sel.ModeFor(session.ClassOperator))
	require.Equal(t, session.ModeShadow, sel.ModeFor(session.ClassAgent))
}

func TestFilesFlags_ShouldDefaultOffAndOnAndBeLiveSettable_When_UpdateFeatureFlag(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	ctx := context.Background()
	helpers := []struct {
		name string
		read func(context.Context) bool
		want bool
	}{
		{"files:editing", FilesEditingEnabled, false},
		{"files:viewer-v2", FilesViewerV2Enabled, true},
		{"files:merge-phone", FilesMergePhoneEnabled, true},
		{"files:range-read", FilesRangeReadEnabled, false},
		{"files:delta-write", FilesDeltaWriteEnabled, false},
		{"files:edit-large", FilesEditLargeEnabled, false},
	}
	for _, h := range helpers {
		t.Run(h.name, func(t *testing.T) {
			require.Equal(t, h.want, h.read(ctx), "default on a fresh config")
			require.NoError(t, flagUpdate(svc, h.name, !h.want))
			require.Equal(t, !h.want, h.read(ctx), "flip applies on the next call without restart")
			require.NoError(t, flagUpdate(svc, h.name, h.want))
			require.Equal(t, h.want, h.read(ctx))
		})
	}
}

func TestFilesFlags_ShouldRejectScopedUpdate_When_ScopeSupplied(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	for _, name := range []string{
		"files:editing", "files:viewer-v2", "files:merge-phone",
		"files:range-read", "files:delta-write", "files:edit-large",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
				Name: name, Enabled: true, Mutation: sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED, Scope: "kind:review",
			}))
			require.Error(t, err)
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

// --- Story 4.1.3: one declaration per flag default -------------------------------

// dfltKnownLiteralSites are the call sites that still pass a bool literal to
// GetFeatureFlagWithDefault for a registered flag. The literal agrees with the
// registry today; the list is a ratchet, so a new literal fails the test.
var dfltKnownLiteralSites = map[string]bool{
	"server/changelog_handler.go|" + ChangelogFlagName:                       true,
	"server/changelog_handler.go|" + UpdateCheckFlagName:                     true,
	"server/services/guards_flag.go|" + hiddenSessionReadonlyGuardsFlagName:  true,
	"server/services/hidden_question_reply.go|" + hiddenSessionReplyFlagName: true,
	"server/services/lease_flag.go|" + terminalWriteLeaseFlagName:            true,
	"github/client.go|" + githubGraphQLMigrationFlagName:                     true,
	"github/http_client.go|" + githubPriorityAdmissionFlagName:               true,
	"session/git_worktree_manager.go|" + worktreeChangeDetectionFlagName:     true,
}

// dfltKnownPlainReadDisagreements are plain GetFeatureFlag reads of a flag whose
// registry default is true, so an unset flag reads false there and true elsewhere.
// Changing the read is a behavior change and needs an owner decision.
var dfltKnownPlainReadDisagreements = map[string]bool{
	"session/instance_tmux.go|" + terminalResyncExecGateFastLaneFlagName: true,
}

type dfltFile struct {
	path string
	src  *ast.File
	fset *token.FileSet
}

func dfltParseRepoSources(t *testing.T) []dfltFile {
	t.Helper()
	root := filepath.Join("..", "..")
	skip := map[string]bool{"node_modules": true, "web-app": true, "gen": true, ".git": true, "ent": true, "tests": true, "docs": true, "project_plans": true, ".claude": true, "tools": true}
	var files []dfltFile
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] && path != root {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		files = append(files, dfltFile{path: filepath.ToSlash(rel), src: f, fset: fset})
		return nil
	}))
	return files
}

// dfltStringConsts maps every top-level string constant name to its value.
func dfltStringConsts(files []dfltFile) map[string]string {
	consts := map[string]string{}
	for _, f := range files {
		for _, decl := range f.src.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if s, err := strconv.Unquote(lit.Value); err == nil {
							consts[name.Name] = s
						}
					}
				}
			}
		}
	}
	return consts
}

func dfltResolveFlag(e ast.Expr, consts map[string]string) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		s, err := strconv.Unquote(v.Value)
		return s, err == nil
	case *ast.Ident:
		s, ok := consts[v.Name]
		return s, ok
	case *ast.SelectorExpr:
		s, ok := consts[v.Sel.Name]
		return s, ok
	}
	return "", false
}

func dfltRegistered(name string) (defaultValue, ok bool) {
	for _, kf := range knownFeatureFlags {
		if kf.name == name {
			return kf.defaultValue, true
		}
	}
	return false, false
}

// dfltDefaultViolations reports, for registered flags only: a bool literal that
// disagrees with the registry, a bool literal not in dfltKnownLiteralSites (a second
// declaration), and a plain GetFeatureFlag read of a default-on flag.
func dfltDefaultViolations(files []dfltFile, consts map[string]string) []string {
	var out []string
	for _, f := range files {
		ast.Inspect(f.src, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "GetFeatureFlag" && sel.Sel.Name != "GetFeatureFlagWithDefault") {
				return true
			}
			name, resolved := dfltResolveFlag(call.Args[0], consts)
			if !resolved {
				return true
			}
			registry, registered := dfltRegistered(name)
			if !registered {
				return true
			}
			pos := f.fset.Position(call.Pos()).String()
			key := f.path + "|" + name
			if sel.Sel.Name == "GetFeatureFlag" {
				if registry && !dfltKnownPlainReadDisagreements[key] {
					out = append(out, pos+": plain GetFeatureFlag reads "+name+" as off when unset, but its registry default is on")
				}
				return true
			}
			if len(call.Args) != 2 {
				return true
			}
			lit, ok := call.Args[1].(*ast.Ident)
			if !ok || (lit.Name != "true" && lit.Name != "false") {
				return true
			}
			switch {
			case (lit.Name == "true") != registry:
				out = append(out, pos+": literal "+lit.Name+" disagrees with the registry default of "+name)
			case !dfltKnownLiteralSites[key]:
				out = append(out, pos+": second declaration of the default of "+name+"; use featureFlagDefault or a config constant")
			}
			return true
		})
	}
	return out
}

func TestFlagDefaults_ConfigConstantsEqualRegistryAndReaders(t *testing.T) {
	withIsolatedStateDir(t)
	cfg := config.LoadConfig()
	rows := []struct {
		flag     string
		constant bool
		reader   func(*config.Config) bool
	}{
		{config.TriageGuidanceHaltFeatureFlag, config.TriageGuidanceHaltDefault, config.EffectiveTriageGuidanceHaltEnabled},
		{config.DiagnoseNudgeFeatureFlag, config.DiagnoseNudgeDefault, config.EffectiveDiagnoseNudgeEnabled},
		{config.AutonomousStallNudgeFeatureFlag, config.AutonomousStallNudgeDefault, config.EffectiveAutonomousStallNudgeEnabled},
		{config.HiddenSessionGateFeatureFlag, config.HiddenSessionGateDefault, nil},
		{config.WorkflowEngineShadowFeatureFlag, config.WorkflowEngineShadowDefault, config.EffectiveWorkflowEngineShadow},
		{config.WorkflowEngineEnforceFeatureFlag, config.WorkflowEngineEnforceDefault, config.EffectiveWorkflowEngineEnforce},
		{config.WorkflowEngineEnforceAgentFeatureFlag, config.WorkflowEngineEnforceAgentDefault, config.EffectiveWorkflowEngineEnforceAgent},
		{config.WorkflowEngineEnforceSystemFeatureFlag, config.WorkflowEngineEnforceSystemDefault, config.EffectiveWorkflowEngineEnforceSystem},
	}
	for _, r := range rows {
		registry, ok := dfltRegistered(r.flag)
		require.True(t, ok, "%s must be registered", r.flag)
		require.Equal(t, r.constant, registry, "registry default of %s differs from its config constant", r.flag)
		if r.reader != nil {
			require.Equal(t, r.constant, r.reader(cfg), "reader of %s differs from its config constant", r.flag)
		}
	}
	require.False(t, sddDefaultPipelineEnabled(), "sdd-default-pipeline stays off when unset")
	require.NoError(t, config.LoadConfig().SetFeatureFlag(sddDefaultPipelineFlagName, true))
	require.True(t, sddDefaultPipelineEnabled())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(sddDefaultPipelineFlagName, false))
	require.False(t, sddDefaultPipelineEnabled(), "an explicit false still opts out")
}

func TestFlagDefaults_NoSecondDeclarationOrDisagreeingRead(t *testing.T) {
	t.Parallel()
	synth := func(src string) []dfltFile {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x.go", src, parser.SkipObjectResolution)
		require.NoError(t, err)
		return []dfltFile{{path: "x.go", src: f, fset: fset}}
	}
	// Scanner self-test: a disagreeing literal, a new agreeing literal and a plain
	// read of a default-on flag all fail.
	bad := synth(`package x
const a = "programs:cli-flag-probe"
const b = "terminal:resync-exec-gate-fast-lane"
func f(c *C) {
	c.GetFeatureFlagWithDefault(a, false)
	c.GetFeatureFlagWithDefault(a, true)
	c.GetFeatureFlag(b)
}`)
	require.Len(t, dfltDefaultViolations(bad, dfltStringConsts(bad)), 3)
	good := synth(`package x
const a = "programs:cli-flag-probe"
func f(c *C) { c.GetFeatureFlagWithDefault(a, featureFlagDefault(a)) }`)
	require.Empty(t, dfltDefaultViolations(good, dfltStringConsts(good)))

	files := dfltParseRepoSources(t)
	require.Empty(t, dfltDefaultViolations(files, dfltStringConsts(files)))
}
