package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// fakeDiagnosticSpawner is a DiagnosticSpawner test double recording the
// item/prompt each call was made with, mirroring fakeTriageRespawner's shape.
type fakeDiagnosticSpawner struct {
	lastItemID string
	lastPrompt string
	instUUID   string
}

func (f *fakeDiagnosticSpawner) SpawnDiagnosticSession(_ context.Context, item *session.BacklogItemData, prompt string) (*session.Instance, error) {
	f.lastItemID = item.ID
	f.lastPrompt = prompt
	inst := &session.Instance{UUID: f.instUUID}
	return inst, nil
}

func TestAssembleDiagnosticBundle_should_IncludeItemContent_When_ItemExists(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:       "Investigate the flaky thing",
		Description: "It flakes sometimes.",
		Status:      string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	svc := NewDiagnosticService(storage, nil)
	resp, err := svc.AssembleDiagnosticBundle(ctx, connect.NewRequest(&sessionv1.AssembleDiagnosticBundleRequest{ItemId: item.ID}))
	require.NoError(t, err)
	assert.Contains(t, resp.Msg.Prompt, "Investigate the flaky thing")
	assert.Contains(t, resp.Msg.Prompt, "It flakes sometimes")
	assert.False(t, resp.Msg.Compacted)
	assert.Greater(t, resp.Msg.EstimatedTokens, int32(0))
}

func TestAssembleDiagnosticBundle_should_Error_When_ItemDoesNotExist(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewDiagnosticService(storage, nil)

	_, err := svc.AssembleDiagnosticBundle(context.Background(), connect.NewRequest(&sessionv1.AssembleDiagnosticBundleRequest{ItemId: "00000000-0000-0000-0000-000000000001"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// TestDispatchDiagnose_should_LinkItemSession_When_DispatchSucceeds covers
// AC0's core dispatch path end to end at the service layer (no live tmux/MCP
// involved — the spawner is faked).
func TestDispatchDiagnose_should_LinkItemSession_When_DispatchSucceeds(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Investigate",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	spawner := &fakeDiagnosticSpawner{instUUID: "diag-sess-1"}
	svc := NewDiagnosticService(storage, spawner)

	resp, err := svc.DispatchDiagnose(ctx, connect.NewRequest(&sessionv1.DispatchDiagnoseRequest{ItemId: item.ID}))
	require.NoError(t, err)
	assert.Equal(t, "diag-sess-1", resp.Msg.DiagnosticSessionUuid)
	assert.False(t, resp.Msg.NudgeAllowed, "no nudge target/reason was supplied, so nudge must not be offered")
	assert.Equal(t, item.ID, spawner.lastItemID)
	assert.Contains(t, spawner.lastPrompt, "submit_diagnosis_result")

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, session.SessionRoleDiagnose, sessions[0].Role)
	assert.Equal(t, "diag-sess-1", sessions[0].SessionUUID)
}

func TestDispatchDiagnose_should_Error_When_SpawnerNotConfigured(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewDiagnosticService(storage, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{Title: "x", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	_, err = svc.DispatchDiagnose(context.Background(), connect.NewRequest(&sessionv1.DispatchDiagnoseRequest{ItemId: item.ID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestEvaluateNudgeEligibility_should_DisallowNudge_When_NoTargetSpecified(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewDiagnosticService(storage, nil)

	allowed, reason := svc.evaluateNudgeEligibility(context.Background(), "item-1", sessionv1.StuckReason_STUCK_REASON_BOUNCING, "")
	assert.False(t, allowed)
	assert.NotEmpty(t, reason)
}

func TestEvaluateNudgeEligibility_should_DisallowNudge_When_NoStuckReasonSpecified(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewDiagnosticService(storage, nil)

	allowed, reason := svc.evaluateNudgeEligibility(context.Background(), "item-1", sessionv1.StuckReason_STUCK_REASON_UNSPECIFIED, "sess-1")
	assert.False(t, allowed)
	assert.NotEmpty(t, reason)
}

func TestEvaluateNudgeEligibility_should_DisallowNudge_When_TargetNotLive(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewDiagnosticService(storage, nil) // no poller/extDiscovery wired

	allowed, reason := svc.evaluateNudgeEligibility(context.Background(), "item-1", sessionv1.StuckReason_STUCK_REASON_BOUNCING, "sess-1")
	assert.False(t, allowed)
	assert.Contains(t, reason, "not found")
}
