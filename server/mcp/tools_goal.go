package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

// goalHandlers implements session goal MCP tools.
type goalHandlers struct {
	storage  *session.Storage
	store    session.InstanceStore
	eventBus *events.EventBus
}

// registerGoalTools registers the goal-related MCP tools.
func registerGoalTools(s *mcpserver.MCPServer, h *goalHandlers) {
	s.AddTool(
		mcpgo.NewTool("set_session_goal",
			mcpgo.WithDescription("Set or update the goal and task tree for a session. If session_id is omitted, the calling session's UUID is used (agent self-reporting). Supports an optional structured task list for tracking sub-tasks. Goal status must be one of: idle, working, blocked, done."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the target session. Optional — defaults to the calling session if unset."),
			),
			mcpgo.WithString("goal",
				mcpgo.Description("Human-readable goal description (max 2000 chars)"),
				mcpgo.Required(),
			),
			mcpgo.WithString("status",
				mcpgo.Description("Goal status: idle, working, blocked, done (default: idle)"),
				mcpgo.DefaultString("idle"),
				mcpgo.Enum("idle", "working", "blocked", "done"),
			),
			mcpgo.WithArray("tasks",
				mcpgo.Description("Optional task tree (max 50 tasks, max depth 3). Each task: {id, title, status, children?}"),
			),
		),
		h.setSessionGoal,
	)

	s.AddTool(
		mcpgo.NewTool("get_session_goal",
			mcpgo.WithDescription("Retrieve the current goal and task state for a session."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the session"),
				mcpgo.Required(),
			),
		),
		h.getSessionGoal,
	)

	s.AddTool(
		mcpgo.NewTool("update_session_task",
			mcpgo.WithDescription("Update the status of a specific task in the calling session's goal. Must be called from within the session (uses STAPLER_SESSION_UUID). Status must be one of: pending, in_progress, done, blocked."),
			mcpgo.WithString("task_id",
				mcpgo.Description("ID of the task to update"),
				mcpgo.Required(),
			),
			mcpgo.WithString("status",
				mcpgo.Description("New task status: pending, in_progress, done, blocked"),
				mcpgo.Required(),
				mcpgo.Enum("pending", "in_progress", "done", "blocked"),
			),
		),
		h.updateSessionTask,
	)
}

// --- set_session_goal ---

type SetSessionGoalResult struct {
	MCPResult
	SessionUUID string `json:"session_uuid"`
	Goal        string `json:"goal"`
	Status      string `json:"status"`
	TasksTotal  int    `json:"tasks_total"`
}

func (h *goalHandlers) setSessionGoal(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	// Priority: (1) session_id param if provided, (2) callerSessionUUID from context.
	var targetUUID string
	if sessionID, ok := args["session_id"].(string); ok && sessionID != "" {
		inst, errR := h.findInstanceByID(sessionID)
		if errR != nil {
			return errR, nil
		}
		targetUUID = inst.UUID
	} else {
		callerUUID, err := callerSessionUUID(ctx)
		if err != nil {
			return errResult(ErrPermissionDenied, "session_id is required or call from within a Stapler Squad session", "Provide session_id or set STAPLER_SESSION_UUID."), nil
		}
		targetUUID = callerUUID
	}

	goal, ok := args["goal"].(string)
	if !ok || goal == "" {
		return errResult(ErrInvalidArgument, "goal is required", ""), nil
	}
	if len(goal) > 2000 {
		return errResult(ErrInvalidArgument, "goal exceeds maximum length of 2000 characters", ""), nil
	}

	status := "idle"
	if s, ok := args["status"].(string); ok && s != "" {
		status = s
	}
	if !isValidGoalStatus(status) {
		return errResult(ErrInvalidArgument, fmt.Sprintf("invalid goal status %q: must be one of idle, working, blocked, done", status), ""), nil
	}

	// Parse tasks array if provided.
	var tasks []session.TaskNode
	if tasksRaw, ok := args["tasks"]; ok && tasksRaw != nil {
		b, err := json.Marshal(tasksRaw)
		if err != nil {
			return errResult(ErrInvalidArgument, fmt.Sprintf("failed to parse tasks: %v", err), ""), nil
		}
		if err := json.Unmarshal(b, &tasks); err != nil {
			return errResult(ErrInvalidArgument, fmt.Sprintf("invalid tasks format: %v", err), ""), nil
		}
	}

	if err := session.ValidateTaskDepth(tasks, 1); err != nil {
		return errResult(ErrInvalidArgument, err.Error(), ""), nil
	}

	setBy := targetUUID // record who set it
	goalData, err := h.storage.SetSessionGoal(ctx, targetUUID, goal, status, tasks, setBy)
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("failed to set goal: %v", err), ""), nil
	}

	// Update in-memory cache if the instance is loaded.
	inst, _ := h.findInstanceByUUID(targetUUID)
	if inst != nil {
		inst.SetSessionGoalCached(goalData)
		if h.eventBus != nil {
			h.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"goal"}))
		}
	}

	return okResult(SetSessionGoalResult{
		MCPResult:   MCPResult{Success: true},
		SessionUUID: targetUUID,
		Goal:        goalData.Goal,
		Status:      goalData.Status,
		TasksTotal:  goalData.TasksTotal(),
	}), nil
}

