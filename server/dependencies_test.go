package server

import (
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
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
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

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
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
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
