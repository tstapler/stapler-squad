package session

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBumpCreationEpoch_should_Increment_When_CalledTwice verifies Story 1.2.1:
// bumpCreationEpoch increments exactly once per call (simulating cancel-then-retry).
func TestBumpCreationEpoch_should_Increment_When_CalledTwice(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-bump-epoch", Status: Creating}

	require.Equal(t, uint64(0), inst.CreationEpoch())

	first := inst.bumpCreationEpoch()
	assert.Equal(t, uint64(1), first)

	second := inst.bumpCreationEpoch()
	assert.Equal(t, uint64(2), second)

	assert.Equal(t, uint64(2), inst.CreationEpoch())
}

// TestTryForceStatusIfEpoch_should_ApplyWrite_When_EpochMatches verifies Story
// 1.2.2's happy path: a caller presenting the current epoch wins the write.
func TestTryForceStatusIfEpoch_should_ApplyWrite_When_EpochMatches(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-force-status-match", Status: Creating}
	inst.bumpCreationEpoch()
	inst.bumpCreationEpoch()
	inst.bumpCreationEpoch() // epoch == 3

	applied := inst.TryForceStatusIfEpoch(3, Active, "")

	assert.True(t, applied)
	assert.Equal(t, Active, inst.Status)
}

// TestTryForceStatusIfEpoch_should_ReturnFalse_When_EpochIsStale verifies Story
// 1.2.2's fencing guarantee: a stale caller (epoch already bumped past its
// captured value by a cancel) no-ops and reports false.
func TestTryForceStatusIfEpoch_should_ReturnFalse_When_EpochIsStale(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-force-status-stale", Status: Creating}
	inst.bumpCreationEpoch()
	inst.bumpCreationEpoch()
	inst.bumpCreationEpoch() // epoch == 3

	applied := inst.TryForceStatusIfEpoch(2, Active, "")

	assert.False(t, applied)
	assert.Equal(t, Creating, inst.Status, "status must be unchanged on a stale write")
}

// TestTryForceStatusIfEpoch_should_ProduceExactlyOneWinner_When_CalledConcurrently
// verifies Story 1.2.2's concurrency guarantee under -race -count=50: two
// goroutines racing with the same captured epoch (success-vs-stale-timeout) must
// produce exactly one true return.
func TestTryForceStatusIfEpoch_should_ProduceExactlyOneWinner_When_CalledConcurrently(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-force-status-race", Status: Creating}
	li := NewLiveInstance(inst)
	defer li.Stop()

	var wins int64
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if inst.TryForceStatusIfEpoch(0, Active, "") {
			atomic.AddInt64(&wins, 1)
		}
	}()
	go func() {
		defer wg.Done()
		if inst.TryForceStatusIfEpoch(0, Failed, "timed out") {
			atomic.AddInt64(&wins, 1)
		}
	}()
	wg.Wait()

	assert.Equal(t, int64(1), atomic.LoadInt64(&wins), "exactly one caller must win the terminal write")
}

// TestTryStartRetry_should_BumpEpochAndResetToCreating_When_StatusIsFailed
// verifies Story 1.2.3's happy path.
func TestTryStartRetry_should_BumpEpochAndResetToCreating_When_StatusIsFailed(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-retry-happy", Status: Failed}
	priorEpoch := inst.CreationEpoch()

	before := time.Now()
	newEpoch, started := inst.TryStartRetry()
	after := time.Now()

	require.True(t, started)
	assert.Equal(t, priorEpoch+1, newEpoch)
	assert.Equal(t, Creating, inst.Status)
	assert.False(t, inst.CreationProgressUpdatedAt().Before(before), "CreationProgressUpdatedAt must be bumped to now, not left at its stale pre-retry value")
	assert.False(t, inst.CreationProgressUpdatedAt().After(after))
}

// TestTryStartRetry_should_ReturnNotStarted_When_StatusIsNotFailed verifies
// Story 1.2.3's guard: retry only applies to a Failed instance.
func TestTryStartRetry_should_ReturnNotStarted_When_StatusIsNotFailed(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-retry-not-failed", Status: Active}

	newEpoch, started := inst.TryStartRetry()

	assert.False(t, started)
	assert.Equal(t, uint64(0), newEpoch)
	assert.Equal(t, Active, inst.Status, "instance must be untouched when not Failed")
	assert.Equal(t, uint64(0), inst.CreationEpoch())
}

// TestTryStartRetry_should_StartExactlyOnce_When_CalledConcurrently verifies
// Story 1.2.3's double-click-retry fencing guarantee under -race -count=50.
func TestTryStartRetry_should_StartExactlyOnce_When_CalledConcurrently(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-retry-race", Status: Failed}
	li := NewLiveInstance(inst)
	defer li.Stop()

	var starts int64
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, started := inst.TryStartRetry(); started {
			atomic.AddInt64(&starts, 1)
		}
	}()
	go func() {
		defer wg.Done()
		if _, started := inst.TryStartRetry(); started {
			atomic.AddInt64(&starts, 1)
		}
	}()
	wg.Wait()

	assert.Equal(t, int64(1), atomic.LoadInt64(&starts), "exactly one caller must start the retry")
	assert.Equal(t, Creating, inst.Status)
}
