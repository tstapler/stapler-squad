package mcp

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"
)

// workflowHandlers implements workflow-management MCP tools. Workflows are
// reusable session-creation presets (e.g. "design doc session") that a
// session can define once and re-run with a different input each time.
type workflowHandlers struct {
	svc *services.SessionService
}

// registerWorkflowTools registers the workflow-related MCP tools.
func registerWorkflowTools(s *mcpserver.MCPServer, h *workflowHandlers) {
	s.AddTool(
		mcpgo.NewTool("create_workflow",
			mcpgo.WithDescription("Create a reusable workflow — a preset for spawning sessions with a fixed command/directory/model combo (e.g. a 'design doc session' workflow). Run it later with run_workflow."),
			mcpgo.WithString("slug",
				mcpgo.Description("Unique short identifier for the workflow (used as @slug in the omnibar)"),
				mcpgo.Required(),
			),
			mcpgo.WithString("name",
				mcpgo.Description("Human-readable workflow name"),
				mcpgo.Required(),
			),
			mcpgo.WithString("description",
				mcpgo.Description("Optional description of what this workflow does"),
			),
			mcpgo.WithString("command",
				mcpgo.Description("Command/prompt to run when the workflow is triggered"),
				mcpgo.Required(),
			),
			mcpgo.WithString("target_directory",
				mcpgo.Description("Directory or repo path the session should be created in"),
			),
			mcpgo.WithString("input_template",
				mcpgo.Description("Template string with {{input}} placeholder, filled in from run_workflow's arg"),
			),
			mcpgo.WithString("session_type",
				mcpgo.Description("Session creation mode (default: directory)"),
				mcpgo.DefaultString("directory"),
				mcpgo.Enum("directory", "new_worktree", "existing_worktree", "one_off", "new_project"),
			),
			mcpgo.WithString("model",
				mcpgo.Description("Optional model override for sessions created by this workflow"),
			),
			mcpgo.WithString("agent_type",
				mcpgo.Description("Optional agent type override (e.g. claude, aider)"),
			),
			mcpgo.WithString("cron_expression",
				mcpgo.Description("Optional cron expression to run this workflow on a schedule"),
			),
			mcpgo.WithBoolean("cron_enabled",
				mcpgo.Description("Whether the cron schedule is active (default: false)"),
			),
			mcpgo.WithBoolean("enabled",
				mcpgo.Description("Whether this trigger is active — gates webhook/github_push firing as well as cron (default: true)"),
			),
			mcpgo.WithNumber("keep_sessions",
				mcpgo.Description("Keep only the N most recent sessions for this workflow (0/omitted = keep all)"),
			),
			mcpgo.WithNumber("archive_after_hours",
				mcpgo.Description("Auto-archive completed sessions after this many hours (0/omitted = disabled)"),
			),
		),
		h.createWorkflow,
	)

	s.AddTool(
		mcpgo.NewTool("update_workflow",
			mcpgo.WithDescription("Update fields on an existing workflow. Only provided fields are changed."),
			mcpgo.WithString("id",
				mcpgo.Description("Workflow ID to update"),
				mcpgo.Required(),
			),
			mcpgo.WithString("name", mcpgo.Description("New name")),
			mcpgo.WithString("description", mcpgo.Description("New description")),
			mcpgo.WithString("command", mcpgo.Description("New command/prompt")),
			mcpgo.WithString("target_directory", mcpgo.Description("New target directory")),
			mcpgo.WithString("input_template", mcpgo.Description("New input template")),
			mcpgo.WithString("session_type",
				mcpgo.Description("New session creation mode"),
				mcpgo.Enum("directory", "new_worktree", "existing_worktree", "one_off", "new_project"),
			),
			mcpgo.WithString("model", mcpgo.Description("New model override")),
			mcpgo.WithString("agent_type", mcpgo.Description("New agent type override")),
			mcpgo.WithString("cron_expression", mcpgo.Description("New cron expression")),
			mcpgo.WithBoolean("cron_enabled", mcpgo.Description("New cron enabled state")),
			mcpgo.WithBoolean("enabled", mcpgo.Description("New trigger-enabled state")),
			mcpgo.WithNumber("keep_sessions", mcpgo.Description("New keep_sessions value")),
			mcpgo.WithNumber("archive_after_hours", mcpgo.Description("New archive_after_hours value")),
		),
		h.updateWorkflow,
	)

	s.AddTool(
		mcpgo.NewTool("delete_workflow",
			mcpgo.WithDescription("Delete a workflow by ID. Does not affect sessions already created from it."),
			mcpgo.WithString("id",
				mcpgo.Description("Workflow ID to delete"),
				mcpgo.Required(),
			),
		),
		h.deleteWorkflow,
	)

	s.AddTool(
		mcpgo.NewTool("list_workflows",
			mcpgo.WithDescription("List all configured workflows."),
		),
		h.listWorkflows,
	)

	s.AddTool(
		mcpgo.NewTool("run_workflow",
			mcpgo.WithDescription("Run a workflow by ID, creating a new session from its preset. If the workflow has an input_template, arg replaces {{input}} in it; otherwise arg is appended to the command."),
			mcpgo.WithString("id",
				mcpgo.Description("Workflow ID to run"),
				mcpgo.Required(),
			),
			mcpgo.WithString("arg",
				mcpgo.Description("Optional argument injected into the workflow's input_template or command"),
			),
		),
		h.runWorkflow,
	)
}

