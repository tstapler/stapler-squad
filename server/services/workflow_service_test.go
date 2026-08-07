package services

import (
	"context"
	"net/http"
	"testing"
	"time"

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

// TestCreateWorkflow_WebhookTriggerType_CronEnabledTrue_Accepted proves the fix for a
// bug found during Phase 7 review: validateTriggerTypeFieldConsistency originally
// rejected cron_enabled=true for any non-"cron" trigger_type, but Phase 2's webhook
// handlers and Phase 7's TriggersPanel toggle both independently reuse CronEnabled as
// the generic per-trigger "enabled" flag across every trigger type — the old check made
// it impossible to ever enable a webhook/github_push trigger through this RPC. This must
// be accepted, and (per TestScheduler_Start_DoesNotRegisterMismatchedTriggerAsCron /
// TestScheduler_Reload_DoesNotRegisterMismatchedTriggerAsCron in scheduler_test.go)
// never causes the resulting row to register as a cron entry, since that gate
// independently requires trigger_type=="cron" too.
func TestCreateWorkflow_WebhookTriggerType_CronEnabledTrue_Accepted(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	resp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "enabled-webhook-wf",
		Name:            "Enabled Webhook Trigger",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		TriggerType:     "webhook",
		WebhookSlug:     "enabled-webhook",
		CronEnabled:     true, // the generic "enabled" flag, not a claim this is a cron trigger
	}))
	require.NoError(t, err, "cron_enabled=true must be accepted for a non-cron trigger_type — it is the generic enable flag")
	assert.True(t, resp.Msg.Workflow.CronEnabled)
	assert.Equal(t, "webhook", resp.Msg.Workflow.TriggerType)
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

	// Flip trigger_type to "manual" while leaving the existing webhook_slug populated —
	// the effective state (trigger_type=manual, webhook_slug=existing-slug) is a genuine
	// mismatch: a manual trigger has no business retaining a webhook routing slug.
	// (cron_enabled=true alone is NOT tested here as a mismatch — it's the reused
	// generic per-trigger enabled flag, valid for any trigger_type; see
	// TestUpdateWorkflow_CronEnabledTrue_AcceptedForNonCronTriggerType below.)
	_, err = svc.UpdateWorkflow(ctx, connect.NewRequest(&sessionv1.UpdateWorkflowRequest{
		Id:          id,
		TriggerType: proto.String("manual"),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestUpdateWorkflow_CronEnabledTrue_AcceptedForNonCronTriggerType is the UpdateWorkflow
// sibling of TestCreateWorkflow_WebhookTriggerType_CronEnabledTrue_Accepted: setting
// cron_enabled=true on an existing non-cron trigger (enabling it) must succeed, not be
// rejected as a trigger-type mismatch.
func TestUpdateWorkflow_CronEnabledTrue_AcceptedForNonCronTriggerType(t *testing.T) {
	_, svc := createTestWorkflowService(t)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "update-enable-webhook",
		Name:            "Original",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		TriggerType:     "webhook",
		WebhookSlug:     "update-enable-slug",
		CronEnabled:     false,
	}))
	require.NoError(t, err)
	id := createResp.Msg.Workflow.Id

	resp, err := svc.UpdateWorkflow(ctx, connect.NewRequest(&sessionv1.UpdateWorkflowRequest{
		Id:          id,
		CronEnabled: proto.Bool(true),
	}))
	require.NoError(t, err, "enabling an existing webhook trigger via cron_enabled must be accepted")
	assert.True(t, resp.Msg.Workflow.CronEnabled)
	assert.Equal(t, "webhook", resp.Msg.Workflow.TriggerType)
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

// ─── webhook_secret (Phase 7 follow-up, Task 7.2) ──────────────────────────────
//
// These tests use webhookTestInfra (webhook_handler_test_helpers_test.go) rather than
// createTestWorkflowService, specifically because it isolates STAPLER_SQUAD_TEST_DIR —
// encryptWebhookSecret (workflow_service.go) calls config.LoadConfig(), which without
// that isolation would read/write the developer's real ~/.stapler-squad/config.json.
//
// The first two tests below verify the round trip via decryptWorkflowSecret +
// VerifyWebhookSecret directly (the exact two functions GenericWebhookHandler.Handle
// itself calls for HMAC verification). TestCreateWorkflow_WebhookSecret_FullHTTPRoundTrip
// closes the loop further by driving an actual HTTP request through
// GenericWebhookHandler.Handle end-to-end — this was blocked until
// validateTriggerTypeFieldConsistency's cron_enabled/trigger_type conflation bug was
// fixed (found via this exact gap): the check originally rejected cron_enabled=true for
// any non-"cron" trigger_type, but GenericWebhookHandler.Handle gates on wf.CronEnabled
// as the generic per-trigger enabled flag — making it impossible to ever enable (and
// therefore ever successfully fire) a webhook/github_push trigger created through the
// real RPC. See validateTriggerTypeFieldConsistency's doc comment for the fix.

// TestCreateWorkflow_WebhookSecret_RoundTripsThroughHMACVerification proves the gap
// closed by CreateWorkflowRequest.webhook_secret: a plaintext secret set via the RPC is
// encrypted, stored, and actually usable by the real inbound webhook HMAC verification
// logic (decryptWorkflowSecret + VerifyWebhookSecret) — not merely persisted in
// isolation.
func TestCreateWorkflow_WebhookSecret_RoundTripsThroughHMACVerification(t *testing.T) {
	infra := newWebhookTestInfra(t)
	svc := NewWorkflowService(infra.workflowRepo, nil, nil)
	ctx := context.Background()

	resp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "secret-rt-wf",
		Name:            "Secret Round Trip",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		TriggerType:     "webhook",
		WebhookSlug:     "secret-rt",
		WebhookSecret:   "s3cr3t-plain",
	}))
	require.NoError(t, err)
	assert.Equal(t, "secret-rt", resp.Msg.Workflow.WebhookSlug)

	wf, err := infra.workflowRepo.GetByID(ctx, uuid.MustParse(resp.Msg.Workflow.Id))
	require.NoError(t, err)
	require.NotEmpty(t, wf.WebhookSecretEncrypted, "webhook_secret must have been encrypted and stored")

	decrypted, err := decryptWorkflowSecret(infra.cfg, wf)
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t-plain", decrypted)

	body := jiraTicketBody(t, "issue_created", nil, "PROJ-1", "fix it")
	goodSig := sign("s3cr3t-plain", body)
	assert.True(t, VerifyWebhookSecret(decrypted, body, goodSig),
		"a request signed with the plaintext secret sent via CreateWorkflow must verify")

	badSig := sign("wrong-secret", body)
	assert.False(t, VerifyWebhookSecret(decrypted, body, badSig),
		"a request signed with a different secret must not verify")
}

