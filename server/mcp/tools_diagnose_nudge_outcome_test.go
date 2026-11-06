package mcp

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestNudgeSession_should_CountSkippedCap_When_NudgeCapAlreadyReached(t *testing.T) {
	rec := session.NewNudgeOutcomeRecorder("", nil)
	defer session.SwapNudgeOutcomeRecorder(rec)()
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.DiagnoseNudgeFeatureFlag, true))
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	itemID, sessUUID := setupDiagnoseSession(t, storage)
	ctx := context.Background()
	_, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, session.BacklogStatusReview, "bouncing")
	require.NoError(t, err)
	_, err = repo.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing, int32(3), nil)
	require.NoError(t, err)

	dh := &diagnoseHandlers{storage: storage}
	_, err = dh.nudgeSession(WithSessionUUID(ctx, sessUUID), makeToolReq(map[string]interface{}{
		"item_id":      itemID,
		"session_id":   uuid.New().String(),
		"stuck_reason": string(domain.StuckReasonBouncing),
		"message":      "please continue",
	}))
	require.NoError(t, err)

	totals := rec.Snapshot().Totals
	require.Len(t, totals, 1)
	assert.Equal(t, session.NudgeKindDiagnose, totals[0].Kind)
	assert.Equal(t, session.NudgeOutcomeSkippedCap, totals[0].Outcome)
	assert.Equal(t, int64(1), totals[0].Value)
}
