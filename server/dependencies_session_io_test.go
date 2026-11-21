package server

import (
	"context"
	"go/ast"
	"go/types"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSessionIO_should_ReturnDistinctWorkingHelpers_When_Built(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	io := buildSessionIO(3)

	require.NotNil(t, io.historyLinker)
	require.NotNil(t, io.scrollbackManager)
	require.NotNil(t, io.tmuxStreamerManager)
	t.Cleanup(func() { _ = io.scrollbackManager.Close() })
	require.NoError(t, io.scrollbackManager.AppendOutput("s1", []byte("hello\n")))
	got, err := io.scrollbackManager.GetRecentBytes("s1", 64)
	require.NoError(t, err)
	assert.Contains(t, string(got), "hello")
}

func TestBuildRuntimeDeps_should_ExposeSessionIOHelpers_When_Built(t *testing.T) {
	rt, logs := buildNarrowRuntimeDeps(t)

	require.NotNil(t, rt.HistoryLinker, "RuntimeDeps.HistoryLinker must come from buildSessionIO")
	require.NotNil(t, rt.ScrollbackManager, "RuntimeDeps.ScrollbackManager must come from buildSessionIO")
	require.NotNil(t, rt.TmuxStreamerManager, "RuntimeDeps.TmuxStreamerManager must come from buildSessionIO")
	assert.Contains(t, logs, "HistoryLinker initialized")
	assert.Contains(t, logs, "initialized ScrollbackManager")
}

func TestBackfillStuckStates_should_CallBackfillOnlyWhen_BacklogEnabled(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{true, false} {
		var calls int
		backfillStuckStates(enabled, func(context.Context) { calls++ })
		want := 0
		if enabled {
			want = 1
		}
		assert.Equal(t, want, calls, "enabled=%v", enabled)
	}
}

func TestStartPausedTmuxReaper_should_ReapOnEveryTick_When_Started(t *testing.T) {
	t.Parallel()
	ticks := make(chan struct{}, 2)
	// After two ticks reap parks forever, so the process-lifetime goroutine stops
	// ticking instead of leaking a busy ticker into the rest of the package run.
	var n atomic.Int32
	startPausedTmuxReaper(20*time.Millisecond, func() {
		if n.Add(1) > 2 {
			select {}
		}
		ticks <- struct{}{}
	})
	for i := 0; i < 2; i++ {
		select {
		case <-ticks:
		case <-time.After(5 * time.Second):
			t.Fatalf("reaper tick %d never happened", i+1)
		}
	}
}

func TestPausedTmuxReapInterval_should_BeThirtyMinutes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 30*time.Minute, pausedTmuxReapInterval)
}

// callArgs returns the printed arguments of the single call to name in fn.
func callArgs(t *testing.T, fn *ast.FuncDecl, name string) []string {
	t.Helper()
	var out []string
	var found int
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != name {
			return true
		}
		found++
		out = nil
		for _, a := range call.Args {
			out = append(out, types.ExprString(a))
		}
		return true
	})
	require.Equal(t, 1, found, "%s must be called exactly once", name)
	return out
}

// The call-site arguments carry the behaviour (flag gate, real reaper method,
// instance count). Observing them through BuildRuntimeDeps would need a production
// seam (spy func var) or a seeded stuck state plus a 30-minute wait, so they are
// pinned at the AST level in the style of the step-order test.
func TestBuildRuntimeDeps_should_PassRealArgumentsToSessionIOSteps(t *testing.T) {
	t.Parallel()
	fn := parseFunc(t, "dependencies.go", "BuildRuntimeDeps")

	assert.Equal(t, []string{"cfg.GetFeatureFlag(\"backlog\")", "backlogLifecycleListener.BackfillStuckStates"},
		callArgs(t, fn, "backfillStuckStates"))
	assert.Equal(t, []string{"pausedTmuxReapInterval", "sessionService.ReapPausedTmuxSessions"},
		callArgs(t, fn, "startPausedTmuxReaper"))
	assert.Equal(t, []string{"len(instances)"}, callArgs(t, fn, "buildSessionIO"))
}
