package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session"
)

func TestHotRestoreEligible_ProbesStoredBackendIdentity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status session.Status
		live   bool
		want   bool
	}{
		{"failed live", session.Failed, true, true},
		{"permanently failed live", session.PermanentlyFailed, true, true},
		{"stopped live", session.Stopped, true, true},
		{"failed dead", session.Failed, false, false},
		{"active", session.Active, true, false},
		{"paused", session.Paused, true, false},
		{"hibernated", session.Hibernated, true, false},
		{"crashed", session.Crashed, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inst := &session.Instance{Status: tc.status}
			probed := false
			got := hotRestoreEligible(inst, func() bool {
				probed = true
				return tc.live
			})
			assert.Equal(t, tc.want, got)
			assert.Equal(t, inst.IsHotRestoreRecoverable(), probed)
		})
	}
}

func TestHotRestoreEligible_ArchivedDoesNotProbe(t *testing.T) {
	t.Parallel()
	now := time.Now()
	inst := &session.Instance{Status: session.PermanentlyFailed, ArchivedAt: &now}
	assert.False(t, hotRestoreEligible(inst, func() bool {
		t.Fatal("archived session must not be probed or revived")
		return true
	}))
}

func TestDeferToReconcile_should_BeTrueOnlyForStatusesWithNoActiveEdge(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status session.Status
		want   bool
	}{
		{session.PermanentlyFailed, true},
		{session.Failed, true},
		{session.Stopped, false},
		{session.Active, false},
		{session.Paused, false},
	} {
		inst, err := session.NewInstance(session.InstanceOptions{Title: "t", Path: t.TempDir(), Program: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		inst.ForceStatus(tc.status)
		assert.Equal(t, tc.want, deferToReconcile(inst), tc.status.String())
	}
}