// TestUpdateWorkflow_WebhookSecret_OmittedLeavesExistingSecretUsable proves the "omit
// means unchanged" contract: an UpdateWorkflow call that changes an unrelated field and
// leaves webhook_secret empty must not clear the previously-set secret — it must still
// decrypt to the original value and verify requests signed with it.
func TestUpdateWorkflow_WebhookSecret_OmittedLeavesExistingSecretUsable(t *testing.T) {
	infra := newWebhookTestInfra(t)
	svc := NewWorkflowService(infra.workflowRepo, nil, nil)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "secret-upd-wf",
		Name:            "Secret Update",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		TriggerType:     "webhook",
		WebhookSlug:     "secret-upd",
		WebhookSecret:   "original-secret",
	}))
	require.NoError(t, err)

	_, err = svc.UpdateWorkflow(ctx, connect.NewRequest(&sessionv1.UpdateWorkflowRequest{
		Id:          createResp.Msg.Workflow.Id,
		Description: proto.String("updated description, no secret change"),
	}))
	require.NoError(t, err)

	wf, err := infra.workflowRepo.GetByID(ctx, uuid.MustParse(createResp.Msg.Workflow.Id))
	require.NoError(t, err)
	decrypted, err := decryptWorkflowSecret(infra.cfg, wf)
	require.NoError(t, err)
	assert.Equal(t, "original-secret", decrypted)

	body := jiraTicketBody(t, "issue_created", nil, "PROJ-1", "fix it")
	sig := sign("original-secret", body)
	assert.True(t, VerifyWebhookSecret(decrypted, body, sig))
}

