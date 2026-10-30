package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The stale-creation sweeper fails a Creating session whose progress timestamp is older than
// its threshold. A retried session carries its previous attempt's timestamp, so re-entering
// Creating must restart that clock or the sweeper fails the retry within a minute.

func TestRecoverFromStopped_should_ResetCreationProgressClock(t *testing.T) {
	t.Parallel()
	for _, from := range []Status{Stopped, PermanentlyFailed, Failed} {
		t.Run(from.String(), func(t *testing.T) {
			t.Parallel()
			old := time.Now().Add(-21 * 24 * time.Hour)
			inst := &Instance{Title: "retry-clock", Status: from, creationProgressUpdatedAt: old}

			inst.RecoverFromStopped()

			require.Equal(t, Creating, inst.Status)
			assert.WithinDuration(t, time.Now(), inst.CreationProgressUpdatedAt(), time.Minute)
		})
	}
}

func TestTransitionTo_should_ResetCreationProgressClock_When_EnteringCreating(t *testing.T) {
	t.Parallel()
	old := time.Now().Add(-21 * 24 * time.Hour)
	inst := &Instance{Title: "retry-clock", Status: Failed, creationProgressUpdatedAt: old}

	require.NoError(t, inst.transitionTo(context.Background(), Creating))

	assert.WithinDuration(t, time.Now(), inst.CreationProgressUpdatedAt(), time.Minute)
}