// --- create_workflow ---

// WorkflowResult is the wire representation of a workflow returned by MCP tools.
type WorkflowResult struct {
	ID                string `json:"id"`
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	Command           string `json:"command"`
	TargetDirectory   string `json:"target_directory,omitempty"`
	InputTemplate     string `json:"input_template,omitempty"`
	SessionType       string `json:"session_type"`
	Model             string `json:"model,omitempty"`
	AgentType         string `json:"agent_type,omitempty"`
	CronExpression    string `json:"cron_expression,omitempty"`
	CronEnabled       bool   `json:"cron_enabled"`
	Enabled           bool   `json:"enabled"`
	KeepSessions      *int32 `json:"keep_sessions,omitempty"`
	ArchiveAfterHours *int32 `json:"archive_after_hours,omitempty"`
}

func workflowToResult(w *sessionv1.WorkflowProto) WorkflowResult {
	return WorkflowResult{
		ID:                w.GetId(),
		Slug:              w.GetSlug(),
		Name:              w.GetName(),
		Description:       w.GetDescription(),
		Command:           w.GetCommand(),
		TargetDirectory:   w.GetTargetDirectory(),
		InputTemplate:     w.GetInputTemplate(),
		SessionType:       w.GetSessionType(),
		Model:             w.GetModel(),
		AgentType:         w.GetAgentType(),
		CronExpression:    w.GetCronExpression(),
		CronEnabled:       w.GetCronEnabled(),
		Enabled:           w.GetEnabled(),
		KeepSessions:      w.KeepSessions,
		ArchiveAfterHours: w.ArchiveAfterHours,
	}
}

type CreateWorkflowResult struct {
	MCPResult
	Workflow WorkflowResult `json:"workflow"`
}

func (h *workflowHandlers) createWorkflow(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	slug, _ := args["slug"].(string)
	if slug == "" {
		return errResult(ErrInvalidArgument, "slug is required", ""), nil
	}
	name, _ := args["name"].(string)
	if name == "" {
		return errResult(ErrInvalidArgument, "name is required", ""), nil
	}
	command, _ := args["command"].(string)
	if command == "" {
		return errResult(ErrInvalidArgument, "command is required", ""), nil
	}

	sessionType, _ := args["session_type"].(string)
	if sessionType == "" {
		sessionType = "directory"
	}

	protoReq := &sessionv1.CreateWorkflowRequest{
		Slug:              slug,
		Name:              name,
		Description:       stringArg(args, "description"),
		Command:           command,
		TargetDirectory:   stringArg(args, "target_directory"),
		InputTemplate:     stringArg(args, "input_template"),
		SessionType:       sessionType,
		Model:             stringArg(args, "model"),
		AgentType:         stringArg(args, "agent_type"),
		CronExpression:    stringArg(args, "cron_expression"),
		CronEnabled:       boolArg(args, "cron_enabled"),
		Enabled:           boolPtrArg(args, "enabled"),
		KeepSessions:      int32PtrArg(args, "keep_sessions"),
		ArchiveAfterHours: int32PtrArg(args, "archive_after_hours"),
	}

	resp, err := h.svc.CreateWorkflow(ctx, connect.NewRequest(protoReq))
	if err != nil {
		return workflowServiceErrResult(err)
	}

	return okResult(CreateWorkflowResult{
		MCPResult: MCPResult{Success: true},
		Workflow:  workflowToResult(resp.Msg.GetWorkflow()),
	}), nil
}

// --- update_workflow ---

type UpdateWorkflowResult struct {
	MCPResult
	Workflow WorkflowResult `json:"workflow"`
}

