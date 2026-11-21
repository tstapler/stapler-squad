package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// The boot-init goroutine reads config.json from the ambient state dir after a settling
// delay, so one that outlives its owner writes a default config.json into whichever test's
// dir is current ("TempDir RemoveAll cleanup: directory not empty"). StopBootInit must
// leave none running.
func TestBuildRuntimeDeps_ShouldLeaveNoBootInitGoroutine_WhenStopBootInitReturns(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	core, err := BuildCoreDepsWithOptions(BuildOptions{EntClient: session.NewTestEntRepository(t).GetEntClient()})
	require.NoError(t, err)
	svc, err := BuildServiceDeps(core)
	require.NoError(t, err)
	rt, err := BuildRuntimeDeps(tmux.TmuxServerReady{}, svc, config.LoadConfig())
	require.NoError(t, err)
	require.NotNil(t, rt.StopBootInit)
	t.Cleanup(rt.StopBootInit)

	rt.StopBootInit()

	for _, stack := range goroutineStacks() {
		require.NotContains(t, stack, "startInstanceBootInit", "boot-init goroutine outlived StopBootInit:\n%s", strings.TrimSpace(stack))
	}
}
