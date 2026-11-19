package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session/instance"
)

func TestStopStuckInstance_MatchingAndStaleness(t *testing.T) {
	t.Parallel()
	mk := func(title, wf string, lastOutput time.Time) *instance.Instance {
		inst, err := instance.NewInstance(instance.InstanceOptions{
			Title: title, Path: t.TempDir(), Program: "claude", WorkflowID: wf,
		})
		require.NoError(t, err)
		inst.SetLastMeaningfulOutput(lastOutput)
		return inst
	}
	cutoff := time.Now().Add(-24 * time.Hour)

	t.Run("recent in-memory output is left alone", func(t *testing.T) {
		t.Parallel()
		inst := mk("same", "wf-1", time.Now())
		err := stopStuckInstance([]*instance.Instance{inst}, "same", "wf-1", cutoff)
		assert.ErrorIs(t, err, workflows.ErrSessionStillActive)
		assert.False(t, inst.IsArchived())
	})
	t.Run("same title in another workflow is not matched", func(t *testing.T) {
		t.Parallel()
		inst := mk("same", "wf-other", time.Now().Add(-48*time.Hour))
		err := stopStuckInstance([]*instance.Instance{inst}, "same", "wf-1", cutoff)
		assert.NoError(t, err)
		assert.False(t, inst.IsArchived())
	})
	t.Run("no in-memory instance is a no-op", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, stopStuckInstance(nil, "same", "wf-1", cutoff))
	})
}
