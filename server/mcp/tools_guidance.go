package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ErrPendingCapExceeded is create_guidance_request's error code for a
// scope already at its per-(scope, scope_key) pending-request cap — visible
// and agent-readable per AC7, not silently swallowed.
const ErrPendingCapExceeded = "PENDING_CAP_EXCEEDED"

// GuidanceRequestResult is the JSON shape returned by
// create_guidance_request/get_guidance_request.
type GuidanceRequestResult struct {
	MCPResult
	ID           string   `json:"id"`
	Scope        string   `json:"scope"`
	ItemID       string   `json:"item_id,omitempty"`
	SessionUUID  string   `json:"session_uuid,omitempty"`
	QuestionText string   `json:"question_text"`
	QuestionType string   `json:"question_type"`
	Options      []string `json:"options,omitempty"`
	Answer       string   `json:"answer,omitempty"`
	Status       string   `json:"status"`
}

// guidanceRequestToResult converts the repository's plain data view into the
// MCP tool result shape.
func guidanceRequestToResult(d *session.GuidanceRequestData) GuidanceRequestResult {
	res := GuidanceRequestResult{
		MCPResult:    MCPResult{Success: true},
		ID:           d.ID,
		Scope:        string(d.Scope),
		SessionUUID:  d.SessionUUID,
		QuestionText: d.QuestionText,
		QuestionType: string(d.QuestionType),
		Answer:       d.Answer,
		Status:       string(d.Status()),
	}
	if d.ItemID != nil {
		res.ItemID = d.ItemID.String()
	}
	if d.Options != "" {
		var opts []string
		if err := json.Unmarshal([]byte(d.Options), &opts); err == nil {
			res.Options = opts
		}
	}
	return res
}

// parseGuidanceOptionsArg converts create_guidance_request's raw "options"
// argument (a JSON array decoded to []any by the MCP framework) into the
// JSON-encoded string session.CreateGuidanceRequestInput.Options expects.
func parseGuidanceOptionsArg(args map[string]any) string {
	rawOpts, ok := args["options"].([]any)
	if !ok || len(rawOpts) == 0 {
		return ""
	}
	opts := make([]string, 0, len(rawOpts))
	for _, o := range rawOpts {
		if s, ok := o.(string); ok {
			opts = append(opts, s)
		}
	}
	if len(opts) == 0 {
		return ""
	}
	b, err := json.Marshal(opts)
	if err != nil {
		return ""
	}
	return string(b)
}

// applyGuidanceRequestScopeFields applies the scope-appropriate ownership
// check and fills in.ItemID/in.SessionUUID, split out of
// createGuidanceRequest to keep that handler under the house
// function-length gate. Returns a non-nil *mcpgo.CallToolResult only on a
// rejection (ownership failure or bad item_id); otherwise in is mutated
// in place and the returned result is nil.
func (h *backlogHandlers) applyGuidanceRequestScopeFields(ctx context.Context, args map[string]any, callerUUID string, in *session.CreateGuidanceRequestInput) *mcpgo.CallToolResult {
	switch in.Scope {
	case domain.RequestScopeBacklogItem:
		itemID, _ := args["item_id"].(string)
		if itemID == "" {
			return errResult(ErrInvalidArgument, "item_id is required for scope=backlog-item", "")
		}
		if err := validateUUID(itemID); err != nil {
			return errResult(ErrInvalidArgument, err.Error(), "")
		}
		if _, errRes := h.resolveItemLink(ctx, callerUUID, itemID); errRes != nil {
			return errRes
		}
		id, err := uuid.Parse(itemID)
		if err != nil {
			return errResult(ErrInvalidArgument, err.Error(), "")
		}
		in.ItemID = &id
	case domain.RequestScopeSession:
		in.SessionUUID = callerUUID
	case domain.RequestScopeStandalone:
		// No ownership check — human-only surface.
	}
	return nil
}

