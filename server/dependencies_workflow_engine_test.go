package server

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	logpkg "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// buildNarrowRuntimeDeps runs the real Core -> Service -> Runtime phases (never
// BuildDependencies(), which makes outbound network calls) on isolated state and
// returns the log output produced while doing so.
func buildNarrowRuntimeDeps(t *testing.T) (*RuntimeDeps, string) {
	t.Helper()
	envtest.NewIsolatedStateDir(t)

	var buf bytes.Buffer
	prev := logpkg.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { logpkg.SetSlogDefaultForTest(prev) })

	core, err := BuildCoreDeps()
	require.NoError(t, err)
	svc, err := BuildServiceDeps(core)
	require.NoError(t, err)
	rt, err := BuildRuntimeDeps(tmux.TmuxServerReady{}, svc, config.LoadConfig())
	require.NoError(t, err)
	if gate := svc.SessionService.DeliveryGate(); gate != nil {
		t.Cleanup(gate.Stop)
	}
	return rt, buf.String()
}

func TestBuildServerDependencies_WiresSelectorIntoBacklogServiceAndSyncLoop_WhenNarrowDeps(t *testing.T) {
	rt, _ := buildNarrowRuntimeDeps(t)

	require.NotNil(t, rt.WorkflowEngine)
	selector := rt.WorkflowEngine.Engine
	require.NotNil(t, selector)

	// BacklogService holds the selector.
	got, ok := rt.BacklogService.WorkflowEngine().(*session.SwitchableWorkflowEngine)
	require.Truef(t, ok, "BacklogService engine is %T, want *session.SwitchableWorkflowEngine", rt.BacklogService.WorkflowEngine())
	require.Same(t, selector, got)

	// The periodic SyncLoop holds the same pointer.
	require.NotNil(t, rt.BacklogController)
	require.NoError(t, rt.BacklogController.Enable(context.Background()))
	t.Cleanup(func() { _ = rt.BacklogController.Disable() })
	syncEngine, ok := rt.BacklogController.SyncLoopWorkflowEngine().(*session.SwitchableWorkflowEngine)
	require.True(t, ok, "SyncLoop engine must be the selector")
	require.Same(t, selector, syncEngine)

	// The lifecycle listener keeps the concrete configured engine, never the selector.
	listenerEngine := rt.BacklogLifecycleListener.WorkflowEngine()
	require.NotNil(t, listenerEngine)
	_, isSelector := listenerEngine.(*session.SwitchableWorkflowEngine)
	require.False(t, isSelector, "the lifecycle listener must not be handed the selector")
	_, isConfigured := listenerEngine.(*session.ConfiguredWorkflowEngine)
	require.Truef(t, isConfigured, "listener engine is %T, want *session.ConfiguredWorkflowEngine", listenerEngine)

	require.NotNil(t, rt.WorkflowEngine.Recorder)
	require.NotNil(t, rt.WorkflowEngine.Soak)
	require.NotNil(t, rt.WorkflowEngine.Soak.Ledger)
}

func TestBuildServerDependencies_LogsEngineModeStartupLine(t *testing.T) {
	_, logs := buildNarrowRuntimeDeps(t)

	require.Contains(t, logs, "workflow engine: mode=shadow configured_loaded=true invalidator_wired=true")
}

func TestBuildServerDependencies_LogsInvalidatorUnwired_WhenStageConfigEngineConstructionFails(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	var buf bytes.Buffer
	prev := logpkg.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { logpkg.SetSlogDefaultForTest(prev) })

	w := newWorkflowEngineWiring(session.NewDefaultWorkflowEngine(), nil, nil, config.LoadConfig)

	require.NotNil(t, w.Engine, "a failed stage-config construction must still yield a selector")
	require.Contains(t, buf.String(), "level=WARN")
	require.Contains(t, buf.String(), "workflow engine: mode=shadow configured_loaded=false invalidator_wired=false")
}
