package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/stagecfg"
)

// createTestWorkflowDelegation wires a SessionService to a WorkflowService over
// an in-memory SQLite ent client.
func createTestWorkflowDelegation(t *testing.T) (*SessionService, *WorkflowService) {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	sessSvc := NewSessionService(storage, events.NewEventBus(100))
	t.Cleanup(func() { sessSvc.Shutdown() })

	workflowRepo := stagecfg.NewEntWorkflowRepository(session.NewTestEntRepository(t).GetEntClient())
	workflowSvc := NewWorkflowService(workflowRepo, nil /* scheduler nil OK for CRUD tests */, storage)
	sessSvc.SetWorkflowService(workflowSvc)
	return sessSvc, workflowSvc
}

// TestSessionService_DelegatesListWorkflows verifies the delegation from SessionService.
func TestSessionService_DelegatesListWorkflows(t *testing.T) {
	t.Parallel()
	sessSvc, workflowSvc := createTestWorkflowDelegation(t)
	ctx := context.Background()

	// Create a workflow first so the list is non-empty.
	_, err := workflowSvc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "delegate-wf",
		Name:            "Delegate Workflow",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	}))
	require.NoError(t, err)

	resp, err := sessSvc.ListWorkflows(ctx, connect.NewRequest(&sessionv1.ListWorkflowsRequest{}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Workflows, 1)
}

// TestListTriggerFireEvents_ReturnsEventsForWorkflow verifies Task 1.2.1d's minimal
// query-only RPC round-trips TriggerFireEvent rows recorded against a workflow.
func TestListTriggerFireEvents_ReturnsEventsForWorkflow(t *testing.T) {
	t.Parallel()
	sessSvc, workflowSvc := createTestWorkflowDelegation(t)
	ctx := context.Background()

	createResp, err := workflowSvc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "fire-events-wf",
		Name:            "Fire Events",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Workflow.Id
	workflowID, err := uuid.Parse(id)
	require.NoError(t, err)

	// Fire events are tracked in their own repository (no FK to Workflow), so a
	// separate ent client is fine here — only workflow_id needs to match by value.
	entRepo := session.NewTestEntRepository(t)
	fireEventRepo := session.NewEntTriggerFireEventRepository(entRepo.GetEntClient())
	require.NoError(t, fireEventRepo.Create(ctx, session.TriggerFireEventInput{
		WorkflowID:   &workflowID,
		Outcome:      "fired_failed",
		ErrorMessage: "WIP limit reached",
	}))
	workflowSvc.SetTriggerFireEventRepo(fireEventRepo)

	resp, err := sessSvc.ListTriggerFireEvents(ctx, connect.NewRequest(&sessionv1.ListTriggerFireEventsRequest{
		WorkflowId: id,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Events, 1)
	assert.Equal(t, "fired_failed", resp.Msg.Events[0].Outcome)
	assert.Equal(t, "WIP limit reached", resp.Msg.Events[0].ErrorMessage)
	assert.Equal(t, id, resp.Msg.Events[0].WorkflowId)
}

// TestListTriggerFireEvents_NilRepo_ReturnsEmptyList verifies the nil-degradation
// convention matching this file's other RPCs (e.g. ListWorkflows with a nil repo).
func TestListTriggerFireEvents_NilRepo_ReturnsEmptyList(t *testing.T) {
	t.Parallel()
	sessSvc, _ := createTestWorkflowDelegation(t)
	ctx := context.Background()

	resp, err := sessSvc.ListTriggerFireEvents(ctx, connect.NewRequest(&sessionv1.ListTriggerFireEventsRequest{
		WorkflowId: uuid.New().String(),
	}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Events)
}
