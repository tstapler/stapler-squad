package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// newTestWorkflowHandlers wires a real *services.SessionService with a
// *services.WorkflowService attached, mirroring
// server/services/workflow_service_test.go's createTestWorkflowService
// helper (unexported there, so it can't be imported directly).
func newTestWorkflowHandlers(t *testing.T) *workflowHandlers {
	t.Helper()

	storage := newTestBacklogStorage(t)

	repo := session.NewTestEntRepository(t)

	workflowRepo := session.NewEntWorkflowRepository(repo.GetEntClient())
	workflowSvc := services.NewWorkflowService(workflowRepo, nil, storage)

	sessSvc := services.NewSessionService(storage, events.NewEventBus(100))
	sessSvc.SetWorkflowService(workflowSvc)

	return &workflowHandlers{svc: sessSvc}
}

func TestCreateWorkflow_should_CreateWorkflow_When_RequiredFieldsProvided(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	res, err := h.createWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"slug":             "design-doc",
		"name":             "Design doc session",
		"command":          "Write a design doc for the requested feature",
		"target_directory": t.TempDir(),
	}))
	if err != nil {
		t.Fatalf("createWorkflow returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", out)
	}
	workflow, ok := out["workflow"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workflow field in result, got: %+v", out)
	}
	if workflow["slug"] != "design-doc" {
		t.Errorf("expected slug=design-doc, got %v", workflow["slug"])
	}
	if workflow["session_type"] != "directory" {
		t.Errorf("expected default session_type=directory, got %v", workflow["session_type"])
	}
}

func TestCreateWorkflow_should_ReturnInvalidArgument_When_SlugMissing(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	res, err := h.createWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"name":    "Missing slug",
		"command": "do something",
	}))
	if err != nil {
		t.Fatalf("createWorkflow returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false when slug is missing, got: %+v", out)
	}
}

func TestCreateWorkflow_should_ReturnInvalidArgument_When_SlugNotKebabCase(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	res, err := h.createWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"slug":    "Bad-Slug",
		"name":    "Bad slug",
		"command": "do something",
	}))
	if err != nil {
		t.Fatalf("createWorkflow returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false for non-kebab-case slug, got: %+v", out)
	}
}

func TestUpdateWorkflow_should_UpdateName_When_WorkflowExists(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	created, err := h.createWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"slug":             "update-me",
		"name":             "Original name",
		"command":          "original command",
		"target_directory": t.TempDir(),
	}))
	require.NoError(t, err)
	createdOut := parseResult(t, created)
	workflow := createdOut["workflow"].(map[string]interface{})
	id := workflow["id"].(string)

	res, err := h.updateWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"id":   id,
		"name": "Updated name",
	}))
	if err != nil {
		t.Fatalf("updateWorkflow returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", out)
	}
	updatedWorkflow := out["workflow"].(map[string]interface{})
	if updatedWorkflow["name"] != "Updated name" {
		t.Errorf("expected name=Updated name, got %v", updatedWorkflow["name"])
	}
}

// TestCreateWorkflow_should_SetEnabledFalse_When_EnabledArgProvided and
// TestUpdateWorkflow_should_UpdateEnabled_When_EnabledArgProvided close a coverage gap
// found during sdd:6-verify's testing review: the enabled MCP arg (create + update) had
// no round-trip test.
func TestCreateWorkflow_should_SetEnabledFalse_When_EnabledArgProvided(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	res, err := h.createWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"slug":             "disabled-on-create",
		"name":             "Disabled On Create",
		"command":          "do something",
		"target_directory": t.TempDir(),
		"enabled":          false,
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", out)
	}
	workflow := out["workflow"].(map[string]interface{})
	if workflow["enabled"] != false {
		t.Errorf("expected enabled=false, got %v", workflow["enabled"])
	}
}

