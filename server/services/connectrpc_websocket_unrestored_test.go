package services

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tymux"
)

// unrestoredTymuxPM models a tymux Instance that was loaded from its record but
// not restored yet: no pane (HasSession false) while the daemon still holds the
// record's stored session live.
type unrestoredTymuxPM struct {
	session.ProcessManager // nil; any unlisted method must not be reached
	restored               atomic.Int32
	attached               atomic.Bool
	exitCallbackSet        atomic.Int32
}

func (p *unrestoredTymuxPM) HasSession() bool            { return p.attached.Load() }
func (p *unrestoredTymuxPM) StoredSessionLive() bool     { return true }
func (p *unrestoredTymuxPM) HasLiveSessionNoCache() bool { return p.attached.Load() }
func (p *unrestoredTymuxPM) RestoreWithWorkDir(string) error {
	p.restored.Add(1)
	p.attached.Store(true)
	return nil
}
func (p *unrestoredTymuxPM) GetPTY() (*os.File, error) { return os.Stdin, nil }
func (p *unrestoredTymuxPM) ResetExitOnce()            {}
func (p *unrestoredTymuxPM) SetOnExitCallback(func(string)) {
	p.exitCallbackSet.Add(1)
}

// ptylessUnrestoredPM restores a pane (RestoreWithWorkDir returns nil, like the real
// RestoreProcess) but has no PTY attached.
type ptylessUnrestoredPM struct{ *unrestoredTymuxPM }

func (p *ptylessUnrestoredPM) GetPTY() (*os.File, error) { return nil, errors.New("pty attach failed") }

type retainedTymuxPM struct {
	*unrestoredTymuxPM
	adopted atomic.Int32
}

func (p *retainedTymuxPM) AdoptForViewing(context.Context) error {
	p.adopted.Add(1)
	p.attached.Store(true)
	return nil
}

type retainedPaintManager struct{ tymux.TymuxManager }

func (retainedPaintManager) HasSession() bool { return true }
func (retainedPaintManager) CapturePaneRedraw(context.Context) (string, bool, bool, error) {
	return "retained first paint", false, true, nil
}

func TestStreamPaths_RetainedTymuxFirstPaintAndWriter(t *testing.T) {
	t.Parallel()
	for _, status := range []session.Status{session.Failed, session.PermanentlyFailed, session.Stopped, session.Crashed} {
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()
			inst := session.NewStartedInstanceForTest(t, "paint", session.NewTymuxBackend(retainedPaintManager{}))
			inst.Status = status
			inst.Backend = session.BackendTymux
			frame, ok := fullStateFrame(context.Background(), inst)
			require.True(t, ok)
			require.Equal(t, "retained first paint", frame.data)
			h := NewConnectRPCWebSocketHandler(nil, nil, nil)
			require.Nil(t, h.writerFor(inst), "retained pane cannot vote for resize or send input")
			require.True(t, inst.Started(), "viewing does not change the existing start flag")
			require.Equal(t, status, inst.GetLifecycleStatus())
		})
	}
}

func TestStreamPaths_RetainedTymuxNeverRestores(t *testing.T) {
	t.Parallel()
	for _, status := range []session.Status{session.Failed, session.PermanentlyFailed, session.Stopped, session.Crashed} {
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()
			pm := &retainedTymuxPM{unrestoredTymuxPM: &unrestoredTymuxPM{}}
			inst := session.NewStartedInstanceForTest(t, "retained", pm)
			inst.Status = status // fixture, before the first snapshot is published
			h := NewConnectRPCWebSocketHandler(nil, nil, nil)
			alive, err := h.ensureHubBackendAlive(inst, "sess")
			require.NoError(t, err)
			require.False(t, alive, "a renderable pane is not proof of a writable process")
			require.NoError(t, h.ensureControlModeStarted(inst, "sess", &pumpTestController{}))
			require.Equal(t, int32(2), pm.adopted.Load())
			require.Zero(t, pm.restored.Load())
			require.Equal(t, status, inst.GetLifecycleStatus())
		})
	}
}

