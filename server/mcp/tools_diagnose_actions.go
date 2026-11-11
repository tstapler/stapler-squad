package mcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

const (
	maxWorkflowRecommendationLen = 500
	harnessGapSeverityDefault    = "medium"
)

var harnessGapSeverities = []string{"low", "medium", "high"}

func registerSuggestWorkflowActionTool(s *mcpserver.MCPServer, dh *diagnoseHandlers) {
	s.AddTool(
		mcpgo.NewTool("suggest_workflow_action",
			mcpgo.WithDescription("Post a structured recommendation for the operator on the backlog item (e.g. \"Move item back to ready — stall resolved\"). It renders as a dismissible card on the item; nothing is applied automatically. Role: diagnose only."),
			mcpgo.WithString("item_id", mcpgo.Description("UUID of the backlog item this diagnostic session was dispatched for"), mcpgo.Required()),
			mcpgo.WithString("recommendation", mcpgo.Description("What the operator should do and why (max 500 chars)"), mcpgo.Required()),
			mcpgo.WithString("action_type", mcpgo.Description("Kind of action recommended"), mcpgo.Enum(session.WorkflowSuggestionActionTypes...), mcpgo.Required()),
		),
		dh.suggestWorkflowAction,
	)
}

func (dh *diagnoseHandlers) suggestWorkflowAction(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	callerUUID, err := callerSessionUUID(ctx)
	if err != nil {
		return errResult(ErrPermissionDenied, err.Error(), "Set STAPLER_SESSION_UUID in your environment."), nil
	}
	args := req.GetArguments()
	itemID, _ := args["item_id"].(string)
	if err := validateUUID(itemID); err != nil {
		return errResult(ErrInvalidArgument, err.Error(), ""), nil
	}
	recommendation, _ := args["recommendation"].(string)
	recommendation = strings.TrimSpace(recommendation)
	if recommendation == "" || len(recommendation) > maxWorkflowRecommendationLen {
		return errResult(ErrInvalidArgument, fmt.Sprintf("recommendation is required and must be at most %d chars", maxWorkflowRecommendationLen), ""), nil
	}
	actionType, _ := args["action_type"].(string)
	if !slices.Contains(session.WorkflowSuggestionActionTypes, actionType) {
		return errResult(ErrInvalidArgument, fmt.Sprintf("action_type must be one of %v", session.WorkflowSuggestionActionTypes), ""), nil
	}
	if _, errRes := resolveDiagnoseItemLink(ctx, dh.storage, callerUUID, itemID); errRes != nil {
		return errRes, nil
	}
	id, appendErr := dh.storage.AppendWorkflowSuggestion(ctx, itemID, session.WorkflowSuggestionData{
		ActionType: actionType, Recommendation: recommendation, SessionUUID: callerUUID,
	})
	if appendErr != nil {
		return errResult(ErrInternalError, fmt.Sprintf("record workflow suggestion: %v", appendErr), ""), nil
	}
	return mcpgo.NewToolResultText(fmt.Sprintf("Workflow suggestion %s recorded for item %s", id, itemID)), nil
}

func registerReportHarnessGapTool(s *mcpserver.MCPServer, dh *diagnoseHandlers) {
	s.AddTool(
		mcpgo.NewTool("report_harness_gap",
			mcpgo.WithDescription("File a bug about a gap in the stapler-squad harness itself (not the item's own code). Creates a backlog item tagged harness:bug and auto-diagnose, blocked by the originating item, with the evidence bundle in its description. Available to any session."),
			mcpgo.WithString("item_id", mcpgo.Description("UUID of the originating backlog item"), mcpgo.Required()),
			mcpgo.WithString("title", mcpgo.Description("Short bug title"), mcpgo.Required()),
			mcpgo.WithString("evidence", mcpgo.Description("What you observed, with the evidence that shows the gap"), mcpgo.Required()),
			mcpgo.WithString("severity", mcpgo.Description("low | medium | high (default medium)"), mcpgo.Enum(harnessGapSeverities...)),
		),
		dh.reportHarnessGap,
	)
}

func (dh *diagnoseHandlers) reportHarnessGap(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	originID, _ := args["item_id"].(string)
	if err := validateUUID(originID); err != nil {
		return errResult(ErrInvalidArgument, err.Error(), ""), nil
	}
	title, _ := args["title"].(string)
	evidence, _ := args["evidence"].(string)
	if strings.TrimSpace(title) == "" || strings.TrimSpace(evidence) == "" {
		return errResult(ErrInvalidArgument, "title and evidence are required", ""), nil
	}
	severity, _ := args["severity"].(string)
	if severity == "" {
		severity = harnessGapSeverityDefault
	}
	if !slices.Contains(harnessGapSeverities, severity) {
		return errResult(ErrInvalidArgument, fmt.Sprintf("severity must be one of %v", harnessGapSeverities), ""), nil
	}
	origin, getErr := dh.storage.GetBacklogItem(ctx, originID)
	if getErr != nil {
		return errResult(ErrItemNotFound, fmt.Sprintf("originating item %s: %v", originID, getErr), ""), nil
	}
	callerUUID := callerSessionUUIDForAudit(ctx)
	created, createErr := dh.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:       title,
		Description: harnessGapDescription(evidence, severity, callerUUID, origin),
		Priority:    session.DefaultBacklogPriority,
		Status:      string(session.BacklogStatusIdea),
		RepoPath:    origin.RepoPath,
		Category:    string(session.BacklogCategoryBugfix),
		Labels:      []string{"harness:bug", "auto-diagnose"},
	})
	if createErr != nil {
		return errResult(ErrInternalError, fmt.Sprintf("create harness-gap item: %v", createErr), ""), nil
	}
	linkNote := ""
	if depErr := dh.storage.AddBacklogItemDependency(ctx, session.BacklogItemDependencyEdge{BlockerID: origin.ID, BlockedID: created.ID}); depErr != nil {
		log.WarningLog().Printf("[mcp:report_harness_gap] link %s blocked_by %s: %v", created.ID, origin.ID, depErr)
		linkNote = fmt.Sprintf(" (warning: could not link blocked_by %s: %v)", origin.ID, depErr)
	}
	log.InfoLog().Printf("[mcp:report_harness_gap] session=%s origin=%s item=%s", callerUUID, origin.ID, created.ID)
	return mcpgo.NewToolResultText(fmt.Sprintf("Filed harness gap as item %s%s", created.ID, linkNote)), nil
}

func harnessGapDescription(evidence, severity, sessionUUID string, origin *session.BacklogItemData) string {
	return fmt.Sprintf("%s\n\n## Evidence\n- Severity: %s\n- Originating item: %s (%s)\n- Reporting session: %s\n- Outcome/summary: see the originating item's diagnosis result\n",
		evidence, severity, origin.ID, origin.Title, sessionUUID)
}
