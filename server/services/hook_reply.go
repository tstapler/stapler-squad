package services

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// hook_reply.go is ADR-005 rung 1: when an AskUserQuestion PreToolUse hook fires
// for a question the operator already answered, the hook returns allow plus
// updatedInput.answers and Claude never draws the dialog. It is CLOSED until
// spike S1 is recorded as reviewed (spikeS1HookReplyPassed): every entry point
// asks flags.Answer(routeHookReply), which composes that constant.
//
// The wire shape is the one spike S1 measured: updatedInput echoes the
// questions unchanged and adds answers keyed by the full question text.

// HookReplier answers an AskUserQuestion PreToolUse hook, or returns nil so the
// native dialog stays. sessionUUID is the Stapler Squad session the hook runs in.
type HookReplier interface {
	Reply(ctx context.Context, sessionUUID string, toolInput map[string]interface{}) *hookipc.HookOutput
}

type hookReplyRows interface {
	ListGuidanceByDelivery(ctx context.Context, route domain.DeliveryRoute, state domain.DeliveryState) ([]*session.GuidanceRequestData, error)
	AnswerGuidanceRequest(ctx context.Context, id uuid.UUID, answer string) (bool, error)
	SetGuidanceDelivery(ctx context.Context, id uuid.UUID, route domain.DeliveryRoute, state domain.DeliveryState, addAttempts int) (bool, error)
}

type hookReplyResolver struct {
	rows  hookReplyRows
	flags actionFlags
	acks  deliverySink
}

// PreAnswer records answer on a still-open single-question row for a hook to
// serve later (a peer or agent answer that arrives before the dialog is asked).
// It reports false when the row was already answered or cancelled.
func (r *hookReplyResolver) PreAnswer(ctx context.Context, rowID, answer string) (bool, error) {
	id, err := uuid.Parse(rowID)
	if err != nil {
		return false, err
	}
	stored, err := r.rows.AnswerGuidanceRequest(ctx, id, answer)
	if err != nil || !stored {
		return false, err
	}
	return r.rows.SetGuidanceDelivery(ctx, id, domain.DeliveryRouteHookReply, domain.DeliveryStatePending, 0)
}

// Reply serves the oldest matching pre-answered row of the session.
func (r *hookReplyResolver) Reply(ctx context.Context, sessionUUID string, toolInput map[string]interface{}) *hookipc.HookOutput {
	if sessionUUID == "" || !r.flags.Answer(routeHookReply) {
		return nil
	}
	asked := ParseAskUserQuestion(toolInput)
	if !asked.Replyable {
		return nil
	}
	rows, err := r.rows.ListGuidanceByDelivery(ctx, domain.DeliveryRouteHookReply, domain.DeliveryStatePending)
	if err != nil {
		log.Warn("[HookReply] could not list pre-answered questions", "err", err)
		return nil
	}
	for _, row := range slices.Backward(rows) { // oldest first
		if out := r.serve(ctx, row, sessionUUID, asked, toolInput); out != nil {
			return out
		}
	}
	return nil
}

func (r *hookReplyResolver) serve(ctx context.Context, row *session.GuidanceRequestData, sessionUUID string, asked AskedQuestion, toolInput map[string]interface{}) *hookipc.HookOutput {
	if row.AnsweredAt == nil || row.CancelledAt != nil || (row.AskerSessionUUID != sessionUUID && row.SessionUUID != sessionUUID) {
		return nil
	}
	payload, err := domain.ParseQuestionPayload([]byte(row.QuestionsJSON))
	if err != nil {
		return nil
	}
	qs := payload.Questions()
	if len(qs) != 1 || qs[0].MultiSelect || qs[0].Text != asked.Question || !slices.Contains(asked.Labels, row.Answer) {
		return nil
	}
	key := rawQuestionText(toolInput)
	if key == "" {
		return nil
	}
	id, err := uuid.Parse(row.ID)
	if err != nil {
		return nil
	}
	won, err := r.rows.SetGuidanceDelivery(ctx, id, domain.DeliveryRouteHookReply, domain.DeliveryStateSent, 1)
	if err != nil || !won {
		return nil // another hook took it, or the row moved: leave the dialog to the operator
	}
	if r.acks != nil {
		r.acks.Sent(SentDelivery{RowID: row.ID, SessionUUID: sessionUUID, Route: domain.DeliveryRouteHookReply})
	}
	return hookAllowWithAnswers(toolInput, map[string]string{key: row.Answer})
}

// rawQuestionText is the unmodified text of question 1: the answers object is
// keyed by it, not by the sanitized copy ParseAskUserQuestion returns.
func rawQuestionText(toolInput map[string]interface{}) string {
	qs, _ := toolInput["questions"].([]interface{})
	if len(qs) == 0 {
		return ""
	}
	first, _ := qs[0].(map[string]interface{})
	text, _ := first["question"].(string)
	return text
}

// hookAllowWithAnswers is the PreToolUse output that answers the dialog.
func hookAllowWithAnswers(toolInput map[string]interface{}, answers map[string]string) *hookipc.HookOutput {
	updated := make(map[string]any, len(toolInput)+1)
	for k, v := range toolInput {
		updated[k] = v
	}
	updated["answers"] = answers
	return &hookipc.HookOutput{HookSpecificOutput: hookipc.HookSpecificOutput{
		HookEventName:      "PreToolUse",
		PermissionDecision: "allow",
		UpdatedInput:       updated,
	}}
}