// TestUpdateWorkflow_WebhookSecret_NonEmptyRotatesStoredSecret proves the complementary
// case: a non-empty webhook_secret on UpdateWorkflow actually rotates the stored secret
// — the old secret stops verifying and the new one takes over.
func TestUpdateWorkflow_WebhookSecret_NonEmptyRotatesStoredSecret(t *testing.T) {
	infra := newWebhookTestInfra(t)
	svc := NewWorkflowService(infra.workflowRepo, nil, nil)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "secret-rot-wf",
		Name:            "Secret Rotate",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		TriggerType:     "webhook",
		WebhookSlug:     "secret-rot",
		WebhookSecret:   "old-secret",
	}))
	require.NoError(t, err)

	_, err = svc.UpdateWorkflow(ctx, connect.NewRequest(&sessionv1.UpdateWorkflowRequest{
		Id:            createResp.Msg.Workflow.Id,
		WebhookSecret: "new-secret",
	}))
	require.NoError(t, err)

	wf, err := infra.workflowRepo.GetByID(ctx, uuid.MustParse(createResp.Msg.Workflow.Id))
	require.NoError(t, err)
	decrypted, err := decryptWorkflowSecret(infra.cfg, wf)
	require.NoError(t, err)
	assert.Equal(t, "new-secret", decrypted)

	body := jiraTicketBody(t, "issue_created", nil, "PROJ-1", "fix it")
	assert.False(t, VerifyWebhookSecret(decrypted, body, sign("old-secret", body)))
	assert.True(t, VerifyWebhookSecret(decrypted, body, sign("new-secret", body)))
}

// TestCreateWorkflow_WebhookSecret_FullHTTPRoundTrip is the true end-to-end proof: a
// webhook trigger created and enabled entirely through the public RPC surface
// (CreateWorkflow with webhook_secret + cron_enabled=true) actually fires when a real
// HTTP request signed with that secret hits GenericWebhookHandler.Handle — not merely
// "the secret is stored and the standalone verify function accepts it" (already covered
// above), but the full stack an operator using TriggerFormModal would exercise.
func TestCreateWorkflow_WebhookSecret_FullHTTPRoundTrip(t *testing.T) {
	infra := newWebhookTestInfra(t)
	svc := NewWorkflowService(infra.workflowRepo, infra.scheduler, nil)
	ctx := context.Background()

	createResp, err := svc.CreateWorkflow(ctx, connect.NewRequest(&sessionv1.CreateWorkflowRequest{
		Slug:            "e2e-webhook-wf",
		Name:            "End To End Webhook",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		TriggerType:     "webhook",
		WebhookSlug:     "e2e-slug",
		WebhookSecret:   "e2e-plain-secret",
		EventFilter:     "issue_created",
		LabelFilter:     "urgent",
		PromptTemplate:  "Triage {{.issue.key}}: {{.issue.summary}}",
		CronEnabled:     true, // the generic enabled flag — must be settable and honored
	}))
	require.NoError(t, err)
	assert.True(t, createResp.Msg.Workflow.CronEnabled, "the trigger must come back enabled")

	h := NewGenericWebhookHandler(infra.workflowRepo, infra.scheduler, infra.dispatcher, infra.fireEvents, infra.cfg)
	mux := newGenericWebhookMux(h)

	body := jiraTicketBody(t, "issue_created", []string{"urgent"}, "PROJ-1", "fix it")
	sig := sign("e2e-plain-secret", body)

	rec := doGenericWebhookRequest(t, mux, "e2e-slug", body, sig)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Eventually(t, func() bool {
		return infra.sessionSvc.callCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond,
		"a session must have been created via the real handler, not just the direct verify functions")

	req := infra.sessionSvc.LastRequest()
	require.NotNil(t, req)
	assert.Contains(t, req.InitialPrompt, "Triage PROJ-1: fix it")
}
