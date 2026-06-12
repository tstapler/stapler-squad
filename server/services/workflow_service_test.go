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
	"github.com/tstapler/stapler-squad/session/ent"
)

// mockScheduler is a test double for WorkflowSchedulerInterface.
type mockScheduler struct {
	reloadCalled  bool
	removeCalled  bool
	fireNowCalled bool
	reloadErr     error
}

func (m *mockScheduler) Reload(_ context.Context, _ *ent.Workflow) error {
	m.reloadCalled = true
	return m.reloadErr
}

func (m *mockScheduler) Remove(_ string) error {
	m.removeCalled = true
	return nil
}

func (m *mockScheduler) FireNow(_ context.Context, _ *ent.Workflow, _ string) (string, error) {
	m.fireNowCalled = true
	return "test-session-id", nil
}

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
	sessSvc, workflowSvc := createTestWorkflowService(t)
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

// TestCreateWorkflow_WithCronEnabled_CallsReload verifies that creating a cron-enabled
// workflow invokes Reload on the scheduler.
func TestCreateWorkflow_WithCronEnabled_CallsReload(t *testing.T) {
	_, workflowSvc := createTestWorkflowService(t)
	ctx := context.Background()

	mock := &mockScheduler{}
	workflowSvc.scheduler = mock

	_, err := workflowSvc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "cron-wf",
		Name:            "Cron Workflow",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		CronExpression:  "0 9 * * 1",
		CronEnabled:     true,
	}))
	require.NoError(t, err)
	assert.True(t, mock.reloadCalled, "Reload should have been called after cron-enabled create")
}

// TestDeleteWorkflow_CallsSchedulerRemove verifies that deleting a workflow
// invokes Remove on the scheduler.
func TestDeleteWorkflow_CallsSchedulerRemove(t *testing.T) {
	_, workflowSvc := createTestWorkflowService(t)
	ctx := context.Background()

	mock := &mockScheduler{}
	workflowSvc.scheduler = mock

	createResp, err := workflowSvc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "remove-me",
		Name:            "Remove Me",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Workflow.Id

	_, err = workflowSvc.DeleteWorkflow(ctx, connect.NewRequest(&sessionv1.DeleteWorkflowRequest{Id: id}))
	require.NoError(t, err)
	assert.True(t, mock.removeCalled, "Remove should have been called after delete")
}

// TestRunWorkflow_HappyPath verifies that RunWorkflow returns the session ID from FireNow.
func TestRunWorkflow_HappyPath(t *testing.T) {
	_, workflowSvc := createTestWorkflowService(t)
	ctx := context.Background()

	mock := &mockScheduler{}
	workflowSvc.scheduler = mock

	createResp, err := workflowSvc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "run-me",
		Name:            "Run Me",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Workflow.Id

	runResp, err := workflowSvc.RunWorkflow(ctx, connect.NewRequest(&sessionv1.RunWorkflowRequest{
		Id:  id,
		Arg: "some-arg",
	}))
	require.NoError(t, err)
	assert.Equal(t, "test-session-id", runResp.Msg.SessionId)
	assert.True(t, mock.fireNowCalled, "FireNow should have been called")
}

// TestRunWorkflow_NotFound verifies that RunWorkflow returns CodeNotFound for unknown IDs.
func TestRunWorkflow_NotFound(t *testing.T) {
	_, workflowSvc := createTestWorkflowService(t)
	ctx := context.Background()

	mock := &mockScheduler{}
	workflowSvc.scheduler = mock

	_, err := workflowSvc.RunWorkflow(ctx, connect.NewRequest(&sessionv1.RunWorkflowRequest{
		Id:  "00000000-0000-0000-0000-000000000000",
		Arg: "",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// TestUpdateWorkflow_MultipleFields verifies that multiple fields can be updated
// simultaneously and that unaffected fields remain unchanged.
func TestUpdateWorkflow_MultipleFields(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "multi-update",
		Name:            "Original Name",
		Command:         "original-cmd",
		TargetDirectory: "/tmp/original",
		Description:     "original desc",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Workflow.Id

	updateResp, err := svc.UpdateWorkflow(ctx, connect.NewRequest(&sessionv1.UpdateWorkflowRequest{
		Id:              id,
		Name:            proto.String("Updated Name"),
		Command:         proto.String("new-cmd"),
		TargetDirectory: proto.String("/tmp/new"),
	}))
	require.NoError(t, err)

	updated := updateResp.Msg.Workflow
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, "new-cmd", updated.Command)
	assert.Equal(t, "/tmp/new", updated.TargetDirectory)
	// Unchanged fields should retain original values.
	assert.Equal(t, "multi-update", updated.Slug)
	assert.Equal(t, "original desc", updated.Description)
}
