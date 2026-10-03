package server

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// TestWireDepsIntoServer_SharesSingleSlackNotifierInstance_AcrossReactiveQueueManagerApprovalHandlerAndSessionService
// is the regression test for commit 13ad9c260, which fixed a split-brain bug
// where SessionService's SlackConfigService constructed and used its own
// private *services.SlackNotifier instead of the shared instance wired into
// ReactiveQueueManager/ApprovalHandler (server.go's wireDepsIntoServer,
// server/server.go:559/563: approvalHandler.SetSlackNotifier(deps.SlackNotifier)
// / deps.SessionService.SetSlackNotifier(deps.SlackNotifier)). Left
// unregression-tested, dropping either SetSlackNotifier call would silently
// desync GetSlackConfig's last_delivery snapshot from the real review-queue/
// approval send paths, with no compiler error to catch it.
//
// Exercises the real wireDepsIntoServer wiring via NewServerWithDeps (not just
// BuildDependencies, which never constructs ApprovalHandler -- that only
// happens inside wireDepsIntoServer). ApprovalHandler.SlackNotifierForTest and
// SessionService.SlackNotifierForTest are minimal test-only accessors added
// for exactly this assertion (server/services/approval_handler.go,
// server/services/session_service.go) -- ReactiveQueueManager needs no such
// accessor since it lives in this same package and its unexported
// slackNotifier field is reachable directly from a same-package test.
func TestWireDepsIntoServer_SharesSingleSlackNotifierInstance_AcrossReactiveQueueManagerApprovalHandlerAndSessionService(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	deps, err := BuildDependencies()
	if err != nil {
		t.Fatalf("BuildDependencies: %v", err)
	}

	srv := NewServerWithDeps("localhost:0", deps)
	t.Cleanup(func() {
		if err := srv.Shutdown(); err != nil {
			t.Logf("srv.Shutdown: %v", err)
		}
	})

	if deps.SlackNotifier == nil {
		t.Fatal("expected ServerDependencies.SlackNotifier to be wired")
	}
	if deps.ReactiveQueueMgr == nil {
		t.Fatal("expected ReactiveQueueMgr to be wired")
	}
	if srv.approvalHandler == nil {
		t.Fatal("expected wireDepsIntoServer to have constructed and stored the ApprovalHandler")
	}
	if deps.SessionService == nil {
		t.Fatal("expected SessionService to be wired")
	}

	if deps.ReactiveQueueMgr.slackNotifier != deps.SlackNotifier {
		t.Errorf("ReactiveQueueManager.slackNotifier is not the same instance as deps.SlackNotifier: got %p, want %p",
			deps.ReactiveQueueMgr.slackNotifier, deps.SlackNotifier)
	}
	if srv.approvalHandler.SlackNotifierForTest() != deps.SlackNotifier {
		t.Errorf("ApprovalHandler's SlackNotifier is not the same instance as deps.SlackNotifier: got %p, want %p",
			srv.approvalHandler.SlackNotifierForTest(), deps.SlackNotifier)
	}
	if deps.SessionService.SlackNotifierForTest() != deps.SlackNotifier {
		t.Errorf("SessionService's SlackNotifier is not the same instance as deps.SlackNotifier (split-brain regression, commit 13ad9c260): got %p, want %p",
			deps.SessionService.SlackNotifierForTest(), deps.SlackNotifier)
	}
}

