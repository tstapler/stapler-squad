package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestDeferToReconcile_should_BeTrueOnlyForStatusesWithNoActiveEdge(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status domain.Status
		want   bool
	}{
		{domain.PermanentlyFailed, true},
		{domain.Failed, true},
		{domain.Stopped, false},
		{domain.Active, false},
		{domain.Paused, false},
	} {
		inst, err := session.NewInstance(session.InstanceOptions{Title: "t", Path: t.TempDir(), Program: "claude"})
		if err != nil {
			t.Fatal(err)
		}
		inst.ForceStatus(tc.status)
		assert.Equal(t, tc.want, deferToReconcile(inst), tc.status.String())
	}
}

func TestShouldBootStart(t *testing.T) {
	t.Parallel()
	now := time.Now()
	started := session.NewStartedInstanceForTest(t, "boot-started", nil)

	assert.True(t, shouldBootStart(&session.Instance{Status: domain.Active}), "unstarted active instance boots")
	assert.False(t, shouldBootStart(nil))
	assert.False(t, shouldBootStart(started), "a client's lazy restore may have started it during the boot stagger")
	assert.False(t, shouldBootStart(&session.Instance{Status: domain.Active, ArchivedAt: &now}), "archived is never auto-started")
	assert.False(t, shouldBootStart(&session.Instance{Status: domain.Failed}), "failed is left to reconcile")
}
