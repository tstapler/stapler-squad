package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// createTestEntClient opens an in-process SQLite database with full migrations.
func createTestEntClient(t *testing.T) *session.EntRepository {
	t.Helper()
	testDir := t.TempDir()
	repo, err := session.NewEntRepository(session.WithDatabasePath(testDir + "/workflow_test.db"))
	require.NoError(t, err, "createTestEntClient: failed to open database")
	t.Cleanup(func() { repo.Close() })
	return repo
}

// createTestWorkflowService wires up a SessionService + WorkflowService with
// an in-memory SQLite ent client.
func createTestWorkflowService(t *testing.T) (*SessionService, *WorkflowService) {
	t.Helper()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	sessSvc := NewSessionService(storage, eventBus)

	repo := createTestEntClient(t)
	workflowRepo := session.NewEntWorkflowRepository(repo.GetEntClient())
	workflowSvc := NewWorkflowService(workflowRepo, nil /* scheduler nil OK for CRUD tests */)
	sessSvc.SetWorkflowService(workflowSvc)

	return sessSvc, workflowSvc
}

// TestCreateWorkflow_HappyPath checks that a valid workflow is persisted.
func TestCreateWorkflow_HappyPath(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	req := connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "test-wf",
		Name:            "Test Workflow",
		Command:         "do something",
		TargetDirectory: "/tmp/test",
	})
	resp, err := svc.CreateWorkflow(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Workflow)
	assert.Equal(t, "test-wf", resp.Msg.Workflow.Slug)
	assert.Equal(t, "Test Workflow", resp.Msg.Workflow.Name)
	assert.NotEmpty(t, resp.Msg.Workflow.Id)
}

// TestCreateWorkflow_InvalidSlug verifies that a slug with uppercase is rejected.
func TestCreateWorkflow_InvalidSlug(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	req := connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "Bad-Slug",
		Name:            "Test",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	})
	_, err := svc.CreateWorkflow(ctx, req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestCreateWorkflow_DuplicateSlug verifies AlreadyExists is returned.
func TestCreateWorkflow_DuplicateSlug(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	req := connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "dup-slug",
		Name:            "First",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	})
	_, err := svc.CreateWorkflow(ctx, req)
	require.NoError(t, err)

	req2 := connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "dup-slug",
		Name:            "Second",
		Command:         "cmd2",
		TargetDirectory: "/tmp/test",
	})
	_, err2 := svc.CreateWorkflow(ctx, req2)
	require.Error(t, err2)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err2))
}

// TestListWorkflows verifies workflows are listed after creation.
func TestListWorkflows(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	// Create two workflows.
	for _, slug := range []string{"wf-one", "wf-two"} {
		_, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
			Slug:            slug,
			Name:            slug,
			Command:         "cmd",
			TargetDirectory: "/tmp/test",
		}))
		require.NoError(t, err)
	}

	resp, err := svc.ListWorkflows(ctx, connect.NewRequest(&sessionv1.ListWorkflowsRequest{}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Workflows, 2)
}

// TestDeleteWorkflow verifies a workflow is removed.
func TestDeleteWorkflow(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "delete-me",
		Name:            "Delete Me",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Workflow.Id

	_, err = svc.DeleteWorkflow(ctx, connect.NewRequest(&sessionv1.DeleteWorkflowRequest{Id: id}))
	require.NoError(t, err)

	listResp, err := svc.ListWorkflows(ctx, connect.NewRequest(&sessionv1.ListWorkflowsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, listResp.Msg.Workflows)
}

// TestUpdateWorkflow verifies that a workflow's name is updated.
func TestUpdateWorkflow(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "update-me",
		Name:            "Original",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Workflow.Id

	updateResp, err := svc.UpdateWorkflow(ctx, connect.NewRequest(&sessionv1.UpdateWorkflowRequest{
		Id:   id,
		Name: proto.String("Updated"),
	}))
	require.NoError(t, err)
	assert.Equal(t, "Updated", updateResp.Msg.Workflow.Name)
}

// TestCreateWorkflow_MissingCommand verifies validation catches empty command.
func TestCreateWorkflow_MissingCommand(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	req := connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "no-cmd",
		Name:            "No Command",
		Command:         "", // missing
		TargetDirectory: "/tmp/test",
	})
	_, err := svc.CreateWorkflow(ctx, req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestSessionService_DelegatesListWorkflows verifies the delegation from SessionService.
func TestSessionService_DelegatesListWorkflows(t *testing.T) {
	sessSvc, _ := createTestWorkflowService(t)
	ctx := context.Background()

	resp, err := sessSvc.ListWorkflows(ctx, connect.NewRequest(&sessionv1.ListWorkflowsRequest{}))
	require.NoError(t, err)
	assert.NotNil(t, resp.Msg.Workflows)
}
