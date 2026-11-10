package mcp

import (
	"context"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func (h *flowHandlers) listFlowDeadlockCandidates(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := h.guard(); r != nil {
		return r, nil
	}
	args := req.GetArguments()
	days, allTime, bad := flowWindowArgs(args)
	if bad != nil {
		return bad, nil
	}
	limit, _ := flowIntArg(args, "limit")
	resp, err := h.svc.ListDeadlockCandidates(ctx, connect.NewRequest(&sessionv1.ListDeadlockCandidatesRequest{
		WindowDays: days, AllTime: allTime, Limit: int32(max(limit, 0)),
	}))
	return flowReply(resp, err), nil
}

func (h *flowHandlers) listFlowThrashEpisodes(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := h.guard(); r != nil {
		return r, nil
	}
	args := req.GetArguments()
	days, allTime, bad := flowWindowArgs(args)
	if bad != nil {
		return bad, nil
	}
	limit, _ := flowIntArg(args, "limit")
	resp, err := h.svc.ListThrashEpisodes(ctx, connect.NewRequest(&sessionv1.ListThrashEpisodesRequest{
		WindowDays: days, AllTime: allTime, Limit: int32(max(limit, 0)),
	}))
	return flowReply(resp, err), nil
}

func (h *flowHandlers) getItemFlowTimeline(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := h.guard(); r != nil {
		return r, nil
	}
	args := req.GetArguments()
	itemID := flowStringArg(args, "item_id")
	if itemID == "" {
		return errResult(ErrInvalidArgument, "item_id is required", "Take an item_id from list_flow_thrash_episodes or list_flow_drilldown."), nil
	}
	days, allTime, bad := flowWindowArgs(args)
	if bad != nil {
		return bad, nil
	}
	resp, err := h.svc.GetItemFlowTimeline(ctx, connect.NewRequest(&sessionv1.GetItemFlowTimelineRequest{
		ItemId: itemID, WindowDays: days, AllTime: allTime,
	}))
	return flowReply(resp, err), nil
}

func limitOption() mcpgo.ToolOption {
	return mcpgo.WithNumber("limit", mcpgo.Description("List size (default 10, max 50)."), mcpgo.DefaultNumber(10), mcpgo.Min(1), mcpgo.Max(50))
}

func listFlowDeadlockCandidatesTool() mcpgo.Tool {
	opts := append([]mcpgo.ToolOption{
		mcpgo.WithDescription("List deadlock candidates: a heuristic ranked review-fail streak list (items whose reviewer kept failing the same criterion, ranked by run_length), not confirmed deadlocks. " +
			"Each row has sequence, resolution, run_length, repeated_criterion, diff_unchanged_pairs and agent_claimed_done; weak_filter repeats the caveat shown in the UI. " +
			"Top-N only (default 10, max 50, no offset): total_count/returned_count/has_more say whether more exist; page through the full set with list_flow_drilldown key review.deadlock. Use get_item_flow_timeline for one item's history. " +
			`Example: {"window_days":30,"limit":10}`),
	}, windowOptions()...)
	return withFlowDataWarning(mcpgo.NewTool("list_flow_deadlock_candidates", append(opts, limitOption())...))
}

func listFlowThrashEpisodesTool() mcpgo.Tool {
	opts := append([]mcpgo.ToolOption{
		mcpgo.WithDescription("List items that thrash between statuses (many rapid flips collapsed into episodes), worst first. Top-N only (default 10, max 50, no offset): total_count/returned_count/has_more so 10 rows are never mistaken for all of them; page through the full set with list_flow_drilldown key thrash.episode. " +
			"Open one with get_item_flow_timeline. " + `Example: {"window_days":30,"limit":10}`),
	}, windowOptions()...)
	return withFlowDataWarning(mcpgo.NewTool("list_flow_thrash_episodes", append(opts, limitOption())...))
}

func getItemFlowTimelineTool() mcpgo.Tool {
	opts := append([]mcpgo.ToolOption{
		mcpgo.WithDescription("One item's status timeline with rapid flips collapsed into thrash episodes (no raw event spam), at most 50 entries; total_entries is the full count and truncated says whether entries were cut. " +
			`Example: {"item_id":"<uuid from list_flow_thrash_episodes>","window_days":30}`),
		mcpgo.WithString("item_id", mcpgo.Description("Backlog item UUID."), mcpgo.Required()),
	}, windowOptions()...)
	return withFlowDataWarning(mcpgo.NewTool("get_item_flow_timeline", opts...))
}