// A not-yet-restored tymux Instance with a live stored session must still be
// restored by the stream paths: tymux StartControlMode attaches nothing without
// a pane, so skipping the restore leaves a blank terminal.
func TestStreamPaths_should_RestoreUnrestoredTymuxInstance_When_StoredSessionIsLive(t *testing.T) {
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)

	t.Run("ensureHubBackendAlive", func(t *testing.T) {
		pm := &unrestoredTymuxPM{}
		inst := session.NewStartedInstanceForTest(t, "unrestored-hub", pm)

		alive, err := h.ensureHubBackendAlive(inst, "sess")

		require.NoError(t, err)
		require.True(t, alive)
		require.Equal(t, int32(1), pm.restored.Load(), "must restore the pane-less Instance")
	})

	t.Run("ensureControlModeStarted", func(t *testing.T) {
		pm := &unrestoredTymuxPM{}
		inst := session.NewStartedInstanceForTest(t, "unrestored-control", pm)
		streamer := &pumpTestController{}

		require.NoError(t, h.ensureControlModeStarted(inst, "sess", streamer))

		require.Equal(t, int32(1), pm.restored.Load(), "must restore the pane-less Instance")
		require.Equal(t, int32(1), streamer.startControlModeCalls.Load())
	})
}

// A boot-loaded Active tymux Instance is not started until something starts it. The lazy restore
// on attach must leave it started, or input, resize and scroll-forward are all refused with
// "cannot send input to instance that has not been started" while the pane still renders.
func TestStreamPaths_should_MarkUnstartedActiveInstanceStarted_When_LazyRestoreSucceeds(t *testing.T) {
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)

	t.Run("ensureHubBackendAlive", func(t *testing.T) {
		pm := &unrestoredTymuxPM{}
		inst := session.NewUnstartedActiveInstanceForTest(t, "lazy-hub", pm)

		alive, err := h.ensureHubBackendAlive(inst, "sess")

		require.NoError(t, err)
		require.True(t, alive, "a restore that attached a pane is alive; the started gate must not veto it")
		require.True(t, inst.Started(), "attach must leave the restored instance started so input is accepted")
		require.Equal(t, int32(1), pm.exitCallbackSet.Load(), "startLocked never ran, so marking started must register the exit callback")
	})

	t.Run("ensureControlModeStarted", func(t *testing.T) {
		pm := &unrestoredTymuxPM{}
		inst := session.NewUnstartedActiveInstanceForTest(t, "lazy-control", pm)

		require.NoError(t, h.ensureControlModeStarted(inst, "sess", &pumpTestController{}))

		require.True(t, inst.Started(), "attach must leave the restored instance started so input is accepted")
		require.Equal(t, int32(1), pm.exitCallbackSet.Load(), "startLocked never ran, so marking started must register the exit callback")
	})
}

// RestoreProcess returns nil when the PTY attach fails, so a nil error alone must not mark the
// instance started: input would be accepted for a pane with nothing attached.
func TestEnsureControlModeStarted_should_NotMarkStarted_When_RestoredWithoutPTY(t *testing.T) {
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	pm := &ptylessUnrestoredPM{&unrestoredTymuxPM{}}
	inst := session.NewUnstartedActiveInstanceForTest(t, "lazy-control-nopty", pm)
	var started atomic.Int32
	inst.RegisterLifecycleListener(startedCounter{&started})
	streamer := &pumpTestController{}

	require.NoError(t, h.ensureControlModeStarted(inst, "sess", streamer))

	require.Equal(t, int32(1), pm.restored.Load())
	require.False(t, inst.Started(), "a PTY-less restore must not mark the instance started")
	require.Zero(t, started.Load(), "no EventStarted without a PTY")
	require.Equal(t, int32(1), streamer.startControlModeCalls.Load(), "control mode still starts, as before")
}

type startedCounter struct{ n *atomic.Int32 }

func (c startedCounter) OnLifecycleEvent(e session.LifecycleEvent, _ string) {
	if e == session.EventStarted {
		c.n.Add(1)
	}
}
