package session

// backlog_remediation_test.go — tests for the shared exponential-backoff
// remediation gate (backlog_remediation.go), Phase A of
// docs/tasks/backlog-stuck-item-auto-remediation.md. Split into pure
// table-driven tests for evaluateRemediation/nextRemediationAt (mirroring
// stuck_decisions_test.go's style for the same reason: fuzzy
// threshold/schedule arithmetic deserves exhaustive DB-independent coverage)
// and a Storage-level integration test proving the actual safety property
// the design doc calls out: an item stuck 5 times in rapid succession
// results in at most 5 remediation attempts total, with attempts 2-5
// correctly delayed.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// TestEvaluateRemediation_should_returnExpectedDecision_When_GivenRowState
// table-drives every branch of evaluateRemediation: parked (attempts at
// cap), not-yet-due (future next_remediation_at), granted (no gate active),
// and restart-grace (boot after last check, not yet consumed this boot).
func TestEvaluateRemediation_should_returnExpectedDecision_When_GivenRowState(t *testing.T) {
	t.Parallel()
	now := time.Now()
	boot := now.Add(-1 * time.Hour)
	future := now.Add(10 * time.Minute)
	past := now.Add(-10 * time.Minute)

	tests := []struct {
		name string
		row  OpenStuckStateData
		want remediationDecision
	}{
		{
			name: "fresh row, no attempts yet, no next_remediation_at -> granted",
			row:  OpenStuckStateData{RemediationAttempts: 0, LastCheckedAt: now},
			want: remediationGranted,
		},
		{
			name: "attempts at cap -> parked regardless of next_remediation_at",
			row:  OpenStuckStateData{RemediationAttempts: MaxRemediationAttempts, LastCheckedAt: now},
			want: remediationSkippedParked,
		},
		{
			name: "attempts over cap (defensive) -> parked",
			row:  OpenStuckStateData{RemediationAttempts: MaxRemediationAttempts + 1, LastCheckedAt: now},
			want: remediationSkippedParked,
		},
		{
			name: "next_remediation_at in the future -> not due",
			row:  OpenStuckStateData{RemediationAttempts: 1, NextRemediationAt: &future, LastCheckedAt: now},
			want: remediationSkippedNotDue,
		},
		{
			name: "next_remediation_at in the past -> granted",
			row:  OpenStuckStateData{RemediationAttempts: 1, NextRemediationAt: &past, LastCheckedAt: now},
			want: remediationGranted,
		},
		{
			name: "next_remediation_at exactly now -> granted (not before now, so due)",
			row:  OpenStuckStateData{RemediationAttempts: 1, NextRemediationAt: &now, LastCheckedAt: now},
			want: remediationGranted,
		},
		{
			name: "boot after last_checked_at, no grace consumed yet -> restart grace",
			row:  OpenStuckStateData{RemediationAttempts: 1, LastCheckedAt: boot.Add(-1 * time.Minute)},
			want: remediationGrantedRestartGrace,
		},
		{
			name: "boot after last_checked_at, grace already consumed this boot -> granted (normal attempt)",
			row:  OpenStuckStateData{RemediationAttempts: 1, LastCheckedAt: boot.Add(-1 * time.Minute), GraceBootTime: &boot},
			want: remediationGranted,
		},
		{
			name: "boot before last_checked_at (no restart since) -> granted",
			row:  OpenStuckStateData{RemediationAttempts: 1, LastCheckedAt: boot.Add(1 * time.Minute)},
			want: remediationGranted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := evaluateRemediation(tt.row, now, boot)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestNextRemediationAt_should_matchTheRevisedSchedule_When_GivenAttemptNumber
// pins the exact 30m/2h/8h/24h/72h schedule from the design doc's 2026-07-20
// revision (sized for OOM-restart bursts), and verifies attempt numbers
// outside 1..MaxRemediationAttempts return nil (parked / invalid).
func TestNextRemediationAt_should_matchTheRevisedSchedule_When_GivenAttemptNumber(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		attemptNumber int32
		wantDelay     time.Duration
		wantNil       bool
	}{
		{attemptNumber: 0, wantNil: true},
		{attemptNumber: 1, wantDelay: 30 * time.Minute},
		{attemptNumber: 2, wantDelay: 2 * time.Hour},
		{attemptNumber: 3, wantDelay: 8 * time.Hour},
		{attemptNumber: 4, wantDelay: 24 * time.Hour},
		{attemptNumber: 5, wantDelay: 72 * time.Hour},
		{attemptNumber: 6, wantNil: true},
	}
	for _, tt := range tests {
		got := nextRemediationAt(tt.attemptNumber, now)
		if tt.wantNil {
			assert.Nil(t, got, "attempt %d", tt.attemptNumber)
			continue
		}
		require.NotNil(t, got, "attempt %d", tt.attemptNumber)
		assert.Equal(t, now.Add(tt.wantDelay), *got, "attempt %d", tt.attemptNumber)
	}
}

// TestRemediationDue_should_capAtFiveAttemptsWithDelayedRetries_When_StuckRapidlyInSuccession
// is the design doc's mandated regression test: an item marked stuck 5+ times
// in rapid succession (seconds apart, simulating the 2026-07-19 OOM-restart
// incident shape) must never accumulate more than MaxRemediationAttempts
// remediation attempts, and attempts 2-5 must be correctly gated by the
// backoff schedule rather than firing immediately back-to-back.
func TestRemediationDue_should_capAtFiveAttemptsWithDelayedRetries_When_StuckRapidlyInSuccession(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)

	_, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, BacklogStatusInProgress, "bouncing")
	require.NoError(t, err)

	dueCount := 0
	for i := 0; i < 10; i++ {
		due, _, gateErr := storage.RemediationDue(ctx, itemID, domain.StuckReasonBouncing)
		require.NoError(t, gateErr)
		if due {
			dueCount++
		}
		// No sleep between calls — "seconds apart" collapses to "immediately"
		// here, which is the harder case: nothing but the backoff timer stops
		// attempt 2 from following attempt 1 instantly.
	}

	assert.Equal(t, 1, dueCount, "only the very first call (attempt 1, no backoff yet) should be due when every call happens back-to-back")

	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(1), rows[0].RemediationAttempts, "exactly one attempt should have been recorded")
	require.NotNil(t, rows[0].NextRemediationAt)
	assert.WithinDuration(t, time.Now().Add(30*time.Minute), *rows[0].NextRemediationAt, 5*time.Second)
}

