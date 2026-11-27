package notifyactions

import (
	"context"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/approvals"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
)

// notification_action_adapters.go binds the dispatcher's consumer-side
// interfaces to the existing handlers. Each adapter only maps a delegate's
// result onto an ActionOutcome; the delegate keeps its own checks.

// approvalServiceAdapter resolves approvals through ApprovalService, which keeps
// the CI-red guard, notification stamping and event broadcast.
type approvalServiceAdapter struct {
	svc   *approvals.ApprovalService
	store *approvalkit.ApprovalStore
}

// NewApprovalResolver adapts ApprovalService to ApprovalResolver.
func NewApprovalResolver(svc *approvals.ApprovalService, store *approvalkit.ApprovalStore) ApprovalResolver {
	return approvalServiceAdapter{svc: svc, store: store}
}

func (a approvalServiceAdapter) ResolveApproval(ctx context.Context, c ApprovalChoice) (questionkit.ActionResult, error) {
	decision := "deny"
	if c.Allow {
		decision = "allow"
	}
	_, err := a.svc.ResolveAsHuman(ctx, c.ApprovalID, decision, c.Reason, c.Override)
	if err == nil {
		return questionkit.Applied(), nil
	}
	switch connect.CodeOf(err) {
	case connect.CodeNotFound:
		return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, a.svc.StampedDecision(c.ApprovalID)), nil
	case connect.CodeFailedPrecondition:
		// Still pending: the CI-red guard blocked it ("Approve anyway" needs confirm).
		// Gone: a rule auto-resolved it while the operator looked.
		if _, pending := a.store.Get(c.ApprovalID); pending {
			return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
		}
		return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, a.svc.StampedDecision(c.ApprovalID)), nil
	case connect.CodeInvalidArgument, connect.CodeAborted:
		return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	}
	return questionkit.ActionResult{}, err
}

// guidanceServiceAdapter answers through GuidanceRequestService.
type guidanceServiceAdapter struct {
	svc *guidance.GuidanceRequestService
}

// NewGuidanceAnswerer adapts GuidanceRequestService to GuidanceAnswerer.
func NewGuidanceAnswerer(svc *guidance.GuidanceRequestService) GuidanceAnswerer {
	return guidanceServiceAdapter{svc}
}

func (a guidanceServiceAdapter) AnswerGuidance(ctx context.Context, requestID, answer string) (questionkit.ActionResult, error) {
	resp, err := a.svc.AnswerGuidanceRequest(ctx, connect.NewRequest(&sessionv1.AnswerGuidanceRequestRequest{Id: requestID, Answer: answer}))
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeNotFound:
			return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, ""), nil
		case connect.CodeInvalidArgument:
			return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
		}
		return questionkit.ActionResult{}, err
	}
	if !resp.Msg.GetApplied() {
		return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, resp.Msg.GetRequest().GetAnswer()), nil
	}
	return questionkit.Applied(), nil
}
