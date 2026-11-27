package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/external"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
)

func listCheckpointCount(t *testing.T, svc *SessionService, id string) (int, error) {
	t.Helper()
	res, err := svc.ListCheckpoints(context.Background(), connect.NewRequest(&sessionv1.ListCheckpointsRequest{SessionId: id}))
	if err != nil {
		return 0, err
	}
	return len(res.Msg.Checkpoints), nil
}

func TestSessionService_WiresReviewQueueApprovalStore(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	require.NoError(t, fix.svc.GetApprovalStore().Create(&approvalkit.PendingApproval{
		ID: "approval-1", SessionID: "waiting", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}))
	fix.svc.GetReviewQueueInstance().Add(&queue.ReviewItem{SessionID: "waiting", Reason: queue.ReasonApprovalPending})

	res, err := fix.svc.GetReviewQueue(context.Background(), connect.NewRequest(&sessionv1.GetReviewQueueRequest{}))

	require.NoError(t, err)
	require.Len(t, res.Msg.ReviewQueue.Items, 1)
	assert.Equal(t, "approval-1", res.Msg.ReviewQueue.Items[0].Metadata["pending_approval_id"])
}

func TestSessionService_ForwardsPollerToReviewCheckpointAndWorkflow(t *testing.T) {
	t.Parallel()
	sessSvc, _ := createTestWorkflowDelegation(t)
	inst, _ := makeInstanceWithCheckpoint("polled")
	inst.WorkflowID = uuid.NewString()
	inst.Status = domain.Stopped
	poller := session.NewReviewQueuePoller(sessSvc.GetReviewQueueInstance(), instance.NewInstanceStatusManager(), nil)
	poller.SetInstances([]*instance.Instance{inst})
	sessSvc.SetReviewQueuePoller(poller)
	ctx := context.Background()

	n, err := listCheckpointCount(t, sessSvc, "polled")
	require.NoError(t, err, "checkpoint poller")
	assert.Equal(t, 1, n)

	_, err = sessSvc.AcknowledgeSession(ctx, connect.NewRequest(&sessionv1.AcknowledgeSessionRequest{Id: "polled"}))
	require.NoError(t, err)
	assert.False(t, inst.Snapshot().LastAcknowledged.IsZero(), "review queue poller")

	_, err = sessSvc.ArchiveWorkflowSessions(ctx, connect.NewRequest(&sessionv1.ArchiveWorkflowSessionsRequest{WorkflowId: inst.WorkflowID}))
	require.NoError(t, err)
	assert.True(t, inst.IsArchived(), "workflow poller")
}

func TestSessionService_WiresCheckpointExternalDiscoveryAndInstanceLoader(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	ctx := context.Background()

	discovery := external.NewExternalSessionDiscovery()
	inst, _ := makeInstanceWithCheckpoint("external")
	discovery.AddSessionForTest("external", inst)
	fix.svc.SetExternalDiscovery(discovery)
	n, err := listCheckpointCount(t, fix.svc, "external")
	require.NoError(t, err, "checkpoint external discovery")
	assert.Equal(t, 1, n)

	stored := &instance.Instance{Title: "stored-only", Path: t.TempDir(), Status: domain.Paused, Program: "claude"}
	require.NoError(t, fix.storage.AddInstance(stored))
	_, err = fix.svc.ClearConversationState(ctx, connect.NewRequest(&sessionv1.ClearConversationStateRequest{Id: "stored-only"}))
	require.NoError(t, err, "checkpoint instance loader")
}
