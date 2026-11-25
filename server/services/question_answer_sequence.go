package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/panereply"
)

// question_answer_sequence.go routes a multi-question answer to the stepwise
// key driver (Story 3.5). The answer rides in QuestionAnswer.OptionLabel as a
// JSON array with one list of labels per question, for example
// [["Redis"],["A","C"]]; no proto field is added.

// DecodeSequencePicks parses the OptionLabel of a multi-question answer.
func DecodeSequencePicks(optionLabel string) ([][]string, error) {
	var picks [][]string
	if err := json.Unmarshal([]byte(optionLabel), &picks); err != nil {
		return nil, fmt.Errorf("a multi-question answer is a JSON list of label lists: %w", err)
	}
	return picks, nil
}

// EncodeSequencePicks is the inverse of DecodeSequencePicks.
func EncodeSequencePicks(picks [][]string) string {
	b, err := json.Marshal(picks)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// sequenceQuestions returns the questions of the durable row when it is a shape
// with a recorded key sequence; any other row falls through to the single-digit path.
func (r *AnswerRouter) sequenceQuestions(ctx context.Context, questionID string) ([]panereply.TabbedQuestion, bool) {
	if r.rows == nil {
		return nil, false
	}
	id, err := uuid.Parse(questionID)
	if err != nil {
		return nil, false
	}
	row, err := r.rows.GetGuidanceRequest(ctx, id)
	if err != nil || row.QuestionsJSON == "" || row.Status() != domain.GuidanceRequestStatusPending {
		return nil, false
	}
	return questionkit.TabbedFromRow(row)
}

// liveSequence types a multi-question answer into a visible session's dialog.
func (r *AnswerRouter) liveSequence(ctx context.Context, s routedSession, in QuestionAnswer, qs []panereply.TabbedQuestion) (questionkit.ActionResult, error) {
	picks, err := DecodeSequencePicks(in.OptionLabel)
	if err != nil {
		return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	}
	steps, err := panereply.PlanTabbedAnswer(qs, picks)
	if err != nil {
		return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	}
	if access.AccessFor(s.inst) != access.TerminalReadWrite {
		return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "answer this question in the open session"), nil
	}
	if r.guard != nil && r.guard.Active(ctx, s.inst) {
		return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, detailHumanActive), nil
	}
	refusal, err := access.EvaluateLocalWrite(ctx, inProcessPeerAddr, access.LocalWriteLocalCaller)
	if err != nil {
		return questionkit.Outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	}
	track := questionkit.BeginDelivery(ctx, r.rows, r.acks, in.QuestionID)
	track.Pending(ctx)
	out := r.visible().RunSequence(ctx, AnswerTargetVisible, sequenceInput{
		sessionID: in.SessionID, sessionUUID: s.inst.GetStableID(), questionID: in.QuestionID, replyID: in.ReplyID,
		steps: steps, summary: sequenceSummary(picks), question: qs[0].Text,
	}, s.inst, steerRequestFacts{refusal: refusal})
	if out.Err != nil {
		track.Finish(ctx, questionkit.ActionResult{}, out.Err, questionkit.SentDelivery{}, "")
		return questionkit.ActionResult{}, out.Err
	}
	res := sequenceActionResult(out)
	track.Finish(ctx, res, nil, questionkit.SentDelivery{
		RowID: in.QuestionID, SessionUUID: s.inst.GetStableID(), Route: domain.DeliveryRouteLiveKeystroke,
		Indeterminate: res.Outcome == sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE,
		Matches:       questionkit.TabbedHeadersMatch(qs),
	}, EncodeSequencePicks(picks))
	return res, nil
}

// sequenceActionResult maps a sequence outcome to the action vocabulary. Any
// answer typed in part and not finished (the dialog changed, or a later key's
// write failed) is STALE with StaleReasonAnswerPartial and the progress in
// Detail: neither Retry nor a second Submit is safe, Open session is the way out.
func sequenceActionResult(o sequenceOutcome) questionkit.ActionResult {
	if o.Partial {
		return questionkit.ActionResult{
			Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: questionkit.StaleReasonAnswerPartial,
			Detail: fmt.Sprintf("%d of %d keys sent", o.Written, o.Total),
		}
	}
	return mapReplyOutcomeValue(o.Outcome, o.RetryAfter)
}
