package services

// nudge_cap_store_test.go covers Epic 3.3's three stories: the ent-backed
// NudgeCapStore's cap/cooldown decisions (3.3.1), diagnoseNudgeGuardMu's
// concurrency guard (3.3.2), and the cap-check closure's fail-closed
// never-log-only contract (3.3.3). See this package's nudge_cap_store.go doc
// comment for why 3.3.3's test lives here rather than
// session/diagnose/nudge_gate_test.go as project_plans/backlog-diagnose-and-nudge/implementation/plan.md
// literally names it.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// newTestNudgeCapStorage returns a *session.Storage backed by a fresh
// in-memory SQLite database, per this repo's test-isolation convention
// (session.NewTestEntRepository's doc comment).
func newTestNudgeCapStorage(t *testing.T) *session.Storage {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage
}

// TestNudgeCapStore_Get_ShouldReturnZeroValueRecord_WhenItemNeverNudged covers
// Story 3.3.1's AC that Get is not an error path for a never-nudged item.
func TestNudgeCapStore_Get_ShouldReturnZeroValueRecord_WhenItemNeverNudged(t *testing.T) {
	t.Parallel()
	store := NewNudgeCapStore(newTestNudgeCapStorage(t))

	rec, err := store.Get(context.Background(), "never-nudged-item")

	require.NoError(t, err)
	assert.Equal(t, "never-nudged-item", rec.ItemID)
	assert.Equal(t, 0, rec.NudgeCount)
	assert.Nil(t, rec.LastNudgeAt)
}

// TestNudgeCapStore_CheckAndReserve_ShouldCreateRowWithCountOne_WhenItemNeverNudged
// covers Story 3.3.1's first AC example.
func TestNudgeCapStore_CheckAndReserve_ShouldCreateRowWithCountOne_WhenItemNeverNudged(t *testing.T) {
	t.Parallel()
	store := NewNudgeCapStore(newTestNudgeCapStorage(t))
	ctx := context.Background()

	ok, err := store.CheckAndReserve(ctx, "fresh-item", 2, 900*time.Second)
	require.NoError(t, err)
	assert.True(t, ok)

	rec, err := store.Get(ctx, "fresh-item")
	require.NoError(t, err)
	assert.Equal(t, 1, rec.NudgeCount)
	require.NotNil(t, rec.LastNudgeAt)
}

// TestNudgeCapStore_CheckAndReserve_ShouldReturnCapReachedWithoutIncrementing_WhenAlreadyAtCap
// covers Story 3.3.1's second AC example.
func TestNudgeCapStore_CheckAndReserve_ShouldReturnCapReachedWithoutIncrementing_WhenAlreadyAtCap(t *testing.T) {
	t.Parallel()
	store := NewNudgeCapStore(newTestNudgeCapStorage(t))
	ctx := context.Background()
	itemID := "at-cap-item"

	for range 2 {
		ok, err := store.CheckAndReserve(ctx, itemID, 2, 0)
		require.NoError(t, err)
		require.True(t, ok)
	}

	ok, err := store.CheckAndReserve(ctx, itemID, 2, 0)
	require.NoError(t, err)
	assert.False(t, ok)

	rec, err := store.Get(ctx, itemID)
	require.NoError(t, err)
	assert.Equal(t, 2, rec.NudgeCount, "a failed reservation must not increment NudgeCount")
}

// TestNudgeCapStore_CheckAndReserve_ShouldReturnFalseWithoutError_WhenWithinCooldownWindow
// covers the cooldown half of Story 3.3.1/ADR-003, still under cap.
func TestNudgeCapStore_CheckAndReserve_ShouldReturnFalseWithoutError_WhenWithinCooldownWindow(t *testing.T) {
	t.Parallel()
	store := NewNudgeCapStore(newTestNudgeCapStorage(t))
	ctx := context.Background()
	itemID := "cooldown-item"

	ok, err := store.CheckAndReserve(ctx, itemID, 5, time.Hour)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = store.CheckAndReserve(ctx, itemID, 5, time.Hour)
	require.NoError(t, err)
	assert.False(t, ok, "second call within the cooldown window must be blocked even though under cap")

	rec, err := store.Get(ctx, itemID)
	require.NoError(t, err)
	assert.Equal(t, 1, rec.NudgeCount, "a cooldown-blocked call must not increment NudgeCount")
}

