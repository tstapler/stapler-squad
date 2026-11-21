package server

import (
	"context"
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
	ticks := make(chan struct{}, 16)
	startPausedTmuxReaper(20*time.Millisecond, func() {
		select {
		case ticks <- struct{}{}:
		default:
		}
	})
	var n atomic.Int32
	deadline := time.After(5 * time.Second)
	for n.Load() < 2 {
		select {
		case <-ticks:
			n.Add(1)
		case <-deadline:
			t.Fatal("reaper did not tick twice")
		}
	}
}