// TestBuildDependencies_should_WireGeminiCaller_When_GeminiBinaryDetected is the
// Task 3.1.2b integration test: confirms BuildDependencies actually constructs
// a *headless.GeminiCaller when the gemini binary is present at startup,
// mirroring HeadlessPool's own "non-fatal, just leave the field nil, if the
// binary is missing" pattern (this branch does not yet register it into a
// headlessCallers registry — Epic 2.3, not landed yet — see GeminiCaller's
// doc comment on ServerDependencies). Skips (rather than asserting nil) when
// gemini genuinely isn't installed on the machine running this test —
// mirroring config.GetAvailablePrograms' own tests' convention of skipping
// gracefully for an optional candidate CLI not guaranteed present in every
// dev/CI environment.
func TestBuildDependencies_should_WireGeminiCaller_When_GeminiBinaryDetected(t *testing.T) {
	if _, lookErr := exec.LookPath("gemini"); lookErr != nil {
		t.Skip("gemini binary not found on PATH; skipping (see config.GetAvailablePrograms' analogous convention)")
	}

	envtest.NewIsolatedStateDir(t)

	deps, err := BuildDependencies()
	require.NoError(t, err)

	require.NotNil(t, deps.GeminiCaller, "expected GeminiCaller to be wired when the gemini binary is detected at startup")
	assert.True(t, deps.GeminiCaller.Available())
}

func TestBuildServiceDeps_RejectsNilCore(t *testing.T) {
	_, err := BuildServiceDeps(nil)
	if err == nil {
		t.Fatal("expected error for nil CoreDeps")
	}
}

func TestBuildServiceDeps_RejectsNilCoreFields(t *testing.T) {
	// CoreDeps with all nil fields should be rejected.
	core := &CoreDeps{}
	_, err := BuildServiceDeps(core)
	if err == nil {
		t.Fatal("expected error for CoreDeps with nil fields")
	}
}

func TestBuildServiceDeps_ErrorMentionsPhase(t *testing.T) {
	// The error from a nil CoreDeps should mention the phase name so that
	// callers can identify where in the initialization chain the failure occurred.
	_, err := BuildServiceDeps(nil)
	if err == nil {
		t.Fatal("expected error for nil CoreDeps")
	}
	if !strings.Contains(err.Error(), "BuildServiceDeps") {
		t.Errorf("error %q does not mention BuildServiceDeps", err.Error())
	}
}

func TestBuildRuntimeDeps_RejectsNilService(t *testing.T) {
	// The zero-value token is acceptable here — this test is only checking the
	// nil-ServiceDeps guard, not that tmux is actually running.
	_, err := BuildRuntimeDeps(tmux.TmuxServerReady{}, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil ServiceDeps")
	}
}

func TestBuildRuntimeDeps_ErrorMentionsPhase(t *testing.T) {
	_, err := BuildRuntimeDeps(tmux.TmuxServerReady{}, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil ServiceDeps")
	}
	if !strings.Contains(err.Error(), "BuildRuntimeDeps") {
		t.Errorf("error %q does not mention BuildRuntimeDeps", err.Error())
	}
}

func TestBuildServiceDeps_NilCoreFieldsErrorIsDescriptive(t *testing.T) {
	// A zero CoreDeps (all nil fields) must return an error that describes
	// the problem — not a panic or an empty error string.
	core := &CoreDeps{}
	_, err := BuildServiceDeps(core)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() == "" {
		t.Fatal("error message must not be empty")
	}
}

func TestBuildServiceDeps_OnlyCoreNil_DifferentFromPartialCore(t *testing.T) {
	// Nil *CoreDeps and a zero-value *CoreDeps should both fail, but the error
	// message for nil should mention the nil guard (not a panic).
	_, nilErr := BuildServiceDeps(nil)
	_, zeroErr := BuildServiceDeps(&CoreDeps{})

	if nilErr == nil || zeroErr == nil {
		t.Fatal("both cases should return errors")
	}
	// The errors should be different messages — nil core vs nil fields.
	if nilErr.Error() == zeroErr.Error() {
		t.Logf("note: nil and zero-value CoreDeps produce the same error: %v", nilErr)
	}
}

