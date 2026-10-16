package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// TestEvaluateDiagnoseNudgeAllowance_TableDriven mirrors
// TestEvaluateRemediation_should_returnExpectedDecision_When_GivenRowState's
// style: exhaustive, DB-independent coverage of the pure decision function.
func TestEvaluateDiagnoseNudgeAllowance_TableDriven(t *testing.T) {
	t.Parallel()
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	tests := []struct {
		name           string
		nudgeCount     int32
		nextEligibleAt *time.Time
		maxAttempts    int32
		wantAllowed    bool
	}{
		{"no history", 0, nil, 3, true},
		{"below cap, no cooldown", 1, nil, 3, true},
		{"below cap, cooldown elapsed", 1, &past, 3, true},
		{"below cap, cooldown active", 1, &future, 3, false},
		{"at cap", 3, nil, 3, false},
		{"above cap", 4, nil, 3, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, reason := EvaluateDiagnoseNudgeAllowance(tt.nudgeCount, tt.nextEligibleAt, tt.maxAttempts, now)
			assert.Equal(t, tt.wantAllowed, allowed)
			if !tt.wantAllowed {
				assert.NotEmpty(t, reason, "a disallowed nudge must carry a human-readable reason for the dispatched agent's prompt")
			} else {
				assert.Empty(t, reason)
			}
		})
	}
}

// newDiagnoseNudgeTestFixture spins up a Storage (and its backing
// *EntRepository, for tests that need to seed state the Storage-level API
// doesn't expose directly, like a cooldown-free count bump) with one open
// BacklogStuckState row (reason: bouncing) for a fresh backlog item — the
// shared setup every test in this file needs before exercising the
// DiagnoseNudgeAllowed/RecordDiagnoseNudgeAttempt gate.
func newDiagnoseNudgeTestFixture(t *testing.T) (storage *Storage, repo *EntRepository, itemID string, ctx context.Context) {
	t.Helper()
	repo, cleanup := createTestEntRepository(t)
	t.Cleanup(cleanup)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx = context.Background()

	itemID = createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)
	_, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, BacklogStatusInProgress, "bouncing")
	require.NoError(t, err)
	return storage, repo, itemID, ctx
}

// TestNudgeCap_StopsAfterMaxAttempts drives diagnose_nudge_count to the
// configured max and asserts DiagnoseNudgeAllowed refuses further nudges
// (AC4, validation.md). Seeds the first maxAttempts-1 attempts directly via
// the repo with no cooldown set, isolating the cap check from the cooldown
// check TestNudgeCooldown_BlocksBeforeWindowElapses covers separately — a
// real caller would still hit the cooldown between nudges, but this test's
// job is to prove the cap fires at all once cooldown is out of the way.
func TestNudgeCap_StopsAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	storage, repo, itemID, ctx := newDiagnoseNudgeTestFixture(t)

	maxAttempts := int32(3)
	_, err := repo.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing, maxAttempts-1, nil)
	require.NoError(t, err)

	allowed, reason, err := storage.DiagnoseNudgeAllowed(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	require.True(t, allowed, "the attempt just under the cap should still be allowed: %s", reason)

	justCapped, err := storage.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	assert.True(t, justCapped, "the attempt that reaches the cap should report justCapped")

	allowed, reason, err = storage.DiagnoseNudgeAllowed(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	assert.False(t, allowed, "nudge must be refused once the cap is reached")
	assert.NotEmpty(t, reason)
}

// TestNudgeCooldown_BlocksBeforeWindowElapses verifies a recorded nudge
// blocks the next one until diagnoseNudgeCooldown elapses, even well under
// the attempt cap.
func TestNudgeCooldown_BlocksBeforeWindowElapses(t *testing.T) {
	t.Parallel()
	storage, _, itemID, ctx := newDiagnoseNudgeTestFixture(t)

	_, err := storage.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)

	allowed, reason, err := storage.DiagnoseNudgeAllowed(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	assert.False(t, allowed, "the cooldown window must block a second nudge immediately after the first")
	assert.NotEmpty(t, reason)

	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(1), rows[0].DiagnoseNudgeCount)
	require.NotNil(t, rows[0].DiagnoseNextEligibleAt)
}

// TestDiagnoseNudgeAllowed_NoOpenRow_IsUngated verifies an item with no
// history for the given reason is not gated — mirrors RemediationDue's
// identical "nothing to gate against yet" default.
func TestDiagnoseNudgeAllowed_NoOpenRow_IsUngated(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)

	allowed, reason, err := storage.DiagnoseNudgeAllowed(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Empty(t, reason)
}

// runConcurrentClaims fires n claims released together by a start barrier and
// returns how many were granted.
func runConcurrentClaims(t *testing.T, storage *Storage, itemID string, n int, maxAttempts int32, reserve int, cooldown time.Duration) int {
	t.Helper()
	var (
		start   = make(chan struct{})
		wg      sync.WaitGroup
		results = make([]ClaimResult, n)
		errs    = make([]error, n)
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx], errs[idx] = storage.claimDiagnoseNudgeSlot(context.Background(), itemID, domain.StuckReasonBouncing, maxAttempts, reserve, cooldown)
		}(i)
	}
	close(start)
	wg.Wait()

	claimed := 0
	for i := range results {
		require.NoError(t, errs[i])
		if results[i].Claimed {
			claimed++
		}
	}
	return claimed
}

