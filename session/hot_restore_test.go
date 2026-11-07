package session

import (
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

func TestHotRestoreIfLive_NilInstance(t *testing.T) {
	t.Parallel()
	got, _ := HotRestoreIfLive(nil, func() bool { return true })
	assert.Equal(t, HotRestoreError, got)
}