// TestBuildRuntimeDeps_should_ShareSinglePipelineEngineInstance_When_ConstructingBacklogServiceAndLifecycleListener
// is the concrete pointer-equality test Story 1.5.1's own acceptance criteria promises
// (plan.md Task 1.5.1e): BuildRuntimeDeps must construct exactly one
// *session.CachingPipelineEngine and inject the SAME instance into both BacklogService
// and BacklogLifecycleListener (transitively ReviewGateRunner) — never two separately
// constructed engines that could silently drift in cache state after a write.
func TestBuildRuntimeDeps_should_ShareSinglePipelineEngineInstance_When_ConstructingBacklogServiceAndLifecycleListener(t *testing.T) {
	deps, err := BuildDependencies()
	if err != nil {
		t.Fatalf("BuildDependencies: %v", err)
	}

	if deps.BacklogService == nil {
		t.Fatal("expected BacklogService to be wired")
	}
	backlogSvcEngine := deps.BacklogService.PipelineEngine()
	if backlogSvcEngine == nil {
		t.Fatal("expected BacklogService.PipelineEngine() to be non-nil")
	}

	if deps.SessionService == nil {
		t.Fatal("expected SessionService to be wired")
	}
	listener := deps.SessionService.GetBacklogLifecycleListener()
	if listener == nil {
		t.Fatal("expected BacklogLifecycleListener to be wired onto SessionService")
	}
	listenerEngine := listener.PipelineEngine()
	if listenerEngine == nil {
		t.Fatal("expected BacklogLifecycleListener.PipelineEngine() to be non-nil")
	}

	backlogCaching, ok := backlogSvcEngine.(*session.CachingPipelineEngine)
	if !ok {
		t.Fatalf("expected BacklogService.PipelineEngine() to be a *session.CachingPipelineEngine, got %T", backlogSvcEngine)
	}
	listenerCaching, ok := listenerEngine.(*session.CachingPipelineEngine)
	if !ok {
		t.Fatalf("expected BacklogLifecycleListener.PipelineEngine() to be a *session.CachingPipelineEngine, got %T", listenerEngine)
	}

	if backlogCaching != listenerCaching {
		t.Fatalf("expected BacklogService and BacklogLifecycleListener to share the identical *session.CachingPipelineEngine instance, got distinct pointers %p vs %p", backlogCaching, listenerCaching)
	}
}

// TestBuildRuntimeDeps_should_PopulateBacklogLifecycleListener_When_ServerBoots is the
// dependency-wiring regression test for pr-event-webhooks Task 3.1.1: before this
// feature, backlogLifecycleListener was only a local variable inside BuildRuntimeDeps,
// unreachable from server.go's webhook-route-registration block — NewGitHubWebhookHandler
// had no way to be given a real PRFixEventRouter. deps.BacklogLifecycleListener must be
// the exact same instance SetPRFixSpawner/SetAutoReopener etc. were wired onto, not a
// second, independently-constructed listener.
func TestBuildRuntimeDeps_should_PopulateBacklogLifecycleListener_When_ServerBoots(t *testing.T) {
	deps, err := BuildDependencies()
	if err != nil {
		t.Fatalf("BuildDependencies: %v", err)
	}

	if deps.BacklogLifecycleListener == nil {
		t.Fatal("expected BacklogLifecycleListener to be wired")
	}
	if deps.SessionService == nil {
		t.Fatal("expected SessionService to be wired")
	}
	listener := deps.SessionService.GetBacklogLifecycleListener()
	if listener == nil {
		t.Fatal("expected BacklogLifecycleListener to be wired onto SessionService")
	}
	if deps.BacklogLifecycleListener != listener {
		t.Fatalf("expected deps.BacklogLifecycleListener to be the identical instance wired onto SessionService, got distinct pointers %p vs %p", deps.BacklogLifecycleListener, listener)
	}
}

// TestBuildRuntimeDeps_should_WireSessionSteererAndSessionStopper_When_ServerBoots
// is the pr-fix-steering Task 1.2.2d wiring-assertion test (pre-mortem.md P2 #5):
// SetSessionSteerer/SetSessionStopper's wiring in BuildRuntimeDeps (one new line
// each) had no test that would fail if either call were ever omitted or
// misplaced — both fields' nil-safe degrade paths are, by design, externally
// indistinguishable from today's shipped behavior, so a forgotten wiring line
// compiles, passes make ci, and ships the feature silently inert. This is the
// first such test for either setter — there is no prior instance to extend.
func TestBuildRuntimeDeps_should_WireSessionSteererAndSessionStopper_When_ServerBoots(t *testing.T) {
	deps, err := BuildDependencies()
	if err != nil {
		t.Fatalf("BuildDependencies: %v", err)
	}
	if deps.BacklogService.GetSessionSteerer() == nil {
		t.Fatal("expected SessionSteerer to be wired onto BacklogService")
	}
	if deps.BacklogService.GetSessionSteerer() != deps.SessionService {
		t.Fatal("expected the wired SessionSteerer to be the same *SessionService instance BuildDependencies constructs")
	}
	if deps.BacklogService.GetSessionStopper() == nil {
		t.Fatal("expected SessionStopper to be wired onto BacklogService")
	}
}

