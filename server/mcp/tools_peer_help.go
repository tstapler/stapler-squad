package mcp

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// peerHuman is request_help's alias for asking the operator.
const peerHuman = "human"

// Reasons of a call-time `rejected` result: none of them creates a row.
const (
	rejectSelfTarget     = "self_target"
	rejectPeerNotFound   = "peer_not_found"
	rejectPeerArchived   = "peer_archived"
	rejectPeerOneShot    = "peer_one_shot"
	rejectPeerItemDone   = "peer_item_terminal"
	rejectCrossWorkspace = "cross_workspace"
)

// Peer-help states counted in peer_help_total.
const (
	peerStateWaiting  = "waiting"
	peerStateAnswered = "answered"
	peerStateRejected = "rejected"
	peerStateNotSent  = "not_sent"
)

// helpAbortReason is stored when a just-created request is withdrawn because
// its asker's answer wake could not be scheduled.
const helpAbortReason = "request_aborted"

// peerHelpHandlers backs request_help and answer_help.
type peerHelpHandlers struct {
	gate    *callerIdentityGate
	flags   wakeFlags
	deps    func() *WakeToolDeps
	storage *session.Storage
	now     func() time.Time
}

func newPeerHelpHandlers(gate *callerIdentityGate, storage *session.Storage) *peerHelpHandlers {
	return &peerHelpHandlers{gate: gate, deps: currentWakeToolDeps, storage: storage, now: time.Now}
}

func registerPeerHelpTools(s *mcpserver.MCPServer, h *peerHelpHandlers) {
	s.AddTool(mcpgo.NewTool(toolRequestHelp,
		mcpgo.WithDescription("Ask another session in your workspace, or the human (peer=human), for help without polling. You are woken when it is answered. A peer must be a live or stopped, non-archived, non-one-shot session of the same repository; a refused request returns a `rejected` result with the reason and creates nothing."),
		mcpgo.WithString("peer", mcpgo.Required(), mcpgo.Description("Session UUID of the peer, or \"human\".")),
		mcpgo.WithString("question", mcpgo.Required()),
	), withCallerIdentityGate(h.gate, toolRequestHelp, h.requestHelp))

	s.AddTool(mcpgo.NewTool(toolAnswerHelp,
		mcpgo.WithDescription("Answer a help request addressed to you. Only the session the request names can answer it."),
		mcpgo.WithString("request_id", mcpgo.Required()),
		mcpgo.WithString("answer", mcpgo.Required()),
	), withCallerIdentityGate(h.gate, toolAnswerHelp, h.answerHelp))
}

// RequestHelpResult is request_help's success shape.
type RequestHelpResult struct {
	MCPResult
	RequestID string `json:"request_id"`
	Peer      string `json:"peer"`
	State     string `json:"state"`
	// Delivery is "sent" when the peer was prompted, "not_sent" when the request
	// stands but the prompt could not be queued (the operator is told).
	Delivery       string `json:"delivery,omitempty"`
	DeliveryReason string `json:"delivery_reason,omitempty"`
}

func (h *peerHelpHandlers) requestHelp(ctx context.Context, caller string, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	deps := h.deps()
	if scope := h.flags.toolOff(toolRequestHelp); scope != "" {
		return flagOff(scope), nil
	}
	if scope := h.flags.triggerOff(domain.WakeTriggerGuidanceAnswered); scope != "" {
		return flagOff(scope), nil // without the answer wake the asker could only poll
	}
	if deps == nil || deps.Backend == nil || deps.Owners == nil {
		return typedErrorResult(CodeUnavailable, "", "the wake dispatcher is not running in this process", "Try again later."), nil
	}
	args := req.GetArguments()
	peer, question := stringArg(args, "peer"), stringArg(args, "question")
	if peer == "" || question == "" {
		return errResult(ErrInvalidArgument, "peer and question are required", ""), nil
	}
	if n := utf8.RuneCountInString(question); n > deps.Backend.Limits().PayloadChars {
		return errResult(ErrInvalidArgument, fmt.Sprintf("question is %d characters, the limit is %d", n, deps.Backend.Limits().PayloadChars), ""), nil
	}
	if peer != peerHuman {
		if rejection := h.rejectPeer(ctx, deps, caller, peer); rejection != nil {
			h.gate.metrics.Inc(wakes.MetricPeerHelp, peerStateRejected)
			return rejection, nil
		}
	}
	row, err := h.createRequest(ctx, deps, caller, peer, question)
	if err != nil {
		return h.createErrorResult(err), nil
	}
	if err := h.scheduleAnswerWake(ctx, deps, caller, row.ID); err != nil {
		h.abort(ctx, row.ID)
		return scheduleErrorResult(err), nil
	}
	out := RequestHelpResult{MCPResult: MCPResult{Success: true}, RequestID: row.ID, Peer: peer, State: peerStateWaiting}
	if peer != peerHuman {
		out.Delivery, out.DeliveryReason = h.promptPeer(ctx, deps, caller, peer, row.ID, question)
	}
	return okResult(out), nil
}

