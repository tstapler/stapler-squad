package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/tymux"
)

// exitedPM is a process manager whose pane has exited: the live capture fails
// fast, the way TymuxBackend does for a DEAD snapshot.
type exitedPM struct {
	ProcessManager
	screen string
}

func (p exitedPM) CapturePaneContent() (string, error) {
	return "", &tymux.PaneExitedError{Screen: p.screen}
}

func TestInstance_Preview_ShouldStillShowTheFinalGrid_WhenThePaneHasExited(t *testing.T) {
	inst := &Instance{Title: "exited-preview", Status: Running}
	inst.started.Store(true)
	inst.processManager = exitedPM{screen: "last output"}

	content, err := inst.Preview()

	require.NoError(t, err)
	assert.Equal(t, "last output", content)
}

func TestInstance_CapturePaneContent_ShouldFailWithPaneExited_ButFinalVariantReturnsTheGrid(t *testing.T) {
	inst := &Instance{Title: "exited-capture", Status: Running}
	inst.started.Store(true)
	inst.processManager = exitedPM{screen: "last output"}

	_, err := inst.CapturePaneContent()
	require.ErrorIs(t, err, tymux.ErrPaneExited)

	final, err := inst.CapturePaneContentFinal()
	require.NoError(t, err)
	assert.Equal(t, "last output", final)
}

func TestInstance_CapturePaneContentPriority_ShouldReturnTheFinalGrid_WhenThePaneHasExited(t *testing.T) {
	inst := &Instance{Title: "exited-priority", Status: Running}
	inst.started.Store(true)
	inst.processManager = exitedPM{screen: "last output"}

	got, err := inst.CapturePaneContentPriority()

	require.NoError(t, err)
	assert.Equal(t, "last output", got)
}

func TestWaitForRedrawQuiescence_ShouldSettleOnTheFinalGrid_WhenThePaneHasExited(t *testing.T) {
	inst := &Instance{Title: "exited-scroll", Status: Running}
	inst.started.Store(true)
	inst.processManager = exitedPM{screen: "last output"}

	settled, deadline, err := waitForRedrawQuiescence(context.Background(), inst.CapturePaneContentPriority, time.Millisecond, time.Second)

	require.NoError(t, err)
	assert.False(t, deadline)
	assert.Equal(t, "last output", settled)
}