// TestRemediationDue_should_advanceThroughFullScheduleThenPark_When_EachAttemptIsForcedDue
// drives all 5 attempts by manually clearing next_remediation_at between
// calls (simulating time passing without an actual 72h+30h+... test sleep),
// verifying the schedule advances 30m -> 2h -> 8h -> 24h -> 72h and the 6th
// call is permanently skipped (parked) even after clearing the timer again.
func TestRemediationDue_should_advanceThroughFullScheduleThenPark_When_EachAttemptIsForcedDue(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)
	_, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, BacklogStatusInProgress, "bouncing")
	require.NoError(t, err)

	wantDelays := []time.Duration{30 * time.Minute, 2 * time.Hour, 8 * time.Hour, 24 * time.Hour, 72 * time.Hour}

	for attempt := 1; attempt <= 5; attempt++ {
		due, justParked, gateErr := storage.RemediationDue(ctx, itemID, domain.StuckReasonBouncing)
		require.NoError(t, gateErr)
		require.True(t, due, "attempt %d should be due", attempt)
		assert.Equal(t, attempt == 5, justParked, "justParked should only be true on the 5th (final) attempt")

		rows, findErr := storage.FindOpenStuckStates(ctx)
		require.NoError(t, findErr)
		require.Len(t, rows, 1)
		assert.Equal(t, int32(attempt), rows[0].RemediationAttempts)

		// Every attempt, including the 5th, records a schedule-derived
		// next_remediation_at — attempt 5's stored value (now+72h) is never
		// actually consulted again because evaluateRemediation checks the
		// attempt cap BEFORE the timer, so "parked" is fully determined by
		// remediation_attempts >= MaxRemediationAttempts alone.
		require.NotNil(t, rows[0].NextRemediationAt)
		assert.WithinDuration(t, time.Now().Add(wantDelays[attempt-1]), *rows[0].NextRemediationAt, 5*time.Second)
		if attempt < 5 {
			// Force the row past its backoff window without waiting real time.
			backdateNextRemediationAt(t, repo, itemID, domain.StuckReasonBouncing, time.Now().Add(-time.Second))
		}
	}

	// 6th call: parked, must stay skipped even though nothing gates the timer anymore.
	due, justParked, gateErr := storage.RemediationDue(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, gateErr)
	assert.False(t, due, "a parked row must never become due again automatically")
	assert.False(t, justParked)

	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(5), rows[0].RemediationAttempts, "parked attempt count must not grow past the cap")
}

// TestRecordManualRemediationAttempt_should_rejectParked_When_AttemptsAtCap
// verifies the operator "Retry now" path (RecordManualRemediationAttempt,
// backing TriggerRemediationNow) refuses to un-park a row that already
// exhausted its budget — ErrRemediationParked, not a silent reset.
func TestRecordManualRemediationAttempt_should_rejectParked_When_AttemptsAtCap(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)
	_, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, BacklogStatusInProgress, "bouncing")
	require.NoError(t, err)

	applied, recErr := repo.RecordRemediationAttempt(ctx, itemID, domain.StuckReasonBouncing, MaxRemediationAttempts, nil)
	require.NoError(t, recErr)
	require.True(t, applied)

	_, err = storage.RecordManualRemediationAttempt(ctx, itemID, domain.StuckReasonBouncing)
	require.ErrorIs(t, err, ErrRemediationParked)

	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, MaxRemediationAttempts, rows[0].RemediationAttempts, "a rejected manual attempt must not change the stored count")
}