func TestUpdateWorkflow_should_UpdateEnabled_When_EnabledArgProvided(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	created, err := h.createWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"slug":             "enabled-update-me",
		"name":             "Original",
		"command":          "original command",
		"target_directory": t.TempDir(),
	}))
	require.NoError(t, err)
	createdOut := parseResult(t, created)
	workflow := createdOut["workflow"].(map[string]interface{})
	id := workflow["id"].(string)
	if workflow["enabled"] != true {
		t.Fatalf("expected create to default enabled=true, got %v", workflow["enabled"])
	}

	res, err := h.updateWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"id":      id,
		"enabled": false,
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", out)
	}
	updatedWorkflow := out["workflow"].(map[string]interface{})
	if updatedWorkflow["enabled"] != false {
		t.Errorf("expected enabled=false, got %v", updatedWorkflow["enabled"])
	}
}

func TestUpdateWorkflow_should_ReturnNotFound_When_WorkflowDoesNotExist(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	res, err := h.updateWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"id":   "does-not-exist",
		"name": "New name",
	}))
	if err != nil {
		t.Fatalf("updateWorkflow returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false for missing workflow, got: %+v", out)
	}
}

func TestDeleteWorkflow_should_RemoveWorkflow_When_WorkflowExists(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	created, err := h.createWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"slug":             "delete-me",
		"name":             "Delete me",
		"command":          "some command",
		"target_directory": t.TempDir(),
	}))
	require.NoError(t, err)
	createdOut := parseResult(t, created)
	workflow := createdOut["workflow"].(map[string]interface{})
	id := workflow["id"].(string)

	res, err := h.deleteWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("deleteWorkflow returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", out)
	}
}

func TestListWorkflows_should_ReturnEmptyList_When_NoWorkflowsCreated(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	res, err := h.listWorkflows(context.Background(), makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("listWorkflows returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", out)
	}
	workflows, ok := out["workflows"].([]interface{})
	if !ok {
		t.Fatalf("expected workflows field in result, got: %+v", out)
	}
	if len(workflows) != 0 {
		t.Errorf("expected empty workflows list, got: %+v", workflows)
	}
}

func TestListWorkflows_should_ReturnWorkflow_When_WorkflowCreated(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	_, err := h.createWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"slug":             "listed-workflow",
		"name":             "Listed workflow",
		"command":          "some command",
		"target_directory": t.TempDir(),
	}))
	require.NoError(t, err)

	res, err := h.listWorkflows(context.Background(), makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("listWorkflows returned error: %v", err)
	}
	out := parseResult(t, res)
	workflows, ok := out["workflows"].([]interface{})
	if !ok {
		t.Fatalf("expected workflows field in result, got: %+v", out)
	}
	found := false
	for _, w := range workflows {
		workflow, ok := w.(map[string]interface{})
		if ok && workflow["slug"] == "listed-workflow" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected listed-workflow to be present in list, got: %+v", workflows)
	}
}

func TestListWorkflows_should_DegradeGracefully_When_WorkflowServiceNotWired(t *testing.T) {
	// list_workflows must not error even when no WorkflowService has been
	// attached to the SessionService (e.g. workflow feature disabled).
	sessSvc := services.NewSessionService(newTestBacklogStorage(t), nil)
	h := &workflowHandlers{svc: sessSvc}

	res, err := h.listWorkflows(context.Background(), makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("listWorkflows returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true even without a wired WorkflowService, got: %+v", out)
	}
}

func TestRunWorkflow_should_ReturnFeatureDisabled_When_WorkflowServiceNotWired(t *testing.T) {
	sessSvc := services.NewSessionService(newTestBacklogStorage(t), nil)
	h := &workflowHandlers{svc: sessSvc}

	res, err := h.runWorkflow(context.Background(), makeToolReq(map[string]interface{}{
		"id": "some-id",
	}))
	if err != nil {
		t.Fatalf("runWorkflow returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false when workflow service is not wired, got: %+v", out)
	}
}

func TestRunWorkflow_should_ReturnInvalidArgument_When_IdMissing(t *testing.T) {
	h := newTestWorkflowHandlers(t)

	res, err := h.runWorkflow(context.Background(), makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("runWorkflow returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false when id is missing, got: %+v", out)
	}
}