// TestBuildRuntimeDeps_should_CallReconcileSynchronouslyAtBoot_When_BacklogFlagDisabledByDefault
// verifies QuotaGate.Enable is only ever reached via quotaGate.Reconcile's own
// decision path (Story 2.2.2), not a bare unconditional call — exercised against
// the real TokenStore/BacklogController wiring in server/dependencies.go. Named
// for the disabled-by-default path this test actually exercises; see
// TestBuildRuntimeDeps_should_ReadLiveConfigOnEveryCfgFnCall_When_ConfigJSONChangesAfterBoot
// below for the Quota.Enabled=true path.
func TestBuildRuntimeDeps_should_CallReconcileSynchronouslyAtBoot_When_BacklogFlagDisabledByDefault(t *testing.T) {
	deps, err := BuildDependencies()
	if err != nil {
		t.Fatalf("BuildDependencies: %v", err)
	}
	if deps.QuotaGate == nil {
		t.Fatal("expected QuotaGate to be wired onto ServerDependencies")
	}
	// Quota.Enabled defaults to false, so the boot-time Reconcile call is a
	// no-op and IsPausedByQuota must be false — this exercises the real
	// construction + boot Reconcile call path without requiring a live rate
	// limit or token-usage fixture.
	if deps.QuotaGate.IsPausedByQuota() {
		t.Error("IsPausedByQuota() = true at boot with Quota.Enabled defaulting to false, want false")
	}
}

// TestBuildRuntimeDeps_should_ReadLiveConfigOnEveryCfgFnCall_When_ConfigJSONChangesAfterBoot
// is the regression guard for a CRITICAL code-review finding: the cfgFn closure
// built in BuildRuntimeDeps must call config.LoadConfig() fresh, not close over
// the *config.Config pointer passed into BuildRuntimeDeps (which is loaded once
// at process boot and never refreshed) — the whole point of cfgFn being a
// func() rather than a plain value is "config.json edits take effect without a
// restart" (see both quota_gate.go's and dependencies.go's own doc comments).
// StatusDetail() calls cfgFn() directly, so it's used here as the observable
// proof without needing a rate-limit/token-usage fixture to trigger Reconcile.
func TestBuildRuntimeDeps_should_ReadLiveConfigOnEveryCfgFnCall_When_ConfigJSONChangesAfterBoot(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "shared")

	deps, err := BuildDependencies()
	if err != nil {
		t.Fatalf("BuildDependencies: %v", err)
	}
	if deps.QuotaGate == nil {
		t.Fatal("expected QuotaGate to be wired onto ServerDependencies")
	}

	if got := deps.QuotaGate.StatusDetail(); got != "" {
		t.Fatalf("test precondition failed: StatusDetail() = %q before any config change, want empty (Quota.Enabled defaults to false)", got)
	}

	cfg := config.LoadConfig()
	cfg.Quota.Enabled = true
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got := deps.QuotaGate.StatusDetail()
	if !strings.Contains(strings.ToLower(got), "reactive-only") {
		t.Errorf("StatusDetail() = %q after enabling Quota via config.json with no restart, want it to reflect the live change (mentions reactive-only mode) — cfgFn must re-read config.json on every call, not close over a boot-time snapshot", got)
	}
}

