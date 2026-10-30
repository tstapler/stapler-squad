package session

import (
	"context"
	"sync/atomic"
	"testing"

	"entgo.io/ent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countLastAddedToQueueWrites counts session mutations that set
// last_added_to_queue, i.e. the single-row UPDATEs that queue behind every
// other write on the one writer connection.
func countLastAddedToQueueWrites(t *testing.T, storage *Storage) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	storage.GetEntClient().Session.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if _, set := m.Field("last_added_to_queue"); set {
				n.Add(1)
			}
			return next.Mutate(ctx, m)
		})
	})
	return &n
}

// A session that stays in the queue unchanged is re-detected on every poll
// cycle. Re-stamping last_added_to_queue each time rewrote the same row
// forever; only a new or changed entry should persist.
func TestAddDetectedItem_Should_PersistLastAddedToQueueOnce_When_EntryUnchanged(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	inst := newTestInstance("queue-persist")
	require.NoError(t, storage.AddInstance(inst))
	writes := countLastAddedToQueueWrites(t, storage)

	rqp := NewReviewQueuePoller(NewReviewQueue(), NewInstanceStatusManager(), storage)
	result := DetectionResult{Action: DetectionActionAdd, Reason: ReasonIdle, Priority: PriorityMedium, Context: "idle"}
	snap := inst.Snapshot()

	for i := 0; i < 5; i++ {
		rqp.addDetectedItem(inst, snap, result, InstanceStatusInfo{})
	}
	assert.Equal(t, int64(1), writes.Load(), "five identical detections must persist once")

	result.Context = "idle, new output"
	rqp.addDetectedItem(inst, snap, result, InstanceStatusInfo{})
	assert.Equal(t, int64(2), writes.Load(), "a changed entry must persist again")
}
