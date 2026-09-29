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

func setupDiagnoseSession(t *testing.T, storage *session.Storage) (itemID, sessionUUID string) {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Diagnose me",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	sessUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessUUID,
		SessionRole: session.SessionRoleDiagnose,
	})
	require.NoError(t, err)

	return item.ID, sessUUID
}

func TestSubmitDiagnosisResult_should_RecordActivityNote_When_CallerIsDiagnoseRole(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupDiagnoseSession(t, storage)
	dh := &diagnoseHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id": itemID,
		"outcome": "BUG_FILED",
		"summary": "Found a tmux session collision, filed as X.",
	})

	result, err := dh.submitDiagnosisResult(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	notes, err := storage.ListActivityNotesForItem(context.Background(), itemID)
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0].Message, "BUG_FILED")
	assert.Contains(t, notes[0].Message, "tmux session collision")
}

func TestSubmitDiagnosisResult_should_RejectWrongRole_When_CallerIsWorkSession(t *testing.T) {
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupReviewSession(t, storage) // review role, not diagnose
	dh := &diagnoseHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessUUID)

	req := makeToolReq(map[string]interface{}{
		"item_id": itemID,
		"outcome": "NOTE_ONLY",
		"summary": "inconclusive",
	})

	result, err := dh.submitDiagnosisResult(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool), "a review-role session must not be able to submit a diagnosis result")
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrPermissionDenied, errObj["code"])
}

// TestNudgeSession_should_Refuse_When_NudgeCapAlreadyReached is AC4's direct
// regression guard at the MCP tool boundary: even if a caller ignores the
// dispatch prompt's instructions, the tool itself refuses once the cap is hit.
func TestNudgeSession_should_Refuse_When_NudgeCapAlreadyReached(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.DiagnoseNudgeFeatureFlag, true))
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	itemID, sessUUID := setupDiagnoseSession(t, storage)
	ctx := context.Background()

	applied, err := repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, session.BacklogStatusReview, "bouncing")
	require.NoError(t, err)
	require.True(t, applied, "MarkStuck's expectedStatus must match setupDiagnoseSession's item status (review)")
	const maxAttempts = int32(3)
	_, err = repo.RecordDiagnoseNudgeAttempt(ctx, itemID, domain.StuckReasonBouncing, maxAttempts, nil)
	require.NoError(t, err)

	dh := &diagnoseHandlers{storage: storage}
	reqCtx := WithSessionUUID(ctx, sessUUID)
	req := makeToolReq(map[string]interface{}{
		"item_id":      itemID,
		"session_id":   uuid.New().String(),
		"stuck_reason": string(domain.StuckReasonBouncing),
		"message":      "please continue",
	})

	result, err := dh.nudgeSession(reqCtx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool), "nudge must be refused once the cap is reached")
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrPermissionDenied, errObj["code"])
}

// TestNudgeSession_should_Refuse_When_FeatureFlagDisabled is AC3's direct
// regression guard at the MCP tool boundary (gap #3, config.DiagnoseNudgeFeatureFlag):
// with the kill switch left at its default (off), diagnose_nudge_session must
// refuse the write before it ever reaches the cap/cooldown or live-target
// checks — proven here by NOT wiring a live target at all (dh.live is nil,
// which the target-not-live path would also reject with a different code) and
// asserting on the specific disabled-flag message, not just the shared
// PERMISSION_DENIED code.
func TestNudgeSession_should_Refuse_When_FeatureFlagDisabled(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupDiagnoseSession(t, storage)

	dh := &diagnoseHandlers{storage: storage, live: nil}
	ctx := WithSessionUUID(context.Background(), sessUUID)
	req := makeToolReq(map[string]interface{}{
		"item_id":      itemID,
		"session_id":   uuid.New().String(),
		"stuck_reason": string(domain.StuckReasonBouncing),
		"message":      "please continue",
	})

	result, err := dh.nudgeSession(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool), "nudge must be refused while diagnose_nudge_enabled is off")
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrPermissionDenied, errObj["code"])
	assert.Equal(t, diagnoseNudgeDisabledMessage, errObj["message"], "must be refused specifically by the kill switch, not the (also-true) not-live path")
}

func TestNudgeSession_should_Refuse_When_TargetSessionNotLive(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.DiagnoseNudgeFeatureFlag, true))
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupDiagnoseSession(t, storage)

	dh := &diagnoseHandlers{storage: storage, live: nil}
	ctx := WithSessionUUID(context.Background(), sessUUID)
	req := makeToolReq(map[string]interface{}{
		"item_id":      itemID,
		"session_id":   uuid.New().String(),
		"stuck_reason": string(domain.StuckReasonBouncing),
		"message":      "please continue",
	})

	result, err := dh.nudgeSession(ctx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool), "nudge must be refused with no live-instance lookup available")
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrSessionNotFound, errObj["code"])
}
