package services

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// setGuidanceMirrorFields copies the ADR-003 columns onto the wire message.
// A questions_json that no longer parses is logged and sent as no questions:
// the row still renders from question_text and options.
func setGuidanceMirrorFields(out *sessionv1.GuidanceRequest, d *session.GuidanceRequestData) {
	out.Source = string(d.Source)
	out.AskerSessionUuid = d.AskerSessionUUID
	out.AnswererKind = string(d.AnswererKind)
	out.AnswererSessionUuid = d.AnswererSessionUUID
	out.MultiSelect = d.MultiSelect
	out.DeliveryRoute = string(d.DeliveryRoute)
	out.DeliveryState = string(d.DeliveryState)
	out.DeliveryAttempts = int32(d.DeliveryAttempts) //nolint:gosec // bounded retry counter
	out.HookCallId = string(d.HookCallID)
	if d.ExpiresAt != nil {
		out.ExpiresAt = timestamppb.New(*d.ExpiresAt)
	}
	if d.QuestionsJSON == "" {
		return
	}
	payload, err := domain.ParseQuestionPayload([]byte(d.QuestionsJSON))
	if err != nil {
		log.Warn("[GuidanceRequest] questions_json does not parse; sending no questions", "id", d.ID, "err", err)
		return
	}
	for _, q := range payload.Questions() {
		pq := &sessionv1.GuidanceQuestion{Question: q.Text, Header: q.Header, MultiSelect: q.MultiSelect}
		for _, o := range q.Options {
			pq.Options = append(pq.Options, &sessionv1.GuidanceOption{Label: o.Label, Description: o.Description})
		}
		out.Questions = append(out.Questions, pq)
	}
}
