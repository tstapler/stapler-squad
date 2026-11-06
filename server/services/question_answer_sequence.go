package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// question_answer_sequence.go routes a multi-question answer to the stepwise
// key driver (Story 3.5). The answer rides in QuestionAnswer.OptionLabel as a
// JSON array with one list of labels per question, for example
// [["Redis"],["A","C"]]; no proto field is added.

// StaleReasonAnswerPartial is ActionResult.StaleReason when part of a
// multi-question answer was typed and the dialog then changed. The card keeps
// Submit disabled and offers Open session or a confirmed Dismiss, never Retry.
const StaleReasonAnswerPartial = "answer_aborted_partial"

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
func (r *AnswerRouter) sequenceQuestions(ctx context.Context, questionID string) ([]session.TabbedQuestion, bool) {
	if r.rows == nil {
		return nil, false
	}
	id, err := uuid.Parse(questionID)
	if err != nil {
		return nil, false
	}
	row, err := r.rows.GetGuidanceRequest(ctx, id)
	if err != nil || row.QuestionsJSON == "" || row.Status() != session.GuidanceRequestStatusPending {
		return nil, false
	}
	return tabbedFromRow(row)
}

func tabbedQuestions(p domain.QuestionPayload) []session.TabbedQuestion {
	src := p.Questions()
	out := make([]session.TabbedQuestion, len(src))
	for i, q := range src {
		labels := make([]string, len(q.Options))
		for j, o := range q.Options {
			labels[j] = o.Label
		}
		out[i] = session.TabbedQuestion{Text: q.Text, Header: q.Header, Labels: labels, MultiSelect: q.MultiSelect}
	}
	return out
}

// liveSequence types a multi-question answer into a visible session's dialog.
func (r *AnswerRouter) liveSequence(ctx context.Context, s routedSession, in QuestionAnswer, qs []session.TabbedQuestion) (ActionResult, error) {
	picks, err := DecodeSequencePicks(in.OptionLabel)
	if err != nil {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	}
	steps, err := session.PlanTabbedAnswer(qs, picks)
	if err != nil {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	}
	if AccessFor(s.inst) != TerminalReadWrite {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "answer this question in the open session"), nil
	}
	if r.guard != nil && r.guard.Active(ctx, s.inst) {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, detailHumanActive), nil
	}
	refusal, err := EvaluateLocalWrite(ctx, inProcessPeerAddr, LocalWriteLocalCaller)
	if err != nil {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, err.Error()), nil
	}
	track := r.beginDelivery(ctx, in.QuestionID)
	track.pending(ctx)
	out := r.visible().RunSequence(ctx, AnswerTargetVisible, sequenceInput{
		sessionID: in.SessionID, sessionUUID: s.inst.GetStableID(), questionID: in.QuestionID, replyID: in.ReplyID,
		steps: steps, summary: sequenceSummary(picks), question: qs[0].Text,
	}, s.inst, steerRequestFacts{refusal: refusal})
	if out.Err != nil {
		track.finish(ctx, ActionResult{}, out.Err, SentDelivery{}, "")
		return ActionResult{}, out.Err
	}
	res := sequenceActionResult(out)
	track.finish(ctx, res, nil, SentDelivery{
		RowID: in.QuestionID, SessionUUID: s.inst.GetStableID(), Route: domain.DeliveryRouteLiveKeystroke,
		Indeterminate: res.Outcome == sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE,
		Matches:       tabbedHeadersMatch(qs),
	}, EncodeSequencePicks(picks))
	return res, nil
}

// sequenceActionResult maps a sequence outcome to the action vocabulary. Any
// answer typed in part and not finished (the dialog changed, or a later key's
// write failed) is STALE with StaleReasonAnswerPartial and the progress in
// Detail: neither Retry nor a second Submit is safe, Open session is the way out.
func sequenceActionResult(o sequenceOutcome) ActionResult {
	if o.Partial {
		return ActionResult{
			Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: StaleReasonAnswerPartial,
			Detail: fmt.Sprintf("%d of %d keys sent", o.Written, o.Total),
		}
	}
	return mapReplyOutcomeValue(o.Outcome, o.RetryAfter)
}
