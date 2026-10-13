package mcp

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

const errBacklogSvcNotWired = "backlog service not wired"

// connectErrResult maps a ConnectRPC handler error onto the MCP error codes so
// tools that delegate to BacklogService report the same failure classes the
// RPC does.
func connectErrResult(op string, err error) *mcpgo.CallToolResult {
	msg := fmt.Sprintf("%s: %v", op, err)
	switch connect.CodeOf(err) {
	case connect.CodeNotFound:
		return errResult(ErrItemNotFound, msg, itemNotFoundRemediation)
	case connect.CodeInvalidArgument:
		return errResult(ErrInvalidArgument, msg, "")
	case connect.CodeFailedPrecondition:
		return errResult(ErrFailedPrecondition, msg, "")
	case connect.CodeAborted, connect.CodeAlreadyExists:
		return errResult(ErrConflict, msg, "")
	case connect.CodeUnavailable:
		return errResult(ErrUnavailable, msg, "")
	default:
		return errResult(ErrInternalError, msg, "")
	}
}

// requireUUIDArg extracts a required UUID string argument.
func requireUUIDArg(args map[string]any, name string) (string, *mcpgo.CallToolResult) {
	v, ok := args[name].(string)
	if !ok || v == "" {
		return "", errResult(ErrInvalidArgument, name+" is required", "")
	}
	if err := validateUUID(v); err != nil {
		return "", errResult(ErrInvalidArgument, fmt.Sprintf("%s: %v", name, err), "Provide a valid UUID (e.g. from list_backlog_items).")
	}
	return v, nil
}

// --- add_backlog_item_dependency / remove_backlog_item_dependency ---

// dependencyOp describes one edge-mutating tool so add/remove share a handler.
type dependencyOp struct {
	tool  string
	verb  string
	apply func(session.BacklogItemDependencyEdge) error
}

func (h *backlogHandlers) addBacklogItemDependency(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return h.mutateDependency(ctx, req, dependencyOp{
		tool: "add_backlog_item_dependency",
		verb: "Added",
		apply: func(e session.BacklogItemDependencyEdge) error {
			_, err := h.backlogSvc.AddBacklogItemDependency(ctx, connect.NewRequest(&sessionv1.AddBacklogItemDependencyRequest{
				BlockerItemId: e.BlockerID,
				BlockedItemId: e.BlockedID,
			}))
			return err
		},
	})
}

func (h *backlogHandlers) removeBacklogItemDependency(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return h.mutateDependency(ctx, req, dependencyOp{
		tool: "remove_backlog_item_dependency",
		verb: "Removed (or already absent)",
		apply: func(e session.BacklogItemDependencyEdge) error {
			_, err := h.backlogSvc.RemoveBacklogItemDependency(ctx, connect.NewRequest(&sessionv1.RemoveBacklogItemDependencyRequest{
				BlockerItemId: e.BlockerID,
				BlockedItemId: e.BlockedID,
			}))
			return err
		},
	})
}

func (h *backlogHandlers) mutateDependency(_ context.Context, req mcpgo.CallToolRequest, op dependencyOp) (*mcpgo.CallToolResult, error) {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r, nil
	}
	if h.backlogSvc == nil {
		return errResult(ErrUnavailable, errBacklogSvcNotWired, ""), nil
	}
	args := req.GetArguments()
	blocker, bad := requireUUIDArg(args, "blocker_item_id")
	if bad != nil {
		return bad, nil
	}
	blocked, bad := requireUUIDArg(args, "blocked_item_id")
	if bad != nil {
		return bad, nil
	}
	if err := op.apply(session.BacklogItemDependencyEdge{BlockerID: blocker, BlockedID: blocked}); err != nil {
		return connectErrResult(op.tool, err), nil
	}
	return mcpgo.NewToolResultText(fmt.Sprintf("%s dependency: %s is blocked by %s.", op.verb, blocked, blocker)), nil
}

// --- trigger_triage ---

func (h *backlogHandlers) triggerTriage(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r, nil
	}
	if h.backlogSvc == nil {
		return errResult(ErrUnavailable, errBacklogSvcNotWired, ""), nil
	}
	args := req.GetArguments()
	itemID, bad := requireUUIDArg(args, "item_id")
	if bad != nil {
		return bad, nil
	}
	feedback, _ := args["feedback"].(string)
	if _, err := h.backlogSvc.TriggerTriage(ctx, connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId:   itemID,
		Feedback: feedback,
	})); err != nil {
		return connectErrResult("trigger_triage", err), nil
	}
	return mcpgo.NewToolResultText(fmt.Sprintf(
		"Triage started for %s. It runs asynchronously: use wait_for_backlog_event or get_backlog_item to observe the result.", itemID,
	)), nil
}

// --- transition_backlog_item_status ---