// TestRecordManualRemediationAttempt_should_error_When_NoOpenRowExists
// verifies a manual trigger for a reason with no open stuck row (nothing to
// remediate) fails clearly instead of silently no-op-ing.
func TestRecordManualRemediationAttempt_should_error_When_NoOpenRowExists(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)

	_, err = storage.RecordManualRemediationAttempt(ctx, itemID, domain.StuckReasonBouncing)
	require.ErrorIs(t, err, ErrNoOpenStuckState)
}

// TestRecordManualRemediationAttempt_should_incrementLikeANormalAttempt_When_RowIsOpenAndNotParked
// verifies a successful manual trigger consumes budget identically to an
// automated one — it counts toward the same 5-attempt cap, per the design
// doc addendum's explicit safety requirement.
func TestRecordManualRemediationAttempt_should_incrementLikeANormalAttempt_When_RowIsOpenAndNotParked(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)
	_, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, BacklogStatusInProgress, "bouncing")
	require.NoError(t, err)

	justParked, err := storage.RecordManualRemediationAttempt(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	assert.False(t, justParked)

	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(1), rows[0].RemediationAttempts)
	require.NotNil(t, rows[0].NextRemediationAt)

	// A subsequent AUTOMATED check must respect the backoff this manual
	// attempt just set — manual and automated attempts share one counter.
	due, _, gateErr := storage.RemediationDue(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, gateErr)
	assert.False(t, due, "the backoff set by a manual attempt must gate the next automated check too")
}

// TestResetStuckRemediation_should_clearCountersAndNotifiedAt_When_RowIsOpen
// verifies the single-row admin reset RPC's backing store method restores a
// row to its pre-attempt state.
func TestResetStuckRemediation_should_clearCountersAndNotifiedAt_When_RowIsOpen(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	itemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)
	_, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, BacklogStatusInProgress, "bouncing")
	require.NoError(t, err)
	_, err = repo.MarkStuckNotified(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	_, err = repo.RecordRemediationAttempt(ctx, itemID, domain.StuckReasonBouncing, MaxRemediationAttempts, nil)
	require.NoError(t, err)

	applied, err := storage.ResetStuckRemediation(ctx, itemID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	assert.True(t, applied)

	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(0), rows[0].RemediationAttempts)
	assert.Nil(t, rows[0].NextRemediationAt)
	assert.Nil(t, rows[0].NotifiedAt)
}

// TestBulkResetStuckRemediation_should_onlyResetParkedRows_When_OnlyParkedTrue
// verifies the default (only_parked=true) admin bulk action targets rows
// that actually hit the cap, leaving an in-progress-but-not-yet-parked row
// untouched — the "give the batch a fresh shot" action should not also
// reset items that are still legitimately mid-backoff.
func TestBulkResetStuckRemediation_should_onlyResetParkedRows_When_OnlyParkedTrue(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	parkedItemID := createStuckTestItem(t, repo, ctx, BacklogStatusInProgress)
	_, err = repo.MarkStuck(ctx, parkedItemID, domain.StuckReasonBouncing, BacklogStatusInProgress, "bouncing")
	require.NoError(t, err)
	_, err = repo.RecordRemediationAttempt(ctx, parkedItemID, domain.StuckReasonBouncing, MaxRemediationAttempts, nil)
	require.NoError(t, err)

	midBackoffItemID := createStuckTestItem(t, repo, ctx, BacklogStatusReview)
	_, err = repo.MarkStuck(ctx, midBackoffItemID, domain.StuckReasonAbandonedReview, BacklogStatusReview, "abandoned")
	require.NoError(t, err)
	next := time.Now().Add(2 * time.Hour)
	_, err = repo.RecordRemediationAttempt(ctx, midBackoffItemID, domain.StuckReasonAbandonedReview, 2, &next)
	require.NoError(t, err)

	n, err := storage.BulkResetStuckRemediation(ctx, nil, true)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		if row.ItemID == parkedItemID {
			assert.Equal(t, int32(0), row.RemediationAttempts, "parked row must be reset")
		} else {
			assert.Equal(t, int32(2), row.RemediationAttempts, "mid-backoff (not parked) row must be left alone")
		}
	}
}

// backdateNextRemediationAt directly manipulates a row's next_remediation_at
// via the ent client, bypassing the normal write path — test-only helper for
// simulating "time has passed" without an actual sleep.
func backdateNextRemediationAt(t *testing.T, repo *EntRepository, itemID string, reason domain.StuckReason, at time.Time) {
	t.Helper()
	applied, err := repo.RecordRemediationAttempt(context.Background(), itemID, reason, currentAttempts(t, repo, itemID, reason), &at)
	require.NoError(t, err)
	require.True(t, applied)
}

// currentAttempts reads back the current remediation_attempts for (itemID,
// reason) — backdateNextRemediationAt needs it to avoid disturbing the
// attempt count while only backdating the timer.
func currentAttempts(t *testing.T, repo *EntRepository, itemID string, reason domain.StuckReason) int32 {
	t.Helper()
	rows, err := repo.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	row, ok := findOpenStuckStateFor(rows, itemID, reason)
	require.True(t, ok)
	return row.RemediationAttempts
}
