package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session"
)

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
