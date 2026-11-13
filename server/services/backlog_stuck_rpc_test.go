package services

// backlog_stuck_rpc_test.go — tests for the stuck-backlog-item read RPC
// surface added in Phase 3 (Epic 3.1) of the backlog-stuck-item-visibility
// feature: ListStuckBacklogItems, SnoozeStuckItem, and the proto <-> domain
// StuckReason mapping helpers in backlog_service_stuck.go.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// TestFromProtoStuckReason_should_ReturnBounceCapExhausted_When_ProtoBounceCapExhausted
// verifies the inverse mapping for the new synthetic aggregate reason
// (backlog-bounce-escalation, Epic 1.1) recovers the correct domain constant.
func TestFromProtoStuckReason_should_ReturnBounceCapExhausted_When_ProtoBounceCapExhausted(t *testing.T) {
	t.Parallel()
	got := FromProtoStuckReason(sessionv1.StuckReason_STUCK_REASON_BOUNCE_CAP_EXHAUSTED)
	assert.Equal(t, domain.StuckReasonBounceCapExhausted, got)
}

// TestBulkResetStuckRemediation_should_resolveToLastWriterDeterministically_When_RacingAConcurrentColdRetryWrite
// covers AC 5 of the reason-scoped-sweep feature: a reason-scoped bulk reset
// racing a concurrent cold-retry heartbeat write (BUG-083, PR #572) on the
// SAME row. This test intentionally does NOT use goroutines: the ent SQLite
// connection is opened with SetMaxOpenConns(1) (session/ent_repository.go),
// so database/sql fully serializes both writers' Exec calls regardless of
// goroutine scheduling — a goroutine-based version of this test cannot
// exercise real interleaving and would pass identically whether the
// underlying logic is correct or not (caught in review of an earlier draft).
// Instead this proves the actual claim directly: both writers issue a
// single-column literal UPDATE (never read-modify-write), so whichever call
// runs LAST always wins outright — exercised here in both orders — and the
// row is never torn/partial. This is the documented last-write-wins outcome
// (research/pitfalls.md §3), not silent corruption.
func TestBulkResetStuckRemediation_should_resolveToLastWriterDeterministically_When_RacingAConcurrentColdRetryWrite(t *testing.T) {
	t.Parallel()

	t.Run("cold-retry re-park runs last: re-park wins", func(t *testing.T) {
		t.Parallel()
		storage := svctest.CreateTestStorage(t)
		ctx := t.Context()
		reason := domain.StuckReasonBouncing

		item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "racing item", Status: string(domain.BacklogStatusReview)})
		require.NoError(t, err)
		svctest.SeedOpenStuckRow(t, storage, item.ID, reason, time.Now(), "bouncing")
		_, err = storage.RecordRemediationAttempt(ctx, item.ID, reason, session.MaxRemediationAttempts, nil)
		require.NoError(t, err)

		_, err = storage.BulkResetStuckRemediation(ctx, &reason, true)
		require.NoError(t, err)
		// Simulates a cold-retry heartbeat tick landing right after the sweep.
		_, err = storage.RecordRemediationAttempt(ctx, item.ID, reason, session.MaxRemediationAttempts, nil)
		require.NoError(t, err)

		rows, err := storage.FindOpenStuckStates(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 1, "row must still exist exactly once — no torn/duplicate write")
		assert.Equal(t, int32(session.MaxRemediationAttempts), rows[0].RemediationAttempts,
			"the write that ran last (re-park) must win outright, not be partially applied")
	})

	t.Run("bulk reset runs last: reset wins", func(t *testing.T) {
		t.Parallel()
		storage := svctest.CreateTestStorage(t)
		ctx := t.Context()
		reason := domain.StuckReasonBouncing

		item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "racing item", Status: string(domain.BacklogStatusReview)})
		require.NoError(t, err)
		svctest.SeedOpenStuckRow(t, storage, item.ID, reason, time.Now(), "bouncing")
		_, err = storage.RecordRemediationAttempt(ctx, item.ID, reason, session.MaxRemediationAttempts, nil)
		require.NoError(t, err)

		// Simulates a cold-retry heartbeat tick landing right before the sweep.
		_, err = storage.RecordRemediationAttempt(ctx, item.ID, reason, session.MaxRemediationAttempts, nil)
		require.NoError(t, err)
		_, err = storage.BulkResetStuckRemediation(ctx, &reason, true)
		require.NoError(t, err)

		rows, err := storage.FindOpenStuckStates(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 1, "row must still exist exactly once — no torn/duplicate write")
		assert.Equal(t, int32(0), rows[0].RemediationAttempts,
			"the write that ran last (reset) must win outright, not be partially applied")
	})
}
