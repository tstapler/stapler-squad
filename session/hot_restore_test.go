package session

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHotRestoreIfLive_Outcomes(t *testing.T) {
	t.Parallel()
	now := time.Now()
	for _, tc := range []struct {
		name       string
		status     Status
		archivedAt *time.Time
		live       bool
		want       HotRestoreOutcome
		wantProbe  bool
		wantStatus Status
	}{
		{"failed live", Failed, nil, true, HotRestoreCleared, true, Active},
		{"permanently failed live", PermanentlyFailed, nil, true, HotRestoreCleared, true, Active},
		{"stopped live", Stopped, nil, true, HotRestoreCleared, true, Active},
		{"permanently failed dead", PermanentlyFailed, nil, false, HotRestoreNotLive, true, PermanentlyFailed},
		{"failed dead", Failed, nil, false, HotRestoreNotLive, true, Failed},
		{"archived live", PermanentlyFailed, &now, true, HotRestoreArchived, false, PermanentlyFailed},
		{"active", Active, nil, true, HotRestoreNotTerminal, false, Active},
		{"paused", Paused, nil, true, HotRestoreNotTerminal, false, Paused},
		{"crashed", Crashed, nil, true, HotRestoreNotTerminal, false, Crashed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inst := &Instance{
				Title:          "t",
				Status:         tc.status,
				ArchivedAt:     tc.archivedAt,
				processManager: &fakeLivenessProcessManager{alive: tc.live},
			}
			probed := false
			got, err := HotRestoreIfLive(inst, func() bool {
				probed = true
				return tc.live
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantProbe, probed)
			assert.Equal(t, tc.wantStatus, inst.GetLifecycleStatus())
		})
	}
}

func TestHotRestoreIfLive_FailedAttachRestoresTerminalStatus(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t", Status: PermanentlyFailed} // no process manager: Start fails
	got, err := HotRestoreIfLive(inst, func() bool { return true })
	require.Error(t, err)
	assert.Equal(t, HotRestoreError, got)
	assert.Equal(t, PermanentlyFailed, inst.GetLifecycleStatus())
}

func TestHotRestoreIfLive_NilInstance(t *testing.T) {
	t.Parallel()
	got, _ := HotRestoreIfLive(nil, func() bool { return true })
	assert.Equal(t, HotRestoreError, got)
}

// failingAttachPM models a live pane whose hot attach (RestoreWithWorkDir) fails.
// Close is what kill-session does on the tmux backend.
type failingAttachPM struct {
	fakeLivenessProcessManager
	closeCalls atomic.Int32
}

func (m *failingAttachPM) HasSession() bool { return true }
func (m *failingAttachPM) Close() error     { m.closeCalls.Add(1); return nil }
func (m *failingAttachPM) RestoreWithWorkDir(string) error {
	return errors.New("attach refused")
}

// releasingFailingAttachPM adds the tymux backend's AdoptedSessionReleaser.
type releasingFailingAttachPM struct {
	failingAttachPM
	released atomic.Int32
}

func (m *releasingFailingAttachPM) ReleaseAdoptedSession() bool { m.released.Add(1); return true }

func TestHotRestoreIfLive_FailedAttachNeverKillsTheLiveSession(t *testing.T) {
	t.Parallel()
	t.Run("tmux-like backend without a releaser", func(t *testing.T) {
		t.Parallel()
		pm := &failingAttachPM{fakeLivenessProcessManager: fakeLivenessProcessManager{alive: true}}
		inst := &Instance{Title: "t", Status: PermanentlyFailed, processManager: pm}
		got, err := HotRestoreIfLive(inst, func() bool { return true })
		require.Error(t, err)
		assert.Equal(t, HotRestoreError, got)
		assert.Zero(t, pm.closeCalls.Load(), "failed hot attach must not kill the pre-existing session")
		assert.Equal(t, PermanentlyFailed, inst.GetLifecycleStatus())
	})
	t.Run("tymux-like backend with a releaser", func(t *testing.T) {
		t.Parallel()
		pm := &releasingFailingAttachPM{failingAttachPM: failingAttachPM{fakeLivenessProcessManager: fakeLivenessProcessManager{alive: true}}}
		inst := &Instance{Title: "t", Status: Failed, processManager: pm}
		_, err := HotRestoreIfLive(inst, func() bool { return true })
		require.Error(t, err)
		assert.Zero(t, pm.closeCalls.Load(), "failed hot attach must not kill the pre-existing pane")
		assert.EqualValues(t, 1, pm.released.Load(), "the adopted pane is released")
		assert.Equal(t, Failed, inst.GetLifecycleStatus())
	})
}

func TestHotRestoreIfLive_ArchivedDuringAttachWindowIsRolledBack(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t", Status: PermanentlyFailed}
	// The probe runs after the first archival check and before the status flip.
	got, err := HotRestoreIfLive(inst, func() bool {
		now := time.Now()
		inst.mu.Lock()
		inst.ArchivedAt = &now
		inst.mu.Unlock()
		return true
	})
	require.NoError(t, err)
	assert.Equal(t, HotRestoreArchived, got)
	assert.Equal(t, PermanentlyFailed, inst.GetLifecycleStatus())
}

func TestHotRestoreIfLive_BusyWhenRestartInFlight(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t", Status: Failed}
	inst.SetRetryInFlightForTest(true)
	probed := false
	got, err := HotRestoreIfLive(inst, func() bool { probed = true; return true })
	require.NoError(t, err)
	assert.Equal(t, HotRestoreBusy, got)
	assert.False(t, probed)
	assert.Equal(t, Failed, inst.GetLifecycleStatus())
}

func TestHotRestoreIfLive_ConcurrentCallsAttachOnce(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	var restores atomic.Int32
	pm := &fakeLivenessProcessManager{alive: true, restoreRelease: release, onRestore: func() { restores.Add(1) }}
	inst := &Instance{Title: "t", Status: Failed, processManager: pm}

	var wg sync.WaitGroup
	outcomes := make(chan HotRestoreOutcome, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _ := HotRestoreIfLive(inst, func() bool { return true })
			outcomes <- got
		}()
	}
	// The loser blocks on hotRestoreMu until the winner finishes its attach.
	require.Eventually(t, func() bool { return restores.Load() == 0 && inst.retryInFlight.Load() }, time.Second, time.Millisecond)
	close(release)
	wg.Wait()
	close(outcomes)

	counts := map[HotRestoreOutcome]int{}
	for o := range outcomes {
		counts[o]++
	}
	assert.Equal(t, 1, counts[HotRestoreCleared])
	assert.Equal(t, 1, counts[HotRestoreNotTerminal])
	assert.EqualValues(t, 1, restores.Load(), "RestoreWithWorkDir runs once")
}
