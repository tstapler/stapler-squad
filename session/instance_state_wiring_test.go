package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// Snapshot pointer identity stands in for a counting build hook: each rebuild stores a new pointer.
func TestUpdateTimestamps_Should_RebuildSnapshotOncePerMeaningfulFrameAndZeroOtherwise_When_CountingHook(t *testing.T) {
	t.Parallel()
	inst, err := instance.NewInstance(instance.InstanceOptions{Title: "touch-frames", Path: "/tmp/test", Program: "claude"})
	require.NoError(t, err)
	inst.SetTmuxSession(tmux.NewTmuxSession("touch-frames", "claude"))

	s0 := inst.Snapshot()
	inst.UpdateTerminalTimestamps("   \n", false)
	assert.Same(t, s0, inst.Snapshot(), "blank frame must not rebuild")
	assert.Equal(t, s0.LastMeaningfulChangeAt, inst.Snapshot().LastMeaningfulChangeAt)

	inst.UpdateTerminalTimestamps("Hello, world!\nError: something happened", false)
	s1 := inst.Snapshot()
	assert.NotSame(t, s0, s1, "meaningful frame rebuilds")
	assert.True(t, s1.LastMeaningfulChangeAt.Equal(s1.LastMeaningfulOutput))
	assert.True(t, s1.LastMeaningfulChangeAt.After(s0.LastMeaningfulChangeAt))

	inst.UpdateTerminalTimestamps("Hello, world!\nError: something happened", false)
	assert.Equal(t, s1.LastMeaningfulChangeAt, inst.Snapshot().LastMeaningfulChangeAt, "same signature does not touch")
}
