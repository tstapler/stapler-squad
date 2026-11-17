package instance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestHotRestoreIfLive_FailedAttachRestoresTerminalStatus(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t", Status: domain.PermanentlyFailed} // no process manager: Start fails
	got, err := HotRestoreIfLive(inst, func() bool { return true })
	require.Error(t, err)
	assert.Equal(t, HotRestoreError, got)
	assert.Equal(t, domain.PermanentlyFailed, inst.GetLifecycleStatus())
}

func TestHotRestoreIfLive_NilInstance(t *testing.T) {
	t.Parallel()
	got, _ := HotRestoreIfLive(nil, func() bool { return true })
	assert.Equal(t, HotRestoreError, got)
}

func TestHotRestoreIfLive_ArchivedDuringAttachWindowIsRolledBack(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t", Status: domain.PermanentlyFailed}
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
	assert.Equal(t, domain.PermanentlyFailed, inst.GetLifecycleStatus())
}

func TestHotRestoreIfLive_BusyWhenRestartInFlight(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t", Status: domain.Failed}
	inst.SetRetryInFlightForTest(true)
	probed := false
	got, err := HotRestoreIfLive(inst, func() bool { probed = true; return true })
	require.NoError(t, err)
	assert.Equal(t, HotRestoreBusy, got)
	assert.False(t, probed)
	assert.Equal(t, domain.Failed, inst.GetLifecycleStatus())
}

func TestHotRestoreIfLive_ArchivedRaceRestoresStartedFlag(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "t", Status: domain.Failed}
	inst.started.Store(true)
	got, _ := HotRestoreIfLive(inst, func() bool {
		now := time.Now()
		inst.mu.Lock()
		inst.ArchivedAt = &now
		inst.mu.Unlock()
		return true
	})
	assert.Equal(t, HotRestoreArchived, got)
	assert.True(t, inst.Started())
}
