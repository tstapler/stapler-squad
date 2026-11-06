package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/tymux"
)

// neverWait fails the test if a driver waits on a timer: a dead pane must end
// the loop on the first capture instead of running out its window.
func neverWait(t *testing.T) func(time.Duration) <-chan time.Time {
	return func(d time.Duration) <-chan time.Time {
		if d == ReplyClosedPollEvery {
			t.Fatalf("driver waited %v on an exited pane", d)
		}
		return nil
	}
}

func TestSubmitReplyOnce_ShouldFailFastWithIndeterminateStage_WhenPaneExitsAfterTheByte(t *testing.T) {
	dialog := loadDialogFixture(t, "question_single_select_3opts.txt")
	inst, lease := replyInstanceAndLease(t)
	pane := &fakeReplyPane{owner: replyOwner, captures: []string{dialog}, capErr: &tymux.PaneExitedError{}}

	stage, err := SubmitReplyOnce(context.Background(), pane, lease, '1', ReplyOptions{Dialog: colorQuestion, After: neverWait(t)})

	require.ErrorIs(t, err, tymux.ErrPaneExited)
	assert.Equal(t, StageWriteEntered, stage)
	AssertLeaseFree(t, inst)
}

// exitingSim is the tabbed simulator whose process exits once deadAfter keys
// have been written (0 means it is dead from the start).
type exitingSim struct {
	*tabbedSim
	deadAfter int
}

func (e *exitingSim) CapturePaneContent() (string, error) {
	if len(e.written()) >= e.deadAfter {
		return "", &tymux.PaneExitedError{}
	}
	return e.tabbedSim.CapturePaneContent()
}

func TestSubmitReplySequence_ShouldFailFastWithNoWrites_WhenPaneIsAlreadyExited(t *testing.T) {
	sim := &exitingSim{tabbedSim: newSim()}
	_, lease := replyInstanceAndLease(t)
	opts := seqOpts()
	opts.After = neverWait(t)

	res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), opts)

	require.ErrorIs(t, res.Err, tymux.ErrPaneExited)
	assert.Equal(t, StageNotStarted, res.Stage)
	assert.Zero(t, res.Written)
	assert.Empty(t, sim.written())
}

func TestSubmitReplySequence_ShouldFailFastAsIndeterminate_WhenPaneExitsAfterTheLastKey(t *testing.T) {
	steps := recordedSteps(t)
	sim := &exitingSim{tabbedSim: newSim(), deadAfter: len(recordedKeys(t))}
	_, lease := replyInstanceAndLease(t)
	opts := seqOpts()
	opts.After = neverWait(t)

	res := SubmitReplySequence(context.Background(), sim, lease, steps, opts)

	require.ErrorIs(t, res.Err, tymux.ErrPaneExited)
	assert.Equal(t, StageWriteEntered, res.Stage)
	assert.Equal(t, 9, res.Written)
}

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

var errRestarting = &tymux.PaneExitedError{RestartPending: true}

// restartingPane fails its first captures as exited-but-restarting, then
// delegates, the way a pane looks while tymuxd backs off and relaunches it.
type restartingPane struct {
	*fakeReplyPane
	pendingFor int
	calls      int
}

func (p *restartingPane) CapturePaneContent() (string, error) {
	p.calls++
	if p.calls <= p.pendingFor {
		return "", errRestarting
	}
	return p.fakeReplyPane.CapturePaneContent()
}

func TestSubmitReplyOnce_ShouldKeepWaiting_WhenPaneIsRestarting(t *testing.T) {
	_, lease := replyInstanceAndLease(t)
	pane := &restartingPane{fakeReplyPane: &fakeReplyPane{owner: replyOwner, captures: []string{"closed\n❯ \n"}}, pendingFor: 3}

	stage, err := SubmitReplyOnce(context.Background(), pane, lease, '1', ReplyOptions{Dialog: colorQuestion, After: readyAfter})

	require.NoError(t, err)
	assert.Equal(t, StageDialogClosed, stage)
	assert.Equal(t, 4, pane.calls, "kept polling through the restart")
}

func TestSubmitReplyOnce_ShouldEndAtTheWindowWithoutError_WhenRestartNeverFinishes(t *testing.T) {
	_, lease := replyInstanceAndLease(t)
	pane := &restartingPane{fakeReplyPane: &fakeReplyPane{owner: replyOwner, captures: []string{"x"}}, pendingFor: 1 << 30}

	stage, err := SubmitReplyOnce(context.Background(), pane, lease, '1', ReplyOptions{Dialog: colorQuestion, After: readyAfter})

	require.NoError(t, err)
	assert.Equal(t, StageDialogStillOpen, stage)
}

// restartingSim is the tabbed simulator that reports a restart for the first
// pending captures once afterWrites keys are written.
type restartingSim struct {
	*tabbedSim
	afterWrites int
	pending     int
	served      int
}

func (r *restartingSim) CapturePaneContent() (string, error) {
	if len(r.written()) >= r.afterWrites && r.served < r.pending {
		r.served++
		return "", errRestarting
	}
	return r.tabbedSim.CapturePaneContent()
}

func TestSubmitReplySequence_ShouldRideThroughARestart_BetweenSteps(t *testing.T) {
	sim := &restartingSim{tabbedSim: newSim(), afterWrites: 1, pending: 2}
	_, lease := replyInstanceAndLease(t)

	res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), seqOpts())

	require.NoError(t, res.Err)
	assert.Equal(t, StageDialogClosed, res.Stage)
	assert.Equal(t, 2, sim.served)
}

func TestSubmitReplySequence_ShouldRideThroughARestart_AfterTheLastKey(t *testing.T) {
	sim := &restartingSim{tabbedSim: newSim(), afterWrites: len(recordedKeys(t)), pending: 2}
	_, lease := replyInstanceAndLease(t)

	res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), seqOpts())

	require.NoError(t, res.Err)
	assert.Equal(t, StageDialogClosed, res.Stage)
	assert.Equal(t, 2, sim.served)
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
