package services

import (
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// unrestoredTymuxPM models a tymux Instance that was loaded from its record but
// not restored yet: no pane (HasSession false) while the daemon still holds the
// record's stored session live.
type unrestoredTymuxPM struct {
	session.ProcessManager // nil; any unlisted method must not be reached
	restored               atomic.Int32
	attached               atomic.Bool
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