// createGuidanceRequest implements the create_guidance_request tool: asks
// the human a structured question durably. For scope=backlog-item, reuses
// h.resolveItemLink (via applyGuidanceRequestScopeFields) — the same
// ownership check every other mutating backlog tool applies (AC6) — before
// creating.
func (h *backlogHandlers) createGuidanceRequest(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r, nil
	}
	callerUUID, err := callerSessionUUID(ctx)
	if err != nil {
		return errResult(ErrPermissionDenied, err.Error(), "Set STAPLER_SESSION_UUID in your environment."), nil
	}

	args := req.GetArguments()
	scope := domain.RequestScope(fmt.Sprint(args["scope"]))
	if !scope.IsValid() {
		return errResult(ErrInvalidArgument, fmt.Sprintf("invalid scope %q", args["scope"]), "scope must be one of: backlog-item, session, standalone"), nil
	}
	questionText, _ := args["question_text"].(string)
	if questionText == "" {
		return errResult(ErrInvalidArgument, "question_text is required", ""), nil
	}
	questionType := domain.QuestionType(fmt.Sprint(args["question_type"]))
	if !questionType.IsValid() {
		return errResult(ErrInvalidArgument, fmt.Sprintf("invalid question_type %q", args["question_type"]), "question_type must be one of: yes-no, multiple-choice, short-answer"), nil
	}

	in := session.CreateGuidanceRequestInput{
		Scope:        scope,
		QuestionText: questionText,
		QuestionType: questionType,
		Options:      parseGuidanceOptionsArg(args),
		// TODO(phase 4): thread through config.GetGuidanceRequestPendingCap()
		// once it lands; 0 falls back to session.DefaultGuidanceRequestPendingCap.
	}
	if errRes := h.applyGuidanceRequestScopeFields(ctx, args, callerUUID, &in); errRes != nil {
		return errRes, nil
	}

	data, err := h.storage.CreateGuidanceRequest(ctx, in)
	if err != nil {
		if errors.Is(err, session.ErrPendingCapExceeded) {
			return errResult(ErrPendingCapExceeded, err.Error(),
				"Too many pending guidance requests for this scope — wait for an existing one to be answered, or re-ask the identical question text to dedup onto it instead of creating a new one."), nil
		}
		return errResult(ErrInternalError, fmt.Sprintf("create guidance request: %v", err), ""), nil
	}
	return okResult(guidanceRequestToResult(data)), nil
}

// getGuidanceRequest implements the get_guidance_request tool: reads a
// guidance request's current state by id. No ownership re-check on read —
// matches this codebase's low-security-classification posture for reads
// (mirrors get_backlog_item/wait_for_backlog_event, which likewise don't
// re-verify the caller's link on every read).
func (h *backlogHandlers) getGuidanceRequest(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r, nil
	}
	args := req.GetArguments()
	idStr, _ := args["question_id"].(string)
	if idStr == "" {
		return errResult(ErrInvalidArgument, "question_id is required", ""), nil
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return errResult(ErrInvalidArgument, fmt.Sprintf("invalid question_id: %v", err), ""), nil
	}
	data, err := h.storage.GetGuidanceRequest(ctx, id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return errResult(ErrItemNotFound, fmt.Sprintf("guidance request %q not found", idStr), ""), nil
		}
		return errResult(ErrInternalError, fmt.Sprintf("get guidance request: %v", err), ""), nil
	}
	return okResult(guidanceRequestToResult(data)), nil
}

// registerGuidanceTools registers create_guidance_request/get_guidance_request
// on the server, sibling to registerBacklogTools.
func registerGuidanceTools(s *mcpserver.MCPServer, h *backlogHandlers) {
	s.AddTool(
		mcpgo.NewTool("create_guidance_request",
			mcpgo.WithDescription("Ask the human a structured question durably — the request survives session restarts/respawns. Poll or re-check via get_guidance_request for the answer once notified/resumed. Use scope=backlog-item when the question concerns a specific backlog item this session is linked to (see link_session_to_item), scope=session when it concerns only this session's own work, or scope=standalone otherwise. Re-asking the identical question_text for the same scope resolves to the existing open request instead of creating a duplicate."),
			mcpgo.WithString("scope",
				mcpgo.Required(),
				mcpgo.Enum("backlog-item", "session", "standalone"),
			),
			mcpgo.WithString("item_id",
				mcpgo.Description("UUID of the backlog item; required when scope=backlog-item, and this session must already be linked to it."),
			),
			mcpgo.WithString("question_text",
				mcpgo.Required(),
			),
			mcpgo.WithString("question_type",
				mcpgo.Required(),
				mcpgo.Enum("yes-no", "multiple-choice", "short-answer"),
			),
			mcpgo.WithArray("options",
				mcpgo.Description("Choice strings; only meaningful when question_type=multiple-choice."),
				mcpgo.Items(map[string]any{"type": "string"}),
			),
		),
		h.createGuidanceRequest,
	)

	s.AddTool(
		mcpgo.NewTool("get_guidance_request",
			mcpgo.WithDescription("Read a guidance request's current state (pending/answered/cancelled) by id — e.g. after a respawn, to check whether the human has answered yet."),
			mcpgo.WithString("question_id",
				mcpgo.Required(),
			),
		),
		h.getGuidanceRequest,
	)
}