func (h *workflowHandlers) updateWorkflow(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	id, _ := args["id"].(string)
	if id == "" {
		return errResult(ErrInvalidArgument, "id is required", ""), nil
	}

	protoReq := &sessionv1.UpdateWorkflowRequest{
		Id:                id,
		Name:              stringPtrArg(args, "name"),
		Description:       stringPtrArg(args, "description"),
		Command:           stringPtrArg(args, "command"),
		TargetDirectory:   stringPtrArg(args, "target_directory"),
		InputTemplate:     stringPtrArg(args, "input_template"),
		SessionType:       stringPtrArg(args, "session_type"),
		Model:             stringPtrArg(args, "model"),
		AgentType:         stringPtrArg(args, "agent_type"),
		CronExpression:    stringPtrArg(args, "cron_expression"),
		CronEnabled:       boolPtrArg(args, "cron_enabled"),
		Enabled:           boolPtrArg(args, "enabled"),
		KeepSessions:      int32PtrArg(args, "keep_sessions"),
		ArchiveAfterHours: int32PtrArg(args, "archive_after_hours"),
	}

	resp, err := h.svc.UpdateWorkflow(ctx, connect.NewRequest(protoReq))
	if err != nil {
		return workflowServiceErrResult(err)
	}

	return okResult(UpdateWorkflowResult{
		MCPResult: MCPResult{Success: true},
		Workflow:  workflowToResult(resp.Msg.GetWorkflow()),
	}), nil
}

// --- delete_workflow ---

func (h *workflowHandlers) deleteWorkflow(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	id, _ := args["id"].(string)
	if id == "" {
		return errResult(ErrInvalidArgument, "id is required", ""), nil
	}

	if _, err := h.svc.DeleteWorkflow(ctx, connect.NewRequest(&sessionv1.DeleteWorkflowRequest{Id: id})); err != nil {
		return workflowServiceErrResult(err)
	}

	return okResult(MCPResult{Success: true}), nil
}

// --- list_workflows ---

type ListWorkflowsResult struct {
	MCPResult
	Workflows []WorkflowResult `json:"workflows"`
}

func (h *workflowHandlers) listWorkflows(ctx context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	resp, err := h.svc.ListWorkflows(ctx, connect.NewRequest(&sessionv1.ListWorkflowsRequest{}))
	if err != nil {
		return workflowServiceErrResult(err)
	}

	out := make([]WorkflowResult, 0, len(resp.Msg.GetWorkflows()))
	for _, w := range resp.Msg.GetWorkflows() {
		out = append(out, workflowToResult(w))
	}

	return okResult(ListWorkflowsResult{
		MCPResult: MCPResult{Success: true},
		Workflows: out,
	}), nil
}

// --- run_workflow ---

type RunWorkflowResult struct {
	MCPResult
	SessionID string `json:"session_id"`
}

func (h *workflowHandlers) runWorkflow(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	id, _ := args["id"].(string)
	if id == "" {
		return errResult(ErrInvalidArgument, "id is required", ""), nil
	}

	resp, err := h.svc.RunWorkflow(ctx, connect.NewRequest(&sessionv1.RunWorkflowRequest{
		Id:  id,
		Arg: stringArg(args, "arg"),
	}))
	if err != nil {
		return workflowServiceErrResult(err)
	}

	return okResult(RunWorkflowResult{
		MCPResult: MCPResult{Success: true},
		SessionID: resp.Msg.GetSessionId(),
	}), nil
}

// --- shared arg helpers ---

func stringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func stringPtrArg(args map[string]any, key string) *string {
	v, ok := args[key].(string)
	if !ok || v == "" {
		return nil
	}
	return &v
}

func boolArg(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}

func boolPtrArg(args map[string]any, key string) *bool {
	v, ok := args[key].(bool)
	if !ok {
		return nil
	}
	return &v
}

func int32PtrArg(args map[string]any, key string) *int32 {
	v, ok := args[key].(float64)
	if !ok {
		return nil
	}
	i := int32(v)
	return &i
}

// workflowServiceErrResult maps a connect error from the workflow/rules
// service layer into an MCP tool error result.
func workflowServiceErrResult(err error) (*mcpgo.CallToolResult, error) {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		switch connectErr.Code() {
		case connect.CodeNotFound:
			return errResult(ErrItemNotFound, connectErr.Message(), ""), nil
		case connect.CodeInvalidArgument:
			return errResult(ErrInvalidArgument, connectErr.Message(), ""), nil
		case connect.CodeUnavailable:
			return errResult(ErrFeatureDisabled, connectErr.Message(), ""), nil
		}
	}
	return errResult(ErrInternalError, fmt.Sprintf("workflow operation failed: %v", err), ""), nil
}
