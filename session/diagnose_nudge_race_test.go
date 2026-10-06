package session

// diagnose_nudge_race_test.go is the -race regression test for gap #2: prior
// to RecordDiagnoseNudgeAttemptIfBelowCap, RecordDiagnoseNudgeAttempt read
// diagnose_nudge_count, computed nextCount in Go, then issued a plain SET —
// two concurrent attempts at the cap boundary could both read the same stale
// count and both "succeed" (each writing count+1 instead of the second one
// writing count+2), silently under-counting attempts and letting the nudge
// cap be bypassed under concurrency. Run with `go test -race`.

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

// TestRecordDiagnoseNudgeAttempt_ExactlyOneSucceedsAtCapBoundary drives an
// open stuck-state row to exactly one unit of headroom under the cap
// (count = maxAttempts-1), then fires concurrentAttempts goroutines all
// calling Storage.RecordDiagnoseNudgeAttempt at once. With only one unit of
// headroom, exactly one attempt can be the one that reaches the cap
// (justCapped=true); every other concurrent attempt must observe the row
// already at cap and report justCapped=false (RecordDiagnoseNudgeAttempt's
// no-op path), never a spurious second justCapped=true and never an
// over-counted final value.
func TestRecordDiagnoseNudgeAttempt_ExactlyOneSucceedsAtCapBoundary(t *testing.T) {
	storage, repo, itemID, ctx := newDiagnoseNudgeTestFixture(t)

	const maxAttempts = int32(3)
	// Seed to one attempt below the cap, no cooldown, so every concurrent
	// attempt below passes DiagnoseNudgeAllowed's cooldown check and the race
	// is purely on the count itself.
	_, err := repo.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing, maxAttempts-1, nil)
	require.NoError(t, err)

	const concurrentAttempts = 10
	var wg sync.WaitGroup
	results := make([]bool, concurrentAttempts)
	errs := make([]error, concurrentAttempts)
	wg.Add(concurrentAttempts)
	for i := 0; i < concurrentAttempts; i++ {
		go func(idx int) {
			defer wg.Done()
			justCapped, recErr := storage.RecordDiagnoseNudgeAttempt(context.Background(), itemID, domain.StuckReasonBouncing)
			results[idx] = justCapped
			errs[idx] = recErr
		}(i)
	}
	wg.Wait()

	cappedCount := 0
	for i, capped := range results {
		require.NoError(t, errs[i])
		if capped {
			cappedCount++
		}
	}
	assert.Equal(t, 1, cappedCount, "exactly one concurrent attempt must be the one that reaches the cap")

	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, maxAttempts, rows[0].DiagnoseNudgeCount, "the stored count must land exactly at the cap, never over it, regardless of how many attempts raced")
}
