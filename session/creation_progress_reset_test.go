package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// The stale-creation sweeper fails a Creating session whose progress timestamp is older than
// its threshold. A retried session carries its previous attempt's timestamp, so re-entering
// Creating must restart that clock or the sweeper fails the retry within a minute.

func TestRecoverFromStopped_should_ResetCreationProgressClock(t *testing.T) {
	t.Parallel()
	for _, from := range []domain.Status{domain.Stopped, domain.PermanentlyFailed, domain.Failed} {
		t.Run(from.String(), func(t *testing.T) {
			t.Parallel()
			old := time.Now().Add(-21 * 24 * time.Hour)
			inst := &Instance{Title: "retry-clock", Status: from, creationProgressUpdatedAt: old}

			inst.RecoverFromStopped()

			require.Equal(t, domain.Creating, inst.Status)
			assert.WithinDuration(t, time.Now(), inst.CreationProgressUpdatedAt(), time.Minute)
		})
	}
}

func TestTransitionTo_should_ResetCreationProgressClock_When_EnteringCreating(t *testing.T) {
	t.Parallel()
	old := time.Now().Add(-21 * 24 * time.Hour)
	inst := &Instance{Title: "retry-clock", Status: domain.Failed, creationProgressUpdatedAt: old}

	require.NoError(t, inst.transitionTo(context.Background(), domain.Creating))

	assert.WithinDuration(t, time.Now(), inst.CreationProgressUpdatedAt(), time.Minute)
}
