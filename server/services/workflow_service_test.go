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
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"google.golang.org/protobuf/proto"
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
	t.Cleanup(func() { sessSvc.Shutdown() })

	repo := createTestEntClient(t)
	workflowRepo := session.NewEntWorkflowRepository(repo.GetEntClient())
	workflowSvc := NewWorkflowService(workflowRepo, nil /* scheduler nil OK for CRUD tests */, storage)
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

// TestCreateWorkflow_TriggerTypeMismatch_Rejected verifies Task 1.1.1e's save-time
// validation: a request whose populated match-criteria fields don't match the
// declared trigger_type is rejected with CodeInvalidArgument.
func TestCreateWorkflow_TriggerTypeMismatch_Rejected(t *testing.T) {
	tests := []struct {
		name string
		req  *sessionv1.CreateWorkflowRequest
	}{
		{
			name: "webhook trigger_type with cron_enabled",
			req: &sessionv1.CreateWorkflowRequest{
				Slug:            "mismatch-webhook-cron",
				Name:            "Mismatch",
				Command:         "cmd",
				TargetDirectory: "/tmp/test",
				TriggerType:     "webhook",
				CronEnabled:     true,
				CronExpression:  "0 9 * * 1",
			},
		},
		{
			name: "manual trigger_type with webhook_slug set",
			req: &sessionv1.CreateWorkflowRequest{
				Slug:            "mismatch-manual-slug",
				Name:            "Mismatch",
				Command:         "cmd",
				TargetDirectory: "/tmp/test",
				TriggerType:     "manual",
				WebhookSlug:     "jira-ticket",
			},
		},
		{
			name: "cron trigger_type with github_repo set",
			req: &sessionv1.CreateWorkflowRequest{
				Slug:            "mismatch-cron-repo",
				Name:            "Mismatch",
				Command:         "cmd",
				TargetDirectory: "/tmp/test",
				TriggerType:     "cron",
				CronExpression:  "0 9 * * 1",
				CronEnabled:     true,
				GithubRepo:      "owner/repo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, svc := createTestWorkflowService(t)
			ctx := context.Background()

			_, err := svc.CreateWorkflow(ctx, connect.NewRequest(tt.req))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

// TestCreateWorkflow_WebhookTrigger_RoundTripsByWebhookSlug verifies Story 1.1.1's
// acceptance criteria: a Workflow row created with trigger_type="webhook" and a
// webhook_slug is retrievable via GetByWebhookSlug.
func TestCreateWorkflow_WebhookTrigger_RoundTripsByWebhookSlug(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	resp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "jira-ticket-wf",
		Name:            "Jira Ticket Trigger",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		TriggerType:     "webhook",
		WebhookSlug:     "jira-ticket",
		EventFilter:     "issue.created",
	}))
	require.NoError(t, err)
	assert.Equal(t, "webhook", resp.Msg.Workflow.TriggerType)
	assert.Equal(t, "jira-ticket", resp.Msg.Workflow.WebhookSlug)
	assert.Empty(t, resp.Msg.Workflow.CronExpression)
	assert.False(t, resp.Msg.Workflow.CronEnabled)

	wf, err := svc.repo.GetByWebhookSlug(ctx, "jira-ticket")
	require.NoError(t, err)
	assert.Equal(t, resp.Msg.Workflow.Id, wf.ID.String())
}

// TestUpdateWorkflow_TriggerTypeMismatch_Rejected verifies the same validation applies
// to UpdateWorkflow, evaluated against the *effective* (existing-row-merged-with-
// request) state — not just the request's own fields in isolation.
func TestUpdateWorkflow_TriggerTypeMismatch_Rejected(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "update-mismatch",
		Name:            "Original",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		TriggerType:     "webhook",
		WebhookSlug:     "existing-slug",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Workflow.Id

	// Flip cron_enabled=true without changing trigger_type away from "webhook" — the
	// effective state (trigger_type=webhook, webhook_slug=existing-slug,
	// cron_enabled=true) is invalid on two counts.
	_, err = svc.UpdateWorkflow(ctx, connect.NewRequest(&sessionv1.UpdateWorkflowRequest{
		Id:             id,
		CronEnabled:    proto.Bool(true),
		CronExpression: proto.String("0 9 * * 1"),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestUpdateWorkflow_UnrelatedFieldChange_NotSpuriouslyRejected verifies that an
// UpdateWorkflow call touching only an unrelated field (not any trigger field) is not
// rejected by the trigger-type consistency check.
func TestUpdateWorkflow_UnrelatedFieldChange_NotSpuriouslyRejected(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "update-unrelated",
		Name:            "Original",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		TriggerType:     "cron",
		CronEnabled:     true,
		CronExpression:  "0 9 * * 1",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Workflow.Id

	updateResp, err := svc.UpdateWorkflow(ctx, connect.NewRequest(&sessionv1.UpdateWorkflowRequest{
		Id:          id,
		Description: proto.String("updated description"),
	}))
	require.NoError(t, err)
	assert.Equal(t, "cron", updateResp.Msg.Workflow.TriggerType)
	assert.Equal(t, "updated description", updateResp.Msg.Workflow.Description)
}

// TestListTriggerFireEvents_ReturnsEventsForWorkflow verifies Task 1.2.1d's minimal
// query-only RPC round-trips TriggerFireEvent rows recorded against a workflow.
func TestListTriggerFireEvents_ReturnsEventsForWorkflow(t *testing.T) {
	sessSvc, workflowSvc := createTestWorkflowService(t)
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
	entRepo := createTestEntClient(t)
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
	sessSvc, _ := createTestWorkflowService(t)
	ctx := context.Background()

	resp, err := sessSvc.ListTriggerFireEvents(ctx, connect.NewRequest(&sessionv1.ListTriggerFireEventsRequest{
		WorkflowId: uuid.New().String(),
	}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Events)
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

// TestCreateWorkflow_MalformedPromptTemplate_Rejected verifies Task 3.1.1b: a
// syntactically invalid prompt_template is rejected at save time with
// CodeInvalidArgument rather than only surfacing at fire time.
func TestCreateWorkflow_MalformedPromptTemplate_Rejected(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	_, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "bad-prompt-template",
		Name:            "Bad Template",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		PromptTemplate:  "Fix {{.issue.key",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestCreateWorkflow_ValidPromptTemplate_Accepted is the control case: a syntactically
// valid prompt_template is accepted and persisted.
func TestCreateWorkflow_ValidPromptTemplate_Accepted(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	resp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "good-prompt-template",
		Name:            "Good Template",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		PromptTemplate:  "Fix {{.issue.key}}",
	}))
	require.NoError(t, err)
	assert.Equal(t, "Fix {{.issue.key}}", resp.Msg.Workflow.PromptTemplate)
}

// TestUpdateWorkflow_MalformedPromptTemplate_Rejected is UpdateWorkflow's sibling of
// TestCreateWorkflow_MalformedPromptTemplate_Rejected.
func TestUpdateWorkflow_MalformedPromptTemplate_Rejected(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "update-bad-template",
		Name:            "Original",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Workflow.Id

	_, err = svc.UpdateWorkflow(ctx, connect.NewRequest(&sessionv1.UpdateWorkflowRequest{
		Id:             id,
		PromptTemplate: proto.String("Fix {{.issue.key"),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
