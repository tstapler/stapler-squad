package session

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// Real-loop tests: they run the actual runSessionDriverWithPrompt goroutine with
// shrunk timing. Not parallel: they swap the process-wide driver timing, and
// t.Setenv isolates HOME from real conversation history.

const loopTotalTimeout = 150 * time.Millisecond

func startLoopDriver(t *testing.T, title string, status detection.DetectedStatus, tags ...string) *Instance {
	t.Helper()
	inst, _ := startLoopDriverWith(t, title, status, loopTotalTimeout, "✻ Working on it… (12m 3s)", tags...)
	return inst
}

func startLoopDriverWith(t *testing.T, title string, status detection.DetectedStatus, total time.Duration, pane string, tags ...string) (*Instance, *stuckDialogProcessManager) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	testDriverTiming.Store(&driverTiming{
		pollInterval: 20 * time.Millisecond,
		readyTimeout: 500 * time.Millisecond,
		totalTimeout: total,
	})
	t.Cleanup(func() { testDriverTiming.Store(nil) })

	pm := &stuckDialogProcessManager{dialogText: pane}
	inst := (&Instance{Title: title, Status: domain.Active, Tags: tags}).WithProcessManagerForTest(pm)
	inst.MarkStarted()
	source := NewPiStatusSource(inst.Title, nil)
	source.SetStatusForTest(status)
	manager := NewInstanceStatusManager()
	manager.RegisterPiStatusSource(inst.Title, source)
	inst.SetStatusManager(manager)
	StartSessionDriverForTest(t, inst, "/tmp")
	return inst, pm
}

func waitDriverExit(t *testing.T, inst *Instance, why string) {
	t.Helper()
	wait.RequireEventually(t, func() bool { return !inst.DriverRunning() }, 5*time.Second, 5*time.Millisecond, why)
}

// An interactive driver on a live, working pane must outlive the autonomous
// safety-net deadline, or its quiet detection can never run.
func TestSessionDriverLoop_InteractiveSurvivesPastTotalDeadline(t *testing.T) {
	inst := startLoopDriver(t, "loop-interactive-survives", detection.StatusExecuting)
	started := time.Now()
	wait.RequireEventually(t, func() bool { return time.Since(started) > 4*loopTotalTimeout }, 5*time.Second, 5*time.Millisecond, "clock never advanced")
	if !inst.DriverRunning() {
		t.Fatal("interactive driver ended at the autonomous total deadline")
	}
}

func TestSessionDriverLoop_AutonomousStillEndsAtTotalDeadline(t *testing.T) {
	inst := startLoopDriver(t, "loop-backlog-deadline", detection.StatusExecuting, TagBacklogWork)
	waitDriverExit(t, inst, "backlog driver never ended at its total deadline")
}

func TestSessionDriverLoop_InteractiveEndsOnArchiveEvenWhenIdle(t *testing.T) {
	inst := startLoopDriver(t, "loop-archive", detection.StatusIdle)
	now := time.Now()
	inst.MutateForTest(func(i *Instance) { i.ArchivedAt = &now })
	waitDriverExit(t, inst, "archived interactive driver kept running")
}

func TestSessionDriverLoop_InteractiveEndsOnHibernateAndCrash(t *testing.T) {
	for _, st := range []domain.Status{domain.Hibernated, domain.Crashed} {
		t.Run(st.String(), func(t *testing.T) {
			inst := startLoopDriver(t, "loop-"+st.String(), detection.StatusExecuting)
			inst.MutateForTest(func(i *Instance) { i.Status = st })
			waitDriverExit(t, inst, "driver kept running for a "+st.String()+" session")
		})
	}
}

// Auto-answering a trust dialog is scoped to the driver's first total-timeout
// window for interactive sessions; a positive control proves the same pane is
// answered inside the window.
func TestSessionDriverLoop_InteractiveDialogsNotAnsweredAfterWindow(t *testing.T) {
	_, open := startLoopDriverWith(t, "loop-dialog-open", detection.StatusExecuting, 10*time.Second, trustDialogText)
	wait.RequireEventually(t, func() bool { return open.sendKeysCount.Load() >= 1 }, 5*time.Second, 5*time.Millisecond, "dialog inside the window was never answered")

	inst, closed := startLoopDriverWith(t, "loop-dialog-closed", detection.StatusExecuting, time.Millisecond, trustDialogText)
	wait.RequireEventually(t, func() bool { return closed.contentCalls.Load() >= 10 }, 5*time.Second, 5*time.Millisecond, "driver never polled")
	if got := closed.sendKeysCount.Load(); got != 0 {
		t.Fatalf("interactive dialog answered %d times after the window", got)
	}
	if !inst.DriverRunning() {
		t.Fatal("interactive driver should keep running after the dialog window")
	}
}
