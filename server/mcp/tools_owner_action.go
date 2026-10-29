package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

const toolRequestOwnerAction = "request_owner_action"

const (
	maxOwnerActionResponses = 8
	maxOwnerActionQuestion  = 500
)

func mustJSONStrings(v []string) string {
	b, _ := json.Marshal(v) // []string always marshals
	return string(b)
}

func registerOwnerActionTools(s *mcpserver.MCPServer, h *peerHelpHandlers) {
	s.AddTool(mcpgo.NewTool(toolRequestOwnerAction,
		mcpgo.WithDescription("Ask the OWNER to take a decision or action only they can take on a backlog item you are linked to — "+
			"e.g. waive an acceptance criterion you cannot satisfy, supply evidence you cannot produce, approve or reject a direction. "+
			"The request is pinned as a banner on the item and in the owner's notifications and review queue until answered; you are woken "+
			"with the chosen response when the owner answers, so do not poll or idle on schedule_wake. "+
			"Use this INSTEAD of: create_guidance_request (a question whose answer you need to proceed but any answer is acceptable and no one is blocked on an owner-only authority); "+
			"request_help (a peer session, or a free-text question to the human); "+
			"report_blocked (an external blocker such as CI or a dependency — it escalates the item to review but gives the owner nothing to answer). "+
			"Choose this when the ONLY way forward is a choice among allowed_responses that the owner alone can make. "+
			"Returns a durable request_id; at most 4 requests may be pending per item."),
		mcpgo.WithString("item_id", mcpgo.Required(), mcpgo.Description("UUID of the backlog item this session is linked to.")),
		mcpgo.WithString("question", mcpgo.Required(), mcpgo.Description("A short ask: what you need and why you cannot proceed without it.")),
		mcpgo.WithArray("allowed_responses", mcpgo.Required(),
			mcpgo.Description("The responses the owner may pick, e.g. [\"record waiver\", \"provide evidence\", \"approve\", \"reject\"]."),
			mcpgo.Items(map[string]any{"type": "string"})),
	), withCallerIdentityGate(h.gate, toolRequestOwnerAction, h.requestOwnerAction))
}

// RequestOwnerActionResult is request_owner_action's success shape.
type RequestOwnerActionResult struct {
	MCPResult
	RequestID string `json:"request_id"`
	ItemID    string `json:"item_id"`
}

func (h *peerHelpHandlers) requestOwnerAction(ctx context.Context, caller string, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if scope := h.flags.toolOff(toolRequestOwnerAction); scope != "" {
		return flagOff(scope), nil
	}
	if scope := h.flags.triggerOff(domain.WakeTriggerGuidanceAnswered); scope != "" {
		return flagOff(scope), nil // without the answer wake the asker could only poll
	}
	deps := h.deps()
	if deps == nil || deps.Backend == nil {
		return typedErrorResult(CodeUnavailable, "", "the wake dispatcher is not running in this process", "Try again later."), nil
	}
	itemID, question, responses, errRes := parseOwnerActionArgs(req.GetArguments())
	if errRes != nil {
		return errRes, nil
	}
	if errRes := h.checkOwnerActionItem(ctx, caller, itemID); errRes != nil {
		return errRes, nil
	}
	id, _ := uuid.Parse(itemID)
	row, err := h.storage.CreateGuidanceRequest(ctx, session.CreateGuidanceRequestInput{
		Scope:        domain.RequestScopeBacklogItem,
		ItemID:       &id,
		QuestionText: domain.OwnerActionQuestionPrefix + question,
		QuestionType: domain.QuestionTypeMultipleChoice,
		Options:      mustJSONStrings(responses),
	})
	if err != nil {
		if errors.Is(err, session.ErrPendingCapExceeded) {
			return errResult(ErrPendingCapExceeded, err.Error(),
				"Wait for an existing request on this item to be answered; you will be woken when it is."), nil
		}
		return errResult(ErrInternalError, fmt.Sprintf("create owner-action request: %v", err), ""), nil
	}
	if err := h.scheduleAnswerWake(ctx, deps, caller, row.ID); err != nil {
		h.abort(ctx, row.ID)
		return scheduleErrorResult(err), nil
	}
	h.notifyOwnerAction(itemID, row.ID, question)
	return okResult(RequestOwnerActionResult{MCPResult: MCPResult{Success: true}, RequestID: row.ID, ItemID: itemID}), nil
}

func parseOwnerActionArgs(args map[string]any) (itemID, question string, responses []string, errRes *mcpgo.CallToolResult) {
	itemID, question = stringArg(args, "item_id"), strings.TrimSpace(stringArg(args, "question"))
	if err := validateUUID(itemID); err != nil {
		return "", "", nil, errResult(ErrInvalidArgument, err.Error(), "")
	}
	if question == "" || len([]rune(question)) > maxOwnerActionQuestion {
		return "", "", nil, errResult(ErrInvalidArgument, fmt.Sprintf("question is required and at most %d characters", maxOwnerActionQuestion), "")
	}
	raw, _ := args["allowed_responses"].([]any)
	seen := map[string]bool{}
	for _, r := range raw {
		if s, ok := r.(string); ok && strings.TrimSpace(s) != "" && !seen[strings.TrimSpace(s)] {
			seen[strings.TrimSpace(s)] = true
			responses = append(responses, strings.TrimSpace(s))
		}
	}
	if len(responses) == 0 || len(responses) > maxOwnerActionResponses {
		return "", "", nil, errResult(ErrInvalidArgument, fmt.Sprintf("allowed_responses needs 1-%d distinct non-empty strings", maxOwnerActionResponses), "")
	}
	return itemID, question, responses, nil
}

// checkOwnerActionItem rejects a caller that is not linked to the item, or an archived item.
func (h *peerHelpHandlers) checkOwnerActionItem(ctx context.Context, caller, itemID string) *mcpgo.CallToolResult {
	if _, linkErr := session.ResolveItemLink(ctx, h.storage, caller, itemID); linkErr != nil {
		switch linkErr.Code {
		case session.ItemLinkNotFound:
			return errResult(ErrItemNotFound, linkErr.Message, linkErr.Remediation)
		case session.ItemLinkPermissionDenied:
			return errResult(ErrPermissionDenied, linkErr.Message, linkErr.Remediation)
		default:
			return errResult(ErrInternalError, linkErr.Message, "")
		}
	}
	item, err := h.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("load item: %v", err), "")
	}
	if item.ArchivedAt != nil {
		return errResult(ErrPermissionDenied, "item is archived; no owner action can be requested on it", "")
	}
	return nil
}

// notifyOwnerAction pushes an actionable notification so the request reaches the
// owner's "Needs a decision" tier; best-effort and a no-op without an event bus.
func (h *peerHelpHandlers) notifyOwnerAction(itemID, requestID, question string) {
	if h.bus == nil {
		return
	}
	h.bus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
		"Owner action requested", question,
		map[string]string{"item_id": itemID, "guidance_request_id": requestID},
	))
}