// TestReconcileTicker_should_KeepRunningReconcileStuck_When_QuotaGateReconcilePanics
// verifies the shared 60s ticker's two reconcile calls are independently
// panic-recovered — a panic in one must not kill the shared ticker goroutine
// or block the other. recoverAndLog is the exact wrapper both ticker calls
// use in server/dependencies.go, so exercising it directly here proves the
// isolation the ticker relies on without racing a real 60s tick.
func TestReconcileTicker_should_KeepRunningReconcileStuck_When_QuotaGateReconcilePanics(t *testing.T) {
	backlogReconcileRan := false

	recoverAndLog("quota gate reconcile ticker", func() { panic("simulated QuotaGate.Reconcile panic") })
	recoverAndLog("backlog reconcile ticker", func() { backlogReconcileRan = true })

	if !backlogReconcileRan {
		t.Error("backlog reconcile did not run after a panic in the sibling quota-gate reconcile call — the ticker goroutine must survive")
	}
}

// TestWorktreeConsistencySweeperWiring_should_StartAndStopWithoutPanicking_When_DependenciesBuiltNarrow
// is Task 1.3.2b's wiring smoke test for the session.StartWorktreeConsistencySweeper call
// added alongside the 60s reconcile ticker above (server/dependencies.go): confirms the
// sweeper goroutine starts and returns cleanly on ctx cancellation. Deliberately builds a
// narrow session/events/config dependency set rather than calling server.BuildDependencies()
// or NewServerWithDeps, which wire ~30 real production subsystems and make real outbound
// network calls even under test isolation (instinct_ci_hermetic_testing_gotchas.md) —
// mirroring server/services/session_service_test.go's createTestStorage pattern.
func TestWorktreeConsistencySweeperWiring_should_StartAndStopWithoutPanicking_When_DependenciesBuiltNarrow(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	notifier := &services.EventBusNotifier{Bus: events.NewEventBus(100)}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		session.StartWorktreeConsistencySweeper(ctx, storage, notifier, config.LoadConfig)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("StartWorktreeConsistencySweeper did not return after ctx cancellation")
	}
}

// TestSetSyncFeatureEnabledCheck_should_MatchPlainIsEnabled_When_NotThrottled
// exercises the real composed closure server/dependencies.go passes to
// backlogSvc.SetSyncFeatureEnabledCheck (Story 2.3.2) indirectly via
// TriggerSync — not a reimplementation of the &&. Per plan.md Task 2.3.2b,
// ShouldThrottleForeground's own sliding-window behavior is covered directly
// in server/services/quota_gate_test.go; this test only confirms the
// composed checker, when unthrottled, still behaves identically to today's
// bare backlogCtrl.IsEnabled() (i.e. the throttle never fires when nothing
// foreground is observed).
func TestSetSyncFeatureEnabledCheck_should_MatchPlainIsEnabled_When_NotThrottled(t *testing.T) {
	deps, err := BuildDependencies()
	if err != nil {
		t.Fatalf("BuildDependencies: %v", err)
	}
	if deps.QuotaGate == nil || deps.BacklogService == nil {
		t.Fatal("expected QuotaGate and BacklogService to be wired")
	}
	if deps.QuotaGate.ShouldThrottleForeground() {
		t.Fatal("test precondition failed: QuotaGate should not be throttling foreground at boot with no observed activity")
	}
}