// rejectPeer returns the typed `rejected` result for a peer that cannot be
// asked, or nil. It reads the peer's state now; the request row does not exist yet.
func (h *peerHelpHandlers) rejectPeer(ctx context.Context, deps *WakeToolDeps, caller, peer string) *mcpgo.CallToolResult {
	reject := func(reason, msg string) *mcpgo.CallToolResult {
		return typedErrorResult(CodeRejected, reason, msg, "")
	}
	if peer == caller {
		return reject(rejectSelfTarget, "a session cannot ask itself for help")
	}
	facts, err := deps.Owners.Owner(ctx, peer)
	if err != nil {
		return errResult(ErrInternalError, err.Error(), "")
	}
	asker, err := deps.Owners.Owner(ctx, caller)
	if err != nil {
		return errResult(ErrInternalError, err.Error(), "")
	}
	switch {
	case !facts.Exists:
		return reject(rejectPeerNotFound, "no such session")
	case facts.Archived:
		return reject(rejectPeerArchived, "that session is archived")
	case facts.OneShot:
		return reject(rejectPeerOneShot, "that is a one-shot review or triage session")
	case facts.ItemTerminal:
		return reject(rejectPeerItemDone, "that session's backlog item is finished")
	case facts.WorkspaceKey == "" || facts.WorkspaceKey != asker.WorkspaceKey:
		return reject(rejectCrossWorkspace, "that session works in a different repository")
	}
	return nil
}

func (h *peerHelpHandlers) createRequest(ctx context.Context, deps *WakeToolDeps, caller, peer, question string) (*session.GuidanceRequestData, error) {
	if peer == peerHuman {
		return h.storage.CreateGuidanceRequest(ctx, session.CreateGuidanceRequestInput{
			Scope: domain.RequestScopeSession, SessionUUID: caller, QuestionText: question, QuestionType: domain.QuestionTypeShortAnswer,
		})
	}
	req, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: caller, QuestionText: question, QuestionType: domain.QuestionTypeShortAnswer,
		Source: domain.QuestionSourcePeer, AskerSessionUUID: caller,
		AnswererKind: domain.AnswererKindSession, AnswererSessionUUID: peer,
	})
	if err != nil {
		return nil, err
	}
	return h.storage.CreateMirroredGuidanceRequest(ctx, req, deps.Backend.Limits().PeerOpenPerAsker)
}

func (h *peerHelpHandlers) createErrorResult(err error) *mcpgo.CallToolResult {
	if errors.Is(err, session.ErrPendingCapExceeded) {
		h.gate.metrics.Inc(wakes.MetricLimitHit, wakes.LimitPeerOpenPerAsker, wakes.PhaseCreate)
		return rawTypedError(typedError{
			MCPError: MCPError{Code: CodeCapExceeded, Message: err.Error(), Remediation: "Wait for an open request to be answered, or cancel it."},
			Limit:    wakes.LimitPeerOpenPerAsker,
		})
	}
	return errResult(ErrInternalError, err.Error(), "")
}

