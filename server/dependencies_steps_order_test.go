package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// buildRuntimeDepsStepOrder is the startup order of BuildRuntimeDeps. Several steps
// depend on earlier ones having run (the delivery gate is seeded before instances are
// wired; the token store starts before QuotaGate's first Reconcile; the boot goroutine
// is launched before external-session discovery is built). A new step is a new row
// in the position it must run, not an append.
var buildRuntimeDepsStepOrder = []string{
	"wireTCCEnrichment",
	"LoadInstances",
	"buildBacklogEngines",
	"newWorkflowEngineWiring",
	"buildHeadlessLLM",
	"buildTagPoller",
	"buildSummaryGenerators",
	"newBacklogLifecycleListener",
	"wireLoadedInstances",
	"startInstanceBootInit",
	"buildReactiveQueue",
	"NewHistoryLinkerFromRealInspector",
	"NewScrollbackManager",
	"wireExternalSessions",
	"wireSessionServiceRuntime",
	"buildUnfinishedWork",
	"buildGitHubUserService",
	"openAnalyticsStores",
	"startDiagnosticsRetention",
	"startTokenStores",
	"startBacklogControl",
	"startHousekeeping",
	"buildBacklogService",
	"wireBacklogLifecycle",
	"checkBrowserDeps",
	"buildInsights",
	"buildWorkflows",
	"buildJules",
	"TransitionGateWired",
}

func TestBuildRuntimeDeps_should_RunStepsInPinnedStartupOrder(t *testing.T) {
	t.Parallel()
	fn := parseFunc(t, "dependencies.go", "BuildRuntimeDeps")
	prev := fn.Body.Pos()
	prevName := "function start"
	for _, step := range buildRuntimeDepsStepOrder {
		pos := callPositions(fn, step)
		require.Len(t, pos, 1, "%s must be called exactly once", step)
		require.Greater(t, int(pos[0]), int(prev), "%s must run after %s", step, prevName)
		prev, prevName = pos[0], step
	}
}

// bootInitStepOrder is the order of the background boot goroutine (steps 6 to 7.6).
var bootInitStepOrder = []string{
	"bootStartInstances",
	"bootHotRestoreLive",
	"ReconcileShells",
	"ReconcileOrphanedTmuxSessions",
	"SaveInstances",
	"ReconcileOrphans",
	"bootStartControllers",
	"bootResumeDrivers",
	"Scan",
	"syncOrphanedApprovalsToQueue",
}

func TestStartInstanceBootInit_should_RunStepsInPinnedOrder(t *testing.T) {
	t.Parallel()
	fn := parseFunc(t, "dependencies_instances.go", "startInstanceBootInit")
	prev := fn.Body.Pos()
	prevName := "function start"
	for _, step := range bootInitStepOrder {
		pos := callPositions(fn, step)
		require.Len(t, pos, 1, "%s must be called exactly once", step)
		require.Greater(t, int(pos[0]), int(prev), "%s must run after %s", step, prevName)
		prev, prevName = pos[0], step
	}
}