// transitionBacklogItemStatus deliberately omits the RPC's override_reason:
// an agent must not be able to bypass the status gates (review verdict,
// unresolved blockers) through this tool.
func (h *backlogHandlers) transitionBacklogItemStatus(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r, nil
	}
	if h.backlogSvc == nil {
		return errResult(ErrUnavailable, errBacklogSvcNotWired, ""), nil
	}
	args := req.GetArguments()
	itemID, bad := requireUUIDArg(args, "item_id")
	if bad != nil {
		return bad, nil
	}
	target, _ := args["target_status"].(string)
	if target == "" {
		return errResult(ErrInvalidArgument, "target_status is required", ""), nil
	}
	expected, _ := args["expected_status"].(string)
	if expected == "" {
		return errResult(ErrInvalidArgument, "expected_status is required", "Pass the status you last observed via get_backlog_item; the transition is rejected if the item has moved since."), nil
	}
	resp, err := h.backlogSvc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:         itemID,
		TargetStatus:   target,
		ExpectedStatus: expected,
	}))
	if err != nil {
		return connectErrResult("transition_backlog_item_status", err), nil
	}
	return mcpgo.NewToolResultText(fmt.Sprintf("Transitioned %s: %s -> %s.", itemID, expected, resp.Msg.GetItem().GetStatus())), nil
}

// parseDependsOn validates create_backlog_item's optional depends_on list and
// confirms each blocker exists, so a bad ID fails the call before any item is
// created rather than leaving a half-linked item behind.
func (h *backlogHandlers) parseDependsOn(ctx context.Context, args map[string]any) ([]string, *mcpgo.CallToolResult) {
	raw, ok := args["depends_on"].([]any)
	if !ok {
		return nil, nil
	}
	seen := make(map[string]bool, len(raw))
	var ids []string
	for _, v := range raw {
		id, _ := v.(string)
		if err := validateUUID(id); err != nil {
			return nil, errResult(ErrInvalidArgument, fmt.Sprintf("depends_on: %v", err), "depends_on takes backlog item UUIDs.")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err := h.storage.GetBacklogItem(ctx, id); err != nil {
			if errors.Is(err, session.ErrNotFound) {
				return nil, errResult(ErrItemNotFound, fmt.Sprintf("depends_on: backlog item %q not found", id), itemNotFoundRemediation)
			}
			return nil, errResult(ErrInternalError, fmt.Sprintf("depends_on: look up %s: %v", id, err), "")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func registerBacklogDependencyTools(s *mcpserver.MCPServer, h *backlogHandlers) {
	edgeTool := func(name, desc string) mcpgo.Tool {
		return mcpgo.NewTool(name,
			mcpgo.WithDescription(desc),
			mcpgo.WithString("blocker_item_id",
				mcpgo.Description("UUID of the prerequisite item that must reach done or archived first"),
				mcpgo.Required(),
			),
			mcpgo.WithString("blocked_item_id",
				mcpgo.Description("UUID of the dependent item that waits on the blocker"),
				mcpgo.Required(),
			),
		)
	}
	s.AddTool(edgeTool("add_backlog_item_dependency",
		"Make blocked_item_id wait on blocker_item_id: the dependent is never auto-started until the blocker is done, archived or deleted. "+
			"Rejects self-dependencies and cycles; adding an existing edge is a no-op. get_backlog_item shows the result under \"## Dependencies\"."),
		h.addBacklogItemDependency)
	s.AddTool(edgeTool("remove_backlog_item_dependency",
		"Remove a blocker->blocked dependency edge. Idempotent: removing an edge that does not exist succeeds."),
		h.removeBacklogItemDependency)

	s.AddTool(
		mcpgo.NewTool("trigger_triage",
			mcpgo.WithDescription("Start (or refine) triage for a backlog item in idea or ready status. Asynchronous: returns once triage has started; use wait_for_backlog_event or get_backlog_item for the outcome. The item needs an absolute repo_path."),
			mcpgo.WithString("item_id", mcpgo.Description("UUID of the backlog item"), mcpgo.Required()),
			mcpgo.WithString("feedback", mcpgo.Description("Optional: refine the most recent completed triage result with this feedback instead of a fresh run")),
		),
		h.triggerTriage,
	)

	s.AddTool(
		mcpgo.NewTool("transition_backlog_item_status",
			mcpgo.WithDescription("Move a backlog item to target_status, guarded by expected_status (compare-and-swap: rejected if the item is no longer in that status). "+
				"Normal gates still apply — e.g. starting work is refused while a blocker is unresolved, and done needs a passing review verdict. There is no override."),
			mcpgo.WithString("item_id", mcpgo.Description("UUID of the backlog item"), mcpgo.Required()),
			mcpgo.WithString("target_status", mcpgo.Description("Status to move to, e.g. queued, in_progress"), mcpgo.Required()),
			mcpgo.WithString("expected_status", mcpgo.Description("The status you last observed; guards against concurrent changes"), mcpgo.Required()),
		),
		h.transitionBacklogItemStatus,
	)
}