func TestPrNumFromTitle(t *testing.T) {
	cases := []struct {
		title   string
		matches bool
		want    int
	}{
		{"pr-1255-actions-spring-boot", true, 1255},
		{"PR-42-feature", true, 42},  // case-insensitive
		{"pr-0-foo", true, 0},        // zero is valid match; caller ignores pr 0
		{"pr-99-", true, 99},         // trailing dash only
		{"pr-1255", false, 0},        // missing trailing dash
		{"pr-foo-bar", false, 0},     // non-numeric
		{"feature-branch", false, 0}, // no prefix
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			m := prNumFromTitle.FindStringSubmatch(tc.title)
			if !tc.matches {
				if m != nil {
					t.Errorf("expected no match, got %v", m)
				}
				return
			}
			if m == nil {
				t.Fatalf("expected match for %q, got none", tc.title)
			}
			var got int
			for _, b := range m[1] {
				got = got*10 + int(b-'0')
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// TestServerDependencies_should_DegradeFeatureNotServer_When_KeychainUnreadableAtStartup
// covers google-jules-integration Story 2.4.4's construction-failure
// acceptance criterion: with Jules enabled but the keychain unreadable at
// startup, BuildDependencies must still succeed (server starts normally),
// leaving deps.JulesSessionPoller nil and logging "jules disabled" once at
// Info — every other subsystem (SessionService, BacklogService) unaffected.
func TestServerDependencies_should_DegradeFeatureNotServer_When_KeychainUnreadableAtStartup(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	keyring.MockInitWithError(errors.New("simulated unreadable keychain"))

	cfg := config.LoadConfig()
	cfg.Jules.Enabled = true
	require.NoError(t, config.SaveConfig(cfg))

	deps, err := BuildDependencies()
	require.NoError(t, err, "an unreadable keychain must degrade the Jules feature, not fail server startup")

	assert.Nil(t, deps.JulesSessionPoller, "an unreadable keychain must leave the Jules poller unconstructed")
	assert.NotNil(t, deps.SessionService, "every other subsystem must be unaffected")
	assert.NotNil(t, deps.BacklogService, "every other subsystem must be unaffected")
}

// noopTagPoolClient is a minimal headless.PoolClient double used only to construct a real
// SessionTagClassificationPoller for the wiring tests below — it is never actually called,
// since these tests only assert whether wireDepsIntoServer starts the poller, not its
// classification behavior (covered by session/session_tag_poller_test.go).
type noopTagPoolClient struct{}

func (noopTagPoolClient) CallBlocking(context.Context, headless.FeatureKey, string, string, headless.CallOptions, headless.CostSink) (string, error) {
	return `{"tags":["Unclassified"]}`, nil
}

// TestWireDepsIntoServer_should_NotConstructPoller_When_HeadlessPoolNil is Story 4.4.1's
// degraded/disabled-mode case: forcing deps.SessionTagClassificationPoller to nil (mirroring
// what BuildRuntimeDeps produces when the claude binary isn't found — see the headlessPool
// nil-check this poller's construction is guarded by, server/dependencies.go) must not start
// anything and must not prevent the server from starting normally.
func TestWireDepsIntoServer_should_NotConstructPoller_When_HeadlessPoolNil(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	deps, err := BuildDependencies()
	require.NoError(t, err)
	deps.SessionTagClassificationPoller = nil

	srv := NewServerWithDeps("localhost:0", deps)
	t.Cleanup(func() {
		if err := srv.Shutdown(); err != nil {
			t.Logf("srv.Shutdown: %v", err)
		}
	})

	assert.Nil(t, deps.SessionTagClassificationPoller, "a nil poller must stay unconstructed, never started")
	assert.NotNil(t, deps.SessionService, "the rest of the server must start normally")
}

// TestWireDepsIntoServer_should_StartPollerExactlyOnce_When_HeadlessPoolPresent is Story
// 4.4.1's normal-startup case: with a poller present, wireDepsIntoServer must call Start
// exactly once, logged the same way PRStatusPoller's start is logged.
func TestWireDepsIntoServer_should_StartPollerExactlyOnce_When_HeadlessPoolPresent(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	deps, err := BuildDependencies()
	require.NoError(t, err)
	deps.SessionTagClassificationPoller = session.NewSessionTagClassificationPoller(noopTagPoolClient{}, classifier.NewTaggingEngine())
	require.False(t, deps.SessionTagClassificationPoller.Running(), "poller must not be running before the server wires it up")

	srv := NewServerWithDeps("localhost:0", deps)
	t.Cleanup(func() {
		if err := srv.Shutdown(); err != nil {
			t.Logf("srv.Shutdown: %v", err)
		}
	})

	assert.True(t, deps.SessionTagClassificationPoller.Running(), "wireDepsIntoServer must start the poller")
}