// scheduleAnswerWake arms the asker's implicit guidance_answered wake, so it is
// prompted when the request is answered instead of polling.
func (h *peerHelpHandlers) scheduleAnswerWake(ctx context.Context, deps *WakeToolDeps, asker, requestID string) error {
	action, err := domain.NewWakeAction(domain.WakeActionSteerText, "Your help request "+requestID+" has been answered. Read the answer below and continue.")
	if err != nil {
		return err
	}
	gen, err := deps.Backend.NextGeneration(ctx, asker)
	if err != nil {
		return err
	}
	expires := h.now().Add(domain.PeerHelpTTL)
	_, _, err = deps.Backend.Schedule(ctx, wakes.ScheduleRequest{
		Owner: asker, Spec: domain.GuidanceAnsweredSpec{GuidanceRequestID: requestID}, Action: action,
		ExpiresAt: &expires, Generation: gen,
	})
	return err
}

// abort withdraws a request whose answer wake could not be armed, so the
// refusal leaves no half-created request behind.
func (h *peerHelpHandlers) abort(ctx context.Context, requestID string) {
	id, err := uuid.Parse(requestID)
	if err == nil {
		_, err = h.storage.CancelGuidanceRequest(ctx, id, helpAbortReason)
	}
	if err != nil {
		log.Warn("request_help: could not withdraw request after wake refusal", "request_id", requestID, "error", err)
	}
}

// promptPeer queues the request as a prompt for the peer through the
// dispatcher, which writes only into an idle pane and never revives an
// archived session. It reports how that went.
func (h *peerHelpHandlers) promptPeer(ctx context.Context, deps *WakeToolDeps, asker, peer, requestID, question string) (delivery, reason string) {
	text := fmt.Sprintf("Help request %s from session %s:\n%s\nReply with answer_help(request_id=%q, answer=...).", requestID, asker, question, requestID)
	action, err := domain.NewWakeAction(domain.WakeActionSteerText, text)
	if err == nil {
		_, _, err = deps.Backend.Schedule(ctx, wakes.ScheduleRequest{
			Owner: peer, Spec: domain.AtSpec{At: h.now()}, Action: action,
		})
	}
	if err != nil {
		h.gate.metrics.Inc(wakes.MetricPeerHelp, peerStateNotSent)
		log.Warn("request_help: could not queue the prompt for the peer", "request_id", requestID, "peer", peer, "error", err)
		return "not_sent", err.Error()
	}
	return "sent", ""
}

// AnswerHelpResult is answer_help's success shape.
type AnswerHelpResult struct {
	MCPResult
	RequestID string `json:"request_id"`
	// Applied is false when the request was already answered or cancelled.
	Applied bool   `json:"applied"`
	Status  string `json:"status"`
	Answer  string `json:"answer,omitempty"`
}

func (h *peerHelpHandlers) answerHelp(ctx context.Context, caller string, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if scope := h.flags.toolOff(toolAnswerHelp); scope != "" {
		return flagOff(scope), nil
	}
	deps := h.deps()
	if deps == nil || deps.Answerer == nil || deps.Backend == nil {
		return typedErrorResult(CodeUnavailable, "", "answering is not available in this process", ""), nil
	}
	args := req.GetArguments()
	id, answer := stringArg(args, "request_id"), stringArg(args, "answer")
	if id == "" || answer == "" {
		return errResult(ErrInvalidArgument, "request_id and answer are required", ""), nil
	}
	if n := utf8.RuneCountInString(answer); n > deps.Backend.Limits().PayloadChars {
		return errResult(ErrInvalidArgument, fmt.Sprintf("answer is %d characters, the limit is %d", n, deps.Backend.Limits().PayloadChars), ""), nil
	}
	data, applied, err := deps.Answerer.AnswerAs(ctx, id, answer, services.SessionAnswerer(caller))
	switch {
	case errors.Is(err, services.ErrAnswererForbidden):
		return typedErrorResult(CodeForbidden, "", "this request is not addressed to your session", ""), nil
	case errors.Is(err, session.ErrNotFound):
		return errResult(ErrItemNotFound, "no such help request", ""), nil
	case err != nil:
		return errResult(ErrInternalError, err.Error(), ""), nil
	}
	if applied {
		h.gate.metrics.Inc(wakes.MetricPeerHelp, peerStateAnswered)
	}
	return okResult(AnswerHelpResult{
		MCPResult: MCPResult{Success: true}, RequestID: data.ID, Applied: applied, Status: string(data.Status()), Answer: data.Answer,
	}), nil
}