// TestNudgeCapStore_CheckAndReserve_ShouldAllowExactlyOneSuccess_WhenTwoGoroutinesRaceForSameItemAtCapOne
// covers Story 3.3.2's AC: run with `go test -race`.
func TestNudgeCapStore_CheckAndReserve_ShouldAllowExactlyOneSuccess_WhenTwoGoroutinesRaceForSameItemAtCapOne(t *testing.T) {
	store := NewNudgeCapStore(newTestNudgeCapStorage(t))
	ctx := context.Background()

	for i := range 100 {
		itemID := fmt.Sprintf("race-item-%d", i)

		var wg sync.WaitGroup
		oks := make([]bool, 2)
		errs := make([]error, 2)
		for g := range 2 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				oks[idx], errs[idx] = store.CheckAndReserve(ctx, itemID, 1, 900*time.Second)
			}(g)
		}
		wg.Wait()

		for _, err := range errs {
			require.NoError(t, err)
		}
		successCount := 0
		for _, ok := range oks {
			if ok {
				successCount++
			}
		}
		assert.Equalf(t, 1, successCount, "iteration %d: expected exactly one success, got %d", i, successCount)
	}
}

// TestNudgeCapGateCheck_ShouldAbortWrite_NeverLogOnly_WhenCheckAndReserveFails
// covers Story 3.3.3: an at-cap reservation must translate directly into
// (false, SafetyGateReasonNudgeCapReached), never a silent/log-only pass.
func TestNudgeCapGateCheck_ShouldAbortWrite_NeverLogOnly_WhenCheckAndReserveFails(t *testing.T) {
	t.Parallel()
	store := NewNudgeCapStore(newTestNudgeCapStorage(t))
	ctx := context.Background()
	itemID := "gate-at-cap-item"

	cfgFn := func() *config.Config {
		return &config.Config{DiagnoseNudge: config.DiagnoseNudgeConfig{MaxNudgesPerItem: 1, CooldownSeconds: 1}}
	}
	check := NewNudgeCapGateCheck(store, cfgFn)

	ok, reason := check(ctx, itemID)
	require.True(t, ok, "first call is under cap and must proceed")
	assert.Equal(t, diagnose.SafetyGateReason(""), reason)

	ok, reason = check(ctx, itemID)
	assert.False(t, ok)
	assert.Equal(t, diagnose.SafetyGateReasonNudgeCapReached, reason)
}

// TestNudgeCapGateCheck_ShouldFailClosed_WhenStoreReturnsError covers this
// implementation's judgment call (nudge_cap_store.go's NewNudgeCapGateCheck
// doc comment): an unexpected store error must abort the write exactly like
// a real cap hit, never propagate as an ambiguous "unknown" state.
func TestNudgeCapGateCheck_ShouldFailClosed_WhenStoreReturnsError(t *testing.T) {
	t.Parallel()
	failingStore := failingNudgeCapStore{}
	cfgFn := func() *config.Config { return &config.Config{} }
	check := NewNudgeCapGateCheck(failingStore, cfgFn)

	ok, reason := check(context.Background(), "any-item")

	assert.False(t, ok)
	assert.Equal(t, diagnose.SafetyGateReasonNudgeCapReached, reason)
}

// failingNudgeCapStore is a NudgeCapStore stub whose CheckAndReserve always
// errors, for TestNudgeCapGateCheck_ShouldFailClosed_WhenStoreReturnsError.
type failingNudgeCapStore struct{}

func (failingNudgeCapStore) Get(context.Context, string) (session.NudgeCapRecordData, error) {
	return session.NudgeCapRecordData{}, fmt.Errorf("stub: not implemented")
}

func (failingNudgeCapStore) CheckAndReserve(context.Context, string, int, time.Duration) (bool, error) {
	return false, fmt.Errorf("stub: simulated store failure")
}
