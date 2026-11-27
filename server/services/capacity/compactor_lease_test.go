package capacity

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/instance"
)

// recordingPM captures SendKeys traffic and reports one pane change after each
// write so instance.SubmitContentWithEnter's settle/confirm polls succeed.
type recordingPM struct {
	instance.ProcessManager
	mu      sync.Mutex
	sent    []string
	pending bool
}

func (r *recordingPM) GetSessionIdentifier() string         { return "" }
func (r *recordingPM) GetPanePID() (int32, error)           { return 0, errors.New("no pane") }
func (r *recordingPM) HasSession() bool                     { return true }
func (r *recordingPM) IsAlive() bool                        { return true }
func (r *recordingPM) HasMeaningfulContent(string) bool     { return false }
func (r *recordingPM) FilterBanners(c string) (string, int) { return c, 0 }
func (r *recordingPM) SendKeys(keys string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, keys)
	r.pending = true
	return len(keys), nil
}

func (r *recordingPM) HasUpdated() (bool, bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending {
		r.pending = false
		return true, false, "updated"
	}
	return false, false, ""
}

// CapturePaneContent reports no capture so submit confirmation falls back to HasUpdated.
func (r *recordingPM) CapturePaneContent() (string, error) {
	return "", errors.New("capture unsupported by fake")
}

func (r *recordingPM) writes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent...)
}

// The default compactor is the chain's acquirer: a busy lease is an error the
// monitor already logs, not a blocked cycle.
func TestCapacityMonitorDefaultCompactor_ShouldReturnErrLeaseBusyWithZeroWrites_WhenTheLeaseIsHeld(t *testing.T) {
	t.Parallel()
	pm := &recordingPM{}
	inst := instance.NewStartedInstanceForTest(t, "compactor-lease", pm)
	inst.SetLeaseFlag(&instance.LeaseFlag{})
	m := NewCapacityMonitor(CapacityMonitorParams{})
	held, ok := inst.TryTerminalWriteLease(instance.LeaseWriterDriver)
	require.True(t, ok)

	err := m.compactor(context.Background(), inst, "/compact")
	require.ErrorIs(t, err, instance.ErrLeaseBusy)
	assert.Empty(t, pm.writes())

	held.Release()
	require.NoError(t, m.compactor(context.Background(), inst, "/compact"))
	assert.Equal(t, []string{"/compact", instance.EnterKeySequence}, pm.writes())
}