// --- get_session_goal ---

func (h *goalHandlers) getSessionGoal(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	inst, errR := h.findInstanceByID(sessionID)
	if errR != nil {
		return errR, nil
	}

	goalData, err := h.storage.GetSessionGoal(ctx, inst.UUID)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return errResult(ErrItemNotFound, fmt.Sprintf("no goal set for session %q", sessionID), "Use set_session_goal to create one."), nil
		}
		return errResult(ErrInternalError, fmt.Sprintf("failed to get goal: %v", err), ""), nil
	}

	b, _ := json.Marshal(goalData)
	return mcpgo.NewToolResultText(string(b)), nil
}

// --- update_session_task ---

type UpdateSessionTaskResult struct {
	MCPResult
	TaskID     string `json:"task_id"`
	NewStatus  string `json:"new_status"`
	TasksDone  int    `json:"tasks_done"`
	TasksTotal int    `json:"tasks_total"`
}

func (h *goalHandlers) updateSessionTask(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	// Strictly requires callerSessionUUID — agent self-reporting only.
	callerUUID, err := callerSessionUUID(ctx)
	if err != nil {
		return errResult(ErrPermissionDenied, err.Error(), "This tool can only be called from within a Stapler Squad session."), nil
	}

	args := req.GetArguments()
	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return errResult(ErrInvalidArgument, "task_id is required", ""), nil
	}

	newStatus, ok := args["status"].(string)
	if !ok || newStatus == "" {
		return errResult(ErrInvalidArgument, "status is required", ""), nil
	}
	if !isValidTaskStatus(newStatus) {
		return errResult(ErrInvalidArgument, fmt.Sprintf("invalid status %q: must be one of pending, in_progress, done, blocked", newStatus), ""), nil
	}

	// Verify the existing goal belongs to the caller session.
	existingGoal, err := h.storage.GetSessionGoal(ctx, callerUUID)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return errResult(ErrItemNotFound, "no goal set for this session", "Use set_session_goal first."), nil
		}
		return errResult(ErrInternalError, fmt.Sprintf("failed to retrieve goal: %v", err), ""), nil
	}
	if existingGoal.SessionUUID != callerUUID {
		return errResult(ErrPermissionDenied, "goal session UUID mismatch", ""), nil
	}

	updatedGoal, err := h.storage.UpdateSessionTaskStatus(ctx, callerUUID, taskID, newStatus)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return errResult(ErrItemNotFound, "no goal set for this session", ""), nil
		}
		return errResult(ErrInternalError, fmt.Sprintf("failed to update task: %v", err), ""), nil
	}
	// Check if the error was about the task not being found in the tree.
	// UpdateSessionTaskStatus returns a descriptive error if task_id is not found.

	// Update in-memory cache.
	inst, _ := h.findInstanceByUUID(callerUUID)
	if inst != nil {
		inst.SetSessionGoalCached(updatedGoal)
		if h.eventBus != nil {
			h.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"goal"}))
		}
	}

	return okResult(UpdateSessionTaskResult{
		MCPResult:  MCPResult{Success: true},
		TaskID:     taskID,
		NewStatus:  newStatus,
		TasksDone:  updatedGoal.TasksDone(),
		TasksTotal: updatedGoal.TasksTotal(),
	}), nil
}

// --- helpers ---

// findInstanceByID finds an instance by session title/ID from the store.
func (h *goalHandlers) findInstanceByID(sessionID string) (*session.Instance, *mcpgo.CallToolResult) {
	instances, err := h.store.LoadInstances()
	if err != nil {
		return nil, errResult(ErrInternalError, "failed to load sessions", "")
	}
	for _, inst := range instances {
		if inst.MatchesID(sessionID) {
			return inst, nil
		}
	}
	return nil, errResult(ErrSessionNotFound, fmt.Sprintf("session %q not found", sessionID), "Use list_sessions to find available sessions.")
}

// findInstanceByUUID finds an instance by its UUID (used for cache updates).
// Returns nil, nil if not found (non-fatal).
func (h *goalHandlers) findInstanceByUUID(uuid string) (*session.Instance, *mcpgo.CallToolResult) {
	instances, err := h.store.LoadInstances()
	if err != nil {
		return nil, nil
	}
	for _, inst := range instances {
		if inst.UUID == uuid {
			return inst, nil
		}
	}
	return nil, nil
}

// isValidGoalStatus returns true if s is a recognized goal status value.
func isValidGoalStatus(s string) bool {
	switch s {
	case session.GoalStatusIdle, session.GoalStatusWorking, session.GoalStatusBlocked, session.GoalStatusDone:
		return true
	}
	return false
}

// isValidTaskStatus returns true if s is a recognized task status value.
func isValidTaskStatus(s string) bool {
	switch s {
	case session.TaskStatusPending, session.TaskStatusInProgress, session.TaskStatusDone, session.TaskStatusBlocked:
		return true
	}
	return false
}