// CLM-1: cap check and increment are one UPDATE, so 8 racing claimers at
// cap 3 / reserve 1 take exactly the 2 claimable slots. The claim's own
// cooldown is disabled here (cooldown 0) so only the cap is under test.
func TestClaimDiagnoseNudgeSlot_ConcurrentCap(t *testing.T) {
	t.Parallel()
	storage, _, itemID, ctx := newDiagnoseNudgeTestFixture(t)

	claimed := runConcurrentClaims(t, storage, itemID, 8, 3, 1, 0)

	assert.Equal(t, 2, claimed)
	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(2), rows[0].DiagnoseNudgeCount)
}

// With the production 1h cooldown the first winner's UPDATE blocks every
// other concurrent claimer, so only one slot is taken per hour.
func TestClaimDiagnoseNudgeSlot_ConcurrentWithCooldownTakesOne(t *testing.T) {
	t.Parallel()
	storage, _, itemID, ctx := newDiagnoseNudgeTestFixture(t)

	claimed := runConcurrentClaims(t, storage, itemID, 8, 3, 1, diagnoseNudgeCooldown)

	assert.Equal(t, 1, claimed)
	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(1), rows[0].DiagnoseNudgeCount)
	require.NotNil(t, rows[0].DiagnoseNextEligibleAt)
}

// CLM-2: the cooldown is part of the claiming UPDATE.
func TestClaimDiagnoseNudgeSlot_CooldownInSameUpdate(t *testing.T) {
	t.Parallel()
	storage, repo, itemID, ctx := newDiagnoseNudgeTestFixture(t)
	future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)

	_, err := repo.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing, 0, &future)
	require.NoError(t, err)
	res, err := storage.ClaimDiagnoseNudgeSlot(ctx, itemID, domain.StuckReasonBouncing, 1)
	require.NoError(t, err)
	assert.False(t, res.Claimed, "future cooldown blocks a claim with count 0")

	_, err = repo.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing, 0, &past)
	require.NoError(t, err)
	res, err = storage.ClaimDiagnoseNudgeSlot(ctx, itemID, domain.StuckReasonBouncing, 1)
	require.NoError(t, err)
	assert.True(t, res.Claimed, "elapsed cooldown is claimable")

	_, err = repo.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing, 0, nil)
	require.NoError(t, err)
	res, err = storage.ClaimDiagnoseNudgeSlot(ctx, itemID, domain.StuckReasonBouncing, 1)
	require.NoError(t, err)
	assert.True(t, res.Claimed, "nil cooldown is claimable")
}

// CLM-3: nothing to claim against fails closed with no error.
func TestClaimDiagnoseNudgeSlot_NoOpenRowFailsClosed(t *testing.T) {
	t.Parallel()
	storage, _, itemID, ctx := newDiagnoseNudgeTestFixture(t)

	res, err := storage.ClaimDiagnoseNudgeSlot(ctx, itemID, domain.StuckReasonAutonomousStuck, 1)

	require.NoError(t, err)
	assert.Equal(t, ClaimResult{}, res)
}

// CLM-4: only the claim that takes the last claimable slot reports JustCapped.
func TestClaimDiagnoseNudgeSlot_JustCappedOnLastSlot(t *testing.T) {
	t.Parallel()
	storage, _, itemID, ctx := newDiagnoseNudgeTestFixture(t)

	res, err := storage.claimDiagnoseNudgeSlot(ctx, itemID, domain.StuckReasonBouncing, 3, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, ClaimResult{Claimed: true}, res)

	res, err = storage.claimDiagnoseNudgeSlot(ctx, itemID, domain.StuckReasonBouncing, 3, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, ClaimResult{Claimed: true, JustCapped: true}, res)

	res, err = storage.claimDiagnoseNudgeSlot(ctx, itemID, domain.StuckReasonBouncing, 3, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, ClaimResult{}, res, "the reserved slot is never claimable")

	// The reserved slot stays available to the MCP path.
	justCapped, err := storage.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	assert.True(t, justCapped)
}

// CLM-6
func TestDiagnoseNudgeAllowedWithReserve_Table(t *testing.T) {
	t.Parallel()
	storage, repo, itemID, ctx := newDiagnoseNudgeTestFixture(t)

	tests := []struct {
		count   int32
		reserve int
		want    bool
	}{
		{0, 1, true},
		{1, 1, true},
		{2, 1, false},
		{2, 0, true},
		{3, 0, false},
	}
	for _, tt := range tests {
		_, err := repo.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing, tt.count, nil)
		require.NoError(t, err)
		got, reason, err := storage.DiagnoseNudgeAllowedWithReserve(ctx, itemID, domain.StuckReasonBouncing, tt.reserve)
		require.NoError(t, err)
		assert.Equal(t, tt.want, got, "count=%d reserve=%d", tt.count, tt.reserve)
		if !got {
			assert.NotEmpty(t, reason)
		}
		base, _, err := storage.DiagnoseNudgeAllowed(ctx, itemID, domain.StuckReasonBouncing)
		require.NoError(t, err)
		if tt.reserve == 0 {
			assert.Equal(t, base, got, "reserve 0 equals DiagnoseNudgeAllowed")
		}
	}

	allowed, _, err := storage.DiagnoseNudgeAllowedWithReserve(ctx, itemID, domain.StuckReasonAutonomousStuck, 1)
	require.NoError(t, err)
	assert.True(t, allowed, "no open row: documented fail-open, the claim is authoritative")
}
